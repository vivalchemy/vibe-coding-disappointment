package parsers

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/vivalchemy/wake/pkg/logger"
	"github.com/vivalchemy/wake/pkg/task"
)

// PackageJSONParser parses package.json scripts and extracts tasks
type PackageJSONParser struct {
	*BaseParser

	// Comment patterns for extracting descriptions from scripts
	commentPattern *regexp.Regexp

	// Comment extractor
	commentExtractor *CommentExtractor
}

var _ Parser = (*PackageJSONParser)(nil)

// PackageJSON represents the structure of a package.json file
type PackageJSON struct {
	Name            string            `json:"name"`
	Version         string            `json:"version"`
	Description     string            `json:"description"`
	Scripts         map[string]any    `json:"scripts"`
	Dependencies    map[string]string `json:"dependencies"`
	DevDependencies map[string]string `json:"devDependencies"`
	Engines         map[string]string `json:"engines"`
	Workspaces      any               `json:"workspaces"`
	Private         bool              `json:"private"`
	Type            string            `json:"type"`
	Main            string            `json:"main"`
	Module          string            `json:"module"`

	// Custom fields that might contain script metadata
	Config      map[string]any `json:"config"`
	ScriptsMeta map[string]any `json:"scripts-meta"`
	ScriptsInfo map[string]any `json:"scripts-info"`
}

// ScriptInfo holds metadata about a script
type ScriptInfo struct {
	Name        string
	Command     string
	Description string
	Tags        []string
	Hidden      bool
	PreScript   string
	PostScript  string
}

// NewPackageJSONParser creates a new package.json parser
func NewPackageJSONParser(log logger.Logger) (*PackageJSONParser, error) {
	patterns := []string{"package.json"}
	base := NewBaseParser(task.RunnerNpm, patterns, log)

	parser := &PackageJSONParser{
		BaseParser: base,
	}

	if err := parser.initializePatterns(); err != nil {
		return nil, fmt.Errorf("failed to initialize patterns: %w", err)
	}

	parser.commentExtractor = NewCommentExtractor([]string{"//", "#"})

	return parser, nil
}

// initializePatterns compiles regex patterns
func (p *PackageJSONParser) initializePatterns() error {
	var err error

	// Pattern to extract comments from script commands
	p.commentPattern, err = regexp.Compile(`#\s*(.+?)(?:\s*&&|\s*\|\||$)`)
	if err != nil {
		return fmt.Errorf("failed to compile comment pattern: %w", err)
	}

	return nil
}

// Parse parses a package.json file and extracts script tasks
func (p *PackageJSONParser) Parse(filePath string) ([]*task.Task, error) {
	return p.ParseFile(filePath)
}

// ParseFile implements Parser.ParseFile
func (p *PackageJSONParser) ParseFile(filePath string) ([]*task.Task, error) {
	p.logger.Debug("Parsing package.json", "path", filePath)

	// Read and parse JSON
	packageData, err := p.readPackageJSON(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read package.json: %w", err)
	}

	// Determine the appropriate runner type
	runnerType := p.determineRunnerType(filepath.Dir(filePath))

	var tasks []*task.Task

	// Parse scripts
	if packageData.Scripts != nil {
		scriptTasks, err := p.parseScripts(packageData, filePath, runnerType)
		if err != nil {
			return nil, fmt.Errorf("failed to parse scripts: %w", err)
		}
		tasks = append(tasks, scriptTasks...)
	}

	p.logger.Debug("Parsed package.json",
		"path", filePath,
		"tasks", len(tasks),
		"runner", runnerType)

	return tasks, nil
}

// ParseContent implements Parser.ParseContent
func (p *PackageJSONParser) ParseContent(reader io.Reader, filePath string) ([]*task.Task, error) {
	// Read all content from the io.Reader
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read content: %w", err)
	}

	// Unmarshal the JSON
	var packageData PackageJSON
	if err := json.Unmarshal(data, &packageData); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Determine runner type based on file path (if possible)
	runnerType := p.determineRunnerType(filepath.Dir(filePath))

	// Parse scripts as usual
	var tasks []*task.Task
	if packageData.Scripts != nil {
		scriptTasks, err := p.parseScripts(&packageData, filePath, runnerType)
		if err != nil {
			return nil, fmt.Errorf("failed to parse scripts: %w", err)
		}
		tasks = append(tasks, scriptTasks...)
	}

	return tasks, nil
}

