package source

import (
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/ToshihitoKon/cc-dashboard-tui/internal/session"
)

// fakeProcessChecker は指定した PID だけを生存扱いにする。
type fakeProcessChecker struct {
	alive map[int]bool
}

func (f fakeProcessChecker) Alive(pid int) bool {
	return f.alive[pid]
}

func newTestSource(fsys fstest.MapFS, alivePIDs ...int) *Source {
	return newTestSourceWithState(fsys, fstest.MapFS{}, alivePIDs...)
}

// newTestSourceWithState は action-required 状態ファイルの木も指定できる版。
func newTestSourceWithState(fsys, stateFsys fstest.MapFS, alivePIDs ...int) *Source {
	alive := make(map[int]bool)
	for _, pid := range alivePIDs {
		alive[pid] = true
	}
	return &Source{
		fsys:           fsys,
		stateFsys:      stateFsys,
		proc:           fakeProcessChecker{alive: alive},
		now:            func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
		activityCache:  make(map[string]activityCache),
		branchCache:    make(map[string]branchCacheEntry),
		branchCacheTTL: 30 * time.Second,
	}
}

func registryJSON(pid int, sessionID, cwd, status string) string {
	// procStart はテストでは使わないため固定値。startedAt も適当な epoch ms。
	return `{"pid":` + strconv.Itoa(pid) + `,"sessionId":"` + sessionID + `","cwd":"` + cwd + `",` +
		`"startedAt":1787665315981,"procStart":"Tue Aug 25 13:41:53 2026",` +
		`"entrypoint":"cli","name":"fixture","status":"` + status + `"}`
}

func Test_Load_DeadProcess_IsExcluded(t *testing.T) {
	fsys := fstest.MapFS{
		"sessions/1001.json": &fstest.MapFile{
			Data: []byte(registryJSON(1001, "aaa", "/tmp/proj", "idle")),
		},
	}
	// 1001 は fake 側で生存指定していない = 死んでいる扱い
	src := newTestSource(fsys)

	result := src.Load()

	if len(result.Sessions) != 0 {
		t.Errorf("Load().Sessions = %d 件, want 0（死んでいるプロセスは除外されるべき）", len(result.Sessions))
	}
}

func Test_Load_AliveProcess_IsIncludedWithDerivedState(t *testing.T) {
	fsys := fstest.MapFS{
		"sessions/1001.json": &fstest.MapFile{
			Data: []byte(registryJSON(1001, "aaa", "/tmp/proj", "idle")),
		},
	}
	src := newTestSource(fsys, 1001)

	result := src.Load()

	if len(result.Sessions) != 1 {
		t.Fatalf("Load().Sessions = %d 件, want 1", len(result.Sessions))
	}
	got := result.Sessions[0]
	if got.SessionID != "aaa" {
		t.Errorf("SessionID = %q, want %q", got.SessionID, "aaa")
	}
	if got.RawStatus != "idle" {
		t.Errorf("RawStatus = %q, want %q", got.RawStatus, "idle")
	}
}

func Test_Load_KeyFileIsIgnored(t *testing.T) {
	fsys := fstest.MapFS{
		"sessions/1001.json": &fstest.MapFile{
			Data: []byte(registryJSON(1001, "aaa", "/tmp/proj", "idle")),
		},
		"sessions/1001.deadbeef.key": &fstest.MapFile{
			Data: []byte(`{"peerToken":"should-never-be-read"}`),
		},
	}
	src := newTestSource(fsys, 1001)

	result := src.Load()

	if len(result.Sessions) != 1 {
		t.Fatalf("Load().Sessions = %d 件, want 1（.key は無視されるべき）", len(result.Sessions))
	}
}

func Test_Load_NoSessionsDir_ReturnsEmptyWithoutError(t *testing.T) {
	src := newTestSource(fstest.MapFS{})

	result := src.Load()

	if len(result.Sessions) != 0 {
		t.Errorf("Sessions = %d 件, want 0", len(result.Sessions))
	}
	if len(result.Errors) != 0 {
		t.Errorf("Errors = %v, want 空（sessions/ 不在は正常系）", result.Errors)
	}
}

