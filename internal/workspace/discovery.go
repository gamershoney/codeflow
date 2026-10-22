package workspace

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"codeflow/internal/discovery"
)

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
