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
	"slices"
)

// YarnRunner implements the Runner interface for Yarn tasks
type YarnRunner struct {
	logger   logger.Logger
	options  *YarnOptions
	patterns *YarnPatterns
}

// YarnOptions contains configuration for the Yarn runner
type YarnOptions struct {
	// Yarn executable
	YarnCommand string
	YarnArgs    []string

	// Package.json handling
	PackageJsonFile string
	WorkspaceRoot   string

	// Yarn version detection
	YarnVersion  string
	UseYarn2     bool
	UseYarnBerry bool

	// Execution options
	Silent         bool
	Verbose        bool
	Production     bool
	Development    bool
	FrozenLockfile bool
	IgnoreOptional bool

	// Workspace options
	Workspaces       bool
	WorkspacePattern string

	// Cache and performance
	UseCache       bool
	CacheFolder    string
	OfflineMode    bool
	PreferOffline  bool
	NetworkTimeout int

	// Registry and authentication
	Registry  string
	AuthToken string
	Scope     string

	// Output handling
	JsonOutput bool
	NoProgress bool

	// Security
	IgnoreScripts bool
	CheckFiles    bool

	// Environment
	NodeEnv         string
	PassEnvironment bool
}

// YarnPatterns contains regex patterns for parsing Yarn output
type YarnPatterns struct {
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
}

// YarnWorkspace represents a Yarn workspace
type YarnWorkspace struct {
	Name       string   `json:"name"`
	Location   string   `json:"location"`
	Workspaces []string `json:"workspaces,omitempty"`
}

// YarnInfo represents Yarn workspace info
type YarnInfo struct {
	Workspaces map[string]*YarnWorkspace `json:"workspaces"`
}

// NewYarnRunner creates a new Yarn runner
func NewYarnRunner(logger logger.Logger) *YarnRunner {
	runner := &YarnRunner{
		logger:   logger.WithGroup("yarn-runner"),
		options:  createDefaultYarnOptions(),
		patterns: createYarnPatterns(),
	}

	// Detect Yarn version
	runner.detectYarnVersion()

	return runner
}

// createDefaultYarnOptions creates default Yarn options
func createDefaultYarnOptions() *YarnOptions {
	return &YarnOptions{
		YarnCommand:      "yarn",
		YarnArgs:         []string{},
		PackageJsonFile:  "package.json",
		WorkspaceRoot:    "",
		YarnVersion:      "",
		UseYarn2:         false,
		UseYarnBerry:     false,
		Silent:           false,
		Verbose:          false,
		Production:       false,
		Development:      false,
		FrozenLockfile:   false,
		IgnoreOptional:   false,
		Workspaces:       false,
		WorkspacePattern: "",
		UseCache:         true,
		CacheFolder:      "",
		OfflineMode:      false,
		PreferOffline:    false,
		NetworkTimeout:   30000,
		Registry:         "",
		AuthToken:        "",
		Scope:            "",
		JsonOutput:       false,
		NoProgress:       false,
		IgnoreScripts:    false,
		CheckFiles:       false,
		NodeEnv:          "",
		PassEnvironment:  true,
	}
}

// createYarnPatterns creates regex patterns for Yarn output parsing
func createYarnPatterns() *YarnPatterns {
	return &YarnPatterns{
		ErrorPattern:      regexp.MustCompile(`^error\s+(.*)$`),
		WarningPattern:    regexp.MustCompile(`^warning\s+(.*)$`),
		SuccessPattern:    regexp.MustCompile(`^success\s+(.*)$`),
		ProgressPattern:   regexp.MustCompile(`^\[\d+/\d+\]\s+(.*)$`),
		ResolutionPattern: regexp.MustCompile(`^info\s+Resolving\s+(.*)$`),
		FetchingPattern:   regexp.MustCompile(`^info\s+Fetching\s+(.*)$`),
		LinkingPattern:    regexp.MustCompile(`^info\s+Linking\s+(.*)$`),
		BuildingPattern:   regexp.MustCompile(`^info\s+Building\s+(.*)$`),
		WorkspacePattern:  regexp.MustCompile(`^info\s+(.*)@workspace:(.*)$`),
		ScriptPattern:     regexp.MustCompile(`^\$\s+(.*)$`),
		TimingPattern:     regexp.MustCompile(`^Done in (.*)s\.$`),
		AuditPattern:      regexp.MustCompile(`^(\d+)\s+vulnerabilit(?:y|ies)\s+found`),
	}
}

// Type returns the runner type
func (yr *YarnRunner) Type() t.RunnerType {
	return t.RunnerYarn
}

