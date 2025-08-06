package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/vivalchemy/wake/internal/app"
	"github.com/vivalchemy/wake/pkg/task"
)

// ListCommand handles the list subcommand
type ListCommand struct {
	app        *app.App
	globalOpts *GlobalOptions

	// List-specific options
	showAll      bool
	showMeta     bool
	sortBy       string
	groupBy      string
	filterStatus string
	limit        int
	offset       int
	search       string
	longFormat   bool
}

// NewListCommand creates a new list command
func NewListCommand(application *app.App, globalOpts *GlobalOptions) (*cobra.Command, error) {
	if application == nil {
		return nil, fmt.Errorf("application cannot be nil")
	}

	listHandler := &ListCommand{
		app:        application,
		globalOpts: globalOpts,
		limit:      -1, // No limit by default
	}

	cmd := &cobra.Command{
		Use:   "list [flags]",
		Short: "List all available tasks",
		Long: `List all available tasks discovered in the current project.

The list command discovers tasks from various task runners and displays them
in a formatted table. You can filter, sort, and format the output in various ways.`,

		Aliases: []string{"ls", "l"},
		RunE:    listHandler.Execute,

		Example: `  wake list                          # List all tasks
  wake list --runner=npm             # List only npm scripts
  wake list --tags=build,test        # List tasks with build or test tags
  wake list --output=json            # Output as JSON
  wake list --sort=runner            # Sort by runner type
  wake list --group=runner           # Group by runner type
  wake list --search=build           # Search for tasks containing 'build'
  wake list --long                   # Show detailed information
  wake list --show-meta              # Show task metadata`,
	}

	// Add list-specific flags
	listHandler.addFlags(cmd)

	return cmd, nil
}

// addFlags adds command-specific flags
func (l *ListCommand) addFlags(cmd *cobra.Command) {
	flags := cmd.Flags()

	// Display options
	flags.BoolVarP(&l.showAll, "all", "a", false,
		"show all tasks including hidden ones")
	flags.BoolVar(&l.showMeta, "show-meta", false,
		"show task metadata")
	flags.BoolVarP(&l.longFormat, "long", "l", false,
		"show detailed task information")

	// Sorting and grouping
	flags.StringVar(&l.sortBy, "sort", "name",
		"sort tasks by field (name, runner, status, file, modified)")
	flags.StringVar(&l.groupBy, "group", "",
		"group tasks by field (runner, status, file, tags)")

	// Filtering
	flags.StringVar(&l.filterStatus, "status", "",
		"filter by task status (idle, running, success, failed, stopped)")
	flags.StringVarP(&l.search, "search", "s", "",
		"search tasks by name or description")

	// Pagination
	flags.IntVar(&l.limit, "limit", -1,
		"limit number of results (default: no limit)")
	flags.IntVar(&l.offset, "offset", 0,
		"offset for pagination")
}

// Execute runs the list command
func (l *ListCommand) Execute(cmd *cobra.Command, args []string) error {
	ctx := cmd.Context()

	// Wait for task discovery to complete
	select {
	case <-l.app.WaitForDiscovery():
		// Discovery completed
	case <-ctx.Done():
		return ctx.Err()
	}

	// Get all tasks
	allTasks := l.app.GetTasks()

	// Apply filters
	filteredTasks := l.filterTasks(allTasks)

	// Apply search
	if l.search != "" {
		filteredTasks = l.searchTasks(filteredTasks, l.search)
	}

	// Sort tasks
	l.sortTasks(filteredTasks)

	// Apply pagination
	paginatedTasks := l.paginateTasks(filteredTasks)

	// Generate output based on format
	return l.generateOutput(paginatedTasks, len(filteredTasks))
}

