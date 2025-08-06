package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/vivalchemy/wake/internal/output"
	"maps"
	"slices"
)

// Config represents the complete Wake configuration
type Config struct {
	// Basic metadata
	Version          string   `toml:"version"`
	ProjectName      string   `toml:"project_name"`
	WorkingDirectory string   `toml:"working_directory"`
	ConfigFile       string   `toml:"config_file"`
	Includes         []string `toml:"includes"`

	// Main configuration sections
	UI          *UIConfig         `toml:"ui"`
	Keybindings KeybindingsConfig `toml:"keybindings"`
	Sources     SourcesConfig     `toml:"sources"`
	Hooks       HooksConfig       `toml:"hooks"`
	Runners     RunnersConfig     `toml:"runners"`
	Discovery   DiscoveryConfig   `toml:"discovery"`
	Output      OutputConfig      `toml:"output"`
	Performance PerformanceConfig `toml:"performance"`
	Logging     LoggingConfig     `toml:"logging"`
}

// UIConfig contains user interface settings
type UIConfig struct {
	Theme            string            `toml:"theme"`
	Layout           string            `toml:"layout"`
	Colors           map[string]string `toml:"colors"`
	ShowIcons        bool              `toml:"show_icons"`
	ShowDescriptions bool              `toml:"show_descriptions"`
	MouseEnabled     bool              `toml:"mouse_enabled"`
	RefreshRate      int               `toml:"refresh_rate"`
	Mode             string            `toml:"mode"`
	KeyBindings      string            `toml:"key_bindings"`
	SplitMode        string            `toml:"split_mode"`
	SidebarWidth     int               `toml:"sidebar_width"`
	ShowLineNumbers  bool              `toml:"show_line_numbers"`
	ShowStatus       bool              `toml:"show_status"`
	ShowHelp         bool              `toml:"show_help"`
	RefreshInterval  string            `toml:"refresh_interval"`
	MouseSupport     bool              `toml:"mouse_support"`
	AltScreen        bool              `toml:"alt_screen"`
}

// KeybindingsConfig contains keybinding mappings
type KeybindingsConfig struct {
	Quit          []string `toml:"quit"`
	Help          []string `toml:"help"`
	Search        []string `toml:"search"`
	Up            []string `toml:"up"`
	Down          []string `toml:"down"`
	Left          []string `toml:"left"`
	Right         []string `toml:"right"`
	Run           []string `toml:"run"`
	Stop          []string `toml:"stop"`
	Restart       []string `toml:"restart"`
	Clear         []string `toml:"clear"`
	FocusOutput   []string `toml:"focus_output"`
	CopyOutput    []string `toml:"copy_output"`
	SaveOutput    []string `toml:"save_output"`
	ToggleSidebar []string `toml:"toggle_sidebar"`
	NextTab       []string `toml:"next_tab"`
	PrevTab       []string `toml:"prev_tab"`
}

// SourcesConfig contains task source settings
type SourcesConfig struct {
	Enabled        []string          `toml:"enabled"`
	Disabled       []string          `toml:"disabled"`
	CustomPatterns map[string]string `toml:"custom_patterns"`
	MaxDepth       int               `toml:"max_depth"`
	IgnorePatterns []string          `toml:"ignore_patterns"`
	FollowSymlinks bool              `toml:"follow_symlinks"`
}

// HooksConfig contains pre/post command hooks
type HooksConfig struct {
	PreCommand  []string          `toml:"pre_command"`
	PostCommand []string          `toml:"post_command"`
	PreSuccess  []string          `toml:"pre_success"`
	PostFailure []string          `toml:"post_failure"`
	Timeout     time.Duration     `toml:"timeout"`
	Environment map[string]string `toml:"environment"`
}

