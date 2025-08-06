package task

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// Task represents a discoverable task from any supported task runner
type Task struct {
	// Unique identifier for the task
	ID string `json:"id"`

	// Human-readable name of the task
	Name string `json:"name"`

	// Description of what the task does
	Description string `json:"description"`

	// Type of task runner (make, npm, just, task, python)
	Runner RunnerType `json:"runner"`

	// File path where the task is defined
	FilePath string `json:"file_path"`

	// Directory where the task should be executed
	WorkingDirectory string `json:"working_directory"`

	// Raw command that will be executed
	Command string `json:"command"`

	// Arguments for the command
	Args []string `json:"args"`

	// Environment variables specific to this task
	Environment map[string]string `json:"environment"`

	// Current execution status
	Status Status `json:"status"`

	// Process ID when running
	PID int `json:"pid,omitempty"`

	// Exit code from last execution
	ExitCode int `json:"exit_code,omitempty"`

	// Execution time metadata
	Timing *ExecutionTiming `json:"timing,omitempty"`

	// Dependencies (other tasks that must run first)
	Dependencies []string `json:"dependencies,omitempty"`

	// Tags for categorization and filtering
	Tags []string `json:"tags,omitempty"`

	// Aliases for the task
	Aliases []string `json:"aliases,omitempty"`

	// Whether the task is hidden from normal listing
	Hidden bool `json:"hidden"`

	// Custom metadata from the task runner
	Metadata map[string]any `json:"metadata,omitempty"`

	// Last discovered/updated timestamp
	DiscoveredAt time.Time `json:"discovered_at"`

	// Last execution timestamp
	LastExecutedAt *time.Time `json:"last_executed_at,omitempty"`
}

// RunnerType represents the type of task runner
type RunnerType string

const (
	RunnerMake   RunnerType = "make"
	RunnerNpm    RunnerType = "npm"
	RunnerYarn   RunnerType = "yarn"
	RunnerPnpm   RunnerType = "pnpm"
	RunnerBun    RunnerType = "bun"
	RunnerJust   RunnerType = "just"
	RunnerTask   RunnerType = "task"
	RunnerPython RunnerType = "python"
	RunnerCustom RunnerType = "custom"
)

// String returns the string representation of the runner type
func (r RunnerType) String() string {
	return string(r)
}

// IsValid checks if the runner type is valid
func (r RunnerType) IsValid() bool {
	switch r {
	case RunnerMake, RunnerNpm, RunnerYarn, RunnerPnpm, RunnerBun,
		RunnerJust, RunnerTask, RunnerPython, RunnerCustom:
		return true
	default:
		return false
	}
}

// IsNpmFamily checks if the runner is part of the npm family
func (r RunnerType) IsNpmFamily() bool {
	switch r {
	case RunnerNpm, RunnerYarn, RunnerPnpm, RunnerBun:
		return true
	default:
		return false
	}
}

// Status represents the current status of a task
type Status string

const (
	StatusIdle    Status = "idle"
	StatusRunning Status = "running"
	StatusSuccess Status = "success"
	StatusFailed  Status = "failed"
	StatusStopped Status = "stopped"
	StatusUnknown Status = "unknown"
)

// String returns the string representation of the status
func (s Status) String() string {
	return string(s)
}

// IsValid checks if the status is valid
func (s Status) IsValid() bool {
	switch s {
	case StatusIdle, StatusRunning, StatusSuccess, StatusFailed, StatusStopped, StatusUnknown:
		return true
	default:
		return false
	}
}

// IsTerminal checks if the status represents a finished execution
func (s Status) IsTerminal() bool {
	switch s {
	case StatusSuccess, StatusFailed, StatusStopped:
		return true
	default:
		return false
	}
}

// ExecutionTiming holds timing information for task execution
type ExecutionTiming struct {
	// When the task started
	StartedAt time.Time `json:"started_at"`

	// When the task finished (nil if still running)
	FinishedAt *time.Time `json:"finished_at,omitempty"`

	// Total duration of execution
	Duration time.Duration `json:"duration"`

	// User CPU time used
	UserTime time.Duration `json:"user_time,omitempty"`

	// System CPU time used
	SystemTime time.Duration `json:"system_time,omitempty"`

	// Maximum memory used (in bytes)
	MaxMemory int64 `json:"max_memory,omitempty"`
}

