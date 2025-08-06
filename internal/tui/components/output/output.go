package output

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/vivalchemy/wake/internal/tui/theme"
	"github.com/vivalchemy/wake/pkg/logger"
	"github.com/vivalchemy/wake/pkg/task"
)

// Output represents the output display component
type Output struct {
	// Model state
	model    *Model
	viewport viewport.Model
	width    int
	height   int
	focused  bool

	// Display state
	currentTask *task.Task
	buffer      *OutputBuffer

	// Display options
	showTimestamps bool
	showTaskInfo   bool
	autoScroll     bool
	wrapLines      bool
	maxLines       int

	// Filtering
	showStdout  bool
	showStderr  bool
	filterLevel LogLevel

	// Styles
	styles *Styles

	// Logger
	logger logger.Logger
}

// LogLevel represents different log levels for filtering
type LogLevel int

const (
	LogLevelAll LogLevel = iota
	LogLevelError
	LogLevelWarn
	LogLevelInfo
	LogLevelDebug
)

// String returns the string representation of LogLevel
func (l LogLevel) String() string {
	switch l {
	case LogLevelAll:
		return "all"
	case LogLevelError:
		return "error"
	case LogLevelWarn:
		return "warn"
	case LogLevelInfo:
		return "info"
	case LogLevelDebug:
		return "debug"
	default:
		return "all"
	}
}

// Styles defines the visual styles for the output component
type Styles struct {
	Container       lipgloss.Style
	Header          lipgloss.Style
	TaskInfo        lipgloss.Style
	OutputLine      lipgloss.Style
	StdoutLine      lipgloss.Style
	StderrLine      lipgloss.Style
	ErrorLine       lipgloss.Style
	WarnLine        lipgloss.Style
	InfoLine        lipgloss.Style
	DebugLine       lipgloss.Style
	Timestamp       lipgloss.Style
	LineNumber      lipgloss.Style
	NoOutput        lipgloss.Style
	StatusBar       lipgloss.Style
	FilterInfo      lipgloss.Style
	ScrollIndicator lipgloss.Style
}

// OutputBuffer manages the output lines
type OutputBuffer struct {
	lines      []*OutputLine
	maxLines   int
	totalLines int
	hasStdout  bool
	hasStderr  bool
	lastUpdate time.Time
}

// OutputLine represents a single line of output
type OutputLine struct {
	Content     string
	Timestamp   time.Time
	TaskID      string
	TaskName    string
	IsStderr    bool
	Level       LogLevel
	LineNumber  int
	Highlighted bool
}

// New creates a new output component
func New(logger logger.Logger, themeProvider *theme.Theme) *Output {
	styles := createStyles(themeProvider)

	// Create viewport
	vp := viewport.New(0, 0)
	vp.SetContent("")

	// Create buffer
	buffer := &OutputBuffer{
		lines:      make([]*OutputLine, 0),
		maxLines:   10000, // Default max lines
		lastUpdate: time.Now(),
	}

	// Create model
	model := NewModel()

	output := &Output{
		model:          model,
		viewport:       vp,
		buffer:         buffer,
		styles:         styles,
		logger:         logger.WithGroup("output"),
		showTimestamps: true,
		showTaskInfo:   true,
		autoScroll:     true,
		wrapLines:      true,
		maxLines:       10000,
		showStdout:     true,
		showStderr:     true,
		filterLevel:    LogLevelAll,
	}

	return output
}

// Init initializes the output component
func (o *Output) Init() tea.Cmd {
	return o.model.Init()
}

