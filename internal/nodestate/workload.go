package nodestate

import (
	"sort"
	"strconv"
	"strings"
)

func classifyProcessPurpose(command string) string {
	lower := strings.ToLower(command)
	switch {
	case strings.Contains(lower, "ollama runner"), strings.Contains(lower, "ollama serve"), strings.Contains(lower, "vllm"):
		return "llm_inference"
	case strings.Contains(lower, "rsync"), strings.Contains(lower, "rclone"), strings.Contains(lower, "syncthing"):
		return "file_sync"
	case strings.Contains(lower, "meshclaw agent run"), strings.Contains(lower, "agent-collector"):
		return "meshclaw_monitoring"
	case strings.Contains(lower, "meshclaw"):
		return "meshclaw_runtime"
	case strings.Contains(lower, "meshdb"):
		return "meshclaw_memory"
	case strings.Contains(lower, "open-webui"):
		return "ai_chat_ui"
	case strings.Contains(lower, "vsshd"), strings.Contains(lower, "vssh"):
		return "remote_execution"
	case strings.Contains(lower, "dockerd"), strings.Contains(lower, "containerd"), strings.Contains(lower, "docker"):
		return "container_runtime"
	case strings.Contains(lower, "postgres"), strings.Contains(lower, "mariadb"), strings.Contains(lower, "mysql"), strings.Contains(lower, "redis"), strings.Contains(lower, "mongod"):
		return "database"
	case strings.Contains(lower, "nginx"), strings.Contains(lower, "caddy"), strings.Contains(lower, "apache"), strings.Contains(lower, "traefik"):
		return "web_proxy"
	case strings.Contains(lower, "uvicorn"), strings.Contains(lower, "gunicorn"), strings.Contains(lower, "streamlit"), strings.Contains(lower, "python"), strings.Contains(lower, "node "), strings.Contains(lower, "npm "), strings.Contains(lower, "bun "):
		return "app_runtime"
	case strings.Contains(lower, "tailscale"), strings.Contains(lower, "wireguard"), strings.Contains(lower, "/wired"), strings.Contains(lower, "rustdesk"), strings.Contains(lower, "socat"):
		return "network_remote_access"
	case strings.Contains(lower, "sshd"):
		return "ssh_access"
	case strings.Contains(lower, "systemd"), strings.Contains(lower, "journald"), strings.Contains(lower, "cron"), strings.Contains(lower, "rsyslog"),
		strings.Contains(lower, "perfpowerservices"), strings.Contains(lower, "runningboardd"), strings.Contains(lower, "xprotectservice"),
		strings.Contains(lower, "syspolicyd"), strings.Contains(lower, "fileproviderd"), strings.Contains(lower, "cloudd"),
		strings.Contains(lower, "trustd"), strings.Contains(lower, "dataaccessd"), strings.Contains(lower, "fseventsd"),
		strings.Contains(lower, "filecoordinationd"),
		strings.Contains(lower, "snapd-desktop-integration"), strings.Contains(lower, "wireplumber"), strings.Contains(lower, "pipewire"),
		strings.Contains(lower, "/sbin/init"), strings.Contains(lower, "kworker/"):
		return "system_service"
	case strings.Contains(lower, "turnserver"):
		return "public_network_service"
	case strings.HasPrefix(lower, "ps "), strings.Contains(lower, " df "), strings.HasPrefix(lower, "df "), strings.Contains(lower, "mds_stores"), strings.Contains(lower, "mdworker"), strings.Contains(lower, "/mds"), strings.Contains(lower, "spotlight"):
		return "system_observer"
	case strings.Contains(lower, "chrome"), strings.Contains(lower, "safari"), strings.Contains(lower, "webkit"), strings.Contains(lower, "codex"), strings.Contains(lower, "claude"), strings.Contains(lower, "windowserver"):
		return "desktop_ai_or_browser"
	default:
		return "unknown"
	}
}

func classifyProcessResource(cpu, mem float64) string {
	switch {
	case cpu >= 100 || mem >= 20:
		return "high"
	case cpu >= 25 || mem >= 5:
		return "medium"
	default:
		return "low"
	}
}

