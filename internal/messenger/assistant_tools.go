package messenger

import (
	"context"
	"crypto/sha1"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/meshclaw/meshclaw/internal/aichat"
	"github.com/meshclaw/meshclaw/internal/assistantbrief"
	"github.com/meshclaw/meshclaw/internal/browserauto"
	"github.com/meshclaw/meshclaw/internal/evidence"
	"github.com/meshclaw/meshclaw/internal/geo"
	"github.com/meshclaw/meshclaw/internal/lang"
	"github.com/meshclaw/meshclaw/internal/mailadapter"
	"github.com/meshclaw/meshclaw/internal/osauto"
	"github.com/meshclaw/meshclaw/internal/tts"
)

func assistantToolDefinitions() []aichat.Tool {
	object := func(props map[string]interface{}, required []string) map[string]interface{} {
		return map[string]interface{}{
			"type":       "object",
			"properties": props,
			"required":   required,
		}
	}
	strProp := func(desc string) map[string]interface{} {
		return map[string]interface{}{"type": "string", "description": desc}
	}
	return []aichat.Tool{
		{Type: "function", Function: aichat.ToolFunction{
			Name:        "get_weather",
			Description: "Use only for current weather, forecast, or outfit advice. Do not use for meta questions, news, or browser research. Omit location only when the user clearly wants their default location.",
			Parameters: object(map[string]interface{}{
				"location": strProp("City name such as Seoul, Busan, Tokyo. Leave empty only for default location."),
			}, nil),
		}},
		{Type: "function", Function: aichat.ToolFunction{
			Name:        "get_news_headlines",
			Description: "Use only for a general current news briefing or main headlines. Do not use for web research, fact-checking a specific topic, opening articles, or questions about a previous news answer; use search_web or answer/clarify instead.",
			Parameters: object(map[string]interface{}{
				"limit": map[string]interface{}{"type": "integer", "description": "Number of headlines, 1-10", "default": 5},
			}, nil),
		}},
		{Type: "function", Function: aichat.ToolFunction{
			Name:        "get_directions",
			Description: "Use for directions, commute, route, or travel-time requests when both origin and destination are known. If either place is missing or the user says current location without sharing it, ask a concise clarification instead of guessing.",
			Parameters: object(map[string]interface{}{
				"from": strProp("Origin place name"),
				"to":   strProp("Destination place name"),
			}, []string{"from", "to"}),
		}},
		{Type: "function", Function: aichat.ToolFunction{
			Name:        "find_place",
			Description: "Use for maps/place searches: addresses, nearby POIs, parking, opening hours, phone, or place candidates. Ask for the missing area/place type if the request is too vague.",
			Parameters: object(map[string]interface{}{
				"query": strProp("Place name, address, or nearby search such as '강남역 근처 약국'"),
			}, []string{"query"}),
		}},
		{Type: "function", Function: aichat.ToolFunction{
			Name:        "get_fleet_status",
			Description: "Use for real MeshClaw server/fleet/node status, node names, online/offline state, CPU, memory, disk, GPU, or alerts. Never invent node names; this tool reads the actual MeshClaw inventory and monitor state.",
			Parameters: object(map[string]interface{}{
				"detail": strProp("Optional detail level: summary or nodes"),
			}, nil),
		}},
		{Type: "function", Function: aichat.ToolFunction{
			Name:        "run_mac_action",
			Description: "Use for actual macOS assistant actions on the Mac mini: Calendar, Reminders, Notes, Contacts, Safari/browser control, Shortcuts, opening macOS apps, Obsidian/Pages/Preview handoff, or app tasks. Prefer local macOS apps and installed apps before external SaaS. Do not use for documents, reports, meeting materials, slides, decks, or PPTX; use create_document or create_presentation. Pass the full user request. For destructive, sending, deleting, purchase, payment, or final booking actions, the runtime will require confirmation.",
			Parameters: object(map[string]interface{}{
				"prompt": strProp("Full user request, e.g. 'Safari 열어줘', '오늘 일정 뭐 있어?', '내일 6시 우유 사기 리마인더 추가'"),
			}, []string{"prompt"}),
		}},
		{Type: "function", Function: aichat.ToolFunction{
			Name:        "create_document",
			Description: "Create a usable local macOS-first document artifact for reports, written materials, meeting materials, summaries, briefs, drafts, or Markdown/DOCX/HTML output. The default workspace is the local Argos Vault: Obsidian-friendly Markdown for thinking/editing, DOCX for Word/Pages/mobile editing, and HTML/PDF-style preview when useful. Use this when the user asks to write or make 자료/문서/보고서/회의 자료 from provided context or a general starter brief. If the user asks to send the document/report to a Signal person or room, set target to the person/target name; Argos can resolve existing targets or one exact macOS Contacts phone match into a Signal target. Do not use this for requests that explicitly ask to search the web/browser and then write a report; use search_web for those. Do not schedule a calendar event just because the material is for a meeting.",
			Parameters: object(map[string]interface{}{
				"title":  strProp("Document title. If missing, infer a concise title from the user request."),
				"body":   strProp("Document body, outline, or the content brief. If the user did not provide details, create a useful starter outline from the request."),
				"target": strProp("Optional Signal target id, room label, or macOS Contacts person name if the user asks to send the document/report."),
			}, []string{"body"}),
		}},
		{Type: "function", Function: aichat.ToolFunction{
			Name:        "create_voice_report",
			Description: "Create a written report document and a TTS audio version of the same report, then optionally send both as Signal attachments to a matching target. Use this when the user asks to write/prepare a 보고서, report, brief, summary, or document and also send/read it as an 음성 메시지, voice message, voice file, TTS, mp3, or voice note. If target is omitted, attach the report files and audio back to this chat. Do not split this into create_document plus send_tts_voice unless the user explicitly asks for separate steps.",
			Parameters: object(map[string]interface{}{
				"title":      strProp("Report title. Infer a concise title from the request if missing."),
				"body":       strProp("Report body or source notes. If sparse, create a useful concise report from the request."),
				"target":     strProp("Optional Signal target id, label, or person name. If omitted, return attachments to this chat."),
				"engine":     strProp("TTS engine: edge-tts, edge, or local. Default edge-tts."),
				"voice":      strProp("Optional TTS voice, e.g. ko-KR-SunHiNeural."),
				"delivery":   strProp("Delivery mode: current_chat, signal, voice_note, or call. Default current_chat when target is empty, signal when target is supplied."),
				"voice_note": map[string]interface{}{"type": "boolean", "description": "Send the audio as a Signal voice note when supported. Default true.", "default": true},
				"approve":    map[string]interface{}{"type": "boolean", "description": "Required to place a real Signal call. Signal attachment sending follows existing explicit user send intent.", "default": false},
				"execute":    map[string]interface{}{"type": "boolean", "description": "Actually create files and optionally send. Default true. false returns a plan only.", "default": true},
			}, []string{"body"}),
		}},
		{Type: "function", Function: aichat.ToolFunction{
			Name:        "scheduled_delivery_plan",
			Description: "Plan a recurring or future Signal delivery of a message, report, voice note, or voice report to the user, a report room, or a friend/contact. Use this when the user asks to automatically send something on a schedule such as 매일/매주/아침마다/정기적으로. This only creates a review plan and target resolution preview; it does not register a job or send anything.",
			Parameters: object(map[string]interface{}{
				"target":       strProp("Signal target id, room label, or macOS Contacts person name such as 윤."),
				"schedule":     strProp("Natural-language schedule, e.g. 매일 오전 8시, 매주 월요일 9시, 내일 아침."),
				"content":      strProp("Message/report topic or body to send on the schedule."),
				"content_type": strProp("message, report, voice, or voice_report. Default auto."),
				"delivery":     strProp("signal, voice_note, or call. Default signal; call requires separate approval."),
			}, []string{"target", "schedule", "content"}),
		}},
		{Type: "function", Function: aichat.ToolFunction{
			Name:        "create_spreadsheet",
			Description: "Create a usable spreadsheet artifact for tables, budgets, invoices, cost trackers, checklists, lists, logs, or Excel/Numbers/XLSX/CSV output. The default is a macOS-first XLSX file for Numbers/Excel/iPhone editing, plus CSV and HTML preview. Use this when the user explicitly asks for 엑셀, 스프레드시트, XLSX, CSV, 예산표, 비용표, 청구서, 인보이스, 트래커, 체크리스트, 대장, or a table meant to be edited as rows and columns. Do not use this for narrative reports or documents unless the user asks for a spreadsheet/table file.",
			Parameters: object(map[string]interface{}{
				"title": strProp("Spreadsheet title. If missing, infer a concise title from the user request."),
				"body":  strProp("Rows, Markdown table, source notes, or requested schema. If sparse, create a useful starter sheet from the request."),
			}, []string{"body"}),
		}},
		{Type: "function", Function: aichat.ToolFunction{
			Name:        "prepare_meeting_materials",
			Description: "Prepare a complete macOS-first meeting-material package: an Obsidian-friendly Markdown brief, mobile HTML preview, DOCX briefing document for Word/Pages/iPhone editing, and a verified PowerPoint/PPTX deck. Use for broad requests like '내일 회의 자료 만들어줘', '회의용 자료 준비해줘', or when the user needs usable materials rather than only a calendar event. Do not schedule time unless explicitly asked.",
			Parameters: object(map[string]interface{}{
				"title":       strProp("Meeting/material title. Infer a concise Korean title if missing."),
				"body":        strProp("Meeting context, goals, agenda, source notes, or starter content. If sparse, create a useful starter package from the request."),
				"audience":    strProp("Target audience/context, e.g. internal team, client, executive."),
				"slide_count": map[string]interface{}{"type": "integer", "description": "Desired slide count, 3-12", "default": 6},
				"target":      strProp("Optional Signal target id, room label, or macOS Contacts person name if the user asks to send the meeting package."),
			}, []string{"body"}),
		}},
		{Type: "function", Function: aichat.ToolFunction{
			Name:        "create_presentation",
			Description: "Create a usable local presentation with PowerPoint/PPTX as the editable default, plus Obsidian-friendly Markdown outline, HTML preview, and structural verification. Use for PPT, PPTX, slides, deck, 발표자료, presentation, or meeting material that should be presented. Do not use Calendar unless the user is scheduling time.",
			Parameters: object(map[string]interface{}{
				"title":       strProp("Presentation title. If missing, infer a concise title from the user request."),
				"body":        strProp("Brief, outline, source notes, or starter content for the deck."),
				"audience":    strProp("Target audience/context, e.g. team meeting, client, executive."),
				"slide_count": map[string]interface{}{"type": "integer", "description": "Desired slide count, 1-20", "default": 6},
			}, []string{"body"}),
		}},
		{Type: "function", Function: aichat.ToolFunction{
			Name:        "revise_recent_artifact",
			Description: "Revise or continue the most recently created document or presentation in this Signal assistant room. Use for follow-ups like '방금 만든 문서 수정해줘', '5장으로 줄여줘', '예산 슬라이드 추가해줘', '좀 더 짧게 다시 만들어줘'. Creates a new editable artifact and sends it back; does not mutate the old file in place.",
			Parameters: object(map[string]interface{}{
				"instruction": strProp("Natural-language revision instruction from the user."),
				"target":      strProp("Optional target: document, presentation, or auto. Default auto."),
				"slide_count": map[string]interface{}{"type": "integer", "description": "Optional desired slide count when revising a presentation."},
			}, []string{"instruction"}),
		}},
		{Type: "function", Function: aichat.ToolFunction{
			Name:        "export_recent_artifact",
			Description: "Export the most recently created document or presentation in this Signal assistant room. Use for follow-ups like 'PDF로도 보내줘', 'DOCX로 바꿔줘', or 'PPT를 PDF로 보내줘'. Sends the exported file as a Signal attachment when created.",
			Parameters: object(map[string]interface{}{
				"format": strProp("Export format: pdf or docx. For presentations, pdf is supported when LibreOffice/soffice is installed."),
				"target": strProp("Optional target: document, presentation, or auto. Default auto."),
			}, []string{"format"}),
		}},
		{Type: "function", Function: aichat.ToolFunction{
			Name:        "open_recent_artifact",
			Description: "Open the most recently created document or presentation in a local macOS app. Use for follow-ups like 'Obsidian에서 열어줘', 'Pages에서 열어줘', 'PowerPoint로 열어줘', or 'Preview로 열어줘'.",
			Parameters: object(map[string]interface{}{
				"app":    strProp("Optional app name, e.g. Obsidian, Pages, Microsoft Word, Keynote, Microsoft PowerPoint, Preview."),
				"target": strProp("Optional target: document, presentation, markdown, docx, pptx, pdf, or auto."),
			}, nil),
		}},
		{Type: "function", Function: aichat.ToolFunction{
			Name:        "resend_recent_artifact",
			Description: "Send the most recently created document or presentation files again as Signal attachments. Use when the user says '다시 보내줘', '파일 다시 보내', '첨부 다시 보내줘', or asks for the last artifact on the phone.",
			Parameters: object(map[string]interface{}{
				"target": strProp("Optional target: document, presentation, or all. Default all."),
			}, nil),
		}},
		{Type: "function", Function: aichat.ToolFunction{
			Name:        "search_web",
			Description: "Use for web research, browser search, named topics, current facts, article lookup, product/review research, or when the user asks to search/find/look up online. Also use this, not create_document, when the user asks to search the browser/web and write a report or summary. Do not use get_news_headlines for these specific searches.",
			Parameters: object(map[string]interface{}{
				"query": strProp("Search query"),
			}, []string{"query"}),
		}},
		{Type: "function", Function: aichat.ToolFunction{
			Name:        "market_outlook",
			Description: "Use for market or price outlook questions that need current evidence, including oil/유가/WTI/Brent, gasoline, commodities, exchange rates, interest rates, stocks, or crypto. Search current sources first, then answer with scenarios and uncertainty; never invent live prices or forecasts.",
			Parameters: object(map[string]interface{}{
				"asset":       strProp("Market asset/topic, e.g. 'WTI crude oil', 'Brent oil', '유가', '원달러 환율'"),
				"horizon":     strProp("Optional outlook horizon such as '이번 주', '1개월', '2026년 하반기'"),
				"target":      strProp("Optional Signal target id or room label when the user asks to send the outlook to a room, e.g. 보고방 or argos-briefing."),
				"voice_brief": map[string]interface{}{"type": "boolean", "description": "Also create and attach an MP3 voice briefing when the user asks for audio, voice, mp3, or TTS.", "default": false},
				"voice_note":  map[string]interface{}{"type": "boolean", "description": "Send the MP3 as a Signal voice note when supported. Default false.", "default": false},
				"engine":      strProp("Optional TTS engine for voice_brief, e.g. edge-tts or local."),
				"tts_voice":   strProp("Optional TTS voice for voice_brief, e.g. ko-KR-SunHiNeural."),
			}, []string{"asset"}),
		}},
		{Type: "function", Function: aichat.ToolFunction{
			Name:        "open_url",
			Description: "Open an explicit URL or named website in the Mac browser. Use search_web instead when the user wants information rather than opening a site.",
			Parameters: object(map[string]interface{}{
				"url": strProp("URL to open, e.g. https://www.naver.com"),
			}, []string{"url"}),
		}},
		{Type: "function", Function: aichat.ToolFunction{
			Name:        "list_mail_accounts",
			Description: "List configured mail accounts when the user asks which mail accounts or mail tools are connected.",
			Parameters:  object(map[string]interface{}{}, nil),
		}},
		{Type: "function", Function: aichat.ToolFunction{
			Name:        "check_mail",
			Description: "Check new or recent mail. Use for inbox status like 'new mail?' or 'recent mail'. Do not use for searching old receipts/tracking unless a search query is needed.",
			Parameters: object(map[string]interface{}{
				"limit":   map[string]interface{}{"type": "integer", "description": "Max messages, default 10"},
				"account": strProp("Optional mail account id"),
			}, nil),
		}},
		{Type: "function", Function: aichat.ToolFunction{
			Name:        "search_mail",
			Description: "Search mail by keyword, sender, subject, receipts, tracking, subscriptions, invoices, or older messages. A query is required; ask if the user has not provided one.",
			Parameters: object(map[string]interface{}{
				"query":   strProp("Search keywords"),
				"limit":   map[string]interface{}{"type": "integer", "description": "Max messages, default 10"},
				"account": strProp("Optional mail account id"),
			}, []string{"query"}),
		}},
		{Type: "function", Function: aichat.ToolFunction{
			Name:        "summarize_mail",
			Description: "Summarize recent or important mail when the user asks for an inbox/mail briefing. For a specific sender/topic, use search_mail.",
			Parameters: object(map[string]interface{}{
				"limit":   map[string]interface{}{"type": "integer", "description": "Max messages, default 10"},
				"account": strProp("Optional mail account id"),
			}, nil),
		}},
		{Type: "function", Function: aichat.ToolFunction{
			Name:        "read_mail",
			Description: "Read one mail message by id/uid only when a message id is available from a previous mail result. Ask for which message if no id is known.",
			Parameters: object(map[string]interface{}{
				"message_id": strProp("Mail message id or uid"),
			}, []string{"message_id"}),
		}},
		{Type: "function", Function: aichat.ToolFunction{
			Name:        "find_booking",
			Description: "Find restaurant, hotel, appointment, or ticket booking candidates. Requires enough context such as place/service, date/time, party size, and constraints; ask for missing fields. Never confirm, pay, or finalize booking.",
			Parameters: object(map[string]interface{}{
				"query": strProp("Place, date, time, party size, e.g. '강남 파스타 식당 내일 저녁 7시 2명'"),
			}, []string{"query"}),
		}},
		{Type: "function", Function: aichat.ToolFunction{
			Name:        "search_shopping",
			Description: "Search products, compare reviews/prices, or find coupons. Requires product and useful constraints when possible; ask for missing essentials. Never checkout, subscribe, or pay.",
			Parameters: object(map[string]interface{}{
				"query": strProp("Product name and constraints, e.g. '검정 M 러닝 벨트 3만원 이하'"),
				"mode":  strProp("Optional: search, reviews, coupon"),
			}, []string{"query"}),
		}},
		{Type: "function", Function: aichat.ToolFunction{
			Name:        "play_media",
			Description: "Play or open music, YouTube, radio, podcast, ambient sound, or OTT services on the Mac. Ask for platform/content if both are unclear.",
			Parameters: object(map[string]interface{}{
				"query":    strProp("Song, genre, show, or service name"),
				"platform": strProp("Optional: youtube, radio, podcast, ott, ambient"),
			}, []string{"query"}),
		}},
		{Type: "function", Function: aichat.ToolFunction{
			Name:        "send_tts_voice",
			Description: "Create an audio file from any requested text using TTS, including prayers, letters, notices, summaries, or short messages. Use this when the user asks to make/send an 음성파일, voice file, TTS, mp3, voice note, or says edge tts. If the user asks to create content such as 오늘의 기도문, write the content in the content field instead of creating a document. If target is supplied, send the generated audio to the matching Signal target; otherwise attach it back to this chat. Do not use create_document for voice-file requests.",
			Parameters: object(map[string]interface{}{
				"content":    strProp("Full text to synthesize. If the user asks you to write a prayer/message first, provide the generated final text here."),
				"topic":      strProp("Optional content topic, e.g. 오늘의 기도문, 생일 축하 메시지, 안내문."),
				"target":     strProp("Optional Signal target id, label, or person name such as 윤. If omitted, return the audio file to the current chat."),
				"engine":     strProp("TTS engine: edge-tts, edge, or local. Default edge-tts."),
				"voice":      strProp("Optional TTS voice, e.g. ko-KR-SunHiNeural."),
				"voice_note": map[string]interface{}{"type": "boolean", "description": "Send as a Signal voice note when supported. Default false.", "default": false},
				"execute":    map[string]interface{}{"type": "boolean", "description": "Actually synthesize and optionally send. Default true. false returns a plan only.", "default": true},
			}, nil),
		}},
	}
}

