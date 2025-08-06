package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/vivalchemy/wake/internal/config"
	"github.com/vivalchemy/wake/internal/tui/components/layout"
	"github.com/vivalchemy/wake/internal/tui/keybindings"
	"github.com/vivalchemy/wake/internal/tui/theme"
	"github.com/vivalchemy/wake/pkg/logger"
	"github.com/vivalchemy/wake/pkg/task"
)

// Model represents the main TUI model
type Model struct {
	// Configuration
	config *config.Config
	logger logger.Logger

	// Components
	layout        *layout.Layout
	keyManager    *keybindings.KeyManager
	themeProvider *theme.ThemeProvider

	// State
	state *ModelState

	// UI state
	width    int
	height   int
	ready    bool
	quitting bool

	// Error handling
	lastError error
	showError bool
	errorTime time.Time

	// Help state
	showHelp    bool
	helpContent string

	// Debug state
	debugMode bool
	debugInfo *DebugInfo
}

// ModelState represents the current state of the model
type ModelState struct {
	// Session info
	SessionStart    time.Time     `json:"session_start"`
	SessionDuration time.Duration `json:"session_duration"`

	// Task state
	Tasks        []*task.Task `json:"tasks"`
	SelectedTask *task.Task   `json:"selected_task"`
	RunningTasks []string     `json:"running_tasks"`
	TasksRun     int          `json:"tasks_run"`

	// UI state
	CurrentTheme string `json:"current_theme"`
	KeyProfile   string `json:"key_profile"`
	SplitMode    string `json:"split_mode"`
	SidebarWidth int    `json:"sidebar_width"`
	ShowSearch   bool   `json:"show_search"`
	SearchQuery  string `json:"search_query"`

	// Statistics
	KeyPresses  int64 `json:"key_presses"`
	CommandsRun int64 `json:"commands_run"`
	ErrorCount  int64 `json:"error_count"`

	// Performance
	LastRenderTime    time.Duration `json:"last_render_time"`
	AverageRenderTime time.Duration `json:"average_render_time"`
	RenderCount       int64         `json:"render_count"`
}

// DebugInfo contains debug information
type DebugInfo struct {
	ModelUpdates    int64          `json:"model_updates"`
	LastUpdate      time.Time      `json:"last_update"`
	LastMessage     string         `json:"last_message"`
	MessageQueue    int            `json:"message_queue"`
	MemoryUsage     int64          `json:"memory_usage"`
	GoroutineCount  int            `json:"goroutine_count"`
	ComponentStates map[string]any `json:"component_states"`
}

// NewModel creates a new TUI model
func NewModel(config *config.Config, logger logger.Logger) *Model {
	return &Model{
		config: config,
		logger: logger.WithGroup("tui-model"),
		state: &ModelState{
			SessionStart: time.Now(),
			Tasks:        make([]*task.Task, 0),
			RunningTasks: make([]string, 0),
			CurrentTheme: "default",
			KeyProfile:   "default",
			SplitMode:    "vertical",
			SidebarWidth: 40,
		},
		ready:     false,
		quitting:  false,
		debugMode: false,
		debugInfo: &DebugInfo{
			ComponentStates: make(map[string]any),
		},
	}
}

// SetComponents sets the UI components
func (m *Model) SetComponents(layout *layout.Layout, keyManager *keybindings.KeyManager, themeProvider *theme.ThemeProvider) {
	m.layout = layout
	m.keyManager = keyManager
	m.themeProvider = themeProvider
}

// Init initializes the model
func (m *Model) Init() tea.Cmd {
	m.logger.Debug("Initializing TUI model")

	var cmds []tea.Cmd

	// Initialize components
	if m.layout != nil {
		cmds = append(cmds, m.layout.Init())
	}

	// Start periodic updates
	cmds = append(cmds, m.tickCmd())

	return tea.Batch(cmds...)
}

