package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/marciniwanicki/crabby/internal/client"
	"github.com/spf13/cobra"
)

var (
	verbose bool
	quiet   bool
)

func chatCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "chat",
		Short: "Start interactive chat",
		Long:  "Start an interactive REPL mode for chatting with the AI.",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client.NewClient(port)
			ctx := context.Background()

			// Determine verbosity
			verbosity := client.VerbosityNormal
			if quiet {
				verbosity = client.VerbosityQuiet
			} else if verbose {
				verbosity = client.VerbosityVerbose
			}

			opts := client.ChatOptions{
				Verbosity: verbosity,
			}

			// Check if daemon is running
			if !c.IsRunning(ctx) {
				return fmt.Errorf("daemon is not running. Start it with: crabby daemon")
			}

			// Interactive REPL mode
			return runREPL(ctx, c, opts)
		},
	}

	cmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "Show tool call details and results")
	cmd.Flags().BoolVarP(&quiet, "quiet", "q", false, "Only show assistant responses (hide tool info)")

	return cmd
}

func runREPL(ctx context.Context, c *client.Client, opts client.ChatOptions) error {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("Crabby REPL - Type 'exit' or Ctrl+C to quit")
	fmt.Println()

	for {
		fmt.Print("❯ ")
		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		if input == "exit" || input == "quit" {
			fmt.Println("Goodbye!")
			break
		}

		if err := c.Chat(ctx, input, os.Stdout, opts); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		fmt.Println()
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}
