package session

import (
	"cmp"
	"slices"
)

// SortSessions はセッション一覧を「ユーザーの操作が必要な順」に in-place で並び替える。
//
// 比較キーは3段:
//  1. State.SortOrder() 昇順（action-required → idle → run → unknown）
//  2. LastActivity 降順（直近に動いたものを上に）
//  3. SessionID 昇順（安定した最終タイブレーク）
//
// 3番目のキーが無いと、LastActivity がゼロ値になりがちな StateUnknown
// セッション同士の並びが fs.ReadDir の列挙順（非決定）に依存してしまい、
// ポーリングのたびに表示順がチラつく。
func SortSessions(sessions []Session) {
	slices.SortFunc(sessions, func(a, b Session) int {
		if c := cmp.Compare(a.State.SortOrder(), b.State.SortOrder()); c != 0 {
			return c
		}
		if c := b.LastActivity.Compare(a.LastActivity); c != 0 { // 降順なので b, a の順で比較
			return c
		}
		return cmp.Compare(a.SessionID, b.SessionID)
	})
}
