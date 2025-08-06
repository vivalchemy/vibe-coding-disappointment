package flags

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/vivalchemy/wake/pkg/task"
)

// GlobalFlags represents all global flags available across commands
type GlobalFlags struct {
	// Configuration
	ConfigFile string `mapstructure:"config-file"`
	LogLevel   string `mapstructure:"log-level"`
	LogFormat  string `mapstructure:"log-format"`

	// Filtering
	Runner     string   `mapstructure:"runner"`
	ShowHidden bool     `mapstructure:"show-hidden"`
	Tags       []string `mapstructure:"tags"`

	// Environment
	EnvFiles   []string `mapstructure:"env-files"`
	WorkingDir string   `mapstructure:"working-dir"`

	// Output
	OutputFormat string `mapstructure:"output-format"`
	NoColor      bool   `mapstructure:"no-color"`
	Quiet        bool   `mapstructure:"quiet"`
	Verbose      bool   `mapstructure:"verbose"`

	// Performance
	MaxDepth   int  `mapstructure:"max-depth"`
	NoCache    bool `mapstructure:"no-cache"`
	Concurrent bool `mapstructure:"concurrent"`

	// Debugging
	Debug   bool `mapstructure:"debug"`
	Profile bool `mapstructure:"profile"`
	DryRun  bool `mapstructure:"dry-run"`
}

// ListFlags represents flags specific to the list command
type ListFlags struct {
	ShowAll    bool   `mapstructure:"show-all"`
	ShowMeta   bool   `mapstructure:"show-meta"`
	LongFormat bool   `mapstructure:"long"`
	SortBy     string `mapstructure:"sort"`
	GroupBy    string `mapstructure:"group"`
	Search     string `mapstructure:"search"`
	Limit      int    `mapstructure:"limit"`
	Offset     int    `mapstructure:"offset"`
}

// RunFlags represents flags specific to the run command
type RunFlags struct {
	Parallel        bool          `mapstructure:"parallel"`
	ContinueOnError bool          `mapstructure:"continue-on-error"`
	Timeout         time.Duration `mapstructure:"timeout"`
	MaxRetries      int           `mapstructure:"retry"`
	RetryDelay      time.Duration `mapstructure:"retry-delay"`
	Interactive     bool          `mapstructure:"interactive"`
	Confirm         bool          `mapstructure:"confirm"`
	Watch           bool          `mapstructure:"watch"`
}

// DryFlags represents flags specific to the dry command
type DryFlags struct {
	ShowEnv      bool `mapstructure:"show-env"`
	ShowDeps     bool `mapstructure:"show-deps"`
	ShowHooks    bool `mapstructure:"show-hooks"`
	ShowCommands bool `mapstructure:"show-commands"`
	Detailed     bool `mapstructure:"detailed"`
	Validate     bool `mapstructure:"validate"`
	Analyze      bool `mapstructure:"analyze"`
	Graph        bool `mapstructure:"graph"`
}

// ConfigFlags represents flags specific to config commands
type ConfigFlags struct {
	Global   bool   `mapstructure:"global"`
	Local    bool   `mapstructure:"local"`
	Key      string `mapstructure:"key"`
	Value    string `mapstructure:"value"`
	Unset    bool   `mapstructure:"unset"`
	List     bool   `mapstructure:"list"`
	Validate bool   `mapstructure:"validate"`
	Format   string `mapstructure:"format"`
}

// CacheFlags represents flags specific to cache commands
type CacheFlags struct {
	Clear bool   `mapstructure:"clear"`
	Stats bool   `mapstructure:"stats"`
	Size  bool   `mapstructure:"size"`
	Path  string `mapstructure:"path"`
}

// FlagValidator provides validation for flag values
type FlagValidator struct {
	validRunners     []string
	validOutputs     []string
	validLogLevels   []string
	validLogFormats  []string
	validSortFields  []string
	validGroupFields []string
}

// NewFlagValidator creates a new flag validator
func NewFlagValidator() *FlagValidator {
	return &FlagValidator{
		validRunners:     []string{"make", "npm", "yarn", "pnpm", "bun", "just", "task", "python"},
		validOutputs:     []string{"table", "json", "yaml", "text"},
		validLogLevels:   []string{"debug", "info", "warn", "error", "fatal"},
		validLogFormats:  []string{"text", "json"},
		validSortFields:  []string{"name", "runner", "status", "file", "modified"},
		validGroupFields: []string{"runner", "status", "file", "tags"},
	}
}

