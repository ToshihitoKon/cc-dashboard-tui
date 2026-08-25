// Package ui は bubbletea を使った TUI 表示を提供する。
//
// 現段階では「コアロジックが正しく動いていることの確認」を優先し、
// テーブル整形やグルーピングの作り込みは行わない。一覧表示とスクロール、
// ステータスの表示・更新のみを実装する。
package ui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"github.com/ToshihitoKon/cc-dashboard-tui/internal/session"
	"github.com/ToshihitoKon/cc-dashboard-tui/internal/source"
)

// pollInterval はセッション一覧を再取得する間隔。
// ファイル走査は mtime キャッシュにより軽量なので 1s でも問題ない。
const pollInterval = 1 * time.Second

// spinnerInterval はスピナーのフレーム更新間隔。
// ポーリングと同一 tick にすると busy 表示がカクつくため別 tick にする。
const spinnerInterval = 100 * time.Millisecond

type sessionsMsg source.LoadResult

type spinnerTickMsg time.Time

// Model は TUI 全体の状態。
type Model struct {
	src      *source.Source
	sessions []session.Session
	loadErrs []error

	viewport viewport.Model
	spinner  spinner.Model
	ready    bool // 最初の WindowSizeMsg を受け取るまで viewport は使えない

	width, height int
}

// NewModel は Model を作る。
func NewModel(src *source.Source) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Spinner{
		Frames: []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"},
		FPS:    spinnerInterval,
	}
	return Model{src: src, spinner: sp}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(pollCmd(m.src), m.spinner.Tick)
}

func pollCmd(src *source.Source) tea.Cmd {
	return func() tea.Msg {
		return sessionsMsg(src.Load())
	}
}

// scheduleNextPoll は次のポーリングを予約する。
//
// 固定スケジュールの tea.Tick を張るのではなく、直前のポーリング完了を
// 受けてから次を発行する自己クロック方式にしている。走査が pollInterval
// より長引いた場合でも、ポーリングが重複して積み上がることがない。
func scheduleNextPoll(src *source.Source) tea.Cmd {
	return tea.Tick(pollInterval, func(time.Time) tea.Msg {
		return sessionsMsg(src.Load())
	})
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if !m.ready {
			m.viewport = viewport.New(msg.Width, contentHeight(msg.Height))
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = contentHeight(msg.Height)
		}
		m.viewport.SetContent(m.render())
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case sessionsMsg:
		m.sessions = msg.Sessions
		m.loadErrs = msg.Errors
		if m.ready {
			m.viewport.SetContent(m.render())
		}
		return m, scheduleNextPoll(m.src)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		if m.ready {
			// スピナーのフレームが進むたびに再描画する。
			// busy なセッションが 0 件でも spinner.Tick 自体は動き続けるが、
			// 表示に使われないだけなので実害はない。
			m.viewport.SetContent(m.render())
		}
		return m, cmd
	}
	return m, nil
}

// contentHeight はヘッダ・フッタ分を引いた viewport の高さ。
func contentHeight(totalHeight int) int {
	const chromeLines = 2 // ヘッダ1行 + フッタ1行
	h := totalHeight - chromeLines
	if h < 1 {
		h = 1
	}
	return h
}

func (m Model) View() string {
	if !m.ready {
		return "起動中...\n"
	}
	header := lipgloss.NewStyle().Bold(true).Render(
		fmt.Sprintf("cc-dashboard-tui — %d session(s) running", len(m.sessions)))
	footer := footerStyle.Render(m.footerText())
	return header + "\n" + m.viewport.View() + "\n" + footer
}

var footerStyle = lipgloss.NewStyle().Faint(true)

func (m Model) footerText() string {
	text := "↑/↓: scroll   q: quit"
	if len(m.loadErrs) > 0 {
		text += fmt.Sprintf("   (%d unreadable)", len(m.loadErrs))
	}
	return text
}

func (m Model) render() string {
	if len(m.sessions) == 0 {
		return "実行中の Claude Code セッションはありません。"
	}

	now := time.Now()
	var b strings.Builder
	for i, s := range m.sessions {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(m.renderSession(s, now))
	}
	return b.String()
}

func (m Model) renderSession(s session.Session, now time.Time) string {
	icon := s.State.Icon()
	if s.State.NeedsSpinner() {
		icon = m.spinner.View()
	}

	elapsed := session.FormatElapsed(now.Sub(s.LastActivity))
	statusText := fmt.Sprintf("%s %s (%s)", icon, s.State.Label(), elapsed)

	line := fmt.Sprintf("%-28s  %-16s  started %s",
		truncate(s.DisplayName(), 28), statusText, s.StartedAt.Format("15:04:05"))
	return statusStyle(s.State).Render(line)
}

func statusStyle(state session.DisplayState) lipgloss.Style {
	switch state {
	case session.StateBusy:
		return lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "22", Dark: "42"})
	case session.StateBusyStale:
		return lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "130", Dark: "220"})
	case session.StateIdle:
		return lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "244", Dark: "244"})
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "240", Dark: "238"})
	}
}

// truncate は表示幅（マルチバイト考慮）で切り詰める。len は使わない。
func truncate(s string, width int) string {
	if lipgloss.Width(s) <= width {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 && lipgloss.Width(string(runes)) > width-1 {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}
