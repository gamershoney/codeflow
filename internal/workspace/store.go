package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"time"
)

var ErrConflict = errors.New("revision conflict")

type Store struct {
	mu       sync.RWMutex
	path     string
	project  Project
	subs     map[chan []byte]struct{}
	presence map[string]Presence
}

func NewStore(path string) (*Store, error) {
	s := &Store{path: path, subs: map[chan []byte]struct{}{}, presence: map[string]Presence{}}
	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &s.project); err != nil {
			return nil, fmt.Errorf("decode project: %w", err)
		}
		return s, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	s.project = seedProject()
	if err := s.persistLocked(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Store) Snapshot() Project {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clone(s.project)
}

func (s *Store) Mutate(req MutationRequest) (Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if req.BaseRevision != s.project.Revision {
		return Project{}, fmt.Errorf("%w: expected %d, current %d", ErrConflict, req.BaseRevision, s.project.Revision)
	}
	next := clone(s.project)
	if err := applyOperations(&next, req.Operations); err != nil {
		return Project{}, err
	}
	next.Revision++
	next.UpdatedAt = time.Now().UTC()
	next.History = append([]Event{{ID: fmt.Sprintf("evt_%d", next.Revision), Revision: next.Revision, Kind: "graph.updated", Summary: summarize(req.Operations), Author: fallback(req.Author, "Anonymous"), CreatedAt: next.UpdatedAt}}, next.History...)
	s.project = next
	if err := s.persistLocked(); err != nil {
		return Project{}, err
	}
	s.broadcastLocked("project.updated")
	return clone(s.project), nil
}

func (s *Store) Review(id string, req ReviewRequest) (Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := clone(s.project)
	idx := slices.IndexFunc(next.Proposals, func(p Proposal) bool { return p.ID == id })
	if idx < 0 {
		return Project{}, errors.New("proposal not found")
	}
	p := &next.Proposals[idx]
	if p.Status != "pending" {
		return Project{}, errors.New("proposal has already been reviewed")
	}
	if req.Decision != "accepted" && req.Decision != "rejected" {
		return Project{}, errors.New("decision must be accepted or rejected")
	}
	if req.Decision == "accepted" {
		if p.BaseRevision != next.Revision {
			return Project{}, fmt.Errorf("%w: proposal targets %d, current %d", ErrConflict, p.BaseRevision, next.Revision)
		}
		if err := applyOperations(&next, p.Operations); err != nil {
			return Project{}, err
		}
		next.Revision++
		next.UpdatedAt = time.Now().UTC()
		next.History = append([]Event{{ID: fmt.Sprintf("evt_%d", next.Revision), Revision: next.Revision, Kind: "proposal.accepted", Summary: p.Title, Author: fallback(req.Reviewer, "Reviewer"), CreatedAt: next.UpdatedAt}}, next.History...)
	}
	now := time.Now().UTC()
	p.Status, p.Reviewer, p.Comment, p.ReviewedAt = req.Decision, fallback(req.Reviewer, "Reviewer"), req.Comment, &now
	s.project = next
	if err := s.persistLocked(); err != nil {
		return Project{}, err
	}
	s.broadcastLocked("proposal.reviewed")
	return clone(s.project), nil
}

func (s *Store) SaveProposal(p Proposal) (Project, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.BaseRevision != s.project.Revision {
		return Project{}, ErrConflict
	}
	if p.ID == "" || p.Title == "" || len(p.Operations) == 0 {
		return Project{}, errors.New("id, title, and operations are required")
	}
	test := clone(s.project)
	if err := applyOperations(&test, p.Operations); err != nil {
		return Project{}, err
	}
	p.Status = "pending"
	p.CreatedAt = time.Now().UTC()
	s.project.Proposals = append([]Proposal{p}, s.project.Proposals...)
	if err := s.persistLocked(); err != nil {
		return Project{}, err
	}
	s.broadcastLocked("proposal.created")
	return clone(s.project), nil
}

