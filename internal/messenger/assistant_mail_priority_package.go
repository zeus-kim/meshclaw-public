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
	"github.com/meshclaw/meshclaw/internal/tts"
)

func formatAssistantMailPriorityPackageSendResult(opts ListenOptions, targetRef, request, reply string, items []signalMailPriorityItem, errors []signalMailAccountError, record evidence.Record, storeErr error) string {
	reply = strings.TrimSpace(signalReplyVisibleText(reply))
	if reply == "" {
		reply = lang.T("assistant.mail_priority_package.default_summary") + "\n" + lang.T("assistant.mail_priority_package.default_empty")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	title := lang.T("assistant.mail_priority_package.title")
	restorePreview := temporarilySkipPreviewImages()
	doc := osauto.CreateArgosDocument(ctx, lang.T("assistant.mail_priority_package.doc_title", title), assistantMailPriorityPackageReportBody(request, items, errors))
	deck := osauto.CreatePresentation(ctx, lang.T("assistant.mail_priority_package.deck_title", title), assistantMailPriorityPackageDeckBody(items, errors), lang.T("assistant.mail_priority_package.deck_audience"), 9, "")
	sheet := osauto.CreateSpreadsheet(ctx, lang.T("assistant.mail_priority_package.sheet_title", title), assistantMailPriorityPackageSheetBody(items, errors))
	boardPath, boardErr := writeAssistantMailPriorityBoardSVG(title, items, errors)
	restorePreview()

	wantsVoice := assistantBrowserResearchPackageWantsVoice(request)
	voiceScript := ""
	var audio tts.Result
	var audioErr error
	if wantsVoice {
		voiceScript = assistantMailPriorityPackageVoiceScript(items, errors)
		audio, audioErr = tts.Synthesize(tts.Options{
			Text:     voiceScript,
			Engine:   "edge-tts",
			Basename: "mail-priority-brief-" + time.Now().UTC().Format("20060102T150405Z"),
		})
	}

	attachments := assistantMailPriorityPackageAttachments(doc, deck, sheet)
	if strings.TrimSpace(boardPath) != "" && boardErr == nil {
		attachments = uniqueShowcaseAttachments(append(attachments, boardPath))
	}
	if strings.TrimSpace(audio.Path) != "" && audioErr == nil {
		attachments = uniqueShowcaseAttachments(append(attachments, audio.Path))
	}
	rememberAssistantArtifact(opts, title, "mail_priority_package", doc, deck, sheet)
	packageRecord, packageStoreErr := evidence.Store("assistant-mail-priority-package", firstNonEmpty(opts.TargetID, "assistant"), title, map[string]interface{}{
		"kind":         "assistant_mail_priority_package",
		"request":      request,
		"items":        items,
		"errors":       errors,
		"voice_script": voiceScript,
		"audio":        audio,
		"audio_error":  errorString(audioErr),
		"board":        boardPath,
		"board_error":  errorString(boardErr),
		"document":     doc,
		"presentation": deck,
		"spreadsheet":  sheet,
		"attachments":  attachments,
		"created_at":   time.Now().UTC(),
	})
	if packageRecord.StoredAt != "" || packageStoreErr != nil {
		record = packageRecord
		storeErr = packageStoreErr
	}

	summary := assistantMailPriorityPackageSummary(reply, items, errors, doc, deck, sheet, boardPath, boardErr, wantsVoice, audio, audioErr)
	target, candidates, targetErr := resolveAssistantVoiceTarget(targetRef)
	if targetErr != nil {
		lines := []string{lang.T("assistant.mail_priority_package.send_target_failed")}
		lines = append(lines, formatAssistantVoiceTargetCandidates(candidates)...)
		lines = append(lines, "", lang.T("assistant.mail_priority_package.attach_here"))
		lines = appendVoiceReportAttachmentMarkers(lines, attachments)
		return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
	}
	if !opts.Execute {
		targetLabel := assistantScheduleLocalizedTarget(target.ID, target.Label)
		lines := []string{
			lang.T("assistant.mail_priority_package.send_ready"),
			lang.T("assistant.mail_priority_package.target", targetLabel),
		}
		if OneWayReportTarget(target) {
			lines = append(lines, lang.T("assistant.mail_priority_package.one_way"))
		}
		lines = append(lines, "", lang.T("assistant.mail_priority_package.to_send"))
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
		"kind":             "assistant_mail_priority_package_send",
		"target":           targetRef,
		"resolved_target":  target,
		"attachment_count": len(attachments),
		"mobile_links":     mobileLinkLines,
		"items":            items,
		"errors":           errors,
		"send":             send,
		"send_error":       errorString(sendErr),
		"created_at":       time.Now().UTC(),
	}
	sendRecord, sendStoreErr := evidence.Store("assistant-mail-priority-package-send", firstNonEmpty(opts.TargetID, "assistant"), target.ID, payload)
	targetLabel := assistantScheduleLocalizedTarget(target.ID, target.Label)
	if sendErr != nil {
		lines := []string{
			lang.T("assistant.mail_priority_package.send_failed"),
			lang.T("assistant.mail_priority_package.target", targetLabel),
			lang.T("assistant.mail_priority_package.problem", sendErr.Error()),
			"",
			lang.T("assistant.mail_priority_package.attach_here"),
		}
		lines = appendAssistantWorkflowMobileLinkLines(lines, mobileLinkLines)
		lines = appendVoiceReportAttachmentMarkers(lines, attachments)
		return strings.Join(appendAssistantEvidenceNote(lines, sendRecord, sendStoreErr), "\n")
	}
	lines := []string{
		lang.T("assistant.mail_priority_package.sent"),
		lang.T("assistant.mail_priority_package.target", targetLabel),
	}
	if id := strings.TrimSpace(send.Stdout); id != "" {
		lines = append(lines, lang.T("assistant.workflow.signal_id", id))
	}
	return strings.Join(appendAssistantEvidenceNote(lines, sendRecord, sendStoreErr), "\n")
}

func assistantMailPriorityPackageSummary(reply string, items []signalMailPriorityItem, errors []signalMailAccountError, doc, deck, sheet osauto.Result, boardPath string, boardErr error, wantsVoice bool, audio tts.Result, audioErr error) []string {
	lines := []string{
		lang.T("assistant.mail_priority_package.created"),
		lang.T("assistant.mail_priority_package.subtitle"),
		"",
		lang.T("assistant.mail_priority_package.preview"),
	}
	lines = append(lines, assistantMailPriorityPackageVisibleSummary(reply)...)
	if plan := assistantMailPriorityReplyPlanLines(items, 3); len(plan) > 0 {
		lines = append(lines, "", lang.T("assistant.mail_priority_package.reply_plan"))
		lines = append(lines, plan...)
	}
	if len(mailPriorityActionableItems(items)) > 0 {
		lines = append(lines, "", lang.T("assistant.mail_priority_package.approval_handoff"))
		lines = append(lines, assistantMailPriorityApprovalHandoffLines()...)
	}
	lines = append(lines, "", lang.T("assistant.mail_priority_package.files"))
	if doc.OK && doc.Error == "" {
		lines = append(lines, "- "+lang.T("assistant.mail_priority_package.file.doc"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.mail_priority_package.file.doc_failed", firstNonEmpty(doc.Error, doc.Stderr, "unknown error")))
	}
	if deck.OK && deck.Error == "" {
		lines = append(lines, "- "+lang.T("assistant.mail_priority_package.file.ppt"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.mail_priority_package.file.ppt_failed", firstNonEmpty(deck.Error, deck.Stderr, "unknown error")))
	}
	if sheet.OK && sheet.Error == "" {
		lines = append(lines, "- "+lang.T("assistant.mail_priority_package.file.sheet"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.mail_priority_package.file.sheet_failed", firstNonEmpty(sheet.Error, sheet.Stderr, "unknown error")))
	}
	if boardErr == nil && strings.TrimSpace(boardPath) != "" {
		lines = append(lines, "- "+lang.T("assistant.mail_priority_package.file.board"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.mail_priority_package.file.board_failed", firstNonEmpty(errorString(boardErr), "unknown error")))
	}
	if wantsVoice {
		if audioErr == nil && strings.TrimSpace(audio.Path) != "" {
			lines = append(lines, "- "+lang.T("assistant.mail_priority_package.file.audio"))
		} else {
			lines = append(lines, "- "+lang.T("assistant.mail_priority_package.file.audio_failed", firstNonEmpty(errorString(audioErr), "unknown error")))
		}
	}
	if len(errors) > 0 {
		lines = append(lines, "", lang.T("assistant.mail_priority_package.errors", len(errors)))
	}
	lines = append(lines, "", lang.T("assistant.mail_priority.summary.boundary"))
	if len(mailPriorityActionableItems(items)) > 0 {
		lines = append(lines,
			"",
			lang.T("assistant.mail_priority_package.next"),
			"- "+lang.T("assistant.mail_priority_package.next.reply"),
			"- "+lang.T("assistant.mail_priority_package.next.read"),
			"- "+lang.T("assistant.mail_priority_package.next.calendar"),
			"- "+lang.T("assistant.mail_priority_package.next.reminder"),
			"- "+lang.T("assistant.mail_priority_package.next.search"),
		)
	}
	return compactBlankLines(lines)
}

func assistantMailPriorityPackageVisibleSummary(reply string) []string {
	lines := []string{}
	for _, line := range strings.Split(strings.TrimSpace(reply), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "바로 이어서") || strings.HasPrefix(line, "메일 발송/삭제") {
			continue
		}
		lines = append(lines, line)
		if len(lines) >= 14 {
			break
		}
	}
	if len(lines) == 0 {
		lines = append(lines, lang.T("assistant.mail_priority_package.default_summary"), lang.T("assistant.mail_priority_package.default_reply_empty"))
	}
	return lines
}

func assistantMailPriorityPackageReportBody(request string, items []signalMailPriorityItem, errors []signalMailAccountError) string {
	now := time.Now().In(time.FixedZone("KST", 9*60*60))
	actionable := mailPriorityActionableItems(items)
	replyCount := 0
	for _, item := range actionable {
		if assistantMailPriorityIsReplyFirst(item.Bucket) {
			replyCount++
		}
	}
	lines := []string{
		lang.T("assistant.mail_priority_package.report.title"),
		"",
		lang.T("assistant.mail_priority_package.report.generated", now.Format("2006-01-02 15:04 KST")),
		lang.T("assistant.mail_priority_package.report.request", strings.TrimSpace(request)),
		lang.T("assistant.mail_priority_package.report.checked", len(items)),
		lang.T("assistant.mail_priority_package.report.candidates", replyCount, len(actionable)-replyCount, len(items)-len(actionable)),
		"",
		lang.T("assistant.mail_priority_package.report.conclusion"),
	}
	if len(actionable) == 0 {
		lines = append(lines, "- "+lang.T("assistant.mail_priority_package.report.conclusion.empty"), "- "+lang.T("assistant.mail_priority_package.report.conclusion.deferred"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.mail_priority_package.report.conclusion.action"), "- "+lang.T("assistant.mail_priority_package.report.conclusion.boundary"))
	}
	lines = append(lines, "", lang.T("assistant.mail_priority_package.report.reply_first"))
	lines = append(lines, assistantMailPriorityPackageMarkdownItems(items, "답장 우선")...)
	lines = append(lines, "", lang.T("assistant.mail_priority_package.report.needs_check"))
	lines = append(lines, assistantMailPriorityPackageMarkdownItems(items, "확인 필요")...)
	if plan := assistantMailPriorityReplyPlanLines(items, 5); len(plan) > 0 {
		lines = append(lines, "", lang.T("assistant.mail_priority_package.report.reply_plan"))
		lines = append(lines, plan...)
	}
	if len(actionable) > 0 {
		lines = append(lines, "", lang.T("assistant.mail_priority_package.report.approval_handoff"))
		lines = append(lines, assistantMailPriorityApprovalHandoffLines()...)
	}
	lines = append(lines, "", lang.T("assistant.mail_priority_package.report.deferred"))
	deferred := assistantMailPriorityDeferredItems(items)
	if len(deferred) == 0 {
		lines = append(lines, "- "+lang.T("assistant.mail_priority_package.report.no_deferred"))
	} else {
		for _, item := range deferred {
			lines = append(lines, "- "+assistantMailPriorityPackageItemTitle(item)+" — "+item.Reason)
			if len(lines) > 80 {
				break
			}
		}
	}
	if len(errors) > 0 {
		lines = append(lines, "", lang.T("assistant.mail_priority_package.report.errors"))
		for _, item := range errors {
			lines = append(lines, fmt.Sprintf("- %s: %s", item.Account.ID, item.Error))
		}
	}
	lines = append(lines,
		"",
		lang.T("assistant.mail_priority_package.report.next"),
		"1. "+lang.T("assistant.mail_priority_package.report.next.read"),
		"2. "+lang.T("assistant.mail_priority_package.report.next.reply"),
		"3. "+lang.T("assistant.mail_priority_package.report.next.calendar"),
		"4. "+lang.T("assistant.mail_priority_package.report.next.reminder"),
		"5. "+lang.T("assistant.mail_priority_package.report.next.approve"),
	)
	return strings.Join(lines, "\n")
}

func assistantMailPriorityPackageDeckBody(items []signalMailPriorityItem, errors []signalMailAccountError) string {
	actionable := mailPriorityActionableItems(items)
	replyCount := 0
	for _, item := range actionable {
		if assistantMailPriorityIsReplyFirst(item.Bucket) {
			replyCount++
		}
	}
	return strings.Join([]string{
		lang.T("assistant.mail_priority_package.deck.conclusion"),
		lang.T("assistant.mail_priority_package.deck.counts", len(items), replyCount, len(actionable)-replyCount, len(items)-len(actionable)),
		lang.T("assistant.mail_priority_package.deck.reply_first"),
		strings.Join(assistantMailPriorityPackageSlideItems(items, "답장 우선"), "\n"),
		lang.T("assistant.mail_priority_package.deck.needs_check"),
		strings.Join(assistantMailPriorityPackageSlideItems(items, "확인 필요"), "\n"),
		lang.T("assistant.mail_priority_package.deck.reply_plan"),
		strings.Join(assistantMailPriorityReplyPlanLines(items, 4), "\n"),
		lang.T("assistant.mail_priority_package.deck.approval_handoff"),
		strings.Join(assistantMailPriorityApprovalHandoffLines(), "\n"),
		lang.T("assistant.mail_priority_package.deck.defer_criteria"),
		lang.T("assistant.mail_priority_package.deck.defer_body"),
		lang.T("assistant.mail_priority_package.deck.errors"),
		assistantMailPriorityPackageErrorsLine(errors),
		lang.T("assistant.mail_priority_package.deck.next"),
		lang.T("assistant.mail_priority_package.deck.next_body"),
		lang.T("assistant.mail_priority_package.deck.boundary"),
		lang.T("assistant.mail_priority_package.deck.boundary_body"),
	}, "\n\n")
}

func assistantMailPriorityPackageSheetBody(items []signalMailPriorityItem, errors []signalMailAccountError) string {
	rows := []string{
		lang.T("assistant.mail_priority_package.sheet.header"),
		"|---|---|---|---|---|---:|---|---|---|",
	}
	for _, item := range items {
		message := item.Message
		rows = append(rows, fmt.Sprintf("|%s|%s|%s|%s|%s|%d|%s|%s|%s|",
			cleanSpreadsheetCell(assistantMailPriorityLocalizedBucket(item.Bucket)),
			cleanSpreadsheetCell(firstNonEmpty(item.Account, "mail")),
			cleanSpreadsheetCell(formatSignalMailMessageTime(message.Date)),
			cleanSpreadsheetCell(cleanSignalMailHeaderForDisplay(message.From, lang.T("assistant.mail_priority_package.from_unknown"))),
			cleanSpreadsheetCell(cleanSignalMailHeaderForDisplay(message.Subject, lang.T("assistant.mail_priority_package.subject_unknown"))),
			item.Score,
			cleanSpreadsheetCell(item.Reason),
			cleanSpreadsheetCell(item.Action),
			cleanSpreadsheetCell(cleanSignalMailSnippet(message.Snippet, 120)),
		))
	}
	for _, item := range errors {
		rows = append(rows, lang.T("assistant.mail_priority_package.sheet.error_row", cleanSpreadsheetCell(item.Account.ID), cleanSpreadsheetCell(item.Error)))
	}
	return strings.Join(rows, "\n")
}

func assistantMailPriorityPackageVoiceScript(items []signalMailPriorityItem, errors []signalMailAccountError) string {
	actionable := mailPriorityActionableItems(items)
	replyCount := 0
	for _, item := range actionable {
		if assistantMailPriorityIsReplyFirst(item.Bucket) {
			replyCount++
		}
	}
	lines := []string{
		lang.T("assistant.mail_priority_package.voice.intro"),
		lang.T("assistant.mail_priority_package.voice.counts", len(items), replyCount, len(actionable)-replyCount),
	}
	if len(actionable) == 0 {
		lines = append(lines, lang.T("assistant.mail_priority_package.voice.empty"))
	} else {
		lines = append(lines, lang.T("assistant.mail_priority_package.voice.first"))
		for i, item := range actionable {
			if i >= 3 {
				break
			}
			lines = append(lines, lang.T("assistant.mail_priority_package.voice.item", i+1, assistantMailPriorityPackageItemTitle(item), item.Reason, item.Action))
		}
		lines = append(lines, lang.T("assistant.mail_priority_package.voice.reply_plan"))
		lines = append(lines, lang.T("assistant.mail_priority_package.voice.approval_handoff"))
	}
	if len(errors) > 0 {
		lines = append(lines, lang.T("assistant.mail_priority_package.voice.errors", len(errors)))
	}
	lines = append(lines, lang.T("assistant.mail_priority_package.voice.boundary"))
	return strings.Join(lines, "\n\n")
}

func assistantMailPriorityPackageAttachments(doc, deck, sheet osauto.Result) []string {
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

func writeAssistantMailPriorityBoardSVG(title string, items []signalMailPriorityItem, errors []signalMailAccountError) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Documents", "Argos Vault", "Charts")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, safeAssistantDepartmentFilename(title)+"-"+time.Now().Format("20060102-150405")+"-mail-board.svg")
	content := renderAssistantMailPriorityBoardSVG(title, items, errors)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return "", err
	}
	return path, nil
}

func renderAssistantMailPriorityBoardSVG(title string, items []signalMailPriorityItem, errors []signalMailAccountError) string {
	actionable := mailPriorityActionableItems(items)
	replyCount := 0
	checkCount := 0
	for _, item := range actionable {
		if assistantMailPriorityIsReplyFirst(item.Bucket) {
			replyCount++
		} else {
			checkCount++
		}
	}
	deferCount := len(items) - len(actionable)
	cards := []struct {
		Label string
		Value int
		Color string
	}{
		{lang.T("assistant.mail_priority_package.board.reply"), replyCount, "#dc2626"},
		{lang.T("assistant.mail_priority_package.board.check"), checkCount, "#d97706"},
		{lang.T("assistant.mail_priority_package.board.defer"), deferCount, "#64748b"},
		{lang.T("assistant.mail_priority_package.board.errors"), len(errors), "#7c3aed"},
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<svg xmlns="http://www.w3.org/2000/svg" width="840" height="620" viewBox="0 0 840 620">` + "\n")
	b.WriteString(`<rect width="840" height="620" fill="#f8fafc"/>` + "\n")
	b.WriteString(`<rect x="24" y="24" width="792" height="572" rx="20" fill="#ffffff" stroke="#d9e2ef"/>` + "\n")
	b.WriteString(fmt.Sprintf(`<text x="48" y="66" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="27" font-weight="850" fill="#0f172a">%s</text>`, html.EscapeString(shortenSVGText(title, 36))))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`<text x="48" y="96" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="15" fill="#64748b">%s</text>`, html.EscapeString(lang.T("assistant.mail_priority_package.board.subtitle"))))
	b.WriteString("\n")
	for i, card := range cards {
		x := 48 + i*188
		b.WriteString(fmt.Sprintf(`<rect x="%d" y="126" width="168" height="92" rx="16" fill="#f8fafc" stroke="#d9e2ef"/>`, x))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf(`<circle cx="%d" cy="158" r="8" fill="%s"/>`, x+22, html.EscapeString(card.Color)))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf(`<text x="%d" y="166" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="14" font-weight="800" fill="#334155">%s</text>`, x+40, html.EscapeString(card.Label)))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf(`<text x="%d" y="202" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="32" font-weight="900" fill="#0f172a">%d</text>`, x+22, card.Value))
		b.WriteString("\n")
	}
	b.WriteString(fmt.Sprintf(`<text x="48" y="258" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="17" font-weight="850" fill="#0f172a">%s</text>`, html.EscapeString(lang.T("assistant.mail_priority_package.board.top"))))
	b.WriteString("\n")
	top := assistantMailPriorityBoardItems(items)
	for i, item := range top {
		if i >= 4 {
			break
		}
		y := 282 + i*68
		color := "#d97706"
		if assistantMailPriorityIsReplyFirst(item.Bucket) {
			color = "#dc2626"
		}
		b.WriteString(fmt.Sprintf(`<rect x="48" y="%d" width="744" height="54" rx="14" fill="#ffffff" stroke="#d9e2ef"/>`, y))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf(`<rect x="48" y="%d" width="8" height="54" rx="4" fill="%s"/>`, y, color))
		b.WriteString("\n")
		subject := cleanSignalMailHeaderForDisplay(item.Message.Subject, lang.T("assistant.mail_priority_package.subject_unknown"))
		b.WriteString(fmt.Sprintf(`<text x="70" y="%d" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="15" font-weight="800" fill="#0f172a">%s</text>`, y+22, html.EscapeString(shortenSVGText(subject, 55))))
		b.WriteString("\n")
		meta := fmt.Sprintf("%s · %s · %s", assistantMailPriorityLocalizedBucket(item.Bucket), cleanSignalMailHeaderForDisplay(item.Message.From, lang.T("assistant.mail_priority_package.from_unknown")), item.Action)
		b.WriteString(fmt.Sprintf(`<text x="70" y="%d" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="13" fill="#64748b">%s</text>`, y+43, html.EscapeString(shortenSVGText(meta, 78))))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf(`<text x="756" y="%d" text-anchor="end" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="14" font-weight="850" fill="#334155">%d</text>`, y+33, item.Score))
		b.WriteString("\n")
	}
	if len(top) == 0 {
		b.WriteString(fmt.Sprintf(`<text x="70" y="320" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="16" fill="#475569">%s</text>`, html.EscapeString(lang.T("assistant.mail_priority_package.board.empty"))))
		b.WriteString("\n")
	}
	b.WriteString(fmt.Sprintf(`<text x="48" y="570" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="14" fill="#64748b">%s</text>`, html.EscapeString(lang.T("assistant.mail_priority_package.board.footer"))))
	b.WriteString("\n</svg>\n")
	return b.String()
}

func assistantMailPriorityBoardItems(items []signalMailPriorityItem) []signalMailPriorityItem {
	out := []signalMailPriorityItem{}
	for _, item := range mailPriorityActionableItems(items) {
		out = append(out, item)
		if len(out) >= 4 {
			return out
		}
	}
	for _, item := range items {
		seen := false
		for _, existing := range out {
			if existing.Account == item.Account && existing.Message.ID == item.Message.ID {
				seen = true
				break
			}
		}
		if seen {
			continue
		}
		out = append(out, item)
		if len(out) >= 4 {
			return out
		}
	}
	return out
}

func assistantMailPriorityPackageMarkdownItems(items []signalMailPriorityItem, bucket string) []string {
	lines := []string{}
	for _, item := range items {
		if item.Bucket != bucket {
			continue
		}
		lines = append(lines,
			"- "+assistantMailPriorityPackageItemTitle(item),
			"  - "+lang.T("assistant.mail_priority_package.markdown.from", cleanSignalMailHeaderForDisplay(item.Message.From, lang.T("assistant.mail_priority_package.from_unknown"))),
			"  - "+lang.T("assistant.mail_priority_package.markdown.reason", item.Reason),
			"  - "+lang.T("assistant.mail_priority_package.markdown.action", item.Action),
		)
		if snippet := cleanSignalMailSnippet(item.Message.Snippet, 180); snippet != "" {
			lines = append(lines, "  - "+lang.T("assistant.mail_priority_package.markdown.preview", snippet))
		}
	}
	if len(lines) == 0 {
		lines = append(lines, "- "+lang.T("assistant.mail_priority_package.markdown.empty"))
	}
	return lines
}

func assistantMailPriorityPackageSlideItems(items []signalMailPriorityItem, bucket string) []string {
	lines := []string{}
	for _, item := range items {
		if item.Bucket != bucket {
			continue
		}
		lines = append(lines, "- "+trimForContext(assistantMailPriorityPackageItemTitle(item)+" / "+item.Reason, 120))
		if len(lines) >= 5 {
			break
		}
	}
	if len(lines) == 0 {
		lines = append(lines, "- "+lang.T("assistant.mail_priority_package.markdown.empty"))
	}
	return lines
}

func assistantMailPriorityReplyPlanLines(items []signalMailPriorityItem, limit int) []string {
	actionable := mailPriorityActionableItems(items)
	if limit <= 0 || limit > len(actionable) {
		limit = len(actionable)
	}
	lines := make([]string, 0, limit)
	for i, item := range actionable {
		if i >= limit {
			break
		}
		title := assistantMailPriorityPackageItemTitle(item)
		if assistantMailPriorityIsReplyFirst(item.Bucket) {
			lines = append(lines, lang.T("assistant.mail_priority_package.reply_plan.reply", i+1, title))
		} else {
			lines = append(lines, lang.T("assistant.mail_priority_package.reply_plan.check", i+1, title))
		}
		if snippet := cleanSignalMailSnippet(item.Message.Snippet, 120); snippet != "" {
			lines = append(lines, lang.T("assistant.mail_priority_package.reply_plan.preview", snippet))
		}
	}
	return lines
}

func assistantMailPriorityApprovalHandoffLines() []string {
	return []string{
		"- " + lang.T("assistant.mail_priority_package.approval_handoff.reply_draft"),
		"- " + lang.T("assistant.mail_priority_package.approval_handoff.send"),
		"- " + lang.T("assistant.mail_priority_package.approval_handoff.boundary"),
	}
}

func assistantMailPriorityDeferredItems(items []signalMailPriorityItem) []signalMailPriorityItem {
	out := []signalMailPriorityItem{}
	for _, item := range items {
		if item.Bucket == "낮음" || item.Bucket == "확인" {
			out = append(out, item)
		}
	}
	return out
}

func assistantMailPriorityPackageItemTitle(item signalMailPriorityItem) string {
	date := ""
	if !item.Message.Date.IsZero() {
		date = formatSignalMailMessageTime(item.Message.Date) + " "
	}
	return strings.TrimSpace(date + cleanSignalMailHeaderForDisplay(item.Message.Subject, lang.T("assistant.mail_priority_package.subject_unknown")))
}

func assistantMailPriorityPackageErrorsLine(errors []signalMailAccountError) string {
	if len(errors) == 0 {
		return lang.T("assistant.mail_priority_package.errors.none")
	}
	lines := []string{}
	for _, item := range errors {
		lines = append(lines, fmt.Sprintf("- %s: %s", item.Account.ID, item.Error))
	}
	return strings.Join(lines, "\n")
}

func assistantMailPriorityIsReplyFirst(bucket string) bool {
	return bucket == "답장 우선"
}

func assistantMailPriorityLocalizedBucket(bucket string) string {
	switch bucket {
	case "답장 우선":
		return lang.T("assistant.mail_priority_package.bucket.reply")
	case "확인 필요", "확인":
		return lang.T("assistant.mail_priority_package.bucket.check")
	case "낮음":
		return lang.T("assistant.mail_priority_package.bucket.low")
	default:
		return firstNonEmpty(bucket, lang.T("assistant.mail_priority_package.bucket.unknown"))
	}
}
