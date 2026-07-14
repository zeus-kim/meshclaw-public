package assistantprofile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestInspectAssistantProfileWarnsWhenMissing(t *testing.T) {
	root := t.TempDir()
	report, err := Inspect(time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC), root)
	if err != nil {
		t.Fatal(err)
	}
	if report.Kind != Kind {
		t.Fatalf("kind=%q", report.Kind)
	}
	if report.Status != "warn" {
		t.Fatalf("status=%q warnings=%v", report.Status, report.Warnings)
	}
	if len(report.Identity.Files) != 3 || len(report.Memory.Files) != 2 {
		t.Fatalf("unexpected files: identity=%d memory=%d", len(report.Identity.Files), len(report.Memory.Files))
	}
	if len(report.Parity) < 5 {
		t.Fatalf("missing parity items: %#v", report.Parity)
	}
}

func TestInspectAssistantProfileReadyWithFilesAndSkill(t *testing.T) {
	root := t.TempDir()
	for _, path := range []string{
		filepath.Join(root, "assistant", "SOUL.md"),
		filepath.Join(root, "assistant", "USER.md"),
		filepath.Join(root, "assistant", "AGENTS.md"),
		filepath.Join(root, "memory", "USER.md"),
		filepath.Join(root, "memory", "MEMORY.md"),
		filepath.Join(root, "skills", "news", "SKILL.md"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("ok\n"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	report, err := Inspect(time.Now(), root)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "ok" {
		t.Fatalf("status=%q warnings=%v", report.Status, report.Warnings)
	}
	if report.Skills.Count != 1 || report.Skills.Items[0].Source != "skill_md" {
		t.Fatalf("skills=%#v", report.Skills)
	}
	if report.Skills.Items[0].Review != "unreviewed" || report.Skills.Items[0].Activation != "approval_required" {
		t.Fatalf("skill gate fields=%#v", report.Skills.Items[0])
	}
	if report.Memory.Policy != "bounded_approval_required" {
		t.Fatalf("memory policy=%q", report.Memory.Policy)
	}
}

func TestInspectAssistantProfileReportsReviewedSkillMarkdown(t *testing.T) {
	root := t.TempDir()
	skillDir := filepath.Join(root, "skills", "briefing")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	skill := "# Morning Briefing\n\nSummarize overnight ops, calendar, and weather without sending messages.\n\n## Review\nHuman-reviewed bounded read path.\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skill), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "REVIEWED.md"), []byte("reviewed 2026-06-12\n"), 0600); err != nil {
		t.Fatal(err)
	}

	report, err := Inspect(time.Now(), root)
	if err != nil {
		t.Fatal(err)
	}
	if report.Skills.Count != 1 {
		t.Fatalf("skills=%#v", report.Skills)
	}
	got := report.Skills.Items[0]
	if got.Title != "Morning Briefing" || !strings.Contains(got.Summary, "overnight ops") {
		t.Fatalf("skill summary=%#v", got)
	}
	if got.Status != "reviewed_approval_required" || got.Review != "reviewed" || got.Activation != "approval_required" {
		t.Fatalf("skill gate fields=%#v", got)
	}
}

func TestSkillTemplatesCoverAssistantParityScenarios(t *testing.T) {
	ids := map[string]bool{}
	for _, tmpl := range SkillTemplates() {
		if tmpl.ID == "" || tmpl.Title == "" || tmpl.Summary == "" || tmpl.Example == "" || tmpl.Status != "available_template" {
			t.Fatalf("template not marketplace-ready: %#v", tmpl)
		}
		ids[tmpl.ID] = true
	}
	for _, want := range []string{
		"research-report",
		"market-analysis",
		"meeting-minutes",
		"travel-prep",
		"shopping-prep",
		"mail-priority",
		"calendar-reminder",
		"daily-briefing",
		"work-reuse",
		"voice-briefing",
	} {
		if !ids[want] {
			t.Fatalf("missing parity skill template %q in %#v", want, ids)
		}
	}
}

func TestInstallSkillPlansAndActivatesPolicyGatedSkill(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	plan, err := InstallSkill(now, root, "Shopping Prep", "Compare options and stop before purchase.", false)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != "planned" || plan.Written || len(plan.Files) != 4 {
		t.Fatalf("plan=%#v", plan)
	}
	if _, err := os.Stat(filepath.Join(root, "skills", "shopping-prep", "SKILL.md")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote skill file: %v", err)
	}

	installed, err := InstallSkill(now, root, "Shopping Prep", "Compare options and stop before purchase.", true)
	if err != nil {
		t.Fatal(err)
	}
	if installed.Status != "active_policy_gated" || !installed.Written {
		t.Fatalf("installed=%#v", installed)
	}
	for _, name := range []string{"SKILL.md", "REVIEWED.md", "ACTIVE.md", "TEST.md"} {
		if _, err := os.Stat(filepath.Join(root, "skills", "shopping-prep", name)); err != nil {
			t.Fatalf("%s missing: %v", name, err)
		}
	}
	report, err := Inspect(now, root)
	if err != nil {
		t.Fatal(err)
	}
	if report.Skills.Count != 1 || report.Skills.Items[0].Status != "active_policy_gated" || report.Skills.Items[0].Activation != "active_policy_gated" {
		t.Fatalf("skills=%#v", report.Skills)
	}

	again, err := InstallSkill(now, root, "Shopping Prep", "Replace existing skill.", true)
	if err != nil {
		t.Fatal(err)
	}
	if again.Status != "rejected" || !again.Rejected {
		t.Fatalf("duplicate install=%#v", again)
	}
}

func TestInstallSkillRejectsRawSecret(t *testing.T) {
	root := t.TempDir()
	report, err := InstallSkill(time.Now(), root, "Secret Skill", "Use token=placeholder-secret-value-123456", true)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "rejected" || !report.Rejected || len(report.GuardFindings) == 0 {
		t.Fatalf("report=%#v", report)
	}
}

func TestInstallSkillKeepsKoreanNameSlugUnique(t *testing.T) {
	root := t.TempDir()
	report, err := InstallSkill(time.Now(), root, "시장조사", "출처 있는 시장조사 요약을 만들고 발송 전에는 멈춘다", false)
	if err != nil {
		t.Fatal(err)
	}
	if report.Slug == "" || report.Slug == "custom-assistant-skill" {
		t.Fatalf("slug=%q", report.Slug)
	}
	if !strings.Contains(report.Slug, "u") {
		t.Fatalf("expected encoded unicode slug, got %q", report.Slug)
	}
}

func TestTestSkillReadsGeneratedSmokeTest(t *testing.T) {
	root := t.TempDir()
	if _, err := InstallSkill(time.Now(), root, "Research Report", "Collect sources and stop before sending.", true); err != nil {
		t.Fatal(err)
	}
	report, err := TestSkill(time.Now(), root, "research-report")
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "ready" || report.Skill.Name != "research-report" {
		t.Fatalf("report=%#v", report)
	}
	if !strings.Contains(report.Prompt, "Collect sources") || len(report.Expected) == 0 {
		t.Fatalf("test content=%#v", report)
	}
	for _, item := range report.Safety {
		if strings.Contains(strings.ToLower(item), "purchase") {
			return
		}
	}
	t.Fatalf("missing purchase safety boundary: %#v", report.Safety)
}

func TestTestSkillMissingSkillNeedsName(t *testing.T) {
	root := t.TempDir()
	report, err := TestSkill(time.Now(), root, "")
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "needs_skill" {
		t.Fatalf("report=%#v", report)
	}
}

func TestSetSkillActivationPlansDeactivatesAndReactivates(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	if _, err := InstallSkill(now, root, "Research Report", "Collect sources and stop before sending.", true); err != nil {
		t.Fatal(err)
	}
	plan, err := SetSkillActivation(now, root, "research-report", false, false)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Status != "planned" || plan.Written {
		t.Fatalf("plan=%#v", plan)
	}
	off, err := SetSkillActivation(now, root, "research-report", false, true)
	if err != nil {
		t.Fatal(err)
	}
	if off.Status != "reviewed_approval_required" || !off.Written {
		t.Fatalf("off=%#v", off)
	}
	if _, err := os.Stat(filepath.Join(root, "skills", "research-report", "ACTIVE.md")); !os.IsNotExist(err) {
		t.Fatalf("ACTIVE.md should be removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "skills", "research-report", "DISABLED.md")); err != nil {
		t.Fatalf("DISABLED.md missing: %v", err)
	}
	on, err := SetSkillActivation(now, root, "research-report", true, true)
	if err != nil {
		t.Fatal(err)
	}
	if on.Status != "active_policy_gated" || !on.Written {
		t.Fatalf("on=%#v", on)
	}
	report, err := Inspect(now, root)
	if err != nil {
		t.Fatal(err)
	}
	if report.Skills.Items[0].Activation != "active_policy_gated" {
		t.Fatalf("skills=%#v", report.Skills)
	}
}

func TestInitDefaultsPlansAndCreatesProfile(t *testing.T) {
	root := t.TempDir()
	plan, err := InitDefaults(time.Now(), root, false)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Execute {
		t.Fatal("dry-run init reported execute")
	}
	if len(plan.Planned) != 5 || len(plan.Created) != 0 {
		t.Fatalf("plan planned=%d created=%d", len(plan.Planned), len(plan.Created))
	}
	created, err := InitDefaults(time.Now(), root, true)
	if err != nil {
		t.Fatal(err)
	}
	if !created.Execute || len(created.Created) != 5 {
		t.Fatalf("created execute=%t created=%d", created.Execute, len(created.Created))
	}
	if created.Profile.Status != "ok" {
		t.Fatalf("profile status=%q warnings=%v", created.Profile.Status, created.Profile.Warnings)
	}
	again, err := InitDefaults(time.Now(), root, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Created) != 0 || len(again.Existing) != 5 {
		t.Fatalf("idempotent created=%d existing=%d", len(again.Created), len(again.Existing))
	}
}

func TestAddMemoryIsPlanOnlyUntilApproved(t *testing.T) {
	root := t.TempDir()
	report, err := AddMemory(time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC), root, "user", "사용자는 한국어 답변을 선호한다", false)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "planned" || report.Written {
		t.Fatalf("report=%#v", report)
	}
	if _, err := os.Stat(filepath.Join(root, "memory", "USER.md")); !os.IsNotExist(err) {
		t.Fatalf("dry-run wrote memory file: %v", err)
	}
}

func TestAddMemoryApprovedAppendsEntry(t *testing.T) {
	root := t.TempDir()
	report, err := AddMemory(time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC), root, "assistant", "뉴스 브리핑은 짧게 먼저 요약한다", true)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "written" || !report.Written {
		t.Fatalf("report=%#v", report)
	}
	data, err := os.ReadFile(filepath.Join(root, "memory", "MEMORY.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "2026-06-12") || !strings.Contains(text, "뉴스 브리핑은 짧게 먼저 요약한다") {
		t.Fatalf("memory missing entry:\n%s", text)
	}
}

func TestAddMemoryRejectsRawSecret(t *testing.T) {
	root := t.TempDir()
	secret := "placeholder-secret-value-123456"
	report, err := AddMemory(time.Now(), root, "user", "token="+secret, true)
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "rejected" || !report.Rejected || len(report.GuardFindings) == 0 {
		t.Fatalf("report=%#v", report)
	}
	if strings.Contains(report.Text, secret) {
		t.Fatalf("redacted text leaked secret: %s", report.Text)
	}
}

func TestLoadMemoryContextIncludesApprovedEntries(t *testing.T) {
	root := t.TempDir()
	if _, err := InitDefaults(time.Now(), root, true); err != nil {
		t.Fatal(err)
	}
	if _, err := AddMemory(time.Date(2026, 6, 12, 0, 0, 0, 0, time.UTC), root, "user", "사용자는 짧은 한국어 답변을 선호한다", true); err != nil {
		t.Fatal(err)
	}
	ctx, err := LoadMemoryContext(time.Now(), root, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if ctx.Status != "ok" {
		t.Fatalf("status=%q warnings=%v", ctx.Status, ctx.Warnings)
	}
	if !strings.Contains(ctx.Text, "User memory") || !strings.Contains(ctx.Text, "짧은 한국어 답변") {
		t.Fatalf("context missing memory:\n%s", ctx.Text)
	}
}

func TestBuildMemorySnapshotSelectsRelevantLayers(t *testing.T) {
	root := t.TempDir()
	if _, err := InitDefaults(time.Now(), root, true); err != nil {
		t.Fatal(err)
	}
	if _, err := AddMemory(time.Now(), root, "user", "사용자는 결론부터 짧게 답하는 것을 선호한다", true); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "skills", "briefing"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "skills", "briefing", "SKILL.md"), []byte("# Briefing Skill\n\nSummarize first, then details.\n"), 0600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := BuildMemorySnapshot(time.Now(), root, "지난번 서버 보안 보고와 답변 선호 기억해?")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Kind != "meshclaw_assistant_memory_snapshot" {
		t.Fatalf("kind=%q", snapshot.Kind)
	}
	for _, want := range []string{"long_term", "episodic", "ops"} {
		if !stringSliceContains(snapshot.Use, want) {
			t.Fatalf("snapshot use missing %s: %#v", want, snapshot.Use)
		}
	}
	layer := findSnapshotLayer(snapshot, "procedural")
	if layer.Items == 0 || !strings.Contains(layer.Summary, "Briefing Skill") {
		t.Fatalf("procedural layer=%#v", layer)
	}
}

func TestBuildMemorySnapshotQuestionLayerContract(t *testing.T) {
	root := t.TempDir()
	if _, err := InitDefaults(time.Now(), root, true); err != nil {
		t.Fatal(err)
	}
	if _, err := AddMemory(time.Now(), root, "user", "사용자는 결론부터 짧게 답하는 것을 선호한다", true); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		text string
		use  []string
	}{
		{
			name: "past_server_security_report",
			text: "지난번 서버 보안 보고 기억해?",
			use:  []string{"long_term", "episodic", "ops"},
		},
		{
			name: "answer_preference",
			text: "내 답변 선호 기억해?",
			use:  []string{"long_term"},
		},
		{
			name: "identity_capability_question",
			text: "넌 누구고 뭘 할 수 있어?",
			use:  nil,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			snapshot, err := BuildMemorySnapshot(time.Now(), root, tc.text)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(snapshot.Use, ",") != strings.Join(tc.use, ",") {
				t.Fatalf("use=%#v want=%#v", snapshot.Use, tc.use)
			}
		})
	}
}

