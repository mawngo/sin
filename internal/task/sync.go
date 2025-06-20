package task

import (
	"github.com/mawngo/go-errors"
	"os"
)

type SyncTask interface {
	ExecSync() error
}

func validateFilePath(path string, msg string) error {
	if stats, err := os.Stat(path); err != nil || stats.IsDir() {
		if err != nil {
			return errors.Wrapf(err, "invalid %s file %s", msg, path)
		}
		return errors.Newf("invalid %s file %s: is a directory", msg, path)
	}
	return nil
}
