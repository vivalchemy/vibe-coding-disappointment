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

// PythonRunner implements the Runner interface for Python tasks
type PythonRunner struct {
	logger   logger.Logger
	options  *PythonOptions
	patterns *PythonPatterns
}

// PythonOptions contains configuration for the Python runner
type PythonOptions struct {
	// Python executable
	PythonCommand string
	PythonVersion string
	PythonArgs    []string

	// Virtual environment
	VirtualEnv       string
	VirtualEnvPath   string
	AutoActivateVenv bool
	CreateVenv       bool

	// Execution options
	Interactive bool
	Unbuffered  bool
	Optimize    int
	Verbose     bool
	Quiet       bool

	// Module execution
	ModuleMode  bool
	PackageMode bool

	// Path and imports
	PythonPath     []string
	SysPath        []string
	NoSitePackages bool
	NoUserSite     bool

	// Script execution
	ScriptArgs       []string
	WorkingDirectory string

	// Environment
	PythonEnv       map[string]string
	PassEnvironment bool

	// Framework detection
	DetectFrameworks    bool
	SupportedFrameworks []string

	// Performance
	EnableProfiling bool
	ProfileOutput   string
	MemoryProfiling bool
}

// PythonPatterns contains regex patterns for parsing Python output
type PythonPatterns struct {
	ErrorPattern       *regexp.Regexp
	WarningPattern     *regexp.Regexp
	ImportErrorPattern *regexp.Regexp
	SyntaxErrorPattern *regexp.Regexp
	TracebackPattern   *regexp.Regexp
	FileLinePattern    *regexp.Regexp
	DeprecationPattern *regexp.Regexp
	UnitTestPattern    *regexp.Regexp
	DocTestPattern     *regexp.Regexp
	FlaskPattern       *regexp.Regexp
	DjangoPattern      *regexp.Regexp
}

// PythonEnvironment represents Python execution environment information
type PythonEnvironment struct {
	PythonVersion     string
	PythonExecutable  string
	VirtualEnv        string
	SitePackages      []string
	InstalledPackages map[string]string
	PythonPath        []string
}

// PythonFramework represents detected Python framework information
type PythonFramework struct {
	Name         string
	Version      string
	ConfigFile   string
	EntryPoint   string
	Dependencies []string
}

// NewPythonRunner creates a new Python runner
func NewPythonRunner(logger logger.Logger) *PythonRunner {
	runner := &PythonRunner{
		logger:   logger.WithGroup("python-runner"),
		options:  createDefaultPythonOptions(),
		patterns: createPythonPatterns(),
	}

	// Detect Python environment
	runner.detectPythonEnvironment()

	return runner
}

// createDefaultPythonOptions creates default Python options
func createDefaultPythonOptions() *PythonOptions {
	return &PythonOptions{
		PythonCommand:       "python",
		PythonVersion:       "",
		PythonArgs:          []string{},
		VirtualEnv:          "",
		VirtualEnvPath:      "",
		AutoActivateVenv:    true,
		CreateVenv:          false,
		Interactive:         false,
		Unbuffered:          true,
		Optimize:            0,
		Verbose:             false,
		Quiet:               false,
		ModuleMode:          false,
		PackageMode:         false,
		PythonPath:          []string{},
		SysPath:             []string{},
		NoSitePackages:      false,
		NoUserSite:          false,
		ScriptArgs:          []string{},
		WorkingDirectory:    "",
		PythonEnv:           make(map[string]string),
		PassEnvironment:     true,
		DetectFrameworks:    true,
		SupportedFrameworks: []string{"flask", "django", "fastapi", "pytest", "unittest"},
		EnableProfiling:     false,
		ProfileOutput:       "",
		MemoryProfiling:     false,
	}
}

