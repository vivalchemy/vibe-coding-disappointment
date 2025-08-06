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

	"github.com/vivalchemy/wake/internal/output"
	"github.com/vivalchemy/wake/pkg/logger"
	t "github.com/vivalchemy/wake/pkg/task"
)

// JustRunner implements the Runner interface for Just tasks
type JustRunner struct {
	logger   logger.Logger
	options  *JustOptions
	patterns *JustPatterns
}

// JustOptions contains configuration for the Just runner
type JustOptions struct {
	// Just executable
	JustCommand     string
	JustArgs        []string
	DefaultJustfile string

	// Execution options
	DryRun    bool
	Verbose   bool
	Quiet     bool
	Shell     string
	ShellArgs []string

	// Working directory options
	WorkingDirectory string
	ChangeDirectory  string

	// Variable options
	Variables map[string]string
	DotEnv    []string

	// Justfile discovery
	JustfileNames []string
	SearchParents bool

	// Output options
	Color string
	List  bool
	Dump  bool

	// Error handling
	ContinueOnError bool

	// Environment
	PassEnvironment bool
}

// JustPatterns contains regex patterns for parsing Just output
type JustPatterns struct {
	ErrorPattern    *regexp.Regexp
	WarningPattern  *regexp.Regexp
	RecipePattern   *regexp.Regexp
	ShellPattern    *regexp.Regexp
	VariablePattern *regexp.Regexp
	CommentPattern  *regexp.Regexp
	ExecutePattern  *regexp.Regexp
	DryRunPattern   *regexp.Regexp
}

// JustRecipe represents a parsed Just recipe
type JustRecipe struct {
	Name         string
	Description  string
	Parameters   []JustParameter
	Dependencies []string
	Body         []string
	Private      bool
	LineNumber   int
}

// JustParameter represents a recipe parameter
type JustParameter struct {
	Name         string
	DefaultValue string
	Optional     bool
	Variadic     bool
}

// NewJustRunner creates a new Just runner
func NewJustRunner(logger logger.Logger) *JustRunner {
	return &JustRunner{
		logger:   logger.WithGroup("just-runner"),
		options:  createDefaultJustOptions(),
		patterns: createJustPatterns(),
	}
}

// createDefaultJustOptions creates default Just options
func createDefaultJustOptions() *JustOptions {
	return &JustOptions{
		JustCommand:      "just",
		JustArgs:         []string{},
		DefaultJustfile:  "justfile",
		DryRun:           false,
		Verbose:          false,
		Quiet:            false,
		Shell:            "",
		ShellArgs:        []string{},
		WorkingDirectory: "",
		ChangeDirectory:  "",
		Variables:        make(map[string]string),
		DotEnv:           []string{},
		JustfileNames:    []string{"justfile", "Justfile", ".justfile"},
		SearchParents:    true,
		Color:            "auto",
		List:             false,
		Dump:             false,
		ContinueOnError:  false,
		PassEnvironment:  true,
	}
}

// createJustPatterns creates regex patterns for Just output parsing
func createJustPatterns() *JustPatterns {
	return &JustPatterns{
		ErrorPattern:    regexp.MustCompile(`^error:\s+(.*)$`),
		WarningPattern:  regexp.MustCompile(`^warning:\s+(.*)$`),
		RecipePattern:   regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_-]*)\s*:.*$`),
		ShellPattern:    regexp.MustCompile(`^\s*[>]\s+(.*)$`),
		VariablePattern: regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_-]*)\s*:=\s*(.*)$`),
		CommentPattern:  regexp.MustCompile(`^\s*#\s*(.*)$`),
		ExecutePattern:  regexp.MustCompile(`^==>\s+(.*)$`),
		DryRunPattern:   regexp.MustCompile(`^\[DRY RUN\]\s+(.*)$`),
	}
}

// Type returns the runner type
func (jr *JustRunner) Type() t.RunnerType {
	return t.RunnerJust
}

// Name returns the runner name
func (jr *JustRunner) Name() string {
	return "Just Runner"
}

// Version returns the runner version
func (jr *JustRunner) Version() string {
	return "1.0.0"
}

// CanRun checks if the runner can execute the given task
func (jr *JustRunner) CanRun(task *t.Task) bool {
	if task.Runner != t.RunnerJust {
		return false
	}

	// Check if Just is available
	if !jr.isJustAvailable() {
		jr.logger.Warn("Just is not available")
		return false
	}

	// Check if Justfile exists
	justfilePath := jr.findJustfile(task)
	if justfilePath == "" {
		jr.logger.Warn("No Justfile found", "task", task.Name)
		return false
	}

	return true
}

