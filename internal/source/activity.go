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
//
// pendingToolUses は bool 判定結果ではなく生データを持つ。
// ActionRequiredAt との突き合わせは毎回 loadActivity 側で行うため。
type activityCache struct {
	mtime           time.Time
	aiTitle         string
	model           string
	pendingToolUses []pendingToolUse
}

// pendingToolUse は本体 jsonl 上で見つかった、対応する tool_result を
// 持たない tool_use 1件分の情報。
type pendingToolUse struct {
	id        string
	timestamp time.Time // 発行元の行の timestamp。無い行ならゼロ値
}

// syntheticModel は実際のモデル推論を経ていない内部的な合成応答に
// 付く model 値。表示対象から除外する。
const syntheticModel = "<synthetic>"

// jsonlLine は ai-title / モデル名 / 未解決 tool_use 検出のために使う
// 最小限のフィールド。
//
// Message.Model は type=="assistant" の行にのみ存在するが、
// type=="attachment" の行にも偶然同名の model キーが別の意味で
// 存在するため、Type を見て assistant 行だけを対象にする。
//
// Timestamp は RawMessage で受ける。time.Time で直接デコードすると
// 想定外の形式が来たときに行全体のデコードが失敗し、ai-title/model の
// 抽出まで巻き込んで落ちてしまうため。
type jsonlLine struct {
	Type        string           `json:"type"`
	AITitle     string           `json:"aiTitle"`
	IsSidechain bool             `json:"isSidechain"`
	Timestamp   json.RawMessage  `json:"timestamp"`
	Message     jsonlLineMessage `json:"message"`
}

// Content は tool_use/tool_result を含む行では配列だが、人間の入力行
// では文字列になる。json.RawMessage で受け、配列としてパースできる
// 場合だけ tool_use/tool_result の集計対象にする。
type jsonlLineMessage struct {
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
}

// jsonlLineContent は assistant 行の tool_use と user 行の tool_result を
// 見分けるための最小限のフィールド。type によって ID / ToolUseID の
// どちらが埋まっているかが変わる。
type jsonlLineContent struct {
	Type      string `json:"type"`
	ID        string `json:"id"`          // type=="tool_use" のとき
	ToolUseID string `json:"tool_use_id"` // type=="tool_result" のとき
}

// activityInfo は loadActivity の走査結果。
type activityInfo struct {
	lastActivity      time.Time // 表示用。subagents 込みの max
	hasPendingToolUse bool      // ActionRequiredAt 以前に発行され、まだ tool_result の無い tool_use があるか
	aiTitle           string
	model             string
}

// projectDirName は cwd から projects/ 配下のディレクトリ名を作る。
//
// このエンコードは非可逆（'/' '.' '_' が全て '-' に潰れる）なので、
// 逆変換は行わず常に cwd から順方向にエンコードする側で使う。
func projectDirName(cwd string) string {
	replacer := strings.NewReplacer("/", "-", ".", "-", "_", "-")
	return replacer.Replace(cwd)
}

// loadActivity はセッションの最終活動時刻・表示名・使用モデル・
// action-required 解除判定用の情報を取得する。
//
// lastActivity（表示用）は subagents 込みの mtime max。
// hasPendingToolUse（解除判定用）は本体 jsonl 単体のみを見る。max を
// 使うと、メインが確認待ちの間にサブエージェントが動いただけで
// 誤って解除されてしまうため、両者は別の値にしている。
func (s *Source) loadActivity(cwd, sessionID string, actionRequiredAt time.Time) activityInfo {
	dir := path.Join("projects", projectDirName(cwd))
	jsonlPath := path.Join(dir, sessionID+".jsonl")

	info, err := fs.Stat(s.fsys, jsonlPath)
	if err != nil {
		return activityInfo{}
	}
	lastActivity := info.ModTime()

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

	var aiTitle, model string
	var pending []pendingToolUse
	if cached, ok := s.activityCache[sessionID]; ok && cached.mtime.Equal(info.ModTime()) {
		aiTitle, model, pending = cached.aiTitle, cached.model, cached.pendingToolUses
	} else {
		aiTitle, model, pending = extractJSONLFields(s.fsys, jsonlPath)
		s.activityCache[sessionID] = activityCache{
			mtime:           info.ModTime(),
			aiTitle:         aiTitle,
			model:           model,
			pendingToolUses: pending,
		}
	}

	return activityInfo{
		lastActivity:      lastActivity,
		hasPendingToolUse: hasPendingToolUseBefore(pending, actionRequiredAt),
		aiTitle:           aiTitle,
		model:             model,
	}
}

