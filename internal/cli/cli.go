package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/vivalchemy/wake/internal/app"
	"github.com/vivalchemy/wake/internal/cli/commands"
	"github.com/vivalchemy/wake/internal/config"
	"github.com/vivalchemy/wake/pkg/logger"
	"github.com/vivalchemy/wake/pkg/task"
)

// CLI represents the command-line interface
type CLI struct {
	app        *app.App
	rootCmd    *cobra.Command
	logger     logger.Logger
	config     *config.Config
	globalOpts *commands.GlobalOptions
}

// NewCLI creates a new CLI instance
func NewCLI(application *app.App) (*CLI, error) {
	if application == nil {
		return nil, fmt.Errorf("application cannot be nil")
	}

	cli := &CLI{
		app:        application,
		logger:     application.Logger().WithGroup("cli"),
		config:     application.Config(),
		globalOpts: &commands.GlobalOptions{},
	}

	if err := cli.setupRootCommand(); err != nil {
		return nil, fmt.Errorf("failed to setup root command: %w", err)
	}

	if err := cli.setupCommands(); err != nil {
		return nil, fmt.Errorf("failed to setup commands: %w", err)
	}

	if err := cli.setupCompletion(); err != nil {
		return nil, fmt.Errorf("failed to setup completion: %w", err)
	}

	return cli, nil
}

// NewRootCommand creates the root command for external use
func NewRootCommand(application *app.App) *cobra.Command {
	cli, err := NewCLI(application)
	if err != nil {
		// Fallback to basic error command
		return &cobra.Command{
			Use:   "wake",
			Short: "Universal Task Runner",
			RunE: func(cmd *cobra.Command, args []string) error {
				return fmt.Errorf("failed to initialize CLI: %w", err)
			},
		}
	}

	return cli.rootCmd
}

// setupRootCommand configures the root command
func (c *CLI) setupRootCommand() error {
	c.rootCmd = &cobra.Command{
		Use:     "wake",
		Short:   "Universal Task Runner",
		Version: "0.1.0", // TODO: Get from build info

		// Global pre-run hook
		PersistentPreRunE: c.globalPreRun,

		// Default action: launch TUI
		RunE: c.defaultAction,

		// Disable automatic completion command
		CompletionOptions: cobra.CompletionOptions{
			DisableDefaultCmd: false,
		},

		// Silence usage on error
		SilenceUsage: true,

		// Custom help template
		Long: c.getLongDescription(),
	}

	// Add global flags
	c.addGlobalFlags()

	return nil
}

// setupCommands adds all subcommands
func (c *CLI) setupCommands() error {
	// List command
	listCmd, err := commands.NewListCommand(c.app, c.globalOpts)
	if err != nil {
		return fmt.Errorf("failed to create list command: %w", err)
	}
	c.rootCmd.AddCommand(listCmd)

	// Run command (handles direct task execution)
	runCmd, err := commands.NewRunCommand(c.app, c.globalOpts)
	if err != nil {
		return fmt.Errorf("failed to create run command: %w", err)
	}
	c.rootCmd.AddCommand(runCmd)

	// Dry run command
	dryCmd, err := commands.NewDryCommand(c.app, c.globalOpts)
	if err != nil {
		return fmt.Errorf("failed to create dry command: %w", err)
	}
	c.rootCmd.AddCommand(dryCmd)

	tuiCmd, err := commands.NewRootCommand(c.app, c.globalOpts)
	if err != nil {
		return fmt.Errorf("failed to create tui command: %w", err)
	}
	c.rootCmd.AddCommand(tuiCmd)

	// Config commands
	// TODO:
	// configCmd, err := commands.NewConfigCommand(c.app, c.globalOpts)
	// if err != nil {
	// 	return fmt.Errorf("failed to create config command: %w", err)
	// }
	// c.rootCmd.AddCommand(configCmd)
	//
	// // Cache commands
	// cacheCmd, err := commands.NewCacheCommand(c.app, c.globalOpts)
	// if err != nil {
	// 	return fmt.Errorf("failed to create cache command: %w", err)
	// }
	// c.rootCmd.AddCommand(cacheCmd)
	//
	// // Completion command
	// completionCmd, err := commands.NewCompletionCommand(c.app, c.globalOpts)
	// if err != nil {
	// 	return fmt.Errorf("failed to create completion command: %w", err)
	// }
	// c.rootCmd.AddCommand(completionCmd)

	return nil
}

