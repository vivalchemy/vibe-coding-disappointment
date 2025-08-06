package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/vivalchemy/wake/internal/app"
	"github.com/vivalchemy/wake/internal/cli"
	"github.com/vivalchemy/wake/internal/config"
	"github.com/vivalchemy/wake/pkg/logger"
)

const (
	// ExitSuccess indicates successful execution
	ExitSuccess = 0
	// ExitError indicates an error occurred
	ExitError = 1
	// ExitInterrupt indicates the program was interrupted
	ExitInterrupt = 130
)

func main() {
	// Setup signal handling for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		cancel()
	}()

	// Run the application and exit with appropriate code
	os.Exit(run(ctx))
}

// run initializes and executes the Wake application
func run(ctx context.Context) int {
	// Initialize logger
	log := logger.New()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading configuration: %v\n", err)
		return ExitError
	}

	// Create application instance
	application, err := app.New(cfg, log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error initializing application: %v\n", err)
		return ExitError
	}

	// Initialize CLI
	rootCmd := cli.NewRootCommand(application)

	// Execute the command
	if err := rootCmd.ExecuteContext(ctx); err != nil {
		// Check if context was cancelled (interrupt)
		if ctx.Err() == context.Canceled {
			fmt.Fprintln(os.Stderr, "\nInterrupted")
			return ExitInterrupt
		}

		// Log error and return error exit code
		log.Error("Command execution failed", "error", err)
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return ExitError
	}

	return ExitSuccess
}
