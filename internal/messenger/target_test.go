package messenger

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTargetStoreLifecycle(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(dir, "targets.json"))

	store, target, err := UpsertTarget(Target{ID: "Owner Signal", Channel: "signal", Recipient: "+821012345678", Label: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	if target.ID != "owner-signal" {
		t.Fatalf("id=%q", target.ID)
	}
	if len(store.Targets) != 1 {
		t.Fatalf("targets=%d", len(store.Targets))
	}
	list, err := ListTargets()
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Targets) != 1 || list.Targets[0].Recipient != "+821012345678" {
		t.Fatalf("list=%#v", list)
	}
	_, removed, err := RemoveTarget("owner-signal")
	if err != nil {
		t.Fatal(err)
	}
	if !removed {
		t.Fatal("target was not removed")
	}
}

func TestSendReportDryRunDoesNotExecuteSignalCLI(t *testing.T) {
	home := seedLatestBundle(t)
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, ".meshclaw", "messenger-targets.json"))
	t.Setenv("MESHCLAW_SIGNAL_CLI", "/no/such/signal-cli")
	if _, _, err := UpsertTarget(Target{ID: "owner", Channel: "signal", Recipient: "+821012345678"}); err != nil {
		t.Fatal(err)
	}
	result, err := Send(SendOptions{TargetID: "owner", Ref: "latest", Kind: "report"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Mode != "dry-run" || result.Executed {
		t.Fatalf("result=%#v", result)
	}
	if !result.Success {
		t.Fatalf("dry-run should succeed: %#v", result)
	}
	if result.RawSecretsIncluded {
		t.Fatal("raw secrets must not be included")
	}
	if len(result.Command) == 0 || result.Command[0] != "/no/such/signal-cli" {
		t.Fatalf("command=%#v", result.Command)
	}
	if !strings.Contains(result.Text, "MeshClaw 보고서") {
		t.Fatalf("text=%q", result.Text)
	}
	if len(result.Attachments) != 1 || filepath.Base(result.Attachments[0]) != "messenger-report.md" {
		t.Fatalf("report should attach Obsidian markdown, attachments=%#v", result.Attachments)
	}
	if strings.Contains(strings.Join(result.Command, " "), "report.html") {
		t.Fatalf("report command should not attach html: %#v", result.Command)
	}
}

func TestSignalGroupTargetUsesGroupFlag(t *testing.T) {
	home := seedLatestBundle(t)
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, ".meshclaw", "messenger-targets.json"))
	t.Setenv("MESHCLAW_SIGNAL_CLI", "/usr/local/bin/signal-cli")
	t.Setenv("MESHCLAW_SIGNAL_ACCOUNT", "+821037579780")
	if _, _, err := UpsertTarget(Target{ID: "argos-guard", Channel: "signal", GroupID: "group-123", Label: "Argos Guard"}); err != nil {
		t.Fatal(err)
	}
	result, err := Send(SendOptions{TargetID: "argos-guard", Ref: "latest", Kind: "report"})
	if err != nil {
		t.Fatal(err)
	}
	command := strings.Join(result.Command, " ")
	if !strings.Contains(command, " -g group-123") {
		t.Fatalf("expected signal group send command, got %#v", result.Command)
	}
	if !strings.Contains(command, "--notify-self") {
		t.Fatalf("group command should notify self for desktop/mobile visibility: %#v", result.Command)
	}
	if strings.Contains(command, "+821012345678") {
		t.Fatalf("group command should not include a direct recipient: %#v", result.Command)
	}
}

func TestSignalGroupNotifySelfCanBeDisabled(t *testing.T) {
	home := seedLatestBundle(t)
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, ".meshclaw", "messenger-targets.json"))
	t.Setenv("MESHCLAW_SIGNAL_CLI", "/usr/local/bin/signal-cli")
	t.Setenv("MESHCLAW_SIGNAL_NOTIFY_SELF", "off")
	if _, _, err := UpsertTarget(Target{ID: "argos-guard", Channel: "signal", GroupID: "group-123", Label: "Argos Guard"}); err != nil {
		t.Fatal(err)
	}
	result, err := Send(SendOptions{TargetID: "argos-guard", Ref: "latest", Kind: "report"})
	if err != nil {
		t.Fatal(err)
	}
	if command := strings.Join(result.Command, " "); strings.Contains(command, "--notify-self") {
		t.Fatalf("notify-self should be disabled by env, got %#v", result.Command)
	}
}

