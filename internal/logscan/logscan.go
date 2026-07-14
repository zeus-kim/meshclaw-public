package logscan

import (
	"regexp"
	"strings"
)

type SourceType string

const (
	SourceHostJournal   SourceType = "host_journal"
	SourceContainerLogs SourceType = "container_logs"
)

type Source struct {
	Type      SourceType `json:"type"`
	Host      string     `json:"host,omitempty"`
	Container string     `json:"container,omitempty"`
	Name      string     `json:"name,omitempty"`
}

type Finding struct {
	Severity        string   `json:"severity"`
	Source          Source   `json:"source"`
	Pattern         string   `json:"pattern"`
	Count           int      `json:"count"`
	Sample          string   `json:"sample,omitempty"`
	UnitCandidates  []string `json:"unit_candidates,omitempty"`
	LikelyCause     string   `json:"likely_cause,omitempty"`
	SuggestedAction string   `json:"suggested_action,omitempty"`
}

type Options struct {
	MaxSamples int
}

type CommandSpec struct {
	Name string   `json:"name"`
	Args []string `json:"args,omitempty"`
}

var patterns = []struct {
	id              string
	severity        string
	likelyCause     string
	suggestedAction string
	match           *regexp.Regexp
}{
	{id: "oom", severity: "critical", likelyCause: "memory pressure or container memory limit exceeded", suggestedAction: "collect memory and restart-count evidence before planning resize, placement, or restart", match: regexp.MustCompile(`(?i)\b(out of memory|oomkilled|oom-killed|killed process)\b`)},
	{id: "crash_loop", severity: "high", likelyCause: "process exits repeatedly or supervisor keeps restarting it", suggestedAction: "inspect last exit reason, config, and dependency logs before any restart/recreate plan", match: regexp.MustCompile(`(?i)\b(crashloopbackoff|back-off restarting|restart loop|failed with result|main process exited|auto-restart)\b`)},
	{id: "healthcheck_failure", severity: "high", likelyCause: "container health/readiness probe is failing", suggestedAction: "compare healthcheck command, exposed port, startup time, and recent app errors before self-heal", match: regexp.MustCompile(`(?i)\b(healthcheck|health check|readiness|liveness)\b.*\b(fail|failed|unhealthy|timeout|refused)\b|\bunhealthy\b`)},
	{id: "port_bind_failure", severity: "high", likelyCause: "container or service cannot bind the requested port", suggestedAction: "inspect listener ownership and service registry before planning restart or route changes", match: regexp.MustCompile(`(?i)\b(address already in use|bind:.*address|listen tcp.*in use|port .*already allocated|cannot assign requested address)\b`)},
	{id: "dependency_connection_failure", severity: "high", likelyCause: "required upstream service, DNS, database, or network path is unavailable", suggestedAction: "verify dependency endpoint health before restarting the dependent container", match: regexp.MustCompile(`(?i)\b(connection refused|connection reset|no route to host|temporary failure in name resolution|dns lookup failed|database .*unavailable|redis .*refused|postgres .*refused|mysql .*refused)\b`)},
	{id: "image_pull_failure", severity: "high", likelyCause: "image tag, registry auth, architecture, or manifest is invalid", suggestedAction: "verify image reference, credentials, and node architecture before recreate or pull", match: regexp.MustCompile(`(?i)\b(imagepullbackoff|errimagepull|pull access denied|manifest unknown|no matching manifest|failed to pull image|repository does not exist)\b`)},
	{id: "exec_format_error", severity: "high", likelyCause: "binary or container image architecture does not match the node runtime", suggestedAction: "verify uname/GOARCH, image manifest, and deployed binary architecture before restart or recreate", match: regexp.MustCompile(`(?i)\b(exec format error|cannot execute binary file|wrong architecture|invalid ELF header)\b`)},
	{id: "working_directory_missing", severity: "high", likelyCause: "systemd WorkingDirectory or mounted application path is missing or unavailable", suggestedAction: "verify service WorkingDirectory, NAS/mount availability, and path ownership before restarting the unit", match: regexp.MustCompile(`(?i)\b(chdir|workingdirectory|working directory)\b.*\b(no such file or directory|not found|does not exist|failed)\b|\bCHDIR\b.*\bstatus=200\b`)},
	{id: "permission_failure", severity: "medium", likelyCause: "filesystem, device, user, or security policy denies access", suggestedAction: "inspect mounts, ownership, capabilities, and secret paths before restart", match: regexp.MustCompile(`(?i)\b(permission denied|operation not permitted|read-only file system|eacces|access denied)\b`)},
	{id: "disk_full", severity: "critical", likelyCause: "node or mounted volume has no writable space", suggestedAction: "run disk investigation and cleanup/volume guardrail plan before restarting the container", match: regexp.MustCompile(`(?i)\b(no space left on device|disk quota exceeded|filesystem full|write .* no space|enospc)\b`)},
	{id: "gpu_runtime_failure", severity: "high", likelyCause: "GPU runtime, CUDA memory, driver, or device visibility failed", suggestedAction: "collect nvidia-smi/runtime evidence and check placement before restarting GPU workloads", match: regexp.MustCompile(`(?i)\b(cuda out of memory|cuda error|nvidia-smi.*failed|failed to initialize nvml|no cuda-capable device|gpu.*unavailable)\b`)},
	{id: "dns_resolver_failure", severity: "high", likelyCause: "system resolver, DNS service, or upstream nameserver is failing", suggestedAction: "collect resolvectl/systemd-resolved status and dependency endpoint evidence before restarting dependent services", match: regexp.MustCompile(`(?i)\b(systemd-resolved|resolved\.service|resolvectl|nameserver|dns)\b.*\b(fail|failed|timeout|refused|unreachable|temporary failure|conflict|not responding)\b|\btemporary failure in name resolution\b`)},
	{id: "auth_failure", severity: "medium", likelyCause: "authentication or authorization attempts are failing", suggestedAction: "check exposed surface, credential scope, and fail2ban/security posture before service changes", match: regexp.MustCompile(`(?i)\b(failed password|authentication failure|invalid user|permission denied)\b`)},
	{id: "http_5xx", severity: "medium", likelyCause: "application or upstream returned server-side HTTP errors", suggestedAction: "correlate with service health, dependency errors, and route/LB evidence", match: regexp.MustCompile(`\bHTTP/[0-9."]*\s+5[0-9][0-9]\b|\bstatus[=:\s]+5[0-9][0-9]\b|\b5[0-9][0-9]\s+(internal server error|bad gateway|service unavailable|gateway timeout)\b`)},
}

