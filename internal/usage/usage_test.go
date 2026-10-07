package usage

import (
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"
)

var (
	testNow    = time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	testResets = testNow.Add(2 * time.Hour)
)

func fiveHour(usedPercent float64) Limits {
	return Limits{FiveHour: &Window{UsedPercent: usedPercent, ResetsAt: testResets}}
}

func Test_Record_ThenLoad_ReturnsRecordedWindows(t *testing.T) {
	dir := t.TempDir()
	incoming := Limits{
		FiveHour: &Window{UsedPercent: 42, ResetsAt: testResets},
		SevenDay: &Window{UsedPercent: 18, ResetsAt: testResets.Add(72 * time.Hour)},
	}

	Record(dir, "session-a", incoming, testNow)

	got := Load(os.DirFS(dir))
	if got.FiveHour == nil || got.FiveHour.UsedPercent != 42 || !got.FiveHour.ResetsAt.Equal(testResets) {
		t.Errorf("FiveHour = %+v, want UsedPercent 42 / ResetsAt %v", got.FiveHour, testResets)
	}
	if got.SevenDay == nil || got.SevenDay.UsedPercent != 18 {
		t.Errorf("SevenDay = %+v, want UsedPercent 18", got.SevenDay)
	}
}

func Test_Record_UnchangedValuesFromIdleSession_DoNotOverrideNewerObservation(t *testing.T) {
	// 止まっているセッションは、statusLine の再描画のたびに最後に受け取った古い値を渡してくる。
	dir := t.TempDir()
	Record(dir, "idle", fiveHour(30), testNow)
	Record(dir, "active", fiveHour(42), testNow.Add(time.Minute))

	Record(dir, "idle", fiveHour(30), testNow.Add(2*time.Minute))

	if got := Load(os.DirFS(dir)); got.FiveHour.UsedPercent != 42 {
		t.Errorf("UsedPercent = %v, want 42（値の変わらない再送で新しい観測を上書きしない）", got.FiveHour.UsedPercent)
	}
}

func Test_Record_IdleSessionAfterFiveHourReset_DoesNotOverrideNewerSevenDay(t *testing.T) {
	// リセット時刻を過ぎると Claude Code は 5時間枠を外して statusLine を呼び直す。
	// 止まっているセッションの 7日間枠は古いままなので、新しい観測とみなしてはいけない。
	dir := t.TempDir()
	sevenDayResets := testResets.Add(72 * time.Hour)
	Record(dir, "idle", Limits{
		FiveHour: &Window{UsedPercent: 30, ResetsAt: testResets},
		SevenDay: &Window{UsedPercent: 10, ResetsAt: sevenDayResets},
	}, testNow)
	Record(dir, "active", Limits{
		FiveHour: &Window{UsedPercent: 40, ResetsAt: testResets},
		SevenDay: &Window{UsedPercent: 15, ResetsAt: sevenDayResets},
	}, testNow.Add(time.Minute))

	afterReset := testResets.Add(time.Second)
	Record(dir, "idle", Limits{SevenDay: &Window{UsedPercent: 10, ResetsAt: sevenDayResets}}, afterReset)

	if got := Load(os.DirFS(dir)); got.SevenDay.UsedPercent != 15 {
		t.Errorf("SevenDay.UsedPercent = %v, want 15（5時間枠が外れただけの再送で古い 7日間枠を採用しない）", got.SevenDay.UsedPercent)
	}
}

func Test_Record_ChangedLowerValue_TakesOver(t *testing.T) {
	// 同じ枠の中で使用率が下がらないことは保証されていないため、新しく観測した値を優先する。
	dir := t.TempDir()
	Record(dir, "session-a", fiveHour(42), testNow)

	Record(dir, "session-b", fiveHour(30), testNow.Add(time.Minute))

	if got := Load(os.DirFS(dir)); got.FiveHour.UsedPercent != 30 {
		t.Errorf("UsedPercent = %v, want 30（新しく観測した値は小さくても採用する）", got.FiveHour.UsedPercent)
	}
}

func Test_Load_SessionsWithDifferentWindows_PicksLatestPerWindow(t *testing.T) {
	// rate_limits の各枠は個別に欠けることがある。
	dir := t.TempDir()
	Record(dir, "session-a", Limits{
		FiveHour: &Window{UsedPercent: 42, ResetsAt: testResets},
		SevenDay: &Window{UsedPercent: 18, ResetsAt: testResets.Add(72 * time.Hour)},
	}, testNow)

	Record(dir, "session-b", fiveHour(43), testNow.Add(time.Minute))

	got := Load(os.DirFS(dir))
	if got.FiveHour.UsedPercent != 43 {
		t.Errorf("FiveHour.UsedPercent = %v, want 43（session-b の方が新しい）", got.FiveHour.UsedPercent)
	}
	if got.SevenDay == nil || got.SevenDay.UsedPercent != 18 {
		t.Errorf("SevenDay = %+v, want UsedPercent 18（7日間枠は session-a にしか無い）", got.SevenDay)
	}
}

