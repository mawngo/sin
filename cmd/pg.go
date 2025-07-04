package cmd

import (
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"log/slog"
	"sin/internal/core"
	"sin/internal/store"
	"sin/internal/task"
)

func NewPGCmd(app *core.App) *cobra.Command {
	flags := task.SyncPostgresConfig{
		PGDumpPath: "pg_dump",
		EnableGzip: false,
		Format:     "custom",
	}

	command := cobra.Command{
		Use:   "pg <uri/file>",
		Args:  cobra.ExactArgs(1),
		Short: "Run backup for postgres using pg_dump",
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
			syncTask, err := task.NewSyncPostgres(app, syncer, flags)
			if err != nil {
				pterm.Error.Println("Error initialize pg task:", err)
				slog.Error("Fatal error initialize pg task",
					slog.String("name", app.Name),
					slog.Any("err", err))
				return
			}

			if err := core.Run(app.Ctx, app.Config.Frequency, syncTask.ExecSync); err != nil {
				pterm.Error.Println(err)
				slog.Error("Fatal error running",
					slog.String("name", app.Name),
					slog.Any("err", err))
			}
		},
	}
	command.Flags().StringVar(&flags.PGDumpPath, "pg_dump", flags.PGDumpPath, "pg_dump command/binary location")
	command.Flags().BoolVar(&flags.EnableGzip, "gzip", flags.EnableGzip, "enable gzip compression")
	command.Flags().StringVar(&flags.Compress, "compress", flags.Compress, "specify compression algorithm or/and level")
	command.Flags().StringVar(&flags.Format, "format", flags.Format, "specify output format")
	command.Flags().IntVar(&flags.NumberOfJobs, "number-of-jobs", flags.NumberOfJobs, "specify number of concurrent jobs when output format is directory")
	return &command
}