// Name returns the runner name
func (yr *YarnRunner) Name() string {
	return "Yarn Runner"
}

// Version returns the runner version
func (yr *YarnRunner) Version() string {
	return "1.0.0"
}

// CanRun checks if the runner can execute the given task
func (yr *YarnRunner) CanRun(task *t.Task) bool {
	if task.Runner != t.RunnerYarn {
		return false
	}

	// Check if Yarn is available
	if !yr.isYarnAvailable() {
		yr.logger.Warn("Yarn is not available")
		return false
	}

	// Check if package.json exists
	packageJsonPath := yr.findPackageJson(task)
	if packageJsonPath == "" {
		yr.logger.Warn("No package.json found", "task", task.Name)
		return false
	}

	return true
}

// Prepare prepares the task for execution
func (yr *YarnRunner) Prepare(ctx context.Context, task *t.Task) error {
	yr.logger.Debug("Preparing Yarn task", "task", task.Name)

	// Validate Yarn availability
	if !yr.isYarnAvailable() {
		return fmt.Errorf("yarn command not found")
	}

	// Find and validate package.json
	packageJsonPath := yr.findPackageJson(task)
	if packageJsonPath == "" {
		return fmt.Errorf("no package.json found for task %s", task.Name)
	}

	// Parse package.json
	packageJson, err := yr.parsePackageJson(packageJsonPath)
	if err != nil {
		return fmt.Errorf("failed to parse package.json: %w", err)
	}

	// Validate script exists
	if err := yr.validateScript(packageJson, task.Name); err != nil {
		return fmt.Errorf("script validation failed: %w", err)
	}

	// Check for yarn.lock if not a global command
	if err := yr.checkLockfile(packageJsonPath); err != nil {
		yr.logger.Warn("Lockfile check failed", "error", err)
	}

	// Detect workspace configuration
	if err := yr.detectWorkspaces(packageJsonPath); err != nil {
		yr.logger.Warn("Workspace detection failed", "error", err)
	}

	return nil
}

// Execute executes the Yarn task
func (yr *YarnRunner) Execute(ctx context.Context, task *t.Task, outputBuffer *output.Buffer) error {
	yr.logger.Info("Executing Yarn task", "task", task.Name)

	// Build command
	cmd, err := yr.buildCommand(task)
	if err != nil {
		return fmt.Errorf("failed to build command: %w", err)
	}

	// Set up command context
	cmd.Dir = yr.getWorkingDirectory(task)
	cmd.Env = yr.buildEnvironment(task)

	yr.logger.Debug("Yarn command built",
		"cmd", cmd.Path,
		"args", cmd.Args,
		"dir", cmd.Dir,
		"yarn_version", yr.options.YarnVersion)

	// Execute with output streaming
	return yr.executeWithOutput(ctx, cmd, outputBuffer)
}

// Stop stops the running Yarn task
func (yr *YarnRunner) Stop(ctx context.Context, task *t.Task) error {
	yr.logger.Info("Stopping Yarn task", "task", task.Name)

	// Yarn doesn't have a built-in graceful stop mechanism for scripts
	// We rely on the process being killed by the executor
	return nil
}

// GetDefaultTimeout returns the default timeout for Yarn tasks
func (yr *YarnRunner) GetDefaultTimeout() time.Duration {
	return 15 * time.Minute
}

