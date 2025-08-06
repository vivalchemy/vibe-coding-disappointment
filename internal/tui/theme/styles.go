package theme

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// StyleSet represents a collection of related styles
type StyleSet struct {
	Name        string
	Description string
	Styles      map[string]lipgloss.Style
}

// StyleBuilder helps build complex styles with method chaining
type StyleBuilder struct {
	style lipgloss.Style
}

// ComponentStyles defines styles for specific UI components
type ComponentStyles struct {
	// List styles
	List     *ListStyles
	Table    *TableStyles
	Form     *FormStyles
	Dialog   *DialogStyles
	Menu     *MenuStyles
	Progress *ProgressStyles
	Tree     *TreeStyles
}

// ListStyles defines styles for list components
type ListStyles struct {
	Container    lipgloss.Style
	Item         lipgloss.Style
	ItemSelected lipgloss.Style
	ItemActive   lipgloss.Style
	ItemDisabled lipgloss.Style
	Header       lipgloss.Style
	Footer       lipgloss.Style
	Scrollbar    lipgloss.Style
	NoItems      lipgloss.Style
}

// TableStyles defines styles for table components
type TableStyles struct {
	Container    lipgloss.Style
	Header       lipgloss.Style
	HeaderCell   lipgloss.Style
	Row          lipgloss.Style
	RowSelected  lipgloss.Style
	RowAlternate lipgloss.Style
	Cell         lipgloss.Style
	CellSelected lipgloss.Style
	Border       lipgloss.Style
	Footer       lipgloss.Style
}

// FormStyles defines styles for form components
type FormStyles struct {
	Container     lipgloss.Style
	Label         lipgloss.Style
	Input         lipgloss.Style
	InputFocus    lipgloss.Style
	InputError    lipgloss.Style
	Textarea      lipgloss.Style
	TextareaFocus lipgloss.Style
	Checkbox      lipgloss.Style
	Radio         lipgloss.Style
	Button        lipgloss.Style
	ButtonPrimary lipgloss.Style
	ButtonDanger  lipgloss.Style
	ErrorMessage  lipgloss.Style
	HelpText      lipgloss.Style
}

// DialogStyles defines styles for dialog components
type DialogStyles struct {
	Overlay     lipgloss.Style
	Container   lipgloss.Style
	Header      lipgloss.Style
	Title       lipgloss.Style
	Body        lipgloss.Style
	Footer      lipgloss.Style
	CloseButton lipgloss.Style
	Shadow      lipgloss.Style
}

// MenuStyles defines styles for menu components
type MenuStyles struct {
	Container    lipgloss.Style
	Item         lipgloss.Style
	ItemSelected lipgloss.Style
	ItemDisabled lipgloss.Style
	Separator    lipgloss.Style
	Shortcut     lipgloss.Style
	Icon         lipgloss.Style
	Submenu      lipgloss.Style
}

// ProgressStyles defines styles for progress components
type ProgressStyles struct {
	Container     lipgloss.Style
	Track         lipgloss.Style
	Fill          lipgloss.Style
	Label         lipgloss.Style
	Percentage    lipgloss.Style
	Indeterminate lipgloss.Style
}

// TreeStyles defines styles for tree components
type TreeStyles struct {
	Container     lipgloss.Style
	Node          lipgloss.Style
	NodeSelected  lipgloss.Style
	NodeExpanded  lipgloss.Style
	NodeCollapsed lipgloss.Style
	Leaf          lipgloss.Style
	Branch        lipgloss.Style
	Indent        lipgloss.Style
}