// RunnersConfig contains task runner priorities and settings
type RunnersConfig struct {
	NpmPriority      []string                     `toml:"npm_priority"`
	DefaultArgs      map[string][]string          `toml:"default_args"`
	Environment      map[string]map[string]string `toml:"environment"`
	WorkingDirectory string                       `toml:"working_directory"`
	Timeout          time.Duration                `toml:"timeout"`
	ConcurrentLimit  int                          `toml:"concurrent_limit"`
	Make             MakeConfig                   `toml:"make"`
	Npm              NpmConfig                    `toml:"npm"`
	Yarn             YarnConfig                   `toml:"yarn"`
	Pnpm             PnpmConfig                   `toml:"pnpm"`
	Bun              BunConfig                    `toml:"bun"`
	Just             JustConfig                   `toml:"just"`
	Task             TaskConfig                   `toml:"task"`
	Python           PythonConfig                 `toml:"python"`
}

// DiscoveryConfig contains task discovery settings
type DiscoveryConfig struct {
	Enabled            bool          `toml:"enabled"`
	Paths              []string      `toml:"paths"`
	CacheEnabled       bool          `toml:"cache_enabled"`
	CacheDir           string        `toml:"cache_dir"`
	CacheTTL           time.Duration `toml:"cache_ttl"`
	ExcludePatterns    []string      `toml:"exclude_patterns"`
	IncludePatterns    []string      `toml:"include_patterns"`
	MaxDepth           int           `toml:"max_depth"`
	IgnoreHidden       bool          `toml:"ignore_hidden"`
	Runners            []string      `toml:"runners"`
	ConcurrentScanning bool          `toml:"concurrent_scanning"`
	MaxConcurrency     int           `toml:"max_concurrency"`
	WatchEnabled       bool          `toml:"watch_enabled"`
	Cache              CacheConfig   `toml:"cache"`
}

// OutputConfig contains output management settings
type OutputConfig struct {
	Format          string        `toml:"format"`
	Verbosity       int           `toml:"verbosity"`
	ShowProgress    bool          `toml:"show_progress"`
	ShowTimestamp   bool          `toml:"show_timestamp"`
	ShowDuration    bool          `toml:"show_duration"`
	Colors          bool          `toml:"colors"`
	File            string        `toml:"file"`
	Append          bool          `toml:"append"`
	BufferSize      int           `toml:"buffer_size"`
	MaxHistoryLines int           `toml:"max_history_lines"`
	PersistOutput   bool          `toml:"persist_output"`
	OutputDir       string        `toml:"output_dir"`
	EnableColors    bool          `toml:"enable_colors"`
	TimestampFormat string        `toml:"timestamp_format"`
	FlushInterval   time.Duration `toml:"flush_interval"`
	StreamOutput    bool          `toml:"stream_output"`
}

// PerformanceConfig contains performance-related settings
type PerformanceConfig struct {
	MemoryLimit      int  `toml:"memory_limit"`
	CPULimit         int  `toml:"cpu_limit"`
	ProfilingEnabled bool `toml:"profiling_enabled"`
	ProfilingPort    int  `toml:"profiling_port"`
}

// LoggingConfig contains logging settings
type LoggingConfig struct {
	Level      string `toml:"level"`
	Format     string `toml:"format"`
	File       string `toml:"file"`
	MaxSize    int    `toml:"max_size"`
	MaxAge     int    `toml:"max_age"`
	MaxBackups int    `toml:"max_backups"`
	Compress   bool   `toml:"compress"`
	LocalTime  bool   `toml:"local_time"`
}

// CacheConfig contains cache settings
type CacheConfig struct {
	Enabled   bool   `toml:"enabled"`
	TTL       string `toml:"ttl"`
	MaxSize   int    `toml:"max_size"`
	Directory string `toml:"directory"`
}

// Runner-specific configurations
type MakeConfig struct {
	Command         string            `toml:"command"`
	Args            []string          `toml:"args"`
	Parallel        bool              `toml:"parallel"`
	Jobs            int               `toml:"jobs"`
	KeepGoing       bool              `toml:"keep_going"`
	Silent          bool              `toml:"silent"`
	DryRun          bool              `toml:"dry_run"`
	LoadAverage     float64           `toml:"load_average"`
	Variables       map[string]string `toml:"variables"`
	DefaultMakefile string            `toml:"default_makefile"`
}