// hasPendingToolUseBefore は、actionRequiredAt 以前に発行された tool_use
// のうち、まだ tool_result が無いものが残っているかを返す。
//
// actionRequiredAt より後に発行された tool_use は対象に含めない。含める
// と、一度解除された後に無関係な別のツール呼び出しが走っただけで、
// TTL の間ずっと action-required 表示に戻ってしまう。
func hasPendingToolUseBefore(pending []pendingToolUse, actionRequiredAt time.Time) bool {
	if actionRequiredAt.IsZero() {
		return false
	}
	for _, p := range pending {
		if !p.timestamp.After(actionRequiredAt) {
			return true
		}
	}
	return false
}

// extractJSONLFields は jsonl を先頭から走査し、最後に出現した
// ai-title・使用モデル名・未解決の tool_use 一覧を返す。
//
// ai-title と model はファイル全体を順方向に読み、最後に見つかった
// ものを採用する（/model コマンド等で途中変更されうるため）。
//
// サブエージェントのログは <sessionId>/subagents/ 配下の別ファイルに
// 書かれ、本体 jsonl には混在しない。isSidechain のチェックは万一の
// 混在に備えた保険。
func extractJSONLFields(fsys fs.FS, jsonlPath string) (aiTitle, model string, pending []pendingToolUse) {
	f, err := fsys.Open(jsonlPath)
	if err != nil {
		return "", "", nil
	}
	defer f.Close()

	pendingByID := make(map[string]pendingToolUse)

	scanner := bufio.NewScanner(f)
	// セッションログの 1 行が bufio のデフォルト上限(64KiB)を超えることがあるため広げる。
	// 巨大な tool_result（大きなファイルの Read 結果等）で数 MiB に達することがあるため、
	// 上限自体も余裕を持たせる。
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		var line jsonlLine
		if err := json.Unmarshal(scanner.Bytes(), &line); err != nil {
			continue // 壊れた行はスキップ。ファイル全体を諦めない
		}
		if line.Type == "ai-title" && line.AITitle != "" {
			aiTitle = line.AITitle
		}
		if line.IsSidechain {
			continue
		}
		if line.Type == "assistant" && line.Message.Model != "" && line.Message.Model != syntheticModel {
			model = line.Message.Model
		}

		var contents []jsonlLineContent
		if err := json.Unmarshal(line.Message.Content, &contents); err != nil {
			continue // content が文字列（人間の入力等）の行は tool_use/tool_result を持たない
		}
		var lineTime time.Time
		_ = json.Unmarshal(line.Timestamp, &lineTime) // 失敗時はゼロ値のまま（安全側）
		for _, c := range contents {
			switch c.Type {
			case "tool_use":
				if c.ID != "" {
					pendingByID[c.ID] = pendingToolUse{id: c.ID, timestamp: lineTime}
				}
			case "tool_result":
				delete(pendingByID, c.ToolUseID)
			}
		}
	}

	if scanner.Err() != nil {
		// バッファ上限超過等でスキャンが途中終了した場合、それ以降の
		// tool_result を読めておらず pendingByID は不完全になる。
		// 誤って古い tool_use を pending 扱いにして action-required を
		// 固着させないよう、判定不能として空を返す（安全側）。
		return aiTitle, model, nil
	}

	pending = make([]pendingToolUse, 0, len(pendingByID))
	for _, p := range pendingByID {
		pending = append(pending, p)
	}
	return aiTitle, model, pending
}
