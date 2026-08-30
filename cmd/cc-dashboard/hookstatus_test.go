package main

import (
	"encoding/json"
	"os"
	"path/filepath"
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
			"PostToolUse": []map[string]any{
				{"hooks": []map[string]any{
					{"type": "command", "command": "/usr/local/bin/cc-dashboard notify-hook"},
				}},
			},
		},
	}
	path := writeSettings(t, settings)

	got := readRegisteredHookEvents(path)

	if !got["PostToolUse"] {
		t.Errorf("registered = %v, want PostToolUse=true", got)
	}
}

func Test_ReadRegisteredHookEvents_UnrelatedCommand_IsNotDetected(t *testing.T) {
	settings := map[string]any{
		"hooks": map[string]any{
			"PostToolUse": []map[string]any{
				{"hooks": []map[string]any{
					{"type": "command", "command": "~/.config/claude/custom_scripts/hook-logger.sh"},
				}},
			},
		},
	}
	path := writeSettings(t, settings)

	got := readRegisteredHookEvents(path)

	if got["PostToolUse"] {
		t.Error("無関係なコマンドが notify-hook として誤検出されている")
	}
}

func Test_ReadRegisteredHookEvents_PreservesUnrelatedHooksInSameEvent(t *testing.T) {
	// 同じイベントに他ツールの hook と notify-hook が両方登録されているケース。
	// 既存の他ツールの hook を読み飛ばしつつ notify-hook だけ検出できるべき。
	settings := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []map[string]any{
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

	if !got["PreToolUse"] {
		t.Error("他ツールの hook と共存していても notify-hook を検出できるべき")
	}
}

func Test_BuildHookSnippet_ProducesValidJSON(t *testing.T) {
	snippet := buildHookSnippet([]string{"Notification", "Stop"}, "/usr/local/bin/cc-dashboard")

	var parsed map[string][]hookEventGroup
	if err := json.Unmarshal([]byte(snippet), &parsed); err != nil {
		t.Fatalf("生成されたスニペットが不正な JSON: %v\n%s", err, snippet)
	}
	for _, event := range []string{"Notification", "Stop"} {
		if _, ok := parsed[event]; !ok {
			t.Errorf("スニペットに %s が含まれていない: %s", event, snippet)
		}
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