// Predefined style sets
var (
	// Default component styles
	DefaultComponentStyles = &ComponentStyles{}

	// Minimal style set
	MinimalStyleSet = &StyleSet{
		Name:        "minimal",
		Description: "Minimal styling with clean lines",
		Styles:      make(map[string]lipgloss.Style),
	}

	// Rounded style set
	RoundedStyleSet = &StyleSet{
		Name:        "rounded",
		Description: "Rounded borders and soft edges",
		Styles:      make(map[string]lipgloss.Style),
	}

	// Bold style set
	BoldStyleSet = &StyleSet{
		Name:        "bold",
		Description: "Bold and prominent styling",
		Styles:      make(map[string]lipgloss.Style),
	}
)

// NewStyleBuilder creates a new style builder
func NewStyleBuilder() *StyleBuilder {
	return &StyleBuilder{
		style: lipgloss.NewStyle(),
	}
}

// Foreground sets the foreground color
func (sb *StyleBuilder) Foreground(color lipgloss.Color) *StyleBuilder {
	sb.style = sb.style.Foreground(color)
	return sb
}

// Background sets the background color
func (sb *StyleBuilder) Background(color lipgloss.Color) *StyleBuilder {
	sb.style = sb.style.Background(color)
	return sb
}

// Bold makes text bold
func (sb *StyleBuilder) Bold(enabled bool) *StyleBuilder {
	sb.style = sb.style.Bold(enabled)
	return sb
}

// Italic makes text italic
func (sb *StyleBuilder) Italic(enabled bool) *StyleBuilder {
	sb.style = sb.style.Italic(enabled)
	return sb
}

// Underline adds underline
func (sb *StyleBuilder) Underline(enabled bool) *StyleBuilder {
	sb.style = sb.style.Underline(enabled)
	return sb
}

// Border sets the border
func (sb *StyleBuilder) Border(border lipgloss.Border) *StyleBuilder {
	sb.style = sb.style.Border(border)
	return sb
}

// BorderForeground sets the border color
func (sb *StyleBuilder) BorderForeground(color lipgloss.Color) *StyleBuilder {
	sb.style = sb.style.BorderForeground(color)
	return sb
}

// Padding sets padding
func (sb *StyleBuilder) Padding(vertical, horizontal int) *StyleBuilder {
	sb.style = sb.style.Padding(vertical, horizontal)
	return sb
}

// PaddingTop sets top padding
func (sb *StyleBuilder) PaddingTop(padding int) *StyleBuilder {
	sb.style = sb.style.PaddingTop(padding)
	return sb
}

// PaddingRight sets right padding
func (sb *StyleBuilder) PaddingRight(padding int) *StyleBuilder {
	sb.style = sb.style.PaddingRight(padding)
	return sb
}

// PaddingBottom sets bottom padding
func (sb *StyleBuilder) PaddingBottom(padding int) *StyleBuilder {
	sb.style = sb.style.PaddingBottom(padding)
	return sb
}

// PaddingLeft sets left padding
func (sb *StyleBuilder) PaddingLeft(padding int) *StyleBuilder {
	sb.style = sb.style.PaddingLeft(padding)
	return sb
}

// Margin sets margin
func (sb *StyleBuilder) Margin(vertical, horizontal int) *StyleBuilder {
	sb.style = sb.style.Margin(vertical, horizontal)
	return sb
}

// MarginTop sets top margin
func (sb *StyleBuilder) MarginTop(margin int) *StyleBuilder {
	sb.style = sb.style.MarginTop(margin)
	return sb
}

// MarginRight sets right margin
func (sb *StyleBuilder) MarginRight(margin int) *StyleBuilder {
	sb.style = sb.style.MarginRight(margin)
	return sb
}

// MarginBottom sets bottom margin
func (sb *StyleBuilder) MarginBottom(margin int) *StyleBuilder {
	sb.style = sb.style.MarginBottom(margin)
	return sb
}

// MarginLeft sets left margin
func (sb *StyleBuilder) MarginLeft(margin int) *StyleBuilder {
	sb.style = sb.style.MarginLeft(margin)
	return sb
}

// Width sets the width
func (sb *StyleBuilder) Width(width int) *StyleBuilder {
	sb.style = sb.style.Width(width)
	return sb
}

