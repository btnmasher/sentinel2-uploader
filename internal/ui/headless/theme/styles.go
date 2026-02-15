package theme

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	PanelStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	TitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("69"))
	FocusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	ErrorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	HelpStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	TabActiveStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("230")).
			Background(lipgloss.Color("27")).
			Border(lipgloss.NormalBorder(), true, true, true, true).
			BorderForeground(lipgloss.Color("39"))
	TabInactiveStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("245")).
				Background(lipgloss.Color("236")).
				Border(lipgloss.NormalBorder(), true, true, true, true).
				BorderForeground(lipgloss.Color("240"))
	TabHoverStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("15")).
			Background(lipgloss.Color("236")).
			Border(lipgloss.NormalBorder(), true, true, true, true).
			BorderForeground(lipgloss.Color("15"))
	ModalBackdrop = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	DisabledButtonBorder = lipgloss.Border{
		Top:         "╌",
		Bottom:      "╌",
		Left:        "┊",
		Right:       "┊",
		TopLeft:     "┌",
		TopRight:    "┐",
		BottomLeft:  "└",
		BottomRight: "┘",
	}
	DisabledBorderColor = lipgloss.Color("240")
	DisabledTextColor   = lipgloss.Color("240")

	ButtonStyle                = lipgloss.NewStyle().Padding(0, 1).Border(lipgloss.NormalBorder())
	ButtonFocusedStyle         = ButtonStyle.BorderForeground(lipgloss.Color("10")).Foreground(lipgloss.Color("10"))
	ButtonHoverStyle           = ButtonStyle.BorderForeground(lipgloss.Color("15")).Foreground(lipgloss.Color("15"))
	ButtonDisabledBaseStyle    = ButtonStyle.Border(DisabledButtonBorder).BorderForeground(DisabledBorderColor)
	ButtonDisabledStyle        = ButtonDisabledBaseStyle.Foreground(DisabledTextColor)
	ButtonDisabledFocusedStyle = ButtonStyle.BorderForeground(lipgloss.Color("255")).Foreground(lipgloss.Color("250"))
	ButtonDisabledHoverStyle   = ButtonDisabledBaseStyle.BorderForeground(lipgloss.Color("255")).Foreground(lipgloss.Color("250"))
	SegmentBaseStyle           = lipgloss.NewStyle().Padding(0, 1)
	SegmentOnStyle             = SegmentBaseStyle.Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("10"))
	SegmentOffStyle            = SegmentBaseStyle.Foreground(lipgloss.Color("245")).Background(lipgloss.Color("236"))
)

var rainbowStops = []string{
	"#ff1f5a", "#ff8f1f", "#ffe44d", "#4ce06b", "#39d3ff", "#4f6bff", "#c45bff",
}

func RainbowSpan() float64 {
	return float64(max(len(rainbowStops)-1, 1))
}

func RainbowColorAt(position float64) string {
	n := float64(len(rainbowStops))
	if n == 0 {
		return "#ffffff"
	}
	wrapped := math.Mod(position, n)
	if wrapped < 0 {
		wrapped += n
	}
	i0 := int(math.Floor(wrapped))
	i1 := (i0 + 1) % len(rainbowStops)
	t := wrapped - float64(i0)
	return interpolateHex(rainbowStops[i0], rainbowStops[i1], t)
}

func interpolateHex(a string, b string, t float64) string {
	ar, ag, ab := parseHexRGB(a)
	br, bg, bb := parseHexRGB(b)
	lerp := func(x int, y int) int {
		return int(float64(x) + (float64(y)-float64(x))*t)
	}
	return fmt.Sprintf("#%02x%02x%02x", lerp(ar, br), lerp(ag, bg), lerp(ab, bb))
}

func parseHexRGB(s string) (int, int, int) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return 255, 255, 255
	}
	v, err := strconv.ParseUint(s, 16, 32)
	if err != nil {
		return 255, 255, 255
	}
	return int((v >> 16) & 0xff), int((v >> 8) & 0xff), int(v & 0xff)
}