func summarizeWorkloads(processes []ProcessState, docker DockerState, network NetworkState) []WorkloadState {
	byPurpose := map[string]*WorkloadState{}
	for _, process := range processes {
		purpose := firstNonEmpty(process.Purpose, "unknown")
		if purpose == "unknown" && process.CPUPct < 1 && process.MemPct < 1 {
			continue
		}
		item := workloadForPurpose(byPurpose, purpose)
		item.Processes++
		item.CPUPct += process.CPUPct
		item.MemPct += process.MemPct
		addUnique(&item.Examples, process.Command, 5)
	}

	if docker.Available {
		item := workloadForPurpose(byPurpose, "containerized_app")
		item.Processes += docker.Running
		for _, container := range docker.Containers {
			if strings.EqualFold(container.State, "running") {
				addUnique(&item.Containers, container.Name, 12)
				if container.Ports != "" {
					addUnique(&item.Examples, container.Name+" ports "+container.Ports, 5)
				}
			}
		}
		for _, port := range docker.Ports {
			if port.Public {
				addUnique(&item.Ports, port.Mapping, 20)
			}
		}
	}

	for _, listener := range network.Listeners {
		if !listener.Public {
			continue
		}
		item := workloadForPurpose(byPurpose, "public_network_service")
		addUnique(&item.Ports, listener.Protocol+"/"+listener.Port+" "+listener.Process, 24)
		if listener.Process != "" {
			addUnique(&item.Examples, listener.Process+" listening on "+listener.Address+":"+listener.Port, 8)
		}
	}

	values := make([]WorkloadState, 0, len(byPurpose))
	for _, item := range byPurpose {
		if item.Processes == 0 && len(item.Containers) == 0 && len(item.Ports) == 0 {
			continue
		}
		item.CPUPct = round1(item.CPUPct)
		item.MemPct = round1(item.MemPct)
		values = append(values, *item)
	}
	sort.SliceStable(values, func(i, j int) bool {
		left := values[i].CPUPct + values[i].MemPct
		right := values[j].CPUPct + values[j].MemPct
		if left == right {
			return values[i].Purpose < values[j].Purpose
		}
		return left > right
	})
	if len(values) > 12 {
		values = values[:12]
	}
	return values
}

func inferInventoryHint(report Report) InventoryHint {
	tags := []string{report.System.OS, report.System.Arch}
	roles := []string{}
	reasons := []string{}
	workerCount := 0

	for _, gpu := range report.GPUs {
		if gpu.Name != "" {
			addUnique(&tags, "gpu", 0)
			addUnique(&roles, "gpu-worker", 0)
			reasons = append(reasons, "GPU detected: "+gpu.Name)
			break
		}
	}
	for _, workload := range report.Workloads {
		switch workload.Purpose {
		case "llm_inference":
			addUnique(&tags, "llm", 0)
			addUnique(&roles, "llm-worker", 0)
			workerCount += maxInt(1, workload.Processes)
		case "ai_chat_ui":
			addUnique(&tags, "openwebui", 0)
			addUnique(&roles, "ai-ui", 0)
		case "containerized_app":
			addUnique(&tags, "docker", 0)
			addUnique(&roles, "app-host", 0)
			workerCount += workload.Processes
		case "database":
			addUnique(&tags, "database", 0)
			addUnique(&roles, "database-host", 0)
		case "meshclaw_monitoring", "meshclaw_runtime":
			addUnique(&tags, "meshclaw", 0)
			addUnique(&roles, "meshclaw-node", 0)
		case "meshclaw_memory":
			addUnique(&tags, "meshdb", 0)
			addUnique(&roles, "memory-node", 0)
		case "remote_execution":
			addUnique(&tags, "vssh", 0)
			addUnique(&roles, "execution-node", 0)
		case "public_network_service":
			addUnique(&tags, "public-service", 0)
		}
	}
	if report.Docker.Available {
		addUnique(&tags, "docker", 0)
	}
	if report.RemoteAccess.TailscaleRunning {
		addUnique(&tags, "tailscale", 0)
	}
	if report.RemoteAccess.VSSHListening {
		addUnique(&tags, "vssh", 0)
		addUnique(&roles, "execution-node", 0)
	}
	if report.NetworkHasPort("25") || report.NetworkHasPort("465") || report.NetworkHasPort("993") {
		addUnique(&tags, "mail", 0)
		addUnique(&roles, "mail-server", 0)
		reasons = append(reasons, "mail ports observed")
	}

	primary := "general-node"
	if len(roles) > 0 {
		primary = roles[0]
	}
	confidence := "medium"
	if len(reasons) > 0 || len(roles) >= 2 {
		confidence = "high"
	}
	return InventoryHint{
		PrimaryRole:     primary,
		SecondaryRoles:  rolesWithoutPrimary(roles, primary),
		Tags:            tags,
		Reason:          firstNonEmpty(strings.Join(firstNStrings(reasons, 3), "; "), workloadReason(report.Workloads)),
		Confidence:      confidence,
		ObservedWorkers: workerCount,
	}
}

