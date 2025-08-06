package utils

import (
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// FileInfo represents enhanced file information
type FileInfo struct {
	Path        string      `json:"path"`
	Name        string      `json:"name"`
	Size        int64       `json:"size"`
	Mode        os.FileMode `json:"mode"`
	ModTime     time.Time   `json:"mod_time"`
	IsDir       bool        `json:"is_dir"`
	IsSymlink   bool        `json:"is_symlink"`
	Target      string      `json:"target,omitempty"`
	Hash        string      `json:"hash,omitempty"`
	Permissions string      `json:"permissions"`
}

// WalkOptions contains options for directory traversal
type WalkOptions struct {
	// Depth control
	MaxDepth       int
	FollowSymlinks bool

	// Filtering
	IncludePatterns []string
	ExcludePatterns []string
	IgnoreHidden    bool

	// File type filtering
	FilesOnly bool
	DirsOnly  bool

	// Content filtering
	MinSize        int64
	MaxSize        int64
	ModifiedAfter  *time.Time
	ModifiedBefore *time.Time

	// Performance
	Parallel    bool
	WorkerCount int
}

// FileWatcher monitors file system changes
type FileWatcher struct {
	paths     []string
	callbacks []func(string, string) // path, event
	running   bool
	stopCh    chan struct{}
	mu        sync.RWMutex
}

// FileCache caches file information for performance
type FileCache struct {
	entries map[string]*FileCacheEntry
	mu      sync.RWMutex
	ttl     time.Duration
	maxSize int
}

// FileCacheEntry represents a cached file entry
type FileCacheEntry struct {
	Info        *FileInfo
	CachedAt    time.Time
	AccessCount int64
}

// FileOperation represents a file operation result
type FileOperation struct {
	Source    string
	Target    string
	Operation string
	Success   bool
	Error     error
	Size      int64
	Duration  time.Duration
}

// DirectoryAnalysis provides analysis of directory contents
type DirectoryAnalysis struct {
	Path                string           `json:"path"`
	TotalFiles          int64            `json:"total_files"`
	TotalDirs           int64            `json:"total_dirs"`
	TotalSize           int64            `json:"total_size"`
	LargestFile         *FileInfo        `json:"largest_file,omitempty"`
	OldestFile          *FileInfo        `json:"oldest_file,omitempty"`
	NewestFile          *FileInfo        `json:"newest_file,omitempty"`
	FileTypes           map[string]int64 `json:"file_types"`
	SizeDistribution    map[string]int64 `json:"size_distribution"`
	ModTimeDistribution map[string]int64 `json:"mod_time_distribution"`
}

func NewFileWatcher() *FileWatcher {
	return &FileWatcher{
		paths:     make([]string, 0),
		callbacks: make([]func(string, string), 0),
		running:   false,
		stopCh:    make(chan struct{}),
		mu:        sync.RWMutex{},
	}
}

func (fw *FileWatcher) Close() error {
	fw.mu.Lock()
	defer fw.mu.Unlock()

	// Check if already stopped
	if !fw.running {
		return nil // Already closed, no error
	}

	// Signal to stop
	close(fw.stopCh)
	fw.running = false

	// Clean up resources
	fw.paths = nil
	fw.callbacks = nil

	return nil
}

// GetFileInfo returns enhanced file information
func GetFileInfo(path string) (*FileInfo, error) {
	stat, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file %s: %w", path, err)
	}

	info := &FileInfo{
		Path:        path,
		Name:        stat.Name(),
		Size:        stat.Size(),
		Mode:        stat.Mode(),
		ModTime:     stat.ModTime(),
		IsDir:       stat.IsDir(),
		IsSymlink:   stat.Mode()&os.ModeSymlink != 0,
		Permissions: stat.Mode().Perm().String(),
	}

	// Resolve symlink target
	if info.IsSymlink {
		target, err := os.Readlink(path)
		if err == nil {
			info.Target = target
		}
	}

	return info, nil
}

