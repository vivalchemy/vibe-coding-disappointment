package runners

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/vivalchemy/wake/internal/output"
	"github.com/vivalchemy/wake/pkg/logger"
	t "github.com/vivalchemy/wake/pkg/task"
)

// TaskRunner implements the Runner interface for Task (Taskfile.yml) tasks
type TaskRunner struct {
	logger   logger.Logger
	options  *TaskOptions
	patterns *TaskPatterns
}

// TaskOptions contains configuration for the Task runner
type TaskOptions struct {
	// Task executable
	TaskCommand     string
	TaskArgs        []string
	DefaultTaskfile string

	// Execution options
	DryRun   bool
	Verbose  bool
	Silent   bool
	Parallel bool
	Continue bool
	Force    bool
	Watch    bool

	// Working directory options
	Directory string

	// Variable options
	Variables   map[string]string
	Environment map[string]string

	// Taskfile discovery
	TaskfileNames []string
	SearchParents bool

	// Output options
	Color   bool
	Summary bool
	List    bool
	ListAll bool

	// Performance options
	Concurrency int
	Interval    string

	// Environment
	PassEnvironment bool
}

// TaskPatterns contains regex patterns for parsing Task output
type TaskPatterns struct {
	ErrorPattern    *regexp.Regexp
	WarningPattern  *regexp.Regexp
	TaskPattern     *regexp.Regexp
	RunningPattern  *regexp.Regexp
	FinishedPattern *regexp.Regexp
	SkippedPattern  *regexp.Regexp
	CommandPattern  *regexp.Regexp
	ContextPattern  *regexp.Regexp
}

// TaskfileRoot represents the root structure of a Taskfile
type TaskfileRoot struct {
	Version  string                   `yaml:"version"`
	Output   string                   `yaml:"output,omitempty"`
	Method   string                   `yaml:"method,omitempty"`
	Includes map[string]any           `yaml:"includes,omitempty"`
	Vars     map[string]any           `yaml:"vars,omitempty"`
	Env      map[string]any           `yaml:"env,omitempty"`
	Tasks    map[string]*TaskfileTask `yaml:"tasks"`
	Silent   bool                     `yaml:"silent,omitempty"`
	Dotenv   []string                 `yaml:"dotenv,omitempty"`
	Run      string                   `yaml:"run,omitempty"`
	Interval string                   `yaml:"interval,omitempty"`
}

// TaskfileTask represents a single task in a Taskfile
type TaskfileTask struct {
	// Basic properties
	Desc    string   `yaml:"desc,omitempty"`
	Summary string   `yaml:"summary,omitempty"`
	Aliases []string `yaml:"aliases,omitempty"`

	// Execution
	Cmds          []any                  `yaml:"cmds,omitempty"`
	Deps          []any                  `yaml:"deps,omitempty"`
	Preconditions []TaskfilePrecondition `yaml:"preconditions,omitempty"`

	// Configuration
	Silent      *bool  `yaml:"silent,omitempty"`
	Method      string `yaml:"method,omitempty"`
	Prefix      string `yaml:"prefix,omitempty"`
	IgnoreError bool   `yaml:"ignore_error,omitempty"`
	Run         string `yaml:"run,omitempty"`

	// Environment and variables
	Vars map[string]any `yaml:"vars,omitempty"`
	Env  map[string]any `yaml:"env,omitempty"`

	// File operations
	Sources   []string `yaml:"sources,omitempty"`
	Generates []string `yaml:"generates,omitempty"`
	Status    []string `yaml:"status,omitempty"`

	// Control
	Dir   string   `yaml:"dir,omitempty"`
	Set   []string `yaml:"set,omitempty"`
	Shopt []string `yaml:"shopt,omitempty"`

	// Internal tracking
	Internal bool `yaml:"internal,omitempty"`
}

// TaskfilePrecondition represents a task precondition
type TaskfilePrecondition struct {
	Sh  string `yaml:"sh,omitempty"`
	Msg string `yaml:"msg,omitempty"`
}

