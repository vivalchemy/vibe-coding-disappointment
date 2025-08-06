package config

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/vivalchemy/wake/pkg/logger"
	"github.com/vivalchemy/wake/pkg/task"
	"slices"
)

// Validator provides configuration validation functionality
type Validator struct {
	logger logger.Logger
}

// ValidationError represents a configuration validation error
type ValidationError struct {
	Field   string
	Value   any
	Message string
}

// Error implements the error interface
func (ve ValidationError) Error() string {
	return fmt.Sprintf("validation error in field '%s': %s (value: %v)", ve.Field, ve.Message, ve.Value)
}

// ValidationResult contains the results of configuration validation
type ValidationResult struct {
	Valid    bool
	Errors   []ValidationError
	Warnings []ValidationError
}

// NewValidator creates a new configuration validator
func NewValidator(logger logger.Logger) *Validator {
	return &Validator{
		logger: logger.WithGroup("config-validator"),
	}
}

// Validate validates the entire configuration
func (v *Validator) Validate(config *Config) *ValidationResult {
	result := &ValidationResult{
		Valid:    true,
		Errors:   []ValidationError{},
		Warnings: []ValidationError{},
	}

	v.logger.Debug("Starting configuration validation")

	// Validate core configuration
	v.validateCore(config, result)

	// Validate discovery configuration
	v.validateDiscovery(&config.Discovery, result)

	// Validate runner configuration
	v.validateRunner(&config.Runners, result)

	// Validate output configuration
	v.validateOutput(&config.Output, result)

	// Validate UI configuration
	v.validateUI(config.UI, result)

	// Validate hooks configuration
	v.validateHooks([]HooksConfig{config.Hooks}, result)

	// Validate logging configuration
	v.validateLogging(&config.Logging, result)

	// Set overall validation result
	result.Valid = len(result.Errors) == 0

	v.logger.Debug("Configuration validation completed",
		"valid", result.Valid,
		"errors", len(result.Errors),
		"warnings", len(result.Warnings))

	return result
}

// validateCore validates core configuration settings
func (v *Validator) validateCore(config *Config, result *ValidationResult) {
	// Validate version
	if config.Version == "" {
		v.addError(result, "version", config.Version, "version is required")
	} else if !v.isValidVersion(config.Version) {
		v.addError(result, "version", config.Version, "invalid version format")
	}

	// Validate project name
	if config.ProjectName != "" && !v.isValidProjectName(config.ProjectName) {
		v.addError(result, "project_name", config.ProjectName, "invalid project name format")
	}

	// Validate working directory
	if config.WorkingDirectory != "" {
		if !v.isValidDirectory(config.WorkingDirectory) {
			v.addError(result, "working_directory", config.WorkingDirectory, "directory does not exist or is not accessible")
		}
	}

	// Validate config file path
	if config.ConfigFile != "" {
		if !v.isValidFile(config.ConfigFile) {
			v.addWarning(result, "config_file", config.ConfigFile, "config file does not exist")
		}
	}

	// Validate includes
	for i, include := range config.Includes {
		if !v.isValidFile(include) {
			v.addError(result, fmt.Sprintf("includes[%d]", i), include, "include file does not exist")
		}
	}
}

