package runners

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/vivalchemy/wake/internal/output"
	"github.com/vivalchemy/wake/pkg/logger"
	t "github.com/vivalchemy/wake/pkg/task"
)

// NpmRunner implements the Runner interface for NPM tasks
type NpmRunner struct {
	logger   logger.Logger
	options  *NpmOptions
	patterns *NpmPatterns
}

// NpmOptions contains configuration for the NPM runner
type NpmOptions struct {
	// NPM executable
	NpmCommand string
	NpmArgs    []string

	// Package.json handling
	PackageJsonFile string
	WorkspaceRoot   string

	// Execution options
	Silent        bool
	Progress      bool
	Production    bool
	Development   bool
	GlobalInstall bool

	// Registry and authentication
	Registry  string
	AuthToken string
	Scope     string

	// Cache and performance
	UseCache      bool
	CacheDir      string
	OfflineMode   bool
	PreferOffline bool

	// Output handling
	LogLevel   string
	JsonOutput bool
	LongOutput bool

	// Security
	AuditLevel    string
	IgnoreScripts bool

	// Environment
	NodeEnv         string
	PassEnvironment bool
}

// NpmPatterns contains regex patterns for parsing NPM output
type NpmPatterns struct {
	ErrorPattern         *regexp.Regexp
	WarningPattern       *regexp.Regexp
	SuccessPattern       *regexp.Regexp
	ProgressPattern      *regexp.Regexp
	DeprecatedPattern    *regexp.Regexp
	VulnerabilityPattern *regexp.Regexp
	ScriptPattern        *regexp.Regexp
	TimingPattern        *regexp.Regexp
}

// PackageJson represents a package.json file structure
type PackageJson struct {
	Name             string            `json:"name"`
	Version          string            `json:"version"`
	Description      string            `json:"description"`
	Scripts          map[string]string `json:"scripts"`
	Dependencies     map[string]string `json:"dependencies"`
	DevDependencies  map[string]string `json:"devDependencies"`
	PeerDependencies map[string]string `json:"peerDependencies"`
	Engines          map[string]string `json:"engines"`
	Workspaces       any               `json:"workspaces"`
	Config           map[string]any    `json:"config"`
}

// NewNpmRunner creates a new NPM runner
func NewNpmRunner(logger logger.Logger) *NpmRunner {
	return &NpmRunner{
		logger:   logger.WithGroup("npm-runner"),
		options:  createDefaultNpmOptions(),
		patterns: createNpmPatterns(),
	}
}

// createDefaultNpmOptions creates default NPM options
func createDefaultNpmOptions() *NpmOptions {
	return &NpmOptions{
		NpmCommand:      "npm",
		NpmArgs:         []string{},
		PackageJsonFile: "package.json",
		WorkspaceRoot:   "",
		Silent:          false,
		Progress:        true,
		Production:      false,
		Development:     false,
		GlobalInstall:   false,
		Registry:        "",
		AuthToken:       "",
		Scope:           "",
		UseCache:        true,
		CacheDir:        "",
		OfflineMode:     false,
		PreferOffline:   false,
		LogLevel:        "info",
		JsonOutput:      false,
		LongOutput:      false,
		AuditLevel:      "moderate",
		IgnoreScripts:   false,
		NodeEnv:         "",
		PassEnvironment: true,
	}
}

// createNpmPatterns creates regex patterns for NPM output parsing
func createNpmPatterns() *NpmPatterns {
	return &NpmPatterns{
		ErrorPattern:         regexp.MustCompile(`^npm ERR!\s+(.*)$`),
		WarningPattern:       regexp.MustCompile(`^npm WARN\s+(.*)$`),
		SuccessPattern:       regexp.MustCompile(`^npm info\s+ok\s*$`),
		ProgressPattern:      regexp.MustCompile(`^npm info\s+.*\s+(\d+)%\s+(.*)$`),
		DeprecatedPattern:    regexp.MustCompile(`^npm WARN deprecated\s+(.*)$`),
		VulnerabilityPattern: regexp.MustCompile(`found (\d+) vulnerabilit(?:y|ies)`),
		ScriptPattern:        regexp.MustCompile(`^>\s+(.*)@.*\s+(.*)$`),
		TimingPattern:        regexp.MustCompile(`^npm timing\s+(.*)$`),
	}
}

