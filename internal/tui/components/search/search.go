package search

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/vivalchemy/wake/internal/tui/theme"
	"github.com/vivalchemy/wake/pkg/logger"
	"github.com/vivalchemy/wake/pkg/task"
)

// Search represents the search/filter component
type Search struct {
	// Model state
	model     *Model
	textInput textinput.Model
	width     int
	height    int
	focused   bool
	visible   bool

	// Search state
	query       string
	mode        SearchMode
	filters     *SearchFilters
	suggestions []string

	// Search options
	caseSensitive bool
	useRegex      bool
	searchFields  []SearchField

	// Results
	lastResults []*task.Task
	resultCount int

	// UI state
	showHelp        bool
	showFilters     bool
	showSuggestions bool

	// Styles
	styles *Styles

	// Logger
	logger logger.Logger
}

// SearchMode represents different search modes
type SearchMode int

const (
	SearchModeText SearchMode = iota
	SearchModeRegex
	SearchModeFilter
	SearchModeAdvanced
)

// String returns the string representation of SearchMode
func (m SearchMode) String() string {
	switch m {
	case SearchModeText:
		return "text"
	case SearchModeRegex:
		return "regex"
	case SearchModeFilter:
		return "filter"
	case SearchModeAdvanced:
		return "advanced"
	default:
		return "text"
	}
}

// SearchField represents different fields to search in
type SearchField int

const (
	SearchFieldName SearchField = iota
	SearchFieldDescription
	SearchFieldRunner
	SearchFieldTags
	SearchFieldFile
	SearchFieldCommand
)

// String returns the string representation of SearchField
func (f SearchField) String() string {
	switch f {
	case SearchFieldName:
		return "name"
	case SearchFieldDescription:
		return "description"
	case SearchFieldRunner:
		return "runner"
	case SearchFieldTags:
		return "tags"
	case SearchFieldFile:
		return "file"
	case SearchFieldCommand:
		return "command"
	default:
		return "name"
	}
}

// SearchFilters represents active search filters
type SearchFilters struct {
	Runner     string
	Status     string
	Tags       []string
	Hidden     *bool // nil = all, true = hidden only, false = visible only
	HasCommand bool
	HasDesc    bool
	MinTags    int
	MaxTags    int
}

// Styles defines the visual styles for the search component
type Styles struct {
	Container          lipgloss.Style
	Input              lipgloss.Style
	InputFocused       lipgloss.Style
	InputPrompt        lipgloss.Style
	ModeIndicator      lipgloss.Style
	FilterBar          lipgloss.Style
	FilterTag          lipgloss.Style
	FilterActive       lipgloss.Style
	ResultCount        lipgloss.Style
	Suggestions        lipgloss.Style
	Suggestion         lipgloss.Style
	SuggestionSelected lipgloss.Style
	Help               lipgloss.Style
	NoResults          lipgloss.Style
}

// New creates a new search component
func New(logger logger.Logger, themeProvider *theme.Theme) *Search {
	styles := createStyles(themeProvider)

	// Create text input
	ti := textinput.New()
	ti.Placeholder = "Search tasks..."
	ti.Focus()
	ti.CharLimit = 256
	ti.Width = 50

	// Create model
	model := NewModel()

	search := &Search{
		model:           model,
		textInput:       ti,
		query:           "",
		mode:            SearchModeText,
		filters:         &SearchFilters{},
		suggestions:     []string{},
		caseSensitive:   false,
		useRegex:        false,
		searchFields:    []SearchField{SearchFieldName, SearchFieldDescription},
		lastResults:     []*task.Task{},
		resultCount:     0,
		showHelp:        false,
		showFilters:     false,
		showSuggestions: true,
		styles:          styles,
		logger:          logger.WithGroup("search"),
		visible:         false,
	}

	return search
}

func createStyles(themeProvider *theme.Theme) *Styles {
	colors := themeProvider.Colors()

	return &Styles{
		Container: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colors.Border).
			Padding(1),

		Input: lipgloss.NewStyle().
			Foreground(colors.Text),

		InputFocused: lipgloss.NewStyle().
			Foreground(colors.TextInverted).
			Background(colors.Primary),

		InputPrompt: lipgloss.NewStyle().
			Foreground(colors.Muted),

		ModeIndicator: lipgloss.NewStyle().
			Foreground(colors.Muted),

		FilterBar: lipgloss.NewStyle().
			Foreground(colors.Muted),

		FilterTag: lipgloss.NewStyle().
			Foreground(colors.Muted),

		FilterActive: lipgloss.NewStyle().
			Foreground(colors.Text),

		ResultCount: lipgloss.NewStyle().
			Foreground(colors.Muted),

		Suggestions: lipgloss.NewStyle().
			Foreground(colors.Muted),

		Suggestion: lipgloss.NewStyle().
			Foreground(colors.Text),

		SuggestionSelected: lipgloss.NewStyle().
			Foreground(colors.TextInverted).
			Background(colors.Primary),

		Help: lipgloss.NewStyle().
			Foreground(colors.Muted),

		NoResults: lipgloss.NewStyle().
			Foreground(colors.Muted).
			Italic(true).
			Align(lipgloss.Center),
	}
}

