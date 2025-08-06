package discovery

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vivalchemy/wake/internal/config"
	"github.com/vivalchemy/wake/internal/discovery/parsers"
	"github.com/vivalchemy/wake/internal/utils"
	"github.com/vivalchemy/wake/pkg/logger"
	"github.com/vivalchemy/wake/pkg/task"
)

// Scanner handles discovery of task files across the project hierarchy
type Scanner struct {
	// Configuration
	config *config.DiscoveryConfig
	logger logger.Logger

	// Parsers for different file types
	parsers map[string]parsers.Parser

	// Cache for discovered tasks
	cache *Cache

	// File system watcher (if enabled)
	watcher *utils.FileWatcher
	// Mutex for thread safety
	mu sync.RWMutex

	// Current scanning state
	isScanning bool
	lastScan   time.Time
}

// ScanResult represents the result of a discovery scan
type ScanResult struct {
	// Discovered tasks
	Tasks []*task.Task `json:"tasks"`

	// Files that were scanned
	ScannedFiles []string `json:"scanned_files"`

	// Files that had errors
	ErrorFiles map[string]error `json:"error_files"`

	// Total scan duration
	Duration time.Duration `json:"duration"`

	// When the scan was performed
	Timestamp time.Time `json:"timestamp"`

	// Whether cache was used
	CacheUsed bool `json:"cache_used"`
}

// FileInfo represents information about a discovered task file
type FileInfo struct {
	// File path
	Path string `json:"path"`

	// File type/runner
	Runner task.RunnerType `json:"runner"`

	// Last modified time
	ModTime time.Time `json:"mod_time"`

	// File size
	Size int64 `json:"size"`

	// Whether file exists
	Exists bool `json:"exists"`
}

// NewScanner creates a new task file scanner
func NewScanner(cfg *config.DiscoveryConfig, log logger.Logger) (*Scanner, error) {
	if cfg == nil {
		return nil, fmt.Errorf("discovery config cannot be nil")
	}
	if log == nil {
		return nil, fmt.Errorf("logger cannot be nil")
	}

	scanner := &Scanner{
		config:  cfg,
		logger:  log.WithGroup("discovery"),
		parsers: make(map[string]parsers.Parser),
	}

	// Initialize parsers
	if err := scanner.initializeParsers(); err != nil {
		return nil, fmt.Errorf("failed to initialize parsers: %w", err)
	}

	// Initialize cache if enabled
	if cfg.CacheEnabled {
		cache, err := NewCache(cfg.CacheDir, cfg.CacheTTL, log)
		if err != nil {
			return nil, fmt.Errorf("failed to initialize cache: %w", err)
		}
		scanner.cache = cache
	}

	// Initialize file watcher if enabled
	if cfg.WatchEnabled {
		scanner.watcher = utils.NewFileWatcher()
	}

	return scanner, nil
}

// initializeParsers sets up parsers for different task file types
func (s *Scanner) initializeParsers() error {
	// Makefile parser
	makeParser, err := parsers.NewMakefileParser(s.logger)
	if err != nil {
		return fmt.Errorf("failed to create makefile parser: %w", err)
	}
	s.parsers["Makefile"] = makeParser
	s.parsers["makefile"] = makeParser
	s.parsers["GNUmakefile"] = makeParser

	// Package.json parser
	packageParser, err := parsers.NewPackageJSONParser(s.logger)
	if err != nil {
		return fmt.Errorf("failed to create package.json parser: %w", err)
	}
	s.parsers["package.json"] = packageParser

	// Justfile parser
	justParser := parsers.NewJustfileParser(s.logger)
	s.parsers["Justfile"] = justParser
	s.parsers["justfile"] = justParser
	s.parsers[".justfile"] = justParser

	// Taskfile.yml parser
	taskParser := parsers.NewTaskfileParser(s.logger)
	s.parsers["Taskfile.yml"] = taskParser
	s.parsers["Taskfile.yaml"] = taskParser
	s.parsers["taskfile.yml"] = taskParser
	s.parsers["taskfile.yaml"] = taskParser

	// Python tasks parser
	pythonParser := parsers.NewPythonParser(s.logger)
	s.parsers["tasks.py"] = pythonParser

	return nil
}

