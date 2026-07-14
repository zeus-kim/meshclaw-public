package messenger

import (
	"context"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/meshclaw/meshclaw/internal/evidence"
	"github.com/meshclaw/meshclaw/internal/lang"
	"github.com/meshclaw/meshclaw/internal/osauto"
	"github.com/meshclaw/meshclaw/internal/tts"
)

func assistantMeetingMinutesPackageReply(opts ListenOptions, request string) (string, bool) {
	if !isAssistantMeetingMinutesPackageRequest(request) {
		return "", false
	}
	title := assistantMeetingMinutesPackageTitle(request)
	notes := assistantMeetingMinutesSourceNotes(request)
	body := assistantMeetingMinutesPackageReportBody(title, notes, request)

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	restorePreview := temporarilySkipPreviewImages()
	doc := osauto.CreateArgosDocument(ctx, lang.T("assistant.meeting_minutes_package.artifact.doc_title", title), body)
	deck := osauto.CreatePresentation(ctx, lang.T("assistant.meeting_minutes_package.artifact.deck_title", title), assistantMeetingMinutesPackageDeckBody(title, notes), lang.T("assistant.meeting_minutes_package.artifact.audience"), 7, "")
	sheet := osauto.CreateSpreadsheet(ctx, lang.T("assistant.meeting_minutes_package.artifact.sheet_title", title), assistantMeetingMinutesPackageSheetBody(notes))
	graphPath, graphErr := writeAssistantMeetingMinutesActionFlowMarkdown(title, notes)
	boardPath, boardErr := writeAssistantMeetingMinutesActionBoardSVG(title, notes)
	restorePreview()

	wantsVoice := assistantBrowserResearchPackageWantsVoice(request)
	voiceScript := ""
	var audio tts.Result
	var audioErr error
	if wantsVoice {
		voiceScript = assistantMeetingMinutesPackageVoiceScript(title, notes)
		audio, audioErr = tts.Synthesize(tts.Options{
			Text:     voiceScript,
			Engine:   "edge-tts",
			Basename: "meeting-minutes-brief-" + time.Now().UTC().Format("20060102T150405Z"),
		})
	}

	attachments := assistantMeetingMinutesPackageAttachments(doc, deck, sheet)
	if strings.TrimSpace(graphPath) != "" && graphErr == nil {
		attachments = uniqueShowcaseAttachments(append(attachments, graphPath))
	}
	if strings.TrimSpace(boardPath) != "" && boardErr == nil {
		attachments = uniqueShowcaseAttachments(append(attachments, boardPath))
	}
	if strings.TrimSpace(audio.Path) != "" && audioErr == nil {
		attachments = uniqueShowcaseAttachments(append(attachments, audio.Path))
	}
	rememberAssistantArtifact(opts, title, "meeting_minutes_package", doc, deck, sheet)
	record, storeErr := evidence.Store("assistant-meeting-minutes-package", firstNonEmpty(opts.TargetID, "assistant"), title, map[string]interface{}{
		"kind":         "assistant_meeting_minutes_package",
		"request":      request,
		"title":        title,
		"notes":        notes,
		"voice_script": voiceScript,
		"audio":        audio,
		"audio_error":  errorString(audioErr),
		"graph":        graphPath,
		"graph_error":  errorString(graphErr),
		"board":        boardPath,
		"board_error":  errorString(boardErr),
		"document":     doc,
		"presentation": deck,
		"spreadsheet":  sheet,
		"attachments":  attachments,
		"created_at":   time.Now().UTC(),
	})

	summary := assistantMeetingMinutesPackageSummary(title, notes, doc, deck, sheet, graphPath, graphErr, boardPath, boardErr, wantsVoice, audio, audioErr)
	if targetRef := inferAssistantSignalTargetRef(request); targetRef != "" {
		return formatAssistantMeetingMinutesPackageSendResult(opts, targetRef, summary, attachments, record, storeErr), true
	}
	lines := append([]string{}, summary...)
	lines = appendVoiceReportAttachmentMarkers(lines, attachments)
	lines = appendAssistantEvidenceNote(lines, record, storeErr)
	return strings.Join(compactBlankLines(lines), "\n"), true
}

func isAssistantMeetingMinutesPackageRequest(request string) bool {
	lower := strings.ToLower(strings.TrimSpace(request))
	if !shouldRouteAsMeetingMinutes(lower) {
		return false
	}
	if looksLikeMeetingMaterialsRequest(lower) && !looksLikeStructuredMeetingMinutesSource(lower) {
		return false
	}
	return containsAny(lower,
		"보고방", "보내", "전송", "공유", "첨부", "패키지",
		"ppt", "pptx", "발표자료", "슬라이드", "표", "엑셀", "xlsx", "csv",
		"음성", "음성파일", "tts", "mp3", "뉴스방송", "브리핑 원고",
		"briefing", "send", "share", "deck", "slides", "spreadsheet", "voice",
	)
}