func inferCapacityState(report Report) CapacityState {
	state := CapacityState{
		CPUHeadroom:     headroomFromLoad(report.System.Load1, float64(maxInt(1, report.System.CPUCount))),
		MemoryHeadroom:  headroomFromPct(report.System.MemoryPct),
		DiskHeadroom:    headroomFromPct(report.System.DiskPct),
		GPUHeadroom:     "unknown",
		Schedulable:     true,
		OverallPressure: "low",
	}
	if len(report.GPUs) > 0 {
		state.GPUHeadroom = "high"
		for _, gpu := range report.GPUs {
			if gpu.Utilization >= 85 || pct(float64(gpu.MemoryUsedMB), float64(gpu.MemoryTotalMB)) >= 85 {
				state.GPUHeadroom = "low"
				break
			}
			if gpu.Utilization >= 60 || pct(float64(gpu.MemoryUsedMB), float64(gpu.MemoryTotalMB)) >= 60 {
				state.GPUHeadroom = "medium"
			}
		}
	}
	if report.System.DiskPct >= 90 {
		state.Blockers = append(state.Blockers, "disk above 90 percent")
		state.AvoidUse = append(state.AvoidUse, "new large downloads", "model training", "large Docker builds")
	} else if report.System.DiskPct >= 80 {
		state.Blockers = append(state.Blockers, "disk above 80 percent")
		state.AvoidUse = append(state.AvoidUse, "large temporary artifacts")
	}
	if report.System.MemoryPct >= 90 {
		state.Blockers = append(state.Blockers, "memory above 90 percent")
		state.AvoidUse = append(state.AvoidUse, "memory-heavy LLM inference")
	}
	for _, finding := range report.Security {
		if finding.Severity == "high" {
			state.Blockers = append(state.Blockers, "high risk: "+finding.Title)
		}
	}
	if len(state.Blockers) > 0 {
		state.Schedulable = false
		state.OverallPressure = "high"
	} else if state.CPUHeadroom == "medium" || state.MemoryHeadroom == "medium" || state.DiskHeadroom == "medium" || state.GPUHeadroom == "medium" {
		state.OverallPressure = "medium"
	}
	if state.Schedulable {
		switch {
		case len(report.GPUs) > 0 && state.GPUHeadroom != "low":
			state.SuggestedUse = append(state.SuggestedUse, "LLM inference", "GPU jobs")
		case report.Docker.Available:
			state.SuggestedUse = append(state.SuggestedUse, "containerized services", "light automation")
		default:
			state.SuggestedUse = append(state.SuggestedUse, "light automation", "read-only monitoring")
		}
	}
	return state
}

