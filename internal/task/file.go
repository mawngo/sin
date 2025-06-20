package task

import (
	"archive/zip"
	"fmt"
	"github.com/mawngo/go-errors"
	"github.com/pterm/pterm"
	"io"
	"os"
	"path/filepath"
	"sin/internal/core"
	"sin/internal/store"
	"sin/internal/utils"
	"strings"
	"time"
)

var _ SyncTask = (*syncFile)(nil)

type syncFile struct {
	app          *core.App
	syncer       *store.Syncer
	name         string
	source       string
	isDir        bool
	destFileName string
}

func NewSyncFile(app *core.App, syncer *store.Syncer, name string, sourcePath string) (SyncTask, error) {
	isDir := false
	//nolint:revive
	if info, err := os.Stat(sourcePath); err != nil {
		return nil, errors.Wrapf(err, "invalid source file %s", sourcePath)
	} else {
		isDir = info.IsDir()
	}

	destFileName := app.Name
	if name != "" {
		destFileName += "." + name
	}
	if isDir {
		destFileName += ".zip"
	} else {
		_, extname, hasExt := strings.Cut(filepath.Base(sourcePath), ".")
		if hasExt {
			destFileName += "." + extname
		}
	}

	return &syncFile{
		app:          app,
		syncer:       syncer,
		name:         name,
		source:       sourcePath,
		isDir:        isDir,
		destFileName: destFileName + core.BackupFileExt,
	}, nil
}

func (f *syncFile) ExecSync() error {
	prefix := ""
	if f.name != "" {
		prefix = fmt.Sprintf("[%s]: ", f.name)
	}

	dest := filepath.Join(f.app.Config.BackupTempDir, f.destFileName)
	pterm.Printf("%sCreating local backup\n", prefix)
	start := time.Now()
	if f.isDir {
		if err := f.zipDir(f.source, dest); err != nil {
			_ = os.Remove(dest)
			return errors.Wrapf(err, "error creating backup")
		}
	} else {
		if err := utils.CopyFile(f.app.Ctx, f.source, dest); err != nil {
			_ = os.Remove(dest)
			return errors.Wrapf(err, "error creating backup")
		}
	}
	pterm.Printf("%sLocal backup created took %s\n", prefix, time.Since(start).String())
	if f.syncer.AdaptersCount() == 0 {
		pterm.Printf("%sLocal backup are kept as there are no targets configured\n", prefix)
		return utils.CreateFileSHA256Checksum(dest)
	}
	err := f.syncer.Sync(f.app.Ctx, dest, start)
	if !f.app.KeepTempFile {
		err = errors.Join(err, os.Remove(dest))
	} else {
		err = errors.Join(err, utils.CreateFileSHA256Checksum(dest))
	}
	return err
}

func (*syncFile) zipDir(src, dst string) (err error) {
	file, err := os.Create(dst)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	w := zip.NewWriter(file)
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
