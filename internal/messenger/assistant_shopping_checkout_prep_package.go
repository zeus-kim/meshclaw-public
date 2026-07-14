package messenger

import (
	"context"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/meshclaw/meshclaw/internal/browserauto"
	"github.com/meshclaw/meshclaw/internal/evidence"
	"github.com/meshclaw/meshclaw/internal/lang"
	"github.com/meshclaw/meshclaw/internal/osauto"
	"github.com/meshclaw/meshclaw/internal/tts"
)

func assistantCoupangCheckoutPrepPackageReply(opts ListenOptions, request string) (string, bool) {
	if !isAssistantCoupangCheckoutPrepPackageRequest(request) {
		return "", false
	}
	query := assistantCoupangCheckoutPrepPackageQuery(request)
	if strings.TrimSpace(query) == "" {
		query = lang.T("assistant.shopping_checkout_prep.default_query")
	}
	searchURL := coupangSearchURL(query)
	selection := shoppingCandidateFollowUpIndex(strings.ToLower(request))
	if selection < 0 {
		selection = 0
	}

	actionReply := ""
	if reply, handled := runSignalArgosAction(opts, searchURL+" 열어줘", 0); handled {
		actionReply = signalReplyVisibleText(reply)
		if lang.Current() != "ko" && assistantCheckoutPrepContainsHangul(actionReply) {
			actionReply = lang.T("assistant.shopping_checkout_prep.action_requires_approval", searchURL)
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
	sourceStatus := assistantCoupangCheckoutPrepSourceStatusLine(searchCandidateCount, fallbackCandidateCount)
	browserStatus := assistantShoppingPackageBrowserStatusLine(actionReply)
	boundaryStatus := assistantShoppingPackageBoundaryStatusLine()
	if selection >= len(candidates) {
		selection = 0
	}
	selected := browserauto.Link{}
	if len(candidates) > 0 {
		selected = candidates[selection]
	}
	title := lang.T("assistant.shopping_checkout_prep.title")

	restorePreview := temporarilySkipPreviewImages()
	doc := osauto.CreateArgosDocument(ctx, lang.T("assistant.shopping_checkout_prep.artifact.doc_title", title), assistantCoupangCheckoutPrepReportBody(request, query, searchURL, candidates, selected, selection, fallback, sourceStatus, browserStatus, boundaryStatus))
	deck := osauto.CreatePresentation(ctx, lang.T("assistant.shopping_checkout_prep.artifact.deck_title", title), assistantCoupangCheckoutPrepDeckBody(query, searchURL, candidates, selected, selection, fallback, sourceStatus, browserStatus, boundaryStatus), lang.T("assistant.shopping_checkout_prep.artifact.audience"), 7, "")
	sheet := osauto.CreateSpreadsheet(ctx, lang.T("assistant.shopping_checkout_prep.artifact.sheet_title", title), assistantCoupangCheckoutPrepSheetBody(query, searchURL, candidates, selected, selection, fallback))
	safetyPath, safetyErr := writeAssistantCoupangCheckoutPrepSafetySVG(title, query, searchURL, selected, selection)
	restorePreview()

	wantsVoice := assistantBrowserResearchPackageWantsVoice(request)
	voiceScript := ""
	var audio tts.Result
	var audioErr error
	if wantsVoice {
		voiceScript = assistantCoupangCheckoutPrepVoiceScript(query, selected, selection, fallback, searchCandidateCount, fallbackCandidateCount, actionReply)
		audio, audioErr = tts.Synthesize(tts.Options{
			Text:     voiceScript,
			Engine:   "edge-tts",
			Basename: "coupang-checkout-prep-" + time.Now().UTC().Format("20060102T150405Z"),
		})
	}

	attachments := assistantShoppingPackageAttachments(doc, deck, sheet)
	if strings.TrimSpace(safetyPath) != "" && safetyErr == nil {
		attachments = uniqueShowcaseAttachments(append(attachments, safetyPath))
	}
	if strings.TrimSpace(audio.Path) != "" && audioErr == nil {
		attachments = uniqueShowcaseAttachments(append(attachments, audio.Path))
	}
	rememberAssistantArtifact(opts, title, "coupang_checkout_prep_package", doc, deck, sheet)
	record, storeErr := evidence.Store("assistant-coupang-checkout-prep-package", firstNonEmpty(opts.TargetID, "assistant"), title, map[string]interface{}{
		"kind":                     "assistant_coupang_checkout_prep_package",
		"request":                  request,
		"query":                    query,
		"search_url":               searchURL,
		"selection":                selection + 1,
		"selected":                 selected,
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

	lines := assistantCoupangCheckoutPrepSummaryLines(query, searchURL, candidates, selected, selection, fallback, actionReply, doc, deck, sheet, safetyPath, safetyErr, wantsVoice, audio, audioErr, voiceScript, sourceStatus, browserStatus, boundaryStatus)
	if targetRef := inferAssistantSignalTargetRef(request); targetRef != "" {
		return formatAssistantCoupangCheckoutPrepSendResult(opts, targetRef, lines, attachments, record, storeErr), true
	}
	lines = appendAssistantWorkflowMobileLinkLines(lines, assistantWorkflowVisibleMobileLinkLines(attachments, 6))
	lines = appendVoiceReportAttachmentMarkers(lines, attachments)
	lines = appendAssistantEvidenceNote(lines, record, storeErr)
	return strings.Join(compactBlankLines(lines), "\n"), true
}

func isAssistantCoupangCheckoutPrepPackageRequest(request string) bool {
	lower := strings.ToLower(strings.TrimSpace(request))
	if lower == "" || !containsAny(lower, "쿠팡", "coupang") {
		return false
	}
	if isAssistantMeetingMinutesPackageRequest(request) {
		return false
	}
	if isPurchaseFinalApprovalText(lower) || containsAny(lower, "구매 완료", "주문 완료", "결제 완료", "purchase completed", "order completed") {
		return false
	}
	prepSignal := containsAny(lower,
		"장바구니 직전", "장바구니 전", "구매 직전", "결제 직전", "주문 직전",
		"장바구니까지 준비", "장바구니 준비", "구매 준비", "구매 테스트", "실제 구매 테스트",
		"진짜 구매", "실제 구매", "실구매", "실전 구매", "라이브 구매", "라이브 테스트",
		"cart prep", "checkout prep", "prepare the cart", "before checkout", "real purchase", "live purchase", "purchase rehearsal", "live test",
	)
	artifactSignal := inferAssistantSignalTargetRef(request) != "" ||
		assistantBrowserResearchPackageWantsVoice(request) ||
		containsAny(lower, "문서", "보고서", "체크리스트", "ppt", "pptx", "엑셀", "xlsx", "csv", "패키지", "파일", "첨부", "보고방", "브리핑방")
	return prepSignal && artifactSignal
}

func assistantCoupangCheckoutPrepPackageQuery(request string) string {
	query := assistantShoppingPackageQuery(request)
	replacer := strings.NewReplacer(
		"장바구니", " ", "구매", " ", "결제", " ", "주문", " ", "직전까지", " ", "직전", " ", "전까지", " ",
		"화면까지", " ", "화면", " ", "체크아웃", " ", "checkout", " ", "cart", " ", "prep", " ",
		"1번", " ", "2번", " ", "3번", " ", "첫번째", " ", "첫 번째", " ", "두번째", " ", "두 번째", " ",
		"후보", " ", "상품", " ", "기준으로", " ", "기준", " ", "선택해서", " ", "선택", " ",
		"하지", " ", "하지마", " ", "하지 마", " ", "말고", " ", "않고", " ", "마", " ",
		"실제", " ", "실구매", " ", "실전", " ", "진짜", " ", "라이브", " ", "테스트", " ", "내일", " ",
		"최종", " ", "읽을", " ", "값", " ", "브리핑", " ", "edge", " ", "tts", " ",
		"할 거니까", " ", "할거니까", " ", "할 거라서", " ", "할거라서", " ", "할 예정이니까", " ", "할 예정이라서", " ",
		"purchase", " ", "real", " ", "live", " ", "rehearsal", " ", "tomorrow", " ",
		"Send", " ", "send", " ", "checklist", " ", "PPT", " ", "ppt", " ", "CSV", " ", "csv", " ", "voice", " ", "briefing room", " ", "briefing", " ", "Do not pay", " ", "do not pay", " ", "to the", " ",
		"mobile", " ", "Mobile", " ", "safety", " ", "Safety", " ", "card", " ", "Card", " ",
		"모바일", " ", "svg", " ", "SVG", " ", "안전 카드", " ", "안전카드", " ", "안전", " ", "체크", " ", "안전 체크", " ", "카드", " ",
		"음성으로도", " ", "음성", " ", "준비해서", " ", "준비해줘", " ", "보고방에", " ", "보고방", " ", "보내줘", " ",
	)
	query = strings.Join(strings.Fields(replacer.Replace(query)), " ")
	fields := []string{}
	for _, field := range strings.Fields(query) {
		clean := strings.Trim(field, " :：,，.。")
		switch strings.ToLower(clean) {
		case "a", "an", "the", "for", "with", "and", "to":
			continue
		}
		if containsAny(clean, "를", "을", "은", "는", "이", "가", "와", "과", "마") && len([]rune(clean)) == 1 {
			continue
		}
		for _, suffix := range []string{"으로", "까지", "에서", "에게", "에는", "으로도", "를", "을", "은", "는", "가", "이", "와", "과"} {
			if len([]rune(clean)) > len([]rune(suffix))+1 && strings.HasSuffix(clean, suffix) {
				clean = strings.TrimSuffix(clean, suffix)
				break
			}
		}
		if clean != "" {
			fields = append(fields, clean)
		}
	}
	return strings.Join(fields, " ")
}

func assistantCoupangCheckoutPrepSummaryLines(query, searchURL string, candidates []browserauto.Link, selected browserauto.Link, selection int, fallback bool, actionReply string, doc, deck, sheet osauto.Result, safetyPath string, safetyErr error, wantsVoice bool, audio tts.Result, audioErr error, voiceScript, sourceStatus, browserStatus, boundaryStatus string) []string {
	lines := []string{
		lang.T("assistant.shopping_checkout_prep.created"),
		lang.T("assistant.shopping_checkout_prep.subtitle"),
		"",
		lang.T("assistant.shopping.coupang.query", query),
		lang.T("assistant.shopping.coupang.link", searchURL),
		lang.T("assistant.shopping_checkout_prep.selected", selection+1, firstNonEmpty(strings.TrimSpace(selected.Text), lang.T("assistant.shopping_checkout_prep.selected_unknown"))),
	}
	if strings.TrimSpace(selected.URL) != "" {
		lines = append(lines, lang.T("assistant.shopping_checkout_prep.selected_link", strings.TrimSpace(selected.URL)))
	}
	lines = append(lines, sourceStatus, browserStatus, boundaryStatus)
	lines = append(lines, assistantCoupangCheckoutPrepLiveCardLines()...)
	lines = append(lines, "", lang.T("assistant.shopping_checkout_prep.choice_gate_title"))
	lines = append(lines, assistantCoupangCheckoutPrepChoiceGateLines()...)
	lines = append(lines, "", lang.T("assistant.shopping_checkout_prep.files"))
	if doc.OK && doc.Error == "" {
		lines = append(lines, "- "+lang.T("assistant.shopping_checkout_prep.file.doc"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.shopping_checkout_prep.file.doc_failed", firstNonEmpty(doc.Error, doc.Stderr, "unknown error")))
	}
	if deck.OK && deck.Error == "" {
		lines = append(lines, "- "+lang.T("assistant.shopping_checkout_prep.file.ppt"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.shopping_checkout_prep.file.ppt_failed", firstNonEmpty(deck.Error, deck.Stderr, "unknown error")))
	}
	if sheet.OK && sheet.Error == "" {
		lines = append(lines, "- "+lang.T("assistant.shopping_checkout_prep.file.sheet"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.shopping_checkout_prep.file.sheet_failed", firstNonEmpty(sheet.Error, sheet.Stderr, "unknown error")))
	}
	if strings.TrimSpace(safetyPath) != "" && safetyErr == nil {
		lines = append(lines, "- "+lang.T("assistant.shopping_checkout_prep.file.safety"))
	} else if safetyErr != nil {
		lines = append(lines, "- "+lang.T("assistant.shopping_checkout_prep.file.safety_failed", firstNonEmpty(errorString(safetyErr), "unknown error")))
	}
	if wantsVoice {
		if audioErr == nil && strings.TrimSpace(audio.Path) != "" {
			lines = append(lines, "- "+lang.T("assistant.shopping_checkout_prep.file.audio"))
		} else {
			lines = append(lines, "- "+lang.T("assistant.shopping_checkout_prep.file.audio_failed", firstNonEmpty(errorString(audioErr), "unknown error")))
		}
	}
	lines = append(lines, "", lang.T("assistant.shopping.coupang.candidates_title"))
	lines = append(lines, assistantCoupangCheckoutPrepCandidateSummary(candidates, fallback)...)
	lines = append(lines,
		"",
		lang.T("assistant.shopping_checkout_prep.checklist_title"),
		"- "+lang.T("assistant.shopping.cart.check.option"),
		"- "+lang.T("assistant.shopping.cart.check.total"),
		"- "+lang.T("assistant.shopping.cart.check.delivery"),
		"- "+lang.T("assistant.shopping.cart.check.address"),
		"- "+lang.T("assistant.shopping.cart.check.payment"),
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
		lang.T("assistant.shopping_checkout_prep.signal_template_title"),
		"- "+lang.T("assistant.shopping_checkout_prep.signal_template.product"),
		"- "+lang.T("assistant.shopping_checkout_prep.signal_template.option"),
		"- "+lang.T("assistant.shopping_checkout_prep.signal_template.total"),
		"- "+lang.T("assistant.shopping_checkout_prep.signal_template.delivery"),
		"- "+lang.T("assistant.shopping_checkout_prep.signal_template.address"),
		"- "+lang.T("assistant.shopping_checkout_prep.signal_template.payment"),
		"- "+lang.T("assistant.shopping_checkout_prep.signal_template.button"),
		"",
		lang.T("assistant.shopping_checkout_prep.choice_gate_title"),
	)
	lines = append(lines, assistantCoupangCheckoutPrepChoiceGateLines()...)
	lines = append(lines,
		"",
		lang.T("assistant.shopping_checkout_prep.approval_draft_title"),
		assistantCoupangCheckoutPrepApprovalDraft(selection),
		"",
		lang.T("assistant.shopping_checkout_prep.live_ready_title"),
		"- "+lang.T("assistant.shopping_checkout_prep.live.browser"),
		"- "+lang.T("assistant.shopping_checkout_prep.live.address"),
		"- "+lang.T("assistant.shopping_checkout_prep.live.payment"),
		"- "+lang.T("assistant.shopping_checkout_prep.live.auth"),
		"- "+lang.T("assistant.shopping_checkout_prep.live.stop"),
		"",
		lang.T("assistant.shopping_checkout_prep.readiness_title"),
	)
	lines = append(lines, assistantCoupangCheckoutPrepLiveReadinessLines()...)
	lines = append(lines,
		"",
		lang.T("assistant.shopping_checkout_prep.next"),
		"- "+lang.T("assistant.shopping_package.next.detail"),
		"- "+lang.T("assistant.shopping_package.next.checkout"),
		"- "+lang.T("assistant.shopping.decision.next_signal.address_choice"),
		"- "+lang.T("assistant.shopping.decision.next_signal.payment_choice"),
		"- "+lang.T("assistant.shopping_checkout_prep.next.decision"),
		"",
		lang.T("assistant.shopping_checkout_prep.boundary"),
	)
	if strings.TrimSpace(actionReply) != "" {
		lines = append(lines, "", lang.T("assistant.shopping.coupang.action_title"), strings.TrimSpace(actionReply))
	}
	if strings.TrimSpace(voiceScript) != "" {
		lines = append(lines, "", lang.T("assistant.shopping_package.voice_preview"), trimForContext(voiceScript, 520))
	}
	return lines
}

func assistantCoupangCheckoutPrepLiveCardLines() []string {
	return []string{
		"",
		lang.T("assistant.shopping_checkout_prep.live_card.title"),
		"- " + lang.T("assistant.shopping_checkout_prep.live_card.ready"),
		"- " + lang.T("assistant.shopping_checkout_prep.live_card.user"),
		"- " + lang.T("assistant.shopping_checkout_prep.live_card.signal"),
		"- " + lang.T("assistant.shopping_checkout_prep.live_card.stop"),
	}
}

func writeAssistantCoupangCheckoutPrepSafetySVG(title, query, searchURL string, selected browserauto.Link, selection int) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Documents", "Argos Vault", "Charts")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	base := safeAssistantDepartmentFilename(title)
	path := filepath.Join(dir, base+"-"+time.Now().Format("20060102-150405")+"-safety.svg")
	content := renderAssistantCoupangCheckoutPrepSafetySVG(title, query, searchURL, selected, selection)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return "", err
	}
	return path, nil
}

func renderAssistantCoupangCheckoutPrepSafetySVG(title, query, searchURL string, selected browserauto.Link, selection int) string {
	product := firstNonEmpty(strings.TrimSpace(selected.Text), lang.T("assistant.shopping_checkout_prep.selected_unknown"))
	link := firstNonEmpty(strings.TrimSpace(selected.URL), searchURL)
	width := 780
	height := 560
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`, width, height, width, height))
	b.WriteString("\n")
	b.WriteString(`<rect width="780" height="560" fill="#f8fafc"/>` + "\n")
	b.WriteString(`<rect x="24" y="24" width="732" height="512" rx="20" fill="#ffffff" stroke="#d9e2ef"/>` + "\n")
	b.WriteString(fmt.Sprintf(`<text x="48" y="66" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="27" font-weight="800" fill="#0f172a">%s</text>`, html.EscapeString(shortenSVGText(title, 36))))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`<text x="48" y="96" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="15" fill="#64748b">%s</text>`, html.EscapeString(lang.T("assistant.shopping_checkout_prep.safety.subtitle"))))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`<text x="48" y="126" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="15" font-weight="750" fill="#2563eb">%s</text>`, html.EscapeString(lang.T("assistant.shopping.coupang.query", query))))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`<text x="48" y="150" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="14" fill="#64748b">%s</text>`, html.EscapeString(shortenSVGText(lang.T("assistant.shopping_checkout_prep.selected", selection+1, product), 72))))
	b.WriteString("\n")

	steps := []struct {
		Label string
		Body  string
		Color string
	}{
		{lang.T("assistant.shopping_checkout_prep.safety.step.search"), lang.T("assistant.shopping_checkout_prep.safety.step.search.body"), "#2563eb"},
		{lang.T("assistant.shopping_checkout_prep.safety.step.detail"), lang.T("assistant.shopping_checkout_prep.safety.step.detail.body"), "#0f766e"},
		{lang.T("assistant.shopping_checkout_prep.safety.step.final"), lang.T("assistant.shopping_checkout_prep.safety.step.final.body"), "#d97706"},
		{lang.T("assistant.shopping_checkout_prep.safety.step.stop"), lang.T("assistant.shopping_checkout_prep.safety.step.stop.body"), "#dc2626"},
	}
	for i, step := range steps {
		x := 48 + i*172
		y := 188
		b.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="148" height="150" rx="16" fill="#f8fafc" stroke="#d9e2ef"/>`, x, y))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf(`<circle cx="%d" cy="%d" r="18" fill="%s"/>`, x+28, y+32, html.EscapeString(step.Color)))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf(`<text x="%d" y="%d" text-anchor="middle" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="14" font-weight="800" fill="#ffffff">%d</text>`, x+28, y+37, i+1))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf(`<text x="%d" y="%d" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="16" font-weight="800" fill="#0f172a">%s</text>`, x+18, y+72, html.EscapeString(shortenSVGText(step.Label, 10))))
		b.WriteString("\n")
		for j, line := range wrapSVGText(step.Body, 15, 3) {
			b.WriteString(fmt.Sprintf(`<text x="%d" y="%d" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="13" fill="#475569">%s</text>`, x+18, y+100+j*20, html.EscapeString(line)))
			b.WriteString("\n")
		}
	}

	b.WriteString(`<rect x="48" y="370" width="684" height="104" rx="16" fill="#fff7ed" stroke="#fed7aa"/>` + "\n")
	b.WriteString(fmt.Sprintf(`<text x="72" y="398" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="18" font-weight="800" fill="#9a3412">%s</text>`, html.EscapeString(lang.T("assistant.shopping_checkout_prep.safety.check_title"))))
	b.WriteString("\n")
	for i, check := range []string{
		lang.T("assistant.shopping.checkout.check.product_only"),
		lang.T("assistant.shopping.checkout.check.option"),
		lang.T("assistant.shopping.checkout.check.quantity"),
		lang.T("assistant.shopping.checkout.check.total"),
		lang.T("assistant.shopping.checkout.check.delivery"),
		lang.T("assistant.shopping.checkout.check.address"),
		lang.T("assistant.shopping.checkout.check.payment"),
		lang.T("assistant.shopping.checkout.check.button"),
	} {
		x := 72
		y := 426 + i*19
		if i >= 3 {
			x = 390
			y = 426 + (i-3)*19
		}
		b.WriteString(fmt.Sprintf(`<text x="%d" y="%d" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="13" fill="#7c2d12">• %s</text>`, x, y, html.EscapeString(shortenSVGText(check, 31))))
		b.WriteString("\n")
	}
	b.WriteString(fmt.Sprintf(`<text x="48" y="506" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="13" fill="#64748b">%s</text>`, html.EscapeString(shortenSVGText(link, 92))))
	b.WriteString("\n")
	for i, line := range wrapSVGText(lang.T("assistant.shopping_checkout_prep.safety.footer"), 82, 2) {
		b.WriteString(fmt.Sprintf(`<text x="48" y="%d" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="14" font-weight="700" fill="#dc2626">%s</text>`, 524+i*18, html.EscapeString(line)))
		b.WriteString("\n")
	}
	b.WriteString("</svg>\n")
	return b.String()
}

func formatAssistantCoupangCheckoutPrepSendResult(opts ListenOptions, targetRef string, summary []string, attachments []string, record evidence.Record, storeErr error) string {
	target, candidates, targetErr := resolveAssistantVoiceTarget(targetRef)
	if targetErr != nil {
		lines := []string{lang.T("assistant.shopping_checkout_prep.send_target_failed")}
		lines = append(lines, formatAssistantVoiceTargetCandidates(candidates)...)
		lines = append(lines, "", lang.T("assistant.shopping_checkout_prep.attach_here"))
		lines = appendAssistantWorkflowMobileLinkLines(lines, assistantWorkflowVisibleMobileLinkLines(attachments, 6))
		lines = appendVoiceReportAttachmentMarkers(lines, attachments)
		return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
	}
	targetLabel := assistantScheduleLocalizedTarget(target.ID, target.Label)
	if !opts.Execute {
		lines := []string{
			lang.T("assistant.shopping_checkout_prep.send_ready"),
			lang.T("assistant.shopping_checkout_prep.target", targetLabel),
		}
		if OneWayReportTarget(target) {
			lines = append(lines, lang.T("assistant.shopping_checkout_prep.one_way"))
		}
		lines = append(lines, "", lang.T("assistant.shopping_checkout_prep.to_send"))
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
		"kind":             "assistant_coupang_checkout_prep_package_send",
		"target":           targetRef,
		"resolved_target":  target,
		"attachment_count": len(attachments),
		"mobile_links":     mobileLinkLines,
		"send":             send,
		"send_error":       errorString(sendErr),
		"created_at":       time.Now().UTC(),
	}
	sendRecord, sendStoreErr := evidence.Store("assistant-coupang-checkout-prep-package-send", firstNonEmpty(opts.TargetID, "assistant"), target.ID, payload)
	if sendErr != nil {
		lines := []string{
			lang.T("assistant.shopping_checkout_prep.send_failed"),
			lang.T("assistant.shopping_checkout_prep.target", targetLabel),
			lang.T("assistant.shopping_checkout_prep.problem", sendErr.Error()),
			"",
			lang.T("assistant.shopping_checkout_prep.attach_here"),
		}
		lines = appendAssistantWorkflowMobileLinkLines(lines, mobileLinkLines)
		lines = appendVoiceReportAttachmentMarkers(lines, attachments)
		return strings.Join(appendAssistantEvidenceNote(lines, sendRecord, sendStoreErr), "\n")
	}
	lines := []string{
		lang.T("assistant.shopping_checkout_prep.sent"),
		lang.T("assistant.shopping_checkout_prep.target", targetLabel),
	}
	if id := strings.TrimSpace(send.Stdout); id != "" {
		lines = append(lines, lang.T("assistant.workflow.signal_id", id))
	}
	return strings.Join(appendAssistantEvidenceNote(lines, sendRecord, sendStoreErr), "\n")
}

func assistantCoupangCheckoutPrepReportBody(request, query, searchURL string, candidates []browserauto.Link, selected browserauto.Link, selection int, fallback bool, sourceStatus, browserStatus, boundaryStatus string) string {
	now := time.Now().In(time.FixedZone("KST", 9*60*60))
	lines := []string{
		"# " + lang.T("assistant.shopping_checkout_prep.title"),
		"",
		"- " + lang.T("assistant.workflow.preview") + " " + now.Format("2006-01-02 15:04 KST"),
		"- " + lang.T("assistant.shopping_checkout_prep.report.request", strings.TrimSpace(request)),
		"- " + lang.T("assistant.shopping.coupang.query", query),
		"- " + lang.T("assistant.shopping.coupang.link", searchURL),
		"- " + lang.T("assistant.shopping_checkout_prep.selected", selection+1, firstNonEmpty(strings.TrimSpace(selected.Text), lang.T("assistant.shopping_checkout_prep.selected_unknown"))),
		"- " + sourceStatus,
		"- " + browserStatus,
		"- " + boundaryStatus,
	}
	if strings.TrimSpace(selected.URL) != "" {
		lines = append(lines, "- "+lang.T("assistant.shopping_checkout_prep.selected_link", strings.TrimSpace(selected.URL)))
	}
	lines = append(lines,
		"",
		"## "+lang.T("assistant.shopping_checkout_prep.checklist_title"),
		"- "+lang.T("assistant.shopping.cart.check.option"),
		"- "+lang.T("assistant.shopping.cart.check.total"),
		"- "+lang.T("assistant.shopping.cart.check.delivery"),
		"- "+lang.T("assistant.shopping.cart.check.address"),
		"- "+lang.T("assistant.shopping.cart.check.payment"),
		"",
		"## "+lang.T("assistant.shopping.coupang.candidates_title"),
	)
	lines = append(lines, assistantCoupangCheckoutPrepMarkdownCandidates(candidates, fallback)...)
	lines = append(lines,
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
		"## "+lang.T("assistant.shopping_checkout_prep.signal_template_title"),
		"- "+lang.T("assistant.shopping_checkout_prep.signal_template.product"),
		"- "+lang.T("assistant.shopping_checkout_prep.signal_template.option"),
		"- "+lang.T("assistant.shopping_checkout_prep.signal_template.total"),
		"- "+lang.T("assistant.shopping_checkout_prep.signal_template.delivery"),
		"- "+lang.T("assistant.shopping_checkout_prep.signal_template.address"),
		"- "+lang.T("assistant.shopping_checkout_prep.signal_template.payment"),
		"- "+lang.T("assistant.shopping_checkout_prep.signal_template.button"),
		"",
		"## "+lang.T("assistant.shopping_checkout_prep.report.approval_draft"),
		assistantCoupangCheckoutPrepApprovalDraft(selection),
		"",
		"## "+lang.T("assistant.shopping_checkout_prep.live_ready_title"),
		"- "+lang.T("assistant.shopping_checkout_prep.live.browser"),
		"- "+lang.T("assistant.shopping_checkout_prep.live.address"),
		"- "+lang.T("assistant.shopping_checkout_prep.live.payment"),
		"- "+lang.T("assistant.shopping_checkout_prep.live.auth"),
		"- "+lang.T("assistant.shopping_checkout_prep.live.stop"),
		"",
		"## "+lang.T("assistant.shopping_checkout_prep.choice_gate_title"),
	)
	lines = append(lines, assistantCoupangCheckoutPrepChoiceGateLines()...)
	lines = append(lines,
		"",
		"## "+lang.T("assistant.shopping_checkout_prep.readiness_title"),
		"- "+lang.T("assistant.shopping_checkout_prep.readiness.login"),
		"- "+lang.T("assistant.shopping_checkout_prep.readiness.assistant_can"),
		"- "+lang.T("assistant.shopping_checkout_prep.readiness.user_input"),
		"- "+lang.T("assistant.shopping_checkout_prep.readiness.final_gate"),
		"",
		"## "+lang.T("assistant.shopping.coupang.next_title"),
		"- "+lang.T("assistant.shopping_package.next.detail"),
		"- "+lang.T("assistant.shopping_package.next.checkout"),
		"- "+lang.T("assistant.shopping.decision.next_signal.address_choice"),
		"- "+lang.T("assistant.shopping.decision.next_signal.payment_choice"),
		"- "+lang.T("assistant.shopping_checkout_prep.next.decision"),
		"",
		"## "+lang.T("assistant.workflow.file.graph"),
		"```mermaid",
		"flowchart TD",
		"  A["+assistantMermaidEscape(lang.T("assistant.shopping_checkout_prep.graph.search"))+"] --> B["+assistantMermaidEscape(lang.T("assistant.shopping_checkout_prep.graph.compare"))+"]",
		"  B --> C["+assistantMermaidEscape(lang.T("assistant.shopping_checkout_prep.graph.detail"))+"]",
		"  C --> D["+assistantMermaidEscape(lang.T("assistant.shopping_checkout_prep.graph.pre_cart"))+"]",
		"  D --> E["+assistantMermaidEscape(lang.T("assistant.shopping_checkout_prep.graph.final_values"))+"]",
		"  E --> F{"+assistantMermaidEscape(lang.T("assistant.shopping_checkout_prep.graph.approval"))+"}",
		"  F -- "+assistantMermaidEscape(lang.T("assistant.shopping_checkout_prep.graph.no"))+" --> G["+assistantMermaidEscape(lang.T("assistant.shopping_checkout_prep.graph.stop"))+"]",
		"  F -- "+assistantMermaidEscape(lang.T("assistant.shopping_checkout_prep.graph.yes"))+" --> H["+assistantMermaidEscape(lang.T("assistant.shopping_checkout_prep.graph.confirm"))+"]",
		"```",
		"",
		"## "+lang.T("assistant.shopping_checkout_prep.boundary"),
		"- "+lang.T("assistant.shopping_checkout_prep.report.boundary.no_auto"),
		"- "+lang.T("assistant.shopping_checkout_prep.report.boundary.final_click"),
	)
	return strings.Join(lines, "\n")
}

func assistantCoupangCheckoutPrepDeckBody(query, searchURL string, candidates []browserauto.Link, selected browserauto.Link, selection int, fallback bool, sourceStatus, browserStatus, boundaryStatus string) string {
	return strings.Join([]string{
		"# " + lang.T("assistant.shopping.coupang.query", query),
		strings.Join([]string{searchURL, sourceStatus, browserStatus, boundaryStatus}, "\n"),
		"# " + lang.T("assistant.shopping_checkout_prep.selected", selection+1, firstNonEmpty(strings.TrimSpace(selected.Text), lang.T("assistant.shopping_checkout_prep.selected_unknown"))),
		firstNonEmpty(strings.TrimSpace(selected.URL), searchURL),
		"# " + lang.T("assistant.shopping_checkout_prep.checklist_title"),
		strings.Join([]string{
			lang.T("assistant.shopping.cart.check.option"),
			lang.T("assistant.shopping.cart.check.total"),
			lang.T("assistant.shopping.cart.check.delivery"),
			lang.T("assistant.shopping.cart.check.address"),
			lang.T("assistant.shopping.cart.check.payment"),
		}, "\n"),
		"# " + lang.T("assistant.shopping_checkout_prep.final_values_title"),
		strings.Join([]string{
			lang.T("assistant.shopping.checkout.check.product_only"),
			lang.T("assistant.shopping.checkout.check.option"),
			lang.T("assistant.shopping.checkout.check.quantity"),
			lang.T("assistant.shopping.checkout.check.total"),
			lang.T("assistant.shopping.checkout.check.delivery"),
			lang.T("assistant.shopping.checkout.check.address"),
			lang.T("assistant.shopping.checkout.check.payment"),
			lang.T("assistant.shopping.checkout.check.button"),
		}, "\n"),
		"# " + lang.T("assistant.shopping_checkout_prep.signal_template_title"),
		strings.Join([]string{
			lang.T("assistant.shopping_checkout_prep.signal_template.product"),
			lang.T("assistant.shopping_checkout_prep.signal_template.option"),
			lang.T("assistant.shopping_checkout_prep.signal_template.total"),
			lang.T("assistant.shopping_checkout_prep.signal_template.delivery"),
			lang.T("assistant.shopping_checkout_prep.signal_template.address"),
			lang.T("assistant.shopping_checkout_prep.signal_template.payment"),
			lang.T("assistant.shopping_checkout_prep.signal_template.button"),
		}, "\n"),
		"# " + lang.T("assistant.shopping_checkout_prep.report.approval_draft"),
		assistantCoupangCheckoutPrepApprovalDraft(selection),
		"# " + lang.T("assistant.shopping_checkout_prep.live_ready_title"),
		strings.Join([]string{
			lang.T("assistant.shopping_checkout_prep.live.browser"),
			lang.T("assistant.shopping_checkout_prep.live.address"),
			lang.T("assistant.shopping_checkout_prep.live.payment"),
			lang.T("assistant.shopping_checkout_prep.live.auth"),
			lang.T("assistant.shopping_checkout_prep.live.stop"),
		}, "\n"),
		"# " + lang.T("assistant.shopping_checkout_prep.choice_gate_title"),
		strings.Join([]string{
			lang.T("assistant.shopping_checkout_prep.choice_gate.address"),
			lang.T("assistant.shopping_checkout_prep.choice_gate.payment"),
			lang.T("assistant.shopping_checkout_prep.choice_gate.no_raw"),
		}, "\n"),
		"# " + lang.T("assistant.shopping_checkout_prep.readiness_title"),
		strings.Join([]string{
			lang.T("assistant.shopping_checkout_prep.readiness.login"),
			lang.T("assistant.shopping_checkout_prep.readiness.assistant_can"),
			lang.T("assistant.shopping_checkout_prep.readiness.user_input"),
			lang.T("assistant.shopping_checkout_prep.readiness.final_gate"),
		}, "\n"),
		"# " + lang.T("assistant.shopping.coupang.candidates_title"),
		strings.Join(assistantCoupangCheckoutPrepMarkdownCandidates(candidates, fallback), "\n"),
		"# " + lang.T("assistant.shopping_checkout_prep.next"),
		strings.Join([]string{
			lang.T("assistant.shopping_package.next.detail"),
			lang.T("assistant.shopping_package.next.checkout"),
			lang.T("assistant.shopping.decision.next_signal.address_choice"),
			lang.T("assistant.shopping.decision.next_signal.payment_choice"),
			lang.T("assistant.shopping_checkout_prep.next.decision"),
		}, "\n"),
		"# " + lang.T("assistant.shopping_checkout_prep.boundary"),
		lang.T("assistant.shopping_checkout_prep.deck.boundary_detail"),
	}, "\n\n")
}

func assistantCoupangCheckoutPrepSheetBody(query, searchURL string, candidates []browserauto.Link, selected browserauto.Link, selection int, fallback bool) string {
	lines := []string{
		lang.T("assistant.shopping_checkout_prep.sheet.header"),
		"| --- | --- | --- | --- | --- |",
		fmt.Sprintf("| 1 | %s | %s | %s | %s |", cleanSpreadsheetCell(lang.T("assistant.shopping_checkout_prep.sheet.search_query")), cleanSpreadsheetCell(query), cleanSpreadsheetCell(lang.T("assistant.shopping_checkout_prep.sheet.search_action")), cleanSpreadsheetCell(searchURL)),
		fmt.Sprintf("| 2 | %s | %s | %s | %s |", cleanSpreadsheetCell(lang.T("assistant.shopping_checkout_prep.sheet.selected")), cleanSpreadsheetCell(lang.T("assistant.shopping_checkout_prep.sheet.selected_value", selection+1, firstNonEmpty(selected.Text, lang.T("assistant.shopping_checkout_prep.selected_unknown")))), cleanSpreadsheetCell(lang.T("assistant.shopping_checkout_prep.sheet.selected_action")), cleanSpreadsheetCell(firstNonEmpty(selected.URL, searchURL))),
		fmt.Sprintf("| 3 | %s | %s | %s |  |", cleanSpreadsheetCell(lang.T("assistant.shopping_checkout_prep.live_ready_title")), cleanSpreadsheetCell(lang.T("assistant.shopping_checkout_prep.live.browser")), cleanSpreadsheetCell(lang.T("assistant.shopping_checkout_prep.live.auth"))),
		fmt.Sprintf("| 4 | %s | %s | %s |  |", cleanSpreadsheetCell(lang.T("assistant.shopping_checkout_prep.sheet.pre_cart")), cleanSpreadsheetCell(lang.T("assistant.shopping_checkout_prep.sheet.pre_cart_values")), cleanSpreadsheetCell(lang.T("assistant.shopping_checkout_prep.sheet.pre_cart_action"))),
		fmt.Sprintf("| 5 | %s | %s | %s |  |", cleanSpreadsheetCell(lang.T("assistant.shopping_checkout_prep.sheet.final_screen")), cleanSpreadsheetCell(lang.T("assistant.shopping_checkout_prep.sheet.final_values")), cleanSpreadsheetCell(lang.T("assistant.shopping_checkout_prep.sheet.final_action"))),
		fmt.Sprintf("| 6 | %s | %s | %s |  |", cleanSpreadsheetCell(lang.T("assistant.shopping_checkout_prep.sheet.approval_boundary")), cleanSpreadsheetCell(lang.T("assistant.shopping_checkout_prep.live.stop")), cleanSpreadsheetCell(lang.T("assistant.shopping_checkout_prep.sheet.no_payment_action"))),
		fmt.Sprintf("| 7 | %s | %s | %s |  |", cleanSpreadsheetCell(lang.T("assistant.shopping_checkout_prep.readiness_title")), cleanSpreadsheetCell(lang.T("assistant.shopping_checkout_prep.readiness.login")), cleanSpreadsheetCell(lang.T("assistant.shopping_checkout_prep.readiness.user_input"))),
		fmt.Sprintf("| 8 | %s | %s | %s |  |", cleanSpreadsheetCell(lang.T("assistant.shopping_checkout_prep.sheet.choice_gate")), cleanSpreadsheetCell(lang.T("assistant.shopping_checkout_prep.sheet.choice_gate_values")), cleanSpreadsheetCell(lang.T("assistant.shopping_checkout_prep.sheet.choice_gate_action"))),
	}
	if len(candidates) == 0 {
		candidates = assistantShoppingPackageFallbackCandidates(query, searchURL)
		fallback = true
	}
	for i, candidate := range assistantShoppingCandidateLines(candidates, 5) {
		status := lang.T("assistant.shopping_checkout_prep.sheet.candidate_status.product_page")
		if fallback {
			status = lang.T("assistant.shopping_checkout_prep.sheet.candidate_status.search_link")
		}
		lines = append(lines, fmt.Sprintf("| %s | %s | %s | %s | %s |",
			cleanSpreadsheetCell(lang.T("assistant.shopping_checkout_prep.sheet.candidate", i+1)),
			cleanSpreadsheetCell(firstNonEmpty(candidate.Text, lang.T("assistant.shopping_checkout_prep.sheet.candidate_fallback", i+1))),
			cleanSpreadsheetCell(status),
			cleanSpreadsheetCell(lang.T("assistant.shopping_checkout_prep.sheet.candidate_action")),
			cleanSpreadsheetCell(firstNonEmpty(candidate.URL, searchURL)),
		))
	}
	return strings.Join(lines, "\n")
}

func assistantCoupangCheckoutPrepApprovalDraft(selection int) string {
	return lang.T("assistant.shopping_checkout_prep.approval_draft", selection+1)
}

func assistantCoupangCheckoutPrepChoiceGateLines() []string {
	return []string{
		"- " + lang.T("assistant.shopping_checkout_prep.choice_gate.address"),
		"- " + lang.T("assistant.shopping_checkout_prep.choice_gate.payment"),
		"- " + lang.T("assistant.shopping_checkout_prep.choice_gate.no_raw"),
	}
}

func assistantCoupangCheckoutPrepLiveReadinessLines() []string {
	return []string{
		"- " + lang.T("assistant.shopping_checkout_prep.readiness.login"),
		"- " + lang.T("assistant.shopping_checkout_prep.readiness.assistant_can"),
		"- " + lang.T("assistant.shopping_checkout_prep.readiness.user_input"),
		"- " + lang.T("assistant.shopping_checkout_prep.readiness.final_gate"),
	}
}

func assistantCoupangCheckoutPrepSourceStatusLine(searchCandidateCount, fallbackCandidateCount int) string {
	return lang.T("assistant.shopping_checkout_prep.source_status", searchCandidateCount, fallbackCandidateCount)
}

func assistantCoupangCheckoutPrepVoiceSourceStatus(searchCandidateCount, fallbackCandidateCount int) string {
	return lang.T("assistant.shopping_checkout_prep.voice.source_status", searchCandidateCount, fallbackCandidateCount)
}

func assistantCoupangCheckoutPrepVoiceScript(query string, selected browserauto.Link, selection int, fallback bool, searchCandidateCount, fallbackCandidateCount int, actionReply string) string {
	lines := []string{
		lang.T("assistant.shopping_checkout_prep.voice.intro"),
		lang.T("assistant.shopping_checkout_prep.voice.query", query),
		assistantCoupangCheckoutPrepVoiceSourceStatus(searchCandidateCount, fallbackCandidateCount),
		assistantShoppingPackageVoiceBrowserStatus(actionReply),
		assistantShoppingPackageVoiceBoundaryStatus(),
		lang.T("assistant.shopping_checkout_prep.voice.selection", selectionSpeech(selection+1)),
	}
	if strings.TrimSpace(selected.Text) != "" {
		lines = append(lines, lang.T("assistant.shopping_checkout_prep.voice.selected", stripURLsForSpeech(selected.Text)))
	}
	if fallback {
		lines = append(lines, lang.T("assistant.shopping_checkout_prep.voice.fallback"))
	}
	lines = append(lines,
		lang.T("assistant.shopping_checkout_prep.voice.checks"),
		lang.T("assistant.shopping_checkout_prep.voice.live_ready"),
		lang.T("assistant.shopping_checkout_prep.voice.auth"),
		lang.T("assistant.shopping_checkout_prep.voice.final_values"),
		lang.T("assistant.shopping_checkout_prep.voice.choice_gate"),
		lang.T("assistant.shopping_checkout_prep.voice.multi_choice_next"),
		lang.T("assistant.shopping_checkout_prep.voice.boundary"),
	)
	return strings.Join(lines, "\n\n")
}

func assistantCoupangCheckoutPrepCandidateSummary(candidates []browserauto.Link, fallback bool) []string {
	lines := []string{}
	for i, candidate := range assistantShoppingCandidateLines(candidates, 5) {
		title := firstNonEmpty(strings.TrimSpace(candidate.Text), lang.T("assistant.shopping_checkout_prep.sheet.candidate_fallback", i+1))
		lines = append(lines, lang.T("assistant.shopping.coupang.candidate_line", i+1, title))
		if strings.TrimSpace(candidate.URL) != "" {
			lines = append(lines, lang.T("assistant.shopping.coupang.candidate_link", strings.TrimSpace(candidate.URL)))
		}
	}
	if len(lines) == 0 {
		lines = append(lines, "- "+lang.T("assistant.shopping.coupang.candidate_unavailable"))
	}
	if fallback {
		lines = append(lines, "- "+lang.T("assistant.shopping_checkout_prep.fallback_note"))
	}
	return lines
}

func assistantCoupangCheckoutPrepMarkdownCandidates(candidates []browserauto.Link, fallback bool) []string {
	lines := []string{}
	for i, candidate := range assistantShoppingCandidateLines(candidates, 5) {
		title := firstNonEmpty(strings.TrimSpace(candidate.Text), lang.T("assistant.shopping_checkout_prep.sheet.candidate_fallback", i+1))
		link := strings.TrimSpace(candidate.URL)
		line := fmt.Sprintf("%d. %s", i+1, title)
		if link != "" {
			line += " - " + link
		}
		lines = append(lines, line)
	}
	if len(lines) == 0 {
		lines = append(lines, "- "+lang.T("assistant.shopping.coupang.candidate_unavailable"))
	}
	if fallback {
		lines = append(lines, "- "+lang.T("assistant.shopping_checkout_prep.fallback_note"))
	}
	return lines
}

func selectionSpeech(n int) string {
	switch n {
	case 1:
		return lang.T("assistant.shopping_checkout_prep.selection.1")
	case 2:
		return lang.T("assistant.shopping_checkout_prep.selection.2")
	case 3:
		return lang.T("assistant.shopping_checkout_prep.selection.3")
	default:
		return lang.T("assistant.shopping_checkout_prep.selection.n", n)
	}
}

func assistantCheckoutPrepContainsHangul(value string) bool {
	for _, r := range value {
		if unicode.In(r, unicode.Hangul) {
			return true
		}
	}
	return false
}
