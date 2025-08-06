package output

import (
	"fmt"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vivalchemy/wake/pkg/task"
)

// Model represents the output component's data model
type Model struct {
	// State tracking
	initialized bool
	lastUpdate  time.Time
	updateCount int64

	// Output state
	lineCount    int64
	totalBytes   int64
	lastLineTime time.Time

	// Buffer state
	bufferSize    int
	maxBufferSize int

	// Display state
	scrollPosition int
	viewportHeight int
	viewportWidth  int

	// Performance tracking
	renderCount    int64
	lastRenderTime time.Time

	// Thread safety
	mu sync.RWMutex
}

// Messages for output component communication

// OutputLineMsg is sent when a new output line is received
type OutputLineMsg struct {
	Line      *OutputLine
	Timestamp time.Time
}

// TaskSelectedMsg is sent when a task is selected for output display
type TaskSelectedMsg struct {
	Task      *task.Task
	Timestamp time.Time
}

// TaskStartedMsg is sent when a task starts execution
type TaskStartedMsg struct {
	Task      *task.Task
	Timestamp time.Time
}

// TaskFinishedMsg is sent when a task finishes execution
type TaskFinishedMsg struct {
	Task      *task.Task
	ExitCode  int
	Duration  time.Duration
	Timestamp time.Time
}

// TaskFailedMsg is sent when a task fails
type TaskFailedMsg struct {
	Task      *task.Task
	Error     string
	ExitCode  int
	Timestamp time.Time
}

// ClearOutputMsg is sent to clear all output
type ClearOutputMsg struct {
	Timestamp time.Time
}

// OutputClearedMsg is sent when output has been cleared
type OutputClearedMsg struct {
	Timestamp time.Time
}

// ToggleTimestampsMsg is sent to toggle timestamp display
type ToggleTimestampsMsg struct {
	Timestamp time.Time
}

// ToggleTaskInfoMsg is sent to toggle task info display
type ToggleTaskInfoMsg struct {
	Timestamp time.Time
}

// ToggleAutoScrollMsg is sent to toggle auto-scroll
type ToggleAutoScrollMsg struct {
	Timestamp time.Time
}

// ToggleWrapLinesMsg is sent to toggle line wrapping
type ToggleWrapLinesMsg struct {
	Timestamp time.Time
}

// ToggleStdoutMsg is sent to toggle stdout display
type ToggleStdoutMsg struct {
	Timestamp time.Time
}

// ToggleStderrMsg is sent to toggle stderr display
type ToggleStderrMsg struct {
	Timestamp time.Time
}

// SetFilterLevelMsg is sent to set the filter level
type SetFilterLevelMsg struct {
	Level     LogLevel
	Timestamp time.Time
}

// ScrollToTopMsg is sent to scroll to the top
type ScrollToTopMsg struct {
	Timestamp time.Time
}

// ScrollToBottomMsg is sent to scroll to the bottom
type ScrollToBottomMsg struct {
	Timestamp time.Time
}

// ViewportResizedMsg is sent when the viewport is resized
type ViewportResizedMsg struct {
	Width     int
	Height    int
	Timestamp time.Time
}

// OutputStatsMsg is sent to request output statistics
type OutputStatsMsg struct {
	Timestamp time.Time
}

// BufferFullMsg is sent when the buffer becomes full
type BufferFullMsg struct {
	BufferSize int
	MaxSize    int
	Timestamp  time.Time
}

// NewModel creates a new output model
func NewModel() *Model {
	return &Model{
		initialized:    false,
		lastUpdate:     time.Now(),
		updateCount:    0,
		lineCount:      0,
		totalBytes:     0,
		lastLineTime:   time.Time{},
		bufferSize:     0,
		maxBufferSize:  10000,
		scrollPosition: 0,
		viewportHeight: 0,
		viewportWidth:  0,
		renderCount:    0,
		lastRenderTime: time.Now(),
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
	case OutputLineMsg:
		cmds = append(cmds, m.handleOutputLine(msg))

	case TaskSelectedMsg:
		cmds = append(cmds, m.handleTaskSelected(msg))

	case TaskStartedMsg:
		cmds = append(cmds, m.handleTaskStarted(msg))

	case TaskFinishedMsg:
		cmds = append(cmds, m.handleTaskFinished(msg))

	case TaskFailedMsg:
		cmds = append(cmds, m.handleTaskFailed(msg))

	case ClearOutputMsg:
		cmds = append(cmds, m.handleClearOutput(msg))

	case ViewportResizedMsg:
		cmds = append(cmds, m.handleViewportResized(msg))

	case OutputStatsMsg:
		cmds = append(cmds, m.handleOutputStats(msg))
	}

	return m, tea.Batch(cmds...)
}

