package session

import (
	"fmt"
	"time"
)

// FormatElapsed は経過時間を "10s" / "5m" / "2h" / "3d" のように可読化する。
// 秒未満の端数は切り捨てる。"5m3s" のような複合表記は視線が滑るので出さない。
func FormatElapsed(d time.Duration) string {
	if d < 0 {
		d = 0 // 時計のずれで負値になっても 0s にクランプする
	}

	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd", int(d.Hours()/24))
	}
}