func TestSignalTargetModesAllowDefaultRoomPreset(t *testing.T) {
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(t.TempDir(), "targets.json"))
	for _, mode := range []string{"guard", "ops", "chat", "briefing", "assistant"} {
		if _, _, err := UpsertTarget(Target{
			ID:      mode,
			Channel: "signal",
			GroupID: "group-" + mode,
			Label:   mode,
			Mode:    mode,
		}); err != nil {
			t.Fatalf("mode %q should be accepted: %v", mode, err)
		}
	}
	if _, _, err := UpsertTarget(Target{ID: "bad", Channel: "signal", GroupID: "group-bad", Mode: "unknown"}); err == nil {
		t.Fatal("expected invalid mode error")
	}
}

func TestParseRoomModelCommand(t *testing.T) {
	model, baseURL, ok := parseRoomModelCommand("모델 gpt-oss:20b http://g4:11434/v1")
	if !ok {
		t.Fatal("expected model command")
	}
	if model != "gpt-oss:20b" || baseURL != "http://g4:11434/v1" {
		t.Fatalf("model=%q baseURL=%q", model, baseURL)
	}
	if _, _, ok := parseRoomModelCommand("안녕"); ok {
		t.Fatal("unexpected model command")
	}
	if _, _, ok := parseRoomModelCommand("/model"); ok {
		t.Fatal("bare /model should show usage instead of changing model")
	}
	if !isRoomModelUsageCommand("/model") || !isRoomModelUsageCommand("모델") {
		t.Fatal("expected bare model command to show usage")
	}
}

func TestSignalSendCanAttachVoiceNote(t *testing.T) {
	home := seedLatestBundle(t)
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, ".meshclaw", "messenger-targets.json"))
	t.Setenv("MESHCLAW_SIGNAL_CLI", "/usr/local/bin/signal-cli")
	if _, _, err := UpsertTarget(Target{ID: "ops", Channel: "signal", GroupID: "group-123", Label: "Ops"}); err != nil {
		t.Fatal(err)
	}
	result, err := Send(SendOptions{TargetID: "ops", Kind: "text", Text: "brief", Attachments: []string{"/tmp/brief.aiff"}, VoiceNote: true})
	if err != nil {
		t.Fatal(err)
	}
	command := strings.Join(result.Command, " ")
	if !strings.Contains(command, "--attachment /tmp/brief.aiff") || !strings.Contains(command, "--voice-note") {
		t.Fatalf("expected attachment voice-note command, got %#v", result.Command)
	}
	if strings.Index(command, "-g group-123") > strings.Index(command, "--attachment") {
		t.Fatalf("group destination must appear before attachment args to avoid signal-cli nargs swallowing it: %#v", result.Command)
	}
}

func TestSignalDirectAttachmentKeepsRecipientBeforeAttachment(t *testing.T) {
	home := seedLatestBundle(t)
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, ".meshclaw", "messenger-targets.json"))
	t.Setenv("MESHCLAW_SIGNAL_CLI", "/usr/local/bin/signal-cli")
	if _, _, err := UpsertTarget(Target{ID: "bot", Channel: "signal", Recipient: "+821037579780", Label: "Bot"}); err != nil {
		t.Fatal(err)
	}
	result, err := Send(SendOptions{TargetID: "bot", Kind: "text", Text: "brief", Attachments: []string{"/tmp/brief.txt"}})
	if err != nil {
		t.Fatal(err)
	}
	command := strings.Join(result.Command, " ")
	if !strings.Contains(command, "+821037579780 --attachment /tmp/brief.txt") {
		t.Fatalf("recipient must appear before attachment args, got %#v", result.Command)
	}
}

func TestSendTextPromotesAttachmentMarkers(t *testing.T) {
	home := seedLatestBundle(t)
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, ".meshclaw", "messenger-targets.json"))
	t.Setenv("MESHCLAW_SIGNAL_CLI", "/usr/local/bin/signal-cli")
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-briefing", Label: "Briefing", Mode: "briefing"}); err != nil {
		t.Fatal(err)
	}
	attachment := filepath.Join(home, "decision.svg")
	if err := os.WriteFile(attachment, []byte("<svg></svg>"), 0600); err != nil {
		t.Fatal(err)
	}
	text := strings.Join([]string{
		"구매 가능 판정입니다.",
		"meshclaw-attachment: " + attachment,
		"아직 구매 버튼은 누르지 않았습니다.",
	}, "\n")
	result, err := Send(SendOptions{TargetID: "argos-briefing", Kind: "text", Text: text})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(result.Text, "meshclaw-attachment:") || strings.Contains(result.Text, attachment) {
		t.Fatalf("visible send text leaked attachment marker:\n%s", result.Text)
	}
	if !strings.Contains(result.Text, "구매 가능 판정입니다.") || !strings.Contains(result.Text, "아직 구매 버튼은 누르지 않았습니다.") {
		t.Fatalf("visible send text lost user-facing content:\n%s", result.Text)
	}
	if len(result.Attachments) != 1 || result.Attachments[0] != attachment {
		t.Fatalf("attachments=%#v", result.Attachments)
	}
	command := strings.Join(result.Command, " ")
	if strings.Contains(command, "meshclaw-attachment:") {
		t.Fatalf("signal command leaked marker: %#v", result.Command)
	}
	if !strings.Contains(command, "--attachment "+attachment) {
		t.Fatalf("signal command missing promoted attachment: %#v", result.Command)
	}
}