// Prepare prepares the task for execution
func (jr *JustRunner) Prepare(ctx context.Context, task *t.Task) error {
	jr.logger.Debug("Preparing Just task", "task", task.Name)

	// Validate Just availability
	if !jr.isJustAvailable() {
		return fmt.Errorf("just command not found")
	}

	// Find and validate Justfile
	justfilePath := jr.findJustfile(task)
	if justfilePath == "" {
		return fmt.Errorf("no Justfile found for task %s", task.Name)
	}

	// Validate recipe exists in Justfile
	if err := jr.validateRecipe(justfilePath, task.Name); err != nil {
		return fmt.Errorf("recipe validation failed: %w", err)
	}

	// Load variables from .env files if specified
	if err := jr.loadDotEnvFiles(justfilePath); err != nil {
		jr.logger.Warn("Failed to load .env files", "error", err)
	}

	return nil
}

// Execute executes the Just task
func (jr *JustRunner) Execute(ctx context.Context, task *t.Task, outputBuffer *output.Buffer) error {
	jr.logger.Info("Executing Just task", "task", task.Name)

	// Build command
	cmd, err := jr.buildCommand(task)
	if err != nil {
		return fmt.Errorf("failed to build command: %w", err)
	}

	// Set up command context
	cmd.Dir = jr.getWorkingDirectory(task)
	cmd.Env = jr.buildEnvironment(task)

	jr.logger.Debug("Just command built",
		"cmd", cmd.Path,
		"args", cmd.Args,
		"dir", cmd.Dir)

	// Execute with output streaming
	return jr.executeWithOutput(ctx, cmd, outputBuffer)
}

// Stop stops the running Just task
func (jr *JustRunner) Stop(ctx context.Context, task *t.Task) error {
	jr.logger.Info("Stopping Just task", "task", task.Name)

	// Just doesn't have a built-in graceful stop mechanism
	// We rely on the process being killed by the executor
	return nil
}

// GetDefaultTimeout returns the default timeout for Just tasks
func (jr *JustRunner) GetDefaultTimeout() time.Duration {
	return 10 * time.Minute
}

// GetEnvironmentVariables returns environment variables for the task
func (jr *JustRunner) GetEnvironmentVariables(task *t.Task) map[string]string {
	env := make(map[string]string)

	// Add Just-specific variables
	env["JUST"] = jr.options.JustCommand

	if task.FilePath != "" {
		env["JUSTFILE"] = task.FilePath
		env["JUSTFILE_DIRECTORY"] = filepath.Dir(task.FilePath)
	}

	// Add custom variables
	for key, value := range jr.options.Variables {
		env[key] = value
	}

	// Add task-specific variables from metadata
	if justVars, ok := task.Metadata["just_variables"].(map[string]string); ok {
		for key, value := range justVars {
			env[key] = value
		}
	}

	return env
}

// Validate validates the task configuration
func (jr *JustRunner) Validate(task *t.Task) error {
	if task.Runner != t.RunnerJust {
		return fmt.Errorf("invalid runner type: expected %s, got %s", t.RunnerJust, task.Runner)
	}

	if task.Name == "" {
		return fmt.Errorf("task name cannot be empty")
	}

	// Additional validation
	if task.Command != "" && task.Command != "just" {
		jr.logger.Warn("Task has custom command, but Just runner expects 'just'", "command", task.Command)
	}

	return nil
}

// SetOptions sets runner options
func (jr *JustRunner) SetOptions(options *JustOptions) {
	jr.options = options
}

// GetOptions returns current runner options
func (jr *JustRunner) GetOptions() *JustOptions {
	return jr.options
}

// Private methods

// isJustAvailable checks if Just is available on the system
func (jr *JustRunner) isJustAvailable() bool {
	_, err := exec.LookPath(jr.options.JustCommand)
	return err == nil
}