// Height sets the height
func (sb *StyleBuilder) Height(height int) *StyleBuilder {
	sb.style = sb.style.Height(height)
	return sb
}

// MaxWidth sets the maximum width
func (sb *StyleBuilder) MaxWidth(width int) *StyleBuilder {
	sb.style = sb.style.MaxWidth(width)
	return sb
}

// MaxHeight sets the maximum height
func (sb *StyleBuilder) MaxHeight(height int) *StyleBuilder {
	sb.style = sb.style.MaxHeight(height)
	return sb
}

// Align sets text alignment
func (sb *StyleBuilder) Align(position lipgloss.Position) *StyleBuilder {
	sb.style = sb.style.Align(position)
	return sb
}

// AlignHorizontal sets horizontal alignment
func (sb *StyleBuilder) AlignHorizontal(position lipgloss.Position) *StyleBuilder {
	sb.style = sb.style.AlignHorizontal(position)
	return sb
}

// AlignVertical sets vertical alignment
func (sb *StyleBuilder) AlignVertical(position lipgloss.Position) *StyleBuilder {
	sb.style = sb.style.AlignVertical(position)
	return sb
}

// Build returns the built style
func (sb *StyleBuilder) Build() lipgloss.Style {
	return sb.style
}

// ApplyTheme applies theme colors to the style builder
func (sb *StyleBuilder) ApplyTheme(theme *Theme) *StyleBuilder {
	// This could apply default theme colors or allow for theme-aware styling
	return sb
}

// CreateComponentStyles creates component styles from a theme
func CreateComponentStyles(theme *Theme) *ComponentStyles {
	colors := theme.Colors()

	return &ComponentStyles{
		List:     createListStyles(colors),
		Table:    createTableStyles(colors),
		Form:     createFormStyles(colors),
		Dialog:   createDialogStyles(colors),
		Menu:     createMenuStyles(colors),
		Progress: createProgressStyles(colors),
		Tree:     createTreeStyles(colors),
	}
}

// createListStyles creates list styles
func createListStyles(colors *Colors) *ListStyles {
	return &ListStyles{
		Container: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colors.Border).
			Padding(1),

		Item: lipgloss.NewStyle().
			Foreground(colors.Text).
			Padding(0, 1),

		ItemSelected: lipgloss.NewStyle().
			Background(colors.Primary).
			Foreground(colors.TextInverted).
			Padding(0, 1).
			Bold(true),

		ItemActive: lipgloss.NewStyle().
			Background(colors.Secondary).
			Foreground(colors.TextInverted).
			Padding(0, 1),

		ItemDisabled: lipgloss.NewStyle().
			Foreground(colors.Disabled).
			Padding(0, 1).
			Italic(true),

		Header: lipgloss.NewStyle().
			Background(colors.Surface).
			Foreground(colors.TextSecondary).
			Padding(0, 1).
			Bold(true).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(colors.Border),

		Footer: lipgloss.NewStyle().
			Background(colors.Surface).
			Foreground(colors.TextMuted).
			Padding(0, 1).
			Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(colors.Border),

		Scrollbar: lipgloss.NewStyle().
			Foreground(colors.Border),

		NoItems: lipgloss.NewStyle().
			Foreground(colors.TextMuted).
			Italic(true).
			Align(lipgloss.Center),
	}
}

