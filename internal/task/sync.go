package task

import (
	"archive/zip"
	"compress/flate"
	"fmt"
	"github.com/mawngo/go-errors"
	"io"
	"os"
	"path/filepath"
	"strings"
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

func readFileTrim(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", errors.Wrapf(err, "error reading file")
	}
	return strings.TrimSpace(string(b)), nil
}

func removeIfExist(path string) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrapf(err, "error removing file")
	}
	if err := os.Remove(path); err != nil {
		return errors.Wrapf(err, "error removing file")
	}
	return nil
}

func removeAllIfExist(path string) error {
	if stats, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return errors.Wrapf(err, "error removing file")
	} else if !stats.IsDir() {
		return errors.Newf("not a directory: %s", path)
	}
	if err := os.RemoveAll(path); err != nil {
		return errors.Wrapf(err, "error removing file")
	}
	return nil
}

// zipDir create a zip file from a directory, without any compression.
func zipDir(src, dst string) (err error) {
	file, err := os.Create(dst)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	w := zip.NewWriter(file)
	w.RegisterCompressor(zip.Deflate, func(out io.Writer) (io.WriteCloser, error) {
		return flate.NewWriter(out, flate.NoCompression)
	})
	defer w.Close()

	src, _ = filepath.Abs(src)
	dir := filepath.Dir(src)
	walker := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}

		if info.IsDir() {
			// Add a trailing slash for creating dir.
			// Must use '/', not filepath.Separator.
			path = fmt.Sprintf("%s%c", rel, '/')
			_, err = w.Create(path)
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		f, err := w.Create(rel)
		if err != nil {
			return err
		}

		_, err = io.Copy(f, file)
		if err != nil {
			return err
		}

		return nil
	}
	return filepath.Walk(src, walker)
}