// validateDiscovery validates discovery configuration
func (v *Validator) validateDiscovery(discovery *DiscoveryConfig, result *ValidationResult) {
	if discovery == nil {
		return
	}

	// Validate paths
	for i, path := range discovery.Paths {
		if !v.isValidPath(path) {
			v.addError(result, fmt.Sprintf("discovery.paths[%d]", i), path, "path does not exist")
		}
	}

	// Validate exclude patterns
	for i, pattern := range discovery.ExcludePatterns {
		if !v.isValidGlobPattern(pattern) {
			v.addError(result, fmt.Sprintf("discovery.exclude_patterns[%d]", i), pattern, "invalid glob pattern")
		}
	}

	// Validate include patterns
	for i, pattern := range discovery.IncludePatterns {
		if !v.isValidGlobPattern(pattern) {
			v.addError(result, fmt.Sprintf("discovery.include_patterns[%d]", i), pattern, "invalid glob pattern")
		}
	}

	// Validate max depth
	if discovery.MaxDepth < 0 {
		v.addError(result, "discovery.max_depth", discovery.MaxDepth, "max depth cannot be negative")
	}

	if discovery.MaxDepth > 100 {
		v.addWarning(result, "discovery.max_depth", discovery.MaxDepth, "very large max depth may impact performance")
	}

	// Validate runners
	validRunners := []string{"make", "npm", "yarn", "pnpm", "bun", "just", "task", "python"}
	for i, runner := range discovery.Runners {
		if !v.isValidRunner(runner, validRunners) {
			v.addError(result, fmt.Sprintf("discovery.runners[%d]", i), runner, "unsupported runner type")
		}
	}

	// Validate cache settings
	// check at the start of the function
	// if discovery != nil {
	v.validateCacheConfig(&discovery.Cache, "discovery.cache", result)
	// }
}

// validateRunner validates runner configuration
func (v *Validator) validateRunner(runner *RunnersConfig, result *ValidationResult) {
	if runner == nil {
		return
	}

	// Validate concurrent limit
	if runner.ConcurrentLimit < 1 {
		v.addError(result, "runner.concurrent_limit", runner.ConcurrentLimit, "concurrent limit must be at least 1")
	}

	if runner.ConcurrentLimit > 100 {
		v.addWarning(result, "runner.concurrent_limit", runner.ConcurrentLimit, "very high concurrent limit may impact system performance")
	}

	// Validate default timeout
	// it is already of type time.Duration
	// if runner.DefaultTimeout != "" {
	// 	if _, err := time.ParseDuration(runner.DefaultTimeout); err != nil {
	// 		v.addError(result, "runner.default_timeout", runner.DefaultTimeout, "invalid duration format")
	// 	}
	// }

	// Validate environment variables
	for key, _ := range runner.Environment { // replace the _ with 'value'
		if !v.isValidEnvironmentVariable(key) {
			v.addError(result, fmt.Sprintf("runner.environment.%s", key), key, "invalid environment variable name")
		}

		// FIXME: type mismatch
		// if strings.Contains(value, "\n") || strings.Contains(value, "\r") {
		// 	v.addWarning(result, fmt.Sprintf("runner.environment.%s", key), value, "environment variable contains newline characters")
		// }
	}

	// Validate runner-specific configurations
	// if runner != nil {
	v.validateMakeConfig(&runner.Make, "runner.make", result)
	v.validateNpmConfig(&runner.Npm, "runner.npm", result)
	v.validateYarnConfig(&runner.Yarn, "runner.yarn", result)
	v.validatePnpmConfig(&runner.Pnpm, "runner.pnpm", result)
	v.validateBunConfig(&runner.Bun, "runner.bun", result)
	v.validateJustConfig(&runner.Just, "runner.just", result)
	v.validateTaskConfig(&runner.Task, "runner.task", result)
	v.validatePythonConfig(&runner.Python, "runner.python", result)
	// }
}

// validateOutput validates output configuration
func (v *Validator) validateOutput(output *OutputConfig, result *ValidationResult) {
	if output == nil {
		return
	}

	// Validate format
	validFormats := []string{"text", "json", "yaml", "table"}
	if !v.isInSlice(output.Format, validFormats) {
		v.addError(result, "output.format", output.Format, "unsupported output format")
	}

	// Validate verbosity level
	if output.Verbosity < 0 || output.Verbosity > 5 {
		v.addError(result, "output.verbosity", output.Verbosity, "verbosity must be between 0 and 5")
	}

	// Validate file output
	if output.File != "" {
		dir := filepath.Dir(output.File)
		if !v.isValidDirectory(dir) {
			v.addError(result, "output.file", output.File, "output directory does not exist")
		}

		if !v.isWritableFile(output.File) {
			v.addWarning(result, "output.file", output.File, "output file may not be writable")
		}
	}

	// Validate buffer size
	if output.BufferSize < 1024 {
		v.addWarning(result, "output.buffer_size", output.BufferSize, "small buffer size may impact performance")
	}

	if output.BufferSize > 10*1024*1024 { // 10MB
		v.addWarning(result, "output.buffer_size", output.BufferSize, "large buffer size may consume excessive memory")
	}
}

