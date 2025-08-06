package runner

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/vivalchemy/wake/pkg/logger"
	"github.com/vivalchemy/wake/pkg/task"
)

// ExecutionContext provides context and environment for task execution
type ExecutionContext struct {
	// Task information
	Task   *task.Task
	Runner Runner

	// Execution metadata
	ExecutionID string
	StartTime   time.Time
	WorkingDir  string

	// Environment
	Environment map[string]string
	Variables   map[string]any

	// Context and cancellation
	Context context.Context
	Cancel  context.CancelFunc

	// Configuration
	Options *ContextOptions

	// State
	prepared bool
	mu       sync.RWMutex

	// Logger
	logger logger.Logger

	// Timeout
	Timeout time.Duration
}

// ContextOptions contains configuration for execution context
type ContextOptions struct {
	// Environment handling
	InheritEnvironment   bool
	ClearEnvironment     bool
	EnvironmentOverrides map[string]string
	EnvironmentBlacklist []string
	EnvironmentWhitelist []string

	// Working directory
	UseTaskDirectory bool
	CustomWorkingDir string
	CreateWorkingDir bool

	// Variable expansion
	ExpandVariables   bool
	VariablePrefix    string
	CaseSensitiveVars bool

	// Path handling
	PrependPath   []string
	AppendPath    []string
	PathSeparator string

	// Timeout and limits
	ExecutionTimeout   time.Duration
	PreparationTimeout time.Duration

	// Security
	RestrictedMode    bool
	AllowedCommands   []string
	ForbiddenCommands []string

	// Logging
	LogEnvironment bool
	LogVariables   bool
	LogWorkingDir  bool
}

// VariableResolver handles variable resolution and expansion
type VariableResolver struct {
	variables   map[string]any
	environment map[string]string
	options     *ContextOptions
	logger      logger.Logger
}

// NewExecutionContext creates a new execution context
func NewExecutionContext(ctx context.Context, task *task.Task, runner Runner, executionID string, logger logger.Logger) *ExecutionContext {
	execCtx, cancel := context.WithCancel(ctx)

	execContext := &ExecutionContext{
		Task:        task,
		Runner:      runner,
		ExecutionID: executionID,
		StartTime:   time.Now(),
		Context:     execCtx,
		Cancel:      cancel,
		Environment: make(map[string]string),
		Variables:   make(map[string]any),
		Options:     createDefaultContextOptions(),
		logger:      logger.WithGroup("execution-context"),
	}

	return execContext
}

// createDefaultContextOptions creates default context options
func createDefaultContextOptions() *ContextOptions {
	return &ContextOptions{
		InheritEnvironment:   true,
		ClearEnvironment:     false,
		EnvironmentOverrides: make(map[string]string),
		EnvironmentBlacklist: []string{},
		EnvironmentWhitelist: []string{},
		UseTaskDirectory:     true,
		CustomWorkingDir:     "",
		CreateWorkingDir:     false,
		ExpandVariables:      true,
		VariablePrefix:       "$",
		CaseSensitiveVars:    true,
		PrependPath:          []string{},
		AppendPath:           []string{},
		PathSeparator:        string(os.PathListSeparator),
		ExecutionTimeout:     5 * time.Minute,
		PreparationTimeout:   30 * time.Second,
		RestrictedMode:       false,
		AllowedCommands:      []string{},
		ForbiddenCommands:    []string{},
		LogEnvironment:       false,
		LogVariables:         false,
		LogWorkingDir:        true,
	}
}

// SetOptions sets context options
func (ec *ExecutionContext) SetOptions(options *ContextOptions) {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	ec.Options = options
}

// Prepare prepares the execution context
func (ec *ExecutionContext) Prepare() error {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	if ec.prepared {
		return nil
	}

	ec.logger.Debug("Preparing execution context", "task", ec.Task.Name, "execution", ec.ExecutionID)

	// Set up timeout for preparation
	ctx := ec.Context
	if ec.Options.PreparationTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, ec.Options.PreparationTimeout)
		defer cancel()
	}

	// Prepare working directory
	if err := ec.prepareWorkingDirectory(ctx); err != nil {
		return fmt.Errorf("failed to prepare working directory: %w", err)
	}

	// Prepare environment
	if err := ec.prepareEnvironment(ctx); err != nil {
		return fmt.Errorf("failed to prepare environment: %w", err)
	}

	// Prepare variables
	if err := ec.prepareVariables(ctx); err != nil {
		return fmt.Errorf("failed to prepare variables: %w", err)
	}

	// Security checks
	if err := ec.performSecurityChecks(ctx); err != nil {
		return fmt.Errorf("security check failed: %w", err)
	}

	ec.prepared = true
	ec.logger.Debug("Execution context prepared successfully")

	if ec.Options.LogWorkingDir {
		ec.logger.Info("Working directory", "dir", ec.WorkingDir)
	}
	if ec.Options.LogEnvironment {
		ec.logger.Debug("Environment variables", "count", len(ec.Environment))
	}
	if ec.Options.LogVariables {
		ec.logger.Debug("Task variables", "count", len(ec.Variables))
	}

	return nil
}

