package utils

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// EnvVar represents an environment variable with metadata
type EnvVar struct {
	Name         string `json:"name"`
	Value        string `json:"value"`
	DefaultValue string `json:"default_value,omitempty"`
	Description  string `json:"description,omitempty"`
	Required     bool   `json:"required"`
	Sensitive    bool   `json:"sensitive"`
	Type         string `json:"type"` // string, int, bool, duration, etc.
}

// EnvManager manages environment variables
type EnvManager struct {
	vars          map[string]*EnvVar
	overrides     map[string]string
	prefix        string
	delimiter     string
	caseSensitive bool
}

// EnvSnapshot represents a snapshot of environment variables
type EnvSnapshot struct {
	Variables map[string]string `json:"variables"`
	Timestamp time.Time         `json:"timestamp"`
	Source    string            `json:"source"`
}

// EnvDiff represents differences between environment snapshots
type EnvDiff struct {
	Added   map[string]string `json:"added"`
	Removed map[string]string `json:"removed"`
	Changed map[string]Change `json:"changed"`
}

// Change represents a change in environment variable value
type Change struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// EnvFile represents a .env file
type EnvFile struct {
	Path      string            `json:"path"`
	Variables map[string]string `json:"variables"`
	Comments  map[string]string `json:"comments"`
}

// NewEnvManager creates a new environment manager
func NewEnvManager(prefix string) *EnvManager {
	return &EnvManager{
		vars:          make(map[string]*EnvVar),
		overrides:     make(map[string]string),
		prefix:        prefix,
		delimiter:     "_",
		caseSensitive: true,
	}
}

// Register registers an environment variable
func (em *EnvManager) Register(name, description, defaultValue string, required bool) *EnvVar {
	envVar := &EnvVar{
		Name:         name,
		Description:  description,
		DefaultValue: defaultValue,
		Required:     required,
		Type:         "string",
	}

	em.vars[name] = envVar
	return envVar
}

// RegisterInt registers an integer environment variable
func (em *EnvManager) RegisterInt(name, description string, defaultValue int, required bool) *EnvVar {
	envVar := em.Register(name, description, strconv.Itoa(defaultValue), required)
	envVar.Type = "int"
	return envVar
}

// RegisterBool registers a boolean environment variable
func (em *EnvManager) RegisterBool(name, description string, defaultValue bool, required bool) *EnvVar {
	envVar := em.Register(name, description, strconv.FormatBool(defaultValue), required)
	envVar.Type = "bool"
	return envVar
}

// RegisterDuration registers a duration environment variable
func (em *EnvManager) RegisterDuration(name, description string, defaultValue time.Duration, required bool) *EnvVar {
	envVar := em.Register(name, description, defaultValue.String(), required)
	envVar.Type = "duration"
	return envVar
}

// SetSensitive marks an environment variable as sensitive
func (em *EnvManager) SetSensitive(name string, sensitive bool) {
	if envVar, exists := em.vars[name]; exists {
		envVar.Sensitive = sensitive
	}
}

// Get retrieves an environment variable value
func (em *EnvManager) Get(name string) (string, bool) {
	// Check overrides first
	if value, exists := em.overrides[name]; exists {
		return value, true
	}

	// Build full name with prefix
	fullName := em.buildFullName(name)

	// Get from environment
	if value, exists := os.LookupEnv(fullName); exists {
		return value, true
	}

	// Try without prefix
	if value, exists := os.LookupEnv(name); exists {
		return value, true
	}

	// Return default value if registered
	if envVar, exists := em.vars[name]; exists && envVar.DefaultValue != "" {
		return envVar.DefaultValue, true
	}

	return "", false
}

// GetString retrieves a string environment variable
func (em *EnvManager) GetString(name string) string {
	if value, exists := em.Get(name); exists {
		return value
	}
	return ""
}

// GetInt retrieves an integer environment variable
func (em *EnvManager) GetInt(name string) (int, error) {
	value, exists := em.Get(name)
	if !exists {
		return 0, fmt.Errorf("environment variable %s not found", name)
	}

	return strconv.Atoi(value)
}

// GetBool retrieves a boolean environment variable
func (em *EnvManager) GetBool(name string) (bool, error) {
	value, exists := em.Get(name)
	if !exists {
		return false, fmt.Errorf("environment variable %s not found", name)
	}

	// Handle various boolean representations
	switch strings.ToLower(value) {
	case "true", "1", "yes", "on", "enable", "enabled":
		return true, nil
	case "false", "0", "no", "off", "disable", "disabled":
		return false, nil
	default:
		return strconv.ParseBool(value)
	}
}

