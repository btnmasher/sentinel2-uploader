package logging

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

func shouldPrettyPrint() bool {
	term := strings.TrimSpace(os.Getenv("TERM"))
	if term == "" || term == "dumb" {
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return true
}

func prettyPrint(level slog.Level, msg string, attrs []slog.Attr) {
	ts := time.Now().Format("15:04:05.000")
	levelLabel, levelStyle := levelBadge(level)
	header := lipgloss.JoinHorizontal(
		lipgloss.Center,
		lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(ts),
		" ",
		levelStyle.Render(levelLabel),
		" ",
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("252")).Render(msg),
	)

	fields := renderAttrs(level, attrs)
	if fields != "" {
		header = header + "\n" + fields
	}

	fmt.Fprintln(os.Stderr, header)
}

func levelBadge(level slog.Level) (string, lipgloss.Style) {
	base := lipgloss.NewStyle().Bold(true).Padding(0, 1)
	switch {
	case level <= slog.LevelDebug:
		return "DEBUG", base.Foreground(lipgloss.Color("255")).Background(lipgloss.Color("240"))
	case level <= slog.LevelInfo:
		return "INFO", base.Foreground(lipgloss.Color("230")).Background(lipgloss.Color("31"))
	case level <= slog.LevelWarn:
		return "WARN", base.Foreground(lipgloss.Color("234")).Background(lipgloss.Color("214"))
	default:
		return "ERROR", base.Foreground(lipgloss.Color("231")).Background(lipgloss.Color("160"))
	}
}

func renderAttrs(level slog.Level, attrs []slog.Attr) string {
	if len(attrs) == 0 {
		return ""
	}

	values := attrsToMap(attrs)
	if len(values) == 0 {
		return ""
	}

	keys := orderedFieldKeys(level, values)

	keyStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("117"))
	valStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("255"))
	sepStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))

	parts := make([]string, 0, len(keys))
	blocks := make([]string, 0, len(keys))
	for _, k := range keys {
		if pretty, ok := prettyJSONString(values[k]); ok {
			blocks = append(blocks, renderJSONFieldBlock(k, pretty))
			continue
		}
		parts = append(parts, keyStyle.Render(k)+sepStyle.Render("=")+valStyle.Render(formatFieldValue(values[k])))
	}
	lines := make([]string, 0, 1+len(blocks))
	if len(parts) > 0 {
		lines = append(lines, strings.Join(parts, " "))
	}
	lines = append(lines, blocks...)
	return lipgloss.NewStyle().MarginLeft(2).Render(strings.Join(lines, "\n"))
}
