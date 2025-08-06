package tui

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vivalchemy/wake/pkg/task"
)

// Core system messages

// QuitMsg is sent to quit the application
type QuitMsg struct {
	Reason string    `json:"reason,omitempty"`
	Time   time.Time `json:"time"`
}

// ErrorMsg is sent when an error occurs
type ErrorMsg struct {
	Error     error     `json:"error"`
	Component string    `json:"component,omitempty"`
	Context   string    `json:"context,omitempty"`
	Time      time.Time `json:"time"`
}

// TickMsg is sent for periodic updates
type TickMsg struct {
	Time time.Time `json:"time"`
}

// WindowResizedMsg is sent when the terminal window is resized
type WindowResizedMsg struct {
	Width  int       `json:"width"`
	Height int       `json:"height"`
	Time   time.Time `json:"time"`
}

// Task-related messages

// TasksDiscoveredMsg is sent when tasks are discovered
type TasksDiscoveredMsg struct {
	Tasks  []*task.Task `json:"tasks"`
	Source string       `json:"source"`
	Count  int          `json:"count"`
	Time   time.Time    `json:"time"`
}

// TaskSelectedMsg is sent when a task is selected
type TaskSelectedMsg struct {
	Task  *task.Task `json:"task"`
	Index int        `json:"index"`
	Time  time.Time  `json:"time"`
}

// RunTaskMsg is sent to run a specific task
type RunTaskMsg struct {
	TaskID   string    `json:"task_id"`
	TaskName string    `json:"task_name,omitempty"`
	DryRun   bool      `json:"dry_run"`
	Time     time.Time `json:"time"`
}

// RunSelectedTaskMsg is sent to run the currently selected task
type RunSelectedTaskMsg struct {
	DryRun bool      `json:"dry_run"`
	Time   time.Time `json:"time"`
}

// StopTaskMsg is sent to stop a running task
type StopTaskMsg struct {
	TaskID string    `json:"task_id"`
	Force  bool      `json:"force"`
	Time   time.Time `json:"time"`
}

// TaskStartedMsg is sent when a task starts execution
type TaskStartedMsg struct {
	TaskID   string    `json:"task_id"`
	TaskName string    `json:"task_name"`
	PID      int       `json:"pid,omitempty"`
	Time     time.Time `json:"time"`
}

// TaskFinishedMsg is sent when a task finishes execution
type TaskFinishedMsg struct {
	TaskID   string        `json:"task_id"`
	TaskName string        `json:"task_name"`
	ExitCode int           `json:"exit_code"`
	Duration time.Duration `json:"duration"`
	Error    error         `json:"error,omitempty"`
	Time     time.Time     `json:"time"`
}

// TaskOutputMsg is sent when task output is received
type TaskOutputMsg struct {
	TaskID   string    `json:"task_id"`
	TaskName string    `json:"task_name"`
	Output   string    `json:"output"`
	IsStderr bool      `json:"is_stderr"`
	Time     time.Time `json:"time"`
}

// TaskStatusChangedMsg is sent when a task status changes
type TaskStatusChangedMsg struct {
	TaskID    string      `json:"task_id"`
	TaskName  string      `json:"task_name"`
	OldStatus task.Status `json:"old_status"`
	NewStatus task.Status `json:"new_status"`
	Time      time.Time   `json:"time"`
}

// Discovery-related messages

// DiscoveryStartedMsg is sent when task discovery begins
type DiscoveryStartedMsg struct {
	Path string    `json:"path"`
	Time time.Time `json:"time"`
}

// DiscoveryCompletedMsg is sent when task discovery completes
type DiscoveryCompletedMsg struct {
	Path      string        `json:"path"`
	TaskCount int           `json:"task_count"`
	Duration  time.Duration `json:"duration"`
	Time      time.Time     `json:"time"`
}

// DiscoveryFailedMsg is sent when task discovery fails
type DiscoveryFailedMsg struct {
	Path  string    `json:"path"`
	Error error     `json:"error"`
	Time  time.Time `json:"time"`
}

// RefreshTasksMsg is sent to refresh the task list
type RefreshTasksMsg struct {
	Force bool      `json:"force"`
	Time  time.Time `json:"time"`
}

// RefreshStartedMsg is sent when task refresh begins
type RefreshStartedMsg struct {
	Time time.Time `json:"time"`
}

// RefreshCompletedMsg is sent when task refresh completes
type RefreshCompletedMsg struct {
	TaskCount int           `json:"task_count"`
	Duration  time.Duration `json:"duration"`
	Time      time.Time     `json:"time"`
}

// UI state messages