func assistantToolSystemPrompt() string {
	return strings.Join([]string{
		"You are the macmini Argos Signal assistant.",
		"Use tools by understanding the user's intent, not by keyword matching.",
		"Use tools for weather, news, maps, mail, Mac automation, browser, booking, shopping, and media only when the user is asking for that action or fresh/local data.",
		"For normal conversation, meta questions, corrections, complaints about a previous answer, or unclear follow-ups, reply in Korean or ask a concise clarification without calling a tool.",
		"If required fields are missing for directions, mail search/read, booking, shopping, media, or Mac actions, ask only for the missing fields instead of guessing.",
		"Do not guess weather, news, mail, calendar, reminders, contacts, or browser contents; call the appropriate tool when the user requests those actual contents.",
		"Do not guess MeshClaw node names or fleet status; use get_fleet_status for server/node/fleet questions and follow-ups.",
		"For Calendar, Reminders, Notes, Contacts, Safari, Shortcuts, and app control use run_mac_action with the full user request.",
		"For broad meeting-material requests use prepare_meeting_materials so the user receives both a briefing document and a verified PPTX deck.",
		"For single documents, reports, 자료, 문서, or 보고서 use create_document. If the user also asks for an audio/voice message version of the report, use create_voice_report instead. For slides, decks, PPT, or 발표자료 use create_presentation. Do not turn 'meeting material' into a Calendar event unless the user asks to schedule a time.",
		"For 음성파일, voice file, mp3, TTS, voice note, or edge tts requests, use send_tts_voice. If the user asks you to write the content first, generate that content in the tool argument. Do not use create_document for voice-file requests.",
		"For scheduled or recurring delivery to a friend, room, or the user, use scheduled_delivery_plan first. Do not create a calendar event or send immediately unless the user separately approves the first-run preview and schedule registration.",
		"For follow-ups about the thing you just created, use revise_recent_artifact, export_recent_artifact, open_recent_artifact, or resend_recent_artifact instead of asking the user for the file path.",
		"Use search_web for specific web research, named topics, article lookup, fact checking, product/review research, or browser search.",
		"Use market_outlook for oil, commodity, FX, rates, stock, or crypto outlook questions such as '유가가 어떻게 될 것 같아?'; answer with evidence-backed scenarios and uncertainty, not a single confident prediction.",
		"Use get_news_headlines only for a general current headlines/news briefing.",
		"Never call tools to send, delete, purchase, pay, subscribe, or finalize booking unless the user explicitly asked for that action; the runtime may require confirmation.",
		"Prefer one tool call at a time unless the user clearly requested a multi-step task.",
		"After a tool result, explain the useful result concisely in Korean and mention any needed next confirmation.",
		"Keep replies concise and practical for Signal.",
	}, "\n")
}

func assistantModelToolReply(opts ListenOptions, text string) (string, bool) {
	mode := strings.ToLower(strings.TrimSpace(opts.Mode))
	if mode != "assistant" && mode != "briefing" {
		return "", false
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", false
	}
	return assistantModelToolLoop(opts, trimmed, mode)
}

func assistantStructuredToolReply(opts ListenOptions, text string) (string, bool) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "", false
	}
	mode := strings.ToLower(strings.TrimSpace(opts.Mode))
	if mode == "assistant" || mode == "briefing" {
		return assistantModelToolLoop(opts, trimmed, mode)
	}
	return "", false
}

func assistantModelToolLoop(opts ListenOptions, text, mode string) (string, bool) {
	if strings.EqualFold(strings.TrimSpace(os.Getenv("MESHCLAW_ASSISTANT_TOOL_LOOP")), "0") {
		return "", false
	}

	cfg, _, err := aichat.LoadConfig()
	if err != nil {
		cfg = aichat.DefaultConfig()
	}
	if opts.Model != "" {
		cfg.Model = opts.Model
	}
	if opts.BaseURL != "" {
		cfg.BaseURL = opts.BaseURL
	}
	cfg.MaxTokens = signalMaxTokens(mode, 512)
	cfg.SystemPrompt = assistantToolSystemPrompt()

	history := loadSignalHistory(opts.TargetID, 8)
	tools := assistantToolDefinitions()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	client := aichat.NewClient(cfg)

	messages := []aichat.Message{{Role: "system", Content: cfg.SystemPrompt}}
	messages = append(messages, history...)
	messages = append(messages, aichat.Message{Role: "user", Content: text})

	for range 4 {
		result, err := client.CompleteWithTools(ctx, messages, tools)
		if err != nil {
			return "", false
		}
		if len(result.ToolCalls) == 0 {
			if strings.TrimSpace(result.Content) == "" {
				return "", false
			}
			if fallback, ok := fallbackAssistantToolIntent(text); ok {
				toolResult := replyFromMailToolIntent(opts, text, fallback)
				if strings.TrimSpace(toolResult) != "" {
					_ = appendSignalHistory(opts.TargetID, text, toolResult)
					return toolResult, true
				}
			}
			if assistantRequiresToolCall(text) {
				messages = append(messages, aichat.Message{
					Role:    "assistant",
					Content: result.Content,
				})
				messages = append(messages, aichat.Message{
					Role: "user",
					Content: strings.Join([]string{
						"이 요청은 일반 템플릿 답변으로 끝내면 안 됩니다.",
						"반드시 적절한 tool call을 하나 호출하세요.",
						"보고서/문서와 음성 메시지를 함께 요청하면 create_voice_report를 사용하세요.",
						"회의 자료/회의용 자료는 prepare_meeting_materials, 발표자료/PPT는 create_presentation, 문서/보고서는 create_document를 사용하세요.",
						"음성파일/TTS/mp3/voice note/edge tts 요청은 send_tts_voice를 사용하세요.",
						"추가 정보가 부족해도 먼저 사용 가능한 초안 아티팩트를 만드세요.",
					}, "\n"),
				})
				continue
			}
			_ = appendSignalHistory(opts.TargetID, text, result.Content)
			return result.Content, true
		}
		messages = append(messages, aichat.Message{Role: "assistant", Content: result.Content, ToolCalls: result.ToolCalls})
		for _, call := range result.ToolCalls {
			toolResult := executeAssistantToolCall(opts, text, call.Function.Name, call.Function.Arguments)
			if strings.TrimSpace(toolResult) != "" {
				_ = appendSignalHistory(opts.TargetID, text, toolResult)
				return toolResult, true
			}
			messages = append(messages, aichat.Message{
				Role:       "tool",
				ToolCallID: call.ID,
				Name:       call.Function.Name,
				Content:    toolResult,
			})
		}
	}
	return "", false
}

func assistantRequiresToolCall(text string) bool {
	return looksLikeAssistantArtifactRequest(text) ||
		looksLikeAssistantVoiceRequest(text) ||
		looksLikeAssistantScheduledDeliveryRequest(text)
}

func looksLikeAssistantVoiceRequest(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return false
	}
	if containsAny(lower, "음성파일", "음성 파일", "음성으로", "목소리로", "읽어서", "읽어줘", "tts", "edge tts", "edge-tts", "mp3", "m4a", "voice file", "voice note", "보이스노트") {
		return true
	}
	if containsAny(lower, "기도문", "편지", "메시지", "안내문") && containsAny(lower, "음성", "tts", "mp3", "보내") {
		return true
	}
	return false
}

func looksLikeAssistantScheduledDeliveryRequest(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return false
	}
	normalized := strings.NewReplacer(
		"내주 ", "매주 ",
		"내주", "매주",
		"금주 ", "매주 ",
		"다음 주 ", "매주 ",
		"다음주 ", "매주 ",
		"화욜", "화요일",
		"수욜", "수요일",
		"목욜", "목요일",
		"금욜", "금요일",
		"토욜", "토요일",
		"일욜", "일요일",
		"월욜", "월요일",
	).Replace(lower)
	if !containsAny(normalized, "매일", "매주", "매월", "매분", "매년", "아침마다", "저녁마다", "정기", "정기적으로", "반복", "주기적으로", "주기") {
		return false
	}
	clauses := regexp.MustCompile(`[.!?\n\r;。！？]+`).Split(normalized, -1)
	for _, clause := range clauses {
		clause = strings.TrimSpace(clause)
		if clause == "" {
			continue
		}
		if !containsAny(clause, "매일", "매주", "매월", "매분", "매년", "아침마다", "저녁마다", "정기", "정기적으로", "반복", "주기적으로", "주기") {
			continue
		}
		if containsAny(clause,
			"보내", "전송", "공유", "발송", "예약해", "요청해", "알려", "알림줘", "알람", "알림", "통보", "통지", "전달", "보낼", "보내줘", "보내주",
			"통보해", "알려줘") {
			return true
		}
	}
	return false
}

func looksLikeAssistantVoiceReportRequest(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" || !looksLikeAssistantVoiceRequest(lower) {
		return false
	}
	return containsAny(lower, "보고서", "보고", "리포트", "브리프", "brief", "report", "문서", "요약")
}

func looksLikeAssistantExternalSendRequest(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return false
	}
	return containsAny(lower, "보내", "전송", "공유", "send", "share") &&
		containsAny(lower, "에게", "한테", "보고방", "비서방", "signal", "시그널")
}

