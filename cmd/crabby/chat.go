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

func chatCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "chat [message]",
		Short: "Send a message or start interactive chat",
		Long:  "Send a single message to the AI, or start an interactive REPL mode if no message is provided.",
		RunE: func(cmd *cobra.Command, args []string) error {
			c := client.NewClient(port)
			ctx := context.Background()

			// Check if daemon is running
			if !c.IsRunning(ctx) {
				return fmt.Errorf("daemon is not running. Start it with: crabby daemon")
			}

			if len(args) > 0 {
				// One-shot mode: send message and exit
				message := strings.Join(args, " ")
				return c.Chat(ctx, message, os.Stdout)
			}

			// Interactive REPL mode
			return runREPL(ctx, c)
		},
	}
}

func runREPL(ctx context.Context, c *client.Client) error {
	scanner := bufio.NewScanner(os.Stdin)
	fmt.Println("Crabby REPL - Type 'exit' or Ctrl+C to quit")
	fmt.Println()

	for {
		fmt.Print("> ")
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

		if err := c.Chat(ctx, input, os.Stdout); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		fmt.Println()
	}

	if err := scanner.Err(); err != nil {
		return err
	}

	return nil
}
