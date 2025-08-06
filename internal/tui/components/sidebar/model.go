package sidebar

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/vivalchemy/wake/pkg/task"
)

// Model represents the sidebar's data model
type Model struct {
	// State tracking
	initialized bool
	lastUpdate  time.Time
	updateCount int64

	// Selection state
	selectedIndex int
	selectedTask  *task.Task

	// List state
	listState ListState

	// Performance tracking
	renderCount    int64
	lastRenderTime time.Time
}

// ListState tracks the internal state of the list
type ListState struct {
	cursor       int
	offset       int
	totalItems   int
	visibleItems int
	hasSelection bool
}

// Messages for sidebar communication

// TasksUpdatedMsg is sent when the task list is updated
type TasksUpdatedMsg struct {
	Tasks     []*task.Task
	Timestamp time.Time
	Source    string
}

// FilterUpdatedMsg is sent when filters are updated
type FilterUpdatedMsg struct {
	Filter    FilterState
	Timestamp time.Time
}

// SearchUpdatedMsg is sent when search term is updated
type SearchUpdatedMsg struct {
	SearchTerm string
	Timestamp  time.Time
}

// TaskSelectedMsg is sent when a task is selected
type TaskSelectedMsg struct {
	Task      *task.Task
	Index     int
	Timestamp time.Time
}

// RunTaskMsg is sent when a task should be run
type RunTaskMsg struct {
	Task      *task.Task
	Timestamp time.Time
}

// DryRunTaskMsg is sent when a task should be dry run
type DryRunTaskMsg struct {
	Task      *task.Task
	Timestamp time.Time
}

// ToggleHiddenMsg is sent to toggle hidden task visibility
type ToggleHiddenMsg struct {
	Timestamp time.Time
}

// GroupByMsg is sent to change grouping
type GroupByMsg struct {
	GroupBy   GroupBy
	Timestamp time.Time
}

// SortByMsg is sent to change sorting
type SortByMsg struct {
	SortBy    SortBy
	Timestamp time.Time
}

// RefreshTasksMsg is sent to refresh the task list
type RefreshTasksMsg struct {
	Timestamp time.Time
}

// FocusChangedMsg is sent when focus changes
type FocusChangedMsg struct {
	Component string
	Focused   bool
	Timestamp time.Time
}

// TaskStatusChangedMsg is sent when a task status changes
type TaskStatusChangedMsg struct {
	Task      *task.Task
	OldStatus task.Status
	NewStatus task.Status
	Timestamp time.Time
}

// SidebarResizedMsg is sent when the sidebar is resized
type SidebarResizedMsg struct {
	Width     int
	Height    int
	Timestamp time.Time
}

// NewModel creates a new sidebar model
func NewModel() *Model {
	return &Model{
		initialized:    false,
		lastUpdate:     time.Now(),
		updateCount:    0,
		selectedIndex:  -1,
		selectedTask:   nil,
		renderCount:    0,
		lastRenderTime: time.Now(),
		listState: ListState{
			cursor:       0,
			offset:       0,
			totalItems:   0,
			visibleItems: 0,
			hasSelection: false,
		},
	}
}

// Init initializes the model
func (m *Model) Init() tea.Cmd {
	m.initialized = true
	m.lastUpdate = time.Now()
	return nil
}

// Update updates the model with a message
func (m *Model) Update(msg tea.Msg) (*Model, tea.Cmd) {
	var cmds []tea.Cmd

	m.updateCount++
	m.lastUpdate = time.Now()

	switch msg := msg.(type) {
	case TasksUpdatedMsg:
		cmds = append(cmds, m.handleTasksUpdated(msg))

	case FilterUpdatedMsg:
		cmds = append(cmds, m.handleFilterUpdated(msg))

	case SearchUpdatedMsg:
		cmds = append(cmds, m.handleSearchUpdated(msg))

	case TaskSelectedMsg:
		cmds = append(cmds, m.handleTaskSelected(msg))

	case TaskStatusChangedMsg:
		cmds = append(cmds, m.handleTaskStatusChanged(msg))

	case SidebarResizedMsg:
		cmds = append(cmds, m.handleSidebarResized(msg))

	case FocusChangedMsg:
		cmds = append(cmds, m.handleFocusChanged(msg))
	}

	return m, tea.Batch(cmds...)
}

