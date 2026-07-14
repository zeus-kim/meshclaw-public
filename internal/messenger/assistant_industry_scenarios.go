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

type assistantIndustryScenario struct {
	Group       string
	Company     string
	Request     string
	Output      string
	Attachments string
	Command     string
}

func assistantIndustryScenarioReply(opts ListenOptions, request string) (string, bool) {
	if !isAssistantIndustryScenarioRequest(request) {
		return "", false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	title := lang.T("assistant.industry.title")
	docBody := assistantIndustryScenarioDocument()
	deckBody := assistantIndustryScenarioDeck()
	sheetBody := assistantIndustryScenarioSpreadsheet()

	doc := osauto.CreateArgosDocument(ctx, title, docBody)
	deck := osauto.CreatePresentation(ctx, lang.T("assistant.industry.deck_title"), deckBody, lang.T("assistant.industry.deck_audience"), 10, "")
	sheet := osauto.CreateSpreadsheet(ctx, lang.T("assistant.industry.sheet_title"), sheetBody)
	graphPath, graphErr := writeAssistantIndustryScenarioGraphMarkdown(title)
	chartPath, chartErr := writeAssistantIndustryScenarioChartSVG(title)
	rememberAssistantArtifact(opts, title, "industry_scenarios", doc, deck, sheet)

	attachments := assistantIndustryScenarioAttachments(doc, deck, sheet)
	if strings.TrimSpace(graphPath) != "" && graphErr == nil {
		attachments = uniqueShowcaseAttachments(append(attachments, graphPath))
	}
	if strings.TrimSpace(chartPath) != "" && chartErr == nil {
		attachments = uniqueShowcaseAttachments(append(attachments, chartPath))
	}
	wantsVoice := assistantBrowserResearchPackageWantsVoice(request)
	voiceScript := ""
	var audio tts.Result
	var audioErr error
	if wantsVoice {
		voiceScript = assistantIndustryScenarioVoiceScript(len(assistantIndustryScenarioCatalog()))
		audio, audioErr = tts.Synthesize(tts.Options{
			Text:     voiceScript,
			Engine:   "edge-tts",
			Basename: "industry-scenarios-brief-" + time.Now().UTC().Format("20060102T150405Z"),
		})
	}
	if strings.TrimSpace(audio.Path) != "" && audioErr == nil {
		attachments = uniqueShowcaseAttachments(append(attachments, audio.Path))
	}
	payload := map[string]interface{}{
		"kind":         "assistant_industry_scenarios",
		"request":      request,
		"voice_script": voiceScript,
		"audio":        audio,
		"audio_error":  errorString(audioErr),
		"graph":        graphPath,
		"graph_error":  errorString(graphErr),
		"chart":        chartPath,
		"chart_error":  errorString(chartErr),
		"document":     doc,
		"presentation": deck,
		"spreadsheet":  sheet,
		"attachments":  attachments,
		"scenario_cnt": len(assistantIndustryScenarioCatalog()),
		"created_at":   time.Now().UTC(),
	}
	record, storeErr := evidence.Store("assistant-industry-scenarios", firstNonEmpty(opts.TargetID, "assistant"), title, payload)

	lines := []string{
		lang.T("assistant.industry.created", title),
		lang.T("assistant.industry.subtitle"),
		lang.T("assistant.industry.count", len(assistantIndustryScenarioCatalog())),
		"",
		lang.T("assistant.industry.files"),
	}
	if doc.OK {
		lines = append(lines, "- "+lang.T("assistant.industry.file.doc"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.industry.file.doc_failed", firstNonEmpty(doc.Error, doc.Stderr, "unknown error")))
	}
	if deck.OK {
		lines = append(lines, "- "+lang.T("assistant.industry.file.ppt"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.industry.file.ppt_failed", firstNonEmpty(deck.Error, deck.Stderr, "unknown error")))
	}
	if sheet.OK {
		lines = append(lines, "- "+lang.T("assistant.industry.file.sheet"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.industry.file.sheet_failed", firstNonEmpty(sheet.Error, sheet.Stderr, "unknown error")))
	}
	if graphErr == nil && strings.TrimSpace(graphPath) != "" {
		lines = append(lines, "- "+lang.T("assistant.industry.file.graph"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.industry.file.graph_failed", firstNonEmpty(errorString(graphErr), "unknown error")))
	}
	if chartErr == nil && strings.TrimSpace(chartPath) != "" {
		lines = append(lines, "- "+lang.T("assistant.industry.file.chart"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.industry.file.chart_failed", firstNonEmpty(errorString(chartErr), "unknown error")))
	}
	if wantsVoice {
		if audioErr == nil && strings.TrimSpace(audio.Path) != "" {
			lines = append(lines, "- "+lang.T("assistant.industry.file.audio"))
		} else {
			lines = append(lines, "- "+lang.T("assistant.industry.file.audio_failed", firstNonEmpty(errorString(audioErr), "unknown error")))
		}
	}

	lines = append(lines,
		"",
		lang.T("assistant.industry.highlights"),
		"- "+lang.T("assistant.industry.highlight.marketing"),
		"- "+lang.T("assistant.industry.highlight.ads"),
		"- "+lang.T("assistant.industry.highlight.hr"),
		"- "+lang.T("assistant.industry.highlight.gov"),
		"- "+lang.T("assistant.industry.highlight.ops"),
		"",
		lang.T("assistant.industry.commands"),
		"- "+lang.T("assistant.industry.command.1"),
		"- "+lang.T("assistant.industry.command.2"),
		"- "+lang.T("assistant.industry.command.3"),
		"- "+lang.T("assistant.industry.command.4"),
		"- "+lang.T("assistant.industry.command.5"),
	)
	if strings.TrimSpace(voiceScript) != "" {
		lines = append(lines, "", lang.T("assistant.industry.voice_preview"), trimForContext(voiceScript, 520))
	}
	if targetRef := inferAssistantSignalTargetRef(request); targetRef != "" {
		return formatAssistantIndustryScenarioSendResult(opts, targetRef, lines, attachments, record, storeErr), true
	}
	lines = appendAssistantWorkflowMobileLinkLines(lines, assistantWorkflowVisibleMobileLinkLines(attachments, 6))
	lines = appendVoiceReportAttachmentMarkers(lines, attachments)
	lines = appendAssistantEvidenceNote(lines, record, storeErr)
	return strings.Join(compactBlankLines(lines), "\n"), true
}

func formatAssistantIndustryScenarioSendResult(opts ListenOptions, targetRef string, summary []string, attachments []string, record evidence.Record, storeErr error) string {
	target, candidates, targetErr := resolveAssistantVoiceTarget(targetRef)
	if targetErr != nil {
		lines := []string{lang.T("assistant.industry.send_target_failed")}
		lines = append(lines, formatAssistantVoiceTargetCandidates(candidates)...)
		lines = append(lines, "", lang.T("assistant.industry.attach_here"))
		lines = appendAssistantWorkflowMobileLinkLines(lines, assistantWorkflowVisibleMobileLinkLines(attachments, 6))
		lines = appendVoiceReportAttachmentMarkers(lines, attachments)
		return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
	}
	targetLabel := assistantScheduleLocalizedTarget(target.ID, target.Label)
	if !opts.Execute {
		lines := []string{
			lang.T("assistant.industry.send_ready"),
			lang.T("assistant.industry.target", targetLabel),
		}
		if OneWayReportTarget(target) {
			lines = append(lines, lang.T("assistant.industry.one_way"))
		}
		lines = append(lines, "", lang.T("assistant.industry.to_send"))
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
		"kind":             "assistant_industry_scenarios_send",
		"target":           targetRef,
		"resolved_target":  target,
		"attachment_count": len(attachments),
		"mobile_links":     mobileLinkLines,
		"send":             send,
		"send_error":       errorString(sendErr),
		"created_at":       time.Now().UTC(),
	}
	sendRecord, sendStoreErr := evidence.Store("assistant-industry-scenarios-send", firstNonEmpty(opts.TargetID, "assistant"), target.ID, payload)
	if sendErr != nil {
		lines := []string{
			lang.T("assistant.industry.send_failed"),
			lang.T("assistant.industry.target", targetLabel),
			lang.T("assistant.industry.problem", sendErr.Error()),
			"",
			lang.T("assistant.industry.attach_here"),
		}
		lines = appendAssistantWorkflowMobileLinkLines(lines, mobileLinkLines)
		lines = appendVoiceReportAttachmentMarkers(lines, attachments)
		return strings.Join(appendAssistantEvidenceNote(lines, sendRecord, sendStoreErr), "\n")
	}
	lines := []string{
		lang.T("assistant.industry.sent"),
		lang.T("assistant.industry.target", targetLabel),
	}
	if id := strings.TrimSpace(send.Stdout); id != "" {
		lines = append(lines, lang.T("assistant.workflow.signal_id", id))
	}
	return strings.Join(appendAssistantEvidenceNote(lines, sendRecord, sendStoreErr), "\n")
}

func writeAssistantIndustryScenarioGraphMarkdown(title string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Documents", "Argos Vault", "Charts")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, safeAssistantDepartmentFilename(title)+"-"+time.Now().Format("20060102-150405")+"-industry-graphs.md")
	graphTitle := title + " 그래프"
	graphIntro := "Signal 모바일에서 바로 열어 볼 수 있는 업종·부서별 업무 흐름 Mermaid와 회의용 그래프 원천 데이터입니다."
	if assistantIndustryScenarioEnglish() {
		graphTitle = title + " graphs"
		graphIntro = "Mobile-openable Mermaid flows and meeting chart source data for industry and department work."
	}
	content := strings.Join([]string{
		"# " + graphTitle,
		"",
		graphIntro,
		"",
		assistantIndustryScenarioGraphBody(),
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return "", err
	}
	return path, nil
}

func assistantIndustryScenarioGraphBody() string {
	if assistantIndustryScenarioEnglish() {
		return strings.Join([]string{
			"## Department Output Flow",
			"",
			"```mermaid",
			"flowchart LR",
			"  User[\"Signal user request\"] --> Marketing[\"Marketing sales analysis\\nproduct growth, ROAS, competitors\"]",
			"  User --> Ads[\"Advertising campaign\\nconcepts, copy, A/B tests\"]",
			"  User --> HR[\"HR recruiting\\nJD, screening, interview guide\"]",
			"  User --> Gov[\"Public-sector service\\ncomplaint triage, press note, citizen notice\"]",
			"  User --> Finance[\"Finance operations\\nbudget, expense, invoice approval\"]",
			"  User --> Product[\"Product team\\nPRD, roadmap, release notes\"]",
			"  Marketing --> SalesFiles[\"DOCX report\\nPPTX meeting deck\\nXLSX revenue chart\"]",
			"  Ads --> AdFiles[\"PPTX proposal\\nCSV KPI table\\nvoice brief\"]",
			"  HR --> HRFiles[\"Job-post DOCX\\ncandidate XLSX\\ninterview guide\"]",
			"  Gov --> GovFiles[\"Policy brief DOCX\\ncase table\\ncitizen notice\"]",
			"  Finance --> FinanceFiles[\"Budget table\\nexpense review\\napproval checklist\"]",
			"  Product --> ProductFiles[\"PRD\\nbacklog table\\nlaunch checklist\"]",
			"  SalesFiles --> Signal[\"Signal briefing-room attachments\"]",
			"  AdFiles --> Signal",
			"  HRFiles --> Signal",
			"  GovFiles --> Signal",
			"  FinanceFiles --> Signal",
			"  ProductFiles --> Signal",
			"```",
			"",
			"## Execution Priority",
			"",
			"```mermaid",
			"quadrantChart",
			"  title User value and automation difficulty",
			"  x-axis Lower automation difficulty --> Higher automation difficulty",
			"  y-axis Lower felt value --> Higher felt value",
			"  quadrant-1 Needs approval",
			"  quadrant-2 Ship now",
			"  quadrant-3 Later",
			"  quadrant-4 Prep automation",
			"  Meeting minutes: [0.32, 0.78]",
			"  Market research report: [0.58, 0.90]",
			"  Marketing revenue analysis: [0.45, 0.84]",
			"  Advertising campaign plan: [0.40, 0.76]",
			"  HR recruiting package: [0.48, 0.82]",
			"  Public-service citizen notice: [0.38, 0.74]",
			"  Finance approval pack: [0.52, 0.80]",
			"  Product PRD pack: [0.46, 0.83]",
			"  Coupang purchase execution: [0.92, 0.88]",
			"  Hotel/flight booking execution: [0.94, 0.86]",
			"```",
			"",
			"## Numbers For A Meeting",
			"",
			"| Work | Key metric | Example value | Immediate output |",
			"|---|---|---:|---|",
			"| Marketing | June revenue | 188M | Product growth report, revenue chart |",
			"| Advertising | Retail-media ROAS | 4.6 | Five concepts, A/B test table |",
			"| HR | Applicants | 312 | JD, screening table, interview guide |",
			"| Public sector | Civil complaints | 1,039 | Case triage, citizen notice, executive brief |",
			"| Finance | Budget execution | 73% | Delay-risk table, approval checklist |",
			"| Product | Release backlog | 42 items | PRD, roadmap, launch checklist |",
		}, "\n")
	}
	return strings.Join([]string{
		"## 부서별 산출물 흐름",
		"",
		"```mermaid",
		"flowchart LR",
		"  User[\"Signal 사용자 요청\"] --> Marketing[\"마케팅팀 매출 분석\\n제품군 성장·ROAS·경쟁사\"]",
		"  User --> Ads[\"광고기획 캠페인\\n콘셉트·카피·A/B 테스트\"]",
		"  User --> HR[\"인사팀 채용\\nJD·스크리닝·면접 질문\"]",
		"  User --> Gov[\"관공서 민원/정책\\n민원 분류·보도자료·시민 안내문\"]",
		"  User --> Finance[\"재무/구매\\n예산·정산·승인 체크\"]",
		"  User --> Product[\"제품팀\\nPRD·로드맵·릴리즈노트\"]",
		"  Marketing --> SalesFiles[\"DOCX 보고서\\nPPTX 회의자료\\nXLSX 매출 그래프\"]",
		"  Ads --> AdFiles[\"PPTX 제안서\\nCSV KPI 표\\n음성 브리핑\"]",
		"  HR --> HRFiles[\"채용공고 DOCX\\n후보자 비교 XLSX\\n면접 질문지\"]",
		"  Gov --> GovFiles[\"정책 브리프 DOCX\\n민원 처리표\\n시민 안내문\"]",
		"  Finance --> FinanceFiles[\"예산표\\n정산 리뷰\\n승인 체크리스트\"]",
		"  Product --> ProductFiles[\"PRD\\n백로그 표\\n출시 체크리스트\"]",
		"  SalesFiles --> Signal[\"Signal 보고방 첨부\"]",
		"  AdFiles --> Signal",
		"  HRFiles --> Signal",
		"  GovFiles --> Signal",
		"  FinanceFiles --> Signal",
		"  ProductFiles --> Signal",
		"```",
		"",
		"## 업무별 실행 우선순위",
		"",
		"```mermaid",
		"quadrantChart",
		"  title 사용자 체감 가치와 자동화 난이도",
		"  x-axis 낮은 자동화 난이도 --> 높은 자동화 난이도",
		"  y-axis 낮은 체감 가치 --> 높은 체감 가치",
		"  quadrant-1 승인 필요",
		"  quadrant-2 즉시 제품화",
		"  quadrant-3 후순위",
		"  quadrant-4 준비 자동화",
		"  회의록 작성: [0.32, 0.78]",
		"  시장조사 보고서: [0.58, 0.90]",
		"  마케팅 매출 분석: [0.45, 0.84]",
		"  광고 캠페인 기획: [0.40, 0.76]",
		"  인사 채용 패키지: [0.48, 0.82]",
		"  관공서 민원 안내문: [0.38, 0.74]",
		"  재무 승인 패키지: [0.52, 0.80]",
		"  제품 PRD 패키지: [0.46, 0.83]",
		"  쿠팡 구매 실행: [0.92, 0.88]",
		"  호텔/항공 예약 실행: [0.94, 0.86]",
		"```",
		"",
		"## 회의에서 바로 볼 숫자",
		"",
		"| 업무 | 핵심 지표 | 예시 값 | 바로 줄 수 있는 산출물 |",
		"|---|---|---:|---|",
		"| 마케팅 | 6월 매출 | 188M | 제품군 성장 원인 보고서, 매출 그래프 |",
		"| 광고기획 | 리테일미디어 ROAS | 4.6 | 캠페인 콘셉트 5개, A/B 테스트표 |",
		"| 인사팀 | 지원자 | 312명 | JD, 후보자 스크리닝 표, 면접 질문지 |",
		"| 관공서 | 민원 접수 | 1,039건 | 민원 분류표, 시민 안내문, 간부 보고 |",
		"| 재무 | 예산 집행률 | 73% | 지연 위험표, 승인 체크리스트 |",
		"| 제품팀 | 릴리즈 백로그 | 42개 | PRD, 로드맵, 출시 체크리스트 |",
	}, "\n")
}

func writeAssistantIndustryScenarioChartSVG(title string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Documents", "Argos Vault", "Charts")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, safeAssistantDepartmentFilename(title)+"-"+time.Now().Format("20060102-150405")+"-industry-chart.svg")
	content := renderAssistantIndustryScenarioChartSVG(title)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return "", err
	}
	return path, nil
}

func renderAssistantIndustryScenarioChartSVG(title string) string {
	points := []assistantDepartmentChartPoint{
		{Label: "마케팅", Value: 92, Color: "#2563eb"},
		{Label: "광고", Value: 88, Color: "#7c3aed"},
		{Label: "인사", Value: 84, Color: "#0f766e"},
		{Label: "공공", Value: 90, Color: "#d97706"},
	}
	scoreSuffix := "점"
	checks := []string{
		"마케팅: 매출 분석, 제품군 성장 원인, ROAS 예산 재배분",
		"광고: 콘셉트 5개, 카피, A/B 테스트와 KPI 표",
		"인사: 채용공고, 후보자 비교, 면접 질문, 온보딩",
		"공공: 민원 분류, 시민 안내문, 정책 브리프, 보도자료",
	}
	if assistantIndustryScenarioEnglish() {
		points = []assistantDepartmentChartPoint{
			{Label: "Marketing", Value: 92, Color: "#2563eb"},
			{Label: "Ads", Value: 88, Color: "#7c3aed"},
			{Label: "HR", Value: 84, Color: "#0f766e"},
			{Label: "Public", Value: 90, Color: "#d97706"},
		}
		scoreSuffix = " pts"
		checks = []string{
			"Marketing: revenue analysis, product growth, ROAS budget shifts",
			"Ads: five concepts, copy, A/B test plan, KPI table",
			"HR: job post, candidate comparison, interview guide, onboarding",
			"Public: case triage, citizen notice, policy brief, press release",
		}
	}
	max := 100.0
	width := 780.0
	height := 540.0
	left := 64.0
	top := 146.0
	chartW := 600.0
	chartH := 220.0
	gap := 30.0
	barW := (chartW - gap*float64(len(points)-1)) / float64(len(points))
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f">`, width, height, width, height))
	b.WriteString("\n")
	b.WriteString(`<rect width="780" height="540" fill="#f8fafc"/>` + "\n")
	b.WriteString(`<rect x="24" y="24" width="732" height="492" rx="20" fill="#ffffff" stroke="#d9e2ef"/>` + "\n")
	b.WriteString(fmt.Sprintf(`<text x="48" y="66" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="26" font-weight="850" fill="#0f172a">%s</text>`, html.EscapeString(shortenSVGText(title, 34))))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`<text x="48" y="96" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="15" fill="#64748b">%s</text>`, html.EscapeString(lang.T("assistant.industry.chart.subtitle"))))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`<line x1="%.0f" y1="%.0f" x2="%.0f" y2="%.0f" stroke="#94a3b8" stroke-width="2"/>`, left, top+chartH, left+chartW, top+chartH))
	b.WriteString("\n")
	for i, point := range points {
		x := left + float64(i)*(barW+gap)
		barH := chartH * (point.Value / max)
		y := top + chartH - barH
		b.WriteString(fmt.Sprintf(`<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="10" fill="%s"/>`, x, y, barW, barH, html.EscapeString(point.Color)))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="middle" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="17" font-weight="800" fill="#0f172a">%.0f%s</text>`, x+barW/2, y-12, point.Value, html.EscapeString(scoreSuffix)))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="middle" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="16" font-weight="700" fill="#334155">%s</text>`, x+barW/2, top+chartH+30, html.EscapeString(point.Label)))
		b.WriteString("\n")
	}
	for i, check := range checks {
		y := 424 + i*22
		b.WriteString(fmt.Sprintf(`<text x="48" y="%d" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="14" fill="#475569">• %s</text>`, y, html.EscapeString(shortenSVGText(check, 66))))
		b.WriteString("\n")
	}
	b.WriteString("</svg>\n")
	return b.String()
}

func assistantIndustryScenarioEnglish() bool {
	return lang.Current() == "en"
}

func assistantIndustryScenarioVoiceScript(scenarioCount int) string {
	lines := []string{
		lang.T("assistant.industry.voice_intro"),
		lang.T("assistant.industry.voice_scope", scenarioCount),
		lang.T("assistant.industry.voice_outputs"),
		lang.T("assistant.industry.highlight.marketing"),
		lang.T("assistant.industry.highlight.ads"),
		lang.T("assistant.industry.highlight.hr"),
		lang.T("assistant.industry.highlight.gov"),
		lang.T("assistant.industry.highlight.ops"),
		lang.T("assistant.industry.voice_next"),
	}
	for i, line := range lines {
		lines[i] = strings.NewReplacer("`", "", "DOCX", "닥스", "PPTX", "피피티엑스", "XLSX", "엑셀").Replace(stripURLsForSpeech(line))
	}
	return strings.Join(lines, "\n\n")
}

func isAssistantIndustryScenarioRequest(request string) bool {
	lower := strings.ToLower(strings.TrimSpace(request))
	if lower == "" {
		return false
	}
	scenarioSignal := containsAny(lower,
		"업종별", "산업별", "부서별", "직무별", "가상회사", "가상 회사", "회사들의",
		"시나리오", "시나이리오", "scenario", "use case", "사용 예제", "업무 예제",
		"scenarios", "industry", "industries", "department", "departments", "virtual company", "business use cases",
	)
	if !scenarioSignal {
		return false
	}
	return containsAny(lower,
		"회사", "마케팅", "매출", "그래프", "광고", "광고기획", "인사", "구인", "구직",
		"채용", "관공서", "정부", "공공기관", "부처", "산업", "업종", "영업", "대시보드",
		"company", "companies", "marketing", "revenue", "graph", "advertising", "ads", "hr",
		"recruiting", "government", "public agency", "public sector", "sales", "finance", "legal",
		"product", "operations", "dashboard",
	)
}

func assistantIndustryScenarioAttachments(doc, deck, sheet osauto.Result) []string {
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

func assistantIndustryScenarioDocument() string {
	if assistantIndustryScenarioEnglish() {
		return assistantIndustryScenarioDocumentEnglish()
	}
	var b strings.Builder
	b.WriteString("이 문서는 Argos가 OpenClaw/Hermes형 개인·업무 비서처럼 사용자에게 직접 보여줄 결과물을 기준으로 만든 업종별 가상회사 시나리오입니다.\n\n")
	b.WriteString("핵심 원칙은 간단합니다. 사용자는 내부 런타임 상태가 아니라 완성된 보고서, 표, 발표자료, 음성 브리핑, 메일 초안, 예약/구매 후보, 채용 파이프라인 같은 업무 결과를 Signal에서 받아야 합니다.\n\n")
	b.WriteString("## 바로 체감되는 산출물\n\n")
	b.WriteString("- 보고서: DOCX와 Markdown으로 만들어 iPhone Signal에서 바로 열고, Obsidian/Pages/Word에서 이어 편집합니다.\n")
	b.WriteString("- 발표자료: PPTX와 outline Markdown을 함께 만들어 PowerPoint, Keynote, Files 앱에서 바로 엽니다.\n")
	b.WriteString("- 분석표: XLSX와 CSV로 월별 매출, 리드, 전환율, ROAS, 채용 파이프라인, 민원 처리량을 정리합니다.\n")
	b.WriteString("- 그래프 데이터: Excel/Numbers에서 바로 차트로 바꿀 수 있도록 막대그래프, 라인차트, 퍼널차트, 히트맵 원천 데이터를 둡니다.\n")
	b.WriteString("- 공공 업무: 정부 부처와 관공서의 민원 분류, 처리 현황, 정책 브리프, 보도자료, 시민 안내문을 문서와 표로 만듭니다.\n")
	b.WriteString("- 실행 경계: 실제 결제, 예약 확정, 발송, 서버 변경은 마지막 승인 전 단계까지 준비하고 멈춥니다.\n\n")
	b.WriteString("## 업종·부서별 상세 시나리오\n\n")
	catalog := assistantIndustryScenarioCatalog()
	b.WriteString(lang.T("assistant.industry.scenario_count", len(catalog)) + "\n\n")
	for i, s := range catalog {
		b.WriteString(fmt.Sprintf("### %02d. %s - %s\n\n", i+1, s.Group, s.Company))
		b.WriteString("**사용자 요청 예문**\n\n")
		b.WriteString(s.Request + "\n\n")
		b.WriteString("**Argos가 만들어서 Signal로 보내야 하는 결과**\n\n")
		b.WriteString(s.Output + "\n\n")
		b.WriteString("**모바일 첨부**\n\n")
		b.WriteString(s.Attachments + "\n\n")
		b.WriteString("**바로 실행 명령**\n\n")
		b.WriteString("`" + s.Command + "`\n\n")
	}
	b.WriteString("## 그래프 처리 방식\n\n")
	b.WriteString("| 그래프 | 원천 데이터 | 사용자에게 보이는 결과 |\n")
	b.WriteString("|---|---|---|\n")
	b.WriteString("| 월별 매출 막대그래프 | 월, 제품군, 매출, 전월 대비 | 어느 제품군이 성장을 만들었는지 한눈에 표시 |\n")
	b.WriteString("| 마케팅 퍼널 | 노출, 클릭, 리드, 상담, 계약 | 어느 단계에서 고객이 빠지는지 표시 |\n")
	b.WriteString("| ROAS 라인차트 | 채널, 광고비, 매출, ROAS | 예산을 늘릴 채널과 줄일 채널 제안 |\n")
	b.WriteString("| 채용 파이프라인 | 지원, 서류, 면접, 오퍼, 입사 | 채용 병목과 다음 액션 표시 |\n")
	b.WriteString("| 민원 처리 히트맵 | 부서, 유형, 처리시간, 미해결 | 시민 불편과 부서별 병목 표시 |\n\n")
	b.WriteString("## 사용자가 Signal에서 이렇게 말하면 됩니다\n\n")
	b.WriteString("- `우리 화학회사 마케팅팀용으로 이번 주 민감 뉴스와 경쟁사 이슈를 보고서와 PPT로 만들어줘`\n")
	b.WriteString("- `지난 6개월 매출 CSV를 보고 제품군별 성장 그래프와 원인 분석을 만들어줘`\n")
	b.WriteString("- `광고기획팀 회의용으로 캠페인 콘셉트 5개와 A/B 테스트표를 만들어줘`\n")
	b.WriteString("- `인사팀 채용공고, 후보자 스크리닝 표, 면접 질문지를 한 번에 만들어줘`\n")
	b.WriteString("- `구청 민원 데이터를 요약해서 부서별 처리 현황과 시민 안내문 초안을 만들어줘`\n")
	return b.String()
}

func assistantIndustryScenarioDocumentEnglish() string {
	var b strings.Builder
	b.WriteString("This document shows virtual company scenarios where Argos behaves like an OpenClaw/Hermes-style personal and business assistant that sends concrete work products to the user.\n\n")
	b.WriteString("The principle is simple: the user should receive finished reports, tables, decks, voice briefs, mail drafts, booking or purchase candidates, recruiting pipelines, public notices, finance approval packs, and product documents in Signal instead of internal runtime status.\n\n")
	b.WriteString("## Work Products Users Can Feel\n\n")
	b.WriteString("- Reports: DOCX and Markdown that open from iPhone Signal and continue in Obsidian, Pages, or Word.\n")
	b.WriteString("- Slides: PPTX plus outline Markdown that open in PowerPoint, Keynote, or the Files app.\n")
	b.WriteString("- Analysis tables: XLSX and CSV for revenue, leads, conversion, ROAS, recruiting pipeline, complaints, budget, and product backlog.\n")
	b.WriteString("- Graph data: bar charts, line charts, funnels, heatmaps, and Mermaid flows that can become Excel or Numbers charts.\n")
	b.WriteString("- Public-sector work: case triage, status summaries, policy briefs, press releases, citizen notices, and inspection templates.\n")
	b.WriteString("- Execution boundary: payment, booking confirmation, sending, server changes, and account changes stop before final approval.\n\n")
	b.WriteString("## Detailed Industry And Department Scenarios\n\n")
	catalog := assistantIndustryScenarioCatalog()
	b.WriteString(lang.T("assistant.industry.scenario_count", len(catalog)) + "\n\n")
	for i, s := range catalog {
		b.WriteString(fmt.Sprintf("### %02d. %s - %s\n\n", i+1, s.Group, s.Company))
		b.WriteString("**User request example**\n\n")
		b.WriteString(s.Request + "\n\n")
		b.WriteString("**Result Argos should send to Signal**\n\n")
		b.WriteString(s.Output + "\n\n")
		b.WriteString("**Mobile attachments**\n\n")
		b.WriteString(s.Attachments + "\n\n")
		b.WriteString("**Runnable command**\n\n")
		b.WriteString("`" + s.Command + "`\n\n")
	}
	b.WriteString("## Graph Handling Pattern\n\n")
	b.WriteString("| Graph | Source data | User-visible result |\n")
	b.WriteString("|---|---|---|\n")
	b.WriteString("| Monthly revenue bars | Month, product line, revenue, month-over-month change | Which product line created growth |\n")
	b.WriteString("| Marketing funnel | Impressions, clicks, leads, consultations, contracts | Where customers drop out |\n")
	b.WriteString("| ROAS line chart | Channel, ad spend, revenue, ROAS | Which channels to grow or cut |\n")
	b.WriteString("| Recruiting pipeline | Applicants, screen pass, interviews, offers, hires | Hiring bottlenecks and next actions |\n")
	b.WriteString("| Public-service heatmap | Department, case type, processing time, unresolved count | Citizen pain points and department bottlenecks |\n")
	b.WriteString("| Product backlog chart | Feature, impact, effort, owner, release risk | What should ship now and what needs approval |\n\n")
	b.WriteString("## Commands Users Can Send In Signal\n\n")
	b.WriteString("- `Create a report and PPT for this week's sensitive news and competitor issues for our chemical-company marketing team`\n")
	b.WriteString("- `Analyze this six-month sales CSV and make product-line growth charts with root causes`\n")
	b.WriteString("- `Create five campaign concepts and an A/B test table for the advertising planning meeting`\n")
	b.WriteString("- `Create the job post, candidate screening table, and interview guide for HR`\n")
	b.WriteString("- `Summarize district-office civil complaints and draft department status plus citizen notices`\n")
	return b.String()
}

func assistantIndustryScenarioDeck() string {
	if assistantIndustryScenarioEnglish() {
		return strings.Join([]string{
			"# Purpose",
			"Show that Argos is not just chat: it produces work products for companies and public agencies and sends them to Signal.",
			"# First-Screen Message",
			"Argos creates reports, decks, analysis tables, voice briefs, mail drafts, recruiting packages, advertising plans, public-service materials, finance packs, and product documents.",
			"# Marketing and Sales",
			"Market research, competitor monitoring, monthly revenue analysis, ROAS analysis, customer segments, and sales pipeline artifacts become XLSX and PPTX outputs.",
			"# Advertising",
			"Campaign concepts, target personas, messaging, channel mix, copy, A/B hypotheses, and KPI dashboards become a proposal pack.",
			"# HR",
			"Job posts, candidate screening tables, interview guides, score rubrics, offer-mail drafts, and onboarding checklists become a recruiting pack.",
			"# Public Sector",
			"Civil-service triage, press releases, policy briefs, budget tables, meeting minutes, citizen notices, and field-inspection templates become usable documents.",
			"# Corporate Departments",
			"Finance, procurement, legal, customer support, product, IT/security, and executive staff work follow the same document, deck, table, graph, and voice pattern.",
			"# Signal Experience",
			"The user opens the files, plays the audio, and sends the next execution command instead of reading a status-only report.",
			"# Execution Boundary",
			"Purchase, booking confirmation, email sending, payment approval, and server changes stop before the final explicit approval.",
			"# Next Improvement",
			"Split these scenarios into skills and feed them real browser, mail, calendar, file, and spreadsheet inputs.",
		}, "\n\n")
	}
	return strings.Join([]string{
		"# 목적",
		"업종별 회사와 공공기관에서 Argos가 단순 채팅이 아니라 업무 결과물을 만들어 Signal로 보내는 비서라는 점을 보여준다.",
		"# 첫 화면 메시지",
		"Argos는 보고서, 발표자료, 분석표, 음성 브리핑, 메일 초안, 채용·광고·영업·공공 업무 결과를 만든다.",
		"# 마케팅/영업",
		"시장조사, 경쟁사 모니터링, 월별 매출 분석, ROAS 분석, 고객 세그먼트, 영업 파이프라인을 XLSX와 PPTX로 만든다.",
		"# 광고기획",
		"캠페인 콘셉트, 타깃, 메시지, 채널 믹스, 카피 10종, A/B 테스트 가설, KPI 대시보드를 만든다.",
		"# 인사팀",
		"채용공고, 후보자 스크리닝 표, 면접 질문, 평가 루브릭, 오퍼 메일, 온보딩 체크리스트를 만든다.",
		"# 공공기관",
		"민원 분류, 보도자료, 정책 브리프, 예산 집행표, 회의록, 시민 안내문, 현장 점검표를 만든다.",
		"# 산업별 확장",
		"SaaS, 이커머스, 금융, 제약, 제조, 모빌리티, 식음료, 미디어, 화학, 병원, 물류, 교육까지 같은 패턴으로 확장한다.",
		"# Signal 체감",
		"사용자는 완료 보고가 아니라 파일을 탭해서 열고, 음성을 재생하고, 다음 실행 명령을 보내는 방식으로 경험한다.",
		"# 실행 경계",
		"구매, 예약 확정, 메일 발송, 서버 변경은 최종 승인 전 단계까지 준비하고 멈춘다.",
		"# 다음 개선",
		"각 시나리오를 스킬로 나누고, 실제 브라우저·메일·캘린더·파일·스프레드시트 입력을 받아 완성도를 높인다.",
	}, "\n\n")
}

func assistantIndustryScenarioSpreadsheet() string {
	if assistantIndustryScenarioEnglish() {
		return strings.Join([]string{
			"|Area|Month/Stage|Metric|Value|Chart|Argos readout|Next action|",
			"|---|---:|---|---:|---|---|---|",
			"|Marketing|Jan|Revenue|120000000|Monthly revenue bar chart|Baseline before launch|Prepare search ads and content tests|",
			"|Marketing|Feb|Revenue|138000000|Monthly revenue bar chart|Promotion drove 15% growth|Improve high-conversion SKU pages|",
			"|Marketing|Mar|Revenue|155000000|Monthly revenue bar chart|Lead growth converted to revenue|Set sales follow-up SLA|",
			"|Marketing|Apr|Revenue|149000000|Monthly revenue bar chart|Ad spend rose but revenue stalled|Cut inefficient channel budget|",
			"|Marketing|May|Revenue|171000000|Monthly revenue bar chart|B2B large account inflow|Expand ABM campaign|",
			"|Marketing|Jun|Revenue|188000000|Monthly revenue bar chart|Product line A drove growth|Create product-line A casebook|",
			"|Advertising|Search ads|ROAS|3.8|ROAS line chart|Stable efficient channel|Increase budget by 15%|",
			"|Advertising|SNS|ROAS|1.4|ROAS line chart|Awareness high, purchase weak|Change retargeting message|",
			"|Advertising|YouTube|ROAS|2.1|ROAS line chart|Upper-funnel contribution|Connect with brand-keyword search|",
			"|Advertising|Retail media|ROAS|4.6|ROAS line chart|High purchase-intent channel|Focus on core SKUs|",
			"|Sales|Leads|Funnel|5400|Funnel chart|Top-of-funnel is enough|Score lead quality|",
			"|Sales|Consultations|Funnel|820|Funnel chart|Consultation conversion is 15.2%|Improve landing-page CTA|",
			"|Sales|Proposals|Funnel|210|Funnel chart|Proposal-stage bottleneck|Auto-generate pricing and cases|",
			"|Sales|Contracts|Funnel|64|Funnel chart|Close rate is 30.5%|Share top sales scripts|",
			"|HR|Applicants|Recruiting pipeline|312|Funnel chart|Applicant volume is enough|Add must-have skill questions|",
			"|HR|Screen pass|Recruiting pipeline|78|Funnel chart|Screen pass is 25%|Redefine JD requirements|",
			"|HR|First interview|Recruiting pipeline|41|Funnel chart|Interview scheduling is bottleneck|Suggest interview slots|",
			"|HR|Offers|Recruiting pipeline|9|Funnel chart|Offer acceptance needs review|Create compensation benchmark table|",
			"|Public sector|Traffic complaints|Case volume|428|Complaint heatmap|Commuting-hour cluster|Publish FAQ and notice|",
			"|Public sector|Welfare questions|Case volume|216|Complaint heatmap|Repeated document guidance|Create application checklist|",
			"|Public sector|Environment reports|Case volume|132|Complaint heatmap|Field checks delayed|Create inspection priority table|",
			"|Public sector|Budget execution|Execution rate|73|Budget bar chart|Quarter-end execution risk|Re-plan monthly execution|",
			"|Finance|Expense review|Open items|47|Approval queue|Missing receipts concentrated|Request missing receipts|",
			"|Product|Release backlog|Items|42|Roadmap chart|Decision bottleneck before QA|Split release checklist|",
		}, "\n")
	}
	return strings.Join([]string{
		"|분야|월/단계|지표|값|그래프|Argos 해석|다음 액션|",
		"|---|---:|---|---:|---|---|---|",
		"|마케팅|1월|매출|120000000|월별 매출 막대그래프|신제품 런칭 전 기준선|검색광고와 콘텐츠 테스트 준비|",
		"|마케팅|2월|매출|138000000|월별 매출 막대그래프|프로모션 후 15% 성장|전환율 높은 SKU 상세페이지 강화|",
		"|마케팅|3월|매출|155000000|월별 매출 막대그래프|리드 증가가 매출로 연결|영업팀 후속 콜 SLA 지정|",
		"|마케팅|4월|매출|149000000|월별 매출 막대그래프|광고비 증가 대비 정체|저효율 채널 예산 축소|",
		"|마케팅|5월|매출|171000000|월별 매출 막대그래프|B2B 대형 고객 유입|ABM 캠페인 확대|",
		"|마케팅|6월|매출|188000000|월별 매출 막대그래프|제품군 A 성장 주도|제품군 A 사례집 제작|",
		"|광고|검색광고|ROAS|3.8|ROAS 라인차트|효율 안정적|예산 15% 증액|",
		"|광고|SNS|ROAS|1.4|ROAS 라인차트|인지도는 높으나 구매 약함|리타겟팅 메시지 변경|",
		"|광고|유튜브|ROAS|2.1|ROAS 라인차트|상단 퍼널 기여|브랜드 키워드 검색 연동|",
		"|광고|리테일미디어|ROAS|4.6|ROAS 라인차트|구매 의도 높은 채널|핵심 SKU 집중 집행|",
		"|영업|리드|퍼널|5400|퍼널차트|상단 유입 충분|리드 품질 점수화|",
		"|영업|상담|퍼널|820|퍼널차트|상담 전환 15.2%|랜딩페이지 CTA 개선|",
		"|영업|제안|퍼널|210|퍼널차트|제안 전환 병목|가격표와 사례자료 자동 생성|",
		"|영업|계약|퍼널|64|퍼널차트|계약률 30.5%|우수 세일즈 스크립트 공유|",
		"|인사|지원|채용 파이프라인|312|퍼널차트|지원자는 충분|필수 역량 질문 추가|",
		"|인사|서류통과|채용 파이프라인|78|퍼널차트|서류 통과율 25%|JD 요건 재정의|",
		"|인사|1차면접|채용 파이프라인|41|퍼널차트|면접 일정 병목|자동 일정 후보 제안|",
		"|인사|오퍼|채용 파이프라인|9|퍼널차트|오퍼 수락률 확인 필요|보상/복지 비교표 생성|",
		"|공공기관|교통민원|민원 처리량|428|민원 히트맵|출퇴근 시간대 집중|FAQ와 안내문 배포|",
		"|공공기관|복지문의|민원 처리량|216|민원 히트맵|서류 안내 반복|신청서 체크리스트 생성|",
		"|공공기관|환경신고|민원 처리량|132|민원 히트맵|현장 확인 지연|점검표와 출동 우선순위 생성|",
		"|공공기관|예산집행|집행률|73|집행률 막대그래프|분기 말 집행 집중 위험|월별 집행 계획 재배치|",
	}, "\n")
}

func assistantIndustryScenarioCatalog() []assistantIndustryScenario {
	if assistantIndustryScenarioEnglish() {
		return assistantIndustryScenarioExpandCatalog(assistantIndustryScenarioCatalogEnglish())
	}
	return assistantIndustryScenarioExpandCatalog(assistantIndustryScenarioCatalogKorean())
}

func assistantIndustryScenarioExpandCatalog(base []assistantIndustryScenario) []assistantIndustryScenario {
	out := make([]assistantIndustryScenario, 0, len(base)*3)
	for _, scenario := range base {
		out = append(out, scenario)
		out = append(out, assistantIndustryScenarioVariant(scenario, "meeting"))
		out = append(out, assistantIndustryScenarioVariant(scenario, "execution"))
	}
	return out
}

func assistantIndustryScenarioVariant(s assistantIndustryScenario, kind string) assistantIndustryScenario {
	switch kind {
	case "execution":
		return assistantIndustryScenario{
			Group:       lang.T("assistant.industry.variant.execution.group", s.Group),
			Company:     lang.T("assistant.industry.variant.execution.company", s.Company),
			Request:     lang.T("assistant.industry.variant.execution.request", s.Request),
			Output:      lang.T("assistant.industry.variant.execution.output", s.Output),
			Attachments: lang.T("assistant.industry.variant.execution.attachments", s.Attachments),
			Command:     lang.T("assistant.industry.variant.execution.command", s.Command),
		}
	default:
		return assistantIndustryScenario{
			Group:       lang.T("assistant.industry.variant.meeting.group", s.Group),
			Company:     lang.T("assistant.industry.variant.meeting.company", s.Company),
			Request:     lang.T("assistant.industry.variant.meeting.request", s.Request),
			Output:      lang.T("assistant.industry.variant.meeting.output", s.Output),
			Attachments: lang.T("assistant.industry.variant.meeting.attachments", s.Attachments),
			Command:     lang.T("assistant.industry.variant.meeting.command", s.Command),
		}
	}
}

func assistantIndustryScenarioCatalogKorean() []assistantIndustryScenario {
	return []assistantIndustryScenario{
		{"SaaS 마케팅", "B2B 협업툴 회사", "경쟁 SaaS 가격제와 기능 변화를 찾아서 우리 요금제 개편 회의자료를 만들어줘.", "경쟁사 가격표 요약, 기능 매트릭스, 고객군별 가격 민감도, 3가지 요금제 개편안, 임원용 PPT를 만든다.", "DOCX 보고서, PPTX 제안서, 가격 비교 XLSX", "경쟁 SaaS 가격제 변화 보고서와 PPT 만들어줘"},
		{"SaaS 영업", "보안 솔루션 회사", "지난 6개월 리드와 계약 데이터를 보고 영업 파이프라인 병목을 그래프로 보여줘.", "리드-상담-제안-계약 퍼널, 단계별 전환율, 병목 원인, 세일즈 스크립트 개선안을 만든다.", "XLSX 퍼널표, PPTX 영업 회의자료", "영업 파이프라인 병목 분석표 만들어줘"},
		{"이커머스 마케팅", "생활용품 온라인몰", "매출이 떨어진 카테고리를 찾아서 상품 상세페이지와 광고 개선안을 내줘.", "카테고리별 매출 변화, 장바구니 이탈 지점, 상세페이지 카피 개선안, 상위 3개 SKU 집중안을 만든다.", "XLSX 매출표, DOCX 개선 보고서", "카테고리별 매출 하락 원인 분석해줘"},
		{"이커머스 MD", "패션 커머스 회사", "경쟁몰 프로모션을 비교해서 이번 주 할인 전략을 짜줘.", "경쟁몰 할인율, 무료배송 조건, 베스트 상품, 우리몰 대응 프로모션 캘린더를 만든다.", "PPTX 프로모션 계획, CSV 경쟁몰 추적표", "경쟁몰 프로모션 대응안 만들어줘"},
		{"금융 마케팅", "핀테크 카드 추천 앱", "2030 고객에게 먹힐 카드 혜택 메시지와 캠페인 구조를 만들어줘.", "고객 세그먼트, 혜택 포지셔닝, 광고 카피 12개, 랜딩페이지 구조, KPI 표를 만든다.", "DOCX 캠페인 브리프, PPTX 광고안", "카드 혜택 캠페인 기획안 만들어줘"},
		{"금융 영업", "지역 금융사", "지점별 실적을 보고 어느 지점을 어떻게 코칭할지 정리해줘.", "목표 대비 실적, 상품별 기여도, 지점별 코칭 포인트, 다음 주 영업 액션을 만든다.", "XLSX 지점 랭킹, PPTX 지점장 회의자료", "지점별 영업 실적 코칭표 만들어줘"},
		{"제약/헬스케어", "전문의약품 영업팀", "경쟁 제품 메시지와 학회 발표를 읽고 다음 병원 방문 메시지를 만들어줘.", "경쟁 메시지 변화, 의사 질문 예상, 제품 차별점, 병원 방문 스크립트를 만든다.", "DOCX 영업 브리프, PPTX 제품 메시지", "제약 경쟁 제품 메시지 분석해줘"},
		{"병원 운영", "피부과 네트워크", "환자 리뷰를 보고 서비스 개선 우선순위를 정리해줘.", "리뷰 감성 분류, 불만 반복 키워드, 진료과별 개선안, 직원 교육 스크립트를 만든다.", "DOCX 서비스 개선안, XLSX 리뷰 분류표", "병원 리뷰 기반 개선안 만들어줘"},
		{"제조 영업", "산업용 센서 회사", "대리점별 판매성과를 비교하고 다음 달 지원 우선순위를 정해줘.", "대리점 랭킹, 성장률, 제품군별 약점, 교육/판촉 지원 우선순위를 만든다.", "XLSX 대리점 성과표, PPTX 영업 회의자료", "대리점별 판매성과 분석해줘"},
		{"제조 구매", "화학 소재 회사", "원자재 가격 변동이 마진에 미치는 영향을 그래프로 정리해줘.", "원자재별 가격 추이, BOM 영향, 제품별 마진 변화, 가격 조정 시나리오를 만든다.", "XLSX 원가 시뮬레이션, DOCX 경영 보고서", "원자재 가격 마진 영향 분석해줘"},
		{"모빌리티", "전기차 충전 스타트업", "지역별 충전 수요와 경쟁 설치 현황을 비교해 우선 진출 지역을 골라줘.", "지역별 수요 점수, 경쟁 충전기 밀도, 후보 입지, 영업 제휴 타깃을 만든다.", "XLSX 지역 점수표, PPTX 진출 전략", "충전 인프라 진출 지역 분석해줘"},
		{"식음료", "편의점 음료 브랜드", "POS 데이터를 보고 신제품 매대 전략과 발주 가이드를 만들어줘.", "시간대별 판매, 지역별 SKU 반응, 매대 위치 제안, 발주량 가이드를 만든다.", "XLSX POS 분석표, DOCX 매장 가이드", "편의점 POS 판매 분석해줘"},
		{"미디어", "콘텐츠 제작사", "경쟁 유튜브 채널의 인기 포맷을 분석해서 다음 달 콘텐츠 캘린더를 만들어줘.", "조회수/유지율 상위 포맷, 제목 패턴, 썸네일 방향, 4주 업로드 캘린더를 만든다.", "PPTX 콘텐츠 전략, XLSX 콘텐츠 캘린더", "경쟁 채널 콘텐츠 캘린더 분석해줘"},
		{"광고기획", "종합광고대행사", "신제품 런칭 캠페인 콘셉트와 채널 믹스, A/B 테스트 계획을 만들어줘.", "캠페인 콘셉트 5개, 타깃 페르소나, 카피 12개, 채널별 예산, A/B 테스트표를 만든다.", "PPTX 캠페인 제안서, XLSX KPI 표", "신제품 광고 캠페인 제안서 만들어줘"},
		{"광고기획", "지역 병원 광고팀", "병원 신규 진료과 오픈 광고를 과장 없이 신뢰감 있게 기획해줘.", "타깃 환자군, 금지 표현 체크, 랜딩페이지 구조, 검색광고 키워드, 전화 상담 스크립트를 만든다.", "DOCX 광고 브리프, XLSX 키워드 표", "병원 광고기획안과 키워드표 만들어줘"},
		{"인사팀", "AI 스타트업", "백엔드 엔지니어 채용공고와 후보자 스크리닝 표, 면접 질문을 만들어줘.", "JD, 필수/우대 요건, 후보자 평가표, 기술 면접 질문, 합격/불합격 메일 초안을 만든다.", "DOCX 채용 패키지, XLSX 후보자 스크리닝 표", "백엔드 엔지니어 채용 패키지 만들어줘"},
		{"인사팀", "제조 중견기업", "생산관리 직무 지원자를 비교하고 면접 순서를 정리해줘.", "후보자 경력 요약, 필수역량 점수, 면접 우선순위, 질문지와 평가 루브릭을 만든다.", "XLSX 후보자 비교표, DOCX 면접 질문지", "생산관리 후보자 스크리닝표 만들어줘"},
		{"인사팀", "외식 프랜차이즈", "매장 매니저 온보딩 자료와 첫 30일 체크리스트를 만들어줘.", "입사 첫날 안내, 교육 순서, 점검표, 30/60/90일 목표, 점장 피드백 양식을 만든다.", "DOCX 온보딩 문서, XLSX 체크리스트", "매장 매니저 온보딩 자료 만들어줘"},
		{"공공기관", "구청 민원실", "지난달 민원을 유형별로 분류하고 시민 안내문 초안을 만들어줘.", "민원 유형 분류, 처리 시간, 반복 질문 FAQ, 시민 안내문, 부서별 개선 제안을 만든다.", "DOCX 민원 보고서, XLSX 민원 처리표", "구청 민원 유형 분석과 안내문 만들어줘"},
		{"공공기관", "중앙부처 정책팀", "새 정책 발표용 보도자료와 예상 질문 답변을 만들어줘.", "정책 요약, 보도자료 초안, Q&A, 이해관계자별 반응, 브리핑 발언문을 만든다.", "DOCX 보도자료, PPTX 브리핑 자료", "정책 보도자료와 Q&A 만들어줘"},
		{"공공기관", "시청 예산팀", "분기 예산 집행 현황을 보고 지연 사업과 리스크를 정리해줘.", "사업별 집행률, 지연 사유, 위험도, 다음 조치, 보고용 그래프 데이터를 만든다.", "XLSX 예산 집행표, PPTX 간부회의 자료", "분기 예산 집행 현황 보고서 만들어줘"},
		{"공공기관", "현장 점검반", "공사 현장 점검표와 결과 보고서 양식을 만들어줘.", "점검 항목, 위험도 기준, 사진 첨부 위치, 조치 기한, 보고서 템플릿을 만든다.", "DOCX 점검표, XLSX 조치 관리표", "현장 점검표와 조치관리표 만들어줘"},
		{"교육", "온라인 교육 회사", "수강생 이탈 원인을 분석하고 리텐션 캠페인을 만들어줘.", "완강률, 이탈 구간, 메시지 시점, 쿠폰/튜터링 실험안, 리텐션 KPI를 만든다.", "XLSX 수강 퍼널, DOCX 캠페인안", "수강생 이탈 원인 분석해줘"},
		{"물류", "풀필먼트 회사", "배송 지연 데이터를 보고 고객 안내문과 운영 개선안을 만들어줘.", "지연 구간, 센터별 병목, 고객 안내 메시지, 운영 개선 우선순위를 만든다.", "XLSX 배송 지연표, DOCX 고객 안내문", "배송 지연 원인과 안내문 만들어줘"},
		{"전략기획", "중견 화학회사", "이번 분기 원자재·환율·규제 리스크를 경영회의용으로 정리해줘.", "나프타/환율/물류/규제 신호, 제품군별 영향, 의사결정 안건, 경영진 질문 리스트를 만든다.", "DOCX 경영 브리프, PPTX 이사회 자료, XLSX 리스크 표", "화학회사 분기 리스크 경영회의 자료 만들어줘"},
		{"재무팀", "SaaS 스타트업", "부서별 SaaS 구독료와 사용률을 보고 줄일 비용을 찾아줘.", "구독료, 활성 사용자, 중복 도구, 해지 후보, 승인 전 확인 항목을 만든다.", "XLSX 구독비 분석표, DOCX 비용절감 보고서", "SaaS 구독비 절감 후보표 만들어줘"},
		{"재무팀", "제조기업", "법인카드 영수증과 예산 코드를 비교해서 누락을 찾아줘.", "영수증 누락, 예산 코드 오류, 결재 보류 항목, 담당자별 요청 메시지를 만든다.", "XLSX 정산표, DOCX 결재 보완 요청서", "법인카드 영수증 정산표 만들어줘"},
		{"법무팀", "B2B 소프트웨어 회사", "벤더 계약서에서 자동갱신과 개인정보 조항 리스크를 정리해줘.", "책임 제한, DPA, 자동갱신, 해지, 협상 질문, 승인 체크리스트를 만든다.", "DOCX 계약 검토표, XLSX 조항 리스크 매트릭스", "벤더 계약 리스크 검토 패키지 만들어줘"},
		{"고객지원", "구독 서비스", "지난주 환불/장애 티켓을 분류하고 답변 초안을 만들어줘.", "티켓 유형, 긴급도, 환불 기준, 에스컬레이션, 고객 답변 초안을 만든다.", "XLSX 티켓 분류표, DOCX 답변 초안", "고객지원 티켓 분류와 답변 초안 만들어줘"},
		{"제품팀", "모바일 앱 회사", "사용자 피드백을 PRD와 다음 릴리즈 백로그로 바꿔줘.", "문제 정의, 사용자 스토리, 요구사항, 제외 범위, 백로그, QA 체크리스트를 만든다.", "DOCX PRD, PPTX 로드맵, XLSX 백로그", "사용자 피드백으로 PRD 패키지 만들어줘"},
		{"IT/보안", "금융 계열사", "최근 로그인 이상 징후와 권한 변경을 보안 보고서로 정리해줘.", "이상 로그인, 권한 변경, 미사용 계정, 위험도, 조치 요청 문구를 만든다.", "DOCX 보안 보고서, XLSX 계정 점검표", "계정 보안 점검 보고서 만들어줘"},
		{"구매팀", "식품 제조사", "납품업체 견적 4개를 비교해서 발주 추천안을 만들어줘.", "가격, 납기, 품질 이력, 결제 조건, 발주 추천, 협상 질문을 만든다.", "XLSX 견적 비교표, DOCX 발주 추천서", "납품업체 견적 비교 발주안 만들어줘"},
		{"경영지원", "전문 서비스 회사", "월간 회의록과 비용 항목을 묶어 대표 보고서를 만들어줘.", "부서별 진행 상황, 비용 이상치, 의사결정 요청, 다음 달 일정표를 만든다.", "DOCX 대표 보고서, PPTX 월간 운영회의 자료", "월간 경영지원 보고서 만들어줘"},
		{"영업운영", "B2B 장비 회사", "상위 리드 20개를 점수화하고 다음 콜 스크립트를 만들어줘.", "리드 점수, 산업/규모, 관심 제품, 다음 콜 우선순위, 콜 스크립트를 만든다.", "XLSX 리드 점수표, DOCX 콜 스크립트", "상위 리드 20개 후속 콜 계획 만들어줘"},
		{"CS 운영", "전자제품 회사", "불량 문의를 원인별로 묶고 FAQ와 교환 기준을 만들어줘.", "문의 유형, 반복 원인, 교환/수리 기준, 고객 안내문, 내부 처리표를 만든다.", "DOCX FAQ, XLSX 처리 기준표", "불량 문의 FAQ와 교환 기준 만들어줘"},
		{"홍보팀", "지자체 문화재단", "행사 취소 공지와 언론 Q&A를 차분한 문장으로 준비해줘.", "공지문, 보도자료, 예상 질문, 민원 응대 문장, SNS 짧은 안내문을 만든다.", "DOCX 공지/보도자료, PPTX 브리핑 자료", "행사 취소 공지와 Q&A 만들어줘"},
		{"데이터팀", "리테일 체인", "매장별 매출과 재고 데이터를 보고 품절 위험을 예측해줘.", "품절 위험 SKU, 매장별 재고일수, 발주 우선순위, 대체 상품 제안을 만든다.", "XLSX 재고 위험표, PPTX 매장 운영자료", "매장별 품절 위험 분석표 만들어줘"},
		{"품질팀", "자동차 부품사", "클레임 데이터를 8D 보고서와 CAPA 표로 바꿔줘.", "불량 유형, 원인 가설, 임시조치, 근본원인, CAPA, 고객 회신 초안을 만든다.", "DOCX 8D 보고서, XLSX CAPA 표", "품질 클레임 8D CAPA 패키지 만들어줘"},
		{"해외영업", "K-뷰티 브랜드", "일본 바이어 미팅 전 제품 소개와 가격표를 준비해줘.", "바이어 관심사, 제품 포지셔닝, 가격표, MOQ, 미팅 질문, 후속 메일 초안을 만든다.", "PPTX 바이어 미팅자료, XLSX 가격표", "일본 바이어 미팅 준비자료 만들어줘"},
		{"부동산 운영", "공유오피스 회사", "입주 문의와 공실률을 보고 다음 주 영업 캠페인을 짜줘.", "공실률, 문의 채널, 투어 예약, 가격 프로모션, 영업 스크립트를 만든다.", "XLSX 공실 분석표, DOCX 캠페인안", "공유오피스 공실 영업 캠페인 만들어줘"},
		{"비영리", "후원 캠페인 팀", "지난 캠페인 후원자 데이터를 보고 재참여 메시지를 만들어줘.", "후원자 세그먼트, 재참여 타이밍, 메시지 8개, 성과 지표표를 만든다.", "DOCX 캠페인 메시지, XLSX 후원자 세그먼트", "후원자 재참여 캠페인 만들어줘"},
		{"연구개발", "바이오 소재 연구소", "실험 노트를 요약해서 다음 실험 계획과 리스크를 정리해줘.", "실험 결과 요약, 변수별 관찰, 실패 원인, 다음 실험 설계, 안전 체크를 만든다.", "DOCX 실험 요약, XLSX 실험 변수표", "실험 노트 다음 계획표 만들어줘"},
	}
}

func assistantIndustryScenarioCatalogEnglish() []assistantIndustryScenario {
	return []assistantIndustryScenario{
		{"SaaS Marketing", "B2B collaboration software company", "Find competitor pricing and feature changes, then make meeting materials for our pricing redesign.", "Creates competitor pricing summary, feature matrix, customer-segment price sensitivity, three pricing redesign options, and an executive deck.", "DOCX report, PPTX proposal, XLSX price comparison", "Create a SaaS competitor pricing change report and PPT"},
		{"SaaS Sales", "Security software company", "Analyze six months of leads and contracts and show the sales-pipeline bottleneck as a graph.", "Creates lead-consultation-proposal-contract funnel, conversion rates, bottleneck causes, and script improvements.", "XLSX funnel table, PPTX sales meeting deck", "Create a sales pipeline bottleneck analysis table"},
		{"E-commerce Marketing", "Home-goods online store", "Find declining categories and suggest product-page and ad improvements.", "Creates category revenue change, cart-abandonment points, product-page copy fixes, and top-three SKU focus plan.", "XLSX revenue table, DOCX improvement report", "Analyze category revenue decline"},
		{"E-commerce MD", "Fashion commerce company", "Compare competitor promotions and create this week's discount strategy.", "Creates competitor discount rates, free-shipping rules, best products, and our promotion calendar.", "PPTX promotion plan, CSV competitor tracker", "Create a competitor promotion response plan"},
		{"Financial Marketing", "Fintech card recommendation app", "Create benefit messaging and campaign structure for customers in their 20s and 30s.", "Creates customer segments, benefit positioning, 12 ad-copy ideas, landing-page structure, and KPI table.", "DOCX campaign brief, PPTX ad plan", "Create a card-benefit campaign plan"},
		{"Financial Sales", "Regional financial firm", "Review branch performance and summarize which branches need coaching.", "Creates target-vs-result table, product contribution, branch coaching points, and next-week sales actions.", "XLSX branch ranking, PPTX branch-lead meeting deck", "Create a branch sales coaching table"},
		{"Pharma Healthcare", "Prescription-drug sales team", "Read competitor product messaging and conference notes, then prepare hospital-visit messaging.", "Creates competitor-message shifts, expected physician questions, differentiation points, and visit script.", "DOCX sales brief, PPTX product message", "Analyze pharma competitor product messaging"},
		{"Hospital Operations", "Dermatology clinic network", "Analyze patient reviews and decide service-improvement priorities.", "Creates review sentiment, repeated complaints, department improvements, and staff training script.", "DOCX service improvement plan, XLSX review classifier", "Create a hospital review improvement plan"},
		{"Manufacturing Sales", "Industrial sensor company", "Compare distributor performance and set next-month support priorities.", "Creates distributor ranking, growth rate, product-line weakness, and training/promotion priorities.", "XLSX distributor performance table, PPTX sales deck", "Analyze distributor sales performance"},
		{"Manufacturing Procurement", "Chemical materials company", "Graph how raw-material price changes affect margin.", "Creates raw-material price trend, BOM impact, product-margin scenarios, and price-adjustment options.", "XLSX cost simulation, DOCX management report", "Analyze raw-material margin impact"},
		{"Mobility", "EV charging startup", "Compare local charging demand and competitor installations to pick expansion areas.", "Creates regional demand score, competitor charger density, candidate sites, and partnership targets.", "XLSX regional scorecard, PPTX expansion strategy", "Analyze EV charging expansion regions"},
		{"Food and Beverage", "Convenience-store beverage brand", "Use POS data to create a new-product shelf strategy and reorder guide.", "Creates time-of-day sales, regional SKU response, shelf-position proposal, and reorder quantity guide.", "XLSX POS analysis, DOCX store guide", "Analyze convenience-store POS sales"},
		{"Media", "Content production company", "Analyze popular formats from competitor YouTube channels and make next month's content calendar.", "Creates top formats by views and retention, title patterns, thumbnail direction, and four-week calendar.", "PPTX content strategy, XLSX content calendar", "Analyze competitor channel content calendar"},
		{"Advertising Planning", "Full-service ad agency", "Create launch campaign concepts, channel mix, and A/B test plan for a new product.", "Creates five campaign concepts, personas, 12 copy ideas, channel budget, and A/B test table.", "PPTX campaign proposal, XLSX KPI table", "Create a new-product ad campaign proposal"},
		{"Advertising Planning", "Local hospital ad team", "Plan a trustworthy, non-exaggerated campaign for a new clinic department.", "Creates target patient groups, prohibited-expression check, landing structure, search keywords, and phone script.", "DOCX ad brief, XLSX keyword table", "Create a hospital ad plan and keyword table"},
		{"HR", "AI startup", "Create a backend engineer job post, candidate screening table, and interview questions.", "Creates JD, must-have and preferred requirements, candidate scorecard, technical interview questions, and email drafts.", "DOCX recruiting package, XLSX screening table", "Create a backend engineer recruiting package"},
		{"HR", "Mid-sized manufacturer", "Compare production-management candidates and order interview priority.", "Creates candidate career summary, skill score, interview priority, questions, and evaluation rubric.", "XLSX candidate comparison, DOCX interview guide", "Create a production-management candidate screening table"},
		{"HR", "Restaurant franchise", "Create store-manager onboarding material and a first-30-days checklist.", "Creates day-one guide, training order, checklists, 30/60/90 goals, and manager feedback form.", "DOCX onboarding document, XLSX checklist", "Create store-manager onboarding materials"},
		{"Public Agency", "District civil-service office", "Classify last month's civil complaints and draft citizen notices.", "Creates case-type classification, processing time, repeated FAQ, citizen notice, and department improvement proposals.", "DOCX complaint report, XLSX case table", "Create district civil-complaint analysis and notice"},
		{"Public Agency", "Central ministry policy team", "Create a press release and expected Q&A for a new policy announcement.", "Creates policy summary, press release draft, Q&A, stakeholder reactions, and briefing remarks.", "DOCX press release, PPTX briefing deck", "Create a policy press release and Q&A"},
		{"Public Agency", "City budget team", "Report quarterly budget execution and summarize delayed projects and risks.", "Creates project execution rate, delay causes, risk level, next measures, and graph data.", "XLSX budget table, PPTX leadership deck", "Create a quarterly budget execution report"},
		{"Public Agency", "Field inspection team", "Create a construction-site inspection checklist and result-report template.", "Creates inspection items, risk criteria, photo fields, action deadlines, and report template.", "DOCX inspection checklist, XLSX action tracker", "Create a field inspection checklist and action tracker"},
		{"Education", "Online education company", "Analyze learner churn and create a retention campaign.", "Creates completion rate, churn segment, message timing, coupon/tutoring experiments, and retention KPIs.", "XLSX learner funnel, DOCX campaign plan", "Analyze learner churn causes"},
		{"Logistics", "Fulfillment company", "Analyze delivery-delay data and create customer notices plus operations fixes.", "Creates delay segments, center bottlenecks, customer message, and operations priority list.", "XLSX delay table, DOCX customer notice", "Create delivery-delay causes and customer notice"},
		{"Strategy", "Mid-sized chemical company", "Summarize quarterly raw-material, FX, and regulatory risks for the management meeting.", "Creates naphtha, FX, logistics, and regulation signals, product-line impact, decision agenda, and executive questions.", "DOCX executive brief, PPTX board deck, XLSX risk table", "Create chemical-company quarterly risk meeting materials"},
		{"Finance", "SaaS startup", "Review department SaaS subscription cost and usage to find savings.", "Creates subscription cost, active users, duplicate tools, cancellation candidates, and checks before approval.", "XLSX subscription analysis, DOCX cost-saving report", "Create SaaS subscription savings candidates"},
		{"Finance", "Manufacturer", "Compare corporate-card receipts with budget codes and find missing items.", "Creates missing receipts, budget-code errors, approval holds, and owner request messages.", "XLSX reconciliation table, DOCX approval request", "Create corporate-card receipt reconciliation"},
		{"Legal", "B2B software company", "Review vendor contract risks in auto-renewal and data-processing clauses.", "Creates liability, DPA, auto-renewal, termination, negotiation questions, and approval checklist.", "DOCX contract review, XLSX clause-risk matrix", "Create a vendor-contract risk review package"},
		{"Customer Support", "Subscription service", "Classify last week's refund and outage tickets and draft replies.", "Creates ticket types, urgency, refund criteria, escalation route, and customer-reply drafts.", "XLSX ticket classifier, DOCX reply drafts", "Create customer-support ticket triage and replies"},
		{"Product", "Mobile app company", "Turn user feedback into a PRD and next-release backlog.", "Creates problem definition, user stories, requirements, exclusions, backlog, and QA checklist.", "DOCX PRD, PPTX roadmap, XLSX backlog", "Create a PRD package from user feedback"},
		{"IT Security", "Financial affiliate", "Summarize recent suspicious logins and permission changes into a security report.", "Creates unusual-login review, permission changes, unused accounts, risk scoring, and action-request wording.", "DOCX security report, XLSX account audit", "Create an account security audit report"},
		{"Procurement", "Food manufacturer", "Compare four supplier quotes and recommend a purchase order path.", "Creates price, lead time, quality history, payment terms, order recommendation, and negotiation questions.", "XLSX quote comparison, DOCX purchase recommendation", "Create supplier quote comparison and PO recommendation"},
		{"Business Operations", "Professional services company", "Combine monthly meeting minutes and expense items into a CEO report.", "Creates department status, expense anomalies, decision requests, and next-month schedule table.", "DOCX CEO report, PPTX monthly operations deck", "Create a monthly business-operations report"},
		{"Sales Operations", "B2B equipment company", "Score the top 20 leads and create the next-call script.", "Creates lead score, industry and size, product interest, call priority, and call script.", "XLSX lead score table, DOCX call script", "Create a top-20 lead follow-up plan"},
		{"CS Operations", "Electronics company", "Group defect inquiries by cause and create FAQ plus exchange criteria.", "Creates inquiry types, repeated causes, exchange/repair criteria, customer notices, and internal processing table.", "DOCX FAQ, XLSX handling criteria", "Create defect-inquiry FAQ and exchange criteria"},
		{"Public Relations", "Local cultural foundation", "Prepare cancellation notice and press Q&A in calm language.", "Creates public notice, press release, expected questions, complaint responses, and short social posts.", "DOCX notice and press release, PPTX briefing deck", "Create event-cancellation notice and Q&A"},
		{"Data Team", "Retail chain", "Use store revenue and inventory data to predict stockout risks.", "Creates stockout-risk SKUs, store inventory days, reorder priority, and substitute-product suggestions.", "XLSX inventory-risk table, PPTX store operations deck", "Create store-level stockout risk analysis"},
		{"Quality", "Automotive parts supplier", "Turn claim data into an 8D report and CAPA table.", "Creates defect type, cause hypotheses, containment, root cause, CAPA, and customer reply draft.", "DOCX 8D report, XLSX CAPA table", "Create a quality-claim 8D CAPA package"},
		{"International Sales", "K-beauty brand", "Prepare product introduction and price sheet before a Japan buyer meeting.", "Creates buyer interests, product positioning, price sheet, MOQ, meeting questions, and follow-up email draft.", "PPTX buyer meeting deck, XLSX price sheet", "Create Japan buyer meeting materials"},
		{"Real Estate Operations", "Shared-office company", "Review inbound inquiries and vacancy rate and plan next week's sales campaign.", "Creates vacancy rate, inquiry channel, tour booking flow, price promotion, and sales script.", "XLSX vacancy analysis, DOCX campaign plan", "Create a shared-office vacancy campaign"},
		{"Nonprofit", "Donation campaign team", "Use last campaign donor data to create re-engagement messages.", "Creates donor segments, re-engagement timing, eight messages, and performance KPI table.", "DOCX campaign messages, XLSX donor segments", "Create a donor re-engagement campaign"},
		{"R&D", "Biomaterials lab", "Summarize experiment notes into next experiment plan and risks.", "Creates experiment summary, observations by variable, failure causes, next experiment design, and safety checks.", "DOCX experiment summary, XLSX variable table", "Create next-plan table from experiment notes"},
	}
}
