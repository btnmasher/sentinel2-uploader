package render

import (
	"strings"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"sentinel2-uploader/internal/ui/headless/theme"
)

func Frame(content string, width int, rainbow bool, phase int, panelStyle lipgloss.Style) string {
	innerWidth := width - panelStyle.GetHorizontalFrameSize()
	innerWidth = max(innerWidth, 1)
	framed := panelStyle.Width(innerWidth).Render(content)
	if !rainbow {
		return framed
	}
	return rainbowizeFrameBorders(framed, phase)
}

func rainbowizeFrameBorders(framed string, phase int) string {
	lines := strings.Split(framed, "\n")
	if len(lines) == 0 {
		return framed
	}
	out := make([]string, len(lines))
	last := len(lines) - 1
	for y, line := range lines {
		switch {
		case y == 0 || y == last:
			out[y] = colorizeHorizontalBorder(line, y, phase)
		default:
			out[y] = colorizeVerticalEdges(line, y, phase)
		}
	}
	return strings.Join(out, "\n")
}

func colorizeHorizontalBorder(line string, y int, phase int) string {
	var b strings.Builder
	x := 0
	for _, r := range line {
		ch := string(r)
		if isFrameBorderRune(r) {
			ch = colorizeBorderChar(ch, x, y, phase)
		}
		b.WriteString(ch)
		x++
	}
	return b.String()
}

func colorizeVerticalEdges(line string, y int, phase int) string {
	if line == "" {
		return line
	}
	leftRune, leftSize := utf8.DecodeRuneInString(line)
	if leftRune != '│' {
		return line
	}
	rightIdx := strings.LastIndex(line, "│")
	if rightIdx <= 0 {
		return line
	}
	rightX := ansi.StringWidth(line[:rightIdx])
	leftColored := colorizeBorderChar("│", 0, y, phase)
	rightColored := colorizeBorderChar("│", rightX, y, phase)
	return leftColored + line[leftSize:rightIdx] + rightColored
}

func isFrameBorderRune(r rune) bool {
	switch r {
	case '╭', '╮', '╰', '╯', '─', '│':
		return true
	default:
		return false
	}
}

func colorizeBorderChar(ch string, x int, y int, phase int) string {
	position := float64(x+y)/3.0 - float64(phase)*0.35
	return lipgloss.NewStyle().Foreground(lipgloss.Color(theme.RainbowColorAt(position))).Render(ch)
}

func TruncateDisplayWidth(value string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(value) <= width {
		return value
	}
	if width == 1 {
		return "…"
	}
	limit := width - ansi.StringWidth("…")
	limit = max(limit, 0)
	var b strings.Builder
	current := 0
	for _, r := range value {
		w := ansi.StringWidth(string(r))
		if current+w > limit {
			break
		}
		b.WriteRune(r)
		current += w
	}
	return b.String() + "…"
}
