package theme

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ColorPalette defines a set of related colors
type ColorPalette struct {
	Name        string
	Description string
	Colors      map[string]lipgloss.Color
}

// GradientColors represents a color gradient
type GradientColors struct {
	Start lipgloss.Color
	End   lipgloss.Color
	Steps int
}

// ColorUtils provides utility functions for color manipulation
type ColorUtils struct{}

// Predefined color palettes
var (
	// Material Design colors
	MaterialPalette = &ColorPalette{
		Name:        "material",
		Description: "Material Design color palette",
		Colors: map[string]lipgloss.Color{
			"red50":     lipgloss.Color("#FFEBEE"),
			"red100":    lipgloss.Color("#FFCDD2"),
			"red500":    lipgloss.Color("#F44336"),
			"red900":    lipgloss.Color("#B71C1C"),
			"blue50":    lipgloss.Color("#E3F2FD"),
			"blue100":   lipgloss.Color("#BBDEFB"),
			"blue500":   lipgloss.Color("#2196F3"),
			"blue900":   lipgloss.Color("#0D47A1"),
			"green50":   lipgloss.Color("#E8F5E8"),
			"green100":  lipgloss.Color("#C8E6C9"),
			"green500":  lipgloss.Color("#4CAF50"),
			"green900":  lipgloss.Color("#1B5E20"),
			"yellow50":  lipgloss.Color("#FFFDE7"),
			"yellow100": lipgloss.Color("#FFF9C4"),
			"yellow500": lipgloss.Color("#FFEB3B"),
			"yellow900": lipgloss.Color("#F57F17"),
			"purple50":  lipgloss.Color("#F3E5F5"),
			"purple100": lipgloss.Color("#E1BEE7"),
			"purple500": lipgloss.Color("#9C27B0"),
			"purple900": lipgloss.Color("#4A148C"),
		},
	}

	// Solarized color palette
	SolarizedPalette = &ColorPalette{
		Name:        "solarized",
		Description: "Solarized color scheme",
		Colors: map[string]lipgloss.Color{
			"base03":  lipgloss.Color("#002b36"),
			"base02":  lipgloss.Color("#073642"),
			"base01":  lipgloss.Color("#586e75"),
			"base00":  lipgloss.Color("#657b83"),
			"base0":   lipgloss.Color("#839496"),
			"base1":   lipgloss.Color("#93a1a1"),
			"base2":   lipgloss.Color("#eee8d5"),
			"base3":   lipgloss.Color("#fdf6e3"),
			"yellow":  lipgloss.Color("#b58900"),
			"orange":  lipgloss.Color("#cb4b16"),
			"red":     lipgloss.Color("#dc322f"),
			"magenta": lipgloss.Color("#d33682"),
			"violet":  lipgloss.Color("#6c71c4"),
			"blue":    lipgloss.Color("#268bd2"),
			"cyan":    lipgloss.Color("#2aa198"),
			"green":   lipgloss.Color("#859900"),
		},
	}

	// Nord color palette
	NordPalette = &ColorPalette{
		Name:        "nord",
		Description: "Nord color scheme",
		Colors: map[string]lipgloss.Color{
			"nord0":  lipgloss.Color("#2e3440"),
			"nord1":  lipgloss.Color("#3b4252"),
			"nord2":  lipgloss.Color("#434c5e"),
			"nord3":  lipgloss.Color("#4c566a"),
			"nord4":  lipgloss.Color("#d8dee9"),
			"nord5":  lipgloss.Color("#e5e9f0"),
			"nord6":  lipgloss.Color("#eceff4"),
			"nord7":  lipgloss.Color("#8fbcbb"),
			"nord8":  lipgloss.Color("#88c0d0"),
			"nord9":  lipgloss.Color("#81a1c1"),
			"nord10": lipgloss.Color("#5e81ac"),
			"nord11": lipgloss.Color("#bf616a"),
			"nord12": lipgloss.Color("#d08770"),
			"nord13": lipgloss.Color("#ebcb8b"),
			"nord14": lipgloss.Color("#a3be8c"),
			"nord15": lipgloss.Color("#b48ead"),
		},
	}

	// Dracula color palette
	DraculaPalette = &ColorPalette{
		Name:        "dracula",
		Description: "Dracula color scheme",
		Colors: map[string]lipgloss.Color{
			"background": lipgloss.Color("#282a36"),
			"current":    lipgloss.Color("#44475a"),
			"selection":  lipgloss.Color("#44475a"),
			"foreground": lipgloss.Color("#f8f8f2"),
			"comment":    lipgloss.Color("#6272a4"),
			"cyan":       lipgloss.Color("#8be9fd"),
			"green":      lipgloss.Color("#50fa7b"),
			"orange":     lipgloss.Color("#ffb86c"),
			"pink":       lipgloss.Color("#ff79c6"),
			"purple":     lipgloss.Color("#bd93f9"),
			"red":        lipgloss.Color("#ff5555"),
			"yellow":     lipgloss.Color("#f1fa8c"),
		},
	}

	// Monokai color palette
	MonokaiPalette = &ColorPalette{
		Name:        "monokai",
		Description: "Monokai color scheme",
		Colors: map[string]lipgloss.Color{
			"background": lipgloss.Color("#272822"),
			"foreground": lipgloss.Color("#f8f8f2"),
			"comment":    lipgloss.Color("#75715e"),
			"red":        lipgloss.Color("#f92672"),
			"orange":     lipgloss.Color("#fd971f"),
			"yellow":     lipgloss.Color("#e6db74"),
			"green":      lipgloss.Color("#a6e22e"),
			"blue":       lipgloss.Color("#66d9ef"),
			"purple":     lipgloss.Color("#ae81ff"),
		},
	}
)

