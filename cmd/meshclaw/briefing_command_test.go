package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestBriefingForwardArgsPreservesOptionsAndJSON(t *testing.T) {
	got := briefingForwardArgs(true, "morning", []string{"--location", "Seoul", "--target", "argos-briefing"})
	want := []string{"morning", "--location", "Seoul", "--target", "argos-briefing", "--json"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("briefingForwardArgs() = %#v, want %#v", got, want)
	}
}

func TestBriefingCommandUsageErrors(t *testing.T) {
	if err := briefingCommand(nil); err == nil {
		t.Fatal("briefingCommand(nil) expected usage error")
	}
	if err := briefingCommand([]string{"unknown"}); err == nil {
		t.Fatal("briefingCommand(unknown) expected usage error")
	}
}

func TestBriefingHelpCommands(t *testing.T) {
	if err := briefingCommand([]string{"--help"}); err != nil {
		t.Fatalf("briefingCommand(--help) = %v", err)
	}
	if err := runLocalAIBriefing([]string{"--help"}); err != nil {
		t.Fatalf("runLocalAIBriefing(--help) = %v", err)
	}
	if err := daemonCommand([]string{"install", "local-ai-briefing", "--help"}); err != nil {
		t.Fatalf("daemonCommand(local-ai-briefing --help) = %v", err)
	}
}

func TestLocalAIBriefingScriptPathOverride(t *testing.T) {
	t.Setenv("LOCAL_AI_BRIEFING_SCRIPT", "/tmp/custom-briefing.py")
	if got := localAIBriefingScriptPath(); got != "/tmp/custom-briefing.py" {
		t.Fatalf("localAIBriefingScriptPath() = %q", got)
	}
}

func TestLocalAIBriefingScriptPathDefault(t *testing.T) {
	t.Setenv("LOCAL_AI_BRIEFING_SCRIPT", "")
	got := localAIBriefingScriptPath()
	home, _ := os.UserHomeDir()
	if home != "" && !strings.HasPrefix(got, home) {
		t.Fatalf("localAIBriefingScriptPath() = %q, want under home %q", got, home)
	}
	if !strings.HasSuffix(got, ".meshclaw/bin/local_ai_daily_briefing.py") {
		t.Fatalf("localAIBriefingScriptPath() = %q, want local_ai_daily_briefing.py", got)
	}
}

