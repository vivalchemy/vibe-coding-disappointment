package layout

import (
	"fmt"
	"reflect"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/vivalchemy/wake/internal/tui/theme"
	"github.com/vivalchemy/wake/pkg/logger"
)

// SplitLayout manages advanced split layouts with multiple panes
type SplitLayout struct {
	// Layout configuration
	panes       []*Pane
	orientation SplitOrientation
	width       int
	height      int

	// Split configuration
	minPaneSize  int
	maxPanes     int
	splitterSize int

	// Interaction state
	resizing     bool
	resizingPane int
	lastMouseX   int
	lastMouseY   int

	// Styles
	styles *SplitStyles

	// Logger
	logger logger.Logger
}

// SplitOrientation represents the orientation of the split
type SplitOrientation int

const (
	OrientationVertical SplitOrientation = iota
	OrientationHorizontal
)

// String returns the string representation of SplitOrientation
func (o SplitOrientation) String() string {
	switch o {
	case OrientationVertical:
		return "vertical"
	case OrientationHorizontal:
		return "horizontal"
	default:
		return "vertical"
	}
}

// Pane represents a single pane in the split layout
type Pane struct {
	// Identification
	ID    string
	Title string

	// Size and position
	X       int
	Y       int
	Width   int
	Height  int
	MinSize int
	MaxSize int

	// Size configuration
	Size  float64 // Proportional size (0.0 - 1.0)
	Fixed bool    // Whether the pane has a fixed size

	// State
	Visible bool
	Focused bool
	Content string

	// Component reference
	Component tea.Model

	// Styling
	Style lipgloss.Style
}

// SplitStyles defines the visual styles for split layouts
type SplitStyles struct {
	Pane          lipgloss.Style
	PaneFocused   lipgloss.Style
	PaneTitle     lipgloss.Style
	Splitter      lipgloss.Style
	SplitterHover lipgloss.Style
	ResizeHandle  lipgloss.Style
}

// NewSplitLayout creates a new split layout
func NewSplitLayout(orientation SplitOrientation, logger logger.Logger, themeProvider *theme.Theme) *SplitLayout {
	styles := createSplitStyles(themeProvider)

	return &SplitLayout{
		panes:        make([]*Pane, 0),
		orientation:  orientation,
		width:        0,
		height:       0,
		minPaneSize:  10,
		maxPanes:     4,
		splitterSize: 1,
		resizing:     false,
		resizingPane: -1,
		styles:       styles,
		logger:       logger.WithGroup("split-layout"),
	}
}

// AddPane adds a new pane to the layout
func (sl *SplitLayout) AddPane(pane *Pane) error {
	if len(sl.panes) >= sl.maxPanes {
		return fmt.Errorf("maximum number of panes (%d) reached", sl.maxPanes)
	}

	if pane.ID == "" {
		pane.ID = fmt.Sprintf("pane-%d", len(sl.panes))
	}

	if pane.Size == 0 {
		// Distribute size evenly among all panes
		pane.Size = 1.0 / float64(len(sl.panes)+1)

		// Adjust existing panes
		for _, existingPane := range sl.panes {
			if !existingPane.Fixed {
				existingPane.Size = 1.0 / float64(len(sl.panes)+1)
			}
		}
	}

	if pane.MinSize == 0 {
		pane.MinSize = sl.minPaneSize
	}

	pane.Visible = true
	sl.panes = append(sl.panes, pane)

	sl.logger.Debug("Added pane", "id", pane.ID, "size", pane.Size)
	sl.recalculateLayout()

	return nil
}

// RemovePane removes a pane from the layout
func (sl *SplitLayout) RemovePane(paneID string) error {
	paneIndex := -1
	for i, pane := range sl.panes {
		if pane.ID == paneID {
			paneIndex = i
			break
		}
	}

	if paneIndex == -1 {
		return fmt.Errorf("pane not found: %s", paneID)
	}

	removedPane := sl.panes[paneIndex]
	sl.panes = append(sl.panes[:paneIndex], sl.panes[paneIndex+1:]...)

	// Redistribute the removed pane's size
	if !removedPane.Fixed && len(sl.panes) > 0 {
		sizePerPane := removedPane.Size / float64(len(sl.panes))
		for _, pane := range sl.panes {
			if !pane.Fixed {
				pane.Size += sizePerPane
			}
		}
	}

	sl.logger.Debug("Removed pane", "id", paneID)
	sl.recalculateLayout()

	return nil
}

