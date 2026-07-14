package nodestate

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/meshclaw/meshclaw/internal/logscan"
	"github.com/meshclaw/meshclaw/internal/policy"
)

const ReportKind = "meshclaw_node_report"

type Options struct {
	TopProcesses int
	Services     []string
	Timeout      time.Duration
}

type Report struct {
	Kind         string            `json:"kind"`
	Version      string            `json:"version"`
	Hostname     string            `json:"hostname"`
	NodeName     string            `json:"node_name,omitempty"`
	CollectedAt  time.Time         `json:"collected_at"`
	Agent        AgentState        `json:"agent,omitempty"`
	Identity     IdentityState     `json:"identity,omitempty"`
	System       SystemState       `json:"system"`
	GPUs         []GPUState        `json:"gpus,omitempty"`
	Docker       DockerState       `json:"docker,omitempty"`
	Network      NetworkState      `json:"network,omitempty"`
	RemoteAccess RemoteAccessState `json:"remote_access,omitempty"`
	Firewall     FirewallState     `json:"firewall,omitempty"`
	Fail2Ban     Fail2BanState     `json:"fail2ban,omitempty"`
	Schedules    ScheduleState     `json:"schedules,omitempty"`
	Logins       LoginState        `json:"logins,omitempty"`
	Health       HostHealthState   `json:"health,omitempty"`
	Workloads    []WorkloadState   `json:"workloads,omitempty"`
	Processes    []ProcessState    `json:"processes,omitempty"`
	Services     []ServiceState    `json:"services,omitempty"`
	Logs         LogState          `json:"logs,omitempty"`
	Security     []SecurityFinding `json:"security,omitempty"`
	Inventory    InventoryHint     `json:"inventory,omitempty"`
	Capacity     CapacityState     `json:"capacity,omitempty"`
	AIView       AIView            `json:"ai_view,omitempty"`
	Errors       []string          `json:"errors,omitempty"`
}

type AgentState struct {
	SchemaVersion string `json:"schema_version,omitempty"`
	Collector     string `json:"collector,omitempty"`
	Binary        string `json:"binary,omitempty"`
	User          string `json:"user,omitempty"`
	TimeoutMS     int64  `json:"timeout_ms,omitempty"`
}

type IdentityState struct {
	Timezone             string `json:"timezone,omitempty"`
	BootIDFingerprint    string `json:"boot_id_fingerprint,omitempty"`
	MachineIDFingerprint string `json:"machine_id_fingerprint,omitempty"`
	Virtualization       string `json:"virtualization,omitempty"`
}

type SystemState struct {
	OS            string  `json:"os"`
	Arch          string  `json:"arch"`
	CPUCount      int     `json:"cpu_count"`
	Load1         float64 `json:"load1,omitempty"`
	Load5         float64 `json:"load5,omitempty"`
	Load15        float64 `json:"load15,omitempty"`
	MemoryTotalMB int64   `json:"memory_total_mb,omitempty"`
	MemoryUsedMB  int64   `json:"memory_used_mb,omitempty"`
	MemoryPct     float64 `json:"memory_pct,omitempty"`
	DiskTotalGB   int64   `json:"disk_total_gb,omitempty"`
	DiskUsedGB    int64   `json:"disk_used_gb,omitempty"`
	DiskPct       float64 `json:"disk_pct,omitempty"`
	UptimeSeconds int64   `json:"uptime_seconds,omitempty"`
}

type GPUState struct {
	Index         int     `json:"index"`
	Name          string  `json:"name"`
	Utilization   float64 `json:"utilization_pct,omitempty"`
	MemoryTotalMB int64   `json:"memory_total_mb,omitempty"`
	MemoryUsedMB  int64   `json:"memory_used_mb,omitempty"`
	TemperatureC  int     `json:"temperature_c,omitempty"`
}

type DockerState struct {
	Available  bool              `json:"available"`
	Running    int               `json:"running,omitempty"`
	Stopped    int               `json:"stopped,omitempty"`
	Images     int               `json:"images,omitempty"`
	Containers []DockerContainer `json:"containers,omitempty"`
	Ports      []DockerPort      `json:"ports,omitempty"`
}

type DockerContainer struct {
	Name          string `json:"name"`
	Image         string `json:"image,omitempty"`
	State         string `json:"state,omitempty"`
	Status        string `json:"status,omitempty"`
	Ports         string `json:"ports,omitempty"`
	HealthStatus  string `json:"health_status,omitempty"`
	RestartPolicy string `json:"restart_policy,omitempty"`
	RestartCount  int    `json:"restart_count,omitempty"`
	OOMKilled     bool   `json:"oom_killed,omitempty"`
	ExitCode      int    `json:"exit_code,omitempty"`
	StartedAt     string `json:"started_at,omitempty"`
}

type DockerPort struct {
	Container string `json:"container"`
	Mapping   string `json:"mapping"`
	Public    bool   `json:"public,omitempty"`
}

type NetworkState struct {
	Interfaces []NetworkInterface `json:"interfaces,omitempty"`
	Listeners  []NetworkListener  `json:"listeners,omitempty"`
	Routes     []NetworkRoute     `json:"routes,omitempty"`
	DNS        []string           `json:"dns,omitempty"`
}

