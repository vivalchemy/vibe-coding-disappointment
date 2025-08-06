package discovery

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/vivalchemy/wake/internal/discovery/parsers"
	"github.com/vivalchemy/wake/pkg/logger"
	"github.com/vivalchemy/wake/pkg/task"
	"slices"
)

// Registry manages task discovery across different parsers and runners
type Registry struct {
	// Parsers
	parsers     map[task.RunnerType]Parser
	parserOrder []task.RunnerType

	// Configuration
	options *RegistryOptions

	// Cache
	cache *DiscoveryCache

	// State
	mu            sync.RWMutex
	lastDiscovery time.Time

	// Statistics
	stats *DiscoveryStats

	// Logger
	logger logger.Logger
}

// RegistryOptions contains configuration for the discovery registry
type RegistryOptions struct {
	// Discovery behavior
	MaxDepth       int
	FollowSymlinks bool
	IgnoreHidden   bool
	IgnorePatterns []string

	// Performance
	EnableCache     bool
	CacheTTL        time.Duration
	MaxCacheSize    int
	ConcurrentLimit int

	// Filtering
	EnabledParsers  []task.RunnerType
	DisabledParsers []task.RunnerType
	FilePatterns    []string
	ExcludePatterns []string
}

// DiscoveryCache manages caching of discovered tasks
type DiscoveryCache struct {
	entries map[string]*DiscoveryCacheEntry
	maxSize int
	ttl     time.Duration
	mu      sync.RWMutex
}

// CacheEntry represents a cached discovery result
type DiscoveryCacheEntry struct {
	Tasks      []*task.Task
	FilePath   string
	ModTime    time.Time
	CachedAt   time.Time
	AccessedAt time.Time
	Hash       string
}

// DiscoveryStats tracks discovery statistics
type DiscoveryStats struct {
	// Counters
	TotalDiscoveries  int64
	CacheHits         int64
	CacheMisses       int64
	FilesProcessed    int64
	TasksDiscovered   int64
	ErrorsEncountered int64

	// Timing
	TotalDuration     time.Duration
	AverageDuration   time.Duration
	LastDiscoveryTime time.Time

	// Parser stats
	ParserStats map[task.RunnerType]*ParserStats

	// Performance
	ConcurrentJobs  int64
	PeakConcurrency int64
}

// ParserStats tracks statistics for individual parsers
type ParserStats struct {
	FilesProcessed  int64
	TasksDiscovered int64
	Errors          int64
	TotalDuration   time.Duration
	AverageDuration time.Duration
}

// Parser interface that all task parsers must implement
type Parser interface {
	// File handling
	CanParse(filePath string) bool
	GetSupportedFiles() []string

	// Parsing
	ParseFile(filePath string) ([]*task.Task, error)
	ParseContent(reader io.Reader, filePath string) ([]*task.Task, error)

	// Metadata
	// GetMetadata() map[string]any

	// Validation (optional)
	Validate(filePath string) error
}

// NewRegistry creates a new discovery registry
func NewRegistry(logger logger.Logger) *Registry {
	registry := &Registry{
		parsers:     make(map[task.RunnerType]Parser),
		parserOrder: []task.RunnerType{},
		options:     createDefaultRegistryOptions(),
		cache:       newDiscoveryCache(1000, 5*time.Minute),
		stats:       newDiscoveryStats(),
		logger:      logger.WithGroup("discovery-registry"),
	}

	// Register default parsers
	registry.registerDefaultParsers()

	return registry
}

// createDefaultRegistryOptions creates default registry options
func createDefaultRegistryOptions() *RegistryOptions {
	return &RegistryOptions{
		MaxDepth:        10,
		FollowSymlinks:  false,
		IgnoreHidden:    true,
		IgnorePatterns:  []string{".git", "node_modules", "__pycache__", ".venv", "venv"},
		EnableCache:     true,
		CacheTTL:        5 * time.Minute,
		MaxCacheSize:    1000,
		ConcurrentLimit: 10,
		EnabledParsers:  []task.RunnerType{},
		DisabledParsers: []task.RunnerType{},
		FilePatterns:    []string{},
		ExcludePatterns: []string{"*.tmp", "*.bak", "*~"},
	}
}

