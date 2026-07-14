package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/meshclaw/meshclaw/internal/aichat"
	"github.com/meshclaw/meshclaw/internal/datadoctor"
	"github.com/meshclaw/meshclaw/internal/fileorg"
	"github.com/meshclaw/meshclaw/internal/legacyskills"
	"github.com/meshclaw/meshclaw/internal/messenger"
)

func TestMCPToolsExposeCatalog(t *testing.T) {
	tools := map[string]mcpTool{}
	for _, tool := range mcpTools() {
		tools[tool.Name] = tool
	}
	tool, ok := tools["meshclaw_mcp_catalog"]
	if !ok {
		t.Fatal("missing meshclaw_mcp_catalog")
	}
	if tool.Description == "" {
		t.Fatal("meshclaw_mcp_catalog has empty description")
	}
	if profile, ok := tool.InputSchema.Properties["profile"]; !ok || profile.Default != "all" {
		t.Fatalf("profile schema = %#v", tool.InputSchema.Properties["profile"])
	}
}

func TestMCPAutohealContainerToolsDocumentLogscanEvidence(t *testing.T) {
	tools := map[string]mcpTool{}
	for _, tool := range mcpTools() {
		tools[tool.Name] = tool
	}
	for _, name := range []string{
		"meshclaw_autoheal_container_verification_plan",
		"meshclaw_autoheal_container_completion_plan",
	} {
		tool, ok := tools[name]
		if !ok {
			t.Fatalf("missing %s", name)
		}
		if !strings.Contains(tool.Description, "container-logscan") {
			t.Fatalf("%s should document container-logscan evidence: %q", name, tool.Description)
		}
	}
}

func TestMCPAutohealContainerToolsExposeFullPlanningChain(t *testing.T) {
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
		"meshclaw_autoheal_container_readiness_summary",
	} {
		tool, ok := tools[name]
		if !ok {
			t.Fatalf("missing container self-heal planning tool %s", name)
		}
		if !strings.Contains(tool.Description, "executes docker commands") {
			t.Fatalf("%s should document that it never executes docker commands: %q", name, tool.Description)
		}
	}
}

func TestMCPClaudeLiteProfileReducesToolSurface(t *testing.T) {
	full := mcpTools()
	lite := mcpToolsForProfile("claude-lite")
	if len(lite) == 0 {
		t.Fatal("claude-lite returned no tools")
	}
	if len(lite) >= len(full) {
		t.Fatalf("claude-lite should reduce tools: lite=%d full=%d", len(lite), len(full))
	}
	if len(lite) > 84 {
		t.Fatalf("claude-lite should stay compact, got %d tools", len(lite))
	}
	seen := map[string]mcpTool{}
	for _, tool := range lite {
		seen[tool.Name] = tool
		if len(tool.Description) > 180 {
			t.Fatalf("claude-lite description too long for %s: %q", tool.Name, tool.Description)
		}
	}
	for _, name := range []string{
		"meshclaw_local_assistant",
		"meshclaw_mcp_catalog",
		"meshclaw_tool_recommend",
		"meshclaw_mail_summarize",
		"meshclaw_document_create",
		"meshclaw_calendar_create_event",
		"meshclaw_maps_proof",
		"meshclaw_scheduled_delivery_plan",
		"meshclaw_mcp_surface",
		"meshclaw_mcp_rollout_plan",
		"meshclaw_mcp_smoke_test_plan",
		"meshclaw_service_registry_plan",
		"meshclaw_capacity_scale_plan",
		"meshclaw_storage_guardrail_plan",
		"meshclaw_ops_integration_plan",
		"meshclaw_data_archive_plan",
		"meshclaw_argos_ask",
		"meshclaw_util_weather",
		"meshclaw_util_convert",
	} {
		if _, ok := seen[name]; !ok {
			t.Fatalf("claude-lite missing core tool %s", name)
		}
	}
	for _, name := range []string{
		"meshclaw_fleet_scan",
		"meshclaw_inventory_override_set",
		"meshclaw_mail_delete",
		"meshclaw_signal_rooms_cleanup",
	} {
		if _, ok := seen[name]; ok {
			t.Fatalf("claude-lite exposed advanced/destructive tool %s", name)
		}
	}
}

func TestMCPLocalLiteProfileIsSmallForLocalModels(t *testing.T) {
	claudeLite := mcpToolsForProfile("claude-lite")
	localLite := mcpToolsForProfile("local-lite")
	if len(localLite) < 15 || len(localLite) > 25 {
		t.Fatalf("local-lite should expose 15-25 tools, got %d", len(localLite))
	}
	if len(localLite) >= len(claudeLite) {
		t.Fatalf("local-lite should be smaller than claude-lite: local=%d claude=%d", len(localLite), len(claudeLite))
	}
	raw, err := json.Marshal(map[string]interface{}{"tools": localLite})
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) > 14000 {
		t.Fatalf("local-lite tools/list should stay compact, got %d bytes", len(raw))
	}
	seen := map[string]mcpTool{}
	for _, tool := range localLite {
		seen[tool.Name] = tool
		if len(tool.Description) > 120 {
			t.Fatalf("local-lite description too long for %s: %q", tool.Name, tool.Description)
		}
	}
	for _, name := range []string{
		"meshclaw_local_assistant",
		"meshclaw_mcp_catalog",
		"meshclaw_tool_recommend",
		"meshclaw_mail_summarize",
		"meshclaw_calendar_list",
		"meshclaw_reminder_create",
		"meshclaw_document_create",
		"meshclaw_data_archive_plan",
		"meshclaw_argos_ask",
	} {
		if _, ok := seen[name]; !ok {
			t.Fatalf("local-lite missing core tool %s", name)
		}
	}
	for _, name := range []string{
		"meshclaw_browser_fetch",
		"meshclaw_visible_browser_search",
		"meshclaw_presentation_create",
		"meshclaw_scheduled_delivery_apply",
		"meshclaw_screen_capture",
		"meshclaw_media_play",
		"meshclaw_purchase_click",
		"meshclaw_fleet_scan",
	} {
		if _, ok := seen[name]; ok {
			t.Fatalf("local-lite exposed non-core/high-token tool %s", name)
		}
	}
}

func TestMCPLocalLiteExposesAssistantFirstSafeSurfaceOnly(t *testing.T) {
	localLite := mcpToolsForProfile("local-lite")
	if len(localLite) == 0 {
		t.Fatal("local-lite returned no tools")
	}
	if localLite[0].Name != "meshclaw_local_assistant" {
		t.Fatalf("local-lite first tool = %q, want meshclaw_local_assistant", localLite[0].Name)
	}
	seen := map[string]bool{}
	for _, tool := range localLite {
		seen[tool.Name] = true
	}
	for _, name := range []string{
		"meshclaw_local_assistant",
		"meshclaw_mcp_catalog",
		"meshclaw_tool_recommend",
		"meshclaw_mail_summarize",
		"meshclaw_calendar_list",
		"meshclaw_calendar_create_event",
		"meshclaw_reminder_create",
		"meshclaw_document_create",
		"meshclaw_data_archive_plan",
		"meshclaw_downloads_cleanup_plan",
		"meshclaw_argos_ask",
	} {
		if !seen[name] {
			t.Fatalf("local-lite missing assistant-first safe tool %s", name)
		}
	}
	for _, name := range []string{
		"meshclaw_run_evidence",
		"meshclaw_terminal_run",
		"meshclaw_shortcut_text_run",
		"meshclaw_job_start",
		"meshclaw_job_cancel",
		"meshclaw_autoheal_apply_safe",
		"meshclaw_data_clean_apply",
		"meshclaw_downloads_cleanup_apply",
		"meshclaw_inventory_override_set",
		"meshclaw_service_quarantine",
		"meshclaw_service_remove",
		"meshclaw_mail_delete",
		"meshclaw_purchase_click",
		"meshclaw_signal_call",
		"meshclaw_screen_capture",
		"meshclaw_screen_record",
	} {
		if seen[name] {
			t.Fatalf("local-lite exposed destructive/direct tool %s", name)
		}
	}

	rawParams, err := json.Marshal(map[string]interface{}{
		"name":      "meshclaw_terminal_run",
		"arguments": map[string]interface{}{"command": "echo should-not-run"},
	})
	if err != nil {
		t.Fatal(err)
	}
	resp := handleMCPWithProfile(mcpRequest{JSONRPC: "2.0", Method: "tools/call", Params: rawParams, ID: 1}, "local-lite")
	if resp.Error == nil {
		t.Fatalf("local-lite tools/call should reject direct tool, got result: %#v", resp.Result)
	}
	if !strings.Contains(resp.Error.Message, "local-lite") || !strings.Contains(resp.Error.Message, "meshclaw_terminal_run") {
		t.Fatalf("unexpected rejection message: %#v", resp.Error)
	}
}