// Type returns the runner type
func (nr *NpmRunner) Type() t.RunnerType {
	return t.RunnerNpm
}

// Name returns the runner name
func (nr *NpmRunner) Name() string {
	return "NPM Runner"
}

// Version returns the runner version
func (nr *NpmRunner) Version() string {
	return "1.0.0"
}

// CanRun checks if the runner can execute the given task
func (nr *NpmRunner) CanRun(task *t.Task) bool {
	if task.Runner != t.RunnerNpm {
		return false
	}

	// Check if NPM is available
	if !nr.isNpmAvailable() {
		nr.logger.Warn("NPM is not available")
		return false
	}

	// Check if package.json exists
	packageJsonPath := nr.findPackageJson(task)
	if packageJsonPath == "" {
		nr.logger.Warn("No package.json found", "task", task.Name)
		return false
	}

	return true
}

// Prepare prepares the task for execution
func (nr *NpmRunner) Prepare(ctx context.Context, task *t.Task) error {
	nr.logger.Debug("Preparing NPM task", "task", task.Name)

	// Validate NPM availability
	if !nr.isNpmAvailable() {
		return fmt.Errorf("npm command not found")
	}

	// Find and validate package.json
	packageJsonPath := nr.findPackageJson(task)
	if packageJsonPath == "" {
		return fmt.Errorf("no package.json found for task %s", task.Name)
	}

	// Parse package.json
	packageJson, err := nr.parsePackageJson(packageJsonPath)
	if err != nil {
		return fmt.Errorf("failed to parse package.json: %w", err)
	}

	// Validate script exists
	if err := nr.validateScript(packageJson, task.Name); err != nil {
		return fmt.Errorf("script validation failed: %w", err)
	}

	// Check for node_modules if not a global command
	if err := nr.checkDependencies(packageJsonPath); err != nil {
		nr.logger.Warn("Dependencies check failed", "error", err)
	}

	return nil
}

// Execute executes the NPM task
func (nr *NpmRunner) Execute(ctx context.Context, task *t.Task, outputBuffer *output.Buffer) error {
	nr.logger.Info("Executing NPM task", "task", task.Name)

	// Build command
	cmd, err := nr.buildCommand(task)
	if err != nil {
		return fmt.Errorf("failed to build command: %w", err)
	}

	// Set up command context
	cmd.Dir = nr.getWorkingDirectory(task)
	cmd.Env = nr.buildEnvironment(task)

	nr.logger.Debug("NPM command built",
		"cmd", cmd.Path,
		"args", cmd.Args,
		"dir", cmd.Dir)

	// Execute with output streaming
	return nr.executeWithOutput(ctx, cmd, outputBuffer)
}

// Stop stops the running NPM task
func (nr *NpmRunner) Stop(ctx context.Context, task *t.Task) error {
	nr.logger.Info("Stopping NPM task", "task", task.Name)

	// NPM doesn't have a built-in graceful stop mechanism for scripts
	// We rely on the process being killed by the executor
	return nil
}

// GetDefaultTimeout returns the default timeout for NPM tasks
func (nr *NpmRunner) GetDefaultTimeout() time.Duration {
	return 15 * time.Minute
}

