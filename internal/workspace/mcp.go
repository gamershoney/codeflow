package workspace

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net/http"
	"os"
	"strings"

	"codeflow/internal/discovery"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const graphResourceURI = "architecture://projects/codeflow/graph"

type mcpRole string

const (
	roleRead  mcpRole = "read"
	roleWrite mcpRole = "write"
)

type roleContextKey struct{}
type readArchitectureInput struct{}
type readArchitectureOutput struct {
	Project Project `json:"project" jsonschema:"canonical architecture project and its current revision"`
}
type proposeArchitectureInput struct {
	ID           string      `json:"id" jsonschema:"stable unique proposal ID"`
	Title        string      `json:"title" jsonschema:"short human-readable proposal title"`
	Summary      string      `json:"summary" jsonschema:"reason and intended outcome"`
	Author       string      `json:"author" jsonschema:"agent identity used for attribution"`
	BaseRevision int64       `json:"baseRevision" jsonschema:"project revision this proposal was prepared against"`
	Operations   []Operation `json:"operations" jsonschema:"atomic architecture operations requiring human review"`
}
type proposeArchitectureOutput struct {
	ProposalID string `json:"proposalId"`
	Status     string `json:"status"`
	Revision   int64  `json:"revision"`
}
type discoverRepositoryInput struct {
	Path     string `json:"path" jsonschema:"absolute or configured-root-relative repository directory to inspect"`
	MaxFiles int    `json:"maxFiles,omitempty" jsonschema:"maximum number of files to inventory; defaults to 25000"`
}

func newMCPHandler(store *Store) http.Handler {
	readServer := buildMCPServer(store, false)
	writeServer := buildMCPServer(store, true)
	transport := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		if role, _ := r.Context().Value(roleContextKey{}).(mcpRole); role == roleRead {
			return readServer
		}
		return writeServer
	}, &mcp.StreamableHTTPOptions{JSONResponse: true, Stateless: true})
	return authorizeMCP(transport)
}

func buildMCPServer(store *Store, writable bool) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "codeflow", Version: "v0.2.0"}, nil)
	server.AddResource(&mcp.Resource{
		URI: graphResourceURI, Name: "codeflow-architecture-graph", Title: "CodeFlow architecture graph",
		Description: "Canonical typed architecture graph, blocks, relationships, proposals, views, and current revision.", MIMEType: "application/json",
	}, func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		data, err := json.MarshalIndent(store.Snapshot(), "", "  ")
		if err != nil {
			return nil, err
		}
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{URI: graphResourceURI, MIMEType: "application/json", Text: string(data)}}}, nil
	})
	mcp.AddTool(server, &mcp.Tool{Name: "read_architecture", Description: "Read the canonical CodeFlow architecture graph and its current optimistic-lock revision."},
		func(_ context.Context, _ *mcp.CallToolRequest, _ readArchitectureInput) (*mcp.CallToolResult, readArchitectureOutput, error) {
			return nil, readArchitectureOutput{Project: store.Snapshot()}, nil
		})
	mcp.AddTool(server, &mcp.Tool{Name: "discover_repository", Description: "Read a configured local repository and return detected languages, frameworks, source components, symbols, and evidence-backed dependency edges. This tool never modifies repository files or the architecture graph."},
		func(_ context.Context, _ *mcp.CallToolRequest, input discoverRepositoryInput) (*mcp.CallToolResult, discovery.Report, error) {
			report, err := discoverRepository(input.Path, input.MaxFiles)
			if err != nil {
				return nil, discovery.Report{}, err
			}
			return nil, report, nil
		})
	if writable {
		mcp.AddTool(server, &mcp.Tool{Name: "propose_architecture_change", Description: "Validate and submit an attributed, revision-bound architecture change for human review. This never mutates the authoritative graph directly."},
			func(_ context.Context, _ *mcp.CallToolRequest, input proposeArchitectureInput) (*mcp.CallToolResult, proposeArchitectureOutput, error) {
				project, err := store.SaveProposal(Proposal{ID: input.ID, Title: input.Title, Summary: input.Summary, Author: fallback(input.Author, "MCP agent"), AuthorType: "agent", BaseRevision: input.BaseRevision, Operations: input.Operations})
				if err != nil {
					return nil, proposeArchitectureOutput{}, err
				}
				return nil, proposeArchitectureOutput{ProposalID: input.ID, Status: "pending", Revision: project.Revision}, nil
			})
	}
	return server
}

func authorizeMCP(next http.Handler) http.Handler {
	readToken := os.Getenv("CODEFLOW_MCP_READ_TOKEN")
	writeToken := os.Getenv("CODEFLOW_MCP_WRITE_TOKEN")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role := roleWrite
		if readToken != "" || writeToken != "" {
			token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
			switch {
			case writeToken != "" && secureEqual(token, writeToken):
				role = roleWrite
			case readToken != "" && secureEqual(token, readToken):
				role = roleRead
			default:
				w.Header().Set("WWW-Authenticate", `Bearer realm="codeflow-mcp"`)
				writeError(w, http.StatusUnauthorized, "valid MCP bearer token required")
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), roleContextKey{}, role)))
	})
}

func secureEqual(got, want string) bool {
	if got == "" || want == "" || len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}
