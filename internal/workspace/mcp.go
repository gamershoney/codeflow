package workspace

import (
	"context"
	"encoding/json"
	"net/http"

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
type proposeDiscoveredArchitectureInput struct {
	Path          string `json:"path" jsonschema:"repository directory inside CODEFLOW_DISCOVERY_ROOTS"`
	BaseRevision  int64  `json:"baseRevision" jsonschema:"architecture revision returned by read_architecture"`
	ParentID      string `json:"parentId,omitempty" jsonschema:"existing component that will contain the imported repository; defaults to the platform root"`
	Author        string `json:"author,omitempty" jsonschema:"agent identity used for proposal attribution"`
	MaxFiles      int    `json:"maxFiles,omitempty" jsonschema:"maximum files to inventory; defaults to 25000"`
	MaxComponents int    `json:"maxComponents,omitempty" jsonschema:"maximum discovered components to include; defaults to 50 and caps at 200"`
}
type proposeDiscoveredArchitectureOutput struct {
	ProposalID        string `json:"proposalId,omitempty"`
	Status            string `json:"status"`
	Revision          int64  `json:"revision"`
	OperationCount    int    `json:"operationCount"`
	ComponentCount    int    `json:"componentCount"`
	RelationshipCount int    `json:"relationshipCount"`
}

type mcpService struct {
	Handler         http.Handler
	MetadataHandler http.Handler
}

func newMCPService(store *Store) mcpService {
	settings, err := loadMCPAuthSettings()
	if err != nil {
		panic("invalid MCP authentication configuration: " + err.Error())
	}
	readServer := buildMCPServer(store, false, settings)
	writeServer := buildMCPServer(store, true, settings)
	transport := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server {
		if role, _ := r.Context().Value(roleContextKey{}).(mcpRole); role == roleRead {
			return readServer
		}
		return writeServer
	}, &mcp.StreamableHTTPOptions{JSONResponse: true, Stateless: true})
	return mcpService{Handler: authorizeMCP(transport, settings), MetadataHandler: settings.metadataHandler()}
}

func buildMCPServer(store *Store, writable bool, authSettings mcpAuthSettings) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{Name: "codeflow", Version: "v0.4.0"}, &mcp.ServerOptions{Instructions: "Read the current architecture revision before proposing changes. Repository discovery is read-only. All graph changes must be submitted as proposals for human review; never claim a proposal changed the canonical graph until it is accepted."})
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
	mcp.AddTool(server, &mcp.Tool{Name: "read_architecture", Description: "Read the canonical CodeFlow architecture graph and its current optimistic-lock revision.", Meta: authSettings.toolSecurityMeta(false)},
		func(_ context.Context, _ *mcp.CallToolRequest, _ readArchitectureInput) (*mcp.CallToolResult, readArchitectureOutput, error) {
			return nil, readArchitectureOutput{Project: store.Snapshot()}, nil
		})
	mcp.AddTool(server, &mcp.Tool{Name: "discover_repository", Description: "Read a configured local repository and return detected languages, frameworks, source components, symbols, and evidence-backed dependency edges. This tool never modifies repository files or the architecture graph.", Meta: authSettings.toolSecurityMeta(false)},
		func(_ context.Context, _ *mcp.CallToolRequest, input discoverRepositoryInput) (*mcp.CallToolResult, discovery.Report, error) {
			report, err := discoverRepository(input.Path, input.MaxFiles)
			if err != nil {
				return nil, discovery.Report{}, err
			}
			return nil, report, nil
		})
	if writable {
		mcp.AddTool(server, &mcp.Tool{Name: "propose_discovered_architecture", Description: "Rescan a configured repository, translate its packages and verified dependencies into additive graph operations, and save them as a review-gated proposal. Repeated calls are idempotent and never modify source files or the authoritative graph directly.", Meta: authSettings.toolSecurityMeta(true)},
			func(_ context.Context, _ *mcp.CallToolRequest, input proposeDiscoveredArchitectureInput) (*mcp.CallToolResult, proposeDiscoveredArchitectureOutput, error) {
				result, err := proposeDiscoveredArchitecture(store, DiscoveryProposalRequest{Path: input.Path, BaseRevision: input.BaseRevision, ParentID: input.ParentID, Author: input.Author, MaxFiles: input.MaxFiles, MaxComponents: input.MaxComponents})
				if err != nil {
					return nil, proposeDiscoveredArchitectureOutput{}, err
				}
				return nil, proposeDiscoveredArchitectureOutput{ProposalID: result.ProposalID, Status: result.Status, Revision: result.Revision, OperationCount: result.OperationCount, ComponentCount: result.ComponentCount, RelationshipCount: result.RelationshipCount}, nil
			})
		mcp.AddTool(server, &mcp.Tool{Name: "propose_architecture_change", Description: "Validate and submit an attributed, revision-bound architecture change for human review. This never mutates the authoritative graph directly.", Meta: authSettings.toolSecurityMeta(true)},
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
