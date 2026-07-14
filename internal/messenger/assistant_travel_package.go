package messenger

import (
	"context"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/meshclaw/meshclaw/internal/evidence"
	"github.com/meshclaw/meshclaw/internal/lang"
	"github.com/meshclaw/meshclaw/internal/osauto"
	"github.com/meshclaw/meshclaw/internal/publish"
	"github.com/meshclaw/meshclaw/internal/tts"
)

func assistantCalendarTravelPackageReply(opts ListenOptions, request string) (string, bool) {
	if !isAssistantCalendarTravelPackageRequest(request) {
		return "", false
	}
	destination := assistantTravelPackageDestination(request)
	title := lang.T("assistant.travel_package.title", destination)
	query := assistantTravelPackageQuery(destination, request)
	calendarPrompt := lang.T("assistant.travel_package.calendar_prompt")
	calendarReply := executeAssistantToolCall(opts, calendarPrompt, "run_mac_action", fmt.Sprintf(`{"prompt":%q}`, calendarPrompt))

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var report publish.ResearchReport
	var researchErr error
	researchDisabled := strings.TrimSpace(os.Getenv("MESHCLAW_ASSISTANT_RESEARCH_DISABLE")) != ""
	if !researchDisabled {
		report, researchErr = publish.Research(ctx, publish.ResearchOptions{Query: query, Limit: 8, Timeout: 18})
	} else {
		report.Query = query
	}

	calendarVisible := signalReplyVisibleText(calendarReply)
	calendarStatus := assistantTravelCalendarStatusLine(calendarVisible)
	searchStatus := assistantTravelSearchStatusLine(report)
	searchProvenance := assistantTravelSearchProvenanceLine(report, researchErr, researchDisabled)
	body := assistantTravelPackageReportBody(title, destination, request, query, calendarVisible, report, researchErr, researchDisabled)
	restorePreview := temporarilySkipPreviewImages()
	doc := osauto.CreateArgosDocument(ctx, lang.T("assistant.travel_package.doc_title", title), body)
	deck := osauto.CreatePresentation(ctx, lang.T("assistant.travel_package.deck_title", title), assistantTravelPackageDeckBody(destination, query, calendarVisible, report, researchErr, researchDisabled), lang.T("assistant.travel_package.deck_audience"), 7, "")
	sheet := osauto.CreateSpreadsheet(ctx, lang.T("assistant.travel_package.sheet_title", title), assistantTravelPackageSheetBody(destination, query, calendarVisible, report))
	timelinePath, timelineErr := writeAssistantTravelPlanTimelineSVG(title, destination, calendarVisible)
	restorePreview()

	wantsVoice := assistantBrowserResearchPackageWantsVoice(request)
	voiceScript := ""
	var audio tts.Result
	var audioErr error
	if wantsVoice {
		voiceScript = assistantTravelPackageVoiceScript(destination, query, calendarVisible, report, researchErr, researchDisabled)
		audio, audioErr = tts.Synthesize(tts.Options{
			Text:     voiceScript,
			Engine:   "edge-tts",
			Basename: destination + "-travel-brief-" + time.Now().UTC().Format("20060102T150405Z"),
		})
	}

	attachments := assistantTravelPackageAttachments(doc, deck, sheet, report)
	if strings.TrimSpace(timelinePath) != "" && timelineErr == nil {
		attachments = uniqueShowcaseAttachments(append(attachments, timelinePath))
	}
	if strings.TrimSpace(audio.Path) != "" && audioErr == nil {
		attachments = uniqueShowcaseAttachments(append(attachments, audio.Path))
	}
	rememberAssistantArtifact(opts, title, "calendar_travel_package", doc, deck, sheet)
	record, storeErr := evidence.Store("assistant-calendar-travel-package", firstNonEmpty(opts.TargetID, "assistant"), title, map[string]interface{}{
		"kind":              "assistant_calendar_travel_package",
		"request":           request,
		"destination":       destination,
		"query":             query,
		"calendar_visible":  calendarVisible,
		"calendar_status":   calendarStatus,
		"research":          report,
		"research_error":    errorString(researchErr),
		"research_disabled": researchDisabled,
		"search_status":     searchStatus,
		"search_provenance": searchProvenance,
		"voice_script":      voiceScript,
		"audio":             audio,
		"audio_error":       errorString(audioErr),
		"timeline":          timelinePath,
		"timeline_error":    errorString(timelineErr),
		"document":          doc,
		"presentation":      deck,
		"spreadsheet":       sheet,
		"attachments":       attachments,
		"created_at":        time.Now().UTC(),
	})

	lines := []string{
		lang.T("assistant.travel_package.created", title),
		lang.T("assistant.travel_package.subtitle"),
		lang.T("assistant.travel_package.intro"),
		"",
		lang.T("assistant.travel_package.destination", destination),
		lang.T("assistant.travel_package.query", query),
		calendarStatus,
		searchStatus,
		searchProvenance,
		"",
		lang.T("assistant.travel_package.files"),
	}
	if doc.OK && doc.Error == "" {
		lines = append(lines, "- "+lang.T("assistant.travel_package.file.doc"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.travel_package.file.doc_failed", firstNonEmpty(doc.Error, doc.Stderr, "unknown error")))
	}
	if deck.OK && deck.Error == "" {
		lines = append(lines, "- "+lang.T("assistant.travel_package.file.ppt"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.travel_package.file.ppt_failed", firstNonEmpty(deck.Error, deck.Stderr, "unknown error")))
	}
	if sheet.OK && sheet.Error == "" {
		lines = append(lines, "- "+lang.T("assistant.travel_package.file.sheet"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.travel_package.file.sheet_failed", firstNonEmpty(sheet.Error, sheet.Stderr, "unknown error")))
	}
	if strings.TrimSpace(timelinePath) != "" && timelineErr == nil {
		lines = append(lines, "- "+lang.T("assistant.travel_package.file.timeline"))
	} else if timelineErr != nil {
		lines = append(lines, "- "+lang.T("assistant.travel_package.file.timeline_failed", firstNonEmpty(errorString(timelineErr), "unknown error")))
	}
	if wantsVoice {
		if audioErr == nil && strings.TrimSpace(audio.Path) != "" {
			lines = append(lines, "- "+lang.T("assistant.travel_package.file.audio"))
		} else {
			lines = append(lines, "- "+lang.T("assistant.travel_package.file.audio_failed", firstNonEmpty(errorString(audioErr), "unknown error")))
		}
	}
	lines = append(lines,
		"",
		lang.T("assistant.travel_package.best_windows"),
	)
	lines = append(lines, assistantTravelAvailabilityCandidateLines(calendarVisible, 2)...)
	lines = append(lines,
		"",
		lang.T("assistant.travel_package.itinerary_title"),
		"- "+lang.T("assistant.travel_package.itinerary.day1"),
		"- "+lang.T("assistant.travel_package.itinerary.day2"),
		"- "+lang.T("assistant.travel_package.itinerary.day3"),
		"",
		lang.T("assistant.travel_package.criteria_title"),
		"- "+lang.T("assistant.travel_package.criteria.flight"),
		"- "+lang.T("assistant.travel_package.criteria.hotel"),
		"",
		lang.T("assistant.travel_package.decision_table_title"),
		"- "+lang.T("assistant.travel_package.decision.flight", query),
		"- "+lang.T("assistant.travel_package.decision.hotel", destination),
		"- "+lang.T("assistant.travel_package.decision.approval"),
		"",
		lang.T("assistant.travel_package.source_links_title"),
	)
	lines = append(lines, lang.T("assistant.travel_package.flight_links_title"))
	lines = append(lines, assistantTravelCandidateSourceLines(report, "flight", 3)...)
	lines = append(lines, lang.T("assistant.travel_package.hotel_links_title"))
	lines = append(lines, assistantTravelCandidateSourceLines(report, "hotel", 3)...)
	lines = append(lines, lang.T("assistant.travel_package.all_links_title"))
	for _, line := range assistantResearchSourceLines(report) {
		lines = append(lines, line)
		if strings.Count(strings.Join(lines, "\n"), "\n") > 80 {
			break
		}
	}
	lines = append(lines,
		"",
		lang.T("assistant.travel_package.preview"),
	)
	lines = append(lines, assistantTravelPackageSummary(calendarVisible, report, researchErr)...)
	lines = append(lines,
		"",
		lang.T("assistant.travel_package.companion_title"),
		assistantTravelCompanionMessageDraft(destination),
	)
	if strings.TrimSpace(voiceScript) != "" {
		lines = append(lines, "", lang.T("assistant.travel_package.voice_preview"))
		lines = append(lines, trimForContext(voiceScript, 520))
	}
	lines = append(lines, "", lang.T("assistant.travel_package.next"))
	lines = append(lines,
		"- "+lang.T("assistant.travel_package.next.narrow"),
		"- "+lang.T("assistant.travel_package.next.budget"),
		"- "+lang.T("assistant.travel_package.next.message"),
	)
	lines = append(lines, "", lang.T("assistant.travel_package.next_line"))
	if targetRef := inferAssistantSignalTargetRef(request); targetRef != "" {
		return formatAssistantTravelPackageSendResult(opts, targetRef, lines, attachments, record, storeErr), true
	}
	lines = appendAssistantWorkflowMobileLinkLines(lines, assistantWorkflowVisibleMobileLinkLines(attachments, 6))
	lines = appendVoiceReportAttachmentMarkers(lines, attachments)
	lines = appendAssistantEvidenceNote(lines, record, storeErr)
	return strings.Join(compactBlankLines(lines), "\n"), true
}

func formatAssistantTravelPackageSendResult(opts ListenOptions, targetRef string, summary []string, attachments []string, record evidence.Record, storeErr error) string {
	target, candidates, targetErr := resolveAssistantVoiceTarget(targetRef)
	if targetErr != nil {
		lines := []string{lang.T("assistant.travel_package.send_target_failed")}
		lines = append(lines, formatAssistantVoiceTargetCandidates(candidates)...)
		lines = append(lines, lang.T("assistant.travel_package.attach_here"))
		lines = appendAssistantWorkflowMobileLinkLines(lines, assistantWorkflowVisibleMobileLinkLines(attachments, 6))
		lines = appendVoiceReportAttachmentMarkers(lines, attachments)
		return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
	}
	targetLabel := assistantScheduleLocalizedTarget(target.ID, target.Label)
	if !opts.Execute {
		lines := []string{
			lang.T("assistant.travel_package.send_ready"),
			lang.T("assistant.travel_package.target", targetLabel),
		}
		if OneWayReportTarget(target) {
			lines = append(lines, lang.T("assistant.travel_package.one_way"))
		}
		lines = append(lines, "", lang.T("assistant.travel_package.to_send"))
		lines = append(lines, summary...)
		lines = append(lines, "", lang.T("assistant.travel_package.send_attachments"))
		lines = appendAssistantWorkflowMobileLinkLines(lines, assistantWorkflowVisibleMobileLinkLines(attachments, 6))
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
		"kind":             "assistant_calendar_travel_package_send",
		"target":           targetRef,
		"resolved_target":  target,
		"attachment_count": len(attachments),
		"mobile_links":     mobileLinkLines,
		"send":             send,
		"send_error":       errorString(sendErr),
		"created_at":       time.Now().UTC(),
	}
	sendRecord, sendStoreErr := evidence.Store("assistant-calendar-travel-package-send", firstNonEmpty(opts.TargetID, "assistant"), target.ID, payload)
	if sendErr != nil {
		lines := []string{
			lang.T("assistant.travel_package.send_failed"),
			lang.T("assistant.travel_package.target", targetLabel),
			lang.T("assistant.travel_package.problem", sendErr.Error()),
			lang.T("assistant.travel_package.attach_here"),
		}
		lines = appendAssistantWorkflowMobileLinkLines(lines, mobileLinkLines)
		lines = appendVoiceReportAttachmentMarkers(lines, attachments)
		return strings.Join(appendAssistantEvidenceNote(lines, sendRecord, sendStoreErr), "\n")
	}
	lines := []string{
		lang.T("assistant.travel_package.sent"),
		lang.T("assistant.travel_package.target", targetLabel),
	}
	if id := strings.TrimSpace(send.Stdout); id != "" {
		lines = append(lines, lang.T("assistant.workflow.signal_id", id))
	}
	return strings.Join(appendAssistantEvidenceNote(lines, sendRecord, sendStoreErr), "\n")
}

func isAssistantCalendarTravelPackageRequest(request string) bool {
	lower := strings.ToLower(strings.TrimSpace(request))
	if lower == "" {
		return false
	}
	if !containsAny(lower, "여행", "출장", "travel", "trip") {
		return false
	}
	timeSignal := containsAny(lower, "일주일", "7일", "이번 주", "다음 주", "가장 시간", "시간 많이", "가용시간", "캘린더", "calendar", "빈 시간", "next week", "this week", "availability", "open time", "free time", "available time")
	planSignal := containsAny(lower, "여행계획", "여행 계획", "여행 일정", "스케줄", "비행기", "항공", "호텔", "숙소", "후보", "ppt", "pptx", "표", "엑셀", "음성", "travel plan", "itinerary", "flight", "hotel", "candidate", "table", "spreadsheet", "voice", "deck")
	return timeSignal && planSignal
}

func assistantTravelPackageDestination(request string) string {
	lower := strings.ToLower(request)
	switch {
	case containsAny(lower, "후쿠오카", "fukuoka"):
		return lang.T("assistant.travel_package.destination.fukuoka")
	case containsAny(lower, "부산", "busan"):
		return lang.T("assistant.travel_package.destination.busan")
	case containsAny(lower, "도쿄", "tokyo"):
		return lang.T("assistant.travel_package.destination.tokyo")
	case containsAny(lower, "오사카", "osaka"):
		return lang.T("assistant.travel_package.destination.osaka")
	case containsAny(lower, "제주", "jeju"):
		return lang.T("assistant.travel_package.destination.jeju")
	default:
		return lang.T("assistant.travel_package.destination.jeju")
	}
}

func assistantTravelPackageQuery(destination, request string) string {
	lower := strings.ToLower(request)
	days := "2 nights 3 days"
	if containsAny(lower, "1박", "1 night") {
		days = "1 night 2 days"
	} else if containsAny(lower, "3박", "3 nights") {
		days = "3 nights 4 days"
	}
	return "Seoul " + destination + " flights hotels " + days + " next week fare free cancellation official booking"
}

func assistantTravelPackageReportBody(title, destination, request, query, calendarVisible string, report publish.ResearchReport, researchErr error, researchDisabled bool) string {
	now := time.Now().In(time.FixedZone("KST", 9*60*60))
	lines := []string{
		lang.T("assistant.travel_package.report.title", title),
		"",
		lang.T("assistant.travel_package.report.generated", now.Format("2006-01-02 15:04 KST")),
		lang.T("assistant.travel_package.report.request", strings.TrimSpace(request)),
		lang.T("assistant.travel_package.report.destination", destination),
		lang.T("assistant.travel_package.report.query", query),
		"- " + assistantTravelCalendarStatusLine(calendarVisible),
		"- " + assistantTravelSearchStatusLine(report),
		"- " + assistantTravelSearchProvenanceLine(report, researchErr, researchDisabled),
		"",
		lang.T("assistant.travel_package.report.conclusion"),
		"- " + lang.T("assistant.travel_package.report.conclusion.calendar"),
		"- " + lang.T("assistant.travel_package.report.conclusion.approval"),
		"",
		lang.T("assistant.travel_package.report.recommended_windows"),
	}
	lines = append(lines, assistantTravelAvailabilityCandidateLines(calendarVisible, 3)...)
	lines = append(lines, "", lang.T("assistant.travel_package.report.calendar_summary"))
	if calendar := assistantTravelCompactCalendarLines(calendarVisible, 8); len(calendar) > 0 {
		lines = append(lines, calendar...)
	} else {
		lines = append(lines, "- "+lang.T("assistant.travel_package.report.calendar_empty"))
	}
	lines = append(lines,
		"",
		lang.T("assistant.travel_package.report.criteria"),
		"- "+lang.T("assistant.travel_package.report.criteria.flight"),
		"- "+lang.T("assistant.travel_package.report.criteria.hotel"),
		"- "+lang.T("assistant.travel_package.report.criteria.route"),
		"",
		lang.T("assistant.travel_package.report.source_links"),
		"- "+assistantTravelSearchStatusLine(report),
		"- "+assistantTravelSearchProvenanceLine(report, researchErr, researchDisabled),
	)
	lines = append(lines, assistantResearchSourceLines(report)...)
	lines = append(lines,
		"",
		lang.T("assistant.travel_package.report.prebook"),
		"- "+lang.T("assistant.travel_package.report.prebook.id"),
		"- "+lang.T("assistant.travel_package.report.prebook.total"),
		"- "+lang.T("assistant.travel_package.report.prebook.approval"),
		"",
		lang.T("assistant.travel_package.report.companion"),
		assistantTravelCompanionMessageDraft(destination),
	)
	if researchErr != nil {
		lines = append(lines, "", lang.T("assistant.travel_package.report.search_status"), "- "+lang.T("assistant.travel_package.report.search_error", researchErr.Error()))
	}
	return strings.Join(lines, "\n")
}

func assistantTravelPackageDeckBody(destination, query, calendarVisible string, report publish.ResearchReport, researchErr error, researchDisabled bool) string {
	sourceLines := assistantResearchSourceLines(report)
	sourceSummary := "- " + lang.T("assistant.travel_package.deck.source_empty")
	if len(sourceLines) > 0 {
		limit := len(sourceLines)
		if limit > 3 {
			limit = 3
		}
		sourceSummary = strings.Join(sourceLines[:limit], "\n")
	}
	calendarSummary := strings.Join(assistantTravelAvailabilityCandidateLines(calendarVisible, 3), "\n")
	if strings.TrimSpace(calendarSummary) == "" {
		calendarSummary = "- " + lang.T("assistant.travel_package.deck.calendar_empty")
	}
	status := lang.T("assistant.travel_package.deck.status_ok")
	if researchErr != nil {
		status = lang.T("assistant.travel_package.deck.status_error", researchErr.Error())
	} else if researchDisabled {
		status = lang.T("assistant.travel_package.deck.status_disabled")
	}
	status = strings.Join([]string{
		assistantTravelCalendarStatusLine(calendarVisible),
		assistantTravelSearchStatusLine(report),
		assistantTravelSearchProvenanceLine(report, researchErr, researchDisabled),
		status,
	}, "\n")
	return strings.Join([]string{
		lang.T("assistant.travel_package.deck.destination"),
		lang.T("assistant.travel_package.deck.destination_value", destination),
		lang.T("assistant.travel_package.deck.availability"),
		calendarSummary,
		lang.T("assistant.travel_package.deck.recommended_window"),
		strings.Join(assistantTravelAvailabilityCandidateLines(calendarVisible, 2), "\n"),
		lang.T("assistant.travel_package.deck.search"),
		lang.T("assistant.travel_package.deck.search_query", query),
		lang.T("assistant.travel_package.deck.links"),
		sourceSummary,
		lang.T("assistant.travel_package.deck.booking_criteria"),
		lang.T("assistant.travel_package.deck.booking_criteria_body"),
		lang.T("assistant.travel_package.deck.boundary"),
		lang.T("assistant.travel_package.deck.boundary_body"),
		lang.T("assistant.travel_package.deck.status"),
		status,
	}, "\n\n")
}

func assistantTravelPackageSheetBody(destination, query, calendarVisible string, report publish.ResearchReport) string {
	candidate1, candidate2 := lang.T("assistant.travel_package.sheet.candidate1"), lang.T("assistant.travel_package.sheet.candidate2")
	candidates := assistantTravelAvailabilityCandidateLines(calendarVisible, 2)
	bullets := []string{}
	for _, candidate := range candidates {
		if strings.HasPrefix(candidate, "- ") {
			bullets = append(bullets, strings.TrimPrefix(candidate, "- "))
		}
	}
	if len(bullets) > 0 {
		candidate1 = bullets[0]
	}
	if len(bullets) > 1 {
		candidate2 = bullets[1]
	}
	lines := []string{
		lang.T("assistant.travel_package.sheet.table",
			cleanSpreadsheetCell(candidate1),
			cleanSpreadsheetCell(destination),
			cleanSpreadsheetCell(candidate2),
			cleanSpreadsheetCell(destination),
			cleanSpreadsheetCell(query),
			cleanSpreadsheetCell(query),
		),
		"",
		lang.T("assistant.travel_package.sheet.web_candidates"),
		lang.T("assistant.travel_package.flight_links_title"),
		strings.Join(assistantTravelCandidateSourceLines(report, "flight", 5), "\n"),
		"",
		lang.T("assistant.travel_package.hotel_links_title"),
		strings.Join(assistantTravelCandidateSourceLines(report, "hotel", 5), "\n"),
		"",
		lang.T("assistant.travel_package.all_links_title"),
		browserSearchSpreadsheetMarkdown(query, report.Search, report.SourcePages),
	}
	return strings.Join(lines, "\n")
}

func writeAssistantTravelPlanTimelineSVG(title, destination, calendarVisible string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Documents", "Argos Vault", "Charts")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	base := safeAssistantDepartmentFilename(title)
	path := filepath.Join(dir, base+"-"+time.Now().Format("20060102-150405")+"-timeline.svg")
	content := renderAssistantTravelPlanTimelineSVG(title, destination, calendarVisible)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return "", err
	}
	return path, nil
}

func renderAssistantTravelPlanTimelineSVG(title, destination, calendarVisible string) string {
	candidates := assistantTravelAvailabilityCandidateLines(calendarVisible, 2)
	candidateA := lang.T("assistant.travel_package.availability_fallback")
	candidateB := lang.T("assistant.travel_package.availability_unavailable")
	bullets := []string{}
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(strings.TrimPrefix(candidate, "- "))
		if strings.TrimSpace(candidate) == "" || strings.Contains(candidate, lang.T("assistant.travel_package.availability_title")) {
			continue
		}
		bullets = append(bullets, candidate)
	}
	if len(bullets) > 0 {
		candidateA = bullets[0]
	}
	if len(bullets) > 1 {
		candidateB = bullets[1]
	}

	width := 780
	height := 560
	cardW := 220
	cardY := 176
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`, width, height, width, height))
	b.WriteString("\n")
	b.WriteString(`<rect width="780" height="560" fill="#f8fafc"/>` + "\n")
	b.WriteString(`<rect x="24" y="24" width="732" height="512" rx="20" fill="#ffffff" stroke="#d9e2ef"/>` + "\n")
	b.WriteString(fmt.Sprintf(`<text x="48" y="66" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="27" font-weight="800" fill="#0f172a">%s</text>`, html.EscapeString(shortenSVGText(title, 36))))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`<text x="48" y="96" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="15" fill="#64748b">%s · %s</text>`, html.EscapeString(lang.T("assistant.travel_package.timeline.subtitle")), html.EscapeString(destination)))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`<text x="48" y="132" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="15" font-weight="750" fill="#2563eb">%s</text>`, html.EscapeString(lang.T("assistant.travel_package.timeline.option1", shortenSVGText(candidateA, 64)))))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`<text x="48" y="156" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="15" font-weight="650" fill="#64748b">%s</text>`, html.EscapeString(lang.T("assistant.travel_package.timeline.option2", shortenSVGText(candidateB, 64)))))
	b.WriteString("\n")

	stops := []struct {
		Day    string
		Title  string
		Detail string
		Color  string
	}{
		{lang.T("assistant.travel_package.timeline.day1"), lang.T("assistant.travel_package.timeline.day1.title"), lang.T("assistant.travel_package.timeline.day1.detail"), "#2563eb"},
		{lang.T("assistant.travel_package.timeline.day2"), lang.T("assistant.travel_package.timeline.day2.title"), lang.T("assistant.travel_package.timeline.day2.detail"), "#0f766e"},
		{lang.T("assistant.travel_package.timeline.day3"), lang.T("assistant.travel_package.timeline.day3.title"), lang.T("assistant.travel_package.timeline.day3.detail"), "#d97706"},
	}
	for i, stop := range stops {
		x := 48 + i*(cardW+18)
		b.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="%d" height="164" rx="16" fill="#f8fafc" stroke="#d9e2ef"/>`, x, cardY, cardW))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf(`<circle cx="%d" cy="%d" r="18" fill="%s"/>`, x+28, cardY+32, html.EscapeString(stop.Color)))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf(`<text x="%d" y="%d" text-anchor="middle" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="14" font-weight="800" fill="#ffffff">%d</text>`, x+28, cardY+37, i+1))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf(`<text x="%d" y="%d" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="15" font-weight="750" fill="%s">%s</text>`, x+56, cardY+29, html.EscapeString(stop.Color), html.EscapeString(stop.Day)))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf(`<text x="%d" y="%d" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="19" font-weight="800" fill="#0f172a">%s</text>`, x+20, cardY+74, html.EscapeString(stop.Title)))
		b.WriteString("\n")
		for j, line := range wrapSVGText(stop.Detail, 22, 3) {
			b.WriteString(fmt.Sprintf(`<text x="%d" y="%d" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="13" fill="#475569">%s</text>`, x+20, cardY+104+j*21, html.EscapeString(line)))
			b.WriteString("\n")
		}
	}

	checkY := 392
	b.WriteString(`<rect x="48" y="370" width="684" height="126" rx="16" fill="#fff7ed" stroke="#fed7aa"/>` + "\n")
	b.WriteString(fmt.Sprintf(`<text x="72" y="%d" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="18" font-weight="800" fill="#9a3412">%s</text>`, checkY, html.EscapeString(lang.T("assistant.travel_package.timeline.check_title"))))
	b.WriteString("\n")
	checks := []string{
		lang.T("assistant.travel_package.timeline.check.flight"),
		lang.T("assistant.travel_package.timeline.check.hotel"),
		lang.T("assistant.travel_package.timeline.check.approval"),
	}
	for i, check := range checks {
		y := checkY + 28 + i*24
		b.WriteString(fmt.Sprintf(`<text x="74" y="%d" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="14" fill="#7c2d12">• %s</text>`, y, html.EscapeString(check)))
		b.WriteString("\n")
	}
	b.WriteString(fmt.Sprintf(`<text x="48" y="522" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="14" fill="#64748b">%s</text>`, html.EscapeString(lang.T("assistant.travel_package.timeline.footer"))))
	b.WriteString("\n</svg>\n")
	return b.String()
}

