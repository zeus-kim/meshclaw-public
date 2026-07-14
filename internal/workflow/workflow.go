package workflow

import (
	"fmt"
	"strings"

	"github.com/meshclaw/meshclaw/internal/inventory"
	"github.com/meshclaw/meshclaw/internal/logscan"
	"github.com/meshclaw/meshclaw/internal/runtime"
)

type Finding struct {
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Evidence string `json:"evidence"`
	Next     string `json:"next"`
}

type Report struct {
	Name            string            `json:"name"`
	Host            string            `json:"host"`
	Status          string            `json:"status"`
	Findings        []Finding         `json:"findings"`
	LogFindings     []logscan.Finding `json:"log_findings,omitempty"`
	AutohealHandoff *AutohealHandoff  `json:"autoheal_handoff,omitempty"`
}

type AutohealHandoff struct {
	Decision                 string   `json:"decision"`
	Confidence               string   `json:"confidence,omitempty"`
	RecommendedTools         []string `json:"recommended_tools"`
	EvidenceRequired         []string `json:"evidence_required"`
	RuntimeEvidenceChecklist []string `json:"runtime_evidence_checklist,omitempty"`
	StopBefore               []string `json:"stop_before"`
	MustNot                  []string `json:"must_not"`
	RefreshTriggers          []string `json:"refresh_triggers"`
}

type SoftwareInventoryReport struct {
	Name       string            `json:"name"`
	Host       string            `json:"host"`
	Status     string            `json:"status"`
	OS         string            `json:"os,omitempty"`
	Kernel     string            `json:"kernel,omitempty"`
	Arch       string            `json:"arch,omitempty"`
	Tools      map[string]string `json:"tools,omitempty"`
	Services   []string          `json:"services,omitempty"`
	Containers []string          `json:"containers,omitempty"`
	GPU        []string          `json:"gpu,omitempty"`
	Error      string            `json:"error,omitempty"`
}

func SoftwareInventory(host string) SoftwareInventoryReport {
	if _, ok := inventory.Find(host); !ok {
		return SoftwareInventoryReport{Name: "software-inventory", Host: host, Status: "unknown_node", Error: "node is not in inventory"}
	}
	command := `export PATH="/Users/example/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin:$PATH"
echo "---system---"
if [ -f /etc/os-release ]; then . /etc/os-release; echo "os=${PRETTY_NAME:-$NAME}"; else echo "os=$(uname -s)"; fi
echo "kernel=$(uname -r 2>/dev/null)"
echo "arch=$(uname -m 2>/dev/null)"
echo "---tools---"
for tool in tailscale vssh meshclaw docker podman nerdctl kubectl helm k3s python3 go node npm uv ollama nvidia-smi; do
  if command -v "$tool" >/dev/null 2>&1; then
    version=$($tool --version 2>/dev/null | head -1)
    if [ -z "$version" ]; then version=$(command -v "$tool"); fi
    echo "$tool=$version"
  fi
done
echo "---services---"
if command -v systemctl >/dev/null 2>&1; then
  systemctl list-units --type=service --state=running --no-legend --plain 2>/dev/null | awk '{print $1}' | head -80
fi
echo "---containers---"
if command -v docker >/dev/null 2>&1; then
  docker ps --format '{{.Names}} {{.Image}}' 2>/dev/null | head -80
fi
echo "---gpu---"
if command -v nvidia-smi >/dev/null 2>&1; then
  nvidia-smi --query-gpu=name,memory.total,driver_version --format=csv,noheader 2>/dev/null | head -16
fi`
	result := runtime.NewRunner().RunEvidence(host, command)
	report := SoftwareInventoryReport{
		Name:       "software-inventory",
		Host:       host,
		Status:     "ok",
		Tools:      map[string]string{},
		Services:   []string{},
		Containers: []string{},
		GPU:        []string{},
	}
	if !result.Success {
		report.Status = "failed"
		report.Error = truncate(result.Stderr+result.Stdout, 1200)
		return report
	}
	parseSoftwareInventory(result.Stdout, &report)
	return report
}

func parseSoftwareInventory(output string, report *SoftwareInventoryReport) {
	section := ""
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "---") && strings.HasSuffix(line, "---") {
			section = strings.Trim(line, "-")
			continue
		}
		switch section {
		case "system":
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			switch key {
			case "os":
				report.OS = value
			case "kernel":
				report.Kernel = value
			case "arch":
				report.Arch = value
			}
		case "tools":
			key, value, ok := strings.Cut(line, "=")
			if ok {
				report.Tools[key] = value
			}
		case "services":
			report.Services = append(report.Services, line)
		case "containers":
			report.Containers = append(report.Containers, line)
		case "gpu":
			report.GPU = append(report.GPU, line)
		}
	}
}

func ProcessTop(host string) Report {
	if _, ok := inventory.Find(host); !ok {
		return unknownNodeReport("process-top", host)
	}
	command := `echo "---memory---"
ps -eo pid,ppid,user,comm,%mem,%cpu,rss --sort=-%mem 2>/dev/null | head -21 || ps aux 2>/dev/null | sort -nrk 4 | head -20
echo "---cpu---"
ps -eo pid,ppid,user,comm,%mem,%cpu,rss --sort=-%cpu 2>/dev/null | head -21 || ps aux 2>/dev/null | sort -nrk 3 | head -20`
	result := runtime.NewRunner().RunEvidence(host, command)
	if !result.Success {
		return Report{
			Name:   "process-top",
			Host:   host,
			Status: "failed",
			Findings: []Finding{{
				Severity: "error",
				Title:    "Process snapshot command failed",
				Evidence: truncate(result.Stderr+result.Stdout, 1200),
				Next:     "Run doctor and verify vssh/ssh read-only command execution.",
			}},
		}
	}
	output := strings.TrimSpace(result.Stdout)
	title := "Top process snapshot collected"
	status := "ok"
	severity := "info"
	if output == "" {
		title = "Process snapshot was empty"
		status = "findings"
		severity = "warning"
	}
	return Report{
		Name:   "process-top",
		Host:   host,
		Status: status,
		Findings: []Finding{{
			Severity: severity,
			Title:    title,
			Evidence: truncate(output, 5000),
			Next:     "Compare top memory/CPU processes with monitor-check and run service-check for repeated service names.",
		}},
	}
}

