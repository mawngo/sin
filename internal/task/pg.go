package task

import (
	"archive/tar"
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

	// Compress specifies compression algorithm and/or level,
	// basically the compress flag of pg_dump with some constraint.
	//
	// If enable gzip is specified, compress must only specify a level, otherwise, it must always specify an algorithm.
	// If you enable gzip using this field instead of [SyncPostgresConfig.EnableGzip],
	// the output file won't have gz suffix.
	//
	// By default, no compression is used (equivalent to `--compress=none`).
	Compress string
	// Format is the format option of pg_dump.
	// However, we only support plain, directory, and custom (default).
	// For directory format, the output will be bundled into one single file using tar.
	Format string
}

type syncPostgres struct {
	app          *core.App
	syncer       *store.Syncer
	destFileName string
	SyncPostgresConfig
}

func NewSyncPostgres(app *core.App, syncer *store.Syncer, config SyncPostgresConfig) (SyncTask, error) {
	if !isPostgresConnectionString(config.URI) {
		if err := validateFilePath(config.URI, "postgres connection string"); err != nil {
			return nil, err
		}
		v, err := readFile(config.URI)
		if err != nil {
			return nil, err
		}
		// Support connection string in a text file.
		if isPostgresConnectionString(v) {
			config.URI = v
		} else {
			return nil, errors.New("invalid connection string uri")
		}
	}

	if config.PGDumpPath != "" && strings.ContainsRune(config.PGDumpPath, os.PathSeparator) {
		if err := validateFilePath(config.PGDumpPath, "pg_dump"); err != nil {
			return nil, err
		}
	} else {
		config.PGDumpPath = "pg_dump"
	}

	destFileName := app.Name
	if config.Tag != "" {
		destFileName = fmt.Sprintf("[%s] %s", config.Tag, destFileName)
	}

	if config.EnableGzip {
		if config.Compress != "" {
			if !utils.IsNumeric(config.Compress) {
				return nil, errors.New("compress must only specify a level when gzip is enabled")
			}
			config.Compress = ":" + config.Compress
		}
		config.Compress = "gzip" + config.Compress
	} else if utils.IsNumeric(config.Compress) {
		return nil, errors.New("compress must specify an algorithm when gzip is disabled")
	}

	if config.Compress == "" {
		config.Compress = "none"
	}

	if config.Format == "" {
		config.Format = "custom"
	}

	// Handle extension.
	if config.Format == "directory" {
		destFileName += ".tar"
	} else if config.EnableGzip {
		destFileName += ".gz"
	}

	if config.Format != "custom" && config.Format != "directory" && config.Format != "plain" {
		return nil, errors.Newf("invalid format '%s'", config.Format)
	}

	return &syncPostgres{
		app:                app,
		syncer:             syncer,
		SyncPostgresConfig: config,
		destFileName:       destFileName + core.BackupFileExt,
	}, nil
}

func isPostgresConnectionString(uri string) bool {
	return strings.HasPrefix(uri, "postgresql://") || strings.HasPrefix(uri, "postgres://")
}

func (p *syncPostgres) ExecSync() error {
	prefix := ""
	if p.Tag != "" {
		prefix = fmt.Sprintf("[%s]: ", p.Tag)
	}

	dest := filepath.Join(p.app.Config.BackupTempDir, p.destFileName)
	dumpArgs := []string{
		"-d", p.URI,
		"-v",
		"-F", p.Format,
		"-Z", p.Compress,
	}

	pterm.Printf("%sCreating local backup %s\n", prefix, p.destFileName)

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
			tarWriter := tar.NewWriter(w)
			defer tarWriter.Close()
			defer tarWriter.Flush()
			w = tarWriter
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

	pterm.Printf("%sLocal backup %s created took %s\n", prefix, p.destFileName, time.Since(start).String())
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
	pterm.Printf("%sSync %s finished\n", prefix, p.destFileName)
	return err
}
