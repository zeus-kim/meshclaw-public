package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/meshclaw/meshclaw/internal/evidence"
	"github.com/meshclaw/meshclaw/internal/nodestate"
)

func TestMCPReconcilePlanTool(t *testing.T) {
	tools := map[string]mcpTool{}
	for _, tool := range mcpTools() {
		tools[tool.Name] = tool
	}
	tool, ok := tools["meshclaw_reconcile_plan"]
	if !ok {
		t.Fatal("missing meshclaw_reconcile_plan")
	}
	if _, ok := tool.InputSchema.Properties["desired_path"]; !ok {
		t.Fatalf("desired_path schema missing: %#v", tool.InputSchema.Properties)
	}
}

func TestMCPReconcilePlanDryRun(t *testing.T) {
	dir := t.TempDir()
	desiredPath := filepath.Join(dir, "desired-state.yaml")
	actualPath := filepath.Join(dir, "g4.json")
	if err := os.WriteFile(desiredPath, []byte(`
nodes:
  g4:
    roles: [openwebui-worker]
    services:
      open-webui: running
`), 0600); err != nil {
		t.Fatal(err)
	}
	actualJSON, err := json.Marshal(nodestate.Report{
		NodeName:  "g4",
		Inventory: nodestate.InventoryHint{PrimaryRole: "openwebui-worker"},
		Services:  []nodestate.ServiceState{{Name: "open-webui", Active: "active"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(actualPath, actualJSON, 0600); err != nil {
		t.Fatal(err)
	}

	result, err := callMCPTool("meshclaw_reconcile_plan", map[string]interface{}{"desired_path": desiredPath, "actual_report_path": actualPath})
	if err != nil {
		t.Fatalf("callMCPTool error = %v", err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	report, ok := payload["report"].(reconcilePlanReport)
	if !ok {
		t.Fatalf("report type = %T payload=%#v", payload["report"], payload)
	}
	if report.Kind != "meshclaw_reconcile_plan" || report.Mode != "dry_run" {
		t.Fatalf("bad report: %+v", report)
	}
	if len(report.Results) != 1 || !report.Results[0].Matched {
		t.Fatalf("expected actual report to satisfy dry-run result: %+v", report.Results)
	}
	counts, ok := payload["counts"].(map[string]int)
	if !ok {
		t.Fatalf("counts type = %T payload=%#v", payload["counts"], payload)
	}
	if counts["matched"] != 1 || counts["actions"] != 0 || counts["actual_reports"] != 1 {
		t.Fatalf("bad MCP plan counts: %+v", counts)
	}
	if report.Counts["matched"] != counts["matched"] || report.Counts["actions"] != counts["actions"] {
		t.Fatalf("MCP counts diverged from report counts: report=%+v payload=%+v", report.Counts, counts)
	}
	if len(report.ActualPaths) != 1 || report.ActualPaths[0] != actualPath {
		t.Fatalf("actual paths = %#v", report.ActualPaths)
	}
	if payload["evidence"] == nil {
		t.Fatalf("missing evidence in payload: %#v", payload)
	}
	contract, ok := payload["reconcile_plan_contract"].(map[string]interface{})
	if !ok {
		t.Fatalf("reconcile plan contract missing: %#v", payload)
	}
	if contract["dry_run_only"] != true || contract["apply_allowed"] != false || contract["execute_implemented"] != false || contract["mutates_live_servers"] != false {
		t.Fatalf("reconcile plan contract should be dry-run and non-mutating: %#v", contract)
	}
	if contract["requires_validation_before_apply"] != true || contract["requires_approval_request_before_gate"] != true || contract["requires_apply_gate_before_executor"] != true || contract["requires_fresh_actual_evidence"] != true {
		t.Fatalf("reconcile plan contract should require validation, approval, gate, and actual evidence: %#v", contract)
	}
	if contract["actions"] != counts["actions"] || contract["approval_required_actions"] != counts["approval_required"] || contract["grants_future_approval"] != false {
		t.Fatalf("reconcile plan contract should mirror counts and grant no approval: %#v", contract)
	}
	stopBefore, _ := contract["stop_before"].([]string)
	if !containsString(stopBefore, "treating reconcile plan as live apply") {
		t.Fatalf("reconcile plan contract should block live-apply interpretation: %#v", contract)
	}
}

func TestMCPReconcilePlanIncludesPolicyAnnotations(t *testing.T) {
	dir := t.TempDir()
	desiredPath := filepath.Join(dir, "desired-state.yaml")
	if err := os.WriteFile(desiredPath, []byte(`
nodes:
  g4:
    roles: [missing-role]
`), 0600); err != nil {
		t.Fatal(err)
	}
	actualPath := filepath.Join(dir, "g4.json")
	actualJSON, err := json.Marshal(nodestate.Report{NodeName: "g4"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(actualPath, actualJSON, 0600); err != nil {
		t.Fatal(err)
	}
	result, err := callMCPTool("meshclaw_reconcile_plan", map[string]interface{}{"desired_path": desiredPath, "actual_report_path": actualPath})
	if err != nil {
		t.Fatalf("callMCPTool error = %v", err)
	}
	payload := result.(map[string]interface{})
	report := payload["report"].(reconcilePlanReport)
	if len(report.Results) != 1 || len(report.Results[0].Actions) != 1 {
		t.Fatalf("expected one action: %+v", report.Results)
	}
	action := report.Results[0].Actions[0]
	if action.PolicyDecision == "" || action.PolicyReason == "" {
		t.Fatalf("missing policy annotation: %+v", action)
	}
	counts := payload["counts"].(map[string]int)
	if counts["actions"] != 1 || counts["approval_required"] != 1 || counts["policy_require_approval"] != 1 {
		t.Fatalf("bad MCP policy counts: %+v", counts)
	}
	contract := payload["reconcile_plan_contract"].(map[string]interface{})
	if contract["actions"] != 1 || contract["approval_required_actions"] != 1 || contract["policy_require_approval_actions"] != 1 {
		t.Fatalf("reconcile plan contract should expose policy-gated action counts: %#v", contract)
	}
}

func TestMCPReconcilePlanRejectsExecute(t *testing.T) {
	_, err := callMCPTool("meshclaw_reconcile_plan", map[string]interface{}{"desired_path": "desired-state.yaml", "execute": true})
	if err == nil || !strings.Contains(err.Error(), "dry-run only") {
		t.Fatalf("expected dry-run rejection, got %v", err)
	}
}

func TestMCPReconcileValidateDesired(t *testing.T) {
	dir := t.TempDir()
	desiredPath := filepath.Join(dir, "desired-state.yaml")
	if err := os.WriteFile(desiredPath, []byte(`
nodes:
  g4:
    roles: [gpu]
    capacity:
      max_memory_used_pct: 85
`), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := callMCPTool("meshclaw_reconcile_validate_desired", map[string]interface{}{"desired_path": desiredPath})
	if err != nil {
		t.Fatalf("validate desired error = %v", err)
	}
	payload := result.(map[string]interface{})
	report := payload["desired_validation"].(reconcileDesiredValidationReport)
	if !report.Ready || report.Status != "ready" || report.Counts["nodes"] != 1 {
		t.Fatalf("bad desired validation: %+v", report)
	}
	if report.Handoff.Decision != "validated_for_dry_run_reconcile" || !containsString(report.Handoff.AllowedNextTools, "meshclaw_reconcile_plan") {
		t.Fatalf("desired validation should expose dry-run handoff: %+v", report.Handoff)
	}
	if !containsString(report.Handoff.StopBefore, "treating validation as apply approval") {
		t.Fatalf("desired validation handoff should block apply semantics: %+v", report.Handoff)
	}
	contract, ok := payload["desired_validation_contract"].(map[string]interface{})
	if !ok {
		t.Fatalf("desired validation contract missing: %#v", payload)
	}
	if contract["validation_only"] != true || contract["apply_allowed"] != false || contract["execute_implemented"] != false || contract["mutates_live_servers"] != false {
		t.Fatalf("desired validation contract should be validation-only and non-mutating: %#v", contract)
	}
	if contract["yaml_keys_grant_approval"] != false || contract["grants_future_approval"] != false || contract["allows_dry_run_reconcile"] != true {
		t.Fatalf("desired validation contract should allow only dry-run handoff without approval: %#v", contract)
	}
	if contract["requires_stored_validation_evidence"] != true || contract["requires_revalidation_on_yaml_change"] != true || contract["requires_approval_request_before_gate"] != true {
		t.Fatalf("desired validation contract should require stored evidence and approval request before gates: %#v", contract)
	}
}

func TestMCPReconcileValidateDesiredRejectsExecute(t *testing.T) {
	_, err := callMCPTool("meshclaw_reconcile_validate_desired", map[string]interface{}{"desired_path": "desired-state.yaml", "execute": true})
	if err == nil || !strings.Contains(err.Error(), "validates YAML only") {
		t.Fatalf("expected execute rejection, got %v", err)
	}
}

func TestMCPReconcileValidateDesiredContractIgnoresYAMLApplyKeys(t *testing.T) {
	dir := t.TempDir()
	desiredPath := filepath.Join(dir, "desired-state.yaml")
	if err := os.WriteFile(desiredPath, []byte(`
schema_version: v3
nodes:
  g4:
    execute: true
    services:
      docker: running
`), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := callMCPTool("meshclaw_reconcile_validate_desired", map[string]interface{}{"desired_path": desiredPath})
	if err != nil {
		t.Fatalf("validate desired error = %v", err)
	}
	payload := result.(map[string]interface{})
	report := payload["desired_validation"].(reconcileDesiredValidationReport)
	if !report.Ready || report.Counts["ignored_apply_keys"] != 1 {
		t.Fatalf("ignored YAML execute key should warn without blocking dry-run validation: %+v", report)
	}
	contract := payload["desired_validation_contract"].(map[string]interface{})
	if contract["ignored_apply_keys"] != 1 || contract["yaml_keys_grant_approval"] != false || contract["apply_allowed"] != false || contract["grants_future_approval"] != false {
		t.Fatalf("desired validation contract should expose ignored apply keys without approval: %#v", contract)
	}
	stopBefore, _ := contract["stop_before"].([]string)
	if !containsString(stopBefore, "treating ignored YAML apply/execute keys as approval") {
		t.Fatalf("desired validation contract should stop before ignored YAML approval semantics: %#v", contract)
	}
	refreshTriggers, _ := contract["refresh_triggers"].([]string)
	if !containsString(refreshTriggers, "ignored apply/execute YAML key changed") {
		t.Fatalf("desired validation contract should refresh when ignored YAML keys change: %#v", contract)
	}
}

func TestMCPReconcileRunOnceRequiresDryRun(t *testing.T) {
	tools := map[string]mcpTool{}
	for _, tool := range mcpTools() {
		tools[tool.Name] = tool
	}
	if _, ok := tools["meshclaw_reconcile_run_once"]; !ok {
		t.Fatal("missing meshclaw_reconcile_run_once")
	}
	_, err := callMCPTool("meshclaw_reconcile_run_once", map[string]interface{}{"desired_path": "desired-state.yaml"})
	if err == nil || !strings.Contains(err.Error(), "dry_run=true") {
		t.Fatalf("expected dry_run=true rejection, got %v", err)
	}
}

func TestMCPReconcileRunOnceDryRun(t *testing.T) {
	dir := t.TempDir()
	desiredPath := filepath.Join(dir, "desired-state.yaml")
	if err := os.WriteFile(desiredPath, []byte(`
nodes:
  g4:
    roles: [openwebui-worker]
`), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := callMCPTool("meshclaw_reconcile_run_once", map[string]interface{}{"desired_path": desiredPath, "dry_run": true})
	if err != nil {
		t.Fatalf("callMCPTool error = %v", err)
	}
	payload := result.(map[string]interface{})
	report := payload["report"].(reconcilePlanReport)
	if report.Kind != "meshclaw_reconcile_run_once" || report.Mode != "dry_run" {
		t.Fatalf("bad run-once report: %+v", report)
	}
}

func TestMCPReconcileApprovalRequest(t *testing.T) {
	tools := map[string]mcpTool{}
	for _, tool := range mcpTools() {
		tools[tool.Name] = tool
	}
	if _, ok := tools["meshclaw_reconcile_approval_request"]; !ok {
		t.Fatal("missing meshclaw_reconcile_approval_request")
	}
	dir := t.TempDir()
	desiredPath := filepath.Join(dir, "desired-state.yaml")
	actualPath := filepath.Join(dir, "g4.json")
	if err := os.WriteFile(desiredPath, []byte(`
nodes:
  g4:
    roles: [missing-role]
`), 0600); err != nil {
		t.Fatal(err)
	}
	actualJSON, err := json.Marshal(nodestate.Report{NodeName: "g4"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(actualPath, actualJSON, 0600); err != nil {
		t.Fatal(err)
	}
	result, err := callMCPTool("meshclaw_reconcile_approval_request", map[string]interface{}{"desired_path": desiredPath, "actual_report_path": actualPath})
	if err != nil {
		t.Fatalf("callMCPTool error = %v", err)
	}
	payload := result.(map[string]interface{})
	request := payload["approval_request"].(reconcileApprovalRequest)
	if request.Status != "approval_required" || len(request.Actions) != 1 {
		t.Fatalf("bad approval request: %+v", request)
	}
	if payload["evidence"] == nil {
		t.Fatalf("missing evidence: %#v", payload)
	}
	contract, ok := payload["approval_request_contract"].(map[string]interface{})
	if !ok {
		t.Fatalf("approval request contract missing: %#v", payload)
	}
	if contract["request_only"] != true || contract["apply_allowed"] != false || contract["execute_implemented"] != false || contract["mutates_live_servers"] != false {
		t.Fatalf("approval request contract should be request-only and non-mutating: %#v", contract)
	}
	if contract["approval_required"] != true || contract["actions_requiring_approval"] != 1 || contract["operator_approval_recorded"] != false {
		t.Fatalf("approval request contract should expose required approvals without recording approval: %#v", contract)
	}
	if contract["requires_apply_gate_after_approval"] != true || contract["requires_operator_approved_by_at_gate"] != true || contract["blocked_actions_must_not_execute"] != true || contract["grants_future_approval"] != false {
		t.Fatalf("approval request contract should require later gate and grant no approval: %#v", contract)
	}
	stopBefore, _ := contract["stop_before"].([]string)
	if !containsString(stopBefore, "treating approval-request evidence as operator approval") {
		t.Fatalf("approval request contract should block approval interpretation: %#v", contract)
	}
}

func TestMCPReconcileApprovalRequestRejectsExecute(t *testing.T) {
	_, err := callMCPTool("meshclaw_reconcile_approval_request", map[string]interface{}{"desired_path": "desired-state.yaml", "execute": true})
	if err == nil || !strings.Contains(err.Error(), "does not execute") {
		t.Fatalf("expected execute rejection, got %v", err)
	}
}

func TestMCPReconcileApplyGate(t *testing.T) {
	tools := map[string]mcpTool{}
	for _, tool := range mcpTools() {
		tools[tool.Name] = tool
	}
	if _, ok := tools["meshclaw_reconcile_apply_gate"]; !ok {
		t.Fatal("missing meshclaw_reconcile_apply_gate")
	}
	dir := t.TempDir()
	desiredPath := filepath.Join(dir, "desired-state.yaml")
	actualPath := filepath.Join(dir, "g4.json")
	if err := os.WriteFile(desiredPath, []byte(`
nodes:
  g4:
    roles: [missing-role]
`), 0600); err != nil {
		t.Fatal(err)
	}
	actualJSON, err := json.Marshal(nodestate.Report{NodeName: "g4"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(actualPath, actualJSON, 0600); err != nil {
		t.Fatal(err)
	}
	approvalPayload, err := callMCPTool("meshclaw_reconcile_approval_request", map[string]interface{}{"desired_path": desiredPath, "actual_report_path": actualPath})
	if err != nil {
		t.Fatalf("approval request error = %v", err)
	}
	approvalRecord := approvalPayload.(map[string]interface{})["evidence"].(evidence.Record)
	result, err := callMCPTool("meshclaw_reconcile_apply_gate", map[string]interface{}{"desired_path": desiredPath, "approval_evidence_path": approvalRecord.StoredAt, "approved_by": "zeus"})
	if err != nil {
		t.Fatalf("apply gate error = %v", err)
	}
	payload := result.(map[string]interface{})
	gate := payload["apply_gate"].(reconcileApplyGateReport)
	if !gate.Ready || gate.Status != "ready" || gate.ApprovedBy != "zeus" {
		t.Fatalf("bad apply gate: %+v", gate)
	}
	contract, ok := payload["apply_gate_contract"].(map[string]interface{})
	if !ok {
		t.Fatalf("apply gate contract missing: %#v", payload)
	}
	if contract["gate_only"] != true || contract["apply_allowed"] != false || contract["mutates_live_servers"] != false || contract["grants_future_approval"] != false {
		t.Fatalf("apply gate contract should be gate-only and non-mutating: %#v", contract)
	}
	if contract["approved_by_present"] != true || contract["requires_apply_plan_before_executor"] != true {
		t.Fatalf("apply gate contract should require approval identity and apply-plan before executor: %#v", contract)
	}
	stopBefore, _ := contract["stop_before"].([]string)
	if !containsString(stopBefore, "treating apply gate evidence as execution approval") {
		t.Fatalf("apply gate contract should preserve stop-before boundary: %#v", contract)
	}
}

func TestMCPReconcileApplyGateRejectsExecute(t *testing.T) {
	_, err := callMCPTool("meshclaw_reconcile_apply_gate", map[string]interface{}{"desired_path": "desired-state.yaml", "approval_evidence_path": "approval.json", "execute": true})
	if err == nil || !strings.Contains(err.Error(), "validates gates only") {
		t.Fatalf("expected execute rejection, got %v", err)
	}
}

func TestMCPReconcileApplyPlan(t *testing.T) {
	dir := t.TempDir()
	desiredPath := filepath.Join(dir, "desired-state.yaml")
	actualPath := filepath.Join(dir, "g4.json")
	if err := os.WriteFile(desiredPath, []byte(`
nodes:
  g4:
    roles: [missing-role]
`), 0600); err != nil {
		t.Fatal(err)
	}
	actualJSON, err := json.Marshal(nodestate.Report{NodeName: "g4"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(actualPath, actualJSON, 0600); err != nil {
		t.Fatal(err)
	}
	approvalPayload, err := callMCPTool("meshclaw_reconcile_approval_request", map[string]interface{}{"desired_path": desiredPath, "actual_report_path": actualPath})
	if err != nil {
		t.Fatalf("approval request error = %v", err)
	}
	approvalRecord := approvalPayload.(map[string]interface{})["evidence"].(evidence.Record)
	gatePayload, err := callMCPTool("meshclaw_reconcile_apply_gate", map[string]interface{}{"desired_path": desiredPath, "approval_evidence_path": approvalRecord.StoredAt, "approved_by": "zeus"})
	if err != nil {
		t.Fatalf("apply gate error = %v", err)
	}
	gateRecord := gatePayload.(map[string]interface{})["evidence"].(evidence.Record)
	result, err := callMCPTool("meshclaw_reconcile_apply_plan", map[string]interface{}{"gate_evidence_path": gateRecord.StoredAt})
	if err != nil {
		t.Fatalf("apply plan error = %v", err)
	}
	payload := result.(map[string]interface{})
	plan := payload["apply_plan"].(reconcileApplyPlanReport)
	if !plan.Ready || plan.Status != "ready" || len(plan.Steps) != 1 {
		t.Fatalf("bad apply plan: %+v", plan)
	}
	contract, ok := payload["apply_plan_contract"].(map[string]interface{})
	if !ok {
		t.Fatalf("apply plan contract missing: %#v", payload)
	}
	if contract["plan_only"] != true || contract["apply_allowed"] != false || contract["execute_implemented"] != false || contract["mutates_live_servers"] != false {
		t.Fatalf("apply plan contract should be plan-only and non-mutating: %#v", contract)
	}
	if contract["requires_execution_preview_before_executor"] != true || contract["requires_verification_after_executor"] != true {
		t.Fatalf("apply plan contract should require preview and verification before executor: %#v", contract)
	}
	stopBefore, _ := contract["stop_before"].([]string)
	if !containsString(stopBefore, "executing apply steps from chat") {
		t.Fatalf("apply plan contract should preserve stop-before boundary: %#v", contract)
	}
	if plan.Steps[0].Operation != "inventory_review" {
		t.Fatalf("unexpected operation: %+v", plan.Steps[0])
	}
}

func TestMCPReconcileApplyPlanRejectsExecute(t *testing.T) {
	_, err := callMCPTool("meshclaw_reconcile_apply_plan", map[string]interface{}{"gate_evidence_path": "gate.json", "execute": true})
	if err == nil || !strings.Contains(err.Error(), "builds a plan only") {
		t.Fatalf("expected execute rejection, got %v", err)
	}
}

func TestMCPReconcileExecutionPreview(t *testing.T) {
	dir := t.TempDir()
	desiredPath := filepath.Join(dir, "desired-state.yaml")
	actualPath := filepath.Join(dir, "g4.json")
	if err := os.WriteFile(desiredPath, []byte(`
nodes:
  g4:
    roles: [missing-role]
`), 0600); err != nil {
		t.Fatal(err)
	}
	actualJSON, err := json.Marshal(nodestate.Report{NodeName: "g4"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(actualPath, actualJSON, 0600); err != nil {
		t.Fatal(err)
	}
	approvalPayload, err := callMCPTool("meshclaw_reconcile_approval_request", map[string]interface{}{"desired_path": desiredPath, "actual_report_path": actualPath})
	if err != nil {
		t.Fatalf("approval request error = %v", err)
	}
	approvalRecord := approvalPayload.(map[string]interface{})["evidence"].(evidence.Record)
	gatePayload, err := callMCPTool("meshclaw_reconcile_apply_gate", map[string]interface{}{"desired_path": desiredPath, "approval_evidence_path": approvalRecord.StoredAt, "approved_by": "zeus"})
	if err != nil {
		t.Fatalf("apply gate error = %v", err)
	}
	gateRecord := gatePayload.(map[string]interface{})["evidence"].(evidence.Record)
	planPayload, err := callMCPTool("meshclaw_reconcile_apply_plan", map[string]interface{}{"gate_evidence_path": gateRecord.StoredAt})
	if err != nil {
		t.Fatalf("apply plan error = %v", err)
	}
	planRecord := planPayload.(map[string]interface{})["evidence"].(evidence.Record)
	result, err := callMCPTool("meshclaw_reconcile_execution_preview", map[string]interface{}{"apply_plan_evidence_path": planRecord.StoredAt})
	if err != nil {
		t.Fatalf("execution preview error = %v", err)
	}
	payload := result.(map[string]interface{})
	preview := payload["execution_preview"].(reconcileExecutionPreviewReport)
	if !preview.Ready || preview.Status != "ready" || len(preview.Commands) != 1 {
		t.Fatalf("bad execution preview: %+v", preview)
	}
	contract, ok := payload["execution_preview_contract"].(map[string]interface{})
	if !ok {
		t.Fatalf("execution preview contract missing: %#v", payload)
	}
	if contract["preview_only"] != true || contract["apply_allowed"] != false || contract["execute_implemented"] != false || contract["commands_are_inert_templates"] != true {
		t.Fatalf("execution preview contract should be inert and non-mutating: %#v", contract)
	}
	if contract["requires_verification_after_executor"] != true || contract["grants_future_approval"] != false {
		t.Fatalf("execution preview contract should require verification and grant no approval: %#v", contract)
	}
	stopBefore, _ := contract["stop_before"].([]string)
	if !containsString(stopBefore, "running preview command templates") {
		t.Fatalf("execution preview contract should preserve stop-before boundary: %#v", contract)
	}
	if preview.Commands[0].CommandTemplate == "" || !preview.Commands[0].RequiresVerification {
		t.Fatalf("bad command preview: %+v", preview.Commands[0])
	}
}

func TestMCPReconcileContainerMetadataFlow(t *testing.T) {
	dir := t.TempDir()
	desiredPath := filepath.Join(dir, "desired-state.yaml")
	actualPath := filepath.Join(dir, "g4.json")
	if err := os.WriteFile(desiredPath, []byte(`
nodes:
  g4:
    containers:
      open-webui:
        desired: running
        image: web:v2
        health: healthy
        restart: approval_required
`), 0600); err != nil {
		t.Fatal(err)
	}
	actualJSON, err := json.Marshal(nodestate.Report{
		NodeName: "g4",
		Docker: nodestate.DockerState{Containers: []nodestate.DockerContainer{{
			Name:          "open-webui",
			Image:         "web:v1",
			State:         "running",
			HealthStatus:  "unhealthy",
			RestartPolicy: "no",
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(actualPath, actualJSON, 0600); err != nil {
		t.Fatal(err)
	}
	approvalPayload, err := callMCPTool("meshclaw_reconcile_approval_request", map[string]interface{}{"desired_path": desiredPath, "actual_report_path": actualPath})
	if err != nil {
		t.Fatalf("approval request error = %v", err)
	}
	approvalRecord := approvalPayload.(map[string]interface{})["evidence"].(evidence.Record)
	gatePayload, err := callMCPTool("meshclaw_reconcile_apply_gate", map[string]interface{}{"desired_path": desiredPath, "approval_evidence_path": approvalRecord.StoredAt, "approved_by": "zeus"})
	if err != nil {
		t.Fatalf("apply gate error = %v", err)
	}
	gateRecord := gatePayload.(map[string]interface{})["evidence"].(evidence.Record)
	planPayload, err := callMCPTool("meshclaw_reconcile_apply_plan", map[string]interface{}{"gate_evidence_path": gateRecord.StoredAt})
	if err != nil {
		t.Fatalf("apply plan error = %v", err)
	}
	planRecord := planPayload.(map[string]interface{})["evidence"].(evidence.Record)
	previewPayload, err := callMCPTool("meshclaw_reconcile_execution_preview", map[string]interface{}{"apply_plan_evidence_path": planRecord.StoredAt})
	if err != nil {
		t.Fatalf("execution preview error = %v", err)
	}
	preview := previewPayload.(map[string]interface{})["execution_preview"].(reconcileExecutionPreviewReport)
	if len(preview.Commands) != 1 {
		t.Fatalf("commands = %+v", preview.Commands)
	}
	command := preview.Commands[0]
	for _, want := range []string{"--desired-state 'running'", "--desired-image 'web:v2'", "--desired-health 'healthy'", "--desired-restart 'approval_required'"} {
		if !strings.Contains(command.CommandTemplate, want) {
			t.Fatalf("command template missing %q: %+v", want, command)
		}
	}
	if command.Metadata["desired_image"] != "web:v2" || command.Metadata["actual_image"] != "web:v1" || command.Metadata["actual_health"] != "unhealthy" || command.Metadata["actual_restart"] != "no" {
		t.Fatalf("container metadata not propagated to preview: %+v", command.Metadata)
	}
	previewRecord := previewPayload.(map[string]interface{})["evidence"].(evidence.Record)
	verifyPayload, err := callMCPTool("meshclaw_reconcile_verification_plan", map[string]interface{}{"execution_preview_evidence_path": previewRecord.StoredAt})
	if err != nil {
		t.Fatalf("verification plan error = %v", err)
	}
	verification := verifyPayload.(map[string]interface{})["verification_plan"].(reconcileVerificationPlanReport)
	if len(verification.Checks) != 1 {
		t.Fatalf("checks = %+v", verification.Checks)
	}
	check := verification.Checks[0]
	if check.RequiredEvidence != "agent-collect+container-logscan" {
		t.Fatalf("required evidence = %q, want focused container logscan", check.RequiredEvidence)
	}
	if check.Metadata["desired_image"] != "web:v2" || check.Metadata["actual_image"] != "web:v1" || check.Metadata["desired_health"] != "healthy" || check.Metadata["actual_health"] != "unhealthy" || check.Metadata["actual_restart"] != "no" {
		t.Fatalf("container metadata not propagated to verification: %+v", check.Metadata)
	}
}

func TestMCPReconcileExecutionPreviewRejectsExecute(t *testing.T) {
	_, err := callMCPTool("meshclaw_reconcile_execution_preview", map[string]interface{}{"apply_plan_evidence_path": "plan.json", "execute": true})
	if err == nil || !strings.Contains(err.Error(), "renders previews only") {
		t.Fatalf("expected execute rejection, got %v", err)
	}
}

func TestMCPReconcileVerificationPlan(t *testing.T) {
	dir := t.TempDir()
	desiredPath := filepath.Join(dir, "desired-state.yaml")
	actualPath := filepath.Join(dir, "g4.json")
	if err := os.WriteFile(desiredPath, []byte(`
nodes:
  g4:
    roles: [missing-role]
`), 0600); err != nil {
		t.Fatal(err)
	}
	actualJSON, err := json.Marshal(nodestate.Report{NodeName: "g4"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(actualPath, actualJSON, 0600); err != nil {
		t.Fatal(err)
	}
	approvalPayload, err := callMCPTool("meshclaw_reconcile_approval_request", map[string]interface{}{"desired_path": desiredPath, "actual_report_path": actualPath})
	if err != nil {
		t.Fatalf("approval request error = %v", err)
	}
	approvalRecord := approvalPayload.(map[string]interface{})["evidence"].(evidence.Record)
	gatePayload, err := callMCPTool("meshclaw_reconcile_apply_gate", map[string]interface{}{"desired_path": desiredPath, "approval_evidence_path": approvalRecord.StoredAt, "approved_by": "zeus"})
	if err != nil {
		t.Fatalf("apply gate error = %v", err)
	}
	gateRecord := gatePayload.(map[string]interface{})["evidence"].(evidence.Record)
	planPayload, err := callMCPTool("meshclaw_reconcile_apply_plan", map[string]interface{}{"gate_evidence_path": gateRecord.StoredAt})
	if err != nil {
		t.Fatalf("apply plan error = %v", err)
	}
	planRecord := planPayload.(map[string]interface{})["evidence"].(evidence.Record)
	previewPayload, err := callMCPTool("meshclaw_reconcile_execution_preview", map[string]interface{}{"apply_plan_evidence_path": planRecord.StoredAt})
	if err != nil {
		t.Fatalf("execution preview error = %v", err)
	}
	previewRecord := previewPayload.(map[string]interface{})["evidence"].(evidence.Record)
	result, err := callMCPTool("meshclaw_reconcile_verification_plan", map[string]interface{}{"execution_preview_evidence_path": previewRecord.StoredAt})
	if err != nil {
		t.Fatalf("verification plan error = %v", err)
	}
	payload := result.(map[string]interface{})
	report := payload["verification_plan"].(reconcileVerificationPlanReport)
	if !report.Ready || report.Status != "ready" || len(report.Checks) != 1 {
		t.Fatalf("bad verification plan: %+v", report)
	}
	if report.Checks[0].RequiredEvidence != "node-inventory" {
		t.Fatalf("unexpected verification check: %+v", report.Checks[0])
	}
	contract, ok := payload["verification_plan_contract"].(map[string]interface{})
	if !ok {
		t.Fatalf("verification plan contract missing: %#v", payload)
	}
	if contract["plan_only"] != true || contract["apply_allowed"] != false || contract["execute_implemented"] != false || contract["mutates_live_servers"] != false {
		t.Fatalf("verification plan contract should be plan-only and non-mutating: %#v", contract)
	}
	if contract["requires_post_action_evidence"] != true || contract["requires_completion_plan_before_done"] != true || contract["grants_future_approval"] != false {
		t.Fatalf("verification plan contract should require evidence before completion: %#v", contract)
	}
	stopBefore, _ := contract["stop_before"].([]string)
	if !containsString(stopBefore, "treating verification plan as completed verification evidence") {
		t.Fatalf("verification plan contract should preserve completion boundary: %#v", contract)
	}
}

func TestMCPReconcileVerificationPlanRejectsExecute(t *testing.T) {
	_, err := callMCPTool("meshclaw_reconcile_verification_plan", map[string]interface{}{"execution_preview_evidence_path": "preview.json", "execute": true})
	if err == nil || !strings.Contains(err.Error(), "builds verification requirements only") {
		t.Fatalf("expected execute rejection, got %v", err)
	}
}

func TestMCPReconcileRunbook(t *testing.T) {
	dir := t.TempDir()
	desiredPath := filepath.Join(dir, "desired-state.yaml")
	actualPath := filepath.Join(dir, "g4.json")
	if err := os.WriteFile(desiredPath, []byte(`
nodes:
  g4:
    roles: [missing-role]
`), 0600); err != nil {
		t.Fatal(err)
	}
	actualJSON, err := json.Marshal(nodestate.Report{NodeName: "g4"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(actualPath, actualJSON, 0600); err != nil {
		t.Fatal(err)
	}
	approvalPayload, err := callMCPTool("meshclaw_reconcile_approval_request", map[string]interface{}{"desired_path": desiredPath, "actual_report_path": actualPath})
	if err != nil {
		t.Fatalf("approval request error = %v", err)
	}
	approvalRecord := approvalPayload.(map[string]interface{})["evidence"].(evidence.Record)
	gatePayload, err := callMCPTool("meshclaw_reconcile_apply_gate", map[string]interface{}{"desired_path": desiredPath, "approval_evidence_path": approvalRecord.StoredAt, "approved_by": "zeus"})
	if err != nil {
		t.Fatalf("apply gate error = %v", err)
	}
	gateRecord := gatePayload.(map[string]interface{})["evidence"].(evidence.Record)
	planPayload, err := callMCPTool("meshclaw_reconcile_apply_plan", map[string]interface{}{"gate_evidence_path": gateRecord.StoredAt})
	if err != nil {
		t.Fatalf("apply plan error = %v", err)
	}
	planRecord := planPayload.(map[string]interface{})["evidence"].(evidence.Record)
	previewPayload, err := callMCPTool("meshclaw_reconcile_execution_preview", map[string]interface{}{"apply_plan_evidence_path": planRecord.StoredAt})
	if err != nil {
		t.Fatalf("execution preview error = %v", err)
	}
	previewRecord := previewPayload.(map[string]interface{})["evidence"].(evidence.Record)
	verifyPayload, err := callMCPTool("meshclaw_reconcile_verification_plan", map[string]interface{}{"execution_preview_evidence_path": previewRecord.StoredAt})
	if err != nil {
		t.Fatalf("verification plan error = %v", err)
	}
	verifyRecord := verifyPayload.(map[string]interface{})["evidence"].(evidence.Record)
	result, err := callMCPTool("meshclaw_reconcile_runbook", map[string]interface{}{"verification_plan_evidence_path": verifyRecord.StoredAt})
	if err != nil {
		t.Fatalf("runbook error = %v", err)
	}
	payload := result.(map[string]interface{})
	runbook := payload["runbook"].(reconcileRunbookReport)
	if !runbook.Ready || runbook.Status != "ready" || len(runbook.Steps) != 1 {
		t.Fatalf("bad runbook: %+v", runbook)
	}
	if runbook.Steps[0].CommandTemplate == "" || runbook.Steps[0].RequiredEvidence == "" {
		t.Fatalf("bad runbook step: %+v", runbook.Steps[0])
	}
	contract, ok := payload["runbook_contract"].(map[string]interface{})
	if !ok {
		t.Fatalf("runbook contract missing: %#v", payload)
	}
	if contract["review_only"] != true || contract["apply_allowed"] != false || contract["execute_implemented"] != false || contract["mutates_live_servers"] != false {
		t.Fatalf("runbook contract should be review-only and non-mutating: %#v", contract)
	}
	if contract["requires_runbook_check"] != true || contract["requires_post_action_evidence"] != true || contract["grants_future_approval"] != false {
		t.Fatalf("runbook contract should require checks and grant no approval: %#v", contract)
	}
	stopBefore, _ := contract["stop_before"].([]string)
	if !containsString(stopBefore, "treating runbook text as automatic execution") {
		t.Fatalf("runbook contract should preserve review-only boundary: %#v", contract)
	}
}

func TestMCPReconcileRunbookRejectsExecute(t *testing.T) {
	_, err := callMCPTool("meshclaw_reconcile_runbook", map[string]interface{}{"verification_plan_evidence_path": "verify.json", "execute": true})
	if err == nil || !strings.Contains(err.Error(), "review-only") {
		t.Fatalf("expected execute rejection, got %v", err)
	}
}

func TestMCPReconcileRunbookCheck(t *testing.T) {
	dir := t.TempDir()
	desiredPath := filepath.Join(dir, "desired-state.yaml")
	actualPath := filepath.Join(dir, "g4.json")
	if err := os.WriteFile(desiredPath, []byte(`
nodes:
  g4:
    roles: [missing-role]
`), 0600); err != nil {
		t.Fatal(err)
	}
	actualJSON, err := json.Marshal(nodestate.Report{NodeName: "g4"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(actualPath, actualJSON, 0600); err != nil {
		t.Fatal(err)
	}
	approvalPayload, err := callMCPTool("meshclaw_reconcile_approval_request", map[string]interface{}{"desired_path": desiredPath, "actual_report_path": actualPath})
	if err != nil {
		t.Fatalf("approval request error = %v", err)
	}
	approvalRecord := approvalPayload.(map[string]interface{})["evidence"].(evidence.Record)
	gatePayload, err := callMCPTool("meshclaw_reconcile_apply_gate", map[string]interface{}{"desired_path": desiredPath, "approval_evidence_path": approvalRecord.StoredAt, "approved_by": "zeus"})
	if err != nil {
		t.Fatalf("apply gate error = %v", err)
	}
	gateRecord := gatePayload.(map[string]interface{})["evidence"].(evidence.Record)
	planPayload, err := callMCPTool("meshclaw_reconcile_apply_plan", map[string]interface{}{"gate_evidence_path": gateRecord.StoredAt})
	if err != nil {
		t.Fatalf("apply plan error = %v", err)
	}
	planRecord := planPayload.(map[string]interface{})["evidence"].(evidence.Record)
	previewPayload, err := callMCPTool("meshclaw_reconcile_execution_preview", map[string]interface{}{"apply_plan_evidence_path": planRecord.StoredAt})
	if err != nil {
		t.Fatalf("execution preview error = %v", err)
	}
	previewRecord := previewPayload.(map[string]interface{})["evidence"].(evidence.Record)
	verifyPayload, err := callMCPTool("meshclaw_reconcile_verification_plan", map[string]interface{}{"execution_preview_evidence_path": previewRecord.StoredAt})
	if err != nil {
		t.Fatalf("verification plan error = %v", err)
	}
	verifyRecord := verifyPayload.(map[string]interface{})["evidence"].(evidence.Record)
	runbookPayload, err := callMCPTool("meshclaw_reconcile_runbook", map[string]interface{}{"verification_plan_evidence_path": verifyRecord.StoredAt})
	if err != nil {
		t.Fatalf("runbook error = %v", err)
	}
	runbookRecord := runbookPayload.(map[string]interface{})["evidence"].(evidence.Record)
	result, err := callMCPTool("meshclaw_reconcile_runbook_check", map[string]interface{}{"runbook_evidence_path": runbookRecord.StoredAt})
	if err != nil {
		t.Fatalf("runbook check error = %v", err)
	}
	payload := result.(map[string]interface{})
	check := payload["runbook_check"].(reconcileRunbookCheckReport)
	if !check.Ready || check.Status != "ready" || len(check.Findings) != 0 {
		t.Fatalf("bad runbook check: %+v", check)
	}
	contract, ok := payload["runbook_check_contract"].(map[string]interface{})
	if !ok {
		t.Fatalf("runbook check contract missing: %#v", payload)
	}
	if contract["gate_only"] != true || contract["apply_allowed"] != false || contract["execute_implemented"] != false || contract["mutates_live_servers"] != false {
		t.Fatalf("runbook check contract should be gate-only and non-mutating: %#v", contract)
	}
	if contract["requires_zero_critical_findings"] != true || contract["critical_findings"] != 0 || contract["grants_future_approval"] != false {
		t.Fatalf("runbook check contract should require zero critical findings and grant no approval: %#v", contract)
	}
	if contract["requires_rollback_plan"] != true || contract["requires_completion_plan"] != true {
		t.Fatalf("runbook check contract should require rollback and completion planning: %#v", contract)
	}
	stopBefore, _ := contract["stop_before"].([]string)
	if !containsString(stopBefore, "treating runbook-check readiness as execution approval") {
		t.Fatalf("runbook check contract should preserve gate-only boundary: %#v", contract)
	}
}

func TestMCPReconcileRunbookCheckRejectsExecute(t *testing.T) {
	_, err := callMCPTool("meshclaw_reconcile_runbook_check", map[string]interface{}{"runbook_evidence_path": "runbook.json", "execute": true})
	if err == nil || !strings.Contains(err.Error(), "validates runbooks only") {
		t.Fatalf("expected execute rejection, got %v", err)
	}
}

func TestMCPReconcileRollbackPlan(t *testing.T) {
	dir := t.TempDir()
	desiredPath := filepath.Join(dir, "desired-state.yaml")
	actualPath := filepath.Join(dir, "g4.json")
	if err := os.WriteFile(desiredPath, []byte(`
nodes:
  g4:
    roles: [missing-role]
`), 0600); err != nil {
		t.Fatal(err)
	}
	actualJSON, err := json.Marshal(nodestate.Report{NodeName: "g4"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(actualPath, actualJSON, 0600); err != nil {
		t.Fatal(err)
	}
	approvalPayload, err := callMCPTool("meshclaw_reconcile_approval_request", map[string]interface{}{"desired_path": desiredPath, "actual_report_path": actualPath})
	if err != nil {
		t.Fatalf("approval request error = %v", err)
	}
	approvalRecord := approvalPayload.(map[string]interface{})["evidence"].(evidence.Record)
	gatePayload, err := callMCPTool("meshclaw_reconcile_apply_gate", map[string]interface{}{"desired_path": desiredPath, "approval_evidence_path": approvalRecord.StoredAt, "approved_by": "zeus"})
	if err != nil {
		t.Fatalf("apply gate error = %v", err)
	}
	gateRecord := gatePayload.(map[string]interface{})["evidence"].(evidence.Record)
	planPayload, err := callMCPTool("meshclaw_reconcile_apply_plan", map[string]interface{}{"gate_evidence_path": gateRecord.StoredAt})
	if err != nil {
		t.Fatalf("apply plan error = %v", err)
	}
	planRecord := planPayload.(map[string]interface{})["evidence"].(evidence.Record)
	previewPayload, err := callMCPTool("meshclaw_reconcile_execution_preview", map[string]interface{}{"apply_plan_evidence_path": planRecord.StoredAt})
	if err != nil {
		t.Fatalf("execution preview error = %v", err)
	}
	previewRecord := previewPayload.(map[string]interface{})["evidence"].(evidence.Record)
	verifyPayload, err := callMCPTool("meshclaw_reconcile_verification_plan", map[string]interface{}{"execution_preview_evidence_path": previewRecord.StoredAt})
	if err != nil {
		t.Fatalf("verification plan error = %v", err)
	}
	verifyRecord := verifyPayload.(map[string]interface{})["evidence"].(evidence.Record)
	runbookPayload, err := callMCPTool("meshclaw_reconcile_runbook", map[string]interface{}{"verification_plan_evidence_path": verifyRecord.StoredAt})
	if err != nil {
		t.Fatalf("runbook error = %v", err)
	}
	runbookRecord := runbookPayload.(map[string]interface{})["evidence"].(evidence.Record)
	checkPayload, err := callMCPTool("meshclaw_reconcile_runbook_check", map[string]interface{}{"runbook_evidence_path": runbookRecord.StoredAt})
	if err != nil {
		t.Fatalf("runbook check error = %v", err)
	}
	checkRecord := checkPayload.(map[string]interface{})["evidence"].(evidence.Record)
	result, err := callMCPTool("meshclaw_reconcile_rollback_plan", map[string]interface{}{"runbook_check_evidence_path": checkRecord.StoredAt})
	if err != nil {
		t.Fatalf("rollback plan error = %v", err)
	}
	payload := result.(map[string]interface{})
	rollback := payload["rollback_plan"].(reconcileRollbackPlanReport)
	if !rollback.Ready || rollback.Status != "ready" || len(rollback.Steps) != 1 {
		t.Fatalf("bad rollback plan: %+v", rollback)
	}
	contract, ok := payload["rollback_plan_contract"].(map[string]interface{})
	if !ok {
		t.Fatalf("rollback plan contract missing: %#v", payload)
	}
	if contract["plan_only"] != true || contract["apply_allowed"] != false || contract["rollback_allowed"] != false || contract["mutates_live_servers"] != false {
		t.Fatalf("rollback plan contract should be plan-only and non-mutating: %#v", contract)
	}
	if contract["requires_completion_plan"] != true || contract["requires_operator_approval"] != true || contract["grants_future_approval"] != false {
		t.Fatalf("rollback plan contract should require completion planning and approval: %#v", contract)
	}
	stopBefore, _ := contract["stop_before"].([]string)
	if !containsString(stopBefore, "treating rollback plan as automatic rollback execution") {
		t.Fatalf("rollback plan contract should preserve plan-only boundary: %#v", contract)
	}
}

func TestMCPReconcileRollbackPlanRejectsExecute(t *testing.T) {
	_, err := callMCPTool("meshclaw_reconcile_rollback_plan", map[string]interface{}{"runbook_check_evidence_path": "check.json", "execute": true})
	if err == nil || !strings.Contains(err.Error(), "builds rollback guidance only") {
		t.Fatalf("expected execute rejection, got %v", err)
	}
}

func TestMCPReconcileCompletionPlan(t *testing.T) {
	dir := t.TempDir()
	desiredPath := filepath.Join(dir, "desired-state.yaml")
	actualPath := filepath.Join(dir, "g4.json")
	if err := os.WriteFile(desiredPath, []byte(`
nodes:
  g4:
    roles: [missing-role]
`), 0600); err != nil {
		t.Fatal(err)
	}
	actualJSON, err := json.Marshal(nodestate.Report{NodeName: "g4"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(actualPath, actualJSON, 0600); err != nil {
		t.Fatal(err)
	}
	approvalPayload, err := callMCPTool("meshclaw_reconcile_approval_request", map[string]interface{}{"desired_path": desiredPath, "actual_report_path": actualPath})
	if err != nil {
		t.Fatalf("approval request error = %v", err)
	}
	approvalRecord := approvalPayload.(map[string]interface{})["evidence"].(evidence.Record)
	gatePayload, err := callMCPTool("meshclaw_reconcile_apply_gate", map[string]interface{}{"desired_path": desiredPath, "approval_evidence_path": approvalRecord.StoredAt, "approved_by": "zeus"})
	if err != nil {
		t.Fatalf("apply gate error = %v", err)
	}
	gateRecord := gatePayload.(map[string]interface{})["evidence"].(evidence.Record)
	planPayload, err := callMCPTool("meshclaw_reconcile_apply_plan", map[string]interface{}{"gate_evidence_path": gateRecord.StoredAt})
	if err != nil {
		t.Fatalf("apply plan error = %v", err)
	}
	planRecord := planPayload.(map[string]interface{})["evidence"].(evidence.Record)
	previewPayload, err := callMCPTool("meshclaw_reconcile_execution_preview", map[string]interface{}{"apply_plan_evidence_path": planRecord.StoredAt})
	if err != nil {
		t.Fatalf("execution preview error = %v", err)
	}
	previewRecord := previewPayload.(map[string]interface{})["evidence"].(evidence.Record)
	verifyPayload, err := callMCPTool("meshclaw_reconcile_verification_plan", map[string]interface{}{"execution_preview_evidence_path": previewRecord.StoredAt})
	if err != nil {
		t.Fatalf("verification plan error = %v", err)
	}
	verifyRecord := verifyPayload.(map[string]interface{})["evidence"].(evidence.Record)
	runbookPayload, err := callMCPTool("meshclaw_reconcile_runbook", map[string]interface{}{"verification_plan_evidence_path": verifyRecord.StoredAt})
	if err != nil {
		t.Fatalf("runbook error = %v", err)
	}
	runbookRecord := runbookPayload.(map[string]interface{})["evidence"].(evidence.Record)
	checkPayload, err := callMCPTool("meshclaw_reconcile_runbook_check", map[string]interface{}{"runbook_evidence_path": runbookRecord.StoredAt})
	if err != nil {
		t.Fatalf("runbook check error = %v", err)
	}
	checkRecord := checkPayload.(map[string]interface{})["evidence"].(evidence.Record)
	rollbackPayload, err := callMCPTool("meshclaw_reconcile_rollback_plan", map[string]interface{}{"runbook_check_evidence_path": checkRecord.StoredAt})
	if err != nil {
		t.Fatalf("rollback plan error = %v", err)
	}
	rollbackRecord := rollbackPayload.(map[string]interface{})["evidence"].(evidence.Record)
	result, err := callMCPTool("meshclaw_reconcile_completion_plan", map[string]interface{}{"rollback_plan_evidence_path": rollbackRecord.StoredAt})
	if err != nil {
		t.Fatalf("completion plan error = %v", err)
	}
	payload := result.(map[string]interface{})
	completion := payload["completion_plan"].(reconcileCompletionPlanReport)
	if !completion.Ready || completion.Status != "ready" || len(completion.Requirements) == 0 {
		t.Fatalf("bad completion plan: %+v", completion)
	}
	contract, ok := payload["completion_plan_contract"].(map[string]interface{})
	if !ok {
		t.Fatalf("completion plan contract missing: %#v", payload)
	}
	if contract["plan_only"] != true || contract["apply_allowed"] != false || contract["complete_allowed"] != false || contract["mutates_live_servers"] != false {
		t.Fatalf("completion plan contract should be plan-only and non-mutating: %#v", contract)
	}
	if contract["requires_final_evidence"] != true || contract["requires_readiness_summary"] != true || contract["grants_future_approval"] != false {
		t.Fatalf("completion plan contract should require final evidence and readiness summary: %#v", contract)
	}
	if contract["completion_requirements"] != len(completion.Requirements) || contract["requires_operator_visible_summary"] != true {
		t.Fatalf("completion plan contract should describe completion requirements: %#v", contract)
	}
	stopBefore, _ := contract["stop_before"].([]string)
	if !containsString(stopBefore, "declaring reconcile complete from completion-plan evidence") {
		t.Fatalf("completion plan contract should preserve completion boundary: %#v", contract)
	}
}

func TestMCPReconcileCompletionPlanRejectsExecute(t *testing.T) {
	_, err := callMCPTool("meshclaw_reconcile_completion_plan", map[string]interface{}{"rollback_plan_evidence_path": "rollback.json", "execute": true})
	if err == nil || !strings.Contains(err.Error(), "builds completion requirements only") {
		t.Fatalf("expected execute rejection, got %v", err)
	}
}

func TestMCPReconcileReadinessSummary(t *testing.T) {
	dir := t.TempDir()
	desiredPath := filepath.Join(dir, "desired-state.yaml")
	actualPath := filepath.Join(dir, "g4.json")
	if err := os.WriteFile(desiredPath, []byte(`
nodes:
  g4:
    roles: [missing-role]
`), 0600); err != nil {
		t.Fatal(err)
	}
	actualJSON, err := json.Marshal(nodestate.Report{NodeName: "g4"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(actualPath, actualJSON, 0600); err != nil {
		t.Fatal(err)
	}
	approvalPayload, err := callMCPTool("meshclaw_reconcile_approval_request", map[string]interface{}{"desired_path": desiredPath, "actual_report_path": actualPath})
	if err != nil {
		t.Fatalf("approval request error = %v", err)
	}
	approvalRecord := approvalPayload.(map[string]interface{})["evidence"].(evidence.Record)
	gatePayload, err := callMCPTool("meshclaw_reconcile_apply_gate", map[string]interface{}{"desired_path": desiredPath, "approval_evidence_path": approvalRecord.StoredAt, "approved_by": "zeus"})
	if err != nil {
		t.Fatalf("apply gate error = %v", err)
	}
	gateRecord := gatePayload.(map[string]interface{})["evidence"].(evidence.Record)
	planPayload, err := callMCPTool("meshclaw_reconcile_apply_plan", map[string]interface{}{"gate_evidence_path": gateRecord.StoredAt})
	if err != nil {
		t.Fatalf("apply plan error = %v", err)
	}
	planRecord := planPayload.(map[string]interface{})["evidence"].(evidence.Record)
	previewPayload, err := callMCPTool("meshclaw_reconcile_execution_preview", map[string]interface{}{"apply_plan_evidence_path": planRecord.StoredAt})
	if err != nil {
		t.Fatalf("execution preview error = %v", err)
	}
	previewRecord := previewPayload.(map[string]interface{})["evidence"].(evidence.Record)
	verifyPayload, err := callMCPTool("meshclaw_reconcile_verification_plan", map[string]interface{}{"execution_preview_evidence_path": previewRecord.StoredAt})
	if err != nil {
		t.Fatalf("verification plan error = %v", err)
	}
	verifyRecord := verifyPayload.(map[string]interface{})["evidence"].(evidence.Record)
	runbookPayload, err := callMCPTool("meshclaw_reconcile_runbook", map[string]interface{}{"verification_plan_evidence_path": verifyRecord.StoredAt})
	if err != nil {
		t.Fatalf("runbook error = %v", err)
	}
	runbookRecord := runbookPayload.(map[string]interface{})["evidence"].(evidence.Record)
	checkPayload, err := callMCPTool("meshclaw_reconcile_runbook_check", map[string]interface{}{"runbook_evidence_path": runbookRecord.StoredAt})
	if err != nil {
		t.Fatalf("runbook check error = %v", err)
	}
	checkRecord := checkPayload.(map[string]interface{})["evidence"].(evidence.Record)
	rollbackPayload, err := callMCPTool("meshclaw_reconcile_rollback_plan", map[string]interface{}{"runbook_check_evidence_path": checkRecord.StoredAt})
	if err != nil {
		t.Fatalf("rollback plan error = %v", err)
	}
	rollbackRecord := rollbackPayload.(map[string]interface{})["evidence"].(evidence.Record)
	completionPayload, err := callMCPTool("meshclaw_reconcile_completion_plan", map[string]interface{}{"rollback_plan_evidence_path": rollbackRecord.StoredAt})
	if err != nil {
		t.Fatalf("completion plan error = %v", err)
	}
	completionRecord := completionPayload.(map[string]interface{})["evidence"].(evidence.Record)
	result, err := callMCPTool("meshclaw_reconcile_readiness_summary", map[string]interface{}{"completion_plan_evidence_path": completionRecord.StoredAt})
	if err != nil {
		t.Fatalf("readiness summary error = %v", err)
	}
	payload := result.(map[string]interface{})
	summary := payload["readiness_summary"].(reconcileReadinessSummaryReport)
	if !summary.Ready || summary.Status != "ready" || len(summary.ReadyStages) == 0 || summary.Counts["completion_requirements"] == 0 {
		t.Fatalf("bad readiness summary: %+v", summary)
	}
	contract, ok := payload["readiness_contract"].(map[string]interface{})
	if !ok {
		t.Fatalf("readiness contract missing: %#v", payload)
	}
	if contract["summary_tools_are_approval"] != false || contract["apply_allowed"] != false || contract["grants_future_approval"] != false {
		t.Fatalf("readiness contract should not grant approval/apply: %#v", contract)
	}
	stopBefore, _ := contract["stop_before"].([]string)
	if !containsString(stopBefore, "treating readiness summary as operator approval") {
		t.Fatalf("readiness contract should preserve stop-before gates: %#v", contract)
	}
	for _, key := range []string{
		"readiness_summary_ready",
		"readiness_stages",
		"readiness_blockers",
		"apply_steps",
		"container_apply_steps",
		"verification_checks",
		"container_verification_checks",
		"completion_requirements",
		"container_completion_requirements",
	} {
		if _, ok := summary.Counts[key]; !ok {
			t.Fatalf("missing readiness count %q in %+v", key, summary.Counts)
		}
	}
}

func TestMCPReconcileReadinessSummaryRejectsExecute(t *testing.T) {
	_, err := callMCPTool("meshclaw_reconcile_readiness_summary", map[string]interface{}{"completion_plan_evidence_path": "completion.json", "execute": true})
	if err == nil || !strings.Contains(err.Error(), "summarizes readiness only") {
		t.Fatalf("expected execute rejection, got %v", err)
	}
}
