package parsers

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/vivalchemy/wake/pkg/logger"
	"github.com/vivalchemy/wake/pkg/task"
	"slices"
)

var _ Parser = (*TaskfileParser)(nil)

// TaskfileParser parses Taskfile.yml task definitions
type TaskfileParser struct {
	logger  logger.Logger
	options TaskfileOptions
}

// TaskfileOptions contains configuration for Taskfile parsing
type TaskfileOptions struct {
	// File matching
	FileNames   []string
	MaxFileSize int64

	// Parsing options
	ParseIncludes  bool
	ParseVars      bool
	ParseEnv       bool
	ValidateSchema bool

	// Content options
	IncludeCmds bool
	MaxCmdLines int
	ParseDeps   bool
}

// TaskfileRoot represents the root structure of a Taskfile
type TaskfileRoot struct {
	Version  string                   `yaml:"version"`
	Output   string                   `yaml:"output,omitempty"`
	Method   string                   `yaml:"method,omitempty"`
	Includes map[string]any           `yaml:"includes,omitempty"`
	Vars     map[string]any           `yaml:"vars,omitempty"`
	Env      map[string]any           `yaml:"env,omitempty"`
	Tasks    map[string]*TaskfileTask `yaml:"tasks"`
	Silent   bool                     `yaml:"silent,omitempty"`
	Dotenv   []string                 `yaml:"dotenv,omitempty"`
	Run      string                   `yaml:"run,omitempty"`
	Interval string                   `yaml:"interval,omitempty"`
}

// TaskfileTask represents a single task in a Taskfile
type TaskfileTask struct {
	// Basic properties
	Desc    string   `yaml:"desc,omitempty"`
	Summary string   `yaml:"summary,omitempty"`
	Aliases []string `yaml:"aliases,omitempty"`

	// Execution
	Cmds          []any                  `yaml:"cmds,omitempty"`
	Deps          []any                  `yaml:"deps,omitempty"`
	Preconditions []TaskfilePrecondition `yaml:"preconditions,omitempty"`

	// Configuration
	Silent      *bool  `yaml:"silent,omitempty"`
	Method      string `yaml:"method,omitempty"`
	Prefix      string `yaml:"prefix,omitempty"`
	IgnoreError bool   `yaml:"ignore_error,omitempty"`
	Run         string `yaml:"run,omitempty"`

	// Environment and variables
	Vars map[string]any `yaml:"vars,omitempty"`
	Env  map[string]any `yaml:"env,omitempty"`

	// File operations
	Sources   []string `yaml:"sources,omitempty"`
	Generates []string `yaml:"generates,omitempty"`
	Status    []string `yaml:"status,omitempty"`

	// Control
	Dir   string   `yaml:"dir,omitempty"`
	Set   []string `yaml:"set,omitempty"`
	Shopt []string `yaml:"shopt,omitempty"`

	// Internal tracking
	Internal bool `yaml:"internal,omitempty"`

	// Metadata (not from YAML)
	LineNumber int
	TaskName   string
}

// TaskfilePrecondition represents a task precondition
type TaskfilePrecondition struct {
	Sh  string `yaml:"sh,omitempty"`
	Msg string `yaml:"msg,omitempty"`
}

// TaskfileCommand represents different command formats
type TaskfileCommand struct {
	Cmd         string
	Task        string
	Defer       string
	Silent      bool
	IgnoreError bool
	Vars        map[string]any
}

// NewTaskfileParser creates a new Taskfile parser
func NewTaskfileParser(logger logger.Logger) *TaskfileParser {
	return &TaskfileParser{
		logger:  logger.WithGroup("taskfile-parser"),
		options: createDefaultTaskfileOptions(),
	}
}

// createDefaultTaskfileOptions creates default parsing options
func createDefaultTaskfileOptions() TaskfileOptions {
	return TaskfileOptions{
		FileNames: []string{
			"Taskfile.yml",
			"Taskfile.yaml",
			"taskfile.yml",
			"taskfile.yaml",
			"Taskfile.dist.yml",
			"Taskfile.dist.yaml",
		},
		MaxFileSize:    10 * 1024 * 1024, // 10MB
		ParseIncludes:  true,
		ParseVars:      true,
		ParseEnv:       true,
		ValidateSchema: true,
		IncludeCmds:    true,
		MaxCmdLines:    50,
		ParseDeps:      true,
	}
}

