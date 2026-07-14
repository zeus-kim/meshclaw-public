package messenger

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/meshclaw/meshclaw/internal/runtimeflow"
)

type ReportOptions struct {
	Ref      string
	Channel  string
	Audience string
}

type Report struct {
	Kind           string                 `json:"kind"`
	Channel        string                 `json:"channel"`
	Audience       string                 `json:"audience"`
	Ref            string                 `json:"ref"`
	Workflow       string                 `json:"workflow"`
	BundleDir      string                 `json:"bundle_dir"`
	GeneratedAt    time.Time              `json:"generated_at"`
	Decision       string                 `json:"decision"`
	Headline       string                 `json:"headline"`
	Summary        runtimeflow.Summary    `json:"summary"`
	Counts         Counts                 `json:"counts"`
	ApprovalNeeded []ApprovalRequest      `json:"approval_needed,omitempty"`
	Failures       []ReportItem           `json:"failures,omitempty"`
	Retryable      []ReportItem           `json:"retryable,omitempty"`
	RepairNeeded   []ReportItem           `json:"repair_needed,omitempty"`
	VaultNeeded    []ReportItem           `json:"vault_needed,omitempty"`
	NextActions    []string               `json:"next_actions"`
	Evidence       EvidencePaths          `json:"evidence"`
	Redaction      RedactionPolicy        `json:"redaction"`
	Text           string                 `json:"text"`
	Source         runtimeflow.ResumePlan `json:"-"`
}

type Counts struct {
	ApprovalNeeded int `json:"approval_needed"`
	Failures       int `json:"failures"`
	Retryable      int `json:"retryable"`
	RepairNeeded   int `json:"repair_needed"`
	VaultNeeded    int `json:"vault_needed"`
}

type ReportItem struct {
	Step       string `json:"step"`
	Title      string `json:"title"`
	Node       string `json:"node,omitempty"`
	Status     string `json:"status"`
	Reason     string `json:"reason"`
	NextAction string `json:"next_action,omitempty"`
}

type ApprovalRequest struct {
	Step        string                 `json:"step"`
	Title       string                 `json:"title"`
	Action      string                 `json:"action,omitempty"`
	Resource    string                 `json:"resource,omitempty"`
	Strong      bool                   `json:"strong"`
	Reason      string                 `json:"reason"`
	ApprovalCLI string                 `json:"approval_cli,omitempty"`
	ApprovalMCP map[string]interface{} `json:"approval_mcp_call,omitempty"`
	Text        string                 `json:"text"`
}

type EvidencePaths struct {
	Plan      string `json:"plan"`
	Execution string `json:"execution"`
	Steps     string `json:"steps"`
	Actions   string `json:"actions"`
	Report    string `json:"report"`
	Approvals string `json:"approvals"`
}

type RedactionPolicy struct {
	RawSecretsIncluded bool     `json:"raw_secrets_included"`
	Allowed            []string `json:"allowed"`
	Forbidden          []string `json:"forbidden"`
}

func BuildReport(opts ReportOptions) (Report, error) {
	ref := strings.TrimSpace(opts.Ref)
	if ref == "" {
		ref = "latest"
	}
	channel := strings.TrimSpace(opts.Channel)
	if channel == "" {
		channel = "signal"
	}
	audience := strings.TrimSpace(opts.Audience)
	if audience == "" {
		audience = "owner"
	}
	plan, err := runtimeflow.Resume(ref)
	if err != nil {
		return Report{}, err
	}
	report := Report{
		Kind:        "meshclaw_messenger_report",
		Channel:     channel,
		Audience:    audience,
		Ref:         ref,
		Workflow:    plan.Workflow,
		BundleDir:   plan.BundleDir,
		GeneratedAt: time.Now().UTC(),
		Summary:     plan.Summary,
		NextActions: plan.Next,
		Evidence: EvidencePaths{
			Plan:      filepath.Join(plan.BundleDir, "plan.md"),
			Execution: filepath.Join(plan.BundleDir, "execution.json"),
			Steps:     filepath.Join(plan.BundleDir, "steps.jsonl"),
			Actions:   filepath.Join(plan.BundleDir, "meshclaw-actions.md"),
			Report:    ReportMarkdownPath(plan.BundleDir),
			Approvals: plan.ApprovalsPath,
		},
		Redaction: defaultRedactionPolicy(),
		Source:    plan,
	}
	for _, item := range plan.Items {
		switch item.Status {
		case "approval_pending":
			report.ApprovalNeeded = append(report.ApprovalNeeded, approvalFromItem(item))
		case "failed":
			report.Failures = append(report.Failures, itemSummary(item))
		case "retryable_failed":
			report.Retryable = append(report.Retryable, itemSummary(item))
		case "degraded_repair":
			report.RepairNeeded = append(report.RepairNeeded, itemSummary(item))
		case "vault_missing":
			report.VaultNeeded = append(report.VaultNeeded, itemSummary(item))
		}
	}
	report.Counts = Counts{
		ApprovalNeeded: len(report.ApprovalNeeded),
		Failures:       len(report.Failures),
		Retryable:      len(report.Retryable),
		RepairNeeded:   len(report.RepairNeeded),
		VaultNeeded:    len(report.VaultNeeded),
	}
	report.Decision = reportDecision(report)
	report.Headline = reportHeadline(report)
	report.Text = FormatText(report)
	return report, nil
}

