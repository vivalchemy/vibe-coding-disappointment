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

// PnpmRunner implements the Runner interface for pnpm tasks
type PnpmRunner struct {
	logger   logger.Logger
	options  *PnpmOptions
	patterns *PnpmPatterns
}

// PnpmOptions contains configuration for the pnpm runner
type PnpmOptions struct {
	// pnpm executable
	PnpmCommand string
	PnpmArgs    []string

	// Package.json handling
	PackageJsonFile string
	WorkspaceRoot   string

	// Execution options
	Silent               bool
	Verbose              bool
	Production           bool
	Development          bool
	FrozenLockfile       bool
	IgnoreOptional       bool
	PreferFrozenLockfile bool

	// Workspace options
	Workspaces       bool
	WorkspacePattern string
	Recursive        bool
	Filter           string

	// Store and cache
	StoreDir        string
	CacheDir        string
	UseStoreServer  bool
	SharedWorkspace bool

	// Registry and authentication
	Registry  string
	AuthToken string
	Scope     string

	// Performance options
	NetworkConcurrency int
	ChildConcurrency   int
	SideEffectsCache   bool

	// Output handling
	JsonOutput   bool
	NoProgress   bool
	ReporterType string

	// Security and validation
	IgnoreScripts        bool
	CheckFiles           bool
	VerifyStoreIntegrity bool

	// Environment
	NodeEnv         string
	PassEnvironment bool
}

// PnpmPatterns contains regex patterns for parsing pnpm output
type PnpmPatterns struct {
	ErrorPattern      *regexp.Regexp
	WarningPattern    *regexp.Regexp
	SuccessPattern    *regexp.Regexp
	ProgressPattern   *regexp.Regexp
	ResolutionPattern *regexp.Regexp
	FetchingPattern   *regexp.Regexp
	LinkingPattern    *regexp.Regexp
	BuildingPattern   *regexp.Regexp
	WorkspacePattern  *regexp.Regexp
	ScriptPattern     *regexp.Regexp
	TimingPattern     *regexp.Regexp
	AuditPattern      *regexp.Regexp
	StorePattern      *regexp.Regexp
}

// PnpmWorkspace represents a pnpm workspace configuration
type PnpmWorkspace struct {
	Packages []string `json:"packages" yaml:"packages"`
}

// PnpmLockfile represents pnpm-lock.yaml structure (simplified)
type PnpmLockfile struct {
	LockfileVersion string                    `yaml:"lockfileVersion"`
	Dependencies    map[string]string         `yaml:"dependencies"`
	DevDependencies map[string]string         `yaml:"devDependencies"`
	Packages        map[string]map[string]any `yaml:"packages"`
}

// NewPnpmRunner creates a new pnpm runner
func NewPnpmRunner(logger logger.Logger) *PnpmRunner {
	return &PnpmRunner{
		logger:   logger.WithGroup("pnpm-runner"),
		options:  createDefaultPnpmOptions(),
		patterns: createPnpmPatterns(),
	}
}

// createDefaultPnpmOptions creates default pnpm options
func createDefaultPnpmOptions() *PnpmOptions {
	return &PnpmOptions{
		PnpmCommand:          "pnpm",
		PnpmArgs:             []string{},
		PackageJsonFile:      "package.json",
		WorkspaceRoot:        "",
		Silent:               false,
		Verbose:              false,
		Production:           false,
		Development:          false,
		FrozenLockfile:       false,
		IgnoreOptional:       false,
		PreferFrozenLockfile: false,
		Workspaces:           false,
		WorkspacePattern:     "",
		Recursive:            false,
		Filter:               "",
		StoreDir:             "",
		CacheDir:             "",
		UseStoreServer:       false,
		SharedWorkspace:      false,
		Registry:             "",
		AuthToken:            "",
		Scope:                "",
		NetworkConcurrency:   16,
		ChildConcurrency:     5,
		SideEffectsCache:     true,
		JsonOutput:           false,
		NoProgress:           false,
		ReporterType:         "default",
		IgnoreScripts:        false,
		CheckFiles:           false,
		VerifyStoreIntegrity: false,
		NodeEnv:              "",
		PassEnvironment:      true,
	}
}

