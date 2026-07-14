package nodestate

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bBearer\s+[A-Za-z0-9._\-+/=]{12,}`),
	regexp.MustCompile(`(?i)\b(sk-[A-Za-z0-9_\-]{12,})`),
	regexp.MustCompile(`(?i)\b(sk-ant-[A-Za-z0-9_\-]{12,})`),
	regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9_]{20,}\b`),
	regexp.MustCompile(`\bAIza[A-Za-z0-9_\-]{20,}\b`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+\b`),
	regexp.MustCompile(`(?i)\b(password|passwd|token|secret|api[_-]?key)\s*[:=]\s*["']?[^"'\s,;]{6,}`),
}

func RedactText(value string) string {
	out := value
	for _, pattern := range secretPatterns {
		out = pattern.ReplaceAllStringFunc(out, func(match string) string {
			if strings.Contains(strings.ToLower(match), "bearer ") {
				return "Bearer [REDACTED]"
			}
			if key, _, ok := strings.Cut(match, "="); ok && containsSecretKey(key) {
				return key + "=[REDACTED]"
			}
			if key, _, ok := strings.Cut(match, ":"); ok && containsSecretKey(key) {
				return key + ":[REDACTED]"
			}
			return "[REDACTED_SECRET]"
		})
	}
	return out
}

func containsSecretKey(value string) bool {
	value = strings.ToLower(value)
	return strings.Contains(value, "password") ||
		strings.Contains(value, "passwd") ||
		strings.Contains(value, "token") ||
		strings.Contains(value, "secret") ||
		strings.Contains(value, "api_key") ||
		strings.Contains(value, "apikey")
}

func parseUptimeLoad(out string, s *SystemState) {
	if idx := strings.LastIndex(out, "load averages:"); idx >= 0 {
		parseLoadList(out[idx+len("load averages:"):], s)
		return
	}
	if idx := strings.LastIndex(out, "load average:"); idx >= 0 {
		parseLoadList(out[idx+len("load average:"):], s)
	}
}

func parseLoadList(value string, s *SystemState) {
	parts := splitTrim(strings.ReplaceAll(value, ",", " "), " ")
	if len(parts) > 0 {
		s.Load1 = atof(parts[0])
	}
	if len(parts) > 1 {
		s.Load5 = atof(parts[1])
	}
	if len(parts) > 2 {
		s.Load15 = atof(parts[2])
	}
}

func parseLinuxMem(data string, s *SystemState) {
	values := map[string]int64{}
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		values[strings.TrimSuffix(fields[0], ":")] = int64(atoi(fields[1]))
	}
	total := values["MemTotal"] / 1024
	available := values["MemAvailable"] / 1024
	if total > 0 {
		s.MemoryTotalMB = total
		s.MemoryUsedMB = total - available
		s.MemoryPct = pct(float64(s.MemoryUsedMB), float64(total))
	}
}

func parseLinuxUptime(data string, s *SystemState) {
	fields := strings.Fields(data)
	if len(fields) > 0 {
		s.UptimeSeconds = int64(atof(fields[0]))
	}
}

func parseDarwinMem(vmstat, memsize string, s *SystemState) {
	totalBytes := int64(atoi(strings.TrimSpace(memsize)))
	if totalBytes <= 0 {
		return
	}
	pageSize := int64(4096)
	var free, inactive, speculative int64
	for _, line := range strings.Split(vmstat, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "."))
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		n := int64(atoi(strings.TrimSpace(value)))
		switch key {
		case "Pages free":
			free = n
		case "Pages inactive":
			inactive = n
		case "Pages speculative":
			speculative = n
		}
	}
	totalMB := totalBytes / 1024 / 1024
	availableMB := (free + inactive + speculative) * pageSize / 1024 / 1024
	s.MemoryTotalMB = totalMB
	s.MemoryUsedMB = totalMB - availableMB
	s.MemoryPct = pct(float64(s.MemoryUsedMB), float64(totalMB))
}

func parseDarwinUptime(boottime string, s *SystemState) {
	re := regexp.MustCompile(`sec = ([0-9]+)`)
	matches := re.FindStringSubmatch(boottime)
	if len(matches) != 2 {
		return
	}
	boot := int64(atoi(matches[1]))
	if boot > 0 {
		s.UptimeSeconds = time.Now().Unix() - boot
	}
}

