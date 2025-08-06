package commands

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"slices"

	"github.com/vivalchemy/wake/internal/app"
	"github.com/vivalchemy/wake/internal/tui"
	"github.com/vivalchemy/wake/pkg/logger"
	"github.com/vivalchemy/wake/pkg/task"
)

// RootCommand handles the default behavior when no subcommand is specified
type RootCommand struct {
	app        *app.App
	globalOpts *GlobalOptions
	logger     logger.Logger
}

// GlobalOptions represents global CLI options (imported from cli package)
type GlobalOptions struct {
	// Configuration options
	ConfigFile string
	LogLevel   string
	LogFormat  string

	// Filtering options
	Runner     string
	ShowHidden bool
	Tags       []string

	// Environment options
	EnvFiles   []string
	WorkingDir string

	// Output options
	OutputFormat string
	NoColor      bool
	Quiet        bool
	Verbose      bool

	// Performance options
	MaxDepth   int
	NoCache    bool
	Concurrent bool

	// Debugging options
	Debug   bool
	Profile bool
	DryRun  bool
}

// NewRootCommand creates a new root command handler
func NewRootCommand(application *app.App, globalOpts *GlobalOptions) (*cobra.Command, error) {
	if application == nil {
		return nil, fmt.Errorf("application cannot be nil")
	}

	rootHandler := &RootCommand{
		app:        application,
		globalOpts: globalOpts,
		logger:     application.Logger().WithGroup("root-cmd"),
	}

	cmd := &cobra.Command{
		Use:   "wake [task]",
		Short: "Universal Task Runner",
		Long: `Wake is a universal task runner that provides a unified interface 
for various build tools and task runners.

When called without arguments, it launches an interactive TUI.
When called with a task name, it executes that task directly.`,

		Args: cobra.ArbitraryArgs,
		RunE: rootHandler.Execute,

		ValidArgsFunction: rootHandler.CompleteTaskNames,

		Example: `  wake                    # Launch interactive TUI
  wake build              # Execute build task
  wake test --env=.env    # Execute test with environment file
  wake --runner=npm list  # List only npm scripts`,
	}

	return cmd, nil
}

// Execute handles the root command execution
func (r *RootCommand) Execute(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	r.logger.Debug("Root command executing", "args", args)

	// If arguments are provided, treat as task execution
	if len(args) > 0 {
		return r.executeTask(ctx, args[0], args[1:])
	}

	// Otherwise, launch TUI or fallback to list
	return r.launchInterface(ctx)
}

// executeTask executes a task directly from the command line
func (r *RootCommand) executeTask(ctx context.Context, taskName string, taskArgs []string) error {
	r.logger.Info("Executing task from command line", "task", taskName, "args", taskArgs)

	// Show discovery progress if verbose
	if r.globalOpts.Verbose && !r.globalOpts.Quiet {
		fmt.Fprintf(os.Stderr, "Discovering tasks...\n")
	}

	// Wait for task discovery to complete with timeout
	discoveryCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	select {
	case <-r.app.WaitForDiscovery():
		// Discovery completed
	case <-discoveryCtx.Done():
		return fmt.Errorf("task discovery timed out")
	}

	// Find the task
	targetTask, err := r.findTask(taskName)
	if err != nil {
		return err
	}

	// Handle dry run
	if r.globalOpts.DryRun {
		return r.showDryRun(targetTask, taskArgs)
	}

	// Execute the task
	return r.runTask(ctx, targetTask, taskArgs)
}

