package runtimeflow

import (
	"testing"
	"time"
)

func TestWorkflowTimeoutDefaults(t *testing.T) {
	t.Setenv("MESHCLAW_WORKFLOW_LOCAL_TIMEOUT_SECONDS", "")
	t.Setenv("MESHCLAW_WORKFLOW_REMOTE_TIMEOUT_SECONDS", "")

	if got := workflowLocalTimeout(); got != 90*time.Second {
		t.Fatalf("local timeout = %s", got)
	}
	if got := workflowRemoteTimeout(); got != 15*time.Second {
		t.Fatalf("remote timeout = %s", got)
	}
}

func TestWorkflowTimeoutEnvOverride(t *testing.T) {
	t.Setenv("MESHCLAW_WORKFLOW_LOCAL_TIMEOUT_SECONDS", "7")
	t.Setenv("MESHCLAW_WORKFLOW_REMOTE_TIMEOUT_SECONDS", "3")

	if got := workflowLocalTimeout(); got != 7*time.Second {
		t.Fatalf("local timeout = %s", got)
	}
	if got := workflowRemoteTimeout(); got != 3*time.Second {
		t.Fatalf("remote timeout = %s", got)
	}
}

func TestStepTimeoutOverridesDefault(t *testing.T) {
	step := StepSpec{TimeoutSeconds: 2}
	if got := stepTimeout(step, 90*time.Second); got != 2*time.Second {
		t.Fatalf("step timeout = %s", got)
	}
	if got := stepTimeout(StepSpec{}, 90*time.Second); got != 90*time.Second {
		t.Fatalf("fallback timeout = %s", got)
	}
}

func TestManualExecutePreservesStepNote(t *testing.T) {
	step := StepSpec{
		ID:         "why",
		Title:      "Explain runtime boundary",
		Node:       "macbook",
		Transport:  "manual",
		Action:     "read_state",
		Resource:   "workflow",
		DryRunNote: "Codex reasons; MeshClaw records policy and evidence.",
	}

	result := runStep("demo", Execute, step, ApprovalRecord{}, "")
	if !result.Success || !result.Skipped {
		t.Fatalf("manual step result = %#v", result)
	}
	if result.SkipReason != step.DryRunNote || result.Stdout != step.DryRunNote {
		t.Fatalf("manual note not preserved: %#v", result)
	}
}
