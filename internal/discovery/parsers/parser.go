package parsers

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/vivalchemy/wake/pkg/logger"
	"github.com/vivalchemy/wake/pkg/task"
	"maps"
)

// Parser defines the interface for parsing task files
type Parser interface {
	// Parse parses a task file and returns discovered tasks
	Parse(filePath string) ([]*task.Task, error)

	// CanParse checks if the parser can handle the given file
	CanParse(filePath string) bool

	// GetRunnerType returns the runner type this parser handles
	GetRunnerType() task.RunnerType

	// GetSupportedFiles returns file patterns this parser supports
	GetSupportedFiles() []string
}

// BaseParser provides common functionality for all parsers
type BaseParser struct {
	logger     logger.Logger
	runnerType task.RunnerType
	patterns   []string
}

// NewBaseParser creates a new base parser
func NewBaseParser(runnerType task.RunnerType, patterns []string, log logger.Logger) *BaseParser {
	return &BaseParser{
		logger:     log.WithGroup(fmt.Sprintf("parser-%s", runnerType)),
		runnerType: runnerType,
		patterns:   patterns,
	}
}

// CanParse checks if the parser can handle the given file
func (p *BaseParser) CanParse(filePath string) bool {
	filename := filepath.Base(filePath)
	for _, pattern := range p.patterns {
		if matched, _ := filepath.Match(pattern, filename); matched {
			return true
		}
		if pattern == filename {
			return true
		}
	}
	return false
}

// GetRunnerType returns the runner type
func (p *BaseParser) GetRunnerType() task.RunnerType {
	return p.runnerType
}

// GetSupportedFiles returns supported file patterns
func (p *BaseParser) GetSupportedFiles() []string {
	return p.patterns
}

// ParseResult represents the result of parsing a task file
type ParseResult struct {
	// Successfully parsed tasks
	Tasks []*task.Task

	// Errors encountered during parsing
	Errors []ParseError

	// Warnings (non-fatal issues)
	Warnings []string

	// Metadata about the parsing process
	Metadata map[string]any
}

// ParseError represents an error during parsing
type ParseError struct {
	// Line number where error occurred (0 if unknown)
	Line int

	// Column number where error occurred (0 if unknown)
	Column int

	// Error message
	Message string

	// Context around the error
	Context string

	// Whether this error is fatal
	Fatal bool
}

// Error implements the error interface
func (e ParseError) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("line %d: %s", e.Line, e.Message)
	}
	return e.Message
}

// ParseContext provides context for parsing operations
type ParseContext struct {
	// File being parsed
	FilePath string

	// Working directory
	WorkingDir string

	// Environment variables
	Environment map[string]string

	// Parser options
	Options map[string]any

	// Current line number (for error reporting)
	LineNumber int
}

// FileReader provides utilities for reading and parsing files
type FileReader struct {
	filePath string
	logger   logger.Logger
}

// NewFileReader creates a new file reader
func NewFileReader(filePath string, log logger.Logger) *FileReader {
	return &FileReader{
		filePath: filePath,
		logger:   log,
	}
}

