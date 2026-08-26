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

func Test_RunNotifyHook_IdlePrompt_ClearsStateFile(t *testing.T) {
	dir := t.TempDir()
	write := `{"session_id":"abc-123","hook_event_name":"Notification","notification_type":"permission_prompt"}`
	runNotifyHook(strings.NewReader(write), dir, time.Now())

	clear := `{"session_id":"abc-123","hook_event_name":"Notification","notification_type":"idle_prompt"}`
	runNotifyHook(strings.NewReader(clear), dir, time.Now())

	if _, err := os.Stat(filepath.Join(dir, "sessions", "abc-123.json")); !os.IsNotExist(err) {
		t.Errorf("idle_prompt で状態ファイルが解除されるべき: err=%v", err)
	}
}

func Test_RunNotifyHook_PostToolUse_ClearsStateFile(t *testing.T) {
	// PostToolUse はツールが実行された＝許可されたことを示す最も直接的な
	// 解除信号。これが無いと承認後も action-required 表示が残り続ける。
	dir := t.TempDir()
	write := `{"session_id":"abc-123","hook_event_name":"Notification","notification_type":"permission_prompt"}`
	runNotifyHook(strings.NewReader(write), dir, time.Now())

	clear := `{"session_id":"abc-123","hook_event_name":"PostToolUse"}`
	runNotifyHook(strings.NewReader(clear), dir, time.Now())

	if _, err := os.Stat(filepath.Join(dir, "sessions", "abc-123.json")); !os.IsNotExist(err) {
		t.Errorf("PostToolUse で状態ファイルが解除されるべき: err=%v", err)
	}
}

func Test_RunNotifyHook_UserRejectedViaPreToolUse_ClearsStateFile(t *testing.T) {
	// ユーザーが拒否した場合、PostToolUse は来ずに次の PreToolUse や
	// UserPromptSubmit が来ることがある（約19%のケース）。これらでも
	// 解除できないと action-required 表示が残留する。
	dir := t.TempDir()
	write := `{"session_id":"abc-123","hook_event_name":"Notification","notification_type":"permission_prompt"}`
	runNotifyHook(strings.NewReader(write), dir, time.Now())

	clear := `{"session_id":"abc-123","hook_event_name":"PreToolUse"}`
	runNotifyHook(strings.NewReader(clear), dir, time.Now())

	if _, err := os.Stat(filepath.Join(dir, "sessions", "abc-123.json")); !os.IsNotExist(err) {
		t.Errorf("PreToolUse で状態ファイルが解除されるべき: err=%v", err)
	}
}

func Test_RunNotifyHook_SubagentStop_DoesNotClearStateFile(t *testing.T) {
	// メインセッションがパーミッション確認待ちの間にサブエージェントが
	// 終了しても、待機状態を誤って解除してはいけない。
	dir := t.TempDir()
	write := `{"session_id":"abc-123","hook_event_name":"Notification","notification_type":"permission_prompt"}`
	runNotifyHook(strings.NewReader(write), dir, time.Now())

	unrelated := `{"session_id":"abc-123","hook_event_name":"SubagentStop"}`
	runNotifyHook(strings.NewReader(unrelated), dir, time.Now())

	if _, err := os.Stat(filepath.Join(dir, "sessions", "abc-123.json")); err != nil {
		t.Errorf("SubagentStop で状態ファイルが消えるべきではない: %v", err)
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
