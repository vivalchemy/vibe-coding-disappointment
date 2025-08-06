package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// PathType represents the type of a path
type PathType int

const (
	PathTypeUnknown PathType = iota
	PathTypeFile
	PathTypeDir
	PathTypeSymlink
	PathTypeBroken
)

// PathInfo contains detailed information about a path
type PathInfo struct {
	Original   string   `json:"original"`
	Absolute   string   `json:"absolute"`
	Relative   string   `json:"relative"`
	Clean      string   `json:"clean"`
	Dir        string   `json:"dir"`
	Base       string   `json:"base"`
	Ext        string   `json:"ext"`
	Name       string   `json:"name"`
	Type       PathType `json:"type"`
	Exists     bool     `json:"exists"`
	IsAbsolute bool     `json:"is_absolute"`
	Components []string `json:"components"`
}

// PathMatcher provides path matching utilities
type PathMatcher struct {
	patterns   []string
	isGlob     []bool
	isNegation []bool
}

// PathResolver resolves paths with various strategies
type PathResolver struct {
	basePaths     []string
	extensions    []string
	caseSensitive bool
}

// PathNormalizer normalizes paths across platforms
type PathNormalizer struct {
	separator    rune
	preserveCase bool
	expandTilde  bool
	expandVars   bool
}

// String returns the string representation of PathType
func (pt PathType) String() string {
	switch pt {
	case PathTypeFile:
		return "file"
	case PathTypeDir:
		return "directory"
	case PathTypeSymlink:
		return "symlink"
	case PathTypeBroken:
		return "broken"
	default:
		return "unknown"
	}
}

// GetPathInfo returns comprehensive information about a path
func GetPathInfo(path string) (*PathInfo, error) {
	info := &PathInfo{
		Original: path,
		Clean:    filepath.Clean(path),
	}

	// Get absolute path
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}
	info.Absolute = abs
	info.IsAbsolute = filepath.IsAbs(path)

	// Get relative path from current directory
	if cwd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(cwd, abs); err == nil {
			info.Relative = rel
		}
	}

	// Parse path components
	info.Dir = filepath.Dir(info.Clean)
	info.Base = filepath.Base(info.Clean)
	info.Ext = filepath.Ext(info.Base)
	info.Name = strings.TrimSuffix(info.Base, info.Ext)

	// Split into components
	info.Components = splitPath(info.Clean)

	// Check existence and type
	if stat, err := os.Lstat(abs); err == nil {
		info.Exists = true

		switch {
		case stat.Mode()&os.ModeSymlink != 0:
			info.Type = PathTypeSymlink
			// Check if symlink target exists
			if _, err := os.Stat(abs); err != nil {
				info.Type = PathTypeBroken
			}
		case stat.IsDir():
			info.Type = PathTypeDir
		default:
			info.Type = PathTypeFile
		}
	} else {
		info.Type = PathTypeUnknown
	}

	return info, nil
}

// JoinPaths joins multiple path components
func JoinPaths(components ...string) string {
	if len(components) == 0 {
		return ""
	}

	// Filter out empty components
	filtered := make([]string, 0, len(components))
	for _, component := range components {
		if component != "" {
			filtered = append(filtered, component)
		}
	}

	if len(filtered) == 0 {
		return ""
	}

	return filepath.Join(filtered...)
}

// ResolvePath resolves a path to an absolute path
func ResolvePath(path string) (string, error) {
	// Expand tilde
	if strings.HasPrefix(path, "~") {
		home, err := GetHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to get home directory: %w", err)
		}

		if path == "~" {
			path = home
		} else if strings.HasPrefix(path, "~/") {
			path = filepath.Join(home, path[2:])
		}
	}

	// Expand environment variables
	path = os.ExpandEnv(path)

	// Get absolute path
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	return abs, nil
}

// NormalizePath normalizes a path for the current platform
func NormalizePath(path string) string {
	// Clean the path
	clean := filepath.Clean(path)

	// Convert separators to platform-appropriate ones
	if runtime.GOOS == "windows" {
		clean = strings.ReplaceAll(clean, "/", "\\")
	} else {
		clean = strings.ReplaceAll(clean, "\\", "/")
	}

	return clean
}

