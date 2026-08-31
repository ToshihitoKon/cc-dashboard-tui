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
| `-version` | バージョンを表示して終了する |

### サブコマンド

| サブコマンド | 説明 |
| --- | --- |
| `hook-status` | action-required hook が設定済みか確認する |
| `notify-hook` | Claude Code の hooks から呼び出されるエントリポイント（手動実行は不要） |

action-required の検出精度を上げるには、`cc-dashboard hook-status` の案内に従って `~/.claude/settings.json` に notify-hook を登録してください。`hook-status` は不要になった旧イベントが登録されている場合、その削除も案内します。

## action-required の検出方式

セッションの実行状態は registry（`sessions/<PID>.json`）の `status`（busy/idle/欠損）から取得できるが、「実際に処理が進行中」なのか「パーミッション確認待ちで停止している」のかはこれだけでは区別できない。この一点を補うために `notify-hook` を用意している。参照するのは `status == busy` のときだけで、idle/unknown は registry だけで判断が完結する。

hook は必須ではなく、未設定でも busy/idle/unknown の表示は動作する（opt-in）。`hook-status` は `~/.claude/settings.json` を読み取り専用で確認し、不足分の追記用 JSON を提示するだけで、設定ファイル自体は書き換えない。状態は XDG state dir（例: `~/.local/state/cc-dashboard/`）に置き、Claude Code 本体の領域である `~/.claude` 配下には書き込まない。

### 発生は Notification、解除は jsonl の中身で判定する

action-required の「発生」は `Notification`（`permission_prompt`）hook 1つだけで検出する。「解除」には hook を使わず、セッションの本体 jsonl 上に「action-required 発生時刻（`ActionRequiredAt`）以前に発行され、対応する `tool_result` をまだ持たない `tool_use`」が残っているかどうかで判定する。承認されてツールが完了すると `tool_result` が append され、この未解決状態が解消されると想定している（未検証。「設計上のコスト」参照）。

`ActionRequiredAt` より後に発行された `tool_use` は判定に含めない。含めると、一度解除された後に無関係な別のツール呼び出しが走っただけで TTL の間ずっと action-required 表示に戻ってしまう。

発生の検出だけは hook に残す必要がある。「未解決の `tool_use` が一定時間残っている」という経過時間だけで発生を推測する方式（stall 検出）は、長いツール呼び出し中の busy を「止まっている」と誤認してしまうため避けている。

### サブエージェントとの区別

サブエージェントのログは本体 jsonl とは別ファイル（`<sessionId>/subagents/*.jsonl`）に書かれる。表示用の最終活動時刻（`LastActivity`）はこのログも含めた mtime の max を取るが、action-required の解除判定には本体 jsonl 単体だけを見る。max を使うと、メインが確認待ちの間にサブエージェントが動いただけで誤って解除されてしまう。

### 30分の期限

パーミッション確認が拒否されて `tool_result` が発行されない場合のフォールバック。記録から30分経過した action-required 状態は無視し、通常の busy/idle 判定にフォールバックする。ディスク上の状態ファイル自体も TTL 超過分を日和見的に削除するが、これは他セッションで確認プロンプトが発生したときにしか実行されないため、以後発生しないセッションのファイルは残り続ける（表示への影響は無い）。

### 設計上のコスト

- パーミッション確認を拒否した場合も `tool_result` が発行される、および対象の `tool_use` 行がプロンプト表示より前に jsonl へ書き込まれている、という2つの前提の上に成り立っている。どちらも未検証で、外れる場合は解除されず TTL まで action-required 表示が残留する
- サブエージェントの tool_use/tool_result が本体 jsonl に混入しないという前提も、実ログでの確認までは取れていない

## 開発

```sh
go build ./...
go test ./...
```
