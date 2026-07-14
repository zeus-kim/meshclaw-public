package messenger

import (
	"testing"
	"time"
)

func TestFilterUpcomingCalendarEventsDropsPast(t *testing.T) {
	now := time.Date(2026, 5, 30, 15, 0, 0, 0, time.UTC)
	events := []calendarListEvent{
		{Title: "Past meeting", Start: "2026-05-20T10:00:00Z", End: "2026-05-20T11:00:00Z"},
		{Title: "Upcoming dinner", Start: "2026-05-30T18:00:00Z", End: "2026-05-30T20:00:00Z"},
	}
	got := filterUpcomingCalendarEvents(events, now)
	if len(got) != 1 || got[0].Title != "Upcoming dinner" {
		t.Fatalf("got=%#v", got)
	}
}
