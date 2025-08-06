package hooks

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"maps"
	"slices"

	"github.com/google/uuid"
	"github.com/vivalchemy/wake/pkg/logger"
	"github.com/vivalchemy/wake/pkg/task"
)

// HookExecutor manages and executes hooks
type Executor struct {
	Hooks   map[HookTrigger][]*Hook
	mu      sync.RWMutex
	logger  logger.Logger
	options *ExecutorOptions
}

// ExecutorOptions contains configuration for the hook executor
type ExecutorOptions struct {
	DefaultTimeout   time.Duration
	MaxConcurrency   int
	ContinueOnError  bool
	Environment      map[string]string
	Shell            string
	WorkingDirectory string
	Verbose          bool
}

// Hook represents a single hook
type Hook struct {
	// Identification
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`

	// Trigger configuration
	Trigger   HookTrigger `json:"trigger"`
	Condition string      `json:"condition,omitempty"`

	// Execution configuration
	Command string   `json:"command"`
	Args    []string `json:"args,omitempty"`
	Script  string   `json:"script,omitempty"`
	Shell   string   `json:"shell,omitempty"`

	// Environment
	WorkingDirectory string            `json:"working_directory,omitempty"`
	Environment      map[string]string `json:"environment,omitempty"`
	InheritEnv       bool              `json:"inherit_env"`

	// Execution options
	Timeout      time.Duration `json:"timeout,omitempty"`
	Async        bool          `json:"async"`
	IgnoreErrors bool          `json:"ignore_errors"`
	RunOnce      bool          `json:"run_once"`

	// Filters
	TaskFilter   string   `json:"task_filter,omitempty"`
	RunnerFilter []string `json:"runner_filter,omitempty"`
	PathFilter   string   `json:"path_filter,omitempty"`

	// State tracking
	Enabled      bool      `json:"enabled"`
	ExecutedAt   time.Time `json:"executed_at"`
	ExecuteCount int       `json:"execute_count"`

	// Metadata
	Tags     []string          `json:"tags,omitempty"`
	Labels   map[string]string `json:"labels,omitempty"`
	Metadata map[string]any    `json:"metadata,omitempty"`
}

// HookTrigger represents when a hook should be executed
type HookTrigger string

const (
	TriggerPreDiscovery  HookTrigger = "pre_discovery"
	TriggerPostDiscovery HookTrigger = "post_discovery"
	TriggerPreRun        HookTrigger = "pre_run"
	TriggerPostRun       HookTrigger = "post_run"
	TriggerPreTask       HookTrigger = "pre_task"
	TriggerPostTask      HookTrigger = "post_task"
	TriggerOnSuccess     HookTrigger = "on_success"
	TriggerOnError       HookTrigger = "on_error"
	TriggerOnStart       HookTrigger = "on_start"
	TriggerOnExit        HookTrigger = "on_exit"
	TriggerOnChange      HookTrigger = "on_change"
	TriggerScheduled     HookTrigger = "scheduled"
)

// ExecutionContext contains context information for hook execution
type ExecutionContext struct {
	// Core context
	Trigger     HookTrigger `json:"trigger"`
	Timestamp   time.Time   `json:"timestamp"`
	ExecutionID string      `json:"execution_id"`

	// Task context
	Task *task.Task `json:"task,omitempty"`

	// Execution context
	ExitCode int           `json:"exit_code,omitempty"`
	Success  bool          `json:"success"`
	Duration time.Duration `json:"duration,omitempty"`
	Error    string        `json:"error,omitempty"`
	Output   string        `json:"output,omitempty"`

	// Discovery context
	DiscoveredTasks int    `json:"discovered_tasks,omitempty"`
	DiscoveryPath   string `json:"discovery_path,omitempty"`

	// Environment
	Environment map[string]string `json:"environment,omitempty"`
	Variables   map[string]any    `json:"variables,omitempty"`
}

// ExecutionResult represents the result of hook execution
type ExecutionResult struct {
	// Hook information
	HookID   string      `json:"hook_id"`
	HookName string      `json:"hook_name"`
	Trigger  HookTrigger `json:"trigger"`

	// Execution details
	StartTime time.Time     `json:"start_time"`
	EndTime   time.Time     `json:"end_time"`
	Duration  time.Duration `json:"duration"`
	Async     bool          `json:"async"`

	// Results
	Success  bool   `json:"success"`
	ExitCode int    `json:"exit_code"`
	Output   string `json:"output,omitempty"`
	Error    string `json:"error,omitempty"`

	// Process information
	PID       int      `json:"pid,omitempty"`
	Command   string   `json:"command"`
	Arguments []string `json:"arguments,omitempty"`

	// Context
	Context *ExecutionContext `json:"context,omitempty"`
}

// NewHookExecutor creates a new hook executor
func NewHookExecutor(logger logger.Logger, options *ExecutorOptions) *Executor {
	if options == nil {
		options = &ExecutorOptions{
			DefaultTimeout:  30 * time.Second,
			MaxConcurrency:  5,
			ContinueOnError: true,
			// InheritEnv:      true,
			Shell: getDefaultShell(),
		}
	}

	return &Executor{
		Hooks:   make(map[HookTrigger][]*Hook),
		logger:  logger.WithGroup("hook-executor"),
		options: options,
	}
}

// Register registers a new hook
func (he *Executor) Register(hook *Hook) error {
	if hook.ID == "" {
		hook.ID = generateHookID()
	}

	if hook.Name == "" {
		hook.Name = hook.ID
	}

	// Validate hook
	if err := he.validateHook(hook); err != nil {
		return fmt.Errorf("invalid hook: %w", err)
	}

	// Set defaults
	if hook.Timeout == 0 {
		hook.Timeout = he.options.DefaultTimeout
	}

	if hook.Shell == "" {
		hook.Shell = he.options.Shell
	}

	hook.Enabled = true
	hook.InheritEnv = true

	he.mu.Lock()
	defer he.mu.Unlock()

	// Add to trigger map
	he.Hooks[hook.Trigger] = append(he.Hooks[hook.Trigger], hook)

	he.logger.Debug("Registered hook",
		"id", hook.ID,
		"name", hook.Name,
		"trigger", hook.Trigger)

	return nil
}

// Unregister removes a hook
func (he *Executor) Unregister(hookID string) bool {
	he.mu.Lock()
	defer he.mu.Unlock()

	for trigger, hooks := range he.Hooks {
		for i, hook := range hooks {
			if hook.ID == hookID {
				// Remove hook from slice
				he.Hooks[trigger] = slices.Delete(hooks, i, i+1)
				he.logger.Debug("Unregistered hook", "id", hookID)
				return true
			}
		}
	}

	return false
}

func GenerateExecutionID() string {
	timestamp := time.Now().UTC().Format("20060102T150405Z")
	id := uuid.New().String()
	return fmt.Sprintf("exec-%s-%s", timestamp, id)
}

// Execute executes all hooks for a given trigger
func (he *Executor) Execute(ctx context.Context, trigger HookTrigger, execCtx *ExecutionContext) ([]*ExecutionResult, error) {
	he.mu.RLock()
	hooks := he.Hooks[trigger]
	he.mu.RUnlock()

	if len(hooks) == 0 {
		return nil, nil
	}

	he.logger.Debug("Executing hooks", "trigger", trigger, "count", len(hooks))

	var results []*ExecutionResult
	var errors []error

	// Execute hooks based on concurrency settings
	if he.options.MaxConcurrency <= 1 {
		// Sequential execution
		for _, hook := range hooks {
			if !he.shouldExecuteHook(hook, execCtx) {
				continue
			}

			result, err := he.executeHook(ctx, hook, execCtx)
			if result != nil {
				results = append(results, result)
			}

			if err != nil {
				errors = append(errors, err)
				if !he.options.ContinueOnError {
					break
				}
			}
		}
	} else {
		// Concurrent execution
		results, errors = he.executeConcurrent(ctx, hooks, execCtx)
	}

	// Handle errors
	if len(errors) > 0 && !he.options.ContinueOnError {
		return results, fmt.Errorf("hook execution failed: %v", errors[0])
	}

	return results, nil
}

// ExecuteHook executes a specific hook
func (he *Executor) ExecuteHook(ctx context.Context, hookID string, execCtx *ExecutionContext) (*ExecutionResult, error) {
	he.mu.RLock()
	defer he.mu.RUnlock()

	// Find hook by ID
	for _, hooks := range he.Hooks {
		for _, hook := range hooks {
			if hook.ID == hookID {
				if !he.shouldExecuteHook(hook, execCtx) {
					return nil, fmt.Errorf("hook execution conditions not met")
				}
				return he.executeHook(ctx, hook, execCtx)
			}
		}
	}

	return nil, fmt.Errorf("hook not found: %s", hookID)
}

func (he *Executor) ExecutePreHooks(ctx context.Context, task *task.Task) error {
	preHookTriggers := []HookTrigger{
		TriggerPreDiscovery,
		TriggerPreRun,
		TriggerPreTask,
		TriggerOnStart,
		TriggerScheduled,
	}

	for _, trigger := range preHookTriggers {
		he.mu.RLock()
		hooks, exists := he.Hooks[trigger]
		he.mu.RUnlock()

		if !exists {
			continue
		}

		// Construct ExecutionContext
		execCtx := &ExecutionContext{
			Trigger:     trigger,
			Timestamp:   time.Now(),
			ExecutionID: GenerateExecutionID(), // assume this exists
			Task:        task,
			Environment: task.Environment,
			Variables:   map[string]any{},
		}

		for _, hook := range hooks {
			result, err := he.ExecuteHook(ctx, hook.ID, execCtx)
			if err != nil {
				he.logger.Warn("Pre-hook execution failed", "trigger", trigger, "hook", hook.ID, "error", err)
				return fmt.Errorf("failed to execute pre-hook '%s': %w", hook.ID, err)
			}
			he.logger.Debug("Executed pre-hook", "trigger", trigger, "hook", hook.ID, "result", result)
		}
	}

	return nil
}

func (he *Executor) ExecutePostHooks(ctx context.Context, task *task.Task) error {
	postHookTriggers := []HookTrigger{
		TriggerPostDiscovery,
		TriggerPostRun,
		TriggerPostTask,
		TriggerOnSuccess,
		TriggerOnError,
		TriggerOnExit,
		TriggerOnChange,
	}

	for _, trigger := range postHookTriggers {
		he.mu.RLock()
		hooks, exists := he.Hooks[trigger]
		he.mu.RUnlock()

		if !exists {
			continue
		}

		// Construct ExecutionContext
		execCtx := &ExecutionContext{
			Trigger:     trigger,
			Timestamp:   time.Now(),
			ExecutionID: GenerateExecutionID(), // assume this exists
			Task:        task,
			Environment: task.Environment,
			Variables:   map[string]any{},
		}

		for _, hook := range hooks {
			result, err := he.ExecuteHook(ctx, hook.ID, execCtx)
			if err != nil {
				he.logger.Warn("Post-hook execution failed", "trigger", trigger, "hook", hook.ID, "error", err)
				return fmt.Errorf("failed to execute pre-hook '%s': %w", hook.ID, err)
			}
			he.logger.Debug("Executed post-hook", "trigger", trigger, "hook", hook.ID, "result", result)
		}
	}

	return nil
}

// GetHooks returns all registered hooks
func (he *Executor) GetHooks() map[HookTrigger][]*Hook {
	he.mu.RLock()
	defer he.mu.RUnlock()

	// Return a deep copy
	result := make(map[HookTrigger][]*Hook)
	for trigger, hooks := range he.Hooks {
		result[trigger] = make([]*Hook, len(hooks))
		for i, hook := range hooks {
			hookCopy := *hook
			result[trigger][i] = &hookCopy
		}
	}

	return result
}

// GetHooksByTrigger returns hooks for a specific trigger
func (he *Executor) GetHooksByTrigger(trigger HookTrigger) []*Hook {
	he.mu.RLock()
	defer he.mu.RUnlock()

	hooks := he.Hooks[trigger]
	result := make([]*Hook, len(hooks))
	for i, hook := range hooks {
		hookCopy := *hook
		result[i] = &hookCopy
	}

	return result
}

// EnableHook enables a hook
func (he *Executor) EnableHook(hookID string) bool {
	return he.setHookEnabled(hookID, true)
}

// DisableHook disables a hook
func (he *Executor) DisableHook(hookID string) bool {
	return he.setHookEnabled(hookID, false)
}

// Private methods

// executeHook executes a single hook
func (he *Executor) executeHook(ctx context.Context, hook *Hook, execCtx *ExecutionContext) (*ExecutionResult, error) {
	startTime := time.Now()

	result := &ExecutionResult{
		HookID:    hook.ID,
		HookName:  hook.Name,
		Trigger:   hook.Trigger,
		StartTime: startTime,
		Async:     hook.Async,
		Context:   execCtx,
	}

	he.logger.Debug("Executing hook", "id", hook.ID, "name", hook.Name)

	// Create execution context with timeout
	hookCtx := ctx
	if hook.Timeout > 0 {
		var cancel context.CancelFunc
		hookCtx, cancel = context.WithTimeout(ctx, hook.Timeout)
		defer cancel()
	}

	// Build command
	cmd, err := he.buildCommand(hookCtx, hook, execCtx)
	if err != nil {
		result.Error = fmt.Sprintf("failed to build command: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		return result, err
	}

	result.Command = cmd.Path
	result.Arguments = cmd.Args[1:] // Skip the executable

	// Execute command
	if hook.Async {
		// Asynchronous execution
		go func() {
			he.runCommand(cmd, result, hook)
		}()

		// For async hooks, return immediately with basic info
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		result.Success = true

		he.logger.Debug("Started async hook", "id", hook.ID)

	} else {
		// Synchronous execution
		he.runCommand(cmd, result, hook)

		he.logger.Debug("Completed hook",
			"id", hook.ID,
			"success", result.Success,
			"duration", result.Duration)
	}

	// Update hook execution tracking
	he.updateHookExecution(hook, result)

	if !result.Success && !hook.IgnoreErrors {
		return result, fmt.Errorf("hook %s failed: %s", hook.ID, result.Error)
	}

	return result, nil
}

// runCommand runs the actual command
func (he *Executor) runCommand(cmd *exec.Cmd, result *ExecutionResult, _ *Hook) {
	// Start command
	if err := cmd.Start(); err != nil {
		result.Error = fmt.Sprintf("failed to start command: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		return
	}

	result.PID = cmd.Process.Pid

	// Wait for completion
	err := cmd.Wait()
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	// Handle results
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitError.ExitCode()
		}
		result.Error = err.Error()
	} else {
		result.Success = true
	}

	// Get output if captured
	if output, err := cmd.CombinedOutput(); err == nil {
		result.Output = string(output)
	}
}

// buildCommand builds the command to execute
func (he *Executor) buildCommand(ctx context.Context, hook *Hook, execCtx *ExecutionContext) (*exec.Cmd, error) {
	var cmd *exec.Cmd

	if hook.Script != "" {
		// Execute script
		cmd = exec.CommandContext(ctx, hook.Shell, "-c", hook.Script)
	} else if hook.Command != "" {
		// Execute command with args
		if len(hook.Args) > 0 {
			cmd = exec.CommandContext(ctx, hook.Command, hook.Args...)
		} else {
			cmd = exec.CommandContext(ctx, hook.Command)
		}
	} else {
		return nil, fmt.Errorf("hook has no command or script")
	}

	// Set working directory
	workDir := hook.WorkingDirectory
	if workDir == "" {
		workDir = he.options.WorkingDirectory
	}
	if workDir != "" {
		cmd.Dir = workDir
	}

	// Set environment
	env := he.buildEnvironment(hook, execCtx)
	cmd.Env = env

	return cmd, nil
}

// buildEnvironment builds the environment for hook execution
func (he *Executor) buildEnvironment(hook *Hook, execCtx *ExecutionContext) []string {
	var env []string

	// Start with system environment if inheriting
	if hook.InheritEnv {
		env = os.Environ()
	}

	// Add executor default environment
	for key, value := range he.options.Environment {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}

	// Add hook-specific environment
	for key, value := range hook.Environment {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}

	// Add context variables
	if execCtx != nil {
		env = append(env, fmt.Sprintf("WAKE_HOOK_TRIGGER=%s", execCtx.Trigger))
		env = append(env, fmt.Sprintf("WAKE_EXECUTION_ID=%s", execCtx.ExecutionID))

		if execCtx.Task.ID != "" {
			env = append(env, fmt.Sprintf("WAKE_TASK_ID=%s", execCtx.Task.ID))
		}
		if execCtx.Task.Name != "" {
			env = append(env, fmt.Sprintf("WAKE_TASK_NAME=%s", execCtx.Task.Name))
		}
		if execCtx.Task.Runner != "" {
			env = append(env, fmt.Sprintf("WAKE_TASK_RUNNER=%s", execCtx.Task.Runner))
		}
		if execCtx.Task.FilePath != "" {
			env = append(env, fmt.Sprintf("WAKE_TASK_PATH=%s", execCtx.Task.FilePath))
		}

		env = append(env, fmt.Sprintf("WAKE_SUCCESS=%t", execCtx.Success))
		env = append(env, fmt.Sprintf("WAKE_EXIT_CODE=%d", execCtx.ExitCode))

		if execCtx.Duration > 0 {
			env = append(env, fmt.Sprintf("WAKE_DURATION=%s", execCtx.Duration))
		}

		// Add custom variables
		for key, value := range execCtx.Variables {
			if strValue, ok := value.(string); ok {
				env = append(env, fmt.Sprintf("WAKE_%s=%s", strings.ToUpper(key), strValue))
			}
		}
	}

	return env
}

// shouldExecuteHook determines if a hook should be executed
func (he *Executor) shouldExecuteHook(hook *Hook, execCtx *ExecutionContext) bool {
	if !hook.Enabled {
		return false
	}

	// Check run-once constraint
	if hook.RunOnce && hook.ExecuteCount > 0 {
		return false
	}

	// Check task filter
	if hook.TaskFilter != "" && execCtx != nil && execCtx.Task.Name != "" {
		if !he.matchesPattern(execCtx.Task.Name, hook.TaskFilter) {
			return false
		}
	}

	// Check runner filter
	if len(hook.RunnerFilter) > 0 && execCtx != nil && execCtx.Task.Runner != "" {
		found := slices.Contains(hook.RunnerFilter, execCtx.Task.Runner.String())
		if !found {
			return false
		}
	}

	// Check path filter
	if hook.PathFilter != "" && execCtx != nil && execCtx.Task.FilePath != "" {
		if !he.matchesPattern(execCtx.Task.FilePath, hook.PathFilter) {
			return false
		}
	}

	// Check condition (simple evaluation)
	if hook.Condition != "" {
		if !he.evaluateCondition(hook.Condition, execCtx) {
			return false
		}
	}

	return true
}

// executeConcurrent executes hooks concurrently
func (he *Executor) executeConcurrent(ctx context.Context, hooks []*Hook, execCtx *ExecutionContext) ([]*ExecutionResult, []error) {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var results []*ExecutionResult
	var errors []error

	// Create semaphore to limit concurrency
	semaphore := make(chan struct{}, he.options.MaxConcurrency)

	for _, hook := range hooks {
		if !he.shouldExecuteHook(hook, execCtx) {
			continue
		}

		wg.Add(1)
		go func(h *Hook) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			result, err := he.executeHook(ctx, h, execCtx)

			mu.Lock()
			if result != nil {
				results = append(results, result)
			}
			if err != nil {
				errors = append(errors, err)
			}
			mu.Unlock()
		}(hook)
	}

	wg.Wait()
	return results, errors
}

// validateHook validates a hook configuration
func (he *Executor) validateHook(hook *Hook) error {
	if hook.Command == "" && hook.Script == "" {
		return fmt.Errorf("hook must have either command or script")
	}

	if hook.Command != "" && hook.Script != "" {
		return fmt.Errorf("hook cannot have both command and script")
	}

	if hook.Trigger == "" {
		return fmt.Errorf("hook must have a trigger")
	}

	return nil
}

// setHookEnabled enables or disables a hook
func (he *Executor) setHookEnabled(hookID string, enabled bool) bool {
	he.mu.Lock()
	defer he.mu.Unlock()

	for _, hooks := range he.Hooks {
		for _, hook := range hooks {
			if hook.ID == hookID {
				hook.Enabled = enabled
				return true
			}
		}
	}

	return false
}

// updateHookExecution updates hook execution tracking
func (he *Executor) updateHookExecution(hook *Hook, result *ExecutionResult) {
	hook.ExecutedAt = result.EndTime
	hook.ExecuteCount++
}

// matchesPattern checks if text matches a simple pattern
func (he *Executor) matchesPattern(text, pattern string) bool {
	// Simple wildcard matching
	return strings.Contains(strings.ToLower(text), strings.ToLower(pattern))
}

// evaluateCondition evaluates a simple condition
func (he *Executor) evaluateCondition(condition string, execCtx *ExecutionContext) bool {
	// Simple condition evaluation - could be enhanced with a proper expression parser
	condition = strings.TrimSpace(condition)

	// Handle simple cases
	switch condition {
	case "success":
		return execCtx != nil && execCtx.Success
	case "failure", "error":
		return execCtx != nil && !execCtx.Success
	case "always":
		return true
	case "never":
		return false
	}

	// Default to true for unknown conditions
	return true
}

// Utility functions

// generateHookID generates a unique hook ID
func generateHookID() string {
	return fmt.Sprintf("hook_%d", time.Now().UnixNano())
}

// getDefaultShell returns the default shell for the platform
func getDefaultShell() string {
	if shell := os.Getenv("SHELL"); shell != "" {
		return shell
	}

	// Platform defaults
	if os.PathSeparator == '\\' {
		return "cmd"
	}
	return "/bin/sh"
}

// NewExecutionContext creates a new execution context
func NewExecutionContext(trigger HookTrigger) *ExecutionContext {
	return &ExecutionContext{
		Trigger:     trigger,
		Timestamp:   time.Now(),
		ExecutionID: fmt.Sprintf("exec_%d", time.Now().UnixNano()),
		Environment: make(map[string]string),
		Variables:   make(map[string]any),
	}
}

// SetTaskContext sets task-related context
func (ec *ExecutionContext) SetTaskContext(taskID, taskName, taskRunner, taskPath string) {
	ec.Task.ID = taskID
	ec.Task.Name = taskName
	ec.Task.Runner = task.RunnerType(taskRunner)
	ec.Task.FilePath = taskPath
}

// SetExecutionResult sets execution result context
func (ec *ExecutionContext) SetExecutionResult(success bool, exitCode int, duration time.Duration, err error, output string) {
	ec.Success = success
	ec.ExitCode = exitCode
	ec.Duration = duration
	ec.Output = output

	if err != nil {
		ec.Error = err.Error()
	}
}

// SetDiscoveryContext sets discovery-related context
func (ec *ExecutionContext) SetDiscoveryContext(discoveredTasks int, discoveryPath string) {
	ec.DiscoveredTasks = discoveredTasks
	ec.DiscoveryPath = discoveryPath
}

// SetVariable sets a custom variable
func (ec *ExecutionContext) SetVariable(key string, value any) {
	if ec.Variables == nil {
		ec.Variables = make(map[string]any)
	}
	ec.Variables[key] = value
}

// SetEnvironment sets an environment variable
func (ec *ExecutionContext) SetEnvironment(key, value string) {
	if ec.Environment == nil {
		ec.Environment = make(map[string]string)
	}
	ec.Environment[key] = value
}

// NewHook creates a new hook with basic configuration
func NewHook(name string, trigger HookTrigger, command string) *Hook {
	return &Hook{
		ID:          generateHookID(),
		Name:        name,
		Trigger:     trigger,
		Command:     command,
		Enabled:     true,
		InheritEnv:  true,
		Timeout:     30 * time.Second,
		Environment: make(map[string]string),
		Labels:      make(map[string]string),
		Metadata:    make(map[string]any),
	}
}

// NewScriptHook creates a new script hook
func NewScriptHook(name string, trigger HookTrigger, script string) *Hook {
	return &Hook{
		ID:          generateHookID(),
		Name:        name,
		Trigger:     trigger,
		Script:      script,
		Enabled:     true,
		InheritEnv:  true,
		Timeout:     30 * time.Second,
		Shell:       getDefaultShell(),
		Environment: make(map[string]string),
		Labels:      make(map[string]string),
		Metadata:    make(map[string]any),
	}
}

// WithTimeout sets the hook timeout
func (h *Hook) WithTimeout(timeout time.Duration) *Hook {
	h.Timeout = timeout
	return h
}

// WithWorkingDirectory sets the working directory
func (h *Hook) WithWorkingDirectory(dir string) *Hook {
	h.WorkingDirectory = dir
	return h
}

// WithEnvironment adds environment variables
func (h *Hook) WithEnvironment(env map[string]string) *Hook {
	if h.Environment == nil {
		h.Environment = make(map[string]string)
	}
	maps.Copy(h.Environment, env)
	return h
}

// WithAsync makes the hook asynchronous
func (h *Hook) WithAsync() *Hook {
	h.Async = true
	return h
}

// WithIgnoreErrors makes the hook ignore errors
func (h *Hook) WithIgnoreErrors() *Hook {
	h.IgnoreErrors = true
	return h
}

// WithRunOnce makes the hook run only once
func (h *Hook) WithRunOnce() *Hook {
	h.RunOnce = true
	return h
}

// WithCondition sets a condition for hook execution
func (h *Hook) WithCondition(condition string) *Hook {
	h.Condition = condition
	return h
}

// WithTaskFilter sets a task filter
func (h *Hook) WithTaskFilter(filter string) *Hook {
	h.TaskFilter = filter
	return h
}

// WithRunnerFilter sets a runner filter
func (h *Hook) WithRunnerFilter(runners ...string) *Hook {
	h.RunnerFilter = runners
	return h
}

// WithPathFilter sets a path filter
func (h *Hook) WithPathFilter(filter string) *Hook {
	h.PathFilter = filter
	return h
}