func ServiceCheck(host, service string) Report {
	if _, ok := inventory.Find(host); !ok {
		return unknownNodeReport("service-check", host)
	}
	command := serviceCheckCommand(service)
	result := runtime.NewRunner().RunEvidence(host, command)
	if !result.Success {
		return Report{
			Name:   "service-check",
			Host:   host,
			Status: "failed",
			Findings: []Finding{{
				Severity: "error",
				Title:    "Service check command failed",
				Evidence: truncate(result.Stderr+result.Stdout, 1200),
				Next:     "Run doctor and verify systemd/journal access.",
			}},
		}
	}
	output := summarizeServiceCheckOutput(strings.TrimSpace(result.Stdout))
	severity := "info"
	title := "Service snapshot collected"
	status := "ok"
	lower := strings.ToLower(output)
	if strings.Contains(lower, "active: inactive (dead)") && strings.Contains(lower, "disabled") {
		title = "Service is inactive and disabled"
	} else if strings.Contains(lower, "failed") || strings.Contains(lower, "inactive") || strings.Contains(lower, "auto-restart") || strings.Contains(lower, "status=203/exec") {
		severity = "warning"
		title = "Service has failed or inactive evidence"
		status = "findings"
	}
	return Report{
		Name:   "service-check",
		Host:   host,
		Status: status,
		Findings: []Finding{{
			Severity: severity,
			Title:    title,
			Evidence: truncate(output, 4000),
			Next:     "Use restart only after reviewing unit config and recent logs.",
		}},
	}
}

func serviceCheckCommand(service string) string {
	return fmt.Sprintf(`echo "---system-status---"
systemctl status %s --no-pager 2>/dev/null || true
echo "---system-unit---"
systemctl cat %s 2>/dev/null || true
echo "---system-logs---"
sudo journalctl -u %s -n 80 --no-pager 2>/dev/null || true
echo "---user-status---"
systemctl --user status %s --no-pager 2>/dev/null || true
echo "---user-unit---"
systemctl --user cat %s 2>/dev/null || true
echo "---user-logs---"
journalctl --user -u %s -n 80 --no-pager 2>/dev/null || true`, shellQuote(service), shellQuote(service), shellQuote(service), shellQuote(service), shellQuote(service), shellQuote(service))
}

func summarizeServiceCheckOutput(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return ""
	}
	var b strings.Builder
	section := ""
	counts := map[string]int{}
	order := []string{}
	flushLogs := func() {
		if len(order) == 0 {
			return
		}
		for _, msg := range order {
			if counts[msg] > 1 {
				fmt.Fprintf(&b, "%s (repeated %d times)\n", msg, counts[msg])
			} else {
				b.WriteString(msg)
				b.WriteByte('\n')
			}
		}
		counts = map[string]int{}
		order = nil
	}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "---") && strings.HasSuffix(line, "---") {
			if strings.HasSuffix(section, "logs") {
				flushLogs()
			}
			section = strings.Trim(line, "-")
			b.WriteString(line)
			b.WriteByte('\n')
			continue
		}
		if !strings.HasSuffix(section, "logs") {
			b.WriteString(line)
			b.WriteByte('\n')
			continue
		}
		msg := serviceJournalMessage(line)
		if _, ok := counts[msg]; !ok {
			order = append(order, msg)
		}
		counts[msg]++
	}
	if strings.HasSuffix(section, "logs") {
		flushLogs()
	}
	return strings.TrimSpace(b.String())
}

func ServiceAudit(host string) Report {
	if _, ok := inventory.Find(host); !ok {
		return unknownNodeReport("service-audit", host)
	}
	command := `echo "---failed---"
systemctl --failed --no-legend --plain 2>/dev/null || true
echo "---activating---"
systemctl list-units --type=service --state=activating,failed --no-legend --plain 2>/dev/null || true
echo "---recent-service-errors---"
if command -v journalctl >/dev/null 2>&1; then
  sudo journalctl -p warning..alert -n 200 --no-pager 2>/dev/null | egrep -i 'service: failed|failed with result|auto-restart|can.t open file|no such file|execstart|main process exited' | tail -80 || true
fi`
	result := runtime.NewRunner().RunEvidence(host, command)
	if !result.Success {
		return Report{
			Name:   "service-audit",
			Host:   host,
			Status: "failed",
			Findings: []Finding{{
				Severity: "error",
				Title:    "Service audit command failed",
				Evidence: truncate(result.Stderr+result.Stdout, 1200),
				Next:     "Run doctor and verify systemd/journal access.",
			}},
		}
	}
	rawOutput := strings.TrimSpace(result.Stdout)
	if rawOutput == "" || onlyEmptyServiceAudit(rawOutput) {
		return Report{
			Name:   "service-audit",
			Host:   host,
			Status: "ok",
			Findings: []Finding{{
				Severity: "info",
				Title:    "No failed or restarting service evidence found",
				Evidence: truncate(rawOutput, 1200),
				Next:     "Continue normal monitoring.",
			}},
		}
	}
	service := firstServiceName(rawOutput)
	next := "Run service-check for each failed service before any restart or quarantine."
	if service != "" {
		next = fmt.Sprintf("Run `meshclaw service-check %s %s`; if ExecStart target is missing, `meshclaw service-quarantine %s %s` is the safe path.", host, service, host, service)
	}
	output := summarizeServiceAuditOutput(rawOutput)
	return Report{
		Name:   "service-audit",
		Host:   host,
		Status: "findings",
		Findings: []Finding{{
			Severity: "warning",
			Title:    "Failed or restarting service evidence found",
			Evidence: truncate(output, 5000),
			Next:     next,
		}},
	}
}

