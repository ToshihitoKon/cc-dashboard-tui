package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// requiredHookEvents は action-required 検出に必要な hook イベント。
//
// PreToolUse/PostToolUse/UserPromptSubmit/Stop/SessionEnd は
// action-required の「解除」検出に、Notification は「発生」検出に使う。
var requiredHookEvents = []string{
	"Notification",
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
		return
	}

	fmt.Printf("\nAdd the following to the hooks in %s (using %s):\n", settingsPath, exe)
	fmt.Println(buildHookSnippet(missing, exe))
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