func wrapSVGText(value string, limit, maxLines int) []string {
	words := strings.Fields(value)
	if len(words) == 0 || limit <= 0 || maxLines <= 0 {
		return nil
	}
	lines := []string{}
	current := ""
	for _, word := range words {
		next := strings.TrimSpace(current + " " + word)
		if len([]rune(next)) > limit && current != "" {
			lines = append(lines, current)
			current = word
			if len(lines) >= maxLines {
				break
			}
			continue
		}
		current = next
	}
	if len(lines) < maxLines && strings.TrimSpace(current) != "" {
		lines = append(lines, current)
	}
	if len(lines) == maxLines {
		last := []rune(lines[len(lines)-1])
		if len(last) > limit-1 {
			lines[len(lines)-1] = string(last[:limit-1]) + "…"
		}
	}
	return lines
}

func assistantTravelPackageSummary(calendarVisible string, report publish.ResearchReport, researchErr error) []string {
	lines := assistantTravelAvailabilityCandidateLines(calendarVisible, 2)
	lines = append(lines,
		"- "+lang.T("assistant.travel_package.summary.stop"),
	)
	if calendar := assistantTravelCompactCalendarLines(calendarVisible, 2); len(calendar) > 0 {
		lines = append(lines, calendar...)
	}
	sourceLines := assistantResearchSourceLines(report)
	for i, line := range sourceLines {
		if i >= 3 {
			break
		}
		lines = append(lines, "- "+strings.TrimPrefix(line, "- "))
	}
	if researchErr != nil {
		lines = append(lines, "- "+lang.T("assistant.travel_package.summary.search_error"))
	}
	return lines
}