// Update handles messages and updates the model
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	startTime := time.Now()

	// Update debug info
	m.updateDebugInfo(msg)

	var cmds []tea.Cmd
	var cmd tea.Cmd

	// Handle global messages first
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.logger.Debug("Window resized", "width", msg.Width, "height", msg.Height)

	case tea.KeyMsg:
		// Increment key press counter
		m.state.KeyPresses++

		// Handle global keys
		if m.keyManager != nil {
			cmd = m.keyManager.HandleKey(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

		// Handle model-specific keys
		cmd = m.handleKeyPress(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case QuitMsg:
		m.quitting = true
		return m, tea.Quit

	case ErrorMsg:
		m.handleError(msg.Error)

	case ShowHelpMsg:
		m.showHelp = !m.showHelp
		m.updateHelpContent()

	case RefreshTasksMsg:
		cmd = m.refreshTasks()
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case TasksDiscoveredMsg:
		m.state.Tasks = msg.Tasks
		m.logger.Info("Tasks updated", "count", len(msg.Tasks))

	case RunSelectedTaskMsg:
		cmd = m.runSelectedTask()
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case RunTaskMsg:
		cmd = m.runTask(msg.TaskID)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case TaskStartedMsg:
		m.handleTaskStarted(msg.TaskID)

	case TaskFinishedMsg:
		m.handleTaskFinished(msg.TaskID, msg.ExitCode, msg.Error)

	case ToggleSearchMsg:
		cmd = m.toggleSearch()
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case ThemeChangedMsg:
		m.state.CurrentTheme = msg.Theme
		m.logger.Info("Theme changed", "theme", msg.Theme)

	case KeyProfileChangedMsg:
		m.state.KeyProfile = msg.Profile
		m.logger.Info("Key profile changed", "profile", msg.Profile)

	case EnableDebugMsg:
		m.debugMode = true
		m.logger.Info("Debug mode enabled")

	case DisableDebugMsg:
		m.debugMode = false
		m.logger.Info("Debug mode disabled")

	case TickMsg:
		// Periodic update
		m.updateSessionDuration()
		cmds = append(cmds, m.tickCmd())

	case DiscoveryStartedMsg:
		m.logger.Debug("Task discovery started")

	case DiscoveryCompletedMsg:
		m.logger.Debug("Task discovery completed")

	case ConfigChangedMsg:
		m.logger.Debug("Configuration changed")

	case TaskCountChangedMsg:
		m.logger.Debug("Task count changed", "count", msg.Count)
	}

	// Update layout component
	if m.layout != nil && m.ready {
		m.layout, cmd = m.layout.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	// Update render time statistics
	renderTime := time.Since(startTime)
	m.updateRenderStats(renderTime)

	return m, tea.Batch(cmds...)
}

// View renders the model
func (m *Model) View() string {
	if !m.ready {
		return m.renderLoading()
	}

	if m.quitting {
		return m.renderGoodbye()
	}

	var content string

	if m.showError {
		content = m.renderError()
	} else if m.showHelp {
		content = m.renderHelp()
	} else if m.layout != nil {
		content = m.layout.View()
	} else {
		content = m.renderFallback()
	}

	// Add debug overlay if enabled
	if m.debugMode {
		content = m.addDebugOverlay(content)
	}

	return content
}

// handleKeyPress handles model-specific key presses
func (m *Model) handleKeyPress(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "ctrl+c":
		return func() tea.Msg { return QuitMsg{} }

	case "?":
		return func() tea.Msg { return ShowHelpMsg{} }

	case "ctrl+r":
		return func() tea.Msg { return RefreshTasksMsg{} }

	case "d":
		if m.debugMode {
			return func() tea.Msg { return DisableDebugMsg{} }
		} else {
			return func() tea.Msg { return EnableDebugMsg{} }
		}

	case "esc":
		if m.showError {
			m.showError = false
		} else if m.showHelp {
			m.showHelp = false
		}
	}

	return nil
}

// handleError handles error messages
func (m *Model) handleError(err error) {
	m.lastError = err
	m.showError = true
	m.errorTime = time.Now()
	m.state.ErrorCount++

	m.logger.Error("TUI error", "error", err)

	// Auto-hide error after 5 seconds
	go func() {
		time.Sleep(5 * time.Second)
		if time.Since(m.errorTime) >= 5*time.Second {
			m.showError = false
		}
	}()
}

// refreshTasks refreshes the task list
func (m *Model) refreshTasks() tea.Cmd {
	return func() tea.Msg {
		// This would trigger task discovery
		return RefreshStartedMsg{}
	}
}

// runSelectedTask runs the currently selected task
func (m *Model) runSelectedTask() tea.Cmd {
	if m.state.SelectedTask == nil {
		return func() tea.Msg {
			return ErrorMsg{Error: fmt.Errorf("no task selected")}
		}
	}

	return m.runTask(m.state.SelectedTask.ID)
}

// runTask runs a specific task
func (m *Model) runTask(taskID string) tea.Cmd {
	// Find the task
	var targetTask *task.Task
	for _, t := range m.state.Tasks {
		if t.ID == taskID {
			targetTask = t
			break
		}
	}

	if targetTask == nil {
		return func() tea.Msg {
			return ErrorMsg{Error: fmt.Errorf("task not found: %s", taskID)}
		}
	}

	m.state.CommandsRun++
	m.logger.Info("Running task", "task", targetTask.Name, "id", taskID)

	return func() tea.Msg {
		return TaskStartedMsg{TaskID: taskID}
	}
}

// handleTaskStarted handles task start events
func (m *Model) handleTaskStarted(taskID string) {
	m.state.RunningTasks = append(m.state.RunningTasks, taskID)
	m.state.TasksRun++

	m.logger.Debug("Task started", "task_id", taskID)
}

// handleTaskFinished handles task completion events
func (m *Model) handleTaskFinished(taskID string, exitCode int, err error) {
	// Remove from running tasks
	for i, id := range m.state.RunningTasks {
		if id == taskID {
			m.state.RunningTasks = append(m.state.RunningTasks[:i], m.state.RunningTasks[i+1:]...)
			break
		}
	}

	if err != nil {
		m.logger.Error("Task failed", "task_id", taskID, "exit_code", exitCode, "error", err)
		m.handleError(fmt.Errorf("task %s failed: %w", taskID, err))
	} else {
		m.logger.Info("Task completed", "task_id", taskID, "exit_code", exitCode)
	}
}

// toggleSearch toggles the search interface
func (m *Model) toggleSearch() tea.Cmd {
	m.state.ShowSearch = !m.state.ShowSearch

	if m.layout != nil {
		if m.state.ShowSearch {
			return func() tea.Msg {
				return layout.ShowSearchMsg{InitialQuery: m.state.SearchQuery}
			}
		} else {
			return func() tea.Msg {
				return layout.HideSearchMsg{}
			}
		}
	}

	return nil
}

// updateSessionDuration updates the session duration
func (m *Model) updateSessionDuration() {
	m.state.SessionDuration = time.Since(m.state.SessionStart)
}

// updateRenderStats updates render time statistics
func (m *Model) updateRenderStats(renderTime time.Duration) {
	m.state.RenderCount++
	m.state.LastRenderTime = renderTime

	// Calculate running average
	if m.state.RenderCount == 1 {
		m.state.AverageRenderTime = renderTime
	} else {
		// Simple running average
		alpha := 0.1 // Smoothing factor
		m.state.AverageRenderTime = time.Duration(
			float64(m.state.AverageRenderTime)*(1-alpha) + float64(renderTime)*alpha,
		)
	}
}

// updateDebugInfo updates debug information
func (m *Model) updateDebugInfo(msg tea.Msg) {
	m.debugInfo.ModelUpdates++
	m.debugInfo.LastUpdate = time.Now()
	m.debugInfo.LastMessage = fmt.Sprintf("%T", msg)

	// Update component states
	if m.layout != nil {
		m.debugInfo.ComponentStates["layout_focused"] = m.layout.GetFocusedComponent().String()
		m.debugInfo.ComponentStates["layout_split"] = m.layout.GetSplitMode().String()
		m.debugInfo.ComponentStates["search_visible"] = m.layout.IsSearchVisible()
	}
}

// updateHelpContent updates the help content
func (m *Model) updateHelpContent() {
	if m.keyManager == nil {
		m.helpContent = "No key bindings available"
		return
	}

	sections := m.keyManager.GetHelpSections()
	var content []string

	content = append(content, "Wake TUI - Help")
	content = append(content, "")

	for _, section := range sections {
		content = append(content, fmt.Sprintf("## %s", section.Title))
		content = append(content, "")

		for _, key := range section.Keys {
			help := key.Help()
			content = append(content, fmt.Sprintf("  %-20s %s", help.Key, help.Desc))
		}
		content = append(content, "")
	}

	m.helpContent = strings.Join(content, "\n")
}

// tickCmd returns a command for periodic updates
func (m *Model) tickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg {
		return TickMsg{Time: t}
	})
}

