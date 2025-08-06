package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Logger defines the logging interface used throughout Wake
type Logger interface {
	// Debug logs debug-level messages
	Debug(msg string, args ...any)

	// Info logs info-level messages
	Info(msg string, args ...any)

	// Warn logs warning-level messages
	Warn(msg string, args ...any)

	// Error logs error-level messages
	Error(msg string, args ...any)

	// Fatal logs fatal-level messages and exits
	Fatal(msg string, args ...any)

	// With returns a logger with additional context
	With(args ...any) Logger

	// WithGroup returns a logger with a group name
	WithGroup(name string) Logger

	// SetLevel sets the logging level
	SetLevel(level Level)

	// GetLevel returns the current logging level
	GetLevel() Level

	// Close closes the logger and flushes any buffered logs
	Close() error
}

// ToSlogLevel converts our Level to slog.Level
func (l Level) ToSlogLevel() slog.Level {
	switch l {
	case DebugLevel:
		return slog.LevelDebug
	case InfoLevel:
		return slog.LevelInfo
	case WarnLevel:
		return slog.LevelWarn
	case ErrorLevel:
		return slog.LevelError
	case FatalLevel:
		return slog.LevelError + 4 // Higher than error
	default:
		return slog.LevelInfo
	}
}

// Config holds logger configuration
type Config struct {
	// Level is the minimum log level
	Level Level

	// Format specifies the log format (text, json)
	Format string

	// Output specifies where to write logs (stdout, stderr, file path)
	Output string

	// EnableCaller adds caller information to logs
	EnableCaller bool

	// EnableTimestamp adds timestamps to logs
	EnableTimestamp bool

	// TimeFormat specifies the timestamp format
	TimeFormat string

	// MaxFileSize is the maximum size of log files in MB
	MaxFileSize int64

	// MaxBackups is the maximum number of backup files
	MaxBackups int

	// MaxAge is the maximum age of log files in days
	MaxAge int

	// Compress enables compression of backup files
	Compress bool
}

// DefaultConfig returns a default logger configuration
func DefaultConfig() *Config {
	return &Config{
		Level:           InfoLevel,
		Format:          "text",
		Output:          "stderr",
		EnableCaller:    false,
		EnableTimestamp: true,
		TimeFormat:      time.RFC3339,
		MaxFileSize:     10, // 10 MB
		MaxBackups:      3,
		MaxAge:          7, // 7 days
		Compress:        true,
	}
}

// wakeLogger is the concrete implementation of Logger
type wakeLogger struct {
	slogger *slog.Logger
	config  *Config
	writer  io.WriteCloser
	level   *slog.LevelVar
}

// New creates a new logger with default configuration
func New() Logger {
	return NewWithConfig(DefaultConfig())
}

// NewWithConfig creates a new logger with the specified configuration
func NewWithConfig(config *Config) Logger {
	if config == nil {
		config = DefaultConfig()
	}

	// Create writer based on output configuration
	writer, err := createWriter(config)
	if err != nil {
		// Fallback to stderr if writer creation fails
		writer = os.Stderr
	}

	// Create level var for dynamic level changes
	levelVar := &slog.LevelVar{}
	levelVar.Set(config.Level.ToSlogLevel())

	// Create handler based on format
	var handler slog.Handler

	handlerOpts := &slog.HandlerOptions{
		Level:     levelVar,
		AddSource: config.EnableCaller,
	}

	switch strings.ToLower(config.Format) {
	case "json":
		handler = slog.NewJSONHandler(writer, handlerOpts)
	case "text", "":
		handler = slog.NewTextHandler(writer, handlerOpts)
	default:
		// Default to text format
		handler = slog.NewTextHandler(writer, handlerOpts)
	}

	// Create slog logger
	slogger := slog.New(handler)

	return &wakeLogger{
		slogger: slogger,
		config:  config,
		writer:  writer,
		level:   levelVar,
	}
}