// prepareWorkingDirectory sets up the working directory
func (ec *ExecutionContext) prepareWorkingDirectory(ctx context.Context) error {
	var workingDir string

	// Determine working directory
	if ec.Options.CustomWorkingDir != "" {
		workingDir = ec.Options.CustomWorkingDir
	} else if ec.Options.UseTaskDirectory && ec.Task.WorkingDirectory != "" {
		workingDir = ec.Task.WorkingDirectory
	} else if ec.Task.FilePath != "" {
		workingDir = filepath.Dir(ec.Task.FilePath)
	} else {
		var err error
		workingDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
	}

	// Convert to absolute path
	if !filepath.IsAbs(workingDir) {
		var err error
		workingDir, err = filepath.Abs(workingDir)
		if err != nil {
			return fmt.Errorf("failed to get absolute path: %w", err)
		}
	}

	// Create directory if needed
	if ec.Options.CreateWorkingDir {
		if err := os.MkdirAll(workingDir, 0755); err != nil {
			return fmt.Errorf("failed to create working directory: %w", err)
		}
	}

	// Verify directory exists
	if _, err := os.Stat(workingDir); os.IsNotExist(err) {
		return fmt.Errorf("working directory does not exist: %s", workingDir)
	}

	ec.WorkingDir = workingDir
	return nil
}

// prepareEnvironment sets up environment variables
func (ec *ExecutionContext) prepareEnvironment(ctx context.Context) error {
	env := make(map[string]string)

	// Start with inherited environment if enabled
	if ec.Options.InheritEnvironment && !ec.Options.ClearEnvironment {
		for _, pair := range os.Environ() {
			parts := strings.SplitN(pair, "=", 2)
			if len(parts) == 2 {
				key, value := parts[0], parts[1]

				// Apply whitelist/blacklist
				if ec.isEnvironmentVariableAllowed(key) {
					env[key] = value
				}
			}
		}
	}

	// Add task-specific environment variables
	if ec.Task.Environment != nil {
		for key, value := range ec.Task.Environment {
			if ec.isEnvironmentVariableAllowed(key) {
				env[key] = value
			}
		}
	}

	// Add runner-specific environment variables
	if ec.Runner != nil {
		runnerEnv := ec.Runner.GetEnvironmentVariables(ec.Task)
		for key, value := range runnerEnv {
			if ec.isEnvironmentVariableAllowed(key) {
				env[key] = value
			}
		}
	}

	// Apply overrides
	for key, value := range ec.Options.EnvironmentOverrides {
		if ec.isEnvironmentVariableAllowed(key) {
			env[key] = value
		}
	}

	// Add context-specific variables
	env["WAKE_TASK_ID"] = ec.Task.ID
	env["WAKE_TASK_NAME"] = ec.Task.Name
	env["WAKE_EXECUTION_ID"] = ec.ExecutionID
	env["WAKE_TASK_RUNNER"] = string(ec.Task.Runner)
	env["WAKE_WORKING_DIR"] = ec.WorkingDir
	env["WAKE_START_TIME"] = ec.StartTime.Format(time.RFC3339)

	if ec.Task.FilePath != "" {
		env["WAKE_TASK_FILE"] = ec.Task.FilePath
		env["WAKE_TASK_DIR"] = filepath.Dir(ec.Task.FilePath)
	}

	// Handle PATH modifications
	if err := ec.modifyPath(env); err != nil {
		return fmt.Errorf("failed to modify PATH: %w", err)
	}

	// Expand variables if enabled
	if ec.Options.ExpandVariables {
		env = ec.expandEnvironmentVariables(env)
	}

	ec.Environment = env
	return nil
}