func ServiceQuarantine(host, service string) Report {
	if _, ok := inventory.Find(host); !ok {
		return unknownNodeReport("service-quarantine", host)
	}
	command := serviceQuarantineCommand(service)
	result := runtime.NewRunner().RunEvidence(host, command)
	severity := "info"
	title := "Broken service quarantined"
	status := "ok"
	next := "Run service-check to confirm it no longer flaps."
	if !result.Success {
		severity = "warning"
		title = "Service quarantine did not apply"
		status = "blocked"
		next = "Review evidence; quarantine is refused unless ExecStart is missing or not executable."
	}
	return Report{
		Name:   "service-quarantine",
		Host:   host,
		Status: status,
		Findings: []Finding{{
			Severity: severity,
			Title:    title,
			Evidence: truncate(result.Stdout+result.Stderr, 3000),
			Next:     next,
		}},
	}
}

func serviceQuarantineCommand(service string) string {
	return fmt.Sprintf(`set -eu
service=%s
scope=system
exec_line=$(systemctl show "$service" -p ExecStart --value 2>/dev/null)
load_state=$(systemctl show "$service" -p LoadState --value 2>/dev/null || true)
if [ -z "$exec_line" ] || [ "$load_state" = "not-found" ]; then
  user_exec_line=$(systemctl --user show "$service" -p ExecStart --value 2>/dev/null || true)
  user_load_state=$(systemctl --user show "$service" -p LoadState --value 2>/dev/null || true)
  if [ -n "$user_exec_line" ] && [ "$user_load_state" != "not-found" ]; then
    scope=user
    exec_line="$user_exec_line"
  fi
fi
exec_path=$(printf '%%s\n' "$exec_line" | sed -n 's/.* path=\([^ ;]*\).*/\1/p' | head -1)
missing_arg=$(printf '%%s\n' "$exec_line" | sed -n 's/.* argv\[\]=[^ ]* \([^ ;]*\).*/\1/p' | head -1)
echo "service=$service"
echo "scope=$scope"
echo "exec_line=$exec_line"
echo "exec_path=$exec_path"
if [ -z "$exec_path" ]; then
  echo "no ExecStart path parsed; refusing quarantine"
  exit 2
fi
if [ -n "$missing_arg" ]; then
  echo "arg_path=$missing_arg"
  if [ -e "$missing_arg" ]; then
    echo "ExecStart argument exists; refusing quarantine"
    exit 3
  fi
elif [ -x "$exec_path" ]; then
  echo "ExecStart exists and is executable; refusing quarantine"
  exit 3
fi
if [ "$scope" = "user" ]; then
  systemctl --user disable --now "$service"
  systemctl --user is-enabled "$service" 2>/dev/null || true
  systemctl --user is-active "$service" 2>/dev/null || true
else
  sudo systemctl disable --now "$service"
  systemctl is-enabled "$service" 2>/dev/null || true
  systemctl is-active "$service" 2>/dev/null || true
fi`, shellQuote(service))
}

func ServiceRemove(host, service, path string) Report {
	if _, ok := inventory.Find(host); !ok {
		return unknownNodeReport("service-remove", host)
	}
	if path != "" && !strings.HasPrefix(path, "/") {
		return Report{
			Name:   "service-remove",
			Host:   host,
			Status: "invalid_path",
			Findings: []Finding{{
				Severity: "error",
				Title:    "Path must be absolute",
				Evidence: path,
				Next:     "Use an absolute service working directory path.",
			}},
		}
	}
	command := fmt.Sprintf(`set -eu
service=%s
remove_path=%s
unit_path=$(systemctl show "$service" -p FragmentPath --value 2>/dev/null || true)
workdir=$(systemctl show "$service" -p WorkingDirectory --value 2>/dev/null || true)
active=$(systemctl is-active "$service" 2>/dev/null || true)
enabled=$(systemctl is-enabled "$service" 2>/dev/null || true)
echo "service=$service"
echo "unit_path=$unit_path"
echo "workdir=$workdir"
echo "active_before=$active"
echo "enabled_before=$enabled"
if [ -z "$unit_path" ] || [ "$unit_path" = "n/a" ]; then
  if [ -z "$remove_path" ] || [ ! -e "$remove_path" ]; then
    echo "already_absent=true"
    exit 0
  fi
  echo "unit not found but remove path still exists; refusing remove"
  exit 2
fi
if [ -n "$remove_path" ]; then
  case "$remove_path" in
    /|/root|/home|/var|/usr|/etc|/opt|/srv|/tmp)
      echo "refusing broad path: $remove_path"
      exit 3
      ;;
  esac
  if [ "$workdir" != "$remove_path" ]; then
    echo "remove path does not match service WorkingDirectory"
    exit 4
  fi
fi
sudo systemctl disable --now "$service" 2>/dev/null || true
if printf '%%s\n' "$unit_path" | grep -q '^/etc/systemd/system/'; then
  sudo rm -f -- "$unit_path"
else
  echo "not removing non-local unit path: $unit_path"
fi
sudo systemctl daemon-reload
if [ -n "$remove_path" ] && [ -e "$remove_path" ]; then
  sudo rm -rf -- "$remove_path"
fi
echo "active_after=$(systemctl is-active "$service" 2>/dev/null || true)"
echo "enabled_after=$(systemctl is-enabled "$service" 2>/dev/null || true)"
if [ -n "$remove_path" ]; then
  [ ! -e "$remove_path" ] && echo "path_removed=true"
fi`, shellQuote(service), shellQuote(path))
	result := runtime.NewRunner().RunEvidence(host, command)
	status := "ok"
	severity := "info"
	title := "Service removed"
	if !result.Success {
		status = "failed"
		severity = "error"
		title = "Service remove failed"
	}
	return Report{
		Name:   "service-remove",
		Host:   host,
		Status: status,
		Findings: []Finding{{
			Severity: severity,
			Title:    title,
			Evidence: truncate(result.Stdout+result.Stderr, 5000),
			Next:     "Run service-check and monitor-check to verify removal.",
		}},
	}
}

