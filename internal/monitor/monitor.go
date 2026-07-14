package monitor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/meshclaw/meshclaw/internal/inventory"
)

type Config struct {
	CheckInterval time.Duration `json:"check_interval"`
	NodeFilter    *regexp.Regexp
	SSHUsers      map[string]string // hostname -> user
}

type Monitor struct {
	config          *Config
	nodes           map[string]string // hostname -> IP
	tailscaleOnline map[string]bool
	states          map[string]*NodeState
	mu              sync.RWMutex
}

type NodeState struct {
	Name      string    `json:"name"`
	IP        string    `json:"ip"`
	Online    bool      `json:"online"`
	CPU       float64   `json:"cpu"`
	Memory    float64   `json:"memory"`
	Disk      float64   `json:"disk"`
	LastCheck time.Time `json:"last_check"`
	GPUUsage  float64   `json:"gpu_usage,omitempty"`
	GPUMemory float64   `json:"gpu_memory,omitempty"`
	Services  []string  `json:"services,omitempty"`
	Error     string    `json:"error,omitempty"`
}

type Alert struct {
	Level   string    `json:"level"`
	Node    string    `json:"node"`
	Type    string    `json:"type"`
	Message string    `json:"message"`
	Time    time.Time `json:"time"`
}

type NodeConfig struct {
	TailscaleFilter string                       `json:"tailscale_filter"`
	StaticNodes     map[string]map[string]string `json:"static_nodes"`
	SSHUsers        map[string]string            `json:"ssh_users"`
}

func DefaultConfig() *Config {
	home, _ := os.UserHomeDir()
	cfg := &Config{
		CheckInterval: 5 * time.Minute,
		NodeFilter:    regexp.MustCompile(`^(c[0-9]+|d[0-9]+|g[0-9]+|s[0-9]+|n[0-9]+|v[0-9]+|macmini)$`),
		SSHUsers:      make(map[string]string),
	}

	// Load from config file. Keep the old Argos path as a compatibility fallback
	// while MeshClaw settles on its server-operations control-plane shape.
	configPath := filepath.Join(home, ".meshclaw", "nodes.json")
	if data, err := os.ReadFile(configPath); err == nil {
		var nodeCfg NodeConfig
		if json.Unmarshal(data, &nodeCfg) == nil {
			if nodeCfg.TailscaleFilter != "" {
				cfg.NodeFilter = regexp.MustCompile(nodeCfg.TailscaleFilter)
			}
			if nodeCfg.SSHUsers != nil {
				cfg.SSHUsers = nodeCfg.SSHUsers
			}
		}
	} else if data, err := os.ReadFile(filepath.Join(home, ".argos", "nodes.json")); err == nil {
		var nodeCfg NodeConfig
		if json.Unmarshal(data, &nodeCfg) == nil {
			if nodeCfg.TailscaleFilter != "" {
				cfg.NodeFilter = regexp.MustCompile(nodeCfg.TailscaleFilter)
			}
			if nodeCfg.SSHUsers != nil {
				cfg.SSHUsers = nodeCfg.SSHUsers
			}
		}
	}
	for name, user := range loadWireUsers() {
		if _, exists := cfg.SSHUsers[name]; !exists {
			cfg.SSHUsers[name] = user
		}
	}
	for _, node := range inventory.DefaultNodes() {
		if _, exists := cfg.SSHUsers[node.Name]; !exists && node.User != "" {
			cfg.SSHUsers[node.Name] = node.User
		}
	}

	return cfg
}

func loadStaticNodes() map[string]map[string]string {
	home, _ := os.UserHomeDir()
	configPath := filepath.Join(home, ".meshclaw", "nodes.json")

	if data, err := os.ReadFile(configPath); err == nil {
		var nodeCfg NodeConfig
		if json.Unmarshal(data, &nodeCfg) == nil {
			return nodeCfg.StaticNodes
		}
	}
	if data, err := os.ReadFile(filepath.Join(home, ".argos", "nodes.json")); err == nil {
		var nodeCfg NodeConfig
		if json.Unmarshal(data, &nodeCfg) == nil {
			return nodeCfg.StaticNodes
		}
	}
	return nil
}

