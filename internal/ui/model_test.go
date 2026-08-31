package ui

import (
	"strings"
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

func Test_IsLongRun_ZeroLastActivity_ReturnsFalse(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC)
	s := session.Session{State: session.StateBusy, LastActivity: time.Time{}}

	if isLongRun(s, now) {
		t.Error("isLongRun() = true, want false（LastActivity がゼロ値の場合は判定不能として除外）")
	}
}

func Test_ElapsedLabel_ZeroLastActivity_ReturnsPlaceholder(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC)

	got := elapsedLabel(time.Time{}, now)
	if got != "-" {
		t.Errorf("elapsedLabel() = %q, want %q（LastActivity 取得失敗時のプレースホルダー）", got, "-")
	}
}

func Test_ElapsedLabel_NormalLastActivity_ReturnsFormattedElapsed(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC)
	lastActivity := now.Add(-5 * time.Minute)

	got := elapsedLabel(lastActivity, now)
	if got != "5m" {
		t.Errorf("elapsedLabel() = %q, want %q", got, "5m")
	}
}

// 起動直後で jsonl の取得に失敗し LastActivity がゼロ値のまま renderSession に
// 渡されても、status 列が折り返されて行が増えないことを確認する回帰テスト。
// 修正前は "● idl (106751d)" のような異常に長い文字列が生成され、
// lipgloss の単語折り返しにより1セッションの表示が2行に分かれていた。
func Test_RenderSession_ZeroLastActivity_RendersSingleLine(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC)
	s := session.Session{State: session.StateIdle, StartedAt: now.Add(-36 * time.Second)}

	got := NewModel(nil).renderSession(s, now)
	if strings.Contains(got, "\n") {
		t.Errorf("renderSession() が改行を含む（status 列が折り返された）: %q", got)
	}
}

// StartedAt はレジストリJSONの startedAt キーが欠損すると time.UnixMilli(0)
// （1970年1月1日）になりうる。time.Time{} と異なり IsZero() では検出できない値だが、
// truncate による切り詰めで started 列の折り返しは防げることを確認する。
func Test_RenderSession_EpochStartedAt_RendersSingleLine(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 10, 0, 0, time.UTC)
	s := session.Session{State: session.StateIdle, StartedAt: time.UnixMilli(0)}

	got := NewModel(nil).renderSession(s, now)
	if strings.Contains(got, "\n") {
		t.Errorf("renderSession() が改行を含む（started 列が折り返された）: %q", got)
	}
}