// GetPane returns a pane by ID
func (sl *SplitLayout) GetPane(paneID string) *Pane {
	for _, pane := range sl.panes {
		if pane.ID == paneID {
			return pane
		}
	}
	return nil
}

// SetPaneSize sets the size of a pane
func (sl *SplitLayout) SetPaneSize(paneID string, size float64) error {
	pane := sl.GetPane(paneID)
	if pane == nil {
		return fmt.Errorf("pane not found: %s", paneID)
	}

	if pane.Fixed {
		return fmt.Errorf("cannot resize fixed pane: %s", paneID)
	}

	// Ensure size is within bounds
	if size < 0.1 {
		size = 0.1
	} else if size > 0.8 {
		size = 0.8
	}

	oldSize := pane.Size
	pane.Size = size

	// Adjust other non-fixed panes
	sizeDiff := size - oldSize
	adjustablePanes := make([]*Pane, 0)

	for _, p := range sl.panes {
		if p.ID != paneID && !p.Fixed {
			adjustablePanes = append(adjustablePanes, p)
		}
	}

	if len(adjustablePanes) > 0 {
		adjustmentPerPane := -sizeDiff / float64(len(adjustablePanes))
		for _, p := range adjustablePanes {
			p.Size += adjustmentPerPane
			if p.Size < 0.1 {
				p.Size = 0.1
			}
		}
	}

	sl.recalculateLayout()
	return nil
}

// SetOrientation sets the split orientation
func (sl *SplitLayout) SetOrientation(orientation SplitOrientation) {
	sl.orientation = orientation
	sl.recalculateLayout()
}

// SetSize sets the overall size of the split layout
func (sl *SplitLayout) SetSize(width, height int) {
	sl.width = width
	sl.height = height
	sl.recalculateLayout()
}

// recalculateLayout recalculates the layout of all panes
func (sl *SplitLayout) recalculateLayout() {
	if len(sl.panes) == 0 || sl.width == 0 || sl.height == 0 {
		return
	}

	// Normalize sizes to ensure they sum to 1.0
	sl.normalizeSizes()

	switch sl.orientation {
	case OrientationVertical:
		sl.calculateVerticalLayout()
	case OrientationHorizontal:
		sl.calculateHorizontalLayout()
	}

	sl.logger.Debug("Recalculated layout",
		"orientation", sl.orientation.String(),
		"panes", len(sl.panes),
		"width", sl.width,
		"height", sl.height)
}

// normalizeSizes ensures all pane sizes sum to 1.0
func (sl *SplitLayout) normalizeSizes() {
	totalSize := 0.0
	for _, pane := range sl.panes {
		if pane.Visible {
			totalSize += pane.Size
		}
	}

	if totalSize == 0 {
		return
	}

	// Normalize sizes
	for _, pane := range sl.panes {
		if pane.Visible {
			pane.Size = pane.Size / totalSize
		}
	}
}

// calculateVerticalLayout calculates layout for vertical orientation
func (sl *SplitLayout) calculateVerticalLayout() {
	availableWidth := sl.width - (len(sl.panes)-1)*sl.splitterSize
	currentX := 0

	for _, pane := range sl.panes {
		if !pane.Visible {
			continue
		}

		// Calculate width
		paneWidth := int(float64(availableWidth) * pane.Size)

		// Ensure minimum size
		if paneWidth < pane.MinSize {
			paneWidth = pane.MinSize
		}

		// Ensure maximum size
		if pane.MaxSize > 0 && paneWidth > pane.MaxSize {
			paneWidth = pane.MaxSize
		}

		// Set pane position and size
		pane.X = currentX
		pane.Y = 0
		pane.Width = paneWidth
		pane.Height = sl.height

		currentX += paneWidth + sl.splitterSize

		sl.logger.Debug("Calculated vertical pane layout",
			"pane", pane.ID,
			"x", pane.X,
			"y", pane.Y,
			"width", pane.Width,
			"height", pane.Height)
	}
}

// calculateHorizontalLayout calculates layout for horizontal orientation
func (sl *SplitLayout) calculateHorizontalLayout() {
	availableHeight := sl.height - (len(sl.panes)-1)*sl.splitterSize
	currentY := 0

	for _, pane := range sl.panes {
		if !pane.Visible {
			continue
		}

		// Calculate height
		paneHeight := int(float64(availableHeight) * pane.Size)

		// Ensure minimum size
		if paneHeight < pane.MinSize {
			paneHeight = pane.MinSize
		}

		// Ensure maximum size
		if pane.MaxSize > 0 && paneHeight > pane.MaxSize {
			paneHeight = pane.MaxSize
		}

		// Set pane position and size
		pane.X = 0
		pane.Y = currentY
		pane.Width = sl.width
		pane.Height = paneHeight

		currentY += paneHeight + sl.splitterSize

		sl.logger.Debug("Calculated horizontal pane layout",
			"pane", pane.ID,
			"x", pane.X,
			"y", pane.Y,
			"width", pane.Width,
			"height", pane.Height)
	}
}

