package source

import (
	"bufio"
	"encoding/json"
	"io/fs"
	"path"
	"strings"
	"time"
)

// activityCache は 1 セッション分の jsonl 走査結果のキャッシュ。
// mtime が変化しない限り再読み込みしない。
type activityCache struct {
	mtime   time.Time
	aiTitle string
}

// jsonlLine は ai-title 抽出のためだけに使う最小限のフィールド。
type jsonlLine struct {
	Type    string `json:"type"`
	AITitle string `json:"aiTitle"`
}

// projectDirName は cwd から projects/ 配下のディレクトリ名を作る。
//
// このエンコードは非可逆（'/' '.' '_' が全て '-' に潰れる）なので、
// 逆変換は行わず常に cwd から順方向にエンコードする側で使う。
func projectDirName(cwd string) string {
	replacer := strings.NewReplacer("/", "-", ".", "-", "_", "-")
	return replacer.Replace(cwd)
}

// loadActivity はセッションの最終活動時刻と表示名を取得する。
//
// 最終活動時刻は本体 jsonl と subagents/*.jsonl の mtime の max を取る。
// サブエージェントが動いている間も「活動中」として検出するため。
func (s *Source) loadActivity(cwd, sessionID string) (lastActivity time.Time, aiTitle string) {
	dir := path.Join("projects", projectDirName(cwd))
	jsonlPath := path.Join(dir, sessionID+".jsonl")

	info, err := fs.Stat(s.fsys, jsonlPath)
	if err != nil {
		return time.Time{}, ""
	}
	lastActivity = info.ModTime()

	// subagents/ のディレクトリはセッション毎に <session-uuid>/subagents/*.jsonl。
	if entries, err := fs.ReadDir(s.fsys, path.Join(dir, sessionID, "subagents")); err == nil {
		for _, e := range entries {
			if path.Ext(e.Name()) != ".jsonl" {
				continue
			}
			if fi, err := e.Info(); err == nil && fi.ModTime().After(lastActivity) {
				lastActivity = fi.ModTime()
			}
		}
	}

	cached, ok := s.activityCache[sessionID]
	if ok && cached.mtime.Equal(info.ModTime()) {
		return lastActivity, cached.aiTitle
	}

	aiTitle = extractAITitle(s.fsys, jsonlPath)
	s.activityCache[sessionID] = activityCache{mtime: info.ModTime(), aiTitle: aiTitle}
	return lastActivity, aiTitle
}

// extractAITitle は jsonl を先頭から走査し、最後に出現した ai-title を返す。
//
// ai-title は timestamp を持たずセッション中に繰り返し追記されるため、
// 末尾行が必ずしも最新の ai-title とは限らない。ファイル全体を
// 順方向に読み、最後に見つかったものを採用する。
func extractAITitle(fsys fs.FS, jsonlPath string) string {
	f, err := fsys.Open(jsonlPath)
	if err != nil {
		return ""
	}
	defer f.Close()

	var latest string
	scanner := bufio.NewScanner(f)
	// セッションログの 1 行が bufio のデフォルト上限(64KiB)を超えることがあるため広げる。
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		var line jsonlLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue // 壊れた行はスキップ。ファイル全体を諦めない
		}
		if line.Type == "ai-title" && line.AITitle != "" {
			latest = line.AITitle
		}
	}
	return latest
}