func Test_Load_MalformedJSON_IsSkippedNotFatal(t *testing.T) {
	fsys := fstest.MapFS{
		"sessions/1001.json": &fstest.MapFile{Data: []byte(`{not valid json`)},
		"sessions/1002.json": &fstest.MapFile{
			Data: []byte(registryJSON(1002, "bbb", "/tmp/proj", "idle")),
		},
	}
	src := newTestSource(fsys, 1001, 1002)

	result := src.Load()

	if len(result.Sessions) != 1 {
		t.Fatalf("Sessions = %d 件, want 1（壊れた 1 件はスキップされ、正常な 1 件は残る）", len(result.Sessions))
	}
	if len(result.Errors) != 1 {
		t.Errorf("Errors = %d 件, want 1", len(result.Errors))
	}
}

func Test_LoadActivity_SubagentNewerThanParent_UsesSubagentMtime(t *testing.T) {
	parentTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	subagentTime := parentTime.Add(time.Hour)

	fsys := fstest.MapFS{
		"projects/-tmp-proj/aaa.jsonl": &fstest.MapFile{
			Data:    []byte(`{"type":"ai-title","aiTitle":"テストセッション"}` + "\n"),
			ModTime: parentTime,
		},
		"projects/-tmp-proj/aaa/subagents/s1.jsonl": &fstest.MapFile{
			Data:    []byte(`{}`),
			ModTime: subagentTime,
		},
	}
	src := newTestSource(fsys)

	got := src.loadActivity("/tmp/proj", "aaa", time.Time{})

	if !got.lastActivity.Equal(subagentTime) {
		t.Errorf("lastActivity = %v, want %v（subagent の mtime が優先されるべき）", got.lastActivity, subagentTime)
	}
	if got.aiTitle != "テストセッション" {
		t.Errorf("aiTitle = %q, want %q", got.aiTitle, "テストセッション")
	}
}

func Test_LoadActivity_MultipleAITitles_UsesLastOne(t *testing.T) {
	fsys := fstest.MapFS{
		"projects/-tmp-proj/aaa.jsonl": &fstest.MapFile{
			Data: []byte(
				`{"type":"ai-title","aiTitle":"最初のタイトル"}` + "\n" +
					`{"type":"user"}` + "\n" +
					`{"type":"ai-title","aiTitle":"更新後のタイトル"}` + "\n",
			),
			ModTime: time.Now(),
		},
	}
	src := newTestSource(fsys)

	got := src.loadActivity("/tmp/proj", "aaa", time.Time{})

	if got.aiTitle != "更新後のタイトル" {
		t.Errorf("aiTitle = %q, want %q（末尾から見て最後の ai-title を採用すべき）", got.aiTitle, "更新後のタイトル")
	}
}

func Test_LoadActivity_MultipleModels_UsesLastOne(t *testing.T) {
	// /model コマンド等でセッション中にモデルが変わりうる。
	fsys := fstest.MapFS{
		"projects/-tmp-proj/aaa.jsonl": &fstest.MapFile{
			Data: []byte(
				`{"type":"assistant","message":{"model":"claude-opus-5"}}` + "\n" +
					`{"type":"user"}` + "\n" +
					`{"type":"assistant","message":{"model":"claude-sonnet-5"}}` + "\n",
			),
			ModTime: time.Now(),
		},
	}
	src := newTestSource(fsys)

	got := src.loadActivity("/tmp/proj", "aaa", time.Time{})

	if got.model != "claude-sonnet-5" {
		t.Errorf("model = %q, want %q（最後に見つかったモデルを採用すべき）", got.model, "claude-sonnet-5")
	}
}