// Update handles messages for the output component
func (o *Output) Update(msg tea.Msg) (*Output, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	// Update model
	o.model, cmd = o.model.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if o.focused {
			cmd = o.handleKeyPress(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

	case tea.WindowSizeMsg:
		o.width = msg.Width
		o.height = msg.Height
		o.updateSize()

	case TaskSelectedMsg:
		o.currentTask = msg.Task
		o.clearOutput()
		o.updateContent()

	case OutputLineMsg:
		o.addOutputLine(msg.Line)
		if o.autoScroll {
			o.scrollToBottom()
		}
		o.updateContent()

	case TaskStartedMsg:
		o.addTaskEvent(msg.Task, "Task started")

	case TaskFinishedMsg:
		o.addTaskEvent(msg.Task, fmt.Sprintf("Task finished (exit code: %d)", msg.ExitCode))

	case TaskFailedMsg:
		o.addTaskEvent(msg.Task, fmt.Sprintf("Task failed: %s", msg.Error))

	case ClearOutputMsg:
		o.clearOutput()
		o.updateContent()

	case ToggleTimestampsMsg:
		o.showTimestamps = !o.showTimestamps
		o.updateContent()

	case ToggleTaskInfoMsg:
		o.showTaskInfo = !o.showTaskInfo
		o.updateContent()

	case ToggleAutoScrollMsg:
		o.autoScroll = !o.autoScroll

	case ToggleWrapLinesMsg:
		o.wrapLines = !o.wrapLines
		o.updateContent()

	case SetFilterLevelMsg:
		o.filterLevel = msg.Level
		o.updateContent()

	case ToggleStdoutMsg:
		o.showStdout = !o.showStdout
		o.updateContent()

	case ToggleStderrMsg:
		o.showStderr = !o.showStderr
		o.updateContent()
	}

	// Update viewport
	o.viewport, cmd = o.viewport.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	return o, tea.Batch(cmds...)
}

// View renders the output component
func (o *Output) View() string {
	if o.width == 0 || o.height == 0 {
		return ""
	}

	content := o.renderContent()

	return o.styles.Container.
		Width(o.width).
		Height(o.height).
		Render(content)
}

// renderContent renders the main content
func (o *Output) renderContent() string {
	var sections []string

	// Header
	sections = append(sections, o.renderHeader())

	// Task info (if enabled and task selected)
	if o.showTaskInfo && o.currentTask != nil {
		sections = append(sections, o.renderTaskInfo())
	}

	// Filter info (if filters are active)
	if o.hasActiveFilters() {
		sections = append(sections, o.renderFilterInfo())
	}

	// Output viewport
	sections = append(sections, o.renderOutput())

	// Status bar
	sections = append(sections, o.renderStatusBar())

	return strings.Join(sections, "\n")
}

// renderHeader renders the header
func (o *Output) renderHeader() string {
	title := "Output"

	if o.currentTask != nil {
		title = fmt.Sprintf("Output - %s", o.currentTask.Name)
	}

	return o.styles.Header.Render(title)
}

// renderTaskInfo renders task information
func (o *Output) renderTaskInfo() string {
	if o.currentTask == nil {
		return ""
	}

	task := o.currentTask
	var info []string

	info = append(info, fmt.Sprintf("Runner: %s", task.Runner))
	info = append(info, fmt.Sprintf("Status: %s", o.formatStatus(task.Status)))

	if task.Description != "" {
		info = append(info, fmt.Sprintf("Description: %s", task.Description))
	}

	if task.FilePath != "" {
		info = append(info, fmt.Sprintf("File: %s", task.FilePath))
	}

	if len(task.Tags) > 0 {
		info = append(info, fmt.Sprintf("Tags: %s", strings.Join(task.Tags, ", ")))
	}

	content := strings.Join(info, " • ")
	return o.styles.TaskInfo.Render(content)
}

// renderFilterInfo renders active filter information
func (o *Output) renderFilterInfo() string {
	var filters []string

	if !o.showStdout {
		filters = append(filters, "stdout:hidden")
	}

	if !o.showStderr {
		filters = append(filters, "stderr:hidden")
	}

	if o.filterLevel != LogLevelAll {
		filters = append(filters, fmt.Sprintf("level:%s+", o.filterLevel.String()))
	}

	if len(filters) == 0 {
		return ""
	}

	filterText := "Filters: " + strings.Join(filters, " ")
	return o.styles.FilterInfo.Render(filterText)
}

// renderOutput renders the output viewport
func (o *Output) renderOutput() string {
	if o.buffer.totalLines == 0 {
		return o.renderNoOutput()
	}

	// Calculate available height
	headerHeight := lipgloss.Height(o.renderHeader())
	taskInfoHeight := 0
	if o.showTaskInfo && o.currentTask != nil {
		taskInfoHeight = lipgloss.Height(o.renderTaskInfo())
	}
	filterHeight := 0
	if o.hasActiveFilters() {
		filterHeight = lipgloss.Height(o.renderFilterInfo())
	}
	statusHeight := lipgloss.Height(o.renderStatusBar())

	availableHeight := o.height - headerHeight - taskInfoHeight - filterHeight - statusHeight - 4 // padding

	o.viewport.Width = o.width - 4 // account for container padding
	o.viewport.Height = availableHeight

	return o.viewport.View()
}

// renderNoOutput renders the no output message
func (o *Output) renderNoOutput() string {
	var message string

	if o.currentTask == nil {
		message = "Select a task to view its output"
	} else {
		switch o.currentTask.Status {
		case task.StatusIdle:
			message = "Task not started yet"
		case task.StatusRunning:
			message = "Waiting for output..."
		default:
			message = "No output available"
		}
	}

	return o.styles.NoOutput.Render(message)
}

// renderStatusBar renders the status bar
func (o *Output) renderStatusBar() string {
	var parts []string

	// Line count
	visibleLines := o.countVisibleLines()
	totalLines := o.buffer.totalLines
	parts = append(parts, fmt.Sprintf("Lines: %d/%d", visibleLines, totalLines))

	// Scroll position
	if o.viewport.TotalLineCount() > 0 {
		percentage := int((float64(o.viewport.YOffset) / float64(o.viewport.TotalLineCount())) * 100)
		parts = append(parts, fmt.Sprintf("Scroll: %d%%", percentage))
	}

	// Auto-scroll indicator
	if o.autoScroll {
		parts = append(parts, "Auto-scroll: ON")
	}

	// Wrap indicator
	if o.wrapLines {
		parts = append(parts, "Wrap: ON")
	}

	statusText := strings.Join(parts, " • ")
	return o.styles.StatusBar.Render(statusText)
}

// handleKeyPress handles key press events
func (o *Output) handleKeyPress(msg tea.KeyMsg) tea.Cmd {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("c"))):
		return o.clearOutput()

	case key.Matches(msg, key.NewBinding(key.WithKeys("t"))):
		return func() tea.Msg { return ToggleTimestampsMsg{} }

	case key.Matches(msg, key.NewBinding(key.WithKeys("i"))):
		return func() tea.Msg { return ToggleTaskInfoMsg{} }

	case key.Matches(msg, key.NewBinding(key.WithKeys("a"))):
		return func() tea.Msg { return ToggleAutoScrollMsg{} }

	case key.Matches(msg, key.NewBinding(key.WithKeys("w"))):
		return func() tea.Msg { return ToggleWrapLinesMsg{} }

	case key.Matches(msg, key.NewBinding(key.WithKeys("1"))):
		return func() tea.Msg { return ToggleStdoutMsg{} }

	case key.Matches(msg, key.NewBinding(key.WithKeys("2"))):
		return func() tea.Msg { return ToggleStderrMsg{} }

	case key.Matches(msg, key.NewBinding(key.WithKeys("g"))):
		o.viewport.GotoTop()
		return nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("G"))):
		o.viewport.GotoBottom()
		return nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("home"))):
		o.viewport.GotoTop()
		return nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("end"))):
		o.viewport.GotoBottom()
		return nil

	case key.Matches(msg, key.NewBinding(key.WithKeys("f"))):
		return o.cycleFilterLevel()
	}

	return nil
}