// filterTasks applies various filters to the task list
func (l *ListCommand) filterTasks(tasks []*task.Task) []*task.Task {
	var filtered []*task.Task

	for _, t := range tasks {
		// Apply global runner filter
		if l.globalOpts.Runner != "" && string(t.Runner) != l.globalOpts.Runner {
			continue
		}

		// Apply hidden filter
		if !l.showAll && !l.globalOpts.ShowHidden && t.Hidden {
			continue
		}

		// Apply global tags filter
		if len(l.globalOpts.Tags) > 0 {
			hasAllTags := true
			for _, requiredTag := range l.globalOpts.Tags {
				if !t.HasTag(requiredTag) {
					hasAllTags = false
					break
				}
			}
			if !hasAllTags {
				continue
			}
		}

		// Apply status filter
		if l.filterStatus != "" && string(t.Status) != l.filterStatus {
			continue
		}

		filtered = append(filtered, t)
	}

	return filtered
}

// searchTasks filters tasks based on search term
func (l *ListCommand) searchTasks(tasks []*task.Task, searchTerm string) []*task.Task {
	searchTerm = strings.ToLower(searchTerm)
	var matches []*task.Task

	for _, t := range tasks {
		// Search in name
		if strings.Contains(strings.ToLower(t.Name), searchTerm) {
			matches = append(matches, t)
			continue
		}

		// Search in description
		if strings.Contains(strings.ToLower(t.Description), searchTerm) {
			matches = append(matches, t)
			continue
		}

		// Search in tags
		for _, tag := range t.Tags {
			if strings.Contains(strings.ToLower(tag), searchTerm) {
				matches = append(matches, t)
				goto nextTask
			}
		}

		// Search in file path
		if strings.Contains(strings.ToLower(t.FilePath), searchTerm) {
			matches = append(matches, t)
			continue
		}

	nextTask:
	}

	return matches
}

// sortTasks sorts tasks based on the specified field
func (l *ListCommand) sortTasks(tasks []*task.Task) {
	sort.Slice(tasks, func(i, j int) bool {
		switch l.sortBy {
		case "name":
			return tasks[i].Name < tasks[j].Name
		case "runner":
			if tasks[i].Runner == tasks[j].Runner {
				return tasks[i].Name < tasks[j].Name
			}
			return tasks[i].Runner < tasks[j].Runner
		case "status":
			if tasks[i].Status == tasks[j].Status {
				return tasks[i].Name < tasks[j].Name
			}
			return tasks[i].Status < tasks[j].Status
		case "file":
			if tasks[i].FilePath == tasks[j].FilePath {
				return tasks[i].Name < tasks[j].Name
			}
			return tasks[i].FilePath < tasks[j].FilePath
		case "modified":
			return tasks[i].DiscoveredAt.After(tasks[j].DiscoveredAt)
		default:
			return tasks[i].Name < tasks[j].Name
		}
	})
}

// paginateTasks applies pagination to the task list
func (l *ListCommand) paginateTasks(tasks []*task.Task) []*task.Task {
	total := len(tasks)

	// Apply offset
	start := l.offset
	if start >= total {
		return []*task.Task{}
	}

	// Apply limit
	end := total
	if l.limit > 0 {
		end = start + l.limit
		if end > total {
			end = total
		}
	}

	return tasks[start:end]
}

// generateOutput generates output in the specified format
func (l *ListCommand) generateOutput(tasks []*task.Task, totalCount int) error {
	switch l.globalOpts.OutputFormat {
	case "json":
		return l.outputJSON(tasks, totalCount)
	case "yaml":
		return l.outputYAML(tasks, totalCount)
	case "text":
		return l.outputText(tasks, totalCount)
	case "table", "":
		if l.groupBy != "" {
			return l.outputGroupedTable(tasks, totalCount)
		}
		return l.outputTable(tasks, totalCount)
	default:
		return fmt.Errorf("unsupported output format: %s", l.globalOpts.OutputFormat)
	}
}

