package source

import (
	"errors"
	"os"
	"syscall"
)

// ProcessChecker はプロセスの生存を確認する。テストでは fake に差し替える。
type ProcessChecker interface {
	Alive(pid int) bool
}

// osProcessChecker は signal 0 でプロセス生存を確認する本番実装。
type osProcessChecker struct{}

func NewOSProcessChecker() ProcessChecker {
	return osProcessChecker{}
}

func (osProcessChecker) Alive(pid int) bool {
	// os.FindProcess は Unix では失敗しない（struct を返すだけ）。
	// 生死の判定は Signal(0) の戻り値で行う。
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	err = proc.Signal(syscall.Signal(0))
	if err == nil {
		return true // 生存
	}
	if errors.Is(err, os.ErrProcessDone) {
		return false
	}
	if errors.Is(err, syscall.ESRCH) {
		return false
	}
	if errors.Is(err, syscall.EPERM) {
		// 権限エラーは「シグナルを送れないだけで存在はする」ケース
		// （他ユーザーのプロセス等）。存在自体は生存として扱う。
		return true
	}
	return false
}

// 注意: PID 再利用（生存確認した PID が別プロセスに再割り当てされているケース）は
// 標準ライブラリだけでは検出できない。Go にはプロセス開始時刻を取得する
// ポータブルな API が無く、/proc（Linux 限定）や ps の呼び出しが必要になる。
// このアプリのポーリング間隔（1s）に対して PID 再利用が起きる確率は
// 無視できるため、procStart は表示・デバッグ用の参考値に留め、
// 生存判定は signal 0 の結果のみで行う。
