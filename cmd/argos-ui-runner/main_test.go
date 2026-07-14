package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestClickValueReadsRunnerConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".meshclaw")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "argos-ui-runner.json"), []byte(`{"signal_start_click":"10,20","signal_hangup_click":"30,40"}`), 0600); err != nil {
		t.Fatal(err)
	}

	start, source := clickValue("ARGOS_SIGNAL_START_CLICK")
	if start != "10,20" || source == "" {
		t.Fatalf("start=%q source=%q", start, source)
	}
	click, err := clickFromEnv("ARGOS_SIGNAL_HANGUP_CLICK")
	if err != nil {
		t.Fatal(err)
	}
	if click.X != 30 || click.Y != 40 {
		t.Fatalf("click=%+v", click)
	}
}

func TestClickValueEnvOverridesRunnerConfig(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ARGOS_SIGNAL_START_CLICK", "1,2")
	value, source := clickValue("ARGOS_SIGNAL_START_CLICK")
	if value != "1,2" || source != "ARGOS_SIGNAL_START_CLICK" {
		t.Fatalf("value=%q source=%q", value, source)
	}
}
