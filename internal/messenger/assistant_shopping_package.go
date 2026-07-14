package messenger

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/meshclaw/meshclaw/internal/browserauto"
	"github.com/meshclaw/meshclaw/internal/evidence"
	"github.com/meshclaw/meshclaw/internal/lang"
	"github.com/meshclaw/meshclaw/internal/osauto"
	"github.com/meshclaw/meshclaw/internal/tts"
)

func assistantCoupangShoppingPackageReply(opts ListenOptions, request string) (string, bool) {
	if !isAssistantCoupangShoppingPackageRequest(request) {
		return "", false
	}
	query := assistantShoppingPackageQuery(request)
	if strings.TrimSpace(query) == "" {
		query = lang.T("assistant.shopping_package.default_query")
	}
	searchURL := coupangSearchURL(query)

	actionReply := ""
	if reply, handled := runSignalArgosAction(opts, searchURL+" 열어줘", 0); handled {
		actionReply = signalReplyVisibleText(reply)
		if lang.Current() != "ko" && assistantCheckoutPrepContainsHangul(actionReply) {
			actionReply = lang.T("assistant.shopping_package.action_requires_approval", searchURL)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	candidates := coupangShoppingCandidates(ctx, query, 5)
	searchCandidateCount := len(candidates)
	fallback := false
	if len(candidates) == 0 {
		candidates = assistantShoppingPackageFallbackCandidates(query, searchURL)
		fallback = true
	}
	candidates = assistantShoppingCandidateLines(candidates, 5)
	fallbackCandidateCount := 0
	if fallback {
		fallbackCandidateCount = len(candidates)
	}
	sourceStatus := assistantShoppingPackageSourceStatusLine(searchCandidateCount, fallbackCandidateCount)
	browserStatus := assistantShoppingPackageBrowserStatusLine(actionReply)
	boundaryStatus := assistantShoppingPackageBoundaryStatusLine()
	title := lang.T("assistant.shopping_package.title")
	selected := browserauto.Link{}
	if len(candidates) > 0 {
		selected = candidates[0]
	}

	restorePreview := temporarilySkipPreviewImages()
	doc := osauto.CreateArgosDocument(ctx, lang.T("assistant.shopping_package.artifact.doc_title", title), assistantShoppingPackageReportBody(request, query, searchURL, candidates, fallback, sourceStatus, browserStatus, boundaryStatus))
	deck := osauto.CreatePresentation(ctx, lang.T("assistant.shopping_package.artifact.deck_title", title), assistantShoppingPackageDeckBody(query, searchURL, candidates, fallback, sourceStatus, browserStatus, boundaryStatus), lang.T("assistant.shopping_package.artifact.audience"), 7, "")
	sheet := osauto.CreateSpreadsheet(ctx, lang.T("assistant.shopping_package.artifact.sheet_title", title), assistantShoppingPackageSheetBody(query, searchURL, candidates, fallback))
	safetyPath, safetyErr := writeAssistantCoupangCheckoutPrepSafetySVG(title, query, searchURL, selected, 0)
	restorePreview()

	wantsVoice := assistantBrowserResearchPackageWantsVoice(request)
	voiceScript := ""
	var audio tts.Result
	var audioErr error
	if wantsVoice {
		voiceScript = assistantShoppingPackageVoiceScript(query, candidates, fallback, searchCandidateCount, fallbackCandidateCount, actionReply)
		audio, audioErr = tts.Synthesize(tts.Options{
			Text:     voiceScript,
			Engine:   "edge-tts",
			Basename: "coupang-shopping-brief-" + time.Now().UTC().Format("20060102T150405Z"),
		})
	}

	attachments := assistantShoppingPackageAttachments(doc, deck, sheet)
	if strings.TrimSpace(safetyPath) != "" && safetyErr == nil {
		attachments = uniqueShowcaseAttachments(append(attachments, safetyPath))
	}
	if strings.TrimSpace(audio.Path) != "" && audioErr == nil {
		attachments = uniqueShowcaseAttachments(append(attachments, audio.Path))
	}
	rememberAssistantArtifact(opts, title, "coupang_shopping_package", doc, deck, sheet)
	record, storeErr := evidence.Store("assistant-coupang-shopping-package", firstNonEmpty(opts.TargetID, "assistant"), title, map[string]interface{}{
		"kind":                     "assistant_coupang_shopping_package",
		"request":                  request,
		"query":                    query,
		"search_url":               searchURL,
		"fallback":                 fallback,
		"search_candidate_count":   searchCandidateCount,
		"fallback_candidate_count": fallbackCandidateCount,
		"source_status":            sourceStatus,
		"browser_status":           browserStatus,
		"purchase_boundary_status": boundaryStatus,
		"candidates":               candidates,
		"action_reply":             actionReply,
		"voice_script":             voiceScript,
		"audio":                    audio,
		"audio_error":              errorString(audioErr),
		"safety_svg":               safetyPath,
		"safety_error":             errorString(safetyErr),
		"document":                 doc,
		"presentation":             deck,
		"spreadsheet":              sheet,
		"attachments":              attachments,
		"created_at":               time.Now().UTC(),
	})

	lines := []string{
		lang.T("assistant.shopping_package.created"),
		lang.T("assistant.shopping_package.subtitle"),
		"",
		lang.T("assistant.shopping.coupang.query", query),
		lang.T("assistant.shopping.coupang.link", searchURL),
		sourceStatus,
		browserStatus,
		boundaryStatus,
		"",
		lang.T("assistant.shopping_package.files"),
	}
	if doc.OK && doc.Error == "" {
		lines = append(lines, "- "+lang.T("assistant.shopping_package.file.doc"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.shopping_package.file.doc_failed", firstNonEmpty(doc.Error, doc.Stderr, "unknown error")))
	}
	if deck.OK && deck.Error == "" {
		lines = append(lines, "- "+lang.T("assistant.shopping_package.file.ppt"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.shopping_package.file.ppt_failed", firstNonEmpty(deck.Error, deck.Stderr, "unknown error")))
	}
	if sheet.OK && sheet.Error == "" {
		lines = append(lines, "- "+lang.T("assistant.shopping_package.file.sheet"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.shopping_package.file.sheet_failed", firstNonEmpty(sheet.Error, sheet.Stderr, "unknown error")))
	}
	if strings.TrimSpace(safetyPath) != "" && safetyErr == nil {
		lines = append(lines, "- "+lang.T("assistant.shopping_checkout_prep.file.safety"))
	} else if safetyErr != nil {
		lines = append(lines, "- "+lang.T("assistant.shopping_checkout_prep.file.safety_failed", firstNonEmpty(errorString(safetyErr), "unknown error")))
	}
	if wantsVoice {
		if audioErr == nil && strings.TrimSpace(audio.Path) != "" {
			lines = append(lines, "- "+lang.T("assistant.shopping_package.file.audio"))
		} else {
			lines = append(lines, "- "+lang.T("assistant.shopping_package.file.audio_failed", firstNonEmpty(errorString(audioErr), "unknown error")))
		}
	}
	lines = append(lines, "", lang.T("assistant.shopping.coupang.candidates_title"))
	lines = append(lines, assistantShoppingPackageCandidateSummary(candidates, fallback)...)
	lines = append(lines,
		"",
		lang.T("assistant.shopping.coupang.checklist_title"),
		"- "+lang.T("assistant.shopping.coupang.check.rocket"),
		"- "+lang.T("assistant.shopping.coupang.check.unit_price"),
		"- "+lang.T("assistant.shopping.coupang.check.review"),
		"- "+lang.T("assistant.shopping.coupang.check.option"),
		"",
		lang.T("assistant.shopping_checkout_prep.final_values_title"),
		"- "+lang.T("assistant.shopping.checkout.check.product_only"),
		"- "+lang.T("assistant.shopping.checkout.check.option"),
		"- "+lang.T("assistant.shopping.checkout.check.quantity"),
		"- "+lang.T("assistant.shopping.checkout.check.total"),
		"- "+lang.T("assistant.shopping.checkout.check.delivery"),
		"- "+lang.T("assistant.shopping.checkout.check.address"),
		"- "+lang.T("assistant.shopping.checkout.check.payment"),
		"- "+lang.T("assistant.shopping.checkout.check.button"),
		"",
		lang.T("assistant.shopping_checkout_prep.live_ready_title"),
		"- "+lang.T("assistant.shopping_checkout_prep.live.browser"),
		"- "+lang.T("assistant.shopping_checkout_prep.live.address"),
		"- "+lang.T("assistant.shopping_checkout_prep.live.payment"),
		"- "+lang.T("assistant.shopping_checkout_prep.live.auth"),
		"- "+lang.T("assistant.shopping_checkout_prep.live.stop"),
		"",
		lang.T("assistant.shopping_package.next"),
		"- "+lang.T("assistant.shopping_package.next.detail"),
		"- "+lang.T("assistant.shopping_package.next.cart"),
		"- "+lang.T("assistant.shopping_package.next.checkout"),
		"- "+lang.T("assistant.shopping_package.next.approval"),
		"",
		lang.T("assistant.shopping.coupang.boundary"),
	)
	if strings.TrimSpace(actionReply) != "" {
		lines = append(lines, "", lang.T("assistant.shopping.coupang.action_title"), strings.TrimSpace(actionReply))
	}
	if strings.TrimSpace(voiceScript) != "" {
		lines = append(lines, "", lang.T("assistant.shopping_package.voice_preview"), trimForContext(voiceScript, 520))
	}
	if targetRef := inferAssistantSignalTargetRef(request); targetRef != "" {
		return formatAssistantShoppingPackageSendResult(opts, targetRef, lines, attachments, record, storeErr), true
	}
	lines = appendAssistantWorkflowMobileLinkLines(lines, assistantWorkflowVisibleMobileLinkLines(attachments, 6))
	lines = appendVoiceReportAttachmentMarkers(lines, attachments)
	lines = appendAssistantEvidenceNote(lines, record, storeErr)
	return strings.Join(compactBlankLines(lines), "\n"), true
}

func formatAssistantShoppingPackageSendResult(opts ListenOptions, targetRef string, summary []string, attachments []string, record evidence.Record, storeErr error) string {
	target, candidates, targetErr := resolveAssistantVoiceTarget(targetRef)
	if targetErr != nil {
		lines := []string{lang.T("assistant.shopping_package.send_target_failed")}
		lines = append(lines, formatAssistantVoiceTargetCandidates(candidates)...)
		lines = append(lines, "", lang.T("assistant.shopping_package.attach_here"))
		lines = appendAssistantWorkflowMobileLinkLines(lines, assistantWorkflowVisibleMobileLinkLines(attachments, 6))
		lines = appendVoiceReportAttachmentMarkers(lines, attachments)
		return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
	}
	targetLabel := assistantScheduleLocalizedTarget(target.ID, target.Label)
	if !opts.Execute {
		lines := []string{
			lang.T("assistant.shopping_package.send_ready"),
			lang.T("assistant.shopping_package.target", targetLabel),
		}
		if OneWayReportTarget(target) {
			lines = append(lines, lang.T("assistant.shopping_package.one_way"))
		}
		lines = append(lines, "", lang.T("assistant.shopping_package.to_send"))
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
		"kind":             "assistant_coupang_shopping_package_send",
		"target":           targetRef,
		"resolved_target":  target,
		"attachment_count": len(attachments),
		"mobile_links":     mobileLinkLines,
		"send":             send,
		"send_error":       errorString(sendErr),
		"created_at":       time.Now().UTC(),
	}
	sendRecord, sendStoreErr := evidence.Store("assistant-coupang-shopping-package-send", firstNonEmpty(opts.TargetID, "assistant"), target.ID, payload)
	if sendErr != nil {
		lines := []string{
			lang.T("assistant.shopping_package.send_failed"),
			lang.T("assistant.shopping_package.target", targetLabel),
			lang.T("assistant.shopping_package.problem", sendErr.Error()),
			"",
			lang.T("assistant.shopping_package.attach_here"),
		}
		lines = appendAssistantWorkflowMobileLinkLines(lines, mobileLinkLines)
		lines = appendVoiceReportAttachmentMarkers(lines, attachments)
		return strings.Join(appendAssistantEvidenceNote(lines, sendRecord, sendStoreErr), "\n")
	}
	lines := []string{
		lang.T("assistant.shopping_package.sent"),
		lang.T("assistant.shopping_package.target", targetLabel),
	}
	if id := strings.TrimSpace(send.Stdout); id != "" {
		lines = append(lines, lang.T("assistant.workflow.signal_id", id))
	}
	return strings.Join(appendAssistantEvidenceNote(lines, sendRecord, sendStoreErr), "\n")
}

func isAssistantCoupangShoppingPackageRequest(request string) bool {
	lower := strings.ToLower(strings.TrimSpace(request))
	if lower == "" || !containsAny(lower, "쿠팡", "coupang") {
		return false
	}
	if containsAny(lower, "리허설", "흐름도", "워크북", "rehearsal", "flowchart", "workbook") {
		return false
	}
	if containsAny(lower, "첫 번째", "첫번째", "두 번째", "두번째", "1번", "2번", "3번", "상세", "장바구니", "결제 화면", "최종 화면", "구매 실행 승인", "주문 실행 승인", "detail", "cart", "checkout", "final screen") {
		return false
	}
	compareSignal := containsAny(lower, "후보", "비교", "비교표", "추천", "가격", "리뷰", "후기", "로켓배송", "찾아", "검색", "compare", "candidate", "review", "rocket")
	artifactSignal := containsAny(lower, "표", "엑셀", "xlsx", "csv", "ppt", "pptx", "문서", "보고서", "체크리스트", "음성", "음성파일", "tts", "mp3", "패키지", "table", "sheet", "deck", "slides", "voice")
	realSearchSignal := containsAny(lower, "로켓배송", "로켓 배송", "리뷰", "후기", "ppt", "pptx", "음성", "음성파일", "tts", "mp3", "실제", "검색해서", "검색해", "찾아", "열어", "rocket", "review", "voice")
	return compareSignal && artifactSignal && realSearchSignal
}

func assistantShoppingPackageQuery(request string) string {
	query := strings.TrimSpace(request)
	replacements := []string{
		"쿠팡에서", "쿠팡", "coupang에서", "coupang", "에서",
		"Coupang", "on Coupang", "from Coupang", "from coupang",
		"로켓배송으로", "로켓 배송으로", "로켓배송", "로켓 배송", "로켓",
		"Rocket delivery", "rocket delivery", "rocket-delivery", "rocket",
		"가격", "리뷰", "후기", "좋은 후보 5개", "좋은 후보 3개", "후보 5개", "후보 3개", "좋은 후보", "후보",
		"prices", "price", "reviews", "review", "good candidates", "top candidates", "candidates", "candidate",
		"비교해서", "비교표와", "비교표를", "비교표", "비교해줘", "비교",
		"compare", "comparison",
		"검색해서", "검색해줘", "검색해", "찾아줘", "찾아",
		"search for", "search", "find", "recommend",
		"표와", "표를", "표로", "표", "엑셀로", "엑셀", "xlsx", "csv",
		"table", "spreadsheet", "sheet",
		"PPTX와", "PPT와", "pptx와", "ppt와",
		"PPTX로", "PPTX", "PPT로", "PPT", "pptx로", "pptx", "ppt로", "ppt", "발표자료",
		"slides", "slide", "deck",
		"체크리스트와", "체크리스트를", "체크리스트",
		"checklist",
		"보고방에", "보고방으로", "보고방", "브리핑방에", "브리핑방으로", "브리핑방", "argos-briefing",
		"비서방에", "비서방으로", "비서방", "argos-assistant",
		"briefing room", "assistant room", "send it to", "send to", "send",
		"보내주세요", "보내줘", "보내", "전송해주세요", "전송해줘", "전송", "발송해주세요", "발송해줘", "발송",
		"edge tts로", "edge tts", "edge-tts로", "edge-tts", "tts로", "tts",
		"음성파일로도", "음성 파일로도", "음성으로도",
		"음성파일로", "음성 파일로", "음성으로", "음성파일", "음성 파일", "음성",
		"voice briefing", "voice", "audio", "mp3",
		"패키지로", "패키지", "문서로", "문서", "보고서로", "보고서",
		"package", "document", "report",
		"준비해서", "준비해줘", "준비해", "준비", "정리해서", "정리해줘", "정리해", "정리", "만들어서", "만들어줘", "만들어", "만들",
		"prepare", "summarize", "organize", "create", "make",
		"내일 실제 테스트 전", "내일 실제 테스트", "실제 테스트 전", "실제 테스트", "내일", "실제", "테스트 전", "테스트",
		"Tomorrow live test prep", "Tomorrow live test", "Live test prep", "Live test", "tomorrow live test prep", "tomorrow live test", "live test prep", "live test", "Tomorrow", "tomorrow", "Live", "live",
		"같이 보여주세요", "같이 보여줘", "같이 보여", "보여주세요", "보여줘", "보여",
		"Show me too", "Show too", "Show me", "show me too", "show too", "show me", "Show", "show",
		"으로", "좋은",
		"with", "for", "Also", "also", "Too", "too",
	}
	for _, old := range replacements {
		query = strings.ReplaceAll(query, old, " ")
	}
	query = strings.NewReplacer(",", " ", "，", " ", ".", " ", "!", " ", "?", " ").Replace(query)
	fields := []string{}
	for _, field := range strings.Fields(strings.TrimSpace(query)) {
		switch strings.TrimSpace(field) {
		case "와", "과", "및", "도", "and", "&":
			continue
		default:
			fields = append(fields, field)
		}
	}
	return strings.Join(fields, " ")
}

func assistantShoppingPackageFallbackCandidates(query, searchURL string) []browserauto.Link {
	clean := firstNonEmpty(strings.TrimSpace(query), lang.T("assistant.shopping.fallback.default_query"))
	return []browserauto.Link{
		{Text: lang.T("assistant.shopping.fallback.candidate.1", clean), URL: searchURL},
		{Text: lang.T("assistant.shopping.fallback.candidate.2", clean), URL: searchURL},
		{Text: lang.T("assistant.shopping.fallback.candidate.3", clean), URL: searchURL},
	}
}

func assistantShoppingPackageCandidateSummary(candidates []browserauto.Link, fallback bool) []string {
	lines := []string{}
	for i, candidate := range assistantShoppingCandidateLines(candidates, 5) {
		lines = append(lines, lang.T("assistant.shopping.coupang.candidate_line", i+1, firstNonEmpty(strings.TrimSpace(candidate.Text), lang.T("assistant.shopping_package.candidate_fallback", i+1))))
		if strings.TrimSpace(candidate.URL) != "" {
			lines = append(lines, lang.T("assistant.shopping.coupang.candidate_link", strings.TrimSpace(candidate.URL)))
		}
	}
	if len(lines) == 0 {
		lines = append(lines, "- "+lang.T("assistant.shopping.coupang.candidate_unavailable"))
	}
	if fallback {
		lines = append(lines, "- "+lang.T("assistant.shopping_package.fallback_note"))
	}
	return lines
}

func assistantShoppingPackageReportBody(request, query, searchURL string, candidates []browserauto.Link, fallback bool, sourceStatus, browserStatus, boundaryStatus string) string {
	now := time.Now().In(time.FixedZone("KST", 9*60*60))
	lines := []string{
		"# " + lang.T("assistant.shopping_package.report.title"),
		"",
		"- " + lang.T("assistant.shopping_package.report.created_at", now.Format("2006-01-02 15:04 KST")),
		"- " + lang.T("assistant.shopping_package.report.request", strings.TrimSpace(request)),
		"- " + lang.T("assistant.shopping.coupang.query", query),
		"- " + lang.T("assistant.shopping.coupang.link", searchURL),
		"- " + sourceStatus,
		"- " + browserStatus,
		"- " + boundaryStatus,
		"",
		"## " + lang.T("assistant.shopping_package.report.conclusion"),
		"- " + lang.T("assistant.shopping_package.report.conclusion.1"),
		"- " + lang.T("assistant.shopping_package.report.conclusion.2"),
		"",
		"## " + lang.T("assistant.shopping_package.report.candidates"),
	}
	lines = append(lines, assistantShoppingPackageMarkdownCandidates(candidates, fallback)...)
	lines = append(lines,
		"",
		"## "+lang.T("assistant.shopping_package.report.check_criteria"),
		"- "+lang.T("assistant.shopping.coupang.check.rocket"),
		"- "+lang.T("assistant.shopping.coupang.check.unit_price"),
		"- "+lang.T("assistant.shopping.coupang.check.review"),
		"- "+lang.T("assistant.shopping.coupang.check.option"),
		"",
		"## "+lang.T("assistant.shopping_checkout_prep.final_values_title"),
		"- "+lang.T("assistant.shopping_checkout_prep.report.final.product"),
		"- "+lang.T("assistant.shopping_checkout_prep.report.final.option"),
		"- "+lang.T("assistant.shopping_checkout_prep.report.final.total"),
		"- "+lang.T("assistant.shopping_checkout_prep.report.final.delivery"),
		"- "+lang.T("assistant.shopping_checkout_prep.report.final.address"),
		"- "+lang.T("assistant.shopping_checkout_prep.report.final.payment"),
		"- "+lang.T("assistant.shopping_checkout_prep.report.final.button"),
		"",
		"## "+lang.T("assistant.shopping_checkout_prep.live_ready_title"),
		"- "+lang.T("assistant.shopping_checkout_prep.live.browser"),
		"- "+lang.T("assistant.shopping_checkout_prep.live.address"),
		"- "+lang.T("assistant.shopping_checkout_prep.live.payment"),
		"- "+lang.T("assistant.shopping_checkout_prep.live.auth"),
		"- "+lang.T("assistant.shopping_checkout_prep.live.stop"),
		"",
		"## "+lang.T("assistant.shopping_package.report.next_execution"),
		"- "+lang.T("assistant.shopping_package.report.next_detail"),
		"- "+lang.T("assistant.shopping_package.report.next_cart"),
		"- "+lang.T("assistant.shopping_package.report.next_checkout"),
		"- "+lang.T("assistant.shopping_package.report.next_approval"),
		"- "+lang.T("assistant.shopping_package.report.boundary"),
	)
	return strings.Join(lines, "\n")
}

func assistantShoppingPackageDeckBody(query, searchURL string, candidates []browserauto.Link, fallback bool, sourceStatus, browserStatus, boundaryStatus string) string {
	candidateSummary := strings.Join(assistantShoppingPackageMarkdownCandidates(candidates, fallback), "\n")
	return strings.Join([]string{
		"# " + lang.T("assistant.shopping_package.deck.search"),
		strings.Join([]string{query, searchURL, sourceStatus, browserStatus, boundaryStatus}, "\n"),
		"# " + lang.T("assistant.shopping_package.deck.candidates"),
		candidateSummary,
		"# " + lang.T("assistant.shopping_package.deck.criteria"),
		lang.T("assistant.shopping_package.deck.criteria_body"),
		"# " + lang.T("assistant.shopping_package.deck.recommendation"),
		lang.T("assistant.shopping_package.deck.recommendation_body"),
		"# " + lang.T("assistant.shopping_checkout_prep.live_ready_title"),
		strings.Join([]string{
			lang.T("assistant.shopping_checkout_prep.live.browser"),
			lang.T("assistant.shopping_checkout_prep.live.address"),
			lang.T("assistant.shopping_checkout_prep.live.payment"),
			lang.T("assistant.shopping_checkout_prep.live.auth"),
			lang.T("assistant.shopping_checkout_prep.live.stop"),
		}, "\n"),
		"# " + lang.T("assistant.shopping_package.deck.execution"),
		lang.T("assistant.shopping_package.deck.execution_body"),
		"# " + lang.T("assistant.shopping_package.deck.boundary"),
		lang.T("assistant.shopping_package.deck.boundary_body"),
	}, "\n\n")
}

func assistantShoppingPackageSheetBody(query, searchURL string, candidates []browserauto.Link, fallback bool) string {
	lines := []string{
		lang.T("assistant.shopping_package.sheet.header"),
		"| --- | --- | --- | --- | --- | --- | --- | --- |",
	}
	if len(candidates) == 0 {
		candidates = assistantShoppingPackageFallbackCandidates(query, searchURL)
		fallback = true
	}
	for i, candidate := range assistantShoppingCandidateLines(candidates, 5) {
		title := cleanSpreadsheetCell(firstNonEmpty(strings.TrimSpace(candidate.Text), lang.T("assistant.shopping_package.candidate_fallback", i+1)))
		link := cleanSpreadsheetCell(firstNonEmpty(strings.TrimSpace(candidate.URL), searchURL))
		status := lang.T("assistant.shopping_package.sheet.status.product_page")
		if fallback {
			status = lang.T("assistant.shopping_package.sheet.status.search_link")
		}
		lines = append(lines, fmt.Sprintf("| %d | %s | %s | %s | %s | %s | %s | %s |",
			i+1,
			title,
			cleanSpreadsheetCell(status),
			cleanSpreadsheetCell(lang.T("assistant.shopping_package.sheet.delivery_check")),
			cleanSpreadsheetCell(lang.T("assistant.shopping_package.sheet.review_check")),
			cleanSpreadsheetCell(lang.T("assistant.shopping_package.sheet.option_check")),
			cleanSpreadsheetCell(lang.T("assistant.shopping_package.sheet.next_action")),
			link,
		))
	}
	return strings.Join(lines, "\n")
}

func assistantShoppingPackageMarkdownCandidates(candidates []browserauto.Link, fallback bool) []string {
	lines := []string{}
	for i, candidate := range assistantShoppingCandidateLines(candidates, 5) {
		title := firstNonEmpty(strings.TrimSpace(candidate.Text), lang.T("assistant.shopping_package.candidate_fallback", i+1))
		link := strings.TrimSpace(candidate.URL)
		line := fmt.Sprintf("%d. %s", i+1, title)
		if link != "" {
			line += " - " + link
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		lines = append(lines, "- "+lang.T("assistant.shopping_package.markdown.no_candidates"))
	}
	if fallback {
		lines = append(lines, "- "+lang.T("assistant.shopping_package.markdown.fallback_note"))
	}
	return lines
}

func assistantShoppingPackageSourceStatusLine(searchCandidateCount, fallbackCandidateCount int) string {
	return lang.T("assistant.shopping_package.source_status", searchCandidateCount, fallbackCandidateCount)
}

func assistantShoppingPackageBrowserStatusLine(actionReply string) string {
	if strings.TrimSpace(actionReply) == "" {
		return lang.T("assistant.shopping_package.browser_status.planned")
	}
	lower := strings.ToLower(signalReplyVisibleText(actionReply))
	if containsAny(lower, "승인", "권한", "approval", "permission", "requires approval") {
		return lang.T("assistant.shopping_package.browser_status.approval_required")
	}
	return lang.T("assistant.shopping_package.browser_status.opened")
}

func assistantShoppingPackageBoundaryStatusLine() string {
	return lang.T("assistant.shopping_package.purchase_boundary_status")
}

func assistantShoppingPackageVoiceSourceStatus(searchCandidateCount, fallbackCandidateCount int) string {
	return lang.T("assistant.shopping_package.voice.source_status", searchCandidateCount, fallbackCandidateCount)
}

func assistantShoppingPackageVoiceBrowserStatus(actionReply string) string {
	if strings.TrimSpace(actionReply) == "" {
		return lang.T("assistant.shopping_package.voice.browser_status.planned")
	}
	lower := strings.ToLower(signalReplyVisibleText(actionReply))
	if containsAny(lower, "승인", "권한", "approval", "permission", "requires approval") {
		return lang.T("assistant.shopping_package.voice.browser_status.approval_required")
	}
	return lang.T("assistant.shopping_package.voice.browser_status.opened")
}

func assistantShoppingPackageVoiceBoundaryStatus() string {
	return lang.T("assistant.shopping_package.voice.purchase_boundary_status")
}

func assistantShoppingPackageVoiceScript(query string, candidates []browserauto.Link, fallback bool, searchCandidateCount, fallbackCandidateCount int, actionReply string) string {
	lines := []string{
		lang.T("assistant.shopping_package.voice.intro"),
		lang.T("assistant.shopping_package.voice.query", query),
		assistantShoppingPackageVoiceSourceStatus(searchCandidateCount, fallbackCandidateCount),
		assistantShoppingPackageVoiceBrowserStatus(actionReply),
		assistantShoppingPackageVoiceBoundaryStatus(),
	}
	trimmed := assistantShoppingCandidateLines(candidates, 3)
	if len(trimmed) > 0 {
		lines = append(lines, lang.T("assistant.shopping_package.voice.candidates_intro"))
		for i, candidate := range trimmed {
			lines = append(lines, lang.T("assistant.shopping_package.voice.candidate", i+1, stripURLsForSpeech(firstNonEmpty(candidate.Text, lang.T("assistant.shopping_package.candidate_fallback", i+1)))))
		}
	}
	if fallback {
		lines = append(lines, lang.T("assistant.shopping_package.voice.fallback"))
	}
	lines = append(lines,
		lang.T("assistant.shopping_package.voice.criteria"),
		lang.T("assistant.shopping_package.voice.approval"),
		lang.T("assistant.shopping_package.voice.boundary"),
	)
	return strings.Join(lines, "\n\n")
}

func assistantShoppingPackageAttachments(doc, deck, sheet osauto.Result) []string {
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