// addGlobalFlags adds persistent flags available to all commands
func (c *CLI) addGlobalFlags() {
	flags := c.rootCmd.PersistentFlags()

	// Configuration flags
	flags.StringVarP(&c.globalOpts.ConfigFile, "config", "c", "",
		"config file (default is .wakeconfig.toml or ~/.config/wake/config.toml)")
	flags.StringVar(&c.globalOpts.LogLevel, "log-level", "info",
		"logging level (debug, info, warn, error)")
	flags.StringVar(&c.globalOpts.LogFormat, "log-format", "text",
		"logging format (text, json)")

	// Filtering flags
	flags.StringVar(&c.globalOpts.Runner, "runner", "",
		"filter by runner type (make, npm, yarn, pnpm, bun, just, task, python)")
	flags.BoolVar(&c.globalOpts.ShowHidden, "show-hidden", false,
		"show hidden tasks")
	flags.StringSliceVarP(&c.globalOpts.Tags, "tags", "t", nil,
		"filter by tags (comma-separated)")

	// Environment flags
	flags.StringSliceVarP(&c.globalOpts.EnvFiles, "env", "e", nil,
		"environment files to load (comma-separated)")
	flags.StringVarP(&c.globalOpts.WorkingDir, "directory", "d", "",
		"working directory (default is current directory)")

	// Output flags
	flags.StringVarP(&c.globalOpts.OutputFormat, "output", "o", "table",
		"output format (table, json, yaml, text)")
	flags.BoolVar(&c.globalOpts.NoColor, "no-color", false,
		"disable colored output")
	flags.BoolVarP(&c.globalOpts.Quiet, "quiet", "q", false,
		"quiet mode (minimal output)")
	flags.BoolVarP(&c.globalOpts.Verbose, "verbose", "v", false,
		"verbose mode (detailed output)")

	// Performance flags
	flags.IntVar(&c.globalOpts.MaxDepth, "max-depth", 10,
		"maximum directory depth for discovery")
	flags.BoolVar(&c.globalOpts.NoCache, "no-cache", false,
		"disable task discovery caching")
	flags.BoolVar(&c.globalOpts.Concurrent, "concurrent", true,
		"enable concurrent task discovery")

	// Debugging flags
	flags.BoolVar(&c.globalOpts.Debug, "debug", false,
		"enable debug mode")
	flags.BoolVar(&c.globalOpts.Profile, "profile", false,
		"enable profiling")
	flags.BoolVar(&c.globalOpts.DryRun, "dry-run", false,
		"show what would be executed without running")

	// Bind flags to viper for config file support
	c.bindFlags()
}

// bindFlags binds CLI flags to viper for configuration file support
func (c *CLI) bindFlags() {
	viper.BindPFlag("log_level", c.rootCmd.PersistentFlags().Lookup("log-level"))
	viper.BindPFlag("log_format", c.rootCmd.PersistentFlags().Lookup("log-format"))
	viper.BindPFlag("runner", c.rootCmd.PersistentFlags().Lookup("runner"))
	viper.BindPFlag("show_hidden", c.rootCmd.PersistentFlags().Lookup("show-hidden"))
	viper.BindPFlag("tags", c.rootCmd.PersistentFlags().Lookup("tags"))
	viper.BindPFlag("env_files", c.rootCmd.PersistentFlags().Lookup("env"))
	viper.BindPFlag("working_dir", c.rootCmd.PersistentFlags().Lookup("directory"))
	viper.BindPFlag("output_format", c.rootCmd.PersistentFlags().Lookup("output"))
	viper.BindPFlag("no_color", c.rootCmd.PersistentFlags().Lookup("no-color"))
	viper.BindPFlag("quiet", c.rootCmd.PersistentFlags().Lookup("quiet"))
	viper.BindPFlag("verbose", c.rootCmd.PersistentFlags().Lookup("verbose"))
	viper.BindPFlag("max_depth", c.rootCmd.PersistentFlags().Lookup("max-depth"))
	viper.BindPFlag("no_cache", c.rootCmd.PersistentFlags().Lookup("no-cache"))
	viper.BindPFlag("concurrent", c.rootCmd.PersistentFlags().Lookup("concurrent"))
	viper.BindPFlag("debug", c.rootCmd.PersistentFlags().Lookup("debug"))
	viper.BindPFlag("profile", c.rootCmd.PersistentFlags().Lookup("profile"))
	viper.BindPFlag("dry_run", c.rootCmd.PersistentFlags().Lookup("dry-run"))
}

