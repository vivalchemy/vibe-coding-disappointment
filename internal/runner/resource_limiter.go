package runner

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/vivalchemy/wake/internal/config"
	"github.com/vivalchemy/wake/pkg/logger"
	"github.com/vivalchemy/wake/pkg/task"
)

// ResourceLimiter manages system resource limits for task execution
type ResourceLimiter struct {
	config *config.RunnersConfig
	logger logger.Logger

	// Resource tracking
	activeExecutions map[string]*ResourceAllocation
	mu               sync.RWMutex

	// Limits
	maxConcurrentTasks int
	maxMemoryMB        int64
	maxCPUPercent      float64

	// Resource monitoring
	systemMonitor *SystemMonitor

	// Semaphores for limiting resources
	concurrencySem chan struct{}

	// Statistics
	stats *ResourceStats
}

// ResourceAllocation represents resources allocated to a task
type ResourceAllocation struct {
	TaskID       string
	TaskName     string
	RunnerType   task.RunnerType
	AllocatedAt  time.Time
	EstimatedMem int64   // Estimated memory usage in MB
	EstimatedCPU float64 // Estimated CPU usage percentage
	Priority     int     // Task priority (higher = more important)

	// Actual usage (updated during execution)
	CurrentMem int64
	CurrentCPU float64
	PeakMem    int64
	PeakCPU    float64
}

// ResourceStats tracks resource usage statistics
type ResourceStats struct {
	// Current state
	ActiveTasks       int     `json:"active_tasks"`
	TotalMemoryMB     int64   `json:"total_memory_mb"`
	UsedMemoryMB      int64   `json:"used_memory_mb"`
	MemoryUtilization float64 `json:"memory_utilization"`
	CPUUtilization    float64 `json:"cpu_utilization"`

	// Historical data
	TotalTasksExecuted   int64         `json:"total_tasks_executed"`
	AverageExecutionTime time.Duration `json:"average_execution_time"`
	ResourceWaitTime     time.Duration `json:"resource_wait_time"`

	// Limits
	MaxConcurrentTasks int     `json:"max_concurrent_tasks"`
	MaxMemoryMB        int64   `json:"max_memory_mb"`
	MaxCPUPercent      float64 `json:"max_cpu_percent"`

	// Rejections
	RejectedDueToMemory      int64 `json:"rejected_due_to_memory"`
	RejectedDueToCPU         int64 `json:"rejected_due_to_cpu"`
	RejectedDueToConcurrency int64 `json:"rejected_due_to_concurrency"`

	mu sync.RWMutex
}

// SystemMonitor monitors system resource usage
type SystemMonitor struct {
	logger   logger.Logger
	interval time.Duration

	// System stats
	totalMemoryMB int64
	numCPUs       int

	// Current usage
	currentMemoryMB int64
	currentCPU      float64

	// Control
	stopChan chan struct{}
	done     chan struct{}
	mu       sync.RWMutex
}

// SystemResourceUsage represents current system resource usage
type SystemResourceUsage struct {
	TotalMemoryMB     int64     `json:"total_memory_mb"`
	UsedMemoryMB      int64     `json:"used_memory_mb"`
	AvailableMemoryMB int64     `json:"available_memory_mb"`
	MemoryPercent     float64   `json:"memory_percent"`
	CPUPercent        float64   `json:"cpu_percent"`
	NumCPUs           int       `json:"num_cpus"`
	LoadAverage       []float64 `json:"load_average,omitempty"`
	Timestamp         time.Time `json:"timestamp"`
}

// NewResourceLimiter creates a new resource limiter
func NewResourceLimiter(cfg *config.RunnersConfig, log logger.Logger) (*ResourceLimiter, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config cannot be nil")
	}

	// Set default limits based on system resources
	maxConcurrent := runtime.NumCPU() * 2 // Default: 2x CPU cores
	maxMemoryMB := int64(8 * 1024)        // Default: 8GB
	maxCPUPercent := 80.0                 // Default: 80% CPU

	// Override with config values if provided
	// TODO: Add these fields to config.RunnersConfig when available
	// if cfg.MaxConcurrentTasks > 0 {
	//     maxConcurrent = cfg.MaxConcurrentTasks
	// }

	// Create system monitor
	systemMonitor := NewSystemMonitor(log)
	if err := systemMonitor.Start(); err != nil {
		return nil, fmt.Errorf("failed to start system monitor: %w", err)
	}

	limiter := &ResourceLimiter{
		config:             cfg,
		logger:             log.WithGroup("resource-limiter"),
		activeExecutions:   make(map[string]*ResourceAllocation),
		maxConcurrentTasks: maxConcurrent,
		maxMemoryMB:        maxMemoryMB,
		maxCPUPercent:      maxCPUPercent,
		systemMonitor:      systemMonitor,
		concurrencySem:     make(chan struct{}, maxConcurrent),
		stats: &ResourceStats{
			MaxConcurrentTasks: maxConcurrent,
			MaxMemoryMB:        maxMemoryMB,
			MaxCPUPercent:      maxCPUPercent,
		},
	}

	return limiter, nil
}

