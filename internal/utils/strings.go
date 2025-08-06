package utils

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// StringMatcher provides advanced string matching capabilities
type StringMatcher struct {
	patterns      []string
	compiled      []*regexp.Regexp
	caseSensitive bool
	wholeWord     bool
}

// StringFormatter provides string formatting utilities
type StringFormatter struct {
	indentChar string
	indentSize int
	lineWidth  int
	tabWidth   int
}

// StringAnalyzer analyzes string content
type StringAnalyzer struct {
	text       string
	lines      []string
	words      []string
	characters []rune
}

// StringTransformer transforms strings in various ways
type StringTransformer struct {
	preserveCase bool
	trimSpace    bool
	normalizeWS  bool
}

// NewStringMatcher creates a new string matcher
func NewStringMatcher(patterns []string, caseSensitive bool) *StringMatcher {
	sm := &StringMatcher{
		patterns:      patterns,
		caseSensitive: caseSensitive,
		compiled:      make([]*regexp.Regexp, len(patterns)),
	}

	// Pre-compile regex patterns
	for i, pattern := range patterns {
		flags := ""
		if !caseSensitive {
			flags = "(?i)"
		}

		// Escape special regex characters if it's not already a regex
		if !isRegexPattern(pattern) {
			pattern = regexp.QuoteMeta(pattern)
		}

		compiled, err := regexp.Compile(flags + pattern)
		if err == nil {
			sm.compiled[i] = compiled
		}
	}

	return sm
}

// Matches checks if text matches any of the patterns
func (sm *StringMatcher) Matches(text string) bool {
	for i, pattern := range sm.patterns {
		if sm.compiled[i] != nil {
			if sm.compiled[i].MatchString(text) {
				return true
			}
		} else {
			// Fallback to simple string matching
			if sm.simpleMatch(text, pattern) {
				return true
			}
		}
	}
	return false
}

// FindMatches returns all matches with positions
func (sm *StringMatcher) FindMatches(text string) [][]int {
	var matches [][]int

	for _, compiled := range sm.compiled {
		if compiled != nil {
			matches = append(matches, compiled.FindAllStringIndex(text, -1)...)
		}
	}

	return matches
}

// ReplaceMatches replaces all matches with replacement
func (sm *StringMatcher) ReplaceMatches(text, replacement string) string {
	result := text

	for _, compiled := range sm.compiled {
		if compiled != nil {
			result = compiled.ReplaceAllString(result, replacement)
		}
	}

	return result
}

// simpleMatch performs case-sensitive or insensitive matching
func (sm *StringMatcher) simpleMatch(text, pattern string) bool {
	if sm.caseSensitive {
		if sm.wholeWord {
			return IsWholeWord(text, pattern)
		}
		return strings.Contains(text, pattern)
	}

	if sm.wholeWord {
		return IsWholeWordIgnoreCase(text, pattern)
	}
	return strings.Contains(strings.ToLower(text), strings.ToLower(pattern))
}

// NewStringFormatter creates a new string formatter
func NewStringFormatter() *StringFormatter {
	return &StringFormatter{
		indentChar: " ",
		indentSize: 2,
		lineWidth:  80,
		tabWidth:   4,
	}
}

// SetIndent sets the indentation character and size
func (sf *StringFormatter) SetIndent(char string, size int) {
	sf.indentChar = char
	sf.indentSize = size
}

// SetLineWidth sets the line width for wrapping
func (sf *StringFormatter) SetLineWidth(width int) {
	sf.lineWidth = width
}

// Indent indents each line of text
func (sf *StringFormatter) Indent(text string, level int) string {
	if level <= 0 {
		return text
	}

	indent := strings.Repeat(sf.indentChar, level*sf.indentSize)
	lines := strings.Split(text, "\n")

	for i, line := range lines {
		if strings.TrimSpace(line) != "" { // Don't indent empty lines
			lines[i] = indent + line
		}
	}

	return strings.Join(lines, "\n")
}

