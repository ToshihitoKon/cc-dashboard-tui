package source

import (
	"strconv"
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

	lastActivity, aiTitle, _ := src.loadActivity("/tmp/proj", "aaa")

	if !lastActivity.Equal(subagentTime) {
		t.Errorf("lastActivity = %v, want %v（subagent の mtime が優先されるべき）", lastActivity, subagentTime)
	}
	if aiTitle != "テストセッション" {
		t.Errorf("aiTitle = %q, want %q", aiTitle, "テストセッション")
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

	_, aiTitle, _ := src.loadActivity("/tmp/proj", "aaa")

	if aiTitle != "更新後のタイトル" {
		t.Errorf("aiTitle = %q, want %q（末尾から見て最後の ai-title を採用すべき）", aiTitle, "更新後のタイトル")
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

	_, _, model := src.loadActivity("/tmp/proj", "aaa")

	if model != "claude-sonnet-5" {
		t.Errorf("model = %q, want %q（最後に見つかったモデルを採用すべき）", model, "claude-sonnet-5")
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

	_, _, model := src.loadActivity("/tmp/proj", "aaa")

	if model != "claude-sonnet-5" {
		t.Errorf("model = %q, want %q（<synthetic> は無視し、直前の有効な値を保持すべき）", model, "claude-sonnet-5")
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

	_, _, model := src.loadActivity("/tmp/proj", "aaa")

	if model != "claude-sonnet-5" {
		t.Errorf("model = %q, want %q（isSidechain の行は無視すべき）", model, "claude-sonnet-5")
	}
}

func Test_Load_ActionRequiredStateFile_IsReflectedInDerivedState(t *testing.T) {
	fsys := fstest.MapFS{
		"sessions/1001.json": &fstest.MapFile{
			Data: []byte(registryJSON(1001, "aaa", "/tmp/proj", "busy")),
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