func parseDisk(out string, s *SystemState) {
	lines := nonEmptyLines(out)
	if len(lines) < 2 {
		return
	}
	fields := strings.Fields(lines[len(lines)-1])
	if len(fields) < 5 {
		return
	}
	totalKB := int64(atoi(fields[1]))
	usedKB := int64(atoi(fields[2]))
	if totalKB <= 0 {
		return
	}
	s.DiskTotalGB = totalKB / 1024 / 1024
	s.DiskUsedGB = usedKB / 1024 / 1024
	if len(fields) >= 5 && strings.HasSuffix(fields[4], "%") {
		s.DiskPct = atof(fields[4])
	} else {
		s.DiskPct = pct(float64(usedKB), float64(totalKB))
	}
}

func parseSwap(data string) SwapState {
	values := map[string]int64{}
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			values[strings.TrimSuffix(fields[0], ":")] = int64(atoi(fields[1]))
		}
	}
	total := values["SwapTotal"] / 1024
	free := values["SwapFree"] / 1024
	used := total - free
	return SwapState{TotalMB: total, UsedMB: used, UsedPct: pct(float64(used), float64(total))}
}

func parseMounts(out string) []MountState {
	lines := nonEmptyLines(out)
	if len(lines) < 2 {
		return nil
	}
	var mounts []MountState
	for _, line := range lines[1:] {
		fields := strings.Fields(line)
		if len(fields) < 6 {
			continue
		}
		fs := fields[0]
		mountPoint := fields[len(fields)-1]
		if skipMount(fs, mountPoint) {
			continue
		}
		mounts = append(mounts, MountState{
			Filesystem: fs,
			MountPoint: mountPoint,
			UsedPct:    atof(fields[4]),
		})
		if len(mounts) >= 40 {
			break
		}
	}
	return mounts
}

func mergeMountInodes(mounts, inodes []MountState) []MountState {
	byMount := map[string]float64{}
	for _, inode := range inodes {
		byMount[inode.MountPoint] = inode.UsedPct
	}
	for i := range mounts {
		mounts[i].InodePct = byMount[mounts[i].MountPoint]
	}
	return mounts
}

func skipMount(fs, mountPoint string) bool {
	if strings.HasPrefix(fs, "tmpfs") || strings.HasPrefix(fs, "devtmpfs") || strings.HasPrefix(fs, "overlay") {
		return true
	}
	for _, prefix := range []string{"/run", "/dev", "/sys", "/proc", "/var/lib/docker/"} {
		if strings.HasPrefix(mountPoint, prefix) {
			return true
		}
	}
	return false
}

func parseLinuxInterfaces(out string) []NetworkInterface {
	var interfaces []NetworkInterface
	for _, line := range nonEmptyLines(out) {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		item := NetworkInterface{Name: fields[0], State: fields[1]}
		for _, field := range fields[2:] {
			if strings.Contains(field, ".") && !strings.Contains(field, ":") {
				item.IPv4 = field
			} else if strings.Contains(field, ":") {
				item.IPv6 = field
			}
		}
		interfaces = append(interfaces, item)
	}
	return interfaces
}

func parseLinuxRoutes(out string) []NetworkRoute {
	var routes []NetworkRoute
	for _, line := range nonEmptyLines(out) {
		fields := strings.Fields(line)
		route := NetworkRoute{}
		for i := 0; i < len(fields); i++ {
			switch fields[i] {
			case "default":
				route.Destination = "default"
			case "via":
				if i+1 < len(fields) {
					route.Gateway = fields[i+1]
				}
			case "dev":
				if i+1 < len(fields) {
					route.Device = fields[i+1]
				}
			}
		}
		if route.Destination != "" || route.Gateway != "" || route.Device != "" {
			routes = append(routes, route)
		}
	}
	return routes
}

func parseSSListeners(out string) []NetworkListener {
	var listeners []NetworkListener
	for _, line := range nonEmptyLines(out) {
		fields := strings.Fields(line)
		if len(fields) < 5 || strings.EqualFold(fields[0], "Netid") {
			continue
		}
		local := fields[4]
		proc := ""
		if len(fields) > 6 {
			proc = strings.Join(fields[6:], " ")
		}
		listeners = append(listeners, listenerFromAddress(fields[0], local, proc))
	}
	return listeners
}

func parseNetstatListeners(out string) []NetworkListener {
	var listeners []NetworkListener
	for _, line := range nonEmptyLines(out) {
		fields := strings.Fields(line)
		if len(fields) < 4 || strings.HasPrefix(strings.ToLower(fields[0]), "proto") {
			continue
		}
		proto := fields[0]
		local := fields[3]
		proc := ""
		if len(fields) > 6 {
			proc = fields[6]
		}
		listeners = append(listeners, listenerFromAddress(proto, local, proc))
	}
	return listeners
}