// GetDuration retrieves a duration environment variable
func (em *EnvManager) GetDuration(name string) (time.Duration, error) {
	value, exists := em.Get(name)
	if !exists {
		return 0, fmt.Errorf("environment variable %s not found", name)
	}

	return time.ParseDuration(value)
}

// GetSlice retrieves a slice environment variable (comma-separated)
func (em *EnvManager) GetSlice(name string, separator string) []string {
	value, exists := em.Get(name)
	if !exists || value == "" {
		return []string{}
	}

	if separator == "" {
		separator = ","
	}

	parts := strings.Split(value, separator)
	result := make([]string, 0, len(parts))

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}

// Set sets an environment variable override
func (em *EnvManager) Set(name, value string) {
	em.overrides[name] = value
}

// Unset removes an environment variable override
func (em *EnvManager) Unset(name string) {
	delete(em.overrides, name)
}

// Validate validates all registered environment variables
func (em *EnvManager) Validate() error {
	var errors []string

	for name, envVar := range em.vars {
		value, exists := em.Get(name)

		// Check required variables
		if envVar.Required && (!exists || value == "") {
			errors = append(errors, fmt.Sprintf("required environment variable %s is not set", name))
			continue
		}

		// Skip validation if variable is not set and not required
		if !exists {
			continue
		}

		// Type validation
		switch envVar.Type {
		case "int":
			if _, err := strconv.Atoi(value); err != nil {
				errors = append(errors, fmt.Sprintf("environment variable %s must be an integer", name))
			}
		case "bool":
			if _, err := em.GetBool(name); err != nil {
				errors = append(errors, fmt.Sprintf("environment variable %s must be a boolean", name))
			}
		case "duration":
			if _, err := time.ParseDuration(value); err != nil {
				errors = append(errors, fmt.Sprintf("environment variable %s must be a valid duration", name))
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("environment validation failed:\n%s", strings.Join(errors, "\n"))
	}

	return nil
}

// GetAll returns all environment variables with their values
func (em *EnvManager) GetAll() map[string]string {
	result := make(map[string]string)

	// Add system environment variables
	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}

	// Apply overrides
	for name, value := range em.overrides {
		fullName := em.buildFullName(name)
		result[fullName] = value
	}

	return result
}

// GetRegistered returns all registered environment variables
func (em *EnvManager) GetRegistered() map[string]*EnvVar {
	result := make(map[string]*EnvVar)

	for name, envVar := range em.vars {
		// Create a copy with current value
		copy := *envVar
		if value, exists := em.Get(name); exists {
			copy.Value = value
		}
		result[name] = &copy
	}

	return result
}

// Export exports environment variables as shell commands
func (em *EnvManager) Export(shell string) []string {
	var commands []string

	vars := em.GetRegistered()
	names := make([]string, 0, len(vars))
	for name := range vars {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		envVar := vars[name]
		if envVar.Value == "" {
			continue
		}

		fullName := em.buildFullName(name)

		switch shell {
		case "bash", "zsh", "sh":
			commands = append(commands, fmt.Sprintf("export %s=%s", fullName, ShellEscape(envVar.Value)))
		case "fish":
			commands = append(commands, fmt.Sprintf("set -x %s %s", fullName, ShellEscape(envVar.Value)))
		case "powershell":
			commands = append(commands, fmt.Sprintf("$env:%s = %s", fullName, PowerShellEscape(envVar.Value)))
		case "cmd":
			commands = append(commands, fmt.Sprintf("set %s=%s", fullName, envVar.Value))
		default:
			commands = append(commands, fmt.Sprintf("export %s=%s", fullName, ShellEscape(envVar.Value)))
		}
	}

	return commands
}

// buildFullName builds the full environment variable name with prefix
func (em *EnvManager) buildFullName(name string) string {
	if em.prefix == "" {
		return name
	}

	if !em.caseSensitive {
		name = strings.ToUpper(name)
	}

	return em.prefix + em.delimiter + name
}

// Environment utility functions

// GetEnv gets an environment variable with a default value
func GetEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// GetEnvInt gets an integer environment variable with a default value
func GetEnvInt(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intValue, err := strconv.Atoi(value); err == nil {
			return intValue
		}
	}
	return defaultValue
}

