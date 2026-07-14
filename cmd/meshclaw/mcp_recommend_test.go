package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/meshclaw/meshclaw/internal/capability"
	"github.com/meshclaw/meshclaw/internal/datadoctor"
	"github.com/meshclaw/meshclaw/internal/evidence"
	"github.com/meshclaw/meshclaw/internal/inventory"
	"github.com/meshclaw/meshclaw/internal/mission"
	"github.com/meshclaw/meshclaw/internal/workflow"
)

func TestRecommendToolMCP(t *testing.T) {
	tests := []struct {
		name             string
		intent           string
		wantTool         string
		wantApproval     bool
		wantReasonSubstr string
	}{
		{
			name:             "cleanup plans before deletion",
			intent:           "d1 디스크 정리하고 중복 체크포인트 지워",
			wantTool:         "meshclaw_data_clean_plan",
			wantApproval:     true,
			wantReasonSubstr: "Cleanup should start with a plan",
		},
		{
			name:             "direct vssh transport question",
			intent:           "그냥 ssh 말고 vssh를 직접 써야 하는 경우가 뭐야",
			wantTool:         "direct vssh MCP",
			wantApproval:     false,
			wantReasonSubstr: "low-level transport primitives",
		},
		{
			name:             "service incident",
			intent:           "open-webui 서비스가 d1에서 죽는 것 같아 로그랑 상태 봐줘",
			wantTool:         "meshclaw_service_triage",
			wantApproval:     false,
			wantReasonSubstr: "Service incidents should be triaged first",
		},
		{
			name:             "server status management",
			intent:           "전체 서버 상태 보고 뭐 조치해야 하는지 알려줘",
			wantTool:         "meshclaw_ops_control",
			wantApproval:     false,
			wantReasonSubstr: "Broad server management",
		},
		{
			name:             "process resource workload",
			intent:           "서버에서 어떤 프로세스가 어떤 자원을 쓰고 무슨 용도인지 알려줘",
			wantTool:         "meshclaw_agent_workloads",
			wantApproval:     false,
			wantReasonSubstr: "process purpose",
		},
		{
			name:             "sensitive data leak check",
			intent:           "서버 전체에서 개인정보 민감정보가 새는지 확인해",
			wantTool:         "meshclaw_agent_security",
			wantApproval:     false,
			wantReasonSubstr: "cached fleet security posture",
		},
		{
			name:             "fleet security posture",
			intent:           "포트 방화벽 fail2ban cron 도커 포트 상태를 전체적으로 봐줘",
			wantTool:         "meshclaw_agent_security",
			wantApproval:     false,
			wantReasonSubstr: "public listeners",
		},
		{
			name:             "combined ops orchestration workflow",
			intent:           "메일과 올라마 워커를 합친 오케스트레이션 데모를 policy and evidence로 실행해",
			wantTool:         "meshclaw_workflow_run",
			wantApproval:     false,
			wantReasonSubstr: "Combined mail/model/server orchestration",
		},
		{
			name:             "inventory override before placement",
			intent:           "c1을 mail-server 역할로 등록하고 태그 mail,mox 붙여",
			wantTool:         "meshclaw_inventory_override_set",
			wantApproval:     false,
			wantReasonSubstr: "private fleet meaning",
		},
		{
			name:             "capability selection before action",
			intent:           "GPU 추론 작업은 어느 노드에서 돌리면 좋아?",
			wantTool:         "meshclaw_capability_recommend",
			wantApproval:     false,
			wantReasonSubstr: "placement or capability selection",
		},
		{
			name:             "data status doctor",
			intent:           "쌓이는 데이터가 꼬이지 않는지 확인해줘",
			wantTool:         "meshclaw_data_doctor",
			wantApproval:     false,
			wantReasonSubstr: "state JSON",
		},
		{
			name:             "automation schedule backlog",
			intent:           "자동화가 밀렸는지 스케줄 상태 봐줘",
			wantTool:         "meshclaw_schedule_status",
			wantApproval:     false,
			wantReasonSubstr: "scheduled automation is current or backlogged",
		},
		{
			name:             "schedule runner daemon health",
			intent:           "schedule-runner 데몬이 멈춘건지 launchd 상태 확인해",
			wantTool:         "meshclaw_daemon_schedule_status",
			wantApproval:     false,
			wantReasonSubstr: "launchd schedule-runner",
		},
		{
			name:             "correlated power event",
			intent:           "어제 전원 나갔는지 여러 서버가 동시에 재부팅됐는지 확인해줘",
			wantTool:         "meshclaw_opsdb_power_events",
			wantApproval:     false,
			wantReasonSubstr: "boot-history correlation",
		},
		{
			name:             "container self heal starts with plan",
			intent:           "g4의 unhealthy 도커 컨테이너를 복구하고 재시작 계획 세워",
			wantTool:         "meshclaw_autoheal_plan",
			wantApproval:     true,
			wantReasonSubstr: "approval-gated container apply/verification/runbook chain",
		},
		{
			name:             "container apply plan",
			intent:           "도커 컨테이너 apply plan을 autoheal evidence에서 만들어줘",
			wantTool:         "meshclaw_autoheal_container_apply_plan",
			wantApproval:     true,
			wantReasonSubstr: "plan-only and never executes docker commands",
		},
		{
			name:             "container verification plan",
			intent:           "도커 컨테이너 verification 계획과 logscan 확인 계획을 만들어줘",
			wantTool:         "meshclaw_autoheal_container_verification_plan",
			wantApproval:     true,
			wantReasonSubstr: "focused container-logscan requirements",
		},
		{
			name:             "container runbook",
			intent:           "도커 컨테이너 자가치유 런북과 작업 절차를 만들어줘",
			wantTool:         "meshclaw_autoheal_container_runbook",
			wantApproval:     true,
			wantReasonSubstr: "review-only remediation artifacts",
		},
		{
			name:             "container runbook check",
			intent:           "도커 컨테이너 runbook-check로 런북 검증해줘",
			wantTool:         "meshclaw_autoheal_container_runbook_check",
			wantApproval:     true,
			wantReasonSubstr: "gate-only and never executes docker commands",
		},
		{
			name:             "container rollback plan",
			intent:           "도커 컨테이너 롤백 계획을 만들어줘",
			wantTool:         "meshclaw_autoheal_container_rollback_plan",
			wantApproval:     true,
			wantReasonSubstr: "review-only rollback guidance",
		},
		{
			name:             "container completion plan",
			intent:           "도커 컨테이너 완료 조건과 completion plan을 만들어줘",
			wantTool:         "meshclaw_autoheal_container_completion_plan",
			wantApproval:     true,
			wantReasonSubstr: "final evidence requirements",
		},
		{
			name:             "container readiness summary",
			intent:           "도커 컨테이너 readiness summary 준비 상태 요약해줘",
			wantTool:         "meshclaw_autoheal_container_readiness_summary",
			wantApproval:     false,
			wantReasonSubstr: "summary-only view",
		},
		{
			name:             "container executor gate",
			intent:           "도커 컨테이너 executor gate admission 실행 게이트 확인해줘",
			wantTool:         "meshclaw_autoheal_container_executor_gate",
			wantApproval:     true,
			wantReasonSubstr: "readiness-summary evidence",
		},
		{
			name:             "container executor dry run",
			intent:           "도커 컨테이너 실제 실행기 executor dry-run preview 만들어줘",
			wantTool:         "meshclaw_autoheal_container_executor",
			wantApproval:     true,
			wantReasonSubstr: "dry-run preview",
		},
		{
			name:             "container executor verify closeout",
			intent:           "도커 컨테이너 실행 후 검증 executor verify closeout 해줘",
			wantTool:         "meshclaw_autoheal_container_executor_verify",
			wantApproval:     true,
			wantReasonSubstr: "post-action agent-collect",
		},
		{
			name:             "container apply loop gates",
			intent:           "도커 컨테이너 apply loop gates 준비 상태 확인해줘",
			wantTool:         "meshclaw_autoheal_container_readiness_summary",
			wantApproval:     false,
			wantReasonSubstr: "apply-loop gates",
		},
		{
			name:             "service discovery lb plan",
			intent:           "openwebui 서비스 디스커버리와 lb-lite 라우팅 계획 만들어줘",
			wantTool:         "meshclaw_service_registry_plan",
			wantApproval:     true,
			wantReasonSubstr: "endpoint inventory",
		},
		{
			name:             "capacity scale plan",
			intent:           "vllm GPU workload 오토스케일 용량 계획 만들어줘",
			wantTool:         "meshclaw_capacity_scale_plan",
			wantApproval:     true,
			wantReasonSubstr: "utilization signals",
		},
		{
			name:             "storage guardrail plan",
			intent:           "d1 디스크 압박과 백업 guardrail 계획 만들어줘",
			wantTool:         "meshclaw_storage_guardrail_plan",
			wantApproval:     true,
			wantReasonSubstr: "disk, mount, volume dependency",
		},
		{
			name:             "ops integration plan",
			intent:           "Prometheus Grafana Loki Ansible 기존 운영툴을 MCP 대화로 연동하는 계획 만들어줘",
			wantTool:         "meshclaw_ops_integration_plan",
			wantApproval:     true,
			wantReasonSubstr: "semantic evidence sources",
		},
		{
			name:             "mcp rollout plan",
			intent:           "새 MCP 도구가 Claude에서 보이게 바이너리 갱신 후 어떻게 써보는지 계획해줘",
			wantTool:         "meshclaw_mcp_rollout_plan",
			wantApproval:     false,
			wantReasonSubstr: "built into the MeshClaw binary",
		},
		{
			name:             "mcp smoke test plan",
			intent:           "새 MCP 도구 확인용 smoke test 계획 만들어줘",
			wantTool:         "meshclaw_mcp_smoke_test_plan",
			wantApproval:     false,
			wantReasonSubstr: "read-only smoke sequence",
		},
		{
			name:             "mcp profile visibility check",
			intent:           "claude-lite profile에서 새 MCP 도구 노출 확인해줘",
			wantTool:         "meshclaw_mcp_profile_visibility_check",
			wantApproval:     false,
			wantReasonSubstr: "actually exposed",
		},
		{
			name:             "automation rule plan",
			intent:           "openwebui 컨테이너 죽으면 자동 복구되게 자동화 규칙 계획 만들어줘",
			wantTool:         "meshclaw_automation_rule_plan",
			wantApproval:     true,
			wantReasonSubstr: "trigger, condition, evidence",
		},
		{
			name:             "automation rule check",
			intent:           "automation rule check /tmp/automation-rule-plan.json approved_by zeus 검증해줘",
			wantTool:         "meshclaw_automation_rule_check",
			wantApproval:     true,
			wantReasonSubstr: "Gate-only",
		},
		{
			name:             "automation rule readiness summary",
			intent:           "automation rule readiness summary /tmp/automation-rule-check.json 준비 상태 요약해줘",
			wantTool:         "meshclaw_automation_rule_readiness_summary",
			wantApproval:     false,
			wantReasonSubstr: "final summary-only view",
		},
		{
			name:             "automation rule writer plan",
			intent:           "automation rule writer plan /tmp/automation-rule-readiness-summary.json 규칙 작성 계획만 만들어줘",
			wantTool:         "meshclaw_automation_rule_writer_plan",
			wantApproval:     true,
			wantReasonSubstr: "future writer",
		},
		{
			name:             "desired state yaml validation",
			intent:           "desired-state YAML을 먼저 검증하고 문제만 알려줘",
			wantTool:         "meshclaw_reconcile_validate_desired",
			wantApproval:     false,
			wantReasonSubstr: "auto_apply keys are warnings only",
		},
		{
			name:             "desired state reconcile dry run",
			intent:           "원하는 상태 YAML로 현재 서버 상태를 맞추는 reconcile 계획을 dry-run으로 만들어줘",
			wantTool:         "meshclaw_reconcile_plan",
			wantApproval:     false,
			wantReasonSubstr: "dry-run plan",
		},
		{
			name:             "desired state approval request",
			intent:           "desired-state YAML 변경사항 승인 요청 evidence를 만들어줘",
			wantTool:         "meshclaw_reconcile_approval_request",
			wantApproval:     true,
			wantReasonSubstr: "approval-request evidence",
		},
		{
			name:             "desired state apply gate",
			intent:           "reconcile approval evidence로 desired-state apply gate 준비 상태 확인해줘",
			wantTool:         "meshclaw_reconcile_apply_gate",
			wantApproval:     true,
			wantReasonSubstr: "gate-only and never mutates servers",
		},
		{
			name:             "desired state apply plan",
			intent:           "desired-state apply plan을 gate evidence에서 만들어줘",
			wantTool:         "meshclaw_reconcile_apply_plan",
			wantApproval:     true,
			wantReasonSubstr: "plan-only and never executes server changes",
		},
		{
			name:             "desired state execution preview",
			intent:           "reconcile execution preview로 명령 미리보기만 만들어줘",
			wantTool:         "meshclaw_reconcile_execution_preview",
			wantApproval:     true,
			wantReasonSubstr: "previews only and never executes commands",
		},
		{
			name:             "desired state runbook",
			intent:           "desired-state reconcile 런북과 작업 절차를 만들어줘",
			wantTool:         "meshclaw_reconcile_runbook",
			wantApproval:     true,
			wantReasonSubstr: "review-only artifacts",
		},
		{
			name:             "desired state runbook check",
			intent:           "desired-state reconcile runbook-check로 런북 검증해줘",
			wantTool:         "meshclaw_reconcile_runbook_check",
			wantApproval:     true,
			wantReasonSubstr: "gate-only and never executes server changes",
		},
		{
			name:             "desired state rollback plan",
			intent:           "desired-state reconcile 롤백 계획을 만들어줘",
			wantTool:         "meshclaw_reconcile_rollback_plan",
			wantApproval:     true,
			wantReasonSubstr: "review-only rollback guidance",
		},
		{
			name:             "desired state completion plan",
			intent:           "desired-state reconcile 완료 조건과 completion plan을 만들어줘",
			wantTool:         "meshclaw_reconcile_completion_plan",
			wantApproval:     true,
			wantReasonSubstr: "final evidence requirements",
		},
		{
			name:             "desired state readiness summary",
			intent:           "desired-state reconcile readiness summary 준비 상태 요약해줘",
			wantTool:         "meshclaw_reconcile_readiness_summary",
			wantApproval:     false,
			wantReasonSubstr: "summary-only view",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := recommendToolMCP(tt.intent, "codex")
			if gotTool := stringMapValue(got, "recommended_tool"); gotTool != tt.wantTool {
				t.Fatalf("recommended_tool=%q want %q, got=%v", gotTool, tt.wantTool, got)
			}
			if gotApproval, _ := got["approval_expected"].(bool); gotApproval != tt.wantApproval {
				t.Fatalf("approval_expected=%v want %v, got=%v", gotApproval, tt.wantApproval, got)
			}
			policy, _ := got["execution_policy"].(map[string]interface{})
			if policy["recommendation_only"] != true || policy["executes_changes"] != false {
				t.Fatalf("execution_policy should mark recommendation as non-executing: %v", got)
			}
			if policy["requires_approval"] != tt.wantApproval {
				t.Fatalf("execution_policy.requires_approval=%v want %v, got=%v", policy["requires_approval"], tt.wantApproval, got)
			}
			requiredEvidence, _ := policy["required_evidence"].([]string)
			if !containsString(requiredEvidence, "stored evidence path from the recommended tool response") {
				t.Fatalf("execution_policy should expose required evidence contract: %v", got)
			}
			stopBefore, _ := policy["stop_before"].([]string)
			if !containsString(stopBefore, "raw shell execution outside MeshClaw evidence") {
				t.Fatalf("execution_policy should expose stop-before boundary: %v", got)
			}
			if tt.wantApproval {
				if !containsString(requiredEvidence, "explicit operator approval evidence before any mutating executor") {
					t.Fatalf("approval-gated recommendation should require operator approval evidence: %v", got)
				}
				if !containsString(stopBefore, "apply=true or execute=true without operator approval evidence") {
					t.Fatalf("approval-gated recommendation should stop before apply/execute: %v", got)
				}
			}
			contract, _ := got["evidence_contract"].(map[string]interface{})
			if contract["confidence_expected"] != true || contract["safe_checks_first"] != true || contract["mutating_actions_gated"] != true {
				t.Fatalf("evidence_contract should require confidence, safe checks, and gated mutation: %v", got)
			}
			sections, _ := contract["expected_sections"].([]string)
			for _, section := range []string{"evidence", "interpretation", "likely_causes", "remediation_options", "rollback"} {
				if !containsString(sections, section) {
					t.Fatalf("evidence_contract missing %q: %v", section, got)
				}
			}
			if reason := stringMapValue(got, "reason"); !strings.Contains(reason, tt.wantReasonSubstr) {
				t.Fatalf("reason=%q does not contain %q", reason, tt.wantReasonSubstr)
			}
			if tt.name == "combined ops orchestration workflow" {
				args, _ := got["arguments"].(map[string]interface{})
				if args["name"] != "meshclaw-ops-orchestration-demo" {
					t.Fatalf("workflow name=%v, got=%v", args["name"], got)
				}
			}
			if tt.name == "container self heal starts with plan" {
				alternatives, _ := got["alternatives"].([]string)
				if !containsString(alternatives, "meshclaw_autoheal_container_apply_plan") || !containsString(alternatives, "meshclaw_autoheal_container_readiness_summary") {
					t.Fatalf("container self-heal alternatives should expose planning chain: %v", got)
				}
				avoid, _ := got["avoid"].([]string)
				if !containsString(avoid, "direct docker restart without evidence") {
					t.Fatalf("container self-heal should avoid direct docker restart: %v", got)
				}
			}
			if tt.name == "container readiness summary" || tt.name == "container apply loop gates" {
				avoid, _ := got["avoid"].([]string)
				if !containsString(avoid, "executing docker commands from summary text") {
					t.Fatalf("container readiness summary should avoid execution semantics: %v", got)
				}
				if !containsString(avoid, "skipping apply-loop gate evidence") {
					t.Fatalf("container readiness summary should require apply-loop gate evidence: %v", got)
				}
				if !containsString(requiredEvidence, "container completion-plan evidence with stop_before gates and final container-logscan requirements") {
					t.Fatalf("container readiness summary should require stop-before completion evidence: %v", got)
				}
				if !containsString(stopBefore, "starting container executor without ready completion-plan evidence") {
					t.Fatalf("container readiness summary should stop before executor start without ready evidence: %v", got)
				}
			}
			if tt.name == "container executor gate" {
				avoid, _ := got["avoid"].([]string)
				if !containsString(avoid, "executing docker commands from gate evidence") {
					t.Fatalf("container executor gate should avoid command execution: %v", got)
				}
				if !containsString(requiredEvidence, "ready container-readiness-summary evidence and approved_by") {
					t.Fatalf("container executor gate should require readiness summary evidence: %v", got)
				}
				if !containsString(stopBefore, "treating executor-gate readiness as live execution approval") {
					t.Fatalf("container executor gate should stop before live approval semantics: %v", got)
				}
			}
			if tt.name == "container executor dry run" {
				avoid, _ := got["avoid"].([]string)
				if !containsString(avoid, "running without exact live approval phrase") {
					t.Fatalf("container executor should avoid missing approval phrase: %v", got)
				}
				if !containsString(requiredEvidence, "exact live approval phrase before execute=true can mutate containers") {
					t.Fatalf("container executor should require exact live approval phrase: %v", got)
				}
				if !containsString(stopBefore, "execute=true before dry-run preview and exact live approval phrase") {
					t.Fatalf("container executor should stop before unapproved live execution: %v", got)
				}
			}
			if tt.name == "container executor verify closeout" {
				avoid, _ := got["avoid"].([]string)
				if !containsString(avoid, "skipping post-action container-logscan evidence") {
					t.Fatalf("container executor verify should avoid skipping logscan evidence: %v", got)
				}
				if !containsString(requiredEvidence, "post-action agent-collect evidence for every executed container step") {
					t.Fatalf("container executor verify should require post-action agent evidence: %v", got)
				}
				if !containsString(requiredEvidence, "focused container-logscan evidence for every executed container step") {
					t.Fatalf("container executor verify should require focused post-action logscan evidence: %v", got)
				}
				if !containsString(stopBefore, "closing self-heal before post-action agent and container-logscan evidence") {
					t.Fatalf("container executor verify should stop before evidence-free closeout: %v", got)
				}
			}
			if tt.name == "container apply plan" {
				if !containsString(requiredEvidence, "autoheal-plan evidence with container action candidate and approved_by") {
					t.Fatalf("container apply recommendation should require autoheal approval evidence: %v", got)
				}
				if !containsString(stopBefore, "docker restart/recreate from recommendation text") {
					t.Fatalf("container apply recommendation should block direct docker mutation: %v", got)
				}
			}
			if tt.name == "service discovery lb plan" {
				avoid, _ := got["avoid"].([]string)
				if !containsString(avoid, "editing proxy config directly") {
					t.Fatalf("service registry plan should avoid direct proxy edits: %v", got)
				}
			}
			if tt.name == "capacity scale plan" {
				avoid, _ := got["avoid"].([]string)
				if !containsString(avoid, "provider create without approval") {
					t.Fatalf("capacity scale plan should avoid provider mutation: %v", got)
				}
			}
			if tt.name == "storage guardrail plan" {
				avoid, _ := got["avoid"].([]string)
				if !containsString(avoid, "rm before backup evidence") {
					t.Fatalf("storage guardrail plan should avoid destructive cleanup: %v", got)
				}
			}
			if tt.name == "ops integration plan" {
				avoid, _ := got["avoid"].([]string)
				if !containsString(avoid, "letting the model use raw tool APIs without evidence mapping") {
					t.Fatalf("ops integration plan should avoid raw APIs without evidence mapping: %v", got)
				}
			}
			if tt.name == "mcp rollout plan" {
				avoid, _ := got["avoid"].([]string)
				if !containsString(avoid, "assuming PR code is already in the running MCP binary") {
					t.Fatalf("mcp rollout plan should avoid stale binary assumptions: %v", got)
				}
			}
			if tt.name == "mcp smoke test plan" {
				avoid, _ := got["avoid"].([]string)
				if !containsString(avoid, "using smoke tests to mutate servers") {
					t.Fatalf("mcp smoke test plan should avoid mutation: %v", got)
				}
			}
			if tt.name == "automation rule plan" {
				avoid, _ := got["avoid"].([]string)
				if !containsString(avoid, "enabling auto-apply from chat") {
					t.Fatalf("automation rule plan should avoid enabling auto apply: %v", got)
				}
			}
			if tt.name == "desired state reconcile dry run" {
				avoid, _ := got["avoid"].([]string)
				if !containsString(avoid, "kubectl-style live apply") {
					t.Fatalf("desired-state reconcile should avoid live apply semantics: %v", got)
				}
			}
			if tt.name == "desired state yaml validation" {
				avoid, _ := got["avoid"].([]string)
				if !containsString(avoid, "treating YAML apply/execute/auto_apply keys as approval") {
					t.Fatalf("desired-state validation should warn that YAML apply keys are not approval: %v", got)
				}
			}
			if tt.name == "desired state apply gate" {
				avoid, _ := got["avoid"].([]string)
				if !containsString(avoid, "treating gate readiness as execution") {
					t.Fatalf("desired-state apply gate should avoid execution semantics: %v", got)
				}
				if !containsString(requiredEvidence, "reconcile approval-request evidence and approved_by") {
					t.Fatalf("desired-state apply gate should require approval-request evidence: %v", got)
				}
			}
			if tt.name == "desired state execution preview" {
				avoid, _ := got["avoid"].([]string)
				if !containsString(avoid, "running preview commands") {
					t.Fatalf("desired-state execution preview should avoid command execution: %v", got)
				}
			}
			if tt.name == "desired state readiness summary" {
				avoid, _ := got["avoid"].([]string)
				if !containsString(avoid, "executing changes from summary text") {
					t.Fatalf("desired-state readiness summary should avoid execution semantics: %v", got)
				}
				if !containsString(requiredEvidence, "reconcile completion-plan evidence with stop_before gates and approval/apply-gate/verification evidence") {
					t.Fatalf("desired-state readiness summary should require stop-before completion evidence: %v", got)
				}
				if !containsString(stopBefore, "starting reconcile executor without ready completion-plan evidence") {
					t.Fatalf("desired-state readiness summary should stop before executor start without ready evidence: %v", got)
				}
			}
		})
	}
}

