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

	"github.com/meshclaw/meshclaw/internal/evidence"
	"github.com/meshclaw/meshclaw/internal/lang"
	"github.com/meshclaw/meshclaw/internal/osauto"
	"github.com/meshclaw/meshclaw/internal/publish"
	"github.com/meshclaw/meshclaw/internal/tts"
)

type assistantDepartmentWorkflowSpec struct {
	Kind       string
	Title      string
	Audience   string
	Summary    string
	Highlights []string
	Commands   []string
	DocBody    string
	DeckBody   string
	SheetBody  string
	GraphBody  string
}

type assistantStructuredWorkflowSpec struct {
	Kind            string
	TitleKey        string
	AudienceKey     string
	SummaryKey      string
	HighlightKeys   []string
	CommandKeys     []string
	Sections        []assistantStructuredWorkflowSection
	DeckSlides      []assistantStructuredWorkflowSlide
	SheetHeaders    []string
	SheetRows       [][]string
	FlowTitleKey    string
	FlowNodes       []assistantStructuredWorkflowNode
	FlowEdges       []assistantStructuredWorkflowEdge
	Quadrant        assistantStructuredWorkflowQuadrant
	ChecklistTitle  string
	ChecklistHeader []string
	ChecklistRows   [][]string
}

type assistantStructuredWorkflowSection struct {
	TitleKey string
	BodyKeys []string
}

type assistantStructuredWorkflowSlide struct {
	TitleKey string
	BodyKeys []string
}

type assistantStructuredWorkflowNode struct {
	ID       string
	LabelKey string
}

type assistantStructuredWorkflowEdge struct {
	From string
	To   string
}

type assistantStructuredWorkflowQuadrant struct {
	TitleKey string
	XAxisKey string
	YAxisKey string
	Q1Key    string
	Q2Key    string
	Q3Key    string
	Q4Key    string
	Points   []assistantStructuredWorkflowPoint
}

type assistantStructuredWorkflowPoint struct {
	LabelKey string
	X        float64
	Y        float64
}

func assistantStructuredDepartmentWorkflowSpec(data assistantStructuredWorkflowSpec) assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:       data.Kind,
		Title:      lang.T(data.TitleKey),
		Audience:   lang.T(data.AudienceKey),
		Summary:    lang.T(data.SummaryKey),
		Highlights: assistantStructuredWorkflowTexts(data.HighlightKeys),
		Commands:   assistantStructuredWorkflowTexts(data.CommandKeys),
		DocBody:    assistantStructuredWorkflowDocBody(data),
		DeckBody:   assistantStructuredWorkflowDeckBody(data),
		SheetBody:  assistantStructuredWorkflowSheetBody(data),
		GraphBody:  assistantStructuredWorkflowGraphBody(data),
	}
}

func assistantStructuredWorkflowTexts(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if text := strings.TrimSpace(assistantStructuredWorkflowText(value)); text != "" {
			out = append(out, text)
		}
	}
	return out
}

func assistantStructuredWorkflowText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "assistant.") {
		return lang.T(value)
	}
	return value
}

func assistantStructuredWorkflowDocBody(data assistantStructuredWorkflowSpec) string {
	lines := []string{
		"# " + lang.T(data.TitleKey),
		"",
		lang.T(data.SummaryKey),
	}
	for _, section := range data.Sections {
		title := strings.TrimSpace(lang.T(section.TitleKey))
		if title == "" {
			continue
		}
		lines = append(lines, "", "## "+title)
		for _, item := range assistantStructuredWorkflowTexts(section.BodyKeys) {
			lines = append(lines, "- "+item)
		}
	}
	if len(data.SheetHeaders) > 0 && len(data.SheetRows) > 0 {
		lines = append(lines, "", "## "+lang.T("assistant.workflow.structured.table"), "")
		lines = append(lines, assistantStructuredWorkflowTable(data.SheetHeaders, data.SheetRows))
	}
	return strings.Join(compactBlankLines(lines), "\n")
}

func assistantStructuredWorkflowDeckBody(data assistantStructuredWorkflowSpec) string {
	lines := []string{
		"# " + lang.T(data.TitleKey),
		lang.T(data.SummaryKey),
	}
	for _, slide := range data.DeckSlides {
		title := strings.TrimSpace(lang.T(slide.TitleKey))
		if title == "" {
			continue
		}
		lines = append(lines, "", "# "+title)
		body := assistantStructuredWorkflowTexts(slide.BodyKeys)
		if len(body) == 0 {
			continue
		}
		lines = append(lines, strings.Join(body, "\n"))
	}
	return strings.Join(compactBlankLines(lines), "\n")
}

func assistantStructuredWorkflowSheetBody(data assistantStructuredWorkflowSpec) string {
	return assistantStructuredWorkflowTable(data.SheetHeaders, data.SheetRows)
}

func assistantStructuredWorkflowTable(headers []string, rows [][]string) string {
	if len(headers) == 0 {
		return ""
	}
	translatedHeaders := assistantStructuredWorkflowTexts(headers)
	if len(translatedHeaders) == 0 {
		return ""
	}
	lines := []string{
		"| " + strings.Join(translatedHeaders, " | ") + " |",
		"| " + strings.Join(assistantStructuredWorkflowTableDividers(len(translatedHeaders)), " | ") + " |",
	}
	for _, row := range rows {
		cells := assistantStructuredWorkflowTexts(row)
		if len(cells) == 0 {
			continue
		}
		for len(cells) < len(translatedHeaders) {
			cells = append(cells, "")
		}
		if len(cells) > len(translatedHeaders) {
			cells = cells[:len(translatedHeaders)]
		}
		lines = append(lines, "| "+strings.Join(cells, " | ")+" |")
	}
	return strings.Join(lines, "\n")
}

func assistantStructuredWorkflowTableDividers(count int) []string {
	out := make([]string, count)
	for i := range out {
		out[i] = "---"
	}
	return out
}

func assistantStructuredWorkflowGraphBody(data assistantStructuredWorkflowSpec) string {
	lines := []string{}
	if len(data.FlowNodes) > 0 && len(data.FlowEdges) > 0 {
		lines = append(lines, "## "+lang.T(data.FlowTitleKey), "", "```mermaid", "flowchart LR")
		nodeLabels := map[string]string{}
		for _, node := range data.FlowNodes {
			id := assistantStructuredWorkflowMermaidID(node.ID)
			if id == "" {
				continue
			}
			nodeLabels[id] = lang.T(node.LabelKey)
		}
		for _, edge := range data.FlowEdges {
			from := assistantStructuredWorkflowMermaidID(edge.From)
			to := assistantStructuredWorkflowMermaidID(edge.To)
			if from == "" || to == "" {
				continue
			}
			lines = append(lines, fmt.Sprintf("  %s[\"%s\"] --> %s[\"%s\"]", from, assistantMermaidEscape(nodeLabels[from]), to, assistantMermaidEscape(nodeLabels[to])))
		}
		lines = append(lines, "```")
	}
	if strings.TrimSpace(data.Quadrant.TitleKey) != "" && len(data.Quadrant.Points) > 0 {
		lines = append(lines, "", "## "+lang.T(data.Quadrant.TitleKey), "", "```mermaid", "quadrantChart")
		lines = append(lines,
			"  title "+lang.T(data.Quadrant.TitleKey),
			"  x-axis "+lang.T(data.Quadrant.XAxisKey),
			"  y-axis "+lang.T(data.Quadrant.YAxisKey),
			"  quadrant-1 "+lang.T(data.Quadrant.Q1Key),
			"  quadrant-2 "+lang.T(data.Quadrant.Q2Key),
			"  quadrant-3 "+lang.T(data.Quadrant.Q3Key),
			"  quadrant-4 "+lang.T(data.Quadrant.Q4Key),
		)
		for _, point := range data.Quadrant.Points {
			lines = append(lines, fmt.Sprintf("  %s: [%.2f, %.2f]", lang.T(point.LabelKey), point.X, point.Y))
		}
		lines = append(lines, "```")
	}
	if strings.TrimSpace(data.ChecklistTitle) != "" && len(data.ChecklistHeader) > 0 && len(data.ChecklistRows) > 0 {
		lines = append(lines, "", "## "+lang.T(data.ChecklistTitle), "", assistantStructuredWorkflowTable(data.ChecklistHeader, data.ChecklistRows))
	}
	return strings.Join(compactBlankLines(lines), "\n")
}

