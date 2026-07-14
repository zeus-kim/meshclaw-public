package nodestate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type StoredReport struct {
	Kind        string    `json:"kind"`
	StoredAt    time.Time `json:"stored_at"`
	Report      Report    `json:"report"`
	StatePath   string    `json:"state_path"`
	HistoryPath string    `json:"history_path,omitempty"`
}

type StoreOptions struct {
	AppendHistory bool
	MaxHistory    int
}

func Store(report Report) (StoredReport, error) {
	return StoreWithOptions(report, StoreOptions{})
}

func StoreWithOptions(report Report, opts StoreOptions) (StoredReport, error) {
	path, err := reportPath(report.NodeName)
	if err != nil {
		return StoredReport{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return StoredReport{}, err
	}
	stored := StoredReport{
		Kind:      "meshclaw_stored_node_report",
		StoredAt:  time.Now().UTC(),
		Report:    report,
		StatePath: path,
	}
	if opts.AppendHistory {
		historyPath, err := appendHistory(stored, opts.MaxHistory)
		if err != nil {
			return StoredReport{}, err
		}
		stored.HistoryPath = historyPath
	}
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return StoredReport{}, err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o600); err != nil {
		return StoredReport{}, err
	}
	latest, err := reportPath("latest")
	if err == nil {
		_ = os.WriteFile(latest, append(data, '\n'), 0o600)
	}
	return stored, nil
}

func appendHistory(stored StoredReport, maxHistory int) (string, error) {
	path, err := historyPath(stored.Report.NodeName)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	data, err := json.Marshal(stored)
	if err != nil {
		return "", err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return "", err
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		_ = f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if maxHistory > 0 {
		_ = trimHistory(path, maxHistory)
	}
	return path, nil
}

func TailHistory(node string, limit int) ([]StoredReport, error) {
	path, err := historyPath(node)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	lines := nonEmptyLines(string(data))
	if limit > 0 && len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	out := make([]StoredReport, 0, len(lines))
	for _, line := range lines {
		var stored StoredReport
		if json.Unmarshal([]byte(line), &stored) == nil && stored.Report.Kind == ReportKind {
			out = append(out, stored)
		}
	}
	return out, nil
}

func trimHistory(path string, maxLines int) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := nonEmptyLines(string(data))
	if len(lines) <= maxLines {
		return nil
	}
	lines = lines[len(lines)-maxLines:]
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600)
}

func List(maxAge time.Duration) ([]StoredReport, error) {
	dir, err := reportDir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	now := time.Now()
	var reports []StoredReport
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" || entry.Name() == "latest.json" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		var stored StoredReport
		if json.Unmarshal(data, &stored) != nil || stored.Report.Kind != ReportKind {
			continue
		}
		if maxAge > 0 && !stored.StoredAt.IsZero() && now.Sub(stored.StoredAt) > maxAge {
			continue
		}
		reports = append(reports, stored)
	}
	return reports, nil
}

func reportDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("home directory not found")
	}
	return filepath.Join(home, ".meshclaw", "state", "nodes"), nil
}

func reportPath(node string) (string, error) {
	dir, err := reportDir()
	if err != nil {
		return "", err
	}
	node = sanitizeFilename(firstNonEmpty(node, "unknown"))
	return filepath.Join(dir, node+".json"), nil
}

func historyPath(node string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return "", fmt.Errorf("home directory not found")
	}
	node = sanitizeFilename(firstNonEmpty(node, "unknown"))
	return filepath.Join(home, ".meshclaw", "state", "history", node+".jsonl"), nil
}

func sanitizeFilename(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
