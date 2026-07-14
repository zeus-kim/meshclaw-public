package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/meshclaw/meshclaw/internal/messenger"
	"github.com/meshclaw/meshclaw/internal/policy"
)

func TestParseSignalClick(t *testing.T) {
	click, err := parseSignalClick("12.5, 34")
	if err != nil {
		t.Fatalf("parseSignalClick returned error: %v", err)
	}
	if click.X != 12.5 || click.Y != 34 {
		t.Fatalf("click=%+v", click)
	}
}

func TestParseSignalCallOptions(t *testing.T) {
	opts, err := parseSignalCallOptions([]string{
		"owner-signal",
		"--audio", "/tmp/brief.aiff",
		"--start-click", "10,20",
		"--hangup-click", "30,40",
		"--timeout", "45",
		"--restore-delay", "2",
		"--approve",
		"--execute",
	})
	if err != nil {
		t.Fatalf("parseSignalCallOptions returned error: %v", err)
	}
	if opts.TargetID != "owner-signal" || opts.Audio != "/tmp/brief.aiff" {
		t.Fatalf("opts=%+v", opts)
	}
	if !opts.Execute || !opts.Approved {
		t.Fatalf("execute=%t approved=%t", opts.Execute, opts.Approved)
	}
	if opts.StartClick == nil || opts.StartClick.X != 10 || opts.StartClick.Y != 20 {
		t.Fatalf("start click=%+v", opts.StartClick)
	}
	if opts.HangupClick == nil || opts.HangupClick.X != 30 || opts.HangupClick.Y != 40 {
		t.Fatalf("hangup click=%+v", opts.HangupClick)
	}
}

func TestSignalCallPreflightDryRunAllowsMissingRunner(t *testing.T) {
	audio := tempSignalCallAudio(t)
	opts := signalCallOptions{TargetID: "owner-signal", Audio: audio}
	target := messenger.Target{ID: "owner-signal", Channel: "signal"}
	decision := policy.Result{Decision: policy.RequireApproval, Reason: "test"}
	doctor := callDoctorResult{SignalRunning: false, UIRunnerOK: false, BlackHoleAvailable: false}

	problems := signalCallPreflightProblems(opts, target, decision, doctor)
	if len(problems) != 0 {
		t.Fatalf("problems=%v, want none for dry-run", problems)
	}
}

func TestSignalCallPreflightExecuteRequiresApprovalAndRunner(t *testing.T) {
	audio := tempSignalCallAudio(t)
	opts := signalCallOptions{TargetID: "owner-signal", Audio: audio, Execute: true}
	target := messenger.Target{ID: "owner-signal", Channel: "signal"}
	decision := policy.Result{Decision: policy.RequireApproval, Reason: "test approval"}
	doctor := callDoctorResult{SignalRunning: true, UIRunnerOK: false, BlackHoleAvailable: true, SignalLog: "/tmp/signal.log"}

	problems := signalCallPreflightProblems(opts, target, decision, doctor)
	requireProblem(t, problems, "execute mode requires --approve")
	requireProblem(t, problems, "policy requires approval")
	requireProblem(t, problems, "Argos UI Runner is not ready")
}

func TestCheckArgosUIRunnerReadsAccessibilityTrust(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Fatalf("path=%s, want /health", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"accessibility_trusted":true,"signal_running":true}`))
	}))
	defer server.Close()

	health, errText := checkArgosUIRunner(server.URL)
	if errText != "" {
		t.Fatalf("checkArgosUIRunner error=%q", errText)
	}
	if !health.OK || !health.AccessibilityTrusted || !health.SignalRunning {
		t.Fatalf("health=%+v", health)
	}
}

func TestCheckArgosUIRunnerReturnsProblemsWhenUnready(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":false,"accessibility_trusted":false,"signal_running":true,"problems":["macOS Accessibility permission is not granted to Argos UI Runner."]}`))
	}))
	defer server.Close()

	health, errText := checkArgosUIRunner(server.URL)
	if errText == "" {
		t.Fatalf("expected error text for unready runner")
	}
	if health.OK || health.AccessibilityTrusted {
		t.Fatalf("health=%+v", health)
	}
	if !strings.Contains(errText, "Accessibility permission") {
		t.Fatalf("error=%q", errText)
	}
}

func TestSignalCallDryRunRoutesAudioBeforeStartingCall(t *testing.T) {
	audio := tempSignalCallAudio(t)
	tempSignalCallTargets(t)
	result, err := assistantSignalCall([]string{"owner-signal", "--audio", audio, "--dry-run"})
	if err != nil {
		t.Fatalf("assistantSignalCall returned error: %v", err)
	}
	if len(result.Commands) < 4 {
		t.Fatalf("commands=%v", result.Commands)
	}
	if !strings.Contains(result.Commands[0], "call-route start") {
		t.Fatalf("first command must route audio before ringing, got %v", result.Commands)
	}
	if strings.Contains(result.Commands[1], "start-call") {
		t.Fatalf("start-call must not run before audio route, got %v", result.Commands)
	}
}

func tempSignalCallAudio(t *testing.T) string {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "brief-*.aiff")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("audio"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return file.Name()
}

func tempSignalCallTargets(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "targets.json")
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", path)
	if _, _, err := messenger.UpsertTarget(messenger.Target{ID: "owner-signal", Channel: "signal", Recipient: "+821012345678", Label: "owner"}); err != nil {
		t.Fatal(err)
	}
	return path
}

func requireProblem(t *testing.T, problems []string, want string) {
	t.Helper()
	for _, problem := range problems {
		if strings.Contains(problem, want) {
			return
		}
	}
	t.Fatalf("problems=%v, want substring %q", problems, want)
}
