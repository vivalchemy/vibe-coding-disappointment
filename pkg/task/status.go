package task

import (
	"fmt"
	"strings"
	"time"
)

// StatusTransition represents a change in task status
type StatusTransition struct {
	// Previous status
	From Status `json:"from"`

	// New status
	To Status `json:"to"`

	// When the transition occurred
	Timestamp time.Time `json:"timestamp"`

	// Optional reason for the transition
	Reason string `json:"reason,omitempty"`

	// Additional context
	Context map[string]any `json:"context,omitempty"`
}

// StatusHistory tracks the history of status changes for a task
type StatusHistory struct {
	// Task ID this history belongs to
	TaskID string `json:"task_id"`

	// All status transitions
	Transitions []StatusTransition `json:"transitions"`

	// Current status (cached for performance)
	CurrentStatus Status `json:"current_status"`

	// When the history was last updated
	LastUpdated time.Time `json:"last_updated"`
}

// NewStatusHistory creates a new status history for a task
func NewStatusHistory(taskID string, initialStatus Status) *StatusHistory {
	now := time.Now()

	return &StatusHistory{
		TaskID:        taskID,
		CurrentStatus: initialStatus,
		LastUpdated:   now,
		Transitions: []StatusTransition{
			{
				From:      StatusUnknown,
				To:        initialStatus,
				Timestamp: now,
				Reason:    "initial_status",
			},
		},
	}
}

// AddTransition adds a new status transition
func (h *StatusHistory) AddTransition(to Status, reason string, context map[string]any) {
	now := time.Now()

	transition := StatusTransition{
		From:      h.CurrentStatus,
		To:        to,
		Timestamp: now,
		Reason:    reason,
		Context:   context,
	}

	h.Transitions = append(h.Transitions, transition)
	h.CurrentStatus = to
	h.LastUpdated = now
}

// GetLastTransition returns the most recent status transition
func (h *StatusHistory) GetLastTransition() *StatusTransition {
	if len(h.Transitions) == 0 {
		return nil
	}
	return &h.Transitions[len(h.Transitions)-1]
}

// GetTransitionsForStatus returns all transitions to a specific status
func (h *StatusHistory) GetTransitionsForStatus(status Status) []StatusTransition {
	var matches []StatusTransition
	for _, transition := range h.Transitions {
		if transition.To == status {
			matches = append(matches, transition)
		}
	}
	return matches
}

// GetDurationInStatus returns how long the task has been in the current status
func (h *StatusHistory) GetDurationInStatus() time.Duration {
	lastTransition := h.GetLastTransition()
	if lastTransition == nil {
		return 0
	}
	return time.Since(lastTransition.Timestamp)
}

// GetTotalExecutionTime returns the total time spent in running status
func (h *StatusHistory) GetTotalExecutionTime() time.Duration {
	var total time.Duration
	var runningStart *time.Time

	for _, transition := range h.Transitions {
		if transition.To == StatusRunning {
			runningStart = &transition.Timestamp
		} else if runningStart != nil && transition.From == StatusRunning {
			total += transition.Timestamp.Sub(*runningStart)
			runningStart = nil
		}
	}

	// If still running, add current duration
	if runningStart != nil {
		total += time.Since(*runningStart)
	}

	return total
}

// GetSuccessRate returns the success rate as a percentage
func (h *StatusHistory) GetSuccessRate() float64 {
	successCount := 0
	totalRuns := 0

	for _, transition := range h.Transitions {
		if transition.To == StatusSuccess || transition.To == StatusFailed {
			totalRuns++
			if transition.To == StatusSuccess {
				successCount++
			}
		}
	}

	if totalRuns == 0 {
		return 0
	}

	return float64(successCount) / float64(totalRuns) * 100
}

// StatusManager manages status transitions and validation
type StatusManager struct {
	// Valid status transitions
	transitions map[Status][]Status

	// Status history for tasks
	histories map[string]*StatusHistory
}