// addOutputLine adds a new output line
func (o *Output) addOutputLine(line *OutputLine) {
	o.buffer.addLine(line)
	o.model.IncrementLineCount()
}

// addTaskEvent adds a task event line
func (o *Output) addTaskEvent(task *task.Task, message string) {
	line := &OutputLine{
		Content:    message,
		Timestamp:  time.Now(),
		TaskID:     task.ID,
		TaskName:   task.Name,
		IsStderr:   false,
		Level:      LogLevelInfo,
		LineNumber: o.buffer.totalLines + 1,
	}

	o.addOutputLine(line)
}

// clearOutput clears all output
func (o *Output) clearOutput() tea.Cmd {
	o.buffer.clear()
	o.viewport.SetContent("")
	o.model.ClearLines()
	return func() tea.Msg { return OutputClearedMsg{} }
}

// updateContent updates the viewport content
func (o *Output) updateContent() {
	content := o.formatOutput()
	o.viewport.SetContent(content)
}

// formatOutput formats the output for display
func (o *Output) formatOutput() string {
	filteredLines := o.getFilteredLines()

	if len(filteredLines) == 0 {
		return ""
	}

	var formatted []string

	for _, line := range filteredLines {
		formattedLine := o.formatLine(line)
		formatted = append(formatted, formattedLine)
	}

	return strings.Join(formatted, "\n")
}

