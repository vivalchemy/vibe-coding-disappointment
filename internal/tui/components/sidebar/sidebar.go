package sidebar

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/vivalchemy/wake/internal/tui/theme"
	"github.com/vivalchemy/wake/pkg/logger"
	"github.com/vivalchemy/wake/pkg/task"
)

// Sidebar represents the task list sidebar component
type Sidebar struct {
	// Model state
	model   *Model
	list    list.Model
	width   int
	height  int
	focused bool

	// Data
	tasks         []*task.Task
	filteredTasks []*task.Task

	// Filtering and search
	filter     FilterState
	searchTerm string

	// Display options
	showHidden bool
	groupBy    GroupBy
	sortBy     SortBy

	// Styles
	styles *Styles

	// Logger
	logger logger.Logger
}

// GroupBy represents grouping options
type GroupBy int

const (
	GroupByNone GroupBy = iota
	GroupByRunner
	GroupByStatus
	GroupByFile
	GroupByTags
)

// String returns the string representation of GroupBy
func (g GroupBy) String() string {
	switch g {
	case GroupByNone:
		return "none"
	case GroupByRunner:
		return "runner"
	case GroupByStatus:
		return "status"
	case GroupByFile:
		return "file"
	case GroupByTags:
		return "tags"
	default:
		return "none"
	}
}

// SortBy represents sorting options
type SortBy int

const (
	SortByName SortBy = iota
	SortByRunner
	SortByStatus
	SortByFile
	SortByModified
)

// String returns the string representation of SortBy
func (s SortBy) String() string {
	switch s {
	case SortByName:
		return "name"
	case SortByRunner:
		return "runner"
	case SortByStatus:
		return "status"
	case SortByFile:
		return "file"
	case SortByModified:
		return "modified"
	default:
		return "name"
	}
}

// Styles defines the visual styles for the sidebar
type Styles struct {
	Container    lipgloss.Style
	Header       lipgloss.Style
	FilterInfo   lipgloss.Style
	TaskItem     lipgloss.Style
	TaskSelected lipgloss.Style
	TaskRunning  lipgloss.Style
	TaskSuccess  lipgloss.Style
	TaskFailed   lipgloss.Style
	TaskHidden   lipgloss.Style
	GroupHeader  lipgloss.Style
	StatusIcon   lipgloss.Style
	RunnerBadge  lipgloss.Style
	NoTasks      lipgloss.Style
}

// TaskItem represents a task item in the list
type TaskItem struct {
	task     *task.Task
	display  string
	runner   string
	status   string
	isHidden bool
	tags     []string
}

// FilterValue returns the filter value for the list
func (t TaskItem) FilterValue() string {
	return t.display + " " + t.runner + " " + strings.Join(t.tags, " ")
}

// Title returns the title for the list item
func (t TaskItem) Title() string {
	return t.display
}

// Description returns the description for the list item
func (t TaskItem) Description() string {
	desc := fmt.Sprintf("%s • %s", t.runner, t.status)
	if t.task.Description != "" {
		desc += " • " + t.task.Description
	}
	return desc
}

// New creates a new sidebar component
func New(logger logger.Logger, themeProvider *theme.Theme) *Sidebar {
	styles := CreateStyles(themeProvider)

	// Create list model
	l := list.New([]list.Item{}, NewTaskDelegate(styles), 0, 0)
	l.Title = "Tasks"
	l.SetShowStatusBar(true)
	l.SetFilteringEnabled(true)
	l.SetShowHelp(false)
	l.DisableQuitKeybindings()

	// Create model
	model := NewModel()

	sidebar := &Sidebar{
		model:         model,
		list:          l,
		tasks:         []*task.Task{},
		filteredTasks: []*task.Task{},
		filter:        FilterState{},
		styles:        styles,
		logger:        logger.WithGroup("sidebar"),
		groupBy:       GroupByNone,
		sortBy:        SortByName,
	}

	return sidebar
}

// Init initializes the sidebar component
func (s *Sidebar) Init() tea.Cmd {
	return nil
}

