package opsdb

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// opsdb is MeshClaw's DevOps state store boundary. It is intentionally separate
// from meshdb, which can contain broad personal and workspace memory.
type DB struct {
	Root string `json:"root"`
}

type Paths struct {
	Root          string `json:"root"`
	NodesDir      string `json:"nodes_dir"`
	HistoryDir    string `json:"history_dir"`
	DesiredDir    string `json:"desired_dir"`
	DriftDir      string `json:"drift_dir"`
	ApprovalsDir  string `json:"approvals_dir"`
	EvidenceIndex string `json:"evidence_index"`
}

type Event struct {
	Time       time.Time              `json:"time"`
	Kind       string                 `json:"kind"`
	Node       string                 `json:"node,omitempty"`
	Severity   string                 `json:"severity,omitempty"`
	Summary    string                 `json:"summary"`
	Source     string                 `json:"source,omitempty"`
	EvidenceID string                 `json:"evidence_id,omitempty"`
	Tags       []string               `json:"tags,omitempty"`
	Data       map[string]interface{} `json:"data,omitempty"`
}

type EvidenceIndexEntry struct {
	Time     time.Time `json:"time"`
	ID       string    `json:"id"`
	Kind     string    `json:"kind"`
	Node     string    `json:"node,omitempty"`
	Summary  string    `json:"summary"`
	StoredAt string    `json:"stored_at"`
}

type RecentOptions struct {
	Node  string
	Kind  string
	Limit int
}

type RecentEvents struct {
	Events   []Event              `json:"events"`
	Evidence []EvidenceIndexEntry `json:"evidence"`
}

func Default() DB {
	return DB{Root: defaultRoot()}
}

func Open(root string) DB {
	root = strings.TrimSpace(root)
	if root == "" {
		return Default()
	}
	return DB{Root: root}
}

func (db DB) Paths() Paths {
	root := strings.TrimSpace(db.Root)
	if root == "" {
		root = defaultRoot()
	}
	return Paths{
		Root:          root,
		NodesDir:      filepath.Join(root, "nodes"),
		HistoryDir:    filepath.Join(root, "history"),
		DesiredDir:    filepath.Join(root, "desired"),
		DriftDir:      filepath.Join(root, "drift"),
		ApprovalsDir:  filepath.Join(root, "approvals"),
		EvidenceIndex: filepath.Join(root, "evidence-index.jsonl"),
	}
}

func (db DB) Ensure() error {
	paths := db.Paths()
	for _, dir := range []string{paths.Root, paths.NodesDir, paths.HistoryDir, paths.DesiredDir, paths.DriftDir, paths.ApprovalsDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}

func (db DB) NodePath(nodeID string) string {
	return filepath.Join(db.Paths().NodesDir, safeName(nodeID)+".json")
}

func (db DB) HistoryPath(nodeID string) string {
	return filepath.Join(db.Paths().HistoryDir, safeName(nodeID)+".jsonl")
}

func (db DB) DesiredPath(name string) string {
	return filepath.Join(db.Paths().DesiredDir, safeName(name)+".yaml")
}

func (db DB) DriftPath(nodeID string) string {
	return filepath.Join(db.Paths().DriftDir, safeName(nodeID)+".jsonl")
}

func (db DB) AppendEvidenceIndex(entry EvidenceIndexEntry) error {
	if err := db.Ensure(); err != nil {
		return err
	}
	if entry.Time.IsZero() {
		entry.Time = time.Now().UTC()
	}
	entry.ID = strings.TrimSpace(entry.ID)
	entry.Kind = strings.TrimSpace(entry.Kind)
	entry.Node = strings.TrimSpace(entry.Node)
	entry.Summary = strings.TrimSpace(entry.Summary)
	entry.StoredAt = strings.TrimSpace(entry.StoredAt)
	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(db.Paths().EvidenceIndex, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(data, '\n'))
	return err
}

func (db DB) Recent(opts RecentOptions) (RecentEvents, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	events, err := db.readRecentEvents(opts, limit)
	if err != nil {
		return RecentEvents{}, err
	}
	evidence, err := db.readRecentEvidence(opts, limit)
	if err != nil {
		return RecentEvents{}, err
	}
	return RecentEvents{Events: events, Evidence: evidence}, nil
}

func (db DB) readRecentEvents(opts RecentOptions, limit int) ([]Event, error) {
	var paths []string
	if strings.TrimSpace(opts.Node) != "" {
		paths = append(paths, db.DriftPath(opts.Node))
	} else {
		dir := db.Paths().DriftDir
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, nil
			}
			return nil, err
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".jsonl" {
				continue
			}
			paths = append(paths, filepath.Join(dir, entry.Name()))
		}
	}
	var out []Event
	for _, path := range paths {
		items, err := readJSONL[Event](path)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if matchRecent(opts.Node, opts.Kind, item.Node, item.Kind) {
				out = append(out, item)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Time.After(out[j].Time)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (db DB) readRecentEvidence(opts RecentOptions, limit int) ([]EvidenceIndexEntry, error) {
	items, err := readJSONL[EvidenceIndexEntry](db.Paths().EvidenceIndex)
	if err != nil {
		return nil, err
	}
	var out []EvidenceIndexEntry
	for _, item := range items {
		if matchRecent(opts.Node, opts.Kind, item.Node, item.Kind) {
			out = append(out, item)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Time.After(out[j].Time)
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func readJSONL[T any](path string) ([]T, error) {
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()
	var out []T
	scanner := bufio.NewScanner(file)
	buf := make([]byte, 0, 1024*1024)
	scanner.Buffer(buf, 10*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var item T
		if err := json.Unmarshal([]byte(line), &item); err == nil {
			out = append(out, item)
		}
	}
	return out, scanner.Err()
}

func matchRecent(wantNode, wantKind, node, kind string) bool {
	if strings.TrimSpace(wantNode) != "" && strings.TrimSpace(wantNode) != strings.TrimSpace(node) {
		return false
	}
	if strings.TrimSpace(wantKind) != "" && strings.TrimSpace(wantKind) != strings.TrimSpace(kind) {
		return false
	}
	return true
}

func (db DB) AppendEvent(event Event) error {
	_, err := db.AppendEventRecord(event)
	return err
}

func (db DB) AppendEventRecord(event Event) (Event, error) {
	if err := db.Ensure(); err != nil {
		return Event{}, err
	}
	if event.Time.IsZero() {
		event.Time = time.Now().UTC()
	}
	event.Kind = strings.TrimSpace(event.Kind)
	if event.Kind == "" {
		event.Kind = "observation"
	}
	event.Summary = strings.TrimSpace(event.Summary)
	if event.Summary == "" {
		event.Summary = "no summary"
	}
	data, err := json.Marshal(event)
	if err != nil {
		return Event{}, err
	}
	path := db.DriftPath(firstNonEmpty(event.Node, "fleet"))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return Event{}, err
	}
	defer f.Close()
	if _, err := f.Write(append(data, '\n')); err != nil {
		return Event{}, err
	}
	return event, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func defaultRoot() string {
	if env := strings.TrimSpace(os.Getenv("MESHCLAW_OPSDB")); env != "" {
		return env
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".meshclaw", "state")
	}
	return filepath.Join(".", ".meshclaw", "state")
}

func safeName(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "default"
	}
	var b strings.Builder
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-', r == '_', r == '.':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "._-")
	if out == "" {
		return "default"
	}
	return out
}
