// Package mission implements MeshClaw Core Mission State v0.
//
// Phase 1 stores the canonical Core mission files on the MacBook user account
// that runs AI Studio MCP, normally /Users/example. The macmini Signal Argos
// dispatcher has its own ~/.meshclaw and must not silently duplicate these
// files; remote read/sync for Signal is a Phase 2 design.
package mission

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Mission struct {
	ID         string                   `json:"id"`
	Goal       string                   `json:"goal"`
	Status     string                   `json:"status"`
	NextAction string                   `json:"next_action"`
	Tasks      []map[string]interface{} `json:"tasks"`
	Artifacts  []map[string]interface{} `json:"artifacts"`
	UpdatedAt  string                   `json:"updated_at"`
}

type UpdateOptions struct {
	Goal       *string
	Status     *string
	NextAction *string
}

type Summary struct {
	ID         string `json:"id"`
	Goal       string `json:"goal"`
	Status     string `json:"status"`
	NextAction string `json:"next_action"`
	UpdatedAt  string `json:"updated_at"`
}

type ActiveRef struct {
	ID string `json:"id"`
}

type Store struct {
	Dir string
}

func DefaultStore() (Store, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Store{}, err
	}
	return Store{Dir: filepath.Join(home, ".meshclaw", "state", "missions")}, nil
}

func (s Store) Get(id string) (Mission, string, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		active, _, err := s.Active()
		if err != nil {
			return Mission{}, "", err
		}
		id = active.ID
	}
	if err := validateID(id); err != nil {
		return Mission{}, "", err
	}
	path := filepath.Join(s.Dir, id+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return Mission{}, path, err
	}
	mission, err := Decode(data)
	if err != nil {
		return Mission{}, path, fmt.Errorf("%s: %w", path, err)
	}
	if mission.ID != id {
		return Mission{}, path, fmt.Errorf("%s: mission id %q does not match filename id %q", path, mission.ID, id)
	}
	return mission, path, nil
}

func (s Store) List() ([]Summary, string, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []Summary{}, s.Dir, nil
		}
		return nil, s.Dir, err
	}
	out := []Summary{}
	for _, entry := range entries {
		if entry.IsDir() || entry.Name() == "active.json" || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		path := filepath.Join(s.Dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, s.Dir, err
		}
		mission, err := Decode(data)
		if err != nil {
			return nil, s.Dir, fmt.Errorf("%s: %w", path, err)
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		if mission.ID != id {
			return nil, s.Dir, fmt.Errorf("%s: mission id %q does not match filename id %q", path, mission.ID, id)
		}
		out = append(out, Summary{
			ID:         mission.ID,
			Goal:       mission.Goal,
			Status:     mission.Status,
			NextAction: mission.NextAction,
			UpdatedAt:  mission.UpdatedAt,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Status == "active" && out[j].Status != "active" {
			return true
		}
		if out[i].Status != "active" && out[j].Status == "active" {
			return false
		}
		return out[i].ID < out[j].ID
	})
	return out, s.Dir, nil
}

func (s Store) Active() (ActiveRef, string, error) {
	path := filepath.Join(s.Dir, "active.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return ActiveRef{}, path, errors.New("mission id is required because active.json is not configured")
		}
		return ActiveRef{}, path, err
	}
	var ref ActiveRef
	if err := json.Unmarshal(data, &ref); err != nil {
		return ActiveRef{}, path, err
	}
	ref.ID = strings.TrimSpace(ref.ID)
	if err := validateID(ref.ID); err != nil {
		return ActiveRef{}, path, err
	}
	return ref, path, nil
}

func (s Store) Update(id string, opts UpdateOptions) (Mission, string, error) {
	return s.mutate(id, func(m *Mission) error {
		if opts.Goal != nil {
			m.Goal = strings.TrimSpace(*opts.Goal)
		}
		if opts.Status != nil {
			m.Status = strings.TrimSpace(*opts.Status)
		}
		if opts.NextAction != nil {
			m.NextAction = strings.TrimSpace(*opts.NextAction)
		}
		return nil
	})
}

func (s Store) AddTask(id, title, status, notes string) (Mission, map[string]interface{}, string, error) {
	var added map[string]interface{}
	mission, path, err := s.mutate(id, func(m *Mission) error {
		title = strings.TrimSpace(title)
		if title == "" {
			return errors.New("task title is required")
		}
		status = strings.TrimSpace(status)
		if status == "" {
			status = "pending"
		}
		if err := validateTaskStatus(status); err != nil {
			return err
		}
		added = map[string]interface{}{
			"id":     nextItemID(m.Tasks, "task"),
			"title":  title,
			"status": status,
			"notes":  strings.TrimSpace(notes),
		}
		m.Tasks = append(m.Tasks, added)
		return nil
	})
	return mission, added, path, err
}

