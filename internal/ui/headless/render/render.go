package render

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"sentinel2-uploader/internal/ui/headless/theme"
)

func Frame(content string, width int, rainbow bool, phase int, panelStyle lipgloss.Style) string {
	if !rainbow {
		innerWidth := width - 4 // rounded border (2) + horizontal padding (2)
		innerWidth = max(innerWidth, 1)
		return panelStyle.Width(innerWidth).Render(content)
	}
	return rainbowFrame(content, width, phase)
}

func rainbowFrame(content string, width int, phase int) string {
	lines := strings.Split(content, "\n")
	innerWidth := width - 2
	innerWidth = max(innerWidth, 20)
	clamp := lipgloss.NewStyle().MaxWidth(innerWidth).Width(innerWidth)

	top := colorizeBorderLine("╭", "─", "╮", innerWidth, 0, phase)
	bottom := colorizeBorderLine("╰", "─", "╯", innerWidth, len(lines)+1, phase)
	framed := make([]string, 0, len(lines)+2)
	framed = append(framed, top)
	for i, line := range lines {
		padded := clamp.Render(line)
		left := colorizeBorderChar("│", 0, i+1, phase)
		right := colorizeBorderChar("│", innerWidth+1, i+1, phase)
		framed = append(framed, left+padded+right)
	}
	framed = append(framed, bottom)
	return strings.Join(framed, "\n")
}

func colorizeBorderLine(left, fill, right string, width int, y int, phase int) string {
	var b strings.Builder
	b.WriteString(colorizeBorderChar(left, 0, y, phase))
	for x := 1; x <= width; x++ {
		b.WriteString(colorizeBorderChar(fill, x, y, phase))
	}
	b.WriteString(colorizeBorderChar(right, width+1, y, phase))
	return b.String()
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
