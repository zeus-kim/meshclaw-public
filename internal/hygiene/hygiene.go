package hygiene

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/meshclaw/meshclaw/internal/inventory"
	"github.com/meshclaw/meshclaw/internal/runtime"
)

type FindingType string

const (
	FindingSecretPattern FindingType = "secret_pattern"
	FindingPIIPattern    FindingType = "pii_pattern"
	FindingPermission    FindingType = "risky_permission"
	FindingLogLeak       FindingType = "log_leak"
)

type ActionMode string

const (
	ModeAutoSafe         ActionMode = "auto_safe"
	ModeRequiresApproval ActionMode = "requires_approval"
)

type Finding struct {
	Type     FindingType `json:"type"`
	Severity string      `json:"severity"`
	Target   string      `json:"target"`
	Evidence string      `json:"evidence"`
}

type Action struct {
	ID      string     `json:"id"`
	Mode    ActionMode `json:"mode"`
	Target  string     `json:"target"`
	Command string     `json:"command"`
	Reason  string     `json:"reason"`
	Verify  string     `json:"verify"`
}

type Report struct {
	Host     string    `json:"host"`
	Status   string    `json:"status"`
	Findings []Finding `json:"findings"`
	Actions  []Action  `json:"actions"`
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[_-]?key|secret|token|password)\s*[:=]\s*['"]?[^'"\s]{8,}`),
	regexp.MustCompile(`-----BEGIN (RSA |OPENSSH |EC |DSA )?PRIVATE KEY-----`),
	regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}`),
}

var piiPatterns = []*regexp.Regexp{
	regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`),
	regexp.MustCompile(`01[016789]-?\d{3,4}-?\d{4}`),
}

func Plan(host string) Report {
	if _, ok := inventory.Find(host); !ok {
		return Report{
			Host:   host,
			Status: "unknown_node",
			Findings: []Finding{{
				Type:     FindingLogLeak,
				Severity: "error",
				Target:   host,
				Evidence: "node is not in inventory",
			}},
		}
	}

	return Report{
		Host:   host,
		Status: "plan_only",
		Actions: []Action{
			{
				ID:      "scan-secrets",
				Mode:    ModeAutoSafe,
				Target:  host,
				Command: "find /etc /home /opt /var/log -maxdepth 4 -type f \\( -name '*.env' -o -name '*.log' -o -name '*.conf' -o -name '*.json' \\) -size -5M 2>/dev/null",
				Reason:  "enumerate likely secret and log files without reading large data sets",
				Verify:  "scan output contains only paths; file contents are read with redaction in the next step",
			},
			{
				ID:      "fix-secret-permissions",
				Mode:    ModeAutoSafe,
				Target:  host,
				Command: "chmod 600 <confirmed-secret-file>",
				Reason:  "restrict world/group-readable secret files after confirmation",
				Verify:  "stat -c '%a %n' <confirmed-secret-file>",
			},
			{
				ID:      "rotate-exposed-secret",
				Mode:    ModeRequiresApproval,
				Target:  host,
				Command: "provider-specific key rotation",
				Reason:  "secret rotation can break services and requires human approval",
				Verify:  "service health check and old key revocation evidence",
			},
		},
	}
}

func ScanText(target, text string) Report {
	var findings []Finding
	for _, pattern := range secretPatterns {
		for _, match := range pattern.FindAllString(text, -1) {
			findings = append(findings, Finding{
				Type:     FindingSecretPattern,
				Severity: "high",
				Target:   target,
				Evidence: redact(match),
			})
		}
	}
	for _, pattern := range piiPatterns {
		for _, match := range pattern.FindAllString(text, -1) {
			findings = append(findings, Finding{
				Type:     FindingPIIPattern,
				Severity: "medium",
				Target:   target,
				Evidence: redact(match),
			})
		}
	}

	actions := make([]Action, 0, len(findings))
	for i, finding := range findings {
		mode := ModeAutoSafe
		command := "redact/quarantine affected log segment"
		reason := "redaction and quarantine preserve evidence while removing accidental exposure"
		if finding.Type == FindingSecretPattern {
			mode = ModeRequiresApproval
			command = "rotate exposed secret after owner approval"
			reason = "key rotation can break dependent services"
		}
		actions = append(actions, Action{
			ID:      fmt.Sprintf("hygiene-%d", i+1),
			Mode:    mode,
			Target:  finding.Target,
			Command: command,
			Reason:  reason,
			Verify:  "re-scan target and attach redacted evidence",
		})
	}

	status := "clean"
	if len(findings) > 0 {
		status = "findings"
	}
	return Report{Host: "local-text", Status: status, Findings: findings, Actions: actions}
}