// Discover performs task discovery starting from the current directory
func (s *Scanner) Discover(ctx context.Context) ([]*task.Task, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current directory: %w", err)
	}

	return s.DiscoverInPath(ctx, currentDir)
}

// DiscoverInPath performs task discovery starting from the specified path
func (s *Scanner) DiscoverInPath(ctx context.Context, rootPath string) ([]*task.Task, error) {
	s.mu.Lock()
	if s.isScanning {
		s.mu.Unlock()
		return nil, fmt.Errorf("scan already in progress")
	}
	s.isScanning = true
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		s.isScanning = false
		s.lastScan = time.Now()
		s.mu.Unlock()
	}()

	startTime := time.Now()
	s.logger.Info("Starting task discovery", "path", rootPath)

	// Check cache first
	if s.cache != nil {
		if cachedTasks, found := s.cache.Get(rootPath); found {
			s.logger.Debug("Using cached discovery results", "task_count", len(cachedTasks))
			return cachedTasks, nil
		}
	}

	// Perform actual discovery
	result, err := s.performDiscovery(ctx, rootPath)
	if err != nil {
		return nil, err
	}

	// Cache results if cache is enabled
	if s.cache != nil && len(result.Tasks) > 0 {
		s.cache.Set(rootPath, result.Tasks)
	}

	// Set up file watching if enabled
	if s.watcher != nil {
		s.setupFileWatching(result.ScannedFiles)
	}

	duration := time.Since(startTime)
	s.logger.Info("Task discovery completed",
		"task_count", len(result.Tasks),
		"files_scanned", len(result.ScannedFiles),
		"duration", duration)

	return result.Tasks, nil
}

// performDiscovery does the actual file system scanning and parsing
func (s *Scanner) performDiscovery(ctx context.Context, rootPath string) (*ScanResult, error) {
	result := &ScanResult{
		Tasks:        make([]*task.Task, 0),
		ScannedFiles: make([]string, 0),
		ErrorFiles:   make(map[string]error),
		Timestamp:    time.Now(),
	}

	startTime := time.Now()
	defer func() {
		result.Duration = time.Since(startTime)
	}()

	// Find all task files
	taskFiles, err := s.findTaskFiles(ctx, rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to find task files: %w", err)
	}

	// Parse files concurrently if enabled
	if s.config.ConcurrentScanning && len(taskFiles) > 1 {
		return s.parseFilesConcurrently(ctx, taskFiles, result)
	}

	// Parse files sequentially
	return s.parseFilesSequentially(ctx, taskFiles, result)
}

// findTaskFiles recursively finds all supported task files
func (s *Scanner) findTaskFiles(ctx context.Context, rootPath string) ([]*FileInfo, error) {
	var taskFiles []*FileInfo
	visited := make(map[string]bool) // To avoid infinite loops with symlinks

	err := s.walkDirectory(ctx, rootPath, 0, visited, &taskFiles)
	if err != nil {
		return nil, err
	}

	return taskFiles, nil
}

