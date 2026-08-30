package ui

import (
	"testing"
	"time"

	"github.com/ToshihitoKon/cc-dashboard-tui/internal/session"
)

func Test_IsLongRun_BusyJustUnderThreshold_ReturnsFalse(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC)
	s := session.Session{State: session.StateBusy, LastActivity: now.Add(-longRunThreshold + time.Second)}

	if isLongRun(s, now) {
		t.Error("isLongRun() = true, want false（閾値未満）")
	}
}

func Test_IsLongRun_BusyAtThresholdBoundary_ReturnsFalse(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC)
	s := session.Session{State: session.StateBusy, LastActivity: now.Add(-longRunThreshold)}

	if isLongRun(s, now) {
		t.Error("isLongRun() = true, want false（ちょうど閾値は境界内）")
	}
}

func Test_IsLongRun_BusyJustOverThreshold_ReturnsTrue(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC)
	s := session.Session{State: session.StateBusy, LastActivity: now.Add(-longRunThreshold - time.Second)}

	if !isLongRun(s, now) {
		t.Error("isLongRun() = false, want true（閾値超過）")
	}
}

func Test_IsLongRun_NonBusyStateOverThreshold_ReturnsFalse(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC)
	longAgo := now.Add(-longRunThreshold - time.Hour)

	for _, state := range []session.DisplayState{session.StateIdle, session.StateActionRequired, session.StateUnknown} {
		s := session.Session{State: state, LastActivity: longAgo}
		if isLongRun(s, now) {
			t.Errorf("isLongRun() = true for state %v, want false（busy 以外は対象外）", state)
		}
	}
}