// handleTasksUpdated handles task list updates
func (m *Model) handleTasksUpdated(msg TasksUpdatedMsg) tea.Cmd {
	m.listState.totalItems = len(msg.Tasks)

	// Reset selection if tasks changed significantly
	if m.selectedTask != nil {
		found := false
		for _, task := range msg.Tasks {
			if task.ID == m.selectedTask.ID {
				found = true
				break
			}
		}
		if !found {
			m.selectedTask = nil
			m.selectedIndex = -1
			m.listState.hasSelection = false
		}
	}

	return nil
}

// handleFilterUpdated handles filter updates
func (m *Model) handleFilterUpdated(msg FilterUpdatedMsg) tea.Cmd {
	// Reset selection when filters change
	m.selectedIndex = -1
	m.selectedTask = nil
	m.listState.hasSelection = false
	m.listState.cursor = 0
	m.listState.offset = 0

	return nil
}

// handleSearchUpdated handles search updates
func (m *Model) handleSearchUpdated(msg SearchUpdatedMsg) tea.Cmd {
	// Similar to filter updates
	m.selectedIndex = -1
	m.selectedTask = nil
	m.listState.hasSelection = false
	m.listState.cursor = 0
	m.listState.offset = 0

	return nil
}

// handleTaskSelected handles task selection
func (m *Model) handleTaskSelected(msg TaskSelectedMsg) tea.Cmd {
	m.selectedTask = msg.Task
	m.selectedIndex = msg.Index
	m.listState.hasSelection = true

	// Emit selection changed event
	return func() tea.Msg {
		return SelectionChangedMsg{
			Task:      msg.Task,
			Index:     msg.Index,
			Timestamp: time.Now(),
		}
	}
}

// handleTaskStatusChanged handles task status changes
func (m *Model) handleTaskStatusChanged(msg TaskStatusChangedMsg) tea.Cmd {
	// Update selected task if it matches
	if m.selectedTask != nil && m.selectedTask.ID == msg.Task.ID {
		m.selectedTask = msg.Task
	}

	return nil
}

// handleSidebarResized handles sidebar resize events
func (m *Model) handleSidebarResized(msg SidebarResizedMsg) tea.Cmd {
	// Recalculate visible items based on new height
	// This is a simplified calculation
	headerHeight := 3 // Approximate header height
	itemHeight := 1   // Approximate item height

	m.listState.visibleItems = (msg.Height - headerHeight) / itemHeight
	if m.listState.visibleItems < 0 {
		m.listState.visibleItems = 0
	}

	return nil
}

// handleFocusChanged handles focus change events
func (m *Model) handleFocusChanged(msg FocusChangedMsg) tea.Cmd {
	// Handle focus-specific logic if needed
	return nil
}

// GetSelectedTask returns the currently selected task
func (m *Model) GetSelectedTask() *task.Task {
	return m.selectedTask
}

// GetSelectedIndex returns the currently selected index
func (m *Model) GetSelectedIndex() int {
	return m.selectedIndex
}

// HasSelection returns whether there is a selection
func (m *Model) HasSelection() bool {
	return m.listState.hasSelection
}

// GetListState returns the current list state
func (m *Model) GetListState() ListState {
	return m.listState
}

// IsInitialized returns whether the model is initialized
func (m *Model) IsInitialized() bool {
	return m.initialized
}

// GetStats returns model statistics
func (m *Model) GetStats() ModelStats {
	return ModelStats{
		UpdateCount:   m.updateCount,
		RenderCount:   m.renderCount,
		LastUpdate:    m.lastUpdate,
		LastRender:    m.lastRenderTime,
		TotalItems:    m.listState.totalItems,
		VisibleItems:  m.listState.visibleItems,
		HasSelection:  m.listState.hasSelection,
		SelectedIndex: m.selectedIndex,
	}
}

