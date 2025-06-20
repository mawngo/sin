package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/getsentry/sentry-go"
	"github.com/go-viper/mapstructure/v2"
	"github.com/mawngo/go-errors"
	"github.com/pterm/pterm"
	slogmulti "github.com/samber/slog-multi"
	slogsentry "github.com/samber/slog-sentry/v2"
	"github.com/spf13/viper"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"
)

type AppInitConfig struct {
	ConfigFile   string
	Name         string
	AutomaticEnv bool
	FailFast     bool
	Keep         int
	NoMkdir      bool
}

type App struct {
	Ctx context.Context
	Config
	Revision string

	cancel       context.CancelFunc
	logFile      *os.File
	nameLockPath string
}

type Config struct {
	Name      string `json:"name"`
	SentryDSN string `json:"sentryDSN"`

	FailFast bool `json:"failFast"`
	// BackupTempDir the directory for storing created backup.
	BackupTempDir string `json:"backupTempDir"`
	// KeepTempFile does not remove recently created backup after sync.
	KeepTempFile bool `json:"keepTempFile"`

	// Number of backups to keep.
	Keep int `json:"keep"`

	// Frequency of the backup process.
	// Support cron and duration string.
	// If not specified, run once and stop.
	Frequency string `json:"frequency"`

	Targets []map[string]any `json:"targets"`
}

// Init setup application core.
func (app *App) Init(c AppInitConfig) error {
	app.Config = Config{
		Keep: -1,
	}
	app.Revision = loadRevision()
	app.Ctx, app.cancel = context.WithCancel(context.Background())
	if err := loadJSONConfigInto(&app.Config, c.ConfigFile, c.AutomaticEnv); err != nil {
		return err
	}
	if c.Name != "" {
		app.Name = c.Name
	}
	if app.Name == "" {
		app.Name = DefaultAppName
	}
	if c.FailFast {
		app.FailFast = c.FailFast
	}
	if c.Keep > 0 {
		app.Keep = c.Keep
	}
	if app.BackupTempDir == "" {
		app.BackupTempDir = "."
	}

	if err := setupLogging(app); err != nil {
		return err
	}

	if c.NoMkdir {
		if info, err := os.Stat(app.BackupTempDir); err != nil || !info.IsDir() {
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return errors.Wrapf(err, "error checking backup temp dir %s", app.BackupTempDir)
			}
			if info != nil && !info.IsDir() {
				return errors.New("backup temp dir is not a directory " + app.BackupTempDir)
			}
		}
	} else {
		if err := os.MkdirAll(app.BackupTempDir, os.ModePerm); err != nil {
			return err
		}
	}

	// Handle the lock file.
	nameLockPath := filepath.Join(os.TempDir(), app.Name+".sinnamelock")
	if _, err := os.Stat(nameLockPath); err == nil {
		// Multi instance running with the same name can cause trouble if the user is not careful enough.
		// So we forbid them from the start.
		pterm.Error.Println("Another instance of sin is running under the same name: ", app.Name)
		pterm.Error.Println("Please use different --name")
		pterm.Info.Println("If there are no other instance of sin running, this could be caused by improper shutdown of previous instance.")
		pterm.Info.Println("In that case, please remove the lock file: ", nameLockPath)
		err := errors.New("multiple instance running with same name")
		slog.Error("Error initializing", slog.Any("err", err))
		return err
	}
	f, err := os.Create(nameLockPath)
	if err != nil {
		err := errors.Wrapf(err, "cannot create lock file")
		slog.Error("Error initializing", slog.Any("err", err))
		return err
	}
	defer f.Close()
	app.nameLockPath = nameLockPath

	if app.Config.SentryDSN != "" {
		// Make sure we can connect to sentry.
		slog.Warn("Ping sentry", slog.String("status", "initialized"))
	}
	// Make sure slog logger work.
	slog.Info("Initialized",
		slog.String("name", app.Name),
		slog.String("revision", app.Revision),
		slog.Bool("env", c.AutomaticEnv))
	return nil
}

// Close handle cleanup when shutdown.
func (app *App) Close() error {
	if app.Ctx != nil {
		defer app.cancel()
	}
	if app.nameLockPath != "" {
		err := os.Remove(app.nameLockPath)
		if err != nil {
			pterm.Error.Println("Error removing lock file", app.nameLockPath, err)
			slog.Error("Error shutdown",
				slog.String("path", app.nameLockPath),
				slog.Any("err", errors.Wrapf(err, "cannot remove lock file")),
			)
		}
	}
	if app.SentryDSN != "" {
		sentry.Flush(5 * time.Second)
	}
	if app.logFile != nil {
		return app.logFile.Close()
	}
	return nil
}

func (app *App) MustClose() {
	if err := app.Close(); err != nil {
		pterm.Error.Println(err)
	}
}

func setupLogging(app *App) error {
	f, err := os.OpenFile(fmt.Sprintf("%s%s", app.Name, LogFileExt), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return errors.Wrapf(err, "error opening log file")
	}

	handler := slog.NewJSONHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo})
	app.logFile = f
	if app.SentryDSN == "" {
		slog.SetDefault(slog.New(handler))
		return nil
	}

	err = sentry.Init(sentry.ClientOptions{
		Dsn:           app.SentryDSN,
		Release:       app.Name + "@" + app.Revision,
		EnableTracing: false,
	})
	if err != nil {
		return errors.Wrapf(err, "error initializing sentry")
	}

	slog.SetDefault(slog.New(
		slogmulti.Fanout(
			handler,
			slogsentry.Option{Level: slog.LevelWarn}.NewSentryHandler(),
		),
	))
	return nil
}

func loadRevision() string {
	revision := "unknown"
	info, ok := debug.ReadBuildInfo()
	if !ok {
		pterm.Warning.Println("Cannot read build info")
	} else {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" {
				revision = s.Value
			}
		}
	}
	pterm.Println("Revision: " + revision)
	return revision
}

func loadJSONConfigInto(cfg *Config, path string, automaticEnv bool) error {
	cfgJSONBytes, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	viper.SetConfigType("json")
	if automaticEnv {
		viper.SetEnvKeyReplacer(strings.NewReplacer(`.`, `__`))
		viper.AutomaticEnv()
	}

	// Load default required keys from struct.
	if err := viper.ReadConfig(bytes.NewReader(cfgJSONBytes)); err != nil {
		return err
	}

	if path != "" {
		// Load core file.
		viper.SetConfigFile(path)
		if err := viper.MergeInConfig(); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return errors.New("config file not found")
			}
			return err
		}
		err = viper.Unmarshal(cfg, func(config *mapstructure.DecoderConfig) {
			config.TagName = "json"
			config.Squash = true
		})
		if err != nil {
			return err
		}
	} else {
		pterm.Warning.Println("No config file specified via --config")
		if !automaticEnv {
			return errors.New("must enable automatic env if not specify a config file")
		}
	}
	return nil
}
