package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ToshihitoKon/cc-dashboard-tui/internal/usage"
)

func Test_ReadRegisteredHookEvents_NoFile_ReturnsEmpty(t *testing.T) {
	got := readRegisteredHookEvents(filepath.Join(t.TempDir(), "settings.json"))
	if len(got) != 0 {
		t.Errorf("registered = %v, want empty", got)
	}
}

func Test_ReadRegisteredHookEvents_RegisteredCommand_IsDetected(t *testing.T) {
	settings := map[string]any{
		"hooks": map[string]any{
			"Notification": []map[string]any{
				{"hooks": []map[string]any{
					{"type": "command", "command": "/usr/local/bin/cc-dashboard notify-hook"},
				}},
			},
		},
	}
	path := writeSettings(t, settings)

	got := readRegisteredHookEvents(path)

	if !got["Notification"] {
		t.Errorf("registered = %v, want Notification=true", got)
	}
}

func Test_ReadRegisteredHookEvents_UnrelatedCommand_IsNotDetected(t *testing.T) {
	settings := map[string]any{
		"hooks": map[string]any{
			"Notification": []map[string]any{
				{"hooks": []map[string]any{
					{"type": "command", "command": "~/.config/claude/custom_scripts/hook-logger.sh"},
				}},
			},
		},
	}
	path := writeSettings(t, settings)

	got := readRegisteredHookEvents(path)

	if got["Notification"] {
		t.Error("無関係なコマンドが notify-hook として誤検出されている")
	}
}

func Test_ReadRegisteredHookEvents_PreservesUnrelatedHooksInSameEvent(t *testing.T) {
	// 同じイベントに他ツールの hook と notify-hook が両方登録されているケース。
	// 既存の他ツールの hook を読み飛ばしつつ notify-hook だけ検出できるべき。
	settings := map[string]any{
		"hooks": map[string]any{
			"Notification": []map[string]any{
				{"hooks": []map[string]any{
					{"type": "command", "command": "~/.config/claude/custom_scripts/hook-logger.sh"},
				}},
				{"hooks": []map[string]any{
					{"type": "command", "command": "cc-dashboard notify-hook"},
				}},
			},
		},
	}
	path := writeSettings(t, settings)

	got := readRegisteredHookEvents(path)

	if !got["Notification"] {
		t.Error("他ツールの hook と共存していても notify-hook を検出できるべき")
	}
}

func Test_BuildHookSnippet_ProducesValidJSON(t *testing.T) {
	snippet := buildHookSnippet([]string{"Notification"}, "/usr/local/bin/cc-dashboard")

	var parsed map[string][]hookEventGroup
	if err := json.Unmarshal([]byte(snippet), &parsed); err != nil {
		t.Fatalf("生成されたスニペットが不正な JSON: %v\n%s", err, snippet)
	}
	if _, ok := parsed["Notification"]; !ok {
		t.Errorf("スニペットに Notification が含まれていない: %s", snippet)
	}
}

func Test_PrintObsoleteHookAdvisory_ReportsRegisteredObsoleteEvents(t *testing.T) {
	registered := map[string]bool{"PostToolUse": true, "Stop": true, "Notification": true}

	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	printObsoleteHookAdvisory(registered)
	w.Close()
	os.Stdout = old
	buf.ReadFrom(r)
	out := buf.String()

	for _, event := range []string{"PostToolUse", "Stop"} {
		if !strings.Contains(out, event) {
			t.Errorf("出力に %s への言及が無い: %s", event, out)
		}
	}
	if strings.Contains(out, "Notification") {
		t.Errorf("現行イベント Notification は advisory に含まれるべきではない: %s", out)
	}
}

func Test_PrintObsoleteHookAdvisory_NoOutputWhenNothingObsolete(t *testing.T) {
	registered := map[string]bool{"Notification": true}

	var buf bytes.Buffer
	old := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w
	printObsoleteHookAdvisory(registered)
	w.Close()
	os.Stdout = old
	buf.ReadFrom(r)

	if buf.Len() != 0 {
		t.Errorf("廃止イベントが無いのに出力がある: %q", buf.String())
	}
}

func Test_ReadStatusLineCommand_NoStatusLine_ReturnsNotConfigured(t *testing.T) {
	path := writeSettings(t, map[string]any{"hooks": map[string]any{}})

	if _, isConfigured := readStatusLineCommand(path); isConfigured {
		t.Error("statusLine キーが無いのに設定済みと判定された")
	}
}

func Test_ReadStatusLineCommand_WithCommand_ReturnsCommand(t *testing.T) {
	path := writeSettings(t, map[string]any{
		"statusLine": map[string]any{"type": "command", "command": "~/scripts/statusline.sh"},
	})

	command, isConfigured := readStatusLineCommand(path)
	if !isConfigured || command != "~/scripts/statusline.sh" {
		t.Errorf("readStatusLineCommand() = (%q, %v), want (%q, true)", command, isConfigured, "~/scripts/statusline.sh")
	}
}

// recordFiveHour はテスト用に、now 時点でリセット前の 5時間枠を記録する。
func recordFiveHour(t *testing.T, stateDir string, now time.Time, resetsAt time.Time) {
	t.Helper()
	usage.Record(stateDir, "abc-123", usage.Limits{
		FiveHour: &usage.Window{UsedPercent: 42, ResetsAt: resetsAt},
	}, now)
}

