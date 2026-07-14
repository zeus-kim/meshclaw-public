package router

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/meshclaw/meshclaw/internal/policy"
)

func TestClassifyOpenWebUIStatusExecutesReadOnly(t *testing.T) {
	t.Setenv("MESHCLAW_POLICY_FILE", filepath.Join(t.TempDir(), "missing.json"))
	plan := Classify(Options{Source: "openwebui", Subject: "local-llm", Message: "MeshClaw로 g4 Open WebUI 상태 확인해줘"})
	if plan.Intent != "openwebui_status" {
		t.Fatalf("intent = %q, want openwebui_status", plan.Intent)
	}
	if plan.Lane != "meshclaw:direct" {
		t.Fatalf("lane = %q, want meshclaw:direct", plan.Lane)
	}
	if plan.Target != "g4" {
		t.Fatalf("target = %q, want g4", plan.Target)
	}
	if !plan.Execute || plan.ApprovalRequired || plan.SendToModel {
		t.Fatalf("bad execution flags: execute=%t approval=%t send_to_model=%t", plan.Execute, plan.ApprovalRequired, plan.SendToModel)
	}
	if plan.Decision.Decision != policy.Allow {
		t.Fatalf("decision = %q, want allow", plan.Decision.Decision)
	}
}

func TestClassifyMeshClawHelpExecutesReadOnly(t *testing.T) {
	t.Setenv("MESHCLAW_POLICY_FILE", filepath.Join(t.TempDir(), "missing.json"))
	for _, message := range []string{
		"메시클로 기능",
		"여기서 뭐 할 수 있어?",
		"Open WebUI에서 뭘 할 수 있어?",
		"what can MeshClaw do?",
	} {
		plan := Classify(Options{Source: "openwebui", Subject: "local-llm", Message: message})
		if plan.Intent != "meshclaw_help" {
			t.Fatalf("%q intent = %q, want meshclaw_help", message, plan.Intent)
		}
		if plan.Route != "meshclaw_help" {
			t.Fatalf("%q route = %q, want meshclaw_help", message, plan.Route)
		}
		if plan.SendToModel || !plan.Execute {
			t.Fatalf("%q product help should be handled by MeshClaw, execute=%t send_to_model=%t", message, plan.Execute, plan.SendToModel)
		}
	}
}

func TestClassifyDataDoctorExecutesReadOnly(t *testing.T) {
	t.Setenv("MESHCLAW_POLICY_FILE", filepath.Join(t.TempDir(), "missing.json"))
	for _, message := range []string{
		"쌓이는 데이터가 꼬이지 않는지 봐줘",
		"메시클로 데이터 상태 확인해줘",
		"data doctor",
	} {
		plan := Classify(Options{Source: "signal", Subject: "argos", Message: message})
		if plan.Intent != "data_doctor" {
			t.Fatalf("%q intent = %q, want data_doctor", message, plan.Intent)
		}
		if !plan.Execute || plan.SendToModel || plan.ApprovalRequired {
			t.Fatalf("%q bad flags: execute=%t send_to_model=%t approval=%t", message, plan.Execute, plan.SendToModel, plan.ApprovalRequired)
		}
	}
}

func TestClassifyScheduleStatusExecutesReadOnly(t *testing.T) {
	t.Setenv("MESHCLAW_POLICY_FILE", filepath.Join(t.TempDir(), "missing.json"))
	for _, message := range []string{
		"자동화가 밀렸는지 스케줄 상태 봐줘",
		"schedule-runner 데몬 상태 확인해줘",
		"next due 작업이 뭐야?",
	} {
		plan := Classify(Options{Source: "signal", Subject: "argos", Message: message})
		if plan.Intent != "schedule_status" {
			t.Fatalf("%q intent = %q, want schedule_status", message, plan.Intent)
		}
		if plan.Route != "schedule_status" || plan.Resource != "scheduler" {
			t.Fatalf("%q route/resource = %q/%q", message, plan.Route, plan.Resource)
		}
		if !plan.Execute || plan.SendToModel || plan.ApprovalRequired {
			t.Fatalf("%q bad flags: execute=%t send_to_model=%t approval=%t", message, plan.Execute, plan.SendToModel, plan.ApprovalRequired)
		}
	}
}

