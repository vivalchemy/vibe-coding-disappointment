package output

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/vivalchemy/wake/pkg/logger"
	"slices"
)

// History manages the history of task execution output
type History struct {
	// Configuration
	maxEntries  int
	persistFile string
	autoSave    bool
	compression bool

	// Data storage
	entries    []*HistoryEntry
	entriesMap map[string]*HistoryEntry
	sessions   map[string]*HistorySession

	// State management
	started        bool
	currentSession string

	// Statistics
	stats *HistoryStats

	// Background operations
	saveTimer    *time.Timer
	saveInterval time.Duration

	// Synchronization
	mu     sync.RWMutex
	saveMu sync.Mutex

	// Logger
	logger logger.Logger
}

// HistoryEntry represents a single history entry
type HistoryEntry struct {
	// Identification
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	TaskID    string `json:"task_id"`
	TaskName  string `json:"task_name"`

	// Timing
	Timestamp time.Time     `json:"timestamp"`
	Duration  time.Duration `json:"duration,omitempty"`

	// Content
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
	Level  string `json:"level"`

	// Metadata
	Runner   string         `json:"runner,omitempty"`
	ExitCode int            `json:"exit_code,omitempty"`
	Tags     []string       `json:"tags,omitempty"`
	Metadata map[string]any `json:"metadata,omitempty"`

	// Size tracking
	Size int64 `json:"size"`
}

// HistorySession represents a session of task executions
type HistorySession struct {
	// Identification
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`

	// Timing
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`

	// Statistics
	TaskCount    int `json:"task_count"`
	SuccessCount int `json:"success_count"`
	FailureCount int `json:"failure_count"`

	// Metadata
	Environment map[string]string `json:"environment,omitempty"`
	Config      map[string]any    `json:"config,omitempty"`

	// Entry references
	EntryIDs []string `json:"entry_ids"`
}

// HistoryStats tracks history usage statistics
type HistoryStats struct {
	// Counts
	TotalEntries   int64 `json:"total_entries"`
	TotalSessions  int64 `json:"total_sessions"`
	CurrentEntries int   `json:"current_entries"`

	// Size tracking
	TotalSize    int64 `json:"total_size"`
	AverageSize  int64 `json:"average_size"`
	LargestEntry int64 `json:"largest_entry"`

	// Operations
	AddOperations  int64 `json:"add_operations"`
	SaveOperations int64 `json:"save_operations"`
	LoadOperations int64 `json:"load_operations"`

	// Timing
	LastSave    time.Time `json:"last_save"`
	LastLoad    time.Time `json:"last_load"`
	OldestEntry time.Time `json:"oldest_entry"`
	NewestEntry time.Time `json:"newest_entry"`
}

// HistoryQuery represents a query for history entries
type HistoryQuery struct {
	// Filters
	TaskID    string
	TaskName  string
	SessionID string
	Runner    string
	Level     string
	Tags      []string

	// Time range
	StartTime *time.Time
	EndTime   *time.Time

	// Content search
	ContentSearch string
	ErrorSearch   string

	// Sorting and pagination
	SortBy    string // timestamp, duration, size, task_name
	SortOrder string // asc, desc
	Limit     int
	Offset    int

	// Result options
	IncludeOutput   bool
	IncludeMetadata bool
}