// readPackageJSON reads and parses the package.json file
func (p *PackageJSONParser) readPackageJSON(filePath string) (*PackageJSON, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	var packageData PackageJSON
	if err := json.Unmarshal(data, &packageData); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return &packageData, nil
}

// determineRunnerType determines which npm-family runner to use based on lock files
func (p *PackageJSONParser) determineRunnerType(projectDir string) task.RunnerType {
	// Check for lock files in order of preference
	lockFiles := []struct {
		file   string
		runner task.RunnerType
	}{
		{"pnpm-lock.yaml", task.RunnerPnpm},
		{"yarn.lock", task.RunnerYarn},
		{"bun.lockb", task.RunnerBun},
		{"package-lock.json", task.RunnerNpm},
	}

	for _, lockFile := range lockFiles {
		lockPath := filepath.Join(projectDir, lockFile.file)
		if _, err := os.Stat(lockPath); err == nil {
			p.logger.Debug("Detected runner from lock file", "file", lockFile.file, "runner", lockFile.runner)
			return lockFile.runner
		}
	}

	// Default to npm if no lock file found
	return task.RunnerNpm
}

// parseScripts parses the scripts section and creates tasks
func (p *PackageJSONParser) parseScripts(packageData *PackageJSON, filePath string, runnerType task.RunnerType) ([]*task.Task, error) {
	var tasks []*task.Task
	workingDir := filepath.Dir(filePath)

	for scriptName, scriptValue := range packageData.Scripts {
		scriptInfo, err := p.parseScriptValue(scriptName, scriptValue)
		if err != nil {
			p.logger.Warn("Failed to parse script", "name", scriptName, "error", err)
			continue
		}

		// Create task
		taskObj := p.createTaskFromScript(scriptInfo, packageData, filePath, workingDir, runnerType)
		tasks = append(tasks, taskObj)

		// Create pre/post script tasks if they exist
		if preTask := p.createPrePostScriptTask(scriptName, "pre", packageData, filePath, workingDir, runnerType); preTask != nil {
			tasks = append(tasks, preTask)
		}

		if postTask := p.createPrePostScriptTask(scriptName, "post", packageData, filePath, workingDir, runnerType); postTask != nil {
			tasks = append(tasks, postTask)
		}
	}

	return tasks, nil
}

// parseScriptValue parses a script value which can be string or object
func (p *PackageJSONParser) parseScriptValue(name string, value any) (*ScriptInfo, error) {
	scriptInfo := &ScriptInfo{
		Name: name,
		Tags: make([]string, 0),
	}

	switch v := value.(type) {
	case string:
		scriptInfo.Command = v
		scriptInfo.Description = p.extractDescriptionFromCommand(v)

	case map[string]any:
		// Handle object-style script definition (custom extension)
		if cmd, ok := v["command"].(string); ok {
			scriptInfo.Command = cmd
		} else if cmd, ok := v["cmd"].(string); ok {
			scriptInfo.Command = cmd
		}

		if desc, ok := v["description"].(string); ok {
			scriptInfo.Description = desc
		}

		if tags, ok := v["tags"].([]any); ok {
			for _, tag := range tags {
				if tagStr, ok := tag.(string); ok {
					scriptInfo.Tags = append(scriptInfo.Tags, tagStr)
				}
			}
		}

		if hidden, ok := v["hidden"].(bool); ok {
			scriptInfo.Hidden = hidden
		}

	default:
		return nil, fmt.Errorf("unsupported script value type: %T", value)
	}

	if scriptInfo.Command == "" {
		return nil, fmt.Errorf("script command cannot be empty")
	}

	return scriptInfo, nil
}

// extractDescriptionFromCommand tries to extract description from script comments
func (p *PackageJSONParser) extractDescriptionFromCommand(command string) string {
	// Look for comments in the command
	if matches := p.commentPattern.FindStringSubmatch(command); len(matches) > 1 {
		return CleanDescription(matches[1])
	}

	// Try to infer description from common script patterns
	return p.inferDescriptionFromScript(command)
}

