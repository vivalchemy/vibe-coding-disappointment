package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/vivalchemy/wake/internal/app"
	"github.com/vivalchemy/wake/pkg/task"
	"maps"
	"slices"
)

// DryCommand handles the dry-run subcommand for previewing task execution
type DryCommand struct {
	app        *app.App
	globalOpts *GlobalOptions

	// Dry-run specific options
	showEnv      bool
	showDeps     bool
	showHooks    bool
	showCommands bool
	detailed     bool
	validate     bool
	analyze      bool
	graph        bool
}

// ExecutionPlan represents the planned execution of tasks
type ExecutionPlan struct {
	Tasks       []*PlannedTask    `json:"tasks"`
	Environment map[string]string `json:"environment"`
	Summary     *ExecutionSummary `json:"summary"`
	Warnings    []string          `json:"warnings,omitempty"`
	Errors      []string          `json:"errors,omitempty"`
	Generated   time.Time         `json:"generated"`
}

// PlannedTask represents a task in the execution plan
type PlannedTask struct {
	*task.Task

	// Execution details
	ResolvedCommand  string            `json:"resolved_command"`
	ResolvedArgs     []string          `json:"resolved_args"`
	ResolvedEnv      map[string]string `json:"resolved_environment"`
	WorkingDirectory string            `json:"working_directory"`

	// Dependencies and hooks
	Dependencies []string `json:"dependencies,omitempty"`
	PreHooks     []string `json:"pre_hooks,omitempty"`
	PostHooks    []string `json:"post_hooks,omitempty"`

	// Analysis
	EstimatedDuration time.Duration `json:"estimated_duration,omitempty"`
	RiskLevel         string        `json:"risk_level,omitempty"`
	Warnings          []string      `json:"warnings,omitempty"`

	// Validation
	IsValid          bool     `json:"is_valid"`
	ValidationErrors []string `json:"validation_errors,omitempty"`
}

// ExecutionSummary provides a summary of the execution plan
type ExecutionSummary struct {
	TotalTasks        int           `json:"total_tasks"`
	ValidTasks        int           `json:"valid_tasks"`
	InvalidTasks      int           `json:"invalid_tasks"`
	EstimatedDuration time.Duration `json:"estimated_duration"`
	RiskAssessment    string        `json:"risk_assessment"`
	Runners           []string      `json:"runners"`
	Files             []string      `json:"files"`
}

// NewDryCommand creates a new dry-run command
func NewDryCommand(application *app.App, globalOpts *GlobalOptions) (*cobra.Command, error) {
	if application == nil {
		return nil, fmt.Errorf("application cannot be nil")
	}

	dryHandler := &DryCommand{
		app:        application,
		globalOpts: globalOpts,
	}

	cmd := &cobra.Command{
		Use:   "dry [task] [task2] [task3] ...",
		Short: "Show what would be executed without running tasks",
		Long: `Preview task execution without actually running the tasks.

The dry-run command analyzes the execution plan, resolves environment variables,
checks dependencies, and shows exactly what would happen if the tasks were executed.
This is useful for debugging, validation, and understanding task behavior.`,

		Args:    cobra.ArbitraryArgs,
		RunE:    dryHandler.Execute,
		Aliases: []string{"dry-run", "preview", "plan"},

		ValidArgsFunction: dryHandler.CompleteTaskNames,

		Example: `  wake dry build                     # Preview build task execution
  wake dry --show-env test           # Show environment variables
  wake dry --show-deps deploy        # Show task dependencies
  wake dry --detailed build test     # Show detailed execution plan
  wake dry --validate all            # Validate all tasks
  wake dry --analyze --output=json   # Analyze and output as JSON`,
	}

	// Add dry-run specific flags
	dryHandler.addFlags(cmd)

	return cmd, nil
}

// addFlags adds command-specific flags
func (d *DryCommand) addFlags(cmd *cobra.Command) {
	flags := cmd.Flags()

	// Display options
	flags.BoolVar(&d.showEnv, "show-env", false,
		"show environment variables that would be set")
	flags.BoolVar(&d.showDeps, "show-deps", false,
		"show task dependencies")
	flags.BoolVar(&d.showHooks, "show-hooks", false,
		"show pre/post execution hooks")
	flags.BoolVar(&d.showCommands, "show-commands", true,
		"show resolved commands (default: true)")
	flags.BoolVarP(&d.detailed, "detailed", "d", false,
		"show detailed execution plan")

	// Analysis options
	flags.BoolVar(&d.validate, "validate", false,
		"validate tasks and show validation errors")
	flags.BoolVar(&d.analyze, "analyze", false,
		"perform risk analysis and estimation")
	flags.BoolVar(&d.graph, "graph", false,
		"show dependency graph (text-based)")
}

