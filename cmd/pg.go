package cmd

import (
	"compress/gzip"
	"github.com/mawngo/go-errors"
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
	"sin/internal/utils"
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
				destFileName += ".gz"
			}
			destFileName += core.BackupFileExt

			dest := filepath.Join(app.Config.BackupTempDir, destFileName)
			dumpArgs := []string{"-d", uri}

			err = core.Run(app.Ctx, app.Config.Frequency, func() error {
				pterm.Println("Creating local backup")

				pterm.Debug.Println("Truncating old local backup")
				f, err := os.Create(dest)
				if err != nil {
					return errors.Wrapf(err, "error creating backup file %s", dest)
				}

				command := exec.CommandContext(app.Ctx, pgdump, dumpArgs...)
				command.Stderr = os.Stderr
				var w io.Writer = f
				out, err := command.StdoutPipe()
				if err != nil {
					return errors.Wrapf(err, "error creating stdout pipe")
				}

				start := time.Now()
				err = (func() error {
					if enableGzip {
						gzw := gzip.NewWriter(w)
						defer gzw.Close()
						defer gzw.Flush()
						w = gzip.NewWriter(f)
					}
					if err := command.Start(); err != nil {
						return errors.Wrapf(err, "error running command")
					}
					if _, err := io.Copy(w, out); err != nil {
						return errors.Wrapf(err, "error piping pg_dump output to file %s", dest)
					}
					if err := command.Wait(); err != nil {
						return errors.Wrapf(err, "error running pg_dump")
					}
					return nil
				})()
				if err != nil {
					return err
				}

				pterm.Println("Local backup created took", time.Since(start).String())
				slog.Info("Local backup created", slog.String("name", app.Name), slog.String("took", time.Since(start).String()))
				if syncher.AdaptersCount() == 0 {
					pterm.Println("Local backup are kept as there are no targets configured")
					return utils.CreateFileSHA256Checksum(dest)
				}
				err = syncher.Sync(app.Ctx, dest, start)
				if !app.KeepTempFile {
					err = errors.Join(err, os.Remove(dest))
				} else {
					err = errors.Join(err, utils.CreateFileSHA256Checksum(dest))
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
