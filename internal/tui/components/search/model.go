package search

import (
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vivalchemy/wake/pkg/task"
	"slices"
)

// Model represents the search component's data model
type Model struct {
	// State tracking
	initialized bool
	lastUpdate  time.Time
	updateCount int64

	// Search state
	searchCount    int64
	lastSearchTime time.Time
	searchHistory  []string
	maxHistorySize int

	// Performance tracking
	renderCount    int64
	lastRenderTime time.Time

	// Results tracking
	lastResultCount  int
	lastQueryTime    time.Duration
	averageQueryTime time.Duration
	totalQueries     int64

	// Suggestion state
	suggestionCache map[string][]string
	cacheTTL        time.Duration
	cacheUpdated    time.Time

	// Thread safety
	mu sync.RWMutex
}

// Messages for search component communication

// ShowSearchMsg is sent to show the search component
type ShowSearchMsg struct {
	InitialQuery string
	Timestamp    time.Time
}

// HideSearchMsg is sent to hide the search component
type HideSearchMsg struct {
	Timestamp time.Time
}

// SearchRequestMsg is sent to request a search
type SearchRequestMsg struct {
	Query         string
	Mode          SearchMode
	Filters       *SearchFilters
	CaseSensitive bool
	UseRegex      bool
	SearchFields  []SearchField
	Timestamp     time.Time
}

// SearchResultsMsg is sent with search results
type SearchResultsMsg struct {
	Results    []*task.Task
	Query      string
	QueryTime  time.Duration
	TotalTasks int
	Timestamp  time.Time
}

// SearchQueryChangedMsg is sent when the search query changes
type SearchQueryChangedMsg struct {
	Query     string
	PrevQuery string
	Timestamp time.Time
}

// SetSearchModeMsg is sent to change the search mode
type SetSearchModeMsg struct {
	Mode      SearchMode
	Timestamp time.Time
}

// SetSearchFiltersMsg is sent to update search filters
type SetSearchFiltersMsg struct {
	Filters   *SearchFilters
	Timestamp time.Time
}

// ToggleSearchOptionMsg is sent to toggle search options
type ToggleSearchOptionMsg struct {
	Option    string
	Timestamp time.Time
}

// SuggestionsUpdatedMsg is sent when search suggestions are updated
type SuggestionsUpdatedMsg struct {
	Suggestions []string
	Query       string
	Timestamp   time.Time
}

// SearchHistoryMsg is sent to request search history
type SearchHistoryMsg struct {
	Timestamp time.Time
}

// SearchStatsMsg is sent to request search statistics
type SearchStatsMsg struct {
	Timestamp time.Time
}

// ClearSearchHistoryMsg is sent to clear search history
type ClearSearchHistoryMsg struct {
	Timestamp time.Time
}

// SearchErrorMsg is sent when a search error occurs
type SearchErrorMsg struct {
	Error     error
	Query     string
	Timestamp time.Time
}

// NewModel creates a new search model
func NewModel() *Model {
	return &Model{
		initialized:      false,
		lastUpdate:       time.Now(),
		updateCount:      0,
		searchCount:      0,
		lastSearchTime:   time.Time{},
		searchHistory:    make([]string, 0),
		maxHistorySize:   50,
		renderCount:      0,
		lastRenderTime:   time.Now(),
		lastResultCount:  0,
		lastQueryTime:    0,
		averageQueryTime: 0,
		totalQueries:     0,
		suggestionCache:  make(map[string][]string),
		cacheTTL:         5 * time.Minute,
		cacheUpdated:     time.Now(),
	}
}

// Init initializes the model
func (m *Model) Init() tea.Cmd {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.initialized = true
	m.lastUpdate = time.Now()
	return nil
}

