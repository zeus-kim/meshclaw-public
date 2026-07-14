// Package guardcode implements MeshClaw Guard's AI-code-review SAST scan.
//
// It runs deterministic static analyzers (semgrep + bandit) against a repo on
// a fleet node and aggregates their findings. This is the deterministic core of
// the capability previously exposed as pwagent's `ai_edits`. It belongs in the
// MeshClaw DevOps/Guard layer because it operates on fleet hosts and source
// trees, not on the credential vault.
//
// Scope notes:
//   - Secret/credential leak detection is intentionally NOT duplicated here; it
//     is already covered by Guard's secret detection (meshclaw_guard_vuln /
//     hygiene). guardcode focuses on the SAST analyzers that meshclaw lacked.
//   - LLM triage of findings (the qwen step in the old pipeline) can be layered
//     on top via MeshClaw's model gateway; this package returns the raw,
//     deterministic findings that such triage would consume.
//
// Like guardcve, the package is transport-agnostic: the caller runs
// ScanCommand() on the target host (via the runtime runner) and passes stdout
// to BuildReport().
package guardcode

import (
	"encoding/json"
	"sort"
	"strings"
)

// Finding is one normalized static-analysis finding.
// The raw code snippet is intentionally omitted so scan output never carries
// source lines (which may embed secrets) off the host.
type Finding struct {
	Tool     string `json:"tool"` // semgrep | bandit
	RuleID   string `json:"rule_id"`
	Severity string `json:"severity"` // high | warn | info
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Message  string `json:"message"`
}

// Report is the structured result of a code review on a host repo.
type Report struct {
	Mode         string         `json:"mode"`
	Host         string         `json:"host"`
	RepoPath     string         `json:"repo_path"`
	Status       string         `json:"status"` // clean | findings | failed
	FindingCount int            `json:"finding_count"`
	ByTool       map[string]int `json:"by_tool"`
	BySeverity   map[string]int `json:"by_severity"`
	HighSeverity []Finding      `json:"high_severity"`
	Findings     []Finding      `json:"findings"`
	Errors       []string       `json:"errors,omitempty"`
	Principle    string         `json:"principle"`
}

const principle = "read-only SAST on a fleet node; findings are redacted (no source snippets leave the host); patching is approval-gated"

const (
	markSemgrep    = "###SEMGREP###"
	markSemgrepEnd = "###SEMGREP_END###"
	markBandit     = "###BANDIT###"
	markBanditEnd  = "###BANDIT_END###"
)

// shellQuote wraps a string in single quotes for safe POSIX shell embedding.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ScanCommand returns a POSIX shell script that runs semgrep and bandit against
// repoPath and emits each analyzer's JSON between marker lines. Each analyzer is
// guarded by command -v so a missing tool is skipped rather than failing.
func ScanCommand(repoPath string) string {
	q := shellQuote(repoPath)
	var b strings.Builder
	b.WriteString("set +e\n")
	b.WriteString("echo " + markSemgrep + "\n")
	b.WriteString("if command -v semgrep >/dev/null 2>&1; then\n")
	b.WriteString("  semgrep --config p/security-audit --json --quiet --no-git-ignore " + q + " 2>/dev/null\n")
	b.WriteString("fi\n")
	b.WriteString("echo " + markSemgrepEnd + "\n")
	b.WriteString("echo " + markBandit + "\n")
	b.WriteString("if command -v bandit >/dev/null 2>&1; then\n")
	b.WriteString("  bandit -r -f json -q " + q + " 2>/dev/null\n")
	b.WriteString("fi\n")
	b.WriteString("echo " + markBanditEnd + "\n")
	return b.String()
}

func section(stdout, start, end string) string {
	i := strings.Index(stdout, start)
	if i < 0 {
		return ""
	}
	i += len(start)
	j := strings.Index(stdout[i:], end)
	if j < 0 {
		return strings.TrimSpace(stdout[i:])
	}
	return strings.TrimSpace(stdout[i : i+j])
}

// jsonObject trims surrounding noise to the outermost JSON object.
func jsonObject(s string) string {
	i := strings.Index(s, "{")
	j := strings.LastIndex(s, "}")
	if i < 0 || j < 0 || j < i {
		return ""
	}
	return s[i : j+1]
}

