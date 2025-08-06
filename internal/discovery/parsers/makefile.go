package parsers

import (
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/vivalchemy/wake/pkg/logger"
	"github.com/vivalchemy/wake/pkg/task"
	"slices"
)

// MakefileParser parses Makefile targets and extracts tasks
type MakefileParser struct {
	*BaseParser

	// Regex patterns for parsing
	targetPattern      *regexp.Regexp
	variablePattern    *regexp.Regexp
	commentPattern     *regexp.Regexp
	phonyPattern       *regexp.Regexp
	includePattern     *regexp.Regexp
	conditionalPattern *regexp.Regexp

	// Comment extractor
	commentExtractor *CommentExtractor

	// Variable extractor
	variableExtractor *VariableExtractor
}

var _ Parser = (*MakefileParser)(nil)

// NewMakefileParser creates a new Makefile parser
func NewMakefileParser(log logger.Logger) (*MakefileParser, error) {
	patterns := []string{"Makefile", "makefile", "GNUmakefile"}
	base := NewBaseParser(task.RunnerMake, patterns, log)

	parser := &MakefileParser{
		BaseParser: base,
	}

	if err := parser.initializePatterns(); err != nil {
		return nil, fmt.Errorf("failed to initialize patterns: %w", err)
	}

	parser.commentExtractor = NewCommentExtractor([]string{"#"})
	parser.variableExtractor = NewVariableExtractor([]string{
		`^([A-Za-z_][A-Za-z0-9_]*)\s*[:=]\s*(.*)$`, // VAR = value or VAR := value
		`^([A-Za-z_][A-Za-z0-9_]*)\s*\+=\s*(.*)$`,  // VAR += value
		`^([A-Za-z_][A-Za-z0-9_]*)\s*\?=\s*(.*)$`,  // VAR ?= value
	})

	return parser, nil
}

// initializePatterns compiles all regex patterns
func (p *MakefileParser) initializePatterns() error {
	var err error

	// Target pattern: matches "target: dependencies"
	// Handles multiple targets and dependencies
	p.targetPattern, err = regexp.Compile(`^([^#\s:][^#:]*?):\s*([^#]*?)(?:\s*#\s*(.*))?$`)
	if err != nil {
		return fmt.Errorf("failed to compile target pattern: %w", err)
	}

	// Variable assignment pattern
	p.variablePattern, err = regexp.Compile(`^([A-Za-z_][A-Za-z0-9_]*)\s*[:=?+]=\s*(.*)$`)
	if err != nil {
		return fmt.Errorf("failed to compile variable pattern: %w", err)
	}

	// Comment pattern
	p.commentPattern, err = regexp.Compile(`^\s*#\s*(.*)$`)
	if err != nil {
		return fmt.Errorf("failed to compile comment pattern: %w", err)
	}

	// .PHONY pattern
	p.phonyPattern, err = regexp.Compile(`^\.PHONY\s*:\s*(.+)$`)
	if err != nil {
		return fmt.Errorf("failed to compile phony pattern: %w", err)
	}

	// Include pattern
	p.includePattern, err = regexp.Compile(`^-?include\s+(.+)$`)
	if err != nil {
		return fmt.Errorf("failed to compile include pattern: %w", err)
	}

	// Conditional pattern (ifdef, ifndef, ifeq, ifneq)
	p.conditionalPattern, err = regexp.Compile(`^(ifdef|ifndef|ifeq|ifneq)\s+(.*)$`)
	if err != nil {
		return fmt.Errorf("failed to compile conditional pattern: %w", err)
	}

	return nil
}

// Parse parses a Makefile and extracts tasks
func (p *MakefileParser) Parse(filePath string) ([]*task.Task, error) {
	return p.ParseFile(filePath)
}

