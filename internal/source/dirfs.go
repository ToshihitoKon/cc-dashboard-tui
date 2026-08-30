package source

import (
	"io/fs"
	"os"
	"path/filepath"

	"github.com/ToshihitoKon/cc-dashboard-tui/internal/xdgstate"
)

// newDirFS は root を fs.FS として開く。
//
// ~/.claude は ~/.config/claude への symlink であることがあるため、
// os.DirFS に渡す前に EvalSymlinks で実パスに解決しておく。
// fs.FS はスラッシュ区切りの相対パスのみを扱うため、
// symlink 解決はここで完結させ、以降の層に生パスを漏らさない。
func newDirFS(root string) fs.FS {
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil {
		// root 自体が存在しない場合はそのまま渡す。
		// os.DirFS はパスの存在を検証しないため、Stat/ReadDir 呼び出し時に
		// fs.ErrNotExist を返すだけで済み、アプリ側は「空状態」として扱える。
		resolved = root
	}
	return os.DirFS(resolved)
}

// unresolvableStateDir は XDG state ディレクトリが解決できなかった場合に
// 使う、存在しないことが保証されたパス。fs.ReadDir が fs.ErrNotExist を
// 返すだけになり、hook 機能が使えないだけで致命的にはならない。
const unresolvableStateDir = "/nonexistent-cc-dashboard-state-dir"

// newStateFS は action-required 状態ファイル用の fs.FS を開く。
func newStateFS() fs.FS {
	dir := xdgstate.ResolveDir()
	if dir == "" {
		dir = unresolvableStateDir
	}
	return os.DirFS(dir)
}
