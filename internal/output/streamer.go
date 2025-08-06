package output

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/vivalchemy/wake/pkg/logger"
)

// Streamer handles real-time streaming of task output
type Streamer struct {
	// Configuration
	taskID  string
	options *StreamerOptions

	// Output destinations
	stdout io.Writer
	stderr io.Writer
	file   io.Writer

	// Buffering
	stdoutBuf *LineBuffer
	stderrBuf *LineBuffer

	// State management
	running bool
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup

	// Statistics
	stats *StreamerStats

	// Synchronization
	mu sync.RWMutex

	// Logger
	logger logger.Logger
}

// StreamerOptions contains configuration for a streamer
type StreamerOptions struct {
	// Buffering
	BufferSize    int
	FlushInterval time.Duration
	LineBuffering bool

	// Formatting
	Colors     bool
	Timestamps bool
	TaskPrefix bool

	// Output control
	Tee            bool
	DropDuplicates bool
	MaxLineLength  int

	// Performance
	AsyncFlush bool
	BatchSize  int

	// Filtering
	StdoutFilter func(string) bool
	StderrFilter func(string) bool
}

// StreamerStats tracks streaming statistics
type StreamerStats struct {
	// Counters
	StdoutLines    int64
	StderrLines    int64
	StdoutBytes    int64
	StderrBytes    int64
	DroppedLines   int64
	DuplicateLines int64

	// Timing
	StartTime     time.Time
	LastWrite     time.Time
	TotalDuration time.Duration

	// Performance
	FlushCount    int64
	BufferFlushes int64
	AsyncWrites   int64
}

// LineBuffer handles line-buffered output with optional processing
type LineBuffer struct {
	buffer    []byte
	lines     chan string
	processor func(string) string
	maxSize   int
	mu        sync.Mutex
}

// NewStreamer creates a new output streamer
func NewStreamer(taskID string, options *StreamerOptions, logger logger.Logger) *Streamer {
	if options == nil {
		options = createDefaultStreamerOptions()
	}

	ctx, cancel := context.WithCancel(context.Background())

	streamer := &Streamer{
		taskID:  taskID,
		options: options,
		ctx:     ctx,
		cancel:  cancel,
		stats:   &StreamerStats{StartTime: time.Now()},
		logger:  logger.WithGroup("streamer").With("task_id", taskID),
	}

	// Initialize line buffers
	streamer.stdoutBuf = NewLineBuffer(options.BufferSize, streamer.processStdoutLine)
	streamer.stderrBuf = NewLineBuffer(options.BufferSize, streamer.processStderrLine)

	return streamer
}

// createDefaultStreamerOptions creates default streamer options
func createDefaultStreamerOptions() *StreamerOptions {
	return &StreamerOptions{
		BufferSize:     8192,
		FlushInterval:  100 * time.Millisecond,
		LineBuffering:  true,
		Colors:         true,
		Timestamps:     false,
		TaskPrefix:     true,
		Tee:            false,
		DropDuplicates: false,
		MaxLineLength:  0, // No limit
		AsyncFlush:     true,
		BatchSize:      10,
		StdoutFilter:   nil,
		StderrFilter:   nil,
	}
}

// Start starts the streamer
func (s *Streamer) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("streamer already running")
	}

	s.logger.Debug("Starting streamer")

	// Start line buffer processors
	s.wg.Add(2)
	go s.processStdoutLines()
	go s.processStderrLines()

	// Start flush routine if async flushing is enabled
	if s.options.AsyncFlush {
		s.wg.Add(1)
		go s.flushRoutine()
	}

	s.running = true
	s.stats.StartTime = time.Now()

	s.logger.Debug("Streamer started")
	return nil
}

// Stop stops the streamer
func (s *Streamer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.logger.Debug("Stopping streamer")

	// Cancel context to stop background routines
	s.cancel()

	// Close line buffer channels
	s.stdoutBuf.Close()
	s.stderrBuf.Close()

	// Wait for all routines to finish
	s.wg.Wait()

	// Final flush
	s.flush()

	s.running = false
	s.stats.TotalDuration = time.Since(s.stats.StartTime)

	s.logger.Debug("Streamer stopped", "duration", s.stats.TotalDuration)
	return nil
}

