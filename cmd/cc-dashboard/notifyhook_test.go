package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func Test_RunNotifyHook_PermissionPrompt_WritesStateFile(t *testing.T) {
	dir := t.TempDir()
	payload := `{"session_id":"abc-123","hook_event_name":"Notification","notification_type":"permission_prompt"}`

	runNotifyHook(strings.NewReader(payload), dir, time.Now())

	if _, err := os.Stat(filepath.Join(dir, "sessions", "abc-123.json")); err != nil {
		t.Errorf("状態ファイルが作られていない: %v", err)
	}
}

func Test_RunNotifyHook_IdlePrompt_DoesNotClearStateFile(t *testing.T) {
	// 解除は本体 jsonl 上の未解決 tool_use の有無で構造的に判定するため、
	// idle_prompt を含むどの hook イベントも状態ファイルを解除しない。
	dir := t.TempDir()
	write := `{"session_id":"abc-123","hook_event_name":"Notification","notification_type":"permission_prompt"}`
	runNotifyHook(strings.NewReader(write), dir, time.Now())

	clear := `{"session_id":"abc-123","hook_event_name":"Notification","notification_type":"idle_prompt"}`
	runNotifyHook(strings.NewReader(clear), dir, time.Now())

	if _, err := os.Stat(filepath.Join(dir, "sessions", "abc-123.json")); err != nil {
		t.Errorf("idle_prompt で状態ファイルが消えるべきではない: %v", err)
	}
}

func Test_RunNotifyHook_FormerClearEvents_AreNoOp(t *testing.T) {
	// 旧バージョンで解除トリガーとして使っていたイベント群。
	// settings.json に残っていても無害（no-op）であることを確認する。
	events := []string{
		"PreToolUse",
		"PostToolUse",
		"UserPromptSubmit",
		"Stop",
		"SessionEnd",
		"SubagentStart",
		"SubagentStop",
		"PreCompact",
		"SessionStart",
	}
	for _, event := range events {
		t.Run(event, func(t *testing.T) {
			dir := t.TempDir()
			write := `{"session_id":"abc-123","hook_event_name":"Notification","notification_type":"permission_prompt"}`
			runNotifyHook(strings.NewReader(write), dir, time.Now())

			payload := `{"session_id":"abc-123","hook_event_name":"` + event + `"}`
			runNotifyHook(strings.NewReader(payload), dir, time.Now())

			if _, err := os.Stat(filepath.Join(dir, "sessions", "abc-123.json")); err != nil {
				t.Errorf("%s で状態ファイルが消えるべきではない: %v", event, err)
			}
		})
	}
}

func Test_RunNotifyHook_MalformedJSON_DoesNotPanic(t *testing.T) {
	dir := t.TempDir()
	runNotifyHook(strings.NewReader(`{not valid json`), dir, time.Now())
	// panic しないことのみを確認する。
}

func Test_RunNotifyHook_MissingSessionID_DoesNothing(t *testing.T) {
	dir := t.TempDir()
	payload := `{"hook_event_name":"Notification","notification_type":"permission_prompt"}`

	runNotifyHook(strings.NewReader(payload), dir, time.Now())

	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Errorf("sessionId が無いのに何か書き込まれている: %v", entries)
	}
}

func Test_RunNotifyHook_EmptyStdin_DoesNotPanic(t *testing.T) {
	dir := t.TempDir()
	runNotifyHook(strings.NewReader(""), dir, time.Now())
}
