package output

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/vivalchemy/wake/pkg/logger"
)

// Manager manages output streams and formatting for task execution
type Manager struct {
	// Configuration
	options *ManagerOptions

	// Output destinations
	stdout io.Writer
	stderr io.Writer
	file   io.Writer

	// Streamers
	streamers   map[string]*Streamer
	streamersMu sync.RWMutex

	// Buffers
	buffers   map[string]*Buffer
	buffersMu sync.RWMutex

	// History
	history *History

	// State
	started bool
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup

	// Synchronization
	mu sync.RWMutex

	// Logger
	logger logger.Logger
}

// ManagerOptions contains configuration for the output manager
type ManagerOptions struct {
	// Output format and verbosity
	Format        OutputFormat
	Verbosity     int
	ShowProgress  bool
	ShowTimestamp bool
	ShowDuration  bool
	Colors        bool

	// File output
	File   string
	Append bool

	// Buffering
	BufferSize    int
	FlushInterval time.Duration
	StreamOutput  bool

	// History
	MaxHistory  int
	SaveHistory bool
	HistoryFile string

	// Performance
	AsyncWrite bool
	BatchSize  int

	// Filtering
	FilterLevels []string
	FilterTasks  []string
}

// OutputFormat represents different output formats
type OutputFormat int

const (
	FormatText OutputFormat = iota
	FormatJSON
	FormatYAML
	FormatTable
	FormatRaw
)

// String returns the string representation of OutputFormat
func (f OutputFormat) String() string {
	switch f {
	case FormatText:
		return "text"
	case FormatJSON:
		return "json"
	case FormatYAML:
		return "yaml"
	case FormatTable:
		return "table"
	case FormatRaw:
		return "raw"
	default:
		return "unknown"
	}
}

// OutputEntry represents a single output entry
type OutputEntry struct {
	Timestamp time.Time
	TaskID    string
	TaskName  string
	Level     string
	Message   string
	Data      map[string]any
	IsStderr  bool
	Source    string
}

// NewManager creates a new output manager
func NewManager(options *ManagerOptions, logger logger.Logger) *Manager {
	if options == nil {
		options = createDefaultManagerOptions()
	}

	ctx, cancel := context.WithCancel(context.Background())

	manager := &Manager{
		options:   options,
		stdout:    os.Stdout,
		stderr:    os.Stderr,
		streamers: make(map[string]*Streamer),
		buffers:   make(map[string]*Buffer),
		history:   NewHistory(options.MaxHistory, logger),
		ctx:       ctx,
		cancel:    cancel,
		logger:    logger.WithGroup("output-manager"),
	}

	// Initialize file output if specified
	if err := manager.initializeFileOutput(); err != nil {
		logger.Warn("Failed to initialize file output", "error", err)
	}

	return manager
}

// createDefaultManagerOptions creates default manager options
func createDefaultManagerOptions() *ManagerOptions {
	return &ManagerOptions{
		Format:        FormatText,
		Verbosity:     1,
		ShowProgress:  true,
		ShowTimestamp: false,
		ShowDuration:  true,
		Colors:        true,
		File:          "",
		Append:        false,
		BufferSize:    64 * 1024,
		FlushInterval: time.Second,
		StreamOutput:  true,
		MaxHistory:    1000,
		SaveHistory:   false,
		HistoryFile:   "",
		AsyncWrite:    true,
		BatchSize:     100,
		FilterLevels:  []string{},
		FilterTasks:   []string{},
	}
}

// Start starts the output manager
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.started {
		return fmt.Errorf("output manager already started")
	}

	m.logger.Debug("Starting output manager")

	// Start history if enabled
	if m.options.SaveHistory {
		if err := m.history.Start(m.options.HistoryFile); err != nil {
			m.logger.Warn("Failed to start history", "error", err)
		}
	}

	// Start background flush routine if async writing is enabled
	if m.options.AsyncWrite {
		m.wg.Add(1)
		go m.flushRoutine()
	}

	m.started = true
	m.logger.Debug("Output manager started")

	return nil
}

// Stop stops the output manager
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.started {
		return nil
	}

	m.logger.Debug("Stopping output manager")

	// Cancel context to stop background routines
	m.cancel()

	// Wait for background routines to finish
	m.wg.Wait()

	// Flush all buffers
	m.flushAllBuffers()

	// Stop history
	if err := m.history.Stop(); err != nil {
		m.logger.Warn("Failed to stop history", "error", err)
	}

	// Close file if opened
	if m.file != nil && m.file != os.Stdout && m.file != os.Stderr {
		if closer, ok := m.file.(io.Closer); ok {
			closer.Close()
		}
	}

	m.started = false
	m.logger.Debug("Output manager stopped")

	return nil
}