func assistantTravelCompanionMessageDraft(destination string) string {
	return lang.T("assistant.travel_package.companion_body", destination)
}

func assistantTravelCalendarStatusLine(calendarVisible string) string {
	if count := len(parseAssistantTravelCalendarVisibleEvents(calendarVisible)); count > 0 {
		return lang.T("assistant.travel_package.calendar_status.used_count", count)
	}
	lower := strings.ToLower(strings.TrimSpace(signalReplyVisibleText(calendarVisible)))
	switch {
	case lower == "":
		return lang.T("assistant.travel_package.calendar_status.unavailable")
	case containsAny(lower, "실패", "권한", "permission", "denied", "not authorized", "helper is not available", "not available", "error"):
		return lang.T("assistant.travel_package.calendar_status.needs_review")
	case containsAny(lower, "일정을 확인", "일정 ", "calendar", "event"):
		return lang.T("assistant.travel_package.calendar_status.used")
	default:
		return lang.T("assistant.travel_package.calendar_status.draft")
	}
}

func assistantTravelVoiceCalendarStatus(calendarVisible string) string {
	if count := len(parseAssistantTravelCalendarVisibleEvents(calendarVisible)); count > 0 {
		return lang.T("assistant.travel_package.voice.calendar_status.used_count", count)
	}
	lower := strings.ToLower(strings.TrimSpace(signalReplyVisibleText(calendarVisible)))
	switch {
	case lower == "":
		return lang.T("assistant.travel_package.voice.calendar_status.unavailable")
	case containsAny(lower, "실패", "권한", "permission", "denied", "not authorized", "helper is not available", "not available", "error"):
		return lang.T("assistant.travel_package.voice.calendar_status.needs_review")
	case containsAny(lower, "일정을 확인", "일정 ", "calendar", "event"):
		return lang.T("assistant.travel_package.voice.calendar_status.used")
	default:
		return lang.T("assistant.travel_package.voice.calendar_status.draft")
	}
}

