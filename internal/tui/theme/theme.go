package theme

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/vivalchemy/wake/pkg/logger"
)

// Theme represents a complete UI theme
type Theme struct {
	// Theme metadata
	name        string
	description string
	author      string
	version     string

	// Color palette
	colors *Colors

	// Base styles
	styles *Styles

	// Logger
	logger logger.Logger
}

// Colors defines the color palette for the theme
type Colors struct {
	// Primary colors
	Primary   lipgloss.Color
	Secondary lipgloss.Color
	Tertiary  lipgloss.Color

	// Text colors
	Text          lipgloss.Color
	TextInverted  lipgloss.Color
	TextMuted     lipgloss.Color
	TextSecondary lipgloss.Color

	// Background colors
	Background lipgloss.Color
	Surface    lipgloss.Color
	SurfaceAlt lipgloss.Color

	// State colors
	Success lipgloss.Color
	Warning lipgloss.Color
	Error   lipgloss.Color
	Info    lipgloss.Color

	// Interactive colors
	Border       lipgloss.Color
	BorderFocus  lipgloss.Color
	BorderActive lipgloss.Color

	// Status colors
	Running   lipgloss.Color
	Stopped   lipgloss.Color
	Failed    lipgloss.Color
	Completed lipgloss.Color

	// Special colors
	Accent    lipgloss.Color
	Highlight lipgloss.Color
	Muted     lipgloss.Color
	Disabled  lipgloss.Color
}

// Styles defines the base styles for the theme
type Styles struct {
	// Base styles
	Base      lipgloss.Style
	Container lipgloss.Style
	Panel     lipgloss.Style

	// Text styles
	Title    lipgloss.Style
	Subtitle lipgloss.Style
	Body     lipgloss.Style
	Caption  lipgloss.Style
	Code     lipgloss.Style

	// Interactive styles
	Button       lipgloss.Style
	ButtonFocus  lipgloss.Style
	ButtonActive lipgloss.Style
	Input        lipgloss.Style
	InputFocus   lipgloss.Style

	// Layout styles
	Border  lipgloss.Style
	Divider lipgloss.Style
	Header  lipgloss.Style
	Footer  lipgloss.Style
	Sidebar lipgloss.Style

	// State styles
	Success lipgloss.Style
	Warning lipgloss.Style
	Error   lipgloss.Style
	Info    lipgloss.Style

	// Special styles
	Highlight lipgloss.Style
	Muted     lipgloss.Style
	Disabled  lipgloss.Style
}

// ThemeProvider manages theme loading and application
type ThemeProvider struct {
	// Available themes
	themes       map[string]*Theme
	currentTheme *Theme
	defaultTheme string

	// Configuration
	autoDetect   bool
	terminalInfo *TerminalInfo

	// Logger
	logger logger.Logger
}

// TerminalInfo contains information about the terminal
type TerminalInfo struct {
	ColorSupport ColorSupport
	Width        int
	Height       int
	IsTrueColor  bool
	Is256Color   bool
	IsMonochrome bool
	HasMouse     bool
	HasAltScreen bool
}

// ColorSupport represents the color support level
type ColorSupport int

const (
	ColorSupportNone ColorSupport = iota
	ColorSupport16
	ColorSupport256
	ColorSupportTrueColor
)

// String returns the string representation of ColorSupport
func (c ColorSupport) String() string {
	switch c {
	case ColorSupportNone:
		return "none"
	case ColorSupport16:
		return "16-color"
	case ColorSupport256:
		return "256-color"
	case ColorSupportTrueColor:
		return "true-color"
	default:
		return "unknown"
	}
}

// NewThemeProvider creates a new theme provider
func NewThemeProvider(logger logger.Logger) *ThemeProvider {
	provider := &ThemeProvider{
		themes:       make(map[string]*Theme),
		defaultTheme: "default",
		autoDetect:   true,
		logger:       logger.WithGroup("theme"),
	}

	// Initialize with built-in themes
	provider.initializeBuiltinThemes()

	// Detect terminal capabilities
	provider.detectTerminalInfo()

	// Set initial theme
	provider.SetTheme(provider.defaultTheme)

	return provider
}

