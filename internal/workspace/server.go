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
}

func NewServer(store *Store, staticDir string) http.Handler {
	s := &Server{store: store, staticDir: staticDir}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "time": time.Now().UTC()})
	})
	mux.HandleFunc("GET /api/project", s.getProject)
	mux.HandleFunc("POST /api/mutations", s.mutate)
	mux.HandleFunc("POST /api/proposals", s.createProposal)
	mux.HandleFunc("POST /api/proposals/{id}/review", s.reviewProposal)
	mux.HandleFunc("POST /api/presence", s.presence)
	mux.HandleFunc("GET /api/events", s.events)
	mux.HandleFunc("POST /mcp", s.mcp)
	mux.HandleFunc("/", s.static)
	return withCORS(mux)
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

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}
type rpcResponse struct {
	JSONRPC string `json:"jsonrpc"`
	ID      any    `json:"id"`
	Result  any    `json:"result,omitempty"`
	Error   any    `json:"error,omitempty"`
}

func (s *Server) mcp(w http.ResponseWriter, r *http.Request) {
	var req rpcRequest
	if !decode(w, r, &req) {
		return
	}
	res := rpcResponse{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		res.Result = map[string]any{"protocolVersion": "2025-03-26", "capabilities": map[string]any{"tools": map[string]any{}, "resources": map[string]any{}}, "serverInfo": map[string]any{"name": "codeflow", "version": "0.1.0"}}
	case "resources/list":
		res.Result = map[string]any{"resources": []map[string]any{{"uri": "architecture://projects/codeflow/graph", "name": "Commerce Platform architecture graph", "mimeType": "application/json"}}}
	case "resources/read":
		data, _ := json.MarshalIndent(s.store.Snapshot(), "", "  ")
		res.Result = map[string]any{"contents": []map[string]any{{"uri": "architecture://projects/codeflow/graph", "mimeType": "application/json", "text": string(data)}}}
	case "tools/list":
		res.Result = map[string]any{"tools": []map[string]any{
			{"name": "read_architecture", "description": "Read the canonical project graph and current revision", "inputSchema": map[string]any{"type": "object", "properties": map[string]any{}}},
			{"name": "propose_architecture_change", "description": "Submit reviewable operations without mutating the authoritative graph", "inputSchema": map[string]any{"type": "object", "required": []string{"id", "title", "baseRevision", "operations"}}},
		}}
	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		_ = json.Unmarshal(req.Params, &params)
		if params.Name == "read_architecture" {
			data, _ := json.MarshalIndent(s.store.Snapshot(), "", "  ")
			res.Result = map[string]any{"content": []map[string]string{{"type": "text", "text": string(data)}}}
		} else if params.Name == "propose_architecture_change" {
			var p Proposal
			if err := json.Unmarshal(params.Arguments, &p); err != nil {
				res.Error = rpcErr(-32602, err.Error())
				break
			}
			if p.Author == "" {
				p.Author = "MCP agent"
			}
			p.AuthorType = "agent"
			project, err := s.store.SaveProposal(p)
			if err != nil {
				res.Error = rpcErr(-32000, err.Error())
				break
			}
			data, _ := json.Marshal(map[string]any{"proposalId": p.ID, "status": "pending", "revision": project.Revision})
			res.Result = map[string]any{"content": []map[string]string{{"type": "text", "text": string(data)}}}
		} else {
			res.Error = rpcErr(-32601, "tool not found")
		}
	default:
		res.Error = rpcErr(-32601, "method not found")
	}
	writeJSON(w, http.StatusOK, res)
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
func rpcErr(code int, msg string) map[string]any { return map[string]any{"code": code, "message": msg} }
func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
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