func Test_PrintUsageRecordStatus_StatusLineCallsRecordUsage_DoesNotSuggest(t *testing.T) {
	path := writeSettings(t, map[string]any{
		"statusLine": map[string]any{"type": "command", "command": "cc-dashboard record-usage | npx ccstatusline"},
	})
	var out bytes.Buffer

	printUsageRecordStatus(&out, path, "cc-dashboard", t.TempDir(), time.Now())

	if !strings.Contains(out.String(), "[ok] statusLine calls record-usage") {
		t.Errorf("record-usage の呼び出しを検出できていない: %s", out.String())
	}
	if strings.Contains(out.String(), "Add the following") || strings.Contains(out.String(), "<existing statusLine command>") {
		t.Errorf("設定済みなのに追記の案内が出ている: %s", out.String())
	}
}

func Test_PrintUsageRecordStatus_NotConfiguredAndNoRecord_SuggestsSnippet(t *testing.T) {
	path := writeSettings(t, map[string]any{})
	var out bytes.Buffer

	printUsageRecordStatus(&out, path, "/usr/local/bin/cc-dashboard", t.TempDir(), time.Now())

	if !strings.Contains(out.String(), "/usr/local/bin/cc-dashboard record-usage > /dev/null") {
		t.Errorf("statusLine の追記例が出ていない: %s", out.String())
	}
}

func Test_PrintUsageRecordStatus_NotConfiguredWithExpiredRecord_SuggestsSnippet(t *testing.T) {
	// record-usage をやめた後などに古い記録だけが残っている場合は、連携できているとみなさない。
	path := writeSettings(t, map[string]any{})
	stateDir := t.TempDir()
	recordedAt := time.Now().Add(-48 * time.Hour)
	recordFiveHour(t, stateDir, recordedAt, recordedAt.Add(time.Hour))
	var out bytes.Buffer

	printUsageRecordStatus(&out, path, "cc-dashboard", stateDir, time.Now())

	if !strings.Contains(out.String(), "[--] no current usage record") {
		t.Errorf("リセット済みの記録を有効扱いしている: %s", out.String())
	}
	if !strings.Contains(out.String(), "cc-dashboard record-usage > /dev/null") {
		t.Errorf("statusLine の追記例が出ていない: %s", out.String())
	}
}

func Test_PrintUsageRecordStatus_ScriptStatusLineWithoutRecord_SuggestsPipe(t *testing.T) {
	path := writeSettings(t, map[string]any{
		"statusLine": map[string]any{"type": "command", "command": "input=$(cat); echo \"$input\" | ~/scripts/statusline.sh"},
	})
	var out bytes.Buffer

	printUsageRecordStatus(&out, path, "cc-dashboard", t.TempDir(), time.Now())

	if !strings.Contains(out.String(), "[--] statusLine does not call record-usage directly") {
		t.Errorf("record-usage を呼んでいないことを示していない: %s", out.String())
	}
	if !strings.Contains(out.String(), "cc-dashboard record-usage | <existing statusLine command>") {
		t.Errorf("既存コマンドの前段に挟む案内が出ていない: %s", out.String())
	}
	// 複合コマンドの前にパイプを足しても入力が届かないため、既存コマンドを例に埋め込まない。
	if strings.Contains(out.String(), "input=$(cat)") {
		t.Errorf("既存コマンドが出力に含まれている: %s", out.String())
	}
}

func Test_PrintUsageRecordStatus_ScriptStatusLineWithRecord_DoesNotSuggest(t *testing.T) {
	// statusLine のスクリプト内で record-usage を呼んでいる構成。コマンド文字列からは
	// 判定できないが、リセット前の値が記録されていれば連携できているとみなす。
	path := writeSettings(t, map[string]any{
		"statusLine": map[string]any{"type": "command", "command": "~/scripts/statusline.sh"},
	})
	stateDir := t.TempDir()
	now := time.Now()
	recordFiveHour(t, stateDir, now, now.Add(time.Hour))
	var out bytes.Buffer

	printUsageRecordStatus(&out, path, "cc-dashboard", stateDir, now)

	if !strings.Contains(out.String(), "[ok] usage recorded (last updated") {
		t.Errorf("記録を検出できていない: %s", out.String())
	}
	if strings.Contains(out.String(), "record-usage |") || strings.Contains(out.String(), "Add the following") {
		t.Errorf("記録できているのに追記の案内が出ている: %s", out.String())
	}
}

func Test_BuildStatusLineSnippet_ProducesValidJSON(t *testing.T) {
	snippet := buildStatusLineSnippet("/usr/local/bin/cc-dashboard")

	var parsed struct {
		StatusLine struct {
			Type    string `json:"type"`
			Command string `json:"command"`
		} `json:"statusLine"`
	}
	if err := json.Unmarshal([]byte(snippet), &parsed); err != nil {
		t.Fatalf("生成されたスニペットが不正な JSON: %v\n%s", err, snippet)
	}
	if parsed.StatusLine.Type != "command" || !isRecordUsageCommand(parsed.StatusLine.Command) {
		t.Errorf("statusLine = %+v, want record-usage を呼ぶ command", parsed.StatusLine)
	}
}

func writeSettings(t *testing.T, settings map[string]any) string {
	t.Helper()
	data, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
