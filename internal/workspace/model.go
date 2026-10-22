package workspace

import "time"

type Project struct {
	ID            string         `json:"id"`
	Name          string         `json:"name"`
	Summary       string         `json:"summary"`
	Revision      int64          `json:"revision"`
	UpdatedAt     time.Time      `json:"updatedAt"`
	Components    []Component    `json:"components"`
	Blocks        []Block        `json:"blocks"`
	Relationships []Relationship `json:"relationships"`
	Views         []View         `json:"views"`
	Proposals     []Proposal     `json:"proposals"`
	History       []Event        `json:"history"`
}

type Component struct {
	ID          string            `json:"id"`
	Type        string            `json:"type"`
	Title       string            `json:"title"`
	Description string            `json:"description"`
	ParentID    string            `json:"parentId,omitempty"`
	Properties  map[string]string `json:"properties,omitempty"`
	Provenance  Provenance        `json:"provenance"`
}

type Block struct {
	ID               string            `json:"id"`
	OwnerComponentID string            `json:"ownerComponentId"`
	ParentID         string            `json:"parentId,omitempty"`
	Type             string            `json:"type"`
	Title            string            `json:"title"`
	Content          string            `json:"content"`
	Properties       map[string]string `json:"properties,omitempty"`
	ChildIDs         []string          `json:"childIds,omitempty"`
}

type Relationship struct {
	ID         string     `json:"id"`
	SourceID   string     `json:"sourceId"`
	TargetID   string     `json:"targetId"`
	Kind       string     `json:"kind"`
	Label      string     `json:"label"`
	Provenance Provenance `json:"provenance"`
}

type Provenance struct {
	Kind       string  `json:"kind"`
	Author     string  `json:"author"`
	Confidence float64 `json:"confidence"`
}

type View struct {
	ID        string              `json:"id"`
	Name      string              `json:"name"`
	ScopeID   string              `json:"scopeId"`
	Positions map[string]Position `json:"positions"`
}

type Position struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Proposal struct {
	ID           string      `json:"id"`
	Title        string      `json:"title"`
	Summary      string      `json:"summary"`
	Author       string      `json:"author"`
	AuthorType   string      `json:"authorType"`
	BaseRevision int64       `json:"baseRevision"`
	Status       string      `json:"status"`
	CreatedAt    time.Time   `json:"createdAt"`
	ReviewedAt   *time.Time  `json:"reviewedAt,omitempty"`
	Reviewer     string      `json:"reviewer,omitempty"`
	Comment      string      `json:"comment,omitempty"`
	Operations   []Operation `json:"operations"`
}

type Operation struct {
	Kind         string        `json:"kind"`
	Component    *Component    `json:"component,omitempty"`
	Block        *Block        `json:"block,omitempty"`
	Relationship *Relationship `json:"relationship,omitempty"`
	TargetID     string        `json:"targetId,omitempty"`
	Title        string        `json:"title,omitempty"`
	Description  string        `json:"description,omitempty"`
}

type Event struct {
	ID        string    `json:"id"`
	Revision  int64     `json:"revision"`
	Kind      string    `json:"kind"`
	Summary   string    `json:"summary"`
	Author    string    `json:"author"`
	CreatedAt time.Time `json:"createdAt"`
}

type MutationRequest struct {
	BaseRevision int64       `json:"baseRevision"`
	Author       string      `json:"author"`
	Operations   []Operation `json:"operations"`
}

type ReviewRequest struct {
	Reviewer string `json:"reviewer"`
	Decision string `json:"decision"`
	Comment  string `json:"comment"`
}

type Presence struct {
	SessionID string    `json:"sessionId"`
	Name      string    `json:"name"`
	Color     string    `json:"color"`
	Selection string    `json:"selection,omitempty"`
	LastSeen  time.Time `json:"lastSeen"`
}