func Test_LoadActivity_SyntheticModel_IsIgnored(t *testing.T) {
	// <synthetic> は実際のモデル推論を経ていない内部的な合成応答に付く値で、
	// 表示すべきモデル名ではない。
	fsys := fstest.MapFS{
		"projects/-tmp-proj/aaa.jsonl": &fstest.MapFile{
			Data: []byte(
				`{"type":"assistant","message":{"model":"claude-sonnet-5"}}` + "\n" +
					`{"type":"assistant","message":{"model":"<synthetic>"}}` + "\n",
			),
			ModTime: time.Now(),
		},
	}
	src := newTestSource(fsys)

	got := src.loadActivity("/tmp/proj", "aaa", time.Time{})

	if got.model != "claude-sonnet-5" {
		t.Errorf("model = %q, want %q（<synthetic> は無視し、直前の有効な値を保持すべき）", got.model, "claude-sonnet-5")
	}
}

func Test_LoadActivity_StringContent_DoesNotBreakExtraction(t *testing.T) {
	// message.content は tool_use/tool_result を含む行では配列だが、
	// 人間が入力したプロンプトの行では文字列になる。前者を優先して
	// パースしようとする実装で、後者に遭遇した際に ai-title/model の
	// 抽出まで巻き込んで落ちないことを確認する。
	fsys := fstest.MapFS{
		"projects/-tmp-proj/aaa.jsonl": &fstest.MapFile{
			Data: []byte(
				`{"type":"user","message":{"content":"こんにちは"}}` + "\n" +
					`{"type":"assistant","message":{"model":"claude-sonnet-5","content":[{"type":"tool_use","id":"toolu_1"}]}}` + "\n" +
					`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_1"}]}}` + "\n",
			),
			ModTime: time.Now(),
		},
	}
	src := newTestSource(fsys)

	got := src.loadActivity("/tmp/proj", "aaa", time.Now().Add(time.Hour))

	if got.model != "claude-sonnet-5" {
		t.Errorf("model = %q, want %q（文字列 content の行を挟んでも後続の抽出が壊れないべき）", got.model, "claude-sonnet-5")
	}
	if got.hasPendingToolUse {
		t.Error("hasPendingToolUse = true, want false（tool_result で解決済み）")
	}
}

func Test_LoadActivity_SidechainAssistant_IsIgnored(t *testing.T) {
	// サブエージェントのログは別ファイル（subagents/*.jsonl）に書かれ、
	// 本体 jsonl に混在しないはずだが、isSidechain のチェックを保険として持つ。
	fsys := fstest.MapFS{
		"projects/-tmp-proj/aaa.jsonl": &fstest.MapFile{
			Data: []byte(
				`{"type":"assistant","message":{"model":"claude-sonnet-5"}}` + "\n" +
					`{"type":"assistant","isSidechain":true,"message":{"model":"claude-opus-5"}}` + "\n",
			),
			ModTime: time.Now(),
		},
	}
	src := newTestSource(fsys)

	got := src.loadActivity("/tmp/proj", "aaa", time.Time{})

	if got.model != "claude-sonnet-5" {
		t.Errorf("model = %q, want %q（isSidechain の行は無視すべき）", got.model, "claude-sonnet-5")
	}
}

func Test_LoadActivity_UnresolvedToolUseBeforeActionRequiredAt_HasPendingToolUseIsTrue(t *testing.T) {
	toolUseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	actionRequiredAt := toolUseTime.Add(time.Second) // tool_use の後に発生を記録

	fsys := fstest.MapFS{
		"projects/-tmp-proj/aaa.jsonl": &fstest.MapFile{
			Data: []byte(
				`{"type":"assistant","timestamp":"` + toolUseTime.Format(time.RFC3339) + `","message":{"content":[{"type":"tool_use","id":"toolu_1"}]}}` + "\n",
			),
			ModTime: time.Now(),
		},
	}
	src := newTestSource(fsys)

	got := src.loadActivity("/tmp/proj", "aaa", actionRequiredAt)

	if !got.hasPendingToolUse {
		t.Error("hasPendingToolUse = false, want true（対応する tool_result が無い）")
	}
}

