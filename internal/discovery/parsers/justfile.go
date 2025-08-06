package parsers

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/vivalchemy/wake/pkg/logger"
	"github.com/vivalchemy/wake/pkg/task"
	"maps"
)

// JustfileParser parses Justfile task definitions
type JustfileParser struct {
	logger   logger.Logger
	patterns *JustfilePatterns
	options  JustfileOptions
}

// JustfileOptions contains configuration for Justfile parsing
type JustfileOptions struct {
	// File matching
	FileNames     []string
	CaseSensitive bool
	MaxFileSize   int64

	// Parsing options
	ParseComments     bool
	ParseVariables    bool
	ParseDependencies bool
	ParsePrivateTasks bool

	// Content options
	IncludeBody    bool
	MaxBodyLines   int
	TrimWhitespace bool
}

// JustfilePatterns contains regex patterns for parsing Justfiles
type JustfilePatterns struct {
	// Recipe patterns
	RecipeHeader *regexp.Regexp
	RecipeBody   *regexp.Regexp
	RecipeParam  *regexp.Regexp

	// Comment patterns
	Comment    *regexp.Regexp
	DocComment *regexp.Regexp

	// Variable patterns
	Variable   *regexp.Regexp
	Assignment *regexp.Regexp

	// Dependency patterns
	Dependency *regexp.Regexp

	// Attribute patterns
	Attribute *regexp.Regexp
	Private   *regexp.Regexp
}

// JustfileTask represents a parsed task from a Justfile
type JustfileTask struct {
	// Basic info
	Name        string
	Description string
	FilePath    string
	LineNumber  int

	// Recipe details
	Parameters   []JustfileParameter
	Body         []string
	Dependencies []string
	Attributes   []string

	// Metadata
	IsPrivate bool
	Variables map[string]string
	Comments  []string

	// File info
	ModTime time.Time
}

// JustfileParameter represents a recipe parameter
type JustfileParameter struct {
	Name         string
	DefaultValue string
	IsOptional   bool
	IsVariadic   bool
	Type         string
}

// NewJustfileParser creates a new Justfile parser
func NewJustfileParser(logger logger.Logger) *JustfileParser {
	return &JustfileParser{
		logger:   logger.WithGroup("justfile-parser"),
		patterns: createJustfilePatterns(),
		options:  createDefaultJustfileOptions(),
	}
}