type NpmConfig struct {
	Command       string   `toml:"command"`
	Args          []string `toml:"args"`
	Registry      string   `toml:"registry"`
	Silent        bool     `toml:"silent"`
	Progress      bool     `toml:"progress"`
	Production    bool     `toml:"production"`
	GlobalInstall bool     `toml:"global_install"`
	CacheDir      string   `toml:"cache_dir"`
	OfflineMode   bool     `toml:"offline_mode"`
	LogLevel      string   `toml:"log_level"`
	IgnoreScripts bool     `toml:"ignore_scripts"`
	AuditLevel    string   `toml:"audit_level"`
}

type YarnConfig struct {
	Command        string   `toml:"command"`
	Args           []string `toml:"args"`
	Registry       string   `toml:"registry"`
	Silent         bool     `toml:"silent"`
	Verbose        bool     `toml:"verbose"`
	Production     bool     `toml:"production"`
	FrozenLockfile bool     `toml:"frozen_lockfile"`
	OfflineMode    bool     `toml:"offline_mode"`
	CacheFolder    string   `toml:"cache_folder"`
	NetworkTimeout int      `toml:"network_timeout"`
	IgnoreScripts  bool     `toml:"ignore_scripts"`
	CheckFiles     bool     `toml:"check_files"`
}

type PnpmConfig struct {
	Command              string   `toml:"command"`
	Args                 []string `toml:"args"`
	Registry             string   `toml:"registry"`
	Silent               bool     `toml:"silent"`
	Production           bool     `toml:"production"`
	FrozenLockfile       bool     `toml:"frozen_lockfile"`
	StoreDir             string   `toml:"store_dir"`
	NetworkConcurrency   int      `toml:"network_concurrency"`
	ChildConcurrency     int      `toml:"child_concurrency"`
	IgnoreScripts        bool     `toml:"ignore_scripts"`
	VerifyStoreIntegrity bool     `toml:"verify_store_integrity"`
}

type BunConfig struct {
	Command       string   `toml:"command"`
	Args          []string `toml:"args"`
	Registry      string   `toml:"registry"`
	Target        string   `toml:"target"`
	Format        string   `toml:"format"`
	MinifyOutput  bool     `toml:"minify_output"`
	SourceMap     bool     `toml:"source_map"`
	Hot           bool     `toml:"hot"`
	Watch         bool     `toml:"watch"`
	IgnoreScripts bool     `toml:"ignore_scripts"`
}

type JustConfig struct {
	Command          string            `toml:"command"`
	Args             []string          `toml:"args"`
	Shell            string            `toml:"shell"`
	ShellArgs        []string          `toml:"shell_args"`
	WorkingDirectory string            `toml:"working_directory"`
	DryRun           bool              `toml:"dry_run"`
	Verbose          bool              `toml:"verbose"`
	Quiet            bool              `toml:"quiet"`
	Color            string            `toml:"color"`
	Variables        map[string]string `toml:"variables"`
	DotEnv           []string          `toml:"dot_env"`
	SearchParents    bool              `toml:"search_parents"`
}

type TaskConfig struct {
	Command     string            `toml:"command"`
	Args        []string          `toml:"args"`
	Taskfile    string            `toml:"taskfile"`
	Directory   string            `toml:"directory"`
	DryRun      bool              `toml:"dry_run"`
	Verbose     bool              `toml:"verbose"`
	Silent      bool              `toml:"silent"`
	Parallel    bool              `toml:"parallel"`
	Continue    bool              `toml:"continue"`
	Force       bool              `toml:"force"`
	Watch       bool              `toml:"watch"`
	Color       bool              `toml:"color"`
	Summary     bool              `toml:"summary"`
	List        bool              `toml:"list"`
	Concurrency int               `toml:"concurrency"`
	Interval    string            `toml:"interval"`
	Variables   map[string]string `toml:"variables"`
}