// createWriter creates an appropriate writer based on the output configuration
func createWriter(config *Config) (io.WriteCloser, error) {
	switch strings.ToLower(config.Output) {
	case "stdout":
		return nopWriteCloser{os.Stdout}, nil
	case "stderr", "":
		return nopWriteCloser{os.Stderr}, nil
	default:
		// Treat as file path
		return createFileWriter(config)
	}
}

// createFileWriter creates a file writer with rotation if needed
func createFileWriter(config *Config) (io.WriteCloser, error) {
	// Ensure directory exists
	dir := filepath.Dir(config.Output)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	// For now, just create a simple file writer
	// In production, you might want to use a rotating file writer
	file, err := os.OpenFile(config.Output, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	return file, nil
}

// nopWriteCloser wraps an io.Writer to make it an io.WriteCloser
type nopWriteCloser struct {
	io.Writer
}

func (nopWriteCloser) Close() error { return nil }

// Debug logs debug-level messages
func (l *wakeLogger) Debug(msg string, args ...any) {
	l.slogger.Debug(msg, args...)
}

// Info logs info-level messages
func (l *wakeLogger) Info(msg string, args ...any) {
	l.slogger.Info(msg, args...)
}

// Warn logs warning-level messages
func (l *wakeLogger) Warn(msg string, args ...any) {
	l.slogger.Warn(msg, args...)
}

// Error logs error-level messages
func (l *wakeLogger) Error(msg string, args ...any) {
	l.slogger.Error(msg, args...)
}

// Fatal logs fatal-level messages and exits
func (l *wakeLogger) Fatal(msg string, args ...any) {
	l.slogger.Error(msg, args...) // Log as error since slog doesn't have fatal
	os.Exit(1)
}

// With returns a logger with additional context
func (l *wakeLogger) With(args ...any) Logger {
	return &wakeLogger{
		slogger: l.slogger.With(args...),
		config:  l.config,
		writer:  l.writer,
		level:   l.level,
	}
}

// WithGroup returns a logger with a group name
func (l *wakeLogger) WithGroup(name string) Logger {
	return &wakeLogger{
		slogger: l.slogger.WithGroup(name),
		config:  l.config,
		writer:  l.writer,
		level:   l.level,
	}
}

// SetLevel sets the logging level
func (l *wakeLogger) SetLevel(level Level) {
	l.level.Set(level.ToSlogLevel())
}

// GetLevel returns the current logging level
func (l *wakeLogger) GetLevel() Level {
	slogLevel := l.level.Level()

	switch {
	case slogLevel <= slog.LevelDebug:
		return DebugLevel
	case slogLevel <= slog.LevelInfo:
		return InfoLevel
	case slogLevel <= slog.LevelWarn:
		return WarnLevel
	case slogLevel <= slog.LevelError:
		return ErrorLevel
	default:
		return FatalLevel
	}
}

// Close closes the logger and flushes any buffered logs
func (l *wakeLogger) Close() error {
	if l.writer != nil {
		return l.writer.Close()
	}
	return nil
}

// NoopLogger is a logger that does nothing (useful for testing)
type NoopLogger struct{}

// NewNoop creates a new no-op logger
func NewNoop() Logger {
	return &NoopLogger{}
}

func (NoopLogger) Debug(msg string, args ...any)  {}
func (NoopLogger) Info(msg string, args ...any)   {}
func (NoopLogger) Warn(msg string, args ...any)   {}
func (NoopLogger) Error(msg string, args ...any)  {}
func (NoopLogger) Fatal(msg string, args ...any)  { os.Exit(1) }
func (n NoopLogger) With(args ...any) Logger      { return n }
func (n NoopLogger) WithGroup(name string) Logger { return n }
func (NoopLogger) SetLevel(level Level)           {}
func (NoopLogger) GetLevel() Level                { return InfoLevel }
func (NoopLogger) Close() error                   { return nil }

// TestLogger is a logger that captures logs for testing
type TestLogger struct {
	logs   []LogEntry
	level  Level
	groups []string
	attrs  []any
}

// LogEntry represents a captured log entry
type LogEntry struct {
	Level   Level
	Message string
	Args    []any
	Groups  []string
}

// NewTest creates a new test logger
func NewTest() *TestLogger {
	return &TestLogger{
		logs:  make([]LogEntry, 0),
		level: DebugLevel,
	}
}

// Debug logs debug-level messages
func (t *TestLogger) Debug(msg string, args ...any) {
	t.log(DebugLevel, msg, args...)
}

// Info logs info-level messages
func (t *TestLogger) Info(msg string, args ...any) {
	t.log(InfoLevel, msg, args...)
}

// Warn logs warning-level messages
func (t *TestLogger) Warn(msg string, args ...any) {
	t.log(WarnLevel, msg, args...)
}

// Error logs error-level messages
func (t *TestLogger) Error(msg string, args ...any) {
	t.log(ErrorLevel, msg, args...)
}

// Fatal logs fatal-level messages (doesn't exit in test mode)
func (t *TestLogger) Fatal(msg string, args ...any) {
	t.log(FatalLevel, msg, args...)
}

// With returns a logger with additional context
func (t *TestLogger) With(args ...any) Logger {
	newAttrs := make([]any, len(t.attrs)+len(args))
	copy(newAttrs, t.attrs)
	copy(newAttrs[len(t.attrs):], args)

	return &TestLogger{
		logs:   t.logs,
		level:  t.level,
		groups: t.groups,
		attrs:  newAttrs,
	}
}

// WithGroup returns a logger with a group name
func (t *TestLogger) WithGroup(name string) Logger {
	newGroups := make([]string, len(t.groups)+1)
	copy(newGroups, t.groups)
	newGroups[len(t.groups)] = name

	return &TestLogger{
		logs:   t.logs,
		level:  t.level,
		groups: newGroups,
		attrs:  t.attrs,
	}
}

// SetLevel sets the logging level
func (t *TestLogger) SetLevel(level Level) {
	t.level = level
}

// GetLevel returns the current logging level
func (t *TestLogger) GetLevel() Level {
	return t.level
}

// Close closes the logger
func (t *TestLogger) Close() error {
	return nil
}

// log adds a log entry if the level is appropriate
func (t *TestLogger) log(level Level, msg string, args ...any) {
	if level < t.level {
		return
	}

	// Combine context attributes with message attributes
	allArgs := make([]any, len(t.attrs)+len(args))
	copy(allArgs, t.attrs)
	copy(allArgs[len(t.attrs):], args)

	entry := LogEntry{
		Level:   level,
		Message: msg,
		Args:    allArgs,
		Groups:  make([]string, len(t.groups)),
	}
	copy(entry.Groups, t.groups)

	t.logs = append(t.logs, entry)
}

// GetLogs returns all captured log entries
func (t *TestLogger) GetLogs() []LogEntry {
	return t.logs
}

// GetLogsForLevel returns log entries for a specific level
func (t *TestLogger) GetLogsForLevel(level Level) []LogEntry {
	var filtered []LogEntry
	for _, entry := range t.logs {
		if entry.Level == level {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

// Clear clears all captured logs
func (t *TestLogger) Clear() {
	t.logs = t.logs[:0]
}

// HasLog checks if a log with the given message exists
func (t *TestLogger) HasLog(level Level, message string) bool {
	for _, entry := range t.logs {
		if entry.Level == level && entry.Message == message {
			return true
		}
	}
	return false
}

// LogCount returns the number of captured logs
func (t *TestLogger) LogCount() int {
	return len(t.logs)
}

// LogCountForLevel returns the number of logs for a specific level
func (t *TestLogger) LogCountForLevel(level Level) int {
	count := 0
	for _, entry := range t.logs {
		if entry.Level == level {
			count++
		}
	}
	return count
}