func TestRecommendToolMCPSeparatesContainerLogsFromSelfHeal(t *testing.T) {
	logs := recommendToolMCP("g4 unhealthy 도커 컨테이너 로그 분석해서 에러 패턴만 찾아줘", "codex")
	if gotTool := stringMapValue(logs, "recommended_tool"); gotTool != "meshclaw_analyze_logs" {
		t.Fatalf("container log intent recommended_tool=%q want meshclaw_analyze_logs, got=%v", gotTool, logs)
	}
	if approval, _ := logs["approval_expected"].(bool); approval {
		t.Fatalf("container log analysis should remain read-only: %v", logs)
	}
	if reason := stringMapValue(logs, "reason"); !strings.Contains(reason, "container:<name>") {
		t.Fatalf("container log intent should mention container source guidance: %v", logs)
	}
	if reason := stringMapValue(logs, "reason"); !strings.Contains(reason, "unit identity and system/user scope") {
		t.Fatalf("log intent should mention unit identity gate for systemd logs: %v", logs)
	}
	avoid, _ := logs["avoid"].([]string)
	if !containsString(avoid, "container apply/restart planning before focused container-logscan evidence") {
		t.Fatalf("log intent should avoid container apply/restart planning before focused logscan: %v", logs)
	}
	if !containsString(avoid, "service restart before unit identity and scope are confirmed") {
		t.Fatalf("log intent should avoid service restart before unit identity: %v", logs)
	}
	executionPolicy, _ := logs["execution_policy"].(map[string]interface{})
	requiredEvidence, _ := executionPolicy["required_evidence"].([]string)
	if !containsString(requiredEvidence, "for container logs, use source=container:<name> and retain focused container-logscan evidence before apply planning") {
		t.Fatalf("log intent should require focused container logscan evidence: %v", logs)
	}
	if !containsString(requiredEvidence, "for container logs, inspect autoheal_handoff.runtime_evidence_checklist and satisfy docker inspect status/health items before apply planning") {
		t.Fatalf("log intent should require runtime evidence checklist review: %v", logs)
	}
	if !containsString(requiredEvidence, "for container self-heal planning, retain fresh runtime inspect/status evidence with image, status, health, ports, and restart policy") {
		t.Fatalf("log intent should require fresh container runtime evidence before self-heal planning: %v", logs)
	}
	if !containsString(requiredEvidence, "for systemd logs, retain unit_candidates and follow with targeted meshclaw_service_check evidence before restart planning") {
		t.Fatalf("log intent should require unit candidate service-check follow-up: %v", logs)
	}
	stopBefore, _ := executionPolicy["stop_before"].([]string)
	if !containsString(stopBefore, "container apply or restart planning before focused container-logscan evidence is reviewed") {
		t.Fatalf("log intent should stop before container apply/restart planning without focused logscan: %v", logs)
	}
	if !containsString(stopBefore, "container apply planning before fresh runtime inspect/status evidence is reviewed") {
		t.Fatalf("log intent should stop before container apply planning without runtime evidence: %v", logs)
	}
	if !containsString(stopBefore, "systemd restart planning before unit_candidates and targeted service-check evidence are reviewed") {
		t.Fatalf("log intent should stop before systemd restart planning without unit service-check evidence: %v", logs)
	}

	selfHeal := recommendToolMCP("g4 unhealthy 도커 컨테이너를 복구하고 재시작 계획 세워", "codex")
	if gotTool := stringMapValue(selfHeal, "recommended_tool"); gotTool != "meshclaw_autoheal_plan" {
		t.Fatalf("container self-heal intent recommended_tool=%q want meshclaw_autoheal_plan, got=%v", gotTool, selfHeal)
	}
	if approval, _ := selfHeal["approval_expected"].(bool); !approval {
		t.Fatalf("container self-heal should expect approval-gated planning: %v", selfHeal)
	}
}

func TestRecommendToolMCPInfersDesiredStatePath(t *testing.T) {
	tests := []struct {
		name string
		text string
		tool string
	}{
		{
			name: "validate",
			text: "desired-state YAML /tmp/meshclaw-desired.yaml 검증해줘",
			tool: "meshclaw_reconcile_validate_desired",
		},
		{
			name: "plan",
			text: "원하는 상태 YAML ./ops/desired.yml reconcile 계획 만들어줘",
			tool: "meshclaw_reconcile_plan",
		},
		{
			name: "approval",
			text: "desired-state YAML /srv/mesh/desired.yaml 승인 요청 evidence 만들어줘",
			tool: "meshclaw_reconcile_approval_request",
		},
		{
			name: "apply gate",
			text: "desired-state YAML /srv/mesh/desired.yaml apply gate 준비 상태 확인해줘",
			tool: "meshclaw_reconcile_apply_gate",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := recommendToolMCP(tt.text, "codex")
			if stringMapValue(got, "recommended_tool") != tt.tool {
				t.Fatalf("recommended_tool=%q want %q, got=%v", stringMapValue(got, "recommended_tool"), tt.tool, got)
			}
			args, _ := got["arguments"].(map[string]interface{})
			if path, _ := args["desired_path"].(string); !strings.HasSuffix(path, ".yaml") && !strings.HasSuffix(path, ".yml") {
				t.Fatalf("desired_path should infer yaml path: %+v", got)
			}
		})
	}
}

func TestRecommendToolMCPInfersApplyGateEvidencePath(t *testing.T) {
	got := recommendToolMCP("desired-state YAML /srv/mesh/desired.yaml apply gate /tmp/reconcile-approval.json 확인해줘", "codex")
	if stringMapValue(got, "recommended_tool") != "meshclaw_reconcile_apply_gate" {
		t.Fatalf("recommended_tool=%q want meshclaw_reconcile_apply_gate, got=%v", stringMapValue(got, "recommended_tool"), got)
	}
	args, _ := got["arguments"].(map[string]interface{})
	if args["desired_path"] != "/srv/mesh/desired.yaml" {
		t.Fatalf("desired_path should be inferred: %+v", got)
	}
	if args["approval_evidence_path"] != "/tmp/reconcile-approval.json" {
		t.Fatalf("approval_evidence_path should be inferred: %+v", got)
	}
}

func TestRecommendToolMCPInfersApplyPlanGateEvidencePath(t *testing.T) {
	got := recommendToolMCP("desired-state apply plan /tmp/reconcile-apply-gate.json 만들어줘", "codex")
	if stringMapValue(got, "recommended_tool") != "meshclaw_reconcile_apply_plan" {
		t.Fatalf("recommended_tool=%q want meshclaw_reconcile_apply_plan, got=%v", stringMapValue(got, "recommended_tool"), got)
	}
	args, _ := got["arguments"].(map[string]interface{})
	if args["gate_evidence_path"] != "/tmp/reconcile-apply-gate.json" {
		t.Fatalf("gate_evidence_path should be inferred: %+v", got)
	}
}

func TestRecommendToolMCPInfersExecutionPreviewApplyPlanPath(t *testing.T) {
	got := recommendToolMCP("reconcile execution preview /tmp/reconcile-apply-plan.json 명령 미리보기만 만들어줘", "codex")
	if stringMapValue(got, "recommended_tool") != "meshclaw_reconcile_execution_preview" {
		t.Fatalf("recommended_tool=%q want meshclaw_reconcile_execution_preview, got=%v", stringMapValue(got, "recommended_tool"), got)
	}
	args, _ := got["arguments"].(map[string]interface{})
	if args["apply_plan_evidence_path"] != "/tmp/reconcile-apply-plan.json" {
		t.Fatalf("apply_plan_evidence_path should be inferred: %+v", got)
	}
}

func TestRecommendToolMCPInfersRunbookVerificationPath(t *testing.T) {
	got := recommendToolMCP("desired-state runbook /tmp/reconcile-verification-plan.json 기반으로 만들어줘", "codex")
	if stringMapValue(got, "recommended_tool") != "meshclaw_reconcile_runbook" {
		t.Fatalf("recommended_tool=%q want meshclaw_reconcile_runbook, got=%v", stringMapValue(got, "recommended_tool"), got)
	}
	args, _ := got["arguments"].(map[string]interface{})
	if args["verification_plan_evidence_path"] != "/tmp/reconcile-verification-plan.json" {
		t.Fatalf("verification_plan_evidence_path should be inferred: %+v", got)
	}
}

func TestRecommendToolMCPInfersRunbookCheckRunbookPath(t *testing.T) {
	got := recommendToolMCP("desired-state runbook-check /tmp/reconcile-runbook.json 검증해줘", "codex")
	if stringMapValue(got, "recommended_tool") != "meshclaw_reconcile_runbook_check" {
		t.Fatalf("recommended_tool=%q want meshclaw_reconcile_runbook_check, got=%v", stringMapValue(got, "recommended_tool"), got)
	}
	args, _ := got["arguments"].(map[string]interface{})
	if args["runbook_evidence_path"] != "/tmp/reconcile-runbook.json" {
		t.Fatalf("runbook_evidence_path should be inferred: %+v", got)
	}
}

func TestRecommendToolMCPInfersRollbackRunbookCheckPath(t *testing.T) {
	got := recommendToolMCP("desired-state rollback plan /tmp/reconcile-runbook-check.json 기준으로 만들어줘", "codex")
	if stringMapValue(got, "recommended_tool") != "meshclaw_reconcile_rollback_plan" {
		t.Fatalf("recommended_tool=%q want meshclaw_reconcile_rollback_plan, got=%v", stringMapValue(got, "recommended_tool"), got)
	}
	args, _ := got["arguments"].(map[string]interface{})
	if args["runbook_check_evidence_path"] != "/tmp/reconcile-runbook-check.json" {
		t.Fatalf("runbook_check_evidence_path should be inferred: %+v", got)
	}
}

func TestRecommendToolMCPInfersCompletionRollbackPath(t *testing.T) {
	got := recommendToolMCP("desired-state completion plan /tmp/reconcile-rollback-plan.json 완료 조건 만들어줘", "codex")
	if stringMapValue(got, "recommended_tool") != "meshclaw_reconcile_completion_plan" {
		t.Fatalf("recommended_tool=%q want meshclaw_reconcile_completion_plan, got=%v", stringMapValue(got, "recommended_tool"), got)
	}
	args, _ := got["arguments"].(map[string]interface{})
	if args["rollback_plan_evidence_path"] != "/tmp/reconcile-rollback-plan.json" {
		t.Fatalf("rollback_plan_evidence_path should be inferred: %+v", got)
	}
}

func TestRecommendToolMCPInfersReadinessCompletionPath(t *testing.T) {
	got := recommendToolMCP("desired-state readiness summary /tmp/reconcile-completion-plan.json 준비 상태 요약해줘", "codex")
	if stringMapValue(got, "recommended_tool") != "meshclaw_reconcile_readiness_summary" {
		t.Fatalf("recommended_tool=%q want meshclaw_reconcile_readiness_summary, got=%v", stringMapValue(got, "recommended_tool"), got)
	}
	args, _ := got["arguments"].(map[string]interface{})
	if args["completion_plan_evidence_path"] != "/tmp/reconcile-completion-plan.json" {
		t.Fatalf("completion_plan_evidence_path should be inferred: %+v", got)
	}
}

func TestRecommendToolMCPInfersContainerVerificationApplyPlanPath(t *testing.T) {
	got := recommendToolMCP("docker verification plan /tmp/container-apply-plan.json 기준으로 만들어줘", "codex")
	if stringMapValue(got, "recommended_tool") != "meshclaw_autoheal_container_verification_plan" {
		t.Fatalf("recommended_tool=%q want meshclaw_autoheal_container_verification_plan, got=%v", stringMapValue(got, "recommended_tool"), got)
	}
	args, _ := got["arguments"].(map[string]interface{})
	if args["container_apply_plan_evidence_path"] != "/tmp/container-apply-plan.json" {
		t.Fatalf("container_apply_plan_evidence_path should be inferred: %+v", got)
	}
	executionPolicy, _ := got["execution_policy"].(map[string]interface{})
	requiredEvidence, _ := executionPolicy["required_evidence"].([]string)
	if !containsString(requiredEvidence, "container_apply_plan_contract.runtime_evidence_required_count preserved as apply_plan_runtime_evidence_required_count") {
		t.Fatalf("container verification recommendation should require apply-plan runtime evidence count preservation: %+v", got)
	}
}

func TestRecommendToolMCPInfersContainerRunbookVerificationPath(t *testing.T) {
	got := recommendToolMCP("docker runbook /tmp/container-verification-plan.json 기준으로 만들어줘", "codex")
	if stringMapValue(got, "recommended_tool") != "meshclaw_autoheal_container_runbook" {
		t.Fatalf("recommended_tool=%q want meshclaw_autoheal_container_runbook, got=%v", stringMapValue(got, "recommended_tool"), got)
	}
	args, _ := got["arguments"].(map[string]interface{})
	if args["container_verification_plan_evidence_path"] != "/tmp/container-verification-plan.json" {
		t.Fatalf("container_verification_plan_evidence_path should be inferred: %+v", got)
	}
	executionPolicy, _ := got["execution_policy"].(map[string]interface{})
	requiredEvidence, _ := executionPolicy["required_evidence"].([]string)
	if !containsString(requiredEvidence, "container verification-plan evidence with apply_plan_runtime_evidence_required_count") {
		t.Fatalf("container runbook recommendation should require runtime evidence count handoff: %+v", got)
	}
}

func TestRecommendToolMCPInfersContainerRunbookCheckRunbookPath(t *testing.T) {
	got := recommendToolMCP("docker runbook-check /tmp/container-runbook.json 검증해줘", "codex")
	if stringMapValue(got, "recommended_tool") != "meshclaw_autoheal_container_runbook_check" {
		t.Fatalf("recommended_tool=%q want meshclaw_autoheal_container_runbook_check, got=%v", stringMapValue(got, "recommended_tool"), got)
	}
	args, _ := got["arguments"].(map[string]interface{})
	if args["container_runbook_evidence_path"] != "/tmp/container-runbook.json" {
		t.Fatalf("container_runbook_evidence_path should be inferred: %+v", got)
	}
}

func TestRecommendToolMCPInfersContainerRollbackRunbookCheckPath(t *testing.T) {
	got := recommendToolMCP("docker rollback plan /tmp/container-runbook-check.json 기준으로 만들어줘", "codex")
	if stringMapValue(got, "recommended_tool") != "meshclaw_autoheal_container_rollback_plan" {
		t.Fatalf("recommended_tool=%q want meshclaw_autoheal_container_rollback_plan, got=%v", stringMapValue(got, "recommended_tool"), got)
	}
	args, _ := got["arguments"].(map[string]interface{})
	if args["container_runbook_check_evidence_path"] != "/tmp/container-runbook-check.json" {
		t.Fatalf("container_runbook_check_evidence_path should be inferred: %+v", got)
	}
}

func TestRecommendToolMCPInfersContainerCompletionRollbackPath(t *testing.T) {
	got := recommendToolMCP("docker completion plan /tmp/container-rollback-plan.json 완료 조건 만들어줘", "codex")
	if stringMapValue(got, "recommended_tool") != "meshclaw_autoheal_container_completion_plan" {
		t.Fatalf("recommended_tool=%q want meshclaw_autoheal_container_completion_plan, got=%v", stringMapValue(got, "recommended_tool"), got)
	}
	args, _ := got["arguments"].(map[string]interface{})
	if args["container_rollback_plan_evidence_path"] != "/tmp/container-rollback-plan.json" {
		t.Fatalf("container_rollback_plan_evidence_path should be inferred: %+v", got)
	}
}

func TestRecommendToolMCPInfersContainerReadinessCompletionPath(t *testing.T) {
	got := recommendToolMCP("docker readiness summary /tmp/container-completion-plan.json 준비 상태 요약해줘", "codex")
	if stringMapValue(got, "recommended_tool") != "meshclaw_autoheal_container_readiness_summary" {
		t.Fatalf("recommended_tool=%q want meshclaw_autoheal_container_readiness_summary, got=%v", stringMapValue(got, "recommended_tool"), got)
	}
	args, _ := got["arguments"].(map[string]interface{})
	if args["container_completion_plan_evidence_path"] != "/tmp/container-completion-plan.json" {
		t.Fatalf("container_completion_plan_evidence_path should be inferred: %+v", got)
	}
}

func TestRecommendToolMCPInfersContainerApplyLoopGateCompletionPath(t *testing.T) {
	got := recommendToolMCP("docker apply-loop gates /tmp/container-completion-plan.json 확인해줘", "codex")
	if stringMapValue(got, "recommended_tool") != "meshclaw_autoheal_container_readiness_summary" {
		t.Fatalf("recommended_tool=%q want meshclaw_autoheal_container_readiness_summary, got=%v", stringMapValue(got, "recommended_tool"), got)
	}
	args, _ := got["arguments"].(map[string]interface{})
	if args["container_completion_plan_evidence_path"] != "/tmp/container-completion-plan.json" {
		t.Fatalf("container_completion_plan_evidence_path should be inferred: %+v", got)
	}
}

func TestRecommendToolMCPInfersContainerApplyAutohealPlanPath(t *testing.T) {
	got := recommendToolMCP("docker apply plan /tmp/autoheal-plan.json 승인 준비해줘", "codex")
	if stringMapValue(got, "recommended_tool") != "meshclaw_autoheal_container_apply_plan" {
		t.Fatalf("recommended_tool=%q want meshclaw_autoheal_container_apply_plan, got=%v", stringMapValue(got, "recommended_tool"), got)
	}
	args, _ := got["arguments"].(map[string]interface{})
	if args["plan_evidence_path"] != "/tmp/autoheal-plan.json" {
		t.Fatalf("plan_evidence_path should be inferred: %+v", got)
	}
	executionPolicy, _ := got["execution_policy"].(map[string]interface{})
	requiredEvidence, _ := executionPolicy["required_evidence"].([]string)
	if !containsString(requiredEvidence, "analyze_logs handoff_contract.apply_allowed=false when the action is based on container logscan evidence") {
		t.Fatalf("container apply-plan recommendation should require non-apply logscan handoff evidence: %+v", got)
	}
	if !containsString(requiredEvidence, "container_apply_plan_contract.direct_restart_allowed=false and requires_focused_runtime_evidence=true before verification planning") {
		t.Fatalf("container apply-plan recommendation should require runtime gate contract evidence: %+v", got)
	}
	if !containsString(requiredEvidence, "container_apply_plan_contract.runtime_evidence_required_count must match planned container steps") {
		t.Fatalf("container apply-plan recommendation should require runtime evidence count: %+v", got)
	}
}

