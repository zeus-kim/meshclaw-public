package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meshclaw/meshclaw/internal/evidence"
	"github.com/meshclaw/meshclaw/internal/nodestate"
	"github.com/meshclaw/meshclaw/internal/reconciler"
)

func TestReconcileDesiredPath(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"--desired", "desired-state.yaml", "--json"}, "desired-state.yaml"},
		{[]string{"-f", "fleet.yaml"}, "fleet.yaml"},
		{[]string{"fleet.yaml", "--json"}, "fleet.yaml"},
	}
	for _, tt := range cases {
		if got := reconcileDesiredPath(tt.args); got != tt.want {
			t.Fatalf("reconcileDesiredPath(%v) = %q, want %q", tt.args, got, tt.want)
		}
	}
}

func TestBuildReconcilePlanReportIsDryRunOnly(t *testing.T) {
	desired := reconciler.DesiredState{Nodes: []reconciler.DesiredNode{{
		ID:       "g4",
		Roles:    []string{"openwebui-worker"},
		Services: map[string]string{"open-webui": "running"},
	}}}
	report := buildReconcilePlanReport("desired-state.yaml", desired, nil, nil)
	if report.Kind != "meshclaw_reconcile_plan" || report.Mode != "dry_run" {
		t.Fatalf("bad report kind/mode: %+v", report)
	}
	if report.Safety["mutation"] != "disabled in this command" {
		t.Fatalf("missing mutation safety: %+v", report.Safety)
	}
	if len(report.Results) != 1 {
		t.Fatalf("results = %d, want 1", len(report.Results))
	}
	if report.Counts["nodes"] != 1 || report.Counts["unmatched"] != 1 || report.Counts["actions"] != 1 {
		t.Fatalf("bad plan counts: %+v", report.Counts)
	}
	if report.Counts["kind_diagnose_offline_node"] != 1 || report.Counts["policy_allow"] != 1 {
		t.Fatalf("bad action counts: %+v", report.Counts)
	}
	if report.Results[0].Matched {
		t.Fatalf("placeholder actual state should not match: %+v", report.Results[0])
	}
	if len(report.Results[0].Actions) != 1 || report.Results[0].Actions[0].Kind != "diagnose_offline_node" {
		t.Fatalf("expected offline diagnosis until actual binding exists: %+v", report.Results[0].Actions)
	}
	action := report.Results[0].Actions[0]
	if action.PolicyDecision != "allow" || action.PolicyResource != "server" || action.PolicyReason == "" {
		t.Fatalf("missing policy annotation: %+v", action)
	}
}

func TestBuildReconcilePlanReportIncludesSchemaVersion(t *testing.T) {
	desired := reconciler.DesiredState{
		SchemaVersion: "v3",
		Nodes: []reconciler.DesiredNode{{
			ID:       "g4",
			Services: map[string]string{"docker": "running"},
		}},
	}
	report := buildReconcilePlanReport("desired-state.yaml", desired, nil, nil)
	if report.SchemaVersion != "v3" || report.Counts["schema_version_present"] != 1 {
		t.Fatalf("schema version not exposed in plan report: %+v", report)
	}
}

func TestReconcilePlanSummaryLineIncludesSchemaVersion(t *testing.T) {
	line := reconcilePlanSummaryLine(reconcilePlanReport{
		Mode:          "dry_run",
		DesiredPath:   "desired-state.yaml",
		SchemaVersion: "v3",
		Desired:       reconciler.DesiredState{Nodes: []reconciler.DesiredNode{{ID: "g4"}}},
	})
	want := "reconcile plan mode=dry_run desired=desired-state.yaml schema_version=v3 nodes=1"
	if line != want {
		t.Fatalf("plan summary line = %q, want %q", line, want)
	}
}

func TestReconcilePlanSummaryLineMissingSchemaVersion(t *testing.T) {
	line := reconcilePlanSummaryLine(reconcilePlanReport{
		Mode:        "dry_run",
		DesiredPath: "desired-state.yaml",
		Desired:     reconciler.DesiredState{Nodes: []reconciler.DesiredNode{{ID: "g4"}}},
	})
	want := "reconcile plan mode=dry_run desired=desired-state.yaml schema_version=<unset> nodes=1"
	if line != want {
		t.Fatalf("plan summary line without schema version = %q, want %q", line, want)
	}
}

func TestBuildReconcileDesiredValidationReportBlocksCriticalFindings(t *testing.T) {
	tooHigh := 101.0
	report := buildReconcileDesiredValidationReport("desired-state.yaml", reconciler.DesiredState{Nodes: []reconciler.DesiredNode{{
		ID:               "g4",
		MaxMemoryUsedPct: &tooHigh,
	}}})
	if report.Ready || report.Status != "blocked" || report.Counts["critical"] != 1 {
		t.Fatalf("bad desired validation report: %+v", report)
	}
	if report.Handoff.Decision != "blocked_until_desired_yaml_fixed" {
		t.Fatalf("critical validation should block handoff: %+v", report.Handoff)
	}
	if !containsString(report.Handoff.AllowedNextTools, "meshclaw_reconcile_validate_desired") || !containsString(report.Handoff.BlockedNextTools, "meshclaw_reconcile_plan") {
		t.Fatalf("critical validation handoff should only allow revalidation: %+v", report.Handoff)
	}
}

func TestBuildReconcileDesiredValidationReportAllowsWarnings(t *testing.T) {
	report := buildReconcileDesiredValidationReport("desired-state.yaml", reconciler.DesiredState{Nodes: []reconciler.DesiredNode{{
		ID:    "g4",
		Roles: []string{"gpu", "gpu"},
	}}})
	if !report.Ready || report.Status != "ready_with_warnings" || report.Counts["warning"] != 1 {
		t.Fatalf("bad desired validation warning report: %+v", report)
	}
	if report.Handoff.Decision != "validated_for_dry_run_reconcile" {
		t.Fatalf("warning-only validation should still allow dry-run handoff: %+v", report.Handoff)
	}
	if !containsString(report.Handoff.AllowedNextTools, "meshclaw_reconcile_plan") || !containsString(report.Handoff.AllowedNextTools, "meshclaw_reconcile_approval_request") {
		t.Fatalf("warning-only validation should allow plan/approval request: %+v", report.Handoff)
	}
	if !containsString(report.Handoff.StopBefore, "treating validation as apply approval") {
		t.Fatalf("validation handoff should block apply approval semantics: %+v", report.Handoff)
	}
}

func TestBuildReconcileDesiredValidationReportIncludesSchemaVersion(t *testing.T) {
	report := buildReconcileDesiredValidationReport("desired-state.yaml", reconciler.DesiredState{
		SchemaVersion: "v3",
		Nodes: []reconciler.DesiredNode{{
			ID:       "g4",
			Services: map[string]string{"docker": "running"},
		}},
	})
	if report.SchemaVersion != "v3" || report.Counts["schema_version_present"] != 1 {
		t.Fatalf("schema version not exposed in validation report: %+v", report)
	}
	if !containsString(report.Handoff.RequiredEvidence, "schema_version and validation finding counts") {
		t.Fatalf("validation handoff should require schema evidence: %+v", report.Handoff)
	}
	if !containsString(report.Handoff.RefreshTriggers, "desired-state YAML changed") {
		t.Fatalf("validation handoff should refresh on YAML changes: %+v", report.Handoff)
	}
}

func TestBuildReconcileDesiredValidationReportCountsIgnoredApplyKeys(t *testing.T) {
	report := buildReconcileDesiredValidationReport("desired-state.yaml", reconciler.DesiredState{
		SchemaVersion: "v3",
		ParseFindings: []reconciler.DesiredValidationFinding{{
			Severity: "warning",
			NodeID:   "g4",
			Field:    "nodes.g4.execute",
			Message:  "desired-state YAML key execute is ignored and does not grant apply, execute, or approval",
		}},
		Nodes: []reconciler.DesiredNode{{
			ID:       "g4",
			Services: map[string]string{"docker": "running"},
		}},
	})

	if report.Status != "ready_with_warnings" || !report.Ready {
		t.Fatalf("ignored apply key should warn without blocking dry-run readiness: %+v", report)
	}
	if report.Counts["ignored_apply_keys"] != 1 {
		t.Fatalf("ignored apply key count missing: %+v", report.Counts)
	}
	if !containsString(report.Handoff.StopBefore, "treating ignored YAML apply/execute keys as approval") {
		t.Fatalf("validation handoff should stop before ignored YAML approval semantics: %+v", report.Handoff)
	}
	if !containsString(report.Handoff.RefreshTriggers, "ignored apply/execute YAML key changed") {
		t.Fatalf("validation handoff should refresh on ignored key changes: %+v", report.Handoff)
	}
}

func TestReconcileDesiredValidationSummaryLineIncludesSchemaVersion(t *testing.T) {
	line := reconcileDesiredValidationSummaryLine(reconcileDesiredValidationReport{
		Ready:         true,
		Status:        "ready",
		DesiredPath:   "desired-state.yaml",
		SchemaVersion: "v3",
		Findings:      []reconciler.DesiredValidationFinding{{Severity: "warning"}},
	})
	want := "reconcile validate-desired ready=true status=ready desired=desired-state.yaml schema_version=v3 findings=1"
	if line != want {
		t.Fatalf("validation summary line = %q, want %q", line, want)
	}
}

func TestReconcileDesiredValidationHandoffLine(t *testing.T) {
	line := reconcileDesiredValidationHandoffLine(reconcileDesiredValidationReport{
		Handoff: reconcileValidationHandoff{
			Decision:         "validated_for_dry_run_reconcile",
			AllowedNextTools: []string{"meshclaw_reconcile_plan", "meshclaw_reconcile_approval_request"},
			BlockedNextTools: []string{"meshclaw_reconcile_apply_gate"},
			RequiredEvidence: []string{"stored validation evidence", "schema counts"},
			StopBefore:       []string{"treating validation as apply approval"},
			RefreshTriggers:  []string{"desired-state YAML changed"},
		},
	})
	want := "validation_handoff decision=validated_for_dry_run_reconcile allowed=meshclaw_reconcile_plan,meshclaw_reconcile_approval_request blocked=1 required_evidence=2 stop_before=1 refresh_triggers=1"
	if line != want {
		t.Fatalf("validation handoff line = %q, want %q", line, want)
	}
}

func TestReconcileDesiredValidationHandoffLineEmpty(t *testing.T) {
	line := reconcileDesiredValidationHandoffLine(reconcileDesiredValidationReport{})
	want := "validation_handoff decision=<unset> allowed=<none> blocked=0 required_evidence=0 stop_before=0 refresh_triggers=0"
	if line != want {
		t.Fatalf("empty validation handoff line = %q, want %q", line, want)
	}
}

func TestReconcileApprovalRequestSummaryLineIncludesSchemaVersion(t *testing.T) {
	line := reconcileApprovalRequestSummaryLine(reconcileApprovalRequest{
		Status:        "blocked",
		DesiredPath:   "desired-state.yaml",
		SchemaVersion: "v3",
		Actions:       []reconciler.Action{{ID: "g4:approve"}},
		BlockedActions: []reconciler.Action{{
			ID: "g4:deny",
		}},
	})
	want := "reconcile approval-request status=blocked approvals=1 blocked=1 desired=desired-state.yaml schema_version=v3"
	if line != want {
		t.Fatalf("approval request summary line = %q, want %q", line, want)
	}
}

func TestReconcileApprovalRequestSummaryLineMissingSchemaVersion(t *testing.T) {
	line := reconcileApprovalRequestSummaryLine(reconcileApprovalRequest{
		Status:      "no_approval_required",
		DesiredPath: "desired-state.yaml",
	})
	want := "reconcile approval-request status=no_approval_required approvals=0 blocked=0 desired=desired-state.yaml schema_version=<unset>"
	if line != want {
		t.Fatalf("approval request summary line without schema version = %q, want %q", line, want)
	}
}

func TestReconcileApplyGateSummaryLineIncludesSchemaVersion(t *testing.T) {
	line := reconcileApplyGateSummaryLine(reconcileApplyGateReport{
		Ready:              true,
		Status:             "ready",
		DesiredPath:        "desired-state.yaml",
		SchemaVersion:      "v3",
		ApprovalEvidenceID: "evidence-1",
	})
	want := "reconcile apply-gate ready=true status=ready desired=desired-state.yaml schema_version=v3 approval_evidence=evidence-1"
	if line != want {
		t.Fatalf("apply gate summary line = %q, want %q", line, want)
	}
}

func TestReconcileApplyGateSummaryLineMissingSchemaVersion(t *testing.T) {
	line := reconcileApplyGateSummaryLine(reconcileApplyGateReport{
		Status:      "not_ready",
		DesiredPath: "desired-state.yaml",
	})
	want := "reconcile apply-gate ready=false status=not_ready desired=desired-state.yaml schema_version=<unset> approval_evidence=<unset>"
	if line != want {
		t.Fatalf("apply gate summary line without schema version = %q, want %q", line, want)
	}
}

func TestReconcileApplyPlanSummaryLineIncludesSchemaVersion(t *testing.T) {
	line := reconcileApplyPlanSummaryLine(reconcileApplyPlanReport{
		Ready:          true,
		Status:         "ready",
		SchemaVersion:  "v3",
		GateEvidenceID: "gate-1",
		Steps:          []reconcileApplyStep{{Step: 1}},
	})
	want := "reconcile apply-plan ready=true status=ready schema_version=v3 steps=1 gate=gate-1"
	if line != want {
		t.Fatalf("apply plan summary line = %q, want %q", line, want)
	}
}

func TestReconcileApplyPlanSummaryLineMissingSchemaVersion(t *testing.T) {
	line := reconcileApplyPlanSummaryLine(reconcileApplyPlanReport{
		Status: "blocked",
	})
	want := "reconcile apply-plan ready=false status=blocked schema_version=<unset> steps=0 gate=<unset>"
	if line != want {
		t.Fatalf("apply plan summary line without schema version = %q, want %q", line, want)
	}
}

func TestReconcileExecutionPreviewSummaryLineIncludesSchemaVersion(t *testing.T) {
	line := reconcileExecutionPreviewSummaryLine(reconcileExecutionPreviewReport{
		Ready:               true,
		Status:              "ready",
		SchemaVersion:       "v3",
		ApplyPlanEvidenceID: "plan-1",
		Commands:            []reconcileExecutionCommand{{Step: 1}},
	})
	want := "reconcile execution-preview ready=true status=ready schema_version=v3 commands=1 apply_plan=plan-1"
	if line != want {
		t.Fatalf("execution preview summary line = %q, want %q", line, want)
	}
}

