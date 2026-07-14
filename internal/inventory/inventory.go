package inventory

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const storeVersion = 1

type Node struct {
	Name      string   `json:"name"`
	Role      string   `json:"role,omitempty"`
	Location  string   `json:"location,omitempty"`
	WireIP    string   `json:"wire_ip,omitempty"`
	Tailscale string   `json:"tailscale,omitempty"`
	LAN       string   `json:"lan,omitempty"`
	User      string   `json:"user,omitempty"`
	Tags      []string `json:"tags,omitempty"`
	OS        string   `json:"os,omitempty"`
	Online    bool     `json:"online,omitempty"`
	Source    string   `json:"source,omitempty"`
}

type Store struct {
	Version int    `json:"version"`
	Nodes   []Node `json:"nodes"`
}

type Diff struct {
	InventoryPath string `json:"inventory_path"`
	Current       []Node `json:"current"`
	Discovered    []Node `json:"discovered"`
	Missing       []Node `json:"missing"`
	Changed       []Node `json:"changed"`
	Unchanged     []Node `json:"unchanged"`
}

var managedName = regexp.MustCompile(`^(c[0-9]+|d[0-9]+|g[0-9]+|s[0-9]+|v[0-9]+|macmini|m1)$`)

func DefaultNodes() []Node {
	nodes, err := Load()
	if err == nil && len(nodes) > 0 {
		return ApplyOverrides(nodes)
	}
	discovered, err := Discover()
	if err == nil && len(discovered) > 0 {
		return ApplyOverrides(discovered)
	}
	legacy, err := loadLegacyNodes()
	if err == nil && len(legacy) > 0 {
		return ApplyOverrides(legacy)
	}
	return nil
}

func Find(name string) (Node, bool) {
	for _, node := range DefaultNodes() {
		if node.Name == name {
			return node, true
		}
	}
	return Node{}, false
}

func Path() string {
	if path := strings.TrimSpace(os.Getenv("MESHCLAW_INVENTORY_FILE")); path != "" {
		return path
	}
	return filepath.Join(configDir(), "inventory.json")
}

func OverridesPath() string {
	if path := strings.TrimSpace(os.Getenv("MESHCLAW_INVENTORY_OVERRIDES_FILE")); path != "" {
		return path
	}
	return filepath.Join(configDir(), "inventory_overrides.json")
}

func Load() ([]Node, error) {
	data, err := os.ReadFile(Path())
	if err != nil {
		return nil, err
	}
	var store Store
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, err
	}
	nodes := normalizeNodes(store.Nodes)
	if len(nodes) == 0 {
		return nil, errors.New("inventory has no nodes")
	}
	return ApplyOverrides(nodes), nil
}

func LoadOverrides() ([]Node, error) {
	data, err := os.ReadFile(OverridesPath())
	if err != nil {
		return nil, err
	}
	var store Store
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, err
	}
	return normalizeNodes(store.Nodes), nil
}

func SaveOverrides(nodes []Node) error {
	nodes = normalizeNodes(nodes)
	if err := os.MkdirAll(filepath.Dir(OverridesPath()), 0700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(Store{Version: storeVersion, Nodes: nodes}, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(OverridesPath(), append(payload, '\n'), 0600)
}

func SetOverride(node Node) ([]Node, error) {
	node = normalizeNode(node)
	if node.Name == "" {
		return nil, errors.New("node name is required")
	}
	existing, err := LoadOverrides()
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	nodes := Merge(existing, []Node{node})
	if err := SaveOverrides(nodes); err != nil {
		return nil, err
	}
	return nodes, nil
}

func RemoveOverride(name string) ([]Node, bool, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return nil, false, errors.New("node name is required")
	}
	existing, err := LoadOverrides()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}
	out := []Node{}
	removed := false
	for _, node := range existing {
		if node.Name == name {
			removed = true
			continue
		}
		out = append(out, node)
	}
	if err := SaveOverrides(out); err != nil {
		return nil, removed, err
	}
	return out, removed, nil
}