func DataCleanPlan(host, path string) Report {
	if _, ok := inventory.Find(host); !ok {
		return unknownNodeReport("data-clean-plan", host)
	}
	if !strings.HasPrefix(path, "/") {
		return Report{
			Name:   "data-clean-plan",
			Host:   host,
			Status: "invalid_path",
			Findings: []Finding{{
				Severity: "error",
				Title:    "Path must be absolute",
				Evidence: path,
				Next:     "Use an absolute project path.",
			}},
		}
	}
	command := dataCleanPlanCommand(path)
	result := runtime.NewRunner().RunEvidence(host, command)
	status := "ok"
	severity := "info"
	title := "Data cleanup plan collected"
	if !result.Success {
		status = "failed"
		severity = "error"
		title = "Data cleanup plan failed"
	}
	return Report{
		Name:   "data-clean-plan",
		Host:   host,
		Status: status,
		Findings: []Finding{{
			Severity: severity,
			Title:    title,
			Evidence: truncate(result.Stdout+result.Stderr, 5000),
			Next:     "Run data-clean-apply only after confirming the manifest keeps clean/final outputs.",
		}},
	}
}

func dataCleanPlanCommand(path string) string {
	return fmt.Sprintf(`set -eu
root=%s
manifest=/tmp/meshclaw-data-clean-plan-$(hostname)-$(date -u +%%Y%%m%%dT%%H%%M%%SZ).txt
jsonl=${manifest%%.txt}.jsonl
min_kb=10240
: > "$manifest"
find "$root" -xdev \( \
  -path "*/site-packages" -o -path "*/site-packages/*" \
  -o -path "*/dist-packages" -o -path "*/dist-packages/*" \
  -o -path "*/.local/lib/python*" -o -path "*/.local/lib/python*/*" \
  -o -path "*/go/pkg/mod" -o -path "*/go/pkg/mod/*" \
  -o -path "*/go-install" -o -path "*/go-install/*" \
  -o -path "*/venv" -o -path "*/venv/*" \
  -o -path "*/venv_*" -o -path "*/venv_*/*" \
  -o -path "*/.venv" -o -path "*/.venv/*" \
  -o -path "*/env" -o -path "*/env/*" \
  -o -path "*/whisper_env" -o -path "*/whisper_env/*" \
  -o -path "*/node_modules" -o -path "*/node_modules/*" \
\) -prune -o \( \
  -type d \( -name "checkpoint-*" -o -name "checkpoints" -o -name "checkpoints_*" -o -name "__pycache__" -o -name ".pytest_cache" \) \
  -o -type f \( \
    -name "*.ckpt" -o -name "*checkpoint*.pt" -o -name "*checkpoint*.pth" -o -name "*checkpoint*.bin" \
    -o -name "*checkpoint*.safetensors" -o -name "*epoch*.pt" -o -name "*_light.txt" -o -name "*_pure.txt" \
    -o -name "*_mixed*.txt" -o -name "*_combined*.txt" -o -name "*_combined*.jsonl" \
    -o -name "*_new.jsonl" -o -name "chunk_*.npy" -o -name "*raw*.jsonl" -o -name "*raw*.txt" \
  \) \
\) -print | while IFS= read -r p; do
  case "$p" in
    *clean*|*final*|*/final_model.pt|*/jamovec_final.pt) continue ;;
  esac
  size_kb=$(du -sk "$p" 2>/dev/null | awk '{print $1}')
  [ -n "$size_kb" ] || continue
  [ "$size_kb" -ge "$min_kb" ] || continue
  printf '%%s\n' "$p"
done | sort -u > "$manifest"
if command -v python3 >/dev/null 2>&1; then
  python3 - "$manifest" "$jsonl" <<'PY'
import json
import os
import sys

manifest, jsonl = sys.argv[1], sys.argv[2]

def classify(path):
    base = os.path.basename(path)
    lowered = path.lower()
    if base in {"__pycache__", ".pytest_cache"}:
        return "cache", "low", "Python/test cache generated from source."
    if base.startswith("checkpoint-") or base.startswith("checkpoints"):
        return "model_checkpoint_dir", "approval_required", "Training checkpoint directory; preserve final/clean outputs before deletion."
    if "checkpoint" in base or base.endswith(".ckpt"):
        return "model_checkpoint_file", "approval_required", "Intermediate model checkpoint file."
    if "epoch" in base and base.endswith(".pt"):
        return "model_epoch_file", "approval_required", "Intermediate epoch model file."
    if "raw" in lowered and (base.endswith(".jsonl") or base.endswith(".txt")):
        return "raw_intermediate_data", "approval_required", "Raw/intermediate dataset artifact; confirm clean/final derivative exists."
    if base.startswith("chunk_") and base.endswith(".npy"):
        return "intermediate_numpy_chunk", "approval_required", "Generated chunk artifact."
    return "intermediate_artifact", "approval_required", "Matched cleanup pattern; manual review required."

total_kb = 0
count = 0
with open(jsonl, "w", encoding="utf-8") as out:
    with open(manifest, encoding="utf-8") as f:
        for line in f:
            path = line.rstrip("\n")
            if not path:
                continue
            try:
                st = os.stat(path)
            except OSError:
                size_kb = 0
                kind = "missing"
            else:
                size_kb = max(1, st.st_size // 1024)
                kind = "dir" if os.path.isdir(path) else "file"
                if kind == "dir":
                    try:
                        size_kb = int(os.popen("du -sk " + json.dumps(path)).read().split()[0])
                    except Exception:
                        pass
            category, risk, reason = classify(path)
            total_kb += size_kb
            count += 1
            out.write(json.dumps({
                "path": path,
                "kind": kind,
                "size_kb": size_kb,
                "category": category,
                "risk": risk,
                "reason": reason,
                "apply_mode": "manifest_only_after_review"
            }, ensure_ascii=False) + "\n")
print(f"jsonl={jsonl}")
print(f"structured_count={count}")
print(f"estimated_reclaim_kb={total_kb}")
PY
fi
echo "manifest=$manifest"
if [ -f "$jsonl" ]; then echo "jsonl=$jsonl"; fi
echo "min_size=10MiB"
while IFS= read -r p; do [ -e "$p" ] && du -sh "$p" 2>/dev/null || true; done < "$manifest" | sort -hr | head -200
echo "count=$(wc -l < "$manifest")"`, shellQuote(path))
}

