package messenger

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/meshclaw/meshclaw/internal/evidence"
	"github.com/meshclaw/meshclaw/internal/lang"
	"github.com/meshclaw/meshclaw/internal/mailadapter"
	"github.com/meshclaw/meshclaw/internal/osauto"
	"github.com/meshclaw/meshclaw/internal/tts"
)

type assistantDailyAgendaData struct {
	Title          string
	Start          time.Time
	End            time.Time
	CalendarResult osauto.Result
	ReminderResult osauto.Result
	Events         []calendarListEvent
	Reminders      []assistantDailyAgendaReminder
	MailRequested  bool
	MailItems      []signalMailPriorityItem
	MailErrors     []signalMailAccountError
}

type assistantDailyAgendaReminder struct {
	Title    string `json:"title"`
	Due      string `json:"due"`
	Calendar string `json:"calendar"`
	Notes    string `json:"notes"`
}

func assistantDailyAgendaPackageReply(opts ListenOptions, request string) (string, bool) {
	if !isAssistantDailyAgendaPackageRequest(request) {
		return "", false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()

	data := collectAssistantDailyAgendaData(ctx, request)
	restorePreview := temporarilySkipPreviewImages()
	doc := osauto.CreateArgosDocument(ctx, lang.T("assistant.daily_agenda_package.doc_title", data.Title), assistantDailyAgendaPackageReportBody(request, data))
	deck := osauto.CreatePresentation(ctx, lang.T("assistant.daily_agenda_package.deck_title", data.Title), assistantDailyAgendaPackageDeckBody(data), lang.T("assistant.daily_agenda_package.deck_audience"), 7, "")
	sheet := osauto.CreateSpreadsheet(ctx, lang.T("assistant.daily_agenda_package.sheet_title", data.Title), assistantDailyAgendaPackageSheetBody(data))
	timelinePath, timelineErr := writeAssistantDailyAgendaTimelineSVG(data)
	restorePreview()

	wantsVoice := assistantBrowserResearchPackageWantsVoice(request)
	voiceScript := ""
	var audio tts.Result
	var audioErr error
	if wantsVoice {
		voiceScript = assistantDailyAgendaPackageVoiceScript(data)
		audio, audioErr = tts.Synthesize(tts.Options{
			Text:     voiceScript,
			Engine:   "edge-tts",
			Basename: "daily-agenda-brief-" + time.Now().UTC().Format("20060102T150405Z"),
		})
	}

	attachments := assistantDailyAgendaPackageAttachments(doc, deck, sheet)
	if strings.TrimSpace(timelinePath) != "" && timelineErr == nil {
		attachments = uniqueShowcaseAttachments(append(attachments, timelinePath))
	}
	if strings.TrimSpace(audio.Path) != "" && audioErr == nil {
		attachments = uniqueShowcaseAttachments(append(attachments, audio.Path))
	}
	rememberAssistantArtifact(opts, data.Title, "daily_agenda_package", doc, deck, sheet)
	record, storeErr := evidence.Store("assistant-daily-agenda-package", firstNonEmpty(opts.TargetID, "assistant"), data.Title, map[string]interface{}{
		"kind":            "assistant_daily_agenda_package",
		"request":         request,
		"start":           data.Start,
		"end":             data.End,
		"events":          data.Events,
		"reminders":       data.Reminders,
		"mail_requested":  data.MailRequested,
		"mail_items":      data.MailItems,
		"mail_errors":     data.MailErrors,
		"calendar_result": data.CalendarResult,
		"reminder_result": data.ReminderResult,
		"voice_script":    voiceScript,
		"audio":           audio,
		"audio_error":     errorString(audioErr),
		"timeline":        timelinePath,
		"timeline_error":  errorString(timelineErr),
		"document":        doc,
		"presentation":    deck,
		"spreadsheet":     sheet,
		"attachments":     attachments,
		"created_at":      time.Now().UTC(),
	})

	summary := assistantDailyAgendaPackageSummary(data, doc, deck, sheet, timelinePath, timelineErr, wantsVoice, audio, audioErr)
	if targetRef := inferAssistantSignalTargetRef(request); targetRef != "" {
		return formatAssistantDailyAgendaPackageSendResult(opts, targetRef, summary, attachments, record, storeErr), true
	}
	lines := append([]string{}, summary...)
	lines = appendVoiceReportAttachmentMarkers(lines, attachments)
	lines = appendAssistantEvidenceNote(lines, record, storeErr)
	return strings.Join(compactBlankLines(lines), "\n"), true
}

func isAssistantDailyAgendaPackageRequest(request string) bool {
	lower := strings.ToLower(strings.TrimSpace(request))
	if lower == "" {
		return false
	}
	timeSignal := containsAny(lower, "오늘", "내일", "하루", "아침", "daily", "agenda", "plan my day", "day plan")
	workSignal := containsAny(lower, "일정", "캘린더", "할 일", "할일", "리마인더", "todo", "reminder", "calendar")
	artifactSignal := inferAssistantSignalTargetRef(request) != "" || containsAny(lower,
		"보고방", "보내", "전송", "첨부", "패키지", "문서", "보고서", "ppt", "pptx", "표", "엑셀", "xlsx", "csv", "음성", "음성파일", "tts", "mp3",
		"send", "share", "package", "deck", "slides", "spreadsheet", "voice",
	)
	return timeSignal && workSignal && artifactSignal
}

func collectAssistantDailyAgendaData(ctx context.Context, request string) assistantDailyAgendaData {
	start, end, title := assistantDailyAgendaRange(request)
	data := assistantDailyAgendaData{Title: title, Start: start, End: end}
	if strings.TrimSpace(os.Getenv("MESHCLAW_ASSISTANT_DAILY_AGENDA_NO_FETCH")) != "" {
		return data
	}
	startISO := start.Format(time.RFC3339)
	endISO := end.Format(time.RFC3339)
	if fake := strings.TrimSpace(os.Getenv("MESHCLAW_ASSISTANT_DAILY_AGENDA_CALENDAR_STDOUT")); fake != "" {
		data.CalendarResult = osauto.Result{Kind: "meshclaw_automation_calendar_events_list", Action: "calendar_events_list", OK: true, Stdout: fake, CreatedAt: time.Now().UTC()}
	} else {
		data.CalendarResult = osauto.ListCalendarEvents(ctx, startISO, endISO, "")
	}
	if fake := strings.TrimSpace(os.Getenv("MESHCLAW_ASSISTANT_DAILY_AGENDA_REMINDER_STDOUT")); fake != "" {
		data.ReminderResult = osauto.Result{Kind: "meshclaw_automation_reminders_list", Action: "reminders_list", OK: true, Stdout: fake, CreatedAt: time.Now().UTC()}
	} else {
		data.ReminderResult = osauto.ListReminders(ctx, startISO, endISO, "")
	}
	data.Events = parseAssistantDailyAgendaEvents(data.CalendarResult)
	data.Reminders = parseAssistantDailyAgendaReminders(data.ReminderResult)
	data.MailRequested = assistantDailyAgendaWantsMail(request)
	if data.MailRequested {
		data.MailItems, data.MailErrors = collectAssistantDailyAgendaMailPriority()
	}
	return data
}

func assistantDailyAgendaWantsMail(request string) bool {
	lower := strings.ToLower(strings.TrimSpace(request))
	return containsAny(lower, "메일", "이메일", "mail", "email")
}

func collectAssistantDailyAgendaMailPriority() ([]signalMailPriorityItem, []signalMailAccountError) {
	if fake := strings.TrimSpace(os.Getenv("MESHCLAW_ASSISTANT_DAILY_AGENDA_MAIL_STDOUT")); fake != "" {
		var payload signalMailMultiSearchResult
		if err := json.Unmarshal([]byte(fake), &payload); err == nil {
			return mailPriorityItemsFromResults(payload.Results), payload.Errors
		}
	}
	selected, accounts, err := defaultSignalMailAccount()
	if err != nil {
		return nil, []signalMailAccountError{{Account: mailadapter.AccountPublic{ID: "mail"}, Error: err.Error()}}
	}
	results := []mailadapter.SearchResult{}
	errors := []signalMailAccountError{}
	if selected != "" {
		result, searchErr := mailadapter.Search(mailadapter.SearchOptions{Account: selected, Limit: 10})
		if searchErr != nil {
			return nil, []signalMailAccountError{{Account: mailadapter.AccountPublic{ID: selected}, Error: searchErr.Error()}}
		}
		results = append(results, result)
	} else {
		if len(accounts) == 0 {
			return nil, []signalMailAccountError{{Account: mailadapter.AccountPublic{ID: "mail"}, Error: lang.T("assistant.daily_agenda_package.mail.no_account")}}
		}
		for _, account := range accounts {
			result, searchErr := mailadapter.Search(mailadapter.SearchOptions{Account: account.ID, Limit: 10})
			if searchErr != nil {
				errors = append(errors, signalMailAccountError{Account: account, Error: searchErr.Error()})
				continue
			}
			results = append(results, result)
		}
	}
	return mailPriorityItemsFromResults(results), errors
}

func assistantDailyAgendaRange(request string) (time.Time, time.Time, string) {
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		loc = time.FixedZone("KST", 9*60*60)
	}
	now := time.Now().In(loc)
	day := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	lower := strings.ToLower(request)
	title := lang.T("assistant.daily_agenda_package.title.today")
	if containsAny(lower, "내일", "tomorrow") {
		day = day.AddDate(0, 0, 1)
		title = lang.T("assistant.daily_agenda_package.title.tomorrow")
	}
	days := 1
	if containsAny(lower, "이번 주", "일주일", "7일", "week") {
		days = 7
		title = lang.T("assistant.daily_agenda_package.title.week")
	}
	return day, day.AddDate(0, 0, days), title
}

