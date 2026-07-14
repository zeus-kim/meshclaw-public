package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meshclaw/meshclaw/internal/monitor"
	"github.com/meshclaw/meshclaw/internal/workflow"
)

func TestFormatOpsVoiceBriefScriptIncludesSecurityOpsAndLogs(t *testing.T) {
	report := opsVoiceBriefReport{
		Generated: time.Now().UTC(),
		Ops: opsBriefReport{
			Monitor: monitorCheckOutput{
				States: map[string]*monitor.NodeState{
					"g4": {Name: "g4", Online: true, Memory: 91, Disk: 72},
					"d1": {Name: "d1", Online: true, Memory: 44, Disk: 88},
				},
				Alerts: []monitor.Alert{{Level: "warning", Node: "g4", Type: "memory", Message: "memory high"}},
			},
			Workloads: agentWorkloadsReport{
				Summary: []agentWorkloadPurposeTotal{{Purpose: "llm_inference", CPUPct: 52.1, MemPct: 66.2}},
				Risks:   []agentWorkloadFleetRisk{{Node: "g4", Severity: "high", Title: "memory pressure", Detail: "ollama dominates memory"}},
			},
		},
		Security: agentSecurityReport{
			Totals: agentSecurityTotals{Nodes: 2, PublicListeners: 3, PublicDockerPorts: 1, FailedUnits: 2, HighFindings: 1, MediumFindings: 2},
			Risks:  []agentSecurityRisk{{Node: "d1", Severity: "high", Category: "network", Title: "public listening ports present", Detail: "3 public listeners"}},
		},
		Logs: []workflow.Report{{
			Name:   "analyze-logs",
			Host:   "g4",
			Status: "findings",
			Findings: []workflow.Finding{{
				Severity: "warning",
				Title:    "Recent warning/error log evidence found",
			}},
		}},
	}

	got := formatOpsVoiceBriefScript(report)
	for _, want := range []string{
		"Argos 운영 보안 브리핑",
		"먼저 볼 항목",
		"참고 집계",
		"공개 리스너 3개",
		"공개 Docker 포트 1개",
		"실패한 systemd unit 2개",
		"LLM 추론 CPU 52.1%",
		"이상 로그",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("script missing %q:\n%s", want, got)
		}
	}
	for _, banned := range []string{"1번", "고르", "출처라고 보내"} {
		if strings.Contains(got, banned) {
			t.Fatalf("script contains interactive/report-room wording %q:\n%s", banned, got)
		}
	}
}

func TestOpsVoiceLogNodesPrioritizesAlertSecurityAndWorkloadNodes(t *testing.T) {
	ops := opsBriefReport{
		Monitor: monitorCheckOutput{
			Alerts: []monitor.Alert{{Level: "warning", Node: "g4", Type: "memory"}},
		},
		Workloads: agentWorkloadsReport{
			Risks: []agentWorkloadFleetRisk{{Node: "d1", Severity: "high", Title: "disk"}},
		},
	}
	security := agentSecurityReport{
		Risks: []agentSecurityRisk{{Node: "s1", Severity: "high", Title: "public port"}},
	}

	got := opsVoiceLogNodes(ops, security, 2)
	if strings.Join(got, ",") != "g4,s1" {
		t.Fatalf("nodes=%v, want g4,s1", got)
	}
}

func TestOpsVoiceWorkloadSentenceAvoidsHugeMulticorePercentages(t *testing.T) {
	got := opsVoiceWorkloadSentence(agentWorkloadsReport{
		Summary: []agentWorkloadPurposeTotal{
			{Purpose: "system_observer", CPUPct: 600, MemPct: 0.1},
			{Purpose: "app_runtime", CPUPct: 277.5, MemPct: 37.1},
		},
	})
	if strings.Contains(got, "600") || strings.Contains(got, "277") {
		t.Fatalf("workload sentence should avoid huge raw CPU percentages: %s", got)
	}
	if !strings.Contains(got, "멀티코어 누적값") {
		t.Fatalf("workload sentence should explain omitted raw percentages: %s", got)
	}
}

func TestFormatOpsVoiceBriefTreatsS1S2AsKnownOffline(t *testing.T) {
	report := opsVoiceBriefReport{
		Ops: opsBriefReport{
			Monitor: monitorCheckOutput{
				States: map[string]*monitor.NodeState{
					"g4": {Name: "g4", Online: true},
					"s1": {Name: "s1", Online: false, Error: "timeout"},
					"s2": {Name: "s2", Online: false, Error: "timeout"},
					"d1": {Name: "d1", Online: true, Disk: 88},
				},
				Alerts: []monitor.Alert{
					{Level: "critical", Node: "s1", Type: "offline", Message: "Node s1 is offline"},
					{Level: "critical", Node: "s2", Type: "offline", Message: "Node s2 is offline"},
					{Level: "warning", Node: "d1", Type: "disk", Message: "Disk usage high: 88.0%"},
				},
			},
		},
	}
	got := formatOpsVoiceBriefScript(report)
	for _, want := range []string{"알려진 오프라인", "의도적으로 꺼둔", "d1"} {
		if !strings.Contains(got, want) {
			t.Fatalf("script missing %q:\n%s", want, got)
		}
	}
	for _, banned := range []string{"s1: 긴급", "s2: 긴급", "s1: 전원", "s2: 전원"} {
		if strings.Contains(got, banned) {
			t.Fatalf("script should not treat known offline as urgent %q:\n%s", banned, got)
		}
	}
}

func TestParseOpsVoiceBriefDefaultsToFastCachedReport(t *testing.T) {
	opts, err := parseOpsVoiceBriefOptions(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !opts.Fast {
		t.Fatal("default ops voice brief should use fast report mode")
	}
	if opts.RefreshAgent {
		t.Fatal("default ops voice brief should not refresh every agent")
	}
	if opts.IncludeLogs {
		t.Fatal("default ops voice brief should not run slow log analysis")
	}
	if opts.LogLimit != 1 {
		t.Fatalf("log limit=%d, want 1", opts.LogLimit)
	}
	if opts.Engine != "edge-tts" {
		t.Fatalf("engine=%q, want edge-tts", opts.Engine)
	}
}

func TestParseOpsVoiceBriefFullRequestsFreshFleetRefresh(t *testing.T) {
	opts, err := parseOpsVoiceBriefOptions([]string{"--full"})
	if err != nil {
		t.Fatal(err)
	}
	if opts.Fast {
		t.Fatal("--full should disable fast report mode")
	}
	if !opts.RefreshAgent {
		t.Fatal("--full should refresh agent snapshots")
	}
	if !opts.IncludeLogs {
		t.Fatal("--full should include log analysis")
	}
}

func TestVSSHNativeTargetForHostPrefersTailscale(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "inventory.json")
	data := []byte(`{"version":1,"nodes":[{"name":"g4","wire_ip":"192.0.2.180","tailscale":"192.0.2.84"}]}`)
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MESHCLAW_INVENTORY_FILE", path)
	t.Setenv("MESHCLAW_INVENTORY_OVERRIDES_FILE", filepath.Join(dir, "overrides.json"))

	if got := vsshNativeTargetForHost("g4"); got != "192.0.2.84" {
		t.Fatalf("target=%q, want tailscale IP", got)
	}
}