func looksLikeStructuredMeetingMinutesSource(lower string) bool {
	lower = strings.ToLower(strings.TrimSpace(lower))
	if lower == "" {
		return false
	}
	return containsAny(lower, "결정:", "결정：", "결정사항", "결정 사항", "decision:", "decisions:") &&
		containsAny(lower, "할 일:", "할일:", "액션:", "액션아이템", "action:", "action item") &&
		containsAny(lower, "리스크:", "위험:", "risk:")
}

func assistantMeetingMinutesPackageTitle(request string) string {
	lower := strings.ToLower(request)
	switch {
	case containsAny(lower, "제품", "product"):
		return lang.T("assistant.meeting_minutes_package.title.product")
	case containsAny(lower, "매출", "영업", "sales", "revenue"):
		return lang.T("assistant.meeting_minutes_package.title.sales")
	case containsAny(lower, "개발", "dev", "engineering"):
		return lang.T("assistant.meeting_minutes_package.title.engineering")
	default:
		return lang.T("assistant.meeting_minutes_package.title.default")
	}
}

func assistantMeetingMinutesSourceNotes(request string) string {
	text := strings.TrimSpace(request)
	for _, marker := range []string{"내용:", "내용：", "논의:", "논의：", "회의 메모:", "회의메모:", "메모:", "notes:", "transcript:"} {
		if idx := strings.Index(strings.ToLower(text), strings.ToLower(marker)); idx >= 0 {
			return strings.TrimSpace(text[idx+len(marker):])
		}
	}
	return text
}

