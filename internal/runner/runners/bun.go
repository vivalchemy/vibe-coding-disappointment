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

// BunRunner implements the Runner interface for Bun tasks
type BunRunner struct {
	logger   logger.Logger
	options  *BunOptions
	patterns *BunPatterns
}

// BunOptions contains configuration for the Bun runner
type BunOptions struct {
	// Bun executable
	BunCommand string
	BunArgs    []string

	// Package.json handling
	PackageJsonFile string
	WorkspaceRoot   string

	// Execution options
	Silent      bool
	Verbose     bool
	Production  bool
	Development bool
	Watch       bool
	Hot         bool

	// Bun-specific options
	BunVersion   string
	UseJSX       bool
	Target       string
	Format       string
	MinifyOutput bool
	SourceMap    bool

	// Workspace options
	Workspaces       bool
	WorkspacePattern string
	Filter           string

	// Cache and performance
	UseCache      bool
	CacheDir      string
	NoInstall     bool
	PreferOffline bool

	// Registry and authentication
	Registry  string
	AuthToken string
	Scope     string

	// Output handling
	JsonOutput bool
	NoProgress bool
	LogLevel   string

	// Security
	TrustedDependencies []string
	IgnoreScripts       bool

	// Environment
	NodeEnv         string
	PassEnvironment bool
}

// BunPatterns contains regex patterns for parsing Bun output
type BunPatterns struct {
	ErrorPattern     *regexp.Regexp
	WarningPattern   *regexp.Regexp
	SuccessPattern   *regexp.Regexp
	ProgressPattern  *regexp.Regexp
	InstallPattern   *regexp.Regexp
	BuildPattern     *regexp.Regexp
	RunPattern       *regexp.Regexp
	TestPattern      *regexp.Regexp
	DevPattern       *regexp.Regexp
	HotReloadPattern *regexp.Regexp
	TimingPattern    *regexp.Regexp
	MemoryPattern    *regexp.Regexp
}

// BunConfig represents bun.config.ts/js structure
type BunConfig struct {
	Preload     []string       `json:"preload,omitempty"`
	Entrypoints []string       `json:"entrypoints,omitempty"`
	Outdir      string         `json:"outdir,omitempty"`
	Target      string         `json:"target,omitempty"`
	Format      string         `json:"format,omitempty"`
	Minify      bool           `json:"minify,omitempty"`
	Sourcemap   string         `json:"sourcemap,omitempty"`
	External    []string       `json:"external,omitempty"`
	Define      map[string]any `json:"define,omitempty"`
}

// BunLockfile represents bun.lockb metadata (simplified)
type BunLockfile struct {
	Version      string `json:"version"`
	PackageCount int    `json:"packageCount"`
}

// NewBunRunner creates a new Bun runner
func NewBunRunner(logger logger.Logger) *BunRunner {
	runner := &BunRunner{
		logger:   logger.WithGroup("bun-runner"),
		options:  createDefaultBunOptions(),
		patterns: createBunPatterns(),
	}

	// Detect Bun version
	runner.detectBunVersion()

	return runner
}

// createDefaultBunOptions creates default Bun options
func createDefaultBunOptions() *BunOptions {
	return &BunOptions{
		BunCommand:          "bun",
		BunArgs:             []string{},
		PackageJsonFile:     "package.json",
		WorkspaceRoot:       "",
		Silent:              false,
		Verbose:             false,
		Production:          false,
		Development:         false,
		Watch:               false,
		Hot:                 false,
		BunVersion:          "",
		UseJSX:              false,
		Target:              "bun",
		Format:              "esm",
		MinifyOutput:        false,
		SourceMap:           false,
		Workspaces:          false,
		WorkspacePattern:    "",
		Filter:              "",
		UseCache:            true,
		CacheDir:            "",
		NoInstall:           false,
		PreferOffline:       false,
		Registry:            "",
		AuthToken:           "",
		Scope:               "",
		JsonOutput:          false,
		NoProgress:          false,
		LogLevel:            "info",
		TrustedDependencies: []string{},
		IgnoreScripts:       false,
		NodeEnv:             "",
		PassEnvironment:     true,
	}
}

