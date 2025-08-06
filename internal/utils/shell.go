package utils

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Shell represents different shell types
type Shell int

const (
	ShellUnknown Shell = iota
	ShellBash
	ShellZsh
	ShellFish
	ShellPowerShell
	ShellCmd
	ShellSh
)

// String returns the string representation of Shell
func (s Shell) String() string {
	switch s {
	case ShellBash:
		return "bash"
	case ShellZsh:
		return "zsh"
	case ShellFish:
		return "fish"
	case ShellPowerShell:
		return "powershell"
	case ShellCmd:
		return "cmd"
	case ShellSh:
		return "sh"
	default:
		return "unknown"
	}
}

// Command represents a shell command with its execution context
type Command struct {
	// Command details
	Command string
	Args    []string
	Shell   Shell

	// Execution context
	Dir   string
	Env   []string
	Stdin io.Reader

	// Timeout and cancellation
	Timeout time.Duration
	Context context.Context

	// Output handling
	CaptureOutput  bool
	StreamOutput   bool
	OutputCallback func(line string, isStderr bool)

	// Execution options
	IgnoreErrors   bool
	SeparateStderr bool
	InheritEnv     bool
}

// CommandResult represents the result of command execution
type CommandResult struct {
	// Command info
	Command string
	Args    []string
	Dir     string

	// Execution details
	StartTime time.Time
	EndTime   time.Time
	Duration  time.Duration
	PID       int

	// Results
	ExitCode int
	Success  bool
	Stdout   string
	Stderr   string
	Combined string

	// Error information
	Error    error
	Signal   os.Signal
	Killed   bool
	TimedOut bool
}

// ShellDetector detects the current shell
type ShellDetector struct {
	cache      Shell
	cacheValid bool
	mu         sync.RWMutex
}

// CommandBuilder helps build shell commands
type CommandBuilder struct {
	shell      Shell
	dir        string
	env        []string
	timeout    time.Duration
	inheritEnv bool
}

// ShellExecutor executes shell commands
type ShellExecutor struct {
	defaultShell Shell
	defaultDir   string
	defaultEnv   []string
	timeout      time.Duration
}

// NewCommand creates a new command
func NewCommand(command string, args ...string) *Command {
	return &Command{
		Command:    command,
		Args:       args,
		Shell:      DetectShell(),
		InheritEnv: true,
		Timeout:    30 * time.Second,
		Context:    context.Background(),
	}
}

// WithShell sets the shell for the command
func (c *Command) WithShell(shell Shell) *Command {
	c.Shell = shell
	return c
}

// WithDir sets the working directory
func (c *Command) WithDir(dir string) *Command {
	c.Dir = dir
	return c
}

// WithEnv sets environment variables
func (c *Command) WithEnv(env []string) *Command {
	c.Env = env
	return c
}

// WithTimeout sets the command timeout
func (c *Command) WithTimeout(timeout time.Duration) *Command {
	c.Timeout = timeout
	return c
}

// WithContext sets the command context
func (c *Command) WithContext(ctx context.Context) *Command {
	c.Context = ctx
	return c
}

// WithStdin sets the stdin reader
func (c *Command) WithStdin(stdin io.Reader) *Command {
	c.Stdin = stdin
	return c
}

// WithOutputCallback sets the output callback
func (c *Command) WithOutputCallback(callback func(string, bool)) *Command {
	c.OutputCallback = callback
	return c
}

// CaptureOutput enables output capture
func (c *Command) CaptureOutputFunc() *Command {
	c.CaptureOutput = true
	return c
}

// StreamOutput enables output streaming
func (c *Command) StreamOutputFunc() *Command {
	c.StreamOutput = true
	return c
}

// IgnoreErrors makes the command ignore errors
func (c *Command) IgnoreErrorsFunc() *Command {
	c.IgnoreErrors = true
	return c
}