// Execute runs the dry-run command
func (d *DryCommand) Execute(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Wait for task discovery to complete
	select {
	case <-d.app.WaitForDiscovery():
		// Discovery completed
	case <-ctx.Done():
		return ctx.Err()
	}

	// Determine which tasks to analyze
	var targetTasks []*task.Task
	var err error

	if len(args) == 0 {
		// No specific tasks - show all or prompt
		if d.globalOpts.Quiet {
			return fmt.Errorf("no tasks specified and running in quiet mode")
		}

		fmt.Println("No tasks specified. Analyzing all available tasks...")
		targetTasks = d.app.GetTasks()
	} else {
		// Resolve specified tasks
		targetTasks, err = d.resolveTasks(args)
		if err != nil {
			return err
		}
	}

	// Apply filters
	targetTasks = d.filterTasks(targetTasks)

	if len(targetTasks) == 0 {
		return fmt.Errorf("no tasks found matching criteria")
	}

	// Create execution plan
	plan, err := d.createExecutionPlan(targetTasks)
	if err != nil {
		return fmt.Errorf("failed to create execution plan: %w", err)
	}

	// Generate output
	return d.generateOutput(plan)
}

// resolveTasks resolves task names to task objects
func (d *DryCommand) resolveTasks(taskNames []string) ([]*task.Task, error) {
	allTasks := d.app.GetTasks()
	var resolvedTasks []*task.Task

	for _, taskName := range taskNames {
		if taskName == "all" {
			// Special case: "all" means all available tasks
			return allTasks, nil
		}

		task, err := d.findTask(taskName, allTasks)
		if err != nil {
			return nil, err
		}
		resolvedTasks = append(resolvedTasks, task)
	}

	return resolvedTasks, nil
}

// findTask finds a task by name, ID, or alias
func (d *DryCommand) findTask(taskName string, allTasks []*task.Task) (*task.Task, error) {
	var matches []*task.Task

	// Look for exact matches first
	for _, t := range allTasks {
		if t.Name == taskName || t.ID == taskName {
			matches = append(matches, t)
		}
	}

	// If no exact matches, look for alias matches
	if len(matches) == 0 {
		for _, t := range allTasks {
			if slices.Contains(t.Aliases, taskName) {
				matches = append(matches, t)
			}
		}
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("task not found: %s", taskName)
	}

	if len(matches) > 1 {
		return nil, fmt.Errorf("multiple tasks match '%s': %s",
			taskName, d.formatTaskMatches(matches))
	}

	return matches[0], nil
}

// filterTasks applies global filters
func (d *DryCommand) filterTasks(tasks []*task.Task) []*task.Task {
	var filtered []*task.Task

	for _, t := range tasks {
		// Apply runner filter
		if d.globalOpts.Runner != "" && string(t.Runner) != d.globalOpts.Runner {
			continue
		}

		// Apply hidden filter
		if !d.globalOpts.ShowHidden && t.Hidden {
			continue
		}

		// Apply tags filter
		if len(d.globalOpts.Tags) > 0 {
			hasAllTags := true
			for _, requiredTag := range d.globalOpts.Tags {
				if !t.HasTag(requiredTag) {
					hasAllTags = false
					break
				}
			}
			if !hasAllTags {
				continue
			}
		}

		filtered = append(filtered, t)
	}

	return filtered
}

