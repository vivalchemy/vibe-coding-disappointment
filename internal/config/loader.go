package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	// Global config file name
	GlobalConfigFile = "config.toml"

	// Project config file name
	ProjectConfigFile = ".wakeconfig.toml"

	// Legacy project config file name
	LegacyProjectConfigFile = ".wake.toml"

	// Config directory name
	ConfigDirName = "wake"
)

// Loader handles configuration loading and discovery
type Loader struct {
	// Override paths for testing
	homeDir     string
	currentDir  string
	configPaths []string
}

// NewLoader creates a new configuration loader
func NewLoader() *Loader {
	homeDir, _ := os.UserHomeDir()
	currentDir, _ := os.Getwd()

	return &Loader{
		homeDir:    homeDir,
		currentDir: currentDir,
	}
}

// NewLoaderWithPaths creates a loader with custom paths (for testing)
func NewLoaderWithPaths(homeDir, currentDir string, configPaths ...string) *Loader {
	return &Loader{
		homeDir:     homeDir,
		currentDir:  currentDir,
		configPaths: configPaths,
	}
}

// Load discovers and loads the configuration with proper precedence
func Load() (*Config, error) {
	loader := NewLoader()
	return loader.LoadConfig()
}

// LoadConfig discovers and loads configuration files
func (l *Loader) LoadConfig() (*Config, error) {
	// Start with default configuration
	config := Default()

	// Discover configuration files in order of precedence (lowest to highest)
	configFiles := l.discoverConfigFiles()

	// Load and merge each configuration file
	for _, configFile := range configFiles {
		if err := l.loadAndMergeConfig(config, configFile); err != nil {
			return nil, fmt.Errorf("failed to load config from %s: %w", configFile, err)
		}
	}

	// Validate the final configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return config, nil
}

// LoadFromFile loads configuration from a specific file
func (l *Loader) LoadFromFile(filename string) (*Config, error) {
	config := Default()

	if err := l.loadAndMergeConfig(config, filename); err != nil {
		return nil, fmt.Errorf("failed to load config from %s: %w", filename, err)
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return config, nil
}

// discoverConfigFiles finds all configuration files in order of precedence
func (l *Loader) discoverConfigFiles() []string {
	var configFiles []string

	// If custom config paths are provided (for testing), use those
	if len(l.configPaths) > 0 {
		for _, path := range l.configPaths {
			if l.fileExists(path) {
				configFiles = append(configFiles, path)
			}
		}
		return configFiles
	}

	// 1. Global configuration file (lowest precedence)
	globalConfig := l.getGlobalConfigPath()
	if l.fileExists(globalConfig) {
		configFiles = append(configFiles, globalConfig)
	}

	// 2. Project-specific configuration files (higher precedence)
	projectConfigs := l.findProjectConfigs()
	configFiles = append(configFiles, projectConfigs...)

	return configFiles
}

// getGlobalConfigPath returns the path to the global configuration file
func (l *Loader) getGlobalConfigPath() string {
	// Try XDG config directory first
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		return filepath.Join(xdgConfig, ConfigDirName, GlobalConfigFile)
	}

	// Fall back to ~/.config/wake/config.toml
	return filepath.Join(l.homeDir, ".config", ConfigDirName, GlobalConfigFile)
}

// findProjectConfigs finds project-specific configuration files
func (l *Loader) findProjectConfigs() []string {
	var configs []string

	// Start from current directory and walk up to find project configs
	dir := l.currentDir

	for {
		// Check for project config files in current directory
		projectConfig := filepath.Join(dir, ProjectConfigFile)
		if l.fileExists(projectConfig) {
			configs = append([]string{projectConfig}, configs...) // Prepend for correct precedence
		}

		// Check for legacy config file
		legacyConfig := filepath.Join(dir, LegacyProjectConfigFile)
		if l.fileExists(legacyConfig) {
			configs = append([]string{legacyConfig}, configs...) // Prepend for correct precedence
		}

		// Move up one directory
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			break
		}
		dir = parent

		// Stop at home directory to avoid going too far up
		if dir == l.homeDir {
			break
		}
	}

	return configs
}

// loadAndMergeConfig loads a config file and merges it with the base config
func (l *Loader) loadAndMergeConfig(baseConfig *Config, filename string) error {
	// Read the file
	data, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Parse TOML
	var fileConfig Config
	if err := toml.Unmarshal(data, &fileConfig); err != nil {
		return fmt.Errorf("failed to parse TOML: %w", err)
	}

	// Merge with base configuration
	baseConfig.Merge(&fileConfig)

	return nil
}

// fileExists checks if a file exists and is readable
func (l *Loader) fileExists(filename string) bool {
	info, err := os.Stat(filename)
	if err != nil {
		return false
	}
	return !info.IsDir()
}