// createPythonPatterns creates regex patterns for Python output parsing
func createPythonPatterns() *PythonPatterns {
	return &PythonPatterns{
		ErrorPattern:       regexp.MustCompile(`^.*Error:\s+(.*)$`),
		WarningPattern:     regexp.MustCompile(`^.*Warning:\s+(.*)$`),
		ImportErrorPattern: regexp.MustCompile(`^ImportError:\s+(.*)$`),
		SyntaxErrorPattern: regexp.MustCompile(`^SyntaxError:\s+(.*)$`),
		TracebackPattern:   regexp.MustCompile(`^Traceback\s+\(most recent call last\):$`),
		FileLinePattern:    regexp.MustCompile(`^\s+File\s+"([^"]+)",\s+line\s+(\d+)(?:,\s+in\s+(.+))?$`),
		DeprecationPattern: regexp.MustCompile(`^DeprecationWarning:\s+(.*)$`),
		UnitTestPattern:    regexp.MustCompile(`^(\.+|F+|E+|s+)\s*$`),
		DocTestPattern:     regexp.MustCompile(`^Trying:\s+(.*)$`),
		FlaskPattern:       regexp.MustCompile(`^\s*\*\s+Running\s+on\s+(http://[^\s]+)`),
		DjangoPattern:      regexp.MustCompile(`^Starting development server at (http://[^\s]+)`),
	}
}

// Type returns the runner type
func (pr *PythonRunner) Type() t.RunnerType {
	return t.RunnerPython
}

// Name returns the runner name
func (pr *PythonRunner) Name() string {
	return "Python Runner"
}

// Version returns the runner version
func (pr *PythonRunner) Version() string {
	return "1.0.0"
}

// CanRun checks if the runner can execute the given task
func (pr *PythonRunner) CanRun(task *t.Task) bool {
	if task.Runner != t.RunnerPython {
		return false
	}

	// Check if Python is available
	if !pr.isPythonAvailable() {
		pr.logger.Warn("Python is not available")
		return false
	}

	// Check if the task has a valid Python file or module
	if !pr.isValidPythonTask(task) {
		pr.logger.Warn("Invalid Python task", "task", task.Name)
		return false
	}

	return true
}

// Prepare prepares the task for execution
func (pr *PythonRunner) Prepare(ctx context.Context, task *t.Task) error {
	pr.logger.Debug("Preparing Python task", "task", task.Name)

	// Validate Python availability
	if !pr.isPythonAvailable() {
		return fmt.Errorf("python command not found")
	}

	// Setup virtual environment if needed
	if err := pr.setupVirtualEnvironment(ctx, task); err != nil {
		return fmt.Errorf("failed to setup virtual environment: %w", err)
	}

	// Validate Python file/module exists
	if err := pr.validatePythonTarget(task); err != nil {
		return fmt.Errorf("Python target validation failed: %w", err)
	}

	// Detect frameworks if enabled
	if pr.options.DetectFrameworks {
		if err := pr.detectFrameworks(task); err != nil {
			pr.logger.Warn("Framework detection failed", "error", err)
		}
	}

	// Install dependencies if requirements file exists
	if err := pr.installDependencies(ctx, task); err != nil {
		pr.logger.Warn("Failed to install dependencies", "error", err)
	}

	return nil
}

// Execute executes the Python task
func (pr *PythonRunner) Execute(ctx context.Context, task *t.Task, outputBuffer *output.Buffer) error {
	pr.logger.Info("Executing Python task", "task", task.Name)

	// Build command
	cmd, err := pr.buildCommand(task)
	if err != nil {
		return fmt.Errorf("failed to build command: %w", err)
	}

	// Set up command context
	cmd.Dir = pr.getWorkingDirectory(task)
	cmd.Env = pr.buildEnvironment(task)

	pr.logger.Debug("Python command built",
		"cmd", cmd.Path,
		"args", cmd.Args,
		"dir", cmd.Dir,
		"python_version", pr.options.PythonVersion)

	// Execute with output streaming
	return pr.executeWithOutput(ctx, cmd, outputBuffer)
}

// Stop stops the running Python task
func (pr *PythonRunner) Stop(ctx context.Context, task *t.Task) error {
	pr.logger.Info("Stopping Python task", "task", task.Name)

	// Python scripts can be gracefully stopped with SIGTERM
	// We rely on the process being killed by the executor
	return nil
}

// GetDefaultTimeout returns the default timeout for Python tasks
func (pr *PythonRunner) GetDefaultTimeout() time.Duration {
	return 10 * time.Minute
}

