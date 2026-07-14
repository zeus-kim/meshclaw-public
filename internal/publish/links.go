package publish

import (
	"crypto/sha256"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const publicPort = "48303"

func DocumentPreviewImage(path string) string {
	preview := strings.TrimSuffix(path, filepath.Ext(path)) + ".html"
	if !fileNonEmpty(preview) {
		return ""
	}
	image := preview + ".png"
	if fileNonEmpty(image) {
		return image
	}
	return ""
}

func DocumentLink(path string) string {
	links := DocumentLinks(path)
	if len(links) == 0 {
		return ""
	}
	return links[0]
}

func DocumentLinks(path string) []string {
	if !fileNonEmpty(path) {
		return nil
	}
	published, ok := PublishPublicFile(path)
	if !ok {
		return nil
	}
	if !publicServerDisabled() {
		EnsurePublicServer()
	}
	return LinksForPublished(published)
}

func LinksForPublished(published string) []string {
	if strings.TrimSpace(published) == "" {
		return nil
	}
	name := url.PathEscape(filepath.Base(published))
	seen := map[string]bool{}
	links := []string{}
	add := func(link string) {
		link = strings.TrimSpace(link)
		if link == "" || seen[link] {
			return
		}
		seen[link] = true
		links = append(links, link)
	}
	for _, baseURL := range PublicBaseURLs() {
		add(strings.TrimRight(baseURL, "/") + "/" + name)
	}
	for _, host := range PublicHosts() {
		add("http://" + host + ":" + publicPort + "/argos/" + name)
	}
	return links
}

func PublishPublicFile(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return WritePublicFile(TokenizedPublicName(path, data), data)
}

func WritePublicFile(name string, data []byte) (string, bool) {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "." || name == "" || len(data) == 0 {
		return "", false
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", false
	}
	dir := filepath.Join(home, ".meshclaw", "public", "argos")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", false
	}
	preparePublicDirectory(filepath.Join(home, ".meshclaw", "public"), dir)
	cleanupPublicDirectory(dir)
	dst := filepath.Join(dir, name)
	if err := os.WriteFile(dst, data, 0600); err != nil {
		return "", false
	}
	return dst, true
}

func FirstLinkForPublished(path string) string {
	links := LinksForPublished(path)
	if len(links) == 0 {
		return ""
	}
	return links[0]
}

