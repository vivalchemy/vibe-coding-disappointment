package runners

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/vivalchemy/wake/internal/output"
	"github.com/vivalchemy/wake/pkg/logger"
	t "github.com/vivalchemy/wake/pkg/task"
)

// MakeRunner implements the Runner interface for Make tasks
type MakeRunner struct {
	logger   logger.Logger
	options  *MakeOptions
	patterns *MakePatterns
}

// MakeOptions contains configuration for the Make runner
type MakeOptions struct {
	// Make executable
	MakeCommand     string
	MakeArgs        []string
	DefaultMakefile string

	// Execution options
	Parallel  bool
	Jobs      int
	KeepGoing bool
	Silent    bool
	DryRun    bool

	// Environment
	MakeVars        map[string]string
	PassEnvironment bool

	// Output handling
	ShowCommands  bool
	ShowDirectory bool

	// Error handling
	IgnoreErrors bool

	// Performance
	UseJobServer bool
	LoadAverage  float64
}

// MakePatterns contains regex patterns for parsing Make output
type MakePatterns struct {
	ErrorPattern   *regexp.Regexp
	WarningPattern *regexp.Regexp
	EnteringDir    *regexp.Regexp
	LeavingDir     *regexp.Regexp
	CommandPattern *regexp.Regexp
	TargetPattern  *regexp.Regexp
}

// NewMakeRunner creates a new Make runner
func NewMakeRunner(logger logger.Logger) *MakeRunner {
	return &MakeRunner{
		logger:   logger.WithGroup("make-runner"),
		options:  createDefaultMakeOptions(),
		patterns: createMakePatterns(),
	}
}

// createDefaultMakeOptions creates default Make options
func createDefaultMakeOptions() *MakeOptions {
	return &MakeOptions{
		MakeCommand:     "make",
		MakeArgs:        []string{},
		DefaultMakefile: "Makefile",
		Parallel:        false,
		Jobs:            1,
		KeepGoing:       false,
		Silent:          false,
		DryRun:          false,
		MakeVars:        make(map[string]string),
		PassEnvironment: true,
		ShowCommands:    false,
		ShowDirectory:   false,
		IgnoreErrors:    false,
		UseJobServer:    false,
		LoadAverage:     0.0,
	}
}

// createMakePatterns creates regex patterns for Make output parsing
func createMakePatterns() *MakePatterns {
	return &MakePatterns{
		ErrorPattern:   regexp.MustCompile(`^([^:]+):(\d+):\s*(?:\*\*\*\s*)?(.*)$`),
		WarningPattern: regexp.MustCompile(`^([^:]+):(\d+):\s*warning:\s*(.*)$`),
		EnteringDir:    regexp.MustCompile(`^make(?:\[\d+\])?: Entering directory ['"]([^'"]+)['"]$`),
		LeavingDir:     regexp.MustCompile(`^make(?:\[\d+\])?: Leaving directory ['"]([^'"]+)['"]$`),
		CommandPattern: regexp.MustCompile(`^\s*(.+?)\s*$`),
		TargetPattern:  regexp.MustCompile(`^make(?:\[\d+\])?: .* target ['"]([^'"]+)['"]$`),
	}
}

// Type returns the runner type
func (mr *MakeRunner) Type() t.RunnerType {
	return t.RunnerMake
}

// Name returns the runner name
func (mr *MakeRunner) Name() string {
	return "Make Runner"
}

// Version returns the runner version
func (mr *MakeRunner) Version() string {
	return "1.0.0"
}

// CanRun checks if the runner can execute the given task
func (mr *MakeRunner) CanRun(task *t.Task) bool {
	if task.Runner != t.RunnerMake {
		return false
	}

	// Check if Make is available
	if !mr.isMakeAvailable() {
		mr.logger.Warn("Make is not available")
		return false
	}

	// Check if Makefile exists
	makefilePath := mr.findMakefile(task)
	if makefilePath == "" {
		mr.logger.Warn("No Makefile found", "task", task.Name)
		return false
	}

	return true
}

