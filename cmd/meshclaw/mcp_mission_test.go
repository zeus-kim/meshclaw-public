package main

import "testing"

func TestMCPMissionWriteToolsRegistered(t *testing.T) {
	tools := map[string]mcpTool{}
	for _, tool := range allMCPTools() {
		tools[tool.Name] = tool
	}
	for _, name := range []string{"mission_update", "task_add", "task_complete", "artifact_add"} {
		if _, ok := tools[name]; !ok {
			t.Fatalf("%s not registered", name)
		}
	}
	for _, name := range []string{"task_add", "task_complete", "artifact_add"} {
		if len(tools[name].InputSchema.Required) == 0 {
			t.Fatalf("%s should declare required arguments", name)
		}
	}
}

func TestVisibleMCPToolsHidesMissionWritesWhenNotCanonical(t *testing.T) {
	tools := []mcpTool{
		{Name: "mission_get"},
		{Name: "mission_update"},
		{Name: "task_add"},
		{Name: "task_complete"},
		{Name: "artifact_add"},
		{Name: "meshclaw_argos_research"},
	}

	filtered := map[string]bool{}
	for _, tool := range visibleMCPTools(tools, false) {
		filtered[tool.Name] = true
	}
	for _, name := range []string{"mission_update", "task_add", "task_complete", "artifact_add"} {
		if filtered[name] {
			t.Fatalf("%s should be hidden outside the MacBook canonical Core host", name)
		}
	}
	for _, name := range []string{"mission_get", "meshclaw_argos_research"} {
		if !filtered[name] {
			t.Fatalf("%s should remain visible", name)
		}
	}
	if got := len(visibleMCPTools(tools, true)); got != len(tools) {
		t.Fatalf("canonical host should expose all tools, got %d want %d", got, len(tools))
	}
}
