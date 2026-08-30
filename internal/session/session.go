// Package session は Claude Code セッションのドメインモデルを提供する。
// このパッケージは外部 I/O を一切持たず、純粋関数のみで構成される。
package session

import (
	"regexp"
	"strings"
	"time"
)

// Session は表示対象となる 1 セッションの情報。
//
// 注意: ユーザーのプロンプト本文（lastPrompt, queue-operation の content 等）に
// 由来するフィールドは意図的に定義していない。フィールドが存在しなければ
// 表示にもログにも漏れない。
type Session struct {
	// sessions/<PID>.json 由来
	PID          int
	SessionID    string
	CWD          string // 実パス。projects/ のディレクトリ名からの逆変換はしない
	StartedAt    time.Time
	ProcStart    time.Time // PID 再利用を検出するための照合用
	RawStatus    string    // "busy" / "idle" / "" (キー欠損)
	Entrypoint   string    // "cli" / "sdk-cli"
	RegistryName string

	// jsonl 由来
	AITitle      string
	Model        string    // 最後に使われたモデル名（/model 等で変わりうる）。取得不可なら空文字
	LastActivity time.Time // max(本体 jsonl, subagents/*.jsonl) の mtime

	// 派生
	GitBranch string
	State     DisplayState
}

// DisplayName は画面に出す名前を返す。
// プロンプト本文由来の文字列は候補に含めない。
func (s Session) DisplayName() string {
	if s.AITitle != "" {
		return s.AITitle
	}
	if s.RegistryName != "" {
		return s.RegistryName
	}
	if len(s.SessionID) >= 8 {
		return s.SessionID[:8]
	}
	return s.SessionID
}

// modelDateSuffix は "claude-sonnet-4-5-20250929" のような日付付き ID の
// 末尾 "-YYYYMMDD" 部分にマッチする。バージョン番号（"-4-5" 等）は
// 桁数が異なるため誤って削らない。
var modelDateSuffix = regexp.MustCompile(`-\d{8}$`)

// DisplayModel はモデル名を列幅に収まる短い形に正規化する。
// 例: "claude-sonnet-4-5-20250929" → "sonnet-4-5"
func (s Session) DisplayModel() string {
	m := strings.TrimPrefix(s.Model, "claude-")
	return modelDateSuffix.ReplaceAllString(m, "")
}