// globalPreRun is executed before any command runs
func (c *CLI) globalPreRun(cmd *cobra.Command, args []string) error {
	// Load configuration file if specified
	if c.globalOpts.ConfigFile != "" {
		viper.SetConfigFile(c.globalOpts.ConfigFile)
		if err := viper.ReadInConfig(); err != nil {
			return fmt.Errorf("failed to read config file: %w", err)
		}
	}

	// Apply global options
	if err := c.applyGlobalOptions(); err != nil {
		return fmt.Errorf("failed to apply global options: %w", err)
	}

	// Change working directory if specified
	if c.globalOpts.WorkingDir != "" {
		if err := os.Chdir(c.globalOpts.WorkingDir); err != nil {
			return fmt.Errorf("failed to change directory to %s: %w", c.globalOpts.WorkingDir, err)
		}
	}

	// Initialize application if not done already
	if err := c.app.Initialize(); err != nil {
		return fmt.Errorf("failed to initialize application: %w", err)
	}

	return nil
}

// applyGlobalOptions applies global CLI options to the application
func (c *CLI) applyGlobalOptions() error {
	// Apply logging configuration
	if c.globalOpts.LogLevel != "" {
		if level, err := parseLogLevel(c.globalOpts.LogLevel); err == nil {
			c.app.Logger().SetLevel(level)
		} else {
			return fmt.Errorf("invalid log level: %s", c.globalOpts.LogLevel)
		}
	}

	// Apply debug mode
	if c.globalOpts.Debug {
		c.app.Logger().SetLevel(logger.DebugLevel)
	}

	// Apply quiet mode
	if c.globalOpts.Quiet {
		c.app.Logger().SetLevel(logger.ErrorLevel)
	}

	return nil
}

// parseLogLevel parses a log level string
func parseLogLevel(level string) (logger.Level, error) {
	switch strings.ToLower(level) {
	case "debug":
		return logger.DebugLevel, nil
	case "info":
		return logger.InfoLevel, nil
	case "warn", "warning":
		return logger.WarnLevel, nil
	case "error":
		return logger.ErrorLevel, nil
	case "fatal":
		return logger.FatalLevel, nil
	default:
		return logger.InfoLevel, fmt.Errorf("unknown log level: %s", level)
	}
}

// defaultAction is the default action when no subcommand is specified
func (c *CLI) defaultAction(cmd *cobra.Command, args []string) error {
	// If arguments are provided, treat as task execution
	if len(args) > 0 {
		return c.executeTask(cmd.Context(), args[0], args[1:])
	}

	// Otherwise, launch TUI
	return c.launchTUI(cmd.Context())
}

// executeTask executes a task directly from the command line
func (c *CLI) executeTask(ctx context.Context, taskName string, taskArgs []string) error {
	c.logger.Debug("Executing task from CLI", "task", taskName, "args", taskArgs)

	// Wait for task discovery to complete
	<-c.app.WaitForDiscovery()

	// Find the task
	tasks := c.app.GetTasks()
	var targetTask *task.Task

	for _, t := range tasks {
		if t.Name == taskName || t.ID == taskName {
			targetTask = t
			break
		}

		// Check aliases
		for _, alias := range t.Aliases {
			if alias == taskName {
				targetTask = t
				break
			}
		}

		if targetTask != nil {
			break
		}
	}

	if targetTask == nil {
		return fmt.Errorf("task not found: %s", taskName)
	}

	// Handle dry run
	if c.globalOpts.DryRun {
		return c.showDryRun(targetTask, taskArgs)
	}

	// Prepare environment
	env := make(map[string]string)
	if err := c.loadEnvironmentFiles(env); err != nil {
		return fmt.Errorf("failed to load environment files: %w", err)
	}

	// Execute the task
	c.logger.Info("Executing task", "task", targetTask.Name, "id", targetTask.ID)

	if err := c.app.RunTask(targetTask.ID, env); err != nil {
		return fmt.Errorf("failed to execute task: %w", err)
	}

	// Wait for task completion
	return c.waitForTaskCompletion(ctx, targetTask.ID)
}

// launchTUI launches the terminal user interface
func (c *CLI) launchTUI(ctx context.Context) error {
	// TUI implementation will be in the tui package
	// For now, return an error indicating TUI is not implemented
	return fmt.Errorf("TUI not yet implemented - use 'wake list' to see available tasks")
}