// TaskfileCommand represents different command formats
type TaskfileCommand struct {
	Cmd         string
	Task        string
	Defer       string
	Silent      bool
	IgnoreError bool
	Vars        map[string]any
}

// NewTaskRunner creates a new Task runner
func NewTaskRunner(logger logger.Logger) *TaskRunner {
	return &TaskRunner{
		logger:   logger.WithGroup("task-runner"),
		options:  createDefaultTaskOptions(),
		patterns: createTaskPatterns(),
	}
}

// createDefaultTaskOptions creates default Task options
func createDefaultTaskOptions() *TaskOptions {
	return &TaskOptions{
		TaskCommand:     "task",
		TaskArgs:        []string{},
		DefaultTaskfile: "Taskfile.yml",
		DryRun:          false,
		Verbose:         false,
		Silent:          false,
		Parallel:        false,
		Continue:        false,
		Force:           false,
		Watch:           false,
		Directory:       "",
		Variables:       make(map[string]string),
		Environment:     make(map[string]string),
		TaskfileNames:   []string{"Taskfile.yml", "Taskfile.yaml", "taskfile.yml", "taskfile.yaml"},
		SearchParents:   true,
		Color:           true,
		Summary:         false,
		List:            false,
		ListAll:         false,
		Concurrency:     0,
		Interval:        "",
		PassEnvironment: true,
	}
}

// createTaskPatterns creates regex patterns for Task output parsing
func createTaskPatterns() *TaskPatterns {
	return &TaskPatterns{
		ErrorPattern:    regexp.MustCompile(`^task:\s+(.*)$`),
		WarningPattern:  regexp.MustCompile(`^task:\s+\[WARN\]\s+(.*)$`),
		TaskPattern:     regexp.MustCompile(`^task:\s+\[(.+)\]\s+(.*)$`),
		RunningPattern:  regexp.MustCompile(`^task:\s+Started\s+(.*)$`),
		FinishedPattern: regexp.MustCompile(`^task:\s+Finished\s+(.*)$`),
		SkippedPattern:  regexp.MustCompile(`^task:\s+Task\s+"(.*)"\s+is\s+up\s+to\s+date$`),
		CommandPattern:  regexp.MustCompile(`^\s*\$\s+(.*)$`),
		ContextPattern:  regexp.MustCompile(`^task:\s+Context:\s+(.*)$`),
	}
}

// Type returns the runner type
func (tr *TaskRunner) Type() t.RunnerType {
	return t.RunnerTask
}

// Name returns the runner name
func (tr *TaskRunner) Name() string {
	return "Task Runner"
}

// Version returns the runner version
func (tr *TaskRunner) Version() string {
	return "1.0.0"
}

// CanRun checks if the runner can execute the given task
func (tr *TaskRunner) CanRun(task *t.Task) bool {
	if task.Runner != t.RunnerTask {
		return false
	}

	// Check if Task is available
	if !tr.isTaskAvailable() {
		tr.logger.Warn("Task is not available")
		return false
	}

	// Check if Taskfile exists
	taskfilePath := tr.findTaskfile(task)
	if taskfilePath == "" {
		tr.logger.Warn("No Taskfile found", "task", task.Name)
		return false
	}

	return true
}

// Prepare prepares the task for execution
func (tr *TaskRunner) Prepare(ctx context.Context, task *t.Task) error {
	tr.logger.Debug("Preparing Task task", "task", task.Name)

	// Validate Task availability
	if !tr.isTaskAvailable() {
		return fmt.Errorf("task command not found")
	}

	// Find and validate Taskfile
	taskfilePath := tr.findTaskfile(task)
	if taskfilePath == "" {
		return fmt.Errorf("no Taskfile found for task %s", task.Name)
	}

	// Parse Taskfile
	taskfile, err := tr.parseTaskfile(taskfilePath)
	if err != nil {
		return fmt.Errorf("failed to parse Taskfile: %w", err)
	}

	// Validate task exists in Taskfile
	if err := tr.validateTaskInFile(taskfile, task.Name); err != nil {
		return fmt.Errorf("task validation failed: %w", err)
	}

	return nil
}

