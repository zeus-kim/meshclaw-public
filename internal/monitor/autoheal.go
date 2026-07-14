package monitor

import (
	"fmt"
	"strings"
	"time"

	"github.com/meshclaw/meshclaw/internal/nodestate"
	"github.com/meshclaw/meshclaw/internal/runtime"
)

type AutoHealer struct {
	monitor     *Monitor
	enabled     bool
	lastActions map[string]time.Time // prevent action spam
	cooldown    time.Duration
}

type HealAction struct {
	Node             string    `json:"node"`
	Type             string    `json:"type"`
	Command          string    `json:"command"`
	Result           string    `json:"result"`
	Success          bool      `json:"success"`
	Transport        string    `json:"transport,omitempty"`
	ExitCode         int       `json:"exit_code,omitempty"`
	PolicyAction     string    `json:"policy_action,omitempty"`
	PolicyDecision   string    `json:"policy_decision,omitempty"`
	PolicyReason     string    `json:"policy_reason,omitempty"`
	ApprovalRequired bool      `json:"approval_required"`
	Skipped          bool      `json:"skipped"`
	SkipReason       string    `json:"skip_reason,omitempty"`
	Time             time.Time `json:"time"`
}

type HealPlanAction struct {
	Node             string  `json:"node"`
	Type             string  `json:"type"`
	Container        string  `json:"container,omitempty"`
	Severity         string  `json:"severity"`
	Metric           string  `json:"metric"`
	Value            float64 `json:"value"`
	Mode             string  `json:"mode"`
	Command          string  `json:"command"`
	Reason           string  `json:"reason"`
	Verify           string  `json:"verify"`
	PolicyAction     string  `json:"policy_action,omitempty"`
	PolicyDecision   string  `json:"policy_decision,omitempty"`
	PolicyReason     string  `json:"policy_reason,omitempty"`
	ApprovalRequired bool    `json:"approval_required"`
}

func NewAutoHealer(m *Monitor) *AutoHealer {
	return &AutoHealer{
		monitor:     m,
		enabled:     true,
		lastActions: make(map[string]time.Time),
		cooldown:    10 * time.Minute,
	}
}

func (h *AutoHealer) Enable()  { h.enabled = true }
func (h *AutoHealer) Disable() { h.enabled = false }

func (h *AutoHealer) Plan() []HealPlanAction {
	var actions []HealPlanAction
	states := h.monitor.GetAllStates()
	for _, state := range states {
		if !state.Online {
			actions = append(actions, HealPlanAction{
				Node:     state.Name,
				Type:     "connectivity",
				Severity: "critical",
				Metric:   "online",
				Value:    0,
				Mode:     "requires_approval",
				Command:  "meshclaw doctor " + state.Name,
				Reason:   "Node is offline or SSH/Tailscale path failed.",
				Verify:   "meshclaw monitor-check",
			})
			continue
		}
		if state.Disk >= 90 {
			actions = append(actions, HealPlanAction{
				Node:     state.Name,
				Type:     "disk_cleanup",
				Severity: "critical",
				Metric:   "disk_percent",
				Value:    state.Disk,
				Mode:     "auto_safe",
				Command:  diskCleanupCommand(),
				Reason:   "Root disk usage is critical; bounded cache, journal, temp, and Docker cleanup is safe to try first.",
				Verify:   "df -P /",
			})
			actions = append(actions, HealPlanAction{
				Node:     state.Name,
				Type:     "disk_investigate",
				Severity: "critical",
				Metric:   "disk_percent",
				Value:    state.Disk,
				Mode:     "read_only",
				Command:  "meshclaw disk-investigate " + state.Name + " /",
				Reason:   "Cleanup may not be enough; collect top-level disk evidence before any targeted deletion.",
				Verify:   "meshclaw monitor-check",
			})
		} else if state.Disk >= 80 {
			actions = append(actions, HealPlanAction{
				Node:     state.Name,
				Type:     "disk_investigate",
				Severity: "warning",
				Metric:   "disk_percent",
				Value:    state.Disk,
				Mode:     "read_only",
				Command:  "meshclaw disk-investigate " + state.Name + " /",
				Reason:   "Disk usage is high but not critical; inspect largest directories before cleanup.",
				Verify:   "df -P /",
			})
		}
		if state.Memory >= 95 {
			actions = append(actions, HealPlanAction{
				Node:     state.Name,
				Type:     "memory_cleanup",
				Severity: "warning",
				Metric:   "memory_percent",
				Value:    state.Memory,
				Mode:     "auto_safe",
				Command:  memoryCleanupCommand(),
				Reason:   "Memory usage is very high; dropping Linux page cache is bounded and non-destructive.",
				Verify:   "free -m",
			})
		}
	}
	return actions
}