func TestMCPSurfaceIncludesWorkflowValidate(t *testing.T) {
	surface := mcpSurfaceGuide()
	defaultTools, ok := surface["default_tools"].([]string)
	if !ok {
		t.Fatalf("default_tools missing: %#v", surface)
	}
	if !containsString(defaultTools, "meshclaw_workflow_validate") {
		t.Fatalf("default tools missing workflow validate: %#v", defaultTools)
	}
	if !containsString(defaultTools, "meshclaw_workflow_scaffold") {
		t.Fatalf("default tools missing workflow scaffold: %#v", defaultTools)
	}
	if !containsString(defaultTools, "meshclaw_workflow_run") {
		t.Fatalf("default tools missing workflow run: %#v", defaultTools)
	}
	if !containsString(defaultTools, "meshclaw_workflow_plan_execute") {
		t.Fatalf("default tools missing workflow plan execute: %#v", defaultTools)
	}
	if !containsString(defaultTools, "meshclaw_inventory_override_list") {
		t.Fatalf("default tools missing inventory override list: %#v", defaultTools)
	}
	if !containsString(defaultTools, "meshclaw_quickstart") {
		t.Fatalf("default tools missing quickstart: %#v", defaultTools)
	}
	if !containsString(defaultTools, "meshclaw_doctor") {
		t.Fatalf("default tools missing doctor: %#v", defaultTools)
	}
	if !containsString(defaultTools, "meshclaw_setup_assistant") {
		t.Fatalf("default tools missing assistant setup: %#v", defaultTools)
	}
	if !containsString(defaultTools, "meshclaw_setup_signal") {
		t.Fatalf("default tools missing signal setup: %#v", defaultTools)
	}
	if !containsString(defaultTools, "meshclaw_daemon_schedule_status") {
		t.Fatalf("default tools missing schedule daemon status: %#v", defaultTools)
	}
	if !containsString(defaultTools, "meshclaw_setup_argos_runner") {
		t.Fatalf("default tools missing argos runner setup: %#v", defaultTools)
	}
	if !containsString(defaultTools, "meshclaw_capability_recommend") {
		t.Fatalf("default tools missing capability recommend: %#v", defaultTools)
	}
	if !containsString(defaultTools, "meshclaw_service_registry_plan") {
		t.Fatalf("default tools missing service registry plan: %#v", defaultTools)
	}
	if !containsString(defaultTools, "meshclaw_capacity_scale_plan") {
		t.Fatalf("default tools missing capacity scale plan: %#v", defaultTools)
	}
	if !containsString(defaultTools, "meshclaw_storage_guardrail_plan") {
		t.Fatalf("default tools missing storage guardrail plan: %#v", defaultTools)
	}
	if !containsString(defaultTools, "meshclaw_ops_integration_plan") {
		t.Fatalf("default tools missing ops integration plan: %#v", defaultTools)
	}
	if !containsString(defaultTools, "meshclaw_mcp_rollout_plan") {
		t.Fatalf("default tools missing mcp rollout plan: %#v", defaultTools)
	}
	if !containsString(defaultTools, "meshclaw_mcp_smoke_test_plan") {
		t.Fatalf("default tools missing mcp smoke test plan: %#v", defaultTools)
	}
	if !containsString(defaultTools, "meshclaw_mcp_profile_visibility_check") {
		t.Fatalf("default tools missing mcp profile visibility check: %#v", defaultTools)
	}
	if !containsString(defaultTools, "meshclaw_automation_rule_plan") {
		t.Fatalf("default tools missing automation rule plan: %#v", defaultTools)
	}
	if !containsString(defaultTools, "meshclaw_automation_rule_check") {
		t.Fatalf("default tools missing automation rule check: %#v", defaultTools)
	}
	if !containsString(defaultTools, "meshclaw_automation_rule_readiness_summary") {
		t.Fatalf("default tools missing automation rule readiness summary: %#v", defaultTools)
	}
	if !containsString(defaultTools, "meshclaw_automation_rule_writer_plan") {
		t.Fatalf("default tools missing automation rule writer plan: %#v", defaultTools)
	}
	if !containsString(defaultTools, "meshclaw_agent_workloads") || !containsString(defaultTools, "meshclaw_agent_changes") || !containsString(defaultTools, "meshclaw_agent_security") {
		t.Fatalf("default tools missing agent workload state tools: %#v", defaultTools)
	}
	if !containsString(defaultTools, "meshclaw_agent_inventory_plan") {
		t.Fatalf("default tools missing agent inventory plan: %#v", defaultTools)
	}
	if !containsString(defaultTools, "meshclaw_data_doctor") {
		t.Fatalf("default tools missing data doctor: %#v", defaultTools)
	}
	path, ok := surface["default_path"].([]map[string]interface{})
	if !ok || len(path) < 6 {
		t.Fatalf("default_path missing: %#v", surface["default_path"])
	}
	if path[1]["tool"] != "meshclaw_quickstart" || path[2]["tool"] != "meshclaw_doctor" || path[6]["tool"] != "meshclaw_server_list" || path[10]["tool"] != "meshclaw_capability_recommend" {
		t.Fatalf("default path should start with inventory and capability recommendation: %#v", path)
	}
	loop, ok := surface["canonical_loop"].([]string)
	if !ok || !containsString(loop, "workflow dry-run") {
		t.Fatalf("canonical loop missing workflow dry-run: %#v", surface["canonical_loop"])
	}
	for _, want := range []string{"desired-state validation", "reconcile plan", "container self-heal plan"} {
		if !containsString(loop, want) {
			t.Fatalf("canonical loop missing %s: %#v", want, loop)
		}
	}
	for _, want := range []string{"reconcile readiness summary", "container readiness summary", "readiness evidence review before executor"} {
		if !containsString(loop, want) {
			t.Fatalf("canonical loop missing readiness gate step %s: %#v", want, loop)
		}
	}
	if !containsString(loop, "service registry plan") {
		t.Fatalf("canonical loop missing service registry plan: %#v", loop)
	}
	if !containsString(loop, "capacity scale plan") {
		t.Fatalf("canonical loop missing capacity scale plan: %#v", loop)
	}
	if !containsString(loop, "storage guardrail plan") {
		t.Fatalf("canonical loop missing storage guardrail plan: %#v", loop)
	}
	if !containsString(loop, "ops integration plan") {
		t.Fatalf("canonical loop missing ops integration plan: %#v", loop)
	}
	if !containsString(loop, "mcp rollout plan") {
		t.Fatalf("canonical loop missing mcp rollout plan: %#v", loop)
	}
	if !containsString(loop, "mcp smoke test plan") {
		t.Fatalf("canonical loop missing mcp smoke test plan: %#v", loop)
	}
	if !containsString(loop, "mcp profile visibility check") {
		t.Fatalf("canonical loop missing mcp profile visibility check: %#v", loop)
	}
	if !containsString(loop, "automation rule plan") {
		t.Fatalf("canonical loop missing automation rule plan: %#v", loop)
	}
	if !containsString(loop, "automation rule check") {
		t.Fatalf("canonical loop missing automation rule check: %#v", loop)
	}
	if !containsString(loop, "automation rule readiness summary") {
		t.Fatalf("canonical loop missing automation rule readiness summary: %#v", loop)
	}
	if !containsString(loop, "automation rule writer plan") {
		t.Fatalf("canonical loop missing automation rule writer plan: %#v", loop)
	}
	readinessGateSequence, ok := surface["readiness_gate_sequence"].([]map[string]interface{})
	if !ok || len(readinessGateSequence) != 2 {
		t.Fatalf("readiness gate sequence missing: %#v", surface["readiness_gate_sequence"])
	}
	if !containsReadinessGate(readinessGateSequence, "desired_state_reconcile", "meshclaw_reconcile_readiness_summary", "starting reconcile executor without ready completion-plan evidence") {
		t.Fatalf("readiness gate sequence missing desired-state stop-before contract: %#v", readinessGateSequence)
	}
	if !containsReadinessGate(readinessGateSequence, "container_self_heal", "meshclaw_autoheal_container_readiness_summary", "starting container executor without ready completion-plan evidence") {
		t.Fatalf("readiness gate sequence missing container stop-before contract: %#v", readinessGateSequence)
	}
	layers, ok := surface["runtime_layers"].([]map[string]interface{})
	if !ok || !containsMapWithID(layers, "meshclaw_core_runtime") || !containsMapWithID(layers, "meshclaw_worker") || !containsMapWithID(layers, "execution_transport") {
		t.Fatalf("runtime layers should separate core runtime, worker, and transport: %#v", surface["runtime_layers"])
	}
	principles, ok := surface["semantic_ops_principles"].([]string)
	if !ok || !containsString(principles, "Prefer semantic tools over ad hoc shell commands.") || !containsString(principles, "Install full runtime on core nodes and lightweight workers on fleet nodes.") {
		t.Fatalf("semantic ops principles should guide AI/runtime boundaries: %#v", surface["semantic_ops_principles"])
	}
	evidenceContracts, ok := surface["evidence_contracts"].([]string)
	if !ok ||
		!containsString(evidenceContracts, "container logscan can return autoheal_handoff.runtime_evidence_checklist before self-heal planning") ||
		!containsString(evidenceContracts, "readiness summaries are checkpoints, not operator approval") {
		t.Fatalf("evidence contracts should guide checklist review before execution: %#v", surface["evidence_contracts"])
	}

	result, err := callMCPTool("meshclaw_workflow_list", map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("payload=%#v", result)
	}
	if payload["validate_tool"] != "meshclaw_workflow_validate" {
		t.Fatalf("validate_tool=%v", payload["validate_tool"])
	}
	if payload["scaffold_tool"] != "meshclaw_workflow_scaffold" {
		t.Fatalf("scaffold_tool=%v", payload["scaffold_tool"])
	}
	if payload["plan_execute_tool"] != "meshclaw_workflow_plan_execute" {
		t.Fatalf("plan_execute_tool=%v", payload["plan_execute_tool"])
	}
}

func TestMCPAIGuideExposesReadinessGateContract(t *testing.T) {
	result, err := callMCPTool("meshclaw_ai_guide", map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("payload=%#v", result)
	}
	contract, ok := payload["readiness_gate_contract"].(map[string]interface{})
	if !ok {
		t.Fatalf("readiness gate contract missing: %#v", payload)
	}
	if contract["summary_tools_are_approval"] != false || contract["mutates_live_servers"] != false {
		t.Fatalf("readiness guide contract should be non-approval and non-mutating: %#v", contract)
	}
	executorRequires, ok := contract["executor_requires"].([]string)
	if !ok || !containsString(executorRequires, "ready completion-plan evidence") || !containsString(executorRequires, "final verification or logscan evidence") || !containsString(executorRequires, "container runtime_evidence_checklist review before self-heal execution") {
		t.Fatalf("readiness guide contract should require completion and verification/logscan evidence: %#v", contract["executor_requires"])
	}
	evidenceContracts, ok := payload["evidence_contracts"].([]string)
	if !ok ||
		!containsString(evidenceContracts, "container logscan can return autoheal_handoff.runtime_evidence_checklist before self-heal planning") ||
		!containsString(evidenceContracts, "readiness summaries are checkpoints, not operator approval") {
		t.Fatalf("AI guide evidence contracts should mirror MCP surface contracts: %#v", payload["evidence_contracts"])
	}
	desiredState, ok := contract["desired_state"].(map[string]interface{})
	if !ok || desiredState["readiness_tool"] != "meshclaw_reconcile_readiness_summary" {
		t.Fatalf("desired-state readiness guide missing: %#v", contract["desired_state"])
	}
	desiredStops, _ := desiredState["stop_before"].([]string)
	if !containsString(desiredStops, "starting reconcile executor without ready completion-plan evidence") {
		t.Fatalf("desired-state readiness guide should stop before executor: %#v", desiredState)
	}
	container, ok := contract["container_self_heal"].(map[string]interface{})
	if !ok || container["readiness_tool"] != "meshclaw_autoheal_container_readiness_summary" {
		t.Fatalf("container readiness guide missing: %#v", contract["container_self_heal"])
	}
	containerStops, _ := container["stop_before"].([]string)
	if !containsString(containerStops, "starting container executor without ready completion-plan evidence") {
		t.Fatalf("container readiness guide should stop before executor: %#v", container)
	}
}

