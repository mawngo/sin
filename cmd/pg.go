package cmd

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"github.com/pterm/pterm"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sin/internal/core"
	"sin/internal/store"
	"strings"
	"time"
)

func NewPGCmd(app *core.App) *cobra.Command {
	command := cobra.Command{
		Use:   "pg <uri>",
		Args:  cobra.ExactArgs(1),
		Short: "Run backup for postgres using pg_dump",
		Run: func(cmd *cobra.Command, args []string) {
			uri := args[0]
			if !strings.HasPrefix(uri, "postgresql://") {
				pterm.Error.Println("Invalid connection string uri")
				slog.Error("Fatal error invalid pg connection string")
			}

			syncher, err := store.NewSyncer(app)
			if err != nil {
				pterm.Error.Println("Error initialize syncer:", err)
				slog.Error("Fatal error initialize syncer",
					slog.String("name", app.Name),
					slog.Any("err", err))
				return
			}

			pgdump := lo.Must(cmd.Flags().GetString("pg_dump"))
			enableGzip := lo.Must(cmd.Flags().GetBool("gzip"))

			destFileName := app.Name
			if enableGzip {
				destFileName += ".gz" + core.BackupFileExt
			} else {
				destFileName += core.BackupFileExt
			}

			dest := filepath.Join(app.Config.BackupTempDir, destFileName)
			dumpArgs := []string{"-d", uri}

			err = core.Run(app.Ctx, app.Config.Frequency, func() error {
				pterm.Println("Creating local backup")

				start := time.Now()
				pterm.Debug.Println("Truncating old local backup")
				f, err := os.Create(dest)
				if err != nil {
					return fmt.Errorf("error creating backup file %s: %w", dest, err)
				}

				command := exec.CommandContext(app.Ctx, pgdump, dumpArgs...)
				var stderr bytes.Buffer
				command.Stderr = &stderr
				var w io.Writer = f
				if enableGzip {
					gzw := gzip.NewWriter(w)
					defer gzw.Close()
					defer gzw.Flush()
					w = gzip.NewWriter(f)
				}

				out, err := command.StdoutPipe()
				if err != nil {
					return fmt.Errorf("error creating stdout pipe: %w", err)
				}
				if err := command.Start(); err != nil {
					return fmt.Errorf("error running command: %w", err)
				}
				if _, err := io.Copy(w, out); err != nil {
					return fmt.Errorf("error piping pg_dump output to file %s: %w", dest, err)
				}
				if err := command.Wait(); err != nil {
					msg := stderr.String()
					pterm.Error.Println(msg)
					return fmt.Errorf("error running pg_dump [%s]: %w", msg[:max(len(msg), 100)], err)
				}

				pterm.Println("Local backup created took", time.Since(start).String())
				slog.Info("Local backup created", slog.String("name", app.Name), slog.String("took", time.Since(start).String()))
				if syncher.AdaptersCount() == 0 {
					pterm.Println("Local backup are kept as there are no targets configured")
					return nil
				}
				err = syncher.Sync(app.Ctx, dest)
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
	command.Flags().String("pg_dump", "pg_dump", "pg_dump command/binary location")
	command.Flags().Bool("gzip", false, "enable gzip compression")
	return &command
}