func (tp *TaskfileParser) GetRunnerType() task.RunnerType {
	return task.RunnerTask
}

func (tp *TaskfileParser) Parse(filePath string) ([]*task.Task, error) {
	return tp.ParseFile(filePath)
}

// SetOptions sets parsing options
func (tp *TaskfileParser) SetOptions(options TaskfileOptions) {
	tp.options = options
}

// GetSupportedFiles returns supported file patterns
func (tp *TaskfileParser) GetSupportedFiles() []string {
	return tp.options.FileNames
}

// CanParse checks if the parser can handle the given file
func (tp *TaskfileParser) CanParse(filePath string) bool {
	fileName := filepath.Base(filePath)

	return slices.Contains(tp.options.FileNames, fileName)
}

// ParseFile parses a Taskfile and returns discovered tasks
func (tp *TaskfileParser) ParseFile(filePath string) ([]*task.Task, error) {
	tp.logger.Debug("Parsing Taskfile", "file", filePath)

	// Check if we can parse this file
	if !tp.CanParse(filePath) {
		return nil, fmt.Errorf("unsupported file: %s", filePath)
	}

	// Check file size
	if err := tp.checkFileSize(filePath); err != nil {
		return nil, err
	}

	// Open and read file
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Get file info
	fileInfo, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to get file info: %w", err)
	}

	// Parse the file
	tasks, err := tp.parseContent(file, filePath, fileInfo.ModTime())
	if err != nil {
		return nil, fmt.Errorf("failed to parse content: %w", err)
	}

	tp.logger.Info("Parsed Taskfile", "file", filePath, "tasks", len(tasks))
	return tasks, nil
}

// ParseContent parses Taskfile content from a reader
func (tp *TaskfileParser) ParseContent(reader io.Reader, filePath string) ([]*task.Task, error) {
	return tp.parseContent(reader, filePath, time.Now())
}

// checkFileSize checks if file size is within limits
func (tp *TaskfileParser) checkFileSize(filePath string) error {
	if tp.options.MaxFileSize <= 0 {
		return nil
	}

	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return err
	}

	if fileInfo.Size() > tp.options.MaxFileSize {
		return fmt.Errorf("file too large: %d bytes (max: %d)", fileInfo.Size(), tp.options.MaxFileSize)
	}

	return nil
}

// parseContent parses the actual YAML content
func (tp *TaskfileParser) parseContent(reader io.Reader, filePath string, modTime time.Time) ([]*task.Task, error) {
	// Read all content
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read content: %w", err)
	}

	// Parse YAML
	var taskfile TaskfileRoot
	if err := yaml.Unmarshal(content, &taskfile); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Validate schema if enabled
	if tp.options.ValidateSchema {
		if err := tp.validateTaskfile(&taskfile); err != nil {
			return nil, fmt.Errorf("schema validation failed: %w", err)
		}
	}

	// Add line numbers to tasks
	tp.addLineNumbers(&taskfile, content)

	// Convert to generic tasks
	tasks := tp.convertToTasks(&taskfile, filePath, modTime)

	return tasks, nil
}

// validateTaskfile validates the Taskfile schema
func (tp *TaskfileParser) validateTaskfile(taskfile *TaskfileRoot) error {
	// Check version
	if taskfile.Version == "" {
		return fmt.Errorf("version is required")
	}

	// Validate version format
	validVersions := []string{"2", "3"}
	isValidVersion := slices.Contains(validVersions, taskfile.Version)

	if !isValidVersion {
		tp.logger.Warn("Unsupported Taskfile version", "version", taskfile.Version)
	}

	// Check that we have tasks
	if len(taskfile.Tasks) == 0 {
		return fmt.Errorf("no tasks defined")
	}

	// Validate each task
	for taskName, taskDef := range taskfile.Tasks {
		if err := tp.validateTask(taskName, taskDef); err != nil {
			return fmt.Errorf("task '%s': %w", taskName, err)
		}
	}

	return nil
}