// validateUI validates UI configuration
func (v *Validator) validateUI(ui *UIConfig, result *ValidationResult) {
	if ui == nil {
		return
	}

	// Validate theme
	validThemes := []string{"default", "dark", "light", "minimal", "colorful"}
	if !v.isInSlice(ui.Theme, validThemes) {
		v.addWarning(result, "ui.theme", ui.Theme, "unknown theme, will use default")
	}

	// Validate key bindings
	validKeyBindings := []string{"default", "vim", "emacs"}
	if !v.isInSlice(ui.KeyBindings, validKeyBindings) {
		v.addWarning(result, "ui.key_bindings", ui.KeyBindings, "unknown key bindings, will use default")
	}

	// Validate split mode
	validSplitModes := []string{"horizontal", "vertical", "dynamic", "none"}
	if !v.isInSlice(ui.SplitMode, validSplitModes) {
		v.addError(result, "ui.split_mode", ui.SplitMode, "invalid split mode")
	}

	// Validate sidebar width
	if ui.SidebarWidth < 10 || ui.SidebarWidth > 200 {
		v.addWarning(result, "ui.sidebar_width", ui.SidebarWidth, "sidebar width should be between 10 and 200 characters")
	}

	// Validate refresh interval
	if ui.RefreshInterval != "" {
		if duration, err := time.ParseDuration(ui.RefreshInterval); err != nil {
			v.addError(result, "ui.refresh_interval", ui.RefreshInterval, "invalid duration format")
		} else if duration < 100*time.Millisecond {
			v.addWarning(result, "ui.refresh_interval", ui.RefreshInterval, "very short refresh interval may impact performance")
		}
	}
}

// validateHooks validates hooks configuration
// FIXME: check the hooks structure and then validate accordingly
func (v *Validator) validateHooks(hooks []HooksConfig, result *ValidationResult) {
	for i, hook := range hooks {
		prefix := fmt.Sprintf("hooks[%d]", i)

		// // Validate hook name
		// if hook.Name == "" {
		// 	v.addError(result, prefix+".name", hook.Name, "hook name is required")
		// }

		// Validate trigger
		// validTriggers := []string{"pre_run", "post_run", "pre_discovery", "post_discovery", "on_error", "on_success"}
		// if !v.isInSlice(hook.Trigger, validTriggers) {
		// 	v.addError(result, prefix+".trigger", hook.Trigger, "invalid hook trigger")
		// }

		// Validate command
		// if hook.Command == "" {
		// 	v.addError(result, prefix+".command", hook.Command, "hook command is required")
		// }

		// Validate timeout
		// if hook.Timeout != "" {
		// 	if _, err := time.ParseDuration(hook.Timeout); err != nil {
		// 		v.addError(result, prefix+".timeout", hook.Timeout, "invalid timeout duration")
		// 	}
		// }

		// Validate working directory
		// if hook.WorkingDirectory != "" && !v.isValidDirectory(hook.WorkingDirectory) {
		// 	v.addError(result, prefix+".working_directory", hook.WorkingDirectory, "working directory does not exist")
		// }

		// Validate environment variables
		for key := range hook.Environment {
			if !v.isValidEnvironmentVariable(key) {
				v.addError(result, prefix+".environment."+key, key, "invalid environment variable name")
			}
		}
	}
}