// GetProjectRoot attempts to find the project root directory
func (l *Loader) GetProjectRoot() (string, error) {
	dir := l.currentDir

	for {
		// Check for project indicators
		indicators := []string{
			ProjectConfigFile,
			LegacyProjectConfigFile,
			".git",
			"go.mod",
			"package.json",
			"Cargo.toml",
			"pyproject.toml",
		}

		for _, indicator := range indicators {
			path := filepath.Join(dir, indicator)
			if l.fileExists(path) || l.dirExists(path) {
				return dir, nil
			}
		}

		// Move up one directory
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			break
		}
		dir = parent

		// Stop at home directory
		if dir == l.homeDir {
			break
		}
	}

	// Default to current directory if no project root found
	return l.currentDir, nil
}

// dirExists checks if a directory exists
func (l *Loader) dirExists(dirname string) bool {
	info, err := os.Stat(dirname)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// CreateGlobalConfig creates a global configuration file with defaults
func (l *Loader) CreateGlobalConfig() error {
	configPath := l.getGlobalConfigPath()
	configDir := filepath.Dir(configPath)

	// Create config directory if it doesn't exist
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	// Check if config file already exists
	if l.fileExists(configPath) {
		return fmt.Errorf("global config file already exists: %s", configPath)
	}

	// Create default config
	config := Default()

	// Write config to file
	if err := l.WriteConfigToFile(config, configPath); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// CreateProjectConfig creates a project-specific configuration file
func (l *Loader) CreateProjectConfig(dir string) error {
	if dir == "" {
		dir = l.currentDir
	}

	configPath := filepath.Join(dir, ProjectConfigFile)

	// Check if config file already exists
	if l.fileExists(configPath) {
		return fmt.Errorf("project config file already exists: %s", configPath)
	}

	// Create minimal project config
	config := &Config{
		Sources: SourcesConfig{
			Enabled: []string{"make", "npm", "just", "task"},
		},
		UI: &UIConfig{
			Theme: "dark",
		},
	}

	// Write config to file
	if err := l.WriteConfigToFile(config, configPath); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
}

// WriteConfigToFile writes a configuration to a TOML file
func (l *Loader) WriteConfigToFile(config *Config, filename string) error {
	// Create directory if it doesn't exist
	dir := filepath.Dir(filename)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Create or truncate the file
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Write header comment
	header := generateConfigHeader()
	if _, err := file.WriteString(header); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// Encode config as TOML
	encoder := toml.NewEncoder(file)
	if err := encoder.Encode(config); err != nil {
		return fmt.Errorf("failed to encode TOML: %w", err)
	}

	return nil
}

// generateConfigHeader generates a header comment for config files
func generateConfigHeader() string {
	return `# Wake Universal Task Runner Configuration
# This file configures the behavior of Wake.
# See https://github.com/vivalchemy/wake for documentation.

`
}

// GetConfigPaths returns all discovered configuration file paths
func (l *Loader) GetConfigPaths() []string {
	return l.discoverConfigFiles()
}

// ValidateConfigFile validates a configuration file without loading it
func (l *Loader) ValidateConfigFile(filename string) error {
	// Check if file exists
	if !l.fileExists(filename) {
		return fmt.Errorf("config file does not exist: %s", filename)
	}

	// Try to load and validate
	_, err := l.LoadFromFile(filename)
	return err
}

// MergeConfigs merges multiple configuration files
func (l *Loader) MergeConfigs(filenames ...string) (*Config, error) {
	config := Default()

	for _, filename := range filenames {
		if l.fileExists(filename) {
			if err := l.loadAndMergeConfig(config, filename); err != nil {
				return nil, fmt.Errorf("failed to merge config from %s: %w", filename, err)
			}
		}
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("merged configuration validation failed: %w", err)
	}

	return config, nil
}

// ExportConfig exports the current configuration to a file
func (l *Loader) ExportConfig(filename string) error {
	config, err := l.LoadConfig()
	if err != nil {
		return fmt.Errorf("failed to load current config: %w", err)
	}

	return l.WriteConfigToFile(config, filename)
}

// ListConfigFiles returns information about discovered config files
func (l *Loader) ListConfigFiles() []ConfigFileInfo {
	configFiles := l.discoverConfigFiles()
	var info []ConfigFileInfo

	for _, file := range configFiles {
		fileInfo := ConfigFileInfo{
			Path:   file,
			Exists: l.fileExists(file),
		}

		// Determine type
		if strings.Contains(file, ".config") || strings.Contains(file, "XDG") {
			fileInfo.Type = "global"
		} else {
			fileInfo.Type = "project"
		}

		// Get file size
		if stat, err := os.Stat(file); err == nil {
			fileInfo.Size = stat.Size()
		}

		info = append(info, fileInfo)
	}

	return info
}

// ConfigFileInfo contains information about a configuration file
type ConfigFileInfo struct {
	Path   string `json:"path"`
	Type   string `json:"type"` // "global" or "project"
	Exists bool   `json:"exists"`
	Size   int64  `json:"size"`
}