// IncrementRenderCount increments the render counter
func (m *Model) IncrementRenderCount() {
	m.renderCount++
	m.lastRenderTime = time.Now()
}

// UpdateListState updates the list state
func (m *Model) UpdateListState(cursor, offset, totalItems, visibleItems int) {
	m.listState.cursor = cursor
	m.listState.offset = offset
	m.listState.totalItems = totalItems
	m.listState.visibleItems = visibleItems
	m.listState.hasSelection = cursor >= 0 && cursor < totalItems
}

// SelectionChangedMsg is sent when the selection changes
type SelectionChangedMsg struct {
	Task      *task.Task
	Index     int
	Timestamp time.Time
}

// ModelStats provides statistics about the model
type ModelStats struct {
	UpdateCount   int64     `json:"update_count"`
	RenderCount   int64     `json:"render_count"`
	LastUpdate    time.Time `json:"last_update"`
	LastRender    time.Time `json:"last_render"`
	TotalItems    int       `json:"total_items"`
	VisibleItems  int       `json:"visible_items"`
	HasSelection  bool      `json:"has_selection"`
	SelectedIndex int       `json:"selected_index"`
}

// GroupHeaderItem represents a group header in the list
type GroupHeaderItem struct {
	name  string
	count int
}

// FilterValue returns the filter value for group headers
func (g GroupHeaderItem) FilterValue() string {
	return g.name
}

// Title returns the title for group headers
func (g GroupHeaderItem) Title() string {
	return g.name
}

// Description returns the description for group headers
func (g GroupHeaderItem) Description() string {
	return fmt.Sprintf("%d tasks", g.count)
}

// TaskDelegate implements the list.ItemDelegate interface for task items
type TaskDelegate struct {
	styles *Styles
}

// NewTaskDelegate creates a new task delegate
func NewTaskDelegate(styles *Styles) list.DefaultDelegate {
	delegate := list.NewDefaultDelegate()

	// Customize the delegate appearance
	delegate.ShowDescription = true
	delegate.SetHeight(2)  // Height for each item
	delegate.SetSpacing(0) // Spacing between items

	return delegate
}

// Render renders a list item
func (d TaskDelegate) Render(w io.Writer, m list.Model, index int, listItem list.Item) {
	var str string

	switch item := listItem.(type) {
	case TaskItem:
		str = d.renderTaskItem(item, index == m.Index())
	case GroupHeaderItem:
		str = d.renderGroupHeader(item)
	default:
		str = item.FilterValue()
	}

	fmt.Fprint(w, str)
}

// renderTaskItem renders a task item
func (d TaskDelegate) renderTaskItem(item TaskItem, isSelected bool) string {
	var style lipgloss.Style

	if isSelected {
		style = d.styles.TaskSelected
	} else {
		switch item.task.Status {
		case task.StatusRunning:
			style = d.styles.TaskRunning
		case task.StatusSuccess:
			style = d.styles.TaskSuccess
		case task.StatusFailed:
			style = d.styles.TaskFailed
		default:
			if item.isHidden {
				style = d.styles.TaskHidden
			} else {
				style = d.styles.TaskItem
			}
		}
	}

	title := item.Title()
	description := item.Description()

	// Apply style to the entire item
	content := title + "\n" + description
	return style.Render(content)
}

// renderGroupHeader renders a group header
func (d TaskDelegate) renderGroupHeader(item GroupHeaderItem) string {
	content := fmt.Sprintf("▼ %s (%d)", item.name, item.count)
	return d.styles.GroupHeader.Render(content)
}

// Height returns the height of list items
func (d TaskDelegate) Height() int {
	return 2 // Title + description
}

// Spacing returns the spacing between list items
func (d TaskDelegate) Spacing() int {
	return 0
}