// validateLogging validates logging configuration
func (v *Validator) validateLogging(logging *LoggingConfig, result *ValidationResult) {
	if logging == nil {
		return
	}

	// Validate level
	validLevels := []string{"debug", "info", "warn", "error", "fatal"}
	if !v.isInSlice(logging.Level, validLevels) {
		v.addError(result, "logging.level", logging.Level, "invalid log level")
	}

	// Validate format
	validFormats := []string{"text", "json", "logfmt"}
	if !v.isInSlice(logging.Format, validFormats) {
		v.addError(result, "logging.format", logging.Format, "invalid log format")
	}

	// Validate file output
	if logging.File != "" {
		dir := filepath.Dir(logging.File)
		if !v.isValidDirectory(dir) {
			v.addError(result, "logging.file", logging.File, "log directory does not exist")
		}

		if !v.isWritableFile(logging.File) {
			v.addWarning(result, "logging.file", logging.File, "log file may not be writable")
		}
	}

	// Validate max size
	if logging.MaxSize < 1 {
		v.addError(result, "logging.max_size", logging.MaxSize, "max size must be at least 1MB")
	}

	if logging.MaxSize > 1000 {
		v.addWarning(result, "logging.max_size", logging.MaxSize, "very large max size may consume excessive disk space")
	}

	// Validate max age
	if logging.MaxAge < 1 {
		v.addError(result, "logging.max_age", logging.MaxAge, "max age must be at least 1 day")
	}

	// Validate max backups
	if logging.MaxBackups < 0 {
		v.addError(result, "logging.max_backups", logging.MaxBackups, "max backups cannot be negative")
	}
}

// Validation helper methods for specific runner configurations

// validateMakeConfig validates Make-specific configuration
func (v *Validator) validateMakeConfig(make *MakeConfig, prefix string, result *ValidationResult) {
	if make.Command != "" && !v.isValidCommand(make.Command) {
		v.addError(result, prefix+".command", make.Command, "make command not found")
	}

	if make.Jobs < 1 {
		v.addError(result, prefix+".jobs", make.Jobs, "jobs must be at least 1")
	}

	if make.LoadAverage < 0 {
		v.addError(result, prefix+".load_average", make.LoadAverage, "load average cannot be negative")
	}
}

// validateNpmConfig validates NPM-specific configuration
func (v *Validator) validateNpmConfig(npm *NpmConfig, prefix string, result *ValidationResult) {
	if npm.Command != "" && !v.isValidCommand(npm.Command) {
		v.addError(result, prefix+".command", npm.Command, "npm command not found")
	}

	if npm.Registry != "" && !v.isValidURL(npm.Registry) {
		v.addError(result, prefix+".registry", npm.Registry, "invalid registry URL")
	}

	if npm.CacheDir != "" && !v.isValidDirectory(npm.CacheDir) {
		v.addWarning(result, prefix+".cache_dir", npm.CacheDir, "cache directory does not exist")
	}
}

// validateYarnConfig validates Yarn-specific configuration
func (v *Validator) validateYarnConfig(yarn *YarnConfig, prefix string, result *ValidationResult) {
	if yarn.Command != "" && !v.isValidCommand(yarn.Command) {
		v.addError(result, prefix+".command", yarn.Command, "yarn command not found")
	}

	if yarn.Registry != "" && !v.isValidURL(yarn.Registry) {
		v.addError(result, prefix+".registry", yarn.Registry, "invalid registry URL")
	}

	if yarn.CacheFolder != "" && !v.isValidDirectory(yarn.CacheFolder) {
		v.addWarning(result, prefix+".cache_folder", yarn.CacheFolder, "cache folder does not exist")
	}

	if yarn.NetworkTimeout < 1000 {
		v.addWarning(result, prefix+".network_timeout", yarn.NetworkTimeout, "very short network timeout may cause failures")
	}
}