// initializeBuiltinThemes initializes the built-in themes
func (tp *ThemeProvider) initializeBuiltinThemes() {
	// Default theme
	defaultTheme := createDefaultTheme(tp.logger)
	tp.themes["default"] = defaultTheme

	// Dark theme
	darkTheme := createDarkTheme(tp.logger)
	tp.themes["dark"] = darkTheme

	// Light theme
	lightTheme := createLightTheme(tp.logger)
	tp.themes["light"] = lightTheme

	// High contrast theme
	highContrastTheme := createHighContrastTheme(tp.logger)
	tp.themes["high-contrast"] = highContrastTheme

	// Minimal theme
	minimalTheme := createMinimalTheme(tp.logger)
	tp.themes["minimal"] = minimalTheme

	// Colorful theme
	colorfulTheme := createColorfulTheme(tp.logger)
	tp.themes["colorful"] = colorfulTheme

	tp.logger.Info("Initialized built-in themes", "count", len(tp.themes))
}

// detectTerminalInfo detects terminal capabilities
func (tp *ThemeProvider) detectTerminalInfo() {
	info := &TerminalInfo{
		ColorSupport: tp.detectColorSupport(),
		HasMouse:     tp.detectMouseSupport(),
		HasAltScreen: tp.detectAltScreenSupport(),
	}

	// Set color flags based on support
	switch info.ColorSupport {
	case ColorSupportTrueColor:
		info.IsTrueColor = true
		info.Is256Color = true
	case ColorSupport256:
		info.Is256Color = true
	case ColorSupportNone:
		info.IsMonochrome = true
	}

	tp.terminalInfo = info

	tp.logger.Info("Detected terminal capabilities",
		"color_support", info.ColorSupport.String(),
		"true_color", info.IsTrueColor,
		"256_color", info.Is256Color,
		"monochrome", info.IsMonochrome,
		"mouse", info.HasMouse,
		"alt_screen", info.HasAltScreen)
}

// detectColorSupport detects the color support level
func (tp *ThemeProvider) detectColorSupport() ColorSupport {
	// Check environment variables for color support hints
	colorTerm := strings.ToLower(os.Getenv("COLORTERM"))
	if strings.Contains(colorTerm, "truecolor") || strings.Contains(colorTerm, "24bit") {
		return ColorSupportTrueColor
	}

	term := strings.ToLower(os.Getenv("TERM"))
	if strings.Contains(term, "256") || strings.Contains(term, "256color") {
		return ColorSupport256
	}

	// Check color profile using termenv
	profile := termenv.ColorProfile()
	switch profile {
	case termenv.TrueColor:
		return ColorSupportTrueColor
	case termenv.ANSI256:
		return ColorSupport256
	case termenv.ANSI:
		return ColorSupport16
	default:
		return ColorSupportNone
	}
}

// detectMouseSupport detects mouse support
func (tp *ThemeProvider) detectMouseSupport() bool {
	// Check for common terminals that support mouse
	term := strings.ToLower(os.Getenv("TERM"))
	termProgram := strings.ToLower(os.Getenv("TERM_PROGRAM"))

	// Most modern terminals support mouse
	supportedTerms := []string{
		"xterm", "screen", "tmux", "alacritty", "kitty",
		"iterm", "gnome", "konsole", "terminal",
	}

	for _, supportedTerm := range supportedTerms {
		if strings.Contains(term, supportedTerm) || strings.Contains(termProgram, supportedTerm) {
			return true
		}
	}

	return false
}

// detectAltScreenSupport detects alternate screen support
func (tp *ThemeProvider) detectAltScreenSupport() bool {
	// Most terminals that support mouse also support alternate screen
	return tp.detectMouseSupport()
}

// SetTheme sets the current theme
func (tp *ThemeProvider) SetTheme(themeName string) error {
	theme, exists := tp.themes[themeName]
	if !exists {
		return fmt.Errorf("theme not found: %s", themeName)
	}

	tp.currentTheme = theme
	tp.logger.Info("Theme changed", "theme", themeName)

	return nil
}

// GetTheme returns the current theme
func (tp *ThemeProvider) GetTheme() *Theme {
	return tp.currentTheme
}

// GetAvailableThemes returns a list of available theme names
func (tp *ThemeProvider) GetAvailableThemes() []string {
	themes := make([]string, 0, len(tp.themes))
	for name := range tp.themes {
		themes = append(themes, name)
	}
	return themes
}