// GetFileHash calculates MD5 hash of a file
func GetFileHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open file %s: %w", path, err)
	}
	defer file.Close()

	hash := md5.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("failed to calculate hash for %s: %w", path, err)
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// WalkDirectory walks a directory tree with options
func WalkDirectory(root string, options *WalkOptions) ([]*FileInfo, error) {
	if options == nil {
		options = &WalkOptions{
			MaxDepth:    10,
			WorkerCount: 4,
		}
	}

	var files []*FileInfo
	var mu sync.Mutex

	walkFunc := func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue walking despite errors
		}

		// Calculate relative depth
		relPath, _ := filepath.Rel(root, path)
		depth := strings.Count(relPath, string(filepath.Separator))

		// Check max depth
		if options.MaxDepth > 0 && depth > options.MaxDepth {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if should skip hidden files/dirs
		if options.IgnoreHidden && strings.HasPrefix(info.Name(), ".") {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Apply file type filters
		if options.FilesOnly && info.IsDir() {
			return nil
		}
		if options.DirsOnly && !info.IsDir() {
			return nil
		}

		// Apply size filters
		if !info.IsDir() {
			if options.MinSize > 0 && info.Size() < options.MinSize {
				return nil
			}
			if options.MaxSize > 0 && info.Size() > options.MaxSize {
				return nil
			}
		}

		// Apply time filters
		if options.ModifiedAfter != nil && info.ModTime().Before(*options.ModifiedAfter) {
			return nil
		}
		if options.ModifiedBefore != nil && info.ModTime().After(*options.ModifiedBefore) {
			return nil
		}

		// Apply pattern filters
		if !matchesPatterns(path, options.IncludePatterns, options.ExcludePatterns) {
			if info.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		// Create file info
		fileInfo, err := GetFileInfo(path)
		if err != nil {
			return nil // Continue despite errors
		}

		mu.Lock()
		files = append(files, fileInfo)
		mu.Unlock()

		return nil
	}

	if err := filepath.Walk(root, walkFunc); err != nil {
		return nil, fmt.Errorf("failed to walk directory %s: %w", root, err)
	}

	return files, nil
}

// FindFiles finds files matching patterns
func FindFiles(root string, patterns []string) ([]string, error) {
	var matches []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue walking
		}

		if info.IsDir() {
			return nil
		}

		for _, pattern := range patterns {
			matched, err := filepath.Match(pattern, info.Name())
			if err != nil {
				continue
			}
			if matched {
				matches = append(matches, path)
				break
			}
		}

		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to find files: %w", err)
	}

	return matches, nil
}

// CopyFile copies a file from source to destination
func CopyFile(src, dst string) (*FileOperation, error) {
	start := time.Now()
	op := &FileOperation{
		Source:    src,
		Target:    dst,
		Operation: "copy",
	}

	// Open source file
	srcFile, err := os.Open(src)
	if err != nil {
		op.Error = fmt.Errorf("failed to open source file: %w", err)
		return op, op.Error
	}
	defer srcFile.Close()

	// Get source file info
	srcInfo, err := srcFile.Stat()
	if err != nil {
		op.Error = fmt.Errorf("failed to get source file info: %w", err)
		return op, op.Error
	}
	op.Size = srcInfo.Size()

	// Create destination directory if needed
	dstDir := filepath.Dir(dst)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		op.Error = fmt.Errorf("failed to create destination directory: %w", err)
		return op, op.Error
	}

	// Create destination file
	dstFile, err := os.Create(dst)
	if err != nil {
		op.Error = fmt.Errorf("failed to create destination file: %w", err)
		return op, op.Error
	}
	defer dstFile.Close()

	// Copy data
	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		op.Error = fmt.Errorf("failed to copy data: %w", err)
		return op, op.Error
	}

	// Copy file permissions and timestamps
	if err := os.Chmod(dst, srcInfo.Mode()); err != nil {
		// Non-fatal error
	}
	if err := os.Chtimes(dst, srcInfo.ModTime(), srcInfo.ModTime()); err != nil {
		// Non-fatal error
	}

	op.Success = true
	op.Duration = time.Since(start)
	return op, nil
}

// MoveFile moves a file from source to destination
func MoveFile(src, dst string) (*FileOperation, error) {
	start := time.Now()
	op := &FileOperation{
		Source:    src,
		Target:    dst,
		Operation: "move",
	}

	// Get source file info
	srcInfo, err := os.Stat(src)
	if err != nil {
		op.Error = fmt.Errorf("failed to get source file info: %w", err)
		return op, op.Error
	}
	op.Size = srcInfo.Size()

	// Create destination directory if needed
	dstDir := filepath.Dir(dst)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		op.Error = fmt.Errorf("failed to create destination directory: %w", err)
		return op, op.Error
	}

	// Try rename first (fastest for same filesystem)
	err = os.Rename(src, dst)
	if err == nil {
		op.Success = true
		op.Duration = time.Since(start)
		return op, nil
	}

	// If rename failed, try copy and delete
	_, err = CopyFile(src, dst)
	if err != nil {
		op.Error = fmt.Errorf("failed to copy file: %w", err)
		return op, op.Error
	}

	// Remove source file
	if err := os.Remove(src); err != nil {
		op.Error = fmt.Errorf("failed to remove source file: %w", err)
		// Try to clean up destination
		os.Remove(dst)
		return op, op.Error
	}

	op.Success = true
	op.Duration = time.Since(start)
	return op, nil
}

