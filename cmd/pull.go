package cmd

import (
	"github.com/pterm/pterm"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"log/slog"
	"sin/internal/core"
	"sin/internal/store"
)

func NewPullCmd(app *core.App) *cobra.Command {
	command := cobra.Command{
		Use:   "pull <target names...?>",
		Args:  cobra.MinimumNArgs(0),
		Short: "Pull remote backup to local",
		Run: func(cmd *cobra.Command, args []string) {
			syncher, err := store.NewSyncer(app)
			if err != nil {
				pterm.Error.Println("Error initialize puller:", err)
				slog.Error("Fatal error initialize puller",
					slog.String("name", app.Name),
					slog.Any("err", err))
				return
			}

			extension := lo.Must(cmd.Flags().GetString("ext"))
			destFileName := app.Name
			if extension == "*" {
				destFileName += "(.\\w+)?"
			} else if extension == "+" {
				destFileName += ".\\w+"
			} else if extension != "" {
				destFileName += "." + extension
			}
			destFileName += core.BackupFileExt

			err = core.Run(app.Ctx, app.Config.Frequency, func() error {
				return syncher.Pull(app.Ctx, destFileName, args...)
			})

			if err != nil {
				pterm.Error.Println(err)
				slog.Error("Fatal error running", slog.String("name", app.Name), slog.Any("err", err))
			}
		},
	}
	command.Flags().StringP("ext", "e", "*", "specify the extension of target file (without dot)")
	return &command
}
