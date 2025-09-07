/*
Copyright © 2025 Emiliano Spinella (eminwux)
*/

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sbsh/pkg/client"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func main() {
	Execute()
}

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "sb",
	Short: "A brief description of your application",
	Long:  `A longer description ...`,
	Run: func(cmd *cobra.Command, args []string) {

		err := LoadConfig()
		if err != nil {
			fmt.Fprintln(os.Stderr, "Config error:", err)
			os.Exit(1)
		}
		run()

	},
}

func run() {
	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create a new Controller
	// var ctrl api.Controller

	_, err := client.NewController("/home/inwx/.sbsh/socket")
	if err != nil {
		log.Printf("[client] Supervisor not running: %v ", err)
		return
	} else {
		log.Printf("[client] Supervisor is running: %v ", err)

	}

}

///////////////////////////////////////////////

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

		} else {
			return err // Config file was found but another error was produced
		}
	}
	return nil
}