// prepareVariables sets up task variables
func (ec *ExecutionContext) prepareVariables(ctx context.Context) error {
	variables := make(map[string]any)

	// Add task metadata as variables
	variables["task_id"] = ec.Task.ID
	variables["task_name"] = ec.Task.Name
	variables["task_runner"] = string(ec.Task.Runner)
	variables["task_description"] = ec.Task.Description
	variables["execution_id"] = ec.ExecutionID
	variables["start_time"] = ec.StartTime
	variables["working_dir"] = ec.WorkingDir

	if ec.Task.FilePath != "" {
		variables["task_file"] = ec.Task.FilePath
		variables["task_dir"] = filepath.Dir(ec.Task.FilePath)
		variables["task_filename"] = filepath.Base(ec.Task.FilePath)
	}

	// Add task-specific variables from metadata
	if ec.Task.Metadata != nil {
		for key, value := range ec.Task.Metadata {
			// Prefix task metadata to avoid conflicts
			variables["meta_"+key] = value
		}
	}

	// Add runner-specific variables
	if ec.Runner != nil {
		variables["runner_name"] = ec.Runner.Name()
		variables["runner_version"] = ec.Runner.Version()
		variables["runner_timeout"] = ec.Runner.GetDefaultTimeout()
	}

	// Add environment variables as lowercase variables for convenience
	for key, value := range ec.Environment {
		if ec.Options.CaseSensitiveVars {
			variables["env_"+key] = value
		} else {
			variables["env_"+strings.ToLower(key)] = value
		}
	}

	ec.Variables = variables
	return nil
}

// performSecurityChecks performs security validation
func (ec *ExecutionContext) performSecurityChecks(ctx context.Context) error {
	if !ec.Options.RestrictedMode {
		return nil
	}

	// Check command against allowed/forbidden lists
	command := ec.Task.Command
	if command != "" {
		// Check forbidden commands
		for _, forbidden := range ec.Options.ForbiddenCommands {
			if strings.Contains(command, forbidden) {
				return fmt.Errorf("forbidden command detected: %s", forbidden)
			}
		}

		// Check allowed commands if whitelist is specified
		if len(ec.Options.AllowedCommands) > 0 {
			allowed := false
			for _, allowedCmd := range ec.Options.AllowedCommands {
				if strings.HasPrefix(command, allowedCmd) {
					allowed = true
					break
				}
			}
			if !allowed {
				return fmt.Errorf("command not in allowed list: %s", command)
			}
		}
	}

	// Additional security checks could be added here
	// - Path traversal checks
	// - File access restrictions
	// - Network access restrictions

	return nil
}

// isEnvironmentVariableAllowed checks if an environment variable is allowed
func (ec *ExecutionContext) isEnvironmentVariableAllowed(key string) bool {
	// Check blacklist
	for _, blocked := range ec.Options.EnvironmentBlacklist {
		if strings.EqualFold(key, blocked) {
			return false
		}
	}

	// Check whitelist if specified
	if len(ec.Options.EnvironmentWhitelist) > 0 {
		for _, allowed := range ec.Options.EnvironmentWhitelist {
			if strings.EqualFold(key, allowed) {
				return true
			}
		}
		return false // Not in whitelist
	}

	return true // No restrictions or passed blacklist
}

// modifyPath modifies the PATH environment variable
func (ec *ExecutionContext) modifyPath(env map[string]string) error {
	currentPath := env["PATH"]
	if currentPath == "" {
		currentPath = os.Getenv("PATH")
	}

	var pathComponents []string

	// Add prepend paths
	pathComponents = append(pathComponents, ec.Options.PrependPath...)

	// Add current path
	if currentPath != "" {
		pathComponents = append(pathComponents, currentPath)
	}

	// Add append paths
	pathComponents = append(pathComponents, ec.Options.AppendPath...)

	// Join and set
	newPath := strings.Join(pathComponents, ec.Options.PathSeparator)
	env["PATH"] = newPath

	return nil
}

// expandEnvironmentVariables expands variables in environment values
func (ec *ExecutionContext) expandEnvironmentVariables(env map[string]string) map[string]string {
	resolver := &VariableResolver{
		variables:   ec.Variables,
		environment: env,
		options:     ec.Options,
		logger:      ec.logger,
	}

	expanded := make(map[string]string)
	for key, value := range env {
		expanded[key] = resolver.ExpandString(value)
	}

	return expanded
}

