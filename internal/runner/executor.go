package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"github.com/vivalchemy/wake/internal/output"
	"github.com/vivalchemy/wake/internal/runner/runners"
	"github.com/vivalchemy/wake/pkg/hooks"
	"github.com/vivalchemy/wake/pkg/logger"
	"github.com/vivalchemy/wake/pkg/task"
	"slices"
)

// Executor manages task execution across different runners
type Executor struct {
	// Runners
	runners     map[task.RunnerType]Runner
	runnerOrder []task.RunnerType

	// Configuration
	options *ExecutorOptions

	// Execution tracking
	executions map[string]*Execution
	mux        sync.RWMutex

	// Output management
	outputManager *output.Manager

	// Hooks
	hookExecutor *hooks.Executor

	// Statistics
	stats   *ExecutorStats
	statsMu sync.RWMutex

	// Concurrency control
	semaphore chan struct{}

	// Logger
	logger logger.Logger
}

// ExecutorOptions contains configuration for the executor
type ExecutorOptions struct {
	// Execution limits
	MaxConcurrentTasks int
	DefaultTimeout     time.Duration
	KillTimeout        time.Duration

	// Output handling
	CaptureOutput bool
	StreamOutput  bool
	BufferSize    int

	// Error handling
	ContinueOnError bool
	RetryAttempts   int
	RetryDelay      time.Duration

	// Environment
	InheritEnvironment bool
	CustomEnvironment  map[string]string
	WorkingDirectory   string

	// Hooks
	EnableHooks bool
	HookTimeout time.Duration

	// Performance
	EnableProfiling bool
	CollectMetrics  bool
}

// Execution represents a running or completed task execution
type Execution struct {
	// Task information
	Task   *task.Task
	Runner Runner

	// Execution state
	ID        string
	Status    ExecutionStatus
	StartTime time.Time
	EndTime   *time.Time
	Duration  time.Duration

	// Process information
	Process      *os.Process
	ProcessState *os.ProcessState
	ExitCode     int

	// Context and cancellation
	Context context.Context
	Cancel  context.CancelFunc

	// Output streams
	Output      *output.Buffer
	ErrorOutput *output.Buffer

	// Statistics
	Stats *ExecutionStats

	// Error information
	Error error

	// Synchronization
	Mux  sync.RWMutex
	Done chan struct{}
}

// ExecutionStatus represents the status of a task execution
type ExecutionStatus int

const (
	StatusQueued ExecutionStatus = iota
	StatusStarting
	StatusRunning
	StatusStopping
	StatusCompleted
	StatusFailed
	StatusCancelled
	StatusTimeout
)

// String returns the string representation of ExecutionStatus
func (s ExecutionStatus) String() string {
	switch s {
	case StatusQueued:
		return "queued"
	case StatusStarting:
		return "starting"
	case StatusRunning:
		return "running"
	case StatusStopping:
		return "stopping"
	case StatusCompleted:
		return "completed"
	case StatusFailed:
		return "failed"
	case StatusCancelled:
		return "cancelled"
	case StatusTimeout:
		return "timeout"
	default:
		return "unknown"
	}
}

// ExecutorStats tracks execution statistics
type ExecutorStats struct {
	// Counters
	TotalExecutions      int64
	SuccessfulExecutions int64
	FailedExecutions     int64
	CancelledExecutions  int64
	TimeoutExecutions    int64

	// Timing
	TotalDuration   time.Duration
	AverageDuration time.Duration
	MinDuration     time.Duration
	MaxDuration     time.Duration

	// Concurrency
	CurrentExecutions int64
	PeakConcurrency   int64

	// Runner stats
	RunnerStats map[task.RunnerType]*RunnerStats

	// Performance
	LastExecutionTime time.Time
}

// ExecutionStats tracks statistics for individual executions
type ExecutionStats struct {
	StartupTime   time.Duration
	ExecutionTime time.Duration
	ShutdownTime  time.Duration
	OutputBytes   int64
	ErrorBytes    int64
	RetryCount    int
	MemoryUsage   int64
	CPUTime       time.Duration
}

