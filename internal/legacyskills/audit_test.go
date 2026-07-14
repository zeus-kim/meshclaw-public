package legacyskills

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAuditClassifiesLegacySkillsWithoutRunning(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, "morning-prayer", `{
  "name": "morning-prayer",
  "description": "Generate and speak morning prayer with TTS",
  "model": "claude-sonnet-4-20250514",
  "tools": ["bash"],
  "schedule": "every day at 06:00"
}`)
	writeSkill(t, dir, "custom-home-thing", `{
  "name": "custom-home-thing",
  "description": "Control a personal device",
  "model": "claude-sonnet-4-20250514",
  "tools": ["bash"]
}`)
	writeSkill(t, dir, "hello", `{
  "name": "hello",
  "description": "Simple greeting agent",
  "model": "claude-sonnet-4-20250514",
  "tools": ["bash"]
}`)

	report, err := Audit(time.Date(2026, 6, 3, 0, 0, 0, 0, time.UTC), dir, 20)
	if err != nil {
		t.Fatal(err)
	}
	if report.Kind != "meshclaw_legacy_skills_audit" {
		t.Fatalf("kind = %q", report.Kind)
	}
	if report.TotalSkills != 3 || report.InvalidFiles != 0 {
		t.Fatalf("unexpected counts: %#v", report)
	}
	if report.AlreadyCovered != 1 || report.MigrateCandidates != 1 || report.ArchiveCandidates != 1 {
		t.Fatalf("unexpected classification counts: covered=%d migrate=%d archive=%d", report.AlreadyCovered, report.MigrateCandidates, report.ArchiveCandidates)
	}
	if report.ApprovalNote == "" || report.UserMessage == "" {
		t.Fatalf("missing model-friendly guidance: %#v", report)
	}
	foundPrayer := false
	for _, skill := range report.Skills {
		if skill.Name == "morning-prayer" {
			foundPrayer = true
			if skill.RecommendedAction != "already_covered_by_mcp" {
				t.Fatalf("morning-prayer action = %q", skill.RecommendedAction)
			}
			if len(skill.ReplacementTools) == 0 {
				t.Fatalf("morning-prayer missing replacements: %#v", skill)
			}
		}
	}
	if !foundPrayer {
		t.Fatal("missing morning-prayer finding")
	}
}

func TestAuditMissingPathIsPlanResult(t *testing.T) {
	report, err := Audit(time.Now(), filepath.Join(t.TempDir(), "missing"), 10)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "path_missing" {
		t.Fatalf("status = %q", report.Status)
	}
	if report.TotalSkills != 0 {
		t.Fatalf("total = %d", report.TotalSkills)
	}
}

func writeSkill(t *testing.T, base, name, body string) {
	t.Helper()
	dir := filepath.Join(base, name)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "skill.json"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}