// findTask finds a task by name, ID, or alias
func (r *RootCommand) findTask(taskName string) (*task.Task, error) {
	tasks := r.app.GetTasks()

	// Apply filters
	filteredTasks := r.filterTasks(tasks)

	var matches []*task.Task

	// Look for exact matches first
	for _, t := range filteredTasks {
		if t.Name == taskName || t.ID == taskName {
			matches = append(matches, t)
		}
	}

	// If no exact matches, look for alias matches
	if len(matches) == 0 {
		for _, t := range filteredTasks {
			if slices.Contains(t.Aliases, taskName) {
				matches = append(matches, t)
			}
		}
	}

	// If still no matches, look for partial matches
	if len(matches) == 0 {
		lowerTaskName := strings.ToLower(taskName)
		for _, t := range filteredTasks {
			if strings.Contains(strings.ToLower(t.Name), lowerTaskName) {
				matches = append(matches, t)
			}
		}
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("task not found: %s\n\nAvailable tasks:\n%s",
			taskName, r.formatAvailableTasks(filteredTasks))
	}

	if len(matches) > 1 {
		return nil, fmt.Errorf("multiple tasks match '%s':\n%s\n\nPlease be more specific",
			taskName, r.formatTaskMatches(matches))
	}

	return matches[0], nil
}

// filterTasks applies global filters to the task list
func (r *RootCommand) filterTasks(tasks []*task.Task) []*task.Task {
	var filtered []*task.Task

	for _, t := range tasks {
		// Apply runner filter
		if r.globalOpts.Runner != "" && string(t.Runner) != r.globalOpts.Runner {
			continue
		}

		// Apply hidden filter
		if !r.globalOpts.ShowHidden && t.Hidden {
			continue
		}

		// Apply tag filter
		if len(r.globalOpts.Tags) > 0 {
			hasAllTags := true
			for _, requiredTag := range r.globalOpts.Tags {
				if !t.HasTag(requiredTag) {
					hasAllTags = false
					break
				}
			}
			if !hasAllTags {
				continue
			}
		}

		filtered = append(filtered, t)
	}

	return filtered
}

// runTask executes a task with proper environment and output handling
func (r *RootCommand) runTask(ctx context.Context, targetTask *task.Task, taskArgs []string) error {
	// Prepare environment
	env, err := r.prepareEnvironment()
	if err != nil {
		return fmt.Errorf("failed to prepare environment: %w", err)
	}

	// Add task arguments to environment if any
	if len(taskArgs) > 0 {
		env["WAKE_TASK_ARGS"] = strings.Join(taskArgs, " ")
	}

	// Show task info if verbose
	if r.globalOpts.Verbose {
		r.showTaskInfo(targetTask)
	}

	// Execute the task
	r.logger.Info("Starting task execution", "task", targetTask.Name, "id", targetTask.ID)

	if err := r.app.RunTask(targetTask.ID, env); err != nil {
		return fmt.Errorf("failed to start task: %w", err)
	}

	// Handle task execution monitoring
	return r.monitorTaskExecution(ctx, targetTask.ID)
}

// prepareEnvironment loads environment files and prepares task environment
func (r *RootCommand) prepareEnvironment() (map[string]string, error) {
	env := make(map[string]string)

	// Load environment files
	for _, envFile := range r.globalOpts.EnvFiles {
		if err := r.loadEnvironmentFile(envFile, env); err != nil {
			return nil, fmt.Errorf("failed to load environment file %s: %w", envFile, err)
		}
	}

	// Add Wake-specific environment variables
	env["WAKE_VERSION"] = "0.1.0" // TODO: Get from build info
	env["WAKE_PID"] = fmt.Sprintf("%d", os.Getpid())

	return env, nil
}

// loadEnvironmentFile loads a single environment file
func (r *RootCommand) loadEnvironmentFile(filename string, env map[string]string) error {
	r.logger.Debug("Loading environment file", "file", filename)

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
			r.logger.Warn("Invalid environment line", "file", filename, "line", i+1, "content", line)
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes if present
		value = r.unquoteValue(value)

		env[key] = value
	}

	r.logger.Debug("Loaded environment file", "file", filename, "variables", len(env))
	return nil
}