// createBunPatterns creates regex patterns for Bun output parsing
func createBunPatterns() *BunPatterns {
	return &BunPatterns{
		ErrorPattern:     regexp.MustCompile(`^error:\s+(.*)$`),
		WarningPattern:   regexp.MustCompile(`^warn:\s+(.*)$`),
		SuccessPattern:   regexp.MustCompile(`^✓\s+(.*)$`),
		ProgressPattern:  regexp.MustCompile(`^\[\d+/\d+\]\s+(.*)$`),
		InstallPattern:   regexp.MustCompile(`^bun install\s+(.*)$`),
		BuildPattern:     regexp.MustCompile(`^bun build\s+(.*)$`),
		RunPattern:       regexp.MustCompile(`^\$\s+bun\s+run\s+(.*)$`),
		TestPattern:      regexp.MustCompile(`^bun test\s+(.*)$`),
		DevPattern:       regexp.MustCompile(`^bun dev\s+(.*)$`),
		HotReloadPattern: regexp.MustCompile(`^Hot reloaded\s+(.*)$`),
		TimingPattern:    regexp.MustCompile(`^(.*)ms\s+(.*)$`),
		MemoryPattern:    regexp.MustCompile(`^(\d+)MB\s+(.*)$`),
	}
}

// Type returns the runner type
func (br *BunRunner) Type() t.RunnerType {
	return t.RunnerBun
}

// Name returns the runner name
func (br *BunRunner) Name() string {
	return "Bun Runner"
}

// Version returns the runner version
func (br *BunRunner) Version() string {
	return "1.0.0"
}

// CanRun checks if the runner can execute the given task
func (br *BunRunner) CanRun(task *t.Task) bool {
	if task.Runner != t.RunnerBun {
		return false
	}

	// Check if Bun is available
	if !br.isBunAvailable() {
		br.logger.Warn("Bun is not available")
		return false
	}

	// Check if package.json exists
	packageJsonPath := br.findPackageJson(task)
	if packageJsonPath == "" {
		br.logger.Warn("No package.json found", "task", task.Name)
		return false
	}

	return true
}

// Prepare prepares the task for execution
func (br *BunRunner) Prepare(ctx context.Context, task *t.Task) error {
	br.logger.Debug("Preparing Bun task", "task", task.Name)

	// Validate Bun availability
	if !br.isBunAvailable() {
		return fmt.Errorf("bun command not found")
	}

	// Find and validate package.json
	packageJsonPath := br.findPackageJson(task)
	if packageJsonPath == "" {
		return fmt.Errorf("no package.json found for task %s", task.Name)
	}

	// Parse package.json
	packageJson, err := br.parsePackageJson(packageJsonPath)
	if err != nil {
		return fmt.Errorf("failed to parse package.json: %w", err)
	}

	// Validate script exists (for script tasks)
	if br.isLifecycleScript(task.Name) {
		if err := br.validateScript(packageJson, task.Name); err != nil {
			return fmt.Errorf("script validation failed: %w", err)
		}
	}

	// Check for bun.lockb
	if err := br.checkLockfile(packageJsonPath); err != nil {
		br.logger.Warn("Lockfile check failed", "error", err)
	}

	// Detect workspace configuration
	if err := br.detectWorkspaces(packageJsonPath); err != nil {
		br.logger.Warn("Workspace detection failed", "error", err)
	}

	// Load Bun configuration
	if err := br.loadBunConfig(packageJsonPath); err != nil {
		br.logger.Debug("No Bun config found or failed to load", "error", err)
	}

	return nil
}

// Execute executes the Bun task
func (br *BunRunner) Execute(ctx context.Context, task *t.Task, outputBuffer *output.Buffer) error {
	br.logger.Info("Executing Bun task", "task", task.Name)

	// Build command
	cmd, err := br.buildCommand(task)
	if err != nil {
		return fmt.Errorf("failed to build command: %w", err)
	}

	// Set up command context
	cmd.Dir = br.getWorkingDirectory(task)
	cmd.Env = br.buildEnvironment(task)

	br.logger.Debug("Bun command built",
		"cmd", cmd.Path,
		"args", cmd.Args,
		"dir", cmd.Dir,
		"bun_version", br.options.BunVersion)

	// Execute with output streaming
	return br.executeWithOutput(ctx, cmd, outputBuffer)
}