// DeleteFile deletes a file or directory
func DeleteFile(path string) (*FileOperation, error) {
	start := time.Now()
	op := &FileOperation{
		Source:    path,
		Operation: "delete",
	}

	// Get file info
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			op.Success = true // File doesn't exist, consider it success
			op.Duration = time.Since(start)
			return op, nil
		}
		op.Error = fmt.Errorf("failed to get file info: %w", err)
		return op, op.Error
	}
	op.Size = info.Size()

	// Remove file or directory
	var removeErr error
	if info.IsDir() {
		removeErr = os.RemoveAll(path)
	} else {
		removeErr = os.Remove(path)
	}

	if removeErr != nil {
		op.Error = fmt.Errorf("failed to remove: %w", removeErr)
		return op, op.Error
	}

	op.Success = true
	op.Duration = time.Since(start)
	return op, nil
}

// CreateDirectory creates a directory with all parent directories
func CreateDirectory(path string, mode os.FileMode) error {
	return os.MkdirAll(path, mode)
}

// IsDirectory checks if path is a directory
func IsDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// IsFile checks if path is a regular file
func IsFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// Exists checks if path exists
func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// IsReadable checks if file/directory is readable
func IsReadable(path string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	file.Close()
	return true
}

// IsWritable checks if file/directory is writable
func IsWritable(path string) bool {
	// For existing files, try to open with write access
	if Exists(path) {
		file, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err != nil {
			return false
		}
		file.Close()
		return true
	}

	// For non-existing files, check parent directory
	parent := filepath.Dir(path)
	return IsWritable(parent)
}

// GetDirSize calculates the total size of a directory
func GetDirSize(path string) (int64, error) {
	var size int64

	err := filepath.Walk(path, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue despite errors
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})

	return size, err
}

// AnalyzeDirectory provides comprehensive directory analysis
func AnalyzeDirectory(path string) (*DirectoryAnalysis, error) {
	analysis := &DirectoryAnalysis{
		Path:                path,
		FileTypes:           make(map[string]int64),
		SizeDistribution:    make(map[string]int64),
		ModTimeDistribution: make(map[string]int64),
	}

	var largestSize int64
	var oldestTime, newestTime time.Time

	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue despite errors
		}

		if info.IsDir() {
			analysis.TotalDirs++
		} else {
			analysis.TotalFiles++
			analysis.TotalSize += info.Size()

			// Track largest file
			if info.Size() > largestSize {
				largestSize = info.Size()
				if fileInfo, err := GetFileInfo(filePath); err == nil {
					analysis.LargestFile = fileInfo
				}
			}

			// Track oldest and newest files
			if oldestTime.IsZero() || info.ModTime().Before(oldestTime) {
				oldestTime = info.ModTime()
				if fileInfo, err := GetFileInfo(filePath); err == nil {
					analysis.OldestFile = fileInfo
				}
			}

			if newestTime.IsZero() || info.ModTime().After(newestTime) {
				newestTime = info.ModTime()
				if fileInfo, err := GetFileInfo(filePath); err == nil {
					analysis.NewestFile = fileInfo
				}
			}

			// Track file types
			ext := strings.ToLower(filepath.Ext(info.Name()))
			if ext == "" {
				ext = "(no extension)"
			}
			analysis.FileTypes[ext]++

			// Track size distribution
			sizeCategory := categorizeSizeBytes(info.Size())
			analysis.SizeDistribution[sizeCategory]++

			// Track modification time distribution
			timeCategory := categorizeModTime(info.ModTime())
			analysis.ModTimeDistribution[timeCategory]++
		}

		return nil
	})

	return analysis, err
}

