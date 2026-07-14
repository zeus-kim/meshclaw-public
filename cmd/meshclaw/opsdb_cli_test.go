package main

import (
	"path/filepath"
	"testing"

	"github.com/meshclaw/meshclaw/internal/opsdb"
)

func TestBuildOpsDBStatusKeepsMeshDBBoundary(t *testing.T) {
	root := t.TempDir()
	db := opsdb.Open(filepath.Join(root, "opsdb"))
	if err := db.Ensure(); err != nil {
		t.Fatal(err)
	}
	report := buildOpsDBStatus(db, true)
	if report.Root != filepath.Join(root, "opsdb") {
		t.Fatalf("Root = %q", report.Root)
	}
	if report.Boundary["meshdb"] == "" || report.Boundary["opsdb"] == "" {
		t.Fatalf("missing boundary: %+v", report.Boundary)
	}
	if report.Counts["nodes"] != 0 || report.Counts["history"] != 0 {
		t.Fatalf("unexpected counts: %+v", report.Counts)
	}
	if len(report.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %+v", report.Warnings)
	}
}

func TestOpsDBRecordRequiresSummary(t *testing.T) {
	err := opsDBRecord([]string{"--node", "g4", "--json"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestOpsDBEventsRejectsBadLimit(t *testing.T) {
	err := opsDBEvents([]string{"--limit", "0", "--json"})
	if err == nil {
		t.Fatal("expected error")
	}
}