// createPnpmPatterns creates regex patterns for pnpm output parsing
func createPnpmPatterns() *PnpmPatterns {
	return &PnpmPatterns{
		ErrorPattern:      regexp.MustCompile(`^ERR_PNPM_.*\s+(.*)$`),
		WarningPattern:    regexp.MustCompile(`^WARN\s+(.*)$`),
		SuccessPattern:    regexp.MustCompile(`^.*\s+✓\s+(.*)$`),
		ProgressPattern:   regexp.MustCompile(`^Progress:\s+resolved\s+(\d+),\s+reused\s+(\d+),\s+downloaded\s+(\d+)`),
		ResolutionPattern: regexp.MustCompile(`^Resolving:\s+(.*)$`),
		FetchingPattern:   regexp.MustCompile(`^Downloading\s+(.*)$`),
		LinkingPattern:    regexp.MustCompile(`^Linking\s+dependencies\s+for\s+(.*)$`),
		BuildingPattern:   regexp.MustCompile(`^Building\s+(.*)$`),
		WorkspacePattern:  regexp.MustCompile(`^Scope:\s+(.*)$`),
		ScriptPattern:     regexp.MustCompile(`^>\s+(.*)@.*\s+(.*)$`),
		TimingPattern:     regexp.MustCompile(`^Done\s+in\s+(.*)s?$`),
		AuditPattern:      regexp.MustCompile(`^(\d+)\s+vulnerabilit(?:y|ies)\s+found`),
		StorePattern:      regexp.MustCompile(`^.*\s+packages\s+are\s+hard\s+linked\s+from\s+the\s+content-addressable\s+store$`),
	}
}

// Type returns the runner type
func (pr *PnpmRunner) Type() t.RunnerType {
	return t.RunnerPnpm
}

// Name returns the runner name
func (pr *PnpmRunner) Name() string {
	return "pnpm Runner"
}

// Version returns the runner version
func (pr *PnpmRunner) Version() string {
	return "1.0.0"
}

// CanRun checks if the runner can execute the given task
func (pr *PnpmRunner) CanRun(task *t.Task) bool {
	if task.Runner != t.RunnerPnpm {
		return false
	}

	// Check if pnpm is available
	if !pr.isPnpmAvailable() {
		pr.logger.Warn("pnpm is not available")
		return false
	}

	// Check if package.json exists
	packageJsonPath := pr.findPackageJson(task)
	if packageJsonPath == "" {
		pr.logger.Warn("No package.json found", "task", task.Name)
		return false
	}

	return true
}

// Prepare prepares the task for execution
func (pr *PnpmRunner) Prepare(ctx context.Context, task *t.Task) error {
	pr.logger.Debug("Preparing pnpm task", "task", task.Name)

	// Validate pnpm availability
	if !pr.isPnpmAvailable() {
		return fmt.Errorf("pnpm command not found")
	}

	// Find and validate package.json
	packageJsonPath := pr.findPackageJson(task)
	if packageJsonPath == "" {
		return fmt.Errorf("no package.json found for task %s", task.Name)
	}

	// Parse package.json
	packageJson, err := pr.parsePackageJson(packageJsonPath)
	if err != nil {
		return fmt.Errorf("failed to parse package.json: %w", err)
	}

	// Validate script exists
	if err := pr.validateScript(packageJson, task.Name); err != nil {
		return fmt.Errorf("script validation failed: %w", err)
	}

	// Check for pnpm-lock.yaml
	if err := pr.checkLockfile(packageJsonPath); err != nil {
		pr.logger.Warn("Lockfile check failed", "error", err)
	}

	// Detect workspace configuration
	if err := pr.detectWorkspaces(packageJsonPath); err != nil {
		pr.logger.Warn("Workspace detection failed", "error", err)
	}

	// Check store integrity if enabled
	if pr.options.VerifyStoreIntegrity {
		if err := pr.verifyStore(ctx); err != nil {
			pr.logger.Warn("Store verification failed", "error", err)
		}
	}

	return nil
}

