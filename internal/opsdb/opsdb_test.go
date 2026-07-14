package opsdb

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultUsesEnvOverride(t *testing.T) {
	t.Setenv("MESHCLAW_OPSDB", "/tmp/meshclaw-opsdb-test")
	db := Default()
	if db.Root != "/tmp/meshclaw-opsdb-test" {
		t.Fatalf("Root = %q", db.Root)
	}
}

func TestPathsAreUnderRoot(t *testing.T) {
	db := Open("/tmp/opsdb")
	paths := db.Paths()
	if paths.NodesDir != "/tmp/opsdb/nodes" {
		t.Fatalf("NodesDir = %q", paths.NodesDir)
	}
	if paths.EvidenceIndex != "/tmp/opsdb/evidence-index.jsonl" {
		t.Fatalf("EvidenceIndex = %q", paths.EvidenceIndex)
	}
}

func TestEnsureCreatesPrivateDirectories(t *testing.T) {
	root := t.TempDir()
	db := Open(filepath.Join(root, "ops"))
	if err := db.Ensure(); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{db.Paths().Root, db.Paths().NodesDir, db.Paths().HistoryDir, db.Paths().DesiredDir, db.Paths().DriftDir, db.Paths().ApprovalsDir} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat %s: %v", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not dir", dir)
		}
		if info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("%s permissions too open: %v", dir, info.Mode().Perm())
		}
	}
}

func TestSafeNodePaths(t *testing.T) {
	db := Open("/tmp/opsdb")
	if got := db.NodePath("../g4 prod"); got != "/tmp/opsdb/nodes/g4_prod.json" {
		t.Fatalf("NodePath = %q", got)
	}
	if got := db.HistoryPath(""); got != "/tmp/opsdb/history/default.jsonl" {
		t.Fatalf("HistoryPath = %q", got)
	}
	if got := db.DesiredPath("fleet/base"); got != "/tmp/opsdb/desired/fleet_base.yaml" {
		t.Fatalf("DesiredPath = %q", got)
	}
}

func TestAppendEventWritesDriftEvent(t *testing.T) {
	db := Open(filepath.Join(t.TempDir(), "ops"))
	stored, err := db.AppendEventRecord(Event{
		Kind:       "process_observation",
		Node:       "g4",
		Severity:   "warning",
		Summary:    "mine process observed",
		Source:     "mcp",
		EvidenceID: "ev1",
		Tags:       []string{"process", "security"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if stored.Time.IsZero() {
		t.Fatal("stored event time should be set")
	}
	data, err := os.ReadFile(db.DriftPath("g4"))
	if err != nil {
		t.Fatal(err)
	}
	var event Event
	if err := json.Unmarshal(data[:len(data)-1], &event); err != nil {
		t.Fatal(err)
	}
	if event.Node != "g4" || event.Kind != "process_observation" || event.EvidenceID != "ev1" {
		t.Fatalf("bad event: %+v", event)
	}
}

func TestAppendEvidenceIndex(t *testing.T) {
	db := Open(filepath.Join(t.TempDir(), "ops"))
	err := db.AppendEvidenceIndex(EvidenceIndexEntry{
		ID:       "ev1",
		Kind:     "process-top",
		Node:     "g4",
		Summary:  "top processes",
		StoredAt: "/tmp/ev1.json",
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(db.Paths().EvidenceIndex)
	if err != nil {
		t.Fatal(err)
	}
	var entry EvidenceIndexEntry
	if err := json.Unmarshal(data[:len(data)-1], &entry); err != nil {
		t.Fatal(err)
	}
	if entry.ID != "ev1" || entry.Node != "g4" || entry.Time.IsZero() {
		t.Fatalf("bad evidence index entry: %+v", entry)
	}
}

func TestRecentFiltersEventsAndEvidence(t *testing.T) {
	db := Open(filepath.Join(t.TempDir(), "ops"))
	if _, err := db.AppendEventRecord(Event{Kind: "process_observation", Node: "g4", Summary: "mine"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.AppendEventRecord(Event{Kind: "service_observation", Node: "c1", Summary: "wire"}); err != nil {
		t.Fatal(err)
	}
	if err := db.AppendEvidenceIndex(EvidenceIndexEntry{ID: "ev-g4", Kind: "process-top", Node: "g4", Summary: "top"}); err != nil {
		t.Fatal(err)
	}
	recent, err := db.Recent(RecentOptions{Node: "g4", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(recent.Events) != 1 || recent.Events[0].Node != "g4" {
		t.Fatalf("bad recent events: %+v", recent.Events)
	}
	if len(recent.Evidence) != 1 || recent.Evidence[0].ID != "ev-g4" {
		t.Fatalf("bad recent evidence: %+v", recent.Evidence)
	}
}

func TestDetectPowerEventsGroupsCorrelatedBootChanges(t *testing.T) {
	db := Open(filepath.Join(t.TempDir(), "ops"))
	if err := db.Ensure(); err != nil {
		t.Fatal(err)
	}
	base := time.Date(2026, 5, 26, 11, 0, 0, 0, time.UTC)
	writeStoredReport(t, db, "g3", base.Add(-10*time.Minute), "boot-old-g3", 86400)
	writeStoredReport(t, db, "g3", base, "boot-new-g3", 120)
	writeStoredReport(t, db, "g4", base.Add(-9*time.Minute), "boot-old-g4", 86400)
	writeStoredReport(t, db, "g4", base.Add(2*time.Minute), "boot-new-g4", 180)
	writeStoredReport(t, db, "c1", base.Add(-9*time.Minute), "boot-c1", 86400)
	writeStoredReport(t, db, "c1", base.Add(2*time.Minute), "boot-c1", 86520)

	report, err := db.DetectPowerEvents(PowerEventOptions{Window: 5 * time.Minute, UptimeMax: time.Hour, MinNodes: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Incidents) != 1 {
		t.Fatalf("incidents=%+v", report.Incidents)
	}
	if got := len(report.Incidents[0].Nodes); got != 2 {
		t.Fatalf("nodes=%d, want 2", got)
	}
	if report.Incidents[0].Severity != "warning" {
		t.Fatalf("severity=%q", report.Incidents[0].Severity)
	}
}

func TestDetectPowerEventsIncludesRecordedFleetEvents(t *testing.T) {
	db := Open(filepath.Join(t.TempDir(), "ops"))
	if _, err := db.AppendEventRecord(Event{
		Kind:     "power_event",
		Node:     "fleet",
		Severity: "high",
		Summary:  "manual power event note",
	}); err != nil {
		t.Fatal(err)
	}
	report, err := db.DetectPowerEvents(PowerEventOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.RecordedEvents) != 1 || report.RecordedEvents[0].Summary != "manual power event note" {
		t.Fatalf("recorded events=%+v", report.RecordedEvents)
	}
}

func writeStoredReport(t *testing.T, db DB, node string, collected time.Time, boot string, uptime int64) {
	t.Helper()
	record := map[string]interface{}{
		"stored_at": collected,
		"report": map[string]interface{}{
			"node_name":    node,
			"hostname":     node,
			"collected_at": collected,
			"identity": map[string]interface{}{
				"boot_id_fingerprint": boot,
			},
			"system": map[string]interface{}{
				"uptime_seconds": uptime,
			},
		},
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(db.HistoryPath(node), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		t.Fatal(err)
	}
}
