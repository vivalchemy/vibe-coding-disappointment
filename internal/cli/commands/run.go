package commands

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/vivalchemy/wake/internal/app"
	"github.com/vivalchemy/wake/pkg/task"
	"maps"
	"slices"
)

// RunCommand handles the run subcommand for executing tasks
type RunCommand struct {
	app        *app.App
	globalOpts *GlobalOptions

	// Run-specific options
	parallel        bool
	continueOnError bool
	timeout         time.Duration
	maxRetries      int
	retryDelay      time.Duration
	interactive     bool
	confirm         bool
	watch           bool

	// Execution state
	runningTasks map[string]*TaskExecution
	results      map[string]*ExecutionResult
}

// TaskExecution tracks the state of a running task
type TaskExecution struct {
	Task      *task.Task
	StartTime time.Time
	Context   context.Context
	Cancel    context.CancelFunc
	Done      chan struct{}
	Error     error
	Retries   int
}

// ExecutionResult holds the result of task execution
type ExecutionResult struct {
	Task      *task.Task
	Success   bool
	Error     error
	Duration  time.Duration
	ExitCode  int
	Output    []string
	StartTime time.Time
	EndTime   time.Time
	Retries   int
}

// NewRunCommand creates a new run command
func NewRunCommand(application *app.App, globalOpts *GlobalOptions) (*cobra.Command, error) {
	if application == nil {
		return nil, fmt.Errorf("application cannot be nil")
	}

	runHandler := &RunCommand{
		app:          application,
		globalOpts:   globalOpts,
		runningTasks: make(map[string]*TaskExecution),
		results:      make(map[string]*ExecutionResult),
	}

	cmd := &cobra.Command{
		Use:   "run <task> [task2] [task3] ...",
		Short: "Run one or more tasks",
		Long: `Run one or more tasks by name, ID, or alias.

The run command executes tasks and monitors their progress. It supports
running multiple tasks in parallel or sequence, with retry logic and
timeout controls.`,

		Args:    cobra.MinimumNArgs(1),
		RunE:    runHandler.Execute,
		Aliases: []string{"r", "exec", "execute"},

		ValidArgsFunction: runHandler.CompleteTaskNames,

		Example: `  wake run build                     # Run build task
  wake run test lint                 # Run test and lint tasks sequentially
  wake run --parallel test lint      # Run test and lint tasks in parallel
  wake run --timeout=5m build        # Run with 5 minute timeout
  wake run --retry=3 deploy          # Run with up to 3 retries
  wake run --confirm deploy          # Ask for confirmation before running
  wake run --watch test              # Re-run when files change`,
	}

	// Add run-specific flags
	runHandler.addFlags(cmd)

	return cmd, nil
}

// addFlags adds command-specific flags
func (r *RunCommand) addFlags(cmd *cobra.Command) {
	flags := cmd.Flags()

	// Execution control
	flags.BoolVarP(&r.parallel, "parallel", "p", false,
		"run tasks in parallel instead of sequentially")
	flags.BoolVar(&r.continueOnError, "continue-on-error", false,
		"continue running other tasks if one fails")
	flags.DurationVarP(&r.timeout, "timeout", "t", 30*time.Minute,
		"timeout for task execution")

	// Retry logic
	flags.IntVar(&r.maxRetries, "retry", 0,
		"maximum number of retries on failure")
	flags.DurationVar(&r.retryDelay, "retry-delay", 5*time.Second,
		"delay between retries")

	// Interactive options
	flags.BoolVarP(&r.interactive, "interactive", "i", false,
		"run in interactive mode")
	flags.BoolVarP(&r.confirm, "confirm", "y", false,
		"ask for confirmation before running")
	flags.BoolVarP(&r.watch, "watch", "w", false,
		"watch for file changes and re-run tasks")
}