// Update updates the model with a message
func (m *Model) Update(msg tea.Msg) (*Model, tea.Cmd) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var cmds []tea.Cmd

	m.updateCount++
	m.lastUpdate = time.Now()

	switch msg := msg.(type) {
	case SearchRequestMsg:
		cmds = append(cmds, m.handleSearchRequest(msg))

	case SearchResultsMsg:
		cmds = append(cmds, m.handleSearchResults(msg))

	case SearchQueryChangedMsg:
		cmds = append(cmds, m.handleQueryChanged(msg))

	case SuggestionsUpdatedMsg:
		cmds = append(cmds, m.handleSuggestionsUpdated(msg))

	case SearchHistoryMsg:
		cmds = append(cmds, m.handleSearchHistory(msg))

	case SearchStatsMsg:
		cmds = append(cmds, m.handleSearchStats(msg))

	case ClearSearchHistoryMsg:
		cmds = append(cmds, m.handleClearHistory(msg))

	case SearchErrorMsg:
		cmds = append(cmds, m.handleSearchError(msg))
	}

	return m, tea.Batch(cmds...)
}

// handleSearchRequest handles search requests
func (m *Model) handleSearchRequest(msg SearchRequestMsg) tea.Cmd {
	m.searchCount++
	m.lastSearchTime = msg.Timestamp

	// Add to search history if not empty and not duplicate
	if msg.Query != "" && (len(m.searchHistory) == 0 || m.searchHistory[len(m.searchHistory)-1] != msg.Query) {
		m.addToHistory(msg.Query)
	}

	// Start timing the query
	startTime := time.Now()

	return func() tea.Msg {
		// This would trigger the actual search in the parent component
		// For now, we just return a placeholder command
		return SearchProcessingMsg{
			Query:     msg.Query,
			StartTime: startTime,
			Timestamp: time.Now(),
		}
	}
}

// handleSearchResults handles search results
func (m *Model) handleSearchResults(msg SearchResultsMsg) tea.Cmd {
	m.lastResultCount = len(msg.Results)
	m.lastQueryTime = msg.QueryTime
	m.totalQueries++

	// Update average query time
	if m.totalQueries == 1 {
		m.averageQueryTime = msg.QueryTime
	} else {
		// Running average
		m.averageQueryTime = (m.averageQueryTime*time.Duration(m.totalQueries-1) + msg.QueryTime) / time.Duration(m.totalQueries)
	}

	return nil
}

// handleQueryChanged handles query change events
func (m *Model) handleQueryChanged(msg SearchQueryChangedMsg) tea.Cmd {
	// Generate suggestions based on the new query
	suggestions := m.generateSuggestions(msg.Query)

	if len(suggestions) > 0 {
		return func() tea.Msg {
			return SuggestionsUpdatedMsg{
				Suggestions: suggestions,
				Query:       msg.Query,
				Timestamp:   time.Now(),
			}
		}
	}

	return nil
}

// handleSuggestionsUpdated handles suggestion updates
func (m *Model) handleSuggestionsUpdated(msg SuggestionsUpdatedMsg) tea.Cmd {
	// Cache suggestions
	m.suggestionCache[msg.Query] = msg.Suggestions
	m.cacheUpdated = msg.Timestamp

	return nil
}

// handleSearchHistory handles search history requests
func (m *Model) handleSearchHistory(_ SearchHistoryMsg) tea.Cmd {
	history := make([]string, len(m.searchHistory))
	copy(history, m.searchHistory)

	return func() tea.Msg {
		return SearchHistoryResponseMsg{
			History:   history,
			Timestamp: time.Now(),
		}
	}
}

// handleSearchStats handles search statistics requests
func (m *Model) handleSearchStats(_ SearchStatsMsg) tea.Cmd {
	stats := m.getStats()

	return func() tea.Msg {
		return SearchStatsResponseMsg{
			Stats:     stats,
			Timestamp: time.Now(),
		}
	}
}

// handleClearHistory handles history clearing
func (m *Model) handleClearHistory(_ ClearSearchHistoryMsg) tea.Cmd {
	m.searchHistory = m.searchHistory[:0]

	return func() tea.Msg {
		return SearchHistoryClearedMsg{
			Timestamp: time.Now(),
		}
	}
}

// handleSearchError handles search errors
func (m *Model) handleSearchError(_ SearchErrorMsg) tea.Cmd {
	// Log error statistics or handle error state
	return nil
}

