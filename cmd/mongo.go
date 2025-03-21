package cmd

import (
	"errors"
	"fmt"
	"github.com/pterm/pterm"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
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

func NewMongoCmd(app *core.App) *cobra.Command {
	command := cobra.Command{
		Use:   "mongo <uri/config file>",
		Args:  cobra.ExactArgs(1),
		Short: "Run backup for mongo using mongodump",
		Run: func(cmd *cobra.Command, args []string) {
			uri := args[0]
			useConfigFile := false
			if !strings.HasPrefix(uri, "mongodb://") && !strings.HasPrefix(uri, "mongodb+srv://") {
				if stats, err := os.Stat(uri); err != nil || stats.IsDir() {
					pterm.Error.Println("Not a valid mongodump config file")
					slog.Error("Fatal error invalid mongo config file",
						slog.String("file", uri))
					return
				}
				useConfigFile = true
			}

			syncher, err := store.NewSyncer(app)
			if err != nil {
				pterm.Error.Println("Error initialize syncer:", err)
				slog.Error("Fatal error initialize syncer",
					slog.String("name", app.Name),
					slog.Any("err", err))
				return
			}

			mongodump := lo.Must(cmd.Flags().GetString("mongodump"))
			enableGzip := lo.Must(cmd.Flags().GetBool("gzip"))

			destFileName := app.Name
			if enableGzip {
				destFileName += ".gz" + core.BackupFileExt
			} else {
				destFileName += core.BackupFileExt
			}

			dest := filepath.Join(app.Config.BackupTempDir, destFileName)
			dumpArgs := []string{
				"--archive=" + dest,
			}
			if enableGzip {
				dumpArgs = append(dumpArgs, "--gzip")
			}
			if useConfigFile {
				dumpArgs = append(dumpArgs, "--config", uri)
			} else {
				dumpArgs = append(dumpArgs, uri)
			}

			err = core.Run(app.Ctx, app.Config.Frequency, func() error {
				command := exec.CommandContext(app.Ctx, mongodump, dumpArgs...)
				command.Stderr = os.Stderr
				pterm.Println("Creating local backup:")

				pterm.Debug.Println("Removing old local backup")
				_ = os.Remove(dest)

				start := time.Now()
				if err := command.Run(); err != nil {
					return fmt.Errorf("error running mongodump %w", err)
				}
				pterm.Println("Local backup created took", time.Since(start).String())
				slog.Info("Local backup created", slog.String("name", app.Name), slog.String("took", time.Since(start).String()))
				if syncher.AdaptersCount() == 0 {
					pterm.Println("Local backup are kept as there are no targets configured")
					return utils.CreateFileSHA256Checksum(dest)
				}
				err := syncher.Sync(app.Ctx, dest, start)
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
	command.Flags().String("mongodump", "mongodump", "mongodump command/binary location")
	command.Flags().Bool("gzip", false, "enable gzip compression")
	return &command
}
