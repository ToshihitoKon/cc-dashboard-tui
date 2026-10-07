package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ToshihitoKon/cc-dashboard-tui/internal/usage"
)

// testRecordNow は payload の resets_at（2026-01-01 前後）より前の時刻。
// リセット済みの枠は記録時に掃除されるため、記録時刻をそれより前に置く。
var testRecordNow = time.Date(2025, 12, 31, 12, 0, 0, 0, time.UTC)

func Test_RunRecordUsage_WithRateLimits_RecordsBothWindows(t *testing.T) {
	dir := t.TempDir()
	payload := `{"session_id":"abc-123","model":{"id":"claude-opus"},"rate_limits":{` +
		`"five_hour":{"used_percentage":42,"resets_at":1767276000},` +
		`"seven_day":{"used_percentage":18.5,"resets_at":1767600000}}}`

	runRecordUsage(strings.NewReader(payload), &bytes.Buffer{}, dir, testRecordNow)

	got := usage.Load(os.DirFS(dir))
	if got.FiveHour == nil || got.FiveHour.UsedPercent != 42 || !got.FiveHour.ResetsAt.Equal(time.Unix(1767276000, 0)) {
		t.Errorf("FiveHour = %+v, want UsedPercent 42 / ResetsAt 1767276000", got.FiveHour)
	}
	if got.SevenDay == nil || got.SevenDay.UsedPercent != 18.5 {
		t.Errorf("SevenDay = %+v, want UsedPercent 18.5", got.SevenDay)
	}
}

func Test_RunRecordUsage_AnyInput_PassesStdinThroughToStdout(t *testing.T) {
	// 既存の statusLine コマンドの前段にパイプで挟めるよう、入力を加工せずに流す。
	payload := `{"session_id":"abc-123","rate_limits":{"five_hour":{"used_percentage":42,"resets_at":1767276000}}}`
	var stdout bytes.Buffer

	runRecordUsage(strings.NewReader(payload), &stdout, t.TempDir(), testRecordNow)

	if stdout.String() != payload {
		t.Errorf("stdout = %q, want %q", stdout.String(), payload)
	}
}

func Test_RunRecordUsage_InvalidJSON_PassesThroughWithoutRecording(t *testing.T) {
	dir := t.TempDir()
	payload := "not json"
	var stdout bytes.Buffer

	runRecordUsage(strings.NewReader(payload), &stdout, dir, testRecordNow)

	if stdout.String() != payload {
		t.Errorf("stdout = %q, want %q（パースに失敗しても入力は流す）", stdout.String(), payload)
	}
	if got := usage.Load(os.DirFS(dir)); !got.IsEmpty() {
		t.Errorf("記録された: %+v, want empty", got)
	}
}

func Test_RunRecordUsage_WithoutRateLimits_DoesNotRecord(t *testing.T) {
	// API キー利用時など、サブスクリプションでない場合は rate_limits が入らない。
	dir := t.TempDir()
	payload := `{"session_id":"abc-123","model":{"id":"claude-opus"}}`

	runRecordUsage(strings.NewReader(payload), &bytes.Buffer{}, dir, testRecordNow)

	if got := usage.Load(os.DirFS(dir)); !got.IsEmpty() {
		t.Errorf("記録された: %+v, want empty", got)
	}
}

func Test_RunRecordUsage_WithoutSessionID_DoesNotRecord(t *testing.T) {
	// 空の session_id で記録すると、出どころの違う入力どうしが同じ記録を上書きし合う。
	dir := t.TempDir()
	payload := `{"rate_limits":{"five_hour":{"used_percentage":42,"resets_at":1767276000}}}`

	runRecordUsage(strings.NewReader(payload), &bytes.Buffer{}, dir, testRecordNow)

	if got := usage.Load(os.DirFS(dir)); !got.IsEmpty() {
		t.Errorf("記録された: %+v, want empty", got)
	}
}

func Test_RunRecordUsage_WindowWithMissingValue_IsSkipped(t *testing.T) {
	tests := map[string]string{
		"resets_at が null":    `"five_hour":{"used_percentage":42,"resets_at":null}`,
		"used_percentage が欠損": `"five_hour":{"resets_at":1767276000}`,
	}
	for name, fiveHour := range tests {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			payload := `{"session_id":"abc-123","rate_limits":{` + fiveHour + `,` +
				`"seven_day":{"used_percentage":18,"resets_at":1767600000}}}`

			runRecordUsage(strings.NewReader(payload), &bytes.Buffer{}, dir, testRecordNow)

			got := usage.Load(os.DirFS(dir))
			if got.FiveHour != nil {
				t.Errorf("FiveHour = %+v, want nil（値の欠けた枠は記録しない）", got.FiveHour)
			}
			if got.SevenDay == nil {
				t.Error("SevenDay = nil, want 記録される")
			}
		})
	}
}