// Execute executes the pnpm task
func (pr *PnpmRunner) Execute(ctx context.Context, task *t.Task, outputBuffer *output.Buffer) error {
	pr.logger.Info("Executing pnpm task", "task", task.Name)

	// Build command
	cmd, err := pr.buildCommand(task)
	if err != nil {
		return fmt.Errorf("failed to build command: %w", err)
	}

	// Set up command context
	cmd.Dir = pr.getWorkingDirectory(task)
	cmd.Env = pr.buildEnvironment(task)

	pr.logger.Debug("pnpm command built",
		"cmd", cmd.Path,
		"args", cmd.Args,
		"dir", cmd.Dir)

	// Execute with output streaming
	return pr.executeWithOutput(ctx, cmd, outputBuffer)
}

// Stop stops the running pnpm task
func (pr *PnpmRunner) Stop(ctx context.Context, task *t.Task) error {
	pr.logger.Info("Stopping pnpm task", "task", task.Name)

	// pnpm doesn't have a built-in graceful stop mechanism for scripts
	// We rely on the process being killed by the executor
	return nil
}

// GetDefaultTimeout returns the default timeout for pnpm tasks
func (pr *PnpmRunner) GetDefaultTimeout() time.Duration {
	return 15 * time.Minute
}

// GetEnvironmentVariables returns environment variables for the task
func (pr *PnpmRunner) GetEnvironmentVariables(task *t.Task) map[string]string {
	env := make(map[string]string)

	// Add pnpm-specific variables
	env["npm_execpath"] = pr.options.PnpmCommand
	env["npm_config_user_agent"] = "pnpm"

	if pr.options.NodeEnv != "" {
		env["NODE_ENV"] = pr.options.NodeEnv
	}

	if task.FilePath != "" {
		env["npm_package_json"] = task.FilePath
		env["INIT_CWD"] = filepath.Dir(task.FilePath)
	}

	// Add registry if specified
	if pr.options.Registry != "" {
		env["npm_config_registry"] = pr.options.Registry
	}

	// Add store directory if specified
	if pr.options.StoreDir != "" {
		env["PNPM_HOME"] = pr.options.StoreDir
	}

	// Add cache directory if specified
	if pr.options.CacheDir != "" {
		env["PNPM_CACHE_DIR"] = pr.options.CacheDir
	}

	// Add concurrency settings
	env["npm_config_network_concurrency"] = fmt.Sprintf("%d", pr.options.NetworkConcurrency)
	env["npm_config_child_concurrency"] = fmt.Sprintf("%d", pr.options.ChildConcurrency)

	// Add package.json variables
	if packageJsonPath := pr.findPackageJson(task); packageJsonPath != "" {
		if packageJson, err := pr.parsePackageJson(packageJsonPath); err == nil {
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
func (pr *PnpmRunner) Validate(task *t.Task) error {
	if task.Runner != t.RunnerPnpm {
		return fmt.Errorf("invalid runner type: expected %s, got %s", t.RunnerPnpm, task.Runner)
	}

	if task.Name == "" {
		return fmt.Errorf("task name cannot be empty")
	}

	// Additional validation
	if task.Command != "" && task.Command != "pnpm" {
		pr.logger.Warn("Task has custom command, but pnpm runner expects 'pnpm'", "command", task.Command)
	}

	return nil
}

// SetOptions sets runner options
func (pr *PnpmRunner) SetOptions(options *PnpmOptions) {
	pr.options = options
}

// GetOptions returns current runner options
func (pr *PnpmRunner) GetOptions() *PnpmOptions {
	return pr.options
}

// Private methods

// isPnpmAvailable checks if pnpm is available on the system
func (pr *PnpmRunner) isPnpmAvailable() bool {
	_, err := exec.LookPath(pr.options.PnpmCommand)
	return err == nil
}

// findPackageJson finds the appropriate package.json for the task
func (pr *PnpmRunner) findPackageJson(task *t.Task) string {
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
		packageJsonPath := filepath.Join(currentDir, pr.options.PackageJsonFile)
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
func (pr *PnpmRunner) parsePackageJson(packageJsonPath string) (*PackageJson, error) {
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
func (pr *PnpmRunner) validateScript(packageJson *PackageJson, scriptName string) error {
	if packageJson.Scripts == nil {
		return fmt.Errorf("no scripts defined in package.json")
	}

	if _, exists := packageJson.Scripts[scriptName]; !exists {
		return fmt.Errorf("script '%s' not found in package.json", scriptName)
	}

	return nil
}

// checkLockfile checks if pnpm-lock.yaml exists
func (pr *PnpmRunner) checkLockfile(packageJsonPath string) error {
	packageDir := filepath.Dir(packageJsonPath)
	lockfilePath := filepath.Join(packageDir, "pnpm-lock.yaml")

	if _, err := os.Stat(lockfilePath); os.IsNotExist(err) {
		return fmt.Errorf("no pnpm-lock.yaml found, run 'pnpm install' first")
	}

	return nil
}

// detectWorkspaces detects workspace configuration
func (pr *PnpmRunner) detectWorkspaces(packageJsonPath string) error {
	packageDir := filepath.Dir(packageJsonPath)

	// Check for pnpm-workspace.yaml
	workspaceFile := filepath.Join(packageDir, "pnpm-workspace.yaml")
	if _, err := os.Stat(workspaceFile); err == nil {
		pr.options.Workspaces = true
		pr.options.WorkspaceRoot = packageDir
		pr.logger.Debug("Detected pnpm workspaces", "root", pr.options.WorkspaceRoot)
		return nil
	}

	// Check for workspaces in package.json
	packageJson, err := pr.parsePackageJson(packageJsonPath)
	if err != nil {
		return err
	}

	if packageJson.Workspaces != nil {
		pr.options.Workspaces = true
		pr.options.WorkspaceRoot = packageDir
		pr.logger.Debug("Detected workspaces in package.json", "root", pr.options.WorkspaceRoot)
	}

	return nil
}

// verifyStore verifies the pnpm store integrity
func (pr *PnpmRunner) verifyStore(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, pr.options.PnpmCommand, "store", "status")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("store verification failed: %w", err)
	}
	return nil
}

// buildCommand builds the pnpm command
func (pr *PnpmRunner) buildCommand(task *t.Task) (*exec.Cmd, error) {
	args := []string{}

	// Add default pnpm arguments
	args = append(args, pr.options.PnpmArgs...)

	// Determine command type
	if pr.isLifecycleScript(task.Name) {
		args = append(args, "run", task.Name)
	} else {
		// Built-in pnpm commands
		args = append(args, task.Name)
	}

	// Add options based on configuration
	if pr.options.Silent {
		args = append(args, "--silent")
	}

	if pr.options.Verbose {
		args = append(args, "--verbose")
	}

	if pr.options.Production {
		args = append(args, "--prod")
	}

	if pr.options.Development {
		args = append(args, "--dev")
	}

	if pr.options.FrozenLockfile {
		args = append(args, "--frozen-lockfile")
	}

	if pr.options.PreferFrozenLockfile {
		args = append(args, "--prefer-frozen-lockfile")
	}

	if pr.options.IgnoreOptional {
		args = append(args, "--ignore-optional")
	}

	if pr.options.Registry != "" {
		args = append(args, "--registry", pr.options.Registry)
	}

	if pr.options.StoreDir != "" {
		args = append(args, "--store-dir", pr.options.StoreDir)
	}

	if pr.options.CacheDir != "" {
		args = append(args, "--cache-dir", pr.options.CacheDir)
	}

	if pr.options.UseStoreServer {
		args = append(args, "--use-store-server")
	}

	if pr.options.JsonOutput {
		args = append(args, "--json")
	}

	if pr.options.NoProgress {
		args = append(args, "--no-progress")
	}

	if pr.options.ReporterType != "" && pr.options.ReporterType != "default" {
		args = append(args, "--reporter", pr.options.ReporterType)
	}

	if pr.options.IgnoreScripts {
		args = append(args, "--ignore-scripts")
	}

	if pr.options.CheckFiles {
		args = append(args, "--check-files")
	}

	if pr.options.NetworkConcurrency > 0 {
		args = append(args, "--network-concurrency", fmt.Sprintf("%d", pr.options.NetworkConcurrency))
	}

	if pr.options.ChildConcurrency > 0 {
		args = append(args, "--child-concurrency", fmt.Sprintf("%d", pr.options.ChildConcurrency))
	}

	if !pr.options.SideEffectsCache {
		args = append(args, "--no-side-effects-cache")
	}

	// Add workspace options
	if pr.options.Workspaces {
		if pr.options.Recursive {
			args = append(args, "--recursive")
		}

		if pr.options.Filter != "" {
			args = append(args, "--filter", pr.options.Filter)
		}

		if pr.options.WorkspacePattern != "" {
			args = append(args, "--workspace-pattern", pr.options.WorkspacePattern)
		}
	}

	// Add task arguments if any
	if len(task.Args) > 1 { // Skip first arg which is usually the script name
		// Add separator for script arguments
		if pr.isLifecycleScript(task.Name) {
			args = append(args, "--")
		}
		args = append(args, task.Args[1:]...)
	}

	cmd := exec.Command(pr.options.PnpmCommand, args...)
	return cmd, nil
}

// isLifecycleScript checks if the task is a custom script vs built-in pnpm command
func (pr *PnpmRunner) isLifecycleScript(taskName string) bool {
	builtinCommands := []string{
		"install", "i", "add", "remove", "rm", "uninstall", "un",
		"update", "up", "upgrade", "outdated", "list", "ls", "ll", "la",
		"link", "unlink", "import", "rebuild", "prune", "publish",
		"pack", "root", "bin", "why", "audit", "fund", "config",
		"store", "server", "recursive", "exec", "dlx", "create",
		"env", "info", "init", "patch", "patch-commit", "patch-remove",
		"setup", "deploy", "test", "run", "start", "restart", "stop",
	}

	for _, cmd := range builtinCommands {
		if taskName == cmd {
			return false
		}
	}

	return true
}

// getWorkingDirectory returns the working directory for the task
func (pr *PnpmRunner) getWorkingDirectory(task *t.Task) string {
	if task.WorkingDirectory != "" {
		return task.WorkingDirectory
	}

	if packageJsonPath := pr.findPackageJson(task); packageJsonPath != "" {
		return filepath.Dir(packageJsonPath)
	}

	return "."
}

// buildEnvironment builds the environment variables for the command
func (pr *PnpmRunner) buildEnvironment(task *t.Task) []string {
	var env []string

	// Start with current environment if enabled
	if pr.options.PassEnvironment {
		env = os.Environ()
	}

	// Add runner-specific environment variables
	runnerEnv := pr.GetEnvironmentVariables(task)
	for key, value := range runnerEnv {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}

	return env
}

// executeWithOutput executes the command with output streaming
func (pr *PnpmRunner) executeWithOutput(ctx context.Context, cmd *exec.Cmd, outputBuffer *output.Buffer) error {
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
	go pr.streamOutput(stdout, outputBuffer, false)
	go pr.streamOutput(stderr, outputBuffer, true)

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
			return fmt.Errorf("pnpm command failed: %w", err)
		}
		return nil
	}
}

// streamOutput streams command output to the buffer
func (pr *PnpmRunner) streamOutput(pipe any, outputBuffer *output.Buffer, isStderr bool) {
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
		enhancedLine := pr.parseOutputLine(line, isStderr)

		// Write to buffer
		if isStderr {
			outputBuffer.WriteStderr([]byte(enhancedLine + "\n"))
		} else {
			outputBuffer.WriteStdout([]byte(enhancedLine + "\n"))
		}
	}
}