// createTableStyles creates table styles
func createTableStyles(colors *Colors) *TableStyles {
	return &TableStyles{
		Container: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colors.Border),

		Header: lipgloss.NewStyle().
			Background(colors.Primary).
			Foreground(colors.TextInverted).
			Bold(true),

		HeaderCell: lipgloss.NewStyle().
			Padding(0, 2).
			Bold(true),

		Row: lipgloss.NewStyle().
			Foreground(colors.Text),

		RowSelected: lipgloss.NewStyle().
			Background(colors.Highlight).
			Foreground(colors.TextInverted),

		RowAlternate: lipgloss.NewStyle().
			Background(colors.SurfaceAlt).
			Foreground(colors.Text),

		Cell: lipgloss.NewStyle().
			Padding(0, 2),

		CellSelected: lipgloss.NewStyle().
			Background(colors.Primary).
			Foreground(colors.TextInverted).
			Padding(0, 2),

		Border: lipgloss.NewStyle().
			Foreground(colors.Border),

		Footer: lipgloss.NewStyle().
			Background(colors.Surface).
			Foreground(colors.TextMuted).
			Italic(true),
	}
}

// createFormStyles creates form styles
func createFormStyles(colors *Colors) *FormStyles {
	return &FormStyles{
		Container: lipgloss.NewStyle().
			Padding(1),

		Label: lipgloss.NewStyle().
			Foreground(colors.TextSecondary).
			Bold(true).
			MarginRight(1),

		Input: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(colors.Border).
			Padding(0, 1).
			Background(colors.Surface),

		InputFocus: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(colors.Primary).
			Padding(0, 1).
			Background(colors.Surface),

		InputError: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(colors.Error).
			Padding(0, 1).
			Background(colors.Surface),

		Textarea: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colors.Border).
			Padding(1).
			Background(colors.Surface),

		TextareaFocus: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colors.Primary).
			Padding(1).
			Background(colors.Surface),

		Checkbox: lipgloss.NewStyle().
			Foreground(colors.Primary),

		Radio: lipgloss.NewStyle().
			Foreground(colors.Primary),

		Button: lipgloss.NewStyle().
			Background(colors.Secondary).
			Foreground(colors.TextInverted).
			Padding(0, 2).
			Margin(0, 1),

		ButtonPrimary: lipgloss.NewStyle().
			Background(colors.Primary).
			Foreground(colors.TextInverted).
			Padding(0, 2).
			Margin(0, 1).
			Bold(true),

		ButtonDanger: lipgloss.NewStyle().
			Background(colors.Error).
			Foreground(colors.TextInverted).
			Padding(0, 2).
			Margin(0, 1).
			Bold(true),

		ErrorMessage: lipgloss.NewStyle().
			Foreground(colors.Error).
			Italic(true),

		HelpText: lipgloss.NewStyle().
			Foreground(colors.TextMuted).
			Italic(true),
	}
}

// createDialogStyles creates dialog styles
func createDialogStyles(colors *Colors) *DialogStyles {
	return &DialogStyles{
		Overlay: lipgloss.NewStyle().
			Background(lipgloss.Color("#000000")).
			Foreground(lipgloss.Color("#000000")),

		Container: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colors.Border).
			Background(colors.Surface).
			Padding(1),

		Header: lipgloss.NewStyle().
			Background(colors.Primary).
			Foreground(colors.TextInverted).
			Padding(0, 1).
			Bold(true).
			Border(lipgloss.NormalBorder(), false, false, true, false).
			BorderForeground(colors.Border),

		Title: lipgloss.NewStyle().
			Bold(true).
			Align(lipgloss.Center),

		Body: lipgloss.NewStyle().
			Padding(1),

		Footer: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(colors.Border).
			Padding(0, 1).
			Align(lipgloss.Right),

		CloseButton: lipgloss.NewStyle().
			Foreground(colors.Error).
			Bold(true),

		Shadow: lipgloss.NewStyle().
			Background(lipgloss.Color("#000000")).
			Foreground(lipgloss.Color("#000000")),
	}
}