// registerDefaultParsers registers all built-in parsers
func (r *Registry) registerDefaultParsers() {
	// Register Make parser
	makeParser, err := parsers.NewMakefileParser(r.logger)
	if err != nil {
		r.logger.Warn("Failed to register Make parser", "error", err)
		return
	}
	r.RegisterParser(task.RunnerMake, makeParser)

	// Register NPM parser
	npmParser, err := parsers.NewPackageJSONParser(r.logger)
	if err != nil {
		r.logger.Warn("Failed to register NPM parser", "error", err)
		return
	}
	r.RegisterParser(task.RunnerNpm, npmParser)

	// Register Yarn parser (uses same parser as NPM)
	yarnParser, err := parsers.NewPackageJSONParser(r.logger)
	if err != nil {
		r.logger.Warn("Failed to register Yarn parser", "error", err)
		return
	}
	r.RegisterParser(task.RunnerYarn, yarnParser)

	// Register pnpm parser (uses same parser as NPM)
	pnpmParser, err := parsers.NewPackageJSONParser(r.logger)
	if err != nil {
		r.logger.Warn("Failed to register pnpm parser", "error", err)
		return
	}
	r.RegisterParser(task.RunnerPnpm, pnpmParser)

	// Register Bun parser (uses same parser as NPM)
	bunParser, err := parsers.NewPackageJSONParser(r.logger)
	if err != nil {
		r.logger.Warn("Failed to register Bun parser", "error", err)
		return
	}
	r.RegisterParser(task.RunnerBun, bunParser)

	// Register Just parser
	r.RegisterParser(task.RunnerJust, parsers.NewJustfileParser(r.logger))

	// Register Task parser
	r.RegisterParser(task.RunnerTask, parsers.NewTaskfileParser(r.logger))

	// Register Python parser
	r.RegisterParser(task.RunnerPython, parsers.NewPythonParser(r.logger))

	r.logger.Info("Registered default parsers", "count", len(r.parsers))
}

// RegisterParser registers a parser for a specific runner type
func (r *Registry) RegisterParser(runnerType task.RunnerType, parser Parser) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.parsers[runnerType] = parser

	// Add to order if not already present
	if slices.Contains(r.parserOrder, runnerType) {
		return
	}
	r.parserOrder = append(r.parserOrder, runnerType)

	r.logger.Debug("Registered parser", "runner", runnerType)
}

// UnregisterParser removes a parser
func (r *Registry) UnregisterParser(runnerType task.RunnerType) {
	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.parsers, runnerType)

	// Remove from order
	for i, existing := range r.parserOrder {
		if existing == runnerType {
			r.parserOrder = slices.Delete(r.parserOrder, i, i+1)
			break
		}
	}

	r.logger.Debug("Unregistered parser", "runner", runnerType)
}

// GetParser returns a parser for the specified runner type
func (r *Registry) GetParser(runnerType task.RunnerType) (Parser, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	parser, exists := r.parsers[runnerType]
	return parser, exists
}

// GetRegisteredParsers returns all registered parser types
func (r *Registry) GetRegisteredParsers() []task.RunnerType {
	r.mu.RLock()
	defer r.mu.RUnlock()

	parsers := make([]task.RunnerType, len(r.parserOrder))
	copy(parsers, r.parserOrder)
	return parsers
}

// SetOptions sets registry options
func (r *Registry) SetOptions(options *RegistryOptions) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.options = options

	// Update cache settings
	if r.cache != nil {
		r.cache.maxSize = options.MaxCacheSize
		r.cache.ttl = options.CacheTTL
	}
}

