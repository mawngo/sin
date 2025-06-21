package task

import (
	"fmt"
	"github.com/mawngo/go-errors"
	"github.com/pterm/pterm"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sin/internal/core"
	"sin/internal/store"
	"sin/internal/utils"
	"strconv"
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
	// For directory format, the output will be bundled into one single file using zip.
	Format string
	// NumberOfJobs parallel pg_dump, only applicable to directory format.
	NumberOfJobs int
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
		v, err := readFileTrim(config.URI)
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
		destFileName += ".zip"
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
	if p.Format == "directory" {
		dest = strings.TrimSuffix(dest, ".zip"+core.BackupFileExt)
	}
	dumpArgs := []string{
		"-d", p.URI,
		"-v",
		"-F", p.Format,
		"-Z", p.Compress,
		"-f", dest,
	}

	command := exec.CommandContext(p.app.Ctx, p.PGDumpPath, dumpArgs...)
	command.Stderr = os.Stderr
	pterm.Printf("%sCreating local backup %s\n", prefix, p.destFileName)

	if p.Format == "directory" {
		if p.NumberOfJobs > 0 {
			dumpArgs = append([]string{"-j", strconv.Itoa(p.NumberOfJobs)}, dumpArgs...)
		}
		if err := removeAllIfExist(dest); err != nil {
			return errors.Wrapf(err, "error local backup directory with same name exist")
		}
	} else {
		if err := removeIfExist(dest); err != nil {
			return errors.Wrapf(err, "error local backup with same name exist")
		}
	}

	start := time.Now()
	if err := command.Run(); err != nil {
		if p.Format == "directory" {
			err = errors.Join(
				removeAllIfExist(dest+".error"),
				os.Rename(dest, dest+".error"),
			)
			if err != nil {
				pterm.Warning.Printf("%sFailed to rename errored backup directory %s\n", prefix, dest)
			}
		} else {
			if err := os.Rename(dest, dest+".error"); err != nil {
				pterm.Warning.Printf("%sFailed to rename errored backup %s\n", prefix, p.destFileName)
			}
		}
		return errors.Wrapf(err, "error running pg_dump")
	}

	if p.Format == "directory" {
		dumpDir := dest
		dest = dest + ".zip" + core.BackupFileExt
		pterm.Printf("%sZiping pg_dump output directory %s\n", prefix, dumpDir)
		if err := removeIfExist(dest); err != nil {
			return errors.Wrapf(err, "error local backup with same name exist")
		}

		if err := zipDir(dumpDir, dest); err != nil {
			_ = os.Remove(dest)
			return errors.Wrapf(err, "error zipping pg_dump output directory")
		}
		if err := os.RemoveAll(dumpDir); err != nil {
			pterm.Warning.Printf("%sCannot remove pg_dump output directory %s: %s\n", prefix, dumpDir, err.Error())
		}
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
	err := p.syncer.Sync(p.app.Ctx, dest, start)
	if !p.app.KeepTempFile {
		err = errors.Join(err, os.Remove(dest))
	} else {
		err = errors.Join(err, utils.CreateFileSHA256Checksum(dest))
		pterm.Printf("%sLocal backup are kept\n", prefix)
	}
	pterm.Printf("%sSync %s finished\n", prefix, p.destFileName)
	return err
}
