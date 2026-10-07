// Package ui は bubbletea を使った TUI 表示を提供する。
//
// 一覧はステータスでグルーピングして表示する（軸の切り替えは無く常時固定）。
// テーブル整形（罫線付きの表組み）はまだ作り込んでいない。
package ui

import (
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/ToshihitoKon/cc-dashboard-tui/internal/session"
	"github.com/ToshihitoKon/cc-dashboard-tui/internal/source"
	"github.com/ToshihitoKon/cc-dashboard-tui/internal/usage"
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
	src        *source.Source
	sessions   []session.Session
	loadErrs   []error
	rateLimits usage.Limits

	viewport viewport.Model
	spinner  spinner.Model
	usageBar progress.Model
	ready    bool // 最初の WindowSizeMsg を受け取るまで viewport は使えない

	// 端末のサイズ。使用率の記録の有無で footer の行数が変わったときに、
	// WindowSizeMsg を待たずに viewport の高さを計算し直すために保持する。
	width  int
	height int
}

// 使用率バーの幅（文字数）の下限と上限。バーは footer の残り幅いっぱいに伸ばすが、
// 狭い端末でも割合を読み取れる幅を残し、広い端末で間延びしないよう上限を設ける。
const (
	usageBarMinWidth = 5
	usageBarMaxWidth = 40
)