// inferDescriptionFromScript infers description based on common script patterns
func (p *PackageJSONParser) inferDescriptionFromScript(command string) string {
	lowerCmd := strings.ToLower(command)

	// Common patterns
	patterns := map[string]string{
		"build":       "Build the project",
		"test":        "Run tests",
		"start":       "Start the application",
		"dev":         "Start development server",
		"lint":        "Lint code",
		"format":      "Format code",
		"clean":       "Clean build artifacts",
		"deploy":      "Deploy the application",
		"serve":       "Serve the application",
		"watch":       "Watch for changes",
		"compile":     "Compile source code",
		"bundle":      "Bundle application",
		"typecheck":   "Type check code",
		"coverage":    "Generate test coverage",
		"docs":        "Generate documentation",
		"release":     "Create a release",
		"publish":     "Publish package",
		"install":     "Install dependencies",
		"update":      "Update dependencies",
		"outdated":    "Check for outdated dependencies",
		"audit":       "Audit dependencies for vulnerabilities",
		"bench":       "Run benchmarks",
		"e2e":         "Run end-to-end tests",
		"unit":        "Run unit tests",
		"integration": "Run integration tests",
	}

	for pattern, description := range patterns {
		if strings.Contains(lowerCmd, pattern) {
			return description
		}
	}

	return ""
}

// createTaskFromScript creates a task from script info
func (p *PackageJSONParser) createTaskFromScript(scriptInfo *ScriptInfo, packageData *PackageJSON, filePath, workingDir string, runnerType task.RunnerType) *task.Task {
	builder := NewTaskBuilder(scriptInfo.Name, runnerType, p.logger)

	// Set basic properties
	builder.SetFilePath(filePath).
		SetWorkingDirectory(workingDir).
		SetDescription(scriptInfo.Description)

	// Set command based on runner type
	p.setRunnerCommand(builder, runnerType, scriptInfo.Name)

	// Add tags
	builder.AddTags(scriptInfo.Tags)
	p.addScriptTags(builder, scriptInfo.Name, scriptInfo.Command)

	// Set hidden flag
	builder.SetHidden(scriptInfo.Hidden)

	// Add metadata
	builder.AddMetadata("package_name", packageData.Name).
		AddMetadata("package_version", packageData.Version).
		AddMetadata("script_command", scriptInfo.Command).
		AddMetadata("runner_detected", runnerType.String())

	// Add environment from package config
	if packageData.Config != nil {
		env := make(map[string]string)
		for key, value := range packageData.Config {
			if strValue, ok := value.(string); ok {
				env[fmt.Sprintf("npm_config_%s", key)] = strValue
			}
		}
		builder.SetEnvironment(env)
	}

	return builder.Build()
}

// setRunnerCommand sets the appropriate command and args for the runner type
func (p *PackageJSONParser) setRunnerCommand(builder *TaskBuilder, runnerType task.RunnerType, scriptName string) {
	switch runnerType {
	case task.RunnerNpm:
		builder.SetCommand("npm").AddArg("run").AddArg(scriptName)
	case task.RunnerYarn:
		builder.SetCommand("yarn").AddArg(scriptName)
	case task.RunnerPnpm:
		builder.SetCommand("pnpm").AddArg("run").AddArg(scriptName)
	case task.RunnerBun:
		builder.SetCommand("bun").AddArg("run").AddArg(scriptName)
	default:
		builder.SetCommand("npm").AddArg("run").AddArg(scriptName)
	}
}

// addScriptTags adds tags based on script name and command patterns
func (p *PackageJSONParser) addScriptTags(builder *TaskBuilder, scriptName, command string) {
	lowerName := strings.ToLower(scriptName)
	lowerCmd := strings.ToLower(command)

	// Tag based on script name
	namePatterns := map[string]string{
		"build":       "build",
		"test":        "test",
		"start":       "start",
		"dev":         "development",
		"lint":        "lint",
		"format":      "format",
		"clean":       "clean",
		"deploy":      "deploy",
		"serve":       "serve",
		"watch":       "watch",
		"compile":     "compile",
		"bundle":      "bundle",
		"typecheck":   "typecheck",
		"coverage":    "coverage",
		"docs":        "documentation",
		"release":     "release",
		"publish":     "publish",
		"bench":       "benchmark",
		"e2e":         "e2e-test",
		"unit":        "unit-test",
		"integration": "integration-test",
	}

	for pattern, tag := range namePatterns {
		if strings.Contains(lowerName, pattern) {
			builder.AddTag(tag)
		}
	}

	// Tag based on command content
	cmdPatterns := map[string]string{
		"jest":         "jest",
		"mocha":        "mocha",
		"cypress":      "cypress",
		"playwright":   "playwright",
		"webpack":      "webpack",
		"rollup":       "rollup",
		"vite":         "vite",
		"parcel":       "parcel",
		"eslint":       "eslint",
		"prettier":     "prettier",
		"typescript":   "typescript",
		"tsc":          "typescript",
		"nodemon":      "nodemon",
		"concurrently": "concurrent",
		"rimraf":       "cleanup",
		"cross-env":    "cross-platform",
	}

	for pattern, tag := range cmdPatterns {
		if strings.Contains(lowerCmd, pattern) {
			builder.AddTag(tag)
		}
	}

	// Special script name patterns
	if strings.HasPrefix(lowerName, "pre") {
		builder.AddTag("pre-hook")
	} else if strings.HasPrefix(lowerName, "post") {
		builder.AddTag("post-hook")
	}

	// Development vs production
	if strings.Contains(lowerName, "dev") || strings.Contains(lowerName, "development") {
		builder.AddTag("development")
	} else if strings.Contains(lowerName, "prod") || strings.Contains(lowerName, "production") {
		builder.AddTag("production")
	}
}

