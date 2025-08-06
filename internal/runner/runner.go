package runner

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/vivalchemy/wake/internal/config"
	"github.com/vivalchemy/wake/internal/runner/runners"
	"github.com/vivalchemy/wake/pkg/hooks"
	"github.com/vivalchemy/wake/pkg/logger"
	t "github.com/vivalchemy/wake/pkg/task"
	"maps"
)

// Executor manages task execution across different runners
type TaskExecutor struct {
	// Configuration
	config *config.RunnersConfig
	logger logger.Logger

	// Runner implementations
	runners map[t.RunnerType]Runner

	// Active executions
	executions map[string]*Execution
	mu         sync.RWMutex

	// Hooks
	hooks *hooks.HookManager

	// Resource management
	resourceLimiter *ResourceLimiter
}

// Runner defines the interface for task runners
type TaskRunner interface {
	// CanRun checks if this runner can execute the given task
	CanRun(task *t.Task) bool

	// Run executes a task and returns a process handle
	Run(ctx context.Context, task *t.Task, execCtx *ExecutionContext) (*Process, error)

	// GetRunnerType returns the runner type
	GetRunnerType() t.RunnerType

	// Validate validates that the runner can work in the current environment
	Validate() error

	// GetDefaultArgs returns default arguments for this runner
	GetDefaultArgs() []string

	// GetRequiredEnv returns required environment variables
	GetRequiredEnv() []string
}

// ExecutionContext provides context for task execution
type TaskExecutionContext struct {
	// Task to execute
	Task *t.Task

	// Environment variables
	Environment map[string]string

	// Working directory override
	WorkingDirectory string

	// Output channels
	OutputChan chan<- *OutputEvent

	// Execution options
	Timeout time.Duration

	// Custom arguments
	CustomArgs []string

	// Metadata
	Metadata map[string]any
}

// OutputEvent represents output from a running task
type OutputEvent struct {
	TaskID    string
	Line      string
	IsErr     bool
	Timestamp time.Time
}

// initializeRunners sets up all the task runners
func (e *Executor) initializeRunners() error {
	// Make runner
	makeRunner := runners.NewMakeRunner(e.logger)
	e.runners[t.RunnerMake] = makeRunner

	// NPM runner
	npmRunner := runners.NewNpmRunner(e.logger)
	e.runners[t.RunnerNpm] = npmRunner

	// Yarn runner
	yarnRunner := runners.NewYarnRunner(e.logger)
	e.runners[t.RunnerYarn] = yarnRunner

	// PNPM runner
	pnpmRunner := runners.NewPnpmRunner(e.logger)
	e.runners[t.RunnerPnpm] = pnpmRunner

	// Bun runner
	bunRunner := runners.NewBunRunner(e.logger)
	e.runners[t.RunnerBun] = bunRunner

	// Just runner
	justRunner := runners.NewJustRunner(e.logger)
	e.runners[t.RunnerJust] = justRunner

	// Task runner
	taskRunner := runners.NewTaskRunner(e.logger)
	e.runners[t.RunnerTask] = taskRunner

	// Python runner
	pythonRunner := runners.NewPythonRunner(e.logger)
	e.runners[t.RunnerPython] = pythonRunner

	// FIXME:
	// // Validate all runners
	// for runnerType, runner := range e.runners {
	// 	if err := runner.Validate(); err != nil {
	// 		e.logger.Warn("Runner validation failed", "runner", runnerType, "error", err)
	// 	}
	// }

	return nil
}

// Start starts executing a task
func (e *Executor) Start(execCtx *ExecutionContext) error {
	if execCtx == nil || execCtx.Task == nil {
		return fmt.Errorf("execution context and task cannot be nil")
	}

	task := execCtx.Task
	e.logger.Debug("Starting task execution", "task", task.Name, "id", task.ID)

	// Check if task is already running
	e.mux.RLock()
	if _, exists := e.executions[task.ID]; exists {
		e.mux.RUnlock()
		return fmt.Errorf("task is already running: %s", task.ID)
	}
	e.mux.RUnlock()

	// Find appropriate runner
	runner, err := e.findRunner(task)
	if err != nil {
		return fmt.Errorf("no suitable runner found: %w", err)
	}

	// FIXME:
	// // Check resource limits
	// if err := e.resourceLimiter.Acquire(task); err != nil {
	// 	return fmt.Errorf("resource limit exceeded: %w", err)
	// }

	// Create execution context
	ctx, cancel := context.WithCancel(context.Background())
	if execCtx.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, execCtx.Timeout)
	}

	execution := &Execution{
		Task:      task,
		Context:   ctx,
		StartTime: time.Now(),
		Status:    StatusQueued, // FIXME:
		Cancel:    cancel,
		Done:      make(chan struct{}),
	}

	// Register execution
	e.mux.Lock()
	e.executions[task.ID] = execution
	e.mux.Unlock()

	// Start execution in goroutine
	go e.executeTask(execution, runner)

	return nil
}