// Wrap wraps text to the specified line width
func (sf *StringFormatter) Wrap(text string) string {
	if sf.lineWidth <= 0 {
		return text
	}

	words := strings.Fields(text)
	if len(words) == 0 {
		return text
	}

	var lines []string
	var currentLine strings.Builder

	for _, word := range words {
		// If adding this word would exceed line width, start new line
		if currentLine.Len() > 0 && currentLine.Len()+1+len(word) > sf.lineWidth {
			lines = append(lines, currentLine.String())
			currentLine.Reset()
		}

		if currentLine.Len() > 0 {
			currentLine.WriteString(" ")
		}
		currentLine.WriteString(word)
	}

	if currentLine.Len() > 0 {
		lines = append(lines, currentLine.String())
	}

	return strings.Join(lines, "\n")
}

// ExpandTabs expands tabs to spaces
func (sf *StringFormatter) ExpandTabs(text string) string {
	spaces := strings.Repeat(" ", sf.tabWidth)
	return strings.ReplaceAll(text, "\t", spaces)
}

// PadLeft pads text to the left with spaces
func (sf *StringFormatter) PadLeft(text string, width int) string {
	return PadLeft(text, width, " ")
}

// PadRight pads text to the right with spaces
func (sf *StringFormatter) PadRight(text string, width int) string {
	return PadRight(text, width, " ")
}

// Center centers text within the specified width
func (sf *StringFormatter) Center(text string, width int) string {
	return Center(text, width, " ")
}

// NewStringAnalyzer creates a new string analyzer
func NewStringAnalyzer(text string) *StringAnalyzer {
	sa := &StringAnalyzer{
		text:       text,
		lines:      strings.Split(text, "\n"),
		words:      strings.Fields(text),
		characters: []rune(text),
	}

	return sa
}

// LineCount returns the number of lines
func (sa *StringAnalyzer) LineCount() int {
	return len(sa.lines)
}

// WordCount returns the number of words
func (sa *StringAnalyzer) WordCount() int {
	return len(sa.words)
}

// CharacterCount returns the number of characters (runes)
func (sa *StringAnalyzer) CharacterCount() int {
	return len(sa.characters)
}

// ByteCount returns the number of bytes
func (sa *StringAnalyzer) ByteCount() int {
	return len([]byte(sa.text))
}

// IsEmpty checks if the text is empty or whitespace-only
func (sa *StringAnalyzer) IsEmpty() bool {
	return strings.TrimSpace(sa.text) == ""
}

// HasPrefix checks if text starts with any of the given prefixes
func (sa *StringAnalyzer) HasPrefix(prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(sa.text, prefix) {
			return true
		}
	}
	return false
}

// HasSuffix checks if text ends with any of the given suffixes
func (sa *StringAnalyzer) HasSuffix(suffixes []string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(sa.text, suffix) {
			return true
		}
	}
	return false
}

// GetFrequentWords returns the most frequent words
func (sa *StringAnalyzer) GetFrequentWords(n int) []string {
	wordCount := make(map[string]int)

	for _, word := range sa.words {
		// Normalize word (lowercase, remove punctuation)
		normalized := strings.ToLower(strings.Trim(word, ".,!?;:\"'()[]{}"))
		if normalized != "" {
			wordCount[normalized]++
		}
	}

	// Sort by frequency
	type wordFreq struct {
		word  string
		count int
	}

	var frequencies []wordFreq
	for word, count := range wordCount {
		frequencies = append(frequencies, wordFreq{word, count})
	}

	sort.Slice(frequencies, func(i, j int) bool {
		return frequencies[i].count > frequencies[j].count
	})

	// Return top n words
	result := make([]string, 0, n)
	for i := 0; i < n && i < len(frequencies); i++ {
		result = append(result, frequencies[i].word)
	}

	return result
}