// GetEnvironmentVariables returns environment variables for the task
func (nr *NpmRunner) GetEnvironmentVariables(task *t.Task) map[string]string {
	env := make(map[string]string)

	// Add NPM-specific variables
	env["npm_execpath"] = nr.options.NpmCommand
	env["npm_config_target_platform"] = "node"

	if nr.options.NodeEnv != "" {
		env["NODE_ENV"] = nr.options.NodeEnv
	}

	if task.FilePath != "" {
		env["npm_package_json"] = task.FilePath
		env["INIT_CWD"] = filepath.Dir(task.FilePath)
	}

	// Add registry if specified
	if nr.options.Registry != "" {
		env["npm_config_registry"] = nr.options.Registry
	}

	// Add cache directory if specified
	if nr.options.CacheDir != "" {
		env["npm_config_cache"] = nr.options.CacheDir
	}

	// Add package.json variables
	if packageJsonPath := nr.findPackageJson(task); packageJsonPath != "" {
		if packageJson, err := nr.parsePackageJson(packageJsonPath); err == nil {
			env["npm_package_name"] = packageJson.Name
			env["npm_package_version"] = packageJson.Version

			// Add script-specific variables
			if script, exists := packageJson.Scripts[task.Name]; exists {
				env["npm_lifecycle_event"] = task.Name
				env["npm_lifecycle_script"] = script
			}

			// Add config variables
			for key, value := range packageJson.Config {
				if strValue, ok := value.(string); ok {
					env[fmt.Sprintf("npm_package_config_%s", key)] = strValue
				}
			}
		}
	}

	return env
}

// Validate validates the task configuration
func (nr *NpmRunner) Validate(task *t.Task) error {
	if task.Runner != t.RunnerNpm {
		return fmt.Errorf("invalid runner type: expected %s, got %s", t.RunnerNpm, task.Runner)
	}

	if task.Name == "" {
		return fmt.Errorf("task name cannot be empty")
	}

	// Additional validation
	if task.Command != "" && task.Command != "npm" {
		nr.logger.Warn("Task has custom command, but NPM runner expects 'npm'", "command", task.Command)
	}

	return nil
}

// SetOptions sets runner options
func (nr *NpmRunner) SetOptions(options *NpmOptions) {
	nr.options = options
}

// GetOptions returns current runner options
func (nr *NpmRunner) GetOptions() *NpmOptions {
	return nr.options
}

// Private methods

// isNpmAvailable checks if NPM is available on the system
func (nr *NpmRunner) isNpmAvailable() bool {
	_, err := exec.LookPath(nr.options.NpmCommand)
	return err == nil
}

// findPackageJson finds the appropriate package.json for the task
func (nr *NpmRunner) findPackageJson(task *t.Task) string {
	// If task has a specific file path, use it
	if task.FilePath != "" && strings.HasSuffix(task.FilePath, "package.json") {
		if _, err := os.Stat(task.FilePath); err == nil {
			return task.FilePath
		}
	}

	// Look for package.json in task directory
	taskDir := task.WorkingDirectory
	if taskDir == "" && task.FilePath != "" {
		taskDir = filepath.Dir(task.FilePath)
	}
	if taskDir == "" {
		taskDir = "."
	}

	// Check current directory and parent directories
	currentDir := taskDir
	for {
		packageJsonPath := filepath.Join(currentDir, nr.options.PackageJsonFile)
		if _, err := os.Stat(packageJsonPath); err == nil {
			return packageJsonPath
		}

		parent := filepath.Dir(currentDir)
		if parent == currentDir || parent == "/" {
			break // Reached root
		}
		currentDir = parent
	}

	return ""
}

// parsePackageJson parses a package.json file
func (nr *NpmRunner) parsePackageJson(packageJsonPath string) (*PackageJson, error) {
	file, err := os.Open(packageJsonPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open package.json: %w", err)
	}
	defer file.Close()

	var packageJson PackageJson
	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&packageJson); err != nil {
		return nil, fmt.Errorf("failed to decode package.json: %w", err)
	}

	return &packageJson, nil
}

// validateScript checks if the script exists in package.json
func (nr *NpmRunner) validateScript(packageJson *PackageJson, scriptName string) error {
	if packageJson.Scripts == nil {
		return fmt.Errorf("no scripts defined in package.json")
	}

	if _, exists := packageJson.Scripts[scriptName]; !exists {
		return fmt.Errorf("script '%s' not found in package.json", scriptName)
	}

	return nil
}