// NewStatusManager creates a new status manager
func NewStatusManager() *StatusManager {
	sm := &StatusManager{
		transitions: make(map[Status][]Status),
		histories:   make(map[string]*StatusHistory),
	}

	// Define valid status transitions
	sm.transitions[StatusIdle] = []Status{StatusRunning}
	sm.transitions[StatusRunning] = []Status{StatusSuccess, StatusFailed, StatusStopped}
	sm.transitions[StatusSuccess] = []Status{StatusRunning}
	sm.transitions[StatusFailed] = []Status{StatusRunning}
	sm.transitions[StatusStopped] = []Status{StatusRunning}
	sm.transitions[StatusUnknown] = []Status{StatusIdle, StatusRunning, StatusSuccess, StatusFailed, StatusStopped}

	return sm
}

// IsValidTransition checks if a status transition is valid
func (sm *StatusManager) IsValidTransition(from, to Status) bool {
	validTransitions, exists := sm.transitions[from]
	if !exists {
		return false
	}

	for _, validTo := range validTransitions {
		if validTo == to {
			return true
		}
	}

	return false
}

// TransitionTask transitions a task to a new status
func (sm *StatusManager) TransitionTask(taskID string, currentStatus, newStatus Status, reason string, context map[string]any) error {
	// Validate transition
	if !sm.IsValidTransition(currentStatus, newStatus) {
		return fmt.Errorf("invalid status transition from %s to %s", currentStatus, newStatus)
	}

	// Get or create history
	history, exists := sm.histories[taskID]
	if !exists {
		history = NewStatusHistory(taskID, currentStatus)
		sm.histories[taskID] = history
	}

	// Add transition
	history.AddTransition(newStatus, reason, context)

	return nil
}

// GetHistory returns the status history for a task
func (sm *StatusManager) GetHistory(taskID string) *StatusHistory {
	return sm.histories[taskID]
}

// GetAllHistories returns all status histories
func (sm *StatusManager) GetAllHistories() map[string]*StatusHistory {
	// Return a copy to prevent external modification
	histories := make(map[string]*StatusHistory)
	for k, v := range sm.histories {
		histories[k] = v
	}
	return histories
}

// ClearHistory removes the status history for a task
func (sm *StatusManager) ClearHistory(taskID string) {
	delete(sm.histories, taskID)
}

// GetTasksByStatus returns all task IDs with the given status
func (sm *StatusManager) GetTasksByStatus(status Status) []string {
	var taskIDs []string
	for taskID, history := range sm.histories {
		if history.CurrentStatus == status {
			taskIDs = append(taskIDs, taskID)
		}
	}
	return taskIDs
}

// StatusColor represents color information for status display
type StatusColor struct {
	// ANSI color code
	ANSI string `json:"ansi"`

	// Hex color code
	Hex string `json:"hex"`

	// RGB values
	RGB [3]int `json:"rgb"`

	// Human-readable name
	Name string `json:"name"`
}

// GetStatusColor returns the appropriate color for a status
func GetStatusColor(status Status) StatusColor {
	switch status {
	case StatusIdle:
		return StatusColor{
			ANSI: "\033[37m", // White
			Hex:  "#FFFFFF",
			RGB:  [3]int{255, 255, 255},
			Name: "white",
		}
	case StatusRunning:
		return StatusColor{
			ANSI: "\033[33m", // Yellow
			Hex:  "#FFFF00",
			RGB:  [3]int{255, 255, 0},
			Name: "yellow",
		}
	case StatusSuccess:
		return StatusColor{
			ANSI: "\033[32m", // Green
			Hex:  "#00FF00",
			RGB:  [3]int{0, 255, 0},
			Name: "green",
		}
	case StatusFailed:
		return StatusColor{
			ANSI: "\033[31m", // Red
			Hex:  "#FF0000",
			RGB:  [3]int{255, 0, 0},
			Name: "red",
		}
	case StatusStopped:
		return StatusColor{
			ANSI: "\033[35m", // Magenta
			Hex:  "#FF00FF",
			RGB:  [3]int{255, 0, 255},
			Name: "magenta",
		}
	default:
		return StatusColor{
			ANSI: "\033[90m", // Dark gray
			Hex:  "#808080",
			RGB:  [3]int{128, 128, 128},
			Name: "gray",
		}
	}
}

