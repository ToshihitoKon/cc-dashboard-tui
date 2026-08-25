package source

import (
	"os"
	"path/filepath"
	"strings"
)

// gitBranchOf は cwd の Git ブランチ名を返す。非 Git ディレクトリなら空文字。
//
// sessions/*.json にブランチ情報は含まれていないため、cwd/.git/HEAD を
// 直接読む。cwd は ~/.claude の外なので fs.FS ではなく os パッケージで扱う。
func gitBranchOf(cwd string) string {
	headPath, ok := resolveHeadPath(cwd)
	if !ok {
		return ""
	}

	raw, err := os.ReadFile(headPath)
	if err != nil {
		return ""
	}
	return parseHeadRef(string(raw))
}

// resolveHeadPath は cwd/.git を見て HEAD ファイルの実パスを返す。
//
// .git が通常のリポジトリではディレクトリ、worktree では
// "gitdir: <path>" と書かれたファイルになる。
func resolveHeadPath(cwd string) (string, bool) {
	gitPath := filepath.Join(cwd, ".git")
	info, err := os.Stat(gitPath)
	if err != nil {
		return "", false
	}

	if info.IsDir() {
		return filepath.Join(gitPath, "HEAD"), true
	}

	raw, err := os.ReadFile(gitPath)
	if err != nil {
		return "", false
	}
	const prefix = "gitdir:"
	line := strings.TrimSpace(string(raw))
	if !strings.HasPrefix(line, prefix) {
		return "", false
	}
	gitDir := strings.TrimSpace(strings.TrimPrefix(line, prefix))
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(cwd, gitDir)
	}
	return filepath.Join(gitDir, "HEAD"), true
}

// parseHeadRef は HEAD ファイルの中身からブランチ名を取り出す。
// "ref: refs/heads/main" ならブランチ名、SHA 直書き（detached HEAD）なら "detached"。
func parseHeadRef(content string) string {
	content = strings.TrimSpace(content)
	const refPrefix = "ref: refs/heads/"
	if strings.HasPrefix(content, refPrefix) {
		return strings.TrimPrefix(content, refPrefix)
	}
	if content == "" {
		return ""
	}
	return "detached"
}