// addToHistory adds a query to the search history
func (m *Model) addToHistory(query string) {
	// Remove duplicates
	for i, existing := range m.searchHistory {
		if existing == query {
			// Move to end
			m.searchHistory = slices.Delete(m.searchHistory, i, i+1)
			break
		}
	}

	// Add to end
	m.searchHistory = append(m.searchHistory, query)

	// Trim if too long
	if len(m.searchHistory) > m.maxHistorySize {
		m.searchHistory = m.searchHistory[1:]
	}
}

// generateSuggestions generates search suggestions based on query
func (m *Model) generateSuggestions(query string) []string {
	if query == "" {
		return []string{}
	}

	// Check cache first
	if suggestions, exists := m.suggestionCache[query]; exists && time.Since(m.cacheUpdated) < m.cacheTTL {
		return suggestions
	}

	var suggestions []string
	queryLower := strings.ToLower(query)

	// Generate suggestions from history
	for _, historyItem := range m.searchHistory {
		if strings.Contains(strings.ToLower(historyItem), queryLower) && historyItem != query {
			suggestions = append(suggestions, historyItem)
		}
	}

	// Add common search patterns
	commonPatterns := []string{
		"runner:",
		"status:",
		"tags:",
		"hidden:",
		"name:",
		"description:",
	}

	for _, pattern := range commonPatterns {
		if strings.HasPrefix(queryLower, pattern[:len(pattern)-1]) {
			// Add pattern completions
			switch pattern {
			case "runner:":
				suggestions = append(suggestions, "runner:make", "runner:npm", "runner:yarn", "runner:pnpm", "runner:bun", "runner:just", "runner:task", "runner:python")
			case "status:":
				suggestions = append(suggestions, "status:idle", "status:running", "status:success", "status:failed", "status:stopped")
			case "hidden:":
				suggestions = append(suggestions, "hidden:true", "hidden:false")
			}
		}
	}

	// Remove duplicates and limit
	suggestions = m.uniqueStrings(suggestions)
	if len(suggestions) > 10 {
		suggestions = suggestions[:10]
	}

	return suggestions
}

// uniqueStrings removes duplicate strings from a slice
func (m *Model) uniqueStrings(strings []string) []string {
	keys := make(map[string]bool)
	var result []string

	for _, str := range strings {
		if !keys[str] {
			keys[str] = true
			result = append(result, str)
		}
	}

	return result
}

// getStats returns model statistics
func (m *Model) getStats() SearchStats {
	now := time.Now()
	timeSinceInit := now.Sub(m.lastUpdate)

	var searchesPerSecond float64
	if timeSinceInit.Seconds() > 0 {
		searchesPerSecond = float64(m.searchCount) / timeSinceInit.Seconds()
	}

	return SearchStats{
		UpdateCount:       m.updateCount,
		RenderCount:       m.renderCount,
		SearchCount:       m.searchCount,
		TotalQueries:      m.totalQueries,
		LastUpdate:        m.lastUpdate,
		LastRender:        m.lastRenderTime,
		LastSearchTime:    m.lastSearchTime,
		LastResultCount:   m.lastResultCount,
		LastQueryTime:     m.lastQueryTime,
		AverageQueryTime:  m.averageQueryTime,
		SearchesPerSecond: searchesPerSecond,
		HistorySize:       len(m.searchHistory),
		MaxHistorySize:    m.maxHistorySize,
		CacheSize:         len(m.suggestionCache),
		CacheUpdated:      m.cacheUpdated,
		Initialized:       m.initialized,
	}
}

// Additional message types

// SearchProcessingMsg is sent when search processing starts
type SearchProcessingMsg struct {
	Query     string
	StartTime time.Time
	Timestamp time.Time
}

// SearchHistoryResponseMsg contains search history
type SearchHistoryResponseMsg struct {
	History   []string
	Timestamp time.Time
}

// SearchStatsResponseMsg contains search statistics
type SearchStatsResponseMsg struct {
	Stats     SearchStats
	Timestamp time.Time
}

// SearchHistoryClearedMsg is sent when history is cleared
type SearchHistoryClearedMsg struct {
	Timestamp time.Time
}

