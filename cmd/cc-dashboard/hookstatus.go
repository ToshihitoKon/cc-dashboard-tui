package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ToshihitoKon/cc-dashboard-tui/internal/session"
	"github.com/ToshihitoKon/cc-dashboard-tui/internal/usage"
	"github.com/ToshihitoKon/cc-dashboard-tui/internal/xdgstate"
)

// requiredHookEvents は action-required 検出に必要な hook イベント。
//
// 「発生」は Notification で検出する。「解除」は本体 jsonl 上の
// 未解決 tool_use の有無で構造的に判定するため、hook イベントは不要。
var requiredHookEvents = []string{
	"Notification",
}

// obsoleteHookEvents は過去のバージョンで登録を案内していたが、
// 現在は notify-hook が参照しないイベント。settings.json に残っていても
// 無害（notify-hook が no-op で無視する）だが、無駄なプロセス起動を
// 避けるため削除を促す。
var obsoleteHookEvents = []string{
	"PreToolUse",
	"PostToolUse",
	"UserPromptSubmit",
	"Stop",
	"SessionEnd",
}

// settingsHooks は ~/.claude/settings.json の hooks キーの必要部分のみ。
// 未知のキーは json.Unmarshal が無視するので、他の設定を壊す心配はない。
type settingsHooks struct {
	Hooks map[string][]hookEventGroup `json:"hooks"`
}

type hookEventGroup struct {
	Matcher string      `json:"matcher,omitempty"`
	Hooks   []hookEntry `json:"hooks"`
}

type hookEntry struct {
	Type    string `json:"type"`
	Command string `json:"command"`
}

// runHookStatus は hook-status サブコマンドの本体。
//
// settings.json は読み取り専用で確認する。書き込みは一切行わず、
// 不足しているイベントがあれば追記すべき JSON 片を提示するだけに留める。
func runHookStatus() {
	settingsPath := filepath.Join(defaultRoot(), "settings.json")
	exe, err := os.Executable()
	if err != nil {
		exe = "cc-dashboard" // フォールバック。PATH が通っていない環境では動かない可能性がある旨は出力で補う
	}

	registered := readRegisteredHookEvents(settingsPath)

	fmt.Println("action-required hook status:")
	var missing []string
	for _, event := range requiredHookEvents {
		if registered[event] {
			fmt.Printf("  [ok] %s\n", event)
		} else {
			fmt.Printf("  [--] %s (not configured)\n", event)
			missing = append(missing, event)
		}
	}

	if len(missing) == 0 {
		fmt.Println("\nAll hooks are configured.")
	} else {
		fmt.Printf("\nAdd the following to the hooks in %s (using %s):\n", settingsPath, exe)
		fmt.Println(buildHookSnippet(missing, exe))
	}

	printObsoleteHookAdvisory(registered)

	printUsageRecordStatus(os.Stdout, settingsPath, exe, xdgstate.ResolveDir(), time.Now())
}

// settingsStatusLine は ~/.claude/settings.json の statusLine キーの必要部分のみ。
type settingsStatusLine struct {
	StatusLine *struct {
		Command string `json:"command"`
	} `json:"statusLine"`
}

// printUsageRecordStatus は footer の使用率表示に必要な statusLine 連携の状況を出す。
//
// statusLine のスクリプト内やプロジェクト側の settings.json で record-usage を呼ぶ
// 構成もあり、ユーザー設定のコマンド文字列だけでは判定しきれない。そのため
// リセット前の値が記録されていれば連携できているとみなし、追記の案内は出さない。
func printUsageRecordStatus(w io.Writer, settingsPath, exe, stateDir string, now time.Time) {
	fmt.Fprintln(w, "\nusage record (record-usage) status:")

	command, isConfigured := readStatusLineCommand(settingsPath)
	callsRecordUsage := isRecordUsageCommand(command)
	switch {
	case callsRecordUsage:
		fmt.Fprintln(w, "  [ok] statusLine calls record-usage")
	case isConfigured:
		fmt.Fprintln(w, "  [--] statusLine does not call record-usage directly")
	default:
		fmt.Fprintln(w, "  [--] statusLine (not configured)")
	}

	isRecording := stateDir != "" && usage.Load(os.DirFS(stateDir)).HasActiveWindow(now)
	if isRecording {
		fmt.Fprint(w, "  [ok] usage recorded")
		if recordedAt, ok := usage.LastRecordedAt(stateDir); ok {
			fmt.Fprintf(w, " (last updated %s ago)", session.FormatElapsed(now.Sub(recordedAt)))
		}
		fmt.Fprintln(w)
	} else {
		fmt.Fprintln(w, "  [--] no current usage record (recorded after the first API response on a Pro/Max plan)")
	}

	if isRecording || callsRecordUsage {
		return
	}
	if isConfigured {
		// 既存コマンドは複合コマンドのこともあり、前にパイプを足すだけでは壊れうるため具体例に埋め込まない。
		fmt.Fprintf(w, "\nPipe the statusLine input through record-usage before your existing command in %s,\n", settingsPath)
		fmt.Fprintln(w, "or call it inside your statusLine script:")
		fmt.Fprintf(w, "  %s record-usage | <existing statusLine command>\n", exe)
		return
	}
	fmt.Fprintf(w, "\nAdd the following to %s (using %s):\n", settingsPath, exe)
	fmt.Fprintln(w, buildStatusLineSnippet(exe))
}