// Run executes the command and returns the result
func (c *Command) Run() (*CommandResult, error) {
	startTime := time.Now()

	result := &CommandResult{
		Command:   c.Command,
		Args:      c.Args,
		Dir:       c.Dir,
		StartTime: startTime,
	}

	// Create execution context with timeout
	ctx := c.Context
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}

	// Build the actual command
	cmd, err := c.buildExecCommand(ctx)
	if err != nil {
		result.Error = fmt.Errorf("failed to build command: %w", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		return result, result.Error
	}

	// Set up output handling
	var stdout, stderr bytes.Buffer
	var stdoutPipe, stderrPipe io.ReadCloser

	if c.CaptureOutput || c.StreamOutput {
		if stdoutPipe, err = cmd.StdoutPipe(); err != nil {
			result.Error = fmt.Errorf("failed to create stdout pipe: %w", err)
			return result, result.Error
		}

		if stderrPipe, err = cmd.StderrPipe(); err != nil {
			result.Error = fmt.Errorf("failed to create stderr pipe: %w", err)
			return result, result.Error
		}
	} else {
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
	}

	// Set stdin if provided
	if c.Stdin != nil {
		cmd.Stdin = c.Stdin
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		result.Error = fmt.Errorf("failed to start command: %w", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		return result, result.Error
	}

	result.PID = cmd.Process.Pid

	// Handle output streaming/capture
	if stdoutPipe != nil && stderrPipe != nil {
		var wg sync.WaitGroup

		// Stream stdout
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.streamOutput(stdoutPipe, &stdout, false)
		}()

		// Stream stderr
		wg.Add(1)
		go func() {
			defer wg.Done()
			c.streamOutput(stderrPipe, &stderr, true)
		}()

		// Wait for streaming to complete
		wg.Wait()
	}

	// Wait for command to complete
	err = cmd.Wait()
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	// Extract exit code and error information
	if err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitError.ExitCode()
			if status, ok := exitError.Sys().(syscall.WaitStatus); ok {
				if status.Signaled() {
					result.Signal = status.Signal()
				}
			}
		} else {
			result.Error = err
		}

		// Check if command was killed due to timeout
		select {
		case <-ctx.Done():
			if ctx.Err() == context.DeadlineExceeded {
				result.TimedOut = true
			}
			result.Killed = true
		default:
		}
	} else {
		result.ExitCode = 0
		result.Success = true
	}

	// Set output
	result.Stdout = stdout.String()
	result.Stderr = stderr.String()
	result.Combined = result.Stdout + result.Stderr

	// Return error only if not ignoring errors and command failed
	if !c.IgnoreErrors && !result.Success {
		if result.Error == nil {
			result.Error = fmt.Errorf("command failed with exit code %d", result.ExitCode)
		}
		return result, result.Error
	}

	return result, nil
}

// buildExecCommand builds the actual exec.Cmd
func (c *Command) buildExecCommand(ctx context.Context) (*exec.Cmd, error) {
	var cmd *exec.Cmd

	switch c.Shell {
	case ShellBash:
		cmd = exec.CommandContext(ctx, "bash", "-c", c.buildCommandString())
	case ShellZsh:
		cmd = exec.CommandContext(ctx, "zsh", "-c", c.buildCommandString())
	case ShellFish:
		cmd = exec.CommandContext(ctx, "fish", "-c", c.buildCommandString())
	case ShellSh:
		cmd = exec.CommandContext(ctx, "sh", "-c", c.buildCommandString())
	case ShellPowerShell:
		cmd = exec.CommandContext(ctx, "powershell", "-Command", c.buildCommandString())
	case ShellCmd:
		cmd = exec.CommandContext(ctx, "cmd", "/C", c.buildCommandString())
	default:
		// Direct execution without shell
		if len(c.Args) > 0 {
			cmd = exec.CommandContext(ctx, c.Command, c.Args...)
		} else {
			cmd = exec.CommandContext(ctx, c.Command)
		}
	}

	// Set working directory
	if c.Dir != "" {
		cmd.Dir = c.Dir
	}

	// Set environment
	if c.InheritEnv {
		cmd.Env = os.Environ()
	}
	if len(c.Env) > 0 {
		if c.InheritEnv {
			cmd.Env = append(cmd.Env, c.Env...)
		} else {
			cmd.Env = c.Env
		}
	}

	return cmd, nil
}

// buildCommandString builds the command string for shell execution
func (c *Command) buildCommandString() string {
	if len(c.Args) == 0 {
		return c.Command
	}

	// Escape arguments based on shell type
	var parts []string
	parts = append(parts, c.escapeArg(c.Command))

	for _, arg := range c.Args {
		parts = append(parts, c.escapeArg(arg))
	}

	return strings.Join(parts, " ")
}

// escapeArg escapes an argument for shell execution
func (c *Command) escapeArg(arg string) string {
	switch c.Shell {
	case ShellBash, ShellZsh, ShellSh:
		return bashEscape(arg)
	case ShellFish:
		return fishEscape(arg)
	case ShellPowerShell:
		return powershellEscape(arg)
	case ShellCmd:
		return cmdEscape(arg)
	default:
		return arg
	}
}

// streamOutput streams output from a pipe
func (c *Command) streamOutput(pipe io.ReadCloser, buffer *bytes.Buffer, isStderr bool) {
	defer pipe.Close()

	scanner := bufio.NewScanner(pipe)
	for scanner.Scan() {
		line := scanner.Text()

		// Write to buffer if capturing
		if c.CaptureOutput {
			buffer.WriteString(line + "\n")
		}

		// Call callback if streaming
		if c.StreamOutput && c.OutputCallback != nil {
			c.OutputCallback(line, isStderr)
		}
	}
}