// GetOptions returns current registry options
func (r *Registry) GetOptions() *RegistryOptions {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy to prevent modifications
	options := *r.options
	return &options
}

// DiscoverTasks discovers tasks in the given directory
func (r *Registry) DiscoverTasks(ctx context.Context, rootPath string) ([]*task.Task, error) {
	startTime := time.Now()

	r.mu.Lock()
	r.stats.TotalDiscoveries++
	r.lastDiscovery = startTime
	r.mu.Unlock()

	r.logger.Info("Starting task discovery", "path", rootPath)

	// Find all relevant files
	files, err := r.findFiles(ctx, rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to find files: %w", err)
	}

	// Discover tasks from files
	tasks, err := r.discoverFromFiles(ctx, files)
	if err != nil {
		return nil, fmt.Errorf("failed to discover tasks: %w", err)
	}

	// Update statistics
	duration := time.Since(startTime)
	r.updateStats(len(files), len(tasks), duration)

	r.logger.Info("Discovery completed",
		"files", len(files),
		"tasks", len(tasks),
		"duration", duration)

	return tasks, nil
}

// DiscoverTasksFromFile discovers tasks from a specific file
func (r *Registry) DiscoverTasksFromFile(ctx context.Context, filePath string) ([]*task.Task, error) {
	r.logger.Debug("Discovering tasks from file", "file", filePath)

	// Check cache first
	if r.options.EnableCache {
		if cached := r.cache.get(filePath); cached != nil {
			r.stats.CacheHits++
			return cached.Tasks, nil
		}
		r.stats.CacheMisses++
	}

	// Find appropriate parser
	parser := r.findParserForFile(filePath)
	if parser == nil {
		return nil, fmt.Errorf("no parser found for file: %s", filePath)
	}

	// Parse file
	startTime := time.Now()
	tasks, err := parser.ParseFile(filePath)
	duration := time.Since(startTime)

	if err != nil {
		r.updateParserStats(r.getRunnerTypeForParser(parser), 0, 1, duration)
		return nil, fmt.Errorf("failed to parse file %s: %w", filePath, err)
	}

	// Update parser statistics
	r.updateParserStats(r.getRunnerTypeForParser(parser), len(tasks), 0, duration)

	// Cache results
	if r.options.EnableCache && len(tasks) > 0 {
		r.cache.put(filePath, tasks)
	}

	return tasks, nil
}