// showDryRun shows what would be executed without actually running
func (c *CLI) showDryRun(task *task.Task, args []string) error {
	fmt.Printf("Task: %s\n", task.Name)
	fmt.Printf("Runner: %s\n", task.Runner)
	fmt.Printf("Command: %s\n", task.Command)

	if len(task.Args) > 0 {
		fmt.Printf("Args: %s\n", strings.Join(task.Args, " "))
	}

	if len(args) > 0 {
		fmt.Printf("Additional Args: %s\n", strings.Join(args, " "))
	}

	fmt.Printf("Working Directory: %s\n", task.WorkingDirectory)

	if len(task.Environment) > 0 {
		fmt.Println("Environment:")
		for key, value := range task.Environment {
			fmt.Printf("  %s=%s\n", key, value)
		}
	}

	return nil
}

// loadEnvironmentFiles loads environment variables from files
func (c *CLI) loadEnvironmentFiles(env map[string]string) error {
	for _, envFile := range c.globalOpts.EnvFiles {
		if err := c.loadEnvironmentFile(envFile, env); err != nil {
			return fmt.Errorf("failed to load environment file %s: %w", envFile, err)
		}
	}
	return nil
}

// loadEnvironmentFile loads a single environment file
func (c *CLI) loadEnvironmentFile(filename string, env map[string]string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse KEY=VALUE
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			c.logger.Warn("Invalid environment line", "file", filename, "line", i+1, "content", line)
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes if present
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		env[key] = value
	}

	return nil
}

// waitForTaskCompletion waits for a task to complete
func (c *CLI) waitForTaskCompletion(ctx context.Context, taskID string) error {
	taskEvents := c.app.GetTaskEvents()
	outputEvents := c.app.GetOutputEvents()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case event := <-taskEvents:
			if event.Task.ID == taskID {
				switch event.Type {
				case app.TaskEventFinished:
					c.logger.Info("Task completed successfully", "task", taskID)
					return nil
				case app.TaskEventFailed:
					c.logger.Error("Task failed", "task", taskID, "error", event.Error)
					return fmt.Errorf("task failed: %v", event.Error)
				}
			}

		case output := <-outputEvents:
			if output.TaskID == taskID {
				// Print task output to stdout/stderr
				if output.IsErr {
					fmt.Fprint(os.Stderr, output.Line)
				} else {
					fmt.Print(output.Line)
				}
			}
		}
	}
}

// setupCompletion sets up shell completion
func (c *CLI) setupCompletion() error {
	// Add completion for task names
	c.rootCmd.ValidArgsFunction = c.completeTaskNames

	return nil
}

// completeTaskNames provides completion for task names
func (c *CLI) completeTaskNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Get available tasks
	tasks := c.app.GetTasks()
	var completions []string

	for _, task := range tasks {
		if strings.HasPrefix(task.Name, toComplete) {
			description := task.Description
			if description == "" {
				description = fmt.Sprintf("%s task", task.Runner)
			}
			completions = append(completions, fmt.Sprintf("%s\t%s", task.Name, description))
		}
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}

// getLongDescription returns the long description for the root command
func (c *CLI) getLongDescription() string {
	return `Wake is a universal task runner that provides a unified interface 
for various build tools and task runners (make, npm, just, task, etc.).

Features:
  • Auto-discovers task files across your project hierarchy
  • Supports multiple task runners: make, npm, yarn, pnpm, bun, just, task, python
  • Interactive TUI mode for visual task management
  • Real-time task execution tracking
  • Configurable environment and hooks
  • Intelligent caching for fast discovery

Examples:
  wake                    # Launch interactive TUI
  wake list               # List all available tasks
  wake build              # Execute build task
  wake test --env=.env    # Execute test task with environment file
  wake --runner=npm list  # List only npm scripts

Configuration:
  Wake looks for configuration in:
  1. .wakeconfig.toml (project-specific)
  2. ~/.config/wake/config.toml (global)
  3. Command-line flags (highest priority)

For more information, visit: https://github.com/vivalchemy/wake`
}

// Execute runs the CLI
func (c *CLI) Execute() error {
	return c.rootCmd.Execute()
}

// GetRootCommand returns the root command
func (c *CLI) GetRootCommand() *cobra.Command {
	return c.rootCmd
}

// GetGlobalOptions returns the global options
func (c *CLI) GetGlobalOptions() *commands.GlobalOptions {
	return c.globalOpts
}
