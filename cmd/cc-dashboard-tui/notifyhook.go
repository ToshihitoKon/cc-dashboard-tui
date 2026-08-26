package main

import (
	"encoding/json"
	"io"
	"time"

	"github.com/ToshihitoKon/cc-dashboard-tui/internal/hookstate"
)

// maxHookPayloadSize は hook ペイロードの読み取り上限。
// tool_input 等を含むペイロードは大きくなりうるが、無制限に読み込んで
// ブロックすることは避ける。
const maxHookPayloadSize = 1 << 20 // 1MiB

// hookPayload は Claude Code が hook の stdin に渡す JSON の必要部分のみ。
// フィールド名は実際の hook-debug.log での観測に基づくスネークケース。
type hookPayload struct {
	SessionID        string `json:"session_id"`
	HookEventName    string `json:"hook_event_name"`
	NotificationType string `json:"notification_type"`

	// AgentID はサブエージェント実行時にのみ付与される。
	// PreToolUse/PostToolUse はメイン・サブエージェントどちらでも同じ
	// session_id で発火するため、この値の有無でしか区別できない
	// （hook-debug.log で実測済み。Notification/Stop/UserPromptSubmit/
	// SessionEnd はメイン由来のみで、サブエージェント版は
	// SubagentStart/SubagentStop という別イベントに分かれている）。
	AgentID string `json:"agent_id"`
}

// runNotifyHook は notify-hook サブコマンドの本体。
//
// Claude Code 本体の hook 実行を妨げてはならないため、あらゆる異常系
// （不正な JSON、sessionId 欠損、状態ディレクトリへの書き込み失敗等）で
// 何も出力せず、呼び出し元の main は常に exit 0 で終了する。
// panic からの回復もこの関数の責務に含める。
func runNotifyHook(stdin io.Reader, stateDir string, now time.Time) {
	defer func() { recover() }() //nolint:errcheck // hook の異常系は握りつぶして無害化する

	limited := io.LimitReader(stdin, maxHookPayloadSize)
	raw, err := io.ReadAll(limited)
	if err != nil {
		return
	}

	var payload hookPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return
	}
	if payload.SessionID == "" {
		return
	}

	switch payload.HookEventName {
	case "Notification":
		if payload.NotificationType == "permission_prompt" {
			hookstate.Write(stateDir, payload.SessionID, now)
		} else {
			// idle_prompt 等、パーミッション確認以外の通知は
			// 前の action-required 状態を上書きする形で解除する。
			hookstate.Clear(stateDir, payload.SessionID)
		}
	case "PreToolUse", "PostToolUse":
		// この2イベントはサブエージェント実行中も同じ session_id で発火し、
		// agent_id の有無でしかメイン/サブエージェントを区別できない
		// （hook-debug.log で実測済み）。ここを無条件で解除すると、
		// メインがパーミッション確認待ちの間にサブエージェントがツールを
		// 使っただけで誤って解除されてしまう。
		if payload.AgentID == "" {
			hookstate.Clear(stateDir, payload.SessionID)
		}
	case "UserPromptSubmit", "Stop", "SessionEnd":
		// これらはメイン由来のみで発火する（サブエージェント版は
		// SubagentStart/SubagentStop という別イベントに分かれている）。
		// PostToolUse は承認された場合の最も直接的な解除信号（約8割）だが、
		// 拒否されてターンが終わる等の経路もここでカバーする。
		hookstate.Clear(stateDir, payload.SessionID)
	default:
		// SubagentStart/SubagentStop/PreCompact/SessionStart 等は無視する。
		// メインセッションがパーミッション確認待ちの間にサブエージェントが
		// 終了しても、待機状態を誤って解除しないようにするため。
	}
}
