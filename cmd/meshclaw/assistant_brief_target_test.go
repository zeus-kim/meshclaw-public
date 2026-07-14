package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/meshclaw/meshclaw/internal/assistantbrief"
	"github.com/meshclaw/meshclaw/internal/messenger"
)

func TestAssistantMenuForReportTargetIsNonInteractive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, ".meshclaw", "messenger-targets.json"))
	if _, _, err := messenger.UpsertTarget(messenger.Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-report", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatal(err)
	}

	brief := assistantBriefForTarget(assistantbrief.Menu(), "argos-briefing")
	if strings.Contains(brief.Text, "번호로") || strings.Contains(brief.Text, "1.") {
		t.Fatalf("report room menu should not be interactive:\n%s", brief.Text)
	}
	if !strings.Contains(brief.Text, "one-way") || !strings.Contains(brief.Text, "argos-assistant") {
		t.Fatalf("report room replacement should explain the route:\n%s", brief.Text)
	}
}

func TestAssistantMenuForInteractiveTargetStaysInteractive(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, ".meshclaw", "messenger-targets.json"))
	if _, _, err := messenger.UpsertTarget(messenger.Target{ID: "argos-assistant", Channel: "signal", GroupID: "group-assistant", Label: "비서방", Mode: "assistant"}); err != nil {
		t.Fatal(err)
	}

	brief := assistantBriefForTarget(assistantbrief.Menu(), "argos-assistant")
	if !strings.Contains(brief.Text, "번호로 답하면") {
		t.Fatalf("assistant room menu should stay interactive:\n%s", brief.Text)
	}
}
