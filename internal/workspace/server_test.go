package workspace

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
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
	if slices.ContainsFunc(readerTools.Tools, func(tool *mcp.Tool) bool {
		return tool.Name == "propose_architecture_change" || tool.Name == "propose_discovered_architecture"
	}) {
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
	if !slices.ContainsFunc(writerTools.Tools, func(tool *mcp.Tool) bool { return tool.Name == "propose_discovered_architecture" }) {
		t.Fatal("write token did not receive discovery proposal tool")
	}
}

func TestMCPOAuthMetadataJWTValidationAndScopes(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	const keyID = "codeflow-test-key"
	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"keys": []any{map[string]any{
			"kty": "RSA", "kid": keyID, "use": "sig", "alg": "RS256",
			"n": base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes()), "e": "AQAB",
		}}})
	}))
	defer jwksServer.Close()

	const resource = "https://codeflow.example.test"
	const issuer = "https://identity.example.test/"
	t.Setenv("CODEFLOW_MCP_RESOURCE", resource)
	t.Setenv("CODEFLOW_MCP_OAUTH_ISSUER", issuer)
	t.Setenv("CODEFLOW_MCP_OAUTH_JWKS_URL", jwksServer.URL)
	store, _ := NewStore(filepath.Join(t.TempDir(), "project.json"))
	httpServer := httptest.NewServer(NewServer(store, filepath.Join(t.TempDir(), "dist")))
	defer httpServer.Close()

	metadataResponse, err := http.Get(httpServer.URL + "/.well-known/oauth-protected-resource")
	if err != nil {
		t.Fatal(err)
	}
	defer metadataResponse.Body.Close()
	var metadata map[string]any
	if err := json.NewDecoder(metadataResponse.Body).Decode(&metadata); err != nil {
		t.Fatal(err)
	}
	if metadataResponse.StatusCode != http.StatusOK || metadata["resource"] != resource {
		t.Fatalf("protected resource metadata = %d %#v", metadataResponse.StatusCode, metadata)
	}

	unauthorized, err := http.Post(httpServer.URL+"/mcp", "application/json", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	defer unauthorized.Body.Close()
	challenge := unauthorized.Header.Get("WWW-Authenticate")
	if unauthorized.StatusCode != http.StatusUnauthorized || !strings.Contains(challenge, "resource_metadata=") || !strings.Contains(challenge, defaultMCPWriteScope) {
		t.Fatalf("OAuth challenge = %d %q", unauthorized.StatusCode, challenge)
	}

	readJWT := signMCPTestJWT(t, privateKey, keyID, issuer, resource, []string{defaultMCPReadScope})
	reader := connectMCP(t, httpServer.URL+"/mcp", readJWT)
	defer reader.Close()
	readerTools, err := reader.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(readerTools.Tools, func(tool *mcp.Tool) bool { return tool.Name == "propose_architecture_change" }) {
		t.Fatal("OAuth read scope received write-capable tools")
	}
	readToolIndex := slices.IndexFunc(readerTools.Tools, func(tool *mcp.Tool) bool { return tool.Name == "read_architecture" })
	if readToolIndex < 0 {
		t.Fatal("read tool was not discovered")
	}
	readTool := readerTools.Tools[readToolIndex]
	if readTool.Meta == nil || readTool.Meta["securitySchemes"] == nil {
		t.Fatal("read tool did not advertise its OAuth security scheme")
	}

	writeJWT := signMCPTestJWT(t, privateKey, keyID, issuer, resource, []string{defaultMCPReadScope, defaultMCPWriteScope})
	writer := connectMCP(t, httpServer.URL+"/mcp", writeJWT)
	defer writer.Close()
	writerTools, err := writer.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(writerTools.Tools, func(tool *mcp.Tool) bool { return tool.Name == "propose_architecture_change" }) {
		t.Fatal("OAuth write scope did not receive write-capable tools")
	}

	wrongAudience := signMCPTestJWT(t, privateKey, keyID, issuer, "https://other.example.test", []string{defaultMCPReadScope, defaultMCPWriteScope})
	request, _ := http.NewRequest(http.MethodPost, httpServer.URL+"/mcp", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+wrongAudience)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong-audience token status = %d", response.StatusCode)
	}

	writeOnly := signMCPTestJWT(t, privateKey, keyID, issuer, resource, []string{defaultMCPWriteScope})
	request, _ = http.NewRequest(http.MethodPost, httpServer.URL+"/mcp", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+writeOnly)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("write-only token status = %d", response.StatusCode)
	}
}