// executeTask executes a task with the given runner
func (e *Executor) executeTask(execution *Execution, runner Runner) {
	defer close(execution.Done)
	defer func() {
		// Clean up resources
		// FIXME:
		// e.resourceLimiter.Release(execution.Task)

		// Remove from active executions
		e.mux.Lock()
		delete(e.executions, execution.Task.ID)
		e.mux.Unlock()
	}()

	task := execution.Task
	e.logger.Info("Executing task", "task", task.Name, "runner", task.Runner)

	// Update status
	execution.Mux.Lock()
	execution.Status = StatusRunning
	execution.Mux.Unlock()

	// Update task status
	task.SetStatus(t.StatusRunning)

	// Execute pre-hooks
	if err := e.hookExecutor.ExecutePreHooks(execution.Context, task); err != nil {
		e.logger.Warn("Pre-hook execution failed", "task", task.Name, "error", err)
	}

	// Execute the task
	// Build commandk
	err := runner.Execute(execution.Context, task, execution.Output)
	if err != nil {
		e.handleExecutionError(execution, err)
		return
	}

	// Wait for completion
	endTime := time.Now()

	execution.Mux.Lock()
	execution.EndTime = &[]time.Time{endTime}[0]
	execution.Mux.Unlock()

	// Execute post-hooks
	if err := e.hookExecutor.ExecutePostHooks(execution.Context, task); err != nil {
		e.handleExecutionError(execution, err)
	} else {
		e.handleExecutionSuccess(execution)
	}
}

// handleExecutionError handles task execution errors
func (e *Executor) handleExecutionError(execution *Execution, err error) {
	execution.Mux.Lock()
	execution.Error = err

	// Determine failure type
	if execution.Context.Err() == context.DeadlineExceeded {
		execution.Status = StatusTimeout
		execution.Task.SetStatus(t.StatusFailed)
	} else if execution.Context.Err() == context.Canceled {
		execution.Status = StatusCancelled
		execution.Task.SetStatus(t.StatusStopped)
	} else {
		execution.Status = StatusFailed
		execution.Task.SetStatus(t.StatusFailed)
	}
	execution.Mux.Unlock()

	e.logger.Error("Task execution failed",
		"task", execution.Task.Name,
		"status", execution.Status.String(),
		"error", err)
}

// handleExecutionSuccess handles successful task execution
func (e *Executor) handleExecutionSuccess(execution *Execution) {
	execution.Mux.Lock()
	execution.Status = StatusCompleted
	execution.Mux.Unlock()

	execution.Task.SetStatus(t.StatusSuccess)

	e.logger.Info("Task execution completed",
		"task", execution.Task.Name,
		"duration", execution.EndTime.Sub(execution.StartTime))
}

// Stop stops a running task
func (e *Executor) Stop(taskID string) error {
	e.mux.RLock()
	execution, exists := e.executions[taskID]
	e.mux.RUnlock()

	if !exists {
		return fmt.Errorf("task not found: %s", taskID)
	}

	e.logger.Info("Stopping task", "task", execution.Task.Name)

	// Cancel the execution context
	execution.finishWithCancellation()

	// If process is running, try to stop it gracefully
	execution.Mux.RLock()
	process := execution.Process
	execution.Mux.RUnlock()

	if process != nil {
		if err := process.Release(); err != nil {
			e.logger.Warn("Failed to stop process gracefully", "task", execution.Task.Name, "error", err)

			// Force kill if graceful stop fails
			if err := process.Kill(); err != nil {
				return fmt.Errorf("failed to kill process: %w", err)
			}
		}
	}

	// Wait for execution to complete
	select {
	case <-execution.Done:
		// Execution completed
	case <-time.After(5 * time.Second):
		e.logger.Warn("Timeout waiting for task to stop", "task", execution.Task.Name)
	}

	return nil
}

// findRunner finds the appropriate runner for a task
func (e *Executor) findRunner(task *t.Task) (Runner, error) {
	runner, exists := e.runners[task.Runner]
	if !exists {
		return nil, fmt.Errorf("no runner registered for type: %s", task.Runner)
	}

	if !runner.CanRun(task) {
		return nil, fmt.Errorf("runner cannot execute task: %s", task.Name)
	}

	return runner, nil
}

