package workspace

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestProjectAPIAndOfficialMCPDiscovery(t *testing.T) {
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

	httpServer := httptest.NewServer(server)
	defer httpServer.Close()
	session := connectMCP(t, httpServer.URL+"/mcp", "")
	defer session.Close()
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(tools.Tools, func(tool *mcp.Tool) bool { return tool.Name == "propose_architecture_change" }) {
		t.Fatal("write-capable MCP tool was not discovered")
	}
	resource, err := session.ReadResource(context.Background(), &mcp.ReadResourceParams{URI: graphResourceURI})
	if err != nil {
		t.Fatal(err)
	}
	if len(resource.Contents) != 1 || !strings.Contains(resource.Contents[0].Text, "cmp_backend") {
		t.Fatal("canonical graph resource was not readable")
	}
}

func TestOfficialMCPProposalIsReviewGated(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	httpServer := httptest.NewServer(NewServer(store, filepath.Join(t.TempDir(), "dist")))
	defer httpServer.Close()
	session := connectMCP(t, httpServer.URL+"/mcp", "")
	defer session.Close()
	before := store.Snapshot()
	input := proposeArchitectureInput{
		ID: "prop_mcp_test", Title: "Add audit service", Summary: "Centralize immutable security events", Author: "integration-test", BaseRevision: before.Revision,
		Operations: []Operation{{Kind: "add_component", Component: &Component{ID: "cmp_audit", Type: "service", Title: "Audit service", ParentID: "cmp_platform", Provenance: Provenance{Kind: "inferred", Author: "integration-test", Confidence: .8}}}},
	}
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "propose_architecture_change", Arguments: input})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("proposal tool returned an error: %#v", result.Content)
	}
	after := store.Snapshot()
	if componentExists(&after, "cmp_audit") {
		t.Fatal("MCP proposal mutated authoritative graph before review")
	}
	if len(after.Proposals) != len(before.Proposals)+1 || after.Proposals[0].ID != input.ID {
		t.Fatal("proposal was not recorded")
	}
}

func TestMCPBearerScopes(t *testing.T) {
	t.Setenv("CODEFLOW_MCP_READ_TOKEN", "reader-secret")
	t.Setenv("CODEFLOW_MCP_WRITE_TOKEN", "writer-secret")
	store, _ := NewStore(filepath.Join(t.TempDir(), "project.json"))
	server := NewServer(store, filepath.Join(t.TempDir(), "dist"))
	httpServer := httptest.NewServer(server)
	defer httpServer.Close()

	unauthorized, err := http.Post(httpServer.URL+"/mcp", "application/json", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	defer unauthorized.Body.Close()
	if unauthorized.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.StatusCode)
	}

	reader := connectMCP(t, httpServer.URL+"/mcp", "reader-secret")
	defer reader.Close()
	readerTools, err := reader.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(readerTools.Tools, func(tool *mcp.Tool) bool { return tool.Name == "propose_architecture_change" }) {
		t.Fatal("read token received write-capable tool")
	}
	writer := connectMCP(t, httpServer.URL+"/mcp", "writer-secret")
	defer writer.Close()
	writerTools, err := writer.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(writerTools.Tools, func(tool *mcp.Tool) bool { return tool.Name == "propose_architecture_change" }) {
		t.Fatal("write token did not receive proposal tool")
	}
}

func connectMCP(t *testing.T, endpoint, token string) *mcp.ClientSession {
	t.Helper()
	client := mcp.NewClient(&mcp.Implementation{Name: "codeflow-test", Version: "v0.1.0"}, nil)
	httpClient := &http.Client{}
	if token != "" {
		httpClient.Transport = bearerTransport{token: token, next: http.DefaultTransport}
	}
	session, err := client.Connect(context.Background(), &mcp.StreamableClientTransport{Endpoint: endpoint, HTTPClient: httpClient, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return session
}

type bearerTransport struct {
	token string
	next  http.RoundTripper
}

func (t bearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	clone.Header.Set("Authorization", "Bearer "+t.token)
	return t.next.RoundTrip(clone)
}
