package discovery

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vivalchemy/wake/pkg/logger"
	"github.com/vivalchemy/wake/pkg/task"
)

// Cache provides caching functionality for task discovery results
type Cache struct {
	// Configuration
	cacheDir string
	ttl      time.Duration
	logger   logger.Logger

	// In-memory cache
	memCache map[string]*CacheEntry
	mu       sync.RWMutex

	// File system cache
	fsCache *FileSystemCache

	// Statistics
	stats *CacheStats
}

// CacheEntry represents a single cache entry
type CacheEntry struct {
	// Cache key
	Key string `json:"key"`

	// Cached tasks
	Tasks []*task.Task `json:"tasks"`

	// When the entry was created
	CreatedAt time.Time `json:"created_at"`

	// When the entry expires
	ExpiresAt time.Time `json:"expires_at"`

	// File checksums for invalidation
	FileChecksums map[string]string `json:"file_checksums"`

	// Cache metadata
	Metadata map[string]any `json:"metadata"`
}

// CacheStats tracks cache performance metrics
type CacheStats struct {
	// Hit/miss counters
	Hits   int64 `json:"hits"`
	Misses int64 `json:"misses"`

	// Entry counts
	TotalEntries   int64 `json:"total_entries"`
	ExpiredEntries int64 `json:"expired_entries"`
	InvalidEntries int64 `json:"invalid_entries"`

	// Size metrics
	MemorySizeBytes int64 `json:"memory_size_bytes"`
	DiskSizeBytes   int64 `json:"disk_size_bytes"`

	// Timing metrics
	LastCleanup time.Time `json:"last_cleanup"`

	// Mutex for thread safety
	mu sync.RWMutex
}

// FileSystemCache handles persistent caching to disk
type FileSystemCache struct {
	baseDir string
	logger  logger.Logger
	mu      sync.RWMutex
}

// NewCache creates a new cache instance
func NewCache(cacheDir string, ttl time.Duration, log logger.Logger) (*Cache, error) {
	if cacheDir == "" {
		return nil, fmt.Errorf("cache directory cannot be empty")
	}

	// Create cache directory if it doesn't exist
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Initialize file system cache
	fsCache, err := NewFileSystemCache(cacheDir, log)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize filesystem cache: %w", err)
	}

	cache := &Cache{
		cacheDir: cacheDir,
		ttl:      ttl,
		logger:   log.WithGroup("cache"),
		memCache: make(map[string]*CacheEntry),
		fsCache:  fsCache,
		stats:    &CacheStats{},
	}

	// Load existing cache entries from disk
	if err := cache.loadFromDisk(); err != nil {
		cache.logger.Warn("Failed to load cache from disk", "error", err)
	}

	// Start background cleanup routine
	go cache.cleanupRoutine()

	return cache, nil
}

// NewFileSystemCache creates a new filesystem cache
func NewFileSystemCache(baseDir string, log logger.Logger) (*FileSystemCache, error) {
	return &FileSystemCache{
		baseDir: baseDir,
		logger:  log.WithGroup("fs-cache"),
	}, nil
}

// Get retrieves tasks from cache if they exist and are valid
func (c *Cache) Get(path string) ([]*task.Task, bool) {
	key := c.generateKey(path)

	c.mu.RLock()
	entry, exists := c.memCache[key]
	c.mu.RUnlock()

	if !exists {
		// Try loading from disk
		if diskEntry, loaded := c.fsCache.Load(key); loaded {
			c.mu.Lock()
			c.memCache[key] = diskEntry
			entry = diskEntry
			exists = true
			c.mu.Unlock()
		}
	}

	if !exists {
		c.recordMiss()
		return nil, false
	}

	// Check if entry is expired
	if time.Now().After(entry.ExpiresAt) {
		c.logger.Debug("Cache entry expired", "key", key, "expired_at", entry.ExpiresAt)
		c.invalidateKey(key)
		c.recordMiss()
		c.stats.mu.Lock()
		c.stats.ExpiredEntries++
		c.stats.mu.Unlock()
		return nil, false
	}

	// Check if files have been modified
	if c.areFilesModified(path, entry.FileChecksums) {
		c.logger.Debug("Cache entry invalidated due to file changes", "key", key)
		c.invalidateKey(key)
		c.recordMiss()
		c.stats.mu.Lock()
		c.stats.InvalidEntries++
		c.stats.mu.Unlock()
		return nil, false
	}

	c.recordHit()
	c.logger.Debug("Cache hit", "key", key, "tasks", len(entry.Tasks))

	// Return a deep copy to prevent external modification
	return c.cloneTasks(entry.Tasks), true
}