// WriteStdout writes data to stdout stream
func (s *Streamer) WriteStdout(data []byte) (int, error) {
	if !s.running {
		return 0, fmt.Errorf("streamer not running")
	}

	s.stats.StdoutBytes += int64(len(data))
	s.stats.LastWrite = time.Now()

	if s.options.LineBuffering {
		return s.stdoutBuf.Write(data)
	}

	return s.writeToStdout(data)
}

// WriteStderr writes data to stderr stream
func (s *Streamer) WriteStderr(data []byte) (int, error) {
	if !s.running {
		return 0, fmt.Errorf("streamer not running")
	}

	s.stats.StderrBytes += int64(len(data))
	s.stats.LastWrite = time.Now()

	if s.options.LineBuffering {
		return s.stderrBuf.Write(data)
	}

	return s.writeToStderr(data)
}

// SetStdout sets the stdout writer
func (s *Streamer) SetStdout(w io.Writer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stdout = w
}

// SetStderr sets the stderr writer
func (s *Streamer) SetStderr(w io.Writer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stderr = w
}

// SetFile sets the file writer
func (s *Streamer) SetFile(w io.Writer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.file = w
}

// Flush flushes all pending output
func (s *Streamer) Flush() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.flush()
}

// Close closes the streamer
func (s *Streamer) Close() error {
	return s.Stop()
}

// GetStats returns streaming statistics
func (s *Streamer) GetStats() *StreamerStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Return a copy to prevent concurrent access issues
	stats := *s.stats
	return &stats
}

// Private methods

// processStdoutLines processes stdout lines from the buffer
func (s *Streamer) processStdoutLines() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		case line, ok := <-s.stdoutBuf.lines:
			if !ok {
				return
			}

			// Apply stdout filter if configured
			if s.options.StdoutFilter != nil && !s.options.StdoutFilter(line) {
				s.stats.DroppedLines++
				continue
			}

			// Process and write the line
			if _, err := s.writeProcessedLine(line, false); err != nil {
				s.logger.Warn("Failed to write stdout line", "error", err)
			}

			s.stats.StdoutLines++
		}
	}
}

// processStderrLines processes stderr lines from the buffer
func (s *Streamer) processStderrLines() {
	defer s.wg.Done()

	for {
		select {
		case <-s.ctx.Done():
			return
		case line, ok := <-s.stderrBuf.lines:
			if !ok {
				return
			}

			// Apply stderr filter if configured
			if s.options.StderrFilter != nil && !s.options.StderrFilter(line) {
				s.stats.DroppedLines++
				continue
			}

			// Process and write the line
			if _, err := s.writeProcessedLine(line, true); err != nil {
				s.logger.Warn("Failed to write stderr line", "error", err)
			}

			s.stats.StderrLines++
		}
	}
}

// processStdoutLine processes a single stdout line
func (s *Streamer) processStdoutLine(line string) string {
	return s.formatLine(line, false)
}

// processStderrLine processes a single stderr line
func (s *Streamer) processStderrLine(line string) string {
	return s.formatLine(line, true)
}

// formatLine formats a line with timestamps, prefixes, and colors
func (s *Streamer) formatLine(line string, isStderr bool) string {
	var result strings.Builder

	// Add timestamp if enabled
	if s.options.Timestamps {
		timestamp := time.Now().Format("15:04:05.000")
		if s.options.Colors {
			result.WriteString(fmt.Sprintf("\033[90m%s\033[0m ", timestamp))
		} else {
			result.WriteString(fmt.Sprintf("%s ", timestamp))
		}
	}

	// Add task prefix if enabled
	if s.options.TaskPrefix {
		if s.options.Colors {
			if isStderr {
				result.WriteString(fmt.Sprintf("\033[31m[%s]\033[0m ", s.taskID))
			} else {
				result.WriteString(fmt.Sprintf("\033[36m[%s]\033[0m ", s.taskID))
			}
		} else {
			result.WriteString(fmt.Sprintf("[%s] ", s.taskID))
		}
	}

	// Add the line content
	if s.options.Colors && isStderr {
		result.WriteString(fmt.Sprintf("\033[31m%s\033[0m", line))
	} else {
		result.WriteString(line)
	}

	// Ensure line ends with newline
	if !strings.HasSuffix(line, "\n") {
		result.WriteString("\n")
	}

	return result.String()
}