// Render renders the split layout
func (sl *SplitLayout) Render() string {
	if len(sl.panes) == 0 {
		return ""
	}

	var content string

	switch sl.orientation {
	case OrientationVertical:
		content = sl.renderVertical()
	case OrientationHorizontal:
		content = sl.renderHorizontal()
	}

	return content
}

// renderVertical renders the vertical layout
func (sl *SplitLayout) renderVertical() string {
	var paneViews []string

	for i, pane := range sl.panes {
		if !pane.Visible {
			continue
		}

		// Render pane content
		paneView := sl.renderPane(pane)
		paneViews = append(paneViews, paneView)

		// Add splitter if not the last pane
		if i < len(sl.panes)-1 {
			splitter := sl.renderVerticalSplitter()
			paneViews = append(paneViews, splitter)
		}
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, paneViews...)
}

// renderHorizontal renders the horizontal layout
func (sl *SplitLayout) renderHorizontal() string {
	var paneViews []string

	for i, pane := range sl.panes {
		if !pane.Visible {
			continue
		}

		// Render pane content
		paneView := sl.renderPane(pane)
		paneViews = append(paneViews, paneView)

		// Add splitter if not the last pane
		if i < len(sl.panes)-1 {
			splitter := sl.renderHorizontalSplitter()
			paneViews = append(paneViews, splitter)
		}
	}

	return lipgloss.JoinVertical(lipgloss.Left, paneViews...)
}

// renderPane renders a single pane
func (sl *SplitLayout) renderPane(pane *Pane) string {
	var style lipgloss.Style

	if pane.Focused {
		style = sl.styles.PaneFocused
	} else {
		style = sl.styles.Pane
	}

	// Apply custom pane style if set
	if !reflect.DeepEqual(pane.Style, lipgloss.Style{}) {
		style = pane.Style
	}

	var content string

	// Render component if available
	if pane.Component != nil {
		content = pane.Component.View()
	} else {
		content = pane.Content
	}

	// Add title if present
	if pane.Title != "" {
		title := sl.styles.PaneTitle.Render(pane.Title)
		content = title + "\n" + content
	}

	return style.
		Width(pane.Width).
		Height(pane.Height).
		Render(content)
}

// renderVerticalSplitter renders a vertical splitter
func (sl *SplitLayout) renderVerticalSplitter() string {
	splitterContent := ""
	for i := 0; i < sl.height; i++ {
		splitterContent += "│\n"
	}

	return sl.styles.Splitter.
		Width(sl.splitterSize).
		Height(sl.height).
		Render(splitterContent)
}

// renderHorizontalSplitter renders a horizontal splitter
func (sl *SplitLayout) renderHorizontalSplitter() string {
	splitterContent := lipgloss.PlaceHorizontal(
		sl.width,
		lipgloss.Center,
		"─",
	)

	return sl.styles.Splitter.
		Width(sl.width).
		Height(sl.splitterSize).
		Render(splitterContent)
}

// HandleMouse handles mouse events for resizing
func (sl *SplitLayout) HandleMouse(msg tea.MouseMsg) tea.Cmd {
	switch {
	case msg.Button == tea.MouseButtonLeft:
		// Check if clicking on a splitter
		splitterIndex := sl.getSplitterIndex(msg.X, msg.Y)
		if splitterIndex >= 0 {
			sl.resizing = true
			sl.resizingPane = splitterIndex
			sl.lastMouseX = msg.X
			sl.lastMouseY = msg.Y
		}

	case msg.Action == tea.MouseActionRelease:
		sl.resizing = false
		sl.resizingPane = -1

	case msg.Action == tea.MouseActionMotion:
		if sl.resizing && sl.resizingPane >= 0 {
			return sl.handleResize(msg.X, msg.Y)
		}
	}

	return nil
}