// Render methods

// renderLoading renders the loading screen
func (m *Model) renderLoading() string {
	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#00FF00")).
			Render("Loading Wake TUI..."),
	)
}

// renderGoodbye renders the goodbye screen
func (m *Model) renderGoodbye() string {
	stats := fmt.Sprintf(
		"Session Stats:\n"+
			"Duration: %v\n"+
			"Tasks run: %d\n"+
			"Key presses: %d\n"+
			"Commands: %d\n",
		m.state.SessionDuration.Round(time.Second),
		m.state.TasksRun,
		m.state.KeyPresses,
		m.state.CommandsRun,
	)

	goodbye := "Thanks for using Wake!\n\n" + stats

	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#00FFFF")).
			Render(goodbye),
	)
}

// renderError renders the error screen
func (m *Model) renderError() string {
	if m.lastError == nil {
		return ""
	}

	errorStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#FF0000")).
		Foreground(lipgloss.Color("#FFFFFF")).
		Padding(1, 2).
		Bold(true)

	errorMsg := fmt.Sprintf("Error: %s\n\nPress ESC to dismiss", m.lastError.Error())

	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		errorStyle.Render(errorMsg),
	)
}

// renderHelp renders the help screen
func (m *Model) renderHelp() string {
	helpStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#00FFFF")).
		Padding(1, 2).
		MaxWidth(m.width - 4).
		MaxHeight(m.height - 4)

	content := m.helpContent
	if content == "" {
		content = "Help content not available"
	}

	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		helpStyle.Render(content),
	)
}

