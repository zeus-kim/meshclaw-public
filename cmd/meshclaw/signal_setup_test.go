package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meshclaw/meshclaw/internal/messenger"
	"github.com/meshclaw/meshclaw/internal/scheduler"
)

func TestSignalSetupUXWarningsExplainNameVerificationAndGuardSafety(t *testing.T) {
	warnings := signalSetupUXWarnings([]messenger.Target{
		{ID: "argos-guard", Channel: "signal", GroupID: "group-guard", Mode: "guard"},
		{ID: "meshclaw-ops", Channel: "signal", GroupID: "group-ops", Mode: "ops"},
	})
	if len(warnings) != 2 {
		t.Fatalf("warnings=%#v, want 2", warnings)
	}
	if warnings[0].ID != "signal_name_not_verified_ui" || warnings[0].Severity != "info" {
		t.Fatalf("first warning=%#v", warnings[0])
	}
	if warnings[1].ID != "guard_requires_safety_number" || warnings[1].AppliesTo != "argos-guard" {
		t.Fatalf("guard warning=%#v", warnings[1])
	}
}

func TestSignalSetupUXWarningsEmptyWithoutTargets(t *testing.T) {
	if warnings := signalSetupUXWarnings(nil); len(warnings) != 0 {
		t.Fatalf("warnings=%#v, want none", warnings)
	}
}

func TestMissingScheduleTargets(t *testing.T) {
	missing := missingScheduleTargets([]messenger.Target{{ID: "argos-assistant"}})
	want := []string{"argos-briefing", "argos-ops"}
	if strings.Join(missing, ",") != strings.Join(want, ",") {
		t.Fatalf("missing=%#v want %#v", missing, want)
	}
	missing = missingScheduleTargets([]messenger.Target{
		{ID: "argos-assistant"},
		{ID: "argos-briefing"},
		{ID: "argos-ops"},
	})
	if len(missing) != 0 {
		t.Fatalf("missing=%#v, want none", missing)
	}
}

func TestScheduleRunnerIdleWarningExplainsLaunchdOneShot(t *testing.T) {
	warning, ok := scheduleRunnerIdleWarning(daemonActionResult{
		Service:   "schedule-runner",
		Installed: true,
		Running:   false,
	})
	if !ok {
		t.Fatal("expected idle warning for installed stopped schedule-runner")
	}
	if warning.ID != "schedule_runner_idle" || warning.Severity != "info" {
		t.Fatalf("warning=%#v", warning)
	}
	if !strings.Contains(warning.Message, "normal between 15-minute launchd runs") {
		t.Fatalf("warning should explain launchd one-shot idle state: %#v", warning)
	}
	if !strings.Contains(warning.Next, "meshclaw schedule status") {
		t.Fatalf("warning should point to schedule status: %#v", warning)
	}
	if _, ok := scheduleRunnerIdleWarning(daemonActionResult{Installed: true, Running: true}); ok {
		t.Fatal("running schedule-runner should not warn")
	}
	if _, ok := scheduleRunnerIdleWarning(daemonActionResult{Installed: false, Running: false}); ok {
		t.Fatal("missing schedule-runner should be a setup problem, not idle warning")
	}
}

func TestScheduleRunnerFailureWarningExplainsLastStatus(t *testing.T) {
	warning, ok := scheduleRunnerFailureWarning(daemonActionResult{
		Service:    "schedule-runner",
		Installed:  true,
		Running:    false,
		LastStatus: "1",
	})
	if !ok {
		t.Fatal("expected warning for failed schedule-runner last status")
	}
	if warning.ID != "schedule_runner_last_failed" || warning.Severity != "warning" {
		t.Fatalf("warning=%#v", warning)
	}
	if !strings.Contains(warning.Message, "last launchd exit status was 1") {
		t.Fatalf("warning should mention last status: %#v", warning)
	}
	if !strings.Contains(warning.Next, "schedule-runner.err.log") || !strings.Contains(warning.Next, "schedule run-due --execute") {
		t.Fatalf("warning should point to logs and catch-up: %#v", warning)
	}
	for _, result := range []daemonActionResult{
		{Installed: true, Running: false, LastStatus: "0"},
		{Installed: true, Running: true, LastStatus: "1"},
		{Installed: false, Running: false, LastStatus: "1"},
	} {
		if _, ok := scheduleRunnerFailureWarning(result); ok {
			t.Fatalf("unexpected failure warning for %#v", result)
		}
	}
}