// getFilteredLines returns lines that pass the current filters
func (o *Output) getFilteredLines() []*OutputLine {
	var filtered []*OutputLine

	for _, line := range o.buffer.lines {
		// Filter by stdout/stderr
		if line.IsStderr && !o.showStderr {
			continue
		}
		if !line.IsStderr && !o.showStdout {
			continue
		}

		// Filter by level
		if o.filterLevel != LogLevelAll && line.Level < o.filterLevel {
			continue
		}

		filtered = append(filtered, line)
	}

	return filtered
}

// formatLine formats a single output line
func (o *Output) formatLine(line *OutputLine) string {
	var parts []string
	var style lipgloss.Style

	// Choose style based on line type
	if line.IsStderr {
		style = o.styles.StderrLine
	} else {
		switch line.Level {
		case LogLevelError:
			style = o.styles.ErrorLine
		case LogLevelWarn:
			style = o.styles.WarnLine
		case LogLevelInfo:
			style = o.styles.InfoLine
		case LogLevelDebug:
			style = o.styles.DebugLine
		default:
			style = o.styles.StdoutLine
		}
	}

	// Add line number if enabled
	if o.showTimestamps || true { // Always show line numbers for now
		lineNum := o.styles.LineNumber.Render(fmt.Sprintf("%4d", line.LineNumber))
		parts = append(parts, lineNum)
	}

	// Add timestamp if enabled
	if o.showTimestamps {
		timestamp := o.styles.Timestamp.Render(line.Timestamp.Format("15:04:05.000"))
		parts = append(parts, timestamp)
	}

	// Add task name if multiple tasks or if enabled
	if o.showTaskInfo && line.TaskName != "" {
		taskName := fmt.Sprintf("[%s]", line.TaskName)
		parts = append(parts, taskName)
	}

	// Add content
	content := line.Content
	if o.wrapLines && o.viewport.Width > 0 {
		// Simple line wrapping
		maxWidth := o.viewport.Width - 20 // Account for timestamps, etc.
		if maxWidth > 0 && len(content) > maxWidth {
			// This is a simple implementation; a more sophisticated one might preserve word boundaries
			content = content[:maxWidth] + "..."
		}
	}

	parts = append(parts, content)

	// Join parts and apply style
	line_content := strings.Join(parts, " ")
	return style.Render(line_content)
}

// formatStatus formats a task status with appropriate styling
func (o *Output) formatStatus(status task.Status) string {
	switch status {
	case task.StatusRunning:
		return "🔄 Running"
	case task.StatusSuccess:
		return "✅ Success"
	case task.StatusFailed:
		return "❌ Failed"
	case task.StatusStopped:
		return "⏹️ Stopped"
	default:
		return "⏸️ Idle"
	}
}

// countVisibleLines counts lines that pass current filters
func (o *Output) countVisibleLines() int {
	return len(o.getFilteredLines())
}

