package source

import (
	"strconv"
	"testing"
	"testing/fstest"
	"time"
)

// fakeProcessChecker は指定した PID だけを生存扱いにする。
type fakeProcessChecker struct {
	alive map[int]bool
}

func (f fakeProcessChecker) Alive(pid int) bool {
	return f.alive[pid]
}

func newTestSource(fsys fstest.MapFS, alivePIDs ...int) *Source {
	alive := make(map[int]bool)
	for _, pid := range alivePIDs {
		alive[pid] = true
	}
	return &Source{
		fsys:           fsys,
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

	lastActivity, aiTitle := src.loadActivity("/tmp/proj", "aaa")

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

	_, aiTitle := src.loadActivity("/tmp/proj", "aaa")

	if aiTitle != "更新後のタイトル" {
		t.Errorf("aiTitle = %q, want %q（末尾から見て最後の ai-title を採用すべき）", aiTitle, "更新後のタイトル")
	}
}
