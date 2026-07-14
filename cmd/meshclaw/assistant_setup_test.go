package main

import "testing"

func TestAssistantSetupReportIncludesFirstSuccessTasks(t *testing.T) {
	report := buildAssistantSetupReport()
	if report.Kind != "meshclaw_assistant_setup" {
		t.Fatalf("kind=%q", report.Kind)
	}
	if report.Profile != "one-machine-assistant" {
		t.Fatalf("profile=%q", report.Profile)
	}
	if len(report.FirstSuccessTasks) < 5 {
		t.Fatalf("first success tasks=%#v", report.FirstSuccessTasks)
	}
	seen := map[string]bool{}
	for _, task := range report.FirstSuccessTasks {
		seen[task.ID] = true
		if task.Request == "" || task.Expected == "" || task.Surface == "" {
			t.Fatalf("incomplete task=%#v", task)
		}
	}
	for _, id := range []string{"briefing", "fleet", "research", "presentation", "reminder"} {
		if !seen[id] {
			t.Fatalf("missing first success task %s in %#v", id, report.FirstSuccessTasks)
		}
	}
	if len(report.InstallCommands) == 0 {
		t.Fatal("missing install commands")
	}
	if report.Catalog == nil || report.Catalog["selected_profile"] == nil {
		t.Fatalf("missing one-machine catalog: %#v", report.Catalog)
	}
}