// findJustfile finds the appropriate Justfile for the task
func (jr *JustRunner) findJustfile(task *t.Task) string {
	// If task has a specific file path, use it
	if task.FilePath != "" {
		// Check if it's a Justfile
		fileName := filepath.Base(task.FilePath)
		for _, name := range jr.options.JustfileNames {
			if fileName == name {
				if _, err := os.Stat(task.FilePath); err == nil {
					return task.FilePath
				}
			}
		}
	}

	// Look for Justfile in task directory
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
		for _, name := range jr.options.JustfileNames {
			justfilePath := filepath.Join(currentDir, name)
			if _, err := os.Stat(justfilePath); err == nil {
				return justfilePath
			}
		}

		if !jr.options.SearchParents {
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

// validateRecipe checks if the recipe exists in the Justfile
func (jr *JustRunner) validateRecipe(justfilePath, recipeName string) error {
	recipes, err := jr.parseJustfile(justfilePath)
	if err != nil {
		return fmt.Errorf("failed to parse Justfile: %w", err)
	}

	for _, recipe := range recipes {
		if recipe.Name == recipeName {
			return nil // Recipe found
		}
	}

	// Recipe not found - might be a dynamic recipe or alias
	jr.logger.Warn("Recipe not found in Justfile", "recipe", recipeName, "file", justfilePath)
	return nil
}

// parseJustfile parses a Justfile and extracts recipes
func (jr *JustRunner) parseJustfile(justfilePath string) ([]*JustRecipe, error) {
	file, err := os.Open(justfilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open Justfile: %w", err)
	}
	defer file.Close()

	var recipes []*JustRecipe
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	var currentRecipe *JustRecipe
	var pendingComment string

	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		trimmedLine := strings.TrimSpace(line)

		// Skip empty lines
		if trimmedLine == "" {
			continue
		}

		// Parse comments
		if jr.patterns.CommentPattern.MatchString(trimmedLine) {
			matches := jr.patterns.CommentPattern.FindStringSubmatch(trimmedLine)
			if len(matches) > 1 {
				pendingComment = strings.TrimSpace(matches[1])
			}
			continue
		}

		// Parse variables
		if jr.patterns.VariablePattern.MatchString(trimmedLine) {
			// Variables are handled separately
			continue
		}

		// Parse recipe headers
		if jr.patterns.RecipePattern.MatchString(trimmedLine) {
			// Save previous recipe if any
			if currentRecipe != nil {
				recipes = append(recipes, currentRecipe)
			}

			// Parse new recipe
			recipe, err := jr.parseRecipeHeader(trimmedLine, lineNumber)
			if err != nil {
				jr.logger.Warn("Failed to parse recipe header", "line", lineNumber, "error", err)
				continue
			}

			// Add pending comment as description
			if pendingComment != "" {
				recipe.Description = pendingComment
				pendingComment = ""
			}

			currentRecipe = recipe
			continue
		}

		// Parse recipe body (indented lines)
		if currentRecipe != nil && (strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "    ")) {
			bodyLine := strings.TrimLeft(line, " \t")
			currentRecipe.Body = append(currentRecipe.Body, bodyLine)
			continue
		}

		// If we have a current recipe but this line doesn't match body pattern,
		// the recipe is complete
		if currentRecipe != nil {
			recipes = append(recipes, currentRecipe)
			currentRecipe = nil
		}

		// Clear pending comment if it wasn't used
		pendingComment = ""
	}

	// Save last recipe if any
	if currentRecipe != nil {
		recipes = append(recipes, currentRecipe)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading Justfile: %w", err)
	}

	return recipes, nil
}

// parseRecipeHeader parses a recipe header line
func (jr *JustRunner) parseRecipeHeader(line string, lineNumber int) (*JustRecipe, error) {
	// Simple parsing - could be enhanced for more complex cases
	parts := strings.SplitN(line, ":", 2)
	if len(parts) < 1 {
		return nil, fmt.Errorf("invalid recipe header format")
	}

	nameAndParams := strings.TrimSpace(parts[0])
	dependencies := ""
	if len(parts) > 1 {
		dependencies = strings.TrimSpace(parts[1])
	}

	// Parse name and parameters
	var name string
	var parameters []JustParameter

	if strings.Contains(nameAndParams, " ") {
		// Has parameters
		nameParts := strings.Fields(nameAndParams)
		name = nameParts[0]

		// Parse parameters (simplified)
		for _, param := range nameParts[1:] {
			parameter := JustParameter{Name: param}

			// Check for default values
			if strings.Contains(param, "=") {
				paramParts := strings.SplitN(param, "=", 2)
				parameter.Name = paramParts[0]
				parameter.DefaultValue = paramParts[1]
				parameter.Optional = true
			}

			// Check for variadic parameters
			if strings.HasPrefix(param, "*") {
				parameter.Name = strings.TrimPrefix(param, "*")
				parameter.Variadic = true
			}

			// Check for optional parameters
			if strings.HasPrefix(param, "+") {
				parameter.Name = strings.TrimPrefix(param, "+")
				parameter.Optional = true
			}

			parameters = append(parameters, parameter)
		}
	} else {
		name = nameAndParams
	}

	recipe := &JustRecipe{
		Name:       name,
		Parameters: parameters,
		Body:       []string{},
		Private:    strings.HasPrefix(name, "_"),
		LineNumber: lineNumber,
	}

	// Parse dependencies
	if dependencies != "" {
		deps := strings.Fields(dependencies)
		recipe.Dependencies = deps
	}

	return recipe, nil
}