// unquoteValue removes surrounding quotes from a value
func (r *RootCommand) unquoteValue(value string) string {
	if len(value) >= 2 {
		if (value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

// monitorTaskExecution monitors task execution and handles output
func (r *RootCommand) monitorTaskExecution(ctx context.Context, taskID string) error {
	taskEvents := r.app.GetTaskEvents()
	outputEvents := r.app.GetOutputEvents()

	// Create a timeout context for task execution
	timeoutCtx, cancel := context.WithTimeout(ctx, 30*time.Minute) // Default 30 minute timeout
	defer cancel()

	for {
		select {
		case <-timeoutCtx.Done():
			r.logger.Error("Task execution timed out", "task", taskID)
			r.app.StopTask(taskID) // Try to stop the task
			return fmt.Errorf("task execution timed out")

		case event := <-taskEvents:
			if event.Task.ID == taskID {
				switch event.Type {
				case app.TaskEventFinished:
					r.logger.Info("Task completed successfully", "task", taskID)
					if r.globalOpts.Verbose {
						fmt.Fprintf(os.Stderr, "\n✓ Task '%s' completed successfully\n", event.Task.Name)
					}
					return nil

				case app.TaskEventFailed:
					r.logger.Error("Task failed", "task", taskID, "error", event.Error)
					if r.globalOpts.Verbose {
						fmt.Fprintf(os.Stderr, "\n✗ Task '%s' failed: %v\n", event.Task.Name, event.Error)
					}
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

// launchInterface launches the appropriate interface (TUI or fallback)
func (r *RootCommand) launchInterface(ctx context.Context) error {
	// Check if we can launch TUI
	if r.canLaunchTUI() {
		return r.launchTUI(ctx)
	}

	// Fallback to list command
	r.logger.Info("TUI not available, falling back to list command")
	if !r.globalOpts.Quiet {
		fmt.Fprintf(os.Stderr, "TUI not available in this terminal, showing task list:\n\n")
	}

	return r.showTaskList(ctx)
}

// canLaunchTUI checks if TUI can be launched
func (r *RootCommand) canLaunchTUI() bool {
	// Check if we're in a terminal
	if !isTerminal() {
		return false
	}

	// Check if quiet mode is enabled
	if r.globalOpts.Quiet {
		return false
	}

	// Check if output format is specified (implies non-interactive use)
	if r.globalOpts.OutputFormat != "table" {
		return false
	}

	return true
}

// isTerminal checks if we're running in a terminal
func isTerminal() bool {
	// Simple check - in production, you might want a more sophisticated check
	stat, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}

// launchTUI launches the terminal user interface
func (r *RootCommand) launchTUI(ctx context.Context) error {
	r.logger.Debug("Launching TUI")

	// Wait for initial task discovery
	select {
	case <-r.app.WaitForDiscovery():
		// Discovery completed
	case <-ctx.Done():
		return ctx.Err()
	}

	// type Options struct {
	// 	Config       *config.Config
	// 	Discovery    *discovery.Registry
	// 	Executor     *runner.Executor
	// 	Logger       logger.Logger
	// 	AppContext   *app.Context
	// 	Theme        string
	// 	KeyProfile   string
	// 	AltScreen    bool
	// 	MouseEnabled bool
	// }

	// Create and run TUI
	tuiApp, err := tui.New(tui.Options{
		Config:       r.app.Config(),
		Logger:       r.app.Logger(),
		AppContext:   r.app.GetAppContext(r.app.Config(), r.app.Logger()),
		Theme:        r.app.Config().UI.Theme,
		KeyProfile:   r.app.Config().UI.KeyBindings,
		AltScreen:    r.app.Config().UI.AltScreen,
		MouseEnabled: r.app.Config().UI.MouseEnabled,
	})
	if err != nil {
		return fmt.Errorf("failed to create TUI: %w", err)
	}

	return tuiApp.Run()
}

// showTaskList shows a simple task list as fallback
func (r *RootCommand) showTaskList(ctx context.Context) error {
	// Wait for task discovery
	select {
	case <-r.app.WaitForDiscovery():
		// Discovery completed
	case <-ctx.Done():
		return ctx.Err()
	}

	// Get and filter tasks
	tasks := r.app.GetTasks()
	filteredTasks := r.filterTasks(tasks)

	if len(filteredTasks) == 0 {
		fmt.Println("No tasks found.")
		return nil
	}

	// Display tasks
	fmt.Printf("Available tasks (%d):\n\n", len(filteredTasks))

	for _, t := range filteredTasks {
		fmt.Printf("  %s", t.Name)

		if t.Runner != "" {
			fmt.Printf(" (%s)", t.Runner)
		}

		if t.Description != "" {
			fmt.Printf(" - %s", t.Description)
		}

		fmt.Println()
	}

	fmt.Printf("\nRun 'wake <task>' to execute a task or 'wake list' for more details.\n")

	return nil
}

// showDryRun shows what would be executed without actually running
func (r *RootCommand) showDryRun(targetTask *task.Task, taskArgs []string) error {
	fmt.Printf("Dry run for task: %s\n", targetTask.Name)
	fmt.Printf("=================%s\n", strings.Repeat("=", len(targetTask.Name)))
	fmt.Printf("Runner: %s\n", targetTask.Runner)
	fmt.Printf("Command: %s\n", targetTask.Command)

	if len(targetTask.Args) > 0 {
		fmt.Printf("Args: %s\n", strings.Join(targetTask.Args, " "))
	}

	if len(taskArgs) > 0 {
		fmt.Printf("Additional Args: %s\n", strings.Join(taskArgs, " "))
	}

	fmt.Printf("Working Directory: %s\n", targetTask.WorkingDirectory)
	fmt.Printf("File: %s\n", targetTask.FilePath)

	if len(targetTask.Environment) > 0 {
		fmt.Println("\nEnvironment Variables:")
		for key, value := range targetTask.Environment {
			fmt.Printf("  %s=%s\n", key, value)
		}
	}

	if len(targetTask.Dependencies) > 0 {
		fmt.Printf("\nDependencies: %s\n", strings.Join(targetTask.Dependencies, ", "))
	}

	if len(targetTask.Tags) > 0 {
		fmt.Printf("Tags: %s\n", strings.Join(targetTask.Tags, ", "))
	}

	fmt.Printf("\nThis task would be executed with the above configuration.\n")

	return nil
}

// showTaskInfo displays information about a task before execution
func (r *RootCommand) showTaskInfo(targetTask *task.Task) {
	fmt.Fprintf(os.Stderr, "→ Executing task: %s (%s)\n", targetTask.Name, targetTask.Runner)
	if targetTask.Description != "" {
		fmt.Fprintf(os.Stderr, "  %s\n", targetTask.Description)
	}
	fmt.Fprintf(os.Stderr, "\n")
}

// formatAvailableTasks formats the available tasks for error messages
func (r *RootCommand) formatAvailableTasks(tasks []*task.Task) string {
	if len(tasks) == 0 {
		return "  (no tasks available)"
	}

	var lines []string
	for _, t := range tasks {
		line := fmt.Sprintf("  %s", t.Name)
		if t.Runner != "" {
			line += fmt.Sprintf(" (%s)", t.Runner)
		}
		if t.Description != "" {
			line += fmt.Sprintf(" - %s", t.Description)
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// formatTaskMatches formats multiple task matches for error messages
func (r *RootCommand) formatTaskMatches(matches []*task.Task) string {
	var lines []string
	for _, t := range matches {
		line := fmt.Sprintf("  %s", t.Name)
		if t.ID != t.Name {
			line += fmt.Sprintf(" (ID: %s)", t.ID)
		}
		if t.Runner != "" {
			line += fmt.Sprintf(" [%s]", t.Runner)
		}
		if t.FilePath != "" {
			line += fmt.Sprintf(" from %s", t.FilePath)
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

// CompleteTaskNames provides shell completion for task names
func (r *RootCommand) CompleteTaskNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}

	// Get available tasks
	tasks := r.app.GetTasks()
	filteredTasks := r.filterTasks(tasks)

	var completions []string

	for _, task := range filteredTasks {
		if strings.HasPrefix(task.Name, toComplete) {
			description := task.Description
			if description == "" {
				description = fmt.Sprintf("%s task", task.Runner)
			}
			completions = append(completions, fmt.Sprintf("%s\t%s", task.Name, description))
		}

		// Also check aliases
		for _, alias := range task.Aliases {
			if strings.HasPrefix(alias, toComplete) {
				description := fmt.Sprintf("alias for %s", task.Name)
				completions = append(completions, fmt.Sprintf("%s\t%s", alias, description))
			}
		}
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}