// readStatusLineCommand は settings.json の statusLine のコマンドを返す。
// statusLine が無い・ファイルが読めない場合は false。
func readStatusLineCommand(settingsPath string) (string, bool) {
	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		return "", false
	}
	var settings settingsStatusLine
	if err := json.Unmarshal(raw, &settings); err != nil || settings.StatusLine == nil {
		return "", false
	}
	return settings.StatusLine.Command, true
}

// isRecordUsageCommand はコマンド文字列が本アプリの record-usage 呼び出しを含むかを判定する。
// isNotifyHookCommand と同じく、絶対パスやパイプの付き方の違いを吸収するため部分一致で緩く判定する。
func isRecordUsageCommand(command string) bool {
	return strings.Contains(command, "cc-dashboard") && strings.Contains(command, "record-usage")
}

// buildStatusLineSnippet は statusLine 未設定のときの追記用 JSON 片を組み立てる。
// record-usage は入力をそのまま stdout に流すため、捨てないと JSON が表示されてしまう。
func buildStatusLineSnippet(exe string) string {
	snippet := map[string]any{
		"statusLine": map[string]string{
			"type":    "command",
			"command": exe + " record-usage > /dev/null",
		},
	}
	// json.Marshal は > を HTML 向けに Unicode エスケープし、貼り付ける例として読みにくくなるため無効にする。
	var b strings.Builder
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(snippet); err != nil {
		return "(failed to generate JSON)"
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// printObsoleteHookAdvisory は、もう参照されなくなった旧イベントが
// settings.json に登録済みのままなら削除を促す案内を出す。
// これらは notify-hook が no-op で無視するため動作に支障はないが、
// ツール呼び出しのたびに無駄なプロセスが起動され続ける。
func printObsoleteHookAdvisory(registered map[string]bool) {
	var obsolete []string
	for _, event := range obsoleteHookEvents {
		if registered[event] {
			obsolete = append(obsolete, event)
		}
	}
	if len(obsolete) == 0 {
		return
	}

	fmt.Println("\nnote: the following events are registered but no longer used by cc-dashboard")
	fmt.Println("and can be safely removed from settings.json:")
	for _, event := range obsolete {
		fmt.Printf("  - %s\n", event)
	}
}

// readRegisteredHookEvents は settings.json を読み、各イベントに
// notify-hook コマンドが既に登録されているかを返す。
// ファイルが無い・パースできない場合は「何も設定されていない」として扱う
// （settings.json 自体が壊れているケースの修復はこのコマンドの責務ではない）。
func readRegisteredHookEvents(settingsPath string) map[string]bool {
	registered := make(map[string]bool)

	raw, err := os.ReadFile(settingsPath)
	if err != nil {
		return registered
	}
	var settings settingsHooks
	if err := json.Unmarshal(raw, &settings); err != nil {
		return registered
	}

	for event, groups := range settings.Hooks {
		for _, group := range groups {
			for _, h := range group.Hooks {
				if isNotifyHookCommand(h.Command) {
					registered[event] = true
				}
			}
		}
	}
	return registered
}

// isNotifyHookCommand はコマンド文字列が本アプリの notify-hook 呼び出しかを判定する。
// 絶対パスや引数の付き方が環境で変わりうるため、部分一致で緩く判定する。
func isNotifyHookCommand(command string) bool {
	return strings.Contains(command, "cc-dashboard") && strings.Contains(command, "notify-hook")
}

// buildHookSnippet は不足イベント分の追記用 JSON 片を組み立てる。
// 既存の hooks 配列を破壊しないよう「置き換え」ではなく「追加」する形で提示する。
func buildHookSnippet(events []string, exe string) string {
	entry := hookEventGroup{Hooks: []hookEntry{{Type: "command", Command: exe + " notify-hook"}}}
	snippet := make(map[string][]hookEventGroup, len(events))
	for _, event := range events {
		snippet[event] = []hookEventGroup{entry}
	}
	data, err := json.MarshalIndent(snippet, "", "  ")
	if err != nil {
		return "(failed to generate JSON)"
	}
	return string(data)
}