// Set stores tasks in the cache
func (c *Cache) Set(path string, tasks []*task.Task) {
	key := c.generateKey(path)

	// Calculate file checksums for invalidation
	checksums := c.calculateFileChecksums(path, tasks)

	entry := &CacheEntry{
		Key:           key,
		Tasks:         c.cloneTasks(tasks),
		CreatedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(c.ttl),
		FileChecksums: checksums,
		Metadata: map[string]any{
			"path":       path,
			"task_count": len(tasks),
		},
	}

	// Store in memory cache
	c.mu.Lock()
	c.memCache[key] = entry
	c.stats.TotalEntries++
	c.mu.Unlock()

	// Store in filesystem cache
	c.fsCache.Store(key, entry)

	c.logger.Debug("Cache entry stored", "key", key, "tasks", len(tasks), "expires_at", entry.ExpiresAt)
}

// Clear removes all cache entries
func (c *Cache) Clear() {
	c.mu.Lock()
	c.memCache = make(map[string]*CacheEntry)
	c.stats.TotalEntries = 0
	c.mu.Unlock()

	c.fsCache.Clear()

	c.logger.Debug("Cache cleared")
}

// Invalidate removes a specific cache entry
func (c *Cache) Invalidate(path string) {
	key := c.generateKey(path)
	c.invalidateKey(key)
}

// invalidateKey removes a cache entry by key
func (c *Cache) invalidateKey(key string) {
	c.mu.Lock()
	delete(c.memCache, key)
	c.stats.TotalEntries--
	c.mu.Unlock()

	c.fsCache.Remove(key)

	c.logger.Debug("Cache entry invalidated", "key", key)
}

// GetStats returns cache statistics
func (c *Cache) GetStats() *CacheStats {
	c.stats.mu.RLock()
	defer c.stats.mu.RUnlock()

	// Calculate current memory usage
	c.mu.RLock()
	memorySize := c.calculateMemoryUsage()
	c.mu.RUnlock()

	// Create a copy of stats
	stats := *c.stats
	stats.MemorySizeBytes = memorySize
	stats.DiskSizeBytes = c.fsCache.GetSize()

	return &stats
}

// Close shuts down the cache and cleans up resources
func (c *Cache) Close() error {
	c.logger.Debug("Shutting down cache")

	// Final cleanup
	c.cleanup()

	return nil
}

// generateKey generates a cache key for a given path
func (c *Cache) generateKey(path string) string {
	// Normalize path
	normalizedPath := filepath.Clean(path)

	// Create hash of path for consistent key generation
	hash := md5.Sum([]byte(normalizedPath))
	return fmt.Sprintf("%x", hash)
}

// calculateFileChecksums calculates checksums for all relevant files
func (c *Cache) calculateFileChecksums(path string, tasks []*task.Task) map[string]string {
	checksums := make(map[string]string)

	// Get all unique file paths from tasks
	filePaths := make(map[string]bool)
	filePaths[path] = true // Include the scan path itself

	for _, t := range tasks {
		if t.FilePath != "" {
			filePaths[t.FilePath] = true
		}
	}

	// Calculate checksum for each file
	for filePath := range filePaths {
		if checksum, err := c.calculateFileChecksum(filePath); err == nil {
			checksums[filePath] = checksum
		} else {
			c.logger.Debug("Failed to calculate checksum", "file", filePath, "error", err)
		}
	}

	return checksums
}

// calculateFileChecksum calculates MD5 checksum of a file
func (c *Cache) calculateFileChecksum(filePath string) (string, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return "", err
	}

	hash := md5.Sum(data)
	return fmt.Sprintf("%x", hash), nil
}

// areFilesModified checks if any files have been modified since caching
func (c *Cache) areFilesModified(path string, checksums map[string]string) bool {
	currentChecksums := c.calculateFileChecksums(path, nil)

	// Check if any files are missing or modified
	for filePath, oldChecksum := range checksums {
		currentChecksum, exists := currentChecksums[filePath]
		if !exists || currentChecksum != oldChecksum {
			return true
		}
	}

	return false
}

// cloneTasks creates deep copies of tasks to prevent external modification
func (c *Cache) cloneTasks(tasks []*task.Task) []*task.Task {
	cloned := make([]*task.Task, len(tasks))
	for i, t := range tasks {
		cloned[i] = t.Clone()
	}
	return cloned
}

// calculateMemoryUsage estimates memory usage of in-memory cache
func (c *Cache) calculateMemoryUsage() int64 {
	var size int64

	for _, entry := range c.memCache {
		// Rough estimation of memory usage
		size += int64(len(entry.Key))
		size += int64(len(entry.Tasks)) * 500         // Rough estimate per task
		size += int64(len(entry.FileChecksums)) * 100 // Rough estimate per checksum
	}

	return size
}

// recordHit increments hit counter
func (c *Cache) recordHit() {
	c.stats.mu.Lock()
	c.stats.Hits++
	c.stats.mu.Unlock()
}

// recordMiss increments miss counter
func (c *Cache) recordMiss() {
	c.stats.mu.Lock()
	c.stats.Misses++
	c.stats.mu.Unlock()
}