// IsRunning checks if the timing indicates the task is still running
func (t *ExecutionTiming) IsRunning() bool {
	return t.FinishedAt == nil
}

// UpdateDuration updates the duration based on current time or finish time
func (t *ExecutionTiming) UpdateDuration() {
	if t.FinishedAt != nil {
		t.Duration = t.FinishedAt.Sub(t.StartedAt)
	} else {
		t.Duration = time.Since(t.StartedAt)
	}
}

// NewTask creates a new task with default values
func NewTask(name string, runner RunnerType) *Task {
	id := GenerateTaskID(name, runner, "")

	return &Task{
		ID:           id,
		Name:         name,
		Runner:       runner,
		Status:       StatusIdle,
		Environment:  make(map[string]string),
		Metadata:     make(map[string]any),
		Tags:         make([]string, 0),
		Aliases:      make([]string, 0),
		Dependencies: make([]string, 0),
		DiscoveredAt: time.Now(),
	}
}

// GenerateTaskID generates a unique ID for a task
func GenerateTaskID(name string, runner RunnerType, filePath string) string {
	// Use a combination of runner, name, and file path for uniqueness
	parts := []string{string(runner), name}

	if filePath != "" {
		// Use relative path and clean it
		cleanPath := filepath.Clean(filePath)
		parts = append(parts, cleanPath)
	}

	// Join with colons and replace problematic characters
	id := strings.Join(parts, ":")
	id = strings.ReplaceAll(id, " ", "_")
	id = strings.ReplaceAll(id, "/", "_")
	id = strings.ReplaceAll(id, "\\", "_")

	return id
}

// Clone creates a deep copy of the task
func (t *Task) Clone() *Task {
	clone := *t

	// Deep copy slices
	if t.Args != nil {
		clone.Args = make([]string, len(t.Args))
		copy(clone.Args, t.Args)
	}

	if t.Tags != nil {
		clone.Tags = make([]string, len(t.Tags))
		copy(clone.Tags, t.Tags)
	}

	if t.Aliases != nil {
		clone.Aliases = make([]string, len(t.Aliases))
		copy(clone.Aliases, t.Aliases)
	}

	if t.Dependencies != nil {
		clone.Dependencies = make([]string, len(t.Dependencies))
		copy(clone.Dependencies, t.Dependencies)
	}

	// Deep copy maps
	if t.Environment != nil {
		clone.Environment = make(map[string]string)
		maps.Copy(clone.Environment, t.Environment)
	}

	if t.Metadata != nil {
		clone.Metadata = make(map[string]any)
		maps.Copy(clone.Metadata, t.Metadata)
	}

	// Deep copy timing if it exists
	if t.Timing != nil {
		timingClone := *t.Timing
		clone.Timing = &timingClone
	}

	// Deep copy time pointers
	if t.LastExecutedAt != nil {
		lastExec := *t.LastExecutedAt
		clone.LastExecutedAt = &lastExec
	}

	return &clone
}

// Matches checks if the task matches the given criteria
func (t *Task) Matches(criteria *SearchCriteria) bool {
	if criteria == nil {
		return true
	}

	// Check name match
	if criteria.Name != "" {
		if !strings.Contains(strings.ToLower(t.Name), strings.ToLower(criteria.Name)) {
			return false
		}
	}

	// Check runner match
	if criteria.Runner != "" {
		if t.Runner != RunnerType(criteria.Runner) {
			return false
		}
	}

	// Check status match
	if criteria.Status != "" {
		if t.Status != Status(criteria.Status) {
			return false
		}
	}

	// Check tags match
	if len(criteria.Tags) > 0 {
		for _, requiredTag := range criteria.Tags {
			found := slices.Contains(t.Tags, requiredTag)
			if !found {
				return false
			}
		}
	}

	// Check hidden filter
	if criteria.ShowHidden != nil && *criteria.ShowHidden != t.Hidden {
		return false
	}

	return true
}

