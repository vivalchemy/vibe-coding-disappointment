package task

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// MetadataManager manages task metadata
type MetadataManager struct {
	metadata map[string]*TaskMetadata
	mu       sync.RWMutex
}

// TaskMetadata represents comprehensive metadata for a task
type TaskMetadata struct {
	// Core identification
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`

	// Discovery information
	DiscoveredAt time.Time `json:"discovered_at"`
	Source       string    `json:"source"`
	FilePath     string    `json:"file_path"`
	LineNumber   int       `json:"line_number,omitempty"`

	// Runner information
	Runner     RunnerType      `json:"runner"`
	RunnerInfo *RunnerMetadata `json:"runner_info,omitempty"`

	// Task configuration
	Command     string            `json:"command,omitempty"`
	Args        []string          `json:"args,omitempty"`
	Directory   string            `json:"directory,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`

	// Dependencies and relationships
	Dependencies []string `json:"dependencies,omitempty"`
	Dependents   []string `json:"dependents,omitempty"`
	Tags         []string `json:"tags,omitempty"`

	// Execution metadata
	LastRun    *ExecutionMetadata   `json:"last_run,omitempty"`
	RunHistory []*ExecutionMetadata `json:"run_history,omitempty"`
	Statistics *TaskStatistics      `json:"statistics,omitempty"`

	// File and content metadata
	FileInfo *FileMetadata    `json:"file_info,omitempty"`
	Content  *ContentMetadata `json:"content,omitempty"`

	// Custom metadata
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]any    `json:"annotations,omitempty"`

	// Validation and health
	Valid    bool     `json:"valid"`
	Errors   []string `json:"errors,omitempty"`
	Warnings []string `json:"warnings,omitempty"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// RunnerMetadata contains metadata specific to the task runner
type RunnerMetadata struct {
	Type         RunnerType      `json:"type"`
	Version      string          `json:"version,omitempty"`
	Executable   string          `json:"executable,omitempty"`
	ConfigFile   string          `json:"config_file,omitempty"`
	ProjectType  string          `json:"project_type,omitempty"`
	Framework    string          `json:"framework,omitempty"`
	Language     string          `json:"language,omitempty"`
	Features     []string        `json:"features,omitempty"`
	Capabilities map[string]bool `json:"capabilities,omitempty"`
	Settings     map[string]any  `json:"settings,omitempty"`
}

// ExecutionMetadata contains metadata about task execution
type ExecutionMetadata struct {
	ID          string            `json:"id"`
	StartTime   time.Time         `json:"start_time"`
	EndTime     time.Time         `json:"end_time"`
	Duration    time.Duration     `json:"duration"`
	ExitCode    int               `json:"exit_code"`
	Success     bool              `json:"success"`
	Output      string            `json:"output,omitempty"`
	Error       string            `json:"error,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
	Host        string            `json:"host,omitempty"`
	User        string            `json:"user,omitempty"`
	PID         int               `json:"pid,omitempty"`
	MemoryUsage int64             `json:"memory_usage,omitempty"`
	CPUTime     time.Duration     `json:"cpu_time,omitempty"`
}

// TaskStatistics contains statistical information about task execution
type TaskStatistics struct {
	TotalRuns      int     `json:"total_runs"`
	SuccessfulRuns int     `json:"successful_runs"`
	FailedRuns     int     `json:"failed_runs"`
	SuccessRate    float64 `json:"success_rate"`

	AverageDuration time.Duration `json:"average_duration"`
	MinDuration     time.Duration `json:"min_duration"`
	MaxDuration     time.Duration `json:"max_duration"`

	LastSuccess time.Time `json:"last_success"`
	LastFailure time.Time `json:"last_failure"`

	CommonExitCodes map[int]int    `json:"common_exit_codes"`
	FrequentErrors  map[string]int `json:"frequent_errors"`
}

// FileMetadata contains metadata about the task's source file
type FileMetadata struct {
	Path        string    `json:"path"`
	Size        int64     `json:"size"`
	ModTime     time.Time `json:"mod_time"`
	Permissions string    `json:"permissions"`
	Hash        string    `json:"hash,omitempty"`
	Encoding    string    `json:"encoding,omitempty"`
	MimeType    string    `json:"mime_type,omitempty"`
}

