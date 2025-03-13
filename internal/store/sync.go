package store

import (
	"context"
	"errors"
	"fmt"
	"github.com/pterm/pterm"
	"github.com/samber/lo"
	"log/slog"
	"path/filepath"
	"regexp"
	"sin/internal/core"
	"slices"
	"strings"
	"time"
)

// Syncer sync local backup to remote.
// Syncer instance is not thread safe.
type Syncer struct {
	adapters []Adapter

	// iter backup iteration.
	iter int64

	// keep the last N backups.
	keep int
}

func NewSyncer(app *core.App) (*Syncer, error) {
	s := Syncer{
		keep:     app.Keep,
		adapters: make([]Adapter, 0, len(app.Config.Targets)),
	}
	for _, target := range app.Targets {
		if raw, ok := target["disabled"]; ok {
			if v, ok := raw.(bool); ok && v {
				continue
			}
		}

		if raw, ok := target["type"]; !ok {
			return nil, errors.New("missing type in config targets")
		} else if _, ok := raw.(string); !ok {
			return nil, errors.New("type in config targets must be string")
		}

		if raw, ok := target["name"]; !ok {
			return nil, errors.New("missing name in config targets")
		} else if _, ok := raw.(string); !ok {
			return nil, errors.New("name in config targets must be string")
		}

		t := target["type"].(string)
		name := target["name"].(string)
		switch t {
		case "file":
			adapter, err := newFileAdapter(target)
			if err != nil {
				return nil, fmt.Errorf("error creating file adapter %s: %w", name, err)
			}
			s.adapters = append(s.adapters, adapter)
		case "s3":
		default:
			return nil, errors.New("unknown type in config targets: " + t)
		}
	}
	return &s, nil
}

func (s *Syncer) Sync(ctx context.Context, source string) error {
	if len(s.adapters) == 0 {
		return nil
	}

	filename := strings.TrimSuffix(filepath.Base(source), core.BackupFileExt)
	pterm.Printf("Start sync to %d destinations\n", len(s.adapters))
	successes := make([]Adapter, 0, len(s.adapters))
	for _, adapter := range s.adapters {
		conf := adapter.Config()
		if conf.Each > 1 && s.iter%int64(conf.Each) != 0 {
			slog.Info("Skip sync due to config",
				slog.String("adapter", conf.Name),
				slog.String("filename", filename),
				slog.Int("each", conf.Each))
			pterm.Success.Println("Skipped sync", conf.Name)
			continue
		}

		pterm.Debug.Println("Start sync to", conf.Name)
		dest := time.Now().Format("060102_1504_") + filename + core.BackupFileExt
		slog.Info("Start sync", slog.String("adapter", conf.Name), slog.String("filename", filename))

		// TODO: retry.
		err := adapter.Save(ctx, source, dest)
		if err != nil {
			// Only report instead of stop completely.
			pterm.Error.Println("Error syncing", err)
			slog.Error("Error syncing",
				slog.String("adapter", conf.Name),
				slog.String("filename", filename),
				slog.Any("err", err))
			continue
		}
		pterm.Success.Println("Synced to", conf.Name)
		successes = append(successes, adapter)
	}

	if len(successes) == 0 {
		return nil
	}
	s.iter++
	for _, adapter := range successes {
		if err := s.compact(ctx, adapter, filename); err != nil {
			// Currently we ignore compact error as it is not critical, and compact can be run again next sync.
			// But if the error happens continuously, it could be a problem.
			// TODO: handle error if it happens continuously on multiple sync.
			pterm.Warning.Printf("Error compacting %s: %s\n", adapter.Config().Name, err)
			slog.Warn("Error compacting",
				slog.String("adapter", adapter.Config().Name),
				slog.Any("err", err))
		}
	}
	pterm.Println("Synced to", len(successes), "destinations")
	return nil
}

// compact deletes old backup to keep the total number of backup bellows Keep config.
func (s *Syncer) compact(ctx context.Context, adapter Adapter, filename string) error {
	conf := adapter.Config()
	keep := max(s.keep, adapter.Config().Keep)
	if keep < 1 {
		slog.Info("Skip delete old backup due to config",
			slog.String("adapter", conf.Name),
			slog.String("filename", filename),
			slog.Int("keep", keep))
		return nil
	}

	names, err := adapter.ListFileNames(ctx)
	if err != nil {
		return fmt.Errorf("error listing file names for destinations %s: %w", conf.Name, err)
	}
	reg, err := regexp.Compile(fmt.Sprintf(`^\d{6}_\d{4}_%s%s$`, filename, core.BackupFileExt))
	if err != nil {
		return fmt.Errorf("error compiling regexp: %w", err)
	}
	names = lo.Filter(names, func(name string, _ int) bool {
		return reg.MatchString(name)
	})
	slices.Sort(names)
	if len(names) <= keep {
		slog.Info("Skip delete old backup",
			slog.String("adapter", conf.Name),
			slog.String("filename", filename),
			slog.Int("keep", keep),
			slog.Int("count", len(names)))
		return nil
	}

	// Delete old backup.
	for _, name := range names[:len(names)-keep] {
		slog.Info("Deleting old backup",
			slog.String("adapter", conf.Name),
			slog.String("filename", filename),
			slog.String("target", name),
		)
		if err := adapter.Del(ctx, name); err != nil {
			return fmt.Errorf("error deleting old backup: %w", err)
		}
	}
	return nil
}

// Adapter abstract storage adapter.
type Adapter interface {
	// Save saves a file to the storage, override if the file already exists.
	// If extra pathElems are given, pathElems will be joined.
	Save(ctx context.Context, source string, pathElem string, pathElems ...string) error

	// Del removes a file from the storage.
	// If extra pathElems are given, pathElems will be joined.
	// Throws error if the file is a directory.
	Del(ctx context.Context, pathElem string, pathElems ...string) error

	// ListFileNames return list of file names in the given path.
	// Return empty if not a directory, pathElems will be joined.
	ListFileNames(ctx context.Context, pathElems ...string) ([]string, error)

	Config() AdapterConfig
}

type AdapterConfig struct {
	Name string `json:"name"`

	// Disabled whether this adapter should be skipped.
	Disabled bool `json:"disabled"`

	// Keep override the Syncer Keep. Default 0 (using the Syncer Keep).
	Keep int `json:"keep"`

	// Each controls the number of actual syncs.
	// Default it will sync every backup.
	// If set to number n > 1, it will sync every nth backup.
	Each int `json:"each"`
}
