package main

import (
	"strings"
	"testing"
	"time"

	"github.com/meshclaw/meshclaw/internal/datadoctor"
	"github.com/meshclaw/meshclaw/internal/evidence"
	"github.com/meshclaw/meshclaw/internal/monitor"
	"github.com/meshclaw/meshclaw/internal/nodestate"
	"github.com/meshclaw/meshclaw/internal/router"
	"github.com/meshclaw/meshclaw/internal/runtime"
	"github.com/meshclaw/meshclaw/internal/workers"
	"github.com/meshclaw/meshclaw/internal/workflow"
)

func TestRouterReplyLanguage(t *testing.T) {
	tests := []struct {
		message string
		want    string
	}{
		{"Can you check g4 services?", "en"},
		{"g4 서비스 확인해줘", "ko"},
		{"g4 Open WebUI 상태", "ko"},
	}
	for _, tt := range tests {
		if got := routerReplyLanguage(tt.message); got != tt.want {
			t.Fatalf("routerReplyLanguage(%q) = %q, want %q", tt.message, got, tt.want)
		}
	}
}

func TestParseRouterLLMClassificationClearsFleetWideTarget(t *testing.T) {
	cls, ok := parseRouterLLMClassification(`{"intent":"fleet_status","target":"all","confidence":0.91,"reason":"fleet-wide report"}`)
	if !ok {
		t.Fatal("classification was rejected")
	}
	if cls.Target != "" {
		t.Fatalf("target = %q, want empty fleet-wide target", cls.Target)
	}
}

func TestFormatRouterScheduleStatus(t *testing.T) {
	plan := router.Plan{Intent: "schedule_status", Message: "자동화가 밀렸는지 봐줘"}
	got := formatRouterExecutedDisplay(plan, map[string]interface{}{"result": map[string]interface{}{"result": map[string]interface{}{
		"status": map[string]interface{}{
			"status":              "healthy",
			"due_count":           0,
			"due_jobs":            []string{},
			"next_due_job":        "mail-watch",
			"next_due_in_seconds": 300,
		},
		"daemon": daemonActionResult{Service: "schedule-runner", Installed: true, Running: false, LastStatus: "0"},
	}}})
	for _, want := range []string{"Argos 자동화 상태", "status: healthy", "due_count: 0", "schedule_runner: installed=true running=false last_status=0", "next_due_job: mail-watch", "밀린 자동화 작업은 없습니다"} {
		if !strings.Contains(got, want) {
			t.Fatalf("schedule status output missing %q:\n%s", want, got)
		}
	}
}

func TestFormatRouterDataDoctorShowsStateJSONHealth(t *testing.T) {
	plan := router.Plan{Intent: "data_doctor", Message: "데이터 꼬이는지 봐줘"}
	got := formatRouterExecutedDisplay(plan, map[string]interface{}{"result": map[string]interface{}{"result": map[string]interface{}{
		"report": datadoctor.Report{
			OK: true,
			Locations: []datadoctor.Location{{
				ID: "public_argos", Exists: true, Files: 2, Bytes: 2048, AutoCleaned: true,
			}},
			Evidence: datadoctor.EvidenceState{Files: 3, Bytes: 4096, ArchiveOnly: true},
			StateFiles: []datadoctor.StateFile{
				{ID: "messenger-targets", Exists: true, OK: true},
				{ID: "schedule-state", Exists: true, OK: true},
			},
		},
	}}})
	for _, want := range []string{"MeshClaw 데이터 상태: ok=true", "public_argos: 2개", "evidence: 3개", "state JSON: 2개 검사 / 깨진 파일 0개", "현재 경고는 없습니다"} {
		if !strings.Contains(got, want) {
			t.Fatalf("data doctor output missing %q:\n%s", want, got)
		}
	}
}