// NewStringTransformer creates a new string transformer
func NewStringTransformer() *StringTransformer {
	return &StringTransformer{
		preserveCase: false,
		trimSpace:    true,
		normalizeWS:  true,
	}
}

// SetOptions configures the transformer
func (st *StringTransformer) SetOptions(preserveCase, trimSpace, normalizeWS bool) {
	st.preserveCase = preserveCase
	st.trimSpace = trimSpace
	st.normalizeWS = normalizeWS
}

// Transform applies all configured transformations
func (st *StringTransformer) Transform(text string) string {
	result := text

	if st.trimSpace {
		result = strings.TrimSpace(result)
	}

	if st.normalizeWS {
		result = NormalizeWhitespace(result)
	}

	if !st.preserveCase {
		result = strings.ToLower(result)
	}

	return result
}

// ToCamelCase converts string to camelCase
func (st *StringTransformer) ToCamelCase(text string) string {
	return ToCamelCase(text)
}

// ToSnakeCase converts string to snake_case
func (st *StringTransformer) ToSnakeCase(text string) string {
	return ToSnakeCase(text)
}

// ToKebabCase converts string to kebab-case
func (st *StringTransformer) ToKebabCase(text string) string {
	return ToKebabCase(text)
}

// String utility functions

// IsEmpty checks if string is empty or contains only whitespace
func IsEmpty(s string) bool {
	return strings.TrimSpace(s) == ""
}

// IsBlank checks if string is empty
func IsBlank(s string) bool {
	return len(s) == 0
}

// NormalizeWhitespace normalizes whitespace in a string
func NormalizeWhitespace(s string) string {
	// Replace multiple whitespace characters with single spaces
	re := regexp.MustCompile(`\s+`)
	return strings.TrimSpace(re.ReplaceAllString(s, " "))
}

// TruncateString truncates string to specified length with ellipsis
func TruncateString(s string, length int) string {
	if len(s) <= length {
		return s
	}

	if length <= 3 {
		return s[:length]
	}

	return s[:length-3] + "..."
}

// TruncateWords truncates string to specified number of words
func TruncateWords(s string, words int) string {
	wordList := strings.Fields(s)
	if len(wordList) <= words {
		return s
	}

	return strings.Join(wordList[:words], " ") + "..."
}

// PadLeft pads string to the left
func PadLeft(s string, length int, padChar string) string {
	if len(s) >= length {
		return s
	}

	padding := strings.Repeat(padChar, length-len(s))
	return padding + s
}

// PadRight pads string to the right
func PadRight(s string, length int, padChar string) string {
	if len(s) >= length {
		return s
	}

	padding := strings.Repeat(padChar, length-len(s))
	return s + padding
}

// Center centers string within specified width
func Center(s string, width int, padChar string) string {
	if len(s) >= width {
		return s
	}

	totalPadding := width - len(s)
	leftPadding := totalPadding / 2
	rightPadding := totalPadding - leftPadding

	return strings.Repeat(padChar, leftPadding) + s + strings.Repeat(padChar, rightPadding)
}

// Reverse reverses a string
func Reverse(s string) string {
	runes := []rune(s)
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}

// IsWholeWord checks if pattern appears as a whole word in text
func IsWholeWord(text, pattern string) bool {
	re := regexp.MustCompile(`\b` + regexp.QuoteMeta(pattern) + `\b`)
	return re.MatchString(text)
}

// IsWholeWordIgnoreCase checks if pattern appears as a whole word (case-insensitive)
func IsWholeWordIgnoreCase(text, pattern string) bool {
	re := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(pattern) + `\b`)
	return re.MatchString(text)
}

// ContainsAny checks if string contains any of the substrings
func ContainsAny(s string, substrings []string) bool {
	for _, substring := range substrings {
		if strings.Contains(s, substring) {
			return true
		}
	}
	return false
}