// validateTask validates a single task definition
func (tp *TaskfileParser) validateTask(taskName string, taskDef *TaskfileTask) error {
	// Task name validation
	if taskName == "" {
		return fmt.Errorf("task name cannot be empty")
	}

	// Check for invalid characters in task name
	invalidChars := regexp.MustCompile(`[^\w\-:.]`)
	if invalidChars.MatchString(taskName) {
		tp.logger.Warn("Task name contains potentially problematic characters", "task", taskName)
	}

	// Validate commands and dependencies exist
	if len(taskDef.Cmds) == 0 && len(taskDef.Deps) == 0 {
		tp.logger.Warn("Task has no commands or dependencies", "task", taskName)
	}

	return nil
}

// addLineNumbers adds line numbers to tasks by parsing the raw YAML
func (tp *TaskfileParser) addLineNumbers(taskfile *TaskfileRoot, content []byte) {
	lines := strings.Split(string(content), "\n")

	// Look for tasks section
	inTasksSection := false

	for lineNum, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// Check if we're entering the tasks section
		if trimmedLine == "tasks:" {
			inTasksSection = true
			continue
		}

		if !inTasksSection {
			continue
		}

		// Check if this is a new task (not indented or less indented than task level)
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") {
			// This might be a task name
			if strings.Contains(trimmedLine, ":") {
				taskName := strings.TrimSpace(strings.Split(trimmedLine, ":")[0])
				if task, exists := taskfile.Tasks[taskName]; exists {
					task.LineNumber = lineNum + 1 // 1-based line numbers
					task.TaskName = taskName
				}
			}
		}

		// If line doesn't start with spaces, we've left the tasks section
		if trimmedLine != "" && !strings.HasPrefix(line, " ") && inTasksSection && trimmedLine != "tasks:" {
			inTasksSection = false
		}
	}
}

// convertToTasks converts TaskfileTask objects to generic Tasks
func (tp *TaskfileParser) convertToTasks(taskfile *TaskfileRoot, filePath string, modTime time.Time) []*task.Task {
	var tasks []*task.Task

	for taskName, taskDef := range taskfile.Tasks {
		// Skip internal tasks
		if taskDef.Internal {
			tp.logger.Debug("Skipping internal task", "task", taskName)
			continue
		}

		t := &task.Task{
			ID:               generateTaskfileID(taskName, filePath, taskDef.LineNumber),
			Name:             taskName,
			Description:      tp.buildDescription(taskDef),
			Runner:           task.RunnerTask,
			FilePath:         filePath,
			WorkingDirectory: filepath.Dir(filePath),
			Command:          tp.buildCommand(taskDef),
			Args:             tp.buildArgs(taskName, taskDef),
			Environment:      tp.buildEnvironment(taskfile, taskDef),
			Tags:             tp.buildTags(taskDef),
			Dependencies:     tp.buildDependencies(taskDef),
			Hidden:           taskDef.Internal,
			DiscoveredAt:     time.Now(),
			Status:           task.StatusIdle,
		}

		// Add metadata
		t.Metadata = map[string]any{
			"taskfile_version":       taskfile.Version,
			"taskfile_commands":      tp.parseCommands(taskDef.Cmds),
			"taskfile_vars":          taskDef.Vars,
			"taskfile_env":           taskDef.Env,
			"taskfile_sources":       taskDef.Sources,
			"taskfile_generates":     taskDef.Generates,
			"taskfile_status":        taskDef.Status,
			"taskfile_preconditions": taskDef.Preconditions,
			"taskfile_aliases":       taskDef.Aliases,
			"taskfile_method":        taskDef.Method,
			"taskfile_silent":        taskDef.Silent,
			"taskfile_ignore_error":  taskDef.IgnoreError,
			"taskfile_dir":           taskDef.Dir,
			"file_modified":          modTime,
		}

		tasks = append(tasks, t)
	}

	return tasks
}

// buildDescription builds the task description
func (tp *TaskfileParser) buildDescription(taskDef *TaskfileTask) string {
	if taskDef.Desc != "" {
		return taskDef.Desc
	}
	if taskDef.Summary != "" {
		return taskDef.Summary
	}

	// Generate description from first command if no description provided
	if len(taskDef.Cmds) > 0 {
		firstCmd := tp.parseCommand(taskDef.Cmds[0])
		if firstCmd.Cmd != "" {
			// Truncate long commands
			if len(firstCmd.Cmd) > 50 {
				return firstCmd.Cmd[:47] + "..."
			}
			return firstCmd.Cmd
		}
	}

	return ""
}