// GetEnvironmentVariables returns environment variables for the task
func (pr *PythonRunner) GetEnvironmentVariables(task *t.Task) map[string]string {
	env := make(map[string]string)

	// Add Python-specific variables
	env["PYTHON"] = pr.options.PythonCommand
	env["PYTHONUNBUFFERED"] = "1" // Always unbuffer Python output

	if pr.options.PythonVersion != "" {
		env["PYTHON_VERSION"] = pr.options.PythonVersion
	}

	// Add virtual environment variables
	if pr.options.VirtualEnvPath != "" {
		env["VIRTUAL_ENV"] = pr.options.VirtualEnvPath
		env["PYTHONHOME"] = "" // Unset PYTHONHOME in venv

		// Add venv bin to PATH
		venvBin := filepath.Join(pr.options.VirtualEnvPath, "bin")
		if _, err := os.Stat(venvBin); err == nil {
			env["PATH"] = venvBin + string(os.PathListSeparator) + os.Getenv("PATH")
		}
	}

	// Add PYTHONPATH
	if len(pr.options.PythonPath) > 0 {
		pythonPath := strings.Join(pr.options.PythonPath, string(os.PathListSeparator))
		if existingPath := os.Getenv("PYTHONPATH"); existingPath != "" {
			pythonPath = pythonPath + string(os.PathListSeparator) + existingPath
		}
		env["PYTHONPATH"] = pythonPath
	}

	// Add task directory to PYTHONPATH
	if task.FilePath != "" {
		taskDir := filepath.Dir(task.FilePath)
		if pythonPath := env["PYTHONPATH"]; pythonPath != "" {
			env["PYTHONPATH"] = taskDir + string(os.PathListSeparator) + pythonPath
		} else {
			env["PYTHONPATH"] = taskDir
		}
	}

	// Add optimization level
	if pr.options.Optimize > 0 {
		env["PYTHONOPTIMIZE"] = strconv.Itoa(pr.options.Optimize)
	}

	// Add site packages control
	if pr.options.NoSitePackages {
		env["PYTHONNOUSERSITE"] = "1"
	}

	if pr.options.NoUserSite {
		env["PYTHONNOUSERSITE"] = "1"
	}

	// Add custom Python environment variables
	for key, value := range pr.options.PythonEnv {
		env[key] = value
	}

	// Add task-specific variables from metadata
	if pythonVars, ok := task.Metadata["python_environment"].(map[string]string); ok {
		for key, value := range pythonVars {
			env[key] = value
		}
	}

	return env
}

// Validate validates the task configuration
func (pr *PythonRunner) Validate(task *t.Task) error {
	if task.Runner != t.RunnerPython {
		return fmt.Errorf("invalid runner type: expected %s, got %s", t.RunnerPython, task.Runner)
	}

	if task.Name == "" {
		return fmt.Errorf("task name cannot be empty")
	}

	// Additional validation
	if task.Command != "" && task.Command != "python" {
		pr.logger.Warn("Task has custom command, but Python runner expects 'python'", "command", task.Command)
	}

	return nil
}

// SetOptions sets runner options
func (pr *PythonRunner) SetOptions(options *PythonOptions) {
	pr.options = options
}

// GetOptions returns current runner options
func (pr *PythonRunner) GetOptions() *PythonOptions {
	return pr.options
}

// Private methods

// isPythonAvailable checks if Python is available on the system
func (pr *PythonRunner) isPythonAvailable() bool {
	_, err := exec.LookPath(pr.options.PythonCommand)
	return err == nil
}

// detectPythonEnvironment detects Python environment information
func (pr *PythonRunner) detectPythonEnvironment() {
	if version, err := pr.GetPythonVersion(); err == nil {
		pr.options.PythonVersion = version
		pr.logger.Debug("Detected Python version", "version", version)
	}

	// Detect virtual environment
	if venv := os.Getenv("VIRTUAL_ENV"); venv != "" {
		pr.options.VirtualEnvPath = venv
		pr.logger.Debug("Detected virtual environment", "path", venv)
	}
}

// isValidPythonTask checks if the task represents a valid Python execution target
func (pr *PythonRunner) isValidPythonTask(task *t.Task) bool {
	// Check if it's a Python file
	if task.FilePath != "" {
		if strings.HasSuffix(task.FilePath, ".py") {
			return true
		}
	}

	// Check if it's a module execution (python -m module)
	if task.Command == "python" && len(task.Args) > 1 && task.Args[0] == "-m" {
		return true
	}

	// Check if it's a function execution based on metadata
	if pythonFunc, ok := task.Metadata["python_function"].(string); ok && pythonFunc != "" {
		return true
	}

	// Check if it's a script execution
	if task.Name == "run-script" {
		return true
	}

	return false
}

