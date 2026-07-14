package evidence

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/meshclaw/meshclaw/internal/opsdb"
)

type Record struct {
	ID       string      `json:"id"`
	Time     time.Time   `json:"time"`
	Kind     string      `json:"kind"`
	Host     string      `json:"host,omitempty"`
	Summary  string      `json:"summary"`
	Payload  interface{} `json:"payload"`
	StoredAt string      `json:"stored_at"`
}

type Summary struct {
	ID       string    `json:"id"`
	Time     time.Time `json:"time"`
	Kind     string    `json:"kind"`
	Host     string    `json:"host,omitempty"`
	Summary  string    `json:"summary"`
	StoredAt string    `json:"stored_at"`
}

func Store(kind, host, summary string, payload interface{}) (Record, error) {
	now := time.Now().UTC()
	id := fmt.Sprintf("%s-%09d-%s-%s", now.Format("20060102T150405Z"), now.Nanosecond(), sanitize(kind), sanitize(host))
	dir, err := evidenceDir(now)
	if err != nil {
		return Record{}, err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return Record{}, err
	}
	path := filepath.Join(dir, id+".json")
	record := Record{
		ID:       id,
		Time:     now,
		Kind:     kind,
		Host:     host,
		Summary:  summary,
		Payload:  payload,
		StoredAt: path,
	}
	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return Record{}, err
	}
	if err := os.WriteFile(path, append(data, '\n'), 0600); err != nil {
		return Record{}, err
	}
	_ = opsdb.Default().AppendEvidenceIndex(opsdb.EvidenceIndexEntry{
		Time:     record.Time,
		ID:       record.ID,
		Kind:     record.Kind,
		Node:     record.Host,
		Summary:  record.Summary,
		StoredAt: record.StoredAt,
	})
	return record, nil
}

func List(limit int) ([]Summary, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	root := filepath.Join(home, ".meshclaw", "evidence")
	var paths []string
	if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d == nil || d.IsDir() || filepath.Ext(path) != ".json" {
			return nil
		}
		paths = append(paths, path)
		return nil
	}); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	sortStringsDesc(paths)
	if limit <= 0 || limit > len(paths) {
		limit = len(paths)
	}
	summaries := make([]Summary, 0, limit)
	for _, path := range paths[:limit] {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var record Record
		if json.Unmarshal(data, &record) != nil {
			continue
		}
		summaries = append(summaries, Summary{
			ID:       record.ID,
			Time:     record.Time,
			Kind:     record.Kind,
			Host:     record.Host,
			Summary:  record.Summary,
			StoredAt: record.StoredAt,
		})
	}
	return summaries, nil
}

func Load(path string) (Record, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return Record{}, fmt.Errorf("evidence path is required")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Record{}, err
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return Record{}, err
	}
	return record, nil
}

func sortStringsDesc(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] > values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func evidenceDir(now time.Time) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".meshclaw", "evidence", now.Format("2006-01-02")), nil
}

func sanitize(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return "none"
	}
	var b strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