// GetStatusIcon returns an appropriate icon/symbol for a status
func GetStatusIcon(status Status) string {
	switch status {
	case StatusIdle:
		return "⏸" // Pause symbol
	case StatusRunning:
		return "▶" // Play symbol
	case StatusSuccess:
		return "✓" // Check mark
	case StatusFailed:
		return "✗" // X mark
	case StatusStopped:
		return "⏹" // Stop symbol
	default:
		return "?" // Question mark
	}
}

// GetStatusEmoji returns an appropriate emoji for a status
func GetStatusEmoji(status Status) string {
	switch status {
	case StatusIdle:
		return "⏸️"
	case StatusRunning:
		return "🏃"
	case StatusSuccess:
		return "✅"
	case StatusFailed:
		return "❌"
	case StatusStopped:
		return "⏹️"
	default:
		return "❓"
	}
}

// FormatStatus returns a formatted string representation of a status
func FormatStatus(status Status, useColor bool, useIcon bool) string {
	var parts []string

	if useIcon {
		parts = append(parts, GetStatusIcon(status))
	}

	statusStr := strings.ToUpper(string(status))

	if useColor {
		color := GetStatusColor(status)
		statusStr = color.ANSI + statusStr + "\033[0m" // Reset color
	}

	parts = append(parts, statusStr)

	return strings.Join(parts, " ")
}

// ParseStatus parses a string into a Status, case-insensitive
func ParseStatus(s string) (Status, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "idle":
		return StatusIdle, nil
	case "running":
		return StatusRunning, nil
	case "success", "succeeded", "ok":
		return StatusSuccess, nil
	case "failed", "failure", "error":
		return StatusFailed, nil
	case "stopped", "cancelled", "canceled":
		return StatusStopped, nil
	case "unknown":
		return StatusUnknown, nil
	default:
		return StatusUnknown, fmt.Errorf("unknown status: %s", s)
	}
}

// StatusStats provides statistics about task statuses
type StatusStats struct {
	// Count by status
	Counts map[Status]int `json:"counts"`

	// Total number of tasks
	Total int `json:"total"`

	// Success rate percentage
	SuccessRate float64 `json:"success_rate"`

	// Average execution time
	AverageExecutionTime time.Duration `json:"average_execution_time"`

	// When stats were calculated
	CalculatedAt time.Time `json:"calculated_at"`
}

// CalculateStats calculates statistics from a slice of tasks
func CalculateStats(tasks []*Task) *StatusStats {
	stats := &StatusStats{
		Counts:       make(map[Status]int),
		Total:        len(tasks),
		CalculatedAt: time.Now(),
	}

	var totalExecutionTime time.Duration
	var executionCount int
	var successCount int
	var completedCount int

	for _, task := range tasks {
		// Count by status
		stats.Counts[task.Status]++

		// Calculate execution time averages
		if task.Timing != nil && task.Status.IsTerminal() {
			totalExecutionTime += task.Timing.Duration
			executionCount++
			completedCount++

			if task.Status == StatusSuccess {
				successCount++
			}
		}
	}

	// Calculate averages
	if executionCount > 0 {
		stats.AverageExecutionTime = totalExecutionTime / time.Duration(executionCount)
	}

	if completedCount > 0 {
		stats.SuccessRate = float64(successCount) / float64(completedCount) * 100
	}

	return stats
}

// String returns a human-readable representation of the stats
func (s *StatusStats) String() string {
	var parts []string

	parts = append(parts, fmt.Sprintf("Total: %d", s.Total))

	for status, count := range s.Counts {
		if count > 0 {
			parts = append(parts, fmt.Sprintf("%s: %d", status, count))
		}
	}

	if s.SuccessRate > 0 {
		parts = append(parts, fmt.Sprintf("Success Rate: %.1f%%", s.SuccessRate))
	}

	if s.AverageExecutionTime > 0 {
		parts = append(parts, fmt.Sprintf("Avg Time: %v", s.AverageExecutionTime))
	}

	return strings.Join(parts, ", ")
}