// IsSubPath checks if child is a subpath of parent
func IsSubPath(parent, child string) bool {
	parentAbs, err := filepath.Abs(parent)
	if err != nil {
		return false
	}

	childAbs, err := filepath.Abs(child)
	if err != nil {
		return false
	}

	rel, err := filepath.Rel(parentAbs, childAbs)
	if err != nil {
		return false
	}

	return !strings.HasPrefix(rel, "..") && rel != "."
}

// FindCommonPath finds the common path among multiple paths
func FindCommonPath(paths []string) (string, error) {
	if len(paths) == 0 {
		return "", fmt.Errorf("no paths provided")
	}

	if len(paths) == 1 {
		return filepath.Dir(paths[0]), nil
	}

	// Convert all paths to absolute
	absPaths := make([]string, len(paths))
	for i, path := range paths {
		abs, err := filepath.Abs(path)
		if err != nil {
			return "", fmt.Errorf("failed to get absolute path for %s: %w", path, err)
		}
		absPaths[i] = abs
	}

	// Split all paths into components
	pathComponents := make([][]string, len(absPaths))
	for i, path := range absPaths {
		pathComponents[i] = splitPath(path)
	}

	// Find common components
	var commonComponents []string
	minLength := len(pathComponents[0])
	for i := 1; i < len(pathComponents); i++ {
		if len(pathComponents[i]) < minLength {
			minLength = len(pathComponents[i])
		}
	}

	for i := 0; i < minLength; i++ {
		component := pathComponents[0][i]
		isCommon := true

		for j := 1; j < len(pathComponents); j++ {
			if !equalPath(pathComponents[j][i], component) {
				isCommon = false
				break
			}
		}

		if isCommon {
			commonComponents = append(commonComponents, component)
		} else {
			break
		}
	}

	if len(commonComponents) == 0 {
		return string(filepath.Separator), nil
	}

	return filepath.Join(commonComponents...), nil
}

// GetHomeDir returns the user's home directory
func GetHomeDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		// Fallback to environment variables
		if runtime.GOOS == "windows" {
			home = os.Getenv("USERPROFILE")
			if home == "" {
				home = os.Getenv("HOMEDRIVE") + os.Getenv("HOMEPATH")
			}
		} else {
			home = os.Getenv("HOME")
		}

		if home == "" {
			return "", fmt.Errorf("unable to determine home directory")
		}
	}

	return home, nil
}

// GetConfigDir returns the user's configuration directory
func GetConfigDir(appName string) (string, error) {
	var configDir string

	switch runtime.GOOS {
	case "windows":
		configDir = os.Getenv("APPDATA")
		if configDir == "" {
			home, err := GetHomeDir()
			if err != nil {
				return "", err
			}
			configDir = filepath.Join(home, "AppData", "Roaming")
		}

	case "darwin":
		home, err := GetHomeDir()
		if err != nil {
			return "", err
		}
		configDir = filepath.Join(home, "Library", "Application Support")

	default: // Linux and other Unix-like
		configDir = os.Getenv("XDG_CONFIG_HOME")
		if configDir == "" {
			home, err := GetHomeDir()
			if err != nil {
				return "", err
			}
			configDir = filepath.Join(home, ".config")
		}
	}

	if appName != "" {
		configDir = filepath.Join(configDir, appName)
	}

	return configDir, nil
}

// GetCacheDir returns the user's cache directory
func GetCacheDir(appName string) (string, error) {
	var cacheDir string

	switch runtime.GOOS {
	case "windows":
		cacheDir = os.Getenv("LOCALAPPDATA")
		if cacheDir == "" {
			home, err := GetHomeDir()
			if err != nil {
				return "", err
			}
			cacheDir = filepath.Join(home, "AppData", "Local")
		}

	case "darwin":
		home, err := GetHomeDir()
		if err != nil {
			return "", err
		}
		cacheDir = filepath.Join(home, "Library", "Caches")

	default: // Linux and other Unix-like
		cacheDir = os.Getenv("XDG_CACHE_HOME")
		if cacheDir == "" {
			home, err := GetHomeDir()
			if err != nil {
				return "", err
			}
			cacheDir = filepath.Join(home, ".cache")
		}
	}

	if appName != "" {
		cacheDir = filepath.Join(cacheDir, appName)
	}

	return cacheDir, nil
}