func parseLsofListeners(out string) []NetworkListener {
	var listeners []NetworkListener
	for _, line := range nonEmptyLines(out) {
		fields := strings.Fields(line)
		if len(fields) < 9 || fields[0] == "COMMAND" {
			continue
		}
		address := fields[8]
		listeners = append(listeners, listenerFromAddress("tcp", address, fields[0]))
	}
	return listeners
}

func listenerFromAddress(proto, local, proc string) NetworkListener {
	address, port := splitAddressPort(local)
	return NetworkListener{
		Protocol: proto,
		Address:  address,
		Port:     port,
		Process:  truncateLine(RedactText(proc), 160),
		Public:   isPublicListenAddress(address),
	}
}

func splitAddressPort(value string) (string, string) {
	value = strings.TrimSpace(strings.Trim(value, "[]"))
	if strings.Contains(value, "->") {
		value = strings.Split(value, "->")[0]
	}
	idx := strings.LastIndex(value, ":")
	if idx < 0 {
		return value, ""
	}
	return strings.Trim(value[:idx], "[]"), strings.Trim(value[idx+1:], "[]")
}

func isPublicListenAddress(address string) bool {
	address = strings.Trim(address, "[]")
	return address == "*" || address == "0.0.0.0" || address == "::" || address == ":::" || address == ""
}

func parseResolvConf(data string) []string {
	var dns []string
	for _, line := range nonEmptyLines(data) {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "nameserver" {
			dns = append(dns, fields[1])
		}
	}
	return dns
}

func parseDarwinInterfaces(out string) []NetworkInterface {
	var interfaces []NetworkInterface
	var current *NetworkInterface
	for _, line := range strings.Split(out, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !strings.HasPrefix(line, "\t") && strings.Contains(line, ":") {
			name := strings.TrimSuffix(strings.Fields(line)[0], ":")
			interfaces = append(interfaces, NetworkInterface{Name: name})
			current = &interfaces[len(interfaces)-1]
			if strings.Contains(line, "UP") {
				current.State = "UP"
			}
			continue
		}
		if current == nil {
			continue
		}
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) >= 2 && fields[0] == "inet" {
			current.IPv4 = fields[1]
		}
		if len(fields) >= 2 && fields[0] == "inet6" && current.IPv6 == "" {
			current.IPv6 = fields[1]
		}
	}
	return interfaces
}

func parseDarwinRoutes(out string) []NetworkRoute {
	route := NetworkRoute{Destination: "default"}
	for _, line := range strings.Split(out, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(key) {
		case "gateway":
			route.Gateway = strings.TrimSpace(value)
		case "interface":
			route.Device = strings.TrimSpace(value)
		}
	}
	if route.Gateway == "" && route.Device == "" {
		return nil
	}
	return []NetworkRoute{route}
}

func parseDarwinDNS(out string) []string {
	var dns []string
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "nameserver[") {
			_, value, ok := strings.Cut(line, ":")
			if ok {
				dns = append(dns, strings.TrimSpace(value))
			}
		}
	}
	return dns
}

func parseFail2BanJails(out string) []string {
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(strings.ToLower(line), "jail list") {
			continue
		}
		_, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		parts := splitTrim(value, ",")
		return parts
	}
	return nil
}

func listPlists(dir string, limit int) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".plist" {
			continue
		}
		out = append(out, entry.Name())
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}

func firstLine(value string) string {
	lines := nonEmptyLines(value)
	if len(lines) == 0 {
		return ""
	}
	return truncateLine(lines[0], 180)
}

func readFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}

func splitTrim(value, sep string) []string {
	raw := strings.Split(value, sep)
	out := make([]string, 0, len(raw))
	for _, part := range raw {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func nonEmptyLines(value string) []string {
	var out []string
	for _, line := range strings.Split(value, "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func truncateLine(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit-1] + "..."
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func fingerprintString(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])[:16]
}

func atoi(value string) int {
	n, _ := strconv.Atoi(strings.Trim(strings.TrimSpace(value), "%"))
	return n
}

func atof(value string) float64 {
	n, _ := strconv.ParseFloat(strings.Trim(strings.TrimSpace(value), "%"), 64)
	return n
}

func pct(used, total float64) float64 {
	if total <= 0 {
		return 0
	}
	return used / total * 100
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