// createExecutionPlan creates a detailed execution plan
func (d *DryCommand) createExecutionPlan(tasks []*task.Task) (*ExecutionPlan, error) {
	plan := &ExecutionPlan{
		Tasks:       make([]*PlannedTask, 0, len(tasks)),
		Environment: make(map[string]string),
		Generated:   time.Now(),
		Warnings:    make([]string, 0),
		Errors:      make([]string, 0),
	}

	// Prepare global environment
	globalEnv, err := d.prepareGlobalEnvironment()
	if err != nil {
		plan.Errors = append(plan.Errors, fmt.Sprintf("Failed to prepare environment: %v", err))
	} else {
		plan.Environment = globalEnv
	}

	// Process each task
	runners := make(map[string]bool)
	files := make(map[string]bool)
	validTasks := 0
	totalEstimatedDuration := time.Duration(0)

	for _, t := range tasks {
		plannedTask, err := d.planTask(t, globalEnv)
		if err != nil {
			plan.Errors = append(plan.Errors, fmt.Sprintf("Task %s: %v", t.Name, err))
			continue
		}

		plan.Tasks = append(plan.Tasks, plannedTask)

		// Collect summary data
		runners[string(t.Runner)] = true
		if t.FilePath != "" {
			files[t.FilePath] = true
		}

		if plannedTask.IsValid {
			validTasks++
		}

		totalEstimatedDuration += plannedTask.EstimatedDuration

		// Collect warnings
		plan.Warnings = append(plan.Warnings, plannedTask.Warnings...)
	}

	// Create summary
	runnerList := make([]string, 0, len(runners))
	for runner := range runners {
		runnerList = append(runnerList, runner)
	}
	sort.Strings(runnerList)

	fileList := make([]string, 0, len(files))
	for file := range files {
		fileList = append(fileList, file)
	}
	sort.Strings(fileList)

	plan.Summary = &ExecutionSummary{
		TotalTasks:        len(plan.Tasks),
		ValidTasks:        validTasks,
		InvalidTasks:      len(plan.Tasks) - validTasks,
		EstimatedDuration: totalEstimatedDuration,
		RiskAssessment:    d.assessRisk(plan.Tasks),
		Runners:           runnerList,
		Files:             fileList,
	}

	return plan, nil
}

// planTask creates a planned task with detailed analysis
func (d *DryCommand) planTask(t *task.Task, globalEnv map[string]string) (*PlannedTask, error) {
	planned := &PlannedTask{
		Task:             t,
		ResolvedEnv:      make(map[string]string),
		Dependencies:     make([]string, len(t.Dependencies)),
		PreHooks:         make([]string, 0),
		PostHooks:        make([]string, 0),
		Warnings:         make([]string, 0),
		ValidationErrors: make([]string, 0),
		IsValid:          true,
	}

	// Copy dependencies
	copy(planned.Dependencies, t.Dependencies)

	// Resolve command and arguments
	planned.ResolvedCommand = t.Command
	planned.ResolvedArgs = make([]string, len(t.Args))
	copy(planned.ResolvedArgs, t.Args)

	// Resolve working directory
	planned.WorkingDirectory = t.WorkingDirectory
	if planned.WorkingDirectory == "" {
		if t.FilePath != "" {
			planned.WorkingDirectory = filepath.Dir(t.FilePath)
		} else {
			if cwd, err := os.Getwd(); err == nil {
				planned.WorkingDirectory = cwd
			}
		}
	}

	// Resolve environment variables
	maps.Copy(planned.ResolvedEnv, globalEnv)
	for key, value := range t.Environment {
		planned.ResolvedEnv[key] = d.resolveEnvironmentValue(value, planned.ResolvedEnv)
	}

	// Add Wake-specific environment variables
	planned.ResolvedEnv["WAKE_TASK_NAME"] = t.Name
	planned.ResolvedEnv["WAKE_TASK_ID"] = t.ID
	planned.ResolvedEnv["WAKE_TASK_RUNNER"] = string(t.Runner)
	planned.ResolvedEnv["WAKE_TASK_FILE"] = t.FilePath

	// Perform validation if requested
	if d.validate {
		d.validateTask(planned)
	}

	// Perform analysis if requested
	if d.analyze {
		d.analyzeTask(planned)
	}

	return planned, nil
}

// validateTask validates a planned task
func (d *DryCommand) validateTask(planned *PlannedTask) {
	// Check if command exists
	if planned.ResolvedCommand == "" {
		planned.ValidationErrors = append(planned.ValidationErrors, "command is empty")
		planned.IsValid = false
	}

	// Check if working directory exists
	if planned.WorkingDirectory != "" {
		if _, err := os.Stat(planned.WorkingDirectory); os.IsNotExist(err) {
			planned.ValidationErrors = append(planned.ValidationErrors,
				fmt.Sprintf("working directory does not exist: %s", planned.WorkingDirectory))
			planned.IsValid = false
		}
	}

	// Check if source file exists
	if planned.FilePath != "" {
		if _, err := os.Stat(planned.FilePath); os.IsNotExist(err) {
			planned.ValidationErrors = append(planned.ValidationErrors,
				fmt.Sprintf("source file does not exist: %s", planned.FilePath))
			planned.IsValid = false
		}
	}

	// Validate runner-specific requirements
	d.validateRunnerRequirements(planned)
}