func DataCleanApply(host, manifest string) Report {
	if _, ok := inventory.Find(host); !ok {
		return unknownNodeReport("data-clean-apply", host)
	}
	if !strings.HasPrefix(manifest, "/tmp/meshclaw-data-clean-plan-") {
		return Report{
			Name:   "data-clean-apply",
			Host:   host,
			Status: "invalid_manifest",
			Findings: []Finding{{
				Severity: "error",
				Title:    "Refusing untrusted manifest path",
				Evidence: manifest,
				Next:     "Use a manifest generated by data-clean-plan.",
			}},
		}
	}
	command := dataCleanApplyCommand(manifest)
	result := runtime.NewRunner().RunEvidence(host, command)
	status := "ok"
	severity := "info"
	title := "Data cleanup manifest applied"
	if !result.Success {
		status = "failed"
		severity = "error"
		title = "Data cleanup apply failed"
	}
	return Report{
		Name:   "data-clean-apply",
		Host:   host,
		Status: status,
		Findings: []Finding{{
			Severity: severity,
			Title:    title,
			Evidence: truncate(result.Stdout+result.Stderr, 5000),
			Next:     "Run disk-investigate and monitor-check to verify reclaimed space.",
		}},
	}
}

func dataCleanApplyCommand(manifest string) string {
	return fmt.Sprintf(`set -eu
manifest=%s
if [ ! -f "$manifest" ]; then
  echo "missing manifest: $manifest"
  exit 2
fi
jsonl=${manifest%%.txt}.jsonl
if [ -f "$jsonl" ] && command -v python3 >/dev/null 2>&1; then
  python3 - "$jsonl" <<'PY'
import json
import sys
from collections import Counter

path = sys.argv[1]
rows = []
with open(path, encoding="utf-8") as f:
    for line in f:
        line = line.strip()
        if line:
            rows.append(json.loads(line))
categories = Counter(row.get("category", "unknown") for row in rows)
risks = Counter(row.get("risk", "unknown") for row in rows)
total_kb = sum(int(row.get("size_kb") or 0) for row in rows)
print(f"structured_plan={path}")
print(f"structured_count={len(rows)}")
print(f"estimated_reclaim_kb={total_kb}")
print("categories=" + json.dumps(dict(categories), ensure_ascii=False, sort_keys=True))
print("risks=" + json.dumps(dict(risks), ensure_ascii=False, sort_keys=True))
PY
fi
backup="$HOME/meshclaw-data-clean-applied-$(date -u +%%Y%%m%%dT%%H%%M%%SZ).txt"
cp "$manifest" "$backup"
before=$(df -BG / | awk 'NR==2{print $3" used, "$4" free, "$5}')
count=0
while IFS= read -r p; do
  [ -e "$p" ] || continue
  case "$p" in
    *clean*|*final*|*/final_model.pt|*/jamovec_final.pt)
      echo "refuse_preserved_name $p"
      continue
      ;;
    */site-packages|*/site-packages/*|*/dist-packages|*/dist-packages/*|*/.local/lib/python*|*/go/pkg/mod|*/go/pkg/mod/*|*/go-install|*/go-install/*|*/venv|*/venv/*|*/venv_*|*/venv_*/*|*/.venv|*/.venv/*|*/env|*/env/*|*/whisper_env|*/whisper_env/*|*/node_modules|*/node_modules/*)
      echo "refuse_dependency_path $p"
      continue
      ;;
  esac
  base=$(basename "$p")
  if [ -d "$p" ]; then
    case "$base" in
      checkpoint-*|checkpoints|checkpoints_*|__pycache__|.pytest_cache) ;;
      *)
        echo "refuse_unrecognized_dir $p"
        continue
        ;;
    esac
  else
    case "$base" in
      *.ckpt|*checkpoint*.pt|*checkpoint*.pth|*checkpoint*.bin|*checkpoint*.safetensors|*epoch*.pt|*_light.txt|*_pure.txt|*_mixed*.txt|*_combined*.txt|*_combined*.jsonl|*_new.jsonl|chunk_*.npy|*raw*.jsonl|*raw*.txt) ;;
      *)
        echo "refuse_unrecognized_file $p"
        continue
        ;;
    esac
  esac
  rm -rf -- "$p"
  count=$((count+1))
done < "$manifest"
after=$(df -BG / | awk 'NR==2{print $3" used, "$4" free, "$5}')
echo "backup=$backup"
echo "deleted_count=$count"
echo "before=$before"
echo "after=$after"`, shellQuote(manifest))
}

