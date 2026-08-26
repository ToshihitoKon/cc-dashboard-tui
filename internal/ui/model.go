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
// 経過時間表示の粒度もこれに従うため、頻繁すぎる更新は不要という
// ユーザー判断により 10s にしている。
const pollInterval = 10 * time.Second

// spinnerFrameInterval はスピナーの 1 フレームあたりの時間。
// 3fps 相当（Braille 8 フレームで 1 周 約2.7秒）。
const spinnerFrameInterval = time.Second / 3

type sessionsMsg source.LoadResult

// Model は TUI 全体の状態。
type Model struct {
	src      *source.Source
	sessions []session.Session
	loadErrs []error

	viewport viewport.Model
	spinner  spinner.Model
	ready    bool // 最初の WindowSizeMsg を受け取るまで viewport は使えない
}

// NewModel は Model を作る。
func NewModel(src *source.Source) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Spinner{
		Frames: []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"},
		FPS:    spinnerFrameInterval,
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
		// tick チェーンは busy の有無にかかわらず必ず継続させる。
		// ここで cmd を返さず止めると、後で busy セッションが現れたときに
		// チェーンを再起動する経路が必要になり、tick が重複しやすくなる。
		m.spinner, cmd = m.spinner.Update(msg)
		if m.ready && m.hasSpinningSession() {
			// 経過時間ラベルは pollInterval 側の更新で十分なので、
			// ここでの再描画は busy セッションのアニメーションのためだけに行う。
			m.viewport.SetContent(m.render())
		}
		return m, cmd
	}
	return m, nil
}

// hasSpinningSession は現在 busy 表示（スピナー使用）のセッションが
// 1 件でもあるかを返す。
func (m Model) hasSpinningSession() bool {
	for _, s := range m.sessions {
		if s.State.NeedsSpinner() {
			return true
		}
	}
	return false
}

// contentHeight はヘッダ・フッタ分を引いた viewport の高さ。
func contentHeight(totalHeight int) int {
	const chromeLines = 3 // タイトル1行 + 列見出し1行 + フッタ1行
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
	title := lipgloss.NewStyle().Bold(true).Render(
		fmt.Sprintf("cc-dashboard-tui — %d session(s) running", len(m.sessions)))
	header := columnHeader()
	footer := footerStyle.Render(m.footerText())
	return title + "\n" + header + "\n" + m.viewport.View() + "\n" + footer
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

// カラム幅。日本語（表示幅2）が混ざっても揃うよう、パディングは
// fmt の %-Ns（rune 数基準）ではなく lipgloss.Style.Width（表示幅基準）で行う。
const (
	statusColWidth  = 13 // 例: "● run (999s)"
	titleColWidth   = 28
	startedColWidth = 8 // 例: "2h ago"
)

var (
	statusColStyle  = lipgloss.NewStyle().Width(statusColWidth)
	titleColStyle   = lipgloss.NewStyle().Width(titleColWidth)
	startedColStyle = lipgloss.NewStyle().Width(startedColWidth).Align(lipgloss.Right)
)

// columnHeader は render() の列（status, title, started）に
// 対応する見出し行。viewport の外（スクロールされない領域）に置く。
func columnHeader() string {
	cells := []string{
		statusColStyle.Render("status"),
		titleColStyle.Render("title"),
		startedColStyle.Render("started"),
	}
	return footerStyle.Render(strings.Join(cells, "  "))
}

func (m Model) renderSession(s session.Session, now time.Time) string {
	icon := s.State.Icon()
	if s.State.NeedsSpinner() {
		icon = m.spinner.View()
	}

	elapsed := session.FormatElapsed(now.Sub(s.LastActivity))
	statusCell := statusColStyle.Render(fmt.Sprintf("%s %s (%s)", icon, s.State.Label(), elapsed))
	titleCell := titleColStyle.Render(truncate(s.DisplayName(), titleColWidth))
	startedCell := startedColStyle.Render(session.FormatElapsed(now.Sub(s.StartedAt)) + " ago")

	line := strings.Join([]string{statusCell, titleCell, startedCell}, "  ")
	return statusStyle(s.State).Render(line)
}

// 現状はダーク背景のターミナル専用に固定色を使う。
// lipgloss.AdaptiveColor は背景色の自動判定に依存するため環境によって
// 誤判定されることがあり、ライトテーマ対応は別途検討する。
func statusStyle(state session.DisplayState) lipgloss.Style {
	switch state {
	case session.StateBusy:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("42")) // 明るい緑
	case session.StateBusyStale:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("220")) // 明るい黄
	case session.StateIdle:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("252")) // 明るいグレー（本文相当の視認性）
	default:
		// unknown はグレーの明度調整では idle と見分けが付かなくなるため、
		// 明度ではなく色相を変える（暗めのシアン）。「idle より薄い」ではなく
		// 「状態が分からない」という別種の意味を持たせる。
		return lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
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
