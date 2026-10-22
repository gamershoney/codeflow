package discovery

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
)

type Options struct {
	MaxFiles int `json:"maxFiles,omitempty"`
}

type Report struct {
	Root         string         `json:"root"`
	Name         string         `json:"name"`
	Kind         string         `json:"kind"`
	GeneratedAt  time.Time      `json:"generatedAt"`
	TotalFiles   int            `json:"totalFiles"`
	SourceFiles  int            `json:"sourceFiles"`
	Languages    map[string]int `json:"languages"`
	Frameworks   []string       `json:"frameworks"`
	Manifests    []string       `json:"manifests"`
	Components   []Component    `json:"components"`
	Dependencies []Dependency   `json:"dependencies"`
	Warnings     []string       `json:"warnings,omitempty"`
	Truncated    bool           `json:"truncated"`
}

type Component struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Path        string   `json:"path"`
	Kind        string   `json:"kind"`
	Language    string   `json:"language"`
	FileCount   int      `json:"fileCount"`
	SymbolCount int      `json:"symbolCount"`
	Symbols     []string `json:"symbols,omitempty"`
}

type Dependency struct {
	SourceID string   `json:"sourceId"`
	TargetID string   `json:"targetId"`
	Kind     string   `json:"kind"`
	Count    int      `json:"count"`
	Evidence []string `json:"evidence"`
}

type sourceFile struct {
	abs, rel, dir, language string
}

type componentBuild struct {
	component Component
	languages map[string]int
}