type PythonConfig struct {
	Command          string            `toml:"command"`
	Args             []string          `toml:"args"`
	VirtualEnv       string            `toml:"virtual_env"`
	Interactive      bool              `toml:"interactive"`
	Unbuffered       bool              `toml:"unbuffered"`
	Optimize         int               `toml:"optimize"`
	Verbose          bool              `toml:"verbose"`
	Quiet            bool              `toml:"quiet"`
	PythonPath       []string          `toml:"python_path"`
	Environment      map[string]string `toml:"environment"`
	AutoActivateVenv bool              `toml:"auto_activate_venv"`
	DetectFrameworks bool              `toml:"detect_frameworks"`
}

// Default returns a configuration with sensible defaults
func Default() *Config {
	homeDir, _ := os.UserHomeDir()
	cacheDir := filepath.Join(homeDir, ".cache", "wake")

	return &Config{
		Version:          "1.0.0",
		ProjectName:      "",
		WorkingDirectory: "",
		ConfigFile:       "",
		Includes:         []string{},
		UI: &UIConfig{
			Theme:            "dark",
			Layout:           "split",
			Colors:           make(map[string]string),
			ShowIcons:        true,
			ShowDescriptions: true,
			MouseEnabled:     true,
			RefreshRate:      16, // 60 FPS
			Mode:             "auto",
			KeyBindings:      "default",
			SplitMode:        "vertical",
			SidebarWidth:     40,
			ShowLineNumbers:  true,
			ShowStatus:       true,
			ShowHelp:         true,
			RefreshInterval:  "100ms",
			MouseSupport:     true,
			AltScreen:        true,
		},
		Keybindings: KeybindingsConfig{
			Quit:          []string{"q", "esc", "ctrl+c"},
			Help:          []string{"?", "F1"},
			Search:        []string{"/"},
			Up:            []string{"k", "up"},
			Down:          []string{"j", "down"},
			Left:          []string{"h", "left"},
			Right:         []string{"l", "right"},
			Run:           []string{"space", "enter"},
			Stop:          []string{"s", "ctrl+c"},
			Restart:       []string{"r"},
			Clear:         []string{"c"},
			FocusOutput:   []string{"tab"},
			CopyOutput:    []string{"y"},
			SaveOutput:    []string{"w"},
			ToggleSidebar: []string{"z"},
			NextTab:       []string{"ctrl+n"},
			PrevTab:       []string{"ctrl+p"},
		},
		Sources: SourcesConfig{
			Enabled:        []string{"make", "npm", "just", "task", "python"},
			Disabled:       []string{},
			CustomPatterns: make(map[string]string),
			MaxDepth:       10,
			IgnorePatterns: []string{
				"node_modules",
				".git",
				".svn",
				".hg",
				"vendor",
				"target",
				"build",
				"dist",
			},
			FollowSymlinks: false,
		},
		Hooks: HooksConfig{
			PreCommand:  []string{},
			PostCommand: []string{},
			PreSuccess:  []string{},
			PostFailure: []string{},
			Timeout:     30 * time.Second,
			Environment: make(map[string]string),
		},
		Runners: RunnersConfig{
			NpmPriority:     []string{"pnpm", "yarn", "npm", "bun"},
			DefaultArgs:     make(map[string][]string),
			Environment:     make(map[string]map[string]string),
			Timeout:         5 * time.Minute,
			ConcurrentLimit: 5,
			Make:            getDefaultMakeConfig(),
			Npm:             getDefaultNpmConfig(),
			Yarn:            getDefaultYarnConfig(),
			Pnpm:            getDefaultPnpmConfig(),
			Bun:             getDefaultBunConfig(),
			Just:            getDefaultJustConfig(),
			Task:            getDefaultTaskConfig(),
			Python:          getDefaultPythonConfig(),
		},
		Discovery: DiscoveryConfig{
			Enabled:            true,
			Paths:              []string{"."},
			CacheEnabled:       true,
			CacheDir:           cacheDir,
			CacheTTL:           1 * time.Hour,
			ExcludePatterns:    getDefaultExcludePatterns(),
			IncludePatterns:    []string{},
			MaxDepth:           10,
			IgnoreHidden:       true,
			Runners:            []string{"make", "npm", "yarn", "pnpm", "bun", "just", "task", "python"},
			ConcurrentScanning: true,
			MaxConcurrency:     runtime.NumCPU(),
			WatchEnabled:       true,
			Cache:              getDefaultCacheConfig(),
		},
		Output: OutputConfig{
			Format:          "text",
			Verbosity:       1,
			ShowProgress:    true,
			ShowTimestamp:   false,
			ShowDuration:    true,
			Colors:          true,
			File:            "",
			Append:          false,
			BufferSize:      64 * 1024, // 64KB
			MaxHistoryLines: 1000,
			PersistOutput:   false,
			OutputDir:       filepath.Join(cacheDir, "output"),
			EnableColors:    true,
			TimestampFormat: "15:04:05",
			FlushInterval:   time.Second,
			StreamOutput:    true,
		},
		Performance: PerformanceConfig{
			MemoryLimit:      50, // 50 MB
			CPULimit:         25, // 25%
			ProfilingEnabled: false,
			ProfilingPort:    6060,
		},
		Logging: LoggingConfig{
			Level:      "info",
			Format:     "text",
			File:       "",
			MaxSize:    10,
			MaxAge:     30,
			MaxBackups: 5,
			Compress:   true,
			LocalTime:  true,
		},
	}
}