// validatePnpmConfig validates pnpm-specific configuration
func (v *Validator) validatePnpmConfig(pnpm *PnpmConfig, prefix string, result *ValidationResult) {
	if pnpm.Command != "" && !v.isValidCommand(pnpm.Command) {
		v.addError(result, prefix+".command", pnpm.Command, "pnpm command not found")
	}

	if pnpm.Registry != "" && !v.isValidURL(pnpm.Registry) {
		v.addError(result, prefix+".registry", pnpm.Registry, "invalid registry URL")
	}

	if pnpm.StoreDir != "" && !v.isValidDirectory(pnpm.StoreDir) {
		v.addWarning(result, prefix+".store_dir", pnpm.StoreDir, "store directory does not exist")
	}

	if pnpm.NetworkConcurrency < 1 {
		v.addError(result, prefix+".network_concurrency", pnpm.NetworkConcurrency, "network concurrency must be at least 1")
	}

	if pnpm.ChildConcurrency < 1 {
		v.addError(result, prefix+".child_concurrency", pnpm.ChildConcurrency, "child concurrency must be at least 1")
	}
}

// validateBunConfig validates Bun-specific configuration
func (v *Validator) validateBunConfig(bun *BunConfig, prefix string, result *ValidationResult) {
	if bun.Command != "" && !v.isValidCommand(bun.Command) {
		v.addError(result, prefix+".command", bun.Command, "bun command not found")
	}

	if bun.Registry != "" && !v.isValidURL(bun.Registry) {
		v.addError(result, prefix+".registry", bun.Registry, "invalid registry URL")
	}

	validTargets := []string{"bun", "node", "browser"}
	if bun.Target != "" && !v.isInSlice(bun.Target, validTargets) {
		v.addError(result, prefix+".target", bun.Target, "invalid target")
	}

	validFormats := []string{"esm", "cjs", "iife"}
	if bun.Format != "" && !v.isInSlice(bun.Format, validFormats) {
		v.addError(result, prefix+".format", bun.Format, "invalid output format")
	}
}

// validateJustConfig validates Just-specific configuration
func (v *Validator) validateJustConfig(just *JustConfig, prefix string, result *ValidationResult) {
	if just.Command != "" && !v.isValidCommand(just.Command) {
		v.addError(result, prefix+".command", just.Command, "just command not found")
	}

	if just.Shell != "" && !v.isValidCommand(just.Shell) {
		v.addWarning(result, prefix+".shell", just.Shell, "specified shell not found")
	}

	validColors := []string{"auto", "always", "never"}
	if just.Color != "" && !v.isInSlice(just.Color, validColors) {
		v.addError(result, prefix+".color", just.Color, "invalid color option")
	}

	if just.WorkingDirectory != "" && !v.isValidDirectory(just.WorkingDirectory) {
		v.addError(result, prefix+".working_directory", just.WorkingDirectory, "working directory does not exist")
	}
}

// validateTaskConfig validates Task-specific configuration
func (v *Validator) validateTaskConfig(taskConfig *TaskConfig, prefix string, result *ValidationResult) {
	if taskConfig.Command != "" && !v.isValidCommand(taskConfig.Command) {
		v.addError(result, prefix+".command", taskConfig.Command, "task command not found")
	}

	if taskConfig.Concurrency < 1 {
		v.addError(result, prefix+".concurrency", taskConfig.Concurrency, "concurrency must be at least 1")
	}

	if taskConfig.Interval != "" {
		if _, err := time.ParseDuration(taskConfig.Interval); err != nil {
			v.addError(result, prefix+".interval", taskConfig.Interval, "invalid interval duration")
		}
	}

	if taskConfig.Directory != "" && !v.isValidDirectory(taskConfig.Directory) {
		v.addError(result, prefix+".directory", taskConfig.Directory, "directory does not exist")
	}
}

