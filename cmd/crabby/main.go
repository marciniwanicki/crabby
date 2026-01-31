package main

import (
	"os"

	"github.com/spf13/cobra"
)

var (
	// Global flags
	port      int
	ollamaURL string
	model     string
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "crabby",
		Short: "Personal AI CLI with daemon architecture",
		Long:  "Crabby is a CLI tool for personal AI interactions, connecting to a local Qwen 2.5 model via Ollama.",
	}

	// Global flags
	rootCmd.PersistentFlags().IntVar(&port, "port", 8787, "Daemon listen port")
	rootCmd.PersistentFlags().StringVar(&ollamaURL, "ollama-url", "http://localhost:11434", "Ollama API endpoint")
	rootCmd.PersistentFlags().StringVar(&model, "model", "qwen2.5:14b", "Model to use for chat")

	// Add subcommands
	rootCmd.AddCommand(daemonCmd())
	rootCmd.AddCommand(chatCmd())
	rootCmd.AddCommand(statusCmd())
	rootCmd.AddCommand(stopCmd())

	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
