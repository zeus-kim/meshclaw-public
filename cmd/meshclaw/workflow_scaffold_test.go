package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/meshclaw/meshclaw/internal/runtimeflow"
)

func TestScaffoldWorkflowCreatesValidDefinition(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom-ops.json")
	result, err := scaffoldWorkflow([]string{"Custom Ops!", "--output", path}, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Workflow != "custom-ops" {
		t.Fatalf("workflow=%q want custom-ops", result.Workflow)
	}
	if !result.Created || result.Path != path {
		t.Fatalf("unexpected scaffold result: %#v", result)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("workflow file missing: %v", err)
	}
	validation, err := runtimeflow.Validate(path)
	if err != nil {
		t.Fatal(err)
	}
	if !validation.Valid {
		t.Fatalf("scaffold validation failed: %#v", validation)
	}
}

func TestScaffoldWorkflowRefusesOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "custom-ops.json")
	if _, err := scaffoldWorkflow([]string{"custom-ops", "--output", path}, false); err != nil {
		t.Fatal(err)
	}
	if _, err := scaffoldWorkflow([]string{"custom-ops", "--output", path}, false); err == nil {
		t.Fatal("expected overwrite refusal")
	}
	if _, err := scaffoldWorkflow([]string{"custom-ops", "--output", path, "--force"}, false); err != nil {
		t.Fatalf("force overwrite failed: %v", err)
	}
}

func TestScaffoldWorkflowHelpDoesNotCreateWorkflow(t *testing.T) {
	if _, err := scaffoldWorkflow([]string{"--help"}, false); err == nil {
		t.Fatal("expected usage error")
	}
}

func TestMCPWorkflowScaffold(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp-custom.json")
	result, err := callMCPTool("meshclaw_workflow_scaffold", map[string]interface{}{
		"name":   "MCP Custom",
		"output": path,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := result.(workflowScaffoldResult)
	if !ok {
		t.Fatalf("payload=%T %#v", result, result)
	}
	if payload.Workflow != "mcp-custom" || !payload.Validation.Valid {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("workflow file missing: %v", err)
	}
}