// RunnerStats tracks statistics for individual runners
type RunnerStats struct {
	Executions      int64
	Successes       int64
	Failures        int64
	TotalDuration   time.Duration
	AverageDuration time.Duration
}

// Runner interface that all task runners must implement
type Runner interface {
	// Runner information
	Type() task.RunnerType
	Name() string
	Version() string

	// Task execution
	CanRun(task *task.Task) bool
	Prepare(ctx context.Context, task *task.Task) error
	Execute(ctx context.Context, task *task.Task, output *output.Buffer) error
	Stop(ctx context.Context, task *task.Task) error

	// Configuration
	GetDefaultTimeout() time.Duration
	GetEnvironmentVariables(task *task.Task) map[string]string

	// Validation
	Validate(task *task.Task) error
}

// NewExecutor creates a new task executor
func NewExecutor(outputManager *output.Manager, hookExecutor *hooks.Executor, logger logger.Logger) *Executor {
	options := createDefaultExecutorOptions()

	executor := &Executor{
		runners:       make(map[task.RunnerType]Runner),
		runnerOrder:   []task.RunnerType{},
		options:       options,
		executions:    make(map[string]*Execution),
		outputManager: outputManager,
		hookExecutor:  hookExecutor,
		stats:         newExecutorStats(),
		semaphore:     make(chan struct{}, options.MaxConcurrentTasks),
		logger:        logger.WithGroup("executor"),
	}

	// Register default runners
	executor.registerDefaultRunners()

	return executor
}

// createDefaultExecutorOptions creates default executor options
func createDefaultExecutorOptions() *ExecutorOptions {
	return &ExecutorOptions{
		MaxConcurrentTasks: 5,
		DefaultTimeout:     5 * time.Minute,
		KillTimeout:        10 * time.Second,
		CaptureOutput:      true,
		StreamOutput:       true,
		BufferSize:         64 * 1024,
		ContinueOnError:    false,
		RetryAttempts:      0,
		RetryDelay:         time.Second,
		InheritEnvironment: true,
		CustomEnvironment:  make(map[string]string),
		WorkingDirectory:   "",
		EnableHooks:        true,
		HookTimeout:        30 * time.Second,
		EnableProfiling:    false,
		CollectMetrics:     true,
	}
}

// registerDefaultRunners registers all built-in runners
func (e *Executor) registerDefaultRunners() {
	// Register Make runner
	e.RegisterRunner(runners.NewMakeRunner(e.logger))

	// Register NPM runner
	e.RegisterRunner(runners.NewNpmRunner(e.logger))

	// Register Yarn runner
	e.RegisterRunner(runners.NewYarnRunner(e.logger))

	// Register pnpm runner
	e.RegisterRunner(runners.NewPnpmRunner(e.logger))

	// Register Bun runner
	e.RegisterRunner(runners.NewBunRunner(e.logger))

	// Register Just runner
	e.RegisterRunner(runners.NewJustRunner(e.logger))

	// Register Task runner
	e.RegisterRunner(runners.NewTaskRunner(e.logger))

	// Register Python runner
	e.RegisterRunner(runners.NewPythonRunner(e.logger))

	e.logger.Info("Registered default runners", "count", len(e.runners))
}

// RegisterRunner registers a task runner
func (e *Executor) RegisterRunner(runner Runner) {
	e.mux.Lock()
	defer e.mux.Unlock()

	runnerType := runner.Type()
	e.runners[runnerType] = runner

	// Add to order if not already present
	if slices.Contains(e.runnerOrder, runnerType) {
		return
	}
	e.runnerOrder = append(e.runnerOrder, runnerType)

	e.logger.Debug("Registered runner", "type", runnerType, "name", runner.Name())
}

// UnregisterRunner removes a runner
func (e *Executor) UnregisterRunner(runnerType task.RunnerType) {
	e.mux.Lock()
	defer e.mux.Unlock()

	delete(e.runners, runnerType)

	// Remove from order
	for i, existing := range e.runnerOrder {
		if existing == runnerType {
			e.runnerOrder = slices.Delete(e.runnerOrder, i, i+1)
			break
		}
	}

	e.logger.Debug("Unregistered runner", "type", runnerType)
}

