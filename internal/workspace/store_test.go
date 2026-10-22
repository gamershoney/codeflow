package workspace

import (
	"errors"
	"path/filepath"
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