func TestLaunchdStartCommandsAvoidRepeatedBootstrapWhenAlreadyRunning(t *testing.T) {
	opts := daemonInstallOptions{Label: "ai.meshclaw.schedule-runner", PlistPath: "/tmp/ai.meshclaw.schedule-runner.plist"}
	commands := launchdStartCommands(opts, daemonActionResult{Installed: true, Running: true, PID: "123", LastStatus: "0"})
	if len(commands) != 0 {
		t.Fatalf("running launchd service should not be bootstrapped/kickstarted again: %#v", commands)
	}
}

func TestLaunchdStartCommandsKickstartLoadedIdleServiceWithoutBootstrap(t *testing.T) {
	opts := daemonInstallOptions{Label: "ai.meshclaw.schedule-runner", PlistPath: "/tmp/ai.meshclaw.schedule-runner.plist"}
	commands := launchdStartCommands(opts, daemonActionResult{Installed: true, Running: false, PID: "-", LastStatus: "0"})
	if len(commands) != 1 {
		t.Fatalf("loaded idle launchd service should only be kickstarted, got %#v", commands)
	}
	got := strings.Join(commands[0], " ")
	if !strings.Contains(got, "kickstart") || strings.Contains(got, "bootstrap") {
		t.Fatalf("loaded idle launchd service should avoid bootstrap, got %q", got)
	}
}

func TestLaunchdStartCommandsBootstrapOnlyWhenServiceIsNotLoaded(t *testing.T) {
	opts := daemonInstallOptions{Label: "ai.meshclaw.schedule-runner", PlistPath: "/tmp/ai.meshclaw.schedule-runner.plist"}
	commands := launchdStartCommands(opts, daemonActionResult{Installed: true, Running: false})
	if len(commands) != 2 {
		t.Fatalf("unloaded launchd service should bootstrap then kickstart, got %#v", commands)
	}
	if strings.Join(commands[0], " ") != "launchctl bootstrap "+launchdDomain()+" /tmp/ai.meshclaw.schedule-runner.plist" {
		t.Fatalf("first command should bootstrap plist, got %#v", commands[0])
	}
	if !strings.Contains(strings.Join(commands[1], " "), "kickstart") {
		t.Fatalf("second command should kickstart service, got %#v", commands[1])
	}
}

func TestSignalDispatchLogHealthFlagsRecentSuspiciousErrors(t *testing.T) {
	dir := t.TempDir()
	errLog := filepath.Join(dir, "argos-signal-dispatcher.err.log")
	now := time.Date(2026, 6, 5, 11, 0, 0, 0, time.UTC)
	if err := os.WriteFile(errLog, []byte("info: warm\nERROR signal dispatch failed: timeout\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(errLog, now.Add(-5*time.Minute), now.Add(-5*time.Minute)); err != nil {
		t.Fatal(err)
	}

	health := readSignalDispatchLogHealth(filepath.Join(dir, "dispatcher.log"), errLog, now)
	if health.Error != "" {
		t.Fatalf("health error=%s", health.Error)
	}
	if len(health.RecentSuspiciousLines) != 1 {
		t.Fatalf("suspicious=%#v, want one recent error", health.RecentSuspiciousLines)
	}
	if !strings.Contains(health.RecentSuspiciousLines[0], "timeout") {
		t.Fatalf("unexpected suspicious line: %#v", health.RecentSuspiciousLines)
	}
}

func TestSignalDispatchLogHealthIgnoresOldErrors(t *testing.T) {
	dir := t.TempDir()
	errLog := filepath.Join(dir, "argos-signal-dispatcher.err.log")
	now := time.Date(2026, 6, 5, 11, 0, 0, 0, time.UTC)
	if err := os.WriteFile(errLog, []byte("ERROR old failure\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(errLog, now.Add(-2*time.Hour), now.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}

	health := readSignalDispatchLogHealth(filepath.Join(dir, "dispatcher.log"), errLog, now)
	if len(health.RecentSuspiciousLines) != 0 {
		t.Fatalf("old log should not be flagged: %#v", health)
	}
}

func TestDevLoopRepoRootPrefersProjectsMeshclawAfterRepoMove(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Fatalf("restore working directory: %v", err)
		}
	})
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	projectRoot := filepath.Join(home, "Projects", "meshclaw")
	if err := os.MkdirAll(filepath.Join(projectRoot, "scripts"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "scripts", "dev-loop.sh"), []byte("#!/bin/sh\n"), 0700); err != nil {
		t.Fatal(err)
	}

	if got := devLoopRepoRoot(); got != projectRoot {
		t.Fatalf("devLoopRepoRoot()=%s, want %s", got, projectRoot)
	}
}