// AddGlobalFlags adds global flags to a command
func AddGlobalFlags(cmd *cobra.Command, flags *GlobalFlags) {
	pflags := cmd.PersistentFlags()

	// Configuration flags
	pflags.StringVarP(&flags.ConfigFile, "config", "c", "",
		"config file (default is .wakeconfig.toml or ~/.config/wake/config.toml)")
	pflags.StringVar(&flags.LogLevel, "log-level", "info",
		"logging level (debug, info, warn, error, fatal)")
	pflags.StringVar(&flags.LogFormat, "log-format", "text",
		"logging format (text, json)")

	// Filtering flags
	pflags.StringVar(&flags.Runner, "runner", "",
		"filter by runner type (make, npm, yarn, pnpm, bun, just, task, python)")
	pflags.BoolVar(&flags.ShowHidden, "show-hidden", false,
		"show hidden tasks")
	pflags.StringSliceVarP(&flags.Tags, "tags", "t", nil,
		"filter by tags (comma-separated)")

	// Environment flags
	pflags.StringSliceVarP(&flags.EnvFiles, "env", "e", nil,
		"environment files to load (comma-separated)")
	pflags.StringVarP(&flags.WorkingDir, "directory", "d", "",
		"working directory (default is current directory)")

	// Output flags
	pflags.StringVarP(&flags.OutputFormat, "output", "o", "table",
		"output format (table, json, yaml, text)")
	pflags.BoolVar(&flags.NoColor, "no-color", false,
		"disable colored output")
	pflags.BoolVarP(&flags.Quiet, "quiet", "q", false,
		"quiet mode (minimal output)")
	pflags.BoolVarP(&flags.Verbose, "verbose", "v", false,
		"verbose mode (detailed output)")

	// Performance flags
	pflags.IntVar(&flags.MaxDepth, "max-depth", 10,
		"maximum directory depth for discovery")
	pflags.BoolVar(&flags.NoCache, "no-cache", false,
		"disable task discovery caching")
	pflags.BoolVar(&flags.Concurrent, "concurrent", true,
		"enable concurrent task discovery")

	// Debugging flags
	pflags.BoolVar(&flags.Debug, "debug", false,
		"enable debug mode")
	pflags.BoolVar(&flags.Profile, "profile", false,
		"enable profiling")
	pflags.BoolVar(&flags.DryRun, "dry-run", false,
		"show what would be executed without running")

	// Bind flags to viper
	bindGlobalFlagsToViper(cmd)
}

// AddListFlags adds list-specific flags to a command
func AddListFlags(cmd *cobra.Command, flags *ListFlags) {
	cmdFlags := cmd.Flags()

	// Display options
	cmdFlags.BoolVarP(&flags.ShowAll, "all", "a", false,
		"show all tasks including hidden ones")
	cmdFlags.BoolVar(&flags.ShowMeta, "show-meta", false,
		"show task metadata")
	cmdFlags.BoolVarP(&flags.LongFormat, "long", "l", false,
		"show detailed task information")

	// Sorting and grouping
	cmdFlags.StringVar(&flags.SortBy, "sort", "name",
		"sort tasks by field (name, runner, status, file, modified)")
	cmdFlags.StringVar(&flags.GroupBy, "group", "",
		"group tasks by field (runner, status, file, tags)")

	// Search and filtering
	cmdFlags.StringVarP(&flags.Search, "search", "s", "",
		"search tasks by name or description")

	// Pagination
	cmdFlags.IntVar(&flags.Limit, "limit", -1,
		"limit number of results")
	cmdFlags.IntVar(&flags.Offset, "offset", 0,
		"offset for pagination")
}

// AddRunFlags adds run-specific flags to a command
func AddRunFlags(cmd *cobra.Command, flags *RunFlags) {
	cmdFlags := cmd.Flags()

	// Execution control
	cmdFlags.BoolVarP(&flags.Parallel, "parallel", "p", false,
		"run tasks in parallel instead of sequentially")
	cmdFlags.BoolVar(&flags.ContinueOnError, "continue-on-error", false,
		"continue running other tasks if one fails")
	cmdFlags.DurationVarP(&flags.Timeout, "timeout", "t", 30*time.Minute,
		"timeout for task execution")

	// Retry logic
	cmdFlags.IntVar(&flags.MaxRetries, "retry", 0,
		"maximum number of retries on failure")
	cmdFlags.DurationVar(&flags.RetryDelay, "retry-delay", 5*time.Second,
		"delay between retries")

	// Interactive options
	cmdFlags.BoolVarP(&flags.Interactive, "interactive", "i", false,
		"run in interactive mode")
	cmdFlags.BoolVarP(&flags.Confirm, "confirm", "y", false,
		"ask for confirmation before running")
	cmdFlags.BoolVarP(&flags.Watch, "watch", "w", false,
		"watch for file changes and re-run tasks")
}