func TestClassifyKoreanFleetOperatorReport(t *testing.T) {
	t.Setenv("MESHCLAW_POLICY_FILE", filepath.Join(t.TempDir(), "missing.json"))
	plan := Classify(Options{Source: "openwebui", Subject: "local-llm", Message: "서버 전체를 운영자 관점으로 짧게 보고해줘. 바로 볼 위험과 다음 조치만 알려줘."})
	if plan.Intent != "fleet_status" {
		t.Fatalf("intent = %q, want fleet_status: %+v", plan.Intent, plan)
	}
	if plan.Route != "monitor_check" {
		t.Fatalf("route = %q, want monitor_check", plan.Route)
	}
	if plan.Target != "" {
		t.Fatalf("target = %q, want empty fleet target", plan.Target)
	}
	if !plan.Execute || plan.SendToModel || plan.ApprovalRequired {
		t.Fatalf("bad execution flags: execute=%t send_to_model=%t approval=%t", plan.Execute, plan.SendToModel, plan.ApprovalRequired)
	}
}

func TestClassifyPowerEventRequestUsesOpsDB(t *testing.T) {
	t.Setenv("MESHCLAW_POLICY_FILE", filepath.Join(t.TempDir(), "missing.json"))
	for _, message := range []string{
		"어제 여러 서버가 동시에 꺼진 것 같은데 정전이나 전압 문제였는지 확인해줘",
		"did we have a power outage or simultaneous reboot?",
		"UPS나 순간 전압 문제로 같이 떨어졌는지 봐줘",
	} {
		plan := Classify(Options{Source: "openwebui", Subject: "local-llm", Message: message})
		if plan.Intent != "opsdb_power_events" {
			t.Fatalf("%q intent = %q, want opsdb_power_events: %+v", message, plan.Intent, plan)
		}
		if plan.Route != "opsdb_power_events" || plan.Resource != "opsdb" {
			t.Fatalf("%q route/resource = %q/%q", message, plan.Route, plan.Resource)
		}
		if !plan.Execute || plan.SendToModel || plan.ApprovalRequired {
			t.Fatalf("%q bad flags: execute=%t send_to_model=%t approval=%t", message, plan.Execute, plan.SendToModel, plan.ApprovalRequired)
		}
	}
}

func TestClassifyDeletionRequiresApproval(t *testing.T) {
	t.Setenv("MESHCLAW_POLICY_FILE", filepath.Join(t.TempDir(), "missing.json"))
	for _, message := range []string{
		"d1에서 오래된 체크포인트 삭제해줘",
		"d1 디스크 정리해줘",
		"g4 로그 삭제해줘",
		"remove old docker images on g3",
	} {
		plan := Classify(Options{Source: "signal", Subject: "local-llm", Message: message})
		if plan.Intent != "data_clean" {
			t.Fatalf("%q intent = %q, want data_clean", message, plan.Intent)
		}
		if plan.Lane != "meshclaw:approval" {
			t.Fatalf("%q lane = %q, want meshclaw:approval", message, plan.Lane)
		}
		if !plan.ApprovalRequired || plan.Execute {
			t.Fatalf("%q bad approval flags: execute=%t approval=%t", message, plan.Execute, plan.ApprovalRequired)
		}
		if plan.Decision.Decision != policy.RequireApproval {
			t.Fatalf("%q decision = %q, want require_approval", message, plan.Decision.Decision)
		}
	}
}