// Update handles messages for the sidebar
func (s *Sidebar) Update(msg tea.Msg) (*Sidebar, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle key presses when focused
		if s.focused {
			cmd = s.handleKeyPress(msg)
		}

	case tea.WindowSizeMsg:
		s.width = msg.Width
		s.height = msg.Height
		s.updateSize()

	case TasksUpdatedMsg:
		s.tasks = msg.Tasks
		s.updateTaskList()

	case FilterUpdatedMsg:
		s.filter = msg.Filter
		s.updateTaskList()

	case SearchUpdatedMsg:
		s.searchTerm = msg.SearchTerm
		s.filter.HasSearch = msg.SearchTerm != ""
		s.updateTaskList()

	case ToggleHiddenMsg:
		s.showHidden = !s.showHidden
		s.updateTaskList()

	case GroupByMsg:
		s.groupBy = msg.GroupBy
		s.updateTaskList()

	case SortByMsg:
		s.sortBy = msg.SortBy
		s.updateTaskList()
	}

	// Update list model
	s.list, cmd = s.list.Update(msg)

	return s, cmd
}

// View renders the sidebar
func (s *Sidebar) View() string {
	if s.width == 0 || s.height == 0 {
		return ""
	}

	content := s.renderContent()

	return s.styles.Container.
		Width(s.width).
		Height(s.height).
		Render(content)
}

// renderContent renders the main content of the sidebar
func (s *Sidebar) renderContent() string {
	var sections []string

	// Header
	sections = append(sections, s.renderHeader())

	// Filter info
	if s.hasActiveFilters() {
		sections = append(sections, s.renderFilterInfo())
	}

	// Task list
	sections = append(sections, s.renderTaskList())

	return strings.Join(sections, "\n")
}

// renderHeader renders the sidebar header
func (s *Sidebar) renderHeader() string {
	title := "Tasks"

	// Add count information
	totalTasks := len(s.tasks)
	filteredTasks := len(s.filteredTasks)

	if s.hasActiveFilters() {
		title = fmt.Sprintf("Tasks (%d/%d)", filteredTasks, totalTasks)
	} else {
		title = fmt.Sprintf("Tasks (%d)", totalTasks)
	}

	return s.styles.Header.Render(title)
}

// renderFilterInfo renders active filter information
func (s *Sidebar) renderFilterInfo() string {
	var filters []string

	if s.filter.Runner != "" {
		filters = append(filters, fmt.Sprintf("runner:%s", s.filter.Runner))
	}

	if s.filter.Status != "" {
		filters = append(filters, fmt.Sprintf("status:%s", s.filter.Status))
	}

	if len(s.filter.Tags) > 0 {
		filters = append(filters, fmt.Sprintf("tags:%s", strings.Join(s.filter.Tags, ",")))
	}

	if s.filter.HasSearch {
		filters = append(filters, fmt.Sprintf("search:%s", s.searchTerm))
	}

	if s.groupBy != GroupByNone {
		filters = append(filters, fmt.Sprintf("group:%s", s.groupBy.String()))
	}

	if s.sortBy != SortByName {
		filters = append(filters, fmt.Sprintf("sort:%s", s.sortBy.String()))
	}

	if s.showHidden {
		filters = append(filters, "hidden:true")
	}

	if len(filters) == 0 {
		return ""
	}

	filterText := strings.Join(filters, " ")
	return s.styles.FilterInfo.Render("Filters: " + filterText)
}

// renderTaskList renders the task list
func (s *Sidebar) renderTaskList() string {
	if len(s.filteredTasks) == 0 {
		return s.renderNoTasks()
	}

	// Calculate available height for the list
	headerHeight := lipgloss.Height(s.renderHeader())
	filterHeight := 0
	if s.hasActiveFilters() {
		filterHeight = lipgloss.Height(s.renderFilterInfo())
	}

	availableHeight := s.height - headerHeight - filterHeight - 2 // padding

	s.list.SetSize(s.width-2, availableHeight) // account for padding

	return s.list.View()
}