// createMenuStyles creates menu styles
func createMenuStyles(colors *Colors) *MenuStyles {
	return &MenuStyles{
		Container: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colors.Border).
			Background(colors.Surface).
			Padding(1),

		Item: lipgloss.NewStyle().
			Foreground(colors.Text).
			Padding(0, 2),

		ItemSelected: lipgloss.NewStyle().
			Background(colors.Primary).
			Foreground(colors.TextInverted).
			Padding(0, 2),

		ItemDisabled: lipgloss.NewStyle().
			Foreground(colors.Disabled).
			Padding(0, 2).
			Italic(true),

		Separator: lipgloss.NewStyle().
			Foreground(colors.Border).
			Margin(1, 0),

		Shortcut: lipgloss.NewStyle().
			Foreground(colors.TextMuted).
			Align(lipgloss.Right),

		Icon: lipgloss.NewStyle().
			Foreground(colors.Secondary).
			MarginRight(1),

		Submenu: lipgloss.NewStyle().
			Foreground(colors.Secondary),
	}
}

// createProgressStyles creates progress bar styles
func createProgressStyles(colors *Colors) *ProgressStyles {
	return &ProgressStyles{
		Container: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colors.Border),

		Track: lipgloss.NewStyle().
			Background(colors.SurfaceAlt),

		Fill: lipgloss.NewStyle().
			Background(colors.Primary),

		Label: lipgloss.NewStyle().
			Foreground(colors.TextSecondary).
			Bold(true),

		Percentage: lipgloss.NewStyle().
			Foreground(colors.Primary).
			Bold(true),

		Indeterminate: lipgloss.NewStyle().
			Background(colors.Secondary),
	}
}

// createTreeStyles creates tree view styles
func createTreeStyles(colors *Colors) *TreeStyles {
	return &TreeStyles{
		Container: lipgloss.NewStyle().
			Padding(1),

		Node: lipgloss.NewStyle().
			Foreground(colors.Text),

		NodeSelected: lipgloss.NewStyle().
			Background(colors.Primary).
			Foreground(colors.TextInverted).
			Bold(true),

		NodeExpanded: lipgloss.NewStyle().
			Foreground(colors.Primary).
			Bold(true),

		NodeCollapsed: lipgloss.NewStyle().
			Foreground(colors.Secondary),

		Leaf: lipgloss.NewStyle().
			Foreground(colors.TextSecondary),

		Branch: lipgloss.NewStyle().
			Foreground(colors.Primary),

		Indent: lipgloss.NewStyle().
			Foreground(colors.Border),
	}
}

// StyleUtils provides utility functions for style manipulation
type StyleUtils struct{}

// NewStyleUtils creates a new style utilities instance
func NewStyleUtils() *StyleUtils {
	return &StyleUtils{}
}

// MergeStyles merges multiple styles, with later styles taking precedence
func (su *StyleUtils) MergeStyles(styles ...lipgloss.Style) lipgloss.Style {
	if len(styles) == 0 {
		return lipgloss.NewStyle()
	}

	result := styles[0]
	for i := 1; i < len(styles); i++ {
		// This is a simplified merge - lipgloss doesn't expose all properties
		// In practice, you might need to implement more sophisticated merging
		result = result.Inherit(styles[i])
	}

	return result
}

// ConditionalStyle applies a style conditionally
func (su *StyleUtils) ConditionalStyle(condition bool, trueStyle, falseStyle lipgloss.Style) lipgloss.Style {
	if condition {
		return trueStyle
	}
	return falseStyle
}

// StateStyle returns different styles based on state
func (su *StyleUtils) StateStyle(state string, styles map[string]lipgloss.Style, defaultStyle lipgloss.Style) lipgloss.Style {
	if style, exists := styles[state]; exists {
		return style
	}
	return defaultStyle
}

// ResponsiveStyle adjusts style based on available width
func (su *StyleUtils) ResponsiveStyle(width int, breakpoints map[int]lipgloss.Style, defaultStyle lipgloss.Style) lipgloss.Style {
	// Find the largest breakpoint that fits
	largestBreakpoint := 0
	for breakpoint := range breakpoints {
		if breakpoint <= width && breakpoint > largestBreakpoint {
			largestBreakpoint = breakpoint
		}
	}

	if largestBreakpoint > 0 {
		return breakpoints[largestBreakpoint]
	}

	return defaultStyle
}

