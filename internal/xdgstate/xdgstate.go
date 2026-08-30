// Package xdgstate は XDG Base Directory 仕様に基づく state ディレクトリを解決する。
package xdgstate

import (
	"errors"
	"os"
	"path/filepath"
)

// appDirName はこのアプリの state ディレクトリ名。
const appDirName = "cc-dashboard"

// Dir はアプリの state ディレクトリを返す。
//
// ~/.claude 配下は Claude Code 本体の管理領域であり、部外ツールが書き込む
// べきではないため、状態ファイルは XDG state ディレクトリに置く。
// os.UserCacheDir 等の標準ライブラリ API は macOS で ~/Library/... を
// 返してしまい XDG の慣習と異なるため使わず、ここで直接組み立てる。
func Dir(xdgStateHome, home string) (string, error) {
	if xdgStateHome != "" {
		return filepath.Join(xdgStateHome, appDirName), nil
	}
	if home == "" {
		return "", errors.New("xdgstate: home directory is unknown")
	}
	return filepath.Join(home, ".local", "state", appDirName), nil
}

// ResolveDir は環境変数・ホームディレクトリの取得まで含めた本番用ラッパー。
// 解決できない場合は空文字を返す（呼び出し側は「hook 機能を諦める」扱いにする）。
func ResolveDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = ""
	}
	dir, err := Dir(os.Getenv("XDG_STATE_HOME"), home)
	if err != nil {
		return ""
	}
	return dir
}