func Test_LoadActivity_ResolvedToolUse_HasPendingToolUseIsFalse(t *testing.T) {
	toolUseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	actionRequiredAt := toolUseTime.Add(time.Second)

	fsys := fstest.MapFS{
		"projects/-tmp-proj/aaa.jsonl": &fstest.MapFile{
			Data: []byte(
				`{"type":"assistant","timestamp":"` + toolUseTime.Format(time.RFC3339) + `","message":{"content":[{"type":"tool_use","id":"toolu_1"}]}}` + "\n" +
					`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_1"}]}}` + "\n",
			),
			ModTime: time.Now(),
		},
	}
	src := newTestSource(fsys)

	got := src.loadActivity("/tmp/proj", "aaa", actionRequiredAt)

	if got.hasPendingToolUse {
		t.Error("hasPendingToolUse = true, want false（tool_result で解決済み）")
	}
}

func Test_LoadActivity_SidechainToolUse_IsExcludedFromPending(t *testing.T) {
	// サブエージェントの tool_use がメイン jsonl に紛れ込んだ場合の保険。
	// 混在しない前提だが、混ざっても誤検知しないことを確認する。
	toolUseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	actionRequiredAt := toolUseTime.Add(time.Second)

	fsys := fstest.MapFS{
		"projects/-tmp-proj/aaa.jsonl": &fstest.MapFile{
			Data: []byte(
				`{"type":"assistant","isSidechain":true,"timestamp":"` + toolUseTime.Format(time.RFC3339) + `","message":{"content":[{"type":"tool_use","id":"toolu_1"}]}}` + "\n",
			),
			ModTime: time.Now(),
		},
	}
	src := newTestSource(fsys)

	got := src.loadActivity("/tmp/proj", "aaa", actionRequiredAt)

	if got.hasPendingToolUse {
		t.Error("hasPendingToolUse = true, want false（isSidechain の tool_use は集計対象外）")
	}
}

func Test_LoadActivity_ToolUseAfterActionRequiredAt_HasPendingToolUseIsFalse(t *testing.T) {
	// action-required の発生後に、それとは無関係な新しいツール呼び出しが
	// 始まったケース（承認され、通常業務としてツールが動いている状態）。
	// 発生時点より後に発行された tool_use は解除判定の対象に含めない。
	actionRequiredAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	toolUseTime := actionRequiredAt.Add(2 * time.Minute)

	fsys := fstest.MapFS{
		"projects/-tmp-proj/aaa.jsonl": &fstest.MapFile{
			Data: []byte(
				`{"type":"assistant","timestamp":"` + toolUseTime.Format(time.RFC3339) + `","message":{"content":[{"type":"tool_use","id":"toolu_2"}]}}` + "\n",
			),
			ModTime: time.Now(),
		},
	}
	src := newTestSource(fsys)

	got := src.loadActivity("/tmp/proj", "aaa", actionRequiredAt)

	if got.hasPendingToolUse {
		t.Error("hasPendingToolUse = true, want false（action-required 発生後の無関係な tool_use は対象外であるべき）")
	}
}

func Test_LoadActivity_LineExceedsScannerBuffer_DoesNotLatchStalePendingToolUse(t *testing.T) {
	// 巨大な tool_result（大きなファイルの Read 結果等）1 行が
	// bufio.Scanner のバッファ上限を超えると、それ以降の行は一切
	// 読まれない。この時点で pendingByID に残っていた古い tool_use を
	// そのまま返すと、対応する tool_result がずっと後続の行にあっても
	// 永遠に「未解決」扱いになり、action-required が固着してしまう。
	// スキャンが不完全に終わった場合は判定不能として false を返すべき。
	staleToolUseTime := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	actionRequiredAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	hugeLine := `{"type":"user","message":{"content":"` + strings.Repeat("x", 20*1024*1024) + `"}}`

	fsys := fstest.MapFS{
		"projects/-tmp-proj/aaa.jsonl": &fstest.MapFile{
			Data: []byte(
				`{"type":"assistant","timestamp":"` + staleToolUseTime.Format(time.RFC3339) + `","message":{"content":[{"type":"tool_use","id":"toolu_stale"}]}}` + "\n" +
					hugeLine + "\n" +
					`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_stale"}]}}` + "\n",
			),
			ModTime: time.Now(),
		},
	}
	src := newTestSource(fsys)

	got := src.loadActivity("/tmp/proj", "aaa", actionRequiredAt)

	if got.hasPendingToolUse {
		t.Error("hasPendingToolUse = true, want false（スキャン打ち切りで不完全な pending を action-required 扱いしてはいけない）")
	}
}

