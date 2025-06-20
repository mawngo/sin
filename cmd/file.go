package cmd

import (
	"github.com/pterm/pterm"
	"github.com/spf13/cobra"
	"log/slog"
	"sin/internal/core"
	"sin/internal/store"
	"sin/internal/task"
)

func NewFileCmd(app *core.App) *cobra.Command {
	command := cobra.Command{
		Use:   "file <path>",
		Args:  cobra.ExactArgs(1),
		Short: "Run backup for file/directory",
		Run: func(_ *cobra.Command, args []string) {
			syncer, err := store.NewSyncer(app)
			if err != nil {
				pterm.Error.Println("Error initialize syncer:", err)
				slog.Error("Fatal error initialize syncer",
					slog.String("name", app.Name),
					slog.Any("err", err))
				return
			}

			syncTask, err := task.NewSyncFile(app, syncer, "", args[0])
			if err != nil {
				pterm.Error.Println("Error initialize file task:", err)
				slog.Error("Fatal error initialize file task",
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
	return &command
}