func (o OutputConfig) ToManagerOptions() *output.ManagerOptions {
	oformat, err := strconv.Atoi(o.Format)
	if err != nil {
		fmt.Println(err)

	}
	return &output.ManagerOptions{
		// Fill these according to what OutputConfig contains
		Format:        output.OutputFormat(oformat),
		Colors:        o.Colors,
		File:          o.File,
		MaxHistory:    o.MaxHistoryLines,
		FlushInterval: o.FlushInterval,
	}
}

// Helper functions for default configurations

func getDefaultCacheConfig() CacheConfig {
	homeDir, _ := os.UserHomeDir()
	return CacheConfig{
		Enabled:   true,
		TTL:       "5m",
		MaxSize:   1000,
		Directory: filepath.Join(homeDir, ".cache", "wake"),
	}
}

func getDefaultMakeConfig() MakeConfig {
	return MakeConfig{
		Command:         "make",
		Args:            []string{},
		Parallel:        false,
		Jobs:            1,
		KeepGoing:       false,
		Silent:          false,
		DryRun:          false,
		LoadAverage:     0.0,
		Variables:       make(map[string]string),
		DefaultMakefile: "Makefile",
	}
}

func getDefaultNpmConfig() NpmConfig {
	return NpmConfig{
		Command:       "npm",
		Args:          []string{},
		Registry:      "",
		Silent:        false,
		Progress:      true,
		Production:    false,
		GlobalInstall: false,
		CacheDir:      "",
		OfflineMode:   false,
		LogLevel:      "info",
		IgnoreScripts: false,
		AuditLevel:    "moderate",
	}
}

func getDefaultYarnConfig() YarnConfig {
	return YarnConfig{
		Command:        "yarn",
		Args:           []string{},
		Registry:       "",
		Silent:         false,
		Verbose:        false,
		Production:     false,
		FrozenLockfile: false,
		OfflineMode:    false,
		CacheFolder:    "",
		NetworkTimeout: 30000,
		IgnoreScripts:  false,
		CheckFiles:     false,
	}
}