// GetRunner returns a runner for the specified type
func (e *Executor) GetRunner(runnerType task.RunnerType) (Runner, bool) {
	e.mux.RLock()
	defer e.mux.RUnlock()

	runner, exists := e.runners[runnerType]
	return runner, exists
}

// ExecuteTask executes a task
func (e *Executor) ExecuteTask(ctx context.Context, task *task.Task) (*Execution, error) {
	e.logger.Info("Executing task", "task", task.Name, "runner", task.Runner)

	// Find appropriate runner
	runner, exists := e.GetRunner(task.Runner)
	if !exists {
		return nil, fmt.Errorf("no runner found for type: %s", task.Runner)
	}

	// Validate task
	if err := runner.Validate(task); err != nil {
		return nil, fmt.Errorf("task validation failed: %w", err)
	}

	// Create execution
	execution := e.createExecution(ctx, task, runner)

	// Track execution
	e.trackExecution(execution)

	// Execute asynchronously
	go e.executeAsync(execution)

	return execution, nil
}

// createExecution creates a new execution instance
func (e *Executor) createExecution(ctx context.Context, task *task.Task, runner Runner) *Execution {
	executionID := generateExecutionID(task)
	execCtx, cancel := context.WithCancel(ctx)

	// Apply timeout if specified
	// if task.Timeout > 0 {
	// 	execCtx, cancel = context.WithTimeout(execCtx, task.Timeout)
	// } else if e.options.DefaultTimeout > 0 {
	// 	execCtx, cancel = context.WithTimeout(execCtx, e.options.DefaultTimeout)
	// }

	execution := &Execution{
		Task:        task,
		Runner:      runner,
		ID:          executionID,
		Status:      StatusQueued,
		Context:     execCtx,
		Cancel:      cancel,
		Output:      output.NewBuffer(e.options.BufferSize),
		ErrorOutput: output.NewBuffer(e.options.BufferSize),
		Stats:       &ExecutionStats{},
		Done:        make(chan struct{}),
	}

	return execution
}

// trackExecution adds execution to tracking
func (e *Executor) trackExecution(execution *Execution) {
	e.mux.Lock()
	defer e.mux.Unlock()

	e.executions[execution.ID] = execution

	// Update stats
	e.statsMu.Lock()
	e.stats.TotalExecutions++
	e.stats.CurrentExecutions++
	if e.stats.CurrentExecutions > e.stats.PeakConcurrency {
		e.stats.PeakConcurrency = e.stats.CurrentExecutions
	}
	e.statsMu.Unlock()
}

// executeAsync executes a task asynchronously
func (e *Executor) executeAsync(execution *Execution) {
	defer close(execution.Done)
	defer e.untrackExecution(execution)

	// Acquire semaphore for concurrency control
	e.semaphore <- struct{}{}
	defer func() { <-e.semaphore }()

	// Update status
	execution.setStatus(StatusStarting)
	execution.StartTime = time.Now()

	// Execute pre-hooks if enabled
	if e.options.EnableHooks && e.hookExecutor != nil {
		if err := e.executePreHooks(execution); err != nil {
			e.logger.Warn("Pre-hook execution failed", "execution", execution.ID, "error", err)
		}
	}

	// Prepare task
	execution.setStatus(StatusRunning)
	startupStart := time.Now()

	if err := execution.Runner.Prepare(execution.Context, execution.Task); err != nil {
		execution.finishWithError(fmt.Errorf("preparation failed: %w", err))
		return
	}

	execution.Stats.StartupTime = time.Since(startupStart)

	// Execute task
	executionStart := time.Now()
	err := execution.Runner.Execute(execution.Context, execution.Task, execution.Output)
	execution.Stats.ExecutionTime = time.Since(executionStart)

	// Handle execution result
	if err != nil {
		if execution.Context.Err() == context.DeadlineExceeded {
			execution.finishWithTimeout()
		} else if execution.Context.Err() == context.Canceled {
			execution.finishWithCancellation()
		} else {
			execution.finishWithError(err)
		}
	} else {
		execution.finishSuccessfully()
	}

	// Execute post-hooks if enabled
	if e.options.EnableHooks && e.hookExecutor != nil {
		if err := e.executePostHooks(execution); err != nil {
			e.logger.Warn("Post-hook execution failed", "execution", execution.ID, "error", err)
		}
	}

	// Update runner statistics
	e.updateRunnerStats(execution)
}

