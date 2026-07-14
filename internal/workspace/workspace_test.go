package workspace

import (
	"path/filepath"
	"testing"
)

func TestUpsertAndListWorkspace(t *testing.T) {
	t.Setenv("MESHCLAW_WORKSPACES_FILE", filepath.Join(t.TempDir(), "workspaces.json"))

	ws, _, err := Upsert(Workspace{Host: "d1", Path: "/home/dell/kobolt", Owner: "codex", Purpose: "training cleanup"})
	if err != nil {
		t.Fatal(err)
	}
	if ws.ID != "d1-kobolt" {
		t.Fatalf("id = %q, want d1-kobolt", ws.ID)
	}

	list, _, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Owner != "codex" {
		t.Fatalf("list = %#v", list)
	}
}

func TestRecordActivityUpdatesWorkspace(t *testing.T) {
	t.Setenv("MESHCLAW_WORKSPACES_FILE", filepath.Join(t.TempDir(), "workspaces.json"))
	if _, _, err := Upsert(Workspace{ID: "main", Host: "local", Path: "/tmp/project"}); err != nil {
		t.Fatal(err)
	}
	record, err := RecordActivity(Activity{WorkspaceID: "main", Actor: "claude", Action: "review", Summary: "reviewed plan"})
	if err != nil {
		t.Fatal(err)
	}
	if record.Kind != "workspace-activity" {
		t.Fatalf("record kind = %q", record.Kind)
	}
	list, _, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if list[0].LastActivity != "reviewed plan" {
		t.Fatalf("last activity = %q", list[0].LastActivity)
	}
}