// validatePythonConfig validates Python-specific configuration
func (v *Validator) validatePythonConfig(python *PythonConfig, prefix string, result *ValidationResult) {
	if python.Command != "" && !v.isValidCommand(python.Command) {
		v.addError(result, prefix+".command", python.Command, "python command not found")
	}

	if python.VirtualEnv != "" && !v.isValidDirectory(python.VirtualEnv) {
		v.addWarning(result, prefix+".virtual_env", python.VirtualEnv, "virtual environment directory does not exist")
	}

	if python.Optimize < 0 || python.Optimize > 2 {
		v.addError(result, prefix+".optimize", python.Optimize, "optimization level must be 0, 1, or 2")
	}

	// Validate PYTHONPATH entries
	for i, path := range python.PythonPath {
		if !v.isValidPath(path) {
			v.addWarning(result, fmt.Sprintf("%s.python_path[%d]", prefix, i), path, "path does not exist")
		}
	}
}

// validateCacheConfig validates cache configuration
func (v *Validator) validateCacheConfig(cache *CacheConfig, prefix string, result *ValidationResult) {
	if cache.TTL != "" {
		if duration, err := time.ParseDuration(cache.TTL); err != nil {
			v.addError(result, prefix+".ttl", cache.TTL, "invalid TTL duration")
		} else if duration < time.Second {
			v.addWarning(result, prefix+".ttl", cache.TTL, "very short TTL may reduce cache effectiveness")
		}
	}

	if cache.MaxSize < 1 {
		v.addError(result, prefix+".max_size", cache.MaxSize, "max size must be at least 1")
	}

	if cache.MaxSize > 10000 {
		v.addWarning(result, prefix+".max_size", cache.MaxSize, "very large cache size may consume excessive memory")
	}

	if cache.Directory != "" && !v.isValidDirectory(cache.Directory) {
		v.addWarning(result, prefix+".directory", cache.Directory, "cache directory does not exist")
	}
}

// Helper validation methods

// isValidVersion checks if version string is valid
func (v *Validator) isValidVersion(version string) bool {
	versionRegex := regexp.MustCompile(`^\d+(\.\d+)*(-[a-zA-Z0-9]+)?$`)
	return versionRegex.MatchString(version)
}

// isValidProjectName checks if project name is valid
func (v *Validator) isValidProjectName(name string) bool {
	nameRegex := regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)
	return nameRegex.MatchString(name)
}