func Save(nodes []Node) error {
	nodes = normalizeNodes(nodes)
	if err := os.MkdirAll(filepath.Dir(Path()), 0700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(Store{Version: storeVersion, Nodes: nodes}, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	return os.WriteFile(Path(), payload, 0600)
}

func InitFromDiscovery(force bool) ([]Node, error) {
	if !force {
		if nodes, err := Load(); err == nil && len(nodes) > 0 {
			return nodes, nil
		}
	}
	discovered, err := Discover()
	if err != nil {
		return nil, err
	}
	if len(discovered) == 0 {
		return nil, errors.New("no managed nodes discovered")
	}
	if existing, err := Load(); err == nil {
		discovered = Merge(existing, discovered)
	}
	if err := Save(discovered); err != nil {
		return nil, err
	}
	return discovered, nil
}

func Discover() ([]Node, error) {
	base := []Node{}
	if legacy, err := loadLegacyNodes(); err == nil {
		base = Merge(base, legacy)
	}
	if users := loadWireUsers(); len(users) > 0 {
		base = Merge(base, nodesFromUsers(users))
	}
	if peers, err := discoverVSSHPeers(); err == nil {
		base = Merge(base, peers)
	}
	nodes, err := discoverTailscale()
	if err != nil {
		if len(base) > 0 {
			return base, nil
		}
		return nil, err
	}
	nodes = Merge(base, nodes)
	return ApplyOverrides(normalizeNodes(nodes)), nil
}

func ApplyOverrides(nodes []Node) []Node {
	overrides, err := LoadOverrides()
	if err != nil || len(overrides) == 0 {
		return normalizeNodes(nodes)
	}
	return Merge(nodes, overrides)
}

func ComputeDiff() (Diff, error) {
	current, _ := Load()
	discovered, err := Discover()
	if err != nil {
		return Diff{}, err
	}
	currentByName := byName(current)
	diff := Diff{InventoryPath: Path(), Current: current, Discovered: discovered}
	for _, node := range discovered {
		existing, ok := currentByName[node.Name]
		if !ok {
			diff.Missing = append(diff.Missing, node)
			continue
		}
		if nodeChanged(existing, node) {
			diff.Changed = append(diff.Changed, mergeNode(existing, node))
		} else {
			diff.Unchanged = append(diff.Unchanged, existing)
		}
	}
	return diff, nil
}

func Merge(base []Node, incoming []Node) []Node {
	merged := byName(base)
	for _, node := range incoming {
		node = normalizeNode(node)
		if node.Name == "" {
			continue
		}
		if existing, ok := merged[node.Name]; ok {
			merged[node.Name] = mergeNode(existing, node)
		} else {
			merged[node.Name] = node
		}
	}
	out := make([]Node, 0, len(merged))
	for _, node := range merged {
		out = append(out, normalizeNode(node))
	}
	sortNodes(out)
	return out
}

func configDir() string {
	if dir := strings.TrimSpace(os.Getenv("MESHCLAW_CONFIG_DIR")); dir != "" {
		return dir
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".meshclaw")
	}
	return ".meshclaw"
}

type legacyConfig struct {
	Nodes       map[string]legacyNode        `json:"nodes"`
	StaticNodes map[string]map[string]string `json:"static_nodes"`
	SSHUsers    map[string]string            `json:"ssh_users"`
}

type legacyNode struct {
	IP   string `json:"ip"`
	User string `json:"user"`
}

func loadLegacyNodes() ([]Node, error) {
	paths := []string{
		filepath.Join(configDir(), "nodes.json"),
	}
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths, filepath.Join(home, ".argos", "nodes.json"))
	}
	var lastErr error
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			lastErr = err
			continue
		}
		var cfg legacyConfig
		if err := json.Unmarshal(data, &cfg); err != nil {
			lastErr = err
			continue
		}
		nodes := make([]Node, 0, len(cfg.Nodes)+len(cfg.StaticNodes))
		for name, item := range cfg.Nodes {
			node := inferNode(name, "", item.IP)
			node.Tailscale = item.IP
			node.User = item.User
			node.Source = "legacy-nodes-json"
			nodes = append(nodes, node)
		}
		for name, item := range cfg.StaticNodes {
			node := inferNode(name, "", item["ip"])
			node.Tailscale = item["ip"]
			node.User = item["user"]
			node.Source = "legacy-static-nodes"
			nodes = append(nodes, node)
		}
		for name, user := range cfg.SSHUsers {
			node := inferNode(name, "", "")
			node.User = user
			node.Source = "legacy-ssh-users"
			nodes = append(nodes, node)
		}
		return normalizeNodes(nodes), nil
	}
	if lastErr == nil {
		lastErr = os.ErrNotExist
	}
	return nil, lastErr
}