func assistantTravelSearchStatusLine(report publish.ResearchReport) string {
	stats := assistantBrowserResearchSourceStatsFor(report)
	return lang.T("assistant.travel_package.search_status", stats.SearchCandidates, stats.PagesRead, stats.OfficialCandidates, stats.ReadFailures)
}

func assistantTravelSearchProvenanceLine(report publish.ResearchReport, researchErr error, researchDisabled bool) string {
	if researchDisabled {
		return lang.T("assistant.travel_package.search_provenance.disabled")
	}
	if researchErr != nil {
		return lang.T("assistant.travel_package.search_provenance.partial")
	}
	stats := assistantBrowserResearchSourceStatsFor(report)
	if stats.PagesRead == 0 {
		return lang.T("assistant.travel_package.search_provenance.thin")
	}
	return lang.T("assistant.travel_package.search_provenance.live")
}

func assistantTravelVoiceSearchStatus(report publish.ResearchReport) string {
	stats := assistantBrowserResearchSourceStatsFor(report)
	return lang.T("assistant.travel_package.voice.search_status", stats.SearchCandidates, stats.PagesRead, stats.OfficialCandidates, stats.ReadFailures)
}

func assistantTravelVoiceSearchProvenance(report publish.ResearchReport, researchErr error, researchDisabled bool) string {
	if researchDisabled {
		return lang.T("assistant.travel_package.voice.search_provenance.disabled")
	}
	if researchErr != nil {
		return lang.T("assistant.travel_package.voice.search_provenance.partial")
	}
	stats := assistantBrowserResearchSourceStatsFor(report)
	if stats.PagesRead == 0 {
		return lang.T("assistant.travel_package.voice.search_provenance.thin")
	}
	return lang.T("assistant.travel_package.voice.search_provenance.live")
}

