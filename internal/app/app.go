package app

import (
	"context"
	"fmt"
	"sync"

	"github.com/vivalchemy/wake/internal/config"
	"github.com/vivalchemy/wake/internal/discovery"
	"github.com/vivalchemy/wake/internal/output"
	"github.com/vivalchemy/wake/internal/runner"
	"github.com/vivalchemy/wake/pkg/hooks"
	"github.com/vivalchemy/wake/pkg/logger"
	"github.com/vivalchemy/wake/pkg/task"
)

// App represents the main Wake application
type App struct {
	// Core components
	config    *config.Config
	logger    logger.Logger
	Discovery *discovery.Scanner
	runner    *runner.Executor
	output    *output.Manager

	// State management
	tasks   map[string]*task.Task
	tasksMu sync.RWMutex
	Ctx     context.Context
	cancel  context.CancelFunc

	// Event channels
	taskEvents    chan *TaskEvent
	outputEvents  chan *OutputEvent
	discoveryDone chan struct{}
}

// TaskEvent represents a task state change
type TaskEvent struct {
	Task   *task.Task
	Type   TaskEventType
	Error  error
	Output string
}

// TaskEventType represents the type of task event
type TaskEventType int

const (
	TaskEventStarted TaskEventType = iota
	TaskEventFinished
	TaskEventFailed
	TaskEventOutput
)

// OutputEvent represents output from a running task
type OutputEvent struct {
	TaskID string
	Line   string
	IsErr  bool
}

// New creates a new Wake application instance
func New(cfg *config.Config, log logger.Logger) (*App, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}
	if log == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Initialize discovery scanner
	scanner, err := discovery.NewScanner(&cfg.Discovery, log)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("failed to create discovery scanner: %w", err)
	}

	// Initialize output manager
	outputMgr := output.NewManager(cfg.Output.ToManagerOptions(), log)

	// FIXME: Add hook executor options
	hookExecutor := hooks.NewHookExecutor(log, nil)

	// Initialize task runner
	executor := runner.NewExecutor(outputMgr, hookExecutor, log)

	app := &App{
		config:        cfg,
		logger:        log,
		Discovery:     scanner,
		runner:        executor,
		output:        outputMgr,
		tasks:         make(map[string]*task.Task),
		Ctx:           ctx,
		cancel:        cancel,
		taskEvents:    make(chan *TaskEvent, 100),
		outputEvents:  make(chan *OutputEvent, 1000),
		discoveryDone: make(chan struct{}),
	}

	return app, nil
}

func (a *App) GetAppContext(cfg *config.Config, logger logger.Logger) *Context {
	ctx, err := NewContext(cfg, logger)
	if err != nil {
		logger.Error("Failed to create context", "error", err)
		return nil
	}
	return ctx
}

// Initialize performs initial setup and task discovery
func (a *App) Initialize() error {
	a.logger.Info("Initializing Wake application")

	// Start output manager
	if err := a.output.Start(); err != nil {
		return fmt.Errorf("failed to start output manager: %w", err)
	}

	// Start discovery in background
	go a.runDiscovery()

	// Start event processing
	go a.processEvents()

	return nil
}

// Shutdown gracefully shuts down the application
func (a *App) Shutdown() error {
	a.logger.Info("Shutting down Wake application")

	// Cancel context to stop all operations
	a.cancel()

	// Stop all running tasks
	a.tasksMu.RLock()
	for _, t := range a.tasks {
		if t.Status == task.StatusRunning {
			if err := a.runner.Stop(t.ID); err != nil {
				a.logger.Error("Failed to stop task during shutdown", "task", t.ID, "error", err)
			}
		}
	}
	a.tasksMu.RUnlock()

	// Shutdown output manager
	if err := a.output.Stop(); err != nil {
		a.logger.Error("Failed to shutdown output manager", "error", err)
	}

	return nil
}

// GetTasks returns all discovered tasks
func (a *App) GetTasks() []*task.Task {
	a.tasksMu.RLock()
	defer a.tasksMu.RUnlock()

	tasks := make([]*task.Task, 0, len(a.tasks))
	for _, t := range a.tasks {
		tasks = append(tasks, t)
	}

	return tasks
}

// GetTask returns a specific task by ID
func (a *App) GetTask(id string) (*task.Task, bool) {
	a.tasksMu.RLock()
	defer a.tasksMu.RUnlock()

	t, exists := a.tasks[id]
	return t, exists
}