func TestMacBookCannotExecuteArgosSignalSend(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", "/Users/example")
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, ".meshclaw", "messenger-targets.json"))
	t.Setenv("MESHCLAW_SIGNAL_CLI", "/no/such/signal-cli")
	if _, _, err := UpsertTarget(Target{ID: "argos-briefing", Channel: "signal", GroupID: "group-123", Label: "Argos Briefing", Mode: "briefing"}); err != nil {
		t.Fatal(err)
	}
	result, err := Send(SendOptions{TargetID: "argos-briefing", Kind: "text", Text: "brief", Execute: true})
	if err == nil {
		t.Fatal("expected MacBook Argos Signal send guard")
	}
	if result.Executed {
		t.Fatalf("guarded send must not execute: %#v", result)
	}
	if !strings.Contains(err.Error(), "Mac mini Argos Signal runtime") {
		t.Fatalf("unexpected guard error: %v", err)
	}
}

func TestSendExecuteTimesOutSignalCLI(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(home, ".meshclaw", "messenger-targets.json"))
	fakeSignal := filepath.Join(home, "signal-cli")
	if err := os.WriteFile(fakeSignal, []byte("#!/bin/sh\nexec sleep 5\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MESHCLAW_SIGNAL_CLI", fakeSignal)
	if _, _, err := UpsertTarget(Target{ID: "owner", Channel: "signal", Recipient: "+821012345678"}); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	result, err := Send(SendOptions{TargetID: "owner", Kind: "text", Text: "brief", Execute: true, TimeoutSeconds: 1})
	if err == nil {
		t.Fatal("expected timeout error")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("send did not time out promptly, elapsed=%s", elapsed)
	}
	if !result.TimedOut || result.Success {
		t.Fatalf("expected timed out failure, got %#v", result)
	}
	if result.TimeoutSeconds != 1 {
		t.Fatalf("timeout seconds=%d", result.TimeoutSeconds)
	}
	if !strings.Contains(result.Error, "timed out after 1 seconds") {
		t.Fatalf("unexpected error: %q", result.Error)
	}
}

func TestSendSignalReplyTimesOutSignalCLI(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	fakeSignal := filepath.Join(home, "signal-cli")
	if err := os.WriteFile(fakeSignal, []byte("#!/bin/sh\nexec sleep 5\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MESHCLAW_SIGNAL_CLI", fakeSignal)
	t.Setenv("MESHCLAW_SIGNAL_SEND_TIMEOUT_SECONDS", "1")

	start := time.Now()
	ok, detail := sendSignalReply(Target{ID: "owner", Channel: "signal", Recipient: "+821012345678"}, "brief")
	if ok {
		t.Fatal("expected sendSignalReply timeout failure")
	}
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("sendSignalReply did not time out promptly, elapsed=%s", elapsed)
	}
	if !strings.Contains(detail, "timed out after 1 seconds") {
		t.Fatalf("unexpected timeout detail: %q", detail)
	}
}

func TestSignalTargetRequiresOneDestination(t *testing.T) {
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(t.TempDir(), "targets.json"))
	if _, _, err := UpsertTarget(Target{ID: "empty", Channel: "signal"}); err == nil {
		t.Fatal("expected missing destination error")
	}
	if _, _, err := UpsertTarget(Target{ID: "both", Channel: "signal", Recipient: "+821012345678", GroupID: "group-123"}); err == nil {
		t.Fatal("expected mutually exclusive destination error")
	}
}

func seedLatestBundle(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	bundle := filepath.Join(home, ".meshclaw", "evidence", "2026-05-22", "demo")
	if err := os.MkdirAll(bundle, 0700); err != nil {
		t.Fatal(err)
	}
	data := []byte(`{
  "success": true,
  "workflow": "demo",
  "mode": "dry-run",
  "generated_at": "2026-05-22T00:00:00Z",
  "bundle_dir": "` + bundle + `",
  "evidence_bundle": "` + bundle + `",
  "summary": {"total": 1, "succeeded": 1, "failed": 0, "approval_required": 0, "skipped": 0, "retryable": 0},
  "steps": [{"success": true, "workflow": "demo", "step": "inspect", "title": "Inspect", "status": "ok", "policy_decision": "allow"}]
}`)
	if err := os.WriteFile(filepath.Join(bundle, "execution.json"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(bundle, filepath.Join(home, ".meshclaw", "evidence", "latest")); err != nil {
		t.Fatal(err)
	}
	return home
}
