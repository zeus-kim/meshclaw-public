package messenger

import (
	"path/filepath"
	"testing"
)

func TestParseSignalRoomsJSON(t *testing.T) {
	rooms, err := ParseSignalRoomsJSON([]byte(`[
	  {"id":"guard-group","name":"MeshClaw Guard","isMember":true,"messageExpirationTime":300,"members":["+1"],"admins":["+2"]},
	  {"id":"old-group","name":"Argos-Zeus","isMember":true,"messageExpirationTime":86400},
	  {"id":"left-group","name":"Old Room","isMember":false}
	]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(rooms) != 3 {
		t.Fatalf("rooms=%d", len(rooms))
	}
	if rooms[0].ID != "guard-group" || rooms[0].MessageExpirationTime != 300 {
		t.Fatalf("room=%#v", rooms[0])
	}
}

func TestParseSignalRoomsJSONIgnoresSignalCLIInfoPrefix(t *testing.T) {
	rooms, err := ParseSignalRoomsJSON([]byte(`INFO  Manager - using account
[
  {"id":"guard-group","name":"MeshClaw Guard","isMember":true}
]`))
	if err != nil {
		t.Fatal(err)
	}
	if len(rooms) != 1 || rooms[0].ID != "guard-group" {
		t.Fatalf("rooms=%#v", rooms)
	}
}

func TestClassifyRoomsProtectsRegisteredTargets(t *testing.T) {
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(t.TempDir(), "targets.json"))
	if _, _, err := UpsertTarget(Target{ID: "argos-guard", Channel: "signal", GroupID: "guard-group", Label: "MeshClaw Guard", Mode: "guard"}); err != nil {
		t.Fatal(err)
	}
	store, err := ListTargets()
	if err != nil {
		t.Fatal(err)
	}
	statuses, missing := classifyRooms(store, []SignalRoom{
		{ID: "guard-group", Name: "MeshClaw Guard", IsMember: true},
		{ID: "old-group", Name: "Argos-Zeus", IsMember: true},
	})
	if len(missing) != 0 {
		t.Fatalf("missing=%#v", missing)
	}
	byID := map[string]RoomStatus{}
	for _, status := range statuses {
		byID[status.Room.ID] = status
	}
	guard := byID["guard-group"]
	if !guard.Protected || guard.CanDelete || guard.Class != "protected_active" {
		t.Fatalf("guard status=%#v", guard)
	}
	old := byID["old-group"]
	if old.Protected || !old.CanDelete || old.Class != "orphan_member" {
		t.Fatalf("old status=%#v", old)
	}
}

func TestClassifyRoomsReportsMissingTarget(t *testing.T) {
	t.Setenv("MESHCLAW_MESSENGER_TARGETS", filepath.Join(t.TempDir(), "targets.json"))
	if _, _, err := UpsertTarget(Target{ID: "ops", Channel: "signal", GroupID: "missing-group", Label: "Ops", Mode: "ops"}); err != nil {
		t.Fatal(err)
	}
	store, err := ListTargets()
	if err != nil {
		t.Fatal(err)
	}
	_, missing := classifyRooms(store, []SignalRoom{{ID: "other", Name: "Other", IsMember: true}})
	if len(missing) != 1 || missing[0].ID != "ops" {
		t.Fatalf("missing=%#v", missing)
	}
}

func TestAnalyzeRoomTopologyWarnsAboutSameMembersRoleRooms(t *testing.T) {
	statuses := []RoomStatus{
		{Room: SignalRoom{ID: "guard", Name: "MeshClaw Guard", IsMember: true, Members: []string{"+2", "+1"}}, TargetID: "argos-guard", Protected: true},
		{Room: SignalRoom{ID: "ops", Name: "MeshClaw Ops", IsMember: true, Members: []string{"+1", "+2"}}, TargetID: "meshclaw-ops", Protected: true},
		{Room: SignalRoom{ID: "chat", Name: "Argos Local Chat", IsMember: true, Members: []string{"+1", "+2"}}, TargetID: "argos-chat", Protected: true},
		{Room: SignalRoom{ID: "shared", Name: "Briefing", IsMember: true, Members: []string{"+1", "+2", "+3"}}, TargetID: "briefing", Protected: true},
	}
	warnings := analyzeRoomTopology(statuses)
	if len(warnings) != 1 {
		t.Fatalf("warnings=%#v, want 1", warnings)
	}
	if warnings[0].ID != "same_members_many_role_rooms" || len(warnings[0].Rooms) != 3 {
		t.Fatalf("warning=%#v", warnings[0])
	}
}

func TestAnalyzeRoomTopologyIgnoresSmallRoomSets(t *testing.T) {
	statuses := []RoomStatus{
		{Room: SignalRoom{ID: "a", Name: "A", IsMember: true, Members: []string{"+1", "+2"}}, TargetID: "a", Protected: true},
		{Room: SignalRoom{ID: "b", Name: "B", IsMember: true, Members: []string{"+1", "+2"}}, TargetID: "b", Protected: true},
	}
	if warnings := analyzeRoomTopology(statuses); len(warnings) != 0 {
		t.Fatalf("warnings=%#v, want none", warnings)
	}
}

func TestGuessRoomMode(t *testing.T) {
	cases := map[string]string{
		"서버 운영방":           "ops",
		"가족 브리핑":           "briefing",
		"비밀번호 보관":          "guard",
		"Argos local chat": "chat",
		"나의 아르고스":          "assistant",
	}
	for name, want := range cases {
		if got := guessRoomMode(name); got != want {
			t.Fatalf("guessRoomMode(%q)=%q, want %q", name, got, want)
		}
	}
}