// HistoryResult represents query results
type HistoryResult struct {
	Entries  []*HistoryEntry `json:"entries"`
	Total    int             `json:"total"`
	Filtered int             `json:"filtered"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
	Query    *HistoryQuery   `json:"query,omitempty"`
}

// NewHistory creates a new history manager
func NewHistory(maxEntries int, logger logger.Logger) *History {
	return &History{
		maxEntries:   maxEntries,
		autoSave:     true,
		compression:  true,
		entries:      make([]*HistoryEntry, 0, maxEntries),
		entriesMap:   make(map[string]*HistoryEntry),
		sessions:     make(map[string]*HistorySession),
		stats:        &HistoryStats{},
		saveInterval: 5 * time.Minute,
		logger:       logger.WithGroup("history"),
	}
}

// Start starts the history manager
func (h *History) Start(persistFile string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.started {
		return fmt.Errorf("history already started")
	}

	h.persistFile = persistFile
	h.currentSession = h.generateSessionID()

	// Create new session
	session := &HistorySession{
		ID:        h.currentSession,
		StartTime: time.Now(),
		EntryIDs:  []string{},
	}
	h.sessions[h.currentSession] = session

	// Load existing history if file exists
	if persistFile != "" && h.fileExists(persistFile) {
		if err := h.loadFromFile(); err != nil {
			h.logger.Warn("Failed to load history", "file", persistFile, "error", err)
		}
	}

	// Start auto-save timer
	if h.autoSave && h.saveInterval > 0 {
		h.saveTimer = time.NewTimer(h.saveInterval)
		go h.autoSaveRoutine()
	}

	h.started = true
	h.logger.Debug("History started", "file", persistFile, "session", h.currentSession)

	return nil
}

// Stop stops the history manager
func (h *History) Stop() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.started {
		return nil
	}

	h.logger.Debug("Stopping history")

	// Stop auto-save timer
	if h.saveTimer != nil {
		h.saveTimer.Stop()
	}

	// End current session
	if session, exists := h.sessions[h.currentSession]; exists {
		session.EndTime = time.Now()
	}

	// Final save
	if h.persistFile != "" {
		if err := h.saveToFile(); err != nil {
			h.logger.Warn("Failed to save history on stop", "error", err)
		}
	}

	h.started = false
	h.logger.Debug("History stopped")

	return nil
}

// Add adds a new entry to the history
func (h *History) Add(entry *OutputEntry) {
	if !h.started {
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Create history entry
	historyEntry := &HistoryEntry{
		ID:        h.generateEntryID(),
		SessionID: h.currentSession,
		TaskID:    entry.TaskID,
		TaskName:  entry.TaskName,
		Timestamp: entry.Timestamp,
		Output:    entry.Message,
		Level:     entry.Level,
		Size:      int64(len(entry.Message)),
		Metadata:  entry.Data,
	}

	// Set error field if this is an error entry
	if entry.IsStderr || entry.Level == "error" {
		historyEntry.Error = entry.Message
	}

	// Add to collections
	h.addEntry(historyEntry)

	// Update session
	if session, exists := h.sessions[h.currentSession]; exists {
		session.EntryIDs = append(session.EntryIDs, historyEntry.ID)
		session.TaskCount++

		if entry.Level == "error" {
			session.FailureCount++
		} else {
			session.SuccessCount++
		}
	}

	h.logger.Debug("Added history entry", "id", historyEntry.ID, "task", entry.TaskName)
}

// AddTaskCompletion adds a task completion entry
func (h *History) AddTaskCompletion(taskID, taskName string, duration time.Duration, exitCode int, runner string) {
	if !h.started {
		return
	}

	entry := &HistoryEntry{
		ID:        h.generateEntryID(),
		SessionID: h.currentSession,
		TaskID:    taskID,
		TaskName:  taskName,
		Timestamp: time.Now(),
		Duration:  duration,
		Level:     "info",
		Runner:    runner,
		ExitCode:  exitCode,
		Output:    fmt.Sprintf("Task completed in %v with exit code %d", duration, exitCode),
		Size:      int64(len(fmt.Sprintf("Task completed in %v with exit code %d", duration, exitCode))),
	}

	if exitCode != 0 {
		entry.Level = "error"
		entry.Error = fmt.Sprintf("Task failed with exit code %d", exitCode)
	}

	h.mu.Lock()
	h.addEntry(entry)
	h.mu.Unlock()
}

// Query searches the history with the given criteria
func (h *History) Query(query *HistoryQuery) (*HistoryResult, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Filter entries
	filtered := h.filterEntries(query)

	// Sort entries
	h.sortEntries(filtered, query)

	// Apply pagination
	total := len(filtered)
	start := query.Offset
	end := start + query.Limit

	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	if query.Limit <= 0 {
		start = 0
		end = total
	}

	result := &HistoryResult{
		Entries:  filtered[start:end],
		Total:    len(h.entries),
		Filtered: total,
		Query:    query,
	}

	if query.Limit > 0 {
		result.Page = query.Offset/query.Limit + 1
		result.PageSize = query.Limit
	}

	return result, nil
}

// GetEntry returns a specific entry by ID
func (h *History) GetEntry(id string) (*HistoryEntry, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	entry, exists := h.entriesMap[id]
	return entry, exists
}

// GetSession returns a specific session by ID
func (h *History) GetSession(id string) (*HistorySession, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	session, exists := h.sessions[id]
	return session, exists
}

// GetCurrentSession returns the current session
func (h *History) GetCurrentSession() (*HistorySession, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return h.GetSession(h.currentSession)
}

// GetAllSessions returns all sessions
func (h *History) GetAllSessions() []*HistorySession {
	h.mu.RLock()
	defer h.mu.RUnlock()

	sessions := make([]*HistorySession, 0, len(h.sessions))
	for _, session := range h.sessions {
		sessions = append(sessions, session)
	}

	// Sort by start time (newest first)
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartTime.After(sessions[j].StartTime)
	})

	return sessions
}

// Clear clears all history entries
func (h *History) Clear() {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.entries = h.entries[:0]
	h.entriesMap = make(map[string]*HistoryEntry)
	h.sessions = make(map[string]*HistorySession)

	// Reset statistics
	h.stats = &HistoryStats{}

	// Create new session
	h.currentSession = h.generateSessionID()
	session := &HistorySession{
		ID:        h.currentSession,
		StartTime: time.Now(),
		EntryIDs:  []string{},
	}
	h.sessions[h.currentSession] = session

	h.logger.Debug("History cleared")
}

// GetStats returns history statistics
func (h *History) GetStats() *HistoryStats {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Update current statistics
	stats := *h.stats
	stats.CurrentEntries = len(h.entries)
	stats.TotalSessions = int64(len(h.sessions))

	if len(h.entries) > 0 {
		stats.AverageSize = stats.TotalSize / int64(len(h.entries))
		stats.OldestEntry = h.entries[0].Timestamp
		stats.NewestEntry = h.entries[len(h.entries)-1].Timestamp
	}

	return &stats
}

// Export exports history to a file
func (h *History) Export(filename string, format string) error {
	h.mu.RLock()
	defer h.mu.RUnlock()

	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create export file: %w", err)
	}
	defer file.Close()

	switch format {
	case "json":
		return h.exportJSON(file)
	case "csv":
		return h.exportCSV(file)
	case "text":
		return h.exportText(file)
	default:
		return fmt.Errorf("unsupported export format: %s", format)
	}
}

// Private methods

// addEntry adds an entry to the history (must be called with lock held)
func (h *History) addEntry(entry *HistoryEntry) {
	// Check if we need to remove old entries
	if len(h.entries) >= h.maxEntries {
		h.removeOldestEntry()
	}

	// Add new entry
	h.entries = append(h.entries, entry)
	h.entriesMap[entry.ID] = entry

	// Update statistics
	h.stats.TotalEntries++
	h.stats.AddOperations++
	h.stats.TotalSize += entry.Size

	if entry.Size > h.stats.LargestEntry {
		h.stats.LargestEntry = entry.Size
	}
}

// removeOldestEntry removes the oldest entry
func (h *History) removeOldestEntry() {
	if len(h.entries) == 0 {
		return
	}

	// Remove from map
	oldestEntry := h.entries[0]
	delete(h.entriesMap, oldestEntry.ID)

	// Remove from slice
	h.entries = h.entries[1:]

	// Update statistics
	h.stats.TotalSize -= oldestEntry.Size
}

// filterEntries filters entries based on query criteria
func (h *History) filterEntries(query *HistoryQuery) []*HistoryEntry {
	var filtered []*HistoryEntry

	for _, entry := range h.entries {
		if h.matchesQuery(entry, query) {
			// Create a copy if output/metadata should be excluded
			entryCopy := *entry
			if !query.IncludeOutput {
				entryCopy.Output = ""
				entryCopy.Error = ""
			}
			if !query.IncludeMetadata {
				entryCopy.Metadata = nil
			}
			filtered = append(filtered, &entryCopy)
		}
	}

	return filtered
}

// matchesQuery checks if an entry matches the query criteria
func (h *History) matchesQuery(entry *HistoryEntry, query *HistoryQuery) bool {
	// Task ID filter
	if query.TaskID != "" && entry.TaskID != query.TaskID {
		return false
	}

	// Task name filter
	if query.TaskName != "" && !h.matchesPattern(entry.TaskName, query.TaskName) {
		return false
	}

	// Session ID filter
	if query.SessionID != "" && entry.SessionID != query.SessionID {
		return false
	}

	// Runner filter
	if query.Runner != "" && entry.Runner != query.Runner {
		return false
	}

	// Level filter
	if query.Level != "" && entry.Level != query.Level {
		return false
	}

	// Tags filter
	if len(query.Tags) > 0 && !h.containsAllTags(entry.Tags, query.Tags) {
		return false
	}

	// Time range filter
	if query.StartTime != nil && entry.Timestamp.Before(*query.StartTime) {
		return false
	}
	if query.EndTime != nil && entry.Timestamp.After(*query.EndTime) {
		return false
	}

	// Content search
	if query.ContentSearch != "" && !h.containsIgnoreCase(entry.Output, query.ContentSearch) {
		return false
	}

	// Error search
	if query.ErrorSearch != "" && !h.containsIgnoreCase(entry.Error, query.ErrorSearch) {
		return false
	}

	return true
}

// sortEntries sorts entries based on query criteria
func (h *History) sortEntries(entries []*HistoryEntry, query *HistoryQuery) {
	sortBy := query.SortBy
	if sortBy == "" {
		sortBy = "timestamp"
	}

	sortDesc := query.SortOrder == "desc"

	sort.Slice(entries, func(i, j int) bool {
		var less bool

		switch sortBy {
		case "timestamp":
			less = entries[i].Timestamp.Before(entries[j].Timestamp)
		case "duration":
			less = entries[i].Duration < entries[j].Duration
		case "size":
			less = entries[i].Size < entries[j].Size
		case "task_name":
			less = entries[i].TaskName < entries[j].TaskName
		default:
			less = entries[i].Timestamp.Before(entries[j].Timestamp)
		}

		if sortDesc {
			return !less
		}
		return less
	})
}

// Helper methods

func (h *History) matchesPattern(text, pattern string) bool {
	// Simple wildcard matching (could be enhanced with regex)
	return strings.Contains(strings.ToLower(text), strings.ToLower(pattern))
}

func (h *History) containsAllTags(entryTags, queryTags []string) bool {
	for _, queryTag := range queryTags {
		found := slices.Contains(entryTags, queryTag)
		if !found {
			return false
		}
	}
	return true
}

func (h *History) containsIgnoreCase(text, search string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(search))
}

func (h *History) generateEntryID() string {
	return fmt.Sprintf("entry_%d_%d", time.Now().UnixNano(), len(h.entries))
}

func (h *History) generateSessionID() string {
	return fmt.Sprintf("session_%d", time.Now().UnixNano())
}

func (h *History) fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

// File operations

func (h *History) saveToFile() error {
	if h.persistFile == "" {
		return nil
	}

	h.saveMu.Lock()
	defer h.saveMu.Unlock()

	// Create directory if it doesn't exist
	dir := filepath.Dir(h.persistFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Create temporary file
	tempFile := h.persistFile + ".tmp"
	file, err := os.Create(tempFile)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer file.Close()

	var writer io.Writer = file

	// Add compression if enabled
	if h.compression {
		gzipWriter := gzip.NewWriter(file)
		defer gzipWriter.Close()
		writer = gzipWriter
	}

	// Create export data
	exportData := struct {
		Entries  []*HistoryEntry   `json:"entries"`
		Sessions []*HistorySession `json:"sessions"`
		Stats    *HistoryStats     `json:"stats"`
		Version  string            `json:"version"`
		SavedAt  time.Time         `json:"saved_at"`
	}{
		Entries:  h.entries,
		Sessions: h.getAllSessionsSlice(),
		Stats:    h.stats,
		Version:  "1.0",
		SavedAt:  time.Now(),
	}

	// Encode to JSON
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(exportData); err != nil {
		return fmt.Errorf("failed to encode history: %w", err)
	}

	// Close writers
	if h.compression {
		if gzipWriter, ok := writer.(*gzip.Writer); ok {
			gzipWriter.Close()
		}
	}
	file.Close()

	// Atomic replace
	if err := os.Rename(tempFile, h.persistFile); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("failed to replace history file: %w", err)
	}

	h.stats.SaveOperations++
	h.stats.LastSave = time.Now()

	return nil
}

func (h *History) loadFromFile() error {
	file, err := os.Open(h.persistFile)
	if err != nil {
		return fmt.Errorf("failed to open history file: %w", err)
	}
	defer file.Close()

	var reader io.Reader = file

	// Check if file is compressed
	if h.compression {
		gzipReader, err := gzip.NewReader(file)
		if err != nil {
			// File might not be compressed, reset and try without compression
			file.Seek(0, 0)
			reader = file
		} else {
			defer gzipReader.Close()
			reader = gzipReader
		}
	}

	// Decode JSON
	var exportData struct {
		Entries  []*HistoryEntry   `json:"entries"`
		Sessions []*HistorySession `json:"sessions"`
		Stats    *HistoryStats     `json:"stats"`
		Version  string            `json:"version"`
		SavedAt  time.Time         `json:"saved_at"`
	}

	decoder := json.NewDecoder(reader)
	if err := decoder.Decode(&exportData); err != nil {
		return fmt.Errorf("failed to decode history: %w", err)
	}

	// Load data
	h.entries = exportData.Entries
	h.entriesMap = make(map[string]*HistoryEntry)
	for _, entry := range h.entries {
		h.entriesMap[entry.ID] = entry
	}

	h.sessions = make(map[string]*HistorySession)
	for _, session := range exportData.Sessions {
		h.sessions[session.ID] = session
	}

	if exportData.Stats != nil {
		h.stats = exportData.Stats
	}

	h.stats.LoadOperations++
	h.stats.LastLoad = time.Now()

	h.logger.Debug("Loaded history from file",
		"entries", len(h.entries),
		"sessions", len(h.sessions),
		"version", exportData.Version)

	return nil
}

func (h *History) getAllSessionsSlice() []*HistorySession {
	sessions := make([]*HistorySession, 0, len(h.sessions))
	for _, session := range h.sessions {
		sessions = append(sessions, session)
	}
	return sessions
}

func (h *History) autoSaveRoutine() {
	for {
		select {
		case <-h.saveTimer.C:
			if err := h.saveToFile(); err != nil {
				h.logger.Warn("Auto-save failed", "error", err)
			}
			h.saveTimer.Reset(h.saveInterval)
		}
	}
}

// Export methods

func (h *History) exportJSON(writer io.Writer) error {
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")

	exportData := struct {
		Entries  []*HistoryEntry   `json:"entries"`
		Sessions []*HistorySession `json:"sessions"`
		Stats    *HistoryStats     `json:"stats"`
	}{
		Entries:  h.entries,
		Sessions: h.getAllSessionsSlice(),
		Stats:    h.stats,
	}

	return encoder.Encode(exportData)
}

func (h *History) exportCSV(writer io.Writer) error {
	// CSV header
	fmt.Fprintln(writer, "ID,SessionID,TaskID,TaskName,Timestamp,Duration,Level,Runner,ExitCode,OutputSize")

	for _, entry := range h.entries {
		fmt.Fprintf(writer, "%s,%s,%s,%s,%s,%v,%s,%s,%d,%d\n",
			entry.ID,
			entry.SessionID,
			entry.TaskID,
			entry.TaskName,
			entry.Timestamp.Format(time.RFC3339),
			entry.Duration,
			entry.Level,
			entry.Runner,
			entry.ExitCode,
			entry.Size,
		)
	}

	return nil
}

func (h *History) exportText(writer io.Writer) error {
	for _, entry := range h.entries {
		fmt.Fprintf(writer, "[%s] [%s] %s: %s\n",
			entry.Timestamp.Format("2006-01-02 15:04:05"),
			entry.TaskName,
			strings.ToUpper(entry.Level),
			entry.Output,
		)

		if entry.Error != "" && entry.Error != entry.Output {
			fmt.Fprintf(writer, "ERROR: %s\n", entry.Error)
		}

		fmt.Fprintln(writer, "")
	}

	return nil
}