func loadWireUsers() map[string]string {
	if home, err := os.UserHomeDir(); err == nil {
		path := filepath.Join(home, ".wire", "users.json")
		data, err := os.ReadFile(path)
		if err == nil {
			users := map[string]string{}
			if json.Unmarshal(data, &users) == nil {
				return users
			}
		}
	}
	return nil
}

func nodesFromUsers(users map[string]string) []Node {
	nodes := make([]Node, 0, len(users))
	for name, user := range users {
		if !managedName.MatchString(strings.ToLower(name)) {
			continue
		}
		node := inferNode(name, "", "")
		node.User = user
		node.Source = "wire-users"
		nodes = append(nodes, node)
	}
	return normalizeNodes(nodes)
}

func discoverVSSHPeers() ([]Node, error) {
	binary := strings.TrimSpace(os.Getenv("MESHCLAW_VSSH_BINARY"))
	if binary == "" {
		if home, err := os.UserHomeDir(); err == nil {
			candidate := filepath.Join(home, "bin", "vssh")
			if st, err := os.Stat(candidate); err == nil && !st.IsDir() && st.Mode()&0111 != 0 {
				binary = candidate
			}
		}
	}
	if binary == "" {
		binary = "vssh"
	}
	out, err := exec.Command(binary, "list").Output()
	if err != nil {
		return nil, err
	}
	nodes := []Node{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		name := strings.ToLower(fields[0])
		if !managedName.MatchString(name) || net.ParseIP(fields[1]) == nil {
			continue
		}
		node := inferNode(name, "", "")
		node.WireIP = fields[1]
		node.Source = "vssh-list"
		nodes = append(nodes, node)
	}
	return normalizeNodes(nodes), nil
}

type tailscaleStatus struct {
	Self tailscalePeer            `json:"Self"`
	Peer map[string]tailscalePeer `json:"Peer"`
}

type tailscalePeer struct {
	HostName     string   `json:"HostName"`
	DNSName      string   `json:"DNSName"`
	OS           string   `json:"OS"`
	TailscaleIPs []string `json:"TailscaleIPs"`
	Addrs        []string `json:"Addrs"`
	CurAddr      string   `json:"CurAddr"`
	Online       bool     `json:"Online"`
}

func discoverTailscale() ([]Node, error) {
	out, err := exec.Command("tailscale", "status", "--json").Output()
	if err != nil {
		return nil, fmt.Errorf("tailscale status --json failed: %w", err)
	}
	var status tailscaleStatus
	if err := json.Unmarshal(out, &status); err != nil {
		return nil, err
	}
	nodes := []Node{}
	addPeer := func(peer tailscalePeer) {
		node := nodeFromPeer(peer)
		if node.Name == "" || !managedName.MatchString(node.Name) {
			return
		}
		nodes = append(nodes, node)
	}
	addPeer(status.Self)
	for _, peer := range status.Peer {
		addPeer(peer)
	}
	return normalizeNodes(nodes), nil
}

func nodeFromPeer(peer tailscalePeer) Node {
	name := canonicalName(peer)
	node := inferNode(name, peer.OS, firstIPv4(peer.TailscaleIPs))
	node.Tailscale = firstIPv4(peer.TailscaleIPs)
	node.LAN = firstLAN(peer)
	node.OS = peer.OS
	node.Online = peer.Online
	node.Source = "tailscale"
	return node
}

func canonicalName(peer tailscalePeer) string {
	if peer.DNSName != "" {
		name := strings.TrimSuffix(peer.DNSName, ".")
		if i := strings.IndexByte(name, '.'); i > 0 {
			return strings.ToLower(name[:i])
		}
	}
	name := strings.TrimSpace(peer.HostName)
	name = strings.ReplaceAll(name, " ", "-")
	return strings.ToLower(name)
}

func inferNode(name string, osName string, ip string) Node {
	node := Node{Name: strings.ToLower(strings.TrimSpace(name)), Tailscale: ip}
	osLower := strings.ToLower(osName)
	switch {
	case strings.HasPrefix(node.Name, "v"):
		node.Role = "relay"
		node.Location = "vps"
		node.User = "root"
		node.Tags = []string{"linux", "vps"}
	case strings.HasPrefix(node.Name, "c"):
		node.Role = "server"
		node.Location = "vps"
		node.User = "root"
		node.Tags = []string{"linux", "vps"}
	case strings.HasPrefix(node.Name, "d"):
		node.Role = "ai-workload"
		node.Tags = []string{"linux", "gpu"}
	case strings.HasPrefix(node.Name, "g"):
		node.Role = "docker-host"
		node.Tags = []string{"linux", "gpu"}
	case strings.HasPrefix(node.Name, "s"):
		node.Role = "nas"
		node.Tags = []string{"linux", "storage"}
	case strings.Contains(osLower, "mac"):
		node.Role = "desktop-worker"
		node.Tags = []string{"macos", "client"}
	default:
		if osLower != "" {
			node.Tags = []string{osLower}
		}
	}
	return normalizeNode(node)
}

