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

// actionRequiredStateTTL は internal/session.actionRequiredTTL と値を
// 一致させること（パッケージ依存を避けるため独立して持っている）。
const actionRequiredStateTTL = 30 * time.Minute

// runNotifyHook は notify-hook サブコマンドの本体。
//
// Claude Code 本体の hook 実行を妨げてはならないため、あらゆる異常系
// （不正な JSON、sessionId 欠損、状態ディレクトリへの書き込み失敗等）で
// 何も出力せず、呼び出し元の main は常に exit 0 で終了する。
// panic からの回復もこの関数の責務に含める。
//
// 「解除」はこの hook では扱わない。internal/session.DeriveState が
// 本体 jsonl から構造的に判定するため、ここでは「発生」の記録のみ行う。
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

	if payload.HookEventName == "Notification" && payload.NotificationType == "permission_prompt" {
		hookstate.Write(stateDir, payload.SessionID, now, actionRequiredStateTTL)
	}
	// それ以外の hook_event_name / notification_type は無視する。
}