// ShowHelpMsg is sent to show/hide help
type ShowHelpMsg struct {
	Toggle bool      `json:"toggle"`
	Time   time.Time `json:"time"`
}

// HideHelpMsg is sent to hide help
type HideHelpMsg struct {
	Time time.Time `json:"time"`
}

// ToggleSearchMsg is sent to toggle search interface
type ToggleSearchMsg struct {
	Time time.Time `json:"time"`
}

// ShowSearchMsg is sent to show search interface
type ShowSearchMsg struct {
	InitialQuery string    `json:"initial_query,omitempty"`
	Time         time.Time `json:"time"`
}

// HideSearchMsg is sent to hide search interface
type HideSearchMsg struct {
	Time time.Time `json:"time"`
}

// SearchQueryChangedMsg is sent when search query changes
type SearchQueryChangedMsg struct {
	Query    string    `json:"query"`
	OldQuery string    `json:"old_query"`
	Time     time.Time `json:"time"`
}

// SearchResultsMsg is sent with search results
type SearchResultsMsg struct {
	Query    string        `json:"query"`
	Results  []*task.Task  `json:"results"`
	Count    int           `json:"count"`
	Duration time.Duration `json:"duration"`
	Time     time.Time     `json:"time"`
}

// Focus and navigation messages

// FocusChangedMsg is sent when focus changes between components
type FocusChangedMsg struct {
	PrevComponent string    `json:"prev_component"`
	NewComponent  string    `json:"new_component"`
	Time          time.Time `json:"time"`
}

// NavigateUpMsg is sent to navigate up
type NavigateUpMsg struct {
	Steps int       `json:"steps"`
	Time  time.Time `json:"time"`
}

// NavigateDownMsg is sent to navigate down
type NavigateDownMsg struct {
	Steps int       `json:"steps"`
	Time  time.Time `json:"time"`
}

// NavigateLeftMsg is sent to navigate left
type NavigateLeftMsg struct {
	Steps int       `json:"steps"`
	Time  time.Time `json:"time"`
}

// NavigateRightMsg is sent to navigate right
type NavigateRightMsg struct {
	Steps int       `json:"steps"`
	Time  time.Time `json:"time"`
}

// PageUpMsg is sent to page up
type PageUpMsg struct {
	Time time.Time `json:"time"`
}

// PageDownMsg is sent to page down
type PageDownMsg struct {
	Time time.Time `json:"time"`
}

// HomeMsg is sent to go to the beginning
type HomeMsg struct {
	Time time.Time `json:"time"`
}

// EndMsg is sent to go to the end
type EndMsg struct {
	Time time.Time `json:"time"`
}

// Layout and view messages

// SplitModeChangedMsg is sent when split mode changes
type SplitModeChangedMsg struct {
	OldMode string    `json:"old_mode"`
	NewMode string    `json:"new_mode"`
	Time    time.Time `json:"time"`
}

// SidebarResizedMsg is sent when sidebar is resized
type SidebarResizedMsg struct {
	OldWidth int       `json:"old_width"`
	NewWidth int       `json:"new_width"`
	Delta    int       `json:"delta"`
	Time     time.Time `json:"time"`
}

// ToggleFullscreenMsg is sent to toggle fullscreen mode
type ToggleFullscreenMsg struct {
	Component string    `json:"component"`
	Time      time.Time `json:"time"`
}

// Theme and styling messages

// ThemeChangedMsg is sent when theme changes
type ThemeChangedMsg struct {
	Theme    string    `json:"theme"`
	OldTheme string    `json:"old_theme,omitempty"`
	Time     time.Time `json:"time"`
}

// KeyProfileChangedMsg is sent when key profile changes
type KeyProfileChangedMsg struct {
	Profile    string    `json:"profile"`
	OldProfile string    `json:"old_profile,omitempty"`
	Time       time.Time `json:"time"`
}

// ColorsChangedMsg is sent when color scheme changes
type ColorsChangedMsg struct {
	Scheme    string    `json:"scheme"`
	OldScheme string    `json:"old_scheme,omitempty"`
	Time      time.Time `json:"time"`
}

// Configuration messages

// ConfigChangedMsg is sent when configuration changes
type ConfigChangedMsg struct {
	Key   string    `json:"key,omitempty"`
	Value any       `json:"value,omitempty"`
	Time  time.Time `json:"time"`
}

// ConfigReloadedMsg is sent when configuration is reloaded
type ConfigReloadedMsg struct {
	Path string    `json:"path"`
	Time time.Time `json:"time"`
}

