package runtimeflow

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meshclaw/meshclaw/internal/guardvault"
	"github.com/meshclaw/meshclaw/internal/policy"
)

func TestFleetHealthDemoIsBuiltIn(t *testing.T) {
	dir := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldwd)
	})
	t.Setenv("HOME", dir)
	t.Setenv("MESHCLAW_WORKFLOW_DIR", filepath.Join(dir, "missing-workflows"))
	if !IsKnown("fleet-health-demo") {
		t.Fatal("fleet-health-demo should be available without external workflow files")
	}
	validation, err := Validate("fleet-health-demo")
	if err != nil {
		t.Fatal(err)
	}
	if !validation.Valid || validation.StepCount == 0 {
		t.Fatalf("invalid built-in fleet-health-demo: %#v", validation)
	}
}

func TestInspectEmailWorkflowExposesApprovalGatesAndCapabilities(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MESHCLAW_INVENTORY_FILE", filepath.Join(dir, "inventory.json"))
	t.Setenv("MESHCLAW_CAPABILITY_FILE", filepath.Join(dir, "missing-capabilities.json"))
	t.Setenv("MESHCLAW_GUARD_VAULT_DIR", filepath.Join(dir, "guard-vault"))
	writeJSON(t, filepath.Join(dir, "inventory.json"), map[string]interface{}{
		"version": 1,
		"nodes": []map[string]interface{}{
			{"name": "c1", "role": "mail-server", "tags": []string{"linux", "mail"}, "online": true},
			{"name": "macmini", "role": "desktop-worker", "tags": []string{"macos", "client"}, "online": true},
		},
	})

	inspection, err := Inspect("email-orchestration-demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(inspection.ApprovalGates) != 7 {
		t.Fatalf("approval gates = %d, want 7", len(inspection.ApprovalGates))
	}
	if len(inspection.CapabilityMatches["macbook"]) == 0 {
		t.Fatalf("expected macbook controller capability: %#v", inspection.CapabilityMatches)
	}
	if len(inspection.CapabilityMatches["c1"]) == 0 {
		t.Fatalf("expected c1 capability match: %#v", inspection.CapabilityMatches)
	}
	if len(inspection.CapabilityHints) == 0 {
		t.Fatalf("expected step capability hints")
	}
	var foundMailHint bool
	for _, ids := range inspection.CapabilityHints {
		for _, id := range ids {
			if id == "c1-mail" {
				foundMailHint = true
			}
		}
	}
	if !foundMailHint {
		t.Fatalf("expected c1-mail recommendation in capability hints: %#v", inspection.CapabilityHints)
	}
	if inspection.Steps[0].Adapter.Name == "" || inspection.Steps[0].Adapter.Kind == "" {
		t.Fatalf("expected adapter metadata in inspection: %#v", inspection.Steps[0])
	}
	var foundVaultHandle bool
	for _, step := range inspection.Steps {
		for _, handle := range step.Step.VaultHandles {
			if handle == "vault://meshclaw/cloudflare/dns-token" {
				foundVaultHandle = true
			}
		}
	}
	if !foundVaultHandle {
		t.Fatalf("expected Cloudflare vault handle in email workflow inspection")
	}
	var foundVaultPreflight bool
	for _, step := range inspection.Steps {
		for _, check := range step.VaultChecks {
			if check.Handle == "vault://meshclaw/cloudflare/dns-token" && !check.Exists {
				foundVaultPreflight = true
			}
		}
	}
	if !foundVaultPreflight {
		t.Fatalf("expected missing Cloudflare vault preflight in email workflow inspection")
	}
	for _, step := range inspection.Steps {
		for _, check := range step.VaultChecks {
			if check.Handle == "vault://meshclaw/cloudflare/dns-token" && !strings.Contains(check.ImportCLI, "guard-vault-put") {
				t.Fatalf("missing import CLI for vault preflight: %#v", check)
			}
		}
	}
}

func TestAdapterRegistrySeparatesExecutableAndEvidenceOnlyAdapters(t *testing.T) {
	if !stepExecutable(StepSpec{Transport: "vssh", Command: "uptime"}) {
		t.Fatalf("vssh command should be executable")
	}
	if stepExecutable(StepSpec{Transport: "mail", Action: "email_send", Resource: "email"}) {
		t.Fatalf("mail placeholder should not be executable")
	}
	if approvalCanExecute(StepSpec{Transport: "policy", Action: "email_send", Resource: "email"}) {
		t.Fatalf("policy adapter should not execute after approval")
	}
}

func TestFileBackedWorkflowDefinition(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MESHCLAW_WORKFLOW_DIR", dir)
	writeJSON(t, filepath.Join(dir, "generic-demo.json"), WorkflowDefinition{
		Name:        "generic-demo",
		Description: "Generic workflow loaded from file",
		Steps: []StepSpec{
			{
				ID:         "overview",
				Title:      "Collect overview",
				Transport:  "manual",
				Action:     "read_state",
				Resource:   "fleet",
				DryRunNote: "file workflow step",
			},
		},
	})

	if !IsKnown("generic-demo") {
		t.Fatalf("file workflow should be known")
	}
	steps, err := workflowSteps("generic-demo")
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 1 || steps[0].ID != "overview" {
		t.Fatalf("steps = %#v", steps)
	}
	if workflowDescription("generic-demo") != "Generic workflow loaded from file" {
		t.Fatalf("description = %q", workflowDescription("generic-demo"))
	}
}

