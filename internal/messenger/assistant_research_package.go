package messenger

import (
	"context"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/meshclaw/meshclaw/internal/evidence"
	"github.com/meshclaw/meshclaw/internal/lang"
	"github.com/meshclaw/meshclaw/internal/osauto"
	"github.com/meshclaw/meshclaw/internal/publish"
	"github.com/meshclaw/meshclaw/internal/tts"
)

func assistantBrowserResearchPackageReply(opts ListenOptions, request string) (string, bool) {
	if !isAssistantBrowserResearchPackageRequest(request) {
		return "", false
	}
	query := assistantBrowserResearchPackageQuery(request)
	title := assistantBrowserResearchPackageTitle(request, query)

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

	restorePreview := temporarilySkipPreviewImages()
	doc := osauto.CreateArgosDocument(ctx, lang.T("assistant.research_package.artifact.doc_title", title), assistantBrowserResearchReportBody(request, query, report, researchErr, researchDisabled))
	deck := osauto.CreatePresentation(ctx, lang.T("assistant.research_package.artifact.deck_title", title), assistantBrowserResearchDeckBody(request, query, report, researchErr, researchDisabled), lang.T("assistant.research_package.artifact.audience"), 7, "")
	sheet := osauto.CreateSpreadsheet(ctx, lang.T("assistant.research_package.artifact.sheet_title", title), assistantBrowserResearchSheetBody(query, report))
	sourceMapPath, sourceMapErr := writeAssistantBrowserResearchSourceMapSVG(title, query, report, researchErr)
	restorePreview()

	wantsVoice := assistantBrowserResearchPackageWantsVoice(request)
	voiceScript := ""
	var audio tts.Result
	var audioErr error
	if wantsVoice {
		voiceScript = assistantBrowserResearchVoiceScript(title, query, report, researchErr, researchDisabled)
		audio, audioErr = tts.Synthesize(tts.Options{
			Text:     voiceScript,
			Engine:   "edge-tts",
			Basename: title + "-voice-brief-" + time.Now().UTC().Format("20060102T150405Z"),
		})
	}

	attachments := assistantBrowserResearchPackageAttachments(doc, deck, sheet, report)
	if strings.TrimSpace(sourceMapPath) != "" && sourceMapErr == nil {
		attachments = uniqueShowcaseAttachments(append(attachments, sourceMapPath))
	}
	if strings.TrimSpace(audio.Path) != "" && audioErr == nil {
		attachments = uniqueShowcaseAttachments(append(attachments, audio.Path))
	}
	rememberAssistantArtifact(opts, title, "browser_research_package", doc, deck, sheet)
	record, storeErr := evidence.Store("assistant-browser-research-package", firstNonEmpty(opts.TargetID, "assistant"), title, map[string]interface{}{
		"kind":              "assistant_browser_research_package",
		"request":           request,
		"query":             query,
		"research":          report,
		"research_error":    errorString(researchErr),
		"research_disabled": researchDisabled,
		"voice_script":      voiceScript,
		"audio":             audio,
		"audio_error":       errorString(audioErr),
		"source_map":        sourceMapPath,
		"source_map_err":    errorString(sourceMapErr),
		"document":          doc,
		"presentation":      deck,
		"spreadsheet":       sheet,
		"attachments":       attachments,
		"created_at":        time.Now().UTC(),
	})

	lines := []string{
		lang.T("assistant.research_package.created", title),
		assistantBrowserResearchSubtitle(report, researchErr, researchDisabled),
		"",
		lang.T("assistant.research_package.query", query),
		assistantBrowserResearchSourceStatusLine(report),
		assistantBrowserResearchProvenanceLine(report, researchErr, researchDisabled),
		"",
		lang.T("assistant.research_package.report.conclusion"),
	}
	lines = append(lines, assistantBrowserResearchConclusionLines()...)
	lines = append(lines,
		"",
		lang.T("assistant.research_package.files"),
	)
	if doc.OK && doc.Error == "" {
		lines = append(lines, "- "+lang.T("assistant.research_package.file.doc"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.research_package.file.doc_failed", firstNonEmpty(doc.Error, doc.Stderr, "unknown error")))
	}
	if deck.OK && deck.Error == "" {
		lines = append(lines, "- "+lang.T("assistant.research_package.file.ppt"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.research_package.file.ppt_failed", firstNonEmpty(deck.Error, deck.Stderr, "unknown error")))
	}
	if sheet.OK && sheet.Error == "" {
		lines = append(lines, "- "+lang.T("assistant.research_package.file.sheet"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.research_package.file.sheet_failed", firstNonEmpty(sheet.Error, sheet.Stderr, "unknown error")))
	}
	if sourceMapErr == nil && strings.TrimSpace(sourceMapPath) != "" {
		lines = append(lines, "- "+lang.T("assistant.research_package.file.source_map"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.research_package.file.source_map_failed", firstNonEmpty(errorString(sourceMapErr), "unknown error")))
	}
	if wantsVoice {
		if audioErr == nil && strings.TrimSpace(audio.Path) != "" {
			lines = append(lines, "- "+lang.T("assistant.research_package.file.audio"))
		} else {
			lines = append(lines, "- "+lang.T("assistant.research_package.file.audio_failed", firstNonEmpty(errorString(audioErr), "unknown error")))
		}
	}
	lines = append(lines, "", lang.T("assistant.research_package.preview"))
	lines = append(lines, assistantBrowserResearchPackageSummary(report, researchErr)...)
	if sourceNotes := assistantBrowserResearchVisibleSourceNoteLines(report, 2); len(sourceNotes) > 0 {
		lines = append(lines, "", lang.T("assistant.research_package.source_notes_preview"))
		lines = append(lines, sourceNotes...)
	}
	lines = append(lines, "", lang.T("assistant.research_package.meeting_questions"))
	lines = append(lines, assistantBrowserResearchMeetingQuestionLines(request, query, 3)...)
	lines = append(lines, "", lang.T("assistant.research_package.decision_options"))
	lines = append(lines, assistantBrowserResearchDecisionOptionLines(request, query)...)
	lines = append(lines, "", lang.T("assistant.research_package.run_plan"))
	lines = append(lines, assistantBrowserResearchRunPlanLines()...)
	lines = append(lines, "", lang.T("assistant.research_package.followup_handoff"))
	lines = append(lines, assistantBrowserResearchFollowupHandoffLines()...)
	lines = append(lines, "", lang.T("assistant.research_package.report.next_actions"))
	lines = append(lines, assistantBrowserResearchActionLines()...)
	if strings.TrimSpace(voiceScript) != "" {
		lines = append(lines, "", lang.T("assistant.research_package.voice_preview"))
		lines = append(lines, trimForContext(voiceScript, 520))
	}
	lines = append(lines, "", lang.T("assistant.research_package.next"))
	lines = append(lines,
		"- "+lang.T("assistant.research_package.next.executive"),
		"- "+lang.T("assistant.research_package.next.official"),
		"- "+lang.T("assistant.research_package.next.voice"),
	)
	if targetRef := inferAssistantSignalTargetRef(request); targetRef != "" && assistantBrowserResearchHasExplicitSignalTarget(request) {
		return formatAssistantBrowserResearchPackageSendResult(opts, targetRef, lines, attachments, record, storeErr), true
	}
	lines = appendAssistantWorkflowMobileLinkLines(lines, assistantWorkflowVisibleMobileLinkLines(attachments, 6))
	lines = appendVoiceReportAttachmentMarkers(lines, attachments)
	lines = appendAssistantEvidenceNote(lines, record, storeErr)
	return strings.Join(compactBlankLines(lines), "\n"), true
}

func assistantBrowserResearchHasExplicitSignalTarget(request string) bool {
	lower := strings.ToLower(strings.TrimSpace(request))
	return containsAny(lower,
		"보고방", "브리핑방", "비서방", "채팅방",
		"argos-briefing", "argos-assistant", "argos-chat", "argos-ops",
		"내게", "나에게", "나한테", "내 signal", "내 시그널",
		"briefing room", "assistant room", "chat room", "ops room", "send to signal", "send it to signal",
	)
}

func formatAssistantBrowserResearchPackageSendResult(opts ListenOptions, targetRef string, summary []string, attachments []string, record evidence.Record, storeErr error) string {
	target, candidates, targetErr := resolveAssistantVoiceTarget(targetRef)
	if targetErr != nil {
		lines := []string{lang.T("assistant.research_package.send_target_failed")}
		lines = append(lines, formatAssistantVoiceTargetCandidates(candidates)...)
		lines = append(lines, "", lang.T("assistant.research_package.attach_here"))
		lines = appendAssistantWorkflowMobileLinkLines(lines, assistantWorkflowVisibleMobileLinkLines(attachments, 6))
		lines = appendVoiceReportAttachmentMarkers(lines, attachments)
		return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
	}
	targetLabel := assistantScheduleLocalizedTarget(target.ID, target.Label)
	if !opts.Execute {
		lines := []string{
			lang.T("assistant.research_package.send_ready"),
			lang.T("assistant.research_package.target", targetLabel),
		}
		if OneWayReportTarget(target) {
			lines = append(lines, lang.T("assistant.research_package.one_way"))
		}
		lines = append(lines, "", lang.T("assistant.research_package.to_send"))
		lines = append(lines, summary...)
		lines = appendAssistantWorkflowMobileLinkLines(lines, assistantWorkflowVisibleMobileLinkLines(attachments, 6))
		lines = appendVoiceReportAttachmentMarkers(lines, attachments)
		return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
	}
	mobileLinkLines := assistantWorkflowMobileLinkLines(attachments, 6)
	sendSummary := appendAssistantWorkflowMobileLinkLines(summary, mobileLinkLines)
	sendText := strings.Join(compactBlankLines(sendSummary), "\n")
	send, sendErr := Send(SendOptions{TargetID: target.ID, Kind: "text", Text: sendText, Attachments: attachments, Execute: true, TimeoutSeconds: 90})
	payload := map[string]interface{}{
		"kind":             "assistant_browser_research_package_send",
		"target":           targetRef,
		"resolved_target":  target,
		"attachment_count": len(attachments),
		"mobile_links":     mobileLinkLines,
		"send":             send,
		"send_error":       errorString(sendErr),
		"created_at":       time.Now().UTC(),
	}
	sendRecord, sendStoreErr := evidence.Store("assistant-browser-research-package-send", firstNonEmpty(opts.TargetID, "assistant"), target.ID, payload)
	if sendErr != nil {
		lines := []string{
			lang.T("assistant.research_package.send_failed"),
			lang.T("assistant.research_package.target", targetLabel),
			lang.T("assistant.research_package.problem", sendErr.Error()),
			"",
			lang.T("assistant.research_package.attach_here"),
		}
		lines = appendAssistantWorkflowMobileLinkLines(lines, mobileLinkLines)
		lines = appendVoiceReportAttachmentMarkers(lines, attachments)
		return strings.Join(appendAssistantEvidenceNote(lines, sendRecord, sendStoreErr), "\n")
	}
	lines := []string{
		lang.T("assistant.research_package.sent"),
		lang.T("assistant.research_package.target", targetLabel),
	}
	if id := strings.TrimSpace(send.Stdout); id != "" {
		lines = append(lines, lang.T("assistant.workflow.signal_id", id))
	}
	return strings.Join(appendAssistantEvidenceNote(lines, sendRecord, sendStoreErr), "\n")
}

func isAssistantBrowserResearchPackageRequest(request string) bool {
	lower := strings.ToLower(strings.TrimSpace(request))
	if lower == "" {
		return false
	}
	if containsAny(lower, "쿠팡", "coupang", "장바구니", "결제", "구매 실행") {
		return false
	}
	if containsAny(lower, "스킬", "skill") {
		return false
	}
	if containsAny(lower, "브라우저", "웹 검색", "web search", "browser") &&
		containsAny(lower, "검색", "출처", "근거", "source", "sources") &&
		containsAny(lower, "보고서", "리포트", "문서", "표", "엑셀", "xlsx", "csv", "ppt", "pptx", "발표자료", "회의자료", "정리", "분석", "시장조사", "시장 조사", "report", "table", "deck", "package") {
		return true
	}
	if isAssistantMarketResearchMeetingPackageRequest(lower) {
		return true
	}
	webSignal := containsAny(lower,
		"브라우저", "웹에서", "웹 검색", "검색해서", "검색해", "검색 결과", "출처", "근거", "source", "sources", "browser", "web search",
	)
	artifactSignal := containsAny(lower,
		"보고서", "리포트", "문서", "표", "엑셀", "xlsx", "csv", "ppt", "pptx", "발표자료", "회의자료", "정리", "분석", "시장조사", "시장 조사", "research",
	)
	return webSignal && artifactSignal
}

func isAssistantMarketResearchMeetingPackageRequest(lower string) bool {
	lower = strings.ToLower(strings.TrimSpace(lower))
	if lower == "" {
		return false
	}
	if containsAny(lower, "쿠팡", "coupang", "장바구니", "결제", "구매", "buy", "purchase", "checkout") {
		return false
	}
	if containsAny(lower,
		"회의록", "회의 메모", "회의메모", "액션아이템", "action item", "action items", "meeting minutes",
		"워룸", "war room", "cmo", "브랜드 워룸", "마케팅 워룸",
	) {
		return false
	}
	meetingSignal := containsAny(lower, "회의자료", "회의 자료", "발표자료", "회의 토픽", "회의 안건", "회의용", "meeting material", "meeting materials", "meeting agenda", "meeting topics")
	packageSignal := containsAny(lower, "패키지", "문서", "보고서", "ppt", "pptx", "docx", "markdown", "슬라이드", "표", "엑셀", "xlsx", "package", "deck", "slides")
	followupSignal := containsAny(lower, "이걸", "이 브리프", "그 브리프", "방금", "위 내용", "this brief", "that brief", "turn this")
	explicitMarketMaterial := containsAny(lower,
		"시장조사로 분석", "시장 조사로 분석", "시장분석으로", "시장 분석으로", "시장조사 회의자료", "시장 조사 회의자료", "시장조사 회의 자료",
		"market research meeting material", "market research meeting package", "market analysis meeting material",
	)
	return (explicitMarketMaterial && meetingSignal && packageSignal) || (followupSignal && meetingSignal && packageSignal)
}

func assistantBrowserResearchPackageWantsVoice(request string) bool {
	lower := strings.ToLower(strings.TrimSpace(request))
	return containsAny(lower,
		"음성", "음성파일", "음성 파일", "음성으로", "목소리", "읽어서", "읽어줘",
		"tts", "edge tts", "edge-tts", "mp3", "m4a", "audio", "voice", "voice file", "voice note", "voice message", "voice report", "voice briefing", "audio briefing", "read aloud", "narration", "보이스노트",
		"뉴스 방송", "뉴스방송", "방송 원고", "방송원고", "브리핑 원고",
	)
}

func assistantBrowserResearchPackageQuery(request string) string {
	query := strings.TrimSpace(request)
	koreanRequest := assistantBrowserResearchQueryUsesKorean(query)
	replacer := strings.NewReplacer(
		"브라우저로", " ", "브라우저에서", " ", "브라우저", " ",
		"웹에서", " ", "웹 검색으로", " ", "웹 검색", " ", "웹으로", " ",
		"검색해서", " ", "검색해줘", " ", "검색해", " ", "검색 결과", " ", "검색", " ",
		"보고방에", " ", "보고방", " ", "브리핑방에", " ", "브리핑방", " ",
		"비서방에", " ", "비서방", " ", "채팅방에", " ", "채팅방", " ",
		"내게", " ", "나에게", " ", "나한테", " ", "내 시그널로", " ", "내 시그널", " ",
		"출처 있는", " ", "출처있는", " ", "출처를", " ", "출처", " ",
		"근거 있는", " ", "근거있는", " ", "근거를", " ", "근거", " ",
		"보고서와", " ", "보고서를", " ", "보고서", " ",
		"리포트와", " ", "리포트를", " ", "리포트", " ",
		"문서와", " ", "문서를", " ", "문서", " ",
		"markdown으로", " ", "markdown", " ", "Markdown으로", " ", "Markdown", " ",
		"DOCX로", " ", "DOCX", " ", "docx로", " ", "docx", " ",
		"표로", " ", "표와", " ", "표를", " ", "표", " ",
		"엑셀로", " ", "엑셀", " ", "xlsx", " ", "csv", " ",
		"PPTX로", " ", "PPTX", " ", "PPT로", " ", "PPT", " ",
		"pptx로", " ", "pptx", " ", "ppt로", " ", "ppt", " ", "발표자료", " ", "회의자료", " ",
		"내일 회의에서", " ", "내일 회의", " ", "회의에서", " ", "회의 토픽", " ", "토픽과", " ", "토픽", " ",
		"뉴스 방송형", " ", "뉴스 방송", " ", "뉴스방송", " ", "방송 원고", " ", "방송원고", " ", "브리핑 원고", " ", "브리핑도", " ", "브리핑", " ",
		"원고와", " ", "원고를", " ", "원고로", " ", "원고", " ",
		"edge tts로", " ", "edge tts", " ", "edge-tts로", " ", "edge-tts", " ", "tts로", " ", "tts", " ",
		"음성파일로", " ", "음성 파일로", " ", "음성으로", " ", "음성파일", " ", "음성 파일", " ", "음성", " ",
		"browser search for", " ", "browser search", " ", "web search for", " ", "web search", " ", "search for", " ", "search", " ",
		"with sources", " ", "sources", " ", "source", " ",
		"meeting-ready", " ", "meeting ready", " ", "meeting topics", " ", "tomorrow meeting", " ", "tomorrow's meeting", " ",
		"research report", " ", "market research", " ", "research", " ", "report", " ", "document", " ",
		"source table", " ", "table", " ", "spreadsheet", " ", "xlsx", " ", "csv", " ",
		"slide deck", " ", "slides", " ", "deck", " ", "pptx", " ", "ppt", " ",
		"broadcast-style", " ", "broadcast style", " ", "broadcast script", " ", "voice briefing", " ", "voice brief", " ", "voice", " ", "audio", " ", "mp3", " ",
		"edge tts", " ", "edge-tts", " ", "tts", " ",
		"send it to the briefing room", " ", "send to the briefing room", " ", "briefing room", " ",
		"send it to signal", " ", "send to signal", " ", "send it", " ", "send", " ",
		"analyze", " ", "analyse", " ", "summarize", " ", "summarise", " ", "prepare", " ", "create", " ", "make", " ", "package", " ",
		"정리해서", " ", "정리해줘", " ", "정리해", " ", "정리", " ",
		"분석해서", " ", "분석해줘", " ", "분석해", " ",
		"준비해서", " ", "준비해줘", " ", "준비해", " ", "준비", " ",
		"만들고", " ", "만들어서", " ", "만들어줘", " ", "만들어", " ", "만들", " ",
		"첨부해서", " ", "첨부해줘", " ", "첨부", " ",
		"보내줘", " ", "보내", " ",
	)
	fields := []string{}
	for _, field := range strings.Fields(replacer.Replace(query)) {
		field = strings.TrimSpace(strings.Trim(field, ".,，。:：;；"))
		field = assistantBrowserResearchCleanQueryField(field)
		if field == "" {
			continue
		}
		fields = append(fields, field)
	}
	query = strings.Join(fields, " ")
	if query == "" || assistantBrowserResearchPackageLooksLikeTopicFollowup(request, query) {
		if koreanRequest {
			query = "최신 시장 조사 고객 수요 경쟁사 리스크 기회"
		} else {
			query = "latest market research customer demand competitor risk opportunities"
		}
	}
	lower := strings.ToLower(query)
	if !containsAny(lower, "latest", "recent", "2026", "최근", "오늘", "뉴스", "news") {
		if koreanRequest {
			query += " 최신 2026"
		} else {
			query += " latest 2026"
		}
	}
	if !containsAny(lower, "official", "source", "출처", "공식", "report", "data") {
		if koreanRequest {
			query += " 공식 출처 데이터"
		} else {
			query += " official sources data"
		}
	}
	return query
}

func assistantBrowserResearchPackageLooksLikeTopicFollowup(request, query string) bool {
	lower := strings.ToLower(strings.TrimSpace(request))
	query = strings.TrimSpace(query)
	if query == "" {
		return true
	}
	if !containsAny(lower, "이걸", "이 브리프", "그 브리프", "방금", "위 내용", "this brief", "that brief", "turn this") {
		return false
	}
	cleaned := strings.NewReplacer("이걸", "", "이", "", "그", "", "방금", "", "위", "", "내용", "", "this", "", "that", "", "brief", "").Replace(strings.ToLower(query))
	cleaned = strings.TrimSpace(cleaned)
	return len([]rune(cleaned)) < 4
}

func assistantBrowserResearchCleanQueryField(field string) string {
	field = strings.TrimSpace(field)
	runes := []rune(field)
	if len(runes) <= 1 {
		return field
	}
	last := runes[len(runes)-1]
	switch last {
	case '을', '를', '은', '는', '이', '가', '와':
		return string(runes[:len(runes)-1])
	case '과':
		if len(runes) > 2 {
			return string(runes[:len(runes)-1])
		}
	}
	return field
}

func assistantBrowserResearchQueryUsesKorean(value string) bool {
	for _, r := range value {
		if (r >= 0xAC00 && r <= 0xD7A3) || (r >= 0x3130 && r <= 0x318F) || (r >= 0x1100 && r <= 0x11FF) {
			return true
		}
	}
	return false
}

func assistantBrowserResearchPackageTitle(request, query string) string {
	lower := strings.ToLower(strings.TrimSpace(request + " " + query))
	switch {
	case containsAny(lower, "전기차", "ev", "보조금"):
		return lang.T("assistant.research_package.title.ev")
	case containsAny(lower, "화학", "chemical"):
		return lang.T("assistant.research_package.title.chemical")
	case containsAny(lower, "ai", "인공지능"):
		return lang.T("assistant.research_package.title.ai")
	case containsAny(lower, "광고", "마케팅", "marketing", "advertising"):
		return lang.T("assistant.research_package.title.marketing")
	case containsAny(lower, "채용", "인사", "hr", "recruit"):
		return lang.T("assistant.research_package.title.hr")
	default:
		return lang.T("assistant.research_package.title.default")
	}
}

func assistantBrowserResearchReportBody(request, query string, report publish.ResearchReport, researchErr error, researchDisabled bool) string {
	now := time.Now().In(time.FixedZone("KST", 9*60*60))
	title := assistantBrowserResearchPackageTitle(request, query)
	lines := []string{
		"# " + lang.T("assistant.research_package.report.title", title),
		"",
		"- " + lang.T("assistant.research_package.report.created_at", now.Format("2006-01-02 15:04 KST")),
		"- " + lang.T("assistant.research_package.report.request", strings.TrimSpace(request)),
		"- " + lang.T("assistant.research_package.report.query", query),
		"- " + assistantBrowserResearchSourceStatusLine(report),
		"- " + assistantBrowserResearchProvenanceLine(report, researchErr, researchDisabled),
		"- " + lang.T("assistant.research_package.report.purpose"),
		"",
		"## " + lang.T("assistant.research_package.report.conclusion"),
		"- " + lang.T("assistant.research_package.report.conclusion.1"),
		"- " + lang.T("assistant.research_package.report.conclusion.2"),
		"- " + lang.T("assistant.research_package.report.conclusion.3"),
		"",
		"## " + lang.T("assistant.research_package.report.meeting_topics"),
	}
	lines = append(lines, assistantBrowserResearchMeetingQuestionLines(request, query, 5)...)
	lines = append(lines,
		"",
		"## "+lang.T("assistant.research_package.report.decision_options"),
	)
	lines = append(lines, assistantBrowserResearchDecisionOptionLines(request, query)...)
	lines = append(lines,
		"",
		"## "+lang.T("assistant.research_package.report.run_plan"),
	)
	lines = append(lines, assistantBrowserResearchRunPlanLines()...)
	lines = append(lines,
		"",
		"## "+lang.T("assistant.research_package.report.followup_handoff"),
	)
	lines = append(lines, assistantBrowserResearchFollowupHandoffLines()...)
	lines = append(lines,
		"",
		"## "+lang.T("assistant.research_package.report.sources"),
	)
	lines = append(lines, assistantResearchSourceLines(report)...)
	lines = append(lines,
		"",
		"## "+lang.T("assistant.research_package.report.notes"),
	)
	sourceNotes := assistantBrowserResearchSourceNotes(report)
	if len(sourceNotes) == 0 {
		lines = append(lines, "- "+lang.T("assistant.research_package.report.no_notes"))
	} else {
		lines = append(lines, sourceNotes...)
	}
	lines = append(lines,
		"",
		"## "+lang.T("assistant.research_package.report.next_actions"),
		"- "+lang.T("assistant.research_package.report.action.1"),
		"- "+lang.T("assistant.research_package.report.action.2"),
		"- "+lang.T("assistant.research_package.report.action.3"),
	)
	if researchErr != nil {
		lines = append(lines, "", "## "+lang.T("assistant.research_package.report.search_status"), "- "+lang.T("assistant.research_package.report.search_error", researchErr.Error()))
	}
	return strings.Join(lines, "\n")
}

func assistantBrowserResearchDeckBody(request, query string, report publish.ResearchReport, researchErr error, researchDisabled bool) string {
	sourceLines := assistantResearchSourceLines(report)
	sourceSummary := "- " + lang.T("assistant.research_package.deck.no_sources")
	if len(sourceLines) > 0 {
		limit := len(sourceLines)
		if limit > 3 {
			limit = 3
		}
		sourceSummary = strings.Join(sourceLines[:limit], "\n")
	}
	status := lang.T("assistant.research_package.deck.status_ok")
	if researchDisabled {
		status = lang.T("assistant.research_package.deck.status_disabled")
	} else if researchErr != nil {
		status = lang.T("assistant.research_package.deck.status_error", researchErr.Error())
	}
	status = strings.Join([]string{status, assistantBrowserResearchProvenanceLine(report, researchErr, researchDisabled)}, "\n")
	return strings.Join([]string{
		"# " + lang.T("assistant.research_package.deck.goal"),
		lang.T("assistant.research_package.deck.goal_body"),
		"# " + lang.T("assistant.research_package.deck.request"),
		strings.TrimSpace(request),
		"# " + lang.T("assistant.research_package.deck.query"),
		query,
		"# " + lang.T("assistant.research_package.deck.sources"),
		sourceSummary,
		"# " + lang.T("assistant.research_package.deck.decision_questions"),
		assistantBrowserResearchDeckDecisionBody(request, query),
		"# " + lang.T("assistant.research_package.deck.options"),
		strings.Join(assistantBrowserResearchDecisionOptionLines(request, query), "\n"),
		"# " + lang.T("assistant.research_package.deck.run_plan"),
		strings.Join(assistantBrowserResearchRunPlanLines(), "\n"),
		"# " + lang.T("assistant.research_package.deck.followup_handoff"),
		strings.Join(assistantBrowserResearchFollowupHandoffLines(), "\n"),
		"# " + lang.T("assistant.research_package.deck.risk"),
		lang.T("assistant.research_package.deck.risk_body"),
		"# " + lang.T("assistant.research_package.deck.next"),
		lang.T("assistant.research_package.deck.next_body"),
		"# " + lang.T("assistant.research_package.deck.status"),
		status,
	}, "\n\n")
}

func assistantBrowserResearchSheetBody(query string, report publish.ResearchReport) string {
	return browserSearchSpreadsheetMarkdown(query, report.Search, report.SourcePages)
}

func assistantBrowserResearchPackageSummary(report publish.ResearchReport, researchErr error) []string {
	lines := []string{}
	sourceLines := assistantResearchSourceLines(report)
	for i, line := range sourceLines {
		if i >= 4 {
			break
		}
		lines = append(lines, "- "+strings.TrimPrefix(line, "- "))
	}
	if researchErr != nil {
		lines = append(lines, "- "+lang.T("assistant.research_package.summary.error"))
	}
	if len(lines) == 0 {
		lines = append(lines, "- "+lang.T("assistant.research_package.summary.no_sources"))
	}
	return lines
}

func assistantBrowserResearchVisibleSourceNoteLines(report publish.ResearchReport, limit int) []string {
	if limit <= 0 {
		limit = 2
	}
	lines := []string{}
	for i, page := range report.SourcePages {
		if len(lines) >= limit {
			break
		}
		if strings.TrimSpace(page.Error) != "" {
			continue
		}
		excerpt := strings.TrimSpace(publish.SourceExcerpt(page.Text, 150))
		if excerpt == "" {
			continue
		}
		if !assistantBrowserResearchExcerptMatchesQuery(excerpt, report.Query) {
			continue
		}
		rawURL := firstNonEmpty(page.FinalURL, page.URL)
		title := firstNonEmpty(page.Title, rawURL, lang.T("assistant.research_package.source.label", i+1))
		sourceType := assistantBrowserResearchVisibleSourceType(title, rawURL)
		title = shortenSVGText(title, 54)
		excerpt = shortenSVGText(excerpt, 110)
		lines = append(lines, shortenSVGText(fmt.Sprintf("- [S%d/%s] %s: %s", i+1, sourceType, title, excerpt), 180))
	}
	return lines
}

func assistantBrowserResearchVisibleSourceType(title, rawURL string) string {
	host := searchResultHost(rawURL)
	sourceType, _ := classifyBrowserSearchSource(title, host, rawURL)
	if looksLikeOfficialResearchSource(rawURL + " " + host) {
		sourceType = lang.T("assistant.research_package.source.type.official")
	}
	return sourceType
}

func assistantBrowserResearchExcerptMatchesQuery(excerpt, query string) bool {
	query = strings.TrimSpace(query)
	if query == "" {
		return true
	}
	excerpt = strings.ToLower(excerpt)
	for _, field := range strings.Fields(strings.ToLower(query)) {
		field = strings.TrimSpace(strings.Trim(field, ".,，。:：;；"))
		field = assistantBrowserResearchCleanQueryField(field)
		if len([]rune(field)) < 2 {
			continue
		}
		if containsAny(field, "latest", "recent", "official", "source", "sources", "data", "최신", "공식", "출처", "데이터") {
			continue
		}
		if strings.Contains(excerpt, strings.ToLower(field)) {
			return true
		}
	}
	return false
}

func assistantBrowserResearchConclusionLines() []string {
	return []string{
		"- " + lang.T("assistant.research_package.report.conclusion.1"),
		"- " + lang.T("assistant.research_package.report.conclusion.2"),
		"- " + lang.T("assistant.research_package.report.conclusion.3"),
	}
}

func assistantBrowserResearchMeetingQuestionLines(request, query string, limit int) []string {
	topics := assistantBrowserResearchTopicTexts(request, query)
	if limit <= 0 || limit > len(topics) {
		limit = len(topics)
	}
	lines := make([]string, 0, limit)
	for i := 0; i < limit; i++ {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, topics[i]))
	}
	return lines
}

func assistantBrowserResearchTopicTexts(request, query string) []string {
	profile := assistantBrowserResearchScenarioProfile(request, query)
	topics := make([]string, 0, 5)
	for i := 1; i <= 5; i++ {
		topics = append(topics, lang.T(fmt.Sprintf("assistant.research_package.%s.topic.%d", profile, i)))
	}
	return topics
}

func assistantBrowserResearchDecisionOptionLines(request, query string) []string {
	profile := assistantBrowserResearchScenarioProfile(request, query)
	return []string{
		"- " + lang.T(fmt.Sprintf("assistant.research_package.%s.option.1", profile)),
		"- " + lang.T(fmt.Sprintf("assistant.research_package.%s.option.2", profile)),
		"- " + lang.T(fmt.Sprintf("assistant.research_package.%s.option.3", profile)),
	}
}

func assistantBrowserResearchRunPlanLines() []string {
	return []string{
		"- " + lang.T("assistant.research_package.run_plan.before"),
		"- " + lang.T("assistant.research_package.run_plan.during"),
		"- " + lang.T("assistant.research_package.run_plan.after"),
	}
}

func assistantBrowserResearchFollowupHandoffLines() []string {
	return []string{
		"- " + lang.T("assistant.research_package.followup_handoff.owner_deadline"),
		"- " + lang.T("assistant.research_package.followup_handoff.reminder"),
		"- " + lang.T("assistant.research_package.followup_handoff.boundary"),
	}
}

func assistantBrowserResearchDeckDecisionBody(request, query string) string {
	questions := assistantBrowserResearchMeetingQuestionLines(request, query, 3)
	for i, question := range questions {
		questions[i] = strings.TrimSpace(strings.TrimLeft(question, "1234567890. "))
	}
	return strings.Join(questions, "\n")
}

func assistantBrowserResearchScenarioProfile(request, query string) string {
	lower := strings.ToLower(strings.TrimSpace(request + " " + query))
	switch {
	case containsAny(lower, "화학", "chemical", "pfas", "reach", "나프타", "naphtha", "석유화학"):
		return "chemical"
	case containsAny(lower, "광고", "마케팅", "marketing", "advertising", "roas", "campaign"):
		return "marketing"
	case containsAny(lower, "채용", "인사", "hr", "recruit", "candidate", "job"):
		return "hr"
	default:
		return "generic"
	}
}

func assistantBrowserResearchActionLines() []string {
	return []string{
		"- " + lang.T("assistant.research_package.report.action.1"),
		"- " + lang.T("assistant.research_package.report.action.2"),
		"- " + lang.T("assistant.research_package.report.action.3"),
	}
}

func assistantBrowserResearchVoiceScript(title, query string, report publish.ResearchReport, researchErr error, researchDisabled bool) string {
	lines := []string{
		lang.T("assistant.research_package.voice.intro", title),
		assistantBrowserResearchVoiceSourceStatus(report),
		assistantBrowserResearchVoiceProvenance(report, researchErr, researchDisabled),
	}
	sourceLines := assistantResearchSourceLines(report)
	if len(sourceLines) > 0 && !strings.Contains(sourceLines[0], lang.T("assistant.research_package.source.none")) {
		lines = append(lines, lang.T("assistant.research_package.voice.search", query))
		lines = append(lines, lang.T("assistant.research_package.voice.sources_intro"))
		for i, line := range sourceLines {
			if i >= 3 {
				break
			}
			line = strings.TrimSpace(strings.TrimPrefix(line, "- "))
			line = strings.TrimSpace(strings.TrimLeft(line, "1234567890. "))
			line = stripURLsForSpeech(line)
			if line != "" {
				lines = append(lines, line+".")
			}
		}
	} else {
		lines = append(lines, lang.T("assistant.research_package.voice.insufficient"))
	}
	notes := assistantBrowserResearchSourceNotes(report)
	if len(notes) > 0 {
		lines = append(lines, lang.T("assistant.research_package.voice.notes"))
	} else {
		lines = append(lines, lang.T("assistant.research_package.voice.no_notes"))
	}
	lines = append(lines,
		lang.T("assistant.research_package.voice.meeting"),
		strings.TrimPrefix(assistantBrowserResearchDecisionOptionLines(title, query)[0], "- "),
		lang.T("assistant.research_package.voice.run_plan"),
		lang.T("assistant.research_package.voice.followup_handoff"),
		lang.T("assistant.research_package.voice.attachments"),
	)
	if researchErr != nil {
		lines = append(lines, lang.T("assistant.research_package.voice.error"))
	}
	lines = append(lines, lang.T("assistant.research_package.voice.closing"))
	return strings.Join(lines, "\n\n")
}

type assistantBrowserResearchSourceStats struct {
	SearchCandidates   int
	PagesRead          int
	OfficialCandidates int
	ReadFailures       int
}

func assistantBrowserResearchSourceStatsFor(report publish.ResearchReport) assistantBrowserResearchSourceStats {
	stats := assistantBrowserResearchSourceStats{SearchCandidates: len(report.Search.Results)}
	for _, page := range report.SourcePages {
		if strings.TrimSpace(page.Error) != "" {
			stats.ReadFailures++
			continue
		}
		if strings.TrimSpace(page.Text) != "" {
			stats.PagesRead++
		}
		if looksLikeOfficialResearchSource(firstNonEmpty(page.FinalURL, page.URL, page.Title)) {
			stats.OfficialCandidates++
		}
	}
	if stats.OfficialCandidates == 0 {
		for _, link := range report.Search.Results {
			if looksLikeOfficialResearchSource(link.URL + " " + link.Text) {
				stats.OfficialCandidates++
			}
		}
	}
	return stats
}

func assistantBrowserResearchSourceStatusLine(report publish.ResearchReport) string {
	stats := assistantBrowserResearchSourceStatsFor(report)
	return lang.T("assistant.research_package.source_status", stats.SearchCandidates, stats.PagesRead, stats.OfficialCandidates, stats.ReadFailures)
}

func assistantBrowserResearchSubtitle(report publish.ResearchReport, researchErr error, researchDisabled bool) string {
	if researchDisabled {
		return lang.T("assistant.research_package.subtitle.disabled")
	}
	if researchErr != nil {
		return lang.T("assistant.research_package.subtitle.partial")
	}
	stats := assistantBrowserResearchSourceStatsFor(report)
	if stats.SearchCandidates == 0 && stats.PagesRead == 0 {
		return lang.T("assistant.research_package.subtitle.thin")
	}
	return lang.T("assistant.research_package.subtitle")
}

func assistantBrowserResearchProvenanceLine(report publish.ResearchReport, researchErr error, researchDisabled bool) string {
	if researchDisabled {
		return lang.T("assistant.research_package.provenance.disabled")
	}
	if researchErr != nil {
		return lang.T("assistant.research_package.provenance.partial")
	}
	stats := assistantBrowserResearchSourceStatsFor(report)
	if stats.SearchCandidates == 0 && stats.PagesRead == 0 {
		return lang.T("assistant.research_package.provenance.thin")
	}
	return lang.T("assistant.research_package.provenance.live")
}

func assistantBrowserResearchVoiceSourceStatus(report publish.ResearchReport) string {
	stats := assistantBrowserResearchSourceStatsFor(report)
	return lang.T("assistant.research_package.voice.source_status", stats.SearchCandidates, stats.PagesRead, stats.OfficialCandidates, stats.ReadFailures)
}

func assistantBrowserResearchVoiceProvenance(report publish.ResearchReport, researchErr error, researchDisabled bool) string {
	if researchDisabled {
		return lang.T("assistant.research_package.voice.provenance.disabled")
	}
	if researchErr != nil {
		return lang.T("assistant.research_package.voice.provenance.partial")
	}
	stats := assistantBrowserResearchSourceStatsFor(report)
	if stats.SearchCandidates == 0 && stats.PagesRead == 0 {
		return lang.T("assistant.research_package.voice.provenance.thin")
	}
	return lang.T("assistant.research_package.voice.provenance.live")
}

func stripURLsForSpeech(text string) string {
	parts := strings.Fields(text)
	kept := make([]string, 0, len(parts))
	for _, part := range parts {
		lower := strings.ToLower(strings.TrimSpace(part))
		if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
			continue
		}
		kept = append(kept, part)
	}
	cleaned := strings.TrimSpace(strings.Join(kept, " "))
	return strings.TrimSpace(strings.TrimRight(cleaned, "-–—:：,，. "))
}

func assistantBrowserResearchSourceNotes(report publish.ResearchReport) []string {
	lines := []string{}
	for i, page := range report.SourcePages {
		if i >= 5 {
			break
		}
		if strings.TrimSpace(page.Error) != "" {
			continue
		}
		excerpt := strings.TrimSpace(publish.SourceExcerpt(page.Text, 220))
		if excerpt == "" {
			continue
		}
		title := firstNonEmpty(page.Title, page.FinalURL, page.URL, lang.T("assistant.research_package.source.label", i+1))
		lines = append(lines, fmt.Sprintf("- [S%d] %s: %s", i+1, title, excerpt))
	}
	return lines
}

func writeAssistantBrowserResearchSourceMapSVG(title, query string, report publish.ResearchReport, researchErr error) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Documents", "Argos Vault", "Charts")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, safeAssistantDepartmentFilename(title)+"-"+time.Now().Format("20060102-150405")+"-source-map.svg")
	content := renderAssistantBrowserResearchSourceMapSVG(title, query, report, researchErr)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return "", err
	}
	return path, nil
}

func renderAssistantBrowserResearchSourceMapSVG(title, query string, report publish.ResearchReport, researchErr error) string {
	stats := assistantBrowserResearchSourceStatsFor(report)
	points := []assistantDepartmentChartPoint{
		{Label: lang.T("assistant.research_package.source_map.search"), Value: float64(maxInt(stats.SearchCandidates, 1)), Color: "#2563eb"},
		{Label: lang.T("assistant.research_package.source_map.read"), Value: float64(maxInt(stats.PagesRead, 0)), Color: "#0f766e"},
		{Label: lang.T("assistant.research_package.source_map.official"), Value: float64(maxInt(stats.OfficialCandidates, 0)), Color: "#7c3aed"},
		{Label: lang.T("assistant.research_package.source_map.errors"), Value: float64(maxInt(stats.ReadFailures, 0)), Color: "#d97706"},
	}
	max := 1.0
	for _, point := range points {
		if point.Value > max {
			max = point.Value
		}
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="820" height="560" viewBox="0 0 820 560">` + "\n")
	b.WriteString(`<rect width="820" height="560" fill="#f8fafc"/>` + "\n")
	b.WriteString(`<rect x="24" y="24" width="772" height="512" rx="20" fill="#ffffff" stroke="#d9e2ef"/>` + "\n")
	b.WriteString(fmt.Sprintf(`<text x="48" y="66" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="27" font-weight="850" fill="#0f172a">%s</text>`, html.EscapeString(shortenSVGText(title, 38))))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`<text x="48" y="96" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="15" fill="#64748b">%s</text>`, html.EscapeString(shortenSVGText(query, 78))))
	b.WriteString("\n")
	left := 68.0
	top := 144.0
	chartW := 608.0
	chartH := 198.0
	gap := 28.0
	barW := (chartW - gap*float64(len(points)-1)) / float64(len(points))
	b.WriteString(fmt.Sprintf(`<line x1="%.0f" y1="%.0f" x2="%.0f" y2="%.0f" stroke="#94a3b8" stroke-width="2"/>`, left, top+chartH, left+chartW, top+chartH))
	b.WriteString("\n")
	for i, point := range points {
		x := left + float64(i)*(barW+gap)
		barH := chartH * (point.Value / max)
		y := top + chartH - barH
		b.WriteString(fmt.Sprintf(`<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="10" fill="%s"/>`, x, y, barW, barH, html.EscapeString(point.Color)))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="middle" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="18" font-weight="850" fill="#0f172a">%.0f</text>`, x+barW/2, y-12, point.Value))
		b.WriteString("\n")
		for j, line := range wrapSVGText(point.Label, 10, 2) {
			b.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="middle" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="14" font-weight="700" fill="#334155">%s</text>`, x+barW/2, top+chartH+28+float64(j*18), html.EscapeString(line)))
			b.WriteString("\n")
		}
	}
	topSources := assistantBrowserResearchTopSourceLabels(report)
	b.WriteString(fmt.Sprintf(`<text x="48" y="412" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="16" font-weight="850" fill="#0f172a">%s</text>`, html.EscapeString(lang.T("assistant.research_package.source_map.priority"))))
	b.WriteString("\n")
	for i, source := range topSources {
		b.WriteString(fmt.Sprintf(`<text x="48" y="%d" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="14" fill="#475569">%d. %s</text>`, 440+i*22, i+1, html.EscapeString(shortenSVGText(source, 82))))
		b.WriteString("\n")
	}
	if researchErr != nil {
		b.WriteString(fmt.Sprintf(`<text x="48" y="520" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="13" fill="#b45309">%s</text>`, html.EscapeString(shortenSVGText(lang.T("assistant.research_package.source_map.warning"), 92))))
	} else {
		b.WriteString(fmt.Sprintf(`<text x="48" y="520" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="13" fill="#64748b">%s</text>`, html.EscapeString(lang.T("assistant.research_package.source_map.footer"))))
	}
	b.WriteString("\n</svg>\n")
	return b.String()
}

func assistantBrowserResearchTopSourceLabels(report publish.ResearchReport) []string {
	out := []string{}
	for _, page := range report.SourcePages {
		if len(out) >= 3 {
			break
		}
		label := firstNonEmpty(page.Title, page.FinalURL, page.URL)
		if label != "" {
			out = append(out, label)
		}
	}
	for _, link := range report.Search.Results {
		if len(out) >= 3 {
			break
		}
		label := firstNonEmpty(link.Text, link.URL)
		if label != "" {
			out = append(out, label)
		}
	}
	if len(out) == 0 {
		out = append(out, lang.T("assistant.research_package.source_map.no_sources"))
	}
	return out
}

func looksLikeOfficialResearchSource(value string) bool {
	lower := strings.ToLower(value)
	return containsAny(lower, ".gov", ".go.", ".or.kr", "official", "공식", "ministry", "agency", "commission", "정부", "부처", "청", "공단", "공사", "regulation", "sec.gov", "fda.gov", "epa.gov")
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func assistantBrowserResearchPackageAttachments(doc, deck, sheet osauto.Result, report publish.ResearchReport) []string {
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
