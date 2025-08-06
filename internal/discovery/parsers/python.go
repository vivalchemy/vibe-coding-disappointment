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

	"slices"

	"github.com/vivalchemy/wake/pkg/logger"
	"github.com/vivalchemy/wake/pkg/task"
)

// PythonParser parses Python scripts for executable tasks
type PythonParser struct {
	logger   logger.Logger
	patterns *PythonPatterns
	options  PythonOptions
}

var _ Parser = (*PythonParser)(nil)

// PythonOptions contains configuration for Python parsing
type PythonOptions struct {
	// File matching
	FileExtensions []string
	ScriptPatterns []string
	MaxFileSize    int64

	// Parsing options
	ParseDocstrings bool
	ParseArgparse   bool
	ParseClick      bool
	ParseFire       bool
	ParseCustom     bool
	ParseShebang    bool

	// Discovery options
	FindMainFunction  bool
	FindIfNameMain    bool
	FindEntryPoints   bool
	ParseClassMethods bool

	// Content options
	MaxFunctionLines int
	IncludePrivate   bool
	IncludeDocstring bool
	ParseDecorators  bool
}

// PythonPatterns contains regex patterns for parsing Python files
type PythonPatterns struct {
	// Function patterns
	FunctionDef  *regexp.Regexp
	ClassDef     *regexp.Regexp
	Method       *regexp.Regexp
	MainFunction *regexp.Regexp
	IfNameMain   *regexp.Regexp

	// Documentation patterns
	Docstring       *regexp.Regexp
	DocstringTriple *regexp.Regexp
	Comment         *regexp.Regexp

	// Import patterns
	Import     *regexp.Regexp
	FromImport *regexp.Regexp

	// Argument parsing patterns
	ArgparseParser *regexp.Regexp
	ArgparseAdd    *regexp.Regexp
	ClickCommand   *regexp.Regexp
	ClickOption    *regexp.Regexp
	FireMain       *regexp.Regexp

	// Decorator patterns
	Decorator *regexp.Regexp

	// Shebang pattern
	Shebang *regexp.Regexp

	// Entry point patterns
	SetupPy       *regexp.Regexp
	PyprojectToml *regexp.Regexp
}

// PythonTask represents a parsed Python task
type PythonTask struct {
	// Basic info
	Name        string
	Description string
	FilePath    string
	LineNumber  int

	// Function details
	FunctionName string
	ClassName    string
	Parameters   []PythonParameter
	ReturnType   string
	Decorators   []string

	// Documentation
	Docstring string
	Comments  []string

	// Argument parsing
	Arguments   []PythonArgument
	HasArgparse bool
	HasClick    bool
	HasFire     bool

	// Type information
	IsFunction bool
	IsMethod   bool
	IsMain     bool
	IsPrivate  bool
	IsAsync    bool

	// Dependencies
	Imports      []string
	Dependencies []string

	// Metadata
	ModTime time.Time
}

// PythonParameter represents a function parameter
type PythonParameter struct {
	Name         string
	Type         string
	DefaultValue string
	IsOptional   bool
	IsVarArgs    bool
	IsKwArgs     bool
	Annotation   string
}

// PythonArgument represents a command-line argument
type PythonArgument struct {
	Name     string
	Type     string
	Required bool
	Help     string
	Default  string
	Choices  []string
	Action   string
	Dest     string
}

// NewPythonParser creates a new Python parser
func NewPythonParser(logger logger.Logger) *PythonParser {
	return &PythonParser{
		logger:   logger.WithGroup("python-parser"),
		patterns: createPythonPatterns(),
		options:  createDefaultPythonOptions(),
	}
}

