package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"

	"codeflow/internal/discovery"
)

type DiscoveryProposalRequest struct {
	Path          string `json:"path"`
	BaseRevision  int64  `json:"baseRevision"`
	ParentID      string `json:"parentId,omitempty"`
	Author        string `json:"author,omitempty"`
	MaxFiles      int    `json:"maxFiles,omitempty"`
	MaxComponents int    `json:"maxComponents,omitempty"`
}

type DiscoveryProposalResult struct {
	ProposalID        string           `json:"proposalId,omitempty"`
	Status            string           `json:"status"`
	Revision          int64            `json:"revision"`
	OperationCount    int              `json:"operationCount"`
	ComponentCount    int              `json:"componentCount"`
	RelationshipCount int              `json:"relationshipCount"`
	Report            discovery.Report `json:"report"`
	Project           Project          `json:"project"`
}

func discoverRepository(path string, maxFiles int) (discovery.Report, error) {
	if strings.TrimSpace(path) == "" {
		return discovery.Report{}, errors.New("path is required")
	}
	absPath, err := filepath.Abs(path)
	if err != nil {
		return discovery.Report{}, err
	}
	absPath, err = filepath.EvalSymlinks(absPath)
	if err != nil {
		return discovery.Report{}, err
	}
	allowed, err := discoveryRoots()
	if err != nil {
		return discovery.Report{}, err
	}
	for _, root := range allowed {
		rel, err := filepath.Rel(root, absPath)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return discovery.Scan(absPath, discovery.Options{MaxFiles: maxFiles})
		}
	}
	return discovery.Report{}, errors.New("path is outside CODEFLOW_DISCOVERY_ROOTS")
}

func proposeDiscoveredArchitecture(store *Store, req DiscoveryProposalRequest) (DiscoveryProposalResult, error) {
	report, err := discoverRepository(req.Path, req.MaxFiles)
	if err != nil {
		return DiscoveryProposalResult{}, err
	}
	project := store.Snapshot()
	if req.BaseRevision != project.Revision {
		return DiscoveryProposalResult{}, fmt.Errorf("%w: expected %d, current %d", ErrConflict, req.BaseRevision, project.Revision)
	}
	parentID := req.ParentID
	if parentID == "" {
		if componentExists(&project, "cmp_platform") {
			parentID = "cmp_platform"
		} else if len(project.Components) > 0 {
			parentID = project.Components[0].ID
		}
	}
	if parentID != "" && !componentExists(&project, parentID) {
		return DiscoveryProposalResult{}, fmt.Errorf("parent %s not found", parentID)
	}
	if req.MaxComponents <= 0 {
		req.MaxComponents = 50
	}
	if req.MaxComponents > 200 {
		req.MaxComponents = 200
	}

	selected := selectDiscoveryComponents(report.Components, req.MaxComponents)
	repositoryID := graphStableID("cmp_repo", normalizedRepositoryPath(report.Root))
	idMap := map[string]string{}
	for _, component := range selected {
		if component.Path == "." {
			idMap[component.ID] = repositoryID
		} else {
			idMap[component.ID] = graphStableID("cmp_src", normalizedRepositoryPath(report.Root)+"|"+component.Path)
		}
	}
	existingComponents := map[string]bool{}
	for _, component := range project.Components {
		existingComponents[component.ID] = true
	}
	operations := []Operation{}
	addedComponents := 0
	provenance := Provenance{Kind: "verified", Author: fallback(req.Author, "Repository discovery"), Confidence: .9}
	if !existingComponents[repositoryID] {
		operations = append(operations, Operation{Kind: "add_component", Component: &Component{
			ID: repositoryID, Type: "system", Title: report.Name, ParentID: parentID,
			Description: fmt.Sprintf("Repository discovered from %s with %d source files.", report.Root, report.SourceFiles),
			Properties:  map[string]string{"repository": report.Root, "kind": report.Kind, "frameworks": strings.Join(report.Frameworks, ", "), "sourceFiles": strconv.Itoa(report.SourceFiles)}, Provenance: provenance,
		}})
		addedComponents++
		existingComponents[repositoryID] = true
	}
	for _, source := range selected {
		if source.Path == "." {
			continue
		}
		id := idMap[source.ID]
		if id == "" || existingComponents[id] {
			continue
		}
		componentParent := repositoryID
		for ancestor := filepath.ToSlash(filepath.Dir(filepath.FromSlash(source.Path))); ancestor != "." && ancestor != ""; ancestor = filepath.ToSlash(filepath.Dir(filepath.FromSlash(ancestor))) {
			if candidate := discoveryIDForPath(selected, ancestor); candidate != "" && idMap[candidate] != "" {
				componentParent = idMap[candidate]
				break
			}
		}
		operations = append(operations, Operation{Kind: "add_component", Component: &Component{
			ID: id, Type: graphComponentType(source.Kind), Title: source.Name, ParentID: componentParent,
			Description: fmt.Sprintf("%s package discovered at %s.", source.Language, source.Path),
			Properties:  map[string]string{"repository": report.Root, "path": source.Path, "language": source.Language, "files": strconv.Itoa(source.FileCount), "symbols": strconv.Itoa(source.SymbolCount), "sampleSymbols": strings.Join(source.Symbols, ", ")}, Provenance: provenance,
		}})
		addedComponents++
		existingComponents[id] = true
	}
	existingRelationships := map[string]bool{}
	for _, relationship := range project.Relationships {
		existingRelationships[relationship.SourceID+"\x00"+relationship.TargetID+"\x00"+relationship.Kind] = true
	}
	addedRelationships := 0
	for _, dependency := range report.Dependencies {
		sourceID, targetID := idMap[dependency.SourceID], idMap[dependency.TargetID]
		if sourceID == "" || targetID == "" || sourceID == targetID || !existingComponents[sourceID] || !existingComponents[targetID] {
			continue
		}
		key := sourceID + "\x00" + targetID + "\x00" + dependency.Kind
		if existingRelationships[key] {
			continue
		}
		operations = append(operations, Operation{Kind: "connect", Relationship: &Relationship{
			ID: graphStableID("rel_src", key), SourceID: sourceID, TargetID: targetID, Kind: dependency.Kind,
			Label: dependencyLabel(dependency), Provenance: Provenance{Kind: "verified", Author: fallback(req.Author, "Repository discovery"), Confidence: .95},
		}})
		addedRelationships++
		existingRelationships[key] = true
	}
	result := DiscoveryProposalResult{Status: "up_to_date", Revision: project.Revision, OperationCount: len(operations), ComponentCount: addedComponents, RelationshipCount: addedRelationships, Report: report, Project: project}
	if len(operations) == 0 {
		return result, nil
	}
	proposalID := graphStableID("prop_discovery", normalizedRepositoryPath(report.Root)+"|"+strconv.FormatInt(project.Revision, 10))
	if existing := slices.IndexFunc(project.Proposals, func(proposal Proposal) bool { return proposal.ID == proposalID && proposal.Status == "pending" }); existing >= 0 {
		result.ProposalID = proposalID
		result.Status = "pending"
		return result, nil
	}
	title := "Import discovered architecture for " + report.Name
	summary := fmt.Sprintf("Add %d repository components and %d evidence-backed dependency relationships from a read-only scan of %s.", addedComponents, addedRelationships, report.Root)
	updated, err := store.SaveProposal(Proposal{ID: proposalID, Title: title, Summary: summary, Author: fallback(req.Author, "Repository discovery"), AuthorType: "agent", BaseRevision: req.BaseRevision, Operations: operations})
	if err != nil {
		return DiscoveryProposalResult{}, err
	}
	result.ProposalID, result.Status, result.Project = proposalID, "pending", updated
	return result, nil
}

