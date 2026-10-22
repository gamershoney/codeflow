package discovery

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestScanFindsMixedRepositoryArchitecture(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "go.mod", "module example.test/app\n\ngo 1.25\n")
	writeFixture(t, root, "cmd/api/main.go", "package main\nimport \"example.test/app/internal/store\"\nfunc main() {}\n")
	writeFixture(t, root, "internal/store/store.go", "package store\ntype Store struct{}\nfunc Open() Store { return Store{} }\n")
	writeFixture(t, root, "package.json", `{"name":"mixed-app"}`)
	writeFixture(t, root, "web/app.ts", "import { save } from './data'\nexport function start() { save() }\n")
	writeFixture(t, root, "web/data.ts", "export function save() {}\n")
	writeFixture(t, root, "game/project.godot", "[application]\nconfig/name=\"Fixture\"\n")
	writeFixture(t, root, "game/scenes/main.tscn", "[gd_scene load_steps=2]\n[ext_resource path=\"res://scripts/player.gd\" type=\"Script\"]\n")
	writeFixture(t, root, "game/scripts/player.gd", "class_name Player\n")

	report, err := Scan(root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if report.SourceFiles != 6 {
		t.Fatalf("source files = %d, want 6", report.SourceFiles)
	}
	if report.Kind != "game" || !slices.Contains(report.Frameworks, "Godot") || !slices.Contains(report.Frameworks, "Go modules") || !slices.Contains(report.Frameworks, "Node.js") {
		t.Fatalf("unexpected classification: kind=%s frameworks=%v", report.Kind, report.Frameworks)
	}
	if len(report.Components) < 5 {
		t.Fatalf("components = %d, want at least 5", len(report.Components))
	}
	if len(report.Dependencies) < 2 {
		t.Fatalf("dependencies = %d, want cross-component Go and Godot edges", len(report.Dependencies))
	}
}

func TestScanSkipsGeneratedDirectoriesAndHonorsLimit(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "src/main.go", "package src\nfunc Main() {}\n")
	writeFixture(t, root, "node_modules/pkg/index.js", "function generated() {}\n")
	writeFixture(t, root, "other.go", "package main\n")
	report, err := Scan(root, Options{MaxFiles: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Truncated {
		t.Fatal("expected truncated report")
	}
	if report.Languages["JavaScript"] != 0 {
		t.Fatal("node_modules should be skipped")
	}
}

func writeFixture(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