// Acquire attempts to acquire resources for a task
func (rl *ResourceLimiter) Acquire(t *task.Task) error {
	if t == nil {
		return fmt.Errorf("task cannot be nil")
	}

	rl.logger.Debug("Acquiring resources", "task", t.Name, "runner", t.Runner)

	// Estimate resource requirements
	allocation := rl.estimateResources(t)

	// Check if we can acquire resources
	if err := rl.checkResourceAvailability(allocation); err != nil {
		rl.updateRejectionStats(err)
		return err
	}

	// Try to acquire concurrency semaphore (non-blocking check first)
	select {
	case rl.concurrencySem <- struct{}{}:
		// Successfully acquired concurrency slot
	default:
		rl.stats.mu.Lock()
		rl.stats.RejectedDueToConcurrency++
		rl.stats.mu.Unlock()
		return fmt.Errorf("maximum concurrent tasks limit reached (%d)", rl.maxConcurrentTasks)
	}

	// Register the allocation
	rl.mu.Lock()
	rl.activeExecutions[t.ID] = allocation
	rl.mu.Unlock()

	// Update statistics
	rl.updateAcquisitionStats(allocation)

	rl.logger.Info("Resources acquired",
		"task", t.Name,
		"estimated_memory_mb", allocation.EstimatedMem,
		"estimated_cpu_percent", allocation.EstimatedCPU)

	return nil
}

// Release releases resources for a task
func (rl *ResourceLimiter) Release(t *task.Task) {
	if t == nil {
		return
	}

	rl.logger.Debug("Releasing resources", "task", t.Name)

	// Remove from active executions
	rl.mu.Lock()
	allocation, exists := rl.activeExecutions[t.ID]
	if exists {
		delete(rl.activeExecutions, t.ID)
	}
	rl.mu.Unlock()

	if !exists {
		rl.logger.Warn("Attempted to release resources for unknown task", "task", t.Name)
		return
	}

	// Release concurrency semaphore
	select {
	case <-rl.concurrencySem:
		// Successfully released
	default:
		rl.logger.Warn("Concurrency semaphore was already empty", "task", t.Name)
	}

	// Update statistics
	rl.updateReleaseStats(allocation)

	rl.logger.Info("Resources released",
		"task", t.Name,
		"peak_memory_mb", allocation.PeakMem,
		"peak_cpu_percent", allocation.PeakCPU)
}

// estimateResources estimates resource requirements for a task
func (rl *ResourceLimiter) estimateResources(t *task.Task) *ResourceAllocation {
	allocation := &ResourceAllocation{
		TaskID:      t.ID,
		TaskName:    t.Name,
		RunnerType:  t.Runner,
		AllocatedAt: time.Now(),
		Priority:    rl.calculateTaskPriority(t),
	}

	// Estimate based on runner type and task characteristics
	switch t.Runner {
	case task.RunnerMake:
		allocation.EstimatedMem = rl.estimateMemoryForMake(t)
		allocation.EstimatedCPU = rl.estimateCPUForMake(t)

	case task.RunnerNpm, task.RunnerYarn, task.RunnerPnpm, task.RunnerBun:
		allocation.EstimatedMem = rl.estimateMemoryForNode(t)
		allocation.EstimatedCPU = rl.estimateCPUForNode(t)

	case task.RunnerJust:
		allocation.EstimatedMem = rl.estimateMemoryForJust(t)
		allocation.EstimatedCPU = rl.estimateCPUForJust(t)

	case task.RunnerTask:
		allocation.EstimatedMem = rl.estimateMemoryForTask(t)
		allocation.EstimatedCPU = rl.estimateCPUForTask(t)

	case task.RunnerPython:
		allocation.EstimatedMem = rl.estimateMemoryForPython(t)
		allocation.EstimatedCPU = rl.estimateCPUForPython(t)

	default:
		// Default estimates
		allocation.EstimatedMem = 512  // 512MB
		allocation.EstimatedCPU = 25.0 // 25% CPU
	}

	return allocation
}