func parseAssistantDailyAgendaEvents(result osauto.Result) []calendarListEvent {
	payload := strings.TrimSpace(result.Stdout)
	if payload == "" {
		return nil
	}
	var outer struct {
		Stdout string `json:"stdout"`
	}
	if err := json.Unmarshal([]byte(payload), &outer); err == nil && strings.TrimSpace(outer.Stdout) != "" {
		payload = strings.TrimSpace(outer.Stdout)
	}
	var list struct {
		Events []calendarListEvent `json:"events"`
	}
	if err := json.Unmarshal([]byte(payload), &list); err != nil {
		return nil
	}
	return list.Events
}

func parseAssistantDailyAgendaReminders(result osauto.Result) []assistantDailyAgendaReminder {
	payload := strings.TrimSpace(result.Stdout)
	if payload == "" {
		return nil
	}
	var outer struct {
		Stdout string `json:"stdout"`
	}
	if err := json.Unmarshal([]byte(payload), &outer); err == nil && strings.TrimSpace(outer.Stdout) != "" {
		payload = strings.TrimSpace(outer.Stdout)
	}
	var list struct {
		Reminders []assistantDailyAgendaReminder `json:"reminders"`
	}
	if err := json.Unmarshal([]byte(payload), &list); err != nil {
		return nil
	}
	return list.Reminders
}

