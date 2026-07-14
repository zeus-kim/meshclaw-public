package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/meshclaw/meshclaw/internal/opsdb"
)

func TestMCPOpsDBStatus(t *testing.T) {
	t.Setenv("MESHCLAW_OPSDB", filepath.Join(t.TempDir(), "opsdb"))
	result, err := callMCPTool("meshclaw_opsdb_status", map[string]interface{}{"ensure": true})
	if err != nil {
		t.Fatal(err)
	}
	report, ok := result.(opsDBStatusReport)
	if !ok {
		t.Fatalf("result=%T %#v", result, result)
	}
	if !report.Ensured {
		t.Fatal("expected ensured status")
	}
	if report.Boundary["opsdb"] == "" || report.Boundary["meshdb"] == "" {
		t.Fatalf("missing boundary: %+v", report.Boundary)
	}
}

func TestMCPOpsDBEventsAndRecord(t *testing.T) {
	root := filepath.Join(t.TempDir(), "opsdb")
	t.Setenv("MESHCLAW_OPSDB", root)
	db := opsdb.Default()
	if _, err := db.AppendEventRecord(opsdb.Event{
		Time:     time.Date(2026, 5, 26, 5, 0, 0, 0, time.UTC),
		Kind:     "risk",
		Node:     "g4",
		Severity: "warning",
		Summary:  "mine process observed on g4",
		Source:   "test",
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.AppendEvidenceIndex(opsdb.EvidenceIndexEntry{
		Time:     time.Date(2026, 5, 26, 5, 1, 0, 0, time.UTC),
		ID:       "ev-g4-process",
		Kind:     "process-top",
		Node:     "g4",
		Summary:  "process top captured",
		StoredAt: "/tmp/evidence.json",
	}); err != nil {
		t.Fatal(err)
	}

	result, err := callMCPTool("meshclaw_opsdb_events", map[string]interface{}{"node": "g4", "limit": float64(5)})
	if err != nil {
		t.Fatal(err)
	}
	report, ok := result.(opsDBEventsReport)
	if !ok {
		t.Fatalf("result=%T %#v", result, result)
	}
	if len(report.Events) != 1 || report.Events[0].Summary == "" {
		t.Fatalf("events=%+v", report.Events)
	}
	if len(report.Evidence) != 1 || report.Evidence[0].ID != "ev-g4-process" {
		t.Fatalf("evidence=%+v", report.Evidence)
	}

	result, err = callMCPTool("meshclaw_opsdb_record", map[string]interface{}{
		"node":     "g4",
		"kind":     "triage",
		"severity": "info",
		"summary":  "operator confirmed process was intentional",
		"source":   "mcp-test",
	})
	if err != nil {
		t.Fatal(err)
	}
	record, ok := result.(opsDBRecordReport)
	if !ok {
		t.Fatalf("record result=%T %#v", result, result)
	}
	if record.Event.Source != "mcp-test" || record.Event.Kind != "triage" {
		t.Fatalf("record=%+v", record)
	}
}

func TestMCPSurfaceIncludesOpsDB(t *testing.T) {
	surface := mcpSurfaceGuide()
	defaultTools, ok := surface["default_tools"].([]string)
	if !ok {
		t.Fatalf("default tools missing: %#v", surface)
	}
	for _, tool := range []string{"meshclaw_opsdb_status", "meshclaw_opsdb_events"} {
		if !containsString(defaultTools, tool) {
			t.Fatalf("default tools missing %s: %#v", tool, defaultTools)
		}
	}
	if !containsString(defaultTools, "meshclaw_opsdb_power_events") {
		t.Fatalf("default tools missing meshclaw_opsdb_power_events: %#v", defaultTools)
	}
	advancedTools, ok := surface["advanced_tools"].([]string)
	if !ok || !containsString(advancedTools, "meshclaw_opsdb_record") {
		t.Fatalf("advanced tools missing opsdb record: %#v", surface["advanced_tools"])
	}
}