// createPythonPatterns creates regex patterns for parsing
func createPythonPatterns() *PythonPatterns {
	return &PythonPatterns{
		// Function and class definitions
		FunctionDef:  regexp.MustCompile(`^\s*def\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*\(([^)]*)\)\s*(?:->\s*([^:]+))?\s*:`),
		ClassDef:     regexp.MustCompile(`^\s*class\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*(?:\([^)]*\))?\s*:`),
		Method:       regexp.MustCompile(`^\s*def\s+([a-zA-Z_][a-zA-Z0-9_]*)\s*\(self[^)]*\)\s*(?:->\s*([^:]+))?\s*:`),
		MainFunction: regexp.MustCompile(`^\s*def\s+main\s*\(`),
		IfNameMain:   regexp.MustCompile(`^\s*if\s+__name__\s*==\s*['""]__main__['""]:`),

		// Documentation
		Docstring:       regexp.MustCompile(`^\s*['"]{3}([^'"]*)['"]{3}`),
		DocstringTriple: regexp.MustCompile(`^\s*['"]{3}(.*?)$`),
		Comment:         regexp.MustCompile(`^\s*#\s*(.*)`),

		// Imports
		Import:     regexp.MustCompile(`^\s*import\s+([a-zA-Z_][a-zA-Z0-9_.,\s]*)`),
		FromImport: regexp.MustCompile(`^\s*from\s+([a-zA-Z_][a-zA-Z0-9_.]*)\s+import\s+([a-zA-Z_][a-zA-Z0-9_.,\s*]*)`),

		// Argument parsing frameworks
		ArgparseParser: regexp.MustCompile(`argparse\.ArgumentParser\(`),
		ArgparseAdd:    regexp.MustCompile(`\.add_argument\(\s*['""]([^'"]*)['""]`),
		ClickCommand:   regexp.MustCompile(`@click\.(command|group)`),
		ClickOption:    regexp.MustCompile(`@click\.option\(\s*['""]([^'"]*)['""]`),
		FireMain:       regexp.MustCompile(`fire\.Fire\(`),

		// Decorators
		Decorator: regexp.MustCompile(`^\s*@([a-zA-Z_][a-zA-Z0-9_.]*)`),

		// Shebang
		Shebang: regexp.MustCompile(`^#!/.*python[0-9]*`),

		// Entry points
		SetupPy:       regexp.MustCompile(`console_scripts`),
		PyprojectToml: regexp.MustCompile(`\[tool\.poetry\.scripts\]`),
	}
}

// createDefaultPythonOptions creates default parsing options
func createDefaultPythonOptions() PythonOptions {
	return PythonOptions{
		FileExtensions: []string{".py", ".pyw"},
		ScriptPatterns: []string{"*.py", "*.pyw"},
		MaxFileSize:    5 * 1024 * 1024, // 5MB

		ParseDocstrings: true,
		ParseArgparse:   true,
		ParseClick:      true,
		ParseFire:       true,
		ParseCustom:     true,
		ParseShebang:    true,

		FindMainFunction:  true,
		FindIfNameMain:    true,
		FindEntryPoints:   true,
		ParseClassMethods: false, // Usually not executable tasks

		MaxFunctionLines: 200,
		IncludePrivate:   false,
		IncludeDocstring: true,
		ParseDecorators:  true,
	}
}

func (pp *PythonParser) GetRunnerType() task.RunnerType {
	return task.RunnerPython
}

func (pp *PythonParser) Parse(filePath string) ([]*task.Task, error) {
	return pp.ParseFile(filePath)
}

// SetOptions sets parsing options
func (pp *PythonParser) SetOptions(options PythonOptions) {
	pp.options = options
}

// GetSupportedFiles returns supported file patterns
func (pp *PythonParser) GetSupportedFiles() []string {
	return pp.options.ScriptPatterns
}

// CanParse checks if the parser can handle the given file
func (pp *PythonParser) CanParse(filePath string) bool {
	ext := strings.ToLower(filepath.Ext(filePath))

	return slices.Contains(pp.options.FileExtensions, ext)
}

// ParseFile parses a Python file and returns discovered tasks
func (pp *PythonParser) ParseFile(filePath string) ([]*task.Task, error) {
	pp.logger.Debug("Parsing Python file", "file", filePath)

	// Check if we can parse this file
	if !pp.CanParse(filePath) {
		return nil, fmt.Errorf("unsupported file: %s", filePath)
	}

	// Check file size
	if err := pp.checkFileSize(filePath); err != nil {
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
	pythonTasks, err := pp.parseContent(file, filePath, fileInfo.ModTime())
	if err != nil {
		return nil, fmt.Errorf("failed to parse content: %w", err)
	}

	// Convert to generic tasks
	tasks := pp.convertToTasks(pythonTasks, filePath)

	pp.logger.Info("Parsed Python file", "file", filePath, "tasks", len(tasks))
	return tasks, nil
}

// ParseContent parses Python content from a reader
func (pp *PythonParser) ParseContent(reader io.Reader, filePath string) ([]*task.Task, error) {
	pythonTasks, err := pp.parseContent(reader, filePath, time.Now())
	if err != nil {
		return nil, err
	}

	return pp.convertToTasks(pythonTasks, filePath), nil
}

// checkFileSize checks if file size is within limits
func (pp *PythonParser) checkFileSize(filePath string) error {
	if pp.options.MaxFileSize <= 0 {
		return nil
	}

	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return err
	}

	if fileInfo.Size() > pp.options.MaxFileSize {
		return fmt.Errorf("file too large: %d bytes (max: %d)", fileInfo.Size(), pp.options.MaxFileSize)
	}

	return nil
}