// loadFromDisk loads existing cache entries from disk
func (c *Cache) loadFromDisk() error {
	entries, err := c.fsCache.LoadAll()
	if err != nil {
		return fmt.Errorf("failed to load cache entries: %w", err)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	loaded := 0
	for key, entry := range entries {
		// Check if entry is still valid
		if time.Now().Before(entry.ExpiresAt) {
			c.memCache[key] = entry
			loaded++
		}
	}

	c.stats.TotalEntries = int64(loaded)
	c.logger.Debug("Loaded cache entries from disk", "loaded", loaded, "total", len(entries))

	return nil
}

// cleanupRoutine runs periodic cleanup
func (c *Cache) cleanupRoutine() {
	ticker := time.NewTicker(time.Hour) // Cleanup every hour
	defer ticker.Stop()

	for range ticker.C {
		c.cleanup()
	}
}

// cleanup removes expired entries and performs maintenance
func (c *Cache) cleanup() {
	c.logger.Debug("Starting cache cleanup")

	now := time.Now()
	removed := 0

	c.mu.Lock()
	for key, entry := range c.memCache {
		if now.After(entry.ExpiresAt) {
			delete(c.memCache, key)
			removed++
		}
	}
	c.stats.TotalEntries = int64(len(c.memCache))
	c.mu.Unlock()

	// Cleanup filesystem cache
	fsRemoved := c.fsCache.Cleanup()

	c.stats.mu.Lock()
	c.stats.ExpiredEntries += int64(removed + fsRemoved)
	c.stats.LastCleanup = now
	c.stats.mu.Unlock()

	c.logger.Debug("Cache cleanup completed",
		"memory_removed", removed,
		"disk_removed", fsRemoved,
		"total_entries", c.stats.TotalEntries)
}

// FileSystemCache methods

// Store saves a cache entry to disk
func (fc *FileSystemCache) Store(key string, entry *CacheEntry) error {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	filePath := fc.getFilePath(key)

	// Create directory if it doesn't exist
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Marshal entry to JSON
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal cache entry: %w", err)
	}

	// Write to file
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	return nil
}

// Load retrieves a cache entry from disk
func (fc *FileSystemCache) Load(key string) (*CacheEntry, bool) {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	filePath := fc.getFilePath(key)

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, false
	}

	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		fc.logger.Debug("Failed to unmarshal cache entry", "key", key, "error", err)
		return nil, false
	}

	return &entry, true
}

// LoadAll loads all cache entries from disk
func (fc *FileSystemCache) LoadAll() (map[string]*CacheEntry, error) {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	entries := make(map[string]*CacheEntry)

	err := filepath.Walk(fc.baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}

		// Extract key from filename
		key := fc.getKeyFromPath(path)

		data, err := os.ReadFile(path)
		if err != nil {
			fc.logger.Debug("Failed to read cache file", "path", path, "error", err)
			return nil // Continue walking
		}

		var entry CacheEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			fc.logger.Debug("Failed to unmarshal cache entry", "path", path, "error", err)
			return nil // Continue walking
		}

		entries[key] = &entry
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("failed to walk cache directory: %w", err)
	}

	return entries, nil
}

// Remove deletes a cache entry from disk
func (fc *FileSystemCache) Remove(key string) error {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	filePath := fc.getFilePath(key)

	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove cache file: %w", err)
	}

	return nil
}

// Clear removes all cache entries from disk
func (fc *FileSystemCache) Clear() error {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	return os.RemoveAll(fc.baseDir)
}

// Cleanup removes expired cache files
func (fc *FileSystemCache) Cleanup() int {
	fc.mu.Lock()
	defer fc.mu.Unlock()

	removed := 0
	now := time.Now()

	filepath.Walk(fc.baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".json") {
			return nil
		}

		// Read entry to check expiration
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}

		var entry CacheEntry
		if err := json.Unmarshal(data, &entry); err != nil {
			// Remove malformed files
			os.Remove(path)
			removed++
			return nil
		}

		// Remove expired entries
		if now.After(entry.ExpiresAt) {
			os.Remove(path)
			removed++
		}

		return nil
	})

	return removed
}

// GetSize calculates total disk usage of cache
func (fc *FileSystemCache) GetSize() int64 {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	var totalSize int64

	filepath.Walk(fc.baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		totalSize += info.Size()
		return nil
	})

	return totalSize
}

// getFilePath generates the file path for a cache key
func (fc *FileSystemCache) getFilePath(key string) string {
	// Use first 2 characters for subdirectory to avoid too many files in one directory
	subdir := key[:2]
	return filepath.Join(fc.baseDir, subdir, key+".json")
}

// getKeyFromPath extracts the cache key from a file path
func (fc *FileSystemCache) getKeyFromPath(filePath string) string {
	filename := filepath.Base(filePath)
	return strings.TrimSuffix(filename, ".json")
}