func TestMCPAutomationRulePlanIsPlanOnly(t *testing.T) {
	result, err := callMCPTool("meshclaw_automation_rule_plan", map[string]interface{}{
		"name":       "openwebui-container-repair",
		"trigger":    "container_unhealthy",
		"condition":  "unhealthy for 3 checks and logs contain timeout",
		"action":     "autoheal_plan -> approval_request -> container_apply_plan",
		"scope":      "g4/openwebui",
		"auto_apply": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("payload=%#v", result)
	}
	report, ok := payload["report"].(map[string]interface{})
	if !ok {
		t.Fatalf("report missing: %#v", payload)
	}
	if report["kind"] != "automation_rule_plan" || report["name"] != "openwebui-container-repair" || report["auto_apply"] != false {
		t.Fatalf("unexpected automation rule report: %#v", report)
	}
	contract, ok := report["rule_contract"].(map[string]interface{})
	if !ok || contract["writes_rule"] != false || contract["requires_review"] != true || contract["rollback_required"] != true {
		t.Fatalf("automation rule should be plan-only and review-gated: %#v", report["rule_contract"])
	}
	controls, ok := report["safety_controls"].([]string)
	if !ok || !containsString(controls, "default to auto_apply=false until the operator explicitly approves a narrow allow rule") {
		t.Fatalf("automation rule should default to no auto apply: %#v", report["safety_controls"])
	}
	evidenceRecord, ok := payload["evidence"].(evidence.Record)
	if !ok || evidenceRecord.Kind != "automation-rule-plan" {
		t.Fatalf("automation rule plan should store evidence: %#v", payload["evidence"])
	}
}

func TestMCPAutomationRuleCheckIsGateOnly(t *testing.T) {
	planResult, err := callMCPTool("meshclaw_automation_rule_plan", map[string]interface{}{
		"name":       "openwebui-container-repair",
		"trigger":    "container_unhealthy",
		"condition":  "unhealthy for 3 checks and logs contain timeout",
		"action":     "autoheal_plan -> approval_request -> container_apply_plan",
		"scope":      "g4/openwebui",
		"auto_apply": false,
	})
	if err != nil {
		t.Fatal(err)
	}
	planPayload, ok := planResult.(map[string]interface{})
	if !ok {
		t.Fatalf("plan payload=%#v", planResult)
	}
	planEvidence, ok := planPayload["evidence"].(evidence.Record)
	if !ok || planEvidence.Kind != "automation-rule-plan" || planEvidence.StoredAt == "" {
		t.Fatalf("automation rule plan should store evidence path: %#v", planPayload["evidence"])
	}

	result, err := callMCPTool("meshclaw_automation_rule_check", map[string]interface{}{
		"rule_plan_evidence_path": planEvidence.StoredAt,
		"approved_by":             "zeus",
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("payload=%#v", result)
	}
	report, ok := payload["report"].(map[string]interface{})
	if !ok {
		t.Fatalf("report missing: %#v", payload)
	}
	if report["kind"] != "automation_rule_check" || report["ready"] != true || report["writes_rule"] != false || report["executes_automation"] != false {
		t.Fatalf("automation rule check should be ready and gate-only: %#v", report)
	}
	if report["source_evidence_kind"] != "automation-rule-plan" || report["approved_by"] != "zeus" {
		t.Fatalf("automation rule check should load plan evidence and approval identity: %#v", report)
	}
	checkEvidence, ok := payload["evidence"].(evidence.Record)
	if !ok || checkEvidence.Kind != "automation-rule-check" {
		t.Fatalf("automation rule check should store evidence: %#v", payload["evidence"])
	}

	summaryResult, err := callMCPTool("meshclaw_automation_rule_readiness_summary", map[string]interface{}{
		"rule_check_evidence_path": checkEvidence.StoredAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	summaryPayload, ok := summaryResult.(map[string]interface{})
	if !ok {
		t.Fatalf("summary payload=%#v", summaryResult)
	}
	summary, ok := summaryPayload["readiness_summary"].(map[string]interface{})
	if !ok {
		t.Fatalf("readiness summary missing: %#v", summaryPayload)
	}
	if summary["kind"] != "automation_rule_readiness_summary" || summary["ready"] != true || summary["writes_rule"] != false || summary["executes_automation"] != false {
		t.Fatalf("automation rule readiness should be summary-only and ready: %#v", summary)
	}
	if summary["source_evidence_kind"] != "automation-rule-check" {
		t.Fatalf("automation rule readiness should load check evidence: %#v", summary)
	}
	summaryEvidence, ok := summaryPayload["evidence"].(evidence.Record)
	if !ok || summaryEvidence.Kind != "automation-rule-readiness-summary" {
		t.Fatalf("automation rule readiness should store evidence: %#v", summaryPayload["evidence"])
	}
	writerResult, err := callMCPTool("meshclaw_automation_rule_writer_plan", map[string]interface{}{
		"readiness_evidence_path": summaryEvidence.StoredAt,
		"rule_store":              "~/.meshclaw/rules",
	})
	if err != nil {
		t.Fatal(err)
	}
	writerPayload, ok := writerResult.(map[string]interface{})
	if !ok {
		t.Fatalf("writer payload=%#v", writerResult)
	}
	writerPlan, ok := writerPayload["writer_plan"].(map[string]interface{})
	if !ok {
		t.Fatalf("writer plan missing: %#v", writerPayload)
	}
	if writerPlan["kind"] != "automation_rule_writer_plan" || writerPlan["ready"] != true || writerPlan["writes_rule"] != false || writerPlan["executes_automation"] != false {
		t.Fatalf("automation rule writer plan should be ready and plan-only: %#v", writerPlan)
	}
	envelope, ok := writerPlan["rule_envelope_preview"].(map[string]interface{})
	if !ok || envelope["enabled"] != false || envelope["schema_version"] != "automation-rule/v1-draft" {
		t.Fatalf("writer plan should preview disabled draft envelope: %#v", writerPlan["rule_envelope_preview"])
	}
	writerEvidence, ok := writerPayload["evidence"].(evidence.Record)
	if !ok || writerEvidence.Kind != "automation-rule-writer-plan" {
		t.Fatalf("automation rule writer plan should store evidence: %#v", writerPayload["evidence"])
	}

	if _, err := callMCPTool("meshclaw_automation_rule_check", map[string]interface{}{
		"rule_plan_evidence_path": planEvidence.StoredAt,
		"approved_by":             "zeus",
		"apply":                   true,
	}); err == nil {
		t.Fatal("automation rule check should reject apply=true")
	}
	if _, err := callMCPTool("meshclaw_automation_rule_readiness_summary", map[string]interface{}{
		"rule_check_evidence_path": checkEvidence.StoredAt,
		"execute":                  true,
	}); err == nil {
		t.Fatal("automation rule readiness should reject execute=true")
	}
	if _, err := callMCPTool("meshclaw_automation_rule_writer_plan", map[string]interface{}{
		"readiness_evidence_path": summaryEvidence.StoredAt,
		"apply":                   true,
	}); err == nil {
		t.Fatal("automation rule writer plan should reject apply=true")
	}
}

func TestMCPSmokeTestPlanIsReadOnly(t *testing.T) {
	result, err := callMCPTool("meshclaw_mcp_smoke_test_plan", map[string]interface{}{
		"client": "claude",
		"scope":  "k8s-replacement",
		"tools":  "meshclaw_service_registry_plan,meshclaw_capacity_scale_plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("payload=%#v", result)
	}
	report, ok := payload["report"].(map[string]interface{})
	if !ok {
		t.Fatalf("report missing: %#v", payload)
	}
	if report["kind"] != "mcp_smoke_test_plan" || report["client"] != "claude" || report["scope"] != "k8s-replacement" {
		t.Fatalf("unexpected mcp smoke report: %#v", report)
	}
	checks, ok := report["checks"].([]map[string]interface{})
	if !ok || len(checks) != 5 {
		t.Fatalf("mcp smoke test should include surface, recommendation, and tool checks: %#v", report["checks"])
	}
	if checks[1]["tool"] != "meshclaw_mcp_profile_visibility_check" {
		t.Fatalf("mcp smoke test should include profile visibility before tool calls: %#v", checks)
	}
	args, ok := checks[1]["arguments"].(map[string]interface{})
	if !ok || args["profile"] != "claude-lite" || args["expected_tools"] != "meshclaw_service_registry_plan,meshclaw_capacity_scale_plan" {
		t.Fatalf("profile visibility check should receive smoke tools: %#v", checks[1]["arguments"])
	}
	passCriteria, ok := report["pass_criteria"].([]string)
	if !ok || !containsString(passCriteria, "no smoke step requests apply=true, execute=true, provider changes, file deletion, proxy edits, or live server mutation") {
		t.Fatalf("mcp smoke test should be read-only: %#v", report["pass_criteria"])
	}
	if !containsString(passCriteria, "meshclaw_mcp_profile_visibility_check reports no missing tools for the selected MCP profile") {
		t.Fatalf("mcp smoke test should require profile visibility: %#v", report["pass_criteria"])
	}
	if !containsString(passCriteria, "meshclaw_tool_recommend routes sample prompts to expected plan-only tools and exposes required_evidence plus stop_before boundaries") {
		t.Fatalf("mcp smoke test should require recommendation evidence boundaries: %#v", report["pass_criteria"])
	}
	evidenceRecord, ok := payload["evidence"].(evidence.Record)
	if !ok || evidenceRecord.Kind != "mcp-smoke-test-plan" {
		t.Fatalf("mcp smoke test plan should store evidence: %#v", payload["evidence"])
	}
}

func TestMCPProfileVisibilityCheckFindsClaudeLiteTools(t *testing.T) {
	expected := "meshclaw_mcp_profile_visibility_check,meshclaw_automation_rule_plan,meshclaw_automation_rule_check,meshclaw_automation_rule_readiness_summary,meshclaw_automation_rule_writer_plan"
	result, err := callMCPTool("meshclaw_mcp_profile_visibility_check", map[string]interface{}{
		"profile":        "claude-lite",
		"expected_tools": expected,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("payload=%#v", result)
	}
	report, ok := payload["report"].(map[string]interface{})
	if !ok {
		t.Fatalf("report missing: %#v", payload)
	}
	if report["kind"] != "mcp_profile_visibility_check" || report["profile"] != "claude-lite" || report["ready"] != true {
		t.Fatalf("unexpected profile visibility report: %#v", report)
	}
	missing, ok := report["missing_tools"].([]string)
	if !ok || len(missing) != 0 {
		t.Fatalf("expected all tools visible, missing=%#v", report["missing_tools"])
	}
	if report["writes_profile"] != false || report["restarts_client"] != false || report["runs_tools"] != false {
		t.Fatalf("profile visibility check should be read-only: %#v", report)
	}
	evidenceRecord, ok := payload["evidence"].(evidence.Record)
	if !ok || evidenceRecord.Kind != "mcp-profile-visibility-check" {
		t.Fatalf("profile visibility check should store evidence: %#v", payload["evidence"])
	}

	if _, err := callMCPTool("meshclaw_mcp_profile_visibility_check", map[string]interface{}{
		"profile":        "claude-lite",
		"expected_tools": expected,
		"execute":        true,
	}); err == nil {
		t.Fatal("profile visibility check should reject execute=true")
	}
}

func TestMCPProfileVisibilityCheckHandlesFullProfileAndAliases(t *testing.T) {
	fullResult, err := callMCPTool("meshclaw_mcp_profile_visibility_check", map[string]interface{}{
		"profile":        "all",
		"expected_tools": "meshclaw_storage_guardrail_plan,meshclaw_ops_integration_plan,meshclaw_mcp_profile_visibility_check,meshclaw_autoheal_container_executor_verify",
	})
	if err != nil {
		t.Fatal(err)
	}
	fullPayload, ok := fullResult.(map[string]interface{})
	if !ok {
		t.Fatalf("full payload=%#v", fullResult)
	}
	fullReport, ok := fullPayload["report"].(map[string]interface{})
	if !ok {
		t.Fatalf("full report missing: %#v", fullPayload)
	}
	if fullReport["profile"] != "all" || fullReport["ready"] != true {
		t.Fatalf("full profile should expose full MCP tools: %#v", fullReport)
	}
	if missing, ok := fullReport["missing_tools"].([]string); !ok || len(missing) != 0 {
		t.Fatalf("full profile should not miss expected tools: %#v", fullReport["missing_tools"])
	}

	aliasResult, err := callMCPTool("meshclaw_mcp_profile_visibility_check", map[string]interface{}{
		"profile":        "claude",
		"expected_tools": "meshclaw_mcp_profile_visibility_check",
	})
	if err != nil {
		t.Fatal(err)
	}
	aliasPayload, ok := aliasResult.(map[string]interface{})
	if !ok {
		t.Fatalf("alias payload=%#v", aliasResult)
	}
	aliasReport, ok := aliasPayload["report"].(map[string]interface{})
	if !ok {
		t.Fatalf("alias report missing: %#v", aliasPayload)
	}
	if aliasReport["profile"] != "claude-lite" || aliasReport["ready"] != true {
		t.Fatalf("claude alias should normalize to claude-lite: %#v", aliasReport)
	}
}

func TestMCPProfileVisibilityCheckFindsClaudeLiteReconcileTools(t *testing.T) {
	result, err := callMCPTool("meshclaw_mcp_profile_visibility_check", map[string]interface{}{
		"profile":        "claude-lite",
		"expected_tools": strings.Join(desiredStateReconcilePlanningToolNames(), ","),
	})
	if err != nil {
		t.Fatalf("callMCPTool error = %v", err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	report, ok := payload["report"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing report: %#v", payload)
	}
	if report["profile"] != "claude-lite" || report["ready"] != true {
		t.Fatalf("claude-lite reconcile tools should be visible: %#v", report)
	}
	if missing, _ := report["missing_tools"].([]string); len(missing) != 0 {
		t.Fatalf("claude-lite should not miss reconcile tools: %#v", missing)
	}
}

func TestMCPRolloutPlanUsesFullProfileForContainerExecutorVerify(t *testing.T) {
	result, err := callMCPTool("meshclaw_mcp_rollout_plan", map[string]interface{}{
		"client":         "claude",
		"branch":         "codex/container-executor-verify-recommendation",
		"expected_tools": "meshclaw_autoheal_container_executor_gate,meshclaw_autoheal_container_executor,meshclaw_autoheal_container_executor_verify",
	})
	if err != nil {
		t.Fatalf("callMCPTool error = %v", err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	report, ok := payload["report"].(map[string]interface{})
	if !ok {
		t.Fatalf("report missing: %#v", payload)
	}
	if report["visibility_profile"] != "all" {
		t.Fatalf("rollout should select full profile for executor_verify: %#v", report["visibility_profile"])
	}
	visibility, ok := report["profile_visibility"].(map[string]interface{})
	if !ok {
		t.Fatalf("rollout should include profile visibility: %#v", report["profile_visibility"])
	}
	if visibility["profile"] != "all" || visibility["ready"] != true {
		t.Fatalf("full profile visibility should be ready for executor_verify: %#v", visibility)
	}
	if missing, _ := visibility["missing_tools"].([]string); len(missing) != 0 {
		t.Fatalf("full profile should expose executor verify tools: %#v", missing)
	}
	steps, ok := report["verification_steps"].([]map[string]interface{})
	if !ok || len(steps) < 3 {
		t.Fatalf("rollout should include verification steps: %#v", report["verification_steps"])
	}
	args, ok := steps[2]["arguments"].(map[string]interface{})
	if !ok || args["profile"] != "all" {
		t.Fatalf("profile visibility step should use full profile for executor_verify: %#v", steps[2])
	}
	if args["expected_tools"] != "meshclaw_autoheal_container_executor_gate,meshclaw_autoheal_container_executor,meshclaw_autoheal_container_executor_verify" {
		t.Fatalf("profile visibility step should preserve expected tools: %#v", args)
	}
	localSteps, ok := report["local_verification_steps"].([]map[string]interface{})
	if !ok || len(localSteps) < 4 {
		t.Fatalf("rollout should include local verification steps: %#v", report["local_verification_steps"])
	}
	localArgs, ok := localSteps[3]["arguments"].(map[string]interface{})
	if !ok || localArgs["profile"] != "all" {
		t.Fatalf("local verification step should use full profile for executor_verify: %#v", localSteps[3])
	}
}

func TestMCPSmokeTestPlanUsesFullProfileForContainerExecutorVerify(t *testing.T) {
	result, err := callMCPTool("meshclaw_mcp_smoke_test_plan", map[string]interface{}{
		"client": "claude",
		"tools":  "meshclaw_autoheal_container_executor_verify",
	})
	if err != nil {
		t.Fatalf("callMCPTool error = %v", err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	report, ok := payload["report"].(map[string]interface{})
	if !ok {
		t.Fatalf("report missing: %#v", payload)
	}
	if report["visibility_profile"] != "all" {
		t.Fatalf("smoke should select full profile for executor_verify: %#v", report["visibility_profile"])
	}
	checks, ok := report["checks"].([]map[string]interface{})
	if !ok || len(checks) < 2 {
		t.Fatalf("smoke should include checks: %#v", report["checks"])
	}
	args, ok := checks[1]["arguments"].(map[string]interface{})
	if !ok || args["profile"] != "all" || args["expected_tools"] != "meshclaw_autoheal_container_executor_verify" {
		t.Fatalf("smoke profile visibility should use full profile for executor_verify: %#v", checks[1])
	}
	visibility, ok := report["profile_visibility"].(map[string]interface{})
	if !ok || visibility["profile"] != "all" || visibility["ready"] != true {
		t.Fatalf("smoke full profile visibility should be ready: %#v", report["profile_visibility"])
	}
}

func TestMCPProfileVisibilityCheckFindsClaudeLiteContainerSelfHealTools(t *testing.T) {
	expectedTools := append([]string{"meshclaw_autoheal_plan", "meshclaw_analyze_logs"}, containerSelfHealPlanningToolNames()...)
	expectedTools = append(expectedTools, "meshclaw_autoheal_container_executor_gate", "meshclaw_autoheal_container_executor")
	result, err := callMCPTool("meshclaw_mcp_profile_visibility_check", map[string]interface{}{
		"profile":        "claude-lite",
		"expected_tools": strings.Join(expectedTools, ","),
	})
	if err != nil {
		t.Fatalf("callMCPTool error = %v", err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	report, ok := payload["report"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing report: %#v", payload)
	}
	if report["profile"] != "claude-lite" || report["ready"] != true {
		t.Fatalf("claude-lite container self-heal tools should be visible: %#v", report)
	}
	if missing, _ := report["missing_tools"].([]string); len(missing) != 0 {
		t.Fatalf("claude-lite should not miss container self-heal tools: %#v", missing)
	}
}

func TestMCPSmokeTestPlanDefaultsIncludeK8sReplacementChain(t *testing.T) {
	result, err := callMCPTool("meshclaw_mcp_smoke_test_plan", map[string]interface{}{
		"client": "claude",
		"scope":  "k8s-replacement",
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("payload=%#v", result)
	}
	report, ok := payload["report"].(map[string]interface{})
	if !ok {
		t.Fatalf("report missing: %#v", payload)
	}
	tools, ok := report["tools"].([]string)
	if !ok {
		t.Fatalf("tools missing: %#v", report["tools"])
	}
	for _, want := range []string{
		"meshclaw_service_registry_plan",
		"meshclaw_capacity_scale_plan",
		"meshclaw_storage_guardrail_plan",
		"meshclaw_ops_integration_plan",
		"meshclaw_reconcile_validate_desired",
		"meshclaw_reconcile_plan",
		"meshclaw_reconcile_approval_request",
		"meshclaw_reconcile_apply_gate",
		"meshclaw_reconcile_apply_plan",
		"meshclaw_reconcile_execution_preview",
		"meshclaw_reconcile_readiness_summary",
		"meshclaw_autoheal_plan",
		"meshclaw_autoheal_container_apply_plan",
		"meshclaw_autoheal_container_readiness_summary",
		"meshclaw_analyze_logs",
		"meshclaw_mcp_rollout_plan",
	} {
		if !containsString(tools, want) {
			t.Fatalf("default smoke tools should include %s: %#v", want, tools)
		}
	}
	checks, ok := report["checks"].([]map[string]interface{})
	if !ok || len(checks) != len(tools)+3 {
		t.Fatalf("mcp smoke defaults should include surface, visibility, recommendation, and tool checks: %#v", report["checks"])
	}
	recommendationExpected, ok := checks[2]["expected"].(string)
	if !ok || !strings.Contains(recommendationExpected, "required_evidence") || !strings.Contains(recommendationExpected, "stop_before") {
		t.Fatalf("recommendation smoke check should require evidence and stop-before fields: %#v", checks[2])
	}
	args, ok := checks[1]["arguments"].(map[string]interface{})
	if !ok || args["profile"] != "claude-lite" {
		t.Fatalf("profile visibility check should target claude-lite: %#v", checks[1]["arguments"])
	}
	expectedTools, ok := args["expected_tools"].(string)
	if !ok {
		t.Fatalf("profile visibility expected_tools should be a string: %#v", args["expected_tools"])
	}
	for _, want := range []string{"meshclaw_reconcile_validate_desired", "meshclaw_reconcile_plan", "meshclaw_reconcile_approval_request", "meshclaw_reconcile_apply_gate", "meshclaw_reconcile_apply_plan", "meshclaw_reconcile_execution_preview", "meshclaw_reconcile_readiness_summary", "meshclaw_autoheal_container_apply_plan", "meshclaw_autoheal_container_readiness_summary", "meshclaw_autoheal_container_executor_gate", "meshclaw_autoheal_container_executor", "meshclaw_analyze_logs"} {
		if !strings.Contains(expectedTools, want) {
			t.Fatalf("profile visibility expected_tools should include %s: %q", want, expectedTools)
		}
	}
	visibility, ok := report["profile_visibility"].(map[string]interface{})
	if !ok {
		t.Fatalf("default smoke report should include profile visibility preview: %#v", report["profile_visibility"])
	}
	if visibility["profile"] != "claude-lite" || visibility["ready"] != true {
		t.Fatalf("default smoke visibility should be ready for claude-lite: %#v", visibility)
	}
	if missing, ok := visibility["missing_tools"].([]string); !ok || len(missing) != 0 {
		t.Fatalf("default smoke visibility should have zero missing tools: %#v", visibility["missing_tools"])
	}
	checkByTool := map[string]map[string]interface{}{}
	for _, check := range checks {
		if tool, _ := check["tool"].(string); tool != "" {
			checkByTool[tool] = check
		}
	}
	for _, tool := range tools {
		check := checkByTool[tool]
		if check == nil {
			t.Fatalf("missing smoke check for tool %s: %#v", tool, checks)
		}
		args, ok := check["arguments"].(map[string]interface{})
		if !ok {
			t.Fatalf("smoke check should include safe arguments for %s: %#v", tool, check)
		}
		if _, ok := args["apply"]; ok {
			t.Fatalf("smoke arguments must not include apply for %s: %#v", tool, args)
		}
		if _, ok := args["execute"]; ok {
			t.Fatalf("smoke arguments must not include execute for %s: %#v", tool, args)
		}
	}
	if fixture, ok := checkByTool["meshclaw_autoheal_container_executor_gate"]["fixture_required"].(bool); !ok || !fixture {
		t.Fatalf("container executor gate smoke check should require readiness-summary evidence fixture: %#v", checkByTool["meshclaw_autoheal_container_executor_gate"])
	}
	if fixture, ok := checkByTool["meshclaw_autoheal_container_executor"]["fixture_required"].(bool); !ok || !fixture {
		t.Fatalf("container executor smoke check should require executor-gate evidence fixture: %#v", checkByTool["meshclaw_autoheal_container_executor"])
	}
	if args, ok := checkByTool["meshclaw_autoheal_container_executor_gate"]["arguments"].(map[string]interface{}); !ok || args["container_readiness_summary_evidence_path"] != "<test-fixture-container-readiness-summary-evidence.json>" || args["dry_run"] != true {
		t.Fatalf("container executor gate smoke args should stay dry-run fixture-only: %#v", checkByTool["meshclaw_autoheal_container_executor_gate"])
	}
	if args, ok := checkByTool["meshclaw_autoheal_container_executor"]["arguments"].(map[string]interface{}); !ok || args["container_executor_gate_evidence_path"] != "<test-fixture-container-executor-gate-evidence.json>" || args["dry_run"] != true {
		t.Fatalf("container executor smoke args should stay dry-run fixture-only: %#v", checkByTool["meshclaw_autoheal_container_executor"])
	}
	if args, ok := checkByTool["meshclaw_service_registry_plan"]["arguments"].(map[string]interface{}); !ok || args["service"] != "openwebui" || args["scope"] != "all" {
		t.Fatalf("service registry smoke arguments should be concrete and read-only: %#v", checkByTool["meshclaw_service_registry_plan"])
	}
	if args, ok := checkByTool["meshclaw_capacity_scale_plan"]["arguments"].(map[string]interface{}); !ok || args["workload"] != "vllm" || args["budget_usd"] != 0 {
		t.Fatalf("capacity smoke arguments should avoid spend: %#v", checkByTool["meshclaw_capacity_scale_plan"])
	}
	if args, ok := checkByTool["meshclaw_storage_guardrail_plan"]["arguments"].(map[string]interface{}); !ok || args["backup"] != true {
		t.Fatalf("storage smoke arguments should require backup guardrail: %#v", checkByTool["meshclaw_storage_guardrail_plan"])
	}
	if args, ok := checkByTool["meshclaw_ops_integration_plan"]["arguments"].(map[string]interface{}); !ok || args["readonly"] != true {
		t.Fatalf("ops integration smoke arguments should stay read-only: %#v", checkByTool["meshclaw_ops_integration_plan"])
	}
	if fixture, ok := checkByTool["meshclaw_reconcile_validate_desired"]["fixture_required"].(bool); !ok || !fixture {
		t.Fatalf("desired validation smoke check should require a desired-state fixture: %#v", checkByTool["meshclaw_reconcile_validate_desired"])
	}
	if fixture, ok := checkByTool["meshclaw_reconcile_plan"]["fixture_required"].(bool); !ok || !fixture {
		t.Fatalf("reconcile smoke check should require a desired-state fixture: %#v", checkByTool["meshclaw_reconcile_plan"])
	}
	desiredValidationExpected, _ := checkByTool["meshclaw_reconcile_validate_desired"]["expected"].(string)
	if !strings.Contains(desiredValidationExpected, "desired_validation_contract.validation_only=true") || !strings.Contains(desiredValidationExpected, "desired_validation_contract.yaml_keys_grant_approval=false") {
		t.Fatalf("desired validation smoke expected output should require validation-only contract: %#v", checkByTool["meshclaw_reconcile_validate_desired"])
	}
	reconcilePlanExpected, _ := checkByTool["meshclaw_reconcile_plan"]["expected"].(string)
	if !strings.Contains(reconcilePlanExpected, "reconcile_plan_contract.dry_run_only=true") || !strings.Contains(reconcilePlanExpected, "reconcile_plan_contract.apply_allowed=false") {
		t.Fatalf("reconcile plan smoke expected output should require dry-run contract: %#v", checkByTool["meshclaw_reconcile_plan"])
	}
	approvalRequestExpected, _ := checkByTool["meshclaw_reconcile_approval_request"]["expected"].(string)
	if !strings.Contains(approvalRequestExpected, "approval_request_contract.request_only=true") || !strings.Contains(approvalRequestExpected, "approval_request_contract.operator_approval_recorded=false") {
		t.Fatalf("approval-request smoke expected output should require request-only contract: %#v", checkByTool["meshclaw_reconcile_approval_request"])
	}
	applyGateExpected, _ := checkByTool["meshclaw_reconcile_apply_gate"]["expected"].(string)
	if !strings.Contains(applyGateExpected, "apply_gate_contract.gate_only=true") || !strings.Contains(applyGateExpected, "apply_gate_contract.apply_allowed=false") {
		t.Fatalf("apply-gate smoke expected output should require gate-only contract: %#v", checkByTool["meshclaw_reconcile_apply_gate"])
	}
	applyPlanExpected, _ := checkByTool["meshclaw_reconcile_apply_plan"]["expected"].(string)
	if !strings.Contains(applyPlanExpected, "apply_plan_contract.plan_only=true") || !strings.Contains(applyPlanExpected, "apply_plan_contract.requires_execution_preview_before_executor=true") {
		t.Fatalf("apply-plan smoke expected output should require preview-before-executor contract: %#v", checkByTool["meshclaw_reconcile_apply_plan"])
	}
	executionPreviewExpected, _ := checkByTool["meshclaw_reconcile_execution_preview"]["expected"].(string)
	if !strings.Contains(executionPreviewExpected, "execution_preview_contract.preview_only=true") || !strings.Contains(executionPreviewExpected, "execution_preview_contract.commands_are_inert_templates=true") {
		t.Fatalf("execution-preview smoke expected output should require inert-template contract: %#v", checkByTool["meshclaw_reconcile_execution_preview"])
	}
	if fixture, ok := checkByTool["meshclaw_reconcile_readiness_summary"]["fixture_required"].(bool); !ok || !fixture {
		t.Fatalf("reconcile readiness smoke check should require completion-plan evidence fixture: %#v", checkByTool["meshclaw_reconcile_readiness_summary"])
	}
	reconcileReadinessExpected, _ := checkByTool["meshclaw_reconcile_readiness_summary"]["expected"].(string)
	if !strings.Contains(reconcileReadinessExpected, "readiness_contract.apply_allowed=false") || !strings.Contains(reconcileReadinessExpected, "readiness_contract.grants_future_approval=false") {
		t.Fatalf("reconcile readiness smoke expected output should require non-approval readiness contract: %#v", checkByTool["meshclaw_reconcile_readiness_summary"])
	}
	if fixture, ok := checkByTool["meshclaw_autoheal_container_apply_plan"]["fixture_required"].(bool); !ok || !fixture {
		t.Fatalf("container apply-plan smoke check should require autoheal-plan evidence fixture: %#v", checkByTool["meshclaw_autoheal_container_apply_plan"])
	}
	if fixture, ok := checkByTool["meshclaw_autoheal_container_readiness_summary"]["fixture_required"].(bool); !ok || !fixture {
		t.Fatalf("container readiness smoke check should require completion-plan evidence fixture: %#v", checkByTool["meshclaw_autoheal_container_readiness_summary"])
	}
	containerReadinessExpected, _ := checkByTool["meshclaw_autoheal_container_readiness_summary"]["expected"].(string)
	if !strings.Contains(containerReadinessExpected, "container_readiness_summary_contract.summary_only=true") || !strings.Contains(containerReadinessExpected, "container_readiness_summary_contract.requires_approval_gated_executor=true") {
		t.Fatalf("container readiness smoke expected output should require container readiness summary contract: %#v", checkByTool["meshclaw_autoheal_container_readiness_summary"])
	}
	if fixture, ok := checkByTool["meshclaw_analyze_logs"]["fixture_required"].(bool); !ok || !fixture {
		t.Fatalf("logscan smoke check should require redacted log evidence fixture: %#v", checkByTool["meshclaw_analyze_logs"])
	}
	logscanPrompt, _ := checkByTool["meshclaw_analyze_logs"]["prompt"].(string)
	if !strings.Contains(logscanPrompt, "systemd runtime") || !strings.Contains(logscanPrompt, "unit identity") {
		t.Fatalf("logscan smoke prompt should exercise systemd unit identity gate: %#v", checkByTool["meshclaw_analyze_logs"])
	}
	if !strings.Contains(logscanPrompt, "source=container:openwebui") {
		t.Fatalf("logscan smoke prompt should exercise focused container log source: %#v", checkByTool["meshclaw_analyze_logs"])
	}
	logscanExpected, _ := checkByTool["meshclaw_analyze_logs"]["expected"].(string)
	if !strings.Contains(logscanExpected, "autoheal_handoff.runtime_evidence_checklist") {
		t.Fatalf("logscan smoke expected output should require runtime checklist: %#v", checkByTool["meshclaw_analyze_logs"])
	}
	if !strings.Contains(logscanExpected, "handoff_contract.apply_allowed=false") {
		t.Fatalf("logscan smoke expected output should require non-apply handoff contract: %#v", checkByTool["meshclaw_analyze_logs"])
	}
	for _, want := range []string{"handoff_contract.direct_restart_allowed=false", "handoff_contract.requires_focused_runtime_evidence=true", "handoff_contract.runtime_evidence_checklist_count", "handoff_contract.next_required_tool"} {
		if !strings.Contains(logscanExpected, want) {
			t.Fatalf("logscan smoke expected output should require structured handoff gate %q: %#v", want, checkByTool["meshclaw_analyze_logs"])
		}
	}
	fixtures, ok := report["fixture_manifest"].([]map[string]interface{})
	if !ok || len(fixtures) != 12 {
		t.Fatalf("default smoke plan should list required fixtures: %#v", report["fixture_manifest"])
	}
	fixtureByTool := map[string]map[string]interface{}{}
	for _, fixture := range fixtures {
		tool, _ := fixture["tool"].(string)
		fixtureByTool[tool] = fixture
		if fixture["mutates_state"] != false {
			t.Fatalf("smoke fixtures must be non-mutating: %#v", fixture)
		}
	}
	if fixtureByTool["meshclaw_reconcile_validate_desired"]["placeholder"] != "<test-fixture-desired-state.yaml>" {
		t.Fatalf("desired validation fixture should name desired-state placeholder: %#v", fixtureByTool["meshclaw_reconcile_validate_desired"])
	}
	if fixtureByTool["meshclaw_reconcile_plan"]["placeholder"] != "<test-fixture-desired-state.yaml>" {
		t.Fatalf("reconcile fixture should name desired-state placeholder: %#v", fixtureByTool["meshclaw_reconcile_plan"])
	}
	if fixtureByTool["meshclaw_reconcile_approval_request"]["placeholder"] != "<test-fixture-desired-state.yaml>" {
		t.Fatalf("approval-request fixture should name desired-state placeholder: %#v", fixtureByTool["meshclaw_reconcile_approval_request"])
	}
	if fixtureByTool["meshclaw_reconcile_apply_gate"]["placeholder"] != "<test-fixture-reconcile-approval-request-evidence.json>" {
		t.Fatalf("apply-gate fixture should name approval-request evidence placeholder: %#v", fixtureByTool["meshclaw_reconcile_apply_gate"])
	}
	if fixtureByTool["meshclaw_reconcile_apply_plan"]["placeholder"] != "<test-fixture-reconcile-apply-gate-evidence.json>" {
		t.Fatalf("apply-plan fixture should name apply-gate evidence placeholder: %#v", fixtureByTool["meshclaw_reconcile_apply_plan"])
	}
	if fixtureByTool["meshclaw_reconcile_execution_preview"]["placeholder"] != "<test-fixture-reconcile-apply-plan-evidence.json>" {
		t.Fatalf("execution-preview fixture should name apply-plan evidence placeholder: %#v", fixtureByTool["meshclaw_reconcile_execution_preview"])
	}
	if fixtureByTool["meshclaw_reconcile_readiness_summary"]["placeholder"] != "<test-fixture-reconcile-completion-plan-evidence.json>" {
		t.Fatalf("reconcile readiness fixture should name completion-plan evidence placeholder: %#v", fixtureByTool["meshclaw_reconcile_readiness_summary"])
	}
	reconcileReadinessMinimumContent, ok := fixtureByTool["meshclaw_reconcile_readiness_summary"]["minimum_content"].([]string)
	if !ok || !containsString(reconcileReadinessMinimumContent, "stop_before gates") {
		t.Fatalf("reconcile readiness fixture should require stop-before gates: %#v", fixtureByTool["meshclaw_reconcile_readiness_summary"])
	}
	if fixtureByTool["meshclaw_autoheal_container_apply_plan"]["placeholder"] != "<test-fixture-autoheal-plan-evidence.json>" {
		t.Fatalf("container apply fixture should name autoheal evidence placeholder: %#v", fixtureByTool["meshclaw_autoheal_container_apply_plan"])
	}
	containerApplyMinimumContent, ok := fixtureByTool["meshclaw_autoheal_container_apply_plan"]["minimum_content"].([]string)
	if !ok || !containsString(containerApplyMinimumContent, "runtime_evidence_required") {
		t.Fatalf("container apply fixture should require runtime evidence contract content: %#v", fixtureByTool["meshclaw_autoheal_container_apply_plan"])
	}
	if !containsString(containerApplyMinimumContent, "container_apply_plan_contract.runtime_evidence_required_count") {
		t.Fatalf("container apply fixture should require runtime evidence count contract content: %#v", fixtureByTool["meshclaw_autoheal_container_apply_plan"])
	}
	if !containsString(containerApplyMinimumContent, "analyze_logs handoff_contract.apply_allowed=false when sourced from logscan") {
		t.Fatalf("container apply fixture should require non-apply logscan handoff content: %#v", fixtureByTool["meshclaw_autoheal_container_apply_plan"])
	}
	if fixtureByTool["meshclaw_autoheal_container_readiness_summary"]["placeholder"] != "<test-fixture-container-completion-plan-evidence.json>" {
		t.Fatalf("container readiness fixture should name completion-plan evidence placeholder: %#v", fixtureByTool["meshclaw_autoheal_container_readiness_summary"])
	}
	containerReadinessMinimumContent, ok := fixtureByTool["meshclaw_autoheal_container_readiness_summary"]["minimum_content"].([]string)
	if !ok || !containsString(containerReadinessMinimumContent, "stop_before gates") || !containsString(containerReadinessMinimumContent, "runtime_evidence_gate") || !containsString(containerReadinessMinimumContent, "runtime_evidence_findings") {
		t.Fatalf("container readiness fixture should require runtime evidence and stop-before gates: %#v", fixtureByTool["meshclaw_autoheal_container_readiness_summary"])
	}
	if fixtureByTool["meshclaw_autoheal_container_executor_gate"]["placeholder"] != "<test-fixture-container-readiness-summary-evidence.json>" {
		t.Fatalf("container executor gate fixture should name readiness-summary evidence placeholder: %#v", fixtureByTool["meshclaw_autoheal_container_executor_gate"])
	}
	containerExecutorGateMinimumContent, ok := fixtureByTool["meshclaw_autoheal_container_executor_gate"]["minimum_content"].([]string)
	if !ok || !containsString(containerExecutorGateMinimumContent, "container_executor_gate_contract.gate_only=true") || !containsString(containerExecutorGateMinimumContent, "container_executor_gate_contract.executor_allowed=false") {
		t.Fatalf("container executor gate fixture should require admission-only gate content: %#v", fixtureByTool["meshclaw_autoheal_container_executor_gate"])
	}
	if fixtureByTool["meshclaw_autoheal_container_executor"]["placeholder"] != "<test-fixture-container-executor-gate-evidence.json>" {
		t.Fatalf("container executor fixture should name executor-gate evidence placeholder: %#v", fixtureByTool["meshclaw_autoheal_container_executor"])
	}
	if fixtureByTool["meshclaw_analyze_logs"]["placeholder"] != "<test-fixture-container-logscan-evidence.txt>" {
		t.Fatalf("logscan fixture should name container log evidence placeholder: %#v", fixtureByTool["meshclaw_analyze_logs"])
	}
	minimumContent, ok := fixtureByTool["meshclaw_analyze_logs"]["minimum_content"].([]string)
	if !ok || !containsString(minimumContent, "autoheal_handoff recommended_tools") || !containsString(minimumContent, "autoheal_handoff confidence") || !containsString(minimumContent, "stop_before direct apply") || !containsString(minimumContent, "must_not direct restart/recreate") {
		t.Fatalf("logscan fixture should require handoff contract content: %#v", fixtureByTool["meshclaw_analyze_logs"])
	}
	for _, want := range []string{"systemd Exec format error sample", "WorkingDirectory missing sample", "DNS resolver failure sample", "log_findings unit_candidates field", "meshclaw_service_check arguments hint", "meshclaw_analyze_logs arguments hint", "exec_format_error pattern", "working_directory_missing pattern", "dns_resolver_failure pattern", "autoheal_handoff runtime evidence requirements", "autoheal_handoff runtime_evidence_checklist", "docker inspect status/health checklist", "autoheal_handoff unit identity evidence", "stop_before service restart before unit identity"} {
		if !containsString(minimumContent, want) {
			t.Fatalf("logscan fixture should require runtime-pattern content %q: %#v", want, fixtureByTool["meshclaw_analyze_logs"])
		}
	}
	validationMinimumContent, ok := fixtureByTool["meshclaw_reconcile_validate_desired"]["minimum_content"].([]string)
	if !ok || !containsString(validationMinimumContent, "validation_handoff") || !containsString(validationMinimumContent, "no apply/execute") {
		t.Fatalf("desired validation fixture should require handoff and non-apply contract content: %#v", fixtureByTool["meshclaw_reconcile_validate_desired"])
	}
	for _, want := range []string{"ignored apply/execute key sample", "ignored_apply_keys count", "validation_handoff stop_before ignored YAML apply/execute keys"} {
		if !containsString(validationMinimumContent, want) {
			t.Fatalf("desired validation fixture should require ignored apply-key content %q: %#v", want, fixtureByTool["meshclaw_reconcile_validate_desired"])
		}
	}
	runStrategy, ok := report["run_strategy"].(map[string]interface{})
	if !ok {
		t.Fatalf("default smoke plan should include run strategy: %#v", report["run_strategy"])
	}
	if runStrategy["mode"] != "read-only-plan" || runStrategy["stop_before_mutation"] != true || runStrategy["requires_client_reload"] != false {
		t.Fatalf("run strategy should stay read-only and avoid client reload: %#v", runStrategy)
	}
	readyNow, ok := runStrategy["ready_now"].([]string)
	if !ok || containsString(readyNow, "meshclaw_analyze_logs") || containsString(readyNow, "meshclaw_reconcile_validate_desired") || containsString(readyNow, "meshclaw_reconcile_plan") {
		t.Fatalf("run strategy ready_now should contain no-fixture checks only: %#v", runStrategy["ready_now"])
	}
	requiresFixture, ok := runStrategy["requires_fixture"].([]string)
	if !ok || !containsString(requiresFixture, "meshclaw_reconcile_validate_desired") || !containsString(requiresFixture, "meshclaw_reconcile_plan") || !containsString(requiresFixture, "meshclaw_autoheal_container_apply_plan") || !containsString(requiresFixture, "meshclaw_analyze_logs") {
		t.Fatalf("run strategy should separate fixture-required checks: %#v", runStrategy["requires_fixture"])
	}
	captureOrder, ok := runStrategy["evidence_capture_order"].([]map[string]interface{})
	if !ok || len(captureOrder) < 5 {
		t.Fatalf("run strategy should include evidence capture order: %#v", runStrategy["evidence_capture_order"])
	}
	if captureOrder[0]["phase"] != "surface" || captureOrder[1]["phase"] != "profile_visibility" || captureOrder[2]["phase"] != "recommendation" {
		t.Fatalf("evidence capture order should start with surface/profile/recommendation: %#v", captureOrder)
	}
	if captureOrder[len(captureOrder)-1]["phase"] != "fixture_required" || captureOrder[len(captureOrder)-1]["mutates_state"] != false {
		t.Fatalf("fixture-required capture should remain non-mutating and last: %#v", captureOrder)
	}
	summaryCounts, ok := report["summary_counts"].(map[string]interface{})
	if !ok {
		t.Fatalf("default smoke plan should include summary counts: %#v", report["summary_counts"])
	}
	if summaryCounts["tools"] != len(tools) || summaryCounts["checks"] != len(tools)+3 {
		t.Fatalf("summary counts should match tools and checks: %#v", summaryCounts)
	}
	if summaryCounts["ready_now"] != 6 || summaryCounts["fixture_required"] != 12 || summaryCounts["mutating_checks"] != 0 {
		t.Fatalf("summary counts should separate ready/fixture/non-mutating checks: %#v", summaryCounts)
	}
	if summaryCounts["requires_operator_apply"] != false {
		t.Fatalf("smoke summary should not require operator apply: %#v", summaryCounts)
	}
	approvalBoundary, ok := report["approval_boundary"].(map[string]interface{})
	if !ok {
		t.Fatalf("default smoke plan should include approval boundary: %#v", report["approval_boundary"])
	}
	if approvalBoundary["decision"] != "stop_before_apply" || approvalBoundary["mutates_live_servers"] != false || approvalBoundary["restarts_mcp_client"] != false || approvalBoundary["grants_future_approval"] != false {
		t.Fatalf("approval boundary should stop before apply and avoid client/server mutation: %#v", approvalBoundary)
	}
	denies, ok := approvalBoundary["smoke_denies"].([]string)
	if !ok || !containsString(denies, "apply=true") || !containsString(denies, "execute=true") || !containsString(denies, "client restart") || !containsString(denies, "live server deployment") {
		t.Fatalf("approval boundary should deny mutating smoke actions: %#v", approvalBoundary["smoke_denies"])
	}
	readiness, ok := report["readiness"].(map[string]interface{})
	if !ok {
		t.Fatalf("default smoke plan should include readiness summary: %#v", report["readiness"])
	}
	if readiness["status"] != "ready_now_partial" || readiness["can_run_without_fixtures"] != true || readiness["can_complete_all_checks"] != false || readiness["mutates_state"] != false {
		t.Fatalf("readiness should allow partial read-only smoke before fixtures: %#v", readiness)
	}
	if readiness["ready_now_count"] != 6 || readiness["fixture_required_count"] != 12 || readiness["operator_action_required"] != true {
		t.Fatalf("readiness counts should match default smoke plan: %#v", readiness)
	}
	blockers, ok := readiness["blocking_fixture_tools"].([]string)
	if !ok || !containsString(blockers, "meshclaw_reconcile_validate_desired") || !containsString(blockers, "meshclaw_reconcile_plan") || !containsString(blockers, "meshclaw_reconcile_approval_request") || !containsString(blockers, "meshclaw_reconcile_apply_gate") || !containsString(blockers, "meshclaw_reconcile_apply_plan") || !containsString(blockers, "meshclaw_reconcile_execution_preview") || !containsString(blockers, "meshclaw_reconcile_readiness_summary") || !containsString(blockers, "meshclaw_autoheal_container_apply_plan") || !containsString(blockers, "meshclaw_autoheal_container_readiness_summary") || !containsString(blockers, "meshclaw_analyze_logs") {
		t.Fatalf("readiness should list fixture blockers: %#v", readiness["blocking_fixture_tools"])
	}
}

func TestMCPSmokeTestPlanReconcileContractChain(t *testing.T) {
	tools := []string{
		"meshclaw_reconcile_apply_plan",
		"meshclaw_reconcile_execution_preview",
		"meshclaw_reconcile_verification_plan",
		"meshclaw_reconcile_runbook",
		"meshclaw_reconcile_runbook_check",
		"meshclaw_reconcile_rollback_plan",
		"meshclaw_reconcile_completion_plan",
	}
	result, err := callMCPTool("meshclaw_mcp_smoke_test_plan", map[string]interface{}{
		"client": "claude",
		"scope":  "k8s-replacement",
		"tools":  strings.Join(tools, ","),
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := result.(map[string]interface{})
	report := payload["report"].(map[string]interface{})
	checks := report["checks"].([]map[string]interface{})
	checkByTool := map[string]map[string]interface{}{}
	for _, check := range checks {
		if tool, _ := check["tool"].(string); tool != "" {
			checkByTool[tool] = check
		}
	}
	for _, tool := range tools {
		check := checkByTool[tool]
		if check == nil {
			t.Fatalf("missing smoke check for %s: %#v", tool, checks)
		}
		if fixture, _ := check["fixture_required"].(bool); !fixture {
			t.Fatalf("%s should require fixture evidence: %#v", tool, check)
		}
		args, ok := check["arguments"].(map[string]interface{})
		if !ok || len(args) != 1 {
			t.Fatalf("%s should expose one evidence-path argument: %#v", tool, check)
		}
		if _, ok := args["apply"]; ok {
			t.Fatalf("%s smoke args must not include apply: %#v", tool, args)
		}
		if _, ok := args["execute"]; ok {
			t.Fatalf("%s smoke args must not include execute: %#v", tool, args)
		}
	}
	expectedChecks := map[string][]string{
		"meshclaw_reconcile_apply_plan":        []string{"apply_plan_contract.plan_only=true", "apply_plan_contract.apply_allowed=false", "apply_plan_contract.requires_execution_preview_before_executor=true"},
		"meshclaw_reconcile_execution_preview": []string{"execution_preview_contract.preview_only=true", "execution_preview_contract.apply_allowed=false", "execution_preview_contract.commands_are_inert_templates=true"},
		"meshclaw_reconcile_verification_plan": []string{"verification_plan_contract.apply_allowed=false", "verification_plan_contract.requires_post_action_evidence=true"},
		"meshclaw_reconcile_runbook":           []string{"runbook_contract.review_only=true", "runbook_contract.apply_allowed=false"},
		"meshclaw_reconcile_runbook_check":     []string{"runbook_check_contract.gate_only=true", "runbook_check_contract.apply_allowed=false", "runbook_check_contract.requires_zero_critical_findings=true"},
		"meshclaw_reconcile_rollback_plan":     []string{"rollback_plan_contract.apply_allowed=false", "rollback_plan_contract.rollback_allowed=false"},
		"meshclaw_reconcile_completion_plan":   []string{"completion_plan_contract.complete_allowed=false", "completion_plan_contract.requires_final_evidence=true"},
	}
	for tool, wants := range expectedChecks {
		expected, _ := checkByTool[tool]["expected"].(string)
		for _, want := range wants {
			if !strings.Contains(expected, want) {
				t.Fatalf("%s expected output should include %q: %q", tool, want, expected)
			}
		}
	}
	manifest, ok := report["fixture_manifest"].([]map[string]interface{})
	if !ok || len(manifest) != len(tools) {
		t.Fatalf("reconcile contract smoke should include one fixture manifest per tool: %#v", report["fixture_manifest"])
	}
	manifestByTool := map[string]map[string]interface{}{}
	for _, fixture := range manifest {
		tool, _ := fixture["tool"].(string)
		manifestByTool[tool] = fixture
		if fixture["mutates_state"] != false {
			t.Fatalf("reconcile contract fixtures must be non-mutating: %#v", fixture)
		}
	}
	if manifestByTool["meshclaw_reconcile_verification_plan"]["placeholder"] != "<test-fixture-reconcile-execution-preview-evidence.json>" {
		t.Fatalf("verification fixture should name execution-preview evidence: %#v", manifestByTool["meshclaw_reconcile_verification_plan"])
	}
	if manifestByTool["meshclaw_reconcile_apply_plan"]["placeholder"] != "<test-fixture-reconcile-apply-gate-evidence.json>" {
		t.Fatalf("apply-plan fixture should name apply-gate evidence: %#v", manifestByTool["meshclaw_reconcile_apply_plan"])
	}
	if manifestByTool["meshclaw_reconcile_execution_preview"]["placeholder"] != "<test-fixture-reconcile-apply-plan-evidence.json>" {
		t.Fatalf("execution-preview fixture should name apply-plan evidence: %#v", manifestByTool["meshclaw_reconcile_execution_preview"])
	}
	if manifestByTool["meshclaw_reconcile_completion_plan"]["placeholder"] != "<test-fixture-reconcile-rollback-plan-evidence.json>" {
		t.Fatalf("completion fixture should name rollback-plan evidence: %#v", manifestByTool["meshclaw_reconcile_completion_plan"])
	}
}

func TestMCPSmokeTestPlanReconcileApprovalRequestContract(t *testing.T) {
	result, err := callMCPTool("meshclaw_mcp_smoke_test_plan", map[string]interface{}{
		"client": "claude",
		"scope":  "k8s-replacement",
		"tools":  "meshclaw_reconcile_approval_request",
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := result.(map[string]interface{})
	report := payload["report"].(map[string]interface{})
	checks := report["checks"].([]map[string]interface{})
	check := findCheckByTool(checks, "meshclaw_reconcile_approval_request")
	if check == nil {
		t.Fatalf("missing approval-request smoke check: %#v", checks)
	}
	if fixture, _ := check["fixture_required"].(bool); !fixture {
		t.Fatalf("approval-request smoke should require fixture evidence: %#v", check)
	}
	args, ok := check["arguments"].(map[string]interface{})
	if !ok || args["desired_path"] != "<test-fixture-desired-state.yaml>" {
		t.Fatalf("approval-request smoke should include desired fixture path: %#v", check)
	}
	if _, ok := args["apply"]; ok {
		t.Fatalf("approval-request smoke args must not include apply: %#v", args)
	}
	if _, ok := args["execute"]; ok {
		t.Fatalf("approval-request smoke args must not include execute: %#v", args)
	}
	expected, _ := check["expected"].(string)
	for _, want := range []string{"approval_request_contract.request_only=true", "approval_request_contract.operator_approval_recorded=false"} {
		if !strings.Contains(expected, want) {
			t.Fatalf("approval-request expected output should include %q: %q", want, expected)
		}
	}
	manifest := report["fixture_manifest"].([]map[string]interface{})
	if len(manifest) != 1 || manifest[0]["tool"] != "meshclaw_reconcile_approval_request" || manifest[0]["mutates_state"] != false {
		t.Fatalf("approval-request smoke should expose one non-mutating fixture: %#v", manifest)
	}
	minimumContent, ok := manifest[0]["minimum_content"].([]string)
	if !ok || !containsString(minimumContent, "approval_request_contract.request_only=true") || !containsString(minimumContent, "blocked actions must not execute") {
		t.Fatalf("approval-request fixture should require request-only contract content: %#v", manifest[0])
	}
}

func TestMCPSmokeTestPlanReconcileApplyGateContract(t *testing.T) {
	result, err := callMCPTool("meshclaw_mcp_smoke_test_plan", map[string]interface{}{
		"client": "claude",
		"scope":  "k8s-replacement",
		"tools":  "meshclaw_reconcile_apply_gate",
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := result.(map[string]interface{})
	report := payload["report"].(map[string]interface{})
	checks := report["checks"].([]map[string]interface{})
	check := findCheckByTool(checks, "meshclaw_reconcile_apply_gate")
	if check == nil {
		t.Fatalf("missing apply-gate smoke check: %#v", checks)
	}
	if fixture, _ := check["fixture_required"].(bool); !fixture {
		t.Fatalf("apply-gate smoke should require fixture evidence: %#v", check)
	}
	args, ok := check["arguments"].(map[string]interface{})
	if !ok || args["desired_path"] != "<test-fixture-desired-state.yaml>" || args["approval_evidence_path"] != "<test-fixture-reconcile-approval-request-evidence.json>" {
		t.Fatalf("apply-gate smoke should include desired and approval evidence fixture paths: %#v", check)
	}
	if _, ok := args["apply"]; ok {
		t.Fatalf("apply-gate smoke args must not include apply: %#v", args)
	}
	if _, ok := args["execute"]; ok {
		t.Fatalf("apply-gate smoke args must not include execute: %#v", args)
	}
	expected, _ := check["expected"].(string)
	for _, want := range []string{"apply_gate_contract.gate_only=true", "apply_gate_contract.apply_allowed=false", "apply_gate_contract.requires_operator_approval=true"} {
		if !strings.Contains(expected, want) {
			t.Fatalf("apply-gate expected output should include %q: %q", want, expected)
		}
	}
	manifest := report["fixture_manifest"].([]map[string]interface{})
	if len(manifest) != 1 || manifest[0]["tool"] != "meshclaw_reconcile_apply_gate" || manifest[0]["mutates_state"] != false {
		t.Fatalf("apply-gate smoke should expose one non-mutating fixture: %#v", manifest)
	}
	minimumContent, ok := manifest[0]["minimum_content"].([]string)
	if !ok || !containsString(minimumContent, "apply_gate_contract.gate_only=true") || !containsString(minimumContent, "apply_gate_contract.apply_allowed=false") {
		t.Fatalf("apply-gate fixture should require gate-only contract content: %#v", manifest[0])
	}
}

func TestMCPSmokeTestPlanContainerContractChain(t *testing.T) {
	tools := []string{
		"meshclaw_autoheal_container_apply_plan",
		"meshclaw_autoheal_container_verification_plan",
		"meshclaw_autoheal_container_runbook",
		"meshclaw_autoheal_container_runbook_check",
		"meshclaw_autoheal_container_rollback_plan",
		"meshclaw_autoheal_container_completion_plan",
		"meshclaw_autoheal_container_readiness_summary",
	}
	result, err := callMCPTool("meshclaw_mcp_smoke_test_plan", map[string]interface{}{
		"client": "claude",
		"scope":  "k8s-replacement",
		"tools":  strings.Join(tools, ","),
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := result.(map[string]interface{})
	report := payload["report"].(map[string]interface{})
	checks := report["checks"].([]map[string]interface{})
	checkByTool := map[string]map[string]interface{}{}
	for _, check := range checks {
		if tool, _ := check["tool"].(string); tool != "" {
			checkByTool[tool] = check
		}
	}
	for _, tool := range tools {
		check := checkByTool[tool]
		if check == nil {
			t.Fatalf("missing smoke check for %s: %#v", tool, checks)
		}
		if fixture, _ := check["fixture_required"].(bool); !fixture {
			t.Fatalf("%s should require fixture evidence: %#v", tool, check)
		}
		args, ok := check["arguments"].(map[string]interface{})
		if !ok || len(args) == 0 {
			t.Fatalf("%s should expose safe fixture arguments: %#v", tool, check)
		}
		if tool == "meshclaw_autoheal_container_apply_plan" {
			if args["plan_evidence_path"] != "<test-fixture-autoheal-plan-evidence.json>" || args["approved_by"] != "<operator>" {
				t.Fatalf("%s should expose autoheal plan fixture and operator placeholder: %#v", tool, check)
			}
		} else if len(args) != 1 {
			t.Fatalf("%s should expose one evidence-path argument: %#v", tool, check)
		}
		if _, ok := args["apply"]; ok {
			t.Fatalf("%s smoke args must not include apply: %#v", tool, args)
		}
		if _, ok := args["execute"]; ok {
			t.Fatalf("%s smoke args must not include execute: %#v", tool, args)
		}
	}
	expectedChecks := map[string][]string{
		"meshclaw_autoheal_container_apply_plan":        []string{"container_apply_plan_contract.plan_only=true", "container_apply_plan_contract.apply_allowed=false", "container_apply_plan_contract.direct_restart_allowed=false", "container_apply_plan_contract.requires_focused_runtime_evidence=true", "container_apply_plan_contract.runtime_evidence_required_count"},
		"meshclaw_autoheal_container_verification_plan": []string{"container_verification_plan_contract.apply_allowed=false", "container_verification_plan_contract.requires_container_logscan=true"},
		"meshclaw_autoheal_container_runbook":           []string{"container_runbook_contract.review_only=true", "container_runbook_contract.apply_allowed=false", "container_runbook_contract.apply_plan_runtime_evidence_required_count"},
		"meshclaw_autoheal_container_runbook_check":     []string{"container_runbook_check_contract.gate_only=true", "container_runbook_check_contract.requires_zero_critical_findings=true"},
		"meshclaw_autoheal_container_rollback_plan":     []string{"container_rollback_plan_contract.rollback_allowed=false", "container_rollback_plan_contract.requires_operator_approval=true"},
		"meshclaw_autoheal_container_completion_plan":   []string{"container_completion_plan_contract.complete_allowed=false", "container_completion_plan_contract.requires_final_evidence=true"},
		"meshclaw_autoheal_container_readiness_summary": []string{"container_readiness_summary_contract.summary_only=true", "container_readiness_summary_contract.requires_approval_gated_executor=true"},
	}
	for tool, wants := range expectedChecks {
		expected, _ := checkByTool[tool]["expected"].(string)
		for _, want := range wants {
			if !strings.Contains(expected, want) {
				t.Fatalf("%s expected output should include %q: %q", tool, want, expected)
			}
		}
	}
	manifest, ok := report["fixture_manifest"].([]map[string]interface{})
	if !ok || len(manifest) != len(tools) {
		t.Fatalf("container contract smoke should include one fixture manifest per tool: %#v", report["fixture_manifest"])
	}
	manifestByTool := map[string]map[string]interface{}{}
	for _, fixture := range manifest {
		tool, _ := fixture["tool"].(string)
		manifestByTool[tool] = fixture
		if fixture["mutates_state"] != false {
			t.Fatalf("container contract fixtures must be non-mutating: %#v", fixture)
		}
	}
	if manifestByTool["meshclaw_autoheal_container_verification_plan"]["placeholder"] != "<test-fixture-container-apply-plan-evidence.json>" {
		t.Fatalf("container verification fixture should name apply-plan evidence: %#v", manifestByTool["meshclaw_autoheal_container_verification_plan"])
	}
	verificationMinimumContent, ok := manifestByTool["meshclaw_autoheal_container_verification_plan"]["minimum_content"].([]string)
	if !ok || !containsString(verificationMinimumContent, "container_apply_plan_contract.runtime_evidence_required_count") {
		t.Fatalf("container verification fixture should require apply-plan runtime evidence count: %#v", manifestByTool["meshclaw_autoheal_container_verification_plan"])
	}
	if manifestByTool["meshclaw_autoheal_container_apply_plan"]["placeholder"] != "<test-fixture-autoheal-plan-evidence.json>" {
		t.Fatalf("container apply fixture should name autoheal-plan evidence: %#v", manifestByTool["meshclaw_autoheal_container_apply_plan"])
	}
	runbookMinimumContent, ok := manifestByTool["meshclaw_autoheal_container_runbook"]["minimum_content"].([]string)
	if !ok || !containsString(runbookMinimumContent, "apply_plan_runtime_evidence_required_count") {
		t.Fatalf("container runbook fixture should require verification runtime evidence count: %#v", manifestByTool["meshclaw_autoheal_container_runbook"])
	}
	if manifestByTool["meshclaw_autoheal_container_completion_plan"]["placeholder"] != "<test-fixture-container-rollback-plan-evidence.json>" {
		t.Fatalf("container completion fixture should name rollback-plan evidence: %#v", manifestByTool["meshclaw_autoheal_container_completion_plan"])
	}
}

func TestMCPRolloutPlanExplainsClientRefresh(t *testing.T) {
	result, err := callMCPTool("meshclaw_mcp_rollout_plan", map[string]interface{}{
		"client":         "claude",
		"branch":         "codex/ops-integration-plan-surface",
		"expected_tools": "meshclaw_ops_integration_plan,meshclaw_storage_guardrail_plan",
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("payload=%#v", result)
	}
	report, ok := payload["report"].(map[string]interface{})
	if !ok {
		t.Fatalf("report missing: %#v", payload)
	}
	if report["kind"] != "mcp_rollout_plan" || report["client"] != "claude" || report["branch"] != "codex/ops-integration-plan-surface" {
		t.Fatalf("unexpected mcp rollout report: %#v", report)
	}
	steps, ok := report["verification_steps"].([]map[string]interface{})
	if !ok || len(steps) < 6 {
		t.Fatalf("mcp rollout should include client verification steps: %#v", report["verification_steps"])
	}
	if steps[2]["tool"] != "meshclaw_mcp_profile_visibility_check" {
		t.Fatalf("mcp rollout should check profile visibility before client refresh: %#v", steps)
	}
	args, ok := steps[2]["arguments"].(map[string]interface{})
	if !ok || args["profile"] != "claude-lite" || args["expected_tools"] != "meshclaw_ops_integration_plan,meshclaw_storage_guardrail_plan" {
		t.Fatalf("mcp rollout profile visibility step should include expected tools: %#v", steps[2]["arguments"])
	}
	truth, ok := report["truth_model"].([]string)
	if !ok || !containsString(truth, "PR/source changes are not visible to an MCP client until a MeshClaw binary containing those changes is built and selected by that client.") {
		t.Fatalf("mcp rollout should explain binary/client visibility: %#v", report["truth_model"])
	}
	evidenceRecord, ok := payload["evidence"].(evidence.Record)
	if !ok || evidenceRecord.Kind != "mcp-rollout-plan" {
		t.Fatalf("mcp rollout plan should store evidence: %#v", payload["evidence"])
	}
}

func TestMCPRolloutPlanDefaultsIncludeK8sReplacementChain(t *testing.T) {
	result, err := callMCPTool("meshclaw_mcp_rollout_plan", map[string]interface{}{
		"client": "claude",
		"branch": "codex/default-mcp-surface",
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("payload=%#v", result)
	}
	report, ok := payload["report"].(map[string]interface{})
	if !ok {
		t.Fatalf("report missing: %#v", payload)
	}
	expectedTools, ok := report["expected_tools"].([]string)
	if !ok {
		t.Fatalf("expected_tools missing: %#v", report["expected_tools"])
	}
	for _, want := range []string{
		"meshclaw_mcp_surface",
		"meshclaw_tool_recommend",
		"meshclaw_service_registry_plan",
		"meshclaw_capacity_scale_plan",
		"meshclaw_storage_guardrail_plan",
		"meshclaw_ops_integration_plan",
		"meshclaw_reconcile_validate_desired",
		"meshclaw_reconcile_plan",
		"meshclaw_reconcile_approval_request",
		"meshclaw_reconcile_apply_gate",
		"meshclaw_reconcile_apply_plan",
		"meshclaw_reconcile_execution_preview",
		"meshclaw_reconcile_readiness_summary",
		"meshclaw_autoheal_plan",
		"meshclaw_autoheal_container_apply_plan",
		"meshclaw_autoheal_container_readiness_summary",
		"meshclaw_autoheal_container_executor_gate",
		"meshclaw_autoheal_container_executor",
		"meshclaw_analyze_logs",
	} {
		if !containsString(expectedTools, want) {
			t.Fatalf("default rollout expected_tools should include %s: %#v", want, expectedTools)
		}
	}
	steps, ok := report["verification_steps"].([]map[string]interface{})
	if !ok || len(steps) < 6 {
		t.Fatalf("mcp rollout should include client verification steps: %#v", report["verification_steps"])
	}
	args, ok := steps[2]["arguments"].(map[string]interface{})
	if !ok || args["profile"] != "claude-lite" {
		t.Fatalf("mcp rollout profile visibility step should target claude-lite: %#v", steps[2]["arguments"])
	}
	visibilityExpectedTools, ok := args["expected_tools"].(string)
	if !ok {
		t.Fatalf("profile visibility expected_tools should be a string: %#v", args["expected_tools"])
	}
	for _, want := range []string{"meshclaw_reconcile_validate_desired", "meshclaw_reconcile_plan", "meshclaw_reconcile_approval_request", "meshclaw_reconcile_apply_gate", "meshclaw_reconcile_apply_plan", "meshclaw_reconcile_execution_preview", "meshclaw_reconcile_readiness_summary", "meshclaw_autoheal_container_apply_plan", "meshclaw_autoheal_container_readiness_summary", "meshclaw_autoheal_container_executor_gate", "meshclaw_autoheal_container_executor", "meshclaw_analyze_logs"} {
		if !strings.Contains(visibilityExpectedTools, want) {
			t.Fatalf("profile visibility expected_tools should include %s: %q", want, visibilityExpectedTools)
		}
	}
	visibility, ok := report["profile_visibility"].(map[string]interface{})
	if !ok {
		t.Fatalf("default rollout report should include profile visibility preview: %#v", report["profile_visibility"])
	}
	if visibility["profile"] != "claude-lite" || visibility["ready"] != true {
		t.Fatalf("default rollout visibility should be ready for claude-lite: %#v", visibility)
	}
	if missing, ok := visibility["missing_tools"].([]string); !ok || len(missing) != 0 {
		t.Fatalf("default rollout visibility should have zero missing tools: %#v", visibility["missing_tools"])
	}
	readiness, ok := report["rollout_readiness"].(map[string]interface{})
	if !ok {
		t.Fatalf("default rollout report should include rollout readiness: %#v", report["rollout_readiness"])
	}
	if readiness["status"] != "plan_ready" || readiness["source_ready"] != true || readiness["binary_built"] != false || readiness["client_reloaded"] != false {
		t.Fatalf("rollout readiness should describe pre-build/pre-reload state: %#v", readiness)
	}
	if readiness["expected_tool_count"] != len(expectedTools) || readiness["can_use_new_tools_now"] != false {
		t.Fatalf("rollout readiness should count tools and block immediate use: %#v", readiness)
	}
	if readiness["mutates_live_servers"] != false || readiness["deploys_binary"] != false || readiness["restarts_mcp_client"] != false || readiness["grants_future_approval"] != false {
		t.Fatalf("rollout readiness should remain plan-only: %#v", readiness)
	}
	checkpoints, ok := report["operator_checkpoints"].([]map[string]interface{})
	if !ok || len(checkpoints) != 5 {
		t.Fatalf("mcp rollout should include operator checkpoints: %#v", report["operator_checkpoints"])
	}
	if checkpoints[1]["phase"] != "binary" || checkpoints[1]["mutates_live_servers"] != false {
		t.Fatalf("binary checkpoint should not mutate live servers: %#v", checkpoints[1])
	}
	if checkpoints[3]["tool"] != "meshclaw_mcp_profile_visibility_check" {
		t.Fatalf("visibility checkpoint should name profile visibility tool: %#v", checkpoints[3])
	}
	if checkpoints[4]["phase"] != "smoke" || checkpoints[4]["tool"] != "meshclaw_mcp_smoke_test_plan" {
		t.Fatalf("smoke checkpoint should require read-only smoke plan: %#v", checkpoints[4])
	}
	smokePrompts, ok := report["post_reload_smoke_prompts"].([]map[string]interface{})
	if !ok || len(smokePrompts) != len(expectedTools) {
		t.Fatalf("mcp rollout should include one post-reload smoke prompt per expected tool: %#v", report["post_reload_smoke_prompts"])
	}
	if smokePrompts[0]["tool"] != expectedTools[0] || smokePrompts[0]["expected_recommendation"] != expectedTools[0] {
		t.Fatalf("post-reload smoke prompt should map to the expected tool: %#v", smokePrompts[0])
	}
	for _, prompt := range smokePrompts {
		if prompt["requires_approval"] != false {
			t.Fatalf("post-reload smoke prompts should not require approval: %#v", prompt)
		}
		behavior, ok := prompt["expected_behavior"].(string)
		if !ok || !strings.Contains(behavior, "no apply") || !strings.Contains(behavior, "live server mutation") {
			t.Fatalf("post-reload smoke prompts should describe non-mutating behavior: %#v", prompt)
		}
	}
	logscanSmoke := findSmokePromptByTool(smokePrompts, "meshclaw_analyze_logs")
	if logscanSmoke == nil || !strings.Contains(stringMapValue(logscanSmoke, "expected_behavior"), "autoheal_handoff.runtime_evidence_checklist") {
		t.Fatalf("logscan post-reload smoke should require runtime checklist evidence: %#v", logscanSmoke)
	}
	if logscanSmoke == nil || !strings.Contains(stringMapValue(logscanSmoke, "expected_behavior"), "handoff_contract.apply_allowed=false") {
		t.Fatalf("logscan post-reload smoke should require non-apply handoff contract: %#v", logscanSmoke)
	}
	for _, want := range []string{"handoff_contract.direct_restart_allowed=false", "handoff_contract.requires_focused_runtime_evidence=true", "handoff_contract.runtime_evidence_checklist_count", "handoff_contract.next_required_tool"} {
		if logscanSmoke == nil || !strings.Contains(stringMapValue(logscanSmoke, "expected_behavior"), want) {
			t.Fatalf("logscan post-reload smoke should require structured handoff gate %q: %#v", want, logscanSmoke)
		}
	}
	reconcileReadinessSmoke := findSmokePromptByTool(smokePrompts, "meshclaw_reconcile_readiness_summary")
	if reconcileReadinessSmoke == nil || !strings.Contains(stringMapValue(reconcileReadinessSmoke, "expected_behavior"), "readiness_contract.apply_allowed=false") || !strings.Contains(stringMapValue(reconcileReadinessSmoke, "expected_behavior"), "readiness_contract.grants_future_approval=false") {
		t.Fatalf("reconcile readiness post-reload smoke should require non-approval readiness contract: %#v", reconcileReadinessSmoke)
	}
	desiredValidationSmoke := findSmokePromptByTool(smokePrompts, "meshclaw_reconcile_validate_desired")
	if desiredValidationSmoke == nil || !strings.Contains(stringMapValue(desiredValidationSmoke, "expected_behavior"), "desired_validation_contract.validation_only=true") || !strings.Contains(stringMapValue(desiredValidationSmoke, "expected_behavior"), "desired_validation_contract.yaml_keys_grant_approval=false") {
		t.Fatalf("desired validation post-reload smoke should require validation contract: %#v", desiredValidationSmoke)
	}
	reconcilePlanSmoke := findSmokePromptByTool(smokePrompts, "meshclaw_reconcile_plan")
	if reconcilePlanSmoke == nil || !strings.Contains(stringMapValue(reconcilePlanSmoke, "expected_behavior"), "reconcile_plan_contract.dry_run_only=true") || !strings.Contains(stringMapValue(reconcilePlanSmoke, "expected_behavior"), "reconcile_plan_contract.apply_allowed=false") {
		t.Fatalf("reconcile plan post-reload smoke should require dry-run contract: %#v", reconcilePlanSmoke)
	}
	approvalRequestSmoke := findSmokePromptByTool(smokePrompts, "meshclaw_reconcile_approval_request")
	if approvalRequestSmoke == nil || !strings.Contains(stringMapValue(approvalRequestSmoke, "expected_behavior"), "approval_request_contract.request_only=true") || !strings.Contains(stringMapValue(approvalRequestSmoke, "expected_behavior"), "approval_request_contract.operator_approval_recorded=false") {
		t.Fatalf("approval-request post-reload smoke should require request-only contract: %#v", approvalRequestSmoke)
	}
	applyGateSmoke := findSmokePromptByTool(smokePrompts, "meshclaw_reconcile_apply_gate")
	if applyGateSmoke == nil || !strings.Contains(stringMapValue(applyGateSmoke, "expected_behavior"), "apply_gate_contract.gate_only=true") || !strings.Contains(stringMapValue(applyGateSmoke, "expected_behavior"), "apply_gate_contract.apply_allowed=false") {
		t.Fatalf("apply-gate post-reload smoke should require gate-only contract: %#v", applyGateSmoke)
	}
	applyPlanSmoke := findSmokePromptByTool(smokePrompts, "meshclaw_reconcile_apply_plan")
	if applyPlanSmoke == nil || !strings.Contains(stringMapValue(applyPlanSmoke, "expected_behavior"), "apply_plan_contract.plan_only=true") || !strings.Contains(stringMapValue(applyPlanSmoke, "expected_behavior"), "apply_plan_contract.requires_execution_preview_before_executor=true") {
		t.Fatalf("apply-plan post-reload smoke should require preview-before-executor contract: %#v", applyPlanSmoke)
	}
	executionPreviewSmoke := findSmokePromptByTool(smokePrompts, "meshclaw_reconcile_execution_preview")
	if executionPreviewSmoke == nil || !strings.Contains(stringMapValue(executionPreviewSmoke, "expected_behavior"), "execution_preview_contract.preview_only=true") || !strings.Contains(stringMapValue(executionPreviewSmoke, "expected_behavior"), "execution_preview_contract.commands_are_inert_templates=true") {
		t.Fatalf("execution-preview post-reload smoke should require inert-template contract: %#v", executionPreviewSmoke)
	}
	containerReadinessSmoke := findSmokePromptByTool(smokePrompts, "meshclaw_autoheal_container_readiness_summary")
	if containerReadinessSmoke == nil || !strings.Contains(stringMapValue(containerReadinessSmoke, "expected_behavior"), "container_readiness_summary_contract.summary_only=true") || !strings.Contains(stringMapValue(containerReadinessSmoke, "expected_behavior"), "container_readiness_summary_contract.requires_approval_gated_executor=true") {
		t.Fatalf("container readiness post-reload smoke should require container readiness summary contract: %#v", containerReadinessSmoke)
	}
	troubleshooting, ok := report["rollout_troubleshooting"].([]map[string]interface{})
	if !ok || len(troubleshooting) != 4 {
		t.Fatalf("mcp rollout should include troubleshooting guidance: %#v", report["rollout_troubleshooting"])
	}
	for i, wantLayer := range []string{"binary", "profile_allowlist", "client_cache", "recommendation_policy"} {
		if troubleshooting[i]["likely_layer"] != wantLayer {
			t.Fatalf("troubleshooting layer %d should be %s: %#v", i, wantLayer, troubleshooting[i])
		}
	}
	if troubleshooting[1]["next_tool"] != "meshclaw_mcp_profile_visibility_check" {
		t.Fatalf("profile troubleshooting should use visibility check: %#v", troubleshooting[1])
	}
	if tools, ok := troubleshooting[1]["expected_tools"].([]string); !ok || len(tools) != len(expectedTools) {
		t.Fatalf("profile troubleshooting should carry expected tools: %#v", troubleshooting[1]["expected_tools"])
	}
	recommendationCheck, ok := troubleshooting[3]["check"].(string)
	if !ok || !strings.Contains(recommendationCheck, "required_evidence") || !strings.Contains(recommendationCheck, "stop_before") {
		t.Fatalf("recommendation troubleshooting should require evidence boundaries: %#v", troubleshooting[3])
	}
	safeResponse, ok := troubleshooting[0]["safe_response"].(string)
	if !ok || !strings.Contains(safeResponse, "do not deploy to live servers") {
		t.Fatalf("binary troubleshooting should block live deploys: %#v", troubleshooting[0])
	}
	evidenceChecklist, ok := report["rollout_evidence_checklist"].([]map[string]interface{})
	if !ok || len(evidenceChecklist) != 6 {
		t.Fatalf("mcp rollout should include evidence checklist: %#v", report["rollout_evidence_checklist"])
	}
	for i, wantPhase := range []string{"source", "binary", "client", "visibility", "smoke", "rollback"} {
		if evidenceChecklist[i]["phase"] != wantPhase || evidenceChecklist[i]["required"] != true || evidenceChecklist[i]["mutates_live_servers"] != false {
			t.Fatalf("evidence checklist item %d should be required and non-mutating for phase %s: %#v", i, wantPhase, evidenceChecklist[i])
		}
	}
	if evidenceChecklist[3]["tool"] != "meshclaw_mcp_profile_visibility_check" || evidenceChecklist[4]["tool"] != "meshclaw_mcp_smoke_test_plan" {
		t.Fatalf("evidence checklist should name visibility and smoke tools: %#v", evidenceChecklist)
	}
	if tools, ok := evidenceChecklist[3]["value"].([]string); !ok || len(tools) != len(expectedTools) {
		t.Fatalf("visibility evidence should carry expected tools: %#v", evidenceChecklist[3]["value"])
	}
	successCriteria, ok := report["rollout_success_criteria"].(map[string]interface{})
	if !ok {
		t.Fatalf("mcp rollout should include success criteria: %#v", report["rollout_success_criteria"])
	}
	if successCriteria["status_if_all_pass"] != "ready_for_operator_review" || successCriteria["expected_tool_count"] != len(expectedTools) {
		t.Fatalf("success criteria should summarize operator review readiness: %#v", successCriteria)
	}
	for _, key := range []string{"success", "blocked", "rollback_recommended"} {
		items, ok := successCriteria[key].([]string)
		if !ok || len(items) == 0 {
			t.Fatalf("success criteria should include %s entries: %#v", key, successCriteria[key])
		}
	}
	if successCriteria["mutates_live_servers"] != false || successCriteria["grants_apply_approval"] != false {
		t.Fatalf("success criteria should remain non-mutating and not grant apply approval: %#v", successCriteria)
	}
	successItems, ok := successCriteria["success"].([]string)
	if !ok || !containsString(successItems, "reconcile readiness summary exposes desired-state executor-contract evidence before any future apply loop") || !containsString(successItems, "container readiness summary exposes apply-loop gate and executor-contract evidence before any future apply loop") {
		t.Fatalf("success criteria should mention readiness executor contracts: %#v", successCriteria["success"])
	}
	handoff, ok := report["operator_handoff"].(map[string]interface{})
	if !ok {
		t.Fatalf("mcp rollout should include operator handoff: %#v", report["operator_handoff"])
	}
	if handoff["handoff_required"] != true || handoff["handoff_to"] != "operator" {
		t.Fatalf("operator handoff should require human continuation: %#v", handoff)
	}
	stopsBefore, ok := handoff["codex_stops_before"].([]string)
	if !ok || !containsString(stopsBefore, "deploying to live servers") || !containsString(stopsBefore, "restarting MCP clients") {
		t.Fatalf("operator handoff should state Codex stop boundaries: %#v", handoff["codex_stops_before"])
	}
	resumesWith, ok := handoff["operator_resumes_with"].([]string)
	if !ok || !containsString(resumesWith, "refresh the MCP client/server process") {
		t.Fatalf("operator handoff should state operator resume steps: %#v", handoff["operator_resumes_with"])
	}
	liveSafety, ok := handoff["container_executor_live_safety"].([]string)
	if !ok || !containsString(liveSafety, "run meshclaw_autoheal_container_executor with dry_run=true before any live execution") || !containsString(liveSafety, "require matching approved_by, dry_run=false, execute=true, and live_approval_phrase exactly 'execute container self-heal approved' for live restart") || !containsString(liveSafety, "run meshclaw_autoheal_container_executor_verify with post-action agent evidence and focused container-logscan evidence before closeout") {
		t.Fatalf("operator handoff should include container executor live safety: %#v", handoff["container_executor_live_safety"])
	}
	if handoff["deploys_binary"] != false || handoff["restarts_mcp_client"] != false || handoff["mutates_live_servers"] != false || handoff["grants_future_approval"] != false {
		t.Fatalf("operator handoff should remain plan-only: %#v", handoff)
	}
	localSteps, ok := report["local_verification_steps"].([]map[string]interface{})
	if !ok || len(localSteps) != 4 {
		t.Fatalf("mcp rollout should include local verification steps: %#v", report["local_verification_steps"])
	}
	for _, step := range localSteps {
		if step["mutates_live_servers"] != false {
			t.Fatalf("local verification step should not mutate live servers: %#v", step)
		}
	}
	if localSteps[1]["command"] != "go build ./..." {
		t.Fatalf("local build step should be explicit: %#v", localSteps[1])
	}
	if localSteps[2]["tool"] != "meshclaw_mcp_surface" || localSteps[3]["tool"] != "meshclaw_mcp_profile_visibility_check" {
		t.Fatalf("MCP verification should use tool calls, not shell commands: %#v", localSteps)
	}
	profileArgs, ok := localSteps[3]["arguments"].(map[string]interface{})
	if !ok || profileArgs["profile"] != "claude-lite" {
		t.Fatalf("profile verification should carry client profile arguments: %#v", localSteps[3])
	}
	stackDiscipline, ok := report["stack_review_discipline"].(map[string]interface{})
	if !ok {
		t.Fatalf("mcp rollout should include stack review discipline: %#v", report["stack_review_discipline"])
	}
	if stackDiscipline["head_branch"] != "codex/default-mcp-surface" {
		t.Fatalf("stack discipline should carry rollout head branch: %#v", stackDiscipline)
	}
	reviewOrder, ok := stackDiscipline["review_order"].([]string)
	if !ok || len(reviewOrder) < 4 || !strings.Contains(reviewOrder[0], "lowest-base PR first") {
		t.Fatalf("stack discipline should include review order: %#v", stackDiscipline["review_order"])
	}
	mergeGate, ok := stackDiscipline["merge_gate"].(string)
	if !ok || !strings.Contains(mergeGate, "all lower layers reviewed") {
		t.Fatalf("stack discipline should include merge gate: %#v", stackDiscipline["merge_gate"])
	}
	if stackDiscipline["mutates_live_servers"] != false || stackDiscipline["deploys_binary"] != false || stackDiscipline["restarts_mcp_client"] != false || stackDiscipline["grants_apply_approval"] != false {
		t.Fatalf("stack discipline should remain plan-only: %#v", stackDiscipline)
	}
	rollbackPoints, ok := report["rollback_decision_points"].([]map[string]interface{})
	if !ok || len(rollbackPoints) != 4 {
		t.Fatalf("mcp rollout should include rollback decision points: %#v", report["rollback_decision_points"])
	}
	for i, wantPhase := range []string{"surface", "profile", "recommendation", "smoke"} {
		if rollbackPoints[i]["phase"] != wantPhase || rollbackPoints[i]["mutates_live_servers"] != false {
			t.Fatalf("rollback point %d should be non-mutating for phase %s: %#v", i, wantPhase, rollbackPoints[i])
		}
		if rollbackPoints[i]["rollback_if"] == "" || rollbackPoints[i]["restore"] == "" || rollbackPoints[i]["evidence"] == "" {
			t.Fatalf("rollback point should include condition, restore target, and evidence: %#v", rollbackPoints[i])
		}
	}
	if rollbackPoints[0]["expected_tool_count"] != len(expectedTools) {
		t.Fatalf("surface rollback point should count expected tools: %#v", rollbackPoints[0])
	}
	summaryCounts, ok := report["rollout_summary_counts"].(map[string]interface{})
	if !ok {
		t.Fatalf("mcp rollout should include summary counts: %#v", report["rollout_summary_counts"])
	}
	if summaryCounts["expected_tools"] != len(expectedTools) || summaryCounts["post_reload_smoke_prompts"] != len(expectedTools) {
		t.Fatalf("summary counts should count expected tools and smoke prompts: %#v", summaryCounts)
	}
	for key, want := range map[string]int{
		"operator_checkpoints":     5,
		"verification_steps":       6,
		"local_verification_steps": 4,
		"evidence_checklist_items": 6,
		"troubleshooting_layers":   4,
		"rollback_decision_points": 4,
	} {
		if summaryCounts[key] != want {
			t.Fatalf("summary count %s should be %d: %#v", key, want, summaryCounts)
		}
	}
	for _, key := range []string{"mutating_steps", "live_server_mutations", "binary_deployments", "mcp_client_restarts", "future_approval_grants"} {
		if summaryCounts[key] != 0 {
			t.Fatalf("summary count %s should be zero: %#v", key, summaryCounts)
		}
	}
	readinessLabels, ok := report["rollout_readiness_labels"].(map[string]interface{})
	if !ok {
		t.Fatalf("mcp rollout should include readiness labels: %#v", report["rollout_readiness_labels"])
	}
	for _, key := range []string{"safe_now", "operator_required", "blocked_until", "never_in_plan"} {
		items, ok := readinessLabels[key].([]string)
		if !ok || len(items) == 0 {
			t.Fatalf("readiness labels should include %s items: %#v", key, readinessLabels[key])
		}
	}
	if readinessLabels["can_use_new_tools_now"] != false || readinessLabels["mutates_live_servers"] != false {
		t.Fatalf("readiness labels should block immediate tool use and live mutation: %#v", readinessLabels)
	}
	refreshMatrix, ok := report["client_refresh_matrix"].([]map[string]interface{})
	if !ok || len(refreshMatrix) != 3 {
		t.Fatalf("mcp rollout should include client refresh matrix: %#v", report["client_refresh_matrix"])
	}
	if refreshMatrix[0]["client"] != "claude" || refreshMatrix[1]["client"] != "claude-lite" || refreshMatrix[2]["client"] != "generic-mcp-client" {
		t.Fatalf("client refresh matrix should cover selected, claude-lite, and generic clients: %#v", refreshMatrix)
	}
	for _, row := range refreshMatrix {
		if row["restarts_mcp_client"] != false || row["mutates_live_servers"] != false || row["reversible"] != true {
			t.Fatalf("client refresh matrix should stay operator-driven and reversible: %#v", row)
		}
		verifyWith, ok := row["verify_with"].([]string)
		if !ok || len(verifyWith) == 0 {
			t.Fatalf("client refresh matrix should name verification tools: %#v", row)
		}
	}
	acceptance, ok := report["operator_acceptance_checklist"].([]map[string]interface{})
	if !ok || len(acceptance) != 5 {
		t.Fatalf("mcp rollout should include operator acceptance checklist: %#v", report["operator_acceptance_checklist"])
	}
	for _, item := range acceptance {
		if item["required"] != true || item["mutates_live_servers"] != false {
			t.Fatalf("acceptance checklist items should be required and non-mutating: %#v", item)
		}
		if item["accept_if"] == "" || item["evidence"] == "" {
			t.Fatalf("acceptance checklist items should include condition and evidence: %#v", item)
		}
	}
	if acceptance[2]["expected_tool_count"] != len(expectedTools) {
		t.Fatalf("surface acceptance should count expected tools: %#v", acceptance[2])
	}
	operatorBrief, ok := report["operator_brief"].(map[string]interface{})
	if !ok {
		t.Fatalf("mcp rollout should include operator brief: %#v", report["operator_brief"])
	}
	if operatorBrief["client"] != "claude" || operatorBrief["branch"] != "codex/default-mcp-surface" || operatorBrief["expected_tool_count"] != len(expectedTools) {
		t.Fatalf("operator brief should carry client, branch, and tool count: %#v", operatorBrief)
	}
	briefLines, ok := operatorBrief["shareable_summary"].([]string)
	if !ok || len(briefLines) < 5 {
		t.Fatalf("operator brief should include shareable summary lines: %#v", operatorBrief["shareable_summary"])
	}
	goNoGo, ok := operatorBrief["go_no_go"].(map[string]interface{})
	if !ok || goNoGo["go_when"] == "" || goNoGo["no_go_when"] == "" {
		t.Fatalf("operator brief should include go/no-go criteria: %#v", operatorBrief["go_no_go"])
	}
	if operatorBrief["mutates_live_servers"] != false || operatorBrief["deploys_binary"] != false || operatorBrief["restarts_mcp_client"] != false || operatorBrief["grants_apply_approval"] != false {
		t.Fatalf("operator brief should remain plan-only: %#v", operatorBrief)
	}
	copyPasteStatus, ok := report["copy_paste_status"].([]string)
	if !ok || len(copyPasteStatus) != 6 {
		t.Fatalf("mcp rollout should include copy-paste status lines: %#v", report["copy_paste_status"])
	}
	if !strings.Contains(copyPasteStatus[0], "client=claude") || !strings.Contains(copyPasteStatus[0], "branch=codex/default-mcp-surface") {
		t.Fatalf("copy-paste status should include client and branch: %#v", copyPasteStatus)
	}
	for _, want := range []string{"live server deployment", "apply=true", "execute=true", "future approval grants"} {
		if !strings.Contains(copyPasteStatus[5], want) {
			t.Fatalf("copy-paste status should include forbidden action %q: %#v", want, copyPasteStatus[5])
		}
	}
	riskRegister, ok := report["rollout_risk_register"].([]map[string]interface{})
	if !ok || len(riskRegister) != 5 {
		t.Fatalf("mcp rollout should include risk register: %#v", report["rollout_risk_register"])
	}
	for i, wantLayer := range []string{"source", "binary", "client_refresh", "profile", "smoke"} {
		if riskRegister[i]["layer"] != wantLayer || riskRegister[i]["mutates_live_servers"] != false {
			t.Fatalf("risk register item %d should be non-mutating for layer %s: %#v", i, wantLayer, riskRegister[i])
		}
		for _, key := range []string{"risk", "impact", "detection", "mitigation"} {
			if riskRegister[i][key] == "" {
				t.Fatalf("risk register item should include %s: %#v", key, riskRegister[i])
			}
		}
	}
	commandCompatibility, ok := report["command_compatibility"].([]map[string]interface{})
	if !ok || len(commandCompatibility) != 3 {
		t.Fatalf("mcp rollout should include MCP command compatibility forms: %#v", report["command_compatibility"])
	}
	for i, wantForm := range []string{"meshclaw mcp --profile claude-lite", "meshclaw mcp serve --profile claude-lite", "meshclaw mcp stdio --profile claude-lite"} {
		if commandCompatibility[i]["form"] != wantForm || commandCompatibility[i]["client"] != "claude" || commandCompatibility[i]["mutates_live_servers"] != false {
			t.Fatalf("command compatibility item %d should describe non-mutating form %q: %#v", i, wantForm, commandCompatibility[i])
		}
	}
	if commandCompatibility[0]["recommended"] != true || commandCompatibility[1]["recommended"] != false || commandCompatibility[2]["recommended"] != false {
		t.Fatalf("command compatibility should mark one preferred form: %#v", commandCompatibility)
	}
}

func TestMCPRolloutPlanReconcileContractSmokePrompts(t *testing.T) {
	expectedTools := []string{
		"meshclaw_reconcile_apply_plan",
		"meshclaw_reconcile_execution_preview",
		"meshclaw_reconcile_verification_plan",
		"meshclaw_reconcile_runbook",
		"meshclaw_reconcile_runbook_check",
		"meshclaw_reconcile_rollback_plan",
		"meshclaw_reconcile_completion_plan",
	}
	result, err := callMCPTool("meshclaw_mcp_rollout_plan", map[string]interface{}{
		"client":         "claude",
		"branch":         "codex/reconcile-contract-chain",
		"expected_tools": strings.Join(expectedTools, ","),
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := result.(map[string]interface{})
	report := payload["report"].(map[string]interface{})
	smokePrompts := report["post_reload_smoke_prompts"].([]map[string]interface{})
	expectedChecks := map[string][]string{
		"meshclaw_reconcile_apply_plan":        []string{"apply_plan_contract.plan_only=true", "apply_plan_contract.apply_allowed=false", "apply_plan_contract.requires_execution_preview_before_executor=true"},
		"meshclaw_reconcile_execution_preview": []string{"execution_preview_contract.preview_only=true", "execution_preview_contract.apply_allowed=false", "execution_preview_contract.commands_are_inert_templates=true"},
		"meshclaw_reconcile_verification_plan": []string{"verification_plan_contract.apply_allowed=false", "verification_plan_contract.requires_post_action_evidence=true"},
		"meshclaw_reconcile_runbook":           []string{"runbook_contract.review_only=true", "runbook_contract.apply_allowed=false"},
		"meshclaw_reconcile_runbook_check":     []string{"runbook_check_contract.gate_only=true", "runbook_check_contract.requires_zero_critical_findings=true"},
		"meshclaw_reconcile_rollback_plan":     []string{"rollback_plan_contract.apply_allowed=false", "rollback_plan_contract.rollback_allowed=false"},
		"meshclaw_reconcile_completion_plan":   []string{"completion_plan_contract.complete_allowed=false", "completion_plan_contract.requires_final_evidence=true"},
	}
	for tool, wants := range expectedChecks {
		smoke := findSmokePromptByTool(smokePrompts, tool)
		if smoke == nil {
			t.Fatalf("missing rollout smoke prompt for %s: %#v", tool, smokePrompts)
		}
		behavior := stringMapValue(smoke, "expected_behavior")
		for _, want := range wants {
			if !strings.Contains(behavior, want) {
				t.Fatalf("%s rollout smoke should include %q: %q", tool, want, behavior)
			}
		}
		if !strings.Contains(behavior, "no apply") || !strings.Contains(behavior, "live server mutation") {
			t.Fatalf("%s rollout smoke should remain non-mutating: %q", tool, behavior)
		}
	}
}

func TestMCPRolloutPlanReconcileApprovalRequestContractSmokePrompt(t *testing.T) {
	result, err := callMCPTool("meshclaw_mcp_rollout_plan", map[string]interface{}{
		"client":         "claude",
		"branch":         "codex/reconcile-approval-request-contract",
		"expected_tools": "meshclaw_reconcile_approval_request",
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := result.(map[string]interface{})
	report := payload["report"].(map[string]interface{})
	smokePrompts := report["post_reload_smoke_prompts"].([]map[string]interface{})
	smoke := findSmokePromptByTool(smokePrompts, "meshclaw_reconcile_approval_request")
	if smoke == nil {
		t.Fatalf("missing approval-request rollout smoke prompt: %#v", smokePrompts)
	}
	behavior := stringMapValue(smoke, "expected_behavior")
	for _, want := range []string{"approval_request_contract.request_only=true", "approval_request_contract.operator_approval_recorded=false", "no apply", "live server mutation"} {
		if !strings.Contains(behavior, want) {
			t.Fatalf("approval-request rollout smoke should include %q: %q", want, behavior)
		}
	}
	if smoke["requires_approval"] != false {
		t.Fatalf("approval-request rollout smoke itself should not require approval: %#v", smoke)
	}
}

func TestMCPRolloutPlanReconcileApplyGateContractSmokePrompt(t *testing.T) {
	result, err := callMCPTool("meshclaw_mcp_rollout_plan", map[string]interface{}{
		"client":         "claude",
		"branch":         "codex/reconcile-apply-gate-contract",
		"expected_tools": "meshclaw_reconcile_apply_gate",
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := result.(map[string]interface{})
	report := payload["report"].(map[string]interface{})
	smokePrompts := report["post_reload_smoke_prompts"].([]map[string]interface{})
	smoke := findSmokePromptByTool(smokePrompts, "meshclaw_reconcile_apply_gate")
	if smoke == nil {
		t.Fatalf("missing apply-gate rollout smoke prompt: %#v", smokePrompts)
	}
	behavior := stringMapValue(smoke, "expected_behavior")
	for _, want := range []string{"apply_gate_contract.gate_only=true", "apply_gate_contract.apply_allowed=false", "apply_gate_contract.requires_operator_approval=true", "no apply", "live server mutation"} {
		if !strings.Contains(behavior, want) {
			t.Fatalf("apply-gate rollout smoke should include %q: %q", want, behavior)
		}
	}
	if smoke["requires_approval"] != false {
		t.Fatalf("apply-gate rollout smoke itself should not require approval: %#v", smoke)
	}
}

func TestMCPRolloutPlanContainerContractSmokePrompts(t *testing.T) {
	expectedTools := []string{
		"meshclaw_autoheal_container_apply_plan",
		"meshclaw_autoheal_container_verification_plan",
		"meshclaw_autoheal_container_runbook",
		"meshclaw_autoheal_container_runbook_check",
		"meshclaw_autoheal_container_rollback_plan",
		"meshclaw_autoheal_container_completion_plan",
		"meshclaw_autoheal_container_readiness_summary",
	}
	result, err := callMCPTool("meshclaw_mcp_rollout_plan", map[string]interface{}{
		"client":         "claude",
		"branch":         "codex/container-contract-chain",
		"expected_tools": strings.Join(expectedTools, ","),
	})
	if err != nil {
		t.Fatal(err)
	}
	payload := result.(map[string]interface{})
	report := payload["report"].(map[string]interface{})
	smokePrompts := report["post_reload_smoke_prompts"].([]map[string]interface{})
	expectedChecks := map[string][]string{
		"meshclaw_autoheal_container_apply_plan":        []string{"container_apply_plan_contract.plan_only=true", "container_apply_plan_contract.apply_allowed=false", "container_apply_plan_contract.direct_restart_allowed=false", "container_apply_plan_contract.requires_focused_runtime_evidence=true", "container_apply_plan_contract.runtime_evidence_required_count"},
		"meshclaw_autoheal_container_verification_plan": []string{"container_verification_plan_contract.apply_allowed=false", "container_verification_plan_contract.requires_container_logscan=true"},
		"meshclaw_autoheal_container_runbook":           []string{"container_runbook_contract.review_only=true", "container_runbook_contract.apply_allowed=false", "container_runbook_contract.apply_plan_runtime_evidence_required_count"},
		"meshclaw_autoheal_container_runbook_check":     []string{"container_runbook_check_contract.gate_only=true", "container_runbook_check_contract.requires_zero_critical_findings=true"},
		"meshclaw_autoheal_container_rollback_plan":     []string{"container_rollback_plan_contract.rollback_allowed=false", "container_rollback_plan_contract.requires_operator_approval=true"},
		"meshclaw_autoheal_container_completion_plan":   []string{"container_completion_plan_contract.complete_allowed=false", "container_completion_plan_contract.requires_final_evidence=true"},
		"meshclaw_autoheal_container_readiness_summary": []string{"container_readiness_summary_contract.summary_only=true", "container_readiness_summary_contract.requires_approval_gated_executor=true"},
	}
	for tool, wants := range expectedChecks {
		smoke := findSmokePromptByTool(smokePrompts, tool)
		if smoke == nil {
			t.Fatalf("missing rollout smoke prompt for %s: %#v", tool, smokePrompts)
		}
		behavior := stringMapValue(smoke, "expected_behavior")
		for _, want := range wants {
			if !strings.Contains(behavior, want) {
				t.Fatalf("%s rollout smoke should include %q: %q", tool, want, behavior)
			}
		}
		if !strings.Contains(behavior, "no apply") || !strings.Contains(behavior, "live server mutation") {
			t.Fatalf("%s rollout smoke should remain non-mutating: %q", tool, behavior)
		}
	}
}

func TestMCPOpsIntegrationPlanIsPlanOnly(t *testing.T) {
	result, err := callMCPTool("meshclaw_ops_integration_plan", map[string]interface{}{
		"tools":    "prometheus,grafana,loki,ansible,ntfy",
		"goal":     "chat-driven incident triage",
		"scope":    "fleet",
		"readonly": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("payload=%#v", result)
	}
	report, ok := payload["report"].(map[string]interface{})
	if !ok {
		t.Fatalf("report missing: %#v", payload)
	}
	if report["kind"] != "ops_integration_plan" || report["goal"] != "chat-driven incident triage" || report["readonly"] != true {
		t.Fatalf("unexpected ops integration report: %#v", report)
	}
	sources, ok := report["evidence_sources"].([]map[string]interface{})
	if !ok || len(sources) != 5 {
		t.Fatalf("evidence sources should preserve provided tools: %#v", report["evidence_sources"])
	}
	if sources[0]["meshclaw_surface"] != "meshclaw_monitor_check" || sources[2]["meshclaw_surface"] != "meshclaw_analyze_logs" {
		t.Fatalf("ops integration should map metrics/log tools to semantic surfaces: %#v", sources)
	}
	gates, ok := report["approval_gates"].([]string)
	if !ok || !containsString(gates, "secret handles only; no raw API tokens or passwords in model-visible output") {
		t.Fatalf("approval gates should protect credentials: %#v", report["approval_gates"])
	}
	evidenceRecord, ok := payload["evidence"].(evidence.Record)
	if !ok || evidenceRecord.Kind != "ops-integration-plan" {
		t.Fatalf("ops integration plan should store evidence: %#v", payload["evidence"])
	}
}

func TestMCPStorageGuardrailPlanIsPlanOnly(t *testing.T) {
	result, err := callMCPTool("meshclaw_storage_guardrail_plan", map[string]interface{}{
		"node":       "d1",
		"path":       "/srv/data",
		"workload":   "openwebui",
		"risk":       "disk_pressure",
		"backup":     true,
		"retention":  "30d",
		"mount_type": "nas",
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("payload=%#v", result)
	}
	report, ok := payload["report"].(map[string]interface{})
	if !ok {
		t.Fatalf("report missing: %#v", payload)
	}
	if report["kind"] != "storage_guardrail_plan" || report["node"] != "d1" || report["path"] != "/srv/data" || report["backup_required"] != true {
		t.Fatalf("unexpected storage guardrail report: %#v", report)
	}
	options, ok := report["guardrail_options"].([]map[string]interface{})
	if !ok || len(options) < 5 {
		t.Fatalf("guardrail options should include investigate, archive, cleanup, backup, and mount paths: %#v", report["guardrail_options"])
	}
	gates, ok := report["approval_gates"].([]string)
	if !ok || !containsString(gates, "operator approval before deletion, mount edits, volume migration, resize, snapshot writes, or backup writes") {
		t.Fatalf("approval gates should require storage mutation approval: %#v", report["approval_gates"])
	}
	evidenceRecord, ok := payload["evidence"].(evidence.Record)
	if !ok || evidenceRecord.Kind != "storage-guardrail-plan" {
		t.Fatalf("storage guardrail plan should store evidence: %#v", payload["evidence"])
	}
}

func TestMCPCapacityScalePlanIsPlanOnly(t *testing.T) {
	result, err := callMCPTool("meshclaw_capacity_scale_plan", map[string]interface{}{
		"workload":    "vllm",
		"scope":       "gpu",
		"target":      "gpu_free>=1",
		"budget_usd":  float64(25),
		"ttl_hours":   float64(12),
		"constraints": "local-first,avoid-public-ip",
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("payload=%#v", result)
	}
	report, ok := payload["report"].(map[string]interface{})
	if !ok {
		t.Fatalf("report missing: %#v", payload)
	}
	if report["kind"] != "capacity_scale_plan" || report["workload"] != "vllm" || report["scope"] != "gpu" || report["target"] != "gpu_free>=1" {
		t.Fatalf("unexpected capacity scale report: %#v", report)
	}
	options, ok := report["scale_options"].([]map[string]interface{})
	if !ok || len(options) < 4 {
		t.Fatalf("scale options should include rebalance, scale-out, provision, and scale-down paths: %#v", report["scale_options"])
	}
	gates, ok := report["approval_gates"].([]string)
	if !ok || !containsString(gates, "operator approval before provider spend, workload migration, proxy/DNS changes, or container/service mutation") {
		t.Fatalf("approval gates should require mutation approval: %#v", report["approval_gates"])
	}
	evidenceRecord, ok := payload["evidence"].(evidence.Record)
	if !ok || evidenceRecord.Kind != "capacity-scale-plan" {
		t.Fatalf("capacity scale plan should store evidence: %#v", payload["evidence"])
	}
}

func TestMCPServiceRegistryPlanIsPlanOnly(t *testing.T) {
	result, err := callMCPTool("meshclaw_service_registry_plan", map[string]interface{}{
		"service":   "openwebui",
		"scope":     "web",
		"port":      float64(8080),
		"endpoints": "g1:8080,g2:8080",
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("payload=%#v", result)
	}
	report, ok := payload["report"].(map[string]interface{})
	if !ok {
		t.Fatalf("report missing: %#v", payload)
	}
	if report["kind"] != "service_registry_plan" || report["service"] != "openwebui" || report["scope"] != "web" {
		t.Fatalf("unexpected service registry report: %#v", report)
	}
	endpointChecks, ok := report["endpoint_checks"].([]map[string]interface{})
	if !ok || len(endpointChecks) != 2 {
		t.Fatalf("endpoint checks should preserve provided endpoints: %#v", report["endpoint_checks"])
	}
	gates, ok := report["approval_gates"].([]string)
	if !ok || !containsString(gates, "operator approval before proxy, DNS, firewall, or container mutation") {
		t.Fatalf("approval gates should require mutation approval: %#v", report["approval_gates"])
	}
	evidenceRecord, ok := payload["evidence"].(evidence.Record)
	if !ok || evidenceRecord.Kind != "service-registry-plan" {
		t.Fatalf("service registry plan should store evidence: %#v", payload["evidence"])
	}
}

func TestMCPToolNamesUsePortableCharacters(t *testing.T) {
	for _, tool := range mcpTools() {
		if !validMCPToolNameForClients(tool.Name) {
			t.Fatalf("invalid MCP tool name for client compatibility: %q", tool.Name)
		}
	}
}

func TestMCPAnalyzeLogsDocumentsContainerSource(t *testing.T) {
	tools := map[string]mcpTool{}
	for _, tool := range mcpTools() {
		tools[tool.Name] = tool
	}
	tool, ok := tools["meshclaw_analyze_logs"]
	if !ok {
		t.Fatal("missing meshclaw_analyze_logs")
	}
	source := tool.InputSchema.Properties["source"]
	if !strings.Contains(tool.Description, "source=container:<name>") || !strings.Contains(source.Description, "container:<name>") {
		t.Fatalf("analyze logs should document container source: description=%q source=%q", tool.Description, source.Description)
	}
	if !strings.Contains(tool.Description, "unit identity and system/user scope") || !strings.Contains(source.Description, "unit identity before restart planning") {
		t.Fatalf("analyze logs should document systemd unit identity gate: description=%q source=%q", tool.Description, source.Description)
	}
	if !strings.Contains(tool.Description, "unit_candidates") || !strings.Contains(source.Description, "unit_candidates") {
		t.Fatalf("analyze logs should document structured unit candidates: description=%q source=%q", tool.Description, source.Description)
	}
	if !strings.Contains(tool.Description, "autoheal_handoff.runtime_evidence_checklist") || !strings.Contains(source.Description, "autoheal_handoff.runtime_evidence_checklist") {
		t.Fatalf("analyze logs should document container runtime evidence checklist: description=%q source=%q", tool.Description, source.Description)
	}
	recommendation := recommendToolMCP("show container logs for g4 api", "codex")
	if stringMapValue(recommendation, "recommended_tool") != "meshclaw_analyze_logs" || !strings.Contains(stringMapValue(recommendation, "reason"), "container:<name>") {
		t.Fatalf("bad log recommendation: %+v", recommendation)
	}
	if !strings.Contains(stringMapValue(recommendation, "reason"), "autoheal_handoff.runtime_evidence_checklist") {
		t.Fatalf("container log recommendation should mention runtime checklist: %+v", recommendation)
	}
	args, _ := recommendation["arguments"].(map[string]interface{})
	if args["host"] != "g4" || args["source"] != "container:api" {
		t.Fatalf("container log recommendation should infer host/source arguments: %+v", recommendation)
	}

	onRecommendation := recommendToolMCP("show latest docker logs on s2 worker", "codex")
	onArgs, _ := onRecommendation["arguments"].(map[string]interface{})
	if onArgs["host"] != "s2" || onArgs["source"] != "container:worker" {
		t.Fatalf("container log recommendation should infer on-host source arguments: %+v", onRecommendation)
	}

	koreanRecommendation := recommendToolMCP("g4 api 컨테이너 로그 분석해줘", "codex")
	koreanArgs, _ := koreanRecommendation["arguments"].(map[string]interface{})
	if koreanArgs["host"] != "g4" || koreanArgs["source"] != "container:api" {
		t.Fatalf("container log recommendation should infer Korean host/source arguments: %+v", koreanRecommendation)
	}

	systemRecommendation := recommendToolMCP("show recent journal logs for g4", "codex")
	systemArgs, _ := systemRecommendation["arguments"].(map[string]interface{})
	if systemArgs["host"] != "<host>" || systemArgs["source"] != "system" {
		t.Fatalf("system log recommendation should keep default source: %+v", systemRecommendation)
	}

	containerRecommendation := recommendToolMCP("도커 컨테이너 로그 확인해줘", "codex")
	containerArgs, _ := containerRecommendation["arguments"].(map[string]interface{})
	if containerArgs["source"] != "container:<name>" {
		t.Fatalf("ambiguous container log recommendation should use placeholder source: %+v", containerRecommendation)
	}
}

func TestMCPAnalyzeLogsHandoffContractRequiresChecklistReview(t *testing.T) {
	contract := analyzeLogsMCPHandoffContract(&workflow.AutohealHandoff{
		Decision: "plan_only_before_operator_approved_apply",
		RecommendedTools: []string{
			"meshclaw_autoheal_container_apply_plan",
			"meshclaw_autoheal_container_readiness_summary",
		},
		RuntimeEvidenceChecklist: []string{
			"docker inspect api state.status and state.running",
			"docker inspect api state.health when configured",
		},
		StopBefore: []string{
			"container apply-plan unless fresh runtime inspect/status evidence names container:api",
		},
		MustNot: []string{
			"restart or recreate services/containers directly from log output",
		},
	})

	if contract == nil {
		t.Fatal("handoff contract missing")
	}
	if contract["apply_allowed"] != false || contract["mutates_live_servers"] != false || contract["checklist_review_required"] != true {
		t.Fatalf("handoff contract should be plan-only, non-mutating, and checklist-gated: %#v", contract)
	}
	if contract["direct_restart_allowed"] != false || contract["requires_focused_runtime_evidence"] != true {
		t.Fatalf("handoff contract should forbid direct restart and require focused runtime evidence: %#v", contract)
	}
	if contract["evidence_required_count"] != 0 || contract["runtime_evidence_checklist_count"] != 2 || contract["stop_before_count"] != 1 || contract["must_not_count"] != 1 {
		t.Fatalf("handoff contract should expose evidence and gate counts: %#v", contract)
	}
	if contract["next_required_tool"] != "meshclaw_autoheal_container_apply_plan" {
		t.Fatalf("handoff contract should expose next required planning tool: %#v", contract)
	}
	requires, ok := contract["requires"].([]string)
	if !ok || !containsString(requires, "autoheal_handoff.runtime_evidence_checklist reviewed before apply planning") {
		t.Fatalf("handoff contract should require runtime checklist review: %#v", contract["requires"])
	}
	recommendedTools, ok := contract["recommended_tools"].([]string)
	if !ok || !containsString(recommendedTools, "meshclaw_autoheal_container_apply_plan") {
		t.Fatalf("handoff contract should carry recommended planning tools: %#v", contract["recommended_tools"])
	}
	mustNot, ok := contract["must_not"].([]string)
	if !ok || !containsString(mustNot, "restart or recreate services/containers directly from log output") {
		t.Fatalf("handoff contract should preserve no-direct-restart rule: %#v", contract["must_not"])
	}
}

func TestMCPServiceCheckDocumentsLogscanUnitCandidates(t *testing.T) {
	tools := map[string]mcpTool{}
	for _, tool := range mcpTools() {
		tools[tool.Name] = tool
	}
	tool, ok := tools["meshclaw_service_check"]
	if !ok {
		t.Fatal("missing meshclaw_service_check")
	}
	service := tool.InputSchema.Properties["service"]
	if !strings.Contains(tool.Description, "unit_candidates") || !strings.Contains(tool.Description, "exact candidate .service") {
		t.Fatalf("service_check should document logscan unit candidate follow-up: %q", tool.Description)
	}
	if !strings.Contains(service.Description, "log_findings unit_candidates") {
		t.Fatalf("service_check service argument should prefer logscan unit candidates: %q", service.Description)
	}
}

func TestMCPContainerAutohealToolsDocumentRuntimeEvidenceContract(t *testing.T) {
	tools := map[string]mcpTool{}
	for _, tool := range mcpTools() {
		tools[tool.Name] = tool
	}
	for _, name := range []string{
		"meshclaw_autoheal_container_apply_plan",
		"meshclaw_autoheal_container_verification_plan",
		"meshclaw_autoheal_container_runbook",
		"meshclaw_autoheal_container_runbook_check",
		"meshclaw_autoheal_container_rollback_plan",
		"meshclaw_autoheal_container_completion_plan",
	} {
		tool, ok := tools[name]
		if !ok {
			t.Fatalf("missing %s", name)
		}
		if !strings.Contains(tool.Description, "runtime_evidence_required") {
			t.Fatalf("%s should document runtime evidence contract: %q", name, tool.Description)
		}
	}
	applyPlan := tools["meshclaw_autoheal_container_apply_plan"]
	if !strings.Contains(applyPlan.Description, "handoff_contract.apply_allowed=false") || !strings.Contains(applyPlan.InputSchema.Properties["plan_evidence_path"].Description, "handoff_contract.apply_allowed=false") {
		t.Fatalf("container apply-plan should document non-apply logscan handoff contract: description=%q plan_evidence_path=%q", applyPlan.Description, applyPlan.InputSchema.Properties["plan_evidence_path"].Description)
	}
	for _, want := range []string{"container_apply_plan_contract.direct_restart_allowed=false", "requires_focused_runtime_evidence=true", "runtime_evidence_required_count"} {
		if !strings.Contains(applyPlan.Description, want) {
			t.Fatalf("container apply-plan should document runtime gate %q: %q", want, applyPlan.Description)
		}
	}
	if !strings.Contains(applyPlan.InputSchema.Properties["plan_evidence_path"].Description, "container_apply_plan_contract.direct_restart_allowed=false") {
		t.Fatalf("container apply-plan evidence path should document runtime gate: %q", applyPlan.InputSchema.Properties["plan_evidence_path"].Description)
	}
	verificationPlan := tools["meshclaw_autoheal_container_verification_plan"]
	if !strings.Contains(verificationPlan.Description, "apply_plan_runtime_evidence_required_count") {
		t.Fatalf("container verification-plan should document runtime evidence count preservation: %q", verificationPlan.Description)
	}
	if !strings.Contains(verificationPlan.InputSchema.Properties["container_apply_plan_evidence_path"].Description, "container_apply_plan_contract.runtime_evidence_required_count") {
		t.Fatalf("container verification-plan evidence path should document apply-plan runtime count: %q", verificationPlan.InputSchema.Properties["container_apply_plan_evidence_path"].Description)
	}
	runbook := tools["meshclaw_autoheal_container_runbook"]
	if !strings.Contains(runbook.Description, "apply_plan_runtime_evidence_required_count") {
		t.Fatalf("container runbook should document runtime evidence count preservation: %q", runbook.Description)
	}
	if !strings.Contains(runbook.InputSchema.Properties["container_verification_plan_evidence_path"].Description, "container_verification_plan_contract.apply_plan_runtime_evidence_required_count") {
		t.Fatalf("container runbook evidence path should document verification runtime count: %q", runbook.InputSchema.Properties["container_verification_plan_evidence_path"].Description)
	}
	readiness, ok := tools["meshclaw_autoheal_container_readiness_summary"]
	if !ok {
		t.Fatal("missing meshclaw_autoheal_container_readiness_summary")
	}
	if !strings.Contains(readiness.Description, "runtime_evidence_gate") || !strings.Contains(readiness.Description, "runtime_evidence_findings") {
		t.Fatalf("readiness summary should document runtime evidence gate: %q", readiness.Description)
	}
}

func TestMCPAnalyzeLogsLiteDescriptionDocumentsUnitIdentityGate(t *testing.T) {
	description := mcpClaudeLiteDescriptions()["meshclaw_analyze_logs"]
	if !strings.Contains(description, "unit identity before systemd restart planning") {
		t.Fatalf("lite logscan description should document unit identity gate: %q", description)
	}
	if !strings.Contains(description, "source=container:<name>") {
		t.Fatalf("lite logscan description should document container log source: %q", description)
	}
	if !strings.Contains(description, "runtime_evidence_checklist") {
		t.Fatalf("lite logscan description should document runtime evidence checklist: %q", description)
	}
}

func TestMCPClaudeLiteContainerApplyDescriptionDocumentsRuntimeGate(t *testing.T) {
	description := mcpClaudeLiteDescriptions()["meshclaw_autoheal_container_apply_plan"]
	for _, want := range []string{"container_apply_plan_contract.direct_restart_allowed=false", "requires_focused_runtime_evidence=true"} {
		if !strings.Contains(description, want) {
			t.Fatalf("lite container apply-plan description should document runtime gate %q: %q", want, description)
		}
	}
}

func TestMCPAnalyzeLogsClaudeLiteProfileDescriptionKeepsLogscanGuidance(t *testing.T) {
	tools := map[string]mcpTool{}
	for _, tool := range mcpToolsForProfile("claude-lite") {
		tools[tool.Name] = tool
	}
	tool, ok := tools["meshclaw_analyze_logs"]
	if !ok {
		t.Fatal("claude-lite missing meshclaw_analyze_logs")
	}
	if !strings.Contains(tool.Description, "source=container:<name>") || !strings.Contains(tool.Description, "unit identity before systemd restart") || !strings.Contains(tool.Description, "runtime_evidence_checklist") {
		t.Fatalf("claude-lite profile logscan description lost guidance: %q", tool.Description)
	}
}

func TestMCPToolsExposeMissionReadOnlyCore(t *testing.T) {
	tools := mcpTools()
	names := map[string]mcpTool{}
	for _, tool := range tools {
		names[tool.Name] = tool
	}
	for _, name := range []string{"mission_get", "mission_list"} {
		tool, ok := names[name]
		if !ok {
			t.Fatalf("missing %s", name)
		}
		if !strings.Contains(tool.Description, "Read-only") {
			t.Fatalf("%s should be documented as read-only: %s", name, tool.Description)
		}
		if !strings.Contains(tool.Description, "MacBook") {
			t.Fatalf("%s should document MacBook canonical scope: %s", name, tool.Description)
		}
	}
}

func TestMCPMissionGetAndListReadMacBookStore(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".meshclaw", "state", "missions")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	writeMCPTestFile(t, filepath.Join(dir, "active.json"), `{"id":"meshclaw-0.8"}`)
	writeMCPTestFile(t, filepath.Join(dir, "meshclaw-0.8.json"), `{
  "id": "meshclaw-0.8",
  "goal": "Release MeshClaw 0.8",
  "status": "active",
  "next_action": "Fix vssh authentication",
  "tasks": [],
  "artifacts": [],
  "updated_at": "2026-05-31T00:00:00Z"
}`)

	result, err := callMCPTool("mission_get", map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result=%T %#v", result, result)
	}
	if payload["canonical"] != "macbook" || !strings.Contains(fmt.Sprint(payload["scope_note"]), "MacBook") {
		t.Fatalf("payload=%#v", payload)
	}

	result, err = callMCPTool("mission_list", map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	payload, ok = result.(map[string]interface{})
	if !ok {
		t.Fatalf("result=%T %#v", result, result)
	}
	missions, ok := payload["missions"].([]mission.Summary)
	if !ok || len(missions) != 1 || missions[0].ID != "meshclaw-0.8" {
		t.Fatalf("missions=%T %#v", payload["missions"], payload["missions"])
	}
}

func TestMCPSetupSignalMentionsScheduleRunnerDueStatusAndDataDoctor(t *testing.T) {
	var setup *mcpTool
	tools := mcpTools()
	for i := range tools {
		if tools[i].Name == "meshclaw_setup_signal" {
			setup = &tools[i]
			break
		}
	}
	if setup == nil {
		t.Fatal("meshclaw_setup_signal tool missing")
	}
	for _, want := range []string{"schedule-runner", "scheduler due/next-due", "data-doctor"} {
		if !strings.Contains(setup.Description, want) {
			t.Fatalf("setup signal description should mention %q: %q", want, setup.Description)
		}
	}
	repair, ok := setup.InputSchema.Properties["repair"]
	if !ok {
		t.Fatalf("repair property missing: %#v", setup.InputSchema.Properties)
	}
	if !strings.Contains(repair.Description, "schedule-runner") {
		t.Fatalf("repair description should mention schedule-runner: %q", repair.Description)
	}
}

func TestMCPDataDoctor(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, ".meshclaw")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "messenger-targets.json"), []byte(`{"targets":[]}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "schedule-state.json"), []byte(`{"last_run":`), 0600); err != nil {
		t.Fatal(err)
	}
	result, err := callMCPTool("meshclaw_data_doctor", map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected result: %#v", result)
	}
	if payload["report"] == nil || payload["evidence"] == nil {
		t.Fatalf("data doctor should return report and evidence: %#v", payload)
	}
	ref, ok := payload["evidence"].(mcpEvidenceRef)
	if !ok {
		t.Fatalf("evidence ref has unexpected type: %#v", payload["evidence"])
	}
	for _, want := range []string{"state=2", "invalid=1"} {
		if !strings.Contains(ref.Summary, want) {
			t.Fatalf("evidence summary missing %q: %q", want, ref.Summary)
		}
	}
	report, ok := payload["report"].(datadoctor.Report)
	if !ok {
		t.Fatalf("report has unexpected type: %#v", payload["report"])
	}
	if report.OK {
		t.Fatalf("invalid state JSON should make data doctor warn: %#v", report)
	}
	if len(report.StateFiles) != 2 || report.StateFiles[0].ID != "messenger-targets" {
		t.Fatalf("state files should be sorted and returned: %#v", report.StateFiles)
	}
	foundInvalid := false
	for _, file := range report.StateFiles {
		if file.ID == "schedule-state" && file.Exists && !file.OK {
			foundInvalid = true
		}
	}
	if !foundInvalid {
		t.Fatalf("missing invalid schedule-state in MCP data doctor report: %#v", report.StateFiles)
	}
}

func TestMCPScheduleDaemonStatus(t *testing.T) {
	t.Setenv("MESHCLAW_SCHEDULE_STATE", filepath.Join(t.TempDir(), "schedule-state.json"))
	result, err := callMCPTool("meshclaw_daemon_schedule_status", map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok || payload["result"] == nil {
		t.Fatalf("unexpected result: %#v", result)
	}
	status, ok := payload["schedule_status"].(signalSetupSchedule)
	if !ok || status.Jobs == 0 || status.DueCount == 0 || status.NextDueJob == "" {
		t.Fatalf("schedule status summary missing: %#v", payload)
	}
}

func TestMCPScheduleStatusDescriptionMentionsHealthSummary(t *testing.T) {
	tools := mcpTools()
	var scheduleStatus *mcpTool
	for i := range tools {
		if tools[i].Name == "meshclaw_schedule_status" {
			scheduleStatus = &tools[i]
			break
		}
	}
	if scheduleStatus == nil {
		t.Fatal("meshclaw_schedule_status tool missing")
	}
	for _, want := range []string{"status", "due_jobs", "next_due_job"} {
		if !strings.Contains(scheduleStatus.Description, want) {
			t.Fatalf("schedule status description should mention %q: %q", want, scheduleStatus.Description)
		}
	}
}

func TestCapabilityValidateMCP(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "capabilities.json")
	payload := map[string]interface{}{
		"version": 1,
		"capabilities": []map[string]interface{}{
			{
				"id":            "local-model",
				"kind":          "model",
				"provider":      "ollama",
				"host":          "macmini",
				"capabilities":  []string{"chat", "local"},
				"status":        "available",
				"secret_policy": "none",
			},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}

	result, err := callMCPTool("meshclaw_capability_validate", map[string]interface{}{"path": path})
	if err != nil {
		t.Fatal(err)
	}
	switch report := result.(type) {
	case map[string]interface{}:
		if valid, _ := report["valid"].(bool); !valid {
			t.Fatalf("expected valid capability report: %#v", report)
		}
	case capability.ValidationReport:
		if !report.Valid {
			t.Fatalf("expected valid capability report: %#v", report)
		}
	default:
		t.Fatalf("report=%#v", result)
	}
}

func TestCapabilityRecommendMCP(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MESHCLAW_CAPABILITY_FILE", filepath.Join(dir, "missing-capabilities.json"))
	t.Setenv("MESHCLAW_INVENTORY_FILE", filepath.Join(dir, "inventory.json"))
	payload := map[string]interface{}{
		"version": 1,
		"nodes": []map[string]interface{}{
			{"name": "g1", "role": "ollama-worker", "tags": []string{"linux", "gpu"}, "online": true},
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "inventory.json"), data, 0600); err != nil {
		t.Fatal(err)
	}

	result, err := callMCPTool("meshclaw_capability_recommend", map[string]interface{}{"intent": "choose a gpu ollama model worker"})
	if err != nil {
		t.Fatal(err)
	}
	report, ok := result.(capability.RecommendationReport)
	if !ok {
		t.Fatalf("report=%#v", result)
	}
	if report.Class != "model" {
		t.Fatalf("class=%q want model", report.Class)
	}
	if len(report.Candidates) == 0 || report.Candidates[0].Capability.ID != "g1-gpu-compute" {
		t.Fatalf("expected g1-gpu-compute first: %#v", report.Candidates)
	}
}

func TestInventoryOverrideMCP(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("MESHCLAW_INVENTORY_OVERRIDES_FILE", filepath.Join(dir, "overrides.json"))

	result, err := callMCPTool("meshclaw_inventory_override_set", map[string]interface{}{
		"node": "c1",
		"role": "mail-server",
		"tags": "mail,mox",
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("payload=%#v", result)
	}
	nodes, ok := payload["nodes"].([]inventory.Node)
	if !ok || len(nodes) != 1 {
		t.Fatalf("nodes=%#v", payload["nodes"])
	}
	if nodes[0].Name != "c1" || nodes[0].Role != "mail-server" {
		t.Fatalf("unexpected override node: %#v", nodes[0])
	}
	listed, err := callMCPTool("meshclaw_inventory_override_list", map[string]interface{}{})
	if err != nil {
		t.Fatal(err)
	}
	if listed.(map[string]interface{})["path"] == "" {
		t.Fatalf("expected override path: %#v", listed)
	}
	removed, err := callMCPTool("meshclaw_inventory_override_remove", map[string]interface{}{"node": "c1"})
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := removed.(map[string]interface{})["removed"].(bool); !ok {
		t.Fatalf("expected removed=true: %#v", removed)
	}
}

func validMCPToolNameForClients(name string) bool {
	if name == "" {
		return false
	}
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

func containsMapWithID(values []map[string]interface{}, needle string) bool {
	for _, value := range values {
		if value["id"] == needle {
			return true
		}
	}
	return false
}

func containsReadinessGate(values []map[string]interface{}, id, readinessTool, stopBefore string) bool {
	for _, value := range values {
		if value["id"] != id || value["readiness_tool"] != readinessTool || value["mutates_servers"] != false {
			continue
		}
		stops, _ := value["stop_before"].([]string)
		if containsString(stops, stopBefore) {
			return true
		}
	}
	return false
}

func findSmokePromptByTool(values []map[string]interface{}, tool string) map[string]interface{} {
	for _, value := range values {
		if value["tool"] == tool {
			return value
		}
	}
	return nil
}

func findCheckByTool(values []map[string]interface{}, tool string) map[string]interface{} {
	for _, value := range values {
		if value["tool"] == tool {
			return value
		}
	}
	return nil
}

func writeMCPTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