func TestFormatRouterDataDoctorListsInvalidStateJSON(t *testing.T) {
	plan := router.Plan{Intent: "data_doctor", Message: "데이터 상태 봐줘"}
	got := formatRouterExecutedDisplay(plan, map[string]interface{}{"result": map[string]interface{}{"result": map[string]interface{}{
		"report": datadoctor.Report{
			OK:       false,
			Evidence: datadoctor.EvidenceState{ArchiveOnly: true},
			StateFiles: []datadoctor.StateFile{
				{ID: "messenger-targets", Exists: true, OK: true},
				{ID: "schedule-state", Exists: true, OK: false, Error: "invalid json"},
			},
			Warnings: []string{"state file is not valid JSON: schedule-state"},
		},
	}}})
	for _, want := range []string{"state JSON: 2개 검사 / 깨진 파일 1개", "깨진 파일: schedule-state", "주의:", "state file is not valid JSON"} {
		if !strings.Contains(got, want) {
			t.Fatalf("data doctor invalid-state output missing %q:\n%s", want, got)
		}
	}
}

func TestFormatRouterServiceAuditReportEnglish(t *testing.T) {
	plan := router.Plan{Message: "Can you check g4 for failed or restarting services?", Target: "g4", Intent: "service_audit"}
	report := workflow.Report{
		Host:   "g4",
		Status: "warning",
		Findings: []workflow.Finding{{
			Title:    "systemd service findings",
			Evidence: "server-agent.service loaded failed failed\nserver-agent.service loaded activating auto-restart\nserver-agent.service repeated 12 times",
			Next:     "service-check g4 server-agent",
		}},
	}
	got := formatRouterServiceAuditReport(plan, map[string]interface{}{}, report, "ignored")
	for _, want := range []string{"I checked g4.", "failed unit", "restarting/activating", "Related services", "service-check"} {
		if !strings.Contains(got, want) {
			t.Fatalf("English service audit output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "확인했습니다") {
		t.Fatalf("English service audit output contains Korean text:\n%s", got)
	}
}

func TestSplitRouterCompositeMessage(t *testing.T) {
	got := splitRouterBatchMessages("g4 상태랑 보안 로그 같이 봐줘")
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3: %#v", len(got), got)
	}
	for _, want := range []string{"g4 현재 상태", "g4 최근 로그", "g4 보안 상태"} {
		found := false
		for _, item := range got {
			if strings.Contains(item, want) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing composite item containing %q: %#v", want, got)
		}
	}
}

func TestSplitRouterVagueTroubleMessage(t *testing.T) {
	for _, message := range []string{"g4가 이상해", "g4 is acting weird"} {
		got := splitRouterBatchMessages(message)
		if len(got) != 4 {
			t.Fatalf("%q len = %d, want 4: %#v", message, len(got), got)
		}
		for _, want := range []string{"g4 현재 상태", "g4 최근 로그", "g4 실패했거나", "g4 상위 프로세스"} {
			found := false
			for _, item := range got {
				if strings.Contains(item, want) {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("%q missing vague trouble item containing %q: %#v", message, want, got)
			}
		}
	}
}

func TestRouterFleetStateLineKorean(t *testing.T) {
	state := &monitor.NodeState{Name: "g4", IP: "192.0.2.84", Online: true, CPU: 1.42, Memory: 29, Disk: 30}
	got := routerFleetStateLine(state, "ko")
	for _, want := range []string{"g4: 온라인", "ip=192.0.2.84", "load=1.42", "mem=29.0%", "disk=30.0%"} {
		if !strings.Contains(got, want) {
			t.Fatalf("routerFleetStateLine missing %q: %s", want, got)
		}
	}
}

func TestShouldDefaultOpenWebUIToLocalHost(t *testing.T) {
	local := router.Plan{Source: "openwebui", Intent: "openwebui_status", Message: "모델 답이 없어"}
	if !shouldDefaultOpenWebUIToLocalHost(local) {
		t.Fatalf("local Open WebUI trouble should default to current host")
	}
	fleet := router.Plan{Source: "openwebui", Intent: "openwebui_status", Message: "모든 Open WebUI 상태 확인해줘"}
	if shouldDefaultOpenWebUIToLocalHost(fleet) {
		t.Fatalf("explicit fleet Open WebUI scan should not default to current host")
	}
	targeted := router.Plan{Source: "openwebui", Intent: "openwebui_status", Target: "g4", Message: "g4 모델 답이 없어"}
	if shouldDefaultOpenWebUIToLocalHost(targeted) {
		t.Fatalf("explicit target should not be replaced")
	}
}

func TestRouterBatchSectionTitleProcessTop(t *testing.T) {
	item := routerBatchItem{Plan: router.Plan{Intent: "process_top"}}
	if got := routerBatchSectionTitle(item); got != "프로세스" {
		t.Fatalf("routerBatchSectionTitle(process_top) = %q, want 프로세스", got)
	}
}

func TestFormatRouterProcessTopReportSummarizesEvidence(t *testing.T) {
	plan := router.Plan{Intent: "process_top", Target: "g4", Message: "g4가 왜 느린지 운영자처럼 봐줘"}
	report := workflow.Report{
		Name:   "process-top",
		Host:   "g4",
		Status: "ok",
		Findings: []workflow.Finding{{
			Severity: "info",
			Title:    "Top process snapshot collected",
			Evidence: "---memory---\nPID PPID USER COMMAND %MEM %CPU RSS\n10 1 operator ollama 12.3 4.5 123456\n---cpu---\nPID PPID USER COMMAND %MEM %CPU RSS\n20 1 operator python3 0.6 16.2 587860",
			Next:     "Compare top memory/CPU processes with monitor-check.",
		}},
	}
	got := formatRouterProcessTopReport(plan, map[string]interface{}{}, report)
	for _, want := range []string{"g4의 느림 원인", "판단:", "CPU를 쓰는 쪽", "python3", "메모리를 쓰는 쪽", "ollama"} {
		if !strings.Contains(got, want) {
			t.Fatalf("process top summary missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "```text") {
		t.Fatalf("process top summary should not dump a raw code block:\n%s", got)
	}
}

func TestFormatWorkerNewsRunDisplayIsReadable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	got := formatWorkerRunDisplay(map[string]interface{}{
		"job": workers.LinuxWorkerJobResult{
			OK:         true,
			Worker:     workers.LinuxWorkerNode{ID: "g4"},
			Task:       "AI 뉴스 리서치",
			DurationMS: 1293,
			Stdout: strings.Join([]string{
				"meshclaw-worker-news-ok",
				"task=AI 뉴스 리서치",
				"host=g4",
				"items=12",
				"",
				"빠른 리서치 결과:",
				"1. 첫 번째 뉴스 - 예시신문",
				"   google-tech-focus | Tue, 26 May 2026 00:00:00 GMT",
				"   https://news.google.com/rss/articles/example",
			}, "\n"),
		},
	})
	for _, want := range []string{"g4에서 빠르게 찾아봤습니다.", "확인한 기사: 12개", "바로 볼 만한 것만 추렸습니다:", "첫 번째 뉴스", "AI/기술 뉴스", "원문 보기: http://"} {
		if !strings.Contains(got, want) {
			t.Fatalf("worker news display missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "```text") || strings.Contains(got, "먼저 볼 만한 항목") || strings.Contains(got, "https://news.google.com/rss/articles/example") || strings.Contains(got, "원문: http://") {
		t.Fatalf("worker news display should be concise, got:\n%s", got)
	}
}

func TestFormatRouterAlertExplanationIsHumanReadable(t *testing.T) {
	payload := map[string]interface{}{
		"states": map[string]*monitor.NodeState{
			"d1": {Name: "d1", Online: true, Disk: 88},
			"s1": {Name: "s1", Online: false, Error: "timeout"},
		},
		"alerts": []monitor.Alert{
			{Level: "critical", Node: "s1", Type: "offline", Message: "Node s1 is offline"},
			{Level: "warning", Node: "d1", Type: "disk", Message: "Disk usage high: 88.0%"},
		},
	}
	got := formatRouterAlertExplanation(payload, "ko")
	for _, want := range []string{"운영 경고", "먼저 봐야 할 것", "긴급: s1", "전원, Tailscale, vsshd", "주의: d1", "88.0%", "다음 조치"} {
		if !strings.Contains(got, want) {
			t.Fatalf("alert explanation missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "[critical]") || strings.Contains(got, "Node s1 is offline") {
		t.Fatalf("alert explanation should not be a raw alert dump:\n%s", got)
	}
}

func TestFormatTargetAIViewKoreanWarnsWhenGPUBusy(t *testing.T) {
	report := agentWorkloadsReport{Nodes: []agentWorkloadNodeSummary{{
		Node:         "g4",
		Load1:        1.1,
		MemoryPct:    35,
		DiskPct:      31,
		GPUs:         1,
		GPUUsagePct:  93,
		GPUMemoryPct: 82,
		Inventory: nodestate.InventoryHint{
			PrimaryRole: "llm-worker",
		},
		Capacity: nodestate.CapacityState{
			OverallPressure: "medium",
			GPUHeadroom:     "low",
			SuggestedUse:    []string{"LLM inference"},
		},
		AIView: nodestate.AIView{
			PlainSummary:      "g4 is classified as llm-worker; overall pressure is medium.",
			WhatThisNodeIs:    "This node appears to be useful for local AI model inference.",
			WhatIsRunning:     []string{"llm inference using 1 process(es) at about 93.0% CPU and 82.0% memory"},
			CanUseFor:         []string{"LLM inference"},
			ShouldAvoid:       []string{"memory-heavy LLM inference"},
			ImmediateConcerns: []string{"No medium or high concern was detected in this snapshot."},
		},
	}},
	}
	got := formatRouterTargetAIView("g4", report, "ko")
	for _, want := range []string{"g4 노드 판독", "LLM 추론 노드", "GPU는 1개", "이미 바쁩니다", "기존 Ollama/vLLM 작업"} {
		if !strings.Contains(got, want) {
			t.Fatalf("target AI view missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "모델 작업 자원 자체는 가능합니다") {
		t.Fatalf("busy GPU should not be reported as plainly available:\n%s", got)
	}
}

func TestFormatRouterDiskCardKoreanIsOpenWebUIFriendly(t *testing.T) {
	plan := router.Plan{Source: "openwebui", Target: "d1", Message: "d1 디스크 조사해줘. 삭제하지 말고 원인만"}
	payload := map[string]interface{}{}
	run := runtime.Evidence{
		Success:    true,
		Transport:  "vssh-native",
		DurationMs: 2311,
		Stdout: strings.Join([]string{
			"Filesystem      Size  Used Avail Use% Mounted on",
			"/dev/nvme0n1p2  937G  780G  110G  88% /",
			"15G /var",
			"51G /root",
			"79G /usr",
			"629G /home",
			"780G /",
		}, "\n"),
	}
	got := formatRouterDiskCardKO(plan, payload, run, "/")
	for _, want := range []string{"MeshClaw 디스크 요약: d1", "마운트:", "큰 위치", "위치 1: 780G /", "삭제는 하지 않았습니다"} {
		if !strings.Contains(got, want) {
			t.Fatalf("disk card missing %q:\n%s", want, got)
		}
	}
	for _, bad := range []string{"```", "- ", "결과:", "Filesystem      Size"} {
		if strings.Contains(got, bad) {
			t.Fatalf("disk card should stay compact and plain; found %q:\n%s", bad, got)
		}
	}
}

func TestFormatRouterDiskCardKoreanFailureStillExplainsNoDeletion(t *testing.T) {
	plan := router.Plan{Source: "openwebui", Target: "d1", Message: "d1 디스크 조사해줘. 삭제하지 말고 원인만"}
	payload := map[string]interface{}{}
	run := runtime.Evidence{
		Success:    false,
		Transport:  "vssh-native",
		DurationMs: 5003,
		Stderr:     "vssh timed out after 5s",
		Attempts:   []runtime.Attempt{{Transport: "vssh-native", Target: "d1", Error: "timeout"}},
	}
	got := formatRouterDiskCardKO(plan, payload, run, "/")
	for _, want := range []string{"MeshClaw 디스크 요약: d1", "삭제는 하지 않았습니다", "Codex/Claude MCP", "오류:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("disk failure card missing %q:\n%s", want, got)
		}
	}
	for _, bad := range []string{"```", "Filesystem      Size"} {
		if strings.Contains(got, bad) {
			t.Fatalf("disk failure card should stay compact and plain; found %q:\n%s", bad, got)
		}
	}
}

func TestFormatRouterEvidenceListCardIsOpenWebUIFriendly(t *testing.T) {
	now := time.Date(2026, 5, 26, 12, 30, 0, 0, time.Local)
	records := []evidence.Summary{
		{ID: "20260526T033000Z-123-monitor-check-fleet", Time: now, Kind: "monitor-check", Host: "fleet", Summary: "nodes=17 alerts=3"},
		{ID: "20260526T032900Z-456-disk-investigate-d1", Time: now.Add(-time.Minute), Kind: "disk-investigate", Host: "d1", Summary: "/"},
	}
	got := formatRouterEvidenceListCard(router.Plan{Source: "openwebui", Message: "최근 evidence 보여줘"}, records)
	for _, want := range []string{"MeshClaw evidence 요약", "최근 기록:", "기록 1:", "monitor-check", "사용법:", "evidence: 20260526T033000Z-123-monitor-check-fleet"} {
		if !strings.Contains(got, want) {
			t.Fatalf("evidence card missing %q:\n%s", want, got)
		}
	}
	for _, bad := range []string{"\n- ", "```", "\"result\":", "\"display\":"} {
		if strings.Contains(got, bad) {
			t.Fatalf("evidence card should stay compact/plain; found %q:\n%s", bad, got)
		}
	}
}

func TestFormatTargetAIViewKoreanAvoidsMarkdownLists(t *testing.T) {
	report := agentWorkloadsReport{Nodes: []agentWorkloadNodeSummary{{
		Node:      "g4",
		Load1:     0.8,
		MemoryPct: 38,
		DiskPct:   31,
		GPUs:      1,
		Inventory: nodestate.InventoryHint{
			PrimaryRole: "gpu-worker",
		},
		Capacity: nodestate.CapacityState{
			OverallPressure: "low",
		},
		AIView: nodestate.AIView{
			PlainSummary:      "g4 is classified as gpu-worker; overall pressure is low.",
			WhatThisNodeIs:    "This node has GPU resources and can run GPU work when load allows.",
			WhatIsRunning:     []string{"llm inference using 2 processes", "meshclaw monitoring"},
			CanUseFor:         []string{"read-only monitoring"},
			ShouldAvoid:       []string{"No clear avoid workload in this snapshot."},
			ImmediateConcerns: []string{"public listening ports present"},
			RecommendedActions: []string{
				"Review public listeners before heavy work.",
			},
		},
	}},
	}
	got := formatRouterTargetAIView("g4", report, "ko")
	for _, want := range []string{"g4 노드 판독", "현재 하는 일 1:", "맡기기 좋은 작업 1:", "바로 볼 위험 1:", "추천 조치 1:"} {
		if !strings.Contains(got, want) {
			t.Fatalf("target AI view missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "\n- ") || strings.Contains(got, "```") {
		t.Fatalf("OpenWebUI target AI view should avoid markdown lists/code blocks:\n%s", got)
	}
}

func TestFormatRouterFleetOpsCardGuidesDeepWorkToMCP(t *testing.T) {
	got := formatRouterFleetOpsCardKO(
		map[string]*monitor.NodeState{"g4": {Name: "g4", Online: true}},
		nil,
		agentWorkloadsReport{},
		agentSecurityReport{},
		agentChangesReport{},
		agentPullReport{},
		map[string]interface{}{"evidence": "ev-test"},
		1,
	)
	for _, want := range []string{"MeshClaw 운영 요약", "Codex/Claude MCP", "evidence"} {
		if !strings.Contains(got, want) {
			t.Fatalf("fleet card missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "\n- ") || strings.Contains(got, "```") {
		t.Fatalf("OpenWebUI fleet card should avoid markdown lists/code blocks because Safari repeats list text in accessibility output:\n%s", got)
	}
}

func TestOpenWebUILongFleetPromptStaysCompact(t *testing.T) {
	payload := map[string]interface{}{
		"states": map[string]*monitor.NodeState{
			"g4": {Name: "g4", Online: true, Memory: 38, Disk: 31},
			"d1": {Name: "d1", Online: true, Disk: 88},
			"s2": {Name: "s2", Online: false, Error: "timeout"},
		},
		"alerts": []monitor.Alert{
			{Level: "critical", Node: "s2", Type: "offline", Message: "Node s2 is offline"},
			{Level: "warning", Node: "d1", Type: "disk", Message: "Disk usage high: 88.0%"},
		},
		"workloads": agentWorkloadsReport{},
		"security": agentSecurityReport{
			Totals: agentSecurityTotals{Nodes: 3, PublicListeners: 12, PublicDockerPorts: 2, FailedUnits: 1, HighFindings: 1, MediumFindings: 2},
		},
		"changes":       agentChangesReport{},
		"agent_refresh": agentPullReport{OK: 2, Failed: 1},
		"evidence":      "ev-long-openwebui",
	}
	plan := router.Plan{
		Source:  "openwebui",
		Intent:  "fleet_status",
		Message: "서버 전체를 운영자 관점으로 종합 보고해줘. 각 노드가 무슨 역할인지, 지금 어떤 작업을 하고 있는지, 새 작업을 맡겨도 되는지, 바로 봐야 할 위험을 한국어로 설명해줘.",
	}
	got := formatRouterFleetOpsReport(plan, payload, "ko")
	for _, want := range []string{"MeshClaw 운영 요약", "바로 볼 위험", "다음 조치", "Codex/Claude MCP", "evidence"} {
		if !strings.Contains(got, want) {
			t.Fatalf("long OpenWebUI fleet prompt missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "노드별 판독:") || strings.Contains(got, "\n- ") || strings.Contains(got, "```") {
		t.Fatalf("long OpenWebUI fleet prompt must stay card-shaped, got:\n%s", got)
	}
	if len(got) > 1800 {
		t.Fatalf("long OpenWebUI fleet prompt card too large: %d bytes\n%s", len(got), got)
	}
}

func TestFormatRouterFleetSecurityCardGuidesDeepWorkToMCP(t *testing.T) {
	got := formatRouterFleetSecurityCardKO(agentSecurityReport{
		Totals: agentSecurityTotals{Nodes: 1, PublicListeners: 3, HighFindings: 1},
		Risks: []agentSecurityRisk{{
			Node:     "g4",
			Severity: "high",
			Title:    "SSH exposed without confirmed fail2ban",
		}},
	}, map[string]interface{}{"evidence": "ev-security"})
	for _, want := range []string{"MeshClaw 보안 요약", "빠른 보안 요약", "Codex/Claude MCP", "evidence"} {
		if !strings.Contains(got, want) {
			t.Fatalf("security card missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "\n- ") || strings.Contains(got, "```") {
		t.Fatalf("OpenWebUI security card should avoid markdown lists/code blocks because Safari repeats list text in accessibility output:\n%s", got)
	}
}

func TestFormatRouterMeshClawHelpShowsSurfaceBoundaries(t *testing.T) {
	got := formatRouterMeshClawHelp(router.Plan{Source: "openwebui", Message: "메시클로 기능을 설명해줘"}, map[string]interface{}{})
	for _, want := range []string{"MeshClaw는 챗봇이 아닙니다", "보안만 자세히", "사용 기준", "Open WebUI Router", "Codex/Claude MCP", "Signal/Argos"} {
		if !strings.Contains(got, want) {
			t.Fatalf("help card missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "통합 운영 플랫폼") || strings.Contains(got, "슈퍼컴퓨터") {
		t.Fatalf("help card should not use speculative local-model wording:\n%s", got)
	}
}