// AddTheme adds a custom theme
func (tp *ThemeProvider) AddTheme(theme *Theme) error {
	if theme.name == "" {
		return fmt.Errorf("theme name cannot be empty")
	}

	tp.themes[theme.name] = theme
	tp.logger.Info("Added custom theme", "name", theme.name)

	return nil
}

// RemoveTheme removes a theme
func (tp *ThemeProvider) RemoveTheme(themeName string) error {
	if themeName == tp.defaultTheme {
		return fmt.Errorf("cannot remove default theme")
	}

	if _, exists := tp.themes[themeName]; !exists {
		return fmt.Errorf("theme not found: %s", themeName)
	}

	delete(tp.themes, themeName)

	// Switch to default if current theme was removed
	if tp.currentTheme != nil && tp.currentTheme.name == themeName {
		tp.SetTheme(tp.defaultTheme)
	}

	tp.logger.Info("Removed theme", "name", themeName)
	return nil
}

// GetTerminalInfo returns terminal information
func (tp *ThemeProvider) GetTerminalInfo() *TerminalInfo {
	return tp.terminalInfo
}

// AdaptToTerminal adapts the current theme to terminal capabilities
func (tp *ThemeProvider) AdaptToTerminal() {
	if tp.currentTheme == nil || tp.terminalInfo == nil {
		return
	}

	// Adapt colors based on terminal support
	if tp.terminalInfo.IsMonochrome {
		tp.adaptToMonochrome()
	} else if !tp.terminalInfo.Is256Color {
		tp.adaptTo16Color()
	}
}

// adaptToMonochrome adapts theme for monochrome terminals
func (tp *ThemeProvider) adaptToMonochrome() {
	colors := tp.currentTheme.colors

	// Convert all colors to grayscale
	colors.Primary = lipgloss.Color("15")     // White
	colors.Secondary = lipgloss.Color("8")    // Bright black
	colors.Text = lipgloss.Color("15")        // White
	colors.TextInverted = lipgloss.Color("0") // Black
	colors.Background = lipgloss.Color("0")   // Black
	colors.Border = lipgloss.Color("8")       // Bright black

	tp.logger.Info("Adapted theme for monochrome terminal")
}

// adaptTo16Color adapts theme for 16-color terminals
func (tp *ThemeProvider) adaptTo16Color() {
	colors := tp.currentTheme.colors

	// Map colors to ANSI 16-color palette
	colors.Primary = lipgloss.Color("12")   // Bright blue
	colors.Secondary = lipgloss.Color("14") // Bright cyan
	colors.Success = lipgloss.Color("10")   // Bright green
	colors.Warning = lipgloss.Color("11")   // Bright yellow
	colors.Error = lipgloss.Color("9")      // Bright red
	colors.Info = lipgloss.Color("12")      // Bright blue

	tp.logger.Info("Adapted theme for 16-color terminal")
}

// Theme methods

// NewTheme creates a new theme
func NewTheme(name, description, author, version string, colors *Colors, logger logger.Logger) *Theme {
	theme := &Theme{
		name:        name,
		description: description,
		author:      author,
		version:     version,
		colors:      colors,
		logger:      logger.WithGroup("theme"),
	}

	// Generate styles from colors
	theme.styles = generateStyles(colors)

	return theme
}

// Name returns the theme name
func (t *Theme) Name() string {
	return t.name
}

// Description returns the theme description
func (t *Theme) Description() string {
	return t.description
}

// Author returns the theme author
func (t *Theme) Author() string {
	return t.author
}

// Version returns the theme version
func (t *Theme) Version() string {
	return t.version
}

// Colors returns the theme colors
func (t *Theme) Colors() *Colors {
	return t.colors
}

// Styles returns the theme styles
func (t *Theme) Styles() *Styles {
	return t.styles
}

