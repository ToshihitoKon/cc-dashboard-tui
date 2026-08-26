package hookstate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func Test_Write_ValidSessionID_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	Write(dir, "abc-123", now)

	path := filepath.Join(dir, "sessions", "abc-123.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("状態ファイルが作られていない: %v", err)
	}

	var got record
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("書き込まれた JSON をパースできない: %v", err)
	}
	if got.SessionID != "abc-123" {
		t.Errorf("SessionID = %q, want %q", got.SessionID, "abc-123")
	}
	if !got.UpdatedAt.Equal(now) {
		t.Errorf("UpdatedAt = %v, want %v", got.UpdatedAt, now)
	}
}

func Test_Write_NoTempFileLeftBehind(t *testing.T) {
	dir := t.TempDir()
	Write(dir, "abc-123", time.Now())

	entries, err := os.ReadDir(filepath.Join(dir, "sessions"))
	if err != nil {
		t.Fatalf("ReadDir failed: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("sessions/ の中身が %d 件, want 1（一時ファイルが残っていないこと）", len(entries))
	}
	if entries[0].Name() != "abc-123.json" {
		t.Errorf("残っていたファイル名 = %q, want %q", entries[0].Name(), "abc-123.json")
	}
}

func Test_Write_PathTraversalSessionID_IsRejected(t *testing.T) {
	dir := t.TempDir()

	Write(dir, "../../etc/passwd", time.Now())

	// sessions/ ディレクトリ自体が作られていないはず（不正な sessionId は
	// MkdirAll より前に弾かれる）。
	if _, err := os.Stat(filepath.Join(dir, "sessions")); err == nil {
		t.Error("不正な sessionId でもディレクトリが作られてしまっている")
	}
	// 万一の書き込み漏れがないか、想定外の場所にファイルができていないことも確認する。
	if _, err := os.Stat(filepath.Join(dir, "..", "..", "etc", "passwd.json")); err == nil {
		t.Error("ディレクトリトラバーサルが成立してしまっている")
	}
}

func Test_Clear_RemovesFile(t *testing.T) {
	dir := t.TempDir()
	Write(dir, "abc-123", time.Now())

	Clear(dir, "abc-123")

	if _, err := os.Stat(filepath.Join(dir, "sessions", "abc-123.json")); !os.IsNotExist(err) {
		t.Errorf("Clear 後もファイルが残っている: err=%v", err)
	}
}

func Test_Clear_NonExistentFile_DoesNotPanic(t *testing.T) {
	dir := t.TempDir()

	// ファイルが存在しない状態での Clear が panic しないことだけを確認する。
	Clear(dir, "never-existed")
}

func Test_Clear_PathTraversalSessionID_IsRejected(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "victim.json")
	if err := os.WriteFile(outside, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}

	Clear(dir, "../"+filepath.Base(filepath.Dir(outside))+"/victim")

	if _, err := os.Stat(outside); err != nil {
		t.Errorf("無関係のファイルが消えてしまっている: %v", err)
	}
}