// createJustfilePatterns creates regex patterns for parsing
func createJustfilePatterns() *JustfilePatterns {
	return &JustfilePatterns{
		// Recipe header: recipe_name param1 param2="default": dependency1 dependency2
		RecipeHeader: regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_-]*)\s*([^:]*)?:\s*(.*)$`),

		// Recipe body line (starts with tab or spaces)
		RecipeBody: regexp.MustCompile(`^(\t| {4,})(.*)$`),

		// Recipe parameter: param="default" or +param or *param
		RecipeParam: regexp.MustCompile(`([+*]?)([a-zA-Z_][a-zA-Z0-9_-]*)(?:=(.*))?`),

		// Comments
		Comment:    regexp.MustCompile(`^\s*#(.*)$`),
		DocComment: regexp.MustCompile(`^\s*# (.*)$`),

		// Variables: variable := value
		Variable:   regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_-]*)\s*:=\s*(.*)$`),
		Assignment: regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_-]*)\s*=\s*(.*)$`),

		// Dependencies
		Dependency: regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_-]*)`),

		// Attributes
		Attribute: regexp.MustCompile(`^\[([^\]]+)\]$`),
		Private:   regexp.MustCompile(`^_.*`),
	}
}

// createDefaultJustfileOptions creates default parsing options
func createDefaultJustfileOptions() JustfileOptions {
	return JustfileOptions{
		FileNames:         []string{"justfile", "Justfile", ".justfile"},
		CaseSensitive:     true,
		MaxFileSize:       10 * 1024 * 1024, // 10MB
		ParseComments:     true,
		ParseVariables:    true,
		ParseDependencies: true,
		ParsePrivateTasks: false,
		IncludeBody:       true,
		MaxBodyLines:      100,
		TrimWhitespace:    true,
	}
}

// SetOptions sets parsing options
func (jp *JustfileParser) SetOptions(options JustfileOptions) {
	jp.options = options
}

// GetSupportedFiles returns supported file patterns
func (jp *JustfileParser) GetSupportedFiles() []string {
	return jp.options.FileNames
}

func (jp *JustfileParser) GetRunnerType() task.RunnerType {
	return task.RunnerJust
}

// CanParse checks if the parser can handle the given file
func (jp *JustfileParser) CanParse(filePath string) bool {
	fileName := filepath.Base(filePath)

	for _, supportedName := range jp.options.FileNames {
		if jp.options.CaseSensitive {
			if fileName == supportedName {
				return true
			}
		} else {
			if strings.EqualFold(fileName, supportedName) {
				return true
			}
		}
	}

	return false
}

func (jp *JustfileParser) Parse(filePath string) ([]*task.Task, error) {
	return jp.ParseFile(filePath)
}

// ParseFile parses a Justfile and returns discovered tasks
func (jp *JustfileParser) ParseFile(filePath string) ([]*task.Task, error) {
	jp.logger.Debug("Parsing Justfile", "file", filePath)

	// Check if we can parse this file
	if !jp.CanParse(filePath) {
		return nil, fmt.Errorf("unsupported file: %s", filePath)
	}

	// Check file size
	if err := jp.checkFileSize(filePath); err != nil {
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
	justfileTasks, err := jp.parseContent(file, filePath, fileInfo.ModTime())
	if err != nil {
		return nil, fmt.Errorf("failed to parse content: %w", err)
	}

	// Convert to generic tasks
	tasks := jp.convertToTasks(justfileTasks, filePath)

	jp.logger.Info("Parsed Justfile", "file", filePath, "tasks", len(tasks))
	return tasks, nil
}

// ParseContent parses Justfile content from a reader
func (jp *JustfileParser) ParseContent(reader io.Reader, filePath string) ([]*task.Task, error) {
	justfileTasks, err := jp.parseContent(reader, filePath, time.Now())
	if err != nil {
		return nil, err
	}

	return jp.convertToTasks(justfileTasks, filePath), nil
}

// checkFileSize checks if file size is within limits
func (jp *JustfileParser) checkFileSize(filePath string) error {
	if jp.options.MaxFileSize <= 0 {
		return nil
	}

	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return err
	}

	if fileInfo.Size() > jp.options.MaxFileSize {
		return fmt.Errorf("file too large: %d bytes (max: %d)", fileInfo.Size(), jp.options.MaxFileSize)
	}

	return nil
}

// parseContent parses the actual content
func (jp *JustfileParser) parseContent(reader io.Reader, filePath string, modTime time.Time) ([]*JustfileTask, error) {
	scanner := bufio.NewScanner(reader)

	var tasks []*JustfileTask
	var variables = make(map[string]string)
	var currentTask *JustfileTask
	var lineNumber int
	var pendingComments []string

	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		originalLine := line

		if jp.options.TrimWhitespace {
			line = strings.TrimSpace(line)
		}

		// Skip empty lines
		if line == "" {
			// Empty line might end current task
			if currentTask != nil && len(currentTask.Body) > 0 {
				tasks = append(tasks, currentTask)
				currentTask = nil
			}
			continue
		}

		// Parse comments
		if jp.patterns.Comment.MatchString(line) {
			if jp.options.ParseComments {
				comment := jp.parseComment(line)
				if comment != "" {
					pendingComments = append(pendingComments, comment)
				}
			}
			continue
		}

		// Parse variables
		if jp.options.ParseVariables && jp.patterns.Variable.MatchString(line) {
			name, value := jp.parseVariable(line)
			if name != "" {
				variables[name] = value
			}
			continue
		}

		// Parse recipe header
		if jp.patterns.RecipeHeader.MatchString(line) {
			// Save previous task if any
			if currentTask != nil {
				tasks = append(tasks, currentTask)
			}

			// Parse new recipe
			var err error
			currentTask, err = jp.parseRecipeHeader(line, filePath, lineNumber, modTime)
			if err != nil {
				jp.logger.Warn("Failed to parse recipe header", "line", lineNumber, "error", err)
				currentTask = nil
				continue
			}

			// Add pending comments as description
			if len(pendingComments) > 0 {
				currentTask.Description = strings.Join(pendingComments, " ")
				currentTask.Comments = append(currentTask.Comments, pendingComments...)
				pendingComments = nil
			}

			// Add variables
			for k, v := range variables {
				if currentTask.Variables == nil {
					currentTask.Variables = make(map[string]string)
				}
				currentTask.Variables[k] = v
			}

			continue
		}

		// Parse recipe body
		if currentTask != nil && jp.patterns.RecipeBody.MatchString(originalLine) {
			bodyLine := jp.parseRecipeBody(originalLine)
			if bodyLine != "" && jp.options.IncludeBody {
				if jp.options.MaxBodyLines == 0 || len(currentTask.Body) < jp.options.MaxBodyLines {
					currentTask.Body = append(currentTask.Body, bodyLine)
				}
			}
			continue
		}

		// If we have a current task but this line doesn't match body pattern,
		// the task is complete
		if currentTask != nil {
			tasks = append(tasks, currentTask)
			currentTask = nil
		}

		// Clear pending comments if they weren't used
		pendingComments = nil
	}

	// Save last task if any
	if currentTask != nil {
		tasks = append(tasks, currentTask)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	// Filter private tasks if needed
	if !jp.options.ParsePrivateTasks {
		tasks = jp.filterPrivateTasks(tasks)
	}

	return tasks, nil
}

// parseComment parses a comment line
func (jp *JustfileParser) parseComment(line string) string {
	matches := jp.patterns.DocComment.FindStringSubmatch(line)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// parseVariable parses a variable assignment
func (jp *JustfileParser) parseVariable(line string) (string, string) {
	matches := jp.patterns.Variable.FindStringSubmatch(line)
	if len(matches) > 2 {
		name := strings.TrimSpace(matches[1])
		value := strings.TrimSpace(matches[2])

		// Remove quotes if present
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') ||
			(value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}

		return name, value
	}
	return "", ""
}

// parseRecipeHeader parses a recipe header line
func (jp *JustfileParser) parseRecipeHeader(line, filePath string, lineNumber int, modTime time.Time) (*JustfileTask, error) {
	matches := jp.patterns.RecipeHeader.FindStringSubmatch(line)
	if len(matches) < 2 {
		return nil, fmt.Errorf("invalid recipe header format")
	}

	name := strings.TrimSpace(matches[1])
	paramsStr := ""
	dependsStr := ""

	if len(matches) > 2 {
		paramsStr = strings.TrimSpace(matches[2])
	}
	if len(matches) > 3 {
		dependsStr = strings.TrimSpace(matches[3])
	}

	task := &JustfileTask{
		Name:         name,
		FilePath:     filePath,
		LineNumber:   lineNumber,
		ModTime:      modTime,
		Parameters:   []JustfileParameter{},
		Body:         []string{},
		Dependencies: []string{},
		Attributes:   []string{},
		Variables:    make(map[string]string),
		Comments:     []string{},
	}

	// Check if private
	task.IsPrivate = jp.patterns.Private.MatchString(name)

	// Parse parameters
	if paramsStr != "" {
		task.Parameters = jp.parseParameters(paramsStr)
	}

	// Parse dependencies
	if dependsStr != "" && jp.options.ParseDependencies {
		task.Dependencies = jp.parseDependencies(dependsStr)
	}

	return task, nil
}

// parseParameters parses recipe parameters
func (jp *JustfileParser) parseParameters(paramsStr string) []JustfileParameter {
	var params []JustfileParameter

	// Split by whitespace but respect quotes
	paramTokens := jp.splitParameters(paramsStr)

	for _, token := range paramTokens {
		param := jp.parseParameter(token)
		if param.Name != "" {
			params = append(params, param)
		}
	}

	return params
}

// splitParameters splits parameter string respecting quotes
func (jp *JustfileParser) splitParameters(paramsStr string) []string {
	var tokens []string
	var current strings.Builder
	var inQuotes bool
	var quoteChar rune

	for _, char := range paramsStr {
		switch {
		case !inQuotes && (char == '"' || char == '\''):
			inQuotes = true
			quoteChar = char
			current.WriteRune(char)

		case inQuotes && char == quoteChar:
			inQuotes = false
			current.WriteRune(char)

		case !inQuotes && (char == ' ' || char == '\t'):
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}

		default:
			current.WriteRune(char)
		}
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}

// parseParameter parses a single parameter
func (jp *JustfileParser) parseParameter(paramStr string) JustfileParameter {
	matches := jp.patterns.RecipeParam.FindStringSubmatch(paramStr)
	if len(matches) < 3 {
		return JustfileParameter{Name: paramStr}
	}

	param := JustfileParameter{
		Name: matches[2],
	}

	// Check for modifiers
	modifier := matches[1]
	switch modifier {
	case "+":
		param.IsOptional = true
	case "*":
		param.IsVariadic = true
	}

	// Parse default value
	if len(matches) > 3 && matches[3] != "" {
		defaultValue := matches[3]

		// Remove quotes
		if len(defaultValue) >= 2 && ((defaultValue[0] == '"' && defaultValue[len(defaultValue)-1] == '"') ||
			(defaultValue[0] == '\'' && defaultValue[len(defaultValue)-1] == '\'')) {
			defaultValue = defaultValue[1 : len(defaultValue)-1]
		}

		param.DefaultValue = defaultValue
		param.IsOptional = true
	}

	return param
}

// parseDependencies parses recipe dependencies
func (jp *JustfileParser) parseDependencies(dependsStr string) []string {
	matches := jp.patterns.Dependency.FindAllString(dependsStr, -1)
	var deps []string

	for _, match := range matches {
		dep := strings.TrimSpace(match)
		if dep != "" {
			deps = append(deps, dep)
		}
	}

	return deps
}

// parseRecipeBody parses a recipe body line
func (jp *JustfileParser) parseRecipeBody(line string) string {
	matches := jp.patterns.RecipeBody.FindStringSubmatch(line)
	if len(matches) > 2 {
		return matches[2]
	}
	return ""
}

// filterPrivateTasks filters out private tasks
func (jp *JustfileParser) filterPrivateTasks(tasks []*JustfileTask) []*JustfileTask {
	var filtered []*JustfileTask

	for _, task := range tasks {
		if !task.IsPrivate {
			filtered = append(filtered, task)
		}
	}

	return filtered
}

// convertToTasks converts JustfileTasks to generic Tasks
func (jp *JustfileParser) convertToTasks(justfileTasks []*JustfileTask, filePath string) []*task.Task {
	var tasks []*task.Task

	for _, jt := range justfileTasks {
		t := &task.Task{
			ID:               generateTaskID(jt.Name, filePath, jt.LineNumber),
			Name:             jt.Name,
			Description:      jt.Description,
			Runner:           task.RunnerJust,
			FilePath:         filePath,
			WorkingDirectory: filepath.Dir(filePath),
			Command:          jp.buildCommand(jt),
			Args:             jp.buildArgs(jt),
			Environment:      jp.buildEnvironment(jt),
			Tags:             jp.buildTags(jt),
			Dependencies:     jt.Dependencies,
			Hidden:           jt.IsPrivate,
			DiscoveredAt:     time.Now(),
			Status:           task.StatusIdle,
		}

		// Add metadata
		t.Metadata = map[string]any{
			"justfile_parameters": jt.Parameters,
			"justfile_body":       jt.Body,
			"justfile_variables":  jt.Variables,
			"justfile_comments":   jt.Comments,
			"justfile_attributes": jt.Attributes,
			"file_modified":       jt.ModTime,
		}

		tasks = append(tasks, t)
	}

	return tasks
}

// buildCommand builds the command for running the task
func (jp *JustfileParser) buildCommand(_ *JustfileTask) string {
	return "just"
}

// buildArgs builds the arguments for running the task
func (jp *JustfileParser) buildArgs(jt *JustfileTask) []string {
	args := []string{jt.Name}

	// Add parameter placeholders
	for _, param := range jt.Parameters {
		if param.IsOptional {
			if param.DefaultValue != "" {
				args = append(args, param.DefaultValue)
			}
		} else {
			// Required parameter placeholder
			args = append(args, fmt.Sprintf("${%s}", param.Name))
		}
	}

	return args
}

// buildEnvironment builds environment variables
func (jp *JustfileParser) buildEnvironment(jt *JustfileTask) map[string]string {
	env := make(map[string]string)

	// Add Justfile variables as environment
	maps.Copy(env, jt.Variables)

	return env
}

// buildTags builds tags for the task
func (jp *JustfileParser) buildTags(jt *JustfileTask) []string {
	var tags []string

	// Add runner tag
	tags = append(tags, "just")

	// Add private tag if applicable
	if jt.IsPrivate {
		tags = append(tags, "private")
	}

	// Add parameter-based tags
	if len(jt.Parameters) > 0 {
		tags = append(tags, "parameterized")
	}

	// Add dependency-based tags
	if len(jt.Dependencies) > 0 {
		tags = append(tags, "has-dependencies")
	}

	// Add attribute-based tags
	for _, attr := range jt.Attributes {
		tags = append(tags, fmt.Sprintf("attr:%s", attr))
	}

	return tags
}

// generateTaskID generates a unique task ID
func generateTaskID(name, filePath string, lineNumber int) string {
	return fmt.Sprintf("just:%s:%s:%d", filepath.Base(filePath), name, lineNumber)
}

// GetMetadata returns parser metadata
func (jp *JustfileParser) GetMetadata() map[string]any {
	return map[string]any{
		"parser_type":         "justfile",
		"supported_files":     jp.options.FileNames,
		"case_sensitive":      jp.options.CaseSensitive,
		"max_file_size":       jp.options.MaxFileSize,
		"parse_comments":      jp.options.ParseComments,
		"parse_variables":     jp.options.ParseVariables,
		"parse_dependencies":  jp.options.ParseDependencies,
		"parse_private_tasks": jp.options.ParsePrivateTasks,
		"include_body":        jp.options.IncludeBody,
		"max_body_lines":      jp.options.MaxBodyLines,
	}
}

// Validate validates a Justfile for syntax errors
func (jp *JustfileParser) Validate(filePath string) error {
	jp.logger.Debug("Validating Justfile", "file", filePath)

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNumber := 0
	var errors []string

	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()

		// Basic validation - check for common syntax errors
		if err := jp.validateLine(line, lineNumber); err != nil {
			errors = append(errors, fmt.Sprintf("line %d: %s", lineNumber, err.Error()))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("validation failed:\n%s", strings.Join(errors, "\n"))
	}

	return nil
}

// validateLine validates a single line for syntax errors
func (jp *JustfileParser) validateLine(line string, _ int) error {
	// Skip empty lines and comments
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return nil
	}

	// Check for recipe header
	if strings.Contains(line, ":") && !strings.Contains(line, ":=") {
		if !jp.patterns.RecipeHeader.MatchString(line) {
			return fmt.Errorf("invalid recipe header syntax")
		}
	}

	// Check for variable assignment
	if strings.Contains(line, ":=") {
		if !jp.patterns.Variable.MatchString(line) {
			return fmt.Errorf("invalid variable assignment syntax")
		}
	}

	return nil
}

// GetStatistics returns parsing statistics
func (jp *JustfileParser) GetStatistics() map[string]any {
	return map[string]any{
		"parser_type":     "justfile",
		"version":         "1.0.0",
		"patterns_count":  9,
		"supported_files": len(jp.options.FileNames),
	}
}