// Shell detection

var globalShellDetector = &ShellDetector{}

// DetectShell detects the current shell
func DetectShell() Shell {
	return globalShellDetector.Detect()
}

// Detect detects the current shell
func (sd *ShellDetector) Detect() Shell {
	sd.mu.RLock()
	if sd.cacheValid {
		shell := sd.cache
		sd.mu.RUnlock()
		return shell
	}
	sd.mu.RUnlock()

	sd.mu.Lock()
	defer sd.mu.Unlock()

	// Double-check after acquiring write lock
	if sd.cacheValid {
		return sd.cache
	}

	shell := sd.detectShell()
	sd.cache = shell
	sd.cacheValid = true

	return shell
}

// detectShell performs the actual shell detection
func (sd *ShellDetector) detectShell() Shell {
	// Check SHELL environment variable first
	if shellPath := os.Getenv("SHELL"); shellPath != "" {
		shell := shellFromPath(shellPath)
		if shell != ShellUnknown {
			return shell
		}
	}

	// Platform-specific detection
	switch runtime.GOOS {
	case "windows":
		// Check for PowerShell
		if _, err := exec.LookPath("powershell"); err == nil {
			return ShellPowerShell
		}
		return ShellCmd

	default:
		// Unix-like systems
		// Try to detect based on available shells
		shells := []struct {
			name  string
			shell Shell
		}{
			{"bash", ShellBash},
			{"zsh", ShellZsh},
			{"fish", ShellFish},
			{"sh", ShellSh},
		}

		for _, s := range shells {
			if _, err := exec.LookPath(s.name); err == nil {
				return s.shell
			}
		}
	}

	return ShellUnknown
}

// shellFromPath determines shell type from path
func shellFromPath(shellPath string) Shell {
	base := filepath.Base(shellPath)

	switch base {
	case "bash":
		return ShellBash
	case "zsh":
		return ShellZsh
	case "fish":
		return ShellFish
	case "powershell", "pwsh":
		return ShellPowerShell
	case "cmd":
		return ShellCmd
	case "sh":
		return ShellSh
	default:
		return ShellUnknown
	}
}

// Command builder

// NewCommandBuilder creates a new command builder
func NewCommandBuilder() *CommandBuilder {
	return &CommandBuilder{
		shell:      DetectShell(),
		inheritEnv: true,
		timeout:    30 * time.Second,
	}
}

// WithShell sets the shell
func (cb *CommandBuilder) WithShell(shell Shell) *CommandBuilder {
	cb.shell = shell
	return cb
}

// WithDir sets the working directory
func (cb *CommandBuilder) WithDir(dir string) *CommandBuilder {
	cb.dir = dir
	return cb
}

// WithEnv sets environment variables
func (cb *CommandBuilder) WithEnv(env []string) *CommandBuilder {
	cb.env = env
	return cb
}

// WithTimeout sets the timeout
func (cb *CommandBuilder) WithTimeout(timeout time.Duration) *CommandBuilder {
	cb.timeout = timeout
	return cb
}

// Build builds a command
func (cb *CommandBuilder) Build(command string, args ...string) *Command {
	cmd := NewCommand(command, args...)
	cmd.Shell = cb.shell
	cmd.Dir = cb.dir
	cmd.Env = cb.env
	cmd.Timeout = cb.timeout
	cmd.InheritEnv = cb.inheritEnv

	return cmd
}

// Shell executor

// NewShellExecutor creates a new shell executor
func NewShellExecutor() *ShellExecutor {
	return &ShellExecutor{
		defaultShell: DetectShell(),
		timeout:      30 * time.Second,
	}
}

// Execute executes a command
func (se *ShellExecutor) Execute(command string, args ...string) (*CommandResult, error) {
	cmd := NewCommand(command, args...)
	cmd.Shell = se.defaultShell
	cmd.Dir = se.defaultDir
	cmd.Env = se.defaultEnv
	cmd.Timeout = se.timeout

	return cmd.Run()
}

// ExecuteScript executes a script
func (se *ShellExecutor) ExecuteScript(script string) (*CommandResult, error) {
	cmd := NewCommand(script)
	cmd.Shell = se.defaultShell
	cmd.Dir = se.defaultDir
	cmd.Env = se.defaultEnv
	cmd.Timeout = se.timeout

	return cmd.Run()
}

// Utility functions

// RunCommand runs a simple command and returns the result
func RunCommand(command string, args ...string) (*CommandResult, error) {
	cmd := NewCommand(command, args...)
	return cmd.Run()
}

// RunScript runs a script and returns the result
func RunScript(script string) (*CommandResult, error) {
	cmd := NewCommand(script)
	return cmd.Run()
}