func getDefaultPnpmConfig() PnpmConfig {
	return PnpmConfig{
		Command:              "pnpm",
		Args:                 []string{},
		Registry:             "",
		Silent:               false,
		Production:           false,
		FrozenLockfile:       false,
		StoreDir:             "",
		NetworkConcurrency:   16,
		ChildConcurrency:     5,
		IgnoreScripts:        false,
		VerifyStoreIntegrity: false,
	}
}

func getDefaultBunConfig() BunConfig {
	return BunConfig{
		Command:       "bun",
		Args:          []string{},
		Registry:      "",
		Target:        "bun",
		Format:        "esm",
		MinifyOutput:  false,
		SourceMap:     false,
		Hot:           false,
		Watch:         false,
		IgnoreScripts: false,
	}
}

func getDefaultJustConfig() JustConfig {
	return JustConfig{
		Command:          "just",
		Args:             []string{},
		Shell:            getDefaultShell(),
		ShellArgs:        getDefaultShellArgs(),
		WorkingDirectory: "",
		DryRun:           false,
		Verbose:          false,
		Quiet:            false,
		Color:            "auto",
		Variables:        make(map[string]string),
		DotEnv:           []string{},
		SearchParents:    true,
	}
}

func getDefaultTaskConfig() TaskConfig {
	return TaskConfig{
		Command:     "task",
		Args:        []string{},
		Taskfile:    "Taskfile.yml",
		Directory:   "",
		DryRun:      false,
		Verbose:     false,
		Silent:      false,
		Parallel:    false,
		Continue:    false,
		Force:       false,
		Watch:       false,
		Color:       true,
		Summary:     false,
		List:        false,
		Concurrency: 0,
		Interval:    "",
		Variables:   make(map[string]string),
	}
}

func getDefaultPythonConfig() PythonConfig {
	return PythonConfig{
		Command:          "python",
		Args:             []string{},
		VirtualEnv:       "",
		Interactive:      false,
		Unbuffered:       true,
		Optimize:         0,
		Verbose:          false,
		Quiet:            false,
		PythonPath:       []string{},
		Environment:      make(map[string]string),
		AutoActivateVenv: true,
		DetectFrameworks: true,
	}
}

func getDefaultExcludePatterns() []string {
	patterns := []string{
		// Version control
		".git",
		".svn",
		".hg",
		".bzr",

		// Build directories
		"build",
		"dist",
		"target",
		"out",
		"bin",
		"obj",

		// Package manager directories
		"node_modules",
		"bower_components",
		"vendor",

		// Python
		"__pycache__",
		"*.pyc",
		"*.pyo",
		"*.egg-info",
		".venv",
		"venv",
		".env",

		// IDE and editor files
		".vscode",
		".idea",
		"*.swp",
		"*.swo",
		"*~",
		".DS_Store",
		"Thumbs.db",

		// Temporary files
		"tmp",
		"temp",
		"*.tmp",
		"*.bak",
		"*.log",
	}

	// Add platform-specific patterns
	if runtime.GOOS == "windows" {
		patterns = append(patterns, []string{
			"*.exe",
			"*.dll",
			"*.msi",
			"desktop.ini",
		}...)
	}

	return patterns
}

func getDefaultShell() string {
	if runtime.GOOS == "windows" {
		// Check for PowerShell first, then cmd
		if _, err := os.Stat("C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe"); err == nil {
			return "powershell"
		}
		return "cmd"
	}

	// Unix-like systems
	shell := os.Getenv("SHELL")
	if shell != "" {
		return shell
	}

	// Common defaults
	shells := []string{"/bin/bash", "/bin/sh", "/bin/zsh"}
	for _, shell := range shells {
		if _, err := os.Stat(shell); err == nil {
			return shell
		}
	}

	return "/bin/sh" // Fallback
}

