package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "hamr",
	Short: "hamr — Build MCP servers in Go with minimal boilerplate",
	Long: `hamr is a high-level framework for building MCP (Model Context Protocol) servers in Go.
It auto-generates JSON schemas, handles transport, validation, and middleware.`,
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(validateCmd)
	rootCmd.AddCommand(devCmd)
	rootCmd.AddCommand(versionCmd)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print hamr version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("hamr v0.1.0")
	},
}
