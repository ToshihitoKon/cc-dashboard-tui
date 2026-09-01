package session

import "time"

// DisplayState は画面に表示するステータス。
type DisplayState int

const (
	StateUnknown DisplayState = iota
	StateBusy
	StateIdle
	StateActionRequired
)

// actionRequiredTTL は action-required 状態ファイルを信用する期限。
// パーミッション確認が拒否されて tool_result が発行されないケースの
// フォールバック。cmd/cc-dashboard の actionRequiredStateTTL と値を
// 一致させること（変更時は両方直す）。
const actionRequiredTTL = 30 * time.Minute

// StateInput は DeriveState への入力。フィールドを名前付きにすることで、
// time.Time が複数並ぶことによる引数の取り違えを避ける。
type StateInput struct {
	RawStatus string // registry (sessions/<PID>.json) の status。欠損時は空文字
	Now       time.Time

	// ActionRequiredAt は action-required 状態ファイルの記録時刻。
	// hook が未設定、または現在パーミッション確認待ちでなければゼロ値。
	ActionRequiredAt time.Time

	// HasPendingToolUse は本体 jsonl 上に、対応する tool_result を
	// 持たない tool_use が残っているかどうか。action-required の
	// 「解除」判定にのみ使う。true の間は確認待ちが続いていると見なす。
	HasPendingToolUse bool
}

// DeriveState はセッションの表示ステータスを決める。
//
// registry の status が "busy" の場合、それが本当に処理中なのか
// パーミッション確認待ちで止まっているのかを registry だけでは
// 区別できない。hook 由来の action-required 情報（あれば）はこの
// 一点を補うためだけに参照する。
//
// status が "waiting" の場合は registry だけで判断が完結するとみなし、
// hook の有無や TTL を問わず常に action-required とする。実機で観測できた
// のは waitingFor:"permission prompt" が付随する1セッションのみで、waiting
// が他の待ち状態にも使われる可能性は未検証。別の意味で使われていることが
// 分かれば waitingFor の値による分岐に切り替える必要がある。
// idle/unknown も registry だけで判断が完結するため hook を見ない。
//
// 注意: jsonl の mtime は「最後に完了したイベントの時刻」でしかなく、
// 生成中かどうかのハートビートではない。長いツール呼び出し1回で
// 数分間 mtime が動かないことは通常運転でも起きるため、busy を
// 「最近ログが増えていない」だけで stale 扱いにする判定は持たない
// （経過時間が長いことの表現は表示側 = ui パッケージの関心とする）。
func DeriveState(in StateInput) DisplayState {
	switch in.RawStatus {
	case "busy":
		if !in.ActionRequiredAt.IsZero() && in.HasPendingToolUse &&
			in.Now.Sub(in.ActionRequiredAt) <= actionRequiredTTL {
			return StateActionRequired
		}
		return StateBusy
	case "idle":
		return StateIdle
	case "waiting":
		return StateActionRequired
	default:
		// sdk-cli 起動のセッションは status キー自体が無い。
		// 状態が分からないことを Idle に潰さず、そのまま表示する。
		return StateUnknown
	}
}

// Icon はステータスを表す記号。Busy はスピナーが上書きするため空を返す。
func (s DisplayState) Icon() string {
	switch s {
	case StateBusy:
		return "" // 呼び出し側がスピナーのフレームを入れる
	case StateIdle:
		return "●"
	case StateActionRequired:
		return "!"
	default:
		return "◍"
	}
}

// Label はステータスの短い説明（3文字）。
// TUI の限られた列幅向け。ログや --dump 等の詳細表示には String を使う。
func (s DisplayState) Label() string {
	switch s {
	case StateBusy:
		return "run"
	case StateIdle:
		return "idl"
	case StateActionRequired:
		return "act"
	default:
		return "unk"
	}
}

// String はステータスの人間可読な説明（フル表記）。
func (s DisplayState) String() string {
	switch s {
	case StateBusy:
		return "running..."
	case StateIdle:
		return "idle"
	case StateActionRequired:
		return "action required"
	default:
		return "unknown"
	}
}

// NeedsSpinner はこの状態でスピナーを回すべきかを返す。
func (s DisplayState) NeedsSpinner() bool {
	return s == StateBusy
}

// SortOrder はステータス軸でグルーピングしたときのグループの並び順。
// 辞書順ではなく「ユーザーの操作が必要な順」に並べる。
// action-required（操作待ち） → idle（操作可能） → run（操作不要、進行中） → unknown。
func (s DisplayState) SortOrder() int {
	switch s {
	case StateActionRequired:
		return 0
	case StateIdle:
		return 1
	case StateBusy:
		return 2
	default:
		return 3
	}
}