// renderFallback renders a fallback interface
func (m *Model) renderFallback() string {
	fallback := "Wake TUI\n\nNo components available\nPress ? for help"

	return lipgloss.Place(
		m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(2).
			Render(fallback),
	)
}

// addDebugOverlay adds debug information overlay
func (m *Model) addDebugOverlay(content string) string {
	debugInfo := fmt.Sprintf(
		"DEBUG MODE\n"+
			"Updates: %d\n"+
			"Last: %s\n"+
			"Render: %v\n"+
			"Avg Render: %v\n"+
			"Tasks: %d\n"+
			"Running: %d\n"+
			"Errors: %d\n",
		m.debugInfo.ModelUpdates,
		m.debugInfo.LastMessage,
		m.state.LastRenderTime,
		m.state.AverageRenderTime,
		len(m.state.Tasks),
		len(m.state.RunningTasks),
		m.state.ErrorCount,
	)

	debugStyle := lipgloss.NewStyle().
		Background(lipgloss.Color("#000000")).
		Foreground(lipgloss.Color("#00FF00")).
		Border(lipgloss.NormalBorder()).
		Padding(0, 1).
		Width(25)

	debugOverlay := debugStyle.Render(debugInfo)

	// Position debug overlay in top-right corner
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		content,
		lipgloss.NewStyle().Width(m.width-lipgloss.Width(content)-25).Render(""),
		debugOverlay,
	)
}

// GetState returns the current model state
func (m *Model) GetState() *ModelState {
	m.updateSessionDuration()
	return m.state
}

// GetDebugInfo returns debug information
func (m *Model) GetDebugInfo() *DebugInfo {
	return m.debugInfo
}

// IsReady returns whether the model is ready
func (m *Model) IsReady() bool {
	return m.ready
}

// IsQuitting returns whether the model is quitting
func (m *Model) IsQuitting() bool {
	return m.quitting
}

// SetSelectedTask sets the selected task
func (m *Model) SetSelectedTask(task *task.Task) {
	m.state.SelectedTask = task
}

// GetSelectedTask returns the selected task
func (m *Model) GetSelectedTask() *task.Task {
	return m.state.SelectedTask
}

// GetTasks returns all tasks
func (m *Model) GetTasks() []*task.Task {
	return m.state.Tasks
}

// GetRunningTasks returns running task IDs
func (m *Model) GetRunningTasks() []string {
	return m.state.RunningTasks
}

// SetTasks sets the task list
func (m *Model) SetTasks(tasks []*task.Task) {
	m.state.Tasks = tasks
}
