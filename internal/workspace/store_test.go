package workspace

import (
	"errors"
	"path/filepath"
	"slices"
	"testing"
)

func TestMutationIsRevisionedAndPersistent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "project.json")
	store, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	before := store.Snapshot()
	after, err := store.Mutate(MutationRequest{BaseRevision: before.Revision, Author: "test", Operations: []Operation{{Kind: "update_component", TargetID: "cmp_backend", Title: "Core API"}}})
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision+1 {
		t.Fatalf("revision = %d", after.Revision)
	}
	reloaded, err := NewStore(path)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Snapshot().Revision != after.Revision {
		t.Fatal("revision did not persist")
	}
	if _, err := reloaded.Mutate(MutationRequest{BaseRevision: before.Revision}); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected conflict, got %v", err)
	}
}

func TestProposalDoesNotMutateUntilAccepted(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	before := store.Snapshot()
	if componentExists(&before, "cmp_rate_limiter") {
		t.Fatal("proposal leaked into authoritative graph")
	}
	after, err := store.Review("prop_rate_limit", ReviewRequest{Reviewer: "test", Decision: "accepted"})
	if err != nil {
		t.Fatal(err)
	}
	if !componentExists(&after, "cmp_rate_limiter") {
		t.Fatal("accepted proposal was not applied")
	}
	if after.Revision != before.Revision+1 {
		t.Fatal("acceptance must advance revision")
	}
}

func TestRejectLeavesGraphUntouched(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	before := store.Snapshot()
	after, err := store.Review("prop_rate_limit", ReviewRequest{Reviewer: "test", Decision: "rejected"})
	if err != nil {
		t.Fatal(err)
	}
	if after.Revision != before.Revision || len(after.Components) != len(before.Components) {
		t.Fatal("rejection mutated graph")
	}
}

func TestManualGraphEditingValidatesAndCleansReferences(t *testing.T) {
	store, err := NewStore(filepath.Join(t.TempDir(), "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	before := store.Snapshot()
	component := Component{ID: "cmp_worker", Type: "service", Title: "Worker", ParentID: "cmp_platform", Provenance: Provenance{Kind: "human", Author: "test", Confidence: 1}}
	block := Block{ID: "blk_job", OwnerComponentID: component.ID, Type: "behavior", Title: "Process job", Content: "Process one queued job."}
	relation := Relationship{ID: "rel_api_worker", SourceID: "cmp_backend", TargetID: component.ID, Kind: "publishes", Label: "jobs", Provenance: component.Provenance}
	after, err := store.Mutate(MutationRequest{BaseRevision: before.Revision, Author: "test", Operations: []Operation{{Kind: "add_component", Component: &component}, {Kind: "add_block", Block: &block}, {Kind: "connect", Relationship: &relation}}})
	if err != nil {
		t.Fatal(err)
	}
	if !componentExists(&after, component.ID) || len(after.Blocks) != len(before.Blocks)+1 || len(after.Relationships) != len(before.Relationships)+1 {
		t.Fatal("atomic create batch was not applied")
	}
	after, err = store.Mutate(MutationRequest{BaseRevision: after.Revision, Author: "test", Operations: []Operation{{Kind: "delete_component", TargetID: component.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	if componentExists(&after, component.ID) || slices.ContainsFunc(after.Blocks, func(b Block) bool { return b.OwnerComponentID == component.ID }) || slices.ContainsFunc(after.Relationships, func(r Relationship) bool { return r.TargetID == component.ID }) {
		t.Fatal("component deletion left dangling references")
	}
}

func TestCannotDeleteComponentWithChildren(t *testing.T) {
	store, _ := NewStore(filepath.Join(t.TempDir(), "project.json"))
	p := store.Snapshot()
	_, err := store.Mutate(MutationRequest{BaseRevision: p.Revision, Author: "test", Operations: []Operation{{Kind: "delete_component", TargetID: "cmp_backend"}}})
	if err == nil {
		t.Fatal("expected child validation error")
	}
}