func (s Store) CompleteTask(id, taskID, notes string) (Mission, map[string]interface{}, string, error) {
	var updated map[string]interface{}
	mission, path, err := s.mutate(id, func(m *Mission) error {
		taskID = strings.TrimSpace(taskID)
		if taskID == "" {
			return errors.New("task id is required")
		}
		for _, task := range m.Tasks {
			if stringValue(task["id"]) == taskID {
				task["status"] = "done"
				if strings.TrimSpace(notes) != "" {
					task["notes"] = strings.TrimSpace(notes)
				}
				updated = task
				return nil
			}
		}
		return fmt.Errorf("task %q not found", taskID)
	})
	return mission, updated, path, err
}

func (s Store) AddArtifact(id string, artifact map[string]interface{}) (Mission, map[string]interface{}, string, error) {
	var added map[string]interface{}
	mission, path, err := s.mutate(id, func(m *Mission) error {
		kind := strings.TrimSpace(stringValue(artifact["kind"]))
		ref := strings.TrimSpace(stringValue(artifact["ref"]))
		if kind == "" {
			kind = "file"
		}
		if ref == "" {
			return errors.New("artifact ref is required")
		}
		added = map[string]interface{}{
			"id":   nextItemID(m.Artifacts, "art"),
			"kind": kind,
			"ref":  ref,
		}
		for _, key := range []string{"title", "node", "evidence", "notes"} {
			if value := strings.TrimSpace(stringValue(artifact[key])); value != "" {
				added[key] = value
			}
		}
		m.Artifacts = append(m.Artifacts, added)
		return nil
	})
	return mission, added, path, err
}

func (s Store) mutate(id string, fn func(*Mission) error) (Mission, string, error) {
	mission, path, err := s.Get(id)
	if err != nil {
		return Mission{}, path, err
	}
	if fn != nil {
		if err := fn(&mission); err != nil {
			return Mission{}, path, err
		}
	}
	mission.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	if err := mission.Validate(); err != nil {
		return Mission{}, path, err
	}
	if err := writeMissionFile(path, mission); err != nil {
		return Mission{}, path, err
	}
	return mission, path, nil
}

func Decode(data []byte) (Mission, error) {
	var mission Mission
	if err := json.Unmarshal(data, &mission); err != nil {
		return Mission{}, err
	}
	return mission, mission.Validate()
}

func (m Mission) Summary() Summary {
	return Summary{
		ID:         m.ID,
		Goal:       m.Goal,
		Status:     m.Status,
		NextAction: m.NextAction,
		UpdatedAt:  m.UpdatedAt,
	}
}

func (m Mission) Validate() error {
	if err := validateID(strings.TrimSpace(m.ID)); err != nil {
		return err
	}
	if strings.TrimSpace(m.Goal) == "" {
		return errors.New("goal is required")
	}
	switch strings.TrimSpace(m.Status) {
	case "active", "blocked", "done", "archived":
	default:
		return fmt.Errorf("invalid status %q", m.Status)
	}
	if m.Tasks == nil {
		return errors.New("tasks is required")
	}
	if m.Artifacts == nil {
		return errors.New("artifacts is required")
	}
	if strings.TrimSpace(m.UpdatedAt) == "" {
		return errors.New("updated_at is required")
	}
	if _, err := time.Parse(time.RFC3339, m.UpdatedAt); err != nil {
		return fmt.Errorf("updated_at must be RFC3339: %w", err)
	}
	return nil
}

func validateTaskStatus(status string) error {
	switch strings.TrimSpace(status) {
	case "pending", "in_progress", "done", "blocked":
		return nil
	default:
		return fmt.Errorf("invalid task status %q", status)
	}
}

func validateID(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return errors.New("id is required")
	}
	if filepath.Base(id) != id || id == "." || id == ".." || strings.Contains(id, string(filepath.Separator)) {
		return fmt.Errorf("invalid mission id %q", id)
	}
	return nil
}

func nextItemID(items []map[string]interface{}, prefix string) string {
	max := 0
	for _, item := range items {
		id := stringValue(item["id"])
		if !strings.HasPrefix(id, prefix+"-") {
			continue
		}
		var n int
		if _, err := fmt.Sscanf(id, prefix+"-%03d", &n); err == nil && n > max {
			max = n
		}
	}
	return fmt.Sprintf("%s-%03d", prefix, max+1)
}

func stringValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	default:
		return ""
	}
}

func writeMissionFile(path string, mission Mission) error {
	data, err := json.MarshalIndent(mission, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(0600); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