// buildCommand builds the command for running the task
func (tp *TaskfileParser) buildCommand(_ *TaskfileTask) string {
	return "task"
}

// buildArgs builds the arguments for running the task
func (tp *TaskfileParser) buildArgs(taskName string, taskDef *TaskfileTask) []string {
	args := []string{taskName}

	// Add common task flags based on task configuration
	if taskDef.Silent != nil && *taskDef.Silent {
		args = append([]string{"--silent"}, args...)
	}

	if taskDef.Dir != "" {
		args = append([]string{"--dir", taskDef.Dir}, args...)
	}

	return args
}

// buildEnvironment builds environment variables
func (tp *TaskfileParser) buildEnvironment(taskfile *TaskfileRoot, taskDef *TaskfileTask) map[string]string {
	env := make(map[string]string)

	// Add global environment variables
	if tp.options.ParseEnv && taskfile.Env != nil {
		for key, value := range taskfile.Env {
			if strValue, ok := value.(string); ok {
				env[key] = strValue
			}
		}
	}

	// Add task-specific environment variables
	if taskDef.Env != nil {
		for key, value := range taskDef.Env {
			if strValue, ok := value.(string); ok {
				env[key] = strValue
			}
		}
	}

	// Add global variables as environment (Taskfile convention)
	if tp.options.ParseVars && taskfile.Vars != nil {
		for key, value := range taskfile.Vars {
			if strValue, ok := value.(string); ok {
				env["TASK_"+strings.ToUpper(key)] = strValue
			}
		}
	}

	// Add task-specific variables
	if taskDef.Vars != nil {
		for key, value := range taskDef.Vars {
			if strValue, ok := value.(string); ok {
				env["TASK_"+strings.ToUpper(key)] = strValue
			}
		}
	}

	return env
}

// buildTags builds tags for the task
func (tp *TaskfileParser) buildTags(taskDef *TaskfileTask) []string {
	var tags []string

	// Add runner tag
	tags = append(tags, "task")

	// Add method tag
	if taskDef.Method != "" {
		tags = append(tags, fmt.Sprintf("method:%s", taskDef.Method))
	}

	// Add file operation tags
	if len(taskDef.Sources) > 0 {
		tags = append(tags, "has-sources")
	}
	if len(taskDef.Generates) > 0 {
		tags = append(tags, "generates-files")
	}
	if len(taskDef.Status) > 0 {
		tags = append(tags, "has-status")
	}

	// Add configuration tags
	if taskDef.Silent != nil && *taskDef.Silent {
		tags = append(tags, "silent")
	}
	if taskDef.IgnoreError {
		tags = append(tags, "ignore-error")
	}
	if taskDef.Internal {
		tags = append(tags, "internal")
	}

	// Add dependency tags
	if len(taskDef.Deps) > 0 {
		tags = append(tags, "has-dependencies")
	}

	// Add precondition tags
	if len(taskDef.Preconditions) > 0 {
		tags = append(tags, "has-preconditions")
	}

	// Add alias tags
	if len(taskDef.Aliases) > 0 {
		tags = append(tags, "has-aliases")
	}

	return tags
}

// buildDependencies builds task dependencies
func (tp *TaskfileParser) buildDependencies(taskDef *TaskfileTask) []string {
	var deps []string

	if !tp.options.ParseDeps {
		return deps
	}

	for _, dep := range taskDef.Deps {
		switch depValue := dep.(type) {
		case string:
			deps = append(deps, depValue)
		case map[string]any:
			// Handle dependency with parameters
			if task, ok := depValue["task"].(string); ok {
				deps = append(deps, task)
			}
		}
	}

	return deps
}