// GetDataDir returns the user's data directory
func GetDataDir(appName string) (string, error) {
	var dataDir string

	switch runtime.GOOS {
	case "windows":
		dataDir = os.Getenv("LOCALAPPDATA")
		if dataDir == "" {
			home, err := GetHomeDir()
			if err != nil {
				return "", err
			}
			dataDir = filepath.Join(home, "AppData", "Local")
		}

	case "darwin":
		home, err := GetHomeDir()
		if err != nil {
			return "", err
		}
		dataDir = filepath.Join(home, "Library", "Application Support")

	default: // Linux and other Unix-like
		dataDir = os.Getenv("XDG_DATA_HOME")
		if dataDir == "" {
			home, err := GetHomeDir()
			if err != nil {
				return "", err
			}
			dataDir = filepath.Join(home, ".local", "share")
		}
	}

	if appName != "" {
		dataDir = filepath.Join(dataDir, appName)
	}

	return dataDir, nil
}

// NewPathMatcher creates a new path matcher
func NewPathMatcher(patterns []string) *PathMatcher {
	pm := &PathMatcher{
		patterns:   make([]string, len(patterns)),
		isGlob:     make([]bool, len(patterns)),
		isNegation: make([]bool, len(patterns)),
	}

	for i, pattern := range patterns {
		// Check for negation
		if strings.HasPrefix(pattern, "!") {
			pm.isNegation[i] = true
			pattern = pattern[1:]
		}

		// Check for glob patterns
		pm.isGlob[i] = strings.ContainsAny(pattern, "*?[]")
		pm.patterns[i] = pattern
	}

	return pm
}

// Matches checks if a path matches any of the patterns
func (pm *PathMatcher) Matches(path string) bool {
	matched := false

	for i, pattern := range pm.patterns {
		var isMatch bool

		if pm.isGlob[i] {
			// Use filepath.Match for glob patterns
			isMatch, _ = filepath.Match(pattern, filepath.Base(path))

			// Also try matching the full path
			if !isMatch {
				isMatch, _ = filepath.Match(pattern, path)
			}
		} else {
			// Simple string matching
			isMatch = strings.Contains(path, pattern)
		}

		if isMatch {
			if pm.isNegation[i] {
				return false // Negation patterns exclude
			}
			matched = true
		}
	}

	return matched
}

// NewPathResolver creates a new path resolver
func NewPathResolver(basePaths []string, extensions []string) *PathResolver {
	return &PathResolver{
		basePaths:     basePaths,
		extensions:    extensions,
		caseSensitive: runtime.GOOS != "windows",
	}
}

// Resolve resolves a path by searching in base paths and trying extensions
func (pr *PathResolver) Resolve(path string) (string, error) {
	// If path is absolute and exists, return as-is
	if filepath.IsAbs(path) {
		if Exists(path) {
			return path, nil
		}
		return "", fmt.Errorf("absolute path does not exist: %s", path)
	}

	// Try the path as-is first
	if Exists(path) {
		return filepath.Abs(path)
	}

	// Search in base paths
	for _, basePath := range pr.basePaths {
		// Try direct join
		candidate := filepath.Join(basePath, path)
		if Exists(candidate) {
			return filepath.Abs(candidate)
		}

		// Try with extensions
		for _, ext := range pr.extensions {
			candidateWithExt := candidate + ext
			if Exists(candidateWithExt) {
				return filepath.Abs(candidateWithExt)
			}
		}
	}

	return "", fmt.Errorf("path not found: %s", path)
}

// SetCaseSensitive sets case sensitivity for path resolution
func (pr *PathResolver) SetCaseSensitive(sensitive bool) {
	pr.caseSensitive = sensitive
}

// NewPathNormalizer creates a new path normalizer
func NewPathNormalizer() *PathNormalizer {
	return &PathNormalizer{
		separator:    filepath.Separator,
		preserveCase: runtime.GOOS != "windows",
		expandTilde:  true,
		expandVars:   true,
	}
}

