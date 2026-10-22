package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Server struct {
	store     *Store
	staticDir string
	mcp       http.Handler
}

func NewServer(store *Store, staticDir string) http.Handler {
	s := &Server{store: store, staticDir: staticDir}
	s.mcp = newMCPHandler(store)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "time": time.Now().UTC()})
	})
	mux.HandleFunc("GET /api/project", s.getProject)
	mux.HandleFunc("POST /api/discovery", s.discover)
	mux.HandleFunc("POST /api/mutations", s.mutate)
	mux.HandleFunc("POST /api/proposals", s.createProposal)
	mux.HandleFunc("POST /api/proposals/{id}/review", s.reviewProposal)
	mux.HandleFunc("POST /api/presence", s.presence)
	mux.HandleFunc("GET /api/events", s.events)
	mux.Handle("/mcp", s.mcp)
	mux.HandleFunc("/", s.static)
	return withCORS(mux)
}

func (s *Server) discover(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Path     string `json:"path"`
		MaxFiles int    `json:"maxFiles"`
	}
	if !decode(w, r, &req) {
		return
	}
	report, err := discoverRepository(req.Path, req.MaxFiles)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) getProject(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.store.Snapshot())
}

func (s *Server) mutate(w http.ResponseWriter, r *http.Request) {
	var req MutationRequest
	if !decode(w, r, &req) {
		return
	}
	p, err := s.store.Mutate(req)
	respond(w, p, err)
}

func (s *Server) createProposal(w http.ResponseWriter, r *http.Request) {
	var req Proposal
	if !decode(w, r, &req) {
		return
	}
	p, err := s.store.SaveProposal(req)
	respond(w, p, err)
}

func (s *Server) reviewProposal(w http.ResponseWriter, r *http.Request) {
	var req ReviewRequest
	if !decode(w, r, &req) {
		return
	}
	p, err := s.store.Review(r.PathValue("id"), req)
	respond(w, p, err)
}

func (s *Server) presence(w http.ResponseWriter, r *http.Request) {
	var req Presence
	if !decode(w, r, &req) {
		return
	}
	if req.SessionID == "" || req.Name == "" {
		writeError(w, http.StatusBadRequest, "sessionId and name required")
		return
	}
	writeJSON(w, http.StatusOK, s.store.Presence(req))
}

func (s *Server) events(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, http.StatusInternalServerError, "streaming unsupported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch := s.store.Subscribe()
	defer s.store.Unsubscribe(ch)
	fmt.Fprint(w, "event: ready\ndata: {}\n\n")
	flusher.Flush()
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case payload := <-ch:
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", payload)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprint(w, ": keepalive\n\n")
			flusher.Flush()
		}
	}
}

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	clean := filepath.Clean(strings.TrimPrefix(r.URL.Path, "/"))
	if clean == "." {
		clean = "index.html"
	}
	path := filepath.Join(s.staticDir, clean)
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		path = filepath.Join(s.staticDir, "index.html")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		writeError(w, http.StatusNotFound, "frontend not built; run npm run build or npm run dev")
		return
	}
	if ext := filepath.Ext(path); ext != "" {
		w.Header().Set("Content-Type", mimeType(ext))
	}
	_, _ = w.Write(data)
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil && !errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, err.Error())
		return false
	}
	return true
}
func respond(w http.ResponseWriter, p Project, err error) {
	if err == nil {
		writeJSON(w, http.StatusOK, p)
		return
	}
	status := http.StatusBadRequest
	if errors.Is(err, ErrConflict) {
		status = http.StatusConflict
	}
	writeError(w, status, err.Error())
}
func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Mcp-Protocol-Version, Mcp-Session-Id")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func mimeType(ext string) string {
	switch ext {
	case ".html":
		return "text/html; charset=utf-8"
	case ".js":
		return "text/javascript"
	case ".css":
		return "text/css"
	case ".svg":
		return "image/svg+xml"
	default:
		return "application/octet-stream"
	}
}