func loadWireUsers() map[string]string {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".wire", "users.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	users := map[string]string{}
	if json.Unmarshal(data, &users) != nil {
		return nil
	}
	return users
}

func New(cfg *Config) (*Monitor, error) {
	m := &Monitor{
		config:          cfg,
		nodes:           make(map[string]string),
		tailscaleOnline: make(map[string]bool),
		states:          make(map[string]*NodeState),
	}

	// Load Tailscale nodes
	m.loadNodesFromTailscale() // ignore error, VPS might not have tailscale

	// Add static VPS nodes from config
	staticNodes := loadStaticNodes()
	for name, info := range staticNodes {
		if ip, ok := info["ip"]; ok {
			m.nodes[name] = ip
		}
		if user, ok := info["user"]; ok {
			if m.config.SSHUsers == nil {
				m.config.SSHUsers = make(map[string]string)
			}
			m.config.SSHUsers[name] = user
		}
	}
	for _, node := range inventory.DefaultNodes() {
		if node.Tailscale != "" {
			m.nodes[node.Name] = node.Tailscale
			continue
		}
		if node.WireIP != "" {
			m.nodes[node.Name] = node.WireIP
		}
	}

	return m, nil
}

func (m *Monitor) loadNodesFromTailscale() error {
	out, err := exec.Command("tailscale", "status", "--json").Output()
	if err != nil {
		return fmt.Errorf("tailscale status failed: %w", err)
	}

	var status struct {
		Peer map[string]struct {
			HostName     string   `json:"HostName"`
			DNSName      string   `json:"DNSName"`
			TailscaleIPs []string `json:"TailscaleIPs"`
			Online       bool     `json:"Online"`
		} `json:"Peer"`
	}

	if err := json.Unmarshal(out, &status); err != nil {
		return err
	}

	for _, peer := range status.Peer {
		for _, hostname := range tailscalePeerNames(peer.HostName, peer.DNSName) {
			// Filter nodes by pattern
			if m.config.NodeFilter != nil && !m.config.NodeFilter.MatchString(hostname) {
				continue
			}
			if len(peer.TailscaleIPs) > 0 {
				m.nodes[hostname] = peer.TailscaleIPs[0]
			}
			m.tailscaleOnline[hostname] = peer.Online
		}
	}

	return nil
}

func tailscalePeerNames(hostname, dnsName string) []string {
	seen := map[string]bool{}
	var names []string
	add := func(value string) {
		value = strings.ToLower(strings.TrimSpace(value))
		value = strings.TrimSuffix(value, ".")
		if i := strings.IndexByte(value, '.'); i >= 0 {
			value = value[:i]
		}
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		names = append(names, value)
	}
	add(hostname)
	add(dnsName)
	return names
}

func (m *Monitor) getSSHUser(hostname string) string {
	if user, ok := m.config.SSHUsers[hostname]; ok {
		return user
	}
	return "operator"
}

func (m *Monitor) CheckAll() map[string]*NodeState {
	// Refresh nodes from Tailscale
	m.loadNodesFromTailscale()

	if os.Getenv("MESHCLAW_MONITOR_DISABLE_VSSH") != "1" {
		if states, ok := m.checkAllVSSH(); ok {
			m.mu.Lock()
			m.states = states
			m.mu.Unlock()
			return states
		}
		if states, ok := m.checkAllVSSHStatus(); ok {
			m.mu.Lock()
			m.states = states
			m.mu.Unlock()
			return states
		}
	}

	var wg sync.WaitGroup
	results := make(map[string]*NodeState)
	resultsMu := sync.Mutex{}

	for name, ip := range m.nodes {
		wg.Add(1)
		go func(name, ip string) {
			defer wg.Done()
			state := m.checkNode(name, ip)
			resultsMu.Lock()
			results[name] = state
			resultsMu.Unlock()
		}(name, ip)
	}

	wg.Wait()

	m.mu.Lock()
	m.states = results
	m.mu.Unlock()

	return results
}