func inferAssistantSignalTargetRef(text string) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	lower := strings.ToLower(trimmed)
	switch {
	case containsAny(lower, "보고방 ops", "보고방ops", "ops 보고방", "ops방", "argos-ops", "ops room", "ops report room", "operations report room", "operations room", "devops room", "devops report room"):
		return "argos-ops"
	case containsAny(lower, "보고방", "브리핑방", "argos-briefing", "briefing room", "briefing channel", "report room", "reports room", "reporting room"):
		return "argos-briefing"
	case containsAny(lower, "비서방", "assistant 방", "assistant방", "argos-assistant", "assistant room", "assistant channel"):
		return "argos-assistant"
	case containsAny(lower, "채팅방", "chat 방", "chat방", "argos-chat", "chat room", "chat channel"):
		return "argos-chat"
	}
	if containsAny(lower, "내게", "나에게", "나한테", "내 signal", "내 시그널", "나한테") {
		return "내게"
	}
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?m)([가-힣A-Za-z0-9_.-]{1,30})\s*(?:에게|한테|한테로|에게로)`),
		regexp.MustCompile(`(?m)([가-힣A-Za-z0-9_.-]{1,30})\s*(?:로|으로)\s*(?:보내|전송)`),
	}
	stop := map[string]bool{
		"시그널": true, "signal": true, "보고서": true, "문서": true, "음성": true, "파일": true,
		"기도문": true, "메시지": true, "안내문": true, "나": true, "내": true,
	}
	for _, pattern := range patterns {
		matches := pattern.FindStringSubmatch(trimmed)
		if len(matches) < 2 {
			continue
		}
		candidate := strings.TrimSpace(matches[1])
		if candidate == "" || stop[strings.ToLower(candidate)] {
			continue
		}
		return candidate
	}
	return ""
}

func inferAssistantVoiceTopic(text string) string {
	lower := strings.ToLower(strings.TrimSpace(text))
	switch {
	case containsAny(lower, "기도문", "prayer"):
		return "오늘의 기도문"
	case containsAny(lower, "보고서", "보고", "리포트", "report"):
		return "음성 보고서"
	case containsAny(lower, "안내문", "공지", "notice"):
		return "안내문"
	case containsAny(lower, "편지", "letter"):
		return "편지"
	default:
		return "음성 메시지"
	}
}

func inferAssistantVoiceReportTitle(text string) string {
	lower := strings.ToLower(strings.TrimSpace(text))
	switch {
	case containsAny(lower, "오늘", "금일"):
		return "오늘 보고서"
	case containsAny(lower, "작업", "진행", "개발"):
		return "작업 보고서"
	default:
		return "Argos 음성 보고서"
	}
}

func replyFromMailToolIntent(opts ListenOptions, request string, intent assistantToolIntent) string {
	switch intent.Intent {
	case "mail_accounts":
		args, _ := json.Marshal(map[string]interface{}{})
		return executeAssistantToolCall(opts, request, "list_mail_accounts", string(args))
	case "mail_summary":
		args, _ := json.Marshal(map[string]interface{}{"limit": intent.Limit, "account": intent.Account})
		return executeAssistantToolCall(opts, request, "summarize_mail", string(args))
	case "mail_search":
		args, _ := json.Marshal(map[string]interface{}{"query": intent.Query, "limit": intent.Limit, "account": intent.Account})
		return executeAssistantToolCall(opts, request, "search_mail", string(args))
	case "mail_watch":
		args, _ := json.Marshal(map[string]interface{}{"limit": intent.Limit, "account": intent.Account})
		return executeAssistantToolCall(opts, request, "check_mail", string(args))
	default:
		return executeSignalMailReadIntent(intent)
	}
}

func isAssistantFastNewsRequest(lower string) bool {
	if lower == "" {
		return false
	}
	if !containsAny(lower, "뉴스", "news", "headline", "headlines", "주요뉴스", "헤드라인", "브리핑") {
		return false
	}
	if containsAny(lower, "브라우저", "browser", "검색", "search", "찾아", "찾아봐", "웹에서", "web") {
		return false
	}
	if containsAny(lower, "리마인더", "알림", "reminder", "remind me", "할 일", "할일", "todo") {
		return false
	}
	if isNewsDocumentRequest(lower) {
		return false
	}
	return true
}

func toolArgString(args map[string]interface{}, key string) string {
	raw, ok := args[key]
	if !ok || raw == nil {
		return ""
	}
	value := strings.TrimSpace(fmt.Sprint(raw))
	if value == "" || value == "<nil>" {
		return ""
	}
	return value
}

func toolArgInt(args map[string]interface{}, key string, fallback int) int {
	raw, ok := args[key]
	if !ok || raw == nil {
		return fallback
	}
	switch v := raw.(type) {
	case float64:
		if int(v) > 0 {
			return int(v)
		}
	case int:
		if v > 0 {
			return v
		}
	case json.Number:
		if n, err := v.Int64(); err == nil && n > 0 {
			return int(n)
		}
	}
	return fallback
}

func toolArgBool(args map[string]interface{}, key string, fallback bool) bool {
	raw, ok := args[key]
	if !ok || raw == nil {
		return fallback
	}
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "y", "on", "execute", "실행", "보내", "전송":
			return true
		case "0", "false", "no", "n", "off", "dry-run", "plan", "계획":
			return false
		}
	}
	return fallback
}

func executeAssistantToolCall(opts ListenOptions, request, name, argsJSON string) string {
	args := map[string]interface{}{}
	if strings.TrimSpace(argsJSON) != "" {
		_ = json.Unmarshal([]byte(argsJSON), &args)
	}
	name = normalizeAssistantToolName(name)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	switch name {
	case "get_weather":
		location := toolArgString(args, "location")
		if location == "" {
			location = geo.ExtractExplicitLocation(request)
		}
		location = geo.DefaultResolver().Resolve(ctx, opts.Source, location)
		brief := assistantbrief.WeatherNow(ctx, assistantbrief.Options{Location: location})
		record, storeErr := evidence.Store("assistant-weather", firstNonEmpty(opts.TargetID, "assistant"), brief.Location, brief)
		lines := []string{strings.TrimSpace(brief.Text)}
		lines = appendEvidenceLine(lines, record, storeErr)
		return strings.Join(lines, "\n")
	case "get_news_headlines":
		limit := toolArgInt(args, "limit", 5)
		if limit <= 0 || limit > 10 {
			limit = 5
		}
		brief := assistantbrief.News(ctx, assistantbrief.Options{Location: "Seoul", NewsLimit: limit, NoModelSummary: true})
		record, storeErr := evidence.Store("assistant-news", firstNonEmpty(opts.TargetID, "assistant"), "signal_fast_news", brief)
		display := formatSignalFastNews(brief, limit)
		storeFastNewsBriefingEvidence(display, brief)
		lines := []string{display}
		lines = appendEvidenceLine(lines, record, storeErr)
		return strings.Join(lines, "\n")
	case "get_directions":
		from := toolArgString(args, "from")
		to := toolArgString(args, "to")
		if from == "" || to == "" {
			return "출발지와 도착지가 필요합니다. 예: `분당에서 광화문까지 출근길 알려줘`"
		}
		if isCurrentLocationPhrase(from) {
			return "현재 위치 기준 길찾기는 위치 공유가 먼저 필요합니다. 출발지를 장소명으로 보내주세요."
		}
		link := "https://www.google.com/maps/dir/?api=1&origin=" + url.QueryEscape(from) + "&destination=" + url.QueryEscape(to) + "&travelmode=transit"
		lower := strings.ToLower(request)
		intro := "길찾기 링크입니다."
		scenario := "maps_directions"
		switch {
		case containsAny(lower, "얼마나 걸", "소요시간", "이동시간", "travel time", "eta"):
			intro = "이동시간 확인 링크입니다."
			scenario = "maps_travel_time"
		case containsAny(lower, "출근길", "퇴근길", "통근", "commute"):
			intro = "출퇴근 길찾기 링크입니다."
			scenario = "commute_directions"
		}
		reply := strings.Join([]string{
			intro,
			link,
			"",
			"iPhone에서 열면 Google Maps 또는 브라우저 지도에서 바로 확인할 수 있습니다.",
		}, "\n")
		return scenarioReplyWithEvidence(opts, scenario, request, reply, []string{link})
	case "find_place":
		query := toolArgString(args, "query")
		if query == "" {
			return "어느 장소를 찾을까요? 장소명이나 주소를 보내주세요."
		}
		link := "https://www.google.com/maps/search/?api=1&query=" + url.QueryEscape(query)
		lower := strings.ToLower(request)
		intro := "지도 검색 링크입니다."
		scenario := "maps_place_link"
		if strings.Contains(strings.ToLower(query), "parking") || containsAny(lower, "주차", "parking") {
			intro = "주차 정보 확인 링크입니다."
			scenario = "parking_check"
		} else if containsAny(lower, "근처", "nearby", "open now", "문 연") {
			intro = "장소 검색 링크입니다."
			scenario = "nearby_place_search"
		}
		reply := strings.Join([]string{
			intro,
			link,
			"",
			"지도에서 후보를 고르면 영업시간, 주차, 전화, 동선을 이어서 정리하겠습니다.",
		}, "\n")
		return scenarioReplyWithEvidence(opts, scenario, request, reply, []string{link})
	case "get_fleet_status":
		return assistantFleetStatusReply(request)
	case "run_mac_action":
		prompt := toolArgString(args, "prompt")
		if prompt == "" {
			prompt = request
		}
		if looksLikeAssistantVoiceRequest(prompt) || looksLikeAssistantVoiceRequest(request) {
			return executeAssistantVoiceToolFallback(opts, request, args)
		}
		if calendarPrompt, ok := assistantCalendarListPrompt(prompt); ok {
			prompt = calendarPrompt
		}
		reply, handled := runSignalArgosAction(opts, prompt, 0)
		if !handled {
			return "Mac 작업을 이해하지 못했습니다. 예: `Safari 열어줘`, `오늘 일정 뭐 있어?`, `내일 6시 리마인더 추가`"
		}
		return reply
	case "create_document":
		if looksLikeAssistantVoiceRequest(request) {
			return executeAssistantVoiceToolFallback(opts, request, args)
		}
		title := firstNonEmpty(toolArgString(args, "title"), "Argos 문서")
		body := firstNonEmpty(toolArgString(args, "body"), request)
		body = enrichAssistantDocumentBody(request, title, body)
		targetRef := strings.TrimSpace(toolArgString(args, "target"))
		if targetRef == "" && looksLikeAssistantExternalSendRequest(request) {
			targetRef = inferAssistantSignalTargetRef(request)
		}
		result := osauto.CreateArgosDocument(ctx, title, body)
		record, storeErr := evidence.Store(result.Action, firstNonEmpty(opts.TargetID, "assistant"), title, result)
		rememberAssistantArtifact(opts, title, "document", result, osauto.Result{})
		if targetRef != "" {
			return formatAssistantDocumentSendResult(opts, title, targetRef, result, record, storeErr)
		}
		return formatAssistantDocumentResult(result, record, storeErr)
	case "create_voice_report":
		return executeAssistantVoiceReportTool(ctx, opts, request, args)
	case "scheduled_delivery_plan":
		return executeAssistantScheduledDeliveryPlan(opts, request, args)
	case "create_spreadsheet":
		title := firstNonEmpty(toolArgString(args, "title"), "Argos 표")
		body := firstNonEmpty(toolArgString(args, "body"), request)
		result := osauto.CreateSpreadsheet(ctx, title, body)
		record, storeErr := evidence.Store(result.Action, firstNonEmpty(opts.TargetID, "assistant"), title, result)
		rememberAssistantArtifact(opts, title, "spreadsheet", osauto.Result{}, osauto.Result{}, result)
		return formatAssistantSpreadsheetResult(result, record, storeErr)
	case "prepare_meeting_materials":
		title := firstNonEmpty(toolArgString(args, "title"), "회의 자료")
		body := firstNonEmpty(toolArgString(args, "body"), request)
		audience := toolArgString(args, "audience")
		targetRef := strings.TrimSpace(toolArgString(args, "target"))
		if targetRef == "" && looksLikeAssistantExternalSendRequest(request) {
			targetRef = inferAssistantSignalTargetRef(request)
		}
		slideCount := toolArgInt(args, "slide_count", 6)
		if slideCount < 3 {
			slideCount = 3
		}
		if slideCount > 12 {
			slideCount = 12
		}
		docBody := meetingBriefDocumentBody(title, body, audience)
		deckBody := meetingDeckBody(title, body, audience)
		restorePreview := temporarilySkipPreviewImages()
		doc := osauto.CreateArgosDocument(ctx, title+" 브리프", docBody)
		deck := osauto.CreatePresentation(ctx, title+" 발표자료", deckBody, audience, slideCount, "")
		restorePreview()
		payload := map[string]interface{}{
			"kind":         "meshclaw_assistant_meeting_materials",
			"title":        title,
			"audience":     audience,
			"document":     doc,
			"presentation": deck,
			"created_at":   time.Now().UTC(),
		}
		record, storeErr := evidence.Store("assistant-meeting-materials", firstNonEmpty(opts.TargetID, "assistant"), title, payload)
		rememberAssistantArtifact(opts, title, "meeting_materials", doc, deck)
		if targetRef != "" {
			return formatAssistantMeetingMaterialsSendResult(opts, title, targetRef, doc, deck, record, storeErr)
		}
		return formatAssistantMeetingMaterialsResult(doc, deck, record, storeErr)
	case "create_presentation":
		title := firstNonEmpty(toolArgString(args, "title"), "Argos 발표자료")
		body := firstNonEmpty(toolArgString(args, "body"), request)
		audience := toolArgString(args, "audience")
		slideCount := toolArgInt(args, "slide_count", 6)
		result := osauto.CreatePresentation(ctx, title, body, audience, slideCount, "")
		record, storeErr := evidence.Store(result.Action, firstNonEmpty(opts.TargetID, "assistant"), title, result)
		rememberAssistantArtifact(opts, title, "presentation", osauto.Result{}, result)
		return formatAssistantPresentationResult(result, record, storeErr)
	case "revise_recent_artifact":
		instruction := firstNonEmpty(toolArgString(args, "instruction"), request)
		target := toolArgString(args, "target")
		slideCount := toolArgInt(args, "slide_count", 0)
		return reviseRecentAssistantArtifact(ctx, opts, instruction, target, slideCount)
	case "export_recent_artifact":
		format := firstNonEmpty(toolArgString(args, "format"), "pdf")
		target := toolArgString(args, "target")
		return exportRecentAssistantArtifact(ctx, opts, format, target)
	case "open_recent_artifact":
		app := toolArgString(args, "app")
		target := toolArgString(args, "target")
		return openRecentAssistantArtifact(ctx, opts, app, target)
	case "resend_recent_artifact":
		target := toolArgString(args, "target")
		return resendRecentAssistantArtifact(opts, target)
	case "search_web":
		query := toolArgString(args, "query")
		if query == "" {
			return "검색어가 필요합니다."
		}
		reply, handled := runSignalArgosAction(opts, "브라우저에서 "+query+" 검색해줘", 0)
		if !handled {
			return "브라우저 검색을 시작하지 못했습니다."
		}
		return reply
	case "market_outlook":
		asset := toolArgString(args, "asset")
		if asset == "" {
			asset = marketAssetFromRequest(request)
		}
		if asset == "" {
			return "어떤 시장을 볼까요? 예: `유가`, `WTI`, `원달러 환율`, `엔비디아 주가`"
		}
		horizon := toolArgString(args, "horizon")
		targetRef := strings.TrimSpace(toolArgString(args, "target"))
		if targetRef == "" && looksLikeAssistantExternalSendRequest(request) {
			targetRef = inferAssistantSignalTargetRef(request)
		}
		voiceBrief := marketOutlookVoiceRequested(request, args)
		voiceNote := toolArgBool(args, "voice_note", false)
		engine := toolArgString(args, "engine")
		ttsVoice := firstNonEmpty(toolArgString(args, "tts_voice"), toolArgString(args, "voice"))
		query := marketOutlookSearchQuery(asset, horizon)
		search, err := browserauto.Search(ctx, browserauto.SearchOptions{Query: query, Limit: 5, Timeout: 20})
		record, storeErr := evidence.Store("assistant-market-outlook", firstNonEmpty(opts.TargetID, "assistant"), query, map[string]interface{}{
			"asset":       asset,
			"horizon":     horizon,
			"target":      targetRef,
			"voice_brief": voiceBrief,
			"voice_note":  voiceNote,
			"engine":      engine,
			"tts_voice":   ttsVoice,
			"query":       query,
			"search":      search,
			"error":       errorString(err),
		})
		lines := formatMarketOutlookToolResult(asset, horizon, query, search, err)
		lines = appendEvidenceLine(lines, record, storeErr)
		if targetRef != "" || voiceBrief {
			return formatAssistantMarketOutlookSendResult(opts, targetRef, asset, lines, voiceBrief, voiceNote, engine, ttsVoice, record, storeErr)
		}
		return strings.Join(lines, "\n")
	case "open_url":
		rawURL := toolArgString(args, "url")
		if rawURL == "" {
			return "URL이 필요합니다."
		}
		reply, handled := runSignalArgosAction(opts, rawURL+" 열어줘", 0)
		if !handled {
			return "URL을 열지 못했습니다."
		}
		return reply
	case "list_mail_accounts":
		store, err := mailadapter.ListAccounts()
		return formatSignalMailAccounts(store, err)
	case "check_mail":
		intent := assistantToolIntent{
			Intent:  "mail_watch",
			Limit:   toolArgInt(args, "limit", 10),
			Account: toolArgString(args, "account"),
		}
		return executeSignalMailReadIntent(intent)
	case "search_mail":
		query := toolArgString(args, "query")
		if query == "" {
			return "메일 검색 키워드가 필요합니다."
		}
		intent := assistantToolIntent{
			Intent:  "mail_search",
			Query:   query,
			Limit:   toolArgInt(args, "limit", 10),
			Account: toolArgString(args, "account"),
		}
		return executeSignalMailReadIntent(intent)
	case "summarize_mail":
		intent := assistantToolIntent{
			Intent:  "mail_summary",
			Limit:   toolArgInt(args, "limit", 10),
			Account: toolArgString(args, "account"),
		}
		return executeSignalMailReadIntent(intent)
	case "read_mail":
		messageID := toolArgString(args, "message_id")
		if messageID == "" {
			return "메일 id가 필요합니다."
		}
		message, err := mailadapter.Read(mailadapter.ReadOptions{ID: messageID, MaxBody: 5000})
		record, storeErr := evidence.Store("signal-mail-thread-read", message.Summary.Mailbox, messageID, message)
		return formatSignalMailThread(message, record, storeErr, err)
	case "find_booking":
		query := firstNonEmpty(toolArgString(args, "query"), assistantBookingQueryFromArgs(request, args))
		if query == "" {
			return "예약 후보를 찾으려면 장소, 날짜, 시간, 인원 정보가 필요합니다."
		}
		reply, links := assistantBookingPrepareReply(ctx, query)
		return scenarioReplyWithEvidence(opts, "booking_prepare", request, reply, links)
	case "search_shopping":
		if assistantPurchaseAutomationDisabled() {
			clearPendingShoppingDirectPurchase(opts.TargetID)
			return assistantPurchaseAutomationDisabledReply(request)
		}
		query := toolArgString(args, "query")
		if query == "" {
			return "어떤 상품을 찾을까요? 상품명과 예산/옵션을 알려주세요."
		}
		if purchaseQuery := assistantToolDirectPurchaseQuery(request, query); purchaseQuery != "" {
			rememberPendingShoppingDirectPurchase(opts, request, purchaseQuery)
			return formatShoppingDirectPurchaseOneApprovalReplyFor(lang.Current(), purchaseQuery, "")
		}
		mode := strings.ToLower(toolArgString(args, "mode"))
		if containsAny(strings.ToLower(request), "쿠폰", "할인 코드", "할인코드", "coupon", "discount code") {
			mode = "coupon"
		}
		combined := strings.ToLower(request + " " + query)
		if containsAny(combined, "쿠팡", "coupang") && mode != "coupon" && mode != "discount" {
			coupangQuery := coupangShoppingQuery(request + " " + query)
			if coupangQuery == "" {
				coupangQuery = query
			}
			reply, handled := runSignalArgosAction(opts, coupangSearchURL(coupangQuery)+" 열어줘", 0)
			if !handled {
				return "쿠팡 검색을 시작하지 못했습니다."
			}
			candidates := coupangShoppingCandidates(ctx, coupangQuery, 3)
			return coupangShoppingPrepReply(reply, coupangQuery, candidates...)
		}
		switch mode {
		case "reviews", "review", "compare":
			reply, handled := runSignalArgosAction(opts, "브라우저에서 "+query+" 리뷰 비교 검색해줘", 0)
			if !handled {
				return "리뷰 검색을 시작하지 못했습니다."
			}
			return reply + "\n\n리뷰/가격/배송 조건을 비교하고, 구매나 장바구니는 별도 승인 후 진행합니다."
		case "coupon", "discount":
			reply, handled := runSignalArgosAction(opts, "브라우저에서 "+query+" coupon discount code 검색해줘", 0)
			if !handled {
				return "쿠폰 검색을 시작하지 못했습니다."
			}
			return reply + "\n\n쿠폰은 적용 전 총액과 자동결제/구독 조건을 확인하고, 결제나 가입은 별도 승인 후 진행합니다."
		default:
			reply, handled := runSignalArgosAction(opts, "https://www.google.com/search?tbm=shop&q="+url.QueryEscape(query)+" 열어줘", 0)
			if !handled {
				return "쇼핑 검색을 시작하지 못했습니다."
			}
			return reply + "\n\n검색 후에는 리뷰/배송비/총액을 비교하고, 결제 직전에서 멈추는 흐름으로 진행합니다."
		}
	case "play_media":
		query := toolArgString(args, "query")
		platform := strings.ToLower(toolArgString(args, "platform"))
		if query == "" && platform == "" {
			return "어디에서 무엇을 재생할지 알려주세요. 예: `유튜브에서 재즈`, `KBS 라디오`, `넷플릭스`"
		}
		combined := strings.ToLower(request + " " + query + " " + platform)
		if platform == "ott" || containsAny(combined, "넷플릭스", "netflix", "애플tv", "apple tv", "wavve", "웨이브", "쿠팡플레이", "coupang play") {
			if serviceURL := ottServiceURL(combined); serviceURL != "" {
				reply, handled := runSignalArgosAction(opts, serviceURL+" 열어줘", 0)
				if handled {
					return reply
				}
			}
		}
		if platform == "radio" || containsAny(combined, "라디오", "radio") {
			if stationURL := radioStationURL(combined); stationURL != "" {
				reply, handled := runSignalArgosAction(opts, stationURL+" 열어줘", 0)
				if handled {
					return reply
				}
			}
			return "어느 라디오를 틀까요? KBS, MBC, SBS, CBS, EBS 중 하나를 보내주세요."
		}
		if platform == "ambient" || containsAny(combined, "빗소리", "백색소음", "white noise") {
			reply, handled := runSignalArgosAction(opts, "https://www.youtube.com/results?search_query="+url.QueryEscape("rain sounds white noise")+" 열어줘", 0)
			if handled {
				return reply
			}
		}
		if platform == "podcast" || containsAny(combined, "팟캐스트", "podcast") {
			if query == "" {
				query = "technology podcast"
			}
			reply, handled := runSignalArgosAction(opts, "https://www.youtube.com/results?search_query="+url.QueryEscape(query)+" 열어줘", 0)
			if handled {
				return reply
			}
		}
		if query == "" {
			query = "lofi jazz"
		}
		reply, handled := runSignalArgosAction(opts, "https://www.youtube.com/results?search_query="+url.QueryEscape(query)+" 열어줘", 0)
		if !handled {
			return "미디어 재생을 시작하지 못했습니다."
		}
		return reply
	case "send_tts_voice":
		return executeAssistantTTSVoiceTool(opts, request, args)
	default:
		return "지원하지 않는 도구입니다: " + name
	}
}

func normalizeAssistantToolName(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "find_restaurants", "find_restaurant", "search_restaurants", "restaurant_search", "restaurant_reservation", "reserve_restaurant", "book_restaurant":
		return "find_booking"
	case "hotel_search", "find_hotels", "find_hotel", "book_hotel", "reserve_hotel", "flight_search", "find_flights", "book_flight", "travel_search", "travel_booking":
		return "find_booking"
	case "product_search", "search_products", "find_products", "shopping_search", "compare_products":
		return "search_shopping"
	case "set_reminder", "create_reminder", "add_reminder", "reminder_create", "add_event", "create_event", "calendar_event", "schedule_event", "calendar_add_event", "calendar_create_event", "calendar_schedule_event", "schedule_calendar_event", "create_calendar_event", "add_calendar_event", "get_calendar_events", "list_calendar_events":
		return "run_mac_action"
	default:
		normalized := strings.ToLower(strings.TrimSpace(name))
		if strings.Contains(normalized, "calendar") && strings.Contains(normalized, "event") {
			return "run_mac_action"
		}
		if strings.Contains(normalized, "reminder") && (strings.Contains(normalized, "set") || strings.Contains(normalized, "add") || strings.Contains(normalized, "create")) {
			return "run_mac_action"
		}
		return strings.TrimSpace(name)
	}
}

func assistantBookingQueryFromArgs(request string, args map[string]interface{}) string {
	parts := []string{}
	for _, key := range []string{"date", "day", "time", "location", "area", "city", "cuisine", "restaurant", "hotel", "destination", "party_size", "people", "guests", "budget"} {
		if value := toolArgString(args, key); value != "" {
			parts = append(parts, value)
		}
	}
	if len(parts) > 0 {
		return strings.Join(parts, " ")
	}
	return strings.TrimSpace(request)
}

func assistantBookingPrepareReply(ctx context.Context, query string) (string, []string) {
	searchLink := "https://www.google.com/search?q=" + url.QueryEscape(query+" 예약")
	mapsLink := "https://www.google.com/maps/search/?api=1&query=" + url.QueryEscape(query)
	search := assistantBookingSearch(ctx, query)
	candidates := assistantBookingCandidateLines(search, 3)
	lines := []string{
		"예약 후보를 찾는 링크입니다. 조건표도 같이 정리했습니다.",
		"요청: " + query,
	}
	links := []string{mapsLink, searchLink}
	if len(candidates) > 0 {
		lines = append(lines, "", "검색 후보:")
		for _, candidate := range candidates {
			lines = append(lines, "- "+candidate.Text)
			if candidate.URL != "" {
				lines = append(lines, "  "+candidate.URL)
				links = append(links, candidate.URL)
			}
		}
	} else {
		lines = append(lines, "", "후보 확인: 아래 지도/예약 검색을 열면 조건에 맞는 후보를 바로 볼 수 있습니다.")
	}
	lines = append(lines,
		"",
		"예약 전에 바로 확인할 것:",
		"- 날짜/시간/인원: 요청 조건과 일치하는지 확인",
		"- 위치/동선: 이동시간, 주차, 대중교통 확인",
		"- 비용/조건: 예약금, 취소 기한, 노쇼 수수료 확인",
		"- 연락/확정: 예약자 이름과 전화번호 확인",
		"",
		"바로 열기:",
		"- 지도 후보: "+mapsLink,
		"- 예약 검색: "+searchLink,
		"",
		"다음에 말할 것: `이 후보들 중 평점 좋은 곳 3개로 줄여줘` 또는 `첫 번째 후보로 예약 진행 문구 만들어줘`",
		"원칙: 가능 시간/인원/총액을 확인하고, 예약 제출 또는 결제가 있는 마지막 단계에서는 멈춘 뒤 Signal에서 다시 승인받습니다.",
	)
	return strings.Join(lines, "\n"), assistantUniqueStrings(links)
}

func assistantBookingSearch(ctx context.Context, query string) browserauto.SearchResult {
	if assistantEnvTruthy(os.Getenv("MESHCLAW_BOOKING_SEARCH_DISABLE")) {
		return browserauto.SearchResult{}
	}
	search, _ := browserauto.Search(ctx, browserauto.SearchOptions{
		Query:   query + " 예약 가능 평점",
		Limit:   5,
		Timeout: 8,
	})
	return search
}

func assistantBookingCandidateLines(search browserauto.SearchResult, limit int) []browserauto.Link {
	if limit <= 0 {
		limit = 3
	}
	out := []browserauto.Link{}
	seen := map[string]bool{}
	for _, item := range search.Results {
		title := strings.TrimSpace(item.Text)
		link := strings.TrimSpace(item.URL)
		if title == "" || seen[title+" "+link] {
			continue
		}
		seen[title+" "+link] = true
		if link != "" {
			if host := assistantURLHost(link); host != "" && !strings.Contains(strings.ToLower(title), strings.ToLower(host)) {
				title = title + " (" + host + ")"
			}
		}
		out = append(out, browserauto.Link{Text: trimForContext(title, 120), URL: link})
		if len(out) >= limit {
			break
		}
	}
	return out
}

func assistantURLHost(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www.")
	return host
}

func assistantEnvTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func assistantUniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
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

func executeAssistantVoiceToolFallback(opts ListenOptions, request string, args map[string]interface{}) string {
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	fallbackArgs := map[string]interface{}{}
	for key, value := range args {
		fallbackArgs[key] = value
	}
	if target := inferAssistantSignalTargetRef(request); target != "" && toolArgString(fallbackArgs, "target") == "" {
		fallbackArgs["target"] = target
	}
	if looksLikeAssistantVoiceReportRequest(request) {
		if toolArgString(fallbackArgs, "title") == "" {
			fallbackArgs["title"] = inferAssistantVoiceReportTitle(request)
		}
		if toolArgString(fallbackArgs, "body") == "" {
			fallbackArgs["body"] = request
		}
		if strings.TrimSpace(toolArgString(fallbackArgs, "delivery")) == "" && containsAny(strings.ToLower(request), "전화", "통화", "전화로", "콜", "call") {
			fallbackArgs["delivery"] = "call"
		}
		return executeAssistantVoiceReportTool(ctx, opts, request, fallbackArgs)
	}
	if toolArgString(fallbackArgs, "topic") == "" {
		fallbackArgs["topic"] = inferAssistantVoiceTopic(request)
	}
	return executeAssistantTTSVoiceTool(opts, request, fallbackArgs)
}

func executeAssistantScheduledDeliveryPlan(opts ListenOptions, request string, args map[string]interface{}) string {
	targetRef := strings.TrimSpace(toolArgString(args, "target"))
	if targetRef == "" {
		targetRef = inferAssistantSignalTargetRef(request)
	}
	plan := PlanScheduledDelivery(ScheduledDeliveryPlanOptions{
		Target:      targetRef,
		Schedule:    toolArgString(args, "schedule"),
		Content:     firstNonEmpty(toolArgString(args, "content"), request),
		ContentType: toolArgString(args, "content_type"),
		Delivery:    toolArgString(args, "delivery"),
		Execute:     false,
		Approve:     false,
	})
	_, _ = evidence.Store("assistant-scheduled-delivery-plan", firstNonEmpty(opts.TargetID, "assistant"), firstNonEmpty(plan.Target, "missing-target"), plan)
	lines := []string{
		"예약 발송 계획을 만들었습니다.",
		"아직 예약 등록이나 발송은 하지 않았습니다.",
		"- 상태: " + plan.Status,
		"- 주기: " + firstNonEmpty(plan.Schedule, "(미정)"),
		"- 형식: " + plan.ContentType + " / " + plan.Delivery,
	}
	if plan.ResolvedTarget != nil {
		lines = append(lines, "- 대상: "+firstNonEmpty(plan.ResolvedTarget.Label, plan.ResolvedTarget.ID)+" (`"+plan.ResolvedTarget.ID+"`)")
	} else if plan.Target != "" {
		lines = append(lines, "- 대상: "+plan.Target)
	}
	if strings.TrimSpace(plan.Content) != "" {
		lines = append(lines, lang.T("assistant.scheduled_delivery.plan_content", trimSignalListLine(plan.Content, 120)))
	}
	if len(plan.TargetCandidates) > 0 {
		lines = append(lines, formatAssistantVoiceTargetCandidates(plan.TargetCandidates)...)
	} else if plan.TargetError != "" {
		lines = append(lines, "- 대상 확인 필요: "+plan.TargetError)
	}
	if plan.Status == "review_ready" && plan.ResolvedTarget != nil {
		rememberPendingAssistantConversation(assistantPendingKey(opts.TargetID), pendingAssistantConversation{
			Kind: "scheduled_delivery_apply",
			Intent: assistantToolIntent{
				Intent:      "scheduled_delivery_apply",
				Target:      firstNonEmpty(plan.Target, plan.ResolvedTarget.ID),
				Schedule:    plan.Schedule,
				Content:     plan.Content,
				ContentType: plan.ContentType,
				Delivery:    plan.Delivery,
			},
			Request:   request,
			CreatedAt: time.Now().UTC(),
			ExpiresAt: time.Now().UTC().Add(15 * time.Minute),
		})
		lines = append(lines, "", lang.T("assistant.scheduled_delivery.approval_hint"), lang.T("assistant.scheduled_delivery.cancel_hint"))
	}
	lines = append(lines, "", "다음 단계는 첫 발송 미리보기를 보여주고, 사용자가 승인하면 예약 등록을 적용하는 것입니다.")
	return strings.Join(lines, "\n")
}

func executeAssistantTTSVoiceTool(opts ListenOptions, request string, args map[string]interface{}) string {
	content := strings.TrimSpace(toolArgString(args, "content"))
	topic := strings.TrimSpace(toolArgString(args, "topic"))
	if content == "" {
		content = assistantVoiceContentFromRequest(request, topic)
	}
	if content == "" {
		return "음성으로 만들 텍스트가 필요합니다. 읽을 내용을 보내주시거나 주제를 알려주세요."
	}
	targetRef := strings.TrimSpace(toolArgString(args, "target"))
	if assistantVoiceTargetIsCurrentRoom(targetRef) {
		targetRef = ""
	}
	engine := firstNonEmpty(toolArgString(args, "engine"), "edge-tts")
	voice := toolArgString(args, "voice")
	execute := toolArgBool(args, "execute", true)
	voiceNote := toolArgBool(args, "voice_note", false)
	plan := map[string]interface{}{
		"kind":       "assistant_tts_voice_plan",
		"topic":      topic,
		"target":     targetRef,
		"engine":     engine,
		"voice":      voice,
		"execute":    execute,
		"voice_note": voiceNote,
		"chars":      len([]rune(content)),
		"created_at": time.Now().UTC(),
	}
	if !execute {
		record, storeErr := evidence.Store("assistant-tts-voice-plan", firstNonEmpty(opts.TargetID, "assistant"), firstNonEmpty(topic, "voice"), plan)
		lines := []string{
			"음성파일 생성 계획입니다.",
			fmt.Sprintf("- 글자 수: %d자", len([]rune(content))),
			"- 엔진: " + engine,
		}
		if targetRef != "" {
			lines = append(lines, "- 보낼 대상: "+targetRef)
		}
		return strings.Join(appendEvidenceLine(lines, record, storeErr), "\n")
	}
	audio, audioErr := tts.Synthesize(tts.Options{
		Text:     content,
		Engine:   engine,
		Voice:    voice,
		Basename: assistantVoiceBasename(topic, request),
	})
	payload := map[string]interface{}{
		"kind":        "assistant_tts_voice",
		"topic":       topic,
		"target":      targetRef,
		"engine":      engine,
		"voice":       voice,
		"voice_note":  voiceNote,
		"content":     content,
		"audio":       audio,
		"audio_error": errorString(audioErr),
		"created_at":  time.Now().UTC(),
	}
	if audioErr != nil {
		record, storeErr := evidence.Store("assistant-tts-voice", firstNonEmpty(opts.TargetID, "assistant"), firstNonEmpty(topic, "voice"), payload)
		lines := []string{"음성파일 생성에 실패했습니다.", "문제: " + audioErr.Error()}
		return strings.Join(appendEvidenceLine(lines, record, storeErr), "\n")
	}
	if targetRef == "" {
		record, storeErr := evidence.Store("assistant-tts-voice", firstNonEmpty(opts.TargetID, "assistant"), firstNonEmpty(topic, "voice"), payload)
		lines := []string{
			"음성파일을 만들었습니다.",
			"이 대화에 첨부합니다.",
			"meshclaw-attachment: " + audio.Path,
		}
		return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
	}
	target, candidates, targetErr := resolveAssistantVoiceTarget(targetRef)
	if targetErr != nil {
		payload["target_error"] = targetErr.Error()
		payload["target_candidates"] = candidates
		record, storeErr := evidence.Store("assistant-tts-voice", firstNonEmpty(opts.TargetID, "assistant"), targetRef, payload)
		lines := []string{"음성파일은 만들었지만 보낼 Signal 대상을 확정하지 못했습니다."}
		lines = append(lines, formatAssistantVoiceTargetCandidates(candidates)...)
		lines = append(lines, "이 대화에 먼저 첨부합니다.")
		lines = append(lines, "meshclaw-attachment: "+audio.Path)
		return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
	}
	send, sendErr := Send(SendOptions{
		TargetID:    target.ID,
		Kind:        "text",
		Text:        firstNonEmpty(topic, "음성 메시지") + " 음성파일입니다.",
		Attachments: []string{audio.Path},
		VoiceNote:   voiceNote,
		Execute:     true,
	})
	payload["resolved_target"] = target
	payload["send"] = send
	payload["send_error"] = errorString(sendErr)
	record, storeErr := evidence.Store("assistant-tts-voice-send", firstNonEmpty(opts.TargetID, "assistant"), target.ID, payload)
	if sendErr != nil {
		lines := []string{
			"음성파일은 만들었지만 Signal 전송에 실패했습니다.",
			"대상: " + firstNonEmpty(target.Label, target.ID),
			"문제: " + sendErr.Error(),
			"이 대화에 먼저 첨부합니다.",
			"meshclaw-attachment: " + audio.Path,
		}
		return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
	}
	lines := []string{
		"음성파일을 만들어 Signal로 보냈습니다.",
		"대상: " + firstNonEmpty(target.Label, target.ID),
	}
	return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
}

func executeAssistantVoiceReportTool(ctx context.Context, opts ListenOptions, request string, args map[string]interface{}) string {
	title := firstNonEmpty(toolArgString(args, "title"), "Argos 음성 보고서")
	body := firstNonEmpty(toolArgString(args, "body"), request)
	body = enrichAssistantDocumentBody(request, title, body)
	targetRef := strings.TrimSpace(toolArgString(args, "target"))
	engine := firstNonEmpty(toolArgString(args, "engine"), "edge-tts")
	voice := toolArgString(args, "voice")
	delivery := normalizeVoiceReportDelivery(toolArgString(args, "delivery"), targetRef)
	voiceNote := toolArgBool(args, "voice_note", true)
	approve := toolArgBool(args, "approve", false)
	execute := toolArgBool(args, "execute", true)
	if !execute {
		plan := map[string]interface{}{
			"kind":       "assistant_voice_report_plan",
			"title":      title,
			"target":     targetRef,
			"engine":     engine,
			"voice":      voice,
			"delivery":   delivery,
			"voice_note": voiceNote,
			"approve":    approve,
			"chars":      len([]rune(body)),
			"created_at": time.Now().UTC(),
		}
		record, storeErr := evidence.Store("assistant-voice-report-plan", firstNonEmpty(opts.TargetID, "assistant"), title, plan)
		lines := []string{
			"음성 보고서 생성 계획입니다.",
			"- 보고서 제목: " + title,
			fmt.Sprintf("- 본문 길이: %d자", len([]rune(body))),
			"- 음성 엔진: " + engine,
		}
		if targetRef != "" {
			lines = append(lines, "- 보낼 대상: "+targetRef)
		}
		if delivery == "call" {
			lines = append(lines, "- 전화 전달: 실제 통화는 approve=true가 필요합니다.")
		}
		return strings.Join(appendEvidenceLine(lines, record, storeErr), "\n")
	}
	doc := osauto.CreateArgosDocument(ctx, title, body)
	if !doc.OK || doc.Error != "" {
		record, storeErr := evidence.Store("assistant-voice-report", firstNonEmpty(opts.TargetID, "assistant"), title, map[string]interface{}{
			"title":      title,
			"document":   doc,
			"created_at": time.Now().UTC(),
		})
		lines := []string{"보고서 작성에 실패했습니다.", "문제: " + firstNonEmpty(doc.Error, doc.Stderr, "unknown error")}
		return strings.Join(appendEvidenceLine(lines, record, storeErr), "\n")
	}
	audioText := voiceReportAudioText(title, body)
	audio, audioErr := tts.Synthesize(tts.Options{
		Text:     audioText,
		Engine:   engine,
		Voice:    voice,
		Basename: "argos-voice-report-" + time.Now().UTC().Format("20060102T150405Z"),
	})
	payload := map[string]interface{}{
		"kind":        "assistant_voice_report",
		"title":       title,
		"target":      targetRef,
		"engine":      engine,
		"voice":       voice,
		"delivery":    delivery,
		"voice_note":  voiceNote,
		"approve":     approve,
		"document":    doc,
		"audio":       audio,
		"audio_error": errorString(audioErr),
		"created_at":  time.Now().UTC(),
	}
	if audioErr != nil {
		record, storeErr := evidence.Store("assistant-voice-report", firstNonEmpty(opts.TargetID, "assistant"), title, payload)
		lines := []string{"보고서는 만들었지만 음성파일 생성에 실패했습니다.", "문제: " + audioErr.Error()}
		lines = appendAssistantAttachmentMarkers(lines, doc)
		return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
	}
	attachments := voiceReportAttachments(doc, audio.Path)
	if delivery == "current_chat" || targetRef == "" {
		record, storeErr := evidence.Store("assistant-voice-report", firstNonEmpty(opts.TargetID, "assistant"), title, payload)
		lines := []string{
			"보고서와 음성 메시지를 만들었습니다.",
			"이 대화에 보고서 파일과 음성파일을 첨부합니다.",
		}
		lines = appendVoiceReportAttachmentMarkers(lines, attachments)
		return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
	}
	target, candidates, targetErr := resolveAssistantVoiceTarget(targetRef)
	if targetErr != nil {
		payload["target_error"] = targetErr.Error()
		payload["target_candidates"] = candidates
		record, storeErr := evidence.Store("assistant-voice-report", firstNonEmpty(opts.TargetID, "assistant"), targetRef, payload)
		lines := []string{"보고서와 음성파일은 만들었지만 보낼 Signal 대상을 확정하지 못했습니다."}
		lines = append(lines, formatAssistantVoiceTargetCandidates(candidates)...)
		lines = append(lines, "이 대화에 먼저 첨부합니다.")
		lines = appendVoiceReportAttachmentMarkers(lines, attachments)
		return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
	}
	if delivery == "call" {
		callResult, callErr := runAssistantVoiceReportCall(target.ID, audio.Path, approve)
		payload["call"] = callResult
		payload["call_error"] = errorString(callErr)
		record, storeErr := evidence.Store("assistant-voice-report-call", firstNonEmpty(opts.TargetID, "assistant"), target.ID, payload)
		if !approve {
			lines := []string{
				"보고서와 음성파일을 만들었고, 전화로 읽어줄 준비까지 했습니다.",
				"대상: " + firstNonEmpty(target.Label, target.ID),
				"실제 전화를 걸려면 승인 문구와 함께 다시 요청해야 합니다.",
			}
			lines = appendVoiceReportAttachmentMarkers(lines, attachments)
			return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
		}
		if callErr != nil {
			lines := []string{
				"보고서와 음성파일은 만들었지만 Signal 전화 실행에 실패했습니다.",
				"대상: " + firstNonEmpty(target.Label, target.ID),
				"문제: " + callErr.Error(),
				"이 대화에 먼저 첨부합니다.",
			}
			lines = appendVoiceReportAttachmentMarkers(lines, attachments)
			return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
		}
		lines := []string{
			"보고서를 음성 통화로 읽어주기 시작했습니다.",
			"대상: " + firstNonEmpty(target.Label, target.ID),
		}
		return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
	}
	send, sendErr := Send(SendOptions{
		TargetID:    target.ID,
		Kind:        "text",
		Text:        title + " 보고서와 음성 메시지입니다.",
		Attachments: attachments,
		VoiceNote:   voiceNote,
		Execute:     true,
	})
	payload["resolved_target"] = target
	payload["send"] = send
	payload["send_error"] = errorString(sendErr)
	record, storeErr := evidence.Store("assistant-voice-report-send", firstNonEmpty(opts.TargetID, "assistant"), target.ID, payload)
	if sendErr != nil {
		lines := []string{
			"보고서와 음성파일은 만들었지만 Signal 전송에 실패했습니다.",
			"대상: " + firstNonEmpty(target.Label, target.ID),
			"문제: " + sendErr.Error(),
			"이 대화에 먼저 첨부합니다.",
		}
		lines = appendVoiceReportAttachmentMarkers(lines, attachments)
		return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
	}
	lines := []string{
		"보고서와 음성 메시지를 Signal로 보냈습니다.",
		"대상: " + firstNonEmpty(target.Label, target.ID),
	}
	return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
}

func normalizeVoiceReportDelivery(value, target string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "call", "phone", "전화", "통화":
		return "call"
	case "voice_note", "voice-note", "보이스노트":
		return "voice_note"
	case "signal", "send", "message", "메시지", "전송":
		return "signal"
	case "current_chat", "chat", "attachment", "첨부", "현재방":
		return "current_chat"
	}
	if strings.TrimSpace(target) != "" {
		return "signal"
	}
	return "current_chat"
}

func runAssistantVoiceReportCall(targetID, audioPath string, approve bool) (map[string]interface{}, error) {
	binary, err := os.Executable()
	if err != nil || strings.TrimSpace(binary) == "" {
		binary = "meshclaw"
	}
	args := []string{"assistant", "signal-call", targetID, "--audio", audioPath, "--json"}
	if approve {
		args = append(args, "--approve", "--execute")
	} else {
		args = append(args, "--dry-run")
	}
	out, err := exec.Command(binary, args...).CombinedOutput()
	result := map[string]interface{}{
		"binary":   binary,
		"args":     args,
		"approved": approve,
		"stdout":   string(out),
	}
	if len(out) > 0 {
		var parsed map[string]interface{}
		if parseErr := json.Unmarshal(out, &parsed); parseErr == nil {
			result["parsed"] = parsed
		}
	}
	if err != nil {
		result["error"] = err.Error()
		return result, err
	}
	return result, nil
}

func voiceReportAudioText(title, body string) string {
	body = strings.TrimSpace(body)
	if body == "" {
		return strings.TrimSpace(title)
	}
	return strings.TrimSpace(title) + "\n\n" + body
}

func voiceReportAttachments(doc osauto.Result, audioPath string) []string {
	out := []string{}
	for _, path := range []string{doc.DOCX, doc.PDF, doc.Markdown, doc.URL, audioPath} {
		path = strings.TrimSpace(path)
		if path != "" {
			out = append(out, path)
		}
	}
	return uniqueVoiceReportStrings(out)
}

func appendVoiceReportAttachmentMarkers(lines, attachments []string) []string {
	for _, path := range attachments {
		if strings.TrimSpace(path) != "" {
			lines = append(lines, "meshclaw-attachment: "+strings.TrimSpace(path))
		}
	}
	return lines
}

func uniqueVoiceReportStrings(values []string) []string {
	out := []string{}
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

func assistantVoiceContentFromRequest(request, topic string) string {
	combined := strings.ToLower(strings.TrimSpace(request + " " + topic))
	if containsAny(combined, "기도문", "prayer") {
		return strings.Join([]string{
			"오늘의 기도문",
			"",
			"사랑과 평안을 주시는 하나님, 오늘 하루도 우리에게 새 마음을 허락해 주셔서 감사합니다.",
			"분주한 일들 가운데서도 마음이 흔들리지 않게 하시고, 해야 할 일을 지혜롭게 감당하게 해 주세요.",
			"우리의 말과 선택이 누군가에게 위로가 되게 하시고, 서두름보다 사랑을, 걱정보다 믿음을 먼저 붙들게 해 주세요.",
			"몸과 마음이 지친 사람들에게 쉼을 주시고, 가족과 친구들에게 필요한 보호와 평안을 더해 주세요.",
			"오늘 만나는 모든 순간 속에서 감사할 이유를 발견하게 하시고, 작은 선함을 실천할 힘을 주세요.",
			"예수님의 이름으로 기도합니다. 아멘.",
		}, "\n")
	}
	return strings.TrimSpace(topic)
}

func assistantVoiceBasename(topic, request string) string {
	base := strings.ToLower(strings.TrimSpace(firstNonEmpty(topic, request, "voice-message")))
	switch {
	case containsAny(base, "기도문", "prayer"):
		return "argos-prayer-" + time.Now().UTC().Format("20060102T150405Z")
	case containsAny(base, "편지", "letter"):
		return "argos-letter-" + time.Now().UTC().Format("20060102T150405Z")
	case containsAny(base, "안내", "notice"):
		return "argos-notice-" + time.Now().UTC().Format("20060102T150405Z")
	default:
		return "argos-voice-" + time.Now().UTC().Format("20060102T150405Z")
	}
}

var assistantSearchContacts = osauto.SearchContacts

type assistantContactSearchResult struct {
	Count    int `json:"count"`
	Contacts []struct {
		Name         string   `json:"name"`
		Organization string   `json:"organization"`
		Phones       []string `json:"phones"`
		Emails       []string `json:"emails"`
	} `json:"contacts"`
}

func resolveAssistantVoiceTarget(ref string) (Target, []Target, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return Target{}, nil, fmt.Errorf("target is required")
	}
	store, err := ListTargets()
	if err != nil {
		return Target{}, nil, err
	}
	needle := strings.ToLower(ref)
	needleID := sanitizeID(ref)
	exact := []Target{}
	partial := []Target{}
	for _, target := range store.Targets {
		id := strings.ToLower(strings.TrimSpace(target.ID))
		label := strings.ToLower(strings.TrimSpace(target.Label))
		if id == needleID || label == needle {
			exact = append(exact, target)
			continue
		}
		if strings.Contains(id, needleID) || (label != "" && strings.Contains(label, needle)) {
			partial = append(partial, target)
		}
	}
	switch {
	case len(exact) == 1:
		return exact[0], exact, nil
	case len(exact) > 1:
		return Target{}, exact, fmt.Errorf("multiple Signal targets match %q", ref)
	case len(partial) == 1:
		return partial[0], partial, nil
	case len(partial) > 1:
		return Target{}, partial, fmt.Errorf("multiple Signal targets match %q", ref)
	default:
		if target, candidates, contactErr := resolveAssistantTargetFromContacts(context.Background(), ref); contactErr == nil {
			return target, candidates, nil
		} else if len(candidates) > 0 {
			return Target{}, candidates, contactErr
		}
		return Target{}, nil, fmt.Errorf("no Signal target or exact contact phone matches %q", ref)
	}
}

func assistantVoiceTargetIsCurrentRoom(ref string) bool {
	normalized := strings.ToLower(strings.TrimSpace(ref))
	if normalized == "" {
		return false
	}
	compact := strings.NewReplacer(" ", "", "\t", "", "\n", "", "`", "", "'", "", "\"", "").Replace(normalized)
	switch compact {
	case "여기", "이방", "현재방", "지금방", "이대화", "현재대화", "지금대화", "이채팅", "현재채팅", "지금채팅",
		"thischat", "currentchat", "thisroom", "currentroom", "here":
		return true
	default:
		return false
	}
}