// Normalize normalizes a path according to the configured rules
func (pn *PathNormalizer) Normalize(path string) (string, error) {
	result := path

	// Expand tilde if enabled
	if pn.expandTilde && strings.HasPrefix(result, "~") {
		home, err := GetHomeDir()
		if err != nil {
			return "", fmt.Errorf("failed to expand tilde: %w", err)
		}

		if result == "~" {
			result = home
		} else if strings.HasPrefix(result, "~/") {
			result = filepath.Join(home, result[2:])
		}
	}

	// Expand environment variables if enabled
	if pn.expandVars {
		result = os.ExpandEnv(result)
	}

	// Clean the path
	result = filepath.Clean(result)

	// Adjust case if not preserving
	if !pn.preserveCase {
		result = strings.ToLower(result)
	}

	// Convert separators if needed
	if pn.separator != filepath.Separator {
		if pn.separator == '/' {
			result = strings.ReplaceAll(result, "\\", "/")
		} else {
			result = strings.ReplaceAll(result, "/", "\\")
		}
	}

	return result, nil
}

// SetOptions configures the normalizer options
func (pn *PathNormalizer) SetOptions(expandTilde, expandVars, preserveCase bool, separator rune) {
	pn.expandTilde = expandTilde
	pn.expandVars = expandVars
	pn.preserveCase = preserveCase
	pn.separator = separator
}

// Utility functions

// splitPath splits a path into its components
func splitPath(path string) []string {
	// Handle root specially
	if path == string(filepath.Separator) {
		return []string{string(filepath.Separator)}
	}

	// Clean and split
	clean := filepath.Clean(path)
	components := strings.Split(clean, string(filepath.Separator))

	// Filter out empty components (except for root)
	filtered := make([]string, 0, len(components))
	for i, component := range components {
		if component != "" {
			filtered = append(filtered, component)
		} else if i == 0 {
			// Keep root separator
			filtered = append(filtered, string(filepath.Separator))
		}
	}

	return filtered
}

// equalPath compares two path components for equality
func equalPath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// SortPaths sorts paths in a logical order
func SortPaths(paths []string) {
	sort.Slice(paths, func(i, j int) bool {
		// Split paths into components for proper sorting
		aComponents := splitPath(paths[i])
		bComponents := splitPath(paths[j])

		// Compare component by component
		minLen := len(aComponents)
		if len(bComponents) < minLen {
			minLen = len(bComponents)
		}

		for k := 0; k < minLen; k++ {
			cmp := strings.Compare(aComponents[k], bComponents[k])
			if cmp != 0 {
				return cmp < 0
			}
		}

		// If all compared components are equal, shorter path comes first
		return len(aComponents) < len(bComponents)
	})
}

// FilterPaths filters paths based on criteria
func FilterPaths(paths []string, filter func(string) bool) []string {
	var filtered []string

	for _, path := range paths {
		if filter(path) {
			filtered = append(filtered, path)
		}
	}

	return filtered
}

// GroupPathsByDir groups paths by their parent directory
func GroupPathsByDir(paths []string) map[string][]string {
	groups := make(map[string][]string)

	for _, path := range paths {
		dir := filepath.Dir(path)
		groups[dir] = append(groups[dir], path)
	}

	return groups
}

// GetPathDepth returns the depth of a path (number of separators)
func GetPathDepth(path string) int {
	clean := filepath.Clean(path)
	if clean == "." || clean == string(filepath.Separator) {
		return 0
	}

	return strings.Count(clean, string(filepath.Separator))
}

// MakeRelativeTo makes a path relative to a base path
func MakeRelativeTo(path, base string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute path: %w", err)
	}

	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", fmt.Errorf("failed to get absolute base path: %w", err)
	}

	rel, err := filepath.Rel(absBase, absPath)
	if err != nil {
		return "", fmt.Errorf("failed to get relative path: %w", err)
	}

	return rel, nil
}

// IsHidden checks if a path represents a hidden file or directory
func IsHidden(path string) bool {
	base := filepath.Base(path)

	// Unix-style hidden files (start with dot)
	if strings.HasPrefix(base, ".") && base != "." && base != ".." {
		return true
	}

	// Windows-specific hidden file check would go here
	// This is a simplified version

	return false
}

// HasExtension checks if a path has any of the given extensions
func HasExtension(path string, extensions []string) bool {
	ext := strings.ToLower(filepath.Ext(path))

	for _, checkExt := range extensions {
		if strings.ToLower(checkExt) == ext {
			return true
		}
	}

	return false
}

// ChangeExtension changes the extension of a path
func ChangeExtension(path, newExt string) string {
	if !strings.HasPrefix(newExt, ".") {
		newExt = "." + newExt
	}

	dir := filepath.Dir(path)
	name := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))

	return filepath.Join(dir, name+newExt)
}