func TestReconcileExecutionPreviewSummaryLineMissingSchemaVersion(t *testing.T) {
	line := reconcileExecutionPreviewSummaryLine(reconcileExecutionPreviewReport{
		Status: "blocked",
	})
	want := "reconcile execution-preview ready=false status=blocked schema_version=<unset> commands=0 apply_plan=<unset>"
	if line != want {
		t.Fatalf("execution preview summary line without schema version = %q, want %q", line, want)
	}
}

func TestReconcileVerificationPlanSummaryLineIncludesSchemaVersion(t *testing.T) {
	line := reconcileVerificationPlanSummaryLine(reconcileVerificationPlanReport{
		Ready:                      true,
		Status:                     "ready",
		SchemaVersion:              "v3",
		ExecutionPreviewEvidenceID: "preview-1",
		Checks:                     []reconcileVerificationCheck{{Step: 1}},
	})
	want := "reconcile verification-plan ready=true status=ready schema_version=v3 checks=1 execution_preview=preview-1"
	if line != want {
		t.Fatalf("verification plan summary line = %q, want %q", line, want)
	}
}

func TestReconcileVerificationPlanSummaryLineMissingSchemaVersion(t *testing.T) {
	line := reconcileVerificationPlanSummaryLine(reconcileVerificationPlanReport{
		Status: "blocked",
	})
	want := "reconcile verification-plan ready=false status=blocked schema_version=<unset> checks=0 execution_preview=<unset>"
	if line != want {
		t.Fatalf("verification plan summary line without schema version = %q, want %q", line, want)
	}
}

func TestReconcileRunbookSummaryLineIncludesSchemaVersion(t *testing.T) {
	line := reconcileRunbookSummaryLine(reconcileRunbookReport{
		Ready:                      true,
		Status:                     "ready",
		SchemaVersion:              "v3",
		VerificationPlanEvidenceID: "verify-1",
		Steps:                      []reconcileRunbookStep{{Step: 1}},
	})
	want := "reconcile runbook ready=true status=ready schema_version=v3 steps=1 verification_plan=verify-1"
	if line != want {
		t.Fatalf("runbook summary line = %q, want %q", line, want)
	}
}

func TestReconcileRunbookSummaryLineMissingSchemaVersion(t *testing.T) {
	line := reconcileRunbookSummaryLine(reconcileRunbookReport{
		Status: "blocked",
	})
	want := "reconcile runbook ready=false status=blocked schema_version=<unset> steps=0 verification_plan=<unset>"
	if line != want {
		t.Fatalf("runbook summary line without schema version = %q, want %q", line, want)
	}
}

func TestReconcileRunbookCheckSummaryLineIncludesSchemaVersion(t *testing.T) {
	line := reconcileRunbookCheckSummaryLine(reconcileRunbookCheckReport{
		Ready:             true,
		Status:            "ready",
		SchemaVersion:     "v3",
		RunbookEvidenceID: "runbook-1",
		Findings:          []reconcileCheckFinding{{Severity: "warning"}},
	})
	want := "reconcile runbook-check ready=true status=ready schema_version=v3 findings=1 runbook=runbook-1"
	if line != want {
		t.Fatalf("runbook check summary line = %q, want %q", line, want)
	}
}

func TestReconcileRunbookCheckSummaryLineMissingSchemaVersion(t *testing.T) {
	line := reconcileRunbookCheckSummaryLine(reconcileRunbookCheckReport{
		Status: "blocked",
	})
	want := "reconcile runbook-check ready=false status=blocked schema_version=<unset> findings=0 runbook=<unset>"
	if line != want {
		t.Fatalf("runbook check summary line without schema version = %q, want %q", line, want)
	}
}

func TestReconcileRollbackPlanSummaryLineIncludesSchemaVersion(t *testing.T) {
	line := reconcileRollbackPlanSummaryLine(reconcileRollbackPlanReport{
		Ready:                  true,
		Status:                 "ready",
		SchemaVersion:          "v3",
		RunbookCheckEvidenceID: "check-1",
		Steps:                  []reconcileRollbackStep{{Step: 1}},
	})
	want := "reconcile rollback-plan ready=true status=ready schema_version=v3 steps=1 runbook_check=check-1"
	if line != want {
		t.Fatalf("rollback plan summary line = %q, want %q", line, want)
	}
}

func TestReconcileRollbackPlanSummaryLineMissingSchemaVersion(t *testing.T) {
	line := reconcileRollbackPlanSummaryLine(reconcileRollbackPlanReport{
		Status: "blocked",
	})
	want := "reconcile rollback-plan ready=false status=blocked schema_version=<unset> steps=0 runbook_check=<unset>"
	if line != want {
		t.Fatalf("rollback plan summary line without schema version = %q, want %q", line, want)
	}
}

func TestReconcileCompletionPlanSummaryLineIncludesSchemaVersion(t *testing.T) {
	line := reconcileCompletionPlanSummaryLine(reconcileCompletionPlanReport{
		Ready:                  true,
		Status:                 "ready",
		SchemaVersion:          "v3",
		RollbackPlanEvidenceID: "rollback-1",
		Requirements:           []reconcileCompletionRequirement{{Step: 1}},
	})
	want := "reconcile completion-plan ready=true status=ready schema_version=v3 requirements=1 rollback_plan=rollback-1"
	if line != want {
		t.Fatalf("completion plan summary line = %q, want %q", line, want)
	}
}

