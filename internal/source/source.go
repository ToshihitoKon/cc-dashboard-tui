package source

import (
	"io/fs"
	"time"

	"github.com/ToshihitoKon/cc-dashboard-tui/internal/session"
)

// Source は Claude Code のセッション情報ディレクトリから
// 実行中セッションの一覧を組み立てる。
type Source struct {
	fsys fs.FS
	proc ProcessChecker
	now  func() time.Time

	// stateFsys は hookstate パッケージが書き込む action-required 状態
	// ファイルの木。fsys（~/.claude 相当）とは別の場所（XDG state
	// ディレクトリ）を指す。hook 未設定なら中身が空でもよい。
	stateFsys fs.FS

	// activityCache は jsonl の mtime をキーにした ai-title の再抽出防止キャッシュ。
	// key は sessionID。
	activityCache map[string]activityCache

	// branchCache は cwd ごとの git ブランチのキャッシュ。ブランチ切り替えの
	// 反映が多少遅れても実害がないため、ポーリング毎の再読み込みは避ける。
	branchCache    map[string]branchCacheEntry
	branchCacheTTL time.Duration
}

type branchCacheEntry struct {
	branch    string
	fetchedAt time.Time
}

// New は本番用の Source を作る。root は "~/.claude" 相当のディレクトリ。
func New(root string) *Source {
	return &Source{
		fsys:           newDirFS(root),
		stateFsys:      newStateFS(),
		proc:           NewOSProcessChecker(),
		now:            time.Now,
		activityCache:  make(map[string]activityCache),
		branchCache:    make(map[string]branchCacheEntry),
		branchCacheTTL: 30 * time.Second,
	}
}

// LoadResult は 1 回のポーリングの結果。
type LoadResult struct {
	// Sessions は session.SortSessions 済み（ユーザーの操作が必要な順）。
	Sessions []session.Session
	// Errors は個別ファイルの読み込み失敗（basename とエラー種別のみ）。
	// 全体を止めるほどではない部分的な失敗を表示側に伝えるためのもの。
	Errors []error
}

// Load は実行中セッションの一覧を返す。
//
// 「実行中」は sessions/*.json に registry があり、かつ実プロセスが
// 生存しているセッションを指す（登録だけ残って終了しているケースを除く）。
func (s *Source) Load() LoadResult {
	entries, errs := readRegistry(s.fsys)
	actionRequiredTimes := loadActionRequiredTimes(s.stateFsys)
	now := s.now()

	var sessions []session.Session
	for _, e := range entries {
		if !s.proc.Alive(e.PID) {
			continue // registry が残っているだけの終了済みセッションは表示しない
		}

		lastActivity, aiTitle, model := s.loadActivity(e.CWD, e.SessionID)
		procStart, _ := e.procStartTime()

		sess := session.Session{
			PID:          e.PID,
			SessionID:    e.SessionID,
			CWD:          e.CWD,
			StartedAt:    e.startedAt(),
			ProcStart:    procStart,
			RawStatus:    e.Status,
			Entrypoint:   e.Entrypoint,
			RegistryName: e.RegistryName,
			AITitle:      aiTitle,
			Model:        model,
			LastActivity: lastActivity,
			GitBranch:    s.branchOf(e.CWD),
		}
		sess.State = session.DeriveState(session.StateInput{
			RawStatus:        sess.RawStatus,
			Now:              now,
			ActionRequiredAt: actionRequiredTimes[e.SessionID], // 無ければゼロ値
		})
		sessions = append(sessions, sess)
	}

	session.SortSessions(sessions)
	return LoadResult{Sessions: sessions, Errors: errs}
}

func (s *Source) branchOf(cwd string) string {
	now := s.now()
	if cached, ok := s.branchCache[cwd]; ok && now.Sub(cached.fetchedAt) < s.branchCacheTTL {
		return cached.branch
	}
	branch := gitBranchOf(cwd)
	s.branchCache[cwd] = branchCacheEntry{branch: branch, fetchedAt: now}
	return branch
}
