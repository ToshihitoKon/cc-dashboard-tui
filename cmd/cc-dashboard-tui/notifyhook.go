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
	case "PreToolUse", "PostToolUse", "UserPromptSubmit", "Stop", "SessionEnd":
		// PostToolUse はツールが実際に実行された＝許可されたことを示す
		// 最も直接的な解除信号（全体の約8割はこの経路）。
		// それ以外（拒否されてターンが終わる、次の指示が来る等）も含めて
		// 解除対象にしないと、承認・拒否いずれの経路でも action-required
		// 表示が残留してしまう。
		hookstate.Clear(stateDir, payload.SessionID)
	default:
		// SubagentStart/SubagentStop/PreCompact/SessionStart 等は無視する。
		// メインセッションがパーミッション確認待ちの間にサブエージェントが
		// 終了しても、待機状態を誤って解除しないようにするため。
	}
}
