package logger

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode"
)

// Formatter defines the interface for log formatters
type Formatter interface {
	Format(entry *Entry) ([]byte, error)
}

// Entry represents a log entry
type Entry struct {
	Level     Level          `json:"level"`
	Time      time.Time      `json:"time"`
	Message   string         `json:"message"`
	Fields    map[string]any `json:"fields,omitempty"`
	Caller    *Caller        `json:"caller,omitempty"`
	Error     error          `json:"error,omitempty"`
	Logger    string         `json:"logger,omitempty"`
	Component string         `json:"component,omitempty"`
}

// Caller represents caller information
type Caller struct {
	File     string `json:"file"`
	Line     int    `json:"line"`
	Function string `json:"function"`
}

// TextFormatter formats logs as human-readable text
type TextFormatter struct {
	// Time format
	TimeFormat string

	// Colors
	DisableColors             bool
	ForceColors               bool
	EnvironmentOverrideColors bool

	// Layout
	DisableTimestamp bool
	FullTimestamp    bool
	DisableCaller    bool
	CallerPrettyfier func(*runtime.Frame) (function string, file string)

	// Field formatting
	DisableQuote     bool
	QuoteEmptyFields bool
	FieldMap         FieldMap

	// Sorting
	DisableSorting bool
	SortingFunc    func([]string)

	// Padding
	PadLevelText bool

	// Custom
	CustomFormat func(*Entry) string
}

// JSONFormatter formats logs as JSON
type JSONFormatter struct {
	// Time format
	TimeFormat       string
	DisableTimestamp bool

	// Field names
	FieldMap FieldMap

	// Formatting
	DisableHTMLEscape bool
	PrettyPrint       bool

	// Custom
	DataKey      string
	CustomFields func(*Entry) map[string]any
}

// LogfmtFormatter formats logs in logfmt format
type LogfmtFormatter struct {
	// Time format
	TimeFormat       string
	DisableTimestamp bool

	// Field names
	FieldMap FieldMap

	// Formatting
	DisableQuote     bool
	QuoteEmptyFields bool

	// Sorting
	DisableSorting bool
	SortingFunc    func([]string)
}

// ConsoleFormatter formats logs for console output with colors and icons
type ConsoleFormatter struct {
	// Colors
	DisableColors bool
	ForceColors   bool

	// Time format
	TimeFormat       string
	DisableTimestamp bool

	// Layout
	ShowCaller    bool
	ShowLevel     bool
	ShowLogger    bool
	ShowComponent bool

	// Icons and symbols
	UseIcons   bool
	LevelIcons map[Level]string

	// Formatting
	MessageMaxWidth int
	FieldMaxWidth   int
	IndentSize      int

	// Custom
	CustomFormat func(*Entry) string
}

// FieldMap allows customization of field names
type FieldMap map[string]string

// Level represents log levels
type Level int

const (
	PanicLevel Level = iota
	FatalLevel
	ErrorLevel
	WarnLevel
	InfoLevel
	DebugLevel
	TraceLevel
)

// String returns the string representation of the level
func (level Level) String() string {
	switch level {
	case TraceLevel:
		return "trace"
	case DebugLevel:
		return "debug"
	case InfoLevel:
		return "info"
	case WarnLevel:
		return "warn"
	case ErrorLevel:
		return "error"
	case FatalLevel:
		return "fatal"
	case PanicLevel:
		return "panic"
	}
	return "unknown"
}