// RunCommandWithTimeout runs a command with timeout
func RunCommandWithTimeout(timeout time.Duration, command string, args ...string) (*CommandResult, error) {
	cmd := NewCommand(command, args...)
	cmd.Timeout = timeout
	return cmd.Run()
}

// CommandExists checks if a command exists in PATH
func CommandExists(command string) bool {
	_, err := exec.LookPath(command)
	return err == nil
}

// GetCommandPath returns the full path to a command
func GetCommandPath(command string) (string, error) {
	return exec.LookPath(command)
}

// WhichCommand finds the path to a command (like Unix 'which')
func WhichCommand(command string) string {
	path, err := exec.LookPath(command)
	if err != nil {
		return ""
	}
	return path
}

// Argument escaping functions

// bashEscape escapes an argument for bash/zsh/sh
func bashEscape(arg string) string {
	if arg == "" {
		return "''"
	}

	// If the argument contains only safe characters, return as-is
	if regexp.MustCompile(`^[a-zA-Z0-9._/-]+$`).MatchString(arg) {
		return arg
	}

	// Escape single quotes and wrap in single quotes
	escaped := strings.ReplaceAll(arg, "'", "'\"'\"'")
	return "'" + escaped + "'"
}

// fishEscape escapes an argument for fish shell
func fishEscape(arg string) string {
	if arg == "" {
		return "''"
	}

	// Fish uses similar escaping to bash
	return bashEscape(arg)
}

// powershellEscape escapes an argument for PowerShell
func powershellEscape(arg string) string {
	if arg == "" {
		return "''"
	}

	// Escape backticks and single quotes, wrap in single quotes
	escaped := strings.ReplaceAll(arg, "'", "''")
	return "'" + escaped + "'"
}

// cmdEscape escapes an argument for Windows cmd
func cmdEscape(arg string) string {
	if arg == "" {
		return `""`
	}

	// Simple escaping for cmd - wrap in quotes and escape quotes
	if strings.ContainsAny(arg, ` "&<>|^`) {
		escaped := strings.ReplaceAll(arg, `"`, `""`)
		return `"` + escaped + `"`
	}

	return arg
}

// IsInteractiveShell checks if running in an interactive shell
func IsInteractiveShell() bool {
	// Check if stdin is a terminal
	if fileInfo, err := os.Stdin.Stat(); err == nil {
		return (fileInfo.Mode() & os.ModeCharDevice) != 0
	}
	return false
}

// GetShellCompletion returns shell completion script
func GetShellCompletion(shell Shell, programName string) string {
	switch shell {
	case ShellBash:
		return generateBashCompletion(programName)
	case ShellZsh:
		return generateZshCompletion(programName)
	case ShellFish:
		return generateFishCompletion(programName)
	default:
		return ""
	}
}

// generateBashCompletion generates bash completion script
func generateBashCompletion(programName string) string {
	return fmt.Sprintf(`# Bash completion for %s
_%s_completion() {
    local cur prev opts
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    prev="${COMP_WORDS[COMP_CWORD-1]}"

    opts="--help --version --config --verbose --quiet"

    COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
    return 0
}

complete -F _%s_completion %s
`, programName, programName, programName, programName)
}

// generateZshCompletion generates zsh completion script
func generateZshCompletion(programName string) string {
	return fmt.Sprintf(`#compdef %s

_%s() {
    local context state line
    _arguments \
        '--help[Show help]' \
        '--version[Show version]' \
        '--config[Config file]:file:_files' \
        '--verbose[Verbose output]' \
        '--quiet[Quiet output]'
}

_%s "$@"
`, programName, programName, programName)
}

// generateFishCompletion generates fish completion script
func generateFishCompletion(programName string) string {
	return fmt.Sprintf(`# Fish completion for %s
complete -c %s -l help -d "Show help"
complete -c %s -l version -d "Show version"
complete -c %s -l config -d "Config file" -r
complete -c %s -l verbose -d "Verbose output"
complete -c %s -l quiet -d "Quiet output"
`, programName, programName, programName, programName, programName, programName)
}

// ProcessExists checks if a process exists by PID
func ProcessExists(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix, sending signal 0 checks if process exists
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// KillProcess kills a process by PID
func KillProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("failed to find process: %w", err)
	}

	return process.Kill()
}

// GetProcessInfo returns information about the current process
func GetProcessInfo() map[string]any {
	return map[string]any{
		"pid":        os.Getpid(),
		"ppid":       os.Getppid(),
		"uid":        os.Getuid(),
		"gid":        os.Getgid(),
		"executable": os.Args[0],
		"args":       os.Args[1:],
		"working_dir": func() string {
			if wd, err := os.Getwd(); err == nil {
				return wd
			}
			return "unknown"
		}(),
	}
}
