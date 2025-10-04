package attach

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sbsh/pkg/env"
	"sbsh/pkg/rpcclient/supervisor"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// sessionsCmd represents the sessions command
var AttachCmd = &cobra.Command{
	Use:     "attach",
	Aliases: []string{"a"},
	Short:   "A brief description of your command",
	Long: `A longer description that spans multiple lines and likely contains examples
and usage of using your command. For example:
    fmt.Fprintln(os.Stderr, "no supervisor socket found")
    os.Exit(1)
Cobra is a CLI library for Go that empowers applications.
This application is a tool to generate the needed files
to quickly create a Cobra application.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		slog.Debug("-> attach")

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

		fmt.Fprintf(os.Stdout, "attaching..\r\n")
		if err := sup.Detach(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "attach failed: %v\r\n", err)
			os.Exit(1)
		}

		return nil
	},
}

func init() {

}
