package output

import (
	"bytes"
	"fmt"
	"io"
	"sync"
	"time"
)

// Buffer represents a thread-safe output buffer for task execution
type Buffer struct {
	// Data storage
	stdoutBuf *CircularBuffer
	stderrBuf *CircularBuffer

	// Metadata
	taskID    string
	startTime time.Time

	// State tracking
	closed bool

	// Statistics
	stats *BufferStats

	// Synchronization
	mu sync.RWMutex

	// Capacity
	maxSize int
}

// BufferStats tracks buffer usage statistics
type BufferStats struct {
	// Data counters
	StdoutBytes  int64
	StderrBytes  int64
	StdoutWrites int64
	StderrWrites int64

	// Buffer state
	StdoutSize    int
	StderrSize    int
	StdoutWrapped bool
	StderrWrapped bool

	// Timing
	FirstWrite time.Time
	LastWrite  time.Time

	// Operations
	ReadOps  int64
	WriteOps int64
	FlushOps int64
}

// CircularBuffer implements a thread-safe circular buffer
type CircularBuffer struct {
	buffer   []byte
	start    int
	end      int
	size     int
	capacity int
	wrapped  bool
	mu       sync.RWMutex
}

// BufferEntry represents a single buffer entry with metadata
type BufferEntry struct {
	Timestamp time.Time
	Data      []byte
	IsStderr  bool
	Size      int
}

// BufferReader provides a reader interface for the buffer
type BufferReader struct {
	buffer   *Buffer
	position int
	isStderr bool
}

// NewBuffer creates a new output buffer
func NewBuffer(maxSize int) *Buffer {
	if maxSize <= 0 {
		maxSize = 64 * 1024 // Default 64KB
	}

	// Split capacity between stdout and stderr
	halfSize := maxSize / 2

	return &Buffer{
		stdoutBuf: NewCircularBuffer(halfSize),
		stderrBuf: NewCircularBuffer(halfSize),
		startTime: time.Now(),
		stats:     &BufferStats{},
		maxSize:   maxSize,
	}
}

// NewBufferWithTaskID creates a new buffer with a task ID
func NewBufferWithTaskID(taskID string, maxSize int) *Buffer {
	buffer := NewBuffer(maxSize)
	buffer.taskID = taskID
	return buffer
}

// WriteStdout writes data to the stdout buffer
func (b *Buffer) WriteStdout(data []byte) (int, error) {
	if b.isClosed() {
		return 0, fmt.Errorf("buffer is closed")
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// Update statistics
	b.updateWriteStats(len(data), false)

	// Write to stdout buffer
	n, err := b.stdoutBuf.Write(data)

	// Update buffer stats
	b.stats.StdoutSize = b.stdoutBuf.Size()
	b.stats.StdoutWrapped = b.stdoutBuf.HasWrapped()

	return n, err
}

// WriteStderr writes data to the stderr buffer
func (b *Buffer) WriteStderr(data []byte) (int, error) {
	if b.isClosed() {
		return 0, fmt.Errorf("buffer is closed")
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// Update statistics
	b.updateWriteStats(len(data), true)

	// Write to stderr buffer
	n, err := b.stderrBuf.Write(data)

	// Update buffer stats
	b.stats.StderrSize = b.stderrBuf.Size()
	b.stats.StderrWrapped = b.stderrBuf.HasWrapped()

	return n, err
}

// Write implements io.Writer interface (writes to stdout by default)
func (b *Buffer) Write(data []byte) (int, error) {
	return b.WriteStdout(data)
}

// ReadStdout reads all available stdout data
func (b *Buffer) ReadStdout() ([]byte, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	b.stats.ReadOps++
	return b.stdoutBuf.ReadAll(), nil
}

// ReadStderr reads all available stderr data
func (b *Buffer) ReadStderr() ([]byte, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	b.stats.ReadOps++
	return b.stderrBuf.ReadAll(), nil
}

// Read reads all available data from both stdout and stderr
func (b *Buffer) Read() ([]byte, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	b.stats.ReadOps++

	stdoutData := b.stdoutBuf.ReadAll()
	stderrData := b.stderrBuf.ReadAll()

	if len(stdoutData) == 0 && len(stderrData) == 0 {
		return nil, io.EOF
	}

	// Combine stdout and stderr data
	var result bytes.Buffer
	result.Write(stdoutData)
	result.Write(stderrData)

	return result.Bytes(), nil
}

// ReadLines reads data as lines with metadata
func (b *Buffer) ReadLines() ([]*BufferEntry, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	var entries []*BufferEntry

	// Read stdout lines
	stdoutData := b.stdoutBuf.ReadAll()
	if len(stdoutData) > 0 {
		lines := bytes.Split(stdoutData, []byte{'\n'})
		for _, line := range lines {
			if len(line) > 0 || len(lines) == 1 { // Include empty lines only if it's the only line
				entries = append(entries, &BufferEntry{
					Timestamp: time.Now(),
					Data:      line,
					IsStderr:  false,
					Size:      len(line),
				})
			}
		}
	}

	// Read stderr lines
	stderrData := b.stderrBuf.ReadAll()
	if len(stderrData) > 0 {
		lines := bytes.Split(stderrData, []byte{'\n'})
		for _, line := range lines {
			if len(line) > 0 || len(lines) == 1 {
				entries = append(entries, &BufferEntry{
					Timestamp: time.Now(),
					Data:      line,
					IsStderr:  true,
					Size:      len(line),
				})
			}
		}
	}

	return entries, nil
}

// Peek reads data without consuming it
func (b *Buffer) Peek(n int) ([]byte, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	stdoutData := b.stdoutBuf.Peek(n)
	if len(stdoutData) >= n {
		return stdoutData[:n], nil
	}

	// If stdout doesn't have enough data, include stderr
	remaining := n - len(stdoutData)
	stderrData := b.stderrBuf.Peek(remaining)

	var result bytes.Buffer
	result.Write(stdoutData)
	result.Write(stderrData)

	return result.Bytes(), nil
}

// PeekStdout peeks at stdout data without consuming it
func (b *Buffer) PeekStdout(n int) ([]byte, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.stdoutBuf.Peek(n), nil
}

// PeekStderr peeks at stderr data without consuming it
func (b *Buffer) PeekStderr(n int) ([]byte, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.stderrBuf.Peek(n), nil
}

// Size returns the total size of buffered data
func (b *Buffer) Size() int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.stdoutBuf.Size() + b.stderrBuf.Size()
}

// StdoutSize returns the size of stdout buffer
func (b *Buffer) StdoutSize() int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.stdoutBuf.Size()
}

