package main

import (
	"strings"
	"testing"
	"time"

	"github.com/meshclaw/meshclaw/internal/evidence"
	"github.com/meshclaw/meshclaw/internal/fleet"
	"github.com/meshclaw/meshclaw/internal/hygiene"
	"github.com/meshclaw/meshclaw/internal/monitor"
	"github.com/meshclaw/meshclaw/internal/nodestate"
	"github.com/meshclaw/meshclaw/internal/workflow"
)

func TestServiceTriagePlanActionsSkipsIgnoreCandidates(t *testing.T) {
	report := serviceTriageReport{
		Items: []serviceTriageItem{
			{
				Host:      "d1",
				Service:   "open-webui",
				Class:     "real_incident",
				Mode:      "inspect",
				Severity:  "warning",
				Judgement: "User service is flapping.",
				Next:      "meshclaw service-check d1 open-webui",
			},
			{
				Host:     "c1",
				Service:  "cloud-init",
				Class:    "ignore_candidate",
				Mode:     "ignore_candidate",
				Severity: "info",
			},
		},
	}

	actions := serviceTriagePlanActions(report)
	if len(actions) != 1 {
		t.Fatalf("actions=%d, want 1", len(actions))
	}
	action := actions[0]
	if action.Type != "service_triage" {
		t.Fatalf("type=%q", action.Type)
	}
	if action.Metric != "service:open-webui" {
		t.Fatalf("metric=%q", action.Metric)
	}
	if action.Command != "meshclaw service-check d1 open-webui" {
		t.Fatalf("command=%q", action.Command)
	}
	if action.Verify != "meshclaw service-check d1 open-webui" {
		t.Fatalf("verify=%q", action.Verify)
	}
}

func TestServiceTriagePlanActionsMarksQuarantineCandidate(t *testing.T) {
	report := serviceTriageReport{
		Items: []serviceTriageItem{{
			Host:      "v1",
			Service:   "old-worker",
			Class:     "stale_or_missing_target",
			Mode:      "approval_required",
			Severity:  "warning",
			Judgement: "ExecStart target is missing.",
			Next:      "meshclaw service-quarantine v1 old-worker",
		}},
	}

	actions := serviceTriagePlanActions(report)
	if len(actions) != 1 {
		t.Fatalf("actions=%d, want 1", len(actions))
	}
	if actions[0].Type != "service_quarantine_candidate" {
		t.Fatalf("type=%q", actions[0].Type)
	}
	if actions[0].Mode != "approval_required" {
		t.Fatalf("mode=%q", actions[0].Mode)
	}
}

func TestClassifyServiceTriageKeepsBootOnlyServicesOutOfQuarantine(t *testing.T) {
	audit := serviceAuditReportForTest("c1", "systemd-networkd-wait-online")
	check := workflowReportForTest("c1", "findings", "ExecStart=/lib/systemd/systemd-networkd-wait-online\nstatus=failed")

	item := classifyServiceTriage(audit, check, "systemd-networkd-wait-online")
	if item.Class != "stale_or_boot_only" {
		t.Fatalf("class=%q", item.Class)
	}
	if item.Mode != "ignore_candidate" {
		t.Fatalf("mode=%q", item.Mode)
	}
}

func TestClassifyServiceTriageMarksExecFailureAsApprovalRequired(t *testing.T) {
	audit := serviceAuditReportForTest("d1", "open-webui")
	check := workflowReportForTest("d1", "findings", "open-webui.service\nActive: activating (auto-restart)\nMain PID: 1 (code=exited, status=203/EXEC)")

	item := classifyServiceTriage(audit, check, "open-webui")
	if item.Class != "stale_or_missing_target" {
		t.Fatalf("class=%q", item.Class)
	}
	if item.Mode != "approval_required" {
		t.Fatalf("mode=%q", item.Mode)
	}
}

func TestAnnotateHealPlanPoliciesAddsDecisionMetadata(t *testing.T) {
	actions := []monitor.HealPlanAction{
		{
			Node:    "d1",
			Type:    "disk_investigate",
			Mode:    "read_only",
			Command: "meshclaw disk-investigate d1 /",
		},
		{
			Node:    "d1",
			Type:    "service_quarantine_candidate",
			Mode:    "approval_required",
			Command: "meshclaw service-quarantine d1 open-webui",
		},
	}

	annotateHealPlanPolicies(actions)

	if actions[0].PolicyAction != "disk_investigate" || actions[0].PolicyDecision != "allow" || actions[0].ApprovalRequired {
		t.Fatalf("disk action policy = %#v", actions[0])
	}
	if actions[1].PolicyAction != "service_quarantine" || actions[1].PolicyDecision != "require_approval" || !actions[1].ApprovalRequired {
		t.Fatalf("service action policy = %#v", actions[1])
	}
}