func signMCPTestJWT(t *testing.T, privateKey *rsa.PrivateKey, keyID, issuer, audience string, scopes []string) string {
	t.Helper()
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": issuer, "aud": audience, "sub": "codeflow-test-user", "scope": strings.Join(scopes, " "),
		"iat": now.Unix(), "nbf": now.Add(-time.Second).Unix(), "exp": now.Add(time.Hour).Unix(),
	})
	token.Header["kid"] = keyID
	signed, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func TestOfficialMCPRepositoryDiscoveryIsReadOnlyAndScoped(t *testing.T) {
	repository := t.TempDir()
	if err := os.WriteFile(filepath.Join(repository, "go.mod"), []byte("module fixture.test/repo\n\ngo 1.25\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repository, "cmd", "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repository, "internal", "store"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "cmd", "api", "main.go"), []byte("package main\nimport \"fixture.test/repo/internal/store\"\nfunc main() { store.Open() }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "internal", "store", "store.go"), []byte("package store\nfunc Open() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEFLOW_DISCOVERY_ROOTS", repository)
	store, _ := NewStore(filepath.Join(t.TempDir(), "project.json"))
	httpServer := httptest.NewServer(NewServer(store, filepath.Join(t.TempDir(), "dist")))
	defer httpServer.Close()
	session := connectMCP(t, httpServer.URL+"/mcp", "")
	defer session.Close()
	before := store.Snapshot()
	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "discover_repository", Arguments: discoverRepositoryInput{Path: repository}})
	if err != nil || result.IsError {
		t.Fatalf("repository discovery failed: err=%v result=%#v", err, result)
	}
	after := store.Snapshot()
	if after.Revision != before.Revision || len(after.Components) != len(before.Components) {
		t.Fatal("read-only discovery mutated the architecture graph")
	}
	proposalResult, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "propose_discovered_architecture", Arguments: proposeDiscoveredArchitectureInput{Path: repository, BaseRevision: before.Revision, Author: "integration-test"}})
	if err != nil || proposalResult.IsError {
		t.Fatalf("discovered architecture proposal failed: err=%v result=%#v", err, proposalResult)
	}
	proposed := store.Snapshot()
	if proposed.Revision != before.Revision || len(proposed.Components) != len(before.Components) || len(proposed.Proposals) != len(before.Proposals)+1 {
		t.Fatal("discovery proposal bypassed review or was not recorded")
	}
	proposal := proposed.Proposals[0]
	if len(proposal.Operations) < 4 {
		t.Fatalf("proposal operations = %d, want repository, packages, and dependency", len(proposal.Operations))
	}
	accepted, err := store.Review(proposal.ID, ReviewRequest{Reviewer: "integration-test", Decision: "accepted"})
	if err != nil {
		t.Fatal(err)
	}
	if len(accepted.Components) < len(before.Components)+3 || len(accepted.Relationships) < len(before.Relationships)+1 {
		t.Fatal("accepted discovery proposal did not create the repository chart")
	}
	upToDate, err := proposeDiscoveredArchitecture(store, DiscoveryProposalRequest{Path: repository, BaseRevision: accepted.Revision})
	if err != nil || upToDate.Status != "up_to_date" || upToDate.OperationCount != 0 {
		t.Fatalf("repeat discovery was not idempotent: result=%#v err=%v", upToDate, err)
	}
	outside := filepath.Dir(repository)
	blocked, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "discover_repository", Arguments: discoverRepositoryInput{Path: outside}})
	if err == nil && !blocked.IsError {
		t.Fatal("discovery read outside configured roots")
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
