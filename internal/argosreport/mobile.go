package argosreport

import (
	"html"
	"strings"

	"github.com/meshclaw/meshclaw/internal/lang"
)

type MobileReport struct {
	Eyebrow  string
	Title    string
	Subtitle string
	Flow     []string
	Findings []string
	Footer   string
}

func RenderMobileHTML(report MobileReport) string {
	eyebrow := firstNonEmpty(report.Eyebrow, lang.T("argosreport.mobile.eyebrow"))
	title := firstNonEmpty(report.Title, lang.T("argosreport.mobile.title"))
	footer := firstNonEmpty(report.Footer, lang.T("argosreport.mobile.footer"))
	flow := report.Flow
	if len(flow) == 0 {
		flow = []string{
			lang.T("argosreport.mobile.flow.request"),
			lang.T("argosreport.mobile.flow.execute"),
			lang.T("argosreport.mobile.flow.result"),
			lang.T("argosreport.mobile.flow.send"),
		}
	}
	findings := report.Findings
	if len(findings) == 0 {
		findings = []string{lang.T("argosreport.mobile.finding.default")}
	}
	flowLabel := lang.T("argosreport.mobile.flow_label")
	findingsLabel := lang.T("argosreport.mobile.findings_label")
	flowAria := lang.T("argosreport.mobile.flow_aria")
	return "<!doctype html><html><head><meta charset=\"utf-8\"><meta name=\"viewport\" content=\"width=device-width,initial-scale=1,maximum-scale=5,viewport-fit=cover\">" +
		"<title>" + html.EscapeString(title) + "</title>" +
		"<style>:root{color-scheme:light;background:#f7f8fb;color:#15171a;-webkit-text-size-adjust:100%;text-size-adjust:100%}*{box-sizing:border-box}body{font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;margin:0;padding:20px max(18px,env(safe-area-inset-right)) 24px max(18px,env(safe-area-inset-left));background:#f7f8fb;color:#15171a}.page{max-width:720px;margin:0 auto;min-height:100svh;display:flex;flex-direction:column;justify-content:flex-start;gap:16px}.top{padding:6px 2px 2px}.eyebrow{font-size:13px;font-weight:800;letter-spacing:.06em;text-transform:uppercase;color:#2563eb}.title{font-size:34px;line-height:1.12;margin:8px 0 10px;font-weight:850}.query{font-size:18px;line-height:1.5;margin:0;color:#475569;overflow-wrap:anywhere}.grid{display:grid;grid-template-columns:1fr;gap:14px}.card{background:#fff;border:1px solid #d9e2ef;border-radius:8px;padding:18px;box-shadow:0 1px 2px rgba(15,23,42,.06)}.label{font-size:14px;font-weight:850;color:#0f766e;margin:0 0 12px}.flowchart{display:grid;grid-template-columns:1fr;gap:14px;counter-reset:step}.flow-node{position:relative;border:1px solid #dce6f2;border-radius:8px;padding:13px 14px 13px 46px;font-size:17px;line-height:1.35;font-weight:750;background:#f9fbfe;overflow-wrap:anywhere}.flow-node:before{counter-increment:step;content:counter(step);position:absolute;left:14px;top:12px;width:22px;height:22px;border-radius:999px;display:grid;place-items:center;background:#2563eb;color:#fff;font-size:13px;font-weight:850}.flow-node:not(:last-child):after{content:\"↓\";position:absolute;left:24px;bottom:-19px;color:#0f766e;font-size:20px;font-weight:850}.findings{margin:0;padding-left:22px;font-size:18px;line-height:1.58}.findings li{margin:0 0 12px;overflow-wrap:anywhere}.footer{font-size:15px;line-height:1.45;color:#64748b;padding:0 2px;margin:0}@media(min-width:560px){body{padding:28px}.grid{grid-template-columns:.9fr 1.1fr}.title{font-size:40px}}@media(max-width:390px){body{padding-left:16px;padding-right:16px}.title{font-size:31px}.card{padding:16px}.findings{font-size:18px}.flow-node{font-size:17px}}</style>" +
		"</head><body><main class=\"page\"><section class=\"top\"><div class=\"eyebrow\">" + html.EscapeString(eyebrow) + "</div><h1 class=\"title\">" + html.EscapeString(title) + "</h1><p class=\"query\">" + html.EscapeString(report.Subtitle) + "</p></section><section class=\"grid\"><div class=\"card\"><p class=\"label\">" + html.EscapeString(flowLabel) + "</p><div class=\"flowchart\" aria-label=\"" + html.EscapeString(flowAria) + "\">" + renderSteps(flow) + "</div></div><div class=\"card\"><p class=\"label\">" + html.EscapeString(findingsLabel) + "</p><ul class=\"findings\">" + renderFindings(findings) + "</ul></div></section><p class=\"footer\">" + html.EscapeString(footer) + "</p></main></body></html>"
}

func renderSteps(values []string) string {
	var b strings.Builder
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		b.WriteString("<div class=\"flow-node\">")
		b.WriteString(html.EscapeString(value))
		b.WriteString("</div>")
	}
	return b.String()
}

func renderFindings(values []string) string {
	var b strings.Builder
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		b.WriteString("<li>")
		b.WriteString(html.EscapeString(value))
		b.WriteString("</li>")
	}
	return b.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