func assistantMeetingMinutesPackageSummary(title, notes string, doc, deck, sheet osauto.Result, graphPath string, graphErr error, boardPath string, boardErr error, wantsVoice bool, audio tts.Result, audioErr error) []string {
	lines := []string{
		lang.T("assistant.meeting_minutes_package.created", title),
		lang.T("assistant.meeting_minutes_package.subtitle"),
		lang.T("assistant.meeting_minutes_package.report_label"),
		"",
		lang.T("assistant.meeting_minutes_package.inline_read"),
	}
	lines = append(lines, assistantMeetingMinutesInlineSummary(notes)...)
	if decisions := assistantMeetingMinutesExtractedPlain(notes, []string{"결정:", "결정：", "결정사항:", "결정 사항:", "decision:", "decisions:"}); len(decisions) > 0 {
		lines = append(lines, "", lang.T("assistant.meeting_minutes_package.decision_preview"))
		for i, decision := range decisions {
			if i >= 3 {
				break
			}
			lines = append(lines, "- "+trimForContext(decision, 140))
		}
	}
	if actions := assistantMeetingMinutesActionItems(notes); len(actions) > 0 {
		lines = append(lines, "", lang.T("assistant.meeting_minutes_package.action_preview"))
		for i, action := range actions {
			if i >= 4 {
				break
			}
			lines = append(lines, "- "+assistantMeetingMinutesActionPreview(action))
		}
	}
	if risks := assistantMeetingMinutesExtractedPlain(notes, []string{"리스크:", "위험:", "risk:", "risks:"}); len(risks) > 0 {
		lines = append(lines, "", lang.T("assistant.meeting_minutes_package.risk_preview"))
		for i, risk := range risks {
			if i >= 3 {
				break
			}
			lines = append(lines, "- "+trimForContext(risk, 140))
		}
	}
	lines = append(lines, "", lang.T("assistant.meeting_minutes_package.next_signal_title"))
	lines = append(lines, assistantMeetingMinutesNextSignalActionLines(notes, 2)...)
	lines = append(lines, "", lang.T("assistant.meeting_minutes_package.files"))
	if doc.OK && doc.Error == "" {
		lines = append(lines, "- "+lang.T("assistant.meeting_minutes_package.file.doc"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.meeting_minutes_package.file.doc_failed", firstNonEmpty(doc.Error, doc.Stderr, "unknown error")))
	}
	if deck.OK && deck.Error == "" {
		lines = append(lines, "- "+lang.T("assistant.meeting_minutes_package.file.ppt"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.meeting_minutes_package.file.ppt_failed", firstNonEmpty(deck.Error, deck.Stderr, "unknown error")))
	}
	if sheet.OK && sheet.Error == "" {
		lines = append(lines, "- "+lang.T("assistant.meeting_minutes_package.file.sheet"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.meeting_minutes_package.file.sheet_failed", firstNonEmpty(sheet.Error, sheet.Stderr, "unknown error")))
	}
	if graphErr == nil && strings.TrimSpace(graphPath) != "" {
		lines = append(lines, "- "+lang.T("assistant.meeting_minutes_package.file.graph"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.meeting_minutes_package.file.graph_failed", firstNonEmpty(errorString(graphErr), "unknown error")))
	}
	if boardErr == nil && strings.TrimSpace(boardPath) != "" {
		lines = append(lines, "- "+lang.T("assistant.meeting_minutes_package.file.board"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.meeting_minutes_package.file.board_failed", firstNonEmpty(errorString(boardErr), "unknown error")))
	}
	if wantsVoice {
		if audioErr == nil && strings.TrimSpace(audio.Path) != "" {
			lines = append(lines, "- "+lang.T("assistant.meeting_minutes_package.file.audio"))
		} else {
			lines = append(lines, "- "+lang.T("assistant.meeting_minutes_package.file.audio_failed", firstNonEmpty(errorString(audioErr), "unknown error")))
		}
	}
	lines = append(lines, "", lang.T("assistant.meeting_minutes_package.next"))
	lines = append(lines,
		"- "+lang.T("assistant.meeting_minutes_package.next.exec"),
		"- "+lang.T("assistant.meeting_minutes_package.next.owner"),
		"- "+lang.T("assistant.meeting_minutes_package.next.brief"),
	)
	return lines
}

func formatAssistantMeetingMinutesPackageSendResult(opts ListenOptions, targetRef string, summary []string, attachments []string, record evidence.Record, storeErr error) string {
	target, candidates, targetErr := resolveAssistantVoiceTarget(targetRef)
	if targetErr != nil {
		lines := []string{lang.T("assistant.meeting_minutes_package.send_target_failed")}
		lines = append(lines, formatAssistantVoiceTargetCandidates(candidates)...)
		lines = append(lines, lang.T("assistant.meeting_minutes_package.attach_here"))
		lines = appendVoiceReportAttachmentMarkers(lines, attachments)
		return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
	}
	if !opts.Execute {
		targetLabel := assistantScheduleLocalizedTarget(target.ID, target.Label)
		lines := []string{
			lang.T("assistant.meeting_minutes_package.send_ready"),
			lang.T("assistant.meeting_minutes_package.send_ready_detail"),
			lang.T("assistant.meeting_minutes_package.target", targetLabel),
		}
		if OneWayReportTarget(target) {
			lines = append(lines, lang.T("assistant.meeting_minutes_package.one_way"))
		}
		lines = append(lines, "", lang.T("assistant.meeting_minutes_package.to_send"))
		lines = append(lines, summary...)
		lines = appendVoiceReportAttachmentMarkers(lines, attachments)
		return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
	}
	mobileLinkLines := assistantWorkflowMobileLinkLines(attachments, 6)
	sendSummary := appendAssistantWorkflowMobileLinkLines(summary, mobileLinkLines)
	sendText := strings.Join(compactBlankLines(sendSummary), "\n")
	send, sendErr := Send(SendOptions{
		TargetID:       target.ID,
		Kind:           "text",
		Text:           sendText,
		Attachments:    attachments,
		Execute:        true,
		TimeoutSeconds: 90,
	})
	payload := map[string]interface{}{
		"kind":             "assistant_meeting_minutes_package_send",
		"target":           targetRef,
		"resolved_target":  target,
		"attachment_count": len(attachments),
		"mobile_links":     mobileLinkLines,
		"send":             send,
		"send_error":       errorString(sendErr),
		"created_at":       time.Now().UTC(),
	}
	sendRecord, sendStoreErr := evidence.Store("assistant-meeting-minutes-package-send", firstNonEmpty(opts.TargetID, "assistant"), target.ID, payload)
	if sendErr != nil {
		targetLabel := assistantScheduleLocalizedTarget(target.ID, target.Label)
		lines := []string{
			lang.T("assistant.meeting_minutes_package.send_failed"),
			lang.T("assistant.meeting_minutes_package.target", targetLabel),
			lang.T("assistant.meeting_minutes_package.problem", sendErr.Error()),
			lang.T("assistant.meeting_minutes_package.attach_here"),
		}
		lines = appendAssistantWorkflowMobileLinkLines(lines, mobileLinkLines)
		lines = appendVoiceReportAttachmentMarkers(lines, attachments)
		return strings.Join(appendAssistantEvidenceNote(lines, sendRecord, sendStoreErr), "\n")
	}
	targetLabel := assistantScheduleLocalizedTarget(target.ID, target.Label)
	lines := []string{
		lang.T("assistant.meeting_minutes_package.sent"),
		lang.T("assistant.meeting_minutes_package.target", targetLabel),
	}
	if id := strings.TrimSpace(send.Stdout); id != "" {
		lines = append(lines, lang.T("assistant.workflow.signal_id", id))
	}
	return strings.Join(appendAssistantEvidenceNote(lines, sendRecord, sendStoreErr), "\n")
}

func assistantMeetingMinutesPackageReportBody(title, notes, request string) string {
	now := time.Now().In(time.FixedZone("KST", 9*60*60))
	lines := []string{
		"# " + title,
		"",
		"- " + lang.T("assistant.meeting_minutes_package.body.created_at", now.Format("2006-01-02 15:04 KST")),
		"- " + lang.T("assistant.meeting_minutes_package.body.request", strings.TrimSpace(request)),
		"",
		"## " + lang.T("assistant.meeting_minutes_package.body.summary"),
	}
	lines = append(lines, assistantMeetingMinutesInlineSummary(notes)...)
	lines = append(lines,
		"",
		"## "+lang.T("assistant.meeting_minutes_package.body.decisions"),
	)
	lines = append(lines, assistantMeetingMinutesExtractedLines(notes, []string{"결정:", "결정：", "결정사항:", "결정 사항:", "decision:", "decisions:"}, []string{
		lang.T("assistant.meeting_minutes_package.body.decision_default.1"),
		lang.T("assistant.meeting_minutes_package.body.decision_default.2"),
	})...)
	lines = append(lines,
		"",
		"## "+lang.T("assistant.meeting_minutes_package.body.actions"),
	)
	lines = append(lines, assistantMeetingMinutesExtractedLines(notes, []string{"할 일:", "할일:", "액션:", "액션아이템:", "action:"}, []string{
		lang.T("assistant.meeting_minutes_package.body.action_default.1"),
		lang.T("assistant.meeting_minutes_package.body.action_default.2"),
		lang.T("assistant.meeting_minutes_package.body.action_default.3"),
	})...)
	lines = append(lines,
		"",
		"## "+lang.T("assistant.meeting_minutes_package.body.action_table"),
	)
	for _, action := range assistantMeetingMinutesActionItemsWithFallback(notes) {
		lines = append(lines, "- "+lang.T("assistant.meeting_minutes_package.body.action_table_item", action.Task, action.Owner, action.Due))
	}
	lines = append(lines, "", "## "+lang.T("assistant.meeting_minutes_package.next_signal_title"))
	lines = append(lines, assistantMeetingMinutesNextSignalActionLines(notes, 3)...)
	lines = append(lines,
		"",
		"## "+lang.T("assistant.meeting_minutes_package.body.risks"),
	)
	lines = append(lines, assistantMeetingMinutesExtractedLines(notes, []string{"리스크:", "위험:", "risk:"}, []string{
		lang.T("assistant.meeting_minutes_package.body.risk_default.1"),
		lang.T("assistant.meeting_minutes_package.body.risk_default.2"),
	})...)
	lines = append(lines,
		"",
		"## "+lang.T("assistant.meeting_minutes_package.body.next_agenda"),
		"1. "+lang.T("assistant.meeting_minutes_package.body.next_agenda.1"),
		"2. "+lang.T("assistant.meeting_minutes_package.body.next_agenda.2"),
		"3. "+lang.T("assistant.meeting_minutes_package.body.next_agenda.3"),
	)
	return strings.Join(lines, "\n")
}

func assistantMeetingMinutesPackageDeckBody(title, notes string) string {
	return strings.Join([]string{
		"# " + lang.T("assistant.meeting_minutes_package.deck.purpose"),
		lang.T("assistant.meeting_minutes_package.deck.purpose_body", title),
		"# " + lang.T("assistant.meeting_minutes_package.deck.summary"),
		strings.Join(assistantMeetingMinutesInlineSummary(notes), "\n"),
		"# " + lang.T("assistant.meeting_minutes_package.deck.decisions"),
		strings.Join(assistantMeetingMinutesExtractedLines(notes, []string{"결정:", "결정사항:", "decision:", "decisions:"}, []string{
			lang.T("assistant.meeting_minutes_package.deck.decision_default.1"),
			lang.T("assistant.meeting_minutes_package.deck.decision_default.2"),
		}), "\n"),
		"# " + lang.T("assistant.meeting_minutes_package.deck.actions"),
		strings.Join(assistantMeetingMinutesExtractedLines(notes, []string{"할 일:", "액션:", "action:"}, []string{
			lang.T("assistant.meeting_minutes_package.deck.action_default.1"),
			lang.T("assistant.meeting_minutes_package.deck.action_default.2"),
		}), "\n"),
		"# " + lang.T("assistant.meeting_minutes_package.next_signal_title"),
		strings.Join(assistantMeetingMinutesNextSignalActionLines(notes, 3), "\n"),
		"# " + lang.T("assistant.meeting_minutes_package.deck.risks"),
		lang.T("assistant.meeting_minutes_package.deck.risk_body"),
		"# " + lang.T("assistant.meeting_minutes_package.deck.next"),
		lang.T("assistant.meeting_minutes_package.deck.next_body"),
		"# " + lang.T("assistant.meeting_minutes_package.deck.delivery"),
		lang.T("assistant.meeting_minutes_package.deck.delivery_body"),
	}, "\n\n")
}

func assistantMeetingMinutesPackageSheetBody(notes string) string {
	rows := []string{
		lang.T("assistant.meeting_minutes_package.sheet.header"),
		"|---|---|---|---|---|---|---|",
	}
	actions := assistantMeetingMinutesActionItemsWithFallback(notes)
	for _, action := range actions {
		rows = append(rows, fmt.Sprintf("|%s|%s|%s|%s|%s|%s|%s|",
			cleanSpreadsheetCell(lang.T("assistant.meeting_minutes_package.sheet.type_action")),
			cleanSpreadsheetCell(action.Task),
			cleanSpreadsheetCell(action.Owner),
			cleanSpreadsheetCell(action.Due),
			cleanSpreadsheetCell(lang.T("assistant.meeting_minutes_package.sheet.status_pending")),
			cleanSpreadsheetCell(action.Risk),
			cleanSpreadsheetCell(action.Next),
		))
	}
	rows = append(rows, lang.T("assistant.meeting_minutes_package.sheet.decision_row"))
	rows = append(rows, lang.T("assistant.meeting_minutes_package.sheet.risk_row"))
	return strings.Join(rows, "\n")
}

type assistantMeetingActionItem struct {
	Task  string
	Owner string
	Due   string
	Risk  string
	Next  string
}

var (
	assistantMeetingOwnerRE      = regexp.MustCompile(`(?i)(?:담당자?|owner)\s*[:：=]\s*([^,，/|]+)`)
	assistantMeetingDueRE        = regexp.MustCompile(`(?i)(?:마감|기한|due)\s*[:：=]\s*([^,，/|]+)`)
	assistantMeetingLeadingOwner = regexp.MustCompile(`^\s*([가-힣A-Za-z0-9_.-]{2,20})(?:가|이|는|은)\s+(.+)$`)
)

func assistantMeetingMinutesActionItems(notes string) []assistantMeetingActionItem {
	raw := assistantMeetingMinutesExtractedPlain(notes, []string{"할 일:", "할일:", "액션:", "액션아이템:", "action:"})
	items := []assistantMeetingActionItem{}
	for i, item := range raw {
		action := parseAssistantMeetingActionItem(item, i+1)
		if action.Task == "" {
			continue
		}
		items = append(items, action)
	}
	return items
}

func assistantMeetingMinutesActionItemsWithFallback(notes string) []assistantMeetingActionItem {
	actions := assistantMeetingMinutesActionItems(notes)
	if len(actions) > 0 {
		return actions
	}
	return []assistantMeetingActionItem{
		{
			Task:  lang.T("assistant.meeting_minutes_package.fallback.1.task"),
			Owner: lang.T("assistant.meeting_minutes_package.fallback.1.owner"),
			Due:   lang.T("assistant.meeting_minutes_package.fallback.1.due"),
			Risk:  lang.T("assistant.meeting_minutes_package.fallback.1.risk"),
			Next:  lang.T("assistant.meeting_minutes_package.fallback.1.next"),
		},
		{
			Task:  lang.T("assistant.meeting_minutes_package.fallback.2.task"),
			Owner: "PM",
			Due:   lang.T("assistant.meeting_minutes_package.fallback.2.due"),
			Risk:  lang.T("assistant.meeting_minutes_package.fallback.2.risk"),
			Next:  lang.T("assistant.meeting_minutes_package.fallback.2.next"),
		},
		{
			Task:  lang.T("assistant.meeting_minutes_package.fallback.3.task"),
			Owner: lang.T("assistant.meeting_minutes_package.fallback.3.owner"),
			Due:   lang.T("assistant.meeting_minutes_package.fallback.3.due"),
			Risk:  lang.T("assistant.meeting_minutes_package.fallback.3.risk"),
			Next:  lang.T("assistant.meeting_minutes_package.fallback.3.next"),
		},
	}
}

func parseAssistantMeetingActionItem(raw string, index int) assistantMeetingActionItem {
	task := strings.TrimSpace(raw)
	owner := ""
	due := ""
	if match := assistantMeetingOwnerRE.FindStringSubmatch(task); len(match) > 1 {
		owner = trimMeetingActionField(match[1])
		task = strings.TrimSpace(assistantMeetingOwnerRE.ReplaceAllString(task, ""))
	}
	if match := assistantMeetingDueRE.FindStringSubmatch(task); len(match) > 1 {
		due = trimMeetingActionField(match[1])
		task = strings.TrimSpace(assistantMeetingDueRE.ReplaceAllString(task, ""))
	}
	if owner == "" {
		if match := assistantMeetingLeadingOwner.FindStringSubmatch(task); len(match) > 2 {
			owner = trimMeetingActionField(match[1])
			task = strings.TrimSpace(match[2])
		}
	}
	if due == "" {
		due = detectAssistantMeetingDue(task)
	}
	task = strings.Trim(strings.TrimSpace(task), ",，/|")
	if task == "" {
		task = strings.TrimSpace(raw)
	}
	if owner == "" {
		owner = lang.T("assistant.meeting_minutes_package.owner_default", index)
	}
	if due == "" {
		due = lang.T("assistant.meeting_minutes_package.due_default")
	}
	return assistantMeetingActionItem{
		Task:  task,
		Owner: owner,
		Due:   due,
		Risk:  lang.T("assistant.meeting_minutes_package.risk_default"),
		Next:  lang.T("assistant.meeting_minutes_package.next_default"),
	}
}

func trimMeetingActionField(value string) string {
	value = strings.TrimSpace(strings.Trim(value, " ,，./|"))
	return trimForContext(value, 40)
}

func detectAssistantMeetingDue(task string) string {
	lower := strings.ToLower(task)
	for _, marker := range []string{"오늘", "내일", "이번 주", "다음 주", "월요일", "화요일", "수요일", "목요일", "금요일", "토요일", "일요일", "today", "tomorrow", "friday", "monday"} {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return marker
		}
	}
	return ""
}

func assistantMeetingMinutesActionPreview(action assistantMeetingActionItem) string {
	return lang.T("assistant.meeting_minutes_package.action_preview_item", action.Task, action.Owner, action.Due)
}

func assistantMeetingMinutesNextSignalActionLines(notes string, limit int) []string {
	if limit <= 0 {
		limit = 2
	}
	actions := assistantMeetingMinutesActionItemsWithFallback(notes)
	lines := []string{}
	for i, action := range actions {
		if i >= limit {
			break
		}
		lines = append(lines, "- "+lang.T("assistant.meeting_minutes_package.next_signal.owner_followup", action.Owner, action.Task, action.Due))
	}
	if len(actions) > 0 {
		first := actions[0]
		lines = append(lines, "- "+lang.T("assistant.meeting_minutes_package.next_signal.mail_draft", first.Owner, first.Task, first.Due))
		lines = append(lines, "- "+lang.T("assistant.meeting_minutes_package.next_signal.reminder_draft", first.Owner, first.Task, first.Due))
	}
	lines = append(lines, "- "+lang.T("assistant.meeting_minutes_package.next_signal.calendar_draft"))
	lines = append(lines, "- "+lang.T("assistant.meeting_minutes_package.next_signal.briefing_update"))
	lines = append(lines, "- "+lang.T("assistant.meeting_minutes_package.next_signal.boundary"))
	return lines
}

func assistantMeetingMinutesInlineSummary(notes string) []string {
	trimmed := strings.TrimSpace(notes)
	if trimmed == "" {
		return []string{"- " + lang.T("assistant.meeting_minutes_package.summary.empty")}
	}
	sentences := splitAssistantMeetingMinutesSentences(trimmed)
	lines := []string{}
	for _, sentence := range sentences {
		if len(lines) >= 4 {
			break
		}
		lines = append(lines, "- "+trimForContext(sentence, 120))
	}
	if len(lines) == 0 {
		lines = append(lines, "- "+trimForContext(trimmed, 160))
	}
	return lines
}

func assistantMeetingMinutesExtractedLines(notes string, markers, fallback []string) []string {
	items := assistantMeetingMinutesExtractedPlain(notes, markers)
	if len(items) == 0 {
		items = fallback
	}
	lines := []string{}
	for _, item := range items {
		lines = append(lines, "- "+item)
	}
	return lines
}

func assistantMeetingMinutesExtractedPlain(notes string, markers []string) []string {
	lower := strings.ToLower(notes)
	for _, marker := range markers {
		idx := strings.Index(lower, strings.ToLower(marker))
		if idx < 0 {
			continue
		}
		part := strings.TrimSpace(notes[idx+len(marker):])
		for _, stop := range []string{" 결정:", " 결정사항:", " 할 일:", " 할일:", " 액션:", " 액션아이템:", " 리스크:", " 위험:", " risk:", " 음성", " voice", " 다음:", " next:"} {
			if stopIdx := strings.Index(strings.ToLower(part), strings.ToLower(stop)); stopIdx > 0 {
				part = strings.TrimSpace(part[:stopIdx])
			}
		}
		return splitAssistantMeetingMinutesSentences(part)
	}
	return nil
}

func splitAssistantMeetingMinutesSentences(text string) []string {
	replacer := strings.NewReplacer("\n", ". ", ";", ". ", "；", ". ", "。", ". ", "•", ". ", "- ", ". ")
	raw := strings.Split(replacer.Replace(text), ".")
	out := []string{}
	for _, item := range raw {
		item = strings.TrimSpace(strings.Trim(item, "·-:： "))
		if item == "" {
			continue
		}
		out = append(out, item)
		if len(out) >= 8 {
			break
		}
	}
	return out
}

func writeAssistantMeetingMinutesActionFlowMarkdown(title, notes string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Documents", "Argos Vault", "Charts")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, safeAssistantDepartmentFilename(title)+"-"+time.Now().Format("20060102-150405")+"-action-flow.md")
	content := strings.Join([]string{
		"# " + lang.T("assistant.meeting_minutes_package.graph.title", title),
		"",
		lang.T("assistant.meeting_minutes_package.graph.intro"),
		"",
		assistantMeetingMinutesActionFlowMermaid(notes),
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return "", err
	}
	return path, nil
}

func assistantMeetingMinutesActionFlowMermaid(notes string) string {
	actions := assistantMeetingMinutesActionItemsWithFallback(notes)
	lines := []string{
		"```mermaid",
		"flowchart LR",
		fmt.Sprintf("  Summary[\"%s\"] --> Decision[\"%s\"] --> Actions[\"%s\"]",
			assistantMermaidEscape(lang.T("assistant.meeting_minutes_package.graph.node.summary")),
			assistantMermaidEscape(lang.T("assistant.meeting_minutes_package.graph.node.decision")),
			assistantMermaidEscape(lang.T("assistant.meeting_minutes_package.graph.node.actions")),
		),
	}
	for i, action := range actions {
		if i >= 5 {
			break
		}
		id := fmt.Sprintf("A%d", i+1)
		label := lang.T("assistant.meeting_minutes_package.graph.action_label", shortenSVGText(action.Task, 34), shortenSVGText(action.Owner, 18), shortenSVGText(action.Due, 18))
		lines = append(lines, fmt.Sprintf("  Actions --> %s[\"%s\"]", id, strings.ReplaceAll(label, "\"", "'")))
		lines = append(lines, fmt.Sprintf("  %s --> Report%d[\"%s\"]", id, i+1, assistantMermaidEscape(lang.T("assistant.meeting_minutes_package.graph.node.report_update"))))
	}
	lines = append(lines,
		fmt.Sprintf("  Actions --> Risk[\"%s\"] --> Next[\"%s\"]",
			assistantMermaidEscape(lang.T("assistant.meeting_minutes_package.graph.node.risk")),
			assistantMermaidEscape(lang.T("assistant.meeting_minutes_package.graph.node.next")),
		),
		"```",
	)
	return strings.Join(lines, "\n")
}

func writeAssistantMeetingMinutesActionBoardSVG(title, notes string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Documents", "Argos Vault", "Charts")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, safeAssistantDepartmentFilename(title)+"-"+time.Now().Format("20060102-150405")+"-action-board.svg")
	content := renderAssistantMeetingMinutesActionBoardSVG(title, assistantMeetingMinutesActionItemsWithFallback(notes))
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return "", err
	}
	return path, nil
}

func renderAssistantMeetingMinutesActionBoardSVG(title string, actions []assistantMeetingActionItem) string {
	if len(actions) == 0 {
		actions = assistantMeetingMinutesActionItemsWithFallback("")
	}
	if len(actions) > 4 {
		actions = actions[:4]
	}
	colors := []string{"#2563eb", "#0f766e", "#7c3aed", "#d97706"}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="820" height="580" viewBox="0 0 820 580">` + "\n")
	b.WriteString(`<rect width="820" height="580" fill="#f8fafc"/>` + "\n")
	b.WriteString(`<rect x="24" y="24" width="772" height="532" rx="20" fill="#ffffff" stroke="#d9e2ef"/>` + "\n")
	b.WriteString(fmt.Sprintf(`<text x="48" y="68" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="27" font-weight="850" fill="#0f172a">%s</text>`, html.EscapeString(shortenSVGText(title, 38))))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`<text x="48" y="98" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="15" fill="#64748b">%s</text>`, html.EscapeString(lang.T("assistant.meeting_minutes_package.board.subtitle"))))
	b.WriteString("\n")
	b.WriteString(`<rect x="48" y="124" width="716" height="42" rx="12" fill="#eef2ff"/>` + "\n")
	headers := []struct {
		X     int
		Label string
	}{
		{60, lang.T("assistant.meeting_minutes_package.board.task")},
		{404, lang.T("assistant.meeting_minutes_package.board.owner")},
		{550, lang.T("assistant.meeting_minutes_package.board.due")},
		{696, lang.T("assistant.meeting_minutes_package.board.status")},
	}
	for _, header := range headers {
		b.WriteString(fmt.Sprintf(`<text x="%d" y="151" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="14" font-weight="800" fill="#334155">%s</text>`, header.X, html.EscapeString(header.Label)))
		b.WriteString("\n")
	}
	for i, action := range actions {
		y := 184 + i*84
		color := colors[i%len(colors)]
		b.WriteString(fmt.Sprintf(`<rect x="48" y="%d" width="716" height="68" rx="14" fill="#ffffff" stroke="#d9e2ef"/>`, y))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf(`<rect x="48" y="%d" width="8" height="68" rx="4" fill="%s"/>`, y, color))
		b.WriteString("\n")
		for j, line := range wrapSVGText(action.Task, 28, 2) {
			b.WriteString(fmt.Sprintf(`<text x="72" y="%d" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="15" font-weight="750" fill="#0f172a">%s</text>`, y+26+j*20, html.EscapeString(line)))
			b.WriteString("\n")
		}
		b.WriteString(fmt.Sprintf(`<text x="404" y="%d" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="15" fill="#334155">%s</text>`, y+40, html.EscapeString(shortenSVGText(action.Owner, 13))))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf(`<text x="550" y="%d" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="15" fill="#334155">%s</text>`, y+40, html.EscapeString(shortenSVGText(action.Due, 13))))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf(`<rect x="694" y="%d" width="58" height="28" rx="14" fill="#fef3c7"/>`, y+20))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf(`<text x="723" y="%d" text-anchor="middle" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="13" font-weight="800" fill="#92400e">%s</text>`, y+39, html.EscapeString(lang.T("assistant.meeting_minutes_package.board.pending"))))
		b.WriteString("\n")
	}
	b.WriteString(fmt.Sprintf(`<text x="48" y="536" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="14" fill="#64748b">%s</text>`, html.EscapeString(lang.T("assistant.meeting_minutes_package.board.footer"))))
	b.WriteString("\n")
	b.WriteString("</svg>\n")
	return b.String()
}

func assistantMeetingMinutesPackageVoiceScript(title, notes string) string {
	lines := []string{
		lang.T("assistant.meeting_minutes_package.voice.intro"),
		lang.T("assistant.meeting_minutes_package.voice.title", title),
	}
	summary := assistantMeetingMinutesInlineSummary(notes)
	if len(summary) > 0 {
		lines = append(lines, lang.T("assistant.meeting_minutes_package.voice.summary", strings.TrimPrefix(strings.Join(summary, " "), "- ")))
	}
	lines = append(lines,
		lang.T("assistant.meeting_minutes_package.voice.attachments"),
		lang.T("assistant.meeting_minutes_package.voice.boundary"),
		lang.T("assistant.meeting_minutes_package.voice.next"),
	)
	return strings.Join(lines, "\n\n")
}

func assistantMeetingMinutesPackageAttachments(doc, deck, sheet osauto.Result) []string {
	attachments := []string{}
	attachments = append(attachments, assistantDocumentAttachments(doc)...)
	for _, path := range []string{deck.PPTX, deck.PDF, deck.Markdown} {
		if strings.TrimSpace(path) != "" {
			attachments = append(attachments, path)
		}
	}
	for _, path := range []string{sheet.XLSX, sheet.CSV} {
		if strings.TrimSpace(path) != "" {
			attachments = append(attachments, path)
		}
	}
	return uniqueShowcaseAttachments(attachments)
}