// getSplitterIndex returns the index of the splitter at the given coordinates
func (sl *SplitLayout) getSplitterIndex(x, y int) int {
	if sl.orientation == OrientationVertical {
		currentX := 0
		for i, pane := range sl.panes {
			if !pane.Visible {
				continue
			}

			currentX += pane.Width

			// Check if clicking on splitter
			if i < len(sl.panes)-1 && x >= currentX && x < currentX+sl.splitterSize {
				return i
			}

			currentX += sl.splitterSize
		}
	} else {
		currentY := 0
		for i, pane := range sl.panes {
			if !pane.Visible {
				continue
			}

			currentY += pane.Height

			// Check if clicking on splitter
			if i < len(sl.panes)-1 && y >= currentY && y < currentY+sl.splitterSize {
				return i
			}

			currentY += sl.splitterSize
		}
	}

	return -1
}

// handleResize handles pane resizing
func (sl *SplitLayout) handleResize(x, y int) tea.Cmd {
	if sl.resizingPane < 0 || sl.resizingPane >= len(sl.panes)-1 {
		return nil
	}

	leftPane := sl.panes[sl.resizingPane]
	rightPane := sl.panes[sl.resizingPane+1]

	var delta int
	var totalSize int

	if sl.orientation == OrientationVertical {
		delta = x - sl.lastMouseX
		totalSize = sl.width - (len(sl.panes)-1)*sl.splitterSize
		sl.lastMouseX = x
	} else {
		delta = y - sl.lastMouseY
		totalSize = sl.height - (len(sl.panes)-1)*sl.splitterSize
		sl.lastMouseY = y
	}

	if delta == 0 {
		return nil
	}

	// Calculate size changes
	deltaRatio := float64(delta) / float64(totalSize)

	// Check bounds
	newLeftSize := leftPane.Size + deltaRatio
	newRightSize := rightPane.Size - deltaRatio

	minLeftRatio := float64(leftPane.MinSize) / float64(totalSize)
	minRightRatio := float64(rightPane.MinSize) / float64(totalSize)

	if newLeftSize < minLeftRatio {
		newLeftSize = minLeftRatio
		newRightSize = rightPane.Size + (leftPane.Size - newLeftSize)
	}

	if newRightSize < minRightRatio {
		newRightSize = minRightRatio
		newLeftSize = leftPane.Size + (rightPane.Size - newRightSize)
	}

	// Apply changes
	leftPane.Size = newLeftSize
	rightPane.Size = newRightSize

	sl.recalculateLayout()

	return nil
}

// FocusPane sets focus to a specific pane
func (sl *SplitLayout) FocusPane(paneID string) error {
	found := false
	for _, pane := range sl.panes {
		if pane.ID == paneID {
			pane.Focused = true
			found = true
		} else {
			pane.Focused = false
		}
	}

	if !found {
		return fmt.Errorf("pane not found: %s", paneID)
	}

	return nil
}

// GetFocusedPane returns the currently focused pane
func (sl *SplitLayout) GetFocusedPane() *Pane {
	for _, pane := range sl.panes {
		if pane.Focused {
			return pane
		}
	}
	return nil
}

// GetVisiblePanes returns all visible panes
func (sl *SplitLayout) GetVisiblePanes() []*Pane {
	var visible []*Pane
	for _, pane := range sl.panes {
		if pane.Visible {
			visible = append(visible, pane)
		}
	}
	return visible
}

// GetPaneCount returns the total number of panes
func (sl *SplitLayout) GetPaneCount() int {
	return len(sl.panes)
}

// GetVisiblePaneCount returns the number of visible panes
func (sl *SplitLayout) GetVisiblePaneCount() int {
	count := 0
	for _, pane := range sl.panes {
		if pane.Visible {
			count++
		}
	}
	return count
}

// TogglePaneVisibility toggles visibility of a pane
func (sl *SplitLayout) TogglePaneVisibility(paneID string) error {
	pane := sl.GetPane(paneID)
	if pane == nil {
		return fmt.Errorf("pane not found: %s", paneID)
	}

	pane.Visible = !pane.Visible
	sl.recalculateLayout()

	return nil
}

// createSplitStyles creates the styles for split layouts
func createSplitStyles(themeProvider *theme.Theme) *SplitStyles {
	colors := themeProvider.Colors()

	return &SplitStyles{
		Pane: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colors.Border).
			Padding(1),

		PaneFocused: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colors.Primary).
			Padding(1),

		PaneTitle: lipgloss.NewStyle().
			Bold(true).
			Foreground(colors.Primary).
			Padding(0, 1).
			MarginBottom(1),

		Splitter: lipgloss.NewStyle().
			Foreground(colors.Border),

		SplitterHover: lipgloss.NewStyle().
			Foreground(colors.Primary),

		ResizeHandle: lipgloss.NewStyle().
			Background(colors.Primary).
			Foreground(colors.TextInverted),
	}
}
