package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMCPToolsExposeVisibleWorkTools(t *testing.T) {
	tools := map[string]mcpTool{}
	for _, tool := range mcpTools() {
		tools[tool.Name] = tool
	}
	for _, name := range []string{
		"meshclaw_visible_browser_search",
		"meshclaw_document_create",
		"meshclaw_document_export",
		"meshclaw_spreadsheet_create",
		"meshclaw_presentation_create",
		"meshclaw_presentation_verify",
		"meshclaw_presentation_edit",
		"meshclaw_presentation_export",
		"meshclaw_screen_capture",
		"meshclaw_screen_record",
		"meshclaw_media_play",
		"meshclaw_audio_transcribe",
		"meshclaw_app_settings_plan",
		"meshclaw_account_action_plan",
		"meshclaw_purchase_click",
		"meshclaw_terminal_run",
		"meshclaw_shortcut_text_run",
		"meshclaw_result_save",
	} {
		tool, ok := tools[name]
		if !ok {
			t.Fatalf("missing MCP tool %s", name)
		}
		if tool.Description == "" {
			t.Fatalf("%s has empty description", name)
		}
		if name == "meshclaw_presentation_verify" {
			continue
		}
		execute, ok := tool.InputSchema.Properties["execute"]
		if !ok {
			t.Fatalf("%s should expose execute", name)
		}
		if execute.Default != false {
			t.Fatalf("%s execute default=%v, want false", name, execute.Default)
		}
	}
}

func TestMCPVisibleWorkMutationsDefaultToPlanOnly(t *testing.T) {
	fakeVisibleWorkWhisperInPath(t)
	for _, tc := range []struct {
		name string
		args map[string]interface{}
		want string
	}{
		{name: "meshclaw_visible_browser_search", args: map[string]interface{}{"query": "MeshClaw"}, want: "visible_browser_search"},
		{name: "meshclaw_document_create", args: map[string]interface{}{"body": "테스트 문서"}, want: "document_create"},
		{name: "meshclaw_document_export", args: map[string]interface{}{"input": "/tmp/example.md"}, want: "document_export"},
		{name: "meshclaw_spreadsheet_create", args: map[string]interface{}{"body": "예산표"}, want: "spreadsheet_create"},
		{name: "meshclaw_presentation_create", args: map[string]interface{}{"body": "발표자료 내용"}, want: "presentation_create"},
		{name: "meshclaw_presentation_edit", args: map[string]interface{}{"input": "/tmp/example.pptx", "body": "추가 내용"}, want: "presentation_edit"},
		{name: "meshclaw_presentation_export", args: map[string]interface{}{"input": "/tmp/example.pptx"}, want: "presentation_export"},
		{name: "meshclaw_screen_capture", args: map[string]interface{}{"purpose": "map still proof"}, want: "screen_capture"},
		{name: "meshclaw_screen_record", args: map[string]interface{}{"seconds": 1, "purpose": "map route proof"}, want: "screen_record"},
		{name: "meshclaw_media_play", args: map[string]interface{}{"query": "집중 재즈", "source": "youtube"}, want: "media_play"},
		{name: "meshclaw_audio_transcribe", args: map[string]interface{}{"path": fakeVisibleWorkFile(t, "voice.mp3")}, want: "audio_transcribe"},
		{name: "meshclaw_app_settings_plan", args: map[string]interface{}{"app": "System Settings", "pane": "full_disk_access"}, want: "app_settings_preflight"},
		{name: "meshclaw_account_action_plan", args: map[string]interface{}{"service": "Netflix", "action": "cancel"}, want: "account_action_preflight"},
		{name: "meshclaw_purchase_click", args: map[string]interface{}{"merchant": "Coupang", "item": "생수", "total": "12,000원"}, want: "purchase_click"},
		{name: "meshclaw_terminal_run", args: map[string]interface{}{"command": "date"}, want: "terminal_run"},
		{name: "meshclaw_shortcut_text_run", args: map[string]interface{}{"name": "Argos Shortcut", "input": "테스트"}, want: "shortcut_text_run"},
		{name: "meshclaw_result_save", args: map[string]interface{}{"body": "작업 결과"}, want: "result_save"},
	} {
		result, err := callMCPTool(tc.name, tc.args)
		if err != nil {
			t.Fatalf("%s callMCPTool error = %v", tc.name, err)
		}
		payload, ok := result.(map[string]interface{})
		if !ok {
			t.Fatalf("%s result type = %T", tc.name, result)
		}
		plan, ok := payload["plan"].(map[string]interface{})
		if !ok {
			t.Fatalf("%s missing plan: %#v", tc.name, payload)
		}
		if plan["action"] != tc.want || plan["execute"] != false || plan["approval_required"] != true {
			t.Fatalf("%s unexpected plan: %#v", tc.name, plan)
		}
		if tc.name == "meshclaw_screen_record" {
			if plan["kind"] != "meshclaw_screen_proof_plan" {
				t.Fatalf("screen record should return screen proof plan: %#v", plan)
			}
			if !strings.Contains(plan["user_message"].(string), "민감정보") {
				t.Fatalf("screen proof plan missing privacy user message: %#v", plan)
			}
			if plan["purpose"] != "map route proof" {
				t.Fatalf("screen proof plan missing purpose: %#v", plan)
			}
		}
		if tc.name == "meshclaw_screen_capture" {
			if plan["kind"] != "meshclaw_screen_capture_plan" {
				t.Fatalf("screen capture should return screen capture plan: %#v", plan)
			}
			if !strings.Contains(plan["user_message"].(string), "민감정보") {
				t.Fatalf("screen capture plan missing privacy user message: %#v", plan)
			}
			if plan["purpose"] != "map still proof" {
				t.Fatalf("screen capture plan missing purpose: %#v", plan)
			}
			if !strings.HasSuffix(plan["output"].(string), ".png") {
				t.Fatalf("screen capture plan should default to png output: %#v", plan)
			}
		}
		if tc.name == "meshclaw_media_play" {
			if plan["kind"] != "meshclaw_media_play_plan" {
				t.Fatalf("media play should return media plan: %#v", plan)
			}
			if !strings.Contains(plan["user_message"].(string), "재생") {
				t.Fatalf("media plan missing playback user message: %#v", plan)
			}
			if !strings.Contains(plan["target"].(string), "youtube.com/results") {
				t.Fatalf("media plan target should be YouTube search: %#v", plan)
			}
		}
		if tc.name == "meshclaw_audio_transcribe" {
			if plan["kind"] != "meshclaw_audio_transcribe" {
				t.Fatalf("audio transcribe should return audio plan: %#v", plan)
			}
			if !strings.Contains(plan["approval_note"].(string), "approve=true") {
				t.Fatalf("audio transcribe plan missing approval guidance: %#v", plan)
			}
		}
		if tc.name == "meshclaw_app_settings_plan" {
			if plan["kind"] != "meshclaw_app_settings_plan" {
				t.Fatalf("app settings should return settings plan: %#v", plan)
			}
			if !strings.Contains(plan["user_message"].(string), "토글") {
				t.Fatalf("app settings plan missing toggle guidance: %#v", plan)
			}
			if !strings.Contains(plan["target"].(string), "Privacy_AllFiles") {
				t.Fatalf("app settings plan should target full disk access: %#v", plan)
			}
		}
		if tc.name == "meshclaw_account_action_plan" {
			if plan["kind"] != "meshclaw_account_action_plan" {
				t.Fatalf("account action should return account plan: %#v", plan)
			}
			if !strings.Contains(plan["user_message"].(string), "마지막 단계") {
				t.Fatalf("account plan missing stop-before-final guidance: %#v", plan)
			}
			stops, ok := plan["stop_before"].([]string)
			if !ok || len(stops) == 0 || !visibleWorkContainsString(stops, "final cancellation") {
				t.Fatalf("account plan missing cancellation boundary: %#v", plan)
			}
		}
		if tc.name == "meshclaw_purchase_click" {
			if plan["kind"] != "meshclaw_purchase_click_plan" {
				t.Fatalf("purchase click should return purchase plan: %#v", plan)
			}
			if plan["strong_approval"] != true || !strings.Contains(plan["approval_note"].(string), "구매 실행 승인") {
				t.Fatalf("purchase plan missing strong approval boundary: %#v", plan)
			}
		}
	}
}

