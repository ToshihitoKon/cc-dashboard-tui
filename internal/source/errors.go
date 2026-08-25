package source

import (
	"errors"
	"fmt"
	"io/fs"
)

// namedErr はエラーにファイル名（basename のみ）を添える。
// フルパスにはプロジェクト名が含まれうるため、含めるのは名前とエラー種別のみ。
func namedErr(name string, err error) error {
	return fmt.Errorf("%s: %w", name, err)
}

func isNotExist(err error) bool {
	return errors.Is(err, fs.ErrNotExist)
}