// ConfigSavedMsg is sent when configuration is saved
type ConfigSavedMsg struct {
	Path string    `json:"path"`
	Time time.Time `json:"time"`
}

// Filter and sort messages

// FilterChangedMsg is sent when filters change
type FilterChangedMsg struct {
	FilterType string    `json:"filter_type"`
	Value      any       `json:"value"`
	Time       time.Time `json:"time"`
}

// SortChangedMsg is sent when sorting changes
type SortChangedMsg struct {
	SortBy    string    `json:"sort_by"`
	Direction string    `json:"direction"`
	Time      time.Time `json:"time"`
}

// ClearFiltersMsg is sent to clear all filters
type ClearFiltersMsg struct {
	Time time.Time `json:"time"`
}

// Debug and monitoring messages

// EnableDebugMsg is sent to enable debug mode
type EnableDebugMsg struct {
	Time time.Time `json:"time"`
}

// DisableDebugMsg is sent to disable debug mode
type DisableDebugMsg struct {
	Time time.Time `json:"time"`
}

// DebugInfoMsg contains debug information
type DebugInfoMsg struct {
	Component string         `json:"component"`
	Info      map[string]any `json:"info"`
	Time      time.Time      `json:"time"`
}

// PerformanceStatsMsg contains performance statistics
type PerformanceStatsMsg struct {
	Component   string        `json:"component"`
	RenderTime  time.Duration `json:"render_time"`
	UpdateTime  time.Duration `json:"update_time"`
	MessageRate int           `json:"message_rate"`
	Time        time.Time     `json:"time"`
}

// Output and logging messages

// ClearOutputMsg is sent to clear output
type ClearOutputMsg struct {
	Component string    `json:"component,omitempty"`
	Time      time.Time `json:"time"`
}

// SaveOutputMsg is sent to save output to file
type SaveOutputMsg struct {
	Filename string    `json:"filename"`
	Format   string    `json:"format"`
	Time     time.Time `json:"time"`
}

// CopyOutputMsg is sent to copy output to clipboard
type CopyOutputMsg struct {
	Content string    `json:"content"`
	Time    time.Time `json:"time"`
}

// ToggleAutoScrollMsg is sent to toggle auto-scroll
type ToggleAutoScrollMsg struct {
	Enabled bool      `json:"enabled"`
	Time    time.Time `json:"time"`
}

// ToggleWrapMsg is sent to toggle line wrapping
type ToggleWrapMsg struct {
	Enabled bool      `json:"enabled"`
	Time    time.Time `json:"time"`
}

// Status and notification messages

// StatusMsg contains status information
type StatusMsg struct {
	Message  string    `json:"message"`
	Level    string    `json:"level"`
	Duration int       `json:"duration_ms,omitempty"`
	Time     time.Time `json:"time"`
}

// NotificationMsg contains notification information
type NotificationMsg struct {
	Title   string    `json:"title"`
	Message string    `json:"message"`
	Type    string    `json:"type"`
	Time    time.Time `json:"time"`
}

// ToastMsg contains toast notification information
type ToastMsg struct {
	Message  string        `json:"message"`
	Type     string        `json:"type"`
	Duration time.Duration `json:"duration"`
	Time     time.Time     `json:"time"`
}

// Session and state messages

// SessionStartedMsg is sent when TUI session starts
type SessionStartedMsg struct {
	SessionID string    `json:"session_id"`
	Time      time.Time `json:"time"`
}

// SessionEndedMsg is sent when TUI session ends
type SessionEndedMsg struct {
	SessionID string        `json:"session_id"`
	Duration  time.Duration `json:"duration"`
	Stats     SessionStats  `json:"stats"`
	Time      time.Time     `json:"time"`
}

// SessionStats contains session statistics
type SessionStats struct {
	TasksRun       int           `json:"tasks_run"`
	KeyPresses     int64         `json:"key_presses"`
	CommandsIssued int64         `json:"commands_issued"`
	ErrorsShown    int64         `json:"errors_shown"`
	AvgRenderTime  time.Duration `json:"avg_render_time"`
}

// StateChangedMsg is sent when application state changes
type StateChangedMsg struct {
	StateType string    `json:"state_type"`
	OldValue  any       `json:"old_value"`
	NewValue  any       `json:"new_value"`
	Time      time.Time `json:"time"`
}

// TaskCountChangedMsg is sent when task count changes
type TaskCountChangedMsg struct {
	Count    int       `json:"count"`
	OldCount int       `json:"old_count"`
	Time     time.Time `json:"time"`
}

// Animation and visual effect messages

