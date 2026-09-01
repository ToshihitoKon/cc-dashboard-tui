package session

import (
	"testing"
	"time"
)

func Test_DeriveState_Busy_ReturnsBusy(t *testing.T) {
	now := time.Now()

	got := DeriveState(StateInput{RawStatus: "busy", Now: now})

	if got != StateBusy {
		t.Errorf("DeriveState() = %v, want StateBusy", got)
	}
}

func Test_DeriveState_Idle_ReturnsIdle(t *testing.T) {
	now := time.Now()

	got := DeriveState(StateInput{RawStatus: "idle", Now: now})

	if got != StateIdle {
		t.Errorf("DeriveState() = %v, want StateIdle", got)
	}
}

func Test_DeriveState_Waiting_ReturnsActionRequired(t *testing.T) {
	now := time.Now()

	got := DeriveState(StateInput{RawStatus: "waiting", Now: now})

	if got != StateActionRequired {
		t.Errorf("DeriveState() = %v, want StateActionRequired（waiting は hook 不要で action-required とする実装）", got)
	}
}

func Test_DeriveState_WaitingWithStaleActionRequiredAt_StillActionRequired(t *testing.T) {
	// waiting は busy と異なり TTL の対象外という実装。hookstate が無くても、
	// あるいは古くても常に action-required になる。
	now := time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC)

	got := DeriveState(StateInput{
		RawStatus:        "waiting",
		Now:              now,
		ActionRequiredAt: now.Add(-actionRequiredTTL - time.Second), // TTL を過ぎていても無関係
	})

	if got != StateActionRequired {
		t.Errorf("DeriveState() = %v, want StateActionRequired（waiting に TTL は適用されない）", got)
	}
}

func Test_DeriveState_EmptyStatus_ReturnsUnknown(t *testing.T) {
	now := time.Now()

	got := DeriveState(StateInput{RawStatus: "", Now: now})

	if got != StateUnknown {
		t.Errorf("DeriveState() = %v, want StateUnknown", got)
	}
}

func Test_DeriveState_ActionRequiredWithinTTL_OverridesBusy(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC)

	got := DeriveState(StateInput{
		RawStatus:         "busy", // 通知直後は registry 上まだ busy のことが多い
		Now:               now,
		ActionRequiredAt:  now.Add(-time.Minute),
		HasPendingToolUse: true,
	})

	if got != StateActionRequired {
		t.Errorf("DeriveState() = %v, want StateActionRequired（busy より優先されるべき）", got)
	}
}

func Test_DeriveState_ActionRequiredAtThresholdBoundary_StillActionRequired(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC)

	got := DeriveState(StateInput{
		RawStatus:         "busy",
		Now:               now,
		ActionRequiredAt:  now.Add(-actionRequiredTTL), // ちょうど TTL は境界内
		HasPendingToolUse: true,
	})

	if got != StateActionRequired {
		t.Errorf("DeriveState() = %v, want StateActionRequired", got)
	}
}

func Test_DeriveState_ActionRequiredBeyondTTL_FallsBackToRawStatus(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC)

	got := DeriveState(StateInput{
		RawStatus:         "busy",
		Now:               now,
		ActionRequiredAt:  now.Add(-actionRequiredTTL - time.Second),
		HasPendingToolUse: true,
	})

	if got != StateBusy {
		t.Errorf("DeriveState() = %v, want StateBusy（TTL 超過で通常判定にフォールバックすべき。tool_use が拒否されて未解決のまま残るケースの保険）", got)
	}
}

func Test_DeriveState_NoPendingToolUse_ReturnsBusy(t *testing.T) {
	// 承認されてツールが完了し tool_result が append された後は
	// action-required を解除し、通常の busy 判定に戻るべき。
	now := time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC)

	got := DeriveState(StateInput{
		RawStatus:         "busy",
		Now:               now,
		ActionRequiredAt:  now.Add(-time.Minute),
		HasPendingToolUse: false,
	})

	if got != StateBusy {
		t.Errorf("DeriveState() = %v, want StateBusy（未解決の tool_use が無ければ解除されるべき）", got)
	}
}

func Test_DeriveState_NoActionRequiredFile_FallsBackToRawStatus(t *testing.T) {
	now := time.Now()

	// ActionRequiredAt をゼロ値のまま渡す = hook 未設定、または現在待ちなし
	got := DeriveState(StateInput{RawStatus: "busy", Now: now})

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