func TestValidateFileBackedWorkflow(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "valid-demo.json")
	writeJSON(t, path, WorkflowDefinition{
		Name:        "valid-demo",
		Description: "Valid workflow",
		Steps: []StepSpec{
			{
				ID:        "first",
				Title:     "First",
				Transport: "local",
				Command:   "printf ok",
				Action:    "read_state",
				Resource:  "local-workspace",
			},
			{
				ID:          "fallback",
				Title:       "Fallback",
				Transport:   "local",
				Command:     "printf fallback",
				Action:      "read_state",
				Resource:    "local-workspace",
				FallbackFor: []string{"first"},
			},
		},
	})

	validation, err := Validate(path)
	if err != nil {
		t.Fatal(err)
	}
	if !validation.Valid {
		t.Fatalf("validation invalid: %#v", validation)
	}
	if validation.Source != path || validation.StepCount != 2 {
		t.Fatalf("validation metadata: %#v", validation)
	}
}

func TestValidateReportsStructuralErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "invalid-demo.json")
	writeJSON(t, path, WorkflowDefinition{
		Name: "invalid-demo",
		Steps: []StepSpec{
			{
				ID:        "dup",
				Transport: "local",
				Action:    "read_state",
				Resource:  "local-workspace",
			},
			{
				ID:        "dup",
				Transport: "unknown-adapter",
				Command:   "printf nope",
				Action:    "run_command",
				Resource:  "server",
				DependsOn: []string{"missing"},
				SecretEnv: map[string]string{"1TOKEN": "not-a-handle"},
			},
		},
	})

	validation, err := Validate(path)
	if err != nil {
		t.Fatal(err)
	}
	if validation.Valid {
		t.Fatalf("validation valid, want invalid")
	}
	messages := validationMessages(validation.Errors)
	for _, want := range []string{"duplicate step id", "unknown adapter", "unknown dependency", "environment variable name is invalid", "handle must start"} {
		if !strings.Contains(messages, want) {
			t.Fatalf("missing %q in errors: %s", want, messages)
		}
	}
	if len(validation.NextActions) == 0 {
		t.Fatalf("missing next actions: %#v", validation)
	}
}

func TestResumeClassifiesDryRunApprovalAndExecutableSteps(t *testing.T) {
	dir := t.TempDir()
	result := Result{
		Success:     true,
		Workflow:    "email-orchestration-demo",
		Mode:        DryRun,
		GeneratedAt: timeNowForTest(),
		Summary:     Summary{Total: 2, Succeeded: 2, Skipped: 2, ApprovalRequired: 1},
		Steps: []ExecutionResult{
			{
				Success:        true,
				Workflow:       "email-orchestration-demo",
				Step:           "mox-live",
				Title:          "Check Mox service and mail ports",
				Node:           "c1",
				Transport:      "vssh",
				Command:        "systemctl is-active mox",
				Skipped:        true,
				PolicyDecision: "allow",
			},
			{
				Success:          true,
				Workflow:         "email-orchestration-demo",
				Step:             "send-approval",
				Title:            "Gate real email send behind approval",
				Transport:        "policy",
				Skipped:          true,
				ApprovalRequired: true,
				PolicyDecision:   "require_approval",
				PolicyReason:     "sending real email requires approval and evidence",
			},
		},
	}
	writeJSON(t, filepath.Join(dir, "execution.json"), result)

	plan, err := Resume(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Items) != 2 {
		t.Fatalf("items = %d, want 2: %#v", len(plan.Items), plan.Items)
	}
	if plan.Items[0].Status != "ready_for_execute" {
		t.Fatalf("first status = %q", plan.Items[0].Status)
	}
	if plan.Items[0].ExecuteCLI == "" {
		t.Fatalf("first item missing execute command: %#v", plan.Items[0])
	}
	if plan.Items[1].Status != "approval_pending" {
		t.Fatalf("second status = %q", plan.Items[1].Status)
	}
	if plan.Items[1].ApprovalCLI == "" || plan.Items[1].ApprovalMCP == "" {
		t.Fatalf("approval item missing approval hints: %#v", plan.Items[1])
	}
	if plan.Items[1].ApprovalMCPCall == nil || plan.Items[1].ExecuteCLI == "" {
		t.Fatalf("approval item missing structured follow-up hints: %#v", plan.Items[1])
	}
	if _, err := os.Stat(filepath.Join(dir, "resume-plan.json")); err != nil {
		t.Fatal(err)
	}
}

func TestResumeIncludesApprovalMetadata(t *testing.T) {
	dir := t.TempDir()
	result := Result{
		Success:     true,
		Workflow:    "email-orchestration-demo",
		Mode:        DryRun,
		GeneratedAt: timeNowForTest(),
		Steps: []ExecutionResult{
			{
				Success:          true,
				Workflow:         "email-orchestration-demo",
				Step:             "send-approval",
				Title:            "Gate real email send behind approval",
				Transport:        "policy",
				Action:           "email_send",
				Resource:         "email",
				Skipped:          true,
				ApprovalRequired: true,
				PolicyDecision:   "require_approval",
				PolicyReason:     "sending real email requires approval and evidence",
			},
		},
	}
	writeJSON(t, filepath.Join(dir, "execution.json"), result)
	record := ApprovalRecord{
		Time:     timeNowForTest(),
		Actor:    "operator",
		Workflow: "email-orchestration-demo",
		Step:     "send-approval",
		Action:   "email_send",
		Resource: "email",
		Reason:   "approved",
		Bundle:   dir,
		Source:   "cli",
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "approvals.jsonl"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}

	plan, err := Resume(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(plan.Items))
	}
	item := plan.Items[0]
	if item.Status != "approved_ready" {
		t.Fatalf("status = %q", item.Status)
	}
	if !item.Approved || item.ApprovalActor != "operator" || item.ApprovalSource != "cli" {
		t.Fatalf("approval metadata = %#v", item)
	}
	if item.Action != "email_send" || item.Resource != "email" {
		t.Fatalf("action/resource = %s/%s", item.Action, item.Resource)
	}
	if item.ExecuteCLI == "" {
		t.Fatalf("approved item missing execute command: %#v", item)
	}
}