// generateStyles generates styles from colors
func generateStyles(colors *Colors) *Styles {
	return &Styles{
		// Base styles
		Base: lipgloss.NewStyle().
			Foreground(colors.Text).
			Background(colors.Background),

		Container: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colors.Border),

		Panel: lipgloss.NewStyle().
			Background(colors.Surface).
			Padding(1),

		// Text styles
		Title: lipgloss.NewStyle().
			Foreground(colors.Primary).
			Bold(true).
			MarginBottom(1),

		Subtitle: lipgloss.NewStyle().
			Foreground(colors.Secondary).
			Bold(true),

		Body: lipgloss.NewStyle().
			Foreground(colors.Text),

		Caption: lipgloss.NewStyle().
			Foreground(colors.TextMuted).
			Italic(true),

		Code: lipgloss.NewStyle().
			Foreground(colors.Accent).
			Background(colors.SurfaceAlt).
			Padding(0, 1),

		// Interactive styles
		Button: lipgloss.NewStyle().
			Background(colors.Primary).
			Foreground(colors.TextInverted).
			Padding(0, 2).
			Margin(0, 1),

		ButtonFocus: lipgloss.NewStyle().
			Background(colors.Accent).
			Foreground(colors.TextInverted).
			Padding(0, 2).
			Margin(0, 1).
			Bold(true),

		ButtonActive: lipgloss.NewStyle().
			Background(colors.Secondary).
			Foreground(colors.TextInverted).
			Padding(0, 2).
			Margin(0, 1),

		Input: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(colors.Border).
			Padding(0, 1),

		InputFocus: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(colors.BorderFocus).
			Padding(0, 1),

		// Layout styles
		Border: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(colors.Border),

		Divider: lipgloss.NewStyle().
			Foreground(colors.Border),

		Header: lipgloss.NewStyle().
			Background(colors.Primary).
			Foreground(colors.TextInverted).
			Padding(0, 1).
			Bold(true),

		Footer: lipgloss.NewStyle().
			Background(colors.Surface).
			Foreground(colors.TextMuted).
			Padding(0, 1),

		Sidebar: lipgloss.NewStyle().
			Background(colors.SurfaceAlt).
			Padding(1),

		// State styles
		Success: lipgloss.NewStyle().
			Foreground(colors.Success).
			Bold(true),

		Warning: lipgloss.NewStyle().
			Foreground(colors.Warning).
			Bold(true),

		Error: lipgloss.NewStyle().
			Foreground(colors.Error).
			Bold(true),

		Info: lipgloss.NewStyle().
			Foreground(colors.Info),

		// Special styles
		Highlight: lipgloss.NewStyle().
			Background(colors.Highlight).
			Foreground(colors.TextInverted),

		Muted: lipgloss.NewStyle().
			Foreground(colors.Muted),

		Disabled: lipgloss.NewStyle().
			Foreground(colors.Disabled).
			Italic(true),
	}
}

// Built-in theme creators

// createDefaultTheme creates the default theme
func createDefaultTheme(logger logger.Logger) *Theme {
	colors := &Colors{
		Primary:       lipgloss.Color("#0066CC"),
		Secondary:     lipgloss.Color("#6699FF"),
		Tertiary:      lipgloss.Color("#99CCFF"),
		Text:          lipgloss.Color("#FFFFFF"),
		TextInverted:  lipgloss.Color("#000000"),
		TextMuted:     lipgloss.Color("#CCCCCC"),
		TextSecondary: lipgloss.Color("#999999"),
		Background:    lipgloss.Color("#000000"),
		Surface:       lipgloss.Color("#111111"),
		SurfaceAlt:    lipgloss.Color("#222222"),
		Success:       lipgloss.Color("#00FF00"),
		Warning:       lipgloss.Color("#FFFF00"),
		Error:         lipgloss.Color("#FF0000"),
		Info:          lipgloss.Color("#00FFFF"),
		Border:        lipgloss.Color("#444444"),
		BorderFocus:   lipgloss.Color("#0066CC"),
		BorderActive:  lipgloss.Color("#00FF00"),
		Running:       lipgloss.Color("#FFFF00"),
		Stopped:       lipgloss.Color("#FF8800"),
		Failed:        lipgloss.Color("#FF0000"),
		Completed:     lipgloss.Color("#00FF00"),
		Accent:        lipgloss.Color("#FF6600"),
		Highlight:     lipgloss.Color("#FFFF00"),
		Muted:         lipgloss.Color("#666666"),
		Disabled:      lipgloss.Color("#333333"),
	}

	return NewTheme("default", "Default theme", "Wake", "1.0.0", colors, logger)
}