// ReadLines reads all lines from the file
func (r *FileReader) ReadLines() ([]string, error) {
	file, err := os.Open(r.filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	return lines, nil
}

// ReadLinesWithCallback reads lines and calls a callback for each line
func (r *FileReader) ReadLinesWithCallback(callback func(lineNum int, line string) error) error {
	file, err := os.Open(r.filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		if err := callback(lineNum, scanner.Text()); err != nil {
			return err
		}
	}

	return scanner.Err()
}

// ReadBytes reads the entire file as bytes
func (r *FileReader) ReadBytes() ([]byte, error) {
	return os.ReadFile(r.filePath)
}

// ReadString reads the entire file as a string
func (r *FileReader) ReadString() (string, error) {
	bytes, err := r.ReadBytes()
	if err != nil {
		return "", err
	}
	return string(bytes), nil
}

// TaskBuilder helps build task objects with common properties
type TaskBuilder struct {
	task   *task.Task
	logger logger.Logger
}

// NewTaskBuilder creates a new task builder
func NewTaskBuilder(name string, runner task.RunnerType, log logger.Logger) *TaskBuilder {
	t := task.NewTask(name, runner)
	return &TaskBuilder{
		task:   t,
		logger: log,
	}
}

// SetDescription sets the task description
func (b *TaskBuilder) SetDescription(description string) *TaskBuilder {
	b.task.Description = strings.TrimSpace(description)
	return b
}

// SetCommand sets the task command
func (b *TaskBuilder) SetCommand(command string) *TaskBuilder {
	b.task.Command = strings.TrimSpace(command)
	return b
}

// SetArgs sets the task arguments
func (b *TaskBuilder) SetArgs(args []string) *TaskBuilder {
	b.task.Args = make([]string, len(args))
	copy(b.task.Args, args)
	return b
}

// AddArg adds a single argument
func (b *TaskBuilder) AddArg(arg string) *TaskBuilder {
	b.task.Args = append(b.task.Args, arg)
	return b
}

// SetWorkingDirectory sets the working directory
func (b *TaskBuilder) SetWorkingDirectory(dir string) *TaskBuilder {
	b.task.WorkingDirectory = dir
	return b
}

// SetFilePath sets the file path
func (b *TaskBuilder) SetFilePath(path string) *TaskBuilder {
	b.task.FilePath = path
	return b
}

// AddEnvironment adds environment variables
func (b *TaskBuilder) AddEnvironment(key, value string) *TaskBuilder {
	if b.task.Environment == nil {
		b.task.Environment = make(map[string]string)
	}
	b.task.Environment[key] = value
	return b
}

// SetEnvironment sets all environment variables
func (b *TaskBuilder) SetEnvironment(env map[string]string) *TaskBuilder {
	b.task.Environment = make(map[string]string)
	maps.Copy(b.task.Environment, env)
	return b
}

// AddTag adds a tag
func (b *TaskBuilder) AddTag(tag string) *TaskBuilder {
	b.task.AddTag(tag)
	return b
}

// AddTags adds multiple tags
func (b *TaskBuilder) AddTags(tags []string) *TaskBuilder {
	for _, tag := range tags {
		b.task.AddTag(tag)
	}
	return b
}

// AddAlias adds an alias
func (b *TaskBuilder) AddAlias(alias string) *TaskBuilder {
	b.task.AddAlias(alias)
	return b
}

// AddAliases adds multiple aliases
func (b *TaskBuilder) AddAliases(aliases []string) *TaskBuilder {
	for _, alias := range aliases {
		b.task.AddAlias(alias)
	}
	return b
}

// AddDependency adds a dependency
func (b *TaskBuilder) AddDependency(dep string) *TaskBuilder {
	b.task.Dependencies = append(b.task.Dependencies, dep)
	return b
}

// AddDependencies adds multiple dependencies
func (b *TaskBuilder) AddDependencies(deps []string) *TaskBuilder {
	b.task.Dependencies = append(b.task.Dependencies, deps...)
	return b
}

// SetHidden sets whether the task is hidden
func (b *TaskBuilder) SetHidden(hidden bool) *TaskBuilder {
	b.task.Hidden = hidden
	return b
}

// AddMetadata adds metadata
func (b *TaskBuilder) AddMetadata(key string, value any) *TaskBuilder {
	if b.task.Metadata == nil {
		b.task.Metadata = make(map[string]any)
	}
	b.task.Metadata[key] = value
	return b
}

// SetMetadata sets all metadata
func (b *TaskBuilder) SetMetadata(metadata map[string]any) *TaskBuilder {
	b.task.Metadata = make(map[string]any)
	maps.Copy(b.task.Metadata, metadata)
	return b
}

// Build returns the constructed task
func (b *TaskBuilder) Build() *task.Task {
	// Set discovery time
	b.task.DiscoveredAt = time.Now()

	// Validate the task
	if err := b.task.Validate(); err != nil {
		b.logger.Warn("Built task failed validation", "task", b.task.Name, "error", err)
	}

	return b.task
}

// CommentExtractor helps extract descriptions from comments
type CommentExtractor struct {
	// Patterns for different comment styles
	patterns []*regexp.Regexp
}

// NewCommentExtractor creates a new comment extractor
func NewCommentExtractor(commentPrefixes []string) *CommentExtractor {
	extractor := &CommentExtractor{
		patterns: make([]*regexp.Regexp, 0),
	}

	for _, prefix := range commentPrefixes {
		// Create regex pattern for comments
		pattern := fmt.Sprintf(`^\s*%s\s*(.*)$`, regexp.QuoteMeta(prefix))
		if regex, err := regexp.Compile(pattern); err == nil {
			extractor.patterns = append(extractor.patterns, regex)
		}
	}

	return extractor
}

// ExtractFromLine extracts comment text from a line
func (e *CommentExtractor) ExtractFromLine(line string) (string, bool) {
	for _, pattern := range e.patterns {
		if matches := pattern.FindStringSubmatch(line); len(matches) > 1 {
			return strings.TrimSpace(matches[1]), true
		}
	}
	return "", false
}

// ExtractFromLines extracts comments from multiple lines
func (e *CommentExtractor) ExtractFromLines(lines []string) []string {
	var comments []string
	for _, line := range lines {
		if comment, found := e.ExtractFromLine(line); found && comment != "" {
			comments = append(comments, comment)
		}
	}
	return comments
}

// LineProcessor provides utilities for processing lines
type LineProcessor struct {
	trimSpaces bool
	skipEmpty  bool
}

// NewLineProcessor creates a new line processor
func NewLineProcessor(trimSpaces, skipEmpty bool) *LineProcessor {
	return &LineProcessor{
		trimSpaces: trimSpaces,
		skipEmpty:  skipEmpty,
	}
}

// ProcessLine processes a single line according to configuration
func (p *LineProcessor) ProcessLine(line string) (string, bool) {
	if p.trimSpaces {
		line = strings.TrimSpace(line)
	}

	if p.skipEmpty && line == "" {
		return "", false
	}

	return line, true
}

// ProcessLines processes multiple lines
func (p *LineProcessor) ProcessLines(lines []string) []string {
	var processed []string
	for _, line := range lines {
		if processedLine, keep := p.ProcessLine(line); keep {
			processed = append(processed, processedLine)
		}
	}
	return processed
}

// VariableExtractor helps extract variable definitions
type VariableExtractor struct {
	patterns []*regexp.Regexp
}

// NewVariableExtractor creates a new variable extractor
func NewVariableExtractor(patterns []string) *VariableExtractor {
	extractor := &VariableExtractor{
		patterns: make([]*regexp.Regexp, 0),
	}

	for _, pattern := range patterns {
		if regex, err := regexp.Compile(pattern); err == nil {
			extractor.patterns = append(extractor.patterns, regex)
		}
	}

	return extractor
}

// ExtractFromLine extracts variable name and value from a line
func (e *VariableExtractor) ExtractFromLine(line string) (string, string, bool) {
	for _, pattern := range e.patterns {
		if matches := pattern.FindStringSubmatch(line); len(matches) >= 3 {
			name := strings.TrimSpace(matches[1])
			value := strings.TrimSpace(matches[2])
			return name, value, true
		}
	}
	return "", "", false
}

// ExtractFromLines extracts all variables from multiple lines
func (e *VariableExtractor) ExtractFromLines(lines []string) map[string]string {
	variables := make(map[string]string)
	for _, line := range lines {
		if name, value, found := e.ExtractFromLine(line); found {
			variables[name] = value
		}
	}
	return variables
}

// ParserRegistry maintains a registry of available parsers
type ParserRegistry struct {
	parsers map[string]Parser
	logger  logger.Logger
}

// NewParserRegistry creates a new parser registry
func NewParserRegistry(log logger.Logger) *ParserRegistry {
	return &ParserRegistry{
		parsers: make(map[string]Parser),
		logger:  log.WithGroup("parser-registry"),
	}
}

// Register registers a parser for specific file patterns
func (r *ParserRegistry) Register(patterns []string, parser Parser) {
	for _, pattern := range patterns {
		r.parsers[pattern] = parser
		r.logger.Debug("Registered parser", "pattern", pattern, "runner", parser.GetRunnerType())
	}
}

// GetParser returns a parser for the given file path
func (r *ParserRegistry) GetParser(filePath string) (Parser, bool) {
	filename := filepath.Base(filePath)

	// Try exact match first
	if parser, exists := r.parsers[filename]; exists {
		return parser, true
	}

	// Try pattern matching
	for pattern, parser := range r.parsers {
		if matched, _ := filepath.Match(pattern, filename); matched {
			return parser, true
		}
	}

	return nil, false
}

// GetAllParsers returns all registered parsers
func (r *ParserRegistry) GetAllParsers() map[string]Parser {
	// Return a copy to prevent external modification
	parsers := make(map[string]Parser)
	maps.Copy(parsers, r.parsers)
	return parsers
}

// GetSupportedFiles returns all supported file patterns
func (r *ParserRegistry) GetSupportedFiles() []string {
	var patterns []string
	for pattern := range r.parsers {
		patterns = append(patterns, pattern)
	}
	return patterns
}

// Utility functions

// SplitCommandLine splits a command line string into command and arguments
func SplitCommandLine(cmdLine string) (string, []string) {
	// Simple splitting - for more complex cases, consider using a proper shell parser
	parts := strings.Fields(strings.TrimSpace(cmdLine))
	if len(parts) == 0 {
		return "", nil
	}

	command := parts[0]
	args := parts[1:]

	return command, args
}

// CleanDescription cleans and normalizes a task description
func CleanDescription(desc string) string {
	// Remove extra whitespace
	desc = strings.TrimSpace(desc)

	// Remove common prefixes
	prefixes := []string{"# ", "## ", "### ", "// ", "/* ", "*/", "*"}
	for _, prefix := range prefixes {
		desc = strings.TrimPrefix(desc, prefix)
	}

	// Remove trailing punctuation if it's just a period
	if strings.HasSuffix(desc, ".") && !strings.HasSuffix(desc, "..") {
		desc = strings.TrimSuffix(desc, ".")
	}

	return strings.TrimSpace(desc)
}

// IsEmptyOrComment checks if a line is empty or a comment
func IsEmptyOrComment(line string, commentPrefixes []string) bool {
	line = strings.TrimSpace(line)

	if line == "" {
		return true
	}

	for _, prefix := range commentPrefixes {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}

	return false
}

// ParseKeyValuePair parses a key=value pair
func ParseKeyValuePair(line, separator string) (string, string, bool) {
	parts := strings.SplitN(line, separator, 2)
	if len(parts) != 2 {
		return "", "", false
	}

	key := strings.TrimSpace(parts[0])
	value := strings.TrimSpace(parts[1])

	return key, value, true
}