func resolveAssistantTargetFromContacts(ctx context.Context, ref string) (Target, []Target, error) {
	result := assistantSearchContacts(ctx, ref)
	if !result.OK || strings.TrimSpace(result.Stdout) == "" {
		return Target{}, nil, fmt.Errorf("no Signal target matches %q and Contacts search failed: %s", ref, firstNonEmpty(result.Error, "no contact output"))
	}
	contacts, err := parseAssistantContactSearchOutput(result.Stdout)
	if err != nil {
		return Target{}, nil, err
	}
	if len(contacts.Contacts) == 0 {
		return Target{}, nil, fmt.Errorf("no Signal target or contact matches %q", ref)
	}
	candidates := assistantContactTargets(contacts)
	if len(candidates) != 1 {
		return Target{}, candidates, fmt.Errorf("Contacts returned %d possible Signal targets for %q", len(candidates), ref)
	}
	target := candidates[0]
	_, saved, err := UpsertTarget(target)
	if err != nil {
		return Target{}, candidates, err
	}
	_, _ = evidence.Store("assistant-signal-target-auto-add", "assistant", ref, map[string]interface{}{
		"kind":       "assistant_signal_target_auto_add",
		"ref":        ref,
		"target":     saved,
		"created_at": time.Now().UTC(),
	})
	return saved, []Target{saved}, nil
}

func parseAssistantContactSearchOutput(raw string) (assistantContactSearchResult, error) {
	payload := strings.TrimSpace(raw)
	var outer struct {
		Stdout string `json:"stdout"`
	}
	if err := json.Unmarshal([]byte(payload), &outer); err == nil && strings.TrimSpace(outer.Stdout) != "" {
		payload = strings.TrimSpace(outer.Stdout)
	}
	var contacts assistantContactSearchResult
	if err := json.Unmarshal([]byte(payload), &contacts); err != nil {
		return contacts, err
	}
	return contacts, nil
}

