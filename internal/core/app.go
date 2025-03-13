package core

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/getsentry/sentry-go"
	"github.com/mitchellh/mapstructure"
	"github.com/pterm/pterm"
	slogmulti "github.com/samber/slog-multi"
	slogsentry "github.com/samber/slog-sentry/v2"
	"github.com/spf13/viper"
	"io/fs"
	"log/slog"
	"os"
	"runtime/debug"
	"strings"
	"time"
)

type App struct {
	Config
	Revision string
	logFile  *os.File
}

type Config struct {
	Name      string `json:"name"`
	SentryDSN string `json:"sentryDSN"`

	// BackupTempDir the directory for storing created backup.
	BackupTempDir string `json:"tempDir"`
	// KeepTempFile does not remove recently created backup after sync.
	KeepTempFile bool `json:"keepTemp"`

	// Number of backups to keep.
	Keep int `json:"keep"`

	// Frequency of the backup process.
	// Support cron with format `cron:<cron>`.
	// If not specified, run once and stop.
	Frequency string `json:"frequency"`
}

func (app *App) Init(path string, name string, automaticEnv bool) error {
	app.Config = Config{
		Name:          name,
		Keep:          -1,
		BackupTempDir: ".",
	}
	app.Revision = loadRevision()
	if err := loadJSONConfigInto(&app.Config, path, automaticEnv); err != nil {
		return err
	}
	if err := setupLogging(app); err != nil {
		return err
	}
	// Make sure slog logger work.
	slog.Info("Initialized",
		slog.String("name", app.Name),
		slog.String("revision", app.Revision))
	return nil
}

func (app *App) Close() error {
	if app.SentryDSN != "" {
		sentry.Flush(5 * time.Second)
	}
	return app.logFile.Close()
}

func setupLogging(app *App) error {
	f, err := os.OpenFile(fmt.Sprintf("%s%s", app.Name, LogFileExt), os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		return fmt.Errorf("error opening log file: %w", err)
	}

	handler := slog.NewJSONHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo})
	app.logFile = f
	if app.SentryDSN == "" {
		slog.SetDefault(slog.New(handler))
		return nil
	}

	err = sentry.Init(sentry.ClientOptions{
		Dsn:           app.SentryDSN,
		EnableTracing: false,
	})
	if err != nil {
		return fmt.Errorf("error initializing sentry: %w", err)
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