// GetEnvBool gets a boolean environment variable with a default value
func GetEnvBool(key string, defaultValue bool) bool {
	if value, exists := os.LookupEnv(key); exists {
		switch strings.ToLower(value) {
		case "true", "1", "yes", "on":
			return true
		case "false", "0", "no", "off":
			return false
		}
	}
	return defaultValue
}

// GetEnvDuration gets a duration environment variable with a default value
func GetEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value, exists := os.LookupEnv(key); exists {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

// IsEnvSet checks if an environment variable is set
func IsEnvSet(key string) bool {
	_, exists := os.LookupEnv(key)
	return exists
}

// SetEnv sets an environment variable
func SetEnv(key, value string) error {
	return os.Setenv(key, value)
}

// UnsetEnv unsets an environment variable
func UnsetEnv(key string) error {
	return os.Unsetenv(key)
}

// ClearEnv clears all environment variables
func ClearEnv() {
	os.Clearenv()
}

// CreateSnapshot creates a snapshot of current environment variables
func CreateSnapshot(source string) *EnvSnapshot {
	vars := make(map[string]string)

	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			vars[parts[0]] = parts[1]
		}
	}

	return &EnvSnapshot{
		Variables: vars,
		Timestamp: time.Now(),
		Source:    source,
	}
}

// CompareSnapshots compares two environment snapshots
func CompareSnapshots(before, after *EnvSnapshot) *EnvDiff {
	diff := &EnvDiff{
		Added:   make(map[string]string),
		Removed: make(map[string]string),
		Changed: make(map[string]Change),
	}

	// Find added and changed variables
	for name, afterValue := range after.Variables {
		if beforeValue, exists := before.Variables[name]; exists {
			if beforeValue != afterValue {
				diff.Changed[name] = Change{From: beforeValue, To: afterValue}
			}
		} else {
			diff.Added[name] = afterValue
		}
	}

	// Find removed variables
	for name, beforeValue := range before.Variables {
		if _, exists := after.Variables[name]; !exists {
			diff.Removed[name] = beforeValue
		}
	}

	return diff
}

// LoadEnvFile loads environment variables from a .env file
func LoadEnvFile(path string) (*EnvFile, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read env file: %w", err)
	}

	envFile := &EnvFile{
		Path:      path,
		Variables: make(map[string]string),
		Comments:  make(map[string]string),
	}

	lines := strings.Split(string(content), "\n")
	var currentComment string

	for _, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines
		if line == "" {
			currentComment = ""
			continue
		}

		// Handle comments
		if strings.HasPrefix(line, "#") {
			currentComment = strings.TrimSpace(line[1:])
			continue
		}

		// Parse key=value pairs
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes if present
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		// Expand variables in value
		value = ExpandEnvVars(value)

		envFile.Variables[key] = value

		if currentComment != "" {
			envFile.Comments[key] = currentComment
			currentComment = ""
		}
	}

	return envFile, nil
}

// ApplyEnvFile applies environment variables from an env file
func ApplyEnvFile(envFile *EnvFile, overwrite bool) error {
	for key, value := range envFile.Variables {
		// Check if variable already exists
		if _, exists := os.LookupEnv(key); exists && !overwrite {
			continue
		}

		if err := os.Setenv(key, value); err != nil {
			return fmt.Errorf("failed to set environment variable %s: %w", key, err)
		}
	}

	return nil
}

// SaveEnvFile saves environment variables to a .env file
func SaveEnvFile(path string, vars map[string]string, comments map[string]string) error {
	var lines []string

	// Sort keys for consistent output
	keys := make([]string, 0, len(vars))
	for key := range vars {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		value := vars[key]

		// Add comment if available
		if comment, exists := comments[key]; exists {
			lines = append(lines, "# "+comment)
		}

		// Escape value if needed
		if strings.ContainsAny(value, " \t\n\"'$") {
			value = fmt.Sprintf("\"%s\"", strings.ReplaceAll(value, "\"", "\\\""))
		}

		lines = append(lines, fmt.Sprintf("%s=%s", key, value))
		lines = append(lines, "") // Empty line for readability
	}

	content := strings.Join(lines, "\n")
	return os.WriteFile(path, []byte(content), 0644)
}

// ExpandEnvVars expands environment variables in a string
func ExpandEnvVars(s string) string {
	return os.ExpandEnv(s)
}