// StderrSize returns the size of stderr buffer
func (b *Buffer) StderrSize() int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.stderrBuf.Size()
}

// Clear clears all buffered data
func (b *Buffer) Clear() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.stdoutBuf.Clear()
	b.stderrBuf.Clear()

	// Reset statistics
	b.stats.StdoutSize = 0
	b.stats.StderrSize = 0
	b.stats.StdoutWrapped = false
	b.stats.StderrWrapped = false
}

// ClearStdout clears only stdout buffer
func (b *Buffer) ClearStdout() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.stdoutBuf.Clear()
	b.stats.StdoutSize = 0
	b.stats.StdoutWrapped = false
}

// ClearStderr clears only stderr buffer
func (b *Buffer) ClearStderr() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.stderrBuf.Clear()
	b.stats.StderrSize = 0
	b.stats.StderrWrapped = false
}

// Close closes the buffer
func (b *Buffer) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.closed = true
	return nil
}

// IsClosed returns whether the buffer is closed
func (b *Buffer) IsClosed() bool {
	return b.isClosed()
}

// GetStats returns buffer statistics
func (b *Buffer) GetStats() *BufferStats {
	b.mu.RLock()
	defer b.mu.RUnlock()

	// Return a copy to prevent concurrent access issues
	stats := *b.stats
	return &stats
}

// GetTaskID returns the task ID associated with this buffer
func (b *Buffer) GetTaskID() string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.taskID
}

// NewReader creates a new reader for the buffer
func (b *Buffer) NewReader(isStderr bool) *BufferReader {
	return &BufferReader{
		buffer:   b,
		position: 0,
		isStderr: isStderr,
	}
}

// Private methods

// isClosed checks if buffer is closed (not thread-safe, use with locks)
func (b *Buffer) isClosed() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.closed
}

// updateWriteStats updates write statistics
func (b *Buffer) updateWriteStats(size int, isStderr bool) {
	now := time.Now()

	if b.stats.FirstWrite.IsZero() {
		b.stats.FirstWrite = now
	}
	b.stats.LastWrite = now
	b.stats.WriteOps++

	if isStderr {
		b.stats.StderrBytes += int64(size)
		b.stats.StderrWrites++
	} else {
		b.stats.StdoutBytes += int64(size)
		b.stats.StdoutWrites++
	}
}

// CircularBuffer implementation

// NewCircularBuffer creates a new circular buffer
func NewCircularBuffer(capacity int) *CircularBuffer {
	return &CircularBuffer{
		buffer:   make([]byte, capacity),
		capacity: capacity,
	}
}

// Write writes data to the circular buffer
func (cb *CircularBuffer) Write(data []byte) (int, error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if len(data) == 0 {
		return 0, nil
	}

	written := 0
	for _, b := range data {
		cb.buffer[cb.end] = b
		cb.end = (cb.end + 1) % cb.capacity
		written++

		if cb.end == cb.start {
			// Buffer is full, advance start pointer (overwrite old data)
			cb.start = (cb.start + 1) % cb.capacity
			cb.wrapped = true
		} else if cb.size < cb.capacity {
			cb.size++
		}
	}

	return written, nil
}