// Prepare prepares the task for execution
func (mr *MakeRunner) Prepare(ctx context.Context, task *t.Task) error {
	mr.logger.Debug("Preparing Make task", "task", task.Name)

	// Validate Make availability
	if !mr.isMakeAvailable() {
		return fmt.Errorf("make command not found")
	}

	// Find and validate Makefile
	makefilePath := mr.findMakefile(task)
	if makefilePath == "" {
		return fmt.Errorf("no Makefile found for task %s", task.Name)
	}

	// Validate target exists in Makefile
	if err := mr.validateTarget(makefilePath, task.Name); err != nil {
		return fmt.Errorf("target validation failed: %w", err)
	}

	return nil
}

// Execute executes the Make task
func (mr *MakeRunner) Execute(ctx context.Context, task *t.Task, outputBuffer *output.Buffer) error {
	mr.logger.Info("Executing Make task", "task", task.Name)

	// Build command
	cmd, err := mr.buildCommand(task)
	if err != nil {
		return fmt.Errorf("failed to build command: %w", err)
	}

	// Set up command context
	cmd.Dir = mr.getWorkingDirectory(task)
	cmd.Env = mr.buildEnvironment(task)

	mr.logger.Debug("Make command built",
		"cmd", cmd.Path,
		"args", cmd.Args,
		"dir", cmd.Dir)

	// Execute with output streaming
	return mr.executeWithOutput(ctx, cmd, outputBuffer)
}

// Stop stops the running Make task
func (mr *MakeRunner) Stop(ctx context.Context, task *t.Task) error {
	mr.logger.Info("Stopping Make task", "task", task.Name)

	// Make doesn't have a built-in graceful stop mechanism
	// We rely on the process being killed by the executor
	return nil
}

// GetDefaultTimeout returns the default timeout for Make tasks
func (mr *MakeRunner) GetDefaultTimeout() time.Duration {
	return 10 * time.Minute
}

// GetEnvironmentVariables returns environment variables for the task
func (mr *MakeRunner) GetEnvironmentVariables(task *t.Task) map[string]string {
	env := make(map[string]string)

	// Add Make-specific variables
	env["MAKE"] = mr.options.MakeCommand

	if task.FilePath != "" {
		env["MAKEFILE_LIST"] = task.FilePath
		env["MAKEFILE_DIR"] = filepath.Dir(task.FilePath)
	}

	// Add custom Make variables
	for key, value := range mr.options.MakeVars {
		env[key] = value
	}

	// Add task-specific variables from metadata
	if makeVars, ok := task.Metadata["make_variables"].(map[string]string); ok {
		for key, value := range makeVars {
			env[key] = value
		}
	}

	return env
}

// Validate validates the task configuration
func (mr *MakeRunner) Validate(task *t.Task) error {
	if task.Runner != t.RunnerMake {
		return fmt.Errorf("invalid runner type: expected %s, got %s", t.RunnerMake, task.Runner)
	}

	if task.Name == "" {
		return fmt.Errorf("task name cannot be empty")
	}

	// Additional validation
	if task.Command != "" && task.Command != "make" {
		mr.logger.Warn("Task has custom command, but Make runner expects 'make'", "command", task.Command)
	}

	return nil
}

// SetOptions sets runner options
func (mr *MakeRunner) SetOptions(options *MakeOptions) {
	mr.options = options
}

// GetOptions returns current runner options
func (mr *MakeRunner) GetOptions() *MakeOptions {
	return mr.options
}

// Private methods

// isMakeAvailable checks if Make is available on the system
func (mr *MakeRunner) isMakeAvailable() bool {
	_, err := exec.LookPath(mr.options.MakeCommand)
	return err == nil
}