// FilterEnvVars filters environment variables by prefix
func FilterEnvVars(prefix string) map[string]string {
	result := make(map[string]string)

	for _, env := range os.Environ() {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 && strings.HasPrefix(parts[0], prefix) {
			result[parts[0]] = parts[1]
		}
	}

	return result
}

// StripEnvPrefix removes prefix from environment variable names
func StripEnvPrefix(vars map[string]string, prefix string) map[string]string {
	result := make(map[string]string)

	for key, value := range vars {
		if strings.HasPrefix(key, prefix) {
			newKey := strings.TrimPrefix(key, prefix)
			newKey = strings.TrimPrefix(newKey, "_") // Remove separator
			result[newKey] = value
		} else {
			result[key] = value
		}
	}

	return result
}

// ValidateEnvVarName validates an environment variable name
func ValidateEnvVarName(name string) error {
	if name == "" {
		return fmt.Errorf("environment variable name cannot be empty")
	}

	// Check if name starts with a letter or underscore
	if !regexp.MustCompile(`^[a-zA-Z_]`).MatchString(name) {
		return fmt.Errorf("environment variable name must start with a letter or underscore")
	}

	// Check if name contains only valid characters
	if !regexp.MustCompile(`^[a-zA-Z0-9_]+$`).MatchString(name) {
		return fmt.Errorf("environment variable name can only contain letters, numbers, and underscores")
	}

	return nil
}

// Shell escaping functions

// ShellEscape escapes a value for shell usage
func ShellEscape(value string) string {
	// Simple escaping - wrap in single quotes and escape single quotes
	escaped := strings.ReplaceAll(value, "'", "'\"'\"'")
	return "'" + escaped + "'"
}

// PowerShellEscape escapes a value for PowerShell usage
func PowerShellEscape(value string) string {
	// Escape backticks and quotes
	escaped := strings.ReplaceAll(value, "`", "``")
	escaped = strings.ReplaceAll(escaped, "\"", "`\"")
	return "\"" + escaped + "\""
}

// GetUserEnvironment returns user-specific environment variables
func GetUserEnvironment() map[string]string {
	userVars := make(map[string]string)

	// Common user environment variables
	userEnvVars := []string{
		"HOME", "USER", "USERNAME", "USERPROFILE",
		"PATH", "SHELL", "TERM", "LANG", "LC_ALL",
		"XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_CACHE_HOME",
		"APPDATA", "LOCALAPPDATA",
	}

	for _, varName := range userEnvVars {
		if value, exists := os.LookupEnv(varName); exists {
			userVars[varName] = value
		}
	}

	return userVars
}

// GetSystemEnvironment returns system-specific environment variables
func GetSystemEnvironment() map[string]string {
	systemVars := make(map[string]string)

	// Common system environment variables
	systemEnvVars := []string{
		"OS", "OSTYPE", "HOSTTYPE", "MACHTYPE",
		"COMPUTERNAME", "HOSTNAME",
		"PROCESSOR_ARCHITECTURE", "PROCESSOR_IDENTIFIER",
		"NUMBER_OF_PROCESSORS",
		"SYSTEMROOT", "WINDIR", "PROGRAMFILES",
		"TMP", "TEMP", "TMPDIR",
	}

	for _, varName := range systemEnvVars {
		if value, exists := os.LookupEnv(varName); exists {
			systemVars[varName] = value
		}
	}

	return systemVars
}

// MergeEnvironments merges multiple environment variable maps
func MergeEnvironments(envMaps ...map[string]string) map[string]string {
	result := make(map[string]string)

	for _, envMap := range envMaps {
		for key, value := range envMap {
			result[key] = value
		}
	}

	return result
}

// GetEnvInfo returns information about the environment
func GetEnvInfo() map[string]any {
	return map[string]any{
		"total_variables":  len(os.Environ()),
		"user_variables":   len(GetUserEnvironment()),
		"system_variables": len(GetSystemEnvironment()),
		"path_entries":     len(GetEnvSlice("PATH", ":")),
		"shell":            GetEnv("SHELL", "unknown"),
		"home_directory":   GetEnv("HOME", GetEnv("USERPROFILE", "unknown")),
		"temp_directory":   GetEnv("TMPDIR", GetEnv("TEMP", GetEnv("TMP", "/tmp"))),
	}
}

// GetEnvSlice gets an environment variable as a slice
func GetEnvSlice(key, separator string) []string {
	value := GetEnv(key, "")
	if value == "" {
		return []string{}
	}

	parts := strings.Split(value, separator)
	result := make([]string, 0, len(parts))

	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}