// estimateMemoryForMake estimates memory usage for make tasks
func (rl *ResourceLimiter) estimateMemoryForMake(t *task.Task) int64 {
	baseMem := int64(256) // 256MB base

	// Increase for build-related tasks
	if rl.isCompilationTask(t) {
		baseMem *= 4 // 1GB for compilation
	}

	return baseMem
}

// estimateCPUForMake estimates CPU usage for make tasks
func (rl *ResourceLimiter) estimateCPUForMake(t *task.Task) float64 {
	baseCPU := 50.0 // 50% base

	if rl.isCompilationTask(t) {
		baseCPU = 80.0 // Higher CPU for compilation
	}

	return baseCPU
}

// estimateMemoryForNode estimates memory usage for Node.js tasks
func (rl *ResourceLimiter) estimateMemoryForNode(t *task.Task) int64 {
	baseMem := int64(512) // 512MB base for Node.js

	// Increase for specific task types
	if rl.isBuildTask(t) {
		baseMem *= 2 // 1GB for builds
	} else if rl.isTestTask(t) {
		baseMem = int64(1024) // 1GB for tests
	}

	return baseMem
}

// estimateCPUForNode estimates CPU usage for Node.js tasks
func (rl *ResourceLimiter) estimateCPUForNode(t *task.Task) float64 {
	baseCPU := 60.0 // 60% base

	if rl.isBuildTask(t) {
		baseCPU = 75.0
	} else if rl.isTestTask(t) {
		baseCPU = 40.0 // Tests often wait for I/O
	}

	return baseCPU
}

// estimateMemoryForJust estimates memory usage for Just tasks
func (rl *ResourceLimiter) estimateMemoryForJust(t *task.Task) int64 {
	return 256 // 256MB default for Just tasks
}

// estimateCPUForJust estimates CPU usage for Just tasks
func (rl *ResourceLimiter) estimateCPUForJust(t *task.Task) float64 {
	return 30.0 // 30% default
}

// estimateMemoryForTask estimates memory usage for Task runner tasks
func (rl *ResourceLimiter) estimateMemoryForTask(t *task.Task) int64 {
	return 512 // 512MB default
}

// estimateCPUForTask estimates CPU usage for Task runner tasks
func (rl *ResourceLimiter) estimateCPUForTask(t *task.Task) float64 {
	return 50.0 // 50% default
}

// estimateMemoryForPython estimates memory usage for Python tasks
func (rl *ResourceLimiter) estimateMemoryForPython(t *task.Task) int64 {
	baseMem := int64(512) // 512MB base

	// Increase for data processing tasks
	if rl.isDataProcessingTask(t) {
		baseMem *= 4 // 2GB for data processing
	}

	return baseMem
}

// estimateCPUForPython estimates CPU usage for Python tasks
func (rl *ResourceLimiter) estimateCPUForPython(t *task.Task) float64 {
	baseCPU := 40.0 // 40% base

	if rl.isDataProcessingTask(t) {
		baseCPU = 70.0
	}

	return baseCPU
}

// Task classification helpers
func (rl *ResourceLimiter) isCompilationTask(t *task.Task) bool {
	name := strings.ToLower(t.Name)
	return strings.Contains(name, "build") ||
		strings.Contains(name, "compile") ||
		strings.Contains(name, "make")
}

func (rl *ResourceLimiter) isBuildTask(t *task.Task) bool {
	name := strings.ToLower(t.Name)
	return strings.Contains(name, "build") ||
		strings.Contains(name, "bundle") ||
		strings.Contains(name, "webpack") ||
		strings.Contains(name, "rollup")
}

func (rl *ResourceLimiter) isTestTask(t *task.Task) bool {
	name := strings.ToLower(t.Name)
	return strings.Contains(name, "test") ||
		strings.Contains(name, "spec") ||
		strings.Contains(name, "jest") ||
		strings.Contains(name, "mocha")
}

func (rl *ResourceLimiter) isDataProcessingTask(t *task.Task) bool {
	name := strings.ToLower(t.Name)
	description := strings.ToLower(t.Description)

	keywords := []string{"data", "process", "analyze", "ml", "ai", "model", "train"}
	for _, keyword := range keywords {
		if strings.Contains(name, keyword) || strings.Contains(description, keyword) {
			return true
		}
	}
	return false
}

