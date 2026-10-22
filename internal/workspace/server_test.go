package workspace

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectAPIAndMCPDiscovery(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(store, filepath.Join(t.TempDir(), "dist"))

	projectReq := httptest.NewRequest(http.MethodGet, "/api/project", nil)
	projectRes := httptest.NewRecorder()
	server.ServeHTTP(projectRes, projectReq)
	if projectRes.Code != http.StatusOK || !strings.Contains(projectRes.Body.String(), "cmp_backend") {
		t.Fatalf("project response: %d %s", projectRes.Code, projectRes.Body.String())
	}

	rpc := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`)
	mcpReq := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(rpc))
	mcpReq.Header.Set("Content-Type", "application/json")
	mcpRes := httptest.NewRecorder()
	server.ServeHTTP(mcpRes, mcpReq)
	if mcpRes.Code != http.StatusOK || !strings.Contains(mcpRes.Body.String(), "propose_architecture_change") {
		t.Fatalf("MCP response: %d %s", mcpRes.Code, mcpRes.Body.String())
	}
}

func TestMCPProposalIsReviewGated(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	server := NewServer(store, filepath.Join(t.TempDir(), "dist"))
	before := store.Snapshot()
	arguments := Proposal{
		ID: "prop_mcp_test", Title: "Add audit service", BaseRevision: before.Revision,
		Operations: []Operation{{Kind: "add_component", Component: &Component{ID: "cmp_audit", Type: "service", Title: "Audit service", ParentID: "cmp_platform", Provenance: Provenance{Kind: "inferred", Author: "test", Confidence: .8}}}},
	}
	args, _ := json.Marshal(arguments)
	params, _ := json.Marshal(map[string]any{"name": "propose_architecture_change", "arguments": json.RawMessage(args)})
	body, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/call", "params": json.RawMessage(params)})
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(body))
	res := httptest.NewRecorder()
	server.ServeHTTP(res, req)
	if res.Code != http.StatusOK || !strings.Contains(res.Body.String(), "pending") {
		t.Fatalf("proposal response: %d %s", res.Code, res.Body.String())
	}
	after := store.Snapshot()
	if componentExists(&after, "cmp_audit") {
		t.Fatal("MCP proposal mutated authoritative graph before review")
	}
	if len(after.Proposals) != len(before.Proposals)+1 {
		t.Fatal("proposal was not recorded")
	}
}
