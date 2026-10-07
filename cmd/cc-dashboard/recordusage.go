package main

import (
	"encoding/json"
	"io"
	"time"

	"github.com/ToshihitoKon/cc-dashboard-tui/internal/usage"
)

// statusLinePayload は Claude Code が statusLine コマンドの stdin に渡す JSON の
// うち、使用率の記録に必要な部分のみ。rate_limits は Pro/Max 加入時、
// セッションで最初の API 応答を受けた後にだけ入る。
type statusLinePayload struct {
	SessionID  string `json:"session_id"`
	RateLimits struct {
		FiveHour *rateLimitWindow `json:"five_hour"`
		SevenDay *rateLimitWindow `json:"seven_day"`
	} `json:"rate_limits"`
}

type rateLimitWindow struct {
	UsedPercentage *float64 `json:"used_percentage"`
	ResetsAt       float64  `json:"resets_at"` // Unix 秒
}

// toWindow は値が欠けた枠を nil にする。ゼロ値のまま記録すると、
// 0% や 1970 年にリセット済みの枠として表示されてしまうため。
func (w *rateLimitWindow) toWindow() *usage.Window {
	if w == nil || w.UsedPercentage == nil || w.ResetsAt <= 0 {
		return nil
	}
	return &usage.Window{
		UsedPercent: *w.UsedPercentage,
		ResetsAt:    time.Unix(int64(w.ResetsAt), 0),
	}
}

// runRecordUsage は record-usage サブコマンドの本体。
//
// stdin をそのまま stdout に流すため、既存の statusLine コマンドの前段に
// パイプで挟める。statusLine の表示を壊さないよう、異常系（不正な JSON、
// 書き込み失敗等）では記録だけを諦め、呼び出し元の main は常に exit 0 で終了する。
func runRecordUsage(stdin io.Reader, stdout io.Writer, stateDir string, now time.Time) {
	defer func() { recover() }() //nolint:errcheck // statusLine の異常系は握りつぶして無害化する

	raw, err := io.ReadAll(stdin)
	stdout.Write(raw) //nolint:errcheck // 後段が読まずに終了していても記録は続ける
	if err != nil {
		return
	}

	var payload statusLinePayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return
	}
	if payload.SessionID == "" {
		return
	}
	usage.Record(stateDir, payload.SessionID, usage.Limits{
		FiveHour: payload.RateLimits.FiveHour.toWindow(),
		SevenDay: payload.RateLimits.SevenDay.toWindow(),
	}, now)
}
