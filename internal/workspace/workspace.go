package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/meshclaw/meshclaw/internal/evidence"
)

type Workspace struct {
	ID           string     `json:"id"`
	Host         string     `json:"host"`
	Path         string     `json:"path"`
	Owner        string     `json:"owner,omitempty"`
	Branch       string     `json:"branch,omitempty"`
	Purpose      string     `json:"purpose,omitempty"`
	Source       string     `json:"source,omitempty"`
	Frontends    []Frontend `json:"frontends,omitempty"`
	UpdatedAt    time.Time  `json:"updated_at"`
	LastActivity string     `json:"last_activity,omitempty"`
}

type Frontend struct {
	ID        string    `json:"id"`
	Provider  string    `json:"provider"`
	Mode      string    `json:"mode"`
	App       string    `json:"app,omitempty"`
	URL       string    `json:"url,omitempty"`
	Status    string    `json:"status"`
	Login     string    `json:"login,omitempty"`
	Notes     string    `json:"notes,omitempty"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Activity struct {
	WorkspaceID string    `json:"workspace_id"`
	Host        string    `json:"host"`
	Path        string    `json:"path"`
	Actor       string    `json:"actor"`
	Action      string    `json:"action"`
	Summary     string    `json:"summary"`
	Evidence    string    `json:"evidence,omitempty"`
	Time        time.Time `json:"time"`
}

type Store struct {
	Version    string      `json:"version"`
	UpdatedAt  time.Time   `json:"updated_at"`
	Workspaces []Workspace `json:"workspaces"`
}

func List() ([]Workspace, string, error) {
	store, path, err := load()
	if err != nil {
		return nil, path, err
	}
	sort.Slice(store.Workspaces, func(i, j int) bool {
		return store.Workspaces[i].ID < store.Workspaces[j].ID
	})
	return store.Workspaces, path, nil
}

func Upsert(ws Workspace) (Workspace, string, error) {
	if strings.TrimSpace(ws.ID) == "" {
		ws.ID = defaultID(ws.Host, ws.Path)
	}
	if strings.TrimSpace(ws.Host) == "" {
		ws.Host = "local"
	}
	if strings.TrimSpace(ws.Path) == "" {
		return Workspace{}, "", fmt.Errorf("workspace path is required")
	}
	now := time.Now().UTC()
	ws.ID = sanitizeID(ws.ID)
	ws.UpdatedAt = now

	store, path, err := load()
	if err != nil {
		return Workspace{}, path, err
	}
	replaced := false
	for i, existing := range store.Workspaces {
		if existing.ID == ws.ID {
			if ws.Owner == "" {
				ws.Owner = existing.Owner
			}
			if ws.Branch == "" {
				ws.Branch = existing.Branch
			}
			if ws.Purpose == "" {
				ws.Purpose = existing.Purpose
			}
			if ws.Source == "" {
				ws.Source = existing.Source
			}
			if len(ws.Frontends) == 0 {
				ws.Frontends = existing.Frontends
			}
			if ws.LastActivity == "" {
				ws.LastActivity = existing.LastActivity
			}
			store.Workspaces[i] = ws
			replaced = true
			break
		}
	}
	if !replaced {
		store.Workspaces = append(store.Workspaces, ws)
	}
	store.UpdatedAt = now
	if err := save(path, store); err != nil {
		return Workspace{}, path, err
	}
	return ws, path, nil
}

func UpsertFrontend(workspaceID string, frontend Frontend) (Workspace, string, error) {
	workspaceID = sanitizeID(workspaceID)
	if workspaceID == "" {
		return Workspace{}, "", fmt.Errorf("workspace id is required")
	}
	frontend.ID = sanitizeID(firstNonEmpty(frontend.ID, frontend.Provider))
	if frontend.ID == "" {
		return Workspace{}, "", fmt.Errorf("frontend id or provider is required")
	}
	frontend.Provider = strings.ToLower(strings.TrimSpace(frontend.Provider))
	if frontend.Provider == "" {
		frontend.Provider = frontend.ID
	}
	if frontend.Mode == "" {
		frontend.Mode = "subscription_frontend"
	}
	if frontend.Status == "" {
		frontend.Status = "configured"
	}
	now := time.Now().UTC()
	frontend.UpdatedAt = now

	store, path, err := load()
	if err != nil {
		return Workspace{}, path, err
	}
	for wi := range store.Workspaces {
		if store.Workspaces[wi].ID != workspaceID {
			continue
		}
		replaced := false
		for fi := range store.Workspaces[wi].Frontends {
			if store.Workspaces[wi].Frontends[fi].ID == frontend.ID {
				store.Workspaces[wi].Frontends[fi] = frontend
				replaced = true
				break
			}
		}
		if !replaced {
			store.Workspaces[wi].Frontends = append(store.Workspaces[wi].Frontends, frontend)
		}
		store.Workspaces[wi].UpdatedAt = now
		store.UpdatedAt = now
		if err := save(path, store); err != nil {
			return Workspace{}, path, err
		}
		return store.Workspaces[wi], path, nil
	}
	return Workspace{}, path, fmt.Errorf("workspace %q not found", workspaceID)
}

func RecordActivity(activity Activity) (evidence.Record, error) {
	if strings.TrimSpace(activity.WorkspaceID) == "" {
		activity.WorkspaceID = defaultID(activity.Host, activity.Path)
	}
	if strings.TrimSpace(activity.Actor) == "" {
		activity.Actor = "unknown"
	}
	if strings.TrimSpace(activity.Action) == "" {
		activity.Action = "note"
	}
	if activity.Time.IsZero() {
		activity.Time = time.Now().UTC()
	}
	updateLastActivity(activity.WorkspaceID, activity.Summary)
	return evidence.Store("workspace-activity", activity.WorkspaceID, activity.Summary, activity)
}

func load() (Store, string, error) {
	path, err := storePath()
	if err != nil {
		return Store{}, "", err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Store{Version: "1", UpdatedAt: time.Now().UTC(), Workspaces: []Workspace{}}, path, nil
		}
		return Store{}, path, err
	}
	var store Store
	if err := json.Unmarshal(data, &store); err != nil {
		return Store{}, path, err
	}
	if store.Version == "" {
		store.Version = "1"
	}
	if store.Workspaces == nil {
		store.Workspaces = []Workspace{}
	}
	return store, path, nil
}

func save(path string, store Store) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}

func updateLastActivity(id, summary string) {
	store, path, err := load()
	if err != nil {
		return
	}
	now := time.Now().UTC()
	for i := range store.Workspaces {
		if store.Workspaces[i].ID == id {
			store.Workspaces[i].LastActivity = summary
			store.Workspaces[i].UpdatedAt = now
			store.UpdatedAt = now
			_ = save(path, store)
			return
		}
	}
}

func storePath() (string, error) {
	if path := strings.TrimSpace(os.Getenv("MESHCLAW_WORKSPACES_FILE")); path != "" {
		return path, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".meshclaw", "workspaces.json"), nil
}

func defaultID(host, path string) string {
	if strings.TrimSpace(host) == "" {
		host = "local"
	}
	base := strings.Trim(filepath.Base(strings.TrimRight(path, "/")), ".")
	if base == "" || base == "/" {
		base = "workspace"
	}
	return sanitizeID(host + "-" + base)
}

func sanitizeID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "workspace"
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