// Stop stops the running Bun task
func (br *BunRunner) Stop(ctx context.Context, task *t.Task) error {
	br.logger.Info("Stopping Bun task", "task", task.Name)

	// Bun supports graceful shutdown for dev servers and watch mode
	// We rely on the process being killed by the executor
	return nil
}

// GetDefaultTimeout returns the default timeout for Bun tasks
func (br *BunRunner) GetDefaultTimeout() time.Duration {
	return 10 * time.Minute
}

// GetEnvironmentVariables returns environment variables for the task
func (br *BunRunner) GetEnvironmentVariables(task *t.Task) map[string]string {
	env := make(map[string]string)

	// Add Bun-specific variables
	env["BUN_RUNTIME"] = "bun"
	env["npm_execpath"] = br.options.BunCommand
	env["npm_config_user_agent"] = fmt.Sprintf("bun/%s", br.options.BunVersion)

	if br.options.NodeEnv != "" {
		env["NODE_ENV"] = br.options.NodeEnv
	}

	if task.FilePath != "" {
		env["npm_package_json"] = task.FilePath
		env["INIT_CWD"] = filepath.Dir(task.FilePath)
	}

	// Add registry if specified
	if br.options.Registry != "" {
		env["npm_config_registry"] = br.options.Registry
	}

	// Add cache directory if specified
	if br.options.CacheDir != "" {
		env["BUN_INSTALL_CACHE_DIR"] = br.options.CacheDir
	}

	// Add Bun version info
	env["BUN_VERSION"] = br.options.BunVersion

	// Add build-specific variables
	if br.options.Target != "" {
		env["BUN_TARGET"] = br.options.Target
	}

	if br.options.Format != "" {
		env["BUN_FORMAT"] = br.options.Format
	}

	// Add package.json variables
	if packageJsonPath := br.findPackageJson(task); packageJsonPath != "" {
		if packageJson, err := br.parsePackageJson(packageJsonPath); err == nil {
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
func (br *BunRunner) Validate(task *t.Task) error {
	if task.Runner != t.RunnerBun {
		return fmt.Errorf("invalid runner type: expected %s, got %s", t.RunnerBun, task.Runner)
	}

	if task.Name == "" {
		return fmt.Errorf("task name cannot be empty")
	}

	// Additional validation
	if task.Command != "" && task.Command != "bun" {
		br.logger.Warn("Task has custom command, but Bun runner expects 'bun'", "command", task.Command)
	}

	return nil
}

// SetOptions sets runner options
func (br *BunRunner) SetOptions(options *BunOptions) {
	br.options = options
}

// GetOptions returns current runner options
func (br *BunRunner) GetOptions() *BunOptions {
	return br.options
}

// Private methods

// isBunAvailable checks if Bun is available on the system
func (br *BunRunner) isBunAvailable() bool {
	_, err := exec.LookPath(br.options.BunCommand)
	return err == nil
}

// detectBunVersion detects the Bun version
func (br *BunRunner) detectBunVersion() {
	if version, err := br.GetBunVersion(); err == nil {
		br.options.BunVersion = version
		br.logger.Debug("Detected Bun version", "version", version)
	}
}

// findPackageJson finds the appropriate package.json for the task
func (br *BunRunner) findPackageJson(task *t.Task) string {
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
		packageJsonPath := filepath.Join(currentDir, br.options.PackageJsonFile)
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
func (br *BunRunner) parsePackageJson(packageJsonPath string) (*PackageJson, error) {
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
func (br *BunRunner) validateScript(packageJson *PackageJson, scriptName string) error {
	if packageJson.Scripts == nil {
		return fmt.Errorf("no scripts defined in package.json")
	}

	if _, exists := packageJson.Scripts[scriptName]; !exists {
		return fmt.Errorf("script '%s' not found in package.json", scriptName)
	}

	return nil
}

// checkLockfile checks if bun.lockb exists
func (br *BunRunner) checkLockfile(packageJsonPath string) error {
	packageDir := filepath.Dir(packageJsonPath)
	lockfilePath := filepath.Join(packageDir, "bun.lockb")

	if _, err := os.Stat(lockfilePath); os.IsNotExist(err) {
		return fmt.Errorf("no bun.lockb found, run 'bun install' first")
	}

	return nil
}

// detectWorkspaces detects workspace configuration
func (br *BunRunner) detectWorkspaces(packageJsonPath string) error {
	packageJson, err := br.parsePackageJson(packageJsonPath)
	if err != nil {
		return err
	}

	// Check if workspaces are defined
	if packageJson.Workspaces != nil {
		br.options.Workspaces = true
		br.options.WorkspaceRoot = filepath.Dir(packageJsonPath)
		br.logger.Debug("Detected Bun workspaces", "root", br.options.WorkspaceRoot)
	}

	return nil
}

// loadBunConfig loads Bun configuration from bun.config.js/ts
func (br *BunRunner) loadBunConfig(packageJsonPath string) error {
	packageDir := filepath.Dir(packageJsonPath)

	// Check for different config files
	configFiles := []string{"bun.config.js", "bun.config.ts", "bunfig.toml"}

	for _, configFile := range configFiles {
		configPath := filepath.Join(packageDir, configFile)
		if _, err := os.Stat(configPath); err == nil {
			br.logger.Debug("Found Bun config", "file", configFile)
			// Config parsing would be implemented here
			return nil
		}
	}

	return fmt.Errorf("no Bun config found")
}

// buildCommand builds the Bun command
func (br *BunRunner) buildCommand(task *t.Task) (*exec.Cmd, error) {
	args := []string{}

	// Add default Bun arguments
	args = append(args, br.options.BunArgs...)

	// Determine command type and add appropriate subcommand
	if br.isLifecycleScript(task.Name) {
		args = append(args, "run", task.Name)
	} else {
		// Built-in Bun commands
		args = append(args, task.Name)
	}

	// Add options based on configuration
	if br.options.Silent {
		args = append(args, "--silent")
	}

	if br.options.Verbose {
		args = append(args, "--verbose")
	}

	if br.options.Production {
		args = append(args, "--production")
	}

	if br.options.Development {
		args = append(args, "--development")
	}

	if br.options.Watch {
		args = append(args, "--watch")
	}

	if br.options.Hot {
		args = append(args, "--hot")
	}

	if br.options.Registry != "" {
		args = append(args, "--registry", br.options.Registry)
	}

	if br.options.CacheDir != "" {
		args = append(args, "--cache-dir", br.options.CacheDir)
	}

	if br.options.NoInstall {
		args = append(args, "--no-install")
	}

	if br.options.PreferOffline {
		args = append(args, "--prefer-offline")
	}

	if br.options.JsonOutput {
		args = append(args, "--json")
	}

	if br.options.NoProgress {
		args = append(args, "--no-progress")
	}

	if br.options.LogLevel != "" && br.options.LogLevel != "info" {
		args = append(args, "--log-level", br.options.LogLevel)
	}

	if br.options.IgnoreScripts {
		args = append(args, "--ignore-scripts")
	}

	// Add build-specific options for build commands
	if task.Name == "build" || strings.Contains(strings.Join(task.Args, " "), "build") {
		if br.options.Target != "" && br.options.Target != "bun" {
			args = append(args, "--target", br.options.Target)
		}

		if br.options.Format != "" && br.options.Format != "esm" {
			args = append(args, "--format", br.options.Format)
		}

		if br.options.MinifyOutput {
			args = append(args, "--minify")
		}

		if br.options.SourceMap {
			args = append(args, "--sourcemap")
		}
	}

	// Add workspace options
	if br.options.Workspaces {
		if br.options.Filter != "" {
			args = append(args, "--filter", br.options.Filter)
		}

		if br.options.WorkspacePattern != "" {
			args = append(args, "--workspace", br.options.WorkspacePattern)
		}
	}

	// Add trusted dependencies
	for _, dep := range br.options.TrustedDependencies {
		args = append(args, "--trust", dep)
	}

	// Add task arguments if any
	if len(task.Args) > 1 { // Skip first arg which is usually the script name
		// Add separator for script arguments
		if br.isLifecycleScript(task.Name) {
			args = append(args, "--")
		}
		args = append(args, task.Args[1:]...)
	}

	cmd := exec.Command(br.options.BunCommand, args...)
	return cmd, nil
}

// isLifecycleScript checks if the task is a custom script vs built-in Bun command
func (br *BunRunner) isLifecycleScript(taskName string) bool {
	builtinCommands := []string{
		"install", "i", "add", "remove", "rm", "update", "outdated",
		"link", "unlink", "pm", "run", "test", "build", "dev",
		"create", "init", "upgrade", "patch", "patch-commit",
		"x", "exec", "completions", "help", "version",
		// Bun-specific commands
		"bun", "js", "jsx", "ts", "tsx", "compile", "bundle",
		"install-completions", "shell-completions",
	}

	for _, cmd := range builtinCommands {
		if taskName == cmd {
			return false
		}
	}

	return true
}

// getWorkingDirectory returns the working directory for the task
func (br *BunRunner) getWorkingDirectory(task *t.Task) string {
	if task.WorkingDirectory != "" {
		return task.WorkingDirectory
	}

	if packageJsonPath := br.findPackageJson(task); packageJsonPath != "" {
		return filepath.Dir(packageJsonPath)
	}

	return "."
}

// buildEnvironment builds the environment variables for the command
func (br *BunRunner) buildEnvironment(task *t.Task) []string {
	var env []string

	// Start with current environment if enabled
	if br.options.PassEnvironment {
		env = os.Environ()
	}

	// Add runner-specific environment variables
	runnerEnv := br.GetEnvironmentVariables(task)
	for key, value := range runnerEnv {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}

	return env
}

// executeWithOutput executes the command with output streaming
func (br *BunRunner) executeWithOutput(ctx context.Context, cmd *exec.Cmd, outputBuffer *output.Buffer) error {
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
	go br.streamOutput(stdout, outputBuffer, false)
	go br.streamOutput(stderr, outputBuffer, true)

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
			return fmt.Errorf("bun command failed: %w", err)
		}
		return nil
	}
}

// streamOutput streams command output to the buffer
func (br *BunRunner) streamOutput(pipe any, outputBuffer *output.Buffer, isStderr bool) {
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
		enhancedLine := br.parseOutputLine(line, isStderr)

		// Write to buffer
		if isStderr {
			outputBuffer.WriteStderr([]byte(enhancedLine + "\n"))
		} else {
			outputBuffer.WriteStdout([]byte(enhancedLine + "\n"))
		}
	}
}

// parseOutputLine parses and enhances a Bun output line
func (br *BunRunner) parseOutputLine(line string, isStderr bool) string {
	// Add timestamps and formatting
	timestamp := time.Now().Format("15:04:05.000")
	prefix := "OUT"
	if isStderr {
		prefix = "ERR"
	}

	// Check for special Bun patterns
	if br.patterns.ErrorPattern.MatchString(line) {
		prefix = "ERROR"
	} else if br.patterns.WarningPattern.MatchString(line) {
		prefix = "WARN"
	} else if br.patterns.SuccessPattern.MatchString(line) {
		prefix = "SUCCESS"
	} else if br.patterns.ProgressPattern.MatchString(line) {
		prefix = "PROGRESS"
	} else if br.patterns.InstallPattern.MatchString(line) {
		prefix = "INSTALL"
	} else if br.patterns.BuildPattern.MatchString(line) {
		prefix = "BUILD"
	} else if br.patterns.RunPattern.MatchString(line) {
		prefix = "RUN"
	} else if br.patterns.TestPattern.MatchString(line) {
		prefix = "TEST"
	} else if br.patterns.DevPattern.MatchString(line) {
		prefix = "DEV"
	} else if br.patterns.HotReloadPattern.MatchString(line) {
		prefix = "HOT"
	} else if br.patterns.TimingPattern.MatchString(line) {
		prefix = "TIMING"
	} else if br.patterns.MemoryPattern.MatchString(line) {
		prefix = "MEMORY"
	}

	return fmt.Sprintf("[%s] [%s] %s", timestamp, prefix, line)
}

// GetBunVersion returns the version of Bun
func (br *BunRunner) GetBunVersion() (string, error) {
	cmd := exec.Command(br.options.BunCommand, "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get Bun version: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// GetBunInfo returns comprehensive Bun installation info
func (br *BunRunner) GetBunInfo() (map[string]any, error) {
	cmd := exec.Command(br.options.BunCommand, "--print", "process.versions")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get Bun info: %w", err)
	}

	var info map[string]any
	if err := json.Unmarshal(output, &info); err != nil {
		// Fallback to basic info
		return map[string]any{
			"bun": br.options.BunVersion,
		}, nil
	}

	return info, nil
}

// ListScripts lists all available scripts in package.json
func (br *BunRunner) ListScripts(packageJsonPath string) (map[string]string, error) {
	packageJson, err := br.parsePackageJson(packageJsonPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse package.json: %w", err)
	}

	if packageJson.Scripts == nil {
		return make(map[string]string), nil
	}

	return packageJson.Scripts, nil
}

// GetWorkspaceProjects returns list of workspace projects
func (br *BunRunner) GetWorkspaceProjects(packageJsonPath string) ([]string, error) {
	packageDir := filepath.Dir(packageJsonPath)

	cmd := exec.Command(br.options.BunCommand, "pm", "ls", "--all", "--json")
	cmd.Dir = packageDir
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list workspace projects: %w", err)
	}

	var projects []map[string]any
	if err := json.Unmarshal(output, &projects); err != nil {
		return nil, fmt.Errorf("failed to parse workspace projects: %w", err)
	}

	var projectNames []string
	for _, project := range projects {
		if name, ok := project["name"].(string); ok {
			projectNames = append(projectNames, name)
		}
	}

	return projectNames, nil
}

// GetTaskMetadata returns metadata specific to Bun tasks
func (br *BunRunner) GetTaskMetadata(task *t.Task) map[string]any {
	metadata := map[string]any{
		"runner_type":    br.Type(),
		"runner_name":    br.Name(),
		"runner_version": br.Version(),
		"bun_command":    br.options.BunCommand,
		"bun_version":    br.options.BunVersion,
	}

	if packageJsonPath := br.findPackageJson(task); packageJsonPath != "" {
		metadata["package_json_path"] = packageJsonPath

		// Add package.json info
		if packageJson, err := br.parsePackageJson(packageJsonPath); err == nil {
			metadata["package_name"] = packageJson.Name
			metadata["package_version"] = packageJson.Version
			metadata["package_description"] = packageJson.Description

			if scripts, err := br.ListScripts(packageJsonPath); err == nil {
				metadata["available_scripts"] = scripts
			}

			// Add dependency counts
			metadata["dependencies_count"] = len(packageJson.Dependencies)
			metadata["dev_dependencies_count"] = len(packageJson.DevDependencies)

			// Add workspace info
			if br.options.Workspaces {
				metadata["workspaces_enabled"] = true
				metadata["workspace_root"] = br.options.WorkspaceRoot

				if projects, err := br.GetWorkspaceProjects(packageJsonPath); err == nil {
					metadata["workspace_projects"] = projects
					metadata["workspace_count"] = len(projects)
				}
			}
		}
	}

	// Add Bun runtime info
	if bunInfo, err := br.GetBunInfo(); err == nil {
		metadata["bun_runtime_info"] = bunInfo
	}

	// Add build configuration
	metadata["build_config"] = map[string]any{
		"target":    br.options.Target,
		"format":    br.options.Format,
		"minify":    br.options.MinifyOutput,
		"sourcemap": br.options.SourceMap,
		"use_jsx":   br.options.UseJSX,
	}

	return metadata
}