// AddDryFlags adds dry-run specific flags to a command
func AddDryFlags(cmd *cobra.Command, flags *DryFlags) {
	cmdFlags := cmd.Flags()

	// Display options
	cmdFlags.BoolVar(&flags.ShowEnv, "show-env", false,
		"show environment variables that would be set")
	cmdFlags.BoolVar(&flags.ShowDeps, "show-deps", false,
		"show task dependencies")
	cmdFlags.BoolVar(&flags.ShowHooks, "show-hooks", false,
		"show pre/post execution hooks")
	cmdFlags.BoolVar(&flags.ShowCommands, "show-commands", true,
		"show resolved commands")
	cmdFlags.BoolVarP(&flags.Detailed, "detailed", "d", false,
		"show detailed execution plan")

	// Analysis options
	cmdFlags.BoolVar(&flags.Validate, "validate", false,
		"validate tasks and show validation errors")
	cmdFlags.BoolVar(&flags.Analyze, "analyze", false,
		"perform risk analysis and estimation")
	cmdFlags.BoolVar(&flags.Graph, "graph", false,
		"show dependency graph")
}

// AddConfigFlags adds config-specific flags to a command
func AddConfigFlags(cmd *cobra.Command, flags *ConfigFlags) {
	cmdFlags := cmd.Flags()

	// Scope flags
	cmdFlags.BoolVarP(&flags.Global, "global", "g", false,
		"operate on global configuration")
	cmdFlags.BoolVarP(&flags.Local, "local", "l", false,
		"operate on local configuration")

	// Operation flags
	cmdFlags.StringVarP(&flags.Key, "key", "k", "",
		"configuration key")
	cmdFlags.StringVar(&flags.Value, "value", "",
		"configuration value")
	cmdFlags.BoolVar(&flags.Unset, "unset", false,
		"unset configuration key")
	cmdFlags.BoolVar(&flags.List, "list", false,
		"list all configuration")
	cmdFlags.BoolVar(&flags.Validate, "validate", false,
		"validate configuration")
	cmdFlags.StringVar(&flags.Format, "format", "toml",
		"configuration format (toml, json, yaml)")
}

// AddCacheFlags adds cache-specific flags to a command
func AddCacheFlags(cmd *cobra.Command, flags *CacheFlags) {
	cmdFlags := cmd.Flags()

	cmdFlags.BoolVar(&flags.Clear, "clear", false,
		"clear all cache entries")
	cmdFlags.BoolVar(&flags.Stats, "stats", false,
		"show cache statistics")
	cmdFlags.BoolVar(&flags.Size, "size", false,
		"show cache size information")
	cmdFlags.StringVar(&flags.Path, "path", "",
		"cache directory path")
}

// bindGlobalFlagsToViper binds global flags to viper for config file support
func bindGlobalFlagsToViper(cmd *cobra.Command) {
	viper.BindPFlag("log_level", cmd.PersistentFlags().Lookup("log-level"))
	viper.BindPFlag("log_format", cmd.PersistentFlags().Lookup("log-format"))
	viper.BindPFlag("runner", cmd.PersistentFlags().Lookup("runner"))
	viper.BindPFlag("show_hidden", cmd.PersistentFlags().Lookup("show-hidden"))
	viper.BindPFlag("tags", cmd.PersistentFlags().Lookup("tags"))
	viper.BindPFlag("env_files", cmd.PersistentFlags().Lookup("env"))
	viper.BindPFlag("working_dir", cmd.PersistentFlags().Lookup("directory"))
	viper.BindPFlag("output_format", cmd.PersistentFlags().Lookup("output"))
	viper.BindPFlag("no_color", cmd.PersistentFlags().Lookup("no-color"))
	viper.BindPFlag("quiet", cmd.PersistentFlags().Lookup("quiet"))
	viper.BindPFlag("verbose", cmd.PersistentFlags().Lookup("verbose"))
	viper.BindPFlag("max_depth", cmd.PersistentFlags().Lookup("max-depth"))
	viper.BindPFlag("no_cache", cmd.PersistentFlags().Lookup("no-cache"))
	viper.BindPFlag("concurrent", cmd.PersistentFlags().Lookup("concurrent"))
	viper.BindPFlag("debug", cmd.PersistentFlags().Lookup("debug"))
	viper.BindPFlag("profile", cmd.PersistentFlags().Lookup("profile"))
	viper.BindPFlag("dry_run", cmd.PersistentFlags().Lookup("dry-run"))
}

