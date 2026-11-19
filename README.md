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
- After a scan, choose **Create chart proposal** to translate the evidence into an additive architecture proposal. Review the generated components and relationships before accepting them into the canonical graph.

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

Repository imports are additive and idempotent: rescanning an already accepted repository reports it as up to date, while newly discovered components and dependencies become another review-gated proposal. The bridge never deletes existing graph data automatically.

Unreal C++ includes are discovered, but binary Blueprint and asset relationships require a future Unreal Asset Registry export. Godot scene resources, Go module imports, JavaScript/TypeScript relative imports, local C/C++ includes, and Odin packages/procedures/local imports are supported in the prototype.

## Agent bridge

The Go service uses the official `github.com/modelcontextprotocol/go-sdk` and exposes a standards-compliant, stateless Streamable HTTP endpoint at `/mcp` with:

- `resources/list` and `resources/read` for `architecture://projects/codeflow/graph`
- `tools/list`
- `read_architecture`
- `discover_repository` (read-only and restricted to `CODEFLOW_DISCOVERY_ROOTS`)
- `propose_discovered_architecture` (rescans a scoped repository and creates an additive, review-gated chart proposal)
- `propose_architecture_change`

Proposed operations are validated against the canonical graph but are not applied until human review. Mutations and proposal acceptance use optimistic `baseRevision` checks.

For local development, the MCP endpoint is open when no authentication variables are configured. Never expose that mode outside a trusted machine. Local Codex clients can use fixed read/write tokens:

```powershell
$env:CODEFLOW_MCP_READ_TOKEN = '<random-read-token>'
$env:CODEFLOW_MCP_WRITE_TOKEN = '<random-write-token>'
go run ./cmd/server
```

Send the selected value as `Authorization: Bearer <token>`. The read token can discover the graph resource and call `read_architecture` or scoped `discover_repository`; only the write token can create proposals with `propose_discovered_architecture` or `propose_architecture_change`. Token comparisons are constant-time, and invalid or missing credentials receive `401 Unauthorized` once token mode is enabled.

### OAuth for ChatGPT and remote clients

For a remote ChatGPT plugin, configure CodeFlow as an OAuth 2.1 resource server backed by an established identity provider such as Auth0. The provider must issue signed JWT access tokens with the expected issuer, audience, expiration, and `codeflow:read` / `codeflow:write` scopes.

```powershell
$env:CODEFLOW_MCP_RESOURCE = 'https://codeflow.example.com'
$env:CODEFLOW_MCP_OAUTH_ISSUER = 'https://your-tenant.example-idp.com/'
$env:CODEFLOW_MCP_OAUTH_JWKS_URL = 'https://your-tenant.example-idp.com/.well-known/jwks.json'
# Optional when the provider audience differs from the resource identifier:
$env:CODEFLOW_MCP_OAUTH_AUDIENCE = 'https://codeflow.example.com'
go run ./cmd/server
```

Connect the client to `https://codeflow.example.com/mcp`. CodeFlow publishes RFC 9728 protected-resource metadata at `/.well-known/oauth-protected-resource`, returns a discoverable `WWW-Authenticate` challenge, and advertises per-tool OAuth security schemes. Access tokens are verified against the provider's JWKS with an HTTPS-only fetch, accepted signing algorithms, issuer, audience, expiry/not-before, and scopes. Signing keys are cached and refreshed safely for rotation.

The default scopes are `codeflow:read` and `codeflow:write`; override them with `CODEFLOW_MCP_OAUTH_READ_SCOPE` and `CODEFLOW_MCP_OAUTH_WRITE_SCOPE` only when the identity provider uses different names. A write token must contain both scopes. Fixed local tokens can remain enabled alongside OAuth for trusted local automation.

## Verification

```powershell
npm test
```

This runs Go persistence/revision/proposal tests and a strict TypeScript production build. MCP integration tests connect the official SDK client to the HTTP server, discover tools/resources, scan a temporary repository, generate and accept its chart proposal, verify an idempotent rescan, submit a manual review-gated proposal, and verify read/write token scopes.

The graph test suite also covers atomic multi-operation creation, dangling-reference cleanup, and guarded deletion of parent components.