// ContentMetadata contains metadata about the task's content
type ContentMetadata struct {
	LineCount  int                `json:"line_count"`
	WordCount  int                `json:"word_count"`
	CharCount  int                `json:"char_count"`
	Language   string             `json:"language,omitempty"`
	Syntax     string             `json:"syntax,omitempty"`
	Keywords   []string           `json:"keywords,omitempty"`
	Comments   []string           `json:"comments,omitempty"`
	Complexity *ComplexityMetrics `json:"complexity,omitempty"`
}

// ComplexityMetrics contains code complexity metrics
type ComplexityMetrics struct {
	CyclomaticComplexity int     `json:"cyclomatic_complexity"`
	CognitiveComplexity  int     `json:"cognitive_complexity"`
	Maintainability      float64 `json:"maintainability"`
	TechnicalDebt        string  `json:"technical_debt,omitempty"`
}

// MetadataQuery represents a query for task metadata
type MetadataQuery struct {
	// Filters
	IDs     []string          `json:"ids,omitempty"`
	Names   []string          `json:"names,omitempty"`
	Runners []RunnerType      `json:"runners,omitempty"`
	Sources []string          `json:"sources,omitempty"`
	Tags    []string          `json:"tags,omitempty"`
	Labels  map[string]string `json:"labels,omitempty"`

	// Content filters
	NamePattern string `json:"name_pattern,omitempty"`
	PathPattern string `json:"path_pattern,omitempty"`

	// Status filters
	Valid       *bool `json:"valid,omitempty"`
	HasErrors   *bool `json:"has_errors,omitempty"`
	HasWarnings *bool `json:"has_warnings,omitempty"`

	// Time filters
	CreatedAfter  *time.Time `json:"created_after,omitempty"`
	CreatedBefore *time.Time `json:"created_before,omitempty"`
	UpdatedAfter  *time.Time `json:"updated_after,omitempty"`
	UpdatedBefore *time.Time `json:"updated_before,omitempty"`

	// Execution filters
	LastRunAfter   *time.Time `json:"last_run_after,omitempty"`
	LastRunBefore  *time.Time `json:"last_run_before,omitempty"`
	MinSuccessRate *float64   `json:"min_success_rate,omitempty"`
	MaxSuccessRate *float64   `json:"max_success_rate,omitempty"`

	// Sorting and pagination
	SortBy    string `json:"sort_by,omitempty"`
	SortOrder string `json:"sort_order,omitempty"`
	Limit     int    `json:"limit,omitempty"`
	Offset    int    `json:"offset,omitempty"`

	// Include options
	IncludeHistory    bool `json:"include_history"`
	IncludeStatistics bool `json:"include_statistics"`
	IncludeContent    bool `json:"include_content"`
}

// NewMetadataManager creates a new metadata manager
func NewMetadataManager() *MetadataManager {
	return &MetadataManager{
		metadata: make(map[string]*TaskMetadata),
	}
}

// Add adds or updates task metadata
func (mm *MetadataManager) Add(metadata *TaskMetadata) {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	now := time.Now()

	if existing, exists := mm.metadata[metadata.ID]; exists {
		// Update existing metadata
		metadata.CreatedAt = existing.CreatedAt
		metadata.UpdatedAt = now

		// Preserve run history if not provided
		if len(metadata.RunHistory) == 0 && len(existing.RunHistory) > 0 {
			metadata.RunHistory = existing.RunHistory
		}

		// Preserve statistics if not provided
		if metadata.Statistics == nil && existing.Statistics != nil {
			metadata.Statistics = existing.Statistics
		}
	} else {
		// New metadata
		metadata.CreatedAt = now
		metadata.UpdatedAt = now
	}

	mm.metadata[metadata.ID] = metadata
}