// validateRunnerRequirements validates runner-specific requirements
func (d *DryCommand) validateRunnerRequirements(planned *PlannedTask) {
	switch planned.Runner {
	case task.RunnerMake:
		// Check if make is available
		// This is a simplified check - in reality, you'd use exec.LookPath
		if planned.ResolvedCommand == "make" {
			// Simplified validation
		}

	case task.RunnerNpm, task.RunnerYarn, task.RunnerPnpm, task.RunnerBun:
		// Check if package.json exists
		packageJsonPath := filepath.Join(planned.WorkingDirectory, "package.json")
		if _, err := os.Stat(packageJsonPath); os.IsNotExist(err) {
			planned.Warnings = append(planned.Warnings, "package.json not found in working directory")
		}

	case task.RunnerJust:
		// Check if Justfile exists
		justfilePath := filepath.Join(planned.WorkingDirectory, "Justfile")
		if _, err := os.Stat(justfilePath); os.IsNotExist(err) {
			justfilePath = filepath.Join(planned.WorkingDirectory, "justfile")
			if _, err := os.Stat(justfilePath); os.IsNotExist(err) {
				planned.Warnings = append(planned.Warnings, "Justfile not found in working directory")
			}
		}

	case task.RunnerTask:
		// Check if Taskfile.yml exists
		taskfilePath := filepath.Join(planned.WorkingDirectory, "Taskfile.yml")
		if _, err := os.Stat(taskfilePath); os.IsNotExist(err) {
			planned.Warnings = append(planned.Warnings, "Taskfile.yml not found in working directory")
		}
	}
}

// analyzeTask performs analysis on a planned task
func (d *DryCommand) analyzeTask(planned *PlannedTask) {
	// Estimate duration based on task type and historical data
	planned.EstimatedDuration = d.estimateTaskDuration(planned)

	// Assess risk level
	planned.RiskLevel = d.assessTaskRisk(planned)

	// Add analysis warnings
	if planned.EstimatedDuration > 5*time.Minute {
		planned.Warnings = append(planned.Warnings, "task may take longer than 5 minutes")
	}

	if planned.RiskLevel == "high" {
		planned.Warnings = append(planned.Warnings, "task has high risk assessment")
	}
}

// estimateTaskDuration estimates how long a task might take
func (d *DryCommand) estimateTaskDuration(planned *PlannedTask) time.Duration {
	// This is a simplified estimation - in reality, you might use historical data
	baseDuration := 30 * time.Second // Base duration

	// Adjust based on task type
	switch planned.Runner {
	case task.RunnerMake:
		if strings.Contains(planned.Name, "build") {
			baseDuration = 2 * time.Minute
		}
	case task.RunnerNpm, task.RunnerYarn, task.RunnerPnpm, task.RunnerBun:
		if strings.Contains(planned.Name, "install") {
			baseDuration = 1 * time.Minute
		} else if strings.Contains(planned.Name, "build") {
			baseDuration = 3 * time.Minute
		} else if strings.Contains(planned.Name, "test") {
			baseDuration = 2 * time.Minute
		}
	}

	// Adjust based on task complexity (number of arguments, environment variables, etc.)
	complexity := len(planned.ResolvedArgs) + len(planned.ResolvedEnv)/10
	baseDuration += time.Duration(complexity) * 5 * time.Second

	return baseDuration
}

// assessTaskRisk assesses the risk level of a task
func (d *DryCommand) assessTaskRisk(planned *PlannedTask) string {
	riskScore := 0

	// Increase risk for deployment-related tasks
	if strings.Contains(strings.ToLower(planned.Name), "deploy") ||
		strings.Contains(strings.ToLower(planned.Name), "release") ||
		strings.Contains(strings.ToLower(planned.Name), "publish") {
		riskScore += 3
	}

	// Increase risk for cleanup tasks
	if strings.Contains(strings.ToLower(planned.Name), "clean") ||
		strings.Contains(strings.ToLower(planned.Name), "delete") ||
		strings.Contains(strings.ToLower(planned.Name), "remove") {
		riskScore += 2
	}

	// Increase risk for tasks with many environment variables
	if len(planned.ResolvedEnv) > 20 {
		riskScore += 1
	}

	// Decrease risk for common safe tasks
	if strings.Contains(strings.ToLower(planned.Name), "test") ||
		strings.Contains(strings.ToLower(planned.Name), "lint") ||
		strings.Contains(strings.ToLower(planned.Name), "format") {
		riskScore -= 1
	}

	switch {
	case riskScore >= 3:
		return "high"
	case riskScore >= 1:
		return "medium"
	default:
		return "low"
	}
}