type semgrepOut struct {
	Results []struct {
		CheckID string `json:"check_id"`
		Path    string `json:"path"`
		Start   struct {
			Line int `json:"line"`
		} `json:"start"`
		Extra struct {
			Severity string `json:"severity"`
			Message  string `json:"message"`
		} `json:"extra"`
	} `json:"results"`
}

type banditOut struct {
	Results []struct {
		TestID        string `json:"test_id"`
		Filename      string `json:"filename"`
		LineNumber    int    `json:"line_number"`
		IssueSeverity string `json:"issue_severity"`
		IssueText     string `json:"issue_text"`
	} `json:"results"`
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// ParseSemgrep parses semgrep --json output into findings.
func ParseSemgrep(jsonStr string) []Finding {
	obj := jsonObject(jsonStr)
	if obj == "" {
		return nil
	}
	var out semgrepOut
	if err := json.Unmarshal([]byte(obj), &out); err != nil {
		return nil
	}
	sevMap := map[string]string{"error": "high", "warning": "warn", "info": "info"}
	var findings []Finding
	for _, h := range out.Results {
		sev := sevMap[strings.ToLower(h.Extra.Severity)]
		if sev == "" {
			sev = "info"
		}
		findings = append(findings, Finding{
			Tool:     "semgrep",
			RuleID:   h.CheckID,
			Severity: sev,
			Path:     h.Path,
			Line:     h.Start.Line,
			Message:  clip(h.Extra.Message, 240),
		})
	}
	return findings
}

// ParseBandit parses bandit -f json output into findings.
func ParseBandit(jsonStr string) []Finding {
	obj := jsonObject(jsonStr)
	if obj == "" {
		return nil
	}
	var out banditOut
	if err := json.Unmarshal([]byte(obj), &out); err != nil {
		return nil
	}
	sevMap := map[string]string{"high": "high", "medium": "warn", "low": "info"}
	var findings []Finding
	for _, h := range out.Results {
		sev := sevMap[strings.ToLower(h.IssueSeverity)]
		if sev == "" {
			sev = "info"
		}
		findings = append(findings, Finding{
			Tool:     "bandit",
			RuleID:   h.TestID,
			Severity: sev,
			Path:     h.Filename,
			Line:     h.LineNumber,
			Message:  clip(h.IssueText, 240),
		})
	}
	return findings
}

func severityRank(s string) int {
	switch s {
	case "high":
		return 0
	case "warn":
		return 1
	default:
		return 2
	}
}

// BuildReport parses ScanCommand stdout and assembles a report.
func BuildReport(host, repoPath, stdout string, scanOK bool) Report {
	report := Report{
		Mode:         "code-review",
		Host:         host,
		RepoPath:     repoPath,
		ByTool:       map[string]int{},
		BySeverity:   map[string]int{},
		HighSeverity: []Finding{},
		Findings:     []Finding{},
		Principle:    principle,
	}

	semgrepSection := section(stdout, markSemgrep, markSemgrepEnd)
	banditSection := section(stdout, markBandit, markBanditEnd)

	findings := append(ParseSemgrep(semgrepSection), ParseBandit(banditSection)...)
	for _, f := range findings {
		report.ByTool[f.Tool]++
		report.BySeverity[f.Severity]++
		if f.Severity == "high" {
			report.HighSeverity = append(report.HighSeverity, f)
		}
	}
	sort.SliceStable(findings, func(a, b int) bool {
		return severityRank(findings[a].Severity) < severityRank(findings[b].Severity)
	})
	report.Findings = findings
	report.FindingCount = len(findings)

	if !scanOK && len(findings) == 0 {
		report.Status = "failed"
		report.Errors = append(report.Errors, "code-review scan reported a non-success transport result and produced no findings")
		return report
	}
	if report.ByTool["semgrep"] == 0 && report.ByTool["bandit"] == 0 && len(findings) == 0 {
		report.Errors = append(report.Errors, "no findings (analyzers may not be installed on the host, or the repo is clean)")
	}
	if len(findings) == 0 {
		report.Status = "clean"
	} else {
		report.Status = "findings"
	}
	return report
}
