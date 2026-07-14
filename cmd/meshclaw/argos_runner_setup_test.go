package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSetupArgosRunnerWritesConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Fatalf("path=%q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	report, err := setupArgosRunner([]string{
		"--signal-start-click", "10,20",
		"--signal-hangup-click", "30,40",
		"--ui-runner", server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !report.OK || !report.WroteConfig || !report.RunnerOK {
		t.Fatalf("report=%#v", report)
	}
	data, err := os.ReadFile(filepath.Join(home, ".meshclaw", "argos-ui-runner.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]string
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["signal_start_click"] != "10,20" || cfg["signal_hangup_click"] != "30,40" {
		t.Fatalf("cfg=%#v", cfg)
	}
}