// GetEnvironmentVariables returns environment variables for the task
func (yr *YarnRunner) GetEnvironmentVariables(task *t.Task) map[string]string {
	env := make(map[string]string)

	// Add Yarn-specific variables
	env["npm_execpath"] = yr.options.YarnCommand
	env["npm_config_user_agent"] = fmt.Sprintf("yarn/%s", yr.options.YarnVersion)

	if yr.options.NodeEnv != "" {
		env["NODE_ENV"] = yr.options.NodeEnv
	}

	if task.FilePath != "" {
		env["npm_package_json"] = task.FilePath
		env["INIT_CWD"] = filepath.Dir(task.FilePath)
	}

	// Add registry if specified
	if yr.options.Registry != "" {
		env["npm_config_registry"] = yr.options.Registry
	}

	// Add cache folder if specified
	if yr.options.CacheFolder != "" {
		env["YARN_CACHE_FOLDER"] = yr.options.CacheFolder
	}

	// Add Yarn version info
	env["YARN_VERSION"] = yr.options.YarnVersion
	if yr.options.UseYarn2 || yr.options.UseYarnBerry {
		env["YARN_ENABLE_IMMUTABLE_INSTALLS"] = "false"
	}

	// Add package.json variables
	if packageJsonPath := yr.findPackageJson(task); packageJsonPath != "" {
		if packageJson, err := yr.parsePackageJson(packageJsonPath); err == nil {
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
func (yr *YarnRunner) Validate(task *t.Task) error {
	if task.Runner != t.RunnerYarn {
		return fmt.Errorf("invalid runner type: expected %s, got %s", t.RunnerYarn, task.Runner)
	}

	if task.Name == "" {
		return fmt.Errorf("task name cannot be empty")
	}

	// Additional validation
	if task.Command != "" && task.Command != "yarn" {
		yr.logger.Warn("Task has custom command, but Yarn runner expects 'yarn'", "command", task.Command)
	}

	return nil
}

// SetOptions sets runner options
func (yr *YarnRunner) SetOptions(options *YarnOptions) {
	yr.options = options
}

// GetOptions returns current runner options
func (yr *YarnRunner) GetOptions() *YarnOptions {
	return yr.options
}

// Private methods

// isYarnAvailable checks if Yarn is available on the system
func (yr *YarnRunner) isYarnAvailable() bool {
	_, err := exec.LookPath(yr.options.YarnCommand)
	return err == nil
}

// detectYarnVersion detects the Yarn version
func (yr *YarnRunner) detectYarnVersion() {
	if version, err := yr.GetYarnVersion(); err == nil {
		yr.options.YarnVersion = version

		// Detect Yarn 2/Berry
		if strings.HasPrefix(version, "2.") || strings.HasPrefix(version, "3.") || strings.HasPrefix(version, "4.") {
			yr.options.UseYarn2 = true
			yr.options.UseYarnBerry = true
		}

		yr.logger.Debug("Detected Yarn version", "version", version, "yarn2", yr.options.UseYarn2)
	}
}

// findPackageJson finds the appropriate package.json for the task
func (yr *YarnRunner) findPackageJson(task *t.Task) string {
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
		packageJsonPath := filepath.Join(currentDir, yr.options.PackageJsonFile)
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
func (yr *YarnRunner) parsePackageJson(packageJsonPath string) (*PackageJson, error) {
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
func (yr *YarnRunner) validateScript(packageJson *PackageJson, scriptName string) error {
	if packageJson.Scripts == nil {
		return fmt.Errorf("no scripts defined in package.json")
	}

	if _, exists := packageJson.Scripts[scriptName]; !exists {
		return fmt.Errorf("script '%s' not found in package.json", scriptName)
	}

	return nil
}

// checkLockfile checks if yarn.lock exists
func (yr *YarnRunner) checkLockfile(packageJsonPath string) error {
	packageDir := filepath.Dir(packageJsonPath)

	// Check for different lockfile names based on Yarn version
	lockfiles := []string{"yarn.lock"}
	if yr.options.UseYarn2 {
		lockfiles = append(lockfiles, "yarn-lock.json")
	}

	for _, lockfile := range lockfiles {
		lockfilePath := filepath.Join(packageDir, lockfile)
		if _, err := os.Stat(lockfilePath); err == nil {
			return nil // Found lockfile
		}
	}

	return fmt.Errorf("no yarn.lock found, run 'yarn install' first")
}

// detectWorkspaces detects workspace configuration
func (yr *YarnRunner) detectWorkspaces(packageJsonPath string) error {
	packageJson, err := yr.parsePackageJson(packageJsonPath)
	if err != nil {
		return err
	}

	// Check if workspaces are defined
	if packageJson.Workspaces != nil {
		yr.options.Workspaces = true
		yr.options.WorkspaceRoot = filepath.Dir(packageJsonPath)
		yr.logger.Debug("Detected Yarn workspaces", "root", yr.options.WorkspaceRoot)
	}

	return nil
}

// buildCommand builds the Yarn command
func (yr *YarnRunner) buildCommand(task *t.Task) (*exec.Cmd, error) {
	args := []string{}

	// Add default Yarn arguments
	args = append(args, yr.options.YarnArgs...)

	// Determine command type
	if yr.isLifecycleScript(task.Name) {
		if yr.options.UseYarn2 {
			// Yarn 2+ syntax
			args = append(args, "run", task.Name)
		} else {
			// Yarn 1.x can run scripts directly
			args = append(args, task.Name)
		}
	} else {
		// Built-in Yarn commands
		args = append(args, task.Name)
	}

	// Add options based on configuration
	if yr.options.Silent {
		args = append(args, "--silent")
	}

	if yr.options.Verbose {
		args = append(args, "--verbose")
	}

	if yr.options.Production {
		args = append(args, "--production")
	}

	if yr.options.Development {
		if yr.options.UseYarn2 {
			args = append(args, "--include-dev")
		} else {
			args = append(args, "--dev")
		}
	}

	if yr.options.FrozenLockfile {
		if yr.options.UseYarn2 {
			args = append(args, "--immutable")
		} else {
			args = append(args, "--frozen-lockfile")
		}
	}

	if yr.options.IgnoreOptional {
		args = append(args, "--ignore-optional")
	}

	if yr.options.Registry != "" {
		args = append(args, "--registry", yr.options.Registry)
	}

	if yr.options.CacheFolder != "" {
		args = append(args, "--cache-folder", yr.options.CacheFolder)
	}

	if yr.options.OfflineMode {
		args = append(args, "--offline")
	} else if yr.options.PreferOffline {
		args = append(args, "--prefer-offline")
	}

	if yr.options.JsonOutput {
		args = append(args, "--json")
	}

	if yr.options.NoProgress {
		args = append(args, "--no-progress")
	}

	if yr.options.IgnoreScripts {
		args = append(args, "--ignore-scripts")
	}

	if yr.options.CheckFiles {
		args = append(args, "--check-files")
	}

	if yr.options.NetworkTimeout > 0 {
		args = append(args, "--network-timeout", fmt.Sprintf("%d", yr.options.NetworkTimeout))
	}

	// Add workspace options
	if yr.options.Workspaces && yr.options.WorkspacePattern != "" {
		if yr.options.UseYarn2 {
			args = append(args, "--workspace", yr.options.WorkspacePattern)
		} else {
			args = append(args, "-W") // Run in workspace root
		}
	}

	// Add task arguments if any
	if len(task.Args) > 1 { // Skip first arg which is usually the script name
		// Add separator for script arguments
		if yr.isLifecycleScript(task.Name) {
			args = append(args, "--")
		}
		args = append(args, task.Args[1:]...)
	}

	cmd := exec.Command(yr.options.YarnCommand, args...)
	return cmd, nil
}

// isLifecycleScript checks if the task is a custom script vs built-in Yarn command
func (yr *YarnRunner) isLifecycleScript(taskName string) bool {
	builtinCommands := []string{
		"install", "add", "remove", "upgrade", "upgrade-interactive",
		"info", "list", "outdated", "audit", "autoclean", "cache",
		"check", "import", "init", "install", "licenses", "link",
		"login", "logout", "outdated", "owner", "pack", "publish",
		"run", "tag", "team", "unlink", "version", "versions",
		"why", "workspace", "workspaces", "config", "create",
		"exec", "dlx", "constraints", "stage", "commit",
	}

	// Yarn 2+ specific commands
	if yr.options.UseYarn2 {
		builtinCommands = append(builtinCommands,
			"berry", "policies", "plugin", "rebuild", "unplug",
			"up", "set", "node", "npm", "patch", "patch-commit",
		)
	}

	return !slices.Contains(builtinCommands, taskName)
}

// getWorkingDirectory returns the working directory for the task
func (yr *YarnRunner) getWorkingDirectory(task *t.Task) string {
	if task.WorkingDirectory != "" {
		return task.WorkingDirectory
	}

	if packageJsonPath := yr.findPackageJson(task); packageJsonPath != "" {
		return filepath.Dir(packageJsonPath)
	}

	return "."
}

// buildEnvironment builds the environment variables for the command
func (yr *YarnRunner) buildEnvironment(task *t.Task) []string {
	var env []string

	// Start with current environment if enabled
	if yr.options.PassEnvironment {
		env = os.Environ()
	}

	// Add runner-specific environment variables
	runnerEnv := yr.GetEnvironmentVariables(task)
	for key, value := range runnerEnv {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}

	return env
}

// executeWithOutput executes the command with output streaming
func (yr *YarnRunner) executeWithOutput(ctx context.Context, cmd *exec.Cmd, outputBuffer *output.Buffer) error {
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
	go yr.streamOutput(stdout, outputBuffer, false)
	go yr.streamOutput(stderr, outputBuffer, true)

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
			return fmt.Errorf("yarn command failed: %w", err)
		}
		return nil
	}
}

// streamOutput streams command output to the buffer
func (yr *YarnRunner) streamOutput(pipe any, outputBuffer *output.Buffer, isStderr bool) {
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
		enhancedLine := yr.parseOutputLine(line, isStderr)

		// Write to buffer
		if isStderr {
			outputBuffer.WriteStderr([]byte(enhancedLine + "\n"))
		} else {
			outputBuffer.WriteStdout([]byte(enhancedLine + "\n"))
		}
	}
}

// parseOutputLine parses and enhances a Yarn output line
func (yr *YarnRunner) parseOutputLine(line string, isStderr bool) string {
	// Add timestamps and formatting
	timestamp := time.Now().Format("15:04:05.000")
	prefix := "OUT"
	if isStderr {
		prefix = "ERR"
	}

	// Check for special Yarn patterns
	if yr.patterns.ErrorPattern.MatchString(line) {
		prefix = "ERROR"
	} else if yr.patterns.WarningPattern.MatchString(line) {
		prefix = "WARN"
	} else if yr.patterns.SuccessPattern.MatchString(line) {
		prefix = "SUCCESS"
	} else if yr.patterns.ProgressPattern.MatchString(line) {
		prefix = "PROGRESS"
	} else if yr.patterns.ResolutionPattern.MatchString(line) {
		prefix = "RESOLVE"
	} else if yr.patterns.FetchingPattern.MatchString(line) {
		prefix = "FETCH"
	} else if yr.patterns.LinkingPattern.MatchString(line) {
		prefix = "LINK"
	} else if yr.patterns.BuildingPattern.MatchString(line) {
		prefix = "BUILD"
	} else if yr.patterns.WorkspacePattern.MatchString(line) {
		prefix = "WORKSPACE"
	} else if yr.patterns.TimingPattern.MatchString(line) {
		prefix = "TIMING"
	} else if yr.patterns.AuditPattern.MatchString(line) {
		prefix = "AUDIT"
	}

	return fmt.Sprintf("[%s] [%s] %s", timestamp, prefix, line)
}

// GetYarnVersion returns the version of Yarn
func (yr *YarnRunner) GetYarnVersion() (string, error) {
	cmd := exec.Command(yr.options.YarnCommand, "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get Yarn version: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// GetWorkspaceInfo returns information about workspaces
func (yr *YarnRunner) GetWorkspaceInfo(packageJsonPath string) (*YarnInfo, error) {
	packageDir := filepath.Dir(packageJsonPath)

	var cmd *exec.Cmd
	if yr.options.UseYarn2 {
		cmd = exec.Command(yr.options.YarnCommand, "workspaces", "list", "--json")
	} else {
		cmd = exec.Command(yr.options.YarnCommand, "workspaces", "info", "--json")
	}

	cmd.Dir = packageDir
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get workspace info: %w", err)
	}

	var info YarnInfo
	if err := json.Unmarshal(output, &info); err != nil {
		return nil, fmt.Errorf("failed to parse workspace info: %w", err)
	}

	return &info, nil
}

// ListScripts lists all available scripts in package.json
func (yr *YarnRunner) ListScripts(packageJsonPath string) (map[string]string, error) {
	packageJson, err := yr.parsePackageJson(packageJsonPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse package.json: %w", err)
	}

	if packageJson.Scripts == nil {
		return make(map[string]string), nil
	}

	return packageJson.Scripts, nil
}

// GetTaskMetadata returns metadata specific to Yarn tasks
func (yr *YarnRunner) GetTaskMetadata(task *t.Task) map[string]any {
	metadata := map[string]any{
		"runner_type":    yr.Type(),
		"runner_name":    yr.Name(),
		"runner_version": yr.Version(),
		"yarn_command":   yr.options.YarnCommand,
		"yarn_version":   yr.options.YarnVersion,
		"yarn2":          yr.options.UseYarn2,
		"yarn_berry":     yr.options.UseYarnBerry,
	}

	if packageJsonPath := yr.findPackageJson(task); packageJsonPath != "" {
		metadata["package_json_path"] = packageJsonPath

		// Add package.json info
		if packageJson, err := yr.parsePackageJson(packageJsonPath); err == nil {
			metadata["package_name"] = packageJson.Name
			metadata["package_version"] = packageJson.Version
			metadata["package_description"] = packageJson.Description

			if scripts, err := yr.ListScripts(packageJsonPath); err == nil {
				metadata["available_scripts"] = scripts
			}

			// Add dependency counts
			metadata["dependencies_count"] = len(packageJson.Dependencies)
			metadata["dev_dependencies_count"] = len(packageJson.DevDependencies)

			// Add workspace info
			if yr.options.Workspaces {
				metadata["workspaces_enabled"] = true
				metadata["workspace_root"] = yr.options.WorkspaceRoot

				if workspaceInfo, err := yr.GetWorkspaceInfo(packageJsonPath); err == nil {
					metadata["workspace_count"] = len(workspaceInfo.Workspaces)
				}
			}
		}
	}

	return metadata
}