// CreateBuffer creates a new output buffer for a task
func (m *Manager) CreateBuffer(taskID string, bufferSize int) *Buffer {
	m.buffersMu.Lock()
	defer m.buffersMu.Unlock()

	if bufferSize <= 0 {
		bufferSize = m.options.BufferSize
	}

	buffer := NewBuffer(bufferSize)
	m.buffers[taskID] = buffer

	m.logger.Debug("Created buffer for task", "task_id", taskID, "buffer_size", bufferSize)

	return buffer
}

// GetBuffer returns the buffer for a task
func (m *Manager) GetBuffer(taskID string) (*Buffer, bool) {
	m.buffersMu.RLock()
	defer m.buffersMu.RUnlock()

	buffer, exists := m.buffers[taskID]
	return buffer, exists
}

// RemoveBuffer removes the buffer for a task
func (m *Manager) RemoveBuffer(taskID string) {
	m.buffersMu.Lock()
	defer m.buffersMu.Unlock()

	if buffer, exists := m.buffers[taskID]; exists {
		// Flush buffer before removing
		m.flushBuffer(taskID, buffer)
		delete(m.buffers, taskID)
		m.logger.Debug("Removed buffer for task", "task_id", taskID)
	}
}

// CreateStreamer creates a new output streamer for a task
func (m *Manager) CreateStreamer(taskID string, options *StreamerOptions) *Streamer {
	m.streamersMu.Lock()
	defer m.streamersMu.Unlock()

	if options == nil {
		options = &StreamerOptions{
			BufferSize:    m.options.BufferSize,
			FlushInterval: m.options.FlushInterval,
			Colors:        m.options.Colors,
			Timestamps:    m.options.ShowTimestamp,
		}
	}

	streamer := NewStreamer(taskID, options, m.logger)

	// Set output destinations
	streamer.SetStdout(m.stdout)
	streamer.SetStderr(m.stderr)
	if m.file != nil {
		streamer.SetFile(m.file)
	}

	m.streamers[taskID] = streamer

	m.logger.Debug("Created streamer for task", "task_id", taskID)

	return streamer
}

// GetStreamer returns the streamer for a task
func (m *Manager) GetStreamer(taskID string) (*Streamer, bool) {
	m.streamersMu.RLock()
	defer m.streamersMu.RUnlock()

	streamer, exists := m.streamers[taskID]
	return streamer, exists
}

// RemoveStreamer removes the streamer for a task
func (m *Manager) RemoveStreamer(taskID string) {
	m.streamersMu.Lock()
	defer m.streamersMu.Unlock()

	if streamer, exists := m.streamers[taskID]; exists {
		streamer.Close()
		delete(m.streamers, taskID)
		m.logger.Debug("Removed streamer for task", "task_id", taskID)
	}
}

// Write writes an output entry
func (m *Manager) Write(entry *OutputEntry) error {
	if !m.started {
		return fmt.Errorf("output manager not started")
	}

	// Apply filters
	if !m.shouldWrite(entry) {
		return nil
	}

	// Add to history
	if m.options.SaveHistory {
		m.history.Add(entry)
	}

	// Format and write the entry
	return m.writeEntry(entry)
}

// WriteTaskOutput writes task output
func (m *Manager) WriteTaskOutput(taskID, taskName string, data []byte, isStderr bool) error {
	entry := &OutputEntry{
		Timestamp: time.Now(),
		TaskID:    taskID,
		TaskName:  taskName,
		Message:   string(data),
		IsStderr:  isStderr,
		Source:    "task",
	}

	if isStderr {
		entry.Level = "error"
	} else {
		entry.Level = "info"
	}

	return m.Write(entry)
}

// WriteMessage writes a general message
func (m *Manager) WriteMessage(level, message string, data map[string]any) error {
	entry := &OutputEntry{
		Timestamp: time.Now(),
		Level:     level,
		Message:   message,
		Data:      data,
		Source:    "system",
	}

	return m.Write(entry)
}

// WriteProgress writes progress information
func (m *Manager) WriteProgress(taskID, taskName, message string, progress float64) error {
	if !m.options.ShowProgress {
		return nil
	}

	entry := &OutputEntry{
		Timestamp: time.Now(),
		TaskID:    taskID,
		TaskName:  taskName,
		Level:     "progress",
		Message:   message,
		Data: map[string]any{
			"progress": progress,
		},
		Source: "progress",
	}

	return m.Write(entry)
}