func Test_Load_ActionRequiredStateFile_IsReflectedInDerivedState(t *testing.T) {
	// 確認プロンプト発生後、まだツールが実行されていない
	// （tool_result が無い）状態を模す。tool_use は state file の
	// updatedAt（action-required 発生時刻）より前に発行されている。
	fsys := fstest.MapFS{
		"sessions/1001.json": &fstest.MapFile{
			Data: []byte(registryJSON(1001, "aaa", "/tmp/proj", "busy")),
		},
		"projects/-tmp-proj/aaa.jsonl": &fstest.MapFile{
			Data: []byte(
				`{"type":"assistant","timestamp":"2025-12-31T23:59:00Z","message":{"content":[{"type":"tool_use","id":"toolu_1"}]}}` + "\n",
			),
			ModTime: time.Now(),
		},
	}
	stateFsys := fstest.MapFS{
		"sessions/aaa.json": &fstest.MapFile{
			Data: []byte(`{"sessionId":"aaa","updatedAt":"2026-01-01T00:00:00Z"}`),
		},
	}
	src := newTestSourceWithState(fsys, stateFsys, 1001)

	result := src.Load()

	if len(result.Sessions) != 1 {
		t.Fatalf("Sessions = %d 件, want 1", len(result.Sessions))
	}
	if got := result.Sessions[0].State; got != session.StateActionRequired {
		t.Errorf("State = %v, want StateActionRequired", got)
	}
}

func Test_Load_ResolvedToolUse_ClearsActionRequired(t *testing.T) {
	// 承認されてツールが完了し、tool_result が append された後の状態。
	fsys := fstest.MapFS{
		"sessions/1001.json": &fstest.MapFile{
			Data: []byte(registryJSON(1001, "aaa", "/tmp/proj", "busy")),
		},
		"projects/-tmp-proj/aaa.jsonl": &fstest.MapFile{
			Data: []byte(
				`{"type":"assistant","timestamp":"2025-12-31T23:59:00Z","message":{"content":[{"type":"tool_use","id":"toolu_1"}]}}` + "\n" +
					`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_1"}]}}` + "\n",
			),
			ModTime: time.Now(),
		},
	}
	stateFsys := fstest.MapFS{
		"sessions/aaa.json": &fstest.MapFile{
			Data: []byte(`{"sessionId":"aaa","updatedAt":"2026-01-01T00:00:00Z"}`),
		},
	}
	src := newTestSourceWithState(fsys, stateFsys, 1001)

	result := src.Load()

	if len(result.Sessions) != 1 {
		t.Fatalf("Sessions = %d 件, want 1", len(result.Sessions))
	}
	if got := result.Sessions[0].State; got != session.StateBusy {
		t.Errorf("State = %v, want StateBusy（tool_result で解決済みなら解除されるべき）", got)
	}
}

func Test_Load_SecondPromptAfterFirstResolved_IsStillActionRequired(t *testing.T) {
	// 1回目の確認プロンプトが承認されて解決した後、同じセッションで
	// 2回目の確認プロンプトが発生し、まだ解決していないケース。
	// Test_Load_UnrelatedToolUseAfterApproval_DoesNotReArmActionRequired
	// と対になる境界確認: 1回目解決後に「何があっても再武装しない」
	// わけではなく、正当な2回目の発生はきちんと検出できる必要がある。
	fsys := fstest.MapFS{
		"sessions/1001.json": &fstest.MapFile{
			Data: []byte(registryJSON(1001, "aaa", "/tmp/proj", "busy")),
		},
		"projects/-tmp-proj/aaa.jsonl": &fstest.MapFile{
			Data: []byte(
				`{"type":"assistant","timestamp":"2025-12-31T23:59:00Z","message":{"content":[{"type":"tool_use","id":"toolu_1"}]}}` + "\n" +
					`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_1"}]}}` + "\n" +
					`{"type":"assistant","timestamp":"2026-01-01T00:10:00Z","message":{"content":[{"type":"tool_use","id":"toolu_2"}]}}` + "\n",
			),
			ModTime: time.Now(),
		},
	}
	stateFsys := fstest.MapFS{
		"sessions/aaa.json": &fstest.MapFile{
			// 2回目の tool_use（00:10:00）より後に action-required を記録。
			Data: []byte(`{"sessionId":"aaa","updatedAt":"2026-01-01T00:10:01Z"}`),
		},
	}
	src := newTestSourceWithState(fsys, stateFsys, 1001)

	result := src.Load()

	if len(result.Sessions) != 1 {
		t.Fatalf("Sessions = %d 件, want 1", len(result.Sessions))
	}
	if got := result.Sessions[0].State; got != session.StateActionRequired {
		t.Errorf("State = %v, want StateActionRequired（1回目解決後の正当な2回目の発生は検出されるべき）", got)
	}
}

