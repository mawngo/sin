package store

import (
	"context"
	"github.com/mawngo/go-errors"
	"github.com/pterm/pterm"
	"github.com/samber/lo"
	"log/slog"
	"path/filepath"
	"sin/internal/core"
	"sin/internal/utils"
	"slices"
	"strings"
	"time"
)

// Syncer sync local backup to remote, or pull backup from remote to local.
// Syncer instance is not thread safe.
type Syncer struct {
	adapters []Adapter

	failFast bool

	// iter backup iteration.
	iter int64

	// keep the last N backups.
	keep int

	// pullTargetDir the directory to pull backup to.
	pullTargetDir string
}

func NewSyncer(app *core.App) (*Syncer, error) {
	s := Syncer{
		keep:          app.Keep,
		failFast:      app.FailFast,
		adapters:      make([]Adapter, 0, len(app.Config.Targets)),
		pullTargetDir: app.BackupTempDir,
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
		case AdapterFileType:
			adapter, err := newFileAdapter(target)
			if err != nil {
				return nil, errors.Wrapf(err, "error creating file adapter %s", name)
			}
			s.adapters = append(s.adapters, adapter)
		case AdapterS3Type:
			adapter, err := newS3Adapter(target)
			if err != nil {
				return nil, errors.Wrapf(err, "error creating s3 adapter %s", name)
			}
			s.adapters = append(s.adapters, adapter)
		case AdapterMockType:
			adapter, err := newMockAdapter(target)
			if err != nil {
				return nil, errors.Wrapf(err, "error creating mock adapter %s", name)
			}
			s.adapters = append(s.adapters, adapter)
		default:
			return nil, errors.New("unknown type in config targets: " + t)
		}
	}
	return &s, nil
}

func (s *Syncer) AdaptersCount() int {
	return len(s.adapters)
}

func (s *Syncer) Sync(ctx context.Context, source string, start time.Time) error {
	if len(s.adapters) == 0 {
		return nil
	}

	filename := strings.TrimSuffix(filepath.Base(source), core.BackupFileExt)
	pterm.Printf("Start sync to %d destinations\n", len(s.adapters))
	errs := make([]error, 0, len(s.adapters))
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
		dest := start.Format("060102_1504_") + filename + core.BackupFileExt
		slog.Info("Start sync", slog.String("adapter", conf.Name), slog.String("filename", filename))

		// Send the file.
		// The adapter must handle retry if error happens.
		start := time.Now()
		err := adapter.Save(ctx, source, dest)
		if err != nil {
			// Only report instead of stop completely.
			pterm.Error.Println("Error syncing to", conf.Name, err)
			slog.Error("Error syncing",
				slog.String("adapter", conf.Name),
				slog.String("filename", filename),
				slog.Any("err", err))
			errs = append(errs, errors.Wrapf(err, "error syncing %s", conf.Name))
			continue
		}
		pterm.Success.Println("Synced to", conf.Name, "took", time.Since(start).String())
		slog.Info("Complete sync",
			slog.String("adapter", conf.Name),
			slog.String("filename", filename),
			slog.String("took", time.Since(start).String()))
		successes = append(successes, adapter)
	}

	if len(successes) == 0 {
		slog.Warn("All sync failed/skipped")
		pterm.Warning.Println("All sync failed/skipped")
		if s.failFast && len(errs) > 0 {
			return errors.Join(errs...)
		}
		return nil
	}

	// Compacting.
	s.iter++
	for _, adapter := range successes {
		if err := s.compact(ctx, adapter, filename); err != nil {
			errs = append(errs, errors.Wrapf(err, "error compacting %s", adapter.Config().Name))
			// Currently we ignore compact error as it is not critical, and compact can be run again next sync.
			// But if the error happens continuously, it could be a problem.
			pterm.Warning.Printf("Error compacting %s: %s\n", adapter.Config().Name, err)
			slog.Warn("Error compacting",
				slog.String("adapter", adapter.Config().Name),
				slog.Any("err", err))
		}
	}
	pterm.Println("Synced to", len(successes), "destinations")
	if s.failFast {
		return errors.Join(errs...)
	}
	return nil
}

func (s *Syncer) List(ctx context.Context, filename string, adapterNames ...string) error {
	if len(s.adapters) == 0 {
		return errors.New("empty list of targets")
	}
	filename = strings.TrimSuffix(filename, core.BackupFileExt)

	errs := make([]error, 0, len(s.adapters))
	for _, adapter := range s.adapters {
		if len(adapterNames) > 0 && !slices.Contains(adapterNames, adapter.Config().Name) {
			continue
		}

		conf := adapter.Config()
		names, err := adapter.ListFileNames(ctx)
		total := len(names)
		names = utils.FilterBackupFileNames(names, filename)
		backups := len(names)
		pterm.Info.Println("Files in", conf.Name, pterm.Sprintf("(%d/%d)", backups, total))
		if err != nil {
			pterm.Warning.Println("Error listing", conf.Name, err)
			errs = append(errs, errors.Wrapf(err, "error listing %s", conf.Name))
			if s.failFast {
				return errors.Join(errs...)
			}
			continue
		}
		items := lo.Map(names, func(item string, _ int) pterm.BulletListItem {
			return pterm.BulletListItem{Level: 0, Text: item}
		})
		errs = append(errs, pterm.DefaultBulletList.WithItems(items).Render())
	}
	pterm.Println("Completed.")
	return errors.Join(errs...)
}

// compact deletes old backup to keep the total number of backup bellows Keep config.
func (s *Syncer) compact(ctx context.Context, adapter Adapter, filename string) error {
	conf := adapter.Config()
	keep := adapter.Config().Keep
	if keep == 0 {
		keep = s.keep
	}
	if keep < 1 {
		slog.Info("Skip delete old backup due to config",
			slog.String("adapter", conf.Name),
			slog.String("filename", filename),
			slog.Int("keep", keep))
		return nil
	}

	names, err := adapter.ListFileNames(ctx)
	if err != nil {
		return errors.Wrapf(err, "error listing file names for destinations %s", conf.Name)
	}
	names = utils.FilterBackupFileNames(names, filename)
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
			return errors.Wrapf(err, "error deleting old backup")
		}
	}
	return nil
}
