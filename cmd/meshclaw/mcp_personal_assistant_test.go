package main

import (
	"strings"
	"testing"
)

func TestMCPToolsExposePersonalAssistantTools(t *testing.T) {
	tools := map[string]mcpTool{}
	for _, tool := range mcpTools() {
		tools[tool.Name] = tool
	}
	for _, name := range []string{
		"meshclaw_calendar_list",
		"meshclaw_calendar_create_event",
		"meshclaw_reminders_list",
		"meshclaw_reminder_create",
		"meshclaw_reminder_complete",
		"meshclaw_reminder_delete",
		"meshclaw_contacts_search",
		"meshclaw_notes_search",
		"meshclaw_note_create",
		"meshclaw_automation_open_file",
		"meshclaw_notification_show",
		"meshclaw_audio_transcribe",
		"meshclaw_maps_search",
		"meshclaw_maps_directions",
		"meshclaw_maps_proof",
	} {
		tool, ok := tools[name]
		if !ok {
			t.Fatalf("missing MCP tool %s", name)
		}
		if tool.Description == "" {
			t.Fatalf("%s has empty description", name)
		}
	}
	for _, name := range []string{"meshclaw_calendar_create_event", "meshclaw_reminder_create", "meshclaw_note_create", "meshclaw_automation_open_file", "meshclaw_notification_show"} {
		execute, ok := tools[name].InputSchema.Properties["execute"]
		if !ok {
			t.Fatalf("%s should expose execute", name)
		}
		if execute.Default != false {
			t.Fatalf("%s execute default=%v, want false", name, execute.Default)
		}
	}
}

func TestMCPPersonalAssistantMutationsDefaultToPlanOnly(t *testing.T) {
	result, err := callMCPTool("meshclaw_reminder_create", map[string]interface{}{"title": "테스트 알림"})
	if err != nil {
		t.Fatalf("callMCPTool error = %v", err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	plan, ok := payload["plan"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing plan: %#v", payload)
	}
	if plan["action"] != "reminder_create" || plan["execute"] != false || plan["approval_required"] != true {
		t.Fatalf("unexpected plan: %#v", plan)
	}
}

func TestMCPSurfaceIncludesPersonalAssistantDefaults(t *testing.T) {
	guide := mcpSurfaceGuide()
	defaultTools, ok := guide["default_tools"].([]string)
	if !ok {
		t.Fatalf("default_tools type = %T", guide["default_tools"])
	}
	seen := map[string]bool{}
	for _, name := range defaultTools {
		seen[name] = true
	}
	for _, name := range []string{"meshclaw_calendar_list", "meshclaw_reminder_create", "meshclaw_contacts_search", "meshclaw_notes_search", "meshclaw_audio_transcribe", "meshclaw_maps_search", "meshclaw_maps_directions", "meshclaw_maps_proof"} {
		if !seen[name] {
			t.Fatalf("default_tools missing %s", name)
		}
	}
}

func TestMCPMapsProofRequiresApprovalBeforeCapture(t *testing.T) {
	result, err := callMCPTool("meshclaw_maps_proof", map[string]interface{}{"origin": "강남역", "destination": "서울역", "mode": "transit", "provider": "google", "execute": true})
	if err != nil {
		t.Fatalf("callMCPTool error = %v", err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	proof, ok := payload["proof"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing proof payload: %#v", payload)
	}
	if proof["status"] != "approval_required" || proof["approval_missing"] != true {
		t.Fatalf("maps proof should require approval before capture: %#v", proof)
	}
	if proof["url"] == "" {
		t.Fatalf("maps proof should still return URL: %#v", proof)
	}
	handoff, ok := proof["map_handoff"].(map[string]interface{})
	if !ok || handoff["still_proof_tool"] != "meshclaw_screen_capture" {
		t.Fatalf("missing map handoff in proof: %#v", proof)
	}
}

func TestMCPMapsReturnHandoffGuidance(t *testing.T) {
	result, err := callMCPTool("meshclaw_maps_directions", map[string]interface{}{"origin": "강남역", "destination": "서울역", "mode": "transit", "provider": "google"})
	if err != nil {
		t.Fatalf("callMCPTool error = %v", err)
	}
	payload, ok := result.(map[string]interface{})
	if !ok {
		t.Fatalf("result type = %T", result)
	}
	handoff, ok := payload["map_handoff"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing map_handoff: %#v", payload)
	}
	if handoff["still_proof_tool"] != "meshclaw_screen_capture" || handoff["video_proof_tool"] != "meshclaw_screen_record" {
		t.Fatalf("missing proof tool guidance: %#v", handoff)
	}
	if handoff["approval_required"] != true || !strings.Contains(handoff["approval_note"].(string), "approve=true") {
		t.Fatalf("missing approval boundary: %#v", handoff)
	}
	if !strings.Contains(handoff["reply_rule"].(string), "clickable map URL") {
		t.Fatalf("missing clickable URL reply rule: %#v", handoff)
	}
}
