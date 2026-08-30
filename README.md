# cc-dashboard

実行中の [Claude Code](https://docs.claude.com/en/docs/claude-code) セッション一覧をリアルタイムに表示する TUI ツール。

## 主な機能

- 実行中の Claude Code セッションを一覧表示（状態・使用モデル・作業ディレクトリ・Git ブランチなど）
- 対応が必要なセッション（action-required）を優先的に表示するソート・グルーピング
- Claude Code の hooks 機構と連携した action-required 検出

## インストール

### Homebrew

```sh
brew install ToshihitoKon/tap/cc-dashboard
```

### go install

```sh
go install github.com/ToshihitoKon/cc-dashboard-tui/cmd/cc-dashboard@latest
```

### GitHub Releases

[Releases](https://github.com/ToshihitoKon/cc-dashboard-tui/releases) からビルド済みバイナリをダウンロードしてください。

## 使い方

```sh
cc-dashboard
```

デフォルトでは `~/.claude` 配下のセッションを走査する。別のディレクトリを指定する場合は `-root` フラグを使う。

```sh
cc-dashboard -root /path/to/claude/dir
```

### フラグ

| フラグ | 説明 |
| --- | --- |
| `-root` | Claude Code セッションディレクトリ（デフォルト: `~/.claude`） |
| `-dump` | TUI を起動せず、パース結果を一度だけテキスト出力する |

### サブコマンド

| サブコマンド | 説明 |
| --- | --- |
| `hook-status` | action-required hook が設定済みか確認する |
| `notify-hook` | Claude Code の hooks から呼び出されるエントリポイント（手動実行は不要） |

action-required の検出精度を上げるには、`cc-dashboard hook-status` の案内に従って `~/.claude/settings.json` に notify-hook を登録してください。

## 開発

```sh
go build ./...
go test ./...
```
