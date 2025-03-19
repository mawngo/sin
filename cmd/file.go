package cmd

import (
	"archive/zip"
	"errors"
	"fmt"
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sin/internal/core"
	"sin/internal/store"
	"sin/internal/utils"
	"strings"
	"time"
)

func NewFileCmd(app *core.App) *cobra.Command {
	command := cobra.Command{
		Use:   "file <path>",
		Args:  cobra.ExactArgs(1),
		Short: "Run backup for file/directory",
		Run: func(_ *cobra.Command, args []string) {
			source := args[0]
			isdir := false
			//nolint:revive
			if info, err := os.Stat(source); err != nil {
				pterm.Error.Println(err)
				slog.Error("Fatal error invalid source file",
					slog.String("name", app.Name),
					slog.String("source", source),
					slog.Any("err", err))
				return
			} else {
				isdir = info.IsDir()
			}

			syncher, err := store.NewSyncer(app)
			if err != nil {
				pterm.Error.Println("Error initialize syncer:", err)
				slog.Error("Fatal error initialize syncer",
					slog.String("name", app.Name),
					slog.Any("err", err))
				return
			}

			destFilename := app.Name
			if isdir {
				destFilename += ".zip"
			} else {
				_, extname, hasExt := strings.Cut(filepath.Base(source), ".")
				if hasExt {
					destFilename += "." + extname
				}
			}

			dest := filepath.Join(app.Config.BackupTempDir, destFilename+core.BackupFileExt)
			err = core.Run(app.Ctx, app.Config.Frequency, func() error {
				pterm.Println("Creating local backup")
				start := time.Now()
				if isdir {
					if err := zipDir(source, dest); err != nil {
						_ = os.Remove(dest)
						return fmt.Errorf("error creating backup %w", err)
					}
				} else {
					if err := utils.CopyFile(app.Ctx, source, dest); err != nil {
						_ = os.Remove(dest)
						return fmt.Errorf("error creating backup %w", err)
					}
				}
				pterm.Println("Local backup created took", time.Since(start).String())
				if syncher.AdaptersCount() == 0 {
					pterm.Println("Local backup are kept as there are no targets configured")
					return nil
				}
				err := syncher.Sync(app.Ctx, dest, start)
				if !app.KeepTempFile {
					err = errors.Join(err, os.Remove(dest))
				}
				return err
			})

			if err != nil {
				pterm.Error.Println(err)
				slog.Error("Fatal error running", slog.String("name", app.Name), slog.Any("err", err))
			}
		},
	}
	return &command
}

func zipDir(src, dst string) (err error) {
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