// calculateTaskPriority calculates priority for a task
func (rl *ResourceLimiter) calculateTaskPriority(t *task.Task) int {
	priority := 100 // Base priority

	// Increase priority for critical tasks
	if t.HasTag("critical") || t.HasTag("high-priority") {
		priority += 50
	}

	// Decrease priority for background tasks
	if t.HasTag("background") || t.HasTag("low-priority") {
		priority -= 30
	}

	// Adjust based on task type
	if rl.isTestTask(t) {
		priority += 20 // Tests are important
	} else if strings.Contains(strings.ToLower(t.Name), "clean") {
		priority -= 20 // Cleanup tasks are less urgent
	}

	return priority
}

// checkResourceAvailability checks if resources are available
func (rl *ResourceLimiter) checkResourceAvailability(allocation *ResourceAllocation) error {
	// Get current system usage
	systemUsage, err := rl.systemMonitor.GetCurrentUsage()
	if err != nil {
		rl.logger.Warn("Failed to get system usage", "error", err)
		// Continue with estimation
	}

	// Check memory limits
	currentUsage := rl.getCurrentResourceUsage()
	projectedMemory := currentUsage.UsedMemoryMB + allocation.EstimatedMem

	if projectedMemory > rl.maxMemoryMB {
		return &ResourceLimitError{
			Type:      "memory",
			Requested: allocation.EstimatedMem,
			Available: rl.maxMemoryMB - currentUsage.UsedMemoryMB,
			Limit:     rl.maxMemoryMB,
		}
	}

	// Check CPU limits
	if systemUsage != nil {
		projectedCPU := systemUsage.CPUPercent + allocation.EstimatedCPU
		if projectedCPU > rl.maxCPUPercent {
			return &ResourceLimitError{
				Type:      "cpu",
				Requested: int64(allocation.EstimatedCPU),
				Available: int64(rl.maxCPUPercent - systemUsage.CPUPercent),
				Limit:     int64(rl.maxCPUPercent),
			}
		}
	}

	return nil
}

// ResourceLimitError represents a resource limit error
type ResourceLimitError struct {
	Type      string
	Requested int64
	Available int64
	Limit     int64
}

func (e *ResourceLimitError) Error() string {
	return fmt.Sprintf("%s limit exceeded: requested %d, available %d, limit %d",
		e.Type, e.Requested, e.Available, e.Limit)
}

// getCurrentResourceUsage gets current resource usage
func (rl *ResourceLimiter) getCurrentResourceUsage() *ResourceStats {
	rl.stats.mu.RLock()
	defer rl.stats.mu.RUnlock()

	// Calculate current usage from active executions
	rl.mu.RLock()
	usedMemory := int64(0)
	activeTasks := len(rl.activeExecutions)

	for _, allocation := range rl.activeExecutions {
		if allocation.CurrentMem > 0 {
			usedMemory += allocation.CurrentMem
		} else {
			usedMemory += allocation.EstimatedMem
		}
	}
	rl.mu.RUnlock()

	stats := *rl.stats
	stats.ActiveTasks = activeTasks
	stats.UsedMemoryMB = usedMemory

	if stats.TotalMemoryMB > 0 {
		stats.MemoryUtilization = float64(usedMemory) / float64(stats.TotalMemoryMB) * 100
	}

	return &stats
}

// updateAcquisitionStats updates statistics when resources are acquired
func (rl *ResourceLimiter) updateAcquisitionStats(allocation *ResourceAllocation) {
	rl.stats.mu.Lock()
	defer rl.stats.mu.Unlock()

	rl.stats.TotalTasksExecuted++
}

// updateReleaseStats updates statistics when resources are released
func (rl *ResourceLimiter) updateReleaseStats(allocation *ResourceAllocation) {
	rl.stats.mu.Lock()
	defer rl.stats.mu.Unlock()

	// Update average execution time
	executionTime := time.Since(allocation.AllocatedAt)
	if rl.stats.TotalTasksExecuted == 1 {
		rl.stats.AverageExecutionTime = executionTime
	} else {
		// Running average
		total := time.Duration(rl.stats.TotalTasksExecuted)
		rl.stats.AverageExecutionTime = (rl.stats.AverageExecutionTime*(total-1) + executionTime) / total
	}
}