// Execute executes the Task task
func (tr *TaskRunner) Execute(ctx context.Context, task *t.Task, outputBuffer *output.Buffer) error {
	tr.logger.Info("Executing Task task", "task", task.Name)

	// Build command
	cmd, err := tr.buildCommand(task)
	if err != nil {
		return fmt.Errorf("failed to build command: %w", err)
	}

	// Set up command context
	cmd.Dir = tr.getWorkingDirectory(task)
	cmd.Env = tr.buildEnvironment(task)

	tr.logger.Debug("Task command built",
		"cmd", cmd.Path,
		"args", cmd.Args,
		"dir", cmd.Dir)

	// Execute with output streaming
	return tr.executeWithOutput(ctx, cmd, outputBuffer)
}

// Stop stops the running Task task
func (tr *TaskRunner) Stop(ctx context.Context, task *t.Task) error {
	tr.logger.Info("Stopping Task task", "task", task.Name)

	// Task doesn't have a built-in graceful stop mechanism
	// We rely on the process being killed by the executor
	return nil
}

// GetDefaultTimeout returns the default timeout for Task tasks
func (tr *TaskRunner) GetDefaultTimeout() time.Duration {
	return 15 * time.Minute
}

// GetEnvironmentVariables returns environment variables for the task
func (tr *TaskRunner) GetEnvironmentVariables(task *t.Task) map[string]string {
	env := make(map[string]string)

	// Add Task-specific variables
	env["TASK"] = tr.options.TaskCommand

	if task.FilePath != "" {
		env["TASKFILE"] = task.FilePath
		env["TASKFILE_DIR"] = filepath.Dir(task.FilePath)
	}

	// Add custom variables
	for key, value := range tr.options.Variables {
		env[key] = value
	}

	// Add custom environment variables
	for key, value := range tr.options.Environment {
		env[key] = value
	}

	// Add task-specific variables from metadata
	if taskVars, ok := task.Metadata["taskfile_vars"].(map[string]any); ok {
		for key, value := range taskVars {
			if strValue, ok := value.(string); ok {
				env[key] = strValue
			}
		}
	}

	// Add task-specific environment from metadata
	if taskEnv, ok := task.Metadata["taskfile_env"].(map[string]any); ok {
		for key, value := range taskEnv {
			if strValue, ok := value.(string); ok {
				env[key] = strValue
			}
		}
	}

	return env
}

// Validate validates the task configuration
func (tr *TaskRunner) Validate(task *t.Task) error {
	if task.Runner != t.RunnerTask {
		return fmt.Errorf("invalid runner type: expected %s, got %s", t.RunnerTask, task.Runner)
	}

	if task.Name == "" {
		return fmt.Errorf("task name cannot be empty")
	}

	// Additional validation
	if task.Command != "" && task.Command != "task" {
		tr.logger.Warn("Task has custom command, but Task runner expects 'task'", "command", task.Command)
	}

	return nil
}

// SetOptions sets runner options
func (tr *TaskRunner) SetOptions(options *TaskOptions) {
	tr.options = options
}

// GetOptions returns current runner options
func (tr *TaskRunner) GetOptions() *TaskOptions {
	return tr.options
}

// Private methods

// isTaskAvailable checks if Task is available on the system
func (tr *TaskRunner) isTaskAvailable() bool {
	_, err := exec.LookPath(tr.options.TaskCommand)
	return err == nil
}

// findTaskfile finds the appropriate Taskfile for the task
func (tr *TaskRunner) findTaskfile(task *t.Task) string {
	// If task has a specific file path, use it
	if task.FilePath != "" {
		// Check if it's a Taskfile
		fileName := filepath.Base(task.FilePath)
		for _, name := range tr.options.TaskfileNames {
			if fileName == name {
				if _, err := os.Stat(task.FilePath); err == nil {
					return task.FilePath
				}
			}
		}
	}

	// Look for Taskfile in task directory
	taskDir := task.WorkingDirectory
	if taskDir == "" && task.FilePath != "" {
		taskDir = filepath.Dir(task.FilePath)
	}
	if taskDir == "" {
		taskDir = "."
	}

	// Search in current directory and optionally parent directories
	currentDir := taskDir
	for {
		for _, name := range tr.options.TaskfileNames {
			taskfilePath := filepath.Join(currentDir, name)
			if _, err := os.Stat(taskfilePath); err == nil {
				return taskfilePath
			}
		}

		if !tr.options.SearchParents {
			break
		}

		parent := filepath.Dir(currentDir)
		if parent == currentDir || parent == "/" {
			break // Reached root
		}
		currentDir = parent
	}

	return ""
}