// outputJSON outputs tasks as JSON
func (l *ListCommand) outputJSON(tasks []*task.Task, totalCount int) error {
	output := struct {
		Tasks      []*task.Task `json:"tasks"`
		TotalCount int          `json:"total_count"`
		Count      int          `json:"count"`
		Offset     int          `json:"offset"`
		Limit      int          `json:"limit"`
	}{
		Tasks:      tasks,
		TotalCount: totalCount,
		Count:      len(tasks),
		Offset:     l.offset,
		Limit:      l.limit,
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(output)
}

// outputYAML outputs tasks as YAML
func (l *ListCommand) outputYAML(tasks []*task.Task, totalCount int) error {
	output := struct {
		Tasks      []*task.Task `yaml:"tasks"`
		TotalCount int          `yaml:"total_count"`
		Count      int          `yaml:"count"`
		Offset     int          `yaml:"offset"`
		Limit      int          `yaml:"limit"`
	}{
		Tasks:      tasks,
		TotalCount: totalCount,
		Count:      len(tasks),
		Offset:     l.offset,
		Limit:      l.limit,
	}

	encoder := yaml.NewEncoder(os.Stdout)
	encoder.SetIndent(2)
	return encoder.Encode(output)
}

// outputText outputs tasks as plain text
func (l *ListCommand) outputText(tasks []*task.Task, totalCount int) error {
	for _, t := range tasks {
		fmt.Printf("%s", t.Name)

		if l.longFormat {
			fmt.Printf(" (%s)", t.Runner)
			if t.Description != "" {
				fmt.Printf(" - %s", t.Description)
			}
		}

		fmt.Println()
	}

	if !l.globalOpts.Quiet && totalCount != len(tasks) {
		fmt.Fprintf(os.Stderr, "\nShowing %d of %d tasks\n", len(tasks), totalCount)
	}

	return nil
}

// outputTable outputs tasks as a formatted table
func (l *ListCommand) outputTable(tasks []*task.Task, totalCount int) error {
	if len(tasks) == 0 {
		if !l.globalOpts.Quiet {
			fmt.Println("No tasks found.")
		}
		return nil
	}

	// Create tab writer
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer w.Flush()

	// Write header
	if l.longFormat {
		fmt.Fprintln(w, "NAME\tRUNNER\tSTATUS\tFILE\tDESCRIPTION")
		fmt.Fprintln(w, "----\t------\t------\t----\t-----------")
	} else {
		fmt.Fprintln(w, "NAME\tRUNNER\tDESCRIPTION")
		fmt.Fprintln(w, "----\t------\t-----------")
	}

	// Write tasks
	for _, t := range tasks {
		name := t.Name
		if t.Hidden {
			name = fmt.Sprintf("%s (hidden)", name)
		}

		runner := string(t.Runner)
		status := l.formatStatus(t.Status)
		description := t.Description

		// Truncate long descriptions
		if len(description) > 50 && !l.longFormat {
			description = description[:47] + "..."
		}

		if l.longFormat {
			fileName := l.getShortFilePath(t.FilePath)
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
				name, runner, status, fileName, description)
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\n", name, runner, description)
		}

		// Show metadata if requested
		if l.showMeta && len(t.Metadata) > 0 {
			fmt.Fprintf(w, "\t\t%s\n", l.formatMetadata(t.Metadata))
		}

		// Show tags if in long format
		if l.longFormat && len(t.Tags) > 0 {
			fmt.Fprintf(w, "\t\t[%s]\n", strings.Join(t.Tags, ", "))
		}
	}

	// Show summary
	if !l.globalOpts.Quiet {
		fmt.Fprintf(w, "\n")
		if totalCount != len(tasks) {
			fmt.Fprintf(w, "Showing %d of %d tasks\n", len(tasks), totalCount)
		} else {
			fmt.Fprintf(w, "Total: %d tasks\n", totalCount)
		}
	}

	return nil
}