// updateRejectionStats updates rejection statistics
func (rl *ResourceLimiter) updateRejectionStats(err error) {
	if resourceErr, ok := err.(*ResourceLimitError); ok {
		rl.stats.mu.Lock()
		defer rl.stats.mu.Unlock()

		switch resourceErr.Type {
		case "memory":
			rl.stats.RejectedDueToMemory++
		case "cpu":
			rl.stats.RejectedDueToCPU++
		}
	}
}

// GetActiveAllocations returns current resource allocations
func (rl *ResourceLimiter) GetActiveAllocations() map[string]*ResourceAllocation {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	// Return a copy
	allocations := make(map[string]*ResourceAllocation)
	for k, v := range rl.activeExecutions {
		allocations[k] = v
	}

	return allocations
}

// GetResourceStats returns resource usage statistics
func (rl *ResourceLimiter) GetResourceStats() *ResourceStats {
	return rl.getCurrentResourceUsage()
}

// UpdateTaskUsage updates actual resource usage for a task
func (rl *ResourceLimiter) UpdateTaskUsage(taskID string, memoryMB int64, cpuPercent float64) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	if allocation, exists := rl.activeExecutions[taskID]; exists {
		allocation.CurrentMem = memoryMB
		allocation.CurrentCPU = cpuPercent

		// Update peaks
		if memoryMB > allocation.PeakMem {
			allocation.PeakMem = memoryMB
		}
		if cpuPercent > allocation.PeakCPU {
			allocation.PeakCPU = cpuPercent
		}
	}
}

// Shutdown gracefully shuts down the resource limiter
func (rl *ResourceLimiter) Shutdown() error {
	rl.logger.Info("Shutting down resource limiter")

	// Stop system monitor
	if err := rl.systemMonitor.Stop(); err != nil {
		return fmt.Errorf("failed to stop system monitor: %w", err)
	}

	// Wait for all tasks to complete or force release after timeout
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		rl.mu.RLock()
		activeCount := len(rl.activeExecutions)
		rl.mu.RUnlock()

		if activeCount == 0 {
			break
		}

		select {
		case <-timeout:
			rl.logger.Warn("Timeout waiting for tasks to complete, forcing shutdown")
			return nil
		case <-ticker.C:
			rl.logger.Debug("Waiting for tasks to complete", "active", activeCount)
		}
	}

	return nil
}

// NewSystemMonitor creates a new system monitor
func NewSystemMonitor(log logger.Logger) *SystemMonitor {
	// Get system information
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	return &SystemMonitor{
		logger:        log.WithGroup("system-monitor"),
		interval:      5 * time.Second,                   // Monitor every 5 seconds
		totalMemoryMB: int64(memStats.Sys / 1024 / 1024), // Approximation
		numCPUs:       runtime.NumCPU(),
		stopChan:      make(chan struct{}),
		done:          make(chan struct{}),
	}
}

// Start starts system monitoring
func (sm *SystemMonitor) Start() error {
	go sm.monitorSystem()
	return nil
}

// Stop stops system monitoring
func (sm *SystemMonitor) Stop() error {
	select {
	case sm.stopChan <- struct{}{}:
	default:
	}

	// Wait for monitoring to stop
	<-sm.done
	return nil
}

// monitorSystem monitors system resources
func (sm *SystemMonitor) monitorSystem() {
	defer close(sm.done)

	ticker := time.NewTicker(sm.interval)
	defer ticker.Stop()

	for {
		select {
		case <-sm.stopChan:
			return
		case <-ticker.C:
			sm.updateSystemStats()
		}
	}
}

// updateSystemStats updates system statistics
func (sm *SystemMonitor) updateSystemStats() {
	// Get memory stats
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	sm.mu.Lock()
	sm.currentMemoryMB = int64(memStats.Alloc / 1024 / 1024)
	// CPU monitoring would require platform-specific code
	sm.currentCPU = 0 // Placeholder
	sm.mu.Unlock()
}

// GetCurrentUsage returns current system resource usage
func (sm *SystemMonitor) GetCurrentUsage() (*SystemResourceUsage, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	return &SystemResourceUsage{
		TotalMemoryMB:     sm.totalMemoryMB,
		UsedMemoryMB:      sm.currentMemoryMB,
		AvailableMemoryMB: sm.totalMemoryMB - sm.currentMemoryMB,
		MemoryPercent:     float64(sm.currentMemoryMB) / float64(sm.totalMemoryMB) * 100,
		CPUPercent:        sm.currentCPU,
		NumCPUs:           sm.numCPUs,
		Timestamp:         time.Now(),
	}, nil
}