// NewColorUtils creates a new color utilities instance
func NewColorUtils() *ColorUtils {
	return &ColorUtils{}
}

// HexToColor converts a hex string to a lipgloss.Color
func (cu *ColorUtils) HexToColor(hex string) (lipgloss.Color, error) {
	// Remove # prefix if present
	hex = strings.TrimPrefix(hex, "#")

	// Validate hex string
	if len(hex) != 6 && len(hex) != 3 {
		return "", fmt.Errorf("invalid hex color format: %s", hex)
	}

	// Expand 3-digit hex to 6-digit
	if len(hex) == 3 {
		hex = string(hex[0]) + string(hex[0]) +
			string(hex[1]) + string(hex[1]) +
			string(hex[2]) + string(hex[2])
	}

	// Validate hex characters
	for _, char := range hex {
		if !((char >= '0' && char <= '9') ||
			(char >= 'a' && char <= 'f') ||
			(char >= 'A' && char <= 'F')) {
			return "", fmt.Errorf("invalid hex character: %c", char)
		}
	}

	return lipgloss.Color("#" + hex), nil
}

// RGBToColor converts RGB values to a lipgloss.Color
func (cu *ColorUtils) RGBToColor(r, g, b int) (lipgloss.Color, error) {
	// Validate RGB values
	if r < 0 || r > 255 || g < 0 || g > 255 || b < 0 || b > 255 {
		return "", fmt.Errorf("RGB values must be between 0 and 255")
	}

	hex := fmt.Sprintf("#%02X%02X%02X", r, g, b)
	return lipgloss.Color(hex), nil
}

// ColorToHex converts a lipgloss.Color to a hex string
func (cu *ColorUtils) ColorToHex(color lipgloss.Color) string {
	colorStr := string(color)

	// If it's already a hex color, return it
	if strings.HasPrefix(colorStr, "#") && len(colorStr) == 7 {
		return colorStr
	}

	// If it's an ANSI color, convert to approximate hex
	return cu.ansiToHex(colorStr)
}