// ReadAll reads all available data from the buffer
func (cb *CircularBuffer) ReadAll() []byte {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if cb.size == 0 {
		return nil
	}

	result := make([]byte, cb.size)

	if cb.start < cb.end {
		// Data is contiguous
		copy(result, cb.buffer[cb.start:cb.end])
	} else {
		// Data wraps around
		n := copy(result, cb.buffer[cb.start:])
		copy(result[n:], cb.buffer[:cb.end])
	}

	return result
}

// Peek reads up to n bytes without consuming them
func (cb *CircularBuffer) Peek(n int) []byte {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	if cb.size == 0 || n <= 0 {
		return nil
	}

	readSize := n
	if readSize > cb.size {
		readSize = cb.size
	}

	result := make([]byte, readSize)

	if cb.start < cb.end {
		// Data is contiguous
		copy(result, cb.buffer[cb.start:cb.start+readSize])
	} else {
		// Data wraps around
		firstPart := cb.capacity - cb.start
		if firstPart >= readSize {
			copy(result, cb.buffer[cb.start:cb.start+readSize])
		} else {
			copy(result, cb.buffer[cb.start:])
			copy(result[firstPart:], cb.buffer[:readSize-firstPart])
		}
	}

	return result
}

// Size returns the current size of data in the buffer
func (cb *CircularBuffer) Size() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.size
}

// Capacity returns the buffer capacity
func (cb *CircularBuffer) Capacity() int {
	return cb.capacity
}

// HasWrapped returns whether the buffer has wrapped around
func (cb *CircularBuffer) HasWrapped() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.wrapped
}

// Clear clears all data from the buffer
func (cb *CircularBuffer) Clear() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.start = 0
	cb.end = 0
	cb.size = 0
	cb.wrapped = false
}

// BufferReader implementation

// Read implements io.Reader interface
func (br *BufferReader) Read(p []byte) (int, error) {
	var data []byte
	var err error

	if br.isStderr {
		data, err = br.buffer.ReadStderr()
	} else {
		data, err = br.buffer.ReadStdout()
	}

	if err != nil {
		return 0, err
	}

	if len(data) == 0 {
		return 0, io.EOF
	}

	// Copy data to provided buffer
	n := copy(p, data)
	br.position += n

	if n < len(data) {
		// Not all data was copied, this is a simplified implementation
		// In a real implementation, we'd need to track position within the buffer
		return n, nil
	}

	return n, nil
}

// BufferPool manages a pool of reusable buffers
type BufferPool struct {
	pool     sync.Pool
	maxSize  int
	poolSize int
	mu       sync.Mutex
}

// NewBufferPool creates a new buffer pool
func NewBufferPool(maxSize, poolSize int) *BufferPool {
	return &BufferPool{
		pool: sync.Pool{
			New: func() any {
				return NewBuffer(maxSize)
			},
		},
		maxSize:  maxSize,
		poolSize: poolSize,
	}
}

// Get gets a buffer from the pool
func (bp *BufferPool) Get() *Buffer {
	buffer := bp.pool.Get().(*Buffer)
	buffer.Clear() // Clear any previous data
	return buffer
}

// Put returns a buffer to the pool
func (bp *BufferPool) Put(buffer *Buffer) {
	if buffer == nil {
		return
	}

	// Clear the buffer before returning to pool
	buffer.Clear()
	bp.pool.Put(buffer)
}

// BufferManager manages multiple buffers
type BufferManager struct {
	buffers map[string]*Buffer
	pool    *BufferPool
	mu      sync.RWMutex
}

// NewBufferManager creates a new buffer manager
func NewBufferManager(defaultSize int) *BufferManager {
	return &BufferManager{
		buffers: make(map[string]*Buffer),
		pool:    NewBufferPool(defaultSize, 10),
	}
}

// GetOrCreateBuffer gets an existing buffer or creates a new one
func (bm *BufferManager) GetOrCreateBuffer(taskID string) *Buffer {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if buffer, exists := bm.buffers[taskID]; exists {
		return buffer
	}

	buffer := bm.pool.Get()
	buffer.taskID = taskID
	bm.buffers[taskID] = buffer

	return buffer
}

// GetBuffer gets an existing buffer
func (bm *BufferManager) GetBuffer(taskID string) (*Buffer, bool) {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	buffer, exists := bm.buffers[taskID]
	return buffer, exists
}

// RemoveBuffer removes and returns a buffer to the pool
func (bm *BufferManager) RemoveBuffer(taskID string) {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	if buffer, exists := bm.buffers[taskID]; exists {
		delete(bm.buffers, taskID)
		bm.pool.Put(buffer)
	}
}

// GetAllBuffers returns a copy of all buffers
func (bm *BufferManager) GetAllBuffers() map[string]*Buffer {
	bm.mu.RLock()
	defer bm.mu.RUnlock()

	result := make(map[string]*Buffer)
	for k, v := range bm.buffers {
		result[k] = v
	}

	return result
}

// Clear clears all buffers
func (bm *BufferManager) Clear() {
	bm.mu.Lock()
	defer bm.mu.Unlock()

	for taskID, buffer := range bm.buffers {
		bm.pool.Put(buffer)
		delete(bm.buffers, taskID)
	}
}