func TestSignalSetupScheduleStatusCountsDueJobs(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "schedule-state.json")
	t.Setenv("MESHCLAW_SCHEDULE_STATE", statePath)
	now := time.Date(2026, 5, 26, 8, 0, 0, 0, time.UTC)

	status := signalSetupScheduleStatus(now)
	if status.StatePath != statePath || status.Jobs == 0 {
		t.Fatalf("unexpected status: %#v", status)
	}
	if status.Status != "backlog" {
		t.Fatalf("empty state should be backlog: %#v", status)
	}
	if status.DueCount != status.Jobs-1 {
		t.Fatalf("empty state should make enabled jobs due except hourly jobs that have not reached their minute yet: %#v", status)
	}
	if len(status.DueJobs) != status.Jobs-1 || status.DueJobs[0] == "" {
		t.Fatalf("due job ids should be listed: %#v", status)
	}
	if status.NextDueJob == "" || !status.NextDue.Equal(now) || status.NextDueInSeconds != 0 {
		t.Fatalf("due schedule should report next due as now: %#v", status)
	}

	state := scheduler.State{Path: statePath, LastRun: map[string]time.Time{}}
	for _, job := range scheduler.DefaultPlan().Jobs {
		state.LastRun[job.ID] = now
	}
	state.LastRun["local-ai-briefing"] = now.Add(7 * time.Minute)
	if err := scheduler.SaveState(state); err != nil {
		t.Fatal(err)
	}
	status = signalSetupScheduleStatus(now.Add(10 * time.Minute))
	if status.DueCount != 0 || status.Error != "" {
		t.Fatalf("recent state should have no due jobs: %#v", status)
	}
	if status.Status != "healthy" {
		t.Fatalf("recent state should be healthy: %#v", status)
	}
	if len(status.DueJobs) != 0 {
		t.Fatalf("recent state should not list due jobs: %#v", status)
	}
	if status.NextDueJob != "mail-watch" || status.NextDueInSeconds != 300 {
		t.Fatalf("next due should be the 15m mail-watch job in 300s: %#v", status)
	}
}