// ansiToHex converts ANSI color codes to approximate hex values
func (cu *ColorUtils) ansiToHex(ansi string) string {
	ansiToHexMap := map[string]string{
		"0":  "#000000", // Black
		"1":  "#800000", // Dark Red
		"2":  "#008000", // Dark Green
		"3":  "#808000", // Dark Yellow
		"4":  "#000080", // Dark Blue
		"5":  "#800080", // Dark Magenta
		"6":  "#008080", // Dark Cyan
		"7":  "#c0c0c0", // Light Gray
		"8":  "#808080", // Dark Gray
		"9":  "#ff0000", // Red
		"10": "#00ff00", // Green
		"11": "#ffff00", // Yellow
		"12": "#0000ff", // Blue
		"13": "#ff00ff", // Magenta
		"14": "#00ffff", // Cyan
		"15": "#ffffff", // White
	}

	if hex, exists := ansiToHexMap[ansi]; exists {
		return hex
	}

	// Default to white for unknown colors
	return "#ffffff"
}

// Lighten lightens a color by a percentage (0.0 to 1.0)
func (cu *ColorUtils) Lighten(color lipgloss.Color, amount float64) lipgloss.Color {
	r, g, b := cu.hexToRGB(cu.ColorToHex(color))

	// Lighten by moving towards white
	r = int(float64(r) + (255-float64(r))*amount)
	g = int(float64(g) + (255-float64(g))*amount)
	b = int(float64(b) + (255-float64(b))*amount)

	// Ensure values stay within bounds
	if r > 255 {
		r = 255
	}
	if g > 255 {
		g = 255
	}
	if b > 255 {
		b = 255
	}

	result, _ := cu.RGBToColor(r, g, b)
	return result
}

// Darken darkens a color by a percentage (0.0 to 1.0)
func (cu *ColorUtils) Darken(color lipgloss.Color, amount float64) lipgloss.Color {
	r, g, b := cu.hexToRGB(cu.ColorToHex(color))

	// Darken by moving towards black
	r = int(float64(r) * (1.0 - amount))
	g = int(float64(g) * (1.0 - amount))
	b = int(float64(b) * (1.0 - amount))

	// Ensure values stay within bounds
	if r < 0 {
		r = 0
	}
	if g < 0 {
		g = 0
	}
	if b < 0 {
		b = 0
	}

	result, _ := cu.RGBToColor(r, g, b)
	return result
}

// Saturate increases color saturation by a percentage
func (cu *ColorUtils) Saturate(color lipgloss.Color, amount float64) lipgloss.Color {
	r, g, b := cu.hexToRGB(cu.ColorToHex(color))
	h, s, l := cu.rgbToHSL(r, g, b)

	// Increase saturation
	s += amount
	if s > 1.0 {
		s = 1.0
	}
	if s < 0.0 {
		s = 0.0
	}

	r, g, b = cu.hslToRGB(h, s, l)
	result, _ := cu.RGBToColor(r, g, b)
	return result
}

// Desaturate decreases color saturation by a percentage
func (cu *ColorUtils) Desaturate(color lipgloss.Color, amount float64) lipgloss.Color {
	return cu.Saturate(color, -amount)
}