// assessRisk assesses the overall risk of the execution plan
func (d *DryCommand) assessRisk(tasks []*PlannedTask) string {
	highRisk := 0
	mediumRisk := 0

	for _, t := range tasks {
		switch t.RiskLevel {
		case "high":
			highRisk++
		case "medium":
			mediumRisk++
		}
	}

	if highRisk > 0 {
		return "high"
	} else if mediumRisk > 0 {
		return "medium"
	} else {
		return "low"
	}
}

// prepareGlobalEnvironment prepares the global environment
func (d *DryCommand) prepareGlobalEnvironment() (map[string]string, error) {
	env := make(map[string]string)

	// Load environment files
	for _, envFile := range d.globalOpts.EnvFiles {
		if err := d.loadEnvironmentFile(envFile, env); err != nil {
			return nil, fmt.Errorf("failed to load environment file %s: %w", envFile, err)
		}
	}

	// Add Wake-specific environment variables
	env["WAKE_VERSION"] = "0.1.0" // TODO: Get from build info
	env["WAKE_DRY_RUN"] = "true"

	return env, nil
}

// loadEnvironmentFile loads environment variables from a file
func (d *DryCommand) loadEnvironmentFile(filename string, env map[string]string) error {
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}

	lines := strings.SplitSeq(string(data), "\n")
	for line := range lines {
		line = strings.TrimSpace(line)

		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes
		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') ||
				(value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		env[key] = value
	}

	return nil
}

// resolveEnvironmentValue resolves environment variable references in a value
func (d *DryCommand) resolveEnvironmentValue(value string, env map[string]string) string {
	// Simple variable substitution - in reality, you might want more sophisticated parsing
	for key, val := range env {
		placeholder := fmt.Sprintf("${%s}", key)
		value = strings.ReplaceAll(value, placeholder, val)

		placeholder = fmt.Sprintf("$%s", key)
		value = strings.ReplaceAll(value, placeholder, val)
	}

	return value
}

// generateOutput generates the output in the requested format
func (d *DryCommand) generateOutput(plan *ExecutionPlan) error {
	switch d.globalOpts.OutputFormat {
	case "json":
		return d.outputJSON(plan)
	case "yaml":
		return d.outputYAML(plan)
	case "table", "":
		return d.outputTable(plan)
	default:
		return fmt.Errorf("unsupported output format: %s", d.globalOpts.OutputFormat)
	}
}

// outputJSON outputs the plan as JSON
func (d *DryCommand) outputJSON(plan *ExecutionPlan) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(plan)
}

// outputYAML outputs the plan as YAML
func (d *DryCommand) outputYAML(plan *ExecutionPlan) error {
	encoder := yaml.NewEncoder(os.Stdout)
	encoder.SetIndent(2)
	return encoder.Encode(plan)
}

// outputTable outputs the plan as a formatted table
func (d *DryCommand) outputTable(plan *ExecutionPlan) error {
	// Show summary first
	d.showSummary(plan)

	// Show tasks
	if err := d.showTasks(plan); err != nil {
		return err
	}

	// Show warnings and errors
	d.showWarningsAndErrors(plan)

	return nil
}

// showSummary displays the execution plan summary
func (d *DryCommand) showSummary(plan *ExecutionPlan) {
	fmt.Printf("Execution Plan Summary\n")
	fmt.Printf("======================\n\n")

	fmt.Printf("Tasks: %d total, %d valid, %d invalid\n",
		plan.Summary.TotalTasks, plan.Summary.ValidTasks, plan.Summary.InvalidTasks)

	if d.analyze {
		fmt.Printf("Estimated Duration: %v\n", plan.Summary.EstimatedDuration)
		fmt.Printf("Risk Assessment: %s\n", plan.Summary.RiskAssessment)
	}

	fmt.Printf("Runners: %s\n", strings.Join(plan.Summary.Runners, ", "))

	if len(plan.Summary.Files) > 0 {
		fmt.Printf("Files: %d task files\n", len(plan.Summary.Files))
	}

	fmt.Println()
}