// IsRunning checks if a task is currently running
func (e *Executor) IsRunning(taskID string) bool {
	e.mux.RLock()
	defer e.mux.RUnlock()

	execution, exists := e.executions[taskID]
	if !exists {
		return false
	}

	execution.Mux.RLock()
	status := execution.Status
	execution.Mux.RUnlock()

	return status == StatusRunning || status == StatusQueued
}

// GetRunners returns all registered runners
func (e *Executor) GetRunners() map[t.RunnerType]Runner {
	// Return a copy to prevent external modification
	runners := make(map[t.RunnerType]Runner)
	maps.Copy(runners, e.runners)
	return runners
}

// ValidateTask validates that a task can be executed
func (e *Executor) ValidateTask(task *t.Task) error {
	runner, err := e.findRunner(task)
	if err != nil {
		return err
	}

	// Check if runner can execute the task
	if !runner.CanRun(task) {
		return fmt.Errorf("runner cannot execute task")
	}

	// Validate working directory
	if task.WorkingDirectory != "" {
		if _, err := os.Stat(task.WorkingDirectory); os.IsNotExist(err) {
			return fmt.Errorf("working directory does not exist: %s", task.WorkingDirectory)
		}
	}

	// Check required environment variables
	requiredEnv := runner.GetEnvironmentVariables(task)
	for _, envVar := range requiredEnv {
		if _, exists := task.Environment[envVar]; !exists {
			if _, exists := os.LookupEnv(envVar); !exists {
				return fmt.Errorf("required environment variable not set: %s", envVar)
			}
		}
	}

	return nil
}

// GetExecutionStats returns statistics about task executions
func (e *Executor) GetExecutionStats() *RunningExecutionStats {
	e.mux.RLock()
	defer e.mux.RUnlock()

	stats := &RunningExecutionStats{
		ActiveExecutions: len(e.executions),
		RunnerStats:      make(map[t.RunnerType]int),
	}

	for _, execution := range e.executions {
		stats.RunnerStats[execution.Task.Runner]++
	}

	return stats
}

// ExecutionStats provides statistics about task executions
type RunningExecutionStats struct {
	ActiveExecutions int                  `json:"active_executions"`
	RunnerStats      map[t.RunnerType]int `json:"runner_stats"`
}

// Shutdown gracefully shuts down the executor
func (e *Executor) Shutdown(ctx context.Context) error {
	e.logger.Info("Shutting down executor")

	// Get all active executions
	activeExecutions := e.GetActiveExecutions()

	if len(activeExecutions) == 0 {
		return nil
	}

	e.logger.Info("Stopping active executions", "count", len(activeExecutions))

	// Stop all active executions
	for taskID := range activeExecutions {
		if err := e.Stop(taskID); err != nil {
			e.logger.Error("Failed to stop task during shutdown", "task", taskID, "error", err)
		}
	}

	// Wait for all executions to complete or timeout
	deadline := time.Now().Add(10 * time.Second)
	for len(e.GetActiveExecutions()) > 0 && time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
			// Continue waiting
		}
	}

	remaining := e.GetActiveExecutions()
	if len(remaining) > 0 {
		e.logger.Warn("Some executions did not stop gracefully", "count", len(remaining))
	}

	// Shutdown resource limiter
	// FIXME:
	// if err := e.resourceLimiter.Shutdown(); err != nil {
	// 	e.logger.Error("Failed to shutdown resource limiter", "error", err)
	// }

	return nil
}

// GetDuration returns the duration of the execution
func (e *Execution) GetDuration() time.Duration {
	e.Mux.RLock()
	defer e.Mux.RUnlock()

	if e.EndTime != nil {
		return e.EndTime.Sub(e.StartTime)
	}

	return time.Since(e.StartTime)
}

// GetExitCode returns the exit code of the execution
func (e *Execution) GetExitCode() int {
	e.Mux.RLock()
	defer e.Mux.RUnlock()
	return e.ExitCode
}

// GetError returns any error that occurred during execution
func (e *Execution) GetError() error {
	e.Mux.RLock()
	defer e.Mux.RUnlock()
	return e.Error
}

// Wait waits for the execution to complete
func (e *Execution) Wait() error {
	<-e.Done
	return e.GetError()
}

// WaitWithTimeout waits for execution to complete with a timeout
func (e *Execution) WaitWithTimeout(timeout time.Duration) error {
	select {
	case <-e.Done:
		return e.GetError()
	case <-time.After(timeout):
		return fmt.Errorf("execution timeout after %v", timeout)
	}
}
