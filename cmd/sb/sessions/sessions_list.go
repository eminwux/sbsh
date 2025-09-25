/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package sessions

import (
	"context"
	"log/slog"
	"os"
	"sbsh/pkg/discovery"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// sessionsCmd represents the sessions command
var sessionsListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"l"},
	Short:   "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:

Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		slog.Debug("-> sessions list")

		// get env SBSH_RUN_DIR
		// scan run dir for existing sessions
		// read status file
		// return results

		ctx := context.Background()
		return discovery.ScanAndPrintSessions(ctx, viper.GetString("global.runPath"), os.Stdout)
	},
}

func init() {
	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// sessionsCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// sessionsCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