func TestMCPLiteProfilesPreserveProfileToolOrder(t *testing.T) {
	localLite := mcpToolsForProfile("local-lite")
	if len(localLite) == 0 {
		t.Fatal("local-lite returned no tools")
	}
	if localLite[0].Name != "meshclaw_local_assistant" {
		t.Fatalf("local-lite first tool = %q, want meshclaw_local_assistant", localLite[0].Name)
	}
	claudeLite := mcpToolsForProfile("claude-lite")
	if len(claudeLite) == 0 {
		t.Fatal("claude-lite returned no tools")
	}
	if claudeLite[0].Name != "meshclaw_local_assistant" {
		t.Fatalf("claude-lite first tool = %q, want meshclaw_local_assistant", claudeLite[0].Name)
	}
}

func TestMCPLocalAssistantRoutesCommonTasks(t *testing.T) {
	now := time.Date(2026, 6, 5, 9, 0, 0, 0, time.Local)
	tests := []struct {
		name     string
		args     map[string]interface{}
		intent   string
		tool     string
		missing  []string
		mutation bool
	}{
		{
			name:   "schedule status",
			args:   map[string]interface{}{"task": "자동화 상태 확인해줘"},
			intent: "schedule_status",
			tool:   "meshclaw_schedule_status",
		},
		{
			name:   "mail summary",
			args:   map[string]interface{}{"task": "오늘 메일 요약해줘"},
			intent: "mail_summarize",
			tool:   "meshclaw_mail_summarize",
		},
		{
			name:     "calendar create needs structured time",
			args:     map[string]interface{}{"task": "회의 일정 추가해줘", "title": "Argos 테스트 회의"},
			intent:   "calendar_create_event",
			tool:     "meshclaw_calendar_create_event",
			missing:  []string{"start"},
			mutation: true,
		},
		{
			name:     "calendar create infers common Korean time",
			args:     map[string]interface{}{"task": "내일 오후 3시에 Argos 테스트 회의 일정 추가해줘"},
			intent:   "calendar_create_event",
			tool:     "meshclaw_calendar_create_event",
			mutation: true,
		},
		{
			name:   "mail search infers simple Korean query",
			args:   map[string]interface{}{"task": "어제 온 메일 중 영수증 찾아줘"},
			intent: "mail_search",
			tool:   "meshclaw_mail_search",
		},
		{
			name:   "mail search infers last month window",
			args:   map[string]interface{}{"task": "지난달 메일 중 영수증 찾아줘"},
			intent: "mail_search",
			tool:   "meshclaw_mail_search",
		},
		{
			name:   "mail search infers recent days window",
			args:   map[string]interface{}{"task": "최근 3일 메일 중 계약서 찾아줘"},
			intent: "mail_search",
			tool:   "meshclaw_mail_search",
		},
		{
			name:     "document create plans by default",
			args:     map[string]interface{}{"task": "회의 보고서 문서 만들어줘", "title": "회의 보고서"},
			intent:   "document_create",
			tool:     "meshclaw_document_create",
			mutation: true,
		},
		{
			name:     "document title inferred from task text",
			args:     map[string]interface{}{"task": "회의 보고서 문서 만들어줘"},
			intent:   "document_create",
			tool:     "meshclaw_document_create",
			mutation: true,
		},
		{
			name:     "presentation mobile request creates pptx by default",
			args:     map[string]interface{}{"task": "PPT 모바일 테스트 보내줘"},
			intent:   "presentation_create",
			tool:     "meshclaw_presentation_create",
			mutation: true,
		},
		{
			name:     "meeting materials route to presentation artifact",
			args:     map[string]interface{}{"task": "회의자료 샘플 보내줘"},
			intent:   "presentation_create",
			tool:     "meshclaw_presentation_create",
			mutation: true,
		},
		{
			name:   "coupang shopping search opens visible result by default",
			args:   map[string]interface{}{"task": "쿠팡에서 생수 500ml 20개 로켓배송으로 가격, 리뷰 좋은 후보 3개 비교해줘"},
			intent: "shopping_search",
			tool:   "meshclaw_argos_ask",
		},
		{
			name:     "note title inferred from task text",
			args:     map[string]interface{}{"task": "아이디어 메모 작성해줘"},
			intent:   "note_create",
			tool:     "meshclaw_note_create",
			mutation: true,
		},
		{
			name:   "notes search query inferred from task text",
			args:   map[string]interface{}{"task": "아이디어 메모 찾아줘"},
			intent: "notes_search",
			tool:   "meshclaw_notes_search",
		},
		{
			name:     "reminder title inferred from common Korean task",
			args:     map[string]interface{}{"task": "내일 오전 9시에 운동 리마인더 추가해줘"},
			intent:   "reminder_create",
			tool:     "meshclaw_reminder_create",
			mutation: true,
		},
		{
			name:     "reminder due inferred from relative minutes",
			args:     map[string]interface{}{"task": "30분 뒤에 물 마시기 리마인더 추가해줘"},
			intent:   "reminder_create",
			tool:     "meshclaw_reminder_create",
			mutation: true,
		},
		{
			name:     "reminder due inferred from Korean relative hours",
			args:     map[string]interface{}{"task": "두 시간 뒤에 전화하기 리마인더 추가해줘"},
			intent:   "reminder_create",
			tool:     "meshclaw_reminder_create",
			mutation: true,
		},
		{
			name:     "reminder due inferred from named morning time",
			args:     map[string]interface{}{"task": "내일 아침에 약 먹기 리마인더 추가해줘"},
			intent:   "reminder_create",
			tool:     "meshclaw_reminder_create",
			mutation: true,
		},
		{
			name:     "reminder due inferred from named dawn time",
			args:     map[string]interface{}{"task": "내일 새벽에 공항 출발 리마인더 추가해줘"},
			intent:   "reminder_create",
			tool:     "meshclaw_reminder_create",
			mutation: true,
		},
		{
			name:     "reminder due inferred from Korean hour word",
			args:     map[string]interface{}{"task": "내일 오후 일곱시에 운동 리마인더 추가해줘"},
			intent:   "reminder_create",
			tool:     "meshclaw_reminder_create",
			mutation: true,
		},
		{
			name:     "calendar create infers Korean hour and minute words",
			args:     map[string]interface{}{"task": "내일 오후 일곱시 삼십분에 디자인 리뷰 일정 추가해줘"},
			intent:   "calendar_create_event",
			tool:     "meshclaw_calendar_create_event",
			mutation: true,
		},
		{
			name:     "reminder due inferred from noon keyword",
			args:     map[string]interface{}{"task": "내일 정오에 점심 약속 리마인더 추가해줘"},
			intent:   "reminder_create",
			tool:     "meshclaw_reminder_create",
			mutation: true,
		},
		{
			name:     "calendar create infers midnight keyword",
			args:     map[string]interface{}{"task": "내일 자정에 서버 점검 일정 추가해줘"},
			intent:   "calendar_create_event",
			tool:     "meshclaw_calendar_create_event",
			mutation: true,
		},
		{
			name:     "calendar create infers named evening time",
			args:     map[string]interface{}{"task": "오늘 저녁에 가족 식사 일정 추가해줘"},
			intent:   "calendar_create_event",
			tool:     "meshclaw_calendar_create_event",
			mutation: true,
		},
		{
			name:     "calendar create infers half hour Korean time",
			args:     map[string]interface{}{"task": "내일 오후 3시 반에 디자인 리뷰 일정 추가해줘"},
			intent:   "calendar_create_event",
			tool:     "meshclaw_calendar_create_event",
			mutation: true,
		},
		{
			name:     "calendar create infers compact minute Korean time",
			args:     map[string]interface{}{"task": "내일 오후 3시30분에 디자인 리뷰 일정 추가해줘"},
			intent:   "calendar_create_event",
			tool:     "meshclaw_calendar_create_event",
			mutation: true,
		},
		{
			name:     "calendar create infers next week weekday time",
			args:     map[string]interface{}{"task": "다음 주 월요일 오전 10시에 팀 회의 일정 추가해줘"},
			intent:   "calendar_create_event",
			tool:     "meshclaw_calendar_create_event",
			mutation: true,
		},
		{
			name:     "calendar create infers next week weekday shorthand",
			args:     map[string]interface{}{"task": "다음 주 월욜 오전 10시에 팀 회의 일정 추가해줘"},
			intent:   "calendar_create_event",
			tool:     "meshclaw_calendar_create_event",
			mutation: true,
		},
		{
			name:     "calendar create infers next week compact Korean week",
			args:     map[string]interface{}{"task": "내주 월욜 오전 10시에 팀 회의 일정 추가해줘"},
			intent:   "calendar_create_event",
			tool:     "meshclaw_calendar_create_event",
			mutation: true,
		},
		{
			name:     "calendar create infers this week weekday time",
			args:     map[string]interface{}{"task": "이번 주 토요일 오후 2시에 장보기 일정 추가해줘"},
			intent:   "calendar_create_event",
			tool:     "meshclaw_calendar_create_event",
			mutation: true,
		},
		{
			name:     "calendar create infers this week compact Korean week",
			args:     map[string]interface{}{"task": "금주 토욜 오후 2시에 장보기 일정 추가해줘"},
			intent:   "calendar_create_event",
			tool:     "meshclaw_calendar_create_event",
			mutation: true,
		},
		{
			name:     "calendar create infers next month day",
			args:     map[string]interface{}{"task": "다음 달 1일 오전 10시에 정산 회의 일정 추가해줘"},
			intent:   "calendar_create_event",
			tool:     "meshclaw_calendar_create_event",
			mutation: true,
		},
		{
			name:     "calendar create infers this month day",
			args:     map[string]interface{}{"task": "이번 달 15일 오후 4시에 세금 검토 일정 추가해줘"},
			intent:   "calendar_create_event",
			tool:     "meshclaw_calendar_create_event",
			mutation: true,
		},
		{
			name:     "calendar create infers explicit month day",
			args:     map[string]interface{}{"task": "7월 1일 오전 10시에 정산 회의 일정 추가해줘"},
			intent:   "calendar_create_event",
			tool:     "meshclaw_calendar_create_event",
			mutation: true,
		},
		{
			name:     "calendar create infers compact explicit month day",
			args:     map[string]interface{}{"task": "7월1일 오전 10시에 정산 회의 일정 추가해줘"},
			intent:   "calendar_create_event",
			tool:     "meshclaw_calendar_create_event",
			mutation: true,
		},
		{
			name:     "calendar create infers explicit year month day",
			args:     map[string]interface{}{"task": "2026년 7월 1일 오전 10시에 정산 회의 일정 추가해줘"},
			intent:   "calendar_create_event",
			tool:     "meshclaw_calendar_create_event",
			mutation: true,
		},
		{
			name:     "calendar create infers dotted year month day",
			args:     map[string]interface{}{"task": "2026.7.1 오전 10시에 정산 회의 일정 추가해줘"},
			intent:   "calendar_create_event",
			tool:     "meshclaw_calendar_create_event",
			mutation: true,
		},
		{
			name:     "calendar create rolls past explicit month day to next year",
			args:     map[string]interface{}{"task": "1월 5일 오전 9시에 연간 계획 일정 추가해줘"},
			intent:   "calendar_create_event",
			tool:     "meshclaw_calendar_create_event",
			mutation: true,
		},
		{
			name:    "vague contact request asks query",
			args:    map[string]interface{}{"task": "연락처 아무나 찾아봐"},
			intent:  "contacts_search",
			tool:    "meshclaw_contacts_search",
			missing: []string{"query"},
		},
		{
			name:   "contact query inferred from task text",
			args:   map[string]interface{}{"task": "홍길동 연락처 찾아줘"},
			intent: "contacts_search",
			tool:   "meshclaw_contacts_search",
		},
		{
			name:   "downloads cleanup infers large file intent",
			args:   map[string]interface{}{"task": "다운로드 폴더 큰 파일 정리 후보 보여줘"},
			intent: "downloads_cleanup_plan",
			tool:   "meshclaw_downloads_cleanup_plan",
		},
		{
			name:   "data archive plan infers recent keep window",
			args:   map[string]interface{}{"task": "최근 30일은 남기고 증거 아카이브 계획 보여줘"},
			intent: "data_archive_plan",
			tool:   "meshclaw_data_archive_plan",
		},
		{
			name:   "maps directions",
			args:   map[string]interface{}{"task": "지도 길찾기", "origin": "강남역", "destination": "서울역"},
			intent: "maps_directions",
			tool:   "meshclaw_maps_directions",
		},
		{
			name:   "maps destination inferred from common Korean task",
			args:   map[string]interface{}{"task": "서울역 가는 길 찾아줘"},
			intent: "maps_directions",
			tool:   "meshclaw_maps_directions",
		},
		{
			name:   "maps origin and destination inferred from Korean task",
			args:   map[string]interface{}{"task": "강남역에서 서울역 가는 길 찾아줘"},
			intent: "maps_directions",
			tool:   "meshclaw_maps_directions",
		},
		{
			name:   "maps origin and destination inferred from travel time task",
			args:   map[string]interface{}{"task": "강남역에서 서울역까지 얼마나 걸려?"},
			intent: "maps_directions",
			tool:   "meshclaw_maps_directions",
		},
		{
			name:   "maps search query inferred from task text",
			args:   map[string]interface{}{"task": "성수동 카페 지도 보여줘"},
			intent: "maps_search",
			tool:   "meshclaw_maps_search",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			route := localAssistantRouteForArgs(tt.args, now)
			if route.Intent != tt.intent {
				t.Fatalf("intent = %q, want %q", route.Intent, tt.intent)
			}
			if route.Tool != tt.tool {
				t.Fatalf("tool = %q, want %q", route.Tool, tt.tool)
			}
			if route.Mutation != tt.mutation {
				t.Fatalf("mutation = %t, want %t", route.Mutation, tt.mutation)
			}
			for _, field := range tt.missing {
				if !containsString(route.Missing, field) {
					t.Fatalf("missing fields = %#v, want %q", route.Missing, field)
				}
			}
			if tt.name == "reminder title inferred from common Korean task" && route.Args["title"] != "운동" {
				t.Fatalf("title = %#v, want 운동", route.Args["title"])
			}
			if tt.name == "reminder due inferred from relative minutes" {
				if route.Args["title"] != "물 마시기" {
					t.Fatalf("title = %#v, want 물 마시기", route.Args["title"])
				}
				if route.Args["due"] != now.Add(30*time.Minute).Format(time.RFC3339) {
					t.Fatalf("due = %#v, want 30 minutes later", route.Args["due"])
				}
			}
			if tt.name == "presentation mobile request creates pptx by default" {
				if route.Args["execute"] != true || !route.Execute {
					t.Fatalf("execute = route:%#v args:%#v, want true", route.Execute, route.Args["execute"])
				}
				if route.Args["title"] != "모바일 테스트" {
					t.Fatalf("title = %#v, want 모바일 테스트", route.Args["title"])
				}
				if route.Args["audience"] != "iPhone Signal에서 바로 확인할 사용자" {
					t.Fatalf("audience = %#v, want mobile Signal audience", route.Args["audience"])
				}
			}
			if tt.name == "meeting materials route to presentation artifact" {
				if route.Args["execute"] != true || !route.Execute {
					t.Fatalf("execute = route:%#v args:%#v, want true", route.Execute, route.Args["execute"])
				}
				if route.Args["title"] != "샘플" {
					t.Fatalf("title = %#v, want 샘플", route.Args["title"])
				}
				if route.Args["audience"] != "회의 참석자" {
					t.Fatalf("audience = %#v, want 회의 참석자", route.Args["audience"])
				}
			}
			if tt.name == "coupang shopping search opens visible result by default" {
				if route.Args["execute"] != true || !route.Execute {
					t.Fatalf("execute = route:%#v args:%#v, want true", route.Execute, route.Args["execute"])
				}
				text, _ := route.Args["text"].(string)
				if !strings.Contains(text, "https://www.coupang.com/np/search") {
					t.Fatalf("shopping text = %q, want coupang search URL", text)
				}
				if !strings.Contains(text, "%EC%83%9D%EC%88%98+500ml+20") {
					t.Fatalf("shopping text = %q, want cleaned product query", text)
				}
				for _, leaked := range []string{"%EC%A2%8B%EC%9D%80", "%ED%9B%84%EB%B3%B4", "%EB%B9%84%EA%B5%90", "%EB%A1%9C%EC%BC%93"} {
					if strings.Contains(text, leaked) {
						t.Fatalf("shopping text leaked instruction token %s: %q", leaked, text)
					}
				}
			}
			if tt.name == "reminder due inferred from Korean relative hours" {
				if route.Args["title"] != "전화하기" {
					t.Fatalf("title = %#v, want 전화하기", route.Args["title"])
				}
				if route.Args["due"] != now.Add(2*time.Hour).Format(time.RFC3339) {
					t.Fatalf("due = %#v, want 2 hours later", route.Args["due"])
				}
			}
			if tt.name == "reminder due inferred from named morning time" {
				if route.Args["title"] != "약 먹기" {
					t.Fatalf("title = %#v, want 약 먹기", route.Args["title"])
				}
				want := time.Date(2026, 6, 6, 8, 0, 0, 0, time.Local).Format(time.RFC3339)
				if route.Args["due"] != want {
					t.Fatalf("due = %#v, want %s", route.Args["due"], want)
				}
			}
			if tt.name == "reminder due inferred from named dawn time" {
				if route.Args["title"] != "공항 출발" {
					t.Fatalf("title = %#v, want 공항 출발", route.Args["title"])
				}
				want := time.Date(2026, 6, 6, 6, 0, 0, 0, time.Local).Format(time.RFC3339)
				if route.Args["due"] != want {
					t.Fatalf("due = %#v, want %s", route.Args["due"], want)
				}
			}
			if tt.name == "reminder due inferred from Korean hour word" {
				if route.Args["title"] != "운동" {
					t.Fatalf("title = %#v, want 운동", route.Args["title"])
				}
				want := time.Date(2026, 6, 6, 19, 0, 0, 0, time.Local).Format(time.RFC3339)
				if route.Args["due"] != want {
					t.Fatalf("due = %#v, want %s", route.Args["due"], want)
				}
			}
			if tt.name == "calendar create infers Korean hour and minute words" {
				if route.Args["title"] != "디자인 리뷰" {
					t.Fatalf("title = %#v, want 디자인 리뷰", route.Args["title"])
				}
				want := time.Date(2026, 6, 6, 19, 30, 0, 0, time.Local).Format(time.RFC3339)
				if route.Args["start"] != want {
					t.Fatalf("start = %#v, want %s", route.Args["start"], want)
				}
			}
			if tt.name == "reminder due inferred from noon keyword" {
				if route.Args["title"] != "약속" {
					t.Fatalf("title = %#v, want 약속", route.Args["title"])
				}
				want := time.Date(2026, 6, 6, 12, 0, 0, 0, time.Local).Format(time.RFC3339)
				if route.Args["due"] != want {
					t.Fatalf("due = %#v, want %s", route.Args["due"], want)
				}
			}
			if tt.name == "calendar create infers midnight keyword" {
				if route.Args["title"] != "서버 점검" {
					t.Fatalf("title = %#v, want 서버 점검", route.Args["title"])
				}
				want := time.Date(2026, 6, 6, 0, 0, 0, 0, time.Local).Format(time.RFC3339)
				if route.Args["start"] != want {
					t.Fatalf("start = %#v, want %s", route.Args["start"], want)
				}
			}
			if tt.name == "calendar create infers named evening time" {
				if route.Args["title"] != "가족 식사" {
					t.Fatalf("title = %#v, want 가족 식사", route.Args["title"])
				}
				want := time.Date(2026, 6, 5, 19, 0, 0, 0, time.Local).Format(time.RFC3339)
				if route.Args["start"] != want {
					t.Fatalf("start = %#v, want %s", route.Args["start"], want)
				}
			}
			if tt.name == "calendar create infers half hour Korean time" {
				want := time.Date(2026, 6, 6, 15, 30, 0, 0, time.Local).Format(time.RFC3339)
				if route.Args["start"] != want {
					t.Fatalf("start = %#v, want %s", route.Args["start"], want)
				}
			}
			if tt.name == "calendar create infers compact minute Korean time" {
				want := time.Date(2026, 6, 6, 15, 30, 0, 0, time.Local).Format(time.RFC3339)
				if route.Args["start"] != want {
					t.Fatalf("start = %#v, want %s", route.Args["start"], want)
				}
			}
			if tt.name == "calendar create infers next week weekday time" {
				if route.Args["title"] != "팀 회의" {
					t.Fatalf("title = %#v, want 팀 회의", route.Args["title"])
				}
				want := time.Date(2026, 6, 8, 10, 0, 0, 0, time.Local).Format(time.RFC3339)
				if route.Args["start"] != want {
					t.Fatalf("start = %#v, want %s", route.Args["start"], want)
				}
			}
			if tt.name == "calendar create infers next week weekday shorthand" {
				if route.Args["title"] != "팀 회의" {
					t.Fatalf("title = %#v, want 팀 회의", route.Args["title"])
				}
				want := time.Date(2026, 6, 8, 10, 0, 0, 0, time.Local).Format(time.RFC3339)
				if route.Args["start"] != want {
					t.Fatalf("start = %#v, want %s", route.Args["start"], want)
				}
			}
			if tt.name == "calendar create infers next week compact Korean week" {
				if route.Args["title"] != "팀 회의" {
					t.Fatalf("title = %#v, want 팀 회의", route.Args["title"])
				}
				want := time.Date(2026, 6, 8, 10, 0, 0, 0, time.Local).Format(time.RFC3339)
				if route.Args["start"] != want {
					t.Fatalf("start = %#v, want %s", route.Args["start"], want)
				}
			}
			if tt.name == "calendar create infers this week weekday time" {
				if route.Args["title"] != "장보기" {
					t.Fatalf("title = %#v, want 장보기", route.Args["title"])
				}
				want := time.Date(2026, 6, 6, 14, 0, 0, 0, time.Local).Format(time.RFC3339)
				if route.Args["start"] != want {
					t.Fatalf("start = %#v, want %s", route.Args["start"], want)
				}
			}
			if tt.name == "calendar create infers this week compact Korean week" {
				if route.Args["title"] != "장보기" {
					t.Fatalf("title = %#v, want 장보기", route.Args["title"])
				}
				want := time.Date(2026, 6, 6, 14, 0, 0, 0, time.Local).Format(time.RFC3339)
				if route.Args["start"] != want {
					t.Fatalf("start = %#v, want %s", route.Args["start"], want)
				}
			}
			if tt.name == "calendar create infers next month day" {
				if route.Args["title"] != "정산 회의" {
					t.Fatalf("title = %#v, want 정산 회의", route.Args["title"])
				}
				want := time.Date(2026, 7, 1, 10, 0, 0, 0, time.Local).Format(time.RFC3339)
				if route.Args["start"] != want {
					t.Fatalf("start = %#v, want %s", route.Args["start"], want)
				}
			}
			if tt.name == "calendar create infers this month day" {
				if route.Args["title"] != "세금 검토" {
					t.Fatalf("title = %#v, want 세금 검토", route.Args["title"])
				}
				want := time.Date(2026, 6, 15, 16, 0, 0, 0, time.Local).Format(time.RFC3339)
				if route.Args["start"] != want {
					t.Fatalf("start = %#v, want %s", route.Args["start"], want)
				}
			}
			if tt.name == "calendar create infers explicit month day" {
				if route.Args["title"] != "정산 회의" {
					t.Fatalf("title = %#v, want 정산 회의", route.Args["title"])
				}
				want := time.Date(2026, 7, 1, 10, 0, 0, 0, time.Local).Format(time.RFC3339)
				if route.Args["start"] != want {
					t.Fatalf("start = %#v, want %s", route.Args["start"], want)
				}
			}
			if tt.name == "calendar create infers compact explicit month day" {
				if route.Args["title"] != "정산 회의" {
					t.Fatalf("title = %#v, want 정산 회의", route.Args["title"])
				}
				want := time.Date(2026, 7, 1, 10, 0, 0, 0, time.Local).Format(time.RFC3339)
				if route.Args["start"] != want {
					t.Fatalf("start = %#v, want %s", route.Args["start"], want)
				}
			}
			if tt.name == "calendar create infers explicit year month day" {
				if route.Args["title"] != "정산 회의" {
					t.Fatalf("title = %#v, want 정산 회의", route.Args["title"])
				}
				want := time.Date(2026, 7, 1, 10, 0, 0, 0, time.Local).Format(time.RFC3339)
				if route.Args["start"] != want {
					t.Fatalf("start = %#v, want %s", route.Args["start"], want)
				}
			}
			if tt.name == "calendar create infers dotted year month day" {
				if route.Args["title"] != "정산 회의" {
					t.Fatalf("title = %#v, want 정산 회의", route.Args["title"])
				}
				want := time.Date(2026, 7, 1, 10, 0, 0, 0, time.Local).Format(time.RFC3339)
				if route.Args["start"] != want {
					t.Fatalf("start = %#v, want %s", route.Args["start"], want)
				}
			}
			if tt.name == "calendar create rolls past explicit month day to next year" {
				if route.Args["title"] != "연간 계획" {
					t.Fatalf("title = %#v, want 연간 계획", route.Args["title"])
				}
				want := time.Date(2027, 1, 5, 9, 0, 0, 0, time.Local).Format(time.RFC3339)
				if route.Args["start"] != want {
					t.Fatalf("start = %#v, want %s", route.Args["start"], want)
				}
			}
			if tt.name == "maps destination inferred from common Korean task" && route.Args["destination"] != "서울역" {
				t.Fatalf("destination = %#v, want 서울역", route.Args["destination"])
			}
			if tt.name == "calendar create infers common Korean time" && route.Args["start"] == "" {
				t.Fatalf("start was not inferred: %#v", route.Args)
			}
			if tt.name == "mail search infers simple Korean query" && route.Args["query"] != "영수증" {
				t.Fatalf("query = %#v, want 영수증", route.Args["query"])
			}
			if tt.name == "mail search infers last month window" {
				if route.Args["query"] != "영수증" || route.Args["since"] != "31d" {
					t.Fatalf("route args = %#v, want query=영수증 since=31d", route.Args)
				}
			}
			if tt.name == "mail search infers recent days window" {
				if route.Args["query"] != "계약서" || route.Args["since"] != "3d" {
					t.Fatalf("route args = %#v, want query=계약서 since=3d", route.Args)
				}
			}
			if tt.name == "document title inferred from task text" && route.Args["title"] != "회의 보고서" {
				t.Fatalf("title = %#v, want 회의 보고서", route.Args["title"])
			}
			if tt.name == "note title inferred from task text" && route.Args["title"] != "아이디어" {
				t.Fatalf("title = %#v, want 아이디어", route.Args["title"])
			}
			if tt.name == "notes search query inferred from task text" && route.Args["query"] != "아이디어" {
				t.Fatalf("query = %#v, want 아이디어", route.Args["query"])
			}
			if tt.name == "contact query inferred from task text" && route.Args["query"] != "홍길동" {
				t.Fatalf("query = %#v, want 홍길동", route.Args["query"])
			}
			if tt.name == "downloads cleanup infers large file intent" && route.Args["large_mb"] != 100 {
				t.Fatalf("large_mb = %#v, want 100", route.Args["large_mb"])
			}
			if tt.name == "data archive plan infers recent keep window" && route.Args["keep_newest"] != 30 {
				t.Fatalf("keep_newest = %#v, want 30", route.Args["keep_newest"])
			}
			if tt.name == "maps origin and destination inferred from Korean task" {
				if route.Args["origin"] != "강남역" || route.Args["destination"] != "서울역" {
					t.Fatalf("route args = %#v, want origin/destination", route.Args)
				}
			}
			if tt.name == "maps origin and destination inferred from travel time task" {
				if route.Args["origin"] != "강남역" || route.Args["destination"] != "서울역" {
					t.Fatalf("route args = %#v, want origin/destination", route.Args)
				}
			}
			if tt.name == "maps search query inferred from task text" && route.Args["query"] != "성수동 카페" {
				t.Fatalf("query = %#v, want 성수동 카페", route.Args["query"])
			}
		})
	}
}