func selectDiscoveryComponents(components []discovery.Component, limit int) []discovery.Component {
	if len(components) <= limit {
		return slices.Clone(components)
	}
	ranked := slices.Clone(components)
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].FileCount == ranked[j].FileCount {
			return ranked[i].Path < ranked[j].Path
		}
		return ranked[i].FileCount > ranked[j].FileCount
	})
	ranked = ranked[:limit]
	sort.Slice(ranked, func(i, j int) bool { return ranked[i].Path < ranked[j].Path })
	return ranked
}

func discoveryIDForPath(components []discovery.Component, path string) string {
	for _, component := range components {
		if component.Path == path {
			return component.ID
		}
	}
	return ""
}

func graphStableID(prefix, value string) string {
	sum := sha256.Sum256([]byte(value))
	return prefix + "_" + hex.EncodeToString(sum[:6])
}

func normalizedRepositoryPath(path string) string {
	return strings.ToLower(filepath.ToSlash(filepath.Clean(path)))
}

func graphComponentType(kind string) string {
	switch kind {
	case "api":
		return "api"
	case "client":
		return "client"
	case "database":
		return "database"
	case "service", "application":
		return "service"
	default:
		return "component"
	}
}

func dependencyLabel(dependency discovery.Dependency) string {
	label := dependency.Kind
	if dependency.Count > 1 {
		label += " (" + strconv.Itoa(dependency.Count) + ")"
	}
	return label
}

func discoveryRoots() ([]string, error) {
	configured := strings.TrimSpace(os.Getenv("CODEFLOW_DISCOVERY_ROOTS"))
	if configured == "" {
		configured = "."
	}
	parts := filepath.SplitList(configured)
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			continue
		}
		root, err := filepath.Abs(part)
		if err != nil {
			return nil, err
		}
		root, err = filepath.EvalSymlinks(root)
		if err != nil {
			return nil, err
		}
		result = append(result, filepath.Clean(root))
	}
	return result, nil
}
