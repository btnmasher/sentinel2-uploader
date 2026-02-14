//go:build !headless

package gui

import (
	_ "embed"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

//go:embed assets/Hack-Regular.ttf
var hackRegular []byte

//go:embed assets/Hack-Bold.ttf
var hackBold []byte

//go:embed assets/Hack-Italic.ttf
var hackItalic []byte

//go:embed assets/Hack-BoldItalic.ttf
var hackBoldItalic []byte

type uploaderTheme struct {
	base           fyne.Theme
	hackRegular    fyne.Resource
	hackBold       fyne.Resource
	hackItalic     fyne.Resource
	hackBoldItalic fyne.Resource
}

func newUploaderTheme() fyne.Theme {
	return &uploaderTheme{
		base:           theme.DefaultTheme(),
		hackRegular:    fyne.NewStaticResource("Hack-Regular.ttf", hackRegular),
		hackBold:       fyne.NewStaticResource("Hack-Bold.ttf", hackBold),
		hackItalic:     fyne.NewStaticResource("Hack-Italic.ttf", hackItalic),
		hackBoldItalic: fyne.NewStaticResource("Hack-BoldItalic.ttf", hackBoldItalic),
	}
}

func (t *uploaderTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	return t.base.Color(name, variant)
}

func (t *uploaderTheme) Font(style fyne.TextStyle) fyne.Resource {
	if style.Bold && style.Italic {
		return t.hackBoldItalic
	}
	if style.Bold {
		return t.hackBold
	}
	if style.Italic {
		return t.hackItalic
	}
	return t.hackRegular
}

func (t *uploaderTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return t.base.Icon(name)
}

func (t *uploaderTheme) Size(name fyne.ThemeSizeName) float32 {
	return t.base.Size(name)
}