// isValidDirectory checks if directory exists
func (v *Validator) isValidDirectory(path string) bool {
	if path == "" {
		return true
	}

	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// isValidFile checks if file exists
func (v *Validator) isValidFile(path string) bool {
	if path == "" {
		return true
	}

	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// isValidPath checks if path exists (file or directory)
func (v *Validator) isValidPath(path string) bool {
	if path == "" {
		return true
	}

	_, err := os.Stat(path)
	return err == nil
}

// isWritableFile checks if file can be written to
func (v *Validator) isWritableFile(path string) bool {
	if path == "" {
		return true
	}

	// Check if file exists
	if _, err := os.Stat(path); err == nil {
		// File exists, check if writable
		file, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			return false
		}
		file.Close()
		return true
	}

	// File doesn't exist, check if directory is writable
	dir := filepath.Dir(path)
	return v.isValidDirectory(dir)
}

// isValidCommand checks if command exists in PATH
func (v *Validator) isValidCommand(command string) bool {
	if command == "" {
		return true
	}

	// Extract command name (first word)
	cmdName := strings.Fields(command)[0]

	// Check if it's an absolute path
	if filepath.IsAbs(cmdName) {
		return v.isValidFile(cmdName)
	}

	// Check in PATH
	_, err := os.Stat(cmdName)
	return err == nil
}

// isValidURL checks if URL is valid
func (v *Validator) isValidURL(urlStr string) bool {
	if urlStr == "" {
		return true
	}

	_, err := url.Parse(urlStr)
	return err == nil
}

// isValidGlobPattern checks if glob pattern is valid
func (v *Validator) isValidGlobPattern(pattern string) bool {
	if pattern == "" {
		return true
	}

	_, err := filepath.Match(pattern, "test")
	return err == nil
}

// isValidRunner checks if runner type is supported
func (v *Validator) isValidRunner(runner string, validRunners []string) bool {
	return v.isInSlice(runner, validRunners)
}

// isValidEnvironmentVariable checks if environment variable name is valid
func (v *Validator) isValidEnvironmentVariable(name string) bool {
	if name == "" {
		return false
	}

	envVarRegex := regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
	return envVarRegex.MatchString(name)
}

// isInSlice checks if value is in slice
func (v *Validator) isInSlice(value string, slice []string) bool {
	return slices.Contains(slice, value)
}

// addError adds a validation error
func (v *Validator) addError(result *ValidationResult, field string, value any, message string) {
	result.Errors = append(result.Errors, ValidationError{
		Field:   field,
		Value:   value,
		Message: message,
	})
}

// addWarning adds a validation warning
func (v *Validator) addWarning(result *ValidationResult, field string, value any, message string) {
	result.Warnings = append(result.Warnings, ValidationError{
		Field:   field,
		Value:   value,
		Message: message,
	})
}

// ValidateRunnerTypes validates that all specified runner types are supported
func (v *Validator) ValidateRunnerTypes(runners []string) error {
	validRunners := []task.RunnerType{
		task.RunnerMake,
		task.RunnerNpm,
		task.RunnerYarn,
		task.RunnerPnpm,
		task.RunnerBun,
		task.RunnerJust,
		task.RunnerTask,
		task.RunnerPython,
	}

	validRunnerStrings := make([]string, len(validRunners))
	for i, runner := range validRunners {
		validRunnerStrings[i] = string(runner)
	}

	for _, runner := range runners {
		if !v.isInSlice(runner, validRunnerStrings) {
			return fmt.Errorf("unsupported runner type: %s", runner)
		}
	}

	return nil
}

// ValidateEnvironment validates environment variable settings
func (v *Validator) ValidateEnvironment(env map[string]string) []ValidationError {
	var errors []ValidationError

	for key, value := range env {
		if !v.isValidEnvironmentVariable(key) {
			errors = append(errors, ValidationError{
				Field:   key,
				Value:   key,
				Message: "invalid environment variable name",
			})
		}

		if strings.Contains(value, "\x00") {
			errors = append(errors, ValidationError{
				Field:   key,
				Value:   value,
				Message: "environment variable value contains null character",
			})
		}
	}

	return errors
}

// ValidateTimeout validates timeout duration strings
func (v *Validator) ValidateTimeout(timeout string) error {
	if timeout == "" {
		return nil
	}

	duration, err := time.ParseDuration(timeout)
	if err != nil {
		return fmt.Errorf("invalid timeout duration: %w", err)
	}

	if duration < 0 {
		return fmt.Errorf("timeout cannot be negative")
	}

	if duration > 24*time.Hour {
		return fmt.Errorf("timeout exceeds maximum allowed duration (24 hours)")
	}

	return nil
}

// GetValidationSummary returns a formatted summary of validation results
func (v *Validator) GetValidationSummary(result *ValidationResult) string {
	var summary strings.Builder

	if result.Valid {
		summary.WriteString("✓ Configuration is valid\n")
	} else {
		summary.WriteString("✗ Configuration validation failed\n")
	}

	if len(result.Errors) > 0 {
		summary.WriteString(fmt.Sprintf("\nErrors (%d):\n", len(result.Errors)))
		for _, err := range result.Errors {
			summary.WriteString(fmt.Sprintf("  - %s\n", err.Error()))
		}
	}

	if len(result.Warnings) > 0 {
		summary.WriteString(fmt.Sprintf("\nWarnings (%d):\n", len(result.Warnings)))
		for _, warn := range result.Warnings {
			summary.WriteString(fmt.Sprintf("  - %s\n", warn.Error()))
		}
	}

	return summary.String()
}
