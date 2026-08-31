package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
