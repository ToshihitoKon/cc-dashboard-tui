package session

import (
	"testing"
	"time"
)

func Test_DeriveState_BusyWithinThreshold_ReturnsBusy(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC)
	lastActivity := now.Add(-59 * time.Second)

	got := DeriveState("busy", lastActivity, now)

	if got != StateBusy {
		t.Errorf("DeriveState() = %v, want StateBusy", got)
	}
}

func Test_DeriveState_BusyAtThresholdBoundary_ReturnsBusy(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC)
	lastActivity := now.Add(-staleThreshold) // ちょうど 60s は境界内（超過ではない）

	got := DeriveState("busy", lastActivity, now)

	if got != StateBusy {
		t.Errorf("DeriveState() = %v, want StateBusy", got)
	}
}

func Test_DeriveState_BusyBeyondThreshold_ReturnsBusyStale(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 2, 0, 0, time.UTC)
	lastActivity := now.Add(-61 * time.Second)

	got := DeriveState("busy", lastActivity, now)

	if got != StateBusyStale {
		t.Errorf("DeriveState() = %v, want StateBusyStale", got)
	}
}

func Test_DeriveState_Idle_ReturnsIdle(t *testing.T) {
	now := time.Now()

	got := DeriveState("idle", now.Add(-time.Hour), now)

	if got != StateIdle {
		t.Errorf("DeriveState() = %v, want StateIdle", got)
	}
}

func Test_DeriveState_EmptyStatus_ReturnsUnknown(t *testing.T) {
	now := time.Now()

	got := DeriveState("", now, now)

	if got != StateUnknown {
		t.Errorf("DeriveState() = %v, want StateUnknown", got)
	}
}

func Test_DisplayState_NeedsSpinner_OnlyBusyIsTrue(t *testing.T) {
	cases := []struct {
		state DisplayState
		want  bool
	}{
		{StateBusy, true},
		{StateBusyStale, false},
		{StateIdle, false},
		{StateUnknown, false},
	}

	for _, c := range cases {
		if got := c.state.NeedsSpinner(); got != c.want {
			t.Errorf("NeedsSpinner(%v) = %v, want %v", c.state, got, c.want)
		}
	}
}
