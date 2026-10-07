package ui

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ToshihitoKon/cc-dashboard-tui/internal/session"
	"github.com/ToshihitoKon/cc-dashboard-tui/internal/source"
	"github.com/ToshihitoKon/cc-dashboard-tui/internal/usage"
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

func Test_FormatUntilReset_EachRange_UsesTwoUnits(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want string
	}{
		{30 * time.Second, "<1m"},
		{time.Minute, "1m"},
		{45 * time.Minute, "45m"},
		{time.Hour, "1h00m"},
		{2*time.Hour + 5*time.Minute, "2h05m"},
		{24 * time.Hour, "1d00h"},
		{3*24*time.Hour + 4*time.Hour + 30*time.Minute, "3d04h"},
	}
	for _, tt := range tests {
		if got := formatUntilReset(tt.d); got != tt.want {
			t.Errorf("formatUntilReset(%v) = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func Test_UsageColor_AtThresholds_SwitchesColor(t *testing.T) {
	tests := []struct {
		usedPercent float64
		want        string
	}{
		{49, "42"},
		{50, "220"},
		{79, "220"},
		{80, "203"},
	}
	for _, tt := range tests {
		if got := usageColor(tt.usedPercent); got != tt.want {
			t.Errorf("usageColor(%v) = %q, want %q", tt.usedPercent, got, tt.want)
		}
	}
}

func Test_RenderUsageWindow_FiveHourBeforeReset_ShowsResetTimeAndRemaining(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	v := usageWindowView{"5h", usage.Window{UsedPercent: 42, ResetsAt: now.Add(2*time.Hour + 13*time.Minute)}, "15:04"}

	got := NewModel(nil).renderUsageWindow(v, 10, now)

	for _, want := range []string{"5h", "42%", "↻ 14:13 (2h13m)"} {
		if !strings.Contains(got, want) {
			t.Errorf("renderUsageWindow() = %q, want %q を含む", got, want)
		}
	}
	if n := strings.Count(got, "█"); n != 4 {
		t.Errorf("バーの塗り = %d 文字, want 4（42%% を 10 文字幅で四捨五入）", n)
	}
}

func Test_RenderUsage_SevenDayBeforeReset_ShowsMonthDayAndTime(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	m := NewModel(nil)
	m.rateLimits = usage.Limits{SevenDay: &usage.Window{UsedPercent: 18, ResetsAt: now.Add(3*24*time.Hour + 4*time.Hour)}}

	got := m.renderUsage(now)

	if !strings.Contains(got, "↻ 01/04 16:00 (3d04h)") {
		t.Errorf("renderUsage() = %q, want %q を含む（月/日 時刻）", got, "↻ 01/04 16:00 (3d04h)")
	}
}

func Test_RenderUsageWindow_AfterReset_ShowsUnknown(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	v := usageWindowView{"5h", usage.Window{UsedPercent: 90, ResetsAt: now.Add(-time.Minute)}, "15:04"}

	got := NewModel(nil).renderUsageWindow(v, 10, now)

	if !strings.Contains(got, "--  ↻ -") {
		t.Errorf("renderUsageWindow() = %q, want %q を含む（リセット後の使用率は不明）", got, "--  ↻ -")
	}
	if strings.Contains(got, "90%") || strings.Contains(got, "█") {
		t.Errorf("renderUsageWindow() = %q, リセット前の使用率が残っている", got)
	}
}

// countBarCells は使用率バーの文字数（塗り + 空き）を数える。
func countBarCells(s string) int {
	return strings.Count(s, "█") + strings.Count(s, "░")
}

// modelWithBothWindows は 5時間・7日間の両方の枠を持ち、端末幅が width の Model を作る。
func modelWithBothWindows(width int, now time.Time) Model {
	m := NewModel(nil)
	m.width = width
	m.rateLimits = usage.Limits{
		FiveHour: &usage.Window{UsedPercent: 52, ResetsAt: now.Add(10 * time.Minute)},
		SevenDay: &usage.Window{UsedPercent: 36, ResetsAt: now.Add(22*time.Hour + 10*time.Minute)},
	}
	return m
}

func Test_RenderUsage_BothWindows_RendersOneLinePerWindowWithAlignedBars(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	lines := strings.Split(modelWithBothWindows(70, now).renderUsage(now), "\n")

	if len(lines) != 2 || !strings.Contains(lines[0], "5h") || !strings.Contains(lines[1], "7d") {
		t.Fatalf("renderUsage() の行 = %q, want 5h と 7d の 2 行", lines)
	}
	if countBarCells(lines[0]) != countBarCells(lines[1]) {
		t.Errorf("バーの文字数 = %d / %d, want 2 行で揃う", countBarCells(lines[0]), countBarCells(lines[1]))
	}
}

func Test_RenderUsage_MediumTerminal_StretchesBarsToFillWidth(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	const width = 70

	lines := strings.Split(modelWithBothWindows(width, now).renderUsage(now), "\n")

	// バー以外の部分が最も長い 7d の行が、端末幅をちょうど埋める。
	if w := lipgloss.Width(lines[1]); w != width {
		t.Errorf("7d の行の幅 = %d, want %d: %q", w, width, lines[1])
	}
}

func Test_RenderUsage_NarrowTerminal_KeepsMinBarWidth(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	got := modelWithBothWindows(30, now).renderUsage(now)

	if n := countBarCells(got); n != 2*usageBarMinWidth {
		t.Errorf("バーの文字数 = %d, want %d（2 枠とも下限の幅）", n, 2*usageBarMinWidth)
	}
}

func Test_RenderUsage_VeryWideTerminal_CapsBarWidth(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	got := modelWithBothWindows(300, now).renderUsage(now)

	if n := countBarCells(got); n != 2*usageBarMaxWidth {
		t.Errorf("バーの文字数 = %d, want %d（2 枠とも上限の幅）", n, 2*usageBarMaxWidth)
	}
}

func Test_RenderUsage_OnlyFiveHour_OmitsSevenDay(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	m := NewModel(nil)
	m.rateLimits = usage.Limits{FiveHour: &usage.Window{UsedPercent: 42, ResetsAt: now.Add(time.Hour)}}

	got := m.renderUsage(now)

	if !strings.Contains(got, "5h") || strings.Contains(got, "7d") {
		t.Errorf("renderUsage() = %q, want 5h のみ", got)
	}
}

func Test_ContentHeight_WithActiveWindows_ReservesOneLinePerWindow(t *testing.T) {
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	withoutUsage := NewModel(nil)
	withoutUsage.height = 20
	withBoth := modelWithBothWindows(80, now)
	withBoth.height = 20

	if diff := withoutUsage.contentHeight(now) - withBoth.contentHeight(now); diff != 2 {
		t.Errorf("contentHeight() の差 = %d, want 2（5h と 7d の 2 行分）", diff)
	}
}

func Test_ContentHeight_AllWindowsExpired_DoesNotReserveUsageLine(t *testing.T) {
	// API キー利用に切り替えた後などに古い記録が残っていても、footer を 1 行に戻す。
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	m := NewModel(nil)
	m.height = 20
	withoutUsage := m.contentHeight(now)

	m.rateLimits = usage.Limits{FiveHour: &usage.Window{UsedPercent: 42, ResetsAt: now.Add(-time.Hour)}}

	if got := m.contentHeight(now); got != withoutUsage {
		t.Errorf("contentHeight() = %d, want %d（全枠がリセット済みなら使用率の行を出さない）", got, withoutUsage)
	}
}

// updateModel は Update の戻り値を Model に戻すテスト用のヘルパー。
func updateModel(t *testing.T, m Model, msg tea.Msg) Model {
	t.Helper()
	updated, _ := m.Update(msg)
	return updated.(Model)
}

func Test_View_NarrowTerminal_TruncatesUsageLinesToTerminalWidth(t *testing.T) {
	const terminalWidth = 30
	now := time.Now()
	active := usage.Limits{
		FiveHour: &usage.Window{UsedPercent: 52, ResetsAt: now.Add(10 * time.Minute)},
		SevenDay: &usage.Window{UsedPercent: 36, ResetsAt: now.Add(22 * time.Hour)},
	}
	m := updateModel(t, NewModel(nil), tea.WindowSizeMsg{Width: terminalWidth, Height: 20})
	m = updateModel(t, m, sessionsMsg(source.LoadResult{RateLimits: active}))

	// タイトル等は対象外。端末幅を超える使用率の行は、端末側で折り返されて画面の高さがずれる。
	var usageLines []string
	for _, line := range strings.Split(m.View(), "\n") {
		if strings.HasPrefix(line, "5h") || strings.HasPrefix(line, "7d") {
			usageLines = append(usageLines, line)
		}
	}
	if len(usageLines) != 2 {
		t.Fatalf("使用率の行 = %q, want 5h と 7d の 2 行", usageLines)
	}
	for _, line := range usageLines {
		if w := lipgloss.Width(line); w > terminalWidth {
			t.Errorf("使用率の行 %q の幅 = %d, want <= %d（端末幅で切り詰める）", line, w, terminalWidth)
		}
	}
}

func Test_View_WithAndWithoutUsageLine_FillsTerminalHeightExactly(t *testing.T) {
	const terminalHeight = 20
	now := time.Now()
	fiveHourOnly := usage.Limits{FiveHour: &usage.Window{UsedPercent: 42, ResetsAt: now.Add(time.Hour)}}
	bothWindows := modelWithBothWindows(80, now).rateLimits

	for name, limits := range map[string]usage.Limits{"使用率なし": {}, "5h のみ": fiveHourOnly, "5h と 7d": bothWindows} {
		t.Run(name, func(t *testing.T) {
			m := updateModel(t, NewModel(nil), tea.WindowSizeMsg{Width: 80, Height: terminalHeight})
			m = updateModel(t, m, sessionsMsg(source.LoadResult{RateLimits: limits}))

			if got := strings.Count(m.View(), "\n") + 1; got != terminalHeight {
				t.Errorf("View() の行数 = %d, want %d（footer の行数変化に viewport の高さが追従する）", got, terminalHeight)
			}
		})
	}
}
