# CodeFlow — Visual Architecture Workspace prototype

A self-hosted prototype of the Linear **Visual Architecture Workspace** plan. One canonical, revisioned architecture graph drives a spatial canvas, nested outline, typed contract panels, realtime browser updates, and review-gated agent proposals.

## Run locally

Prerequisites: Node 20+ and Go 1.25+.

```powershell
npm install
npm run dev
```

Open <http://localhost:5173>. The Vite frontend proxies the Go API at <http://localhost:8787>.

For the production-style single-server build:

```powershell
npm run build
go run ./cmd/server
```

Then open <http://localhost:8787>. Project state is stored in `data/project.json` and survives restarts.

## Prototype workflows

- Select the same stable component from the canvas or outline.
- Add typed services, databases, clients, APIs, or generic components and choose their semantic parent.
- Connect components by dragging between node handles; relationships use the canonical graph and survive reloads.
- Drag a node and reload to see its view position persist independently of semantic graph data.
- Edit or safely delete a component in the inspector. Deletion removes its blocks and relationships while refusing to orphan child components.
- Add, edit, and remove requirements, behaviors, API contracts, table schemas, constraints, code sketches, and notes.
- Open **Activity** to inspect the current revision and attributed graph history.
- Open **Review** to inspect the seeded agent proposal. Accepting atomically adds its component, edge, and constraint; rejecting leaves the graph unchanged.
- Open two browser sessions. Graph changes arrive over server-sent events, and active selections appear as presence avatars.
- Double-click a component containing children to enter its semantic scope; use breadcrumbs to zoom out.

## Agent bridge

The Go service uses the official `github.com/modelcontextprotocol/go-sdk` and exposes a standards-compliant, stateless Streamable HTTP endpoint at `/mcp` with:

- `resources/list` and `resources/read` for `architecture://projects/codeflow/graph`
- `tools/list`
- `read_architecture`
- `propose_architecture_change`

Proposed operations are validated against the canonical graph but are not applied until human review. Mutations and proposal acceptance use optimistic `baseRevision` checks.

For local development, the MCP endpoint is open when no token variables are configured. For a shared deployment, set one or both variables before starting the server:

```powershell
$env:CODEFLOW_MCP_READ_TOKEN = '<random-read-token>'
$env:CODEFLOW_MCP_WRITE_TOKEN = '<random-write-token>'
go run ./cmd/server
```

Send the selected value as `Authorization: Bearer <token>`. The read token can discover the graph resource and `read_architecture`; only the write token can discover or call `propose_architecture_change`. Token comparisons are constant-time, and invalid or missing credentials receive `401 Unauthorized` once token mode is enabled.

## Verification

```powershell
npm test
```

This runs Go persistence/revision/proposal tests and a strict TypeScript production build. MCP integration tests connect the official SDK client to the HTTP server, discover tools/resources, read the graph, submit a review-gated proposal, and verify read/write token scopes.

The graph test suite also covers atomic multi-operation creation, dangling-reference cleanup, and guarded deletion of parent components.