// GetContrastColor returns black or white based on which provides better contrast
func (cu *ColorUtils) GetContrastColor(background lipgloss.Color) lipgloss.Color {
	r, g, b := cu.hexToRGB(cu.ColorToHex(background))

	// Calculate luminance using the relative luminance formula
	luminance := (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 255

	// Return white for dark backgrounds, black for light backgrounds
	if luminance > 0.5 {
		return lipgloss.Color("#000000")
	}
	return lipgloss.Color("#ffffff")
}

// GenerateGradient generates a gradient between two colors
func (cu *ColorUtils) GenerateGradient(start, end lipgloss.Color, steps int) []lipgloss.Color {
	if steps < 2 {
		return []lipgloss.Color{start, end}
	}

	startR, startG, startB := cu.hexToRGB(cu.ColorToHex(start))
	endR, endG, endB := cu.hexToRGB(cu.ColorToHex(end))

	gradient := make([]lipgloss.Color, steps)

	for i := 0; i < steps; i++ {
		ratio := float64(i) / float64(steps-1)

		r := int(float64(startR) + ratio*float64(endR-startR))
		g := int(float64(startG) + ratio*float64(endG-startG))
		b := int(float64(startB) + ratio*float64(endB-startB))

		color, _ := cu.RGBToColor(r, g, b)
		gradient[i] = color
	}

	return gradient
}

// GetColorFromPalette gets a color from a named palette
func (cu *ColorUtils) GetColorFromPalette(paletteName, colorName string) (lipgloss.Color, error) {
	var palette *ColorPalette

	switch paletteName {
	case "material":
		palette = MaterialPalette
	case "solarized":
		palette = SolarizedPalette
	case "nord":
		palette = NordPalette
	case "dracula":
		palette = DraculaPalette
	case "monokai":
		palette = MonokaiPalette
	default:
		return "", fmt.Errorf("unknown palette: %s", paletteName)
	}

	color, exists := palette.Colors[colorName]
	if !exists {
		return "", fmt.Errorf("color %s not found in palette %s", colorName, paletteName)
	}

	return color, nil
}

// AdaptColorForTerminal adapts a color for terminal capabilities
func (cu *ColorUtils) AdaptColorForTerminal(color lipgloss.Color, colorSupport ColorSupport) lipgloss.Color {
	switch colorSupport {
	case ColorSupportNone:
		// Convert to monochrome
		return cu.toMonochrome(color)
	case ColorSupport16:
		// Convert to nearest 16-color
		return cu.toANSI16(color)
	case ColorSupport256:
		// Convert to nearest 256-color
		return cu.toANSI256(color)
	case ColorSupportTrueColor:
		// Return as-is for true color
		return color
	default:
		return color
	}
}

// toMonochrome converts a color to monochrome
func (cu *ColorUtils) toMonochrome(color lipgloss.Color) lipgloss.Color {
	r, g, b := cu.hexToRGB(cu.ColorToHex(color))

	// Calculate grayscale value using luminance formula
	gray := int(0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b))

	// Convert to nearest terminal grayscale
	if gray < 128 {
		return lipgloss.Color("0") // Black
	}
	return lipgloss.Color("15") // White
}

// toANSI16 converts a color to the nearest 16-color ANSI equivalent
func (cu *ColorUtils) toANSI16(color lipgloss.Color) lipgloss.Color {
	r, g, b := cu.hexToRGB(cu.ColorToHex(color))

	// Define 16-color ANSI palette
	ansi16Colors := []struct {
		code    string
		r, g, b int
	}{
		{"0", 0, 0, 0},        // Black
		{"1", 128, 0, 0},      // Dark Red
		{"2", 0, 128, 0},      // Dark Green
		{"3", 128, 128, 0},    // Dark Yellow
		{"4", 0, 0, 128},      // Dark Blue
		{"5", 128, 0, 128},    // Dark Magenta
		{"6", 0, 128, 128},    // Dark Cyan
		{"7", 192, 192, 192},  // Light Gray
		{"8", 128, 128, 128},  // Dark Gray
		{"9", 255, 0, 0},      // Red
		{"10", 0, 255, 0},     // Green
		{"11", 255, 255, 0},   // Yellow
		{"12", 0, 0, 255},     // Blue
		{"13", 255, 0, 255},   // Magenta
		{"14", 0, 255, 255},   // Cyan
		{"15", 255, 255, 255}, // White
	}

	// Find the closest color
	minDistance := float64(999999)
	closestCode := "7" // Default to light gray

	for _, ansiColor := range ansi16Colors {
		distance := cu.colorDistance(r, g, b, ansiColor.r, ansiColor.g, ansiColor.b)
		if distance < minDistance {
			minDistance = distance
			closestCode = ansiColor.code
		}
	}

	return lipgloss.Color(closestCode)
}