// checkDependencies checks if node_modules exists and suggests npm install if not
func (nr *NpmRunner) checkDependencies(packageJsonPath string) error {
	packageDir := filepath.Dir(packageJsonPath)
	nodeModulesPath := filepath.Join(packageDir, "node_modules")

	if _, err := os.Stat(nodeModulesPath); os.IsNotExist(err) {
		return fmt.Errorf("node_modules not found, run 'npm install' first")
	}

	return nil
}

// buildCommand builds the NPM command
func (nr *NpmRunner) buildCommand(task *t.Task) (*exec.Cmd, error) {
	args := []string{}

	// Add default NPM arguments
	args = append(args, nr.options.NpmArgs...)

	// Determine command type
	if nr.isLifecycleScript(task.Name) {
		args = append(args, "run", task.Name)
	} else {
		// Built-in NPM commands
		args = append(args, task.Name)
	}

	// Add options based on configuration
	if nr.options.Silent {
		args = append(args, "--silent")
	}

	if !nr.options.Progress {
		args = append(args, "--no-progress")
	}

	if nr.options.Production {
		args = append(args, "--production")
	}

	if nr.options.Development {
		args = append(args, "--only=dev")
	}

	if nr.options.GlobalInstall {
		args = append(args, "--global")
	}

	if nr.options.Registry != "" {
		args = append(args, "--registry", nr.options.Registry)
	}

	if nr.options.CacheDir != "" {
		args = append(args, "--cache", nr.options.CacheDir)
	}

	if nr.options.OfflineMode {
		args = append(args, "--offline")
	} else if nr.options.PreferOffline {
		args = append(args, "--prefer-offline")
	}

	if nr.options.LogLevel != "" {
		args = append(args, "--loglevel", nr.options.LogLevel)
	}

	if nr.options.JsonOutput {
		args = append(args, "--json")
	}

	if nr.options.LongOutput {
		args = append(args, "--long")
	}

	if nr.options.IgnoreScripts {
		args = append(args, "--ignore-scripts")
	}

	// Add task arguments if any
	if len(task.Args) > 1 { // Skip first arg which is usually the script name
		// Add separator for script arguments
		if nr.isLifecycleScript(task.Name) {
			args = append(args, "--")
		}
		args = append(args, task.Args[1:]...)
	}

	cmd := exec.Command(nr.options.NpmCommand, args...)
	return cmd, nil
}

// isLifecycleScript checks if the task is a custom script vs built-in NPM command
func (nr *NpmRunner) isLifecycleScript(taskName string) bool {
	builtinCommands := []string{
		"install", "i", "uninstall", "remove", "rm",
		"update", "outdated", "list", "ls", "link", "unlink",
		"publish", "unpublish", "adduser", "logout",
		"whoami", "config", "cache", "version", "view",
		"search", "audit", "fund", "doctor", "ping",
		"repo", "docs", "bugs", "home", "help",
		"init", "start", "stop", "restart", "test",
	}

	for _, cmd := range builtinCommands {
		if taskName == cmd {
			return false
		}
	}

	return true
}

// getWorkingDirectory returns the working directory for the task
func (nr *NpmRunner) getWorkingDirectory(task *t.Task) string {
	if task.WorkingDirectory != "" {
		return task.WorkingDirectory
	}

	if packageJsonPath := nr.findPackageJson(task); packageJsonPath != "" {
		return filepath.Dir(packageJsonPath)
	}

	return "."
}

// buildEnvironment builds the environment variables for the command
func (nr *NpmRunner) buildEnvironment(task *t.Task) []string {
	var env []string

	// Start with current environment if enabled
	if nr.options.PassEnvironment {
		env = os.Environ()
	}

	// Add runner-specific environment variables
	runnerEnv := nr.GetEnvironmentVariables(task)
	for key, value := range runnerEnv {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}

	return env
}

