package logging

import (
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

var forceLipglossColorOnce sync.Once

func ensureLipglossColorOutput() {
	forceLipglossColorOnce.Do(func() {
		lipgloss.SetColorProfile(termenv.TrueColor)
	})
}

// FormatEventANSI renders a single log event using the same terminal styling
// conventions as pretty console output, preserving ANSI color sequences.
func FormatEventANSI(event Event) string {
	ensureLipglossColorOutput()
	ts := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(event.Time.Format("15:04:05.000"))
	levelLabel, levelStyle := levelBadge(event.Level)
	msg := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252")).Render(event.Message)

	line := lipgloss.JoinHorizontal(lipgloss.Center, ts, " ", levelStyle.Render(levelLabel), " ", msg)
	if len(event.Fields) == 0 {
		return line + "\n"
	}

	keys := orderedFieldKeys(event.Level, event.Fields)

	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
	valStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	sepStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	parts := make([]string, 0, len(keys))
	blocks := make([]string, 0, len(keys))
	for _, key := range keys {
		if pretty, ok := prettyJSONString(event.Fields[key]); ok {
			block := renderJSONFieldBlock(key, pretty)
			blocks = append(blocks, block)
			continue
		}
		parts = append(parts, keyStyle.Render(key)+sepStyle.Render("=")+valStyle.Render(formatFieldValue(event.Fields[key])))
	}
	if len(parts) > 0 {
		line += "  " + strings.Join(parts, " ")
	}
	if len(blocks) > 0 {
		for _, block := range blocks {
			line += "\n  " + block
		}
	}
	return line + "\n"
}

func renderJSONFieldBlock(key string, pretty string) string {
	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
	sepStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))
	header := keyStyle.Render(key) + sepStyle.Render("=")
	body := colorizePrettyJSON(pretty)
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("245")).
		Padding(0, 1).
		Render(body)
	return header + "\n" + box
}

func colorizePrettyJSON(pretty string) string {
	punct := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	field := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	lines := strings.Split(pretty, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, colorizeJSONLine(line, punct, field))
	}
	return strings.Join(out, "\n")
}

func colorizeJSONLine(line string, punct lipgloss.Style, field lipgloss.Style) string {
	var b strings.Builder
	inString := false
	escaped := false
	for _, r := range line {
		switch {
		case r == '"':
			b.WriteString(punct.Render(string(r)))
			if !escaped {
				inString = !inString
			}
			escaped = false
		case inString && r == '\\':
			b.WriteString(field.Render(string(r)))
			escaped = !escaped
		case !inString && (r == '{' || r == '}' || r == '[' || r == ']' || r == ':' || r == ','):
			b.WriteString(punct.Render(string(r)))
			escaped = false
		default:
			if r == ' ' || r == '\t' {
				b.WriteRune(r)
			} else {
				b.WriteString(field.Render(string(r)))
			}
			escaped = false
		}
	}
	return b.String()
}
