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
	isDir        bool
	destFileName string
	SyncFileConfig
}

type SyncFileConfig struct {
	SourcePath string
	Tag        string
}

func NewSyncFile(app *core.App, syncer *store.Syncer, config SyncFileConfig) (SyncTask, error) {
	isDir := false
	//nolint:revive
	if info, err := os.Stat(config.SourcePath); err != nil {
		return nil, errors.Wrapf(err, "invalid source file %s", config.SourcePath)
	} else {
		isDir = info.IsDir()
	}

	destFileName := app.Name
	if config.Tag != "" {
		destFileName = fmt.Sprintf("[%s] %s", config.Tag, destFileName)
	}
	if isDir {
		destFileName += ".zip"
	} else {
		_, extname, hasExt := strings.Cut(filepath.Base(config.SourcePath), ".")
		if hasExt {
			destFileName += "." + extname
		}
	}

	return &syncFile{
		app:            app,
		syncer:         syncer,
		isDir:          isDir,
		destFileName:   destFileName + core.BackupFileExt,
		SyncFileConfig: config,
	}, nil
}

func (f *syncFile) ExecSync() error {
	prefix := ""
	if f.Tag != "" {
		prefix = fmt.Sprintf("[%s]: ", f.Tag)
	}

	dest := filepath.Join(f.app.Config.BackupTempDir, f.destFileName)
	pterm.Printf("%sCreating local backup %s\n", prefix, f.destFileName)
	start := time.Now()
	if f.isDir {
		if err := f.zipDir(f.SourcePath, dest); err != nil {
			_ = os.Remove(dest)
			return errors.Wrapf(err, "error creating backup")
		}
	} else {
		if err := utils.CopyFile(f.app.Ctx, f.SourcePath, dest); err != nil {
			_ = os.Remove(dest)
			return errors.Wrapf(err, "error creating backup")
		}
	}
	pterm.Printf("%sLocal backup %s created took %s\n", prefix, f.destFileName, time.Since(start).String())
	if f.syncer.AdaptersCount() == 0 {
		pterm.Printf("%sLocal backup are kept as there are no targets configured\n", prefix)
		return utils.CreateFileSHA256Checksum(dest)
	}
	err := f.syncer.Sync(f.app.Ctx, dest, start)
	if !f.app.KeepTempFile {
		err = errors.Join(err, os.Remove(dest))
	} else {
		err = errors.Join(err, utils.CreateFileSHA256Checksum(dest))
		pterm.Printf("%sLocal backup are kept\n", prefix)
	}
	pterm.Printf("%sSync %s finished\n", prefix, f.destFileName)
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