func TestMCPLocalAssistantCanCallReadOnlyScheduleStatus(t *testing.T) {
	result, err := callMCPTool("meshclaw_local_assistant", map[string]interface{}{"task": "스케줄 상태 알려줘"})
	if err != nil {
		t.Fatalf("callMCPTool error = %v", err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	if payload["selected_tool"] != "meshclaw_schedule_status" {
		t.Fatalf("selected_tool = %#v", payload["selected_tool"])
	}
	if payload["status"] != "ok" {
		t.Fatalf("status = %#v payload=%#v", payload["status"], payload)
	}
	if payload["result"] == nil {
		t.Fatalf("missing nested result: %#v", payload)
	}
}

func TestMCPProfileAliasesKeepClaudeAndLocalSeparate(t *testing.T) {
	if got := normalizeMCPProfile("local"); got != "local-lite" {
		t.Fatalf("local alias = %q", got)
	}
	if got := normalizeMCPProfile("lite"); got != "claude-lite" {
		t.Fatalf("lite alias = %q", got)
	}
}

func TestMCPProfileAliasesIncludePersonalAndClaudeNames(t *testing.T) {
	if got := normalizeMCPProfile("personal-lite"); got != "local-lite" {
		t.Fatalf("personal-lite alias = %q", got)
	}
	if got := normalizeMCPProfile("claude"); got != "claude-lite" {
		t.Fatalf("claude alias = %q", got)
	}
}

func TestNormalizeMCPProfileTrimsWhitespace(t *testing.T) {
	if got := normalizeMCPProfile(" LOCAL-LITE "); got != "local-lite" {
		t.Fatalf("local-lite with whitespace should normalize = %q", got)
	}
	if got := normalizeMCPProfile(" CLAUDE "); got != "claude-lite" {
		t.Fatalf("claude with whitespace should normalize = %q", got)
	}
}

func TestParseMCPProfileFromArgs(t *testing.T) {
	if got := parseMCPProfile([]string{"--profile", "local-lite"}); got != "local-lite" {
		t.Fatalf("explicit --profile = %q", got)
	}
	if got := parseMCPProfile([]string{"--profile", " LOCAL-LITE "}); got != "local-lite" {
		t.Fatalf("whitespace profile arg = %q", got)
	}
	if got := parseMCPProfile([]string{"--profile=claude"}); got != "claude-lite" {
		t.Fatalf("explicit --profile= form = %q", got)
	}
	if got := parseMCPProfile([]string{"--profile= LOCAL-LITE "}); got != "local-lite" {
		t.Fatalf("spaced --profile= form = %q", got)
	}
	if got := parseMCPProfile([]string{"--lite"}); got != "claude-lite" {
		t.Fatalf("--lite maps = %q", got)
	}
	if got := parseMCPProfile([]string{"--full"}); got != "all" {
		t.Fatalf("--full maps = %q", got)
	}
	if got := parseMCPProfile([]string{"serve", "--profile", "claude-lite"}); got != "claude-lite" {
		t.Fatalf("serve compatibility form maps = %q", got)
	}
	if got := parseMCPProfile([]string{"stdio", "--profile=local"}); got != "local-lite" {
		t.Fatalf("stdio compatibility form maps = %q", got)
	}
	t.Setenv("MESHCLAW_MCP_PROFILE", "local")
	if got := parseMCPProfile(nil); got != "local-lite" {
		t.Fatalf("env local map = %q", got)
	}
	if got := parseMCPProfile([]string{"--profile", "unknown", "--lite"}); got != "claude-lite" {
		t.Fatalf("last profile arg wins = %q", got)
	}
}

func TestMCPInitializeUsesRequestedSupportedProtocol(t *testing.T) {
	req := mcpRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
		Params:  json.RawMessage(`{"protocolVersion":"2025-11-25","clientInfo":{"name":"claude-ai","version":"0.1.0"}}`),
		ID:      1,
	}
	resp := handleMCPWithProfile(req, "claude-lite")
	result, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("initialize result type = %T", resp.Result)
	}
	if result["protocolVersion"] != "2025-11-25" {
		t.Fatalf("protocolVersion = %#v", result["protocolVersion"])
	}
	if resp.Error != nil {
		t.Fatalf("initialize error = %#v", resp.Error)
	}
}

