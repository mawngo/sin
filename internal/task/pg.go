package task

import (
	"compress/gzip"
	"fmt"
	"github.com/mawngo/go-errors"
	"github.com/pterm/pterm"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sin/internal/core"
	"sin/internal/store"
	"sin/internal/utils"
	"strings"
	"time"
)

var _ SyncTask = (*syncPostgres)(nil)

type SyncPostgresConfig struct {
	URI        string
	PGDumpPath string
	EnableGzip bool
	Tag        string
}

type syncPostgres struct {
	app          *core.App
	syncer       *store.Syncer
	destFileName string
	SyncPostgresConfig
}

func NewSyncPostgres(app *core.App, syncer *store.Syncer, config SyncPostgresConfig) (SyncTask, error) {
	if !strings.HasPrefix(config.URI, "postgresql://") {
		return nil, errors.New("invalid connection string uri")
	}

	if config.PGDumpPath != "" {
		if err := validateFilePath(config.PGDumpPath, "pg_dump"); err != nil {
			return nil, err
		}
	} else {
		config.PGDumpPath = "pg_dump"
	}

	destFileName := app.Name
	if config.Tag != "" {
		destFileName += "." + config.Tag
	}
	if config.EnableGzip {
		destFileName += ".gz"
	}

	return &syncPostgres{
		app:                app,
		syncer:             syncer,
		SyncPostgresConfig: config,
		destFileName:       destFileName + core.BackupFileExt,
	}, nil
}

func (p *syncPostgres) ExecSync() error {
	prefix := ""
	if p.Tag != "" {
		prefix = fmt.Sprintf("[%s]: ", p.Tag)
	}

	dest := filepath.Join(p.app.Config.BackupTempDir, p.destFileName)
	dumpArgs := []string{"-d", p.URI, "-v"}

	pterm.Printf("%sCreating local backup\n", prefix)

	pterm.Debug.Printf("%sTruncating old local backup\n", prefix)
	f, err := os.Create(dest)
	if err != nil {
		return errors.Wrapf(err, "error creating backup file %s", dest)
	}

	command := exec.CommandContext(p.app.Ctx, p.PGDumpPath, dumpArgs...)
	command.Stderr = os.Stderr
	out, err := command.StdoutPipe()
	if err != nil {
		return errors.Wrapf(err, "error creating stdout pipe")
	}

	start := time.Now()
	err = (func() error {
		defer f.Close()
		var w io.Writer = f
		if p.EnableGzip {
			gzw := gzip.NewWriter(w)
			defer gzw.Close()
			defer gzw.Flush()
			w = gzw
		}
		if err := command.Start(); err != nil {
			return errors.Wrapf(err, "error running command")
		}
		if _, err := io.Copy(w, out); err != nil {
			return errors.Wrapf(err, "error piping pg_dump output to file %s", dest)
		}
		if err := command.Wait(); err != nil {
			return errors.Wrapf(err, "error running pg_dump")
		}
		return nil
	})()
	if err != nil {
		return err
	}

	pterm.Printf("%sLocal backup created took %s\n", prefix, time.Since(start).String())
	slog.Info(fmt.Sprintf("%sLocal backup created", prefix),
		slog.String("name", p.app.Name),
		slog.String("took", time.Since(start).String()),
	)
	if p.syncer.AdaptersCount() == 0 {
		pterm.Printf("%sLocal backup are kept as there are no targets configured\n", prefix)
		return utils.CreateFileSHA256Checksum(dest)
	}
	err = p.syncer.Sync(p.app.Ctx, dest, start)
	if !p.app.KeepTempFile {
		err = errors.Join(err, os.Remove(dest))
	} else {
		err = errors.Join(err, utils.CreateFileSHA256Checksum(dest))
		pterm.Printf("%sLocal backup are kept\n", prefix)
	}
	return err
}