// loadDotEnvFiles loads variables from .env files
func (jr *JustRunner) loadDotEnvFiles(justfilePath string) error {
	justfileDir := filepath.Dir(justfilePath)

	for _, dotEnvFile := range jr.options.DotEnv {
		dotEnvPath := filepath.Join(justfileDir, dotEnvFile)
		if err := jr.loadDotEnvFile(dotEnvPath); err != nil {
			jr.logger.Warn("Failed to load .env file", "file", dotEnvFile, "error", err)
		}
	}

	return nil
}

// loadDotEnvFile loads variables from a single .env file
func (jr *JustRunner) loadDotEnvFile(filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse variable assignment
		if strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				key := strings.TrimSpace(parts[0])
				value := strings.TrimSpace(parts[1])

				// Remove quotes if present
				if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') ||
					(value[0] == '\'' && value[len(value)-1] == '\'')) {
					value = value[1 : len(value)-1]
				}

				jr.options.Variables[key] = value
			}
		}
	}

	return scanner.Err()
}

// buildCommand builds the Just command
func (jr *JustRunner) buildCommand(task *t.Task) (*exec.Cmd, error) {
	args := []string{}

	// Add default Just arguments
	args = append(args, jr.options.JustArgs...)

	// Add Justfile specification if needed
	justfilePath := jr.findJustfile(task)
	if justfilePath != "" {
		fileName := filepath.Base(justfilePath)
		if fileName != "justfile" {
			args = append(args, "--justfile", justfilePath)
		}
	}

	// Add working directory if specified
	if jr.options.WorkingDirectory != "" {
		args = append(args, "--working-directory", jr.options.WorkingDirectory)
	}

	// Add change directory if specified
	if jr.options.ChangeDirectory != "" {
		args = append(args, "--set", "working-directory", jr.options.ChangeDirectory)
	}

	// Add execution options
	if jr.options.DryRun {
		args = append(args, "--dry-run")
	}

	if jr.options.Verbose {
		args = append(args, "--verbose")
	}

	if jr.options.Quiet {
		args = append(args, "--quiet")
	}

	// Add shell configuration
	if jr.options.Shell != "" {
		args = append(args, "--shell", jr.options.Shell)
	}

	for _, shellArg := range jr.options.ShellArgs {
		args = append(args, "--shell-arg", shellArg)
	}

	// Add color option
	if jr.options.Color != "" && jr.options.Color != "auto" {
		args = append(args, "--color", jr.options.Color)
	}

	// Add variables
	for key, value := range jr.options.Variables {
		args = append(args, "--set", key, value)
	}

	// Add dotenv files
	for _, dotEnvFile := range jr.options.DotEnv {
		args = append(args, "--dotenv-path", dotEnvFile)
	}

	// Add special commands
	if jr.options.List {
		args = append(args, "--list")
	} else if jr.options.Dump {
		args = append(args, "--dump")
	} else {
		// Add recipe name
		args = append(args, task.Name)

		// Add task arguments if any
		if len(task.Args) > 1 { // Skip first arg which is usually the recipe name
			args = append(args, task.Args[1:]...)
		}
	}

	cmd := exec.Command(jr.options.JustCommand, args...)
	return cmd, nil
}

// getWorkingDirectory returns the working directory for the task
func (jr *JustRunner) getWorkingDirectory(task *t.Task) string {
	if jr.options.WorkingDirectory != "" {
		return jr.options.WorkingDirectory
	}

	if task.WorkingDirectory != "" {
		return task.WorkingDirectory
	}

	if justfilePath := jr.findJustfile(task); justfilePath != "" {
		return filepath.Dir(justfilePath)
	}

	return "."
}

// buildEnvironment builds the environment variables for the command
func (jr *JustRunner) buildEnvironment(task *t.Task) []string {
	var env []string

	// Start with current environment if enabled
	if jr.options.PassEnvironment {
		env = os.Environ()
	}

	// Add runner-specific environment variables
	runnerEnv := jr.GetEnvironmentVariables(task)
	for key, value := range runnerEnv {
		env = append(env, fmt.Sprintf("%s=%s", key, value))
	}

	return env
}