func Test_Load_SubagentActivityDuringPrompt_DoesNotClearActionRequired(t *testing.T) {
	// メインが確認待ちの間にサブエージェントが動いても、メインの
	// action-required を誤って解除してはいけない（1d7b5e0 と同種の懸念）。
	fsys := fstest.MapFS{
		"sessions/1001.json": &fstest.MapFile{
			Data: []byte(registryJSON(1001, "aaa", "/tmp/proj", "busy")),
		},
		"projects/-tmp-proj/aaa.jsonl": &fstest.MapFile{
			Data: []byte(
				`{"type":"assistant","timestamp":"2025-12-31T23:59:00Z","message":{"content":[{"type":"tool_use","id":"toolu_1"}]}}` + "\n",
			),
			ModTime: time.Now(),
		},
		"projects/-tmp-proj/aaa/subagents/s1.jsonl": &fstest.MapFile{
			Data:    []byte(`{}`),
			ModTime: time.Now().Add(time.Hour), // メインより後に動いた
		},
	}
	stateFsys := fstest.MapFS{
		"sessions/aaa.json": &fstest.MapFile{
			Data: []byte(`{"sessionId":"aaa","updatedAt":"2026-01-01T00:00:00Z"}`),
		},
	}
	src := newTestSourceWithState(fsys, stateFsys, 1001)

	result := src.Load()

	if len(result.Sessions) != 1 {
		t.Fatalf("Sessions = %d 件, want 1", len(result.Sessions))
	}
	if got := result.Sessions[0].State; got != session.StateActionRequired {
		t.Errorf("State = %v, want StateActionRequired（サブエージェントの活動で誤解除されるべきではない）", got)
	}
}

func Test_Load_UnrelatedToolUseAfterApproval_DoesNotReArmActionRequired(t *testing.T) {
	// 承認されて一度解除された後、無関係な新しいツール呼び出し（通常業務、
	// またはサブエージェント経由の Task 呼び出し等）が始まっても、
	// action-required 状態ファイルが TTL 内で残っている間は再武装しない
	// べき（advisor 指摘の回帰: 古い判定はファイル全体の未解決 tool_use の
	// 有無だけを見ており、この無関係な新規呼び出しで action-required に
	// 舞い戻ってしまっていた）。
	fsys := fstest.MapFS{
		"sessions/1001.json": &fstest.MapFile{
			Data: []byte(registryJSON(1001, "aaa", "/tmp/proj", "busy")),
		},
		"projects/-tmp-proj/aaa.jsonl": &fstest.MapFile{
			Data: []byte(
				`{"type":"assistant","timestamp":"2025-12-31T23:59:00Z","message":{"content":[{"type":"tool_use","id":"toolu_1"}]}}` + "\n" +
					`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"toolu_1"}]}}` + "\n" +
					`{"type":"assistant","timestamp":"2026-01-01T00:02:00Z","message":{"content":[{"type":"tool_use","id":"toolu_2"}]}}` + "\n",
			),
			ModTime: time.Now(),
		},
	}
	stateFsys := fstest.MapFS{
		"sessions/aaa.json": &fstest.MapFile{
			Data: []byte(`{"sessionId":"aaa","updatedAt":"2026-01-01T00:00:00Z"}`),
		},
	}
	src := newTestSourceWithState(fsys, stateFsys, 1001)

	result := src.Load()

	if len(result.Sessions) != 1 {
		t.Fatalf("Sessions = %d 件, want 1", len(result.Sessions))
	}
	if got := result.Sessions[0].State; got != session.StateBusy {
		t.Errorf("State = %v, want StateBusy（action-required 発生後の無関係な tool_use で再武装するべきではない）", got)
	}
}

