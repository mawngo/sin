package store

import (
	"context"
	"fmt"
	"github.com/mawngo/go-errors"
	"github.com/pterm/pterm"
	"github.com/samber/lo"
	"log/slog"
	"os"
	"path/filepath"
	"sin/internal/core"
	"sin/internal/utils"
	"slices"
	"strings"
	"time"
)

func (s *Syncer) Pull(ctx context.Context, filename string, adapterNames ...string) error {
	filename = strings.TrimSuffix(filename, core.BackupFileExt)

	if _, err := os.Stat(s.pullTargetDir); err != nil {
		if s.failFast {
			return errors.Wrapf(err, "cannot access local backup directory %s", s.pullTargetDir)
		}
		pterm.Error.Println("Cannot access local backup directory:", err.Error())
		slog.Error("Cannot access local backup directory",
			slog.String("target", s.pullTargetDir),
			slog.Any("err", err))
		return nil
	}

	pterm.Println("Pulling to", s.pullTargetDir)

	downloaders := lo.FilterMap(s.adapters, func(adapter Adapter, _ int) (Downloader, bool) {
		if len(adapterNames) > 0 && !slices.Contains(adapterNames, adapter.Config().Name) {
			return nil, false
		}
		d, ok := adapter.(Downloader)
		return d, ok
	})
	if len(downloaders) == 0 {
		return errors.New("empty list of downloadable targets")
	}

	pullableByDownloader := make(map[Downloader][]string, len(downloaders))
	availableDownloaderLeft := len(downloaders)
	start := time.Now()
	pulledCnt := 0
	errs := make([]error, 0, len(downloaders))
	for availableDownloaderLeft > 0 {
		names, err := utils.ListFileNames(s.pullTargetDir)
		if err != nil {
			pterm.Warning.Println("Cannot count number of pulled file:", err.Error())
			slog.Error("Cannot count number of pulled file", slog.String("filename", filename), slog.Any("err", err))
		}
		names = utils.FilterBackupFileNames(names, filename)
		toPull := 1
		if s.keep > 1 {
			toPull = max(s.keep-len(names), 1)
		}
		pulled := lo.SliceToMap(names, func(name string) (string, struct{}) {
			return name, struct{}{}
		})
		pterm.Printf("Pulling in progress %d pulled, expected %d more...\n", pulledCnt, toPull)

		// Start downloading.
		for _, downloader := range downloaders {
			if toPull == 0 {
				break
			}

			// Prepare a list of downloadable files.
			pullable, ok := pullableByDownloader[downloader]
			if !ok {
				var err error
				pullable, err = downloader.ListFileNames(ctx)
				if err != nil {
					pterm.Warning.Println("Cannot list file names for", downloader.Config().Name, ": ", err.Error())
					slog.Error("Cannot list file names", slog.String("adapter", downloader.Config().Name), slog.Any("err", err))
				}
				pullable = utils.FilterBackupFileNames(pullable, filename)
				pullableByDownloader[downloader] = pullable
			}

			if len(pullable) == 0 {
				availableDownloaderLeft--
				continue
			}

			for i := len(pullable) - 1; i >= 0; i-- {
				file := pullable[i]
				pullable = append(pullable[:i], pullable[i+1:]...)
				pullableByDownloader[downloader] = pullable

				// If the number of files in local is greater than the number of files we want to keep,
				// then we only fetch the newer file.
				// So if the latest file is not newer than our current latest file,
				// we should skip this downloader completely.
				if len(pulled) >= s.keep && len(names) > 0 {
					if file <= names[len(names)-1] {
						pullableByDownloader[downloader] = nil
						availableDownloaderLeft--
						break
					}
				}

				if _, ok := pulled[file]; ok {
					continue
				}
				if err := s.pull(ctx, downloader, file); err == nil {
					toPull--
					pulledCnt++
					if toPull == 0 {
						break
					}
				}
			}
		}

		if toPull == 0 {
			break
		}
	}

	if pulledCnt == 0 {
		slog.Warn("All pull failed/skipped")
		pterm.Warning.Println("All sync failed/skipped")
		if s.failFast && len(errs) > 0 {
			return errors.Join(errs...)
		}
		return nil
	}

	// Compacting.
	if err := s.compactLocal(filename); err != nil {
		errs = append(errs, err)
		// Currently we ignore compact error as it is not critical, and compact can be run again next sync.
		// But if the error happens continuously, it could be a problem.
		pterm.Warning.Printf("Error compacting local: %s\n", err)
		slog.Warn("Error compacting local", slog.Any("err", err))
	}
	pterm.Println("Pulled to local", pulledCnt, "backups", "took", time.Since(start).String())
	if s.failFast {
		return errors.Join(errs...)
	}
	return nil
}

func (s *Syncer) pull(ctx context.Context, downloader Downloader, file string) error {
	start := time.Now()
	conf := downloader.Config()
	destination := filepath.Join(s.pullTargetDir, file)
	err := downloader.Download(ctx, destination, file)
	if err != nil {
		// Only report instead of stop completely.
		pterm.Error.Println("Error pull to local from", downloader.Config().Name, err)
		slog.Error("Error pulling",
			slog.String("adapter", conf.Name),
			slog.String("filename", file),
			slog.Any("err", err))
		return err
	}
	pterm.Success.Println("Pulled from", conf.Name, ":", file, "took", time.Since(start).String())
	slog.Info("Pulled",
		slog.String("adapter", conf.Name),
		slog.String("filename", file),
		slog.String("took", time.Since(start).String()))
	return nil
}

func (s *Syncer) compactLocal(filename string) error {
	if s.keep < 1 {
		slog.Info("Skip delete old pulled backup due to config",
			slog.String("filename", filename),
			slog.Int("keep", s.keep))
		return nil
	}
	names, err := utils.ListFileNames(s.pullTargetDir)
	if err != nil {
		return fmt.Errorf("error listing file names on local %s: %w", s.pullTargetDir, err)
	}
	names = utils.FilterBackupFileNames(names, filename)
	if len(names) <= s.keep {
		slog.Info("Skip delete old local backup",
			slog.String("filename", filename),
			slog.Int("count", len(names)))
		return nil
	}

	// Delete old backup.
	for _, name := range names[:len(names)-s.keep] {
		name = filepath.Join(s.pullTargetDir, name)
		slog.Info("Deleting old backup",
			slog.String("filename", filename),
			slog.String("target", name),
		)
		if err := utils.DelFile(name); err != nil {
			return fmt.Errorf("error deleting old backup: %w", err)
		}
	}
	return nil
}