// Get retrieves task metadata by ID
func (mm *MetadataManager) Get(id string) (*TaskMetadata, bool) {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	metadata, exists := mm.metadata[id]
	if !exists {
		return nil, false
	}

	// Return a copy to prevent external modification
	copy := *metadata
	return &copy, true
}

// Remove removes task metadata
func (mm *MetadataManager) Remove(id string) bool {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	_, exists := mm.metadata[id]
	if exists {
		delete(mm.metadata, id)
	}

	return exists
}

// List returns all task metadata
func (mm *MetadataManager) List() []*TaskMetadata {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	result := make([]*TaskMetadata, 0, len(mm.metadata))
	for _, metadata := range mm.metadata {
		copy := *metadata
		result = append(result, &copy)
	}

	// Sort by name for consistent ordering
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})

	return result
}

// Query searches for task metadata based on criteria
func (mm *MetadataManager) Query(query *MetadataQuery) ([]*TaskMetadata, error) {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	var result []*TaskMetadata

	for _, metadata := range mm.metadata {
		if mm.matchesQuery(metadata, query) {
			copy := *metadata

			// Filter included data based on query options
			if !query.IncludeHistory {
				copy.RunHistory = nil
			}
			if !query.IncludeStatistics {
				copy.Statistics = nil
			}
			if !query.IncludeContent {
				copy.Content = nil
			}

			result = append(result, &copy)
		}
	}

	// Sort results
	mm.sortMetadata(result, query.SortBy, query.SortOrder)

	// Apply pagination
	if query.Offset > 0 || query.Limit > 0 {
		start := query.Offset
		if start > len(result) {
			start = len(result)
		}

		end := len(result)
		if query.Limit > 0 && start+query.Limit < end {
			end = start + query.Limit
		}

		result = result[start:end]
	}

	return result, nil
}

// AddExecution adds execution metadata to a task
func (mm *MetadataManager) AddExecution(taskID string, execution *ExecutionMetadata) error {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	metadata, exists := mm.metadata[taskID]
	if !exists {
		return fmt.Errorf("task metadata not found: %s", taskID)
	}

	// Add to history
	if metadata.RunHistory == nil {
		metadata.RunHistory = []*ExecutionMetadata{}
	}
	metadata.RunHistory = append(metadata.RunHistory, execution)

	// Limit history size
	maxHistory := 100
	if len(metadata.RunHistory) > maxHistory {
		metadata.RunHistory = metadata.RunHistory[len(metadata.RunHistory)-maxHistory:]
	}

	// Update last run
	metadata.LastRun = execution

	// Update statistics
	mm.updateStatistics(metadata, execution)

	// Update timestamp
	metadata.UpdatedAt = time.Now()

	return nil
}

// GetStatistics returns aggregated statistics for all tasks
func (mm *MetadataManager) GetStatistics() map[string]any {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	stats := map[string]any{
		"total_tasks":           len(mm.metadata),
		"valid_tasks":           0,
		"invalid_tasks":         0,
		"tasks_with_errors":     0,
		"tasks_with_warnings":   0,
		"runners":               make(map[string]int),
		"sources":               make(map[string]int),
		"total_executions":      0,
		"successful_executions": 0,
		"failed_executions":     0,
	}

	runners := make(map[string]int)
	sources := make(map[string]int)
	var totalExecs, successfulExecs, failedExecs int

	for _, metadata := range mm.metadata {
		if metadata.Valid {
			stats["valid_tasks"] = stats["valid_tasks"].(int) + 1
		} else {
			stats["invalid_tasks"] = stats["invalid_tasks"].(int) + 1
		}

		if len(metadata.Errors) > 0 {
			stats["tasks_with_errors"] = stats["tasks_with_errors"].(int) + 1
		}

		if len(metadata.Warnings) > 0 {
			stats["tasks_with_warnings"] = stats["tasks_with_warnings"].(int) + 1
		}

		runners[string(metadata.Runner)]++
		sources[metadata.Source]++

		if metadata.Statistics != nil {
			totalExecs += metadata.Statistics.TotalRuns
			successfulExecs += metadata.Statistics.SuccessfulRuns
			failedExecs += metadata.Statistics.FailedRuns
		}
	}

	stats["runners"] = runners
	stats["sources"] = sources
	stats["total_executions"] = totalExecs
	stats["successful_executions"] = successfulExecs
	stats["failed_executions"] = failedExecs

	if totalExecs > 0 {
		stats["overall_success_rate"] = float64(successfulExecs) / float64(totalExecs)
	}

	return stats
}