func TestBuildAutohealPlanIncludesServiceTriageAndPolicy(t *testing.T) {
	m, err := monitor.New(monitor.DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	serviceTriage := serviceTriageReport{
		Items: []serviceTriageItem{{
			Host:      "d1",
			Service:   "open-webui",
			Class:     "stale_or_missing_target",
			Mode:      "approval_required",
			Severity:  "warning",
			Judgement: "ExecStart target is missing.",
			Next:      "meshclaw service-quarantine d1 open-webui",
		}},
	}

	actions := buildAutohealPlan(m, serviceTriage)
	var found bool
	for _, action := range actions {
		if action.Type == "service_quarantine_candidate" && action.Node == "d1" {
			found = true
			if action.PolicyDecision != "require_approval" || !action.ApprovalRequired {
				t.Fatalf("service action missing approval policy metadata: %#v", action)
			}
		}
	}
	if !found {
		t.Fatalf("service triage action not included: %#v", actions)
	}
}

func TestAutohealPlanSummaryLineOK(t *testing.T) {
	line := autohealPlanSummaryLine(autohealPlanReport{})
	want := "autoheal status=ok actions=0"
	if line != want {
		t.Fatalf("autoheal summary line = %q, want %q", line, want)
	}
}

func TestAutohealPlanSummaryLineActions(t *testing.T) {
	line := autohealPlanSummaryLine(autohealPlanReport{
		Actions: []monitor.HealPlanAction{
			{Node: "g4", Type: "container_restart"},
			{Node: "d1", Type: "disk_investigate"},
		},
	})
	want := "autoheal status=actions actions=2"
	if line != want {
		t.Fatalf("autoheal summary line with actions = %q, want %q", line, want)
	}
}

func TestContainerHealthPlanActionsFromStoredReports(t *testing.T) {
	stored := []nodestate.StoredReport{{
		Report: nodestate.Report{
			NodeName: "g4",
			Docker: nodestate.DockerState{
				Available: true,
				Containers: []nodestate.DockerContainer{{
					Name:         "api",
					State:        "running",
					HealthStatus: "unhealthy",
				}},
			},
		},
	}}
	actions := containerHealthPlanActions(stored)
	if len(actions) != 1 {
		t.Fatalf("actions = %+v", actions)
	}
	annotateHealPlanPolicies(actions)
	if actions[0].Node != "g4" || actions[0].Container != "api" || actions[0].Type != "container_restart" {
		t.Fatalf("bad container action: %+v", actions[0])
	}
	if actions[0].Mode != "propose" || actions[0].PolicyDecision != "require_approval" || !actions[0].ApprovalRequired {
		t.Fatalf("container action must remain approval-gated: %+v", actions[0])
	}
	report := buildAutohealPlanReport(actions)
	if report.Counts["mode_propose"] != 1 || report.Counts["policy_require_approval"] != 1 {
		t.Fatalf("bad report counts: %+v", report.Counts)
	}
}

func TestBuildContainerApplyPlanRequiresApproval(t *testing.T) {
	plan := autohealPlanReport{Actions: []monitor.HealPlanAction{{
		Node:             "g4",
		Type:             "container_restart",
		Container:        "api",
		Mode:             "propose",
		Command:          "docker restart 'api'",
		Verify:           "meshclaw agent collect --json",
		PolicyDecision:   "require_approval",
		ApprovalRequired: true,
	}}}
	report := buildContainerApplyPlanReport("plan.json", evidence.Record{ID: "plan-1", Kind: "autoheal-plan", Payload: plan}, plan, "")
	if report.Ready || report.Status != "blocked" || len(report.Steps) != 1 {
		t.Fatalf("missing approved_by should block but preserve steps: %+v", report)
	}
	if !containsString(report.StopBefore, "operator approval if approved_by is empty") {
		t.Fatalf("blocked plan should expose approval stop-before boundary: %+v", report)
	}
	approved := buildContainerApplyPlanReport("plan.json", evidence.Record{ID: "plan-1", Kind: "autoheal-plan", Payload: plan}, plan, "zeus")
	if !approved.Ready || approved.Status != "ready" || len(approved.Steps) != 1 {
		t.Fatalf("approved plan should be ready: %+v", approved)
	}
	if approved.Steps[0].CommandTemplate != "docker restart 'api'" || approved.Steps[0].RequiredEvidence != "agent-collect+container-logscan" {
		t.Fatalf("bad apply step: %+v", approved.Steps[0])
	}
	if approved.Steps[0].RuntimeEvidenceRequired != "fresh runtime inspect/status evidence with image, status, health, ports, and restart policy" {
		t.Fatalf("apply step should expose runtime evidence requirements: %+v", approved.Steps[0])
	}
	if approved.DirectRestartAllowed || !approved.RequiresFocusedRuntimeEvidence || approved.RuntimeEvidenceRequiredCount != 1 {
		t.Fatalf("approved plan should expose direct restart/runtime evidence gate summary: %+v", approved)
	}
	if approved.NextRequiredTool != "meshclaw_autoheal_container_verification_plan" {
		t.Fatalf("approved plan should point to verification as next required tool: %+v", approved)
	}
	if !containsString(approved.StopBefore, "container mutation without agent-collect+container-logscan evidence") || !containsString(approved.StopBefore, "executing command templates; this report is plan-only") {
		t.Fatalf("approved plan should preserve non-execution and evidence stop-before boundaries: %+v", approved)
	}
}

func TestContainerApplyPlanSummaryLineReady(t *testing.T) {
	line := containerApplyPlanSummaryLine(containerApplyPlanReport{
		Ready:          true,
		Status:         "ready",
		PlanEvidenceID: "plan-1",
		Steps:          []containerApplyStep{{Step: 1}},
	})
	want := "container apply-plan ready=true status=ready steps=1 plan=plan-1"
	if line != want {
		t.Fatalf("container apply-plan summary line = %q, want %q", line, want)
	}
}

func TestContainerApplyPlanSummaryLineMissingPlan(t *testing.T) {
	line := containerApplyPlanSummaryLine(containerApplyPlanReport{
		Status: "blocked",
	})
	want := "container apply-plan ready=false status=blocked steps=0 plan=<unset>"
	if line != want {
		t.Fatalf("container apply-plan summary line without plan = %q, want %q", line, want)
	}
}

func TestContainerVerificationPlanSummaryLineReady(t *testing.T) {
	line := containerVerificationPlanSummaryLine(containerVerificationPlanReport{
		Ready:               true,
		Status:              "ready",
		ApplyPlanEvidenceID: "apply-1",
		Checks:              []containerVerificationCheck{{Step: 1}},
	})
	want := "container verification-plan ready=true status=ready checks=1 apply_plan=apply-1"
	if line != want {
		t.Fatalf("container verification-plan summary line = %q, want %q", line, want)
	}
}

func TestContainerVerificationPlanSummaryLineMissingApplyPlan(t *testing.T) {
	line := containerVerificationPlanSummaryLine(containerVerificationPlanReport{
		Status: "blocked",
	})
	want := "container verification-plan ready=false status=blocked checks=0 apply_plan=<unset>"
	if line != want {
		t.Fatalf("container verification-plan summary line without apply plan = %q, want %q", line, want)
	}
}

func TestContainerRunbookSummaryLineReady(t *testing.T) {
	line := containerRunbookSummaryLine(containerRunbookReport{
		Ready:                      true,
		Status:                     "ready",
		VerificationPlanEvidenceID: "verify-1",
		Steps:                      []containerRunbookStep{{Step: 1}},
	})
	want := "container runbook ready=true status=ready steps=1 verification_plan=verify-1"
	if line != want {
		t.Fatalf("container runbook summary line = %q, want %q", line, want)
	}
}

func TestContainerRunbookSummaryLineMissingVerificationPlan(t *testing.T) {
	line := containerRunbookSummaryLine(containerRunbookReport{
		Status: "blocked",
	})
	want := "container runbook ready=false status=blocked steps=0 verification_plan=<unset>"
	if line != want {
		t.Fatalf("container runbook summary line without verification plan = %q, want %q", line, want)
	}
}

func TestContainerRunbookCheckSummaryLineReady(t *testing.T) {
	line := containerRunbookCheckSummaryLine(containerRunbookCheckReport{
		Ready:             true,
		Status:            "ready",
		RunbookEvidenceID: "runbook-1",
		Findings:          []reconcileCheckFinding{{Severity: "warning"}},
	})
	want := "container runbook-check ready=true status=ready findings=1 runbook=runbook-1"
	if line != want {
		t.Fatalf("container runbook-check summary line = %q, want %q", line, want)
	}
}

func TestContainerRunbookCheckSummaryLineMissingRunbook(t *testing.T) {
	line := containerRunbookCheckSummaryLine(containerRunbookCheckReport{
		Status: "blocked",
	})
	want := "container runbook-check ready=false status=blocked findings=0 runbook=<unset>"
	if line != want {
		t.Fatalf("container runbook-check summary line without runbook = %q, want %q", line, want)
	}
}

func TestContainerRollbackPlanSummaryLineReady(t *testing.T) {
	line := containerRollbackPlanSummaryLine(containerRollbackPlanReport{
		Ready:                  true,
		Status:                 "ready",
		RunbookCheckEvidenceID: "check-1",
		Steps:                  []containerRollbackStep{{Step: 1}},
	})
	want := "container rollback-plan ready=true status=ready steps=1 runbook_check=check-1"
	if line != want {
		t.Fatalf("container rollback-plan summary line = %q, want %q", line, want)
	}
}

func TestContainerRollbackPlanSummaryLineMissingRunbookCheck(t *testing.T) {
	line := containerRollbackPlanSummaryLine(containerRollbackPlanReport{
		Status: "blocked",
	})
	want := "container rollback-plan ready=false status=blocked steps=0 runbook_check=<unset>"
	if line != want {
		t.Fatalf("container rollback-plan summary line without runbook check = %q, want %q", line, want)
	}
}

func TestContainerCompletionPlanSummaryLineReady(t *testing.T) {
	line := containerCompletionPlanSummaryLine(containerCompletionPlanReport{
		Ready:                  true,
		Status:                 "ready",
		RollbackPlanEvidenceID: "rollback-1",
		Requirements:           []containerCompletionRequirement{{Step: 1}},
	})
	want := "container completion-plan ready=true status=ready requirements=1 rollback_plan=rollback-1"
	if line != want {
		t.Fatalf("container completion-plan summary line = %q, want %q", line, want)
	}
}

func TestContainerCompletionPlanSummaryLineMissingRollbackPlan(t *testing.T) {
	line := containerCompletionPlanSummaryLine(containerCompletionPlanReport{
		Status: "blocked",
	})
	want := "container completion-plan ready=false status=blocked requirements=0 rollback_plan=<unset>"
	if line != want {
		t.Fatalf("container completion-plan summary line without rollback plan = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummarySummaryLineReady(t *testing.T) {
	line := containerReadinessSummarySummaryLine(containerReadinessSummaryReport{
		Ready:                    true,
		Status:                   "ready",
		CompletionPlanEvidenceID: "completion-1",
		ReadyStages:              []string{"apply_gate"},
		Blockers:                 []string{"manual_review"},
	})
	want := "container readiness-summary ready=true status=ready stages=1 blockers=1 completion_plan=completion-1"
	if line != want {
		t.Fatalf("container readiness-summary line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummarySummaryLineMissingCompletionPlan(t *testing.T) {
	line := containerReadinessSummarySummaryLine(containerReadinessSummaryReport{
		Status: "blocked",
	})
	want := "container readiness-summary ready=false status=blocked stages=0 blockers=0 completion_plan=<unset>"
	if line != want {
		t.Fatalf("container readiness-summary line without completion plan = %q, want %q", line, want)
	}
}

func TestAutohealPlanFromEvidenceRejectsWrongKind(t *testing.T) {
	_, err := autohealPlanFromEvidence(evidence.Record{Kind: "container-apply-plan", Payload: map[string]interface{}{}})
	if err == nil {
		t.Fatal("expected wrong evidence kind error")
	}
}

func TestMCPAutohealContainerApplyPlanRejectsExecute(t *testing.T) {
	_, err := callMCPTool("meshclaw_autoheal_container_apply_plan", map[string]interface{}{"plan_evidence_path": "plan.json", "execute": true})
	if err == nil || !strings.Contains(err.Error(), "builds a plan only") {
		t.Fatalf("expected execute rejection, got %v", err)
	}
}

func TestMCPAutohealContainerApplyPlanReturnsRuntimeGateContract(t *testing.T) {
	plan := autohealPlanReport{Actions: []monitor.HealPlanAction{{
		Node:             "g4",
		Type:             "container_restart",
		Container:        "api",
		Mode:             "propose",
		Command:          "docker restart 'api'",
		Verify:           "meshclaw agent collect --json",
		PolicyDecision:   "require_approval",
		ApprovalRequired: true,
	}}}
	record, err := evidence.Store("autoheal-plan", "g4", "container api", plan)
	if err != nil {
		t.Fatal(err)
	}

	result, err := callMCPTool("meshclaw_autoheal_container_apply_plan", map[string]interface{}{"plan_evidence_path": record.StoredAt, "approved_by": "zeus"})
	if err != nil {
		t.Fatal(err)
	}
	payload := result.(map[string]interface{})
	contract, ok := payload["container_apply_plan_contract"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing container apply-plan contract: %#v", payload)
	}
	if contract["plan_only"] != true || contract["apply_allowed"] != false || contract["mutates_live_servers"] != false {
		t.Fatalf("container apply-plan contract should be plan-only and non-mutating: %#v", contract)
	}
	if contract["direct_restart_allowed"] != false || contract["requires_focused_runtime_evidence"] != true || contract["runtime_evidence_required_count"] != 1 {
		t.Fatalf("container apply-plan contract should expose runtime gate summary: %#v", contract)
	}
	if contract["next_required_tool"] != "meshclaw_autoheal_container_verification_plan" || contract["requires_verification_plan"] != true {
		t.Fatalf("container apply-plan contract should require verification next: %#v", contract)
	}
}

func TestBuildContainerVerificationPlanUsesReadyApplyPlan(t *testing.T) {
	applyPlan := containerApplyPlanReport{
		Kind:                         "meshclaw_container_apply_plan",
		Ready:                        true,
		Status:                       "ready",
		RuntimeEvidenceRequiredCount: 1,
		Steps: []containerApplyStep{{
			Step:                    1,
			NodeID:                  "g4",
			Container:               "api",
			Operation:               "container_restart",
			RuntimeEvidenceRequired: "fresh runtime inspect/status evidence with image, status, health, ports, and restart policy",
		}},
	}
	report := buildContainerVerificationPlanReport("apply.json", evidence.Record{ID: "apply-1", Kind: "container-apply-plan", Payload: applyPlan}, applyPlan)
	if !report.Ready || report.Status != "ready" || len(report.Checks) != 1 {
		t.Fatalf("bad verification plan: %+v", report)
	}
	if report.Checks[0].RequiredEvidence != "agent-collect+container-logscan" || !report.Checks[0].Retryable {
		t.Fatalf("bad verification check: %+v", report.Checks[0])
	}
	if report.Checks[0].RuntimeEvidenceRequired != "fresh runtime inspect/status evidence with image, status, health, ports, and restart policy" {
		t.Fatalf("verification check should retain runtime evidence requirements: %+v", report.Checks[0])
	}
	if report.ApplyPlanRuntimeEvidenceRequiredCount != 1 {
		t.Fatalf("verification plan should retain apply-plan runtime evidence count: %+v", report)
	}
	if !containsString(report.StopBefore, "marking container repair verified without agent-collect+container-logscan evidence") || !containsString(report.StopBefore, "continuing to runbook or completion while verification checks are failing") {
		t.Fatalf("verification plan should expose evidence and failure stop-before gates: %+v", report)
	}
	if strings.Join(report.Checks[0].LogscanPatterns, ",") != strings.Join(containerSelfHealLogscanPatterns(), ",") {
		t.Fatalf("bad verification logscan patterns: %+v", report.Checks[0].LogscanPatterns)
	}
	if len(report.Checks[0].LogscanHints) != len(containerSelfHealLogscanHints()) {
		t.Fatalf("bad verification logscan hints: %+v", report.Checks[0].LogscanHints)
	}
	if report.Checks[0].LogscanHints[0].LikelyCause == "" || report.Checks[0].LogscanHints[0].SuggestedAction == "" {
		t.Fatalf("verification logscan hints should include remediation context: %+v", report.Checks[0].LogscanHints[0])
	}
	criteria := strings.Join(report.Checks[0].SuccessCriteria, " ")
	for _, want := range []string{"healthcheck_failure", "dependency_connection_failure", "disk_full", "port_bind_failure", "image_pull_failure", "permission_failure", "gpu_runtime_failure"} {
		if !strings.Contains(criteria, want) {
			t.Fatalf("verification criteria should include %s: %+v", want, report.Checks[0].SuccessCriteria)
		}
	}
}

func TestApplyAutohealPlanSafeSkipsUnapprovedActions(t *testing.T) {
	actions := applyAutohealPlanSafe([]monitor.HealPlanAction{
		{
			Node:             "g4",
			Type:             "container_restart",
			Mode:             "propose",
			Command:          "docker restart api",
			PolicyAction:     "container_restart",
			PolicyDecision:   "require_approval",
			ApprovalRequired: true,
		},
		{
			Node:           "g4",
			Type:           "disk_investigate",
			Mode:           "read_only",
			Command:        "meshclaw disk-investigate g4 /",
			PolicyAction:   "disk_investigate",
			PolicyDecision: "allow",
		},
	})
	if len(actions) != 2 {
		t.Fatalf("actions = %+v", actions)
	}
	if !actions[0].Skipped || actions[0].SkipReason != "policy does not allow unattended execution" {
		t.Fatalf("approval-required action should be skipped by policy gate: %+v", actions[0])
	}
	if !actions[1].Skipped || actions[1].SkipReason != "plan action is not auto_safe" {
		t.Fatalf("non-auto-safe action should be skipped by mode gate: %+v", actions[1])
	}
	counts := summarizeAutohealApplySafeActions(actions)
	if counts["total"] != 2 || counts["skipped"] != 2 || counts["policy_blocked"] != 1 || counts["mode_blocked"] != 1 || counts["approval_required"] != 1 {
		t.Fatalf("bad apply-safe counts: %+v", counts)
	}
	line := autohealApplySafeCountsLine(counts)
	want := "actions=2 skipped=2 policy_blocked=1 mode_blocked=1 approval_required=1 succeeded=0 failed=0"
	if line != want {
		t.Fatalf("counts line = %q, want %q", line, want)
	}
	payload := autohealApplySafeMCPPayload(actions, counts, evidence.Record{ID: "apply-1"}, nil)
	if got := payload["counts"].(map[string]int); got["policy_blocked"] != 1 || got["mode_blocked"] != 1 {
		t.Fatalf("bad MCP apply-safe counts: %+v", got)
	}
}

func TestBuildContainerVerificationPlanBlocksFailedApplyPlan(t *testing.T) {
	applyPlan := containerApplyPlanReport{
		Kind:    "meshclaw_container_apply_plan",
		Ready:   false,
		Status:  "blocked",
		Reasons: []string{"approved_by is required"},
	}
	report := buildContainerVerificationPlanReport("apply.json", evidence.Record{ID: "apply-1", Kind: "container-apply-plan", Payload: applyPlan}, applyPlan)
	if report.Ready || report.Status != "blocked" || len(report.Checks) != 0 {
		t.Fatalf("blocked apply plan should block verification: %+v", report)
	}
	if !containsString(report.StopBefore, "continuing to runbook or completion while verification checks are failing") {
		t.Fatalf("blocked verification plan should retain stop-before gates: %+v", report)
	}
}

func TestContainerApplyPlanFromEvidenceRejectsWrongKind(t *testing.T) {
	_, err := containerApplyPlanFromEvidence(evidence.Record{Kind: "autoheal-plan", Payload: map[string]interface{}{}})
	if err == nil {
		t.Fatal("expected wrong evidence kind error")
	}
}

func TestMCPAutohealContainerVerificationPlanRejectsExecute(t *testing.T) {
	_, err := callMCPTool("meshclaw_autoheal_container_verification_plan", map[string]interface{}{"container_apply_plan_evidence_path": "apply.json", "execute": true})
	if err == nil || !strings.Contains(err.Error(), "builds verification requirements only") {
		t.Fatalf("expected execute rejection, got %v", err)
	}
}

func TestMCPAutohealContainerVerificationPlanContract(t *testing.T) {
	applyPlan := containerApplyPlanReport{
		Kind:                         "meshclaw_container_apply_plan",
		Ready:                        true,
		Status:                       "ready",
		RuntimeEvidenceRequiredCount: 1,
		Steps: []containerApplyStep{{
			Step:                    1,
			NodeID:                  "g4",
			Container:               "api",
			Operation:               "container_restart",
			RuntimeEvidenceRequired: "fresh runtime inspect/status evidence with image, status, health, ports, and restart policy",
		}},
	}
	record, storeErr := evidence.Store("container-apply-plan", "fleet", "ready=true steps=1", applyPlan)
	if storeErr != nil {
		t.Fatal(storeErr)
	}
	result, err := callMCPTool("meshclaw_autoheal_container_verification_plan", map[string]interface{}{"container_apply_plan_evidence_path": record.StoredAt})
	if err != nil {
		t.Fatalf("verification plan error = %v", err)
	}
	payload := result.(map[string]interface{})
	report := payload["container_verification_plan"].(containerVerificationPlanReport)
	if !report.Ready || report.Status != "ready" || len(report.Checks) != 1 {
		t.Fatalf("bad container verification plan: %+v", report)
	}
	contract, ok := payload["container_verification_plan_contract"].(map[string]interface{})
	if !ok {
		t.Fatalf("container verification contract missing: %#v", payload)
	}
	if contract["plan_only"] != true || contract["apply_allowed"] != false || contract["execute_implemented"] != false || contract["mutates_live_servers"] != false {
		t.Fatalf("container verification contract should be plan-only and non-mutating: %#v", contract)
	}
	if contract["requires_runtime_evidence"] != true || contract["requires_container_logscan"] != true || contract["requires_post_action_evidence"] != true || contract["grants_future_approval"] != false {
		t.Fatalf("container verification contract should require runtime/logscan evidence and grant no approval: %#v", contract)
	}
	if contract["apply_plan_runtime_evidence_required_count"] != 1 {
		t.Fatalf("container verification contract should expose apply-plan runtime evidence count: %#v", contract)
	}
	stopBefore, _ := contract["stop_before"].([]string)
	if !containsString(stopBefore, "marking container repair verified without agent-collect+container-logscan evidence") {
		t.Fatalf("container verification contract should preserve evidence boundary: %#v", contract)
	}
}

func TestBuildContainerRunbookUsesReadyVerificationPlan(t *testing.T) {
	verification := containerVerificationPlanReport{
		Kind:                                  "meshclaw_container_verification_plan",
		Ready:                                 true,
		Status:                                "ready",
		ApplyPlanRuntimeEvidenceRequiredCount: 1,
		ApplyPlan: containerApplyPlanReport{Steps: []containerApplyStep{{
			Step:            1,
			NodeID:          "g4",
			Container:       "api",
			Operation:       "container_restart",
			CommandTemplate: "docker restart 'api'",
		}}},
		Checks: []containerVerificationCheck{{
			Step:                    1,
			NodeID:                  "g4",
			Container:               "api",
			Operation:               "container_restart",
			RequiredEvidence:        "agent-collect+container-logscan",
			RuntimeEvidenceRequired: "fresh runtime inspect/status evidence with image, status, health, ports, and restart policy",
			SuccessCriteria:         []string{"container is running"},
			FailureAction:           "stop",
			Retryable:               true,
		}},
	}
	report := buildContainerRunbookReport("verify.json", evidence.Record{ID: "verify-1", Kind: "container-verification-plan", Payload: verification}, verification)
	if !report.Ready || report.Status != "ready" || len(report.Steps) != 1 {
		t.Fatalf("bad container runbook: %+v", report)
	}
	if report.Steps[0].CommandTemplate != "docker restart 'api'" || !report.Steps[0].RequiresVerification {
		t.Fatalf("bad runbook step: %+v", report.Steps[0])
	}
	if report.Steps[0].RuntimeEvidenceRequired != "fresh runtime inspect/status evidence with image, status, health, ports, and restart policy" {
		t.Fatalf("runbook step should retain runtime evidence requirements: %+v", report.Steps[0])
	}
	if report.ApplyPlanRuntimeEvidenceRequiredCount != 1 {
		t.Fatalf("runbook should retain apply-plan runtime evidence count: %+v", report)
	}
}

func TestBuildContainerRunbookBlocksFailedVerificationPlan(t *testing.T) {
	verification := containerVerificationPlanReport{
		Kind:    "meshclaw_container_verification_plan",
		Ready:   false,
		Status:  "blocked",
		Reasons: []string{"container apply plan is not ready"},
	}
	report := buildContainerRunbookReport("verify.json", evidence.Record{ID: "verify-1", Kind: "container-verification-plan", Payload: verification}, verification)
	if report.Ready || report.Status != "blocked" || len(report.Steps) != 0 {
		t.Fatalf("blocked verification should block runbook: %+v", report)
	}
}

func TestContainerVerificationPlanFromEvidenceRejectsWrongKind(t *testing.T) {
	_, err := containerVerificationPlanFromEvidence(evidence.Record{Kind: "container-apply-plan", Payload: map[string]interface{}{}})
	if err == nil {
		t.Fatal("expected wrong evidence kind error")
	}
}

func TestMCPAutohealContainerRunbookRejectsExecute(t *testing.T) {
	_, err := callMCPTool("meshclaw_autoheal_container_runbook", map[string]interface{}{"container_verification_plan_evidence_path": "verify.json", "execute": true})
	if err == nil || !strings.Contains(err.Error(), "review-only") {
		t.Fatalf("expected execute rejection, got %v", err)
	}
}

func TestMCPAutohealContainerRunbookContract(t *testing.T) {
	verification := containerVerificationPlanReport{
		Kind:                                  "meshclaw_container_verification_plan",
		Ready:                                 true,
		Status:                                "ready",
		ApplyPlanRuntimeEvidenceRequiredCount: 1,
		ApplyPlan: containerApplyPlanReport{Steps: []containerApplyStep{{
			Step:            1,
			NodeID:          "g4",
			Container:       "api",
			Operation:       "container_restart",
			CommandTemplate: "docker restart 'api'",
		}}},
		Checks: []containerVerificationCheck{{
			Step:                    1,
			NodeID:                  "g4",
			Container:               "api",
			Operation:               "container_restart",
			RequiredEvidence:        "agent-collect+container-logscan",
			RuntimeEvidenceRequired: "fresh runtime inspect/status evidence with image, status, health, ports, and restart policy",
			SuccessCriteria:         []string{"container is running"},
			FailureAction:           "stop",
			Retryable:               true,
		}},
	}
	record, storeErr := evidence.Store("container-verification-plan", "fleet", "ready=true checks=1", verification)
	if storeErr != nil {
		t.Fatal(storeErr)
	}
	result, err := callMCPTool("meshclaw_autoheal_container_runbook", map[string]interface{}{"container_verification_plan_evidence_path": record.StoredAt})
	if err != nil {
		t.Fatalf("runbook error = %v", err)
	}
	payload := result.(map[string]interface{})
	runbook := payload["container_runbook"].(containerRunbookReport)
	if !runbook.Ready || runbook.Status != "ready" || len(runbook.Steps) != 1 {
		t.Fatalf("bad container runbook: %+v", runbook)
	}
	contract, ok := payload["container_runbook_contract"].(map[string]interface{})
	if !ok {
		t.Fatalf("container runbook contract missing: %#v", payload)
	}
	if contract["review_only"] != true || contract["apply_allowed"] != false || contract["execute_implemented"] != false || contract["mutates_live_servers"] != false {
		t.Fatalf("container runbook contract should be review-only and non-mutating: %#v", contract)
	}
	if contract["apply_plan_runtime_evidence_required_count"] != 1 {
		t.Fatalf("container runbook contract should expose apply-plan runtime evidence count: %#v", contract)
	}
	if contract["requires_runbook_check"] != true || contract["requires_runtime_evidence"] != true || contract["requires_container_logscan"] != true || contract["requires_post_action_evidence"] != true || contract["grants_future_approval"] != false {
		t.Fatalf("container runbook contract should require checks and grant no approval: %#v", contract)
	}
	stopBefore, _ := contract["stop_before"].([]string)
	if !containsString(stopBefore, "treating container runbook text as docker execution approval") {
		t.Fatalf("container runbook contract should preserve review-only boundary: %#v", contract)
	}
}

func TestBuildContainerRunbookCheckValidatesReadyRunbook(t *testing.T) {
	runbook := containerRunbookReport{
		Kind:   "meshclaw_container_runbook",
		Ready:  true,
		Status: "ready",
		Steps: []containerRunbookStep{{
			Step:                    1,
			NodeID:                  "g4",
			Container:               "api",
			CommandTemplate:         "docker restart 'api'",
			RequiredEvidence:        "agent-collect+container-logscan",
			RuntimeEvidenceRequired: "fresh runtime inspect/status evidence with image, status, health, ports, and restart policy",
			SuccessCriteria:         []string{"container is running"},
			FailureAction:           "stop",
			RequiresVerification:    true,
		}},
	}
	report := buildContainerRunbookCheckReport("runbook.json", evidence.Record{ID: "runbook-1", Kind: "container-runbook", Payload: runbook}, runbook)
	if !report.Ready || report.Status != "ready" || len(report.Findings) != 0 {
		t.Fatalf("bad runbook check: %+v", report)
	}
}

func TestReconcileCommandReferencesValue(t *testing.T) {
	cases := []struct {
		name    string
		command string
		value   string
		want    bool
	}{
		{
			name:    "unquoted value",
			command: "docker restart api",
			value:   "api",
			want:    true,
		},
		{
			name:    "single quoted value",
			command: "docker restart 'api'",
			value:   "api",
			want:    true,
		},
		{
			name:    "double quoted value",
			command: "docker restart \"api\"",
			value:   "api",
			want:    true,
		},
		{
			name:    "prefixed value rejected",
			command: "docker restart api-v2",
			value:   "api",
			want:    false,
		},
		{
			name:    "missing value rejected",
			command: "docker ps",
			value:   "api",
			want:    false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := reconcileCommandReferencesValue(tc.command, tc.value); got != tc.want {
				t.Fatalf("reconcileCommandReferencesValue(%q, %q) = %t, want %t", tc.command, tc.value, got, tc.want)
			}
		})
	}
}

func TestBuildContainerRunbookCheckBlocksInvalidRunbook(t *testing.T) {
	runbook := containerRunbookReport{
		Kind:   "meshclaw_container_runbook",
		Ready:  true,
		Status: "ready",
		Steps:  []containerRunbookStep{{Step: 1, NodeID: "g4"}},
	}
	report := buildContainerRunbookCheckReport("runbook.json", evidence.Record{ID: "runbook-1", Kind: "container-runbook", Payload: runbook}, runbook)
	if report.Ready || report.Status != "blocked" || report.Counts["critical"] == 0 {
		t.Fatalf("invalid runbook should be blocked: %+v", report)
	}
	if report.Counts["command_template_findings"] != 1 {
		t.Fatalf("bad command-template finding count: %+v", report.Counts)
	}
	if report.Counts["container_findings"] != 1 {
		t.Fatalf("bad container finding count: %+v", report.Counts)
	}
}

func TestBuildContainerRunbookCheckCountsMissingSteps(t *testing.T) {
	runbook := containerRunbookReport{
		Kind:   "meshclaw_container_runbook",
		Ready:  true,
		Status: "ready",
	}
	report := buildContainerRunbookCheckReport("runbook.json", evidence.Record{ID: "runbook-1", Kind: "container-runbook", Payload: runbook}, runbook)
	if report.Ready || report.Status != "blocked" || report.Counts["critical"] != 1 {
		t.Fatalf("missing steps should be blocked: %+v", report)
	}
	if report.Counts["steps_findings"] != 1 {
		t.Fatalf("bad steps finding count: %+v", report.Counts)
	}
}

func TestBuildContainerRunbookCheckCountsNotReadyRunbook(t *testing.T) {
	runbook := containerRunbookReport{
		Kind:   "meshclaw_container_runbook",
		Ready:  false,
		Status: "blocked",
		Steps: []containerRunbookStep{{
			Step:                    1,
			NodeID:                  "g4",
			Container:               "api",
			CommandTemplate:         "docker restart 'api'",
			RequiredEvidence:        "agent-collect+container-logscan",
			RuntimeEvidenceRequired: "fresh runtime inspect/status evidence with image, status, health, ports, and restart policy",
			SuccessCriteria:         []string{"container is running"},
			FailureAction:           "stop",
			RequiresVerification:    true,
		}},
	}
	report := buildContainerRunbookCheckReport("runbook.json", evidence.Record{ID: "runbook-1", Kind: "container-runbook", Payload: runbook}, runbook)
	if report.Ready || report.Status != "blocked" || report.Counts["critical"] != 1 {
		t.Fatalf("not-ready runbook should be blocked: %+v", report)
	}
	if report.Counts["runbook_ready_findings"] != 1 {
		t.Fatalf("bad runbook-ready finding count: %+v", report.Counts)
	}
}

func TestBuildContainerRunbookCheckBlocksMismatchedContainerCommand(t *testing.T) {
	runbook := containerRunbookReport{
		Kind:   "meshclaw_container_runbook",
		Ready:  true,
		Status: "ready",
		Steps: []containerRunbookStep{{
			Step:                    1,
			NodeID:                  "g4",
			Container:               "api",
			CommandTemplate:         "docker restart 'web'",
			RequiredEvidence:        "agent-collect+container-logscan",
			RuntimeEvidenceRequired: "fresh runtime inspect/status evidence with image, status, health, ports, and restart policy",
			SuccessCriteria:         []string{"container is running"},
			FailureAction:           "stop",
			RequiresVerification:    true,
		}},
	}
	report := buildContainerRunbookCheckReport("runbook.json", evidence.Record{ID: "runbook-1", Kind: "container-runbook", Payload: runbook}, runbook)
	if report.Ready || report.Status != "blocked" || report.Counts["critical"] != 1 {
		t.Fatalf("mismatched container command should be blocked: %+v", report)
	}
	if report.Counts["command_template_findings"] != 1 {
		t.Fatalf("bad command-template finding count: %+v", report.Counts)
	}
}

func TestBuildContainerRunbookCheckBlocksMissingLogscanEvidence(t *testing.T) {
	runbook := containerRunbookReport{
		Kind:   "meshclaw_container_runbook",
		Ready:  true,
		Status: "ready",
		Steps: []containerRunbookStep{{
			Step:                    1,
			NodeID:                  "g4",
			Container:               "api",
			CommandTemplate:         "docker restart 'api'",
			RequiredEvidence:        "agent-collect",
			RuntimeEvidenceRequired: "fresh runtime inspect/status evidence with image, status, health, ports, and restart policy",
			SuccessCriteria:         []string{"container is running"},
			FailureAction:           "stop",
			RequiresVerification:    true,
		}},
	}
	report := buildContainerRunbookCheckReport("runbook.json", evidence.Record{ID: "runbook-1", Kind: "container-runbook", Payload: runbook}, runbook)
	if report.Ready || report.Status != "blocked" || report.Counts["critical"] != 1 {
		t.Fatalf("missing logscan evidence should be blocked: %+v", report)
	}
	if len(report.Findings) != 1 || !strings.Contains(report.Findings[0].Message, "container-logscan") {
		t.Fatalf("missing container-logscan finding: %+v", report.Findings)
	}
	if report.Counts["required_evidence_findings"] != 1 {
		t.Fatalf("bad required-evidence finding count: %+v", report.Counts)
	}
}

func TestBuildContainerRunbookCheckBlocksMissingRuntimeEvidence(t *testing.T) {
	runbook := containerRunbookReport{
		Kind:   "meshclaw_container_runbook",
		Ready:  true,
		Status: "ready",
		Steps: []containerRunbookStep{{
			Step:                 1,
			NodeID:               "g4",
			Container:            "api",
			CommandTemplate:      "docker restart 'api'",
			RequiredEvidence:     "agent-collect+container-logscan",
			SuccessCriteria:      []string{"container is running"},
			FailureAction:        "stop",
			RequiresVerification: true,
		}},
	}
	report := buildContainerRunbookCheckReport("runbook.json", evidence.Record{ID: "runbook-1", Kind: "container-runbook", Payload: runbook}, runbook)
	if report.Ready || report.Status != "blocked" || report.Counts["critical"] != 1 {
		t.Fatalf("missing runtime evidence should be blocked: %+v", report)
	}
	if len(report.Findings) != 1 || report.Findings[0].Field != "runtime_evidence_required" {
		t.Fatalf("missing runtime evidence finding: %+v", report.Findings)
	}
	if report.Counts["runtime_evidence_findings"] != 1 {
		t.Fatalf("bad runtime-evidence finding count: %+v", report.Counts)
	}
}

func TestContainerRunbookFromEvidenceRejectsWrongKind(t *testing.T) {
	_, err := containerRunbookFromEvidence(evidence.Record{Kind: "container-verification-plan", Payload: map[string]interface{}{}})
	if err == nil {
		t.Fatal("expected wrong evidence kind error")
	}
}

func TestMCPAutohealContainerRunbookCheckRejectsExecute(t *testing.T) {
	_, err := callMCPTool("meshclaw_autoheal_container_runbook_check", map[string]interface{}{"container_runbook_evidence_path": "runbook.json", "execute": true})
	if err == nil || !strings.Contains(err.Error(), "validates runbooks only") {
		t.Fatalf("expected execute rejection, got %v", err)
	}
}

func TestMCPAutohealContainerRunbookCheckContract(t *testing.T) {
	runbook := containerRunbookReport{
		Kind:   "meshclaw_container_runbook",
		Ready:  true,
		Status: "ready",
		Steps: []containerRunbookStep{{
			Step:                    1,
			NodeID:                  "g4",
			Container:               "api",
			CommandTemplate:         "docker restart 'api'",
			RequiredEvidence:        "agent-collect+container-logscan",
			RuntimeEvidenceRequired: "fresh runtime inspect/status evidence with image, status, health, ports, and restart policy",
			SuccessCriteria:         []string{"container is running"},
			FailureAction:           "stop",
			RequiresVerification:    true,
		}},
	}
	record, storeErr := evidence.Store("container-runbook", "fleet", "ready=true steps=1", runbook)
	if storeErr != nil {
		t.Fatal(storeErr)
	}
	result, err := callMCPTool("meshclaw_autoheal_container_runbook_check", map[string]interface{}{"container_runbook_evidence_path": record.StoredAt})
	if err != nil {
		t.Fatalf("runbook check error = %v", err)
	}
	payload := result.(map[string]interface{})
	check := payload["container_runbook_check"].(containerRunbookCheckReport)
	if !check.Ready || check.Status != "ready" || len(check.Findings) != 0 {
		t.Fatalf("bad container runbook check: %+v", check)
	}
	contract, ok := payload["container_runbook_check_contract"].(map[string]interface{})
	if !ok {
		t.Fatalf("container runbook check contract missing: %#v", payload)
	}
	if contract["gate_only"] != true || contract["apply_allowed"] != false || contract["execute_implemented"] != false || contract["mutates_live_servers"] != false {
		t.Fatalf("container runbook check contract should be gate-only and non-mutating: %#v", contract)
	}
	if contract["requires_zero_critical_findings"] != true || contract["critical_findings"] != 0 || contract["requires_runtime_evidence"] != true || contract["requires_container_logscan"] != true || contract["grants_future_approval"] != false {
		t.Fatalf("container runbook check contract should require evidence and zero critical findings: %#v", contract)
	}
	if contract["requires_rollback_plan"] != true || contract["requires_completion_plan"] != true {
		t.Fatalf("container runbook check contract should require rollback and completion planning: %#v", contract)
	}
	stopBefore, _ := contract["stop_before"].([]string)
	if !containsString(stopBefore, "treating container runbook-check readiness as docker execution approval") {
		t.Fatalf("container runbook check contract should preserve gate-only boundary: %#v", contract)
	}
}

func TestBuildContainerRollbackPlanUsesReadyRunbookCheck(t *testing.T) {
	check := containerRunbookCheckReport{
		Kind:   "meshclaw_container_runbook_check",
		Ready:  true,
		Status: "ready",
		Runbook: containerRunbookReport{
			Steps: []containerRunbookStep{{
				Step:                    1,
				NodeID:                  "g4",
				Container:               "api",
				Operation:               "container_restart",
				RequiredEvidence:        "agent-collect+container-logscan",
				RuntimeEvidenceRequired: "fresh runtime inspect/status evidence with image, status, health, ports, and restart policy",
				SuccessCriteria:         []string{"container is running"},
			}},
		},
	}
	report := buildContainerRollbackPlanReport("check.json", evidence.Record{ID: "check-1", Kind: "container-runbook-check", Payload: check}, check)
	if !report.Ready || report.Status != "ready" || len(report.Steps) != 1 {
		t.Fatalf("bad rollback plan: %+v", report)
	}
	step := report.Steps[0]
	if step.NodeID != "g4" || step.Container != "api" || !strings.Contains(step.RollbackAction, "operator-approved") {
		t.Fatalf("bad rollback step: %+v", step)
	}
	if step.RuntimeEvidenceRequired != "fresh runtime inspect/status evidence with image, status, health, ports, and restart policy" {
		t.Fatalf("rollback step should retain runtime evidence requirements: %+v", step)
	}
}

func TestBuildContainerRollbackPlanBlocksFailedRunbookCheck(t *testing.T) {
	check := containerRunbookCheckReport{
		Kind:   "meshclaw_container_runbook_check",
		Ready:  false,
		Status: "blocked",
		Findings: []reconcileCheckFinding{{
			Severity: "critical",
			Step:     1,
			Field:    "command_template",
			Message:  "step has no command template",
		}},
	}
	report := buildContainerRollbackPlanReport("check.json", evidence.Record{ID: "check-1", Kind: "container-runbook-check", Payload: check}, check)
	if report.Ready || report.Status != "blocked" || len(report.Steps) != 0 || len(report.Reasons) < 2 {
		t.Fatalf("failed check should block rollback plan: %+v", report)
	}
}

func TestContainerRunbookCheckFromEvidenceRejectsWrongKind(t *testing.T) {
	_, err := containerRunbookCheckFromEvidence(evidence.Record{Kind: "container-runbook", Payload: map[string]interface{}{}})
	if err == nil {
		t.Fatal("expected wrong evidence kind error")
	}
}

func TestMCPAutohealContainerRollbackPlanRejectsExecute(t *testing.T) {
	_, err := callMCPTool("meshclaw_autoheal_container_rollback_plan", map[string]interface{}{"container_runbook_check_evidence_path": "check.json", "execute": true})
	if err == nil || !strings.Contains(err.Error(), "rollback guidance only") {
		t.Fatalf("expected execute rejection, got %v", err)
	}
}

func TestMCPAutohealContainerRollbackPlanContract(t *testing.T) {
	check := containerRunbookCheckReport{
		Kind:   "meshclaw_container_runbook_check",
		Ready:  true,
		Status: "ready",
		Runbook: containerRunbookReport{
			Steps: []containerRunbookStep{{
				Step:                    1,
				NodeID:                  "g4",
				Container:               "api",
				Operation:               "container_restart",
				RequiredEvidence:        "agent-collect+container-logscan",
				RuntimeEvidenceRequired: "fresh runtime inspect/status evidence with image, status, health, ports, and restart policy",
				SuccessCriteria:         []string{"container is running"},
			}},
		},
	}
	record, storeErr := evidence.Store("container-runbook-check", "fleet", "ready=true findings=0", check)
	if storeErr != nil {
		t.Fatal(storeErr)
	}
	result, err := callMCPTool("meshclaw_autoheal_container_rollback_plan", map[string]interface{}{"container_runbook_check_evidence_path": record.StoredAt})
	if err != nil {
		t.Fatalf("rollback plan error = %v", err)
	}
	payload := result.(map[string]interface{})
	rollback := payload["container_rollback_plan"].(containerRollbackPlanReport)
	if !rollback.Ready || rollback.Status != "ready" || len(rollback.Steps) != 1 {
		t.Fatalf("bad container rollback plan: %+v", rollback)
	}
	contract, ok := payload["container_rollback_plan_contract"].(map[string]interface{})
	if !ok {
		t.Fatalf("container rollback contract missing: %#v", payload)
	}
	if contract["plan_only"] != true || contract["apply_allowed"] != false || contract["rollback_allowed"] != false || contract["execute_implemented"] != false || contract["mutates_live_servers"] != false {
		t.Fatalf("container rollback contract should be plan-only and non-mutating: %#v", contract)
	}
	if contract["requires_operator_approval"] != true || contract["requires_runtime_evidence"] != true || contract["requires_container_logscan"] != true || contract["requires_completion_plan"] != true || contract["grants_future_approval"] != false {
		t.Fatalf("container rollback contract should require approval/evidence/completion and grant no approval: %#v", contract)
	}
	stopBefore, _ := contract["stop_before"].([]string)
	if !containsString(stopBefore, "treating container rollback plan as automatic docker rollback execution") {
		t.Fatalf("container rollback contract should preserve plan-only boundary: %#v", contract)
	}
}

func TestBuildContainerCompletionPlanUsesReadyRollbackPlan(t *testing.T) {
	rollback := containerRollbackPlanReport{
		Kind:   "meshclaw_container_rollback_plan",
		Ready:  true,
		Status: "ready",
		Steps: []containerRollbackStep{{
			Step:                    1,
			NodeID:                  "g4",
			Container:               "api",
			RequiredEvidence:        "agent-collect+container-logscan",
			RuntimeEvidenceRequired: "fresh runtime inspect/status evidence with image, status, health, ports, and restart policy",
		}},
	}
	report := buildContainerCompletionPlanReport("rollback.json", evidence.Record{ID: "rollback-1", Kind: "container-rollback-plan", Payload: rollback}, rollback)
	if !report.Ready || report.Status != "ready" || len(report.Requirements) != 3 {
		t.Fatalf("bad completion plan: %+v", report)
	}
	if report.Requirements[1].Container != "api" || !strings.Contains(strings.Join(report.Requirements[1].Criteria, " "), "log finding") {
		t.Fatalf("bad container completion requirement: %+v", report.Requirements[1])
	}
	if report.Requirements[1].RuntimeEvidenceRequired != "fresh runtime inspect/status evidence with image, status, health, ports, and restart policy" {
		t.Fatalf("completion requirement should retain runtime evidence requirements: %+v", report.Requirements[1])
	}
}

func TestBuildContainerCompletionPlanBlocksFailedRollbackPlan(t *testing.T) {
	rollback := containerRollbackPlanReport{
		Kind:    "meshclaw_container_rollback_plan",
		Ready:   false,
		Status:  "blocked",
		Reasons: []string{"container runbook-check is not ready"},
	}
	report := buildContainerCompletionPlanReport("rollback.json", evidence.Record{ID: "rollback-1", Kind: "container-rollback-plan", Payload: rollback}, rollback)
	if report.Ready || report.Status != "blocked" || len(report.Requirements) != 0 || len(report.Reasons) < 2 {
		t.Fatalf("failed rollback should block completion plan: %+v", report)
	}
}

func TestContainerRollbackPlanFromEvidenceRejectsWrongKind(t *testing.T) {
	_, err := containerRollbackPlanFromEvidence(evidence.Record{Kind: "container-runbook-check", Payload: map[string]interface{}{}})
	if err == nil {
		t.Fatal("expected wrong evidence kind error")
	}
}

func TestMCPAutohealContainerCompletionPlanRejectsExecute(t *testing.T) {
	_, err := callMCPTool("meshclaw_autoheal_container_completion_plan", map[string]interface{}{"container_rollback_plan_evidence_path": "rollback.json", "execute": true})
	if err == nil || !strings.Contains(err.Error(), "completion requirements only") {
		t.Fatalf("expected execute rejection, got %v", err)
	}
}

func TestMCPAutohealContainerCompletionPlanContract(t *testing.T) {
	rollback := containerRollbackPlanReport{
		Kind:   "meshclaw_container_rollback_plan",
		Ready:  true,
		Status: "ready",
		Steps: []containerRollbackStep{{
			Step:                    1,
			NodeID:                  "g4",
			Container:               "api",
			RequiredEvidence:        "agent-collect+container-logscan",
			RuntimeEvidenceRequired: "fresh runtime inspect/status evidence with image, status, health, ports, and restart policy",
		}},
	}
	record, storeErr := evidence.Store("container-rollback-plan", "fleet", "ready=true steps=1", rollback)
	if storeErr != nil {
		t.Fatal(storeErr)
	}
	result, err := callMCPTool("meshclaw_autoheal_container_completion_plan", map[string]interface{}{"container_rollback_plan_evidence_path": record.StoredAt})
	if err != nil {
		t.Fatalf("completion plan error = %v", err)
	}
	payload := result.(map[string]interface{})
	completion := payload["container_completion_plan"].(containerCompletionPlanReport)
	if !completion.Ready || completion.Status != "ready" || len(completion.Requirements) == 0 {
		t.Fatalf("bad container completion plan: %+v", completion)
	}
	contract, ok := payload["container_completion_plan_contract"].(map[string]interface{})
	if !ok {
		t.Fatalf("container completion contract missing: %#v", payload)
	}
	if contract["plan_only"] != true || contract["apply_allowed"] != false || contract["complete_allowed"] != false || contract["execute_implemented"] != false || contract["mutates_live_servers"] != false {
		t.Fatalf("container completion contract should be plan-only and non-mutating: %#v", contract)
	}
	if contract["requires_final_evidence"] != true || contract["requires_runtime_evidence"] != true || contract["requires_container_logscan"] != true || contract["requires_readiness_summary"] != true || contract["grants_future_approval"] != false {
		t.Fatalf("container completion contract should require final evidence/readiness and grant no approval: %#v", contract)
	}
	if contract["completion_requirements"] != len(completion.Requirements) {
		t.Fatalf("container completion contract should count requirements: %#v", contract)
	}
	stopBefore, _ := contract["stop_before"].([]string)
	if !containsString(stopBefore, "declaring container repair complete from completion-plan evidence") {
		t.Fatalf("container completion contract should preserve completion boundary: %#v", contract)
	}
}

func TestBuildContainerReadinessSummaryUsesReadyCompletionPlan(t *testing.T) {
	completion := containerCompletionPlanReport{
		Kind:   "meshclaw_container_completion_plan",
		Ready:  true,
		Status: "ready",
		Requirements: []containerCompletionRequirement{{
			RequiredEvidence: "container-rollback-plan",
			LogscanPatterns:  containerSelfHealLogscanPatterns(),
			LogscanHints:     containerSelfHealLogscanHints(),
		}},
		RollbackPlan: containerRollbackPlanReport{
			Steps: []containerRollbackStep{{Step: 1, NodeID: "g4", Container: "api"}},
			RunbookCheck: containerRunbookCheckReport{
				Findings: []reconcileCheckFinding{
					{Severity: "warning", Field: "runbook_ready", Message: "container runbook readiness warning"},
					{Severity: "warning", Field: "steps", Message: "container steps warning"},
					{Severity: "warning", Step: 1, Field: "node_id", Message: "container node warning"},
					{Severity: "warning", Step: 1, Field: "container", Message: "container target warning"},
					{Severity: "warning", Step: 1, Field: "command_template", Message: "container command template warning"},
					{Severity: "warning", Step: 1, Field: "required_evidence", Message: "container evidence warning"},
					{Severity: "warning", Step: 1, Field: "success_criteria", Message: "container success criteria warning"},
					{Severity: "warning", Step: 1, Field: "failure_action", Message: "container failure action warning"},
					{Severity: "warning", Step: 1, Field: "requires_verification", Message: "container verification warning"},
				},
				Runbook: containerRunbookReport{
					Steps: []containerRunbookStep{{Step: 1, NodeID: "g4", Container: "api"}},
					VerificationPlan: containerVerificationPlanReport{
						Checks: []containerVerificationCheck{{Step: 1, NodeID: "g4", Container: "api", LogscanPatterns: containerSelfHealLogscanPatterns(), LogscanHints: containerSelfHealLogscanHints()}},
						ApplyPlan: containerApplyPlanReport{
							Steps: []containerApplyStep{{Step: 1, NodeID: "g4", Container: "api"}},
						},
					},
				},
			},
		},
	}
	report := buildContainerReadinessSummaryReport("completion.json", evidence.Record{ID: "completion-1", Kind: "container-completion-plan", Payload: completion}, completion)
	if !report.Ready || report.Status != "ready" || len(report.ReadyStages) != 7 {
		t.Fatalf("bad readiness summary: %+v", report)
	}
	if report.Counts["completion_requirements"] != 1 || report.Counts["apply_steps"] != 1 {
		t.Fatalf("bad readiness counts: %+v", report.Counts)
	}
	if report.Counts["logscan_patterns"] != len(containerSelfHealLogscanPatterns()) {
		t.Fatalf("bad logscan pattern count: %+v", report.Counts)
	}
	if strings.Join(report.LogscanPatterns, ",") != strings.Join(containerSelfHealLogscanPatterns(), ",") {
		t.Fatalf("bad readiness logscan patterns: %+v", report.LogscanPatterns)
	}
	if report.Counts["logscan_hints"] != len(containerSelfHealLogscanHints()) {
		t.Fatalf("bad logscan hint count: %+v", report.Counts)
	}
	if report.Counts["apply_loop_gates"] != len(containerApplyLoopGates()) {
		t.Fatalf("bad apply-loop gate count: %+v", report.Counts)
	}
	if report.ExecutorGateContract.Decision != "hold_for_operator_approved_executor" {
		t.Fatalf("bad executor gate contract: %+v", report.ExecutorGateContract)
	}
	if report.Counts["executor_contract_must_have"] != len(report.ExecutorGateContract.MustHave) || report.Counts["executor_contract_must_not"] != len(report.ExecutorGateContract.MustNot) || report.Counts["executor_contract_refresh_triggers"] != len(report.ExecutorGateContract.RefreshTriggers) {
		t.Fatalf("bad executor contract counts: %+v contract=%+v", report.Counts, report.ExecutorGateContract)
	}
	if !containsString(report.ExecutorGateContract.MustHave, "final container-logscan patterns and hints") {
		t.Fatalf("executor contract should require logscan hints: %+v", report.ExecutorGateContract)
	}
	if !containsString(report.ExecutorGateContract.MustHave, "autoheal_handoff.runtime_evidence_checklist reviewed with docker inspect status/health evidence") {
		t.Fatalf("executor contract should require runtime checklist review: %+v", report.ExecutorGateContract)
	}
	if !containsString(report.ExecutorGateContract.MustNot, "execute docker commands from summary text") {
		t.Fatalf("executor contract should forbid summary execution: %+v", report.ExecutorGateContract)
	}
	if !containsString(report.ExecutorGateContract.RefreshTriggers, "new logscan finding appears") {
		t.Fatalf("executor contract should refresh on new logscan findings: %+v", report.ExecutorGateContract)
	}
	if len(report.LogscanHints) != len(containerSelfHealLogscanHints()) {
		t.Fatalf("bad readiness logscan hints: %+v", report.LogscanHints)
	}
	if strings.Join(report.ApplyLoopGates, ",") != strings.Join(containerApplyLoopGates(), ",") {
		t.Fatalf("bad readiness apply-loop gates: %+v", report.ApplyLoopGates)
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
	if report.Counts["container_findings"] != 1 {
		t.Fatalf("bad container finding count: %+v", report.Counts)
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
	if report.Counts["readiness_summary_ready"] != 1 || report.Counts["readiness_stages"] != len(report.ReadyStages) || report.Counts["readiness_blockers"] != 0 {
		t.Fatalf("bad readiness stage counts: %+v", report.Counts)
	}
	if !containsString(report.StopBefore, "treating readiness summary as operator approval") || !containsString(report.StopBefore, "skipping final container-logscan evidence review") {
		t.Fatalf("readiness summary should expose stop-before gates: %+v", report)
	}
}

func TestBuildContainerReadinessSummaryBlocksFailedCompletionPlan(t *testing.T) {
	completion := containerCompletionPlanReport{
		Kind:    "meshclaw_container_completion_plan",
		Ready:   false,
		Status:  "blocked",
		Reasons: []string{"container rollback plan is not ready"},
	}
	report := buildContainerReadinessSummaryReport("completion.json", evidence.Record{ID: "completion-1", Kind: "container-completion-plan", Payload: completion}, completion)
	if report.Ready || report.Status != "blocked" || len(report.Blockers) != 2 {
		t.Fatalf("blocked completion should block readiness summary: %+v", report)
	}
	if report.Counts["readiness_summary_blocked"] != 1 || report.Counts["readiness_blockers"] != len(report.Blockers) {
		t.Fatalf("bad blocked readiness counts: %+v", report.Counts)
	}
	if report.Counts["completion_plan_blockers"] != 1 {
		t.Fatalf("bad completion-plan blocker count: %+v", report.Counts)
	}
	if !containsString(report.StopBefore, "starting an executor without ready completion-plan evidence") {
		t.Fatalf("blocked readiness summary should retain executor stop-before gate: %+v", report)
	}
}

func TestBuildContainerReadinessSummaryBlocksMissingRequirements(t *testing.T) {
	completion := containerCompletionPlanReport{
		Kind:   "meshclaw_container_completion_plan",
		Ready:  true,
		Status: "ready",
	}
	report := buildContainerReadinessSummaryReport("completion.json", evidence.Record{ID: "completion-1", Kind: "container-completion-plan", Payload: completion}, completion)
	if report.Ready || report.Status != "blocked" || len(report.Blockers) != 1 {
		t.Fatalf("missing requirements should block readiness summary: %+v", report)
	}
	if report.Counts["completion_requirements_blockers"] != 1 {
		t.Fatalf("bad completion-requirements blocker count: %+v", report.Counts)
	}
}

func TestContainerReadinessSummaryCountsLine(t *testing.T) {
	line := containerReadinessSummaryCountsLine(containerReadinessSummaryReport{Counts: map[string]int{
		"readiness_stages":                 7,
		"readiness_blockers":               0,
		"completion_plan_blockers":         1,
		"completion_requirements_blockers": 1,
		"apply_steps":                      2,
		"verification_checks":              2,
		"logscan_patterns":                 9,
		"logscan_hints":                    9,
		"apply_loop_gates":                 7,
		"runbook_steps":                    2,
		"runbook_findings":                 1,
		"runbook_ready_findings":           1,
		"steps_findings":                   1,
		"node_id_findings":                 1,
		"container_findings":               1,
		"command_template_findings":        1,
		"required_evidence_findings":       1,
		"runtime_evidence_findings":        1,
		"success_criteria_findings":        1,
		"failure_action_findings":          1,
		"requires_verification_findings":   1,
		"rollback_steps":                   2,
		"completion_requirements":          3,
	}})
	want := "counts stages=7 blockers=0 completion_plan_blockers=1 completion_requirements_blockers=1 apply_steps=2 verification_checks=2 logscan_patterns=9 logscan_hints=9 apply_loop_gates=7 runbook_steps=2 runbook_findings=1 runbook_ready_findings=1 steps_findings=1 node_id_findings=1 container_findings=1 command_template_findings=1 required_evidence_findings=1 runtime_evidence_findings=1 success_criteria_findings=1 failure_action_findings=1 requires_verification_findings=1 rollback_steps=2 completion_requirements=3"
	if line != want {
		t.Fatalf("counts line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryStagesLine(t *testing.T) {
	line := containerReadinessSummaryStagesLine(containerReadinessSummaryReport{ReadyStages: []string{
		"autoheal_plan",
		"container_apply_plan",
		"container_verification_plan",
	}})
	want := "ready_stages autoheal_plan,container_apply_plan,container_verification_plan"
	if line != want {
		t.Fatalf("stages line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryStagesLineEmpty(t *testing.T) {
	line := containerReadinessSummaryStagesLine(containerReadinessSummaryReport{})
	want := "ready_stages <none>"
	if line != want {
		t.Fatalf("empty stages line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummarySafetyLine(t *testing.T) {
	line := containerReadinessSummarySafetyLine(containerReadinessSummaryReport{Safety: map[string]string{
		"mutation":  "disabled; readiness summary only",
		"execute":   "not implemented",
		"readiness": "future executor must require evidence",
	}})
	want := "safety mutation=disabled; readiness summary only execute=not implemented readiness=future executor must require evidence"
	if line != want {
		t.Fatalf("safety line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummarySafetyLineEmpty(t *testing.T) {
	line := containerReadinessSummarySafetyLine(containerReadinessSummaryReport{})
	want := "safety mutation=<unset> execute=<unset> readiness=<unset>"
	if line != want {
		t.Fatalf("empty safety line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryNextLine(t *testing.T) {
	line := containerReadinessSummaryNextLine(containerReadinessSummaryReport{Next: []string{
		"Use this summary as the operator-visible checkpoint.",
		"Keep container apply disabled until evidence is ready.",
	}})
	want := "next Use this summary as the operator-visible checkpoint. | Keep container apply disabled until evidence is ready."
	if line != want {
		t.Fatalf("next line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryNextLineEmpty(t *testing.T) {
	line := containerReadinessSummaryNextLine(containerReadinessSummaryReport{})
	want := "next <none>"
	if line != want {
		t.Fatalf("empty next line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryStopBeforeLine(t *testing.T) {
	line := containerReadinessSummaryStopBeforeLine(containerReadinessSummaryReport{StopBefore: []string{
		"treating readiness summary as operator approval",
		"skipping final container-logscan evidence review",
	}})
	want := "stop_before count=2 treating readiness summary as operator approval | skipping final container-logscan evidence review"
	if line != want {
		t.Fatalf("stop-before line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryStopBeforeLineEmpty(t *testing.T) {
	line := containerReadinessSummaryStopBeforeLine(containerReadinessSummaryReport{})
	want := "stop_before <none>"
	if line != want {
		t.Fatalf("empty stop-before line = %q, want %q", line, want)
	}
}

func TestReadinessBlockerLines(t *testing.T) {
	lines := readinessBlockerLines([]string{"completion plan is not ready", "rollback plan is not ready"})
	want := "blockers:\n- completion plan is not ready\n- rollback plan is not ready"
	if strings.Join(lines, "\n") != want {
		t.Fatalf("blocker lines = %q, want %q", strings.Join(lines, "\n"), want)
	}
}

func TestReadinessBlockerLinesEmpty(t *testing.T) {
	if lines := readinessBlockerLines(nil); len(lines) != 0 {
		t.Fatalf("empty blocker lines = %#v, want none", lines)
	}
}

func TestContainerReadinessSummaryDecisionLineReady(t *testing.T) {
	line := containerReadinessSummaryDecisionLine(containerReadinessSummaryReport{Ready: true, Status: "ready"})
	want := "decision ready action=hold_for_approval_gated_executor"
	if line != want {
		t.Fatalf("decision line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryDecisionLineBlocked(t *testing.T) {
	line := containerReadinessSummaryDecisionLine(containerReadinessSummaryReport{Ready: false, Status: "blocked"})
	want := "decision blocked action=resolve_blockers_before_apply"
	if line != want {
		t.Fatalf("blocked decision line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryDecisionLineEmptyStatus(t *testing.T) {
	line := containerReadinessSummaryDecisionLine(containerReadinessSummaryReport{})
	want := "decision blocked action=resolve_blockers_before_apply"
	if line != want {
		t.Fatalf("empty-status decision line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryApplyLoopGateLine(t *testing.T) {
	line := containerReadinessSummaryApplyLoopGateLine(containerReadinessSummaryReport{ApplyLoopGates: []string{
		"approval evidence",
		"post-action verification",
	}})
	want := "apply_loop_gates approval evidence,post-action verification"
	if line != want {
		t.Fatalf("apply-loop gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryApplyLoopGateLineEmpty(t *testing.T) {
	line := containerReadinessSummaryApplyLoopGateLine(containerReadinessSummaryReport{})
	want := "apply_loop_gates <none>"
	if line != want {
		t.Fatalf("empty apply-loop gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryExecutorContractLine(t *testing.T) {
	line := containerReadinessSummaryExecutorContractLine(containerReadinessSummaryReport{
		ExecutorGateContract: executorGateContract{
			Decision:        "hold_for_operator_approved_executor",
			MustHave:        []string{"completion evidence", "logscan evidence"},
			MustNot:         []string{"execute from summary"},
			RefreshTriggers: []string{"new logscan finding appears", "policy decision changed"},
		},
	})
	want := "executor_contract decision=hold_for_operator_approved_executor must_have=2 must_not=1 refresh_triggers=2"
	if line != want {
		t.Fatalf("executor contract line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryExecutorContractLineEmpty(t *testing.T) {
	line := containerReadinessSummaryExecutorContractLine(containerReadinessSummaryReport{})
	want := "executor_contract decision=<unset> must_have=0 must_not=0 refresh_triggers=0"
	if line != want {
		t.Fatalf("empty executor contract line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryEvidenceLine(t *testing.T) {
	line := containerReadinessSummaryEvidenceLine(
		containerReadinessSummaryReport{CompletionPlanEvidenceID: "completion-1"},
		evidence.Record{ID: "summary-1"},
	)
	want := "evidence summary=summary-1 completion_plan=completion-1"
	if line != want {
		t.Fatalf("evidence line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryEvidenceLineEmpty(t *testing.T) {
	line := containerReadinessSummaryEvidenceLine(containerReadinessSummaryReport{}, evidence.Record{})
	want := "evidence summary=<unset> completion_plan=<unset>"
	if line != want {
		t.Fatalf("empty evidence line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryEvidencePathsLine(t *testing.T) {
	line := containerReadinessSummaryEvidencePathsLine(containerReadinessSummaryReport{CompletionPlanEvidencePath: "completion.json"})
	want := "evidence_paths completion_plan=completion.json"
	if line != want {
		t.Fatalf("evidence paths line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryEvidencePathsLineEmpty(t *testing.T) {
	line := containerReadinessSummaryEvidencePathsLine(containerReadinessSummaryReport{})
	want := "evidence_paths completion_plan=<unset>"
	if line != want {
		t.Fatalf("empty evidence paths line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryModeLine(t *testing.T) {
	line := containerReadinessSummaryModeLine(containerReadinessSummaryReport{
		Mode:   "summary_only",
		Safety: map[string]string{"execute": "not implemented"},
	})
	want := "mode summary_only execute=not implemented"
	if line != want {
		t.Fatalf("mode line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryModeLineEmpty(t *testing.T) {
	line := containerReadinessSummaryModeLine(containerReadinessSummaryReport{})
	want := "mode <unset> execute=<unset>"
	if line != want {
		t.Fatalf("empty mode line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryGeneratedAtLine(t *testing.T) {
	line := containerReadinessSummaryGeneratedAtLine(containerReadinessSummaryReport{
		GeneratedAt: time.Date(2026, 6, 24, 10, 27, 55, 0, time.UTC),
	})
	want := "generated_at 2026-06-24T10:27:55Z"
	if line != want {
		t.Fatalf("generated-at line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryGeneratedAtLineEmpty(t *testing.T) {
	line := containerReadinessSummaryGeneratedAtLine(containerReadinessSummaryReport{})
	want := "generated_at <unset>"
	if line != want {
		t.Fatalf("empty generated-at line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryOverviewLine(t *testing.T) {
	line := containerReadinessSummaryOverviewLine(containerReadinessSummaryReport{
		Ready:       true,
		Status:      "ready",
		Mode:        "summary_only",
		ReadyStages: []string{"autoheal_plan", "container_apply_plan"},
		Blockers:    []string{"blocked"},
	})
	want := "overview ready=true status=ready mode=summary_only stages=2 blockers=1"
	if line != want {
		t.Fatalf("overview line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryOverviewLineEmpty(t *testing.T) {
	line := containerReadinessSummaryOverviewLine(containerReadinessSummaryReport{})
	want := "overview ready=false status=<unset> mode=<unset> stages=0 blockers=0"
	if line != want {
		t.Fatalf("empty overview line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryApplyGateLine(t *testing.T) {
	line := containerReadinessSummaryApplyGateLine(containerReadinessSummaryReport{
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

func TestContainerReadinessSummaryApplyGateLineEmpty(t *testing.T) {
	line := containerReadinessSummaryApplyGateLine(containerReadinessSummaryReport{})
	want := "apply_gate ready=false mutation=<unset> execute=<unset> evidence=required approval=required"
	if line != want {
		t.Fatalf("empty apply gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryVerificationGateLine(t *testing.T) {
	line := containerReadinessSummaryVerificationGateLine(containerReadinessSummaryReport{Counts: map[string]int{
		"verification_checks": 2,
		"logscan_patterns":    9,
		"logscan_hints":       9,
		"runbook_findings":    1,
		"critical":            1,
	}})
	want := "verification_gate checks=2 logscan_patterns=9 logscan_hints=9 runbook_findings=1 critical_findings=1 post_action_evidence=required"
	if line != want {
		t.Fatalf("verification gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryVerificationGateLineEmpty(t *testing.T) {
	line := containerReadinessSummaryVerificationGateLine(containerReadinessSummaryReport{Counts: map[string]int{}})
	want := "verification_gate checks=0 logscan_patterns=0 logscan_hints=0 runbook_findings=0 critical_findings=0 post_action_evidence=required"
	if line != want {
		t.Fatalf("empty verification gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryCompletionGateLine(t *testing.T) {
	line := containerReadinessSummaryCompletionGateLine(containerReadinessSummaryReport{Counts: map[string]int{
		"completion_requirements":          3,
		"completion_requirements_blockers": 1,
	}})
	want := "completion_gate requirements=3 blockers=1 final_evidence=required"
	if line != want {
		t.Fatalf("completion gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryCompletionGateLineEmpty(t *testing.T) {
	line := containerReadinessSummaryCompletionGateLine(containerReadinessSummaryReport{Counts: map[string]int{}})
	want := "completion_gate requirements=0 blockers=0 final_evidence=required"
	if line != want {
		t.Fatalf("empty completion gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryRollbackGateLine(t *testing.T) {
	line := containerReadinessSummaryRollbackGateLine(containerReadinessSummaryReport{Counts: map[string]int{
		"rollback_steps": 2,
	}})
	want := "rollback_gate steps=2 evidence=required operator_review=required"
	if line != want {
		t.Fatalf("rollback gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryRollbackGateLineEmpty(t *testing.T) {
	line := containerReadinessSummaryRollbackGateLine(containerReadinessSummaryReport{Counts: map[string]int{}})
	want := "rollback_gate steps=0 evidence=required operator_review=required"
	if line != want {
		t.Fatalf("empty rollback gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryPolicyGateLine(t *testing.T) {
	line := containerReadinessSummaryPolicyGateLine(containerReadinessSummaryReport{Safety: map[string]string{
		"mutation": "disabled; readiness summary only",
	}})
	want := "policy_gate policy=required approval=required evidence=required mutation=disabled; readiness summary only"
	if line != want {
		t.Fatalf("policy gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryPolicyGateLineEmpty(t *testing.T) {
	line := containerReadinessSummaryPolicyGateLine(containerReadinessSummaryReport{})
	want := "policy_gate policy=required approval=required evidence=required mutation=<unset>"
	if line != want {
		t.Fatalf("empty policy gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryFreshnessGateLine(t *testing.T) {
	line := containerReadinessSummaryFreshnessGateLine(containerReadinessSummaryReport{
		GeneratedAt: time.Date(2026, 6, 24, 11, 37, 58, 0, time.UTC),
	})
	want := "freshness_gate generated_at=2026-06-24T11:37:58Z evidence_present=true refresh_on_input_change=required"
	if line != want {
		t.Fatalf("freshness gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryFreshnessGateLineEmpty(t *testing.T) {
	line := containerReadinessSummaryFreshnessGateLine(containerReadinessSummaryReport{})
	want := "freshness_gate generated_at=<unset> evidence_present=false refresh_on_input_change=required"
	if line != want {
		t.Fatalf("empty freshness gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryOperatorGateLine(t *testing.T) {
	line := containerReadinessSummaryOperatorGateLine(containerReadinessSummaryReport{
		Blockers: []string{"completion plan is not ready"},
	})
	want := "operator_gate approval=required rollback_review=required blockers=1"
	if line != want {
		t.Fatalf("operator gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryOperatorGateLineEmpty(t *testing.T) {
	line := containerReadinessSummaryOperatorGateLine(containerReadinessSummaryReport{})
	want := "operator_gate approval=required rollback_review=required blockers=0"
	if line != want {
		t.Fatalf("empty operator gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryExecutionGateLine(t *testing.T) {
	line := containerReadinessSummaryExecutionGateLine(containerReadinessSummaryReport{
		Counts: map[string]int{"apply_steps": 2},
		Safety: map[string]string{"execute": "not implemented"},
	})
	want := "execution_gate apply_steps=2 execute=not implemented dry_run=required approval=required"
	if line != want {
		t.Fatalf("execution gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryExecutionGateLineEmpty(t *testing.T) {
	line := containerReadinessSummaryExecutionGateLine(containerReadinessSummaryReport{Counts: map[string]int{}})
	want := "execution_gate apply_steps=0 execute=<unset> dry_run=required approval=required"
	if line != want {
		t.Fatalf("empty execution gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryAuditGateLine(t *testing.T) {
	line := containerReadinessSummaryAuditGateLine(containerReadinessSummaryReport{
		CompletionPlanEvidenceID: "evidence-123",
		GeneratedAt:              time.Date(2026, 6, 24, 12, 8, 28, 0, time.UTC),
	})
	want := "audit_gate completion_plan=evidence-123 generated_at=2026-06-24T12:08:28Z evidence_store=required final_summary=required"
	if line != want {
		t.Fatalf("audit gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryAuditGateLineEmpty(t *testing.T) {
	line := containerReadinessSummaryAuditGateLine(containerReadinessSummaryReport{})
	want := "audit_gate completion_plan=<unset> generated_at=<unset> evidence_store=required final_summary=required"
	if line != want {
		t.Fatalf("empty audit gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryHandoffGateLine(t *testing.T) {
	line := containerReadinessSummaryHandoffGateLine(containerReadinessSummaryReport{
		Ready:                    true,
		CompletionPlanEvidenceID: "evidence-123",
		Blockers:                 []string{"operator review pending"},
	})
	want := "handoff_gate ready=true blockers=1 completion_plan=evidence-123 operator_handoff=required"
	if line != want {
		t.Fatalf("handoff gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryHandoffGateLineEmpty(t *testing.T) {
	line := containerReadinessSummaryHandoffGateLine(containerReadinessSummaryReport{})
	want := "handoff_gate ready=false blockers=0 completion_plan=<unset> operator_handoff=required"
	if line != want {
		t.Fatalf("empty handoff gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryPromotionGateLine(t *testing.T) {
	line := containerReadinessSummaryPromotionGateLine(containerReadinessSummaryReport{
		Ready:       true,
		ReadyStages: []string{"autoheal_plan", "container_apply_plan"},
		Blockers:    []string{"operator review pending"},
	})
	want := "promotion_gate ready=true stages=2 blockers=1 executor=disabled approval=required"
	if line != want {
		t.Fatalf("promotion gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryPromotionGateLineEmpty(t *testing.T) {
	line := containerReadinessSummaryPromotionGateLine(containerReadinessSummaryReport{})
	want := "promotion_gate ready=false stages=0 blockers=0 executor=disabled approval=required"
	if line != want {
		t.Fatalf("empty promotion gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryExecutorGateLine(t *testing.T) {
	line := containerReadinessSummaryExecutorGateLine(containerReadinessSummaryReport{
		Mode:   "summary_only",
		Safety: map[string]string{"execute": "not implemented"},
	})
	want := "executor_gate enabled=false mode=summary_only execute=not implemented dry_run=required approval=required"
	if line != want {
		t.Fatalf("executor gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryExecutorGateLineEmpty(t *testing.T) {
	line := containerReadinessSummaryExecutorGateLine(containerReadinessSummaryReport{})
	want := "executor_gate enabled=false mode=<unset> execute=<unset> dry_run=required approval=required"
	if line != want {
		t.Fatalf("empty executor gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryMCPGateLine(t *testing.T) {
	line := containerReadinessSummaryMCPGateLine(containerReadinessSummaryReport{
		Kind: "meshclaw_container_readiness_summary",
		Mode: "summary_only",
	})
	want := "mcp_gate kind=meshclaw_container_readiness_summary mode=summary_only evidence=required mutation=disabled"
	if line != want {
		t.Fatalf("mcp gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryMCPGateLineEmpty(t *testing.T) {
	line := containerReadinessSummaryMCPGateLine(containerReadinessSummaryReport{})
	want := "mcp_gate kind=<unset> mode=<unset> evidence=required mutation=disabled"
	if line != want {
		t.Fatalf("empty mcp gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryApprovalGateLine(t *testing.T) {
	line := containerReadinessSummaryApprovalGateLine(containerReadinessSummaryReport{
		Ready:    true,
		Blockers: []string{"operator review pending"},
	})
	want := "approval_gate required=true ready=true blockers=1 evidence=required"
	if line != want {
		t.Fatalf("approval gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryApprovalGateLineEmpty(t *testing.T) {
	line := containerReadinessSummaryApprovalGateLine(containerReadinessSummaryReport{})
	want := "approval_gate required=true ready=false blockers=0 evidence=required"
	if line != want {
		t.Fatalf("empty approval gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryDryRunGateLine(t *testing.T) {
	line := containerReadinessSummaryDryRunGateLine(containerReadinessSummaryReport{
		Counts: map[string]int{"apply_steps": 2, "verification_checks": 3},
		Safety: map[string]string{"execute": "not implemented"},
	})
	want := "dry_run_gate required=true apply_steps=2 verification_checks=3 execute=not implemented"
	if line != want {
		t.Fatalf("dry-run gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryDryRunGateLineEmpty(t *testing.T) {
	line := containerReadinessSummaryDryRunGateLine(containerReadinessSummaryReport{Counts: map[string]int{}})
	want := "dry_run_gate required=true apply_steps=0 verification_checks=0 execute=<unset>"
	if line != want {
		t.Fatalf("empty dry-run gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryReviewGateLine(t *testing.T) {
	line := containerReadinessSummaryReviewGateLine(containerReadinessSummaryReport{
		Blockers: []string{"operator review pending"},
	})
	want := "review_gate operator_review=required rollback_review=required blockers=1"
	if line != want {
		t.Fatalf("review gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryReviewGateLineEmpty(t *testing.T) {
	line := containerReadinessSummaryReviewGateLine(containerReadinessSummaryReport{})
	want := "review_gate operator_review=required rollback_review=required blockers=0"
	if line != want {
		t.Fatalf("empty review gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryBlockerGateLine(t *testing.T) {
	line := containerReadinessSummaryBlockerGateLine(containerReadinessSummaryReport{
		Ready:    false,
		Status:   "blocked",
		Blockers: []string{"operator review pending"},
	})
	want := "blocker_gate ready=false status=blocked blockers=1 resolve_before_apply=required"
	if line != want {
		t.Fatalf("blocker gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryBlockerGateLineEmpty(t *testing.T) {
	line := containerReadinessSummaryBlockerGateLine(containerReadinessSummaryReport{})
	want := "blocker_gate ready=false status=<unset> blockers=0 resolve_before_apply=required"
	if line != want {
		t.Fatalf("empty blocker gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryEvidenceGateLine(t *testing.T) {
	line := containerReadinessSummaryEvidenceGateLine(containerReadinessSummaryReport{
		CompletionPlanEvidenceID:   "evidence-123",
		CompletionPlanEvidencePath: "evidence/container-completion.json",
	})
	want := "evidence_gate completion_plan=evidence-123 completion_path=evidence/container-completion.json final_evidence=required"
	if line != want {
		t.Fatalf("evidence gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryEvidenceGateLineEmpty(t *testing.T) {
	line := containerReadinessSummaryEvidenceGateLine(containerReadinessSummaryReport{})
	want := "evidence_gate completion_plan=<unset> completion_path=<unset> final_evidence=required"
	if line != want {
		t.Fatalf("empty evidence gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryRuntimeEvidenceGateLine(t *testing.T) {
	line := containerReadinessSummaryRuntimeEvidenceGateLine(containerReadinessSummaryReport{Counts: map[string]int{
		"runtime_evidence_findings": 2,
	}})
	want := "runtime_evidence_gate findings=2 inspect_status=required image_status_health_ports_restart_policy=required"
	if line != want {
		t.Fatalf("runtime evidence gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryRuntimeEvidenceGateLineEmpty(t *testing.T) {
	line := containerReadinessSummaryRuntimeEvidenceGateLine(containerReadinessSummaryReport{Counts: map[string]int{}})
	want := "runtime_evidence_gate findings=0 inspect_status=required image_status_health_ports_restart_policy=required"
	if line != want {
		t.Fatalf("empty runtime evidence gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryRollbackReadinessGateLine(t *testing.T) {
	line := containerReadinessSummaryRollbackReadinessGateLine(containerReadinessSummaryReport{
		Counts: map[string]int{"rollback_steps": 2},
	})
	want := "rollback_readiness_gate steps=2 evidence=required operator_review=required"
	if line != want {
		t.Fatalf("rollback readiness gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryRollbackReadinessGateLineEmpty(t *testing.T) {
	line := containerReadinessSummaryRollbackReadinessGateLine(containerReadinessSummaryReport{Counts: map[string]int{}})
	want := "rollback_readiness_gate steps=0 evidence=required operator_review=required"
	if line != want {
		t.Fatalf("empty rollback readiness gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryVerificationReadinessGateLine(t *testing.T) {
	line := containerReadinessSummaryVerificationReadinessGateLine(containerReadinessSummaryReport{
		Counts: map[string]int{"verification_checks": 3, "runbook_findings": 1},
	})
	want := "verification_readiness_gate checks=3 runbook_findings=1 post_action_evidence=required"
	if line != want {
		t.Fatalf("verification readiness gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryVerificationReadinessGateLineEmpty(t *testing.T) {
	line := containerReadinessSummaryVerificationReadinessGateLine(containerReadinessSummaryReport{Counts: map[string]int{}})
	want := "verification_readiness_gate checks=0 runbook_findings=0 post_action_evidence=required"
	if line != want {
		t.Fatalf("empty verification readiness gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryCompletionReadinessGateLine(t *testing.T) {
	line := containerReadinessSummaryCompletionReadinessGateLine(containerReadinessSummaryReport{
		Counts: map[string]int{"completion_requirements": 3, "completion_requirements_blockers": 1},
	})
	want := "completion_readiness_gate requirements=3 blockers=1 final_evidence=required"
	if line != want {
		t.Fatalf("completion readiness gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryCompletionReadinessGateLineEmpty(t *testing.T) {
	line := containerReadinessSummaryCompletionReadinessGateLine(containerReadinessSummaryReport{Counts: map[string]int{}})
	want := "completion_readiness_gate requirements=0 blockers=0 final_evidence=required"
	if line != want {
		t.Fatalf("empty completion readiness gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryStageReadinessGateLine(t *testing.T) {
	line := containerReadinessSummaryStageReadinessGateLine(containerReadinessSummaryReport{
		ReadyStages: []string{"autoheal_plan", "container_apply_plan"},
		Blockers:    []string{"operator review pending"},
	})
	want := "stage_readiness_gate stages=2 blockers=1 refresh_on_input_change=required"
	if line != want {
		t.Fatalf("stage readiness gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryStageReadinessGateLineEmpty(t *testing.T) {
	line := containerReadinessSummaryStageReadinessGateLine(containerReadinessSummaryReport{})
	want := "stage_readiness_gate stages=0 blockers=0 refresh_on_input_change=required"
	if line != want {
		t.Fatalf("empty stage readiness gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryOperatorHandoffGateLine(t *testing.T) {
	line := containerReadinessSummaryOperatorHandoffGateLine(containerReadinessSummaryReport{
		Blockers: []string{"operator review pending"},
	})
	want := "operator_handoff_gate approval=required review=required evidence=required blockers=1"
	if line != want {
		t.Fatalf("operator handoff gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryOperatorHandoffGateLineEmpty(t *testing.T) {
	line := containerReadinessSummaryOperatorHandoffGateLine(containerReadinessSummaryReport{})
	want := "operator_handoff_gate approval=required review=required evidence=required blockers=0"
	if line != want {
		t.Fatalf("empty operator handoff gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryOperatorApplyGateLine(t *testing.T) {
	line := containerReadinessSummaryOperatorApplyGateLine(containerReadinessSummaryReport{
		Blockers: []string{"rollback evidence missing"},
	})
	want := "operator_apply_gate approval=required dry_run=required rollback_ready=required blockers=1"
	if line != want {
		t.Fatalf("operator apply gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryOperatorApplyGateLineEmpty(t *testing.T) {
	line := containerReadinessSummaryOperatorApplyGateLine(containerReadinessSummaryReport{})
	want := "operator_apply_gate approval=required dry_run=required rollback_ready=required blockers=0"
	if line != want {
		t.Fatalf("empty operator apply gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryPostApplyGateLine(t *testing.T) {
	line := containerReadinessSummaryPostApplyGateLine(containerReadinessSummaryReport{
		Counts:   map[string]int{"verification_checks": 3},
		Blockers: []string{"post-apply evidence missing"},
	})
	want := "post_apply_gate verification_checks=3 evidence=required rollback_ready=required blockers=1"
	if line != want {
		t.Fatalf("post apply gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryPostApplyGateLineEmpty(t *testing.T) {
	line := containerReadinessSummaryPostApplyGateLine(containerReadinessSummaryReport{Counts: map[string]int{}})
	want := "post_apply_gate verification_checks=0 evidence=required rollback_ready=required blockers=0"
	if line != want {
		t.Fatalf("empty post apply gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryOperatorResultGateLine(t *testing.T) {
	line := containerReadinessSummaryOperatorResultGateLine(containerReadinessSummaryReport{
		Counts:   map[string]int{"verification_checks": 3},
		Blockers: []string{"final status missing"},
	})
	want := "operator_result_gate verification_checks=3 evidence=required final_status=required blockers=1"
	if line != want {
		t.Fatalf("operator result gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryOperatorResultGateLineEmpty(t *testing.T) {
	line := containerReadinessSummaryOperatorResultGateLine(containerReadinessSummaryReport{Counts: map[string]int{}})
	want := "operator_result_gate verification_checks=0 evidence=required final_status=required blockers=0"
	if line != want {
		t.Fatalf("empty operator result gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryCloseoutGateLine(t *testing.T) {
	line := containerReadinessSummaryCloseoutGateLine(containerReadinessSummaryReport{
		Blockers: []string{"final status missing"},
	})
	want := "closeout_gate final_status=required evidence=required rollback_retention=required blockers=1"
	if line != want {
		t.Fatalf("closeout gate line = %q, want %q", line, want)
	}
}

func TestContainerReadinessSummaryCloseoutGateLineEmpty(t *testing.T) {
	line := containerReadinessSummaryCloseoutGateLine(containerReadinessSummaryReport{})
	want := "closeout_gate final_status=required evidence=required rollback_retention=required blockers=0"
	if line != want {
		t.Fatalf("empty closeout gate line = %q, want %q", line, want)
	}
}

func TestMCPAutohealContainerReadinessSummaryCounts(t *testing.T) {
	completion := containerCompletionPlanReport{
		Kind:   "meshclaw_container_completion_plan",
		Ready:  true,
		Status: "ready",
		Requirements: []containerCompletionRequirement{{
			RequiredEvidence: "container-rollback-plan",
		}},
		RollbackPlan: containerRollbackPlanReport{
			Steps: []containerRollbackStep{{Step: 1, NodeID: "g4", Container: "api"}},
			RunbookCheck: containerRunbookCheckReport{
				Runbook: containerRunbookReport{
					Steps: []containerRunbookStep{{Step: 1, NodeID: "g4", Container: "api"}},
					VerificationPlan: containerVerificationPlanReport{
						Checks: []containerVerificationCheck{{Step: 1, NodeID: "g4", Container: "api"}},
						ApplyPlan: containerApplyPlanReport{
							Steps: []containerApplyStep{{Step: 1, NodeID: "g4", Container: "api"}},
						},
					},
				},
			},
		},
	}
	record, storeErr := evidence.Store("container-completion-plan", "fleet", "ready=true requirements=1", completion)
	if storeErr != nil {
		t.Fatal(storeErr)
	}
	result, err := callMCPTool("meshclaw_autoheal_container_readiness_summary", map[string]interface{}{"container_completion_plan_evidence_path": record.StoredAt})
	if err != nil {
		t.Fatalf("readiness summary error = %v", err)
	}
	summary := result.(map[string]interface{})["container_readiness_summary"].(containerReadinessSummaryReport)
	if !summary.Ready || summary.Status != "ready" || summary.Counts["readiness_summary_ready"] != 1 {
		t.Fatalf("bad container readiness summary: %+v", summary)
	}
	payload := result.(map[string]interface{})
	contract, ok := payload["readiness_contract"].(map[string]interface{})
	if !ok {
		t.Fatalf("container readiness contract missing: %#v", payload)
	}
	if contract["summary_tools_are_approval"] != false || contract["apply_allowed"] != false || contract["grants_future_approval"] != false {
		t.Fatalf("container readiness contract should not grant approval/apply: %#v", contract)
	}
	stopBefore, _ := contract["stop_before"].([]string)
	if !containsString(stopBefore, "treating readiness summary as operator approval") {
		t.Fatalf("container readiness contract should preserve stop-before gates: %#v", contract)
	}
	containerContract, ok := payload["container_readiness_summary_contract"].(map[string]interface{})
	if !ok {
		t.Fatalf("container readiness summary contract missing: %#v", payload)
	}
	if containerContract["summary_only"] != true || containerContract["apply_allowed"] != false || containerContract["execute_implemented"] != false || containerContract["mutates_live_servers"] != false {
		t.Fatalf("container readiness summary contract should be summary-only and non-mutating: %#v", containerContract)
	}
	if containerContract["requires_operator_approval_for_executor"] != true || containerContract["requires_approval_gated_executor"] != true || containerContract["requires_completion_plan"] != true || containerContract["requires_final_evidence"] != true {
		t.Fatalf("container readiness summary contract should require approval-gated executor and final evidence: %#v", containerContract)
	}
	if containerContract["requires_runtime_evidence"] != true || containerContract["requires_container_logscan"] != true || containerContract["grants_future_approval"] != false || containerContract["summary_tools_are_approval"] != false {
		t.Fatalf("container readiness summary contract should require runtime/logscan evidence and grant no approval: %#v", containerContract)
	}
	if containerContract["readiness_stages"] != len(summary.ReadyStages) || containerContract["readiness_blockers"] != len(summary.Blockers) {
		t.Fatalf("container readiness summary contract should mirror readiness counts: %#v", containerContract)
	}
	containerStopBefore, _ := containerContract["stop_before"].([]string)
	if !containsString(containerStopBefore, "starting an executor without ready completion-plan evidence") {
		t.Fatalf("container readiness summary contract should preserve executor stop-before gates: %#v", containerContract)
	}
	for _, key := range []string{"readiness_stages", "readiness_blockers", "apply_steps", "verification_checks", "runbook_steps", "rollback_steps", "completion_requirements"} {
		if _, ok := summary.Counts[key]; !ok {
			t.Fatalf("missing container readiness count %q in %+v", key, summary.Counts)
		}
	}
}

func TestContainerCompletionPlanFromEvidenceRejectsWrongKind(t *testing.T) {
	_, err := containerCompletionPlanFromEvidence(evidence.Record{Kind: "container-rollback-plan", Payload: map[string]interface{}{}})
	if err == nil {
		t.Fatal("expected wrong evidence kind error")
	}
}

func TestMCPAutohealContainerReadinessSummaryRejectsExecute(t *testing.T) {
	_, err := callMCPTool("meshclaw_autoheal_container_readiness_summary", map[string]interface{}{"container_completion_plan_evidence_path": "completion.json", "execute": true})
	if err == nil || !strings.Contains(err.Error(), "summarizes readiness only") {
		t.Fatalf("expected execute rejection, got %v", err)
	}
}

func TestContainerReadinessSummaryFromEvidenceRejectsWrongKind(t *testing.T) {
	_, err := containerReadinessSummaryFromEvidence(evidence.Record{Kind: "container-completion-plan", Payload: map[string]interface{}{}})
	if err == nil {
		t.Fatal("expected wrong evidence kind error")
	}
}

func TestBuildContainerExecutorGateRequiresApprovalAndDryRun(t *testing.T) {
	summary := containerReadinessSummaryReport{
		Kind:           "meshclaw_container_readiness_summary",
		Ready:          true,
		Status:         "ready",
		ApplyLoopGates: []string{"approval evidence"},
		LogscanPatterns: []string{
			"restart_loop",
		},
		CompletionPlan: containerCompletionPlanReport{Kind: "meshclaw_container_completion_plan"},
	}
	report := buildContainerExecutorGateReport("summary.json", evidence.Record{ID: "summary-1", Kind: "container-readiness-summary", Payload: summary}, summary, "", false, true)
	if report.Ready || report.Status != "blocked" {
		t.Fatalf("gate should block without approval/dry-run: %+v", report)
	}
	for _, blocker := range []string{"operator_approval_present", "dry_run_required", "execute_not_requested"} {
		if !containsString(report.Blockers, blocker) {
			t.Fatalf("missing blocker %q in %+v", blocker, report.Blockers)
		}
	}
	if report.LiveExecutionAllowed {
		t.Fatalf("gate must not allow live execution: %+v", report)
	}
}

func TestMCPAutohealContainerExecutorGateContract(t *testing.T) {
	summary := containerReadinessSummaryReport{
		Kind:           "meshclaw_container_readiness_summary",
		Ready:          true,
		Status:         "ready",
		ApplyLoopGates: []string{"approval evidence"},
		LogscanPatterns: []string{
			"restart_loop",
		},
		CompletionPlan: containerCompletionPlanReport{Kind: "meshclaw_container_completion_plan"},
	}
	record, storeErr := evidence.Store("container-readiness-summary", "fleet", "ready=true", summary)
	if storeErr != nil {
		t.Fatal(storeErr)
	}
	result, err := callMCPTool("meshclaw_autoheal_container_executor_gate", map[string]interface{}{
		"container_readiness_summary_evidence_path": record.StoredAt,
		"approved_by": "zeus",
		"dry_run":     true,
	})
	if err != nil {
		t.Fatalf("executor gate error = %v", err)
	}
	payload := result.(map[string]interface{})
	report := payload["container_executor_gate"].(containerExecutorGateReport)
	if !report.Ready || report.Status != "ready" || len(report.Checks) == 0 || len(report.Blockers) != 0 {
		t.Fatalf("bad executor gate: %+v", report)
	}
	contract, ok := payload["container_executor_gate_contract"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing executor gate contract: %#v", payload)
	}
	if contract["admission_only"] != true || contract["apply_allowed"] != false || contract["execute_implemented"] != false || contract["mutates_live_servers"] != false {
		t.Fatalf("executor gate contract should be admission-only and non-mutating: %#v", contract)
	}
	if contract["requires_operator_approval"] != true || contract["requires_dry_run"] != true || contract["live_execution_allowed"] != false {
		t.Fatalf("executor gate contract should preserve approval/dry-run gates: %#v", contract)
	}
}

func TestContainerExecutorGateFromEvidenceRejectsWrongKind(t *testing.T) {
	_, err := containerExecutorGateFromEvidence(evidence.Record{Kind: "container-readiness-summary", Payload: map[string]interface{}{}})
	if err == nil {
		t.Fatal("expected wrong executor gate evidence kind to fail")
	}
}

func TestBuildContainerExecutorReportDryRunPreview(t *testing.T) {
	gate := readyContainerExecutorGateForTest()
	report := buildContainerExecutorReport("gate.json", evidence.Record{ID: "gate-1", Kind: "container-executor-gate", Payload: gate}, gate, "zeus", true, false, "")
	if !report.Ready || report.Status != "ready" || report.LiveExecutionAllowed || report.Executed {
		t.Fatalf("dry-run executor preview should be ready but non-live: %+v", report)
	}
	if len(report.Steps) != 1 {
		t.Fatalf("executor preview steps=%d, want 1", len(report.Steps))
	}
	step := report.Steps[0]
	if step.NodeID != "g4" || step.Container != "api" || step.Command != "docker restart 'api'" || step.WouldExecute {
		t.Fatalf("bad dry-run executor step: %+v", step)
	}
}

func TestBuildContainerExecutorReportBlocksLiveWithoutExactPhrase(t *testing.T) {
	gate := readyContainerExecutorGateForTest()
	report := buildContainerExecutorReport("gate.json", evidence.Record{ID: "gate-1", Kind: "container-executor-gate", Payload: gate}, gate, "zeus", false, true, "yes")
	if report.Ready || report.LiveExecutionAllowed {
		t.Fatalf("live executor should block without exact phrase: %+v", report)
	}
	if !containsString(report.Blockers, "live_approval_phrase must exactly equal "+containerExecutorLiveApprovalPhrase) {
		t.Fatalf("missing live approval phrase blocker: %+v", report.Blockers)
	}
}

func TestBuildContainerExecutorReportAllowsLiveWithExactPhrase(t *testing.T) {
	gate := readyContainerExecutorGateForTest()
	report := buildContainerExecutorReport("gate.json", evidence.Record{ID: "gate-1", Kind: "container-executor-gate", Payload: gate}, gate, "zeus", false, true, containerExecutorLiveApprovalPhrase)
	if !report.Ready || !report.LiveExecutionAllowed || report.Status != "ready_to_execute" {
		t.Fatalf("live executor should become ready only after exact phrase: %+v", report)
	}
	if len(report.Steps) != 1 || !report.Steps[0].WouldExecute {
		t.Fatalf("live executor should mark the validated step as would_execute: %+v", report.Steps)
	}
}

func TestMCPAutohealContainerExecutorDryRunContract(t *testing.T) {
	gate := readyContainerExecutorGateForTest()
	record, storeErr := evidence.Store("container-executor-gate", "fleet", "ready=true", gate)
	if storeErr != nil {
		t.Fatal(storeErr)
	}
	result, err := callMCPTool("meshclaw_autoheal_container_executor", map[string]interface{}{
		"container_executor_gate_evidence_path": record.StoredAt,
		"approved_by":                           "zeus",
		"dry_run":                               true,
	})
	if err != nil {
		t.Fatalf("container executor error = %v", err)
	}
	payload := result.(map[string]interface{})
	report := payload["container_executor"].(containerExecutorReport)
	if !report.Ready || report.Executed || report.LiveExecutionAllowed || len(report.Steps) != 1 {
		t.Fatalf("bad dry-run executor report: %+v", report)
	}
	contract, ok := payload["container_executor_contract"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing executor contract: %#v", payload)
	}
	if contract["dry_run"] != true || contract["executed"] != false || contract["mutates_live_servers"] != false || contract["requires_exact_live_phrase"] != true {
		t.Fatalf("executor dry-run contract should remain non-mutating: %#v", contract)
	}
}

func TestContainerExecutorFromEvidenceRejectsWrongKind(t *testing.T) {
	_, err := containerExecutorFromEvidence(evidence.Record{Kind: "container-executor-gate", Payload: map[string]interface{}{}})
	if err == nil {
		t.Fatal("expected wrong executor evidence kind to fail")
	}
}

func TestBuildContainerExecutorVerificationBlocksMissingPostEvidence(t *testing.T) {
	executor := completedContainerExecutorForTest()
	report := buildContainerExecutorVerificationReport("executor.json", evidence.Record{ID: "executor-1", Kind: "container-executor", Payload: executor}, executor, nil, nil)
	if report.Ready || report.Status != "blocked" {
		t.Fatalf("verification should block without post-action evidence: %+v", report)
	}
	for _, blocker := range []string{"agent_evidence_per_step", "container_logscan_evidence_per_step"} {
		if !containsString(report.Blockers, blocker) {
			t.Fatalf("missing blocker %q in %+v", blocker, report.Blockers)
		}
	}
}

func TestBuildContainerExecutorVerificationReadyWithPostEvidence(t *testing.T) {
	executor := completedContainerExecutorForTest()
	report := buildContainerExecutorVerificationReport("executor.json", evidence.Record{ID: "executor-1", Kind: "container-executor", Payload: executor}, executor, []string{"agent.json"}, []string{"logscan.json"})
	if !report.Ready || report.Status != "ready" || len(report.Blockers) != 0 {
		t.Fatalf("verification should be ready with post-action evidence: %+v", report)
	}
}

func TestMCPAutohealContainerExecutorVerifyContract(t *testing.T) {
	executor := completedContainerExecutorForTest()
	record, storeErr := evidence.Store("container-executor", "fleet", "executed=true", executor)
	if storeErr != nil {
		t.Fatal(storeErr)
	}
	result, err := callMCPTool("meshclaw_autoheal_container_executor_verify", map[string]interface{}{
		"container_executor_evidence_path": record.StoredAt,
		"agent_evidence_paths":             "agent.json",
		"container_logscan_evidence_paths": "logscan.json",
	})
	if err != nil {
		t.Fatalf("container executor verify error = %v", err)
	}
	payload := result.(map[string]interface{})
	report := payload["container_executor_verification"].(containerExecutorVerificationReport)
	if !report.Ready || len(report.Checks) == 0 || len(report.Blockers) != 0 {
		t.Fatalf("bad executor verification: %+v", report)
	}
	contract, ok := payload["container_executor_verification_contract"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing executor verification contract: %#v", payload)
	}
	if contract["gate_only"] != true || contract["apply_allowed"] != false || contract["execute_allowed"] != false || contract["mutates_live_servers"] != false {
		t.Fatalf("verification contract should be gate-only and non-mutating: %#v", contract)
	}
	if contract["requires_agent_evidence"] != true || contract["requires_container_logscan"] != true {
		t.Fatalf("verification contract should require post-action evidence: %#v", contract)
	}
}

func completedContainerExecutorForTest() containerExecutorReport {
	gate := readyContainerExecutorGateForTest()
	report := buildContainerExecutorReport("gate.json", evidence.Record{ID: "gate-1", Kind: "container-executor-gate", Payload: gate}, gate, "zeus", false, true, containerExecutorLiveApprovalPhrase)
	report.Executed = true
	report.Status = "completed"
	report.LiveExecutionAllowed = true
	for i := range report.Steps {
		report.Steps[i].Executed = true
		report.Steps[i].Success = true
		report.Steps[i].Transport = "vssh-native"
		report.Steps[i].ExitCode = 0
	}
	return report
}

func readyContainerExecutorGateForTest() containerExecutorGateReport {
	runbook := containerRunbookReport{
		Kind:   "meshclaw_container_runbook",
		Ready:  true,
		Status: "ready",
		Steps: []containerRunbookStep{{
			Step:                    1,
			NodeID:                  "g4",
			Container:               "api",
			Operation:               "container_restart",
			CommandTemplate:         "docker restart 'api'",
			RequiredEvidence:        "agent-collect+container-logscan",
			RuntimeEvidenceRequired: "fresh runtime inspect/status evidence with image, status, health, ports, and restart policy",
			SuccessCriteria:         []string{"container is running"},
			FailureAction:           "stop container apply loop and create rollback or manual triage approval",
			RequiresVerification:    true,
			Retryable:               true,
		}},
	}
	summary := containerReadinessSummaryReport{
		Kind:           "meshclaw_container_readiness_summary",
		Ready:          true,
		Status:         "ready",
		ApplyLoopGates: []string{"approval evidence"},
		LogscanPatterns: []string{
			"restart_loop",
		},
		CompletionPlan: containerCompletionPlanReport{
			Kind:  "meshclaw_container_completion_plan",
			Ready: true,
			RollbackPlan: containerRollbackPlanReport{
				Kind:  "meshclaw_container_rollback_plan",
				Ready: true,
				RunbookCheck: containerRunbookCheckReport{
					Kind:    "meshclaw_container_runbook_check",
					Ready:   true,
					Runbook: runbook,
				},
			},
		},
	}
	return buildContainerExecutorGateReport("summary.json", evidence.Record{ID: "summary-1", Kind: "container-readiness-summary", Payload: summary}, summary, "zeus", true, false)
}

func TestPolicyActionForHealPlan(t *testing.T) {
	cases := map[string]string{
		"disk_investigate":             "disk_investigate",
		"disk_cleanup":                 "autoheal_apply_safe",
		"memory_cleanup":               "autoheal_apply_safe",
		"connectivity":                 "doctor",
		"service_triage":               "service_check",
		"service_quarantine_candidate": "service_quarantine",
		"container_restart":            "container_restart",
	}
	for actionType, want := range cases {
		got := policyActionForHealPlan(monitor.HealPlanAction{Type: actionType})
		if got != want {
			t.Fatalf("%s policy action = %q, want %q", actionType, got, want)
		}
	}
}

func TestRecommendedOpsControlMCPPrioritizesStructuredFollowUps(t *testing.T) {
	report := opsControlReport{
		Monitor: monitorCheckOutput{Alerts: []monitor.Alert{
			{Node: "d1", Type: "disk", Level: "warning", Message: "Disk usage high"},
			{Node: "g1", Type: "memory", Level: "warning", Message: "Memory usage high"},
		}},
		ServiceTriage: serviceTriageReport{
			Items: []serviceTriageItem{{
				Host:    "d1",
				Service: "open-webui",
				Class:   "real_incident",
			}},
		},
		ManagementSummary: opsManagementSummary{
			ResourceAlerts:  2,
			ServiceFindings: 1,
			RealIncidents:   1,
		},
	}

	calls := recommendedOpsControlMCP(report)
	if len(calls) == 0 {
		t.Fatal("expected recommended MCP calls")
	}
	if calls[0].Tool != "meshclaw_service_check" {
		t.Fatalf("first tool=%q, want meshclaw_service_check", calls[0].Tool)
	}
	if calls[0].Arguments["host"] != "d1" || calls[0].Arguments["service"] != "open-webui" {
		t.Fatalf("unexpected service_check args: %#v", calls[0].Arguments)
	}
	if !hasRecommendedMCPTool(calls, "meshclaw_disk_investigate") {
		t.Fatalf("missing disk follow-up: %#v", calls)
	}
	if !hasRecommendedMCPTool(calls, "meshclaw_process_top") {
		t.Fatalf("missing memory follow-up: %#v", calls)
	}
	if calls[len(calls)-1].Tool != "meshclaw_evidence_latest" {
		t.Fatalf("last tool=%q, want meshclaw_evidence_latest", calls[len(calls)-1].Tool)
	}
}

func TestRecommendedServiceTriageMCPReturnsActionableCalls(t *testing.T) {
	report := serviceTriageReport{
		ServiceFindings: 2,
		MaxParallel:     4,
		Items: []serviceTriageItem{
			{Host: "d1", Service: "open-webui", Class: "real_incident"},
			{Host: "v1", Service: "old-worker", Class: "stale_or_missing_target"},
		},
	}

	calls := recommendedServiceTriageMCP(report)
	if len(calls) == 0 {
		t.Fatal("expected recommended MCP calls")
	}
	if calls[0].Tool != "meshclaw_service_check" {
		t.Fatalf("first tool=%q, want meshclaw_service_check", calls[0].Tool)
	}
	if !hasRecommendedMCPTool(calls, "meshclaw_analyze_logs") {
		t.Fatalf("missing log analysis follow-up: %#v", calls)
	}
	if !hasRecommendedMCPTool(calls, "meshclaw_policy_check") {
		t.Fatalf("missing policy follow-up: %#v", calls)
	}
	if calls[len(calls)-1].Tool != "meshclaw_evidence_latest" {
		t.Fatalf("last tool=%q, want meshclaw_evidence_latest", calls[len(calls)-1].Tool)
	}
}

func TestRecommendedAutohealPlanMCPSeparatesSafeAndApprovalActions(t *testing.T) {
	actions := []monitor.HealPlanAction{
		{
			Node:             "d1",
			Type:             "disk_investigate",
			Mode:             "read_only",
			Metric:           "disk",
			Command:          "meshclaw disk-investigate d1 /",
			PolicyAction:     "disk_investigate",
			PolicyDecision:   "allow",
			ApprovalRequired: false,
		},
		{
			Node:             "v1",
			Type:             "service_quarantine_candidate",
			Mode:             "approval_required",
			Metric:           "service:old-worker",
			Command:          "meshclaw service-quarantine v1 old-worker",
			PolicyAction:     "service_quarantine",
			PolicyDecision:   "require_approval",
			ApprovalRequired: true,
		},
		{
			Node:             "g1",
			Type:             "memory_cleanup",
			Mode:             "auto_safe",
			Command:          "sudo sync",
			PolicyAction:     "autoheal_apply_safe",
			PolicyDecision:   "allow",
			ApprovalRequired: false,
		},
		{
			Node:             "g4",
			Type:             "container_restart",
			Container:        "api",
			Mode:             "propose",
			Command:          "docker restart 'api'",
			PolicyAction:     "container_restart",
			PolicyDecision:   "require_approval",
			ApprovalRequired: true,
		},
	}

	calls := recommendedAutohealPlanMCP(actions)
	if !hasRecommendedMCPTool(calls, "meshclaw_disk_investigate") {
		t.Fatalf("missing disk evidence call: %#v", calls)
	}
	if !hasRecommendedMCPTool(calls, "meshclaw_policy_check") {
		t.Fatalf("missing policy check call: %#v", calls)
	}
	if !hasRecommendedMCPTool(calls, "meshclaw_autoheal_apply_safe") {
		t.Fatalf("missing apply-safe call: %#v", calls)
	}
	var foundContainerLogs bool
	for _, call := range calls {
		if call.Tool == "meshclaw_analyze_logs" && call.Arguments["host"] == "g4" && call.Arguments["source"] == "container:api" {
			foundContainerLogs = true
		}
	}
	if !foundContainerLogs {
		t.Fatalf("missing container logscan recommendation: %#v", calls)
	}
	if calls[len(calls)-1].Tool != "meshclaw_evidence_latest" {
		t.Fatalf("last tool=%q, want meshclaw_evidence_latest", calls[len(calls)-1].Tool)
	}
}

func TestRecommendedFleetScanMCPMapsFindingsToFocusedTools(t *testing.T) {
	report := fleet.Report{
		Alerts: []monitor.Alert{{Node: "d1", Type: "disk", Level: "warning"}},
		Hosts: []fleet.HostReport{
			{
				Host:   "c1",
				Online: true,
				Security: &workflow.Report{Status: "findings", Findings: []workflow.Finding{{
					Severity: "warning",
					Title:    "Open listeners",
				}}},
				Logs: &workflow.Report{Status: "findings", Findings: []workflow.Finding{{
					Severity: "warning",
					Title:    "Recent warning/error log evidence found",
				}}},
			},
			{
				Host:   "v1",
				Online: true,
				Hygiene: &hygiene.Report{Status: "findings", Findings: []hygiene.Finding{{
					Severity: "high",
					Type:     hygiene.FindingSecretPattern,
				}}},
			},
		},
	}

	calls := recommendedFleetScanMCP(report)
	for _, tool := range []string{"meshclaw_disk_investigate", "meshclaw_security_check", "meshclaw_analyze_logs", "meshclaw_hygiene_scan_host"} {
		if !hasRecommendedMCPTool(calls, tool) {
			t.Fatalf("missing %s in %#v", tool, calls)
		}
	}
	if calls[len(calls)-1].Tool != "meshclaw_evidence_latest" {
		t.Fatalf("last tool=%q, want meshclaw_evidence_latest", calls[len(calls)-1].Tool)
	}
}

func TestRecommendedFleetScanMCPFiltersAlertsToSelectedHosts(t *testing.T) {
	report := fleet.Report{
		Options: fleet.Options{Hosts: []string{"d1"}},
		Alerts: []monitor.Alert{
			{Node: "d1", Type: "disk", Level: "warning"},
			{Node: "v3", Type: "offline", Level: "critical"},
		},
		Hosts: []fleet.HostReport{{Host: "d1", Online: true}},
	}

	calls := recommendedFleetScanMCP(report)
	if !hasRecommendedMCPTool(calls, "meshclaw_disk_investigate") {
		t.Fatalf("missing selected host disk call: %#v", calls)
	}
	for _, call := range calls {
		if call.Tool == "meshclaw_node_repair_plan" && call.Arguments["hosts"] == "v3" {
			t.Fatalf("unexpected unselected host repair call: %#v", calls)
		}
	}
	if calls[len(calls)-1].Tool != "meshclaw_evidence_latest" {
		t.Fatalf("last tool=%q, want meshclaw_evidence_latest", calls[len(calls)-1].Tool)
	}
}

func TestLimitRecommendedMCPCallsKeepsEvidenceLatest(t *testing.T) {
	calls := []recommendedMCPCall{}
	for i := 0; i < 10; i++ {
		calls = append(calls, recommendedMCPCall{Tool: "tool"})
	}
	calls = append(calls, recommendedMCPCall{Tool: "meshclaw_evidence_latest"})

	limited := limitRecommendedMCPCalls(calls, 8)
	if len(limited) != 8 {
		t.Fatalf("len=%d, want 8", len(limited))
	}
	if limited[len(limited)-1].Tool != "meshclaw_evidence_latest" {
		t.Fatalf("last tool=%q, want meshclaw_evidence_latest", limited[len(limited)-1].Tool)
	}
}

func TestRecommendedNodeRepairPlanMCPClassifiesRepairPaths(t *testing.T) {
	report := nodeRepairPlanReport{
		Hosts: []nodeRepairPlanHost{
			{Host: "c1", DaemonStatus: "auth_failed", Severity: "repair"},
			{Host: "s2", DaemonStatus: "protocol_mismatch", AuthDecision: "partial", Severity: "blocked"},
			{Host: "v3", DaemonStatus: "unreachable", Severity: "blocked"},
		},
	}

	calls := recommendedNodeRepairPlanMCP(report)
	for _, tool := range []string{"meshclaw_vssh_auth_paths", "meshclaw_policy_check", "meshclaw_vssh_daemon_audit", "meshclaw_node_inventory"} {
		if !hasRecommendedMCPTool(calls, tool) {
			t.Fatalf("missing %s in %#v", tool, calls)
		}
	}
	if calls[len(calls)-1].Tool != "meshclaw_evidence_latest" {
		t.Fatalf("last tool=%q, want meshclaw_evidence_latest", calls[len(calls)-1].Tool)
	}
}

func hasRecommendedMCPTool(calls []recommendedMCPCall, tool string) bool {
	for _, call := range calls {
		if call.Tool == tool {
			return true
		}
	}
	return false
}

func TestAnnotateHealActionsPoliciesAddsDecisionMetadata(t *testing.T) {
	actions := []monitor.HealAction{
		{
			Node:    "d1",
			Type:    "disk_cleanup",
			Command: "sudo apt-get clean",
		},
		{
			Node:    "d1",
			Type:    "restart_service",
			Command: "sudo systemctl restart nginx",
		},
	}

	annotateHealActionsPolicies(actions)

	if actions[0].PolicyAction != "autoheal_apply_safe" || actions[0].PolicyDecision != "allow" || actions[0].ApprovalRequired {
		t.Fatalf("disk cleanup policy = %#v", actions[0])
	}
	if actions[1].PolicyAction != "restart_service" || actions[1].PolicyDecision != "require_approval" || !actions[1].ApprovalRequired {
		t.Fatalf("restart policy = %#v", actions[1])
	}
}

func TestApplyAutohealPlanSafeSkipsReadOnlyAndApprovalRequired(t *testing.T) {
	plan := []monitor.HealPlanAction{
		{
			Node:             "d1",
			Type:             "disk_investigate",
			Mode:             "read_only",
			Command:          "meshclaw disk-investigate d1 /",
			PolicyAction:     "disk_investigate",
			PolicyDecision:   "allow",
			ApprovalRequired: false,
		},
		{
			Node:             "d1",
			Type:             "service_quarantine_candidate",
			Mode:             "approval_required",
			Command:          "meshclaw service-quarantine d1 open-webui",
			PolicyAction:     "service_quarantine",
			PolicyDecision:   "require_approval",
			ApprovalRequired: true,
		},
	}

	actions := applyAutohealPlanSafe(plan)
	if len(actions) != 2 {
		t.Fatalf("actions=%d, want 2", len(actions))
	}
	if !actions[0].Skipped || actions[0].SkipReason != "plan action is not auto_safe" {
		t.Fatalf("unexpected read-only action: %#v", actions[0])
	}
	if !actions[1].Skipped || actions[1].SkipReason != "policy does not allow unattended execution" {
		t.Fatalf("unexpected approval action: %#v", actions[1])
	}
}

func TestApplyAutohealPlanSafeSkipsAutoSafeWhenPolicyRequiresApproval(t *testing.T) {
	plan := []monitor.HealPlanAction{{
		Node:             "d1",
		Type:             "disk_cleanup",
		Mode:             "auto_safe",
		Command:          "sudo apt-get clean",
		PolicyAction:     "delete_data",
		PolicyDecision:   "require_approval",
		ApprovalRequired: true,
	}}

	actions := applyAutohealPlanSafe(plan)
	if len(actions) != 1 {
		t.Fatalf("actions=%d, want 1", len(actions))
	}
	if !actions[0].Skipped || actions[0].SkipReason != "policy does not allow unattended execution" {
		t.Fatalf("unexpected action: %#v", actions[0])
	}
}

func TestPolicyActionForHealType(t *testing.T) {
	cases := map[string]string{
		"disk_cleanup":      "autoheal_apply_safe",
		"memory_cleanup":    "autoheal_apply_safe",
		"restart_service":   "restart_service",
		"container_restart": "container_restart",
		"unknown":           "run_command",
	}
	for actionType, want := range cases {
		got := policyActionForHealType(actionType)
		if got != want {
			t.Fatalf("%s policy action = %q, want %q", actionType, got, want)
		}
	}
}

func serviceAuditReportForTest(host, service string) workflow.Report {
	return workflowReportForTest(host, "findings", service+".service failed")
}

func workflowReportForTest(host, status, evidence string) workflow.Report {
	return workflow.Report{
		Name:   "service-check",
		Host:   host,
		Status: status,
		Findings: []workflow.Finding{{
			Severity: "warning",
			Title:    "test",
			Evidence: evidence,
		}},
	}
}

func TestAutohealPlanTimeoutDefault(t *testing.T) {
	t.Setenv("MESHCLAW_AUTOHEAL_PLAN_TIMEOUT", "")
	if got := autohealPlanTimeout(); got != defaultAutohealPlanTimeout {
		t.Fatalf("default timeout = %s, want %s", got, defaultAutohealPlanTimeout)
	}
}

func TestAutohealPlanTimeoutOverride(t *testing.T) {
	t.Setenv("MESHCLAW_AUTOHEAL_PLAN_TIMEOUT", "3s")
	if got := autohealPlanTimeout(); got != 3*time.Second {
		t.Fatalf("override timeout = %s, want 3s", got)
	}
	// Invalid values fall back to the default instead of zero/blocking forever.
	t.Setenv("MESHCLAW_AUTOHEAL_PLAN_TIMEOUT", "nonsense")
	if got := autohealPlanTimeout(); got != defaultAutohealPlanTimeout {
		t.Fatalf("invalid override timeout = %s, want default %s", got, defaultAutohealPlanTimeout)
	}
}

func TestDegradedAutohealPlanReportIsPartialAndBounded(t *testing.T) {
	report := degradedAutohealPlanReport(2 * time.Second)
	if report.Status != "partial" {
		t.Fatalf("degraded status = %q, want partial", report.Status)
	}
	if report.Actions == nil {
		t.Fatal("degraded report Actions should be non-nil empty slice, got nil")
	}
	if len(report.Actions) != 0 {
		t.Fatalf("degraded report should have no candidate actions, got %d", len(report.Actions))
	}
	if len(report.Notes) == 0 {
		t.Fatal("degraded report should explain why the plan is partial")
	}
}

func TestCollectAutohealPlanInputsRespectsDeadline(t *testing.T) {
	// A tiny deadline must return timedOut rather than blocking on a live fleet
	// scan when an inventory node is unreachable.
	start := time.Now()
	_, timedOut := collectAutohealPlanInputs(1 * time.Millisecond)
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("collectAutohealPlanInputs blocked for %s past its deadline", elapsed)
	}
	_ = timedOut // may be false on a fast/no-node machine; the bound above is the contract
}