// writeProcessedLine writes a processed line to the appropriate outputs
func (s *Streamer) writeProcessedLine(line string, isStderr bool) (int, error) {
	// Truncate line if maximum length is set
	if s.options.MaxLineLength > 0 && len(line) > s.options.MaxLineLength {
		line = line[:s.options.MaxLineLength] + "...\n"
	}

	data := []byte(line)

	if isStderr {
		return s.writeToStderr(data)
	} else {
		return s.writeToStdout(data)
	}
}

// writeToStdout writes data to stdout destinations
func (s *Streamer) writeToStdout(data []byte) (int, error) {
	var totalWritten int
	var lastErr error

	// Write to primary stdout
	if s.stdout != nil {
		n, err := s.stdout.Write(data)
		totalWritten += n
		if err != nil {
			lastErr = err
		}
	}

	// Write to file if configured and tee is enabled
	if s.file != nil && (s.options.Tee || s.stdout == nil) {
		n, err := s.file.Write(data)
		if s.stdout == nil {
			totalWritten += n
		}
		if err != nil {
			lastErr = err
		}
	}

	return totalWritten, lastErr
}

// writeToStderr writes data to stderr destinations
func (s *Streamer) writeToStderr(data []byte) (int, error) {
	var totalWritten int
	var lastErr error

	// Write to primary stderr
	if s.stderr != nil {
		n, err := s.stderr.Write(data)
		totalWritten += n
		if err != nil {
			lastErr = err
		}
	}

	// Write to file if configured and tee is enabled
	if s.file != nil && (s.options.Tee || s.stderr == nil) {
		n, err := s.file.Write(data)
		if s.stderr == nil {
			totalWritten += n
		}
		if err != nil {
			lastErr = err
		}
	}

	return totalWritten, lastErr
}

// flushRoutine runs periodic flushing in the background
func (s *Streamer) flushRoutine() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.options.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			s.flush()
			s.mu.Unlock()
		}
	}
}

// flush flushes all writers (must be called with mutex held)
func (s *Streamer) flush() error {
	var lastErr error

	// Flush stdout
	if flusher, ok := s.stdout.(interface{ Flush() error }); ok {
		if err := flusher.Flush(); err != nil {
			lastErr = err
		}
	}

	// Flush stderr
	if flusher, ok := s.stderr.(interface{ Flush() error }); ok {
		if err := flusher.Flush(); err != nil {
			lastErr = err
		}
	}

	// Flush file
	if flusher, ok := s.file.(interface{ Flush() error }); ok {
		if err := flusher.Flush(); err != nil {
			lastErr = err
		}
	}

	if lastErr == nil {
		s.stats.FlushCount++
	}

	return lastErr
}

// LineBuffer implementation

// NewLineBuffer creates a new line buffer
func NewLineBuffer(maxSize int, processor func(string) string) *LineBuffer {
	return &LineBuffer{
		buffer:    make([]byte, 0, maxSize),
		lines:     make(chan string, 100), // Buffered channel for lines
		processor: processor,
		maxSize:   maxSize,
	}
}