func AnalyzeLogs(host, source string) Report {
	if _, ok := inventory.Find(host); !ok {
		return unknownNodeReport("analyze-logs", host)
	}
	command, logSource := analyzeLogsCommand(host, source)
	result := runtime.NewRunner().RunEvidence(host, command)
	if !result.Success {
		return Report{
			Name:   "analyze-logs",
			Host:   host,
			Status: "failed",
			Findings: []Finding{{
				Severity: "error",
				Title:    "Log analysis command failed",
				Evidence: truncate(result.Stderr+result.Stdout, 1200),
				Next:     "Run doctor and verify read access to the requested log source.",
			}},
		}
	}
	output := strings.TrimSpace(result.Stdout)
	logFindings := logscan.Analyze(logSource, output, logscan.Options{MaxSamples: 2})
	status := "ok"
	severity := "info"
	title := "No warning/error log evidence found"
	next := "Continue normal monitoring."
	if output != "" && output != "no supported system log source found" {
		status = "findings"
		severity = "warning"
		title = "Recent warning/error log evidence found"
		next = "Review cited lines and run a targeted service check if the same unit repeats."
	}
	if len(logFindings) > 0 {
		status = "findings"
		severity = highestLogSeverity(logFindings)
		title = "Structured logscan patterns found"
		next = "Review log_findings patterns, then run targeted service/container checks before remediation."
	}
	output = summarizeLogNoise(output)
	return Report{
		Name:            "analyze-logs",
		Host:            host,
		Status:          status,
		LogFindings:     logFindings,
		AutohealHandoff: analyzeLogsAutohealHandoff(logSource, logFindings),
		Findings: []Finding{{
			Severity: severity,
			Title:    title,
			Evidence: truncate(output, 3000),
			Next:     next,
		}},
	}
}

func analyzeLogsAutohealHandoff(source logscan.Source, findings []logscan.Finding) *AutohealHandoff {
	if len(findings) == 0 {
		return nil
	}
	handoff := &AutohealHandoff{
		Decision:   "plan_only_before_operator_approved_apply",
		Confidence: analyzeLogsAutohealConfidence(source, findings),
		EvidenceRequired: []string{
			"stored analyze-logs evidence path from this MCP call",
			"redacted log_findings patterns, samples, likely causes, and suggested actions",
			"fresh monitor or agent-collect evidence before any apply step",
		},
		StopBefore: []string{
			"building an apply plan without stored analyze-logs evidence",
			"requesting operator approval without fresh monitor or agent-collect evidence",
			"executing restart/recreate/rollback actions from logscan evidence alone",
		},
		MustNot: []string{
			"restart or recreate services/containers directly from log output",
			"treat old log findings as post-action verification",
			"copy unredacted secrets from raw logs into chat",
		},
		RefreshTriggers: []string{
			"new critical or high logscan finding appears",
			"target service/container state changes",
			"operator changes desired-state YAML or policy gates",
		},
	}
	if source.Type == logscan.SourceContainerLogs {
		handoff.RecommendedTools = []string{
			"meshclaw_autoheal_plan",
			"meshclaw_autoheal_container_apply_plan",
			"meshclaw_autoheal_container_readiness_summary",
		}
		handoff.EvidenceRequired = append(handoff.EvidenceRequired, "focused container-logscan evidence for container:"+source.Container)
		handoff.EvidenceRequired = append(handoff.EvidenceRequired, "fresh container runtime evidence for container:"+source.Container+" including image, status, health, ports, and restart policy")
		handoff.RuntimeEvidenceChecklist = containerRuntimeEvidenceChecklist(source)
		if source.Host != "" && source.Container != "" {
			handoff.EvidenceRequired = appendUniqueStrings(handoff.EvidenceRequired, "meshclaw_analyze_logs arguments for focused container logs: host="+source.Host+" source=container:"+source.Container)
		}
		handoff.StopBefore = append(handoff.StopBefore, "container apply-plan unless the focused container-logscan evidence names container:"+source.Container)
		handoff.StopBefore = append(handoff.StopBefore, "container apply-plan unless fresh runtime inspect/status evidence names container:"+source.Container)
		return handoff
	}
	handoff.RecommendedTools = []string{
		"meshclaw_service_check",
		"meshclaw_autoheal_plan",
		"meshclaw_reconcile_readiness_summary",
	}
	handoff.EvidenceRequired = append(handoff.EvidenceRequired,
		"targeted service-check evidence for repeated units",
		"unit identity evidence with service name and system/user scope",
	)
	if units := logscanUnitCandidates(findings); len(units) > 0 {
		handoff.EvidenceRequired = appendUniqueStrings(handoff.EvidenceRequired, "targeted service-check evidence for units: "+strings.Join(units, ","))
		if source.Host != "" {
			handoff.EvidenceRequired = appendUniqueStrings(handoff.EvidenceRequired, "meshclaw_service_check arguments for unit candidates: host="+source.Host+" service="+strings.Join(units, "|"))
		}
	}
	handoff.StopBefore = append(handoff.StopBefore, "service restart before unit identity and scope are confirmed")
	applySystemLogscanHandoffPatterns(handoff, findings)
	return handoff
}

func containerRuntimeEvidenceChecklist(source logscan.Source) []string {
	target := source.Container
	if target == "" {
		target = "<container>"
	}
	host := source.Host
	if host == "" {
		host = "<host>"
	}
	return []string{
		"docker inspect " + target + " state.status and state.running",
		"docker inspect " + target + " state.health when configured",
		"docker inspect " + target + " image id/name",
		"docker inspect " + target + " hostconfig.restartpolicy",
		"docker inspect " + target + " networksettings.ports",
		"docker ps --filter name=" + target + " on host " + host,
	}
}

func logscanUnitCandidates(findings []logscan.Finding) []string {
	var units []string
	for _, finding := range findings {
		units = appendUniqueStrings(units, finding.UnitCandidates...)
	}
	return units
}