// SearchCriteria defines criteria for searching/filtering tasks
type SearchCriteria struct {
	// Name pattern to match
	Name string `json:"name,omitempty"`

	// Runner type to match
	Runner string `json:"runner,omitempty"`

	// Status to match
	Status string `json:"status,omitempty"`

	// Tags that must be present
	Tags []string `json:"tags,omitempty"`

	// Whether to show hidden tasks
	ShowHidden *bool `json:"show_hidden,omitempty"`

	// File path pattern
	FilePath string `json:"file_path,omitempty"`
}

// HasTag checks if the task has a specific tag
func (t *Task) HasTag(tag string) bool {
	return slices.Contains(t.Tags, tag)
}

// AddTag adds a tag to the task if it doesn't already exist
func (t *Task) AddTag(tag string) {
	if !t.HasTag(tag) {
		t.Tags = append(t.Tags, tag)
	}
}

// RemoveTag removes a tag from the task
func (t *Task) RemoveTag(tag string) {
	for i, ttag := range t.Tags {
		if ttag == tag {
			t.Tags = slices.Delete(t.Tags, i, i+1)
			break
		}
	}
}

// HasAlias checks if the task has a specific alias
func (t *Task) HasAlias(alias string) bool {
	return slices.Contains(t.Aliases, alias)
}

// AddAlias adds an alias to the task if it doesn't already exist
func (t *Task) AddAlias(alias string) {
	if !t.HasAlias(alias) {
		t.Aliases = append(t.Aliases, alias)
	}
}

// GetDisplayName returns the best display name for the task
func (t *Task) GetDisplayName() string {
	if t.Name != "" {
		return t.Name
	}
	return t.ID
}

// GetShortFilePath returns a shortened version of the file path
func (t *Task) GetShortFilePath() string {
	if t.FilePath == "" {
		return ""
	}

	// Get just the filename for display
	return filepath.Base(t.FilePath)
}

// SetStatus updates the task status and related metadata
func (t *Task) SetStatus(status Status) {
	oldStatus := t.Status
	t.Status = status

	// Update timing based on status change
	now := time.Now()

	if status == StatusRunning && oldStatus != StatusRunning {
		// Task is starting
		t.Timing = &ExecutionTiming{
			StartedAt: now,
		}
		t.LastExecutedAt = &now
	} else if status.IsTerminal() && t.Timing != nil && t.Timing.FinishedAt == nil {
		// Task is finishing
		t.Timing.FinishedAt = &now
		t.Timing.UpdateDuration()
	}
}

// GetFormattedDuration returns a human-readable duration string
func (t *Task) GetFormattedDuration() string {
	if t.Timing == nil {
		return ""
	}

	duration := t.Timing.Duration
	if t.Timing.IsRunning() {
		t.Timing.UpdateDuration()
		duration = t.Timing.Duration
	}

	if duration < time.Second {
		return fmt.Sprintf("%dms", duration.Milliseconds())
	} else if duration < time.Minute {
		return fmt.Sprintf("%.1fs", duration.Seconds())
	} else {
		return fmt.Sprintf("%dm%ds", int(duration.Minutes()), int(duration.Seconds())%60)
	}
}

// Validate checks if the task has all required fields
func (t *Task) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("task ID cannot be empty")
	}

	if t.Name == "" {
		return fmt.Errorf("task name cannot be empty")
	}

	if !t.Runner.IsValid() {
		return fmt.Errorf("invalid runner type: %s", t.Runner)
	}

	if !t.Status.IsValid() {
		return fmt.Errorf("invalid status: %s", t.Status)
	}

	if t.Command == "" && len(t.Args) == 0 {
		return fmt.Errorf("task must have either command or args")
	}

	return nil
}

// String returns a string representation of the task
func (t *Task) String() string {
	return fmt.Sprintf("Task{ID: %s, Name: %s, Runner: %s, Status: %s}",
		t.ID, t.Name, t.Runner, t.Status)
}