func assistantStructuredWorkflowMermaidID(value string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(value) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func assistantMermaidEscape(value string) string {
	value = strings.ReplaceAll(value, "\\", "\\\\")
	value = strings.ReplaceAll(value, "\"", "\\\"")
	return value
}

func assistantDepartmentWorkflowReply(opts ListenOptions, request string) (string, bool) {
	spec, ok := assistantDepartmentWorkflowSpecFor(request)
	if !ok {
		return "", false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	restorePreview := temporarilySkipPreviewImages()
	doc := osauto.CreateArgosDocument(ctx, lang.T("assistant.workflow.artifact.report_title", spec.Title), spec.DocBody)
	deck := osauto.CreatePresentation(ctx, lang.T("assistant.workflow.artifact.deck_title", spec.Title), spec.DeckBody, spec.Audience, 8, "")
	sheet := osauto.CreateSpreadsheet(ctx, lang.T("assistant.workflow.artifact.sheet_title", spec.Title), spec.SheetBody)
	restorePreview()

	graphPath, graphErr := writeAssistantDepartmentGraphMarkdown(spec)
	chartPath, chartErr := writeAssistantDepartmentGraphSVG(spec)
	wantsVoice := assistantBrowserResearchPackageWantsVoice(request)
	voiceScript := ""
	var audio tts.Result
	var audioErr error
	if wantsVoice {
		voiceScript = assistantDepartmentWorkflowVoiceScript(spec)
		audio, audioErr = tts.Synthesize(tts.Options{
			Text:     voiceScript,
			Engine:   "edge-tts",
			Basename: "department-workflow-brief-" + time.Now().UTC().Format("20060102T150405Z"),
		})
	}

	rememberAssistantArtifact(opts, spec.Title, "department_workflow", doc, deck, sheet)
	attachments := assistantIndustryScenarioAttachments(doc, deck, sheet)
	if strings.TrimSpace(graphPath) != "" && graphErr == nil {
		attachments = uniqueShowcaseAttachments(append(attachments, graphPath))
	}
	if strings.TrimSpace(chartPath) != "" && chartErr == nil {
		attachments = uniqueShowcaseAttachments(append(attachments, chartPath))
	}
	if strings.TrimSpace(audio.Path) != "" && audioErr == nil {
		attachments = uniqueShowcaseAttachments(append(attachments, audio.Path))
	}
	payload := map[string]interface{}{
		"kind":         "assistant_department_workflow",
		"workflow":     spec.Kind,
		"title":        spec.Title,
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
		"created_at":   time.Now().UTC(),
	}
	record, storeErr := evidence.Store("assistant-department-workflow", firstNonEmpty(opts.TargetID, "assistant"), spec.Title, payload)

	lines := []string{
		lang.T("assistant.workflow.created", spec.Title),
		spec.Summary,
	}
	lines = append(lines, assistantDepartmentWorkflowResultLines(spec)...)
	lines = append(lines, "", lang.T("assistant.workflow.files"))
	if doc.OK {
		lines = append(lines, "- "+lang.T("assistant.workflow.file.doc"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.workflow.file.doc_failed", firstNonEmpty(doc.Error, doc.Stderr, "unknown error")))
	}
	if deck.OK {
		lines = append(lines, "- "+lang.T("assistant.workflow.file.ppt"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.workflow.file.ppt_failed", firstNonEmpty(deck.Error, deck.Stderr, "unknown error")))
	}
	if sheet.OK {
		lines = append(lines, "- "+lang.T("assistant.workflow.file.sheet"))
	} else {
		lines = append(lines, "- "+lang.T("assistant.workflow.file.sheet_failed", firstNonEmpty(sheet.Error, sheet.Stderr, "unknown error")))
	}
	if strings.TrimSpace(spec.GraphBody) != "" {
		if graphErr == nil && strings.TrimSpace(graphPath) != "" {
			lines = append(lines, "- "+lang.T("assistant.workflow.file.graph"))
		} else {
			lines = append(lines, "- "+lang.T("assistant.workflow.file.graph_failed", firstNonEmpty(errorString(graphErr), "unknown error")))
		}
	}
	if strings.TrimSpace(spec.GraphBody) != "" {
		if chartErr == nil && strings.TrimSpace(chartPath) != "" {
			lines = append(lines, "- "+lang.T("assistant.workflow.file.chart"))
		} else {
			lines = append(lines, "- "+lang.T("assistant.workflow.file.chart_failed", firstNonEmpty(errorString(chartErr), "unknown error")))
		}
	}
	if wantsVoice {
		if audioErr == nil && strings.TrimSpace(audio.Path) != "" {
			lines = append(lines, "- "+lang.T("assistant.workflow.file.audio"))
		} else {
			lines = append(lines, "- "+lang.T("assistant.workflow.file.audio_failed", firstNonEmpty(errorString(audioErr), "unknown error")))
		}
	}
	if len(spec.Highlights) > 0 {
		lines = append(lines, "", lang.T("assistant.workflow.preview"))
		for _, item := range spec.Highlights {
			lines = append(lines, "- "+item)
		}
	}
	if len(spec.Commands) > 0 {
		lines = append(lines, "", lang.T("assistant.workflow.next"))
		for _, item := range spec.Commands {
			lines = append(lines, "- `"+item+"`")
		}
	}
	if strings.TrimSpace(voiceScript) != "" {
		lines = append(lines, "", lang.T("assistant.workflow.voice_preview"), trimForContext(voiceScript, 520))
	}
	if targetRef := inferAssistantSignalTargetRef(request); targetRef != "" {
		return formatAssistantDepartmentWorkflowSendResult(opts, targetRef, spec, lines, attachments, record, storeErr), true
	}
	lines = appendAssistantWorkflowMobileLinkLines(lines, assistantWorkflowVisibleMobileLinkLines(attachments, 6))
	lines = appendVoiceReportAttachmentMarkers(lines, attachments)
	lines = appendAssistantEvidenceNote(lines, record, storeErr)
	return strings.Join(compactBlankLines(lines), "\n"), true
}

func assistantDepartmentWorkflowResultLines(spec assistantDepartmentWorkflowSpec) []string {
	lines := []string{"", lang.T("assistant.workflow.result")}
	if len(spec.Highlights) > 0 && strings.TrimSpace(spec.Highlights[0]) != "" {
		lines = append(lines, "- "+lang.T("assistant.workflow.result.first", spec.Highlights[0]))
	}
	if len(spec.Commands) > 0 && strings.TrimSpace(spec.Commands[0]) != "" {
		lines = append(lines, "- "+lang.T("assistant.workflow.result.next", spec.Commands[0]))
	}
	lines = append(lines,
		"- "+lang.T("assistant.workflow.result.mobile"),
		"- "+lang.T("assistant.workflow.result.boundary"),
	)
	return lines
}

func formatAssistantDepartmentWorkflowSendResult(opts ListenOptions, targetRef string, spec assistantDepartmentWorkflowSpec, summary []string, attachments []string, record evidence.Record, storeErr error) string {
	target, candidates, targetErr := resolveAssistantVoiceTarget(targetRef)
	if targetErr != nil {
		lines := []string{lang.T("assistant.workflow.send_target_failed")}
		lines = append(lines, formatAssistantVoiceTargetCandidates(candidates)...)
		lines = append(lines, "", lang.T("assistant.workflow.attach_here"))
		lines = appendAssistantWorkflowMobileLinkLines(lines, assistantWorkflowVisibleMobileLinkLines(attachments, 6))
		lines = appendVoiceReportAttachmentMarkers(lines, attachments)
		return strings.Join(appendAssistantEvidenceNote(lines, record, storeErr), "\n")
	}
	if !opts.Execute {
		targetLabel := assistantScheduleLocalizedTarget(target.ID, target.Label)
		lines := []string{
			lang.T("assistant.workflow.send_ready"),
			lang.T("assistant.workflow.target", targetLabel),
		}
		if OneWayReportTarget(target) {
			lines = append(lines, lang.T("assistant.workflow.one_way"))
		}
		lines = append(lines, "", lang.T("assistant.workflow.to_send"))
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
		"kind":             "assistant_department_workflow_send",
		"workflow":         spec.Kind,
		"title":            spec.Title,
		"target":           targetRef,
		"resolved_target":  target,
		"attachment_count": len(attachments),
		"mobile_links":     mobileLinkLines,
		"send":             send,
		"send_error":       errorString(sendErr),
		"created_at":       time.Now().UTC(),
	}
	sendRecord, sendStoreErr := evidence.Store("assistant-department-workflow-send", firstNonEmpty(opts.TargetID, "assistant"), target.ID, payload)
	if sendErr != nil {
		targetLabel := assistantScheduleLocalizedTarget(target.ID, target.Label)
		lines := []string{
			lang.T("assistant.workflow.send_failed"),
			lang.T("assistant.workflow.target", targetLabel),
			lang.T("assistant.workflow.problem", sendErr.Error()),
			"",
			lang.T("assistant.workflow.attach_here"),
		}
		lines = appendAssistantWorkflowMobileLinkLines(lines, mobileLinkLines)
		lines = appendVoiceReportAttachmentMarkers(lines, attachments)
		return strings.Join(appendAssistantEvidenceNote(lines, sendRecord, sendStoreErr), "\n")
	}
	targetLabel := assistantScheduleLocalizedTarget(target.ID, target.Label)
	lines := []string{
		lang.T("assistant.workflow.sent"),
		lang.T("assistant.workflow.target", targetLabel),
	}
	if id := strings.TrimSpace(send.Stdout); id != "" {
		lines = append(lines, lang.T("assistant.workflow.signal_id", id))
	}
	return strings.Join(appendAssistantEvidenceNote(lines, sendRecord, sendStoreErr), "\n")
}

func appendAssistantWorkflowMobileLinkLines(lines []string, linkLines []string) []string {
	if len(linkLines) == 0 {
		return lines
	}
	lines = append(lines, "", lang.T("assistant.workflow.mobile_links"))
	return append(lines, linkLines...)
}

func assistantWorkflowVisibleMobileLinkLines(attachments []string, limit int) []string {
	if len(publish.PublicBaseURLs()) == 0 {
		return nil
	}
	return assistantWorkflowMobileLinkLines(attachments, limit)
}

func assistantWorkflowMobileLinkLines(attachments []string, limit int) []string {
	if limit <= 0 {
		limit = 6
	}
	seenLabel := map[string]bool{}
	lines := []string{}
	for _, ext := range []string{".docx", ".pptx", ".xlsx", ".svg", ".mp3", ".m4a", ".wav", ".md", ".csv", ".pdf"} {
		for _, path := range attachments {
			path = strings.TrimSpace(path)
			if path == "" || strings.HasPrefix(path, "data:") || strings.ToLower(filepath.Ext(path)) != ext {
				continue
			}
			label := assistantWorkflowMobileFileLabel(path)
			if label == "" || seenLabel[label] {
				continue
			}
			link := signalReplyDocumentLink(path)
			if link == "" {
				continue
			}
			seenLabel[label] = true
			lines = append(lines, lang.T("assistant.workflow.mobile_link", label, link))
			if len(lines) >= limit {
				return lines
			}
		}
	}
	return lines
}

func assistantWorkflowMobileFileLabel(path string) string {
	switch strings.ToLower(filepath.Ext(strings.TrimSpace(path))) {
	case ".docx":
		return lang.T("assistant.workflow.mobile_file.docx")
	case ".pptx":
		return lang.T("assistant.workflow.mobile_file.pptx")
	case ".xlsx":
		return lang.T("assistant.workflow.mobile_file.xlsx")
	case ".svg":
		return lang.T("assistant.workflow.mobile_file.svg")
	case ".mp3", ".m4a", ".wav":
		return lang.T("assistant.workflow.mobile_file.audio")
	case ".md":
		return lang.T("assistant.workflow.mobile_file.md")
	case ".csv":
		return lang.T("assistant.workflow.mobile_file.csv")
	case ".pdf":
		return lang.T("assistant.workflow.mobile_file.pdf")
	default:
		return ""
	}
}

func writeAssistantDepartmentGraphMarkdown(spec assistantDepartmentWorkflowSpec) (string, error) {
	body := strings.TrimSpace(spec.GraphBody)
	if body == "" {
		return "", nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Documents", "Argos Vault", "Charts")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	base := safeAssistantDepartmentFilename(spec.Title)
	path := filepath.Join(dir, base+"-"+time.Now().Format("20060102-150405")+"-graphs.md")
	content := strings.Join([]string{
		"# " + lang.T("assistant.workflow.graph.title", spec.Title),
		"",
		lang.T("assistant.workflow.graph.intro"),
		"",
		body,
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return "", err
	}
	return path, nil
}

type assistantDepartmentChartPoint struct {
	Label string
	Value float64
	Color string
}

func writeAssistantDepartmentGraphSVG(spec assistantDepartmentWorkflowSpec) (string, error) {
	if strings.TrimSpace(spec.GraphBody) == "" {
		return "", nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(home, "Documents", "Argos Vault", "Charts")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", err
	}
	base := safeAssistantDepartmentFilename(spec.Title)
	path := filepath.Join(dir, base+"-"+time.Now().Format("20060102-150405")+"-chart.svg")
	points, unit, note := assistantDepartmentChartSeries(spec)
	if len(points) == 0 {
		return "", nil
	}
	content := renderAssistantDepartmentChartSVG(spec.Title, points, unit, note)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		return "", err
	}
	return path, nil
}

func assistantDepartmentChartSeries(spec assistantDepartmentWorkflowSpec) ([]assistantDepartmentChartPoint, string, string) {
	switch spec.Kind {
	case "marketing_war_room":
		return []assistantDepartmentChartPoint{
			{Label: lang.T("assistant.workflow.marketing_war_room.chart.market"), Value: 86, Color: "#2563eb"},
			{Label: lang.T("assistant.workflow.marketing_war_room.chart.revenue"), Value: 92, Color: "#0f766e"},
			{Label: lang.T("assistant.workflow.marketing_war_room.chart.roas"), Value: 78, Color: "#d97706"},
			{Label: lang.T("assistant.workflow.marketing_war_room.chart.funnel"), Value: 71, Color: "#7c3aed"},
			{Label: lang.T("assistant.workflow.marketing_war_room.chart.experiment"), Value: 64, Color: "#0891b2"},
		}, lang.T("assistant.workflow.marketing_war_room.chart.unit"), lang.T("assistant.workflow.chart.note.marketing_war_room")
	case "marketing_sales_analysis":
		return []assistantDepartmentChartPoint{
			{Label: lang.T("assistant.workflow.marketing_sales.chart.jan"), Value: 120, Color: "#2563eb"},
			{Label: lang.T("assistant.workflow.marketing_sales.chart.feb"), Value: 138, Color: "#2563eb"},
			{Label: lang.T("assistant.workflow.marketing_sales.chart.mar"), Value: 155, Color: "#0f766e"},
			{Label: lang.T("assistant.workflow.marketing_sales.chart.apr"), Value: 149, Color: "#d97706"},
			{Label: lang.T("assistant.workflow.marketing_sales.chart.may"), Value: 171, Color: "#0f766e"},
			{Label: lang.T("assistant.workflow.marketing_sales.chart.jun"), Value: 188, Color: "#0f766e"},
		}, lang.T("assistant.workflow.marketing_sales.chart.unit"), lang.T("assistant.workflow.chart.note.marketing_sales")
	case "public_agency_budget_graph":
		return []assistantDepartmentChartPoint{
			{Label: lang.T("assistant.workflow.public_budget.chart.q1"), Value: 18, Color: "#2563eb"},
			{Label: lang.T("assistant.workflow.public_budget.chart.q2"), Value: 36, Color: "#d97706"},
			{Label: lang.T("assistant.workflow.public_budget.chart.q3"), Value: 59, Color: "#0f766e"},
			{Label: lang.T("assistant.workflow.public_budget.chart.q4"), Value: 73, Color: "#0f766e"},
		}, lang.T("assistant.workflow.public_budget.chart.unit"), lang.T("assistant.workflow.chart.note.public_budget")
	case "public_agency_civil_service":
		return []assistantDepartmentChartPoint{
			{Label: lang.T("assistant.workflow.public_agency_civil.chart.traffic"), Value: 428, Color: "#2563eb"},
			{Label: lang.T("assistant.workflow.public_agency_civil.chart.welfare"), Value: 216, Color: "#0f766e"},
			{Label: lang.T("assistant.workflow.public_agency_civil.chart.environment"), Value: 132, Color: "#7c3aed"},
			{Label: lang.T("assistant.workflow.public_agency_civil.chart.safety"), Value: 74, Color: "#d97706"},
			{Label: lang.T("assistant.workflow.public_agency_civil.chart.daily_life"), Value: 189, Color: "#0891b2"},
		}, lang.T("assistant.workflow.public_agency_civil.chart.unit"), lang.T("assistant.workflow.chart.note.public_agency_civil")
	case "hr_recruiting":
		return []assistantDepartmentChartPoint{
			{Label: lang.T("assistant.workflow.hr_recruiting.chart.applied"), Value: 312, Color: "#2563eb"},
			{Label: lang.T("assistant.workflow.hr_recruiting.chart.screened"), Value: 78, Color: "#0f766e"},
			{Label: lang.T("assistant.workflow.hr_recruiting.chart.first"), Value: 41, Color: "#7c3aed"},
			{Label: lang.T("assistant.workflow.hr_recruiting.chart.final"), Value: 18, Color: "#d97706"},
			{Label: lang.T("assistant.workflow.hr_recruiting.chart.offer"), Value: 9, Color: "#dc2626"},
			{Label: lang.T("assistant.workflow.hr_recruiting.chart.hired"), Value: 6, Color: "#0891b2"},
		}, lang.T("assistant.workflow.hr_recruiting.chart.unit"), lang.T("assistant.workflow.chart.note.hr_recruiting")
	case "chemical_margin_risk":
		return []assistantDepartmentChartPoint{
			{Label: lang.T("assistant.workflow.chemical_margin.chart.naphtha"), Value: 87, Color: "#dc2626"},
			{Label: lang.T("assistant.workflow.chemical_margin.chart.spread"), Value: 72, Color: "#d97706"},
			{Label: lang.T("assistant.workflow.chemical_margin.chart.pfas"), Value: 64, Color: "#7c3aed"},
			{Label: lang.T("assistant.workflow.chemical_margin.chart.logistics"), Value: 51, Color: "#0891b2"},
			{Label: lang.T("assistant.workflow.chemical_margin.chart.pass_through"), Value: 46, Color: "#0f766e"},
		}, lang.T("assistant.workflow.chemical_margin.chart.unit"), lang.T("assistant.workflow.chart.note.chemical_margin")
	case "manufacturing_quality_incident":
		return []assistantDepartmentChartPoint{
			{Label: lang.T("assistant.workflow.manufacturing_quality.chart.defect"), Value: 91, Color: "#dc2626"},
			{Label: lang.T("assistant.workflow.manufacturing_quality.chart.rework"), Value: 67, Color: "#d97706"},
			{Label: lang.T("assistant.workflow.manufacturing_quality.chart.root"), Value: 54, Color: "#7c3aed"},
			{Label: lang.T("assistant.workflow.manufacturing_quality.chart.capa"), Value: 42, Color: "#0f766e"},
			{Label: lang.T("assistant.workflow.manufacturing_quality.chart.customer"), Value: 18, Color: "#2563eb"},
		}, lang.T("assistant.workflow.manufacturing_quality.chart.unit"), lang.T("assistant.workflow.chart.note.manufacturing_quality")
	case "healthcare_clinic_ops":
		return []assistantDepartmentChartPoint{
			{Label: lang.T("assistant.workflow.healthcare_clinic.chart.appointment"), Value: 142, Color: "#2563eb"},
			{Label: lang.T("assistant.workflow.healthcare_clinic.chart.wait"), Value: 37, Color: "#d97706"},
			{Label: lang.T("assistant.workflow.healthcare_clinic.chart.claim"), Value: 29, Color: "#7c3aed"},
			{Label: lang.T("assistant.workflow.healthcare_clinic.chart.result"), Value: 24, Color: "#0f766e"},
			{Label: lang.T("assistant.workflow.healthcare_clinic.chart.notice"), Value: 16, Color: "#0891b2"},
		}, lang.T("assistant.workflow.healthcare_clinic.chart.unit"), lang.T("assistant.workflow.chart.note.healthcare_clinic")
	case "logistics_delivery_ops":
		return []assistantDepartmentChartPoint{
			{Label: lang.T("assistant.workflow.logistics_delivery.chart.delay"), Value: 128, Color: "#dc2626"},
			{Label: lang.T("assistant.workflow.logistics_delivery.chart.picking"), Value: 46, Color: "#d97706"},
			{Label: lang.T("assistant.workflow.logistics_delivery.chart.stockout"), Value: 33, Color: "#7c3aed"},
			{Label: lang.T("assistant.workflow.logistics_delivery.chart.claim"), Value: 27, Color: "#2563eb"},
			{Label: lang.T("assistant.workflow.logistics_delivery.chart.sla"), Value: 91, Color: "#0f766e"},
		}, lang.T("assistant.workflow.logistics_delivery.chart.unit"), lang.T("assistant.workflow.chart.note.logistics_delivery")
	case "retail_franchise_ops":
		return []assistantDepartmentChartPoint{
			{Label: lang.T("assistant.workflow.retail_franchise.chart.sales"), Value: 118, Color: "#2563eb"},
			{Label: lang.T("assistant.workflow.retail_franchise.chart.stockout"), Value: 34, Color: "#dc2626"},
			{Label: lang.T("assistant.workflow.retail_franchise.chart.review"), Value: 27, Color: "#d97706"},
			{Label: lang.T("assistant.workflow.retail_franchise.chart.ad"), Value: 76, Color: "#7c3aed"},
			{Label: lang.T("assistant.workflow.retail_franchise.chart.order"), Value: 92, Color: "#0f766e"},
		}, lang.T("assistant.workflow.retail_franchise.chart.unit"), lang.T("assistant.workflow.chart.note.retail_franchise")
	case "customer_success_retention":
		return []assistantDepartmentChartPoint{
			{Label: lang.T("assistant.workflow.customer_success.chart.risk"), Value: 38, Color: "#dc2626"},
			{Label: lang.T("assistant.workflow.customer_success.chart.usage"), Value: 62, Color: "#d97706"},
			{Label: lang.T("assistant.workflow.customer_success.chart.renewal"), Value: 21, Color: "#2563eb"},
			{Label: lang.T("assistant.workflow.customer_success.chart.expansion"), Value: 14, Color: "#0f766e"},
			{Label: lang.T("assistant.workflow.customer_success.chart.ticket"), Value: 9, Color: "#7c3aed"},
		}, lang.T("assistant.workflow.customer_success.chart.unit"), lang.T("assistant.workflow.chart.note.customer_success")
	case "education_academy_ops":
		return []assistantDepartmentChartPoint{
			{Label: lang.T("assistant.workflow.education_academy.chart.lead"), Value: 184, Color: "#2563eb"},
			{Label: lang.T("assistant.workflow.education_academy.chart.trial"), Value: 62, Color: "#0f766e"},
			{Label: lang.T("assistant.workflow.education_academy.chart.enroll"), Value: 31, Color: "#7c3aed"},
			{Label: lang.T("assistant.workflow.education_academy.chart.absent"), Value: 17, Color: "#d97706"},
			{Label: lang.T("assistant.workflow.education_academy.chart.unpaid"), Value: 12, Color: "#dc2626"},
		}, lang.T("assistant.workflow.education_academy.chart.unit"), lang.T("assistant.workflow.chart.note.education_academy")
	case "shopping_decision_workbook":
		return []assistantDepartmentChartPoint{
			{Label: lang.T("assistant.workflow.shopping_decision.chart.price"), Value: 82, Color: "#2563eb"},
			{Label: lang.T("assistant.workflow.shopping_decision.chart.delivery"), Value: 91, Color: "#0f766e"},
			{Label: lang.T("assistant.workflow.shopping_decision.chart.review"), Value: 74, Color: "#7c3aed"},
			{Label: lang.T("assistant.workflow.shopping_decision.chart.return"), Value: 68, Color: "#d97706"},
			{Label: lang.T("assistant.workflow.shopping_decision.chart.approval"), Value: 100, Color: "#dc2626"},
		}, lang.T("assistant.workflow.shopping_decision.chart.unit"), lang.T("assistant.workflow.chart.note.shopping_decision")
	case "booking_candidate_package":
		return []assistantDepartmentChartPoint{
			{Label: lang.T("assistant.workflow.booking_candidate.chart.location"), Value: 88, Color: "#2563eb"},
			{Label: lang.T("assistant.workflow.booking_candidate.chart.time"), Value: 82, Color: "#0f766e"},
			{Label: lang.T("assistant.workflow.booking_candidate.chart.total"), Value: 74, Color: "#d97706"},
			{Label: lang.T("assistant.workflow.booking_candidate.chart.cancel"), Value: 69, Color: "#7c3aed"},
			{Label: lang.T("assistant.workflow.booking_candidate.chart.contact"), Value: 61, Color: "#0891b2"},
		}, lang.T("assistant.workflow.booking_candidate.chart.unit"), lang.T("assistant.workflow.chart.note.booking_candidate")
	case "finance_expense_reconciliation":
		return []assistantDepartmentChartPoint{
			{Label: lang.T("assistant.workflow.finance_expense.chart.transport"), Value: 148, Color: "#2563eb"},
			{Label: lang.T("assistant.workflow.finance_expense.chart.meals"), Value: 92, Color: "#0f766e"},
			{Label: lang.T("assistant.workflow.finance_expense.chart.subscription"), Value: 76, Color: "#7c3aed"},
			{Label: lang.T("assistant.workflow.finance_expense.chart.entertainment"), Value: 61, Color: "#d97706"},
			{Label: lang.T("assistant.workflow.finance_expense.chart.missing"), Value: 23, Color: "#dc2626"},
		}, lang.T("assistant.workflow.finance_expense.chart.unit"), lang.T("assistant.workflow.chart.note.finance_expense")
	case "finance_ar_cash_collection":
		return []assistantDepartmentChartPoint{
			{Label: lang.T("assistant.workflow.finance_ar.chart.overdue"), Value: 184, Color: "#dc2626"},
			{Label: lang.T("assistant.workflow.finance_ar.chart.due"), Value: 96, Color: "#d97706"},
			{Label: lang.T("assistant.workflow.finance_ar.chart.promised"), Value: 72, Color: "#2563eb"},
			{Label: lang.T("assistant.workflow.finance_ar.chart.dispute"), Value: 38, Color: "#7c3aed"},
			{Label: lang.T("assistant.workflow.finance_ar.chart.cash"), Value: 214, Color: "#0f766e"},
		}, lang.T("assistant.workflow.finance_ar.chart.unit"), lang.T("assistant.workflow.chart.note.finance_ar")
	case "procurement_vendor_selection":
		return []assistantDepartmentChartPoint{
			{Label: lang.T("assistant.workflow.procurement.chart.price"), Value: 82, Color: "#2563eb"},
			{Label: lang.T("assistant.workflow.procurement.chart.tco"), Value: 74, Color: "#0f766e"},
			{Label: lang.T("assistant.workflow.procurement.chart.delivery"), Value: 68, Color: "#d97706"},
			{Label: lang.T("assistant.workflow.procurement.chart.risk"), Value: 59, Color: "#dc2626"},
			{Label: lang.T("assistant.workflow.procurement.chart.approval"), Value: 91, Color: "#7c3aed"},
		}, lang.T("assistant.workflow.procurement.chart.unit"), lang.T("assistant.workflow.chart.note.procurement")
	case "communications_crisis_response":
		return []assistantDepartmentChartPoint{
			{Label: lang.T("assistant.workflow.comms_crisis.chart.media"), Value: 73, Color: "#dc2626"},
			{Label: lang.T("assistant.workflow.comms_crisis.chart.social"), Value: 88, Color: "#d97706"},
			{Label: lang.T("assistant.workflow.comms_crisis.chart.customer"), Value: 64, Color: "#2563eb"},
			{Label: lang.T("assistant.workflow.comms_crisis.chart.employee"), Value: 52, Color: "#0f766e"},
			{Label: lang.T("assistant.workflow.comms_crisis.chart.approval"), Value: 91, Color: "#7c3aed"},
		}, lang.T("assistant.workflow.comms_crisis.chart.unit"), lang.T("assistant.workflow.chart.note.comms_crisis")
	case "it_access_offboarding":
		return []assistantDepartmentChartPoint{
			{Label: lang.T("assistant.workflow.it_offboarding.chart.accounts"), Value: 27, Color: "#dc2626"},
			{Label: lang.T("assistant.workflow.it_offboarding.chart.devices"), Value: 8, Color: "#d97706"},
			{Label: lang.T("assistant.workflow.it_offboarding.chart.licenses"), Value: 14, Color: "#2563eb"},
			{Label: lang.T("assistant.workflow.it_offboarding.chart.data"), Value: 11, Color: "#7c3aed"},
			{Label: lang.T("assistant.workflow.it_offboarding.chart.approval"), Value: 91, Color: "#0f766e"},
		}, lang.T("assistant.workflow.it_offboarding.chart.unit"), lang.T("assistant.workflow.chart.note.it_offboarding")
	case "insurance_claims_triage":
		return []assistantDepartmentChartPoint{
			{Label: lang.T("assistant.workflow.insurance_claims.chart.intake"), Value: 128, Color: "#2563eb"},
			{Label: lang.T("assistant.workflow.insurance_claims.chart.missing"), Value: 37, Color: "#d97706"},
			{Label: lang.T("assistant.workflow.insurance_claims.chart.risk"), Value: 12, Color: "#dc2626"},
			{Label: lang.T("assistant.workflow.insurance_claims.chart.payout"), Value: 64, Color: "#0f766e"},
			{Label: lang.T("assistant.workflow.insurance_claims.chart.customer"), Value: 91, Color: "#7c3aed"},
		}, lang.T("assistant.workflow.insurance_claims.chart.unit"), lang.T("assistant.workflow.chart.note.insurance_claims")
	case "construction_site_control":
		return []assistantDepartmentChartPoint{
			{Label: lang.T("assistant.workflow.construction_site.chart.schedule"), Value: 74, Color: "#d97706"},
			{Label: lang.T("assistant.workflow.construction_site.chart.safety"), Value: 18, Color: "#dc2626"},
			{Label: lang.T("assistant.workflow.construction_site.chart.material"), Value: 11, Color: "#2563eb"},
			{Label: lang.T("assistant.workflow.construction_site.chart.subcontractor"), Value: 7, Color: "#7c3aed"},
			{Label: lang.T("assistant.workflow.construction_site.chart.approval"), Value: 91, Color: "#0f766e"},
		}, lang.T("assistant.workflow.construction_site.chart.unit"), lang.T("assistant.workflow.chart.note.construction_site")
	case "legal_dispute_case_prep":
		return []assistantDepartmentChartPoint{
			{Label: lang.T("assistant.workflow.legal_dispute.chart.issue"), Value: 9, Color: "#dc2626"},
			{Label: lang.T("assistant.workflow.legal_dispute.chart.evidence"), Value: 42, Color: "#2563eb"},
			{Label: lang.T("assistant.workflow.legal_dispute.chart.deadline"), Value: 6, Color: "#d97706"},
			{Label: lang.T("assistant.workflow.legal_dispute.chart.notice"), Value: 3, Color: "#7c3aed"},
			{Label: lang.T("assistant.workflow.legal_dispute.chart.approval"), Value: 91, Color: "#0f766e"},
		}, lang.T("assistant.workflow.legal_dispute.chart.unit"), lang.T("assistant.workflow.chart.note.legal_dispute")
	case "executive_decision_pack":
		return []assistantDepartmentChartPoint{
			{Label: lang.T("assistant.workflow.executive_decision.chart.decision"), Value: 4, Color: "#2563eb"},
			{Label: lang.T("assistant.workflow.executive_decision.chart.risk"), Value: 7, Color: "#dc2626"},
			{Label: lang.T("assistant.workflow.executive_decision.chart.action"), Value: 12, Color: "#0f766e"},
			{Label: lang.T("assistant.workflow.executive_decision.chart.owner"), Value: 6, Color: "#7c3aed"},
			{Label: lang.T("assistant.workflow.executive_decision.chart.approval"), Value: 91, Color: "#d97706"},
		}, lang.T("assistant.workflow.executive_decision.chart.unit"), lang.T("assistant.workflow.chart.note.executive_decision")
	case "sales_rfp_response":
		return []assistantDepartmentChartPoint{
			{Label: lang.T("assistant.workflow.sales_rfp.chart.feature"), Value: 92, Color: "#2563eb"},
			{Label: lang.T("assistant.workflow.sales_rfp.chart.security"), Value: 84, Color: "#0f766e"},
			{Label: lang.T("assistant.workflow.sales_rfp.chart.implementation"), Value: 78, Color: "#0891b2"},
			{Label: lang.T("assistant.workflow.sales_rfp.chart.price"), Value: 71, Color: "#d97706"},
			{Label: lang.T("assistant.workflow.sales_rfp.chart.reference"), Value: 66, Color: "#7c3aed"},
		}, lang.T("assistant.workflow.sales_rfp.chart.unit"), lang.T("assistant.workflow.chart.note.sales_rfp")
	case "support_ticket_response":
		return []assistantDepartmentChartPoint{
			{Label: lang.T("assistant.workflow.support_ticket.chart.urgent"), Value: 6, Color: "#dc2626"},
			{Label: lang.T("assistant.workflow.support_ticket.chart.refund"), Value: 9, Color: "#d97706"},
			{Label: lang.T("assistant.workflow.support_ticket.chart.feature"), Value: 11, Color: "#2563eb"},
			{Label: lang.T("assistant.workflow.support_ticket.chart.howto"), Value: 12, Color: "#0f766e"},
			{Label: lang.T("assistant.workflow.support_ticket.chart.feedback"), Value: 4, Color: "#7c3aed"},
		}, lang.T("assistant.workflow.support_ticket.chart.unit"), lang.T("assistant.workflow.chart.note.support_ticket")
	case "product_prd_release":
		return []assistantDepartmentChartPoint{
			{Label: lang.T("assistant.workflow.product_prd.chart.problem"), Value: 91, Color: "#2563eb"},
			{Label: lang.T("assistant.workflow.product_prd.chart.requirement"), Value: 86, Color: "#0f766e"},
			{Label: lang.T("assistant.workflow.product_prd.chart.backlog"), Value: 78, Color: "#0891b2"},
			{Label: lang.T("assistant.workflow.product_prd.chart.experiment"), Value: 72, Color: "#d97706"},
			{Label: lang.T("assistant.workflow.product_prd.chart.release"), Value: 64, Color: "#7c3aed"},
		}, lang.T("assistant.workflow.product_prd.chart.unit"), lang.T("assistant.workflow.chart.note.product_prd")
	case "legal_contract_review":
		return []assistantDepartmentChartPoint{
			{Label: lang.T("assistant.workflow.legal_contract.chart.liability"), Value: 88, Color: "#dc2626"},
			{Label: lang.T("assistant.workflow.legal_contract.chart.privacy"), Value: 82, Color: "#7c3aed"},
			{Label: lang.T("assistant.workflow.legal_contract.chart.termination"), Value: 73, Color: "#d97706"},
			{Label: lang.T("assistant.workflow.legal_contract.chart.sla"), Value: 69, Color: "#2563eb"},
			{Label: lang.T("assistant.workflow.legal_contract.chart.renewal"), Value: 58, Color: "#0f766e"},
		}, lang.T("assistant.workflow.legal_contract.chart.unit"), lang.T("assistant.workflow.chart.note.legal_contract")
	case "onboarding_training":
		return []assistantDepartmentChartPoint{
			{Label: lang.T("assistant.workflow.onboarding_training.chart.day1"), Value: 92, Color: "#2563eb"},
			{Label: lang.T("assistant.workflow.onboarding_training.chart.week1"), Value: 84, Color: "#0f766e"},
			{Label: lang.T("assistant.workflow.onboarding_training.chart.day30"), Value: 72, Color: "#0891b2"},
			{Label: lang.T("assistant.workflow.onboarding_training.chart.day60"), Value: 64, Color: "#d97706"},
			{Label: lang.T("assistant.workflow.onboarding_training.chart.day90"), Value: 58, Color: "#7c3aed"},
		}, lang.T("assistant.workflow.onboarding_training.chart.unit"), lang.T("assistant.workflow.chart.note.onboarding_training")
	default:
		points := []assistantDepartmentChartPoint{}
		colors := []string{"#2563eb", "#0f766e", "#7c3aed", "#d97706", "#dc2626"}
		for i, item := range spec.Highlights {
			if i >= 5 {
				break
			}
			label := fmt.Sprintf("%d", i+1)
			if strings.TrimSpace(item) != "" {
				label = lang.T("assistant.workflow.chart.generic_label", i+1)
			}
			points = append(points, assistantDepartmentChartPoint{Label: label, Value: float64(40 + i*12), Color: colors[i%len(colors)]})
		}
		return points, lang.T("assistant.workflow.chart.unit.score"), lang.T("assistant.workflow.chart.note.generic")
	}
}

func renderAssistantDepartmentChartSVG(title string, points []assistantDepartmentChartPoint, unit, note string) string {
	max := 1.0
	for _, point := range points {
		if point.Value > max {
			max = point.Value
		}
	}
	width := 720.0
	height := 460.0
	left := 72.0
	top := 94.0
	chartW := 560.0
	chartH := 250.0
	gap := 16.0
	barW := (chartW - gap*float64(len(points)-1)) / float64(len(points))
	if barW > 72 {
		barW = 72
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%.0f" height="%.0f" viewBox="0 0 %.0f %.0f">`, width, height, width, height))
	b.WriteString("\n")
	b.WriteString(`<rect width="720" height="460" fill="#f8fafc"/>` + "\n")
	b.WriteString(`<rect x="24" y="24" width="672" height="412" rx="18" fill="#ffffff" stroke="#d9e2ef"/>` + "\n")
	b.WriteString(fmt.Sprintf(`<text x="48" y="62" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="24" font-weight="800" fill="#0f172a">%s</text>`, html.EscapeString(title)))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`<text x="48" y="88" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="14" fill="#64748b">%s</text>`, html.EscapeString(note)))
	b.WriteString("\n")
	b.WriteString(fmt.Sprintf(`<line x1="%.0f" y1="%.0f" x2="%.0f" y2="%.0f" stroke="#94a3b8" stroke-width="2"/>`, left, top+chartH, left+chartW, top+chartH))
	b.WriteString("\n")
	for i, point := range points {
		x := left + float64(i)*(barW+gap)
		barH := chartH * (point.Value / max)
		y := top + chartH - barH
		color := firstNonEmpty(strings.TrimSpace(point.Color), "#2563eb")
		b.WriteString(fmt.Sprintf(`<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="8" fill="%s"/>`, x, y, barW, barH, html.EscapeString(color)))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="middle" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="15" font-weight="750" fill="#0f172a">%.0f%s</text>`, x+barW/2, y-10, point.Value, html.EscapeString(unit)))
		b.WriteString("\n")
		b.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" text-anchor="middle" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="14" fill="#334155">%s</text>`, x+barW/2, top+chartH+28, html.EscapeString(point.Label)))
		b.WriteString("\n")
	}
	b.WriteString(fmt.Sprintf(`<text x="48" y="398" font-family="-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif" font-size="15" fill="#475569">%s</text>`, html.EscapeString(lang.T("assistant.workflow.chart.footer"))))
	b.WriteString("\n")
	b.WriteString("</svg>\n")
	return b.String()
}

func safeAssistantDepartmentFilename(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "argos-workflow"
	}
	runes := []rune(out)
	if len(runes) > 60 {
		out = strings.Trim(string(runes[:60]), "-")
	}
	return out
}

func assistantDepartmentWorkflowVoiceScript(spec assistantDepartmentWorkflowSpec) string {
	lines := []string{
		lang.T("assistant.workflow.voice.intro"),
		lang.T("assistant.workflow.voice.title", spec.Title),
	}
	if strings.TrimSpace(spec.Summary) != "" {
		lines = append(lines, spec.Summary)
	}
	if len(spec.Highlights) > 0 {
		lines = append(lines, lang.T("assistant.workflow.voice.highlights"))
		for i, item := range spec.Highlights {
			if i >= 4 {
				break
			}
			lines = append(lines, lang.T("assistant.workflow.voice.item", i+1, item))
		}
	}
	lines = append(lines, lang.T("assistant.workflow.voice.closing"))
	return strings.Join(lines, "\n\n")
}

func assistantDepartmentWorkflowSpecFor(request string) (assistantDepartmentWorkflowSpec, bool) {
	lower := strings.ToLower(strings.TrimSpace(request))
	if lower == "" {
		return assistantDepartmentWorkflowSpec{}, false
	}
	if !containsAny(lower, "만들", "작성", "정리", "분석", "준비", "요약", "줄여", "아젠다", "질문", "액션", "짜줘", "도와", "보고서", "표", "ppt", "pptx", "그래프", "비교", "바꿔", "수정", "다시", "써줘", "make", "create", "draft", "analyze", "summarize") {
		return assistantDepartmentWorkflowSpec{}, false
	}
	switch {
	case containsAny(lower, "홍보팀", "홍보 팀", "pr팀", "pr 팀", "커뮤니케이션팀", "커뮤니케이션 팀", "언론 대응", "언론대응", "위기 대응", "위기대응", "보도자료", "입장문", "사과문", "브리핑 문답", "q&a", "faq", "communications team", "pr crisis", "crisis communications", "press response", "press statement", "media response", "spokesperson q&a") &&
		containsAny(lower, "표", "보고서", "ppt", "pptx", "문서", "그래프", "음성", "패키지", "보도자료", "입장문", "사과문", "faq", "문답", "메시지", "타임라인", "리스크", "작성", "정리", "분석", "만들", "package", "statement", "briefing", "message", "timeline", "risk"):
		return assistantCommunicationsCrisisWorkflowSpec(), true
	case containsAny(lower, "it팀", "it 팀", "퇴사자", "오프보딩", "계정 회수", "계정회수", "권한 회수", "권한회수", "saas 라이선스", "라이선스 정리", "기기 반납", "데이터 이관", "접근권한", "접근 권한", "it offboarding", "employee offboarding", "access review", "access removal", "saas license cleanup", "device return", "data handoff", "account deactivation prep") &&
		containsAny(lower, "표", "보고서", "ppt", "pptx", "문서", "그래프", "음성", "패키지", "체크리스트", "승인", "정리", "만들", "작성", "package", "checklist", "approval", "graph"):
		return assistantITOffboardingWorkflowSpec(), true
	case containsAny(lower, "보험사", "보상팀", "보상 팀", "손해사정", "손해 사정", "보험금 청구", "보험금청구", "실손 청구", "실손청구", "청구 심사", "지급 심사", "보험 지급", "insurance claims", "claims triage", "claim triage", "claim adjustment", "claims adjustment", "claims operations", "payout review") &&
		containsAny(lower, "표", "보고서", "ppt", "pptx", "문서", "그래프", "음성", "패키지", "체크리스트", "서류", "누락", "이상 징후", "이상징후", "지급", "고객 안내", "고객안내", "만들", "작성", "정리", "분석", "package", "checklist", "missing", "risk", "payout", "customer"):
		return assistantInsuranceClaimsWorkflowSpec(), true
	case containsAny(lower, "건설사", "건설 현장", "건설현장", "현장소장", "현장 소장", "공사 현장", "공사현장", "공정 지연", "공정지연", "안전 점검", "안전점검", "자재 반입", "자재반입", "협력사", "시공", "construction site", "construction ops", "site manager", "site safety", "schedule delay", "material delivery", "subcontractor") &&
		containsAny(lower, "표", "보고서", "ppt", "pptx", "문서", "그래프", "음성", "패키지", "체크리스트", "공정", "안전", "자재", "협력사", "리스크", "만들", "작성", "정리", "분석", "package", "checklist", "schedule", "safety", "material", "risk"):
		return assistantConstructionSiteWorkflowSpec(), true
	case containsAny(lower, "소송", "분쟁", "내용증명", "내용 증명", "증거 타임라인", "증거목록", "증거 목록", "기일", "변론", "답변서", "준비서면", "합의 옵션", "합의안", "법무팀 분쟁", "litigation", "legal dispute", "dispute response", "demand letter", "evidence timeline", "case deadline", "settlement option", "court filing") &&
		containsAny(lower, "표", "보고서", "ppt", "pptx", "문서", "그래프", "음성", "패키지", "체크리스트", "쟁점", "증거", "기일", "내용증명", "합의", "만들", "작성", "정리", "분석", "package", "checklist", "issue", "evidence", "deadline", "settlement"):
		return assistantLegalDisputeWorkflowSpec(), true
	case containsAny(lower, "경영회의", "경영 회의", "임원회의", "임원 회의", "대표 보고", "의사결정", "의사 결정", "결정안", "선택지", "전략회의", "전략 회의", "leadership meeting", "executive decision", "executive package", "decision memo", "option analysis", "board decision") &&
		containsAny(lower, "표", "보고서", "ppt", "pptx", "문서", "그래프", "음성", "패키지", "리스크", "액션", "담당", "예산", "가격", "제품 출시", "인력", "만들", "작성", "정리", "분석", "package", "risk", "action", "owner", "budget", "pricing", "launch", "staffing"):
		return assistantExecutiveDecisionWorkflowSpec(), true
	case containsAny(lower, "검색광고", "검색 광고", "search ad", "search ads") && containsAny(lower, "카피", "문구", "copy"):
		return assistantAdvertisingCopyWorkflowSpec(), true
	case containsAny(lower, "마케팅 워룸", "마케팅 작전실", "cmo 보고", "cmo", "marketing war room", "growth war room") &&
		containsAny(lower, "시장조사", "시장 조사", "매출", "광고", "캠페인", "roas", "경쟁사", "퍼널", "그래프", "보고방", "음성", "ppt", "pptx", "insight", "campaign", "revenue", "competitor", "funnel", "package"):
		return assistantMarketingWarRoomWorkflowSpec(), true
	case containsAny(lower, "kpi", "실험 일정", "실험일정", "테스트 일정", "다음 주 실험", "다음주 실험"):
		return assistantAdvertisingExperimentScheduleSpec(), true
	case containsAny(lower, "20대 여성", "여성 타깃", "여성 타겟", "타깃으로 바", "타겟으로 바"):
		return assistantAdvertisingYoungWomenTargetSpec(), true
	case containsAny(lower, "광고기획", "광고 기획", "캠페인", "카피", "a/b", "ab 테스트", "채널 믹스", "channel mix"):
		return assistantAdvertisingWorkflowSpec(), true
	case containsAny(lower, "시니어 백엔드", "시니어백엔드", "senior backend", "백엔드 엔지니어용", "백엔드 개발자용"):
		return assistantHRSeniorBackendJDWorkflowSpec(), true
	case containsAny(lower, "후보자 5명", "후보자 다섯", "이력서", "resume", "cv") && containsAny(lower, "비교", "스크리닝", "평가", "표"):
		return assistantHRCandidateComparisonWorkflowSpec(), true
	case containsAny(lower, "압박 없는", "압박없는", "부드러운", "말투") && containsAny(lower, "면접", "질문", "interview"):
		return assistantHRGentleInterviewWorkflowSpec(), true
	case containsAny(lower, "온보딩", "신입 교육", "신입교육", "신규 입사자", "신입사원", "입사자 교육", "교육 운영", "교육자료", "30/60/90", "30일", "60일", "90일", "onboarding", "new hire", "training operations", "training package") &&
		containsAny(lower, "표", "보고서", "ppt", "pptx", "문서", "퀴즈", "체크리스트", "일정표", "안내 메일", "메일", "그래프", "음성", "패키지", "만들", "작성", "정리", "준비", "schedule", "quiz", "checklist", "draft", "package") &&
		!containsAny(lower, "벤더", "계약", "계약서", "dpa", "vendor", "contract"):
		return assistantOnboardingTrainingWorkflowSpec(), true
	case containsAny(lower, "인사팀", "인사", "채용", "구인", "구직", "지원자", "후보자", "면접", "jd", "recruit"):
		return assistantHRWorkflowSpec(), true
	case containsAny(lower, "구청장", "시장 보고", "간부 보고", "한 페이지", "1페이지") && containsAny(lower, "민원", "구청", "시청", "공공기관", "관공서", "시민"):
		return assistantPublicAgencyExecutiveBriefSpec(), true
	case containsAny(lower, "복지 문의", "복지문의", "복지") && containsAny(lower, "시민 안내문", "안내문", "다시 써", "작성"):
		return assistantPublicAgencyWelfareNoticeSpec(), true
	case containsAny(lower, "예산 집행표", "예산집행표", "예산 집행", "집행표") && containsAny(lower, "분기별", "분기", "그래프", "차트"):
		return assistantPublicAgencyBudgetGraphSpec(), true
	case containsAny(lower, "관공서", "정부 부처", "정부부처", "공공기관", "구청", "시청", "민원", "정책 브리프", "보도자료", "예산 집행"):
		return assistantPublicAgencyWorkflowSpec(), true
	case containsAny(lower, "화학회사", "화학 회사", "화학업계", "화학 업계", "케미칼", "chemical", "petrochemical", "원자재", "나프타", "납사", "feedstock", "pfas", "reach") &&
		containsAny(lower, "마진", "원가", "규제", "리스크", "회의", "보고", "분석", "ppt", "pptx", "그래프") &&
		!containsAny(lower, "뉴스", "최근", "찾아", "검색", "search", "news"):
		return assistantChemicalMarginWorkflowSpec(), true
	case containsAny(lower, "제조", "공장", "생산", "품질팀", "품질 팀", "품질관리", "품질 관리", "불량", "불량률", "고객 클레임", "고객클레임", "8d", "capa", "파레토", "pareto", "manufacturing", "quality", "defect", "nonconformance", "corrective action") &&
		containsAny(lower, "8d", "capa", "파레토", "불량", "불량률", "클레임", "원인", "재발방지", "시정조치", "보고서", "ppt", "pptx", "표", "그래프", "음성", "패키지", "만들", "작성", "정리", "분석", "pareto", "root cause", "corrective", "action", "report", "package"):
		return assistantManufacturingQualityWorkflowSpec(), true
	case containsAny(lower, "병원 원무", "원무", "클리닉 운영", "의원 운영", "병원 운영", "진료 예약표", "예약표", "대기시간", "대기 시간", "보험청구", "보험 청구", "검사 안내", "검사결과 안내", "검사 결과 안내", "환자 안내", "환자 공지", "healthcare ops", "clinic ops", "clinic operations", "patient notice", "insurance claim") &&
		containsAny(lower, "표", "보고서", "ppt", "pptx", "문서", "예약", "청구", "검사", "안내문", "공지", "대기", "그래프", "음성", "패키지", "만들", "작성", "정리", "분석", "schedule", "notice", "claim", "package"):
		return assistantHealthcareClinicOpsWorkflowSpec(), true
	case containsAny(lower, "리테일", "유통 본부", "유통본부", "프랜차이즈", "가맹점", "점주", "매장 운영", "매장운영", "편의점", "카페 프랜차이즈", "retail ops", "retail operations", "franchise ops", "store operations", "store sales") &&
		containsAny(lower, "매출", "재고", "품절", "리뷰", "로컬 광고", "지역 광고", "점주 공지", "프로모션", "발주", "행사", "그래프", "ppt", "pptx", "보고서", "패키지", "dashboard", "sales", "stockout", "review", "local ad", "promotion", "replenishment", "package"):
		return assistantRetailFranchiseWorkflowSpec(), true
	case containsAny(lower, "고객성공", "고객 성공", "csm", "해지 위험", "해지위험", "이탈 위험", "이탈위험", "고객 이탈", "재계약", "갱신", "사용량 감소", "사용량감소", "업셀", "업셀링", "customer success", "customer retention", "churn", "renewal", "at-risk account", "at risk account", "usage drop", "expansion pipeline") &&
		containsAny(lower, "표", "보고서", "ppt", "pptx", "문서", "그래프", "음성", "패키지", "리스크", "액션", "플랜", "메일", "만들", "작성", "정리", "분석", "dashboard", "package", "plan", "brief"):
		return assistantCustomerSuccessRetentionWorkflowSpec(), true
	case containsAny(lower, "물류 운영", "물류팀", "물류 팀", "배송 지연", "배송지연", "출고 지연", "출고지연", "창고 운영", "창고", "피킹 오류", "피킹오류", "재고 부족", "재고부족", "반품 처리", "배송 클레임", "배송클레임", "운송장", "logistics ops", "warehouse ops", "delivery delay", "picking error", "stockout", "shipment sla") &&
		containsAny(lower, "표", "보고서", "ppt", "pptx", "문서", "배송", "출고", "재고", "클레임", "반품", "sla", "그래프", "음성", "패키지", "만들", "작성", "정리", "분석", "dashboard", "package"):
		return assistantLogisticsDeliveryWorkflowSpec(), true
	case containsAny(lower, "학원 운영", "학원장", "교육기관", "교육 기관", "수강생 모집", "수강생", "체험수업", "체험 수업", "출결", "수강료", "미납", "재등록", "학부모 공지", "학부모 안내", "커리큘럼", "academy ops", "tutoring center", "education ops", "enrollment funnel", "trial class", "attendance", "tuition", "parent notice", "curriculum schedule", "re-enrollment") &&
		containsAny(lower, "표", "보고서", "ppt", "pptx", "문서", "상담", "체험", "등록", "출결", "미납", "재등록", "공지", "커리큘럼", "그래프", "음성", "패키지", "만들", "작성", "정리", "분석", "dashboard", "package"):
		return assistantEducationAcademyWorkflowSpec(), true
	case containsAny(lower, "예약 후보", "예약후보", "예약 가능", "예약가능", "예약 준비", "예약 문의", "식당 예약", "병원 예약", "클리닉 예약", "호텔 예약", "booking candidate", "booking candidates", "restaurant booking", "appointment") &&
		containsAny(lower, "표", "ppt", "pptx", "문서", "보고서", "체크리스트", "전화문구", "전화 문구", "메시지", "보고방", "음성", "패키지"):
		return assistantBookingCandidateWorkflowSpec(), true
	case containsAny(lower, "미수금", "매출채권", "수금", "연체 인보이스", "연체 청구서", "입금 예정", "현금흐름", "현금 흐름", "청구서 회수", "인보이스 회수", "accounts receivable", "ar aging", "cash collection", "cash forecast", "overdue invoice", "invoice collection", "dso") &&
		containsAny(lower, "표", "보고서", "ppt", "pptx", "문서", "엑셀", "xlsx", "csv", "그래프", "음성", "패키지", "정리", "분석", "만들", "작성", "메일", "연락", "우선순위", "forecast", "package", "draft", "plan"):
		return assistantFinanceARCollectionWorkflowSpec(), true
	case containsAny(lower, "영수증 정산", "영수증정산", "법인카드", "경비 정산", "경비정산", "정산표", "비용 정산", "구매품의", "구매 품의", "결재 요청", "결재요청", "expense", "receipt reconciliation", "corporate card", "vendor invoice", "invoice approval") &&
		containsAny(lower, "표", "보고서", "ppt", "pptx", "문서", "엑셀", "xlsx", "csv", "그래프", "음성", "패키지", "정리", "분석", "만들"):
		return assistantFinanceExpenseWorkflowSpec(), true
	case containsAny(lower, "구매팀", "구매 팀", "구매 품의", "구매품의", "발주 품의", "발주품의", "벤더 비교", "공급사 비교", "견적 비교", "tco", "납기 리스크", "procurement", "purchase approval", "vendor selection", "vendor comparison", "supplier comparison", "quote comparison", "purchase order prep") &&
		containsAny(lower, "표", "보고서", "ppt", "pptx", "문서", "엑셀", "xlsx", "csv", "그래프", "음성", "패키지", "품의서", "견적", "발주", "승인", "리스크", "만들", "작성", "정리", "분석", "package", "memo", "approval", "risk"):
		return assistantProcurementVendorWorkflowSpec(), true
	case containsAny(lower, "rfp", "제안요청서", "제안 요청서", "영업 제안서", "제안서 응답", "수주 제안", "입찰 제안", "프리세일즈", "presales", "sales proposal", "proposal response") &&
		containsAny(lower, "표", "보고서", "ppt", "pptx", "문서", "견적", "가격", "요구사항", "매트릭스", "데모", "후속 메일", "메일", "패키지", "만들", "작성", "분석", "정리", "requirements", "matrix", "pricing", "follow-up", "followup", "package"):
		return assistantSalesRFPWorkflowSpec(), true
	case containsAny(lower, "고객지원", "고객 지원", "고객 문의", "고객문의", "지원 티켓", "cs 티켓", "cs팀", "환불 문의", "장애 문의", "고객 불만", "고객 응대", "support ticket", "customer support", "helpdesk", "refund request", "incident response") &&
		containsAny(lower, "표", "보고서", "ppt", "pptx", "문서", "답변", "초안", "에스컬레이션", "faq", "그래프", "음성", "패키지", "분류", "정리", "분석", "만들", "ticket", "triage", "reply", "draft", "package"):
		return assistantSupportTicketWorkflowSpec(), true
	case containsAny(lower, "제품팀", "제품 기획", "제품기획", "프로덕트", "prd", "제품 요구사항", "기능 요구사항", "신기능", "로드맵", "릴리즈 노트", "릴리즈노트", "product requirements", "product roadmap", "release notes", "feature spec", "product backlog") &&
		containsAny(lower, "문서", "보고서", "ppt", "pptx", "표", "백로그", "로드맵", "지표", "실험", "릴리즈", "qa", "패키지", "만들", "작성", "정리", "분석", "draft", "package", "metrics", "roadmap", "backlog"):
		return assistantProductPRDWorkflowSpec(), true
	case containsAny(lower, "계약서", "계약 검토", "벤더 온보딩", "벤더온보딩", "법무", "구매 계약", "saas 계약", "dpa", "개인정보 처리위탁", "자동 갱신", "책임 제한", "contract review", "vendor onboarding", "vendor contract", "data processing agreement", "liability cap") &&
		containsAny(lower, "표", "보고서", "ppt", "pptx", "문서", "리스크", "조항", "협상", "메일", "체크리스트", "그래프", "음성", "패키지", "만들", "작성", "정리", "분석", "matrix", "checklist", "negotiation", "draft", "package"):
		return assistantLegalContractWorkflowSpec(), true
	case containsAny(lower, "여행", "출장", "trip", "travel") &&
		containsAny(lower, "2박3일", "2박 3일", "준비물", "예산", "동선", "일정표", "packing", "itinerary") &&
		containsAny(lower, "ppt", "pptx", "발표", "자료", "엑셀", "xlsx", "표", "패키지", "만들", "준비"):
		return assistantTravelPrepBundleSpec(), true
	case containsAny(lower, "쿠팡", "coupang", "쇼핑", "상품", "product") &&
		containsAny(lower, "비교표", "구매 체크리스트", "체크리스트", "워크북", "엑셀", "xlsx", "ppt", "pptx", "문서", "comparison", "checklist", "workbook", "spreadsheet", "document") &&
		containsAny(lower, "만들", "작성", "준비", "정리", "비교", "make", "create", "prepare", "compare"):
		return assistantShoppingDecisionWorkbookSpec(), true
	case containsAny(lower, "회의 메모", "회의메모", "회의 노트", "회의노트", "회의록", "미팅 노트", "meeting notes", "meeting minutes", "minutes") &&
		containsAny(lower, "결정사항", "결정 사항", "할일", "할 일", "액션", "리스크", "담당자", "action item", "risk") &&
		!containsAny(lower, "보고방", "보내", "전송", "send", "briefing"):
		return assistantMeetingMinutesPackSpec(), true
	case containsAny(lower, "제품군 a", "제품군a", "제품군별 성장", "성장 원인") && containsAny(lower, "임원", "한 페이지", "1페이지", "보고", "요약"):
		return assistantMarketingProductGrowthBriefSpec(), true
	case containsAny(lower, "지난 6개월", "최근 6개월", "6개월", "월별 매출", "매출 분석", "그래프화") && containsAny(lower, "매출", "그래프", "그래프화", "회의자료", "제품군", "roas"):
		return assistantMarketingSalesWorkflowSpec(), true
	case containsAny(lower, "roas", "저효율", "낮은 채널", "채널을 줄", "예산 재배분", "예산재배분"):
		return assistantMarketingROASBudgetReallocationSpec(), true
	case containsAny(lower, "상위 리드", "리드 20", "후속 콜", "후속콜", "영업 액션", "sla"):
		return assistantMarketingLeadActionPlanSpec(), true
	case containsAny(lower, "경쟁사", "경쟁 업체", "경쟁업체", "competitor", "battlecard", "배틀카드") && containsAny(lower, "비교", "세일즈", "영업", "영업팀", "3곳", "세 곳", "세곳"):
		return assistantMarketingCompetitorBattlecardSpec(), true
	case containsAny(lower, "매출 회의", "영업 회의", "다음 주 회의", "다음주 회의") && containsAny(lower, "아젠다", "질문", "준비", "회의자료"):
		return assistantMarketingRevenueMeetingSpec(), true
	case containsAny(lower, "매출", "roas", "마케팅", "영업", "제품군", "성장 그래프", "전환율", "리드", "sales", "revenue"):
		return assistantMarketingSalesWorkflowSpec(), true
	default:
		return assistantDepartmentWorkflowSpec{}, false
	}
}

func assistantAdvertisingWorkflowSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "advertising_campaign",
		Title:    "광고기획팀 캠페인 실행 패키지",
		Audience: "광고기획팀, 브랜드 매니저, 퍼포먼스 마케터",
		Summary:  "캠페인 콘셉트, 타깃, 채널 믹스, 카피, A/B 테스트, KPI 표를 한 번에 만들었습니다.",
		Highlights: []string{
			"콘셉트 5개: 신뢰형, 문제해결형, 비교형, 체험형, 긴급 프로모션형",
			"카피 12개: 검색광고, SNS, 랜딩페이지 헤드라인, 리타겟팅 문구로 분리",
			"KPI: 노출, 클릭률, 전환율, CAC, ROAS, 장바구니 이탈률",
			"A/B 테스트: 메시지, 이미지, 가격혜택, CTA, 랜딩 상단 구조를 나눠 검증",
		},
		Commands: []string{
			"이 캠페인안을 20대 여성 타깃으로 바꿔줘",
			"검색광고 카피만 더 날카롭게 20개 다시 써줘",
			"이 KPI 표를 다음 주 실험 일정표로 바꿔줘",
		},
		DocBody: strings.Join([]string{
			"# 광고기획팀 캠페인 실행 패키지",
			"",
			"## 목표",
			"신제품 인지도를 높이면서 구매 전환까지 이어지는 2주 캠페인을 설계한다.",
			"",
			"## 타깃",
			"- 1차: 문제를 이미 느끼고 검색 중인 고의도 고객",
			"- 2차: SNS에서 후기와 비교 콘텐츠를 보고 움직이는 잠재 고객",
			"- 3차: 장바구니에 담았지만 결제하지 않은 리타겟팅 고객",
			"",
			"## 캠페인 콘셉트 5개",
			"1. 신뢰형: 실제 사용자 후기와 검증 포인트를 앞세운다.",
			"2. 문제해결형: 고객이 겪는 불편을 첫 문장에 놓고 해결 장면을 보여준다.",
			"3. 비교형: 기존 대안 대비 시간, 비용, 편의성 차이를 표로 보여준다.",
			"4. 체험형: 첫 구매/무료체험/샘플 신청을 낮은 마찰로 유도한다.",
			"5. 긴급 프로모션형: 기간 한정 혜택을 명확히 하되 과장 표현은 피한다.",
			"",
			"## 채널 믹스",
			"- 검색광고 35%: 구매 의도가 강한 키워드 중심",
			"- SNS 25%: 후기형 짧은 영상과 카드뉴스",
			"- 리테일미디어 20%: 구매 직전 고객 대상",
			"- 리타겟팅 15%: 장바구니/상세페이지 방문자",
			"- 실험 예산 5%: 신규 소재와 메시지 테스트",
			"",
			"## A/B 테스트",
			"- A안: 문제해결 메시지 + 후기 이미지 + 즉시 구매 CTA",
			"- B안: 비교표 메시지 + 제품 클로즈업 + 혜택 확인 CTA",
			"- 판정 기준: 전환율 20% 이상 개선 또는 CAC 15% 이상 감소",
		}, "\n"),
		DeckBody: strings.Join([]string{
			"# 캠페인 목표",
			"인지, 클릭, 구매 전환을 2주 안에 동시에 끌어올린다.",
			"# 타깃",
			"고의도 검색 고객, 후기 반응 고객, 장바구니 이탈 고객으로 나눈다.",
			"# 콘셉트",
			"신뢰형, 문제해결형, 비교형, 체험형, 긴급 프로모션형 5개 안을 제안한다.",
			"# 채널 믹스",
			"검색 35%, SNS 25%, 리테일미디어 20%, 리타겟팅 15%, 실험 5%.",
			"# 카피 방향",
			"첫 문장에는 문제, 둘째 문장에는 차이, CTA에는 다음 행동을 둔다.",
			"# 실험 설계",
			"메시지, 이미지, CTA, 혜택, 랜딩 상단 구조를 A/B로 검증한다.",
			"# KPI",
			"CTR, CVR, CAC, ROAS, 장바구니 이탈률, 재방문율을 본다.",
			"# 다음 액션",
			"소재 4종 제작, 랜딩 상단 2안 준비, 첫 주 예산 60%만 집행한다.",
		}, "\n\n"),
		SheetBody: strings.Join([]string{
			"|항목|A안|B안|측정 지표|목표|판정 기준|",
			"|---|---|---|---|---:|---|",
			"|메시지|불편 해결|대안 비교|CTR|3.5|높은 CTR 채택|",
			"|이미지|후기 카드|제품 클로즈업|CVR|4.2|전환율 높은 안 채택|",
			"|CTA|지금 해결하기|혜택 확인하기|CAC|22000|CAC 낮은 안 채택|",
			"|랜딩 상단|문제 제기|비교표|ROAS|3.0|ROAS 높은 안 채택|",
			"|리타겟팅|후기 강조|기간 혜택|장바구니 회복률|12|회복률 높은 안 채택|",
		}, "\n"),
	}
}

func assistantAdvertisingCopyWorkflowSpec() assistantDepartmentWorkflowSpec {
	copies := []string{
		"지금 불편한 과정을 3분 안에 줄여보세요",
		"비교해 보면 선택은 더 쉬워집니다",
		"후기에서 먼저 검증된 선택",
		"오늘 바꾸면 다음 주 업무가 가벼워집니다",
		"복잡한 설정 없이 바로 시작하세요",
		"비용은 낮추고 전환은 선명하게",
		"처음 쓰는 사람도 바로 이해하는 솔루션",
		"놓친 고객을 다시 데려오는 가장 짧은 경로",
		"후회 없는 선택을 위해 핵심만 비교했습니다",
		"반복 업무는 줄이고 중요한 결정에 집중하세요",
		"구매 전 마지막 고민을 줄여드립니다",
		"실제 사용자가 남긴 차이를 확인하세요",
		"빠른 도입, 쉬운 전환, 명확한 효과",
		"가격보다 중요한 건 매일 아끼는 시간입니다",
		"한 번의 설정으로 매주 반복되는 일을 줄이세요",
		"대안은 많지만 기준은 분명해야 합니다",
		"오늘의 작은 개선이 월말 숫자를 바꿉니다",
		"팀 전체가 같은 기준으로 움직이게 하세요",
		"광고비가 새는 지점을 먼저 잡겠습니다",
		"지금 필요한 건 더 많은 클릭이 아니라 더 나은 전환입니다",
	}
	docLines := []string{
		"# 검색광고 카피 20개 개선안",
		"",
		"## 방향",
		"기존 캠페인안을 검색 의도가 높은 고객에게 맞춰 더 짧고 선명한 문장으로 바꿨습니다. 각 문구는 문제 인식, 비교, 후기 검증, 전환 행동을 분리해서 테스트할 수 있게 구성했습니다.",
		"",
		"## 카피 20개",
	}
	for i, copy := range copies {
		docLines = append(docLines, fmt.Sprintf("%d. %s", i+1, copy))
	}
	docLines = append(docLines,
		"",
		"## 운영 메모",
		"- 브랜드 키워드에는 신뢰형 문구를 먼저 배치합니다.",
		"- 경쟁 비교 키워드에는 비교형 문구를 배치합니다.",
		"- 문제 해결 키워드에는 시간 절약과 전환 행동을 강조합니다.",
		"- 첫 3일은 CTR, 그 다음 4일은 CVR과 CAC로 판정합니다.",
	)
	return assistantDepartmentWorkflowSpec{
		Kind:     "advertising_search_ad_copy",
		Title:    "검색광고 카피 20개 개선안",
		Audience: "광고기획팀, 퍼포먼스 마케터",
		Summary:  "검색광고용 카피 20개를 더 날카롭게 다시 쓰고, A/B 테스트 표로 정리했습니다.",
		Highlights: []string{
			"구매 의도 키워드용 문구, 비교 키워드용 문구, 리타겟팅 문구로 분리",
			"각 카피마다 테스트 가설과 측정 지표를 붙일 수 있게 표로 구성",
			"CTR만 보지 않고 CVR, CAC, ROAS까지 이어지는 판정 기준 포함",
		},
		Commands: []string{
			"이 카피를 더 고급 브랜드 톤으로 바꿔줘",
			"카피별 랜딩페이지 헤드라인도 20개 만들어줘",
			"이 표를 이번 주 검색광고 실험 일정표로 바꿔줘",
		},
		DocBody: strings.Join(docLines, "\n"),
		DeckBody: strings.Join([]string{
			"# 목적",
			"검색 의도가 높은 고객에게 더 짧고 날카로운 문구를 테스트한다.",
			"# 카피 구조",
			"문제 인식, 비교, 후기 검증, 전환 행동으로 분리한다.",
			"# 테스트 기준",
			"CTR, CVR, CAC, ROAS를 함께 보고 승자를 정한다.",
			"# 운영",
			"브랜드 키워드, 경쟁 비교 키워드, 문제 해결 키워드에 다른 문구를 넣는다.",
			"# 다음 액션",
			"20개 카피를 4개 그룹으로 나눠 1주 실험 일정에 배치한다.",
		}, "\n\n"),
		SheetBody: assistantAdvertisingCopySheet(copies),
	}
}

func assistantAdvertisingCopySheet(copies []string) string {
	rows := []string{
		"|번호|검색 의도|카피|가설|주요 지표|판정 기준|",
		"|---:|---|---|---|---|---|",
	}
	for i, copy := range copies {
		intent := "문제 해결"
		if i%4 == 1 {
			intent = "비교 검토"
		} else if i%4 == 2 {
			intent = "후기 검증"
		} else if i%4 == 3 {
			intent = "전환 유도"
		}
		rows = append(rows, fmt.Sprintf("|%d|%s|%s|%s 키워드에서 반응이 높을 것|CTR/CVR/CAC|기존 대비 CTR 15%% 또는 CAC 10%% 개선|", i+1, intent, copy, intent))
	}
	return strings.Join(rows, "\n")
}

func assistantAdvertisingExperimentScheduleSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "advertising_experiment_schedule",
		Title:    "다음 주 광고 실험 일정표",
		Audience: "광고기획팀, 퍼포먼스 마케터",
		Summary:  "KPI 표를 다음 주 실행 가능한 광고 실험 일정표로 바꿨습니다.",
		Highlights: []string{
			"월요일은 소재 세팅, 화~목은 A/B 테스트, 금요일은 승자 판단",
			"CTR은 초반 판정, CVR/CAC/ROAS는 최종 판정 지표로 분리",
			"검색광고, SNS, 리타겟팅, 랜딩페이지 상단 실험을 같은 표에서 관리",
		},
		Commands: []string{
			"이 실험 일정표를 담당자별 체크리스트로 바꿔줘",
			"금요일 보고용 결과표 양식도 만들어줘",
			"실험 실패 시 대체 플랜을 추가해줘",
		},
		DocBody: strings.Join([]string{
			"# 다음 주 광고 실험 일정표",
			"",
			"## 운영 원칙",
			"월요일에 소재와 랜딩을 세팅하고, 화요일부터 목요일까지 A/B 테스트를 돌립니다. 금요일 오전에는 CTR, CVR, CAC, ROAS를 기준으로 승자를 판단하고 다음 주 예산 배분안을 확정합니다.",
			"",
			"## 판정 방식",
			"- CTR: 소재와 메시지의 1차 반응",
			"- CVR: 랜딩과 제안의 전환력",
			"- CAC: 실제 획득 비용",
			"- ROAS: 매출 기여도",
			"",
			"## 금요일 결정",
			"승자 안은 예산을 20% 증액하고, 패자 안은 문구 또는 랜딩 상단을 수정해 다음 실험 후보로 넘깁니다.",
		}, "\n"),
		DeckBody: strings.Join([]string{
			"# 목표",
			"다음 주 안에 메시지, 소재, 랜딩, CTA의 승자를 정한다.",
			"# 일정",
			"월 세팅, 화~목 집행, 금 판정.",
			"# 지표",
			"CTR, CVR, CAC, ROAS를 단계별로 본다.",
			"# 의사결정",
			"승자 예산 증액, 패자 수정 후 다음 실험으로 넘긴다.",
			"# 리스크",
			"표본 부족, 랜딩 오류, 쿠폰 조건 오류를 매일 점검한다.",
		}, "\n\n"),
		SheetBody: strings.Join([]string{
			"|요일|실험|A안|B안|담당|주요 지표|판정 기준|",
			"|---|---|---|---|---|---|---|",
			"|월|소재/랜딩 세팅|후기 이미지|제품 클로즈업|광고기획|세팅 완료|오류 0건|",
			"|화|검색광고 메시지|문제 해결|비교 우위|퍼포먼스|CTR|15% 개선|",
			"|수|랜딩 상단|문제 제기|비교표|콘텐츠|CVR|10% 개선|",
			"|목|리타겟팅 CTA|지금 해결하기|혜택 확인하기|퍼포먼스|CAC|10% 절감|",
			"|금|결과 판정|승자 예산 증액|패자 수정|팀장|ROAS|3.0 이상|",
		}, "\n"),
	}
}

func assistantAdvertisingYoungWomenTargetSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "advertising_young_women_target",
		Title:    "20대 여성 타깃 캠페인 수정안",
		Audience: "광고기획팀, 브랜드 매니저, 콘텐츠 마케터",
		Summary:  "기존 캠페인안을 20대 여성 타깃에 맞춰 메시지, 채널, 카피, 실험표까지 다시 구성했습니다.",
		Highlights: []string{
			"후기, 사용 장면, 비교 콘텐츠, 짧은 영상 중심으로 메시지 재구성",
			"SNS와 검색광고를 연결하고 리타겟팅은 장바구니/상세 방문자 위주로 운영",
			"과장 표현보다 실제 사용감, 시간 절약, 실패 회피 메시지를 강조",
		},
		Commands: []string{
			"이 수정안을 인스타 릴스 대본 5개로 바꿔줘",
			"20대 여성 타깃 검색광고 키워드표도 만들어줘",
			"카피를 더 친구가 추천하는 말투로 바꿔줘",
		},
		DocBody: strings.Join([]string{
			"# 20대 여성 타깃 캠페인 수정안",
			"",
			"## 타깃 인사이트",
			"20대 여성 타깃은 광고 문구보다 실제 사용 장면, 후기, 비교 콘텐츠, 실패를 줄이는 설명에 더 민감하게 반응합니다. 첫 문장은 공감, 둘째 문장은 차이, CTA는 가벼운 확인 행동으로 설계합니다.",
			"",
			"## 메시지 방향",
			"- 공감: 매번 비교하느라 지치는 상황을 짧게 짚는다.",
			"- 검증: 실제 후기와 전후 차이를 보여준다.",
			"- 행동: 구매보다 혜택 확인, 후기 보기, 내 상황에 맞는지 확인으로 낮춘다.",
			"",
			"## 채널",
			"- SNS 짧은 영상: 사용 장면과 후기 중심",
			"- 검색광고: 문제 해결과 비교 키워드 중심",
			"- 리타겟팅: 상세페이지 방문자와 장바구니 이탈자 중심",
		}, "\n"),
		DeckBody: strings.Join([]string{
			"# 타깃",
			"20대 여성, 후기와 실제 사용 장면에 민감한 비교 검토 고객.",
			"# 메시지",
			"공감, 검증, 가벼운 행동 유도로 구성한다.",
			"# 채널",
			"SNS 짧은 영상, 검색광고, 리타겟팅을 연결한다.",
			"# 카피",
			"친구가 추천하는 듯한 자연스러운 문장으로 쓴다.",
			"# 실험",
			"후기형 vs 비교형, 혜택 확인 CTA vs 후기 보기 CTA.",
		}, "\n\n"),
		SheetBody: strings.Join([]string{
			"|채널|소재|메시지|CTA|측정 지표|목표|",
			"|---|---|---|---|---|---:|",
			"|SNS 릴스|사용 장면|바쁜 아침에도 바로 쓰는 선택|후기 보기|저장률|8|",
			"|검색광고|비교 문구|비교 시간을 줄여주는 기준|혜택 확인|CTR|4.0|",
			"|리타겟팅|후기 카드|이미 본 사람들이 선택한 이유|다시 보기|CVR|5.0|",
			"|랜딩|전후 비교|쓰기 전과 후의 차이|내게 맞는지 확인|CAC|20000|",
		}, "\n"),
	}
}

func assistantHRWorkflowSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "hr_recruiting",
		Title:    "인사팀 채용 실행 패키지",
		Audience: "인사팀, 채용 담당자, 현업 면접관",
		Summary:  "채용공고, 후보자 스크리닝 표, 면접 질문지, 평가 루브릭, 온보딩 체크리스트를 만들었습니다.",
		Highlights: []string{
			"JD: 역할, 필수역량, 우대조건, 평가 기준을 분리",
			"스크리닝 표: 경력 적합도, 기술역량, 커뮤니케이션, 리스크를 점수화",
			"면접 질문: 기술/경험/협업/문제해결/문화 적합성으로 구성",
			"온보딩: 첫날, 첫 주, 30일 목표와 담당자 체크리스트 포함",
		},
		Commands: []string{
			"이 채용공고를 시니어 백엔드 엔지니어용으로 바꿔줘",
			"후보자 5명 이력서를 붙일테니 이 표 기준으로 비교해줘",
			"면접 질문을 더 압박 없는 말투로 바꿔줘",
		},
		DocBody: strings.Join([]string{
			"# 인사팀 채용 실행 패키지",
			"",
			"## 채용공고 초안",
			"우리는 제품 안정성과 고객 경험을 함께 개선할 백엔드 엔지니어를 찾습니다. 이 역할은 API 설계, 데이터 모델링, 운영 안정성, 팀 간 협업을 책임집니다.",
			"",
			"## 필수 역량",
			"- Go 또는 TypeScript 기반 서버 개발 경험",
			"- 관계형 데이터베이스 설계와 운영 경험",
			"- 장애 분석, 로그 확인, 성능 개선 경험",
			"- 제품/운영/고객지원 팀과 명확히 소통한 경험",
			"",
			"## 면접 질문",
			"1. 최근 장애를 분석하고 재발 방지한 경험을 설명해 주세요.",
			"2. API 설계에서 하위 호환성을 지킨 사례가 있나요?",
			"3. 일정이 촉박할 때 품질과 속도 사이에서 어떤 기준으로 결정했나요?",
			"4. 동료와 기술 의견이 달랐을 때 어떻게 정리했나요?",
			"",
			"## 평가 루브릭",
			"- 5점: 독립적으로 문제를 정의하고 팀에 재사용 가능한 개선을 남김",
			"- 3점: 주어진 문제를 안정적으로 해결하나 구조화는 보완 필요",
			"- 1점: 경험 설명이 추상적이고 실제 판단 기준이 불명확",
			"",
			"## 온보딩 체크리스트",
			"- 첫날: 계정, 저장소, 배포 권한, 제품 데모",
			"- 첫 주: 주요 장애 사례, 코드리뷰 기준, 운영 대시보드",
			"- 30일: 작은 개선 과제 1개 배포, 회고와 성장 계획 작성",
		}, "\n"),
		DeckBody: strings.Join([]string{
			"# 채용 목표",
			"제품 안정성과 운영 역량을 강화할 백엔드 엔지니어 채용.",
			"# 후보자상",
			"서버 개발, 데이터 모델링, 운영 안정성, 협업 커뮤니케이션을 모두 보는 후보.",
			"# JD 핵심",
			"역할, 필수역량, 우대조건, 평가 기준을 명확히 분리한다.",
			"# 스크리닝",
			"경력 적합도, 기술역량, 운영 경험, 커뮤니케이션, 리스크 점수.",
			"# 면접 구성",
			"기술 질문, 장애 경험, 협업 상황, 문제해결 사고를 확인한다.",
			"# 평가 루브릭",
			"5점/3점/1점 기준으로 면접관별 편차를 줄인다.",
			"# 온보딩",
			"첫날, 첫 주, 30일 목표를 담당자와 함께 관리한다.",
			"# 다음 액션",
			"채용공고 게시, 후보자 표 업데이트, 면접관 브리핑 진행.",
		}, "\n\n"),
		SheetBody: strings.Join([]string{
			"|후보자|경력 적합도|기술역량|운영 경험|커뮤니케이션|리스크|다음 액션|",
			"|---|---:|---:|---:|---:|---|---|",
			"|후보 A|5|4|5|4|연봉 기대 높음|1차 면접 우선|",
			"|후보 B|4|5|3|5|운영 경험 보완 필요|기술 과제 요청|",
			"|후보 C|3|3|4|4|도메인 전환 필요|보류 후 추가 질문|",
			"|후보 D|5|5|5|3|협업 사례 확인 필요|현업 면접 진행|",
			"|후보 E|4|4|4|5|입사 가능일 늦음|조건 확인|",
		}, "\n"),
		GraphBody: strings.Join([]string{
			"## 채용 파이프라인 흐름",
			"",
			"```mermaid",
			"flowchart LR",
			"  Applied[\"지원 312명\"] --> Screen[\"서류 통과 78명\"] --> First[\"1차 면접 41명\"] --> Final[\"최종 면접 18명\"] --> Offer[\"오퍼 9명\"] --> Hire[\"입사 6명\"]",
			"  Screen -. \"통과율 25%\" .-> JD[\"JD 필수/우대 요건 재정의\"]",
			"  First -. \"일정 병목\" .-> Slot[\"면접관 슬롯 확보\\n후보자 우선순위 조정\"]",
			"  Offer -. \"수락률 점검\" .-> Package[\"보상/복지 비교표\\n후보자별 오퍼 메시지\"]",
			"```",
			"",
			"## 면접 우선순위",
			"",
			"```mermaid",
			"quadrantChart",
			"  title 후보자 면접 우선순위",
			"  x-axis 기술역량 낮음 --> 기술역량 높음",
			"  y-axis 운영경험 낮음 --> 운영경험 높음",
			"  quadrant-1 즉시 면접",
			"  quadrant-2 운영 확인",
			"  quadrant-3 보류",
			"  quadrant-4 기술 과제",
			"  후보 A: [0.78, 0.86]",
			"  후보 B: [0.88, 0.58]",
			"  후보 C: [0.48, 0.62]",
			"  후보 D: [0.91, 0.91]",
			"  후보 E: [0.74, 0.72]",
			"```",
			"",
			"## 회의에서 바로 볼 데이터",
			"",
			"| 단계 | 인원 | 병목 | 다음 액션 |",
			"|---|---:|---|---|",
			"| 지원 | 312 | 지원자는 충분 | 필수/우대 요건 분리 |",
			"| 서류 통과 | 78 | 통과율 25% | JD 기준 재정의 |",
			"| 1차 면접 | 41 | 면접 일정 병목 | 면접관 슬롯 확보 |",
			"| 오퍼 | 9 | 수락률 확인 필요 | 보상/복지 비교표 생성 |",
			"| 입사 | 6 | 온보딩 관리 필요 | 첫 30일 체크리스트 발송 |",
		}, "\n"),
	}
}

func assistantHRSeniorBackendJDWorkflowSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "hr_senior_backend_jd",
		Title:    "시니어 백엔드 엔지니어 채용공고 패키지",
		Audience: "인사팀, CTO, 백엔드 리드",
		Summary:  "시니어 백엔드 엔지니어용 채용공고, 평가 기준, 면접 흐름, 후보자 스크리닝 표를 만들었습니다.",
		Highlights: []string{
			"JD를 역할, 필수 경험, 우대 경험, 평가 기준으로 분리",
			"운영 안정성, API 설계, 데이터 모델링, 장애 대응 경험을 핵심 역량으로 설정",
			"지원자에게 부담을 주지 않으면서도 실제 경험을 확인하는 질문 포함",
		},
		Commands: []string{
			"이 JD를 더 스타트업다운 톤으로 바꿔줘",
			"이 JD 기준으로 링크드인 스카우트 메시지도 만들어줘",
			"1차 면접 45분 진행표로 바꿔줘",
		},
		DocBody: strings.Join([]string{
			"# 시니어 백엔드 엔지니어 채용공고",
			"",
			"## 포지션 소개",
			"제품의 핵심 API, 데이터 모델, 운영 안정성을 함께 책임질 시니어 백엔드 엔지니어를 찾습니다. 이 역할은 단순 기능 구현보다 장애를 줄이고, 팀이 재사용할 수 있는 구조를 만들고, 제품 의사결정에 필요한 기술적 근거를 제시하는 역할입니다.",
			"",
			"## 주요 업무",
			"- 핵심 API와 백엔드 서비스 설계 및 개발",
			"- 데이터 모델링, 쿼리 성능 개선, 운영 지표 관리",
			"- 장애 분석과 재발 방지 설계",
			"- 배포, 모니터링, 알림, 로그 체계 개선",
			"- 제품/디자인/운영팀과 요구사항을 구조화하고 일정 리스크를 조율",
			"",
			"## 필수 경험",
			"- Go, TypeScript, Java, Kotlin 중 하나 이상의 서버 개발 실무 경험 5년 이상",
			"- RDBMS 기반 서비스 설계와 운영 경험",
			"- 트래픽 증가, 장애, 성능 병목을 직접 해결한 경험",
			"- 코드리뷰와 기술 문서 작성 경험",
			"",
			"## 우대 경험",
			"- 결제, 커머스, B2B SaaS, 메시징, 검색, 데이터 파이프라인 경험",
			"- Kubernetes, observability, queue, event-driven architecture 경험",
			"- 보안/개인정보/권한 모델 설계 경험",
			"",
			"## 평가 기준",
			"좋은 후보는 문제를 기술 용어로만 설명하지 않고, 고객 영향, 장애 확산 경로, 팀 운영 비용, 재발 방지 구조까지 함께 설명할 수 있어야 합니다.",
		}, "\n"),
		DeckBody: strings.Join([]string{
			"# 포지션 목표",
			"제품 안정성과 백엔드 구조를 끌어올릴 시니어 엔지니어 채용.",
			"# 핵심 역량",
			"API 설계, 데이터 모델링, 장애 대응, 운영 안정성, 협업.",
			"# 평가 기준",
			"기능 구현보다 운영 비용과 재발 방지 구조를 설명할 수 있는지 본다.",
			"# 면접 흐름",
			"경험 확인, 시스템 설계, 장애 회고, 협업 상황, 후보자 질문.",
			"# 다음 액션",
			"JD 게시, 스카우트 메시지 발송 초안, 면접관 브리핑.",
		}, "\n\n"),
		SheetBody: strings.Join([]string{
			"|평가 항목|배점|확인 질문|좋은 신호|주의 신호|",
			"|---|---:|---|---|---|",
			"|API 설계|20|최근 설계한 API의 하위 호환성은 어떻게 지켰나요?|버전/계약/모니터링 언급|엔드포인트 설명만 함|",
			"|데이터 모델링|20|성능 문제를 어떤 지표로 확인했나요?|쿼리/인덱스/락/캐시 설명|막연히 튜닝했다고 함|",
			"|장애 대응|25|장애 재발 방지를 어떻게 설계했나요?|원인/대응/예방/알림 구조 설명|개인 노력 위주|",
			"|협업|15|일정과 품질 충돌을 어떻게 조율했나요?|트레이드오프와 소통 방식 설명|일방적 결정|",
			"|기술 리더십|20|팀에 남긴 재사용 가능한 개선은 무엇인가요?|문서/도구/리뷰 기준 제시|개인 성과만 강조|",
		}, "\n"),
	}
}

func assistantHRCandidateComparisonWorkflowSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "hr_candidate_comparison",
		Title:    "후보자 5명 스크리닝 비교표",
		Audience: "인사팀, 현업 면접관, 채용 의사결정자",
		Summary:  "후보자 5명을 같은 기준으로 비교할 수 있는 스크리닝 표, 면접 우선순위, 추가 확인 질문을 만들었습니다.",
		Highlights: []string{
			"경력 적합도, 기술역량, 운영 경험, 커뮤니케이션, 리스크를 점수화",
			"후보자별 다음 액션을 1차 면접, 기술 과제, 추가 확인, 보류로 분리",
			"이력서를 붙이면 같은 표 구조로 실제 후보자 데이터를 다시 채울 수 있음",
		},
		Commands: []string{
			"후보 A와 후보 D만 더 자세히 비교해줘",
			"이 표를 면접 일정표로 바꿔줘",
			"후보자별 불합격/보류 메일 초안 만들어줘",
		},
		DocBody: strings.Join([]string{
			"# 후보자 5명 스크리닝 비교표",
			"",
			"## 사용 방식",
			"후보자 이력서를 붙이면 Argos가 아래 기준으로 경력 적합도, 기술역량, 운영 경험, 커뮤니케이션, 리스크를 채점하고 다음 액션을 제안합니다. 현재 문서는 샘플 후보자 기준입니다.",
			"",
			"## 1차 추천",
			"후보 D와 후보 A는 운영 경험과 기술역량이 높아 1차 면접 우선 대상입니다. 후보 B는 기술역량은 좋지만 운영 경험 확인이 필요해 기술 과제를 먼저 권장합니다.",
			"",
			"## 추가 확인 질문",
			"- 후보 A: 연봉 기대치와 입사 가능일 확인",
			"- 후보 B: 장애 대응과 운영 경험 구체 사례 확인",
			"- 후보 C: 도메인 전환 의지와 학습 속도 확인",
			"- 후보 D: 협업 사례와 리더십 방식 확인",
			"- 후보 E: 입사 가능일과 장기 근속 가능성 확인",
		}, "\n"),
		DeckBody: strings.Join([]string{
			"# 후보자 비교 목적",
			"같은 기준으로 후보자 5명을 빠르게 비교하고 면접 우선순위를 정한다.",
			"# 평가 축",
			"경력 적합도, 기술역량, 운영 경험, 커뮤니케이션, 리스크.",
			"# 우선 후보",
			"후보 D와 후보 A를 1차 면접 우선 대상으로 둔다.",
			"# 보완 확인",
			"후보 B는 운영 경험, 후보 C는 도메인 전환, 후보 E는 입사 가능일 확인.",
			"# 다음 액션",
			"면접 일정 확정, 후보별 추가 질문 전달, 보류 후보 메일 초안 작성.",
		}, "\n\n"),
		SheetBody: strings.Join([]string{
			"|후보자|경력 적합도|기술역량|운영 경험|커뮤니케이션|리스크|종합|다음 액션|",
			"|---|---:|---:|---:|---:|---|---:|---|",
			"|후보 A|5|4|5|4|연봉 기대 높음|18|1차 면접 우선|",
			"|후보 B|4|5|3|5|운영 경험 보완 필요|17|기술 과제 요청|",
			"|후보 C|3|3|4|4|도메인 전환 필요|14|보류 후 추가 질문|",
			"|후보 D|5|5|5|3|협업 사례 확인 필요|18|현업 면접 진행|",
			"|후보 E|4|4|4|5|입사 가능일 늦음|17|조건 확인|",
		}, "\n"),
	}
}

func assistantHRGentleInterviewWorkflowSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "hr_gentle_interview_questions",
		Title:    "압박 없는 면접 질문지",
		Audience: "면접관, 인사팀, 현업 리드",
		Summary:  "후보자가 방어적으로 느끼지 않도록 부드러운 말투의 면접 질문, 꼬리 질문, 평가 메모 표를 만들었습니다.",
		Highlights: []string{
			"압박 질문을 경험 회고형 질문으로 바꿔 후보자가 구체 사례를 말하기 쉽게 구성",
			"기술역량, 장애 대응, 협업, 일정 조율, 성장 태도를 모두 확인",
			"면접관이 바로 쓸 수 있는 오프닝/전환/마무리 문장 포함",
		},
		Commands: []string{
			"이 질문지를 30분 면접 진행표로 바꿔줘",
			"주니어 후보자용으로 더 쉽게 바꿔줘",
			"면접관 평가 메모 양식을 더 자세히 만들어줘",
		},
		DocBody: strings.Join([]string{
			"# 압박 없는 면접 질문지",
			"",
			"## 오프닝",
			"오늘 면접은 정답을 맞히는 자리가 아니라, 그동안 어떤 문제를 어떤 방식으로 풀어오셨는지 함께 이해하는 시간입니다. 편하게 실제 경험 중심으로 말씀해 주세요.",
			"",
			"## 기술 경험",
			"1. 최근에 설계하거나 개선한 백엔드 기능 중 가장 기억에 남는 작업을 소개해 주실 수 있을까요?",
			"2. 그 작업에서 처음에는 예상하지 못했지만 진행하면서 알게 된 제약이 있었나요?",
			"3. 성능이나 안정성을 확인할 때 어떤 지표를 먼저 보셨나요?",
			"",
			"## 장애 대응",
			"4. 운영 중 문제가 생겼을 때 차분히 원인을 좁혀간 경험이 있다면 들려주세요.",
			"5. 그 경험 이후 팀에 남긴 개선이나 문서가 있었나요?",
			"",
			"## 협업",
			"6. 다른 직군과 요구사항을 조율하면서 생각이 바뀐 경험이 있나요?",
			"7. 일정이 빠듯할 때 품질과 범위를 어떻게 설명하고 합의하셨나요?",
			"",
			"## 마무리",
			"오늘 대화에서 저희가 더 잘 이해하면 좋을 강점이나, 질문하지 않았지만 꼭 말씀하고 싶은 경험이 있을까요?",
		}, "\n"),
		DeckBody: strings.Join([]string{
			"# 면접 톤",
			"정답 압박보다 실제 경험을 편하게 끌어내는 질문으로 구성한다.",
			"# 확인 영역",
			"기술 경험, 장애 대응, 협업, 일정 조율, 성장 태도.",
			"# 오프닝",
			"후보자가 방어적으로 느끼지 않도록 면접 목적을 설명한다.",
			"# 꼬리 질문",
			"왜, 어떻게, 그 뒤 무엇이 달라졌는지를 부드럽게 확인한다.",
			"# 평가",
			"구체성, 판단 기준, 재발 방지, 협업 태도를 메모한다.",
		}, "\n\n"),
		SheetBody: strings.Join([]string{
			"|영역|질문|좋은 신호|추가 확인|평가 메모|",
			"|---|---|---|---|---|",
			"|기술 경험|기억에 남는 백엔드 개선 작업은?|문제/제약/결과를 함께 설명|본인 기여 범위| |",
			"|성능|어떤 지표를 먼저 봤나요?|수치와 도구를 말함|재현 방법| |",
			"|장애 대응|원인을 어떻게 좁혔나요?|가설과 검증 순서 설명|재발 방지| |",
			"|협업|요구사항 조율 경험은?|상대 관점과 합의 과정 설명|갈등 처리| |",
			"|일정 조율|범위와 품질을 어떻게 합의했나요?|트레이드오프 설명|의사결정 기록| |",
		}, "\n"),
	}
}

func assistantPublicAgencyWorkflowSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "public_agency_civil_service",
		Title:    "공공기관 민원·정책 업무 패키지",
		Audience: "정부 부처, 구청, 시청, 공공기관 실무자",
		Summary:  "민원 분류, 처리 현황, 시민 안내문, 보도자료, 정책 브리프, 예산 집행표를 만들었습니다.",
		Highlights: []string{
			"민원 유형: 교통, 복지, 환경, 안전, 생활불편으로 분류",
			"처리 기준: 긴급도, 반복성, 법정 처리기한, 담당 부서",
			"시민 안내문: 신청 서류, 처리 절차, 예상 소요 시간 중심",
			"간부 보고: 미해결 건수, 병목 부서, 다음 주 조치로 요약",
		},
		Commands: []string{
			"이 민원 보고서를 구청장 보고용 한 페이지로 줄여줘",
			"복지 문의만 따로 시민 안내문으로 다시 써줘",
			"예산 집행표를 분기별 그래프로 바꿔줘",
		},
		DocBody: strings.Join([]string{
			"# 공공기관 민원·정책 업무 패키지",
			"",
			"## 민원 처리 요약",
			"지난달 민원은 교통, 복지, 환경, 안전, 생활불편 유형으로 나뉘며, 교통 민원이 가장 많고 복지 문의는 서류 안내 반복 비중이 높습니다.",
			"",
			"## 부서별 조치",
			"- 교통: 출퇴근 시간대 불법주정차 집중 민원. 단속 시간표와 우회 안내문 필요.",
			"- 복지: 신청 서류 누락이 반복됨. 체크리스트형 안내문 배포 필요.",
			"- 환경: 소음/폐기물 신고는 현장 확인 지연이 병목. 출동 우선순위표 필요.",
			"- 안전: 시설물 위험 신고는 즉시 조치 기준과 사진 첨부 양식 필요.",
			"",
			"## 시민 안내문 초안",
			"민원 신청 시 주소, 발생 시간, 사진, 연락 가능한 전화번호를 함께 제출해 주세요. 복지 신청은 신분증, 소득 확인 서류, 가족관계 증빙이 필요할 수 있습니다. 처리 결과는 접수 순서와 긴급도에 따라 안내됩니다.",
			"",
			"## 보도자료 초안",
			"구청은 반복 민원 해소를 위해 민원 유형별 처리 기준을 정비하고, 시민이 자주 묻는 절차를 알기 쉬운 안내문으로 배포합니다. 특히 교통·복지·환경 분야는 담당 부서별 처리 현황을 매주 점검합니다.",
			"",
			"## 다음 주 회의 안건",
			"1. 반복 민원 TOP 5 처리 기준 확정",
			"2. 부서별 미해결 건수와 지연 사유 확인",
			"3. 시민 안내문 배포 채널 결정",
			"4. 현장 점검 우선순위와 담당자 지정",
		}, "\n"),
		DeckBody: strings.Join([]string{
			"# 민원 현황",
			"교통, 복지, 환경, 안전, 생활불편 유형으로 분류.",
			"# 핵심 병목",
			"복지 서류 안내 반복, 환경 현장 확인 지연, 교통 출퇴근 시간대 집중.",
			"# 시민 안내",
			"필수 서류, 처리 절차, 예상 소요 시간을 짧게 안내한다.",
			"# 보도자료",
			"반복 민원 해소와 부서별 처리 기준 정비를 중심으로 설명한다.",
			"# 예산 집행",
			"분기별 집행률과 지연 사업을 함께 본다.",
			"# 부서별 액션",
			"교통 단속 시간표, 복지 체크리스트, 환경 출동 우선순위.",
			"# 리스크",
			"법정 처리기한 초과, 안내 불명확, 현장 확인 지연.",
			"# 다음 회의",
			"TOP 5 반복 민원, 미해결 건수, 안내문 배포 채널 결정.",
		}, "\n\n"),
		SheetBody: strings.Join([]string{
			"|유형|접수|처리완료|미해결|평균 처리일|담당 부서|다음 조치|",
			"|---|---:|---:|---:|---:|---|---|",
			"|교통|428|382|46|3.1|교통행정과|출퇴근 집중 단속표|",
			"|복지|216|177|39|5.4|복지정책과|서류 체크리스트 배포|",
			"|환경|132|91|41|6.8|환경관리과|현장 점검 우선순위|",
			"|안전|74|63|11|2.2|안전총괄과|긴급 조치 기준 고지|",
			"|생활불편|189|164|25|4.0|민원여권과|FAQ 업데이트|",
		}, "\n"),
		GraphBody: strings.Join([]string{
			"## 민원 처리 흐름",
			"",
			"```mermaid",
			"flowchart LR",
			"  Intake[\"접수 1,039건\"] --> Classify[\"유형 분류\\n교통·복지·환경·안전·생활불편\"] --> Route[\"담당 부서 배정\"] --> Done[\"처리 완료 877건\"]",
			"  Route --> Backlog[\"미해결 162건\"]",
			"  Backlog -. \"교통 46건\" .-> Traffic[\"출퇴근 집중 단속표\"]",
			"  Backlog -. \"환경 41건\" .-> Field[\"현장 점검 우선순위\"]",
			"  Backlog -. \"복지 39건\" .-> Welfare[\"서류 체크리스트 안내문\"]",
			"```",
			"",
			"## 부서별 병목",
			"",
			"```mermaid",
			"pie title 미해결 민원 162건 구성",
			"  \"교통\" : 46",
			"  \"환경\" : 41",
			"  \"복지\" : 39",
			"  \"생활불편\" : 25",
			"  \"안전\" : 11",
			"```",
			"",
			"## 회의에서 바로 볼 데이터",
			"",
			"| 유형 | 접수 | 처리완료 | 미해결 | 다음 조치 |",
			"|---|---:|---:|---:|---|",
			"| 교통 | 428 | 382 | 46 | 출퇴근 집중 단속표 |",
			"| 복지 | 216 | 177 | 39 | 서류 체크리스트 배포 |",
			"| 환경 | 132 | 91 | 41 | 현장 점검 우선순위 |",
			"| 안전 | 74 | 63 | 11 | 긴급 조치 기준 고지 |",
			"| 생활불편 | 189 | 164 | 25 | FAQ 업데이트 |",
		}, "\n"),
	}
}

func assistantPublicAgencyExecutiveBriefSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "public_agency_executive_brief",
		Title:    "구청장 보고용 민원 1페이지 브리프",
		Audience: "구청장, 부구청장, 국장, 민원 총괄 담당자",
		Summary:  "민원 보고서를 간부가 바로 읽을 수 있는 1페이지 브리프, 회의용 PPT, 핵심 지표표로 줄였습니다.",
		Highlights: []string{
			"핵심 숫자: 접수 1,039건, 미해결 162건, 평균 처리 4.3일",
			"최대 병목: 교통 민원 집중, 복지 서류 안내 반복, 환경 현장 확인 지연",
			"결정 요청: 반복 민원 TOP 5 처리 기준과 시민 안내문 배포 채널 확정",
		},
		Commands: []string{
			"이 브리프를 구청장 구두보고 원고로 바꿔줘",
			"미해결 162건만 부서별 조치표로 다시 만들어줘",
			"시민 안내문 배포 계획만 따로 일정표로 만들어줘",
		},
		DocBody: strings.Join([]string{
			"# 구청장 보고용 민원 1페이지 브리프",
			"",
			"## 한 줄 결론",
			"지난달 민원은 총 1,039건이며 미해결 162건입니다. 교통 민원은 출퇴근 시간대에 집중되고, 복지 문의는 서류 안내 반복이 많으며, 환경 민원은 현장 확인 지연이 병목입니다.",
			"",
			"## 핵심 지표",
			"- 접수: 1,039건",
			"- 처리 완료: 877건",
			"- 미해결: 162건",
			"- 평균 처리일: 4.3일",
			"- 긴급 확인 필요: 환경 현장 확인 41건, 교통 미해결 46건",
			"",
			"## 부서별 판단",
			"- 교통행정과: 출퇴근 집중 민원. 단속 시간표와 우회 안내문 필요.",
			"- 복지정책과: 신청 서류 누락 반복. 체크리스트형 안내문 필요.",
			"- 환경관리과: 현장 확인 지연. 출동 우선순위 기준 필요.",
			"",
			"## 오늘 결정할 일",
			"1. 반복 민원 TOP 5 처리 기준 확정",
			"2. 시민 안내문 배포 채널 결정",
			"3. 환경 현장 점검 우선순위와 담당자 지정",
		}, "\n"),
		DeckBody: strings.Join([]string{
			"# 한 줄 결론",
			"민원 1,039건 중 미해결 162건. 교통, 복지, 환경이 병목.",
			"# 핵심 숫자",
			"처리 완료 877건, 평균 처리 4.3일, 긴급 확인 87건.",
			"# 병목",
			"교통 집중, 복지 서류 반복, 환경 현장 확인 지연.",
			"# 결정 요청",
			"처리 기준, 안내문 채널, 현장 점검 우선순위 확정.",
			"# 다음 액션",
			"부서별 미해결 조치표를 매주 갱신한다.",
		}, "\n\n"),
		SheetBody: strings.Join([]string{
			"|부서|접수|완료|미해결|평균 처리일|위험도|오늘 결정|",
			"|---|---:|---:|---:|---:|---|---|",
			"|교통행정과|428|382|46|3.1|높음|출퇴근 단속 시간표|",
			"|복지정책과|216|177|39|5.4|중간|서류 체크리스트 배포|",
			"|환경관리과|132|91|41|6.8|높음|현장 점검 우선순위|",
			"|안전총괄과|74|63|11|2.2|낮음|긴급 조치 기준 고지|",
			"|민원여권과|189|164|25|4.0|중간|FAQ 업데이트|",
		}, "\n"),
	}
}

func assistantPublicAgencyWelfareNoticeSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "public_agency_welfare_notice",
		Title:    "복지 문의 시민 안내문",
		Audience: "복지정책과, 민원실, 시민 안내 담당자",
		Summary:  "복지 문의만 따로 뽑아 시민이 바로 이해할 수 있는 안내문, FAQ, 제출서류 체크표로 다시 썼습니다.",
		Highlights: []string{
			"시민이 먼저 궁금해하는 신청 대상, 준비 서류, 처리 기간을 앞에 배치",
			"반복 문의를 줄이기 위해 접수 전 체크리스트와 문의 채널을 명확히 표시",
			"민원실 직원이 그대로 읽을 수 있는 전화 응대 스크립트 포함",
		},
		Commands: []string{
			"이 안내문을 문자메시지 500자 버전으로 줄여줘",
			"전화 응대 스크립트만 따로 만들어줘",
			"복지 신청 서류 체크리스트를 인쇄용 표로 바꿔줘",
		},
		DocBody: strings.Join([]string{
			"# 복지 문의 시민 안내문",
			"",
			"## 안내 제목",
			"복지 지원 신청 전에 아래 서류와 절차를 먼저 확인해 주세요.",
			"",
			"## 신청 전 확인할 것",
			"1. 본인 확인을 위한 신분증을 준비해 주세요.",
			"2. 소득 확인이 필요한 사업은 최근 소득 증빙 서류가 필요할 수 있습니다.",
			"3. 가족관계 확인이 필요한 경우 가족관계증명서가 필요할 수 있습니다.",
			"4. 대리 신청은 위임장과 대리인 신분증을 함께 준비해 주세요.",
			"",
			"## 처리 절차",
			"접수 후 담당 부서가 서류를 확인하고, 보완이 필요하면 연락드립니다. 서류가 모두 준비된 경우 일반 문의는 3~5영업일 안에 안내됩니다.",
			"",
			"## 자주 묻는 질문",
			"- 서류가 부족하면 어떻게 되나요? 담당자가 보완 서류를 안내합니다.",
			"- 방문 없이 신청할 수 있나요? 사업별로 온라인 신청 가능 여부가 다릅니다.",
			"- 처리 결과는 어디서 확인하나요? 접수 문자 또는 담당 부서 안내에 따라 확인합니다.",
			"",
			"## 전화 응대 스크립트",
			"문의 주셔서 감사합니다. 먼저 신청하려는 복지 사업명과 현재 준비하신 서류를 확인하겠습니다. 신분증, 소득 확인 서류, 가족관계 증빙이 필요한지 차례로 안내드리겠습니다.",
		}, "\n"),
		DeckBody: strings.Join([]string{
			"# 안내 목적",
			"복지 문의 반복을 줄이고 시민이 신청 전 준비물을 이해하게 한다.",
			"# 핵심 정보",
			"신청 대상, 제출 서류, 처리 절차, 예상 기간.",
			"# 체크리스트",
			"신분증, 소득 확인, 가족관계, 위임장.",
			"# FAQ",
			"서류 부족, 온라인 신청, 결과 확인 방법.",
			"# 직원 스크립트",
			"사업명과 준비 서류를 먼저 확인하고 누락 서류를 안내한다.",
		}, "\n\n"),
		SheetBody: strings.Join([]string{
			"|항목|필수 여부|설명|누락 시 안내|",
			"|---|---|---|---|",
			"|신분증|필수|본인 확인|신분증 지참 후 재방문/재접수|",
			"|소득 증빙|사업별|최근 소득 확인|대상 사업 확인 후 보완 요청|",
			"|가족관계증명서|사업별|가구원 확인|해당 시 발급 안내|",
			"|위임장|대리 신청 시|대리 접수 확인|대리인 신분증 함께 안내|",
			"|연락처|필수|보완 요청 연락|휴대전화 번호 재확인|",
		}, "\n"),
	}
}

func assistantPublicAgencyBudgetGraphSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "public_agency_budget_graph",
		Title:    "분기별 예산 집행 그래프 패키지",
		Audience: "예산팀, 기획조정실, 간부회의 참석자",
		Summary:  "예산 집행표를 분기별 그래프 데이터, 지연 사업 목록, 간부회의용 PPT로 바꿨습니다.",
		Highlights: []string{
			"분기별 집행률과 누적 집행률을 같이 볼 수 있게 구성",
			"지연 사업은 원인, 위험도, 다음 조치까지 표로 분리",
			"간부회의에서는 Q2 지연과 Q4 몰림 위험을 핵심 메시지로 제시",
		},
		Commands: []string{
			"Q2 지연 사업만 따로 보고서로 만들어줘",
			"이 예산 그래프를 국장회의 3장 PPT로 줄여줘",
			"집행률 70% 미만 사업만 조치표로 뽑아줘",
		},
		DocBody: strings.Join([]string{
			"# 분기별 예산 집행 그래프 패키지",
			"",
			"## 요약",
			"연간 예산 100억 기준 누적 집행률은 Q1 18%, Q2 39%, Q3 68%, Q4 예상 96%입니다. Q2에 일부 사업 발주가 지연되었고, Q4에 집행이 몰릴 위험이 있습니다.",
			"",
			"## 핵심 판단",
			"- Q1: 계획 대비 정상",
			"- Q2: 복지·환경 사업 일부 발주 지연",
			"- Q3: 교통·안전 사업 집행 회복",
			"- Q4: 미집행 잔액이 몰리지 않도록 월별 점검 필요",
			"",
			"## 다음 조치",
			"1. 집행률 70% 미만 사업 조치계획 제출",
			"2. 발주 지연 사업 원인 분류",
			"3. Q4 월별 집행 한도와 점검 회의 지정",
		}, "\n"),
		DeckBody: strings.Join([]string{
			"# 예산 집행 현황",
			"Q1 18%, Q2 39%, Q3 68%, Q4 예상 96%.",
			"# 위험 구간",
			"Q2 발주 지연과 Q4 집행 몰림 위험.",
			"# 지연 사업",
			"복지 안내 시스템, 환경 현장 점검 장비, 일부 교통 개선 공사.",
			"# 결정 요청",
			"70% 미만 사업 조치계획과 Q4 월별 점검 일정 확정.",
			"# 다음 액션",
			"월별 집행률 그래프를 매주 갱신한다.",
		}, "\n\n"),
		SheetBody: strings.Join([]string{
			"|분기|계획 집행액|실제 집행액|누적 집행률|상태|그래프 메모|",
			"|---|---:|---:|---:|---|---|",
			"|Q1|1800000000|1800000000|18|정상|막대그래프 시작점|",
			"|Q2|4300000000|3900000000|39|지연|복지·환경 발주 지연 표시|",
			"|Q3|7000000000|6800000000|68|회복|교통·안전 집행 회복|",
			"|Q4 예상|10000000000|9600000000|96|주의|집행 몰림 위험 표시|",
			"|복지 안내 시스템|800000000|520000000|65|주의|70% 미만 조치 필요|",
			"|환경 점검 장비|600000000|390000000|65|주의|발주 지연 원인 확인|",
		}, "\n"),
	}
}

func assistantChemicalMarginWorkflowSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "chemical_margin_risk",
		Title:    "화학회사 원자재·규제 대응 패키지",
		Audience: "화학회사 경영진, 구매팀, 영업팀, 규제대응팀, 재무팀",
		Summary:  "나프타 가격, 에틸렌 스프레드, PFAS/REACH 규제, 물류 리스크, 고객 가격 전가 전략을 회의용 보고서와 그래프로 정리했습니다.",
		Highlights: []string{
			"가장 먼저 볼 위험: 나프타 가격 상승과 에틸렌 스프레드 축소가 7월 마진을 압박",
			"규제 이슈: PFAS 제한 논의와 REACH 등록자료 보완 요청은 고객별 소재 전환표가 필요",
			"영업 액션: 자동차·전자 고객은 가격 전가 근거표, 생활소재 고객은 대체 원료 제안을 먼저 준비",
			"회의 결정: 가격 전가, 원료 헤지, 대체 공급선, 규제 문서 보강을 담당자별 액션으로 분리",
		},
		Commands: []string{
			"이 패키지를 내일 임원회의 5장 PPT로 줄여줘",
			"나프타 10% 상승 시 제품군별 마진 민감도표를 다시 계산해줘",
			"PFAS 규제 영향 고객사별 안내 메일 초안을 만들어줘",
		},
		DocBody: strings.Join([]string{
			"# 화학회사 원자재·규제 대응 패키지",
			"",
			"## 경영진 요약",
			"이번 주 화학회사 회의에서 먼저 다룰 토픽은 원자재 가격, 제품 스프레드, 규제 대응, 고객 가격 전가입니다. 나프타 가격 상승은 범용 제품군 마진을 압박하고, PFAS/REACH 관련 규제는 고객별 소재 전환과 문서 보강을 요구합니다. 단순 뉴스 요약이 아니라 구매, 영업, 규제대응, 재무가 같은 표를 보고 바로 결정을 내릴 수 있게 구성했습니다.",
			"",
			"## 내일 회의 핵심 토픽",
			"1. 나프타 가격 상승이 PE/PP/ABS 제품군 원가에 미치는 영향",
			"2. 에틸렌·프로필렌 스프레드 축소 구간에서 감산 또는 제품 믹스 조정 여부",
			"3. PFAS 제한 논의와 REACH 자료 보완 요청에 따른 고객 커뮤니케이션",
			"4. 자동차·전자 고객 가격 전가 가능성과 계약 갱신 시점",
			"5. 대체 공급선, 재고 일수, 물류 리드타임을 포함한 구매 리스크",
			"",
			"## 부서별 액션",
			"- 구매팀: 나프타·벤젠·프로필렌 가격 4주 이동평균과 공급선별 리드타임을 매주 갱신합니다.",
			"- 영업팀: 고객군별 가격 전가 가능성을 높음/중간/낮음으로 분류하고, 근거 자료를 붙입니다.",
			"- 규제대응팀: PFAS, REACH, 제품 안전자료 요청을 고객사별 제출기한 표로 정리합니다.",
			"- 재무팀: 원료 10% 상승, 환율 3% 변동, 스프레드 5% 축소 시나리오로 EBITDA 민감도를 계산합니다.",
			"- 생산팀: 저마진 범용 제품과 고마진 특수소재의 생산 전환 가능 시간을 확인합니다.",
			"",
			"## 회의 결론 초안",
			"단기적으로는 가격 전가 근거표와 원료 헤지 한도를 확정하고, 중기적으로는 규제 민감 고객에게 대체 소재 제안서를 먼저 보냅니다. 원료 가격이 추가 상승하면 범용 제품 일부는 감산 검토 대상이며, 특수소재와 장기계약 고객은 우선 공급 유지 대상으로 둡니다.",
		}, "\n"),
		DeckBody: strings.Join([]string{
			"# 회의 목적",
			"원자재·스프레드·규제 리스크를 한 장의 의사결정표로 묶는다.",
			"# 오늘의 위험",
			"나프타 가격 상승, 에틸렌 스프레드 축소, PFAS/REACH 대응 부담.",
			"# 마진 영향",
			"PE/PP는 원가 민감도가 높고, 특수소재는 고객 가격 전가 가능성이 상대적으로 높다.",
			"# 고객 대응",
			"자동차·전자 고객은 근거표, 생활소재 고객은 대체 원료 제안서를 먼저 준비한다.",
			"# 구매 액션",
			"대체 공급선, 재고 일수, 헤지 한도, 물류 리드타임을 매주 갱신한다.",
			"# 규제 액션",
			"PFAS/REACH 문서 보강, 고객별 제출기한, 대체 소재 후보를 관리한다.",
			"# 의사결정",
			"가격 전가, 감산 검토, 제품 믹스 조정, 고객 안내 메일 발송 여부.",
			"# 다음 단계",
			"제품군별 민감도표를 실제 원가 데이터로 다시 계산하고, 고객별 안내 초안을 보낸다.",
		}, "\n\n"),
		SheetBody: strings.Join([]string{
			"|위험|현재 신호|마진 영향|담당|오늘 결정|다음 산출물|",
			"|---|---|---:|---|---|---|",
			"|나프타 상승|4주 평균 상승|87|구매팀|헤지 한도 확인|원료 가격 추적표|",
			"|에틸렌 스프레드 축소|범용 제품 약세|72|재무팀|감산 검토 기준|제품군별 마진표|",
			"|PFAS 규제|고객 문의 증가|64|규제대응팀|고객별 문서 기한|규제 제출 체크리스트|",
			"|물류 리드타임|항만 지연 가능|51|구매/물류|대체 공급선 확인|공급선 리스크표|",
			"|가격 전가|계약 갱신 분산|46|영업팀|우선 고객 선정|가격 인상 근거표|",
			"|자동차 고객|품질자료 요구|58|영업/품질|자료 제출 순서|고객별 Q&A|",
			"|전자 고객|규제 문서 요구|61|규제대응|대체 소재 제안|메일 초안|",
		}, "\n"),
		GraphBody: strings.Join([]string{
			"## 마진 워룸 흐름",
			"",
			"```mermaid",
			"flowchart LR",
			"  Feedstock[\"나프타·벤젠·프로필렌 가격\"] --> Cost[\"제품군별 원가 영향\"] --> Margin[\"마진 압박 점수\"]",
			"  Regulation[\"PFAS/REACH 규제 신호\"] --> Customer[\"고객별 문서·대체 소재 요구\"] --> Margin",
			"  Logistics[\"물류 리드타임\"] --> Supply[\"대체 공급선·재고 일수\"] --> Margin",
			"  Margin --> Decision[\"가격 전가 / 감산 / 제품 믹스 조정\"]",
			"  Decision --> Sales[\"고객 안내 메일·가격 인상 근거표\"]",
			"  Decision --> Finance[\"EBITDA 민감도표\"]",
			"```",
			"",
			"## 리스크 우선순위",
			"",
			"```mermaid",
			"quadrantChart",
			"  title 화학회사 회의 우선순위",
			"  x-axis 단기 영향 낮음 --> 단기 영향 높음",
			"  y-axis 대응 난이도 낮음 --> 대응 난이도 높음",
			"  quadrant-1 즉시 의사결정",
			"  quadrant-2 문서 보강",
			"  quadrant-3 관찰",
			"  quadrant-4 영업 대응",
			"  나프타 상승: [0.86, 0.72]",
			"  에틸렌 스프레드: [0.74, 0.66]",
			"  PFAS 규제: [0.63, 0.82]",
			"  물류 리드타임: [0.55, 0.48]",
			"  가격 전가: [0.68, 0.58]",
			"```",
			"",
			"## 회의에서 바로 볼 수치",
			"",
			"| 항목 | 점수 | 의미 | 우선 액션 |",
			"|---|---:|---|---|",
			"| 나프타 | 87 | 원가 압박 최상위 | 헤지 한도와 공급선 확인 |",
			"| 스프레드 | 72 | 범용 제품 마진 축소 | 감산/제품 믹스 검토 |",
			"| PFAS | 64 | 고객 문서 요구 증가 | 제출기한과 대체 소재표 작성 |",
			"| 물류 | 51 | 공급 불확실성 | 리드타임과 재고 일수 갱신 |",
			"| 가격전가 | 46 | 고객별 협상력 차이 | 우선 고객과 근거표 확정 |",
		}, "\n"),
	}
}

func assistantManufacturingQualityWorkflowSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "manufacturing_quality_incident",
		Title:    lang.T("assistant.workflow.manufacturing_quality.title"),
		Audience: lang.T("assistant.workflow.manufacturing_quality.audience"),
		Summary:  lang.T("assistant.workflow.manufacturing_quality.summary"),
		Highlights: []string{
			lang.T("assistant.workflow.manufacturing_quality.h1"),
			lang.T("assistant.workflow.manufacturing_quality.h2"),
			lang.T("assistant.workflow.manufacturing_quality.h3"),
			lang.T("assistant.workflow.manufacturing_quality.h4"),
		},
		Commands: []string{
			lang.T("assistant.workflow.manufacturing_quality.c1"),
			lang.T("assistant.workflow.manufacturing_quality.c2"),
			lang.T("assistant.workflow.manufacturing_quality.c3"),
		},
		DocBody:   lang.T("assistant.workflow.manufacturing_quality.doc"),
		DeckBody:  lang.T("assistant.workflow.manufacturing_quality.deck"),
		SheetBody: lang.T("assistant.workflow.manufacturing_quality.sheet"),
		GraphBody: lang.T("assistant.workflow.manufacturing_quality.graph"),
	}
}

func assistantHealthcareClinicOpsWorkflowSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "healthcare_clinic_ops",
		Title:    lang.T("assistant.workflow.healthcare_clinic.title"),
		Audience: lang.T("assistant.workflow.healthcare_clinic.audience"),
		Summary:  lang.T("assistant.workflow.healthcare_clinic.summary"),
		Highlights: []string{
			lang.T("assistant.workflow.healthcare_clinic.h1"),
			lang.T("assistant.workflow.healthcare_clinic.h2"),
			lang.T("assistant.workflow.healthcare_clinic.h3"),
			lang.T("assistant.workflow.healthcare_clinic.h4"),
		},
		Commands: []string{
			lang.T("assistant.workflow.healthcare_clinic.c1"),
			lang.T("assistant.workflow.healthcare_clinic.c2"),
			lang.T("assistant.workflow.healthcare_clinic.c3"),
		},
		DocBody:   lang.T("assistant.workflow.healthcare_clinic.doc"),
		DeckBody:  lang.T("assistant.workflow.healthcare_clinic.deck"),
		SheetBody: lang.T("assistant.workflow.healthcare_clinic.sheet"),
		GraphBody: lang.T("assistant.workflow.healthcare_clinic.graph"),
	}
}

func assistantLogisticsDeliveryWorkflowSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "logistics_delivery_ops",
		Title:    lang.T("assistant.workflow.logistics_delivery.title"),
		Audience: lang.T("assistant.workflow.logistics_delivery.audience"),
		Summary:  lang.T("assistant.workflow.logistics_delivery.summary"),
		Highlights: []string{
			lang.T("assistant.workflow.logistics_delivery.h1"),
			lang.T("assistant.workflow.logistics_delivery.h2"),
			lang.T("assistant.workflow.logistics_delivery.h3"),
			lang.T("assistant.workflow.logistics_delivery.h4"),
		},
		Commands: []string{
			lang.T("assistant.workflow.logistics_delivery.c1"),
			lang.T("assistant.workflow.logistics_delivery.c2"),
			lang.T("assistant.workflow.logistics_delivery.c3"),
		},
		DocBody:   lang.T("assistant.workflow.logistics_delivery.doc"),
		DeckBody:  lang.T("assistant.workflow.logistics_delivery.deck"),
		SheetBody: lang.T("assistant.workflow.logistics_delivery.sheet"),
		GraphBody: lang.T("assistant.workflow.logistics_delivery.graph"),
	}
}

func assistantRetailFranchiseWorkflowSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "retail_franchise_ops",
		Title:    lang.T("assistant.workflow.retail_franchise.title"),
		Audience: lang.T("assistant.workflow.retail_franchise.audience"),
		Summary:  lang.T("assistant.workflow.retail_franchise.summary"),
		Highlights: []string{
			lang.T("assistant.workflow.retail_franchise.h1"),
			lang.T("assistant.workflow.retail_franchise.h2"),
			lang.T("assistant.workflow.retail_franchise.h3"),
			lang.T("assistant.workflow.retail_franchise.h4"),
		},
		Commands: []string{
			lang.T("assistant.workflow.retail_franchise.c1"),
			lang.T("assistant.workflow.retail_franchise.c2"),
			lang.T("assistant.workflow.retail_franchise.c3"),
		},
		DocBody:   lang.T("assistant.workflow.retail_franchise.doc"),
		DeckBody:  lang.T("assistant.workflow.retail_franchise.deck"),
		SheetBody: lang.T("assistant.workflow.retail_franchise.sheet"),
		GraphBody: lang.T("assistant.workflow.retail_franchise.graph"),
	}
}

func assistantCustomerSuccessRetentionWorkflowSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "customer_success_retention",
		Title:    lang.T("assistant.workflow.customer_success.title"),
		Audience: lang.T("assistant.workflow.customer_success.audience"),
		Summary:  lang.T("assistant.workflow.customer_success.summary"),
		Highlights: []string{
			lang.T("assistant.workflow.customer_success.h1"),
			lang.T("assistant.workflow.customer_success.h2"),
			lang.T("assistant.workflow.customer_success.h3"),
			lang.T("assistant.workflow.customer_success.h4"),
		},
		Commands: []string{
			lang.T("assistant.workflow.customer_success.c1"),
			lang.T("assistant.workflow.customer_success.c2"),
			lang.T("assistant.workflow.customer_success.c3"),
		},
		DocBody:   lang.T("assistant.workflow.customer_success.doc"),
		DeckBody:  lang.T("assistant.workflow.customer_success.deck"),
		SheetBody: lang.T("assistant.workflow.customer_success.sheet"),
		GraphBody: lang.T("assistant.workflow.customer_success.graph"),
	}
}

func assistantEducationAcademyWorkflowSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "education_academy_ops",
		Title:    lang.T("assistant.workflow.education_academy.title"),
		Audience: lang.T("assistant.workflow.education_academy.audience"),
		Summary:  lang.T("assistant.workflow.education_academy.summary"),
		Highlights: []string{
			lang.T("assistant.workflow.education_academy.h1"),
			lang.T("assistant.workflow.education_academy.h2"),
			lang.T("assistant.workflow.education_academy.h3"),
			lang.T("assistant.workflow.education_academy.h4"),
		},
		Commands: []string{
			lang.T("assistant.workflow.education_academy.c1"),
			lang.T("assistant.workflow.education_academy.c2"),
			lang.T("assistant.workflow.education_academy.c3"),
		},
		DocBody:   lang.T("assistant.workflow.education_academy.doc"),
		DeckBody:  lang.T("assistant.workflow.education_academy.deck"),
		SheetBody: lang.T("assistant.workflow.education_academy.sheet"),
		GraphBody: lang.T("assistant.workflow.education_academy.graph"),
	}
}

func assistantBookingCandidateWorkflowSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "booking_candidate_package",
		Title:    "예약 후보 비교·문의 패키지",
		Audience: "개인 사용자, 비서, 총무/운영 담당자",
		Summary:  "식당·병원·호텔 예약 후보를 위치, 시간, 총액, 취소조건, 전화/메시지 문구까지 비교하고 최종 확정 전 멈추는 패키지를 만들었습니다.",
		Highlights: []string{
			"후보 3개: 위치, 이동시간, 예약 가능 시간, 예상 비용, 취소 조건을 한 화면에서 비교",
			"전화/메시지 초안: 예약자명과 연락처를 넣기 전에도 바로 읽을 수 있는 문장 제공",
			"확정 전 확인: 날짜, 시간, 인원, 예약금, 취소 기한, 자리 요청, 알레르기/특이사항 점검",
			"최종 예약 확정, 결제, 개인정보 제출은 별도 승인 전에는 진행하지 않음",
		},
		Commands: []string{
			"첫 번째 후보로 전화용 짧은 예약 문구 만들어줘",
			"예약자명은 홍길동, 연락처는 010-1234-5678로 반영해줘",
			"이 후보표를 병원 예약 후보 비교표로 바꿔줘",
		},
		DocBody: strings.Join([]string{
			"# 예약 후보 비교·문의 패키지",
			"",
			"## 요청 상황",
			"예시 조건은 내일 저녁 7시, 강남 파스타 식당, 2명 방문입니다. 실제 요청이 식당, 병원, 호텔, 출장 예약 중 무엇이든 후보 비교표와 문의 문구를 같은 구조로 만들 수 있습니다.",
			"",
			"## 후보 비교",
			"1. 후보 A: 강남역 5분, 내일 19:00 가능성 높음, 예약금 없음, 창가석 요청 가능 여부 확인",
			"2. 후보 B: 신논현역 7분, 내일 19:30 대안, 콜키지/취소 기한 확인 필요",
			"3. 후보 C: 역삼역 9분, 조용한 좌석 가능성 높음, 예약금 또는 노쇼 정책 확인 필요",
			"",
			"## 전화/메시지 초안",
			"안녕하세요. 내일 저녁 7시에 2명 방문 예약이 가능한지 문의드립니다. 가능하다면 예약 가능한 시간, 예약금 여부, 취소 기한, 창가석이나 조용한 좌석 요청 가능 여부도 함께 확인 부탁드립니다.",
			"",
			"## 예약 전 확인",
			"- 날짜/시간: 내일 저녁 7시가 가능한지, 대안 시간이 필요한지 확인",
			"- 인원: 2명 기준 좌석과 테이블 타입 확인",
			"- 총액/예약금: 예약금, 노쇼 수수료, 최소 주문 조건 확인",
			"- 취소조건: 무료 취소 가능 시간과 변경 가능 여부 확인",
			"- 특이사항: 알레르기, 주차, 유아/휠체어, 조용한 좌석 요청 확인",
			"",
			"## 멈춤선",
			"후보 검색, 비교, 전화/메시지 문구 작성까지 진행합니다. 실제 예약 확정, 예약금 결제, 개인정보 제출은 사용자가 최종 승인하기 전에는 진행하지 않습니다.",
		}, "\n"),
		DeckBody: strings.Join([]string{
			"# 예약 조건",
			"내일 저녁 7시, 강남 파스타 식당, 2명 방문 기준으로 후보를 비교합니다.",
			"# 후보 A",
			"강남역 5분, 19:00 가능성 높음, 예약금 없음 예상, 창가석 요청 여부 확인.",
			"# 후보 B",
			"신논현역 7분, 19:30 대안, 가격은 낮지만 취소 기한과 콜키지 확인 필요.",
			"# 후보 C",
			"역삼역 9분, 조용한 좌석 가능성 높음, 노쇼 정책과 예약금 확인 필요.",
			"# 문의 문구",
			"날짜, 시간, 인원, 예약금, 취소 기한, 좌석 요청 가능 여부를 한 번에 묻습니다.",
			"# 최종 확인",
			"예약 확정, 결제, 개인정보 제출은 사용자 최종 승인 전에는 진행하지 않습니다.",
		}, "\n\n"),
		SheetBody: strings.Join([]string{
			"|후보|위치|가능 시간|예상 비용|취소/예약금|문의 포인트|판정|",
			"|---|---|---|---:|---|---|---|",
			"|후보 A|강남역 5분|19:00 가능성 높음|70000|예약금 없음 예상|창가석/조용한 좌석|1순위|",
			"|후보 B|신논현역 7분|19:30 대안|65000|취소 기한 확인|콜키지/주차|2순위|",
			"|후보 C|역삼역 9분|20:00 대안|80000|노쇼 정책 확인|조용한 좌석|보류|",
			"|전화 문구|공통|내일 저녁 7시 2명|0|확정 전 확인|예약금/취소/좌석 요청|준비 완료|",
		}, "\n"),
		GraphBody: strings.Join([]string{
			"## 예약 후보 처리 흐름",
			"",
			"```mermaid",
			"flowchart LR",
			"  Request[\"예약 조건\\n날짜·시간·인원·지역\"] --> Search[\"지도/예약 후보 검색\"] --> Compare[\"위치·시간·총액·취소조건 비교\"]",
			"  Compare --> Script[\"전화/메시지 초안\"] --> Confirm[\"날짜·시간·인원·예약금 확인\"]",
			"  Confirm --> Stop[\"최종 예약 확정 전 멈춤\"]",
			"  Stop --> Approval[\"사용자 최종 승인 후 진행\"]",
			"```",
			"",
			"## 후보 우선순위",
			"",
			"```mermaid",
			"quadrantChart",
			"  title 예약 후보 우선순위",
			"  x-axis 접근성 낮음 --> 접근성 높음",
			"  y-axis 조건 불확실 --> 조건 명확",
			"  quadrant-1 우선 문의",
			"  quadrant-2 조건 확인",
			"  quadrant-3 보류",
			"  quadrant-4 대안 후보",
			"  후보 A: [0.86, 0.82]",
			"  후보 B: [0.74, 0.66]",
			"  후보 C: [0.62, 0.58]",
			"```",
			"",
			"## 예약 전 확인표",
			"",
			"| 항목 | 확인값 | 멈춤 기준 |",
			"|---|---|---|",
			"| 날짜/시간 | 내일 19:00 | 대안 시간 필요 시 재확인 |",
			"| 인원 | 2명 | 좌석 타입 확인 |",
			"| 예약금 | 확인 필요 | 결제 전 승인 필요 |",
			"| 취소 기한 | 확인 필요 | 무료 취소 가능 시간 확인 |",
			"| 최종 확정 | 미실행 | 사용자 승인 전 중지 |",
		}, "\n"),
	}
}

func assistantFinanceExpenseWorkflowSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "finance_expense_reconciliation",
		Title:    "법인카드·영수증 정산 패키지",
		Audience: "재무팀, 총무팀, 팀장, 비용 승인 담당자",
		Summary:  "법인카드 사용 내역과 영수증을 정산표, 누락증빙 체크리스트, 결재 요청 초안, 비용 그래프로 묶었습니다. 실제 결제·송금·승인 실행은 하지 않습니다.",
		Highlights: []string{
			"비용 분류: 교통, 식대, SaaS 구독, 접대, 사무용품을 계정과목 후보로 정리",
			"누락증빙: 영수증 이미지, 참석자, 사용 목적, 승인자 메모가 필요한 항목을 분리",
			"결재 요청 초안: 팀장이 바로 읽고 승인 여부를 판단할 수 있게 요약",
			"정산 마감: 이번 주 금요일 17시까지 보완할 항목과 담당자를 지정",
		},
		Commands: []string{
			"누락증빙 3건만 직원별 요청 메시지로 바꿔줘",
			"접대비 항목만 팀장 결재 요청 초안으로 줄여줘",
			"이 정산표를 월말 비용 보고 PPT 3장으로 압축해줘",
		},
		DocBody: strings.Join([]string{
			"# 법인카드·영수증 정산 패키지",
			"",
			"## 정산 요약",
			"이번 샘플 정산은 법인카드 사용 내역 18건, 총 400만원 기준입니다. 교통비와 식대는 대부분 바로 정산 가능하지만, 접대비 2건과 SaaS 구독 1건은 사용 목적과 승인자 메모가 필요합니다.",
			"",
			"## 비용 분류",
			"- 교통: 택시, KTX, 주차비. 출장 목적과 방문처를 메모하면 정산 가능",
			"- 식대: 회의 식대와 야근 식대. 참석자 또는 프로젝트명을 추가 확인",
			"- SaaS 구독: 월간 구독료. 팀 사용 목적과 계약 소유자 확인",
			"- 접대: 고객 미팅 식대. 참석자, 회사명, 목적, 사전 승인 여부 확인",
			"- 사무용품: 키보드, 케이블, 소모품. 수령자와 사용 팀 확인",
			"",
			"## 누락증빙 체크",
			"1. 접대비 A: 참석자와 고객사명을 추가 요청",
			"2. SaaS 구독 B: 사용 부서와 비용 귀속 프로젝트 확인",
			"3. 택시비 C: 야근 또는 고객 방문 목적 확인",
			"",
			"## 결재 요청 초안",
			"이번 법인카드 정산은 총 400만원이며, 즉시 정산 가능 316만원, 보완 필요 84만원입니다. 보완 대상은 접대비와 SaaS 구독 중심입니다. 금요일 17시까지 누락증빙이 들어오면 월말 마감에 반영할 수 있습니다.",
			"",
			"## 멈춤선",
			"정산표 작성, 누락증빙 요청 문구, 결재 요청 초안까지 진행합니다. 실제 결제, 송금, 회계 승인, 카드 한도 변경은 명시 승인 전에는 실행하지 않습니다.",
		}, "\n"),
		DeckBody: strings.Join([]string{
			"# 정산 요약",
			"법인카드 18건, 총 400만원. 즉시 정산 가능 316만원, 보완 필요 84만원.",
			"# 비용 구성",
			"교통 148만원, 식대 92만원, SaaS 구독 76만원, 접대 61만원, 누락증빙 23만원.",
			"# 누락증빙",
			"접대비는 참석자와 고객사명, SaaS 구독은 사용 부서와 프로젝트 확인 필요.",
			"# 결재 요청",
			"팀장은 보완 필요 3건만 확인하면 월말 마감에 반영 가능.",
			"# 직원 요청 문구",
			"영수증 이미지, 사용 목적, 참석자, 승인자 메모를 금요일 17시까지 요청.",
			"# 실행 경계",
			"정산표와 결재 초안까지만 작성. 실제 결제·송금·승인은 별도 승인 후 진행.",
		}, "\n\n"),
		SheetBody: strings.Join([]string{
			"|항목|금액|계정과목 후보|증빙 상태|담당|다음 조치|",
			"|---|---:|---|---|---|---|",
			"|택시/KTX/주차|1480000|교통비|목적 일부 필요|총무|출장 목적 보완 요청|",
			"|회의/야근 식대|920000|복리후생비/회의비|대체로 완비|재무|프로젝트명만 확인|",
			"|SaaS 구독|760000|소프트웨어 사용료|사용 부서 필요|IT/재무|계약 소유자 확인|",
			"|고객 접대 식대|610000|접대비|참석자/고객사 필요|영업|참석자와 목적 요청|",
			"|사무용품|230000|소모품비|수령자 필요|총무|수령자 확인|",
			"|즉시 정산 가능|3160000|혼합|완비|재무|월말 정산 반영|",
			"|보완 필요|840000|혼합|누락 있음|담당자|금요일 17시 마감|",
		}, "\n"),
		GraphBody: strings.Join([]string{
			"## 법인카드 정산 흐름",
			"",
			"```mermaid",
			"flowchart LR",
			"  Card[\"법인카드 내역\"] --> Classify[\"계정과목 분류\"] --> Evidence[\"영수증·참석자·목적 확인\"]",
			"  Evidence --> Ready[\"즉시 정산 가능\"]",
			"  Evidence --> Missing[\"누락증빙 요청\"]",
			"  Missing --> Review[\"팀장 결재 검토\"]",
			"  Ready --> Report[\"월말 비용 보고\"]",
			"  Review --> Report",
			"  Report --> Stop[\"실제 결제·송금·승인 전 멈춤\"]",
			"```",
			"",
			"## 비용 위험도",
			"",
			"```mermaid",
			"quadrantChart",
			"  title 정산 우선순위",
			"  x-axis 금액 낮음 --> 금액 높음",
			"  y-axis 증빙 단순 --> 증빙 복잡",
			"  quadrant-1 우선 검토",
			"  quadrant-2 큰 금액 정산",
			"  quadrant-3 자동 정산",
			"  quadrant-4 보완 요청",
			"  교통: [0.82, 0.36]",
			"  식대: [0.61, 0.48]",
			"  SaaS: [0.55, 0.72]",
			"  접대: [0.48, 0.84]",
			"  사무용품: [0.22, 0.42]",
			"```",
			"",
			"## 결재 전 확인표",
			"",
			"| 항목 | 상태 | 다음 행동 |",
			"|---|---|---|",
			"| 즉시 정산 가능 | 316만원 | 월말 보고 반영 |",
			"| 보완 필요 | 84만원 | 직원별 증빙 요청 |",
			"| 접대비 | 참석자 누락 | 고객사/참석자/목적 확인 |",
			"| SaaS 구독 | 귀속 부서 확인 필요 | 계약 소유자 확인 |",
			"| 실제 승인 | 미실행 | 사용자 명시 승인 전 중지 |",
		}, "\n"),
	}
}

func assistantFinanceARCollectionWorkflowSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "finance_ar_cash_collection",
		Title:    lang.T("assistant.workflow.finance_ar.title"),
		Audience: lang.T("assistant.workflow.finance_ar.audience"),
		Summary:  lang.T("assistant.workflow.finance_ar.summary"),
		Highlights: []string{
			lang.T("assistant.workflow.finance_ar.h1"),
			lang.T("assistant.workflow.finance_ar.h2"),
			lang.T("assistant.workflow.finance_ar.h3"),
			lang.T("assistant.workflow.finance_ar.h4"),
		},
		Commands: []string{
			lang.T("assistant.workflow.finance_ar.c1"),
			lang.T("assistant.workflow.finance_ar.c2"),
			lang.T("assistant.workflow.finance_ar.c3"),
		},
		DocBody:   lang.T("assistant.workflow.finance_ar.doc"),
		DeckBody:  lang.T("assistant.workflow.finance_ar.deck"),
		SheetBody: lang.T("assistant.workflow.finance_ar.sheet"),
		GraphBody: lang.T("assistant.workflow.finance_ar.graph"),
	}
}

func assistantProcurementVendorWorkflowSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "procurement_vendor_selection",
		Title:    lang.T("assistant.workflow.procurement.title"),
		Audience: lang.T("assistant.workflow.procurement.audience"),
		Summary:  lang.T("assistant.workflow.procurement.summary"),
		Highlights: []string{
			lang.T("assistant.workflow.procurement.h1"),
			lang.T("assistant.workflow.procurement.h2"),
			lang.T("assistant.workflow.procurement.h3"),
			lang.T("assistant.workflow.procurement.h4"),
		},
		Commands: []string{
			lang.T("assistant.workflow.procurement.c1"),
			lang.T("assistant.workflow.procurement.c2"),
			lang.T("assistant.workflow.procurement.c3"),
		},
		DocBody:   lang.T("assistant.workflow.procurement.doc"),
		DeckBody:  lang.T("assistant.workflow.procurement.deck"),
		SheetBody: lang.T("assistant.workflow.procurement.sheet"),
		GraphBody: lang.T("assistant.workflow.procurement.graph"),
	}
}

func assistantCommunicationsCrisisWorkflowSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "communications_crisis_response",
		Title:    lang.T("assistant.workflow.comms_crisis.title"),
		Audience: lang.T("assistant.workflow.comms_crisis.audience"),
		Summary:  lang.T("assistant.workflow.comms_crisis.summary"),
		Highlights: []string{
			lang.T("assistant.workflow.comms_crisis.h1"),
			lang.T("assistant.workflow.comms_crisis.h2"),
			lang.T("assistant.workflow.comms_crisis.h3"),
			lang.T("assistant.workflow.comms_crisis.h4"),
		},
		Commands: []string{
			lang.T("assistant.workflow.comms_crisis.c1"),
			lang.T("assistant.workflow.comms_crisis.c2"),
			lang.T("assistant.workflow.comms_crisis.c3"),
		},
		DocBody:   lang.T("assistant.workflow.comms_crisis.doc"),
		DeckBody:  lang.T("assistant.workflow.comms_crisis.deck"),
		SheetBody: lang.T("assistant.workflow.comms_crisis.sheet"),
		GraphBody: lang.T("assistant.workflow.comms_crisis.graph"),
	}
}

func assistantITOffboardingWorkflowSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "it_access_offboarding",
		Title:    lang.T("assistant.workflow.it_offboarding.title"),
		Audience: lang.T("assistant.workflow.it_offboarding.audience"),
		Summary:  lang.T("assistant.workflow.it_offboarding.summary"),
		Highlights: []string{
			lang.T("assistant.workflow.it_offboarding.h1"),
			lang.T("assistant.workflow.it_offboarding.h2"),
			lang.T("assistant.workflow.it_offboarding.h3"),
			lang.T("assistant.workflow.it_offboarding.h4"),
		},
		Commands: []string{
			lang.T("assistant.workflow.it_offboarding.c1"),
			lang.T("assistant.workflow.it_offboarding.c2"),
			lang.T("assistant.workflow.it_offboarding.c3"),
		},
		DocBody:   lang.T("assistant.workflow.it_offboarding.doc"),
		DeckBody:  lang.T("assistant.workflow.it_offboarding.deck"),
		SheetBody: lang.T("assistant.workflow.it_offboarding.sheet"),
		GraphBody: lang.T("assistant.workflow.it_offboarding.graph"),
	}
}

func assistantInsuranceClaimsWorkflowSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "insurance_claims_triage",
		Title:    lang.T("assistant.workflow.insurance_claims.title"),
		Audience: lang.T("assistant.workflow.insurance_claims.audience"),
		Summary:  lang.T("assistant.workflow.insurance_claims.summary"),
		Highlights: []string{
			lang.T("assistant.workflow.insurance_claims.h1"),
			lang.T("assistant.workflow.insurance_claims.h2"),
			lang.T("assistant.workflow.insurance_claims.h3"),
			lang.T("assistant.workflow.insurance_claims.h4"),
		},
		Commands: []string{
			lang.T("assistant.workflow.insurance_claims.c1"),
			lang.T("assistant.workflow.insurance_claims.c2"),
			lang.T("assistant.workflow.insurance_claims.c3"),
		},
		DocBody:   lang.T("assistant.workflow.insurance_claims.doc"),
		DeckBody:  lang.T("assistant.workflow.insurance_claims.deck"),
		SheetBody: lang.T("assistant.workflow.insurance_claims.sheet"),
		GraphBody: lang.T("assistant.workflow.insurance_claims.graph"),
	}
}

func assistantConstructionSiteWorkflowSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "construction_site_control",
		Title:    lang.T("assistant.workflow.construction_site.title"),
		Audience: lang.T("assistant.workflow.construction_site.audience"),
		Summary:  lang.T("assistant.workflow.construction_site.summary"),
		Highlights: []string{
			lang.T("assistant.workflow.construction_site.h1"),
			lang.T("assistant.workflow.construction_site.h2"),
			lang.T("assistant.workflow.construction_site.h3"),
			lang.T("assistant.workflow.construction_site.h4"),
		},
		Commands: []string{
			lang.T("assistant.workflow.construction_site.c1"),
			lang.T("assistant.workflow.construction_site.c2"),
			lang.T("assistant.workflow.construction_site.c3"),
		},
		DocBody:   lang.T("assistant.workflow.construction_site.doc"),
		DeckBody:  lang.T("assistant.workflow.construction_site.deck"),
		SheetBody: lang.T("assistant.workflow.construction_site.sheet"),
		GraphBody: lang.T("assistant.workflow.construction_site.graph"),
	}
}

func assistantLegalDisputeWorkflowSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "legal_dispute_case_prep",
		Title:    lang.T("assistant.workflow.legal_dispute.title"),
		Audience: lang.T("assistant.workflow.legal_dispute.audience"),
		Summary:  lang.T("assistant.workflow.legal_dispute.summary"),
		Highlights: []string{
			lang.T("assistant.workflow.legal_dispute.h1"),
			lang.T("assistant.workflow.legal_dispute.h2"),
			lang.T("assistant.workflow.legal_dispute.h3"),
			lang.T("assistant.workflow.legal_dispute.h4"),
		},
		Commands: []string{
			lang.T("assistant.workflow.legal_dispute.c1"),
			lang.T("assistant.workflow.legal_dispute.c2"),
			lang.T("assistant.workflow.legal_dispute.c3"),
		},
		DocBody:   lang.T("assistant.workflow.legal_dispute.doc"),
		DeckBody:  lang.T("assistant.workflow.legal_dispute.deck"),
		SheetBody: lang.T("assistant.workflow.legal_dispute.sheet"),
		GraphBody: lang.T("assistant.workflow.legal_dispute.graph"),
	}
}

func assistantExecutiveDecisionWorkflowSpec() assistantDepartmentWorkflowSpec {
	prefix := "assistant.workflow.executive_decision."
	return assistantStructuredDepartmentWorkflowSpec(assistantStructuredWorkflowSpec{
		Kind:        "executive_decision_pack",
		TitleKey:    prefix + "title",
		AudienceKey: prefix + "audience",
		SummaryKey:  prefix + "summary",
		HighlightKeys: []string{
			prefix + "h1",
			prefix + "h2",
			prefix + "h3",
			prefix + "h4",
		},
		CommandKeys: []string{
			prefix + "c1",
			prefix + "c2",
			prefix + "c3",
		},
		Sections: []assistantStructuredWorkflowSection{
			{TitleKey: prefix + "section.context", BodyKeys: []string{
				prefix + "section.context.1",
				prefix + "section.context.2",
				prefix + "section.context.3",
			}},
			{TitleKey: prefix + "section.options", BodyKeys: []string{
				prefix + "section.options.1",
				prefix + "section.options.2",
				prefix + "section.options.3",
				prefix + "section.options.4",
			}},
			{TitleKey: prefix + "section.risk", BodyKeys: []string{
				prefix + "section.risk.1",
				prefix + "section.risk.2",
				prefix + "section.risk.3",
			}},
			{TitleKey: prefix + "section.stop", BodyKeys: []string{
				prefix + "section.stop.1",
				prefix + "section.stop.2",
			}},
		},
		DeckSlides: []assistantStructuredWorkflowSlide{
			{TitleKey: prefix + "slide.conclusion", BodyKeys: []string{prefix + "slide.conclusion.1", prefix + "slide.conclusion.2"}},
			{TitleKey: prefix + "slide.options", BodyKeys: []string{prefix + "section.options.1", prefix + "section.options.2", prefix + "section.options.3", prefix + "section.options.4"}},
			{TitleKey: prefix + "slide.risk", BodyKeys: []string{prefix + "section.risk.1", prefix + "section.risk.2", prefix + "section.risk.3"}},
			{TitleKey: prefix + "slide.actions", BodyKeys: []string{prefix + "slide.actions.1", prefix + "slide.actions.2"}},
			{TitleKey: prefix + "slide.boundary", BodyKeys: []string{prefix + "section.stop.1", prefix + "section.stop.2"}},
		},
		SheetHeaders: []string{
			prefix + "sheet.agenda",
			prefix + "sheet.option",
			prefix + "sheet.impact",
			prefix + "sheet.risk",
			prefix + "sheet.owner",
			prefix + "sheet.next",
			prefix + "sheet.stop",
		},
		SheetRows: [][]string{
			{prefix + "row.price", prefix + "row.price.option", prefix + "row.price.impact", prefix + "row.price.risk", prefix + "row.price.owner", prefix + "row.price.next", prefix + "row.price.stop"},
			{prefix + "row.budget", prefix + "row.budget.option", prefix + "row.budget.impact", prefix + "row.budget.risk", prefix + "row.budget.owner", prefix + "row.budget.next", prefix + "row.budget.stop"},
			{prefix + "row.launch", prefix + "row.launch.option", prefix + "row.launch.impact", prefix + "row.launch.risk", prefix + "row.launch.owner", prefix + "row.launch.next", prefix + "row.launch.stop"},
			{prefix + "row.staffing", prefix + "row.staffing.option", prefix + "row.staffing.impact", prefix + "row.staffing.risk", prefix + "row.staffing.owner", prefix + "row.staffing.next", prefix + "row.staffing.stop"},
		},
		FlowTitleKey: prefix + "flow.title",
		FlowNodes: []assistantStructuredWorkflowNode{
			{ID: "Context", LabelKey: prefix + "flow.context"},
			{ID: "Options", LabelKey: prefix + "flow.options"},
			{ID: "Numbers", LabelKey: prefix + "flow.numbers"},
			{ID: "Risk", LabelKey: prefix + "flow.risk"},
			{ID: "Decision", LabelKey: prefix + "flow.decision"},
			{ID: "Approval", LabelKey: prefix + "flow.approval"},
			{ID: "Actions", LabelKey: prefix + "flow.actions"},
		},
		FlowEdges: []assistantStructuredWorkflowEdge{
			{From: "Context", To: "Options"},
			{From: "Options", To: "Numbers"},
			{From: "Numbers", To: "Risk"},
			{From: "Risk", To: "Decision"},
			{From: "Decision", To: "Approval"},
			{From: "Approval", To: "Actions"},
		},
		Quadrant: assistantStructuredWorkflowQuadrant{
			TitleKey: prefix + "quadrant.title",
			XAxisKey: prefix + "quadrant.x",
			YAxisKey: prefix + "quadrant.y",
			Q1Key:    prefix + "quadrant.q1",
			Q2Key:    prefix + "quadrant.q2",
			Q3Key:    prefix + "quadrant.q3",
			Q4Key:    prefix + "quadrant.q4",
			Points: []assistantStructuredWorkflowPoint{
				{LabelKey: prefix + "point.price", X: 0.82, Y: 0.54},
				{LabelKey: prefix + "point.budget", X: 0.78, Y: 0.72},
				{LabelKey: prefix + "point.launch", X: 0.90, Y: 0.62},
				{LabelKey: prefix + "point.staffing", X: 0.66, Y: 0.82},
			},
		},
		ChecklistTitle: prefix + "checklist.title",
		ChecklistHeader: []string{
			prefix + "checklist.action",
			prefix + "checklist.ready",
			prefix + "checklist.stop",
		},
		ChecklistRows: [][]string{
			{prefix + "checklist.price.action", prefix + "checklist.price.ready", prefix + "checklist.price.stop"},
			{prefix + "checklist.budget.action", prefix + "checklist.budget.ready", prefix + "checklist.budget.stop"},
			{prefix + "checklist.launch.action", prefix + "checklist.launch.ready", prefix + "checklist.launch.stop"},
			{prefix + "checklist.staffing.action", prefix + "checklist.staffing.ready", prefix + "checklist.staffing.stop"},
		},
	})
}

func assistantSalesRFPWorkflowSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "sales_rfp_response",
		Title:    lang.T("assistant.workflow.sales_rfp.title"),
		Audience: lang.T("assistant.workflow.sales_rfp.audience"),
		Summary:  lang.T("assistant.workflow.sales_rfp.summary"),
		Highlights: []string{
			lang.T("assistant.workflow.sales_rfp.h1"),
			lang.T("assistant.workflow.sales_rfp.h2"),
			lang.T("assistant.workflow.sales_rfp.h3"),
			lang.T("assistant.workflow.sales_rfp.h4"),
		},
		Commands: []string{
			lang.T("assistant.workflow.sales_rfp.c1"),
			lang.T("assistant.workflow.sales_rfp.c2"),
			lang.T("assistant.workflow.sales_rfp.c3"),
		},
		DocBody:   lang.T("assistant.workflow.sales_rfp.doc"),
		DeckBody:  lang.T("assistant.workflow.sales_rfp.deck"),
		SheetBody: lang.T("assistant.workflow.sales_rfp.sheet"),
		GraphBody: lang.T("assistant.workflow.sales_rfp.graph"),
	}
}

func assistantSupportTicketWorkflowSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "support_ticket_response",
		Title:    lang.T("assistant.workflow.support_ticket.title"),
		Audience: lang.T("assistant.workflow.support_ticket.audience"),
		Summary:  lang.T("assistant.workflow.support_ticket.summary"),
		Highlights: []string{
			lang.T("assistant.workflow.support_ticket.h1"),
			lang.T("assistant.workflow.support_ticket.h2"),
			lang.T("assistant.workflow.support_ticket.h3"),
			lang.T("assistant.workflow.support_ticket.h4"),
		},
		Commands: []string{
			lang.T("assistant.workflow.support_ticket.c1"),
			lang.T("assistant.workflow.support_ticket.c2"),
			lang.T("assistant.workflow.support_ticket.c3"),
		},
		DocBody:   lang.T("assistant.workflow.support_ticket.doc"),
		DeckBody:  lang.T("assistant.workflow.support_ticket.deck"),
		SheetBody: lang.T("assistant.workflow.support_ticket.sheet"),
		GraphBody: lang.T("assistant.workflow.support_ticket.graph"),
	}
}

func assistantProductPRDWorkflowSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "product_prd_release",
		Title:    lang.T("assistant.workflow.product_prd.title"),
		Audience: lang.T("assistant.workflow.product_prd.audience"),
		Summary:  lang.T("assistant.workflow.product_prd.summary"),
		Highlights: []string{
			lang.T("assistant.workflow.product_prd.h1"),
			lang.T("assistant.workflow.product_prd.h2"),
			lang.T("assistant.workflow.product_prd.h3"),
			lang.T("assistant.workflow.product_prd.h4"),
		},
		Commands: []string{
			lang.T("assistant.workflow.product_prd.c1"),
			lang.T("assistant.workflow.product_prd.c2"),
			lang.T("assistant.workflow.product_prd.c3"),
		},
		DocBody:   lang.T("assistant.workflow.product_prd.doc"),
		DeckBody:  lang.T("assistant.workflow.product_prd.deck"),
		SheetBody: lang.T("assistant.workflow.product_prd.sheet"),
		GraphBody: lang.T("assistant.workflow.product_prd.graph"),
	}
}

func assistantLegalContractWorkflowSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "legal_contract_review",
		Title:    lang.T("assistant.workflow.legal_contract.title"),
		Audience: lang.T("assistant.workflow.legal_contract.audience"),
		Summary:  lang.T("assistant.workflow.legal_contract.summary"),
		Highlights: []string{
			lang.T("assistant.workflow.legal_contract.h1"),
			lang.T("assistant.workflow.legal_contract.h2"),
			lang.T("assistant.workflow.legal_contract.h3"),
			lang.T("assistant.workflow.legal_contract.h4"),
		},
		Commands: []string{
			lang.T("assistant.workflow.legal_contract.c1"),
			lang.T("assistant.workflow.legal_contract.c2"),
			lang.T("assistant.workflow.legal_contract.c3"),
		},
		DocBody:   lang.T("assistant.workflow.legal_contract.doc"),
		DeckBody:  lang.T("assistant.workflow.legal_contract.deck"),
		SheetBody: lang.T("assistant.workflow.legal_contract.sheet"),
		GraphBody: lang.T("assistant.workflow.legal_contract.graph"),
	}
}

func assistantOnboardingTrainingWorkflowSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "onboarding_training",
		Title:    lang.T("assistant.workflow.onboarding_training.title"),
		Audience: lang.T("assistant.workflow.onboarding_training.audience"),
		Summary:  lang.T("assistant.workflow.onboarding_training.summary"),
		Highlights: []string{
			lang.T("assistant.workflow.onboarding_training.h1"),
			lang.T("assistant.workflow.onboarding_training.h2"),
			lang.T("assistant.workflow.onboarding_training.h3"),
			lang.T("assistant.workflow.onboarding_training.h4"),
		},
		Commands: []string{
			lang.T("assistant.workflow.onboarding_training.c1"),
			lang.T("assistant.workflow.onboarding_training.c2"),
			lang.T("assistant.workflow.onboarding_training.c3"),
		},
		DocBody:   lang.T("assistant.workflow.onboarding_training.doc"),
		DeckBody:  lang.T("assistant.workflow.onboarding_training.deck"),
		SheetBody: lang.T("assistant.workflow.onboarding_training.sheet"),
		GraphBody: lang.T("assistant.workflow.onboarding_training.graph"),
	}
}

func assistantTravelPrepBundleSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "travel_prep_bundle",
		Title:    "후쿠오카 2박 3일 여행 준비 패키지",
		Audience: "출장자, 개인 여행자, 일정 조율 담당자",
		Summary:  "2박 3일 여행 준비물, 예산, 동선, 예약 전 확인표, 모바일용 PPT와 엑셀표를 만들었습니다.",
		Highlights: []string{
			"금요일 저녁 출발, 일요일 저녁 복귀 기준으로 업무 손실이 적은 2박 3일 일정",
			"항공·호텔·현지 교통·식비·비상예산을 분리한 총액 예산표",
			"공항, 하카타, 텐진, 다자이후 동선을 하루 단위로 묶은 이동 계획",
			"예약 확정 전 확인해야 할 이름, 날짜, 취소조건, 총액 체크리스트 포함",
		},
		Commands: []string{
			"이 여행안을 제주 2박 3일로 바꿔줘",
			"호텔 후보 3개를 가격/위치/취소조건 엑셀로 비교해줘",
			"이 여행 PPT를 가족 공유용 5장으로 줄여줘",
		},
		DocBody: strings.Join([]string{
			"# 후쿠오카 2박 3일 여행 준비 패키지",
			"",
			"## 여행 기준",
			"금요일 저녁 출발, 일요일 저녁 복귀를 기본안으로 잡습니다. 업무일 손실을 줄이고, 첫날은 이동과 체크인, 둘째 날은 핵심 일정, 셋째 날은 짧은 쇼핑과 복귀로 구성합니다.",
			"",
			"## 추천 일정",
			"### 1일차: 이동과 체크인",
			"- 서울 출발, 후쿠오카 도착",
			"- 하카타 또는 텐진 숙소 체크인",
			"- 숙소 근처 저녁 식사와 편의점/약국 위치 확인",
			"",
			"### 2일차: 핵심 관광과 여유 일정",
			"- 오전: 다자이후 또는 오호리공원",
			"- 오후: 텐진/하카타 쇼핑, 카페, 짧은 휴식",
			"- 저녁: 예약 후보 식당 방문",
			"",
			"### 3일차: 체크아웃과 복귀",
			"- 오전: 늦은 체크아웃 또는 짐 보관",
			"- 점심: 공항 이동 전 가벼운 식사",
			"- 오후/저녁: 공항 이동, 서울 복귀",
			"",
			"## 준비물",
			"- 여권, 항공 예약번호, 호텔 바우처",
			"- eSIM 또는 로밍, 보조배터리, 충전 케이블",
			"- 작은 우산, 편한 신발, 상비약",
			"- 해외 결제 카드, 현금 일부, 교통카드 앱",
			"",
			"## 예약 전 확인",
			"- 항공: 탑승자 이름, 출발/복귀 날짜, 수하물, 변경/취소 수수료",
			"- 호텔: 위치, 체크인 시간, 무료취소 기한, 2박 총액",
			"- 동선: 공항에서 숙소까지 이동수단, 첫날 늦은 체크인 가능 여부",
			"- 결제: 최종 총액과 환불 조건 확인 전에는 예약 확정하지 않음",
		}, "\n"),
		DeckBody: strings.Join([]string{
			"# 여행 목표",
			"업무일 손실을 줄이는 금요일 저녁 출발, 일요일 저녁 복귀 2박 3일.",
			"# 1일차",
			"이동, 하카타/텐진 체크인, 숙소 근처 저녁.",
			"# 2일차",
			"다자이후 또는 오호리공원, 텐진/하카타 쇼핑, 저녁 예약 후보.",
			"# 3일차",
			"늦은 체크아웃, 짐 보관, 공항 이동 전 짧은 일정.",
			"# 예산",
			"항공, 호텔, 교통, 식비, 쇼핑, 비상예산을 분리해서 본다.",
			"# 예약 전",
			"이름, 날짜, 총액, 수하물, 무료취소 기한을 확인하고 확정한다.",
		}, "\n\n"),
		SheetBody: strings.Join([]string{
			"|항목|예상 금액|확인할 것|상태|",
			"|---|---:|---|---|",
			"|왕복 항공|280000|출발/복귀 시간, 수하물, 변경 수수료|후보 비교|",
			"|호텔 2박|360000|하카타/텐진 위치, 무료취소, 체크인 시간|후보 비교|",
			"|현지 교통|60000|공항-숙소, 지하철/버스, 택시 예비비|예산 반영|",
			"|식비|180000|저녁 2회, 점심 2회, 카페/간식|예산 반영|",
			"|입장/체험|50000|다자이후/전망대/온천 후보|선택|",
			"|비상예산|100000|일정 변경, 우천, 약국/택시|필수|",
			"|합계|1030000|예약 전 최종 총액 재확인|검토 필요|",
		}, "\n"),
	}
}

func assistantShoppingDecisionWorkbookSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "shopping_decision_workbook",
		Title:    lang.T("assistant.workflow.shopping_decision.title"),
		Audience: lang.T("assistant.workflow.shopping_decision.audience"),
		Summary:  lang.T("assistant.workflow.shopping_decision.summary"),
		Highlights: []string{
			lang.T("assistant.workflow.shopping_decision.h1"),
			lang.T("assistant.workflow.shopping_decision.h2"),
			lang.T("assistant.workflow.shopping_decision.h3"),
			lang.T("assistant.workflow.shopping_decision.h4"),
		},
		Commands: []string{
			lang.T("assistant.workflow.shopping_decision.c1"),
			lang.T("assistant.workflow.shopping_decision.c2"),
			lang.T("assistant.workflow.shopping_decision.c3"),
		},
		DocBody:   lang.T("assistant.workflow.shopping_decision.doc"),
		DeckBody:  lang.T("assistant.workflow.shopping_decision.deck"),
		SheetBody: lang.T("assistant.workflow.shopping_decision.sheet"),
		GraphBody: lang.T("assistant.workflow.shopping_decision.graph"),
	}
}

func assistantMeetingMinutesPackSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "meeting_minutes_pack",
		Title:    "회의 메모 기반 실행 회의록 패키지",
		Audience: "회의 참석자, 팀장, PM, 임원 보고 담당자",
		Summary:  "회의 메모를 결정사항, 할 일, 담당자, 마감, 리스크, 다음 회의 안건이 보이는 회의록 패키지로 바꿨습니다.",
		Highlights: []string{
			"결정사항 4개, 액션아이템 7개, 리스크 5개를 분리한 실행형 회의록",
			"담당자와 마감일을 한눈에 볼 수 있는 XLSX/CSV 추적표 포함",
			"임원 공유용 5장 PPT에는 결정, 지연 위험, 다음 요청만 남김",
			"Signal 본문은 요약만, 첨부 파일은 회의록 원문과 추적표로 구성",
		},
		Commands: []string{
			"이 회의록을 임원 보고용 한 페이지로 줄여줘",
			"액션아이템만 담당자별 오늘 할 일로 나눠줘",
			"리스크만 따로 보고방 Ops에 보낼 문장으로 바꿔줘",
		},
		DocBody: strings.Join([]string{
			"# 회의 메모 기반 실행 회의록",
			"",
			"## 회의 목적",
			"Argos 비서 기능을 사용자가 바로 체감할 수 있도록, 런타임 설명보다 실제 결과물 중심의 시나리오를 빠르게 확장합니다.",
			"",
			"## 결정사항",
			"1. 내부 점검 보고보다 사용자 업무 결과물을 먼저 Signal에 보냅니다.",
			"2. 모바일에서 바로 열리는 DOCX, Markdown, PPTX, XLSX를 기본 산출물로 둡니다.",
			"3. 쇼핑과 예약은 검색, 비교, 장바구니 직전 준비까지 하고 최종 결제/예약은 승인 후 진행합니다.",
			"4. 업종별 예제는 마케팅, 영업, 인사, 공공기관, 여행, 쇼핑, 회의록 순서로 확장합니다.",
			"",
			"## 할 일",
			"- PM: 다음 데모에서 실제 사용자 문장 5개를 뽑아 Signal 테스트",
			"- 마케팅 담당: 시장조사/경쟁사 배틀카드 샘플 문구 정리",
			"- 영업 담당: 리드 20개 액션표 샘플 데이터 검토",
			"- 운영 담당: 보고방에는 번호 선택 없이 결과물만 보내도록 확인",
			"- 개발 담당: 회의록 패키지, 여행 패키지, 쇼핑 워크북 라우팅 테스트 유지",
			"",
			"## 리스크",
			"- 사용자가 실제 예약/구매를 기대할 때 최종 승인 경계가 불명확하면 혼란이 생깁니다.",
			"- 보고방에 내부 근거 경로가 반복되면 제품 가치가 약해 보입니다.",
			"- 가상 데이터와 실제 브라우저/검색 결과의 차이를 분명히 표시해야 합니다.",
			"- 모바일에서 PPTX/XLSX가 열리지 않으면 체감 가치가 떨어집니다.",
			"- 스케줄 backlog를 무리하게 catch-up하면 보고방이 스팸처럼 보일 수 있습니다.",
			"",
			"## 다음 회의 안건",
			"1. 실제 쿠팡 검색/비교/장바구니 직전 흐름 검증",
			"2. 브라우저 검색 기반 시장조사 보고서에 출처 표 붙이기",
			"3. 회의록을 자동으로 보고방에 보내는 스케줄 예제 구성",
		}, "\n"),
		DeckBody: strings.Join([]string{
			"# 회의 결론",
			"Argos는 내부 점검보다 사용자가 맡긴 일의 결과물을 먼저 보여줘야 한다.",
			"# 결정사항",
			"Signal 본문은 요약, 첨부는 DOCX/MD/PPTX/XLSX로 제공한다.",
			"# 액션아이템",
			"PM, 마케팅, 영업, 운영, 개발 담당별 다음 일을 분리한다.",
			"# 리스크",
			"최종 구매/예약 승인 경계, 내부 근거 노출, 모바일 파일 열림을 관리한다.",
			"# 다음 회의",
			"쿠팡 실제 검색, 브라우저 시장조사, 자동 회의록 보고를 검증한다.",
		}, "\n\n"),
		SheetBody: strings.Join([]string{
			"|구분|항목|담당|마감|상태|리스크|다음 액션|",
			"|---|---|---|---|---|---|---|",
			"|결정|결과물 중심 Signal 응답|PM|즉시|확정|내부 용어 과다|데모 문장 5개 선정|",
			"|결정|DOCX/MD/PPTX/XLSX 기본 첨부|개발|즉시|확정|모바일 열림 실패|iPhone에서 첨부 열기 확인|",
			"|할 일|시장조사/배틀카드 샘플 정리|마케팅|내일 오전|진행|가상/실제 데이터 혼동|출처 포함 버전 추가|",
			"|할 일|리드 20개 액션표 검토|영업|내일 오전|대기|샘플 데이터 빈약|업종별 리드 예시 보강|",
			"|할 일|보고방 번호 선택 제거 확인|운영|오늘|진행|보고방 상호작용 발생|one-way/no-reply 테스트|",
			"|리스크|최종 구매 승인 경계|개발|상시|관리|실수 클릭|구매 실행 승인 문구만 허용|",
			"|리스크|스케줄 backlog 스팸|운영|상시|관리|보고방 과다 발송|catch-up 수동 실행 금지|",
		}, "\n"),
	}
}

func assistantMarketingSalesWorkflowSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "marketing_sales_analysis",
		Title:    "마케팅 매출 분석 실행 패키지",
		Audience: "마케팅팀, 영업팀, 대표, 재무 담당자",
		Summary:  "월별 매출, 제품군별 성장, 리드 전환율, ROAS, 다음 액션을 보고서와 그래프용 표로 만들었습니다.",
		Highlights: []string{
			"제품군 A가 6개월 성장의 핵심 기여자로 나타나는 샘플 분석",
			"4월에는 광고비 대비 매출 정체가 발생해 채널 조정 필요",
			"검색광고와 리테일미디어 ROAS가 높고 SNS는 리타겟팅 개선 필요",
			"영업 퍼널은 제안 단계에서 병목이 커 사례자료 자동 생성이 필요",
		},
		Commands: []string{
			"실제 매출 CSV를 붙일테니 이 구조로 다시 분석해줘",
			"제품군 A 성장 원인만 임원 보고용으로 한 페이지 요약해줘",
			"ROAS 낮은 채널을 줄이는 예산 재배분안을 만들어줘",
		},
		DocBody: strings.Join([]string{
			"# 마케팅 매출 분석 실행 패키지",
			"",
			"## 요약",
			"최근 6개월 샘플 데이터에서는 제품군 A가 성장을 주도했고, 4월에는 광고비 증가 대비 매출이 정체되었습니다. 검색광고와 리테일미디어의 ROAS가 높으며 SNS는 인지도 기여는 있으나 구매 전환이 약합니다.",
			"",
			"## 월별 해석",
			"- 1월: 신제품 런칭 전 기준선",
			"- 2월: 프로모션 후 15% 성장",
			"- 3월: 리드 증가가 매출로 연결",
			"- 4월: 광고비 증가 대비 정체. 저효율 채널 점검 필요",
			"- 5월: B2B 대형 고객 유입",
			"- 6월: 제품군 A가 성장 주도",
			"",
			"## 채널 판단",
			"- 검색광고: 구매 의도가 높아 예산 증액 후보",
			"- SNS: 상단 퍼널 기여는 있으나 리타겟팅 메시지 개선 필요",
			"- 리테일미디어: 구매 직전 고객에게 효율이 높아 핵심 SKU 집중",
			"",
			"## 다음 액션",
			"1. 제품군 A 사례집과 랜딩페이지 개선",
			"2. 제안 단계 병목 해소를 위한 가격표/사례자료 자동 생성",
			"3. SNS 예산 일부를 검색광고와 리테일미디어로 재배분",
			"4. 다음 주 영업 회의에서 상위 리드 20개 후속 콜 지정",
		}, "\n"),
		DeckBody: strings.Join([]string{
			"# 매출 흐름",
			"6개월 동안 매출은 120M에서 188M까지 성장했다.",
			"# 성장 기여",
			"제품군 A와 B2B 대형 고객 유입이 성장 주도.",
			"# 정체 구간",
			"4월 광고비 증가 대비 매출 정체. 채널 효율 점검 필요.",
			"# ROAS",
			"검색광고와 리테일미디어가 높고 SNS는 리타겟팅 개선 필요.",
			"# 영업 퍼널",
			"리드와 상담은 충분하지만 제안 단계에서 병목 발생.",
			"# 원인",
			"사례자료, 가격표, ROI 설명 자료 부족.",
			"# 권장 액션",
			"제품군 A 사례집, 랜딩 개선, 예산 재배분, 후속 콜 SLA.",
			"# 다음 회의",
			"예산 조정안과 상위 리드 20개 액션을 확정한다.",
		}, "\n\n"),
		SheetBody: assistantIndustryScenarioSpreadsheet(),
		GraphBody: strings.Join([]string{
			"## 매출 성장 흐름",
			"",
			"```mermaid",
			"flowchart LR",
			"  Jan[\"1월 120M\"] --> Feb[\"2월 138M\"] --> Mar[\"3월 155M\"] --> Apr[\"4월 149M\"] --> May[\"5월 171M\"] --> Jun[\"6월 188M\"]",
			"  Apr -. \"광고비 증가 대비 정체\" .-> Fix[\"SNS 예산 축소\\n검색광고/리테일미디어 재배분\"]",
			"  Jun --> Growth[\"제품군 A 사례집\\nB2B 리드 후속 콜\"]",
			"```",
			"",
			"## 채널별 예산 판단",
			"",
			"```mermaid",
			"pie title ROAS 기준 예산 조정 방향",
			"  \"검색광고 증액\" : 37",
			"  \"리테일미디어 증액\" : 27",
			"  \"SNS 축소/리타겟팅\" : 10",
			"  \"리타겟팅 유지\" : 12",
			"  \"실험 예산\" : 4",
			"```",
			"",
			"## 영업 퍼널 병목",
			"",
			"```mermaid",
			"flowchart LR",
			"  Lead[\"리드 5,400\"] --> Consult[\"상담 820\"] --> Proposal[\"제안 210\"] --> Contract[\"계약 64\"]",
			"  Proposal -. \"병목\" .-> Material[\"가격표\\n사례자료\\nROI 계산표 자동 생성\"]",
			"```",
			"",
			"## 회의에서 바로 볼 데이터",
			"",
			"| 항목 | 값 | 해석 | 다음 액션 |",
			"|---|---:|---|---|",
			"| 6개월 매출 증가 | 120M -> 188M | 성장세 유지 | 제품군 A 집중 |",
			"| 4월 정체 | 149M | 광고비 대비 효율 저하 | 저효율 채널 축소 |",
			"| 검색광고 ROAS | 3.8 | 구매 의도 높음 | 예산 15% 증액 |",
			"| SNS ROAS | 1.4 | 구매 전환 약함 | 리타겟팅 메시지 교체 |",
			"| 제안 단계 전환 | 210건 | 병목 발생 | 사례자료 자동 발송 |",
		}, "\n"),
	}
}

func assistantMarketingWarRoomWorkflowSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "marketing_war_room",
		Title:    lang.T("assistant.workflow.marketing_war_room.title"),
		Audience: lang.T("assistant.workflow.marketing_war_room.audience"),
		Summary:  lang.T("assistant.workflow.marketing_war_room.summary"),
		Highlights: []string{
			lang.T("assistant.workflow.marketing_war_room.h1"),
			lang.T("assistant.workflow.marketing_war_room.h2"),
			lang.T("assistant.workflow.marketing_war_room.h3"),
			lang.T("assistant.workflow.marketing_war_room.h4"),
		},
		Commands: []string{
			lang.T("assistant.workflow.marketing_war_room.c1"),
			lang.T("assistant.workflow.marketing_war_room.c2"),
			lang.T("assistant.workflow.marketing_war_room.c3"),
		},
		DocBody:   lang.T("assistant.workflow.marketing_war_room.doc"),
		DeckBody:  lang.T("assistant.workflow.marketing_war_room.deck"),
		SheetBody: lang.T("assistant.workflow.marketing_war_room.sheet"),
		GraphBody: lang.T("assistant.workflow.marketing_war_room.graph"),
	}
}

func assistantMarketingProductGrowthBriefSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "marketing_product_growth_brief",
		Title:    "제품군 A 성장 원인 임원 1페이지 보고",
		Audience: "대표, 임원진, 마케팅팀, 영업팀",
		Summary:  "제품군 A 성장 원인을 임원 보고용 1페이지 브리프, 발표자료, 그래프 표로 정리했습니다.",
		Highlights: []string{
			"제품군 A는 6개월 동안 52M에서 91M으로 성장한 샘플 흐름",
			"성장 요인은 검색 수요 증가, B2B 대형 리드 전환, 사례 기반 랜딩 개선",
			"다음 결정은 예산 증액보다 성공한 메시지와 SKU 집중 여부",
			"임원 질문 대비용으로 마진, 재고, 반복구매, 영업 병목을 함께 제시",
		},
		Commands: []string{
			"이 내용을 대표 보고용 3줄 요약으로 줄여줘",
			"제품군 A 성장 원인을 영업팀 액션표로 바꿔줘",
			"제품군 A와 제품군 B 비교표도 만들어줘",
		},
		DocBody: strings.Join([]string{
			"# 제품군 A 성장 원인 임원 1페이지 보고",
			"",
			"## 한 줄 결론",
			"제품군 A 성장은 단순 광고비 증가가 아니라 검색 수요, B2B 리드 전환, 사례 기반 랜딩 개선이 동시에 맞물린 결과입니다.",
			"",
			"## 숫자로 보는 흐름",
			"- 1월 매출 52M에서 6월 91M까지 증가",
			"- 제품군 A의 전체 매출 기여도는 43%에서 52%로 상승",
			"- 검색광고 ROAS는 3.1에서 4.4로 개선",
			"- B2B 상담 전환율은 12%에서 18%로 상승",
			"",
			"## 원인",
			"1. 검색 수요: 문제 해결형 키워드에서 유입이 증가했습니다.",
			"2. 랜딩 개선: 제품 설명보다 실제 사례와 ROI 계산표를 앞에 배치했습니다.",
			"3. 영업 연계: 상위 리드에 24시간 이내 후속 콜을 지정했습니다.",
			"4. SKU 집중: 마진이 높은 구성품과 번들 제안이 늘었습니다.",
			"",
			"## 임원 결정 요청",
			"- 제품군 A 검색광고와 리테일미디어 예산을 18% 증액",
			"- 제품군 B 저효율 소재는 2주간 중지",
			"- 제품군 A 사례집 3종과 업종별 ROI 계산표를 영업팀 공통 자료로 배포",
			"- 다음 회의에서 재고와 마진 영향을 함께 확인",
		}, "\n"),
		DeckBody: strings.Join([]string{
			"# 결론",
			"제품군 A 성장은 광고비가 아니라 수요, 랜딩, 영업 후속 조치가 맞물린 결과.",
			"# 매출 흐름",
			"52M에서 91M으로 성장, 전체 기여도 43%에서 52%로 상승.",
			"# 성장 원인",
			"검색 수요 증가, 사례 기반 랜딩, B2B 후속 콜, SKU 집중.",
			"# 리스크",
			"재고 부족, 마진 희석, 제품군 B 예산 잠식.",
			"# 결정 요청",
			"제품군 A 예산 18% 증액, 제품군 B 저효율 소재 2주 중지.",
		}, "\n\n"),
		SheetBody: strings.Join([]string{
			"|월|제품군 A 매출|전체 매출 기여도|검색광고 ROAS|B2B 상담 전환율|메모|",
			"|---|---:|---:|---:|---:|---|",
			"|1월|52000000|43|3.1|12|기준선|",
			"|2월|61000000|46|3.4|13|문제 해결 키워드 성장|",
			"|3월|69000000|47|3.7|15|사례형 랜딩 반영|",
			"|4월|71000000|46|3.3|14|광고비 증가 대비 정체|",
			"|5월|83000000|50|4.1|17|B2B 대형 리드 전환|",
			"|6월|91000000|52|4.4|18|SKU 집중 효과|",
		}, "\n"),
	}
}

func assistantMarketingROASBudgetReallocationSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "marketing_roas_budget_reallocation",
		Title:    "ROAS 기반 광고 예산 재배분안",
		Audience: "마케팅팀, 퍼포먼스 마케터, 재무 담당자",
		Summary:  "ROAS가 낮은 채널을 줄이고 검색광고·리테일미디어로 옮기는 예산 재배분안을 만들었습니다.",
		Highlights: []string{
			"SNS 상단 퍼널 예산 일부를 검색광고와 리테일미디어로 이동",
			"저효율 소재는 즉시 중지하지 않고 7일 리타겟팅 개선 실험으로 전환",
			"재배분 후 예상 ROAS는 2.6에서 3.2로 개선되는 샘플 계획",
			"재무팀 검토용으로 월 예산, 예상 매출, 위험 조건을 함께 표기",
		},
		Commands: []string{
			"이 예산안을 보수적/공격적 2가지 버전으로 나눠줘",
			"SNS를 완전히 끄지 않는 실험안을 따로 만들어줘",
			"재무팀 승인용 메일 초안도 만들어줘",
		},
		DocBody: strings.Join([]string{
			"# ROAS 기반 광고 예산 재배분안",
			"",
			"## 결론",
			"ROAS가 낮은 SNS 일반 소재 예산 18M 중 8M을 검색광고와 리테일미디어로 옮기고, 남은 10M은 후기형 리타겟팅 실험으로 제한합니다.",
			"",
			"## 재배분 원칙",
			"- 구매 의도가 높은 채널에 먼저 증액합니다.",
			"- 인지도 채널은 완전히 중지하지 않고 리타겟팅과 후기형 소재로 축소 운영합니다.",
			"- 재무 검토를 위해 총 예산은 유지하고 채널 간 비중만 조정합니다.",
			"",
			"## 예상 효과",
			"- 전체 ROAS: 2.6에서 3.2로 개선 예상",
			"- 월 예상 매출: 182M에서 203M으로 증가 예상",
			"- CAC: 24,000원에서 20,500원으로 감소 예상",
			"",
			"## 위험 조건",
			"검색광고 CPC가 15% 이상 상승하거나 리테일미디어 재고 소진율이 90%를 넘으면 증액 속도를 줄입니다.",
		}, "\n"),
		DeckBody: strings.Join([]string{
			"# 목적",
			"총 예산은 유지하고 ROAS가 높은 채널로 재배분한다.",
			"# 줄일 채널",
			"SNS 일반 소재 예산 18M 중 8M 이동.",
			"# 늘릴 채널",
			"검색광고 5M, 리테일미디어 3M 증액.",
			"# 예상 효과",
			"ROAS 2.6에서 3.2, 월 매출 203M 예상.",
			"# 중단 조건",
			"CPC 급등, 재고 소진, 전환율 하락 시 증액 중지.",
		}, "\n\n"),
		SheetBody: strings.Join([]string{
			"|채널|현재 예산|조정 후 예산|현재 ROAS|예상 ROAS|결정|",
			"|---|---:|---:|---:|---:|---|",
			"|검색광고|32000000|37000000|3.8|4.1|증액|",
			"|리테일미디어|24000000|27000000|3.5|3.9|증액|",
			"|SNS 일반|18000000|10000000|1.6|2.0|축소 후 소재 교체|",
			"|리타겟팅|12000000|12000000|3.0|3.2|유지|",
			"|실험 예산|4000000|4000000|0|0|신규 소재 검증|",
		}, "\n"),
	}
}

func assistantMarketingLeadActionPlanSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "marketing_sales_lead_action_plan",
		Title:    "상위 리드 20개 영업 후속 액션표",
		Audience: "영업팀, 마케팅팀, 세일즈 오퍼레이션",
		Summary:  "상위 리드 20개를 우선순위, 담당자, 다음 연락 문구, 마감 시간으로 나눈 액션표를 만들었습니다.",
		Highlights: []string{
			"리드를 예산 규모, 구매시점, 문제 긴급도, 의사결정자 접근성으로 점수화",
			"24시간 이내 후속 콜 대상과 3일 nurturing 대상을 분리",
			"영업팀이 바로 쓸 수 있는 전화 스크립트와 메일 첫 문장 포함",
			"제안 단계 병목을 줄이기 위해 ROI 계산표와 사례집 발송을 기본 액션으로 지정",
		},
		Commands: []string{
			"이 액션표를 담당자별 오늘 할 일로 나눠줘",
			"리드별 첫 연락 메일 초안을 만들어줘",
			"계약 가능성 높은 5개만 대표 보고용으로 뽑아줘",
		},
		DocBody: strings.Join([]string{
			"# 상위 리드 20개 영업 후속 액션표",
			"",
			"## 운영 원칙",
			"상위 리드는 24시간 안에 첫 후속 콜을 완료하고, 예산과 일정이 확인된 리드는 ROI 계산표와 업종별 사례집을 함께 보냅니다.",
			"",
			"## 우선순위 기준",
			"- 예산 규모 30점",
			"- 구매 시점 25점",
			"- 문제 긴급도 25점",
			"- 의사결정자 접근성 20점",
			"",
			"## 전화 첫 문장",
			"어제 문의 주신 내용 기준으로, 비용보다 먼저 확인하실 부분은 실제 도입 후 줄어드는 운영 시간과 실패 비용입니다. 그래서 3분 안에 보실 수 있는 ROI 계산표와 같은 업종 사례를 먼저 정리했습니다.",
			"",
			"## 제안 병목 해소",
			"가격표만 보내지 말고, 리드의 업종과 규모에 맞춘 사례 1개, 예상 절감표 1개, 다음 미팅 질문 3개를 함께 보냅니다.",
		}, "\n"),
		DeckBody: strings.Join([]string{
			"# 목표",
			"상위 리드 20개를 24시간 안에 다음 단계로 이동시킨다.",
			"# 점수 기준",
			"예산, 구매시점, 긴급도, 의사결정자 접근성.",
			"# 후속 방식",
			"고득점 리드는 콜, 중간 리드는 사례집, 낮은 리드는 nurturing.",
			"# 병목 해소",
			"가격표 대신 ROI 계산표와 업종 사례를 같이 보낸다.",
			"# 오늘 액션",
			"상위 5개 콜, 6~12번 사례집 발송, 13~20번 nurturing 등록.",
		}, "\n\n"),
		SheetBody: strings.Join([]string{
			"|순위|회사|점수|다음 액션|담당|마감|보낼 자료|",
			"|---:|---|---:|---|---|---|---|",
			"|1|한강케미칼|94|후속 콜|영업 A|오늘 14:00|ROI 계산표, 화학 업종 사례|",
			"|2|서해푸드|91|데모 일정 제안|영업 B|오늘 15:00|유통 사례집|",
			"|3|누리공공서비스|88|요구사항 확인|영업 A|오늘 17:00|공공기관 보안 체크리스트|",
			"|4|메디온헬스|86|예산 확인|영업 C|내일 10:00|병원 자동화 사례|",
			"|5|바른물류|84|의사결정자 연결|영업 B|내일 11:00|물류 KPI 표|",
			"|6~20|기타 상위 리드|70~83|사례집 발송 후 nurturing|각 담당|3일 내|업종별 사례자료|",
		}, "\n"),
	}
}

func assistantMarketingCompetitorBattlecardSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "marketing_competitor_battlecard",
		Title:    "경쟁사 3곳 세일즈 배틀카드",
		Audience: "영업팀, 마케팅팀, 대표, 고객성공팀",
		Summary:  "경쟁사 3곳을 가격, 기능, 약점, 반박 논리, 고객 질문별 답변으로 비교한 세일즈 배틀카드를 만들었습니다.",
		Highlights: []string{
			"경쟁사 A는 가격은 낮지만 도입 지원과 통합 기능이 약한 포지션",
			"경쟁사 B는 기능은 넓지만 구축 기간과 운영 비용이 높은 포지션",
			"경쟁사 C는 특정 업종에 강하지만 확장성과 리포팅이 약한 포지션",
			"영업 콜에서 바로 쓰는 반박 문장, 질문 대응, 다음 미팅 유도 문구 포함",
		},
		Commands: []string{
			"이 배틀카드를 가격 민감 고객용으로 바꿔줘",
			"경쟁사 B와 비교하는 5분 세일즈 스크립트를 만들어줘",
			"이 비교표를 고객 제안서용 PPT 5장으로 줄여줘",
		},
		DocBody: strings.Join([]string{
			"# 경쟁사 3곳 세일즈 배틀카드",
			"",
			"## 사용 목적",
			"영업팀이 고객 통화 중 경쟁사 언급이 나왔을 때 바로 대응할 수 있도록 가격, 기능, 도입 난이도, 운영 비용, 고객 질문별 답변을 정리합니다.",
			"",
			"## 포지셔닝",
			"- 우리 제품: 빠른 도입, 운영 자동화, 리포팅, 담당자 업무 절감",
			"- 경쟁사 A: 초기 가격이 낮지만 통합과 지원 범위가 제한적",
			"- 경쟁사 B: 기능은 넓지만 구축 기간과 운영 비용이 높음",
			"- 경쟁사 C: 특정 업종에 강하지만 다른 팀으로 확장하기 어려움",
			"",
			"## 고객 질문별 답변",
			"### Q1. 경쟁사 A가 더 저렴한데요?",
			"초기 비용은 낮아 보일 수 있습니다. 다만 실제 운영에서는 통합, 리포팅, 담당자 반복 업무까지 포함한 총비용을 봐야 합니다. 저희는 도입 후 매주 줄어드는 운영 시간을 같이 계산해 드립니다.",
			"",
			"### Q2. 경쟁사 B는 기능이 더 많던데요?",
			"맞습니다. 다만 현재 팀이 당장 써야 하는 기능과 3개월 안에 정착할 기능을 나누면 판단이 달라집니다. 저희는 구축 기간을 줄이고 실제 사용률이 높은 흐름부터 적용합니다.",
			"",
			"### Q3. 우리 업종에는 경쟁사 C가 더 맞지 않나요?",
			"특정 업종 템플릿은 강점입니다. 다만 조직이 커지면 다른 부서, 리포팅, 권한, 자동화 연결까지 확장성이 중요합니다. 저희는 업종 템플릿과 함께 부서 확장을 전제로 설계합니다.",
			"",
			"## 다음 미팅 유도 문구",
			"비교표만으로 결정하시기보다, 고객사 현재 업무 3개를 기준으로 20분 안에 실제 운영 비용 차이를 같이 계산해 보겠습니다.",
		}, "\n"),
		DeckBody: strings.Join([]string{
			"# 목적",
			"경쟁사 언급이 나왔을 때 영업팀이 같은 메시지로 대응한다.",
			"# 우리 포지션",
			"빠른 도입, 운영 자동화, 리포팅, 반복 업무 절감.",
			"# 경쟁사 A",
			"초기 가격은 낮지만 통합과 지원 범위가 제한적.",
			"# 경쟁사 B",
			"기능은 넓지만 구축 기간과 운영 비용이 높음.",
			"# 경쟁사 C",
			"업종 특화는 강하지만 확장성과 리포팅이 약함.",
			"# 반박 구조",
			"초기 비용이 아니라 총운영비, 기능 수가 아니라 실제 사용률로 답한다.",
			"# 다음 액션",
			"고객 업무 3개 기준으로 ROI 계산 미팅을 제안한다.",
		}, "\n\n"),
		SheetBody: strings.Join([]string{
			"|항목|우리 제품|경쟁사 A|경쟁사 B|경쟁사 C|영업 대응|",
			"|---|---|---|---|---|---|",
			"|초기 비용|중간|낮음|높음|중간|총운영비로 비교|",
			"|도입 속도|빠름|빠름|느림|중간|3개월 정착 계획 제시|",
			"|통합|강함|약함|강함|중간|기존 도구 연결 사례 제시|",
			"|리포팅|강함|약함|중간|약함|임원 보고 자동화 강조|",
			"|업종 템플릿|중간|약함|중간|강함|업종+부서 확장성 강조|",
			"|운영 자동화|강함|중간|중간|약함|반복 업무 절감 시간 계산|",
		}, "\n"),
	}
}

func assistantMarketingRevenueMeetingSpec() assistantDepartmentWorkflowSpec {
	return assistantDepartmentWorkflowSpec{
		Kind:     "marketing_revenue_meeting_pack",
		Title:    "다음 주 매출 회의 아젠다와 질문지",
		Audience: "대표, 마케팅팀, 영업팀, 재무 담당자",
		Summary:  "다음 주 매출 회의에서 바로 쓸 아젠다, 결정 질문, 담당자별 준비자료 표를 만들었습니다.",
		Highlights: []string{
			"회의 시간을 보고가 아니라 결정 중심으로 재구성",
			"제품군 A 증액, 제품군 B 축소, SNS 실험 유지 여부를 결정 질문으로 분리",
			"마케팅·영업·재무가 각각 준비해야 할 자료를 표로 지정",
			"회의 후 바로 실행될 액션과 마감 시간을 포함",
		},
		Commands: []string{
			"이 회의자료를 30분 진행표로 바꿔줘",
			"대표가 물어볼 질문 10개와 답변 초안을 만들어줘",
			"회의 후 실행 체크리스트로 바꿔줘",
		},
		DocBody: strings.Join([]string{
			"# 다음 주 매출 회의 아젠다와 질문지",
			"",
			"## 회의 목적",
			"지난 6개월 매출 흐름을 설명하는 데 그치지 않고, 다음 주 예산 재배분과 영업 후속 조치를 확정합니다.",
			"",
			"## 아젠다",
			"1. 제품군 A 성장 원인과 증액 여부",
			"2. 제품군 B 저효율 소재 중지 여부",
			"3. SNS 일반 소재 예산 축소와 리타겟팅 실험 유지 여부",
			"4. 상위 리드 20개 후속 콜 SLA",
			"5. 재고와 마진 영향 확인",
			"",
			"## 핵심 질문",
			"- 제품군 A 성장이 반복 가능한 구조인가, 일회성 프로모션 효과인가?",
			"- 검색광고 증액 시 CPC 상승을 어디까지 허용할 것인가?",
			"- SNS 예산을 줄였을 때 상단 퍼널 손실은 어떻게 보완할 것인가?",
			"- 영업팀은 상위 리드 20개를 언제까지 다음 단계로 이동시킬 수 있는가?",
			"- 재고와 마진이 증액 속도를 따라갈 수 있는가?",
		}, "\n"),
		DeckBody: strings.Join([]string{
			"# 회의 목적",
			"보고가 아니라 예산과 영업 액션을 결정한다.",
			"# 결정 1",
			"제품군 A 예산 증액 여부.",
			"# 결정 2",
			"제품군 B 저효율 소재 중지 여부.",
			"# 결정 3",
			"SNS 축소와 리타겟팅 실험 유지 여부.",
			"# 결정 4",
			"상위 리드 20개 후속 콜 SLA.",
			"# 회의 후",
			"담당자와 마감 시간을 확정한다.",
		}, "\n\n"),
		SheetBody: strings.Join([]string{
			"|아젠다|준비 담당|필요 자료|결정할 것|회의 후 액션|",
			"|---|---|---|---|---|",
			"|제품군 A 성장|마케팅|월별 매출, ROAS, 랜딩 전환율|예산 18% 증액 여부|검색광고 증액 실험|",
			"|제품군 B 정체|마케팅|소재별 CTR/CVR|저효율 소재 중지|2주 중지 후 재검토|",
			"|상위 리드|영업|리드 20개 점수표|24시간 SLA|담당자별 콜 배정|",
			"|재고/마진|재무|SKU별 마진, 재고|증액 속도 제한|주간 재고 점검|",
		}, "\n"),
	}
}