func applyOperations(p *Project, ops []Operation) error {
	for _, op := range ops {
		switch op.Kind {
		case "add_component":
			if op.Component == nil || op.Component.ID == "" || op.Component.Title == "" {
				return errors.New("invalid component")
			}
			if componentExists(p, op.Component.ID) {
				return fmt.Errorf("component %s already exists", op.Component.ID)
			}
			if op.Component.ParentID != "" && !componentExists(p, op.Component.ParentID) {
				return fmt.Errorf("parent %s not found", op.Component.ParentID)
			}
			p.Components = append(p.Components, *op.Component)
			if len(p.Views) > 0 {
				p.Views[0].Positions[op.Component.ID] = Position{X: 520 + float64(len(p.Components)%3)*70, Y: 380}
			}
		case "delete_component":
			if !componentExists(p, op.TargetID) {
				return fmt.Errorf("component %s not found", op.TargetID)
			}
			if slices.ContainsFunc(p.Components, func(c Component) bool { return c.ParentID == op.TargetID }) {
				return errors.New("move or delete child components first")
			}
			p.Components = slices.DeleteFunc(p.Components, func(c Component) bool { return c.ID == op.TargetID })
			p.Blocks = slices.DeleteFunc(p.Blocks, func(b Block) bool { return b.OwnerComponentID == op.TargetID })
			p.Relationships = slices.DeleteFunc(p.Relationships, func(r Relationship) bool { return r.SourceID == op.TargetID || r.TargetID == op.TargetID })
			for i := range p.Views {
				delete(p.Views[i].Positions, op.TargetID)
			}
		case "update_component":
			idx := slices.IndexFunc(p.Components, func(c Component) bool { return c.ID == op.TargetID })
			if idx < 0 {
				return fmt.Errorf("component %s not found", op.TargetID)
			}
			if op.Title != "" {
				p.Components[idx].Title = op.Title
			}
			if op.Description != "" {
				p.Components[idx].Description = op.Description
			}
			if op.Component != nil && op.Component.Properties != nil {
				p.Components[idx].Properties = op.Component.Properties
			}
		case "add_block":
			if op.Block == nil || op.Block.ID == "" || !componentExists(p, op.Block.OwnerComponentID) {
				return errors.New("invalid block or owner")
			}
			if slices.ContainsFunc(p.Blocks, func(b Block) bool { return b.ID == op.Block.ID }) {
				return fmt.Errorf("block %s already exists", op.Block.ID)
			}
			p.Blocks = append(p.Blocks, *op.Block)
		case "update_block":
			if op.Block == nil || op.Block.ID == "" || !componentExists(p, op.Block.OwnerComponentID) {
				return errors.New("invalid block or owner")
			}
			idx := slices.IndexFunc(p.Blocks, func(b Block) bool { return b.ID == op.Block.ID })
			if idx < 0 {
				return fmt.Errorf("block %s not found", op.Block.ID)
			}
			p.Blocks[idx] = *op.Block
		case "delete_block":
			if !slices.ContainsFunc(p.Blocks, func(b Block) bool { return b.ID == op.TargetID }) {
				return fmt.Errorf("block %s not found", op.TargetID)
			}
			p.Blocks = slices.DeleteFunc(p.Blocks, func(b Block) bool { return b.ID == op.TargetID || b.ParentID == op.TargetID })
		case "connect":
			if op.Relationship == nil || !componentExists(p, op.Relationship.SourceID) || !componentExists(p, op.Relationship.TargetID) {
				return errors.New("relationship has a dangling endpoint")
			}
			if op.Relationship.SourceID == op.Relationship.TargetID {
				return errors.New("a relationship cannot connect a component to itself")
			}
			if slices.ContainsFunc(p.Relationships, func(r Relationship) bool { return r.ID == op.Relationship.ID }) {
				return fmt.Errorf("relationship %s already exists", op.Relationship.ID)
			}
			p.Relationships = append(p.Relationships, *op.Relationship)
		case "disconnect":
			if !slices.ContainsFunc(p.Relationships, func(r Relationship) bool { return r.ID == op.TargetID }) {
				return fmt.Errorf("relationship %s not found", op.TargetID)
			}
			p.Relationships = slices.DeleteFunc(p.Relationships, func(r Relationship) bool { return r.ID == op.TargetID })
		case "move":
			if !componentExists(p, op.TargetID) || op.Component == nil {
				return errors.New("invalid move")
			}
			if len(p.Views) > 0 {
				p.Views[0].Positions[op.TargetID] = Position{X: parseFloat(op.Component.Properties["x"]), Y: parseFloat(op.Component.Properties["y"])}
			}
		default:
			return fmt.Errorf("unsupported operation %q", op.Kind)
		}
	}
	return nil
}

func (s *Store) Presence(p Presence) []Presence {
	s.mu.Lock()
	defer s.mu.Unlock()
	p.LastSeen = time.Now().UTC()
	s.presence[p.SessionID] = p
	cutoff := time.Now().Add(-45 * time.Second)
	for id, item := range s.presence {
		if item.LastSeen.Before(cutoff) {
			delete(s.presence, id)
		}
	}
	result := make([]Presence, 0, len(s.presence))
	for _, item := range s.presence {
		result = append(result, item)
	}
	s.broadcastPresenceLocked(result)
	return result
}

func (s *Store) Subscribe() chan []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	ch := make(chan []byte, 8)
	s.subs[ch] = struct{}{}
	return ch
}
func (s *Store) Unsubscribe(ch chan []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.subs, ch)
	close(ch)
}

func (s *Store) broadcastLocked(kind string) {
	payload, _ := json.Marshal(map[string]any{"type": kind, "revision": s.project.Revision})
	for ch := range s.subs {
		select {
		case ch <- payload:
		default:
		}
	}
}
func (s *Store) broadcastPresenceLocked(p []Presence) {
	payload, _ := json.Marshal(map[string]any{"type": "presence.updated", "presence": p})
	for ch := range s.subs {
		select {
		case ch <- payload:
		default:
		}
	}
}

func (s *Store) persistLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.project, "", "  ")
	if err != nil {
		return err
	}
	// Write directly for Windows compatibility: os.Rename cannot reliably replace
	// an existing file when it is being observed by another process.
	return os.WriteFile(s.path, data, 0o644)
}

func clone(p Project) Project {
	data, _ := json.Marshal(p)
	var out Project
	_ = json.Unmarshal(data, &out)
	return out
}
func componentExists(p *Project, id string) bool {
	return slices.ContainsFunc(p.Components, func(c Component) bool { return c.ID == id })
}
func fallback(v, d string) string {
	if v == "" {
		return d
	}
	return v
}
func summarize(ops []Operation) string {
	if len(ops) == 1 {
		return "Applied 1 architecture operation"
	}
	return fmt.Sprintf("Applied %d architecture operations", len(ops))
}
func parseFloat(v string) float64 { var f float64; _, _ = fmt.Sscanf(v, "%f", &f); return f }
