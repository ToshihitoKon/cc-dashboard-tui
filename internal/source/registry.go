// Package source は Claude Code のセッション情報ディレクトリから
// session.Session を組み立てるデータ取得層。I/O を持つのはこのパッケージのみ。
package source

import (
	"encoding/json"
	"io/fs"
	"path"
	"strings"
	"time"
)

// registryEntry は sessions/<PID>.json の生の形。
//
// status / updatedAt / statusUpdatedAt は sdk-cli 起動のセッションでは
// キー自体が存在しない（null ではなく欠損）。ポインタ型ではなく
// ゼロ値許容の string で受け、欠損時は空文字として扱う。
type registryEntry struct {
	PID          int    `json:"pid"`
	SessionID    string `json:"sessionId"`
	CWD          string `json:"cwd"`
	StartedAtMs  int64  `json:"startedAt"`
	ProcStart    string `json:"procStart"` // 例: "Tue Aug 25 13:41:53 2026"（UTC）
	Entrypoint   string `json:"entrypoint"`
	RegistryName string `json:"name"`
	Status       string `json:"status"`
}

// procStartLayout は procStart 文字列のフォーマット。time.ANSIC に一致する。
const procStartLayout = time.ANSIC

// readRegistry は sessions/ 配下の *.json（*.key は除く）を全て読む。
//
// 個別ファイルの読み込み・パース失敗はスキップして継続する。1 セッション分の
// 欠損で全体を落とすべきではない。
func readRegistry(fsys fs.FS) ([]registryEntry, []error) {
	entries, err := fs.ReadDir(fsys, "sessions")
	if err != nil {
		if isNotExist(err) {
			return nil, nil // sessions/ が無いのは「実行中セッション 0 件」として正常
		}
		return nil, []error{err}
	}

	var out []registryEntry
	var errs []error
	for _, e := range entries {
		name := e.Name()
		// .key ファイルには peerToken（秘匿情報）が入っているため、
		// 拡張子で列挙段階から除外し読み込みロジックに到達させない。
		if path.Ext(name) != ".json" {
			continue
		}

		raw, err := fs.ReadFile(fsys, path.Join("sessions", name))
		if err != nil {
			if isNotExist(err) {
				continue // セッション終了時の正常な競合。エラーに数えない
			}
			errs = append(errs, namedErr(name, err))
			continue
		}

		var entry registryEntry
		if err := json.Unmarshal(raw, &entry); err != nil {
			errs = append(errs, namedErr(name, err))
			continue
		}
		out = append(out, entry)
	}
	return out, errs
}

func (e registryEntry) startedAt() time.Time {
	return time.UnixMilli(e.StartedAtMs)
}

func (e registryEntry) procStartTime() (time.Time, bool) {
	// procStart は UTC で書かれているが time.ANSIC はタイムゾーン情報を
	// 持たないため、ParseInLocation で明示的に UTC として解釈する。
	t, err := time.ParseInLocation(procStartLayout, strings.TrimSpace(e.ProcStart), time.UTC)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