type RemoteAccessState struct {
	TailscaleAvailable bool     `json:"tailscale_available,omitempty"`
	TailscaleRunning   bool     `json:"tailscale_running,omitempty"`
	TailscaleIPs       []string `json:"tailscale_ips,omitempty"`
	VSSHListening      bool     `json:"vssh_listening,omitempty"`
	VSSHPort           string   `json:"vssh_port,omitempty"`
	SSHListening       bool     `json:"ssh_listening,omitempty"`
}

type NetworkInterface struct {
	Name  string `json:"name"`
	State string `json:"state,omitempty"`
	IPv4  string `json:"ipv4,omitempty"`
	IPv6  string `json:"ipv6,omitempty"`
}

type NetworkListener struct {
	Protocol string `json:"protocol,omitempty"`
	Address  string `json:"address"`
	Port     string `json:"port,omitempty"`
	Process  string `json:"process,omitempty"`
	Public   bool   `json:"public,omitempty"`
}

type NetworkRoute struct {
	Destination string `json:"destination,omitempty"`
	Gateway     string `json:"gateway,omitempty"`
	Device      string `json:"device,omitempty"`
}

type FirewallState struct {
	UFW       string   `json:"ufw,omitempty"`
	Firewalld string   `json:"firewalld,omitempty"`
	PF        string   `json:"pf,omitempty"`
	Warnings  []string `json:"warnings,omitempty"`
}

type Fail2BanState struct {
	Available bool     `json:"available"`
	Running   bool     `json:"running,omitempty"`
	Jails     []string `json:"jails,omitempty"`
	Error     string   `json:"error,omitempty"`
}

type ScheduleState struct {
	CronEntries    int      `json:"cron_entries,omitempty"`
	CronSamples    []string `json:"cron_samples,omitempty"`
	SystemdTimers  []string `json:"systemd_timers,omitempty"`
	LaunchdAgents  []string `json:"launchd_agents,omitempty"`
	LaunchdDaemons []string `json:"launchd_daemons,omitempty"`
}

type LoginState struct {
	LoggedInUsers []string `json:"logged_in_users,omitempty"`
	RecentLogins  []string `json:"recent_logins,omitempty"`
}

type HostHealthState struct {
	RebootRequired bool           `json:"reboot_required,omitempty"`
	TimeSync       TimeSyncState  `json:"time_sync,omitempty"`
	Updates        UpdateState    `json:"updates,omitempty"`
	Swap           SwapState      `json:"swap,omitempty"`
	Mounts         []MountState   `json:"mounts,omitempty"`
	FailedUnits    []ServiceState `json:"failed_units,omitempty"`
	Auth           LogState       `json:"auth,omitempty"`
	Kernel         LogState       `json:"kernel,omitempty"`
	Warnings       []string       `json:"warnings,omitempty"`
}

type TimeSyncState struct {
	Available    bool   `json:"available,omitempty"`
	Synchronized bool   `json:"synchronized,omitempty"`
	NTPService   string `json:"ntp_service,omitempty"`
	Error        string `json:"error,omitempty"`
}

type UpdateState struct {
	Available       bool     `json:"available,omitempty"`
	UpgradableCount int      `json:"upgradable_count,omitempty"`
	Samples         []string `json:"samples,omitempty"`
	Error           string   `json:"error,omitempty"`
}

type SwapState struct {
	TotalMB int64   `json:"total_mb,omitempty"`
	UsedMB  int64   `json:"used_mb,omitempty"`
	UsedPct float64 `json:"used_pct,omitempty"`
}

type MountState struct {
	Filesystem string  `json:"filesystem,omitempty"`
	MountPoint string  `json:"mount_point"`
	UsedPct    float64 `json:"used_pct,omitempty"`
	InodePct   float64 `json:"inode_pct,omitempty"`
}

type ProcessState struct {
	PID           int     `json:"pid"`
	User          string  `json:"user,omitempty"`
	CPUPct        float64 `json:"cpu_pct,omitempty"`
	MemPct        float64 `json:"mem_pct,omitempty"`
	Purpose       string  `json:"purpose,omitempty"`
	ResourceClass string  `json:"resource_class,omitempty"`
	Command       string  `json:"command"`
}

type WorkloadState struct {
	Purpose    string   `json:"purpose"`
	Processes  int      `json:"processes,omitempty"`
	CPUPct     float64  `json:"cpu_pct,omitempty"`
	MemPct     float64  `json:"mem_pct,omitempty"`
	Examples   []string `json:"examples,omitempty"`
	Containers []string `json:"containers,omitempty"`
	Ports      []string `json:"ports,omitempty"`
}