// Update handles updates for the delegate
func (d TaskDelegate) Update(msg tea.Msg, m *list.Model) tea.Cmd {
	return nil
}

// FilterState represents the current filtering state
type FilterState struct {
	Runner    string
	Status    string
	Tags      []string
	HasSearch bool
}

// IsEmpty returns true if no filters are active
func (f FilterState) IsEmpty() bool {
	return f.Runner == "" &&
		f.Status == "" &&
		len(f.Tags) == 0 &&
		!f.HasSearch
}

// String returns a string representation of the filter state
func (f FilterState) String() string {
	var parts []string

	if f.Runner != "" {
		parts = append(parts, "runner:"+f.Runner)
	}

	if f.Status != "" {
		parts = append(parts, "status:"+f.Status)
	}

	if len(f.Tags) > 0 {
		parts = append(parts, "tags:"+strings.Join(f.Tags, ","))
	}

	if f.HasSearch {
		parts = append(parts, "search:active")
	}

	if len(parts) == 0 {
		return "none"
	}

	return strings.Join(parts, " ")
}

// Copy creates a copy of the filter state
func (f FilterState) Copy() FilterState {
	tags := make([]string, len(f.Tags))
	copy(tags, f.Tags)

	return FilterState{
		Runner:    f.Runner,
		Status:    f.Status,
		Tags:      tags,
		HasSearch: f.HasSearch,
	}
}

// Equals compares two filter states
func (f FilterState) Equals(other FilterState) bool {
	if f.Runner != other.Runner || f.Status != other.Status || f.HasSearch != other.HasSearch {
		return false
	}

	if len(f.Tags) != len(other.Tags) {
		return false
	}

	for i, tag := range f.Tags {
		if tag != other.Tags[i] {
			return false
		}
	}

	return true
}

// KeyMap defines the key bindings for the sidebar
type KeyMap struct {
	Up           key.Binding
	Down         key.Binding
	Select       key.Binding
	Run          key.Binding
	DryRun       key.Binding
	ToggleHidden key.Binding
	Group        key.Binding
	Sort         key.Binding
	Refresh      key.Binding
	Filter       key.Binding
	Search       key.Binding
}

// DefaultKeyMap returns the default key mappings
func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up: key.NewBinding(
			key.WithKeys("up", "k"),
			key.WithHelp("↑/k", "move up"),
		),
		Down: key.NewBinding(
			key.WithKeys("down", "j"),
			key.WithHelp("↓/j", "move down"),
		),
		Select: key.NewBinding(
			key.WithKeys("enter"),
			key.WithHelp("enter", "select task"),
		),
		Run: key.NewBinding(
			key.WithKeys("r"),
			key.WithHelp("r", "run task"),
		),
		DryRun: key.NewBinding(
			key.WithKeys("d"),
			key.WithHelp("d", "dry run task"),
		),
		ToggleHidden: key.NewBinding(
			key.WithKeys("h"),
			key.WithHelp("h", "toggle hidden"),
		),
		Group: key.NewBinding(
			key.WithKeys("g"),
			key.WithHelp("g", "cycle grouping"),
		),
		Sort: key.NewBinding(
			key.WithKeys("s"),
			key.WithHelp("s", "cycle sorting"),
		),
		Refresh: key.NewBinding(
			key.WithKeys("ctrl+r"),
			key.WithHelp("ctrl+r", "refresh"),
		),
		Filter: key.NewBinding(
			key.WithKeys("f"),
			key.WithHelp("f", "filter"),
		),
		Search: key.NewBinding(
			key.WithKeys("/"),
			key.WithHelp("/", "search"),
		),
	}
}

// ShortHelp returns the short help text
func (k KeyMap) ShortHelp() []key.Binding {
	return []key.Binding{k.Up, k.Down, k.Select, k.Run}
}

// FullHelp returns the full help text
func (k KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		{k.Up, k.Down, k.Select, k.Run},
		{k.DryRun, k.ToggleHidden, k.Group, k.Sort},
		{k.Refresh, k.Filter, k.Search},
	}
}