func assistantTravelCandidateSourceLines(report publish.ResearchReport, kind string, limit int) []string {
	if limit <= 0 {
		limit = 3
	}
	lines := []string{}
	seen := map[string]bool{}
	for _, item := range report.Search.Results {
		title := strings.TrimSpace(item.Text)
		rawURL := strings.TrimSpace(item.URL)
		if title == "" {
			title = lang.T("assistant.research_package.source.untitled")
		}
		if !assistantTravelSourceMatches(kind, title, rawURL) {
			continue
		}
		key := strings.ToLower(firstNonEmpty(rawURL, title))
		if seen[key] {
			continue
		}
		seen[key] = true
		if rawURL != "" {
			lines = append(lines, fmt.Sprintf("%d. %s - %s", len(lines)+1, title, rawURL))
		} else {
			lines = append(lines, fmt.Sprintf("%d. %s", len(lines)+1, title))
		}
		if len(lines) >= limit {
			break
		}
	}
	if len(lines) == 0 {
		return []string{"- " + lang.T("assistant.travel_package.links.none")}
	}
	return lines
}

func assistantTravelSourceMatches(kind, title, rawURL string) bool {
	host := strings.ToLower(searchResultHost(rawURL))
	haystack := strings.ToLower(strings.Join([]string{title, rawURL, host}, " "))
	flightDomain := containsAny(host,
		"skyscanner.", "kayak.", "google.", "koreanair.", "flyasiana.", "asiana.", "jejuair.", "jinair.", "twayair.", "airseoul.", "airbusan.",
	)
	hotelDomain := containsAny(host,
		"booking.com", "agoda.", "hotels.com", "hotel.", "airbnb.", "trivago.",
	)
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "flight":
		if flightDomain {
			return true
		}
		if hotelDomain {
			return false
		}
		return containsAny(haystack,
			"flight", "flights", "airfare", "air ticket", "airline", "skyscanner", "kayak", "google.com/travel/flights",
			"koreanair", "asiana", "jejuair", "jinair", "tway", "airseoul", "대한항공", "아시아나", "제주항공", "진에어", "티웨이", "항공", "항공권", "비행기",
		)
	case "hotel":
		if hotelDomain {
			return true
		}
		if flightDomain && !containsAny(haystack, "hotel", "hotels", "lodging", "accommodation", "resort", "호텔", "숙소", "리조트") {
			return false
		}
		return containsAny(haystack,
			"hotel", "hotels", "booking.com", "agoda", "lodging", "accommodation", "resort", "trip.com", "airbnb",
			"호텔", "숙소", "리조트", "게스트하우스", "펜션",
		)
	default:
		return false
	}
}

