package workspace

import "time"

func seedProject() Project {
	now := time.Now().UTC()
	human := Provenance{Kind: "human", Author: "Bryce Sanders", Confidence: 1}
	agent := Provenance{Kind: "inferred", Author: "Codex", Confidence: .82}
	return Project{
		ID: "project_codeflow", Name: "Commerce Platform", Revision: 12, UpdatedAt: now,
		Summary: "Shared architecture model for the customer-facing commerce platform.",
		Components: []Component{
			{ID: "cmp_platform", Type: "system", Title: "Commerce Platform", Description: "Customer experience, APIs, and durable data.", Provenance: human},
			{ID: "cmp_web", Type: "client", Title: "Web application", Description: "React storefront and account experience.", ParentID: "cmp_platform", Properties: map[string]string{"runtime": "React + TypeScript"}, Provenance: human},
			{ID: "cmp_backend", Type: "service", Title: "Backend API", Description: "Public HTTP API and application orchestration.", ParentID: "cmp_platform", Properties: map[string]string{"runtime": "Go", "port": "8080"}, Provenance: human},
			{ID: "cmp_auth", Type: "service", Title: "Authentication", Description: "Issues sessions and protects account resources.", ParentID: "cmp_backend", Properties: map[string]string{"protocol": "HTTP + secure cookie"}, Provenance: human},
			{ID: "cmp_sessions", Type: "database", Title: "Session store", Description: "Durable user sessions with automatic expiry.", ParentID: "cmp_backend", Properties: map[string]string{"engine": "PostgreSQL", "retention": "30 days"}, Provenance: human},
		},
		Blocks: []Block{
			{ID: "blk_req_auth", OwnerComponentID: "cmp_backend", Type: "requirement", Title: "Secure account access", Content: "Authenticated endpoints require a valid, revocable session.", ChildIDs: []string{"blk_behavior_expiry"}},
			{ID: "blk_behavior_expiry", OwnerComponentID: "cmp_backend", ParentID: "blk_req_auth", Type: "behavior", Title: "Expire idle sessions", Content: "Sessions expire after 30 days of inactivity."},
			{ID: "blk_api_login", OwnerComponentID: "cmp_backend", Type: "api", Title: "POST /v1/sessions", Content: "Create a new authenticated session.", Properties: map[string]string{"method": "POST", "path": "/v1/sessions", "request": "{ email, password }", "response": "201 Session"}},
			{ID: "blk_table_sessions", OwnerComponentID: "cmp_sessions", Type: "table", Title: "sessions", Content: "Server-side session records.", Properties: map[string]string{"columns": "id uuid PK\nuser_id uuid FK\ntoken_hash text\nexpires_at timestamptz", "indexes": "user_id, expires_at"}},
			{ID: "blk_code_handler", OwnerComponentID: "cmp_auth", Type: "code", Title: "Handler sketch", Content: "func CreateSession(ctx context.Context, input Login) (Session, error)"},
		},
		Relationships: []Relationship{
			{ID: "rel_web_backend", SourceID: "cmp_web", TargetID: "cmp_backend", Kind: "calls", Label: "HTTPS / JSON", Provenance: human},
			{ID: "rel_backend_auth", SourceID: "cmp_backend", TargetID: "cmp_auth", Kind: "delegates", Label: "session flow", Provenance: human},
			{ID: "rel_auth_store", SourceID: "cmp_auth", TargetID: "cmp_sessions", Kind: "reads_writes", Label: "SQL", Provenance: human},
		},
		Views: []View{{ID: "view_system", Name: "System overview", ScopeID: "cmp_platform", Positions: map[string]Position{
			"cmp_web": {X: 80, Y: 170}, "cmp_backend": {X: 410, Y: 170}, "cmp_auth": {X: 740, Y: 70}, "cmp_sessions": {X: 740, Y: 290},
		}}},
		Proposals: []Proposal{{
			ID: "prop_rate_limit", Title: "Protect session creation", Summary: "Add a rate limiter in front of the session endpoint to reduce credential-stuffing risk.",
			Author: "Atlas · Architecture agent", AuthorType: "agent", BaseRevision: 12, Status: "pending", CreatedAt: now.Add(-18 * time.Minute),
			Operations: []Operation{
				{Kind: "add_component", Component: &Component{ID: "cmp_rate_limiter", Type: "service", Title: "Rate limiter", Description: "Per-IP and per-account request budgets.", ParentID: "cmp_backend", Properties: map[string]string{"algorithm": "sliding window", "budget": "5 / minute"}, Provenance: agent}},
				{Kind: "connect", Relationship: &Relationship{ID: "rel_backend_limit", SourceID: "cmp_backend", TargetID: "cmp_rate_limiter", Kind: "calls", Label: "before authentication", Provenance: agent}},
				{Kind: "add_block", Block: &Block{ID: "blk_limit_req", OwnerComponentID: "cmp_backend", Type: "constraint", Title: "Throttle login attempts", Content: "Allow at most 5 failed attempts per account and IP each minute."}},
			},
		}},
		History: []Event{{ID: "evt_seed", Revision: 12, Kind: "project.created", Summary: "Imported initial commerce architecture", Author: "Bryce Sanders", CreatedAt: now.Add(-2 * time.Hour)}},
	}
}