// Export exports metadata to various formats
func (mm *MetadataManager) Export(format string) ([]byte, error) {
	mm.mu.RLock()
	defer mm.mu.RUnlock()

	switch strings.ToLower(format) {
	case "json":
		return json.MarshalIndent(mm.metadata, "", "  ")
	default:
		return nil, fmt.Errorf("unsupported export format: %s", format)
	}
}

// Import imports metadata from various formats
func (mm *MetadataManager) Import(data []byte, format string) error {
	var imported map[string]*TaskMetadata

	switch strings.ToLower(format) {
	case "json":
		if err := json.Unmarshal(data, &imported); err != nil {
			return fmt.Errorf("failed to unmarshal JSON: %w", err)
		}
	default:
		return fmt.Errorf("unsupported import format: %s", format)
	}

	mm.mu.Lock()
	defer mm.mu.Unlock()

	for id, metadata := range imported {
		mm.metadata[id] = metadata
	}

	return nil
}

// Clear removes all metadata
func (mm *MetadataManager) Clear() {
	mm.mu.Lock()
	defer mm.mu.Unlock()

	mm.metadata = make(map[string]*TaskMetadata)
}

// Private methods

// matchesQuery checks if metadata matches the query criteria
func (mm *MetadataManager) matchesQuery(metadata *TaskMetadata, query *MetadataQuery) bool {
	// ID filter
	if len(query.IDs) > 0 && !mm.contains(query.IDs, metadata.ID) {
		return false
	}

	// Name filter
	if len(query.Names) > 0 && !mm.contains(query.Names, metadata.Name) {
		return false
	}

	// Runner filter
	if len(query.Runners) > 0 && !mm.containsRunner(query.Runners, metadata.Runner) {
		return false
	}

	// Source filter
	if len(query.Sources) > 0 && !mm.contains(query.Sources, metadata.Source) {
		return false
	}

	// Tags filter
	if len(query.Tags) > 0 && !mm.containsAnyTag(metadata.Tags, query.Tags) {
		return false
	}

	// Labels filter
	if len(query.Labels) > 0 && !mm.matchesLabels(metadata.Labels, query.Labels) {
		return false
	}

	// Pattern filters
	if query.NamePattern != "" && !mm.matchesPattern(metadata.Name, query.NamePattern) {
		return false
	}

	if query.PathPattern != "" && !mm.matchesPattern(metadata.FilePath, query.PathPattern) {
		return false
	}

	// Status filters
	if query.Valid != nil && metadata.Valid != *query.Valid {
		return false
	}

	if query.HasErrors != nil && (len(metadata.Errors) > 0) != *query.HasErrors {
		return false
	}

	if query.HasWarnings != nil && (len(metadata.Warnings) > 0) != *query.HasWarnings {
		return false
	}

	// Time filters
	if query.CreatedAfter != nil && metadata.CreatedAt.Before(*query.CreatedAfter) {
		return false
	}

	if query.CreatedBefore != nil && metadata.CreatedAt.After(*query.CreatedBefore) {
		return false
	}

	if query.UpdatedAfter != nil && metadata.UpdatedAt.Before(*query.UpdatedAfter) {
		return false
	}

	if query.UpdatedBefore != nil && metadata.UpdatedAt.After(*query.UpdatedBefore) {
		return false
	}

	// Execution filters
	if metadata.LastRun != nil {
		if query.LastRunAfter != nil && metadata.LastRun.StartTime.Before(*query.LastRunAfter) {
			return false
		}

		if query.LastRunBefore != nil && metadata.LastRun.StartTime.After(*query.LastRunBefore) {
			return false
		}
	}

	// Success rate filters
	if metadata.Statistics != nil {
		if query.MinSuccessRate != nil && metadata.Statistics.SuccessRate < *query.MinSuccessRate {
			return false
		}

		if query.MaxSuccessRate != nil && metadata.Statistics.SuccessRate > *query.MaxSuccessRate {
			return false
		}
	}

	return true
}