// executePreHooks executes pre-execution hooks
func (e *Executor) executePreHooks(execution *Execution) error {
	hookCtx, cancel := context.WithTimeout(execution.Context, e.options.HookTimeout)
	defer cancel()

	return e.hookExecutor.ExecutePreHooks(hookCtx, execution.Task)
}

// executePostHooks executes post-execution hooks
func (e *Executor) executePostHooks(execution *Execution) error {
	hookCtx, cancel := context.WithTimeout(context.Background(), e.options.HookTimeout)
	defer cancel()

	return e.hookExecutor.ExecutePostHooks(hookCtx, execution.Task)
}

// untrackExecution removes execution from tracking
func (e *Executor) untrackExecution(execution *Execution) {
	e.mux.Lock()
	defer e.mux.Unlock()

	delete(e.executions, execution.ID)

	// Update stats
	e.statsMu.Lock()
	e.stats.CurrentExecutions--
	e.stats.LastExecutionTime = time.Now()
	e.statsMu.Unlock()
}

// updateRunnerStats updates statistics for the runner
func (e *Executor) updateRunnerStats(execution *Execution) {
	e.statsMu.Lock()
	defer e.statsMu.Unlock()

	runnerType := execution.Runner.Type()

	if e.stats.RunnerStats == nil {
		e.stats.RunnerStats = make(map[task.RunnerType]*RunnerStats)
	}

	stats, exists := e.stats.RunnerStats[runnerType]
	if !exists {
		stats = &RunnerStats{}
		e.stats.RunnerStats[runnerType] = stats
	}

	stats.Executions++
	stats.TotalDuration += execution.Duration
	stats.AverageDuration = stats.TotalDuration / time.Duration(stats.Executions)

	if execution.Status == StatusCompleted {
		stats.Successes++
		e.stats.SuccessfulExecutions++
	} else {
		stats.Failures++
		e.stats.FailedExecutions++
	}

	// Update global duration stats
	if e.stats.MinDuration == 0 || execution.Duration < e.stats.MinDuration {
		e.stats.MinDuration = execution.Duration
	}
	if execution.Duration > e.stats.MaxDuration {
		e.stats.MaxDuration = execution.Duration
	}

	e.stats.TotalDuration += execution.Duration
	e.stats.AverageDuration = e.stats.TotalDuration / time.Duration(e.stats.TotalExecutions)
}

// StopTask stops a running task
func (e *Executor) StopTask(taskID string) error {
	e.mux.RLock()
	execution, exists := e.executions[taskID]
	e.mux.RUnlock()

	if !exists {
		return fmt.Errorf("execution not found: %s", taskID)
	}

	return e.stopExecution(execution)
}

// stopExecution stops a specific execution
func (e *Executor) stopExecution(execution *Execution) error {
	execution.Mux.Lock()
	defer execution.Mux.Unlock()

	if execution.Status != StatusRunning {
		return fmt.Errorf("execution is not running: %s", execution.Status)
	}

	execution.Status = StatusStopping

	// Try graceful shutdown first
	if err := execution.Runner.Stop(execution.Context, execution.Task); err != nil {
		e.logger.Warn("Graceful stop failed", "execution", execution.ID, "error", err)
	}

	// Cancel context
	execution.Cancel()

	// Force kill if process doesn't stop
	if execution.Process != nil {
		go func() {
			time.Sleep(e.options.KillTimeout)
			if execution.Status == StatusStopping {
				execution.Process.Kill()
			}
		}()
	}

	return nil
}

// GetActiveExecutions returns all currently active executions
func (e *Executor) GetActiveExecutions() map[string]*Execution {
	e.mux.RLock()
	defer e.mux.RUnlock()

	active := make(map[string]*Execution)
	for id, execution := range e.executions {
		if execution.Status == StatusRunning || execution.Status == StatusStarting {
			active[id] = execution
		}
	}

	return active
}