// parseOutputLine parses and enhances a pnpm output line
func (pr *PnpmRunner) parseOutputLine(line string, isStderr bool) string {
	// Add timestamps and formatting
	timestamp := time.Now().Format("15:04:05.000")
	prefix := "OUT"
	if isStderr {
		prefix = "ERR"
	}

	// Check for special pnpm patterns
	if pr.patterns.ErrorPattern.MatchString(line) {
		prefix = "ERROR"
	} else if pr.patterns.WarningPattern.MatchString(line) {
		prefix = "WARN"
	} else if pr.patterns.SuccessPattern.MatchString(line) {
		prefix = "SUCCESS"
	} else if pr.patterns.ProgressPattern.MatchString(line) {
		prefix = "PROGRESS"
	} else if pr.patterns.ResolutionPattern.MatchString(line) {
		prefix = "RESOLVE"
	} else if pr.patterns.FetchingPattern.MatchString(line) {
		prefix = "FETCH"
	} else if pr.patterns.LinkingPattern.MatchString(line) {
		prefix = "LINK"
	} else if pr.patterns.BuildingPattern.MatchString(line) {
		prefix = "BUILD"
	} else if pr.patterns.WorkspacePattern.MatchString(line) {
		prefix = "WORKSPACE"
	} else if pr.patterns.TimingPattern.MatchString(line) {
		prefix = "TIMING"
	} else if pr.patterns.AuditPattern.MatchString(line) {
		prefix = "AUDIT"
	} else if pr.patterns.StorePattern.MatchString(line) {
		prefix = "STORE"
	}

	return fmt.Sprintf("[%s] [%s] %s", timestamp, prefix, line)
}