// ParseFile implements Parser.ParseFile
func (p *MakefileParser) ParseFile(filePath string) ([]*task.Task, error) {
	p.logger.Debug("Parsing Makefile", "path", filePath)

	reader := NewFileReader(filePath, p.logger)
	lines, err := reader.ReadLines()
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	result := &ParseResult{
		Tasks:    make([]*task.Task, 0),
		Errors:   make([]ParseError, 0),
		Warnings: make([]string, 0),
		Metadata: make(map[string]any),
	}

	context := &MakefileContext{
		FilePath:     filePath,
		WorkingDir:   filepath.Dir(filePath),
		Variables:    make(map[string]string),
		PhonyTargets: make(map[string]bool),
		Comments:     make(map[int]string), // line number -> comment
	}

	// First pass: collect variables, phony targets, and comments
	if err := p.firstPass(lines, context); err != nil {
		return nil, fmt.Errorf("first pass failed: %w", err)
	}

	// Second pass: parse targets and create tasks
	if err := p.secondPass(lines, context, result); err != nil {
		return nil, fmt.Errorf("second pass failed: %w", err)
	}

	// Set metadata
	result.Metadata["variables"] = context.Variables
	result.Metadata["phony_targets"] = context.PhonyTargets
	result.Metadata["file_path"] = filePath

	p.logger.Debug("Parsed Makefile",
		"path", filePath,
		"tasks", len(result.Tasks),
		"variables", len(context.Variables),
		"phony_targets", len(context.PhonyTargets))

	return result.Tasks, nil
}

// ParseContent implements Parser.ParseContent
func (p *MakefileParser) ParseContent(reader io.Reader, filePath string) ([]*task.Task, error) {
	// Read all lines from the provided reader
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read content: %w", err)
	}
	lines := strings.Split(string(data), "\n")

	// Prepare a parsing context
	context := &MakefileContext{
		FilePath:     filePath,
		WorkingDir:   filepath.Dir(filePath),
		Variables:    make(map[string]string),
		PhonyTargets: make(map[string]bool),
		Comments:     make(map[int]string), // line number -> comment
	}

	result := &ParseResult{
		Tasks:    make([]*task.Task, 0),
		Errors:   make([]ParseError, 0),
		Warnings: make([]string, 0),
		Metadata: make(map[string]any),
	}

	if err := p.firstPass(lines, context); err != nil {
		return nil, fmt.Errorf("first pass failed: %w", err)
	}
	if err := p.secondPass(lines, context, result); err != nil {
		return nil, fmt.Errorf("second pass failed: %w", err)
	}
	return result.Tasks, nil
}

// MakefileContext holds context during parsing
type MakefileContext struct {
	FilePath     string
	WorkingDir   string
	Variables    map[string]string
	PhonyTargets map[string]bool
	Comments     map[int]string
}

// firstPass collects variables, phony targets, and comments
func (p *MakefileParser) firstPass(lines []string, context *MakefileContext) error {
	for lineNum, line := range lines {
		line = strings.TrimSpace(line)

		// Skip empty lines
		if line == "" {
			continue
		}

		// Extract comments
		if comment, found := p.commentExtractor.ExtractFromLine(line); found {
			context.Comments[lineNum+1] = comment
			continue
		}

		// Extract variables
		if name, value, found := p.variableExtractor.ExtractFromLine(line); found {
			// Expand variables in value
			expandedValue := p.expandVariables(value, context.Variables)
			context.Variables[name] = expandedValue
			continue
		}

		// Extract phony targets
		if matches := p.phonyPattern.FindStringSubmatch(line); len(matches) > 1 {
			phonyTargets := strings.Fields(matches[1])
			for _, target := range phonyTargets {
				context.PhonyTargets[strings.TrimSpace(target)] = true
			}
			continue
		}
	}

	return nil
}

// secondPass parses targets and creates tasks
func (p *MakefileParser) secondPass(lines []string, context *MakefileContext, result *ParseResult) error {
	for lineNum, line := range lines {
		originalLine := line
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Skip variable assignments and special targets
		if p.variablePattern.MatchString(line) ||
			p.phonyPattern.MatchString(line) ||
			p.includePattern.MatchString(line) ||
			p.conditionalPattern.MatchString(line) {
			continue
		}

		// Parse target lines
		if matches := p.targetPattern.FindStringSubmatch(line); len(matches) >= 2 {
			if err := p.parseTargetLine(matches, lineNum+1, context, result); err != nil {
				result.Errors = append(result.Errors, ParseError{
					Line:    lineNum + 1,
					Message: err.Error(),
					Context: originalLine,
					Fatal:   false,
				})
			}
		}
	}

	return nil
}

