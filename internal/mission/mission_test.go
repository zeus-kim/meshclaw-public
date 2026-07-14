package mission

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStoreGetActiveMission(t *testing.T) {
	dir := t.TempDir()
	store := Store{Dir: dir}
	writeFile(t, filepath.Join(dir, "active.json"), `{"id":"meshclaw-0.8"}`)
	writeFile(t, filepath.Join(dir, "meshclaw-0.8.json"), `{
  "id": "meshclaw-0.8",
  "goal": "Release MeshClaw 0.8",
  "status": "active",
  "next_action": "Fix vssh authentication",
  "tasks": [],
  "artifacts": [],
  "updated_at": "2026-05-31T00:00:00Z"
}`)

	got, path, err := store.Get("")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "meshclaw-0.8" || got.NextAction != "Fix vssh authentication" {
		t.Fatalf("mission=%#v", got)
	}
	if filepath.Base(path) != "meshclaw-0.8.json" {
		t.Fatalf("path=%q", path)
	}
}

func TestStoreListMissionSummaries(t *testing.T) {
	dir := t.TempDir()
	store := Store{Dir: dir}
	writeFile(t, filepath.Join(dir, "active.json"), `{"id":"meshclaw-0.8"}`)
	writeFile(t, filepath.Join(dir, "meshclaw-0.8.json"), `{
  "id": "meshclaw-0.8",
  "goal": "Release MeshClaw 0.8",
  "status": "active",
  "next_action": "Fix vssh authentication",
  "tasks": [],
  "artifacts": [],
  "updated_at": "2026-05-31T00:00:00Z"
}`)
	writeFile(t, filepath.Join(dir, "later.json"), `{
  "id": "later",
  "goal": "Later mission",
  "status": "blocked",
  "next_action": "",
  "tasks": [],
  "artifacts": [],
  "updated_at": "2026-05-31T00:00:01Z"
}`)

	got, path, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if path != dir {
		t.Fatalf("path=%q", path)
	}
	if len(got) != 2 || got[0].ID != "meshclaw-0.8" || got[0].Goal != "Release MeshClaw 0.8" {
		t.Fatalf("summaries=%#v", got)
	}
}

func TestStoreListMissingDirectoryIsEmpty(t *testing.T) {
	store := Store{Dir: filepath.Join(t.TempDir(), "missing")}
	got, _, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("summaries=%#v", got)
	}
}

func TestDecodeRejectsExpandedOrInvalidShape(t *testing.T) {
	if _, err := Decode([]byte(`{"id":"bad","goal":"x","status":"planning","next_action":"","tasks":[],"artifacts":[],"updated_at":"2026-05-31T00:00:00Z"}`)); err == nil {
		t.Fatal("invalid status was accepted")
	}
	if _, err := Decode([]byte(`{"id":"bad","goal":"x","status":"active","next_action":"","updated_at":"2026-05-31T00:00:00Z"}`)); err == nil {
		t.Fatal("missing arrays were accepted")
	}
}

func TestStoreRejectsPathTraversalID(t *testing.T) {
	store := Store{Dir: t.TempDir()}
	if _, _, err := store.Get("../secret"); err == nil {
		t.Fatal("path traversal id was accepted")
	}
}

func TestStoreUpdateMission(t *testing.T) {
	dir := t.TempDir()
	store := Store{Dir: dir}
	writeFile(t, filepath.Join(dir, "active.json"), `{"id":"meshclaw-0.8"}`)
	writeFile(t, filepath.Join(dir, "meshclaw-0.8.json"), sampleMissionJSON())

	next := "Wire Mission artifacts"
	got, path, err := store.Update("", UpdateOptions{NextAction: &next})
	if err != nil {
		t.Fatal(err)
	}
	if got.NextAction != next {
		t.Fatalf("NextAction=%q", got.NextAction)
	}
	reloaded, _, err := store.Get("meshclaw-0.8")
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.NextAction != next || path != filepath.Join(dir, "meshclaw-0.8.json") {
		t.Fatalf("reloaded=%#v path=%q", reloaded, path)
	}
}

func TestStoreTaskAndArtifactMutations(t *testing.T) {
	dir := t.TempDir()
	store := Store{Dir: dir}
	writeFile(t, filepath.Join(dir, "active.json"), `{"id":"meshclaw-0.8"}`)
	writeFile(t, filepath.Join(dir, "meshclaw-0.8.json"), sampleMissionJSON())

	got, task, _, err := store.AddTask("", "Add artifact MCP", "in_progress", "Core Phase 3")
	if err != nil {
		t.Fatal(err)
	}
	if task["id"] != "task-001" || task["status"] != "in_progress" || len(got.Tasks) != 1 {
		t.Fatalf("task=%#v mission=%#v", task, got)
	}
	got, task, _, err = store.CompleteTask("", "task-001", "done")
	if err != nil {
		t.Fatal(err)
	}
	if task["status"] != "done" || task["notes"] != "done" || len(got.Tasks) != 1 {
		t.Fatalf("task=%#v mission=%#v", task, got)
	}
	got, artifact, _, err := store.AddArtifact("", map[string]interface{}{
		"kind": "doc",
		"ref":  "macmini:/Users/argos/Documents/Argos Vault/Work Reports/foo.md",
		"node": "macmini",
	})
	if err != nil {
		t.Fatal(err)
	}
	if artifact["id"] != "art-001" || artifact["node"] != "macmini" || len(got.Artifacts) != 1 {
		t.Fatalf("artifact=%#v mission=%#v", artifact, got)
	}
	data, err := os.ReadFile(filepath.Join(dir, "meshclaw-0.8.json"))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["updated_at"] == "2026-05-31T00:00:00Z" {
		t.Fatalf("updated_at was not refreshed: %s", data)
	}
}

func TestStoreRejectsInvalidMutationInputs(t *testing.T) {
	dir := t.TempDir()
	store := Store{Dir: dir}
	writeFile(t, filepath.Join(dir, "active.json"), `{"id":"meshclaw-0.8"}`)
	writeFile(t, filepath.Join(dir, "meshclaw-0.8.json"), sampleMissionJSON())
	if _, _, _, err := store.AddTask("", "", "", ""); err == nil {
		t.Fatal("empty task title was accepted")
	}
	if _, _, _, err := store.AddTask("", "x", "planning", ""); err == nil {
		t.Fatal("invalid task status was accepted")
	}
	if _, _, _, err := store.CompleteTask("", "missing", ""); err == nil {
		t.Fatal("missing task completion was accepted")
	}
	if _, _, _, err := store.AddArtifact("", map[string]interface{}{"kind": "doc"}); err == nil {
		t.Fatal("artifact without ref was accepted")
	}
}

func sampleMissionJSON() string {
	return `{
  "id": "meshclaw-0.8",
  "goal": "Release MeshClaw 0.8",
  "status": "active",
  "next_action": "Fix vssh authentication",
  "tasks": [],
  "artifacts": [],
  "updated_at": "2026-05-31T00:00:00Z"
}`
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}