// NewModel は Model を作る。
func NewModel(src *source.Source) Model {
	sp := spinner.New()
	sp.Spinner = spinner.Spinner{
		Frames: []string{"⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷"},
		FPS:    spinnerFrameInterval,
	}
	bar := progress.New(progress.WithoutPercentage())
	return Model{src: src, spinner: sp, usageBar: bar}
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
			m.viewport = viewport.New(msg.Width, m.contentHeight(time.Now()))
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = m.contentHeight(time.Now())
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
		m.rateLimits = msg.RateLimits
		if m.ready {
			m.viewport.Height = m.contentHeight(time.Now())
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
//
// 使用率の行は全枠がリセット時刻を過ぎると消えるため、高さの計算と View の
// 判定で時刻がずれうる。ずれても使用率の行が消える向きにしか変わらず、
// 画面からはみ出すことはない。
func (m Model) contentHeight(now time.Time) int {
	chromeLines := 3 // タイトル1行 + 列見出し1行 + フッタ1行
	if m.rateLimits.HasActiveWindow(now) {
		chromeLines += len(m.usageWindowViews()) // フッタの使用率の行（1 枠 1 行）
	}
	h := m.height - chromeLines
	if h < 1 {
		h = 1
	}
	return h
}

func (m Model) View() string {
	if !m.ready {
		return "starting...\n"
	}
	title := lipgloss.NewStyle().Bold(true).Render(
		fmt.Sprintf("cc-dashboard — %d session(s) running", len(m.sessions)))
	header := columnHeader()
	footer := footerStyle.Render(m.footerText())
	if now := time.Now(); m.rateLimits.HasActiveWindow(now) {
		// 折り返すと contentHeight の行数計算がずれるため、端末幅で切り詰める。
		usageLines := lipgloss.NewStyle().MaxWidth(m.width).Render(m.renderUsage(now))
		footer = usageLines + "\n" + footer
	}
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

// usageWindowView は footer に並べる 1 つの枠と、その表示方法。
type usageWindowView struct {
	label       string
	window      usage.Window
	resetLayout string // リセット日時の time.Format レイアウト
}

// usageWindowViews は footer に表示する枠を表示順に返す。記録の無い枠は含めない。
func (m Model) usageWindowViews() []usageWindowView {
	var views []usageWindowView
	if w := m.rateLimits.FiveHour; w != nil {
		views = append(views, usageWindowView{"5h", *w, "15:04"}) // 5時間以内なので日付は省く
	}
	if w := m.rateLimits.SevenDay; w != nil {
		views = append(views, usageWindowView{"7d", *w, "01/02 15:04"}) // 日だけでは何の数字か分かりにくいため月/日で出す
	}
	return views
}

// renderUsage はサブスクリプションの 5時間・7日間の使用率を 1 枠 1 行で描画する。
// 例:
//
//	5h  ███████████████░░░░░░░░░░░░░░░  52%  ↻ 13:00 (10m)
//	7d  ███████████░░░░░░░░░░░░░░░░░░░  36%  ↻ 08/10 11:00 (22h10m)
//
// バーの長さは各行で揃え、バー以外の部分が最も長い行に合わせて端末幅の残りいっぱいに伸ばす。
// バーを下限まで縮めても収まらない狭い端末では、View で行末を切り詰める。
func (m Model) renderUsage(now time.Time) string {
	views := m.usageWindowViews()

	fixedWidth := 0
	for _, v := range views {
		fixedWidth = max(fixedWidth, lipgloss.Width(m.renderUsageWindow(v, 0, now)))
	}
	barWidth := min(max(m.width-fixedWidth, usageBarMinWidth), usageBarMaxWidth)

	lines := make([]string, len(views))
	for i, v := range views {
		lines[i] = m.renderUsageWindow(v, barWidth, now)
	}
	return strings.Join(lines, "\n")
}

// renderUsageWindow は 1 つの枠を "5h  ████░░░░░░  42%  ↻ 13:00 (10m)" の形にする。
//
// リセット時刻を過ぎた枠には、次に statusLine が新しい値を記録するまで
// リセット前の使用率が残っている。リセット後の使用率は claude.ai など他の
// 経路での利用も含めて分からないため、"5h  ░░░░░░░░░░   --  ↻ -" と不明扱いにする。
func (m Model) renderUsageWindow(v usageWindowView, barWidth int, now time.Time) string {
	bar := m.usageBar
	bar.Width = barWidth
	label := footerStyle.Render(v.label) + "  "

	if v.window.IsExpired(now) {
		return label + bar.ViewAs(0) + " " + footerStyle.Render("  --  ↻ -")
	}

	percent := math.Round(v.window.UsedPercent) // 79.6% を「80%」と出しつつ黄色にしないよう、色も丸めた値で決める
	color := usageColor(percent)
	bar.FullColor = color
	percentText := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(fmt.Sprintf("%3.0f%%", percent))
	resetsAt := v.window.ResetsAt.In(now.Location()).Format(v.resetLayout)
	reset := fmt.Sprintf("↻ %s (%s)", resetsAt, formatUntilReset(v.window.ResetsAt.Sub(now)))

	return label + bar.ViewAs(v.window.UsedPercent/100) + " " + percentText + "  " + footerStyle.Render(reset)
}

// 使用率の色を切り替える閾値。半分以上で注意（黄）、8 割以上で警告（赤）とし、
// セッション一覧の配色（長時間 run の黄、action-required の赤）に揃える。
const (
	usageWarnPercent   = 50
	usageDangerPercent = 80
)

func usageColor(usedPercent float64) string {
	switch {
	case usedPercent >= usageDangerPercent:
		return "203"
	case usedPercent >= usageWarnPercent:
		return "220"
	default:
		return "42"
	}
}

// formatUntilReset はリセットまでの残り時間を "45m" / "2h13m" / "3d04h" の形にする。
// session.FormatElapsed と違い 2 単位で出すのは、"3d" のような 1 単位だと
// 最大で 1 単位分（7日間枠なら 1 日）の誤差が出るため。
func formatUntilReset(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "<1m"
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh%02dm", int(d.Hours()), int(d.Minutes())%60)
	default:
		return fmt.Sprintf("%dd%02dh", int(d.Hours())/24, int(d.Hours())%24)
	}
}

// render はセッション一覧を「状態ごとのグループ見出し + 行」の形で描画する。
//
// m.sessions は source.Load() 側で session.SortSessions 済み（状態優先度が
// 主キー）なので、同じ状態の行は必ず連続している。見出しは「直前の要素と
// State が変わった瞬間」に挿入するだけでよく、状態ごとに事前グループ化した
// 中間データを作る必要はない。
func (m Model) render() string {
	if len(m.sessions) == 0 {
		return "No running Claude Code sessions."
	}

	now := time.Now()
	var b strings.Builder
	for i, s := range m.sessions {
		if i == 0 || s.State != m.sessions[i-1].State {
			if i > 0 {
				b.WriteString("\n")
			}
			b.WriteString(groupHeading(s.State, countInGroup(m.sessions, i)))
			b.WriteString("\n")
		}
		b.WriteString(m.renderSession(s, now))
		if i < len(m.sessions)-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// countInGroup は m.sessions[start] から始まる同一 State の連続区間の長さを返す。
func countInGroup(sessions []session.Session, start int) int {
	state := sessions[start].State
	n := 0
	for i := start; i < len(sessions) && sessions[i].State == state; i++ {
		n++
	}
	return n
}

var groupHeadingStyle = lipgloss.NewStyle().Faint(true)

// groupHeading はグループ見出し行の文字列。列幅（statusColWidth 等）とは
// 揃えない。テーブルの1行ではなく区切りとして表示するため。
func groupHeading(state session.DisplayState, count int) string {
	return groupHeadingStyle.Render(fmt.Sprintf("── %s (%d) ──", state.String(), count))
}

// カラム幅。日本語（表示幅2）が混ざっても揃うよう、パディングは
// fmt の %-Ns（rune 数基準）ではなく lipgloss.Style.Width（表示幅基準）で行う。
const (
	statusColWidth  = 13 // 例: "● run (999s)"
	titleColWidth   = 28
	modelColWidth   = 14 // 表示は Session.DisplayModel() で正規化済み（例: "sonnet-4-5"）
	startedColWidth = 8  // 例: "2h ago"
)

var (
	statusColStyle  = lipgloss.NewStyle().Width(statusColWidth)
	titleColStyle   = lipgloss.NewStyle().Width(titleColWidth)
	modelColStyle   = lipgloss.NewStyle().Width(modelColWidth)
	startedColStyle = lipgloss.NewStyle().Width(startedColWidth).Align(lipgloss.Right)
)

// columnHeader は render() の列（status, title, model, started）に
// 対応する見出し行。viewport の外（スクロールされない領域）に置く。
func columnHeader() string {
	cells := []string{
		statusColStyle.Render("status"),
		titleColStyle.Render("title"),
		modelColStyle.Render("model"),
		startedColStyle.Render("started"),
	}
	return footerStyle.Render(strings.Join(cells, "  "))
}

// longRunThreshold を超えて busy が続いている場合、行の色を変えて目立たせる。
//
// jsonl の mtime は完了したイベントの時刻でしかなく、長いツール呼び出し
// 1回で数分間動かないことは通常運転でも起きる（stall 判定を廃止した理由）。
// そのため「異常」の断定はできないが、単なる run の緑色のまま埋もれさせず
// 「一応目に留めておく」程度の注意喚起として長さだけ別色にする。
const longRunThreshold = 5 * time.Minute

// isLongRun は busy 状態が longRunThreshold を超えて続いているかを返す。
// busy 以外の状態には注意喚起の意味がないため常に false。
//
// LastActivity がゼロ値（起動直後で jsonl がまだ存在せず取得に失敗した場合）は
// now との差分が time.Duration の最大値にサチュレートされ、長時間run扱いに
// 誤判定されてしまうため除外する。
func isLongRun(s session.Session, now time.Time) bool {
	return s.State == session.StateBusy && !s.LastActivity.IsZero() && now.Sub(s.LastActivity) > longRunThreshold
}

// elapsedLabel は LastActivity からの経過時間を表示用文字列にする。
// ゼロ値（起動直後で jsonl の取得に失敗した場合）は経過時間が意味を持たないため、
// modelOrPlaceholder と同じ「不明はハイフン」の表示規約に合わせる。
func elapsedLabel(lastActivity, now time.Time) string {
	if lastActivity.IsZero() {
		return "-"
	}
	return session.FormatElapsed(now.Sub(lastActivity))
}

func (m Model) renderSession(s session.Session, now time.Time) string {
	icon := s.State.Icon()
	if s.State.NeedsSpinner() {
		icon = m.spinner.View()
	}

	// icon はスピナー（ANSIエスケープを含みうる）なので truncate の対象から外し、
	// テキスト部分だけを切り詰める。
	label := truncate(fmt.Sprintf("%s (%s)", s.State.Label(), elapsedLabel(s.LastActivity, now)), statusColWidth-2)
	statusCell := statusColStyle.Render(icon + " " + label)
	titleCell := titleColStyle.Render(truncate(s.DisplayName(), titleColWidth))
	modelCell := modelColStyle.Render(truncate(modelOrPlaceholder(s.DisplayModel()), modelColWidth))
	startedCell := startedColStyle.Render(truncate(session.FormatElapsed(now.Sub(s.StartedAt))+" ago", startedColWidth))

	line := strings.Join([]string{statusCell, titleCell, modelCell, startedCell}, "  ")
	return statusStyle(s.State, isLongRun(s, now)).Render(line)
}

// 現状はダーク背景のターミナル専用に固定色を使う。
// lipgloss.AdaptiveColor は背景色の自動判定に依存するため環境によって
// 誤判定されることがあり、ライトテーマ対応は別途検討する。
func statusStyle(state session.DisplayState, isLongRun bool) lipgloss.Style {
	switch state {
	case session.StateActionRequired:
		// 対応が必要な行は他のどの状態よりも目立たせる（赤・太字）。
		return lipgloss.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	case session.StateBusy:
		if isLongRun {
			return lipgloss.NewStyle().Foreground(lipgloss.Color("220")) // 明るい黄。長時間 busy への注意喚起
		}
		return lipgloss.NewStyle().Foreground(lipgloss.Color("42")) // 明るい緑
	case session.StateIdle:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("252")) // 明るいグレー（本文相当の視認性）
	default:
		// unknown はグレーの明度調整では idle と見分けが付かなくなるため、
		// 明度ではなく色相を変える（暗めのシアン）。「idle より薄い」ではなく
		// 「状態が分からない」という別種の意味を持たせる。
		return lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
	}
}

// modelOrPlaceholder はモデル名が取得できない場合（sdk-cli 起動セッション等）に
// 空白の列ではなく分かりやすいプレースホルダーを出す。
func modelOrPlaceholder(model string) string {
	if model == "" {
		return "-"
	}
	return model
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