func ContainerHealthPlan(node string, docker nodestate.DockerState) []HealPlanAction {
	if !docker.Available {
		return nil
	}
	var actions []HealPlanAction
	for _, container := range docker.Containers {
		action, ok := containerHealthPlanAction(node, container)
		if ok {
			actions = append(actions, action)
		}
	}
	return actions
}

func containerHealthPlanAction(node string, container nodestate.DockerContainer) (HealPlanAction, bool) {
	name := strings.TrimSpace(container.Name)
	if name == "" {
		return HealPlanAction{}, false
	}
	base := HealPlanAction{
		Node:      node,
		Type:      "container_restart",
		Container: name,
		Metric:    "container_health",
		Mode:      "propose",
		Command:   "docker restart " + shellToken(name),
		Verify:    "meshclaw agent collect --json && meshclaw analyze-logs " + shellToken(node) + " container:" + shellToken(name),
	}
	switch {
	case container.HealthStatus == "unhealthy":
		base.Severity = "high"
		base.Value = 1
		base.Reason = "Container health status is unhealthy; propose restart after reviewing logs and current workload impact."
		return base, true
	case strings.EqualFold(container.State, "exited") && container.ExitCode != 0:
		base.Severity = "high"
		base.Value = float64(container.ExitCode)
		base.Reason = fmt.Sprintf("Container exited with non-zero code %d; propose restart after inspecting container logs.", container.ExitCode)
		return base, true
	case container.OOMKilled:
		base.Severity = "critical"
		base.Value = 1
		base.Reason = "Container was OOM-killed; propose restart only after checking memory pressure and logs."
		return base, true
	case container.RestartCount >= 5:
		base.Severity = "warning"
		base.Value = float64(container.RestartCount)
		base.Reason = fmt.Sprintf("Container restart count is high (%d); propose restart/triage plan after log review.", container.RestartCount)
		return base, true
	default:
		return HealPlanAction{}, false
	}
}