// parseTargetLine parses a single target line and creates tasks
func (p *MakefileParser) parseTargetLine(matches []string, lineNum int, context *MakefileContext, result *ParseResult) error {
	targetsStr := strings.TrimSpace(matches[1])
	dependenciesStr := ""
	inlineComment := ""

	if len(matches) > 2 {
		dependenciesStr = strings.TrimSpace(matches[2])
	}
	if len(matches) > 3 {
		inlineComment = strings.TrimSpace(matches[3])
	}

	// Parse multiple targets (e.g., "target1 target2: deps")
	targets := p.parseTargets(targetsStr)
	dependencies := p.parseDependencies(dependenciesStr)

	for _, targetName := range targets {
		// Skip special targets that start with . (like .PHONY, .DEFAULT, etc.)
		if strings.HasPrefix(targetName, ".") && targetName != ".PHONY" {
			continue
		}

		// Skip pattern rules (targets with %)
		if strings.Contains(targetName, "%") {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("Skipping pattern rule: %s", targetName))
			continue
		}

		task := p.createTaskFromTarget(targetName, dependencies, lineNum, context, inlineComment)
		result.Tasks = append(result.Tasks, task)
	}

	return nil
}

// parseTargets splits target string into individual target names
func (p *MakefileParser) parseTargets(targetsStr string) []string {
	// Handle escaped spaces and quotes
	var targets []string
	var current strings.Builder
	inQuotes := false
	escaped := false

	for _, char := range targetsStr {
		switch {
		case escaped:
			current.WriteRune(char)
			escaped = false
		case char == '\\':
			escaped = true
		case char == '"' || char == '\'':
			inQuotes = !inQuotes
		case char == ' ' && !inQuotes:
			if current.Len() > 0 {
				targets = append(targets, strings.TrimSpace(current.String()))
				current.Reset()
			}
		default:
			current.WriteRune(char)
		}
	}

	if current.Len() > 0 {
		targets = append(targets, strings.TrimSpace(current.String()))
	}

	return targets
}

// parseDependencies splits dependency string into individual dependencies
func (p *MakefileParser) parseDependencies(depsStr string) []string {
	if depsStr == "" {
		return nil
	}

	// Simple splitting for now - could be enhanced for complex cases
	deps := strings.Fields(depsStr)
	var cleanDeps []string

	for _, dep := range deps {
		dep = strings.TrimSpace(dep)
		if dep != "" {
			cleanDeps = append(cleanDeps, dep)
		}
	}

	return cleanDeps
}

// createTaskFromTarget creates a task object from a parsed target
func (p *MakefileParser) createTaskFromTarget(targetName string, dependencies []string, lineNum int, context *MakefileContext, inlineComment string) *task.Task {
	builder := NewTaskBuilder(targetName, task.RunnerMake, p.logger)

	// Set basic properties
	builder.SetFilePath(context.FilePath).
		SetWorkingDirectory(context.WorkingDir).
		SetCommand("make").
		AddArg(targetName)

	// Set description from comments
	description := p.getTargetDescription(targetName, lineNum, context, inlineComment)
	if description != "" {
		builder.SetDescription(description)
	}

	// Add dependencies
	if len(dependencies) > 0 {
		builder.AddDependencies(dependencies)
	}

	// Add metadata
	builder.AddMetadata("line_number", lineNum).
		AddMetadata("is_phony", context.PhonyTargets[targetName])

	// Add tags based on target characteristics
	p.addTargetTags(builder, targetName, context)

	// Set environment variables
	builder.SetEnvironment(context.Variables)

	return builder.Build()
}

// getTargetDescription extracts description for a target from comments
func (p *MakefileParser) getTargetDescription(_ string, lineNum int, context *MakefileContext, inlineComment string) string {
	// Use inline comment if available
	if inlineComment != "" {
		return CleanDescription(inlineComment)
	}

	// Look for comment on previous line
	if comment, exists := context.Comments[lineNum-1]; exists {
		return CleanDescription(comment)
	}

	// Look for comment on same line in original context
	if comment, exists := context.Comments[lineNum]; exists {
		return CleanDescription(comment)
	}

	return ""
}