// parseTaskfile parses a Taskfile and returns the structure
func (tr *TaskRunner) parseTaskfile(taskfilePath string) (*TaskfileRoot, error) {
	file, err := os.Open(taskfilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open Taskfile: %w", err)
	}
	defer file.Close()

	var taskfile TaskfileRoot
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&taskfile); err != nil {
		return nil, fmt.Errorf("failed to decode Taskfile: %w", err)
	}

	return &taskfile, nil
}

// validateTaskInFile checks if the task exists in the Taskfile
func (tr *TaskRunner) validateTaskInFile(taskfile *TaskfileRoot, taskName string) error {
	if taskfile.Tasks == nil {
		return fmt.Errorf("no tasks defined in Taskfile")
	}

	// Check direct task name
	if _, exists := taskfile.Tasks[taskName]; exists {
		return nil
	}

	// Check aliases
	for _, taskDef := range taskfile.Tasks {
		for _, alias := range taskDef.Aliases {
			if alias == taskName {
				return nil
			}
		}
	}

	return fmt.Errorf("task '%s' not found in Taskfile", taskName)
}

// buildCommand builds the Task command
func (tr *TaskRunner) buildCommand(task *t.Task) (*exec.Cmd, error) {
	args := []string{}

	// Add default Task arguments
	args = append(args, tr.options.TaskArgs...)

	// Add Taskfile specification if needed
	taskfilePath := tr.findTaskfile(task)
	if taskfilePath != "" {
		fileName := filepath.Base(taskfilePath)
		if fileName != "Taskfile.yml" {
			args = append(args, "--taskfile", taskfilePath)
		}
	}

	// Add directory if specified
	if tr.options.Directory != "" {
		args = append(args, "--dir", tr.options.Directory)
	}

	// Add execution options
	if tr.options.DryRun {
		args = append(args, "--dry")
	}

	if tr.options.Verbose {
		args = append(args, "--verbose")
	}

	if tr.options.Silent {
		args = append(args, "--silent")
	}

	if tr.options.Parallel {
		args = append(args, "--parallel")
	}

	if tr.options.Continue {
		args = append(args, "--continue")
	}

	if tr.options.Force {
		args = append(args, "--force")
	}

	if tr.options.Watch {
		args = append(args, "--watch")
	}

	// Add color option
	if !tr.options.Color {
		args = append(args, "--color=never")
	}

	// Add concurrency if specified
	if tr.options.Concurrency > 0 {
		args = append(args, "--concurrency", fmt.Sprintf("%d", tr.options.Concurrency))
	}

	// Add interval if specified
	if tr.options.Interval != "" {
		args = append(args, "--interval", tr.options.Interval)
	}

	// Add variables
	for key, value := range tr.options.Variables {
		args = append(args, "--set", fmt.Sprintf("%s=%s", key, value))
	}

	// Add special commands
	if tr.options.Summary {
		args = append(args, "--summary")
	} else if tr.options.List {
		args = append(args, "--list")
	} else if tr.options.ListAll {
		args = append(args, "--list-all")
	} else {
		// Add task name
		args = append(args, task.Name)

		// Add task arguments if any
		if len(task.Args) > 1 { // Skip first arg which is usually the task name
			args = append(args, "--")
			args = append(args, task.Args[1:]...)
		}
	}

	cmd := exec.Command(tr.options.TaskCommand, args...)
	return cmd, nil
}