// Execute runs the run command
func (r *RunCommand) Execute(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Wait for task discovery to complete
	select {
	case <-r.app.WaitForDiscovery():
		// Discovery completed
	case <-ctx.Done():
		return ctx.Err()
	}

	// Resolve task names to task objects
	tasks, err := r.resolveTasks(args)
	if err != nil {
		return err
	}

	// Ask for confirmation if requested
	if r.confirm && !r.globalOpts.Quiet {
		if !r.askConfirmation(tasks) {
			fmt.Println("Execution cancelled.")
			return nil
		}
	}

	// Show tasks to be executed if verbose
	if r.globalOpts.Verbose {
		r.showExecutionPlan(tasks)
	}

	// Set up signal handling for graceful shutdown
	ctx, cancel := r.setupSignalHandling(ctx)
	defer cancel()

	// Execute tasks based on mode
	if r.watch {
		return r.executeWithWatch(ctx, tasks)
	} else if r.parallel {
		return r.executeParallel(ctx, tasks)
	} else {
		return r.executeSequential(ctx, tasks)
	}
}

// resolveTasks resolves task names to task objects
func (r *RunCommand) resolveTasks(taskNames []string) ([]*task.Task, error) {
	allTasks := r.app.GetTasks()
	var resolvedTasks []*task.Task

	for _, taskName := range taskNames {
		task, err := r.findTask(taskName, allTasks)
		if err != nil {
			return nil, err
		}
		resolvedTasks = append(resolvedTasks, task)
	}

	return resolvedTasks, nil
}

// findTask finds a task by name, ID, or alias
func (r *RunCommand) findTask(taskName string, allTasks []*task.Task) (*task.Task, error) {
	var matches []*task.Task

	// Apply global filters
	filteredTasks := r.filterTasks(allTasks)

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

	if len(matches) == 0 {
		return nil, fmt.Errorf("task not found: %s", taskName)
	}

	if len(matches) > 1 {
		return nil, fmt.Errorf("multiple tasks match '%s': %s",
			taskName, r.formatTaskMatches(matches))
	}

	return matches[0], nil
}