// Init initializes the search component
func (s *Search) Init() tea.Cmd {
	return tea.Batch(
		s.model.Init(),
		textinput.Blink,
	)
}

// Update handles messages for the search component
func (s *Search) Update(msg tea.Msg) (*Search, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	// Update model
	s.model, cmd = s.model.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if s.focused {
			cmd = s.handleKeyPress(msg)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

	case tea.WindowSizeMsg:
		s.width = msg.Width
		s.height = msg.Height
		s.updateSize()

	case ShowSearchMsg:
		s.visible = true
		s.focused = true
		s.textInput.Focus()

	case HideSearchMsg:
		s.visible = false
		s.focused = false
		s.textInput.Blur()

	case SearchQueryChangedMsg:
		if msg.Query != s.query {
			s.query = msg.Query
			s.textInput.SetValue(s.query)
			cmd = s.performSearch()
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
		}

	case SetSearchModeMsg:
		s.mode = msg.Mode
		s.updateInputPlaceholder()
		cmd = s.performSearch()
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case SetSearchFiltersMsg:
		s.filters = msg.Filters
		cmd = s.performSearch()
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case ToggleSearchOptionMsg:
		s.handleToggleOption(msg.Option)
		cmd = s.performSearch()
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

	case SuggestionsUpdatedMsg:
		s.suggestions = msg.Suggestions

	case SearchResultsMsg:
		s.lastResults = msg.Results
		s.resultCount = len(msg.Results)
	}

	// Update text input
	s.textInput, cmd = s.textInput.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	// Check if query changed
	newQuery := s.textInput.Value()
	if newQuery != s.query {
		s.query = newQuery
		cmd = s.performSearch()
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return s, tea.Batch(cmds...)
}

func (s *Search) GetQuery() string {
	return s.query
}

func (s *Search) GetResultCount() int {
	return len(s.lastResults)
}

// View renders the search component
func (s *Search) View() string {
	if !s.visible || s.width == 0 || s.height == 0 {
		return ""
	}

	content := s.renderContent()

	return s.styles.Container.
		Width(s.width).
		Height(s.height).
		Render(content)
}

// renderContent renders the main content
func (s *Search) renderContent() string {
	var sections []string

	// Input field with mode indicator
	sections = append(sections, s.renderInput())

	// Filter bar (if filters are active or shown)
	if s.showFilters || s.hasActiveFilters() {
		sections = append(sections, s.renderFilterBar())
	}

	// Result count
	sections = append(sections, s.renderResultCount())

	// Suggestions (if enabled and available)
	if s.showSuggestions && len(s.suggestions) > 0 && s.focused {
		sections = append(sections, s.renderSuggestions())
	}

	// Help (if shown)
	if s.showHelp {
		sections = append(sections, s.renderHelp())
	}

	return strings.Join(sections, "\n")
}

// renderInput renders the input field with mode indicator
func (s *Search) renderInput() string {
	var inputStyle lipgloss.Style

	if s.focused {
		inputStyle = s.styles.InputFocused
	} else {
		inputStyle = s.styles.Input
	}

	// Mode indicator
	modeIndicator := s.styles.ModeIndicator.Render(s.getModeSymbol())

	// Input field
	input := inputStyle.Render(s.textInput.View())

	// Combine
	return lipgloss.JoinHorizontal(lipgloss.Center, modeIndicator, " ", input)
}

// renderFilterBar renders active filters
func (s *Search) renderFilterBar() string {
	if !s.hasActiveFilters() && !s.showFilters {
		return ""
	}

	var filterTags []string

	// Runner filter
	if s.filters.Runner != "" {
		tag := s.styles.FilterActive.Render(fmt.Sprintf("runner:%s", s.filters.Runner))
		filterTags = append(filterTags, tag)
	}

	// Status filter
	if s.filters.Status != "" {
		tag := s.styles.FilterActive.Render(fmt.Sprintf("status:%s", s.filters.Status))
		filterTags = append(filterTags, tag)
	}

	// Tags filter
	if len(s.filters.Tags) > 0 {
		tag := s.styles.FilterActive.Render(fmt.Sprintf("tags:%s", strings.Join(s.filters.Tags, ",")))
		filterTags = append(filterTags, tag)
	}

	// Hidden filter
	if s.filters.Hidden != nil {
		hiddenText := "visible"
		if *s.filters.Hidden {
			hiddenText = "hidden"
		}
		tag := s.styles.FilterActive.Render(fmt.Sprintf("show:%s", hiddenText))
		filterTags = append(filterTags, tag)
	}

	// Search options
	if s.caseSensitive {
		tag := s.styles.FilterTag.Render("case-sensitive")
		filterTags = append(filterTags, tag)
	}

	if s.useRegex {
		tag := s.styles.FilterTag.Render("regex")
		filterTags = append(filterTags, tag)
	}

	// Search fields
	if len(s.searchFields) != 2 { // Not default (name + description)
		var fields []string
		for _, field := range s.searchFields {
			fields = append(fields, field.String())
		}
		tag := s.styles.FilterTag.Render(fmt.Sprintf("fields:%s", strings.Join(fields, ",")))
		filterTags = append(filterTags, tag)
	}

	if len(filterTags) == 0 {
		return ""
	}

	filterContent := strings.Join(filterTags, " ")
	return s.styles.FilterBar.Render(filterContent)
}

// renderResultCount renders the result count
func (s *Search) renderResultCount() string {
	if s.query == "" && !s.hasActiveFilters() {
		return ""
	}

	var text string
	if s.resultCount == 0 {
		text = "No results found"
	} else if s.resultCount == 1 {
		text = "1 result found"
	} else {
		text = fmt.Sprintf("%d results found", s.resultCount)
	}

	return s.styles.ResultCount.Render(text)
}

// renderSuggestions renders search suggestions
func (s *Search) renderSuggestions() string {
	if len(s.suggestions) == 0 {
		return ""
	}

	var suggestionItems []string
	maxSuggestions := 5 // Limit number of suggestions shown

	for i, suggestion := range s.suggestions {
		if i >= maxSuggestions {
			break
		}

		// Style suggestion
		suggestionItem := s.styles.Suggestion.Render(suggestion)
		suggestionItems = append(suggestionItems, suggestionItem)
	}

	suggestionsContent := strings.Join(suggestionItems, "  ")
	return s.styles.Suggestions.Render("Suggestions: " + suggestionsContent)
}

// renderHelp renders help text
func (s *Search) renderHelp() string {
	var helpLines []string

	helpLines = append(helpLines, "Search Help:")
	helpLines = append(helpLines, "  Enter: Perform search")
	helpLines = append(helpLines, "  Ctrl+R: Toggle regex mode")
	helpLines = append(helpLines, "  Ctrl+F: Toggle filters")
	helpLines = append(helpLines, "  Ctrl+C: Toggle case sensitivity")
	helpLines = append(helpLines, "  Tab: Cycle search mode")
	helpLines = append(helpLines, "  Esc: Close search")

	if s.mode == SearchModeFilter {
		helpLines = append(helpLines, "")
		helpLines = append(helpLines, "Filter syntax:")
		helpLines = append(helpLines, "  runner:make       - Filter by runner")
		helpLines = append(helpLines, "  status:running    - Filter by status")
		helpLines = append(helpLines, "  tags:build,test   - Filter by tags")
		helpLines = append(helpLines, "  hidden:true       - Show hidden tasks")
	}

	helpContent := strings.Join(helpLines, "\n")
	return s.styles.Help.Render(helpContent)
}

// handleKeyPress handles key press events
func (s *Search) handleKeyPress(msg tea.KeyMsg) tea.Cmd {
	switch {
	case key.Matches(msg, key.NewBinding(key.WithKeys("enter"))):
		return s.performSearch()

	case key.Matches(msg, key.NewBinding(key.WithKeys("esc"))):
		return s.hide()

	case key.Matches(msg, key.NewBinding(key.WithKeys("tab"))):
		return s.cycleMode()

	case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+r"))):
		return s.toggleRegex()

	case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+c"))):
		return s.toggleCaseSensitive()

	case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+f"))):
		return s.toggleFilters()

	case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+h", "f1"))):
		return s.toggleHelp()

	case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+s"))):
		return s.toggleSuggestions()

	case key.Matches(msg, key.NewBinding(key.WithKeys("ctrl+l"))):
		return s.clearSearch()
	}

	return nil
}

// performSearch performs the search with current query and filters
func (s *Search) performSearch() tea.Cmd {
	s.model.IncrementSearchCount()

	return func() tea.Msg {
		return SearchRequestMsg{
			Query:         s.query,
			Mode:          s.mode,
			Filters:       s.filters,
			CaseSensitive: s.caseSensitive,
			UseRegex:      s.useRegex,
			SearchFields:  s.searchFields,
			Timestamp:     time.Now(),
		}
	}
}

// hide hides the search component
func (s *Search) hide() tea.Cmd {
	return func() tea.Msg {
		return HideSearchMsg{Timestamp: time.Now()}
	}
}

// cycleMode cycles through search modes
func (s *Search) cycleMode() tea.Cmd {
	nextMode := (s.mode + 1) % 4 // 4 search modes
	return func() tea.Msg {
		return SetSearchModeMsg{
			Mode:      nextMode,
			Timestamp: time.Now(),
		}
	}
}

// toggleRegex toggles regex mode
func (s *Search) toggleRegex() tea.Cmd {
	return func() tea.Msg {
		return ToggleSearchOptionMsg{
			Option:    "regex",
			Timestamp: time.Now(),
		}
	}
}

// toggleCaseSensitive toggles case sensitivity
func (s *Search) toggleCaseSensitive() tea.Cmd {
	return func() tea.Msg {
		return ToggleSearchOptionMsg{
			Option:    "case-sensitive",
			Timestamp: time.Now(),
		}
	}
}

// toggleFilters toggles filter display
func (s *Search) toggleFilters() tea.Cmd {
	s.showFilters = !s.showFilters
	return nil
}

// toggleHelp toggles help display
func (s *Search) toggleHelp() tea.Cmd {
	s.showHelp = !s.showHelp
	return nil
}

// toggleSuggestions toggles suggestion display
func (s *Search) toggleSuggestions() tea.Cmd {
	s.showSuggestions = !s.showSuggestions
	return nil
}

// clearSearch clears the search query
func (s *Search) clearSearch() tea.Cmd {
	s.query = ""
	s.textInput.SetValue("")
	return s.performSearch()
}

// handleToggleOption handles search option toggles
func (s *Search) handleToggleOption(option string) {
	switch option {
	case "regex":
		s.useRegex = !s.useRegex
		s.updateInputPlaceholder()

	case "case-sensitive":
		s.caseSensitive = !s.caseSensitive

	case "suggestions":
		s.showSuggestions = !s.showSuggestions

	case "filters":
		s.showFilters = !s.showFilters

	case "help":
		s.showHelp = !s.showHelp
	}
}

// getModeSymbol returns a symbol for the current search mode
func (s *Search) getModeSymbol() string {
	switch s.mode {
	case SearchModeText:
		return "🔍"
	case SearchModeRegex:
		return "🔧"
	case SearchModeFilter:
		return "🎯"
	case SearchModeAdvanced:
		return "⚙️"
	default:
		return "🔍"
	}
}

// updateInputPlaceholder updates the input placeholder based on mode
func (s *Search) updateInputPlaceholder() {
	switch s.mode {
	case SearchModeText:
		s.textInput.Placeholder = "Search tasks..."
	case SearchModeRegex:
		s.textInput.Placeholder = "Regex pattern..."
	case SearchModeFilter:
		s.textInput.Placeholder = "runner:make status:running tags:build..."
	case SearchModeAdvanced:
		s.textInput.Placeholder = "Advanced search..."
	}
}

// hasActiveFilters returns true if any filters are active
func (s *Search) hasActiveFilters() bool {
	return s.filters.Runner != "" ||
		s.filters.Status != "" ||
		len(s.filters.Tags) > 0 ||
		s.filters.Hidden != nil ||
		s.filters.HasCommand ||
		s.filters.HasDesc ||
		s.filters.MinTags > 0 ||
		s.filters.MaxTags > 0
}

// updateSize updates the component size
func (s *Search) updateSize() {
	// Update text input width based on available space
	availableWidth := s.width - 10 // Account for margins and mode indicator
	if availableWidth > 20 {
		s.textInput.Width = availableWidth
	}
}

// SetVisible sets the visibility state
func (s *Search) SetVisible(visible bool) {
	s.visible = visible
	if visible {
		s.textInput.Focus()
		s.focused = true
	} else {
		s.textInput.Blur()
		s.focused = false
	}
}

// IsVisible returns the visibility state
func (s *Search) IsVisible() bool {
	return s.visible
}

// SetFocused sets the focus state
func (s *Search) SetFocused(focused bool) {
	s.focused = focused
	if focused {
		s.textInput.Focus()
	} else {
		s.textInput.Blur()
	}
}

// IsFocused returns the focus state
func (s *Search) IsFocused() bool {
	return s.focused
}

// GetQuery