// outputGroupedTable outputs tasks grouped by the specified field
func (l *ListCommand) outputGroupedTable(tasks []*task.Task, totalCount int) error {
	if len(tasks) == 0 {
		if !l.globalOpts.Quiet {
			fmt.Println("No tasks found.")
		}
		return nil
	}

	// Group tasks
	groups := l.groupTasks(tasks)

	// Create tab writer
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	defer w.Flush()

	// Write grouped output
	groupNames := make([]string, 0, len(groups))
	for groupName := range groups {
		groupNames = append(groupNames, groupName)
	}
	sort.Strings(groupNames)

	for i, groupName := range groupNames {
		if i > 0 {
			fmt.Fprintln(w, "")
		}

		groupTasks := groups[groupName]
		fmt.Fprintf(w, "%s (%d tasks):\n", strings.ToUpper(groupName), len(groupTasks))
		fmt.Fprintln(w, strings.Repeat("-", len(groupName)+20))

		// Write header
		fmt.Fprintln(w, "NAME\tSTATUS\tDESCRIPTION")
		fmt.Fprintln(w, "----\t------\t-----------")

		// Write tasks in group
		for _, t := range groupTasks {
			name := t.Name
			if t.Hidden {
				name = fmt.Sprintf("%s (hidden)", name)
			}

			status := l.formatStatus(t.Status)
			description := t.Description

			// Truncate long descriptions
			if len(description) > 60 {
				description = description[:57] + "..."
			}

			fmt.Fprintf(w, "%s\t%s\t%s\n", name, status, description)
		}
	}

	// Show summary
	if !l.globalOpts.Quiet {
		fmt.Fprintf(w, "\n")
		if totalCount != len(tasks) {
			fmt.Fprintf(w, "Showing %d of %d tasks in %d groups\n",
				len(tasks), totalCount, len(groups))
		} else {
			fmt.Fprintf(w, "Total: %d tasks in %d groups\n", totalCount, len(groups))
		}
	}

	return nil
}

// groupTasks groups tasks by the specified field
func (l *ListCommand) groupTasks(tasks []*task.Task) map[string][]*task.Task {
	groups := make(map[string][]*task.Task)

	for _, t := range tasks {
		var groupKey string

		switch l.groupBy {
		case "runner":
			groupKey = string(t.Runner)
		case "status":
			groupKey = string(t.Status)
		case "file":
			groupKey = l.getShortFilePath(t.FilePath)
		case "tags":
			if len(t.Tags) > 0 {
				groupKey = t.Tags[0] // Group by first tag
			} else {
				groupKey = "untagged"
			}
		default:
			groupKey = "all"
		}

		if groupKey == "" {
			groupKey = "unknown"
		}

		groups[groupKey] = append(groups[groupKey], t)
	}

	// Sort tasks within each group
	for _, groupTasks := range groups {
		sort.Slice(groupTasks, func(i, j int) bool {
			return groupTasks[i].Name < groupTasks[j].Name
		})
	}

	return groups
}

// formatStatus formats task status with color if enabled
func (l *ListCommand) formatStatus(status task.Status) string {
	if l.globalOpts.NoColor {
		return string(status)
	}

	// Add color codes based on status
	switch status {
	case task.StatusRunning:
		return "\033[33m" + string(status) + "\033[0m" // Yellow
	case task.StatusSuccess:
		return "\033[32m" + string(status) + "\033[0m" // Green
	case task.StatusFailed:
		return "\033[31m" + string(status) + "\033[0m" // Red
	case task.StatusStopped:
		return "\033[35m" + string(status) + "\033[0m" // Magenta
	default:
		return string(status)
	}
}

// formatMetadata formats task metadata for display
func (l *ListCommand) formatMetadata(metadata map[string]any) string {
	var parts []string

	for key, value := range metadata {
		// Skip internal metadata
		if strings.HasPrefix(key, "_") {
			continue
		}

		// Format value
		var valueStr string
		switch v := value.(type) {
		case string:
			valueStr = v
		case bool:
			valueStr = strconv.FormatBool(v)
		case int, int64:
			valueStr = fmt.Sprintf("%d", v)
		case float64:
			valueStr = fmt.Sprintf("%.2f", v)
		case time.Time:
			valueStr = v.Format("2006-01-02 15:04:05")
		default:
			valueStr = fmt.Sprintf("%v", v)
		}

		parts = append(parts, fmt.Sprintf("%s=%s", key, valueStr))
	}

	if len(parts) == 0 {
		return ""
	}

	return fmt.Sprintf("metadata: %s", strings.Join(parts, ", "))
}

// getShortFilePath returns a shortened version of the file path
func (l *ListCommand) getShortFilePath(filePath string) string {
	if filePath == "" {
		return ""
	}

	// Get relative path from current directory
	if cwd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(cwd, filePath); err == nil && len(rel) < len(filePath) {
			return rel
		}
	}

	// Fallback to basename
	return filepath.Base(filePath)
}
