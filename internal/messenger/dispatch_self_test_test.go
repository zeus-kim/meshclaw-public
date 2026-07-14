package messenger

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestDispatchSelfTestKeepsReportRoomsOneWayUnderGlobalAssistantMode(t *testing.T) {
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(t.TempDir(), "targets.json"))
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-report", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := UpsertTarget(Target{ID: "argos-ops", Channel: "signal", GroupID: "group-ops", Label: "운영 보고방", Mode: "ops"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := UpsertTarget(Target{ID: "argos-assistant", Channel: "signal", GroupID: "group-assistant", Label: "비서방", Mode: "assistant"}); err != nil {
		t.Fatal(err)
	}
	result, err := DispatchSelfTest("assistant")
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Fatalf("self-test should pass: %#v", result.Problems)
	}
	byID := map[string]DispatchSelfTestItem{}
	for _, item := range result.Targets {
		byID[item.TargetID] = item
	}
	for _, id := range []string{"argos-briefing", "argos-ops"} {
		item := byID[id]
		if !item.OneWay || item.ReplyPreview != "" || item.Status != "one-way" {
			t.Fatalf("%s should stay one-way under global assistant mode: %#v", id, item)
		}
	}
	assistant := byID["argos-assistant"]
	if assistant.OneWay || assistant.ReplyPreview == "" || assistant.Status != "reply-ready" {
		t.Fatalf("assistant should be reply-ready: %#v", assistant)
	}
}

func TestDispatchSelfTestTreatsGuardModeAsInteractive(t *testing.T) {
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(t.TempDir(), "targets.json"))
	if _, _, err := UpsertTarget(Target{ID: "argos-guard", Channel: "signal", GroupID: "guard-group", Label: "Guard 방", Mode: "guard"}); err != nil {
		t.Fatal(err)
	}
	result, err := DispatchSelfTest("guard")
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Fatalf("guard mode should be treated as interactive and pass: %#v", result.Problems)
	}
	byID := map[string]DispatchSelfTestItem{}
	for _, item := range result.Targets {
		byID[item.TargetID] = item
	}
	guard := byID["argos-guard"]
	if guard.OneWay || guard.ReplyPreview == "" || guard.Status != "reply-ready" {
		t.Fatalf("guard target should remain reply-ready in interactive mode: %#v", guard)
	}
}

func TestDispatchSelfTestWarnsOnMixedModeGroupBinding(t *testing.T) {
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(t.TempDir(), "targets.json"))
	if _, _, err := UpsertTarget(Target{ID: "report-room", Channel: "signal", GroupID: "same-group", Mode: "briefing"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := UpsertTarget(Target{ID: "assistant-room", Channel: "signal", GroupID: "same-group", Mode: "assistant"}); err != nil {
		t.Fatal(err)
	}
	result, err := DispatchSelfTest("")
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Fatalf("mixed binding should warn, not fail: %#v", result.Problems)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "same Signal group_id") {
		t.Fatalf("expected mixed group warning: %#v", result.Warnings)
	}
}

func TestDispatchSelfTestWarnsOnArgosDirectRecipient(t *testing.T) {
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(t.TempDir(), "targets.json"))
	if _, _, err := UpsertTarget(Target{ID: "argos-assistant", Channel: "signal", Mode: "assistant", Recipient: "+11000000000", Label: "비서"}); err != nil {
		t.Fatal(err)
	}
	result, err := DispatchSelfTest("")
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Fatalf("direct recipient Argos binding should warn only: %#v", result.Problems)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "direct recipient") {
		t.Fatalf("expected direct recipient warning for Argos user-facing target: %#v", result.Warnings)
	}
}

func TestDispatchSelfTestCollectsArgosDirectAndMixedModeWarnings(t *testing.T) {
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(t.TempDir(), "targets.json"))
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "same-group", Mode: "briefing"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := UpsertTarget(Target{ID: "argos-assistant", Channel: "signal", GroupID: "same-group", Mode: "assistant"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := UpsertTarget(Target{ID: "argos-single-recipient", Channel: "signal", Recipient: "+11000000000", Mode: "assistant"}); err != nil {
		t.Fatal(err)
	}
	result, err := DispatchSelfTest("")
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK {
		t.Fatalf("combined warnings should remain non-blocking: %#v", result.Problems)
	}
	if len(result.Warnings) != 2 {
		t.Fatalf("expected 2 warnings, got %d: %#v", len(result.Warnings), result.Warnings)
	}
	hasMixed := false
	hasDirect := false
	for _, warning := range result.Warnings {
		if strings.Contains(warning, "same Signal group_id") {
			hasMixed = true
		}
		if strings.Contains(warning, "direct recipient") {
			hasDirect = true
		}
	}
	if !hasMixed || !hasDirect {
		t.Fatalf("expected mixed-mode and direct-recipient warnings: %#v", result.Warnings)
	}
}