// GetEnvironment returns environment variables as a slice
func (ec *ExecutionContext) GetEnvironment() []string {
	ec.mu.RLock()
	defer ec.mu.RUnlock()

	var env []string
	for key, value := range ec.Environment {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}

	return env
}

// GetVariable returns a variable value
func (ec *ExecutionContext) GetVariable(name string) (any, bool) {
	ec.mu.RLock()
	defer ec.mu.RUnlock()

	value, exists := ec.Variables[name]
	return value, exists
}

// SetVariable sets a variable value
func (ec *ExecutionContext) SetVariable(name string, value any) {
	ec.mu.Lock()
	defer ec.mu.Unlock()

	ec.Variables[name] = value
}

// GetWorkingDirectory returns the working directory
func (ec *ExecutionContext) GetWorkingDirectory() string {
	ec.mu.RLock()
	defer ec.mu.RUnlock()

	return ec.WorkingDir
}

// IsPrepaed returns whether the context is prepared
func (ec *ExecutionContext) IsPrepared() bool {
	ec.mu.RLock()
	defer ec.mu.RUnlock()

	return ec.prepared
}

// Clone creates a copy of the execution context
func (ec *ExecutionContext) Clone() *ExecutionContext {
	ec.mu.RLock()
	defer ec.mu.RUnlock()

	clone := &ExecutionContext{
		Task:        ec.Task,
		Runner:      ec.Runner,
		ExecutionID: ec.ExecutionID + "_clone",
		StartTime:   time.Now(),
		WorkingDir:  ec.WorkingDir,
		Environment: make(map[string]string),
		Variables:   make(map[string]any),
		Context:     ec.Context,
		Cancel:      ec.Cancel,
		Options:     ec.Options, // Share options
		prepared:    false,
		logger:      ec.logger,
	}

	// Copy environment
	for k, v := range ec.Environment {
		clone.Environment[k] = v
	}

	// Copy variables
	for k, v := range ec.Variables {
		clone.Variables[k] = v
	}

	return clone
}

// Variable resolver implementation

// NewVariableResolver creates a new variable resolver
func NewVariableResolver(variables map[string]any, environment map[string]string, options *ContextOptions, logger logger.Logger) *VariableResolver {
	return &VariableResolver{
		variables:   variables,
		environment: environment,
		options:     options,
		logger:      logger,
	}
}

// ExpandString expands variables in a string
func (vr *VariableResolver) ExpandString(input string) string {
	if !vr.options.ExpandVariables {
		return input
	}

	// Simple variable expansion - could be enhanced with more sophisticated parsing
	result := input

	// Expand environment variables ($VAR or ${VAR})
	result = os.Expand(result, func(key string) string {
		// Try environment first
		if value, exists := vr.environment[key]; exists {
			return value
		}

		// Try variables
		if value, exists := vr.variables[key]; exists {
			return vr.formatValue(value)
		}

		return ""
	})

	return result
}

// formatValue formats a variable value as a string
func (vr *VariableResolver) formatValue(value any) string {
	switch v := value.(type) {
	case string:
		return v
	case int, int32, int64:
		return fmt.Sprintf("%d", v)
	case float32, float64:
		return fmt.Sprintf("%f", v)
	case bool:
		return strconv.FormatBool(v)
	case time.Time:
		return v.Format(time.RFC3339)
	case time.Duration:
		return v.String()
	default:
		return fmt.Sprintf("%v", v)
	}
}

// ExpandVariables expands variables in a map of strings
func (vr *VariableResolver) ExpandVariables(input map[string]string) map[string]string {
	result := make(map[string]string)

	for key, value := range input {
		result[key] = vr.ExpandString(value)
	}

	return result
}

// ExpandSlice expands variables in a slice of strings
func (vr *VariableResolver) ExpandSlice(input []string) []string {
	result := make([]string, len(input))

	for i, value := range input {
		result[i] = vr.ExpandString(value)
	}

	return result
}

// GetContextMetadata returns metadata about the execution context
func (ec *ExecutionContext) GetContextMetadata() map[string]any {
	ec.mu.RLock()
	defer ec.mu.RUnlock()

	return map[string]any{
		"execution_id":     ec.ExecutionID,
		"task_id":          ec.Task.ID,
		"task_name":        ec.Task.Name,
		"start_time":       ec.StartTime,
		"working_dir":      ec.WorkingDir,
		"prepared":         ec.prepared,
		"environment_vars": len(ec.Environment),
		"variables":        len(ec.Variables),
		"runner_type":      string(ec.Task.Runner),
	}
}