func assistantTravelPackageVoiceScript(destination, query, calendarVisible string, report publish.ResearchReport, researchErr error, researchDisabled bool) string {
	lines := []string{
		lang.T("assistant.travel_package.voice.intro", destination),
		assistantTravelVoiceCalendarStatus(calendarVisible),
		assistantTravelVoiceSearchStatus(report),
		assistantTravelVoiceSearchProvenance(report, researchErr, researchDisabled),
	}
	if candidates := assistantTravelAvailabilityCandidateLines(calendarVisible, 2); len(candidates) > 1 {
		lines = append(lines, lang.T("assistant.travel_package.voice.recommendation", stripMarkdownBulletForSpeech(candidates[1])))
	}
	if calendar := assistantTravelCompactCalendarLines(calendarVisible, 2); len(calendar) > 0 {
		lines = append(lines, lang.T("assistant.travel_package.voice.calendar", strings.TrimPrefix(strings.Join(calendar, " "), "- ")))
	}
	lines = append(lines, lang.T("assistant.travel_package.voice.search", query))
	sourceLines := assistantResearchSourceLines(report)
	if len(sourceLines) > 0 && !assistantTravelInsufficientSourceLine(sourceLines[0]) {
		lines = append(lines, lang.T("assistant.travel_package.voice.candidates_intro"))
		for i, line := range sourceLines {
			if i >= 3 {
				break
			}
			line = strings.TrimSpace(strings.TrimLeft(strings.TrimPrefix(line, "- "), "1234567890. "))
			line = stripURLsForSpeech(line)
			if line != "" {
				lines = append(lines, line+".")
			}
		}
	}
	lines = append(lines,
		lang.T("assistant.travel_package.voice.prebook"),
		lang.T("assistant.travel_package.voice.attachments"),
	)
	if researchErr != nil {
		lines = append(lines, lang.T("assistant.travel_package.voice.error"))
	}
	lines = append(lines, lang.T("assistant.travel_package.voice.closing"))
	return strings.Join(lines, "\n\n")
}

