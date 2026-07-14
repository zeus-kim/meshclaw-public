package main

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/meshclaw/meshclaw/internal/assistantbrief"
	"github.com/meshclaw/meshclaw/internal/audio"
	"github.com/meshclaw/meshclaw/internal/browserauto"
	"github.com/meshclaw/meshclaw/internal/capability"
	"github.com/meshclaw/meshclaw/internal/datadoctor"
	"github.com/meshclaw/meshclaw/internal/doctor"
	"github.com/meshclaw/meshclaw/internal/evidence"
	"github.com/meshclaw/meshclaw/internal/fileorg"
	"github.com/meshclaw/meshclaw/internal/fleet"
	"github.com/meshclaw/meshclaw/internal/guard"
	"github.com/meshclaw/meshclaw/internal/guardcode"
	"github.com/meshclaw/meshclaw/internal/guardcve"
	"github.com/meshclaw/meshclaw/internal/guardvault"
	"github.com/meshclaw/meshclaw/internal/hygiene"
	"github.com/meshclaw/meshclaw/internal/inventory"
	"github.com/meshclaw/meshclaw/internal/legacyskills"
	"github.com/meshclaw/meshclaw/internal/mailadapter"
	"github.com/meshclaw/meshclaw/internal/messenger"
	"github.com/meshclaw/meshclaw/internal/mission"
	"github.com/meshclaw/meshclaw/internal/monitor"
	"github.com/meshclaw/meshclaw/internal/opsdb"
	"github.com/meshclaw/meshclaw/internal/osauto"
	"github.com/meshclaw/meshclaw/internal/policy"
	"github.com/meshclaw/meshclaw/internal/prometheus"
	"github.com/meshclaw/meshclaw/internal/provision"
	"github.com/meshclaw/meshclaw/internal/publish"
	"github.com/meshclaw/meshclaw/internal/reconciler"
	"github.com/meshclaw/meshclaw/internal/router"
	"github.com/meshclaw/meshclaw/internal/runtime"
	"github.com/meshclaw/meshclaw/internal/runtimeflow"
	"github.com/meshclaw/meshclaw/internal/scheduler"
	"github.com/meshclaw/meshclaw/internal/workers"
	"github.com/meshclaw/meshclaw/internal/workflow"
	"github.com/meshclaw/meshclaw/internal/workspace"
)

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
	ID      interface{}     `json:"id,omitempty"`
}

type mcpResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	Result  interface{} `json:"result,omitempty"`
	Error   *mcpError   `json:"error,omitempty"`
	ID      interface{} `json:"id,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type mcpTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema mcpInputSchema `json:"inputSchema"`
}

type mcpInputSchema struct {
	Type       string                 `json:"type"`
	Properties map[string]mcpProperty `json:"properties"`
	Required   []string               `json:"required,omitempty"`
}

type mcpProperty struct {
	Type        string      `json:"type"`
	Description string      `json:"description"`
	Default     interface{} `json:"default,omitempty"`
}

func parseMCPProfile(args []string) string {
	profile := strings.TrimSpace(os.Getenv("MESHCLAW_MCP_PROFILE"))
	for i := 0; i < len(args); i++ {
		arg := strings.TrimSpace(args[i])
		switch {
		case arg == "--profile" || arg == "-profile":
			if i+1 < len(args) {
				profile = args[i+1]
				i++
			}
		case strings.HasPrefix(arg, "--profile="):
			profile = strings.TrimPrefix(arg, "--profile=")
		case arg == "--lite":
			profile = "claude-lite"
		case arg == "--full":
			profile = "all"
		}
	}
	return normalizeMCPProfile(profile)
}

func normalizeMCPProfile(profile string) string {
	profile = strings.ToLower(strings.TrimSpace(profile))
	switch profile {
	case "", "all", "full", "default":
		return "all"
	case "lite", "claude", "claude-lite":
		return "claude-lite"
	case "local", "local-lite", "personal-lite":
		return "local-lite"
	default:
		return profile
	}
}

func cmdMCP(profile string) error {
	profile = normalizeMCPProfile(profile)
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}
		var req mcpRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			continue
		}
		if req.ID == nil {
			continue
		}
		resp := handleMCPWithProfile(req, profile)
		out, _ := json.Marshal(resp)
		fmt.Println(string(out))
	}
	return scanner.Err()
}

func handleMCP(req mcpRequest) mcpResponse {
	return handleMCPWithProfile(req, "all")
}

func handleMCPWithProfile(req mcpRequest, profile string) mcpResponse {
	profile = normalizeMCPProfile(profile)
	switch req.Method {
	case "initialize":
		return mcpResponse{
			JSONRPC: "2.0",
			Result: map[string]interface{}{
				"protocolVersion": mcpProtocolVersion(req.Params),
				"capabilities":    map[string]interface{}{"tools": map[string]interface{}{}},
				"serverInfo":      map[string]string{"name": "meshclaw-mcp", "version": meshclawVersion},
			},
			ID: req.ID,
		}
	case "tools/list":
		return mcpResponse{JSONRPC: "2.0", Result: map[string]interface{}{"tools": mcpToolsForProfile(profile)}, ID: req.ID}
	case "tools/call":
		var params struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return mcpResponse{JSONRPC: "2.0", Error: &mcpError{Code: -32602, Message: err.Error()}, ID: req.ID}
		}
		if !mcpProfileAllowsTool(profile, params.Name) {
			return mcpResponse{JSONRPC: "2.0", Error: &mcpError{Code: -32601, Message: fmt.Sprintf("tool %q is not available in MCP profile %q", params.Name, profile)}, ID: req.ID}
		}
		result, err := callMCPTool(params.Name, params.Arguments)
		if err != nil {
			return mcpResponse{JSONRPC: "2.0", Error: &mcpError{Code: -32000, Message: err.Error()}, ID: req.ID}
		}
		data, _ := json.MarshalIndent(result, "", "  ")
		return mcpResponse{
			JSONRPC: "2.0",
			Result: map[string]interface{}{
				"content": []map[string]string{{"type": "text", "text": string(data)}},
			},
			ID: req.ID,
		}
	default:
		return mcpResponse{JSONRPC: "2.0", Error: &mcpError{Code: -32601, Message: "unknown method: " + req.Method}, ID: req.ID}
	}
}

func mcpProtocolVersion(params json.RawMessage) string {
	const latest = "2025-11-25"
	var initParams struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if len(params) > 0 {
		_ = json.Unmarshal(params, &initParams)
	}
	switch strings.TrimSpace(initParams.ProtocolVersion) {
	case "2025-11-25", "2024-11-05":
		return strings.TrimSpace(initParams.ProtocolVersion)
	default:
		return latest
	}
}

func mcpToolsForProfile(profile string) []mcpTool {
	profile = normalizeMCPProfile(profile)
	if profile == "all" {
		return mcpTools()
	}
	allowed := mcpProfileToolNames(profile)
	if len(allowed) == 0 {
		return mcpTools()
	}
	byName := map[string]mcpTool{}
	for _, tool := range mcpTools() {
		byName[tool.Name] = tool
	}
	out := []mcpTool{}
	for _, name := range allowed {
		if tool, ok := byName[name]; ok {
			out = append(out, compactMCPToolForProfile(tool, profile))
		}
	}
	return out
}

func mcpProfileAllowsTool(profile, name string) bool {
	profile = normalizeMCPProfile(profile)
	if profile == "all" {
		return true
	}
	allowed := mcpProfileToolNames(profile)
	if len(allowed) == 0 {
		return true
	}
	for _, allowedName := range allowed {
		if name == allowedName {
			return true
		}
	}
	return false
}

func mcpProfileToolNames(profile string) []string {
	switch normalizeMCPProfile(profile) {
	case "local-lite":
		return []string{
			"meshclaw_local_assistant",
			"meshclaw_mcp_catalog",
			"meshclaw_tool_recommend",
			"meshclaw_setup_assistant",
			"meshclaw_argos_macos_doctor",
			"meshclaw_schedule_status",
			"meshclaw_mail_accounts",
			"meshclaw_mail_summarize",
			"meshclaw_mail_search",
			"meshclaw_mail_thread",
			"meshclaw_mail_draft_reply",
			"meshclaw_calendar_list",
			"meshclaw_calendar_create_event",
			"meshclaw_reminders_list",
			"meshclaw_reminder_create",
			"meshclaw_contacts_search",
			"meshclaw_notes_search",
			"meshclaw_note_create",
			"meshclaw_document_create",
			"meshclaw_maps_search",
			"meshclaw_maps_directions",
			"meshclaw_data_archive_plan",
			"meshclaw_downloads_cleanup_plan",
			"meshclaw_result_save",
			"meshclaw_argos_ask",
		}
	case "claude-lite":
		return []string{
			"meshclaw_local_assistant",
			"meshclaw_mcp_surface",
			"meshclaw_mcp_catalog",
			"meshclaw_mcp_profile_visibility_check",
			"meshclaw_mcp_rollout_plan",
			"meshclaw_mcp_smoke_test_plan",
			"meshclaw_tool_recommend",
			"meshclaw_setup_assistant",
			"meshclaw_setup_signal",
			"meshclaw_argos_macos_doctor",
			"meshclaw_schedule_status",
			"meshclaw_schedule_run_once",
			"meshclaw_messenger_targets",
			"meshclaw_scheduled_delivery_plan",
			"meshclaw_scheduled_delivery_apply",
			"meshclaw_mail_accounts",
			"meshclaw_mail_summarize",
			"meshclaw_mail_search",
			"meshclaw_mail_thread",
			"meshclaw_mail_draft_reply",
			"meshclaw_mail_compose",
			"meshclaw_visible_browser_search",
			"meshclaw_browser_fetch",
			"meshclaw_argos_research",
			"meshclaw_document_create",
			"meshclaw_document_export",
			"meshclaw_spreadsheet_create",
			"meshclaw_presentation_create",
			"meshclaw_presentation_verify",
			"meshclaw_screen_capture",
			"meshclaw_maps_search",
			"meshclaw_maps_directions",
			"meshclaw_maps_proof",
			"meshclaw_calendar_list",
			"meshclaw_calendar_create_event",
			"meshclaw_reminders_list",
			"meshclaw_reminder_create",
			"meshclaw_contacts_search",
			"meshclaw_notes_search",
			"meshclaw_note_create",
			"meshclaw_automation_open_app",
			"meshclaw_media_play",
			"meshclaw_app_settings_plan",
			"meshclaw_account_action_plan",
			"meshclaw_automation_rule_plan",
			"meshclaw_automation_rule_check",
			"meshclaw_automation_rule_readiness_summary",
			"meshclaw_automation_rule_writer_plan",
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
			"meshclaw_reconcile_verification_plan",
			"meshclaw_reconcile_runbook",
			"meshclaw_reconcile_runbook_check",
			"meshclaw_reconcile_rollback_plan",
			"meshclaw_reconcile_completion_plan",
			"meshclaw_reconcile_readiness_summary",
			"meshclaw_autoheal_plan",
			"meshclaw_autoheal_container_apply_plan",
			"meshclaw_autoheal_container_verification_plan",
			"meshclaw_autoheal_container_runbook",
			"meshclaw_autoheal_container_runbook_check",
			"meshclaw_autoheal_container_rollback_plan",
			"meshclaw_autoheal_container_completion_plan",
			"meshclaw_autoheal_container_readiness_summary",
			"meshclaw_autoheal_container_executor_gate",
			"meshclaw_autoheal_container_executor",
			"meshclaw_analyze_logs",
			"meshclaw_downloads_cleanup_plan",
			"meshclaw_data_archive_plan",
			"meshclaw_data_doctor",
			"meshclaw_result_save",
			"meshclaw_argos_ask",
			"meshclaw_util_crypto_price",
			"meshclaw_util_weather",
			"meshclaw_util_convert",
			"meshclaw_util_generate",
		}
	default:
		return nil
	}
}

func compactMCPToolForProfile(tool mcpTool, profile string) mcpTool {
	profile = normalizeMCPProfile(profile)
	if profile != "claude-lite" && profile != "local-lite" {
		return tool
	}
	if desc, ok := mcpClaudeLiteDescriptions()[tool.Name]; ok {
		tool.Description = desc
	} else {
		tool.Description = firstSentence(tool.Description, 140)
	}
	descLimit := 180
	propLimit := 90
	if profile == "local-lite" {
		descLimit = 120
		propLimit = 70
		tool.Description = firstSentence(tool.Description, descLimit)
	}
	for name, prop := range tool.InputSchema.Properties {
		prop.Description = firstSentence(prop.Description, propLimit)
		tool.InputSchema.Properties[name] = prop
	}
	return tool
}

func mcpClaudeLiteDescriptions() map[string]string {
	return map[string]string{
		"meshclaw_mcp_catalog":                          "Show hidden MeshClaw tool groups and profiles when the lite surface is not enough.",
		"meshclaw_local_assistant":                      "One compact router for local-model personal assistant tasks.",
		"meshclaw_tool_recommend":                       "Pick the safest MeshClaw tool for a natural-language request.",
		"meshclaw_setup_assistant":                      "Check one-machine Argos/MeshClaw readiness.",
		"meshclaw_setup_signal":                         "Check Signal dispatcher, targets, schedule runner, and chat endpoint.",
		"meshclaw_argos_macos_doctor":                   "Check local macOS assistant readiness and permissions.",
		"meshclaw_schedule_status":                      "Show scheduled job health and next due jobs.",
		"meshclaw_schedule_run_once":                    "Run one scheduled job now; dry-run unless execute is true.",
		"meshclaw_messenger_targets":                    "List configured Signal/report targets.",
		"meshclaw_scheduled_delivery_plan":              "Plan recurring Signal delivery with preview; does not register or send.",
		"meshclaw_scheduled_delivery_apply":             "Register an approved scheduled delivery after preview approval.",
		"meshclaw_mail_accounts":                        "List configured mail accounts without secrets.",
		"meshclaw_mail_summarize":                       "Summarize recent mail without sending or mutating.",
		"meshclaw_mail_search":                          "Search mail and return redacted summaries.",
		"meshclaw_mail_thread":                          "Read one mail thread with redaction.",
		"meshclaw_mail_draft_reply":                     "Create an unsent reply draft.",
		"meshclaw_mail_compose":                         "Create an unsent email draft.",
		"meshclaw_visible_browser_search":               "Plan or run visible browser search on the local Mac.",
		"meshclaw_browser_fetch":                        "Fetch and extract a web page.",
		"meshclaw_argos_research":                       "Create a cited research report artifact.",
		"meshclaw_document_create":                      "Plan or create a local document bundle.",
		"meshclaw_document_export":                      "Export Markdown/Obsidian document to DOCX/PDF.",
		"meshclaw_spreadsheet_create":                   "Plan or create XLSX/CSV/HTML spreadsheet artifacts.",
		"meshclaw_presentation_create":                  "Plan or create PPTX presentation artifacts.",
		"meshclaw_presentation_verify":                  "Verify a PPTX artifact.",
		"meshclaw_screen_capture":                       "Plan or capture an approved screenshot proof.",
		"meshclaw_maps_search":                          "Create a map place-search URL.",
		"meshclaw_maps_directions":                      "Create a directions URL.",
		"meshclaw_maps_proof":                           "Plan or capture map proof after approval.",
		"meshclaw_calendar_list":                        "List local Calendar events.",
		"meshclaw_calendar_create_event":                "Plan or create a Calendar event.",
		"meshclaw_reminders_list":                       "List local Reminders.",
		"meshclaw_reminder_create":                      "Plan or create a Reminder.",
		"meshclaw_contacts_search":                      "Search local Contacts.",
		"meshclaw_notes_search":                         "Search local Notes.",
		"meshclaw_note_create":                          "Plan or create a Note.",
		"meshclaw_automation_open_app":                  "Open a visible macOS app.",
		"meshclaw_media_play":                           "Plan or open visible media playback.",
		"meshclaw_app_settings_plan":                    "Plan or open visible app/System Settings surface.",
		"meshclaw_account_action_plan":                  "Plan account/billing/subscription handoff without final action.",
		"meshclaw_mcp_surface":                          "List available MCP tools and guidance.",
		"meshclaw_mcp_rollout_plan":                     "Plan MCP binary/client refresh after merge.",
		"meshclaw_mcp_smoke_test_plan":                  "Plan read-only MCP smoke checks.",
		"meshclaw_service_registry_plan":                "Plan service discovery and LB-lite routing.",
		"meshclaw_capacity_scale_plan":                  "Plan capacity and autoscale guardrails.",
		"meshclaw_storage_guardrail_plan":               "Plan storage and backup safety guardrails.",
		"meshclaw_ops_integration_plan":                 "Plan ops-tool integration as semantic evidence.",
		"meshclaw_reconcile_validate_desired":           "Validate desired-state YAML before reconcile planning.",
		"meshclaw_reconcile_plan":                       "Create a dry-run desired-state reconcile plan.",
		"meshclaw_reconcile_approval_request":           "Create reconcile approval evidence without executing.",
		"meshclaw_reconcile_apply_gate":                 "Check reconcile approval evidence before future apply.",
		"meshclaw_reconcile_apply_plan":                 "Build plan-only reconcile apply steps.",
		"meshclaw_reconcile_execution_preview":          "Preview inert reconcile command templates.",
		"meshclaw_reconcile_verification_plan":          "Plan post-action reconcile verification evidence.",
		"meshclaw_reconcile_runbook":                    "Build a review-only reconcile runbook.",
		"meshclaw_reconcile_runbook_check":              "Validate reconcile runbook evidence.",
		"meshclaw_reconcile_rollback_plan":              "Build plan-only reconcile rollback guidance.",
		"meshclaw_reconcile_completion_plan":            "Build reconcile completion evidence requirements.",
		"meshclaw_reconcile_readiness_summary":          "Summarize reconcile readiness and blockers.",
		"meshclaw_autoheal_plan":                        "Convert fleet alerts into safe autoheal plan evidence.",
		"meshclaw_autoheal_container_apply_plan":        "Build plan-only container apply steps; returns container_apply_plan_contract.direct_restart_allowed=false and requires_focused_runtime_evidence=true.",
		"meshclaw_autoheal_container_verification_plan": "Plan container verification; preserves apply_plan_runtime_evidence_required_count.",
		"meshclaw_autoheal_container_runbook":           "Build a review-only runbook; preserves apply_plan_runtime_evidence_required_count.",
		"meshclaw_autoheal_container_runbook_check":     "Validate container runbook evidence.",
		"meshclaw_autoheal_container_rollback_plan":     "Build plan-only container rollback guidance.",
		"meshclaw_autoheal_container_completion_plan":   "Build container completion evidence requirements.",
		"meshclaw_autoheal_container_readiness_summary": "Summarize container self-heal readiness and blockers.",
		"meshclaw_autoheal_container_executor_gate":     "Check approval/dry-run gates before any container executor.",
		"meshclaw_autoheal_container_executor":          "Dry-run or approval-gated live container restart executor.",
		"meshclaw_autoheal_container_executor_verify":   "Verify post-action evidence before closing self-heal.",
		"meshclaw_analyze_logs":                         "Collect logs; use source=container:<name>, check runtime_evidence_checklist, and confirm unit identity before systemd restart planning.",
		"meshclaw_downloads_cleanup_plan":               "Plan Downloads cleanup without moving or deleting.",
		"meshclaw_data_archive_plan":                    "Plan evidence archival without writing or deleting.",
		"meshclaw_data_doctor":                          "Check MeshClaw local data growth and retention state.",
		"meshclaw_result_save":                          "Save a durable Markdown/HTML result artifact.",
		"meshclaw_argos_ask":                            "Fallback natural-language macOS assistant request when no first-class tool fits.",
	}
}

func firstSentence(text string, limit int) string {
	text = strings.TrimSpace(text)
	if text == "" || limit <= 0 {
		return text
	}
	cut := len(text)
	for _, sep := range []string{". ", "; ", "\n"} {
		if idx := strings.Index(text, sep); idx >= 0 && idx+1 < cut {
			cut = idx + 1
		}
	}
	if cut > limit {
		cut = limit
	}
	out := strings.TrimSpace(text[:cut])
	if len(text) > cut {
		out = strings.TrimRight(out, " .;:,") + "..."
	}
	return out
}

func allMCPTools() []mcpTool {
	tools := []mcpTool{
		{Name: "meshclaw_ai_guide", Description: "Canonical guide for Codex, Claude, Cursor, Open WebUI, and local LLMs. Explains when to use MeshClaw workflows/policy/evidence vs direct vssh transport tools.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_architecture", Description: "Return MeshClaw's current product architecture: layers, modes, boundaries, gaps, and next build order. Use when the operator asks what MeshClaw is or how to continue development.", InputSchema: objectSchema(nil, nil)},
		{Name: "mission_get", Description: "Read one Core Mission State v0 document from the MacBook canonical store ~/.meshclaw/state/missions/{id}.json. If id is omitted, reads active.json. Read-only; Phase 1 does not write or sync macmini Signal state.", InputSchema: objectSchema(map[string]mcpProperty{
			"id": {Type: "string", Description: "Mission id, e.g. meshclaw-0.8. Omit to use active.json."},
		}, nil)},
		{Name: "mission_list", Description: "List Core Mission State v0 summaries from the MacBook canonical store ~/.meshclaw/state/missions/. Read-only; macmini Signal integration is Phase 1.5/2.", InputSchema: objectSchema(nil, nil)},
		{Name: "mission_update", Description: "Update selected fields on one MacBook canonical Core Mission State v0 document. MacBook-only write: do not call on macmini or create duplicate Signal mission state.", InputSchema: objectSchema(map[string]mcpProperty{
			"id":          {Type: "string", Description: "Mission id. Omit to use active.json."},
			"goal":        {Type: "string", Description: "Optional replacement mission goal."},
			"status":      {Type: "string", Description: "Optional status: active, blocked, done, archived."},
			"next_action": {Type: "string", Description: "Optional replacement single next action."},
		}, nil)},
		{Name: "task_add", Description: "Add a task to one MacBook canonical mission. MacBook-only write; macmini Signal sync is not part of this tool.", InputSchema: objectSchema(map[string]mcpProperty{
			"id":     {Type: "string", Description: "Mission id. Omit to use active.json."},
			"title":  {Type: "string", Description: "Task title."},
			"status": {Type: "string", Description: "Task status: pending, in_progress, done, blocked.", Default: "pending"},
			"notes":  {Type: "string", Description: "Optional task notes."},
		}, []string{"title"})},
		{Name: "task_complete", Description: "Mark a task done in one MacBook canonical mission. MacBook-only write; does not touch macmini Signal state.", InputSchema: objectSchema(map[string]mcpProperty{
			"id":      {Type: "string", Description: "Mission id. Omit to use active.json."},
			"task_id": {Type: "string", Description: "Task id to mark done, e.g. task-001."},
			"notes":   {Type: "string", Description: "Optional replacement/completion notes."},
		}, []string{"task_id"})},
		{Name: "artifact_add", Description: "Append an artifact reference to one MacBook canonical mission. Use node-qualified refs such as macmini:/path for Runtime outputs. MacBook-only write.", InputSchema: objectSchema(map[string]mcpProperty{
			"id":       {Type: "string", Description: "Mission id. Omit to use active.json."},
			"kind":     {Type: "string", Description: "Artifact kind, e.g. file, doc, link, evidence.", Default: "file"},
			"ref":      {Type: "string", Description: "Artifact reference path/URL. Prefer node-qualified refs for remote outputs."},
			"title":    {Type: "string", Description: "Optional display title."},
			"node":     {Type: "string", Description: "Optional node, e.g. macmini or m1."},
			"evidence": {Type: "string", Description: "Optional evidence id/path/URL."},
			"notes":    {Type: "string", Description: "Optional notes."},
		}, []string{"ref"})},
		{Name: "meshclaw_quickstart", Description: "Run the first 5 minutes path in one safe call: setup doctor, fleet-health workflow inspection, dry-run evidence bundle, and next actions. Creates dry-run evidence only.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_doctor", Description: "Check local MeshClaw/VSSH/MCP setup before fleet operations. Use this first when tools appear installed but monitor, vssh, or client setup looks inconsistent.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_setup_assistant", Description: "Read-only one-machine assistant onboarding check. Summarizes MeshClaw doctor, Signal dispatcher, Argos UI Runner readiness, recommended MCP catalog, and first successful tasks for a normal user install.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_model_config_status", Description: "Read-only status for the shared OpenAI-compatible chat model gateway used by Signal chat/assistant rooms. Returns base_url, model, max tokens, temperature, and a masked API key marker only; never returns raw secrets.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_setup_signal", Description: "Check Signal/Argos Automation Mode setup: signal-cli account, messenger targets, Signal dispatcher, schedule-runner, scheduler due/next-due status, local data-doctor retention summary, chat model endpoint, and call runner readiness. With repair=true, installs or starts missing/stopped local daemons and runs room doctor with a temporary dispatcher pause.", InputSchema: objectSchema(map[string]mcpProperty{
			"repair": {Type: "boolean", Description: "Attempt safe local repair: install/start Signal dispatcher and schedule-runner, then run room doctor with auto-pause", Default: false},
		}, nil)},
		{Name: "meshclaw_setup_argos_runner", Description: "Check or configure Argos UI Runner for bounded macOS UI automation. With no coordinates it is read-only. With coordinates it writes ~/.meshclaw/argos-ui-runner.json.", InputSchema: objectSchema(map[string]mcpProperty{
			"signal_start_click":  {Type: "string", Description: "Optional x,y coordinate for the Signal start-call action"},
			"signal_hangup_click": {Type: "string", Description: "Optional x,y coordinate for the Signal hangup action"},
			"ui_runner":           {Type: "string", Description: "Optional Argos UI Runner URL", Default: "http://127.0.0.1:48292"},
		}, nil)},
		{Name: "meshclaw_daemon_signal_status", Description: "Return launchd status for the Signal dispatcher daemon.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_daemon_schedule_status", Description: "Return launchd status plus scheduler due/next-due summary for the schedule-runner daemon. A stopped launchd one-shot with due_count=0 can be healthy between interval runs.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_mcp_surface", Description: "Return the recommended MeshClaw MCP tool surface for AI operators: default tools, advanced tools, and when to use direct vssh instead.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_mcp_catalog", Description: "Return a grouped MCP tool catalog and install profiles. Use this when a model needs to understand MeshClaw's native capabilities instead of guessing or relying on an external wrapper UI.", InputSchema: objectSchema(map[string]mcpProperty{
			"profile": {Type: "string", Description: "Optional profile filter: local-lite, claude-lite, one-machine-assistant, multi-node-ops, development, or all", Default: "all"},
		}, nil)},
		{Name: "meshclaw_local_assistant", Description: "Low-token local-model gateway for common personal assistant tasks. Routes natural language to safe read-only tools or plan-only writes, so small models do not need to choose among many MCP tools.", InputSchema: objectSchema(map[string]mcpProperty{
			"task":        {Type: "string", Description: "Natural-language task, e.g. 오늘 메일 요약, 내 일정, 리마인더 추가, 문서 만들어줘"},
			"execute":     {Type: "boolean", Description: "Allow writes/visible UI only when true; default returns plans for mutations", Default: false},
			"title":       {Type: "string", Description: "Optional title for event, reminder, note, or document"},
			"body":        {Type: "string", Description: "Optional body/notes/content"},
			"start":       {Type: "string", Description: "Optional RFC3339/ISO start time"},
			"end":         {Type: "string", Description: "Optional RFC3339/ISO end time"},
			"due":         {Type: "string", Description: "Optional RFC3339/ISO reminder due time"},
			"query":       {Type: "string", Description: "Optional search query"},
			"origin":      {Type: "string", Description: "Optional map origin"},
			"destination": {Type: "string", Description: "Optional map destination"},
		}, []string{"task"})},
		{Name: "meshclaw_legacy_skills_audit", Description: "Read-only audit of old ~/.meshclaw/skills prompt skills. Classifies which legacy skills are already covered by current MCP tools, which should be migrated, and which should stay archived. Does not run skills, install dependencies, edit files, or change active MCP behavior.", InputSchema: objectSchema(map[string]mcpProperty{
			"path":  {Type: "string", Description: "Optional legacy skills directory. Defaults to ~/.meshclaw/skills."},
			"limit": {Type: "number", Description: "Maximum detailed skill rows to return", Default: 120},
		}, nil)},
		{Name: "meshclaw_tool_recommend", Description: "Recommend the safest MeshClaw/vssh tool path for a natural-language operator intent without executing anything.", InputSchema: objectSchema(map[string]mcpProperty{
			"intent":  {Type: "string", Description: "Natural-language operator intent or task"},
			"subject": {Type: "string", Description: "Optional caller/model subject, e.g. codex, claude, local-llm", Default: "codex"},
		}, []string{"intent"})},
		{Name: "meshclaw_server_list", Description: "List known servers and Tailscale-first connection facts. For AI operators, use this for inventory truth before choosing execution targets.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_inventory_discover", Description: "Discover managed nodes from Tailscale, vssh, and local user mappings without changing the saved inventory.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_inventory_diff", Description: "Compare saved inventory with current discovery and report missing or changed nodes.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_inventory_override_list", Description: "List operator-owned inventory role/tag overrides. Use before changing private fleet meaning.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_inventory_override_set", Description: "Set an operator-owned inventory role/tag override for one node. This updates local MeshClaw state only, not the remote server.", InputSchema: objectSchema(map[string]mcpProperty{
			"node":     {Type: "string", Description: "Node name, e.g. c1, g4, s2"},
			"role":     {Type: "string", Description: "Optional role, e.g. mail-server, automation-worker, ollama-worker"},
			"location": {Type: "string", Description: "Optional location label, e.g. vps, home, lab"},
			"user":     {Type: "string", Description: "Optional default SSH/vssh user"},
			"tags":     {Type: "string", Description: "Optional comma-separated tags, e.g. linux,mail,mox"},
		}, []string{"node"})},
		{Name: "meshclaw_inventory_override_remove", Description: "Remove one operator-owned inventory override entry by node name.", InputSchema: objectSchema(map[string]mcpProperty{
			"node": {Type: "string", Description: "Node name to remove from inventory overrides"},
		}, []string{"node"})},
		{Name: "meshclaw_workers", Description: "List model operators, Matrix room surfaces, MeshClaw MCP, and vssh execution workers.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_workspace_list", Description: "List known workspaces: server, folder, owner/model, branch, purpose, and recent activity.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_workspace_add", Description: "Register or update a workspace location used by a model or human operator.", InputSchema: objectSchema(map[string]mcpProperty{
			"id":      {Type: "string", Description: "Stable workspace id"},
			"host":    {Type: "string", Description: "Host name, or local"},
			"path":    {Type: "string", Description: "Absolute workspace path"},
			"owner":   {Type: "string", Description: "Current owner/model/operator"},
			"purpose": {Type: "string", Description: "Why this workspace exists"},
			"branch":  {Type: "string", Description: "Git branch or working lane"},
			"source":  {Type: "string", Description: "Frontend/source, e.g. codex-app, claude-web, matrix"},
		}, []string{"id", "host", "path"})},
		{Name: "meshclaw_workspace_frontend", Description: "Attach a user-approved monthly subscription frontend session to a workspace, 1Code-style. This records app/browser lane metadata only.", InputSchema: objectSchema(map[string]mcpProperty{
			"id":       {Type: "string", Description: "Workspace id"},
			"provider": {Type: "string", Description: "codex, claude, chatgpt, cursor, etc."},
			"app":      {Type: "string", Description: "Local app name, e.g. Codex or Claude"},
			"url":      {Type: "string", Description: "Browser URL for logged-in frontend"},
			"login":    {Type: "string", Description: "Human-readable login/plan note"},
			"status":   {Type: "string", Description: "configured, logged_in, needs_login, broken", Default: "configured"},
		}, []string{"id", "provider"})},
		{Name: "meshclaw_workspace_activity", Description: "Record what an operator/model did in a workspace and store evidence.", InputSchema: objectSchema(map[string]mcpProperty{
			"id":       {Type: "string", Description: "Workspace id"},
			"actor":    {Type: "string", Description: "Actor, e.g. codex, claude, local-llm, human"},
			"action":   {Type: "string", Description: "Action name, e.g. edit, review, deploy, scan"},
			"summary":  {Type: "string", Description: "Short activity summary"},
			"evidence": {Type: "string", Description: "Optional evidence URL/path/id"},
		}, []string{"id", "actor", "action", "summary"})},
		{Name: "meshclaw_capability_list", Description: "List models, APIs, services, and provisioner capabilities without revealing secrets.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_capability_validate", Description: "Validate the MeshClaw capability registry before AI placement or execute mode. Returns structured errors and warnings without revealing secrets.", InputSchema: objectSchema(map[string]mcpProperty{
			"path": {Type: "string", Description: "Optional capability registry path. Defaults to ~/.meshclaw/capabilities.json or MESHCLAW_CAPABILITY_FILE."},
		}, nil)},
		{Name: "meshclaw_capability_recommend", Description: "Recommend capability IDs for an AI operator intent before placement, workflow execution, API use, or model selection. Returns scores, reasons, approval flags, and secret-handling cautions.", InputSchema: objectSchema(map[string]mcpProperty{
			"intent": {Type: "string", Description: "Natural-language intent, e.g. choose a GPU model worker, inspect mail server, archive artifacts, or provision a VPS"},
		}, []string{"intent"})},
		{Name: "meshclaw_mail_setup", Description: "Prepare or save an email account setup. Supports provider presets for Gmail/Naver/etc, existing macOS Keychain items, or direct IMAP settings. Never accepts raw passwords.", InputSchema: objectSchema(map[string]mcpProperty{
			"email":    {Type: "string", Description: "Email address to configure"},
			"mode":     {Type: "string", Description: "auto, provider, keychain, or direct", Default: "auto"},
			"host":     {Type: "string", Description: "Direct IMAP host override"},
			"port":     {Type: "number", Description: "Direct IMAP port", Default: 993},
			"username": {Type: "string", Description: "Direct IMAP username override"},
			"service":  {Type: "string", Description: "Existing macOS Keychain service name for keychain mode"},
			"account":  {Type: "string", Description: "Existing macOS Keychain account name for keychain mode"},
			"execute":  {Type: "boolean", Description: "Save account config and link keychain metadata when true", Default: false},
		}, []string{"email"})},
		{Name: "meshclaw_mail_doctor", Description: "Check configured mail accounts, Guard/Keychain readiness, IMAP network, login, and mailbox status without revealing passwords.", InputSchema: objectSchema(map[string]mcpProperty{
			"account":     {Type: "string", Description: "Optional configured mail account id"},
			"check_login": {Type: "boolean", Description: "Attempt IMAP login using configured Guard/Keychain handle", Default: true},
		}, nil)},
		{Name: "meshclaw_mail_discover_keychain", Description: "List likely macOS Keychain mail credential candidates by service/account metadata only. Does not read raw passwords.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_mail_accounts", Description: "List configured mail accounts without revealing passwords. Mail accounts use password_env or Guard vault handles.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_mail_search", Description: "READ-ONLY mail discovery. Search IMAP and return redacted message summaries only: id, from, subject, date, snippet, attachment flag. If account is omitted, searches every configured account and groups results. Use before reading bodies or drafting replies. Stores redacted evidence.", InputSchema: objectSchema(map[string]mcpProperty{
			"account": {Type: "string", Description: "Configured mail account id"},
			"query":   {Type: "string", Description: "Optional IMAP text search query"},
			"since":   {Type: "string", Description: "Optional duration/date, e.g. 24h, 7d, 2026-05-24"},
			"limit":   {Type: "number", Description: "Maximum messages", Default: 10},
		}, nil)},
		{Name: "meshclaw_mail_summarize", Description: "READ-ONLY mail summary. Search recent mail and return a concise operator summary plus redacted message summaries. If account is omitted, summarizes every configured account. Does not read full bodies, download attachments, move, delete, draft, or send.", InputSchema: objectSchema(map[string]mcpProperty{
			"account": {Type: "string", Description: "Configured mail account id"},
			"query":   {Type: "string", Description: "Optional IMAP text search query"},
			"since":   {Type: "string", Description: "Optional duration/date, e.g. 24h, 7d, 2026-05-24", Default: "24h"},
			"limit":   {Type: "number", Description: "Maximum messages", Default: 10},
		}, nil)},
		{Name: "meshclaw_mail_thread", Description: "READ-ONLY mail body read. Read one IMAP message by UID with redaction and evidence storage. Attachments are not downloaded; use meshclaw_mail_attachments with approve=true for files.", InputSchema: objectSchema(map[string]mcpProperty{
			"account":  {Type: "string", Description: "Configured mail account id"},
			"id":       {Type: "string", Description: "IMAP message UID"},
			"max_body": {Type: "number", Description: "Maximum redacted body characters", Default: 5000},
		}, []string{"id"})},
		{Name: "meshclaw_mail_read_many", Description: "READ-ONLY mail body read for multiple UIDs. Redacts bodies and stores evidence. Attachments are not downloaded. Maximum 20 ids.", InputSchema: objectSchema(map[string]mcpProperty{
			"account":  {Type: "string", Description: "Configured mail account id"},
			"ids":      {Type: "string", Description: "Comma-separated IMAP message UIDs"},
			"max_body": {Type: "number", Description: "Maximum redacted body characters per message", Default: 5000},
		}, []string{"ids"})},
		{Name: "meshclaw_mail_attachments", Description: "APPROVAL-REQUIRED file write. Download attachments from one IMAP message to local disk. Without approve=true this returns approval_required and writes nothing.", InputSchema: objectSchema(map[string]mcpProperty{
			"account": {Type: "string", Description: "Configured mail account id"},
			"id":      {Type: "string", Description: "IMAP message UID"},
			"dir":     {Type: "string", Description: "Optional local output directory"},
			"approve": {Type: "boolean", Description: "Required to write attachment files", Default: false},
		}, []string{"id"})},
		{Name: "meshclaw_mail_move", Description: "APPROVAL-REQUIRED mailbox mutation. Move IMAP messages to another mailbox. Without approve=true this returns approval_required and does not mutate mail.", InputSchema: objectSchema(map[string]mcpProperty{
			"account": {Type: "string", Description: "Configured mail account id"},
			"ids":     {Type: "string", Description: "Comma-separated IMAP message UIDs"},
			"target":  {Type: "string", Description: "Target mailbox"},
			"approve": {Type: "boolean", Description: "Required to mutate mailbox state", Default: false},
		}, []string{"ids", "target"})},
		{Name: "meshclaw_mail_delete", Description: "DESTRUCTIVE approval-required mutation. Delete IMAP messages by marking Deleted and expunging. Without approve=true this returns approval_required and does not delete.", InputSchema: objectSchema(map[string]mcpProperty{
			"account": {Type: "string", Description: "Configured mail account id"},
			"ids":     {Type: "string", Description: "Comma-separated IMAP message UIDs"},
			"approve": {Type: "boolean", Description: "Required to delete mail", Default: false},
		}, []string{"ids"})},
		{Name: "meshclaw_mail_draft_reply", Description: "DRAFT-ONLY reply. Create a local unsent reply draft for a message. This never transmits email; use meshclaw_mail_send with approve=true only after human review.", InputSchema: objectSchema(map[string]mcpProperty{
			"account": {Type: "string", Description: "Configured mail account id"},
			"id":      {Type: "string", Description: "IMAP message UID"},
			"intent":  {Type: "string", Description: "User intent for the reply draft"},
		}, []string{"id", "intent"})},
		{Name: "meshclaw_mail_compose", Description: "DRAFT-ONLY compose. Create a local unsent email draft. This never transmits email; use meshclaw_mail_send with approve=true only after human review.", InputSchema: objectSchema(map[string]mcpProperty{
			"account": {Type: "string", Description: "Configured mail account id"},
			"to":      {Type: "string", Description: "Comma-separated recipients"},
			"subject": {Type: "string", Description: "Draft subject"},
			"body":    {Type: "string", Description: "Draft body"},
		}, []string{"account", "to", "subject", "body"})},
		{Name: "meshclaw_mail_send", Description: "SEND EMAIL. Approval-required transmission through SMTP. Without approve=true and an allowing policy decision this returns approval_required and sends nothing.", InputSchema: objectSchema(map[string]mcpProperty{
			"draft":   {Type: "string", Description: "Draft id"},
			"approve": {Type: "boolean", Description: "Required to transmit email", Default: false},
		}, []string{"draft"})},
		{Name: "meshclaw_mail_watch_once", Description: "One-shot new mail check since a duration such as 15m. Stores redacted evidence; intended for scheduler/daemon use.", InputSchema: objectSchema(map[string]mcpProperty{
			"account": {Type: "string", Description: "Configured mail account id"},
			"since":   {Type: "string", Description: "Duration such as 15m or 2h", Default: "15m"},
			"limit":   {Type: "number", Description: "Maximum messages", Default: 10},
		}, nil)},
		{Name: "meshclaw_adapter_list", Description: "List MeshClaw Runtime adapters and whether each adapter can execute, needs a command, or only records structured evidence.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_opsdb_status", Description: "Return MeshClaw OpsDB status, paths, counts, and the meshdb/opsdb boundary. Use this to check DevOps state without querying personal meshdb.", InputSchema: objectSchema(map[string]mcpProperty{
			"ensure": {Type: "boolean", Description: "Create the opsdb directory layout if it does not exist", Default: false},
		}, nil)},
		{Name: "meshclaw_opsdb_events", Description: "Return recent OpsDB drift events and evidence index entries. Use this when asking what Claude/Codex/local models or agents recently observed in DevOps state.", InputSchema: objectSchema(map[string]mcpProperty{
			"node":  {Type: "string", Description: "Optional node filter, e.g. g4"},
			"kind":  {Type: "string", Description: "Optional event/evidence kind filter, e.g. process-top or observation"},
			"limit": {Type: "number", Description: "Maximum events and evidence records to return", Default: 20},
		}, nil)},
		{Name: "meshclaw_opsdb_power_events", Description: "Detect correlated boot identity changes from OpsDB history. Use this when several machines rebooted or power quality is suspected.", InputSchema: objectSchema(map[string]mcpProperty{
			"window":     {Type: "string", Description: "Correlation window such as 15m", Default: "15m"},
			"uptime_max": {Type: "string", Description: "Only count new boots with uptime below this duration, e.g. 2h", Default: "2h"},
			"min_nodes":  {Type: "number", Description: "Minimum nodes needed to call it a fleet event", Default: 2},
			"limit":      {Type: "number", Description: "Maximum incidents to return", Default: 10},
			"record":     {Type: "boolean", Description: "Also write detected incidents to OpsDB as power_event records", Default: false},
		}, nil)},
		{Name: "meshclaw_opsdb_record", Description: "Record an operator/model observation into OpsDB. This writes DevOps audit state only, separate from personal meshdb, and does not execute remote actions.", InputSchema: objectSchema(map[string]mcpProperty{
			"node":     {Type: "string", Description: "Optional node id, e.g. g4"},
			"kind":     {Type: "string", Description: "Event kind, e.g. observation, risk, triage", Default: "observation"},
			"severity": {Type: "string", Description: "info, warning, high, critical", Default: "info"},
			"summary":  {Type: "string", Description: "Short operator-readable observation summary"},
			"source":   {Type: "string", Description: "Source actor/tool, e.g. claude-mcp, codex, openwebui", Default: "mcp"},
			"evidence": {Type: "string", Description: "Optional evidence id that supports this observation"},
			"tags":     {Type: "string", Description: "Optional comma-separated tags"},
		}, []string{"summary"})},
		{Name: "meshclaw_monitor_check", Description: "Check fleet health through MeshClaw's monitored state and vssh-backed facts. Prefer this over raw vssh facts for fleet status.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_agent_workloads", Description: "Summarize cached node-local agent reports into current server purpose, resource-consuming processes, public services, and medium/high risks. Use this when asking what servers are being used for right now.", InputSchema: objectSchema(map[string]mcpProperty{
			"max_age": {Type: "string", Description: "Maximum cache age, e.g. 15m, 24h, or 0 for all cached reports", Default: "24h"},
			"top":     {Type: "number", Description: "Top resource processes per node", Default: 3},
		}, nil)},
		{Name: "meshclaw_agent_changes", Description: "Compare the latest two node-local agent history samples and report workload/resource/risk changes. Use this when asking what changed recently.", InputSchema: objectSchema(map[string]mcpProperty{
			"node": {Type: "string", Description: "Node name or all", Default: "all"},
		}, nil)},
		{Name: "meshclaw_agent_security", Description: "Summarize cached node-local security posture: public listeners, Docker port exposure, firewall warnings, fail2ban, cron/timers, failed units, and next actions.", InputSchema: objectSchema(map[string]mcpProperty{
			"max_age": {Type: "string", Description: "Maximum cache age, e.g. 15m, 24h, or 0 for all cached reports", Default: "24h"},
		}, nil)},
		{Name: "meshclaw_agent_inventory_plan", Description: "Recommend inventory role/tag overrides from cached node-local workload state. By default this is plan-only; apply=true writes local inventory overrides.", InputSchema: objectSchema(map[string]mcpProperty{
			"apply": {Type: "boolean", Description: "Apply proposed role/tag overrides to local MeshClaw inventory override state", Default: false},
		}, nil)},
		{Name: "meshclaw_ops_brief", Description: "Return an operations brief: fleet health, workload state, recent changes, top risks, next actions, and evidence context. Defaults to cache-first fast mode for MCP clients.", InputSchema: objectSchema(map[string]mcpProperty{
			"fast": {Type: "boolean", Description: "Skip fleet service audit and use monitor/agent cache only. Recommended for MCP default timeouts.", Default: true},
		}, nil)},
		{Name: "meshclaw_ops_control", Description: "Canonical AI operator entry point for server management: health, service findings, autoheal candidates, policy posture, and evidence.", InputSchema: objectSchema(map[string]mcpProperty{
			"apply_safe": {Type: "boolean", Description: "Apply bounded non-destructive autoheal actions", Default: false},
		}, nil)},
		{Name: "meshclaw_metrics_cpu", Description: "Query CPU usage across fleet from Prometheus. Returns percentage per node with alerts for high usage.", InputSchema: objectSchema(map[string]mcpProperty{
			"server": {Type: "string", Description: "Prometheus server name from config (default: use default server)"},
		}, nil)},
		{Name: "meshclaw_metrics_memory", Description: "Query memory usage across fleet from Prometheus. Returns percentage per node with alerts for high usage.", InputSchema: objectSchema(map[string]mcpProperty{
			"server": {Type: "string", Description: "Prometheus server name from config (default: use default server)"},
		}, nil)},
		{Name: "meshclaw_metrics_disk", Description: "Query disk usage across fleet from Prometheus. Returns percentage per node with alerts for high usage.", InputSchema: objectSchema(map[string]mcpProperty{
			"server": {Type: "string", Description: "Prometheus server name from config (default: use default server)"},
		}, nil)},
		{Name: "meshclaw_metrics_load", Description: "Query load average across fleet from Prometheus. Returns load1 per node with alerts for high load.", InputSchema: objectSchema(map[string]mcpProperty{
			"server": {Type: "string", Description: "Prometheus server name from config (default: use default server)"},
		}, nil)},
		{Name: "meshclaw_metrics_all", Description: "Query all basic metrics (CPU, memory, disk, load) across fleet from Prometheus.", InputSchema: objectSchema(map[string]mcpProperty{
			"server": {Type: "string", Description: "Prometheus server name from config (default: use default server)"},
		}, nil)},
		{Name: "meshclaw_metrics_query", Description: "Run a custom PromQL query against Prometheus and return parsed results.", InputSchema: objectSchema(map[string]mcpProperty{
			"query":     {Type: "string", Description: "PromQL query to execute"},
			"server":    {Type: "string", Description: "Prometheus server name from config (default: use default server)"},
			"threshold": {Type: "number", Description: "Optional threshold for alerts", Default: 0},
		}, []string{"query"})},
		{Name: "meshclaw_util_crypto_price", Description: "Get current cryptocurrency price in USD/KRW with 24h change (CoinGecko). Read-only, no side effects.", InputSchema: objectSchema(map[string]mcpProperty{
			"coin": {Type: "string", Description: "CoinGecko coin id, e.g. bitcoin, ethereum", Default: "bitcoin"},
		}, nil)},
		{Name: "meshclaw_util_weather", Description: "Get current weather for a city (wttr.in). Read-only, no side effects.", InputSchema: objectSchema(map[string]mcpProperty{
			"city": {Type: "string", Description: "City name, e.g. Seoul, Tokyo", Default: "Seoul"},
		}, nil)},
		{Name: "meshclaw_util_convert", Description: "Convert a value between common units (length, weight, temperature, volume). Deterministic, no external calls.", InputSchema: objectSchema(map[string]mcpProperty{
			"value": {Type: "number", Description: "Numeric value to convert"},
			"from":  {Type: "string", Description: "Source unit, e.g. km, kg, celsius, l"},
			"to":    {Type: "string", Description: "Target unit, e.g. mile, lb, fahrenheit, gal"},
		}, []string{"value", "from", "to"})},
		{Name: "meshclaw_util_generate", Description: "Generate a UUID, random password, or SHA-256 hash, or base64 encode/decode a string. Deterministic, no external calls.", InputSchema: objectSchema(map[string]mcpProperty{
			"kind":   {Type: "string", Description: "uuid | password | hash | base64_encode | base64_decode"},
			"input":  {Type: "string", Description: "Input text for hash/base64_encode/base64_decode"},
			"length": {Type: "number", Description: "Password length (for kind=password)", Default: 16},
		}, []string{"kind"})},
		{Name: "meshclaw_node_inventory", Description: "Collect read-only software inventory for one node: OS, tools, services, containers, GPU.", InputSchema: objectSchema(map[string]mcpProperty{
			"host": {Type: "string", Description: "Inventory host name"},
		}, []string{"host"})},
		{Name: "meshclaw_fleet_inventory", Description: "Collect read-only software inventory across fleet nodes.", InputSchema: objectSchema(map[string]mcpProperty{
			"hosts":        {Type: "string", Description: "Optional comma-separated inventory hosts"},
			"max_parallel": {Type: "number", Description: "Maximum hosts scanned in parallel", Default: 4},
		}, nil)},
		{Name: "meshclaw_placement_plan", Description: "Recommend nodes for a workload using inventory, monitor facts, and tool availability.", InputSchema: objectSchema(map[string]mcpProperty{
			"workload": {Type: "string", Description: "Natural workload description, e.g. GPU inference, Docker service, local LLM"},
		}, []string{"workload"})},
		{Name: "meshclaw_service_registry_plan", Description: "Plan service discovery and LB-lite routing from endpoint health evidence. Plan-only; never changes proxies, DNS, firewall, or containers.", InputSchema: objectSchema(map[string]mcpProperty{
			"service":   {Type: "string", Description: "Service name, e.g. openwebui, api, vllm"},
			"scope":     {Type: "string", Description: "Fleet scope or tag, e.g. all, gpu, web", Default: "all"},
			"port":      {Type: "number", Description: "Optional service port"},
			"endpoints": {Type: "string", Description: "Optional comma-separated endpoint candidates, e.g. g1:8080,g2:8080"},
		}, []string{"service"})},
		{Name: "meshclaw_capacity_scale_plan", Description: "Plan capacity, placement, and autoscale-like actions from fleet evidence. Plan-only; never rents, deletes, migrates, or mutates servers.", InputSchema: objectSchema(map[string]mcpProperty{
			"workload":    {Type: "string", Description: "Workload or service to scale, e.g. vllm, openwebui, api"},
			"scope":       {Type: "string", Description: "Fleet scope or tag, e.g. all, gpu, web", Default: "all"},
			"target":      {Type: "string", Description: "Desired state or SLO, e.g. p95<500ms, gpu_free>=1, replicas=2"},
			"budget_usd":  {Type: "number", Description: "Optional maximum provider spend for later provisioning plans", Default: 0},
			"ttl_hours":   {Type: "number", Description: "Optional temporary capacity TTL for later provisioning plans", Default: 6},
			"constraints": {Type: "string", Description: "Optional comma-separated constraints, e.g. gpu,local-first,avoid-public-ip"},
		}, []string{"workload"})},
		{Name: "meshclaw_storage_guardrail_plan", Description: "Plan storage, mount, volume, disk-pressure, and backup guardrails from evidence. Plan-only; never deletes files, changes mounts, snapshots, or writes backups.", InputSchema: objectSchema(map[string]mcpProperty{
			"node":       {Type: "string", Description: "Node or scope, e.g. d1, gpu, all", Default: "all"},
			"path":       {Type: "string", Description: "Path, mount point, or volume to protect", Default: "/"},
			"workload":   {Type: "string", Description: "Optional workload depending on this storage, e.g. openwebui, vllm, postgres"},
			"risk":       {Type: "string", Description: "Observed risk, e.g. disk_pressure, mount_missing, backup_needed, volume_move", Default: "disk_pressure"},
			"backup":     {Type: "boolean", Description: "Whether the plan should require backup/snapshot evidence before mutation", Default: true},
			"retention":  {Type: "string", Description: "Optional desired retention policy, e.g. 7d, 30d, keep-newest-5"},
			"mount_type": {Type: "string", Description: "Optional mount/storage type, e.g. local, nas, nfs, zfs, docker-volume"},
		}, []string{"node", "path"})},
		{Name: "meshclaw_ops_integration_plan", Description: "Plan how existing ops tools such as Prometheus, Grafana, Loki, Portainer, Ansible, Uptime Kuma, ntfy, or Tailscale should be wrapped as MeshClaw MCP semantic evidence sources. Plan-only; never changes those tools.", InputSchema: objectSchema(map[string]mcpProperty{
			"tools":    {Type: "string", Description: "Comma-separated external tools, e.g. prometheus,grafana,loki,ansible"},
			"goal":     {Type: "string", Description: "Operational goal, e.g. chat-driven incident triage, autoheal setup, dashboard-to-evidence"},
			"scope":    {Type: "string", Description: "Fleet scope or integration scope", Default: "fleet"},
			"readonly": {Type: "boolean", Description: "Whether to require read-only integration first", Default: true},
		}, []string{"tools", "goal"})},
		{Name: "meshclaw_mcp_rollout_plan", Description: "Plan how a new MeshClaw MCP build becomes visible to Claude, OpenWebUI, or another MCP client. Plan-only; never builds, installs, restarts clients, or deploys servers.", InputSchema: objectSchema(map[string]mcpProperty{
			"client":         {Type: "string", Description: "MCP client name, e.g. claude, openwebui, codex", Default: "claude"},
			"branch":         {Type: "string", Description: "Source branch or PR branch that contains the new tools"},
			"expected_tools": {Type: "string", Description: "Comma-separated MCP tool names expected after rollout"},
		}, nil)},
		{Name: "meshclaw_mcp_smoke_test_plan", Description: "Plan a read-only MCP smoke test sequence for newly exposed plan-only MeshClaw tools. Plan-only; never restarts clients, deploys servers, or runs mutating tools.", InputSchema: objectSchema(map[string]mcpProperty{
			"client": {Type: "string", Description: "MCP client name, e.g. claude, openwebui, codex", Default: "claude"},
			"scope":  {Type: "string", Description: "Smoke test scope, e.g. rollout, k8s-replacement, ops-integration", Default: "k8s-replacement"},
			"tools":  {Type: "string", Description: "Optional comma-separated plan-only tools to include"},
		}, nil)},
		{Name: "meshclaw_mcp_profile_visibility_check", Description: "Check whether expected MCP tools are visible in a specific MeshClaw MCP profile such as claude-lite. Read-only; never rebuilds, restarts clients, deploys servers, or runs tools.", InputSchema: objectSchema(map[string]mcpProperty{
			"profile":        {Type: "string", Description: "MCP profile to inspect, e.g. claude-lite, local-lite, all", Default: "claude-lite"},
			"expected_tools": {Type: "string", Description: "Comma-separated MCP tool names expected in the profile"},
			"apply":          {Type: "boolean", Description: "Rejected; profile visibility check does not change profiles", Default: false},
			"execute":        {Type: "boolean", Description: "Rejected; profile visibility check does not execute tools", Default: false},
		}, []string{"expected_tools"})},
		{Name: "meshclaw_automation_rule_plan", Description: "Plan an approval-gated automation rule from trigger, condition, evidence, action, verification, and rollback requirements. Plan-only; never writes schedules, policies, or live automation rules.", InputSchema: objectSchema(map[string]mcpProperty{
			"name":       {Type: "string", Description: "Automation rule name, e.g. openwebui-container-repair"},
			"trigger":    {Type: "string", Description: "Trigger signal, e.g. container_unhealthy, service_failed, disk_pressure", Default: "container_unhealthy"},
			"condition":  {Type: "string", Description: "Condition expression or natural-language guard, e.g. unhealthy for 3 checks and logs contain timeout"},
			"action":     {Type: "string", Description: "Planned action chain, e.g. autoheal_container_apply_plan, messenger_approval, storage_guardrail_plan"},
			"scope":      {Type: "string", Description: "Node/service/container scope", Default: "fleet"},
			"auto_apply": {Type: "boolean", Description: "Whether the proposal asks for future automatic apply. This plan never enables it.", Default: false},
		}, []string{"name", "trigger", "action"})},
		{Name: "meshclaw_automation_rule_check", Description: "Validate automation-rule-plan evidence before any future writer consumes it. Gate-only; never writes schedules, policies, rules, or runs automation.", InputSchema: objectSchema(map[string]mcpProperty{
			"rule_plan_evidence_path": {Type: "string", Description: "Path to automation-rule-plan evidence JSON"},
			"approved_by":             {Type: "string", Description: "Operator identity for approval-required rule setup"},
			"apply":                   {Type: "boolean", Description: "Rejected; rule check does not apply automation rules", Default: false},
			"execute":                 {Type: "boolean", Description: "Rejected; rule check does not execute automation", Default: false},
		}, []string{"rule_plan_evidence_path"})},
		{Name: "meshclaw_automation_rule_readiness_summary", Description: "Summarize automation-rule readiness from rule-check evidence. Summary-only; never writes schedules, policies, rules, or runs automation.", InputSchema: objectSchema(map[string]mcpProperty{
			"rule_check_evidence_path": {Type: "string", Description: "Path to automation-rule-check evidence JSON"},
			"apply":                    {Type: "boolean", Description: "Rejected; readiness summary does not apply automation rules", Default: false},
			"execute":                  {Type: "boolean", Description: "Rejected; readiness summary does not execute automation", Default: false},
		}, []string{"rule_check_evidence_path"})},
		{Name: "meshclaw_automation_rule_writer_plan", Description: "Preview the automation-rule envelope a future writer would create from ready readiness evidence. Plan-only; never writes schedules, policies, rules, or runs automation.", InputSchema: objectSchema(map[string]mcpProperty{
			"readiness_evidence_path": {Type: "string", Description: "Path to automation-rule-readiness-summary evidence JSON"},
			"rule_store":              {Type: "string", Description: "Planned future rule store path or logical store name", Default: "~/.meshclaw/rules"},
			"apply":                   {Type: "boolean", Description: "Rejected; writer plan does not apply automation rules", Default: false},
			"execute":                 {Type: "boolean", Description: "Rejected; writer plan does not execute automation", Default: false},
		}, []string{"readiness_evidence_path"})},
		{Name: "meshclaw_orchestration_plan", Description: "Explain a complex orchestration scenario: plan decomposition, worker/model placement, why tools like LangChain/n8n are or are not used, documentation checks, secret policy, and evidence.", InputSchema: objectSchema(map[string]mcpProperty{
			"scenario": {Type: "string", Description: "Natural-language orchestration scenario"},
		}, []string{"scenario"})},
		{Name: "meshclaw_fleet_scan", Description: "Run a policy-checked fleet diagnostic scan and store one evidence bundle.", InputSchema: objectSchema(map[string]mcpProperty{
			"hosts":        {Type: "string", Description: "Optional comma-separated inventory hosts"},
			"security":     {Type: "boolean", Description: "Run security snapshots", Default: true},
			"hygiene":      {Type: "boolean", Description: "Run redacted hygiene scans", Default: true},
			"logs":         {Type: "boolean", Description: "Run warning/error log scans", Default: true},
			"max_parallel": {Type: "number", Description: "Maximum hosts scanned in parallel", Default: 3},
		}, nil)},
		{Name: "meshclaw_fleet_service_audit", Description: "Run read-only failed/restarting service audit across Linux fleet nodes.", InputSchema: objectSchema(map[string]mcpProperty{
			"hosts":        {Type: "string", Description: "Optional comma-separated inventory hosts"},
			"max_parallel": {Type: "number", Description: "Maximum hosts scanned in parallel", Default: 4},
		}, nil)},
		{Name: "meshclaw_vssh_daemon_audit", Description: "Check vssh native daemon reachability, auth, version, and service state across fleet nodes.", InputSchema: objectSchema(map[string]mcpProperty{
			"hosts": {Type: "string", Description: "Optional comma-separated inventory hosts"},
		}, nil)},
		{Name: "meshclaw_vssh_auth_paths", Description: "Compare vssh binary, secret source, run-many, and facts paths without revealing the secret.", InputSchema: objectSchema(map[string]mcpProperty{
			"hosts": {Type: "string", Description: "Optional comma-separated inventory hosts"},
		}, nil)},
		{Name: "meshclaw_node_repair_plan", Description: "Canonical AI operator entry point for node execution health: classify daemon/auth/version issues and return safe repair steps.", InputSchema: objectSchema(map[string]mcpProperty{
			"hosts": {Type: "string", Description: "Optional comma-separated inventory hosts"},
		}, nil)},
		{Name: "meshclaw_service_triage", Description: "Audit and classify service failure candidates as real incidents, stale boot-only findings, ignore candidates, or approval-required actions.", InputSchema: objectSchema(map[string]mcpProperty{
			"hosts":        {Type: "string", Description: "Optional comma-separated inventory hosts"},
			"limit":        {Type: "number", Description: "Maximum service candidates to inspect with service-check", Default: 5},
			"max_parallel": {Type: "number", Description: "Maximum hosts audited in parallel", Default: 4},
		}, nil)},
		{Name: "meshclaw_reconcile_validate_desired", Description: "Validate desired-state YAML and store findings before plan/apply readiness. Never executes server changes.", InputSchema: objectSchema(map[string]mcpProperty{
			"desired_path": {Type: "string", Description: "Path to desired-state YAML"},
			"apply":        {Type: "boolean", Description: "Rejected; desired validation does not apply changes", Default: false},
			"execute":      {Type: "boolean", Description: "Rejected; desired validation does not execute changes", Default: false},
		}, []string{"desired_path"})},
		{Name: "meshclaw_reconcile_plan", Description: "Parse desired-state YAML and store a dry-run reconciliation plan. This never applies changes; apply/execute inputs are rejected.", InputSchema: objectSchema(map[string]mcpProperty{
			"desired_path":       {Type: "string", Description: "Path to desired-state YAML"},
			"actual_report_path": {Type: "string", Description: "Optional path to a local MeshClaw node report JSON"},
			"apply":              {Type: "boolean", Description: "Rejected; reconcile plan is dry-run only", Default: false},
			"execute":            {Type: "boolean", Description: "Rejected; reconcile plan is dry-run only", Default: false},
		}, []string{"desired_path"})},
		{Name: "meshclaw_reconcile_approval_request", Description: "Create approval evidence for reconcile actions that policy marks require_approval, and separate denied actions. Does not execute changes.", InputSchema: objectSchema(map[string]mcpProperty{
			"desired_path":       {Type: "string", Description: "Path to desired-state YAML"},
			"actual_report_path": {Type: "string", Description: "Optional path to a local MeshClaw node report JSON"},
			"apply":              {Type: "boolean", Description: "Rejected; approval request does not apply changes", Default: false},
			"execute":            {Type: "boolean", Description: "Rejected; approval request does not execute changes", Default: false},
		}, []string{"desired_path"})},
		{Name: "meshclaw_reconcile_apply_gate", Description: "Validate reconcile approval evidence before any future apply loop. Gate only; never executes server changes.", InputSchema: objectSchema(map[string]mcpProperty{
			"desired_path":           {Type: "string", Description: "Path to desired-state YAML that must match the approval request"},
			"approval_evidence_path": {Type: "string", Description: "Path to reconcile-approval-request evidence JSON"},
			"approved_by":            {Type: "string", Description: "Operator identity for approval-required actions"},
			"apply":                  {Type: "boolean", Description: "Rejected; apply gate does not apply changes", Default: false},
			"execute":                {Type: "boolean", Description: "Rejected; apply gate does not execute changes", Default: false},
		}, []string{"desired_path", "approval_evidence_path"})},
		{Name: "meshclaw_reconcile_apply_plan", Description: "Build structured apply steps from ready reconcile apply-gate evidence. Plan-only; never executes server changes.", InputSchema: objectSchema(map[string]mcpProperty{
			"gate_evidence_path": {Type: "string", Description: "Path to reconcile-apply-gate evidence JSON"},
			"apply":              {Type: "boolean", Description: "Rejected; apply plan does not apply changes", Default: false},
			"execute":            {Type: "boolean", Description: "Rejected; apply plan does not execute changes", Default: false},
		}, []string{"gate_evidence_path"})},
		{Name: "meshclaw_reconcile_execution_preview", Description: "Render inert command templates from reconcile apply-plan evidence, including desired container state/image/health metadata for review. Preview-only; never executes server changes.", InputSchema: objectSchema(map[string]mcpProperty{
			"apply_plan_evidence_path": {Type: "string", Description: "Path to reconcile-apply-plan evidence JSON"},
			"apply":                    {Type: "boolean", Description: "Rejected; execution preview does not apply changes", Default: false},
			"execute":                  {Type: "boolean", Description: "Rejected; execution preview does not execute changes", Default: false},
		}, []string{"apply_plan_evidence_path"})},
		{Name: "meshclaw_reconcile_verification_plan", Description: "Build post-action verification requirements from reconcile execution-preview evidence, preserving desired container metadata and focused container-logscan requirements. Plan-only; never executes server changes.", InputSchema: objectSchema(map[string]mcpProperty{
			"execution_preview_evidence_path": {Type: "string", Description: "Path to reconcile-execution-preview evidence JSON"},
			"apply":                           {Type: "boolean", Description: "Rejected; verification plan does not apply changes", Default: false},
			"execute":                         {Type: "boolean", Description: "Rejected; verification plan does not execute changes", Default: false},
		}, []string{"execution_preview_evidence_path"})},
		{Name: "meshclaw_reconcile_runbook", Description: "Build a review-only reconcile runbook from verification-plan evidence. Never executes server changes.", InputSchema: objectSchema(map[string]mcpProperty{
			"verification_plan_evidence_path": {Type: "string", Description: "Path to reconcile-verification-plan evidence JSON"},
			"apply":                           {Type: "boolean", Description: "Rejected; runbook does not apply changes", Default: false},
			"execute":                         {Type: "boolean", Description: "Rejected; runbook does not execute changes", Default: false},
		}, []string{"verification_plan_evidence_path"})},
		{Name: "meshclaw_reconcile_runbook_check", Description: "Validate reconcile runbook evidence before any future executor consumes it. Gate-only; never executes server changes.", InputSchema: objectSchema(map[string]mcpProperty{
			"runbook_evidence_path": {Type: "string", Description: "Path to reconcile-runbook evidence JSON"},
			"apply":                 {Type: "boolean", Description: "Rejected; runbook check does not apply changes", Default: false},
			"execute":               {Type: "boolean", Description: "Rejected; runbook check does not execute changes", Default: false},
		}, []string{"runbook_evidence_path"})},
		{Name: "meshclaw_reconcile_rollback_plan", Description: "Build rollback guidance from ready reconcile runbook-check evidence. Plan-only; never executes server changes.", InputSchema: objectSchema(map[string]mcpProperty{
			"runbook_check_evidence_path": {Type: "string", Description: "Path to reconcile-runbook-check evidence JSON"},
			"apply":                       {Type: "boolean", Description: "Rejected; rollback plan does not apply changes", Default: false},
			"execute":                     {Type: "boolean", Description: "Rejected; rollback plan does not execute changes", Default: false},
		}, []string{"runbook_check_evidence_path"})},
		{Name: "meshclaw_reconcile_completion_plan", Description: "Build final completion evidence requirements from ready reconcile rollback-plan evidence. Plan-only; never executes server changes.", InputSchema: objectSchema(map[string]mcpProperty{
			"rollback_plan_evidence_path": {Type: "string", Description: "Path to reconcile-rollback-plan evidence JSON"},
			"apply":                       {Type: "boolean", Description: "Rejected; completion plan does not apply changes", Default: false},
			"execute":                     {Type: "boolean", Description: "Rejected; completion plan does not execute changes", Default: false},
		}, []string{"rollback_plan_evidence_path"})},
		{Name: "meshclaw_reconcile_readiness_summary", Description: "Summarize approval-to-completion reconcile readiness from completion-plan evidence. Summary-only; never executes server changes.", InputSchema: objectSchema(map[string]mcpProperty{
			"completion_plan_evidence_path": {Type: "string", Description: "Path to reconcile-completion-plan evidence JSON"},
			"apply":                         {Type: "boolean", Description: "Rejected; readiness summary does not apply changes", Default: false},
			"execute":                       {Type: "boolean", Description: "Rejected; readiness summary does not execute changes", Default: false},
		}, []string{"completion_plan_evidence_path"})},
		{Name: "meshclaw_reconcile_run_once", Description: "Run one reconciliation cycle in dry-run mode only. Requires dry_run=true and never applies changes.", InputSchema: objectSchema(map[string]mcpProperty{
			"desired_path":       {Type: "string", Description: "Path to desired-state YAML"},
			"actual_report_path": {Type: "string", Description: "Optional path to a local MeshClaw node report JSON"},
			"dry_run":            {Type: "boolean", Description: "Required; run-once is dry-run only", Default: true},
			"execute":            {Type: "boolean", Description: "Rejected; execute is not implemented", Default: false},
		}, []string{"desired_path", "dry_run"})},
		{Name: "meshclaw_autoheal_plan", Description: "Convert current fleet alerts into read-only or auto-safe actions.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_autoheal_container_apply_plan", Description: "Build approval-gated, plan-only container restart step templates from autoheal-plan evidence, including runtime_evidence_required inspect/status requirements, prior analyze_logs handoff_contract.apply_allowed=false, and container_apply_plan_contract.direct_restart_allowed=false with requires_focused_runtime_evidence=true and runtime_evidence_required_count. Never executes docker commands.", InputSchema: objectSchema(map[string]mcpProperty{
			"plan_evidence_path": {Type: "string", Description: "Path to autoheal-plan evidence JSON; when derived from container logscan, retain analyze_logs handoff_contract.apply_allowed=false and expect container_apply_plan_contract.direct_restart_allowed=false"},
			"approved_by":        {Type: "string", Description: "Operator identity approving container apply planning"},
			"apply":              {Type: "boolean", Description: "Rejected; container apply plan does not apply changes", Default: false},
			"execute":            {Type: "boolean", Description: "Rejected; container apply plan does not execute changes", Default: false},
		}, []string{"plan_evidence_path"})},
		{Name: "meshclaw_autoheal_container_verification_plan", Description: "Build post-action verification requirements from container apply-plan evidence, preserving runtime_evidence_required, apply_plan_runtime_evidence_required_count, and focused container-logscan evidence. Plan-only; never executes docker commands.", InputSchema: objectSchema(map[string]mcpProperty{
			"container_apply_plan_evidence_path": {Type: "string", Description: "Path to container-apply-plan evidence JSON with container_apply_plan_contract.runtime_evidence_required_count"},
			"apply":                              {Type: "boolean", Description: "Rejected; container verification plan does not apply changes", Default: false},
			"execute":                            {Type: "boolean", Description: "Rejected; container verification plan does not execute changes", Default: false},
		}, []string{"container_apply_plan_evidence_path"})},
		{Name: "meshclaw_autoheal_container_runbook", Description: "Build a review-only container remediation runbook from container verification-plan evidence, preserving runtime_evidence_required and apply_plan_runtime_evidence_required_count. Never executes docker commands.", InputSchema: objectSchema(map[string]mcpProperty{
			"container_verification_plan_evidence_path": {Type: "string", Description: "Path to container-verification-plan evidence JSON with container_verification_plan_contract.apply_plan_runtime_evidence_required_count"},
			"apply":   {Type: "boolean", Description: "Rejected; container runbook does not apply changes", Default: false},
			"execute": {Type: "boolean", Description: "Rejected; container runbook does not execute changes", Default: false},
		}, []string{"container_verification_plan_evidence_path"})},
		{Name: "meshclaw_autoheal_container_runbook_check", Description: "Validate container runbook evidence, including runtime_evidence_required inspect/status terms, before any future executor consumes it. Gate-only; never executes docker commands.", InputSchema: objectSchema(map[string]mcpProperty{
			"container_runbook_evidence_path": {Type: "string", Description: "Path to container-runbook evidence JSON"},
			"apply":                           {Type: "boolean", Description: "Rejected; container runbook check does not apply changes", Default: false},
			"execute":                         {Type: "boolean", Description: "Rejected; container runbook check does not execute changes", Default: false},
		}, []string{"container_runbook_evidence_path"})},
		{Name: "meshclaw_autoheal_container_rollback_plan", Description: "Build rollback guidance from ready container runbook-check evidence, preserving runtime_evidence_required. Plan-only; never executes docker commands.", InputSchema: objectSchema(map[string]mcpProperty{
			"container_runbook_check_evidence_path": {Type: "string", Description: "Path to container-runbook-check evidence JSON"},
			"apply":                                 {Type: "boolean", Description: "Rejected; container rollback plan does not apply changes", Default: false},
			"execute":                               {Type: "boolean", Description: "Rejected; container rollback plan does not execute changes", Default: false},
		}, []string{"container_runbook_check_evidence_path"})},
		{Name: "meshclaw_autoheal_container_completion_plan", Description: "Build final completion evidence requirements from ready container rollback-plan evidence, preserving runtime_evidence_required and final container-logscan evidence. Plan-only; never executes docker commands.", InputSchema: objectSchema(map[string]mcpProperty{
			"container_rollback_plan_evidence_path": {Type: "string", Description: "Path to container-rollback-plan evidence JSON"},
			"apply":                                 {Type: "boolean", Description: "Rejected; container completion plan does not apply changes", Default: false},
			"execute":                               {Type: "boolean", Description: "Rejected; container completion plan does not execute changes", Default: false},
		}, []string{"container_rollback_plan_evidence_path"})},
		{Name: "meshclaw_autoheal_container_readiness_summary", Description: "Summarize container self-heal readiness from completion-plan evidence, including runtime_evidence_gate and runtime_evidence_findings. Summary-only; never executes docker commands.", InputSchema: objectSchema(map[string]mcpProperty{
			"container_completion_plan_evidence_path": {Type: "string", Description: "Path to container-completion-plan evidence JSON"},
			"apply":   {Type: "boolean", Description: "Rejected; container readiness summary does not apply changes", Default: false},
			"execute": {Type: "boolean", Description: "Rejected; container readiness summary does not execute changes", Default: false},
		}, []string{"container_completion_plan_evidence_path"})},
		{Name: "meshclaw_autoheal_container_executor_gate", Description: "Validate ready container readiness-summary evidence, explicit approved_by, dry_run=true, rollback, runtime, and final logscan gates before any future container executor. Admission-only; never executes docker commands.", InputSchema: objectSchema(map[string]mcpProperty{
			"container_readiness_summary_evidence_path": {Type: "string", Description: "Path to container-readiness-summary evidence JSON"},
			"approved_by": {Type: "string", Description: "Operator identity approving executor admission"},
			"dry_run":     {Type: "boolean", Description: "Required for executor gate admission", Default: true},
			"execute":     {Type: "boolean", Description: "Recorded as a blocker; executor gate never executes docker commands", Default: false},
		}, []string{"container_readiness_summary_evidence_path", "dry_run"})},
		{Name: "meshclaw_autoheal_container_executor", Description: "Run the validated container self-heal executor. Defaults to dry-run preview. Live mode restarts only runbook-validated containers and requires ready executor-gate evidence, matching approved_by, dry_run=false, execute=true, and live_approval_phrase exactly 'execute container self-heal approved'.", InputSchema: objectSchema(map[string]mcpProperty{
			"container_executor_gate_evidence_path": {Type: "string", Description: "Path to container-executor-gate evidence JSON"},
			"approved_by":                           {Type: "string", Description: "Operator identity; must match the executor gate approval identity"},
			"dry_run":                               {Type: "boolean", Description: "Preview commands without executing; default true", Default: true},
			"execute":                               {Type: "boolean", Description: "Request live execution. Requires dry_run=false and exact live approval phrase", Default: false},
			"live_approval_phrase":                  {Type: "string", Description: "Exact phrase required for live execution: execute container self-heal approved"},
		}, []string{"container_executor_gate_evidence_path", "approved_by", "dry_run"})},
		{Name: "meshclaw_autoheal_container_executor_verify", Description: "Gate container self-heal closeout after executor evidence. Requires live executor completion plus post-action agent-collect and focused container-logscan evidence for every executed step. Never executes docker commands.", InputSchema: objectSchema(map[string]mcpProperty{
			"container_executor_evidence_path": {Type: "string", Description: "Path to container-executor evidence JSON"},
			"agent_evidence_paths":             {Type: "string", Description: "Comma-separated post-action agent-collect evidence paths"},
			"container_logscan_evidence_paths": {Type: "string", Description: "Comma-separated focused container-logscan evidence paths"},
			"apply":                            {Type: "boolean", Description: "Rejected; verify gate does not apply changes", Default: false},
			"execute":                          {Type: "boolean", Description: "Rejected; verify gate does not execute commands", Default: false},
		}, []string{"container_executor_evidence_path"})},
		{Name: "meshclaw_autoheal_apply_safe", Description: "Apply bounded non-destructive autoheal actions and store evidence.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_workflow_list", Description: "Default workflow discovery tool. Lists repeatable MeshClaw workflows and tells AI operators the recommended next call. Start with fleet-health-demo or examples/workflows/fleet-health.json.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_workflow_scaffold", Description: "Create a validated generic MeshClaw workflow JSON scaffold for user-authored operations. Use when no existing workflow fits the operator intent.", InputSchema: objectSchema(map[string]mcpProperty{
			"name":   {Type: "string", Description: "New workflow name"},
			"output": {Type: "string", Description: "Optional output JSON path. Defaults to ~/.meshclaw/workflows/<name>.json"},
			"force":  {Type: "boolean", Description: "Overwrite an existing scaffold", Default: false},
		}, []string{"name"})},
		{Name: "meshclaw_workflow_validate", Description: "Validate a workflow name or file before running it. Use before meshclaw_workflow_run for user-authored workflows and examples/workflows/*.json.", InputSchema: objectSchema(map[string]mcpProperty{
			"name": {Type: "string", Description: "Workflow name or JSON file path"},
		}, []string{"name"})},
		{Name: "meshclaw_workflow_inspect", Description: "Inspect a workflow before running it: steps, approval gates, adapters, required nodes, policy decisions, and matching capability IDs. Use after validation and before execute mode.", InputSchema: objectSchema(map[string]mcpProperty{
			"name": {Type: "string", Description: "Workflow name or JSON file path"},
		}, []string{"name"})},
		{Name: "meshclaw_workflow_resume", Description: "Read a previous workflow evidence bundle and produce a resume plan: failed steps, retryable failures, approval-pending steps, degraded worker repair hints, and dry-run steps ready for execute mode. Does not execute anything.", InputSchema: objectSchema(map[string]mcpProperty{
			"ref": {Type: "string", Description: "latest, a bundle directory, or an execution.json path", Default: "latest"},
		}, nil)},
		{Name: "meshclaw_workflow_plan_execute", Description: "Preflight an evidence bundle before execute mode. Returns ready/blocking decision, approval/vault/repair blockers, and exact execute call when ready. Does not execute anything.", InputSchema: objectSchema(map[string]mcpProperty{
			"ref": {Type: "string", Description: "latest, a bundle directory, or an execution.json path", Default: "latest"},
		}, nil)},
		{Name: "meshclaw_approvals_list", Description: "List approval records stored in a workflow evidence bundle.", InputSchema: objectSchema(map[string]mcpProperty{
			"ref": {Type: "string", Description: "latest, a bundle directory, or an execution.json path", Default: "latest"},
		}, nil)},
		{Name: "meshclaw_approvals_grant", Description: "Record an approval for one approval-required workflow step. This only records approval evidence; it does not execute the step.", InputSchema: objectSchema(map[string]mcpProperty{
			"ref":    {Type: "string", Description: "latest, a bundle directory, or an execution.json path", Default: "latest"},
			"step":   {Type: "string", Description: "Workflow step id to approve"},
			"actor":  {Type: "string", Description: "Human or operator granting approval"},
			"reason": {Type: "string", Description: "Approval reason"},
			"source": {Type: "string", Description: "Approval source, e.g. cli, matrix, codex, claude", Default: "mcp"},
		}, []string{"step", "actor"})},
		{Name: "meshclaw_messenger_report", Description: "Build a redacted messenger-ready status report from a workflow evidence bundle. Use this when Codex/Claude should report to Signal or another chat without exposing secrets.", InputSchema: objectSchema(map[string]mcpProperty{
			"ref":      {Type: "string", Description: "latest, a bundle directory, or an execution.json path", Default: "latest"},
			"channel":  {Type: "string", Description: "Target report channel, e.g. signal, matrix, email", Default: "signal"},
			"audience": {Type: "string", Description: "Report audience, e.g. owner, team", Default: "owner"},
		}, nil)},
		{Name: "meshclaw_messenger_approval_request", Description: "Build one redacted approval request for a workflow step. Returns handle-safe text plus approval CLI/MCP call; it does not grant approval.", InputSchema: objectSchema(map[string]mcpProperty{
			"ref":      {Type: "string", Description: "latest, a bundle directory, or an execution.json path", Default: "latest"},
			"step":     {Type: "string", Description: "Approval-pending workflow step id"},
			"channel":  {Type: "string", Description: "Target report channel, e.g. signal", Default: "signal"},
			"audience": {Type: "string", Description: "Report audience, e.g. owner", Default: "owner"},
		}, []string{"step"})},
		{Name: "meshclaw_messenger_targets", Description: "List configured messenger targets. Targets store recipient metadata only, not message history or secrets.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_signal_rooms_doctor", Description: "Compare configured Signal group targets with the actual Signal account rooms. Protects registered rooms, flags missing targets, and identifies orphan/test rooms. Read-only.", InputSchema: objectSchema(map[string]mcpProperty{
			"timeout":               {Type: "number", Description: "signal-cli timeout in seconds", Default: 15},
			"auto_pause_dispatcher": {Type: "boolean", Description: "Temporarily stop the Signal dispatcher while reading rooms, then restart it if it was running", Default: false},
		}, nil)},
		{Name: "meshclaw_signal_rooms_discover", Description: "List Signal rooms that Argos has joined but MeshClaw has not bound to a target yet. Use before binding user-created rooms. Read-only.", InputSchema: objectSchema(map[string]mcpProperty{
			"timeout":               {Type: "number", Description: "signal-cli timeout in seconds", Default: 15},
			"auto_pause_dispatcher": {Type: "boolean", Description: "Temporarily stop the Signal dispatcher while reading rooms, then restart it if it was running", Default: false},
		}, nil)},
		{Name: "meshclaw_signal_room_bind", Description: "Bind a user-created Signal room that includes Argos to a MeshClaw target and mode. This is the product onboarding path; Argos is a participant, not the room owner.", InputSchema: objectSchema(map[string]mcpProperty{
			"room":                  {Type: "string", Description: "Signal room id or exact room name"},
			"target":                {Type: "string", Description: "Optional target id. Defaults from room name and mode."},
			"mode":                  {Type: "string", Description: "ops, briefing, assistant, chat, guard, or auto", Default: "auto"},
			"label":                 {Type: "string", Description: "Optional target label"},
			"model":                 {Type: "string", Description: "Optional model for chat-like modes"},
			"base_url":              {Type: "string", Description: "Optional OpenAI-compatible endpoint for chat-like modes"},
			"timeout":               {Type: "number", Description: "signal-cli timeout in seconds", Default: 15},
			"auto_pause_dispatcher": {Type: "boolean", Description: "Temporarily stop the Signal dispatcher while reading rooms, then restart it if it was running", Default: false},
		}, []string{"room"})},
		{Name: "meshclaw_signal_rooms_cleanup", Description: "Clean abandoned Signal test rooms only. Default is dry-run. Execute requires execute=true and approve=true; registered MeshClaw target rooms are never deleted.", InputSchema: objectSchema(map[string]mcpProperty{
			"timeout":               {Type: "number", Description: "signal-cli timeout in seconds", Default: 15},
			"execute":               {Type: "boolean", Description: "Actually quit/delete orphan cleanup candidates", Default: false},
			"approve":               {Type: "boolean", Description: "Required together with execute=true", Default: false},
			"auto_pause_dispatcher": {Type: "boolean", Description: "Temporarily stop the Signal dispatcher while reading rooms, then restart it if it was running", Default: false},
		}, nil)},
		{Name: "meshclaw_messenger_target_add", Description: "Add or update a messenger target such as an owner Signal recipient or Signal group. Does not send a message.", InputSchema: objectSchema(map[string]mcpProperty{
			"id":        {Type: "string", Description: "Stable target id, e.g. owner-signal"},
			"channel":   {Type: "string", Description: "Messenger channel. Currently signal.", Default: "signal"},
			"recipient": {Type: "string", Description: "Signal recipient identifier or phone number. Mutually exclusive with group_id."},
			"group_id":  {Type: "string", Description: "Signal group id for group chat targets. Mutually exclusive with recipient."},
			"label":     {Type: "string", Description: "Optional human label"},
			"mode":      {Type: "string", Description: "Optional target mode: guard, ops, chat, briefing, or assistant"},
			"model":     {Type: "string", Description: "Optional model for chat-like modes, e.g. gpt-oss:20b"},
			"base_url":  {Type: "string", Description: "Optional OpenAI-compatible endpoint for chat-like modes"},
		}, []string{"id"})},
		{Name: "meshclaw_messenger_send_report", Description: "Dry-run or execute sending a redacted evidence report to a configured messenger target. Execute uses signal-cli for Signal.", InputSchema: objectSchema(map[string]mcpProperty{
			"target":  {Type: "string", Description: "Configured messenger target id"},
			"ref":     {Type: "string", Description: "latest, a bundle directory, or an execution.json path", Default: "latest"},
			"execute": {Type: "boolean", Description: "Actually send via the adapter. Default false performs dry-run only.", Default: false},
		}, []string{"target"})},
		{Name: "meshclaw_messenger_send_approval_request", Description: "Dry-run or execute sending one redacted approval request to a configured messenger target. Does not grant approval.", InputSchema: objectSchema(map[string]mcpProperty{
			"target":  {Type: "string", Description: "Configured messenger target id"},
			"step":    {Type: "string", Description: "Approval-pending workflow step id"},
			"ref":     {Type: "string", Description: "latest, a bundle directory, or an execution.json path", Default: "latest"},
			"execute": {Type: "boolean", Description: "Actually send via the adapter. Default false performs dry-run only.", Default: false},
		}, []string{"target", "step"})},
		{Name: "meshclaw_scheduled_delivery_plan", Description: "Plan a future or recurring Signal delivery of a message, report, voice note, or voice report to the user, a room, or a friend/contact. This resolves the target and stores a review plan only; it does not register a schedule, send, call, or create recurring jobs.", InputSchema: objectSchema(map[string]mcpProperty{
			"target":       {Type: "string", Description: "Signal target id, room label, or macOS Contacts person name"},
			"schedule":     {Type: "string", Description: "Natural-language schedule such as 매일 오전 8시 or 매주 월요일 9시"},
			"content":      {Type: "string", Description: "Message/report topic/body to send on the schedule"},
			"content_type": {Type: "string", Description: "message, report, voice, voice_report, or auto", Default: "auto"},
			"delivery":     {Type: "string", Description: "signal, voice_note, call, or auto", Default: "auto"},
			"execute":      {Type: "boolean", Description: "Reserved for future apply. Default false returns a plan only.", Default: false},
			"approve":      {Type: "boolean", Description: "Reserved for future apply; required with execute=true.", Default: false},
		}, []string{"target", "schedule", "content"})},
		{Name: "meshclaw_scheduled_delivery_apply", Description: "Register an approved scheduled Signal delivery job after the first-run preview was shown. This stores an enabled job only; it does not send the first message, place calls, or execute recurring delivery. Requires execute=true, approve=true, and preview_approved=true.", InputSchema: objectSchema(map[string]mcpProperty{
			"target":           {Type: "string", Description: "Signal target id, room label, or macOS Contacts person name"},
			"schedule":         {Type: "string", Description: "Natural-language schedule such as 매일 오전 8시 or 매주 월요일 9시"},
			"content":          {Type: "string", Description: "Message/report topic/body to send on the schedule"},
			"content_type":     {Type: "string", Description: "message, report, voice, voice_report, or auto", Default: "auto"},
			"delivery":         {Type: "string", Description: "signal, voice_note, call, or auto", Default: "auto"},
			"execute":          {Type: "boolean", Description: "Actually register the approved job. Default false returns approval_required.", Default: false},
			"approve":          {Type: "boolean", Description: "Required with execute=true.", Default: false},
			"preview_approved": {Type: "boolean", Description: "Required after the user reviewed the first-run preview.", Default: false},
		}, []string{"target", "schedule", "content"})},
		{Name: "meshclaw_browser_fetch", Description: "Fetch a web page, extract title/text/links, and store browser evidence. Use before summarizing web pages or answering from current web content.", InputSchema: objectSchema(map[string]mcpProperty{
			"url":      {Type: "string", Description: "HTTP/HTTPS URL to fetch"},
			"max_body": {Type: "number", Description: "Maximum extracted text characters", Default: 12000},
			"timeout":  {Type: "number", Description: "Timeout in seconds", Default: 20},
		}, []string{"url"})},
		{Name: "meshclaw_browser_search", Description: "Search the web and return structured result links. Use as the browser-first discovery path before fetching selected pages.", InputSchema: objectSchema(map[string]mcpProperty{
			"query":   {Type: "string", Description: "Search query"},
			"limit":   {Type: "number", Description: "Maximum results", Default: 8},
			"timeout": {Type: "number", Description: "Timeout in seconds", Default: 20},
		}, []string{"query"})},
		{Name: "meshclaw_visible_browser_search", Description: "Plan or execute a user-visible browser search on the local macOS Runtime, then collect structured web results and evidence. Default execute=false returns a plan only; execute=true opens the local browser UI.", InputSchema: objectSchema(map[string]mcpProperty{
			"query":           {Type: "string", Description: "Search query to open visibly and collect"},
			"limit":           {Type: "number", Description: "Maximum structured search results to collect", Default: 5},
			"timeout":         {Type: "number", Description: "Structured search timeout in seconds", Default: 20},
			"execute":         {Type: "boolean", Description: "Actually open the browser UI. Default false returns a plan only.", Default: false},
			"record_seconds":  {Type: "number", Description: "Optional short screen recording after opening the browser, only used with execute=true", Default: 0},
			"record_artifact": {Type: "boolean", Description: "Store browser/search evidence for the visible task", Default: true},
		}, []string{"query"})},
		{Name: "meshclaw_document_create", Description: "Plan or create a macOS-first local Argos document bundle: Obsidian-friendly Markdown, mobile HTML, preview image, and editable DOCX for Word/Pages/iPhone. Default execute=false returns a plan only; execute=true writes files under the local Runtime's Argos Vault.", InputSchema: objectSchema(map[string]mcpProperty{
			"title":   {Type: "string", Description: "Document title", Default: "Argos 문서"},
			"body":    {Type: "string", Description: "Document body/content"},
			"execute": {Type: "boolean", Description: "Actually create the document files. Default false returns a plan only.", Default: false},
		}, []string{"body"})},
		{Name: "meshclaw_document_export", Description: "Export a local Obsidian/Markdown file to DOCX or PDF. Use this after drafting in the Argos Vault or Obsidian. Default execute=false returns a plan only. DOCX can use pandoc or MeshClaw's simple fallback; PDF requires pandoc.", InputSchema: objectSchema(map[string]mcpProperty{
			"input":   {Type: "string", Description: "Input Markdown file path"},
			"format":  {Type: "string", Description: "Output format: docx or pdf", Default: "docx"},
			"output":  {Type: "string", Description: "Optional output path"},
			"execute": {Type: "boolean", Description: "Actually export the document. Default false returns a plan only.", Default: false},
		}, []string{"input"})},
		{Name: "meshclaw_spreadsheet_create", Description: "Plan or create a macOS-first spreadsheet artifact for tables, budgets, invoices, trackers, checklists, logs, search/mail results, or editable row/column work. Default execute=false returns a plan only; execute=true writes XLSX for Numbers/Excel/iPhone editing, CSV, and mobile HTML preview under the local Runtime's Argos Vault.", InputSchema: objectSchema(map[string]mcpProperty{
			"title":   {Type: "string", Description: "Spreadsheet title", Default: "Argos 표"},
			"body":    {Type: "string", Description: "Markdown table, rows, notes, schema, or source content for the sheet"},
			"execute": {Type: "boolean", Description: "Actually create the spreadsheet files. Default false returns a plan only.", Default: false},
		}, []string{"body"})},
		{Name: "meshclaw_presentation_create", Description: "Create a usable macOS-first presentation artifact from the user's intent: editable PowerPoint/PPTX, Obsidian-friendly Markdown outline, audience, slide count, and content brief. Default execute=false returns a plan only; execute=true writes .pptx plus Markdown outline and verifies the PPTX package.", InputSchema: objectSchema(map[string]mcpProperty{
			"title":       {Type: "string", Description: "Presentation title", Default: "Argos Presentation"},
			"body":        {Type: "string", Description: "Brief, outline, or source content for the deck"},
			"audience":    {Type: "string", Description: "Target audience/context"},
			"slide_count": {Type: "number", Description: "Desired slide count, 1-20", Default: 6},
			"output":      {Type: "string", Description: "Optional output .pptx path"},
			"execute":     {Type: "boolean", Description: "Actually create and verify the PPTX. Default false returns a plan only.", Default: false},
		}, []string{"body"})},
		{Name: "meshclaw_presentation_verify", Description: "Verify that a local PowerPoint/PPTX artifact is structurally usable and contains slides. Read-only; use after creating or receiving a deck before telling the user it is done.", InputSchema: objectSchema(map[string]mcpProperty{
			"path": {Type: "string", Description: "Local .pptx path to verify"},
		}, []string{"path"})},
		{Name: "meshclaw_presentation_edit", Description: "Edit a local PowerPoint/PPTX by creating a verified edited copy with additional slide content. Default execute=false returns a plan only; execute=true backs up the input by default and writes a new PPTX.", InputSchema: objectSchema(map[string]mcpProperty{
			"input":   {Type: "string", Description: "Input .pptx path"},
			"title":   {Type: "string", Description: "Title for the added slide/content"},
			"body":    {Type: "string", Description: "Markdown or text content to add as slides"},
			"output":  {Type: "string", Description: "Optional edited output .pptx path"},
			"backup":  {Type: "boolean", Description: "Create a backup copy of the input before editing", Default: true},
			"execute": {Type: "boolean", Description: "Actually create the edited copy. Default false returns a plan only.", Default: false},
		}, []string{"input", "body"})},
		{Name: "meshclaw_presentation_export", Description: "Export a local PowerPoint/PPTX presentation to PDF after verifying the PPTX. Default execute=false returns a plan only; execute=true requires LibreOffice/soffice and writes the PDF.", InputSchema: objectSchema(map[string]mcpProperty{
			"input":   {Type: "string", Description: "Input .pptx path"},
			"format":  {Type: "string", Description: "Output format; currently pdf", Default: "pdf"},
			"output":  {Type: "string", Description: "Optional output .pdf path"},
			"execute": {Type: "boolean", Description: "Actually export to PDF. Default false returns a plan only.", Default: false},
		}, []string{"input"})},
		{Name: "meshclaw_screen_record", Description: "Plan or capture a short local macOS screen recording through Argos UI Runner. Default execute=false returns a plan only; execute=true records and stores evidence.", InputSchema: objectSchema(map[string]mcpProperty{
			"seconds": {Type: "number", Description: "Recording duration in seconds", Default: 3},
			"output":  {Type: "string", Description: "Optional output .mov path. If omitted, MeshClaw chooses a doctor/evidence path."},
			"purpose": {Type: "string", Description: "Optional proof purpose, e.g. map route proof, shopping comparison proof, logged-in form review, or ChatGPT answer handoff."},
			"execute": {Type: "boolean", Description: "Actually capture the screen recording. Default false returns a plan only.", Default: false},
		}, nil)},
		{Name: "meshclaw_screen_capture", Description: "Plan or capture a still local macOS screenshot for visible proof such as a map, shopping comparison, logged-in form review, or AI frontend answer. Default execute=false returns a privacy-checked plan only; execute=true also requires approve=true.", InputSchema: objectSchema(map[string]mcpProperty{
			"output":  {Type: "string", Description: "Optional output .png path. If omitted, MeshClaw chooses a doctor/evidence path."},
			"purpose": {Type: "string", Description: "Optional proof purpose, e.g. map still proof, shopping comparison proof, logged-in form review, or ChatGPT answer handoff."},
			"execute": {Type: "boolean", Description: "Actually capture the still screenshot. Default false returns a plan only.", Default: false},
			"approve": {Type: "boolean", Description: "Required with execute=true because screenshots may include sensitive visible data.", Default: false},
		}, nil)},
		{Name: "meshclaw_news_document", Description: "Create an Argos news Markdown document through the unified publish engine. Defaults to target=macmini so AI Studio/Codex and Signal use the same publication runtime; use target=local only for development.", InputSchema: objectSchema(map[string]mcpProperty{
			"limit":           {Type: "number", Description: "Maximum news items to include, 1-10", Default: 10},
			"target":          {Type: "string", Description: "Publication runtime target: macmini, auto, or local. Default macmini.", Default: "macmini"},
			"mission_id":      {Type: "string", Description: "Optional MacBook canonical mission id for artifact recording. Omit to use active.json."},
			"record_artifact": {Type: "boolean", Description: "Append the produced document to Mission artifacts on the MacBook canonical Core store.", Default: true},
		}, nil)},
		{Name: "meshclaw_argos_research", Description: "Run the unified Argos publication MVP: browser search, source-grounded Work Reports Markdown, mobile HTML preview, and private document links. When source text is fetched, the report includes cited [S1] summaries; otherwise it is a conservative source-candidate note. Defaults to target=macmini so MacBook MCP callers publish through the single Argos runtime.", InputSchema: objectSchema(map[string]mcpProperty{
			"query":           {Type: "string", Description: "Research/search query to publish"},
			"limit":           {Type: "number", Description: "Maximum search results to include, 1-10", Default: 8},
			"timeout":         {Type: "number", Description: "Search timeout in seconds", Default: 20},
			"target":          {Type: "string", Description: "Publication runtime target: macmini, auto, or local. Default macmini.", Default: "macmini"},
			"mission_id":      {Type: "string", Description: "Optional MacBook canonical mission id for artifact recording. Omit to use active.json."},
			"record_artifact": {Type: "boolean", Description: "Append the produced report to Mission artifacts on the MacBook canonical Core store.", Default: true},
		}, []string{"query"})},
		{Name: "meshclaw_automation_shortcuts", Description: "List macOS Shortcuts available to MeshClaw automation. macOS only.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_automation_shortcut_run", Description: "Run a named macOS Shortcut with optional text input. Use for Siri/Shortcuts-style app actions after the user has installed the shortcut.", InputSchema: objectSchema(map[string]mcpProperty{
			"name":  {Type: "string", Description: "Shortcut name"},
			"input": {Type: "string", Description: "Optional text input passed to the shortcut"},
		}, []string{"name"})},
		{Name: "meshclaw_shortcut_text_run", Description: "Plan or run a named macOS Shortcut with text input, then save stdout/stderr as Markdown/HTML result artifacts. Use this for Siri/Shortcuts-style app workflows that must leave a durable result. Default execute=false; execute=true also requires approve=true.", InputSchema: objectSchema(map[string]mcpProperty{
			"name":          {Type: "string", Description: "Shortcut name"},
			"input":         {Type: "string", Description: "Text input passed to the shortcut"},
			"title":         {Type: "string", Description: "Result artifact title", Default: "Shortcuts 작업"},
			"save_artifact": {Type: "boolean", Description: "Save Markdown/HTML output under ~/.meshclaw/automation-results", Default: true},
			"execute":       {Type: "boolean", Description: "Actually run the shortcut. Default false returns a plan only.", Default: false},
			"approve":       {Type: "boolean", Description: "Required together with execute=true because Shortcuts can mutate apps/accounts.", Default: false},
		}, []string{"name"})},
		{Name: "meshclaw_automation_open_url", Description: "Open a URL in the local desktop browser using macOS open or Linux xdg-open. This changes local UI state.", InputSchema: objectSchema(map[string]mcpProperty{
			"url": {Type: "string", Description: "URL to open"},
		}, []string{"url"})},
		{Name: "meshclaw_automation_open_app", Description: "Open a local macOS app by name. Use for user-visible app handoff, not hidden account/session access.", InputSchema: objectSchema(map[string]mcpProperty{
			"name": {Type: "string", Description: "macOS app name, e.g. Codex, Claude, 1Code"},
		}, []string{"name"})},
		{Name: "meshclaw_media_play", Description: "Plan or open a user-visible media playback surface for music, radio, podcast, or video requests. Default execute=false returns source choices and approval guidance. execute=true opens a Music app or a browser URL only; it does not click final playback, subscribe, purchase, or change accounts.", InputSchema: objectSchema(map[string]mcpProperty{
			"query":   {Type: "string", Description: "Requested music, artist, station, podcast, video, or mood"},
			"source":  {Type: "string", Description: "youtube, spotify, music, radio, podcast, or auto", Default: "auto"},
			"execute": {Type: "boolean", Description: "Open the selected app/URL visibly. Default false returns a plan only.", Default: false},
			"approve": {Type: "boolean", Description: "Required with execute=true because playback changes the user's visible/audio state.", Default: false},
		}, nil)},
		{Name: "meshclaw_audio_transcribe", Description: "Plan or transcribe a local audio file with local Whisper CLI. Default execute=false returns a privacy review plan. execute=true requires approve=true and writes a local transcript only; it does not send audio or transcripts to external services.", InputSchema: objectSchema(map[string]mcpProperty{
			"path":     {Type: "string", Description: "Local audio file path, e.g. mp3, m4a, wav, ogg"},
			"output":   {Type: "string", Description: "Optional transcript .txt output path. Defaults to ~/.meshclaw/transcripts/."},
			"model":    {Type: "string", Description: "Whisper model name", Default: "turbo"},
			"language": {Type: "string", Description: "Optional language hint, e.g. ko or en"},
			"task":     {Type: "string", Description: "Whisper task: transcribe or translate", Default: "transcribe"},
			"execute":  {Type: "boolean", Description: "Actually run local Whisper. Default false returns a plan only.", Default: false},
			"approve":  {Type: "boolean", Description: "Required with execute=true because audio can contain private speech.", Default: false},
		}, []string{"path"})},
		{Name: "meshclaw_automation_open_file", Description: "Open a local file in a user-visible macOS app. Prefer Obsidian for Markdown notes/briefs, Word or Pages for DOCX, PowerPoint or Keynote for decks, and Preview for PDF/images. Default execute=false returns a plan only; execute=true opens the file.", InputSchema: objectSchema(map[string]mcpProperty{
			"path":    {Type: "string", Description: "Local file path to open"},
			"app":     {Type: "string", Description: "Optional app name, e.g. Obsidian, Microsoft Word, Pages, Preview"},
			"execute": {Type: "boolean", Description: "Actually open the file. Default false returns a plan only.", Default: false},
		}, []string{"path"})},
		{Name: "meshclaw_notification_show", Description: "Show a local desktop notification. Default execute=false returns a plan only; execute=true posts the notification.", InputSchema: objectSchema(map[string]mcpProperty{
			"title":   {Type: "string", Description: "Notification title", Default: "Argos"},
			"body":    {Type: "string", Description: "Notification body"},
			"execute": {Type: "boolean", Description: "Actually show the notification. Default false returns a plan only.", Default: false},
		}, []string{"body"})},
		{Name: "meshclaw_signal_call_doctor", Description: "Read-only readiness check for approved Signal voice-call automation: Signal Desktop, Argos UI Runner, Accessibility, BlackHole audio route, and Signal logs. Use before any phone/call/morning-call flow.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_signal_call", Description: "Plan or place an approved Signal audio call through Argos UI Runner and Signal Desktop. Default execute=false is dry-run only. A real call requires execute=true, approve=true, an approved Signal target, readable audio file, call doctor OK, and evidence storage.", InputSchema: objectSchema(map[string]mcpProperty{
			"target":        {Type: "string", Description: "Configured Signal messenger target id to call"},
			"audio":         {Type: "string", Description: "Local audio file to play into the call, e.g. generated TTS/aiff/wav file"},
			"timeout":       {Type: "number", Description: "Seconds to wait for call acceptance", Default: 60},
			"restore_delay": {Type: "number", Description: "Seconds to wait before restoring audio route", Default: 1},
			"start_click":   {Type: "string", Description: "Optional fallback x,y coordinate for Signal start-call click"},
			"hangup_click":  {Type: "string", Description: "Optional fallback x,y coordinate for Signal hangup click"},
			"execute":       {Type: "boolean", Description: "Actually place the call. Default false returns dry-run/preflight only.", Default: false},
			"approve":       {Type: "boolean", Description: "Required with execute=true because real calls interrupt people.", Default: false},
		}, []string{"target", "audio"})},
		{Name: "meshclaw_terminal_run", Description: "Plan or run a local terminal command, then save stdout/stderr as Markdown/HTML result artifacts. Use for concrete Mac tasks where a command result must be preserved. Default execute=false; execute=true also requires approve=true.", InputSchema: objectSchema(map[string]mcpProperty{
			"command":       {Type: "string", Description: "Shell command to run locally"},
			"shell":         {Type: "string", Description: "Shell path", Default: "/bin/zsh"},
			"title":         {Type: "string", Description: "Result artifact title", Default: "터미널 작업"},
			"save_artifact": {Type: "boolean", Description: "Save Markdown/HTML output under ~/.meshclaw/automation-results", Default: true},
			"execute":       {Type: "boolean", Description: "Actually run the command. Default false returns a plan only.", Default: false},
			"approve":       {Type: "boolean", Description: "Required together with execute=true because terminal commands can change files or accounts.", Default: false},
		}, []string{"command"})},
		{Name: "meshclaw_result_save", Description: "Save a synthesized work result as local Markdown and mobile HTML artifacts under ~/.meshclaw/automation-results. Use after browser/app/terminal work so the assistant leaves a durable summary instead of only changing UI state.", InputSchema: objectSchema(map[string]mcpProperty{
			"title":   {Type: "string", Description: "Result title", Default: "Argos 작업 결과"},
			"body":    {Type: "string", Description: "Markdown-capable result body"},
			"source":  {Type: "string", Description: "Source/workflow label", Default: "meshclaw"},
			"execute": {Type: "boolean", Description: "Actually write result files. Default false returns a plan only.", Default: false},
		}, []string{"body"})},
		{Name: "meshclaw_maps_search", Description: "Create an Apple Maps or Google Maps place-search URL, and optionally open it visibly. Use for place lookup/location tasks before opening UI. Default execute=false returns the usable map URL only. If the user asks to see the map, use execute=true to open it, then capture still screen proof with meshclaw_screen_capture or short video proof with meshclaw_screen_record.", InputSchema: objectSchema(map[string]mcpProperty{
			"query":    {Type: "string", Description: "Place or address to search"},
			"provider": {Type: "string", Description: "apple or google", Default: "apple"},
			"execute":  {Type: "boolean", Description: "Open the map URL in the local UI. Default false returns URL only.", Default: false},
		}, []string{"query"})},
		{Name: "meshclaw_maps_directions", Description: "Create an Apple Maps or Google Maps directions URL, and optionally open it visibly. Use for route tasks such as '강남역에서 서울역까지'. Default execute=false returns the usable directions URL only. If the user asks to see the route, use execute=true to open it, then capture still screen proof with meshclaw_screen_capture or short video proof with meshclaw_screen_record.", InputSchema: objectSchema(map[string]mcpProperty{
			"origin":      {Type: "string", Description: "Optional start place/address. Omit for current/default location."},
			"destination": {Type: "string", Description: "Destination place/address"},
			"mode":        {Type: "string", Description: "driving, walking, or transit", Default: "driving"},
			"provider":    {Type: "string", Description: "apple or google", Default: "apple"},
			"execute":     {Type: "boolean", Description: "Open the directions URL in the local UI. Default false returns URL only.", Default: false},
		}, []string{"destination"})},
		{Name: "meshclaw_maps_proof", Description: "Create a map/place or directions link and, after explicit approval, open it visibly and capture a still screenshot proof. Use when the user asks for a map photo/capture plus link. Default execute=false returns a plan with the link; execute=true requires approve=true before opening/capturing.", InputSchema: objectSchema(map[string]mcpProperty{
			"query":        {Type: "string", Description: "Place/address query for kind=place"},
			"origin":       {Type: "string", Description: "Optional route start for kind=directions"},
			"destination":  {Type: "string", Description: "Route destination for kind=directions"},
			"kind":         {Type: "string", Description: "place or directions", Default: "directions"},
			"mode":         {Type: "string", Description: "driving, walking, or transit", Default: "driving"},
			"provider":     {Type: "string", Description: "apple or google", Default: "google"},
			"wait_seconds": {Type: "number", Description: "Seconds to wait after opening the map before capturing", Default: 2},
			"output":       {Type: "string", Description: "Optional output .png path for screenshot proof."},
			"execute":      {Type: "boolean", Description: "Open the map and capture screenshot proof. Default false returns a plan only.", Default: false},
			"approve":      {Type: "boolean", Description: "Required with execute=true because screenshots may include sensitive visible data.", Default: false},
		}, nil)},
		{Name: "meshclaw_calendar_list", Description: "List local macOS Calendar events in a time window through the Argos UI Runner Calendar helper. Read-only personal assistant tool; use ISO/RFC3339 start and end times.", InputSchema: objectSchema(map[string]mcpProperty{
			"start": {Type: "string", Description: "Start time in RFC3339/ISO format"},
			"end":   {Type: "string", Description: "End time in RFC3339/ISO format"},
			"query": {Type: "string", Description: "Optional text filter"},
		}, []string{"start", "end"})},
		{Name: "meshclaw_calendar_create_event", Description: "Create a local macOS Calendar event. Mutating personal assistant tool: default execute=false returns a plan only; execute=true changes Calendar and stores evidence.", InputSchema: objectSchema(map[string]mcpProperty{
			"title":   {Type: "string", Description: "Calendar event title"},
			"start":   {Type: "string", Description: "Start time in RFC3339/ISO format"},
			"end":     {Type: "string", Description: "End time in RFC3339/ISO format"},
			"notes":   {Type: "string", Description: "Optional notes"},
			"execute": {Type: "boolean", Description: "Actually create the event. Default false returns a plan only.", Default: false},
		}, []string{"title", "start", "end"})},
		{Name: "meshclaw_reminders_list", Description: "List local macOS Reminders in a time window through the Argos UI Runner Reminders helper. Read-only personal assistant tool.", InputSchema: objectSchema(map[string]mcpProperty{
			"start": {Type: "string", Description: "Start time in RFC3339/ISO format"},
			"end":   {Type: "string", Description: "End time in RFC3339/ISO format"},
			"query": {Type: "string", Description: "Optional text filter"},
		}, []string{"start", "end"})},
		{Name: "meshclaw_reminder_create", Description: "Create a local macOS Reminder. Mutating personal assistant tool: default execute=false returns a plan only; execute=true changes Reminders and stores evidence.", InputSchema: objectSchema(map[string]mcpProperty{
			"title":   {Type: "string", Description: "Reminder title"},
			"due":     {Type: "string", Description: "Optional due time in RFC3339/ISO format"},
			"notes":   {Type: "string", Description: "Optional notes"},
			"execute": {Type: "boolean", Description: "Actually create the reminder. Default false returns a plan only.", Default: false},
		}, []string{"title"})},
		{Name: "meshclaw_reminder_complete", Description: "Complete a local macOS Reminder by id or query. Mutating personal assistant tool: default execute=false returns a plan only; execute=true changes Reminders.", InputSchema: objectSchema(map[string]mcpProperty{
			"id":      {Type: "string", Description: "Optional reminder id"},
			"query":   {Type: "string", Description: "Reminder title/query if id is not known"},
			"execute": {Type: "boolean", Description: "Actually complete the reminder. Default false returns a plan only.", Default: false},
		}, nil)},
		{Name: "meshclaw_reminder_delete", Description: "Delete a local macOS Reminder by id or query. Destructive personal assistant tool: default execute=false returns a plan only; execute=true deletes the reminder.", InputSchema: objectSchema(map[string]mcpProperty{
			"id":      {Type: "string", Description: "Optional reminder id"},
			"query":   {Type: "string", Description: "Reminder title/query if id is not known"},
			"execute": {Type: "boolean", Description: "Actually delete the reminder. Default false returns a plan only.", Default: false},
		}, nil)},
		{Name: "meshclaw_contacts_search", Description: "Search local macOS Contacts through the Argos UI Runner Contacts helper. Read-only personal assistant tool; returns bounded contact matches without mutating contacts.", InputSchema: objectSchema(map[string]mcpProperty{
			"query": {Type: "string", Description: "Name, company, phone, or email query"},
		}, []string{"query"})},
		{Name: "meshclaw_notes_search", Description: "Search or list local macOS Notes through AppleScript. Read-only personal assistant tool; returns bounded note titles and excerpts without creating, editing, moving, exporting, or deleting notes.", InputSchema: objectSchema(map[string]mcpProperty{
			"query": {Type: "string", Description: "Optional note title/body query. Omit or pass empty string to list recent available notes."},
			"limit": {Type: "number", Description: "Maximum notes to return", Default: 10},
		}, nil)},
		{Name: "meshclaw_note_create", Description: "Create a local macOS Notes note. Mutating personal assistant tool: default execute=false returns a plan only; execute=true creates the note.", InputSchema: objectSchema(map[string]mcpProperty{
			"title":   {Type: "string", Description: "Note title", Default: "Argos Note"},
			"body":    {Type: "string", Description: "Note body"},
			"execute": {Type: "boolean", Description: "Actually create the note. Default false returns a plan only.", Default: false},
		}, []string{"body"})},
		{Name: "meshclaw_automation_clipboard_set", Description: "Copy text to the local clipboard for a user-visible handoff. Avoid secrets unless the user explicitly requested a local-only flow.", InputSchema: objectSchema(map[string]mcpProperty{
			"text": {Type: "string", Description: "Text to copy to clipboard"},
		}, []string{"text"})},
		{Name: "meshclaw_automation_ai_handoff", Description: "Copy a prompt to the local clipboard and open a user-approved AI frontend such as Codex, Claude, ChatGPT, or 1Code. This does not extract subscription tokens or call paid subscriptions as a backend API.", InputSchema: objectSchema(map[string]mcpProperty{
			"provider": {Type: "string", Description: "codex, claude, chatgpt, or 1code"},
			"prompt":   {Type: "string", Description: "Prompt/task to hand off"},
		}, []string{"provider", "prompt"})},
		{Name: "meshclaw_app_settings_plan", Description: "Plan or open a visible app/System Settings/account settings page. Default execute=false returns a review plan. execute=true requires approve=true and only opens System Settings, an app, or a URL; it does not toggle permissions, save settings, sign out, delete data, or change accounts.", InputSchema: objectSchema(map[string]mcpProperty{
			"app":     {Type: "string", Description: "App or settings area, e.g. Safari, ChatGPT, System Settings, Privacy"},
			"pane":    {Type: "string", Description: "Optional pane: accessibility, screen_recording, full_disk_access, notifications, privacy, accounts, extensions, or general"},
			"url":     {Type: "string", Description: "Optional exact settings URL. If omitted, MeshClaw chooses a conservative macOS settings URL or opens the app."},
			"execute": {Type: "boolean", Description: "Open the settings surface visibly. Default false returns a plan only.", Default: false},
			"approve": {Type: "boolean", Description: "Required with execute=true because settings pages can change user state.", Default: false},
		}, []string{"app"})},
		{Name: "meshclaw_account_action_plan", Description: "Plan or open a visible account/subscription management page for billing, cancellation, renewal, plan changes, or profile settings. Default execute=false returns a stop-before-final-action plan. execute=true requires approve=true and only opens a visible URL; it does not cancel, subscribe, pay, change plans, submit forms, or alter accounts.", InputSchema: objectSchema(map[string]mcpProperty{
			"service": {Type: "string", Description: "Service or website name, e.g. Netflix, Coupang Play, YouTube Premium, Apple, Google Workspace"},
			"action":  {Type: "string", Description: "manage, billing, cancel, renew, plan_change, profile, or auto", Default: "manage"},
			"url":     {Type: "string", Description: "Optional known management URL. If omitted, MeshClaw uses a conservative web search URL."},
			"execute": {Type: "boolean", Description: "Open the management/search URL visibly. Default false returns a plan only.", Default: false},
			"approve": {Type: "boolean", Description: "Required with execute=true because account/subscription pages can change paid services.", Default: false},
		}, []string{"service"})},
		{Name: "meshclaw_purchase_click", Description: "Final approved purchase-click handoff for logged-in shopping sites such as Coupang. Default execute=false returns a strong approval plan. execute=true requires approve=true, a strong purchase confirmation such as 구매 실행 승인/사줘/주문해/ㅇㅇ, merchant, item, total, payment/shipping summary, URL, and click coordinates; it clicks only the supplied coordinate and stores evidence.", InputSchema: objectSchema(map[string]mcpProperty{
			"merchant":            {Type: "string", Description: "Merchant/site name, e.g. Coupang"},
			"item":                {Type: "string", Description: "Item/product being purchased"},
			"total":               {Type: "string", Description: "Final total price including shipping/discounts/currency"},
			"shipping":            {Type: "string", Description: "Shipping address or redacted delivery summary confirmed by the user"},
			"payment":             {Type: "string", Description: "Redacted payment method summary confirmed by the user, e.g. card ending 1234"},
			"url":                 {Type: "string", Description: "Visible checkout/order page URL"},
			"x":                   {Type: "number", Description: "Screen x coordinate of the final purchase/order button"},
			"y":                   {Type: "number", Description: "Screen y coordinate of the final purchase/order button"},
			"runner_url":          {Type: "string", Description: "Argos UI Runner URL", Default: "http://127.0.0.1:48292"},
			"confirmation":        {Type: "string", Description: "Strong purchase confirmation from the user, e.g. 구매 실행 승인, 사줘, 주문해, ㅇㅇ, purchase execution approved, go ahead, buy it, order now"},
			"proof_path":          {Type: "string", Description: "Optional prior screenshot proof path showing final total/counterparty/button"},
			"post_wait_seconds":   {Type: "number", Description: "Seconds to wait after the click before optional post-click capture", Default: 1},
			"post_capture":        {Type: "boolean", Description: "Capture a post-click screenshot after execution. Requires the same approval.", Default: false},
			"post_capture_output": {Type: "string", Description: "Optional .png output path for the post-click screenshot proof"},
			"execute":             {Type: "boolean", Description: "Actually click the final purchase/order button. Default false returns a plan only.", Default: false},
			"approve":             {Type: "boolean", Description: "Required with execute=true because this may spend money or place an order.", Default: false},
		}, []string{"merchant", "item", "total"})},
		{Name: "meshclaw_subscription_frontends", Description: "Check local subscription frontend readiness for Codex, Claude, and ChatGPT browser lanes. Use before trying a 1Code-style visible local AI session.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_argos_macos_doctor", Description: "Check Argos local Mac assistant readiness: saved permissions, Shortcuts, logged-in AI frontends, and optional one-second screen recording permission test.", InputSchema: objectSchema(map[string]mcpProperty{
			"check_screen_recording": {Type: "boolean", Description: "Run a one-second screencapture test to verify Screen Recording permission", Default: true},
		}, nil)},
		{Name: "meshclaw_argos_macos_setup", Description: "Run the local Argos Mac setup helper: open Argos UI Runner if needed, request Accessibility permission, then run the Mac doctor. macOS permission approval still requires the user to click in System Settings.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_automation_mouse_click", Description: "Click a screen coordinate through Argos UI Runner. Requires one-time macOS Accessibility permission.", InputSchema: objectSchema(map[string]mcpProperty{
			"x":          {Type: "number", Description: "Screen x coordinate"},
			"y":          {Type: "number", Description: "Screen y coordinate"},
			"runner_url": {Type: "string", Description: "Argos UI Runner URL", Default: "http://127.0.0.1:48292"},
		}, []string{"x", "y"})},
		{Name: "meshclaw_automation_script_run", Description: "Run a local automation script through a shell. Use for Ubuntu/browser automation scripts only when the user explicitly asked for script execution.", InputSchema: objectSchema(map[string]mcpProperty{
			"script": {Type: "string", Description: "Script body or command line"},
			"shell":  {Type: "string", Description: "Shell path", Default: "/bin/sh"},
		}, []string{"script"})},
		{Name: "meshclaw_argos_ask", Description: "Classify and optionally execute a natural-language Argos macOS assistant request. Covers browser search/fetch/open, app launch, Notes note creation, Shortcuts, clipboard, and visible AI frontend handoff. Default is dry-run.", InputSchema: objectSchema(map[string]mcpProperty{
			"text":           {Type: "string", Description: "Natural-language macOS assistant request"},
			"execute":        {Type: "boolean", Description: "Actually change local UI state. Default false returns a plan only.", Default: false},
			"record_seconds": {Type: "number", Description: "Optional macOS screen recording duration to capture visible work", Default: 0},
		}, []string{"text"})},
		{Name: "meshclaw_workflow_run", Description: "Canonical orchestration tool for AI operators. Runs a MeshClaw workflow with policy checks, structured step results, approvals, and an evidence bundle. Use dry-run first unless the user explicitly asked to execute.", InputSchema: objectSchema(map[string]mcpProperty{
			"name":          {Type: "string", Description: "Workflow name"},
			"mode":          {Type: "string", Description: "dry-run or execute", Default: "dry-run"},
			"approvals_ref": {Type: "string", Description: "Optional approval bundle ref for execute mode: latest, bundle directory, or execution.json"},
			"steps":         {Type: "string", Description: "Optional comma-separated workflow step IDs to run instead of the whole workflow"},
		}, []string{"name"})},
		{Name: "meshclaw_evidence_latest", Description: "Return the latest MeshClaw Runtime evidence bundle paths and summary, including execution.json, steps.jsonl, report.html, and meshclaw-actions.md preview. Use after meshclaw_workflow_run or meshclaw_run_evidence.", InputSchema: objectSchema(map[string]mcpProperty{
			"actions_preview_bytes": {Type: "number", Description: "Maximum bytes of meshclaw-actions.md to include", Default: 4000},
		}, nil)},
		{Name: "meshclaw_evidence_list", Description: "List recent stored evidence records.", InputSchema: objectSchema(map[string]mcpProperty{"limit": {Type: "number", Description: "Maximum records to return", Default: 20}}, nil)},
		{Name: "meshclaw_data_doctor", Description: "Check local MeshClaw data growth, retention policy, public report files, doctor recordings, logs, evidence counts, and top-level state JSON validity without deleting anything.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_data_archive_plan", Description: "Plan-only evidence archival guidance for MeshClaw data growth warnings. Groups dated evidence directories, keeps the newest directories, and returns archive candidates without writing, moving, compressing, or deleting anything.", InputSchema: objectSchema(map[string]mcpProperty{
			"keep_newest": {Type: "number", Description: "Number of newest evidence date directories to keep out of the archive candidate list", Default: 14},
		}, nil)},
		{Name: "meshclaw_downloads_cleanup_plan", Description: "Plan-only local Downloads cleanup guidance. Scans direct files in Downloads, classifies installer/archive/large/old candidates, and returns review candidates without moving, renaming, archiving, or deleting anything. If macOS denies folder access, returns status=needs_access with access_error and user guidance instead of treating it as a failed cleanup.", InputSchema: objectSchema(map[string]mcpProperty{
			"path":         {Type: "string", Description: "Optional directory to scan. Defaults to ~/Downloads."},
			"min_age_days": {Type: "number", Description: "Old-file threshold in days", Default: 30},
			"large_mb":     {Type: "number", Description: "Large-file threshold in MB", Default: 500},
			"limit":        {Type: "number", Description: "Maximum candidates to return", Default: 50},
		}, nil)},
		{Name: "meshclaw_downloads_cleanup_apply", Description: "Approval-gated file cleanup apply step. Moves explicit approved regular-file paths into a review folder only; it never deletes, trashes, archives, overwrites, or moves folders. Default execute=false returns a move plan; execute=true requires approve=true.", InputSchema: objectSchema(map[string]mcpProperty{
			"paths":       {Type: "string", Description: "Comma-separated explicit file paths selected from a cleanup plan."},
			"destination": {Type: "string", Description: "Optional review folder. Defaults to an Argos Cleanup Review timestamp folder next to the first selected file."},
			"execute":     {Type: "boolean", Description: "Actually move approved files to the review folder. Default false returns a plan only.", Default: false},
			"approve":     {Type: "boolean", Description: "Required with execute=true because moving user files changes filesystem state.", Default: false},
		}, []string{"paths"})},
		{Name: "meshclaw_policy_check", Description: "Decide whether a subject can perform an action on a resource.", InputSchema: objectSchema(map[string]mcpProperty{
			"subject":  {Type: "string", Description: "Operator subject, e.g. codex, claude, local-llm, automation"},
			"action":   {Type: "string", Description: "Action name, e.g. read_state, run_command, provision_server"},
			"resource": {Type: "string", Description: "Resource name, e.g. server, secret, provider"},
			"context":  {Type: "string", Description: "Optional command or extra context"},
		}, []string{"subject", "action", "resource"})},
		{Name: "meshclaw_policy_show", Description: "Return the active MeshClaw policy config without revealing secrets.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_matrix_plan", Description: "Return the supported Matrix bridge role: ops room, approval channel, notification channel, and optional MCP surface.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_route", Description: "Classify a natural-language request before a local model answers. Use for Open WebUI/Signal/local LLM frontends: MeshClaw decides whether to execute, require approval, deny, or delegate to the model.", InputSchema: objectSchema(map[string]mcpProperty{
			"source":  {Type: "string", Description: "Source surface, e.g. openwebui, signal, argos", Default: "openwebui"},
			"subject": {Type: "string", Description: "Policy subject, e.g. local-llm, openwebui, codex, claude", Default: "local-llm"},
			"message": {Type: "string", Description: "Natural-language user request"},
		}, []string{"message"})},
		{Name: "meshclaw_ask", Description: "Run the MeshClaw Router for Open WebUI/Signal/local LLM frontends. Read-only MeshClaw intents are executed, risky intents create approval evidence, and general chat is delegated to the model.", InputSchema: objectSchema(map[string]mcpProperty{
			"source":  {Type: "string", Description: "Source surface, e.g. openwebui, signal, argos", Default: "openwebui"},
			"subject": {Type: "string", Description: "Policy subject, e.g. local-llm, openwebui, codex, claude", Default: "local-llm"},
			"message": {Type: "string", Description: "Natural-language user request"},
		}, []string{"message"})},
		{Name: "meshclaw_schedule_plan", Description: "List default scheduled jobs for local-model quick checks, briefings, and daily Codex/Claude expert audit handoff.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_schedule_status", Description: "Show scheduler health summary: status, due_count, due_jobs, next_due_job, next due time, and per-job schedule state.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_schedule_run_once", Description: "Run one scheduled MeshClaw job now. Default is dry-run: writes evidence and does not send Signal unless execute=true or target is provided.", InputSchema: objectSchema(map[string]mcpProperty{
			"job":     {Type: "string", Description: "serverops-quickcheck, morning-briefing, or daily-expert-audit"},
			"target":  {Type: "string", Description: "Optional messenger target id"},
			"execute": {Type: "boolean", Description: "Actually send configured messenger notification when true", Default: false},
		}, []string{"job"})},
		{Name: "meshclaw_ops_dispatch", Description: "Route a Matrix/Open WebUI/Codex/Claude-style message to a MeshClaw ops result.", InputSchema: objectSchema(map[string]mcpProperty{
			"source":  {Type: "string", Description: "Source surface, e.g. matrix, openwebui, codex, claude"},
			"message": {Type: "string", Description: "Natural-language or command-like message"},
		}, []string{"source", "message"})},
		{Name: "meshclaw_provision_plan", Description: "Plan temporary VPS/server capacity without creating resources.", InputSchema: objectSchema(map[string]mcpProperty{
			"purpose":        {Type: "string", Description: "Why temporary capacity is needed"},
			"budget_usd":     {Type: "number", Description: "Maximum budget cap in USD", Default: 0},
			"ttl_hours":      {Type: "number", Description: "Time-to-live for temporary capacity", Default: 6},
			"required_class": {Type: "string", Description: "Capacity class, e.g. small-vps, gpu, storage", Default: "small-vps"},
			"provider":       {Type: "string", Description: "Provider capability id", Default: "provider-api"},
			"region":         {Type: "string", Description: "Preferred region", Default: "nearest-available"},
		}, []string{"purpose"})},
		{Name: "meshclaw_run_evidence", Description: "Run one policy-checked diagnostic command and store evidence. Prefer this over direct vssh/SSH when an AI operator needs audit trail, fallback, and policy context.", InputSchema: objectSchema(map[string]mcpProperty{
			"host":    {Type: "string", Description: "Inventory host name"},
			"command": {Type: "string", Description: "Command to execute"},
		}, []string{"host", "command"})},
		{Name: "meshclaw_job_start", Description: "Start a long-running vssh daemon job and store evidence.", InputSchema: objectSchema(map[string]mcpProperty{
			"host":    {Type: "string", Description: "Inventory host name"},
			"command": {Type: "string", Description: "Command to run asynchronously"},
		}, []string{"host", "command"})},
		{Name: "meshclaw_job_status", Description: "Return vssh daemon job status and store evidence.", InputSchema: objectSchema(map[string]mcpProperty{
			"host": {Type: "string", Description: "Inventory host name"},
			"id":   {Type: "string", Description: "Job id"},
		}, []string{"host", "id"})},
		{Name: "meshclaw_job_logs", Description: "Return vssh daemon job logs and store evidence.", InputSchema: objectSchema(map[string]mcpProperty{
			"host":       {Type: "string", Description: "Inventory host name"},
			"id":         {Type: "string", Description: "Job id"},
			"tail_bytes": {Type: "number", Description: "Return only the last N bytes", Default: 0},
		}, []string{"host", "id"})},
		{Name: "meshclaw_job_cancel", Description: "Cancel a running vssh daemon job and store evidence.", InputSchema: objectSchema(map[string]mcpProperty{
			"host": {Type: "string", Description: "Inventory host name"},
			"id":   {Type: "string", Description: "Job id"},
		}, []string{"host", "id"})},
		{Name: "meshclaw_artifact_collect", Description: "Collect remote artifact metadata/content through vssh and store evidence.", InputSchema: objectSchema(map[string]mcpProperty{
			"host":      {Type: "string", Description: "Inventory host name"},
			"path":      {Type: "string", Description: "Remote file or directory path"},
			"max_bytes": {Type: "number", Description: "Maximum file bytes to return before base64 encoding", Default: 1048576},
		}, []string{"host", "path"})},
		{Name: "meshclaw_disk_investigate", Description: "Collect read-only disk evidence for a host/path.", InputSchema: objectSchema(map[string]mcpProperty{
			"host": {Type: "string", Description: "Inventory host name"},
			"path": {Type: "string", Description: "Absolute path to inspect", Default: "/"},
		}, []string{"host"})},
		{Name: "meshclaw_process_top", Description: "Collect read-only top memory and CPU process evidence for a host.", InputSchema: objectSchema(map[string]mcpProperty{
			"host": {Type: "string", Description: "Inventory host name"},
		}, []string{"host"})},
		{Name: "meshclaw_data_clean_plan", Description: "Find raw/intermediate/checkpoint cleanup candidates while preserving clean/final outputs.", InputSchema: objectSchema(map[string]mcpProperty{
			"host": {Type: "string", Description: "Inventory host name"},
			"path": {Type: "string", Description: "Absolute project path to inspect"},
		}, []string{"host", "path"})},
		{Name: "meshclaw_data_clean_apply", Description: "Apply a manifest generated by data_clean_plan.", InputSchema: objectSchema(map[string]mcpProperty{
			"host":     {Type: "string", Description: "Inventory host name"},
			"manifest": {Type: "string", Description: "Manifest path produced by data_clean_plan"},
		}, []string{"host", "manifest"})},
		{Name: "meshclaw_analyze_logs", Description: "Collect warning/error log evidence from system journal, a log file, or Docker container logs via source=container:<name>. Container findings may include autoheal_handoff.runtime_evidence_checklist; systemd findings may include unit_candidates; confirm unit identity and system/user scope before restart planning.", InputSchema: objectSchema(map[string]mcpProperty{
			"host":   {Type: "string", Description: "Inventory host name"},
			"source": {Type: "string", Description: "system, journal, syslog, an absolute log path, or container:<name>; container logs may return autoheal_handoff.runtime_evidence_checklist, and systemd logs may return unit_candidates and require unit identity before restart planning", Default: "system"},
		}, []string{"host"})},
		{Name: "meshclaw_security_check", Description: "Collect a read-only security snapshot: users, listeners, failed logins, sudoers, updates.", InputSchema: objectSchema(map[string]mcpProperty{
			"host": {Type: "string", Description: "Inventory host name"},
		}, []string{"host"})},
		{Name: "meshclaw_guard_modes", Description: "List MeshClaw Guard modes: credential, posture, and vuln. Use this to understand the secret/posture/vulnerability safety layer.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_guard_model", Description: "Return the recommended optional local Guard Chat boundary for password/token conversations. MeshClaw core remains model-less.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_guard_clients", Description: "Return local and cloud AI client safety guidance for Guard Chat: memory, RAG, history, local stores, and when to use handles instead of raw secrets.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_guard_session_policy", Description: "Return the ephemeral local Guard session policy: no memory, no RAG, no transcript, handle-only evidence, and purge-after-work rules.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_guard_signal_policy", Description: "Return the Signal-first Guard policy. Signal may receive redacted reports and approvals, but Claude/Codex/GPT cannot read raw secret ingress messages.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_guard_vault", Description: "Return the local-only vault conversation plan: handles, approval gates, model rules, and storage rules for password/token work.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_guard_vault_backends", Description: "List available local Guard vault storage backends: MeshClaw local AES-GCM, Apple Keychain, Ubuntu pass, 1Password, and Bitwarden.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_guard_vault_list", Description: "List local Guard vault handles and metadata. Raw secret values are never returned.", InputSchema: objectSchema(nil, nil)},
		{Name: "meshclaw_guard_vault_metadata", Description: "Return local Guard vault metadata for a handle. Raw secret values are never returned.", InputSchema: objectSchema(map[string]mcpProperty{
			"handle": {Type: "string", Description: "vault://meshclaw/<scope>/<name> handle"},
		}, []string{"handle"})},
		{Name: "meshclaw_guard_detect", Description: "Detect pasted credentials or sensitive tokens in text. Returns redacted evidence and replacement markers, never raw values.", InputSchema: objectSchema(map[string]mcpProperty{
			"target": {Type: "string", Description: "Source label, e.g. chat, openwebui, file path", Default: "input"},
			"text":   {Type: "string", Description: "Text to inspect before it reaches an AI model"},
		}, []string{"text"})},
		{Name: "meshclaw_guard_redact", Description: "Redact pasted credentials or sensitive tokens from text and store redacted evidence.", InputSchema: objectSchema(map[string]mcpProperty{
			"target": {Type: "string", Description: "Source label, e.g. chat, openwebui, file path", Default: "input"},
			"text":   {Type: "string", Description: "Text to redact before passing onward"},
		}, []string{"text"})},
		{Name: "meshclaw_guard_posture", Description: "Run read-only local posture checks for MeshClaw Guard state, evidence, and local-model chat stores.", InputSchema: objectSchema(map[string]mcpProperty{
			"home": {Type: "string", Description: "Optional home directory to inspect", Default: ""},
		}, nil)},
		{Name: "meshclaw_guard_clean_plan", Description: "Plan local cleanup for AI chat history, memory, and RAG stores. Read-only; deletion requires explicit future local approval.", InputSchema: objectSchema(map[string]mcpProperty{
			"home": {Type: "string", Description: "Optional home directory to inspect", Default: ""},
		}, nil)},
		{Name: "meshclaw_guard_vuln", Description: "Scan a local file or directory for redacted secret/code hygiene findings. Does not return raw values.", InputSchema: objectSchema(map[string]mcpProperty{
			"path": {Type: "string", Description: "Local file or directory path to scan"},
		}, []string{"path"})},
		{Name: "meshclaw_guard_vuln_scan", Description: "Scan a fleet node's installed packages (deb/PyPI/npm/brew/cargo) against the OSV.dev CVE database. Collects the package inventory over vssh, queries OSV, and returns CVE counts plus high-severity matches with fixed versions. Read-only and approval-free; rotation/patching is a separate gated action. This is Guard vuln-mode's host package-CVE scanner (the capability formerly exposed as pwagent vuln_scan).", InputSchema: objectSchema(map[string]mcpProperty{
			"host":       {Type: "string", Description: "Inventory host name to scan (e.g. d1)"},
			"offline":    {Type: "boolean", Description: "Inventory only; do not query OSV.dev", Default: false},
			"max_detail": {Type: "integer", Description: "Cap OSV detail (severity/summary) fetches", Default: 40},
			"ecosystems": {Type: "string", Description: "Optional CSV filter, e.g. 'deb,PyPI'. Empty = all supported."},
		}, []string{"host"})},
		{Name: "meshclaw_guard_code_review", Description: "Run deterministic SAST analyzers (semgrep + bandit) against a repository on a fleet node and return aggregated findings by tool and severity. Read-only; source snippets never leave the host, and any patching is approval-gated. This is Guard vuln-mode's AI-code-review scan (the deterministic core formerly exposed as pwagent ai_edits); LLM triage can be layered via MeshClaw's model gateway.", InputSchema: objectSchema(map[string]mcpProperty{
			"host":      {Type: "string", Description: "Inventory host name where the repo lives"},
			"repo_path": {Type: "string", Description: "Absolute path to the repository/directory to scan on the host"},
		}, []string{"host", "repo_path"})},
		{Name: "meshclaw_guard_intent", Description: "Classify a local Guard Chat message into safe structured intents such as vault_import, vault_list, vault_delete, guard_posture, or guard_vuln.", InputSchema: objectSchema(map[string]mcpProperty{
			"text": {Type: "string", Description: "Natural-language local Guard Chat message"},
		}, []string{"text"})},
		{Name: "meshclaw_hygiene_scan_host", Description: "Scan likely remote logs/config files for redacted secret and PII leak evidence.", InputSchema: objectSchema(map[string]mcpProperty{
			"host": {Type: "string", Description: "Inventory host name"},
		}, []string{"host"})},
		{Name: "meshclaw_service_check", Description: "Collect read-only systemd status, unit config, and recent logs for a service. After meshclaw_analyze_logs returns unit_candidates, pass the exact candidate .service name here before restart planning.", InputSchema: objectSchema(map[string]mcpProperty{
			"host":    {Type: "string", Description: "Inventory host name"},
			"service": {Type: "string", Description: "Exact systemd service name, preferably copied from log_findings unit_candidates when present"},
		}, []string{"host", "service"})},
		{Name: "meshclaw_service_audit", Description: "Collect read-only failed/restarting systemd service evidence for a host.", InputSchema: objectSchema(map[string]mcpProperty{
			"host": {Type: "string", Description: "Inventory host name"},
		}, []string{"host"})},
		{Name: "meshclaw_service_quarantine", Description: "Disable a flapping service only when its ExecStart target is missing.", InputSchema: objectSchema(map[string]mcpProperty{
			"host":    {Type: "string", Description: "Inventory host name"},
			"service": {Type: "string", Description: "Systemd service name"},
		}, []string{"host", "service"})},
		{Name: "meshclaw_service_remove", Description: "Stop/disable a local systemd service, remove its unit, and optionally remove its matching WorkingDirectory.", InputSchema: objectSchema(map[string]mcpProperty{
			"host":    {Type: "string", Description: "Inventory host name"},
			"service": {Type: "string", Description: "Systemd service name"},
			"path":    {Type: "string", Description: "Optional absolute WorkingDirectory to remove when it matches the unit"},
		}, []string{"host", "service"})},
	}
	return tools
}

func mcpTools() []mcpTool {
	return visibleMCPTools(allMCPTools(), canExposeMissionWriteTools())
}

func visibleMCPTools(tools []mcpTool, exposeMissionWrites bool) []mcpTool {
	if exposeMissionWrites {
		return tools
	}
	filtered := make([]mcpTool, 0, len(tools))
	for _, tool := range tools {
		if !isMissionWriteTool(tool.Name) {
			filtered = append(filtered, tool)
		}
	}
	return filtered
}

func isMissionWriteTool(name string) bool {
	switch name {
	case "mission_update", "task_add", "task_complete", "artifact_add":
		return true
	default:
		return false
	}
}

func objectSchema(properties map[string]mcpProperty, required []string) mcpInputSchema {
	if properties == nil {
		properties = map[string]mcpProperty{}
	}
	return mcpInputSchema{Type: "object", Properties: properties, Required: required}
}

type mcpEvidenceRef struct {
	ID       string    `json:"id,omitempty"`
	Kind     string    `json:"kind,omitempty"`
	Summary  string    `json:"summary,omitempty"`
	StoredAt string    `json:"stored_at,omitempty"`
	Time     time.Time `json:"time,omitempty"`
}

type mcpOpsControlResult struct {
	Kind             string               `json:"kind"`
	Decision         string               `json:"decision"`
	Summary          opsManagementSummary `json:"summary"`
	TopRisks         []string             `json:"top_risks"`
	NextActions      []string             `json:"next_actions"`
	RecommendedMCP   []recommendedMCPCall `json:"recommended_mcp"`
	ServiceItems     []mcpServiceItem     `json:"service_items,omitempty"`
	PolicyPosture    []opsPolicyPosture   `json:"policy_posture"`
	ApplySafe        bool                 `json:"apply_safe"`
	AppliedSafeCount int                  `json:"applied_safe_count"`
	Evidence         mcpEvidenceRef       `json:"evidence"`
	StoreError       string               `json:"store_error,omitempty"`
	FullReportHint   string               `json:"full_report_hint"`
}

type mcpServiceTriageResult struct {
	Kind            string               `json:"kind"`
	Decision        string               `json:"decision"`
	Counts          map[string]int       `json:"counts"`
	ServiceFindings int                  `json:"service_findings"`
	Items           []mcpServiceItem     `json:"items"`
	NextActions     []string             `json:"next_actions"`
	RecommendedMCP  []recommendedMCPCall `json:"recommended_mcp"`
	Evidence        mcpEvidenceRef       `json:"evidence"`
	StoreError      string               `json:"store_error,omitempty"`
	FullReportHint  string               `json:"full_report_hint"`
}

type mcpVSSHDaemonAuditResult struct {
	Kind           string                 `json:"kind"`
	Decision       string                 `json:"decision"`
	Summary        vsshDaemonAuditSummary `json:"summary"`
	Items          []vsshDaemonAuditItem  `json:"items"`
	Evidence       mcpEvidenceRef         `json:"evidence"`
	StoreError     string                 `json:"store_error,omitempty"`
	FullReportHint string                 `json:"full_report_hint"`
}

type mcpServiceItem struct {
	Host      string `json:"host"`
	Service   string `json:"service,omitempty"`
	Class     string `json:"class"`
	Mode      string `json:"mode"`
	Severity  string `json:"severity"`
	Judgement string `json:"judgement"`
	Next      string `json:"next"`
}

func slimOpsControlMCP(report opsControlReport, record evidence.Record, storeErr error) mcpOpsControlResult {
	decision := "healthy"
	if report.ManagementSummary.RealIncidents > 0 {
		decision = "investigate_real_incidents"
	} else if report.ManagementSummary.ApprovalRequired > 0 {
		decision = "approval_required"
	} else if report.ManagementSummary.ResourceAlerts > 0 || report.ManagementSummary.ServiceFindings > 0 {
		decision = "review_warnings"
	}
	return mcpOpsControlResult{
		Kind:             "ops_control_summary",
		Decision:         decision,
		Summary:          report.ManagementSummary,
		TopRisks:         limitStrings(report.TopRisks, 8),
		NextActions:      limitStrings(report.NextActions, 8),
		RecommendedMCP:   limitRecommendedMCPCalls(report.RecommendedMCP, 8),
		ServiceItems:     slimServiceItems(report.ServiceTriage.Items, 8),
		PolicyPosture:    report.PolicyPosture,
		ApplySafe:        report.ApplySafe,
		AppliedSafeCount: len(report.AppliedSafe),
		Evidence:         evidenceRef(record),
		StoreError:       errString(storeErr),
		FullReportHint:   "Full monitor, service audit, triage evidence, and raw command output are stored in the evidence file.",
	}
}

func slimServiceTriageMCP(report serviceTriageReport, record evidence.Record, storeErr error) mcpServiceTriageResult {
	decision := "no_service_findings"
	if report.Counts["real_incident"] > 0 {
		decision = "investigate_real_incidents"
	} else if report.Counts["approval_required"] > 0 {
		decision = "approval_required"
	} else if len(report.Items) > 0 {
		decision = "monitor_or_ignore_candidates"
	}
	next := make([]string, 0, len(report.Items))
	for _, item := range report.Items {
		if item.Next != "" {
			next = append(next, item.Next)
		}
	}
	return mcpServiceTriageResult{
		Kind:            "service_triage_summary",
		Decision:        decision,
		Counts:          report.Counts,
		ServiceFindings: report.ServiceFindings,
		Items:           slimServiceItems(report.Items, 10),
		NextActions:     limitStrings(uniqueStrings(next), 10),
		RecommendedMCP:  limitRecommendedMCPCalls(report.RecommendedMCP, 8),
		Evidence:        evidenceRef(record),
		StoreError:      errString(storeErr),
		FullReportHint:  "Full service-check evidence and raw systemd output are stored in the evidence file.",
	}
}

func slimVSSHDaemonAuditMCP(report vsshDaemonAuditReport) mcpVSSHDaemonAuditResult {
	decision := "ready"
	if report.Summary.AuthFailed > 0 {
		decision = "fix_auth"
	} else if report.Summary.UpgradeRequired > 0 {
		decision = "upgrade_daemons"
	} else if report.Summary.Unreachable > 0 {
		decision = "check_network"
	} else if report.Summary.Failed > 0 {
		decision = "investigate_daemons"
	}
	return mcpVSSHDaemonAuditResult{
		Kind:           "vssh_daemon_audit_summary",
		Decision:       decision,
		Summary:        report.Summary,
		Items:          report.Items,
		Evidence:       evidenceRef(report.Evidence),
		StoreError:     report.StoreErr,
		FullReportHint: "Full vssh daemon audit payload is stored in the evidence file.",
	}
}

func slimWorkflowMCP(result runtimeflow.Result, decision policy.Result) map[string]interface{} {
	failed := []map[string]interface{}{}
	for _, step := range result.Steps {
		if step.Success {
			continue
		}
		failed = append(failed, map[string]interface{}{
			"step":      step.Step,
			"title":     step.Title,
			"node":      step.Node,
			"transport": step.Transport,
			"retryable": step.Retryable,
			"error":     step.Error,
			"stderr":    truncate(step.Stderr, 1200),
		})
	}
	return map[string]interface{}{
		"kind":              "meshclaw_runtime_workflow_result",
		"success":           result.Success,
		"workflow":          result.Workflow,
		"mode":              result.Mode,
		"generated_at":      result.GeneratedAt,
		"summary":           result.Summary,
		"policy":            decision,
		"bundle_dir":        result.BundleDir,
		"plan_path":         result.PlanPath,
		"execution_path":    result.ExecutionPath,
		"steps_path":        result.StepsPath,
		"actions_path":      result.ActionsPath,
		"capabilities_path": result.CapabilitiesPath,
		"capability_count":  len(result.Capabilities),
		"report_path":       result.ReportPath,
		"failed":            failed,
		"next":              "Read execution_path or call meshclaw_evidence_latest for the action log preview.",
	}
}

func latestRuntimeEvidenceMCP(previewBytes int) (map[string]interface{}, error) {
	if previewBytes <= 0 {
		previewBytes = 4000
	}
	if previewBytes > 20000 {
		previewBytes = 20000
	}
	bundle, err := runtimeflow.LatestBundle()
	if err != nil {
		return nil, err
	}
	executionPath := filepath.Join(bundle, "execution.json")
	actionsPath := filepath.Join(bundle, "meshclaw-actions.md")
	reportPath := filepath.Join(bundle, "report.html")
	planPath := filepath.Join(bundle, "plan.md")
	stepsPath := filepath.Join(bundle, "steps.jsonl")
	capabilitiesPath := filepath.Join(bundle, "capabilities.json")

	var parsedSummary struct {
		Success     bool                `json:"success"`
		Workflow    string              `json:"workflow"`
		Mode        runtimeflow.Mode    `json:"mode"`
		GeneratedAt time.Time           `json:"generated_at"`
		Summary     runtimeflow.Summary `json:"summary"`
	}
	var summary interface{}
	if data, err := os.ReadFile(executionPath); err == nil {
		if json.Unmarshal(data, &parsedSummary) == nil {
			summary = parsedSummary
		}
	}
	preview := ""
	if data, err := os.ReadFile(actionsPath); err == nil {
		preview = string(data)
		if len(preview) > previewBytes {
			preview = preview[:previewBytes] + "\n..."
		}
	}
	return map[string]interface{}{
		"kind":              "meshclaw_runtime_latest_evidence",
		"success":           parsedSummary.Success,
		"workflow":          parsedSummary.Workflow,
		"mode":              parsedSummary.Mode,
		"generated_at":      parsedSummary.GeneratedAt,
		"status_summary":    parsedSummary.Summary,
		"bundle_dir":        bundle,
		"plan_path":         planPath,
		"execution_path":    executionPath,
		"steps_path":        stepsPath,
		"actions_path":      actionsPath,
		"capabilities_path": capabilitiesPath,
		"report_path":       reportPath,
		"summary":           summary,
		"actions_preview":   actionsPreviewOrHint(preview, actionsPath),
	}, nil
}

func actionsPreviewOrHint(preview, path string) string {
	if preview != "" {
		return preview
	}
	return "No meshclaw-actions.md preview available at " + path
}

type localAssistantRoute struct {
	Intent   string                 `json:"intent"`
	Tool     string                 `json:"tool"`
	Args     map[string]interface{} `json:"args"`
	Execute  bool                   `json:"execute"`
	Mutation bool                   `json:"mutation"`
	Missing  []string               `json:"missing,omitempty"`
	Safety   string                 `json:"safety"`
	Next     string                 `json:"next"`
}

func localAssistantMCP(args map[string]interface{}) (interface{}, error) {
	task := strings.TrimSpace(stringArg(args, "task"))
	if task == "" {
		return nil, fmt.Errorf("task is required")
	}
	route := localAssistantRouteForArgs(args, time.Now().Local())
	payload := map[string]interface{}{
		"kind":          "meshclaw_local_assistant",
		"task":          task,
		"intent":        route.Intent,
		"selected_tool": route.Tool,
		"arguments":     route.Args,
		"execute":       route.Execute,
		"mutation":      route.Mutation,
		"safety":        route.Safety,
		"next":          route.Next,
	}
	if len(route.Missing) > 0 {
		payload["status"] = "needs_input"
		payload["missing_fields"] = route.Missing
		return payload, nil
	}
	result, err := callMCPTool(route.Tool, route.Args)
	payload["status"] = "ok"
	payload["result"] = result
	payload["error"] = errString(err)
	return payload, nil
}

func localAssistantRouteForArgs(args map[string]interface{}, now time.Time) localAssistantRoute {
	task := strings.TrimSpace(stringArg(args, "task"))
	normalized := strings.ToLower(task)
	execute := boolArg(args, "execute", false)
	query := firstNonEmpty(stringArg(args, "query"), stringArg(args, "title"))
	title := strings.TrimSpace(stringArg(args, "title"))
	body := strings.TrimSpace(stringArg(args, "body"))
	start := strings.TrimSpace(stringArg(args, "start"))
	end := strings.TrimSpace(stringArg(args, "end"))
	due := strings.TrimSpace(stringArg(args, "due"))
	creating := containsAny(normalized, "추가", "만들", "생성", "작성", "등록", "잡아", "create", "add", "new")
	if title == "" {
		title = inferLocalAssistantTitle(task)
	}
	if start == "" || due == "" {
		inferredStart, inferredEnd := inferLocalAssistantKoreanDateTime(task, now, time.Hour)
		if start == "" {
			start = inferredStart
		}
		if end == "" {
			end = inferredEnd
		}
		if due == "" {
			due = inferredStart
		}
	}

	route := func(intent, tool, safety, next string, toolArgs map[string]interface{}, mutation bool, missing ...string) localAssistantRoute {
		if toolArgs == nil {
			toolArgs = map[string]interface{}{}
		}
		routeExecute := execute
		if value, ok := toolArgs["execute"].(bool); ok {
			routeExecute = value
		}
		return localAssistantRoute{
			Intent:   intent,
			Tool:     tool,
			Args:     toolArgs,
			Execute:  routeExecute,
			Mutation: mutation,
			Missing:  compactStrings(missing),
			Safety:   safety,
			Next:     next,
		}
	}

	switch {
	case containsAny(normalized, "스케줄", "자동화", "schedule", "due_count", "다음 실행", "상태"):
		return route("schedule_status", "meshclaw_schedule_status", "read-only schedule status", "자동화 밀림과 다음 실행 작업을 확인합니다.", nil, false)
	case containsAny(normalized, "메일", "이메일", "mail", "email", "inbox"):
		tool := "meshclaw_mail_summarize"
		intent := "mail_summarize"
		mailQuery := stringArg(args, "query")
		if containsAny(normalized, "검색", "찾", "search", "find") || strings.TrimSpace(stringArg(args, "query")) != "" {
			tool = "meshclaw_mail_search"
			intent = "mail_search"
			if strings.TrimSpace(mailQuery) == "" {
				mailQuery = inferLocalAssistantMailQuery(task)
			}
		}
		return route(intent, tool, "read-only mail access; no reply/send/delete/move", "메일은 읽기/요약만 합니다. 답장/삭제/이동/전송은 별도 승인 도구가 필요합니다.", map[string]interface{}{"query": mailQuery, "since": firstNonEmpty(stringArg(args, "since"), inferLocalAssistantMailSince(task), "24h"), "limit": intArg(args, "limit", 10)}, false)
	case containsAny(normalized, "캘린더", "일정", "calendar", "event"):
		if creating {
			missing := []string{}
			if title == "" {
				missing = append(missing, "title")
			}
			if start == "" {
				missing = append(missing, "start")
			}
			return route("calendar_create_event", "meshclaw_calendar_create_event", "mutation is plan-only unless execute=true", "일정 생성은 기본적으로 계획만 만들고, execute=true일 때만 실제 생성합니다.", map[string]interface{}{"title": title, "notes": body, "start": start, "end": end, "execute": execute}, true, missing...)
		}
		return route("calendar_list", "meshclaw_calendar_list", "read-only local Calendar query", "기본 범위는 지금부터 7일입니다.", map[string]interface{}{"start": firstNonEmpty(start, now.Format(time.RFC3339)), "end": firstNonEmpty(end, now.Add(7*24*time.Hour).Format(time.RFC3339)), "query": stringArg(args, "query")}, false)
	case containsAny(normalized, "리마인더", "미리 알림", "할 일", "todo", "reminder"):
		if creating {
			missing := []string{}
			if title == "" {
				missing = append(missing, "title")
			}
			return route("reminder_create", "meshclaw_reminder_create", "mutation is plan-only unless execute=true", "리마인더 생성은 기본적으로 계획만 만들고, execute=true일 때만 실제 생성합니다.", map[string]interface{}{"title": title, "notes": body, "due": due, "execute": execute}, true, missing...)
		}
		return route("reminders_list", "meshclaw_reminders_list", "read-only local Reminders query", "기본 범위는 지금부터 7일입니다.", map[string]interface{}{"start": firstNonEmpty(start, now.Format(time.RFC3339)), "end": firstNonEmpty(end, now.Add(7*24*time.Hour).Format(time.RFC3339)), "query": stringArg(args, "query")}, false)
	case containsAny(normalized, "연락처", "연락", "contact", "contacts"):
		contactQuery := firstNonEmpty(stringArg(args, "query"), inferLocalAssistantContactQuery(task))
		if strings.TrimSpace(contactQuery) == "" || containsAny(normalized, "아무", "누구", "anyone", "someone") {
			return route("contacts_search", "meshclaw_contacts_search", "read-only contacts search", "연락처 검색어가 필요합니다.", map[string]interface{}{"query": contactQuery}, false, "query")
		}
		return route("contacts_search", "meshclaw_contacts_search", "read-only contacts search", "연락처를 검색합니다.", map[string]interface{}{"query": contactQuery}, false)
	case containsAny(normalized, "메모", "note", "notes"):
		if creating {
			return route("note_create", "meshclaw_note_create", "mutation is plan-only unless execute=true", "메모 생성은 기본적으로 계획만 만들고, execute=true일 때만 실제 생성합니다.", map[string]interface{}{"title": firstNonEmpty(stringArg(args, "title"), inferLocalAssistantNoteTitle(task), "Argos Note"), "body": firstNonEmpty(body, task), "execute": execute}, true)
		}
		return route("notes_search", "meshclaw_notes_search", "read-only local Notes query", "메모를 검색합니다.", map[string]interface{}{"query": firstNonEmpty(stringArg(args, "query"), inferLocalAssistantNoteTitle(task)), "limit": intArg(args, "limit", 10)}, false)
	case containsAny(normalized, "ppt", "pptx", "파워포인트", "powerpoint", "발표자료", "발표 자료", "프레젠테이션", "슬라이드", "slide", "slides", "deck", "presentation", "회의자료", "회의 자료", "회의용 자료", "미팅 자료", "meeting material", "meeting materials"):
		presentationExecute := boolArg(args, "execute", true)
		return route("presentation_create", "meshclaw_presentation_create", "artifact creation runs by default for presentation requests", "PPTX와 Markdown outline을 생성하고, iPhone Signal에서 바로 열 수 있는 발표자료로 반환합니다.", map[string]interface{}{"title": firstNonEmpty(stringArg(args, "title"), inferLocalAssistantPresentationTitle(task), "Argos 발표자료"), "body": firstNonEmpty(body, task), "audience": firstNonEmpty(stringArg(args, "audience"), inferLocalAssistantPresentationAudience(task)), "slide_count": intArg(args, "slide_count", 6), "execute": presentationExecute}, true)
	case containsAny(normalized, "문서", "보고서", "docx", "document", "워드"):
		return route("document_create", "meshclaw_document_create", "artifact creation is plan-only unless execute=true", "문서 생성은 기본적으로 계획만 만들고, execute=true일 때 파일을 생성합니다.", map[string]interface{}{"title": firstNonEmpty(stringArg(args, "title"), inferLocalAssistantDocumentTitle(task), "Argos 문서"), "body": firstNonEmpty(body, task), "execute": execute}, true)
	case localAssistantShoppingSearchRequest(normalized):
		shoppingQuery := firstNonEmpty(stringArg(args, "query"), inferLocalAssistantShoppingQuery(task))
		if strings.TrimSpace(shoppingQuery) == "" {
			return route("shopping_search", "meshclaw_argos_ask", "visible shopping search only; no cart, checkout, payment, or purchase", "상품 검색어가 필요합니다. 결제/주문은 별도 최종 승인 없이는 실행하지 않습니다.", map[string]interface{}{"text": task, "execute": false}, false, "query")
		}
		shoppingURL := localAssistantShoppingSearchURL(task, shoppingQuery)
		shoppingExecute := boolArg(args, "execute", true)
		return route("shopping_search", "meshclaw_argos_ask", "visible shopping search only; no cart, checkout, payment, or purchase", "검색 결과를 열고 가격/배송/리뷰 비교 단계에서 멈춥니다. 결제/주문은 '구매 실행 승인'과 별도 최종 정보 없이는 실행하지 않습니다.", map[string]interface{}{"text": shoppingURL + " 열어줘", "execute": shoppingExecute}, false)
	case containsAny(normalized, "지도", "길찾", "가는 길", "길 찾아", "경로", "까지", "얼마나 걸", "이동시간", "소요시간", "map", "maps", "direction", "directions"):
		origin := strings.TrimSpace(stringArg(args, "origin"))
		destination := strings.TrimSpace(stringArg(args, "destination"))
		directionLike := containsAny(normalized, "길찾", "가는 길", "길 찾아", "경로", "까지", "얼마나 걸", "이동시간", "소요시간", "direction", "directions")
		if directionLike && (origin == "" || destination == "") {
			inferredOrigin, inferredDestination := inferLocalAssistantMapEndpoints(task)
			if origin == "" {
				origin = inferredOrigin
			}
			if destination == "" {
				destination = inferredDestination
			}
		}
		if destination != "" || origin != "" {
			missing := []string{}
			if destination == "" {
				missing = append(missing, "destination")
			}
			return route("maps_directions", "meshclaw_maps_directions", "execute=false returns URL only; no UI change", "길찾기 URL을 반환합니다. execute=true일 때만 지도 앱/브라우저를 엽니다.", map[string]interface{}{"origin": origin, "destination": destination, "execute": execute}, false, missing...)
		}
		mapQuery := firstNonEmpty(query, inferLocalAssistantMapSearchQuery(task))
		if strings.TrimSpace(mapQuery) == "" {
			return route("maps_search", "meshclaw_maps_search", "execute=false returns URL only; no UI change", "장소 검색어가 필요합니다.", map[string]interface{}{"query": mapQuery, "execute": execute}, false, "query")
		}
		return route("maps_search", "meshclaw_maps_search", "execute=false returns URL only; no UI change", "지도 검색 URL을 반환합니다. execute=true일 때만 지도 앱/브라우저를 엽니다.", map[string]interface{}{"query": mapQuery, "execute": execute}, false)
	case containsAny(normalized, "다운로드", "downloads"):
		return route("downloads_cleanup_plan", "meshclaw_downloads_cleanup_plan", "plan-only cleanup; no files moved/deleted", "다운로드 정리 후보만 계산합니다. 적용은 별도 승인 도구가 필요합니다.", map[string]interface{}{"limit": intArg(args, "limit", 50), "large_mb": intArg(args, "large_mb", inferLocalAssistantDownloadsLargeMB(task)), "min_age_days": intArg(args, "min_age_days", inferLocalAssistantDownloadsMinAgeDays(task))}, false)
	case containsAny(normalized, "증거", "evidence", "아카이브", "archive", "감사 로그"):
		return route("data_archive_plan", "meshclaw_data_archive_plan", "plan-only archive; no evidence deleted", "증거 보존 정책에 맞춘 아카이브 계획만 만듭니다.", map[string]interface{}{"keep_newest": intArg(args, "keep_newest", inferLocalAssistantArchiveKeepNewest(task))}, false)
	default:
		return route("argos_ask", "meshclaw_argos_ask", "generic Argos fallback; execute=false plans only", "전용 도구가 애매하면 Argos 자연어 처리로 계획을 만듭니다.", map[string]interface{}{"text": task, "execute": execute}, execute)
	}
}

func inferLocalAssistantTitle(task string) string {
	cleaned := strings.TrimSpace(task)
	if cleaned == "" {
		return ""
	}
	replacers := []string{
		"리마인더", "미리 알림", "할 일", "todo", "reminder",
		"일정", "캘린더", "calendar", "event",
		"추가해줘", "추가", "등록해줘", "등록", "생성해줘", "생성", "만들어줘", "만들",
		"작성해줘", "작성", "잡아줘", "잡아",
	}
	for _, token := range replacers {
		cleaned = strings.ReplaceAll(cleaned, token, " ")
	}
	for _, token := range []string{"오늘", "내일", "모레", "이번", "다음", "내주", "금주", "주", "달", "월요일", "월욜", "월요", "화요일", "화욜", "화요", "수요일", "수욜", "수요", "목요일", "목욜", "목요", "금요일", "금욜", "금요", "토요일", "토욜", "토요", "일요일", "일욜", "일요", "오전", "오후", "새벽", "아침", "점심", "정오", "저녁", "밤", "자정", "시에", "뒤에", "후에", "뒤", "후"} {
		cleaned = strings.ReplaceAll(cleaned, token, " ")
	}
	fields := strings.Fields(cleaned)
	kept := make([]string, 0, len(fields))
	for i, field := range fields {
		if field == "에" {
			continue
		}
		if strings.ContainsAny(field, "0123456789:") {
			continue
		}
		if localAssistantTimeUnitField(field) {
			continue
		}
		if strings.Contains(field, "시") && localAssistantLeadingNumber(field) > 0 {
			continue
		}
		if localAssistantKoreanOrArabicNumber(field) > 0 && (strings.Contains(task, field+"시") || strings.Contains(task, field+"시에")) {
			continue
		}
		if i+1 < len(fields) && strings.HasPrefix(strings.Trim(fields[i+1], ".,!?。， "), "시") && localAssistantKoreanOrArabicNumber(field) > 0 {
			continue
		}
		if i+1 < len(fields) && localAssistantTimeUnitField(fields[i+1]) && localAssistantKoreanOrArabicNumber(field) > 0 {
			continue
		}
		kept = append(kept, field)
	}
	if len(kept) == 0 {
		return ""
	}
	return strings.Join(kept, " ")
}

func inferLocalAssistantDocumentTitle(task string) string {
	cleaned := strings.TrimSpace(task)
	if cleaned == "" {
		return ""
	}
	for _, token := range []string{
		"문서", "워드", "docx", "document",
		"만들어줘", "만들", "작성해줘", "작성", "생성해줘", "생성",
	} {
		cleaned = strings.ReplaceAll(cleaned, token, " ")
	}
	return strings.Join(strings.Fields(cleaned), " ")
}

func inferLocalAssistantPresentationTitle(task string) string {
	cleaned := strings.TrimSpace(task)
	if cleaned == "" {
		return ""
	}
	for _, token := range []string{
		"PPTX", "PPT", "PowerPoint", "pptx", "ppt", "파워포인트", "powerpoint",
		"발표자료", "발표 자료", "프레젠테이션", "슬라이드", "slide", "slides", "deck", "presentation",
		"회의자료", "회의 자료", "회의용 자료", "미팅 자료", "meeting material", "meeting materials",
		"만들어줘", "만들", "작성해줘", "작성", "생성해줘", "생성", "보내줘", "보내",
	} {
		cleaned = strings.ReplaceAll(cleaned, token, " ")
	}
	return strings.Join(strings.Fields(cleaned), " ")
}

func inferLocalAssistantPresentationAudience(task string) string {
	normalized := strings.ToLower(task)
	switch {
	case containsAny(normalized, "모바일", "iphone", "아이폰", "signal", "시그널"):
		return "iPhone Signal에서 바로 확인할 사용자"
	case containsAny(normalized, "회의", "미팅", "meeting"):
		return "회의 참석자"
	default:
		return "사용자"
	}
}

func localAssistantShoppingSearchRequest(normalized string) bool {
	if containsAny(normalized, "구매 실행 승인", "결제 실행 승인", "주문 실행 승인", "approve purchase", "confirm purchase") {
		return false
	}
	return containsAny(normalized, "쿠팡", "coupang", "쇼핑", "상품", "최저가", "가격", "배송", "리뷰", "후기", "재고", "사이즈", "쿠폰", "할인", "shopping", "product", "price", "review", "stock", "coupon", "delivery") &&
		containsAny(normalized, "찾", "검색", "비교", "추천", "열어", "알아", "후보", "로켓", "search", "find", "compare", "recommend", "open")
}

func inferLocalAssistantShoppingQuery(task string) string {
	cleaned := strings.TrimSpace(task)
	if cleaned == "" {
		return ""
	}
	for _, token := range []string{
		"쿠팡에서", "쿠팡", "coupang",
		"쇼핑에서", "쇼핑",
		"로켓배송으로", "로켓 배송으로", "로켓배송", "로켓 배송", "로켓",
		"가격", "리뷰", "후기", "배송", "최저가", "재고", "사이즈", "쿠폰", "할인",
		"좋은 후보 3개", "후보 3개", "좋은 후보", "후보",
		"좋은 상품", "상품",
		"비교해줘", "비교해", "비교",
		"검색해서", "검색해줘", "검색해", "검색",
		"찾아줘", "찾아봐", "찾아", "추천해줘", "추천",
		"열어줘", "열어", "알아봐", "알아",
		"으로", "로", "에서", "좋은", "개",
	} {
		cleaned = strings.ReplaceAll(cleaned, token, " ")
	}
	cleaned = strings.NewReplacer(
		",", " ",
		".", " ",
		"?", " ",
		"!", " ",
		"(", " ",
		")", " ",
		"[", " ",
		"]", " ",
		"'", " ",
		"\"", " ",
		"，", " ",
		"。", " ",
		"？", " ",
		"！", " ",
	).Replace(cleaned)
	return strings.Join(strings.Fields(cleaned), " ")
}

func localAssistantShoppingSearchURL(task, query string) string {
	combined := strings.ToLower(task + " " + query)
	if containsAny(combined, "쿠팡", "coupang") {
		if strings.TrimSpace(query) == "" {
			return "https://www.coupang.com/"
		}
		return "https://www.coupang.com/np/search?q=" + url.QueryEscape(query)
	}
	if strings.TrimSpace(query) == "" {
		return "https://www.google.com/search?tbm=shop"
	}
	return "https://www.google.com/search?tbm=shop&q=" + url.QueryEscape(query)
}

func inferLocalAssistantNoteTitle(task string) string {
	cleaned := strings.TrimSpace(task)
	if cleaned == "" {
		return ""
	}
	for _, token := range []string{
		"메모", "note", "notes",
		"남겨줘", "남겨", "적어줘", "적어", "써줘", "써",
		"만들어줘", "만들", "작성해줘", "작성", "생성해줘", "생성",
		"검색해줘", "검색", "찾아줘", "찾아봐", "찾아", "찾", "보여줘", "알려줘",
	} {
		cleaned = strings.ReplaceAll(cleaned, token, " ")
	}
	return strings.Join(strings.Fields(cleaned), " ")
}

func inferLocalAssistantContactQuery(task string) string {
	cleaned := strings.TrimSpace(task)
	if cleaned == "" {
		return ""
	}
	for _, token := range []string{
		"연락처", "연락", "contact", "contacts",
		"검색해줘", "검색", "찾아줘", "찾아봐", "찾아", "찾", "보여줘", "알려줘",
	} {
		cleaned = strings.ReplaceAll(cleaned, token, " ")
	}
	return strings.Join(strings.Fields(cleaned), " ")
}

func inferLocalAssistantDownloadsLargeMB(task string) int {
	normalized := strings.ToLower(task)
	switch {
	case containsAny(normalized, "큰 파일", "대용량", "large", "big"):
		return 100
	default:
		return 500
	}
}

func inferLocalAssistantDownloadsMinAgeDays(task string) int {
	normalized := strings.ToLower(task)
	switch {
	case containsAny(normalized, "오래된", "옛날", "old"):
		return 30
	case containsAny(normalized, "지난달", "last month"):
		return 31
	case containsAny(normalized, "지난주", "last week"):
		return 7
	default:
		return 30
	}
}

func inferLocalAssistantArchiveKeepNewest(task string) int {
	normalized := strings.ToLower(task)
	if days := inferLocalAssistantRecentDays(normalized); days > 0 {
		return days
	}
	fields := strings.Fields(normalized)
	for i, field := range fields {
		cleaned := strings.Trim(field, ".,!?。， ")
		if strings.Contains(cleaned, "주") {
			if n := localAssistantLeadingNumber(cleaned); n > 0 {
				return n * 7
			}
			if i > 0 {
				if n := localAssistantKoreanOrArabicNumber(fields[i-1]); n > 0 {
					return n * 7
				}
			}
		}
	}
	switch {
	case containsAny(normalized, "지난달", "지난 달", "한 달", "1달", "last month"):
		return 31
	case containsAny(normalized, "지난주", "지난 주", "한 주", "1주", "last week"):
		return 7
	default:
		return 14
	}
}

func inferLocalAssistantKoreanDateTime(task string, now time.Time, duration time.Duration) (string, string) {
	normalized := strings.TrimSpace(task)
	if normalized == "" {
		return "", ""
	}
	if relative := inferLocalAssistantRelativeTime(normalized); relative > 0 {
		start := now.Add(relative).Truncate(time.Minute)
		return start.Format(time.RFC3339), start.Add(duration).Format(time.RFC3339)
	}
	dayOffset := 0
	monthDay, hasMonthDay := inferLocalAssistantMonthDay(normalized, now)
	switch {
	case strings.Contains(normalized, "모레"):
		dayOffset = 2
	case strings.Contains(normalized, "내일"):
		dayOffset = 1
	case strings.Contains(normalized, "오늘"):
		dayOffset = 0
	case hasMonthDay:
	default:
		inferredOffset, ok := inferLocalAssistantWeekdayOffset(normalized, now)
		if !ok {
			return "", ""
		}
		dayOffset = inferredOffset
	}
	hour := -1
	minute := 0
	for _, field := range strings.Fields(normalized) {
		cleaned := strings.Trim(field, ".,!?。， ")
		idx := strings.Index(cleaned, "시")
		if idx <= 0 {
			continue
		}
		raw := strings.Trim(cleaned[:idx], "에 ")
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			parsed = localAssistantKoreanOrArabicNumberPrefix(raw)
			if parsed <= 0 {
				continue
			}
		}
		hour = parsed
		break
	}
	if hour < 0 {
		if inferredHour, ok := inferLocalAssistantNamedHour(normalized); ok {
			hour = inferredHour
		} else {
			return "", ""
		}
	}
	if strings.Contains(normalized, "반") {
		minute = 30
	} else {
		for _, field := range strings.Fields(normalized) {
			cleaned := strings.Trim(field, ".,!?。， ")
			idx := strings.Index(cleaned, "분")
			if idx <= 0 {
				continue
			}
			raw := strings.Trim(cleaned[:idx], "에 ")
			if timeIdx := strings.LastIndex(raw, "시"); timeIdx >= 0 && timeIdx+len("시") < len(raw) {
				raw = strings.TrimSpace(raw[timeIdx+len("시"):])
			}
			parsed, err := strconv.Atoi(raw)
			if err != nil {
				parsed = localAssistantKoreanMinute(raw)
			}
			if parsed < 0 || parsed > 59 {
				continue
			}
			minute = parsed
			break
		}
	}
	if strings.Contains(normalized, "오후") && hour < 12 {
		hour += 12
	}
	if strings.Contains(normalized, "오전") && hour == 12 {
		hour = 0
	}
	baseYear, baseMonth, baseDay := now.Year(), now.Month(), now.Day()
	if hasMonthDay {
		baseYear, baseMonth, baseDay = monthDay.Year(), monthDay.Month(), monthDay.Day()
	}
	start := time.Date(baseYear, baseMonth, baseDay, hour, minute, 0, 0, now.Location()).AddDate(0, 0, dayOffset)
	return start.Format(time.RFC3339), start.Add(duration).Format(time.RFC3339)
}

func inferLocalAssistantNamedHour(task string) (int, bool) {
	switch {
	case strings.Contains(task, "자정"):
		return 0, true
	case strings.Contains(task, "새벽"):
		return 6, true
	case strings.Contains(task, "아침"):
		return 8, true
	case strings.Contains(task, "정오"):
		return 12, true
	case strings.Contains(task, "점심"):
		return 12, true
	case strings.Contains(task, "저녁"):
		return 19, true
	case strings.Contains(task, "밤"):
		return 21, true
	default:
		return 0, false
	}
}

func localAssistantKoreanMinute(value string) int {
	cleaned := strings.Trim(value, ".,!?。， ")
	switch cleaned {
	case "십", "십분":
		return 10
	case "이십", "스물", "스무":
		return 20
	case "삼십", "서른":
		return 30
	case "사십", "마흔":
		return 40
	case "오십", "쉰":
		return 50
	default:
		return localAssistantKoreanOrArabicNumberPrefix(cleaned)
	}
}

func inferLocalAssistantMonthDay(task string, now time.Time) (time.Time, bool) {
	fields := strings.Fields(task)
	for i, field := range fields {
		cleaned := strings.Trim(field, ".,!?。， ")
		if dotted, ok := inferLocalAssistantDottedDate(cleaned, now); ok {
			return dotted, true
		}
		if !strings.Contains(cleaned, "일") {
			continue
		}
		if monthIdx := strings.Index(cleaned, "월"); monthIdx > 0 {
			if dayIdx := strings.Index(cleaned[monthIdx+len("월"):], "일"); dayIdx > 0 {
				month, monthErr := strconv.Atoi(strings.TrimSpace(cleaned[:monthIdx]))
				day, dayErr := strconv.Atoi(strings.TrimSpace(cleaned[monthIdx+len("월") : monthIdx+len("월")+dayIdx]))
				if monthErr == nil && dayErr == nil && month > 0 && month <= 12 && day > 0 && day <= 31 {
					candidate := time.Date(now.Year(), time.Month(month), day, 0, 0, 0, 0, now.Location())
					if candidate.Month() != time.Month(month) {
						return time.Time{}, false
					}
					if candidate.Before(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())) {
						candidate = candidate.AddDate(1, 0, 0)
					}
					return candidate, true
				}
			}
		}
		day := localAssistantLeadingNumber(cleaned)
		if day <= 0 && i > 0 {
			day = localAssistantKoreanOrArabicNumber(fields[i-1])
		}
		if day <= 0 || day > 31 {
			continue
		}
		if i > 0 && strings.Contains(fields[i-1], "월") {
			month := localAssistantLeadingNumber(strings.Trim(fields[i-1], ".,!?。， "))
			if month > 0 && month <= 12 {
				year := now.Year()
				if i > 1 && strings.Contains(fields[i-2], "년") {
					if parsedYear := localAssistantLeadingNumber(strings.Trim(fields[i-2], ".,!?。， ")); parsedYear >= 1900 {
						year = parsedYear
					}
				}
				candidate := time.Date(year, time.Month(month), day, 0, 0, 0, 0, now.Location())
				if candidate.Month() != time.Month(month) {
					return time.Time{}, false
				}
				if year == now.Year() && candidate.Before(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())) {
					candidate = candidate.AddDate(1, 0, 0)
				}
				return candidate, true
			}
		}
		monthOffset := 0
		switch {
		case containsAny(task, "다음 달", "다음달", "next month"):
			monthOffset = 1
		case containsAny(task, "이번 달", "이번달", "this month"):
			monthOffset = 0
		default:
			return time.Time{}, false
		}
		candidate := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).AddDate(0, monthOffset, 0)
		candidate = time.Date(candidate.Year(), candidate.Month(), day, 0, 0, 0, 0, now.Location())
		if candidate.Month() != time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location()).AddDate(0, monthOffset, 0).Month() {
			return time.Time{}, false
		}
		return candidate, true
	}
	return time.Time{}, false
}

func inferLocalAssistantDottedDate(token string, now time.Time) (time.Time, bool) {
	separator := ""
	switch {
	case strings.Contains(token, "."):
		separator = "."
	case strings.Contains(token, "/"):
		separator = "/"
	default:
		return time.Time{}, false
	}
	parts := strings.Split(token, separator)
	year := now.Year()
	monthPart := ""
	dayPart := ""
	switch len(parts) {
	case 2:
		monthPart, dayPart = parts[0], parts[1]
	case 3:
		parsedYear, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil || parsedYear < 1900 {
			return time.Time{}, false
		}
		year = parsedYear
		monthPart, dayPart = parts[1], parts[2]
	default:
		return time.Time{}, false
	}
	month, monthErr := strconv.Atoi(strings.TrimSpace(monthPart))
	day, dayErr := strconv.Atoi(strings.TrimSpace(dayPart))
	if monthErr != nil || dayErr != nil || month <= 0 || month > 12 || day <= 0 || day > 31 {
		return time.Time{}, false
	}
	candidate := time.Date(year, time.Month(month), day, 0, 0, 0, 0, now.Location())
	if candidate.Month() != time.Month(month) {
		return time.Time{}, false
	}
	if len(parts) == 2 && candidate.Before(time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())) {
		candidate = candidate.AddDate(1, 0, 0)
	}
	return candidate, true
}

func inferLocalAssistantWeekdayOffset(task string, now time.Time) (int, bool) {
	target, ok := localAssistantKoreanWeekday(task)
	if !ok {
		return 0, false
	}
	today := int(now.Weekday())
	targetDay := int(target)
	offset := (targetDay - today + 7) % 7
	if containsAny(task, "다음 주", "다음주", "내주", "next week") {
		if offset == 0 {
			offset = 7
		}
		return offset, true
	}
	if containsAny(task, "이번 주", "이번주", "금주", "this week") {
		return offset, true
	}
	if offset == 0 {
		offset = 7
	}
	return offset, true
}

func localAssistantKoreanWeekday(task string) (time.Weekday, bool) {
	checks := []struct {
		tokens []string
		day    time.Weekday
	}{
		{[]string{"일요일", "일욜", "일요"}, time.Sunday},
		{[]string{"월요일", "월욜", "월요"}, time.Monday},
		{[]string{"화요일", "화욜", "화요"}, time.Tuesday},
		{[]string{"수요일", "수욜", "수요"}, time.Wednesday},
		{[]string{"목요일", "목욜", "목요"}, time.Thursday},
		{[]string{"금요일", "금욜", "금요"}, time.Friday},
		{[]string{"토요일", "토욜", "토요"}, time.Saturday},
	}
	for _, check := range checks {
		for _, token := range check.tokens {
			if strings.Contains(task, token) {
				return check.day, true
			}
		}
	}
	return time.Sunday, false
}

func inferLocalAssistantRelativeTime(task string) time.Duration {
	fields := strings.Fields(task)
	for i, field := range fields {
		cleaned := strings.Trim(field, ".,!?。， ")
		if !localAssistantRelativeMarkerNear(fields, i, cleaned) {
			continue
		}
		if strings.Contains(cleaned, "시간") {
			if n := localAssistantLeadingNumber(cleaned); n > 0 {
				return time.Duration(n) * time.Hour
			}
			if i > 0 {
				if n := localAssistantKoreanOrArabicNumber(fields[i-1]); n > 0 {
					return time.Duration(n) * time.Hour
				}
			}
		}
		if strings.Contains(cleaned, "분") {
			if n := localAssistantLeadingNumber(cleaned); n > 0 {
				return time.Duration(n) * time.Minute
			}
			if i > 0 {
				if n := localAssistantKoreanOrArabicNumber(fields[i-1]); n > 0 {
					return time.Duration(n) * time.Minute
				}
			}
		}
	}
	return 0
}

func localAssistantRelativeMarkerNear(fields []string, i int, cleaned string) bool {
	if strings.Contains(strings.ToLower(cleaned), "later") {
		return true
	}
	if strings.Contains(cleaned, "뒤") || strings.Contains(cleaned, "후에") || (strings.HasSuffix(cleaned, "후") && !strings.Contains(cleaned, "오후")) {
		return true
	}
	for _, idx := range []int{i - 1, i + 1} {
		if idx < 0 || idx >= len(fields) {
			continue
		}
		near := strings.Trim(fields[idx], ".,!?。， ")
		if near == "뒤" || near == "뒤에" || near == "후" || near == "후에" {
			return true
		}
	}
	return false
}

func localAssistantTimeUnitField(value string) bool {
	cleaned := strings.Trim(value, ".,!?。， ")
	return strings.Contains(cleaned, "시간") || strings.Contains(cleaned, "분")
}

func localAssistantLeadingNumber(value string) int {
	cleaned := strings.Trim(value, ".,!?。， ")
	var digits strings.Builder
	for _, r := range cleaned {
		if r < '0' || r > '9' {
			break
		}
		digits.WriteRune(r)
	}
	if digits.Len() == 0 {
		return localAssistantKoreanOrArabicNumberPrefix(cleaned)
	}
	parsed, err := strconv.Atoi(digits.String())
	if err != nil {
		return 0
	}
	return parsed
}

func localAssistantKoreanOrArabicNumberPrefix(value string) int {
	cleaned := strings.Trim(value, ".,!?。， ")
	for _, candidate := range []struct {
		token string
		value int
	}{
		{"열둘", 12}, {"열두", 12}, {"십이", 12},
		{"열하나", 11}, {"열한", 11}, {"십일", 11},
		{"다섯", 5}, {"오", 5},
		{"여섯", 6}, {"육", 6},
		{"일곱", 7}, {"칠", 7},
		{"여덟", 8}, {"팔", 8},
		{"아홉", 9}, {"구", 9},
		{"열", 10}, {"십", 10},
		{"하나", 1}, {"한", 1}, {"일", 1},
		{"둘", 2}, {"두", 2}, {"이", 2},
		{"셋", 3}, {"세", 3}, {"삼", 3},
		{"넷", 4}, {"네", 4}, {"사", 4},
	} {
		if strings.HasPrefix(cleaned, candidate.token) {
			return candidate.value
		}
	}
	return 0
}

func localAssistantKoreanOrArabicNumber(value string) int {
	cleaned := strings.Trim(value, ".,!?。， ")
	if parsed, err := strconv.Atoi(cleaned); err == nil {
		return parsed
	}
	switch cleaned {
	case "한", "하나", "일":
		return 1
	case "두", "둘", "이":
		return 2
	case "세", "셋", "삼":
		return 3
	case "네", "넷", "사":
		return 4
	case "다섯", "오":
		return 5
	case "여섯", "육":
		return 6
	case "일곱", "칠":
		return 7
	case "여덟", "팔":
		return 8
	case "아홉", "구":
		return 9
	case "열", "십":
		return 10
	case "열한", "열하나", "십일":
		return 11
	case "열두", "열둘", "십이":
		return 12
	default:
		return 0
	}
}

func inferLocalAssistantMailQuery(task string) string {
	cleaned := strings.TrimSpace(task)
	for _, token := range []string{
		"메일", "이메일", "인박스", "inbox", "mail", "email",
		"어제", "오늘", "최근", "지난주", "이번 주", "이번주", "지난달", "이번 달", "이번달", "온", "중", "에서",
		"검색해줘", "검색", "찾아줘", "찾아", "찾", "보여줘", "알려줘",
	} {
		cleaned = strings.ReplaceAll(cleaned, token, " ")
	}
	fields := strings.Fields(cleaned)
	kept := make([]string, 0, len(fields))
	for i, field := range fields {
		if i+1 < len(fields) && strings.Contains(fields[i+1], "일") && localAssistantKoreanOrArabicNumber(field) > 0 {
			continue
		}
		if strings.Contains(field, "일") && localAssistantLeadingNumber(field) > 0 {
			continue
		}
		kept = append(kept, field)
	}
	return strings.Join(kept, " ")
}

func inferLocalAssistantMailSince(task string) string {
	normalized := strings.ToLower(task)
	if days := inferLocalAssistantRecentDays(normalized); days > 0 {
		return fmt.Sprintf("%dd", days)
	}
	switch {
	case containsAny(normalized, "지난달", "지난 달", "last month"):
		return "31d"
	case containsAny(normalized, "지난주", "이번 주", "this week"):
		return "7d"
	case containsAny(normalized, "어제", "yesterday"):
		return "48h"
	case containsAny(normalized, "오늘", "today"):
		return "24h"
	default:
		return ""
	}
}

func inferLocalAssistantRecentDays(task string) int {
	if !containsAny(task, "최근", "last") {
		return 0
	}
	fields := strings.Fields(task)
	for i, field := range fields {
		cleaned := strings.Trim(field, ".,!?。， ")
		if strings.Contains(cleaned, "일") {
			if n := localAssistantLeadingNumber(cleaned); n > 0 {
				return n
			}
			if i > 0 {
				return localAssistantKoreanOrArabicNumber(fields[i-1])
			}
		}
	}
	return 0
}

func inferLocalAssistantMapEndpoints(task string) (string, string) {
	cleaned := strings.TrimSpace(task)
	if cleaned == "" {
		return "", ""
	}
	if idx := strings.Index(cleaned, "에서"); idx > 0 {
		origin := strings.TrimSpace(cleaned[:idx])
		rest := strings.TrimSpace(cleaned[idx+len("에서"):])
		if untilIdx := strings.Index(rest, "까지"); untilIdx > 0 {
			return origin, strings.TrimSpace(rest[:untilIdx])
		}
		return origin, inferLocalAssistantMapDestination(rest)
	}
	return "", inferLocalAssistantMapDestination(cleaned)
}

func inferLocalAssistantMapDestination(task string) string {
	cleaned := strings.TrimSpace(task)
	if untilIdx := strings.Index(cleaned, "까지"); untilIdx > 0 {
		return strings.TrimSpace(cleaned[:untilIdx])
	}
	candidates := []string{"가는 길", "길 찾아", "길찾기", "경로"}
	for _, token := range candidates {
		if idx := strings.Index(cleaned, token); idx > 0 {
			return strings.TrimSpace(cleaned[:idx])
		}
	}
	for _, suffix := range []string{"찾아줘", "알려줘", "보여줘", "검색해줘", "검색"} {
		cleaned = strings.TrimSuffix(cleaned, suffix)
	}
	for _, token := range []string{"지도", "길", "경로"} {
		cleaned = strings.ReplaceAll(cleaned, token, " ")
	}
	return strings.TrimSpace(cleaned)
}

func inferLocalAssistantMapSearchQuery(task string) string {
	cleaned := strings.TrimSpace(task)
	if cleaned == "" {
		return ""
	}
	for _, token := range []string{
		"지도", "map", "maps",
		"검색해줘", "검색", "찾아줘", "찾아봐", "찾아", "찾", "보여줘", "알려줘",
	} {
		cleaned = strings.ReplaceAll(cleaned, token, " ")
	}
	return strings.Join(strings.Fields(cleaned), " ")
}

func compactStrings(values []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func logRecommendationArgs(intent string) map[string]interface{} {
	args := map[string]interface{}{"host": "<host>", "source": "system"}
	normalized := strings.ToLower(intent)
	if !containsAny(normalized, "container", "docker", "컨테이너", "도커") {
		return args
	}
	tokens := normalizedIntentTokens(normalized)
	if len(tokens) == 0 {
		args["source"] = "container:<name>"
		return args
	}
	for i, token := range tokens {
		if (token == "for" || token == "on" || token == "from") && i+2 < len(tokens) {
			args["host"] = tokens[i+1]
			args["source"] = "container:" + tokens[i+2]
			return args
		}
	}
	for i := 0; i+1 < len(tokens); i++ {
		if looksLikeNodeToken(tokens[i]) && !isLogFillerToken(tokens[i+1]) {
			args["host"] = tokens[i]
			args["source"] = "container:" + tokens[i+1]
			return args
		}
	}
	if container := lastNonLogToken(tokens); container != "" {
		args["source"] = "container:" + container
	} else {
		args["source"] = "container:<name>"
	}
	return args
}

func normalizedIntentTokens(value string) []string {
	replacer := strings.NewReplacer(",", " ", ".", " ", ":", " ", ";", " ", "(", " ", ")", " ", "[", " ", "]", " ")
	fields := strings.Fields(replacer.Replace(value))
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.Trim(field, `"'`)
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

func looksLikeNodeToken(token string) bool {
	if len(token) < 2 {
		return false
	}
	hasLetter := false
	hasDigit := false
	for _, r := range token {
		if r >= 'a' && r <= 'z' {
			hasLetter = true
			continue
		}
		if r >= '0' && r <= '9' {
			hasDigit = true
			continue
		}
		return false
	}
	return hasLetter && hasDigit
}

func lastNonLogToken(tokens []string) string {
	for i := len(tokens) - 1; i >= 0; i-- {
		if isLogFillerToken(tokens[i]) {
			continue
		}
		return tokens[i]
	}
	return ""
}

func isLogFillerToken(token string) bool {
	switch token {
	case "log", "logs", "journal", "container", "docker", "show", "check", "for", "on", "from", "recent", "latest", "tail", "로그", "컨테이너", "도커", "확인", "확인해", "확인해줘", "봐줘", "보여줘", "분석", "분석해", "분석해줘", "최근", "최신":
		return true
	default:
		return false
	}
}

func desiredStatePathArg(intent string) string {
	for _, token := range strings.Fields(intent) {
		token = strings.Trim(token, `"'.,;()[]`)
		lower := strings.ToLower(token)
		if strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml") {
			return token
		}
	}
	return "<desired-state-yaml>"
}

func evidenceJSONPathArg(intent, fallback string) string {
	for _, token := range strings.Fields(intent) {
		token = strings.Trim(token, `"'.,;()[]`)
		if strings.HasSuffix(strings.ToLower(token), ".json") {
			return token
		}
	}
	return fallback
}

func recommendToolMCP(intent, subject string) map[string]interface{} {
	normalized := strings.ToLower(strings.TrimSpace(intent))
	if strings.TrimSpace(subject) == "" {
		subject = "codex"
	}
	rec := map[string]interface{}{
		"kind":       "meshclaw_tool_recommendation",
		"intent":     intent,
		"subject":    subject,
		"layer_rule": "Use MeshClaw for policy/state/evidence/workflows. Use direct vssh only for low-level structured transport primitives or debugging.",
	}
	set := func(tool, reason string, arguments map[string]interface{}, alternatives []string, avoid []string, approval bool) map[string]interface{} {
		rec["recommended_tool"] = tool
		rec["reason"] = reason
		rec["arguments"] = arguments
		rec["alternatives"] = alternatives
		rec["avoid"] = avoid
		rec["approval_expected"] = approval
		rec["execution_policy"] = map[string]interface{}{
			"recommendation_only": true,
			"executes_changes":    false,
			"requires_approval":   approval,
			"approval_gate":       "Use explicit policy/evidence approval before any mutating apply tool.",
			"required_evidence":   recommendationRequiredEvidence(tool, approval),
			"stop_before":         recommendationStopBefore(tool, approval),
		}
		rec["evidence_contract"] = map[string]interface{}{
			"grounding":              "Use returned state, logs, stdout/stderr, and evidence paths before reasoning.",
			"expected_sections":      []string{"evidence", "signals", "interpretation", "likely_causes", "recommended_checks", "remediation_options", "rollback"},
			"confidence_expected":    true,
			"safe_checks_first":      true,
			"mutating_actions_gated": true,
		}
		return rec
	}

	switch {
	case normalized == "":
		return set("meshclaw_ai_guide", "No intent was supplied; ask for the canonical guide first.", map[string]interface{}{}, nil, nil, false)
	case containsAny(normalized, "inventory override", "role override", "tag override", "mail-server", "automation-worker", "ollama-worker", "is mail-server", "is automation-worker", "is ollama-worker", "set role", "set tag", "node role", "node tag", "노드 역할", "역할 등록", "역할 수정", "역할로", "태그 등록", "태그 수정", "메일서버로", "자동화 노드", "자동화 서버"):
		return set("meshclaw_inventory_override_set", "User is clarifying private fleet meaning. Persist it as an operator-owned inventory override before capability selection.", map[string]interface{}{"node": "<node>", "role": "<role>", "tags": "<comma-separated-tags>"}, []string{"meshclaw_inventory_override_list", "meshclaw_capability_recommend"}, []string{"hardcoding private node meaning in source", "choosing placement from memory"}, false)
	case containsAny(normalized, "inventory plan", "role recommendation", "tag recommendation", "auto tag", "auto role", "인벤토리 추천", "역할 추천", "태그 추천", "자동 태그", "자동 역할", "인벤토리 업데이트 추천"):
		return set("meshclaw_agent_inventory_plan", "Use cached workload state to propose inventory role/tag overrides before applying them.", map[string]interface{}{"apply": false}, []string{"meshclaw_agent_workloads", "meshclaw_inventory_override_set"}, []string{"hardcoding roles from hostname only"}, false)
	case containsAny(normalized, "which node", "which model", "which api", "which capability", "where should", "where to run", "placement", "choose node", "choose model", "choose api", "capability", "어느 노드", "어떤 노드", "어디서", "어디에", "어떤 모델", "무슨 모델", "어떤 api", "배치", "추천해"):
		return set("meshclaw_capability_recommend", "The user is asking for placement or capability selection. Score capabilities before running a workflow or raw command.", map[string]interface{}{"intent": intent}, []string{"meshclaw_server_list", "meshclaw_inventory_override_list", "meshclaw_placement_plan"}, []string{"guessing from hostname alone", "using stale chat history as placement state"}, false)
	case containsAny(normalized, "combined", "mail and ollama", "email and ollama", "mail ops", "ops orchestration", "policy and evidence", "통합", "메일", "올라마", "오케스트레이션"):
		return set("meshclaw_workflow_run", "Combined mail/model/server orchestration should use the product workflow so inventory, policy gates, vssh execution, failure capture, and evidence are recorded together.", map[string]interface{}{"name": "meshclaw-ops-orchestration-demo", "mode": "dry-run"}, []string{"meshclaw_workflow_inspect", "meshclaw_evidence_latest"}, []string{"manual replay from chat history", "raw SSH/vssh as the top-level orchestrator"}, false)
	case containsAny(normalized, "workflow", "run demo", "demo", "재현", "데모", "워크플로", "반복 실행"):
		return set("meshclaw_workflow_run", "Multi-step repeatable operations should be run as MeshClaw workflows so plan, policy, steps, and evidence are captured.", map[string]interface{}{"name": "meshclaw-ops-orchestration-demo", "mode": "dry-run"}, []string{"meshclaw_workflow_list", "meshclaw_evidence_latest"}, []string{"manual replay from chat history"}, false)
	case containsAny(normalized, "what should i use", "which tool", "어떤 도구", "뭘 써", "권장", "가이드", "meshclaw vs vssh"):
		return set("meshclaw_ai_guide", "The user is asking which layer/tool to use.", map[string]interface{}{}, []string{"meshclaw_tool_recommend"}, nil, false)
	case containsAny(normalized, "schedule-runner", "schedule runner", "scheduler daemon", "schedule daemon", "launchd one-shot", "스케줄러 데몬", "스케줄 데몬", "자동화 데몬"):
		return set("meshclaw_daemon_schedule_status", "The user is asking whether the launchd schedule-runner is installed, idle, failed, or healthy. Include scheduler due/next-due summary to avoid misreading one-shot idle as failure.", map[string]interface{}{}, []string{"meshclaw_schedule_status", "meshclaw_setup_signal"}, []string{"treating running=false as failure without checking last_status and due_count"}, false)
	case containsAny(normalized, "schedule status", "scheduler status", "due jobs", "due_count", "due count", "next due", "automation backlog", "automation current", "자동화 상태", "자동화 밀", "밀렸", "밀린", "스케줄 상태", "다음 자동화", "다음 실행", "due 작업"):
		return set("meshclaw_schedule_status", "The user is asking whether scheduled automation is current or backlogged. Use the scheduler health summary: status, due_count, due_jobs, and next_due_job.", map[string]interface{}{}, []string{"meshclaw_daemon_schedule_status", "meshclaw_setup_signal"}, []string{"checking launchd running state alone"}, false)
	case containsAny(normalized, "data doctor", "data status", "storage status", "retention", "evidence growth", "log growth", "state json", "invalid json", "corrupt state", "데이터 상태", "저장소 상태", "저장 상태", "쌓이는 데이터", "데이터 쌓", "데이터 꼬", "꼬이지", "자동 정리 상태", "evidence 상태", "로그 용량", "상태 파일", "json 깨"):
		return set("meshclaw_data_doctor", "The user is asking whether MeshClaw's local data, reports, logs, evidence, and state JSON files are healthy. This checks state JSON integrity and is a read-only doctor, not a deletion plan.", map[string]interface{}{}, []string{"meshclaw_evidence_list", "meshclaw_schedule_run_once"}, []string{"meshclaw_data_clean_plan unless the user explicitly asks to delete data"}, false)
	case containsAny(normalized, "power outage", "power event", "power dip", "power quality", "ups", "voltage", "brownout", "blackout", "simultaneous reboot", "correlated reboot", "boot identity", "정전", "전원 나갔", "전원 이벤트", "전압", "순간 전압", "전압강하", "순간 정전", "동시 재부팅", "동시에 재부팅", "같이 꺼", "같이 떨어", "여러 서버가 동시에"):
		return set("meshclaw_opsdb_power_events", "Power, voltage, UPS, and correlated reboot questions should start from OpsDB boot-history correlation before blaming an application or running repair actions.", map[string]interface{}{"window": "15m", "uptime_max": "2h", "min_nodes": 2, "record": false}, []string{"meshclaw_opsdb_events", "meshclaw_monitor_check", "meshclaw_agent_changes"}, []string{"restarting services before checking boot correlation", "assuming CPU/GPU load caused the outage"}, false)
	case containsAny(normalized, "desired-state", "desired state", "desired yaml", "desired-state yaml", "yaml", "원하는 상태", "희망 상태", "목표 상태", "reconcile") && containsAny(normalized, "execution preview", "preview", "command preview", "실행 미리보기", "명령 미리보기"):
		return set("meshclaw_reconcile_execution_preview", "Desired-state execution preview renders inert command templates from apply-plan evidence for review. It previews only and never executes commands.", map[string]interface{}{"apply_plan_evidence_path": evidenceJSONPathArg(intent, "<reconcile-apply-plan-evidence>")}, []string{"meshclaw_reconcile_apply_plan", "meshclaw_reconcile_verification_plan", "meshclaw_reconcile_runbook"}, []string{"running preview commands", "copying inert templates into live shell"}, true)
	case containsAny(normalized, "desired-state", "desired state", "desired yaml", "desired-state yaml", "yaml", "원하는 상태", "희망 상태", "목표 상태", "reconcile") && containsAny(normalized, "apply plan", "apply-plan", "적용 계획"):
		return set("meshclaw_reconcile_apply_plan", "Desired-state apply-plan builds structured apply steps only after a ready apply-gate evidence record. It is plan-only and never executes server changes.", map[string]interface{}{"gate_evidence_path": evidenceJSONPathArg(intent, "<reconcile-apply-gate-evidence>")}, []string{"meshclaw_reconcile_apply_gate", "meshclaw_reconcile_execution_preview", "meshclaw_reconcile_runbook"}, []string{"executing apply steps from chat", "skipping apply-gate evidence"}, true)
	case containsAny(normalized, "desired-state", "desired state", "desired yaml", "desired-state yaml", "yaml", "원하는 상태", "희망 상태", "목표 상태", "reconcile") && containsAny(normalized, "readiness", "readiness summary", "ready summary", "준비 요약", "준비 상태 요약"):
		return set("meshclaw_reconcile_readiness_summary", "Desired-state readiness summary is the final summary-only view over completion-plan evidence. It reports blockers and ready stages without executing server changes.", map[string]interface{}{"completion_plan_evidence_path": evidenceJSONPathArg(intent, "<reconcile-completion-plan-evidence>")}, []string{"meshclaw_reconcile_completion_plan", "meshclaw_evidence_latest"}, []string{"treating readiness summary as approval", "executing changes from summary text"}, false)
	case containsAny(normalized, "desired-state", "desired state", "desired yaml", "desired-state yaml", "yaml", "원하는 상태", "희망 상태", "목표 상태", "reconcile") && containsAny(normalized, "completion", "completion plan", "완료 계획", "완료 조건"):
		return set("meshclaw_reconcile_completion_plan", "Desired-state completion planning records final evidence requirements from rollback-plan evidence. It is plan-only and never executes server changes.", map[string]interface{}{"rollback_plan_evidence_path": evidenceJSONPathArg(intent, "<reconcile-rollback-plan-evidence>")}, []string{"meshclaw_reconcile_rollback_plan", "meshclaw_reconcile_readiness_summary", "meshclaw_evidence_latest"}, []string{"declaring completion without evidence requirements", "mutating servers during completion planning"}, true)
	case containsAny(normalized, "desired-state", "desired state", "desired yaml", "desired-state yaml", "yaml", "원하는 상태", "희망 상태", "목표 상태", "reconcile") && containsAny(normalized, "rollback", "rollback plan", "롤백", "롤백 계획"):
		return set("meshclaw_reconcile_rollback_plan", "Desired-state rollback planning builds review-only rollback guidance from ready runbook-check evidence. It never mutates servers.", map[string]interface{}{"runbook_check_evidence_path": evidenceJSONPathArg(intent, "<reconcile-runbook-check-evidence>")}, []string{"meshclaw_reconcile_runbook_check", "meshclaw_reconcile_completion_plan", "meshclaw_reconcile_readiness_summary"}, []string{"starting changes without rollback guidance", "treating rollback plan as an automatic rollback"}, true)
	case containsAny(normalized, "desired-state", "desired state", "desired yaml", "desired-state yaml", "yaml", "원하는 상태", "희망 상태", "목표 상태", "reconcile") && containsAny(normalized, "runbook check", "runbook-check", "런북 체크", "런북 검증"):
		return set("meshclaw_reconcile_runbook_check", "Desired-state runbook-check validates runbook evidence before any future executor consumes it. It is gate-only and never executes server changes.", map[string]interface{}{"runbook_evidence_path": evidenceJSONPathArg(intent, "<reconcile-runbook-evidence>")}, []string{"meshclaw_reconcile_runbook", "meshclaw_reconcile_rollback_plan", "meshclaw_reconcile_completion_plan"}, []string{"skipping runbook-check evidence", "executing from an unchecked runbook"}, true)
	case containsAny(normalized, "desired-state", "desired state", "desired yaml", "desired-state yaml", "yaml", "원하는 상태", "희망 상태", "목표 상태", "reconcile") && containsAny(normalized, "runbook", "런북", "작업 절차", "실행 절차"):
		return set("meshclaw_reconcile_runbook", "Desired-state runbooks are review-only artifacts built after verification planning. They document ordered checks and rollback context without executing server changes.", map[string]interface{}{"verification_plan_evidence_path": evidenceJSONPathArg(intent, "<reconcile-verification-plan-evidence>")}, []string{"meshclaw_reconcile_verification_plan", "meshclaw_reconcile_runbook_check", "meshclaw_reconcile_rollback_plan"}, []string{"treating runbook text as automatic execution", "running server mutations without runbook-check evidence"}, true)
	case containsAny(normalized, "automation rule", "automation-rule", "autoheal rule", "자동화 규칙", "자동 복구", "자동복구") && containsAny(normalized, "writer plan", "write plan", "rule writer", "작성 계획", "쓰기 계획", "규칙 작성"):
		return set("meshclaw_automation_rule_writer_plan", "Automation rule writer planning previews the rule envelope a future writer would create from ready readiness evidence. It never writes schedules, policies, or live automation rules.", map[string]interface{}{"readiness_evidence_path": evidenceJSONPathArg(intent, "<automation-rule-readiness-summary-evidence>"), "rule_store": "~/.meshclaw/rules"}, []string{"meshclaw_automation_rule_readiness_summary", "meshclaw_messenger_approval_request", "meshclaw_mcp_smoke_test_plan"}, []string{"writing rule files from chat", "treating writer-plan evidence as an enabled rule", "running automation without a separate writer and approval"}, true)
	case containsAny(normalized, "automation rule", "automation-rule", "autoheal rule", "자동화 규칙", "자동 복구", "자동복구") && containsAny(normalized, "readiness", "readiness summary", "ready summary", "준비 요약", "준비 상태 요약", "준비 상태"):
		return set("meshclaw_automation_rule_readiness_summary", "Automation rule readiness summary is the final summary-only view over rule-check evidence. It reports blockers and ready stages without writing rules or running automation.", map[string]interface{}{"rule_check_evidence_path": evidenceJSONPathArg(intent, "<automation-rule-check-evidence>")}, []string{"meshclaw_automation_rule_check", "meshclaw_evidence_latest"}, []string{"treating readiness summary as approval", "writing automation rules from summary text", "running automation from summary text"}, false)
	case containsAny(normalized, "automation rule check", "automation-rule check", "rule check", "rule gate", "automation gate", "rule approval gate", "자동화 규칙 검증", "자동화 규칙 체크", "규칙 검증", "규칙 체크", "자동화 게이트"):
		return set("meshclaw_automation_rule_check", "Automation rule checks validate rule-plan evidence before any future writer consumes it. Gate-only; never writes schedules, policies, or live automation rules.", map[string]interface{}{"rule_plan_evidence_path": evidenceJSONPathArg(intent, "<automation-rule-plan-evidence>"), "approved_by": "<operator>"}, []string{"meshclaw_automation_rule_plan", "meshclaw_mcp_smoke_test_plan", "meshclaw_messenger_approval_request"}, []string{"writing automation rules from unchecked plans", "treating approval as auto-apply permission", "skipping cooldown and rollback checks"}, true)
	case containsAny(normalized, "automation rule", "autoheal rule", "auto-heal rule", "automatic repair", "auto repair", "auto recovery", "자동화 규칙", "자동 복구", "자동복구", "자동 자가치유", "자동 조치", "죽으면 자동", "실패하면 자동", "복구되게", "자동화 설정"):
		return set("meshclaw_automation_rule_plan", "Automation setup should start as a rule plan that binds trigger, condition, evidence, approval mode, action chain, verification, and rollback. It never writes schedules, policies, or live automation rules.", map[string]interface{}{"name": "<automation-rule>", "trigger": "<trigger-signal>", "condition": "<evidence-condition>", "action": "<plan-only-action-chain>", "scope": "fleet", "auto_apply": false}, []string{"meshclaw_autoheal_plan", "meshclaw_autoheal_container_apply_plan", "meshclaw_messenger_approval_request", "meshclaw_reconcile_runbook_check"}, []string{"enabling auto-apply from chat", "running repair before evidence and approval gates", "creating loops without rollback and cooldown"}, true)
	case containsAny(normalized, "desired-state", "desired state", "desired yaml", "desired-state yaml", "yaml", "원하는 상태", "희망 상태", "목표 상태", "reconcile") && containsAny(normalized, "apply gate", "gate", "approval evidence", "승인 증거", "승인 게이트", "적용 게이트"):
		return set("meshclaw_reconcile_apply_gate", "Desired-state apply readiness must pass through approval evidence validation before any future executor consumes the plan. This is gate-only and never mutates servers.", map[string]interface{}{"desired_path": desiredStatePathArg(intent), "approval_evidence_path": evidenceJSONPathArg(intent, "<reconcile-approval-request-evidence>"), "approved_by": "<operator>"}, []string{"meshclaw_reconcile_approval_request", "meshclaw_reconcile_apply_plan", "meshclaw_reconcile_runbook"}, []string{"executing desired state without approval evidence", "treating gate readiness as execution"}, true)
	case containsAny(normalized, "desired-state", "desired state", "desired yaml", "desired-state yaml", "yaml", "원하는 상태", "희망 상태", "목표 상태", "reconcile") && containsAny(normalized, "approval", "approval request", "approve", "승인 요청", "승인받", "승인 준비"):
		return set("meshclaw_reconcile_approval_request", "Desired-state changes that may require approval should produce approval-request evidence before apply-gate or apply-plan steps. This records required approvals and denied actions without executing.", map[string]interface{}{"desired_path": desiredStatePathArg(intent), "actual_report_path": "<optional-actual-report>"}, []string{"meshclaw_reconcile_plan", "meshclaw_reconcile_apply_gate", "meshclaw_reconcile_runbook"}, []string{"executing reconcile actions before approval request evidence", "manual approval notes outside evidence"}, true)
	case containsAny(normalized, "desired-state", "desired state", "desired yaml", "desired-state yaml", "yaml", "원하는 상태", "희망 상태", "목표 상태") && containsAny(normalized, "validate", "validation", "검증", "확인"):
		return set("meshclaw_reconcile_validate_desired", "Desired-state YAML should be validated first and stored as evidence before any reconciliation plan or apply gate is considered. YAML apply/execute/auto_apply keys are warnings only and never grant approval.", map[string]interface{}{"desired_path": desiredStatePathArg(intent)}, []string{"meshclaw_reconcile_plan", "meshclaw_reconcile_run_once"}, []string{"applying desired state before validation", "editing live servers from YAML directly", "treating YAML apply/execute/auto_apply keys as approval"}, false)
	case containsAny(normalized, "desired-state", "desired state", "desired yaml", "desired-state yaml", "yaml", "원하는 상태", "희망 상태", "목표 상태", "reconcile", "조정 계획", "상태 맞춰", "상태 맞추"):
		return set("meshclaw_reconcile_plan", "Desired-state reconciliation starts with a dry-run plan from YAML and actual reports. It never applies changes; approval and apply-gate evidence come later.", map[string]interface{}{"desired_path": desiredStatePathArg(intent), "actual_report_path": "<optional-actual-report>"}, []string{"meshclaw_reconcile_validate_desired", "meshclaw_reconcile_approval_request", "meshclaw_reconcile_apply_gate"}, []string{"kubectl-style live apply", "mutating servers before approval evidence"}, false)
	case containsAny(normalized, "container", "docker", "컨테이너", "도커") && containsAny(normalized, "executor verify", "executor verification", "post-action", "post action", "closeout", "close out", "after execution", "after execute", "실행 후 검증", "사후 검증", "완료 검증", "종료 검증", "닫기 검증"):
		return set("meshclaw_autoheal_container_executor_verify", "Container executor verification gates closeout after live executor evidence. It requires post-action agent-collect and focused container-logscan evidence for every executed step and never executes docker commands.", map[string]interface{}{"container_executor_evidence_path": evidenceJSONPathArg(intent, "<container-executor-evidence>"), "agent_evidence_paths": "<post-action-agent-evidence.json>", "container_logscan_evidence_paths": "<post-action-container-logscan-evidence.json>"}, []string{"meshclaw_autoheal_container_executor", "meshclaw_analyze_logs", "meshclaw_evidence_latest"}, []string{"declaring self-heal complete from executor output alone", "skipping post-action container-logscan evidence", "running docker commands during verification"}, true)
	case containsAny(normalized, "container", "docker", "컨테이너", "도커") && containsAny(normalized, "executor gate", "execution gate", "실행 게이트", "executor admission", "admission gate"):
		return set("meshclaw_autoheal_container_executor_gate", "Container executor gate validates readiness-summary evidence, operator approval, dry-run, rollback, runtime, and final logscan gates before the executor is even considered.", map[string]interface{}{"container_readiness_summary_evidence_path": evidenceJSONPathArg(intent, "<container-readiness-summary-evidence>"), "approved_by": "<operator>", "dry_run": true}, []string{"meshclaw_autoheal_container_readiness_summary", "meshclaw_autoheal_container_executor"}, []string{"treating readiness summary as approval", "setting execute=true before dry-run executor preview", "executing docker commands from gate evidence"}, true)
	case containsAny(normalized, "container", "docker", "컨테이너", "도커") && containsAny(normalized, "executor", "execute container", "live executor", "dry-run executor", "dry run executor", "실행기", "실제 실행", "라이브 실행"):
		return set("meshclaw_autoheal_container_executor", "Container executor defaults to dry-run preview and only runs live with ready executor-gate evidence, matching approved_by, dry_run=false, execute=true, and the exact live approval phrase.", map[string]interface{}{"container_executor_gate_evidence_path": evidenceJSONPathArg(intent, "<container-executor-gate-evidence>"), "approved_by": "<operator>", "dry_run": true}, []string{"meshclaw_autoheal_container_executor_gate", "meshclaw_autoheal_container_executor_verify"}, []string{"running without executor-gate evidence", "running without exact live approval phrase", "trusting docker commands from chat text"}, true)
	case containsAny(normalized, "container", "docker", "컨테이너", "도커") && containsAny(normalized, "readiness", "readiness summary", "ready summary", "apply loop", "apply-loop", "apply loop gate", "apply-loop gate", "executor gate", "실행 게이트", "적용 루프", "적용 루프 게이트", "준비 요약", "준비 상태 요약"):
		return set("meshclaw_autoheal_container_readiness_summary", "Container readiness summary is the final summary-only view over completion-plan evidence and apply-loop gates. It reports blockers and ready stages without treating readiness as approval or executing docker commands.", map[string]interface{}{"container_completion_plan_evidence_path": evidenceJSONPathArg(intent, "<container-completion-plan-evidence>")}, []string{"meshclaw_autoheal_container_completion_plan", "meshclaw_evidence_latest"}, []string{"treating readiness summary as approval", "executing docker commands from summary text", "skipping apply-loop gate evidence"}, false)
	case containsAny(normalized, "container", "docker", "컨테이너", "도커") && containsAny(normalized, "completion", "completion plan", "완료 계획", "완료 조건"):
		return set("meshclaw_autoheal_container_completion_plan", "Container completion planning records final evidence requirements, including final container-logscan evidence. It never executes docker commands.", map[string]interface{}{"container_rollback_plan_evidence_path": evidenceJSONPathArg(intent, "<container-rollback-plan-evidence>")}, []string{"meshclaw_autoheal_container_rollback_plan", "meshclaw_autoheal_container_readiness_summary", "meshclaw_analyze_logs"}, []string{"declaring container repair complete without logscan evidence", "mutating containers during completion planning"}, true)
	case containsAny(normalized, "container", "docker", "컨테이너", "도커") && containsAny(normalized, "rollback", "rollback plan", "롤백", "롤백 계획"):
		return set("meshclaw_autoheal_container_rollback_plan", "Container rollback planning builds review-only rollback guidance from ready runbook-check evidence. It never executes docker commands.", map[string]interface{}{"container_runbook_check_evidence_path": evidenceJSONPathArg(intent, "<container-runbook-check-evidence>")}, []string{"meshclaw_autoheal_container_runbook_check", "meshclaw_autoheal_container_completion_plan", "meshclaw_autoheal_container_readiness_summary"}, []string{"starting remediation without rollback guidance", "treating rollback plan as automatic rollback"}, true)
	case containsAny(normalized, "container", "docker", "컨테이너", "도커") && containsAny(normalized, "runbook check", "runbook-check", "런북 체크", "런북 검증"):
		return set("meshclaw_autoheal_container_runbook_check", "Container runbook-check validates runbook evidence before any future executor consumes it. It is gate-only and never executes docker commands.", map[string]interface{}{"container_runbook_evidence_path": evidenceJSONPathArg(intent, "<container-runbook-evidence>")}, []string{"meshclaw_autoheal_container_runbook", "meshclaw_autoheal_container_rollback_plan", "meshclaw_autoheal_container_completion_plan"}, []string{"skipping runbook-check evidence", "executing unchecked runbook steps"}, true)
	case containsAny(normalized, "container", "docker", "컨테이너", "도커") && containsAny(normalized, "runbook", "런북", "작업 절차", "실행 절차"):
		return set("meshclaw_autoheal_container_runbook", "Container runbooks are review-only remediation artifacts built from verification-plan evidence and preserve apply_plan_runtime_evidence_required_count. They never execute docker commands.", map[string]interface{}{"container_verification_plan_evidence_path": evidenceJSONPathArg(intent, "<container-verification-plan-evidence>")}, []string{"meshclaw_autoheal_container_verification_plan", "meshclaw_autoheal_container_runbook_check", "meshclaw_autoheal_container_rollback_plan"}, []string{"treating runbook text as execution", "manual docker repair without runbook evidence"}, true)
	case containsAny(normalized, "container", "docker", "컨테이너", "도커") && containsAny(normalized, "verification", "verify", "검증 계획", "확인 계획"):
		return set("meshclaw_autoheal_container_verification_plan", "Container verification planning builds post-action checks and focused container-logscan requirements from apply-plan evidence while preserving apply_plan_runtime_evidence_required_count. It never executes docker commands.", map[string]interface{}{"container_apply_plan_evidence_path": evidenceJSONPathArg(intent, "<container-apply-plan-evidence>")}, []string{"meshclaw_autoheal_container_apply_plan", "meshclaw_autoheal_container_runbook", "meshclaw_analyze_logs"}, []string{"assuming restart succeeded without logscan evidence", "executing docker commands during verification planning"}, true)
	case containsAny(normalized, "container", "docker", "컨테이너", "도커") && containsAny(normalized, "apply plan", "apply-plan", "적용 계획"):
		return set("meshclaw_autoheal_container_apply_plan", "Container apply-plan builds approval-gated restart step templates from autoheal-plan evidence and returns container_apply_plan_contract.direct_restart_allowed=false, requires_focused_runtime_evidence=true, and runtime_evidence_required_count. It is plan-only and never executes docker commands.", map[string]interface{}{"plan_evidence_path": evidenceJSONPathArg(intent, "<autoheal-plan-evidence>"), "approved_by": "<operator>"}, []string{"meshclaw_autoheal_plan", "meshclaw_autoheal_container_verification_plan", "meshclaw_autoheal_container_runbook"}, []string{"docker restart from chat", "container mutation without autoheal-plan evidence"}, true)
	case containsAny(normalized, "container", "docker", "컨테이너", "도커") && containsAny(normalized, "autoheal", "self-heal", "self heal", "restart", "recreate", "recover", "repair", "remediate", "자가치유", "복구", "재시작", "재생성", "고쳐"):
		return set("meshclaw_autoheal_plan", "Container self-heal starts from a read-only autoheal plan, then moves through the approval-gated container apply/verification/runbook chain before any executor is allowed.", map[string]interface{}{}, []string{"meshclaw_autoheal_container_apply_plan", "meshclaw_autoheal_container_verification_plan", "meshclaw_autoheal_container_runbook", "meshclaw_autoheal_container_readiness_summary"}, []string{"direct docker restart without evidence", "manual container recreate without approval-gate evidence"}, true)
	case containsAny(normalized, "service discovery", "service registry", "endpoint registry", "endpoint health", "load balancer", "load-balancer", "lb-lite", "lb lite", "proxy route", "route plan", "routing plan", "서비스 디스커버리", "서비스 레지스트리", "엔드포인트", "로드밸런서", "로드 밸런서", "프록시 라우팅", "라우팅 계획"):
		return set("meshclaw_service_registry_plan", "Service discovery and LB-lite work should start with endpoint inventory, health checks, route candidates, approval gates, and rollback guidance. It is plan-only and never changes proxies or DNS.", map[string]interface{}{"service": "<service>", "scope": "all", "port": 0, "endpoints": "<optional-host:port-list>"}, []string{"meshclaw_monitor_check", "meshclaw_agent_workloads", "meshclaw_reconcile_plan"}, []string{"editing proxy config directly", "changing DNS or firewall before endpoint evidence", "assuming endpoints are healthy from chat history"}, true)
	case containsAny(normalized, "autoscale", "auto scale", "auto-scale", "scale out", "scale up", "scale down", "capacity plan", "capacity planning", "capacity", "replica", "replicas", "placement capacity", "오토스케일", "자동 스케일", "자동스케일", "증설", "축소", "용량 계획", "용량", "복제본", "레플리카"):
		return set("meshclaw_capacity_scale_plan", "Capacity and autoscale-like work should start with utilization signals, placement candidates, cost/TTL constraints, approval gates, and rollback guidance. It is plan-only and never rents, deletes, migrates, or mutates servers.", map[string]interface{}{"workload": "<workload>", "scope": "all", "target": "<desired-state-or-slo>", "budget_usd": 0, "ttl_hours": 6, "constraints": "<optional-constraints>"}, []string{"meshclaw_capability_recommend", "meshclaw_placement_plan", "meshclaw_provision_plan", "meshclaw_service_registry_plan"}, []string{"provider create without approval", "moving workloads before placement evidence", "scaling from a single metric without logs and health evidence"}, true)
	case containsAny(normalized, "storage guardrail", "storage plan", "disk pressure", "backup plan", "backup guard", "volume", "mount", "mount dependency", "snapshot", "retention", "스토리지", "저장소", "디스크 압박", "백업", "볼륨", "마운트", "스냅샷", "보존 정책"):
		return set("meshclaw_storage_guardrail_plan", "Storage and backup-sensitive work should start with disk, mount, volume dependency, retention, backup, approval, and rollback evidence. It is plan-only and never deletes files, changes mounts, snapshots, or writes backups.", map[string]interface{}{"node": "<node-or-scope>", "path": "/", "workload": "<optional-workload>", "risk": "disk_pressure", "backup": true, "retention": "<optional-retention>", "mount_type": "<optional-mount-type>"}, []string{"meshclaw_disk_investigate", "meshclaw_data_clean_plan", "meshclaw_data_archive_plan", "meshclaw_reconcile_plan"}, []string{"rm before backup evidence", "changing mounts from chat", "moving volumes without rollback target", "declaring storage safe from df alone"}, true)
	case containsAny(normalized, "prometheus", "grafana", "loki", "opensearch", "elk", "portainer", "ansible", "uptime kuma", "uptime-kuma", "ntfy", "gotify", "tailscale", "wireguard", "cockpit", "existing ops tool", "ops integration", "tool integration", "운영툴", "기존 도구", "프로메테우스", "그라파나", "로키", "포테이너", "앤서블", "업타임 쿠마", "알림 연동", "도구 연동"):
		return set("meshclaw_ops_integration_plan", "Existing ops tools should be wrapped as MeshClaw MCP semantic evidence sources before any automation changes. Start read-only, map tool data to evidence contracts, then add approval-gated automation.", map[string]interface{}{"tools": "<comma-separated-tools>", "goal": intent, "scope": "fleet", "readonly": true}, []string{"meshclaw_monitor_check", "meshclaw_analyze_logs", "meshclaw_service_registry_plan", "meshclaw_capacity_scale_plan", "meshclaw_storage_guardrail_plan"}, []string{"letting the model use raw tool APIs without evidence mapping", "configuring automation before read-only validation", "sending secrets or raw tokens into chat"}, true)
	case containsAny(normalized, "mcp profile", "profile visibility", "tool visibility", "profile allowlist", "claude-lite", "local-lite", "프로필 노출", "프로필 확인", "allowlist 확인") || (containsAny(normalized, "도구 노출", "도구가 보이", "tool visible") && containsAny(normalized, "확인", "check", "visibility", "profile", "프로필", "allowlist")):
		return set("meshclaw_mcp_profile_visibility_check", "MCP profile visibility checks verify that expected tools are actually exposed by the selected MeshClaw MCP profile before asking Claude or another client to use them.", map[string]interface{}{"profile": "claude-lite", "expected_tools": "<comma-separated-tools>"}, []string{"meshclaw_mcp_surface", "meshclaw_mcp_catalog", "meshclaw_mcp_smoke_test_plan"}, []string{"assuming full-profile tools are visible in claude-lite", "restarting clients before checking profile allowlists", "running tools as a visibility check"}, false)
	case containsAny(normalized, "mcp rollout", "mcp refresh", "mcp client", "claude mcp", "openwebui mcp", "new tool", "tool visible", "new binary", "binary refresh", "새 도구", "새 기능", "클로드에서", "mcp에서 보", "바이너리 갱신", "빌드 반영", "어떻게 써", "써보"):
		return set("meshclaw_mcp_rollout_plan", "New MCP tools become visible to clients only after the reviewed code is built into the MeshClaw binary and the MCP client is pointed at or refreshed against that binary. This is a plan-only rollout guide.", map[string]interface{}{"client": "<mcp-client>", "branch": "<branch-or-pr>", "expected_tools": "<comma-separated-tools>"}, []string{"meshclaw_mcp_surface", "meshclaw_tool_recommend", "meshclaw_doctor"}, []string{"assuming PR code is already in the running MCP binary", "restarting or deploying live servers from chat", "testing mutating tools before plan-only surfaces"}, false)
	case containsAny(normalized, "mcp smoke", "smoke test", "smoke-test", "tool smoke", "surface smoke", "새 도구 확인", "도구 확인", "스모크 테스트", "스모크", "mcp 확인", "mcp 점검"):
		return set("meshclaw_mcp_smoke_test_plan", "MCP smoke testing should confirm surface visibility and run plan-only tools with sample prompts before any mutating apply path. This returns a read-only smoke sequence only.", map[string]interface{}{"client": "<mcp-client>", "scope": "k8s-replacement", "tools": "<optional-plan-only-tools>"}, []string{"meshclaw_mcp_surface", "meshclaw_tool_recommend", "meshclaw_mcp_rollout_plan"}, []string{"using smoke tests to mutate servers", "skipping meshclaw_mcp_surface visibility check", "testing against an unknown binary"}, false)
	case containsAny(normalized, "cleanup", "delete", "remove", "checkpoint", "중복", "삭제", "정리", "지워", "지우", "클린"):
		return set("meshclaw_data_clean_plan", "Cleanup should start with a plan that preserves clean/final outputs and marks risky deletions.", map[string]interface{}{"host": "<host>", "path": "<absolute-path>"}, []string{"meshclaw_disk_investigate", "meshclaw_data_clean_apply"}, []string{"direct rm", "manual deletion without manifest"}, true)
	case containsAny(normalized, "server management", "ops control", "전체 상태", "서버 관리", "서버관리", "상태 관리", "조치", "조치 후보", "복구 후보"):
		return set("meshclaw_ops_control", "Broad server management needs MeshClaw's combined health, service triage, policy, autoheal, and evidence summary.", map[string]interface{}{"apply_safe": false}, []string{"meshclaw_ops_brief", "meshclaw_monitor_check"}, []string{"raw vssh facts-many as first choice"}, false)
	case containsAny(normalized, "process", "resource", "workload", "what is using", "what is this server used for", "usage by process", "프로세스", "자원", "리소스", "무슨 용도", "어떤 용도", "어디에 쓰", "뭐에 쓰", "용도", "워크로드"):
		return set("meshclaw_agent_workloads", "Cached node-local agent reports already classify process purpose, resource usage, containers, public listeners, and next actions without forcing fresh SSH across the fleet.", map[string]interface{}{"max_age": "24h", "top": 3}, []string{"meshclaw_agent_changes", "meshclaw_ops_brief", "meshclaw_process_top"}, []string{"raw ps output as the first answer"}, false)
	case containsAny(normalized, "fleet security", "security posture", "public ports", "open ports", "firewall", "fail2ban", "cron", "timer", "docker ports", "보안 상태", "보안점검", "포트", "열린 포트", "방화벽", "크론", "도커 포트"):
		return set("meshclaw_agent_security", "Cached node-local security posture already summarizes public listeners, Docker exposure, firewall warnings, fail2ban, schedules, failed units, and next actions.", map[string]interface{}{"max_age": "24h"}, []string{"meshclaw_agent_workloads", "meshclaw_security_check", "meshclaw_fleet_scan"}, []string{"raw netstat output as the first answer", "fresh SSH across the fleet for every question"}, false)
	case containsAny(normalized, "service", "systemd", "failed", "restarting", "서비스", "장애", "실패", "죽는", "죽어", "죽음"):
		return set("meshclaw_service_triage", "Service incidents should be triaged first to separate real incidents from stale boot-only noise.", map[string]interface{}{"limit": 5, "max_parallel": 4}, []string{"meshclaw_service_check", "meshclaw_fleet_service_audit", "meshclaw_analyze_logs"}, []string{"direct systemctl restart before evidence"}, false)
	case containsAny(normalized, "log", "journal", "로그"):
		return set("meshclaw_analyze_logs", "Log investigation should store warning/error evidence with redaction and next actions. Use source=container:<name> for Docker container logs and inspect autoheal_handoff.runtime_evidence_checklist before container self-heal planning; for systemd logs, confirm unit identity and system/user scope before any restart plan.", logRecommendationArgs(intent), []string{"meshclaw_service_check", "meshclaw_security_check"}, []string{"raw SSH log scraping without evidence", "container apply/restart planning before focused container-logscan evidence", "service restart before unit identity and scope are confirmed"}, false)
	case containsAny(normalized, "security", "보안", "secret", "pii", "개인정보", "민감정보", "시크릿"):
		return set("meshclaw_agent_security", "Start with cached fleet security posture. Use host-level security/hygiene tools only when the user asks for a specific node or sensitive-data scan.", map[string]interface{}{"max_age": "24h"}, []string{"meshclaw_security_check", "meshclaw_hygiene_scan_host", "meshclaw_fleet_scan"}, []string{"cat secret files", "copy raw credentials into chat"}, false)
	case containsAny(normalized, "fleet", "health", "status", "monitor", "서버 상태", "상태", "모니터", "디스크", "메모리", "gpu", "cpu"):
		return set("meshclaw_monitor_check", "Fleet status should use MeshClaw monitoring/facts aggregation rather than raw transport output.", map[string]interface{}{}, []string{"meshclaw_ops_brief", "meshclaw_ops_control"}, []string{"raw vssh facts-many as first choice"}, false)
	case containsAny(normalized, "disk", "du", "df", "storage", "용량", "디스크", "저장공간"):
		return set("meshclaw_disk_investigate", "Disk investigation needs read-only evidence before any cleanup plan.", map[string]interface{}{"host": "<host>", "path": "/"}, []string{"meshclaw_data_clean_plan", "meshclaw_process_top"}, []string{"rm before data_clean_plan"}, false)
	case containsAny(normalized, "run command", "diagnostic command", "명령", "실행", "uptime", "df -h"):
		return set("meshclaw_run_evidence", "Single diagnostic commands should go through MeshClaw for policy, fallback, and evidence.", map[string]interface{}{"host": "<host>", "command": "<command>"}, []string{"direct vssh for transport debugging only"}, []string{"raw SSH when audit trail is needed"}, false)
	case containsAny(normalized, "vssh auth", "vsshd", "vssh daemon", "auth failed", "인증", "데몬"):
		return set("meshclaw_vssh_daemon_audit", "vssh execution health should be diagnosed through MeshClaw's daemon/auth/version classifiers.", map[string]interface{}{"hosts": ""}, []string{"meshclaw_vssh_auth_paths", "meshclaw_node_repair_plan"}, []string{"rotating secrets before audit"}, false)
	case containsAny(normalized, "vssh direct", "direct vssh", "ssh 말고", "그냥 ssh", "vssh를 직접", "vssh 직접", "직접 써", "exec many", "facts many", "rpc", "parallel exec", "transport", "병렬 실행", "전송"):
		return set("direct vssh MCP", "Direct vssh is appropriate for low-level transport primitives, parallel exec, typed facts, daemon RPC, or debugging MeshClaw adapters.", map[string]interface{}{"note": "Use vssh tools directly only when policy/evidence/workflow semantics are not required."}, []string{"meshclaw_run_evidence", "meshclaw_vssh_auth_paths"}, []string{"direct vssh for mutating ops without evidence"}, false)
	case containsAny(normalized, "rent", "vps", "provision", "server capacity", "서버 임대", "임대", "프로비전"):
		return set("meshclaw_provision_plan", "Provider/cost-changing work must start as a plan; real create/delete needs approval.", map[string]interface{}{"purpose": intent, "budget_usd": 0, "ttl_hours": 6}, []string{"meshclaw_placement_plan"}, []string{"provider create without policy approval"}, true)
	default:
		return set("meshclaw_ops_control", "Default to MeshClaw's server-management control report when the intent is ambiguous.", map[string]interface{}{"apply_safe": false}, []string{"meshclaw_ai_guide", "meshclaw_ops_dispatch"}, []string{"guessing raw vssh/SSH commands"}, false)
	}
}

func recommendationRequiredEvidence(tool string, approval bool) []string {
	required := []string{
		"stored evidence path from the recommended tool response",
		"redacted findings or structured report sections before reasoning",
	}
	if !approval {
		switch tool {
		case "meshclaw_analyze_logs":
			required = append(required,
				"for container logs, use source=container:<name> and retain focused container-logscan evidence before apply planning",
				"for container logs, inspect autoheal_handoff.runtime_evidence_checklist and satisfy docker inspect status/health items before apply planning",
				"for container self-heal planning, retain fresh runtime inspect/status evidence with image, status, health, ports, and restart policy",
				"for systemd logs, retain unit_candidates and follow with targeted meshclaw_service_check evidence before restart planning",
			)
		case "meshclaw_autoheal_container_readiness_summary":
			required = append(required, "container completion-plan evidence with stop_before gates and final container-logscan requirements")
		case "meshclaw_reconcile_readiness_summary":
			required = append(required, "reconcile completion-plan evidence with stop_before gates and approval/apply-gate/verification evidence")
		}
		return append(required, "fresh read-only state before suggesting any later apply path")
	}
	required = append(required,
		"explicit operator approval evidence before any mutating executor",
		"rollback or verification evidence before declaring readiness",
	)
	switch tool {
	case "meshclaw_autoheal_container_apply_plan":
		required = append(required, "autoheal-plan evidence with container action candidate and approved_by")
		required = append(required, "analyze_logs handoff_contract.apply_allowed=false when the action is based on container logscan evidence")
		required = append(required, "container_apply_plan_contract.direct_restart_allowed=false and requires_focused_runtime_evidence=true before verification planning")
		required = append(required, "container_apply_plan_contract.runtime_evidence_required_count must match planned container steps")
	case "meshclaw_autoheal_container_verification_plan":
		required = append(required, "container apply-plan evidence and focused container-logscan evidence")
		required = append(required, "container_apply_plan_contract.runtime_evidence_required_count preserved as apply_plan_runtime_evidence_required_count")
	case "meshclaw_autoheal_container_runbook":
		required = append(required, "container verification-plan evidence with apply_plan_runtime_evidence_required_count")
	case "meshclaw_autoheal_container_executor_gate":
		required = append(required, "ready container-readiness-summary evidence and approved_by")
		required = append(required, "dry_run=true gate admission before executor preview")
	case "meshclaw_autoheal_container_executor":
		required = append(required, "ready container-executor-gate evidence and matching approved_by")
		required = append(required, "dry-run executor preview before live execution")
		required = append(required, "exact live approval phrase before execute=true can mutate containers")
	case "meshclaw_autoheal_container_executor_verify":
		required = append(required, "container-executor evidence from a live completed executor run")
		required = append(required, "post-action agent-collect evidence for every executed container step")
		required = append(required, "focused container-logscan evidence for every executed container step")
	case "meshclaw_reconcile_apply_gate":
		required = append(required, "reconcile approval-request evidence and approved_by")
	case "meshclaw_reconcile_apply_plan":
		required = append(required, "ready reconcile apply-gate evidence")
	case "meshclaw_automation_rule_check":
		required = append(required, "automation rule-plan evidence, cooldown, rollback, and approved_by")
	case "meshclaw_automation_rule_writer_plan":
		required = append(required, "ready automation rule-check evidence and separate writer approval")
	}
	return required
}

func recommendationStopBefore(tool string, approval bool) []string {
	stopBefore := []string{
		"raw shell execution outside MeshClaw evidence",
		"copying unredacted secrets into chat",
	}
	if !approval {
		switch tool {
		case "meshclaw_analyze_logs":
			stopBefore = append(stopBefore,
				"container apply or restart planning before focused container-logscan evidence is reviewed",
				"container apply planning before fresh runtime inspect/status evidence is reviewed",
				"systemd restart planning before unit_candidates and targeted service-check evidence are reviewed",
			)
		case "meshclaw_autoheal_container_readiness_summary":
			stopBefore = append(stopBefore,
				"treating readiness summary as operator approval",
				"starting container executor without ready completion-plan evidence",
			)
		case "meshclaw_reconcile_readiness_summary":
			stopBefore = append(stopBefore,
				"treating readiness summary as operator approval",
				"starting reconcile executor without ready completion-plan evidence",
			)
		}
		return append(stopBefore, "treating read-only evidence as approval")
	}
	stopBefore = append(stopBefore,
		"apply=true or execute=true without operator approval evidence",
		"service/container mutation before rollback and verification evidence",
	)
	switch tool {
	case "meshclaw_autoheal_container_apply_plan", "meshclaw_autoheal_container_verification_plan", "meshclaw_autoheal_container_runbook":
		stopBefore = append(stopBefore, "docker restart/recreate from recommendation text")
	case "meshclaw_autoheal_container_executor_gate":
		stopBefore = append(stopBefore, "treating executor-gate readiness as live execution approval")
	case "meshclaw_autoheal_container_executor":
		stopBefore = append(stopBefore, "execute=true before dry-run preview and exact live approval phrase")
	case "meshclaw_autoheal_container_executor_verify":
		stopBefore = append(stopBefore, "closing self-heal before post-action agent and container-logscan evidence")
	case "meshclaw_reconcile_apply_gate", "meshclaw_reconcile_apply_plan", "meshclaw_reconcile_execution_preview":
		stopBefore = append(stopBefore, "kubectl-style live apply from desired-state text")
	case "meshclaw_automation_rule_check", "meshclaw_automation_rule_writer_plan", "meshclaw_automation_rule_plan":
		stopBefore = append(stopBefore, "enabling unattended auto-apply from chat")
	}
	return stopBefore
}

func mcpSurfaceGuide() map[string]interface{} {
	return map[string]interface{}{
		"kind": "meshclaw_mcp_surface",
		"default_path": []map[string]interface{}{
			{"step": 1, "tool": "meshclaw_ai_guide", "when": "first contact or when the model is unsure which layer to use"},
			{"step": 2, "tool": "meshclaw_quickstart", "when": "first 5 minutes path: setup check, workflow inspection, dry-run evidence"},
			{"step": 3, "tool": "meshclaw_doctor", "when": "verify local MeshClaw/VSSH/MCP setup before blaming the fleet"},
			{"step": 4, "tool": "meshclaw_setup_signal", "when": "debug Signal/Argos Automation Mode without shelling out"},
			{"step": 5, "tool": "meshclaw_daemon_signal_status", "when": "check whether the Signal dispatcher daemon is installed/running"},
			{"step": 6, "tool": "meshclaw_setup_argos_runner", "when": "check or calibrate the bounded macOS UI runner"},
			{"step": 7, "tool": "meshclaw_server_list", "when": "read current inventory truth before choosing nodes"},
			{"step": 8, "tool": "meshclaw_inventory_override_list", "when": "check operator-owned role/tag refinements"},
			{"step": 9, "tool": "meshclaw_inventory_override_set", "when": "the user clarifies private fleet meaning, such as c1 is mail-server"},
			{"step": 10, "tool": "meshclaw_capability_validate", "when": "before placement, model/API selection, or execute mode"},
			{"step": 11, "tool": "meshclaw_capability_recommend", "when": "choose a node/model/API/storage/provisioner capability for the intent"},
			{"step": 12, "tool": "meshclaw_schedule_status", "when": "check local-model quick checks, briefings, and daily expert audit schedule state"},
			{"step": 13, "tool": "meshclaw_workflow_list", "when": "discover repeatable workflows"},
			{"step": 14, "tool": "meshclaw_workflow_validate", "when": "before running user-authored or example workflow files"},
			{"step": 15, "tool": "meshclaw_workflow_run", "when": "run dry-run first; execute only after approval gates are resolved"},
			{"step": 16, "tool": "meshclaw_evidence_latest", "when": "read evidence bundle paths and AI handoff after a run"},
			{"step": 17, "tool": "meshclaw_workflow_plan_execute", "when": "check if the latest evidence bundle is ready for execute mode"},
			{"step": 18, "tool": "meshclaw_workflow_resume", "when": "continue from failed, approval-pending, retryable, or targeted steps"},
			{"step": 19, "tool": "meshclaw_messenger_report", "when": "post a redacted owner/team report to Signal or another messenger after evidence review"},
		},
		"canonical_loop": []string{
			"inventory",
			"operator overrides",
			"capability validation",
			"capability recommendation",
			"desired-state validation",
			"reconcile plan",
			"reconcile readiness summary",
			"container self-heal plan",
			"container readiness summary",
			"service registry plan",
			"capacity scale plan",
			"storage guardrail plan",
			"ops integration plan",
			"mcp rollout plan",
			"mcp smoke test plan",
			"mcp profile visibility check",
			"automation rule plan",
			"automation rule check",
			"automation rule readiness summary",
			"automation rule writer plan",
			"workflow dry-run",
			"evidence review",
			"readiness evidence review before executor",
			"approval or repair",
			"targeted execute/resume",
		},
		"readiness_gate_sequence": []map[string]interface{}{
			{
				"id":                "desired_state_reconcile",
				"completion_tool":   "meshclaw_reconcile_completion_plan",
				"readiness_tool":    "meshclaw_reconcile_readiness_summary",
				"required_evidence": []string{"reconcile completion-plan evidence", "approval/apply-gate/verification evidence", "rollback evidence", "stop_before gates"},
				"stop_before":       []string{"treating readiness summary as approval", "starting reconcile executor without ready completion-plan evidence"},
				"next_actor":        "operator",
				"mutates_servers":   false,
			},
			{
				"id":                "container_self_heal",
				"completion_tool":   "meshclaw_autoheal_container_completion_plan",
				"readiness_tool":    "meshclaw_autoheal_container_readiness_summary",
				"required_evidence": []string{"container completion-plan evidence", "apply-loop gate evidence", "final container-logscan evidence", "rollback evidence", "stop_before gates"},
				"stop_before":       []string{"treating readiness summary as approval", "starting container executor without ready completion-plan evidence"},
				"next_actor":        "operator",
				"mutates_servers":   false,
			},
		},
		"runtime_layers": []map[string]interface{}{
			{
				"id":   "model_reasoning",
				"role": "Interpret intent, choose semantic tools, explain evidence, and ask for approval; do not assume direct shell ownership.",
			},
			{
				"id":   "meshclaw_core_runtime",
				"role": "Own fleet state, topology, policy, workflows, schedules, approvals, history, and evidence as the local-first control plane.",
			},
			{
				"id":   "meshclaw_worker",
				"role": "Expose lightweight node capability, inventory, metrics, heartbeat, log streaming, and bounded command execution.",
			},
			{
				"id":   "execution_transport",
				"role": "Use meshpop/vssh/SSH/Wire routing as transport for approved tool paths; transport is not the reasoning layer.",
			},
		},
		"semantic_ops_principles": []string{
			"Prefer semantic tools over ad hoc shell commands.",
			"Ground reasoning in evidence, state, stdout/stderr, logs, and evidence bundle paths.",
			"Keep execution local-first; use cloud or large-model reasoning only when needed.",
			"Treat mutating actions as approval-gated and rollback-aware.",
			"Install full runtime on core nodes and lightweight workers on fleet nodes.",
		},
		"evidence_contracts": mcpEvidenceContracts(),
		"default_tools": []string{
			"mission_get",
			"mission_list",
			"meshclaw_ai_guide",
			"meshclaw_architecture",
			"meshclaw_quickstart",
			"meshclaw_doctor",
			"meshclaw_setup_assistant",
			"meshclaw_model_config_status",
			"meshclaw_setup_signal",
			"meshclaw_setup_argos_runner",
			"meshclaw_daemon_signal_status",
			"meshclaw_daemon_schedule_status",
			"meshclaw_schedule_plan",
			"meshclaw_schedule_status",
			"meshclaw_schedule_run_once",
			"meshclaw_mcp_surface",
			"meshclaw_mcp_catalog",
			"meshclaw_tool_recommend",
			"meshclaw_opsdb_status",
			"meshclaw_opsdb_events",
			"meshclaw_opsdb_power_events",
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
			"meshclaw_reconcile_run_once",
			"meshclaw_autoheal_plan",
			"meshclaw_autoheal_container_apply_plan",
			"meshclaw_autoheal_container_verification_plan",
			"meshclaw_autoheal_container_runbook",
			"meshclaw_autoheal_container_runbook_check",
			"meshclaw_autoheal_container_rollback_plan",
			"meshclaw_autoheal_container_completion_plan",
			"meshclaw_autoheal_container_readiness_summary",
			"meshclaw_autoheal_apply_safe",
			"meshclaw_workflow_list",
			"meshclaw_workflow_scaffold",
			"meshclaw_workflow_validate",
			"meshclaw_workflow_inspect",
			"meshclaw_workflow_run",
			"meshclaw_workflow_plan_execute",
			"meshclaw_workflow_resume",
			"meshclaw_evidence_latest",
			"meshclaw_messenger_report",
			"meshclaw_messenger_approval_request",
			"meshclaw_messenger_targets",
			"meshclaw_scheduled_delivery_plan",
			"meshclaw_scheduled_delivery_apply",
			"meshclaw_signal_rooms_doctor",
			"meshclaw_messenger_send_report",
			"meshclaw_messenger_send_approval_request",
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
			"meshclaw_news_document",
			"meshclaw_argos_research",
			"meshclaw_automation_open_file",
			"meshclaw_media_play",
			"meshclaw_audio_transcribe",
			"meshclaw_app_settings_plan",
			"meshclaw_notification_show",
			"meshclaw_terminal_run",
			"meshclaw_shortcut_text_run",
			"meshclaw_result_save",
			"meshclaw_maps_search",
			"meshclaw_maps_directions",
			"meshclaw_maps_proof",
			"meshclaw_account_action_plan",
			"meshclaw_purchase_click",
			"meshclaw_calendar_list",
			"meshclaw_calendar_create_event",
			"meshclaw_reminders_list",
			"meshclaw_reminder_create",
			"meshclaw_contacts_search",
			"meshclaw_notes_search",
			"meshclaw_note_create",
			"meshclaw_server_list",
			"meshclaw_inventory_override_list",
			"meshclaw_capability_list",
			"meshclaw_capability_validate",
			"meshclaw_capability_recommend",
			"meshclaw_monitor_check",
			"meshclaw_service_registry_plan",
			"meshclaw_capacity_scale_plan",
			"meshclaw_storage_guardrail_plan",
			"meshclaw_ops_integration_plan",
			"meshclaw_mcp_rollout_plan",
			"meshclaw_mcp_smoke_test_plan",
			"meshclaw_mcp_profile_visibility_check",
			"meshclaw_automation_rule_plan",
			"meshclaw_automation_rule_check",
			"meshclaw_automation_rule_readiness_summary",
			"meshclaw_automation_rule_writer_plan",
			"meshclaw_agent_workloads",
			"meshclaw_agent_changes",
			"meshclaw_agent_security",
			"meshclaw_agent_inventory_plan",
			"meshclaw_data_doctor",
			"meshclaw_data_archive_plan",
			"meshclaw_ops_control",
			"meshclaw_policy_check",
			"meshclaw_guard_detect",
			"meshclaw_guard_redact",
			"meshclaw_guard_vault_list",
			"meshclaw_guard_vault_metadata",
		},
		"advanced_tools": []string{
			"meshclaw_fleet_scan",
			"meshclaw_fleet_service_audit",
			"meshclaw_service_triage",
			"meshclaw_service_check",
			"meshclaw_vssh_daemon_audit",
			"meshclaw_vssh_auth_paths",
			"meshclaw_node_repair_plan",
			"meshclaw_inventory_override_set",
			"meshclaw_inventory_override_remove",
			"meshclaw_run_evidence",
			"meshclaw_job_start",
			"meshclaw_job_status",
			"meshclaw_job_logs",
			"meshclaw_artifact_collect",
			"meshclaw_disk_investigate",
			"meshclaw_data_clean_plan",
			"meshclaw_provision_plan",
			"meshclaw_hygiene_scan_host",
			"meshclaw_security_check",
			"meshclaw_opsdb_record",
			"meshclaw_signal_rooms_cleanup",
		},
		"use_direct_vssh_when": []string{
			"debugging low-level transport, daemon RPC, or routing",
			"running typed facts/exec primitives where MeshClaw policy, workflow, and evidence are not required",
			"building or testing the vssh substrate itself",
		},
		"avoid": []string{
			"using raw SSH/vssh as the top-level orchestrator when policy or evidence is needed",
			"asking the model to parse long terminal prose when MeshClaw returns typed JSON",
			"pasting raw secrets into Codex, Claude, Cursor, or workflow evidence",
		},
	}
}

func mcpEvidenceContracts() []string {
	return []string{
		"container logscan can return autoheal_handoff.runtime_evidence_checklist before self-heal planning",
		"systemd logscan can return unit_candidates before targeted service restart planning",
		"readiness summaries are checkpoints, not operator approval",
	}
}

func analyzeLogsMCPHandoffContract(handoff *workflow.AutohealHandoff) map[string]interface{} {
	if handoff == nil {
		return nil
	}
	requires := []string{
		"stored analyze-logs evidence path from this MCP call",
		"operator approval evidence before any apply or restart",
	}
	checklistReviewRequired := len(handoff.RuntimeEvidenceChecklist) > 0
	if checklistReviewRequired {
		requires = append(requires, "autoheal_handoff.runtime_evidence_checklist reviewed before apply planning")
	}
	nextRequiredTool := ""
	if len(handoff.RecommendedTools) > 0 {
		nextRequiredTool = handoff.RecommendedTools[0]
	}
	return map[string]interface{}{
		"kind":                              "analyze_logs_handoff_contract",
		"decision":                          handoff.Decision,
		"apply_allowed":                     false,
		"mutates_live_servers":              false,
		"direct_restart_allowed":            false,
		"checklist_review_required":         checklistReviewRequired,
		"requires_focused_runtime_evidence": checklistReviewRequired,
		"evidence_required_count":           len(handoff.EvidenceRequired),
		"runtime_evidence_checklist_count":  len(handoff.RuntimeEvidenceChecklist),
		"stop_before_count":                 len(handoff.StopBefore),
		"must_not_count":                    len(handoff.MustNot),
		"next_required_tool":                nextRequiredTool,
		"requires":                          requires,
		"recommended_tools":                 handoff.RecommendedTools,
		"stop_before":                       handoff.StopBefore,
		"must_not":                          handoff.MustNot,
	}
}

func readinessMCPContract(kind string, stopBefore []string) map[string]interface{} {
	return map[string]interface{}{
		"kind":                                    kind,
		"summary_tools_are_approval":              false,
		"apply_allowed":                           false,
		"mutates_live_servers":                    false,
		"grants_future_approval":                  false,
		"requires_operator_approval_for_executor": true,
		"stop_before":                             stopBefore,
	}
}

func desiredValidationMCPContract(report reconcileDesiredValidationReport) map[string]interface{} {
	allowsDryRun := false
	for _, tool := range report.Handoff.AllowedNextTools {
		if tool == "meshclaw_reconcile_plan" {
			allowsDryRun = true
			break
		}
	}
	return map[string]interface{}{
		"kind":                                  "desired_validation_contract",
		"validation_only":                       true,
		"ready":                                 report.Ready,
		"apply_allowed":                         false,
		"execute_implemented":                   false,
		"mutates_live_servers":                  false,
		"grants_future_approval":                false,
		"yaml_keys_grant_approval":              false,
		"ignored_apply_keys":                    report.Counts["ignored_apply_keys"],
		"critical_findings":                     report.Counts["critical"],
		"warning_findings":                      report.Counts["warning"],
		"allows_dry_run_reconcile":              allowsDryRun,
		"requires_stored_validation_evidence":   true,
		"requires_revalidation_on_yaml_change":  true,
		"blocks_apply_gate_without_approval":    true,
		"requires_approval_request_before_gate": true,
		"stop_before":                           report.Handoff.StopBefore,
		"refresh_triggers":                      report.Handoff.RefreshTriggers,
	}
}

func reconcilePlanMCPContract(report reconcilePlanReport) map[string]interface{} {
	return map[string]interface{}{
		"kind":                                  "reconcile_plan_contract",
		"dry_run_only":                          true,
		"ready":                                 true,
		"apply_allowed":                         false,
		"execute_implemented":                   false,
		"mutates_live_servers":                  false,
		"grants_future_approval":                false,
		"actions":                               report.Counts["actions"],
		"approval_required_actions":             report.Counts["approval_required"],
		"policy_require_approval_actions":       report.Counts["policy_require_approval"],
		"container_actions":                     report.Counts["container_actions"],
		"requires_validation_before_apply":      true,
		"requires_approval_request_before_gate": true,
		"requires_apply_gate_before_executor":   true,
		"requires_fresh_actual_evidence":        true,
		"stop_before": []string{
			"treating reconcile plan as live apply",
			"running planned actions without approval-request and apply-gate evidence",
			"reusing this plan after desired-state or actual node evidence changes",
		},
	}
}

func reconcileApprovalRequestMCPContract(request reconcileApprovalRequest) map[string]interface{} {
	return map[string]interface{}{
		"kind":                                  "reconcile_approval_request_contract",
		"request_only":                          true,
		"approval_required":                     request.ApprovalRequired,
		"apply_allowed":                         false,
		"execute_implemented":                   false,
		"mutates_live_servers":                  false,
		"grants_future_approval":                false,
		"operator_approval_recorded":            false,
		"actions_requiring_approval":            len(request.Actions),
		"blocked_actions":                       len(request.BlockedActions),
		"container_actions_requiring_approval":  request.Counts["container_approval_required"],
		"container_blocked_actions":             request.Counts["container_blocked"],
		"requires_apply_gate_after_approval":    true,
		"requires_operator_approved_by_at_gate": true,
		"blocked_actions_must_not_execute":      true,
		"stop_before": []string{
			"treating approval-request evidence as operator approval",
			"running approval-required actions before apply-gate evidence",
			"running blocked actions from approval-request evidence",
		},
	}
}

func reconcileApplyGateMCPContract(report reconcileApplyGateReport) map[string]interface{} {
	return map[string]interface{}{
		"kind":                                "reconcile_apply_gate_contract",
		"gate_only":                           true,
		"ready":                               report.Ready,
		"apply_allowed":                       false,
		"mutates_live_servers":                false,
		"grants_future_approval":              false,
		"approved_by_present":                 strings.TrimSpace(report.ApprovedBy) != "",
		"requires_apply_plan_before_executor": true,
		"requires_operator_approval":          true,
		"stop_before": []string{
			"treating apply gate evidence as execution approval",
			"starting reconcile executor without apply-plan and verification evidence",
			"reusing this gate after desired-state or approval evidence changes",
		},
	}
}

func reconcileApplyPlanMCPContract(report reconcileApplyPlanReport) map[string]interface{} {
	return map[string]interface{}{
		"kind":                   "reconcile_apply_plan_contract",
		"plan_only":              true,
		"ready":                  report.Ready,
		"apply_allowed":          false,
		"execute_implemented":    false,
		"mutates_live_servers":   false,
		"grants_future_approval": false,
		"requires_execution_preview_before_executor": true,
		"requires_verification_after_executor":       true,
		"stop_before": []string{
			"executing apply steps from chat",
			"starting reconcile executor without execution-preview evidence",
			"treating apply-plan evidence as live mutation approval",
		},
	}
}

func reconcileExecutionPreviewMCPContract(report reconcileExecutionPreviewReport) map[string]interface{} {
	return map[string]interface{}{
		"kind":                                 "reconcile_execution_preview_contract",
		"preview_only":                         true,
		"ready":                                report.Ready,
		"apply_allowed":                        false,
		"execute_implemented":                  false,
		"mutates_live_servers":                 false,
		"grants_future_approval":               false,
		"commands_are_inert_templates":         true,
		"requires_verification_after_executor": true,
		"stop_before": []string{
			"running preview command templates",
			"copying inert templates into live shell",
			"starting reconcile executor without post-action verification evidence requirements",
		},
	}
}

func reconcileVerificationPlanMCPContract(report reconcileVerificationPlanReport) map[string]interface{} {
	return map[string]interface{}{
		"kind":                                 "reconcile_verification_plan_contract",
		"plan_only":                            true,
		"ready":                                report.Ready,
		"apply_allowed":                        false,
		"execute_implemented":                  false,
		"mutates_live_servers":                 false,
		"grants_future_approval":               false,
		"requires_post_action_evidence":        true,
		"requires_completion_plan_before_done": true,
		"stop_before": []string{
			"treating verification plan as completed verification evidence",
			"declaring reconcile complete before post-action verification evidence exists",
			"starting reconcile executor from verification-plan text",
		},
	}
}

func reconcileRunbookMCPContract(report reconcileRunbookReport) map[string]interface{} {
	return map[string]interface{}{
		"kind":                          "reconcile_runbook_contract",
		"review_only":                   true,
		"ready":                         report.Ready,
		"apply_allowed":                 false,
		"execute_implemented":           false,
		"mutates_live_servers":          false,
		"grants_future_approval":        false,
		"requires_runbook_check":        true,
		"requires_post_action_evidence": true,
		"stop_before": []string{
			"treating runbook text as automatic execution",
			"running server mutations without runbook-check evidence",
			"declaring reconcile complete from a review-only runbook",
		},
	}
}

func reconcileRunbookCheckMCPContract(report reconcileRunbookCheckReport) map[string]interface{} {
	return map[string]interface{}{
		"kind":                            "reconcile_runbook_check_contract",
		"gate_only":                       true,
		"ready":                           report.Ready,
		"apply_allowed":                   false,
		"execute_implemented":             false,
		"mutates_live_servers":            false,
		"grants_future_approval":          false,
		"requires_zero_critical_findings": true,
		"critical_findings":               report.Counts["critical"],
		"requires_rollback_plan":          true,
		"requires_completion_plan":        true,
		"stop_before": []string{
			"treating runbook-check readiness as execution approval",
			"starting reconcile executor without rollback-plan evidence",
			"ignoring critical runbook-check findings",
		},
	}
}

func reconcileRollbackPlanMCPContract(report reconcileRollbackPlanReport) map[string]interface{} {
	return map[string]interface{}{
		"kind":                       "reconcile_rollback_plan_contract",
		"plan_only":                  true,
		"ready":                      report.Ready,
		"apply_allowed":              false,
		"rollback_allowed":           false,
		"execute_implemented":        false,
		"mutates_live_servers":       false,
		"grants_future_approval":     false,
		"requires_completion_plan":   true,
		"requires_operator_approval": true,
		"stop_before": []string{
			"treating rollback plan as automatic rollback execution",
			"running rollback actions without explicit approval-gated executor",
			"mutating servers before completion-plan evidence exists",
		},
	}
}

func reconcileCompletionPlanMCPContract(report reconcileCompletionPlanReport) map[string]interface{} {
	return map[string]interface{}{
		"kind":                              "reconcile_completion_plan_contract",
		"plan_only":                         true,
		"ready":                             report.Ready,
		"apply_allowed":                     false,
		"complete_allowed":                  false,
		"execute_implemented":               false,
		"mutates_live_servers":              false,
		"grants_future_approval":            false,
		"requires_final_evidence":           true,
		"requires_readiness_summary":        true,
		"completion_requirements":           len(report.Requirements),
		"requires_operator_visible_summary": true,
		"stop_before": []string{
			"declaring reconcile complete from completion-plan evidence",
			"skipping final post-action evidence collection",
			"mutating servers during completion planning",
		},
	}
}

func containerApplyPlanMCPContract(report containerApplyPlanReport) map[string]interface{} {
	return map[string]interface{}{
		"kind":                              "container_apply_plan_contract",
		"plan_only":                         true,
		"ready":                             report.Ready,
		"apply_allowed":                     false,
		"execute_implemented":               false,
		"mutates_live_servers":              false,
		"grants_future_approval":            false,
		"direct_restart_allowed":            report.DirectRestartAllowed,
		"requires_focused_runtime_evidence": report.RequiresFocusedRuntimeEvidence,
		"runtime_evidence_required_count":   report.RuntimeEvidenceRequiredCount,
		"requires_verification_plan":        true,
		"next_required_tool":                report.NextRequiredTool,
		"stop_before":                       report.StopBefore,
	}
}

func containerVerificationPlanMCPContract(report containerVerificationPlanReport) map[string]interface{} {
	return map[string]interface{}{
		"kind":                          "container_verification_plan_contract",
		"plan_only":                     true,
		"ready":                         report.Ready,
		"apply_allowed":                 false,
		"execute_implemented":           false,
		"mutates_live_servers":          false,
		"grants_future_approval":        false,
		"requires_runtime_evidence":     true,
		"requires_container_logscan":    true,
		"requires_post_action_evidence": true,
		"apply_plan_runtime_evidence_required_count": report.ApplyPlanRuntimeEvidenceRequiredCount,
		"stop_before": report.StopBefore,
	}
}

func containerRunbookMCPContract(report containerRunbookReport) map[string]interface{} {
	return map[string]interface{}{
		"kind":                                       "container_runbook_contract",
		"review_only":                                true,
		"ready":                                      report.Ready,
		"apply_allowed":                              false,
		"execute_implemented":                        false,
		"mutates_live_servers":                       false,
		"grants_future_approval":                     false,
		"requires_runbook_check":                     true,
		"requires_runtime_evidence":                  true,
		"requires_container_logscan":                 true,
		"requires_post_action_evidence":              true,
		"apply_plan_runtime_evidence_required_count": report.ApplyPlanRuntimeEvidenceRequiredCount,
		"stop_before": []string{
			"treating container runbook text as docker execution approval",
			"running docker commands without runbook-check evidence",
			"declaring container repair complete from a review-only runbook",
		},
	}
}

func containerRunbookCheckMCPContract(report containerRunbookCheckReport) map[string]interface{} {
	return map[string]interface{}{
		"kind":                            "container_runbook_check_contract",
		"gate_only":                       true,
		"ready":                           report.Ready,
		"apply_allowed":                   false,
		"execute_implemented":             false,
		"mutates_live_servers":            false,
		"grants_future_approval":          false,
		"requires_zero_critical_findings": true,
		"critical_findings":               report.Counts["critical"],
		"requires_runtime_evidence":       true,
		"requires_container_logscan":      true,
		"requires_rollback_plan":          true,
		"requires_completion_plan":        true,
		"stop_before": []string{
			"treating container runbook-check readiness as docker execution approval",
			"running docker commands without rollback-plan evidence",
			"ignoring critical container runbook-check findings",
		},
	}
}

func containerRollbackPlanMCPContract(report containerRollbackPlanReport) map[string]interface{} {
	return map[string]interface{}{
		"kind":                       "container_rollback_plan_contract",
		"plan_only":                  true,
		"ready":                      report.Ready,
		"apply_allowed":              false,
		"rollback_allowed":           false,
		"execute_implemented":        false,
		"mutates_live_servers":       false,
		"grants_future_approval":     false,
		"requires_operator_approval": true,
		"requires_runtime_evidence":  true,
		"requires_container_logscan": true,
		"requires_completion_plan":   true,
		"stop_before": []string{
			"treating container rollback plan as automatic docker rollback execution",
			"running rollback actions without explicit operator approval",
			"mutating containers before completion-plan evidence exists",
		},
	}
}

func containerCompletionPlanMCPContract(report containerCompletionPlanReport) map[string]interface{} {
	return map[string]interface{}{
		"kind":                       "container_completion_plan_contract",
		"plan_only":                  true,
		"ready":                      report.Ready,
		"apply_allowed":              false,
		"complete_allowed":           false,
		"execute_implemented":        false,
		"mutates_live_servers":       false,
		"grants_future_approval":     false,
		"requires_final_evidence":    true,
		"requires_runtime_evidence":  true,
		"requires_container_logscan": true,
		"requires_readiness_summary": true,
		"completion_requirements":    len(report.Requirements),
		"stop_before": []string{
			"declaring container repair complete from completion-plan evidence",
			"skipping final container-logscan evidence collection",
			"mutating containers during completion planning",
		},
	}
}

func containerReadinessSummaryMCPContract(report containerReadinessSummaryReport) map[string]interface{} {
	return map[string]interface{}{
		"kind":                       "container_readiness_summary_contract",
		"summary_only":               true,
		"ready":                      report.Ready,
		"apply_allowed":              false,
		"execute_implemented":        false,
		"mutates_live_servers":       false,
		"grants_future_approval":     false,
		"summary_tools_are_approval": false,
		"requires_operator_approval_for_executor": true,
		"requires_completion_plan":                true,
		"requires_final_evidence":                 true,
		"requires_runtime_evidence":               true,
		"requires_container_logscan":              true,
		"requires_approval_gated_executor":        true,
		"readiness_stages":                        len(report.ReadyStages),
		"readiness_blockers":                      len(report.Blockers),
		"stop_before":                             report.StopBefore,
	}
}

func containerExecutorGateMCPContract(report containerExecutorGateReport) map[string]interface{} {
	return map[string]interface{}{
		"kind":                       "container_executor_gate_contract",
		"admission_only":             true,
		"ready":                      report.Ready,
		"apply_allowed":              false,
		"execute_implemented":        false,
		"mutates_live_servers":       false,
		"grants_future_approval":     false,
		"requires_operator_approval": true,
		"requires_dry_run":           true,
		"requires_readiness_summary": true,
		"requires_final_logscan":     true,
		"live_execution_allowed":     report.LiveExecutionAllowed,
		"checks":                     len(report.Checks),
		"blockers":                   len(report.Blockers),
		"stop_before":                report.StopBefore,
	}
}

func containerExecutorMCPContract(report containerExecutorReport) map[string]interface{} {
	return map[string]interface{}{
		"kind":                          "container_executor_contract",
		"dry_run":                       report.DryRun,
		"execute_requested":             report.ExecuteRequested,
		"executed":                      report.Executed,
		"ready":                         report.Ready,
		"live_execution_allowed":        report.LiveExecutionAllowed,
		"live_approval_phrase_ok":       report.LiveApprovalPhraseOK,
		"requires_executor_gate":        true,
		"requires_matching_approved_by": true,
		"requires_exact_live_phrase":    true,
		"mutates_live_servers":          report.Executed,
		"steps":                         len(report.Steps),
		"blockers":                      len(report.Blockers),
		"stop_before":                   report.StopBefore,
	}
}

func containerExecutorVerificationMCPContract(report containerExecutorVerificationReport) map[string]interface{} {
	return map[string]interface{}{
		"kind":                            "container_executor_verification_contract",
		"gate_only":                       true,
		"ready":                           report.Ready,
		"apply_allowed":                   false,
		"execute_allowed":                 false,
		"mutates_live_servers":            false,
		"requires_live_executor_evidence": true,
		"requires_agent_evidence":         true,
		"requires_container_logscan":      true,
		"checks":                          len(report.Checks),
		"blockers":                        len(report.Blockers),
		"stop_before":                     report.StopBefore,
	}
}

func mcpCatalog(profile string) map[string]interface{} {
	profile = strings.ToLower(strings.TrimSpace(profile))
	if profile == "" {
		profile = "all"
	}
	profiles := map[string]interface{}{
		"local-lite": map[string]interface{}{
			"purpose":    "Smallest practical MCP exposure for local LLMs and short-context clients.",
			"user_model": "Expose only high-value personal OS, mail, document, schedule, map, archive-plan, and fallback tools. Hide browser/media/purchase/scheduled-send execution and advanced ops by default.",
			"install_path": []string{
				"meshclaw mcp --profile local-lite",
				"or set MESHCLAW_MCP_PROFILE=local-lite in the MCP client config",
			},
			"default_tools":  mcpProfileToolNames("local-lite"),
			"expansion_rule": "If local-lite cannot handle a request, call meshclaw_mcp_catalog with profile=claude-lite or all to identify the missing tool, then route the task to a larger client/profile.",
			"token_rule":     "Prefer this for local models. It keeps tools/list near 15-25 tools and keeps descriptions shorter than claude-lite.",
		},
		"claude-lite": map[string]interface{}{
			"purpose":    "Low-token MCP exposure for Claude, local models, and other clients where a 190-tool surface is too expensive.",
			"user_model": "Expose the common personal assistant, mail, document, browser, map, schedule, and safe planning tools by default. Use meshclaw_mcp_catalog or switch to the full profile only when a rare tool is needed.",
			"install_path": []string{
				"meshclaw mcp --profile claude-lite",
				"or set MESHCLAW_MCP_PROFILE=claude-lite in the MCP client config",
			},
			"default_tools":      mcpProfileToolNames("claude-lite"),
			"expansion_rule":     "If the request cannot be completed with the lite surface, call meshclaw_mcp_catalog with profile=all to identify the missing full-profile tool, then ask the operator to switch profiles or run that specific tool through a fuller client.",
			"evidence_contracts": mcpEvidenceContracts(),
			"token_rule":         "Keep this as the default for Claude and local models because tools/list is much smaller and tool/property descriptions are compacted.",
		},
		"one-machine-assistant": map[string]interface{}{
			"purpose":    "Single always-on Mac install. Signal Argos, briefings, local app automation, and optional local model endpoint run on one machine.",
			"user_model": "A normal user talks to one Argos Signal identity or one MCP server. MeshClaw produces artifacts and validated results; evidence is only the audit trail.",
			"install_path": []string{
				"pip install -U meshclaw vssh",
				"meshclaw init",
				"meshclaw setup assistant --json",
				"meshclaw setup signal --json",
				"meshclaw setup argos-runner --json",
				"meshclaw mcp",
			},
			"default_tools": []string{
				"meshclaw_mcp_catalog",
				"meshclaw_legacy_skills_audit",
				"meshclaw_setup_assistant",
				"meshclaw_model_config_status",
				"meshclaw_setup_signal",
				"meshclaw_messenger_targets",
				"meshclaw_scheduled_delivery_plan",
				"meshclaw_scheduled_delivery_apply",
				"meshclaw_argos_macos_doctor",
				"meshclaw_argos_macos_setup",
				"meshclaw_news_document",
				"meshclaw_argos_research",
				"meshclaw_calendar_list",
				"meshclaw_reminders_list",
				"meshclaw_contacts_search",
				"meshclaw_notes_search",
				"meshclaw_mail_accounts",
				"meshclaw_mail_summarize",
				"meshclaw_mail_search",
				"meshclaw_mail_thread",
				"meshclaw_mail_draft_reply",
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
				"meshclaw_audio_transcribe",
				"meshclaw_signal_call_doctor",
				"meshclaw_signal_call",
				"meshclaw_subscription_frontends",
				"meshclaw_data_archive_plan",
				"meshclaw_downloads_cleanup_plan",
				"meshclaw_downloads_cleanup_apply",
				"meshclaw_terminal_run",
				"meshclaw_shortcut_text_run",
				"meshclaw_result_save",
				"meshclaw_maps_search",
				"meshclaw_maps_directions",
				"meshclaw_maps_proof",
				"meshclaw_purchase_click",
				"meshclaw_argos_ask",
			},
			"macos_toolsets": []map[string]interface{}{
				{
					"id":      "read_personal_data",
					"purpose": "Read local personal OS data without mutation.",
					"tools":   []string{"meshclaw_calendar_list", "meshclaw_reminders_list", "meshclaw_contacts_search", "meshclaw_notes_search"},
					"examples": []string{
						"오늘 일정 뭐 있어?",
						"이번 주 할 일 보여줘",
						"김대리 연락처 찾아줘",
					},
					"model_rule": "Use these first for Calendar, Reminders, Contacts, and Notes reads. Do not route read-only personal data questions through generic argos_ask.",
				},
				{
					"id":      "mutate_personal_data",
					"purpose": "Plan or perform local Calendar/Reminders/Notes writes with explicit execute flags.",
					"tools":   []string{"meshclaw_calendar_create_event", "meshclaw_reminder_create", "meshclaw_reminder_complete", "meshclaw_reminder_delete", "meshclaw_note_create"},
					"examples": []string{
						"내일 오전 9시에 회의 일정 잡아줘",
						"퇴근길에 우유 사기 리마인더 추가해줘",
						"이 내용을 Notes에 메모해줘",
					},
					"model_rule": "Default to execute=false unless the user clearly confirms. For delete/complete/send-like actions, ask for confirmation before execute=true.",
				},
				{
					"id":      "visible_handoff",
					"purpose": "Open apps, URLs, files, maps, media surfaces, settings surfaces, and notifications in the user's visible macOS session.",
					"tools":   []string{"meshclaw_automation_open_app", "meshclaw_automation_open_url", "meshclaw_automation_open_file", "meshclaw_media_play", "meshclaw_app_settings_plan", "meshclaw_maps_search", "meshclaw_maps_directions", "meshclaw_notification_show"},
					"examples": []string{
						"Obsidian 열어줘",
						"방금 만든 문서를 Pages로 열어줘",
						"유튜브에서 재즈 틀 준비해줘",
						"강남역에서 서울역까지 길 알려줘",
					},
					"model_rule": "Prefer task-level file/map/app/media/settings tools for explicit open/handoff requests. For media/settings, plan choices first and only open a visible app/URL/settings pane after approval. Keep execute=false when a plan is enough.",
				},
				{
					"id":      "visible_map_handoff",
					"purpose": "Return map or directions links, and when the user asks to see the map, open the visible map UI and attach screen proof.",
					"tools":   []string{"meshclaw_maps_search", "meshclaw_maps_directions", "meshclaw_maps_proof", "meshclaw_automation_open_url", "meshclaw_screen_capture", "meshclaw_screen_record"},
					"examples": []string{
						"광화문 교보문고 지도 사진으로 보여줘",
						"강남역에서 서울역까지 길찾기 캡처해서 보내줘",
						"이 장소 지도 링크도 같이 줘",
					},
					"model_rule": "Always return a clickable Maps URL. If the user says 보여줘/사진/캡처/screenshot, prefer maps_proof: it returns the URL, opens the map, and captures still proof only with execute=true plus approve=true; do not answer with coordinates only.",
				},
				{
					"id":      "communication_alerts",
					"purpose": "Notify, remind, brief, or call the user through local macOS and Signal surfaces.",
					"tools":   []string{"meshclaw_notification_show", "meshclaw_reminder_create", "meshclaw_schedule_status", "meshclaw_schedule_run_once", "meshclaw_scheduled_delivery_plan", "meshclaw_scheduled_delivery_apply", "meshclaw_signal_call_doctor", "meshclaw_signal_call"},
					"examples": []string{
						"20분 뒤에 일어나라고 알려줘",
						"내일 아침 브리핑 보내줘",
						"승인되면 나한테 Signal로 전화해서 모닝콜 줘",
						"윤에게 매주 금요일 보고서를 보내는 예약 계획 세워줘",
					},
					"model_rule": "Use notification/reminder/schedule tools for normal alerts. For recurring delivery to a friend or room, use scheduled_delivery_plan first; it only resolves target and stores a first-run preview plan. Use scheduled_delivery_apply only after the user approves that preview; apply stores the job definition only and does not send. Use signal_call only after call_doctor is OK and never place a real call without execute=true plus approve=true.",
				},
				{
					"id":      "audio_transcription",
					"purpose": "Transcribe local audio or voice-note files into local text artifacts without sending audio to external services.",
					"tools":   []string{"meshclaw_audio_transcribe", "meshclaw_result_save"},
					"examples": []string{
						"이 음성파일 받아쓰기 해줘",
						"회의 녹음 텍스트로 바꿔줘",
						"받은 음성메시지 요약하기 전에 먼저 전사해줘",
					},
					"model_rule": "Default execute=false for privacy review. Use execute=true plus approve=true only for a user-supplied local audio path. Keep transcripts local unless the user explicitly asks to send or attach them.",
				},
				{
					"id":      "transactional_web_handoff",
					"purpose": "Research, compare, and prepare shopping, booking, account, form, and logged-in web tasks while stopping before irreversible submission.",
					"tools":   []string{"meshclaw_visible_browser_search", "meshclaw_browser_fetch", "meshclaw_automation_open_url", "meshclaw_argos_ask", "meshclaw_screen_capture", "meshclaw_screen_record", "meshclaw_result_save", "meshclaw_account_action_plan", "meshclaw_purchase_click", "meshclaw_subscription_frontends"},
					"examples": []string{
						"쿠팡에서 이 상품 가격이랑 배송 조건 비교해줘",
						"내일 저녁 강남 식당 예약 가능한 곳 찾아줘",
						"ChatGPT에 물어보고 답변만 가져와줘",
						"이 신청서 작성해두고 제출 전 화면 보여줘",
					},
					"model_rule": "Allowed: search, compare, open logged-in pages, fill drafts, prepare carts/forms/account pages, and capture proof. Use account_action_plan for billing/cancellation/plan-change requests. For Coupang or shopping final purchase, use purchase_click only after the final total, merchant, item, shipping, payment summary, button coordinate, confirmation text, execute=true, and approve=true are all present. Stop before payment/booking/submission if any required field is missing.",
				},
				{
					"id":      "file_cleanup_planning",
					"purpose": "Plan local file organization and Downloads cleanup without mutating files.",
					"tools":   []string{"meshclaw_downloads_cleanup_plan", "meshclaw_downloads_cleanup_apply", "meshclaw_result_save"},
					"examples": []string{
						"다운로드 폴더 정리 후보 보여줘",
						"오래된 설치 파일이랑 압축파일 정리 계획 세워줘",
						"큰 다운로드 파일 후보를 보고서로 저장해줘",
					},
					"model_rule": "Use cleanup_plan first and never move/delete/archive files from a plan. If status=needs_access, explain the access guidance or ask for a readable folder path. For apply, pass only explicit reviewed file paths to cleanup_apply with execute=true plus approve=true; it only moves regular files to a review folder and never deletes.",
				},
				{
					"id":      "legacy_skill_import",
					"purpose": "Inspect old MeshClaw prompt skills as reference material and migrate useful ideas into current approval-gated MCP tools.",
					"tools":   []string{"meshclaw_legacy_skills_audit", "meshclaw_mcp_catalog", "meshclaw_result_save"},
					"examples": []string{
						"예전 스킬 중 지금 MCP로 이미 대체된 것 보여줘",
						"예전 스킬에서 가져와야 할 기능 후보 정리해줘",
						"morning-prayer 같은 음성 스킬을 현행 Argos 도구로 매핑해줘",
					},
					"model_rule": "Treat legacy skills as read-only source material. Never call the old skill runner for normal users; use the audit to choose MCP replacements or propose a new bounded MCP tool with approval/evidence.",
				},
				{
					"id":      "durable_os_work",
					"purpose": "Run local commands or Shortcuts and save the result as Markdown/HTML artifacts.",
					"tools":   []string{"meshclaw_terminal_run", "meshclaw_shortcut_text_run", "meshclaw_result_save"},
					"examples": []string{
						"Downloads 폴더 용량 정리해서 보고서로 저장해줘",
						"내 단축어 실행해서 결과를 문서로 남겨줘",
						"방금 확인한 내용을 작업 결과로 저장해줘",
					},
					"model_rule": "Terminal and Shortcuts require execute=true plus approve=true. Use result_save after UI/browser work so the user gets an artifact, not only changed UI state.",
				},
				{
					"id":      "bounded_ui_fallback",
					"purpose": "Last-mile visible macOS automation when no task-level tool exists.",
					"tools":   []string{"meshclaw_argos_ask", "meshclaw_screen_capture", "meshclaw_screen_record", "meshclaw_automation_mouse_click"},
					"examples": []string{
						"Safari에서 이 페이지 열고 화면으로 보여줘",
						"설정 앱에서 이 항목까지 열어줘",
						"현재 화면을 3초 녹화해서 증거로 남겨줘",
					},
					"model_rule": "Use argos_ask only for natural-language OS tasks that do not match a first-class tool. Mouse clicks are advanced fallback and need explicit coordinates.",
				},
			},
			"first_success_tasks": []map[string]interface{}{
				{"intent": "오늘 브리핑", "tool": "Signal Argos / meshclaw_news_document", "expected": "short briefing or saved news document, not raw evidence counters"},
				{"intent": "브라우저로 세종대왕 검색해서 요약 리포트를 써줘", "tool": "meshclaw_argos_research", "expected": "attached Markdown/HTML report with cited [S1] source-grounded summary when source text was fetched, or a clearly labeled source-candidate note"},
				{"intent": "내일 회의 자료 만들어줘", "tool": "Signal Argos / meshclaw_document_create / meshclaw_presentation_create", "expected": "natural Korean reply plus DOCX/Markdown/HTML/PPTX attachments; raw paths hidden unless requested"},
				{"intent": "브라우저로 Apple Migration Assistant Mac 검색해서 결과를 엑셀 표로 정리해줘", "tool": "Signal Argos / meshclaw_spreadsheet_create", "expected": "editable XLSX/CSV/HTML search-result table with source type, usage notes, links, and fetched body excerpts when available"},
				{"intent": "이번 달 서버 비용 예산표를 엑셀로 만들어줘", "tool": "Signal Argos / meshclaw_spreadsheet_create", "expected": "natural Korean reply plus editable XLSX, CSV, and mobile HTML preview attachments"},
				{"intent": "간단한 발표자료 만들어줘", "tool": "meshclaw_presentation_create", "expected": "editable pptx + outline markdown + preview html + validation"},
				{"intent": "방금 만든 PPT를 3장으로 줄여줘", "tool": "Signal recent-artifact follow-up / meshclaw_presentation_edit", "expected": "new edited PPTX attachment from recent artifact context; no local path question and no in-place mutation"},
				{"intent": "강남역에서 서울역까지 대중교통", "tool": "meshclaw_maps_directions", "expected": "usable Maps URL; visible opening only if execute=true"},
				{"intent": "오늘 일정 뭐 있어?", "tool": "meshclaw_calendar_list", "expected": "read-only calendar result with KST human-readable times"},
				{"intent": "최근 메일 요약해줘", "tool": "meshclaw_mail_summarize / Signal mail fallback", "expected": "concise grouped summary across configured accounts; no mailbox question for read-only summary"},
				{"intent": "첫 번째 메일에 답장 초안 써줘", "tool": "Signal recent-mail follow-up / meshclaw_mail_draft_reply", "expected": "local unsent draft reply using the recent mail reference; no email transmission"},
			},
			"artifact_contract": map[string]interface{}{
				"url":        "primary human-readable preview or artifact",
				"markdown":   "editable outline/report when useful",
				"office":     "pptx/docx/pdf paths when generated",
				"preview":    "thumbnail/image path when available",
				"validation": "proof that the requested artifact exists and is structurally usable",
			},
			"signal_reply_rule": "Return natural Korean prose first. Attach files for documents/decks; hide raw paths, evidence IDs, JSON, internal action names, and UTC timestamps unless the user asks for diagnostics.",
		},
		"multi-node-ops": map[string]interface{}{
			"purpose": "MeshClaw Ops profile: one controller Runtime plus optional MeshClaw Agent/vssh nodes. The controller summarizes inventory, policy, evidence, and fleet state for MCP clients.",
			"install_path": []string{
				"install MeshClaw on the controller Mac first",
				"install MeshClaw Agent and vssh only on nodes that need local collection or execution",
				"use meshclaw_server_list, meshclaw_agent_workloads, and meshclaw_ops_control from the controller MCP",
			},
			"default_tools": []string{
				"meshclaw_mcp_catalog",
				"meshclaw_server_list",
				"meshclaw_ops_control",
				"meshclaw_monitor_check",
				"meshclaw_agent_workloads",
				"meshclaw_agent_changes",
				"meshclaw_agent_security",
				"meshclaw_workflow_run",
				"meshclaw_evidence_latest",
			},
		},
		"development": map[string]interface{}{
			"purpose": "MacBook development mode for Codex/Claude/Cursor/AI Studio. Signal sending is opt-in; dev-memo is local-first.",
			"install_path": []string{
				"go build -o dist/meshclaw ./cmd/meshclaw",
				"dist/meshclaw mcp",
				"dist/meshclaw dev-memo add ...",
			},
			"default_tools": []string{
				"mission_get",
				"mission_list",
				"mission_update",
				"task_add",
				"artifact_add",
				"meshclaw_mcp_surface",
				"meshclaw_mcp_catalog",
				"meshclaw_argos_research",
				"meshclaw_visible_browser_search",
			},
		},
	}
	categories := []map[string]interface{}{
		{"id": "core", "purpose": "Mission/task/artifact state and durable work memory", "tools": []string{"mission_get", "mission_list", "mission_update", "task_add", "task_complete", "artifact_add"}},
		{"id": "setup", "purpose": "Install, doctor, daemon, Signal, model gateway, MCP readiness, local data retention planning, and legacy skill import planning", "tools": []string{"meshclaw_quickstart", "meshclaw_doctor", "meshclaw_setup_assistant", "meshclaw_model_config_status", "meshclaw_setup_signal", "meshclaw_setup_argos_runner", "meshclaw_daemon_signal_status", "meshclaw_daemon_schedule_status", "meshclaw_data_doctor", "meshclaw_data_archive_plan", "meshclaw_legacy_skills_audit"}},
		{"id": "macos_toolsets", "purpose": "Model-friendly OS toolsets for local personal data, communication/alerts, scheduled delivery planning, visible app/file/map/media/settings handoff, audio transcription, transactional web preparation, file cleanup planning, legacy skill import planning, Terminal/Shortcuts result artifacts, and bounded UI fallback", "tools": []string{"meshclaw_argos_macos_doctor", "meshclaw_argos_macos_setup", "meshclaw_calendar_list", "meshclaw_calendar_create_event", "meshclaw_reminders_list", "meshclaw_reminder_create", "meshclaw_reminder_complete", "meshclaw_reminder_delete", "meshclaw_contacts_search", "meshclaw_notes_search", "meshclaw_note_create", "meshclaw_notification_show", "meshclaw_schedule_status", "meshclaw_schedule_run_once", "meshclaw_scheduled_delivery_plan", "meshclaw_scheduled_delivery_apply", "meshclaw_signal_call_doctor", "meshclaw_signal_call", "meshclaw_automation_open_app", "meshclaw_automation_open_url", "meshclaw_automation_open_file", "meshclaw_media_play", "meshclaw_audio_transcribe", "meshclaw_app_settings_plan", "meshclaw_visible_browser_search", "meshclaw_browser_fetch", "meshclaw_account_action_plan", "meshclaw_purchase_click", "meshclaw_subscription_frontends", "meshclaw_downloads_cleanup_plan", "meshclaw_downloads_cleanup_apply", "meshclaw_legacy_skills_audit", "meshclaw_terminal_run", "meshclaw_shortcut_text_run", "meshclaw_result_save", "meshclaw_maps_search", "meshclaw_maps_directions", "meshclaw_maps_proof", "meshclaw_screen_capture", "meshclaw_screen_record", "meshclaw_argos_ask"}},
		{"id": "personal_assistant", "purpose": "Local macOS assistant actions with plan-only defaults for mutations; prefer first-class Calendar/Reminders/Contacts/Notes/Maps/File/Media/Audio/Settings tools before generic argos_ask", "tools": []string{"meshclaw_calendar_list", "meshclaw_calendar_create_event", "meshclaw_reminders_list", "meshclaw_reminder_create", "meshclaw_reminder_complete", "meshclaw_reminder_delete", "meshclaw_contacts_search", "meshclaw_notes_search", "meshclaw_note_create", "meshclaw_notification_show", "meshclaw_automation_open_file", "meshclaw_media_play", "meshclaw_audio_transcribe", "meshclaw_app_settings_plan", "meshclaw_terminal_run", "meshclaw_shortcut_text_run", "meshclaw_result_save", "meshclaw_maps_search", "meshclaw_maps_directions", "meshclaw_maps_proof"}},
		{"id": "mail", "purpose": "Read/search/summarize/draft local mail without sending or mutating unless explicitly approved. Signal may use short numbered follow-ups from recent mail lists for draft replies.", "tools": []string{"meshclaw_mail_accounts", "meshclaw_mail_summarize", "meshclaw_mail_search", "meshclaw_mail_thread", "meshclaw_mail_read_many", "meshclaw_mail_draft_reply", "meshclaw_mail_compose", "meshclaw_mail_send", "meshclaw_mail_attachments", "meshclaw_mail_move", "meshclaw_mail_delete", "meshclaw_mail_watch_once"}},
		{"id": "visible_work", "purpose": "Browser, source-grounded research reports with cited [S1] summaries, document, spreadsheet, presentation, and screen-proof work exposed as first-class MCP tools", "tools": []string{"meshclaw_browser_fetch", "meshclaw_browser_search", "meshclaw_visible_browser_search", "meshclaw_document_create", "meshclaw_document_export", "meshclaw_spreadsheet_create", "meshclaw_presentation_create", "meshclaw_presentation_verify", "meshclaw_presentation_edit", "meshclaw_presentation_export", "meshclaw_screen_capture", "meshclaw_screen_record", "meshclaw_argos_research", "meshclaw_news_document"}},
		{"id": "messenger", "purpose": "Signal targets, reports, scheduled delivery plans, approvals, and one-way briefing/report rooms", "tools": []string{"meshclaw_messenger_targets", "meshclaw_scheduled_delivery_plan", "meshclaw_scheduled_delivery_apply", "meshclaw_messenger_target_add", "meshclaw_messenger_report", "meshclaw_messenger_send_report", "meshclaw_signal_rooms_doctor", "meshclaw_signal_room_bind"}},
		{"id": "fleet_ops", "purpose": "Inventory, monitor state, desired-state reconciliation, workflows, policy, evidence, and safe server operations", "tools": []string{"meshclaw_server_list", "meshclaw_monitor_check", "meshclaw_ops_control", "meshclaw_reconcile_validate_desired", "meshclaw_reconcile_plan", "meshclaw_reconcile_approval_request", "meshclaw_reconcile_apply_gate", "meshclaw_reconcile_apply_plan", "meshclaw_reconcile_execution_preview", "meshclaw_reconcile_verification_plan", "meshclaw_reconcile_runbook", "meshclaw_reconcile_runbook_check", "meshclaw_reconcile_rollback_plan", "meshclaw_reconcile_completion_plan", "meshclaw_reconcile_readiness_summary", "meshclaw_reconcile_run_once", "meshclaw_autoheal_plan", "meshclaw_autoheal_container_apply_plan", "meshclaw_autoheal_container_verification_plan", "meshclaw_autoheal_container_runbook", "meshclaw_autoheal_container_runbook_check", "meshclaw_autoheal_container_rollback_plan", "meshclaw_autoheal_container_completion_plan", "meshclaw_autoheal_container_readiness_summary", "meshclaw_autoheal_container_executor_gate", "meshclaw_autoheal_container_executor", "meshclaw_autoheal_container_executor_verify", "meshclaw_autoheal_apply_safe", "meshclaw_agent_workloads", "meshclaw_agent_changes", "meshclaw_agent_security", "meshclaw_workflow_run", "meshclaw_evidence_latest"}},
	}
	payload := map[string]interface{}{
		"kind":               "meshclaw_mcp_catalog",
		"profile":            profile,
		"product_rule":       "MeshClaw Platform has Argos Assistant, MeshClaw Ops, and MeshClaw Agent product lines on one shared policy/evidence core. Use native tools for runtime state, policy, approvals, evidence, publication, server operations, and macOS automation. External chat UIs are optional frontends, not required runtimes.",
		"artifact_rule":      "For user-facing work, the main output is the requested artifact/result. Evidence should prove creation/validation; it should not be the thing the user reads first.",
		"single_user_rule":   "A normal user can install MeshClaw Platform on one always-on Mac and talk to one Argos Assistant Signal identity. Multi-node MeshClaw Ops installs are optional extensions.",
		"profiles":           profiles,
		"categories":         categories,
		"tool_count":         len(mcpTools()),
		"profile_tool_count": len(mcpToolsForProfile(profile)),
		"scope_note":         "MCP clients should call task-level tools first; low-level vssh or UI primitives are advanced fallbacks.",
		"source_of_truth":    []string{"README.md", "docs/ARCHITECTURE_2026-05-23.md", "docs/PUBLIC_INSTALL_UX.md", "docs/MCP_PERSONAL_ASSISTANT_UPGRADE.md"},
		"macmini_canonical":  "For this operator, Mac mini is the user-facing Signal Argos Runtime; MacBook Signal remains no-send unless explicitly configured.",
		"macbook_core_note":  "Mission writes are MacBook-canonical in this development topology; public one-machine installs may keep Core on the always-on Mac.",
	}
	if profile != "all" {
		if selected, ok := profiles[profile]; ok {
			payload["selected_profile"] = selected
		} else {
			payload["profile_warning"] = "unknown profile; returned all profiles"
		}
	}
	return payload
}

func evidenceRef(record evidence.Record) mcpEvidenceRef {
	return mcpEvidenceRef{
		ID:       record.ID,
		Kind:     record.Kind,
		Summary:  record.Summary,
		StoredAt: record.StoredAt,
		Time:     record.Time,
	}
}

func slimServiceItems(items []serviceTriageItem, limit int) []mcpServiceItem {
	if len(items) <= limit {
		limit = len(items)
	}
	out := make([]mcpServiceItem, 0, limit)
	for _, item := range items[:limit] {
		out = append(out, mcpServiceItem{
			Host:      item.Host,
			Service:   item.Service,
			Class:     item.Class,
			Mode:      item.Mode,
			Severity:  item.Severity,
			Judgement: item.Judgement,
			Next:      item.Next,
		})
	}
	return out
}

func callMCPTool(name string, args map[string]interface{}) (interface{}, error) {
	switch name {
	case "meshclaw_ai_guide":
		return map[string]interface{}{
			"role_split": map[string]interface{}{
				"meshclaw": []string{"policy", "state", "capability registry", "workflow runner", "evidence", "approval boundaries"},
				"vssh":     []string{"low-level structured remote execution", "parallel exec", "typed facts", "daemon RPC", "transport debugging"},
			},
			"readiness_gate_contract": map[string]interface{}{
				"summary_tools_are_approval": false,
				"executor_requires":          []string{"operator approval evidence", "ready completion-plan evidence", "rollback evidence", "final verification or logscan evidence", "container runtime_evidence_checklist review before self-heal execution"},
				"desired_state": map[string]interface{}{
					"readiness_tool": "meshclaw_reconcile_readiness_summary",
					"stop_before":    []string{"treating readiness summary as operator approval", "starting reconcile executor without ready completion-plan evidence"},
				},
				"container_self_heal": map[string]interface{}{
					"readiness_tool": "meshclaw_autoheal_container_readiness_summary",
					"stop_before":    []string{"treating readiness summary as operator approval", "starting container executor without ready completion-plan evidence"},
				},
				"mutates_live_servers": false,
			},
			"evidence_contracts": mcpEvidenceContracts(),
			"mcp_surface":        mcpSurfaceGuide(),
			"guidance":           aiGuideEntries(),
		}, nil
	case "meshclaw_architecture":
		return productArchitectureReport(), nil
	case "mission_get":
		store, err := mission.DefaultStore()
		if err != nil {
			return nil, err
		}
		doc, path, err := store.Get(stringArg(args, "id"))
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"kind":       "meshclaw_core_mission_v0",
			"mission":    doc,
			"summary":    doc.Summary(),
			"path":       path,
			"canonical":  "macbook",
			"scope_note": "Phase 1 canonical Core storage is the MacBook user's ~/.meshclaw/state/missions. macmini Signal read/sync is Phase 1.5/2.",
		}, nil
	case "mission_list":
		store, err := mission.DefaultStore()
		if err != nil {
			return nil, err
		}
		list, path, err := store.List()
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"kind":       "meshclaw_core_mission_list_v0",
			"missions":   list,
			"path":       path,
			"canonical":  "macbook",
			"scope_note": "Phase 1 is read-only and local to MacBook AI Studio MCP. Do not infer macmini Signal has the same mission files.",
		}, nil
	case "mission_update":
		if err := ensureMacBookMissionWrite(); err != nil {
			return nil, err
		}
		store, err := mission.DefaultStore()
		if err != nil {
			return nil, err
		}
		opts := mission.UpdateOptions{}
		if _, ok := args["goal"]; ok {
			value := stringArg(args, "goal")
			opts.Goal = &value
		}
		if _, ok := args["status"]; ok {
			value := stringArg(args, "status")
			opts.Status = &value
		}
		if _, ok := args["next_action"]; ok {
			value := stringArg(args, "next_action")
			opts.NextAction = &value
		}
		doc, path, err := store.Update(stringArg(args, "id"), opts)
		if err != nil {
			return nil, err
		}
		return missionMutationResult("meshclaw_core_mission_update_v0", doc, path, nil), nil
	case "task_add":
		if err := ensureMacBookMissionWrite(); err != nil {
			return nil, err
		}
		store, err := mission.DefaultStore()
		if err != nil {
			return nil, err
		}
		doc, task, path, err := store.AddTask(stringArg(args, "id"), stringArg(args, "title"), stringArg(args, "status"), stringArg(args, "notes"))
		if err != nil {
			return nil, err
		}
		return missionMutationResult("meshclaw_core_task_add_v0", doc, path, map[string]interface{}{"task": task}), nil
	case "task_complete":
		if err := ensureMacBookMissionWrite(); err != nil {
			return nil, err
		}
		store, err := mission.DefaultStore()
		if err != nil {
			return nil, err
		}
		doc, task, path, err := store.CompleteTask(stringArg(args, "id"), stringArg(args, "task_id"), stringArg(args, "notes"))
		if err != nil {
			return nil, err
		}
		return missionMutationResult("meshclaw_core_task_complete_v0", doc, path, map[string]interface{}{"task": task}), nil
	case "artifact_add":
		if err := ensureMacBookMissionWrite(); err != nil {
			return nil, err
		}
		store, err := mission.DefaultStore()
		if err != nil {
			return nil, err
		}
		doc, artifact, path, err := store.AddArtifact(stringArg(args, "id"), map[string]interface{}{
			"kind":     stringArg(args, "kind"),
			"ref":      stringArg(args, "ref"),
			"title":    stringArg(args, "title"),
			"node":     stringArg(args, "node"),
			"evidence": stringArg(args, "evidence"),
			"notes":    stringArg(args, "notes"),
		})
		if err != nil {
			return nil, err
		}
		return missionMutationResult("meshclaw_core_artifact_add_v0", doc, path, map[string]interface{}{"artifact": artifact}), nil
	case "meshclaw_quickstart":
		return buildQuickstartReport()
	case "meshclaw_doctor":
		return doctor.Self(), nil
	case "meshclaw_setup_assistant":
		return buildAssistantSetupReport(), nil
	case "meshclaw_model_config_status":
		report, err := modelConfigStatus()
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"kind":          report.Kind,
			"path":          report.Path,
			"exists":        report.Exists,
			"base_url":      report.BaseURL,
			"model":         report.Model,
			"api_key":       report.APIKey,
			"max_tokens":    report.MaxTokens,
			"temperature":   report.Temperature,
			"secret_policy": "raw API keys are write-only via `meshclaw model-config`; MCP returns only masked status",
			"setup_hint":    "Paste gateway config with `meshclaw model-config import --json < config.json`, then use `/model <model> <base_url>` in Signal rooms.",
		}, nil
	case "meshclaw_setup_signal":
		report := buildSignalSetupReport()
		if !boolArg(args, "repair", false) {
			return report, nil
		}
		repair := repairSignalSetup(report)
		report = buildSignalSetupReport()
		return map[string]interface{}{"setup": report, "repair": repair}, nil
	case "meshclaw_setup_argos_runner":
		argv := []string{}
		if value := strings.TrimSpace(stringArg(args, "signal_start_click")); value != "" {
			argv = append(argv, "--signal-start-click", value)
		}
		if value := strings.TrimSpace(stringArg(args, "signal_hangup_click")); value != "" {
			argv = append(argv, "--signal-hangup-click", value)
		}
		if value := strings.TrimSpace(stringArg(args, "ui_runner")); value != "" {
			argv = append(argv, "--ui-runner", value)
		}
		report, err := setupArgosRunner(argv)
		return map[string]interface{}{"result": report, "error": errString(err)}, nil
	case "meshclaw_daemon_signal_status":
		result, err := manageSignalDispatcher("status")
		return map[string]interface{}{"result": result, "error": errString(err)}, nil
	case "meshclaw_daemon_schedule_status":
		result, err := manageScheduleRunner("status")
		return map[string]interface{}{"result": result, "schedule_status": signalSetupScheduleStatus(time.Now().UTC()), "error": errString(err)}, nil
	case "meshclaw_mcp_surface":
		return mcpSurfaceGuide(), nil
	case "meshclaw_mcp_catalog":
		return mcpCatalog(stringArg(args, "profile")), nil
	case "meshclaw_local_assistant":
		return localAssistantMCP(args)
	case "meshclaw_tool_recommend":
		return recommendToolMCP(stringArg(args, "intent"), stringArg(args, "subject")), nil
	case "meshclaw_server_list":
		return inventory.DefaultNodes(), nil
	case "meshclaw_inventory_discover":
		return inventory.Discover()
	case "meshclaw_inventory_diff":
		return inventory.ComputeDiff()
	case "meshclaw_inventory_override_list":
		nodes, err := inventory.LoadOverrides()
		if err != nil {
			if os.IsNotExist(err) {
				nodes = []inventory.Node{}
			} else {
				return nil, err
			}
		}
		return map[string]interface{}{"path": inventory.OverridesPath(), "nodes": nodes}, nil
	case "meshclaw_inventory_override_set":
		node := inventory.Node{
			Name:     stringArg(args, "node"),
			Role:     stringArg(args, "role"),
			Location: stringArg(args, "location"),
			User:     stringArg(args, "user"),
			Tags:     splitCSV(stringArg(args, "tags")),
		}
		nodes, err := inventory.SetOverride(node)
		if err != nil {
			return nil, err
		}
		record, storeErr := evidence.Store("inventory-override-set", node.Name, strings.Join(node.Tags, ","), map[string]interface{}{"path": inventory.OverridesPath(), "nodes": nodes})
		return map[string]interface{}{"path": inventory.OverridesPath(), "nodes": nodes, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_inventory_override_remove":
		node := stringArg(args, "node")
		nodes, removed, err := inventory.RemoveOverride(node)
		if err != nil {
			return nil, err
		}
		record, storeErr := evidence.Store("inventory-override-remove", node, fmt.Sprintf("removed=%t", removed), map[string]interface{}{"path": inventory.OverridesPath(), "removed": removed, "nodes": nodes})
		return map[string]interface{}{"path": inventory.OverridesPath(), "removed": removed, "nodes": nodes, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_workers":
		return workers.DefaultWorkers(), nil
	case "meshclaw_workspace_list":
		list, path, err := workspace.List()
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"path": path, "workspaces": list}, nil
	case "meshclaw_workspace_add":
		ws := workspace.Workspace{
			ID:      stringArg(args, "id"),
			Host:    stringArg(args, "host"),
			Path:    stringArg(args, "path"),
			Owner:   stringArg(args, "owner"),
			Purpose: stringArg(args, "purpose"),
			Branch:  stringArg(args, "branch"),
			Source:  stringArg(args, "source"),
		}
		saved, path, err := workspace.Upsert(ws)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"path": path, "workspace": saved}, nil
	case "meshclaw_workspace_frontend":
		frontend := workspace.Frontend{
			ID:       stringArg(args, "provider"),
			Provider: stringArg(args, "provider"),
			Mode:     "subscription_frontend",
			App:      stringArg(args, "app"),
			URL:      stringArg(args, "url"),
			Login:    firstNonEmpty(stringArg(args, "login"), "user-approved monthly subscription session"),
			Status:   firstNonEmpty(stringArg(args, "status"), "configured"),
		}
		ws, path, err := workspace.UpsertFrontend(stringArg(args, "id"), frontend)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"path": path, "workspace": ws}, nil
	case "meshclaw_workspace_activity":
		record, err := workspace.RecordActivity(workspace.Activity{
			WorkspaceID: stringArg(args, "id"),
			Actor:       stringArg(args, "actor"),
			Action:      stringArg(args, "action"),
			Summary:     stringArg(args, "summary"),
			Evidence:    stringArg(args, "evidence"),
		})
		return map[string]interface{}{"evidence": record}, err
	case "meshclaw_capability_list":
		return map[string]interface{}{"path": capability.Path(), "capabilities": capability.List()}, nil
	case "meshclaw_capability_validate":
		return capability.Validate(stringArg(args, "path")), nil
	case "meshclaw_capability_recommend":
		return capability.Recommend(stringArg(args, "intent")), nil
	case "meshclaw_mail_setup":
		result, err := mailadapter.Setup(mailadapter.SetupOptions{
			Email:    stringArg(args, "email"),
			Mode:     stringArg(args, "mode"),
			Host:     stringArg(args, "host"),
			Port:     intArg(args, "port", 0),
			Username: stringArg(args, "username"),
			Service:  stringArg(args, "service"),
			Account:  stringArg(args, "account"),
			Execute:  boolArg(args, "execute", false),
		})
		return map[string]interface{}{"result": result, "error": errString(err)}, nil
	case "meshclaw_mail_doctor":
		result, err := mailadapter.Doctor(mailadapter.DoctorOptions{
			Account:    stringArg(args, "account"),
			CheckLogin: boolArg(args, "check_login", true),
		})
		return map[string]interface{}{"result": result, "error": errString(err)}, nil
	case "meshclaw_mail_discover_keychain":
		result, err := mailadapter.DiscoverKeychain()
		return map[string]interface{}{"result": result, "error": errString(err)}, nil
	case "meshclaw_mail_accounts":
		return mailadapter.ListAccounts()
	case "meshclaw_mail_search":
		since, err := parseMailSince(stringArg(args, "since"))
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(stringArg(args, "account")) == "" {
			result, err := mcpMailSearchAll(stringArg(args, "query"), since, intArg(args, "limit", 10))
			record, storeErr := evidence.Store("mail-search-all", "mail", fmt.Sprintf("accounts=%d errors=%d query=%s", len(result.Results), len(result.Errors), stringArg(args, "query")), result)
			return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr), "error": errString(err), "scope_note": "Read-only mail search across configured accounts. No body read, attachment download, draft, mutation, or send occurred."}, nil
		}
		result, err := mailadapter.Search(mailadapter.SearchOptions{
			Account: stringArg(args, "account"),
			Query:   stringArg(args, "query"),
			Since:   since,
			Limit:   intArg(args, "limit", 10),
		})
		record, storeErr := evidence.Store("mail-search", result.Account.ID, fmt.Sprintf("messages=%d query=%s", len(result.Messages), stringArg(args, "query")), result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr), "error": errString(err)}, nil
	case "meshclaw_mail_summarize":
		since, err := parseMailSince(firstNonEmpty(stringArg(args, "since"), "24h"))
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(stringArg(args, "account")) == "" {
			result, err := mcpMailSearchAll(stringArg(args, "query"), since, intArg(args, "limit", 10))
			summary := summarizeMailMultiSearch(result, stringArg(args, "query"))
			record, storeErr := evidence.Store("mail-summarize-all", "mail", summary, result)
			return map[string]interface{}{"summary": summary, "result": result, "evidence": record, "store_error": errString(storeErr), "error": errString(err), "scope_note": "Read-only mail summary across configured accounts. No body read, attachment download, draft, mutation, or send occurred."}, nil
		}
		result, err := mailadapter.Search(mailadapter.SearchOptions{
			Account: stringArg(args, "account"),
			Query:   stringArg(args, "query"),
			Since:   since,
			Limit:   intArg(args, "limit", 10),
		})
		summary := summarizeMailSearch(result, stringArg(args, "query"))
		record, storeErr := evidence.Store("mail-summarize", result.Account.ID, summary, result)
		return map[string]interface{}{"summary": summary, "result": result, "evidence": record, "store_error": errString(storeErr), "error": errString(err), "scope_note": "Read-only mail summary. No body read, attachment download, draft, mutation, or send occurred."}, nil
	case "meshclaw_mail_thread":
		message, err := mailadapter.Read(mailadapter.ReadOptions{
			Account: stringArg(args, "account"),
			ID:      stringArg(args, "id"),
			MaxBody: intArg(args, "max_body", 5000),
		})
		record, storeErr := evidence.Store("mail-thread-read", stringArg(args, "account"), stringArg(args, "id"), message)
		return map[string]interface{}{"message": message, "evidence": record, "store_error": errString(storeErr), "error": errString(err)}, nil
	case "meshclaw_mail_read_many":
		result, err := mailadapter.ReadMany(mailadapter.ReadManyOptions{
			Account: stringArg(args, "account"),
			IDs:     splitCSV(stringArg(args, "ids")),
			MaxBody: intArg(args, "max_body", 5000),
		})
		record, storeErr := evidence.Store("mail-read-many", stringArg(args, "account"), fmt.Sprintf("messages=%d errors=%d", len(result.Messages), len(result.Errors)), result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr), "error": errString(err)}, nil
	case "meshclaw_mail_attachments":
		result, err := mailadapter.DownloadAttachments(mailadapter.AttachmentOptions{
			Account: stringArg(args, "account"),
			ID:      stringArg(args, "id"),
			Dir:     stringArg(args, "dir"),
			Approve: boolArg(args, "approve", false),
		})
		record, storeErr := evidence.Store("mail-attachments", stringArg(args, "account"), fmt.Sprintf("id=%s files=%d executed=%t", stringArg(args, "id"), len(result.Files), result.Executed), result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr), "error": errString(err)}, nil
	case "meshclaw_mail_move":
		result, err := mailadapter.Move(mailadapter.MutateOptions{
			Account: stringArg(args, "account"),
			IDs:     splitCSV(stringArg(args, "ids")),
			Target:  stringArg(args, "target"),
			Approve: boolArg(args, "approve", false),
		})
		record, storeErr := evidence.Store("mail-move", stringArg(args, "account"), fmt.Sprintf("ids=%d target=%s executed=%t", len(result.IDs), result.Target, result.Executed), result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr), "error": errString(err)}, nil
	case "meshclaw_mail_delete":
		result, err := mailadapter.Delete(mailadapter.MutateOptions{
			Account: stringArg(args, "account"),
			IDs:     splitCSV(stringArg(args, "ids")),
			Approve: boolArg(args, "approve", false),
		})
		record, storeErr := evidence.Store("mail-delete", stringArg(args, "account"), fmt.Sprintf("ids=%d executed=%t", len(result.IDs), result.Executed), result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr), "error": errString(err)}, nil
	case "meshclaw_mail_draft_reply":
		draft, err := mailadapter.DraftReply(stringArg(args, "account"), stringArg(args, "id"), stringArg(args, "intent"))
		record, storeErr := evidence.Store("mail-draft-reply", stringArg(args, "account"), draft.ID, draft)
		return map[string]interface{}{"draft": draft, "evidence": record, "store_error": errString(storeErr), "error": errString(err)}, nil
	case "meshclaw_mail_compose":
		draft, err := mailadapter.Compose(mailadapter.ComposeOptions{
			Account: stringArg(args, "account"),
			To:      splitCSV(stringArg(args, "to")),
			Subject: stringArg(args, "subject"),
			Body:    stringArg(args, "body"),
		})
		record, storeErr := evidence.Store("mail-compose", stringArg(args, "account"), draft.ID, draft)
		return map[string]interface{}{"draft": draft, "evidence": record, "store_error": errString(storeErr), "error": errString(err)}, nil
	case "meshclaw_mail_send":
		draft := stringArg(args, "draft")
		decision := policy.Evaluate(policy.Request{Subject: "mcp", Action: "email_send", Resource: "email", Context: "mail send draft " + draft})
		result, err := mailadapter.SendDraft(mailadapter.SendOptions{DraftID: draft, Approve: boolArg(args, "approve", false)})
		record, storeErr := evidence.Store("mail-send", "mail", fmt.Sprintf("draft=%s executed=%t policy=%s", draft, result.Executed, decision.Decision), map[string]interface{}{"result": result, "policy": decision})
		return map[string]interface{}{"result": result, "policy": decision, "evidence": record, "store_error": errString(storeErr), "error": errString(err)}, nil
	case "meshclaw_mail_watch_once":
		duration, parseErr := time.ParseDuration(firstNonEmpty(stringArg(args, "since"), "15m"))
		if parseErr != nil {
			return nil, parseErr
		}
		result, err := mailadapter.WatchOnce(mailadapter.WatchOptions{Account: stringArg(args, "account"), Since: duration, Limit: intArg(args, "limit", 10)})
		record, storeErr := evidence.Store("mail-watch-once", result.Account.ID, fmt.Sprintf("messages=%d since=%s", len(result.Messages), result.Since.Format(time.RFC3339)), result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr), "error": errString(err)}, nil
	case "meshclaw_adapter_list":
		return map[string]interface{}{"adapters": runtimeflow.AdapterRegistry()}, nil
	case "meshclaw_opsdb_status":
		db := opsdb.Default()
		ensure := boolArg(args, "ensure", false)
		if ensure {
			if err := db.Ensure(); err != nil {
				return nil, err
			}
		}
		return buildOpsDBStatus(db, ensure), nil
	case "meshclaw_opsdb_events":
		limit := intArg(args, "limit", 20)
		if limit <= 0 {
			return nil, fmt.Errorf("limit must be greater than 0")
		}
		node := strings.TrimSpace(stringArg(args, "node"))
		kind := strings.TrimSpace(stringArg(args, "kind"))
		recent, err := opsdb.Default().Recent(opsdb.RecentOptions{Node: node, Kind: kind, Limit: limit})
		if err != nil {
			return nil, err
		}
		return opsDBEventsReport{Kind: "meshclaw_opsdb_events", Node: node, Filter: kind, Limit: limit, Events: recent.Events, Evidence: recent.Evidence}, nil
	case "meshclaw_opsdb_power_events":
		window, err := parseOptionalDurationArg(args, "window", 15*time.Minute)
		if err != nil {
			return nil, err
		}
		uptimeMax, err := parseOptionalDurationArg(args, "uptime_max", 2*time.Hour)
		if err != nil {
			return nil, err
		}
		minNodes := intArg(args, "min_nodes", 2)
		limit := intArg(args, "limit", 10)
		if minNodes <= 0 || limit <= 0 {
			return nil, fmt.Errorf("min_nodes and limit must be greater than 0")
		}
		db := opsdb.Default()
		report, err := db.DetectPowerEvents(opsdb.PowerEventOptions{Window: window, UptimeMax: uptimeMax, MinNodes: minNodes, Limit: limit})
		if err != nil {
			return nil, err
		}
		if boolArg(args, "record", false) {
			for _, incident := range report.Incidents {
				_, _ = db.AppendEventRecord(opsdb.Event{
					Time:     incident.Time,
					Kind:     "power_event",
					Node:     "fleet",
					Severity: incident.Severity,
					Summary:  incident.Summary,
					Source:   "mcp-power-events",
					Tags:     []string{"power", "reboot", "physical-layer"},
					Data: map[string]interface{}{
						"confidence": incident.Confidence,
						"nodes":      incident.Nodes,
					},
				})
			}
		}
		return report, nil
	case "meshclaw_opsdb_record":
		summary := strings.TrimSpace(stringArg(args, "summary"))
		if summary == "" {
			return nil, fmt.Errorf("summary is required")
		}
		db := opsdb.Default()
		event, err := db.AppendEventRecord(opsdb.Event{
			Kind:       firstNonEmpty(stringArg(args, "kind"), "observation"),
			Node:       strings.TrimSpace(stringArg(args, "node")),
			Severity:   firstNonEmpty(stringArg(args, "severity"), "info"),
			Summary:    summary,
			Source:     firstNonEmpty(stringArg(args, "source"), "mcp"),
			EvidenceID: strings.TrimSpace(stringArg(args, "evidence")),
			Tags:       splitCSV(stringArg(args, "tags")),
		})
		if err != nil {
			return nil, err
		}
		return opsDBRecordReport{Kind: "meshclaw_opsdb_record", Event: event, Path: db.DriftPath(firstNonEmpty(event.Node, "fleet"))}, nil
	case "meshclaw_monitor_check":
		output, record, storeErr, err := collectMonitorCheckFresh()
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"states": output.States, "alerts": output.Alerts, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_agent_workloads":
		maxAge := 24 * time.Hour
		if raw := strings.TrimSpace(stringArg(args, "max_age")); raw != "" {
			if raw == "0" || strings.EqualFold(raw, "all") {
				maxAge = 0
			} else {
				parsed, err := time.ParseDuration(raw)
				if err != nil {
					return nil, fmt.Errorf("max_age must be a duration, 0, or all")
				}
				maxAge = parsed
			}
		}
		report, err := buildAgentWorkloadsReport(maxAge, intArg(args, "top", 3))
		if err != nil {
			return nil, err
		}
		return report, nil
	case "meshclaw_agent_changes":
		report, err := buildAgentChangesReport(firstNonEmpty(stringArg(args, "node"), "all"))
		if err != nil {
			return nil, err
		}
		return report, nil
	case "meshclaw_agent_security":
		maxAge := 24 * time.Hour
		if raw := strings.TrimSpace(stringArg(args, "max_age")); raw != "" {
			duration, err := time.ParseDuration(raw)
			if err != nil || duration < 0 {
				return nil, fmt.Errorf("max_age must be a valid duration")
			}
			maxAge = duration
		}
		report, err := buildAgentSecurityReport(maxAge)
		if err != nil {
			return nil, err
		}
		return report, nil
	case "meshclaw_agent_inventory_plan":
		report, err := buildAgentInventoryPlan(boolArg(args, "apply", false))
		if err != nil {
			return nil, err
		}
		return report, nil
	case "meshclaw_ops_brief":
		report, record, storeErr, briefErr := buildOpsBriefWithOptions(opsBriefOptions{Fast: boolArg(args, "fast", true)})
		if briefErr != nil {
			return nil, briefErr
		}
		return map[string]interface{}{"report": report, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_ops_control":
		report, record, storeErr, controlErr := buildOpsControl(boolArg(args, "apply_safe", false))
		if controlErr != nil {
			return nil, controlErr
		}
		return slimOpsControlMCP(report, record, storeErr), nil
	case "meshclaw_metrics_cpu":
		client, err := prometheus.NewClient()
		if err != nil {
			return nil, err
		}
		resp, err := client.CPU(stringArg(args, "server"))
		if err != nil {
			return nil, err
		}
		return resp, nil
	case "meshclaw_metrics_memory":
		client, err := prometheus.NewClient()
		if err != nil {
			return nil, err
		}
		resp, err := client.Memory(stringArg(args, "server"))
		if err != nil {
			return nil, err
		}
		return resp, nil
	case "meshclaw_metrics_disk":
		client, err := prometheus.NewClient()
		if err != nil {
			return nil, err
		}
		resp, err := client.Disk(stringArg(args, "server"))
		if err != nil {
			return nil, err
		}
		return resp, nil
	case "meshclaw_metrics_load":
		client, err := prometheus.NewClient()
		if err != nil {
			return nil, err
		}
		resp, err := client.Load(stringArg(args, "server"))
		if err != nil {
			return nil, err
		}
		return resp, nil
	case "meshclaw_metrics_all":
		client, err := prometheus.NewClient()
		if err != nil {
			return nil, err
		}
		results, err := client.All(stringArg(args, "server"))
		if err != nil {
			return nil, err
		}
		return results, nil
	case "meshclaw_metrics_query":
		client, err := prometheus.NewClient()
		if err != nil {
			return nil, err
		}
		resp, err := client.CustomQuery(stringArg(args, "query"), stringArg(args, "server"), floatArg(args, "threshold", 0))
		if err != nil {
			return nil, err
		}
		return resp, nil
	case "meshclaw_util_crypto_price":
		return utilCryptoPrice(stringArg(args, "coin"))
	case "meshclaw_util_weather":
		return utilWeather(stringArg(args, "city"))
	case "meshclaw_util_convert":
		return utilConvert(floatArg(args, "value", 0), stringArg(args, "from"), stringArg(args, "to"))
	case "meshclaw_util_generate":
		return utilGenerate(stringArg(args, "kind"), stringArg(args, "input"), intArg(args, "length", 16))
	case "meshclaw_node_inventory":
		host := stringArg(args, "host")
		report := workflow.SoftwareInventory(host)
		record, err := evidence.Store("node-inventory", host, report.Status, report)
		return map[string]interface{}{"report": report, "evidence": record, "store_error": errString(err)}, nil
	case "meshclaw_fleet_inventory":
		report, record, storeErr := runFleetInventory(splitCSV(stringArg(args, "hosts")), intArg(args, "max_parallel", 4))
		return map[string]interface{}{"report": report, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_placement_plan":
		report, record, storeErr, planErr := buildPlacementPlan(stringArg(args, "workload"))
		if planErr != nil {
			return nil, planErr
		}
		return map[string]interface{}{"report": report, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_service_registry_plan":
		report := buildServiceRegistryPlan(args)
		record, storeErr := evidence.Store("service-registry-plan", stringArg(args, "service"), stringMapValue(report, "summary"), report)
		return map[string]interface{}{"report": report, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_capacity_scale_plan":
		report := buildCapacityScalePlan(args)
		record, storeErr := evidence.Store("capacity-scale-plan", stringArg(args, "workload"), stringMapValue(report, "summary"), report)
		return map[string]interface{}{"report": report, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_storage_guardrail_plan":
		report := buildStorageGuardrailPlan(args)
		record, storeErr := evidence.Store("storage-guardrail-plan", stringArg(args, "node"), stringMapValue(report, "summary"), report)
		return map[string]interface{}{"report": report, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_ops_integration_plan":
		report := buildOpsIntegrationPlan(args)
		record, storeErr := evidence.Store("ops-integration-plan", stringArg(args, "tools"), stringMapValue(report, "summary"), report)
		return map[string]interface{}{"report": report, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_mcp_rollout_plan":
		report := buildMCPRolloutPlan(args)
		record, storeErr := evidence.Store("mcp-rollout-plan", stringArg(args, "client"), stringMapValue(report, "summary"), report)
		return map[string]interface{}{"report": report, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_mcp_smoke_test_plan":
		report := buildMCPSmokeTestPlan(args)
		record, storeErr := evidence.Store("mcp-smoke-test-plan", stringArg(args, "client"), stringMapValue(report, "summary"), report)
		return map[string]interface{}{"report": report, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_mcp_profile_visibility_check":
		if boolArg(args, "apply", false) || boolArg(args, "execute", false) {
			return nil, fmt.Errorf("meshclaw_mcp_profile_visibility_check is read-only; execution is not implemented")
		}
		report := buildMCPProfileVisibilityCheck(args)
		record, storeErr := evidence.Store("mcp-profile-visibility-check", stringMapValue(report, "profile"), stringMapValue(report, "summary"), report)
		return map[string]interface{}{"report": report, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_automation_rule_plan":
		report := buildAutomationRulePlan(args)
		record, storeErr := evidence.Store("automation-rule-plan", stringArg(args, "name"), stringMapValue(report, "summary"), report)
		return map[string]interface{}{"report": report, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_automation_rule_check":
		if boolArg(args, "apply", false) || boolArg(args, "execute", false) {
			return nil, fmt.Errorf("meshclaw_automation_rule_check validates only; execution is not implemented")
		}
		report, err := buildAutomationRuleCheck(args)
		if err != nil {
			return nil, err
		}
		record, storeErr := evidence.Store("automation-rule-check", stringArg(args, "approved_by"), stringMapValue(report, "summary"), report)
		return map[string]interface{}{"report": report, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_automation_rule_readiness_summary":
		if boolArg(args, "apply", false) || boolArg(args, "execute", false) {
			return nil, fmt.Errorf("meshclaw_automation_rule_readiness_summary summarizes readiness only; execution is not implemented")
		}
		report, err := buildAutomationRuleReadinessSummary(args)
		if err != nil {
			return nil, err
		}
		record, storeErr := evidence.Store("automation-rule-readiness-summary", "automation", stringMapValue(report, "summary"), report)
		return map[string]interface{}{"readiness_summary": report, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_automation_rule_writer_plan":
		if boolArg(args, "apply", false) || boolArg(args, "execute", false) {
			return nil, fmt.Errorf("meshclaw_automation_rule_writer_plan previews rule writing only; execution is not implemented")
		}
		report, err := buildAutomationRuleWriterPlan(args)
		if err != nil {
			return nil, err
		}
		record, storeErr := evidence.Store("automation-rule-writer-plan", "automation", stringMapValue(report, "summary"), report)
		return map[string]interface{}{"writer_plan": report, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_orchestration_plan":
		scenario := stringArg(args, "scenario")
		report := buildOrchestrationPlan(scenario)
		record, storeErr := evidence.Store("orchestration-plan", "workflow", scenario, report)
		return map[string]interface{}{"report": report, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_fleet_scan":
		decision := policy.Evaluate(policy.Request{Subject: "mcp", Action: "fleet_scan", Resource: "server"})
		if decision.Decision != policy.Allow {
			return map[string]interface{}{"policy": decision, "success": false}, nil
		}
		opts := fleet.Options{
			Hosts:       splitCSV(stringArg(args, "hosts")),
			Security:    boolArg(args, "security", true),
			Hygiene:     boolArg(args, "hygiene", true),
			Logs:        boolArg(args, "logs", true),
			MaxParallel: intArg(args, "max_parallel", 3),
		}
		report, scanErr := fleet.Scan(opts)
		if scanErr != nil {
			return nil, scanErr
		}
		runtimeReport := buildFleetScanRuntimeReport(report)
		record, storeErr := evidence.Store("fleet-scan", "fleet", fleetScanSummary(report), runtimeReport)
		return map[string]interface{}{"policy": decision, "report": report, "recommended_mcp": runtimeReport.RecommendedMCP, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_fleet_service_audit":
		report, record, storeErr := runFleetServiceAudit(splitCSV(stringArg(args, "hosts")), intArg(args, "max_parallel", 4))
		return map[string]interface{}{"report": report, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_vssh_daemon_audit":
		report, storeErr, auditErr := buildVSSHDaemonAudit(splitCSV(stringArg(args, "hosts")))
		if auditErr != nil {
			return nil, auditErr
		}
		report.StoreErr = errString(storeErr)
		return slimVSSHDaemonAuditMCP(report), nil
	case "meshclaw_vssh_auth_paths":
		return buildVSSHAuthPaths(splitCSV(stringArg(args, "hosts"))), nil
	case "meshclaw_node_repair_plan":
		report, storeErr, planErr := buildNodeRepairPlan(splitCSV(stringArg(args, "hosts")))
		if planErr != nil {
			return nil, planErr
		}
		report.StoreErr = errString(storeErr)
		return report, nil
	case "meshclaw_service_triage":
		report, record, storeErr := buildServiceTriage(splitCSV(stringArg(args, "hosts")), intArg(args, "limit", 5), intArg(args, "max_parallel", 4))
		return slimServiceTriageMCP(report, record, storeErr), nil
	case "meshclaw_reconcile_plan":
		if boolArg(args, "apply", false) || boolArg(args, "execute", false) {
			return nil, fmt.Errorf("meshclaw_reconcile_plan is dry-run only; apply/execute is intentionally not implemented")
		}
		return buildMCPReconcilePlan(args, "meshclaw_reconcile_plan", "reconcile-plan")
	case "meshclaw_reconcile_validate_desired":
		if boolArg(args, "apply", false) || boolArg(args, "execute", false) {
			return nil, fmt.Errorf("meshclaw_reconcile_validate_desired validates YAML only; execution is not implemented")
		}
		return buildMCPReconcileDesiredValidation(args)
	case "meshclaw_reconcile_approval_request":
		if boolArg(args, "apply", false) || boolArg(args, "execute", false) {
			return nil, fmt.Errorf("meshclaw_reconcile_approval_request does not execute changes")
		}
		return buildMCPReconcileApprovalRequest(args)
	case "meshclaw_reconcile_apply_gate":
		if boolArg(args, "apply", false) || boolArg(args, "execute", false) {
			return nil, fmt.Errorf("meshclaw_reconcile_apply_gate validates gates only; execution is not implemented")
		}
		return buildMCPReconcileApplyGate(args)
	case "meshclaw_reconcile_apply_plan":
		if boolArg(args, "apply", false) || boolArg(args, "execute", false) {
			return nil, fmt.Errorf("meshclaw_reconcile_apply_plan builds a plan only; execution is not implemented")
		}
		return buildMCPReconcileApplyPlan(args)
	case "meshclaw_reconcile_execution_preview":
		if boolArg(args, "apply", false) || boolArg(args, "execute", false) {
			return nil, fmt.Errorf("meshclaw_reconcile_execution_preview renders previews only; execution is not implemented")
		}
		return buildMCPReconcileExecutionPreview(args)
	case "meshclaw_reconcile_verification_plan":
		if boolArg(args, "apply", false) || boolArg(args, "execute", false) {
			return nil, fmt.Errorf("meshclaw_reconcile_verification_plan builds verification requirements only; execution is not implemented")
		}
		return buildMCPReconcileVerificationPlan(args)
	case "meshclaw_reconcile_runbook":
		if boolArg(args, "apply", false) || boolArg(args, "execute", false) {
			return nil, fmt.Errorf("meshclaw_reconcile_runbook is review-only; execution is not implemented")
		}
		return buildMCPReconcileRunbook(args)
	case "meshclaw_reconcile_runbook_check":
		if boolArg(args, "apply", false) || boolArg(args, "execute", false) {
			return nil, fmt.Errorf("meshclaw_reconcile_runbook_check validates runbooks only; execution is not implemented")
		}
		return buildMCPReconcileRunbookCheck(args)
	case "meshclaw_reconcile_rollback_plan":
		if boolArg(args, "apply", false) || boolArg(args, "execute", false) {
			return nil, fmt.Errorf("meshclaw_reconcile_rollback_plan builds rollback guidance only; execution is not implemented")
		}
		return buildMCPReconcileRollbackPlan(args)
	case "meshclaw_reconcile_completion_plan":
		if boolArg(args, "apply", false) || boolArg(args, "execute", false) {
			return nil, fmt.Errorf("meshclaw_reconcile_completion_plan builds completion requirements only; execution is not implemented")
		}
		return buildMCPReconcileCompletionPlan(args)
	case "meshclaw_reconcile_readiness_summary":
		if boolArg(args, "apply", false) || boolArg(args, "execute", false) {
			return nil, fmt.Errorf("meshclaw_reconcile_readiness_summary summarizes readiness only; execution is not implemented")
		}
		return buildMCPReconcileReadinessSummary(args)
	case "meshclaw_reconcile_run_once":
		if boolArg(args, "execute", false) || boolArg(args, "apply", false) {
			return nil, fmt.Errorf("meshclaw_reconcile_run_once execute/apply is intentionally not implemented")
		}
		if !boolArg(args, "dry_run", false) {
			return nil, fmt.Errorf("meshclaw_reconcile_run_once requires dry_run=true")
		}
		return buildMCPReconcilePlan(args, "meshclaw_reconcile_run_once", "reconcile-run-once")
	case "meshclaw_autoheal_plan":
		timeout := autohealPlanTimeout()
		in, timedOut := collectAutohealPlanInputs(timeout)
		if timedOut {
			report := degradedAutohealPlanReport(timeout)
			record, storeErr := evidence.Store("autoheal-plan", "fleet", fmt.Sprintf("status=partial timeout=%s", timeout), report)
			return map[string]interface{}{"actions": report.Actions, "recommended_mcp": report.RecommendedMCP, "report": report, "status": report.Status, "evidence": record, "store_error": errString(storeErr)}, nil
		}
		if in.err != nil {
			return nil, in.err
		}
		actions := buildAutohealPlan(in.monitor, in.serviceTriage)
		report := buildAutohealPlanReport(actions)
		record, storeErr := evidence.Store("autoheal-plan", "fleet", fmt.Sprintf("actions=%d", len(actions)), report)
		if storeErr == nil {
			storeErr = in.serviceStoreErr
		}
		return map[string]interface{}{"actions": actions, "recommended_mcp": report.RecommendedMCP, "report": report, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_autoheal_container_apply_plan":
		if boolArg(args, "apply", false) || boolArg(args, "execute", false) {
			return nil, fmt.Errorf("meshclaw_autoheal_container_apply_plan builds a plan only; execution is not implemented")
		}
		planPath := stringArg(args, "plan_evidence_path")
		record, err := evidence.Load(planPath)
		if err != nil {
			return nil, err
		}
		plan, err := autohealPlanFromEvidence(record)
		if err != nil {
			return nil, err
		}
		report := buildContainerApplyPlanReport(planPath, record, plan, stringArg(args, "approved_by"))
		applyRecord, storeErr := evidence.Store("container-apply-plan", "fleet", fmt.Sprintf("ready=%t steps=%d", report.Ready, len(report.Steps)), report)
		return map[string]interface{}{"container_apply_plan": report, "container_apply_plan_contract": containerApplyPlanMCPContract(report), "evidence": applyRecord, "store_error": errString(storeErr)}, nil
	case "meshclaw_autoheal_container_verification_plan":
		if boolArg(args, "apply", false) || boolArg(args, "execute", false) {
			return nil, fmt.Errorf("meshclaw_autoheal_container_verification_plan builds verification requirements only; execution is not implemented")
		}
		applyPath := stringArg(args, "container_apply_plan_evidence_path")
		record, err := evidence.Load(applyPath)
		if err != nil {
			return nil, err
		}
		applyPlan, err := containerApplyPlanFromEvidence(record)
		if err != nil {
			return nil, err
		}
		report := buildContainerVerificationPlanReport(applyPath, record, applyPlan)
		verifyRecord, storeErr := evidence.Store("container-verification-plan", "fleet", fmt.Sprintf("ready=%t checks=%d", report.Ready, len(report.Checks)), report)
		return map[string]interface{}{
			"container_verification_plan":          report,
			"container_verification_plan_contract": containerVerificationPlanMCPContract(report),
			"evidence":                             verifyRecord,
			"store_error":                          errString(storeErr),
		}, nil
	case "meshclaw_autoheal_container_runbook":
		if boolArg(args, "apply", false) || boolArg(args, "execute", false) {
			return nil, fmt.Errorf("meshclaw_autoheal_container_runbook is review-only; execution is not implemented")
		}
		verifyPath := stringArg(args, "container_verification_plan_evidence_path")
		record, err := evidence.Load(verifyPath)
		if err != nil {
			return nil, err
		}
		verification, err := containerVerificationPlanFromEvidence(record)
		if err != nil {
			return nil, err
		}
		report := buildContainerRunbookReport(verifyPath, record, verification)
		runbookRecord, storeErr := evidence.Store("container-runbook", "fleet", fmt.Sprintf("ready=%t steps=%d", report.Ready, len(report.Steps)), report)
		return map[string]interface{}{
			"container_runbook":          report,
			"container_runbook_contract": containerRunbookMCPContract(report),
			"evidence":                   runbookRecord,
			"store_error":                errString(storeErr),
		}, nil
	case "meshclaw_autoheal_container_runbook_check":
		if boolArg(args, "apply", false) || boolArg(args, "execute", false) {
			return nil, fmt.Errorf("meshclaw_autoheal_container_runbook_check validates runbooks only; execution is not implemented")
		}
		runbookPath := stringArg(args, "container_runbook_evidence_path")
		record, err := evidence.Load(runbookPath)
		if err != nil {
			return nil, err
		}
		runbook, err := containerRunbookFromEvidence(record)
		if err != nil {
			return nil, err
		}
		report := buildContainerRunbookCheckReport(runbookPath, record, runbook)
		checkRecord, storeErr := evidence.Store("container-runbook-check", "fleet", fmt.Sprintf("ready=%t findings=%d", report.Ready, len(report.Findings)), report)
		return map[string]interface{}{
			"container_runbook_check":          report,
			"container_runbook_check_contract": containerRunbookCheckMCPContract(report),
			"evidence":                         checkRecord,
			"store_error":                      errString(storeErr),
		}, nil
	case "meshclaw_autoheal_container_rollback_plan":
		if boolArg(args, "apply", false) || boolArg(args, "execute", false) {
			return nil, fmt.Errorf("meshclaw_autoheal_container_rollback_plan builds rollback guidance only; execution is not implemented")
		}
		checkPath := stringArg(args, "container_runbook_check_evidence_path")
		record, err := evidence.Load(checkPath)
		if err != nil {
			return nil, err
		}
		check, err := containerRunbookCheckFromEvidence(record)
		if err != nil {
			return nil, err
		}
		report := buildContainerRollbackPlanReport(checkPath, record, check)
		rollbackRecord, storeErr := evidence.Store("container-rollback-plan", "fleet", fmt.Sprintf("ready=%t steps=%d", report.Ready, len(report.Steps)), report)
		return map[string]interface{}{
			"container_rollback_plan":          report,
			"container_rollback_plan_contract": containerRollbackPlanMCPContract(report),
			"evidence":                         rollbackRecord,
			"store_error":                      errString(storeErr),
		}, nil
	case "meshclaw_autoheal_container_completion_plan":
		if boolArg(args, "apply", false) || boolArg(args, "execute", false) {
			return nil, fmt.Errorf("meshclaw_autoheal_container_completion_plan builds completion requirements only; execution is not implemented")
		}
		rollbackPath := stringArg(args, "container_rollback_plan_evidence_path")
		record, err := evidence.Load(rollbackPath)
		if err != nil {
			return nil, err
		}
		rollback, err := containerRollbackPlanFromEvidence(record)
		if err != nil {
			return nil, err
		}
		report := buildContainerCompletionPlanReport(rollbackPath, record, rollback)
		completionRecord, storeErr := evidence.Store("container-completion-plan", "fleet", fmt.Sprintf("ready=%t requirements=%d", report.Ready, len(report.Requirements)), report)
		return map[string]interface{}{
			"container_completion_plan":          report,
			"container_completion_plan_contract": containerCompletionPlanMCPContract(report),
			"evidence":                           completionRecord,
			"store_error":                        errString(storeErr),
		}, nil
	case "meshclaw_autoheal_container_readiness_summary":
		if boolArg(args, "apply", false) || boolArg(args, "execute", false) {
			return nil, fmt.Errorf("meshclaw_autoheal_container_readiness_summary summarizes readiness only; execution is not implemented")
		}
		completionPath := stringArg(args, "container_completion_plan_evidence_path")
		record, err := evidence.Load(completionPath)
		if err != nil {
			return nil, err
		}
		completion, err := containerCompletionPlanFromEvidence(record)
		if err != nil {
			return nil, err
		}
		report := buildContainerReadinessSummaryReport(completionPath, record, completion)
		summaryRecord, storeErr := evidence.Store("container-readiness-summary", "fleet", fmt.Sprintf("ready=%t stages=%d blockers=%d", report.Ready, len(report.ReadyStages), len(report.Blockers)), report)
		return map[string]interface{}{
			"container_readiness_summary":          report,
			"container_readiness_summary_contract": containerReadinessSummaryMCPContract(report),
			"readiness_contract":                   readinessMCPContract("container_readiness_contract", report.StopBefore),
			"evidence":                             summaryRecord,
			"store_error":                          errString(storeErr),
		}, nil
	case "meshclaw_autoheal_container_executor_gate":
		if boolArg(args, "apply", false) {
			return nil, fmt.Errorf("meshclaw_autoheal_container_executor_gate is admission-only and cannot apply changes")
		}
		readinessPath := stringArg(args, "container_readiness_summary_evidence_path")
		record, err := evidence.Load(readinessPath)
		if err != nil {
			return nil, err
		}
		summary, err := containerReadinessSummaryFromEvidence(record)
		if err != nil {
			return nil, err
		}
		report := buildContainerExecutorGateReport(readinessPath, record, summary, stringArg(args, "approved_by"), boolArg(args, "dry_run", true), boolArg(args, "execute", false))
		gateRecord, storeErr := evidence.Store("container-executor-gate", "fleet", fmt.Sprintf("ready=%t checks=%d blockers=%d", report.Ready, len(report.Checks), len(report.Blockers)), report)
		return map[string]interface{}{
			"container_executor_gate":          report,
			"container_executor_gate_contract": containerExecutorGateMCPContract(report),
			"evidence":                         gateRecord,
			"store_error":                      errString(storeErr),
		}, nil
	case "meshclaw_autoheal_container_executor":
		if boolArg(args, "apply", false) {
			return nil, fmt.Errorf("meshclaw_autoheal_container_executor does not accept apply=true; use execute=true with the live approval phrase")
		}
		gatePath := stringArg(args, "container_executor_gate_evidence_path")
		record, err := evidence.Load(gatePath)
		if err != nil {
			return nil, err
		}
		gate, err := containerExecutorGateFromEvidence(record)
		if err != nil {
			return nil, err
		}
		report := buildContainerExecutorReport(gatePath, record, gate, stringArg(args, "approved_by"), boolArg(args, "dry_run", true), boolArg(args, "execute", false), stringArg(args, "live_approval_phrase"))
		report = executeContainerExecutorReport(report)
		executorRecord, storeErr := evidence.Store("container-executor", "fleet", fmt.Sprintf("status=%s dry_run=%t executed=%t steps=%d blockers=%d", report.Status, report.DryRun, report.Executed, len(report.Steps), len(report.Blockers)), report)
		return map[string]interface{}{
			"container_executor":          report,
			"container_executor_contract": containerExecutorMCPContract(report),
			"evidence":                    executorRecord,
			"store_error":                 errString(storeErr),
		}, nil
	case "meshclaw_autoheal_container_executor_verify":
		if boolArg(args, "apply", false) || boolArg(args, "execute", false) {
			return nil, fmt.Errorf("meshclaw_autoheal_container_executor_verify is gate-only and never executes commands")
		}
		executorPath := stringArg(args, "container_executor_evidence_path")
		record, err := evidence.Load(executorPath)
		if err != nil {
			return nil, err
		}
		executor, err := containerExecutorFromEvidence(record)
		if err != nil {
			return nil, err
		}
		report := buildContainerExecutorVerificationReport(executorPath, record, executor, splitCSV(stringArg(args, "agent_evidence_paths")), splitCSV(stringArg(args, "container_logscan_evidence_paths")))
		verifyRecord, storeErr := evidence.Store("container-executor-verification", "fleet", fmt.Sprintf("ready=%t checks=%d blockers=%d", report.Ready, len(report.Checks), len(report.Blockers)), report)
		return map[string]interface{}{
			"container_executor_verification":          report,
			"container_executor_verification_contract": containerExecutorVerificationMCPContract(report),
			"evidence":    verifyRecord,
			"store_error": errString(storeErr),
		}, nil
	case "meshclaw_autoheal_apply_safe":
		m, err := monitor.New(monitor.DefaultConfig())
		if err != nil {
			return nil, err
		}
		m.CheckAll()
		serviceTriage, _, serviceStoreErr := buildServiceTriage(nil, 5, 4)
		plan := buildAutohealPlan(m, serviceTriage)
		actions := applyAutohealPlanSafe(plan)
		counts := summarizeAutohealApplySafeActions(actions)
		record, storeErr := evidence.Store("autoheal-apply-safe", "fleet", autohealApplySafeCountsLine(counts), actions)
		if storeErr == nil {
			storeErr = serviceStoreErr
		}
		return autohealApplySafeMCPPayload(actions, counts, record, storeErr), nil
	case "meshclaw_workflow_list":
		return map[string]interface{}{
			"workflows":         runtimeflow.WorkflowInfos(),
			"default_workflow":  "fleet-health-demo",
			"default_example":   "examples/workflows/fleet-health.json",
			"advanced_workflow": "meshclaw-ops-orchestration-demo",
			"run_tool":          "meshclaw_workflow_run",
			"scaffold_tool":     "meshclaw_workflow_scaffold",
			"validate_tool":     "meshclaw_workflow_validate",
			"inspect_tool":      "meshclaw_workflow_inspect",
			"plan_execute_tool": "meshclaw_workflow_plan_execute",
			"evidence_tool":     "meshclaw_evidence_latest",
			"readonly_execute":  "fleet-readonly-execute-demo",
			"recommended_start": []map[string]interface{}{
				{"tool": "meshclaw_workflow_validate", "arguments": map[string]interface{}{"name": "fleet-health-demo"}},
				{"tool": "meshclaw_workflow_run", "arguments": map[string]interface{}{"name": "fleet-health-demo", "mode": "dry-run"}},
				{"tool": "meshclaw_workflow_run", "arguments": map[string]interface{}{"name": "fleet-readonly-execute-demo", "mode": "execute"}},
			},
			"new_workflow_start":   map[string]interface{}{"tool": "meshclaw_workflow_scaffold", "arguments": map[string]interface{}{"name": "my-ops-workflow"}},
			"advanced_private_run": map[string]interface{}{"tool": "meshclaw_workflow_run", "arguments": map[string]interface{}{"name": "meshclaw-ops-orchestration-demo", "mode": "dry-run"}},
			"guidance":             "Start with fleet-health-demo dry-run, then use fleet-readonly-execute-demo execute for real bounded server evidence without mutation. For user-authored workflows, validate first, dry-run, then execute only after approval gates are resolved.",
			"guidance_ko":          "처음에는 fleet-health-demo dry-run으로 시작하고, 실제 제한된 서버 증거는 fleet-readonly-execute-demo execute로 확인하세요. 사용자 workflow는 validate, dry-run, 승인 게이트 확인 후 execute 순서로 실행하세요.",
		}, nil
	case "meshclaw_workflow_scaffold":
		mcpArgs := []string{stringArg(args, "name")}
		if output := stringArg(args, "output"); output != "" {
			mcpArgs = append(mcpArgs, "--output", output)
		}
		if boolArg(args, "force", false) {
			mcpArgs = append(mcpArgs, "--force")
		}
		return scaffoldWorkflow(mcpArgs, true)
	case "meshclaw_workflow_validate":
		return runtimeflow.Validate(stringArg(args, "name"))
	case "meshclaw_workflow_inspect":
		name := stringArg(args, "name")
		if !runtimeflow.IsKnown(name) {
			return nil, fmt.Errorf("unknown workflow: %s", name)
		}
		return runtimeflow.Inspect(name)
	case "meshclaw_workflow_resume":
		ref := stringArg(args, "ref")
		if ref == "" {
			ref = "latest"
		}
		return runtimeflow.Resume(ref)
	case "meshclaw_workflow_plan_execute":
		ref := stringArg(args, "ref")
		if ref == "" {
			ref = "latest"
		}
		return runtimeflow.PlanExecute(ref)
	case "meshclaw_approvals_list":
		ref := stringArg(args, "ref")
		if ref == "" {
			ref = "latest"
		}
		records, err := runtimeflow.ListApprovals(ref)
		return map[string]interface{}{"approvals": records}, err
	case "meshclaw_approvals_grant":
		ref := stringArg(args, "ref")
		if ref == "" {
			ref = "latest"
		}
		source := stringArg(args, "source")
		if source == "" {
			source = "mcp"
		}
		record, err := runtimeflow.GrantApproval(ref, stringArg(args, "step"), stringArg(args, "actor"), stringArg(args, "reason"), source)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{
			"kind":    "meshclaw_approval_granted",
			"record":  record,
			"next":    "Run meshclaw_workflow_resume to confirm the step is approved_ready, then run meshclaw_workflow_run with mode=execute and approvals_ref=latest.",
			"execute": map[string]interface{}{"tool": "meshclaw_workflow_run", "arguments": map[string]interface{}{"name": record.Workflow, "mode": string(runtimeflow.Execute), "approvals_ref": "latest", "steps": record.Step}},
		}, nil
	case "meshclaw_messenger_report":
		report, err := messenger.BuildReport(messenger.ReportOptions{
			Ref:      firstNonEmpty(stringArg(args, "ref"), "latest"),
			Channel:  firstNonEmpty(stringArg(args, "channel"), "signal"),
			Audience: firstNonEmpty(stringArg(args, "audience"), "owner"),
		})
		if err != nil {
			return nil, err
		}
		jsonPath, writeErr := messenger.WriteReport(report)
		return map[string]interface{}{"report": report, "report_path": report.Evidence.Report, "report_json_path": jsonPath, "write_error": errString(writeErr)}, nil
	case "meshclaw_messenger_approval_request":
		req, err := messenger.BuildApprovalRequest(
			firstNonEmpty(stringArg(args, "ref"), "latest"),
			stringArg(args, "step"),
			firstNonEmpty(stringArg(args, "channel"), "signal"),
			firstNonEmpty(stringArg(args, "audience"), "owner"),
		)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"approval_request": req}, nil
	case "meshclaw_messenger_targets":
		return messenger.ListTargets()
	case "meshclaw_signal_rooms_doctor":
		pause, resume := pauseSignalDispatcherForRoomCheck(boolArg(args, "auto_pause_dispatcher", false))
		result, err := messenger.RoomsDoctor(messenger.RoomDoctorOptions{Timeout: time.Duration(intArg(args, "timeout", 15)) * time.Second})
		resume()
		result.Dispatcher = pause
		record, storeErr := evidence.Store("signal-rooms-doctor", "signal", "rooms", result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr), "error": errString(err)}, err
	case "meshclaw_signal_rooms_discover":
		pause, resume := pauseSignalDispatcherForRoomCheck(boolArg(args, "auto_pause_dispatcher", false))
		result, err := messenger.RoomsDoctor(messenger.RoomDoctorOptions{Timeout: time.Duration(intArg(args, "timeout", 15)) * time.Second})
		resume()
		result.Dispatcher = pause
		unbound := []messenger.RoomStatus{}
		for _, status := range result.Rooms {
			if status.Class == "orphan_member" {
				unbound = append(unbound, status)
			}
		}
		payload := map[string]interface{}{"kind": "meshclaw_signal_rooms_discover", "unbound_rooms": unbound, "warnings": result.Warnings, "next_actions": result.NextActions, "dispatcher_pause": pause}
		record, storeErr := evidence.Store("signal-rooms-discover", "signal", fmt.Sprintf("unbound=%d", len(unbound)), payload)
		return map[string]interface{}{"result": payload, "evidence": record, "store_error": errString(storeErr), "error": errString(err)}, err
	case "meshclaw_signal_room_bind":
		mode := stringArg(args, "mode")
		if mode == "auto" {
			mode = ""
		}
		pause, resume := pauseSignalDispatcherForRoomCheck(boolArg(args, "auto_pause_dispatcher", false))
		result, err := messenger.BindRoom(messenger.RoomBindOptions{
			Room:     stringArg(args, "room"),
			TargetID: stringArg(args, "target"),
			Mode:     mode,
			Label:    stringArg(args, "label"),
			Model:    stringArg(args, "model"),
			BaseURL:  stringArg(args, "base_url"),
			Timeout:  time.Duration(intArg(args, "timeout", 15)) * time.Second,
		})
		resume()
		record, storeErr := evidence.Store("signal-room-bind", "signal", result.Target.ID, map[string]interface{}{"result": result, "dispatcher_pause": pause})
		return map[string]interface{}{"result": result, "dispatcher_pause": pause, "evidence": record, "store_error": errString(storeErr), "error": errString(err)}, err
	case "meshclaw_signal_rooms_cleanup":
		pause, resume := pauseSignalDispatcherForRoomCheck(boolArg(args, "auto_pause_dispatcher", false))
		result, err := messenger.RoomsCleanup(messenger.RoomCleanupOptions{
			Timeout: time.Duration(intArg(args, "timeout", 15)) * time.Second,
			Execute: boolArg(args, "execute", false),
			Approve: boolArg(args, "approve", false),
		})
		resume()
		result.Dispatcher = pause
		record, storeErr := evidence.Store("signal-rooms-cleanup", "signal", result.Mode, result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr), "error": errString(err)}, err
	case "meshclaw_messenger_target_add":
		store, target, err := messenger.UpsertTarget(messenger.Target{
			ID:        stringArg(args, "id"),
			Channel:   firstNonEmpty(stringArg(args, "channel"), "signal"),
			Recipient: stringArg(args, "recipient"),
			GroupID:   stringArg(args, "group_id"),
			Label:     stringArg(args, "label"),
			Mode:      stringArg(args, "mode"),
			Model:     stringArg(args, "model"),
			BaseURL:   stringArg(args, "base_url"),
		})
		if err != nil {
			return nil, err
		}
		record, storeErr := evidence.Store("messenger-target-add", target.Channel, target.ID, target)
		return map[string]interface{}{"target": target, "store": store, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_messenger_send_report":
		result, err := messenger.Send(messenger.SendOptions{
			TargetID: stringArg(args, "target"),
			Ref:      firstNonEmpty(stringArg(args, "ref"), "latest"),
			Kind:     "report",
			Execute:  boolArg(args, "execute", false),
		})
		record, storeErr := evidence.Store("messenger-send-report", result.Target.Channel, result.Target.ID, result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr), "error": errString(err)}, err
	case "meshclaw_messenger_send_approval_request":
		result, err := messenger.Send(messenger.SendOptions{
			TargetID: stringArg(args, "target"),
			Ref:      firstNonEmpty(stringArg(args, "ref"), "latest"),
			Step:     stringArg(args, "step"),
			Kind:     "approval-request",
			Execute:  boolArg(args, "execute", false),
		})
		record, storeErr := evidence.Store("messenger-send-approval-request", result.Target.Channel, result.Target.ID, result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr), "error": errString(err)}, err
	case "meshclaw_scheduled_delivery_plan":
		plan := messenger.PlanScheduledDelivery(messenger.ScheduledDeliveryPlanOptions{
			Target:      stringArg(args, "target"),
			Schedule:    stringArg(args, "schedule"),
			Content:     stringArg(args, "content"),
			ContentType: stringArg(args, "content_type"),
			Delivery:    stringArg(args, "delivery"),
			Execute:     boolArg(args, "execute", false),
			Approve:     boolArg(args, "approve", false),
			Now:         time.Now().UTC(),
		})
		record, storeErr := evidence.Store("scheduled-delivery-plan", "messenger", firstNonEmpty(plan.Target, "missing-target"), plan)
		return map[string]interface{}{"plan": plan, "evidence": evidenceRef(record), "store_error": errString(storeErr)}, nil
	case "meshclaw_scheduled_delivery_apply":
		result, err := messenger.ApplyScheduledDelivery(messenger.ScheduledDeliveryApplyOptions{
			Target:          stringArg(args, "target"),
			Schedule:        stringArg(args, "schedule"),
			Content:         stringArg(args, "content"),
			ContentType:     stringArg(args, "content_type"),
			Delivery:        stringArg(args, "delivery"),
			Execute:         boolArg(args, "execute", false),
			Approve:         boolArg(args, "approve", false),
			PreviewApproved: boolArg(args, "preview_approved", false),
			Now:             time.Now().UTC(),
		})
		record, storeErr := evidence.Store("scheduled-delivery-apply", "messenger", firstNonEmpty(result.Plan.Target, "missing-target"), result)
		return map[string]interface{}{"result": result, "plan": result.Plan, "evidence": evidenceRef(record), "store_error": errString(storeErr), "error": errString(err)}, err
	case "meshclaw_browser_fetch":
		page, err := browserauto.Fetch(context.Background(), browserauto.FetchOptions{
			URL:     stringArg(args, "url"),
			MaxBody: intArg(args, "max_body", 12000),
			Timeout: intArg(args, "timeout", 20),
		})
		record, storeErr := evidence.Store("browser-fetch", "web", page.URL, page)
		return map[string]interface{}{"page": page, "evidence": record, "store_error": errString(storeErr), "error": errString(err)}, err
	case "meshclaw_browser_search":
		result, err := browserauto.Search(context.Background(), browserauto.SearchOptions{
			Query:   stringArg(args, "query"),
			Limit:   intArg(args, "limit", 8),
			Timeout: intArg(args, "timeout", 20),
		})
		record, storeErr := evidence.Store("browser-search", "web", result.Query, result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr), "error": errString(err)}, err
	case "meshclaw_visible_browser_search":
		query := stringArg(args, "query")
		if !boolArg(args, "execute", false) {
			plan := personalAssistantPlan("visible_browser_search", map[string]interface{}{
				"query":          query,
				"limit":          intArg(args, "limit", 5),
				"timeout":        intArg(args, "timeout", 20),
				"record_seconds": intArg(args, "record_seconds", 0),
			})
			record, storeErr := evidence.Store("visible-browser-search-plan", "browser", query, plan)
			return map[string]interface{}{"plan": plan, "evidence": record, "store_error": errString(storeErr)}, nil
		}
		result := osauto.OpenBrowserSearch(context.Background(), query)
		search, err := browserauto.Search(context.Background(), browserauto.SearchOptions{
			Query:   query,
			Limit:   intArg(args, "limit", 5),
			Timeout: intArg(args, "timeout", 20),
		})
		var recording *osauto.Result
		if seconds := intArg(args, "record_seconds", 0); seconds > 0 {
			output := stringArg(args, "output")
			if strings.TrimSpace(output) == "" {
				output = defaultMCPRecordingPath("visible-browser")
			}
			rec := osauto.ScreenRecord(context.Background(), seconds, output)
			recording = &rec
		}
		record, storeErr := evidence.Store("visible-browser-search", "browser", query, map[string]interface{}{"open": result, "search": search, "recording": recording, "error": errString(err)})
		return map[string]interface{}{"result": result, "search": search, "recording": recording, "evidence": record, "store_error": errString(storeErr), "error": errString(err), "scope_note": "Visible browser work ran on the local Runtime. Use target-specific publication tools for macmini document output."}, err
	case "meshclaw_media_play":
		query := stringArg(args, "query")
		source := stringArg(args, "source")
		plan := mediaPlayPlan(query, source)
		if !boolArg(args, "execute", false) {
			record, storeErr := evidence.Store("media-play-plan", "media", firstNonEmpty(query, source, "media"), plan)
			return map[string]interface{}{"plan": plan, "evidence": record, "store_error": errString(storeErr)}, nil
		}
		if !boolArg(args, "approve", false) {
			plan["approval_missing"] = true
			record, storeErr := evidence.Store("media-play-approval-required", "media", firstNonEmpty(query, source, "media"), plan)
			return map[string]interface{}{"plan": plan, "evidence": record, "store_error": errString(storeErr), "error": "approve=true is required with execute=true"}, nil
		}
		target := stringFromInterface(plan["target"])
		if target == "app:Music" {
			result := osauto.OpenApp(context.Background(), "Music")
			record, storeErr := evidence.Store(result.Action, "media", firstNonEmpty(query, "Music"), result)
			return map[string]interface{}{"result": result, "plan": plan, "evidence": record, "store_error": errString(storeErr)}, nil
		}
		result := osauto.OpenURL(context.Background(), target)
		record, storeErr := evidence.Store(result.Action, "media", firstNonEmpty(query, target), result)
		return map[string]interface{}{"result": result, "plan": plan, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_audio_transcribe":
		plan, err := audio.TranscribeLocalWhisper(
			time.Now().UTC(),
			stringArg(args, "path"),
			stringArg(args, "output"),
			stringArg(args, "model"),
			stringArg(args, "language"),
			stringArg(args, "task"),
			boolArg(args, "execute", false),
			boolArg(args, "approve", false),
		)
		record, storeErr := evidence.Store("audio-transcribe", "audio", firstNonEmpty(stringArg(args, "path"), "missing-path"), plan)
		result := map[string]interface{}{"plan": audioTranscribePlanMap(plan), "evidence": evidenceRef(record), "store_error": errString(storeErr)}
		if err != nil {
			result["error"] = errString(err)
		}
		return result, err
	case "meshclaw_app_settings_plan":
		app := stringArg(args, "app")
		pane := stringArg(args, "pane")
		plan := appSettingsPlan(app, pane, stringArg(args, "url"))
		if !boolArg(args, "execute", false) {
			record, storeErr := evidence.Store("app-settings-plan", "settings", firstNonEmpty(app, pane, "settings"), plan)
			return map[string]interface{}{"plan": plan, "evidence": record, "store_error": errString(storeErr)}, nil
		}
		if !boolArg(args, "approve", false) {
			plan["approval_missing"] = true
			record, storeErr := evidence.Store("app-settings-approval-required", "settings", firstNonEmpty(app, pane, "settings"), plan)
			return map[string]interface{}{"plan": plan, "evidence": record, "store_error": errString(storeErr), "error": "approve=true is required with execute=true"}, nil
		}
		target := stringFromInterface(plan["target"])
		if strings.HasPrefix(target, "app:") {
			result := osauto.OpenApp(context.Background(), strings.TrimPrefix(target, "app:"))
			record, storeErr := evidence.Store(result.Action, "settings", firstNonEmpty(app, target), result)
			return map[string]interface{}{"result": result, "plan": plan, "evidence": record, "store_error": errString(storeErr)}, nil
		}
		result := osauto.OpenURL(context.Background(), target)
		record, storeErr := evidence.Store(result.Action, "settings", firstNonEmpty(app, pane, target), result)
		return map[string]interface{}{"result": result, "plan": plan, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_account_action_plan":
		service := stringArg(args, "service")
		action := stringArg(args, "action")
		plan := accountActionPlan(service, action, stringArg(args, "url"))
		if !boolArg(args, "execute", false) {
			record, storeErr := evidence.Store("account-action-plan", "account", firstNonEmpty(service, action, "account"), plan)
			return map[string]interface{}{"plan": plan, "evidence": record, "store_error": errString(storeErr)}, nil
		}
		if !boolArg(args, "approve", false) {
			plan["approval_missing"] = true
			record, storeErr := evidence.Store("account-action-approval-required", "account", firstNonEmpty(service, action, "account"), plan)
			return map[string]interface{}{"plan": plan, "evidence": record, "store_error": errString(storeErr), "error": "approve=true is required with execute=true"}, nil
		}
		result := osauto.OpenURL(context.Background(), stringFromInterface(plan["target"]))
		record, storeErr := evidence.Store(result.Action, "account", firstNonEmpty(service, action, "account"), result)
		return map[string]interface{}{"result": result, "plan": plan, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_purchase_click":
		payload := runPurchaseClickMCP(args)
		record, storeErr := evidence.Store("purchase-click", "transactional-web", firstNonEmpty(stringArg(args, "merchant"), "purchase"), payload)
		return map[string]interface{}{"purchase": payload, "plan": payload, "evidence": evidenceRef(record), "store_error": errString(storeErr)}, nil
	case "meshclaw_document_create":
		if !boolArg(args, "execute", false) {
			plan := personalAssistantPlan("document_create", map[string]interface{}{
				"title": firstNonEmpty(stringArg(args, "title"), "Argos 문서"),
				"body":  stringArg(args, "body"),
			})
			record, storeErr := evidence.Store("document-create-plan", "document", firstNonEmpty(stringArg(args, "title"), "Argos 문서"), plan)
			return map[string]interface{}{"plan": plan, "evidence": record, "store_error": errString(storeErr)}, nil
		}
		result := osauto.CreateArgosDocument(context.Background(), stringArg(args, "title"), stringArg(args, "body"))
		record, storeErr := evidence.Store(result.Action, "document", firstNonEmpty(stringArg(args, "title"), "Argos 문서"), result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_document_export":
		if !boolArg(args, "execute", false) {
			plan := personalAssistantPlan("document_export", map[string]interface{}{"input": stringArg(args, "input"), "format": firstNonEmpty(stringArg(args, "format"), "docx"), "output": stringArg(args, "output")})
			record, storeErr := evidence.Store("document-export-plan", "document", stringArg(args, "input"), plan)
			return map[string]interface{}{"plan": plan, "evidence": record, "store_error": errString(storeErr)}, nil
		}
		result := osauto.ExportMarkdown(context.Background(), stringArg(args, "input"), stringArg(args, "format"), stringArg(args, "output"))
		record, storeErr := evidence.Store(result.Action, "document", stringArg(args, "input"), result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_spreadsheet_create":
		if !boolArg(args, "execute", false) {
			plan := personalAssistantPlan("spreadsheet_create", map[string]interface{}{
				"title": firstNonEmpty(stringArg(args, "title"), "Argos 표"),
				"body":  stringArg(args, "body"),
			})
			record, storeErr := evidence.Store("spreadsheet-create-plan", "spreadsheet", firstNonEmpty(stringArg(args, "title"), "Argos 표"), plan)
			return map[string]interface{}{"plan": plan, "evidence": record, "store_error": errString(storeErr)}, nil
		}
		result := osauto.CreateSpreadsheet(context.Background(), stringArg(args, "title"), stringArg(args, "body"))
		record, storeErr := evidence.Store(result.Action, "spreadsheet", firstNonEmpty(stringArg(args, "title"), "Argos 표"), result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_presentation_create":
		if !boolArg(args, "execute", false) {
			plan := personalAssistantPlan("presentation_create", map[string]interface{}{
				"title":       firstNonEmpty(stringArg(args, "title"), "Argos Presentation"),
				"body":        stringArg(args, "body"),
				"audience":    stringArg(args, "audience"),
				"slide_count": intArg(args, "slide_count", 6),
				"output":      stringArg(args, "output"),
			})
			record, storeErr := evidence.Store("presentation-create-plan", "document", firstNonEmpty(stringArg(args, "title"), "Argos Presentation"), plan)
			return map[string]interface{}{"plan": plan, "evidence": record, "store_error": errString(storeErr)}, nil
		}
		result := osauto.CreatePresentation(context.Background(), stringArg(args, "title"), stringArg(args, "body"), stringArg(args, "audience"), intArg(args, "slide_count", 6), stringArg(args, "output"))
		record, storeErr := evidence.Store(result.Action, "document", firstNonEmpty(stringArg(args, "title"), "Argos Presentation"), result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr), "validation": map[string]interface{}{"pptx": result.PPTX, "outline": result.Markdown, "ok": result.OK, "error": result.Error}}, nil
	case "meshclaw_presentation_verify":
		result := osauto.VerifyPresentation(context.Background(), stringArg(args, "path"))
		record, storeErr := evidence.Store(result.Action, "document", stringArg(args, "path"), result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr), "validation": map[string]interface{}{"pptx": result.PPTX, "ok": result.OK, "error": result.Error, "summary": result.Stdout}}, nil
	case "meshclaw_presentation_edit":
		if !boolArg(args, "execute", false) {
			plan := personalAssistantPlan("presentation_edit", map[string]interface{}{"input": stringArg(args, "input"), "title": stringArg(args, "title"), "body": stringArg(args, "body"), "output": stringArg(args, "output"), "backup": boolArg(args, "backup", true)})
			record, storeErr := evidence.Store("presentation-edit-plan", "document", stringArg(args, "input"), plan)
			return map[string]interface{}{"plan": plan, "evidence": record, "store_error": errString(storeErr)}, nil
		}
		result := osauto.EditPresentation(context.Background(), stringArg(args, "input"), stringArg(args, "title"), stringArg(args, "body"), stringArg(args, "output"), boolArg(args, "backup", true))
		record, storeErr := evidence.Store(result.Action, "document", stringArg(args, "input"), result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr), "validation": map[string]interface{}{"pptx": result.PPTX, "ok": result.OK, "error": result.Error, "summary": result.Stdout}}, nil
	case "meshclaw_presentation_export":
		if !boolArg(args, "execute", false) {
			plan := personalAssistantPlan("presentation_export", map[string]interface{}{"input": stringArg(args, "input"), "format": firstNonEmpty(stringArg(args, "format"), "pdf"), "output": stringArg(args, "output")})
			record, storeErr := evidence.Store("presentation-export-plan", "document", stringArg(args, "input"), plan)
			return map[string]interface{}{"plan": plan, "evidence": record, "store_error": errString(storeErr)}, nil
		}
		result := osauto.ExportPresentation(context.Background(), stringArg(args, "input"), stringArg(args, "format"), stringArg(args, "output"))
		record, storeErr := evidence.Store(result.Action, "document", stringArg(args, "input"), result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr), "validation": map[string]interface{}{"pptx": result.PPTX, "pdf": result.PDF, "ok": result.OK, "error": result.Error}}, nil
	case "meshclaw_screen_record":
		seconds := intArg(args, "seconds", 3)
		output := stringArg(args, "output")
		if strings.TrimSpace(output) == "" {
			output = defaultMCPRecordingPath("screen-record")
		}
		if !boolArg(args, "execute", false) {
			plan := screenProofPlan(seconds, output, stringArg(args, "purpose"))
			record, storeErr := evidence.Store("screen-record-plan", "automation", output, plan)
			return map[string]interface{}{"plan": plan, "evidence": record, "store_error": errString(storeErr)}, nil
		}
		result := osauto.ScreenRecord(context.Background(), seconds, output)
		record, storeErr := evidence.Store(result.Action, "automation", output, result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_screen_capture":
		output := stringArg(args, "output")
		if strings.TrimSpace(output) == "" {
			output = defaultMCPScreenCapturePath("screen-capture")
		}
		if !boolArg(args, "execute", false) {
			plan := screenCapturePlan(output, stringArg(args, "purpose"))
			record, storeErr := evidence.Store("screen-capture-plan", "automation", output, plan)
			return map[string]interface{}{"plan": plan, "evidence": record, "store_error": errString(storeErr)}, nil
		}
		if !boolArg(args, "approve", false) {
			plan := screenCapturePlan(output, stringArg(args, "purpose"))
			plan["approval_missing"] = true
			return map[string]interface{}{"error": "approval required for screen capture", "plan": plan}, nil
		}
		result := osauto.ScreenCapture(context.Background(), output)
		record, storeErr := evidence.Store(result.Action, "automation", output, result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_news_document":
		target := publicationTarget(args)
		if target != "local" {
			callerArgs := args
			args = cloneMCPArgs(args)
			args["target"] = "local"
			args["record_artifact"] = false
			remote, err := callRemoteMCPTool(target, "meshclaw_news_document", args, 90*time.Second)
			record, storeErr := evidence.Store("publish-news-doc-remote", target, "meshclaw_news_document", map[string]interface{}{"target": target, "remote": remote, "error": errString(err)})
			artifact := recordPublicationArtifact(callerArgs, target, "doc", "Argos news document", nestedString(remotePayload(remote), "document", "path"), firstRemoteLink(remotePayload(remote)), record)
			return map[string]interface{}{"target": target, "remote": remote, "mission_artifact": artifact, "evidence": record, "store_error": errString(storeErr), "error": errString(err), "scope_note": "Publication executed through the selected runtime target. The single user-facing Signal assistant remains the Mac mini Argos account."}, err
		}
		limit := intArg(args, "limit", 10)
		if limit <= 0 || limit > 10 {
			limit = 10
		}
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		brief := assistantbrief.News(ctx, assistantbrief.Options{Location: "Seoul", NewsLimit: limit, NoModelSummary: true})
		doc, err := publish.SaveNewsDocument(brief, publish.NewsDocumentOptions{Limit: limit})
		links := publish.DocumentLinks(doc.Path)
		preview := publish.DocumentPreviewImage(doc.Path)
		record, storeErr := evidence.Store("publish-news-doc", "mcp", "meshclaw_news_document", map[string]interface{}{
			"path":          doc.Path,
			"limit":         limit,
			"target":        "local",
			"items":         doc.Items,
			"links":         links,
			"preview_image": preview,
			"error":         errString(err),
		})
		artifact := recordPublicationArtifact(args, "local", "doc", "Argos news document", doc.Path, firstString(links), record)
		return map[string]interface{}{"target": "local", "document": doc, "links": links, "preview_image": preview, "brief": brief, "mission_artifact": artifact, "evidence": record, "store_error": errString(storeErr), "error": errString(err), "scope_note": "Local development mode. Omit target or use target=macmini for the unified publication runtime."}, err
	case "meshclaw_argos_research":
		target := publicationTarget(args)
		if target != "local" {
			callerArgs := args
			args = cloneMCPArgs(args)
			args["target"] = "local"
			args["record_artifact"] = false
			remote, err := callRemoteMCPTool(target, "meshclaw_argos_research", args, 120*time.Second)
			record, storeErr := evidence.Store("publish-argos-research-remote", target, stringArg(args, "query"), map[string]interface{}{"target": target, "remote": remote, "error": errString(err)})
			artifact := recordPublicationArtifact(callerArgs, target, "doc", "Argos research: "+stringArg(args, "query"), nestedString(remotePayload(remote), "report", "path"), firstRemoteLink(remotePayload(remote)), record)
			return map[string]interface{}{"target": target, "remote": remote, "mission_artifact": artifact, "evidence": record, "store_error": errString(storeErr), "error": errString(err), "scope_note": "Publication executed through the selected runtime target. The single user-facing Signal assistant remains the Mac mini Argos account."}, err
		}
		limit := intArg(args, "limit", 8)
		if limit <= 0 || limit > 10 {
			limit = 8
		}
		report, err := publish.Research(context.Background(), publish.ResearchOptions{
			Query:   stringArg(args, "query"),
			Limit:   limit,
			Timeout: intArg(args, "timeout", 20),
		})
		record, storeErr := evidence.Store("publish-argos-research", "mcp", report.Query, map[string]interface{}{
			"query":         report.Query,
			"target":        "local",
			"path":          report.Path,
			"preview_path":  report.PreviewPath,
			"preview_image": report.PreviewImage,
			"links":         report.Links,
			"items":         len(report.Search.Results),
			"error":         errString(err),
		})
		artifact := recordPublicationArtifact(args, "local", "doc", "Argos research: "+report.Query, report.Path, firstString(report.Links), record)
		return map[string]interface{}{"target": "local", "report": report, "mission_artifact": artifact, "evidence": record, "store_error": errString(storeErr), "error": errString(err), "scope_note": "Local development mode. Omit target or use target=macmini for the unified publication runtime."}, err
	case "meshclaw_automation_shortcuts":
		result := osauto.ShortcutsList(context.Background())
		record, storeErr := evidence.Store(result.Action, "automation", result.Action, result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_automation_shortcut_run":
		result := osauto.ShortcutRun(context.Background(), stringArg(args, "name"), stringArg(args, "input"))
		record, storeErr := evidence.Store(result.Action, "automation", result.Action, result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_shortcut_text_run":
		if !boolArg(args, "execute", false) || !boolArg(args, "approve", false) {
			plan := personalAssistantPlan("shortcut_text_run", map[string]interface{}{
				"name":          stringArg(args, "name"),
				"input":         stringArg(args, "input"),
				"title":         firstNonEmpty(stringArg(args, "title"), "Shortcuts 작업"),
				"save_artifact": boolArg(args, "save_artifact", true),
				"approve":       boolArg(args, "approve", false),
			})
			record, storeErr := evidence.Store("shortcut-text-run-plan", "automation", stringArg(args, "name"), plan)
			return map[string]interface{}{"plan": plan, "evidence": record, "store_error": errString(storeErr), "scope_note": "Shortcuts execution is blocked until execute=true and approve=true are both supplied."}, nil
		}
		result := osauto.RunShortcutTextTask(context.Background(), stringArg(args, "name"), stringArg(args, "input"), stringArg(args, "title"), boolArg(args, "save_artifact", true))
		record, storeErr := evidence.Store(result.Action, "automation", stringArg(args, "name"), result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_automation_open_url":
		result := osauto.OpenURL(context.Background(), stringArg(args, "url"))
		record, storeErr := evidence.Store(result.Action, "automation", result.Action, result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_automation_open_app":
		result := osauto.OpenApp(context.Background(), stringArg(args, "name"))
		record, storeErr := evidence.Store(result.Action, "automation", result.Action, result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_automation_open_file":
		if !boolArg(args, "execute", false) {
			plan := personalAssistantPlan("open_file", map[string]interface{}{"path": stringArg(args, "path"), "app": stringArg(args, "app")})
			record, storeErr := evidence.Store("open-file-plan", "automation", stringArg(args, "path"), plan)
			return map[string]interface{}{"plan": plan, "evidence": record, "store_error": errString(storeErr)}, nil
		}
		result := osauto.OpenFile(context.Background(), stringArg(args, "path"), stringArg(args, "app"))
		record, storeErr := evidence.Store(result.Action, "automation", stringArg(args, "path"), result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_notification_show":
		if !boolArg(args, "execute", false) {
			plan := personalAssistantPlan("notification_show", map[string]interface{}{"title": firstNonEmpty(stringArg(args, "title"), "Argos"), "body": stringArg(args, "body")})
			record, storeErr := evidence.Store("notification-show-plan", "automation", firstNonEmpty(stringArg(args, "title"), "Argos"), plan)
			return map[string]interface{}{"plan": plan, "evidence": record, "store_error": errString(storeErr)}, nil
		}
		result := osauto.ShowNotification(context.Background(), stringArg(args, "title"), stringArg(args, "body"))
		record, storeErr := evidence.Store(result.Action, "automation", firstNonEmpty(stringArg(args, "title"), "Argos"), result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_signal_call_doctor":
		return assistantCallDoctor(), nil
	case "meshclaw_signal_call":
		argv := []string{stringArg(args, "target"), "--audio", stringArg(args, "audio")}
		if value := intArg(args, "timeout", 0); value > 0 {
			argv = append(argv, "--timeout", strconv.Itoa(value))
		}
		if value := intArg(args, "restore_delay", -1); value >= 0 {
			argv = append(argv, "--restore-delay", strconv.Itoa(value))
		}
		if value := strings.TrimSpace(stringArg(args, "start_click")); value != "" {
			argv = append(argv, "--start-click", value)
		}
		if value := strings.TrimSpace(stringArg(args, "hangup_click")); value != "" {
			argv = append(argv, "--hangup-click", value)
		}
		if boolArg(args, "approve", false) {
			argv = append(argv, "--approve")
		}
		if boolArg(args, "execute", false) {
			argv = append(argv, "--execute")
		} else {
			argv = append(argv, "--dry-run")
		}
		result, err := assistantSignalCall(argv)
		return map[string]interface{}{"result": result, "error": errString(err)}, nil
	case "meshclaw_terminal_run":
		if !boolArg(args, "execute", false) || !boolArg(args, "approve", false) {
			plan := personalAssistantPlan("terminal_run", map[string]interface{}{
				"command":       stringArg(args, "command"),
				"shell":         firstNonEmpty(stringArg(args, "shell"), "/bin/zsh"),
				"title":         firstNonEmpty(stringArg(args, "title"), "터미널 작업"),
				"save_artifact": boolArg(args, "save_artifact", true),
				"approve":       boolArg(args, "approve", false),
			})
			record, storeErr := evidence.Store("terminal-run-plan", "automation", stringArg(args, "command"), plan)
			return map[string]interface{}{"plan": plan, "evidence": record, "store_error": errString(storeErr), "scope_note": "Terminal execution is blocked until execute=true and approve=true are both supplied."}, nil
		}
		result := osauto.RunTerminalTask(context.Background(), firstNonEmpty(stringArg(args, "shell"), "/bin/zsh"), stringArg(args, "command"), stringArg(args, "title"), boolArg(args, "save_artifact", true))
		record, storeErr := evidence.Store(result.Action, "automation", stringArg(args, "command"), result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_result_save":
		if !boolArg(args, "execute", false) {
			plan := personalAssistantPlan("result_save", map[string]interface{}{"title": firstNonEmpty(stringArg(args, "title"), "Argos 작업 결과"), "body": stringArg(args, "body"), "source": firstNonEmpty(stringArg(args, "source"), "meshclaw")})
			record, storeErr := evidence.Store("result-save-plan", "automation", firstNonEmpty(stringArg(args, "title"), "Argos 작업 결과"), plan)
			return map[string]interface{}{"plan": plan, "evidence": record, "store_error": errString(storeErr)}, nil
		}
		result := osauto.SaveAutomationResult(context.Background(), stringArg(args, "title"), stringArg(args, "body"), stringArg(args, "source"))
		record, storeErr := evidence.Store(result.Action, "automation", firstNonEmpty(stringArg(args, "title"), "Argos 작업 결과"), result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_maps_search":
		result := osauto.MapsSearch(context.Background(), stringArg(args, "query"), stringArg(args, "provider"), boolArg(args, "execute", false))
		record, storeErr := evidence.Store(result.Action, "maps", stringArg(args, "query"), result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr), "scope_note": "execute=false returns a usable map URL without changing local UI state.", "map_handoff": mapHandoffGuidance(result.URL, "place", boolArg(args, "execute", false))}, nil
	case "meshclaw_maps_directions":
		result := osauto.MapsDirections(context.Background(), stringArg(args, "origin"), stringArg(args, "destination"), stringArg(args, "mode"), stringArg(args, "provider"), boolArg(args, "execute", false))
		record, storeErr := evidence.Store(result.Action, "maps", firstNonEmpty(stringArg(args, "origin")+" -> "+stringArg(args, "destination"), stringArg(args, "destination")), result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr), "scope_note": "execute=false returns a usable directions URL without changing local UI state.", "map_handoff": mapHandoffGuidance(result.URL, "directions", boolArg(args, "execute", false))}, nil
	case "meshclaw_maps_proof":
		payload := runMapsProofMCP(args)
		record, storeErr := evidence.Store("maps-proof", "maps", fmt.Sprintf("%s %s", stringArg(args, "origin"), firstNonEmpty(stringArg(args, "destination"), stringArg(args, "query"))), payload)
		return map[string]interface{}{"proof": payload, "evidence": evidenceRef(record), "store_error": errString(storeErr)}, nil
	case "meshclaw_calendar_list":
		result := osauto.ListCalendarEvents(context.Background(), stringArg(args, "start"), stringArg(args, "end"), stringArg(args, "query"))
		record, storeErr := evidence.Store(result.Action, "calendar", firstNonEmpty(stringArg(args, "query"), stringArg(args, "start")+".."+stringArg(args, "end")), result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr), "scope_note": "Read-only local macOS Calendar query through MeshClaw Runtime."}, nil
	case "meshclaw_calendar_create_event":
		if !boolArg(args, "execute", false) {
			plan := personalAssistantPlan("calendar_event_create", map[string]interface{}{
				"title": stringArg(args, "title"),
				"start": stringArg(args, "start"),
				"end":   stringArg(args, "end"),
				"notes": stringArg(args, "notes"),
			})
			record, storeErr := evidence.Store("calendar-event-create-plan", "calendar", stringArg(args, "title"), plan)
			return map[string]interface{}{"plan": plan, "evidence": record, "store_error": errString(storeErr)}, nil
		}
		result := osauto.CreateCalendarEvent(context.Background(), stringArg(args, "title"), stringArg(args, "notes"), stringArg(args, "start"), stringArg(args, "end"))
		record, storeErr := evidence.Store(result.Action, "calendar", stringArg(args, "title"), result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_reminders_list":
		result := osauto.ListReminders(context.Background(), stringArg(args, "start"), stringArg(args, "end"), stringArg(args, "query"))
		record, storeErr := evidence.Store(result.Action, "reminders", firstNonEmpty(stringArg(args, "query"), stringArg(args, "start")+".."+stringArg(args, "end")), result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr), "scope_note": "Read-only local macOS Reminders query through MeshClaw Runtime."}, nil
	case "meshclaw_reminder_create":
		if !boolArg(args, "execute", false) {
			plan := personalAssistantPlan("reminder_create", map[string]interface{}{
				"title": stringArg(args, "title"),
				"due":   stringArg(args, "due"),
				"notes": stringArg(args, "notes"),
			})
			record, storeErr := evidence.Store("reminder-create-plan", "reminders", stringArg(args, "title"), plan)
			return map[string]interface{}{"plan": plan, "evidence": record, "store_error": errString(storeErr)}, nil
		}
		result := osauto.CreateReminder(context.Background(), stringArg(args, "title"), stringArg(args, "notes"), stringArg(args, "due"))
		record, storeErr := evidence.Store(result.Action, "reminders", stringArg(args, "title"), result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_reminder_complete":
		if !boolArg(args, "execute", false) {
			plan := personalAssistantPlan("reminder_complete", map[string]interface{}{"id": stringArg(args, "id"), "query": stringArg(args, "query")})
			record, storeErr := evidence.Store("reminder-complete-plan", "reminders", firstNonEmpty(stringArg(args, "id"), stringArg(args, "query")), plan)
			return map[string]interface{}{"plan": plan, "evidence": record, "store_error": errString(storeErr)}, nil
		}
		result := osauto.MutateReminder(context.Background(), "complete", stringArg(args, "id"), stringArg(args, "query"))
		record, storeErr := evidence.Store(result.Action, "reminders", firstNonEmpty(stringArg(args, "id"), stringArg(args, "query")), result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_reminder_delete":
		if !boolArg(args, "execute", false) {
			plan := personalAssistantPlan("reminder_delete", map[string]interface{}{"id": stringArg(args, "id"), "query": stringArg(args, "query")})
			record, storeErr := evidence.Store("reminder-delete-plan", "reminders", firstNonEmpty(stringArg(args, "id"), stringArg(args, "query")), plan)
			return map[string]interface{}{"plan": plan, "evidence": record, "store_error": errString(storeErr)}, nil
		}
		result := osauto.MutateReminder(context.Background(), "delete", stringArg(args, "id"), stringArg(args, "query"))
		record, storeErr := evidence.Store(result.Action, "reminders", firstNonEmpty(stringArg(args, "id"), stringArg(args, "query")), result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_contacts_search":
		result := osauto.SearchContacts(context.Background(), stringArg(args, "query"))
		record, storeErr := evidence.Store(result.Action, "contacts", stringArg(args, "query"), result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr), "scope_note": "Read-only local macOS Contacts search through MeshClaw Runtime."}, nil
	case "meshclaw_notes_search":
		result := osauto.SearchNotes(context.Background(), stringArg(args, "query"), intArg(args, "limit", 10))
		record, storeErr := evidence.Store(result.Action, "notes", firstNonEmpty(stringArg(args, "query"), "list"), result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr), "scope_note": "Read-only local macOS Notes search through MeshClaw Runtime."}, nil
	case "meshclaw_note_create":
		if !boolArg(args, "execute", false) {
			plan := personalAssistantPlan("note_create", map[string]interface{}{"title": firstNonEmpty(stringArg(args, "title"), "Argos Note"), "body": stringArg(args, "body")})
			record, storeErr := evidence.Store("note-create-plan", "notes", firstNonEmpty(stringArg(args, "title"), "Argos Note"), plan)
			return map[string]interface{}{"plan": plan, "evidence": record, "store_error": errString(storeErr)}, nil
		}
		result := osauto.CreateNote(context.Background(), firstNonEmpty(stringArg(args, "title"), "Argos Note"), stringArg(args, "body"))
		record, storeErr := evidence.Store(result.Action, "notes", firstNonEmpty(stringArg(args, "title"), "Argos Note"), result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_automation_clipboard_set":
		result := osauto.SetClipboard(context.Background(), stringArg(args, "text"))
		record, storeErr := evidence.Store(result.Action, "automation", result.Action, result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_automation_ai_handoff":
		result := osauto.AIHandoff(context.Background(), osauto.AIHandoffOptions{Provider: stringArg(args, "provider"), Prompt: stringArg(args, "prompt")})
		record, storeErr := evidence.Store(result.Action, "automation", result.Action, result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_subscription_frontends":
		report := osauto.FrontendsDoctor(context.Background())
		record, storeErr := evidence.Store("subscription-frontends", "automation", "subscription_frontends", report)
		return map[string]interface{}{"report": report, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_argos_macos_doctor":
		report := osauto.ArgosMacDoctor(context.Background(), boolArg(args, "check_screen_recording", true))
		record, storeErr := evidence.Store("argos-macos-doctor", "argos", "macos_doctor", report)
		return map[string]interface{}{"report": report, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_argos_macos_setup":
		report := osauto.ArgosMacSetup(context.Background())
		record, storeErr := evidence.Store("argos-macos-setup", "argos", "macos_setup", report)
		return map[string]interface{}{"report": report, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_automation_mouse_click":
		result := osauto.MouseClick(context.Background(), stringArg(args, "runner_url"), floatArg(args, "x", 0), floatArg(args, "y", 0))
		record, storeErr := evidence.Store(result.Action, "automation", result.Action, result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_automation_script_run":
		result := osauto.ScriptRun(context.Background(), stringArg(args, "shell"), stringArg(args, "script"))
		record, storeErr := evidence.Store(result.Action, "automation", result.Action, result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_argos_ask":
		action := osauto.ArgosDo(context.Background(), osauto.ArgosRequest{Text: stringArg(args, "text"), Execute: boolArg(args, "execute", false), RecordSeconds: intArg(args, "record_seconds", 0)})
		record, storeErr := evidence.Store(action.Action, "argos", action.Text, action)
		return map[string]interface{}{"action": action, "evidence": record, "store_error": errString(storeErr)}, nil
	case "meshclaw_workflow_run":
		name := stringArg(args, "name")
		mode := runtimeflow.Mode(stringArg(args, "mode"))
		if mode == "" {
			mode = runtimeflow.DryRun
		}
		if mode != runtimeflow.DryRun && mode != runtimeflow.Execute {
			return nil, fmt.Errorf("mode must be dry-run or execute")
		}
		if !runtimeflow.IsKnown(name) {
			return nil, fmt.Errorf("unknown workflow: %s", name)
		}
		decision := policy.Evaluate(policy.Request{Subject: "mcp", Action: "workflow_run", Resource: "workflow", Context: name + " " + string(mode)})
		if decision.Decision == policy.Deny {
			return map[string]interface{}{"policy": decision, "success": false}, nil
		}
		result, err := runtimeflow.RunSelectedWithApprovals(name, mode, stringArg(args, "approvals_ref"), []string{stringArg(args, "steps")})
		if err != nil {
			return nil, err
		}
		return slimWorkflowMCP(result, decision), nil
	case "meshclaw_evidence_latest":
		return latestRuntimeEvidenceMCP(intArg(args, "actions_preview_bytes", 4000))
	case "meshclaw_evidence_list":
		return evidence.List(intArg(args, "limit", 20))
	case "meshclaw_data_doctor":
		report, err := datadoctor.Check(time.Now().UTC())
		if err != nil {
			return nil, err
		}
		record, storeErr := evidence.Store("data-doctor", "meshclaw", dataDoctorEvidenceSummary(report), report)
		return map[string]interface{}{"report": report, "evidence": evidenceRef(record), "store_error": errString(storeErr)}, nil
	case "meshclaw_data_archive_plan":
		plan, err := datadoctor.EvidenceArchivePlan(time.Now().UTC(), intArg(args, "keep_newest", 14))
		if err != nil {
			return nil, err
		}
		record, storeErr := evidence.Store("data-archive-plan", "meshclaw", fmt.Sprintf("candidates=%d files=%d", len(plan.Candidates), plan.CandidateFiles), plan)
		return map[string]interface{}{"plan": plan, "evidence": evidenceRef(record), "store_error": errString(storeErr)}, nil
	case "meshclaw_downloads_cleanup_plan":
		plan, err := fileorg.DownloadsCleanupPlan(time.Now().UTC(), stringArg(args, "path"), intArg(args, "min_age_days", 30), intArg(args, "large_mb", 500), intArg(args, "limit", 50))
		if err != nil {
			return nil, err
		}
		record, storeErr := evidence.Store("downloads-cleanup-plan", "files", fmt.Sprintf("candidates=%d bytes=%d", plan.CandidateFiles, plan.CandidateBytes), plan)
		return map[string]interface{}{"plan": plan, "evidence": evidenceRef(record), "store_error": errString(storeErr)}, nil
	case "meshclaw_downloads_cleanup_apply":
		paths := splitCSV(stringArg(args, "paths"))
		plan, err := fileorg.DownloadsCleanupApply(time.Now().UTC(), paths, stringArg(args, "destination"), boolArg(args, "execute", false), boolArg(args, "approve", false))
		if err != nil {
			record, storeErr := evidence.Store("downloads-cleanup-apply", "files", fmt.Sprintf("status=%s candidates=%d moved=%d", plan.Status, plan.CandidateFiles, plan.CompletedFiles), plan)
			return map[string]interface{}{"plan": plan, "evidence": evidenceRef(record), "store_error": errString(storeErr), "error": err.Error()}, nil
		}
		record, storeErr := evidence.Store("downloads-cleanup-apply", "files", fmt.Sprintf("status=%s candidates=%d moved=%d", plan.Status, plan.CandidateFiles, plan.CompletedFiles), plan)
		return map[string]interface{}{"plan": plan, "evidence": evidenceRef(record), "store_error": errString(storeErr)}, nil
	case "meshclaw_legacy_skills_audit":
		report, err := legacyskills.Audit(time.Now().UTC(), stringArg(args, "path"), intArg(args, "limit", 120))
		if err != nil {
			return nil, err
		}
		record, storeErr := evidence.Store("legacy-skills-audit", "skills", fmt.Sprintf("total=%d covered=%d migrate=%d archive=%d missing_deps=%d", report.TotalSkills, report.AlreadyCovered, report.MigrateCandidates, report.ArchiveCandidates, report.MissingDeps), report)
		return map[string]interface{}{"report": report, "evidence": evidenceRef(record), "store_error": errString(storeErr)}, nil
	case "meshclaw_policy_check":
		return policy.Evaluate(policy.Request{
			Subject:  stringArg(args, "subject"),
			Action:   stringArg(args, "action"),
			Resource: stringArg(args, "resource"),
			Context:  stringArg(args, "context"),
		}), nil
	case "meshclaw_policy_show":
		cfg, path, err := policy.LoadConfig()
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"path": path, "policy": cfg}, nil
	case "meshclaw_matrix_plan":
		return map[string]interface{}{
			"role": "Matrix is an operations room, approval channel, notification channel, and optional MCP command surface. It is not the assistant brain.",
			"flow": []string{
				"human or AI operator posts an ops request in Matrix",
				"Matrix bridge normalizes the request into a MeshClaw MCP/CLI tool call",
				"MeshClaw policy decides allow, require_approval, or deny",
				"MeshClaw executes only allowed tools through vssh/runtime",
				"Matrix bridge posts concise results and evidence links back to the room",
			},
		}, nil
	case "meshclaw_ops_dispatch":
		return dispatchOpsMessage(stringArg(args, "source"), stringArg(args, "message"))
	case "meshclaw_route":
		return router.Classify(router.Options{
			Source:  firstNonEmpty(stringArg(args, "source"), "openwebui"),
			Subject: firstNonEmpty(stringArg(args, "subject"), "local-llm"),
			Message: stringArg(args, "message"),
		}), nil
	case "meshclaw_ask":
		plan := router.Classify(router.Options{
			Source:  firstNonEmpty(stringArg(args, "source"), "openwebui"),
			Subject: firstNonEmpty(stringArg(args, "subject"), "local-llm"),
			Message: stringArg(args, "message"),
		})
		return runRouterPlanForMCP(plan)
	case "meshclaw_schedule_plan":
		return scheduler.DefaultPlan(), nil
	case "meshclaw_schedule_status":
		plan := scheduler.DefaultPlan()
		state, err := scheduler.LoadState()
		if err != nil {
			return scheduleStatusPayload(plan, state, err, time.Now().UTC()), nil
		}
		return scheduleStatusPayload(plan, state, nil, time.Now().UTC()), nil
	case "meshclaw_schedule_run_once":
		ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
		defer cancel()
		result, err := scheduler.RunOnce(ctx, scheduler.RunOptions{
			JobID:   stringArg(args, "job"),
			Target:  stringArg(args, "target"),
			Execute: boolArg(args, "execute", false),
		})
		return map[string]interface{}{"result": result, "error": errString(err)}, nil
	case "meshclaw_provision_plan":
		req := provision.Request{
			Purpose:       stringArg(args, "purpose"),
			Provider:      stringArg(args, "provider"),
			Region:        stringArg(args, "region"),
			BudgetUSD:     floatArg(args, "budget_usd", 0),
			TTLHours:      intArg(args, "ttl_hours", 6),
			RequiredClass: stringArg(args, "required_class"),
		}
		plan := provision.NewPlan(req)
		record, err := evidence.Store("provision-plan", "provider", req.Purpose, plan)
		return map[string]interface{}{"plan": plan, "evidence": record, "store_error": errString(err)}, nil
	case "meshclaw_run_evidence":
		host := stringArg(args, "host")
		command := stringArg(args, "command")
		decision := policy.Evaluate(policy.Request{Subject: "mcp", Action: "run_command", Resource: "server", Context: command})
		if decision.Decision != policy.Allow {
			return map[string]interface{}{"policy": decision, "success": false}, nil
		}
		result := runtime.NewRunner().RunEvidence(host, command)
		record, err := evidence.Store("run-evidence", host, command, result)
		return map[string]interface{}{"policy": decision, "result": result, "evidence": record, "store_error": errString(err)}, nil
	case "meshclaw_job_start":
		host := stringArg(args, "host")
		command := stringArg(args, "command")
		decision := policy.Evaluate(policy.Request{Subject: "mcp", Action: "run_command", Resource: "server", Context: command})
		if decision.Decision != policy.Allow {
			return map[string]interface{}{"policy": decision, "success": false}, nil
		}
		result, runErr := runtime.NewRunner().VSSHJSON("job-start", host, command)
		record, storeErr := evidence.Store("job-start", host, command, result)
		return map[string]interface{}{"policy": decision, "result": result, "evidence": record, "store_error": errString(storeErr), "run_error": errString(runErr)}, nil
	case "meshclaw_job_status":
		return mcpVSSHJobRead("job-status", "job_status", args)
	case "meshclaw_job_logs":
		host := stringArg(args, "host")
		id := stringArg(args, "id")
		decision := policy.Evaluate(policy.Request{Subject: "mcp", Action: "job_logs", Resource: "server", Context: id})
		if decision.Decision != policy.Allow {
			return map[string]interface{}{"policy": decision, "success": false}, nil
		}
		params := []string{"job-logs", host, id}
		if tail := intArg(args, "tail_bytes", 0); tail > 0 {
			params = append(params, fmt.Sprintf("%d", tail))
		}
		result, runErr := runtime.NewRunner().VSSHJSON(params...)
		record, storeErr := evidence.Store("job-logs", host, id, result)
		return map[string]interface{}{"policy": decision, "result": result, "evidence": record, "store_error": errString(storeErr), "run_error": errString(runErr)}, nil
	case "meshclaw_job_cancel":
		host := stringArg(args, "host")
		id := stringArg(args, "id")
		decision := policy.Evaluate(policy.Request{Subject: "mcp", Action: "job_cancel", Resource: "server", Context: id})
		if decision.Decision != policy.Allow {
			return map[string]interface{}{"policy": decision, "success": false}, nil
		}
		result, runErr := runtime.NewRunner().VSSHJSON("job-cancel", host, id)
		record, storeErr := evidence.Store("job-cancel", host, id, result)
		return map[string]interface{}{"policy": decision, "result": result, "evidence": record, "store_error": errString(storeErr), "run_error": errString(runErr)}, nil
	case "meshclaw_artifact_collect":
		host := stringArg(args, "host")
		path := stringArg(args, "path")
		decision := policy.Evaluate(policy.Request{Subject: "mcp", Action: "artifact_collect", Resource: "server", Context: path})
		if decision.Decision != policy.Allow {
			return map[string]interface{}{"policy": decision, "success": false}, nil
		}
		params := []string{"artifact-collect", host, path}
		if maxBytes := intArg(args, "max_bytes", 0); maxBytes > 0 {
			params = append(params, fmt.Sprintf("%d", maxBytes))
		}
		result, runErr := runtime.NewRunner().VSSHJSON(params...)
		record, storeErr := evidence.Store("artifact-collect", host, path, result)
		return map[string]interface{}{"policy": decision, "result": result, "evidence": record, "store_error": errString(storeErr), "run_error": errString(runErr)}, nil
	case "meshclaw_disk_investigate":
		host := stringArg(args, "host")
		path := stringArg(args, "path")
		if path == "" {
			path = "/"
		}
		command := fmt.Sprintf("df -h %s && sudo du -xhd1 %s 2>/dev/null | sort -h | tail -40", shellQuote(path), shellQuote(path))
		result := runtime.NewRunner().RunEvidence(host, command)
		record, err := evidence.Store("disk-investigate", host, path, result)
		return map[string]interface{}{"result": result, "evidence": record, "store_error": errString(err), "time": time.Now().UTC()}, nil
	case "meshclaw_process_top":
		host := stringArg(args, "host")
		report := workflow.ProcessTop(host)
		record, err := evidence.Store("process-top", host, "top memory/cpu processes", report)
		return map[string]interface{}{"report": report, "evidence": record, "store_error": errString(err)}, nil
	case "meshclaw_data_clean_plan":
		host := stringArg(args, "host")
		path := stringArg(args, "path")
		report := workflow.DataCleanPlan(host, path)
		record, err := evidence.Store("data-clean-plan", host, path, report)
		return map[string]interface{}{"report": report, "evidence": record, "store_error": errString(err)}, nil
	case "meshclaw_data_clean_apply":
		host := stringArg(args, "host")
		manifest := stringArg(args, "manifest")
		report := workflow.DataCleanApply(host, manifest)
		record, err := evidence.Store("data-clean-apply", host, manifest, report)
		return map[string]interface{}{"report": report, "evidence": record, "store_error": errString(err)}, nil
	case "meshclaw_analyze_logs":
		host := stringArg(args, "host")
		source := stringArg(args, "source")
		if source == "" {
			source = "system"
		}
		report := workflow.AnalyzeLogs(host, source)
		record, err := evidence.Store("analyze-logs", host, source, report)
		payload := map[string]interface{}{"report": report, "evidence": record, "store_error": errString(err)}
		if contract := analyzeLogsMCPHandoffContract(report.AutohealHandoff); contract != nil {
			payload["handoff_contract"] = contract
		}
		return payload, nil
	case "meshclaw_security_check":
		host := stringArg(args, "host")
		report := workflow.SecurityCheck(host)
		record, err := evidence.Store("security-check", host, "read-only snapshot", report)
		return map[string]interface{}{"report": report, "evidence": record, "store_error": errString(err)}, nil
	case "meshclaw_guard_modes":
		return map[string]interface{}{"modes": guard.Modes()}, nil
	case "meshclaw_guard_model":
		return map[string]interface{}{"local_model": guard.LocalModel()}, nil
	case "meshclaw_guard_clients":
		return map[string]interface{}{"clients": guard.ClientGuides()}, nil
	case "meshclaw_guard_session_policy":
		return map[string]interface{}{"policy": guard.GuardSessionPolicy()}, nil
	case "meshclaw_guard_signal_policy":
		return map[string]interface{}{"signal": guard.SignalGuardPolicy()}, nil
	case "meshclaw_guard_vault":
		return map[string]interface{}{"vault": guard.VaultConversationPlan()}, nil
	case "meshclaw_guard_vault_backends":
		return map[string]interface{}{"backends": guardvault.Backends()}, nil
	case "meshclaw_guard_vault_list":
		entries, err := guardvault.List()
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"entries": entries}, nil
	case "meshclaw_guard_vault_metadata":
		entry, err := guardvault.MetadataByHandle(stringArg(args, "handle"))
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"entry": entry}, nil
	case "meshclaw_guard_detect":
		target := stringArg(args, "target")
		report := guard.Detect(target, stringArg(args, "text"))
		record, err := evidence.Store("guard-detect", target, report.Status, report)
		return map[string]interface{}{"report": report, "evidence": record, "store_error": errString(err)}, nil
	case "meshclaw_guard_redact":
		target := stringArg(args, "target")
		report := guard.Redact(target, stringArg(args, "text"))
		record, err := evidence.Store("guard-redact", target, report.Status, report)
		return map[string]interface{}{"report": report, "evidence": record, "store_error": errString(err)}, nil
	case "meshclaw_guard_posture":
		report := guard.Posture(stringArg(args, "home"))
		record, err := evidence.Store("guard-posture", "local", report.Status, report)
		return map[string]interface{}{"report": report, "evidence": record, "store_error": errString(err)}, nil
	case "meshclaw_guard_clean_plan":
		plan := guard.Cleanup(stringArg(args, "home"))
		record, err := evidence.Store("guard-clean-plan", "local", plan.Status, plan)
		return map[string]interface{}{"plan": plan, "evidence": record, "store_error": errString(err)}, nil
	case "meshclaw_guard_vuln":
		path := stringArg(args, "path")
		report := guard.VulnScanPath(path)
		record, err := evidence.Store("guard-vuln", path, report.Status, report)
		return map[string]interface{}{"report": report, "evidence": record, "store_error": errString(err)}, nil
	case "meshclaw_guard_vuln_scan":
		host := strings.TrimSpace(stringArg(args, "host"))
		if host == "" {
			return map[string]interface{}{"status": "failed", "error": "host is required"}, nil
		}
		decision := policy.Evaluate(policy.Request{Subject: "mcp", Action: "run_command", Resource: "server", Context: "guard-vuln-scan package inventory"})
		if decision.Decision != policy.Allow {
			return map[string]interface{}{"policy": decision, "status": "denied"}, nil
		}
		inv := runtime.NewRunner().RunEvidence(host, guardcve.InventoryCommand())
		pkgs := guardcve.ParseInventory(inv.Stdout)
		report := guardcve.Scan(host, pkgs, guardcve.Options{
			Offline:    boolArg(args, "offline", false),
			MaxDetail:  intArg(args, "max_detail", 40),
			Ecosystems: splitCSV(stringArg(args, "ecosystems")),
		})
		if !inv.Success {
			report.Errors = append(report.Errors, "inventory collection reported a non-success transport result on "+host)
		}
		record, err := evidence.Store("guard-vuln-scan", host, report.Status, report)
		return map[string]interface{}{
			"policy":              decision,
			"report":              report,
			"evidence":            record,
			"store_error":         errString(err),
			"inventory_transport": inv.Transport,
		}, nil
	case "meshclaw_guard_code_review":
		host := strings.TrimSpace(stringArg(args, "host"))
		repoPath := strings.TrimSpace(stringArg(args, "repo_path"))
		if host == "" || repoPath == "" {
			return map[string]interface{}{"status": "failed", "error": "host and repo_path are required"}, nil
		}
		decision := policy.Evaluate(policy.Request{Subject: "mcp", Action: "run_command", Resource: "server", Context: "guard-code-review semgrep+bandit"})
		if decision.Decision != policy.Allow {
			return map[string]interface{}{"policy": decision, "status": "denied"}, nil
		}
		scan := runtime.NewRunner().RunEvidence(host, guardcode.ScanCommand(repoPath))
		report := guardcode.BuildReport(host, repoPath, scan.Stdout, scan.Success)
		record, err := evidence.Store("guard-code-review", host, report.Status, report)
		return map[string]interface{}{
			"policy":           decision,
			"report":           report,
			"evidence":         record,
			"store_error":      errString(err),
			"review_transport": scan.Transport,
		}, nil
	case "meshclaw_guard_intent":
		return map[string]interface{}{"intent": guard.ParseIntent(stringArg(args, "text"))}, nil
	case "meshclaw_hygiene_scan_host":
		host := stringArg(args, "host")
		report := hygiene.ScanHost(host)
		record, err := evidence.Store("hygiene-scan-host", host, report.Status, report)
		return map[string]interface{}{"report": report, "evidence": record, "store_error": errString(err)}, nil
	case "meshclaw_service_check":
		host := stringArg(args, "host")
		service := stringArg(args, "service")
		report := workflow.ServiceCheck(host, service)
		record, err := evidence.Store("service-check", host, service, report)
		return map[string]interface{}{"report": report, "evidence": record, "store_error": errString(err)}, nil
	case "meshclaw_service_audit":
		host := stringArg(args, "host")
		report := workflow.ServiceAudit(host)
		record, err := evidence.Store("service-audit", host, "failed/restarting services", report)
		return map[string]interface{}{"report": report, "evidence": record, "store_error": errString(err)}, nil
	case "meshclaw_service_quarantine":
		host := stringArg(args, "host")
		service := stringArg(args, "service")
		report := workflow.ServiceQuarantine(host, service)
		record, err := evidence.Store("service-quarantine", host, service, report)
		return map[string]interface{}{"report": report, "evidence": record, "store_error": errString(err)}, nil
	case "meshclaw_service_remove":
		host := stringArg(args, "host")
		service := stringArg(args, "service")
		path := stringArg(args, "path")
		report := workflow.ServiceRemove(host, service, path)
		record, err := evidence.Store("service-remove", host, service, report)
		return map[string]interface{}{"report": report, "evidence": record, "store_error": errString(err)}, nil
	default:
		return nil, fmt.Errorf("unknown tool: %s", name)
	}
}

func buildMCPReconcilePlan(args map[string]interface{}, reportKind, evidenceKind string) (map[string]interface{}, error) {
	desiredPath := stringArg(args, "desired_path")
	desired, err := reconciler.LoadDesiredState(desiredPath)
	if err != nil {
		return nil, err
	}
	actuals, actualPaths, err := reconcileActualReports([]string{"--actual-report", stringArg(args, "actual_report_path")})
	if err != nil {
		return nil, err
	}
	report := buildReconcilePlanReport(desiredPath, desired, actuals, actualPaths)
	report.Kind = reportKind
	record, storeErr := evidence.Store(evidenceKind, "fleet", reconcilePlanCountsLine(report.Counts), report)
	return map[string]interface{}{
		"report":                  report,
		"counts":                  report.Counts,
		"reconcile_plan_contract": reconcilePlanMCPContract(report),
		"evidence":                record,
		"store_error":             errString(storeErr),
	}, nil
}

func buildMCPReconcileDesiredValidation(args map[string]interface{}) (map[string]interface{}, error) {
	desiredPath := stringArg(args, "desired_path")
	desired, err := reconciler.LoadDesiredState(desiredPath)
	if err != nil {
		return nil, err
	}
	report := buildReconcileDesiredValidationReport(desiredPath, desired)
	record, storeErr := evidence.Store("reconcile-desired-validation", "fleet", fmt.Sprintf("ready=%t findings=%d", report.Ready, len(report.Findings)), report)
	return map[string]interface{}{
		"desired_validation":          report,
		"desired_validation_contract": desiredValidationMCPContract(report),
		"evidence":                    record,
		"store_error":                 errString(storeErr),
	}, nil
}

func buildMCPReconcileApprovalRequest(args map[string]interface{}) (map[string]interface{}, error) {
	desiredPath := stringArg(args, "desired_path")
	desired, err := reconciler.LoadDesiredState(desiredPath)
	if err != nil {
		return nil, err
	}
	actuals, actualPaths, err := reconcileActualReports([]string{"--actual-report", stringArg(args, "actual_report_path")})
	if err != nil {
		return nil, err
	}
	plan := buildReconcilePlanReport(desiredPath, desired, actuals, actualPaths)
	plan.Kind = "meshclaw_reconcile_approval_request"
	request := buildReconcileApprovalRequest(plan)
	record, storeErr := evidence.Store("reconcile-approval-request", "fleet", fmt.Sprintf("actions=%d blocked=%d", len(request.Actions), len(request.BlockedActions)), request)
	return map[string]interface{}{
		"approval_request":          request,
		"approval_request_contract": reconcileApprovalRequestMCPContract(request),
		"plan":                      plan,
		"evidence":                  record,
		"store_error":               errString(storeErr),
	}, nil
}

func buildMCPReconcileApplyGate(args map[string]interface{}) (map[string]interface{}, error) {
	evidencePath := stringArg(args, "approval_evidence_path")
	record, err := evidence.Load(evidencePath)
	if err != nil {
		return nil, err
	}
	request, err := approvalRequestFromEvidence(record)
	if err != nil {
		return nil, err
	}
	report := buildReconcileApplyGateReport(stringArg(args, "desired_path"), evidencePath, record, request, stringArg(args, "approved_by"))
	gateRecord, storeErr := evidence.Store("reconcile-apply-gate", "fleet", fmt.Sprintf("ready=%t status=%s", report.Ready, report.Status), report)
	return map[string]interface{}{
		"apply_gate":          report,
		"apply_gate_contract": reconcileApplyGateMCPContract(report),
		"evidence":            gateRecord,
		"store_error":         errString(storeErr),
	}, nil
}

func buildMCPReconcileApplyPlan(args map[string]interface{}) (map[string]interface{}, error) {
	gatePath := stringArg(args, "gate_evidence_path")
	record, err := evidence.Load(gatePath)
	if err != nil {
		return nil, err
	}
	gate, err := applyGateFromEvidence(record)
	if err != nil {
		return nil, err
	}
	report := buildReconcileApplyPlanReport(gatePath, record, gate)
	planRecord, storeErr := evidence.Store("reconcile-apply-plan", "fleet", fmt.Sprintf("ready=%t steps=%d", report.Ready, len(report.Steps)), report)
	return map[string]interface{}{
		"apply_plan":          report,
		"apply_plan_contract": reconcileApplyPlanMCPContract(report),
		"evidence":            planRecord,
		"store_error":         errString(storeErr),
	}, nil
}

func buildMCPReconcileExecutionPreview(args map[string]interface{}) (map[string]interface{}, error) {
	planPath := stringArg(args, "apply_plan_evidence_path")
	record, err := evidence.Load(planPath)
	if err != nil {
		return nil, err
	}
	plan, err := applyPlanFromEvidence(record)
	if err != nil {
		return nil, err
	}
	report := buildReconcileExecutionPreviewReport(planPath, record, plan)
	previewRecord, storeErr := evidence.Store("reconcile-execution-preview", "fleet", fmt.Sprintf("ready=%t commands=%d", report.Ready, len(report.Commands)), report)
	return map[string]interface{}{
		"execution_preview":          report,
		"execution_preview_contract": reconcileExecutionPreviewMCPContract(report),
		"evidence":                   previewRecord,
		"store_error":                errString(storeErr),
	}, nil
}

func buildMCPReconcileVerificationPlan(args map[string]interface{}) (map[string]interface{}, error) {
	previewPath := stringArg(args, "execution_preview_evidence_path")
	record, err := evidence.Load(previewPath)
	if err != nil {
		return nil, err
	}
	preview, err := executionPreviewFromEvidence(record)
	if err != nil {
		return nil, err
	}
	report := buildReconcileVerificationPlanReport(previewPath, record, preview)
	verifyRecord, storeErr := evidence.Store("reconcile-verification-plan", "fleet", fmt.Sprintf("ready=%t checks=%d", report.Ready, len(report.Checks)), report)
	return map[string]interface{}{
		"verification_plan":          report,
		"verification_plan_contract": reconcileVerificationPlanMCPContract(report),
		"evidence":                   verifyRecord,
		"store_error":                errString(storeErr),
	}, nil
}

func buildMCPReconcileRunbook(args map[string]interface{}) (map[string]interface{}, error) {
	verifyPath := stringArg(args, "verification_plan_evidence_path")
	record, err := evidence.Load(verifyPath)
	if err != nil {
		return nil, err
	}
	verification, err := verificationPlanFromEvidence(record)
	if err != nil {
		return nil, err
	}
	report := buildReconcileRunbookReport(verifyPath, record, verification)
	runbookRecord, storeErr := evidence.Store("reconcile-runbook", "fleet", fmt.Sprintf("ready=%t steps=%d", report.Ready, len(report.Steps)), report)
	return map[string]interface{}{
		"runbook":          report,
		"runbook_contract": reconcileRunbookMCPContract(report),
		"evidence":         runbookRecord,
		"store_error":      errString(storeErr),
	}, nil
}

func buildMCPReconcileRunbookCheck(args map[string]interface{}) (map[string]interface{}, error) {
	runbookPath := stringArg(args, "runbook_evidence_path")
	record, err := evidence.Load(runbookPath)
	if err != nil {
		return nil, err
	}
	runbook, err := runbookFromEvidence(record)
	if err != nil {
		return nil, err
	}
	report := buildReconcileRunbookCheckReport(runbookPath, record, runbook)
	checkRecord, storeErr := evidence.Store("reconcile-runbook-check", "fleet", fmt.Sprintf("ready=%t findings=%d", report.Ready, len(report.Findings)), report)
	return map[string]interface{}{
		"runbook_check":          report,
		"runbook_check_contract": reconcileRunbookCheckMCPContract(report),
		"evidence":               checkRecord,
		"store_error":            errString(storeErr),
	}, nil
}

func buildMCPReconcileRollbackPlan(args map[string]interface{}) (map[string]interface{}, error) {
	checkPath := stringArg(args, "runbook_check_evidence_path")
	record, err := evidence.Load(checkPath)
	if err != nil {
		return nil, err
	}
	check, err := runbookCheckFromEvidence(record)
	if err != nil {
		return nil, err
	}
	report := buildReconcileRollbackPlanReport(checkPath, record, check)
	rollbackRecord, storeErr := evidence.Store("reconcile-rollback-plan", "fleet", fmt.Sprintf("ready=%t steps=%d", report.Ready, len(report.Steps)), report)
	return map[string]interface{}{
		"rollback_plan":          report,
		"rollback_plan_contract": reconcileRollbackPlanMCPContract(report),
		"evidence":               rollbackRecord,
		"store_error":            errString(storeErr),
	}, nil
}

func buildMCPReconcileCompletionPlan(args map[string]interface{}) (map[string]interface{}, error) {
	rollbackPath := stringArg(args, "rollback_plan_evidence_path")
	record, err := evidence.Load(rollbackPath)
	if err != nil {
		return nil, err
	}
	rollback, err := rollbackPlanFromEvidence(record)
	if err != nil {
		return nil, err
	}
	report := buildReconcileCompletionPlanReport(rollbackPath, record, rollback)
	completionRecord, storeErr := evidence.Store("reconcile-completion-plan", "fleet", fmt.Sprintf("ready=%t requirements=%d", report.Ready, len(report.Requirements)), report)
	return map[string]interface{}{
		"completion_plan":          report,
		"completion_plan_contract": reconcileCompletionPlanMCPContract(report),
		"evidence":                 completionRecord,
		"store_error":              errString(storeErr),
	}, nil
}

func buildMCPReconcileReadinessSummary(args map[string]interface{}) (map[string]interface{}, error) {
	completionPath := stringArg(args, "completion_plan_evidence_path")
	record, err := evidence.Load(completionPath)
	if err != nil {
		return nil, err
	}
	completion, err := completionPlanFromEvidence(record)
	if err != nil {
		return nil, err
	}
	report := buildReconcileReadinessSummaryReport(completionPath, record, completion)
	summaryRecord, storeErr := evidence.Store("reconcile-readiness-summary", "fleet", fmt.Sprintf("ready=%t stages=%d blockers=%d", report.Ready, len(report.ReadyStages), len(report.Blockers)), report)
	return map[string]interface{}{
		"readiness_summary":  report,
		"readiness_contract": readinessMCPContract("reconcile_readiness_contract", report.StopBefore),
		"evidence":           summaryRecord,
		"store_error":        errString(storeErr),
	}, nil
}

func personalAssistantPlan(action string, args map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"kind":              "meshclaw_personal_assistant_plan",
		"action":            action,
		"arguments":         args,
		"execute":           false,
		"approval_required": true,
		"policy":            "Default is plan-only for personal data mutations. Re-run with execute=true after user confirmation.",
		"scope_note":        "This is a local macOS personal assistant action managed by MeshClaw Runtime with evidence.",
	}
}

func screenProofPlan(seconds int, output, purpose string) map[string]interface{} {
	if seconds <= 0 {
		seconds = 3
	}
	purpose = strings.TrimSpace(purpose)
	if purpose == "" {
		purpose = "visible screen proof"
	}
	return map[string]interface{}{
		"kind":              "meshclaw_screen_proof_plan",
		"action":            "screen_record",
		"arguments":         map[string]interface{}{"seconds": seconds, "output": output, "purpose": purpose},
		"execute":           false,
		"approval_required": true,
		"purpose":           purpose,
		"output":            output,
		"seconds":           seconds,
		"user_message":      "화면 증거를 남기려면 먼저 민감정보가 보이지 않는지 확인한 뒤 짧은 화면 기록을 실행해야 합니다.",
		"privacy_check": []string{
			"Close or hide private chats, passwords, payment details, and personal identifiers.",
			"Keep the recording short and focused on the requested proof.",
			"Re-run with execute=true only after the user approves the visible capture.",
		},
		"use_cases": []string{
			"map route or place proof",
			"shopping/product comparison proof before checkout",
			"logged-in form review before submission",
			"AI frontend handoff proof such as a ChatGPT answer",
		},
		"policy":     "Default is plan-only because screen recordings may include sensitive visible data.",
		"scope_note": "This records the current visible macOS session through Argos UI Runner and stores evidence.",
	}
}

func screenCapturePlan(output, purpose string) map[string]interface{} {
	purpose = strings.TrimSpace(purpose)
	if purpose == "" {
		purpose = "visible still screen proof"
	}
	return map[string]interface{}{
		"kind":              "meshclaw_screen_capture_plan",
		"action":            "screen_capture",
		"arguments":         map[string]interface{}{"output": output, "purpose": purpose},
		"execute":           false,
		"approval_required": true,
		"purpose":           purpose,
		"output":            output,
		"user_message":      "화면 캡처를 남기려면 먼저 민감정보가 보이지 않는지 확인한 뒤 현재 화면을 사진 증거로 저장해야 합니다.",
		"privacy_check": []string{
			"Close or hide private chats, passwords, payment details, and personal identifiers.",
			"Keep only the requested map, comparison, form, or AI answer visible.",
			"Re-run with execute=true and approve=true only after the user approves the visible capture.",
		},
		"use_cases": []string{
			"map route or place screenshot proof",
			"shopping/product comparison screenshot before checkout",
			"logged-in form review before submission",
			"AI frontend handoff proof such as a ChatGPT answer",
		},
		"policy":     "Default is plan-only because screenshots may include sensitive visible data.",
		"scope_note": "This captures the current visible macOS session as a still PNG screenshot and stores evidence.",
	}
}

func mapHandoffGuidance(url, kind string, opened bool) map[string]interface{} {
	label := "지도"
	if kind == "directions" {
		label = "길찾기"
	}
	return map[string]interface{}{
		"kind":               "meshclaw_map_handoff",
		"map_kind":           kind,
		"url":                url,
		"user_message":       fmt.Sprintf("%s 링크를 먼저 전달하고, 사용자가 사진/캡처를 원하면 화면을 열어 민감정보를 확인한 뒤 캡처 승인을 받아야 합니다.", label),
		"opened":             opened,
		"open_step":          "Use meshclaw_maps_search or meshclaw_maps_directions with execute=true to open the visible map UI.",
		"still_proof_tool":   "meshclaw_screen_capture",
		"video_proof_tool":   "meshclaw_screen_record",
		"approval_required":  true,
		"approval_note":      "Still screenshot proof requires meshclaw_screen_capture with execute=true and approve=true after the visible map is safe to capture.",
		"reply_rule":         "Return the clickable map URL even if screen proof is not captured.",
		"privacy_check":      []string{"Hide private chats, notifications, passwords, payment details, and unrelated windows before capture.", "Capture only the requested map or route."},
		"stop_before":        []string{"capturing a screen that contains sensitive visible data", "answering with coordinates only when a map URL is available"},
		"recommended_result": []string{"clickable Maps URL", "optional approved still screenshot proof", "optional short recording only when motion/context matters"},
	}
}

func runMapsProofMCP(args map[string]interface{}) map[string]interface{} {
	kind := strings.ToLower(strings.TrimSpace(stringArg(args, "kind")))
	if kind == "" {
		if strings.TrimSpace(stringArg(args, "destination")) != "" {
			kind = "directions"
		} else {
			kind = "place"
		}
	}
	var result osauto.Result
	switch kind {
	case "place", "search":
		kind = "place"
		result = osauto.MapsSearch(context.Background(), stringArg(args, "query"), stringArg(args, "provider"), false)
	default:
		kind = "directions"
		result = osauto.MapsDirections(context.Background(), stringArg(args, "origin"), stringArg(args, "destination"), stringArg(args, "mode"), stringArg(args, "provider"), false)
	}
	proof := map[string]interface{}{
		"kind":              "meshclaw_maps_proof",
		"map_kind":          kind,
		"result":            result,
		"url":               result.URL,
		"execute":           boolArg(args, "execute", false),
		"approved":          boolArg(args, "approve", false),
		"approval_required": true,
		"map_handoff":       mapHandoffGuidance(result.URL, kind, false),
		"user_message":      "지도 링크를 먼저 만들었습니다. 실제 화면 캡처는 지도를 열고 민감정보를 확인한 뒤 승인된 경우에만 실행합니다.",
	}
	if !result.OK {
		proof["status"] = "failed"
		proof["error"] = result.Error
		return proof
	}
	if !boolArg(args, "execute", false) {
		proof["status"] = "plan"
		return proof
	}
	if !boolArg(args, "approve", false) {
		proof["status"] = "approval_required"
		proof["approval_missing"] = true
		proof["user_message"] = "지도 캡처는 승인 없이는 실행하지 않습니다. 링크를 확인한 뒤 화면 캡처를 승인해야 합니다."
		return proof
	}
	opened := osauto.OpenURL(context.Background(), result.URL)
	proof["open_result"] = opened
	proof["map_handoff"] = mapHandoffGuidance(result.URL, kind, opened.OK)
	if !opened.OK {
		proof["status"] = "open_failed"
		proof["error"] = opened.Error
		return proof
	}
	waitSeconds := intArg(args, "wait_seconds", 2)
	if waitSeconds < 0 {
		waitSeconds = 0
	}
	if waitSeconds > 10 {
		waitSeconds = 10
	}
	if waitSeconds > 0 {
		time.Sleep(time.Duration(waitSeconds) * time.Second)
	}
	output := stringArg(args, "output")
	if strings.TrimSpace(output) == "" {
		output = defaultMCPScreenCapturePath("map-proof")
	}
	capture := osauto.ScreenCapture(context.Background(), output)
	proof["capture"] = capture
	proof["output"] = output
	proof["status"] = "captured"
	if !capture.OK {
		proof["status"] = "capture_failed"
		proof["error"] = capture.Error
	}
	return proof
}

func runPurchaseClickMCP(args map[string]interface{}) map[string]interface{} {
	plan := purchaseClickPlan(args)
	if !boolArg(args, "execute", false) {
		return plan
	}
	missing := purchaseClickMissingFields(args)
	if len(missing) > 0 || !boolArg(args, "approve", false) || !purchaseClickConfirmationAccepted(stringArg(args, "confirmation")) {
		plan["status"] = "approval_required"
		plan["approval_missing"] = true
		plan["missing"] = missing
		plan["user_message"] = "구매 실행은 필수 거래 정보와 강한 구매 확인 문구(예: 구매 실행 승인/사줘/주문해/ㅇㅇ), approve=true가 모두 있어야 합니다."
		return plan
	}
	plan["status"] = "executing"
	click := osauto.MouseClick(context.Background(), stringArg(args, "runner_url"), floatArg(args, "x", 0), floatArg(args, "y", 0))
	plan["click_result"] = click
	plan["status"] = "clicked"
	if !click.OK {
		plan["status"] = "click_failed"
		plan["error"] = click.Error
		return plan
	}
	if boolArg(args, "post_capture", false) {
		waitSeconds := intArg(args, "post_wait_seconds", 1)
		if waitSeconds < 0 {
			waitSeconds = 0
		}
		if waitSeconds > 10 {
			waitSeconds = 10
		}
		if waitSeconds > 0 {
			time.Sleep(time.Duration(waitSeconds) * time.Second)
		}
		output := strings.TrimSpace(stringArg(args, "post_capture_output"))
		if output == "" {
			output = defaultMCPScreenCapturePath("purchase-proof")
		}
		capture := osauto.ScreenCapture(context.Background(), output)
		plan["post_capture"] = capture
		plan["post_capture_output"] = output
	}
	return plan
}

func purchaseClickPlan(args map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"kind":                "meshclaw_purchase_click_plan",
		"action":              "purchase_click",
		"status":              "plan",
		"execute":             boolArg(args, "execute", false),
		"approval_required":   true,
		"strong_approval":     true,
		"merchant":            stringArg(args, "merchant"),
		"item":                stringArg(args, "item"),
		"total":               stringArg(args, "total"),
		"shipping":            stringArg(args, "shipping"),
		"payment":             stringArg(args, "payment"),
		"url":                 stringArg(args, "url"),
		"proof_path":          stringArg(args, "proof_path"),
		"post_capture":        boolArg(args, "post_capture", false),
		"post_capture_output": stringArg(args, "post_capture_output"),
		"click":               map[string]interface{}{"x": floatArg(args, "x", 0), "y": floatArg(args, "y", 0)},
		"confirmation":        stringArg(args, "confirmation"),
		"user_message":        "구매 실행은 금액, 판매자, 상품, 배송지, 결제수단, 최종 버튼 위치를 확인한 뒤 명시 승인으로만 실행합니다.",
		"approval_note":       "To execute, set execute=true, approve=true, and confirmation to a strong purchase confirmation such as '구매 실행 승인', '사줘', '주문해', 'ㅇㅇ', 'purchase execution approved', 'go ahead', or 'buy it'.",
		"stop_before":         []string{"clicking purchase without final total", "clicking purchase without confirmed shipping/payment", "clicking purchase without explicit approval", "guessing coordinates"},
		"allowed_action":      "one visible click on the supplied final purchase/order button coordinate",
		"scope_note":          "This tool does not search, choose products, edit payment methods, change addresses, or bypass site confirmations. It only performs a final approved visible click.",
	}
}

func purchaseClickMissingFields(args map[string]interface{}) []string {
	missing := []string{}
	for _, key := range []string{"merchant", "item", "total", "shipping", "payment", "url"} {
		if strings.TrimSpace(stringArg(args, key)) == "" {
			missing = append(missing, key)
		}
	}
	if floatArg(args, "x", 0) <= 0 {
		missing = append(missing, "x")
	}
	if floatArg(args, "y", 0) <= 0 {
		missing = append(missing, "y")
	}
	if !purchaseClickConfirmationAccepted(stringArg(args, "confirmation")) {
		missing = append(missing, "confirmation")
	}
	return missing
}

func purchaseClickConfirmationAccepted(value string) bool {
	normalized := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), " "))
	compact := strings.NewReplacer(" ", "", "\t", "", "\n", "", ".", "", "!", "", "?", "").Replace(normalized)
	if compact == "구매실행승인" || compact == "주문실행승인" || compact == "결제실행승인" ||
		compact == "purchaseexecutionapproved" || compact == "orderexecutionapproved" || compact == "paymentexecutionapproved" {
		return true
	}
	short := map[string]bool{
		"ㅇㅇ": true, "응": true, "어": true, "그래": true, "오케이": true, "확인": true, "좋아": true, "사": true,
		"yes": true, "y": true, "ok": true, "okay": true, "approved": true,
	}
	if short[normalized] {
		return true
	}
	return containsAny(normalized,
		"사줘", "구매해", "구매해줘", "주문해", "주문해줘", "결제해", "결제해줘", "바로 구매 클릭", "바로 주문 클릭",
		"go ahead", "buy it", "buy now", "order it", "order now", "place the order", "complete checkout", "complete the purchase", "click purchase", "click the order",
	)
}

func mediaPlayPlan(query, source string) map[string]interface{} {
	query = strings.TrimSpace(query)
	source = strings.ToLower(strings.TrimSpace(source))
	if source == "" || source == "auto" {
		source = "youtube"
	}
	target := mediaPlayTarget(query, source)
	return map[string]interface{}{
		"kind":              "meshclaw_media_play_plan",
		"action":            "media_play",
		"query":             query,
		"source":            source,
		"target":            target,
		"execute":           false,
		"approval_required": true,
		"user_message":      "음악이나 영상을 바로 재생하기 전에 재생 표면을 고르고, 소리가 나는 작업임을 확인해야 합니다.",
		"source_choices": []map[string]string{
			{"source": "youtube", "behavior": "Open a YouTube search page in the browser."},
			{"source": "spotify", "behavior": "Open a Spotify search page in the browser/app if installed."},
			{"source": "music", "behavior": "Open the Apple Music app only."},
			{"source": "radio", "behavior": "Open a web search for the requested radio station."},
			{"source": "podcast", "behavior": "Open a web search for the requested podcast."},
		},
		"approval_note": "execute=true also requires approve=true. This only opens a visible app or URL; it does not click play, purchase, subscribe, change accounts, or alter recommendations.",
		"next": []string{
			"Confirm source and whether audible playback is appropriate now.",
			"Open the selected source with execute=true and approve=true if the user approves.",
			"Use screen proof only when the user asks to see the media page.",
		},
	}
}

func mediaPlayTarget(query, source string) string {
	escaped := url.QueryEscape(strings.TrimSpace(query))
	switch source {
	case "music", "apple", "apple_music", "apple-music":
		return "app:Music"
	case "spotify":
		if escaped == "" {
			return "https://open.spotify.com/"
		}
		return "https://open.spotify.com/search/" + escaped
	case "radio":
		if escaped == "" {
			escaped = "radio"
		}
		return "https://www.google.com/search?q=" + escaped + "+radio"
	case "podcast":
		if escaped == "" {
			escaped = "podcast"
		}
		return "https://www.google.com/search?q=" + escaped + "+podcast"
	default:
		if escaped == "" {
			return "https://www.youtube.com/"
		}
		return "https://www.youtube.com/results?search_query=" + escaped
	}
}

func audioTranscribePlanMap(plan audio.TranscribePlan) map[string]interface{} {
	var out map[string]interface{}
	data, err := json.Marshal(plan)
	if err != nil {
		return map[string]interface{}{"kind": plan.Kind, "action": plan.Action, "status": plan.Status}
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return map[string]interface{}{"kind": plan.Kind, "action": plan.Action, "status": plan.Status}
	}
	return out
}

func appSettingsPlan(app, pane, rawURL string) map[string]interface{} {
	app = strings.TrimSpace(app)
	pane = strings.ToLower(strings.TrimSpace(pane))
	target := strings.TrimSpace(rawURL)
	if target == "" {
		target = appSettingsTarget(app, pane)
	}
	return map[string]interface{}{
		"kind":              "meshclaw_app_settings_plan",
		"action":            "app_settings_preflight",
		"app":               app,
		"pane":              pane,
		"target":            target,
		"execute":           false,
		"approval_required": true,
		"user_message":      "설정 화면은 열어둘 수 있지만, 토글 변경/저장/로그아웃/계정 변경은 실행 전에 따로 확인해야 합니다.",
		"allowed": []string{
			"Open a visible app, System Settings pane, or settings URL.",
			"Show the user where the setting is and optionally capture privacy-checked screen proof.",
			"Prepare instructions without applying changes.",
		},
		"stop_before": []string{
			"permission toggle changes",
			"saving settings",
			"sign out",
			"account connection or disconnection",
			"data deletion",
			"password, MFA, or recovery setting changes",
		},
		"approval_note": "execute=true also requires approve=true and only opens the settings surface. Any actual setting/account/permission change needs a separate explicit approval.",
		"next": []string{
			"Open the settings surface only if the user approves visible handoff.",
			"Do not toggle, save, sign out, connect accounts, or delete data without a separate approval.",
			"Use screen proof only when the user asks to see or review the page.",
		},
	}
}

func appSettingsTarget(app, pane string) string {
	switch pane {
	case "accessibility", "손쉬운 사용":
		return "x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility"
	case "screen_recording", "screen-recording", "screen", "화면 기록":
		return "x-apple.systempreferences:com.apple.preference.security?Privacy_ScreenCapture"
	case "full_disk_access", "full-disk-access", "disk", "디스크":
		return "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles"
	case "notifications", "notification", "알림":
		return "x-apple.systempreferences:com.apple.Notifications-Settings.extension"
	case "privacy", "개인정보":
		return "x-apple.systempreferences:com.apple.preference.security"
	case "accounts", "account", "계정":
		return "x-apple.systempreferences:com.apple.preferences.AppleIDPrefPane"
	}
	lowerApp := strings.ToLower(strings.TrimSpace(app))
	if lowerApp == "" || lowerApp == "system settings" || lowerApp == "settings" || lowerApp == "시스템 설정" || lowerApp == "설정" {
		return "x-apple.systempreferences:"
	}
	return "app:" + strings.TrimSpace(app)
}

func accountActionPlan(service, action, rawURL string) map[string]interface{} {
	service = strings.TrimSpace(service)
	action = strings.ToLower(strings.TrimSpace(action))
	if action == "" || action == "auto" {
		action = "manage"
	}
	target := strings.TrimSpace(rawURL)
	if target == "" {
		query := strings.TrimSpace(service + " " + accountActionSearchPhrase(action))
		target = "https://www.google.com/search?q=" + url.QueryEscape(query)
	}
	return map[string]interface{}{
		"kind":              "meshclaw_account_action_plan",
		"action":            "account_action_preflight",
		"service":           service,
		"requested_action":  action,
		"target":            target,
		"execute":           false,
		"approval_required": true,
		"user_message":      "계정이나 구독 화면은 열어둘 수 있지만, 결제/해지/요금제 변경/제출은 마지막 단계 전에 멈추고 확인을 받아야 합니다.",
		"allowed": []string{
			"Open a visible account, billing, or subscription management page.",
			"Summarize the visible counterparty, plan, price, renewal date, or cancellation terms if the user provides them or asks for screen proof.",
			"Prepare a draft action path without submitting it.",
		},
		"stop_before": []string{
			"final cancellation",
			"subscription change",
			"payment or purchase",
			"form submission",
			"account deletion",
			"password, MFA, or recovery setting changes",
		},
		"approval_note": "execute=true also requires approve=true and only opens the target URL. Any final account/billing/subscription action needs a separate explicit approval after summarizing service, action, price/terms, and consequence.",
		"next": []string{
			"Open the management page only if the user approves visible account handoff.",
			"Capture privacy-checked screen proof if the user asks to see or review the page.",
			"Before any final action, summarize the service, action, cost/terms, and consequence, then ask for explicit approval.",
		},
	}
}

func accountActionSearchPhrase(action string) string {
	switch action {
	case "cancel", "cancellation", "해지", "취소":
		return "subscription cancel account management"
	case "billing", "payment", "invoice", "결제":
		return "billing subscription account"
	case "renew", "renewal", "갱신":
		return "renewal subscription account"
	case "plan_change", "plan-change", "plan", "요금제":
		return "change plan subscription account"
	case "profile", "settings", "설정":
		return "account settings"
	default:
		return "subscription account management"
	}
}

func defaultMCPRecordingPath(prefix string) string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		home = "."
	}
	return filepath.Join(home, ".meshclaw", "doctor", fmt.Sprintf("%s-%s.mov", prefix, time.Now().UTC().Format("20060102T150405Z")))
}

func defaultMCPScreenCapturePath(prefix string) string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		home = "."
	}
	return filepath.Join(home, ".meshclaw", "doctor", fmt.Sprintf("%s-%s.png", prefix, time.Now().UTC().Format("20060102T150405Z")))
}

func summarizeMailSearch(result mailadapter.SearchResult, query string) string {
	scope := "recent mail"
	if strings.TrimSpace(query) != "" {
		scope = "mail matching " + strings.TrimSpace(query)
	}
	if len(result.Messages) == 0 {
		return fmt.Sprintf("No %s found.", scope)
	}
	parts := []string{fmt.Sprintf("%d %s item(s)", len(result.Messages), scope)}
	for i, message := range result.Messages {
		if i >= 5 {
			parts = append(parts, fmt.Sprintf("and %d more", len(result.Messages)-i))
			break
		}
		item := strings.TrimSpace(message.Subject)
		if item == "" {
			item = "(no subject)"
		}
		if strings.TrimSpace(message.From) != "" {
			item += " from " + strings.TrimSpace(message.From)
		}
		parts = append(parts, item)
	}
	return strings.Join(parts, "; ")
}

type mcpMailMultiSearchResult struct {
	Kind        string                      `json:"kind"`
	Query       string                      `json:"query,omitempty"`
	Since       time.Time                   `json:"since,omitempty"`
	Limit       int                         `json:"limit"`
	Results     []mailadapter.SearchResult  `json:"results"`
	Errors      []mcpMailAccountSearchError `json:"errors,omitempty"`
	GeneratedAt time.Time                   `json:"generated_at"`
}

type mcpMailAccountSearchError struct {
	Account string `json:"account"`
	Error   string `json:"error"`
}

func mcpMailSearchAll(query string, since time.Time, limit int) (mcpMailMultiSearchResult, error) {
	if limit <= 0 || limit > 50 {
		limit = 10
	}
	public, err := mailadapter.ListAccounts()
	result := mcpMailMultiSearchResult{
		Kind:        "meshclaw_mail_search_all",
		Query:       strings.TrimSpace(query),
		Since:       since,
		Limit:       limit,
		Results:     []mailadapter.SearchResult{},
		GeneratedAt: time.Now().UTC(),
	}
	if err != nil {
		return result, err
	}
	if len(public.Accounts) == 0 {
		return result, fmt.Errorf("no mail accounts are configured")
	}
	for _, account := range public.Accounts {
		if strings.TrimSpace(account.ID) == "" {
			continue
		}
		search, searchErr := mailadapter.Search(mailadapter.SearchOptions{
			Account: account.ID,
			Query:   query,
			Since:   since,
			Limit:   limit,
		})
		if searchErr != nil {
			result.Errors = append(result.Errors, mcpMailAccountSearchError{Account: account.ID, Error: searchErr.Error()})
			continue
		}
		result.Results = append(result.Results, search)
	}
	if len(result.Results) == 0 && len(result.Errors) > 0 {
		return result, fmt.Errorf("mail search failed for all configured accounts")
	}
	return result, nil
}

func summarizeMailMultiSearch(result mcpMailMultiSearchResult, query string) string {
	total := 0
	for _, accountResult := range result.Results {
		total += len(accountResult.Messages)
	}
	scope := "recent mail"
	if strings.TrimSpace(query) != "" {
		scope = "mail matching " + strings.TrimSpace(query)
	}
	parts := []string{fmt.Sprintf("%d %s item(s) across %d account(s), %d error(s)", total, scope, len(result.Results), len(result.Errors))}
	for _, accountResult := range result.Results {
		accountID := strings.TrimSpace(accountResult.Account.ID)
		if accountID == "" {
			accountID = "account"
		}
		parts = append(parts, fmt.Sprintf("[%s] %d item(s)", accountID, len(accountResult.Messages)))
		for i, message := range accountResult.Messages {
			if i >= 3 {
				parts = append(parts, fmt.Sprintf("[%s] and %d more", accountID, len(accountResult.Messages)-i))
				break
			}
			item := strings.TrimSpace(message.Subject)
			if item == "" {
				item = "(no subject)"
			}
			if strings.TrimSpace(message.From) != "" {
				item += " from " + strings.TrimSpace(message.From)
			}
			parts = append(parts, "["+accountID+"] "+item)
		}
	}
	return strings.Join(parts, "; ")
}

func mcpVSSHJobRead(command, action string, args map[string]interface{}) (interface{}, error) {
	host := stringArg(args, "host")
	id := stringArg(args, "id")
	decision := policy.Evaluate(policy.Request{Subject: "mcp", Action: action, Resource: "server", Context: id})
	if decision.Decision != policy.Allow {
		return map[string]interface{}{"policy": decision, "success": false}, nil
	}
	result, runErr := runtime.NewRunner().VSSHJSON(command, host, id)
	record, storeErr := evidence.Store(command, host, id, result)
	return map[string]interface{}{"policy": decision, "result": result, "evidence": record, "store_error": errString(storeErr), "run_error": errString(runErr)}, nil
}

func publicationTarget(args map[string]interface{}) string {
	target := strings.ToLower(strings.TrimSpace(stringArg(args, "target")))
	if target == "" || target == "auto" {
		target = "macmini"
	}
	if target == "macmini" && isMacminiRuntime() {
		return "local"
	}
	return target
}

func isMacminiRuntime() bool {
	home, _ := os.UserHomeDir()
	if home == "/Users/argos" {
		return true
	}
	host, _ := os.Hostname()
	return strings.EqualFold(host, "macmini")
}

func cloneMCPArgs(args map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(args)+1)
	for key, value := range args {
		out[key] = value
	}
	return out
}

func ensureMacBookMissionWrite() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if !canExposeMissionWriteTools() {
		return fmt.Errorf("mission write tools are MacBook-canonical only; refusing to write Core mission state under %s", home)
	}
	return nil
}

func canExposeMissionWriteTools() bool {
	home, err := os.UserHomeDir()
	return err == nil && filepath.Clean(home) == "/Users/example"
}

func missionMutationResult(kind string, doc mission.Mission, path string, extra map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{
		"kind":       kind,
		"mission":    doc,
		"summary":    doc.Summary(),
		"path":       path,
		"canonical":  "macbook",
		"scope_note": "Mission writes update only the MacBook canonical Core store. macmini Signal read/sync remains a separate Phase 1.5/2 design.",
	}
	for key, value := range extra {
		result[key] = value
	}
	return result
}

func recordPublicationArtifact(args map[string]interface{}, target, kind, title, path, link string, record evidence.Record) map[string]interface{} {
	if !boolArg(args, "record_artifact", true) {
		return map[string]interface{}{"recorded": false, "skipped": true, "reason": "record_artifact=false"}
	}
	if strings.TrimSpace(path) == "" {
		return map[string]interface{}{"recorded": false, "error": "publication did not return a document path"}
	}
	if err := ensureMacBookMissionWrite(); err != nil {
		return map[string]interface{}{"recorded": false, "error": err.Error()}
	}
	ref := strings.TrimSpace(path)
	node := strings.TrimSpace(target)
	if node == "" {
		node = "local"
	}
	if node != "local" && !strings.Contains(ref, ":/") {
		ref = node + ":" + ref
	}
	artifact := map[string]interface{}{
		"kind":     firstNonEmpty(kind, "doc"),
		"ref":      ref,
		"title":    strings.TrimSpace(title),
		"node":     node,
		"evidence": record.ID,
	}
	if strings.TrimSpace(link) != "" {
		artifact["notes"] = "link: " + strings.TrimSpace(link)
	}
	store, err := mission.DefaultStore()
	if err != nil {
		return map[string]interface{}{"recorded": false, "error": err.Error()}
	}
	doc, added, missionPath, err := store.AddArtifact(stringArg(args, "mission_id"), artifact)
	if err != nil {
		return map[string]interface{}{"recorded": false, "error": err.Error(), "ref": ref, "node": node}
	}
	return map[string]interface{}{
		"recorded":        true,
		"artifact":        added,
		"mission_summary": doc.Summary(),
		"mission_path":    missionPath,
	}
}

func remotePayload(remote map[string]interface{}) map[string]interface{} {
	if payload, ok := remote["payload"].(map[string]interface{}); ok {
		return payload
	}
	return nil
}

func nestedString(parent map[string]interface{}, objectKey, fieldKey string) string {
	if parent == nil {
		return ""
	}
	child, ok := parent[objectKey].(map[string]interface{})
	if !ok {
		return ""
	}
	return stringFromInterface(child[fieldKey])
}

func firstRemoteLink(payload map[string]interface{}) string {
	if payload == nil {
		return ""
	}
	if links, ok := payload["links"].([]interface{}); ok {
		for _, link := range links {
			if value := strings.TrimSpace(stringFromInterface(link)); value != "" {
				return value
			}
		}
	}
	if report, ok := payload["report"].(map[string]interface{}); ok {
		if links, ok := report["links"].([]interface{}); ok {
			for _, link := range links {
				if value := strings.TrimSpace(stringFromInterface(link)); value != "" {
					return value
				}
			}
		}
	}
	return ""
}

func firstString(values []string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func stringFromInterface(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		return ""
	}
}

func callRemoteMCPTool(target, name string, args map[string]interface{}, timeout time.Duration) (map[string]interface{}, error) {
	params := map[string]interface{}{
		"name":      name,
		"arguments": args,
	}
	rawParams, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	req := mcpRequest{JSONRPC: "2.0", Method: "tools/call", Params: rawParams, ID: 1}
	rawReq, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	binary := remoteMeshClawBinary(target)
	encoded := base64.StdEncoding.EncodeToString(rawReq)
	script := fmt.Sprintf("printf %%s '%s' | base64 -d | %s mcp\n", encoded, shellQuote(binary))

	runner := runtime.NewRunner()
	runner.Timeout = timeout
	result := runner.RunEvidence(target, script)
	remote := map[string]interface{}{
		"target":      target,
		"binary":      binary,
		"transport":   result.Transport,
		"success":     result.Success,
		"duration_ms": result.DurationMs,
		"stdout":      result.Stdout,
		"stderr":      result.Stderr,
		"exit_code":   result.ExitCode,
	}
	if !result.Success {
		return remote, fmt.Errorf("remote MCP call failed on %s via %s", target, result.Transport)
	}
	resp, payload, err := parseRemoteMCPResponse(result.Stdout)
	remote["response"] = resp
	if payload != nil {
		remote["payload"] = payload
	}
	if err != nil {
		return remote, err
	}
	if resp.Error != nil {
		return remote, fmt.Errorf("remote MCP error: %s", resp.Error.Message)
	}
	return remote, nil
}

func remoteMeshClawBinary(target string) string {
	envKey := "MESHCLAW_REMOTE_MESHCLAW_BINARY"
	if target != "" {
		targetKey := "MESHCLAW_REMOTE_MESHCLAW_BINARY_" + strings.ToUpper(strings.NewReplacer("-", "_", ".", "_").Replace(target))
		if value := strings.TrimSpace(os.Getenv(targetKey)); value != "" {
			return value
		}
	}
	if value := strings.TrimSpace(os.Getenv(envKey)); value != "" {
		return value
	}
	if target == "macmini" {
		return "/Users/argos/bin/meshclaw"
	}
	return "meshclaw"
}

func parseRemoteMCPResponse(stdout string) (mcpResponse, map[string]interface{}, error) {
	var resp mcpResponse
	line := lastNonEmptyLine(stdout)
	if line == "" {
		return resp, nil, fmt.Errorf("remote MCP returned empty stdout")
	}
	if err := json.Unmarshal([]byte(line), &resp); err != nil {
		return resp, nil, fmt.Errorf("invalid remote MCP response: %w", err)
	}
	if resp.Error != nil {
		return resp, nil, nil
	}
	text := mcpContentText(resp.Result)
	if text == "" {
		return resp, nil, fmt.Errorf("remote MCP response missing text content")
	}
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		return resp, nil, fmt.Errorf("invalid remote MCP payload: %w", err)
	}
	return resp, payload, nil
}

func lastNonEmptyLine(text string) string {
	lines := strings.Split(text, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" {
			return line
		}
	}
	return ""
}

func mcpContentText(result interface{}) string {
	resultMap, ok := result.(map[string]interface{})
	if !ok {
		return ""
	}
	content, ok := resultMap["content"].([]interface{})
	if !ok || len(content) == 0 {
		return ""
	}
	first, ok := content[0].(map[string]interface{})
	if !ok {
		return ""
	}
	text, _ := first["text"].(string)
	return text
}

func stringArg(args map[string]interface{}, key string) string {
	if value, ok := args[key].(string); ok {
		return value
	}
	return ""
}

func intArg(args map[string]interface{}, key string, fallback int) int {
	switch value := args[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return fallback
	}
}

func boolArg(args map[string]interface{}, key string, fallback bool) bool {
	if value, ok := args[key].(bool); ok {
		return value
	}
	return fallback
}

func buildServiceRegistryPlan(args map[string]interface{}) map[string]interface{} {
	service := strings.TrimSpace(stringArg(args, "service"))
	if service == "" {
		service = "<service>"
	}
	scope := strings.TrimSpace(stringArg(args, "scope"))
	if scope == "" {
		scope = "all"
	}
	port := intArg(args, "port", 0)
	endpoints := splitCSV(stringArg(args, "endpoints"))
	endpointChecks := make([]map[string]interface{}, 0, len(endpoints))
	for _, endpoint := range endpoints {
		endpointChecks = append(endpointChecks, map[string]interface{}{
			"endpoint": endpoint,
			"checks":   []string{"tcp connect", "http health path if configured", "recent container/service health evidence"},
			"status":   "needs_evidence",
		})
	}
	if len(endpointChecks) == 0 {
		endpointChecks = append(endpointChecks, map[string]interface{}{
			"endpoint": "<discovered-from-inventory>",
			"checks":   []string{"inventory role/tag match", "listener/port evidence", "health endpoint evidence"},
			"status":   "needs_discovery",
		})
	}
	return map[string]interface{}{
		"kind":    "service_registry_plan",
		"service": service,
		"scope":   scope,
		"port":    port,
		"summary": fmt.Sprintf("plan-only service registry for %s scope=%s endpoints=%d", service, scope, len(endpointChecks)),
		"evidence": []string{
			"meshclaw_server_list or inventory override state",
			"meshclaw_monitor_check or cached agent workload reports",
			"container/service/log evidence for each endpoint before routing traffic",
		},
		"signals": []string{
			"endpoint candidates must be proven healthy before inclusion",
			"routing changes require approval and rollback evidence",
			"do not infer load balancer membership from chat history alone",
		},
		"endpoint_checks": endpointChecks,
		"route_plan": []map[string]interface{}{
			{"step": 1, "action": "discover endpoint candidates from inventory, listeners, containers, and desired-state labels"},
			{"step": 2, "action": "verify endpoint health with read-only checks and recent log evidence"},
			{"step": 3, "action": "rank primary and fallback endpoints by health, role, latency, and operator overrides"},
			{"step": 4, "action": "render proxy/DNS/firewall change preview only after approval evidence exists"},
		},
		"approval_gates": []string{
			"operator approval before proxy, DNS, firewall, or container mutation",
			"rollback target and previous routing config evidence required before apply",
		},
		"rollback": []string{
			"restore previous route/proxy config from evidence",
			"remove newly added unhealthy endpoints",
			"verify traffic returns to the previous healthy endpoint set",
		},
	}
}

func buildCapacityScalePlan(args map[string]interface{}) map[string]interface{} {
	workload := strings.TrimSpace(stringArg(args, "workload"))
	if workload == "" {
		workload = "<workload>"
	}
	scope := strings.TrimSpace(stringArg(args, "scope"))
	if scope == "" {
		scope = "all"
	}
	target := strings.TrimSpace(stringArg(args, "target"))
	if target == "" {
		target = "<desired-state-or-slo>"
	}
	constraints := splitCSV(stringArg(args, "constraints"))
	return map[string]interface{}{
		"kind":        "capacity_scale_plan",
		"workload":    workload,
		"scope":       scope,
		"target":      target,
		"budget_usd":  floatArg(args, "budget_usd", 0),
		"ttl_hours":   intArg(args, "ttl_hours", 6),
		"constraints": constraints,
		"summary":     fmt.Sprintf("plan-only capacity scale plan for %s scope=%s target=%s", workload, scope, target),
		"evidence": []string{
			"meshclaw_monitor_check metrics for CPU, memory, disk, GPU, and network pressure",
			"meshclaw_agent_workloads process/container purpose and resource usage",
			"meshclaw_agent_security public exposure and firewall constraints",
			"meshclaw_capability_recommend and meshclaw_placement_plan before any move or provisioning",
		},
		"signals": []string{
			"sustained saturation over multiple samples is stronger than one spike",
			"scale-out needs healthy endpoint and service-registry evidence before traffic shift",
			"scale-down needs idle evidence and rollback capacity",
			"provider cost changes require a separate provision plan and operator approval",
		},
		"scale_options": []map[string]interface{}{
			{"id": "rebalance", "action": "move or place workload on an existing healthier node", "risk": "medium", "requires_approval": true},
			{"id": "scale_out_existing", "action": "add an existing healthy endpoint to service registry/LB-lite plan", "risk": "medium", "requires_approval": true},
			{"id": "temporary_capacity", "action": "prepare a separate provision plan for temporary VPS/GPU capacity", "risk": "high", "requires_approval": true},
			{"id": "scale_down", "action": "remove idle endpoint only after readiness, drain, and rollback evidence", "risk": "high", "requires_approval": true},
		},
		"approval_gates": []string{
			"operator approval before provider spend, workload migration, proxy/DNS changes, or container/service mutation",
			"rollback target and previous placement/routing evidence required before apply",
			"post-action verification plan required before declaring completion",
		},
		"rollback": []string{
			"restore previous placement and service registry membership",
			"remove temporary capacity only after traffic drain evidence",
			"return DNS/proxy route to the last healthy endpoint set",
		},
	}
}

func buildStorageGuardrailPlan(args map[string]interface{}) map[string]interface{} {
	node := strings.TrimSpace(stringArg(args, "node"))
	if node == "" {
		node = "all"
	}
	path := strings.TrimSpace(stringArg(args, "path"))
	if path == "" {
		path = "/"
	}
	workload := strings.TrimSpace(stringArg(args, "workload"))
	risk := strings.TrimSpace(stringArg(args, "risk"))
	if risk == "" {
		risk = "disk_pressure"
	}
	mountType := strings.TrimSpace(stringArg(args, "mount_type"))
	retention := strings.TrimSpace(stringArg(args, "retention"))
	backupRequired := boolArg(args, "backup", true)
	return map[string]interface{}{
		"kind":            "storage_guardrail_plan",
		"node":            node,
		"path":            path,
		"workload":        workload,
		"risk":            risk,
		"mount_type":      mountType,
		"retention":       retention,
		"backup_required": backupRequired,
		"summary":         fmt.Sprintf("plan-only storage guardrail for %s:%s risk=%s backup=%t", node, path, risk, backupRequired),
		"evidence": []string{
			"meshclaw_disk_investigate evidence for capacity, inode, and largest paths",
			"mount/listener/container dependency evidence before changing mounts or volumes",
			"backup or snapshot evidence before deletion, migration, resize, or retention changes",
			"post-action verification plan before declaring storage risk resolved",
		},
		"signals": []string{
			"disk pressure should distinguish reclaimable data from workload-critical data",
			"missing mount may be a dependency outage, not local data loss",
			"backup recency and restore target matter more than backup existence alone",
			"retention cleanup must preserve final outputs, state, and newest evidence",
		},
		"guardrail_options": []map[string]interface{}{
			{"id": "investigate", "action": "collect read-only disk, inode, mount, and workload dependency evidence", "risk": "low", "requires_approval": false},
			{"id": "archive_plan", "action": "plan evidence/archive retention without moving or deleting files", "risk": "low", "requires_approval": false},
			{"id": "cleanup_plan", "action": "build a manifest of raw/intermediate cleanup candidates", "risk": "medium", "requires_approval": true},
			{"id": "backup_gate", "action": "require snapshot/backup evidence before mutation", "risk": "medium", "requires_approval": true},
			{"id": "mount_or_volume_change", "action": "prepare a separate desired-state/runbook path for mount, volume, or resize changes", "risk": "high", "requires_approval": true},
		},
		"approval_gates": []string{
			"operator approval before deletion, mount edits, volume migration, resize, snapshot writes, or backup writes",
			"restore target and backup freshness evidence required before destructive cleanup",
			"rollback path required before any storage topology change",
		},
		"rollback": []string{
			"restore from verified backup or snapshot target",
			"revert mount/volume mapping to previous evidence",
			"stop traffic or workload writes before rollback when consistency requires it",
			"verify workload health and disk pressure after rollback",
		},
	}
}

func buildOpsIntegrationPlan(args map[string]interface{}) map[string]interface{} {
	tools := splitCSV(stringArg(args, "tools"))
	if len(tools) == 0 {
		tools = []string{"prometheus", "grafana", "loki", "ansible"}
	}
	goal := strings.TrimSpace(stringArg(args, "goal"))
	if goal == "" {
		goal = "chat-driven operations"
	}
	scope := strings.TrimSpace(stringArg(args, "scope"))
	if scope == "" {
		scope = "fleet"
	}
	readonly := boolArg(args, "readonly", true)
	sources := make([]map[string]interface{}, 0, len(tools))
	for _, tool := range tools {
		normalizedTool := strings.ToLower(strings.TrimSpace(tool))
		sources = append(sources, map[string]interface{}{
			"tool":              normalizedTool,
			"semantic_role":     opsIntegrationRole(normalizedTool),
			"meshclaw_surface":  opsIntegrationSurface(normalizedTool),
			"first_phase":       "read-only evidence import",
			"automation_status": "plan-only",
		})
	}
	return map[string]interface{}{
		"kind":             "ops_integration_plan",
		"tools":            tools,
		"goal":             goal,
		"scope":            scope,
		"readonly":         readonly,
		"summary":          fmt.Sprintf("plan-only ops integration for %d tools scope=%s readonly=%t", len(tools), scope, readonly),
		"evidence_sources": sources,
		"semantic_contract": []string{
			"translate external tool output into MeshClaw evidence, signals, interpretation, likely_causes, recommended_checks, remediation_options, and rollback",
			"prefer semantic MCP tools over raw API calls exposed directly to models",
			"store redacted evidence before asking a model to reason about incidents",
			"separate read-only observation from approval-gated automation setup",
		},
		"integration_steps": []map[string]interface{}{
			{"step": 1, "action": "inventory existing endpoints, tokens, dashboards, alert rules, and permissions without exposing secrets"},
			{"step": 2, "action": "map each external signal to a MeshClaw semantic surface and evidence schema"},
			{"step": 3, "action": "add read-only doctor/import checks with redaction and freshness metadata"},
			{"step": 4, "action": "wire approval-gated automation only after evidence import and rollback paths are tested"},
		},
		"approval_gates": []string{
			"operator approval before writing Grafana/Loki/Prometheus/Portainer/Ansible/notification configuration",
			"secret handles only; no raw API tokens or passwords in model-visible output",
			"rollback path required before enabling automated remediation or notification fan-out",
		},
		"rollback": []string{
			"disable newly added automation rules or notification targets",
			"revert integration config to previous evidence snapshot",
			"fall back to MeshClaw local monitor/logscan evidence if external tool import fails",
		},
	}
}

func opsIntegrationRole(tool string) string {
	switch tool {
	case "prometheus", "grafana":
		return "metrics and dashboard evidence"
	case "loki", "opensearch", "elk":
		return "log evidence and pattern search"
	case "portainer":
		return "docker/container inventory and action planning"
	case "ansible":
		return "approval-gated automation executor"
	case "uptime kuma", "uptime-kuma":
		return "synthetic uptime and endpoint checks"
	case "ntfy", "gotify", "telegram":
		return "notification delivery"
	case "tailscale", "wireguard":
		return "network reachability and route context"
	case "cockpit":
		return "host management context"
	default:
		return "external operational evidence source"
	}
}

func opsIntegrationSurface(tool string) string {
	switch tool {
	case "prometheus", "grafana":
		return "meshclaw_monitor_check"
	case "loki", "opensearch", "elk":
		return "meshclaw_analyze_logs"
	case "portainer":
		return "meshclaw_autoheal_plan"
	case "ansible":
		return "meshclaw_workflow_run"
	case "uptime kuma", "uptime-kuma":
		return "meshclaw_service_registry_plan"
	case "ntfy", "gotify", "telegram":
		return "meshclaw_messenger_report"
	case "tailscale", "wireguard":
		return "meshclaw_server_list"
	case "cockpit":
		return "meshclaw_node_inventory"
	default:
		return "meshclaw_ops_control"
	}
}

func defaultMCPRolloutExpectedTools() []string {
	return []string{
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
	}
}

func defaultMCPSmokeTestTools() []string {
	return []string{
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
		"meshclaw_mcp_rollout_plan",
	}
}

func mcpProfileVisibilityPreview(profile string, expectedTools []string) map[string]interface{} {
	return buildMCPProfileVisibilityCheck(map[string]interface{}{
		"profile":        profile,
		"expected_tools": strings.Join(expectedTools, ","),
	})
}

func mcpRolloutVisibilityProfile(client string, expectedTools []string) string {
	profile := normalizeMCPProfile(client)
	if profile == "all" {
		return "all"
	}
	for _, tool := range expectedTools {
		if !mcpProfileAllowsTool("claude-lite", tool) && mcpProfileAllowsTool("all", tool) {
			return "all"
		}
	}
	return "claude-lite"
}

func buildMCPRolloutPlan(args map[string]interface{}) map[string]interface{} {
	client := strings.TrimSpace(stringArg(args, "client"))
	if client == "" {
		client = "claude"
	}
	branch := strings.TrimSpace(stringArg(args, "branch"))
	if branch == "" {
		branch = "<merged-or-local-branch>"
	}
	expectedTools := splitCSV(stringArg(args, "expected_tools"))
	if len(expectedTools) == 0 {
		expectedTools = defaultMCPRolloutExpectedTools()
	}
	visibilityProfile := mcpRolloutVisibilityProfile(client, expectedTools)
	return map[string]interface{}{
		"kind":                          "mcp_rollout_plan",
		"client":                        client,
		"branch":                        branch,
		"expected_tools":                expectedTools,
		"visibility_profile":            visibilityProfile,
		"profile_visibility":            mcpProfileVisibilityPreview(visibilityProfile, expectedTools),
		"rollout_readiness":             mcpRolloutReadiness(client, branch, expectedTools),
		"rollout_troubleshooting":       mcpRolloutTroubleshooting(expectedTools),
		"rollout_evidence_checklist":    mcpRolloutEvidenceChecklist(client, branch, expectedTools),
		"rollout_success_criteria":      mcpRolloutSuccessCriteria(expectedTools),
		"operator_handoff":              mcpRolloutOperatorHandoff(client, branch),
		"local_verification_steps":      mcpRolloutLocalVerificationSteps(visibilityProfile, branch, expectedTools),
		"stack_review_discipline":       mcpRolloutStackReviewDiscipline(branch),
		"rollback_decision_points":      mcpRolloutRollbackDecisionPoints(expectedTools),
		"rollout_summary_counts":        mcpRolloutSummaryCounts(expectedTools),
		"rollout_readiness_labels":      mcpRolloutReadinessLabels(),
		"client_refresh_matrix":         mcpRolloutClientRefreshMatrix(client),
		"operator_acceptance_checklist": mcpRolloutOperatorAcceptanceChecklist(expectedTools),
		"operator_brief":                mcpRolloutOperatorBrief(client, branch, expectedTools),
		"copy_paste_status":             mcpRolloutCopyPasteStatus(client, branch, expectedTools),
		"rollout_risk_register":         mcpRolloutRiskRegister(),
		"command_compatibility":         mcpRolloutCommandCompatibility(client),
		"post_reload_smoke_prompts":     mcpRolloutSmokePrompts(expectedTools),
		"operator_checkpoints": []map[string]interface{}{
			{"phase": "source", "status": "required", "check": "branch or PR is merged, or the operator intentionally builds from this branch", "evidence": branch},
			{"phase": "binary", "status": "required", "check": "a MeshClaw binary is built from the selected source and the MCP client points at that binary", "mutates_live_servers": false},
			{"phase": "client_reload", "status": "required", "check": "the MCP client/server process is refreshed after selecting the new binary", "reversible": true},
			{"phase": "visibility", "status": "required", "check": "profile visibility reports zero missing expected tools before Claude is asked to use them", "tool": "meshclaw_mcp_profile_visibility_check"},
			{"phase": "smoke", "status": "required", "check": "read-only smoke checks complete before any approval-gated apply path", "tool": "meshclaw_mcp_smoke_test_plan"},
		},
		"summary": fmt.Sprintf("plan-only MCP rollout for client=%s branch=%s expected_tools=%d", client, branch, len(expectedTools)),
		"truth_model": []string{
			"PR/source changes are not visible to an MCP client until a MeshClaw binary containing those changes is built and selected by that client.",
			"An already running MCP server can keep serving older tool definitions until the client/server process refreshes.",
			"Plan-only MCP surfaces should be tried before any approval-gated apply or automation setup path.",
		},
		"verification_steps": []map[string]interface{}{
			{"step": 1, "action": "confirm the desired branch or PR is merged or checked out for the binary build"},
			{"step": 2, "action": "build or select the MeshClaw binary that contains the expected MCP tools"},
			{"step": 3, "action": "call meshclaw_mcp_profile_visibility_check for the selected client profile before client refresh", "tool": "meshclaw_mcp_profile_visibility_check", "arguments": map[string]interface{}{"profile": visibilityProfile, "expected_tools": strings.Join(expectedTools, ",")}},
			{"step": 4, "action": "point the MCP client configuration at that binary or restart/refresh the MCP client process"},
			{"step": 5, "action": "call meshclaw_mcp_surface and confirm expected tools are listed"},
			{"step": 6, "action": "call meshclaw_tool_recommend with a natural-language sample before invoking a specific plan tool"},
		},
		"sample_prompts": []string{
			"openwebui 서비스 디스커버리와 lb-lite 라우팅 계획 만들어줘",
			"vllm GPU workload 오토스케일 용량 계획 만들어줘",
			"d1 디스크 압박과 백업 guardrail 계획 만들어줘",
			"Prometheus Grafana Loki Ansible 기존 운영툴을 MCP 대화로 연동하는 계획 만들어줘",
			"desired-state YAML 기준으로 dry-run reconcile 계획 만들어줘",
			"컨테이너 restart loop를 로그 증거 기반 self-heal 계획으로 정리해줘",
			"openwebui 컨테이너 로그를 source=container:openwebui 로 분석해줘",
		},
		"approval_gates": []string{
			"operator reviews and builds the binary; this plan does not deploy or restart live servers",
			"client refresh should be visible and reversible",
			"profile visibility should show zero missing expected tools before asking the client to use them",
			"mutating tools remain approval-gated after rollout",
		},
		"rollback": []string{
			"point the MCP client back to the previous known-good MeshClaw binary",
			"restart or refresh the MCP client after restoring the previous binary path",
			"rerun meshclaw_mcp_profile_visibility_check against the restored profile",
			"verify meshclaw_mcp_surface returns the previous expected tool set",
		},
	}
}

func mcpRolloutCommandCompatibility(client string) []map[string]interface{} {
	return []map[string]interface{}{
		{
			"form":                 "meshclaw mcp --profile claude-lite",
			"client":               client,
			"recommended":          true,
			"purpose":              "primary stdio command form for Claude-style profile-scoped MCP clients",
			"verify_with":          "tools/list then meshclaw_mcp_surface",
			"mutates_live_servers": false,
		},
		{
			"form":                 "meshclaw mcp serve --profile claude-lite",
			"client":               client,
			"recommended":          false,
			"purpose":              "compatibility form for older local configs that include an explicit serve token",
			"verify_with":          "tools/list and zero missing expected tools",
			"mutates_live_servers": false,
		},
		{
			"form":                 "meshclaw mcp stdio --profile claude-lite",
			"client":               client,
			"recommended":          false,
			"purpose":              "compatibility form for MCP clients that name stdio explicitly",
			"verify_with":          "tools/list and zero missing expected tools",
			"mutates_live_servers": false,
		},
	}
}

func mcpRolloutRiskRegister() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"layer":                "source",
			"risk":                 "operator builds from a branch that differs from the reviewed PR stack",
			"impact":               "MCP client may expose unexpected or missing tools",
			"detection":            "record branch and source commit before binary selection",
			"mitigation":           "pause rollout and rebuild from the reviewed source",
			"mutates_live_servers": false,
		},
		{
			"layer":                "binary",
			"risk":                 "MCP client still points at an older MeshClaw binary",
			"impact":               "Claude sees stale tool definitions after source changes land",
			"detection":            "compare surface output with expected tool list after refresh",
			"mitigation":           "reselect the local binary and rerun visibility checks",
			"mutates_live_servers": false,
		},
		{
			"layer":                "client_refresh",
			"risk":                 "client cache keeps old tool metadata after binary selection",
			"impact":               "tool recommendation may route prompts to stale surfaces",
			"detection":            "rerun meshclaw_mcp_surface and meshclaw_tool_recommend",
			"mitigation":           "operator refreshes the MCP client/server process and recaptures evidence",
			"mutates_live_servers": false,
		},
		{
			"layer":                "profile",
			"risk":                 "profile allowlist hides newly added plan-only tools",
			"impact":               "Claude cannot call tools that exist in the full MCP surface",
			"detection":            "meshclaw_mcp_profile_visibility_check reports missing tools",
			"mitigation":           "update profile source and repeat build/refresh verification",
			"mutates_live_servers": false,
		},
		{
			"layer":                "smoke",
			"risk":                 "smoke test drifts into mutating apply or execute paths",
			"impact":               "rollout validation could bypass approval gates",
			"detection":            "smoke arguments include apply=true, execute=true, provider changes, or file deletion",
			"mitigation":           "stop smoke, capture evidence, and require explicit operator approval before any later mutation",
			"mutates_live_servers": false,
		},
	}
}

func mcpRolloutCopyPasteStatus(client, branch string, expectedTools []string) []string {
	return []string{
		fmt.Sprintf("MCP rollout plan: client=%s branch=%s expected_tools=%d", client, branch, len(expectedTools)),
		"Status: code review and local validation are safe now; new MCP tools are not usable until operator merge/build/client refresh completes.",
		"Operator next: merge or checkout reviewed source, select/build the MeshClaw binary, refresh the MCP client, then run visibility and read-only smoke checks.",
		"Acceptance: source, binary, surface, profile, and read-only smoke evidence must all match the expected tools.",
		"Stop/rollback: missing tools, stale tool definitions, mutating recommendations, or smoke evidence from an unknown binary.",
		"Forbidden in this plan: live server deployment, live service restart, apply=true, execute=true, provider changes, and future approval grants.",
	}
}

func mcpRolloutOperatorBrief(client, branch string, expectedTools []string) map[string]interface{} {
	return map[string]interface{}{
		"headline":            "MCP rollout is code-ready only after operator merge, local binary selection, MCP client refresh, and read-only smoke evidence.",
		"client":              client,
		"branch":              branch,
		"expected_tool_count": len(expectedTools),
		"shareable_summary": []string{
			"Code PRs can be reviewed and validated now.",
			"New MCP tools are not usable by the client until the selected binary contains the merged source.",
			"The operator must refresh the MCP client/server process before visibility checks are meaningful.",
			"Acceptance requires source, binary, surface, profile, and read-only smoke evidence.",
			"Rollback means restoring the previous known-good MCP binary or profile allowlist, not touching live servers.",
		},
		"go_no_go": map[string]interface{}{
			"go_when":    "surface, profile visibility, recommendation, and read-only smoke evidence all match the expected tools",
			"no_go_when": "any expected tool is missing, stale, mutating, or captured from an unknown binary",
		},
		"mutates_live_servers":  false,
		"deploys_binary":        false,
		"restarts_mcp_client":   false,
		"grants_apply_approval": false,
	}
}

func mcpRolloutOperatorAcceptanceChecklist(expectedTools []string) []map[string]interface{} {
	return []map[string]interface{}{
		{
			"item":                 "source reviewed",
			"accept_if":            "the reviewed branch or merged commit is the source used for the selected MCP binary",
			"evidence":             "branch name and source commit recorded",
			"required":             true,
			"mutates_live_servers": false,
		},
		{
			"item":                 "binary selected",
			"accept_if":            "the MCP client points at a MeshClaw binary built from the accepted source",
			"evidence":             "binary path and source commit recorded",
			"required":             true,
			"mutates_live_servers": false,
		},
		{
			"item":                 "surface visible",
			"accept_if":            "MCP surface lists every expected plan-only tool",
			"evidence":             "meshclaw_mcp_surface output",
			"expected_tool_count":  len(expectedTools),
			"required":             true,
			"mutates_live_servers": false,
		},
		{
			"item":                 "profile visible",
			"accept_if":            "profile visibility reports zero missing expected tools",
			"evidence":             "meshclaw_mcp_profile_visibility_check output",
			"required":             true,
			"mutates_live_servers": false,
		},
		{
			"item":                 "smoke completed",
			"accept_if":            "read-only smoke checks complete without apply, execute, provider changes, file deletion, or live mutation",
			"evidence":             "meshclaw_mcp_smoke_test_plan evidence",
			"required":             true,
			"mutates_live_servers": false,
		},
	}
}

func mcpRolloutClientRefreshMatrix(client string) []map[string]interface{} {
	return []map[string]interface{}{
		{
			"client":               client,
			"scope":                "selected",
			"operator_action":      "confirm this MCP client points at the selected MeshClaw binary and refresh the client/server process",
			"verify_with":          []string{"meshclaw_mcp_surface", "meshclaw_mcp_profile_visibility_check"},
			"reversible":           true,
			"restarts_mcp_client":  false,
			"mutates_live_servers": false,
		},
		{
			"client":               "claude-lite",
			"scope":                "profile",
			"operator_action":      "confirm claude-lite profile allowlist includes every expected rollout tool after binary refresh",
			"verify_with":          []string{"meshclaw_mcp_profile_visibility_check"},
			"reversible":           true,
			"restarts_mcp_client":  false,
			"mutates_live_servers": false,
		},
		{
			"client":               "generic-mcp-client",
			"scope":                "fallback",
			"operator_action":      "confirm the client process re-reads tool definitions from the selected binary before smoke checks",
			"verify_with":          []string{"meshclaw_mcp_surface"},
			"reversible":           true,
			"restarts_mcp_client":  false,
			"mutates_live_servers": false,
		},
	}
}

func mcpRolloutReadinessLabels() map[string]interface{} {
	return map[string]interface{}{
		"safe_now": []string{
			"review rollout plan output",
			"review draft PR stack",
			"run local build/test/vet in a clean worktree",
			"prepare read-only MCP smoke prompts",
		},
		"operator_required": []string{
			"merge reviewed PRs",
			"select or build the MeshClaw binary used by the MCP client",
			"refresh the MCP client/server process",
			"capture visibility and smoke evidence",
		},
		"blocked_until": []string{
			"reviewed source is merged or intentionally checked out",
			"binary source commit is known",
			"MCP client is refreshed against the selected binary",
			"profile visibility reports zero missing expected tools",
		},
		"never_in_plan": []string{
			"deploy to live servers",
			"restart live services",
			"run apply=true or execute=true smoke checks",
			"grant future apply approval",
		},
		"can_use_new_tools_now": false,
		"mutates_live_servers":  false,
	}
}

func mcpRolloutSummaryCounts(expectedTools []string) map[string]interface{} {
	return map[string]interface{}{
		"expected_tools":            len(expectedTools),
		"operator_checkpoints":      5,
		"verification_steps":        6,
		"local_verification_steps":  4,
		"evidence_checklist_items":  6,
		"troubleshooting_layers":    4,
		"rollback_decision_points":  4,
		"post_reload_smoke_prompts": len(expectedTools),
		"mutating_steps":            0,
		"live_server_mutations":     0,
		"binary_deployments":        0,
		"mcp_client_restarts":       0,
		"future_approval_grants":    0,
	}
}

func mcpRolloutRollbackDecisionPoints(expectedTools []string) []map[string]interface{} {
	return []map[string]interface{}{
		{
			"phase":                "surface",
			"rollback_if":          "selected MCP surface omits any expected plan-only tool",
			"restore":              "previous known-good MeshClaw MCP binary",
			"evidence":             "meshclaw_mcp_surface output before and after refresh",
			"expected_tool_count":  len(expectedTools),
			"mutates_live_servers": false,
		},
		{
			"phase":                "profile",
			"rollback_if":          "profile visibility reports missing expected tools after client refresh",
			"restore":              "previous profile allowlist or previous MCP binary",
			"evidence":             "meshclaw_mcp_profile_visibility_check result",
			"mutates_live_servers": false,
		},
		{
			"phase":                "recommendation",
			"rollback_if":          "tool recommendation routes a smoke prompt to a mutating tool or stale surface",
			"restore":              "previous known-good MCP binary and tool recommendation surface",
			"evidence":             "meshclaw_tool_recommend result for the smoke prompt",
			"mutates_live_servers": false,
		},
		{
			"phase":                "smoke",
			"rollback_if":          "read-only smoke evidence is missing, stale, or requests apply/execute/provider changes",
			"restore":              "previous known-good MCP binary and rerun read-only smoke plan",
			"evidence":             "meshclaw_mcp_smoke_test_plan evidence",
			"mutates_live_servers": false,
		},
	}
}

func mcpRolloutStackReviewDiscipline(branch string) map[string]interface{} {
	return map[string]interface{}{
		"head_branch": branch,
		"review_order": []string{
			"review and merge the lowest-base PR first",
			"wait for the branch stack to rebase or update before reviewing the next layer",
			"run build/test/vet on the exact branch that will be selected for the MCP binary",
			"stop the rollout if any lower layer changes expected MCP tools or profile visibility",
		},
		"merge_gate":            "all lower layers reviewed, merged, and validated before selecting a binary for MCP client refresh",
		"failure_response":      "pause higher-layer rollout, refresh the rollout plan from the new base, and rerun read-only smoke checks",
		"allows_squash_merge":   true,
		"mutates_live_servers":  false,
		"deploys_binary":        false,
		"restarts_mcp_client":   false,
		"grants_apply_approval": false,
	}
}

func mcpRolloutLocalVerificationSteps(client, branch string, expectedTools []string) []map[string]interface{} {
	return []map[string]interface{}{
		{
			"phase":                "source",
			"command":              fmt.Sprintf("git status --short --branch && git branch --show-current # expect %s", branch),
			"purpose":              "confirm the operator is building from the intended source branch or merged commit",
			"mutates_live_servers": false,
		},
		{
			"phase":                "build",
			"command":              "go build ./...",
			"purpose":              "confirm the source builds before selecting a MeshClaw binary for the MCP client",
			"mutates_live_servers": false,
		},
		{
			"phase":                "surface",
			"tool":                 "meshclaw_mcp_surface",
			"arguments":            map[string]interface{}{},
			"purpose":              "confirm the local MCP surface includes the expected plan-only tools after client refresh",
			"mutates_live_servers": false,
		},
		{
			"phase":                "profile",
			"tool":                 "meshclaw_mcp_profile_visibility_check",
			"arguments":            map[string]interface{}{"profile": client, "expected_tools": strings.Join(expectedTools, ",")},
			"purpose":              "confirm the selected MCP profile exposes every expected tool",
			"mutates_live_servers": false,
		},
	}
}

func mcpRolloutOperatorHandoff(client, branch string) map[string]interface{} {
	return map[string]interface{}{
		"handoff_required": true,
		"handoff_to":       "operator",
		"reason":           "PR/source changes require an operator-controlled merge, local binary build or selection, and MCP client refresh before new tools are visible.",
		"client":           client,
		"branch":           branch,
		"codex_stops_before": []string{
			"merging PRs",
			"building or selecting production binaries",
			"editing MCP client configuration",
			"restarting MCP clients",
			"deploying to live servers",
		},
		"operator_resumes_with": []string{
			"merge or checkout reviewed source",
			"build/select the MeshClaw binary for the MCP client",
			"refresh the MCP client/server process",
			"run profile visibility and read-only smoke checks",
		},
		"container_executor_live_safety": []string{
			"run meshclaw_autoheal_container_executor_gate from ready readiness-summary evidence first",
			"run meshclaw_autoheal_container_executor with dry_run=true before any live execution",
			"require matching approved_by, dry_run=false, execute=true, and live_approval_phrase exactly 'execute container self-heal approved' for live restart",
			"run meshclaw_autoheal_container_executor_verify with post-action agent evidence and focused container-logscan evidence before closeout",
		},
		"mutates_live_servers":   false,
		"deploys_binary":         false,
		"restarts_mcp_client":    false,
		"grants_future_approval": false,
	}
}

func mcpRolloutSuccessCriteria(expectedTools []string) map[string]interface{} {
	return map[string]interface{}{
		"status_if_all_pass": "ready_for_operator_review",
		"success": []string{
			"selected MCP surface lists every expected tool",
			"claude-lite profile visibility reports zero missing expected tools",
			"tool recommendation routes each post-reload smoke prompt to the expected plan-only tool",
			"reconcile readiness summary exposes desired-state executor-contract evidence before any future apply loop",
			"container readiness summary exposes apply-loop gate and executor-contract evidence before any future apply loop",
			"read-only smoke checks complete without apply=true, execute=true, provider changes, file deletion, or live server mutation",
		},
		"blocked": []string{
			"any expected tool is absent from the selected MCP surface",
			"profile visibility reports one or more missing expected tools",
			"tool recommendation selects a mutating tool for a smoke prompt",
			"smoke evidence is missing, stale, or captured from an unknown binary",
		},
		"rollback_recommended": []string{
			"client reload exposes old tool definitions after binary verification",
			"new surface prevents existing plan-only tools from being discovered",
			"smoke prompt produces a mutating recommendation before explicit approval",
		},
		"expected_tool_count":   len(expectedTools),
		"mutates_live_servers":  false,
		"grants_apply_approval": false,
	}
}

func mcpRolloutEvidenceChecklist(client, branch string, expectedTools []string) []map[string]interface{} {
	return []map[string]interface{}{
		{"phase": "source", "record": "branch_or_pr", "value": branch, "required": true, "mutates_live_servers": false},
		{"phase": "binary", "record": "binary_source_commit", "value": "<operator-recorded-commit>", "required": true, "mutates_live_servers": false},
		{"phase": "client", "record": "mcp_client_profile", "value": client, "required": true, "mutates_live_servers": false},
		{"phase": "visibility", "record": "expected_tools", "value": expectedTools, "required": true, "tool": "meshclaw_mcp_profile_visibility_check", "mutates_live_servers": false},
		{"phase": "smoke", "record": "read_only_smoke_result", "value": "<mcp-smoke-test-plan-evidence>", "required": true, "tool": "meshclaw_mcp_smoke_test_plan", "mutates_live_servers": false},
		{"phase": "rollback", "record": "previous_binary_path", "value": "<previous-known-good-binary>", "required": true, "mutates_live_servers": false},
	}
}

func mcpRolloutTroubleshooting(expectedTools []string) []map[string]interface{} {
	return []map[string]interface{}{
		{
			"symptom":       "new tools are absent from the MCP surface",
			"likely_layer":  "binary",
			"check":         "confirm the MCP client is pointing at a MeshClaw binary built from the selected branch or merged PR",
			"next_tool":     "meshclaw_mcp_surface",
			"safe_response": "rebuild or reselect the local binary, then refresh the MCP client; do not deploy to live servers",
		},
		{
			"symptom":        "tools exist in full profile but are missing from claude-lite",
			"likely_layer":   "profile_allowlist",
			"check":          "compare expected tools against the claude-lite profile visibility result",
			"next_tool":      "meshclaw_mcp_profile_visibility_check",
			"expected_tools": expectedTools,
			"safe_response":  "update the profile allowlist in source and repeat the build/reload path",
		},
		{
			"symptom":       "old tool descriptions or old arguments still appear",
			"likely_layer":  "client_cache",
			"check":         "confirm the MCP client/server process was refreshed after binary selection",
			"next_tool":     "meshclaw_mcp_rollout_plan",
			"safe_response": "reload the MCP client process or point it back to the previous known-good binary",
		},
		{
			"symptom":       "plan-only smoke prompt recommends a mutating action",
			"likely_layer":  "recommendation_policy",
			"check":         "rerun tool recommendation and confirm avoid/requires_approval/required_evidence/stop_before fields before invoking any tool",
			"next_tool":     "meshclaw_tool_recommend",
			"safe_response": "stop before apply, capture evidence, and require explicit operator approval for any later mutation",
		},
	}
}

func mcpRolloutSmokePrompts(expectedTools []string) []map[string]interface{} {
	prompts := make([]map[string]interface{}, 0, len(expectedTools))
	for _, tool := range expectedTools {
		prompts = append(prompts, map[string]interface{}{
			"tool":                    tool,
			"prompt":                  smokePromptForTool(tool),
			"expected_recommendation": tool,
			"expected_behavior":       rolloutSmokeExpectedBehavior(tool),
			"requires_approval":       false,
		})
	}
	return prompts
}

func rolloutSmokeExpectedBehavior(tool string) string {
	if tool == "meshclaw_analyze_logs" {
		return "read-only log evidence with autoheal_handoff.runtime_evidence_checklist and handoff_contract.apply_allowed=false, handoff_contract.direct_restart_allowed=false, handoff_contract.requires_focused_runtime_evidence=true, handoff_contract.runtime_evidence_checklist_count, and handoff_contract.next_required_tool for container logs; no apply, execute, provider change, file deletion, or live server mutation"
	}
	if contract := reconcileContractSmokeExpected(tool); contract != "" {
		return contract + "; no apply, execute, provider change, file deletion, or live server mutation"
	}
	if contract := containerContractSmokeExpected(tool); contract != "" {
		return contract + "; no apply, execute, provider change, file deletion, or live server mutation"
	}
	if tool == "meshclaw_reconcile_readiness_summary" || tool == "meshclaw_autoheal_container_readiness_summary" {
		return "summary-only readiness evidence with readiness_contract.apply_allowed=false and readiness_contract.grants_future_approval=false; no apply, execute, provider change, file deletion, or live server mutation"
	}
	return "plan-only evidence or dry-run response; no apply, execute, provider change, file deletion, or live server mutation"
}

func mcpRolloutReadiness(client, branch string, expectedTools []string) map[string]interface{} {
	return map[string]interface{}{
		"status":                 "plan_ready",
		"client":                 client,
		"branch":                 branch,
		"expected_tool_count":    len(expectedTools),
		"source_ready":           branch != "" && branch != "<merged-or-local-branch>",
		"binary_built":           false,
		"client_reloaded":        false,
		"profile_visible":        len(expectedTools) > 0,
		"can_use_new_tools_now":  false,
		"next_required_actions":  []string{"merge or checkout branch", "build/select MeshClaw binary", "refresh MCP client", "rerun profile visibility check"},
		"mutates_live_servers":   false,
		"deploys_binary":         false,
		"restarts_mcp_client":    false,
		"grants_future_approval": false,
	}
}

func buildMCPSmokeTestPlan(args map[string]interface{}) map[string]interface{} {
	client := strings.TrimSpace(stringArg(args, "client"))
	if client == "" {
		client = "claude"
	}
	scope := strings.TrimSpace(stringArg(args, "scope"))
	if scope == "" {
		scope = "k8s-replacement"
	}
	tools := splitCSV(stringArg(args, "tools"))
	if len(tools) == 0 {
		tools = defaultMCPSmokeTestTools()
	}
	visibilityProfile := mcpRolloutVisibilityProfile(client, tools)
	checks := make([]map[string]interface{}, 0, len(tools)+3)
	checks = append(checks,
		map[string]interface{}{"step": 1, "tool": "meshclaw_mcp_surface", "prompt": "MCP surface와 기본 도구 목록 보여줘", "expected": "default_tools contains the plan-only tools under test"},
		map[string]interface{}{"step": 2, "tool": "meshclaw_mcp_profile_visibility_check", "arguments": map[string]interface{}{"profile": visibilityProfile, "expected_tools": strings.Join(tools, ",")}, "expected": "selected MCP profile exposes every tool under test before client reload"},
		map[string]interface{}{"step": 3, "tool": "meshclaw_tool_recommend", "prompt": "openwebui 서비스 디스커버리와 lb-lite 라우팅 계획에 어떤 도구를 써야 해?", "expected": "recommendation routes to a plan-only tool and marks execution_policy.executes_changes=false with required_evidence and stop_before"},
	)
	for i, tool := range tools {
		checks = append(checks, map[string]interface{}{
			"step":             i + 4,
			"tool":             tool,
			"prompt":           smokePromptForTool(tool),
			"arguments":        smokeArgumentsForTool(tool),
			"fixture_required": smokeFixtureRequiredForTool(tool),
			"expected":         smokeExpectedForTool(tool),
		})
	}
	return map[string]interface{}{
		"kind":               "mcp_smoke_test_plan",
		"client":             client,
		"scope":              scope,
		"tools":              tools,
		"visibility_profile": visibilityProfile,
		"fixture_manifest":   smokeFixtureManifest(tools),
		"run_strategy":       smokeRunStrategy(tools),
		"profile_visibility": mcpProfileVisibilityPreview(visibilityProfile, tools),
		"summary_counts":     smokeSummaryCounts(tools),
		"approval_boundary":  smokeApprovalBoundary(),
		"readiness":          smokeReadiness(tools),
		"summary":            fmt.Sprintf("plan-only MCP smoke test for client=%s scope=%s tools=%d", client, scope, len(tools)),
		"checks":             checks,
		"pass_criteria": []string{
			"meshclaw_mcp_surface lists each tool under test",
			"meshclaw_mcp_profile_visibility_check reports no missing tools for the selected MCP profile",
			"meshclaw_tool_recommend routes sample prompts to expected plan-only tools and exposes required_evidence plus stop_before boundaries",
			"each plan-only tool returns report and evidence fields",
			"no smoke step requests apply=true, execute=true, provider changes, file deletion, proxy edits, or live server mutation",
		},
		"failure_triage": []string{
			"if a tool is missing, verify the MCP client points at the newly built MeshClaw binary and the selected profile allowlist includes it",
			"if recommendations route to older tools, refresh the MCP client/server process and rerun meshclaw_mcp_surface",
			"if a plan tool errors, run meshclaw_doctor before blaming the fleet",
		},
		"approval_gates": []string{
			"smoke tests are read-only and do not grant approval for later apply paths",
			"operator approval is still required before any mutating automation, rollout, cleanup, proxy, DNS, or provider action",
		},
		"rollback": []string{
			"stop using the new MCP client surface and return to the previous known-good MeshClaw binary",
			"keep smoke-test evidence for comparison with the previous surface",
		},
	}
}

func smokeExpectedForTool(tool string) string {
	if tool == "meshclaw_analyze_logs" {
		return "returns redacted log report, evidence, autoheal_handoff.runtime_evidence_checklist, and handoff_contract.apply_allowed=false with handoff_contract.direct_restart_allowed=false, handoff_contract.requires_focused_runtime_evidence=true, handoff_contract.runtime_evidence_checklist_count, and handoff_contract.next_required_tool for focused container logs without executing mutations"
	}
	if contract := reconcileContractSmokeExpected(tool); contract != "" {
		return "returns report, evidence, and " + contract + " without executing mutations"
	}
	if contract := containerContractSmokeExpected(tool); contract != "" {
		return "returns report, evidence, and " + contract + " without executing mutations"
	}
	if tool == "meshclaw_reconcile_readiness_summary" || tool == "meshclaw_autoheal_container_readiness_summary" {
		return "returns readiness summary, evidence, and readiness_contract.apply_allowed=false with readiness_contract.grants_future_approval=false without executing mutations"
	}
	return "returns report and evidence without executing mutations"
}

func reconcileContractSmokeExpected(tool string) string {
	switch tool {
	case "meshclaw_reconcile_validate_desired":
		return "desired_validation_contract.validation_only=true with desired_validation_contract.yaml_keys_grant_approval=false"
	case "meshclaw_reconcile_plan":
		return "reconcile_plan_contract.dry_run_only=true with reconcile_plan_contract.apply_allowed=false"
	case "meshclaw_reconcile_approval_request":
		return "approval_request_contract.request_only=true with approval_request_contract.operator_approval_recorded=false"
	case "meshclaw_reconcile_apply_gate":
		return "apply_gate_contract.gate_only=true with apply_gate_contract.apply_allowed=false and apply_gate_contract.requires_operator_approval=true"
	case "meshclaw_reconcile_apply_plan":
		return "apply_plan_contract.plan_only=true with apply_plan_contract.apply_allowed=false and apply_plan_contract.requires_execution_preview_before_executor=true"
	case "meshclaw_reconcile_execution_preview":
		return "execution_preview_contract.preview_only=true with execution_preview_contract.apply_allowed=false and execution_preview_contract.commands_are_inert_templates=true"
	case "meshclaw_reconcile_verification_plan":
		return "verification_plan_contract.apply_allowed=false with verification_plan_contract.requires_post_action_evidence=true"
	case "meshclaw_reconcile_runbook":
		return "runbook_contract.review_only=true with runbook_contract.apply_allowed=false"
	case "meshclaw_reconcile_runbook_check":
		return "runbook_check_contract.gate_only=true with runbook_check_contract.apply_allowed=false and runbook_check_contract.requires_zero_critical_findings=true"
	case "meshclaw_reconcile_rollback_plan":
		return "rollback_plan_contract.apply_allowed=false with rollback_plan_contract.rollback_allowed=false"
	case "meshclaw_reconcile_completion_plan":
		return "completion_plan_contract.complete_allowed=false with completion_plan_contract.requires_final_evidence=true"
	default:
		return ""
	}
}

func containerContractSmokeExpected(tool string) string {
	switch tool {
	case "meshclaw_autoheal_container_apply_plan":
		return "container_apply_plan_contract.plan_only=true with container_apply_plan_contract.apply_allowed=false, container_apply_plan_contract.direct_restart_allowed=false, container_apply_plan_contract.requires_focused_runtime_evidence=true, and container_apply_plan_contract.runtime_evidence_required_count"
	case "meshclaw_autoheal_container_verification_plan":
		return "container_verification_plan_contract.apply_allowed=false with container_verification_plan_contract.requires_container_logscan=true and container_verification_plan_contract.apply_plan_runtime_evidence_required_count"
	case "meshclaw_autoheal_container_runbook":
		return "container_runbook_contract.review_only=true with container_runbook_contract.apply_allowed=false and container_runbook_contract.apply_plan_runtime_evidence_required_count"
	case "meshclaw_autoheal_container_runbook_check":
		return "container_runbook_check_contract.gate_only=true with container_runbook_check_contract.requires_zero_critical_findings=true"
	case "meshclaw_autoheal_container_rollback_plan":
		return "container_rollback_plan_contract.rollback_allowed=false with container_rollback_plan_contract.requires_operator_approval=true"
	case "meshclaw_autoheal_container_completion_plan":
		return "container_completion_plan_contract.complete_allowed=false with container_completion_plan_contract.requires_final_evidence=true"
	case "meshclaw_autoheal_container_readiness_summary":
		return "container_readiness_summary_contract.summary_only=true with container_readiness_summary_contract.requires_approval_gated_executor=true"
	case "meshclaw_autoheal_container_executor_gate":
		return "container_executor_gate_contract.gate_only=true with container_executor_gate_contract.executor_allowed=false and container_executor_gate_contract.requires_dry_run=true"
	case "meshclaw_autoheal_container_executor":
		return "container_executor_contract.dry_run=true by default with container_executor_contract.requires_exact_live_phrase=true and mutates_live_servers=false"
	case "meshclaw_autoheal_container_executor_verify":
		return "container_executor_verification_contract.gate_only=true with container_executor_verification_contract.requires_agent_evidence=true and container_executor_verification_contract.requires_container_logscan=true"
	default:
		return ""
	}
}

func smokePromptForTool(tool string) string {
	switch tool {
	case "meshclaw_service_registry_plan":
		return "openwebui 서비스 디스커버리와 lb-lite 라우팅 계획 만들어줘"
	case "meshclaw_capacity_scale_plan":
		return "vllm GPU workload 오토스케일 용량 계획 만들어줘"
	case "meshclaw_storage_guardrail_plan":
		return "d1 디스크 압박과 백업 guardrail 계획 만들어줘"
	case "meshclaw_ops_integration_plan":
		return "Prometheus Grafana Loki Ansible 기존 운영툴을 MCP 대화로 연동하는 계획 만들어줘"
	case "meshclaw_reconcile_validate_desired":
		return "desired-state YAML을 검증하고 validation_handoff를 확인해줘"
	case "meshclaw_reconcile_plan":
		return "desired-state YAML 기준으로 dry-run reconcile 계획 만들어줘"
	case "meshclaw_reconcile_approval_request":
		return "desired-state reconcile plan 기준으로 approval-request contract를 확인해줘"
	case "meshclaw_reconcile_apply_gate":
		return "reconcile approval-request evidence 기준으로 apply-gate contract를 확인해줘"
	case "meshclaw_reconcile_apply_plan":
		return "reconcile apply-gate evidence 기준으로 plan-only apply-plan contract를 확인해줘"
	case "meshclaw_reconcile_execution_preview":
		return "reconcile apply-plan evidence 기준으로 preview-only inert command template contract를 확인해줘"
	case "meshclaw_reconcile_verification_plan":
		return "reconcile execution-preview evidence 기준으로 post-action verification plan contract를 확인해줘"
	case "meshclaw_reconcile_runbook":
		return "reconcile verification-plan evidence 기준으로 review-only runbook contract를 확인해줘"
	case "meshclaw_reconcile_runbook_check":
		return "reconcile runbook evidence 기준으로 gate-only runbook-check contract를 확인해줘"
	case "meshclaw_reconcile_rollback_plan":
		return "reconcile runbook-check evidence 기준으로 rollback guidance contract를 확인해줘"
	case "meshclaw_reconcile_completion_plan":
		return "reconcile rollback-plan evidence 기준으로 completion requirements contract를 확인해줘"
	case "meshclaw_reconcile_readiness_summary":
		return "reconcile completion evidence 기준으로 desired-state executor contract readiness summary를 확인해줘"
	case "meshclaw_autoheal_plan":
		return "컨테이너 restart loop를 증거 기반 self-heal 계획으로 정리해줘"
	case "meshclaw_autoheal_container_apply_plan":
		return "autoheal plan evidence 기준으로 승인 게이트가 있는 container apply plan을 만들어줘"
	case "meshclaw_autoheal_container_readiness_summary":
		return "container completion evidence 기준으로 apply-loop gates readiness summary를 확인해줘"
	case "meshclaw_autoheal_container_executor_gate":
		return "container readiness-summary evidence 기준으로 executor gate admission-only contract를 확인해줘"
	case "meshclaw_autoheal_container_executor":
		return "container executor-gate evidence 기준으로 dry-run executor preview contract를 확인해줘"
	case "meshclaw_autoheal_container_executor_verify":
		return "container executor evidence와 post-action agent/logscan evidence 기준으로 closeout verification gate를 확인해줘"
	case "meshclaw_analyze_logs":
		return "openwebui 컨테이너 로그를 source=container:openwebui 로 분석하고 systemd runtime 로그 샘플의 unit identity 확인 전 restart 금지 gate를 확인해줘"
	case "meshclaw_mcp_rollout_plan":
		return "새 MCP 도구가 Claude에서 보이게 바이너리 갱신 후 어떻게 써보는지 계획해줘"
	case "meshclaw_mcp_profile_visibility_check":
		return "claude-lite profile에서 새 MCP 도구들이 보이는지 profile visibility check 해줘"
	case "meshclaw_automation_rule_check":
		return "자동화 규칙 plan evidence를 쓰기 전에 approval/cooldown/rollback gate로 검증해줘"
	case "meshclaw_automation_rule_readiness_summary":
		return "자동화 규칙 check evidence 기준으로 readiness summary 준비 상태 요약해줘"
	case "meshclaw_automation_rule_writer_plan":
		return "자동화 규칙 readiness evidence 기준으로 rule writer plan 작성 계획만 만들어줘"
	default:
		return "이 도구를 plan-only smoke test로 확인해줘"
	}
}

func smokeArgumentsForTool(tool string) map[string]interface{} {
	switch tool {
	case "meshclaw_service_registry_plan":
		return map[string]interface{}{"service": "openwebui", "scope": "all", "port": 8080, "endpoints": "g1:8080,g2:8080"}
	case "meshclaw_capacity_scale_plan":
		return map[string]interface{}{"workload": "vllm", "scope": "gpu", "target": "gpu_free>=1", "budget_usd": 0, "ttl_hours": 6, "constraints": "local-first,approval-required"}
	case "meshclaw_storage_guardrail_plan":
		return map[string]interface{}{"node": "d1", "path": "/", "workload": "openwebui", "risk": "disk_pressure", "backup": true, "retention": "keep-newest-5", "mount_type": "local"}
	case "meshclaw_ops_integration_plan":
		return map[string]interface{}{"tools": "prometheus,grafana,loki,ansible,ntfy", "goal": "chat-driven incident triage", "scope": "fleet", "readonly": true}
	case "meshclaw_reconcile_validate_desired":
		return map[string]interface{}{"desired_path": "<test-fixture-desired-state.yaml>"}
	case "meshclaw_reconcile_plan":
		return map[string]interface{}{"desired_path": "<test-fixture-desired-state.yaml>", "actual_report_path": "<optional-test-fixture-actual-report.json>"}
	case "meshclaw_reconcile_approval_request":
		return map[string]interface{}{"desired_path": "<test-fixture-desired-state.yaml>", "actual_report_path": "<optional-test-fixture-actual-report.json>"}
	case "meshclaw_reconcile_apply_gate":
		return map[string]interface{}{"desired_path": "<test-fixture-desired-state.yaml>", "approval_evidence_path": "<test-fixture-reconcile-approval-request-evidence.json>", "approved_by": "<operator>"}
	case "meshclaw_reconcile_apply_plan":
		return map[string]interface{}{"gate_evidence_path": "<test-fixture-reconcile-apply-gate-evidence.json>"}
	case "meshclaw_reconcile_execution_preview":
		return map[string]interface{}{"apply_plan_evidence_path": "<test-fixture-reconcile-apply-plan-evidence.json>"}
	case "meshclaw_reconcile_verification_plan":
		return map[string]interface{}{"execution_preview_evidence_path": "<test-fixture-reconcile-execution-preview-evidence.json>"}
	case "meshclaw_reconcile_runbook":
		return map[string]interface{}{"verification_plan_evidence_path": "<test-fixture-reconcile-verification-plan-evidence.json>"}
	case "meshclaw_reconcile_runbook_check":
		return map[string]interface{}{"runbook_evidence_path": "<test-fixture-reconcile-runbook-evidence.json>"}
	case "meshclaw_reconcile_rollback_plan":
		return map[string]interface{}{"runbook_check_evidence_path": "<test-fixture-reconcile-runbook-check-evidence.json>"}
	case "meshclaw_reconcile_completion_plan":
		return map[string]interface{}{"rollback_plan_evidence_path": "<test-fixture-reconcile-rollback-plan-evidence.json>"}
	case "meshclaw_reconcile_readiness_summary":
		return map[string]interface{}{"completion_plan_evidence_path": "<test-fixture-reconcile-completion-plan-evidence.json>"}
	case "meshclaw_autoheal_plan":
		return map[string]interface{}{}
	case "meshclaw_autoheal_container_apply_plan":
		return map[string]interface{}{"plan_evidence_path": "<test-fixture-autoheal-plan-evidence.json>", "approved_by": "<operator>"}
	case "meshclaw_autoheal_container_verification_plan":
		return map[string]interface{}{"container_apply_plan_evidence_path": "<test-fixture-container-apply-plan-evidence.json>"}
	case "meshclaw_autoheal_container_runbook":
		return map[string]interface{}{"container_verification_plan_evidence_path": "<test-fixture-container-verification-plan-evidence.json>"}
	case "meshclaw_autoheal_container_runbook_check":
		return map[string]interface{}{"container_runbook_evidence_path": "<test-fixture-container-runbook-evidence.json>"}
	case "meshclaw_autoheal_container_rollback_plan":
		return map[string]interface{}{"container_runbook_check_evidence_path": "<test-fixture-container-runbook-check-evidence.json>"}
	case "meshclaw_autoheal_container_completion_plan":
		return map[string]interface{}{"container_rollback_plan_evidence_path": "<test-fixture-container-rollback-plan-evidence.json>"}
	case "meshclaw_autoheal_container_readiness_summary":
		return map[string]interface{}{"container_completion_plan_evidence_path": "<test-fixture-container-completion-plan-evidence.json>"}
	case "meshclaw_autoheal_container_executor_gate":
		return map[string]interface{}{"container_readiness_summary_evidence_path": "<test-fixture-container-readiness-summary-evidence.json>", "approved_by": "<operator>", "dry_run": true}
	case "meshclaw_autoheal_container_executor":
		return map[string]interface{}{"container_executor_gate_evidence_path": "<test-fixture-container-executor-gate-evidence.json>", "approved_by": "<operator>", "dry_run": true}
	case "meshclaw_autoheal_container_executor_verify":
		return map[string]interface{}{"container_executor_evidence_path": "<test-fixture-container-executor-evidence.json>", "agent_evidence_paths": "<test-fixture-post-action-agent-evidence.json>", "container_logscan_evidence_paths": "<test-fixture-post-action-container-logscan-evidence.json>"}
	case "meshclaw_analyze_logs":
		return map[string]interface{}{"host": "g1", "source": "container:openwebui"}
	case "meshclaw_mcp_rollout_plan":
		return map[string]interface{}{"client": "claude", "branch": "<merged-or-local-branch>", "expected_tools": strings.Join(defaultMCPSmokeTestTools(), ",")}
	case "meshclaw_mcp_profile_visibility_check":
		return map[string]interface{}{"profile": "claude-lite", "expected_tools": strings.Join(defaultMCPSmokeTestTools(), ",")}
	default:
		return map[string]interface{}{}
	}
}

func smokeFixtureRequiredForTool(tool string) bool {
	switch tool {
	case "meshclaw_reconcile_validate_desired",
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
		"meshclaw_autoheal_container_apply_plan",
		"meshclaw_autoheal_container_verification_plan",
		"meshclaw_autoheal_container_runbook",
		"meshclaw_autoheal_container_runbook_check",
		"meshclaw_autoheal_container_rollback_plan",
		"meshclaw_autoheal_container_completion_plan",
		"meshclaw_autoheal_container_readiness_summary",
		"meshclaw_autoheal_container_executor_gate",
		"meshclaw_autoheal_container_executor",
		"meshclaw_autoheal_container_executor_verify",
		"meshclaw_analyze_logs":
		return true
	default:
		return false
	}
}

func smokeFixtureManifest(tools []string) []map[string]interface{} {
	manifest := make([]map[string]interface{}, 0)
	for _, tool := range tools {
		switch tool {
		case "meshclaw_reconcile_validate_desired":
			manifest = append(manifest, map[string]interface{}{
				"tool":            tool,
				"fixture":         "desired-state YAML validation input",
				"placeholder":     "<test-fixture-desired-state.yaml>",
				"purpose":         "exercise validation-only desired-state parsing and validation_handoff before reconcile planning",
				"minimum_content": []string{"schema_version", "nodes", "validation_handoff", "no apply/execute", "ignored apply/execute key sample", "ignored_apply_keys count", "validation_handoff stop_before ignored YAML apply/execute keys"},
				"mutates_state":   false,
			})
		case "meshclaw_reconcile_plan":
			manifest = append(manifest, map[string]interface{}{
				"tool":            tool,
				"fixture":         "desired-state YAML",
				"placeholder":     "<test-fixture-desired-state.yaml>",
				"purpose":         "exercise dry-run reconcile parsing without applying desired state",
				"minimum_content": []string{"schema_version", "nodes or workloads", "expected plan-only drift/action candidates"},
				"mutates_state":   false,
			})
		case "meshclaw_reconcile_approval_request":
			manifest = append(manifest, map[string]interface{}{
				"tool":            tool,
				"fixture":         "desired-state YAML plus optional actual report",
				"placeholder":     "<test-fixture-desired-state.yaml>",
				"purpose":         "exercise request-only approval contract without recording operator approval",
				"minimum_content": []string{"schema_version", "actions requiring approval", "approval_request_contract.request_only=true", "approval_request_contract.operator_approval_recorded=false", "blocked actions must not execute"},
				"mutates_state":   false,
			})
		case "meshclaw_reconcile_apply_gate":
			manifest = append(manifest, map[string]interface{}{
				"tool":            tool,
				"fixture":         "reconcile approval-request evidence",
				"placeholder":     "<test-fixture-reconcile-approval-request-evidence.json>",
				"purpose":         "exercise gate-only apply-gate contract without executing desired-state changes",
				"minimum_content": []string{"evidence kind reconcile-approval-request", "approval_request_contract.request_only=true", "approval-required actions", "apply_gate_contract.gate_only=true", "apply_gate_contract.apply_allowed=false", "operator approved_by fixture only"},
				"mutates_state":   false,
			})
		case "meshclaw_reconcile_apply_plan":
			manifest = append(manifest, map[string]interface{}{
				"tool":            tool,
				"fixture":         "reconcile apply-gate evidence",
				"placeholder":     "<test-fixture-reconcile-apply-gate-evidence.json>",
				"purpose":         "exercise plan-only apply-plan contract without executing desired-state changes",
				"minimum_content": []string{"evidence kind reconcile-apply-gate", "apply_gate_contract.gate_only=true", "approved_by fixture", "apply_plan_contract.plan_only=true", "apply_plan_contract.apply_allowed=false", "execution-preview required before executor"},
				"mutates_state":   false,
			})
		case "meshclaw_reconcile_execution_preview":
			manifest = append(manifest, map[string]interface{}{
				"tool":            tool,
				"fixture":         "reconcile apply-plan evidence",
				"placeholder":     "<test-fixture-reconcile-apply-plan-evidence.json>",
				"purpose":         "exercise preview-only inert command templates without executing desired-state changes",
				"minimum_content": []string{"evidence kind reconcile-apply-plan", "apply_plan_contract.plan_only=true", "execution_preview_contract.preview_only=true", "execution_preview_contract.commands_are_inert_templates=true", "no runnable shell commands"},
				"mutates_state":   false,
			})
		case "meshclaw_reconcile_verification_plan":
			manifest = append(manifest, map[string]interface{}{
				"tool":            tool,
				"fixture":         "reconcile execution-preview evidence",
				"placeholder":     "<test-fixture-reconcile-execution-preview-evidence.json>",
				"purpose":         "exercise verification-plan contract without running preview command templates",
				"minimum_content": []string{"evidence kind reconcile-execution-preview", "execution_preview_contract.apply_allowed=false", "inert command templates", "verification hints"},
				"mutates_state":   false,
			})
		case "meshclaw_reconcile_runbook":
			manifest = append(manifest, map[string]interface{}{
				"tool":            tool,
				"fixture":         "reconcile verification-plan evidence",
				"placeholder":     "<test-fixture-reconcile-verification-plan-evidence.json>",
				"purpose":         "exercise review-only runbook contract without executing runbook text",
				"minimum_content": []string{"evidence kind reconcile-verification-plan", "verification_plan_contract.apply_allowed=false", "required evidence checks", "post-action evidence requirements"},
				"mutates_state":   false,
			})
		case "meshclaw_reconcile_runbook_check":
			manifest = append(manifest, map[string]interface{}{
				"tool":            tool,
				"fixture":         "reconcile runbook evidence",
				"placeholder":     "<test-fixture-reconcile-runbook-evidence.json>",
				"purpose":         "exercise gate-only runbook-check contract without granting execution approval",
				"minimum_content": []string{"evidence kind reconcile-runbook", "runbook_contract.review_only=true", "command templates with --require-evidence", "required evidence checks"},
				"mutates_state":   false,
			})
		case "meshclaw_reconcile_rollback_plan":
			manifest = append(manifest, map[string]interface{}{
				"tool":            tool,
				"fixture":         "reconcile runbook-check evidence",
				"placeholder":     "<test-fixture-reconcile-runbook-check-evidence.json>",
				"purpose":         "exercise rollback-plan contract without automatic rollback execution",
				"minimum_content": []string{"evidence kind reconcile-runbook-check", "runbook_check_contract.gate_only=true", "zero critical findings", "ready runbook check"},
				"mutates_state":   false,
			})
		case "meshclaw_reconcile_completion_plan":
			manifest = append(manifest, map[string]interface{}{
				"tool":            tool,
				"fixture":         "reconcile rollback-plan evidence",
				"placeholder":     "<test-fixture-reconcile-rollback-plan-evidence.json>",
				"purpose":         "exercise completion-plan contract without declaring reconcile complete",
				"minimum_content": []string{"evidence kind reconcile-rollback-plan", "rollback_plan_contract.rollback_allowed=false", "rollback guidance", "final evidence requirements"},
				"mutates_state":   false,
			})
		case "meshclaw_reconcile_readiness_summary":
			manifest = append(manifest, map[string]interface{}{
				"tool":            tool,
				"fixture":         "reconcile completion-plan evidence",
				"placeholder":     "<test-fixture-reconcile-completion-plan-evidence.json>",
				"purpose":         "exercise summary-only desired-state executor-contract readiness without server changes",
				"minimum_content": []string{"evidence kind reconcile-completion-plan", "executor_gate_contract", "approval/apply-gate/rollback evidence requirements", "stop_before gates"},
				"mutates_state":   false,
			})
		case "meshclaw_autoheal_container_apply_plan":
			manifest = append(manifest, map[string]interface{}{
				"tool":            tool,
				"fixture":         "autoheal plan evidence",
				"placeholder":     "<test-fixture-autoheal-plan-evidence.json>",
				"purpose":         "exercise approval-gated container apply-plan rendering without docker execution",
				"minimum_content": []string{"evidence kind autoheal-plan", "container action candidate", "approval and rollback context", "runtime_evidence_required", "container_apply_plan_contract.runtime_evidence_required_count", "analyze_logs handoff_contract.apply_allowed=false when sourced from logscan"},
				"mutates_state":   false,
			})
		case "meshclaw_autoheal_container_verification_plan":
			manifest = append(manifest, map[string]interface{}{
				"tool":            tool,
				"fixture":         "container apply-plan evidence",
				"placeholder":     "<test-fixture-container-apply-plan-evidence.json>",
				"purpose":         "exercise container verification-plan contract without docker execution",
				"minimum_content": []string{"evidence kind container-apply-plan", "runtime_evidence_required", "container_apply_plan_contract.runtime_evidence_required_count", "focused container-logscan requirements", "container_verification_plan_contract.apply_allowed=false"},
				"mutates_state":   false,
			})
		case "meshclaw_autoheal_container_runbook":
			manifest = append(manifest, map[string]interface{}{
				"tool":            tool,
				"fixture":         "container verification-plan evidence",
				"placeholder":     "<test-fixture-container-verification-plan-evidence.json>",
				"purpose":         "exercise review-only container runbook contract without executing runbook text",
				"minimum_content": []string{"evidence kind container-verification-plan", "container_verification_plan_contract.requires_container_logscan=true", "apply_plan_runtime_evidence_required_count", "runtime_evidence_required", "container_runbook_contract.review_only=true"},
				"mutates_state":   false,
			})
		case "meshclaw_autoheal_container_runbook_check":
			manifest = append(manifest, map[string]interface{}{
				"tool":            tool,
				"fixture":         "container runbook evidence",
				"placeholder":     "<test-fixture-container-runbook-evidence.json>",
				"purpose":         "exercise gate-only container runbook-check contract without granting execution approval",
				"minimum_content": []string{"evidence kind container-runbook", "container_runbook_contract.review_only=true", "runtime evidence terms", "container_runbook_check_contract.gate_only=true"},
				"mutates_state":   false,
			})
		case "meshclaw_autoheal_container_rollback_plan":
			manifest = append(manifest, map[string]interface{}{
				"tool":            tool,
				"fixture":         "container runbook-check evidence",
				"placeholder":     "<test-fixture-container-runbook-check-evidence.json>",
				"purpose":         "exercise container rollback guidance contract without automatic rollback execution",
				"minimum_content": []string{"evidence kind container-runbook-check", "container_runbook_check_contract.gate_only=true", "zero critical findings", "container_rollback_plan_contract.rollback_allowed=false"},
				"mutates_state":   false,
			})
		case "meshclaw_autoheal_container_completion_plan":
			manifest = append(manifest, map[string]interface{}{
				"tool":            tool,
				"fixture":         "container rollback-plan evidence",
				"placeholder":     "<test-fixture-container-rollback-plan-evidence.json>",
				"purpose":         "exercise container completion-plan contract without declaring repair complete",
				"minimum_content": []string{"evidence kind container-rollback-plan", "container_rollback_plan_contract.rollback_allowed=false", "final container-logscan evidence requirements", "container_completion_plan_contract.complete_allowed=false"},
				"mutates_state":   false,
			})
		case "meshclaw_autoheal_container_readiness_summary":
			manifest = append(manifest, map[string]interface{}{
				"tool":            tool,
				"fixture":         "container completion-plan evidence",
				"placeholder":     "<test-fixture-container-completion-plan-evidence.json>",
				"purpose":         "exercise summary-only apply-loop gate readiness without docker execution",
				"minimum_content": []string{"evidence kind container-completion-plan", "apply_loop_gates", "runtime_evidence_gate", "runtime_evidence_findings", "final logscan and rollback evidence requirements", "stop_before gates"},
				"mutates_state":   false,
			})
		case "meshclaw_autoheal_container_executor_gate":
			manifest = append(manifest, map[string]interface{}{
				"tool":            tool,
				"fixture":         "container readiness-summary evidence",
				"placeholder":     "<test-fixture-container-readiness-summary-evidence.json>",
				"purpose":         "exercise admission-only container executor gate without docker execution",
				"minimum_content": []string{"evidence kind container-readiness-summary", "container_readiness_summary_contract.requires_approval_gated_executor=true", "container_executor_gate_contract.gate_only=true", "container_executor_gate_contract.executor_allowed=false", "requires_dry_run=true"},
				"mutates_state":   false,
			})
		case "meshclaw_autoheal_container_executor":
			manifest = append(manifest, map[string]interface{}{
				"tool":            tool,
				"fixture":         "container executor-gate evidence",
				"placeholder":     "<test-fixture-container-executor-gate-evidence.json>",
				"purpose":         "exercise dry-run container executor preview without docker execution",
				"minimum_content": []string{"evidence kind container-executor-gate", "container_executor_contract.dry_run=true", "requires_exact_live_phrase", "mutates_live_servers=false"},
				"mutates_state":   false,
			})
		case "meshclaw_autoheal_container_executor_verify":
			manifest = append(manifest, map[string]interface{}{
				"tool":            tool,
				"fixture":         "container executor evidence plus post-action evidence references",
				"placeholder":     "<test-fixture-container-executor-evidence.json>",
				"purpose":         "exercise post-action verification closeout gate without docker execution",
				"minimum_content": []string{"evidence kind container-executor", "container_executor_verification_contract.gate_only=true", "requires_agent_evidence=true", "requires_container_logscan=true"},
				"mutates_state":   false,
			})
		case "meshclaw_analyze_logs":
			manifest = append(manifest, map[string]interface{}{
				"tool":            tool,
				"fixture":         "redacted container and systemd runtime log evidence",
				"placeholder":     "<test-fixture-container-logscan-evidence.txt>",
				"purpose":         "exercise logscan to autoheal_handoff routing without live docker or journal access",
				"minimum_content": []string{"healthcheck or restart-loop sample", "systemd Exec format error sample", "WorkingDirectory missing sample", "DNS resolver failure sample", "redacted log_findings", "log_findings unit_candidates field", "meshclaw_service_check arguments hint", "meshclaw_analyze_logs arguments hint", "exec_format_error pattern", "working_directory_missing pattern", "dns_resolver_failure pattern", "autoheal_handoff recommended_tools", "autoheal_handoff confidence", "autoheal_handoff runtime evidence requirements", "autoheal_handoff runtime_evidence_checklist", "docker inspect status/health checklist", "autoheal_handoff unit identity evidence", "stop_before direct apply", "stop_before service restart before unit identity", "must_not direct restart/recreate"},
				"mutates_state":   false,
			})
		}
	}
	return manifest
}

func smokeRunStrategy(tools []string) map[string]interface{} {
	readyNow := make([]string, 0, len(tools))
	requiresFixture := make([]string, 0)
	for _, tool := range tools {
		if smokeFixtureRequiredForTool(tool) {
			requiresFixture = append(requiresFixture, tool)
			continue
		}
		readyNow = append(readyNow, tool)
	}
	return map[string]interface{}{
		"mode":                   "read-only-plan",
		"ready_now":              readyNow,
		"requires_fixture":       requiresFixture,
		"recommended_order":      []string{"surface", "profile_visibility", "recommendation", "ready_now", "fixture_required"},
		"evidence_capture_order": smokeEvidenceCaptureOrder(readyNow, requiresFixture),
		"stop_before_mutation":   true,
		"requires_client_reload": false,
		"notes": []string{
			"run ready_now checks first because they need no local fixture files",
			"prepare fixture_manifest entries before running fixture_required checks",
			"do not add apply=true or execute=true during smoke testing",
		},
	}
}

func smokeSummaryCounts(tools []string) map[string]interface{} {
	readyNow := 0
	fixtureRequired := 0
	for _, tool := range tools {
		if smokeFixtureRequiredForTool(tool) {
			fixtureRequired++
			continue
		}
		readyNow++
	}
	return map[string]interface{}{
		"tools":                   len(tools),
		"ready_now":               readyNow,
		"fixture_required":        fixtureRequired,
		"profile_expected_tools":  len(tools),
		"checks":                  len(tools) + 3,
		"mutating_checks":         0,
		"requires_operator_apply": false,
	}
}

func smokeApprovalBoundary() map[string]interface{} {
	return map[string]interface{}{
		"smoke_allows": []string{
			"tool surface inspection",
			"profile visibility checks",
			"tool recommendation checks",
			"plan-only reports",
			"redacted log evidence collection",
		},
		"smoke_denies": []string{
			"apply=true",
			"execute=true",
			"docker restart",
			"docker recreate",
			"proxy or DNS changes",
			"provider spend",
			"file deletion",
			"client restart",
			"live server deployment",
		},
		"after_smoke_requires": []string{
			"operator approval",
			"stored evidence path",
			"rollback plan",
			"post-action verification plan",
		},
		"decision":               "stop_before_apply",
		"mutates_live_servers":   false,
		"restarts_mcp_client":    false,
		"grants_future_approval": false,
	}
}

func smokeReadiness(tools []string) map[string]interface{} {
	fixtureRequired := make([]string, 0)
	for _, tool := range tools {
		if smokeFixtureRequiredForTool(tool) {
			fixtureRequired = append(fixtureRequired, tool)
		}
	}
	readyNow := len(tools) - len(fixtureRequired)
	status := "ready"
	if len(fixtureRequired) > 0 {
		status = "ready_now_partial"
	}
	return map[string]interface{}{
		"status":                   status,
		"can_run_without_fixtures": readyNow > 0,
		"can_complete_all_checks":  len(fixtureRequired) == 0,
		"ready_now_count":          readyNow,
		"fixture_required_count":   len(fixtureRequired),
		"blocking_fixture_tools":   fixtureRequired,
		"next_step":                "run ready_now checks first, then prepare fixture_manifest before fixture_required checks",
		"operator_action_required": len(fixtureRequired) > 0,
		"mutates_state":            false,
	}
}

func smokeEvidenceCaptureOrder(readyNow, requiresFixture []string) []map[string]interface{} {
	order := []map[string]interface{}{
		{"phase": "surface", "tool": "meshclaw_mcp_surface", "capture": "tool list and default surface guidance"},
		{"phase": "profile_visibility", "tool": "meshclaw_mcp_profile_visibility_check", "capture": "present_tools, missing_tools, and ready flag"},
		{"phase": "recommendation", "tool": "meshclaw_tool_recommend", "capture": "recommended_tool and execution_policy"},
	}
	if len(readyNow) > 0 {
		order = append(order, map[string]interface{}{
			"phase":   "ready_now",
			"tools":   readyNow,
			"capture": "report, evidence path, store_error, and non-mutating result fields",
		})
	}
	if len(requiresFixture) > 0 {
		order = append(order, map[string]interface{}{
			"phase":         "fixture_required",
			"tools":         requiresFixture,
			"capture":       "fixture_manifest path/content plus later report and evidence after fixtures exist",
			"run_after":     "fixture_manifest prepared",
			"mutates_state": false,
		})
	}
	return order
}

func buildMCPProfileVisibilityCheck(args map[string]interface{}) map[string]interface{} {
	profile := normalizeMCPProfile(stringArg(args, "profile"))
	if profile == "" {
		profile = "claude-lite"
	}
	expected := splitCSV(stringArg(args, "expected_tools"))
	visibleTools := mcpToolsForProfile(profile)
	visibleNames := make([]string, 0, len(visibleTools))
	visibleSet := map[string]bool{}
	for _, tool := range visibleTools {
		visibleNames = append(visibleNames, tool.Name)
		visibleSet[tool.Name] = true
	}
	missing := make([]string, 0)
	present := make([]string, 0, len(expected))
	for _, tool := range expected {
		if visibleSet[tool] {
			present = append(present, tool)
		} else {
			missing = append(missing, tool)
		}
	}
	ready := len(expected) > 0 && len(missing) == 0
	return map[string]interface{}{
		"kind":           "mcp_profile_visibility_check",
		"profile":        profile,
		"expected_tools": expected,
		"present_tools":  present,
		"missing_tools":  missing,
		"visible_count":  len(visibleNames),
		"ready":          ready,
		"summary":        fmt.Sprintf("read-only MCP profile visibility profile=%s ready=%t expected=%d missing=%d", profile, ready, len(expected), len(missing)),
		"checks": []map[string]interface{}{
			{"name": "profile_known", "passed": profile == "all" || len(mcpProfileToolNames(profile)) > 0, "profile": profile},
			{"name": "expected_tools_provided", "passed": len(expected) > 0, "count": len(expected)},
			{"name": "all_expected_visible", "passed": len(expected) > 0 && len(missing) == 0, "missing": missing},
		},
		"next_steps": []string{
			"if tools are missing, add them to the selected MCP profile allowlist before client reload",
			"after rebuild, rerun this check against the same profile",
			"only then ask the MCP client to reconnect or restart",
		},
		"writes_profile":  false,
		"restarts_client": false,
		"runs_tools":      false,
	}
}

func buildAutomationRulePlan(args map[string]interface{}) map[string]interface{} {
	name := strings.TrimSpace(stringArg(args, "name"))
	if name == "" {
		name = "<automation-rule>"
	}
	trigger := strings.TrimSpace(stringArg(args, "trigger"))
	if trigger == "" {
		trigger = "container_unhealthy"
	}
	condition := strings.TrimSpace(stringArg(args, "condition"))
	if condition == "" {
		condition = "trigger persists across multiple samples and matching evidence exists"
	}
	action := strings.TrimSpace(stringArg(args, "action"))
	if action == "" {
		action = "autoheal_plan -> approval_request -> apply_plan -> verification_plan"
	}
	scope := strings.TrimSpace(stringArg(args, "scope"))
	if scope == "" {
		scope = "fleet"
	}
	autoApply := boolArg(args, "auto_apply", false)
	return map[string]interface{}{
		"kind":       "automation_rule_plan",
		"name":       name,
		"trigger":    trigger,
		"condition":  condition,
		"action":     action,
		"scope":      scope,
		"auto_apply": autoApply,
		"summary":    fmt.Sprintf("plan-only automation rule %s trigger=%s scope=%s auto_apply=%t", name, trigger, scope, autoApply),
		"rule_contract": map[string]interface{}{
			"writes_rule":       false,
			"requires_review":   true,
			"requires_evidence": true,
			"cooldown_required": true,
			"rollback_required": true,
		},
		"evidence": []string{
			"trigger evidence from monitor/logscan/agent reports",
			"condition evidence showing the issue is persistent and not a stale boot-only finding",
			"policy decision evidence for each planned action",
			"approval evidence before any future mutating apply path",
		},
		"action_chain": []map[string]interface{}{
			{"step": 1, "tool": "meshclaw_tool_recommend", "purpose": "choose the semantic plan surface for the incident"},
			{"step": 2, "tool": "meshclaw_autoheal_plan", "purpose": "produce read-only remediation candidates"},
			{"step": 3, "tool": "meshclaw_messenger_approval_request", "purpose": "ask the operator before any mutating action when required"},
			{"step": 4, "tool": "meshclaw_autoheal_container_apply_plan or meshclaw_reconcile_apply_plan", "purpose": "render approved apply steps as plan-only evidence"},
			{"step": 5, "tool": "verification/runbook/readiness tools", "purpose": "verify and summarize without declaring success from action output alone"},
		},
		"safety_controls": []string{
			"default to auto_apply=false until the operator explicitly approves a narrow allow rule",
			"require cooldown and max-attempts before repeated remediation",
			"require rollback plan and post-action verification before completion",
			"never enable provider spend, deletion, DNS/proxy edits, or container mutation from this plan alone",
		},
		"approval_gates": []string{
			"operator approval before writing schedule/policy/rule state",
			"operator approval before any mutating apply tool",
			"separate approval for future fully automatic apply, if ever enabled",
		},
		"rollback": []string{
			"disable the automation rule before rollback actions",
			"restore previous policy/schedule/rule state from evidence",
			"run readiness summary after rollback to confirm the rule is no longer firing",
		},
	}
}

func buildAutomationRuleCheck(args map[string]interface{}) (map[string]interface{}, error) {
	evidencePath := strings.TrimSpace(stringArg(args, "rule_plan_evidence_path"))
	if evidencePath == "" {
		return nil, fmt.Errorf("rule_plan_evidence_path is required")
	}
	record, err := evidence.Load(evidencePath)
	if err != nil {
		return nil, err
	}
	approvedBy := strings.TrimSpace(stringArg(args, "approved_by"))
	payload, _ := record.Payload.(map[string]interface{})
	contract, _ := payload["rule_contract"].(map[string]interface{})
	checks := []map[string]interface{}{
		{"name": "source_kind", "passed": record.Kind == "automation-rule-plan", "expected": "automation-rule-plan", "actual": record.Kind},
		{"name": "operator_identity", "passed": approvedBy != "", "expected": "approved_by set", "actual": approvedBy},
		{"name": "writes_rule_false", "passed": contract["writes_rule"] == false, "expected": false, "actual": contract["writes_rule"]},
		{"name": "cooldown_required", "passed": contract["cooldown_required"] == true, "expected": true, "actual": contract["cooldown_required"]},
		{"name": "rollback_required", "passed": contract["rollback_required"] == true, "expected": true, "actual": contract["rollback_required"]},
		{"name": "auto_apply_guard", "passed": payload["auto_apply"] == false, "expected": false, "actual": payload["auto_apply"]},
	}
	blockers := make([]string, 0)
	for _, check := range checks {
		if check["passed"] != true {
			blockers = append(blockers, stringMapValue(check, "name"))
		}
	}
	ready := len(blockers) == 0
	return map[string]interface{}{
		"kind":                    "automation_rule_check",
		"rule_plan_evidence_path": evidencePath,
		"source_evidence_kind":    record.Kind,
		"source_evidence_id":      record.ID,
		"source_summary":          record.Summary,
		"approved_by":             approvedBy,
		"ready":                   ready,
		"writes_rule":             false,
		"executes_automation":     false,
		"checks":                  checks,
		"blockers":                blockers,
		"summary":                 fmt.Sprintf("gate-only automation rule check ready=%t blockers=%d", ready, len(blockers)),
		"next_tools": []string{
			"meshclaw_messenger_approval_request",
			"meshclaw_mcp_smoke_test_plan",
			"future automation-rule writer after explicit review only",
		},
		"rollback": []string{
			"do not write the automation rule when any blocker is present",
			"disable any future rule before rollback actions",
			"restore policy/schedule/rule state from evidence before rechecking readiness",
		},
	}, nil
}

func buildAutomationRuleReadinessSummary(args map[string]interface{}) (map[string]interface{}, error) {
	evidencePath := strings.TrimSpace(stringArg(args, "rule_check_evidence_path"))
	if evidencePath == "" {
		return nil, fmt.Errorf("rule_check_evidence_path is required")
	}
	record, err := evidence.Load(evidencePath)
	if err != nil {
		return nil, err
	}
	payload, _ := record.Payload.(map[string]interface{})
	sourceReady, _ := payload["ready"].(bool)
	blockers := stringSliceFromInterface(payload["blockers"])
	checks, _ := payload["checks"].([]interface{})
	if record.Kind != "automation-rule-check" {
		blockers = append(blockers, "source_kind")
	}
	ready := record.Kind == "automation-rule-check" && sourceReady && len(blockers) == 0
	readyStages := []string{
		"automation rule plan evidence exists",
		"rule-check evidence loaded",
	}
	if ready {
		readyStages = append(readyStages, "approval, cooldown, rollback, and auto-apply guard checks passed")
	}
	return map[string]interface{}{
		"kind":                     "automation_rule_readiness_summary",
		"rule_check_evidence_path": evidencePath,
		"source_evidence_kind":     record.Kind,
		"source_evidence_id":       record.ID,
		"source_summary":           record.Summary,
		"ready":                    ready,
		"ready_stages":             readyStages,
		"blockers":                 blockers,
		"check_count":              len(checks),
		"writes_rule":              false,
		"executes_automation":      false,
		"summary":                  fmt.Sprintf("summary-only automation rule readiness ready=%t blockers=%d checks=%d", ready, len(blockers), len(checks)),
		"next_steps": []string{
			"review the readiness summary with the operator",
			"keep rule writing separate from readiness summary evidence",
			"require a future explicit writer and approval before any schedule, policy, or rule state changes",
		},
	}, nil
}

func buildAutomationRuleWriterPlan(args map[string]interface{}) (map[string]interface{}, error) {
	evidencePath := strings.TrimSpace(stringArg(args, "readiness_evidence_path"))
	if evidencePath == "" {
		return nil, fmt.Errorf("readiness_evidence_path is required")
	}
	record, err := evidence.Load(evidencePath)
	if err != nil {
		return nil, err
	}
	ruleStore := strings.TrimSpace(stringArg(args, "rule_store"))
	if ruleStore == "" {
		ruleStore = "~/.meshclaw/rules"
	}
	payload, _ := record.Payload.(map[string]interface{})
	blockers := stringSliceFromInterface(payload["blockers"])
	if record.Kind != "automation-rule-readiness-summary" {
		blockers = append(blockers, "source_kind")
	}
	sourceReady, _ := payload["ready"].(bool)
	ready := record.Kind == "automation-rule-readiness-summary" && sourceReady && len(blockers) == 0
	return map[string]interface{}{
		"kind":                    "automation_rule_writer_plan",
		"readiness_evidence_path": evidencePath,
		"source_evidence_kind":    record.Kind,
		"source_evidence_id":      record.ID,
		"rule_store":              ruleStore,
		"ready":                   ready,
		"blockers":                blockers,
		"writes_rule":             false,
		"executes_automation":     false,
		"summary":                 fmt.Sprintf("plan-only automation rule writer ready=%t blockers=%d store=%s", ready, len(blockers), ruleStore),
		"rule_envelope_preview": map[string]interface{}{
			"schema_version": "automation-rule/v1-draft",
			"enabled":        false,
			"source":         "automation-rule-readiness-summary evidence",
			"rule_store":     ruleStore,
			"requires": []string{
				"separate explicit writer approval",
				"cooldown and max-attempts fields",
				"rollback and disable-rule procedure",
				"post-action verification evidence",
			},
		},
		"next_steps": []string{
			"review this writer plan before implementing a real writer",
			"keep the first writer disabled-by-default",
			"require a separate approval before creating any rule file or scheduler state",
		},
	}, nil
}

func stringSliceFromInterface(value interface{}) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []interface{}:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func parseOptionalDurationArg(args map[string]interface{}, key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(stringArg(args, key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return parsed, nil
}

func floatArg(args map[string]interface{}, key string, fallback float64) float64 {
	switch value := args[key].(type) {
	case float64:
		return value
	case int:
		return float64(value)
	default:
		return fallback
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func runRouterPlanForMCP(plan router.Plan) (interface{}, error) {
	return executeRouterPlan(plan)
}
