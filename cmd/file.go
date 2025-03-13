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
				slog.Error("Error invalid source file",
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
				slog.Error("Error initialize syncer",
					slog.String("name", app.Name),
					slog.Any("err", err))
				return
			}

			dest := filepath.Join(app.Config.BackupTempDir, filepath.Base(source)+core.BackupFileExt)
			err = core.Run(app.Ctx, app.Config.Frequency, func() error {
				pterm.Println("Creating backup")
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
				err := syncher.Sync(app.Ctx, dest)
				if !app.KeepTempFile {
					err = errors.Join(err, os.Remove(dest))
				}
				return err
			})

			if err != nil {
				pterm.Error.Println(err)
				slog.Error("Error running", slog.String("name", app.Name), slog.Any("err", err))
			}
		},
	}
	return &command
}

// TODO: again, review this zip code.
func zipDir(src, dst string) (err error) {
	file, err := os.Create(dst)
	if err != nil {
		panic(err)
	}
	defer file.Close()

	w := zip.NewWriter(file)
	defer w.Close()

	walker := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		// Ensure that `path` is not absolute; it should not start with "/".
		// This snippet happens to work because I don't use
		// absolute paths, but ensure your real-world code
		// transforms the path into a zip-root relative path.
		f, err := w.Create(path)
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
