package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyExampleWorkflows(t *testing.T) {
	dir := t.TempDir()
	copied, err := copyExampleWorkflows(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if copied < 3 {
		t.Fatalf("copied=%d want at least 3", copied)
	}
	if _, err := os.Stat(filepath.Join(dir, "fleet-health.json")); err != nil {
		t.Fatalf("fleet-health example missing: %v", err)
	}
	copiedAgain, err := copyExampleWorkflows(dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if copiedAgain != 0 {
		t.Fatalf("copiedAgain=%d want 0", copiedAgain)
	}
}