// GetExecution returns a specific execution
func (e *Executor) GetExecution(executionID string) (*Execution, bool) {
	e.mux.RLock()
	defer e.mux.RUnlock()

	execution, exists := e.executions[executionID]
	return execution, exists
}

// WaitForExecution waits for an execution to complete
func (e *Executor) WaitForExecution(executionID string) error {
	execution, exists := e.GetExecution(executionID)
	if !exists {
		return fmt.Errorf("execution not found: %s", executionID)
	}

	<-execution.Done
	return execution.Error
}

// GetStats returns executor statistics
func (e *Executor) GetStats() *ExecutorStats {
	e.statsMu.RLock()
	defer e.statsMu.RUnlock()

	// Return a copy to prevent modifications
	stats := *e.stats
	if e.stats.RunnerStats != nil {
		stats.RunnerStats = make(map[task.RunnerType]*RunnerStats)
		for k, v := range e.stats.RunnerStats {
			statsCopy := *v
			stats.RunnerStats[k] = &statsCopy
		}
	}

	return &stats
}

// Execution methods

// setStatus sets the execution status
func (e *Execution) setStatus(status ExecutionStatus) {
	e.Mux.Lock()
	defer e.Mux.Unlock()
	e.Status = status
}

// finishSuccessfully marks execution as completed successfully
func (e *Execution) finishSuccessfully() {
	e.Mux.Lock()
	defer e.Mux.Unlock()

	e.Status = StatusCompleted
	e.EndTime = &[]time.Time{time.Now()}[0]
	e.Duration = e.EndTime.Sub(e.StartTime)
	e.ExitCode = 0
}

// finishWithError marks execution as failed
func (e *Execution) finishWithError(err error) {
	e.Mux.Lock()
	defer e.Mux.Unlock()

	e.Status = StatusFailed
	e.EndTime = &[]time.Time{time.Now()}[0]
	e.Duration = e.EndTime.Sub(e.StartTime)
	e.Error = err
	e.ExitCode = 1

	// Try to extract exit code from error
	if exitErr, ok := err.(*exec.ExitError); ok {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			e.ExitCode = status.ExitStatus()
		}
	}
}

// finishWithTimeout marks execution as timed out
func (e *Execution) finishWithTimeout() {
	e.Mux.Lock()
	defer e.Mux.Unlock()

	e.Status = StatusTimeout
	e.EndTime = &[]time.Time{time.Now()}[0]
	e.Duration = e.EndTime.Sub(e.StartTime)
	e.Error = context.DeadlineExceeded
	e.ExitCode = 124 // Standard timeout exit code
}

// finishWithCancellation marks execution as cancelled
func (e *Execution) finishWithCancellation() {
	e.Mux.Lock()
	defer e.Mux.Unlock()

	e.Status = StatusCancelled
	e.EndTime = &[]time.Time{time.Now()}[0]
	e.Duration = e.EndTime.Sub(e.StartTime)
	e.Error = context.Canceled
	e.ExitCode = 130 // Standard cancellation exit code
}

// IsCompleted returns whether the execution is completed
func (e *Execution) IsCompleted() bool {
	e.Mux.RLock()
	defer e.Mux.RUnlock()

	return e.Status == StatusCompleted || e.Status == StatusFailed ||
		e.Status == StatusCancelled || e.Status == StatusTimeout
}

// GetStatus returns the current execution status
func (e *Execution) GetStatus() ExecutionStatus {
	e.Mux.RLock()
	defer e.Mux.RUnlock()
	return e.Status
}

// utility functions

// generateExecutionID generates a unique execution ID
func generateExecutionID(task *task.Task) string {
	return fmt.Sprintf("%s_%d", task.ID, time.Now().UnixNano())
}

// newExecutorStats creates new executor statistics
func newExecutorStats() *ExecutorStats {
	return &ExecutorStats{
		RunnerStats: make(map[task.RunnerType]*RunnerStats),
	}
}