// handleOutputLine handles new output lines
func (m *Model) handleOutputLine(msg OutputLineMsg) tea.Cmd {
	m.lineCount++
	m.totalBytes += int64(len(msg.Line.Content))
	m.lastLineTime = msg.Timestamp
	m.bufferSize++

	// Check if buffer is getting full
	if m.bufferSize > m.maxBufferSize*9/10 { // 90% full
		return func() tea.Msg {
			return BufferFullMsg{
				BufferSize: m.bufferSize,
				MaxSize:    m.maxBufferSize,
				Timestamp:  time.Now(),
			}
		}
	}

	return nil
}

// handleTaskSelected handles task selection
func (m *Model) handleTaskSelected(msg TaskSelectedMsg) tea.Cmd {
	// Reset counters for new task
	m.lineCount = 0
	m.totalBytes = 0
	m.bufferSize = 0
	m.scrollPosition = 0

	return nil
}

// handleTaskStarted handles task start events
func (m *Model) handleTaskStarted(msg TaskStartedMsg) tea.Cmd {
	// Add a start marker line
	return func() tea.Msg {
		return OutputLineMsg{
			Line: &OutputLine{
				Content:    "=== Task Started ===",
				Timestamp:  msg.Timestamp,
				TaskID:     msg.Task.ID,
				TaskName:   msg.Task.Name,
				IsStderr:   false,
				Level:      LogLevelInfo,
				LineNumber: int(m.lineCount + 1),
			},
			Timestamp: msg.Timestamp,
		}
	}
}

// handleTaskFinished handles task completion events
func (m *Model) handleTaskFinished(msg TaskFinishedMsg) tea.Cmd {
	// Add a completion marker line
	return func() tea.Msg {
		return OutputLineMsg{
			Line: &OutputLine{
				Content:    fmt.Sprintf("=== Task Completed (exit code: %d, duration: %v) ===", msg.ExitCode, msg.Duration),
				Timestamp:  msg.Timestamp,
				TaskID:     msg.Task.ID,
				TaskName:   msg.Task.Name,
				IsStderr:   false,
				Level:      LogLevelInfo,
				LineNumber: int(m.lineCount + 1),
			},
			Timestamp: msg.Timestamp,
		}
	}
}

// handleTaskFailed handles task failure events
func (m *Model) handleTaskFailed(msg TaskFailedMsg) tea.Cmd {
	// Add a failure marker line
	return func() tea.Msg {
		return OutputLineMsg{
			Line: &OutputLine{
				Content:    fmt.Sprintf("=== Task Failed (exit code: %d): %s ===", msg.ExitCode, msg.Error),
				Timestamp:  msg.Timestamp,
				TaskID:     msg.Task.ID,
				TaskName:   msg.Task.Name,
				IsStderr:   true,
				Level:      LogLevelError,
				LineNumber: int(m.lineCount + 1),
			},
			Timestamp: msg.Timestamp,
		}
	}
}

// handleClearOutput handles output clearing
func (m *Model) handleClearOutput(msg ClearOutputMsg) tea.Cmd {
	m.lineCount = 0
	m.totalBytes = 0
	m.bufferSize = 0
	m.scrollPosition = 0
	m.lastLineTime = time.Time{}

	return func() tea.Msg {
		return OutputClearedMsg{
			Timestamp: time.Now(),
		}
	}
}

// handleViewportResized handles viewport resize events
func (m *Model) handleViewportResized(msg ViewportResizedMsg) tea.Cmd {
	m.viewportWidth = msg.Width
	m.viewportHeight = msg.Height

	return nil
}