func EnsurePublicServer() {
	if exec.Command("lsof", "-nP", "-iTCP:"+publicPort, "-sTCP:LISTEN").Run() == nil {
		return
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return
	}
	public := filepath.Join(home, ".meshclaw", "public")
	logDir := filepath.Join(home, ".meshclaw", "logs")
	_ = os.MkdirAll(public, 0700)
	_ = os.MkdirAll(logDir, 0700)
	cmd := exec.Command("/usr/bin/python3", "-m", "http.server", publicPort, "--bind", "0.0.0.0", "--directory", public)
	if logFile, err := os.OpenFile(filepath.Join(logDir, "argos-public-server.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600); err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}
	_ = cmd.Start()
}

func PublicHosts() []string {
	seen := map[string]bool{}
	hosts := []string{}
	add := func(host string) {
		host = strings.TrimSpace(host)
		if host == "" || seen[host] {
			return
		}
		seen[host] = true
		hosts = append(hosts, host)
	}
	if host := strings.TrimSpace(os.Getenv("MESHCLAW_ARGOS_PUBLIC_HOST")); host != "" {
		add(host)
	}
	for _, host := range localPublicIPs() {
		add(host)
	}
	if out, err := exec.Command("hostname").Output(); err == nil {
		if host := strings.TrimSpace(string(out)); host != "" {
			add(host)
		}
	}
	add("localhost")
	return hosts
}

func PublicBaseURLs() []string {
	seen := map[string]bool{}
	baseURLs := []string{}
	add := func(baseURL string) {
		baseURL = normalizePublicBaseURL(baseURL)
		if baseURL == "" || seen[baseURL] {
			return
		}
		seen[baseURL] = true
		baseURLs = append(baseURLs, baseURL)
	}
	for _, key := range []string{"MESHCLAW_ARGOS_PUBLIC_BASE_URL", "MESHCLAW_ARGOS_PUBLIC_BASE_URLS"} {
		for _, value := range strings.Split(os.Getenv(key), ",") {
			add(value)
		}
	}
	if dashboardBase := publicBaseURLFromDashboard(os.Getenv("MESHCLAW_ARGOS_DASHBOARD_URL")); dashboardBase != "" {
		add(dashboardBase)
	}
	if host := strings.TrimSpace(os.Getenv("MESHCLAW_ARGOS_PUBLIC_HOST")); host == "argos.zeus.kim" || host == "arogos.zeus.kim" {
		add("https://" + host + "/argos")
	}
	return baseURLs
}

func normalizePublicBaseURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	if parsed.Path == "" {
		parsed.Path = "/argos"
	}
	return strings.TrimRight(parsed.String(), "/")
}

func publicBaseURLFromDashboard(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return ""
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	trimmedPath := strings.TrimRight(parsed.Path, "/")
	switch {
	case strings.HasSuffix(strings.ToLower(trimmedPath), "/dashboard.html"):
		trimmedPath = strings.TrimSuffix(trimmedPath, "/dashboard.html")
	case pathpkg.Ext(trimmedPath) != "":
		trimmedPath = pathpkg.Dir(trimmedPath)
	}
	if trimmedPath == "" || trimmedPath == "." {
		trimmedPath = "/argos"
	}
	parsed.Path = trimmedPath
	return strings.TrimRight(parsed.String(), "/")
}

func TokenizedPublicName(path string, data []byte) string {
	sum := sha256.Sum256(append([]byte(path+"\x00"), data...))
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	base = SanitizePublicBase(base)
	if base == "" {
		base = "argos-report"
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		ext = ".dat"
	}
	return fmt.Sprintf("%s-%x%s", base, sum[:8], ext)
}

func SanitizePublicBase(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	lastDash := false
	for _, r := range value {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 64 {
		out = strings.Trim(out[:64], "-")
	}
	return out
}

func preparePublicDirectory(root, dir string) {
	index := []byte("<!doctype html><meta charset=\"utf-8\"><title>Argos</title><body>Argos document links are private per report.</body>")
	_ = os.WriteFile(filepath.Join(root, "index.html"), index, 0600)
	_ = os.WriteFile(filepath.Join(dir, "index.html"), index, 0600)
}

func cleanupPublicDirectory(dir string) {
	ttl := publicTTL()
	if ttl <= 0 {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-ttl)
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "index.html" {
			continue
		}
		info, err := entry.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		_ = os.Remove(filepath.Join(dir, entry.Name()))
	}
}

func publicTTL() time.Duration {
	value := strings.TrimSpace(os.Getenv("MESHCLAW_ARGOS_PUBLIC_TTL"))
	if value == "" {
		return 7 * 24 * time.Hour
	}
	if d, err := time.ParseDuration(value); err == nil {
		return d
	}
	if hours, err := strconv.ParseFloat(value, 64); err == nil {
		return time.Duration(hours * float64(time.Hour))
	}
	return 7 * 24 * time.Hour
}

func publicServerDisabled() bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv("MESHCLAW_ARGOS_PUBLIC_SERVER_DISABLED")))
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func localPublicIPs() []string {
	out, err := exec.Command("ifconfig").Output()
	if err != nil {
		return nil
	}
	tailscale := []string{}
	lan := []string{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 || fields[0] != "inet" {
			continue
		}
		ip := fields[1]
		if ip == "127.0.0.1" || strings.HasPrefix(ip, "169.254.") {
			continue
		}
		if strings.HasPrefix(ip, "100.") {
			tailscale = append(tailscale, ip)
			continue
		}
		lan = append(lan, ip)
	}
	return append(tailscale, lan...)
}

func fileNonEmpty(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir() && st.Size() > 0
}
