package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFallbackProjectRootFindsRepoMarker(t *testing.T) {
	root := t.TempDir()
	pipelineDir := filepath.Join(root, "tools", "pipeline")

	if err := os.MkdirAll(pipelineDir, 0o755); err != nil {
		t.Fatalf("mkdir pipeline dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pipelineDir, "go.mod"), []byte("module example\n"), 0o644); err != nil {
		t.Fatalf("write go.mod marker: %v", err)
	}

	got := fallbackProjectRoot(pipelineDir)
	if got != root {
		t.Fatalf("expected root %q, got %q", root, got)
	}
}

func TestFallbackProjectRootLegacyPipelineLayout(t *testing.T) {
	root := t.TempDir()
	pipelineDir := filepath.Join(root, "pipeline")

	if err := os.MkdirAll(pipelineDir, 0o755); err != nil {
		t.Fatalf("mkdir pipeline dir: %v", err)
	}

	got := fallbackProjectRoot(pipelineDir)
	if got != root {
		t.Fatalf("expected root %q, got %q", root, got)
	}
}