// setupVirtualEnvironment sets up Python virtual environment
func (pr *PythonRunner) setupVirtualEnvironment(ctx context.Context, task *t.Task) error {
	if pr.options.VirtualEnv == "" && pr.options.VirtualEnvPath == "" {
		return nil
	}

	var venvPath string

	if pr.options.VirtualEnvPath != "" {
		venvPath = pr.options.VirtualEnvPath
	} else {
		// Look for virtual environment relative to task
		taskDir := pr.getWorkingDirectory(task)
		venvPath = filepath.Join(taskDir, pr.options.VirtualEnv)
	}

	// Check if virtual environment exists
	venvPython := filepath.Join(venvPath, "bin", "python")
	if _, err := os.Stat(venvPython); os.IsNotExist(err) {
		if pr.options.CreateVenv {
			pr.logger.Info("Creating virtual environment", "path", venvPath)
			if err := pr.createVirtualEnvironment(ctx, venvPath); err != nil {
				return fmt.Errorf("failed to create virtual environment: %w", err)
			}
		} else {
			return fmt.Errorf("virtual environment not found: %s", venvPath)
		}
	}

	// Update Python command to use virtual environment
	if pr.options.AutoActivateVenv {
		pr.options.PythonCommand = venvPython
		pr.options.VirtualEnvPath = venvPath
	}

	return nil
}

// createVirtualEnvironment creates a new Python virtual environment
func (pr *PythonRunner) createVirtualEnvironment(ctx context.Context, venvPath string) error {
	cmd := exec.CommandContext(ctx, pr.options.PythonCommand, "-m", "venv", venvPath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to create virtual environment: %w", err)
	}
	return nil
}

// validatePythonTarget validates that the Python target exists and is accessible
func (pr *PythonRunner) validatePythonTarget(task *t.Task) error {
	// Validate Python file
	if task.FilePath != "" && strings.HasSuffix(task.FilePath, ".py") {
		if _, err := os.Stat(task.FilePath); os.IsNotExist(err) {
			return fmt.Errorf("Python file not found: %s", task.FilePath)
		}

		// Basic syntax check
		if err := pr.checkPythonSyntax(task.FilePath); err != nil {
			return fmt.Errorf("Python syntax error: %w", err)
		}
	}

	// Validate module (if module mode)
	if pr.options.ModuleMode {
		if err := pr.validatePythonModule(task.Name); err != nil {
			return fmt.Errorf("Python module validation failed: %w", err)
		}
	}

	return nil
}

// checkPythonSyntax performs basic Python syntax checking
func (pr *PythonRunner) checkPythonSyntax(filePath string) error {
	cmd := exec.Command(pr.options.PythonCommand, "-m", "py_compile", filePath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("syntax check failed: %w", err)
	}
	return nil
}

// validatePythonModule validates that a Python module can be imported
func (pr *PythonRunner) validatePythonModule(moduleName string) error {
	cmd := exec.Command(pr.options.PythonCommand, "-c", fmt.Sprintf("import %s", moduleName))
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("module %s cannot be imported: %w", moduleName, err)
	}
	return nil
}

// detectFrameworks detects Python frameworks in use
func (pr *PythonRunner) detectFrameworks(task *t.Task) error {
	taskDir := pr.getWorkingDirectory(task)

	// Check for common framework files
	frameworkFiles := map[string]string{
		"app.py":         "flask",
		"manage.py":      "django",
		"main.py":        "fastapi",
		"conftest.py":    "pytest",
		"setup.py":       "setuptools",
		"pyproject.toml": "poetry",
	}

	var detectedFrameworks []string

	for file, framework := range frameworkFiles {
		filePath := filepath.Join(taskDir, file)
		if _, err := os.Stat(filePath); err == nil {
			detectedFrameworks = append(detectedFrameworks, framework)
		}
	}

	if len(detectedFrameworks) > 0 {
		pr.logger.Debug("Detected Python frameworks", "frameworks", detectedFrameworks)
	}

	return nil
}

// installDependencies installs Python dependencies if requirements file exists
func (pr *PythonRunner) installDependencies(ctx context.Context, task *t.Task) error {
	taskDir := pr.getWorkingDirectory(task)

	// Check for requirements files
	requirementsFiles := []string{"requirements.txt", "requirements.dev.txt", "dev-requirements.txt"}

	for _, reqFile := range requirementsFiles {
		reqPath := filepath.Join(taskDir, reqFile)
		if _, err := os.Stat(reqPath); err == nil {
			pr.logger.Debug("Found requirements file", "file", reqFile)

			// Install requirements
			cmd := exec.CommandContext(ctx, pr.options.PythonCommand, "-m", "pip", "install", "-r", reqPath)
			cmd.Dir = taskDir

			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to install requirements from %s: %w", reqFile, err)
			}
		}
	}

	return nil
}

