package datadoctor

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestCheckReportsWarningsWithoutDeletingEvidence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	now := time.Date(2026, 5, 26, 6, 0, 0, 0, time.UTC)
	publicDir := filepath.Join(home, ".meshclaw", "public", "argos")
	evidenceDir := filepath.Join(home, ".meshclaw", "evidence", "2026-05-26")
	if err := os.MkdirAll(publicDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(evidenceDir, 0700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 121; i++ {
		if err := os.WriteFile(filepath.Join(publicDir, "report-"+strconv.Itoa(i)+".html"), []byte("html"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	evidencePath := filepath.Join(evidenceDir, "20260526T055500Z-123456789-schedule-local_hygiene-ops.json")
	if err := os.WriteFile(evidencePath, []byte(`{"summary":"ok"}`), 0600); err != nil {
		t.Fatal(err)
	}
	report, err := Check(now)
	if err != nil {
		t.Fatal(err)
	}
	if report.OK {
		t.Fatalf("expected warning state for oversized public report set")
	}
	if report.Evidence.Files != 1 {
		t.Fatalf("evidence files = %d, want 1", report.Evidence.Files)
	}
	if _, err := os.Stat(evidencePath); err != nil {
		t.Fatalf("data doctor must not delete evidence: %v", err)
	}
	if len(report.Evidence.Latest) != 1 || report.Evidence.Latest[0].Kind != "schedule-local_hygiene" {
		t.Fatalf("unexpected latest evidence: %#v", report.Evidence.Latest)
	}
}

func TestEvidenceKindFromIDKeepsAssistantNews(t *testing.T) {
	tests := map[string]string{
		"20260526T055500Z-123456789-schedule-local_hygiene-ops": "schedule-local_hygiene",
		"20260526T055500Z-123456789-assistant-news-assistant":   "assistant-news",
		"20260526T055500Z-123456789-monitor-agent-fleet":        "monitor-agent",
	}
	for id, want := range tests {
		if got := evidenceKindFromID(id); got != want {
			t.Fatalf("evidenceKindFromID(%q) = %q, want %q", id, got, want)
		}
	}
}

func TestCheckWarnsOnCorruptTopLevelStateJSON(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, ".meshclaw")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "messenger-targets.json"), []byte(`{"targets":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "schedule-state.json"), []byte(`{"last_run":`), 0600); err != nil {
		t.Fatal(err)
	}

	report, err := Check(time.Date(2026, 5, 26, 6, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if report.OK {
		t.Fatalf("expected corrupt state warning: %#v", report)
	}
	if len(report.StateFiles) != 2 {
		t.Fatalf("state files=%#v, want 2", report.StateFiles)
	}
	var sawBad, sawGood bool
	for _, file := range report.StateFiles {
		switch file.ID {
		case "schedule-state":
			sawBad = file.Exists && !file.OK && file.Error != ""
		case "messenger-targets":
			sawGood = file.Exists && file.OK
		}
	}
	if !sawBad || !sawGood {
		t.Fatalf("unexpected state validation: %#v", report.StateFiles)
	}
	if len(report.Warnings) == 0 || !strings.Contains(strings.Join(report.Warnings, "\n"), "state file is not valid JSON: schedule-state") {
		t.Fatalf("missing state warning: %#v", report.Warnings)
	}
}

func TestEvidenceArchivePlanKeepsNewestDateDirsAndDoesNotDelete(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, ".meshclaw", "evidence")
	for _, day := range []string{"2026-05-24", "2026-05-25", "2026-05-26"} {
		dir := filepath.Join(root, day)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, strings.ReplaceAll(day, "-", "")+"T010203Z-123-kind-host.json"), []byte(`{"ok":true}`), 0600); err != nil {
			t.Fatal(err)
		}
	}

	plan, err := EvidenceArchivePlan(time.Date(2026, 5, 27, 0, 0, 0, 0, time.UTC), 2)
	if err != nil {
		t.Fatal(err)
	}
	if plan.TotalFiles != 3 || plan.CandidateFiles != 1 {
		t.Fatalf("unexpected plan counts: %#v", plan)
	}
	if len(plan.Candidates) != 1 || plan.Candidates[0].Date != "2026-05-24" {
		t.Fatalf("unexpected candidates: %#v", plan.Candidates)
	}
	if _, err := os.Stat(filepath.Join(root, "2026-05-24")); err != nil {
		t.Fatalf("archive plan must not delete evidence directories: %v", err)
	}
	if !strings.Contains(plan.ApprovalNote, "Plan-only") {
		t.Fatalf("missing plan-only approval note: %#v", plan)
	}
}
