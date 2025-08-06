package layout

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/vivalchemy/wake/internal/tui/components/output"
	"github.com/vivalchemy/wake/internal/tui/components/search"
	"github.com/vivalchemy/wake/internal/tui/components/sidebar"
	"github.com/vivalchemy/wake/internal/tui/theme"
	"github.com/vivalchemy/wake/pkg/logger"
	"github.com/vivalchemy/wake/pkg/task"
)

// Layout manages the overall TUI layout and component coordination
type Layout struct {
	// Components
	sidebar *sidebar.Sidebar
	output  *output.Output
	search  *search.Search

	// Layout state
	width      int
	height     int
	focused    FocusedComponent
	showSearch bool
	splitMode  SplitMode

	// Layout configuration
	sidebarWidth    int
	minSidebarWidth int
	maxSidebarWidth int
	searchHeight    int
	statusBarHeight int

	// Styles
	styles *Styles
	theme  *theme.Theme

	// Logger
	logger logger.Logger
}

// FocusedComponent represents which component has focus
type FocusedComponent int

const (
	FocusSidebar FocusedComponent = iota
	FocusOutput
	FocusSearch
)

// String returns the string representation of FocusedComponent
func (f FocusedComponent) String() string {
	switch f {
	case FocusSidebar:
		return "sidebar"
	case FocusOutput:
		return "output"
	case FocusSearch:
		return "search"
	default:
		return "sidebar"
	}
}

// SplitMode represents different layout split modes
type SplitMode int

const (
	SplitVertical SplitMode = iota
	SplitHorizontal
	SplitDynamic
)

// String returns the string representation of SplitMode
func (s SplitMode) String() string {
	switch s {
	case SplitVertical:
		return "vertical"
	case SplitHorizontal:
		return "horizontal"
	case SplitDynamic:
		return "dynamic"
	default:
		return "vertical"
	}
}

// Styles defines the visual styles for the layout
type Styles struct {
	Container     lipgloss.Style
	Split         lipgloss.Style
	StatusBar     lipgloss.Style
	StatusText    lipgloss.Style
	StatusKey     lipgloss.Style
	StatusValue   lipgloss.Style
	Divider       lipgloss.Style
	SearchOverlay lipgloss.Style
}

// New creates a new layout manager
func New(logger logger.Logger, themeProvider *theme.Theme) (*Layout, error) {
	if logger == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}
	if themeProvider == nil {
		return nil, fmt.Errorf("theme provider cannot be nil")
	}

	styles := createStyles(themeProvider)

	// Create components
	sidebarComponent := sidebar.New(logger, themeProvider)
	outputComponent := output.New(logger, themeProvider)
	searchComponent := search.New(logger, themeProvider)

	layout := &Layout{
		sidebar:         sidebarComponent,
		output:          outputComponent,
		search:          searchComponent,
		width:           0,
		height:          0,
		focused:         FocusSidebar,
		showSearch:      false,
		splitMode:       SplitVertical,
		sidebarWidth:    40,
		minSidebarWidth: 20,
		maxSidebarWidth: 80,
		searchHeight:    8,
		statusBarHeight: 1,
		styles:          styles,
		theme:           themeProvider,
		logger:          logger.WithGroup("layout"),
	}

	// Set initial focus
	layout.updateFocus()

	return layout, nil
}

// Init initializes the layout
func (l *Layout) Init() tea.Cmd {
	return tea.Batch(
		l.sidebar.Init(),
		l.output.Init(),
		l.search.Init(),
	)
}

