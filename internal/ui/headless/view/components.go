package view

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

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

func RenderTabs(isOverview bool) string {
	overview := theme.TabInactiveStyle.Render("Overview")
	settings := theme.TabInactiveStyle.Render("Settings")
	if isOverview {
		overview = theme.TabActiveStyle.Render("Overview")
	} else {
		settings = theme.TabActiveStyle.Render("Settings")
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, overview, " ", settings)
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