func TestResumeLinksDegradedWorkerToNodeRepairPlan(t *testing.T) {
	dir := t.TempDir()
	result := Result{
		Success:     true,
		Workflow:    "meshclaw-ops-orchestration-demo",
		Mode:        Execute,
		GeneratedAt: timeNowForTest(),
		Steps: []ExecutionResult{
			{
				Success:        true,
				Workflow:       "meshclaw-ops-orchestration-demo",
				Step:           "g2-reliability-worker",
				Title:          "Assign g2 reliability reviewer",
				Node:           "macbook",
				Transport:      "local",
				PolicyDecision: "allow",
				Stdout:         `{"status":"degraded","worker":"g2","role":"reliability_review","retryable":true,"reason":"vssh_transport_timeout_observed"}`,
			},
		},
	}
	writeJSON(t, filepath.Join(dir, "execution.json"), result)

	plan, err := Resume(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(plan.Items))
	}
	item := plan.Items[0]
	if item.Status != "degraded_repair" || item.Node != "g2" {
		t.Fatalf("unexpected degraded item: %#v", item)
	}
	if item.RepairCLI == "" || item.RepairMCPCall == nil {
		t.Fatalf("missing repair hints: %#v", item)
	}
}

func TestRunStepRequiresApprovalBeforeExecution(t *testing.T) {
	step := StepSpec{
		ID:               "send-approval",
		Title:            "Gate real email send behind approval",
		Transport:        "policy",
		Action:           "email_send",
		Resource:         "email",
		ApprovalRequired: true,
	}

	result := runStep("email-orchestration-demo", Execute, step, ApprovalRecord{}, "")

	if !result.Success {
		t.Fatalf("success = false, error=%s", result.Error)
	}
	if !result.ApprovalRequired {
		t.Fatalf("approval_required = false")
	}
	if result.Approved {
		t.Fatalf("approved = true")
	}
	if !result.Skipped {
		t.Fatalf("skipped = false")
	}
	if result.SkipReason != "approval required before execution" {
		t.Fatalf("skip reason = %q", result.SkipReason)
	}
	if result.Status != "approval_pending" || result.NextAction == "" {
		t.Fatalf("classification = %q next=%q", result.Status, result.NextAction)
	}
	if result.Action != "email_send" || result.Resource != "email" {
		t.Fatalf("action/resource = %s/%s", result.Action, result.Resource)
	}
	if result.AdapterKind != "approval-gate" || result.AdapterExecutable {
		t.Fatalf("adapter metadata = %q executable=%t", result.AdapterKind, result.AdapterExecutable)
	}
}

func TestRunStepPreservesStrongApprovalMetadata(t *testing.T) {
	step := StepSpec{
		ID:               "delete-account",
		Title:            "Delete account",
		Transport:        "policy",
		Action:           "account_delete",
		Resource:         "account",
		ApprovalRequired: true,
		StrongApproval:   true,
	}

	result := runStep("account-workflow", Execute, step, ApprovalRecord{}, "")

	if !result.StrongApproval || !result.ApprovalRequired {
		t.Fatalf("approval metadata missing: %#v", result)
	}
}

func TestRunStepFailsWhenVaultHandleMissingInExecute(t *testing.T) {
	t.Setenv("MESHCLAW_GUARD_VAULT_DIR", t.TempDir())
	step := StepSpec{
		ID:           "cloudflare-token-policy",
		Title:        "Resolve Cloudflare token",
		Transport:    "policy",
		Action:       "provider_token_use",
		Resource:     "credential",
		VaultHandles: []string{"vault://meshclaw/cloudflare/dns-token"},
	}

	result := runStep("email-orchestration-demo", Execute, step, ApprovalRecord{}, "")

	if result.Success {
		t.Fatalf("success = true, want preflight failure")
	}
	if result.SkipReason != "vault handle preflight failed" {
		t.Fatalf("skip reason = %q", result.SkipReason)
	}
	if !strings.Contains(result.Error, "vault://meshclaw/cloudflare/dns-token") {
		t.Fatalf("missing handle not reported: %s", result.Error)
	}
	if len(result.VaultChecks) != 1 || result.VaultChecks[0].Exists {
		t.Fatalf("unexpected vault checks: %#v", result.VaultChecks)
	}
	if !strings.Contains(result.VaultChecks[0].NextAction, "Import the secret locally") {
		t.Fatalf("missing next action: %#v", result.VaultChecks[0])
	}
	if result.Status != "vault_missing" || result.FailureKind != "vault_missing" {
		t.Fatalf("classification = %q/%q", result.Status, result.FailureKind)
	}
}

