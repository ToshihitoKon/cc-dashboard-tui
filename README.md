# cc-dashboard

実行中の [Claude Code](https://docs.claude.com/en/docs/claude-code) セッション一覧をリアルタイムに表示する TUI ツール。

## 主な機能

- 実行中の Claude Code セッションを一覧表示（状態・使用モデル・作業ディレクトリ・Git ブランチなど）
- 対応が必要なセッション（action-required）を優先的に表示するソート・グルーピング
- Claude Code の hooks 機構と連携した action-required 検出
- Claude サブスクリプション（Pro/Max）の 5時間・7日間制限の使用率とリセットまでの時間を footer に表示（statusLine 連携）

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
| `-version` | バージョンを表示して終了する |

### サブコマンド

| サブコマンド | 説明 |
| --- | --- |
| `hook-status` | action-required hook と使用率の記録（`record-usage`）が設定済みか確認する |
| `notify-hook` | Claude Code の hooks から呼び出されるエントリポイント（手動実行は不要） |
| `record-usage` | Claude Code の statusLine から呼び出し、サブスクリプションの使用率を記録する（手動実行は不要） |

action-required の検出精度を上げるには、`cc-dashboard hook-status` の案内に従って `~/.claude/settings.json` に notify-hook を登録してください。registry の情報だけでは「実際に処理中」か「パーミッション確認待ちで止まっている」かを区別できない場合があり、hook を登録するとそうしたケースも action-required として検出できるようになります。hook は任意（opt-in）で、未設定でも他の状態表示は通常通り動作します。

### サブスクリプションの使用率表示

Claude Code は 5時間・7日間制限の使用率を statusLine コマンドの入力（`rate_limits`）として渡します。statusLine から `record-usage` を呼んで記録すると、TUI はその記録を footer に表示します。

```text
5h  ███████████████░░░░░░░░░░░░░░░  52%  ↻ 13:00 (10m)
7d  ███████████░░░░░░░░░░░░░░░░░░░  36%  ↻ 08/10 11:00 (22h10m)
↑/↓: scroll   q: quit
```

`record-usage` は stdin をそのまま stdout に流すので、既存の statusLine コマンドの前段にパイプで挟めます。`~/.claude/settings.json` の例:

```json
{
  "statusLine": {
    "type": "command",
    "command": "cc-dashboard record-usage | <既存の statusLine コマンド>"
  }
}
```

statusLine を使っていない場合は、入力の JSON がそのまま表示されないよう出力を捨ててください。

```json
{
  "statusLine": {
    "type": "command",
    "command": "cc-dashboard record-usage > /dev/null"
  }
}
```

- 設定できているかは `cc-dashboard hook-status` で確認できます。確認するのは `~/.claude/settings.json` の statusLine です。statusLine のスクリプト内やプロジェクトの設定で `record-usage` を呼んでいる場合も、リセット前の値が記録されていれば設定済みとみなします。
- 使用率は Pro/Max 加入時に、各セッションで最初の API 応答を受けた後から記録されます。API キー利用時は記録されません。
- 記録先は `$XDG_STATE_HOME/cc-dashboard/usage.json`（未設定なら `~/.local/state/cc-dashboard/usage.json`）です。
- 値は、このマシンの Claude Code セッションの statusLine が新しい値を受け取ったときに更新されます。claude.ai など他の経路での利用は、その後にどれかのセッションが新しい値を受け取るまで反映されません。
- リセット時刻を過ぎた枠は、次に記録されるまで `--` と表示します。すべての枠がリセット時刻を過ぎると、footer は従来の 1 行に戻ります。

## 開発

```sh
go build ./...
go test ./...
```