// CleanDirectory removes old/temporary files from directory
func CleanDirectory(path string, options *CleanOptions) ([]*FileOperation, error) {
	if options == nil {
		options = &CleanOptions{}
	}

	var operations []*FileOperation

	err := filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() {
			return nil
		}

		shouldDelete := false

		// Check age
		if options.MaxAge > 0 && time.Since(info.ModTime()) > options.MaxAge {
			shouldDelete = true
		}

		// Check patterns
		for _, pattern := range options.DeletePatterns {
			if matched, _ := filepath.Match(pattern, info.Name()); matched {
				shouldDelete = true
				break
			}
		}

		// Check size
		if options.MaxSize > 0 && info.Size() > options.MaxSize {
			shouldDelete = true
		}

		if shouldDelete {
			op, err := DeleteFile(filePath)
			operations = append(operations, op)
			if err != nil && options.StopOnError {
				return err
			}
		}

		return nil
	})

	return operations, err
}

// CleanOptions contains options for directory cleaning
type CleanOptions struct {
	MaxAge         time.Duration
	MaxSize        int64
	DeletePatterns []string
	StopOnError    bool
}

// Utility functions

// matchesPatterns checks if path matches include/exclude patterns
func matchesPatterns(path string, includePatterns, excludePatterns []string) bool {
	name := filepath.Base(path)

	// Check exclude patterns first
	for _, pattern := range excludePatterns {
		if matched, _ := filepath.Match(pattern, name); matched {
			return false
		}
		// Also check full path for exclude patterns
		if matched, _ := filepath.Match(pattern, path); matched {
			return false
		}
	}

	// If no include patterns, include by default (unless excluded above)
	if len(includePatterns) == 0 {
		return true
	}

	// Check include patterns
	for _, pattern := range includePatterns {
		if matched, _ := filepath.Match(pattern, name); matched {
			return true
		}
		// Also check full path for include patterns
		if matched, _ := filepath.Match(pattern, path); matched {
			return true
		}
	}

	return false
}

// categorizeSizeBytes categorizes file size
func categorizeSizeBytes(size int64) string {
	switch {
	case size < 1024:
		return "0-1KB"
	case size < 1024*1024:
		return "1KB-1MB"
	case size < 1024*1024*10:
		return "1MB-10MB"
	case size < 1024*1024*100:
		return "10MB-100MB"
	default:
		return "100MB+"
	}
}

// categorizeModTime categorizes file modification time
func categorizeModTime(modTime time.Time) string {
	now := time.Now()
	diff := now.Sub(modTime)

	switch {
	case diff < 24*time.Hour:
		return "Today"
	case diff < 7*24*time.Hour:
		return "This week"
	case diff < 30*24*time.Hour:
		return "This month"
	case diff < 365*24*time.Hour:
		return "This year"
	default:
		return "Older"
	}
}

// NewFileCache creates a new file cache
func NewFileCache(ttl time.Duration, maxSize int) *FileCache {
	return &FileCache{
		entries: make(map[string]*FileCacheEntry),
		ttl:     ttl,
		maxSize: maxSize,
	}
}

// Get retrieves a file info from cache
func (fc *FileCache) Get(path string) (*FileInfo, bool) {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	entry, exists := fc.entries[path]
	if !exists {
		return nil, false
	}

	// Check if entry has expired
	if time.Since(entry.CachedAt) > fc.ttl {
		delete(fc.entries, path)
		return nil, false
	}

	entry.AccessCount++
	return entry.Info, true
}

// Put stores a file info in cache
func (fc *FileCache) Put(path string, info *FileInfo) {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	// Check if cache is full
	if len(fc.entries) >= fc.maxSize {
		fc.evictLRU()
	}

	fc.entries[path] = &FileCacheEntry{
		Info:        info,
		CachedAt:    time.Now(),
		AccessCount: 0,
	}
}

// evictLRU evicts least recently used entry
func (fc *FileCache) evictLRU() {
	var lruPath string
	var lruCount int64 = -1

	for path, entry := range fc.entries {
		if lruCount == -1 || entry.AccessCount < lruCount {
			lruPath = path
			lruCount = entry.AccessCount
		}
	}

	if lruPath != "" {
		delete(fc.entries, lruPath)
	}
}

// Clear clears the cache
func (fc *FileCache) Clear() {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	fc.entries = make(map[string]*FileCacheEntry)
}

// GetStats returns cache statistics
func (fc *FileCache) GetStats() map[string]any {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	var totalAccess int64
	for _, entry := range fc.entries {
		totalAccess += entry.AccessCount
	}

	return map[string]any{
		"entries":      len(fc.entries),
		"max_size":     fc.maxSize,
		"ttl_seconds":  int64(fc.ttl.Seconds()),
		"total_access": totalAccess,
	}
}