// parseContent parses the actual Python content
func (pp *PythonParser) parseContent(reader io.Reader, filePath string, modTime time.Time) ([]*PythonTask, error) {
	scanner := bufio.NewScanner(reader)

	var tasks []*PythonTask
	var imports []string
	var currentClass string
	var currentFunction *PythonTask
	var lineNumber int
	var pendingComments []string
	var inDocstring bool
	var docstringLines []string
	var hasShebang bool
	var hasIfNameMain bool
	var argparseUsage bool
	var clickUsage bool
	var fireUsage bool

	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()
		originalLine := line
		line = strings.TrimSpace(line)

		// Check for shebang on first line
		if lineNumber == 1 && pp.options.ParseShebang {
			if pp.patterns.Shebang.MatchString(line) {
				hasShebang = true
			}
		}

		// Skip empty lines
		if line == "" {
			continue
		}

		// Handle docstrings
		if pp.patterns.DocstringTriple.MatchString(line) {
			if inDocstring {
				// End of docstring
				inDocstring = false
				if currentFunction != nil && pp.options.IncludeDocstring {
					currentFunction.Docstring = strings.Join(docstringLines, "\n")
				}
				docstringLines = nil
			} else {
				// Start of docstring
				inDocstring = true
				docstringLines = []string{}
			}
			continue
		}

		if inDocstring {
			docstringLines = append(docstringLines, strings.TrimSpace(line))
			continue
		}

		// Parse comments
		if pp.patterns.Comment.MatchString(line) {
			comment := pp.parseComment(line)
			if comment != "" {
				pendingComments = append(pendingComments, comment)
			}
			continue
		}

		// Parse imports
		if pp.patterns.Import.MatchString(line) || pp.patterns.FromImport.MatchString(line) {
			import_ := pp.parseImport(line)
			if import_ != "" {
				imports = append(imports, import_)

				// Check for argument parsing frameworks
				if strings.Contains(import_, "argparse") {
					argparseUsage = true
				} else if strings.Contains(import_, "click") {
					clickUsage = true
				} else if strings.Contains(import_, "fire") {
					fireUsage = true
				}
			}
			continue
		}

		// Check for if __name__ == "__main__"
		if pp.options.FindIfNameMain && pp.patterns.IfNameMain.MatchString(line) {
			hasIfNameMain = true
			continue
		}

		// Check for argument parsing usage
		if pp.options.ParseArgparse && pp.patterns.ArgparseParser.MatchString(line) {
			argparseUsage = true
		}
		if pp.options.ParseClick && pp.patterns.ClickCommand.MatchString(line) {
			clickUsage = true
		}
		if pp.options.ParseFire && pp.patterns.FireMain.MatchString(line) {
			fireUsage = true
		}

		// Parse class definitions
		if pp.patterns.ClassDef.MatchString(line) {
			currentClass = pp.parseClass(line)
			continue
		}

		// Parse function definitions
		if pp.patterns.FunctionDef.MatchString(line) {
			// Save previous function if any
			if currentFunction != nil {
				tasks = append(tasks, currentFunction)
			}

			// Parse new function
			var err error
			currentFunction, err = pp.parseFunction(line, filePath, lineNumber, modTime)
			if err != nil {
				pp.logger.Warn("Failed to parse function", "line", lineNumber, "error", err)
				currentFunction = nil
				continue
			}

			// Set class context
			if currentClass != "" {
				currentFunction.ClassName = currentClass
				currentFunction.IsMethod = true
			}

			// Check if it's a main function
			if pp.options.FindMainFunction && currentFunction.FunctionName == "main" {
				currentFunction.IsMain = true
			}

			// Check if it's private
			currentFunction.IsPrivate = strings.HasPrefix(currentFunction.FunctionName, "_")

			// Add pending comments as description
			if len(pendingComments) > 0 {
				currentFunction.Description = strings.Join(pendingComments, " ")
				currentFunction.Comments = append(currentFunction.Comments, pendingComments...)
				pendingComments = nil
			}

			// Add imports
			currentFunction.Imports = append(currentFunction.Imports, imports...)

			// Set framework usage flags
			currentFunction.HasArgparse = argparseUsage
			currentFunction.HasClick = clickUsage
			currentFunction.HasFire = fireUsage

			continue
		}

		// Parse decorators
		if pp.options.ParseDecorators && pp.patterns.Decorator.MatchString(line) {
			decorator := pp.parseDecorator(line)
			if currentFunction != nil {
				currentFunction.Decorators = append(currentFunction.Decorators, decorator)
			}
			continue
		}

		// Parse argument parsing within functions
		if currentFunction != nil {
			if pp.options.ParseArgparse && pp.patterns.ArgparseAdd.MatchString(line) {
				arg := pp.parseArgparseArgument(line)
				if arg.Name != "" {
					currentFunction.Arguments = append(currentFunction.Arguments, arg)
				}
			}

			if pp.options.ParseClick && pp.patterns.ClickOption.MatchString(line) {
				arg := pp.parseClickOption(line)
				if arg.Name != "" {
					currentFunction.Arguments = append(currentFunction.Arguments, arg)
				}
			}
		}

		// Clear pending comments if they weren't used
		if !strings.HasPrefix(originalLine, " ") && !strings.HasPrefix(originalLine, "\t") {
			pendingComments = nil
		}
	}

	// Save last function if any
	if currentFunction != nil {
		tasks = append(tasks, currentFunction)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	// Add special tasks for executable scripts
	if hasShebang && hasIfNameMain {
		scriptTask := &PythonTask{
			Name:        "run-script",
			Description: "Execute Python script",
			FilePath:    filePath,
			LineNumber:  1,
			IsMain:      true,
			Imports:     imports,
			ModTime:     modTime,
		}
		tasks = append(tasks, scriptTask)
	}

	// Filter private functions if needed
	if !pp.options.IncludePrivate {
		tasks = pp.filterPrivateTasks(tasks)
	}

	// Filter class methods if needed
	if !pp.options.ParseClassMethods {
		tasks = pp.filterClassMethods(tasks)
	}

	return tasks, nil
}