func TestMCPAudioTranscribeExecutionRequiresApproval(t *testing.T) {
	fakeVisibleWorkWhisperInPath(t)
	result, err := callMCPTool("meshclaw_audio_transcribe", map[string]interface{}{"path": fakeVisibleWorkFile(t, "voice.mp3"), "execute": true})
	if err != nil {
		t.Fatalf("audio transcribe should return structured approval-required result, got error: %v", err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	plan, ok := payload["plan"].(map[string]interface{})
	if !ok || plan["approval_missing"] != true || plan["status"] != "approval_required" {
		t.Fatalf("missing approval-required plan: %#v", payload)
	}
}

func TestMCPMediaPlayExecutionRequiresApproval(t *testing.T) {
	result, err := callMCPTool("meshclaw_media_play", map[string]interface{}{"query": "집중 재즈", "source": "youtube", "execute": true})
	if err != nil {
		t.Fatalf("media play should return structured approval-required result, got error: %v", err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	if payload["error"] == "" {
		t.Fatalf("missing approval error: %#v", payload)
	}
	plan, ok := payload["plan"].(map[string]interface{})
	if !ok || plan["approval_missing"] != true {
		t.Fatalf("missing approval-required plan: %#v", payload)
	}
}

func TestMCPScreenCaptureExecutionRequiresApproval(t *testing.T) {
	result, err := callMCPTool("meshclaw_screen_capture", map[string]interface{}{"purpose": "map still proof", "execute": true})
	if err != nil {
		t.Fatalf("screen capture should return structured approval-required result, got error: %v", err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	if payload["error"] == "" {
		t.Fatalf("missing approval error: %#v", payload)
	}
	plan, ok := payload["plan"].(map[string]interface{})
	if !ok || plan["approval_missing"] != true {
		t.Fatalf("missing approval-required plan: %#v", payload)
	}
}

func TestMCPAccountActionExecutionRequiresApproval(t *testing.T) {
	result, err := callMCPTool("meshclaw_account_action_plan", map[string]interface{}{"service": "Netflix", "action": "cancel", "execute": true})
	if err != nil {
		t.Fatalf("account action should return structured approval-required result, got error: %v", err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	if payload["error"] == "" {
		t.Fatalf("missing approval error: %#v", payload)
	}
	plan, ok := payload["plan"].(map[string]interface{})
	if !ok || plan["approval_missing"] != true {
		t.Fatalf("missing approval-required plan: %#v", payload)
	}
}

func TestMCPPurchaseClickExecutionRequiresStrongApproval(t *testing.T) {
	result, err := callMCPTool("meshclaw_purchase_click", map[string]interface{}{
		"merchant":            "Coupang",
		"item":                "생수",
		"total":               "12,000원",
		"execute":             true,
		"post_capture":        true,
		"post_capture_output": "/tmp/purchase-proof.png",
	})
	if err != nil {
		t.Fatalf("purchase click should return structured approval-required result, got error: %v", err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	purchase, ok := payload["purchase"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing purchase payload: %#v", payload)
	}
	if purchase["approval_missing"] != true || purchase["status"] != "approval_required" {
		t.Fatalf("missing approval-required purchase plan: %#v", purchase)
	}
	if purchase["execute"] != true || purchase["post_capture"] != true || purchase["post_capture_output"] != "/tmp/purchase-proof.png" {
		t.Fatalf("purchase plan should preserve requested execution/proof fields: %#v", purchase)
	}
	missing, ok := purchase["missing"].([]string)
	if !ok || !visibleWorkContainsString(missing, "confirmation") || !visibleWorkContainsString(missing, "shipping") {
		t.Fatalf("purchase plan should list missing required fields: %#v", purchase)
	}
}

func TestMCPPurchaseClickAcceptsShortApprovalAndClicksRunner(t *testing.T) {
	clicked := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/click" {
			t.Fatalf("unexpected runner path: %s", r.URL.Path)
		}
		clicked = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	result, err := callMCPTool("meshclaw_purchase_click", map[string]interface{}{
		"merchant":     "Coupang",
		"item":         "생수",
		"total":        "12,000원",
		"shipping":     "맞음",
		"payment":      "맞음",
		"url":          "https://www.coupang.com/checkout/order",
		"x":            float64(456),
		"y":            float64(789),
		"runner_url":   server.URL,
		"confirmation": "ㅇㅇ",
		"execute":      true,
		"approve":      true,
	})
	if err != nil {
		t.Fatalf("purchase click should accept short approval, got error: %v", err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	purchase, ok := payload["purchase"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing purchase payload: %#v", payload)
	}
	if !clicked || purchase["status"] != "clicked" {
		t.Fatalf("purchase click should call runner and mark clicked: clicked=%t payload=%#v", clicked, purchase)
	}
	if purchase["approval_missing"] == true {
		t.Fatalf("short approval should not be treated as missing approval: %#v", purchase)
	}
}

func TestMCPAppSettingsExecutionRequiresApproval(t *testing.T) {
	result, err := callMCPTool("meshclaw_app_settings_plan", map[string]interface{}{"app": "System Settings", "pane": "accessibility", "execute": true})
	if err != nil {
		t.Fatalf("app settings should return structured approval-required result, got error: %v", err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	if payload["error"] == "" {
		t.Fatalf("missing approval error: %#v", payload)
	}
	plan, ok := payload["plan"].(map[string]interface{})
	if !ok || plan["approval_missing"] != true {
		t.Fatalf("missing approval-required plan: %#v", payload)
	}
}

func visibleWorkContainsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestMCPSurfaceIncludesVisibleWorkDefaults(t *testing.T) {
	guide := mcpSurfaceGuide()
	defaultTools, ok := guide["default_tools"].([]string)
	if !ok {
		t.Fatalf("default_tools type = %T", guide["default_tools"])
	}
	seen := map[string]bool{}
	for _, name := range defaultTools {
		seen[name] = true
	}
	for _, name := range []string{"meshclaw_visible_browser_search", "meshclaw_document_create", "meshclaw_document_export", "meshclaw_spreadsheet_create", "meshclaw_presentation_create", "meshclaw_presentation_verify", "meshclaw_presentation_edit", "meshclaw_presentation_export", "meshclaw_screen_capture", "meshclaw_screen_record", "meshclaw_media_play", "meshclaw_audio_transcribe", "meshclaw_app_settings_plan", "meshclaw_account_action_plan", "meshclaw_purchase_click", "meshclaw_terminal_run", "meshclaw_shortcut_text_run", "meshclaw_result_save"} {
		if !seen[name] {
			t.Fatalf("default_tools missing %s", name)
		}
	}
}

func fakeVisibleWorkFile(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("fake"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func fakeVisibleWorkWhisperInPath(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "whisper")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 0\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
}
