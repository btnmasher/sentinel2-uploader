package theme

import (
	"fmt"
	"math"

	"github.com/charmbracelet/lipgloss"
)

var (
	PanelStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1)
	TitleStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("69"))
	FocusStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	ErrorStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true)
	HelpStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	TabActiveStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("27")).Padding(0, 1)
	TabInactiveStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Background(lipgloss.Color("236")).Padding(0, 1)
	ModalBackdrop    = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

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
	ButtonDisabledBaseStyle    = ButtonStyle.Border(DisabledButtonBorder).BorderForeground(DisabledBorderColor)
	ButtonDisabledStyle        = ButtonDisabledBaseStyle.Foreground(DisabledTextColor)
	ButtonDisabledFocusedStyle = ButtonStyle.BorderForeground(lipgloss.Color("255")).Foreground(lipgloss.Color("250"))
	SegmentBaseStyle           = lipgloss.NewStyle().Padding(0, 1)
	SegmentOnStyle             = SegmentBaseStyle.Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("10"))
	SegmentOffStyle            = SegmentBaseStyle.Foreground(lipgloss.Color("245")).Background(lipgloss.Color("236"))
)

type rgb struct {
	r uint8
	g uint8
	b uint8
}

var rainbowPalette = []rgb{
	{255, 0, 0},
	{255, 127, 0},
	{255, 255, 0},
	{0, 255, 0},
	{0, 180, 255},
	{75, 0, 130},
	{148, 0, 211},
}

func RainbowColorAt(position float64) string {
	n := float64(len(rainbowPalette))
	if n == 0 {
		return "#ffffff"
	}
	wrapped := math.Mod(position, n)
	if wrapped < 0 {
		wrapped += n
	}
	i0 := int(math.Floor(wrapped))
	i1 := (i0 + 1) % len(rainbowPalette)
	t := wrapped - float64(i0)
	c := lerpRGB(rainbowPalette[i0], rainbowPalette[i1], t)
	return fmt.Sprintf("#%02x%02x%02x", c.r, c.g, c.b)
}

func lerpRGB(a rgb, b rgb, t float64) rgb {
	return rgb{
		r: uint8(float64(a.r) + (float64(b.r)-float64(a.r))*t),
		g: uint8(float64(a.g) + (float64(b.g)-float64(a.g))*t),
		b: uint8(float64(a.b) + (float64(b.b)-float64(a.b))*t),
	}
}