func firstIPv4(values []string) string {
	for _, value := range values {
		if ip := net.ParseIP(value); ip != nil && ip.To4() != nil {
			return value
		}
	}
	return ""
}

func firstLAN(peer tailscalePeer) string {
	values := append([]string{}, peer.Addrs...)
	if peer.CurAddr != "" {
		values = append(values, peer.CurAddr)
	}
	for _, value := range values {
		host, _, err := net.SplitHostPort(value)
		if err != nil {
			host = value
		}
		ip := net.ParseIP(host)
		if ip == nil || ip.To4() == nil {
			continue
		}
		if strings.HasPrefix(host, "192.168.") || strings.HasPrefix(host, "10.") || strings.HasPrefix(host, "172.") {
			return host
		}
	}
	return ""
}

func byName(nodes []Node) map[string]Node {
	out := make(map[string]Node, len(nodes))
	for _, node := range nodes {
		node = normalizeNode(node)
		if node.Name != "" {
			out[node.Name] = node
		}
	}
	return out
}

func mergeNode(existing Node, incoming Node) Node {
	out := existing
	if incoming.Role != "" {
		out.Role = incoming.Role
	}
	if incoming.Location != "" {
		out.Location = incoming.Location
	}
	if incoming.WireIP != "" {
		out.WireIP = incoming.WireIP
	}
	if incoming.Tailscale != "" {
		out.Tailscale = incoming.Tailscale
	}
	if incoming.LAN != "" {
		out.LAN = incoming.LAN
	}
	if incoming.User != "" {
		out.User = incoming.User
	}
	if len(incoming.Tags) > 0 {
		out.Tags = mergeStrings(out.Tags, incoming.Tags)
	}
	if incoming.OS != "" {
		out.OS = incoming.OS
	}
	if incoming.Online {
		out.Online = true
	}
	if incoming.Source != "" {
		if out.Source == "" {
			out.Source = incoming.Source
		} else if !strings.Contains(out.Source, incoming.Source) {
			out.Source += "," + incoming.Source
		}
	}
	return normalizeNode(out)
}

func nodeChanged(existing Node, incoming Node) bool {
	merged := mergeNode(existing, incoming)
	return merged.Tailscale != existing.Tailscale ||
		merged.LAN != existing.LAN ||
		merged.Role != existing.Role ||
		merged.Location != existing.Location ||
		strings.Join(merged.Tags, ",") != strings.Join(existing.Tags, ",") ||
		merged.OS != existing.OS
}

func normalizeNodes(nodes []Node) []Node {
	out := make([]Node, 0, len(nodes))
	seen := map[string]Node{}
	for _, node := range nodes {
		node = normalizeNode(node)
		if node.Name == "" {
			continue
		}
		if existing, ok := seen[node.Name]; ok {
			seen[node.Name] = mergeNode(existing, node)
		} else {
			seen[node.Name] = node
		}
	}
	for _, node := range seen {
		out = append(out, node)
	}
	sortNodes(out)
	return out
}

func normalizeNode(node Node) Node {
	node.Name = strings.ToLower(strings.TrimSpace(node.Name))
	node.Role = strings.TrimSpace(node.Role)
	node.Location = strings.TrimSpace(node.Location)
	node.WireIP = strings.TrimSpace(node.WireIP)
	node.Tailscale = strings.TrimSpace(node.Tailscale)
	node.LAN = strings.TrimSpace(node.LAN)
	node.User = strings.TrimSpace(node.User)
	node.OS = strings.TrimSpace(node.OS)
	node.Source = strings.TrimSpace(node.Source)
	node.Tags = mergeStrings(node.Tags, nil)
	return node
}

func mergeStrings(a []string, b []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, item := range append(append([]string{}, a...), b...) {
		item = strings.ToLower(strings.TrimSpace(item))
		if item == "" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	sort.Strings(out)
	return out
}

func sortNodes(nodes []Node) {
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].Name < nodes[j].Name
	})
}