// TruncateStyle creates a style for truncated text
func (su *StyleUtils) TruncateStyle(maxWidth int, suffix string) func(string) string {
	return func(text string) string {
		if len(text) <= maxWidth {
			return text
		}

		if maxWidth <= len(suffix) {
			return suffix[:maxWidth]
		}

		return text[:maxWidth-len(suffix)] + suffix
	}
}

// WrapStyle creates a style for wrapped text
func (su *StyleUtils) WrapStyle(width int) func(string) string {
	return func(text string) string {
		if width <= 0 {
			return text
		}

		words := strings.Fields(text)
		if len(words) == 0 {
			return text
		}

		var lines []string
		var currentLine strings.Builder

		for _, word := range words {
			if currentLine.Len() == 0 {
				currentLine.WriteString(word)
			} else if currentLine.Len()+1+len(word) <= width {
				currentLine.WriteString(" " + word)
			} else {
				lines = append(lines, currentLine.String())
				currentLine.Reset()
				currentLine.WriteString(word)
			}
		}

		if currentLine.Len() > 0 {
			lines = append(lines, currentLine.String())
		}

		return strings.Join(lines, "\n")
	}
}

// GetStyleSetNames returns available style set names
func GetStyleSetNames() []string {
	return []string{"minimal", "rounded", "bold"}
}

// ApplyStyleSet applies a style set to component styles
func ApplyStyleSet(componentStyles *ComponentStyles, setName string) error {
	switch setName {
	case "minimal":
		applyMinimalStyleSet(componentStyles)
	case "rounded":
		applyRoundedStyleSet(componentStyles)
	case "bold":
		applyBoldStyleSet(componentStyles)
	default:
		return fmt.Errorf("unknown style set: %s", setName)
	}
	return nil
}

// applyMinimalStyleSet applies minimal styling
func applyMinimalStyleSet(styles *ComponentStyles) {
	// Remove borders and minimize decorations
	if styles.List != nil {
		styles.List.Container = styles.List.Container.Border(lipgloss.NormalBorder(), false)
		styles.List.Item = styles.List.Item.Border(lipgloss.NormalBorder(), false)
	}

	if styles.Table != nil {
		styles.Table.Container = styles.Table.Container.Border(lipgloss.NormalBorder(), false)
	}
}

// applyRoundedStyleSet applies rounded styling
func applyRoundedStyleSet(styles *ComponentStyles) {
	// Apply rounded borders everywhere
	roundedBorder := lipgloss.RoundedBorder()

	if styles.List != nil {
		styles.List.Container = styles.List.Container.Border(roundedBorder)
	}

	if styles.Table != nil {
		styles.Table.Container = styles.Table.Container.Border(roundedBorder)
	}

	if styles.Form != nil {
		styles.Form.Input = styles.Form.Input.Border(roundedBorder)
		styles.Form.InputFocus = styles.Form.InputFocus.Border(roundedBorder)
		styles.Form.Textarea = styles.Form.Textarea.Border(roundedBorder)
		styles.Form.TextareaFocus = styles.Form.TextareaFocus.Border(roundedBorder)
	}
}

// applyBoldStyleSet applies bold styling
func applyBoldStyleSet(styles *ComponentStyles) {
	// Make everything bold and prominent
	if styles.List != nil {
		styles.List.Item = styles.List.Item.Bold(true)
		styles.List.Header = styles.List.Header.Bold(true)
	}

	if styles.Table != nil {
		styles.Table.Cell = styles.Table.Cell.Bold(true)
		styles.Table.HeaderCell = styles.Table.HeaderCell.Bold(true)
	}

	if styles.Form != nil {
		styles.Form.Label = styles.Form.Label.Bold(true)
		styles.Form.Button = styles.Form.Button.Bold(true)
	}
}