// ValidateGlobalFlags validates global flag values
func (v *FlagValidator) ValidateGlobalFlags(flags *GlobalFlags) error {
	// Validate runner
	if flags.Runner != "" && !contains(v.validRunners, flags.Runner) {
		return fmt.Errorf("invalid runner '%s', must be one of: %s",
			flags.Runner, strings.Join(v.validRunners, ", "))
	}

	// Validate output format
	if !contains(v.validOutputs, flags.OutputFormat) {
		return fmt.Errorf("invalid output format '%s', must be one of: %s",
			flags.OutputFormat, strings.Join(v.validOutputs, ", "))
	}

	// Validate log level
	if !contains(v.validLogLevels, flags.LogLevel) {
		return fmt.Errorf("invalid log level '%s', must be one of: %s",
			flags.LogLevel, strings.Join(v.validLogLevels, ", "))
	}

	// Validate log format
	if !contains(v.validLogFormats, flags.LogFormat) {
		return fmt.Errorf("invalid log format '%s', must be one of: %s",
			flags.LogFormat, strings.Join(v.validLogFormats, ", "))
	}

	// Validate max depth
	if flags.MaxDepth < 1 || flags.MaxDepth > 100 {
		return fmt.Errorf("max depth must be between 1 and 100, got %d", flags.MaxDepth)
	}

	// Validate tags format
	for _, tag := range flags.Tags {
		if !isValidTag(tag) {
			return fmt.Errorf("invalid tag format '%s', tags must contain only alphanumeric characters, hyphens, and underscores", tag)
		}
	}

	// Validate environment files exist
	for _, envFile := range flags.EnvFiles {
		if _, err := os.Stat(envFile); os.IsNotExist(err) {
			return fmt.Errorf("environment file does not exist: %s", envFile)
		}
	}

	// Validate working directory exists
	if flags.WorkingDir != "" {
		if _, err := os.Stat(flags.WorkingDir); os.IsNotExist(err) {
			return fmt.Errorf("working directory does not exist: %s", flags.WorkingDir)
		}
	}

	return nil
}

// ValidateListFlags validates list command flags
func (v *FlagValidator) ValidateListFlags(flags *ListFlags) error {
	// Validate sort field
	if !contains(v.validSortFields, flags.SortBy) {
		return fmt.Errorf("invalid sort field '%s', must be one of: %s",
			flags.SortBy, strings.Join(v.validSortFields, ", "))
	}

	// Validate group field
	if flags.GroupBy != "" && !contains(v.validGroupFields, flags.GroupBy) {
		return fmt.Errorf("invalid group field '%s', must be one of: %s",
			flags.GroupBy, strings.Join(v.validGroupFields, ", "))
	}

	// Validate pagination
	if flags.Limit < -1 || flags.Limit == 0 {
		return fmt.Errorf("limit must be -1 (no limit) or positive, got %d", flags.Limit)
	}

	if flags.Offset < 0 {
		return fmt.Errorf("offset must be non-negative, got %d", flags.Offset)
	}

	return nil
}

// ValidateRunFlags validates run command flags
func (v *FlagValidator) ValidateRunFlags(flags *RunFlags) error {
	// Validate timeout
	if flags.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive, got %v", flags.Timeout)
	}

	// Validate retry settings
	if flags.MaxRetries < 0 {
		return fmt.Errorf("max retries must be non-negative, got %d", flags.MaxRetries)
	}

	if flags.RetryDelay <= 0 {
		return fmt.Errorf("retry delay must be positive, got %v", flags.RetryDelay)
	}

	// Validate conflicting flags
	if flags.Interactive && flags.Parallel {
		return fmt.Errorf("interactive mode and parallel execution cannot be used together")
	}

	return nil
}

// ValidateDryFlags validates dry command flags
func (v *FlagValidator) ValidateDryFlags(flags *DryFlags) error {
	// No specific validation needed for dry flags currently
	return nil
}