func TestRunStepClassifiesPolicyDenied(t *testing.T) {
	step := StepSpec{
		ID:        "deny-danger",
		Title:     "Deny dangerous command",
		Transport: "local",
		Command:   "rm -rf /",
		Action:    "run_command",
		Resource:  "server",
	}

	result := runStep("test-workflow", Execute, step, ApprovalRecord{}, "")

	if result.Success {
		t.Fatalf("success = true, want denied")
	}
	if result.Status != "policy_denied" || result.FailureKind != "policy_denied" {
		t.Fatalf("classification = %q/%q", result.Status, result.FailureKind)
	}
	if !strings.Contains(result.NextAction, "do not rerun") {
		t.Fatalf("next action = %q", result.NextAction)
	}
}

func TestRunSelectedFailsWhenVaultPreflightFails(t *testing.T) {
	t.Setenv("MESHCLAW_GUARD_VAULT_DIR", t.TempDir())
	result, err := RunSelectedWithApprovals("email-orchestration-demo", Execute, "", []string{"cloudflare-token-policy"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Success {
		t.Fatalf("workflow success = true, want false")
	}
	if result.Summary.Failed != 1 {
		t.Fatalf("failed summary = %d, want 1", result.Summary.Failed)
	}
	if len(result.Steps) != 1 || result.Steps[0].SkipReason != "vault handle preflight failed" {
		t.Fatalf("unexpected steps: %#v", result.Steps)
	}
}

func TestResumeClassifiesMissingVaultHandle(t *testing.T) {
	dir := t.TempDir()
	result := Result{
		Success:     false,
		Workflow:    "email-orchestration-demo",
		Mode:        Execute,
		GeneratedAt: timeNowForTest(),
		Steps: []ExecutionResult{
			{
				Success:          false,
				Workflow:         "email-orchestration-demo",
				Step:             "cloudflare-token-policy",
				Title:            "Resolve Cloudflare token",
				Transport:        "policy",
				Action:           "provider_token_use",
				Resource:         "credential",
				ApprovalRequired: true,
				SkipReason:       "vault handle preflight failed",
				Error:            "missing required vault handle(s): vault://meshclaw/cloudflare/dns-token",
				VaultChecks: []VaultCheck{
					{
						Handle:     "vault://meshclaw/cloudflare/dns-token",
						Exists:     false,
						ImportCLI:  "printf '...' | meshclaw guard-vault-put 'cloudflare' 'dns-token' <kind> <description> --json",
						NextAction: "Import the secret locally with guard-vault-put before execute mode.",
					},
				},
			},
		},
	}
	writeJSON(t, filepath.Join(dir, "execution.json"), result)

	plan, err := Resume(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Items) != 1 {
		t.Fatalf("items=%d want 1", len(plan.Items))
	}
	item := plan.Items[0]
	if item.Status != "vault_missing" {
		t.Fatalf("status=%q want vault_missing", item.Status)
	}
	if len(item.VaultChecks) != 1 || !strings.Contains(item.VaultChecks[0].ImportCLI, "guard-vault-put") {
		t.Fatalf("missing vault repair hint: %#v", item)
	}
}

func TestRunStepRecordsApprovedNonExecutableStep(t *testing.T) {
	step := StepSpec{
		ID:               "send-approval",
		Title:            "Gate real email send behind approval",
		Transport:        "policy",
		Action:           "email_send",
		Resource:         "email",
		ApprovalRequired: true,
	}
	approvalTime := timeNowForTest()
	approval := ApprovalRecord{
		Time:   approvalTime,
		Actor:  "operator",
		Step:   "send-approval",
		Reason: "approved test mail",
		Source: "cli",
	}

	result := runStep("email-orchestration-demo", Execute, step, approval, "")

	if !result.Success {
		t.Fatalf("success = false, error=%s", result.Error)
	}
	if !result.Approved {
		t.Fatalf("approved = false")
	}
	if result.ApprovalActor != "operator" {
		t.Fatalf("approval actor = %q", result.ApprovalActor)
	}
	if result.ApprovalTime != approvalTime.Format(time.RFC3339) {
		t.Fatalf("approval time = %q", result.ApprovalTime)
	}
	if result.ApprovalReason != "approved test mail" || result.ApprovalSource != "cli" {
		t.Fatalf("approval metadata = %q/%q", result.ApprovalReason, result.ApprovalSource)
	}
	if !result.Skipped {
		t.Fatalf("skipped = false")
	}
	if !strings.Contains(result.SkipReason, "not executable") {
		t.Fatalf("skip reason = %q", result.SkipReason)
	}
	if !strings.Contains(result.AdapterReason, "Approval is recorded") {
		t.Fatalf("adapter reason = %q", result.AdapterReason)
	}
	if result.Status != "approved_not_executable" || result.NextAction == "" {
		t.Fatalf("classification = %q next=%q", result.Status, result.NextAction)
	}
}

func TestRunStepExecutesApprovedLocalStep(t *testing.T) {
	step := StepSpec{
		ID:               "approved-local",
		Title:            "Run approved local command",
		Transport:        "local",
		Command:          "printf ok",
		Action:           "account_configure",
		Resource:         "server",
		ApprovalRequired: true,
	}
	approval := ApprovalRecord{
		Time:   timeNowForTest(),
		Actor:  "operator",
		Step:   "approved-local",
		Reason: "local smoke test",
	}

	result := runStep("test-workflow", Execute, step, approval, "")

	if !result.Success {
		t.Fatalf("success = false, error=%s stderr=%s", result.Error, result.Stderr)
	}
	if result.Skipped {
		t.Fatalf("skipped = true: %s", result.SkipReason)
	}
	if !result.Approved {
		t.Fatalf("approved = false")
	}
	if result.Stdout != "ok" {
		t.Fatalf("stdout = %q", result.Stdout)
	}
	if result.AdapterKind != "local-exec" || !result.AdapterExecutable {
		t.Fatalf("adapter metadata = %q executable=%t", result.AdapterKind, result.AdapterExecutable)
	}
	if result.Status != "ok" || result.NextAction != "continue workflow" {
		t.Fatalf("classification = %q next=%q", result.Status, result.NextAction)
	}
}

func TestRunStepInjectsSecretEnvAndRedactsOutput(t *testing.T) {
	t.Setenv("MESHCLAW_GUARD_VAULT_DIR", t.TempDir())
	entry, err := guardvault.Put(guardvault.PutOptions{
		Scope:       "test",
		Name:        "token",
		Kind:        "api-token",
		Description: "runtimeflow secret env test",
		Value:       []byte("secret-token-123"),
	})
	if err != nil {
		t.Fatal(err)
	}
	step := StepSpec{
		ID:        "secret-local",
		Title:     "Run local command with secret env",
		Transport: "local",
		Command:   "printf 'token=%s' \"$TOKEN\"",
		Action:    "read_state",
		Resource:  "local-workspace",
		SecretEnv: map[string]string{"TOKEN": entry.Handle},
	}

	result := runStep("generic-secret-workflow", Execute, step, ApprovalRecord{}, "")

	if !result.Success {
		t.Fatalf("success = false, error=%s stderr=%s", result.Error, result.Stderr)
	}
	if strings.Contains(result.Stdout, "secret-token-123") {
		t.Fatalf("stdout leaked secret: %s", result.Stdout)
	}
	if !strings.Contains(result.Stdout, "[REDACTED") {
		t.Fatalf("stdout was not redacted: %s", result.Stdout)
	}
	if result.SecretEnv["TOKEN"] != entry.Handle {
		t.Fatalf("secret env metadata = %#v", result.SecretEnv)
	}
	if len(result.VaultHandles) != 1 || result.VaultHandles[0] != entry.Handle {
		t.Fatalf("secret_env handle was not included in vault handles: %#v", result.VaultHandles)
	}
	if len(result.VaultChecks) != 1 || !result.VaultChecks[0].Exists {
		t.Fatalf("vault checks = %#v", result.VaultChecks)
	}
}

func TestRunStepStoresLargeOutputArtifact(t *testing.T) {
	t.Setenv("MESHCLAW_WORKFLOW_INLINE_OUTPUT_BYTES", "24")
	dir := t.TempDir()
	step := StepSpec{
		ID:        "large-output",
		Title:     "Large output",
		Node:      "macbook",
		Transport: "local",
		Command:   "printf 'abcdefghijklmnopqrstuvwxyz0123456789'",
		Action:    "read_state",
		Resource:  "local-workspace",
	}

	result := runStep("test-workflow", Execute, step, ApprovalRecord{}, dir)

	if !result.Success {
		t.Fatalf("success = false, error=%s stderr=%s", result.Error, result.Stderr)
	}
	if !result.StdoutTruncated {
		t.Fatalf("expected stdout to be truncated: %#v", result)
	}
	if result.StdoutBytes <= len(result.Stdout) {
		t.Fatalf("stdout_bytes should record original length, result=%#v", result)
	}
	if result.OutputArtifact == "" {
		t.Fatalf("expected output artifact, result=%#v", result)
	}
	data, err := os.ReadFile(filepath.Join(dir, result.OutputArtifact))
	if err != nil {
		t.Fatalf("read output artifact: %v", err)
	}
	if !strings.Contains(string(data), "abcdefghijklmnopqrstuvwxyz0123456789") {
		t.Fatalf("artifact did not preserve full output: %s", string(data))
	}
}

func TestRunSelectedBlocksFailedDependency(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_WORKFLOW_DIR", filepath.Join(home, "workflows"))
	t.Setenv("MESHCLAW_CAPABILITY_FILE", filepath.Join(home, "missing-capabilities.json"))
	writeJSON(t, filepath.Join(home, "workflows", "dependency-demo.json"), WorkflowDefinition{
		Name:        "dependency-demo",
		Description: "Dependency gate test",
		Steps: []StepSpec{
			{
				ID:        "first",
				Title:     "Fail first",
				Transport: "local",
				Command:   "exit 42",
				Action:    "read_state",
				Resource:  "local-workspace",
			},
			{
				ID:        "second",
				Title:     "Blocked second",
				Transport: "local",
				Command:   "printf should-not-run",
				Action:    "read_state",
				Resource:  "local-workspace",
				DependsOn: []string{"first"},
			},
		},
	})

	result, err := RunSelectedWithApprovals("dependency-demo", Execute, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("steps=%d want 2", len(result.Steps))
	}
	if result.Steps[1].Status != "dependency_blocked" || result.Steps[1].FailureKind != "dependency_blocked" {
		t.Fatalf("second classification=%q/%q result=%#v", result.Steps[1].Status, result.Steps[1].FailureKind, result.Steps[1])
	}
	if strings.Contains(result.Steps[1].Stdout, "should-not-run") {
		t.Fatalf("blocked dependency executed: %#v", result.Steps[1])
	}
}

func TestRunSelectedExecutesFallbackAfterFailure(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_WORKFLOW_DIR", filepath.Join(home, "workflows"))
	t.Setenv("MESHCLAW_CAPABILITY_FILE", filepath.Join(home, "missing-capabilities.json"))
	writeJSON(t, filepath.Join(home, "workflows", "fallback-demo.json"), WorkflowDefinition{
		Name:        "fallback-demo",
		Description: "Fallback test",
		Steps: []StepSpec{
			{
				ID:        "primary",
				Title:     "Primary fails",
				Transport: "local",
				Command:   "exit 7",
				Action:    "read_state",
				Resource:  "local-workspace",
			},
			{
				ID:          "fallback",
				Title:       "Fallback succeeds",
				Transport:   "local",
				Command:     "printf fallback-ok",
				Action:      "read_state",
				Resource:    "local-workspace",
				FallbackFor: []string{"primary"},
			},
		},
	})

	result, err := RunSelectedWithApprovals("fallback-demo", Execute, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("steps=%d want 2", len(result.Steps))
	}
	if result.Steps[0].Success {
		t.Fatalf("primary unexpectedly succeeded: %#v", result.Steps[0])
	}
	fallback := result.Steps[1]
	if !fallback.Success || fallback.Status != "fallback_ok" || fallback.Stdout != "fallback-ok" {
		t.Fatalf("fallback result=%#v", fallback)
	}
	if len(fallback.FallbackFor) != 1 || fallback.FallbackFor[0] != "primary" {
		t.Fatalf("fallback_for not recorded: %#v", fallback.FallbackFor)
	}
	if result.Success {
		t.Fatalf("workflow should preserve original failure even when fallback succeeds")
	}
}

func TestRunSelectedSkipsUnneededFallback(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_WORKFLOW_DIR", filepath.Join(home, "workflows"))
	t.Setenv("MESHCLAW_CAPABILITY_FILE", filepath.Join(home, "missing-capabilities.json"))
	writeJSON(t, filepath.Join(home, "workflows", "fallback-skip-demo.json"), WorkflowDefinition{
		Name:        "fallback-skip-demo",
		Description: "Fallback skip test",
		Steps: []StepSpec{
			{
				ID:        "primary",
				Title:     "Primary succeeds",
				Transport: "local",
				Command:   "printf primary-ok",
				Action:    "read_state",
				Resource:  "local-workspace",
			},
			{
				ID:          "fallback",
				Title:       "Fallback skipped",
				Transport:   "local",
				Command:     "printf should-not-run",
				Action:      "read_state",
				Resource:    "local-workspace",
				FallbackFor: []string{"primary"},
			},
		},
	})

	result, err := RunSelectedWithApprovals("fallback-skip-demo", Execute, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Steps) != 2 {
		t.Fatalf("steps=%d want 2", len(result.Steps))
	}
	fallback := result.Steps[1]
	if !fallback.Success || !fallback.Skipped || fallback.Status != "fallback_not_needed" {
		t.Fatalf("fallback result=%#v", fallback)
	}
	if strings.Contains(fallback.Stdout, "should-not-run") {
		t.Fatalf("unneeded fallback executed: %#v", fallback)
	}
}

func TestRunStepRetriesLocalCommand(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker")
	step := StepSpec{
		ID:        "retry-local",
		Title:     "Retry local command",
		Transport: "local",
		Command:   "if [ ! -f " + shellQuote(marker) + " ]; then touch " + shellQuote(marker) + "; echo transient >&2; exit 7; fi; printf ok",
		Action:    "read_state",
		Resource:  "local-workspace",
		Retry: RetrySpec{
			MaxAttempts:      2,
			RetryOnExitCodes: []int{7},
		},
	}

	result := runStep("test-workflow", Execute, step, ApprovalRecord{}, "")

	if !result.Success {
		t.Fatalf("success=false result=%#v", result)
	}
	if result.Stdout != "ok" {
		t.Fatalf("stdout=%q", result.Stdout)
	}
	if len(result.Attempts) != 2 {
		t.Fatalf("attempts=%#v", result.Attempts)
	}
	if !result.Attempts[0].Retryable || result.Attempts[1].Retryable {
		t.Fatalf("retryable attempts=%#v", result.Attempts)
	}
	if result.Retry.MaxAttempts != 2 {
		t.Fatalf("retry spec not recorded: %#v", result.Retry)
	}
}

func TestReportIncludesOperatorHeadlineAndNextActions(t *testing.T) {
	result := Result{
		Success:     true,
		Workflow:    "meshclaw-ops-orchestration-demo",
		Mode:        Execute,
		GeneratedAt: timeNowForTest(),
		Summary: Summary{
			Total:            3,
			Succeeded:        3,
			ApprovalRequired: 1,
			Skipped:          1,
		},
		Steps: []ExecutionResult{
			{
				Success:          true,
				Workflow:         "meshclaw-ops-orchestration-demo",
				Step:             "approval-gate",
				Title:            "Approval gate",
				Transport:        "policy",
				ApprovalRequired: true,
				Skipped:          true,
				PolicyDecision:   "require_approval",
				Stdout:           "approval required before execution",
			},
			{
				Success:        true,
				Workflow:       "meshclaw-ops-orchestration-demo",
				Step:           "worker",
				Title:          "Worker lane",
				Transport:      "local",
				PolicyDecision: "allow",
				Stdout:         `{"status":"degraded","worker":"g2"}`,
			},
		},
	}

	actions := renderActions(result)
	if !strings.Contains(actions, "## Operator Headline") {
		t.Fatalf("actions missing headline:\n%s", actions)
	}
	if !strings.Contains(actions, "## Why This Matters") || !strings.Contains(actions, "repeatable runtime run") {
		t.Fatalf("actions missing value statement:\n%s", actions)
	}
	if !strings.Contains(actions, "## Canonical Loop") || !strings.Contains(actions, "Read inventory truth") {
		t.Fatalf("actions missing canonical loop:\n%s", actions)
	}
	if !strings.Contains(actions, "## Next Actions") {
		t.Fatalf("actions missing next actions:\n%s", actions)
	}
	if !strings.Contains(actions, "record explicit approval") {
		t.Fatalf("actions missing approval next action:\n%s", actions)
	}
	if !strings.Contains(actions, "degraded worker state") {
		t.Fatalf("actions missing degraded finding:\n%s", actions)
	}

	html := renderHTMLReport(result)
	if !strings.Contains(html, "Why this matters") || !strings.Contains(html, "Next Actions") {
		t.Fatalf("html report missing operator sections:\n%s", html)
	}
}

func TestGrantApprovalRequiresReasonForStrongApproval(t *testing.T) {
	dir := t.TempDir()
	result := Result{
		Success:  true,
		Workflow: "account-workflow",
		Mode:     DryRun,
		Steps: []ExecutionResult{
			{
				Success:          true,
				Workflow:         "account-workflow",
				Step:             "delete-account",
				Title:            "Delete account",
				Transport:        "policy",
				Action:           "account_delete",
				Resource:         "account",
				Skipped:          true,
				ApprovalRequired: true,
				StrongApproval:   true,
			},
		},
	}
	writeJSON(t, filepath.Join(dir, "execution.json"), result)

	if _, err := GrantApproval(dir, "delete-account", "operator", "", "test"); err == nil {
		t.Fatal("expected strong approval to require explicit reason")
	}
	record, err := GrantApproval(dir, "delete-account", "operator", "delete requested stale test account", "test")
	if err != nil {
		t.Fatal(err)
	}
	if !record.StrongApproval {
		t.Fatalf("strong approval not recorded: %#v", record)
	}
}

func TestRunWithApprovalsCopiesApprovalEvidenceIntoNewBundle(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_CAPABILITY_FILE", filepath.Join(home, "missing-capabilities.json"))

	source := filepath.Join(home, "source-bundle")
	result := Result{
		Success:  true,
		Workflow: "email-orchestration-demo",
		Mode:     DryRun,
		Steps: []ExecutionResult{
			{
				Success:          true,
				Workflow:         "email-orchestration-demo",
				Step:             "send-approval",
				Title:            "Gate real email send behind approval",
				Transport:        "policy",
				Action:           "email_send",
				Resource:         "email",
				Skipped:          true,
				ApprovalRequired: true,
			},
		},
	}
	writeJSON(t, filepath.Join(source, "execution.json"), result)
	record := ApprovalRecord{
		Time:     timeNowForTest(),
		Actor:    "operator",
		Workflow: "email-orchestration-demo",
		Step:     "send-approval",
		Action:   "email_send",
		Resource: "email",
		Reason:   "approved",
		Bundle:   source,
		Source:   "cli",
	}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "approvals.jsonl"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}

	next, err := RunWithApprovals("email-orchestration-demo", DryRun, source)
	if err != nil {
		t.Fatal(err)
	}
	records, err := ListApprovals(next.BundleDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 {
		t.Fatalf("copied approvals = %d, want 1", len(records))
	}
	if records[0].Step != "send-approval" || records[0].Actor != "operator" {
		t.Fatalf("copied approval = %#v", records[0])
	}
}

func TestPlanExecuteReportsApprovalBlockers(t *testing.T) {
	dir := t.TempDir()
	result := Result{
		Success:  true,
		Workflow: "execute-plan-demo",
		Mode:     DryRun,
		Summary:  Summary{Total: 2, Skipped: 2, ApprovalRequired: 1},
		Steps: []ExecutionResult{
			{
				Success:        true,
				Workflow:       "execute-plan-demo",
				Step:           "inspect",
				Title:          "Inspect",
				Transport:      "local",
				Command:        "uptime",
				Action:         "read_state",
				Resource:       "server",
				Skipped:        true,
				PolicyDecision: string(policy.Allow),
			},
			{
				Success:          true,
				Workflow:         "execute-plan-demo",
				Step:             "restart",
				Title:            "Restart",
				Transport:        "vssh",
				Command:          "systemctl restart demo",
				Action:           "restart_service",
				Resource:         "service",
				Skipped:          true,
				ApprovalRequired: true,
				PolicyDecision:   string(policy.RequireApproval),
				PolicyReason:     "service mutation requires approval",
			},
		},
	}
	writeJSON(t, filepath.Join(dir, "execution.json"), result)

	plan, err := PlanExecute(dir)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Ready || plan.Decision != "approval_required" {
		t.Fatalf("unexpected plan readiness: %#v", plan)
	}
	if plan.Counts.Ready != 1 || plan.Counts.ApprovalPending != 1 {
		t.Fatalf("unexpected counts: %#v", plan.Counts)
	}
	if len(plan.Next) == 0 || !strings.Contains(plan.Next[0], "approvals grant") {
		t.Fatalf("missing approval next action: %#v", plan.Next)
	}
	if !hasRecommendedTool(plan.RecommendedMCP, "meshclaw_approvals_grant") {
		t.Fatalf("missing approval MCP recommendation: %#v", plan.RecommendedMCP)
	}
	if plan.RecommendedMCP[len(plan.RecommendedMCP)-1].Tool != "meshclaw_evidence_latest" {
		t.Fatalf("last recommended tool = %q", plan.RecommendedMCP[len(plan.RecommendedMCP)-1].Tool)
	}
}

func TestPlanExecuteReadyAfterApproval(t *testing.T) {
	dir := t.TempDir()
	result := Result{
		Success:  true,
		Workflow: "execute-plan-demo",
		Mode:     DryRun,
		Summary:  Summary{Total: 1, Skipped: 1, ApprovalRequired: 1},
		Steps: []ExecutionResult{
			{
				Success:          true,
				Workflow:         "execute-plan-demo",
				Step:             "restart",
				Title:            "Restart",
				Transport:        "vssh",
				Command:          "systemctl restart demo",
				Action:           "restart_service",
				Resource:         "service",
				Skipped:          true,
				ApprovalRequired: true,
				PolicyDecision:   string(policy.RequireApproval),
				PolicyReason:     "service mutation requires approval",
			},
		},
	}
	writeJSON(t, filepath.Join(dir, "execution.json"), result)
	if _, err := GrantApproval(dir, "restart", "operator", "approved test restart", "test"); err != nil {
		t.Fatal(err)
	}

	plan, err := PlanExecute(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.Ready || plan.Decision != "ready" {
		t.Fatalf("unexpected plan readiness: %#v", plan)
	}
	if plan.ExecuteCLI == "" || plan.ExecuteMCPCall == nil {
		t.Fatalf("missing execute hints: %#v", plan)
	}
	if !hasRecommendedTool(plan.RecommendedMCP, "meshclaw_workflow_run") {
		t.Fatalf("missing workflow run MCP recommendation: %#v", plan.RecommendedMCP)
	}
}

func TestPlanExecuteBlocksInvalidCapabilityRegistry(t *testing.T) {
	dir := t.TempDir()
	capFile := filepath.Join(dir, "capabilities.json")
	t.Setenv("MESHCLAW_CAPABILITY_FILE", capFile)
	writeJSON(t, capFile, map[string]interface{}{
		"version": 1,
		"capabilities": []map[string]interface{}{
			{"id": "broken", "kind": "invalid"},
		},
	})
	result := Result{
		Success:  true,
		Workflow: "execute-plan-demo",
		Mode:     DryRun,
		Summary:  Summary{Total: 1, Skipped: 1},
		Steps: []ExecutionResult{
			{
				Success:        true,
				Workflow:       "execute-plan-demo",
				Step:           "inspect",
				Title:          "Inspect",
				Transport:      "local",
				Command:        "uptime",
				Action:         "read_state",
				Resource:       "server",
				Skipped:        true,
				PolicyDecision: string(policy.Allow),
			},
		},
	}
	writeJSON(t, filepath.Join(dir, "execution.json"), result)

	plan, err := PlanExecute(dir)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Ready || plan.Decision != "capability_registry_invalid" {
		t.Fatalf("unexpected plan readiness: %#v", plan)
	}
	if plan.CapabilityRegistry.Valid || len(plan.CapabilityRegistry.Errors) == 0 {
		t.Fatalf("expected invalid capability registry: %#v", plan.CapabilityRegistry)
	}
	if !hasRecommendedTool(plan.RecommendedMCP, "meshclaw_capability_validate") {
		t.Fatalf("missing capability validation MCP recommendation: %#v", plan.RecommendedMCP)
	}
}

func hasRecommendedTool(calls []RecommendedMCPCall, tool string) bool {
	for _, call := range calls {
		if call.Tool == tool {
			return true
		}
	}
	return false
}

func TestRunSelectedWithApprovalsRunsOnlySelectedSteps(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("MESHCLAW_CAPABILITY_FILE", filepath.Join(home, "missing-capabilities.json"))

	result, err := RunSelectedWithApprovals("email-orchestration-demo", DryRun, "", []string{"send-approval"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Steps) != 1 {
		t.Fatalf("steps = %d, want 1", len(result.Steps))
	}
	if result.Steps[0].Step != "send-approval" {
		t.Fatalf("step = %q", result.Steps[0].Step)
	}
	if result.Summary.Total != 1 || result.Summary.ApprovalRequired != 1 {
		t.Fatalf("summary = %#v", result.Summary)
	}
	if result.AdaptersPath == "" {
		t.Fatalf("adapters path is empty")
	}
	if _, err := os.Stat(result.AdaptersPath); err != nil {
		t.Fatalf("adapters snapshot missing: %v", err)
	}
	if len(result.Steps[0].CapabilityHints) == 0 || result.Steps[0].CapabilityClass == "" {
		t.Fatalf("expected capability hints on execution result: %#v", result.Steps[0])
	}
}

func TestRunSelectedWithApprovalsRejectsUnknownSteps(t *testing.T) {
	_, err := RunSelectedWithApprovals("email-orchestration-demo", DryRun, "", []string{"no-such-step"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "unknown workflow step") {
		t.Fatalf("error = %v", err)
	}
}

func timeNowForTest() time.Time {
	return time.Date(2026, 5, 18, 0, 0, 0, 0, time.UTC)
}

func writeJSON(t *testing.T, path string, value interface{}) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func validationMessages(issues []ValidationIssue) string {
	var b strings.Builder
	for _, issue := range issues {
		b.WriteString(issue.Message)
		b.WriteByte('\n')
	}
	return b.String()
}
