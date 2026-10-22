# CodeFlow — Visual Architecture Workspace prototype

A self-hosted prototype of the Linear **Visual Architecture Workspace** plan. One canonical, revisioned architecture graph drives a spatial canvas, nested outline, typed contract panels, realtime browser updates, and review-gated agent proposals.

## Run locally

Prerequisites: Node 20+ and Go 1.24+.

```powershell
npm install
npm run dev
```

Open <http://localhost:5173>. The Vite frontend proxies the Go API at <http://localhost:8080>.

For the production-style single-server build:

```powershell
npm run build
go run ./cmd/server
```

Then open <http://localhost:8080>. Project state is stored in `data/project.json` and survives restarts.

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

The Go service exposes a compact MCP Streamable HTTP-style JSON-RPC endpoint at `POST /mcp` with:

- `resources/list` and `resources/read` for `architecture://projects/codeflow/graph`
- `tools/list`
- `read_architecture`
- `propose_architecture_change`

Proposed operations are validated against the canonical graph but are not applied until human review. Mutations and proposal acceptance use optimistic `baseRevision` checks.

## Verification

```powershell
npm test
```

This runs Go persistence/revision/proposal tests and a strict TypeScript production build.

The graph test suite also covers atomic multi-operation creation, dangling-reference cleanup, and guarded deletion of parent components.