// ValidateConfigFlags validates config command flags
func (v *FlagValidator) ValidateConfigFlags(flags *ConfigFlags) error {
	// Validate scope flags
	if flags.Global && flags.Local {
		return fmt.Errorf("cannot specify both --global and --local")
	}

	// Validate format
	validFormats := []string{"toml", "json", "yaml"}
	if !contains(validFormats, flags.Format) {
		return fmt.Errorf("invalid format '%s', must be one of: %s",
			flags.Format, strings.Join(validFormats, ", "))
	}

	// Validate key format if provided
	if flags.Key != "" && !isValidConfigKey(flags.Key) {
		return fmt.Errorf("invalid config key format '%s', must use dot notation (e.g., ui.theme)", flags.Key)
	}

	return nil
}

// ValidateCacheFlags validates cache command flags
func (v *FlagValidator) ValidateCacheFlags(flags *CacheFlags) error {
	// Validate cache path if provided
	if flags.Path != "" {
		if _, err := os.Stat(flags.Path); os.IsNotExist(err) {
			return fmt.Errorf("cache path does not exist: %s", flags.Path)
		}
	}

	return nil
}

// Helper functions

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// isValidTag checks if a tag has valid format
func isValidTag(tag string) bool {
	// Tags can contain alphanumeric characters, hyphens, and underscores
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9_-]+$`, tag)
	return matched
}

// isValidConfigKey checks if a config key has valid format
func isValidConfigKey(key string) bool {
	// Config keys should use dot notation
	matched, _ := regexp.MatchString(`^[a-zA-Z][a-zA-Z0-9_]*(\.[a-zA-Z][a-zA-Z0-9_]*)*$`, key)
	return matched
}

// ParseDuration parses duration with support for common units
func ParseDuration(s string) (time.Duration, error) {
	// Handle common suffixes
	if matched, _ := regexp.MatchString(`^\d+$`, s); matched {
		// Plain number, assume seconds
		seconds, err := strconv.Atoi(s)
		if err != nil {
			return 0, err
		}
		return time.Duration(seconds) * time.Second, nil
	}

	// Use standard duration parsing
	return time.ParseDuration(s)
}

// ParseTags parses comma-separated tags with validation
func ParseTags(tagsStr string) ([]string, error) {
	if tagsStr == "" {
		return nil, nil
	}

	tags := strings.Split(tagsStr, ",")
	var validTags []string

	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}

		if !isValidTag(tag) {
			return nil, fmt.Errorf("invalid tag format '%s'", tag)
		}

		validTags = append(validTags, tag)
	}

	return validTags, nil
}

// ValidateRunnerType validates a runner type string
func ValidateRunnerType(runner string) error {
	if runner == "" {
		return nil
	}

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

	for _, validRunner := range validRunners {
		if string(validRunner) == runner {
			return nil
		}
	}

	var validStrings []string
	for _, r := range validRunners {
		validStrings = append(validStrings, string(r))
	}

	return fmt.Errorf("invalid runner type '%s', must be one of: %s",
		runner, strings.Join(validStrings, ", "))
}

// GetFlagUsageTemplate returns a custom usage template for flags
func GetFlagUsageTemplate() string {
	return `{{.Short | trim}}

{{if .Long}}{{.Long | trim}}
{{end}}{{if or .Runnable .HasSubCommands}}{{.UsageString}}{{end}}{{if .HasAvailableSubCommands}}

Available Commands:{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global Flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

Additional help topics:{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`
}

// FlagCompletion provides shell completion for flag values
type FlagCompletion struct {
	validator *FlagValidator
}

// NewFlagCompletion creates a new flag completion provider
func NewFlagCompletion() *FlagCompletion {
	return &FlagCompletion{
		validator: NewFlagValidator(),
	}
}

// CompleteRunner provides completion for runner flag
func (fc *FlagCompletion) CompleteRunner(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	var completions []string
	for _, runner := range fc.validator.validRunners {
		if strings.HasPrefix(runner, toComplete) {
			completions = append(completions, runner)
		}
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}

// CompleteOutputFormat provides completion for output format flag
func (fc *FlagCompletion) CompleteOutputFormat(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	var completions []string
	for _, format := range fc.validator.validOutputs {
		if strings.HasPrefix(format, toComplete) {
			completions = append(completions, format)
		}
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}

// CompleteLogLevel provides completion for log level flag
func (fc *FlagCompletion) CompleteLogLevel(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	var completions []string
	for _, level := range fc.validator.validLogLevels {
		if strings.HasPrefix(level, toComplete) {
			completions = append(completions, level)
		}
	}
	return completions, cobra.ShellCompDirectiveNoFileComp
}