func ScanHost(host string) Report {
	if _, ok := inventory.Find(host); !ok {
		return Report{
			Host:   host,
			Status: "unknown_node",
			Findings: []Finding{{
				Type:     FindingLogLeak,
				Severity: "error",
				Target:   host,
				Evidence: "node is not in inventory",
			}},
		}
	}

	command := `set -eu
roots="${MESHCLAW_HYGIENE_ROOTS:-/etc /home /opt /var/log}"
tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT
for root in $roots; do
  [ -d "$root" ] || continue
  find "$root" -xdev -maxdepth 4 -type f \( \
    -name ".env" -o -name "*.env" -o -name "*.log" -o -name "*.conf" -o -name "*.json" \
    -o -name "*.yaml" -o -name "*.yml" -o -name "*.ini" \
  \) -size -2M 2>/dev/null
done | sort -u | head -250 > "$tmp"
while IFS= read -r file; do
  [ -r "$file" ] || continue
  hits=$(
    grep -IEn -m 8 'api[_-]?key|secret|token|password|BEGIN (RSA |OPENSSH |EC |DSA )?PRIVATE KEY|[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}' "$file" 2>/dev/null \
    | sed -E 's/(api[_-]?key|secret|token|password)([[:space:]]*[:=][[:space:]]*)[^[:space:]\"'\'']+/\1\2[REDACTED]/Ig' \
    | sed -E 's/[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}/[EMAIL_REDACTED]/g' \
    | sed -E 's/eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}/[JWT_REDACTED]/g' \
    | head -8
  )
  if [ -n "$hits" ]; then
    printf 'FILE %s\n' "$file"
    printf '%s\n' "$hits"
  fi
done < "$tmp" | head -500`
	result := runtime.NewRunner().RunEvidence(host, command)
	if !result.Success {
		return Report{
			Host:   host,
			Status: "failed",
			Findings: []Finding{{
				Type:     FindingLogLeak,
				Severity: "error",
				Target:   host,
				Evidence: redact(result.Stderr + result.Stdout),
			}},
		}
	}

	return reportFromRemoteOutput(host, result.Stdout)
}

func reportFromRemoteOutput(host, output string) Report {
	output = strings.TrimSpace(output)
	if output == "" {
		return Report{Host: host, Status: "clean"}
	}

	var findings []Finding
	currentFile := host
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "FILE ") {
			currentFile = strings.TrimSpace(strings.TrimPrefix(line, "FILE "))
			continue
		}
		severity := "medium"
		findingType := FindingLogLeak
		lower := strings.ToLower(line)
		if strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "password") ||
			strings.Contains(lower, "api_key") || strings.Contains(lower, "api-key") || strings.Contains(lower, "private key") {
			severity = "high"
			findingType = FindingSecretPattern
		} else if strings.Contains(line, "[EMAIL_REDACTED]") {
			findingType = FindingPIIPattern
		}
		findings = append(findings, Finding{
			Type:     findingType,
			Severity: severity,
			Target:   currentFile,
			Evidence: redact(line),
		})
	}

	actions := []Action{}
	for i, finding := range findings {
		mode := ModeAutoSafe
		command := "restrict permissions and redact/quarantine affected log copy"
		reason := "remove accidental exposure from readable logs while preserving evidence"
		if finding.Type == FindingSecretPattern {
			mode = ModeRequiresApproval
			command = "rotate exposed secret and then remove/redact stale copy"
			reason = "secret rotation can break dependent services"
		}
		actions = append(actions, Action{
			ID:      fmt.Sprintf("hygiene-host-%d", i+1),
			Mode:    mode,
			Target:  finding.Target,
			Command: command,
			Reason:  reason,
			Verify:  "run hygiene-scan-host again and compare evidence",
		})
	}

	return Report{Host: host, Status: "findings", Findings: findings, Actions: actions}
}

func redact(value string) string {
	value = strings.TrimSpace(value)
	if len(value) <= 8 {
		return "[REDACTED]"
	}
	return value[:4] + "...[REDACTED]..." + value[len(value)-4:]
}