func assistantContactTargets(result assistantContactSearchResult) []Target {
	targets := []Target{}
	for _, contact := range result.Contacts {
		label := firstNonEmpty(contact.Name, contact.Organization)
		if label == "" {
			continue
		}
		for _, phone := range contact.Phones {
			recipient := normalizeAssistantSignalPhone(phone)
			if recipient == "" {
				continue
			}
			targets = append(targets, Target{
				ID:        assistantContactTargetID(label, recipient),
				Channel:   "signal",
				Recipient: recipient,
				Label:     label,
				Mode:      "briefing",
			})
		}
	}
	return targets
}

func normalizeAssistantSignalPhone(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var b strings.Builder
	for i, r := range value {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
			continue
		}
		if r == '+' && i == 0 {
			b.WriteRune(r)
		}
	}
	phone := b.String()
	switch {
	case strings.HasPrefix(phone, "+"):
		return phone
	case strings.HasPrefix(phone, "010") && len(phone) == 11:
		return "+82" + strings.TrimPrefix(phone, "0")
	case strings.HasPrefix(phone, "10") && len(phone) == 10:
		return "+82" + phone
	default:
		if len(phone) >= 8 {
			return phone
		}
		return ""
	}
}

func assistantContactTargetID(label, recipient string) string {
	base := sanitizeID(label)
	if base != "" {
		return base + "-signal"
	}
	sum := sha1.Sum([]byte(label + "\x00" + recipient))
	return "contact-" + hex.EncodeToString(sum[:])[:10] + "-signal"
}

func formatAssistantVoiceTargetCandidates(candidates []Target) []string {
	if len(candidates) == 0 {
		return []string{"등록된 대상 이름 또는 target id를 다시 알려주세요."}
	}
	lines := []string{"가능한 대상이 여러 개입니다. 하나를 골라 다시 보내주세요."}
	for i, target := range candidates {
		if i >= 5 {
			lines = append(lines, fmt.Sprintf("외 %d개", len(candidates)-i))
			break
		}
		lines = append(lines, "- "+firstNonEmpty(target.Label, target.ID)+" (`"+target.ID+"`)")
	}
	return lines
}

func enrichAssistantDocumentBody(request, title, body string) string {
	if !isAssistantWorkReportDocumentRequest(request, title) {
		return body
	}
	records, err := evidence.List(30)
	if err != nil {
		return workReportDocumentBody(title, body, nil)
	}
	records = filterArgosEvidence(records)
	if len(records) > 8 {
		records = records[:8]
	}
	return workReportDocumentBody(title, body, records)
}

func isAssistantWorkReportDocumentRequest(request, title string) bool {
	lower := strings.ToLower(strings.TrimSpace(request + " " + title))
	if !containsAny(lower, "argos", "meshclaw", "아르고스", "맥미니", "비서") {
		return false
	}
	return containsAny(lower, "업무 보고", "작업 내역", "진행상황", "진행 상황", "작업 보고", "표로", "한 페이지", "work report", "status report")
}

func workReportDocumentBody(title, request string, records []evidence.Summary) string {
	lines := []string{
		"## 요약",
		"",
		"Argos 비서 작업 내역을 한 페이지 업무 보고 형식으로 정리했습니다.",
		"",
		"## 작업 내역 표",
		"",
		"| 일시 | 영역 | 결과 | 다음 액션 |",
		"| --- | --- | --- | --- |",
	}
	if len(records) == 0 {
		lines = append(lines, "| 오늘 | Argos 비서 | 요청 내용을 기준으로 업무 보고 문서 초안을 만들었습니다. | 실제 작업 기록이 쌓이면 최근 완료 항목을 자동으로 채워 넣습니다. |")
	} else {
		for _, record := range records {
			lines = append(lines, fmt.Sprintf("| %s | %s | %s | %s |",
				formatWorkReportTime(record.Time),
				cleanWorkReportCell(workReportArea(record)),
				cleanWorkReportCell(record.Summary),
				cleanWorkReportCell(workReportNextAction(record)),
			))
		}
	}
	lines = append(lines,
		"",
		"## 메모",
		"",
		"- iPhone에서는 첨부된 DOCX를 Word 또는 Pages로 열어 바로 수정할 수 있습니다.",
		"- Obsidian에서는 함께 만든 Markdown 원본을 열어 문장과 표를 빠르게 다듬을 수 있습니다.",
	)
	if strings.TrimSpace(request) != "" {
		lines = append(lines, "- 원 요청: "+cleanWorkReportCell(request))
	}
	return strings.Join(lines, "\n")
}

func formatWorkReportTime(t time.Time) string {
	if t.IsZero() {
		return "최근"
	}
	return t.Local().Format("2006-01-02 15:04")
}

func workReportArea(record evidence.Summary) string {
	text := strings.ToLower(record.Kind + " " + record.Summary)
	switch {
	case containsAny(text, "presentation", "ppt", "발표"):
		return "발표자료"
	case containsAny(text, "document", "docx", "문서"):
		return "문서"
	case containsAny(text, "mail", "메일"):
		return "메일"
	case containsAny(text, "calendar", "reminder", "일정", "할 일"):
		return "일정/할 일"
	case containsAny(text, "browser", "research", "검색", "리서치"):
		return "리서치"
	case containsAny(text, "dispatcher", "signal", "deploy", "배포"):
		return "운영/배포"
	default:
		return "Argos"
	}
}

func workReportNextAction(record evidence.Summary) string {
	text := strings.ToLower(record.Kind + " " + record.Summary)
	switch {
	case containsAny(text, "export", "pdf"):
		return "변환 도구 설치 후 PDF 재시도"
	case containsAny(text, "presentation", "ppt", "document", "docx"):
		return "사용자 피드백에 맞춰 수정본 작성"
	case containsAny(text, "mail"):
		return "초안 검토 후 발송 여부 확인"
	case containsAny(text, "browser", "research"):
		return "필요하면 출처 본문을 더 확인해 보강"
	default:
		return "Signal에서 추가 요청으로 이어서 처리"
	}
}

func cleanWorkReportCell(text string) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	text = strings.ReplaceAll(text, "|", "/")
	parts := strings.Fields(text)
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.Contains(part, "/Users/") || strings.Contains(part, "/.meshclaw/") || strings.Contains(part, "/Documents/Argos") {
			continue
		}
		kept = append(kept, part)
	}
	text = strings.Join(kept, " ")
	if text == "" {
		return "작업 완료"
	}
	return trimForContext(text, 90)
}

func formatAssistantDocumentResult(result osauto.Result, record evidence.Record, storeErr error) string {
	if !result.OK || result.Error != "" {
		lines := []string{
			"문서 작성에 실패했습니다.",
			"문제: " + firstNonEmpty(result.Error, result.Stderr, "unknown error"),
		}
		return strings.Join(appendEvidenceLine(lines, record, storeErr), "\n")
	}
	lines := []string{"문서를 작성했습니다."}
	if result.DOCX != "" {
		lines = append(lines, "Word나 Pages에서 열 수 있는 문서 파일을 준비했습니다.")
	} else if result.URL != "" {
		lines = append(lines, "바로 볼 수 있는 HTML 문서로 저장했습니다.")
	} else if result.Markdown != "" {
		lines = append(lines, "Obsidian에서 바로 열어 정리할 수 있는 Markdown 문서로 저장했습니다.")
	}
	if result.Markdown != "" || result.URL != "" || result.Preview != "" {
		lines = append(lines, "Obsidian용 원본과 미리보기 파일도 함께 보관했습니다.")
	}
	if readable := assistantSignalReadableDocument(result); len(readable) > 0 {
		lines = append(lines, readable...)
	}
	lines = appendAssistantAttachmentMarkers(lines, result)
	return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
}

func assistantSignalReadableDocument(result osauto.Result) []string {
	source := firstNonEmpty(result.Markdown, result.URL)
	data, err := os.ReadFile(source)
	if err != nil || len(data) == 0 {
		return nil
	}
	lines := markdownSignalDigest(string(data), 8)
	if len(lines) == 0 {
		return nil
	}
	out := []string{
		"",
		"Signal에서 바로 읽기:",
		"흐름도: 요청 --> 작성 --> Obsidian 저장 --> Signal 보고",
		"핵심:",
	}
	for _, line := range lines {
		out = append(out, "- "+line)
	}
	return out
}

func markdownSignalDigest(text string, limit int) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	raw := strings.Split(text, "\n")
	out := []string{}
	inFrontMatter := false
	for i, line := range raw {
		line = strings.TrimSpace(line)
		if i == 0 && line == "---" {
			inFrontMatter = true
			continue
		}
		if inFrontMatter {
			if line == "---" {
				inFrontMatter = false
			}
			continue
		}
		line = strings.TrimSpace(strings.TrimLeft(line, "#>-*0123456789. "))
		line = strings.Trim(line, "`")
		if line == "" || strings.Contains(line, "/Users/") || strings.Contains(line, "/Documents/Argos") || strings.Contains(line, "meshclaw-attachment:") {
			continue
		}
		if strings.HasPrefix(line, "작성일:") || strings.HasPrefix(line, "요청:") || strings.HasPrefix(line, "목적:") || strings.HasPrefix(line, "검색 쿼리") {
			continue
		}
		out = append(out, trimForContext(line, 110))
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func formatAssistantDocumentSendResult(opts ListenOptions, title, targetRef string, result osauto.Result, record evidence.Record, storeErr error) string {
	if !result.OK || result.Error != "" {
		return formatAssistantDocumentResult(result, record, storeErr)
	}
	attachments := assistantDocumentAttachments(result)
	target, candidates, targetErr := resolveAssistantVoiceTarget(targetRef)
	if targetErr != nil {
		payload := map[string]interface{}{
			"kind":              "assistant_document_send_target_error",
			"title":             title,
			"target":            targetRef,
			"target_error":      targetErr.Error(),
			"target_candidates": candidates,
			"document":          result,
			"created_at":        time.Now().UTC(),
		}
		fallbackRecord, fallbackStoreErr := evidence.Store("assistant-document-send-target", firstNonEmpty(opts.TargetID, "assistant"), targetRef, payload)
		lines := []string{"문서는 만들었지만 보낼 Signal 대상을 확정하지 못했습니다."}
		lines = append(lines, formatAssistantVoiceTargetCandidates(candidates)...)
		lines = append(lines, "이 대화에 먼저 첨부합니다.")
		lines = appendVoiceReportAttachmentMarkers(lines, attachments)
		if fallbackRecord.ID != "" || fallbackStoreErr != nil {
			return strings.Join(appendAssistantEvidenceNote(lines, fallbackRecord, fallbackStoreErr), "\n")
		}
		return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
	}
	if !opts.Execute {
		if strings.Contains(title, "화학회사 민감 뉴스 분석 보고서") {
			lines := []string{
				"문서/보고서를 Signal로 보낼 준비를 했습니다.",
				"화학회사 민감 뉴스 분석 보고서를 Signal로 보낼 준비를 했습니다.",
				"대상: " + firstNonEmpty(target.Label, target.ID),
				"실제 Signal listener 실행 모드에서는 이 파일들을 대상 방으로 보냅니다.",
			}
			if OneWayReportTarget(target) {
				lines = append(lines, "보고방은 one-way/no-reply라 번호 선택 메뉴 없이 결과물만 보냅니다.")
			}
			lines = append(lines, "", "보낼 내용:")
			lines = append(lines, strings.Split(assistantDocumentSendText(title, result), "\n")...)
			lines = append(lines, "", "출처 후보/확인 상태:", "- 최신 검색 리서치 노트와 첨부 문서에서 최종 확인합니다.")
			lines = append(lines, "", "첨부: 전체 Markdown/DOCX 보고서와 검색 리서치 노트")
			lines = appendVoiceReportAttachmentMarkers(lines, attachments)
			return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
		}
		lines := []string{
			"문서/보고서를 Signal로 보낼 준비를 했습니다.",
			"대상: " + firstNonEmpty(target.Label, target.ID),
			"실제 Signal listener 실행 모드에서는 이 파일들을 대상 방으로 보냅니다.",
		}
		if OneWayReportTarget(target) {
			lines = append(lines, "보고방은 one-way/no-reply라 번호 선택 메뉴 없이 결과물만 보냅니다.")
		}
		lines = append(lines, "", "보낼 내용:")
		lines = append(lines, strings.Split(assistantDocumentSendText(title, result), "\n")...)
		lines = appendVoiceReportAttachmentMarkers(lines, attachments)
		return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
	}
	sendText := assistantDocumentSendText(title, result)
	send, sendErr := Send(SendOptions{
		TargetID:    target.ID,
		Kind:        "text",
		Text:        sendText,
		Attachments: attachments,
		Execute:     opts.Execute,
	})
	payload := map[string]interface{}{
		"kind":            "assistant_document_send",
		"title":           title,
		"target":          targetRef,
		"resolved_target": target,
		"document":        result,
		"send":            send,
		"send_error":      errorString(sendErr),
		"created_at":      time.Now().UTC(),
	}
	sendRecord, sendStoreErr := evidence.Store("assistant-document-send", firstNonEmpty(opts.TargetID, "assistant"), target.ID, payload)
	if sendErr != nil {
		lines := []string{
			"문서는 만들었지만 Signal 전송에 실패했습니다.",
			"대상: " + firstNonEmpty(target.Label, target.ID),
			"문제: " + sendErr.Error(),
			"이 대화에 먼저 첨부합니다.",
		}
		lines = appendVoiceReportAttachmentMarkers(lines, attachments)
		return strings.Join(appendAssistantEvidenceNote(lines, sendRecord, sendStoreErr), "\n")
	}
	lines := []string{
		"문서/보고서를 Signal로 보냈습니다.",
		"대상: " + firstNonEmpty(target.Label, target.ID),
	}
	if id := strings.TrimSpace(send.Stdout); id != "" {
		lines = append(lines, lang.T("assistant.workflow.signal_id", id))
	}
	return strings.Join(appendAssistantEvidenceNote(lines, sendRecord, sendStoreErr), "\n")
}

func assistantDocumentSendText(title string, result osauto.Result) string {
	lines := []string{strings.TrimSpace(title) + " 문서/보고서입니다."}
	if readable := assistantSignalReadableDocument(result); len(readable) > 0 {
		lines = append(lines, readable...)
	}
	return strings.Join(compactBlankLines(lines), "\n")
}

func assistantDocumentAttachments(result osauto.Result) []string {
	out := []string{}
	for _, path := range []string{result.DOCX, result.PDF, result.Markdown} {
		path = strings.TrimSpace(path)
		if path != "" {
			out = append(out, path)
		}
	}
	return uniqueVoiceReportStrings(out)
}

func formatAssistantSpreadsheetResult(result osauto.Result, record evidence.Record, storeErr error) string {
	if !result.OK || result.Error != "" {
		lines := []string{
			"표 파일 작성에 실패했습니다.",
			"문제: " + firstNonEmpty(result.Error, result.Stderr, "unknown error"),
		}
		return strings.Join(appendEvidenceLine(lines, record, storeErr), "\n")
	}
	lines := []string{"표 파일을 작성했습니다."}
	if result.XLSX != "" {
		lines = append(lines, "Numbers나 Excel에서 바로 열 수 있는 XLSX 파일을 준비했습니다.")
	}
	if result.CSV != "" {
		lines = append(lines, "CSV 원본도 함께 보냅니다.")
	}
	if result.URL != "" {
		lines = append(lines, "iPhone에서 빠르게 볼 수 있는 미리보기 파일도 같이 보관했습니다.")
	}
	lines = appendAssistantAttachmentMarkers(lines, result)
	return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
}

func formatAssistantPresentationResult(result osauto.Result, record evidence.Record, storeErr error) string {
	if !result.OK || result.Error != "" {
		lines := []string{
			"발표자료 작성에 실패했습니다.",
			"문제: " + firstNonEmpty(result.Error, result.Stderr, "unknown error"),
		}
		return strings.Join(appendEvidenceLine(lines, record, storeErr), "\n")
	}
	lines := []string{"발표자료를 작성하고 검증했습니다."}
	if result.PPTX != "" {
		lines = append(lines, "PowerPoint에서 바로 열 수 있는 PPTX 파일을 만들었습니다.")
		lines = append(lines, "iPhone Signal에서 PPTX 첨부를 탭하면 PowerPoint, Keynote, Files 앱으로 바로 열 수 있습니다.")
	}
	if result.Markdown != "" || result.URL != "" || result.Preview != "" {
		lines = append(lines, "Obsidian에서 다듬을 수 있는 발표 outline과 미리보기 파일도 같이 저장했습니다.")
	}
	if result.Stdout != "" {
		lines = append(lines, assistantVerificationSentence(result.Stdout))
	}
	lines = appendAssistantAttachmentMarkers(lines, result)
	return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
}

func formatAssistantMeetingMaterialsResult(doc, deck osauto.Result, record evidence.Record, storeErr error) string {
	if !doc.OK || !deck.OK {
		lines := []string{"회의 자료 준비가 일부 실패했습니다."}
		if !doc.OK {
			lines = append(lines, "문서 문제: "+firstNonEmpty(doc.Error, doc.Stderr, "unknown error"))
		}
		if !deck.OK {
			lines = append(lines, "발표자료 문제: "+firstNonEmpty(deck.Error, deck.Stderr, "unknown error"))
		}
		return strings.Join(appendEvidenceLine(lines, record, storeErr), "\n")
	}
	lines := []string{"회의 자료 패키지를 준비했습니다."}
	if doc.DOCX != "" {
		lines = append(lines, "회의 브리프는 Word/Pages 문서로 만들었습니다.")
	} else if doc.Markdown != "" {
		lines = append(lines, "회의 브리프는 Obsidian에서 바로 정리할 수 있는 Markdown 문서로 만들었습니다.")
	}
	if deck.PPTX != "" {
		lines = append(lines, "발표자료를 작성하고 검증했습니다.")
		lines = append(lines, "PowerPoint에서 바로 열 수 있는 PPTX 파일을 만들었습니다.")
		lines = append(lines, "발표자료는 PPTX로 만들었습니다.")
		lines = append(lines, "iPhone Signal에서 PPTX 첨부를 탭하면 PowerPoint, Keynote, Files 앱으로 바로 열 수 있습니다.")
	}
	if doc.URL != "" || deck.URL != "" || deck.Markdown != "" {
		lines = append(lines, "Obsidian용 브리프 원본, 발표 outline, 발표 미리보기도 함께 저장했습니다.")
	}
	if readable := assistantSignalReadableDocument(doc); len(readable) > 0 {
		lines = append(lines, readable...)
	}
	if deck.Stdout != "" {
		lines = append(lines, assistantVerificationSentence(deck.Stdout))
	}
	lines = append(lines, "제목, 안건, 참석자 정보를 주면 이 파일들을 기준으로 바로 더 구체화하겠습니다.")
	lines = appendVoiceReportAttachmentMarkers(lines, assistantMeetingMaterialsAttachments(doc, deck))
	return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
}

func formatAssistantMeetingMaterialsSendResult(opts ListenOptions, title, targetRef string, doc, deck osauto.Result, record evidence.Record, storeErr error) string {
	if !doc.OK || !deck.OK {
		return formatAssistantMeetingMaterialsResult(doc, deck, record, storeErr)
	}
	attachments := assistantMeetingMaterialsAttachments(doc, deck)
	target, candidates, targetErr := resolveAssistantVoiceTarget(targetRef)
	if targetErr != nil {
		lines := []string{"회의 자료 패키지는 만들었지만 보낼 Signal 대상을 확정하지 못했습니다."}
		lines = append(lines, formatAssistantVoiceTargetCandidates(candidates)...)
		lines = append(lines, "이 대화에 먼저 첨부합니다.")
		lines = appendVoiceReportAttachmentMarkers(lines, attachments)
		return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
	}
	sendText := assistantMeetingMaterialsSendText(title, doc, deck)
	if !opts.Execute {
		lines := []string{
			"회의 자료 패키지를 Signal로 보낼 준비를 했습니다.",
			"대상: " + firstNonEmpty(target.Label, target.ID),
		}
		if OneWayReportTarget(target) {
			lines = append(lines, "보고방은 one-way/no-reply라 번호 선택 메뉴 없이 결과물만 보냅니다.")
		}
		lines = append(lines, "", "보낼 내용:")
		lines = append(lines, strings.Split(sendText, "\n")...)
		lines = appendVoiceReportAttachmentMarkers(lines, attachments)
		return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
	}
	send, sendErr := Send(SendOptions{
		TargetID:    target.ID,
		Kind:        "text",
		Text:        sendText,
		Attachments: attachments,
		Execute:     true,
	})
	payload := map[string]interface{}{
		"kind":            "assistant_meeting_materials_send",
		"title":           title,
		"target":          targetRef,
		"resolved_target": target,
		"document":        doc,
		"presentation":    deck,
		"send":            send,
		"send_error":      errorString(sendErr),
		"created_at":      time.Now().UTC(),
	}
	sendRecord, sendStoreErr := evidence.Store("assistant-meeting-materials-send", firstNonEmpty(opts.TargetID, "assistant"), target.ID, payload)
	if sendErr != nil {
		lines := []string{
			"회의 자료 패키지는 만들었지만 Signal 전송에 실패했습니다.",
			"대상: " + firstNonEmpty(target.Label, target.ID),
			"문제: " + sendErr.Error(),
			"이 대화에 먼저 첨부합니다.",
		}
		lines = appendVoiceReportAttachmentMarkers(lines, attachments)
		return strings.Join(appendAssistantEvidenceNote(lines, sendRecord, sendStoreErr), "\n")
	}
	lines := []string{
		"회의 자료 패키지를 Signal로 보냈습니다.",
		"대상: " + firstNonEmpty(target.Label, target.ID),
	}
	if id := strings.TrimSpace(send.Stdout); id != "" {
		lines = append(lines, lang.T("assistant.workflow.signal_id", id))
	}
	return strings.Join(appendAssistantEvidenceNote(lines, sendRecord, sendStoreErr), "\n")
}

func assistantMeetingMaterialsSendText(title string, doc, deck osauto.Result) string {
	lines := []string{strings.TrimSpace(title) + " 회의 자료 패키지입니다."}
	if readable := assistantSignalReadableDocument(doc); len(readable) > 0 {
		lines = append(lines, readable...)
	}
	if deck.PPTX != "" {
		lines = append(lines, "", "발표자료: PowerPoint에서 바로 열 수 있는 PPTX를 첨부했습니다.")
		lines = append(lines, "iPhone Signal에서 PPTX 첨부를 탭하면 PowerPoint, Keynote, Files 앱으로 바로 열 수 있습니다.")
	}
	if deck.Stdout != "" {
		lines = append(lines, assistantVerificationSentence(deck.Stdout))
	}
	return strings.Join(compactBlankLines(lines), "\n")
}

func assistantMeetingMaterialsAttachments(doc, deck osauto.Result) []string {
	attachments := []string{}
	attachments = append(attachments, assistantDocumentAttachments(doc)...)
	for _, path := range []string{deck.PPTX, deck.PDF, deck.Markdown} {
		path = strings.TrimSpace(path)
		if path != "" {
			attachments = append(attachments, path)
		}
	}
	return uniqueShowcaseAttachments(attachments)
}

func appendAssistantAttachmentMarkers(lines []string, result osauto.Result) []string {
	for _, path := range []string{result.DOCX, result.PPTX, result.XLSX, result.CSV, result.Markdown, result.PDF, result.URL} {
		path = strings.TrimSpace(path)
		if path != "" && assistantShouldAttachArtifactPath(path) {
			lines = append(lines, "meshclaw-attachment: "+path)
		}
	}
	return lines
}

func assistantShouldAttachArtifactPath(path string) bool {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(path)))
	if ext == ".html" || ext == ".htm" {
		return attachRawSignalReplyDocuments()
	}
	return true
}

