package cmd

import (
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"log/slog"
	"sin/internal/core"
	"sin/internal/store"
	"sin/internal/task"
)

func NewMongoCmd(app *core.App) *cobra.Command {
	flags := task.SyncMongoConfig{
		MongodumpPath: "mongodump",
		EnableGzip:    false,
	}

	command := cobra.Command{
		Use:   "mongo <uri/config file>",
		Args:  cobra.ExactArgs(1),
		Short: "Run backup for mongo using mongodump",
		Run: func(_ *cobra.Command, args []string) {
			syncer, err := store.NewSyncer(app)
			if err != nil {
				pterm.Error.Println("Error initialize syncer:", err)
				slog.Error("Fatal error initialize syncer",
					slog.String("name", app.Name),
					slog.Any("err", err))
				return
			}

			flags.URI = args[0]
			syncTask, err := task.NewSyncMongo(app, syncer, "", flags)
			if err != nil {
				pterm.Error.Println("Error initialize mongo task:", err)
				slog.Error("Fatal error initialize mongo task",
					slog.String("name", app.Name),
					slog.Any("err", err))
				return
			}

			if err := core.Run(app.Ctx, app.Config.Frequency, syncTask.ExecSync); err != nil {
				pterm.Error.Println(err)
				slog.Error("Fatal error running", slog.String("name", app.Name), slog.Any("err", err))
			}
		},
	}
	command.Flags().StringVar(&flags.MongodumpPath, "mongodump", flags.MongodumpPath, "mongodump command/binary location")
	command.Flags().BoolVar(&flags.EnableGzip, "gzip", flags.EnableGzip, "enable gzip compression")
	return &command
}