func TestReconcileCompletionPlanSummaryLineMissingSchemaVersion(t *testing.T) {
	line := reconcileCompletionPlanSummaryLine(reconcileCompletionPlanReport{
		Status: "blocked",
	})
	want := "reconcile completion-plan ready=false status=blocked schema_version=<unset> requirements=0 rollback_plan=<unset>"
	if line != want {
		t.Fatalf("completion plan summary line without schema version = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummarySummaryLineIncludesSchemaVersion(t *testing.T) {
	line := reconcileReadinessSummarySummaryLine(reconcileReadinessSummaryReport{
		Ready:                    true,
		Status:                   "ready",
		SchemaVersion:            "v3",
		CompletionPlanEvidenceID: "completion-1",
		ReadyStages:              []string{"apply_gate"},
		Blockers:                 []string{"manual_review"},
	})
	want := "reconcile readiness-summary ready=true status=ready schema_version=v3 stages=1 blockers=1 completion_plan=completion-1"
	if line != want {
		t.Fatalf("readiness summary line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummarySummaryLineMissingSchemaVersion(t *testing.T) {
	line := reconcileReadinessSummarySummaryLine(reconcileReadinessSummaryReport{
		Status: "blocked",
	})
	want := "reconcile readiness-summary ready=false status=blocked schema_version=<unset> stages=0 blockers=0 completion_plan=<unset>"
	if line != want {
		t.Fatalf("readiness summary line without schema version = %q, want %q", line, want)
	}
}

func TestReconcileCommandHasFlagValue(t *testing.T) {
	cases := []struct {
		name    string
		command string
		flag    string
		value   string
		want    bool
	}{
		{
			name:    "unquoted value",
			command: "meshclaw agent apply-step --node g4 --operation service_reconcile",
			flag:    "--node",
			value:   "g4",
			want:    true,
		},
		{
			name:    "single quoted value",
			command: "meshclaw agent apply-step --container 'open-webui' --require-evidence",
			flag:    "--container",
			value:   "open-webui",
			want:    true,
		},
		{
			name:    "double quoted value",
			command: "meshclaw agent apply-step --container \"open-webui\" --require-evidence",
			flag:    "--container",
			value:   "open-webui",
			want:    true,
		},
		{
			name:    "prefixed value rejected",
			command: "meshclaw agent apply-step --node g4x --operation service_reconcile",
			flag:    "--node",
			value:   "g4",
			want:    false,
		},
		{
			name:    "different flag rejected",
			command: "meshclaw agent apply-step --target g4 --operation service_reconcile",
			flag:    "--node",
			value:   "g4",
			want:    false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := reconcileCommandHasFlagValue(tc.command, tc.flag, tc.value); got != tc.want {
				t.Fatalf("reconcileCommandHasFlagValue(%q, %q, %q) = %t, want %t", tc.command, tc.flag, tc.value, got, tc.want)
			}
		})
	}
}

func TestReconcileActualReportPaths(t *testing.T) {
	paths := reconcileActualReportPaths([]string{"--actual-report", "a.json,b.json", "--json"})
	if len(paths) != 2 || paths[0] != "a.json" || paths[1] != "b.json" {
		t.Fatalf("paths = %#v", paths)
	}
}

func TestReconcilePlanUsesActualReport(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "g4.json")
	data, err := json.Marshal(nodestate.Report{
		NodeName: "g4",
		System:   nodestate.SystemState{DiskPct: 50, MemoryPct: 40},
		Inventory: nodestate.InventoryHint{
			PrimaryRole: "openwebui-worker",
		},
		Services: []nodestate.ServiceState{{Name: "open-webui", Active: "active"}},
		Docker: nodestate.DockerState{Containers: []nodestate.DockerContainer{{
			Name:         "open-webui",
			Image:        "web:v1",
			State:        "running",
			HealthStatus: "healthy",
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	actuals, actualPaths, err := reconcileActualReports([]string{"--actual-report", path})
	if err != nil {
		t.Fatal(err)
	}
	report := buildReconcilePlanReport("desired-state.yaml", reconciler.DesiredState{Nodes: []reconciler.DesiredNode{{
		ID:       "g4",
		Roles:    []string{"openwebui-worker"},
		Services: map[string]string{"open-webui": "running"},
		Containers: map[string]reconciler.DesiredContainer{
			"open-webui": {Desired: "running", Image: "web:v1", Health: "healthy"},
		},
	}}}, actuals, actualPaths)
	if len(report.Results) != 1 || !report.Results[0].Matched || len(report.Results[0].Actions) != 0 {
		t.Fatalf("expected actual report to satisfy desired state: %+v", report.Results)
	}
	if len(report.ActualPaths) != 1 || report.ActualPaths[0] != path {
		t.Fatalf("actual paths = %#v", report.ActualPaths)
	}
	if report.Counts["matched"] != 1 || report.Counts["actual_reports"] != 1 || report.Counts["actions"] != 0 {
		t.Fatalf("bad satisfied plan counts: %+v", report.Counts)
	}
}

func TestReconcilePlanAnnotatesApprovalRequiredPolicy(t *testing.T) {
	report := buildReconcilePlanReport("desired-state.yaml", reconciler.DesiredState{Nodes: []reconciler.DesiredNode{{
		ID:    "g4",
		Roles: []string{"missing-role"},
	}}}, map[string]reconciler.ActualNode{
		"g4": {ID: "g4", Online: true},
	}, nil)
	if len(report.Results) != 1 || len(report.Results[0].Actions) != 1 {
		t.Fatalf("expected one drift action: %+v", report.Results)
	}
	action := report.Results[0].Actions[0]
	if action.PolicyAction != "plan_only" || action.PolicyDecision != "require_approval" || !action.ApprovalRequired {
		t.Fatalf("expected plan_only to be approval annotated: %+v", action)
	}
	if action.PolicyResource != "reconcile_plan" || action.PolicyReason == "" {
		t.Fatalf("missing policy metadata: %+v", action)
	}
	if report.Counts["approval_required"] != 1 || report.Counts["policy_require_approval"] != 1 {
		t.Fatalf("bad approval counts: %+v", report.Counts)
	}
}

func TestReconcilePlanCountsLine(t *testing.T) {
	line := reconcilePlanCountsLine(map[string]int{
		"nodes":             2,
		"results":           2,
		"matched":           1,
		"unmatched":         1,
		"actions":           3,
		"approval_required": 1,
		"container_actions": 2,
	})
	want := "nodes=2 results=2 matched=1 unmatched=1 actions=3 approval_required=1 container_actions=2"
	if line != want {
		t.Fatalf("counts line = %q, want %q", line, want)
	}
}

func TestReconcileRunOnceRequiresDryRun(t *testing.T) {
	if err := reconcileRunOnce([]string{"--desired", "desired-state.yaml"}); err == nil {
		t.Fatal("expected dry-run requirement error")
	}
	if err := reconcileRunOnce([]string{"--dry-run", "--execute", "--desired", "desired-state.yaml"}); err == nil {
		t.Fatal("expected execute rejection")
	}
}

func TestBuildReconcileApprovalRequestSeparatesPolicyActions(t *testing.T) {
	plan := reconcilePlanReport{
		DesiredPath:   "desired-state.yaml",
		SchemaVersion: "v3",
		Results: []reconciler.Result{{
			NodeID: "g4",
			Actions: []reconciler.Action{
				{ID: "g4:role", NodeID: "g4", Summary: "role drift", PolicyDecision: "require_approval", ApprovalRequired: true},
				{ID: "g4:container:api", Kind: "container_drift", NodeID: "g4", Summary: "container drift", PolicyAction: "container_recreate", PolicyDecision: "require_approval", ApprovalRequired: true, Metadata: map[string]string{"container": "api"}},
				{ID: "g4:deny", NodeID: "g4", Summary: "invalid desired state", PolicyDecision: "deny", ApprovalRequired: true},
				{ID: "g4:read", NodeID: "g4", Summary: "read-only check", PolicyDecision: "allow"},
			},
		}},
	}
	request := buildReconcileApprovalRequest(plan)
	if request.Status != "blocked" || !request.ApprovalRequired {
		t.Fatalf("bad request status: %+v", request)
	}
	if len(request.Actions) != 2 || request.Actions[1].ID != "g4:container:api" {
		t.Fatalf("approval actions = %+v", request.Actions)
	}
	if len(request.BlockedActions) != 1 || request.BlockedActions[0].ID != "g4:deny" {
		t.Fatalf("blocked actions = %+v", request.BlockedActions)
	}
	if request.Counts["allowed"] != 1 || request.Counts["approval_required"] != 2 || request.Counts["blocked"] != 1 {
		t.Fatalf("counts = %+v", request.Counts)
	}
	if request.Counts["container_total"] != 1 || request.Counts["container_approval_required"] != 1 {
		t.Fatalf("counts = %+v", request.Counts)
	}
	if request.SchemaVersion != "v3" || request.Counts["schema_version_present"] != 1 {
		t.Fatalf("schema version not exposed in approval request: %+v", request)
	}
}

func TestBuildReconcileApplyGateRequiresApprovalAndBlocksDenied(t *testing.T) {
	request := reconcileApprovalRequest{
		Kind:          "meshclaw_reconcile_approval_request",
		DesiredPath:   "desired-state.yaml",
		SchemaVersion: "v3",
		Counts:        map[string]int{"approval_required": 1, "container_approval_required": 1, "schema_version_present": 1},
		Actions: []reconciler.Action{{
			ID:               "g4:role",
			NodeID:           "g4",
			PolicyDecision:   "require_approval",
			ApprovalRequired: true,
		}},
	}
	record := evidence.Record{ID: "evidence-1", Kind: "reconcile-approval-request", Payload: request}
	withoutApproval := buildReconcileApplyGateReport("desired-state.yaml", "approval.json", record, request, "")
	if withoutApproval.Ready || withoutApproval.Status != "not_ready" {
		t.Fatalf("gate without approved_by should not be ready: %+v", withoutApproval)
	}
	withApproval := buildReconcileApplyGateReport("desired-state.yaml", "approval.json", record, request, "zeus")
	if !withApproval.Ready || withApproval.Status != "ready" {
		t.Fatalf("gate with approved_by should be ready: %+v", withApproval)
	}
	if withApproval.Counts["container_approval_required"] != 1 || withApproval.Counts["gate_ready"] != 1 {
		t.Fatalf("gate counts not propagated: %+v", withApproval.Counts)
	}
	if withApproval.SchemaVersion != "v3" || withApproval.Counts["schema_version_present"] != 1 {
		t.Fatalf("schema version not exposed in apply gate: %+v", withApproval)
	}
	request.BlockedActions = []reconciler.Action{{ID: "g4:deny", PolicyDecision: "deny"}}
	blocked := buildReconcileApplyGateReport("desired-state.yaml", "approval.json", record, request, "zeus")
	if blocked.Ready || blocked.Status != "not_ready" {
		t.Fatalf("blocked gate should not be ready: %+v", blocked)
	}
	if blocked.Counts["gate_blocked"] != 1 {
		t.Fatalf("blocked gate counts = %+v", blocked.Counts)
	}
}

func TestApprovalRequestFromEvidenceRejectsWrongKind(t *testing.T) {
	_, err := approvalRequestFromEvidence(evidence.Record{Kind: "reconcile-plan", Payload: map[string]interface{}{}})
	if err == nil {
		t.Fatal("expected wrong evidence kind error")
	}
}

func TestBuildReconcileApplyPlanUsesReadyGate(t *testing.T) {
	request := reconcileApprovalRequest{
		Kind:        "meshclaw_reconcile_approval_request",
		DesiredPath: "desired-state.yaml",
		Actions: []reconciler.Action{{
			ID:             "g4:service:open-webui",
			Kind:           "service_drift",
			NodeID:         "g4",
			Severity:       "high",
			Summary:        "g4 service drift",
			PolicyAction:   "service_check",
			PolicyDecision: "require_approval",
		}},
	}
	gate := reconcileApplyGateReport{
		Kind:            "meshclaw_reconcile_apply_gate",
		Ready:           true,
		Status:          "ready",
		SchemaVersion:   "v3",
		ApprovedBy:      "zeus",
		ApprovalRequest: request,
		Counts:          map[string]int{"schema_version_present": 1},
	}
	report := buildReconcileApplyPlanReport("gate.json", evidence.Record{ID: "gate-1", Kind: "reconcile-apply-gate", Payload: gate}, gate)
	if !report.Ready || report.Status != "ready" || len(report.Steps) != 1 {
		t.Fatalf("bad apply plan: %+v", report)
	}
	if report.Counts["apply_steps"] != 1 || report.Counts["apply_plan_ready"] != 1 {
		t.Fatalf("bad apply plan counts: %+v", report.Counts)
	}
	if report.SchemaVersion != "v3" || report.Counts["schema_version_present"] != 1 {
		t.Fatalf("schema version not exposed in apply plan: %+v", report)
	}
	step := report.Steps[0]
	if step.Operation != "service_reconcile" || step.Metadata["approved_by"] != "zeus" {
		t.Fatalf("bad apply step: %+v", step)
	}
}

func TestBuildReconcileApplyPlanCarriesContainerMetadata(t *testing.T) {
	request := reconcileApprovalRequest{
		Kind:        "meshclaw_reconcile_approval_request",
		DesiredPath: "desired-state.yaml",
		Actions: []reconciler.Action{{
			ID:             "g4:container:open-webui",
			Kind:           "container_drift",
			NodeID:         "g4",
			Severity:       "high",
			Summary:        "g4 container drift",
			PolicyAction:   "container_recreate",
			PolicyDecision: "require_approval",
			Metadata:       map[string]string{"container": "open-webui", "desired_image": "web:v2", "actual_image": "web:v1", "desired_restart": "approval_required"},
		}},
	}
	gate := reconcileApplyGateReport{
		Kind:            "meshclaw_reconcile_apply_gate",
		Ready:           true,
		Status:          "ready",
		ApprovedBy:      "zeus",
		ApprovalRequest: request,
	}
	report := buildReconcileApplyPlanReport("gate.json", evidence.Record{ID: "gate-1", Kind: "reconcile-apply-gate", Payload: gate}, gate)
	if len(report.Steps) != 1 {
		t.Fatalf("steps = %+v", report.Steps)
	}
	step := report.Steps[0]
	if step.Operation != "container_reconcile" || step.Metadata["container"] != "open-webui" {
		t.Fatalf("bad container apply step: %+v", step)
	}
	if step.Metadata["desired_image"] != "web:v2" || step.Metadata["actual_image"] != "web:v1" {
		t.Fatalf("container action metadata not propagated: %+v", step.Metadata)
	}
	if step.Metadata["desired_restart"] != "approval_required" {
		t.Fatalf("container restart metadata not propagated: %+v", step.Metadata)
	}
	if report.Counts["container_apply_steps"] != 1 {
		t.Fatalf("bad container apply plan counts: %+v", report.Counts)
	}
}

func TestBuildReconcileApplyPlanBlocksNotReadyGate(t *testing.T) {
	gate := reconcileApplyGateReport{
		Kind:    "meshclaw_reconcile_apply_gate",
		Ready:   false,
		Status:  "not_ready",
		Reasons: []string{"approval-required actions need approved_by before apply can proceed"},
	}
	report := buildReconcileApplyPlanReport("gate.json", evidence.Record{ID: "gate-1", Kind: "reconcile-apply-gate", Payload: gate}, gate)
	if report.Ready || report.Status != "blocked" || len(report.Steps) != 0 {
		t.Fatalf("not-ready gate should block apply plan: %+v", report)
	}
	if report.Counts["apply_plan_blocked"] != 1 {
		t.Fatalf("bad blocked apply plan counts: %+v", report.Counts)
	}
}

func TestApplyGateFromEvidenceRejectsWrongKind(t *testing.T) {
	_, err := applyGateFromEvidence(evidence.Record{Kind: "reconcile-approval-request", Payload: map[string]interface{}{}})
	if err == nil {
		t.Fatal("expected wrong evidence kind error")
	}
}

func TestBuildReconcileExecutionPreviewUsesReadyPlan(t *testing.T) {
	plan := reconcileApplyPlanReport{
		Kind:          "meshclaw_reconcile_apply_plan",
		Ready:         true,
		Status:        "ready",
		SchemaVersion: "v3",
		Counts:        map[string]int{"schema_version_present": 1},
		Steps: []reconcileApplyStep{{
			Step:         1,
			NodeID:       "g4",
			ActionID:     "g4:service:open-webui",
			Kind:         "service_drift",
			Operation:    "service_reconcile",
			PolicyAction: "service_check",
		}},
	}
	report := buildReconcileExecutionPreviewReport("plan.json", evidence.Record{ID: "plan-1", Kind: "reconcile-apply-plan", Payload: plan}, plan)
	if !report.Ready || report.Status != "ready" || len(report.Commands) != 1 {
		t.Fatalf("bad execution preview: %+v", report)
	}
	if report.Counts["execution_commands"] != 1 || report.Counts["execution_preview_ready"] != 1 {
		t.Fatalf("bad execution preview counts: %+v", report.Counts)
	}
	if report.SchemaVersion != "v3" || report.Counts["schema_version_present"] != 1 {
		t.Fatalf("schema version not exposed in execution preview: %+v", report)
	}
	command := report.Commands[0]
	if command.Executor != "meshclaw-agent" || !command.RequiresVerification {
		t.Fatalf("bad command preview: %+v", command)
	}
	if command.CommandTemplate == "" || command.VerificationHint == "" {
		t.Fatalf("missing command details: %+v", command)
	}
}

func TestBuildReconcileExecutionPreviewRendersContainerTemplate(t *testing.T) {
	plan := reconcileApplyPlanReport{
		Kind:   "meshclaw_reconcile_apply_plan",
		Ready:  true,
		Status: "ready",
		Steps: []reconcileApplyStep{{
			Step:         1,
			NodeID:       "g4",
			ActionID:     "g4:container:open-webui",
			Kind:         "container_drift",
			Operation:    "container_reconcile",
			PolicyAction: "container_recreate",
			Metadata: map[string]string{
				"container":       "open-webui",
				"desired_state":   "running",
				"actual_state":    "running",
				"desired_image":   "web:v2",
				"actual_image":    "web:v1",
				"desired_health":  "healthy",
				"actual_health":   "unhealthy",
				"desired_restart": "approval_required",
				"actual_restart":  "no",
			},
		}},
	}
	report := buildReconcileExecutionPreviewReport("plan.json", evidence.Record{ID: "plan-1", Kind: "reconcile-apply-plan", Payload: plan}, plan)
	if len(report.Commands) != 1 {
		t.Fatalf("commands = %+v", report.Commands)
	}
	command := report.Commands[0]
	if !strings.Contains(command.CommandTemplate, "--container 'open-webui'") || !strings.Contains(command.VerificationHint, "Docker container state") {
		t.Fatalf("bad container command preview: %+v", command)
	}
	if !strings.Contains(command.CommandTemplate, "--desired-restart 'approval_required'") {
		t.Fatalf("container restart policy missing from command preview: %+v", command)
	}
	for _, want := range []string{"--desired-state 'running'", "--actual-state 'running'", "--desired-image 'web:v2'", "--actual-image 'web:v1'", "--desired-health 'healthy'", "--actual-health 'unhealthy'", "--actual-restart 'no'"} {
		if !strings.Contains(command.CommandTemplate, want) {
			t.Fatalf("container desired metadata missing from command preview %q: %+v", want, command)
		}
	}
	if command.Metadata["container"] != "open-webui" {
		t.Fatalf("container metadata not propagated to command: %+v", command.Metadata)
	}
	if command.Metadata["desired_state"] != "running" || command.Metadata["desired_image"] != "web:v2" || command.Metadata["desired_health"] != "healthy" {
		t.Fatalf("container desired metadata not propagated to command: %+v", command.Metadata)
	}
	if command.Metadata["actual_state"] != "running" || command.Metadata["actual_image"] != "web:v1" || command.Metadata["actual_health"] != "unhealthy" {
		t.Fatalf("container actual metadata not propagated to command: %+v", command.Metadata)
	}
	if command.Metadata["desired_restart"] != "approval_required" {
		t.Fatalf("container restart metadata not propagated to command: %+v", command.Metadata)
	}
	if command.Metadata["actual_restart"] != "no" {
		t.Fatalf("container actual restart metadata not propagated to command: %+v", command.Metadata)
	}
	if report.Counts["container_execution_commands"] != 1 {
		t.Fatalf("bad container execution preview counts: %+v", report.Counts)
	}
}

func TestBuildReconcileExecutionPreviewBlocksNotReadyPlan(t *testing.T) {
	plan := reconcileApplyPlanReport{
		Kind:    "meshclaw_reconcile_apply_plan",
		Ready:   false,
		Status:  "blocked",
		Reasons: []string{"apply gate is not ready"},
	}
	report := buildReconcileExecutionPreviewReport("plan.json", evidence.Record{ID: "plan-1", Kind: "reconcile-apply-plan", Payload: plan}, plan)
	if report.Ready || report.Status != "blocked" || len(report.Commands) != 0 {
		t.Fatalf("not-ready plan should block preview: %+v", report)
	}
	if report.Counts["execution_preview_blocked"] != 1 {
		t.Fatalf("bad blocked execution preview counts: %+v", report.Counts)
	}
}

func TestApplyPlanFromEvidenceRejectsWrongKind(t *testing.T) {
	_, err := applyPlanFromEvidence(evidence.Record{Kind: "reconcile-apply-gate", Payload: map[string]interface{}{}})
	if err == nil {
		t.Fatal("expected wrong evidence kind error")
	}
}

func TestBuildReconcileVerificationPlanUsesReadyPreview(t *testing.T) {
	preview := reconcileExecutionPreviewReport{
		Kind:          "meshclaw_reconcile_execution_preview",
		Ready:         true,
		Status:        "ready",
		SchemaVersion: "v3",
		Counts:        map[string]int{"schema_version_present": 1},
		Commands: []reconcileExecutionCommand{{
			Step:      1,
			NodeID:    "g4",
			Operation: "service_reconcile",
			Executor:  "meshclaw-agent",
		}},
	}
	report := buildReconcileVerificationPlanReport("preview.json", evidence.Record{ID: "preview-1", Kind: "reconcile-execution-preview", Payload: preview}, preview)
	if !report.Ready || report.Status != "ready" || len(report.Checks) != 1 {
		t.Fatalf("bad verification plan: %+v", report)
	}
	if report.Counts["verification_checks"] != 1 || report.Counts["verification_plan_ready"] != 1 {
		t.Fatalf("bad verification plan counts: %+v", report.Counts)
	}
	if report.SchemaVersion != "v3" || report.Counts["schema_version_present"] != 1 {
		t.Fatalf("schema version not exposed in verification plan: %+v", report)
	}
	check := report.Checks[0]
	if check.RequiredEvidence != "service-check" || check.Retryable {
		t.Fatalf("bad service verification check: %+v", check)
	}
	if len(check.SuccessCriteria) == 0 || check.FailureAction == "" {
		t.Fatalf("missing verification details: %+v", check)
	}
}

func TestBuildReconcileVerificationPlanUsesContainerEvidence(t *testing.T) {
	preview := reconcileExecutionPreviewReport{
		Kind:   "meshclaw_reconcile_execution_preview",
		Ready:  true,
		Status: "ready",
		Commands: []reconcileExecutionCommand{{
			Step:             1,
			NodeID:           "g4",
			Operation:        "container_reconcile",
			CommandTemplate:  "meshclaw agent apply-step --node g4 --operation container_reconcile --container 'open-webui'",
			VerificationHint: "collect fresh node report, Docker container state, and focused container-logscan evidence after the container action",
			Metadata: map[string]string{
				"container":       "open-webui",
				"desired_state":   "running",
				"actual_state":    "exited",
				"desired_image":   "web:v2",
				"actual_image":    "web:v1",
				"desired_health":  "healthy",
				"actual_health":   "unhealthy",
				"desired_restart": "approval_required",
				"actual_restart":  "no",
			},
		}},
	}
	report := buildReconcileVerificationPlanReport("preview.json", evidence.Record{ID: "preview-1", Kind: "reconcile-execution-preview", Payload: preview}, preview)
	if !report.Ready || len(report.Checks) != 1 {
		t.Fatalf("bad verification plan: %+v", report)
	}
	check := report.Checks[0]
	if check.RequiredEvidence != "agent-collect+container-logscan" || !strings.Contains(strings.Join(check.SuccessCriteria, " "), "Docker container state") {
		t.Fatalf("bad container verification check: %+v", check)
	}
	if check.Metadata["container"] != "open-webui" {
		t.Fatalf("container metadata not propagated to check: %+v", check.Metadata)
	}
	if check.Metadata["desired_state"] != "running" || check.Metadata["desired_image"] != "web:v2" || check.Metadata["desired_health"] != "healthy" {
		t.Fatalf("container desired metadata not propagated to check: %+v", check.Metadata)
	}
	if check.Metadata["actual_state"] != "exited" || check.Metadata["actual_image"] != "web:v1" || check.Metadata["actual_health"] != "unhealthy" {
		t.Fatalf("container actual metadata not propagated to check: %+v", check.Metadata)
	}
	if check.Metadata["desired_restart"] != "approval_required" {
		t.Fatalf("container restart metadata not propagated to check: %+v", check.Metadata)
	}
	if check.Metadata["actual_restart"] != "no" {
		t.Fatalf("container actual restart metadata not propagated to check: %+v", check.Metadata)
	}
	for _, want := range []string{"Docker container state equals running", "Docker container image equals web:v2", "Docker container health equals healthy", "Docker container restart policy equals approval_required"} {
		if !strings.Contains(strings.Join(check.SuccessCriteria, " "), want) {
			t.Fatalf("container desired criteria %q missing from check: %+v", want, check)
		}
	}
	if !strings.Contains(check.FailureAction, "container rollback") {
		t.Fatalf("bad container failure action: %+v", check)
	}
	if report.Counts["container_verification_checks"] != 1 {
		t.Fatalf("bad container verification plan counts: %+v", report.Counts)
	}
}

func TestBuildReconcileVerificationPlanBlocksNotReadyPreview(t *testing.T) {
	preview := reconcileExecutionPreviewReport{
		Kind:    "meshclaw_reconcile_execution_preview",
		Ready:   false,
		Status:  "blocked",
		Reasons: []string{"apply plan is not ready"},
	}
	report := buildReconcileVerificationPlanReport("preview.json", evidence.Record{ID: "preview-1", Kind: "reconcile-execution-preview", Payload: preview}, preview)
	if report.Ready || report.Status != "blocked" || len(report.Checks) != 0 {
		t.Fatalf("not-ready preview should block verification plan: %+v", report)
	}
	if report.Counts["verification_plan_blocked"] != 1 {
		t.Fatalf("bad blocked verification plan counts: %+v", report.Counts)
	}
}

func TestExecutionPreviewFromEvidenceRejectsWrongKind(t *testing.T) {
	_, err := executionPreviewFromEvidence(evidence.Record{Kind: "reconcile-apply-plan", Payload: map[string]interface{}{}})
	if err == nil {
		t.Fatal("expected wrong evidence kind error")
	}
}

func TestBuildReconcileRunbookCombinesCommandsAndChecks(t *testing.T) {
	verification := reconcileVerificationPlanReport{
		Kind:          "meshclaw_reconcile_verification_plan",
		Ready:         true,
		Status:        "ready",
		SchemaVersion: "v3",
		Counts:        map[string]int{"schema_version_present": 1},
		ExecutionPreview: reconcileExecutionPreviewReport{Commands: []reconcileExecutionCommand{{
			Step:            1,
			NodeID:          "g4",
			Operation:       "service_reconcile",
			CommandTemplate: "meshclaw agent apply-step --node g4",
		}}},
		Checks: []reconcileVerificationCheck{{
			Step:             1,
			NodeID:           "g4",
			Operation:        "service_reconcile",
			RequiredEvidence: "service-check",
			SuccessCriteria:  []string{"service active"},
			FailureAction:    "stop loop",
		}},
	}
	report := buildReconcileRunbookReport("verify.json", evidence.Record{ID: "verify-1", Kind: "reconcile-verification-plan", Payload: verification}, verification)
	if !report.Ready || report.Status != "ready" || len(report.Steps) != 1 {
		t.Fatalf("bad runbook: %+v", report)
	}
	if report.Counts["runbook_steps"] != 1 || report.Counts["runbook_ready"] != 1 {
		t.Fatalf("bad runbook counts: %+v", report.Counts)
	}
	if report.SchemaVersion != "v3" || report.Counts["schema_version_present"] != 1 {
		t.Fatalf("schema version not exposed in runbook: %+v", report)
	}
	step := report.Steps[0]
	if step.CommandTemplate == "" || step.RequiredEvidence != "service-check" || !step.RequiresVerification {
		t.Fatalf("bad runbook step: %+v", step)
	}
}

func TestBuildReconcileRunbookCarriesContainerMetadata(t *testing.T) {
	verification := reconcileVerificationPlanReport{
		Kind:   "meshclaw_reconcile_verification_plan",
		Ready:  true,
		Status: "ready",
		ExecutionPreview: reconcileExecutionPreviewReport{Commands: []reconcileExecutionCommand{{
			Step:            1,
			NodeID:          "g4",
			Operation:       "container_reconcile",
			CommandTemplate: "meshclaw agent apply-step --node g4 --operation container_reconcile --container 'open-webui'",
		}}},
		Checks: []reconcileVerificationCheck{{
			Step:             1,
			NodeID:           "g4",
			Operation:        "container_reconcile",
			RequiredEvidence: "agent-collect+container-logscan",
			SuccessCriteria:  []string{"Docker container state matches desired"},
			FailureAction:    "stop loop and create container rollback",
			Metadata: map[string]string{
				"container":       "open-webui",
				"desired_state":   "running",
				"actual_state":    "running",
				"desired_image":   "web:v2",
				"actual_image":    "web:v1",
				"desired_health":  "healthy",
				"actual_health":   "unhealthy",
				"desired_restart": "approval_required",
				"actual_restart":  "no",
			},
		}},
	}
	report := buildReconcileRunbookReport("verify.json", evidence.Record{ID: "verify-1", Kind: "reconcile-verification-plan", Payload: verification}, verification)
	if len(report.Steps) != 1 || report.Steps[0].Metadata["container"] != "open-webui" {
		t.Fatalf("container metadata not propagated to runbook: %+v", report.Steps)
	}
	if report.Steps[0].Metadata["desired_restart"] != "approval_required" {
		t.Fatalf("container restart metadata not propagated to runbook: %+v", report.Steps)
	}
	if report.Steps[0].Metadata["actual_restart"] != "no" {
		t.Fatalf("container actual restart metadata not propagated to runbook: %+v", report.Steps)
	}
	if report.Steps[0].Metadata["desired_image"] != "web:v2" || report.Steps[0].Metadata["actual_image"] != "web:v1" || report.Steps[0].Metadata["desired_health"] != "healthy" || report.Steps[0].Metadata["actual_health"] != "unhealthy" {
		t.Fatalf("container desired/actual metadata not propagated to runbook: %+v", report.Steps)
	}
	if report.Counts["container_runbook_steps"] != 1 {
		t.Fatalf("bad container runbook counts: %+v", report.Counts)
	}
}

func TestBuildReconcileRunbookBlocksNotReadyVerification(t *testing.T) {
	verification := reconcileVerificationPlanReport{
		Kind:    "meshclaw_reconcile_verification_plan",
		Ready:   false,
		Status:  "blocked",
		Reasons: []string{"execution preview is not ready"},
	}
	report := buildReconcileRunbookReport("verify.json", evidence.Record{ID: "verify-1", Kind: "reconcile-verification-plan", Payload: verification}, verification)
	if report.Ready || report.Status != "blocked" || len(report.Steps) != 0 {
		t.Fatalf("not-ready verification should block runbook: %+v", report)
	}
	if report.Counts["runbook_blocked"] != 1 {
		t.Fatalf("bad blocked runbook counts: %+v", report.Counts)
	}
}

func TestBuildReconcileRunbookCheckBlocksContainerStepWithoutMetadata(t *testing.T) {
	runbook := reconcileRunbookReport{
		Kind:   "meshclaw_reconcile_runbook",
		Ready:  true,
		Status: "ready",
		Steps: []reconcileRunbookStep{{
			Step:                 1,
			NodeID:               "g4",
			Operation:            "container_reconcile",
			CommandTemplate:      "meshclaw agent apply-step --node g4 --operation container_reconcile",
			RequiredEvidence:     "agent-collect+container-logscan",
			SuccessCriteria:      []string{"Docker container state matches desired"},
			FailureAction:        "stop loop",
			RequiresVerification: true,
		}},
	}
	report := buildReconcileRunbookCheckReport("runbook.json", evidence.Record{ID: "runbook-1", Kind: "reconcile-runbook", Payload: runbook}, runbook)
	if report.Ready || report.Counts["critical"] == 0 {
		t.Fatalf("container runbook without metadata should be blocked: %+v", report)
	}
	if report.Counts["runbook_findings"] == 0 || report.Counts["runbook_check_blocked"] != 1 {
		t.Fatalf("bad blocked runbook-check counts: %+v", report.Counts)
	}
	if report.Counts["container_runbook_findings"] == 0 {
		t.Fatalf("missing container runbook finding count: %+v", report.Counts)
	}
}

func TestBuildReconcileRunbookCheckBlocksContainerStepWithoutDesiredCriteria(t *testing.T) {
	runbook := reconcileRunbookReport{
		Kind:   "meshclaw_reconcile_runbook",
		Ready:  true,
		Status: "ready",
		Steps: []reconcileRunbookStep{{
			Step:                 1,
			NodeID:               "g4",
			Operation:            "container_reconcile",
			CommandTemplate:      "meshclaw agent apply-step --node g4 --operation container_reconcile --container 'open-webui' --action-id g4:container:open-webui --require-evidence",
			RequiredEvidence:     "agent-collect+container-logscan",
			SuccessCriteria:      []string{"Docker container state equals running"},
			FailureAction:        "stop loop",
			RequiresVerification: true,
			Metadata: map[string]string{
				"container":       "open-webui",
				"desired_state":   "running",
				"desired_image":   "web:v2",
				"desired_health":  "healthy",
				"desired_restart": "approval_required",
			},
		}},
	}
	report := buildReconcileRunbookCheckReport("runbook.json", evidence.Record{ID: "runbook-1", Kind: "reconcile-runbook", Payload: runbook}, runbook)
	if report.Ready || report.Counts["critical"] != 3 {
		t.Fatalf("container runbook without desired criteria should be blocked: %+v", report)
	}
	for _, want := range []string{"desired image", "desired health", "desired restart policy"} {
		if !strings.Contains(fmt.Sprint(report.Findings), want) {
			t.Fatalf("missing desired criteria finding %q: %+v", want, report.Findings)
		}
	}
}

func TestBuildReconcileRunbookCheckBlocksContainerStepWithoutLogscanEvidence(t *testing.T) {
	runbook := reconcileRunbookReport{
		Kind:   "meshclaw_reconcile_runbook",
		Ready:  true,
		Status: "ready",
		Steps: []reconcileRunbookStep{{
			Step:                 1,
			NodeID:               "g4",
			Operation:            "container_reconcile",
			CommandTemplate:      "meshclaw agent apply-step --node g4 --operation container_reconcile --container 'open-webui' --action-id g4:container:open-webui --require-evidence",
			RequiredEvidence:     "agent-collect",
			SuccessCriteria:      []string{"Docker container state equals running"},
			FailureAction:        "stop loop",
			RequiresVerification: true,
			Metadata: map[string]string{
				"container":     "open-webui",
				"desired_state": "running",
			},
		}},
	}
	report := buildReconcileRunbookCheckReport("runbook.json", evidence.Record{ID: "runbook-1", Kind: "reconcile-runbook", Payload: runbook}, runbook)
	if report.Ready || report.Counts["critical"] != 1 {
		t.Fatalf("container runbook without logscan evidence should be blocked: %+v", report)
	}
	if !strings.Contains(fmt.Sprint(report.Findings), "container-logscan") {
		t.Fatalf("missing container-logscan finding: %+v", report.Findings)
	}
	if report.Counts["required_evidence_findings"] != 1 {
		t.Fatalf("bad required-evidence finding count: %+v", report.Counts)
	}
}

func TestBuildReconcileRunbookCheckBlocksStepWithoutRequireEvidenceFlag(t *testing.T) {
	runbook := reconcileRunbookReport{
		Kind:   "meshclaw_reconcile_runbook",
		Ready:  true,
		Status: "ready",
		Steps: []reconcileRunbookStep{{
			Step:                 1,
			NodeID:               "g4",
			Operation:            "service_reconcile",
			CommandTemplate:      "meshclaw agent apply-step --node g4 --operation service_reconcile --action-id g4:service:web",
			RequiredEvidence:     "service-check",
			SuccessCriteria:      []string{"service active"},
			FailureAction:        "stop loop",
			RequiresVerification: true,
		}},
	}
	report := buildReconcileRunbookCheckReport("runbook.json", evidence.Record{ID: "runbook-1", Kind: "reconcile-runbook", Payload: runbook}, runbook)
	if report.Ready || report.Counts["critical"] != 1 {
		t.Fatalf("runbook without require-evidence flag should be blocked: %+v", report)
	}
	if !strings.Contains(fmt.Sprint(report.Findings), "require evidence") {
		t.Fatalf("missing require-evidence finding: %+v", report.Findings)
	}
}

func TestBuildReconcileRunbookCheckBlocksStepWithoutActionIDFlag(t *testing.T) {
	runbook := reconcileRunbookReport{
		Kind:   "meshclaw_reconcile_runbook",
		Ready:  true,
		Status: "ready",
		Steps: []reconcileRunbookStep{{
			Step:                 1,
			NodeID:               "g4",
			Operation:            "service_reconcile",
			CommandTemplate:      "meshclaw agent apply-step --node g4 --operation service_reconcile --require-evidence",
			RequiredEvidence:     "service-check",
			SuccessCriteria:      []string{"service active"},
			FailureAction:        "stop loop",
			RequiresVerification: true,
		}},
	}
	report := buildReconcileRunbookCheckReport("runbook.json", evidence.Record{ID: "runbook-1", Kind: "reconcile-runbook", Payload: runbook}, runbook)
	if report.Ready || report.Counts["critical"] != 1 {
		t.Fatalf("runbook without action-id flag should be blocked: %+v", report)
	}
	if report.Counts["command_template_findings"] != 1 {
		t.Fatalf("bad command-template finding count: %+v", report.Counts)
	}
	if !strings.Contains(fmt.Sprint(report.Findings), "action id") {
		t.Fatalf("missing action-id finding: %+v", report.Findings)
	}
}

func TestBuildReconcileRunbookCheckBlocksStepWithMismatchedNodeFlag(t *testing.T) {
	runbook := reconcileRunbookReport{
		Kind:   "meshclaw_reconcile_runbook",
		Ready:  true,
		Status: "ready",
		Steps: []reconcileRunbookStep{{
			Step:                 1,
			NodeID:               "g4",
			Operation:            "service_reconcile",
			CommandTemplate:      "meshclaw agent apply-step --node s1 --operation service_reconcile --action-id g4:service:web --require-evidence",
			RequiredEvidence:     "service-check",
			SuccessCriteria:      []string{"service active"},
			FailureAction:        "stop loop",
			RequiresVerification: true,
		}},
	}
	report := buildReconcileRunbookCheckReport("runbook.json", evidence.Record{ID: "runbook-1", Kind: "reconcile-runbook", Payload: runbook}, runbook)
	if report.Ready || report.Counts["critical"] != 1 {
		t.Fatalf("runbook with mismatched node flag should be blocked: %+v", report)
	}
	if !strings.Contains(fmt.Sprint(report.Findings), "node id") {
		t.Fatalf("missing node-id finding: %+v", report.Findings)
	}
}

func TestBuildReconcileRunbookCheckBlocksStepWithNodePrefixFlag(t *testing.T) {
	runbook := reconcileRunbookReport{
		Kind:   "meshclaw_reconcile_runbook",
		Ready:  true,
		Status: "ready",
		Steps: []reconcileRunbookStep{{
			Step:                 1,
			NodeID:               "g4",
			Operation:            "service_reconcile",
			CommandTemplate:      "meshclaw agent apply-step --node g4x --operation service_reconcile --action-id g4:service:web --require-evidence",
			RequiredEvidence:     "service-check",
			SuccessCriteria:      []string{"service active"},
			FailureAction:        "stop loop",
			RequiresVerification: true,
		}},
	}
	report := buildReconcileRunbookCheckReport("runbook.json", evidence.Record{ID: "runbook-1", Kind: "reconcile-runbook", Payload: runbook}, runbook)
	if report.Ready || report.Counts["critical"] != 1 {
		t.Fatalf("runbook with node prefix flag should be blocked: %+v", report)
	}
	if !strings.Contains(fmt.Sprint(report.Findings), "node id") {
		t.Fatalf("missing node-id finding: %+v", report.Findings)
	}
}

func TestBuildReconcileRunbookCheckBlocksStepWithMismatchedOperationFlag(t *testing.T) {
	runbook := reconcileRunbookReport{
		Kind:   "meshclaw_reconcile_runbook",
		Ready:  true,
		Status: "ready",
		Steps: []reconcileRunbookStep{{
			Step:                 1,
			NodeID:               "g4",
			Operation:            "service_reconcile",
			CommandTemplate:      "meshclaw agent apply-step --node g4 --operation container_reconcile --action-id g4:service:web --require-evidence",
			RequiredEvidence:     "service-check",
			SuccessCriteria:      []string{"service active"},
			FailureAction:        "stop loop",
			RequiresVerification: true,
		}},
	}
	report := buildReconcileRunbookCheckReport("runbook.json", evidence.Record{ID: "runbook-1", Kind: "reconcile-runbook", Payload: runbook}, runbook)
	if report.Ready || report.Counts["critical"] != 1 {
		t.Fatalf("runbook with mismatched operation flag should be blocked: %+v", report)
	}
	if !strings.Contains(fmt.Sprint(report.Findings), "operation") {
		t.Fatalf("missing operation finding: %+v", report.Findings)
	}
}

func TestBuildReconcileRunbookCheckBlocksContainerStepWithMismatchedContainerFlag(t *testing.T) {
	runbook := reconcileRunbookReport{
		Kind:   "meshclaw_reconcile_runbook",
		Ready:  true,
		Status: "ready",
		Steps: []reconcileRunbookStep{{
			Step:                 1,
			NodeID:               "g4",
			Operation:            "container_reconcile",
			CommandTemplate:      "meshclaw agent apply-step --node g4 --operation container_reconcile --container 'api' --action-id g4:container:open-webui --require-evidence",
			RequiredEvidence:     "agent-collect+container-logscan",
			SuccessCriteria:      []string{"Docker container state equals running"},
			FailureAction:        "stop loop",
			RequiresVerification: true,
			Metadata: map[string]string{
				"container":     "open-webui",
				"desired_state": "running",
			},
		}},
	}
	report := buildReconcileRunbookCheckReport("runbook.json", evidence.Record{ID: "runbook-1", Kind: "reconcile-runbook", Payload: runbook}, runbook)
	if report.Ready || report.Counts["critical"] != 1 {
		t.Fatalf("runbook with mismatched container flag should be blocked: %+v", report)
	}
	if !strings.Contains(fmt.Sprint(report.Findings), "container") {
		t.Fatalf("missing container finding: %+v", report.Findings)
	}
}

func TestBuildReconcileRunbookCheckBlocksStepWithMismatchedActionIDFlag(t *testing.T) {
	runbook := reconcileRunbookReport{
		Kind:   "meshclaw_reconcile_runbook",
		Ready:  true,
		Status: "ready",
		Steps: []reconcileRunbookStep{{
			Step:                 1,
			NodeID:               "g4",
			Operation:            "service_reconcile",
			CommandTemplate:      "meshclaw agent apply-step --node g4 --operation service_reconcile --action-id g4:service:api --require-evidence",
			RequiredEvidence:     "service-check",
			SuccessCriteria:      []string{"service active"},
			FailureAction:        "stop loop",
			RequiresVerification: true,
			Metadata:             map[string]string{"action_id": "g4:service:web"},
		}},
	}
	report := buildReconcileRunbookCheckReport("runbook.json", evidence.Record{ID: "runbook-1", Kind: "reconcile-runbook", Payload: runbook}, runbook)
	if report.Ready || report.Counts["critical"] != 1 {
		t.Fatalf("runbook with mismatched action-id flag should be blocked: %+v", report)
	}
	if !strings.Contains(fmt.Sprint(report.Findings), "action id") {
		t.Fatalf("missing action-id finding: %+v", report.Findings)
	}
}

func TestVerificationPlanFromEvidenceRejectsWrongKind(t *testing.T) {
	_, err := verificationPlanFromEvidence(evidence.Record{Kind: "reconcile-execution-preview", Payload: map[string]interface{}{}})
	if err == nil {
		t.Fatal("expected wrong evidence kind error")
	}
}

func TestBuildReconcileRunbookCheckPassesReadyRunbook(t *testing.T) {
	runbook := reconcileRunbookReport{
		Kind:          "meshclaw_reconcile_runbook",
		Ready:         true,
		Status:        "ready",
		SchemaVersion: "v3",
		Counts:        map[string]int{"runbook_steps": 1, "schema_version_present": 1},
		Steps: []reconcileRunbookStep{{
			Step:                 1,
			NodeID:               "g4",
			Operation:            "service_reconcile",
			CommandTemplate:      "meshclaw agent apply-step --node g4 --operation service_reconcile --action-id g4:service:web --require-evidence",
			RequiredEvidence:     "service-check",
			SuccessCriteria:      []string{"service active"},
			FailureAction:        "stop loop",
			RequiresVerification: true,
			Metadata:             map[string]string{"action_id": "g4:service:web"},
		}},
	}
	report := buildReconcileRunbookCheckReport("runbook.json", evidence.Record{ID: "runbook-1", Kind: "reconcile-runbook", Payload: runbook}, runbook)
	if !report.Ready || report.Status != "ready" || len(report.Findings) != 0 {
		t.Fatalf("ready runbook should pass: %+v", report)
	}
	if report.Counts["runbook_check_ready"] != 1 || report.Counts["runbook_steps"] != 1 {
		t.Fatalf("bad ready runbook-check counts: %+v", report.Counts)
	}
	if report.SchemaVersion != "v3" || report.Counts["schema_version_present"] != 1 {
		t.Fatalf("schema version not exposed in runbook check: %+v", report)
	}
}

func TestBuildReconcileRunbookCheckBlocksMissingFields(t *testing.T) {
	runbook := reconcileRunbookReport{
		Kind:   "meshclaw_reconcile_runbook",
		Ready:  true,
		Status: "ready",
		Steps:  []reconcileRunbookStep{{Step: 1}},
	}
	report := buildReconcileRunbookCheckReport("runbook.json", evidence.Record{ID: "runbook-1", Kind: "reconcile-runbook", Payload: runbook}, runbook)
	if report.Ready || report.Status != "blocked" {
		t.Fatalf("bad runbook should be blocked: %+v", report)
	}
	if report.Counts["critical"] == 0 {
		t.Fatalf("expected critical findings: %+v", report)
	}
	if report.Counts["runbook_check_blocked"] != 1 {
		t.Fatalf("bad missing-field runbook-check counts: %+v", report.Counts)
	}
}

func TestBuildReconcileRunbookCheckCountsMissingSteps(t *testing.T) {
	runbook := reconcileRunbookReport{
		Kind:   "meshclaw_reconcile_runbook",
		Ready:  true,
		Status: "ready",
	}
	report := buildReconcileRunbookCheckReport("runbook.json", evidence.Record{ID: "runbook-1", Kind: "reconcile-runbook", Payload: runbook}, runbook)
	if report.Ready || report.Status != "blocked" || report.Counts["critical"] != 1 {
		t.Fatalf("missing steps should be blocked: %+v", report)
	}
	if report.Counts["steps_findings"] != 1 {
		t.Fatalf("bad steps finding count: %+v", report.Counts)
	}
}

func TestBuildReconcileRunbookCheckCountsNotReadyRunbook(t *testing.T) {
	runbook := reconcileRunbookReport{
		Kind:   "meshclaw_reconcile_runbook",
		Ready:  false,
		Status: "blocked",
		Steps: []reconcileRunbookStep{{
			Step:                 1,
			NodeID:               "g4",
			Operation:            "service_reconcile",
			CommandTemplate:      "meshclaw agent apply-step --node g4 --operation service_reconcile --action-id g4:service:web --require-evidence",
			RequiredEvidence:     "service-check",
			SuccessCriteria:      []string{"service active"},
			FailureAction:        "stop loop",
			RequiresVerification: true,
		}},
	}
	report := buildReconcileRunbookCheckReport("runbook.json", evidence.Record{ID: "runbook-1", Kind: "reconcile-runbook", Payload: runbook}, runbook)
	if report.Ready || report.Status != "blocked" || report.Counts["critical"] != 1 {
		t.Fatalf("not-ready runbook should be blocked: %+v", report)
	}
	if report.Counts["runbook_ready_findings"] != 1 {
		t.Fatalf("bad runbook-ready finding count: %+v", report.Counts)
	}
}

func TestRunbookFromEvidenceRejectsWrongKind(t *testing.T) {
	_, err := runbookFromEvidence(evidence.Record{Kind: "reconcile-verification-plan", Payload: map[string]interface{}{}})
	if err == nil {
		t.Fatal("expected wrong evidence kind error")
	}
}

func TestBuildReconcileRollbackPlanUsesReadyRunbookCheck(t *testing.T) {
	check := reconcileRunbookCheckReport{
		Kind:          "meshclaw_reconcile_runbook_check",
		Ready:         true,
		Status:        "ready",
		SchemaVersion: "v3",
		Counts:        map[string]int{"schema_version_present": 1},
		Runbook: reconcileRunbookReport{Steps: []reconcileRunbookStep{{
			Step:             1,
			NodeID:           "g4",
			Operation:        "service_reconcile",
			RequiredEvidence: "service-check",
			SuccessCriteria:  []string{"service active"},
		}}},
	}
	report := buildReconcileRollbackPlanReport("check.json", evidence.Record{ID: "check-1", Kind: "reconcile-runbook-check", Payload: check}, check)
	if !report.Ready || report.Status != "ready" || len(report.Steps) != 1 {
		t.Fatalf("bad rollback plan: %+v", report)
	}
	if report.Counts["rollback_steps"] != 1 || report.Counts["rollback_plan_ready"] != 1 {
		t.Fatalf("bad rollback plan counts: %+v", report.Counts)
	}
	if report.SchemaVersion != "v3" || report.Counts["schema_version_present"] != 1 {
		t.Fatalf("schema version not exposed in rollback plan: %+v", report)
	}
	if report.Steps[0].RollbackAction == "" || report.Steps[0].RequiredEvidence != "service-check" {
		t.Fatalf("bad rollback step: %+v", report.Steps[0])
	}
}

func TestBuildReconcileRollbackPlanCarriesContainerMetadata(t *testing.T) {
	check := reconcileRunbookCheckReport{
		Kind:   "meshclaw_reconcile_runbook_check",
		Ready:  true,
		Status: "ready",
		Runbook: reconcileRunbookReport{Steps: []reconcileRunbookStep{{
			Step:             1,
			NodeID:           "g4",
			Operation:        "container_reconcile",
			RequiredEvidence: "agent-collect+container-logscan",
			SuccessCriteria:  []string{"Docker container state matches desired"},
			Metadata: map[string]string{
				"container":       "open-webui",
				"desired_state":   "running",
				"actual_state":    "running",
				"desired_image":   "web:v2",
				"actual_image":    "web:v1",
				"desired_health":  "healthy",
				"actual_health":   "unhealthy",
				"desired_restart": "approval_required",
				"actual_restart":  "no",
			},
		}}},
	}
	report := buildReconcileRollbackPlanReport("check.json", evidence.Record{ID: "check-1", Kind: "reconcile-runbook-check", Payload: check}, check)
	if len(report.Steps) != 1 {
		t.Fatalf("rollback steps = %+v", report.Steps)
	}
	step := report.Steps[0]
	if step.Metadata["container"] != "open-webui" || !strings.Contains(step.RollbackAction, "container reconcile") {
		t.Fatalf("bad container rollback step: %+v", step)
	}
	if step.Metadata["desired_restart"] != "approval_required" {
		t.Fatalf("container restart metadata not propagated to rollback: %+v", step)
	}
	if step.Metadata["actual_restart"] != "no" {
		t.Fatalf("container actual restart metadata not propagated to rollback: %+v", step)
	}
	if step.Metadata["desired_image"] != "web:v2" || step.Metadata["actual_image"] != "web:v1" || step.Metadata["desired_health"] != "healthy" || step.Metadata["actual_health"] != "unhealthy" {
		t.Fatalf("container desired/actual metadata not propagated to rollback: %+v", step)
	}
	if report.Counts["container_rollback_steps"] != 1 {
		t.Fatalf("bad container rollback plan counts: %+v", report.Counts)
	}
}

func TestBuildReconcileRollbackPlanBlocksFailedRunbookCheck(t *testing.T) {
	check := reconcileRunbookCheckReport{
		Kind:   "meshclaw_reconcile_runbook_check",
		Ready:  false,
		Status: "blocked",
		Findings: []reconcileCheckFinding{{
			Severity: "critical",
			Message:  "step has no command template",
		}},
	}
	report := buildReconcileRollbackPlanReport("check.json", evidence.Record{ID: "check-1", Kind: "reconcile-runbook-check", Payload: check}, check)
	if report.Ready || report.Status != "blocked" || len(report.Steps) != 0 {
		t.Fatalf("blocked check should block rollback plan: %+v", report)
	}
	if report.Counts["rollback_plan_blocked"] != 1 {
		t.Fatalf("bad blocked rollback plan counts: %+v", report.Counts)
	}
}

func TestRunbookCheckFromEvidenceRejectsWrongKind(t *testing.T) {
	_, err := runbookCheckFromEvidence(evidence.Record{Kind: "reconcile-runbook", Payload: map[string]interface{}{}})
	if err == nil {
		t.Fatal("expected wrong evidence kind error")
	}
}

func TestBuildReconcileCompletionPlanUsesReadyRollbackPlan(t *testing.T) {
	rollback := reconcileRollbackPlanReport{
		Kind:          "meshclaw_reconcile_rollback_plan",
		Ready:         true,
		Status:        "ready",
		SchemaVersion: "v3",
		Counts:        map[string]int{"schema_version_present": 1},
		Steps: []reconcileRollbackStep{{
			Step:             1,
			NodeID:           "g4",
			RequiredEvidence: "service-check",
		}},
	}
	report := buildReconcileCompletionPlanReport("rollback.json", evidence.Record{ID: "rollback-1", Kind: "reconcile-rollback-plan", Payload: rollback}, rollback)
	if !report.Ready || report.Status != "ready" || len(report.Requirements) != 3 {
		t.Fatalf("bad completion plan: %+v", report)
	}
	if report.Counts["completion_requirements"] != 3 || report.Counts["completion_plan_ready"] != 1 {
		t.Fatalf("bad completion plan counts: %+v", report.Counts)
	}
	if report.SchemaVersion != "v3" || report.Counts["schema_version_present"] != 1 {
		t.Fatalf("schema version not exposed in completion plan: %+v", report)
	}
	if report.Requirements[1].RequiredEvidence != "service-check" {
		t.Fatalf("missing step requirement: %+v", report.Requirements)
	}
}

func TestBuildReconcileCompletionPlanCarriesContainerMetadata(t *testing.T) {
	rollback := reconcileRollbackPlanReport{
		Kind:   "meshclaw_reconcile_rollback_plan",
		Ready:  true,
		Status: "ready",
		Steps: []reconcileRollbackStep{{
			Step:             1,
			NodeID:           "g4",
			Operation:        "container_reconcile",
			RequiredEvidence: "agent-collect+container-logscan",
			Metadata: map[string]string{
				"container":       "open-webui",
				"desired_state":   "running",
				"actual_state":    "running",
				"desired_image":   "web:v2",
				"actual_image":    "web:v1",
				"desired_health":  "healthy",
				"actual_health":   "unhealthy",
				"desired_restart": "approval_required",
				"actual_restart":  "no",
			},
		}},
	}
	report := buildReconcileCompletionPlanReport("rollback.json", evidence.Record{ID: "rollback-1", Kind: "reconcile-rollback-plan", Payload: rollback}, rollback)
	if len(report.Requirements) < 2 || report.Requirements[1].Metadata["container"] != "open-webui" {
		t.Fatalf("container metadata not propagated to completion requirements: %+v", report.Requirements)
	}
	if report.Requirements[1].Metadata["desired_restart"] != "approval_required" {
		t.Fatalf("container restart metadata not propagated to completion requirements: %+v", report.Requirements)
	}
	if report.Requirements[1].Metadata["actual_restart"] != "no" {
		t.Fatalf("container actual restart metadata not propagated to completion requirements: %+v", report.Requirements)
	}
	if report.Requirements[1].Metadata["desired_image"] != "web:v2" || report.Requirements[1].Metadata["actual_image"] != "web:v1" || report.Requirements[1].Metadata["desired_health"] != "healthy" || report.Requirements[1].Metadata["actual_health"] != "unhealthy" {
		t.Fatalf("container desired/actual metadata not propagated to completion requirements: %+v", report.Requirements)
	}
	if report.Counts["container_completion_requirements"] != 1 {
		t.Fatalf("bad container completion plan counts: %+v", report.Counts)
	}
}

func TestBuildReconcileCompletionPlanBlocksFailedRollbackPlan(t *testing.T) {
	rollback := reconcileRollbackPlanReport{
		Kind:    "meshclaw_reconcile_rollback_plan",
		Ready:   false,
		Status:  "blocked",
		Reasons: []string{"runbook check is not ready"},
	}
	report := buildReconcileCompletionPlanReport("rollback.json", evidence.Record{ID: "rollback-1", Kind: "reconcile-rollback-plan", Payload: rollback}, rollback)
	if report.Ready || report.Status != "blocked" || len(report.Requirements) != 0 {
		t.Fatalf("blocked rollback should block completion plan: %+v", report)
	}
	if report.Counts["completion_plan_blocked"] != 1 {
		t.Fatalf("bad blocked completion plan counts: %+v", report.Counts)
	}
}

func TestRollbackPlanFromEvidenceRejectsWrongKind(t *testing.T) {
	_, err := rollbackPlanFromEvidence(evidence.Record{Kind: "reconcile-runbook-check", Payload: map[string]interface{}{}})
	if err == nil {
		t.Fatal("expected wrong evidence kind error")
	}
}

func TestBuildReconcileReadinessSummaryUsesReadyCompletionPlan(t *testing.T) {
	completion := reconcileCompletionPlanReport{
		Kind:          "meshclaw_reconcile_completion_plan",
		Ready:         true,
		Status:        "ready",
		SchemaVersion: "v3",
		Counts:        map[string]int{"upstream_marker": 1, "schema_version_present": 1},
		Requirements: []reconcileCompletionRequirement{{
			RequiredEvidence: "reconcile-rollback-plan",
		}, {
			Step:             1,
			NodeID:           "g4",
			RequiredEvidence: "agent-collect+container-logscan",
			Metadata:         map[string]string{"container": "open-webui"},
		}},
		RollbackPlan: reconcileRollbackPlanReport{
			Steps: []reconcileRollbackStep{{Step: 1, NodeID: "g4", Operation: "container_reconcile", Metadata: map[string]string{"container": "open-webui"}}},
			RunbookCheck: reconcileRunbookCheckReport{
				Findings: []reconcileCheckFinding{
					{Severity: "warning", Field: "runbook_ready", Message: "reconcile runbook readiness warning"},
					{Severity: "warning", Field: "steps", Message: "reconcile steps warning"},
					{Severity: "warning", Step: 1, Field: "node_id", Message: "reconcile node warning"},
					{Severity: "warning", Step: 1, Field: "metadata.container", Message: "reconcile container metadata warning"},
					{Severity: "warning", Step: 1, Field: "success_criteria", Message: "container check warning"},
					{Severity: "warning", Step: 1, Field: "command_template", Message: "container command template warning"},
					{Severity: "warning", Step: 1, Field: "required_evidence", Message: "container evidence warning"},
					{Severity: "warning", Step: 1, Field: "failure_action", Message: "container failure action warning"},
					{Severity: "warning", Step: 1, Field: "requires_verification", Message: "container verification warning"},
				},
				Runbook: reconcileRunbookReport{
					Steps: []reconcileRunbookStep{{Step: 1, NodeID: "g4", Operation: "container_reconcile", Metadata: map[string]string{"container": "open-webui"}}},
					VerificationPlan: reconcileVerificationPlanReport{
						Checks: []reconcileVerificationCheck{{Step: 1, NodeID: "g4", Operation: "container_reconcile", Metadata: map[string]string{"container": "open-webui"}}},
						ExecutionPreview: reconcileExecutionPreviewReport{
							Commands: []reconcileExecutionCommand{{Step: 1, NodeID: "g4", Operation: "container_reconcile", Metadata: map[string]string{"container": "open-webui"}}},
							ApplyPlan: reconcileApplyPlanReport{
								Steps: []reconcileApplyStep{{Step: 1, NodeID: "g4", Operation: "container_reconcile", Metadata: map[string]string{"container": "open-webui"}}},
							},
						},
					},
				},
			},
		},
	}
	report := buildReconcileReadinessSummaryReport("completion.json", evidence.Record{ID: "completion-1", Kind: "reconcile-completion-plan", Payload: completion}, completion)
	if !report.Ready || report.Status != "ready" || len(report.ReadyStages) != 9 {
		t.Fatalf("bad readiness summary: %+v", report)
	}
	if report.Counts["completion_requirements"] != 2 || report.Counts["apply_steps"] != 1 {
		t.Fatalf("bad readiness counts: %+v", report.Counts)
	}
	if report.Counts["upstream_marker"] != 1 || report.Counts["readiness_summary_ready"] != 1 {
		t.Fatalf("bad readiness propagated counts: %+v", report.Counts)
	}
	if report.SchemaVersion != "v3" || report.Counts["schema_version_present"] != 1 {
		t.Fatalf("schema version not exposed in readiness summary: %+v", report)
	}
	if report.Counts["readiness_stages"] != len(report.ReadyStages) || report.Counts["readiness_blockers"] != 0 {
		t.Fatalf("bad readiness stage counts: %+v", report.Counts)
	}
	if report.ExecutorGateContract.Decision != "hold_for_operator_approved_reconcile_executor" {
		t.Fatalf("bad executor gate contract: %+v", report.ExecutorGateContract)
	}
	if report.Counts["executor_contract_must_have"] != len(report.ExecutorGateContract.MustHave) || report.Counts["executor_contract_must_not"] != len(report.ExecutorGateContract.MustNot) || report.Counts["executor_contract_refresh_triggers"] != len(report.ExecutorGateContract.RefreshTriggers) {
		t.Fatalf("bad executor contract counts: %+v contract=%+v", report.Counts, report.ExecutorGateContract)
	}
	if !containsString(report.ExecutorGateContract.MustHave, "approval-request and apply-gate evidence") {
		t.Fatalf("executor contract should require approval and apply-gate evidence: %+v", report.ExecutorGateContract)
	}
	if !containsString(report.ExecutorGateContract.MustNot, "execute server changes from summary text") {
		t.Fatalf("executor contract should forbid summary execution: %+v", report.ExecutorGateContract)
	}
	if !containsString(report.ExecutorGateContract.RefreshTriggers, "desired-state YAML changed") {
		t.Fatalf("executor contract should refresh on desired-state changes: %+v", report.ExecutorGateContract)
	}
	if report.Counts["command_template_findings"] != 1 {
		t.Fatalf("bad command-template finding count: %+v", report.Counts)
	}
	if report.Counts["runbook_ready_findings"] != 1 {
		t.Fatalf("bad runbook-ready finding count: %+v", report.Counts)
	}
	if report.Counts["steps_findings"] != 1 {
		t.Fatalf("bad steps finding count: %+v", report.Counts)
	}
	if report.Counts["node_id_findings"] != 1 {
		t.Fatalf("bad node-id finding count: %+v", report.Counts)
	}
	if report.Counts["metadata_container_findings"] != 1 {
		t.Fatalf("bad metadata-container finding count: %+v", report.Counts)
	}
	if report.Counts["required_evidence_findings"] != 1 {
		t.Fatalf("bad required-evidence finding count: %+v", report.Counts)
	}
	if report.Counts["success_criteria_findings"] != 1 {
		t.Fatalf("bad success-criteria finding count: %+v", report.Counts)
	}
	if report.Counts["failure_action_findings"] != 1 {
		t.Fatalf("bad failure-action finding count: %+v", report.Counts)
	}
	if report.Counts["requires_verification_findings"] != 1 {
		t.Fatalf("bad requires-verification finding count: %+v", report.Counts)
	}
	for _, key := range []string{
		"container_apply_steps",
		"container_execution_commands",
		"container_verification_checks",
		"container_runbook_steps",
		"container_runbook_findings",
		"container_rollback_steps",
		"container_completion_requirements",
	} {
		want := 1
		if key == "container_runbook_findings" {
			want = 7
		}
		if report.Counts[key] != want {
			t.Fatalf("bad %s count in %+v", key, report.Counts)
		}
	}
	if !containsString(report.StopBefore, "treating readiness summary as operator approval") || !containsString(report.StopBefore, "skipping approval-request, apply-gate, or verification evidence review") {
		t.Fatalf("readiness summary should expose stop-before gates: %+v", report)
	}
}

func TestBuildReconcileReadinessSummaryBlocksFailedCompletionPlan(t *testing.T) {
	completion := reconcileCompletionPlanReport{
		Kind:    "meshclaw_reconcile_completion_plan",
		Ready:   false,
		Status:  "blocked",
		Reasons: []string{"rollback plan is not ready"},
	}
	report := buildReconcileReadinessSummaryReport("completion.json", evidence.Record{ID: "completion-1", Kind: "reconcile-completion-plan", Payload: completion}, completion)
	if report.Ready || report.Status != "blocked" || len(report.Blockers) != 2 {
		t.Fatalf("blocked completion should block readiness summary: %+v", report)
	}
	if report.Counts["readiness_summary_blocked"] != 1 {
		t.Fatalf("bad blocked readiness counts: %+v", report.Counts)
	}
	if report.Counts["readiness_blockers"] != len(report.Blockers) {
		t.Fatalf("bad blocked readiness blocker counts: %+v", report.Counts)
	}
	if report.Counts["completion_plan_blockers"] != 1 {
		t.Fatalf("bad completion-plan blocker count: %+v", report.Counts)
	}
	if !containsString(report.StopBefore, "starting an executor without ready completion-plan evidence") {
		t.Fatalf("blocked readiness summary should retain executor stop-before gate: %+v", report)
	}
}

func TestBuildReconcileReadinessSummaryBlocksMissingRequirements(t *testing.T) {
	completion := reconcileCompletionPlanReport{
		Kind:   "meshclaw_reconcile_completion_plan",
		Ready:  true,
		Status: "ready",
	}
	report := buildReconcileReadinessSummaryReport("completion.json", evidence.Record{ID: "completion-1", Kind: "reconcile-completion-plan", Payload: completion}, completion)
	if report.Ready || report.Status != "blocked" || len(report.Blockers) != 1 {
		t.Fatalf("missing requirements should block readiness summary: %+v", report)
	}
	if report.Counts["completion_requirements_blockers"] != 1 {
		t.Fatalf("bad completion-requirements blocker count: %+v", report.Counts)
	}
}

func TestReconcileReadinessSummaryCountsLine(t *testing.T) {
	line := reconcileReadinessSummaryCountsLine(reconcileReadinessSummaryReport{Counts: map[string]int{
		"readiness_stages":                  9,
		"readiness_blockers":                0,
		"completion_plan_blockers":          1,
		"completion_requirements_blockers":  1,
		"apply_steps":                       2,
		"container_apply_steps":             1,
		"verification_checks":               2,
		"container_verification_checks":     1,
		"runbook_findings":                  3,
		"container_runbook_findings":        2,
		"runbook_ready_findings":            1,
		"steps_findings":                    1,
		"node_id_findings":                  1,
		"metadata_container_findings":       1,
		"command_template_findings":         1,
		"required_evidence_findings":        1,
		"success_criteria_findings":         1,
		"failure_action_findings":           1,
		"requires_verification_findings":    1,
		"completion_requirements":           4,
		"container_completion_requirements": 1,
	}})
	want := "counts stages=9 blockers=0 completion_plan_blockers=1 completion_requirements_blockers=1 apply_steps=2 container_apply_steps=1 verification_checks=2 container_verification_checks=1 runbook_findings=3 container_runbook_findings=2 runbook_ready_findings=1 steps_findings=1 node_id_findings=1 metadata_container_findings=1 command_template_findings=1 required_evidence_findings=1 success_criteria_findings=1 failure_action_findings=1 requires_verification_findings=1 completion_requirements=4 container_completion_requirements=1"
	if line != want {
		t.Fatalf("counts line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryStagesLine(t *testing.T) {
	line := reconcileReadinessSummaryStagesLine(reconcileReadinessSummaryReport{ReadyStages: []string{
		"approval_request",
		"apply_gate",
		"apply_plan",
	}})
	want := "ready_stages approval_request,apply_gate,apply_plan"
	if line != want {
		t.Fatalf("stages line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryStagesLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryStagesLine(reconcileReadinessSummaryReport{})
	want := "ready_stages <none>"
	if line != want {
		t.Fatalf("empty stages line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummarySafetyLine(t *testing.T) {
	line := reconcileReadinessSummarySafetyLine(reconcileReadinessSummaryReport{Safety: map[string]string{
		"mutation":  "disabled; readiness summary only",
		"execute":   "not implemented",
		"readiness": "future executor must require ready completion-plan evidence",
	}})
	want := "safety mutation=disabled; readiness summary only execute=not implemented readiness=future executor must require ready completion-plan evidence"
	if line != want {
		t.Fatalf("safety line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummarySafetyLineEmpty(t *testing.T) {
	line := reconcileReadinessSummarySafetyLine(reconcileReadinessSummaryReport{})
	want := "safety mutation=<unset> execute=<unset> readiness=<unset>"
	if line != want {
		t.Fatalf("empty safety line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryNextLine(t *testing.T) {
	line := reconcileReadinessSummaryNextLine(reconcileReadinessSummaryReport{Next: []string{
		"Use this summary as the operator-visible readiness checkpoint.",
		"Keep apply disabled until every stage remains ready.",
	}})
	want := "next Use this summary as the operator-visible readiness checkpoint. | Keep apply disabled until every stage remains ready."
	if line != want {
		t.Fatalf("next line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryNextLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryNextLine(reconcileReadinessSummaryReport{})
	want := "next <none>"
	if line != want {
		t.Fatalf("empty next line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryStopBeforeLine(t *testing.T) {
	line := reconcileReadinessSummaryStopBeforeLine(reconcileReadinessSummaryReport{StopBefore: []string{
		"treating readiness summary as operator approval",
		"skipping approval-request, apply-gate, or verification evidence review",
	}})
	want := "stop_before count=2 treating readiness summary as operator approval | skipping approval-request, apply-gate, or verification evidence review"
	if line != want {
		t.Fatalf("stop-before line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryStopBeforeLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryStopBeforeLine(reconcileReadinessSummaryReport{})
	want := "stop_before <none>"
	if line != want {
		t.Fatalf("empty stop-before line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessBlockerLines(t *testing.T) {
	lines := readinessBlockerLines([]string{"completion plan is not ready", "rollback plan is not ready"})
	want := "blockers:\n- completion plan is not ready\n- rollback plan is not ready"
	if strings.Join(lines, "\n") != want {
		t.Fatalf("blocker lines = %q, want %q", strings.Join(lines, "\n"), want)
	}
}

func TestReconcileReadinessSummaryDecisionLineReady(t *testing.T) {
	line := reconcileReadinessSummaryDecisionLine(reconcileReadinessSummaryReport{Ready: true, Status: "ready"})
	want := "decision ready action=hold_for_approval_gated_executor"
	if line != want {
		t.Fatalf("decision line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryDecisionLineBlocked(t *testing.T) {
	line := reconcileReadinessSummaryDecisionLine(reconcileReadinessSummaryReport{Ready: false, Status: "blocked"})
	want := "decision blocked action=resolve_blockers_before_apply"
	if line != want {
		t.Fatalf("blocked decision line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryDecisionLineEmptyStatus(t *testing.T) {
	line := reconcileReadinessSummaryDecisionLine(reconcileReadinessSummaryReport{})
	want := "decision blocked action=resolve_blockers_before_apply"
	if line != want {
		t.Fatalf("empty-status decision line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryExecutorContractLine(t *testing.T) {
	line := reconcileReadinessSummaryExecutorContractLine(reconcileReadinessSummaryReport{
		ExecutorGateContract: executorGateContract{
			Decision:        "hold_for_operator_approved_reconcile_executor",
			MustHave:        []string{"completion evidence", "approval evidence"},
			MustNot:         []string{"execute from summary"},
			RefreshTriggers: []string{"desired-state YAML changed", "policy decision changed"},
		},
	})
	want := "executor_contract decision=hold_for_operator_approved_reconcile_executor must_have=2 must_not=1 refresh_triggers=2"
	if line != want {
		t.Fatalf("executor contract line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryExecutorContractLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryExecutorContractLine(reconcileReadinessSummaryReport{})
	want := "executor_contract decision=<unset> must_have=0 must_not=0 refresh_triggers=0"
	if line != want {
		t.Fatalf("empty executor contract line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryEvidenceLine(t *testing.T) {
	line := reconcileReadinessSummaryEvidenceLine(
		reconcileReadinessSummaryReport{CompletionPlanEvidenceID: "completion-1"},
		evidence.Record{ID: "summary-1"},
	)
	want := "evidence summary=summary-1 completion_plan=completion-1"
	if line != want {
		t.Fatalf("evidence line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryEvidenceLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryEvidenceLine(reconcileReadinessSummaryReport{}, evidence.Record{})
	want := "evidence summary=<unset> completion_plan=<unset>"
	if line != want {
		t.Fatalf("empty evidence line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryEvidencePathsLine(t *testing.T) {
	line := reconcileReadinessSummaryEvidencePathsLine(reconcileReadinessSummaryReport{CompletionPlanEvidencePath: "completion.json"})
	want := "evidence_paths completion_plan=completion.json"
	if line != want {
		t.Fatalf("evidence paths line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryEvidencePathsLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryEvidencePathsLine(reconcileReadinessSummaryReport{})
	want := "evidence_paths completion_plan=<unset>"
	if line != want {
		t.Fatalf("empty evidence paths line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryModeLine(t *testing.T) {
	line := reconcileReadinessSummaryModeLine(reconcileReadinessSummaryReport{
		Mode:   "summary_only",
		Safety: map[string]string{"execute": "not implemented"},
	})
	want := "mode summary_only execute=not implemented"
	if line != want {
		t.Fatalf("mode line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryModeLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryModeLine(reconcileReadinessSummaryReport{})
	want := "mode <unset> execute=<unset>"
	if line != want {
		t.Fatalf("empty mode line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryGeneratedAtLine(t *testing.T) {
	line := reconcileReadinessSummaryGeneratedAtLine(reconcileReadinessSummaryReport{
		GeneratedAt: time.Date(2026, 6, 24, 10, 32, 55, 0, time.UTC),
	})
	want := "generated_at 2026-06-24T10:32:55Z"
	if line != want {
		t.Fatalf("generated-at line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryGeneratedAtLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryGeneratedAtLine(reconcileReadinessSummaryReport{})
	want := "generated_at <unset>"
	if line != want {
		t.Fatalf("empty generated-at line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryOverviewLine(t *testing.T) {
	line := reconcileReadinessSummaryOverviewLine(reconcileReadinessSummaryReport{
		Ready:       true,
		Status:      "ready",
		Mode:        "summary_only",
		ReadyStages: []string{"approval_request", "apply_gate"},
		Blockers:    []string{"blocked"},
	})
	want := "overview ready=true status=ready mode=summary_only stages=2 blockers=1"
	if line != want {
		t.Fatalf("overview line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryOverviewLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryOverviewLine(reconcileReadinessSummaryReport{})
	want := "overview ready=false status=<unset> mode=<unset> stages=0 blockers=0"
	if line != want {
		t.Fatalf("empty overview line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryApplyGateLine(t *testing.T) {
	line := reconcileReadinessSummaryApplyGateLine(reconcileReadinessSummaryReport{
		Ready: true,
		Safety: map[string]string{
			"mutation": "disabled; readiness summary only",
			"execute":  "not implemented",
		},
	})
	want := "apply_gate ready=true mutation=disabled; readiness summary only execute=not implemented evidence=required approval=required"
	if line != want {
		t.Fatalf("apply gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryApplyGateLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryApplyGateLine(reconcileReadinessSummaryReport{})
	want := "apply_gate ready=false mutation=<unset> execute=<unset> evidence=required approval=required"
	if line != want {
		t.Fatalf("empty apply gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryVerificationGateLine(t *testing.T) {
	line := reconcileReadinessSummaryVerificationGateLine(reconcileReadinessSummaryReport{Counts: map[string]int{
		"verification_checks": 2,
		"runbook_findings":    1,
		"critical":            1,
	}})
	want := "verification_gate checks=2 runbook_findings=1 critical_findings=1 post_action_evidence=required"
	if line != want {
		t.Fatalf("verification gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryVerificationGateLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryVerificationGateLine(reconcileReadinessSummaryReport{Counts: map[string]int{}})
	want := "verification_gate checks=0 runbook_findings=0 critical_findings=0 post_action_evidence=required"
	if line != want {
		t.Fatalf("empty verification gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryCompletionGateLine(t *testing.T) {
	line := reconcileReadinessSummaryCompletionGateLine(reconcileReadinessSummaryReport{Counts: map[string]int{
		"completion_requirements":          3,
		"completion_requirements_blockers": 1,
	}})
	want := "completion_gate requirements=3 blockers=1 final_evidence=required"
	if line != want {
		t.Fatalf("completion gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryCompletionGateLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryCompletionGateLine(reconcileReadinessSummaryReport{Counts: map[string]int{}})
	want := "completion_gate requirements=0 blockers=0 final_evidence=required"
	if line != want {
		t.Fatalf("empty completion gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryRollbackGateLine(t *testing.T) {
	line := reconcileReadinessSummaryRollbackGateLine(reconcileReadinessSummaryReport{Counts: map[string]int{
		"rollback_steps":           2,
		"container_rollback_steps": 1,
	}})
	want := "rollback_gate steps=2 container_steps=1 evidence=required operator_review=required"
	if line != want {
		t.Fatalf("rollback gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryRollbackGateLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryRollbackGateLine(reconcileReadinessSummaryReport{Counts: map[string]int{}})
	want := "rollback_gate steps=0 container_steps=0 evidence=required operator_review=required"
	if line != want {
		t.Fatalf("empty rollback gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryPolicyGateLine(t *testing.T) {
	line := reconcileReadinessSummaryPolicyGateLine(reconcileReadinessSummaryReport{Safety: map[string]string{
		"mutation": "disabled; readiness summary only",
	}})
	want := "policy_gate policy=required approval=required evidence=required mutation=disabled; readiness summary only"
	if line != want {
		t.Fatalf("policy gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryPolicyGateLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryPolicyGateLine(reconcileReadinessSummaryReport{})
	want := "policy_gate policy=required approval=required evidence=required mutation=<unset>"
	if line != want {
		t.Fatalf("empty policy gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryFreshnessGateLine(t *testing.T) {
	line := reconcileReadinessSummaryFreshnessGateLine(reconcileReadinessSummaryReport{
		GeneratedAt: time.Date(2026, 6, 24, 11, 42, 58, 0, time.UTC),
	})
	want := "freshness_gate generated_at=2026-06-24T11:42:58Z evidence_present=true refresh_on_input_change=required"
	if line != want {
		t.Fatalf("freshness gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryFreshnessGateLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryFreshnessGateLine(reconcileReadinessSummaryReport{})
	want := "freshness_gate generated_at=<unset> evidence_present=false refresh_on_input_change=required"
	if line != want {
		t.Fatalf("empty freshness gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryOperatorGateLine(t *testing.T) {
	line := reconcileReadinessSummaryOperatorGateLine(reconcileReadinessSummaryReport{
		Blockers: []string{"completion plan is not ready"},
	})
	want := "operator_gate approval=required rollback_review=required blockers=1"
	if line != want {
		t.Fatalf("operator gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryOperatorGateLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryOperatorGateLine(reconcileReadinessSummaryReport{})
	want := "operator_gate approval=required rollback_review=required blockers=0"
	if line != want {
		t.Fatalf("empty operator gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryExecutionGateLine(t *testing.T) {
	line := reconcileReadinessSummaryExecutionGateLine(reconcileReadinessSummaryReport{
		Counts: map[string]int{"apply_steps": 2, "container_apply_steps": 1},
		Safety: map[string]string{"execute": "not implemented"},
	})
	want := "execution_gate apply_steps=2 container_apply_steps=1 execute=not implemented dry_run=required approval=required"
	if line != want {
		t.Fatalf("execution gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryExecutionGateLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryExecutionGateLine(reconcileReadinessSummaryReport{Counts: map[string]int{}})
	want := "execution_gate apply_steps=0 container_apply_steps=0 execute=<unset> dry_run=required approval=required"
	if line != want {
		t.Fatalf("empty execution gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryAuditGateLine(t *testing.T) {
	line := reconcileReadinessSummaryAuditGateLine(reconcileReadinessSummaryReport{
		CompletionPlanEvidenceID: "evidence-456",
		GeneratedAt:              time.Date(2026, 6, 24, 12, 13, 28, 0, time.UTC),
	})
	want := "audit_gate completion_plan=evidence-456 generated_at=2026-06-24T12:13:28Z evidence_store=required final_summary=required"
	if line != want {
		t.Fatalf("audit gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryAuditGateLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryAuditGateLine(reconcileReadinessSummaryReport{})
	want := "audit_gate completion_plan=<unset> generated_at=<unset> evidence_store=required final_summary=required"
	if line != want {
		t.Fatalf("empty audit gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryHandoffGateLine(t *testing.T) {
	line := reconcileReadinessSummaryHandoffGateLine(reconcileReadinessSummaryReport{
		Ready:                    true,
		CompletionPlanEvidenceID: "evidence-456",
		Blockers:                 []string{"operator review pending"},
	})
	want := "handoff_gate ready=true blockers=1 completion_plan=evidence-456 operator_handoff=required"
	if line != want {
		t.Fatalf("handoff gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryHandoffGateLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryHandoffGateLine(reconcileReadinessSummaryReport{})
	want := "handoff_gate ready=false blockers=0 completion_plan=<unset> operator_handoff=required"
	if line != want {
		t.Fatalf("empty handoff gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryPromotionGateLine(t *testing.T) {
	line := reconcileReadinessSummaryPromotionGateLine(reconcileReadinessSummaryReport{
		Ready:       true,
		ReadyStages: []string{"desired_validation", "apply_plan"},
		Blockers:    []string{"operator review pending"},
	})
	want := "promotion_gate ready=true stages=2 blockers=1 executor=disabled approval=required"
	if line != want {
		t.Fatalf("promotion gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryPromotionGateLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryPromotionGateLine(reconcileReadinessSummaryReport{})
	want := "promotion_gate ready=false stages=0 blockers=0 executor=disabled approval=required"
	if line != want {
		t.Fatalf("empty promotion gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryExecutorGateLine(t *testing.T) {
	line := reconcileReadinessSummaryExecutorGateLine(reconcileReadinessSummaryReport{
		Mode:   "summary_only",
		Safety: map[string]string{"execute": "not implemented"},
	})
	want := "executor_gate enabled=false mode=summary_only execute=not implemented dry_run=required approval=required"
	if line != want {
		t.Fatalf("executor gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryExecutorGateLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryExecutorGateLine(reconcileReadinessSummaryReport{})
	want := "executor_gate enabled=false mode=<unset> execute=<unset> dry_run=required approval=required"
	if line != want {
		t.Fatalf("empty executor gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryMCPGateLine(t *testing.T) {
	line := reconcileReadinessSummaryMCPGateLine(reconcileReadinessSummaryReport{
		Kind: "meshclaw_reconcile_readiness_summary",
		Mode: "summary_only",
	})
	want := "mcp_gate kind=meshclaw_reconcile_readiness_summary mode=summary_only evidence=required mutation=disabled"
	if line != want {
		t.Fatalf("mcp gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryMCPGateLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryMCPGateLine(reconcileReadinessSummaryReport{})
	want := "mcp_gate kind=<unset> mode=<unset> evidence=required mutation=disabled"
	if line != want {
		t.Fatalf("empty mcp gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryApprovalGateLine(t *testing.T) {
	line := reconcileReadinessSummaryApprovalGateLine(reconcileReadinessSummaryReport{
		Ready:    true,
		Blockers: []string{"operator review pending"},
	})
	want := "approval_gate required=true ready=true blockers=1 evidence=required"
	if line != want {
		t.Fatalf("approval gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryApprovalGateLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryApprovalGateLine(reconcileReadinessSummaryReport{})
	want := "approval_gate required=true ready=false blockers=0 evidence=required"
	if line != want {
		t.Fatalf("empty approval gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryDryRunGateLine(t *testing.T) {
	line := reconcileReadinessSummaryDryRunGateLine(reconcileReadinessSummaryReport{
		Counts: map[string]int{"apply_steps": 2, "container_apply_steps": 1, "verification_checks": 3},
		Safety: map[string]string{"execute": "not implemented"},
	})
	want := "dry_run_gate required=true apply_steps=2 container_apply_steps=1 verification_checks=3 execute=not implemented"
	if line != want {
		t.Fatalf("dry-run gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryDryRunGateLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryDryRunGateLine(reconcileReadinessSummaryReport{Counts: map[string]int{}})
	want := "dry_run_gate required=true apply_steps=0 container_apply_steps=0 verification_checks=0 execute=<unset>"
	if line != want {
		t.Fatalf("empty dry-run gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryReviewGateLine(t *testing.T) {
	line := reconcileReadinessSummaryReviewGateLine(reconcileReadinessSummaryReport{
		Blockers: []string{"operator review pending"},
	})
	want := "review_gate operator_review=required rollback_review=required blockers=1"
	if line != want {
		t.Fatalf("review gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryReviewGateLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryReviewGateLine(reconcileReadinessSummaryReport{})
	want := "review_gate operator_review=required rollback_review=required blockers=0"
	if line != want {
		t.Fatalf("empty review gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryBlockerGateLine(t *testing.T) {
	line := reconcileReadinessSummaryBlockerGateLine(reconcileReadinessSummaryReport{
		Ready:    false,
		Status:   "blocked",
		Blockers: []string{"operator review pending"},
	})
	want := "blocker_gate ready=false status=blocked blockers=1 resolve_before_apply=required"
	if line != want {
		t.Fatalf("blocker gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryBlockerGateLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryBlockerGateLine(reconcileReadinessSummaryReport{})
	want := "blocker_gate ready=false status=<unset> blockers=0 resolve_before_apply=required"
	if line != want {
		t.Fatalf("empty blocker gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryEvidenceGateLine(t *testing.T) {
	line := reconcileReadinessSummaryEvidenceGateLine(reconcileReadinessSummaryReport{
		CompletionPlanEvidenceID:   "evidence-456",
		CompletionPlanEvidencePath: "evidence/reconcile-completion.json",
	})
	want := "evidence_gate completion_plan=evidence-456 completion_path=evidence/reconcile-completion.json final_evidence=required"
	if line != want {
		t.Fatalf("evidence gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryEvidenceGateLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryEvidenceGateLine(reconcileReadinessSummaryReport{})
	want := "evidence_gate completion_plan=<unset> completion_path=<unset> final_evidence=required"
	if line != want {
		t.Fatalf("empty evidence gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryRollbackReadinessGateLine(t *testing.T) {
	line := reconcileReadinessSummaryRollbackReadinessGateLine(reconcileReadinessSummaryReport{
		Counts: map[string]int{"rollback_steps": 2, "container_rollback_steps": 1},
	})
	want := "rollback_readiness_gate steps=2 container_steps=1 evidence=required operator_review=required"
	if line != want {
		t.Fatalf("rollback readiness gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryRollbackReadinessGateLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryRollbackReadinessGateLine(reconcileReadinessSummaryReport{Counts: map[string]int{}})
	want := "rollback_readiness_gate steps=0 container_steps=0 evidence=required operator_review=required"
	if line != want {
		t.Fatalf("empty rollback readiness gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryVerificationReadinessGateLine(t *testing.T) {
	line := reconcileReadinessSummaryVerificationReadinessGateLine(reconcileReadinessSummaryReport{
		Counts: map[string]int{"verification_checks": 3, "runbook_findings": 1},
	})
	want := "verification_readiness_gate checks=3 runbook_findings=1 post_action_evidence=required"
	if line != want {
		t.Fatalf("verification readiness gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryVerificationReadinessGateLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryVerificationReadinessGateLine(reconcileReadinessSummaryReport{Counts: map[string]int{}})
	want := "verification_readiness_gate checks=0 runbook_findings=0 post_action_evidence=required"
	if line != want {
		t.Fatalf("empty verification readiness gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryCompletionReadinessGateLine(t *testing.T) {
	line := reconcileReadinessSummaryCompletionReadinessGateLine(reconcileReadinessSummaryReport{
		Counts: map[string]int{"completion_requirements": 3, "completion_requirements_blockers": 1},
	})
	want := "completion_readiness_gate requirements=3 blockers=1 final_evidence=required"
	if line != want {
		t.Fatalf("completion readiness gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryCompletionReadinessGateLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryCompletionReadinessGateLine(reconcileReadinessSummaryReport{Counts: map[string]int{}})
	want := "completion_readiness_gate requirements=0 blockers=0 final_evidence=required"
	if line != want {
		t.Fatalf("empty completion readiness gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryStageReadinessGateLine(t *testing.T) {
	line := reconcileReadinessSummaryStageReadinessGateLine(reconcileReadinessSummaryReport{
		ReadyStages: []string{"desired_validation", "apply_plan"},
		Blockers:    []string{"operator review pending"},
	})
	want := "stage_readiness_gate stages=2 blockers=1 refresh_on_input_change=required"
	if line != want {
		t.Fatalf("stage readiness gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryStageReadinessGateLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryStageReadinessGateLine(reconcileReadinessSummaryReport{})
	want := "stage_readiness_gate stages=0 blockers=0 refresh_on_input_change=required"
	if line != want {
		t.Fatalf("empty stage readiness gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryOperatorHandoffGateLine(t *testing.T) {
	line := reconcileReadinessSummaryOperatorHandoffGateLine(reconcileReadinessSummaryReport{
		Blockers: []string{"operator review pending"},
	})
	want := "operator_handoff_gate approval=required review=required evidence=required blockers=1"
	if line != want {
		t.Fatalf("operator handoff gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryOperatorHandoffGateLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryOperatorHandoffGateLine(reconcileReadinessSummaryReport{})
	want := "operator_handoff_gate approval=required review=required evidence=required blockers=0"
	if line != want {
		t.Fatalf("empty operator handoff gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryOperatorApplyGateLine(t *testing.T) {
	line := reconcileReadinessSummaryOperatorApplyGateLine(reconcileReadinessSummaryReport{
		Blockers: []string{"rollback evidence missing"},
	})
	want := "operator_apply_gate approval=required dry_run=required rollback_ready=required blockers=1"
	if line != want {
		t.Fatalf("operator apply gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryOperatorApplyGateLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryOperatorApplyGateLine(reconcileReadinessSummaryReport{})
	want := "operator_apply_gate approval=required dry_run=required rollback_ready=required blockers=0"
	if line != want {
		t.Fatalf("empty operator apply gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryPostApplyGateLine(t *testing.T) {
	line := reconcileReadinessSummaryPostApplyGateLine(reconcileReadinessSummaryReport{
		Counts:   map[string]int{"verification_checks": 3},
		Blockers: []string{"post-apply evidence missing"},
	})
	want := "post_apply_gate verification_checks=3 evidence=required rollback_ready=required blockers=1"
	if line != want {
		t.Fatalf("post apply gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryPostApplyGateLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryPostApplyGateLine(reconcileReadinessSummaryReport{Counts: map[string]int{}})
	want := "post_apply_gate verification_checks=0 evidence=required rollback_ready=required blockers=0"
	if line != want {
		t.Fatalf("empty post apply gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryOperatorResultGateLine(t *testing.T) {
	line := reconcileReadinessSummaryOperatorResultGateLine(reconcileReadinessSummaryReport{
		Counts:   map[string]int{"verification_checks": 3},
		Blockers: []string{"final status missing"},
	})
	want := "operator_result_gate verification_checks=3 evidence=required final_status=required blockers=1"
	if line != want {
		t.Fatalf("operator result gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryOperatorResultGateLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryOperatorResultGateLine(reconcileReadinessSummaryReport{Counts: map[string]int{}})
	want := "operator_result_gate verification_checks=0 evidence=required final_status=required blockers=0"
	if line != want {
		t.Fatalf("empty operator result gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryCloseoutGateLine(t *testing.T) {
	line := reconcileReadinessSummaryCloseoutGateLine(reconcileReadinessSummaryReport{
		Blockers: []string{"final status missing"},
	})
	want := "closeout_gate final_status=required evidence=required rollback_retention=required blockers=1"
	if line != want {
		t.Fatalf("closeout gate line = %q, want %q", line, want)
	}
}

func TestReconcileReadinessSummaryCloseoutGateLineEmpty(t *testing.T) {
	line := reconcileReadinessSummaryCloseoutGateLine(reconcileReadinessSummaryReport{})
	want := "closeout_gate final_status=required evidence=required rollback_retention=required blockers=0"
	if line != want {
		t.Fatalf("empty closeout gate line = %q, want %q", line, want)
	}
}

func TestCompletionPlanFromEvidenceRejectsWrongKind(t *testing.T) {
	_, err := completionPlanFromEvidence(evidence.Record{Kind: "reconcile-rollback-plan", Payload: map[string]interface{}{}})
	if err == nil {
		t.Fatal("expected wrong evidence kind error")
	}
}
