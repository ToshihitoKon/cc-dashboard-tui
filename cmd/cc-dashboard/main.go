// Command cc-dashboard は実行中の Claude Code セッション一覧を表示する TUI。
package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ToshihitoKon/cc-dashboard-tui/internal/source"
	"github.com/ToshihitoKon/cc-dashboard-tui/internal/ui"
	"github.com/ToshihitoKon/cc-dashboard-tui/internal/usage"
	"github.com/ToshihitoKon/cc-dashboard-tui/internal/xdgstate"
)

// version は goreleaser がビルド時に -ldflags "-X main.version=..." で埋め込む。
// go build で直接ビルドした場合は "dev" のまま。
var version = "dev"

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
	// record-usage も statusLine の更新のたびに呼ばれるため、notify-hook と同様に扱う。
	if len(os.Args) > 1 && os.Args[1] == "record-usage" {
		// 後段のコマンドが stdin を読まずに終了すると stdout への書き込みで
		// SIGPIPE を受け、Go は記録の前にプロセスを終了させてしまう。
		signal.Ignore(syscall.SIGPIPE)
		runRecordUsage(os.Stdin, os.Stdout, xdgstate.ResolveDir(), time.Now())
		os.Exit(0)
	}

	flag.Usage = printUsage
	root := flag.String("root", defaultRoot(), "Claude Code session directory (defaults to ~/.claude)")
	dump := flag.Bool("dump", false, "print parsed sessions once and exit, without starting the TUI")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	src := source.New(*root)

	if *dump {
		runDump(src)
		return
	}

	if err := runTUI(src); err != nil {
		fmt.Fprintln(os.Stderr, "cc-dashboard:", err)
		os.Exit(1)
	}
}

// printUsage は -h/--help で表示される内容。
// サブコマンド（notify-hook/hook-status/record-usage）は flag パッケージの管理外で
// os.Args を直接見て分岐しているため、flag.PrintDefaults だけでは
// 存在が案内されない。ここに明記して案内漏れを防ぐ。
func printUsage() {
	fmt.Fprintln(os.Stderr, "usage: cc-dashboard [flags]")
	fmt.Fprintln(os.Stderr, "\nflags:")
	flag.PrintDefaults()
	fmt.Fprintln(os.Stderr, "\nsubcommands:")
	fmt.Fprintln(os.Stderr, "  hook-status    check whether the action-required hook and record-usage are configured")
	fmt.Fprintln(os.Stderr, "  notify-hook    entry point called by Claude Code's hooks (no need to run manually)")
	fmt.Fprintln(os.Stderr, "  record-usage   record subscription usage from statusLine input (passes stdin through to stdout)")
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
		fmt.Printf("- [%s] %s  model=%s  pid=%d  cwd=%s  branch=%s  started=%s  lastActivity=%s\n",
			s.State.String(), s.DisplayName(), s.DisplayModel(), s.PID, s.CWD, s.GitBranch,
			s.StartedAt.Format("15:04:05"), s.LastActivity.Format("15:04:05"))
	}

	printRateLimits(result.RateLimits, time.Now())

	if len(result.Errors) > 0 {
		fmt.Printf("%d error(s):\n", len(result.Errors))
		for _, err := range result.Errors {
			fmt.Println(" -", err)
		}
	}
}

func printRateLimits(limits usage.Limits, now time.Time) {
	if limits.IsEmpty() {
		fmt.Println("rate limits: no record (configure record-usage in statusLine)")
		return
	}
	fmt.Println("rate limits:")
	for _, w := range []struct {
		label  string
		window *usage.Window
	}{
		{"5h", limits.FiveHour},
		{"7d", limits.SevenDay},
	} {
		if w.window == nil {
			continue
		}
		expired := ""
		if w.window.IsExpired(now) {
			expired = " (expired)"
		}
		fmt.Printf("- %s: %.0f%% used, resets at %s%s\n",
			w.label, w.window.UsedPercent, w.window.ResetsAt.In(now.Location()).Format("01/02 15:04"), expired)
	}
}

func runTUI(src *source.Source) error {
	p := tea.NewProgram(ui.NewModel(src))
	_, err := p.Run()
	return err
}