func BuildApprovalRequest(ref, step, channel, audience string) (ApprovalRequest, error) {
	report, err := BuildReport(ReportOptions{Ref: ref, Channel: channel, Audience: audience})
	if err != nil {
		return ApprovalRequest{}, err
	}
	for _, item := range report.ApprovalNeeded {
		if item.Step == step {
			return item, nil
		}
	}
	return ApprovalRequest{}, fmt.Errorf("approval step %q not pending in %s", step, report.BundleDir)
}

func WriteReport(report Report) (string, error) {
	path := filepath.Join(report.BundleDir, "messenger-report.json")
	mdPath := ReportMarkdownPath(report.BundleDir)
	if err := os.WriteFile(mdPath, []byte(FormatMarkdown(report)), 0600); err != nil {
		return "", err
	}
	report.Evidence.Report = mdPath
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	return path, os.WriteFile(path, append(data, '\n'), 0600)
}

func ReportMarkdownPath(bundleDir string) string {
	return filepath.Join(bundleDir, "messenger-report.md")
}

func FormatText(report Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "MeshClaw 보고서: %s\n", report.Headline)
	fmt.Fprintf(&b, "워크플로: %s | 결정: %s\n", report.Workflow, report.Decision)
	fmt.Fprintf(&b, "요약: total=%d succeeded=%d failed=%d skipped=%d approval=%d retryable=%d\n", report.Summary.Total, report.Summary.Succeeded, report.Summary.Failed, report.Summary.Skipped, report.Summary.ApprovalRequired, report.Summary.Retryable)
	if len(report.ApprovalNeeded) > 0 {
		b.WriteString("승인 필요:\n")
		for _, item := range report.ApprovalNeeded {
			fmt.Fprintf(&b, "- %s: %s\n", item.Step, item.Reason)
		}
	}
	if len(report.Failures) > 0 {
		b.WriteString("실패:\n")
		for _, item := range report.Failures {
			fmt.Fprintf(&b, "- %s: %s\n", item.Step, item.Reason)
		}
	}
	if len(report.Retryable) > 0 {
		b.WriteString("재시도 가능:\n")
		for _, item := range report.Retryable {
			fmt.Fprintf(&b, "- %s: %s\n", item.Step, item.Reason)
		}
	}
	if len(report.NextActions) > 0 {
		b.WriteString("다음:\n")
		for _, action := range report.NextActions {
			fmt.Fprintf(&b, "- %s\n", action)
		}
	}
	b.WriteString("세부 보고서는 Obsidian Markdown 첨부로 보냈습니다.\n")
	b.WriteString("redaction: raw secrets are not included; use vault handles and approval gates only.")
	return b.String()
}

func FormatMarkdown(report Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "---\n")
	fmt.Fprintf(&b, "type: meshclaw-argos-report\n")
	fmt.Fprintf(&b, "channel: %s\n", markdownScalar(report.Channel))
	fmt.Fprintf(&b, "audience: %s\n", markdownScalar(report.Audience))
	fmt.Fprintf(&b, "workflow: %s\n", markdownScalar(report.Workflow))
	fmt.Fprintf(&b, "decision: %s\n", markdownScalar(report.Decision))
	fmt.Fprintf(&b, "generated_at: %s\n", report.GeneratedAt.Format(time.RFC3339))
	fmt.Fprintf(&b, "tags:\n  - argos\n  - meshclaw\n  - report\n---\n\n")
	fmt.Fprintf(&b, "# %s\n\n", reportFirstNonEmpty(report.Headline, "MeshClaw Argos Report"))
	fmt.Fprintf(&b, "## Summary\n\n")
	fmt.Fprintf(&b, "| Field | Value |\n| --- | ---: |\n")
	fmt.Fprintf(&b, "| Total | %d |\n", report.Summary.Total)
	fmt.Fprintf(&b, "| Succeeded | %d |\n", report.Summary.Succeeded)
	fmt.Fprintf(&b, "| Failed | %d |\n", report.Summary.Failed)
	fmt.Fprintf(&b, "| Skipped | %d |\n", report.Summary.Skipped)
	fmt.Fprintf(&b, "| Approval required | %d |\n", report.Summary.ApprovalRequired)
	fmt.Fprintf(&b, "| Retryable | %d |\n\n", report.Summary.Retryable)
	writeApprovalRequestsMarkdown(&b, report.ApprovalNeeded)
	writeReportItemsMarkdown(&b, "Failures", report.Failures)
	writeReportItemsMarkdown(&b, "Retryable", report.Retryable)
	writeReportItemsMarkdown(&b, "Repair Needed", report.RepairNeeded)
	writeReportItemsMarkdown(&b, "Vault Needed", report.VaultNeeded)
	if len(report.NextActions) > 0 {
		b.WriteString("## Next Actions\n\n")
		for _, action := range report.NextActions {
			fmt.Fprintf(&b, "- %s\n", action)
		}
		b.WriteString("\n")
	}
	b.WriteString("## Evidence\n\n")
	fmt.Fprintf(&b, "- Bundle: `%s`\n", report.BundleDir)
	fmt.Fprintf(&b, "- Execution: `%s`\n", report.Evidence.Execution)
	fmt.Fprintf(&b, "- Plan: `%s`\n", report.Evidence.Plan)
	fmt.Fprintf(&b, "- Steps: `%s`\n", report.Evidence.Steps)
	fmt.Fprintf(&b, "- Actions: `%s`\n", report.Evidence.Actions)
	if report.Evidence.Approvals != "" {
		fmt.Fprintf(&b, "- Approvals: `%s`\n", report.Evidence.Approvals)
	}
	b.WriteString("\n## Redaction\n\n")
	b.WriteString("- Raw secrets are not included.\n")
	b.WriteString("- Use vault handles and approval gates only.\n")
	return b.String()
}