type InventoryHint struct {
	PrimaryRole     string   `json:"primary_role,omitempty"`
	SecondaryRoles  []string `json:"secondary_roles,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	Reason          string   `json:"reason,omitempty"`
	Confidence      string   `json:"confidence,omitempty"`
	ObservedWorkers int      `json:"observed_workers,omitempty"`
}

type CapacityState struct {
	CPUHeadroom     string   `json:"cpu_headroom,omitempty"`
	MemoryHeadroom  string   `json:"memory_headroom,omitempty"`
	DiskHeadroom    string   `json:"disk_headroom,omitempty"`
	GPUHeadroom     string   `json:"gpu_headroom,omitempty"`
	Schedulable     bool     `json:"schedulable"`
	Blockers        []string `json:"blockers,omitempty"`
	SuggestedUse    []string `json:"suggested_use,omitempty"`
	AvoidUse        []string `json:"avoid_use,omitempty"`
	OverallPressure string   `json:"overall_pressure,omitempty"`
}

type AIView struct {
	PlainSummary       string   `json:"plain_summary,omitempty"`
	WhatThisNodeIs     string   `json:"what_this_node_is,omitempty"`
	WhatIsRunning      []string `json:"what_is_running,omitempty"`
	CanUseFor          []string `json:"can_use_for,omitempty"`
	ShouldAvoid        []string `json:"should_avoid,omitempty"`
	ImmediateConcerns  []string `json:"immediate_concerns,omitempty"`
	RecommendedActions []string `json:"recommended_actions,omitempty"`
	InterpretationTips []string `json:"interpretation_tips,omitempty"`
}

type ServiceState struct {
	Name   string `json:"name"`
	Active string `json:"active,omitempty"`
	State  string `json:"state,omitempty"`
	Error  string `json:"error,omitempty"`
}

type LogState struct {
	RecentErrorCount int               `json:"recent_error_count,omitempty"`
	Samples          []string          `json:"samples,omitempty"`
	LogFindings      []logscan.Finding `json:"log_findings,omitempty"`
}

type SecurityFinding struct {
	Category string `json:"category"`
	Severity string `json:"severity"`
	Title    string `json:"title"`
	Detail   string `json:"detail,omitempty"`
	Redacted bool   `json:"redacted,omitempty"`
}

func Collect(ctx context.Context, opts Options) Report {
	if opts.TopProcesses <= 0 {
		opts.TopProcesses = 8
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 3 * time.Second
	}
	hostname, _ := os.Hostname()
	exe, _ := os.Executable()
	report := Report{
		Kind:        ReportKind,
		Version:     "3",
		Hostname:    hostname,
		NodeName:    firstNonEmpty(os.Getenv("MESHCLAW_NODE_NAME"), hostname),
		CollectedAt: time.Now().UTC(),
		Agent: AgentState{
			SchemaVersion: "2026-05-26",
			Collector:     "meshclaw agent collect",
			Binary:        exe,
			User:          firstNonEmpty(os.Getenv("USER"), os.Getenv("LOGNAME")),
			TimeoutMS:     opts.Timeout.Milliseconds(),
		},
		System: SystemState{
			OS:       runtime.GOOS,
			Arch:     runtime.GOARCH,
			CPUCount: runtime.NumCPU(),
		},
	}

	c := collector{timeout: opts.Timeout}
	report.Identity = c.collectIdentity(ctx)
	report.System = c.collectSystem(ctx, report.System)
	report.GPUs = c.collectGPUs(ctx)
	report.Docker = c.collectDocker(ctx)
	report.Network = c.collectNetwork(ctx)
	report.RemoteAccess = c.collectRemoteAccess(ctx, report.Network)
	report.Firewall = c.collectFirewall(ctx)
	report.Fail2Ban = c.collectFail2Ban(ctx)
	report.Schedules = c.collectSchedules(ctx)
	report.Logins = c.collectLogins(ctx)
	report.Health = c.collectHealth(ctx)
	report.Health.Warnings = append(report.Health.Warnings, dockerHealthWarnings(report.Docker)...)
	report.Processes = c.collectProcesses(ctx, opts.TopProcesses)
	report.Workloads = summarizeWorkloads(report.Processes, report.Docker, report.Network)
	report.Services = c.collectServices(ctx, serviceNamesForCollect(opts.Services))
	report.Logs = c.collectLogs(ctx)
	report.Security = c.securityFindings(report)
	report.Inventory = inferInventoryHint(report)
	report.Capacity = inferCapacityState(report)
	report.AIView = buildAIView(report)
	report.Errors = c.errors
	return report
}

type collector struct {
	timeout time.Duration
	errors  []string
}

func (c *collector) run(ctx context.Context, name string, args ...string) (string, bool) {
	if !c.policyAllows(name, args...) {
		return "", false
	}
	cmdCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, name, args...)
	out, err := cmd.CombinedOutput()
	text := RedactText(strings.TrimSpace(string(out)))
	if cmdCtx.Err() == context.DeadlineExceeded {
		c.errors = append(c.errors, name+" timed out")
		return text, false
	}
	if err != nil {
		if text != "" {
			c.errors = append(c.errors, name+": "+truncateLine(text, 180))
		} else {
			c.errors = append(c.errors, name+": "+err.Error())
		}
		return text, false
	}
	return text, true
}

func (c *collector) runOptional(ctx context.Context, name string, args ...string) (string, bool) {
	if _, err := exec.LookPath(name); err != nil {
		return "", false
	}
	if !c.policyAllows(name, args...) {
		return "", false
	}
	cmdCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, name, args...)
	out, err := cmd.CombinedOutput()
	text := RedactText(strings.TrimSpace(string(out)))
	if cmdCtx.Err() == context.DeadlineExceeded {
		return text, false
	}
	if err != nil {
		return text, false
	}
	return text, true
}

func (c *collector) runQuiet(ctx context.Context, name string, args ...string) (string, bool) {
	if _, err := exec.LookPath(name); err != nil {
		return "", false
	}
	if !c.policyAllows(name, args...) {
		return "", false
	}
	cmdCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, name, args...)
	out, err := cmd.CombinedOutput()
	text := RedactText(strings.TrimSpace(string(out)))
	if cmdCtx.Err() == context.DeadlineExceeded || err != nil {
		return text, false
	}
	return text, true
}

func (c *collector) policyAllows(name string, args ...string) bool {
	command := strings.TrimSpace(name + " " + strings.Join(args, " "))
	result := policy.Evaluate(policy.Request{
		Subject:  "agent",
		Action:   "run_command",
		Resource: "server",
		Context:  command,
	})
	if result.Decision != policy.Allow {
		if result.Decision == policy.Deny {
			c.errors = append(c.errors, name+": blocked by policy: "+result.Reason)
		}
		return false
	}
	return true
}

func (c *collector) collectSystem(ctx context.Context, s SystemState) SystemState {
	if out, ok := c.run(ctx, "uptime"); ok {
		parseUptimeLoad(out, &s)
	}
	if runtime.GOOS == "linux" {
		parseLinuxMem(readFile("/proc/meminfo"), &s)
		parseLinuxUptime(readFile("/proc/uptime"), &s)
	} else if runtime.GOOS == "darwin" {
		parseDarwinMem(c.runText(ctx, "vm_stat"), c.runText(ctx, "sysctl", "-n", "hw.memsize"), &s)
		parseDarwinUptime(c.runText(ctx, "sysctl", "-n", "kern.boottime"), &s)
	}
	parseDisk(c.runText(ctx, "df", "-k", "/"), &s)
	return s
}

func (c *collector) runText(ctx context.Context, name string, args ...string) string {
	out, _ := c.run(ctx, name, args...)
	return out
}

func (c *collector) collectIdentity(ctx context.Context) IdentityState {
	state := IdentityState{}
	if out, ok := c.runQuiet(ctx, "date", "+%Z"); ok {
		state.Timezone = strings.TrimSpace(out)
	}
	if runtime.GOOS == "linux" {
		state.BootIDFingerprint = fingerprintString(readFile("/proc/sys/kernel/random/boot_id"))
		state.MachineIDFingerprint = fingerprintString(firstNonEmpty(readFile("/etc/machine-id"), readFile("/var/lib/dbus/machine-id")))
		if out, ok := c.runQuiet(ctx, "systemd-detect-virt"); ok {
			state.Virtualization = strings.TrimSpace(out)
		}
	} else if runtime.GOOS == "darwin" {
		if out, ok := c.runQuiet(ctx, "sysctl", "-n", "kern.boottime"); ok {
			state.BootIDFingerprint = fingerprintString(out)
		}
		if out, ok := c.runQuiet(ctx, "system_profiler", "SPHardwareDataType"); ok {
			state.MachineIDFingerprint = fingerprintString(out)
		}
	}
	return state
}

func (c *collector) collectGPUs(ctx context.Context) []GPUState {
	out, ok := c.runOptional(ctx, "nvidia-smi", "--query-gpu=index,name,utilization.gpu,memory.total,memory.used,temperature.gpu", "--format=csv,noheader,nounits")
	if !ok || out == "" {
		return nil
	}
	var gpus []GPUState
	for _, line := range strings.Split(out, "\n") {
		parts := splitTrim(line, ",")
		if len(parts) < 6 {
			continue
		}
		gpus = append(gpus, GPUState{
			Index:         atoi(parts[0]),
			Name:          parts[1],
			Utilization:   atof(parts[2]),
			MemoryTotalMB: int64(atoi(parts[3])),
			MemoryUsedMB:  int64(atoi(parts[4])),
			TemperatureC:  atoi(parts[5]),
		})
	}
	return gpus
}

func (c *collector) collectDocker(ctx context.Context) DockerState {
	out, ok := c.runOptional(ctx, "docker", "ps", "-a", "--format", "{{.Names}}\t{{.Image}}\t{{.State}}\t{{.Status}}\t{{.Ports}}")
	if !ok {
		return DockerState{Available: false}
	}
	state := DockerState{Available: true}
	names := []string{}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		container := DockerContainer{}
		if len(parts) > 0 {
			container.Name = parts[0]
		}
		if len(parts) > 1 {
			container.Image = parts[1]
		}
		if len(parts) > 2 {
			container.State = parts[2]
		}
		if len(parts) > 3 {
			container.Status = parts[3]
		}
		if len(parts) > 4 {
			container.Ports = parts[4]
			for _, port := range dockerPorts(container.Name, parts[4]) {
				if len(state.Ports) < 80 {
					state.Ports = append(state.Ports, port)
				}
			}
		}
		if container.Name != "" && len(state.Containers) < 30 {
			state.Containers = append(state.Containers, container)
			names = append(names, container.Name)
		}
		if strings.EqualFold(container.State, "running") {
			state.Running++
		} else {
			state.Stopped++
		}
	}
	if images, ok := c.runOptional(ctx, "docker", "images", "-q"); ok && images != "" {
		state.Images = len(nonEmptyLines(images))
	}
	mergeDockerInspect(&state, c.collectDockerInspect(ctx, names))
	return state
}

func (c *collector) collectDockerInspect(ctx context.Context, names []string) map[string]DockerContainer {
	if len(names) == 0 {
		return nil
	}
	if len(names) > 30 {
		names = names[:30]
	}
	format := "{{.Name}}\t{{if .State.Health}}{{.State.Health.Status}}{{else}}none{{end}}\t{{.RestartCount}}\t{{.State.OOMKilled}}\t{{.State.ExitCode}}\t{{.State.StartedAt}}\t{{.HostConfig.RestartPolicy.Name}}"
	args := append([]string{"inspect", "--format", format}, names...)
	out, ok := c.runOptional(ctx, "docker", args...)
	if !ok || out == "" {
		return nil
	}
	return parseDockerInspect(out)
}

func parseDockerInspect(out string) map[string]DockerContainer {
	values := map[string]DockerContainer{}
	for _, line := range nonEmptyLines(out) {
		parts := strings.Split(line, "\t")
		if len(parts) < 6 {
			continue
		}
		name := strings.TrimPrefix(strings.TrimSpace(parts[0]), "/")
		if name == "" {
			continue
		}
		values[name] = DockerContainer{
			Name:         name,
			HealthStatus: normalizeDockerHealth(parts[1]),
			RestartCount: atoi(parts[2]),
			OOMKilled:    strings.EqualFold(strings.TrimSpace(parts[3]), "true"),
			ExitCode:     atoi(parts[4]),
			StartedAt:    normalizeDockerStartedAt(parts[5]),
		}
		if len(parts) > 6 {
			info := values[name]
			info.RestartPolicy = normalizeDockerRestartPolicy(parts[6])
			values[name] = info
		}
	}
	return values
}

func mergeDockerInspect(state *DockerState, inspected map[string]DockerContainer) {
	if state == nil || len(inspected) == 0 {
		return
	}
	for i := range state.Containers {
		info, ok := inspected[state.Containers[i].Name]
		if !ok {
			continue
		}
		state.Containers[i].HealthStatus = info.HealthStatus
		state.Containers[i].RestartPolicy = info.RestartPolicy
		state.Containers[i].RestartCount = info.RestartCount
		state.Containers[i].OOMKilled = info.OOMKilled
		state.Containers[i].ExitCode = info.ExitCode
		state.Containers[i].StartedAt = info.StartedAt
	}
}

func normalizeDockerRestartPolicy(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "always", "unless-stopped", "on-failure", "no":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return ""
	}
}

func normalizeDockerHealth(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "healthy", "unhealthy", "starting":
		return strings.ToLower(strings.TrimSpace(value))
	case "", "<nil>", "nil":
		return "none"
	default:
		return "none"
	}
}

func normalizeDockerStartedAt(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "0001-01-01") {
		return ""
	}
	return value
}

func dockerHealthWarnings(state DockerState) []string {
	if !state.Available {
		return nil
	}
	warnings := []string{}
	for _, container := range state.Containers {
		switch {
		case container.HealthStatus == "unhealthy":
			warnings = append(warnings, "docker container "+container.Name+" is unhealthy")
		case container.OOMKilled:
			warnings = append(warnings, "docker container "+container.Name+" was OOM-killed")
		case container.RestartCount >= 5:
			warnings = append(warnings, "docker container "+container.Name+" has high restart count "+strconv.Itoa(container.RestartCount))
		case strings.EqualFold(container.State, "exited") && container.ExitCode != 0:
			warnings = append(warnings, "docker container "+container.Name+" exited with code "+strconv.Itoa(container.ExitCode))
		}
		if len(warnings) >= 8 {
			break
		}
	}
	return warnings
}

func dockerPorts(container, value string) []DockerPort {
	var ports []DockerPort
	for _, part := range strings.Split(value, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		ports = append(ports, DockerPort{
			Container: container,
			Mapping:   part,
			Public:    strings.Contains(part, "0.0.0.0:") || strings.Contains(part, "[::]:") || strings.Contains(part, ":::"),
		})
	}
	return ports
}

func (c *collector) collectNetwork(ctx context.Context) NetworkState {
	state := NetworkState{}
	if runtime.GOOS == "linux" {
		state.Interfaces = limitInterfaces(parseLinuxInterfaces(c.runText(ctx, "ip", "-br", "addr")), 40)
		state.Routes = parseLinuxRoutes(c.runText(ctx, "ip", "route", "show", "default"))
		if out, ok := c.runOptional(ctx, "ss", "-ltnup"); ok {
			state.Listeners = limitListeners(parseSSListeners(out), 80)
		} else if out, ok := c.runOptional(ctx, "netstat", "-ltnup"); ok {
			state.Listeners = limitListeners(parseNetstatListeners(out), 80)
		}
		state.DNS = parseResolvConf(readFile("/etc/resolv.conf"))
		return state
	}
	if runtime.GOOS == "darwin" {
		state.Interfaces = limitInterfaces(parseDarwinInterfaces(c.runText(ctx, "ifconfig")), 40)
		state.Routes = parseDarwinRoutes(c.runText(ctx, "route", "-n", "get", "default"))
		if out, ok := c.runOptional(ctx, "lsof", "-nP", "-iTCP", "-sTCP:LISTEN"); ok {
			state.Listeners = limitListeners(parseLsofListeners(out), 80)
		}
		state.DNS = parseDarwinDNS(c.runText(ctx, "scutil", "--dns"))
	}
	return state
}

func (c *collector) collectRemoteAccess(ctx context.Context, network NetworkState) RemoteAccessState {
	state := RemoteAccessState{}
	if _, err := exec.LookPath("tailscale"); err == nil {
		state.TailscaleAvailable = true
		if out, ok := c.runQuiet(ctx, "tailscale", "ip", "-4"); ok {
			for _, line := range nonEmptyLines(out) {
				state.TailscaleIPs = append(state.TailscaleIPs, strings.TrimSpace(line))
			}
			state.TailscaleRunning = len(state.TailscaleIPs) > 0
		}
	}
	for _, listener := range network.Listeners {
		switch listener.Port {
		case "22":
			state.SSHListening = true
		case "48291":
			state.VSSHListening = true
			state.VSSHPort = listener.Port
		}
	}
	return state
}

func (c *collector) collectFirewall(ctx context.Context) FirewallState {
	state := FirewallState{}
	if runtime.GOOS == "linux" {
		if out, ok := c.runOptional(ctx, "ufw", "status"); ok {
			state.UFW = firstLine(out)
			if strings.Contains(strings.ToLower(out), "inactive") {
				state.Warnings = append(state.Warnings, "ufw inactive")
			}
		}
		if out, ok := c.runOptional(ctx, "firewall-cmd", "--state"); ok {
			state.Firewalld = strings.TrimSpace(out)
		}
		return state
	}
	if runtime.GOOS == "darwin" {
		if out, ok := c.runOptional(ctx, "pfctl", "-s", "info"); ok {
			state.PF = firstLine(out)
		}
	}
	return state
}

func (c *collector) collectFail2Ban(ctx context.Context) Fail2BanState {
	if runtime.GOOS != "linux" {
		return Fail2BanState{}
	}
	state := Fail2BanState{}
	if _, err := exec.LookPath("fail2ban-client"); err != nil {
		return state
	}
	state.Available = true
	out, ok := c.runOptional(ctx, "fail2ban-client", "status")
	if !ok {
		state.Error = truncateLine(out, 180)
		return state
	}
	state.Running = true
	state.Jails = parseFail2BanJails(out)
	return state
}

func (c *collector) collectSchedules(ctx context.Context) ScheduleState {
	state := ScheduleState{}
	if runtime.GOOS == "linux" {
		if out, ok := c.runOptional(ctx, "crontab", "-l"); ok {
			for _, line := range nonEmptyLines(out) {
				if strings.HasPrefix(strings.TrimSpace(line), "#") {
					continue
				}
				state.CronEntries++
				if len(state.CronSamples) < 5 {
					state.CronSamples = append(state.CronSamples, truncateLine(RedactText(line), 180))
				}
			}
		}
		if out, ok := c.runOptional(ctx, "systemctl", "list-timers", "--all", "--no-pager", "--no-legend"); ok {
			for _, line := range nonEmptyLines(out) {
				if len(state.SystemdTimers) >= 10 {
					break
				}
				state.SystemdTimers = append(state.SystemdTimers, truncateLine(line, 180))
			}
		}
		return state
	}
	if runtime.GOOS == "darwin" {
		state.LaunchdAgents = listPlists(os.ExpandEnv("$HOME/Library/LaunchAgents"), 10)
		state.LaunchdDaemons = listPlists("/Library/LaunchDaemons", 10)
	}
	return state
}

func (c *collector) collectLogins(ctx context.Context) LoginState {
	state := LoginState{}
	if out, ok := c.runQuiet(ctx, "who"); ok {
		seen := map[string]bool{}
		for _, line := range nonEmptyLines(out) {
			fields := strings.Fields(line)
			if len(fields) == 0 {
				continue
			}
			user := RedactText(fields[0])
			if !seen[user] {
				seen[user] = true
				state.LoggedInUsers = append(state.LoggedInUsers, user)
			}
			if len(state.LoggedInUsers) >= 12 {
				break
			}
		}
	}
	if runtime.GOOS == "linux" || runtime.GOOS == "darwin" {
		if out, ok := c.runQuiet(ctx, "last", "-n", "5"); ok {
			for _, line := range nonEmptyLines(out) {
				if strings.Contains(strings.ToLower(line), "wtmp begins") {
					continue
				}
				state.RecentLogins = append(state.RecentLogins, truncateLine(RedactText(line), 180))
				if len(state.RecentLogins) >= 5 {
					break
				}
			}
		}
	}
	return state
}

func (c *collector) collectHealth(ctx context.Context) HostHealthState {
	state := HostHealthState{}
	if runtime.GOOS == "linux" {
		_, err := os.Stat("/var/run/reboot-required")
		state.RebootRequired = err == nil
		state.TimeSync = c.collectTimeSync(ctx)
		state.Updates = c.collectUpdates(ctx)
		state.Swap = parseSwap(readFile("/proc/meminfo"))
		state.Mounts = mergeMountInodes(parseMounts(c.runText(ctx, "df", "-Pk")), parseMounts(c.runText(ctx, "df", "-Pi")))
		state.FailedUnits = c.collectFailedUnits(ctx)
		state.Auth = c.collectAuthSignals(ctx)
		state.Kernel = c.collectKernelSignals(ctx)
	}
	return state
}

func (c *collector) collectTimeSync(ctx context.Context) TimeSyncState {
	out, ok := c.runQuiet(ctx, "timedatectl", "show", "--property=NTPSynchronized,NTP,NTPSynchronized,CanNTP")
	if !ok {
		return TimeSyncState{Error: truncateLine(out, 180)}
	}
	state := TimeSyncState{Available: true}
	for _, line := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "NTPSynchronized":
			state.Synchronized = value == "yes"
		case "NTP":
			state.NTPService = value
		}
	}
	return state
}

func (c *collector) collectUpdates(ctx context.Context) UpdateState {
	out, ok := c.runQuiet(ctx, "apt", "list", "--upgradable")
	if !ok {
		return UpdateState{}
	}
	lines := nonEmptyLines(out)
	state := UpdateState{Available: true}
	for _, line := range lines {
		if strings.HasPrefix(line, "Listing") {
			continue
		}
		state.UpgradableCount++
		if len(state.Samples) < 5 {
			state.Samples = append(state.Samples, truncateLine(line, 160))
		}
	}
	return state
}

func (c *collector) collectFailedUnits(ctx context.Context) []ServiceState {
	out, ok := c.runQuiet(ctx, "systemctl", "list-units", "--failed", "--no-legend", "--no-pager")
	if !ok || out == "" {
		return nil
	}
	var units []ServiceState
	for _, line := range nonEmptyLines(out) {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		units = append(units, ServiceState{Name: fields[0], Active: fields[2], State: fields[3]})
		if len(units) >= 20 {
			break
		}
	}
	return units
}

func (c *collector) collectAuthSignals(ctx context.Context) LogState {
	out, ok := c.runQuiet(ctx, "journalctl", "-u", "ssh", "-u", "sshd", "-n", "50", "--no-pager", "--output", "short-iso")
	if !ok || out == "" {
		return LogState{}
	}
	var samples []string
	count := 0
	for _, line := range nonEmptyLines(out) {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "failed password") || strings.Contains(lower, "invalid user") || strings.Contains(lower, "authentication failure") {
			count++
			if len(samples) < 5 {
				samples = append(samples, truncateLine(RedactText(line), 240))
			}
		}
	}
	return LogState{RecentErrorCount: count, Samples: samples}
}

func (c *collector) collectKernelSignals(ctx context.Context) LogState {
	out, ok := c.runQuiet(ctx, "journalctl", "-k", "-p", "warning", "-n", "30", "--no-pager", "--output", "short-iso")
	if !ok || out == "" {
		return LogState{}
	}
	lines := nonEmptyLines(out)
	var samples []string
	for _, line := range lines {
		if len(samples) >= 5 {
			break
		}
		samples = append(samples, truncateLine(RedactText(line), 240))
	}
	return LogState{RecentErrorCount: len(lines), Samples: samples}
}

func (c *collector) collectProcesses(ctx context.Context, limit int) []ProcessState {
	args := []string{"-axo", "pid=,user=,%cpu=,%mem=,command=", "-r"}
	if runtime.GOOS == "linux" {
		args = []string{"-eo", "pid=,user=,pcpu=,pmem=,command=", "--sort=-pcpu"}
	}
	out, ok := c.run(ctx, "ps", args...)
	if !ok || out == "" {
		return nil
	}
	var procs []ProcessState
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		cpu := atof(fields[2])
		mem := atof(fields[3])
		command := truncateLine(RedactText(strings.Join(fields[4:], " ")), 220)
		procs = append(procs, ProcessState{
			PID:           atoi(fields[0]),
			User:          fields[1],
			CPUPct:        cpu,
			MemPct:        mem,
			Purpose:       classifyProcessPurpose(command),
			ResourceClass: classifyProcessResource(cpu, mem),
			Command:       command,
		})
	}
	sort.SliceStable(procs, func(i, j int) bool { return procs[i].CPUPct > procs[j].CPUPct })
	if len(procs) > limit {
		procs = procs[:limit]
	}
	return procs
}

func (c *collector) collectServices(ctx context.Context, names []string) []ServiceState {
	if runtime.GOOS != "linux" || len(names) == 0 {
		return nil
	}
	var services []ServiceState
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		out, ok := c.runQuiet(ctx, "systemctl", "show", name, "--property=LoadState,ActiveState,SubState", "--no-page")
		service := ServiceState{Name: name}
		if ok {
			for _, line := range strings.Split(out, "\n") {
				key, value, found := strings.Cut(line, "=")
				if !found {
					continue
				}
				switch key {
				case "LoadState":
					if value == "not-found" {
						service.State = "not-found"
					}
				case "ActiveState":
					service.Active = value
				case "SubState":
					if service.State == "" {
						service.State = value
					}
				}
			}
		} else {
			service.Error = truncateLine(out, 180)
		}
		services = append(services, service)
	}
	return services
}

func serviceNamesForCollect(explicit []string) []string {
	if len(explicit) > 0 || runtime.GOOS != "linux" {
		return explicit
	}
	return []string{
		"ssh",
		"sshd",
		"docker",
		"containerd",
		"fail2ban",
		"tailscaled",
		"vsshd",
		"vssh",
		"ollama",
		"open-webui",
		"cron",
		"crond",
	}
}

func (c *collector) collectLogs(ctx context.Context) LogState {
	if runtime.GOOS != "linux" {
		return LogState{}
	}
	out, ok := c.run(ctx, "journalctl", "-p", "err", "-n", "20", "--no-pager", "--output", "short-iso")
	if !ok || out == "" {
		return LogState{}
	}
	lines := nonEmptyLines(out)
	samples := make([]string, 0, minInt(5, len(lines)))
	for _, line := range lines {
		if len(samples) >= 5 {
			break
		}
		samples = append(samples, truncateLine(RedactText(line), 240))
	}
	findings := logscan.Analyze(logscan.Source{Type: logscan.SourceHostJournal, Name: "journal"}, strings.Join(samples, "\n"), logscan.Options{MaxSamples: 2})
	return LogState{RecentErrorCount: len(lines), Samples: samples, LogFindings: findings}
}

func (c *collector) securityFindings(report Report) []SecurityFinding {
	var findings []SecurityFinding
	if report.System.DiskPct >= 90 {
		findings = append(findings, SecurityFinding{Category: "capacity", Severity: "high", Title: "root disk above 90 percent"})
	} else if report.System.DiskPct >= 80 {
		findings = append(findings, SecurityFinding{Category: "capacity", Severity: "medium", Title: "root disk above 80 percent"})
	}
	if report.Logs.RecentErrorCount > 0 {
		findings = append(findings, SecurityFinding{Category: "logs", Severity: "low", Title: "recent error logs present", Detail: strconv.Itoa(report.Logs.RecentErrorCount) + " recent error lines"})
	}
	publicListeners := 0
	for _, listener := range report.Network.Listeners {
		if listener.Public {
			publicListeners++
		}
	}
	if publicListeners > 0 {
		findings = append(findings, SecurityFinding{Category: "network", Severity: "medium", Title: "public listening ports present", Detail: strconv.Itoa(publicListeners) + " public listeners"})
	}
	publicDockerPorts := 0
	for _, port := range report.Docker.Ports {
		if port.Public {
			publicDockerPorts++
		}
	}
	if publicDockerPorts > 0 {
		findings = append(findings, SecurityFinding{Category: "docker", Severity: "medium", Title: "docker ports exposed on public interfaces", Detail: strconv.Itoa(publicDockerPorts) + " public Docker port mappings"})
	}
	if report.System.OS == "linux" && !report.Fail2Ban.Available && hasPublicPort(report.Network.Listeners, "22") {
		findings = append(findings, SecurityFinding{Category: "security", Severity: "high", Title: "SSH appears public and fail2ban is unavailable", Detail: "install/enable fail2ban or confirm another brute-force protection layer"})
	} else if report.System.OS == "linux" && report.Fail2Ban.Available && !report.Fail2Ban.Running {
		findings = append(findings, SecurityFinding{Category: "security", Severity: "medium", Title: "fail2ban installed but not reporting running status"})
	}
	if report.Health.RebootRequired {
		findings = append(findings, SecurityFinding{Category: "maintenance", Severity: "medium", Title: "reboot required"})
	}
	if report.Health.TimeSync.Available && !report.Health.TimeSync.Synchronized {
		findings = append(findings, SecurityFinding{Category: "time", Severity: "high", Title: "system clock is not synchronized"})
	}
	if report.Health.Updates.UpgradableCount > 0 {
		findings = append(findings, SecurityFinding{Category: "updates", Severity: "low", Title: "package updates available", Detail: strconv.Itoa(report.Health.Updates.UpgradableCount) + " packages"})
	}
	if report.Health.Swap.UsedPct >= 80 {
		findings = append(findings, SecurityFinding{Category: "memory", Severity: "medium", Title: "swap usage above 80 percent"})
	}
	for _, mount := range report.Health.Mounts {
		if mount.UsedPct >= 90 {
			findings = append(findings, SecurityFinding{Category: "storage", Severity: "high", Title: "mount usage above 90 percent", Detail: mount.MountPoint})
		} else if mount.InodePct >= 90 {
			findings = append(findings, SecurityFinding{Category: "storage", Severity: "high", Title: "inode usage above 90 percent", Detail: mount.MountPoint})
		}
	}
	if len(report.Health.FailedUnits) > 0 {
		findings = append(findings, SecurityFinding{Category: "systemd", Severity: "high", Title: "failed systemd units present", Detail: strconv.Itoa(len(report.Health.FailedUnits)) + " units"})
	}
	if report.Health.Auth.RecentErrorCount > 0 {
		findings = append(findings, SecurityFinding{Category: "auth", Severity: "medium", Title: "recent SSH authentication failures", Detail: strconv.Itoa(report.Health.Auth.RecentErrorCount) + " matching log lines"})
	}
	if report.Health.Kernel.RecentErrorCount > 0 {
		findings = append(findings, SecurityFinding{Category: "kernel", Severity: "medium", Title: "recent kernel warnings", Detail: strconv.Itoa(report.Health.Kernel.RecentErrorCount) + " recent kernel warning lines"})
	}
	for _, process := range report.Processes {
		if process.Purpose == "unknown" && (process.CPUPct >= 25 || process.MemPct >= 5) {
			findings = append(findings, SecurityFinding{Category: "process", Severity: "medium", Title: "unknown high-resource process", Detail: truncateLine(process.Command, 160)})
			break
		}
	}
	for _, warning := range report.Firewall.Warnings {
		severity := "low"
		if publicListeners > 0 {
			severity = "medium"
		}
		findings = append(findings, SecurityFinding{Category: "firewall", Severity: severity, Title: warning})
	}
	for _, sample := range report.Logs.Samples {
		if strings.Contains(sample, "[REDACTED") {
			findings = append(findings, SecurityFinding{Category: "redaction", Severity: "medium", Title: "sensitive-looking value redacted from logs", Redacted: true})
			break
		}
	}
	return findings
}

func limitInterfaces(values []NetworkInterface, limit int) []NetworkInterface {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return values[:limit]
}

func limitListeners(values []NetworkListener, limit int) []NetworkListener {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return values[:limit]
}

func hasPublicPort(listeners []NetworkListener, port string) bool {
	for _, listener := range listeners {
		if listener.Public && listener.Port == port {
			return true
		}
	}
	return false
}
