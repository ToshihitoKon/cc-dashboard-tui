package session

import (
	"testing"
	"time"
)

func Test_DeriveState_BusyWithinThreshold_ReturnsBusy(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC)
	lastActivity := now.Add(-59 * time.Second)

	got := DeriveState(StateInput{RawStatus: "busy", LastActivity: lastActivity, Now: now})

	if got != StateBusy {
		t.Errorf("DeriveState() = %v, want StateBusy", got)
	}
}

func Test_DeriveState_BusyAtThresholdBoundary_ReturnsBusy(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 1, 0, 0, time.UTC)
	lastActivity := now.Add(-staleThreshold) // ちょうど 60s は境界内（超過ではない）

	got := DeriveState(StateInput{RawStatus: "busy", LastActivity: lastActivity, Now: now})

	if got != StateBusy {
		t.Errorf("DeriveState() = %v, want StateBusy", got)
	}
}

func Test_DeriveState_BusyBeyondThreshold_ReturnsBusyStale(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 2, 0, 0, time.UTC)
	lastActivity := now.Add(-61 * time.Second)

	got := DeriveState(StateInput{RawStatus: "busy", LastActivity: lastActivity, Now: now})

	if got != StateBusyStale {
		t.Errorf("DeriveState() = %v, want StateBusyStale", got)
	}
}

func Test_DeriveState_Idle_ReturnsIdle(t *testing.T) {
	now := time.Now()

	got := DeriveState(StateInput{RawStatus: "idle", LastActivity: now.Add(-time.Hour), Now: now})

	if got != StateIdle {
		t.Errorf("DeriveState() = %v, want StateIdle", got)
	}
}

func Test_DeriveState_EmptyStatus_ReturnsUnknown(t *testing.T) {
	now := time.Now()

	got := DeriveState(StateInput{RawStatus: "", LastActivity: now, Now: now})

	if got != StateUnknown {
		t.Errorf("DeriveState() = %v, want StateUnknown", got)
	}
}

func Test_DeriveState_ActionRequiredWithinTTL_OverridesBusy(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC)

	got := DeriveState(StateInput{
		RawStatus:        "busy", // 通知直後は registry 上まだ busy のことが多い
		LastActivity:     now,
		Now:              now,
		ActionRequiredAt: now.Add(-time.Minute),
	})

	if got != StateActionRequired {
		t.Errorf("DeriveState() = %v, want StateActionRequired（busy より優先されるべき）", got)
	}
}

func Test_DeriveState_ActionRequiredAtThresholdBoundary_StillActionRequired(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC)

	got := DeriveState(StateInput{
		RawStatus:        "busy",
		LastActivity:     now,
		Now:              now,
		ActionRequiredAt: now.Add(-actionRequiredTTL), // ちょうど TTL は境界内
	})

	if got != StateActionRequired {
		t.Errorf("DeriveState() = %v, want StateActionRequired", got)
	}
}

func Test_DeriveState_ActionRequiredBeyondTTL_FallsBackToRawStatus(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC)

	got := DeriveState(StateInput{
		RawStatus:        "busy",
		LastActivity:     now,
		Now:              now,
		ActionRequiredAt: now.Add(-actionRequiredTTL - time.Second),
	})

	if got != StateBusy {
		t.Errorf("DeriveState() = %v, want StateBusy（TTL 超過で通常判定にフォールバックすべき）", got)
	}
}

func Test_DeriveState_NoActionRequiredFile_FallsBackToRawStatus(t *testing.T) {
	now := time.Now()

	// ActionRequiredAt をゼロ値のまま渡す = hook 未設定、または現在待ちなし
	got := DeriveState(StateInput{RawStatus: "busy", LastActivity: now, Now: now})

	if got != StateBusy {
		t.Errorf("DeriveState() = %v, want StateBusy", got)
	}
}

func Test_DeriveState_ActionRequiredFileButRawStatusIdle_IsIgnored(t *testing.T) {
	// idle は registry だけで判断が完結するため、hook の情報を見ない。
	// （実務上 idle のときに action-required ファイルが残っているのは
	// 解除漏れのはずだが、その場合も registry の値をそのまま信頼する）
	now := time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC)

	got := DeriveState(StateInput{
		RawStatus:        "idle",
		LastActivity:     now,
		Now:              now,
		ActionRequiredAt: now.Add(-time.Minute),
	})

	if got != StateIdle {
		t.Errorf("DeriveState() = %v, want StateIdle（idle のときは hook を見ないべき）", got)
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
		{StateActionRequired, false},
	}

	for _, c := range cases {
		if got := c.state.NeedsSpinner(); got != c.want {
			t.Errorf("NeedsSpinner(%v) = %v, want %v", c.state, got, c.want)
		}
	}
}