func TestClassifyNegatedDeleteDiskInvestigationIsReadOnly(t *testing.T) {
	t.Setenv("MESHCLAW_POLICY_FILE", filepath.Join(t.TempDir(), "missing.json"))
	plan := Classify(Options{Source: "openwebui", Subject: "local-llm", Message: "d1 디스크 조사해줘. 삭제하지 말고 원인만"})
	if plan.Intent != "disk_investigate" {
		t.Fatalf("intent = %q, want disk_investigate: %+v", plan.Intent, plan)
	}
	if plan.Target != "d1" {
		t.Fatalf("target = %q, want d1", plan.Target)
	}
	if !plan.Execute || plan.SendToModel || plan.ApprovalRequired {
		t.Fatalf("bad flags: execute=%t send_to_model=%t approval=%t", plan.Execute, plan.SendToModel, plan.ApprovalRequired)
	}
}

func TestClassifyRestartBeatsReadOnlyOpenWebUIStatus(t *testing.T) {
	t.Setenv("MESHCLAW_POLICY_FILE", filepath.Join(t.TempDir(), "missing.json"))
	plan := Classify(Options{Source: "openwebui", Subject: "local-llm", Message: "g4 openwebui 재시작해줘"})
	if plan.Intent != "restart_service" {
		t.Fatalf("intent = %q, want restart_service: %+v", plan.Intent, plan)
	}
	if plan.Lane != "meshclaw:approval" || !plan.ApprovalRequired || plan.Execute {
		t.Fatalf("bad restart policy flags: lane=%q execute=%t approval=%t", plan.Lane, plan.Execute, plan.ApprovalRequired)
	}
	if plan.Target != "g4" {
		t.Fatalf("target = %q, want g4", plan.Target)
	}
}

