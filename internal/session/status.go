package session

import "time"

// DisplayState は画面に表示するステータス。
type DisplayState int

const (
	StateUnknown DisplayState = iota
	StateBusy
	StateBusyStale
	StateIdle
	StateActionRequired
)

// staleThreshold は busy 表示を信用しなくなるまでの猶予。
//
// registry の updatedAt はハートビートではなくイベント駆動で更新されるため、
// プロセスが応答を返さないまま status:"busy" が残り続けることがある。
// jsonl の mtime がこの時間を超えて動いていなければ、busy を額面通り扱わない。
const staleThreshold = 60 * time.Second

// actionRequiredTTL は action-required 状態ファイルを信用する期限。
//
// hook の解除イベントを取りこぼした場合（プロセスクラッシュ等）、
// 状態ファイルが削除されないまま残り続けることがある。この期限を
// 超えたファイルは無視し、通常の busy/idle 判定にフォールバックする。
const actionRequiredTTL = 30 * time.Minute

// StateInput は DeriveState への入力。フィールドを名前付きにすることで、
// time.Time が複数並ぶことによる引数の取り違えを避ける。
type StateInput struct {
	RawStatus    string    // registry (sessions/<PID>.json) の status。欠損時は空文字
	LastActivity time.Time // jsonl の mtime
	Now          time.Time

	// ActionRequiredAt は action-required 状態ファイルの記録時刻。
	// hook が未設定、または現在パーミッション確認待ちでなければゼロ値。
	ActionRequiredAt time.Time
}

// DeriveState はセッションの表示ステータスを決める。
//
// registry の status が "busy" の場合、それが本当に処理中なのか
// パーミッション確認待ちで止まっているのかを registry だけでは
// 区別できない。hook 由来の action-required 情報（あれば）はこの
// 一点を補うためだけに参照する。idle/unknown は registry だけで
// 判断が完結するため hook を見ない。
func DeriveState(in StateInput) DisplayState {
	switch in.RawStatus {
	case "busy":
		if !in.ActionRequiredAt.IsZero() && in.Now.Sub(in.ActionRequiredAt) <= actionRequiredTTL {
			return StateActionRequired
		}
		if in.Now.Sub(in.LastActivity) > staleThreshold {
			return StateBusyStale
		}
		return StateBusy
	case "idle":
		return StateIdle
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
	case StateBusyStale:
		return "◌"
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
	case StateBusyStale:
		return "stl"
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
	case StateBusyStale:
		return "stalled?"
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
// 辞書順ではなく「対応が必要な順」に並べたい。
func (s DisplayState) SortOrder() int {
	switch s {
	case StateActionRequired:
		return 0
	case StateBusy:
		return 1
	case StateBusyStale:
		return 2
	case StateIdle:
		return 3
	default:
		return 4
	}
}