// toANSI256 converts a color to the nearest 256-color ANSI equivalent
func (cu *ColorUtils) toANSI256(color lipgloss.Color) lipgloss.Color {
	r, g, b := cu.hexToRGB(cu.ColorToHex(color))

	// For 256-color mode, we can use a simplified mapping
	// This is a basic implementation - a more sophisticated one would
	// map to the actual 256-color palette

	// Convert to 6x6x6 color cube (colors 16-231)
	r6 := (r * 5) / 255
	g6 := (g * 5) / 255
	b6 := (b * 5) / 255

	ansi256Code := 16 + (36 * r6) + (6 * g6) + b6

	return lipgloss.Color(strconv.Itoa(ansi256Code))
}

// colorDistance calculates the Euclidean distance between two RGB colors
func (cu *ColorUtils) colorDistance(r1, g1, b1, r2, g2, b2 int) float64 {
	dr := float64(r1 - r2)
	dg := float64(g1 - g2)
	db := float64(b1 - b2)

	return dr*dr + dg*dg + db*db
}

// hexToRGB converts a hex color to RGB values
func (cu *ColorUtils) hexToRGB(hex string) (int, int, int) {
	hex = strings.TrimPrefix(hex, "#")

	if len(hex) != 6 {
		return 255, 255, 255 // Default to white
	}

	r, _ := strconv.ParseInt(hex[0:2], 16, 64)
	g, _ := strconv.ParseInt(hex[2:4], 16, 64)
	b, _ := strconv.ParseInt(hex[4:6], 16, 64)

	return int(r), int(g), int(b)
}

// rgbToHSL converts RGB to HSL
func (cu *ColorUtils) rgbToHSL(r, g, b int) (float64, float64, float64) {
	fr := float64(r) / 255.0
	fg := float64(g) / 255.0
	fb := float64(b) / 255.0

	max := fr
	if fg > max {
		max = fg
	}
	if fb > max {
		max = fb
	}

	min := fr
	if fg < min {
		min = fg
	}
	if fb < min {
		min = fb
	}

	h := 0.0
	s := 0.0
	l := (max + min) / 2.0

	if max != min {
		d := max - min
		if l > 0.5 {
			s = d / (2.0 - max - min)
		} else {
			s = d / (max + min)
		}

		switch max {
		case fr:
			h = (fg - fb) / d
			if fg < fb {
				h += 6.0
			}
		case fg:
			h = (fb-fr)/d + 2.0
		case fb:
			h = (fr-fg)/d + 4.0
		}
		h /= 6.0
	}

	return h, s, l
}

// hslToRGB converts HSL to RGB
func (cu *ColorUtils) hslToRGB(h, s, l float64) (int, int, int) {
	var r, g, b float64

	if s == 0 {
		r = l
		g = l
		b = l
	} else {
		hue2rgb := func(p, q, t float64) float64 {
			if t < 0 {
				t += 1
			}
			if t > 1 {
				t -= 1
			}
			if t < 1.0/6.0 {
				return p + (q-p)*6.0*t
			}
			if t < 1.0/2.0 {
				return q
			}
			if t < 2.0/3.0 {
				return p + (q-p)*(2.0/3.0-t)*6.0
			}
			return p
		}

		var q float64
		if l < 0.5 {
			q = l * (1 + s)
		} else {
			q = l + s - l*s
		}
		p := 2*l - q

		r = hue2rgb(p, q, h+1.0/3.0)
		g = hue2rgb(p, q, h)
		b = hue2rgb(p, q, h-1.0/3.0)
	}

	return int(r * 255), int(g * 255), int(b * 255)
}

// GetAvailablePalettes returns a list of available color palettes
func GetAvailablePalettes() []string {
	return []string{"material", "solarized", "nord", "dracula", "monokai"}
}

// GetPaletteInfo returns information about a color palette
func GetPaletteInfo(paletteName string) (*ColorPalette, error) {
	switch paletteName {
	case "material":
		return MaterialPalette, nil
	case "solarized":
		return SolarizedPalette, nil
	case "nord":
		return NordPalette, nil
	case "dracula":
		return DraculaPalette, nil
	case "monokai":
		return MonokaiPalette, nil
	default:
		return nil, fmt.Errorf("unknown palette: %s", paletteName)
	}
}