func getDefaultShellArgs() []string {
	shell := getDefaultShell()

	switch {
	case strings.Contains(shell, "powershell"):
		return []string{"-Command"}
	case strings.Contains(shell, "cmd"):
		return []string{"/C"}
	case strings.Contains(shell, "bash"):
		return []string{"-c"}
	case strings.Contains(shell, "zsh"):
		return []string{"-c"}
	case strings.Contains(shell, "fish"):
		return []string{"-c"}
	default:
		return []string{"-c"}
	}
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	// Validate UI theme
	validThemes := []string{"dark", "light", "auto"}
	if !contains(validThemes, c.UI.Theme) {
		return fmt.Errorf("invalid theme: %s, must be one of: %v", c.UI.Theme, validThemes)
	}

	// Validate UI layout
	validLayouts := []string{"split", "tabs"}
	if !contains(validLayouts, c.UI.Layout) {
		return fmt.Errorf("invalid layout: %s, must be one of: %v", c.UI.Layout, validLayouts)
	}

	// Validate refresh rate
	if c.UI.RefreshRate < 1 || c.UI.RefreshRate > 1000 {
		return fmt.Errorf("invalid refresh rate: %d, must be between 1 and 1000", c.UI.RefreshRate)
	}

	// Validate sources max depth
	if c.Sources.MaxDepth < 1 || c.Sources.MaxDepth > 100 {
		return fmt.Errorf("invalid max depth: %d, must be between 1 and 100", c.Sources.MaxDepth)
	}

	// Validate discovery max concurrency
	if c.Discovery.MaxConcurrency < 1 || c.Discovery.MaxConcurrency > 100 {
		return fmt.Errorf("invalid max concurrency: %d, must be between 1 and 100", c.Discovery.MaxConcurrency)
	}

	// Validate output buffer size
	if c.Output.BufferSize < 100 || c.Output.BufferSize > 1000000 {
		return fmt.Errorf("invalid buffer size: %d, must be between 100 and 1000000", c.Output.BufferSize)
	}

	// Validate performance limits
	if c.Performance.MemoryLimit < 1 || c.Performance.MemoryLimit > 1024 {
		return fmt.Errorf("invalid memory limit: %d, must be between 1 and 1024 MB", c.Performance.MemoryLimit)
	}

	if c.Performance.CPULimit < 1 || c.Performance.CPULimit > 100 {
		return fmt.Errorf("invalid CPU limit: %d, must be between 1 and 100%%", c.Performance.CPULimit)
	}

	return nil
}

// Merge combines this config with another, with the other taking precedence
func (c *Config) Merge(other *Config) {
	if other == nil {
		return
	}

	// Merge UI config
	if other.UI.Theme != "" {
		c.UI.Theme = other.UI.Theme
	}
	if other.UI.Layout != "" {
		c.UI.Layout = other.UI.Layout
	}
	if other.UI.RefreshRate > 0 {
		c.UI.RefreshRate = other.UI.RefreshRate
	}
	c.UI.ShowIcons = other.UI.ShowIcons
	c.UI.ShowDescriptions = other.UI.ShowDescriptions
	c.UI.MouseEnabled = other.UI.MouseEnabled

	// Merge colors
	maps.Copy(c.UI.Colors, other.UI.Colors)

	// Merge keybindings (replace entirely if specified)
	if len(other.Keybindings.Quit) > 0 {
		c.Keybindings.Quit = other.Keybindings.Quit
	}
	// Add other keybinding merges as needed...

	// Merge sources
	if len(other.Sources.Enabled) > 0 {
		c.Sources.Enabled = other.Sources.Enabled
	}
	if len(other.Sources.Disabled) > 0 {
		c.Sources.Disabled = other.Sources.Disabled
	}
	if other.Sources.MaxDepth > 0 {
		c.Sources.MaxDepth = other.Sources.MaxDepth
	}

	// Merge custom patterns
	maps.Copy(c.Sources.CustomPatterns, other.Sources.CustomPatterns)
}

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	return slices.Contains(slice, item)
}
