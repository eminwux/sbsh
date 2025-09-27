package detach

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"sbsh/pkg/env"
	"sbsh/pkg/rpcclient/supervisor"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	supSocketPath string
)

// sessionsCmd represents the sessions command
var DetachCmd = &cobra.Command{
	Use:     "detach",
	Aliases: []string{"d"},
	Short:   "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:
    fmt.Fprintln(os.Stderr, "no supervisor socket found")
    os.Exit(1)
Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		slog.Debug("-> detach")

		// check SBSH_SUP_SOCKET

		socket := viper.GetString(env.SUP_SOCKET.ViperKey)
		if socket == "" {
			fmt.Fprintln(os.Stderr, "no supervisor socket found")
			os.Exit(1)
		}
		sup := supervisor.NewUnix(socket)
		defer sup.Close()

		ctx, cancel := context.WithTimeout(cmd.Context(), 3*time.Second)
		defer cancel()

		fmt.Fprintf(os.Stdout, "detaching..\r\n")
		if err := sup.Detach(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "detach failed: %v\r\n", err)
			os.Exit(1)

		}

		return nil
	},
}

func init() {
	flagS := "socket"
	DetachCmd.Flags().StringVar(&supSocketPath, flagS, "", "Supervisor Socket Path")

	_ = env.SUP_SOCKET.BindEnv()

	if err := viper.BindPFlag(env.SUP_SOCKET.ViperKey, DetachCmd.Flags().Lookup(flagS)); err != nil {
		slog.Debug("could not bind cobra flag to viper")
		log.Fatal(err)
	}

}
