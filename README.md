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
- Open **Discover repository** from the left rail to inspect a configured local codebase. Discovery is read-only and reports languages, frameworks, source components, symbols, and evidence-backed dependency edges.

## Repository discovery

The scanner skips generated/dependency directories, limits individual source reads to 2 MiB, and never writes to the inspected repository or the architecture graph. Run it directly against any local project:

```powershell
npm run discover -- --root H:\ProjectsFolder\YourProject
```

The browser API and MCP tool are restricted to explicitly configured roots. Multiple roots use the operating system path-list separator (`;` on Windows, `:` on macOS/Linux):

```powershell
$env:CODEFLOW_DISCOVERY_ROOTS = 'H:\ProjectsFolder'
npm run dev
```

Unreal C++ includes are discovered, but binary Blueprint and asset relationships require a future Unreal Asset Registry export. Godot scene resources, Go module imports, JavaScript/TypeScript relative imports, and local C/C++ includes are supported in the prototype.

## Agent bridge

The Go service uses the official `github.com/modelcontextprotocol/go-sdk` and exposes a standards-compliant, stateless Streamable HTTP endpoint at `/mcp` with:

- `resources/list` and `resources/read` for `architecture://projects/codeflow/graph`
- `tools/list`
- `read_architecture`
- `discover_repository` (read-only and restricted to `CODEFLOW_DISCOVERY_ROOTS`)
- `propose_architecture_change`

Proposed operations are validated against the canonical graph but are not applied until human review. Mutations and proposal acceptance use optimistic `baseRevision` checks.

For local development, the MCP endpoint is open when no token variables are configured. For a shared deployment, set one or both variables before starting the server:

```powershell
$env:CODEFLOW_MCP_READ_TOKEN = '<random-read-token>'
$env:CODEFLOW_MCP_WRITE_TOKEN = '<random-write-token>'
go run ./cmd/server
```

Send the selected value as `Authorization: Bearer <token>`. The read token can discover the graph resource and call `read_architecture` or scoped `discover_repository`; only the write token can discover or call `propose_architecture_change`. Token comparisons are constant-time, and invalid or missing credentials receive `401 Unauthorized` once token mode is enabled.

## Verification

```powershell
npm test
```

This runs Go persistence/revision/proposal tests and a strict TypeScript production build. MCP integration tests connect the official SDK client to the HTTP server, discover tools/resources, read the graph, submit a review-gated proposal, and verify read/write token scopes.

The graph test suite also covers atomic multi-operation creation, dangling-reference cleanup, and guarded deletion of parent components.