func buildAIView(report Report) AIView {
	view := AIView{
		WhatThisNodeIs:     explainRole(report.Inventory.PrimaryRole),
		WhatIsRunning:      explainWorkloads(report.Workloads),
		CanUseFor:          append([]string{}, report.Capacity.SuggestedUse...),
		ShouldAvoid:        append([]string{}, report.Capacity.AvoidUse...),
		ImmediateConcerns:  explainConcerns(report),
		RecommendedActions: explainRecommendedActions(report),
		InterpretationTips: []string{
			"Use ai_view for a quick natural-language answer.",
			"Use inventory for role/tag decisions.",
			"Use capacity before scheduling new work.",
			"Use security and logs before making remediation plans.",
			"Do not treat public listeners as incidents until the service owner confirms intent.",
		},
	}
	if len(view.CanUseFor) == 0 {
		view.CanUseFor = []string{"read-only monitoring"}
	}
	if len(view.ShouldAvoid) == 0 {
		view.ShouldAvoid = []string{"no obvious avoid list from current snapshot"}
	}
	view.PlainSummary = buildPlainSummary(report, view)
	return view
}

func workloadForPurpose(values map[string]*WorkloadState, purpose string) *WorkloadState {
	if item, ok := values[purpose]; ok {
		return item
	}
	item := &WorkloadState{Purpose: purpose}
	values[purpose] = item
	return item
}

func explainRole(role string) string {
	switch role {
	case "llm-worker":
		return "This node appears to be useful for local AI model inference."
	case "gpu-worker":
		return "This node has GPU capacity and can run GPU jobs if current load allows."
	case "ai-ui":
		return "This node appears to host an AI chat interface such as Open WebUI."
	case "app-host":
		return "This node appears to host containerized applications."
	case "database-host":
		return "This node appears to run database services."
	case "mail-server":
		return "This node appears to run mail-related services."
	case "memory-node":
		return "This node appears to store MeshClaw or meshdb memory/state."
	case "execution-node":
		return "This node appears reachable for remote execution through vssh or related tooling."
	case "meshclaw-node":
		return "This node appears to run MeshClaw monitoring or runtime components."
	default:
		return "This node has no strong specialized role yet."
	}
}

func explainWorkloads(workloads []WorkloadState) []string {
	if len(workloads) == 0 {
		return []string{"No dominant workload was observed in the current snapshot."}
	}
	lines := []string{}
	for _, workload := range firstNWorkloads(workloads, 5) {
		if workload.Purpose == "system_observer" && len(workloads) > 1 {
			continue
		}
		label := strings.ReplaceAll(workload.Purpose, "_", " ")
		detail := label
		if workload.Processes > 0 {
			detail += " using " + strconv.Itoa(workload.Processes) + " process(es)"
		}
		if workload.CPUPct > 0 || workload.MemPct > 0 {
			detail += " at about " + formatFloat1(workload.CPUPct) + "% CPU and " + formatFloat1(workload.MemPct) + "% memory"
		}
		if len(workload.Containers) > 0 {
			detail += "; containers: " + strings.Join(firstNStrings(workload.Containers, 3), ", ")
		}
		if len(workload.Ports) > 0 {
			detail += "; public ports observed"
		}
		lines = append(lines, detail)
	}
	if len(lines) == 0 {
		return []string{"No dominant workload was observed in the current snapshot."}
	}
	return lines
}

func explainConcerns(report Report) []string {
	concerns := []string{}
	for _, blocker := range report.Capacity.Blockers {
		concerns = append(concerns, blocker)
	}
	for _, finding := range report.Security {
		if finding.Severity == "high" || finding.Severity == "medium" {
			concern := finding.Severity + ": " + finding.Title
			if finding.Detail != "" {
				concern += " (" + finding.Detail + ")"
			}
			concerns = append(concerns, concern)
		}
		if len(concerns) >= 8 {
			break
		}
	}
	if len(concerns) == 0 {
		concerns = append(concerns, "No medium or high concern was detected in this snapshot.")
	}
	return concerns
}

