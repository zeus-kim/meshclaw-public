package fileorg

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDownloadsCleanupPlanFindsCandidatesWithoutMutating(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC)
	oldTime := now.Add(-40 * 24 * time.Hour)
	cases := map[string][]byte{
		"installer.dmg": []byte(strings.Repeat("a", 10)),
		"archive.zip":   []byte(strings.Repeat("b", 20)),
		"old-note.txt":  []byte(strings.Repeat("c", 30)),
		"fresh.txt":     []byte(strings.Repeat("d", 40)),
		"large.mov":     []byte(strings.Repeat("e", 2*1024*1024)),
	}
	for name, data := range cases {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Chtimes(filepath.Join(dir, "old-note.txt"), oldTime, oldTime); err != nil {
		t.Fatal(err)
	}

	plan, err := DownloadsCleanupPlan(now, dir, 30, 1, 20)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ScannedFiles != 5 || plan.CandidateFiles != 4 {
		t.Fatalf("unexpected counts: %#v", plan)
	}
	if plan.Status != "review_ready" {
		t.Fatalf("unexpected status: %#v", plan)
	}
	if !strings.Contains(plan.UserMessage, "검토") {
		t.Fatalf("missing user-facing review guidance: %#v", plan.UserMessage)
	}
	names := map[string]bool{}
	for _, candidate := range plan.Candidates {
		names[candidate.Name] = true
	}
	for _, want := range []string{"installer.dmg", "archive.zip", "old-note.txt", "large.mov"} {
		if !names[want] {
			t.Fatalf("missing candidate %q in %#v", want, plan.Candidates)
		}
		if _, err := os.Stat(filepath.Join(dir, want)); err != nil {
			t.Fatalf("plan must not delete %s: %v", want, err)
		}
	}
	if names["fresh.txt"] {
		t.Fatalf("fresh small file should not be a cleanup candidate: %#v", plan.Candidates)
	}
	if !strings.Contains(plan.ApprovalNote, "Plan-only") {
		t.Fatalf("missing plan-only note: %#v", plan)
	}
}

func TestDownloadsCleanupPlanReportsPermissionWithoutMCPError(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dir, 0700)

	plan, err := DownloadsCleanupPlan(time.Now().UTC(), dir, 30, 500, 10)
	if err != nil {
		t.Fatalf("permission error should be reported in plan, not returned as tool error: %v", err)
	}
	if plan.AccessError == "" {
		t.Fatalf("missing access error in plan: %#v", plan)
	}
	if plan.Status != "needs_access" {
		t.Fatalf("unexpected status: %#v", plan)
	}
	if !strings.Contains(plan.UserMessage, "접근 권한") {
		t.Fatalf("missing Korean access guidance: %#v", plan.UserMessage)
	}
	if len(plan.AccessGuidance) == 0 {
		t.Fatalf("missing access guidance steps: %#v", plan)
	}
	if !strings.Contains(strings.Join(plan.Next, " "), "Grant file access") {
		t.Fatalf("missing access guidance: %#v", plan.Next)
	}
}

func TestDownloadsCleanupApplyRequiresExplicitApproval(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "old-installer.dmg")
	if err := os.WriteFile(path, []byte("installer"), 0600); err != nil {
		t.Fatal(err)
	}

	plan, err := DownloadsCleanupApply(time.Now().UTC(), []string{path}, "", true, false)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.ApprovalMissing || plan.Status != "review_required" {
		t.Fatalf("expected approval-missing review plan: %#v", plan)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file should not move without approval: %v", err)
	}
	if len(plan.PlannedMoves) != 1 || plan.PlannedMoves[0].Destination == "" {
		t.Fatalf("expected planned destination: %#v", plan)
	}
}

func TestDownloadsCleanupApplyMovesOnlyApprovedRegularFiles(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "archive.zip")
	folder := filepath.Join(dir, "folder")
	if err := os.WriteFile(file, []byte("archive"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(folder, 0700); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "review")

	plan, err := DownloadsCleanupApply(time.Now().UTC(), []string{file, folder}, dest, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != "completed" || plan.CompletedFiles != 1 {
		t.Fatalf("unexpected apply result: %#v", plan)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Fatalf("source file should have moved, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "archive.zip")); err != nil {
		t.Fatalf("moved file missing: %v", err)
	}
	if _, err := os.Stat(folder); err != nil {
		t.Fatalf("folder should not move: %v", err)
	}
	if len(plan.Skipped) != 1 || !strings.Contains(plan.Skipped[0].Error, "Folders") && !strings.Contains(strings.ToLower(plan.Skipped[0].Error), "folders") {
		t.Fatalf("expected skipped folder result: %#v", plan.Skipped)
	}
	if !strings.Contains(plan.ApprovalNote, "never deletes") {
		t.Fatalf("missing non-delete boundary: %#v", plan.ApprovalNote)
	}
}
