package store

import (
	"errors"
	"fmt"
)

// ErrCorrupt identifies an unreadable SQLite image, not a missing core and not
// permission to initialize an empty configuration over existing data.
var ErrCorrupt = errors.New("SQLite 数据库已损坏或文件不完整")

func describeOpenError(err error) error {
	var coded interface{ Code() int }
	if errors.As(err, &coded) {
		// SQLite extended result codes retain the primary code in the low byte.
		code := coded.Code() & 0xff
		if code == 11 || code == 26 { // SQLITE_CORRUPT / SQLITE_NOTADB
			return fmt.Errorf("%w (SQLite code %d)：请停止所有实例后，从完整备份恢复数据库及匹配的 master.key；不要在运行中覆盖数据库，也不要删除 WAL/SHM 文件；程序不会自动清空数据", ErrCorrupt, code)
		}
	}
	return err
}