func applySystemLogscanHandoffPatterns(handoff *AutohealHandoff, findings []logscan.Finding) {
	for _, finding := range findings {
		switch finding.Pattern {
		case "exec_format_error":
			handoff.RecommendedTools = appendUniqueStrings(handoff.RecommendedTools, "meshclaw_agent_workloads", "meshclaw_node_repair_plan")
			handoff.EvidenceRequired = appendUniqueStrings(handoff.EvidenceRequired, "node architecture and deployed binary/image architecture evidence")
			handoff.StopBefore = appendUniqueStrings(handoff.StopBefore, "restart/recreate before architecture mismatch is ruled out")
		case "working_directory_missing":
			handoff.RecommendedTools = appendUniqueStrings(handoff.RecommendedTools, "meshclaw_disk_investigate", "meshclaw_storage_guardrail_plan")
			handoff.EvidenceRequired = appendUniqueStrings(handoff.EvidenceRequired, "WorkingDirectory path, mount, and ownership evidence")
			handoff.StopBefore = appendUniqueStrings(handoff.StopBefore, "service restart before WorkingDirectory and mount evidence are checked")
		case "dns_resolver_failure":
			handoff.RecommendedTools = appendUniqueStrings(handoff.RecommendedTools, "meshclaw_agent_security", "meshclaw_service_registry_plan")
			handoff.EvidenceRequired = appendUniqueStrings(handoff.EvidenceRequired, "resolver status and dependency endpoint health evidence")
			handoff.StopBefore = appendUniqueStrings(handoff.StopBefore, "dependent service restart before resolver and endpoint evidence are checked")
		}
	}
}

func analyzeLogsAutohealConfidence(source logscan.Source, findings []logscan.Finding) string {
	highest := highestLogSeverity(findings)
	if source.Type == logscan.SourceContainerLogs && (highest == "critical" || highest == "high") {
		return "high"
	}
	if highest == "critical" || highest == "high" {
		return "medium"
	}
	return "low"
}

func appendUniqueStrings(values []string, additions ...string) []string {
	for _, addition := range additions {
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

func analyzeLogsCommand(host, source string) (string, logscan.Source) {
	source = strings.TrimSpace(source)
	if container := strings.TrimPrefix(source, "container:"); container != source {
		container = strings.TrimSpace(container)
		spec := logscan.DockerLogsCommand(container, 200, "1h")
		return shellCommand(spec.Name, spec.Args), logscan.Source{Type: logscan.SourceContainerLogs, Host: host, Container: container, Name: source}
	}
	command := `if command -v journalctl >/dev/null 2>&1; then
  sudo journalctl -p warning..alert -n 120 --no-pager 2>/dev/null
elif [ -f /var/log/syslog ]; then
  sudo tail -200 /var/log/syslog 2>/dev/null | egrep -i 'error|warn|fail|critical|panic' | tail -120
elif [ -f /var/log/messages ]; then
  sudo tail -200 /var/log/messages 2>/dev/null | egrep -i 'error|warn|fail|critical|panic' | tail -120
else
  echo "no supported system log source found"
fi`
	if source != "system" && source != "syslog" && source != "journal" {
		command = fmt.Sprintf("sudo tail -200 %s 2>/dev/null | egrep -i 'error|warn|fail|critical|panic' | tail -120", shellQuote(source))
	}
	return command, logscan.Source{Type: logscan.SourceHostJournal, Host: host, Name: source}
}

func shellCommand(name string, args []string) string {
	parts := []string{shellQuote(name)}
	for _, arg := range args {
		parts = append(parts, shellQuote(arg))
	}
	return strings.Join(parts, " ")
}

func highestLogSeverity(findings []logscan.Finding) string {
	rank := map[string]int{"info": 0, "warning": 1, "medium": 2, "high": 3, "critical": 4, "error": 4}
	highest := "info"
	for _, finding := range findings {
		if rank[finding.Severity] > rank[highest] {
			highest = finding.Severity
		}
	}
	if highest == "medium" {
		return "warning"
	}
	return highest
}

func summarizeLogNoise(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return output
	}
	var ufwCount int
	seen := map[string]int{}
	order := []string{}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "ufw block") || strings.Contains(lower, "[ufw block]") {
			ufwCount++
			continue
		}
		key := normalizeLogLine(line)
		if seen[key] == 0 {
			order = append(order, key)
		}
		seen[key]++
	}
	lines := []string{}
	if ufwCount > 0 {
		lines = append(lines, fmt.Sprintf("UFW/kernel firewall noise: %d lines suppressed. Run security-check for network-specific review.", ufwCount))
	}
	for _, key := range order {
		count := seen[key]
		if count > 1 {
			lines = append(lines, fmt.Sprintf("%s [repeated %d times]", key, count))
		} else {
			lines = append(lines, key)
		}
		if len(lines) >= 40 {
			lines = append(lines, "...additional log lines suppressed...")
			break
		}
	}
	return strings.Join(lines, "\n")
}

func normalizeLogLine(line string) string {
	fields := strings.Fields(line)
	if len(fields) > 3 {
		// Drop syslog timestamp prefix; it prevents repeated service failures from deduplicating.
		line = strings.Join(fields[3:], " ")
	}
	replacers := []string{"pid=", "Main PID:", "code=exited,", "status="}
	for _, marker := range replacers {
		line = normalizeTokenAfterMarker(line, marker)
	}
	return line
}

func normalizeTokenAfterMarker(line, marker string) string {
	idx := strings.Index(line, marker)
	if idx < 0 {
		return line
	}
	start := idx + len(marker)
	end := start
	for end < len(line) && line[end] != ' ' && line[end] != ',' && line[end] != ')' {
		end++
	}
	if end == start {
		return line
	}
	return line[:start] + "*" + line[end:]
}

func SecurityCheck(host string) Report {
	if _, ok := inventory.Find(host); !ok {
		return unknownNodeReport("security-check", host)
	}
	command := `echo "host=$(hostname)"
echo "---users-with-shell---"
awk -F: '$7 !~ /(false|nologin)$/ {print $1 ":" $3 ":" $7}' /etc/passwd 2>/dev/null
echo "---ssh-listeners---"
(ss -tulpen 2>/dev/null || netstat -tulpen 2>/dev/null) | egrep '(:22|:80|:443|LISTEN)' || true
echo "---failed-logins---"
(sudo journalctl _COMM=sshd -p info -n 80 --no-pager 2>/dev/null || sudo tail -120 /var/log/auth.log 2>/dev/null || true) | egrep -i 'failed|invalid|authentication failure' | tail -30 || true
echo "---sudoers-risk---"
sudo grep -R "NOPASSWD\|ALL=(ALL:ALL) ALL" /etc/sudoers /etc/sudoers.d 2>/dev/null || true
echo "---updates---"
if command -v apt-get >/dev/null 2>&1; then apt-get -s upgrade 2>/dev/null | awk '/^[0-9]+ upgraded/ {print}'; fi`
	result := runtime.NewRunner().RunEvidence(host, command)
	if !result.Success {
		return Report{
			Name:   "security-check",
			Host:   host,
			Status: "failed",
			Findings: []Finding{{
				Severity: "error",
				Title:    "Security check command failed",
				Evidence: truncate(result.Stderr+result.Stdout, 1200),
				Next:     "Run doctor and verify SSH/sudo read access.",
			}},
		}
	}
	output := strings.TrimSpace(result.Stdout)
	findings := securityFindings(output)
	return Report{
		Name:     "security-check",
		Host:     host,
		Status:   "findings",
		Findings: findings,
	}
}