func TestBuildMemorySnapshotLayerDescriptionsMatchContract(t *testing.T) {
	root := t.TempDir()
	if _, err := InitDefaults(time.Now(), root, true); err != nil {
		t.Fatal(err)
	}
	snapshot, err := BuildMemorySnapshot(time.Now(), root, "메모리 구조")
	if err != nil {
		t.Fatal(err)
	}
	wantUses := map[string]string{
		"short_term": "직전 대화 흐름",
		"working":    "승인 대기",
		"long_term":  "승인된 사용자 선호",
		"episodic":   "과거 발송, 보고서",
		"procedural": "반복 절차",
		"ops":        "노드 상태",
	}
	for name, want := range wantUses {
		layer := findSnapshotLayer(snapshot, name)
		if layer.Name == "" {
			t.Fatalf("missing layer %s", name)
		}
		if !strings.Contains(layer.Use, want) {
			t.Fatalf("%s use=%q want substring %q", name, layer.Use, want)
		}
		if strings.TrimSpace(layer.Boundary) == "" || strings.TrimSpace(layer.Retention) == "" {
			t.Fatalf("%s missing boundary/retention: %#v", name, layer)
		}
	}
}

func findSnapshotLayer(snapshot MemorySnapshot, name string) MemoryLayer {
	for _, layer := range snapshot.Layers {
		if layer.Name == name {
			return layer
		}
	}
	return MemoryLayer{}
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