// showTasks displays the task details
func (d *DryCommand) showTasks(plan *ExecutionPlan) error {
	if len(plan.Tasks) == 0 {
		fmt.Println("No tasks to execute.")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer w.Flush()

	// Header
	if d.detailed {
		fmt.Fprintln(w, "TASK\tRUNNER\tSTATUS\tRISK\tDURATION\tCOMMAND")
		fmt.Fprintln(w, "----\t------\t------\t----\t--------\t-------")
	} else {
		fmt.Fprintln(w, "TASK\tRUNNER\tCOMMAND")
		fmt.Fprintln(w, "----\t------\t-------")
	}

	// Tasks
	for _, planned := range plan.Tasks {
		status := "valid"
		if !planned.IsValid {
			status = "invalid"
		}

		command := d.formatCommand(planned)

		if d.detailed {
			duration := "-"
			if d.analyze {
				duration = planned.EstimatedDuration.String()
			}

			risk := "-"
			if d.analyze {
				risk = planned.RiskLevel
			}

			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				planned.Name, planned.Runner, status, risk, duration, command)
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\n", planned.Name, planned.Runner, command)
		}

		// Show additional details if requested
		if d.detailed {
			d.showTaskDetails(w, planned)
		}
	}

	return nil
}

// formatCommand formats the command for display
func (d *DryCommand) formatCommand(planned *PlannedTask) string {
	if !d.showCommands {
		return "-"
	}

	parts := []string{planned.ResolvedCommand}
	parts = append(parts, planned.ResolvedArgs...)

	command := strings.Join(parts, " ")

	// Truncate long commands
	if len(command) > 50 {
		command = command[:47] + "..."
	}

	return command
}

// showTaskDetails shows additional task details
func (d *DryCommand) showTaskDetails(w *tabwriter.Writer, planned *PlannedTask) {
	// Working directory
	if planned.WorkingDirectory != "" {
		fmt.Fprintf(w, "\t\tDir: %s\n", planned.WorkingDirectory)
	}

	// Dependencies
	if d.showDeps && len(planned.Dependencies) > 0 {
		fmt.Fprintf(w, "\t\tDeps: %s\n", strings.Join(planned.Dependencies, ", "))
	}

	// Environment variables
	if d.showEnv && len(planned.ResolvedEnv) > 0 {
		envCount := len(planned.ResolvedEnv)
		fmt.Fprintf(w, "\t\tEnv: %d variables\n", envCount)
	}

	// Validation errors
	if len(planned.ValidationErrors) > 0 {
		for _, err := range planned.ValidationErrors {
			fmt.Fprintf(w, "\t\tError: %s\n", err)
		}
	}

	// Warnings
	if len(planned.Warnings) > 0 {
		for _, warning := range planned.Warnings {
			fmt.Fprintf(w, "\t\tWarning: %s\n", warning)
		}
	}
}

// showWarningsAndErrors displays warnings and errors
func (d *DryCommand) showWarningsAndErrors(plan *ExecutionPlan) {
	if len(plan.Warnings) > 0 {
		fmt.Printf("\nWarnings:\n")
		for _, warning := range plan.Warnings {
			fmt.Printf("  ⚠ %s\n", warning)
		}
	}

	if len(plan.Errors) > 0 {
		fmt.Printf("\nErrors:\n")
		for _, err := range plan.Errors {
			fmt.Printf("  ✗ %s\n", err)
		}
	}

	if len(plan.Warnings) > 0 || len(plan.Errors) > 0 {
		fmt.Println()
	}
}

// formatTaskMatches formats multiple task matches for error messages
func (d *DryCommand) formatTaskMatches(matches []*task.Task) string {
	var names []string
	for _, t := range matches {
		names = append(names, fmt.Sprintf("%s (%s)", t.Name, t.Runner))
	}
	return strings.Join(names, ", ")
}

// CompleteTaskNames provides shell completion for task names
func (d *DryCommand) CompleteTaskNames(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	// Get available tasks
	tasks := d.app.GetTasks()
	filteredTasks := d.filterTasks(tasks)

	var completions []string

	// Add "all" as an option
	if strings.HasPrefix("all", toComplete) {
		completions = append(completions, "all\tAnalyze all available tasks")
	}

	for _, task := range filteredTasks {
		if strings.HasPrefix(task.Name, toComplete) {
			description := task.Description
			if description == "" {
				description = fmt.Sprintf("%s task", task.Runner)
			}
			completions = append(completions, fmt.Sprintf("%s\t%s", task.Name, description))
		}
	}

	return completions, cobra.ShellCompDirectiveNoFileComp
}