// renderNoTasks renders the no tasks message
func (s *Sidebar) renderNoTasks() string {
	message := "No tasks found"

	if s.hasActiveFilters() {
		message = "No tasks match current filters"
	} else if len(s.tasks) == 0 {
		message = "No tasks discovered yet"
	}

	return s.styles.NoTasks.Render(message)
}

// handleKeyPress handles key press events
func (s *Sidebar) handleKeyPress(msg tea.KeyMsg) tea.Cmd {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		return s.selectCurrentTask()

	case key.Matches(msg, key.NewBinding(key.WithKeys("r"))):
		return s.runCurrentTask()

	case key.Matches(msg, key.NewBinding(key.WithKeys("d"))):
		return s.dryRunCurrentTask()

	case key.Matches(msg, key.NewBinding(key.WithKeys("h"))):
		return s.toggleHidden()

	case key.Matches(msg, key.NewBinding(key.WithKeys("g"))):
		return s.cycleGroupBy()

	case key.Matches(msg, key.NewBinding(key.WithKeys("s"))):
		return s.cycleSortBy()

	case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+r"))):
		return s.refreshTasks()
	}

	return nil
}

// selectCurrentTask selects the current task
func (s *Sidebar) selectCurrentTask() tea.Cmd {
	if item, ok := s.list.SelectedItem().(TaskItem); ok {
		return func() tea.Msg {
			return TaskSelectedMsg{Task: item.task}
		}
	}
	return nil
}

// runCurrentTask runs the current task
func (s *Sidebar) runCurrentTask() tea.Cmd {
	if item, ok := s.list.SelectedItem().(TaskItem); ok {
		return func() tea.Msg {
			return RunTaskMsg{Task: item.task}
		}
	}
	return nil
}

// dryRunCurrentTask dry runs the current task
func (s *Sidebar) dryRunCurrentTask() tea.Cmd {
	if item, ok := s.list.SelectedItem().(TaskItem); ok {
		return func() tea.Msg {
			return DryRunTaskMsg{Task: item.task}
		}
	}
	return nil
}

// toggleHidden toggles showing hidden tasks
func (s *Sidebar) toggleHidden() tea.Cmd {
	return func() tea.Msg {
		return ToggleHiddenMsg{}
	}
}

// cycleGroupBy cycles through grouping options
func (s *Sidebar) cycleGroupBy() tea.Cmd {
	nextGroup := (s.groupBy + 1) % 5 // 5 grouping options
	return func() tea.Msg {
		return GroupByMsg{GroupBy: nextGroup}
	}
}

// cycleSortBy cycles through sorting options
func (s *Sidebar) cycleSortBy() tea.Cmd {
	nextSort := (s.sortBy + 1) % 5 // 5 sorting options
	return func() tea.Msg {
		return SortByMsg{SortBy: nextSort}
	}
}

// refreshTasks refreshes the task list
func (s *Sidebar) refreshTasks() tea.Cmd {
	return func() tea.Msg {
		return RefreshTasksMsg{}
	}
}

// updateTaskList updates the task list based on current filters
func (s *Sidebar) updateTaskList() {
	// Apply filters
	s.filteredTasks = s.filterTasks(s.tasks)

	// Sort tasks
	s.sortTasks(s.filteredTasks)

	// Convert to list items
	items := s.createListItems(s.filteredTasks)

	// Update list
	s.list.SetItems(items)

	s.logger.Debug("Task list updated",
		"total", len(s.tasks),
		"filtered", len(s.filteredTasks),
		"group_by", s.groupBy.String(),
		"sort_by", s.sortBy.String())
}

