package source

import (
	"io/fs"
	"os"
	"path/filepath"
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