func TestRunLocalAIBriefingCheckOnly(t *testing.T) {
	script := filepath.Join(t.TempDir(), "briefing.py")
	if err := os.WriteFile(script, []byte("raise SystemExit('should not run')\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOCAL_AI_BRIEFING_SCRIPT", script)
	if err := runLocalAIBriefing([]string{"--check"}); err != nil {
		t.Fatalf("runLocalAIBriefing(--check) = %v", err)
	}
}

func TestRunLocalAIBriefingNoSendSetsEnvironment(t *testing.T) {
	script := filepath.Join(t.TempDir(), "briefing.py")
	if err := os.WriteFile(script, []byte("import os, sys\nok = os.environ.get('BRIEF_SEND') == '0' and os.environ.get('BRIEF_PRINT_REPORT') == '0' and os.environ.get('BRIEF_SIGNAL_SHOW_PATHS') == '0'\nsys.exit(0 if ok else 42)\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LOCAL_AI_BRIEFING_SCRIPT", script)
	t.Setenv("BRIEF_SEND", "1")
	if err := runLocalAIBriefing([]string{"--no-send"}); err != nil {
		t.Fatalf("runLocalAIBriefing(--no-send) = %v", err)
	}
}

func TestParseLocalAIBriefingDaemonOptions(t *testing.T) {
	opts, err := parseLocalAIBriefingDaemonOptions([]string{
		"--minute", "12",
		"--target", "argos-briefing",
		"--model", "gpt-oss:20b",
		"--script", "/tmp/brief.py",
		"--plist-path", "/tmp/staged.plist",
		"--log-path", "/tmp/staged.log",
		"--error-log-path", "/tmp/staged.err.log",
		"--workdir", "/tmp/briefings",
		"--no-send",
	})
	if err != nil {
		t.Fatal(err)
	}
	if opts.CalendarMinute != 12 || opts.Target != "argos-briefing" || opts.Model != "gpt-oss:20b" || opts.ScriptPath != "/tmp/brief.py" || opts.Execute {
		t.Fatalf("parseLocalAIBriefingDaemonOptions() = %#v", opts)
	}
	if opts.PlistPath != "/tmp/staged.plist" || opts.LogPath != "/tmp/staged.log" || opts.ErrorLogPath != "/tmp/staged.err.log" || opts.WorkingDirectory != "/tmp/briefings" {
		t.Fatalf("parseLocalAIBriefingDaemonOptions() = %#v", opts)
	}
}

func TestInstallLocalAIBriefingCanWriteStagedPlist(t *testing.T) {
	dir := t.TempDir()
	opts := defaultLocalAIBriefingOptions()
	opts.Binary = "/Users/example/bin/meshclaw"
	opts.PlistPath = filepath.Join(dir, "staged.plist")
	opts.LogPath = filepath.Join(dir, "staged.log")
	opts.ErrorLogPath = filepath.Join(dir, "staged.err.log")
	opts.WorkingDirectory = filepath.Join(dir, "briefings")
	opts.Execute = false

	result, err := installLocalAIBriefing(opts)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Installed {
		t.Fatalf("installLocalAIBriefing() installed = false")
	}
	if !result.Changed {
		t.Fatalf("first installLocalAIBriefing() changed = false")
	}
	content, err := os.ReadFile(opts.PlistPath)
	if err != nil {
		t.Fatal(err)
	}
	plist := string(content)
	for _, want := range []string{
		"<string>/Users/example/bin/meshclaw</string>",
		"<string>briefing</string>",
		"<string>local-ai</string>",
		"<key>BRIEF_SEND</key>",
		"<string>0</string>",
		"<key>BRIEF_ATTACH_MARKDOWN</key>",
		"<string>1</string>",
		"<key>BRIEF_ATTACH_HTML</key>",
		"<string>0</string>",
		"<key>BRIEF_PRINT_REPORT</key>",
		"<string>0</string>",
		"<key>BRIEF_SIGNAL_SHOW_PATHS</key>",
		"<string>0</string>",
	} {
		if !strings.Contains(plist, want) {
			t.Fatalf("staged plist missing %q:\n%s", want, plist)
		}
	}
	second, err := installLocalAIBriefing(opts)
	if err != nil {
		t.Fatal(err)
	}
	if second.Changed {
		t.Fatalf("second installLocalAIBriefing() changed = true for identical plist")
	}
}

func TestLocalAIBriefingPlistUsesMeshclawCommandSurface(t *testing.T) {
	opts := defaultLocalAIBriefingOptions()
	opts.Binary = "/Users/example/bin/meshclaw"
	opts.CalendarMinute = 7
	opts.Target = "argos-briefing"
	opts.Model = "gpt-oss:20b"
	opts.Execute = true
	plist := localAIBriefingPlist(opts)
	for _, want := range []string{
		"<string>/Users/example/bin/meshclaw</string>",
		"<string>briefing</string>",
		"<string>local-ai</string>",
		"<integer>7</integer>",
		"<key>BRIEF_TARGET</key>",
		"<string>argos-briefing</string>",
		"<key>BRIEF_ATTACH_MARKDOWN</key>",
		"<string>1</string>",
		"<key>BRIEF_ATTACH_HTML</key>",
		"<string>0</string>",
		"<key>BRIEF_PRINT_REPORT</key>",
		"<string>0</string>",
		"<key>BRIEF_SIGNAL_SHOW_PATHS</key>",
		"<string>0</string>",
	} {
		if !strings.Contains(plist, want) {
			t.Fatalf("localAIBriefingPlist() missing %q:\n%s", want, plist)
		}
	}
}