// GetPnpmVersion returns the version of pnpm
func (pr *PnpmRunner) GetPnpmVersion() (string, error) {
	cmd := exec.Command(pr.options.PnpmCommand, "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get pnpm version: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// GetStoreStatus returns pnpm store status
func (pr *PnpmRunner) GetStoreStatus() (map[string]any, error) {
	cmd := exec.Command(pr.options.PnpmCommand, "store", "status", "--json")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get store status: %w", err)
	}

	var status map[string]any
	if err := json.Unmarshal(output, &status); err != nil {
		return nil, fmt.Errorf("failed to parse store status: %w", err)
	}

	return status, nil
}

// ListScripts lists all available scripts in package.json
func (pr *PnpmRunner) ListScripts(packageJsonPath string) (map[string]string, error) {
	packageJson, err := pr.parsePackageJson(packageJsonPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse package.json: %w", err)
	}

	if packageJson.Scripts == nil {
		return make(map[string]string), nil
	}

	return packageJson.Scripts, nil
}

// GetWorkspaceProjects returns list of workspace projects
func (pr *PnpmRunner) GetWorkspaceProjects(packageJsonPath string) ([]string, error) {
	packageDir := filepath.Dir(packageJsonPath)

	cmd := exec.Command(pr.options.PnpmCommand, "list", "--json", "--depth", "0")
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

// GetTaskMetadata returns metadata specific to pnpm tasks
func (pr *PnpmRunner) GetTaskMetadata(task *t.Task) map[string]any {
	metadata := map[string]any{
		"runner_type":    pr.Type(),
		"runner_name":    pr.Name(),
		"runner_version": pr.Version(),
		"pnpm_command":   pr.options.PnpmCommand,
	}

	if packageJsonPath := pr.findPackageJson(task); packageJsonPath != "" {
		metadata["package_json_path"] = packageJsonPath

		// Add package.json info
		if packageJson, err := pr.parsePackageJson(packageJsonPath); err == nil {
			metadata["package_name"] = packageJson.Name
			metadata["package_version"] = packageJson.Version
			metadata["package_description"] = packageJson.Description

			if scripts, err := pr.ListScripts(packageJsonPath); err == nil {
				metadata["available_scripts"] = scripts
			}

			// Add dependency counts
			metadata["dependencies_count"] = len(packageJson.Dependencies)
			metadata["dev_dependencies_count"] = len(packageJson.DevDependencies)

			// Add workspace info
			if pr.options.Workspaces {
				metadata["workspaces_enabled"] = true
				metadata["workspace_root"] = pr.options.WorkspaceRoot

				if projects, err := pr.GetWorkspaceProjects(packageJsonPath); err == nil {
					metadata["workspace_projects"] = projects
					metadata["workspace_count"] = len(projects)
				}
			}
		}
	}

	// Add pnpm version
	if version, err := pr.GetPnpmVersion(); err == nil {
		metadata["pnpm_version"] = version
	}

	// Add store information
	if storeStatus, err := pr.GetStoreStatus(); err == nil {
		metadata["store_status"] = storeStatus
	}

	return metadata
}