// findFiles finds all files that should be processed for task discovery
func (r *Registry) findFiles(ctx context.Context, rootPath string) ([]string, error) {
	var files []string
	var mu sync.Mutex

	// Get supported file patterns
	patterns := r.getSupportedFilePatterns()

	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err != nil {
			r.logger.Warn("Error walking path", "path", path, "error", err)
			return nil // Continue walking
		}

		// Skip directories
		if info.IsDir() {
			// Check if we should skip this directory
			if r.shouldSkipDirectory(path, info) {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if file should be processed
		if r.shouldProcessFile(path, info, patterns) {
			mu.Lock()
			files = append(files, path)
			mu.Unlock()
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return files, nil
}

// discoverFromFiles discovers tasks from a list of files
func (r *Registry) discoverFromFiles(ctx context.Context, files []string) ([]*task.Task, error) {
	var allTasks []*task.Task
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Limit concurrent processing
	semaphore := make(chan struct{}, r.options.ConcurrentLimit)

	for _, file := range files {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		wg.Add(1)
		go func(filePath string) {
			defer wg.Done()

			semaphore <- struct{}{}        // Acquire
			defer func() { <-semaphore }() // Release

			tasks, err := r.DiscoverTasksFromFile(ctx, filePath)
			if err != nil {
				r.logger.Warn("Failed to discover tasks from file", "file", filePath, "error", err)
				return
			}

			if len(tasks) > 0 {
				mu.Lock()
				allTasks = append(allTasks, tasks...)
				mu.Unlock()
			}
		}(file)
	}

	wg.Wait()

	// Sort tasks by file path and line number for consistent ordering
	sort.Slice(allTasks, func(i, j int) bool {
		if allTasks[i].FilePath != allTasks[j].FilePath {
			return allTasks[i].FilePath < allTasks[j].FilePath
		}
		return allTasks[i].Command < allTasks[j].Command
	})

	return allTasks, nil
}

// findParserForFile finds the appropriate parser for a file
func (r *Registry) findParserForFile(filePath string) Parser {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, runnerType := range r.parserOrder {
		// Skip disabled parsers
		if r.isParserDisabled(runnerType) {
			continue
		}

		parser := r.parsers[runnerType]
		if parser.CanParse(filePath) {
			return parser
		}
	}

	return nil
}

// getSupportedFilePatterns returns all supported file patterns
func (r *Registry) getSupportedFilePatterns() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var patterns []string

	for _, runnerType := range r.parserOrder {
		if r.isParserDisabled(runnerType) {
			continue
		}

		parser := r.parsers[runnerType]
		patterns = append(patterns, parser.GetSupportedFiles()...)
	}

	return patterns
}

// shouldSkipDirectory checks if a directory should be skipped
func (r *Registry) shouldSkipDirectory(_ string, info os.FileInfo) bool {
	name := info.Name()

	// Skip hidden directories if configured
	if r.options.IgnoreHidden && strings.HasPrefix(name, ".") {
		return true
	}

	// Check ignore patterns
	for _, pattern := range r.options.IgnorePatterns {
		if matched, _ := filepath.Match(pattern, name); matched {
			return true
		}
	}

	return false
}

// shouldProcessFile checks if a file should be processed
func (r *Registry) shouldProcessFile(_ string, info os.FileInfo, patterns []string) bool {
	name := info.Name()

	// Skip hidden files if configured
	if r.options.IgnoreHidden && strings.HasPrefix(name, ".") {
		return false
	}

	// Check exclude patterns
	for _, pattern := range r.options.ExcludePatterns {
		if matched, _ := filepath.Match(pattern, name); matched {
			return false
		}
	}

	// Check if any parser can handle this file
	for _, pattern := range patterns {
		if matched, _ := filepath.Match(pattern, name); matched {
			return true
		}
	}

	return false
}

// isParserDisabled checks if a parser is disabled
func (r *Registry) isParserDisabled(runnerType task.RunnerType) bool {
	// Check if explicitly disabled
	if slices.Contains(r.options.DisabledParsers, runnerType) {
		return true
	}

	// If enabled parsers list is specified, check if this parser is in it
	if len(r.options.EnabledParsers) > 0 {
		return !slices.Contains(r.options.EnabledParsers, runnerType) // Not in enabled list
	}

	return false
}

// getRunnerTypeForParser returns the runner type for a parser
func (r *Registry) getRunnerTypeForParser(parser Parser) task.RunnerType {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for runnerType, p := range r.parsers {
		if p == parser {
			return runnerType
		}
	}

	return task.RunnerType("unknown")
}

// updateStats updates discovery statistics
func (r *Registry) updateStats(filesProcessed, tasksDiscovered int, duration time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.stats.FilesProcessed += int64(filesProcessed)
	r.stats.TasksDiscovered += int64(tasksDiscovered)
	r.stats.TotalDuration += duration
	r.stats.AverageDuration = r.stats.TotalDuration / time.Duration(r.stats.TotalDiscoveries)
	r.stats.LastDiscoveryTime = time.Now()
}

// updateParserStats updates statistics for a specific parser
func (r *Registry) updateParserStats(runnerType task.RunnerType, tasksDiscovered int, errors int, duration time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.stats.ParserStats == nil {
		r.stats.ParserStats = make(map[task.RunnerType]*ParserStats)
	}

	stats, exists := r.stats.ParserStats[runnerType]
	if !exists {
		stats = &ParserStats{}
		r.stats.ParserStats[runnerType] = stats
	}

	stats.FilesProcessed++
	stats.TasksDiscovered += int64(tasksDiscovered)
	stats.Errors += int64(errors)
	stats.TotalDuration += duration
	stats.AverageDuration = stats.TotalDuration / time.Duration(stats.FilesProcessed)
}

// GetStats returns current discovery statistics
func (r *Registry) GetStats() *DiscoveryStats {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// Return a copy to prevent modifications
	stats := *r.stats
	if r.stats.ParserStats != nil {
		stats.ParserStats = make(map[task.RunnerType]*ParserStats)
		for k, v := range r.stats.ParserStats {
			statsCopy := *v
			stats.ParserStats[k] = &statsCopy
		}
	}

	return &stats
}

// ClearCache clears the discovery cache
func (r *Registry) ClearCache() {
	if r.cache != nil {
		r.cache.clear()
	}
}

// GetCacheStats returns cache statistics
func (r *Registry) GetCacheStats() map[string]any {
	if r.cache != nil {
		return r.cache.getStats()
	}
	return map[string]any{}
}

// Discovery cache implementation

// newDiscoveryCache creates a new discovery cache
func newDiscoveryCache(maxSize int, ttl time.Duration) *DiscoveryCache {
	return &DiscoveryCache{
		entries: make(map[string]*DiscoveryCacheEntry),
		maxSize: maxSize,
		ttl:     ttl,
	}
}

// get retrieves an entry from the cache
func (dc *DiscoveryCache) get(filePath string) *DiscoveryCacheEntry {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	entry, exists := dc.entries[filePath]
	if !exists {
		return nil
	}

	// Check if entry is expired
	if time.Since(entry.CachedAt) > dc.ttl {
		dc.mu.RUnlock()
		dc.mu.Lock()
		delete(dc.entries, filePath)
		dc.mu.Unlock()
		dc.mu.RLock()
		return nil
	}

	// Update access time
	entry.AccessedAt = time.Now()
	return entry
}

// put stores an entry in the cache
func (dc *DiscoveryCache) put(filePath string, tasks []*task.Task) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	// Check if cache is full
	if len(dc.entries) >= dc.maxSize {
		dc.evictOldest()
	}

	// Get file modification time
	var modTime time.Time
	if info, err := os.Stat(filePath); err == nil {
		modTime = info.ModTime()
	}

	entry := &DiscoveryCacheEntry{
		Tasks:      tasks,
		FilePath:   filePath,
		ModTime:    modTime,
		CachedAt:   time.Now(),
		AccessedAt: time.Now(),
		Hash:       calculateFileHash(filePath),
	}

	dc.entries[filePath] = entry
}

// evictOldest removes the oldest entry from the cache
func (dc *DiscoveryCache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range dc.entries {
		if oldestKey == "" || entry.AccessedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.AccessedAt
		}
	}

	if oldestKey != "" {
		delete(dc.entries, oldestKey)
	}
}

// clear clears all cache entries
func (dc *DiscoveryCache) clear() {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	dc.entries = make(map[string]*DiscoveryCacheEntry)
}

// getStats returns cache statistics
func (dc *DiscoveryCache) getStats() map[string]any {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	return map[string]any{
		"entries":  len(dc.entries),
		"max_size": dc.maxSize,
		"ttl":      dc.ttl,
	}
}

// newDiscoveryStats creates new discovery statistics
func newDiscoveryStats() *DiscoveryStats {
	return &DiscoveryStats{
		ParserStats: make(map[task.RunnerType]*ParserStats),
	}
}

// calculateFileHash calculates a hash for cache validation
func calculateFileHash(filePath string) string {
	// Simple implementation - could be enhanced with actual file content hashing
	if info, err := os.Stat(filePath); err == nil {
		return fmt.Sprintf("%d_%d", info.Size(), info.ModTime().Unix())
	}
	return ""
}