func TestSignalSetupDataDoctorStatusSummarizesRetentionHealth(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	now := time.Date(2026, 5, 26, 8, 0, 0, 0, time.UTC)
	publicDir := filepath.Join(home, ".meshclaw", "public", "argos")
	logDir := filepath.Join(home, ".meshclaw", "logs")
	evidenceDir := filepath.Join(home, ".meshclaw", "evidence", "2026-05-26")
	for _, dir := range []string{publicDir, logDir, evidenceDir} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 121; i++ {
		if err := os.WriteFile(filepath.Join(publicDir, fmt.Sprintf("report-%03d.html", i)), []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(logDir, "argos.log"), []byte("log"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(evidenceDir, "20260526T080000Z-000000000-data-doctor-meshclaw.json"), []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}

	status := signalSetupDataDoctorStatus(now)
	if status.OK || status.Error != "" {
		t.Fatalf("status=%#v, want warning state without error", status)
	}
	if status.PublicFiles != 121 || status.LogFiles != 1 || status.EvidenceFiles != 1 {
		t.Fatalf("unexpected counts: %#v", status)
	}
	if len(status.Warnings) == 0 || !strings.Contains(status.Warnings[0], "public Argos files") {
		t.Fatalf("missing public retention warning: %#v", status.Warnings)
	}
}

func TestSignalSetupDataDoctorStatusIncludesInvalidStateCounts(t *testing.T) {
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

	status := signalSetupDataDoctorStatus(time.Date(2026, 5, 26, 8, 0, 0, 0, time.UTC))
	if status.OK {
		t.Fatalf("status=%#v, want invalid state warning", status)
	}
	if status.StateFilesChecked != 2 || status.StateFilesInvalid != 1 {
		t.Fatalf("unexpected state counts: %#v", status)
	}
	if len(status.InvalidStateFiles) != 1 || status.InvalidStateFiles[0] != "schedule-state" {
		t.Fatalf("invalid files=%#v, want schedule-state", status.InvalidStateFiles)
	}
}

func TestScheduleDueWarningPointsToCatchUp(t *testing.T) {
	warning, ok := scheduleDueWarning(signalSetupSchedule{Jobs: 8, DueCount: 2})
	if !ok {
		t.Fatal("expected warning when scheduled jobs are due")
	}
	if warning.ID != "schedule_due" || warning.Severity != "warning" {
		t.Fatalf("warning=%#v", warning)
	}
	if !strings.Contains(warning.Message, "2 scheduled job") {
		t.Fatalf("warning should mention due count: %#v", warning)
	}
	if !strings.Contains(warning.Next, "schedule run-due --execute") {
		t.Fatalf("warning should point to immediate catch-up: %#v", warning)
	}
	if _, ok := scheduleDueWarning(signalSetupSchedule{Jobs: 8, DueCount: 0}); ok {
		t.Fatal("no due jobs should not warn")
	}
	if _, ok := scheduleDueWarning(signalSetupSchedule{Jobs: 8, DueCount: 2, Error: "bad state"}); ok {
		t.Fatal("state load error is already a problem and should not produce due warning")
	}
}

func TestRepairSignalSetupInstallsAndStartsMissingDaemons(t *testing.T) {
	origInstallSignal := installSignalDispatcherForSetup
	origManageSignal := manageSignalDispatcherForSetup
	origInstallSchedule := installScheduleRunnerForSetup
	origManageSchedule := manageScheduleRunnerForSetup
	origPause := pauseSignalDispatcherForSetup
	origRoomsDoctor := signalSetupRoomsDoctor
	defer func() {
		installSignalDispatcherForSetup = origInstallSignal
		manageSignalDispatcherForSetup = origManageSignal
		installScheduleRunnerForSetup = origInstallSchedule
		manageScheduleRunnerForSetup = origManageSchedule
		pauseSignalDispatcherForSetup = origPause
		signalSetupRoomsDoctor = origRoomsDoctor
	}()

	var calls []string
	installSignalDispatcherForSetup = func(opts daemonInstallOptions) (daemonActionResult, error) {
		calls = append(calls, "install-signal")
		return daemonActionResult{Service: "signal-dispatcher", Action: "install", Installed: true, Running: false, Options: opts}, nil
	}
	manageSignalDispatcherForSetup = func(action string) (daemonActionResult, error) {
		calls = append(calls, "signal-"+action)
		return daemonActionResult{Service: "signal-dispatcher", Action: action, Installed: true, Running: true}, nil
	}
	installScheduleRunnerForSetup = func(opts daemonInstallOptions) (daemonActionResult, error) {
		calls = append(calls, "install-schedule")
		return daemonActionResult{Service: "schedule-runner", Action: "install", Installed: true, Running: false, Options: opts}, nil
	}
	manageScheduleRunnerForSetup = func(action string) (daemonActionResult, error) {
		calls = append(calls, "schedule-"+action)
		return daemonActionResult{Service: "schedule-runner", Action: action, Installed: true, Running: false, LastStatus: "0"}, nil
	}
	pauseSignalDispatcherForSetup = func(autoPause bool) (*messenger.DispatcherPauseStatus, func()) {
		calls = append(calls, "rooms-pause")
		return &messenger.DispatcherPauseStatus{Requested: autoPause}, func() { calls = append(calls, "rooms-resume") }
	}
	signalSetupRoomsDoctor = func(opts messenger.RoomDoctorOptions) (messenger.RoomDoctorResult, error) {
		calls = append(calls, "rooms-doctor")
		return messenger.RoomDoctorResult{}, nil
	}

	result := repairSignalSetup(signalSetupReport{CallRunner: argosRunnerSetupReport{OK: true}})
	if len(result.Problems) != 0 {
		t.Fatalf("problems=%#v, want none", result.Problems)
	}
	want := "install-signal,signal-start,install-schedule,schedule-start,rooms-pause,rooms-doctor,rooms-resume"
	if got := strings.Join(calls, ","); got != want {
		t.Fatalf("calls=%s want %s", got, want)
	}
	if len(result.Actions) != 4 {
		t.Fatalf("actions=%#v, want install/start for both daemons", result.Actions)
	}
}

func TestSignalDispatcherPlistIncludesVisionEnvironment(t *testing.T) {
	t.Setenv("MESHCLAW_SIGNAL_VISION_MODEL", "gemma3:4b")
	t.Setenv("MESHCLAW_SIGNAL_VISION_BASE_URL", "http://g4:11434/v1")
	t.Setenv("MESHCLAW_SIGNAL_VISION_API_KEY", "ollama")

	opts := defaultSignalDispatcherOptions()
	plist := signalDispatcherPlist(opts)
	for _, want := range []string{
		"<key>MESHCLAW_SIGNAL_VISION_MODEL</key>",
		"<string>gemma3:4b</string>",
		"<key>MESHCLAW_SIGNAL_VISION_BASE_URL</key>",
		"<string>http://g4:11434/v1</string>",
		"<key>MESHCLAW_SIGNAL_VISION_API_KEY</key>",
		"<string>ollama</string>",
	} {
		if !strings.Contains(plist, want) {
			t.Fatalf("plist missing %q:\n%s", want, plist)
		}
	}
}

func TestSignalDispatcherPlistEnablesMemoryIntentModelByDefault(t *testing.T) {
	opts := defaultSignalDispatcherOptions()
	plist := signalDispatcherPlist(opts)
	for _, want := range []string{
		"<key>MESHCLAW_ASSISTANT_MEMORY_INTENT_BASE_URL</key>",
		"<string>http://localhost:11434/v1</string>",
		"<key>MESHCLAW_ASSISTANT_MEMORY_INTENT_MODEL</key>",
		"<string>1</string>",
		"<key>MESHCLAW_ASSISTANT_MEMORY_INTENT_MODEL_NAME</key>",
		"<string>gemma3:4b</string>",
		"<key>MESHCLAW_ASSISTANT_MEMORY_INTENT_TIMEOUT_MS</key>",
		"<string>12000</string>",
		"<key>MESHCLAW_SHOPPING_BROWSER_DISABLE</key>",
		"<string>1</string>",
		"<key>MESHCLAW_SHOPPING_SEARCH_DISABLE</key>",
		"<string>1</string>",
		"<key>MESHCLAW_SIGNAL_SINGLE_ACCOUNT_CONSOLE</key>",
		"<string>0</string>",
	} {
		if !strings.Contains(plist, want) {
			t.Fatalf("plist missing %q:\n%s", want, plist)
		}
	}
}

func TestSignalDispatcherPlistKeepsMemoryIntentOverrides(t *testing.T) {
	t.Setenv("MESHCLAW_ASSISTANT_MEMORY_INTENT_BASE_URL", "http://g4:11434/v1")
	t.Setenv("MESHCLAW_ASSISTANT_MEMORY_INTENT_MODEL", "0")
	t.Setenv("MESHCLAW_ASSISTANT_MEMORY_INTENT_MODEL_NAME", "gpt-oss:20b")
	t.Setenv("MESHCLAW_ASSISTANT_MEMORY_INTENT_TIMEOUT_MS", "900")
	t.Setenv("MESHCLAW_ASSISTANT_MEMORY_INTENT_API_KEY", "ollama")
	t.Setenv("MESHCLAW_MATRIX_AI_CONFIG", "/Users/argos/.meshclaw/custom-matrix-ai.json")

	opts := defaultSignalDispatcherOptions()
	plist := signalDispatcherPlist(opts)
	for _, want := range []string{
		"<key>MESHCLAW_ASSISTANT_MEMORY_INTENT_BASE_URL</key>",
		"<string>http://g4:11434/v1</string>",
		"<key>MESHCLAW_ASSISTANT_MEMORY_INTENT_MODEL</key>",
		"<string>0</string>",
		"<key>MESHCLAW_ASSISTANT_MEMORY_INTENT_MODEL_NAME</key>",
		"<string>gpt-oss:20b</string>",
		"<key>MESHCLAW_ASSISTANT_MEMORY_INTENT_TIMEOUT_MS</key>",
		"<string>900</string>",
		"<key>MESHCLAW_ASSISTANT_MEMORY_INTENT_API_KEY</key>",
		"<string>ollama</string>",
		"<key>MESHCLAW_MATRIX_AI_CONFIG</key>",
		"<string>/Users/argos/.meshclaw/custom-matrix-ai.json</string>",
	} {
		if !strings.Contains(plist, want) {
			t.Fatalf("plist missing %q:\n%s", want, plist)
		}
	}
}

func TestScheduleRunnerPlistRunsScheduleDue(t *testing.T) {
	opts := defaultScheduleRunnerOptions()
	opts.IntervalSeconds = 900
	plist := scheduleRunnerPlist(opts)
	for _, want := range []string{
		"<string>schedule</string>",
		"<string>run-due</string>",
		"<string>--execute</string>",
		"<key>StartInterval</key>",
		"<integer>900</integer>",
	} {
		if !strings.Contains(plist, want) {
			t.Fatalf("schedule runner plist missing %q:\n%s", want, plist)
		}
	}
}

func TestArgosSignalVisionBackendDoctorReportsConfiguredBackend(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"gemma3:4b"}]}`))
	}))
	defer server.Close()
	t.Setenv("MESHCLAW_SIGNAL_VISION_MODEL", "gemma3:4b")
	t.Setenv("MESHCLAW_SIGNAL_VISION_BASE_URL", server.URL)
	t.Setenv("MESHCLAW_SIGNAL_VISION_API_KEY", "test-key")

	report := argosSignalVisionBackendDoctor()
	if !report.OK || !report.Configured || !report.Reachable {
		t.Fatalf("report=%#v", report)
	}
	if report.Model != "gemma3:4b" || report.Endpoint != server.URL+"/models" {
		t.Fatalf("report=%#v", report)
	}
}

func TestArgosSignalVisionBackendDoctorExplainsFallbackWhenUnconfigured(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_SIGNAL_VISION_MODEL", "")
	t.Setenv("MESHCLAW_SIGNAL_VISION_BASE_URL", "")

	report := argosSignalVisionBackendDoctor()
	if report.OK || report.Configured || !strings.Contains(report.Problem, "OCR fallback") {
		t.Fatalf("report=%#v", report)
	}
}

func TestArgosSignalVisionBackendDoctorReadsDispatcherPlist(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			t.Fatalf("path=%s", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"data":[{"id":"gemma3:4b"}]}`))
	}))
	defer server.Close()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_SIGNAL_VISION_MODEL", "")
	t.Setenv("MESHCLAW_SIGNAL_VISION_BASE_URL", "")
	opts := defaultSignalDispatcherOptions()
	if err := os.MkdirAll(filepath.Dir(opts.PlistPath), 0700); err != nil {
		t.Fatal(err)
	}
	plist := strings.ReplaceAll(signalDispatcherPlist(opts), "<key>PATH</key>\n    <string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>", strings.Join([]string{
		"<key>MESHCLAW_SIGNAL_VISION_BASE_URL</key>",
		"    <string>" + server.URL + "</string>",
		"    <key>MESHCLAW_SIGNAL_VISION_MODEL</key>",
		"    <string>gemma3:4b</string>",
		"    <key>PATH</key>",
		"    <string>/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin</string>",
	}, "\n"))
	if err := os.WriteFile(opts.PlistPath, []byte(plist), 0600); err != nil {
		t.Fatal(err)
	}

	report := argosSignalVisionBackendDoctor()
	if !report.OK || report.Source != "signal-dispatcher-plist" || report.Model != "gemma3:4b" || report.BaseURL != server.URL {
		t.Fatalf("report=%#v", report)
	}
}