func TestMCPCatalogIncludesClaudeLiteProfile(t *testing.T) {
	result, err := callMCPTool("meshclaw_mcp_catalog", map[string]interface{}{"profile": "claude-lite"})
	if err != nil {
		t.Fatalf("callMCPTool error = %v", err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	selected, ok := payload["selected_profile"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing claude-lite selected_profile: %#v", payload)
	}
	if !strings.Contains(fmt.Sprint(selected["purpose"]), "Low-token") {
		t.Fatalf("claude-lite purpose missing token guidance: %#v", selected)
	}
	contracts, ok := selected["evidence_contracts"].([]string)
	if !ok ||
		!containsString(contracts, "container logscan can return autoheal_handoff.runtime_evidence_checklist before self-heal planning") ||
		!containsString(contracts, "readiness summaries are checkpoints, not operator approval") {
		t.Fatalf("claude-lite evidence contracts missing runtime checklist/readiness guidance: %#v", selected["evidence_contracts"])
	}
	defaultTools, ok := selected["default_tools"].([]string)
	if !ok || !containsString(defaultTools, "meshclaw_argos_ask") {
		t.Fatalf("claude-lite default tools missing fallback: %#v", selected)
	}
}

func TestMCPCatalogClaudeLiteIncludesReconcilePlanningChain(t *testing.T) {
	result, err := callMCPTool("meshclaw_mcp_catalog", map[string]interface{}{"profile": "claude-lite"})
	if err != nil {
		t.Fatalf("callMCPTool error = %v", err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	selected, ok := payload["selected_profile"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing claude-lite selected_profile: %#v", payload)
	}
	defaultTools, ok := selected["default_tools"].([]string)
	if !ok {
		t.Fatalf("claude-lite default_tools type = %T", selected["default_tools"])
	}
	for _, name := range desiredStateReconcilePlanningToolNames() {
		if !containsString(defaultTools, name) {
			t.Fatalf("claude-lite default_tools missing reconcile planning tool %s", name)
		}
	}
}

func TestMCPCatalogClaudeLiteIncludesContainerSelfHealPlanningChain(t *testing.T) {
	result, err := callMCPTool("meshclaw_mcp_catalog", map[string]interface{}{"profile": "claude-lite"})
	if err != nil {
		t.Fatalf("callMCPTool error = %v", err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	selected, ok := payload["selected_profile"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing claude-lite selected_profile: %#v", payload)
	}
	defaultTools, ok := selected["default_tools"].([]string)
	if !ok {
		t.Fatalf("claude-lite default_tools type = %T", selected["default_tools"])
	}
	if !containsString(defaultTools, "meshclaw_autoheal_plan") {
		t.Fatal("claude-lite default_tools missing meshclaw_autoheal_plan")
	}
	for _, name := range containerSelfHealPlanningToolNames() {
		if !containsString(defaultTools, name) {
			t.Fatalf("claude-lite default_tools missing container self-heal planning tool %s", name)
		}
	}
	if !containsString(defaultTools, "meshclaw_analyze_logs") {
		t.Fatal("claude-lite default_tools missing meshclaw_analyze_logs for container-logscan evidence")
	}
}

func TestMCPCatalogIncludesLocalLiteProfile(t *testing.T) {
	result, err := callMCPTool("meshclaw_mcp_catalog", map[string]interface{}{"profile": "local-lite"})
	if err != nil {
		t.Fatalf("callMCPTool error = %v", err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	selected, ok := payload["selected_profile"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing local-lite selected_profile: %#v", payload)
	}
	if !strings.Contains(fmt.Sprint(selected["purpose"]), "local LLM") {
		t.Fatalf("local-lite purpose missing local model guidance: %#v", selected)
	}
	defaultTools, ok := selected["default_tools"].([]string)
	if !ok || !containsString(defaultTools, "meshclaw_document_create") {
		t.Fatalf("local-lite default tools missing document_create: %#v", selected)
	}
}

func TestMCPCatalogDefaultSurfaceIncludesContainerSelfHealChain(t *testing.T) {
	result, err := callMCPTool("meshclaw_mcp_catalog", map[string]interface{}{})
	if err != nil {
		t.Fatalf("callMCPTool error = %v", err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	defaultTools := mcpToolNames(mcpToolsForProfile("all"))
	if len(defaultTools) == 0 {
		t.Fatal("all profile returned no default tools")
	}
	fleetOpsTools := catalogCategoryTools(payload, "fleet_ops")
	if len(fleetOpsTools) == 0 {
		t.Fatalf("fleet_ops category missing from catalog: %#v", payload["categories"])
	}
	for _, name := range containerSelfHealPlanningToolNames() {
		if !containsString(defaultTools, name) {
			t.Fatalf("default_tools missing container self-heal tool %s", name)
		}
		if !containsString(fleetOpsTools, name) {
			t.Fatalf("fleet_ops category missing container self-heal tool %s", name)
		}
	}
	for _, name := range containerSelfHealExecutionToolNames() {
		if !containsString(defaultTools, name) {
			t.Fatalf("default_tools missing container self-heal execution tool %s", name)
		}
		if !containsString(fleetOpsTools, name) {
			t.Fatalf("fleet_ops category missing container self-heal execution tool %s", name)
		}
	}
}

func TestMCPCatalogProfilesAndCategories(t *testing.T) {
	result, err := callMCPTool("meshclaw_mcp_catalog", map[string]interface{}{"profile": "one-machine-assistant"})
	if err != nil {
		t.Fatalf("callMCPTool error = %v", err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	if payload["kind"] != "meshclaw_mcp_catalog" {
		t.Fatalf("kind = %#v", payload["kind"])
	}
	if payload["selected_profile"] == nil {
		t.Fatalf("missing selected_profile: %#v", payload)
	}
	if payload["artifact_rule"] == "" {
		t.Fatalf("missing artifact_rule: %#v", payload)
	}
	selected, ok := payload["selected_profile"].(map[string]interface{})
	if !ok {
		t.Fatalf("selected_profile type = %T", payload["selected_profile"])
	}
	if selected["artifact_contract"] == nil || selected["first_success_tasks"] == nil {
		t.Fatalf("selected_profile missing public UX fields: %#v", selected)
	}
	toolsets, ok := selected["macos_toolsets"].([]map[string]interface{})
	if !ok || len(toolsets) == 0 {
		t.Fatalf("selected_profile missing macos_toolsets: %#v", selected)
	}
	foundReadPersonal := false
	foundCommunicationAlerts := false
	foundVisibleMapHandoff := false
	foundTransactionalWebHandoff := false
	foundFileCleanupPlanning := false
	foundAudioTranscription := false
	foundLegacySkillImport := false
	foundDurableOSWork := false
	foundBoundedFallback := false
	for _, toolset := range toolsets {
		id, _ := toolset["id"].(string)
		rule := fmt.Sprint(toolset["model_rule"])
		switch id {
		case "read_personal_data":
			foundReadPersonal = strings.Contains(rule, "Do not route read-only personal data questions through generic argos_ask")
		case "communication_alerts":
			foundCommunicationAlerts = strings.Contains(rule, "never place a real call without execute=true plus approve=true")
		case "visible_map_handoff":
			foundVisibleMapHandoff = strings.Contains(rule, "Always return a clickable Maps URL") &&
				strings.Contains(rule, "prefer maps_proof") &&
				strings.Contains(rule, "execute=true plus approve=true")
		case "transactional_web_handoff":
			foundTransactionalWebHandoff = strings.Contains(rule, "purchase_click") &&
				strings.Contains(rule, "execute=true") &&
				strings.Contains(rule, "approve=true")
		case "file_cleanup_planning":
			foundFileCleanupPlanning = strings.Contains(rule, "Use cleanup_plan first") &&
				strings.Contains(rule, "never move/delete/archive files from a plan") &&
				strings.Contains(rule, "cleanup_apply with execute=true plus approve=true")
		case "audio_transcription":
			foundAudioTranscription = strings.Contains(rule, "Default execute=false") &&
				strings.Contains(rule, "execute=true plus approve=true") &&
				strings.Contains(rule, "local")
		case "legacy_skill_import":
			foundLegacySkillImport = strings.Contains(rule, "Treat legacy skills as read-only source material") &&
				strings.Contains(rule, "Never call the old skill runner")
		case "durable_os_work":
			foundDurableOSWork = strings.Contains(rule, "result_save")
		case "bounded_ui_fallback":
			foundBoundedFallback = strings.Contains(rule, "Use argos_ask only")
		}
	}
	if !foundReadPersonal || !foundCommunicationAlerts || !foundVisibleMapHandoff || !foundTransactionalWebHandoff || !foundFileCleanupPlanning || !foundAudioTranscription || !foundLegacySkillImport || !foundDurableOSWork || !foundBoundedFallback {
		t.Fatalf("macos_toolsets missing model selection guidance: %#v", toolsets)
	}
	defaultTools, ok := selected["default_tools"].([]string)
	if !ok {
		t.Fatalf("default_tools type = %T", selected["default_tools"])
	}
	toolSeen := map[string]bool{}
	for _, tool := range defaultTools {
		toolSeen[tool] = true
	}
	for _, tool := range []string{"meshclaw_setup_assistant", "meshclaw_model_config_status", "meshclaw_presentation_create", "meshclaw_maps_directions", "meshclaw_maps_proof", "meshclaw_mail_summarize", "meshclaw_data_archive_plan", "meshclaw_downloads_cleanup_plan", "meshclaw_downloads_cleanup_apply", "meshclaw_legacy_skills_audit", "meshclaw_purchase_click", "meshclaw_audio_transcribe", "meshclaw_scheduled_delivery_plan", "meshclaw_scheduled_delivery_apply"} {
		if !toolSeen[tool] {
			t.Fatalf("selected_profile default_tools missing %s", tool)
		}
	}
	if selected["signal_reply_rule"] == "" {
		t.Fatalf("selected_profile missing signal_reply_rule: %#v", selected)
	}
	firstTasks, ok := selected["first_success_tasks"].([]map[string]interface{})
	if !ok {
		t.Fatalf("first_success_tasks type = %T", selected["first_success_tasks"])
	}
	foundMailFollowup := false
	foundResearchReport := false
	foundSearchSpreadsheet := false
	foundArtifactRevision := false
	for _, task := range firstTasks {
		if strings.Contains(fmt.Sprint(task["intent"]), "첫 번째 메일") &&
			strings.Contains(fmt.Sprint(task["expected"]), "no email transmission") {
			foundMailFollowup = true
		}
		if strings.Contains(fmt.Sprint(task["tool"]), "meshclaw_argos_research") &&
			strings.Contains(fmt.Sprint(task["expected"]), "cited [S1] source-grounded summary") {
			foundResearchReport = true
		}
		if strings.Contains(fmt.Sprint(task["intent"]), "결과를 엑셀 표로 정리") &&
			strings.Contains(fmt.Sprint(task["expected"]), "fetched body excerpts") {
			foundSearchSpreadsheet = true
		}
		if strings.Contains(fmt.Sprint(task["intent"]), "방금 만든 PPT") &&
			strings.Contains(fmt.Sprint(task["expected"]), "no in-place mutation") {
			foundArtifactRevision = true
		}
	}
	if !foundMailFollowup {
		t.Fatalf("missing mail follow-up first success task: %#v", firstTasks)
	}
	if !foundResearchReport {
		t.Fatalf("missing source-grounded research first success task: %#v", firstTasks)
	}
	if !foundSearchSpreadsheet {
		t.Fatalf("missing search-result spreadsheet first success task: %#v", firstTasks)
	}
	if !foundArtifactRevision {
		t.Fatalf("missing recent artifact revision first success task: %#v", firstTasks)
	}
	categories, ok := payload["categories"].([]map[string]interface{})
	if !ok {
		t.Fatalf("categories type = %T", payload["categories"])
	}
	seen := map[string]bool{}
	mailPurpose := ""
	visibleWorkPurpose := ""
	macosToolsetsPurpose := ""
	for _, category := range categories {
		if id, _ := category["id"].(string); id != "" {
			seen[id] = true
			if id == "mail" {
				mailPurpose, _ = category["purpose"].(string)
			}
			if id == "visible_work" {
				visibleWorkPurpose, _ = category["purpose"].(string)
			}
			if id == "macos_toolsets" {
				macosToolsetsPurpose, _ = category["purpose"].(string)
			}
		}
	}
	for _, id := range []string{"setup", "macos_toolsets", "personal_assistant", "mail", "visible_work", "fleet_ops"} {
		if !seen[id] {
			t.Fatalf("missing category %s in %#v", id, categories)
		}
	}
	if !strings.Contains(mailPurpose, "numbered follow-ups") {
		t.Fatalf("mail category missing follow-up guidance: %q", mailPurpose)
	}
	if !strings.Contains(visibleWorkPurpose, "cited [S1] summaries") {
		t.Fatalf("visible_work category missing research citation guidance: %q", visibleWorkPurpose)
	}
	if !strings.Contains(macosToolsetsPurpose, "Model-friendly OS toolsets") {
		t.Fatalf("macos_toolsets category missing model guidance: %q", macosToolsetsPurpose)
	}
}

func TestMCPDataArchivePlanIsPlanOnly(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, ".meshclaw", "evidence")
	for _, day := range []string{"2026-05-24", "2026-05-25", "2026-05-26"} {
		dir := filepath.Join(root, day)
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, strings.ReplaceAll(day, "-", "")+"T010203Z-123-kind-host.json"), []byte(`{"ok":true}`), 0600); err != nil {
			t.Fatal(err)
		}
	}

	result, err := callMCPTool("meshclaw_data_archive_plan", map[string]interface{}{"keep_newest": float64(2)})
	if err != nil {
		t.Fatalf("callMCPTool error = %v", err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	plan, ok := payload["plan"].(datadoctor.ArchivePlan)
	if !ok {
		t.Fatalf("plan type = %T", payload["plan"])
	}
	if plan.CandidateFiles != 1 || len(plan.Candidates) != 1 || plan.Candidates[0].Date != "2026-05-24" {
		t.Fatalf("unexpected archive plan: %#v", plan)
	}
	if _, err := os.Stat(filepath.Join(root, "2026-05-24")); err != nil {
		t.Fatalf("archive plan must not delete evidence directories: %v", err)
	}
}

func TestMCPDownloadsCleanupPlanIsPlanOnly(t *testing.T) {
	dir := t.TempDir()
	nowOld := time.Now().Add(-40 * 24 * time.Hour)
	oldFile := filepath.Join(dir, "old-installer.dmg")
	freshFile := filepath.Join(dir, "fresh.txt")
	if err := os.WriteFile(oldFile, []byte("installer"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(freshFile, []byte("fresh"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(oldFile, nowOld, nowOld); err != nil {
		t.Fatal(err)
	}

	result, err := callMCPTool("meshclaw_downloads_cleanup_plan", map[string]interface{}{"path": dir, "min_age_days": float64(30)})
	if err != nil {
		t.Fatalf("callMCPTool error = %v", err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	plan, ok := payload["plan"].(fileorg.DownloadsPlan)
	if !ok {
		t.Fatalf("plan type = %T", payload["plan"])
	}
	if plan.CandidateFiles != 1 || len(plan.Candidates) != 1 || plan.Candidates[0].Name != "old-installer.dmg" {
		t.Fatalf("unexpected downloads plan: %#v", plan)
	}
	if plan.Status != "review_ready" || !strings.Contains(plan.UserMessage, "검토") {
		t.Fatalf("missing review-ready user guidance: %#v", plan)
	}
	if _, err := os.Stat(oldFile); err != nil {
		t.Fatalf("downloads plan must not delete candidate: %v", err)
	}
	if _, err := os.Stat(freshFile); err != nil {
		t.Fatalf("downloads plan must not touch fresh file: %v", err)
	}
}

func TestMCPDownloadsCleanupApplyRequiresApproval(t *testing.T) {
	dir := t.TempDir()
	oldFile := filepath.Join(dir, "old-installer.dmg")
	if err := os.WriteFile(oldFile, []byte("installer"), 0600); err != nil {
		t.Fatal(err)
	}

	result, err := callMCPTool("meshclaw_downloads_cleanup_apply", map[string]interface{}{"paths": oldFile, "execute": true})
	if err != nil {
		t.Fatalf("callMCPTool error = %v", err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	plan, ok := payload["plan"].(fileorg.DownloadsApplyPlan)
	if !ok {
		t.Fatalf("plan type = %T", payload["plan"])
	}
	if !plan.ApprovalMissing || plan.CompletedFiles != 0 {
		t.Fatalf("expected approval-missing apply plan: %#v", plan)
	}
	if _, err := os.Stat(oldFile); err != nil {
		t.Fatalf("file should not move without approval: %v", err)
	}
}

func TestMCPScheduledDeliveryPlanIsReviewOnly(t *testing.T) {
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(t.TempDir(), "targets.json"))
	if _, _, err := messenger.UpsertTarget(messenger.Target{ID: "report-room", Channel: "signal", GroupID: "group-123", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatal(err)
	}
	result, err := callMCPTool("meshclaw_scheduled_delivery_plan", map[string]interface{}{
		"target":       "보고방",
		"schedule":     "매일 오전 8시",
		"content":      "오늘 업무 보고",
		"content_type": "voice_report",
		"execute":      true,
	})
	if err != nil {
		t.Fatalf("callMCPTool error = %v", err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	plan, ok := payload["plan"].(messenger.ScheduledDeliveryPlan)
	if !ok {
		t.Fatalf("plan type = %T in %#v", payload["plan"], payload)
	}
	if plan.Status != "approval_required" || !plan.ApprovalMissing {
		t.Fatalf("scheduled delivery should require approval: %#v", plan)
	}
	if plan.ResolvedTarget == nil || plan.ResolvedTarget.ID != "report-room" {
		t.Fatalf("target not resolved: %#v", plan)
	}
}

func TestMCPScheduledDeliveryApplyRequiresPreviewApproval(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(dir, "targets.json"))
	storePath := filepath.Join(dir, "scheduled.json")
	t.Setenv("MESHCLAW_SCHEDULED_DELIVERIES", storePath)
	if _, _, err := messenger.UpsertTarget(messenger.Target{ID: "report-room", Channel: "signal", GroupID: "group-123", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatal(err)
	}
	result, err := callMCPTool("meshclaw_scheduled_delivery_apply", map[string]interface{}{
		"target":       "보고방",
		"schedule":     "매일 오전 8시",
		"content":      "오늘 업무 보고",
		"content_type": "voice_report",
		"execute":      true,
		"approve":      true,
	})
	if err != nil {
		t.Fatalf("callMCPTool error = %v", err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	apply, ok := payload["result"].(messenger.ScheduledDeliveryApplyResult)
	if !ok {
		t.Fatalf("apply result type = %T", payload["result"])
	}
	if apply.Status != "approval_required" || !apply.ApprovalMissing {
		t.Fatalf("apply should require preview approval: %#v", apply)
	}
	if _, err := os.Stat(storePath); !os.IsNotExist(err) {
		t.Fatalf("store should not be written without preview approval, stat err=%v", err)
	}
}

func TestMCPScheduledDeliveryApplyRegistersApprovedJob(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(dir, "targets.json"))
	t.Setenv("MESHCLAW_SCHEDULED_DELIVERIES", filepath.Join(dir, "scheduled.json"))
	if _, _, err := messenger.UpsertTarget(messenger.Target{ID: "report-room", Channel: "signal", GroupID: "group-123", Label: "보고방", Mode: "briefing"}); err != nil {
		t.Fatal(err)
	}
	result, err := callMCPTool("meshclaw_scheduled_delivery_apply", map[string]interface{}{
		"target":           "보고방",
		"schedule":         "매일 오전 8시",
		"content":          "오늘 업무 보고",
		"content_type":     "voice_report",
		"execute":          true,
		"approve":          true,
		"preview_approved": true,
	})
	if err != nil {
		t.Fatalf("callMCPTool error = %v", err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	apply, ok := payload["result"].(messenger.ScheduledDeliveryApplyResult)
	if !ok {
		t.Fatalf("apply result type = %T", payload["result"])
	}
	if apply.Status != "registered" || apply.Job == nil || apply.Job.TargetID != "report-room" {
		t.Fatalf("expected registered job: %#v", apply)
	}
	store, err := messenger.LoadScheduledDeliveryStore()
	if err != nil {
		t.Fatalf("LoadScheduledDeliveryStore error = %v", err)
	}
	if len(store.Jobs) != 1 {
		t.Fatalf("expected one stored job: %#v", store)
	}
}

func TestMCPLegacySkillsAuditIsReadOnlyImportPlan(t *testing.T) {
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "morning-prayer")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	body := `{
  "name": "morning-prayer",
  "description": "Generate and speak morning prayer with TTS",
  "model": "claude-sonnet-4-20250514",
  "tools": ["bash"]
}`
	if err := os.WriteFile(filepath.Join(skillDir, "skill.json"), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}

	result, err := callMCPTool("meshclaw_legacy_skills_audit", map[string]interface{}{"path": dir})
	if err != nil {
		t.Fatalf("callMCPTool error = %v", err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	report, ok := payload["report"].(legacyskills.AuditReport)
	if !ok {
		t.Fatalf("report type = %T", payload["report"])
	}
	if report.Kind != "meshclaw_legacy_skills_audit" {
		t.Fatalf("kind = %q", report.Kind)
	}
	if report.ApprovalNote == "" || !strings.Contains(report.ApprovalNote, "Read-only") {
		t.Fatalf("missing read-only approval note: %#v", report)
	}
	if report.TotalSkills != 1 || report.AlreadyCovered != 1 {
		t.Fatalf("unexpected audit report: %#v", report)
	}
}

func TestMCPSignalCallToolsAreApprovalGated(t *testing.T) {
	tools := map[string]mcpTool{}
	for _, tool := range mcpTools() {
		tools[tool.Name] = tool
	}
	if _, ok := tools["meshclaw_signal_call_doctor"]; !ok {
		t.Fatal("missing meshclaw_signal_call_doctor")
	}
	callTool, ok := tools["meshclaw_signal_call"]
	if !ok {
		t.Fatal("missing meshclaw_signal_call")
	}
	if !strings.Contains(callTool.Description, "execute=true") || !strings.Contains(callTool.Description, "approve=true") {
		t.Fatalf("signal call tool should advertise approval gate: %q", callTool.Description)
	}
	result, err := callMCPTool("meshclaw_signal_call", map[string]interface{}{
		"target":  "missing-target",
		"audio":   "/tmp/missing-audio.aiff",
		"execute": false,
	})
	if err != nil {
		t.Fatalf("signal call MCP wrapper should return structured dry-run result, got error: %v", err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	if payload["result"] == nil {
		t.Fatalf("missing structured signal call result: %#v", payload)
	}
	if fmt.Sprint(payload["error"]) == "" {
		t.Fatalf("missing preflight error for invalid target/audio: %#v", payload)
	}
}

func TestModelConfigStatusMCPDoesNotExposeRawSecret(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "matrix_ai.json")
	rawSecret := "test-raw-secret-value-1234567890"
	if err := aichat.SaveConfig(configPath, aichat.Config{
		BaseURL:     "https://gateway.example/v1",
		APIKey:      rawSecret,
		Model:       "gpt-4.1",
		MaxTokens:   2048,
		Temperature: 0.2,
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MESHCLAW_MATRIX_AI_CONFIG", configPath)

	result, err := callMCPTool("meshclaw_model_config_status", map[string]interface{}{})
	if err != nil {
		t.Fatalf("callMCPTool error = %v", err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	if payload["kind"] != "meshclaw_model_config" {
		t.Fatalf("kind=%#v", payload["kind"])
	}
	if payload["secret_policy"] == "" {
		t.Fatalf("missing secret policy: %#v", payload)
	}
	key, _ := payload["api_key"].(string)
	if strings.Contains(key, rawSecret) || strings.Contains(key, "raw-secret") || !strings.Contains(key, "...") {
		t.Fatalf("api key should be masked, got %q", key)
	}
}

func TestMCPSurfaceIncludesCatalog(t *testing.T) {
	guide := mcpSurfaceGuide()
	defaultTools, ok := guide["default_tools"].([]string)
	if !ok {
		t.Fatalf("default_tools type = %T", guide["default_tools"])
	}
	for _, name := range defaultTools {
		if name == "meshclaw_mcp_catalog" || name == "meshclaw_setup_assistant" {
			continue
		}
	}
	if !containsString(defaultTools, "meshclaw_mcp_catalog") {
		t.Fatal("default_tools missing meshclaw_mcp_catalog")
	}
	if !containsString(defaultTools, "meshclaw_setup_assistant") {
		t.Fatal("default_tools missing meshclaw_setup_assistant")
	}
}

func containerSelfHealPlanningToolNames() []string {
	return []string{
		"meshclaw_autoheal_container_apply_plan",
		"meshclaw_autoheal_container_verification_plan",
		"meshclaw_autoheal_container_runbook",
		"meshclaw_autoheal_container_runbook_check",
		"meshclaw_autoheal_container_rollback_plan",
		"meshclaw_autoheal_container_completion_plan",
		"meshclaw_autoheal_container_readiness_summary",
	}
}

func containerSelfHealExecutionToolNames() []string {
	return []string{
		"meshclaw_autoheal_container_executor_gate",
		"meshclaw_autoheal_container_executor",
		"meshclaw_autoheal_container_executor_verify",
	}
}

func desiredStateReconcilePlanningToolNames() []string {
	return []string{
		"meshclaw_reconcile_validate_desired",
		"meshclaw_reconcile_plan",
		"meshclaw_reconcile_approval_request",
		"meshclaw_reconcile_apply_gate",
		"meshclaw_reconcile_apply_plan",
		"meshclaw_reconcile_execution_preview",
		"meshclaw_reconcile_verification_plan",
		"meshclaw_reconcile_runbook",
		"meshclaw_reconcile_runbook_check",
		"meshclaw_reconcile_rollback_plan",
		"meshclaw_reconcile_completion_plan",
		"meshclaw_reconcile_readiness_summary",
	}
}

func catalogCategoryTools(payload map[string]interface{}, id string) []string {
	categories, _ := payload["categories"].([]map[string]interface{})
	for _, category := range categories {
		if category["id"] != id {
			continue
		}
		tools, _ := category["tools"].([]string)
		return tools
	}
	return nil
}

func catalogProfileDefaultTools(payload map[string]interface{}, id string) []string {
	profiles, _ := payload["profiles"].(map[string]interface{})
	profile, _ := profiles[id].(map[string]interface{})
	tools, _ := profile["default_tools"].([]string)
	return tools
}

func mcpToolNames(tools []mcpTool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Name)
	}
	return names
}
