/*
Copyright © 2025 Emiliano Spinella (eminwux)
*/

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"sbsh/pkg/llm"
	"sbsh/pkg/tty"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var currentMode tty.Mode = tty.ModeSmart

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "sbsh",
	Short: "A brief description of your application",
	Long:  `A longer description ...`,
	Run: func(cmd *cobra.Command, args []string) {

		err := LoadConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Config error:", err)
			os.Exit(1)
		}
		apiKey := viper.GetString("openai_api_key")
		model := viper.GetString("openai_model")
		endpoint := viper.GetString("openai_endpoint")

		// Now you can call your Init function:
		err = llm.Init(apiKey, model, endpoint)
		if err != nil {
			fmt.Fprintln(os.Stderr, "Initialization error:", err)
			os.Exit(1)
		}

		if len(args) > 0 {
			need := strings.Join(args, " ")
			llm.Run(need)
			return
		}
		tty.Start(currentMode)
		// tty.ShowPrompt(currentMode, rl)
	},
}

func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

// LoadConfig loads config.yaml from the given path or HOME/.sbsh
func LoadConfig() error {
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")

	// Then ~/.sbsh
	home, err := os.UserHomeDir()
	if err == nil {
		viper.AddConfigPath(filepath.Join(home, ".sbsh"))
	}

	// Bind ENV variable as a fallback
	viper.BindEnv("openai_api_key", "OPENAI_API_KEY")

	if err := viper.ReadInConfig(); err != nil {
		// File not found is OK if ENV is set
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			if viper.GetString("openai_api_key") == "" {
				return fmt.Errorf("No config file found and OPENAI_API_KEY is not set")
			}
		} else {
			return err // Config file was found but another error was produced
		}
	}
	return nil
}
