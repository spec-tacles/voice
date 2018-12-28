package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "voice",
	Short: "A voice utility for Discord.",
}

func init() {
	rootCmd.AddCommand(connectCmd)
}

// Execute the CLI app
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