// findMakefile finds the appropriate Makefile for the task
func (mr *MakeRunner) findMakefile(task *t.Task) string {
	// If task has a specific file path, use it
	if task.FilePath != "" {
		if _, err := os.Stat(task.FilePath); err == nil {
			return task.FilePath
		}
	}

	// Look for Makefile in task directory
	taskDir := task.WorkingDirectory
	if taskDir == "" && task.FilePath != "" {
		taskDir = filepath.Dir(task.FilePath)
	}
	if taskDir == "" {
		taskDir = "."
	}

	// Common Makefile names
	makefileNames := []string{
		"Makefile",
		"makefile",
		"GNUmakefile",
		mr.options.DefaultMakefile,
	}

	for _, name := range makefileNames {
		makefilePath := filepath.Join(taskDir, name)
		if _, err := os.Stat(makefilePath); err == nil {
			return makefilePath
		}
	}

	return ""
}

// validateTarget checks if the target exists in the Makefile
func (mr *MakeRunner) validateTarget(makefilePath, target string) error {
	file, err := os.Open(makefilePath)
	if err != nil {
		return fmt.Errorf("failed to open Makefile: %w", err)
	}
	defer file.Close()

	// Simple target validation - could be enhanced
	scanner := bufio.NewScanner(file)
	targetPattern := regexp.MustCompile(`^` + regexp.QuoteMeta(target) + `\s*:`)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if targetPattern.MatchString(line) {
			return nil // Target found
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading Makefile: %w", err)
	}

	// Target not found - this might be okay for phony targets
	mr.logger.Warn("Target not found in Makefile", "target", target, "file", makefilePath)
	return nil
}

// buildCommand builds the Make command
func (mr *MakeRunner) buildCommand(task *t.Task) (*exec.Cmd, error) {
	args := []string{}

	// Add default Make arguments
	args = append(args, mr.options.MakeArgs...)

	// Add Makefile specification if needed
	makefilePath := mr.findMakefile(task)
	if makefilePath != "" && filepath.Base(makefilePath) != "Makefile" {
		args = append(args, "-f", makefilePath)
	}

	// Add job control options
	if mr.options.Parallel && mr.options.Jobs > 1 {
		args = append(args, "-j", strconv.Itoa(mr.options.Jobs))
	}

	// Add other options
	if mr.options.KeepGoing {
		args = append(args, "-k")
	}

	if mr.options.Silent {
		args = append(args, "-s")
	}

	if mr.options.DryRun {
		args = append(args, "-n")
	}

	if mr.options.ShowCommands {
		args = append(args, "--print-data-base", "--print-directory")
	}

	if mr.options.ShowDirectory {
		args = append(args, "--print-directory")
	}

	if mr.options.IgnoreErrors {
		args = append(args, "-i")
	}

	if mr.options.LoadAverage > 0 {
		args = append(args, "-l", fmt.Sprintf("%.2f", mr.options.LoadAverage))
	}

	// Add Make variables
	for key, value := range mr.options.MakeVars {
		args = append(args, fmt.Sprintf("%s=%s", key, value))
	}

	// Add task-specific variables
	if task.Environment != nil {
		for key, value := range task.Environment {
			args = append(args, fmt.Sprintf("%s=%s", key, value))
		}
	}

	// Add target name
	args = append(args, task.Name)

	// Add task arguments if any
	if len(task.Args) > 1 { // Skip first arg which is usually the task name
		args = append(args, task.Args[1:]...)
	}

	cmd := exec.Command(mr.options.MakeCommand, args...)
	return cmd, nil
}

// getWorkingDirectory returns the working directory for the task
func (mr *MakeRunner) getWorkingDirectory(task *t.Task) string {
	if task.WorkingDirectory != "" {
		return task.WorkingDirectory
	}

	if task.FilePath != "" {
		return filepath.Dir(task.FilePath)
	}

	return "."
}

// buildEnvironment builds the environment variables for the command
func (mr *MakeRunner) buildEnvironment(task *t.Task) []string {
	var env []string

	// Start with current environment if enabled
	if mr.options.PassEnvironment {
		env = os.Environ()
	}

	// Add runner-specific environment variables
	runnerEnv := mr.GetEnvironmentVariables(task)
	for key, value := range runnerEnv {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}

	return env
}

