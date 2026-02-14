//go:build !headless

package gui

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"unicode/utf8"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
	"github.com/charmbracelet/x/ansi"
)

var (
	ansiDefaultFG  = color.NRGBA{R: 220, G: 220, B: 220, A: 255}
	ansiDefaultBG  = color.NRGBA{R: 0, G: 0, B: 0, A: 255}
	ansiStyleCache = map[string]*widget.CustomTextGridStyle{}
)

type ansiTextState struct {
	fg      color.NRGBA
	bg      color.NRGBA
	fgSet   bool
	bgSet   bool
	bold    bool
	dim     bool
	reverse bool
}

func splitLogLines(input string) []string {
	normalized := strings.ReplaceAll(input, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	// Log events already end with '\n'; avoid appending a synthetic blank row.
	if len(lines) > 1 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func stripANSIText(input string) string {
	return ansi.Strip(input)
}

func wrapANSILines(lines []string, columns int) []string {
	if columns <= 1 {
		return append([]string(nil), lines...)
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		wrapped := ansi.Wrap(line, columns, "")
		out = append(out, splitLogLines(wrapped)...)
	}
	return out
}

func parseANSITextGridRows(input string, columns int) []widget.TextGridRow {
	lines := wrapANSILines(splitLogLines(input), columns)
	out := make([]widget.TextGridRow, 0, len(lines))
	for _, line := range lines {
		out = append(out, parseANSITextGridRow(line))
	}
	return out
}

func parseANSITextGridRow(line string) widget.TextGridRow {
	row := widget.TextGridRow{Cells: make([]widget.TextGridCell, 0, len(line))}
	state := defaultANSIState()
	i := 0
	for i < len(line) {
		if line[i] == '\x1b' && i+1 < len(line) && line[i+1] == '[' {
			end := strings.IndexByte(line[i+2:], 'm')
			if end >= 0 {
				seq := line[i+2 : i+2+end]
				state = applyANSIStyle(seq, state)
				i += end + 3
				continue
			}
		}

		r, size := utf8.DecodeRuneInString(line[i:])
		if r == utf8.RuneError && size == 1 {
			r = rune(line[i])
		}
		row.Cells = append(row.Cells, widget.TextGridCell{
			Rune:  r,
			Style: ansiStyleFromState(state),
		})
		i += size
	}
	if len(row.Cells) == 0 {
		row.Cells = append(row.Cells, widget.TextGridCell{
			Rune:  ' ',
			Style: ansiStyleFromState(state),
		})
	}
	return row
}

func defaultANSIState() ansiTextState {
	return ansiTextState{}
}

func applyANSIStyle(seq string, current ansiTextState) ansiTextState {
	if seq == "" {
		return defaultANSIState()
	}
	seq = strings.ReplaceAll(seq, ":", ";")
	parts := strings.Split(seq, ";")
	state := current

	for i := 0; i < len(parts); i++ {
		code, err := strconv.Atoi(parts[i])
		if err != nil {
			continue
		}

		switch code {
		case 0:
			state = defaultANSIState()
		case 1:
			state.bold = true
		case 2:
			state.dim = true
		case 22:
			state.bold = false
			state.dim = false
		case 7:
			state.reverse = true
		case 27:
			state.reverse = false
		case 39:
			state.fgSet = false
		case 49:
			state.bgSet = false
		case 30, 31, 32, 33, 34, 35, 36, 37:
			state.fg = ansiBasicColor(code - 30)
			state.fgSet = true
		case 90, 91, 92, 93, 94, 95, 96, 97:
			state.fg = ansiBasicColor(code - 90 + 8)
			state.fgSet = true
		case 40, 41, 42, 43, 44, 45, 46, 47:
			state.bg = ansiBasicColor(code - 40)
			state.bgSet = true
		case 100, 101, 102, 103, 104, 105, 106, 107:
			state.bg = ansiBasicColor(code - 100 + 8)
			state.bgSet = true
		case 38:
			if c, consumed, ok := parseANSIExtendedColor(parts, i); ok {
				state.fg = c
				state.fgSet = true
				i += consumed
			}
		case 48:
			if c, consumed, ok := parseANSIExtendedColor(parts, i); ok {
				state.bg = c
				state.bgSet = true
				i += consumed
			}
		}
	}
	return state
}

func parseANSIExtendedColor(parts []string, start int) (color.NRGBA, int, bool) {
	// 38;5;<idx> / 48;5;<idx>
	if start+2 < len(parts) && parts[start+1] == "5" {
		idx, err := strconv.Atoi(parts[start+2])
		if err != nil {
			return color.NRGBA{}, 0, false
		}
		return ansi256ToColor(idx), 2, true
	}
	// 38;2;<r>;<g>;<b> / 48;2;<r>;<g>;<b>
	if start+4 < len(parts) && parts[start+1] == "2" {
		r, rErr := strconv.Atoi(parts[start+2])
		g, gErr := strconv.Atoi(parts[start+3])
		b, bErr := strconv.Atoi(parts[start+4])
		if rErr != nil || gErr != nil || bErr != nil {
			return color.NRGBA{}, 0, false
		}
		return color.NRGBA{
			R: uint8(clampColor(r)),
			G: uint8(clampColor(g)),
			B: uint8(clampColor(b)),
			A: 255,
		}, 4, true
	}
	return color.NRGBA{}, 0, false
}

func ansiStyleFromState(state ansiTextState) widget.TextGridStyle {
	fg := ansiDefaultFG
	bg := ansiDefaultBG
	if state.fgSet {
		fg = state.fg
	}
	if state.bgSet {
		bg = state.bg
	}
	if state.reverse {
		fg, bg = bg, fg
	}
	if state.dim {
		fg = dimColor(fg)
	}

	key := fmt.Sprintf(
		"%d:%d:%d:%d|%d:%d:%d:%d|b:%t|m:%t",
		fg.R, fg.G, fg.B, fg.A,
		bg.R, bg.G, bg.B, bg.A,
		state.bold, true,
	)
	if cached, ok := ansiStyleCache[key]; ok {
		return cached
	}

	style := &widget.CustomTextGridStyle{
		FGColor: fg,
		BGColor: bg,
		TextStyle: fyne.TextStyle{
			Bold:      state.bold,
			Monospace: true,
		},
	}
	ansiStyleCache[key] = style
	return style
}

func ansiBasicColor(index int) color.NRGBA {
	base := []color.NRGBA{
		{0, 0, 0, 255},
		{205, 49, 49, 255},
		{13, 188, 121, 255},
		{229, 229, 16, 255},
		{36, 114, 200, 255},
		{188, 63, 188, 255},
		{17, 168, 205, 255},
		{229, 229, 229, 255},
		{102, 102, 102, 255},
		{241, 76, 76, 255},
		{35, 209, 139, 255},
		{245, 245, 67, 255},
		{59, 142, 234, 255},
		{214, 112, 214, 255},
		{41, 184, 219, 255},
		{255, 255, 255, 255},
	}
	if index < 0 {
		index = 0
	}
	if index >= len(base) {
		index = len(base) - 1
	}
	return base[index]
}

func dimColor(c color.NRGBA) color.NRGBA {
	return color.NRGBA{
		R: uint8(int(c.R) * 70 / 100),
		G: uint8(int(c.G) * 70 / 100),
		B: uint8(int(c.B) * 70 / 100),
		A: c.A,
	}
}

func clampColor(v int) int {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return v
}

func ansi256ToColor(index int) color.NRGBA {
	if index < 0 {
		index = 0
	}
	if index > 255 {
		index = 255
	}

	base := []color.NRGBA{
		{0, 0, 0, 255},
		{128, 0, 0, 255},
		{0, 128, 0, 255},
		{128, 128, 0, 255},
		{0, 0, 128, 255},
		{128, 0, 128, 255},
		{0, 128, 128, 255},
		{192, 192, 192, 255},
		{128, 128, 128, 255},
		{255, 0, 0, 255},
		{0, 255, 0, 255},
		{255, 255, 0, 255},
		{0, 0, 255, 255},
		{255, 0, 255, 255},
		{0, 255, 255, 255},
		{255, 255, 255, 255},
	}
	if index < 16 {
		return base[index]
	}
	if index <= 231 {
		c := index - 16
		r := c / 36
		g := (c % 36) / 6
		b := c % 6
		scale := []uint8{0, 95, 135, 175, 215, 255}
		return color.NRGBA{R: scale[r], G: scale[g], B: scale[b], A: 255}
	}
	v := uint8(8 + (index-232)*10)
	return color.NRGBA{R: v, G: v, B: v, A: 255}
}