// createPrePostScriptTask creates pre/post script tasks if they exist
func (p *PackageJSONParser) createPrePostScriptTask(scriptName, prefix string, packageData *PackageJSON, filePath, workingDir string, runnerType task.RunnerType) *task.Task {
	prePostScriptName := prefix + scriptName

	if scriptValue, exists := packageData.Scripts[prePostScriptName]; exists {
		scriptInfo, err := p.parseScriptValue(prePostScriptName, scriptValue)
		if err != nil {
			p.logger.Debug("Failed to parse pre/post script", "name", prePostScriptName, "error", err)
			return nil
		}

		task := p.createTaskFromScript(scriptInfo, packageData, filePath, workingDir, runnerType)

		// Add special tags for pre/post scripts
		task.AddTag(prefix + "-hook")
		task.AddTag("hook")

		// Add metadata indicating this is a hook
		task.Metadata["hook_type"] = prefix
		task.Metadata["target_script"] = scriptName

		return task
	}

	return nil
}

// GetPackageInfo returns information about the package.json
func (p *PackageJSONParser) GetPackageInfo(filePath string) (*PackageInfo, error) {
	packageData, err := p.readPackageJSON(filePath)
	if err != nil {
		return nil, err
	}

	runnerType := p.determineRunnerType(filepath.Dir(filePath))

	scriptsCount := 0
	if packageData.Scripts != nil {
		scriptsCount = len(packageData.Scripts)
	}

	depsCount := len(packageData.Dependencies) + len(packageData.DevDependencies)

	return &PackageInfo{
		FilePath:          filePath,
		Name:              packageData.Name,
		Version:           packageData.Version,
		Description:       packageData.Description,
		RunnerType:        runnerType,
		ScriptsCount:      scriptsCount,
		DependenciesCount: depsCount,
		Private:           packageData.Private,
		HasWorkspaces:     packageData.Workspaces != nil,
	}, nil
}

// PackageInfo contains information about a parsed package.json
type PackageInfo struct {
	FilePath          string          `json:"file_path"`
	Name              string          `json:"name"`
	Version           string          `json:"version"`
	Description       string          `json:"description"`
	RunnerType        task.RunnerType `json:"runner_type"`
	ScriptsCount      int             `json:"scripts_count"`
	DependenciesCount int             `json:"dependencies_count"`
	Private           bool            `json:"private"`
	HasWorkspaces     bool            `json:"has_workspaces"`
}

// Validate validates a package.json file
func (p *PackageJSONParser) Validate(filePath string) error {
	_, err := p.readPackageJSON(filePath)
	if err != nil {
		return fmt.Errorf("invalid package.json: %w", err)
	}

	return nil
}

// GetScriptDependencies analyzes script dependencies (pre/post hooks)
func (p *PackageJSONParser) GetScriptDependencies(filePath string) (map[string][]string, error) {
	packageData, err := p.readPackageJSON(filePath)
	if err != nil {
		return nil, err
	}

	dependencies := make(map[string][]string)

	if packageData.Scripts == nil {
		return dependencies, nil
	}

	// Analyze pre/post script relationships
	for scriptName := range packageData.Scripts {
		var deps []string

		// Check for pre-script
		preScriptName := "pre" + scriptName
		if _, exists := packageData.Scripts[preScriptName]; exists {
			deps = append(deps, preScriptName)
		}

		// Check for post-script
		postScriptName := "post" + scriptName
		if _, exists := packageData.Scripts[postScriptName]; exists {
			// Post scripts depend on the main script
			if _, exists := dependencies[postScriptName]; !exists {
				dependencies[postScriptName] = []string{}
			}
			dependencies[postScriptName] = append(dependencies[postScriptName], scriptName)
		}

		if len(deps) > 0 {
			dependencies[scriptName] = deps
		}
	}

	return dependencies, nil
}
