package view

import (
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"

	"sentinel2-uploader/internal/ui/headless/health"
	"sentinel2-uploader/internal/ui/headless/theme"
)

const (
	statusIdle = iota
	statusConnecting
	statusConnected
	statusStopping
	statusError
)

const (
	minComponentWidth = 1
	scrollbarMinThumb = 0
)

func RenderTabs(activeTab int, hoverZone string) string {
	overview := theme.TabInactiveStyle.Render(" Overview ")
	settings := theme.TabInactiveStyle.Render(" Settings ")
	if hoverZone == zoneTabOverview {
		overview = theme.TabHoverStyle.Render(" Overview ")
	}
	if hoverZone == zoneTabSettings {
		settings = theme.TabHoverStyle.Render(" Settings ")
	}
	if activeTab == TabOverview {
		overview = theme.TabActiveStyle.Render(" Overview ")
	}
	if activeTab == TabSettings {
		settings = theme.TabActiveStyle.Render(" Settings ")
	}

	overview = zone.Mark(zoneTabOverview, overview)
	settings = zone.Mark(zoneTabSettings, settings)

	return lipgloss.JoinHorizontal(lipgloss.Bottom, overview, settings)
}

func RenderStatus(status string, kind int) string {
	switch kind {
	case statusConnected:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(status)
	case statusConnecting:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("226")).Render(status)
	case statusStopping:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("214")).Render(status)
	case statusError:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(status)
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(status)
	}
}

func RenderActionsRow(segments []string, maxWidth int) string {
	if maxWidth <= 0 {
		maxWidth = minComponentWidth
	}
	lines := make([]string, 0, len(segments))
	rowParts := make([]string, 0, len(segments))
	joinRow := func(parts []string) string {
		if len(parts) == 0 {
			return ""
		}
		row := parts[0]
		for i := 1; i < len(parts); i++ {
			row = lipgloss.JoinHorizontal(lipgloss.Top, row, " ", parts[i])
		}
		return row
	}
	for _, seg := range segments {
		if len(rowParts) == 0 {
			rowParts = append(rowParts, seg)
			continue
		}
		candidateParts := append(append([]string(nil), rowParts...), seg)
		candidate := joinRow(candidateParts)
		if lipgloss.Width(candidate) <= maxWidth {
			rowParts = candidateParts
			continue
		}
		lines = append(lines, joinRow(rowParts))
		rowParts = []string{seg}
	}
	if len(rowParts) > 0 {
		lines = append(lines, joinRow(rowParts))
	}
	return strings.Join(lines, "\n")
}

func ChannelDotStyle(kind health.Kind) (string, lipgloss.Style) {
	dot := "●"
	switch kind {
	case health.Active:
		return dot, lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
	case health.Warn:
		return dot, lipgloss.NewStyle().Foreground(lipgloss.Color("226"))
	case health.Stale:
		return dot, lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
	default:
		return dot, lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	}
}

func RainbowText(value string, phase int) string {
	var b strings.Builder
	for i, r := range value {
		position := float64(i)/2.0 - float64(phase)*0.4
		style := lipgloss.NewStyle().Foreground(lipgloss.Color(theme.RainbowColorAt(position)))
		b.WriteString(style.Render(string(r)))
	}
	return b.String()
}

func RainbowTitle(value string, phase int, animated bool) string {
	runes := []rune(value)
	if len(runes) == 0 {
		return value
	}
	parts := make([]string, 0, len(runes))
	span := theme.RainbowSpan()
	phaseF := float64(phase)
	for i := range runes {
		t := float64(i) / float64(max(len(runes)-1, 1))
		x := t * span
		if animated {
			// Smooth visible wave: a global drift plus a per-character sinusoidal ripple.
			x += -phaseF*0.14 + math.Sin((float64(i)*0.42)+(phaseF*0.12))*0.85
		}
		color := theme.RainbowColorAt(x)
		parts = append(parts, lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(color)).Render(string(runes[i])))
	}
	return strings.Join(parts, "")
}

func WithScrollBar(content string, width int, height int, percent float64) string {
	if height <= 0 {
		return content
	}
	width = max(width, minComponentWidth)
	lines := strings.Split(content, "\n")
	if len(lines) < height {
		pad := make([]string, 0, height-len(lines))
		for range height - len(lines) {
			pad = append(pad, "")
		}
		lines = append(lines, pad...)
	}
	if len(lines) > height {
		lines = lines[:height]
	}

	thumb := int(percent * float64(height-1))
	thumb = max(thumb, scrollbarMinThumb)
	if thumb >= height {
		thumb = height - 1
	}
	barInactive := lipgloss.NewStyle().Foreground(lipgloss.Color("238")).Render("┊")
	barActive := lipgloss.NewStyle().Foreground(lipgloss.Color("250")).Render("▯")

	out := make([]string, 0, height)
	for i := range height {
		bar := barInactive
		if i == thumb {
			bar = barActive
		}
		text := ansi.Cut(lines[i], 0, width)
		if pad := width - ansi.StringWidth(text); pad > 0 {
			text += strings.Repeat(" ", pad)
		}
		out = append(out, text+" "+bar)
	}
	return strings.Join(out, "\n")
}