func shellToken(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func (h *AutoHealer) CheckAndHeal() []HealAction {
	if !h.enabled {
		return nil
	}

	var actions []HealAction
	states := h.monitor.GetAllStates()

	for _, state := range states {
		if !state.Online {
			continue
		}

		// Disk critical (>90%) → auto cleanup
		if state.Disk >= 90 {
			if h.canAct(state.Name + ":disk") {
				action := h.healDisk(state)
				actions = append(actions, action)
			}
		}

		// Memory critical (>95%) → clear cache
		if state.Memory >= 95 {
			if h.canAct(state.Name + ":memory") {
				action := h.healMemory(state)
				actions = append(actions, action)
			}
		}
	}

	return actions
}

func (h *AutoHealer) canAct(key string) bool {
	if last, ok := h.lastActions[key]; ok {
		if time.Since(last) < h.cooldown {
			return false
		}
	}
	h.lastActions[key] = time.Now()
	return true
}

func (h *AutoHealer) healDisk(state *NodeState) HealAction {
	action := HealAction{
		Node: state.Name,
		Type: "disk_cleanup",
		Time: time.Now(),
	}

	action.Command = diskCleanupCommand()

	result := runtime.NewRunner().RunEvidence(state.Name, action.Command)
	applyRuntimeResult(&action, result)
	if result.Success {
		action.Result = "Cleanup completed"

		// Check new disk usage
		check := runtime.NewRunner().RunEvidence(state.Name, "df -h / | tail -1 | awk '{print $5}'")
		if check.Success {
			action.Result = fmt.Sprintf("Cleanup completed. Disk now: %s", strings.TrimSpace(check.Stdout))
		}
	}

	fmt.Printf("[autoheal] %s: %s → %s\n", state.Name, action.Type, action.Result)
	return action
}

func diskCleanupCommand() string {
	cleanupCmds := []string{
		"sudo apt-get clean 2>/dev/null || true",
		"sudo journalctl --vacuum-time=3d 2>/dev/null || true",
		"find /tmp -type f -atime +7 -delete 2>/dev/null || true",
		"rm -rf ~/.cache/pip 2>/dev/null || true",
		"docker system prune -f 2>/dev/null || true",
	}
	return strings.Join(cleanupCmds, " && ")
}

func (h *AutoHealer) healMemory(state *NodeState) HealAction {
	action := HealAction{
		Node: state.Name,
		Type: "memory_cleanup",
		Time: time.Now(),
	}

	action.Command = memoryCleanupCommand()

	result := runtime.NewRunner().RunEvidence(state.Name, action.Command)
	applyRuntimeResult(&action, result)
	if result.Success {
		action.Result = "Cache cleared"
	}

	fmt.Printf("[autoheal] %s: %s → %s\n", state.Name, action.Type, action.Result)
	return action
}

func memoryCleanupCommand() string {
	return "sync && echo 3 | sudo tee /proc/sys/vm/drop_caches > /dev/null 2>&1 || true"
}

// HealDiskNow forces immediate disk cleanup on a node
func (h *AutoHealer) HealDiskNow(nodeName string) HealAction {
	state := h.monitor.GetState(nodeName)
	if state == nil {
		return HealAction{
			Node:    nodeName,
			Type:    "disk_cleanup",
			Success: false,
			Result:  "Node not found",
			Time:    time.Now(),
		}
	}
	return h.healDisk(state)
}

// RestartService restarts a service on a node
func (h *AutoHealer) RestartService(nodeName, service string) HealAction {
	action := HealAction{
		Node:    nodeName,
		Type:    "restart_service",
		Command: fmt.Sprintf("sudo systemctl restart %s", service),
		Time:    time.Now(),
	}

	state := h.monitor.GetState(nodeName)
	if state == nil {
		action.Success = false
		action.Result = "Node not found"
		return action
	}

	result := runtime.NewRunner().RunEvidence(nodeName, action.Command)
	applyRuntimeResult(&action, result)
	if result.Success {
		action.Result = fmt.Sprintf("Service %s restarted", service)
	}

	fmt.Printf("[autoheal] %s: restart %s → %s\n", nodeName, service, action.Result)
	return action
}

// RunCommand executes a command on a node
func (h *AutoHealer) RunCommand(nodeName, command string) HealAction {
	action := HealAction{
		Node:    nodeName,
		Type:    "run_command",
		Command: command,
		Time:    time.Now(),
	}

	state := h.monitor.GetState(nodeName)
	if state == nil {
		action.Success = false
		action.Result = "Node not found"
		return action
	}

	result := runtime.NewRunner().RunEvidence(nodeName, command)
	applyRuntimeResult(&action, result)

	return action
}

func applyRuntimeResult(action *HealAction, result runtime.Evidence) {
	action.Success = result.Success
	action.Transport = result.Transport
	action.ExitCode = result.ExitCode
	action.Result = formatRuntimeResult(result)
}

func formatRuntimeResult(result runtime.Evidence) string {
	output := strings.TrimSpace(result.Stdout + result.Stderr)
	if result.Success {
		if output == "" {
			return fmt.Sprintf("completed via %s", result.Transport)
		}
		return output
	}
	if output == "" {
		return fmt.Sprintf("failed via %s exit=%d", result.Transport, result.ExitCode)
	}
	return fmt.Sprintf("failed via %s exit=%d\n%s", result.Transport, result.ExitCode, output)
}