// RunTask executes a task by ID
func (a *App) RunTask(taskID string, env map[string]string) error {
	a.tasksMu.RLock()
	t, exists := a.tasks[taskID]
	a.tasksMu.RUnlock()

	if !exists {
		return fmt.Errorf("task not found: %s", taskID)
	}

	if t.Status == task.StatusRunning {
		return fmt.Errorf("task is already running: %s", taskID)
	}

	// Create execution context
	execCtx := runner.NewExecutionContext(a.Ctx, t, a.runner.GetRunners()[t.Runner], taskID, a.logger)

	// Start the task
	if err := a.runner.Start(execCtx); err != nil {
		return fmt.Errorf("failed to start task %s: %w", taskID, err)
	}

	// Update task status
	a.updateTaskStatus(t, task.StatusRunning)

	// Emit task started event
	a.taskEvents <- &TaskEvent{
		Task: t,
		Type: TaskEventStarted,
	}

	return nil
}

// StopTask stops a running task
func (a *App) StopTask(taskID string) error {
	a.tasksMu.RLock()
	t, exists := a.tasks[taskID]
	a.tasksMu.RUnlock()

	if !exists {
		return fmt.Errorf("task not found: %s", taskID)
	}

	if t.Status != task.StatusRunning {
		return fmt.Errorf("task is not running: %s", taskID)
	}

	if err := a.runner.Stop(taskID); err != nil {
		return fmt.Errorf("failed to stop task %s: %w", taskID, err)
	}

	return nil
}

// GetTaskEvents returns the task events channel
func (a *App) GetTaskEvents() <-chan *TaskEvent {
	return a.taskEvents
}

// GetOutputEvents returns the output events channel
func (a *App) GetOutputEvents() <-chan *OutputEvent {
	return a.outputEvents
}

// WaitForDiscovery blocks until initial task discovery is complete
func (a *App) WaitForDiscovery() <-chan struct{} {
	return a.discoveryDone
}

// runDiscovery performs task discovery in a background goroutine
func (a *App) runDiscovery() {
	defer close(a.discoveryDone)

	a.logger.Info("Starting task discovery")

	// Discover tasks
	discoveredTasks, err := a.Discovery.Discover(a.Ctx)
	if err != nil {
		a.logger.Error("Task discovery failed", "error", err)
		return
	}

	// Update task registry
	a.tasksMu.Lock()
	a.tasks = make(map[string]*task.Task)
	for _, t := range discoveredTasks {
		a.tasks[t.ID] = t
	}
	a.tasksMu.Unlock()

	a.logger.Info("Task discovery completed", "count", len(discoveredTasks))
}

// processEvents handles task and output events
func (a *App) processEvents() {
	for {
		select {
		case <-a.Ctx.Done():
			return

		case event := <-a.taskEvents:
			a.handleTaskEvent(event)

		case event := <-a.outputEvents:
			a.handleOutputEvent(event)
		}
	}
}

// handleTaskEvent processes task state change events
func (a *App) handleTaskEvent(event *TaskEvent) {
	switch event.Type {
	case TaskEventFinished:
		a.updateTaskStatus(event.Task, task.StatusSuccess)
		a.logger.Info("Task completed successfully", "task", event.Task.ID)

	case TaskEventFailed:
		a.updateTaskStatus(event.Task, task.StatusFailed)
		a.logger.Error("Task failed", "task", event.Task.ID, "error", event.Error)
	}
}

// handleOutputEvent processes task output events
func (a *App) handleOutputEvent(event *OutputEvent) {
	data := []byte(event.Line + "\n")
	taskName := "" // Fill if available
	_ = a.output.WriteTaskOutput(event.TaskID, taskName, data, event.IsErr)
}

// updateTaskStatus safely updates a task's status
func (a *App) updateTaskStatus(t *task.Task, status task.Status) {
	a.tasksMu.Lock()
	defer a.tasksMu.Unlock()

	if existingTask, exists := a.tasks[t.ID]; exists {
		existingTask.Status = status
	}
}

// Config returns the application configuration
func (a *App) Config() *config.Config {
	return a.config
}

// Logger returns the application logger
func (a *App) Logger() logger.Logger {
	return a.logger
}