// handleOutputStats handles output statistics requests
func (m *Model) handleOutputStats(msg OutputStatsMsg) tea.Cmd {
	stats := m.getStats()

	return func() tea.Msg {
		return OutputStatsResponseMsg{
			Stats:     stats,
			Timestamp: time.Now(),
		}
	}
}

// OutputStatsResponseMsg contains output statistics
type OutputStatsResponseMsg struct {
	Stats     ModelStats
	Timestamp time.Time
}

// ModelStats provides statistics about the model
type ModelStats struct {
	// Basic counters
	UpdateCount   int64 `json:"update_count"`
	RenderCount   int64 `json:"render_count"`
	LineCount     int64 `json:"line_count"`
	TotalBytes    int64 `json:"total_bytes"`
	BufferSize    int   `json:"buffer_size"`
	MaxBufferSize int   `json:"max_buffer_size"`

	// Timing information
	LastUpdate   time.Time `json:"last_update"`
	LastRender   time.Time `json:"last_render"`
	LastLineTime time.Time `json:"last_line_time"`

	// Display state
	ScrollPosition int `json:"scroll_position"`
	ViewportWidth  int `json:"viewport_width"`
	ViewportHeight int `json:"viewport_height"`

	// Performance metrics
	UpdatesPerSecond float64 `json:"updates_per_second"`
	RendersPerSecond float64 `json:"renders_per_second"`
	BytesPerSecond   float64 `json:"bytes_per_second"`
	LinesPerSecond   float64 `json:"lines_per_second"`

	// State flags
	Initialized bool `json:"initialized"`
	HasOutput   bool `json:"has_output"`
	BufferFull  bool `json:"buffer_full"`
}

// GetStats returns model statistics
func (m *Model) GetStats() ModelStats {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.getStats()
}

// getStats returns model statistics (internal, assumes lock is held)
func (m *Model) getStats() ModelStats {
	now := time.Now()
	timeSinceInit := now.Sub(m.lastUpdate)

	var updatesPerSecond, rendersPerSecond, bytesPerSecond, linesPerSecond float64

	if timeSinceInit.Seconds() > 0 {
		updatesPerSecond = float64(m.updateCount) / timeSinceInit.Seconds()
		rendersPerSecond = float64(m.renderCount) / timeSinceInit.Seconds()
		bytesPerSecond = float64(m.totalBytes) / timeSinceInit.Seconds()
		linesPerSecond = float64(m.lineCount) / timeSinceInit.Seconds()
	}

	return ModelStats{
		UpdateCount:      m.updateCount,
		RenderCount:      m.renderCount,
		LineCount:        m.lineCount,
		TotalBytes:       m.totalBytes,
		BufferSize:       m.bufferSize,
		MaxBufferSize:    m.maxBufferSize,
		LastUpdate:       m.lastUpdate,
		LastRender:       m.lastRenderTime,
		LastLineTime:     m.lastLineTime,
		ScrollPosition:   m.scrollPosition,
		ViewportWidth:    m.viewportWidth,
		ViewportHeight:   m.viewportHeight,
		UpdatesPerSecond: updatesPerSecond,
		RendersPerSecond: rendersPerSecond,
		BytesPerSecond:   bytesPerSecond,
		LinesPerSecond:   linesPerSecond,
		Initialized:      m.initialized,
		HasOutput:        m.lineCount > 0,
		BufferFull:       m.bufferSize >= m.maxBufferSize,
	}
}

// IncrementLineCount increments the line counter
func (m *Model) IncrementLineCount() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.lineCount++
	m.bufferSize++
}

// IncrementRenderCount increments the render counter
func (m *Model) IncrementRenderCount() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.renderCount++
	m.lastRenderTime = time.Now()
}

// ClearLines resets line counters
func (m *Model) ClearLines() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.lineCount = 0
	m.totalBytes = 0
	m.bufferSize = 0
	m.scrollPosition = 0
	m.lastLineTime = time.Time{}
}

// GetLineCount returns the current line count
func (m *Model) GetLineCount() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.lineCount
}