var (
	quotedImport = regexp.MustCompile(`(?m)(?:from\s+|require\s*\(|import\s*)["']([^"']+)["']`)
	includePath  = regexp.MustCompile(`(?m)^\s*#include\s*["<]([^">]+)[">]`)
	goImport     = regexp.MustCompile(`(?m)(?:^\s*import\s+|^\s*)"([^"]+)"`)
	godotPath    = regexp.MustCompile(`path="res://([^"]+)"`)
	symbolLine   = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:async\s+)?(?:func|function|class|struct|interface|enum|type)\s+([A-Za-z_][A-Za-z0-9_]*)`)
)

var sourceLanguages = map[string]string{
	".go": "Go", ".js": "JavaScript", ".mjs": "JavaScript", ".cjs": "JavaScript", ".jsx": "JavaScript",
	".ts": "TypeScript", ".tsx": "TypeScript", ".cpp": "C++", ".cc": "C++", ".c": "C", ".h": "C/C++ Header", ".hpp": "C/C++ Header",
	".cs": "C#", ".gd": "GDScript", ".py": "Python", ".java": "Java", ".kt": "Kotlin", ".rs": "Rust", ".rb": "Ruby", ".php": "PHP",
}

var skippedDirectories = map[string]bool{
	".git": true, ".idea": true, ".vscode": true, "node_modules": true, "vendor": true, "dist": true, "build": true,
	"bin": true, "obj": true, "binaries": true, "intermediate": true, "saved": true, "deriveddatacache": true, ".godot": true,
}

func Scan(root string, options Options) (Report, error) {
	if options.MaxFiles <= 0 {
		options.MaxFiles = 25000
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return Report{}, err
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return Report{}, err
	}
	if !info.IsDir() {
		return Report{}, errors.New("repository root must be a directory")
	}

	report := Report{Root: absRoot, Name: filepath.Base(absRoot), Kind: "software", GeneratedAt: time.Now().UTC(), Languages: map[string]int{}}
	components := map[string]*componentBuild{}
	files := []sourceFile{}
	fileByBase := map[string]string{}
	goModule := ""

	err = filepath.WalkDir(absRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			report.Warnings = append(report.Warnings, fmt.Sprintf("skip %s: %v", path, walkErr))
			return nil
		}
		if entry.IsDir() {
			if path != absRoot && (skippedDirectories[strings.ToLower(entry.Name())] || strings.HasPrefix(entry.Name(), ".cache")) {
				return filepath.SkipDir
			}
			return nil
		}
		report.TotalFiles++
		if report.TotalFiles > options.MaxFiles {
			report.Truncated = true
			return fs.SkipAll
		}
		rel, err := filepath.Rel(absRoot, path)
		if err != nil {
			return nil
		}
		rel = cleanPath(rel)
		base := strings.ToLower(entry.Name())
		if framework, ok := manifestFramework(base); ok {
			report.Manifests = append(report.Manifests, rel)
			addUnique(&report.Frameworks, framework)
			if base == "go.mod" {
				goModule = readGoModule(path)
			}
		}
		if strings.HasSuffix(base, ".uproject") || strings.HasSuffix(base, ".uplugin") {
			addUnique(&report.Frameworks, "Unreal Engine")
			report.Kind = "game"
		}
		if base == "project.godot" {
			addUnique(&report.Frameworks, "Godot")
			report.Kind = "game"
		}
		if ext := strings.ToLower(filepath.Ext(entry.Name())); ext == ".unity" || ext == ".asmdef" {
			addUnique(&report.Frameworks, "Unity")
			report.Kind = "game"
		}
		language, source := sourceLanguages[strings.ToLower(filepath.Ext(entry.Name()))]
		if !source && strings.ToLower(filepath.Ext(entry.Name())) != ".tscn" {
			return nil
		}
		if !source {
			language = "Godot Scene"
		}
		report.SourceFiles++
		report.Languages[language]++
		dir := cleanPath(filepath.Dir(rel))
		build := components[dir]
		if build == nil {
			build = &componentBuild{component: Component{ID: stableID("component", dir), Name: componentName(dir, report.Name), Path: dir, Kind: componentKind(dir), Language: language}, languages: map[string]int{}}
			components[dir] = build
		}
		build.component.FileCount++
		build.languages[language]++
		fileByBase[strings.ToLower(filepath.Base(rel))] = dir
		files = append(files, sourceFile{abs: path, rel: rel, dir: dir, language: language})
		return nil
	})
	if err != nil {
		return Report{}, err
	}

	for _, file := range files {
		data, err := readSmall(file.abs)
		if err != nil {
			continue
		}
		build := components[file.dir]
		for _, match := range symbolLine.FindAllStringSubmatch(string(data), 30) {
			build.component.SymbolCount++
			if len(build.component.Symbols) < 12 {
				build.component.Symbols = append(build.component.Symbols, match[1])
			}
		}
	}
	for _, build := range components {
		build.component.Language = dominant(build.languages)
		report.Components = append(report.Components, build.component)
	}
	sort.Slice(report.Components, func(i, j int) bool { return report.Components[i].Path < report.Components[j].Path })
	report.Dependencies = discoverDependencies(files, components, fileByBase, goModule)
	sort.Strings(report.Frameworks)
	sort.Strings(report.Manifests)
	if slices.Contains(report.Frameworks, "Unreal Engine") {
		report.Warnings = append(report.Warnings, "Unreal binary assets are inventoried by file count only; Blueprint and asset-reference discovery requires Asset Registry export.")
	}
	return report, nil
}

func discoverDependencies(files []sourceFile, components map[string]*componentBuild, fileByBase map[string]string, goModule string) []Dependency {
	type edgeBuild struct {
		source, target, kind string
		evidence             []string
		count                int
	}
	edges := map[string]*edgeBuild{}
	for _, file := range files {
		data, err := readSmall(file.abs)
		if err != nil {
			continue
		}
		text := string(data)
		imports := []string{}
		switch file.language {
		case "Go":
			for _, match := range goImport.FindAllStringSubmatch(text, -1) {
				imports = append(imports, match[1])
			}
		case "JavaScript", "TypeScript":
			for _, match := range quotedImport.FindAllStringSubmatch(text, -1) {
				imports = append(imports, match[1])
			}
		case "C", "C++", "C/C++ Header":
			for _, match := range includePath.FindAllStringSubmatch(text, -1) {
				imports = append(imports, match[1])
			}
		case "Godot Scene", "GDScript":
			for _, match := range godotPath.FindAllStringSubmatch(text, -1) {
				imports = append(imports, match[1])
			}
		}
		for _, imported := range imports {
			target := resolveImport(file, imported, components, fileByBase, goModule)
			if target == "" || target == file.dir {
				continue
			}
			key := file.dir + "\x00" + target
			edge := edges[key]
			if edge == nil {
				edge = &edgeBuild{source: file.dir, target: target, kind: "imports"}
				edges[key] = edge
			}
			edge.count++
			if len(edge.evidence) < 5 {
				edge.evidence = append(edge.evidence, file.rel+" → "+imported)
			}
		}
	}
	result := make([]Dependency, 0, len(edges))
	for _, edge := range edges {
		result = append(result, Dependency{SourceID: components[edge.source].component.ID, TargetID: components[edge.target].component.ID, Kind: edge.kind, Count: edge.count, Evidence: edge.evidence})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].SourceID == result[j].SourceID {
			return result[i].TargetID < result[j].TargetID
		}
		return result[i].SourceID < result[j].SourceID
	})
	return result
}

func resolveImport(file sourceFile, imported string, components map[string]*componentBuild, fileByBase map[string]string, goModule string) string {
	imported = strings.TrimSpace(strings.ReplaceAll(imported, "\\", "/"))
	if file.language == "Go" && goModule != "" && strings.HasPrefix(imported, goModule) {
		return nearestComponent(strings.TrimPrefix(strings.TrimPrefix(imported, goModule), "/"), components)
	}
	if strings.HasPrefix(imported, ".") {
		return nearestComponent(cleanPath(filepath.Join(file.dir, imported)), components)
	}
	if target, ok := fileByBase[strings.ToLower(filepath.Base(imported))]; ok {
		return target
	}
	if strings.HasPrefix(imported, "res://") {
		return nearestComponent(cleanPath(filepath.Dir(strings.TrimPrefix(imported, "res://"))), components)
	}
	return ""
}

func nearestComponent(path string, components map[string]*componentBuild) string {
	path = cleanPath(strings.TrimSuffix(strings.TrimSuffix(path, filepath.Ext(path)), "/index"))
	for path != "." && path != "" {
		if components[path] != nil {
			return path
		}
		path = cleanPath(filepath.Dir(path))
	}
	if components["."] != nil {
		return "."
	}
	return ""
}

func manifestFramework(name string) (string, bool) {
	switch name {
	case "go.mod":
		return "Go modules", true
	case "package.json":
		return "Node.js", true
	case "cargo.toml":
		return "Rust/Cargo", true
	case "project.godot":
		return "Godot", true
	case "pom.xml", "build.gradle", "build.gradle.kts":
		return "JVM", true
	case "pyproject.toml", "requirements.txt":
		return "Python", true
	case "composer.json":
		return "PHP/Composer", true
	}
	if strings.HasSuffix(name, ".uproject") {
		return "Unreal Engine", true
	}
	if strings.HasSuffix(name, ".sln") || strings.HasSuffix(name, ".csproj") {
		return ".NET", true
	}
	return "", false
}

func readGoModule(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 2 && fields[0] == "module" {
			return fields[1]
		}
	}
	return ""
}

func readSmall(path string) ([]byte, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > 2<<20 {
		return nil, errors.New("source file exceeds 2 MiB")
	}
	return os.ReadFile(path)
}

func stableID(prefix, value string) string {
	sum := sha256.Sum256([]byte(value))
	return prefix + "_" + hex.EncodeToString(sum[:6])
}
func cleanPath(path string) string {
	path = filepath.ToSlash(filepath.Clean(path))
	if path == "" {
		return "."
	}
	return path
}
func componentName(path, root string) string {
	if path == "." {
		return root
	}
	return filepath.Base(filepath.FromSlash(path))
}
func componentKind(path string) string {
	lower := strings.ToLower(path)
	for _, pair := range []struct{ token, kind string }{{"test", "tests"}, {"cmd", "application"}, {"api", "api"}, {"server", "service"}, {"client", "client"}, {"web", "client"}, {"data", "database"}, {"model", "model"}, {"asset", "assets"}, {"scene", "scene"}, {"plugin", "plugin"}} {
		if strings.Contains(lower, pair.token) {
			return pair.kind
		}
	}
	return "package"
}
func addUnique(values *[]string, value string) {
	if !slices.Contains(*values, value) {
		*values = append(*values, value)
	}
}
func dominant(counts map[string]int) string {
	best := ""
	max := -1
	for language, count := range counts {
		if count > max || (count == max && language < best) {
			best, max = language, count
		}
	}
	return best
}