// Color codes for different levels
const (
	ColorReset  = "\033[0m"
	ColorBold   = "\033[1m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorWhite  = "\033[37m"
	ColorGray   = "\033[90m"
)

// Default field names
const (
	DefaultTimestampFormat = time.RFC3339
	DefaultLevelKey        = "level"
	DefaultTimeKey         = "time"
	DefaultMessageKey      = "msg"
	DefaultErrorKey        = "error"
	DefaultCallerKey       = "caller"
	DefaultLoggerKey       = "logger"
	DefaultComponentKey    = "component"
)

// NewTextFormatter creates a new text formatter
func NewTextFormatter() *TextFormatter {
	return &TextFormatter{
		TimeFormat:    DefaultTimestampFormat,
		DisableColors: false,
		FullTimestamp: true,
		PadLevelText:  true,
		FieldMap:      make(FieldMap),
	}
}

// Format formats a log entry as text
func (f *TextFormatter) Format(entry *Entry) ([]byte, error) {
	var b strings.Builder

	prefixFieldClashes(entry, f.FieldMap)

	// Custom format override
	if f.CustomFormat != nil {
		return []byte(f.CustomFormat(entry)), nil
	}

	// Determine if we should use colors
	useColors := f.shouldUseColors()

	// Write timestamp
	if !f.DisableTimestamp {
		f.writeTimestamp(&b, entry, useColors)
	}

	// Write level
	f.writeLevel(&b, entry, useColors)

	// Write caller
	if !f.DisableCaller && entry.Caller != nil {
		f.writeCaller(&b, entry, useColors)
	}

	// Write logger/component
	f.writeLoggerInfo(&b, entry, useColors)

	// Write message
	f.writeMessage(&b, entry, useColors)

	// Write fields
	f.writeFields(&b, entry, useColors)

	// Write error
	if entry.Error != nil {
		f.writeError(&b, entry, useColors)
	}

	b.WriteByte('\n')
	return []byte(b.String()), nil
}

// shouldUseColors determines if colors should be used
func (f *TextFormatter) shouldUseColors() bool {
	if f.ForceColors {
		return true
	}
	if f.DisableColors {
		return false
	}
	// Could add terminal detection logic here
	return true
}

// writeTimestamp writes the timestamp
func (f *TextFormatter) writeTimestamp(b *strings.Builder, entry *Entry, useColors bool) {
	timeFormat := f.TimeFormat
	if timeFormat == "" {
		timeFormat = DefaultTimestampFormat
	}

	timestamp := entry.Time.Format(timeFormat)

	if useColors {
		b.WriteString(ColorGray)
	}
	b.WriteString(timestamp)
	if useColors {
		b.WriteString(ColorReset)
	}
	b.WriteByte(' ')
}

// writeLevel writes the log level
func (f *TextFormatter) writeLevel(b *strings.Builder, entry *Entry, useColors bool) {
	levelText := strings.ToUpper(entry.Level.String())

	if f.PadLevelText {
		levelText = fmt.Sprintf("%-5s", levelText)
	}

	if useColors {
		color := f.getLevelColor(entry.Level)
		b.WriteString(color)
		b.WriteString(ColorBold)
	}

	b.WriteString(levelText)

	if useColors {
		b.WriteString(ColorReset)
	}
	b.WriteByte(' ')
}

// writeCaller writes caller information
func (f *TextFormatter) writeCaller(b *strings.Builder, entry *Entry, useColors bool) {
	caller := entry.Caller

	file := caller.File
	if f.CallerPrettyfier != nil {
		function, file := f.CallerPrettyfier(&runtime.Frame{
			File:     caller.File,
			Line:     caller.Line,
			Function: caller.Function,
		})
		if function != "" {
			caller.Function = function
		}
		if file != "" {
			caller.File = file
		}
	} else {
		// Default prettification
		file = filepath.Base(caller.File)
	}

	if useColors {
		b.WriteString(ColorCyan)
	}

	fmt.Fprintf(b, "%s:%d", file, caller.Line)

	if useColors {
		b.WriteString(ColorReset)
	}
	b.WriteByte(' ')
}

// writeLoggerInfo writes logger and component information
func (f *TextFormatter) writeLoggerInfo(b *strings.Builder, entry *Entry, useColors bool) {
	if entry.Logger != "" || entry.Component != "" {
		if useColors {
			b.WriteString(ColorPurple)
		}

		b.WriteByte('[')
		if entry.Logger != "" {
			b.WriteString(entry.Logger)
			if entry.Component != "" {
				b.WriteByte(':')
				b.WriteString(entry.Component)
			}
		} else if entry.Component != "" {
			b.WriteString(entry.Component)
		}
		b.WriteByte(']')

		if useColors {
			b.WriteString(ColorReset)
		}
		b.WriteByte(' ')
	}
}

// writeMessage writes the log message
func (f *TextFormatter) writeMessage(b *strings.Builder, entry *Entry, useColors bool) {
	if useColors && entry.Level <= ErrorLevel {
		b.WriteString(ColorBold)
	}

	b.WriteString(entry.Message)

	if useColors && entry.Level <= ErrorLevel {
		b.WriteString(ColorReset)
	}
}

// writeFields writes additional fields
func (f *TextFormatter) writeFields(b *strings.Builder, entry *Entry, useColors bool) {
	if len(entry.Fields) == 0 {
		return
	}

	// Get field keys
	keys := make([]string, 0, len(entry.Fields))
	for key := range entry.Fields {
		keys = append(keys, key)
	}

	// Sort if enabled
	if !f.DisableSorting {
		if f.SortingFunc != nil {
			f.SortingFunc(keys)
		} else {
			// Default sorting
			for i := 0; i < len(keys); i++ {
				for j := i + 1; j < len(keys); j++ {
					if keys[i] > keys[j] {
						keys[i], keys[j] = keys[j], keys[i]
					}
				}
			}
		}
	}

	// Write fields
	for i, key := range keys {
		if i == 0 {
			b.WriteByte(' ')
		}

		value := entry.Fields[key]

		if useColors {
			b.WriteString(ColorBlue)
		}
		b.WriteString(key)
		if useColors {
			b.WriteString(ColorReset)
		}
		b.WriteByte('=')

		f.writeValue(b, value, useColors)

		if i < len(keys)-1 {
			b.WriteByte(' ')
		}
	}
}

// writeError writes error information
func (f *TextFormatter) writeError(b *strings.Builder, entry *Entry, useColors bool) {
	b.WriteByte(' ')

	if useColors {
		b.WriteString(ColorRed)
		b.WriteString(ColorBold)
	}

	b.WriteString("error=")
	f.writeValue(b, entry.Error.Error(), useColors)

	if useColors {
		b.WriteString(ColorReset)
	}
}

// writeValue writes a field value
func (f *TextFormatter) writeValue(b *strings.Builder, value any, useColors bool) {
	stringVal := fmt.Sprintf("%v", value)

	if !f.needsQuoting(stringVal) {
		b.WriteString(stringVal)
	} else {
		if useColors {
			b.WriteString(ColorGreen)
		}
		b.WriteByte('"')
		b.WriteString(strings.ReplaceAll(stringVal, "\"", "\\\""))
		b.WriteByte('"')
		if useColors {
			b.WriteString(ColorReset)
		}
	}
}

// needsQuoting determines if a value needs quoting
func (f *TextFormatter) needsQuoting(text string) bool {
	if f.DisableQuote {
		return false
	}

	if text == "" {
		return f.QuoteEmptyFields
	}

	for _, char := range text {
		if !((char >= 'a' && char <= 'z') ||
			(char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') ||
			char == '-' || char == '.' || char == '_' || char == '/' || char == '@' || char == '^' || char == '+') {
			return true
		}
	}

	return false
}

// getLevelColor returns the color for a log level
func (f *TextFormatter) getLevelColor(level Level) string {
	switch level {
	case TraceLevel:
		return ColorPurple
	case DebugLevel:
		return ColorBlue
	case InfoLevel:
		return ColorGreen
	case WarnLevel:
		return ColorYellow
	case ErrorLevel:
		return ColorRed
	case FatalLevel:
		return ColorRed
	case PanicLevel:
		return ColorRed
	default:
		return ColorWhite
	}
}

// NewJSONFormatter creates a new JSON formatter
func NewJSONFormatter() *JSONFormatter {
	return &JSONFormatter{
		TimeFormat: DefaultTimestampFormat,
		FieldMap:   make(FieldMap),
	}
}

// Format formats a log entry as JSON
func (f *JSONFormatter) Format(entry *Entry) ([]byte, error) {
	data := make(map[string]any)

	prefixFieldClashes(entry, f.FieldMap)

	// Add timestamp
	if !f.DisableTimestamp {
		timeKey := f.FieldMap.resolve(DefaultTimeKey)
		if f.TimeFormat != "" {
			data[timeKey] = entry.Time.Format(f.TimeFormat)
		} else {
			data[timeKey] = entry.Time.Format(DefaultTimestampFormat)
		}
	}

	// Add level
	levelKey := f.FieldMap.resolve(DefaultLevelKey)
	data[levelKey] = entry.Level.String()

	// Add message
	messageKey := f.FieldMap.resolve(DefaultMessageKey)
	data[messageKey] = entry.Message

	// Add caller
	if entry.Caller != nil {
		callerKey := f.FieldMap.resolve(DefaultCallerKey)
		data[callerKey] = fmt.Sprintf("%s:%d", entry.Caller.File, entry.Caller.Line)
	}

	// Add logger info
	if entry.Logger != "" {
		loggerKey := f.FieldMap.resolve(DefaultLoggerKey)
		data[loggerKey] = entry.Logger
	}

	if entry.Component != "" {
		componentKey := f.FieldMap.resolve(DefaultComponentKey)
		data[componentKey] = entry.Component
	}

	// Add error
	if entry.Error != nil {
		errorKey := f.FieldMap.resolve(DefaultErrorKey)
		data[errorKey] = entry.Error.Error()
	}

	// Add fields
	for key, value := range entry.Fields {
		data[key] = value
	}

	// Add custom fields
	if f.CustomFields != nil {
		customFields := f.CustomFields(entry)
		for key, value := range customFields {
			data[key] = value
		}
	}

	// Wrap in data key if specified
	if f.DataKey != "" {
		data = map[string]any{
			f.DataKey: data,
		}
	}

	encoder := json.NewEncoder(&strings.Builder{})
	encoder.SetEscapeHTML(!f.DisableHTMLEscape)

	if f.PrettyPrint {
		return json.MarshalIndent(data, "", "  ")
	}

	return json.Marshal(data)
}

// NewLogfmtFormatter creates a new logfmt formatter
func NewLogfmtFormatter() *LogfmtFormatter {
	return &LogfmtFormatter{
		TimeFormat: DefaultTimestampFormat,
		FieldMap:   make(FieldMap),
	}
}

// Format formats a log entry in logfmt format
func (f *LogfmtFormatter) Format(entry *Entry) ([]byte, error) {
	var b strings.Builder

	prefixFieldClashes(entry, f.FieldMap)

	// Write timestamp
	if !f.DisableTimestamp {
		timeKey := f.FieldMap.resolve(DefaultTimeKey)
		timeFormat := f.TimeFormat
		if timeFormat == "" {
			timeFormat = DefaultTimestampFormat
		}
		f.writeKeyValue(&b, timeKey, entry.Time.Format(timeFormat), false)
	}

	// Write level
	levelKey := f.FieldMap.resolve(DefaultLevelKey)
	f.writeKeyValue(&b, levelKey, entry.Level.String(), true)

	// Write logger info
	if entry.Logger != "" {
		loggerKey := f.FieldMap.resolve(DefaultLoggerKey)
		f.writeKeyValue(&b, loggerKey, entry.Logger, true)
	}

	if entry.Component != "" {
		componentKey := f.FieldMap.resolve(DefaultComponentKey)
		f.writeKeyValue(&b, componentKey, entry.Component, true)
	}

	// Write caller
	if entry.Caller != nil {
		callerKey := f.FieldMap.resolve(DefaultCallerKey)
		caller := fmt.Sprintf("%s:%d", filepath.Base(entry.Caller.File), entry.Caller.Line)
		f.writeKeyValue(&b, callerKey, caller, true)
	}

	// Write message
	messageKey := f.FieldMap.resolve(DefaultMessageKey)
	f.writeKeyValue(&b, messageKey, entry.Message, true)

	// Write fields
	if len(entry.Fields) > 0 {
		keys := make([]string, 0, len(entry.Fields))
		for key := range entry.Fields {
			keys = append(keys, key)
		}

		// Sort if enabled
		if !f.DisableSorting {
			if f.SortingFunc != nil {
				f.SortingFunc(keys)
			} else {
				// Default sorting
				for i := 0; i < len(keys); i++ {
					for j := i + 1; j < len(keys); j++ {
						if keys[i] > keys[j] {
							keys[i], keys[j] = keys[j], keys[i]
						}
					}
				}
			}
		}

		for _, key := range keys {
			f.writeKeyValue(&b, key, entry.Fields[key], true)
		}
	}

	// Write error
	if entry.Error != nil {
		errorKey := f.FieldMap.resolve(DefaultErrorKey)
		f.writeKeyValue(&b, errorKey, entry.Error.Error(), true)
	}

	b.WriteByte('\n')
	return []byte(b.String()), nil
}

// writeKeyValue writes a key-value pair in logfmt format
func (f *LogfmtFormatter) writeKeyValue(b *strings.Builder, key string, value any, needSpace bool) {
	if needSpace && b.Len() > 0 {
		b.WriteByte(' ')
	}

	b.WriteString(key)
	b.WriteByte('=')

	stringVal := fmt.Sprintf("%v", value)

	if f.needsQuoting(stringVal) {
		b.WriteByte('"')
		b.WriteString(strings.ReplaceAll(stringVal, "\"", "\\\""))
		b.WriteByte('"')
	} else {
		b.WriteString(stringVal)
	}
}

// needsQuoting determines if a value needs quoting in logfmt
func (f *LogfmtFormatter) needsQuoting(text string) bool {
	if f.DisableQuote {
		return false
	}

	if text == "" {
		return f.QuoteEmptyFields
	}

	// Check for spaces or special characters
	for _, char := range text {
		if unicode.IsSpace(char) || char == '"' || char == '=' {
			return true
		}
	}

	return false
}

// NewConsoleFormatter creates a new console formatter
func NewConsoleFormatter() *ConsoleFormatter {
	return &ConsoleFormatter{
		TimeFormat:      "15:04:05",
		ShowLevel:       true,
		ShowCaller:      false,
		ShowLogger:      true,
		ShowComponent:   true,
		UseIcons:        true,
		MessageMaxWidth: 80,
		FieldMaxWidth:   20,
		IndentSize:      2,
		LevelIcons:      getDefaultLevelIcons(),
	}
}

// Format formats a log entry for console output
func (f *ConsoleFormatter) Format(entry *Entry) ([]byte, error) {
	var b strings.Builder

	// Custom format override
	if f.CustomFormat != nil {
		return []byte(f.CustomFormat(entry)), nil
	}

	useColors := f.shouldUseColors()

	// Write timestamp
	if !f.DisableTimestamp {
		f.writeConsoleTimestamp(&b, entry, useColors)
	}

	// Write level with icon
	if f.ShowLevel {
		f.writeConsoleLevel(&b, entry, useColors)
	}

	// Write logger/component
	if f.ShowLogger || f.ShowComponent {
		f.writeConsoleLoggerInfo(&b, entry, useColors)
	}

	// Write caller
	if f.ShowCaller && entry.Caller != nil {
		f.writeConsoleCaller(&b, entry, useColors)
	}

	// Write message
	f.writeConsoleMessage(&b, entry, useColors)

	// Write fields on new lines if present
	f.writeConsoleFields(&b, entry, useColors)

	// Write error
	if entry.Error != nil {
		f.writeConsoleError(&b, entry, useColors)
	}

	b.WriteByte('\n')
	return []byte(b.String()), nil
}

// shouldUseColors determines if colors should be used for console output
func (f *ConsoleFormatter) shouldUseColors() bool {
	if f.ForceColors {
		return true
	}
	if f.DisableColors {
		return false
	}
	// Could add terminal detection logic here
	return true
}

// writeConsoleTimestamp writes timestamp for console
func (f *ConsoleFormatter) writeConsoleTimestamp(b *strings.Builder, entry *Entry, useColors bool) {
	timeFormat := f.TimeFormat
	if timeFormat == "" {
		timeFormat = "15:04:05"
	}

	if useColors {
		b.WriteString(ColorGray)
	}
	b.WriteString(entry.Time.Format(timeFormat))
	if useColors {
		b.WriteString(ColorReset)
	}
	b.WriteByte(' ')
}

// writeConsoleLevel writes level with icon for console
func (f *ConsoleFormatter) writeConsoleLevel(b *strings.Builder, entry *Entry, useColors bool) {
	if useColors {
		color := f.getLevelColor(entry.Level)
		b.WriteString(color)
		b.WriteString(ColorBold)
	}

	if f.UseIcons {
		if icon, exists := f.LevelIcons[entry.Level]; exists {
			b.WriteString(icon)
		} else {
			b.WriteString("●")
		}
	} else {
		levelText := strings.ToUpper(entry.Level.String())
		b.WriteString(fmt.Sprintf("%-5s", levelText))
	}

	if useColors {
		b.WriteString(ColorReset)
	}
	b.WriteByte(' ')
}

// writeConsoleLoggerInfo writes logger info for console
func (f *ConsoleFormatter) writeConsoleLoggerInfo(b *strings.Builder, entry *Entry, useColors bool) {
	hasInfo := false

	if f.ShowLogger && entry.Logger != "" {
		if useColors {
			b.WriteString(ColorPurple)
		}
		b.WriteString(entry.Logger)
		hasInfo = true
	}

	if f.ShowComponent && entry.Component != "" {
		if hasInfo {
			if useColors {
				b.WriteString(ColorGray)
			}
			b.WriteByte(':')
			if useColors {
				b.WriteString(ColorPurple)
			}
		} else if useColors {
			b.WriteString(ColorPurple)
		}
		b.WriteString(entry.Component)
		hasInfo = true
	}

	if hasInfo {
		if useColors {
			b.WriteString(ColorReset)
		}
		b.WriteByte(' ')
	}
}

// writeConsoleCaller writes caller info for console
func (f *ConsoleFormatter) writeConsoleCaller(b *strings.Builder, entry *Entry, useColors bool) {
	if useColors {
		b.WriteString(ColorCyan)
	}

	file := filepath.Base(entry.Caller.File)
	fmt.Fprintf(b, "(%s:%d)", file, entry.Caller.Line)

	if useColors {
		b.WriteString(ColorReset)
	}
	b.WriteByte(' ')
}

// writeConsoleMessage writes message for console
func (f *ConsoleFormatter) writeConsoleMessage(b *strings.Builder, entry *Entry, useColors bool) {
	message := entry.Message

	// Truncate if too long
	if f.MessageMaxWidth > 0 && len(message) > f.MessageMaxWidth {
		message = message[:f.MessageMaxWidth-3] + "..."
	}

	if useColors && entry.Level <= ErrorLevel {
		b.WriteString(ColorBold)
	}

	b.WriteString(message)

	if useColors && entry.Level <= ErrorLevel {
		b.WriteString(ColorReset)
	}
}

// writeConsoleFields writes fields for console
func (f *ConsoleFormatter) writeConsoleFields(b *strings.Builder, entry *Entry, useColors bool) {
	if len(entry.Fields) == 0 {
		return
	}

	indent := strings.Repeat(" ", f.IndentSize)

	for key, value := range entry.Fields {
		b.WriteByte('\n')
		b.WriteString(indent)

		if useColors {
			b.WriteString(ColorBlue)
		}

		// Truncate field name if too long
		fieldName := key
		if f.FieldMaxWidth > 0 && len(fieldName) > f.FieldMaxWidth {
			fieldName = fieldName[:f.FieldMaxWidth-3] + "..."
		}

		b.WriteString(fieldName)
		if useColors {
			b.WriteString(ColorReset)
		}
		b.WriteString(": ")

		if useColors {
			b.WriteString(ColorGreen)
		}
		b.WriteString(fmt.Sprintf("%v", value))
		if useColors {
			b.WriteString(ColorReset)
		}
	}
}

// writeConsoleError writes error for console
func (f *ConsoleFormatter) writeConsoleError(b *strings.Builder, entry *Entry, useColors bool) {
	b.WriteByte('\n')
	indent := strings.Repeat(" ", f.IndentSize)
	b.WriteString(indent)

	if useColors {
		b.WriteString(ColorRed)
		b.WriteString(ColorBold)
	}

	b.WriteString("ERROR: ")
	b.WriteString(entry.Error.Error())

	if useColors {
		b.WriteString(ColorReset)
	}
}

// getLevelColor returns color for console level
func (f *ConsoleFormatter) getLevelColor(level Level) string {
	switch level {
	case TraceLevel:
		return ColorPurple
	case DebugLevel:
		return ColorBlue
	case InfoLevel:
		return ColorGreen
	case WarnLevel:
		return ColorYellow
	case ErrorLevel:
		return ColorRed
	case FatalLevel:
		return ColorRed
	case PanicLevel:
		return ColorRed
	default:
		return ColorWhite
	}
}

// Helper functions

// getDefaultLevelIcons returns default icons for each level
func getDefaultLevelIcons() map[Level]string {
	return map[Level]string{
		TraceLevel: "🔍",
		DebugLevel: "🐛",
		InfoLevel:  "ℹ️",
		WarnLevel:  "⚠️",
		ErrorLevel: "❌",
		FatalLevel: "💀",
		PanicLevel: "🚨",
	}
}

// resolve resolves a field name using the field map
func (fm FieldMap) resolve(key string) string {
	if mapped, exists := fm[key]; exists {
		return mapped
	}
	return key
}

// prefixFieldClashes handles field name clashes
func prefixFieldClashes(entry *Entry, fieldMap FieldMap) {
	timeKey := fieldMap.resolve(DefaultTimeKey)
	levelKey := fieldMap.resolve(DefaultLevelKey)
	messageKey := fieldMap.resolve(DefaultMessageKey)
	errorKey := fieldMap.resolve(DefaultErrorKey)
	callerKey := fieldMap.resolve(DefaultCallerKey)
	loggerKey := fieldMap.resolve(DefaultLoggerKey)
	componentKey := fieldMap.resolve(DefaultComponentKey)

	reserved := []string{timeKey, levelKey, messageKey, errorKey, callerKey, loggerKey, componentKey}

	for _, key := range reserved {
		if _, exists := entry.Fields[key]; exists {
			// Prefix with "fields." to avoid clash
			entry.Fields["fields."+key] = entry.Fields[key]
			delete(entry.Fields, key)
		}
	}
}

// NewEntry creates a new log entry
func NewEntry() *Entry {
	return &Entry{
		Time:   time.Now(),
		Fields: make(map[string]any),
	}
}

// WithLevel sets the log level
func (e *Entry) WithLevel(level Level) *Entry {
	e.Level = level
	return e
}

// WithMessage sets the message
func (e *Entry) WithMessage(message string) *Entry {
	e.Message = message
	return e
}

// WithField adds a field
func (e *Entry) WithField(key string, value any) *Entry {
	if e.Fields == nil {
		e.Fields = make(map[string]any)
	}
	e.Fields[key] = value
	return e
}

// WithFields adds multiple fields
func (e *Entry) WithFields(fields map[string]any) *Entry {
	if e.Fields == nil {
		e.Fields = make(map[string]any)
	}
	for k, v := range fields {
		e.Fields[k] = v
	}
	return e
}

// WithError adds an error
func (e *Entry) WithError(err error) *Entry {
	e.Error = err
	return e
}

// WithCaller adds caller information
func (e *Entry) WithCaller(file string, line int, function string) *Entry {
	e.Caller = &Caller{
		File:     file,
		Line:     line,
		Function: function,
	}
	return e
}

// WithLogger sets the logger name
func (e *Entry) WithLogger(logger string) *Entry {
	e.Logger = logger
	return e
}

// WithComponent sets the component name
func (e *Entry) WithComponent(component string) *Entry {
	e.Component = component
	return e
}