// getWorkingDirectory returns the working directory for the task
func (tr *TaskRunner) getWorkingDirectory(task *t.Task) string {
	if tr.options.Directory != "" {
		return tr.options.Directory
	}

	if task.WorkingDirectory != "" {
		return task.WorkingDirectory
	}

	if taskfilePath := tr.findTaskfile(task); taskfilePath != "" {
		return filepath.Dir(taskfilePath)
	}

	return "."
}

// buildEnvironment builds the environment variables for the command
func (tr *TaskRunner) buildEnvironment(task *t.Task) []string {
	var env []string

	// Start with current environment if enabled
	if tr.options.PassEnvironment {
		env = os.Environ()
	}

	// Add runner-specific environment variables
	runnerEnv := tr.GetEnvironmentVariables(task)
	for key, value := range runnerEnv {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}

	return env
}

// executeWithOutput executes the command with output streaming
func (tr *TaskRunner) executeWithOutput(ctx context.Context, cmd *exec.Cmd, outputBuffer *output.Buffer) error {
	// Create pipes for stdout and stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start command: %w", err)
	}

	// Stream output
	go tr.streamOutput(stdout, outputBuffer, false)
	go tr.streamOutput(stderr, outputBuffer, true)

	// Wait for command to complete or context to be cancelled
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case <-ctx.Done():
		// Context cancelled, kill the process
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
		return ctx.Err()

	case err := <-done:
		if err != nil {
			return fmt.Errorf("task command failed: %w", err)
		}
		return nil
	}
}

// streamOutput streams command output to the buffer
func (tr *TaskRunner) streamOutput(pipe any, outputBuffer *output.Buffer, isStderr bool) {
	var scanner *bufio.Scanner

	switch p := pipe.(type) {
	case *os.File:
		scanner = bufio.NewScanner(p)
	default:
		scanner = bufio.NewScanner(pipe.(interface{ Read([]byte) (int, error) }))
	}

	for scanner.Scan() {
		line := scanner.Text()

		// Parse and enhance output
		enhancedLine := tr.parseOutputLine(line, isStderr)

		// Write to buffer
		if isStderr {
			outputBuffer.WriteStderr([]byte(enhancedLine + "\n"))
		} else {
			outputBuffer.WriteStdout([]byte(enhancedLine + "\n"))
		}
	}
}

// parseOutputLine parses and enhances a Task output line
func (tr *TaskRunner) parseOutputLine(line string, isStderr bool) string {
	// Add timestamps and formatting
	timestamp := time.Now().Format("15:04:05.000")
	prefix := "OUT"
	if isStderr {
		prefix = "ERR"
	}

	// Check for special Task patterns
	if tr.patterns.ErrorPattern.MatchString(line) {
		prefix = "ERROR"
	} else if tr.patterns.WarningPattern.MatchString(line) {
		prefix = "WARN"
	} else if tr.patterns.TaskPattern.MatchString(line) {
		prefix = "TASK"
	} else if tr.patterns.RunningPattern.MatchString(line) {
		prefix = "RUNNING"
	} else if tr.patterns.FinishedPattern.MatchString(line) {
		prefix = "FINISHED"
	} else if tr.patterns.SkippedPattern.MatchString(line) {
		prefix = "SKIPPED"
	} else if tr.patterns.CommandPattern.MatchString(line) {
		prefix = "CMD"
	} else if tr.patterns.ContextPattern.MatchString(line) {
		prefix = "CONTEXT"
	}

	return fmt.Sprintf("[%s] [%s] %s", timestamp, prefix, line)
}

// GetTaskVersion returns the version of Task
func (tr *TaskRunner) GetTaskVersion() (string, error) {
	cmd := exec.Command(tr.options.TaskCommand, "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get Task version: %w", err)
	}

	// Parse version from output (format: "Task version: x.y.z")
	versionStr := strings.TrimSpace(string(output))
	if strings.HasPrefix(versionStr, "Task version: ") {
		return strings.TrimPrefix(versionStr, "Task version: "), nil
	}

	return versionStr, nil
}