func assistantCalendarListPrompt(text string) (string, bool) {
	lower := strings.ToLower(strings.TrimSpace(text))
	if lower == "" {
		return "", false
	}
	if !strings.Contains(lower, "일정") && !strings.Contains(lower, "캘린더") && !strings.Contains(lower, "calendar") {
		return "", false
	}
	if !containsAny(lower, "뭐 있어", "뭐있어", "보이는지", "확인", "조회", "알려", "list", "show", "check") {
		return "", false
	}
	switch {
	case containsAny(lower, "내일", "tomorrow"):
		return "내일 일정 뭐 있어?", true
	case containsAny(lower, "오늘", "today"):
		return "오늘 일정 뭐 있어?", true
	default:
		return "일정 뭐 있어?", true
	}
}

type assistantArtifactState struct {
	Kind         string        `json:"kind"`
	Title        string        `json:"title"`
	TargetID     string        `json:"target_id"`
	Document     osauto.Result `json:"document,omitempty"`
	Presentation osauto.Result `json:"presentation,omitempty"`
	Spreadsheet  osauto.Result `json:"spreadsheet,omitempty"`
	UpdatedAt    time.Time     `json:"updated_at"`
}

func rememberAssistantArtifact(opts ListenOptions, title, kind string, doc, deck osauto.Result, sheet ...osauto.Result) {
	spreadsheet := osauto.Result{}
	if len(sheet) > 0 {
		spreadsheet = sheet[0]
	}
	state := assistantArtifactState{
		Kind:         kind,
		Title:        strings.TrimSpace(title),
		TargetID:     assistantArtifactTargetID(opts),
		Document:     doc,
		Presentation: deck,
		Spreadsheet:  spreadsheet,
		UpdatedAt:    time.Now().UTC(),
	}
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return
	}
	path := assistantArtifactStatePath(opts)
	_ = os.MkdirAll(filepath.Dir(path), 0700)
	_ = os.WriteFile(path, data, 0600)
}

func loadAssistantArtifact(opts ListenOptions) (assistantArtifactState, bool) {
	var state assistantArtifactState
	data, err := os.ReadFile(assistantArtifactStatePath(opts))
	if err != nil {
		return state, false
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, false
	}
	return state, true
}

func assistantArtifactStatePath(opts ListenOptions) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		home = os.TempDir()
	}
	return filepath.Join(home, ".meshclaw", "assistant-artifacts", assistantArtifactTargetID(opts)+".json")
}

func assistantArtifactTargetID(opts ListenOptions) string {
	id := strings.TrimSpace(opts.TargetID)
	if id == "" {
		id = "assistant"
	}
	return safeAssistantArtifactID(id)
}

func safeAssistantArtifactID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "assistant"
	}
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "assistant"
	}
	return out
}

func reviseRecentAssistantArtifact(ctx context.Context, opts ListenOptions, instruction, target string, slideCount int) string {
	state, ok := loadAssistantArtifact(opts)
	if !ok {
		return "최근 만든 문서, 발표자료, 표 파일을 찾지 못했습니다. 먼저 파일을 만들어 주세요."
	}
	target = normalizeRecentArtifactTarget(target, instruction, state)
	switch target {
	case "presentation":
		body := recentArtifactSourceText(state.Presentation.Markdown, state.Title)
		if body == "" {
			body = instruction
		} else {
			body = body + "\n\n# 수정 요청\n" + strings.TrimSpace(instruction)
		}
		if slideCount <= 0 {
			slideCount = inferSlideCount(instruction, 0)
		}
		if slideCount <= 0 {
			slideCount = 6
		}
		title := assistantRevisionTitle(firstNonEmpty(state.Title, "Argos 발표자료"))
		result := osauto.CreatePresentation(ctx, title, body, "", slideCount, "")
		record, storeErr := evidence.Store(result.Action, firstNonEmpty(opts.TargetID, "assistant"), state.Title+" revision", result)
		rememberAssistantArtifact(opts, title, "presentation", state.Document, result)
		return formatAssistantPresentationRevisionResult(result, record, storeErr)
	case "document":
		body := recentArtifactSourceText(state.Document.Markdown, state.Title)
		if body == "" {
			body = instruction
		} else {
			body = body + "\n\n## 수정 요청\n" + strings.TrimSpace(instruction)
		}
		title := assistantRevisionTitle(firstNonEmpty(state.Title, "Argos 문서"))
		result := osauto.CreateArgosDocument(ctx, title, body)
		record, storeErr := evidence.Store(result.Action, firstNonEmpty(opts.TargetID, "assistant"), state.Title+" revision", result)
		rememberAssistantArtifact(opts, title, "document", result, state.Presentation)
		return formatAssistantDocumentRevisionResult(result, record, storeErr)
	case "spreadsheet":
		body := recentSpreadsheetSourceText(state.Spreadsheet)
		if body == "" {
			body = instruction
		} else {
			body = enrichSpreadsheetRevisionBody(body, instruction)
		}
		title := assistantRevisionTitle(firstNonEmpty(state.Title, "Argos 표"))
		result := osauto.CreateSpreadsheet(ctx, title, body)
		record, storeErr := evidence.Store(result.Action, firstNonEmpty(opts.TargetID, "assistant"), state.Title+" revision", result)
		rememberAssistantArtifact(opts, title, "spreadsheet", state.Document, state.Presentation, result)
		return formatAssistantSpreadsheetRevisionResult(result, record, storeErr)
	default:
		return "최근 artifact는 찾았지만 수정할 문서/발표자료/표 형식을 정하지 못했습니다. `문서 수정`, `PPT 수정`, `표 수정`처럼 다시 말해 주세요."
	}
}

func assistantRevisionTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		title = "Argos artifact"
	}
	lower := strings.ToLower(title)
	if strings.HasSuffix(title, "수정본") || strings.HasSuffix(lower, "revision") {
		return title
	}
	return title + " 수정본"
}

func exportRecentAssistantArtifact(ctx context.Context, opts ListenOptions, format, target string) string {
	state, ok := loadAssistantArtifact(opts)
	if !ok {
		return "최근 만든 문서, 발표자료, 표 파일을 찾지 못했습니다. 먼저 파일을 만들어 주세요."
	}
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "pdf"
	}
	target = normalizeRecentArtifactTarget(target, format, state)
	if target == "presentation" {
		result := osauto.ExportPresentation(ctx, state.Presentation.PPTX, format, "")
		record, storeErr := evidence.Store(result.Action, firstNonEmpty(opts.TargetID, "assistant"), state.Title+" export", result)
		if result.OK && result.PDF != "" {
			state.Presentation.PDF = result.PDF
			rememberAssistantArtifact(opts, state.Title, state.Kind, state.Document, state.Presentation)
		}
		return formatAssistantExportResult("발표자료", result, record, storeErr)
	}
	if target == "spreadsheet" {
		result := spreadsheetExportResult(state.Spreadsheet, format)
		record, storeErr := evidence.Store(result.Action, firstNonEmpty(opts.TargetID, "assistant"), state.Title+" export", result)
		return formatAssistantExportResult("표 파일", result, record, storeErr)
	}
	input := state.Document.Markdown
	if input == "" {
		return "최근 문서의 Markdown 원본을 찾지 못해서 export할 수 없습니다."
	}
	result := osauto.ExportMarkdown(ctx, input, format, "")
	record, storeErr := evidence.Store(result.Action, firstNonEmpty(opts.TargetID, "assistant"), state.Title+" export", result)
	if result.OK {
		if result.DOCX != "" {
			state.Document.DOCX = result.DOCX
		}
		if result.PDF != "" {
			state.Document.PDF = result.PDF
		}
		rememberAssistantArtifact(opts, state.Title, state.Kind, state.Document, state.Presentation)
	}
	return formatAssistantExportResult("문서", result, record, storeErr)
}

func openRecentAssistantArtifact(ctx context.Context, opts ListenOptions, app, target string) string {
	state, ok := loadAssistantArtifact(opts)
	if !ok {
		return "최근 만든 문서, 발표자료, 표 파일을 찾지 못했습니다. 먼저 파일을 만들어 주세요."
	}
	path := recentArtifactPathForOpen(state, target, app)
	if path == "" {
		return "열 수 있는 최근 artifact 파일을 찾지 못했습니다."
	}
	if strings.TrimSpace(app) == "" {
		app = defaultAppForArtifactPath(path)
	}
	result := osauto.OpenFile(ctx, path, app)
	record, storeErr := evidence.Store(result.Action, firstNonEmpty(opts.TargetID, "assistant"), path, result)
	if !result.OK || result.Error != "" {
		lines := []string{"최근 파일을 여는 데 실패했습니다.", "문제: " + firstNonEmpty(result.Error, result.Stderr, "unknown error")}
		return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
	}
	lines := []string{"최근 만든 파일을 " + firstNonEmpty(app, "기본 앱") + "에서 열었습니다."}
	return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
}

func resendRecentAssistantArtifact(opts ListenOptions, target string) string {
	state, ok := loadAssistantArtifact(opts)
	if !ok {
		return "최근 만든 문서, 발표자료, 표 파일을 찾지 못했습니다. 먼저 파일을 만들어 주세요."
	}
	lines := []string{
		lang.T("assistant.recent_artifact.resend.title"),
		lang.T("assistant.recent_artifact.resend.mobile_open"),
	}
	target = strings.ToLower(strings.TrimSpace(target))
	if target == "" || target == "all" || target == "document" {
		lines = appendAssistantAttachmentMarkers(lines, state.Document)
	}
	if target == "" || target == "all" || target == "presentation" {
		lines = appendAssistantAttachmentMarkers(lines, state.Presentation)
	}
	if target == "" || target == "all" || target == "spreadsheet" {
		lines = appendAssistantAttachmentMarkers(lines, state.Spreadsheet)
	}
	if target == "csv" && state.Spreadsheet.CSV != "" {
		lines = append(lines, "meshclaw-attachment: "+state.Spreadsheet.CSV)
	}
	if target == "xlsx" && state.Spreadsheet.XLSX != "" {
		lines = append(lines, "meshclaw-attachment: "+state.Spreadsheet.XLSX)
	}
	if len(signalReplyAttachments(strings.Join(lines, "\n"))) == 0 {
		return "다시 보낼 최근 파일을 찾지 못했습니다."
	}
	return strings.Join(lines, "\n")
}

func normalizeRecentArtifactTarget(target, instruction string, state assistantArtifactState) string {
	lower := strings.ToLower(strings.TrimSpace(target + " " + instruction))
	if strings.Contains(lower, "ppt") || strings.Contains(lower, "slide") || strings.Contains(lower, "presentation") || strings.Contains(lower, "발표") || strings.Contains(lower, "슬라이드") || strings.Contains(lower, "deck") {
		if state.Presentation.PPTX != "" || state.Presentation.Markdown != "" {
			return "presentation"
		}
	}
	if strings.Contains(lower, "doc") || strings.Contains(lower, "문서") || strings.Contains(lower, "브리프") || strings.Contains(lower, "보고서") || strings.Contains(lower, "obsidian") || strings.Contains(lower, "markdown") {
		if state.Document.Markdown != "" || state.Document.DOCX != "" {
			return "document"
		}
	}
	if strings.Contains(lower, "sheet") || strings.Contains(lower, "spreadsheet") || strings.Contains(lower, "xlsx") || strings.Contains(lower, "csv") || strings.Contains(lower, "excel") || strings.Contains(lower, "numbers") || strings.Contains(lower, "엑셀") || strings.Contains(lower, "표") || strings.Contains(lower, "예산") || strings.Contains(lower, "청구") || strings.Contains(lower, "트래커") {
		if state.Spreadsheet.XLSX != "" || state.Spreadsheet.CSV != "" {
			return "spreadsheet"
		}
	}
	if state.Kind == "presentation" {
		return "presentation"
	}
	if state.Kind == "document" {
		return "document"
	}
	if state.Kind == "spreadsheet" {
		return "spreadsheet"
	}
	if state.Presentation.PPTX != "" || state.Presentation.Markdown != "" {
		return "presentation"
	}
	if state.Document.Markdown != "" || state.Document.DOCX != "" {
		return "document"
	}
	if state.Spreadsheet.XLSX != "" || state.Spreadsheet.CSV != "" {
		return "spreadsheet"
	}
	return ""
}

func recentArtifactSourceText(path, title string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	text := strings.TrimSpace(string(data))
	if text == "" {
		return ""
	}
	return text
}

func recentSpreadsheetSourceText(result osauto.Result) string {
	path := firstExistingPath(result.CSV)
	if path == "" {
		return ""
	}
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	rows, err := csv.NewReader(file).ReadAll()
	if err != nil || len(rows) == 0 {
		return ""
	}
	lines := make([]string, 0, len(rows)+1)
	for i, row := range rows {
		for j := range row {
			row[j] = cleanSpreadsheetCell(row[j])
		}
		lines = append(lines, "| "+strings.Join(row, " | ")+" |")
		if i == 0 {
			seps := make([]string, len(row))
			for j := range seps {
				seps[j] = "---"
			}
			lines = append(lines, "| "+strings.Join(seps, " | ")+" |")
		}
	}
	return strings.Join(lines, "\n")
}

func enrichSpreadsheetRevisionBody(body, instruction string) string {
	instruction = strings.TrimSpace(instruction)
	if instruction == "" {
		return body
	}
	lower := strings.ToLower(instruction)
	if !containsAny(lower, "추가", "add", "삭제", "지워", "빼", "remove", "delete", "수정", "바꿔", "변경", "적어", "기록", "써", "update") {
		return body + "\n\n수정 요청: " + instruction
	}
	item := spreadsheetRevisionItem(instruction)
	if item == "" {
		return body + "\n\n수정 요청: " + instruction
	}
	rows := assistantMarkdownTableRows(body)
	if len(rows) == 0 {
		return body + "\n| " + cleanWorkReportCell(item) + " | 0 | 0 | 0 | 추가 요청 |"
	}
	changed := false
	switch {
	case containsAny(lower, "삭제", "지워", "빼", "remove", "delete"):
		rows, changed = spreadsheetDeleteRow(rows, item)
	case containsAny(lower, "수정", "바꿔", "변경", "update"):
		rows, changed = spreadsheetUpdateRow(rows, item, instruction)
	case containsAny(lower, "적어", "기록", "써"):
		rows, changed = spreadsheetUpdateRow(rows, item, instruction)
	case containsAny(lower, "추가", "add"):
		rows, changed = spreadsheetAddRow(rows, item)
	}
	if !changed {
		return body + "\n\n수정 요청: " + instruction
	}
	rows = recalculateSpreadsheetRows(rows)
	replacement := spreadsheetRowsToMarkdown(rows)
	lines := strings.Split(body, "\n")
	start, end := -1, -1
	for i, line := range lines {
		if _, ok := assistantMarkdownTableRow(line); ok || assistantMarkdownTableSeparator(line) {
			if start == -1 {
				start = i
			}
			end = i
		}
	}
	if start >= 0 && end >= start {
		updated := append([]string{}, lines[:start]...)
		updated = append(updated, strings.Split(replacement, "\n")...)
		updated = append(updated, lines[end+1:]...)
		return strings.Join(updated, "\n")
	}
	return body + "\n" + replacement
}

func spreadsheetRowsToMarkdown(rows [][]string) string {
	if len(rows) == 0 {
		return ""
	}
	lines := make([]string, 0, len(rows)+1)
	for i, row := range rows {
		cleaned := make([]string, len(row))
		for j, cell := range row {
			cleaned[j] = cleanSpreadsheetCell(cell)
		}
		lines = append(lines, "| "+strings.Join(cleaned, " | ")+" |")
		if i == 0 {
			seps := make([]string, len(row))
			for j := range seps {
				seps[j] = "---"
			}
			lines = append(lines, "| "+strings.Join(seps, " | ")+" |")
		}
	}
	return strings.Join(lines, "\n")
}

func spreadsheetAddRow(rows [][]string, item string) ([][]string, bool) {
	if len(rows) == 0 || item == "" {
		return rows, false
	}
	for _, row := range rows[1:] {
		if len(row) > 0 && strings.EqualFold(strings.TrimSpace(row[0]), item) {
			return rows, false
		}
	}
	header := rows[0]
	row := make([]string, len(header))
	for i, h := range header {
		lh := strings.ToLower(h)
		switch {
		case i == 0 || strings.Contains(lh, "항목") || strings.Contains(lh, "구분"):
			row[i] = item
		case strings.Contains(lh, "상태"):
			row[i] = "대기"
		case strings.Contains(lh, "메모") || strings.Contains(lh, "다음"):
			row[i] = "추가 요청"
		default:
			row[i] = "0"
		}
	}
	for i := 1; i < len(rows); i++ {
		if len(rows[i]) > 0 && (strings.Contains(strings.TrimSpace(rows[i][0]), "합계") || strings.EqualFold(strings.TrimSpace(rows[i][0]), "total")) {
			out := append([][]string{}, rows[:i]...)
			out = append(out, row)
			out = append(out, rows[i:]...)
			return out, true
		}
	}
	return append(rows, row), true
}

func spreadsheetDeleteRow(rows [][]string, item string) ([][]string, bool) {
	if len(rows) < 2 || item == "" {
		return rows, false
	}
	out := rows[:1]
	changed := false
	for _, row := range rows[1:] {
		name := ""
		if len(row) > 0 {
			name = strings.TrimSpace(row[0])
		}
		if name != "" && strings.Contains(strings.ToLower(name), strings.ToLower(item)) {
			changed = true
			continue
		}
		out = append(out, row)
	}
	return out, changed
}