// sortMetadata sorts metadata based on criteria
func (mm *MetadataManager) sortMetadata(metadata []*TaskMetadata, sortBy, sortOrder string) {
	if sortBy == "" {
		sortBy = "name"
	}

	ascending := sortOrder != "desc"

	sort.Slice(metadata, func(i, j int) bool {
		var less bool

		switch sortBy {
		case "name":
			less = metadata[i].Name < metadata[j].Name
		case "created_at":
			less = metadata[i].CreatedAt.Before(metadata[j].CreatedAt)
		case "updated_at":
			less = metadata[i].UpdatedAt.Before(metadata[j].UpdatedAt)
		case "runner":
			less = string(metadata[i].Runner) < string(metadata[j].Runner)
		case "source":
			less = metadata[i].Source < metadata[j].Source
		case "success_rate":
			rate1 := 0.0
			if metadata[i].Statistics != nil {
				rate1 = metadata[i].Statistics.SuccessRate
			}
			rate2 := 0.0
			if metadata[j].Statistics != nil {
				rate2 = metadata[j].Statistics.SuccessRate
			}
			less = rate1 < rate2
		default:
			less = metadata[i].Name < metadata[j].Name
		}

		if ascending {
			return less
		}
		return !less
	})
}

// updateStatistics updates task statistics with new execution data
func (mm *MetadataManager) updateStatistics(metadata *TaskMetadata, execution *ExecutionMetadata) {
	if metadata.Statistics == nil {
		metadata.Statistics = &TaskStatistics{
			CommonExitCodes: make(map[int]int),
			FrequentErrors:  make(map[string]int),
		}
	}

	stats := metadata.Statistics
	stats.TotalRuns++

	if execution.Success {
		stats.SuccessfulRuns++
		stats.LastSuccess = execution.EndTime
	} else {
		stats.FailedRuns++
		stats.LastFailure = execution.EndTime

		// Track error frequency
		if execution.Error != "" {
			stats.FrequentErrors[execution.Error]++
		}
	}

	// Update success rate
	if stats.TotalRuns > 0 {
		stats.SuccessRate = float64(stats.SuccessfulRuns) / float64(stats.TotalRuns)
	}

	// Update duration statistics
	if stats.TotalRuns == 1 {
		stats.AverageDuration = execution.Duration
		stats.MinDuration = execution.Duration
		stats.MaxDuration = execution.Duration
	} else {
		// Update average
		total := stats.AverageDuration * time.Duration(stats.TotalRuns-1)
		stats.AverageDuration = (total + execution.Duration) / time.Duration(stats.TotalRuns)

		// Update min/max
		if execution.Duration < stats.MinDuration {
			stats.MinDuration = execution.Duration
		}
		if execution.Duration > stats.MaxDuration {
			stats.MaxDuration = execution.Duration
		}
	}

	// Track exit codes
	stats.CommonExitCodes[execution.ExitCode]++
}

// Helper methods

func (mm *MetadataManager) contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func (mm *MetadataManager) containsRunner(slice []RunnerType, item RunnerType) bool {
	for _, r := range slice {
		if r == item {
			return true
		}
	}
	return false
}

func (mm *MetadataManager) containsAnyTag(taskTags, queryTags []string) bool {
	for _, queryTag := range queryTags {
		for _, taskTag := range taskTags {
			if taskTag == queryTag {
				return true
			}
		}
	}
	return false
}

func (mm *MetadataManager) matchesLabels(taskLabels, queryLabels map[string]string) bool {
	for key, value := range queryLabels {
		if taskValue, exists := taskLabels[key]; !exists || taskValue != value {
			return false
		}
	}
	return true
}

func (mm *MetadataManager) matchesPattern(text, pattern string) bool {
	// Simple wildcard matching
	return strings.Contains(strings.ToLower(text), strings.ToLower(pattern))
}

