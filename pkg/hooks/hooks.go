package hooks

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/vivalchemy/wake/internal/config"
	"github.com/vivalchemy/wake/pkg/logger"
	"maps"
)

// HookManager manages pre and post execution hooks
type HookManager struct {
	config *config.HooksConfig
	logger logger.Logger

	// Template processor for hook commands
	templateProcessor *TemplateProcessor

	// Hook execution tracking
	mu          sync.RWMutex
	activeHooks map[string]*HookExecution
	hookStats   *HookStats
}

// HookExecution represents an executing hook
type HookExecution struct {
	ID        string
	Command   string
	Type      HookType
	StartTime time.Time
	EndTime   *time.Time
	ExitCode  int
	Error     error
	Output    string
	Context   *ExecutionContext

	// Control
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// HookType represents the type of hook
type HookType int

const (
	HookTypePreCommand HookType = iota
	HookTypePostCommand
	HookTypePreSuccess
	HookTypePostFailure
)

// String returns the string representation of hook type
func (ht HookType) String() string {
	switch ht {
	case HookTypePreCommand:
		return "pre-command"
	case HookTypePostCommand:
		return "post-command"
	case HookTypePreSuccess:
		return "pre-success"
	case HookTypePostFailure:
		return "post-failure"
	default:
		return "unknown"
	}
}

// HookStats tracks hook execution statistics
type HookStats struct {
	TotalExecuted   int64         `json:"total_executed"`
	TotalSuccessful int64         `json:"total_successful"`
	TotalFailed     int64         `json:"total_failed"`
	TotalTimeout    int64         `json:"total_timeout"`
	AverageExecTime time.Duration `json:"average_exec_time"`
	LastExecution   time.Time     `json:"last_execution"`

	mu sync.RWMutex
}

// TemplateProcessor processes template variables in hook commands
type TemplateProcessor struct {
	logger logger.Logger

	// Template variable patterns
	patterns map[string]*regexp.Regexp
}

// HookResult represents the result of hook execution
type HookResult struct {
	Success  bool
	ExitCode int
	Error    error
	Output   string
	Duration time.Duration
	TimedOut bool
}

// NewHookManager creates a new hook manager
func NewHookManager(cfg *config.RunnersConfig, log logger.Logger) (*HookManager, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Extract hooks config (assuming it's part of runners config or create default)
	hooksConfig := &config.HooksConfig{
		PreCommand:  []string{},
		PostCommand: []string{},
		PreSuccess:  []string{},
		PostFailure: []string{},
		Timeout:     30 * time.Second,
		Environment: make(map[string]string),
	}

	// if cfg.Hooks != nil {
	//     hooksConfig = cfg.Hooks
	// }

	templateProcessor, err := NewTemplateProcessor(log)
	if err != nil {
		return nil, fmt.Errorf("failed to create template processor: %w", err)
	}

	manager := &HookManager{
		config:            hooksConfig,
		logger:            log.WithGroup("hooks"),
		templateProcessor: templateProcessor,
		activeHooks:       make(map[string]*HookExecution),
		hookStats:         &HookStats{},
	}

	return manager, nil
}

// ExecutePreHooks executes all pre-command hooks
func (hm *HookManager) ExecutePreHooks(execCtx *ExecutionContext) error {
	if len(hm.config.PreCommand) == 0 {
		return nil
	}

	hm.logger.Debug("Executing pre-command hooks",
		"task", execCtx.Task.Name,
		"hooks", len(hm.config.PreCommand))

	for i, hookCmd := range hm.config.PreCommand {
		hookID := fmt.Sprintf("pre-%s-%d", execCtx.Task.ID, i)

		result, err := hm.executeHook(hookID, hookCmd, HookTypePreCommand, execCtx)
		if err != nil {
			return fmt.Errorf("pre-command hook %d failed: %w", i, err)
		}

		if !result.Success {
			return fmt.Errorf("pre-command hook %d failed with exit code %d", i, result.ExitCode)
		}
	}

	return nil
}

// ExecutePostHooks executes all post-command hooks
func (hm *HookManager) ExecutePostHooks(execCtx *ExecutionContext, exitCode int, execError error) error {
	// Determine which post hooks to run
	var hooksToRun []string
	hookType := HookTypePostCommand

	if execError == nil && exitCode == 0 {
		// Task succeeded
		hooksToRun = append(hooksToRun, hm.config.PostCommand...)
		if len(hm.config.PreSuccess) > 0 {
			hooksToRun = append(hooksToRun, hm.config.PreSuccess...)
			hookType = HookTypePreSuccess
		}
	} else {
		// Task failed
		hooksToRun = append(hooksToRun, hm.config.PostCommand...)
		if len(hm.config.PostFailure) > 0 {
			hooksToRun = append(hooksToRun, hm.config.PostFailure...)
			hookType = HookTypePostFailure
		}
	}

	if len(hooksToRun) == 0 {
		return nil
	}

	hm.logger.Debug("Executing post-command hooks",
		"task", execCtx.Task.Name,
		"hooks", len(hooksToRun),
		"hook_type", hookType.String())

	// Add execution context for hooks
	hookCtx := hm.createHookContext(execCtx, exitCode, execError)

	for i, hookCmd := range hooksToRun {
		hookID := fmt.Sprintf("post-%s-%d", execCtx.Task.ID, i)

		result, err := hm.executeHookWithContext(hookID, hookCmd, hookType, hookCtx)
		if err != nil {
			hm.logger.Warn("Post-command hook failed",
				"hook", i,
				"error", err,
				"task", execCtx.Task.Name)
			// Don't fail the overall execution for post-hook failures
			continue
		}

		if !result.Success {
			hm.logger.Warn("Post-command hook failed",
				"hook", i,
				"exit_code", result.ExitCode,
				"task", execCtx.Task.Name)
		}
	}

	return nil
}

// executeHook executes a single hook
func (hm *HookManager) executeHook(hookID, command string, hookType HookType, execCtx *ExecutionContext) (*HookResult, error) {
	return hm.executeHookWithContext(hookID, command, hookType, hm.createHookContext(execCtx, 0, nil))
}

// executeHookWithContext executes a hook with additional context
func (hm *HookManager) executeHookWithContext(hookID, command string, hookType HookType, hookCtx map[string]string) (*HookResult, error) {
	// Process template variables in the command
	processedCmd, err := hm.templateProcessor.Process(command, hookCtx)
	if err != nil {
		return nil, fmt.Errorf("failed to process hook template: %w", err)
	}

	hm.logger.Debug("Executing hook",
		"id", hookID,
		"type", hookType.String(),
		"command", processedCmd)

	// Create execution context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), hm.config.Timeout)
	defer cancel()

	// Create hook execution tracker
	hookExec := &HookExecution{
		ID:        hookID,
		Command:   processedCmd,
		Type:      hookType,
		StartTime: time.Now(),
		ctx:       ctx,
		cancel:    cancel,
		done:      make(chan struct{}),
	}

	// Register active hook
	hm.mu.Lock()
	hm.activeHooks[hookID] = hookExec
	hm.mu.Unlock()

	// Execute the hook
	result := hm.runHookCommand(hookExec, hookCtx)

	// Clean up
	hm.mu.Lock()
	delete(hm.activeHooks, hookID)
	hm.mu.Unlock()

	// Update statistics
	hm.updateHookStats(result)

	close(hookExec.done)

	return result, nil
}

// runHookCommand runs the actual hook command
func (hm *HookManager) runHookCommand(hookExec *HookExecution, env map[string]string) *HookResult {
	// Parse command (simple shell parsing)
	parts, err := hm.parseCommand(hookExec.Command)
	if err != nil {
		return &HookResult{
			Success:  false,
			Error:    fmt.Errorf("failed to parse command: %w", err),
			Duration: time.Since(hookExec.StartTime),
		}
	}

	if len(parts) == 0 {
		return &HookResult{
			Success:  false,
			Error:    fmt.Errorf("empty command"),
			Duration: time.Since(hookExec.StartTime),
		}
	}

	// Create command
	cmd := exec.CommandContext(hookExec.ctx, parts[0], parts[1:]...)

	// Set environment
	cmd.Env = hm.buildEnvironment(env)

	// Set working directory from context if available
	if hookExec.Context != nil && hookExec.Context.DiscoveryPath != "" {
		cmd.Dir = hookExec.Context.DiscoveryPath
		cmd.Dir = hookExec.Context.DiscoveryPath
		cmd.Dir = hookExec.Context.Task.WorkingDirectory
	}

	// Capture output
	output, err := cmd.CombinedOutput()
	endTime := time.Now()

	hookExec.EndTime = &endTime
	hookExec.Output = string(output)

	result := &HookResult{
		Output:   string(output),
		Duration: endTime.Sub(hookExec.StartTime),
	}

	if err != nil {
		hookExec.Error = err
		result.Error = err

		// Check if it was a timeout
		if hookExec.ctx.Err() == context.DeadlineExceeded {
			result.TimedOut = true
			result.ExitCode = -1
		} else if exitError, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitError.ExitCode()
		} else {
			result.ExitCode = -1
		}

		result.Success = false
	} else {
		result.Success = true
		result.ExitCode = 0
		hookExec.ExitCode = 0
	}

	hm.logger.Debug("Hook execution completed",
		"id", hookExec.ID,
		"success", result.Success,
		"exit_code", result.ExitCode,
		"duration", result.Duration,
		"timed_out", result.TimedOut)

	return result
}

// createHookContext creates context variables for hooks
func (hm *HookManager) createHookContext(execCtx *ExecutionContext, exitCode int, execError error) map[string]string {
	ctx := make(map[string]string)

	if execCtx != nil && execCtx.Task != nil {
		task := execCtx.Task

		// Task information
		ctx["WAKE_TASK_NAME"] = task.Name
		ctx["WAKE_TASK_ID"] = task.ID
		ctx["WAKE_TASK_RUNNER"] = string(task.Runner)
		ctx["WAKE_TASK_FILE"] = task.FilePath
		ctx["WAKE_TASK_DIR"] = task.WorkingDirectory
		ctx["WAKE_TASK_COMMAND"] = task.Command
		ctx["WAKE_TASK_DESCRIPTION"] = task.Description

		// Task status
		ctx["WAKE_TASK_STATUS"] = string(task.Status)
		ctx["WAKE_EXIT_CODE"] = fmt.Sprintf("%d", exitCode)

		if execError != nil {
			ctx["WAKE_ERROR"] = execError.Error()
		}

		// Tags
		if len(task.Tags) > 0 {
			ctx["WAKE_TASK_TAGS"] = strings.Join(task.Tags, ",")
		}

		// Dependencies
		if len(task.Dependencies) > 0 {
			ctx["WAKE_TASK_DEPS"] = strings.Join(task.Dependencies, ",")
		}
	}

	// Add execution context environment
	if execCtx != nil {
		maps.Copy(ctx, execCtx.Environment)
	}

	// Add hook-specific environment
	maps.Copy(ctx, hm.config.Environment)

	// Add system information
	ctx["WAKE_PID"] = fmt.Sprintf("%d", os.Getpid())
	ctx["WAKE_TIME"] = time.Now().Format(time.RFC3339)

	return ctx
}

// buildEnvironment builds the environment for hook execution
func (hm *HookManager) buildEnvironment(hookCtx map[string]string) []string {
	// Start with system environment
	env := os.Environ()

	// Add hook context variables
	for key, value := range hookCtx {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}

	return env
}

// parseCommand parses a command string into parts
func (hm *HookManager) parseCommand(command string) ([]string, error) {
	// Simple command parsing - in production, you might want more sophisticated parsing
	command = strings.TrimSpace(command)
	if command == "" {
		return nil, fmt.Errorf("empty command")
	}

	// Split by spaces, respecting quotes
	var parts []string
	var current strings.Builder
	inQuotes := false
	escaped := false

	for _, char := range command {
		switch {
		case escaped:
			current.WriteRune(char)
			escaped = false
		case char == '\\':
			escaped = true
		case char == '"' || char == '\'':
			inQuotes = !inQuotes
		case char == ' ' && !inQuotes:
			if current.Len() > 0 {
				parts = append(parts, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(char)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts, nil
}

// updateHookStats updates hook execution statistics
func (hm *HookManager) updateHookStats(result *HookResult) {
	hm.hookStats.mu.Lock()
	defer hm.hookStats.mu.Unlock()

	hm.hookStats.TotalExecuted++
	hm.hookStats.LastExecution = time.Now()

	if result.Success {
		hm.hookStats.TotalSuccessful++
	} else {
		hm.hookStats.TotalFailed++
	}

	if result.TimedOut {
		hm.hookStats.TotalTimeout++
	}

	// Update average execution time
	if hm.hookStats.TotalExecuted == 1 {
		hm.hookStats.AverageExecTime = result.Duration
	} else {
		// Running average calculation
		total := time.Duration(hm.hookStats.TotalExecuted)
		hm.hookStats.AverageExecTime = (hm.hookStats.AverageExecTime*(total-1) + result.Duration) / total
	}
}

// GetActiveHooks returns currently executing hooks
func (hm *HookManager) GetActiveHooks() map[string]*HookExecution {
	hm.mu.RLock()
	defer hm.mu.RUnlock()

	// Return a copy to prevent external modification
	active := make(map[string]*HookExecution)
	maps.Copy(active, hm.activeHooks)

	return active
}

// GetHookStats returns hook execution statistics
func (hm *HookManager) GetHookStats() *HookStats {
	hm.hookStats.mu.RLock()
	defer hm.hookStats.mu.RUnlock()

	// Return a copy
	stats := *hm.hookStats
	return &stats
}

// CancelHook cancels a running hook
func (hm *HookManager) CancelHook(hookID string) error {
	hm.mu.RLock()
	hookExec, exists := hm.activeHooks[hookID]
	hm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("hook not found: %s", hookID)
	}

	hm.logger.Info("Cancelling hook", "id", hookID)
	hookExec.cancel()

	// Wait for hook to complete
	select {
	case <-hookExec.done:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout waiting for hook to cancel")
	}
}

// ValidateHooks validates hook configuration
func (hm *HookManager) ValidateHooks() error {
	allHooks := append(hm.config.PreCommand, hm.config.PostCommand...)
	allHooks = append(allHooks, hm.config.PreSuccess...)
	allHooks = append(allHooks, hm.config.PostFailure...)

	for i, hook := range allHooks {
		if strings.TrimSpace(hook) == "" {
			return fmt.Errorf("hook %d is empty", i)
		}

		// Validate template syntax
		if err := hm.templateProcessor.Validate(hook); err != nil {
			return fmt.Errorf("hook %d has invalid template syntax: %w", i, err)
		}
	}

	return nil
}

// NewTemplateProcessor creates a new template processor
func NewTemplateProcessor(log logger.Logger) (*TemplateProcessor, error) {
	processor := &TemplateProcessor{
		logger:   log.WithGroup("template"),
		patterns: make(map[string]*regexp.Regexp),
	}

	// Compile template patterns
	var err error
	processor.patterns["variable"], err = regexp.Compile(`\$\{([^}]+)\}`)
	if err != nil {
		return nil, fmt.Errorf("failed to compile variable pattern: %w", err)
	}

	processor.patterns["command"], err = regexp.Compile(`\$\(([^)]+)\)`)
	if err != nil {
		return nil, fmt.Errorf("failed to compile command pattern: %w", err)
	}

	return processor, nil
}

// Process processes a template string with the given context
func (tp *TemplateProcessor) Process(template string, ctx map[string]string) (string, error) {
	result := template

	// Replace ${VAR} patterns
	result = tp.patterns["variable"].ReplaceAllStringFunc(result, func(match string) string {
		// Extract variable name
		varName := match[2 : len(match)-1] // Remove ${ and }

		if value, exists := ctx[varName]; exists {
			return value
		}

		// Check environment variables as fallback
		if value := os.Getenv(varName); value != "" {
			return value
		}

		tp.logger.Warn("Template variable not found", "variable", varName)
		return match // Keep original if not found
	})

	// Replace $(CMD) patterns (command substitution)
	result = tp.patterns["command"].ReplaceAllStringFunc(result, func(match string) string {
		cmdStr := match[2 : len(match)-1] // Remove $( and )

		// Execute command and return output
		cmd := exec.Command("sh", "-c", cmdStr)
		output, err := cmd.Output()
		if err != nil {
			tp.logger.Warn("Command substitution failed", "command", cmdStr, "error", err)
			return match // Keep original if command fails
		}

		return strings.TrimSpace(string(output))
	})

	return result, nil
}

// Validate validates template syntax
func (tp *TemplateProcessor) Validate(template string) error {
	// Check for unmatched braces/parentheses
	braceCount := 0
	parenCount := 0
	inVariable := false
	inCommand := false

	for i, char := range template {
		switch char {
		case '$':
			if i+1 < len(template) {
				next := template[i+1]
				if next == '{' {
					inVariable = true
				} else if next == '(' {
					inCommand = true
				}
			}
		case '{':
			if inVariable {
				braceCount++
				inVariable = false
			}
		case '}':
			braceCount--
			if braceCount < 0 {
				return fmt.Errorf("unmatched closing brace at position %d", i)
			}
		case '(':
			if inCommand {
				parenCount++
				inCommand = false
			}
		case ')':
			parenCount--
			if parenCount < 0 {
				return fmt.Errorf("unmatched closing parenthesis at position %d", i)
			}
		}
	}

	if braceCount != 0 {
		return fmt.Errorf("unmatched braces in template")
	}

	if parenCount != 0 {
		return fmt.Errorf("unmatched parentheses in template")
	}

	return nil
}