func TestClassifyGeneralChatDelegatesToModel(t *testing.T) {
	t.Setenv("MESHCLAW_POLICY_FILE", filepath.Join(t.TempDir(), "missing.json"))
	plan := Classify(Options{Source: "openwebui", Subject: "local-llm", Message: "이순신이 누구야?"})
	if plan.Intent != "general_chat" || !plan.SendToModel || plan.Execute {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if plan.Lane != "model:local_chat" {
		t.Fatalf("lane = %q, want model:local_chat", plan.Lane)
	}
}

func TestClassifyLinuxWorkerNodes(t *testing.T) {
	t.Setenv("MESHCLAW_POLICY_FILE", filepath.Join(t.TempDir(), "missing.json"))
	plan := Classify(Options{Source: "signal", Subject: "argos", Message: "g 계열 워커 상태 점검해줘"})
	if plan.Intent != "linux_worker_nodes" || plan.Route != "linux_worker_nodes" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if !plan.Execute || plan.SendToModel || plan.ApprovalRequired {
		t.Fatalf("bad flags: execute=%t send_to_model=%t approval=%t", plan.Execute, plan.SendToModel, plan.ApprovalRequired)
	}
}

func TestClassifyWorkerRun(t *testing.T) {
	t.Setenv("MESHCLAW_POLICY_FILE", filepath.Join(t.TempDir(), "missing.json"))
	plan := Classify(Options{Source: "signal", Subject: "argos", Message: "g4에 뉴스 리서치 맡겨"})
	if plan.Intent != "worker_run" || plan.Route != "worker_run" || plan.Target != "g4" {
		t.Fatalf("unexpected plan: %+v", plan)
	}
	if !plan.Execute || plan.SendToModel || plan.ApprovalRequired {
		t.Fatalf("bad flags: execute=%t send_to_model=%t approval=%t", plan.Execute, plan.SendToModel, plan.ApprovalRequired)
	}
}

func TestClassifyAssistantWeatherAndNews(t *testing.T) {
	t.Setenv("MESHCLAW_POLICY_FILE", filepath.Join(t.TempDir(), "missing.json"))
	tests := []struct {
		message string
		intent  string
	}{
		{"오늘 날씨 알려줘", "assistant_weather"},
		{"서울 비 와?", "assistant_weather"},
		{"오늘의 주요뉴스 정리해줘", "assistant_news"},
		{"한글로 주여뉴스 알려달라고", "assistant_news"},
		{"뉴스 좀 알려줘", "assistant_news"},
		{"오늘 볼만한 기사 뭐 있어?", "assistant_news"},
		{"latest headlines please", "assistant_news"},
		{"Argos, give me the five most important news items I should read this morning. Keep it short and use today's fresh sources, not old cached news.", "assistant_news"},
		{"출처 보여줘", "assistant_news_sources"},
		{"show source links for the latest briefing", "assistant_news_sources"},
		{"3번 자세히 설명해줘", "assistant_news_detail"},
		{"tell me more about 4", "assistant_news_detail"},
		{"아침 브리핑 해줘", "assistant_morning"},
	}
	for _, tt := range tests {
		plan := Classify(Options{Source: "openwebui", Subject: "local-llm", Message: tt.message})
		if plan.Intent != tt.intent {
			t.Fatalf("message %q intent = %q, want %q", tt.message, plan.Intent, tt.intent)
		}
		if !plan.Execute || plan.SendToModel || plan.ApprovalRequired {
			t.Fatalf("message %q flags execute=%t send_to_model=%t approval=%t", tt.message, plan.Execute, plan.SendToModel, plan.ApprovalRequired)
		}
	}
}

func TestClassifyExplainWithoutNewsReferenceStaysGeneral(t *testing.T) {
	plan := Classify(Options{Source: "openwebui", Subject: "local-llm", Message: "explain options trading"})
	if plan.Intent != "general_chat" || plan.Execute || !plan.SendToModel {
		t.Fatalf("plan = intent=%q execute=%t send_to_model=%t", plan.Intent, plan.Execute, plan.SendToModel)
	}
}

func TestClassifyFleetStatusEnglishPhrases(t *testing.T) {
	t.Setenv("MESHCLAW_POLICY_FILE", filepath.Join(t.TempDir(), "missing.json"))
	tests := []struct {
		message string
		target  string
	}{
		{"server status", ""},
		{"?server status", ""},
		{"node health", ""},
		{"fleet health", ""},
		{"g4 server status", "g4"},
		{"g4 status", "g4"},
		{"check disk usage on d1", "d1"},
	}
	for _, tt := range tests {
		plan := Classify(Options{Source: "openwebui", Subject: "local-llm", Message: tt.message})
		if plan.Intent != "fleet_status" && plan.Intent != "disk_investigate" {
			t.Fatalf("message %q intent = %q, want fleet_status or disk_investigate", tt.message, plan.Intent)
		}
		if tt.target != "" && plan.Target != tt.target {
			t.Fatalf("message %q target = %q, want %q", tt.message, plan.Target, tt.target)
		}
		if !plan.Execute || plan.SendToModel || plan.ApprovalRequired {
			t.Fatalf("message %q flags execute=%t send_to_model=%t approval=%t", tt.message, plan.Execute, plan.SendToModel, plan.ApprovalRequired)
		}
	}
}

func TestClassifyWeatherMetaDoesNotTriggerWeatherTool(t *testing.T) {
	t.Setenv("MESHCLAW_POLICY_FILE", filepath.Join(t.TempDir(), "missing.json"))
	plan := Classify(Options{Source: "openwebui", Subject: "local-llm", Message: "오늘날씨는 모르네"})
	if plan.Intent == "assistant_weather" {
		t.Fatalf("meta complaint incorrectly routed to weather: %+v", plan)
	}
}

func TestClassifyMultilingualPhrases(t *testing.T) {
	t.Setenv("MESHCLAW_POLICY_FILE", filepath.Join(t.TempDir(), "missing.json"))
	tests := []struct {
		message string
		intent  string
	}{
		{"g4 の Open WebUI 状態確認", "openwebui_status"},
		{"请确认 g4 服务器状态", "fleet_status"},
		{"ตรวจสอบสถานะเซิร์ฟเวอร์ g4", "fleet_status"},
		{"g4 trạng thái máy chủ", "fleet_status"},
		{"g4 서비스를 再起動 해줘", "restart_service"},
		{"g4에서 로그 删除 해줘", "data_clean"},
	}
	for _, tt := range tests {
		plan := Classify(Options{Source: "openwebui", Subject: "local-llm", Message: tt.message})
		if plan.Intent != tt.intent {
			t.Fatalf("message %q intent = %q, want %q", tt.message, plan.Intent, tt.intent)
		}
	}
}

func TestClassifyDevOpsNaturalLanguage(t *testing.T) {
	t.Setenv("MESHCLAW_POLICY_FILE", filepath.Join(t.TempDir(), "missing.json"))
	tests := []struct {
		message string
		intent  string
	}{
		{"g4 로그 에러 좀 봐줘", "analyze_logs"},
		{"g4 open-webui 상태 좀 봐줘", "openwebui_status"},
		{"g4 최근 로그에서 이상한 에러가 있는지 사람 말로 정리해줘", "analyze_logs"},
		{"g4 보안 상태 점검해줘", "security_check"},
		{"g4에 뭐가 설치되어 있는지 알려줘", "node_inventory"},
		{"g4 죽은 서비스 있는지 봐줘", "service_audit"},
		{"g4 죽었거나 재시작 중인 서비스를 확인해줘", "service_audit"},
		{"g4 서비스가 재시작 반복하는지 봐줘", "service_audit"},
		{"g4 왜 이렇게 느려?", "process_top"},
		{"g4가 왜 느린지 운영자처럼 짧고 정확하게 봐줘", "process_top"},
		{"g4에서 모델 답이 없어", "openwebui_status"},
		{"g4 Open WebUI 500 에러 나", "openwebui_status"},
		{"모델 답이 없어", "openwebui_status"},
		{"서버들 오늘 괜찮아?", "fleet_status"},
		{"are the servers healthy today?", "fleet_status"},
	}
	for _, tt := range tests {
		plan := Classify(Options{Source: "openwebui", Subject: "local-llm", Message: tt.message})
		if plan.Intent != tt.intent {
			t.Fatalf("message %q intent = %q, want %q", tt.message, plan.Intent, tt.intent)
		}
		if strings.Contains(tt.message, "g4") && plan.Target != "g4" {
			t.Fatalf("message %q target = %q, want g4", tt.message, plan.Target)
		}
		if !plan.Execute || plan.SendToModel || plan.ApprovalRequired {
			t.Fatalf("message %q flags execute=%t send_to_model=%t approval=%t", tt.message, plan.Execute, plan.SendToModel, plan.ApprovalRequired)
		}
	}
}

func TestClassifyOperationalTermFollowUpsAsFleetStatus(t *testing.T) {
	t.Setenv("MESHCLAW_POLICY_FILE", filepath.Join(t.TempDir(), "missing.json"))
	tests := []string{
		"보안만 자세히 알려줘. 공개 포트와 fail2ban 중심으로",
		"fail2ban, docker, 열린 포트, 프로세스까지 봐줘",
		"침입 징후와 방화벽, 크론잡 상태를 종합적으로 알려줘",
		"public ports and fail2ban posture",
	}
	for _, message := range tests {
		plan := Classify(Options{Source: "openwebui", Subject: "local-llm", Message: message})
		if plan.Intent != "fleet_status" {
			t.Fatalf("message %q intent = %q, want fleet_status", message, plan.Intent)
		}
		if !plan.Execute || plan.SendToModel || plan.ApprovalRequired {
			t.Fatalf("message %q flags execute=%t send_to_model=%t approval=%t", message, plan.Execute, plan.SendToModel, plan.ApprovalRequired)
		}
	}
}