// filterTasks applies global filters to the task list
func (r *RunCommand) filterTasks(tasks []*task.Task) []*task.Task {
	var filtered []*task.Task

	for _, t := range tasks {
		// Apply runner filter
		if r.globalOpts.Runner != "" && string(t.Runner) != r.globalOpts.Runner {
			continue
		}

		// Apply tags filter
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

// askConfirmation asks user for confirmation before executing tasks
func (r *RunCommand) askConfirmation(tasks []*task.Task) bool {
	fmt.Printf("About to execute %d task(s):\n", len(tasks))
	for _, t := range tasks {
		fmt.Printf("  - %s (%s)", t.Name, t.Runner)
		if t.Description != "" {
			fmt.Printf(" - %s", t.Description)
		}
		fmt.Println()
	}

	fmt.Print("\nContinue? [y/N]: ")
	var response string
	fmt.Scanln(&response)

	return strings.ToLower(response) == "y" || strings.ToLower(response) == "yes"
}

// showExecutionPlan displays the execution plan
func (r *RunCommand) showExecutionPlan(tasks []*task.Task) {
	mode := "sequential"
	if r.parallel {
		mode = "parallel"
	}

	fmt.Printf("→ Execution plan (%s mode):\n", mode)
	for i, t := range tasks {
		fmt.Printf("  %d. %s (%s)", i+1, t.Name, t.Runner)
		if t.Description != "" {
			fmt.Printf(" - %s", t.Description)
		}
		fmt.Println()
	}
	fmt.Println()
}

// setupSignalHandling sets up signal handling for graceful shutdown
func (r *RunCommand) setupSignalHandling(ctx context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(ctx)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Fprintf(os.Stderr, "\n→ Received interrupt signal, stopping tasks...\n")
		cancel()

		// Stop all running tasks
		for taskID, exec := range r.runningTasks {
			fmt.Fprintf(os.Stderr, "→ Stopping task: %s\n", exec.Task.Name)
			r.app.StopTask(taskID)
			exec.Cancel()
		}
	}()

	return ctx, cancel
}

// executeSequential executes tasks one by one
func (r *RunCommand) executeSequential(ctx context.Context, tasks []*task.Task) error {
	var failedTasks []string

	for i, t := range tasks {
		if !r.globalOpts.Quiet {
			fmt.Printf("→ [%d/%d] Running task: %s\n", i+1, len(tasks), t.Name)
		}

		result, err := r.executeTask(ctx, t)
		r.results[t.ID] = result

		if err != nil {
			failedTasks = append(failedTasks, t.Name)

			if !r.continueOnError {
				return fmt.Errorf("task '%s' failed: %w", t.Name, err)
			}

			if !r.globalOpts.Quiet {
				fmt.Fprintf(os.Stderr, "✗ Task '%s' failed: %v\n", t.Name, err)
				fmt.Fprintf(os.Stderr, "→ Continuing with next task...\n")
			}
		} else {
			if !r.globalOpts.Quiet {
				fmt.Printf("✓ Task '%s' completed successfully\n", t.Name)
			}
		}
	}

	// Show summary
	r.showExecutionSummary()

	if len(failedTasks) > 0 {
		return fmt.Errorf("tasks failed: %s", strings.Join(failedTasks, ", "))
	}

	return nil
}

// executeParallel executes tasks in parallel
func (r *RunCommand) executeParallel(ctx context.Context, tasks []*task.Task) error {
	if !r.globalOpts.Quiet {
		fmt.Printf("→ Running %d tasks in parallel...\n", len(tasks))
	}

	// Create a channel to collect results
	resultsChan := make(chan *ExecutionResult, len(tasks))

	// Start all tasks
	for _, t := range tasks {
		go func(task *task.Task) {
			result, _ := r.executeTask(ctx, task)
			resultsChan <- result
		}(t)
	}

	// Collect results
	var failedTasks []string
	for range tasks {
		result := <-resultsChan
		r.results[result.Task.ID] = result

		if !result.Success {
			failedTasks = append(failedTasks, result.Task.Name)

			if !r.globalOpts.Quiet {
				fmt.Fprintf(os.Stderr, "✗ Task '%s' failed: %v\n", result.Task.Name, result.Error)
			}
		} else {
			if !r.globalOpts.Quiet {
				fmt.Printf("✓ Task '%s' completed successfully\n", result.Task.Name)
			}
		}
	}

	// Show summary
	r.showExecutionSummary()

	if len(failedTasks) > 0 && !r.continueOnError {
		return fmt.Errorf("tasks failed: %s", strings.Join(failedTasks, ", "))
	}

	return nil
}

// executeWithWatch executes tasks and watches for file changes
func (r *RunCommand) executeWithWatch(ctx context.Context, tasks []*task.Task) error {
	// This is a placeholder for watch functionality
	// In a full implementation, you would use a file watcher library
	// to monitor files and re-run tasks when changes are detected

	fmt.Println("→ Watch mode is not yet implemented")
	fmt.Println("→ Falling back to single execution")

	return r.executeSequential(ctx, tasks)
}

// executeTask executes a single task with retry logic
func (r *RunCommand) executeTask(ctx context.Context, t *task.Task) (*ExecutionResult, error) {
	result := &ExecutionResult{
		Task:      t,
		StartTime: time.Now(),
	}

	var lastErr error

	for attempt := 0; attempt <= r.maxRetries; attempt++ {
		if attempt > 0 {
			if !r.globalOpts.Quiet {
				fmt.Printf("→ Retrying task '%s' (attempt %d/%d)...\n",
					t.Name, attempt, r.maxRetries)
			}

			// Wait before retry
			select {
			case <-time.After(r.retryDelay):
			case <-ctx.Done():
				result.Error = ctx.Err()
				return result, ctx.Err()
			}
		}

		// Create execution context with timeout
		execCtx, cancel := context.WithTimeout(ctx, r.timeout)
		defer cancel()

		// Track execution
		exec := &TaskExecution{
			Task:      t,
			StartTime: time.Now(),
			Context:   execCtx,
			Cancel:    cancel,
			Done:      make(chan struct{}),
			Retries:   attempt,
		}

		r.runningTasks[t.ID] = exec

		// Execute task
		err := r.runSingleTask(execCtx, t, exec)

		// Clean up
		delete(r.runningTasks, t.ID)
		close(exec.Done)

		if err == nil {
			// Success
			result.Success = true
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(result.StartTime)
			result.Retries = attempt
			return result, nil
		}

		lastErr = err
		result.Retries = attempt + 1

		// Check if we should retry
		if attempt >= r.maxRetries || isContextCanceled(err) {
			break
		}
	}

	// All attempts failed
	result.Success = false
	result.Error = lastErr
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	return result, lastErr
}

// runSingleTask runs a single task execution attempt
func (r *RunCommand) runSingleTask(ctx context.Context, t *task.Task, _ *TaskExecution) error {
	// Prepare environment
	env, err := r.prepareEnvironment(t)
	if err != nil {
		return fmt.Errorf("failed to prepare environment: %w", err)
	}

	// Start the task
	if err := r.app.RunTask(t.ID, env); err != nil {
		return fmt.Errorf("failed to start task: %w", err)
	}

	// Monitor task execution
	return r.monitorTaskExecution(ctx, t.ID)
}

// prepareEnvironment prepares the execution environment
func (r *RunCommand) prepareEnvironment(t *task.Task) (map[string]string, error) {
	env := make(map[string]string)

	// Load environment files
	for _, envFile := range r.globalOpts.EnvFiles {
		if err := r.loadEnvironmentFile(envFile, env); err != nil {
			return nil, fmt.Errorf("failed to load environment file %s: %w", envFile, err)
		}
	}

	// Add task-specific environment
	maps.Copy(env, t.Environment)

	// Add Wake-specific environment variables
	env["WAKE_TASK_NAME"] = t.Name
	env["WAKE_TASK_ID"] = t.ID
	env["WAKE_TASK_RUNNER"] = string(t.Runner)
	env["WAKE_TASK_FILE"] = t.FilePath

	return env, nil
}

// loadEnvironmentFile loads environment variables from a file
func (r *RunCommand) loadEnvironmentFile(filename string, env map[string]string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	lines := strings.SplitSeq(string(data), "\n")
	for line := range lines {
		line = strings.TrimSpace(line)

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes
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

// monitorTaskExecution monitors a task's execution
func (r *RunCommand) monitorTaskExecution(ctx context.Context, taskID string) error {
	taskEvents := r.app.GetTaskEvents()
	outputEvents := r.app.GetOutputEvents()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case event := <-taskEvents:
			if event.Task.ID == taskID {
				switch event.Type {
				case app.TaskEventFinished:
					return nil
				case app.TaskEventFailed:
					return fmt.Errorf("task failed: %v", event.Error)
				}
			}

		case output := <-outputEvents:
			if output.TaskID == taskID {
				if output.IsErr {
					fmt.Fprint(os.Stderr, output.Line)
				} else {
					fmt.Print(output.Line)
				}
			}
		}
	}
}

// showExecutionSummary displays a summary of execution results
func (r *RunCommand) showExecutionSummary() {
	if r.globalOpts.Quiet {
		return
	}

	successful := 0
	failed := 0
	totalDuration := time.Duration(0)

	for _, result := range r.results {
		if result.Success {
			successful++
		} else {
			failed++
		}
		totalDuration += result.Duration
	}

	fmt.Printf("\n→ Execution Summary:\n")
	fmt.Printf("  Total tasks: %d\n", len(r.results))
	fmt.Printf("  Successful: %d\n", successful)
	fmt.Printf("  Failed: %d\n", failed)
	fmt.Printf("  Total duration: %v\n", totalDuration)
}

// formatTaskMatches formats multiple task matches for error messages
func (r *RunCommand) formatTaskMatches(matches []*task.Task) string {
	var names []string
	for _, t := range matches {
		names = append(names, fmt.Sprintf("%s (%s)", t.Name, t.Runner))
	}
	return strings.Join(names, ", ")
}

// isContextCanceled checks if an error is due to context cancellation
func isContextCanceled(err error) bool {
	return err == context.Canceled || err == context.DeadlineExceeded
}

// CompleteTaskNames provides shell completion for task names
func (r *RunCommand) CompleteTaskNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
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
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}