// SearchStats provides statistics about the search model
type SearchStats struct {
	UpdateCount       int64         `json:"update_count"`
	RenderCount       int64         `json:"render_count"`
	SearchCount       int64         `json:"search_count"`
	TotalQueries      int64         `json:"total_queries"`
	LastUpdate        time.Time     `json:"last_update"`
	LastRender        time.Time     `json:"last_render"`
	LastSearchTime    time.Time     `json:"last_search_time"`
	LastResultCount   int           `json:"last_result_count"`
	LastQueryTime     time.Duration `json:"last_query_time"`
	AverageQueryTime  time.Duration `json:"average_query_time"`
	SearchesPerSecond float64       `json:"searches_per_second"`
	HistorySize       int           `json:"history_size"`
	MaxHistorySize    int           `json:"max_history_size"`
	CacheSize         int           `json:"cache_size"`
	CacheUpdated      time.Time     `json:"cache_updated"`
	Initialized       bool          `json:"initialized"`
}

// Public methods for accessing model state

// GetStats returns model statistics
func (m *Model) GetStats() SearchStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.getStats()
}

// IncrementSearchCount increments the search counter
func (m *Model) IncrementSearchCount() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.searchCount++
	m.lastSearchTime = time.Now()
}

// IncrementRenderCount increments the render counter
func (m *Model) IncrementRenderCount() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.renderCount++
	m.lastRenderTime = time.Now()
}

// GetSearchHistory returns the search history
func (m *Model) GetSearchHistory() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	history := make([]string, len(m.searchHistory))
	copy(history, m.searchHistory)
	return history
}

// AddToSearchHistory adds a query to the search history
func (m *Model) AddToSearchHistory(query string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.addToHistory(query)
}

// ClearSearchHistory clears the search history
func (m *Model) ClearSearchHistory() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.searchHistory = m.searchHistory[:0]
}

// GetSuggestions returns cached suggestions for a query
func (m *Model) GetSuggestions(query string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if suggestions, exists := m.suggestionCache[query]; exists && time.Since(m.cacheUpdated) < m.cacheTTL {
		result := make([]string, len(suggestions))
		copy(result, suggestions)
		return result
	}

	return []string{}
}

// GenerateSuggestions generates suggestions for a query
func (m *Model) GenerateSuggestions(query string) []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.generateSuggestions(query)
}

// GetSearchCount returns the total search count
func (m *Model) GetSearchCount() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.searchCount
}

// GetLastSearchTime returns the time of the last search
func (m *Model) GetLastSearchTime() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.lastSearchTime
}

// GetAverageQueryTime returns the average query execution time
func (m *Model) GetAverageQueryTime() time.Duration {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.averageQueryTime
}

// GetLastResultCount returns the number of results from the last search
func (m *Model) GetLastResultCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.lastResultCount
}

// IsInitialized returns whether the model is initialized
func (m *Model) IsInitialized() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.initialized
}

// SetMaxHistorySize sets the maximum history size
func (m *Model) SetMaxHistorySize(size int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.maxHistorySize = size

	// Trim history if necessary
	if len(m.searchHistory) > size {
		start := len(m.searchHistory) - size
		m.searchHistory = m.searchHistory[start:]
	}
}

// GetMaxHistorySize returns the maximum history size
func (m *Model) GetMaxHistorySize() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.maxHistorySize
}

// ClearCache clears the suggestion cache
func (m *Model) ClearCache() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.suggestionCache = make(map[string][]string)
	m.cacheUpdated = time.Now()
}

// GetCacheSize returns the current cache size
func (m *Model) GetCacheSize() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.suggestionCache)
}

// Reset resets the model to its initial state
func (m *Model) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.searchCount = 0
	m.lastSearchTime = time.Time{}
	m.searchHistory = m.searchHistory[:0]
	m.renderCount = 0
	m.lastRenderTime = time.Now()
	m.lastResultCount = 0
	m.lastQueryTime = 0
	m.averageQueryTime = 0
	m.totalQueries = 0
	m.suggestionCache = make(map[string][]string)
	m.cacheUpdated = time.Now()
	m.updateCount = 0
	m.lastUpdate = time.Now()
}