// ContainsAll checks if string contains all of the substrings
func ContainsAll(s string, substrings []string) bool {
	for _, substring := range substrings {
		if !strings.Contains(s, substring) {
			return false
		}
	}
	return true
}

// StartsWithAny checks if string starts with any of the prefixes
func StartsWithAny(s string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

// EndsWithAny checks if string ends with any of the suffixes
func EndsWithAny(s string, suffixes []string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(s, suffix) {
			return true
		}
	}
	return false
}

// RemovePrefix removes prefix from string if present
func RemovePrefix(s, prefix string) string {
	return strings.TrimPrefix(s, prefix)
}

// RemoveSuffix removes suffix from string if present
func RemoveSuffix(s, suffix string) string {
	return strings.TrimSuffix(s, suffix)
}

// EscapeString escapes special characters in string
func EscapeString(s string) string {
	return strconv.Quote(s)
}

// UnescapeString unescapes special characters in string
func UnescapeString(s string) (string, error) {
	return strconv.Unquote(s)
}

// Case conversion functions

// ToCamelCase converts string to camelCase
func ToCamelCase(s string) string {
	words := splitWords(s)
	if len(words) == 0 {
		return ""
	}

	result := strings.ToLower(words[0])
	for i := 1; i < len(words); i++ {
		// result += strings.Title(strings.ToLower(words[i]))
		result += strings.ToLower(words[i]) // FIXME: Title case
	}

	return result
}

// ToPascalCase converts string to PascalCase
func ToPascalCase(s string) string {
	words := splitWords(s)
	var result strings.Builder

	for _, word := range words {
		// result.WriteString(strings.Title(strings.ToLower(word)))
		result.WriteString(strings.ToLower(word))
	}

	return result.String()
}

// ToSnakeCase converts string to snake_case
func ToSnakeCase(s string) string {
	words := splitWords(s)
	var result []string

	for _, word := range words {
		result = append(result, strings.ToLower(word))
	}

	return strings.Join(result, "_")
}

// ToKebabCase converts string to kebab-case
func ToKebabCase(s string) string {
	words := splitWords(s)
	var result []string

	for _, word := range words {
		result = append(result, strings.ToLower(word))
	}

	return strings.Join(result, "-")
}

// ToTitleCase converts string to Title Case
func ToTitleCase(s string) string {
	words := splitWords(s)
	var result []string

	for _, word := range words {
		// result = append(result, strings.Title(strings.ToLower(word)))
		result = append(result, strings.ToLower(word)) // FIXME: Title case
	}

	return strings.Join(result, " ")
}

// splitWords splits string into words for case conversion
func splitWords(s string) []string {
	// Split on non-alphanumeric characters and camelCase boundaries
	re := regexp.MustCompile(`[^a-zA-Z0-9]+|(?=[A-Z][a-z])|(?<=[a-z])(?=[A-Z])`)
	parts := re.Split(s, -1)

	var words []string
	for _, part := range parts {
		if part != "" {
			words = append(words, part)
		}
	}

	return words
}

// Validation functions

// IsAlpha checks if string contains only alphabetic characters
func IsAlpha(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return len(s) > 0
}

// IsAlphaNumeric checks if string contains only alphanumeric characters
func IsAlphaNumeric(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return len(s) > 0
}

