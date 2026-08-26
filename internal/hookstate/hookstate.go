// Package hookstate は notify-hook サブコマンドが action-required 状態ファイルを
// 読み書きするためのロジックを提供する。
package hookstate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// sessionIDPattern に一致しない sessionId は使わない。
//
// hook から渡される session_id は Claude Code が発行する UUID の想定だが、
// 万一不正な値（パス区切り文字を含む等）が来てもファイル名に使わないよう
// 許可リスト方式で弾く。ディレクトリトラバーサル対策。
var sessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// record は状態ファイルの中身。internal/source の actionStateFile と対応する。
type record struct {
	SessionID string    `json:"sessionId"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Write はセッションが action-required 状態であることを記録する。
// sessionId が不正、またはディレクトリ作成・書き込みに失敗した場合は
// 何もせず終了する（呼び出し元の notify-hook は常に exit 0 で返す方針のため）。
func Write(stateDir, sessionID string, now time.Time) {
	if stateDir == "" || !sessionIDPattern.MatchString(sessionID) {
		return
	}

	dir := filepath.Join(stateDir, "sessions")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return
	}

	data, err := json.Marshal(record{SessionID: sessionID, UpdatedAt: now})
	if err != nil {
		return
	}

	// 同一ディレクトリ内で一時ファイルに書いてから rename することで、
	// 読み取り側（Source.Load）が書きかけの不完全な JSON を見ないようにする。
	tmp, err := os.CreateTemp(dir, sessionID+".*.tmp")
	if err != nil {
		return
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return
	}
	os.Rename(tmpPath, filepath.Join(dir, sessionID+".json"))
}

// Clear はセッションの action-required 状態を解除する。
// ファイルが無い場合も含め、失敗しても何もしない。
func Clear(stateDir, sessionID string) {
	if stateDir == "" || !sessionIDPattern.MatchString(sessionID) {
		return
	}
	os.Remove(filepath.Join(stateDir, "sessions", sessionID+".json"))
}