func Test_Load_ActionRequiredFileForUnknownSession_IsIgnored(t *testing.T) {
	fsys := fstest.MapFS{
		"sessions/1001.json": &fstest.MapFile{
			Data: []byte(registryJSON(1001, "aaa", "/tmp/proj", "idle")),
		},
	}
	// registry に存在しない sessionId（孤児ファイル）。
	stateFsys := fstest.MapFS{
		"sessions/orphan.json": &fstest.MapFile{
			Data: []byte(`{"sessionId":"orphan","updatedAt":"2026-01-01T00:00:00Z"}`),
		},
	}
	src := newTestSourceWithState(fsys, stateFsys, 1001)

	result := src.Load()

	if len(result.Sessions) != 1 {
		t.Fatalf("Sessions = %d 件, want 1", len(result.Sessions))
	}
	if got := result.Sessions[0].State; got != session.StateIdle {
		t.Errorf("State = %v, want StateIdle（孤児の状態ファイルは無視されるべき）", got)
	}
}

func Test_Load_NoStateDir_FallsBackToRawStatus(t *testing.T) {
	fsys := fstest.MapFS{
		"sessions/1001.json": &fstest.MapFile{
			Data: []byte(registryJSON(1001, "aaa", "/tmp/proj", "idle")),
		},
	}
	// stateFsys が空（hook 未設定を模す）。
	src := newTestSourceWithState(fsys, fstest.MapFS{}, 1001)

	result := src.Load()

	if len(result.Sessions) != 1 {
		t.Fatalf("Sessions = %d 件, want 1", len(result.Sessions))
	}
	if got := result.Sessions[0].State; got != session.StateIdle {
		t.Errorf("State = %v, want StateIdle（hook 未設定でも従来通り動くべき）", got)
	}
}

func Test_Load_NoStateDir_BusyStatus_FallsBackToRawStatus(t *testing.T) {
	fsys := fstest.MapFS{
		"sessions/1001.json": &fstest.MapFile{
			Data: []byte(registryJSON(1001, "aaa", "/tmp/proj", "busy")),
		},
	}
	// stateFsys が空（hook 未設定を模す）。ActionRequiredAt がゼロ値になり、
	// DeriveState の busy 分岐に入らず素通りすることを確認する。
	src := newTestSourceWithState(fsys, fstest.MapFS{}, 1001)

	result := src.Load()

	if len(result.Sessions) != 1 {
		t.Fatalf("Sessions = %d 件, want 1", len(result.Sessions))
	}
	if got := result.Sessions[0].State; got != session.StateBusy {
		t.Errorf("State = %v, want StateBusy（hook 未設定でも従来通り動くべき）", got)
	}
}

func Test_Load_WaitingStatus_IsActionRequiredWithoutStateFile(t *testing.T) {
	fsys := fstest.MapFS{
		"sessions/1001.json": &fstest.MapFile{
			Data: []byte(registryJSON(1001, "aaa", "/tmp/proj", "waiting")),
		},
	}
	// stateFsys が空（hook 未設定）でも、waiting は registry だけで
	// action-required と判定される実装になっている。
	src := newTestSourceWithState(fsys, fstest.MapFS{}, 1001)

	result := src.Load()

	if len(result.Sessions) != 1 {
		t.Fatalf("Sessions = %d 件, want 1", len(result.Sessions))
	}
	if got := result.Sessions[0].State; got != session.StateActionRequired {
		t.Errorf("State = %v, want StateActionRequired（hook 未設定でも waiting は検出されるべき）", got)
	}
}