func assistantTravelCompactCalendarLines(calendarVisible string, limit int) []string {
	lines := compactCalendarSignalLines(calendarVisible, limit)
	if lang.Current() != "en" {
		return lines
	}
	filtered := []string{}
	for _, line := range lines {
		if assistantTravelHasHangul(line) {
			if containsAny(strings.ToLower(line), "캘린더", "조회", "권한") {
				filtered = append(filtered, "- "+lang.T("assistant.travel_package.report.calendar_empty"))
			}
			continue
		}
		filtered = append(filtered, line)
	}
	return filtered
}

func assistantTravelInsufficientSourceLine(line string) bool {
	lower := strings.ToLower(line)
	return containsAny(lower, "검색 결과가 충분하지", "검색 결과가 부족", "not enough", "insufficient", "no search results", "search results were thin", "verification checklist", "decision questions")
}

func assistantTravelHasHangul(value string) bool {
	for _, r := range value {
		if (r >= 0xAC00 && r <= 0xD7A3) || (r >= 0x1100 && r <= 0x11FF) || (r >= 0x3130 && r <= 0x318F) {
			return true
		}
	}
	return false
}

type assistantTravelCalendarEvent struct {
	Title string
	Start time.Time
	End   time.Time
}

type assistantTravelAvailabilityCandidate struct {
	Start     time.Time
	End       time.Time
	Free      time.Duration
	EventHits int
}

var assistantTravelCalendarVisibleLineRE = regexp.MustCompile(`^\d+\.\s+(.+?)\s+\|\s+(\d{4}-\d{2}-\d{2})\s+(\d{2}:\d{2})\s+~\s+(?:(\d{4}-\d{2}-\d{2})\s+)?(\d{2}:\d{2})`)

func assistantTravelAvailabilityCandidateLines(calendarVisible string, limit int) []string {
	return assistantTravelAvailabilityCandidateLinesAt(calendarVisible, time.Now(), limit)
}