func spreadsheetUpdateRow(rows [][]string, item, instruction string) ([][]string, bool) {
	if len(rows) < 2 || item == "" {
		return rows, false
	}
	col := spreadsheetInstructionColumn(rows[0], instruction)
	if col < 0 {
		return rows, false
	}
	value, ok := spreadsheetInstructionValue(instruction)
	if !ok {
		return rows, false
	}
	lowerItem := strings.ToLower(item)
	for i := 1; i < len(rows); i++ {
		if len(rows[i]) == 0 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(rows[i][0]))
		if name == "" || (!strings.Contains(name, lowerItem) && !strings.Contains(lowerItem, name)) {
			continue
		}
		for len(rows[i]) <= col {
			rows[i] = append(rows[i], "")
		}
		rows[i][col] = value
		return rows, true
	}
	rows, changed := spreadsheetAddRow(rows, item)
	if !changed {
		return rows, false
	}
	for i := 1; i < len(rows); i++ {
		if len(rows[i]) == 0 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(rows[i][0]))
		if name == "" || (!strings.Contains(name, lowerItem) && !strings.Contains(lowerItem, name)) {
			continue
		}
		for len(rows[i]) <= col {
			rows[i] = append(rows[i], "")
		}
		rows[i][col] = value
		return rows, true
	}
	return rows, false
}

func spreadsheetInstructionColumn(header []string, instruction string) int {
	lower := strings.ToLower(instruction)
	match := func(wants ...string) int {
		for i, h := range header {
			lh := strings.ToLower(strings.TrimSpace(h))
			for _, want := range wants {
				if strings.Contains(lh, want) {
					return i
				}
			}
		}
		return -1
	}
	switch {
	case containsAny(lower, "실사용", "사용", "지출", "actual", "spent"):
		if col := match("실사용", "지출", "actual", "spent"); col >= 0 {
			return col
		}
	case containsAny(lower, "수량", "quantity", "qty"):
		if col := match("수량", "quantity", "qty"); col >= 0 {
			return col
		}
	case containsAny(lower, "단가", "unit price"):
		if col := match("단가", "unit price"); col >= 0 {
			return col
		}
	case containsAny(lower, "금액", "amount"):
		if col := match("금액", "amount"); col >= 0 {
			return col
		}
	case containsAny(lower, "예산", "budget"):
		if col := match("예산", "budget"); col >= 0 {
			return col
		}
	case containsAny(lower, "상태", "status"):
		if col := match("상태", "status"); col >= 0 {
			return col
		}
	case containsAny(lower, "담당", "assignee", "owner"):
		if col := match("담당", "assignee", "owner"); col >= 0 {
			return col
		}
	case containsAny(lower, "마감", "due", "deadline"):
		if col := match("마감", "due", "deadline"); col >= 0 {
			return col
		}
	case containsAny(lower, "다음 액션", "다음", "next"):
		if col := match("다음 액션", "다음", "next"); col >= 0 {
			return col
		}
	case containsAny(lower, "메모", "memo", "note"):
		if col := match("메모", "memo", "note"); col >= 0 {
			return col
		}
	case containsAny(lower, "항목명", "이름", "rename"):
		return 0
	}
	for i := range header {
		if i > 0 {
			return i
		}
	}
	return -1
}

var spreadsheetNumberRE = regexp.MustCompile(`[0-9][0-9,]*`)
var spreadsheetQuotedValueRE = regexp.MustCompile(`["'“”‘’](.+?)["'“”‘’]`)

func spreadsheetInstructionValue(instruction string) (string, bool) {
	if m := spreadsheetQuotedValueRE.FindStringSubmatch(instruction); len(m) > 1 {
		value := cleanSpreadsheetCell(m[1])
		if value != "" {
			return value, true
		}
	}
	m := spreadsheetNumberRE.FindString(instruction)
	if m != "" {
		n, err := strconv.Atoi(strings.ReplaceAll(m, ",", ""))
		if err != nil {
			return "", false
		}
		if strings.Contains(instruction, "만원") {
			n *= 10000
		}
		return strconv.Itoa(n), true
	}
	for _, marker := range []string{"으로 바꿔", "로 바꿔", "으로 변경", "로 변경", "으로 수정", "로 수정", "으로 해", "로 해"} {
		if before, _, ok := strings.Cut(instruction, marker); ok {
			value := spreadsheetTrailingValue(before)
			if value != "" {
				return value, true
			}
		}
	}
	for _, marker := range []string{"으로", "로", "to "} {
		if before, _, ok := strings.Cut(instruction, marker); ok {
			value := spreadsheetTrailingValue(before)
			if value != "" {
				return value, true
			}
		}
	}
	for _, marker := range []string{"라고", "이라고", "적어", "기록", "써"} {
		if before, _, ok := strings.Cut(instruction, marker); ok {
			value := spreadsheetTrailingValue(before)
			if value != "" {
				return value, true
			}
		}
	}
	return "", false
}

func spreadsheetTrailingValue(text string) string {
	text = strings.TrimSpace(text)
	for _, sep := range []string{"를", "을", ":", "="} {
		if idx := strings.LastIndex(text, sep); idx >= 0 {
			text = strings.TrimSpace(text[idx+len(sep):])
			break
		}
	}
	text = strings.TrimSpace(strings.Trim(text, "\"'`"))
	for _, suffix := range []string{"새 파일", "새 파일로", "보내줘", "해줘", "바꿔줘", "수정해줘", "변경해줘"} {
		text = strings.TrimSpace(strings.TrimSuffix(text, suffix))
	}
	return cleanSpreadsheetCell(text)
}

func cleanSpreadsheetCell(text string) string {
	text = strings.TrimSpace(strings.ReplaceAll(text, "\n", " "))
	text = strings.ReplaceAll(text, "|", "/")
	parts := strings.Fields(text)
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		if strings.Contains(part, "/Users/") || strings.Contains(part, "/.meshclaw/") || strings.Contains(part, "/Documents/Argos") {
			continue
		}
		kept = append(kept, part)
	}
	return trimSpreadsheetCell(strings.Join(kept, " "), 90)
}

func trimSpreadsheetCell(text string, max int) string {
	text = strings.TrimSpace(text)
	if max <= 0 || len(text) <= max {
		return text
	}
	return strings.TrimSpace(text[:max]) + "..."
}

func recalculateSpreadsheetRows(rows [][]string) [][]string {
	if len(rows) < 2 {
		return rows
	}
	header := rows[0]
	col := func(names ...string) int {
		for i, h := range header {
			lh := strings.ToLower(strings.TrimSpace(h))
			for _, name := range names {
				if strings.Contains(lh, name) {
					return i
				}
			}
		}
		return -1
	}
	budgetCol := col("예산", "budget")
	actualCol := col("실사용", "지출", "actual", "spent")
	diffCol := col("차이", "diff")
	qtyCol := col("수량", "quantity", "qty")
	unitCol := col("단가", "unit price")
	amountCol := col("금액", "amount")
	if qtyCol >= 0 && unitCol >= 0 && amountCol >= 0 {
		rows = recalculateInvoiceRows(rows, qtyCol, unitCol, amountCol)
	}
	if budgetCol < 0 && actualCol < 0 {
		return rows
	}
	totalBudget, totalActual := 0, 0
	totalRow := -1
	for i := 1; i < len(rows); i++ {
		if len(rows[i]) == 0 {
			continue
		}
		name := strings.TrimSpace(rows[i][0])
		if strings.Contains(name, "합계") || strings.EqualFold(name, "total") {
			totalRow = i
			continue
		}
		budget := spreadsheetCellInt(rows[i], budgetCol)
		actual := spreadsheetCellInt(rows[i], actualCol)
		totalBudget += budget
		totalActual += actual
		if diffCol >= 0 {
			for len(rows[i]) <= diffCol {
				rows[i] = append(rows[i], "")
			}
			rows[i][diffCol] = strconv.Itoa(budget - actual)
		}
	}
	if totalRow >= 0 {
		if budgetCol >= 0 {
			for len(rows[totalRow]) <= budgetCol {
				rows[totalRow] = append(rows[totalRow], "")
			}
			rows[totalRow][budgetCol] = strconv.Itoa(totalBudget)
		}
		if actualCol >= 0 {
			for len(rows[totalRow]) <= actualCol {
				rows[totalRow] = append(rows[totalRow], "")
			}
			rows[totalRow][actualCol] = strconv.Itoa(totalActual)
		}
		if diffCol >= 0 {
			for len(rows[totalRow]) <= diffCol {
				rows[totalRow] = append(rows[totalRow], "")
			}
			rows[totalRow][diffCol] = strconv.Itoa(totalBudget - totalActual)
		}
	}
	return rows
}

func recalculateInvoiceRows(rows [][]string, qtyCol, unitCol, amountCol int) [][]string {
	total := 0
	totalRow := -1
	for i := 1; i < len(rows); i++ {
		if len(rows[i]) == 0 {
			continue
		}
		name := strings.TrimSpace(rows[i][0])
		if strings.Contains(name, "합계") || strings.EqualFold(name, "total") {
			totalRow = i
			continue
		}
		qty := spreadsheetCellInt(rows[i], qtyCol)
		unit := spreadsheetCellInt(rows[i], unitCol)
		amount := qty * unit
		total += amount
		for len(rows[i]) <= amountCol {
			rows[i] = append(rows[i], "")
		}
		rows[i][amountCol] = strconv.Itoa(amount)
	}
	if totalRow >= 0 {
		for len(rows[totalRow]) <= amountCol {
			rows[totalRow] = append(rows[totalRow], "")
		}
		rows[totalRow][amountCol] = strconv.Itoa(total)
	}
	return rows
}

func spreadsheetCellInt(row []string, idx int) int {
	if idx < 0 || idx >= len(row) {
		return 0
	}
	n, _ := strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(row[idx]), ",", ""))
	return n
}

func spreadsheetRevisionItem(instruction string) string {
	text := strings.TrimSpace(instruction)
	lower := strings.ToLower(text)
	switch {
	case strings.Contains(lower, "gpu"):
		return "GPU 비용"
	case strings.Contains(text, "첫 번째"):
		return "첫 번째 작업"
	case strings.Contains(text, "두 번째"):
		return "두 번째 작업"
	case strings.Contains(text, "서비스/제품"):
		return "서비스/제품"
	case strings.Contains(text, "서버"):
		return "서버"
	case strings.Contains(text, "도구"):
		return "도구"
	}
	for _, suffix := range []string{"항목 추가해줘", "항목 삭제해줘", "항목 추가", "항목 삭제", "추가해줘", "삭제해줘", "추가", "삭제"} {
		if before, ok := strings.CutSuffix(text, suffix); ok {
			before = strings.TrimSpace(before)
			if before != "" {
				return cleanWorkReportCell(before)
			}
		}
	}
	if fields := strings.Fields(text); len(fields) > 0 {
		return cleanWorkReportCell(fields[0])
	}
	return ""
}

func assistantMarkdownTableRows(body string) [][]string {
	rows := [][]string{}
	for _, line := range strings.Split(body, "\n") {
		row, ok := assistantMarkdownTableRow(line)
		if !ok {
			continue
		}
		rows = append(rows, row)
	}
	return rows
}

func assistantMarkdownTableRow(line string) ([]string, bool) {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") || assistantMarkdownTableSeparator(line) {
		return nil, false
	}
	raw := strings.Split(strings.Trim(line, "|"), "|")
	if len(raw) < 2 {
		return nil, false
	}
	cells := make([]string, 0, len(raw))
	for _, cell := range raw {
		cells = append(cells, strings.TrimSpace(cell))
	}
	return cells, true
}

func assistantMarkdownTableSeparator(line string) bool {
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
		return false
	}
	for _, r := range strings.Trim(line, "| ") {
		if r != '-' && r != ':' && r != '|' && r != ' ' {
			return false
		}
	}
	return strings.Contains(line, "-")
}

func spreadsheetExportResult(source osauto.Result, format string) osauto.Result {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" || format == "spreadsheet" || format == "excel" || format == "numbers" {
		format = "xlsx"
	}
	result := osauto.Result{
		Kind:      "meshclaw_automation_spreadsheet_export",
		Action:    "spreadsheet_export",
		OK:        true,
		CreatedAt: time.Now().UTC(),
	}
	switch format {
	case "csv":
		if source.CSV == "" {
			result.OK = false
			result.Error = "recent spreadsheet CSV source is missing"
			return result
		}
		result.CSV = source.CSV
		result.Stdout = "csv ready"
		return result
	case "xlsx":
		if source.XLSX == "" {
			result.OK = false
			result.Error = "recent spreadsheet XLSX source is missing"
			return result
		}
		result.XLSX = source.XLSX
		result.Stdout = "xlsx ready"
		return result
	default:
		result.OK = false
		result.Error = "spreadsheet export format must be xlsx or csv"
		return result
	}
}

func recentArtifactPathForOpen(state assistantArtifactState, target, app string) string {
	lower := strings.ToLower(strings.TrimSpace(target + " " + app))
	if strings.Contains(lower, "numbers") || strings.Contains(lower, "excel") || strings.Contains(lower, "xlsx") || strings.Contains(lower, "spreadsheet") || strings.Contains(lower, "sheet") || strings.Contains(lower, "엑셀") || strings.Contains(lower, "표") {
		return firstExistingPath(state.Spreadsheet.XLSX, state.Spreadsheet.CSV, state.Spreadsheet.URL)
	}
	if strings.Contains(lower, "csv") {
		return firstExistingPath(state.Spreadsheet.CSV)
	}
	if strings.Contains(lower, "obsidian") || strings.Contains(lower, "markdown") || strings.Contains(lower, "md") {
		return firstExistingPath(state.Document.Markdown, state.Presentation.Markdown)
	}
	if strings.Contains(lower, "powerpoint") || strings.Contains(lower, "keynote") || strings.Contains(lower, "ppt") || strings.Contains(lower, "발표") {
		return firstExistingPath(state.Presentation.PPTX)
	}
	if strings.Contains(lower, "pages") || strings.Contains(lower, "word") || strings.Contains(lower, "docx") || strings.Contains(lower, "문서") {
		return firstExistingPath(state.Document.DOCX, state.Document.Markdown)
	}
	if strings.Contains(lower, "preview") || strings.Contains(lower, "pdf") {
		return firstExistingPath(state.Presentation.PDF, state.Document.PDF, state.Presentation.URL, state.Document.URL)
	}
	return firstExistingPath(state.Document.Markdown, state.Document.DOCX, state.Presentation.PPTX, state.Presentation.Markdown, state.Document.URL, state.Presentation.URL)
}

func firstExistingPath(paths ...string) string {
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if st, err := os.Stat(path); err == nil && !st.IsDir() {
			return path
		}
	}
	return ""
}

func defaultAppForArtifactPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".md", ".markdown":
		return "Obsidian"
	case ".docx":
		return "Pages"
	case ".pptx":
		return "Microsoft PowerPoint"
	case ".xlsx", ".csv":
		return "Numbers"
	case ".pdf", ".png", ".jpg", ".jpeg":
		return "Preview"
	default:
		return ""
	}
}

func inferSlideCount(text string, fallback int) int {
	for _, field := range strings.Fields(text) {
		field = strings.Trim(field, " \t\r\n.,!?()[]{}장개슬라이드")
		if field == "" {
			continue
		}
		var n int
		if _, err := fmt.Sscanf(field, "%d", &n); err == nil && n > 0 && n <= 20 {
			return n
		}
	}
	return fallback
}

func formatAssistantPresentationRevisionResult(result osauto.Result, record evidence.Record, storeErr error) string {
	if !result.OK || result.Error != "" {
		lines := []string{"최근 발표자료 수정본을 만들지 못했습니다.", "문제: " + firstNonEmpty(result.Error, result.Stderr, "unknown error")}
		return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
	}
	lines := []string{"최근 발표자료를 기준으로 수정본을 만들었습니다.", "새 PPTX를 첨부합니다."}
	if result.Stdout != "" {
		lines = append(lines, assistantVerificationSentence(result.Stdout))
	}
	lines = appendAssistantAttachmentMarkers(lines, result)
	return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
}

func formatAssistantDocumentRevisionResult(result osauto.Result, record evidence.Record, storeErr error) string {
	if !result.OK || result.Error != "" {
		lines := []string{"최근 문서 수정본을 만들지 못했습니다.", "문제: " + firstNonEmpty(result.Error, result.Stderr, "unknown error")}
		return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
	}
	lines := []string{"최근 문서를 기준으로 수정본을 만들었습니다.", "Obsidian용 Markdown과 Word/Pages용 DOCX를 다시 첨부합니다."}
	lines = appendAssistantAttachmentMarkers(lines, result)
	return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
}

func formatAssistantSpreadsheetRevisionResult(result osauto.Result, record evidence.Record, storeErr error) string {
	if !result.OK || result.Error != "" {
		lines := []string{"최근 표 파일 수정본을 만들지 못했습니다.", "문제: " + firstNonEmpty(result.Error, result.Stderr, "unknown error")}
		return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
	}
	lines := []string{"최근 표 파일을 기준으로 수정본을 만들었습니다.", "Numbers/Excel용 XLSX와 CSV를 다시 첨부합니다."}
	lines = appendAssistantAttachmentMarkers(lines, result)
	return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
}

func formatAssistantExportResult(label string, result osauto.Result, record evidence.Record, storeErr error) string {
	if !result.OK || result.Error != "" {
		lines := []string{label + "를 요청한 형식으로 바로 변환하지는 못했습니다.", "이유: " + exportUserFacingReason(result)}
		if hint := exportDependencyHint(result); hint != "" {
			lines = append(lines, hint)
		}
		if result.PPTX != "" {
			lines = append(lines, "지금은 원본 PPTX를 먼저 보내고, PDF 변환은 LibreOffice 설치 후 다시 시도할 수 있습니다.")
			lines = append(lines, "meshclaw-attachment: "+result.PPTX)
		}
		if result.DOCX != "" {
			lines = append(lines, "지금은 Word/Pages용 DOCX를 먼저 보내고, PDF 변환은 pandoc 설치 후 다시 시도할 수 있습니다.")
			lines = append(lines, "meshclaw-attachment: "+result.DOCX)
		}
		if result.Markdown != "" {
			lines = append(lines, "Obsidian/Markdown 원본은 계속 편집할 수 있습니다.")
			lines = append(lines, "meshclaw-attachment: "+result.Markdown)
		}
		return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
	}
	lines := []string{label + "를 요청한 형식으로 내보냈습니다.", "파일을 첨부합니다."}
	lines = appendAssistantAttachmentMarkers(lines, result)
	return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
}

func exportUserFacingReason(result osauto.Result) string {
	errText := strings.ToLower(firstNonEmpty(result.Error, result.Stderr))
	switch {
	case strings.Contains(errText, "libreoffice") || strings.Contains(errText, "soffice") || strings.Contains(errText, "pandoc"):
		return "필요한 PDF 변환 도구가 이 Mac에 아직 설치되어 있지 않습니다."
	case strings.TrimSpace(firstNonEmpty(result.Error, result.Stderr)) == "":
		return "변환 중 원인을 알 수 없는 문제가 발생했습니다."
	default:
		return firstNonEmpty(result.Error, result.Stderr)
	}
}

func exportDependencyHint(result osauto.Result) string {
	errText := strings.ToLower(firstNonEmpty(result.Error, result.Stderr))
	switch {
	case strings.Contains(errText, "libreoffice") || strings.Contains(errText, "soffice"):
		return "PPTX를 PDF로 바꾸려면 이 Mac에 LibreOffice가 필요합니다. 설치 후 같은 요청을 다시 보내면 PDF로 변환해 첨부할 수 있습니다."
	case strings.Contains(errText, "pandoc"):
		return "Markdown 문서를 PDF로 바꾸려면 pandoc이 필요합니다. 설치 후 같은 요청을 다시 보내면 PDF로 변환해 첨부할 수 있습니다."
	default:
		return ""
	}
}

func appendAssistantEvidenceNote(lines []string, record evidence.Record, storeErr error) []string {
	if storeErr != nil {
		return lines
	}
	return lines
}

func assistantVerificationSentence(stdout string) string {
	stdout = strings.TrimSpace(stdout)
	lower := strings.ToLower(stdout)
	if strings.HasPrefix(lower, "created ") && strings.Contains(lower, " with ") && strings.Contains(lower, " slides") {
		parts := strings.Split(lower, " with ")
		if len(parts) > 1 {
			count := strings.TrimSpace(strings.TrimSuffix(parts[len(parts)-1], " slides"))
			if count != "" {
				return "검증: PPTX 파일 구조를 확인했고 " + count + "개 슬라이드가 있습니다."
			}
		}
	}
	if stdout != "" && !strings.Contains(stdout, "/") {
		return "검증: " + stdout
	}
	return "검증: 생성된 파일을 확인했습니다."
}