// hasActiveFilters returns true if any filters are active
func (o *Output) hasActiveFilters() bool {
	return !o.showStdout || !o.showStderr || o.filterLevel != LogLevelAll
}

// scrollToBottom scrolls to the bottom of the output
func (o *Output) scrollToBottom() {
	o.viewport.GotoBottom()
}

// cycleFilterLevel cycles through filter levels
func (o *Output) cycleFilterLevel() tea.Cmd {
	nextLevel := (o.filterLevel + 1) % 5 // 5 filter levels
	return func() tea.Msg {
		return SetFilterLevelMsg{Level: nextLevel}
	}
}

// updateSize updates the component size
func (o *Output) updateSize() {
	// Viewport size will be updated in renderOutput
}

// SetFocused sets the focus state
func (o *Output) SetFocused(focused bool) {
	o.focused = focused
}

// IsFocused returns the focus state
func (o *Output) IsFocused() bool {
	return o.focused
}

// GetCurrentTask returns the current task
func (o *Output) GetCurrentTask() *task.Task {
	return o.currentTask
}

// SetCurrentTask sets the current task
func (o *Output) SetCurrentTask(task *task.Task) {
	o.currentTask = task
	o.clearOutput()
}

// GetBuffer returns the output buffer
func (o *Output) GetBuffer() *OutputBuffer {
	return o.buffer
}

// OutputBuffer methods

// addLine adds a line to the buffer
func (b *OutputBuffer) addLine(line *OutputLine) {
	b.lines = append(b.lines, line)
	b.totalLines++
	b.lastUpdate = time.Now()

	// Track output types
	if line.IsStderr {
		b.hasStderr = true
	} else {
		b.hasStdout = true
	}

	// Trim buffer if it exceeds max lines
	if len(b.lines) > b.maxLines {
		b.lines = b.lines[1:]
	}
}

// clear clears all lines from the buffer
func (b *OutputBuffer) clear() {
	b.lines = b.lines[:0]
	b.totalLines = 0
	b.hasStdout = false
	b.hasStderr = false
	b.lastUpdate = time.Now()
}

// getLines returns all lines in the buffer
func (b *OutputBuffer) getLines() []*OutputLine {
	return b.lines
}

// getLineCount returns the total number of lines
func (b *OutputBuffer) getLineCount() int {
	return b.totalLines
}

// hasOutput returns true if there is any output
func (b *OutputBuffer) hasOutput() bool {
	return len(b.lines) > 0
}

// createStyles creates the styles for the output component
func createStyles(themeProvider *theme.Theme) *Styles {
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

		TaskInfo: lipgloss.NewStyle().
			Foreground(colors.Secondary).
			MarginBottom(1),

		OutputLine: lipgloss.NewStyle().
			Foreground(colors.Text),

		StdoutLine: lipgloss.NewStyle().
			Foreground(colors.Text),

		StderrLine: lipgloss.NewStyle().
			Foreground(colors.Error),

		ErrorLine: lipgloss.NewStyle().
			Foreground(colors.Error).
			Bold(true),

		WarnLine: lipgloss.NewStyle().
			Foreground(colors.Warning),

		InfoLine: lipgloss.NewStyle().
			Foreground(colors.Info),

		DebugLine: lipgloss.NewStyle().
			Foreground(colors.Muted),

		Timestamp: lipgloss.NewStyle().
			Foreground(colors.Muted),

		LineNumber: lipgloss.NewStyle().
			Foreground(colors.Muted).
			Width(4).
			Align(lipgloss.Right),

		NoOutput: lipgloss.NewStyle().
			Foreground(colors.Muted).
			Italic(true).
			Align(lipgloss.Center),

		StatusBar: lipgloss.NewStyle().
			Foreground(colors.Muted).
			Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(colors.Border).
			Padding(0, 1),

		FilterInfo: lipgloss.NewStyle().
			Foreground(colors.Warning).
			Italic(true).
			MarginBottom(1),

		ScrollIndicator: lipgloss.NewStyle().
			Foreground(colors.Primary),
	}
}
