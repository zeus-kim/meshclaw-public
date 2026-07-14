package main

import (
	"strings"
	"testing"

	"github.com/meshclaw/meshclaw/internal/inventory"
	"github.com/meshclaw/meshclaw/internal/nodestate"
)

func TestNaturalWorkloadSummary(t *testing.T) {
	summary := naturalWorkloadSummary(agentWorkloadNodeSummary{
		Node:      "g4",
		Load1:     1.2,
		MemoryPct: 30,
		Workloads: []nodestate.WorkloadState{
			{Purpose: "llm_inference", CPUPct: 45.2, MemPct: 12, Processes: 1},
			{Purpose: "containerized_app", Containers: []string{"open-webui", "n8n"}},
		},
	})
	for _, want := range []string{"LLM inference", "containerized apps"} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary %q missing %q", summary, want)
		}
	}
}

func TestTopResourceProcesses(t *testing.T) {
	got := topResourceProcesses([]nodestate.ProcessState{
		{Command: "low", CPUPct: 1, MemPct: 1},
		{Command: "cpu", CPUPct: 50, MemPct: 1},
		{Command: "mem", CPUPct: 1, MemPct: 70},
	}, 2)
	if len(got) != 2 || got[0].Command != "mem" || got[1].Command != "cpu" {
		t.Fatalf("unexpected top processes: %#v", got)
	}
}

func TestWorkloadPurposeLabel(t *testing.T) {
	if got := workloadPurposeLabel("public_network_service"); got != "public network services" {
		t.Fatalf("label=%q", got)
	}
	if got := workloadPurposeLabel("custom"); got != "custom" {
		t.Fatalf("custom label=%q", got)
	}
}

func TestAgentWorkloadNextActions(t *testing.T) {
	report := agentWorkloadsReport{Risks: []agentWorkloadFleetRisk{{
		Node:     "g4",
		Severity: "high",
		Title:    "SSH appears public and fail2ban is unavailable",
	}}}
	actions := agentWorkloadNextActions(report)
	if len(actions) == 0 || !strings.Contains(actions[0], "fail2ban") || !strings.Contains(actions[0], "g4") {
		t.Fatalf("actions=%#v", actions)
	}
}

func TestInferInventoryRoleTags(t *testing.T) {
	role, tags, reason, confidence := inferInventoryRoleTags(agentWorkloadNodeSummary{
		Node: "g4",
		OS:   "linux",
		GPUs: 1,
		Workloads: []nodestate.WorkloadState{
			{Purpose: "llm_inference"},
			{Purpose: "ai_chat_ui"},
		},
	})
	if role != "llm-chat-worker" || confidence != "high" || !strings.Contains(reason, "LLM inference") {
		t.Fatalf("role=%q confidence=%q reason=%q", role, confidence, reason)
	}
	if !containsString(tags, "gpu") || !containsString(tags, "openwebui") {
		t.Fatalf("tags=%#v", tags)
	}
}

func TestInferInventoryRoleTagsOpenWebUIContainer(t *testing.T) {
	role, tags, _, confidence := inferInventoryRoleTags(agentWorkloadNodeSummary{
		Node: "g4",
		OS:   "linux",
		GPUs: 1,
		Workloads: []nodestate.WorkloadState{
			{Purpose: "llm_inference"},
			{Purpose: "containerized_app", Containers: []string{"open-webui"}},
		},
	})
	if role != "llm-chat-worker" || confidence != "high" || !containsString(tags, "openwebui") {
		t.Fatalf("role=%q tags=%#v confidence=%q", role, tags, confidence)
	}
}

func TestInferInventoryRoleTagsDarwinControllerBeatsFileSync(t *testing.T) {
	role, tags, _, confidence := inferInventoryRoleTags(agentWorkloadNodeSummary{
		Node: "MacBook-Pro",
		OS:   "darwin",
		Workloads: []nodestate.WorkloadState{
			{Purpose: "file_sync"},
			{Purpose: "desktop_ai_or_browser"},
		},
	})
	if role != "desktop-controller" || confidence != "high" || !containsString(tags, "controller") {
		t.Fatalf("role=%q tags=%#v confidence=%q", role, tags, confidence)
	}
}

func TestProposeInventoryDetectsNoChange(t *testing.T) {
	proposal := proposeInventoryFromWorkload(agentWorkloadNodeSummary{
		Node: "g4",
		OS:   "linux",
		Workloads: []nodestate.WorkloadState{
			{Purpose: "llm_inference"},
		},
	}, inventory.Node{Name: "g4", Role: "ollama-worker", Tags: []string{"linux", "llm", "ollama"}})
	if proposal.Changed {
		t.Fatalf("proposal should not change: %#v", proposal)
	}
}

func TestWorkerPlanPrefersGSeriesAndExcludesMacBook(t *testing.T) {
	report := scoreWorkerPlan("Argos background research on g-series workers", []inventory.Node{
		{Name: "m1", Role: "desktop-controller", Tags: []string{"macos", "client"}},
		{Name: "macmini", Role: "controller", Tags: []string{"macos", "client"}},
		{Name: "g2", Role: "headless-worker", Tailscale: "100.64.0.2", User: "operator", Tags: []string{"g-series", "gpu", "llm", "batch", "no-desktop"}},
		{Name: "g4", Role: "llm-chat-worker", Tailscale: "100.64.0.4", User: "operator", Tags: []string{"g-series", "gpu", "llm", "ollama", "openwebui", "no-desktop"}},
	})
	if len(report.Candidates) == 0 {
		t.Fatalf("expected g-series candidates: %#v", report)
	}
	if report.Candidates[0].Host != "g4" {
		t.Fatalf("top candidate=%q, want g4: %#v", report.Candidates[0].Host, report.Candidates)
	}
	for _, candidate := range report.Candidates {
		if candidate.Host == "m1" || candidate.Host == "macmini" {
			t.Fatalf("desktop/controller candidate should be excluded: %#v", report.Candidates)
		}
	}
}