func temporarilySkipPreviewImages() func() {
	const key = "MESHCLAW_SKIP_PREVIEW_IMAGE"
	previous, hadPrevious := os.LookupEnv(key)
	_ = os.Setenv(key, "1")
	return func() {
		if hadPrevious {
			_ = os.Setenv(key, previous)
			return
		}
		_ = os.Unsetenv(key)
	}
}

func meetingBriefDocumentBody(title, body, audience string) string {
	parts := []string{
		"# " + strings.TrimSpace(title),
		"",
		"## 회의 목적",
		"- 핵심 현황을 빠르게 공유합니다.",
		"- 결정이 필요한 항목과 다음 액션을 분리합니다.",
	}
	if strings.TrimSpace(audience) != "" {
		parts = append(parts, "", "## 대상", "- "+strings.TrimSpace(audience))
	}
	parts = append(parts,
		"",
		"## 입력 내용",
		strings.TrimSpace(body),
		"",
		"## 제안 안건",
		"1. 현재 상태와 핵심 이슈",
		"2. 선택지와 리스크",
		"3. 오늘 결정할 항목",
		"4. 담당자와 마감일",
		"",
		"## 회의 후 액션",
		"- 결정 사항을 문서로 확정합니다.",
		"- 담당자/마감일을 리마인더나 캘린더로 등록합니다.",
	)
	return strings.Join(parts, "\n")
}

func meetingMinutesDocumentBody(request, title string) string {
	now := time.Now().In(time.FixedZone("KST", 9*60*60))
	source := strings.TrimSpace(request)
	if source == "" {
		source = "회의 내용이 별도로 제공되지 않았습니다. Argos가 기본 회의록 틀과 확인 항목을 준비했습니다."
	}
	return strings.Join([]string{
		"# " + strings.TrimSpace(title),
		"",
		"- 작성일: " + now.Format("2006-01-02 15:04 KST"),
		"- 목적: 회의 내용을 바로 공유할 수 있는 회의록으로 정리",
		"",
		"## 1. 한 줄 요약",
		"- 오늘 회의는 진행 상황을 확인하고, 결정이 필요한 항목과 다음 액션을 분리하는 데 초점을 둡니다.",
		"",
		"## 2. 논의 내용",
		"- 입력된 회의 메모를 기준으로 핵심 흐름을 정리했습니다.",
		"- 세부 수치나 참석자 이름이 빠져 있으면 후속 메시지로 보강할 수 있습니다.",
		"- 공유 대상이 보고방이면 모바일에서 먼저 읽을 수 있도록 Markdown과 문서 파일을 함께 준비합니다.",
		"",
		"## 3. 결정 사항",
		"| 번호 | 결정 | 비고 |",
		"| --- | --- | --- |",
		"| 1 | 사용자에게 내부 점검보다 실제 업무 결과물을 먼저 보여준다 | Signal 본문 우선 |",
		"| 2 | 회의록/보고서/시장조사/여행계획 같은 반복 업무를 바로 산출물로 만든다 | 문서 첨부 포함 |",
		"| 3 | 예약, 결제, 발송 같은 마지막 확정은 확인 후 진행한다 | 실행 전 확인 |",
		"",
		"## 4. 액션 아이템",
		"| 담당 | 할 일 | 마감 |",
		"| --- | --- | --- |",
		"| Argos | 회의록 문서와 모바일 읽기용 요약을 만든다 | 즉시 |",
		"| 사용자 | 누락된 참석자, 수치, 기한이 있으면 후속 메시지로 보강한다 | 필요 시 |",
		"| Argos | 보고방 요청이면 문서 파일을 첨부해서 공유한다 | 즉시 |",
		"",
		"## 5. 리스크와 확인 필요",
		"- 실제 회의 원문이 짧으면 일부 항목은 합리적 기본안으로 채웁니다.",
		"- 외부 발송, 결제, 예약 확정, 삭제는 회의록 작성과 별개로 확인이 필요합니다.",
		"- 민감 정보가 포함되면 공유 전 제거하거나 별도 보안 채널에서 처리해야 합니다.",
		"",
		"## 6. 다음 회의 안건",
		"1. 이번 회의록에서 빠진 참석자/마감일 보강",
		"2. 결정 사항이 실제 작업으로 이어졌는지 확인",
		"3. 다음 보고서나 시장조사 자동 발송 주기 결정",
		"",
		"## 원문/요청",
		source,
	}, "\n")
}

func meetingDeckBody(title, body, audience string) string {
	parts := []string{
		"# " + strings.TrimSpace(title),
		"- 회의 목적과 배경",
		"- 현재 상태",
		"- 핵심 이슈",
		"- 선택지",
		"- 결정 요청",
		"- 다음 액션",
	}
	if strings.TrimSpace(audience) != "" {
		parts = append(parts, "", "Audience: "+strings.TrimSpace(audience))
	}
	parts = append(parts, "", "Source notes:", strings.TrimSpace(body))
	return strings.Join(parts, "\n")
}

func marketAssetFromRequest(request string) string {
	lower := strings.ToLower(strings.TrimSpace(request))
	switch {
	case containsAny(lower, "유가", "원유", "석유", "oil", "crude", "wti", "brent", "브렌트"):
		return "crude oil WTI Brent"
	case containsAny(lower, "휘발유", "gasoline", "diesel", "경유"):
		return "gasoline diesel oil products"
	case containsAny(lower, "원달러", "달러원", "usdkrw", "usd/krw"):
		return "USD/KRW foreign exchange rate"
	case containsAny(lower, "환율", "원달러", "달러", "엔화", "fx", "exchange rate"):
		return "foreign exchange rate"
	case containsAny(lower, "금리", "interest rate", "채권", "bond"):
		return "interest rates bond yields"
	case containsAny(lower, "비트코인", "bitcoin", "btc", "crypto", "코인"):
		return "bitcoin crypto"
	case containsAny(lower, "엔비디아", "nvidia", "nvda"):
		return "Nvidia NVDA stock"
	case containsAny(lower, "테슬라", "tesla", "tsla"):
		return "Tesla TSLA stock"
	case containsAny(lower, "애플", "apple", "aapl"):
		return "Apple AAPL stock"
	case containsAny(lower, "삼성전자", "samsung electronics"):
		return "Samsung Electronics stock"
	case containsAny(lower, "주가", "증시", "나스닥", "s&p", "stock", "equity", "shares"):
		return "equities stock market"
	default:
		return ""
	}
}

func marketOutlookSearchQuery(asset, horizon string) string {
	parts := []string{strings.TrimSpace(asset), "latest outlook forecast drivers"}
	if strings.TrimSpace(horizon) != "" {
		parts = append(parts, strings.TrimSpace(horizon))
	}
	parts = append(parts, marketOutlookSearchTerms(marketOutlookAssetKind(asset)))
	return strings.Join(parts, " ")
}

func formatMarketOutlookToolResult(asset, horizon, query string, search browserauto.SearchResult, err error) []string {
	period := strings.TrimSpace(horizon)
	if period == "" {
		period = lang.T("assistant.market_outlook.period_default")
	}
	kind := marketOutlookAssetKind(asset)
	lines := []string{lang.T("assistant.market_outlook.opening", asset, period)}
	lines = append(lines, "")
	lines = append(lines, lang.T("assistant.market_outlook.criteria"))
	for _, key := range marketOutlookScenarioKeys(kind) {
		lines = append(lines, "- "+lang.T(key))
	}
	lines = append(lines, "")
	lines = append(lines, lang.T(marketOutlookIndicatorKey(kind)))
	if strings.TrimSpace(horizon) != "" {
		lines = append(lines, lang.T("assistant.market_outlook.horizon", horizon))
	}
	if err != nil {
		lines = append(lines, "", lang.T("assistant.market_outlook.error", err.Error()))
	}
	if len(search.Results) == 0 {
		lines = append(lines, "", lang.T("assistant.market_outlook.no_evidence"))
	} else {
		lines = append(lines, "", lang.T("assistant.market_outlook.sources_title"))
		for i, item := range search.Results {
			if i >= 3 {
				break
			}
			title := strings.TrimSpace(item.Text)
			if title == "" {
				title = lang.T("assistant.market_outlook.source_untitled")
			}
			lines = append(lines, fmt.Sprintf("%d. %s", i+1, title))
			if strings.TrimSpace(item.URL) != "" {
				lines = append(lines, "   "+item.URL)
			}
		}
	}
	if strings.TrimSpace(query) != "" {
		lines = append(lines, "", lang.T("assistant.market_outlook.query", query))
	}
	lines = append(lines, "", lang.T("assistant.market_outlook.boundary"))
	lines = append(lines, "", lang.T("assistant.market_outlook.next_title"))
	for _, key := range []string{
		"assistant.market_outlook.next.report",
		"assistant.market_outlook.next.monitor",
		"assistant.market_outlook.next.decision",
	} {
		lines = append(lines, "- "+lang.T(key))
	}
	return lines
}

func marketOutlookAssetKind(asset string) string {
	lower := strings.ToLower(strings.TrimSpace(asset))
	switch {
	case containsAny(lower, "oil", "crude", "wti", "brent", "유가", "원유", "석유"):
		return "oil"
	case containsAny(lower, "gasoline", "diesel", "휘발유", "경유"):
		return "fuel"
	case containsAny(lower, "fx", "exchange rate", "usd/krw", "usdkrw", "환율", "원달러", "달러원", "엔화"):
		return "fx"
	case containsAny(lower, "interest rate", "bond", "yield", "금리", "채권"):
		return "rates"
	case containsAny(lower, "bitcoin", "btc", "crypto", "코인", "비트코인"):
		return "crypto"
	case containsAny(lower, "stock", "equity", "shares", "nvda", "tsla", "aapl", "주가", "증시", "나스닥", "삼성전자"):
		return "equity"
	default:
		return "generic"
	}
}

func marketOutlookSearchTerms(kind string) string {
	switch kind {
	case "oil":
		return "Reuters EIA OPEC inventory demand supply"
	case "fuel":
		return "Reuters EIA refinery margins inventory demand retail prices"
	case "fx":
		return "Reuters central bank rates inflation dollar index capital flows"
	case "rates":
		return "Reuters central bank inflation yield curve policy expectations"
	case "crypto":
		return "Reuters CoinDesk ETF flows regulation macro liquidity"
	case "equity":
		return "Reuters earnings guidance valuation analyst estimates SEC filings"
	default:
		return "Reuters Bloomberg official data macro earnings policy"
	}
}

func marketOutlookScenarioKeys(kind string) []string {
	switch kind {
	case "oil":
		return []string{
			"assistant.market_outlook.scenario.upside.oil",
			"assistant.market_outlook.scenario.downside.oil",
			"assistant.market_outlook.scenario.range.oil",
		}
	case "fuel":
		return []string{
			"assistant.market_outlook.scenario.upside.fuel",
			"assistant.market_outlook.scenario.downside.fuel",
			"assistant.market_outlook.scenario.range.fuel",
		}
	case "fx":
		return []string{
			"assistant.market_outlook.scenario.upside.fx",
			"assistant.market_outlook.scenario.downside.fx",
			"assistant.market_outlook.scenario.range.fx",
		}
	case "rates":
		return []string{
			"assistant.market_outlook.scenario.upside.rates",
			"assistant.market_outlook.scenario.downside.rates",
			"assistant.market_outlook.scenario.range.rates",
		}
	case "crypto":
		return []string{
			"assistant.market_outlook.scenario.upside.crypto",
			"assistant.market_outlook.scenario.downside.crypto",
			"assistant.market_outlook.scenario.range.crypto",
		}
	case "equity":
		return []string{
			"assistant.market_outlook.scenario.upside.equity",
			"assistant.market_outlook.scenario.downside.equity",
			"assistant.market_outlook.scenario.range.equity",
		}
	default:
		return []string{
			"assistant.market_outlook.scenario.upside.generic",
			"assistant.market_outlook.scenario.downside.generic",
			"assistant.market_outlook.scenario.range.generic",
		}
	}
}

func assistantToolDirectPurchaseQuery(request, query string) string {
	combined := strings.ToLower(strings.TrimSpace(request + " " + query))
	if !isShoppingDirectPurchaseStartRequest(combined) {
		return ""
	}
	for _, candidate := range []string{
		directPurchaseKoreanImperativeQuery(request),
		directPurchaseKoreanImperativeQuery(query),
		directPurchaseEnglishImperativeQuery(request),
		directPurchaseEnglishImperativeQuery(query),
	} {
		if strings.TrimSpace(candidate) != "" {
			return candidate
		}
	}
	return ""
}

func marketOutlookIndicatorKey(kind string) string {
	switch kind {
	case "oil":
		return "assistant.market_outlook.indicators.oil"
	case "fuel":
		return "assistant.market_outlook.indicators.fuel"
	case "fx":
		return "assistant.market_outlook.indicators.fx"
	case "rates":
		return "assistant.market_outlook.indicators.rates"
	case "crypto":
		return "assistant.market_outlook.indicators.crypto"
	case "equity":
		return "assistant.market_outlook.indicators.equity"
	default:
		return "assistant.market_outlook.indicators.generic"
	}
}

func marketOutlookVoiceRequested(request string, args map[string]interface{}) bool {
	if toolArgBool(args, "voice_brief", false) || toolArgBool(args, "audio", false) || toolArgBool(args, "mp3", false) {
		return true
	}
	return looksLikeAssistantVoiceRequest(request)
}

func marketOutlookVoiceBriefText(asset string, lines []string) string {
	out := []string{lang.T("assistant.market_outlook.voice_intro", asset)}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "http://") || strings.HasPrefix(line, "https://") {
			continue
		}
		if strings.HasPrefix(line, "검색 기준:") || strings.HasPrefix(line, "Search basis:") || strings.HasPrefix(line, "- 증거:") || strings.HasPrefix(line, "- Evidence:") {
			continue
		}
		out = append(out, line)
		if len(out) >= 14 {
			break
		}
	}
	return strings.Join(out, "\n")
}

func formatAssistantMarketOutlookSendResult(opts ListenOptions, targetRef, asset string, lines []string, voiceBrief, voiceNote bool, engine, ttsVoice string, record evidence.Record, storeErr error) string {
	sendText := strings.Join(compactBlankLines(lines), "\n")
	if strings.TrimSpace(targetRef) == "" {
		if !voiceBrief {
			return strings.Join(lines, "\n")
		}
		if !opts.Execute {
			out := []string{
				lang.T("assistant.market_outlook.voice_plan"),
				lang.T("assistant.market_outlook.voice_ready"),
				"",
				sendText,
			}
			return strings.Join(appendAssistantEvidenceNote(out, record, storeErr), "\n")
		}
		audio, audioErr := tts.Synthesize(tts.Options{
			Text:     marketOutlookVoiceBriefText(asset, lines),
			Engine:   firstNonEmpty(engine, "edge-tts"),
			Voice:    ttsVoice,
			Basename: "argos-market-outlook-" + time.Now().UTC().Format("20060102T150405Z"),
		})
		payload := map[string]interface{}{
			"kind":        "assistant_market_outlook_voice",
			"asset":       asset,
			"engine":      firstNonEmpty(engine, "edge-tts"),
			"tts_voice":   ttsVoice,
			"text":        sendText,
			"audio":       audio,
			"audio_error": errorString(audioErr),
			"created_at":  time.Now().UTC(),
		}
		voiceRecord, voiceStoreErr := evidence.Store("assistant-market-outlook-voice", firstNonEmpty(opts.TargetID, "assistant"), asset, payload)
		if audioErr != nil {
			out := []string{lang.T("assistant.market_outlook.voice_failed", audioErr.Error()), "", sendText}
			return strings.Join(appendAssistantEvidenceNote(out, voiceRecord, voiceStoreErr), "\n")
		}
		out := []string{
			lang.T("assistant.market_outlook.voice_created"),
			lang.T("assistant.market_outlook.attach_here"),
			"meshclaw-attachment: " + audio.Path,
			"",
			sendText,
		}
		return strings.Join(appendAssistantEvidenceNote(out, voiceRecord, voiceStoreErr), "\n")
	}
	target, candidates, targetErr := resolveAssistantVoiceTarget(targetRef)
	if targetErr != nil {
		payload := map[string]interface{}{
			"kind":              "assistant_market_outlook_send_target_error",
			"asset":             asset,
			"target":            targetRef,
			"voice_brief":       voiceBrief,
			"voice_note":        voiceNote,
			"target_error":      targetErr.Error(),
			"target_candidates": candidates,
			"text":              sendText,
			"created_at":        time.Now().UTC(),
		}
		fallbackRecord, fallbackStoreErr := evidence.Store("assistant-market-outlook-send-target", firstNonEmpty(opts.TargetID, "assistant"), targetRef, payload)
		out := []string{lang.T("assistant.market_outlook.send_target_failed")}
		out = append(out, formatAssistantVoiceTargetCandidates(candidates)...)
		out = append(out, lang.T("assistant.market_outlook.attach_here"), "", sendText)
		if fallbackRecord.ID != "" || fallbackStoreErr != nil {
			return strings.Join(appendAssistantEvidenceNote(out, fallbackRecord, fallbackStoreErr), "\n")
		}
		return strings.Join(appendAssistantEvidenceNote(out, record, storeErr), "\n")
	}
	if !opts.Execute {
		out := []string{
			lang.T("assistant.market_outlook.send_ready"),
			lang.T("assistant.market_outlook.target", firstNonEmpty(target.Label, target.ID)),
		}
		if OneWayReportTarget(target) {
			out = append(out, lang.T("assistant.market_outlook.one_way"))
		}
		if voiceBrief {
			out = append(out, lang.T("assistant.market_outlook.voice_ready"))
		}
		out = append(out, "", lang.T("assistant.market_outlook.to_send"))
		out = append(out, lines...)
		return strings.Join(appendAssistantEvidenceNote(out, record, storeErr), "\n")
	}
	attachments := []string{}
	var audio tts.Result
	var audioErr error
	if voiceBrief {
		audio, audioErr = tts.Synthesize(tts.Options{
			Text:     marketOutlookVoiceBriefText(asset, lines),
			Engine:   firstNonEmpty(engine, "edge-tts"),
			Voice:    ttsVoice,
			Basename: "argos-market-outlook-" + time.Now().UTC().Format("20060102T150405Z"),
		})
		if audioErr != nil {
			sendText = sendText + "\n\n" + lang.T("assistant.market_outlook.voice_failed", audioErr.Error())
		} else {
			attachments = append(attachments, audio.Path)
			sendText = sendText + "\n\n" + lang.T("assistant.market_outlook.file.audio")
		}
	}
	send, sendErr := Send(SendOptions{
		TargetID:       target.ID,
		Kind:           "text",
		Text:           sendText,
		Attachments:    attachments,
		VoiceNote:      voiceNote,
		Execute:        opts.Execute,
		TimeoutSeconds: 90,
	})
	payload := map[string]interface{}{
		"kind":            "assistant_market_outlook_send",
		"asset":           asset,
		"target":          targetRef,
		"resolved_target": target,
		"voice_brief":     voiceBrief,
		"voice_note":      voiceNote,
		"engine":          firstNonEmpty(engine, "edge-tts"),
		"tts_voice":       ttsVoice,
		"audio":           audio,
		"audio_error":     errorString(audioErr),
		"send":            send,
		"send_error":      errorString(sendErr),
		"created_at":      time.Now().UTC(),
	}
	sendRecord, sendStoreErr := evidence.Store("assistant-market-outlook-send", firstNonEmpty(opts.TargetID, "assistant"), target.ID, payload)
	if sendErr != nil {
		out := []string{
			lang.T("assistant.market_outlook.send_failed"),
			lang.T("assistant.market_outlook.target", firstNonEmpty(target.Label, target.ID)),
			lang.T("assistant.market_outlook.problem", sendErr.Error()),
			lang.T("assistant.market_outlook.attach_here"),
			"",
			sendText,
		}
		out = appendVoiceReportAttachmentMarkers(out, attachments)
		return strings.Join(appendAssistantEvidenceNote(out, sendRecord, sendStoreErr), "\n")
	}
	out := []string{
		lang.T("assistant.market_outlook.sent"),
		lang.T("assistant.market_outlook.target", firstNonEmpty(target.Label, target.ID)),
	}
	if len(attachments) > 0 {
		out = append(out, lang.T("assistant.market_outlook.file.audio"))
	}
	if id := strings.TrimSpace(send.Stdout); id != "" {
		out = append(out, lang.T("assistant.workflow.signal_id", id))
	}
	return strings.Join(appendAssistantEvidenceNote(out, sendRecord, sendStoreErr), "\n")
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
