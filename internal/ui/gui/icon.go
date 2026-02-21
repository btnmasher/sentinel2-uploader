//go:build !headless

package gui

import (
	_ "embed"

	"fyne.io/fyne/v2"
)

//go:embed assets/s2-uploader-icon.png
var s2UploaderIconPNG []byte

func uploaderIconResource() fyne.Resource {
	return fyne.NewStaticResource("s2-uploader-icon.png", s2UploaderIconPNG)
}

func AppIconResource() fyne.Resource {
	return uploaderIconResource()
}