var redactors = []struct {
	match *regexp.Regexp
	repl  string
}{
	{match: regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`), repl: "[redacted-email]"},
	{match: regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`), repl: "[redacted-ip]"},
	{match: regexp.MustCompile(`(?i)\b(token|api[_-]?key|secret|password|passwd|bearer)\s*[:=]\s*["']?[^"'\s]+`), repl: "$1=[redacted]"},
	{match: regexp.MustCompile(`(?i)\bauthorization:\s*bearer\s+[A-Za-z0-9._\-]+`), repl: "authorization: bearer [redacted]"},
}

var systemdUnitPattern = regexp.MustCompile(`\b([A-Za-z0-9_.@:-]+\.service)\b`)

func Analyze(source Source, text string, opts Options) []Finding {
	maxSamples := opts.MaxSamples
	if maxSamples <= 0 {
		maxSamples = 1
	}
	byPattern := map[string]*Finding{}
	order := []string{}
	sampleCounts := map[string]int{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		for _, pattern := range patterns {
			if !pattern.match.MatchString(line) {
				continue
			}
			finding, ok := byPattern[pattern.id]
			if !ok {
				finding = &Finding{
					Severity:        pattern.severity,
					Source:          source,
					Pattern:         pattern.id,
					LikelyCause:     pattern.likelyCause,
					SuggestedAction: pattern.suggestedAction,
				}
				byPattern[pattern.id] = finding
				order = append(order, pattern.id)
			}
			finding.Count++
			finding.UnitCandidates = appendUniqueStrings(finding.UnitCandidates, extractSystemdUnits(line)...)
			if sampleCounts[pattern.id] < maxSamples {
				appendSample(finding, Redact(line))
				sampleCounts[pattern.id]++
			}
		}
	}
	findings := make([]Finding, 0, len(order))
	for _, id := range order {
		findings = append(findings, *byPattern[id])
	}
	return findings
}

func Redact(text string) string {
	out := text
	for _, redactor := range redactors {
		out = redactor.match.ReplaceAllString(out, redactor.repl)
	}
	return out
}

func HostJournalCommand(since string) CommandSpec {
	if strings.TrimSpace(since) == "" {
		since = "1 hour ago"
	}
	return CommandSpec{Name: "journalctl", Args: []string{"-p", "err", "--since", since, "--no-pager"}}
}

func DockerLogsCommand(container string, tail int, since string) CommandSpec {
	args := []string{"logs"}
	if tail <= 0 {
		tail = 200
	}
	args = append(args, "--tail", intString(tail))
	if strings.TrimSpace(since) != "" {
		args = append(args, "--since", since)
	}
	args = append(args, container)
	return CommandSpec{Name: "docker", Args: args}
}

func appendSample(finding *Finding, sample string) {
	if finding.Sample == "" {
		finding.Sample = sample
		return
	}
	finding.Sample += "\n" + sample
}

func extractSystemdUnits(line string) []string {
	matches := systemdUnitPattern.FindAllStringSubmatch(line, -1)
	if len(matches) == 0 {
		return nil
	}
	units := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			units = appendUniqueStrings(units, match[1])
		}
	}
	return units
}

func appendUniqueStrings(values []string, additions ...string) []string {
	for _, addition := range additions {
		addition = strings.TrimSpace(addition)
		if addition == "" {
			continue
		}
		exists := false
		for _, value := range values {
			if value == addition {
				exists = true
				break
			}
		}
		if !exists {
			values = append(values, addition)
		}
	}
	return values
}

func intString(value int) string {
	if value == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	n := value
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
