package source

import (
	"encoding/json"
	"io/fs"
	"path"
	"time"
)

// actionStateFile は hookstate パッケージが書き込む状態ファイルの形。
// notify-hook サブコマンドが書き込む JSON と対応する。
type actionStateFile struct {
	SessionID string    `json:"sessionId"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// loadActionRequiredTimes は state ディレクトリ配下の状態ファイルを全て読み、
// sessionID ごとの記録時刻を返す。
//
// hook 機能は任意（optional）なので、ディレクトリが存在しない・空・
// 個別ファイルが壊れているといった状況は一切エラーにせず、単に
// 該当セッションの情報が無いものとして扱う。
func loadActionRequiredTimes(fsys fs.FS) map[string]time.Time {
	entries, err := fs.ReadDir(fsys, "sessions")
	if err != nil {
		return nil // hook 未設定、または一度も発火していない
	}

	times := make(map[string]time.Time)
	for _, e := range entries {
		if path.Ext(e.Name()) != ".json" {
			continue
		}
		raw, err := fs.ReadFile(fsys, path.Join("sessions", e.Name()))
		if err != nil {
			continue
		}
		var f actionStateFile
		if err := json.Unmarshal(raw, &f); err != nil {
			continue // 壊れたファイルは無視する。書き込み中の競合等で起こりうる
		}
		if f.SessionID == "" {
			continue
		}
		times[f.SessionID] = f.UpdatedAt
	}
	return times
}