// NewTaskMetadata creates new task metadata
func NewTaskMetadata(id, name string) *TaskMetadata {
	now := time.Now()

	return &TaskMetadata{
		ID:           id,
		Name:         name,
		DiscoveredAt: now,
		CreatedAt:    now,
		UpdatedAt:    now,
		Valid:        true,
		Environment:  make(map[string]string),
		Labels:       make(map[string]string),
		Annotations:  make(map[string]any),
	}
}

// AddError adds an error to task metadata
func (tm *TaskMetadata) AddError(err string) {
	if tm.Errors == nil {
		tm.Errors = []string{}
	}
	tm.Errors = append(tm.Errors, err)
	tm.Valid = false
	tm.UpdatedAt = time.Now()
}

// AddWarning adds a warning to task metadata
func (tm *TaskMetadata) AddWarning(warning string) {
	if tm.Warnings == nil {
		tm.Warnings = []string{}
	}
	tm.Warnings = append(tm.Warnings, warning)
	tm.UpdatedAt = time.Now()
}

// AddTag adds a tag to task metadata
func (tm *TaskMetadata) AddTag(tag string) {
	if tm.Tags == nil {
		tm.Tags = []string{}
	}

	// Check if tag already exists
	for _, existingTag := range tm.Tags {
		if existingTag == tag {
			return
		}
	}

	tm.Tags = append(tm.Tags, tag)
	tm.UpdatedAt = time.Now()
}

// SetLabel sets a label on task metadata
func (tm *TaskMetadata) SetLabel(key, value string) {
	if tm.Labels == nil {
		tm.Labels = make(map[string]string)
	}
	tm.Labels[key] = value
	tm.UpdatedAt = time.Now()
}

// SetAnnotation sets an annotation on task metadata
func (tm *TaskMetadata) SetAnnotation(key string, value any) {
	if tm.Annotations == nil {
		tm.Annotations = make(map[string]any)
	}
	tm.Annotations[key] = value
	tm.UpdatedAt = time.Now()
}

// GetLastSuccessfulRun returns the last successful execution
func (tm *TaskMetadata) GetLastSuccessfulRun() *ExecutionMetadata {
	if tm.RunHistory == nil {
		return nil
	}

	// Search backwards for the last successful run
	for i := len(tm.RunHistory) - 1; i >= 0; i-- {
		if tm.RunHistory[i].Success {
			return tm.RunHistory[i]
		}
	}

	return nil
}

// GetLastFailedRun returns the last failed execution
func (tm *TaskMetadata) GetLastFailedRun() *ExecutionMetadata {
	if tm.RunHistory == nil {
		return nil
	}

	// Search backwards for the last failed run
	for i := len(tm.RunHistory) - 1; i >= 0; i-- {
		if !tm.RunHistory[i].Success {
			return tm.RunHistory[i]
		}
	}

	return nil
}

// IsHealthy returns whether the task is considered healthy
func (tm *TaskMetadata) IsHealthy() bool {
	if !tm.Valid || len(tm.Errors) > 0 {
		return false
	}

	if tm.Statistics == nil {
		return true // No execution history, assume healthy
	}

	// Consider healthy if success rate is above 80%
	return tm.Statistics.SuccessRate >= 0.8
}

// GetComplexityScore returns a complexity score for the task
func (tm *TaskMetadata) GetComplexityScore() int {
	score := 0

	// Base complexity from dependencies
	score += len(tm.Dependencies) * 2

	// Complexity from content metrics
	if tm.Content != nil && tm.Content.Complexity != nil {
		score += tm.Content.Complexity.CyclomaticComplexity
		score += tm.Content.Complexity.CognitiveComplexity
	}

	// Complexity from execution variability
	if tm.Statistics != nil && tm.Statistics.TotalRuns > 5 {
		// High variability in execution time indicates complexity
		if tm.Statistics.MaxDuration > 0 {
			ratio := float64(tm.Statistics.MaxDuration) / float64(tm.Statistics.MinDuration)
			if ratio > 3.0 {
				score += 10
			}
		}
	}

	return score
}