// buildCommand builds the Python command
func (pr *PythonRunner) buildCommand(task *t.Task) (*exec.Cmd, error) {
	args := []string{}

	// Add default Python arguments
	args = append(args, pr.options.PythonArgs...)

	// Add execution options
	if pr.options.Unbuffered {
		args = append(args, "-u")
	}

	if pr.options.Interactive {
		args = append(args, "-i")
	}

	if pr.options.Optimize > 0 {
		for i := 0; i < pr.options.Optimize; i++ {
			args = append(args, "-O")
		}
	}

	if pr.options.Verbose {
		args = append(args, "-v")
	}

	if pr.options.Quiet {
		args = append(args, "-q")
	}

	if pr.options.NoSitePackages {
		args = append(args, "-S")
	}

	if pr.options.NoUserSite {
		args = append(args, "-s")
	}

	// Add profiling if enabled
	if pr.options.EnableProfiling {
		args = append(args, "-m", "cProfile")
		if pr.options.ProfileOutput != "" {
			args = append(args, "-o", pr.options.ProfileOutput)
		}
	}

	// Determine execution mode
	if pr.options.ModuleMode || (len(task.Args) > 0 && task.Args[0] == "-m") {
		// Module execution: python -m module_name
		args = append(args, "-m")
		if pr.options.ModuleMode {
			args = append(args, task.Name)
		} else {
			// Skip the "-m" from task args
			args = append(args, task.Args[1:]...)
		}
	} else if task.FilePath != "" && strings.HasSuffix(task.FilePath, ".py") {
		// Script execution: python script.py
		args = append(args, task.FilePath)
	} else if pythonFunc, ok := task.Metadata["python_function"].(string); ok {
		// Function execution: python -c "from module import func; func()"
		var importPath string
		if task.FilePath != "" {
			// Convert file path to module path
			relPath, _ := filepath.Rel(pr.getWorkingDirectory(task), task.FilePath)
			importPath = strings.TrimSuffix(relPath, ".py")
			importPath = strings.ReplaceAll(importPath, "/", ".")
		} else {
			importPath = task.Name
		}

		executeCode := fmt.Sprintf("from %s import %s; %s()", importPath, pythonFunc, pythonFunc)
		args = append(args, "-c", executeCode)
	} else {
		// Direct execution or command
		if task.Command != "" && task.Command != "python" {
			return nil, fmt.Errorf("unsupported command: %s", task.Command)
		}
		args = append(args, task.Name)
	}

	// Add script arguments
	args = append(args, pr.options.ScriptArgs...)

	// Add task arguments (skip first arg if it's the script/module name)
	if len(task.Args) > 1 && !pr.options.ModuleMode {
		args = append(args, task.Args[1:]...)
	} else if pr.options.ModuleMode && len(task.Args) > 0 {
		args = append(args, task.Args...)
	}

	cmd := exec.Command(pr.options.PythonCommand, args...)
	return cmd, nil
}

// getWorkingDirectory returns the working directory for the task
func (pr *PythonRunner) getWorkingDirectory(task *t.Task) string {
	if pr.options.WorkingDirectory != "" {
		return pr.options.WorkingDirectory
	}

	if task.WorkingDirectory != "" {
		return task.WorkingDirectory
	}

	if task.FilePath != "" {
		return filepath.Dir(task.FilePath)
	}

	return "."
}

// buildEnvironment builds the environment variables for the command
func (pr *PythonRunner) buildEnvironment(task *t.Task) []string {
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
func (pr *PythonRunner) executeWithOutput(ctx context.Context, cmd *exec.Cmd, outputBuffer *output.Buffer) error {
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
			return fmt.Errorf("python command failed: %w", err)
		}
		return nil
	}
}