func assistantTravelAvailabilityCandidateLinesAt(calendarVisible string, now time.Time, limit int) []string {
	if limit <= 0 {
		limit = 2
	}
	lower := strings.ToLower(signalReplyVisibleText(calendarVisible))
	if containsAny(lower, "실패", "권한", "permission", "helper is not available", "not available", "error") {
		return []string{"- " + lang.T("assistant.travel_package.availability_unavailable")}
	}
	candidates := rankAssistantTravelAvailabilityCandidates(parseAssistantTravelCalendarVisibleEvents(calendarVisible), now, 7)
	lines := []string{lang.T("assistant.travel_package.availability_title")}
	if len(candidates) == 0 {
		lines = append(lines, "- "+lang.T("assistant.travel_package.availability_fallback"))
		return lines
	}
	for i, candidate := range candidates {
		if i >= limit {
			break
		}
		conflicts := ""
		if candidate.EventHits > 0 {
			conflicts = lang.T("assistant.travel_package.availability_conflicts", candidate.EventHits)
		}
		lines = append(lines, "- "+lang.T("assistant.travel_package.availability_candidate", i+1, assistantTravelAvailabilityRangeLabel(candidate.Start, candidate.End), assistantTravelDurationLabel(candidate.Free), conflicts))
	}
	return lines
}

func parseAssistantTravelCalendarVisibleEvents(calendarVisible string) []assistantTravelCalendarEvent {
	visible := signalReplyVisibleText(calendarVisible)
	if strings.TrimSpace(visible) == "" {
		return nil
	}
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		loc = time.FixedZone("KST", 9*60*60)
	}
	events := []assistantTravelCalendarEvent{}
	for _, raw := range strings.Split(visible, "\n") {
		match := assistantTravelCalendarVisibleLineRE.FindStringSubmatch(strings.TrimSpace(raw))
		if len(match) != 6 {
			continue
		}
		endDate := strings.TrimSpace(match[4])
		if endDate == "" {
			endDate = match[2]
		}
		start, startErr := time.ParseInLocation("2006-01-02 15:04", match[2]+" "+match[3], loc)
		end, endErr := time.ParseInLocation("2006-01-02 15:04", endDate+" "+match[5], loc)
		if startErr != nil || endErr != nil {
			continue
		}
		if !end.After(start) {
			end = end.Add(24 * time.Hour)
		}
		events = append(events, assistantTravelCalendarEvent{Title: strings.TrimSpace(match[1]), Start: start, End: end})
	}
	return events
}

func rankAssistantTravelAvailabilityCandidates(events []assistantTravelCalendarEvent, now time.Time, days int) []assistantTravelAvailabilityCandidate {
	if days <= 0 {
		days = 7
	}
	loc, err := time.LoadLocation("Asia/Seoul")
	if err != nil {
		loc = time.FixedZone("KST", 9*60*60)
	}
	now = now.In(loc)
	base := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	candidates := []assistantTravelAvailabilityCandidate{}
	for i := 0; i < days; i++ {
		day := base.AddDate(0, 0, i)
		start := time.Date(day.Year(), day.Month(), day.Day(), 8, 0, 0, 0, loc)
		end := time.Date(day.Year(), day.Month(), day.Day(), 22, 0, 0, 0, loc)
		if i == 0 && now.After(start) {
			start = now
		}
		if !end.After(start.Add(90 * time.Minute)) {
			continue
		}
		busy := time.Duration(0)
		hits := 0
		for _, event := range events {
			overlap := overlapDuration(start, end, event.Start.In(loc), event.End.In(loc))
			if overlap <= 0 {
				continue
			}
			busy += overlap
			hits++
		}
		free := end.Sub(start) - busy
		if free < 0 {
			free = 0
		}
		candidates = append(candidates, assistantTravelAvailabilityCandidate{Start: start, End: end, Free: free, EventHits: hits})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Free != candidates[j].Free {
			return candidates[i].Free > candidates[j].Free
		}
		return candidates[i].Start.Before(candidates[j].Start)
	})
	if len(candidates) > 4 {
		return candidates[:4]
	}
	return candidates
}

func overlapDuration(aStart, aEnd, bStart, bEnd time.Time) time.Duration {
	start := aStart
	if bStart.After(start) {
		start = bStart
	}
	end := aEnd
	if bEnd.Before(end) {
		end = bEnd
	}
	if !end.After(start) {
		return 0
	}
	return end.Sub(start)
}

func assistantTravelAvailabilityRangeLabel(start, end time.Time) string {
	if lang.Current() == "en" {
		return lang.T("assistant.travel_package.availability_range", start.Format("Jan 2"), assistantTravelWeekday(start.Weekday()), start.Format("15:04"), end.Format("15:04"))
	}
	return lang.T("assistant.travel_package.availability_range", start.Format("1월 2일"), assistantTravelWeekday(start.Weekday()), start.Format("15:04"), end.Format("15:04"))
}

func assistantTravelWeekday(day time.Weekday) string {
	if lang.Current() == "en" {
		names := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
		return names[int(day)%len(names)]
	}
	names := []string{"일", "월", "화", "수", "목", "금", "토"}
	return names[int(day)%len(names)]
}

func assistantTravelDurationLabel(value time.Duration) string {
	if value < 0 {
		value = 0
	}
	hours := int(value.Hours())
	minutes := int(value.Minutes()) % 60
	if minutes == 0 {
		return lang.T("assistant.travel_package.duration.hours", hours)
	}
	return lang.T("assistant.travel_package.duration.hours_minutes", hours, minutes)
}

func stripMarkdownBulletForSpeech(line string) string {
	line = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "-"))
	line = strings.TrimSuffix(line, ".")
	return line
}

func assistantTravelPackageAttachments(doc, deck, sheet osauto.Result, report publish.ResearchReport) []string {
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
	attachments = append(attachments, assistantPublishResearchAttachments(report)...)
	return uniqueShowcaseAttachments(attachments)
}