// Write writes data to the line buffer
func (lb *LineBuffer) Write(data []byte) (int, error) {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	originalLen := len(data)

	for len(data) > 0 {
		// Find newline
		nlIndex := -1
		for i, b := range data {
			if b == '\n' {
				nlIndex = i
				break
			}
		}

		if nlIndex == -1 {
			// No newline found, add all data to buffer
			if len(lb.buffer)+len(data) > lb.maxSize {
				// Buffer would overflow, truncate
				available := lb.maxSize - len(lb.buffer)
				if available > 0 {
					lb.buffer = append(lb.buffer, data[:available]...)
				}
				break
			}
			lb.buffer = append(lb.buffer, data...)
			break
		}

		// Found newline, process line
		lineData := data[:nlIndex]
		if len(lb.buffer) > 0 {
			// Combine with buffered data
			line := string(append(lb.buffer, lineData...))
			lb.buffer = lb.buffer[:0] // Reset buffer

			// Process and send line
			if lb.processor != nil {
				line = lb.processor(line)
			}

			select {
			case lb.lines <- line:
			default:
				// Channel full, drop line
			}
		} else if len(lineData) > 0 {
			// Send line directly
			line := string(lineData)
			if lb.processor != nil {
				line = lb.processor(line)
			}

			select {
			case lb.lines <- line:
			default:
				// Channel full, drop line
			}
		}

		// Move to next part of data
		data = data[nlIndex+1:]
	}

	return originalLen, nil
}

// Close closes the line buffer
func (lb *LineBuffer) Close() {
	lb.mu.Lock()
	defer lb.mu.Unlock()

	// Send any remaining buffered data as a final line
	if len(lb.buffer) > 0 {
		line := string(lb.buffer)
		if lb.processor != nil {
			line = lb.processor(line)
		}

		select {
		case lb.lines <- line:
		default:
		}

		lb.buffer = lb.buffer[:0]
	}

	close(lb.lines)
}

// StreamerFactory creates streamers with common configurations
type StreamerFactory struct {
	defaultOptions *StreamerOptions
	logger         logger.Logger
}

// NewStreamerFactory creates a new streamer factory
func NewStreamerFactory(options *StreamerOptions, logger logger.Logger) *StreamerFactory {
	if options == nil {
		options = createDefaultStreamerOptions()
	}

	return &StreamerFactory{
		defaultOptions: options,
		logger:         logger,
	}
}

// CreateStreamer creates a new streamer with factory defaults
func (sf *StreamerFactory) CreateStreamer(taskID string, overrides *StreamerOptions) *Streamer {
	options := *sf.defaultOptions // Copy defaults

	// Apply overrides
	if overrides != nil {
		if overrides.BufferSize > 0 {
			options.BufferSize = overrides.BufferSize
		}
		if overrides.FlushInterval > 0 {
			options.FlushInterval = overrides.FlushInterval
		}
		// Apply other overrides as needed
	}

	return NewStreamer(taskID, &options, sf.logger)
}

// MultiStreamer handles streaming to multiple destinations
type MultiStreamer struct {
	streamers []*Streamer
	mu        sync.RWMutex
}

// NewMultiStreamer creates a new multi-streamer
func NewMultiStreamer(streamers ...*Streamer) *MultiStreamer {
	return &MultiStreamer{
		streamers: streamers,
	}
}

// AddStreamer adds a streamer to the multi-streamer
func (ms *MultiStreamer) AddStreamer(streamer *Streamer) {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	ms.streamers = append(ms.streamers, streamer)
}

// WriteStdout writes to all streamers' stdout
func (ms *MultiStreamer) WriteStdout(data []byte) (int, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	var lastErr error
	for _, streamer := range ms.streamers {
		if _, err := streamer.WriteStdout(data); err != nil {
			lastErr = err
		}
	}

	return len(data), lastErr
}

// WriteStderr writes to all streamers' stderr
func (ms *MultiStreamer) WriteStderr(data []byte) (int, error) {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	var lastErr error
	for _, streamer := range ms.streamers {
		if _, err := streamer.WriteStderr(data); err != nil {
			lastErr = err
		}
	}

	return len(data), lastErr
}

// Flush flushes all streamers
func (ms *MultiStreamer) Flush() error {
	ms.mu.RLock()
	defer ms.mu.RUnlock()

	var lastErr error
	for _, streamer := range ms.streamers {
		if err := streamer.Flush(); err != nil {
			lastErr = err
		}
	}

	return lastErr
}

// Close closes all streamers
func (ms *MultiStreamer) Close() error {
	ms.mu.Lock()
	defer ms.mu.Unlock()

	var lastErr error
	for _, streamer := range ms.streamers {
		if err := streamer.Close(); err != nil {
			lastErr = err
		}
	}

	return lastErr
}