// GetTotalBytes returns the total bytes processed
func (m *Model) GetTotalBytes() int64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.totalBytes
}

// GetBufferSize returns the current buffer size
func (m *Model) GetBufferSize() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.bufferSize
}

// IsBufferFull returns whether the buffer is full
func (m *Model) IsBufferFull() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.bufferSize >= m.maxBufferSize
}

// SetScrollPosition sets the scroll position
func (m *Model) SetScrollPosition(position int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.scrollPosition = position
}

// GetScrollPosition returns the current scroll position
func (m *Model) GetScrollPosition() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.scrollPosition
}

// SetViewportSize sets the viewport dimensions
func (m *Model) SetViewportSize(width, height int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.viewportWidth = width
	m.viewportHeight = height
}

// GetViewportSize returns the viewport dimensions
func (m *Model) GetViewportSize() (int, int) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.viewportWidth, m.viewportHeight
}

// SetMaxBufferSize sets the maximum buffer size
func (m *Model) SetMaxBufferSize(size int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.maxBufferSize = size
}

// GetMaxBufferSize returns the maximum buffer size
func (m *Model) GetMaxBufferSize() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.maxBufferSize
}

// IsInitialized returns whether the model is initialized
func (m *Model) IsInitialized() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.initialized
}

// GetLastUpdate returns the time of the last update
func (m *Model) GetLastUpdate() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.lastUpdate
}

// GetLastLineTime returns the time of the last line received
func (m *Model) GetLastLineTime() time.Time {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.lastLineTime
}

// HasRecentActivity returns whether there has been recent activity
func (m *Model) HasRecentActivity(threshold time.Duration) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.lastLineTime.IsZero() {
		return false
	}

	return time.Since(m.lastLineTime) < threshold
}

// GetActivityRate returns the current activity rate (lines per second)
func (m *Model) GetActivityRate(window time.Duration) float64 {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.lastLineTime.IsZero() || window <= 0 {
		return 0
	}

	timeSinceLastLine := time.Since(m.lastLineTime)
	if timeSinceLastLine > window {
		return 0 // No recent activity
	}

	// Simple rate calculation based on total lines and time
	totalTime := time.Since(m.lastUpdate)
	if totalTime.Seconds() > 0 {
		return float64(m.lineCount) / totalTime.Seconds()
	}

	return 0
}

// Reset resets the model to its initial state
func (m *Model) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.lineCount = 0
	m.totalBytes = 0
	m.bufferSize = 0
	m.scrollPosition = 0
	m.lastLineTime = time.Time{}
	m.lastUpdate = time.Now()
	m.updateCount = 0
	m.renderCount = 0
	m.lastRenderTime = time.Now()
}

// OutputMetrics provides metrics about output processing
type OutputMetrics struct {
	LinesPerSecond    float64 `json:"lines_per_second"`
	BytesPerSecond    float64 `json:"bytes_per_second"`
	UpdatesPerSecond  float64 `json:"updates_per_second"`
	BufferUtilization float64 `json:"buffer_utilization"`
	AverageLineLength float64 `json:"average_line_length"`
}

// GetMetrics returns current output metrics
func (m *Model) GetMetrics() OutputMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	now := time.Now()
	duration := now.Sub(m.lastUpdate)

	var linesPerSecond, bytesPerSecond, updatesPerSecond float64
	var averageLineLength float64

	if duration.Seconds() > 0 {
		linesPerSecond = float64(m.lineCount) / duration.Seconds()
		bytesPerSecond = float64(m.totalBytes) / duration.Seconds()
		updatesPerSecond = float64(m.updateCount) / duration.Seconds()
	}

	if m.lineCount > 0 {
		averageLineLength = float64(m.totalBytes) / float64(m.lineCount)
	}

	bufferUtilization := float64(m.bufferSize) / float64(m.maxBufferSize) * 100

	return OutputMetrics{
		LinesPerSecond:    linesPerSecond,
		BytesPerSecond:    bytesPerSecond,
		UpdatesPerSecond:  updatesPerSecond,
		BufferUtilization: bufferUtilization,
		AverageLineLength: averageLineLength,
	}
}