// filterTasks filters tasks based on current filter state
func (s *Sidebar) filterTasks(tasks []*task.Task) []*task.Task {
	var filtered []*task.Task

	for _, task := range tasks {
		// Skip hidden tasks unless explicitly shown
		if task.Hidden && !s.showHidden {
			continue
		}

		// Apply runner filter
		if s.filter.Runner != "" && string(task.Runner) != s.filter.Runner {
			continue
		}

		// Apply status filter
		if s.filter.Status != "" && string(task.Status) != s.filter.Status {
			continue
		}

		// Apply tags filter
		if len(s.filter.Tags) > 0 {
			hasAllTags := true
			for _, filterTag := range s.filter.Tags {
				if !task.HasTag(filterTag) {
					hasAllTags = false
					break
				}
			}
			if !hasAllTags {
				continue
			}
		}

		// Apply search filter
		if s.filter.HasSearch && s.searchTerm != "" {
			searchLower := strings.ToLower(s.searchTerm)
			taskText := strings.ToLower(task.Name + " " + task.Description + " " + strings.Join(task.Tags, " "))
			if !strings.Contains(taskText, searchLower) {
				continue
			}
		}

		filtered = append(filtered, task)
	}

	return filtered
}

// sortTasks sorts tasks based on current sort criteria
func (s *Sidebar) sortTasks(tasks []*task.Task) {
	sort.Slice(tasks, func(i, j int) bool {
		switch s.sortBy {
		case SortByName:
			return tasks[i].Name < tasks[j].Name
		case SortByRunner:
			if tasks[i].Runner == tasks[j].Runner {
				return tasks[i].Name < tasks[j].Name
			}
			return tasks[i].Runner < tasks[j].Runner
		case SortByStatus:
			if tasks[i].Status == tasks[j].Status {
				return tasks[i].Name < tasks[j].Name
			}
			return tasks[i].Status < tasks[j].Status
		case SortByFile:
			if tasks[i].FilePath == tasks[j].FilePath {
				return tasks[i].Name < tasks[j].Name
			}
			return tasks[i].FilePath < tasks[j].FilePath
		case SortByModified:
			return tasks[i].DiscoveredAt.After(tasks[j].DiscoveredAt)
		default:
			return tasks[i].Name < tasks[j].Name
		}
	})
}

// createListItems creates list items from tasks
func (s *Sidebar) createListItems(tasks []*task.Task) []list.Item {
	if s.groupBy == GroupByNone {
		return s.createFlatListItems(tasks)
	}

	return s.createGroupedListItems(tasks)
}

// createFlatListItems creates a flat list of task items
func (s *Sidebar) createFlatListItems(tasks []*task.Task) []list.Item {
	items := make([]list.Item, len(tasks))

	for i, task := range tasks {
		items[i] = TaskItem{
			task:     task,
			display:  s.formatTaskDisplay(task),
			runner:   string(task.Runner),
			status:   string(task.Status),
			isHidden: task.Hidden,
			tags:     task.Tags,
		}
	}

	return items
}

// createGroupedListItems creates a grouped list of task items
func (s *Sidebar) createGroupedListItems(tasks []*task.Task) []list.Item {
	groups := s.groupTasks(tasks)
	var items []list.Item

	// Sort group names
	groupNames := make([]string, 0, len(groups))
	for name := range groups {
		groupNames = append(groupNames, name)
	}
	sort.Strings(groupNames)

	for _, groupName := range groupNames {
		groupTasks := groups[groupName]

		// Add group header
		items = append(items, GroupHeaderItem{
			name:  groupName,
			count: len(groupTasks),
		})

		// Add tasks in group
		for _, task := range groupTasks {
			items = append(items, TaskItem{
				task:     task,
				display:  "  " + s.formatTaskDisplay(task), // Indent group items
				runner:   string(task.Runner),
				status:   string(task.Status),
				isHidden: task.Hidden,
				tags:     task.Tags,
			})
		}
	}

	return items
}