// streamOutput streams command output to the buffer
func (pr *PythonRunner) streamOutput(pipe any, outputBuffer *output.Buffer, isStderr bool) {
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

// parseOutputLine parses and enhances a Python output line
func (pr *PythonRunner) parseOutputLine(line string, isStderr bool) string {
	// Add timestamps and formatting
	timestamp := time.Now().Format("15:04:05.000")
	prefix := "OUT"
	if isStderr {
		prefix = "ERR"
	}

	// Check for special Python patterns
	if pr.patterns.ErrorPattern.MatchString(line) {
		prefix = "ERROR"
	} else if pr.patterns.WarningPattern.MatchString(line) {
		prefix = "WARN"
	} else if pr.patterns.ImportErrorPattern.MatchString(line) {
		prefix = "IMPORT-ERR"
	} else if pr.patterns.SyntaxErrorPattern.MatchString(line) {
		prefix = "SYNTAX-ERR"
	} else if pr.patterns.TracebackPattern.MatchString(line) {
		prefix = "TRACEBACK"
	} else if pr.patterns.FileLinePattern.MatchString(line) {
		prefix = "FILE"
	} else if pr.patterns.DeprecationPattern.MatchString(line) {
		prefix = "DEPRECATED"
	} else if pr.patterns.UnitTestPattern.MatchString(line) {
		prefix = "TEST"
	} else if pr.patterns.DocTestPattern.MatchString(line) {
		prefix = "DOCTEST"
	} else if pr.patterns.FlaskPattern.MatchString(line) {
		prefix = "FLASK"
	} else if pr.patterns.DjangoPattern.MatchString(line) {
		prefix = "DJANGO"
	}

	return fmt.Sprintf("[%s] [%s] %s", timestamp, prefix, line)
}

// GetPythonVersion returns the version of Python
func (pr *PythonRunner) GetPythonVersion() (string, error) {
	cmd := exec.Command(pr.options.PythonCommand, "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get Python version: %w", err)
	}

	// Parse version from output (format: "Python x.y.z")
	versionStr := strings.TrimSpace(string(output))
	if strings.HasPrefix(versionStr, "Python ") {
		return strings.TrimPrefix(versionStr, "Python "), nil
	}

	return versionStr, nil
}

// GetPythonEnvironment returns comprehensive Python environment information
func (pr *PythonRunner) GetPythonEnvironment() (*PythonEnvironment, error) {
	env := &PythonEnvironment{}

	// Get Python version
	if version, err := pr.GetPythonVersion(); err == nil {
		env.PythonVersion = version
	}

	// Get Python executable path
	if executable, err := exec.LookPath(pr.options.PythonCommand); err == nil {
		env.PythonExecutable = executable
	}

	// Get virtual environment
	env.VirtualEnv = pr.options.VirtualEnvPath

	// Get Python path
	cmd := exec.Command(pr.options.PythonCommand, "-c", "import sys; print('\\n'.join(sys.path))")
	if output, err := cmd.Output(); err == nil {
		paths := strings.Split(strings.TrimSpace(string(output)), "\n")
		env.PythonPath = paths
	}

	return env, nil
}

// GetInstalledPackages returns list of installed Python packages
func (pr *PythonRunner) GetInstalledPackages() (map[string]string, error) {
	cmd := exec.Command(pr.options.PythonCommand, "-m", "pip", "list", "--format=json")
	_, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get installed packages: %w", err)
	}

	// Parse JSON output would go here
	// For now, return empty map
	return make(map[string]string), nil
}

// GetTaskMetadata returns metadata specific to Python tasks
func (pr *PythonRunner) GetTaskMetadata(task *t.Task) map[string]any {
	metadata := map[string]any{
		"runner_type":    pr.Type(),
		"runner_name":    pr.Name(),
		"runner_version": pr.Version(),
		"python_command": pr.options.PythonCommand,
		"python_version": pr.options.PythonVersion,
	}

	// Add Python environment info
	if env, err := pr.GetPythonEnvironment(); err == nil {
		metadata["python_environment"] = env
	}

	// Add virtual environment info
	if pr.options.VirtualEnvPath != "" {
		metadata["virtual_env"] = pr.options.VirtualEnvPath
		metadata["virtual_env_active"] = true
	}

	// Add task-specific metadata
	if task.FilePath != "" {
		metadata["python_file"] = task.FilePath
		metadata["python_dir"] = filepath.Dir(task.FilePath)
	}

	// Add configuration
	metadata["module_mode"] = pr.options.ModuleMode
	metadata["package_mode"] = pr.options.PackageMode
	metadata["optimization_level"] = pr.options.Optimize
	metadata["profiling_enabled"] = pr.options.EnableProfiling
	metadata["python_path"] = pr.options.PythonPath

	return metadata
}