// Flush flushes all pending output
func (m *Manager) Flush() error {
	m.flushAllBuffers()

	// Flush all streamers
	m.streamersMu.RLock()
	for _, streamer := range m.streamers {
		streamer.Flush()
	}
	m.streamersMu.RUnlock()

	// Flush file output
	if m.file != nil {
		if flusher, ok := m.file.(interface{ Flush() error }); ok {
			return flusher.Flush()
		}
	}

	return nil
}

// GetHistory returns the output history
func (m *Manager) GetHistory() *History {
	return m.history
}

// SetStdout sets the stdout writer
func (m *Manager) SetStdout(w io.Writer) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.stdout = w

	// Update all streamers
	m.streamersMu.RLock()
	for _, streamer := range m.streamers {
		streamer.SetStdout(w)
	}
	m.streamersMu.RUnlock()
}

// SetStderr sets the stderr writer
func (m *Manager) SetStderr(w io.Writer) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.stderr = w

	// Update all streamers
	m.streamersMu.RLock()
	for _, streamer := range m.streamers {
		streamer.SetStderr(w)
	}
	m.streamersMu.RUnlock()
}

// SetFile sets the file writer
func (m *Manager) SetFile(w io.Writer) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.file = w

	// Update all streamers
	m.streamersMu.RLock()
	for _, streamer := range m.streamers {
		streamer.SetFile(w)
	}
	m.streamersMu.RUnlock()
}

// Private methods

// initializeFileOutput initializes file output if configured
func (m *Manager) initializeFileOutput() error {
	if m.options.File == "" {
		return nil
	}

	flags := os.O_CREATE | os.O_WRONLY
	if m.options.Append {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}

	file, err := os.OpenFile(m.options.File, flags, 0644)
	if err != nil {
		return fmt.Errorf("failed to open output file: %w", err)
	}

	m.file = file
	m.logger.Debug("Initialized file output", "file", m.options.File, "append", m.options.Append)

	return nil
}