// groupTasks groups tasks based on the current grouping criteria
func (s *Sidebar) groupTasks(tasks []*task.Task) map[string][]*task.Task {
	groups := make(map[string][]*task.Task)

	for _, task := range tasks {
		var groupKey string

		switch s.groupBy {
		case GroupByRunner:
			groupKey = string(task.Runner)
		case GroupByStatus:
			groupKey = string(task.Status)
		case GroupByFile:
			if task.FilePath != "" {
				groupKey = filepath.Base(task.FilePath)
			} else {
				groupKey = "unknown"
			}
		case GroupByTags:
			if len(task.Tags) > 0 {
				groupKey = task.Tags[0] // Group by first tag
			} else {
				groupKey = "untagged"
			}
		default:
			groupKey = "all"
		}

		groups[groupKey] = append(groups[groupKey], task)
	}

	return groups
}

// formatTaskDisplay formats the display text for a task
func (s *Sidebar) formatTaskDisplay(task *task.Task) string {
	display := task.Name

	// Add status icon
	icon := s.getStatusIcon(task.Status)
	if icon != "" {
		display = icon + " " + display
	}

	// Add hidden indicator
	if task.Hidden {
		display = display + " (hidden)"
	}

	return display
}

// getStatusIcon returns an icon for the task status
func (s *Sidebar) getStatusIcon(status task.Status) string {
	switch status {
	case task.StatusRunning:
		return "▶"
	case task.StatusSuccess:
		return "✓"
	case task.StatusFailed:
		return "✗"
	case task.StatusStopped:
		return "⏹"
	default:
		return "⏸"
	}
}

// hasActiveFilters returns true if any filters are active
func (s *Sidebar) hasActiveFilters() bool {
	return s.filter.Runner != "" ||
		s.filter.Status != "" ||
		len(s.filter.Tags) > 0 ||
		s.filter.HasSearch ||
		s.groupBy != GroupByNone ||
		s.sortBy != SortByName ||
		s.showHidden
}

// updateSize updates the component size
func (s *Sidebar) updateSize() {
	// The list size will be updated in renderTaskList
}

// SetFocused sets the focus state
func (s *Sidebar) SetFocused(focused bool) {
	s.focused = focused
}

// IsFocused returns the focus state
func (s *Sidebar) IsFocused() bool {
	return s.focused
}

// GetSelectedTask returns the currently selected task
func (s *Sidebar) GetSelectedTask() *task.Task {
	if item, ok := s.list.SelectedItem().(TaskItem); ok {
		return item.task
	}
	return nil
}

// SetTasks updates the task list
func (s *Sidebar) SetTasks(tasks []*task.Task) {
	s.tasks = tasks
	s.updateTaskList()
}

// CreateStyles creates the styles for the sidebar
func CreateStyles(themeProvider *theme.Theme) *Styles {
	colors := themeProvider.Colors()

	return &Styles{
		Container: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colors.Border).
			Padding(1),

		Header: lipgloss.NewStyle().
			Bold(true).
			Foreground(colors.Primary).
			MarginBottom(1),

		FilterInfo: lipgloss.NewStyle().
			Foreground(colors.Muted).
			Italic(true).
			MarginBottom(1),

		TaskItem: lipgloss.NewStyle().
			Foreground(colors.Text),

		TaskSelected: lipgloss.NewStyle().
			Foreground(colors.TextInverted).
			Background(colors.Primary),

		TaskRunning: lipgloss.NewStyle().
			Foreground(colors.Warning),

		TaskSuccess: lipgloss.NewStyle().
			Foreground(colors.Success),

		TaskFailed: lipgloss.NewStyle().
			Foreground(colors.Error),

		TaskHidden: lipgloss.NewStyle().
			Foreground(colors.Muted).
			Italic(true),

		GroupHeader: lipgloss.NewStyle().
			Bold(true).
			Foreground(colors.Secondary).
			MarginTop(1),

		StatusIcon: lipgloss.NewStyle().
			Bold(true),

		RunnerBadge: lipgloss.NewStyle().
			Background(colors.Secondary).
			Foreground(colors.TextInverted).
			Padding(0, 1).
			Margin(0, 1),

		NoTasks: lipgloss.NewStyle().
			Foreground(colors.Muted).
			Italic(true).
			Align(lipgloss.Center),
	}
}