type vsshFactsManyResult struct {
	Target string          `json:"target"`
	Result *vsshServerInfo `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

type vsshServerInfo struct {
	Hostname string          `json:"hostname"`
	OS       string          `json:"os"`
	Arch     string          `json:"arch"`
	CPUs     int             `json:"cpus"`
	Memory   *vsshMemoryInfo `json:"memory"`
	Load     *vsshLoadInfo   `json:"load"`
	Disk     []vsshDiskInfo  `json:"disk"`
	GPU      []vsshGPUInfo   `json:"gpu,omitempty"`
}

type vsshMemoryInfo struct {
	Total     int64 `json:"total_mb"`
	Used      int64 `json:"used_mb"`
	Free      int64 `json:"free_mb"`
	Available int64 `json:"available_mb"`
	Cached    int64 `json:"cached_mb"`
}

type vsshLoadInfo struct {
	Load1  float64 `json:"load_1"`
	Load5  float64 `json:"load_5"`
	Load15 float64 `json:"load_15"`
	CPUs   int     `json:"cpus"`
}

type vsshDiskInfo struct {
	Filesystem string `json:"filesystem"`
	Size       string `json:"size"`
	Used       string `json:"used"`
	Avail      string `json:"avail"`
	UsePercent string `json:"use_percent"`
	MountPoint string `json:"mount_point"`
}

type vsshGPUInfo struct {
	Index       int    `json:"index"`
	Name        string `json:"name"`
	MemoryTotal int64  `json:"memory_total_mb"`
	MemoryUsed  int64  `json:"memory_used_mb"`
	MemoryFree  int64  `json:"memory_free_mb"`
	Utilization int    `json:"utilization_percent"`
	Temperature int    `json:"temperature_c"`
}

func (m *Monitor) checkAllVSSH() (map[string]*NodeState, bool) {
	targets := make([]string, 0, len(m.nodes))
	for name := range m.nodes {
		if online, known := m.tailscaleOnline[name]; known && !online {
			continue
		}
		targets = append(targets, name)
	}
	if len(targets) == 0 {
		for name := range m.nodes {
			targets = append(targets, name)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), vsshFactsTimeout())
	defer cancel()
	binary := strings.TrimSpace(os.Getenv("MESHCLAW_VSSH_BINARY"))
	if binary == "" {
		binary = "vssh"
	}
	cmd := exec.CommandContext(ctx, binary, "facts-many", strings.Join(targets, ","))
	output, err := cmd.Output()
	if err != nil {
		return nil, false
	}

	var facts []vsshFactsManyResult
	if err := json.Unmarshal(output, &facts); err != nil {
		return nil, false
	}

	statusStates, _ := m.readVSSHStatusStates(binary)
	results := make(map[string]*NodeState)
	for _, item := range facts {
		ip := m.nodes[item.Target]
		if item.Result == nil || item.Error != "" || !vsshFactsHasMetrics(item.Result) {
			reason := item.Error
			if reason == "" {
				reason = "empty vssh facts result"
			}
			if info, ok := m.tryAlternateVSSHFacts(binary, item.Target, ip, reason); ok {
				results[item.Target] = nodeStateFromVSSHFacts(item.Target, ip, info)
				continue
			}
			if state, ok := onlineStatusFallback(item.Target, ip, reason, statusStates); ok {
				results[item.Target] = state
				continue
			}
			results[item.Target] = m.checkNode(item.Target, ip)
			if results[item.Target].Error == "" && reason != "" {
				results[item.Target].Error = reason
			}
			continue
		}
		results[item.Target] = nodeStateFromVSSHFacts(item.Target, ip, item.Result)
	}
	for name, ip := range m.nodes {
		if _, exists := results[name]; !exists {
			if state, ok := onlineStatusFallback(name, ip, "missing from vssh facts-many result", statusStates); ok {
				results[name] = state
				continue
			}
			state := &NodeState{Name: name, IP: ip, Online: false, LastCheck: time.Now()}
			if online, known := m.tailscaleOnline[name]; known && !online {
				state.Error = "offline according to Tailscale"
			} else {
				state = m.checkNode(name, ip)
			}
			results[name] = state
		}
	}
	return results, true
}

func vsshFactsHasMetrics(info *vsshServerInfo) bool {
	return info != nil && (info.Memory != nil || info.Load != nil || len(info.Disk) > 0 || len(info.GPU) > 0)
}

func (m *Monitor) tryAlternateVSSHFacts(binary, name, ip, reason string) (*vsshServerInfo, bool) {
	if !shouldTryAlternateVSSHPort(reason) {
		return nil, false
	}
	for _, target := range alternateVSSHTargets(name, ip) {
		info, ok := fetchVSSHFacts(binary, target)
		if ok {
			return info, true
		}
	}
	return nil, false
}

func shouldTryAlternateVSSHPort(reason string) bool {
	reason = strings.ToLower(reason)
	return strings.Contains(reason, "auth failed") ||
		strings.Contains(reason, "tls") ||
		strings.Contains(reason, "handshake") ||
		strings.Contains(reason, "no plaintext fallback")
}

func alternateVSSHTargets(name, ip string) []string {
	ports := strings.Split(strings.TrimSpace(os.Getenv("MESHCLAW_VSSH_ALT_PORTS")), ",")
	if len(ports) == 1 && strings.TrimSpace(ports[0]) == "" {
		ports = []string{"48292"}
	}
	var targets []string
	seen := map[string]bool{}
	add := func(target string) {
		target = strings.TrimSpace(target)
		if target == "" || seen[target] {
			return
		}
		seen[target] = true
		targets = append(targets, target)
	}
	for _, rawPort := range ports {
		port := strings.TrimSpace(rawPort)
		if port == "" {
			continue
		}
		add(name + ":" + port)
		if ip != "" && !strings.Contains(ip, ":") {
			add(ip + ":" + port)
		}
	}
	return targets
}

func fetchVSSHFacts(binary, target string) (*vsshServerInfo, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), vsshFactsTimeout())
	defer cancel()
	output, err := exec.CommandContext(ctx, binary, "facts", target).Output()
	if err != nil {
		return nil, false
	}
	var info vsshServerInfo
	if err := json.Unmarshal(output, &info); err != nil {
		return nil, false
	}
	if !vsshFactsHasMetrics(&info) {
		return nil, false
	}
	return &info, true
}

func onlineStatusFallback(name, fallbackIP, factsError string, statusStates map[string]*NodeState) (*NodeState, bool) {
	state, ok := statusStates[name]
	if !ok || state == nil || !state.Online {
		return nil, false
	}
	copied := *state
	if copied.IP == "" || copied.IP == "-" {
		copied.IP = fallbackIP
	}
	if factsError != "" {
		copied.Error = "vssh facts unavailable: " + factsError
	}
	return &copied, true
}

func (m *Monitor) checkAllVSSHStatus() (map[string]*NodeState, bool) {
	binary := strings.TrimSpace(os.Getenv("MESHCLAW_VSSH_BINARY"))
	if binary == "" {
		binary = "vssh"
	}
	states, ok := m.readVSSHStatusStates(binary)
	if !ok {
		return nil, false
	}
	for name, ip := range m.nodes {
		if _, exists := states[name]; !exists {
			states[name] = &NodeState{Name: name, IP: ip, Online: false, LastCheck: time.Now(), Error: "missing from vssh status"}
		}
		if m.tailscaleOnline[name] && !states[name].Online {
			states[name].Online = true
			states[name].IP = ip
			if states[name].Error == "" || strings.Contains(states[name].Error, "vssh status") || strings.Contains(states[name].Error, "missing from vssh status") {
				states[name].Error = "Tailscale online; vssh status path is stale or using a non-Tailscale endpoint"
			}
		}
	}
	return states, true
}

func (m *Monitor) readVSSHStatusStates(binary string) (map[string]*NodeState, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), vsshFactsTimeout())
	defer cancel()
	output, err := exec.CommandContext(ctx, binary, "status").Output()
	if err != nil || len(output) == 0 {
		return nil, false
	}
	states := parseVSSHStatus(output, m.nodes)
	if len(states) == 0 {
		return nil, false
	}
	return states, true
}

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func parseVSSHStatus(output []byte, known map[string]string) map[string]*NodeState {
	results := map[string]*NodeState{}
	now := time.Now()
	for _, rawLine := range strings.Split(string(output), "\n") {
		line := strings.TrimSpace(ansiPattern.ReplaceAllString(rawLine, ""))
		if line == "" || !(strings.HasPrefix(line, "●") || strings.HasPrefix(line, "○")) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		name := fields[1]
		ip := fields[2]
		if ip == "" || ip == "-" {
			if knownIP, ok := known[name]; ok && knownIP != "" {
				ip = knownIP
			}
		}
		state := &NodeState{
			Name:      name,
			IP:        ip,
			Online:    fields[0] == "●",
			LastCheck: now,
		}
		state.CPU = parseStatusFloat(fields[3])
		state.Memory = parseStatusPercent(fields[4])
		state.Disk = parseStatusPercent(fields[5])
		if !state.Online {
			state.Error = "offline according to vssh status"
		}
		results[name] = state
	}
	return results
}

func parseStatusFloat(value string) float64 {
	if value == "-" {
		return 0
	}
	parsed, _ := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return parsed
}

func parseStatusPercent(value string) float64 {
	if value == "-" {
		return 0
	}
	return parsePercent(value)
}

func vsshFactsTimeout() time.Duration {
	value := strings.TrimSpace(os.Getenv("MESHCLAW_VSSH_FACTS_TIMEOUT"))
	if value == "" {
		return 25 * time.Second
	}
	if seconds, err := strconv.ParseFloat(value, 64); err == nil && seconds > 0 {
		return time.Duration(seconds * float64(time.Second))
	}
	if duration, err := time.ParseDuration(value); err == nil && duration > 0 {
		return duration
	}
	return 6 * time.Second
}

func nodeStateFromVSSHFacts(name, ip string, info *vsshServerInfo) *NodeState {
	state := &NodeState{
		Name:      name,
		IP:        ip,
		Online:    true,
		LastCheck: time.Now(),
	}
	if info.Load != nil {
		state.CPU = info.Load.Load1
	}
	if info.Memory != nil && info.Memory.Total > 0 {
		state.Memory = float64(info.Memory.Used) * 100 / float64(info.Memory.Total)
	}
	state.Disk = rootDiskPercent(info.Disk)
	if len(info.GPU) > 0 {
		state.GPUUsage = float64(info.GPU[0].Utilization)
		if info.GPU[0].MemoryTotal > 0 {
			state.GPUMemory = float64(info.GPU[0].MemoryUsed) * 100 / float64(info.GPU[0].MemoryTotal)
		}
	}
	return state
}

func rootDiskPercent(disks []vsshDiskInfo) float64 {
	var fallback float64
	for _, disk := range disks {
		value := parsePercent(disk.UsePercent)
		if fallback == 0 {
			fallback = value
		}
		if disk.MountPoint == "/" {
			return value
		}
	}
	return fallback
}

func parsePercent(value string) float64 {
	value = strings.TrimSpace(strings.TrimSuffix(value, "%"))
	parsed, _ := strconv.ParseFloat(value, 64)
	return parsed
}

func (m *Monitor) checkNode(name, ip string) *NodeState {
	state := &NodeState{
		Name:      name,
		IP:        ip,
		LastCheck: time.Now(),
	}

	user := m.getSSHUser(name)

	cmd := exec.Command("ssh", "-o", "ConnectTimeout=5", "-o", "StrictHostKeyChecking=no",
		fmt.Sprintf("%s@%s", user, ip),
		"sh", "-s")
	cmd.Stdin = strings.NewReader(`echo online
df -P / | awk 'NR==2 {gsub("%", "", $5); print $5}'
if command -v free >/dev/null 2>&1; then
  free -m | awk 'NR==2 && $2 > 0 {printf "%.1f\n", $3*100/$2}'
elif command -v vm_stat >/dev/null 2>&1; then
  vm_stat | awk '
    /Pages active/ {gsub("\\.", "", $3); active=$3}
    /Pages inactive/ {gsub("\\.", "", $3); inactive=$3}
    /Pages speculative/ {gsub("\\.", "", $3); speculative=$3}
    /Pages wired down/ {gsub("\\.", "", $4); wired=$4}
    /Pages occupied by compressor/ {gsub("\\.", "", $5); comp=$5}
    /Pages free/ {gsub("\\.", "", $3); free=$3}
    END {
      used=active+wired+comp
      total=used+inactive+speculative+free
      if (total > 0) printf "%.1f\n", used*100/total
      else print "0"
    }'
else
  echo 0
fi
`)

	output, err := cmd.CombinedOutput()
	if err != nil {
		state.Online = false
		state.Error = strings.TrimSpace(err.Error() + " " + string(output))
		return state
	}

	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(lines) >= 1 && lines[0] == "online" {
		state.Online = true
	}
	if len(lines) >= 2 {
		fmt.Sscanf(strings.TrimSuffix(lines[1], "%"), "%f", &state.Disk)
	}
	if len(lines) >= 3 {
		fmt.Sscanf(lines[2], "%f", &state.Memory)
	}

	// GPU 상태 (nvidia-smi)
	gpuCmd := exec.Command("ssh", "-o", "ConnectTimeout=3",
		fmt.Sprintf("%s@%s", user, ip),
		"nvidia-smi --query-gpu=utilization.gpu,memory.used,memory.total --format=csv,noheader,nounits 2>/dev/null | head -1")

	if gpuOutput, err := gpuCmd.Output(); err == nil {
		parts := strings.Split(strings.TrimSpace(string(gpuOutput)), ",")
		if len(parts) >= 3 {
			fmt.Sscanf(strings.TrimSpace(parts[0]), "%f", &state.GPUUsage)
			var used, total float64
			fmt.Sscanf(strings.TrimSpace(parts[1]), "%f", &used)
			fmt.Sscanf(strings.TrimSpace(parts[2]), "%f", &total)
			if total > 0 {
				state.GPUMemory = (used / total) * 100
			}
		}
	}

	return state
}

func (m *Monitor) GetState(name string) *NodeState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.states[name]
}

func (m *Monitor) GetAllStates() map[string]*NodeState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	copy := make(map[string]*NodeState)
	for k, v := range m.states {
		copy[k] = v
	}
	return copy
}

func (m *Monitor) DetectAlerts() []Alert {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var alerts []Alert

	for _, state := range m.states {
		if !state.Online {
			alerts = append(alerts, Alert{
				Level:   "critical",
				Node:    state.Name,
				Type:    "offline",
				Message: fmt.Sprintf("Node %s is offline", state.Name),
				Time:    time.Now(),
			})
			continue
		}

		if state.Disk >= 90 {
			alerts = append(alerts, Alert{
				Level:   "critical",
				Node:    state.Name,
				Type:    "disk",
				Message: fmt.Sprintf("Disk usage critical: %.1f%%", state.Disk),
				Time:    time.Now(),
			})
		} else if state.Disk >= 80 {
			alerts = append(alerts, Alert{
				Level:   "warning",
				Node:    state.Name,
				Type:    "disk",
				Message: fmt.Sprintf("Disk usage high: %.1f%%", state.Disk),
				Time:    time.Now(),
			})
		}

		if state.Memory >= 90 {
			alerts = append(alerts, Alert{
				Level:   "warning",
				Node:    state.Name,
				Type:    "memory",
				Message: fmt.Sprintf("Memory usage high: %.1f%%", state.Memory),
				Time:    time.Now(),
			})
		}
	}

	return alerts
}

func (m *Monitor) RunLoop(stopCh <-chan struct{}) {
	ticker := time.NewTicker(m.config.CheckInterval)
	defer ticker.Stop()

	// Initial check
	m.CheckAll()

	for {
		select {
		case <-ticker.C:
			m.CheckAll()
			alerts := m.DetectAlerts()
			for _, alert := range alerts {
				fmt.Printf("[%s] %s: %s\n", alert.Level, alert.Node, alert.Message)
			}
		case <-stopCh:
			return
		}
	}
}