// createDarkTheme creates a dark theme
func createDarkTheme(logger logger.Logger) *Theme {
	colors := &Colors{
		Primary:       lipgloss.Color("#BB86FC"),
		Secondary:     lipgloss.Color("#03DAC6"),
		Tertiary:      lipgloss.Color("#3700B3"),
		Text:          lipgloss.Color("#FFFFFF"),
		TextInverted:  lipgloss.Color("#000000"),
		TextMuted:     lipgloss.Color("#AAAAAA"),
		TextSecondary: lipgloss.Color("#888888"),
		Background:    lipgloss.Color("#121212"),
		Surface:       lipgloss.Color("#1E1E1E"),
		SurfaceAlt:    lipgloss.Color("#2C2C2C"),
		Success:       lipgloss.Color("#4CAF50"),
		Warning:       lipgloss.Color("#FF9800"),
		Error:         lipgloss.Color("#F44336"),
		Info:          lipgloss.Color("#2196F3"),
		Border:        lipgloss.Color("#444444"),
		BorderFocus:   lipgloss.Color("#BB86FC"),
		BorderActive:  lipgloss.Color("#03DAC6"),
		Running:       lipgloss.Color("#FF9800"),
		Stopped:       lipgloss.Color("#9E9E9E"),
		Failed:        lipgloss.Color("#F44336"),
		Completed:     lipgloss.Color("#4CAF50"),
		Accent:        lipgloss.Color("#FF6B35"),
		Highlight:     lipgloss.Color("#FFD700"),
		Muted:         lipgloss.Color("#666666"),
		Disabled:      lipgloss.Color("#424242"),
	}

	return NewTheme("dark", "Material Dark theme", "Wake", "1.0.0", colors, logger)
}

// createLightTheme creates a light theme
func createLightTheme(logger logger.Logger) *Theme {
	colors := &Colors{
		Primary:       lipgloss.Color("#6200EE"),
		Secondary:     lipgloss.Color("#018786"),
		Tertiary:      lipgloss.Color("#3700B3"),
		Text:          lipgloss.Color("#000000"),
		TextInverted:  lipgloss.Color("#FFFFFF"),
		TextMuted:     lipgloss.Color("#666666"),
		TextSecondary: lipgloss.Color("#888888"),
		Background:    lipgloss.Color("#FFFFFF"),
		Surface:       lipgloss.Color("#FAFAFA"),
		SurfaceAlt:    lipgloss.Color("#F5F5F5"),
		Success:       lipgloss.Color("#4CAF50"),
		Warning:       lipgloss.Color("#FF9800"),
		Error:         lipgloss.Color("#F44336"),
		Info:          lipgloss.Color("#2196F3"),
		Border:        lipgloss.Color("#E0E0E0"),
		BorderFocus:   lipgloss.Color("#6200EE"),
		BorderActive:  lipgloss.Color("#018786"),
		Running:       lipgloss.Color("#FF9800"),
		Stopped:       lipgloss.Color("#9E9E9E"),
		Failed:        lipgloss.Color("#F44336"),
		Completed:     lipgloss.Color("#4CAF50"),
		Accent:        lipgloss.Color("#FF6B35"),
		Highlight:     lipgloss.Color("#FFF59D"),
		Muted:         lipgloss.Color("#BDBDBD"),
		Disabled:      lipgloss.Color("#E0E0E0"),
	}

	return NewTheme("light", "Material Light theme", "Wake", "1.0.0", colors, logger)
}

// createHighContrastTheme creates a high contrast theme
func createHighContrastTheme(logger logger.Logger) *Theme {
	colors := &Colors{
		Primary:       lipgloss.Color("#FFFFFF"),
		Secondary:     lipgloss.Color("#FFFF00"),
		Tertiary:      lipgloss.Color("#00FFFF"),
		Text:          lipgloss.Color("#FFFFFF"),
		TextInverted:  lipgloss.Color("#000000"),
		TextMuted:     lipgloss.Color("#CCCCCC"),
		TextSecondary: lipgloss.Color("#AAAAAA"),
		Background:    lipgloss.Color("#000000"),
		Surface:       lipgloss.Color("#000000"),
		SurfaceAlt:    lipgloss.Color("#111111"),
		Success:       lipgloss.Color("#00FF00"),
		Warning:       lipgloss.Color("#FFFF00"),
		Error:         lipgloss.Color("#FF0000"),
		Info:          lipgloss.Color("#00FFFF"),
		Border:        lipgloss.Color("#FFFFFF"),
		BorderFocus:   lipgloss.Color("#FFFF00"),
		BorderActive:  lipgloss.Color("#00FF00"),
		Running:       lipgloss.Color("#FFFF00"),
		Stopped:       lipgloss.Color("#FFFFFF"),
		Failed:        lipgloss.Color("#FF0000"),
		Completed:     lipgloss.Color("#00FF00"),
		Accent:        lipgloss.Color("#FF00FF"),
		Highlight:     lipgloss.Color("#FFFF00"),
		Muted:         lipgloss.Color("#888888"),
		Disabled:      lipgloss.Color("#444444"),
	}

	return NewTheme("high-contrast", "High contrast theme", "Wake", "1.0.0", colors, logger)
}