// parseCommands parses the commands from the task definition
func (tp *TaskfileParser) parseCommands(cmds []any) []TaskfileCommand {
	var commands []TaskfileCommand

	if !tp.options.IncludeCmds {
		return commands
	}

	for i, cmd := range cmds {
		if tp.options.MaxCmdLines > 0 && i >= tp.options.MaxCmdLines {
			break
		}

		parsed := tp.parseCommand(cmd)
		if parsed.Cmd != "" || parsed.Task != "" {
			commands = append(commands, parsed)
		}
	}

	return commands
}

// parseCommand parses a single command
func (tp *TaskfileParser) parseCommand(cmd any) TaskfileCommand {
	switch cmdValue := cmd.(type) {
	case string:
		return TaskfileCommand{Cmd: cmdValue}

	case map[string]any:
		command := TaskfileCommand{}

		if cmdStr, ok := cmdValue["cmd"].(string); ok {
			command.Cmd = cmdStr
		}
		if taskStr, ok := cmdValue["task"].(string); ok {
			command.Task = taskStr
		}
		if deferStr, ok := cmdValue["defer"].(string); ok {
			command.Defer = deferStr
		}
		if silent, ok := cmdValue["silent"].(bool); ok {
			command.Silent = silent
		}
		if ignoreError, ok := cmdValue["ignore_error"].(bool); ok {
			command.IgnoreError = ignoreError
		}
		if vars, ok := cmdValue["vars"].(map[string]any); ok {
			command.Vars = vars
		}

		return command

	default:
		tp.logger.Warn("Unknown command format", "type", fmt.Sprintf("%T", cmd))
		return TaskfileCommand{}
	}
}

// generateTaskfileID generates a unique task ID
func generateTaskfileID(name, filePath string, lineNumber int) string {
	return fmt.Sprintf("task:%s:%s:%d", filepath.Base(filePath), name, lineNumber)
}

// GetMetadata returns parser metadata
func (tp *TaskfileParser) GetMetadata() map[string]any {
	return map[string]any{
		"parser_type":     "taskfile",
		"supported_files": tp.options.FileNames,
		"max_file_size":   tp.options.MaxFileSize,
		"parse_includes":  tp.options.ParseIncludes,
		"parse_vars":      tp.options.ParseVars,
		"parse_env":       tp.options.ParseEnv,
		"validate_schema": tp.options.ValidateSchema,
		"include_cmds":    tp.options.IncludeCmds,
		"max_cmd_lines":   tp.options.MaxCmdLines,
		"parse_deps":      tp.options.ParseDeps,
	}
}

// Validate validates a Taskfile for syntax and semantic errors
func (tp *TaskfileParser) Validate(filePath string) error {
	tp.logger.Debug("Validating Taskfile", "file", filePath)

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	content, err := io.ReadAll(file)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Parse YAML
	var taskfile TaskfileRoot
	if err := yaml.Unmarshal(content, &taskfile); err != nil {
		return fmt.Errorf("YAML parsing error: %w", err)
	}

	// Validate schema
	if err := tp.validateTaskfile(&taskfile); err != nil {
		return fmt.Errorf("validation error: %w", err)
	}

	return nil
}

// GetStatistics returns parsing statistics
func (tp *TaskfileParser) GetStatistics() map[string]any {
	return map[string]any{
		"parser_type":     "taskfile",
		"version":         "1.0.0",
		"supported_files": len(tp.options.FileNames),
		"yaml_based":      true,
	}
}

// GetTaskAliases returns all aliases for tasks in the file
func (tp *TaskfileParser) GetTaskAliases(filePath string) (map[string][]string, error) {
	tasks, err := tp.ParseFile(filePath)
	if err != nil {
		return nil, err
	}

	aliases := make(map[string][]string)

	for _, t := range tasks {
		if taskAliases, ok := t.Metadata["taskfile_aliases"].([]string); ok {
			if len(taskAliases) > 0 {
				aliases[t.Name] = taskAliases
			}
		}
	}

	return aliases, nil
}

// GetTaskDependencyGraph returns the dependency graph for tasks
func (tp *TaskfileParser) GetTaskDependencyGraph(filePath string) (map[string][]string, error) {
	tasks, err := tp.ParseFile(filePath)
	if err != nil {
		return nil, err
	}

	graph := make(map[string][]string)

	for _, t := range tasks {
		graph[t.Name] = t.Dependencies
	}

	return graph, nil
}