func writeApprovalRequestsMarkdown(b *strings.Builder, items []ApprovalRequest) {
	if len(items) == 0 {
		return
	}
	b.WriteString("## Approval Needed\n\n")
	fmt.Fprintf(b, "| Step | Title | Action | Resource | Reason | Approval CLI |\n| --- | --- | --- | --- | --- | --- |\n")
	for _, item := range items {
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s | %s |\n",
			markdownTableCell(item.Step),
			markdownTableCell(item.Title),
			markdownTableCell(item.Action),
			markdownTableCell(item.Resource),
			markdownTableCell(item.Reason),
			markdownTableCell(item.ApprovalCLI),
		)
	}
	b.WriteString("\n")
}

func writeReportItemsMarkdown(b *strings.Builder, title string, items []ReportItem) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "## %s\n\n", title)
	fmt.Fprintf(b, "| Step | Title | Status | Reason | Next |\n| --- | --- | --- | --- | --- |\n")
	for _, item := range items {
		fmt.Fprintf(b, "| %s | %s | %s | %s | %s |\n",
			markdownTableCell(item.Step),
			markdownTableCell(item.Title),
			markdownTableCell(item.Status),
			markdownTableCell(item.Reason),
			markdownTableCell(item.NextAction),
		)
	}
	b.WriteString("\n")
}

func markdownTableCell(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", "\\|")
	return strings.TrimSpace(value)
}

func markdownScalar(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return `""`
	}
	value = strings.ReplaceAll(value, `"`, `\"`)
	return `"` + value + `"`
}

func reportFirstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func approvalFromItem(item runtimeflow.ResumeItem) ApprovalRequest {
	req := ApprovalRequest{
		Step:        item.Step,
		Title:       item.Title,
		Action:      item.Action,
		Resource:    item.Resource,
		Strong:      item.StrongApproval,
		Reason:      item.Reason,
		ApprovalCLI: item.ApprovalCLI,
		ApprovalMCP: item.ApprovalMCPCall,
	}
	req.Text = fmt.Sprintf("Approval needed for `%s` (%s). Reason: %s. Approve with: %s", req.Step, req.Title, req.Reason, req.ApprovalCLI)
	return req
}

func itemSummary(item runtimeflow.ResumeItem) ReportItem {
	return ReportItem{
		Step:       item.Step,
		Title:      item.Title,
		Node:       item.Node,
		Status:     item.Status,
		Reason:     item.Reason,
		NextAction: item.NextAction,
	}
}

func reportDecision(report Report) string {
	switch {
	case report.Counts.Failures > 0:
		return "blocked"
	case report.Counts.Retryable > 0 || report.Counts.RepairNeeded > 0:
		return "repair_required"
	case report.Counts.VaultNeeded > 0:
		return "vault_required"
	case report.Counts.ApprovalNeeded > 0:
		return "approval_required"
	default:
		return "ok"
	}
}

func reportHeadline(report Report) string {
	switch report.Decision {
	case "blocked":
		return fmt.Sprintf("%s has %d blocking failure(s).", report.Workflow, report.Counts.Failures)
	case "repair_required":
		return fmt.Sprintf("%s needs repair or retry before continuation.", report.Workflow)
	case "vault_required":
		return fmt.Sprintf("%s needs local vault handles before execution.", report.Workflow)
	case "approval_required":
		return fmt.Sprintf("%s is ready to continue after %d approval gate(s).", report.Workflow, report.Counts.ApprovalNeeded)
	default:
		return fmt.Sprintf("%s is ready; no blocker is visible in the evidence bundle.", report.Workflow)
	}
}

func defaultRedactionPolicy() RedactionPolicy {
	return RedactionPolicy{
		RawSecretsIncluded: false,
		Allowed: []string{
			"workflow status",
			"step IDs and titles",
			"redacted failure reasons",
			"vault:// handles",
			"evidence paths",
			"approval commands",
		},
		Forbidden: []string{
			"raw passwords",
			"raw API tokens",
			"private keys",
			"Signal raw ingress messages",
			"copyable secret substrings",
		},
	}
}