// createMinimalTheme creates a minimal theme
func createMinimalTheme(logger logger.Logger) *Theme {
	colors := &Colors{
		Primary:       lipgloss.Color("#FFFFFF"),
		Secondary:     lipgloss.Color("#CCCCCC"),
		Tertiary:      lipgloss.Color("#999999"),
		Text:          lipgloss.Color("#FFFFFF"),
		TextInverted:  lipgloss.Color("#000000"),
		TextMuted:     lipgloss.Color("#888888"),
		TextSecondary: lipgloss.Color("#666666"),
		Background:    lipgloss.Color("#000000"),
		Surface:       lipgloss.Color("#000000"),
		SurfaceAlt:    lipgloss.Color("#111111"),
		Success:       lipgloss.Color("#FFFFFF"),
		Warning:       lipgloss.Color("#FFFFFF"),
		Error:         lipgloss.Color("#FFFFFF"),
		Info:          lipgloss.Color("#FFFFFF"),
		Border:        lipgloss.Color("#333333"),
		BorderFocus:   lipgloss.Color("#FFFFFF"),
		BorderActive:  lipgloss.Color("#FFFFFF"),
		Running:       lipgloss.Color("#FFFFFF"),
		Stopped:       lipgloss.Color("#888888"),
		Failed:        lipgloss.Color("#FFFFFF"),
		Completed:     lipgloss.Color("#FFFFFF"),
		Accent:        lipgloss.Color("#FFFFFF"),
		Highlight:     lipgloss.Color("#333333"),
		Muted:         lipgloss.Color("#444444"),
		Disabled:      lipgloss.Color("#222222"),
	}

	return NewTheme("minimal", "Minimal monochrome theme", "Wake", "1.0.0", colors, logger)
}

// createColorfulTheme creates a colorful theme
func createColorfulTheme(logger logger.Logger) *Theme {
	colors := &Colors{
		Primary:       lipgloss.Color("#FF6B6B"),
		Secondary:     lipgloss.Color("#4ECDC4"),
		Tertiary:      lipgloss.Color("#45B7D1"),
		Text:          lipgloss.Color("#FFFFFF"),
		TextInverted:  lipgloss.Color("#2C3E50"),
		TextMuted:     lipgloss.Color("#BDC3C7"),
		TextSecondary: lipgloss.Color("#95A5A6"),
		Background:    lipgloss.Color("#2C3E50"),
		Surface:       lipgloss.Color("#34495E"),
		SurfaceAlt:    lipgloss.Color("#3B4752"),
		Success:       lipgloss.Color("#2ECC71"),
		Warning:       lipgloss.Color("#F39C12"),
		Error:         lipgloss.Color("#E74C3C"),
		Info:          lipgloss.Color("#3498DB"),
		Border:        lipgloss.Color("#7F8C8D"),
		BorderFocus:   lipgloss.Color("#FF6B6B"),
		BorderActive:  lipgloss.Color("#4ECDC4"),
		Running:       lipgloss.Color("#F39C12"),
		Stopped:       lipgloss.Color("#95A5A6"),
		Failed:        lipgloss.Color("#E74C3C"),
		Completed:     lipgloss.Color("#2ECC71"),
		Accent:        lipgloss.Color("#9B59B6"),
		Highlight:     lipgloss.Color("#F1C40F"),
		Muted:         lipgloss.Color("#7F8C8D"),
		Disabled:      lipgloss.Color("#34495E"),
	}

	return NewTheme("colorful", "Vibrant colorful theme", "Wake", "1.0.0", colors, logger)
}