// executeWithOutput executes the command with output streaming
func (jr *JustRunner) executeWithOutput(ctx context.Context, cmd *exec.Cmd, outputBuffer *output.Buffer) error {
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
	go jr.streamOutput(stdout, outputBuffer, false)
	go jr.streamOutput(stderr, outputBuffer, true)

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
			return fmt.Errorf("just command failed: %w", err)
		}
		return nil
	}
}

// streamOutput streams command output to the buffer
func (jr *JustRunner) streamOutput(pipe any, outputBuffer *output.Buffer, isStderr bool) {
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
		enhancedLine := jr.parseOutputLine(line, isStderr)

		// Write to buffer
		if isStderr {
			outputBuffer.WriteStderr([]byte(enhancedLine + "\n"))
		} else {
			outputBuffer.WriteStdout([]byte(enhancedLine + "\n"))
		}
	}
}

// parseOutputLine parses and enhances a Just output line
func (jr *JustRunner) parseOutputLine(line string, isStderr bool) string {
	// Add timestamps and formatting
	timestamp := time.Now().Format("15:04:05.000")
	prefix := "OUT"
	if isStderr {
		prefix = "ERR"
	}

	// Check for special Just patterns
	if jr.patterns.ErrorPattern.MatchString(line) {
		prefix = "ERROR"
	} else if jr.patterns.WarningPattern.MatchString(line) {
		prefix = "WARN"
	} else if jr.patterns.ExecutePattern.MatchString(line) {
		prefix = "EXEC"
	} else if jr.patterns.DryRunPattern.MatchString(line) {
		prefix = "DRY-RUN"
	} else if jr.patterns.ShellPattern.MatchString(line) {
		prefix = "SHELL"
	}

	return fmt.Sprintf("[%s] [%s] %s", timestamp, prefix, line)
}

// GetJustVersion returns the version of Just
func (jr *JustRunner) GetJustVersion() (string, error) {
	cmd := exec.Command(jr.options.JustCommand, "--version")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get Just version: %w", err)
	}

	// Parse version from output (format: "just x.y.z")
	versionStr := strings.TrimSpace(string(output))
	parts := strings.Fields(versionStr)
	if len(parts) >= 2 {
		return parts[1], nil
	}

	return versionStr, nil
}

// ListRecipes lists all available recipes in the Justfile
func (jr *JustRunner) ListRecipes(justfilePath string) ([]*JustRecipe, error) {
	return jr.parseJustfile(justfilePath)
}

// GetRecipeInfo returns detailed information about a specific recipe
func (jr *JustRunner) GetRecipeInfo(justfilePath, recipeName string) (*JustRecipe, error) {
	recipes, err := jr.parseJustfile(justfilePath)
	if err != nil {
		return nil, err
	}

	for _, recipe := range recipes {
		if recipe.Name == recipeName {
			return recipe, nil
		}
	}

	return nil, fmt.Errorf("recipe '%s' not found", recipeName)
}

// GetTaskMetadata returns metadata specific to Just tasks
func (jr *JustRunner) GetTaskMetadata(task *t.Task) map[string]any {
	metadata := map[string]any{
		"runner_type":    jr.Type(),
		"runner_name":    jr.Name(),
		"runner_version": jr.Version(),
		"just_command":   jr.options.JustCommand,
	}

	if justfilePath := jr.findJustfile(task); justfilePath != "" {
		metadata["justfile_path"] = justfilePath

		// Add recipe information
		if recipe, err := jr.GetRecipeInfo(justfilePath, task.Name); err == nil {
			metadata["recipe_description"] = recipe.Description
			metadata["recipe_parameters"] = recipe.Parameters
			metadata["recipe_dependencies"] = recipe.Dependencies
			metadata["recipe_private"] = recipe.Private
			metadata["recipe_line_number"] = recipe.LineNumber
		}

		// Add available recipes
		if recipes, err := jr.ListRecipes(justfilePath); err == nil {
			var recipeNames []string
			for _, recipe := range recipes {
				if !recipe.Private { // Only include public recipes
					recipeNames = append(recipeNames, recipe.Name)
				}
			}
			metadata["available_recipes"] = recipeNames
		}
	}

	// Add Just version
	if version, err := jr.GetJustVersion(); err == nil {
		metadata["just_version"] = version
	}

	// Add configuration
	metadata["variables"] = jr.options.Variables
	metadata["dotenv_files"] = jr.options.DotEnv
	metadata["shell"] = jr.options.Shell
	metadata["shell_args"] = jr.options.ShellArgs

	return metadata
}