// Update handles messages for the layout
func (l *Layout) Update(msg tea.Msg) (*Layout, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		cmd = l.handleKeyPress(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case tea.WindowSizeMsg:
		l.width = msg.Width
		l.height = msg.Height
		l.updateLayout()

	case FocusChangedMsg:
		l.focused = msg.Component
		l.updateFocus()

	case ToggleSearchMsg:
		l.showSearch = !l.showSearch
		l.search.SetVisible(l.showSearch)
		if l.showSearch {
			l.focused = FocusSearch
		} else {
			l.focused = FocusSidebar
		}
		l.updateFocus()
		l.updateLayout()

	case ShowSearchMsg:
		l.showSearch = true
		l.search.SetVisible(true)
		l.focused = FocusSearch
		l.updateFocus()
		l.updateLayout()

	case HideSearchMsg:
		l.showSearch = false
		l.search.SetVisible(false)
		l.focused = FocusSidebar
		l.updateFocus()
		l.updateLayout()

	case SetSplitModeMsg:
		l.splitMode = msg.Mode
		l.updateLayout()

	case ResizeSidebarMsg:
		l.resizeSidebar(msg.Delta)
		l.updateLayout()

	case TaskSelectedMsg:
		// Forward to output component
		l.output, cmd = l.output.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case search.SearchResultsMsg:
		// Forward to sidebar for filtering
		sidebarMsg := sidebar.TasksUpdatedMsg{
			Tasks:     msg.Results,
			Timestamp: msg.Timestamp,
		}
		l.sidebar, cmd = l.sidebar.Update(sidebarMsg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	// Update components
	l.sidebar, cmd = l.sidebar.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	l.output, cmd = l.output.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	l.search, cmd = l.search.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	return l, tea.Batch(cmds...)
}

// View renders the layout
func (l *Layout) View() string {
	if l.width == 0 || l.height == 0 {
		return "Initializing..."
	}

	// Calculate available space
	availableHeight := l.height - l.statusBarHeight
	if l.showSearch {
		availableHeight -= l.searchHeight
	}

	var content string

	// Render main content based on split mode
	switch l.splitMode {
	case SplitVertical:
		content = l.renderVerticalSplit(availableHeight)
	case SplitHorizontal:
		content = l.renderHorizontalSplit(availableHeight)
	case SplitDynamic:
		content = l.renderDynamicSplit(availableHeight)
	default:
		content = l.renderVerticalSplit(availableHeight)
	}

	var sections []string

	// Add search overlay if visible
	if l.showSearch {
		sections = append(sections, l.renderSearchOverlay())
	}

	// Add main content
	sections = append(sections, content)

	// Add status bar
	sections = append(sections, l.renderStatusBar())

	return l.styles.Container.
		Width(l.width).
		Height(l.height).
		Render(strings.Join(sections, "\n"))
}

// renderVerticalSplit renders a vertical split layout
func (l *Layout) renderVerticalSplit(availableHeight int) string {
	// Calculate widths
	sidebarWidth := l.sidebarWidth
	outputWidth := l.width - sidebarWidth - 1 // -1 for divider

	// Ensure minimum widths
	if sidebarWidth < l.minSidebarWidth {
		sidebarWidth = l.minSidebarWidth
		outputWidth = l.width - sidebarWidth - 1
	}

	if outputWidth < 20 {
		outputWidth = 20
		sidebarWidth = l.width - outputWidth - 1
	}

	// Update component sizes
	l.updateComponentSizes(sidebarWidth, availableHeight, outputWidth, availableHeight)

	// Render components
	sidebarView := l.sidebar.View()
	outputView := l.output.View()
	divider := l.renderVerticalDivider(availableHeight)

	// Combine horizontally
	return lipgloss.JoinHorizontal(
		lipgloss.Top,
		sidebarView,
		divider,
		outputView,
	)
}

// renderHorizontalSplit renders a horizontal split layout
func (l *Layout) renderHorizontalSplit(availableHeight int) string {
	// Calculate heights
	sidebarHeight := availableHeight / 2
	outputHeight := availableHeight - sidebarHeight - 1 // -1 for divider

	// Update component sizes
	l.updateComponentSizes(l.width, sidebarHeight, l.width, outputHeight)

	// Render components
	sidebarView := l.sidebar.View()
	outputView := l.output.View()
	divider := l.renderHorizontalDivider()

	// Combine vertically
	return lipgloss.JoinVertical(
		lipgloss.Left,
		sidebarView,
		divider,
		outputView,
	)
}

// renderDynamicSplit renders a dynamic split based on content
func (l *Layout) renderDynamicSplit(availableHeight int) string {
	// Use vertical split for wide screens, horizontal for narrow
	if l.width > 120 {
		return l.renderVerticalSplit(availableHeight)
	} else {
		return l.renderHorizontalSplit(availableHeight)
	}
}

// renderSearchOverlay renders the search component as an overlay
func (l *Layout) renderSearchOverlay() string {
	searchWidth := min(l.width-4, 80)

	// Update search size
	searchMsg := tea.WindowSizeMsg{
		Width:  searchWidth,
		Height: l.searchHeight,
	}
	l.search.Update(searchMsg)

	searchView := l.search.View()

	// Center the search overlay
	return l.styles.SearchOverlay.
		Width(l.width).
		Align(lipgloss.Center).
		Render(searchView)
}

// renderStatusBar renders the status bar
func (l *Layout) renderStatusBar() string {
	var statusItems []string

	// Focus indicator
	focusText := l.styles.StatusKey.Render("Focus: ") +
		l.styles.StatusValue.Render(l.focused.String())
	statusItems = append(statusItems, focusText)

	// Split mode
	splitText := l.styles.StatusKey.Render("Split: ") +
		l.styles.StatusValue.Render(l.splitMode.String())
	statusItems = append(statusItems, splitText)

	// Selected task info
	if selectedTask := l.sidebar.GetSelectedTask(); selectedTask != nil {
		taskText := l.styles.StatusKey.Render("Task: ") +
			l.styles.StatusValue.Render(fmt.Sprintf("%s (%s)", selectedTask.Name, selectedTask.Runner))
		statusItems = append(statusItems, taskText)
	}

	// Search status
	if l.showSearch {
		query := l.search.GetQuery()
		if query != "" {
			searchText := l.styles.StatusKey.Render("Search: ") +
				l.styles.StatusValue.Render(fmt.Sprintf("'%s' (%d results)", query, l.search.GetResultCount()))
			statusItems = append(statusItems, searchText)
		}
	}

	// Help hint
	helpHint := l.styles.StatusKey.Render("Help: ") +
		l.styles.StatusValue.Render("? for help")
	statusItems = append(statusItems, helpHint)

	statusContent := strings.Join(statusItems, " • ")

	// Truncate if too long
	if lipgloss.Width(statusContent) > l.width-4 {
		maxWidth := l.width - 7 // Account for "..."
		statusContent = statusContent[:maxWidth] + "..."
	}

	return l.styles.StatusBar.
		Width(l.width).
		Render(statusContent)
}

// renderVerticalDivider renders a vertical divider
func (l *Layout) renderVerticalDivider(height int) string {
	dividerChar := "│"
	var lines []string

	for range height {
		lines = append(lines, dividerChar)
	}

	return l.styles.Divider.Render(strings.Join(lines, "\n"))
}

// renderHorizontalDivider renders a horizontal divider
func (l *Layout) renderHorizontalDivider() string {
	dividerChar := "─"
	divider := strings.Repeat(dividerChar, l.width)
	return l.styles.Divider.Render(divider)
}

// handleKeyPress handles key press events
func (l *Layout) handleKeyPress(msg tea.KeyMsg) tea.Cmd {
	// Global key bindings
	switch msg.String() {
	case "ctrl+c":
		return tea.Quit

	case "tab":
		return l.cycleFocus()

	case "/", "ctrl+f":
		return l.toggleSearch()

	case "ctrl+s":
		return l.cycleSplitMode()

	case "ctrl+left":
		return l.resizeSidebarCmd(-5)

	case "ctrl+right":
		return l.resizeSidebarCmd(5)

	case "?", "f1":
		return l.showHelp()

	case "esc":
		if l.showSearch {
			return l.hideSearch()
		}
	}

	return nil
}

// cycleFocus cycles through focused components
func (l *Layout) cycleFocus() tea.Cmd {
	if l.showSearch {
		// Cycle between search, sidebar, and output
		switch l.focused {
		case FocusSearch:
			l.focused = FocusSidebar
		case FocusSidebar:
			l.focused = FocusOutput
		case FocusOutput:
			l.focused = FocusSearch
		}
	} else {
		// Cycle between sidebar and output
		switch l.focused {
		case FocusSidebar:
			l.focused = FocusOutput
		case FocusOutput:
			l.focused = FocusSidebar
		default:
			l.focused = FocusSidebar
		}
	}

	l.updateFocus()

	return func() tea.Msg {
		return FocusChangedMsg{Component: l.focused}
	}
}

// toggleSearch toggles the search component
func (l *Layout) toggleSearch() tea.Cmd {
	return func() tea.Msg {
		return ToggleSearchMsg{}
	}
}

// cycleSplitMode cycles through split modes
func (l *Layout) cycleSplitMode() tea.Cmd {
	nextMode := (l.splitMode + 1) % 3 // 3 split modes
	return func() tea.Msg {
		return SetSplitModeMsg{Mode: nextMode}
	}
}

// resizeSidebarCmd creates a resize sidebar command
func (l *Layout) resizeSidebarCmd(delta int) tea.Cmd {
	return func() tea.Msg {
		return ResizeSidebarMsg{Delta: delta}
	}
}

// showHelp shows help information
func (l *Layout) showHelp() tea.Cmd {
	return func() tea.Msg {
		return ShowHelpMsg{}
	}
}

// hideSearch hides the search component
func (l *Layout) hideSearch() tea.Cmd {
	return func() tea.Msg {
		return HideSearchMsg{}
	}
}

// updateFocus updates component focus states
func (l *Layout) updateFocus() {
	l.sidebar.SetFocused(l.focused == FocusSidebar)
	l.output.SetFocused(l.focused == FocusOutput)
	l.search.SetFocused(l.focused == FocusSearch)
}

// updateLayout updates the layout based on current state
func (l *Layout) updateLayout() {
	// This will be called after size changes or mode changes
	// Component sizes are updated in the render methods
}

// updateComponentSizes updates component sizes
func (l *Layout) updateComponentSizes(sidebarWidth, sidebarHeight, outputWidth, outputHeight int) {
	// Update sidebar size
	sidebarSizeMsg := tea.WindowSizeMsg{
		Width:  sidebarWidth,
		Height: sidebarHeight,
	}
	l.sidebar.Update(sidebarSizeMsg)

	// Update output size
	outputSizeMsg := tea.WindowSizeMsg{
		Width:  outputWidth,
		Height: outputHeight,
	}
	l.output.Update(outputSizeMsg)
}

// resizeSidebar resizes the sidebar by delta
func (l *Layout) resizeSidebar(delta int) {
	newWidth := l.sidebarWidth + delta

	if newWidth < l.minSidebarWidth {
		newWidth = l.minSidebarWidth
	} else if newWidth > l.maxSidebarWidth {
		newWidth = l.maxSidebarWidth
	}

	// Ensure there's enough space for output
	minOutputWidth := 30
	if l.width-newWidth-1 < minOutputWidth {
		newWidth = l.width - minOutputWidth - 1
	}

	l.sidebarWidth = newWidth
}

// GetFocusedComponent returns the currently focused component
func (l *Layout) GetFocusedComponent() FocusedComponent {
	return l.focused
}

// SetFocusedComponent sets the focused component
func (l *Layout) SetFocusedComponent(component FocusedComponent) {
	l.focused = component
	l.updateFocus()
}

// GetSplitMode returns the current split mode
func (l *Layout) GetSplitMode() SplitMode {
	return l.splitMode
}

// SetSplitMode sets the split mode
func (l *Layout) SetSplitMode(mode SplitMode) {
	l.splitMode = mode
}

// IsSearchVisible returns whether search is visible
func (l *Layout) IsSearchVisible() bool {
	return l.showSearch
}

// GetSidebar returns the sidebar component
func (l *Layout) GetSidebar() *sidebar.Sidebar {
	return l.sidebar
}

// GetOutput returns the output component
func (l *Layout) GetOutput() *output.Output {
	return l.output
}

// GetSearch returns the search component
func (l *Layout) GetSearch() *search.Search {
	return l.search
}

// createStyles creates the styles for the layout
func createStyles(themeProvider *theme.Theme) *Styles {
	colors := themeProvider.Colors()

	return &Styles{
		Container: lipgloss.NewStyle(),

		Split: lipgloss.NewStyle(),

		StatusBar: lipgloss.NewStyle().
			Background(colors.Background).
			Foreground(colors.Text).
			Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(colors.Border).
			Padding(0, 1),

		StatusText: lipgloss.NewStyle().
			Foreground(colors.Text),

		StatusKey: lipgloss.NewStyle().
			Foreground(colors.Secondary).
			Bold(true),

		StatusValue: lipgloss.NewStyle().
			Foreground(colors.Primary),

		Divider: lipgloss.NewStyle().
			Foreground(colors.Border),

		SearchOverlay: lipgloss.NewStyle().
			Background(colors.Background).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colors.Primary).
			Padding(1).
			Margin(1, 0),
	}
}

// Message types for layout communication

// FocusChangedMsg is sent when focus changes
type FocusChangedMsg struct {
	Component FocusedComponent
}

// ToggleSearchMsg is sent to toggle search visibility
type ToggleSearchMsg struct{}

// ShowSearchMsg is sent to show search
type ShowSearchMsg struct {
	InitialQuery string
}

// HideSearchMsg is sent to hide search
type HideSearchMsg struct{}

// SetSplitModeMsg is sent to change split mode
type SetSplitModeMsg struct {
	Mode SplitMode
}

// ResizeSidebarMsg is sent to resize the sidebar
type ResizeSidebarMsg struct {
	Delta int
}

// ShowHelpMsg is sent to show help
type ShowHelpMsg struct{}

// TaskSelectedMsg is sent when a task is selected
type TaskSelectedMsg struct {
	Task *task.Task
}