// executeWithOutput executes the command with output streaming
func (mr *MakeRunner) executeWithOutput(ctx context.Context, cmd *exec.Cmd, outputBuffer *output.Buffer) error {
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
	go mr.streamOutput(stdout, outputBuffer, false)
	go mr.streamOutput(stderr, outputBuffer, true)

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
			return fmt.Errorf("make command failed: %w", err)
		}
		return nil
	}
}

// streamOutput streams command output to the buffer
func (mr *MakeRunner) streamOutput(pipe any, outputBuffer *output.Buffer, isStderr bool) {
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
		enhancedLine := mr.parseOutputLine(line, isStderr)

		// Write to buffer
		if isStderr {
			outputBuffer.WriteStderr([]byte(enhancedLine + "\n"))
		} else {
			outputBuffer.WriteStdout([]byte(enhancedLine + "\n"))
		}
	}
}

// parseOutputLine parses and enhances a Make output line
func (mr *MakeRunner) parseOutputLine(line string, isStderr bool) string {
	// Add timestamps and formatting
	timestamp := time.Now().Format("15:04:05.000")
	prefix := "OUT"
	if isStderr {
		prefix = "ERR"
	}

	// Check for special Make patterns
	if mr.patterns.ErrorPattern.MatchString(line) {
		prefix = "ERROR"
	} else if mr.patterns.WarningPattern.MatchString(line) {
		prefix = "WARN"
	} else if mr.patterns.EnteringDir.MatchString(line) {
		prefix = "DIR"
	} else if mr.patterns.LeavingDir.MatchString(line) {
		prefix = "DIR"
	}

	return fmt.Sprintf("[%s] [%s] %s", timestamp, prefix, line)
}

// GetMakeVersion returns the version of Make
func (mr *MakeRunner) GetMakeVersion() (string, error) {
	cmd := exec.Command(mr.options.MakeCommand, "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get Make version: %w", err)
	}

	// Parse version from output
	lines := strings.Split(string(output), "\n")
	if len(lines) > 0 {
		versionLine := lines[0]
		// Extract version number (this is a simplified extraction)
		parts := strings.Fields(versionLine)
		for i, part := range parts {
			if strings.Contains(part, ".") && i+1 < len(parts) {
				return parts[i+1], nil
			}
		}
	}

	return "unknown", nil
}

// ListTargets lists all available targets in the Makefile
func (mr *MakeRunner) ListTargets(makefilePath string) ([]string, error) {
	cmd := exec.Command(mr.options.MakeCommand, "-f", makefilePath, "-qp")
	cmd.Stderr = nil // Suppress stderr

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list targets: %w", err)
	}

	var targets []string
	targetPattern := regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_.-]*)\s*:`)

	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		matches := targetPattern.FindStringSubmatch(line)
		if len(matches) > 1 {
			target := matches[1]

			// Skip special targets
			if !strings.HasPrefix(target, ".") && target != "Makefile" {
				targets = append(targets, target)
			}
		}
	}

	return targets, nil
}

// GetTaskMetadata returns metadata specific to Make tasks
func (mr *MakeRunner) GetTaskMetadata(task *t.Task) map[string]any {
	metadata := map[string]any{
		"runner_type":    mr.Type(),
		"runner_name":    mr.Name(),
		"runner_version": mr.Version(),
		"make_command":   mr.options.MakeCommand,
	}

	if makefilePath := mr.findMakefile(task); makefilePath != "" {
		metadata["makefile_path"] = makefilePath

		// Add target list if available
		if targets, err := mr.ListTargets(makefilePath); err == nil {
			metadata["available_targets"] = targets
		}
	}

	// Add Make version
	if version, err := mr.GetMakeVersion(); err == nil {
		metadata["make_version"] = version
	}

	return metadata
}
