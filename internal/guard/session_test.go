package guard

import "testing"

func TestGuardSessionLifecycle(t *testing.T) {
	t.Setenv("MESHCLAW_GUARD_SESSION_DIR", t.TempDir())
	session, err := StartSession(SessionStartOptions{Surface: "signal"})
	if err != nil {
		t.Fatal(err)
	}
	if session.Status != "active" || session.RawStored {
		t.Fatalf("unexpected session: %+v", session)
	}
	sessions, err := ListSessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 || sessions[0].ID != session.ID {
		t.Fatalf("sessions = %+v", sessions)
	}
	ended, err := EndSession(session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if ended.Status != "ended" || ended.Transcript != "purged" || ended.Memory != "purged" || ended.RAG != "purged" {
		t.Fatalf("session was not purged on end: %+v", ended)
	}
	result, err := PurgeSessions()
	if err != nil {
		t.Fatal(err)
	}
	if result["removed"].(int) != 1 {
		t.Fatalf("purge result = %+v", result)
	}
}
