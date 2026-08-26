package session

import "time"

// DisplayState は画面に表示するステータス。
type DisplayState int

const (
	StateUnknown DisplayState = iota
	StateBusy
	StateBusyStale
	StateIdle
)

// staleThreshold は busy 表示を信用しなくなるまでの猶予。
//
// registry の updatedAt はハートビートではなくイベント駆動で更新されるため、
// プロセスが応答を返さないまま status:"busy" が残り続けることがある。
// jsonl の mtime がこの時間を超えて動いていなければ、busy を額面通り扱わない。
const staleThreshold = 60 * time.Second

// DeriveState は registry の status と最終活動時刻からステータスを決める。
//
// registry の status 単独では判定できない。上記 staleThreshold の理由による。
func DeriveState(rawStatus string, lastActivity, now time.Time) DisplayState {
	switch rawStatus {
	case "busy":
		if now.Sub(lastActivity) > staleThreshold {
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
	default:
		return "unknown"
	}
}

// NeedsSpinner はこの状態でスピナーを回すべきかを返す。
func (s DisplayState) NeedsSpinner() bool {
	return s == StateBusy
}

// SortOrder はステータス軸でグルーピングしたときのグループの並び順。
// 辞書順ではなく busy → idle → unknown の意味順に並べたい。
func (s DisplayState) SortOrder() int {
	switch s {
	case StateBusy:
		return 0
	case StateBusyStale:
		return 1
	case StateIdle:
		return 2
	default:
		return 3
	}
}