func assistantDailyAgendaPackageSummary(data assistantDailyAgendaData, doc, deck, sheet osauto.Result, timelinePath string, timelineErr error, wantsVoice bool, audio tts.Result, audioErr error) []string {
	lines := []string{
		lang.T("assistant.daily_agenda_package.created", data.Title),
		lang.T("assistant.daily_agenda_package.subtitle"),
		"",
		lang.T("assistant.daily_agenda_package.range", data.Start.Format("2006-01-02"), data.End.Add(-time.Second).Format("2006-01-02")),
		lang.T("assistant.daily_agenda_package.counts", len(data.Events), len(data.Reminders)),
	}
	if data.MailRequested {
		lines = append(lines, lang.T("assistant.daily_agenda_package.mail_counts", len(data.MailItems), len(mailPriorityActionableItems(data.MailItems))))
	}
	lines = append(lines, assistantDailyAgendaSourceIssueLines(data)...)
	lines = append(lines, "", lang.T("assistant.daily_agenda_package.preview"))
	lines = append(lines, assistantDailyAgendaInlinePlan(data)...)
	lines = append(lines, "", lang.T("assistant.daily_agenda_package.approval_drafts"))
	lines = append(lines, assistantDailyAgendaApprovalDraftLines()...)
	lines = append(lines, "", lang.T("assistant.daily_agenda_package.files"))
	if doc.OK && doc.Error == "" {
		lines = append(lines, "- "+lang.T("assistant.daily_agenda_package.file.doc"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.daily_agenda_package.file.doc_failed", firstNonEmpty(doc.Error, doc.Stderr, "unknown error")))
	}
	if deck.OK && deck.Error == "" {
		lines = append(lines, "- "+lang.T("assistant.daily_agenda_package.file.ppt"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.daily_agenda_package.file.ppt_failed", firstNonEmpty(deck.Error, deck.Stderr, "unknown error")))
	}
	if sheet.OK && sheet.Error == "" {
		lines = append(lines, "- "+lang.T("assistant.daily_agenda_package.file.sheet"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.daily_agenda_package.file.sheet_failed", firstNonEmpty(sheet.Error, sheet.Stderr, "unknown error")))
	}
	if strings.TrimSpace(timelinePath) != "" && timelineErr == nil {
		lines = append(lines, "- "+lang.T("assistant.daily_agenda_package.file.timeline"))
	} else if timelineErr != nil {
		lines = append(lines, "- "+lang.T("assistant.daily_agenda_package.file.timeline_failed", firstNonEmpty(errorString(timelineErr), "unknown error")))
	}
	if wantsVoice {
		if audioErr == nil && strings.TrimSpace(audio.Path) != "" {
			lines = append(lines, "- "+lang.T("assistant.daily_agenda_package.file.audio"))
		} else {
			lines = append(lines, "- "+lang.T("assistant.daily_agenda_package.file.audio_failed", firstNonEmpty(errorString(audioErr), "unknown error")))
		}
	}
	lines = append(lines,
		"",
		lang.T("assistant.daily_agenda_package.next"),
		"- "+lang.T("assistant.daily_agenda_package.next.focus"),
		"- "+lang.T("assistant.daily_agenda_package.next.calendar"),
		"- "+lang.T("assistant.daily_agenda_package.next.reminder"),
		"- "+lang.T("assistant.daily_agenda_package.next.brief"),
	)
	return compactBlankLines(lines)
}

func assistantDailyAgendaSourceIssueLines(data assistantDailyAgendaData) []string {
	lines := []string{}
	if strings.TrimSpace(data.CalendarResult.Error) != "" || strings.TrimSpace(data.ReminderResult.Error) != "" || len(data.MailErrors) > 0 {
		lines = append(lines, lang.T("assistant.daily_agenda_package.source_issues"))
	}
	if strings.TrimSpace(data.CalendarResult.Error) != "" {
		lines = append(lines, "- "+lang.T("assistant.daily_agenda_package.source_issue.calendar", trimForContext(data.CalendarResult.Error, 160)))
	}
	if strings.TrimSpace(data.ReminderResult.Error) != "" {
		lines = append(lines, "- "+lang.T("assistant.daily_agenda_package.source_issue.reminders", trimForContext(data.ReminderResult.Error, 160)))
	}
	for _, item := range data.MailErrors {
		lines = append(lines, "- "+lang.T("assistant.daily_agenda_package.source_issue.mail", strings.TrimSpace(item.Account.ID), trimForContext(item.Error, 160)))
	}
	return lines
}

func formatAssistantDailyAgendaPackageSendResult(opts ListenOptions, targetRef string, summary []string, attachments []string, record evidence.Record, storeErr error) string {
	target, candidates, targetErr := resolveAssistantVoiceTarget(targetRef)
	if targetErr != nil {
		lines := []string{lang.T("assistant.daily_agenda_package.send_target_failed")}
		lines = append(lines, formatAssistantVoiceTargetCandidates(candidates)...)
		lines = append(lines, "", lang.T("assistant.daily_agenda_package.attach_here"))
		lines = appendVoiceReportAttachmentMarkers(lines, attachments)
		return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
	}
	if !opts.Execute {
		targetLabel := assistantScheduleLocalizedTarget(target.ID, target.Label)
		lines := []string{
			lang.T("assistant.daily_agenda_package.send_ready"),
			lang.T("assistant.daily_agenda_package.target", targetLabel),
		}
		if OneWayReportTarget(target) {
			lines = append(lines, lang.T("assistant.daily_agenda_package.one_way"))
		}
		lines = append(lines, "", lang.T("assistant.daily_agenda_package.to_send"))
		lines = append(lines, summary...)
		lines = appendVoiceReportAttachmentMarkers(lines, attachments)
		return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
	}
	mobileLinkLines := assistantWorkflowMobileLinkLines(attachments, 6)
	sendSummary := appendAssistantWorkflowMobileLinkLines(summary, mobileLinkLines)
	sendText := strings.Join(compactBlankLines(sendSummary), "\n")
	send, sendErr := Send(SendOptions{TargetID: target.ID, Kind: "text", Text: sendText, Attachments: attachments, Execute: true, TimeoutSeconds: 90})
	payload := map[string]interface{}{
		"kind":             "assistant_daily_agenda_package_send",
		"target":           targetRef,
		"resolved_target":  target,
		"attachment_count": len(attachments),
		"mobile_links":     mobileLinkLines,
		"send":             send,
		"send_error":       errorString(sendErr),
		"created_at":       time.Now().UTC(),
	}
	sendRecord, sendStoreErr := evidence.Store("assistant-daily-agenda-package-send", firstNonEmpty(opts.TargetID, "assistant"), target.ID, payload)
	targetLabel := assistantScheduleLocalizedTarget(target.ID, target.Label)
	if sendErr != nil {
		lines := []string{
			lang.T("assistant.daily_agenda_package.send_failed"),
			lang.T("assistant.daily_agenda_package.target", targetLabel),
			lang.T("assistant.daily_agenda_package.problem", sendErr.Error()),
			"",
			lang.T("assistant.daily_agenda_package.attach_here"),
		}
		lines = appendAssistantWorkflowMobileLinkLines(lines, mobileLinkLines)
		lines = appendVoiceReportAttachmentMarkers(lines, attachments)
		return strings.Join(appendAssistantEvidenceNote(lines, sendRecord, sendStoreErr), "\n")
	}
	lines := []string{
		lang.T("assistant.daily_agenda_package.sent"),
		lang.T("assistant.daily_agenda_package.target", targetLabel),
	}
	if id := strings.TrimSpace(send.Stdout); id != "" {
		lines = append(lines, lang.T("assistant.workflow.signal_id", id))
	}
	return strings.Join(appendAssistantEvidenceNote(lines, sendRecord, sendStoreErr), "\n")
}

func assistantDailyAgendaPackageReportBody(request string, data assistantDailyAgendaData) string {
	lines := []string{
		lang.T("assistant.daily_agenda_package.report.title", data.Title),
		"",
		lang.T("assistant.daily_agenda_package.report.generated", time.Now().In(time.FixedZone("KST", 9*60*60)).Format("2006-01-02 15:04 KST")),
		lang.T("assistant.daily_agenda_package.report.request", strings.TrimSpace(request)),
		lang.T("assistant.daily_agenda_package.report.range", data.Start.Format("2006-01-02"), data.End.Add(-time.Second).Format("2006-01-02")),
		"",
		lang.T("assistant.daily_agenda_package.report.conclusion"),
	}
	lines = append(lines, assistantDailyAgendaInlinePlan(data)...)
	lines = append(lines, "", lang.T("assistant.daily_agenda_package.report.events"))
	if len(data.Events) == 0 {
		lines = append(lines, "- "+lang.T("assistant.daily_agenda_package.report.no_events"))
	} else {
		for _, event := range data.Events {
			lines = append(lines, "- "+assistantDailyAgendaEventLine(event))
		}
	}
	lines = append(lines, "", lang.T("assistant.daily_agenda_package.report.tasks"))
	if len(data.Reminders) == 0 {
		lines = append(lines, "- "+lang.T("assistant.daily_agenda_package.report.no_tasks"))
	} else {
		for _, reminder := range data.Reminders {
			lines = append(lines, "- "+assistantDailyAgendaReminderLine(reminder))
		}
	}
	if data.MailRequested {
		lines = append(lines, "", lang.T("assistant.daily_agenda_package.report.mail"))
		lines = append(lines, assistantDailyAgendaMailBullets(data.MailItems)...)
	}
	lines = append(lines, "", lang.T("assistant.daily_agenda_package.report.approval_drafts"))
	lines = append(lines, assistantDailyAgendaApprovalDraftLines()...)
	lines = append(lines,
		"",
		lang.T("assistant.daily_agenda_package.report.execution"),
		"1. "+lang.T("assistant.daily_agenda_package.report.execution.1"),
		"2. "+lang.T("assistant.daily_agenda_package.report.execution.2"),
		"3. "+lang.T("assistant.daily_agenda_package.report.execution.3"),
		"4. "+lang.T("assistant.daily_agenda_package.report.execution.4"),
		"5. "+lang.T("assistant.daily_agenda_package.report.execution.5"),
	)
	if data.CalendarResult.Error != "" || data.ReminderResult.Error != "" {
		lines = append(lines, "", lang.T("assistant.daily_agenda_package.report.needs_check"))
		if data.CalendarResult.Error != "" {
			lines = append(lines, "- Calendar: "+data.CalendarResult.Error)
		}
		if data.ReminderResult.Error != "" {
			lines = append(lines, "- Reminders: "+data.ReminderResult.Error)
		}
	}
	if len(data.MailErrors) > 0 {
		lines = append(lines, "", lang.T("assistant.daily_agenda_package.report.needs_check"))
		for _, item := range data.MailErrors {
			lines = append(lines, "- "+lang.T("assistant.daily_agenda_package.source_issue.mail", strings.TrimSpace(item.Account.ID), item.Error))
		}
	}
	return strings.Join(lines, "\n")
}

func assistantDailyAgendaPackageDeckBody(data assistantDailyAgendaData) string {
	return strings.Join([]string{
		lang.T("assistant.daily_agenda_package.deck.conclusion"),
		lang.T("assistant.daily_agenda_package.deck.conclusion_body", len(data.Events), len(data.Reminders)),
		lang.T("assistant.daily_agenda_package.deck.events"),
		strings.Join(assistantDailyAgendaEventBullets(data.Events), "\n"),
		lang.T("assistant.daily_agenda_package.deck.tasks"),
		strings.Join(assistantDailyAgendaReminderBullets(data.Reminders), "\n"),
		lang.T("assistant.daily_agenda_package.deck.mail"),
		strings.Join(assistantDailyAgendaMailBullets(data.MailItems), "\n"),
		lang.T("assistant.daily_agenda_package.deck.focus"),
		lang.T("assistant.daily_agenda_package.deck.focus_body"),
		lang.T("assistant.daily_agenda_package.deck.risks"),
		lang.T("assistant.daily_agenda_package.deck.risks_body"),
		lang.T("assistant.daily_agenda_package.deck.next"),
		lang.T("assistant.daily_agenda_package.deck.next_body"),
		lang.T("assistant.daily_agenda_package.deck.approval_drafts"),
		strings.Join(assistantDailyAgendaApprovalDraftLines(), "\n"),
		lang.T("assistant.daily_agenda_package.deck.boundary"),
		lang.T("assistant.daily_agenda_package.deck.boundary_body"),
	}, "\n\n")
}

func assistantDailyAgendaPackageSheetBody(data assistantDailyAgendaData) string {
	rows := []string{
		lang.T("assistant.daily_agenda_package.sheet.header"),
		"|---|---|---|---|---|",
	}
	for _, event := range data.Events {
		rows = append(rows, fmt.Sprintf("|%s|%s|%s|%s|%s|",
			cleanSpreadsheetCell(lang.T("assistant.daily_agenda_package.sheet.type.event")),
			cleanSpreadsheetCell(assistantDailyAgendaTimeRange(event.Start, event.End)),
			cleanSpreadsheetCell(firstNonEmpty(event.Title, lang.T("assistant.daily_agenda_package.untitled"))),
			cleanSpreadsheetCell(event.Calendar),
			cleanSpreadsheetCell(lang.T("assistant.daily_agenda_package.sheet.action.event")),
		))
	}
	for _, reminder := range data.Reminders {
		rows = append(rows, fmt.Sprintf("|%s|%s|%s|%s|%s|",
			cleanSpreadsheetCell(lang.T("assistant.daily_agenda_package.sheet.type.task")),
			cleanSpreadsheetCell(assistantDailyAgendaTimeRange(reminder.Due, "")),
			cleanSpreadsheetCell(firstNonEmpty(reminder.Title, lang.T("assistant.daily_agenda_package.untitled"))),
			cleanSpreadsheetCell(reminder.Calendar),
			cleanSpreadsheetCell(lang.T("assistant.daily_agenda_package.sheet.action.task")),
		))
	}
	if data.MailRequested {
		for _, item := range assistantDailyAgendaMailTopItems(data.MailItems, 8) {
			message := item.Message
			rows = append(rows, fmt.Sprintf("|%s|%s|%s|%s|%s|",
				cleanSpreadsheetCell(lang.T("assistant.daily_agenda_package.sheet.type.mail")),
				cleanSpreadsheetCell(formatSignalMailMessageTime(message.Date)),
				cleanSpreadsheetCell(cleanSignalMailHeaderForDisplay(message.Subject, lang.T("assistant.mail_priority_package.subject_unknown"))),
				cleanSpreadsheetCell(firstNonEmpty(item.Account, cleanSignalMailHeaderForDisplay(message.From, lang.T("assistant.mail_priority_package.from_unknown")))),
				cleanSpreadsheetCell(item.Action),
			))
		}
	}
	if len(rows) == 2 {
		rows = append(rows, lang.T("assistant.daily_agenda_package.sheet.empty_row"))
	}
	return strings.Join(rows, "\n")
}

func assistantDailyAgendaPackageVoiceScript(data assistantDailyAgendaData) string {
	lines := []string{
		lang.T("assistant.daily_agenda_package.voice.intro"),
		lang.T("assistant.daily_agenda_package.voice.counts", len(data.Events), len(data.Reminders)),
	}
	if data.MailRequested {
		lines = append(lines, lang.T("assistant.daily_agenda_package.voice.mail_counts", len(data.MailItems), len(mailPriorityActionableItems(data.MailItems))))
	}
	if len(data.Events) > 0 {
		lines = append(lines, lang.T("assistant.daily_agenda_package.voice.first_event", assistantDailyAgendaEventLine(data.Events[0])))
	}
	if len(data.Reminders) > 0 {
		lines = append(lines, lang.T("assistant.daily_agenda_package.voice.first_task", assistantDailyAgendaReminderLine(data.Reminders[0])))
	}
	if mailItem, ok := assistantDailyAgendaFirstMailItem(data.MailItems); ok {
		lines = append(lines, lang.T("assistant.daily_agenda_package.voice.first_mail", assistantDailyAgendaMailLine(mailItem)))
	}
	lines = append(lines, lang.T("assistant.daily_agenda_package.voice.plan"))
	lines = append(lines, lang.T("assistant.daily_agenda_package.voice.approval_drafts"))
	lines = append(lines, lang.T("assistant.daily_agenda_package.voice.boundary"))
	return strings.Join(lines, "\n\n")
}

func assistantDailyAgendaPackageAttachments(doc, deck, sheet osauto.Result) []string {
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

type assistantDailyAgendaTimelineEntry struct {
	Kind   string
	When   string
	Title  string
	Detail string
	Time   time.Time
	Color  string
}

func writeAssistantDailyAgendaTimelineSVG(data assistantDailyAgendaData) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Documents", "Argos Vault", "Charts")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	base := safeAssistantDepartmentFilename(data.Title)
	path := filepath.Join(dir, base+"-"+time.Now().Format("20060102-150405")+"-timeline.svg")
	if err := os.WriteFile(path, []byte(renderAssistantDailyAgendaTimelineSVG(data)), 0600); err != nil {
		return "", err
	}
	return path, nil
}

func renderAssistantDailyAgendaTimelineSVG(data assistantDailyAgendaData) string {
	entries := assistantDailyAgendaTimelineEntries(data)
	width := 760.0
	height := 520.0
	if extra := len(entries) - 5; extra > 0 {
		height += float64(extra * 58)
	}
	cardW := width - 48
	lineX := 88.0
	startY := 150.0
	rowGap := 62.0
	title := firstNonEmpty(data.Title, lang.T("assistant.daily_agenda_package.timeline.default_title"))
	rangeText := fmt.Sprintf("%s ~ %s", data.Start.Format("2006-01-02"), data.End.Add(-time.Second).Format("2006-01-02"))

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f">`, width, height, width, height))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`<rect width="%.0f" height="%.0f" fill="#f8fafc"/>`, width, height))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`<rect x="24" y="24" width="%.0f" height="%.0f" rx="20" fill="#ffffff" stroke="#d9e2ef"/>`, cardW, height-48))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`<text x="48" y="66" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="27" font-weight="800" fill="#0f172a">%s</text>`, html.EscapeString(title)))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`<text x="48" y="96" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="15" fill="#64748b">%s · %s</text>`, html.EscapeString(lang.T("assistant.daily_agenda_package.timeline.subtitle", len(data.Events), len(data.Reminders))), html.EscapeString(rangeText)))
	b.WriteString("\n")
	if len(entries) > 0 {
		b.WriteString(fmt.Sprintf(`<line x1="%.0f" y1="%.0f" x2="%.0f" y2="%.0f" stroke="#cbd5e1" stroke-width="4" stroke-linecap="round"/>`, lineX, startY-18, lineX, startY+rowGap*float64(len(entries)-1)+18))
		b.WriteString("\n")
	}
	for i, entry := range entries {
		y := startY + float64(i)*rowGap
		color := firstNonEmpty(entry.Color, "#2563eb")
		b.WriteString(fmt.Sprintf(`<circle cx="%.0f" cy="%.0f" r="13" fill="%s"/>`, lineX, y, html.EscapeString(color)))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf(`<text x="122" y="%.0f" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="15" font-weight="750" fill="%s">%s</text>`, y-15, html.EscapeString(color), html.EscapeString(entry.When)))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf(`<text x="122" y="%.0f" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="20" font-weight="800" fill="#111827">%s</text>`, y+9, html.EscapeString(shortenSVGText(entry.Title, 42))))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf(`<text x="122" y="%.0f" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="14" fill="#64748b">%s</text>`, y+31, html.EscapeString(shortenSVGText(entry.Detail, 72))))
		b.WriteString("\n")
	}
	b.WriteString(fmt.Sprintf(`<text x="48" y="%.0f" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="15" fill="#475569">%s</text>`, height-60, html.EscapeString(lang.T("assistant.daily_agenda_package.timeline.footer"))))
	b.WriteString("\n")
	b.WriteString("</svg>\n")
	return b.String()
}

func assistantDailyAgendaTimelineEntries(data assistantDailyAgendaData) []assistantDailyAgendaTimelineEntry {
	entries := make([]assistantDailyAgendaTimelineEntry, 0, len(data.Events)+len(data.Reminders)+1)
	for _, event := range data.Events {
		start := parseCalendarEventTime(event.Start)
		detail := strings.TrimSpace(firstNonEmpty(event.Location, event.Calendar, "Calendar"))
		entries = append(entries, assistantDailyAgendaTimelineEntry{
			Kind:   "event",
			When:   firstNonEmpty(assistantDailyAgendaTimeRange(event.Start, event.End), lang.T("assistant.daily_agenda_package.timeline.time_unknown")),
			Title:  firstNonEmpty(event.Title, lang.T("assistant.daily_agenda_package.untitled")),
			Detail: detail,
			Time:   start,
			Color:  "#2563eb",
		})
	}
	for _, reminder := range data.Reminders {
		due := parseCalendarEventTime(reminder.Due)
		detail := strings.TrimSpace(firstNonEmpty(reminder.Calendar, reminder.Notes, "Reminders"))
		entries = append(entries, assistantDailyAgendaTimelineEntry{
			Kind:   "reminder",
			When:   firstNonEmpty(assistantDailyAgendaTimeRange(reminder.Due, ""), lang.T("assistant.daily_agenda_package.timeline.due_unknown")),
			Title:  firstNonEmpty(reminder.Title, lang.T("assistant.daily_agenda_package.untitled")),
			Detail: detail,
			Time:   due,
			Color:  "#0f766e",
		})
	}
	if data.MailRequested {
		base := data.Start.Add(11 * time.Hour)
		for i, item := range assistantDailyAgendaMailTopItems(data.MailItems, 3) {
			entries = append(entries, assistantDailyAgendaTimelineEntry{
				Kind:   "mail",
				When:   lang.T("assistant.daily_agenda_package.timeline.mail_time"),
				Title:  cleanSignalMailHeaderForDisplay(item.Message.Subject, lang.T("assistant.mail_priority_package.subject_unknown")),
				Detail: firstNonEmpty(item.Action, item.Reason),
				Time:   base.Add(time.Duration(i) * time.Minute),
				Color:  "#d97706",
			})
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		left := entries[i].Time
		right := entries[j].Time
		if left.IsZero() && right.IsZero() {
			return entries[i].Kind < entries[j].Kind
		}
		if left.IsZero() {
			return false
		}
		if right.IsZero() {
			return true
		}
		return left.Before(right)
	})
	if len(entries) == 0 {
		return []assistantDailyAgendaTimelineEntry{
			{
				Kind:   "focus",
				When:   data.Start.Format("2006-01-02"),
				Title:  lang.T("assistant.daily_agenda_package.timeline.empty_title"),
				Detail: lang.T("assistant.daily_agenda_package.timeline.empty_detail"),
				Time:   data.Start,
				Color:  "#7c3aed",
			},
		}
	}
	if len(entries) > 8 {
		return entries[:8]
	}
	return entries
}

func shortenSVGText(value string, limit int) string {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	if limit <= 1 {
		return string(runes[:limit])
	}
	return string(runes[:limit-1]) + "…"
}

func assistantDailyAgendaInlinePlan(data assistantDailyAgendaData) []string {
	lines := []string{
		"- " + lang.T("assistant.daily_agenda_package.inline.counts", len(data.Events), len(data.Reminders)),
	}
	if len(data.Events) > 0 {
		lines = append(lines, "- "+lang.T("assistant.daily_agenda_package.inline.first_event", assistantDailyAgendaEventLine(data.Events[0])))
	} else {
		lines = append(lines, "- "+lang.T("assistant.daily_agenda_package.inline.no_events"))
	}
	if len(data.Reminders) > 0 {
		lines = append(lines, "- "+lang.T("assistant.daily_agenda_package.inline.first_task", assistantDailyAgendaReminderLine(data.Reminders[0])))
	} else {
		lines = append(lines, "- "+lang.T("assistant.daily_agenda_package.inline.no_tasks"))
	}
	if data.MailRequested {
		if mailItem, ok := assistantDailyAgendaFirstMailItem(data.MailItems); ok {
			lines = append(lines, "- "+lang.T("assistant.daily_agenda_package.inline.first_mail", assistantDailyAgendaMailLine(mailItem)))
		} else {
			lines = append(lines, "- "+lang.T("assistant.daily_agenda_package.inline.no_mail"))
		}
	}
	lines = append(lines, "- "+lang.T("assistant.daily_agenda_package.inline.boundary"))
	return lines
}

func assistantDailyAgendaApprovalDraftLines() []string {
	return []string{
		"- " + lang.T("assistant.daily_agenda_package.approval_drafts.calendar"),
		"- " + lang.T("assistant.daily_agenda_package.approval_drafts.reminder"),
		"- " + lang.T("assistant.daily_agenda_package.approval_drafts.boundary"),
	}
}

func assistantDailyAgendaEventBullets(events []calendarListEvent) []string {
	if len(events) == 0 {
		return []string{"- " + lang.T("assistant.daily_agenda_package.report.no_events")}
	}
	lines := []string{}
	for i, event := range events {
		if i >= 6 {
			lines = append(lines, "- "+lang.T("assistant.daily_agenda_package.more", len(events)-i))
			break
		}
		lines = append(lines, "- "+assistantDailyAgendaEventLine(event))
	}
	return lines
}

func assistantDailyAgendaReminderBullets(reminders []assistantDailyAgendaReminder) []string {
	if len(reminders) == 0 {
		return []string{"- " + lang.T("assistant.daily_agenda_package.report.no_tasks")}
	}
	lines := []string{}
	for i, reminder := range reminders {
		if i >= 6 {
			lines = append(lines, "- "+lang.T("assistant.daily_agenda_package.more", len(reminders)-i))
			break
		}
		lines = append(lines, "- "+assistantDailyAgendaReminderLine(reminder))
	}
	return lines
}

func assistantDailyAgendaMailBullets(items []signalMailPriorityItem) []string {
	top := assistantDailyAgendaMailTopItems(items, 6)
	if len(top) == 0 {
		return []string{"- " + lang.T("assistant.daily_agenda_package.report.no_mail")}
	}
	lines := []string{}
	for _, item := range top {
		lines = append(lines, "- "+assistantDailyAgendaMailLine(item))
	}
	return lines
}

func assistantDailyAgendaMailTopItems(items []signalMailPriorityItem, limit int) []signalMailPriorityItem {
	top := mailPriorityActionableItems(items)
	if len(top) == 0 {
		top = items
	}
	if limit <= 0 || limit > len(top) {
		limit = len(top)
	}
	if limit == 0 {
		return nil
	}
	return append([]signalMailPriorityItem{}, top[:limit]...)
}

func assistantDailyAgendaFirstMailItem(items []signalMailPriorityItem) (signalMailPriorityItem, bool) {
	top := assistantDailyAgendaMailTopItems(items, 1)
	if len(top) == 0 {
		return signalMailPriorityItem{}, false
	}
	return top[0], true
}

func assistantDailyAgendaMailLine(item signalMailPriorityItem) string {
	message := item.Message
	subject := cleanSignalMailHeaderForDisplay(message.Subject, lang.T("assistant.mail_priority_package.subject_unknown"))
	from := cleanSignalMailHeaderForDisplay(message.From, lang.T("assistant.mail_priority_package.from_unknown"))
	bucket := assistantMailPriorityLocalizedBucket(item.Bucket)
	action := firstNonEmpty(item.Action, lang.T("assistant.mail_priority.classify.action.review"))
	line := fmt.Sprintf("[%s] %s", bucket, subject)
	if from != "" {
		line += " | " + from
	}
	if action != "" {
		line += " | " + action
	}
	return line
}

func assistantDailyAgendaEventLine(event calendarListEvent) string {
	line := firstNonEmpty(event.Title, lang.T("assistant.daily_agenda_package.untitled"))
	if when := assistantDailyAgendaTimeRange(event.Start, event.End); strings.TrimSpace(when) != "" {
		line += " | " + when
	}
	if event.Location != "" {
		line += " | " + event.Location
	}
	return line
}

func assistantDailyAgendaReminderLine(reminder assistantDailyAgendaReminder) string {
	line := firstNonEmpty(reminder.Title, lang.T("assistant.daily_agenda_package.untitled"))
	if when := assistantDailyAgendaTimeRange(reminder.Due, ""); strings.TrimSpace(when) != "" {
		line += " | " + when
	}
	return line
}

func assistantDailyAgendaTimeRange(startValue, endValue string) string {
	if lang.Current() != "en" {
		return formatSignalKoreanTimeRange(startValue, endValue)
	}
	start := parseCalendarEventTime(startValue)
	end := parseCalendarEventTime(endValue)
	if start.IsZero() && end.IsZero() {
		return strings.TrimSpace(strings.TrimSpace(startValue) + " ~ " + strings.TrimSpace(endValue))
	}
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		loc = time.FixedZone("KST", 9*60*60)
	}
	if !start.IsZero() {
		start = start.In(loc)
	}
	if !end.IsZero() {
		end = end.In(loc)
	}
	if !start.IsZero() && !end.IsZero() {
		if start.Format("2006-01-02") == end.Format("2006-01-02") {
			return start.Format("2006-01-02 15:04") + " ~ " + end.Format("15:04")
		}
		return start.Format("2006-01-02 15:04") + " ~ " + end.Format("2006-01-02 15:04")
	}
	if !start.IsZero() {
		return start.Format("2006-01-02 15:04")
	}
	return lang.T("assistant.daily_agenda_package.time.due_by", end.Format("2006-01-02 15:04"))
}
