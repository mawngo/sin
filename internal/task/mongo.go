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
	"strings"
	"time"
)

var _ SyncTask = (*syncMongo)(nil)

type SyncMongoConfig struct {
	URI           string
	MongodumpPath string
	EnableGzip    bool
	Tag           string
}

type syncMongo struct {
	app           *core.App
	syncer        *store.Syncer
	useConfigFile bool
	destFileName  string
	SyncMongoConfig
}

func NewSyncMongo(app *core.App, syncer *store.Syncer, config SyncMongoConfig) (SyncTask, error) {
	useConfigFile := false
	if !strings.HasPrefix(config.URI, "mongodb://") && !strings.HasPrefix(config.URI, "mongodb+srv://") {
		if err := validateFilePath(config.URI, "mongo config"); err != nil {
			return nil, err
		}
		useConfigFile = true
	}

	if config.MongodumpPath != "" {
		if err := validateFilePath(config.MongodumpPath, "mongodump"); err != nil {
			return nil, err
		}
	} else {
		config.MongodumpPath = "mongodump"
	}

	destFileName := app.Name
	if config.Tag != "" {
		destFileName += "." + config.Tag
	}
	if config.EnableGzip {
		destFileName += ".gz"
	}

	return &syncMongo{
		app:             app,
		syncer:          syncer,
		SyncMongoConfig: config,
		useConfigFile:   useConfigFile,
		destFileName:    destFileName + core.BackupFileExt,
	}, nil
}

func (f *syncMongo) ExecSync() error {
	prefix := ""
	if f.Tag != "" {
		prefix = fmt.Sprintf("[%s]: ", f.Tag)
	}

	dest := filepath.Join(f.app.Config.BackupTempDir, f.destFileName)
	dumpArgs := []string{
		"--archive=" + dest,
	}
	if f.EnableGzip {
		dumpArgs = append(dumpArgs, "--gzip")
	}
	if f.useConfigFile {
		dumpArgs = append(dumpArgs, "--config", f.URI)
	} else {
		dumpArgs = append(dumpArgs, f.URI)
	}

	command := exec.CommandContext(f.app.Ctx, f.MongodumpPath, dumpArgs...)
	command.Stderr = os.Stderr
	pterm.Printf("%sCreating local backup: %s\n", prefix, f.destFileName)

	pterm.Debug.Printf("%sRemoving old local backup\n", prefix)
	_ = os.Remove(dest)

	start := time.Now()
	if err := command.Run(); err != nil {
		return errors.Wrapf(err, "error running mongodump")
	}
	pterm.Printf("%sLocal backup created took %s\n", prefix, time.Since(start).String())

	slog.Info(fmt.Sprintf("%sLocal backup created", prefix),
		slog.String("name", f.app.Name),
		slog.String("took", time.Since(start).String()))
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
