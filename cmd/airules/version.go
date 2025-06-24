package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is the current version of airules (set at build time)
var Version = "dev"

// versionCmd represents the version command
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of airules",
	Long:  `Print the version of airules CLI tool.`,
	Run: func(_ *cobra.Command, _ []string) {
		fmt.Printf("airules version %s\n", Version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