func Test_Record_SessionWithAllWindowsExpired_IsPruned(t *testing.T) {
	dir := t.TempDir()
	Record(dir, "ended", fiveHour(42), testNow)

	afterReset := testResets.Add(time.Minute)
	Record(dir, "active", Limits{FiveHour: &Window{UsedPercent: 5, ResetsAt: afterReset.Add(5 * time.Hour)}}, afterReset)

	if _, ok := loadRecordFile(os.DirFS(dir)).Sessions["ended"]; ok {
		t.Error("全枠がリセット済みのセッションの記録が残っている")
	}
}

func Test_Record_ExpiredIncomingWindow_DoesNotHideNextWindow(t *testing.T) {
	// 時計のずれなどでリセット時刻を過ぎた枠が渡されても、今の観測として記録しない。
	dir := t.TempDir()
	afterReset := testResets.Add(time.Minute)
	nextResets := testResets.Add(5 * time.Hour)
	Record(dir, "active", Limits{FiveHour: &Window{UsedPercent: 5, ResetsAt: nextResets}}, afterReset)

	Record(dir, "stale", Limits{
		FiveHour: &Window{UsedPercent: 90, ResetsAt: testResets},
		SevenDay: &Window{UsedPercent: 18, ResetsAt: testResets.Add(72 * time.Hour)},
	}, afterReset.Add(time.Minute))

	if got := Load(os.DirFS(dir)); !got.FiveHour.ResetsAt.Equal(nextResets) {
		t.Errorf("FiveHour.ResetsAt = %v, want %v（リセット済みの枠で次の枠を隠さない）", got.FiveHour.ResetsAt, nextResets)
	}
}

func Test_Record_SameSessionRenewedWindow_UpdatesResetsAt(t *testing.T) {
	dir := t.TempDir()
	Record(dir, "session-a", fiveHour(42), testNow)

	renewed := testResets.Add(5 * time.Hour)
	afterReset := testResets.Add(time.Minute)
	Record(dir, "session-a", Limits{FiveHour: &Window{UsedPercent: 42, ResetsAt: renewed}}, afterReset)

	if got := Load(os.DirFS(dir)); !got.FiveHour.ResetsAt.Equal(renewed) {
		t.Errorf("FiveHour.ResetsAt = %v, want %v（使用率が同じでも新しい枠として記録する）", got.FiveHour.ResetsAt, renewed)
	}
}

func Test_Record_EmptyLimits_DoesNotCreateFile(t *testing.T) {
	dir := t.TempDir()

	Record(dir, "session-a", Limits{}, testNow)

	if _, err := os.Stat(filepath.Join(dir, fileName)); !os.IsNotExist(err) {
		t.Errorf("rate_limits が無い入力で記録ファイルが作られた: err = %v", err)
	}
}

func Test_Record_Succeeds_LeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()

	Record(dir, "session-a", fiveHour(42), testNow)

	matches, err := filepath.Glob(filepath.Join(dir, "*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) > 0 {
		t.Errorf("一時ファイルが残っている: %v", matches)
	}
}

func Test_Record_RenameFails_LeavesNoTempFile(t *testing.T) {
	dir := t.TempDir()
	// 記録ファイルの位置にディレクトリがあると rename が失敗する。
	if err := os.Mkdir(filepath.Join(dir, fileName), 0o700); err != nil {
		t.Fatal(err)
	}

	Record(dir, "session-a", fiveHour(42), testNow)

	matches, err := filepath.Glob(filepath.Join(dir, "*.tmp"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) > 0 {
		t.Errorf("rename 失敗時に一時ファイルが残っている: %v", matches)
	}
}

func Test_Load_MissingFile_ReturnsEmpty(t *testing.T) {
	got := Load(fstest.MapFS{})
	if !got.IsEmpty() {
		t.Errorf("Load() = %+v, want empty", got)
	}
}

func Test_Load_BrokenJSON_ReturnsEmpty(t *testing.T) {
	fsys := fstest.MapFS{fileName: &fstest.MapFile{Data: []byte("{broken")}}

	got := Load(fsys)
	if !got.IsEmpty() {
		t.Errorf("Load() = %+v, want empty", got)
	}
}

func Test_HasActiveWindow_AllExpired_ReturnsFalse(t *testing.T) {
	l := Limits{
		FiveHour: &Window{UsedPercent: 42, ResetsAt: testNow},
		SevenDay: &Window{UsedPercent: 18, ResetsAt: testNow.Add(-time.Hour)},
	}

	if l.HasActiveWindow(testNow) {
		t.Error("HasActiveWindow() = true, want false（リセット時刻ちょうどは過ぎた扱い）")
	}
}