// walkDirectory recursively walks the directory tree
func (s *Scanner) walkDirectory(ctx context.Context, dir string, depth int, visited map[string]bool, taskFiles *[]*FileInfo) error {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Check depth limit
	if depth > s.config.MaxDepth {
		return nil
	}

	// Resolve symlinks and check for loops
	realPath, err := filepath.EvalSymlinks(dir)
	if err != nil {
		realPath = dir // Use original path if symlink resolution fails
	}

	if visited[realPath] {
		return nil // Already visited, avoid infinite loop
	}
	visited[realPath] = true

	// Read directory entries
	entries, err := os.ReadDir(dir)
	if err != nil {
		s.logger.Debug("Failed to read directory", "dir", dir, "error", err)
		return nil // Skip directories we can't read
	}

	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())

		// Check if we should ignore this path
		if s.shouldIgnore(path, entry.Name()) {
			continue
		}

		if entry.IsDir() {
			// Recursively scan subdirectory
			if err := s.walkDirectory(ctx, path, depth+1, visited, taskFiles); err != nil {
				return err
			}
		} else {
			// Check if this is a task file
			if s.isTaskFile(entry.Name()) {
				info, err := entry.Info()
				if err != nil {
					s.logger.Debug("Failed to get file info", "path", path, "error", err)
					continue
				}

				fileInfo := &FileInfo{
					Path:    path,
					Runner:  s.getRunnerForFile(entry.Name()),
					ModTime: info.ModTime(),
					Size:    info.Size(),
					Exists:  true,
				}

				*taskFiles = append(*taskFiles, fileInfo)
			}
		}
	}

	return nil
}

// shouldIgnore checks if a path should be ignored during scanning
func (s *Scanner) shouldIgnore(path, name string) bool {
	// Check against ignore patterns
	for _, pattern := range s.config.ExcludePatterns {
		if matched, _ := filepath.Match(pattern, name); matched {
			return true
		}
		if matched, _ := filepath.Match(pattern, filepath.Base(path)); matched {
			return true
		}
	}

	// Ignore hidden files/directories by default
	if strings.HasPrefix(name, ".") && name != ".justfile" {
		return true
	}

	return false
}

// isTaskFile checks if a filename represents a supported task file
func (s *Scanner) isTaskFile(filename string) bool {
	_, exists := s.parsers[filename]
	return exists
}

// getRunnerForFile determines the runner type for a given filename
func (s *Scanner) getRunnerForFile(filename string) task.RunnerType {
	switch filename {
	case "Makefile", "makefile", "GNUmakefile":
		return task.RunnerMake
	case "package.json":
		return task.RunnerNpm // Will be refined by parser based on lock files
	case "Justfile", "justfile", ".justfile":
		return task.RunnerJust
	case "Taskfile.yml", "Taskfile.yaml", "taskfile.yml", "taskfile.yaml":
		return task.RunnerTask
	case "tasks.py":
		return task.RunnerPython
	default:
		return task.RunnerCustom
	}
}

// parseFilesSequentially parses task files one by one
func (s *Scanner) parseFilesSequentially(ctx context.Context, taskFiles []*FileInfo, result *ScanResult) (*ScanResult, error) {
	for _, fileInfo := range taskFiles {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		tasks, err := s.parseTaskFile(fileInfo)
		if err != nil {
			result.ErrorFiles[fileInfo.Path] = err
			s.logger.Debug("Failed to parse task file", "path", fileInfo.Path, "error", err)
			continue
		}

		result.Tasks = append(result.Tasks, tasks...)
		result.ScannedFiles = append(result.ScannedFiles, fileInfo.Path)
	}

	return result, nil
}

// parseFilesConcurrently parses task files concurrently
func (s *Scanner) parseFilesConcurrently(ctx context.Context, taskFiles []*FileInfo, result *ScanResult) (*ScanResult, error) {
	// Create worker pool
	maxWorkers := s.config.MaxConcurrency
	if maxWorkers <= 0 {
		maxWorkers = 4
	}

	fileChan := make(chan *FileInfo, len(taskFiles))
	resultChan := make(chan parseResult, len(taskFiles))

	// Start workers
	var wg sync.WaitGroup
	for range maxWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for fileInfo := range fileChan {
				select {
				case <-ctx.Done():
					return
				default:
				}

				tasks, err := s.parseTaskFile(fileInfo)
				resultChan <- parseResult{
					fileInfo: fileInfo,
					tasks:    tasks,
					err:      err,
				}
			}
		}()
	}

	// Send files to workers
	go func() {
		defer close(fileChan)
		for _, fileInfo := range taskFiles {
			select {
			case <-ctx.Done():
				return
			case fileChan <- fileInfo:
			}
		}
	}()

	// Collect results
	go func() {
		wg.Wait()
		close(resultChan)
	}()

	for parseRes := range resultChan {
		if parseRes.err != nil {
			result.ErrorFiles[parseRes.fileInfo.Path] = parseRes.err
			s.logger.Debug("Failed to parse task file", "path", parseRes.fileInfo.Path, "error", parseRes.err)
		} else {
			result.Tasks = append(result.Tasks, parseRes.tasks...)
			result.ScannedFiles = append(result.ScannedFiles, parseRes.fileInfo.Path)
		}
	}

	return result, nil
}

