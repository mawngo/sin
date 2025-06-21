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
			return errors.Wrapf(err, "invalid %s file", msg)
		}
		return errors.Newf("invalid %s file: is a directory: %s", msg, path)
	}
	return nil
}

func readFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", errors.Wrapf(err, "error reading file")
	}
	return string(b), nil
}