func explainRecommendedActions(report Report) []string {
	actions := []string{}
	if !report.Capacity.Schedulable {
		actions = append(actions, "Do not schedule new heavy work here until blockers are reviewed.")
	}
	if report.System.DiskPct >= 80 {
		actions = append(actions, "Investigate disk usage before downloading models, building containers, or running training jobs.")
	}
	if report.System.MemoryPct >= 85 {
		actions = append(actions, "Check memory-heavy processes before starting another model or browser workload.")
	}
	if report.RemoteAccess.TailscaleAvailable && !report.RemoteAccess.TailscaleRunning {
		actions = append(actions, "Check Tailscale before relying on this node for remote execution.")
	}
	if !report.RemoteAccess.VSSHListening {
		actions = append(actions, "Check vssh daemon if this node should be controlled by MeshClaw.")
	}
	for _, finding := range report.Security {
		switch {
		case strings.Contains(strings.ToLower(finding.Title), "fail2ban"):
			actions = append(actions, "Review SSH exposure and fail2ban protection.")
		case strings.Contains(strings.ToLower(finding.Title), "public listening"):
			actions = append(actions, "Confirm public listening ports are intentional.")
		case strings.Contains(strings.ToLower(finding.Title), "failed systemd"):
			actions = append(actions, "Inspect failed systemd units before applying remediation.")
		}
		if len(actions) >= 8 {
			break
		}
	}
	if len(actions) == 0 {
		actions = append(actions, "Keep collecting snapshots and compare changes over time.")
	}
	return dedupeStrings(actions, 8)
}

func buildPlainSummary(report Report, view AIView) string {
	parts := []string{}
	parts = append(parts, report.NodeName+" is classified as "+firstNonEmpty(report.Inventory.PrimaryRole, "general-node"))
	if report.Capacity.OverallPressure != "" {
		parts = append(parts, "overall pressure is "+report.Capacity.OverallPressure)
	}
	if report.Capacity.Schedulable {
		parts = append(parts, "it is currently schedulable for "+strings.Join(firstNStrings(view.CanUseFor, 2), " and "))
	} else {
		parts = append(parts, "it is not a good target for new heavy work right now")
	}
	if len(view.ImmediateConcerns) > 0 && !strings.HasPrefix(view.ImmediateConcerns[0], "No ") {
		parts = append(parts, "main concern: "+view.ImmediateConcerns[0])
	}
	return strings.Join(parts, "; ") + "."
}

func dedupeStrings(values []string, limit int) []string {
	out := []string{}
	for _, value := range values {
		addUnique(&out, value, limit)
	}
	return out
}

func formatFloat1(value float64) string {
	return strconv.FormatFloat(round1(value), 'f', 1, 64)
}

func (report Report) NetworkHasPort(port string) bool {
	for _, listener := range report.Network.Listeners {
		if listener.Port == port {
			return true
		}
	}
	return false
}

func rolesWithoutPrimary(values []string, primary string) []string {
	out := []string{}
	for _, value := range values {
		if value != primary {
			addUnique(&out, value, 0)
		}
	}
	return out
}

func workloadReason(workloads []WorkloadState) string {
	if len(workloads) == 0 {
		return "no dominant workload observed yet"
	}
	parts := []string{}
	for _, workload := range firstNWorkloads(workloads, 3) {
		parts = append(parts, workload.Purpose)
	}
	return "observed workloads: " + strings.Join(parts, ", ")
}

func headroomFromPct(value float64) string {
	switch {
	case value >= 85:
		return "low"
	case value >= 65:
		return "medium"
	case value > 0:
		return "high"
	default:
		return "unknown"
	}
}

func headroomFromLoad(load, cpus float64) string {
	if cpus <= 0 || load <= 0 {
		return "unknown"
	}
	ratio := load / cpus
	switch {
	case ratio >= 0.85:
		return "low"
	case ratio >= 0.55:
		return "medium"
	default:
		return "high"
	}
}

func firstNStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return values[:limit]
}

func firstNWorkloads(values []WorkloadState, limit int) []WorkloadState {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return values[:limit]
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func addUnique(values *[]string, value string, limit int) {
	value = strings.TrimSpace(value)
	if value == "" || (limit > 0 && len(*values) >= limit) {
		return
	}
	for _, existing := range *values {
		if existing == value {
			return
		}
	}
	*values = append(*values, value)
}

func round1(value float64) float64 {
	if value == 0 {
		return 0
	}
	if value < 0 {
		return float64(int(value*10-0.5)) / 10
	}
	return float64(int(value*10+0.5)) / 10
}