// executeWithOutput executes the command with output streaming
func (nr *NpmRunner) executeWithOutput(ctx context.Context, cmd *exec.Cmd, outputBuffer *output.Buffer) error {
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
	go nr.streamOutput(stdout, outputBuffer, false)
	go nr.streamOutput(stderr, outputBuffer, true)

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
			return fmt.Errorf("npm command failed: %w", err)
		}
		return nil
	}
}

// streamOutput streams command output to the buffer
func (nr *NpmRunner) streamOutput(pipe any, outputBuffer *output.Buffer, isStderr bool) {
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
		enhancedLine := nr.parseOutputLine(line, isStderr)

		// Write to buffer
		if isStderr {
			outputBuffer.WriteStderr([]byte(enhancedLine + "\n"))
		} else {
			outputBuffer.WriteStdout([]byte(enhancedLine + "\n"))
		}
	}
}

// parseOutputLine parses and enhances an NPM output line
func (nr *NpmRunner) parseOutputLine(line string, isStderr bool) string {
	// Add timestamps and formatting
	timestamp := time.Now().Format("15:04:05.000")
	prefix := "OUT"
	if isStderr {
		prefix = "ERR"
	}

	// Check for special NPM patterns
	if nr.patterns.ErrorPattern.MatchString(line) {
		prefix = "ERROR"
	} else if nr.patterns.WarningPattern.MatchString(line) {
		prefix = "WARN"
	} else if nr.patterns.DeprecatedPattern.MatchString(line) {
		prefix = "DEPRECATED"
	} else if nr.patterns.VulnerabilityPattern.MatchString(line) {
		prefix = "SECURITY"
	} else if nr.patterns.ProgressPattern.MatchString(line) {
		prefix = "PROGRESS"
	} else if nr.patterns.TimingPattern.MatchString(line) {
		prefix = "TIMING"
	}

	return fmt.Sprintf("[%s] [%s] %s", timestamp, prefix, line)
}

// GetNpmVersion returns the version of NPM
func (nr *NpmRunner) GetNpmVersion() (string, error) {
	cmd := exec.Command(nr.options.NpmCommand, "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get NPM version: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// GetNodeVersion returns the version of Node.js
func (nr *NpmRunner) GetNodeVersion() (string, error) {
	cmd := exec.Command("node", "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get Node.js version: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// ListScripts lists all available scripts in package.json
func (nr *NpmRunner) ListScripts(packageJsonPath string) (map[string]string, error) {
	packageJson, err := nr.parsePackageJson(packageJsonPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse package.json: %w", err)
	}

	if packageJson.Scripts == nil {
		return make(map[string]string), nil
	}

	return packageJson.Scripts, nil
}

// GetTaskMetadata returns metadata specific to NPM tasks
func (nr *NpmRunner) GetTaskMetadata(task *t.Task) map[string]any {
	metadata := map[string]any{
		"runner_type":    nr.Type(),
		"runner_name":    nr.Name(),
		"runner_version": nr.Version(),
		"npm_command":    nr.options.NpmCommand,
	}

	if packageJsonPath := nr.findPackageJson(task); packageJsonPath != "" {
		metadata["package_json_path"] = packageJsonPath

		// Add package.json info
		if packageJson, err := nr.parsePackageJson(packageJsonPath); err == nil {
			metadata["package_name"] = packageJson.Name
			metadata["package_version"] = packageJson.Version
			metadata["package_description"] = packageJson.Description

			if scripts, err := nr.ListScripts(packageJsonPath); err == nil {
				metadata["available_scripts"] = scripts
			}

			// Add dependency counts
			metadata["dependencies_count"] = len(packageJson.Dependencies)
			metadata["dev_dependencies_count"] = len(packageJson.DevDependencies)
		}
	}

	// Add NPM version
	if version, err := nr.GetNpmVersion(); err == nil {
		metadata["npm_version"] = version
	}

	// Add Node.js version
	if version, err := nr.GetNodeVersion(); err == nil {
		metadata["node_version"] = version
	}

	return metadata
}