func securityFindings(output string) []Finding {
	sections := splitSections(output)
	findings := []Finding{{
		Severity: "info",
		Title:    "Users with login shells",
		Evidence: truncate(sections["users-with-shell"], 1000),
		Next:     "Review unexpected interactive users.",
	}}
	listeners := strings.TrimSpace(sections["ssh-listeners"])
	findings = append(findings, Finding{
		Severity: "warning",
		Title:    "Open listeners",
		Evidence: truncate(listeners, 2000),
		Next:     "Confirm public listeners are intentional; prefer Tailscale-only for admin surfaces.",
	})
	failed := strings.TrimSpace(sections["failed-logins"])
	if failed == "" {
		findings = append(findings, Finding{
			Severity: "info",
			Title:    "No recent failed SSH login evidence",
			Evidence: "empty failed-login section",
			Next:     "Continue monitoring.",
		})
	} else {
		findings = append(findings, Finding{
			Severity: "warning",
			Title:    "Recent failed login evidence",
			Evidence: truncate(failed, 1500),
			Next:     "Consider rate limiting, key-only SSH, and closing public SSH where possible.",
		})
	}
	sudoers := strings.TrimSpace(sections["sudoers-risk"])
	if sudoers == "" {
		findings = append(findings, Finding{
			Severity: "info",
			Title:    "No sudoers risk pattern found",
			Evidence: "empty sudoers-risk section",
			Next:     "Continue monitoring.",
		})
	} else {
		findings = append(findings, Finding{
			Severity: "warning",
			Title:    "Sudoers review required",
			Evidence: truncate(sudoers, 1500),
			Next:     "Review NOPASSWD and broad ALL grants.",
		})
	}
	updates := strings.TrimSpace(sections["updates"])
	if updates == "" {
		updates = "no apt upgrade summary found"
	}
	findings = append(findings, Finding{
		Severity: "info",
		Title:    "Update summary",
		Evidence: truncate(updates, 1000),
		Next:     "Schedule updates if packages are pending.",
	})
	return findings
}

func splitSections(output string) map[string]string {
	sections := map[string]string{}
	current := "root"
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, "---") && strings.HasSuffix(line, "---") {
			current = strings.Trim(line, "-")
			sections[current] = ""
			continue
		}
		sections[current] += line + "\n"
	}
	return sections
}

func unknownNodeReport(name, host string) Report {
	return Report{
		Name:   name,
		Host:   host,
		Status: "unknown_node",
		Findings: []Finding{{
			Severity: "error",
			Title:    "Unknown node",
			Evidence: host,
			Next:     "Add the node to inventory before running this workflow.",
		}},
	}
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func onlyEmptyServiceAudit(output string) bool {
	trimmed := strings.TrimSpace(strings.ReplaceAll(output, "\n", ""))
	return trimmed == "---failed------activating------recent-service-errors---"
}

func summarizeServiceAuditOutput(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return ""
	}
	var b strings.Builder
	section := ""
	counts := map[string]int{}
	order := []string{}
	flushRecent := func() {
		if len(order) == 0 {
			return
		}
		for _, msg := range order {
			if counts[msg] > 1 {
				fmt.Fprintf(&b, "%s (repeated %d times)\n", msg, counts[msg])
			} else {
				b.WriteString(msg)
				b.WriteByte('\n')
			}
		}
		counts = map[string]int{}
		order = nil
	}
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "---") && strings.HasSuffix(line, "---") {
			if section == "recent-service-errors" {
				flushRecent()
			}
			section = strings.Trim(line, "-")
			b.WriteString(line)
			b.WriteByte('\n')
			continue
		}
		if section != "recent-service-errors" {
			b.WriteString(line)
			b.WriteByte('\n')
			continue
		}
		msg := serviceJournalMessage(line)
		if _, ok := counts[msg]; !ok {
			order = append(order, msg)
		}
		counts[msg]++
	}
	if section == "recent-service-errors" {
		flushRecent()
	}
	return strings.TrimSpace(b.String())
}

func serviceJournalMessage(line string) string {
	line = normalizeServiceJournalLine(line)
	if idx := strings.Index(line, "]: "); idx >= 0 {
		return strings.TrimSpace(line[idx+3:])
	}
	return strings.TrimSpace(line)
}

func normalizeServiceJournalLine(line string) string {
	if idx := strings.Index(line, "restart counter is at "); idx >= 0 {
		start := idx + len("restart counter is at ")
		end := start
		for end < len(line) && line[end] >= '0' && line[end] <= '9' {
			end++
		}
		return line[:start] + "<n>" + line[end:]
	}
	return line
}

func firstServiceName(output string) string {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "---") {
			continue
		}
		fields := strings.Fields(line)
		for _, field := range fields {
			field = strings.Trim(field, "●")
			if strings.HasSuffix(field, ".service") {
				return strings.TrimSuffix(field, ".service")
			}
		}
	}
	return ""
}

func truncate(value string, max int) string {
	value = strings.TrimSpace(value)
	if len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	return value[:max-3] + "..."
}