// ListTasks lists all available tasks in the Taskfile
func (tr *TaskRunner) ListTasks(taskfilePath string) (map[string]*TaskfileTask, error) {
	taskfile, err := tr.parseTaskfile(taskfilePath)
	if err != nil {
		return nil, err
	}

	return taskfile.Tasks, nil
}

// GetTaskInfo returns detailed information about a specific task
func (tr *TaskRunner) GetTaskInfo(taskfilePath, taskName string) (*TaskfileTask, error) {
	taskfile, err := tr.parseTaskfile(taskfilePath)
	if err != nil {
		return nil, err
	}

	// Check direct task name
	if taskDef, exists := taskfile.Tasks[taskName]; exists {
		return taskDef, nil
	}

	// Check aliases
	for _, taskDef := range taskfile.Tasks {
		for _, alias := range taskDef.Aliases {
			if alias == taskName {
				return taskDef, nil
			}
		}
	}

	return nil, fmt.Errorf("task '%s' not found", taskName)
}

// GetTaskDependencies returns the dependency graph for tasks
func (tr *TaskRunner) GetTaskDependencies(taskfilePath string) (map[string][]string, error) {
	taskfile, err := tr.parseTaskfile(taskfilePath)
	if err != nil {
		return nil, err
	}

	dependencies := make(map[string][]string)

	for taskName, taskDef := range taskfile.Tasks {
		var deps []string

		for _, dep := range taskDef.Deps {
			switch depValue := dep.(type) {
			case string:
				deps = append(deps, depValue)
			case map[string]any:
				// Handle dependency with parameters
				if task, ok := depValue["task"].(string); ok {
					deps = append(deps, task)
				}
			}
		}

		dependencies[taskName] = deps
	}

	return dependencies, nil
}

// GetTaskMetadata returns metadata specific to Task tasks
func (tr *TaskRunner) GetTaskMetadata(task *t.Task) map[string]any {
	metadata := map[string]any{
		"runner_type":    tr.Type(),
		"runner_name":    tr.Name(),
		"runner_version": tr.Version(),
		"task_command":   tr.options.TaskCommand,
	}

	if taskfilePath := tr.findTaskfile(task); taskfilePath != "" {
		metadata["taskfile_path"] = taskfilePath

		// Add Taskfile version and info
		if taskfile, err := tr.parseTaskfile(taskfilePath); err == nil {
			metadata["taskfile_version"] = taskfile.Version
			metadata["taskfile_method"] = taskfile.Method
			metadata["taskfile_output"] = taskfile.Output
			metadata["taskfile_silent"] = taskfile.Silent

			// Add task information
			if taskInfo, err := tr.GetTaskInfo(taskfilePath, task.Name); err == nil {
				metadata["task_description"] = taskInfo.Desc
				metadata["task_summary"] = taskInfo.Summary
				metadata["task_aliases"] = taskInfo.Aliases
				metadata["task_silent"] = taskInfo.Silent
				metadata["task_method"] = taskInfo.Method
				metadata["task_ignore_error"] = taskInfo.IgnoreError
				metadata["task_sources"] = taskInfo.Sources
				metadata["task_generates"] = taskInfo.Generates
				metadata["task_status"] = taskInfo.Status
				metadata["task_internal"] = taskInfo.Internal
			}

			// Add available tasks
			if tasks, err := tr.ListTasks(taskfilePath); err == nil {
				var taskNames []string
				for taskName, taskDef := range tasks {
					if !taskDef.Internal { // Only include non-internal tasks
						taskNames = append(taskNames, taskName)
					}
				}
				metadata["available_tasks"] = taskNames
			}

			// Add dependency graph
			if deps, err := tr.GetTaskDependencies(taskfilePath); err == nil {
				metadata["task_dependencies"] = deps
			}
		}
	}

	// Add Task version
	if version, err := tr.GetTaskVersion(); err == nil {
		metadata["task_version"] = version
	}

	// Add configuration
	metadata["variables"] = tr.options.Variables
	metadata["environment"] = tr.options.Environment
	metadata["concurrency"] = tr.options.Concurrency
	metadata["interval"] = tr.options.Interval

	return metadata
}
