// Command cc-dashboard-tui は実行中の Claude Code セッション一覧を表示する TUI。
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToshihitoKon/cc-dashboard-tui/internal/source"
	"github.com/ToshihitoKon/cc-dashboard-tui/internal/ui"
	"github.com/ToshihitoKon/cc-dashboard-tui/internal/xdgstate"
)

func main() {
	// notify-hook は Claude Code の hook から高頻度で呼ばれる想定のため、
	// flag パッケージのオーバーヘッドを避けて os.Args を直接見て分岐する。
	if len(os.Args) > 1 && os.Args[1] == "notify-hook" {
		runNotifyHook(os.Stdin, xdgstate.ResolveDir(), time.Now())
		os.Exit(0)
	}
	if len(os.Args) > 1 && os.Args[1] == "hook-status" {
		runHookStatus()
		return
	}

	root := flag.String("root", defaultRoot(), "Claude Code のセッション情報ディレクトリ（~/.claude 相当）")
	dump := flag.Bool("dump", false, "TUI を起動せず、パース結果を1回だけ出力して終了する")
	flag.Parse()

	src := source.New(*root)

	if *dump {
		runDump(src)
		return
	}

	if err := runTUI(src); err != nil {
		fmt.Fprintln(os.Stderr, "cc-dashboard-tui:", err)
		os.Exit(1)
	}
}

func defaultRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ".claude"
	}
	return filepath.Join(home, ".claude")
}

// runDump は --dump 用の経路。開発中、実データの読み取り結果を
// TUI を介さずテキストで確認するための唯一の手段。
func runDump(src *source.Source) {
	result := src.Load()

	fmt.Printf("%d session(s)\n", len(result.Sessions))
	for _, s := range result.Sessions {
		fmt.Printf("- [%s] %s  pid=%d  cwd=%s  branch=%s  started=%s  lastActivity=%s\n",
			s.State.String(), s.DisplayName(), s.PID, s.CWD, s.GitBranch,
			s.StartedAt.Format("15:04:05"), s.LastActivity.Format("15:04:05"))
	}

	if len(result.Errors) > 0 {
		fmt.Printf("%d error(s):\n", len(result.Errors))
		for _, err := range result.Errors {
			fmt.Println(" -", err)
		}
	}
}

func runTUI(src *source.Source) error {
	p := tea.NewProgram(ui.NewModel(src))
	_, err := p.Run()
	return err
}