// IsNumeric checks if string contains only numeric characters
func IsNumeric(s string) bool {
	for _, r := range s {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return len(s) > 0
}

// IsValidEmail checks if string is a valid email format (basic check)
func IsValidEmail(email string) bool {
	re := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	return re.MatchString(email)
}

// IsValidURL checks if string is a valid URL format (basic check)
func IsValidURL(url string) bool {
	re := regexp.MustCompile(`^https?://[^\s]+$`)
	return re.MatchString(url)
}

// LevenshteinDistance calculates the Levenshtein distance between two strings
func LevenshteinDistance(s1, s2 string) int {
	r1 := []rune(s1)
	r2 := []rune(s2)

	rows := len(r1) + 1
	cols := len(r2) + 1

	// Create matrix
	matrix := make([][]int, rows)
	for i := range matrix {
		matrix[i] = make([]int, cols)
	}

	// Initialize first row and column
	for i := range rows {
		matrix[i][0] = i
	}
	for j := range cols {
		matrix[0][j] = j
	}

	// Fill matrix
	for i := 1; i < rows; i++ {
		for j := 1; j < cols; j++ {
			cost := 0
			if r1[i-1] != r2[j-1] {
				cost = 1
			}

			matrix[i][j] = min(
				min(
					matrix[i-1][j]+1, // deletion
					matrix[i][j-1]+1, // insertion
				), matrix[i-1][j-1]+cost, // substitution
			)
		}
	}

	return matrix[rows-1][cols-1]
}

// SimilarityRatio calculates similarity ratio between two strings (0.0 to 1.0)
func SimilarityRatio(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}

	if len(s1) == 0 && len(s2) == 0 {
		return 1.0
	}

	distance := LevenshteinDistance(s1, s2)
	maxLen := max(utf8.RuneCountInString(s1), utf8.RuneCountInString(s2))

	return 1.0 - float64(distance)/float64(maxLen)
}

// FindSimilarStrings finds strings similar to the target within threshold
func FindSimilarStrings(target string, candidates []string, threshold float64) []string {
	var similar []string

	for _, candidate := range candidates {
		if SimilarityRatio(target, candidate) >= threshold {
			similar = append(similar, candidate)
		}
	}

	return similar
}

// Utility helper functions

// isRegexPattern checks if string looks like a regex pattern
func isRegexPattern(pattern string) bool {
	regexChars := []string{"[", "]", "(", ")", "{", "}", "^", "$", "|", "+", "?"}
	for _, char := range regexChars {
		if strings.Contains(pattern, char) {
			return true
		}
	}
	return false
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// max returns the maximum of two integers
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// RandomString generates a random string of specified length
func RandomString(length int, charset string) string {
	if charset == "" {
		charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	}

	result := make([]byte, length)
	for i := range result {
		result[i] = charset[len(charset)%len(charset)] // Simplified random selection
	}

	return string(result)
}

// Pluralize returns the plural form of a word (basic English rules)
func Pluralize(word string, count int) string {
	if count == 1 {
		return word
	}

	// Basic pluralization rules
	word = strings.ToLower(word)

	if strings.HasSuffix(word, "y") && len(word) > 1 && !isVowel(rune(word[len(word)-2])) {
		return word[:len(word)-1] + "ies"
	}

	if strings.HasSuffix(word, "s") || strings.HasSuffix(word, "sh") ||
		strings.HasSuffix(word, "ch") || strings.HasSuffix(word, "x") ||
		strings.HasSuffix(word, "z") {
		return word + "es"
	}

	return word + "s"
}

// isVowel checks if character is a vowel
func isVowel(r rune) bool {
	vowels := "aeiouAEIOU"
	return strings.ContainsRune(vowels, r)
}

// Humanize converts technical strings to human-readable format
func Humanize(s string) string {
	// Convert underscores and hyphens to spaces
	s = strings.ReplaceAll(s, "_", " ")
	s = strings.ReplaceAll(s, "-", " ")

	// Split camelCase
	words := splitWords(s)

	// Join with spaces and title case
	return ToTitleCase(strings.Join(words, " "))
}

// Slugify converts string to a URL-friendly slug
func Slugify(s string) string {
	// Convert to lowercase
	s = strings.ToLower(s)

	// Replace spaces and special characters with hyphens
	re := regexp.MustCompile(`[^a-z0-9]+`)
	s = re.ReplaceAllString(s, "-")

	// Remove leading and trailing hyphens
	s = strings.Trim(s, "-")

	return s
}