// AnimationStartMsg is sent to start an animation
type AnimationStartMsg struct {
	AnimationType string        `json:"animation_type"`
	Duration      time.Duration `json:"duration"`
	Target        string        `json:"target"`
	Time          time.Time     `json:"time"`
}

// AnimationEndMsg is sent when an animation ends
type AnimationEndMsg struct {
	AnimationType string    `json:"animation_type"`
	Target        string    `json:"target"`
	Time          time.Time `json:"time"`
}

// SpinnerStartMsg is sent to start a spinner
type SpinnerStartMsg struct {
	Message string    `json:"message"`
	Time    time.Time `json:"time"`
}

// SpinnerStopMsg is sent to stop a spinner
type SpinnerStopMsg struct {
	Time time.Time `json:"time"`
}

// Batch message for multiple operations

// BatchMsg contains multiple messages to be processed
type BatchMsg struct {
	Messages []tea.Msg `json:"messages"`
	Time     time.Time `json:"time"`
}

// Utility functions for message creation

// NewQuitMsg creates a new quit message
func NewQuitMsg(reason string) QuitMsg {
	return QuitMsg{
		Reason: reason,
		Time:   time.Now(),
	}
}

// NewErrorMsg creates a new error message
func NewErrorMsg(err error, component, context string) ErrorMsg {
	return ErrorMsg{
		Error:     err,
		Component: component,
		Context:   context,
		Time:      time.Now(),
	}
}

// NewTaskSelectedMsg creates a new task selected message
func NewTaskSelectedMsg(task *task.Task, index int) TaskSelectedMsg {
	return TaskSelectedMsg{
		Task:  task,
		Index: index,
		Time:  time.Now(),
	}
}

// NewRunTaskMsg creates a new run task message
func NewRunTaskMsg(taskID, taskName string, dryRun bool) RunTaskMsg {
	return RunTaskMsg{
		TaskID:   taskID,
		TaskName: taskName,
		DryRun:   dryRun,
		Time:     time.Now(),
	}
}

// NewTaskStartedMsg creates a new task started message
func NewTaskStartedMsg(taskID, taskName string, pid int) TaskStartedMsg {
	return TaskStartedMsg{
		TaskID:   taskID,
		TaskName: taskName,
		PID:      pid,
		Time:     time.Now(),
	}
}

// NewTaskFinishedMsg creates a new task finished message
func NewTaskFinishedMsg(taskID, taskName string, exitCode int, duration time.Duration, err error) TaskFinishedMsg {
	return TaskFinishedMsg{
		TaskID:   taskID,
		TaskName: taskName,
		ExitCode: exitCode,
		Duration: duration,
		Error:    err,
		Time:     time.Now(),
	}
}

// NewStatusMsg creates a new status message
func NewStatusMsg(message, level string, duration int) StatusMsg {
	return StatusMsg{
		Message:  message,
		Level:    level,
		Duration: duration,
		Time:     time.Now(),
	}
}

// NewNotificationMsg creates a new notification message
func NewNotificationMsg(title, message, msgType string) NotificationMsg {
	return NotificationMsg{
		Title:   title,
		Message: message,
		Type:    msgType,
		Time:    time.Now(),
	}
}

// Message type constants for easier identification
const (
	// Message types
	MsgTypeSystem        = "system"
	MsgTypeTask          = "task"
	MsgTypeUI            = "ui"
	MsgTypeNavigation    = "navigation"
	MsgTypeConfiguration = "configuration"
	MsgTypeDebug         = "debug"
	MsgTypeStatus        = "status"

	// Status levels
	StatusLevelInfo    = "info"
	StatusLevelWarning = "warning"
	StatusLevelError   = "error"
	StatusLevelSuccess = "success"

	// Notification types
	NotificationTypeInfo    = "info"
	NotificationTypeWarning = "warning"
	NotificationTypeError   = "error"
	NotificationTypeSuccess = "success"
)

// MessageRouter provides message routing utilities
type MessageRouter struct {
	handlers map[string]func(tea.Msg) tea.Cmd
}

// NewMessageRouter creates a new message router
func NewMessageRouter() *MessageRouter {
	return &MessageRouter{
		handlers: make(map[string]func(tea.Msg) tea.Cmd),
	}
}

// Register registers a handler for a message type
func (mr *MessageRouter) Register(msgType string, handler func(tea.Msg) tea.Cmd) {
	mr.handlers[msgType] = handler
}

// Route routes a message to the appropriate handler
func (mr *MessageRouter) Route(msg tea.Msg) tea.Cmd {
	msgType := fmt.Sprintf("%T", msg)
	if handler, exists := mr.handlers[msgType]; exists {
		return handler(msg)
	}
	return nil
}