// parseComment parses a comment line
func (pp *PythonParser) parseComment(line string) string {
	matches := pp.patterns.Comment.FindStringSubmatch(line)
	if len(matches) > 1 {
		return strings.TrimSpace(matches[1])
	}
	return ""
}

// parseImport parses an import statement
func (pp *PythonParser) parseImport(line string) string {
	if matches := pp.patterns.Import.FindStringSubmatch(line); len(matches) > 1 {
		return matches[1]
	}
	if matches := pp.patterns.FromImport.FindStringSubmatch(line); len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// parseClass parses a class definition
func (pp *PythonParser) parseClass(line string) string {
	matches := pp.patterns.ClassDef.FindStringSubmatch(line)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// parseFunction parses a function definition
func (pp *PythonParser) parseFunction(line, filePath string, lineNumber int, modTime time.Time) (*PythonTask, error) {
	matches := pp.patterns.FunctionDef.FindStringSubmatch(line)
	if len(matches) < 2 {
		return nil, fmt.Errorf("invalid function definition format")
	}

	functionName := matches[1]
	paramsStr := ""
	returnType := ""

	if len(matches) > 2 {
		paramsStr = matches[2]
	}
	if len(matches) > 3 {
		returnType = matches[3]
	}

	task := &PythonTask{
		Name:         functionName,
		FilePath:     filePath,
		LineNumber:   lineNumber,
		FunctionName: functionName,
		ReturnType:   strings.TrimSpace(returnType),
		Parameters:   []PythonParameter{},
		Arguments:    []PythonArgument{},
		Decorators:   []string{},
		Comments:     []string{},
		Imports:      []string{},
		Dependencies: []string{},
		IsFunction:   true,
		ModTime:      modTime,
	}

	// Check if async
	if strings.Contains(line, "async def") {
		task.IsAsync = true
	}

	// Parse parameters
	if paramsStr != "" {
		task.Parameters = pp.parseParameters(paramsStr)
	}

	return task, nil
}

// parseParameters parses function parameters
func (pp *PythonParser) parseParameters(paramsStr string) []PythonParameter {
	var params []PythonParameter

	// Simple parameter parsing - could be enhanced for complex cases
	paramTokens := strings.SplitSeq(paramsStr, ",")

	for token := range paramTokens {
		param := pp.parseParameter(strings.TrimSpace(token))
		if param.Name != "" {
			params = append(params, param)
		}
	}

	return params
}

// parseParameter parses a single parameter
func (pp *PythonParser) parseParameter(paramStr string) PythonParameter {
	param := PythonParameter{}

	// Handle *args and **kwargs
	if strings.HasPrefix(paramStr, "**") {
		param.Name = paramStr[2:]
		param.IsKwArgs = true
		return param
	}
	if strings.HasPrefix(paramStr, "*") {
		param.Name = paramStr[1:]
		param.IsVarArgs = true
		return param
	}

	// Split by = for default values
	parts := strings.SplitN(paramStr, "=", 2)
	nameAndType := strings.TrimSpace(parts[0])

	if len(parts) > 1 {
		param.DefaultValue = strings.TrimSpace(parts[1])
		param.IsOptional = true
	}

	// Split by : for type annotations
	typeParts := strings.SplitN(nameAndType, ":", 2)
	param.Name = strings.TrimSpace(typeParts[0])

	if len(typeParts) > 1 {
		param.Type = strings.TrimSpace(typeParts[1])
		param.Annotation = param.Type
	}

	return param
}

// parseDecorator parses a decorator
func (pp *PythonParser) parseDecorator(line string) string {
	matches := pp.patterns.Decorator.FindStringSubmatch(line)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// parseArgparseArgument parses an argparse add_argument call
func (pp *PythonParser) parseArgparseArgument(line string) PythonArgument {
	matches := pp.patterns.ArgparseAdd.FindStringSubmatch(line)
	if len(matches) > 1 {
		return PythonArgument{
			Name:     matches[1],
			Required: !strings.HasPrefix(matches[1], "--"),
		}
	}
	return PythonArgument{}
}

// parseClickOption parses a Click option decorator
func (pp *PythonParser) parseClickOption(line string) PythonArgument {
	matches := pp.patterns.ClickOption.FindStringSubmatch(line)
	if len(matches) > 1 {
		return PythonArgument{
			Name:     matches[1],
			Required: false, // Click options are usually optional
		}
	}
	return PythonArgument{}
}

// filterPrivateTasks filters out private tasks
func (pp *PythonParser) filterPrivateTasks(tasks []*PythonTask) []*PythonTask {
	var filtered []*PythonTask

	for _, task := range tasks {
		if !task.IsPrivate {
			filtered = append(filtered, task)
		}
	}

	return filtered
}

// filterClassMethods filters out class methods
func (pp *PythonParser) filterClassMethods(tasks []*PythonTask) []*PythonTask {
	var filtered []*PythonTask

	for _, task := range tasks {
		if !task.IsMethod {
			filtered = append(filtered, task)
		}
	}

	return filtered
}

// convertToTasks converts PythonTasks to generic Tasks
func (pp *PythonParser) convertToTasks(pythonTasks []*PythonTask, filePath string) []*task.Task {
	var tasks []*task.Task

	for _, pt := range pythonTasks {
		t := &task.Task{
			ID:               generatePythonTaskID(pt.Name, filePath, pt.LineNumber),
			Name:             pt.Name,
			Description:      pt.Description,
			Runner:           task.RunnerPython,
			FilePath:         filePath,
			WorkingDirectory: filepath.Dir(filePath),
			Command:          pp.buildCommand(pt),
			Args:             pp.buildArgs(pt),
			Environment:      pp.buildEnvironment(pt),
			Tags:             pp.buildTags(pt),
			Dependencies:     pt.Dependencies,
			Hidden:           pt.IsPrivate,
			DiscoveredAt:     time.Now(),
			Status:           task.StatusIdle,
		}

		// Add metadata
		t.Metadata = map[string]any{
			"python_function":     pt.FunctionName,
			"python_class":        pt.ClassName,
			"python_parameters":   pt.Parameters,
			"python_arguments":    pt.Arguments,
			"python_decorators":   pt.Decorators,
			"python_docstring":    pt.Docstring,
			"python_comments":     pt.Comments,
			"python_imports":      pt.Imports,
			"python_return_type":  pt.ReturnType,
			"python_is_async":     pt.IsAsync,
			"python_is_main":      pt.IsMain,
			"python_is_method":    pt.IsMethod,
			"python_has_argparse": pt.HasArgparse,
			"python_has_click":    pt.HasClick,
			"python_has_fire":     pt.HasFire,
			"file_modified":       pt.ModTime,
		}

		tasks = append(tasks, t)
	}

	return tasks
}

// buildCommand builds the command for running the task
func (pp *PythonParser) buildCommand(_ *PythonTask) string {
	return "python"
}

// buildArgs builds the arguments for running the task
func (pp *PythonParser) buildArgs(pt *PythonTask) []string {
	if pt.Name == "run-script" {
		// For script execution
		return []string{filepath.Base(pt.FilePath)}
	}

	// For function execution, we'd need a wrapper script or use -c
	args := []string{"-c"}

	if pt.ClassName != "" {
		// Method call
		args = append(args, fmt.Sprintf("from %s import %s; %s().%s()",
			strings.TrimSuffix(filepath.Base(pt.FilePath), ".py"),
			pt.ClassName,
			pt.ClassName,
			pt.FunctionName))
	} else {
		// Function call
		args = append(args, fmt.Sprintf("from %s import %s; %s()",
			strings.TrimSuffix(filepath.Base(pt.FilePath), ".py"),
			pt.FunctionName,
			pt.FunctionName))
	}

	return args
}

// buildEnvironment builds environment variables
func (pp *PythonParser) buildEnvironment(pt *PythonTask) map[string]string {
	env := make(map[string]string)

	// Add Python-specific environment variables
	env["PYTHONPATH"] = filepath.Dir(pt.FilePath)

	return env
}

// buildTags builds tags for the task
func (pp *PythonParser) buildTags(pt *PythonTask) []string {
	var tags []string

	// Add runner tag
	tags = append(tags, "python")

	// Add type tags
	if pt.IsFunction {
		tags = append(tags, "function")
	}
	if pt.IsMethod {
		tags = append(tags, "method")
	}
	if pt.IsMain {
		tags = append(tags, "main")
	}
	if pt.IsAsync {
		tags = append(tags, "async")
	}

	// Add framework tags
	if pt.HasArgparse {
		tags = append(tags, "argparse")
	}
	if pt.HasClick {
		tags = append(tags, "click")
	}
	if pt.HasFire {
		tags = append(tags, "fire")
	}

	// Add parameter tags
	if len(pt.Parameters) > 0 {
		tags = append(tags, "parameterized")
	}

	// Add decorator tags
	for _, decorator := range pt.Decorators {
		tags = append(tags, fmt.Sprintf("decorator:%s", decorator))
	}

	// Add visibility tag
	if pt.IsPrivate {
		tags = append(tags, "private")
	}

	return tags
}

// generatePythonTaskID generates a unique task ID
func generatePythonTaskID(name, filePath string, lineNumber int) string {
	return fmt.Sprintf("python:%s:%s:%d", filepath.Base(filePath), name, lineNumber)
}

// GetMetadata returns parser metadata
func (pp *PythonParser) GetMetadata() map[string]any {
	return map[string]any{
		"parser_type":          "python",
		"supported_extensions": pp.options.FileExtensions,
		"max_file_size":        pp.options.MaxFileSize,
		"parse_docstrings":     pp.options.ParseDocstrings,
		"parse_argparse":       pp.options.ParseArgparse,
		"parse_click":          pp.options.ParseClick,
		"parse_fire":           pp.options.ParseFire,
		"find_main_function":   pp.options.FindMainFunction,
		"find_if_name_main":    pp.options.FindIfNameMain,
		"parse_class_methods":  pp.options.ParseClassMethods,
		"include_private":      pp.options.IncludePrivate,
		"max_function_lines":   pp.options.MaxFunctionLines,
	}
}

// Validate validates a Python file for syntax errors
func (pp *PythonParser) Validate(filePath string) error {
	pp.logger.Debug("Validating Python file", "file", filePath)

	// Basic validation - could be enhanced with AST parsing
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineNumber := 0
	var errors []string
	indentStack := []int{0}

	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()

		// Basic indentation check
		if strings.TrimSpace(line) != "" {
			indent := len(line) - len(strings.TrimLeft(line, " \t"))

			// Check for consistent indentation
			if indent > indentStack[len(indentStack)-1] {
				indentStack = append(indentStack, indent)
			} else if indent < indentStack[len(indentStack)-1] {
				// Pop indentation levels
				for len(indentStack) > 1 && indent < indentStack[len(indentStack)-1] {
					indentStack = indentStack[:len(indentStack)-1]
				}
				if indent != indentStack[len(indentStack)-1] {
					errors = append(errors, fmt.Sprintf("line %d: inconsistent indentation", lineNumber))
				}
			}
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("validation failed:\n%s", strings.Join(errors, "\n"))
	}

	return nil
}

// GetStatistics returns parsing statistics
func (pp *PythonParser) GetStatistics() map[string]any {
	return map[string]any{
		"parser_type":          "python",
		"version":              "1.0.0",
		"supported_extensions": len(pp.options.FileExtensions),
		"pattern_count":        12,
	}
}