// shouldWrite determines if an entry should be written based on filters
func (m *Manager) shouldWrite(entry *OutputEntry) bool {
	// Check verbosity level
	if !m.checkVerbosity(entry.Level) {
		return false
	}

	// Check level filters
	if len(m.options.FilterLevels) > 0 {
		found := false
		for _, level := range m.options.FilterLevels {
			if entry.Level == level {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check task filters
	if len(m.options.FilterTasks) > 0 && entry.TaskID != "" {
		found := false
		for _, taskID := range m.options.FilterTasks {
			if entry.TaskID == taskID || entry.TaskName == taskID {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

// checkVerbosity checks if the entry should be written based on verbosity level
func (m *Manager) checkVerbosity(level string) bool {
	levelValues := map[string]int{
		"error":    0,
		"warn":     1,
		"info":     2,
		"debug":    3,
		"trace":    4,
		"progress": 2,
	}

	entryLevel, exists := levelValues[level]
	if !exists {
		entryLevel = 2 // Default to info level
	}

	return entryLevel <= m.options.Verbosity
}

// writeEntry writes a formatted entry to the appropriate outputs
func (m *Manager) writeEntry(entry *OutputEntry) error {
	// Format the entry
	formatted, err := m.formatEntry(entry)
	if err != nil {
		return fmt.Errorf("failed to format entry: %w", err)
	}

	// Write to appropriate output
	var writer io.Writer
	if entry.IsStderr {
		writer = m.stderr
	} else {
		writer = m.stdout
	}

	if m.options.AsyncWrite {
		// For async writing, we would queue the write operation
		// For simplicity, we'll write synchronously here
		_, err = writer.Write(formatted)
	} else {
		_, err = writer.Write(formatted)
	}

	if err != nil {
		return fmt.Errorf("failed to write entry: %w", err)
	}

	// Also write to file if configured
	if m.file != nil && m.file != writer {
		if _, err := m.file.Write(formatted); err != nil {
			m.logger.Warn("Failed to write to file", "error", err)
		}
	}

	return nil
}

// formatEntry formats an output entry according to the configured format
func (m *Manager) formatEntry(entry *OutputEntry) ([]byte, error) {
	switch m.options.Format {
	case FormatText:
		return m.formatTextEntry(entry), nil
	case FormatJSON:
		return m.formatJSONEntry(entry)
	case FormatYAML:
		return m.formatYAMLEntry(entry)
	case FormatTable:
		return m.formatTableEntry(entry), nil
	case FormatRaw:
		return []byte(entry.Message), nil
	default:
		return m.formatTextEntry(entry), nil
	}
}

// formatTextEntry formats an entry as plain text
func (m *Manager) formatTextEntry(entry *OutputEntry) []byte {
	var result string

	// Add timestamp if enabled
	if m.options.ShowTimestamp {
		result += entry.Timestamp.Format("15:04:05.000") + " "
	}

	// Add task information
	if entry.TaskName != "" {
		if m.options.Colors {
			result += fmt.Sprintf("\033[36m[%s]\033[0m ", entry.TaskName)
		} else {
			result += fmt.Sprintf("[%s] ", entry.TaskName)
		}
	}

	// Add level with colors if enabled
	if entry.Level != "" && entry.Level != "info" {
		if m.options.Colors {
			levelColor := m.getLevelColor(entry.Level)
			result += fmt.Sprintf("\033[%sm%s\033[0m: ", levelColor, strings.ToUpper(entry.Level))
		} else {
			result += fmt.Sprintf("%s: ", strings.ToUpper(entry.Level))
		}
	}

	// Add message
	result += entry.Message

	// Add newline if not present
	if !strings.HasSuffix(result, "\n") {
		result += "\n"
	}

	return []byte(result)
}

// formatJSONEntry formats an entry as JSON
func (m *Manager) formatJSONEntry(entry *OutputEntry) ([]byte, error) {
	// This would use a JSON encoder to format the entry
	// For now, return a simple JSON representation
	jsonStr := fmt.Sprintf(`{"timestamp":"%s","task":"%s","level":"%s","message":"%s"}`,
		entry.Timestamp.Format(time.RFC3339),
		entry.TaskName,
		entry.Level,
		entry.Message)

	return []byte(jsonStr + "\n"), nil
}

// formatYAMLEntry formats an entry as YAML
func (m *Manager) formatYAMLEntry(entry *OutputEntry) ([]byte, error) {
	// This would use a YAML encoder to format the entry
	// For now, return a simple YAML representation
	yamlStr := fmt.Sprintf("timestamp: %s\ntask: %s\nlevel: %s\nmessage: %s\n---\n",
		entry.Timestamp.Format(time.RFC3339),
		entry.TaskName,
		entry.Level,
		entry.Message)

	return []byte(yamlStr), nil
}

// formatTableEntry formats an entry as a table row
func (m *Manager) formatTableEntry(entry *OutputEntry) []byte {
	// This would format the entry as a table row
	// For now, return a simple tab-separated format
	tableRow := fmt.Sprintf("%s\t%s\t%s\t%s\n",
		entry.Timestamp.Format("15:04:05"),
		entry.TaskName,
		entry.Level,
		entry.Message)

	return []byte(tableRow)
}

// getLevelColor returns the ANSI color code for a log level
func (m *Manager) getLevelColor(level string) string {
	colors := map[string]string{
		"error":    "31", // Red
		"warn":     "33", // Yellow
		"info":     "32", // Green
		"debug":    "36", // Cyan
		"trace":    "35", // Magenta
		"progress": "34", // Blue
	}

	if color, exists := colors[level]; exists {
		return color
	}

	return "0" // Default (no color)
}

// flushRoutine runs in the background to periodically flush buffers
func (m *Manager) flushRoutine() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.options.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.flushAllBuffers()
		}
	}
}

// flushAllBuffers flushes all task buffers
func (m *Manager) flushAllBuffers() {
	m.buffersMu.RLock()
	for taskID, buffer := range m.buffers {
		m.flushBuffer(taskID, buffer)
	}
	m.buffersMu.RUnlock()
}

// flushBuffer flushes a specific buffer
func (m *Manager) flushBuffer(taskID string, buffer *Buffer) {
	// Read all available data from buffer
	for {
		data, err := buffer.Read()
		if err != nil || len(data) == 0 {
			break
		}

		// Write the data (this is a simplified implementation)
		if _, err := m.stdout.Write(data); err != nil {
			m.logger.Warn("Failed to flush buffer", "task_id", taskID, "error", err)
		}
	}
}

// GetStats returns output manager statistics
func (m *Manager) GetStats() map[string]any {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := map[string]any{
		"started":          m.started,
		"active_buffers":   len(m.buffers),
		"active_streamers": len(m.streamers),
		"format":           m.options.Format.String(),
		"verbosity":        m.options.Verbosity,
		"async_write":      m.options.AsyncWrite,
	}

	// Add history stats
	if m.history != nil {
		stats["history"] = m.history.GetStats()
	}

	return stats
}