// addTargetTags adds appropriate tags based on target characteristics
func (p *MakefileParser) addTargetTags(builder *TaskBuilder, targetName string, context *MakefileContext) {
	// Add phony tag
	if context.PhonyTargets[targetName] {
		builder.AddTag("phony")
	}

	// Add tags based on common target name patterns
	lowerName := strings.ToLower(targetName)

	switch {
	case strings.Contains(lowerName, "build"):
		builder.AddTag("build")
	case strings.Contains(lowerName, "test"):
		builder.AddTag("test")
	case strings.Contains(lowerName, "clean"):
		builder.AddTag("clean")
	case strings.Contains(lowerName, "install"):
		builder.AddTag("install")
	case strings.Contains(lowerName, "deploy"):
		builder.AddTag("deploy")
	case strings.Contains(lowerName, "lint"):
		builder.AddTag("lint")
	case strings.Contains(lowerName, "format"):
		builder.AddTag("format")
	case strings.Contains(lowerName, "doc"):
		builder.AddTag("documentation")
	case strings.Contains(lowerName, "release"):
		builder.AddTag("release")
	case strings.Contains(lowerName, "dev"):
		builder.AddTag("development")
	}

	// Add default tag for common targets
	commonTargets := []string{"all", "default", "help"}
	if slices.Contains(commonTargets, lowerName) {
		builder.AddTag("default")
	}
}

// expandVariables expands make variables in a string
func (p *MakefileParser) expandVariables(value string, variables map[string]string) string {
	// Simple variable expansion for $(VAR) and ${VAR}
	// This is a simplified version - make's variable expansion is much more complex

	result := value

	// Expand $(VAR) style variables
	dollarparen := regexp.MustCompile(`\$\(([^)]+)\)`)
	result = dollarparen.ReplaceAllStringFunc(result, func(match string) string {
		varName := match[2 : len(match)-1] // Remove $( and )
		if varValue, exists := variables[varName]; exists {
			return varValue
		}
		return match // Keep original if variable not found
	})

	// Expand ${VAR} style variables
	dollarbrace := regexp.MustCompile(`\$\{([^}]+)\}`)
	result = dollarbrace.ReplaceAllStringFunc(result, func(match string) string {
		varName := match[2 : len(match)-1] // Remove ${ and }
		if varValue, exists := variables[varName]; exists {
			return varValue
		}
		return match // Keep original if variable not found
	})

	return result
}

// GetMakefileInfo returns information about the Makefile structure
func (p *MakefileParser) GetMakefileInfo(filePath string) (*MakefileInfo, error) {
	reader := NewFileReader(filePath, p.logger)
	lines, err := reader.ReadLines()
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	context := &MakefileContext{
		FilePath:     filePath,
		WorkingDir:   filepath.Dir(filePath),
		Variables:    make(map[string]string),
		PhonyTargets: make(map[string]bool),
		Comments:     make(map[int]string),
	}

	if err := p.firstPass(lines, context); err != nil {
		return nil, fmt.Errorf("analysis failed: %w", err)
	}

	// Count targets
	targetCount := 0
	for _, line := range lines {
		if p.targetPattern.MatchString(strings.TrimSpace(line)) {
			targetCount++
		}
	}

	return &MakefileInfo{
		FilePath:      filePath,
		Variables:     context.Variables,
		PhonyTargets:  context.PhonyTargets,
		TargetCount:   targetCount,
		VariableCount: len(context.Variables),
		LineCount:     len(lines),
	}, nil
}

// MakefileInfo contains information about a parsed Makefile
type MakefileInfo struct {
	FilePath      string            `json:"file_path"`
	Variables     map[string]string `json:"variables"`
	PhonyTargets  map[string]bool   `json:"phony_targets"`
	TargetCount   int               `json:"target_count"`
	VariableCount int               `json:"variable_count"`
	LineCount     int               `json:"line_count"`
}

// Validate validates a Makefile without fully parsing it
func (p *MakefileParser) Validate(filePath string) error {
	reader := NewFileReader(filePath, p.logger)

	return reader.ReadLinesWithCallback(func(lineNum int, line string) error {
		line = strings.TrimSpace(line)

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			return nil
		}

		// Check for basic syntax errors
		if strings.Contains(line, ":") {
			// This looks like a target, basic validation
			if strings.HasPrefix(line, ":") {
				return fmt.Errorf("line %d: target name cannot be empty", lineNum)
			}
		}

		return nil
	})
}
