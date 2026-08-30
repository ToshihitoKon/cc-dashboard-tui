package session

import (
	"testing"
	"time"
)

func Test_SortSessions_MixedStates_OrdersByStatePriority(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	sessions := []Session{
		{SessionID: "idle-1", State: StateIdle, LastActivity: now},
		{SessionID: "unknown-1", State: StateUnknown, LastActivity: now},
		{SessionID: "action-1", State: StateActionRequired, LastActivity: now},
		{SessionID: "busy-1", State: StateBusy, LastActivity: now},
	}

	SortSessions(sessions)

	want := []string{"action-1", "idle-1", "busy-1", "unknown-1"}
	for i, id := range want {
		if sessions[i].SessionID != id {
			t.Errorf("sessions[%d].SessionID = %q, want %q", i, sessions[i].SessionID, id)
		}
	}
}

func Test_SortSessions_SameState_OrdersByLastActivityDescending(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	sessions := []Session{
		{SessionID: "old", State: StateIdle, LastActivity: base},
		{SessionID: "new", State: StateIdle, LastActivity: base.Add(time.Hour)},
		{SessionID: "mid", State: StateIdle, LastActivity: base.Add(30 * time.Minute)},
	}

	SortSessions(sessions)

	want := []string{"new", "mid", "old"}
	for i, id := range want {
		if sessions[i].SessionID != id {
			t.Errorf("sessions[%d].SessionID = %q, want %q", i, sessions[i].SessionID, id)
		}
	}
}

func Test_SortSessions_SameLastActivity_TieBreaksBySessionID(t *testing.T) {
	// StateUnknown は LastActivity がゼロ値になりがち。SessionID による
	// タイブレークが無いと fs.ReadDir 由来の非決定な順序がそのまま出てしまう。
	sessions := []Session{
		{SessionID: "zzz", State: StateUnknown},
		{SessionID: "aaa", State: StateUnknown},
		{SessionID: "mmm", State: StateUnknown},
	}

	SortSessions(sessions)

	want := []string{"aaa", "mmm", "zzz"}
	for i, id := range want {
		if sessions[i].SessionID != id {
			t.Errorf("sessions[%d].SessionID = %q, want %q", i, sessions[i].SessionID, id)
		}
	}
}

func Test_SortSessions_EmptySlice_DoesNotPanic(t *testing.T) {
	sessions := []Session{}
	SortSessions(sessions)
	if len(sessions) != 0 {
		t.Errorf("len(sessions) = %d, want 0", len(sessions))
	}
}