// parseResult represents the result of parsing a single file
type parseResult struct {
	fileInfo *FileInfo
	tasks    []*task.Task
	err      error
}

// parseTaskFile parses a single task file
func (s *Scanner) parseTaskFile(fileInfo *FileInfo) ([]*task.Task, error) {
	parser, exists := s.parsers[filepath.Base(fileInfo.Path)]
	if !exists {
		return nil, fmt.Errorf("no parser available for file: %s", fileInfo.Path)
	}

	s.logger.Debug("Parsing task file", "path", fileInfo.Path, "runner", fileInfo.Runner)

	tasks, err := parser.Parse(fileInfo.Path)
	if err != nil {
		return nil, fmt.Errorf("parser error: %w", err)
	}

	// Set file path and working directory for all tasks
	workingDir := filepath.Dir(fileInfo.Path)
	for _, t := range tasks {
		t.FilePath = fileInfo.Path
		t.WorkingDirectory = workingDir
		t.DiscoveredAt = time.Now()
	}

	return tasks, nil
}

// setupFileWatching sets up file system watching for the discovered files
func (s *Scanner) setupFileWatching(_ []string) {
	if s.watcher == nil {
		return
	}

	// FIXME:
	// for _, file := range files {
	// 	if err := s.watcher.Watch(file); err != nil {
	// 		s.logger.Debug("Failed to watch file", "file", file, "error", err)
	// 	}
	// }
}

// Refresh forces a refresh of the task discovery
func (s *Scanner) Refresh(ctx context.Context) ([]*task.Task, error) {
	// Clear cache if it exists
	if s.cache != nil {
		s.cache.Clear()
	}

	return s.Discover(ctx)
}

// IsScanning returns whether a scan is currently in progress
func (s *Scanner) IsScanning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isScanning
}

// LastScanTime returns the time of the last completed scan
func (s *Scanner) LastScanTime() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastScan
}

// Close shuts down the scanner and cleans up resources
func (s *Scanner) Close() error {
	var errors []error

	// Close file watcher
	if s.watcher != nil {
		if err := s.watcher.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close file watcher: %w", err))
		}
	}

	// Close cache
	if s.cache != nil {
		if err := s.cache.Close(); err != nil {
			errors = append(errors, fmt.Errorf("failed to close cache: %w", err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors during scanner close: %v", errors)
	}

	return nil
}

// GetSupportedFiles returns a list of supported task file patterns
func (s *Scanner) GetSupportedFiles() []string {
	var patterns []string
	for filename := range s.parsers {
		patterns = append(patterns, filename)
	}
	return patterns
}

// GetStats returns discovery statistics
func (s *Scanner) GetStats() map[string]any {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := make(map[string]any)
	stats["is_scanning"] = s.isScanning
	stats["last_scan"] = s.lastScan
	stats["supported_files"] = len(s.parsers)

	if s.cache != nil {
		stats["cache_enabled"] = true
		stats["cache_stats"] = s.cache.GetStats()
	} else {
		stats["cache_enabled"] = false
	}

	if s.watcher != nil {
		stats["watcher_enabled"] = true
	} else {
		stats["watcher_enabled"] = false
	}

	return stats
}
