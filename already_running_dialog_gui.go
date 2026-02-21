//go:build !headless

package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"sentinel2-uploader/internal/ui/gui"
)

func showAlreadyRunningDialog() {
	uiApp := app.New()
	uiApp.SetIcon(gui.AppIconResource())
	win := uiApp.NewWindow("Sentinel2 Uploader")
	win.SetFixedSize(true)
	win.Resize(fyne.NewSize(420, 140))
	ok := widget.NewButton("OK", func() {
		uiApp.Quit()
	})
	message := widget.NewLabel("Sentinel2 Uploader is already running.\n(Check your system tray?)")
	message.Alignment = fyne.TextAlignCenter
	buttonWrap := container.NewGridWrap(fyne.NewSize(104, 34), ok)
	rightGap := canvas.NewRectangle(nil)
	rightGap.SetMinSize(fyne.NewSize(3, 1))
	buttonBar := container.NewHBox(layout.NewSpacer(), buttonWrap, rightGap)
	bottomGap := canvas.NewRectangle(nil)
	bottomGap.SetMinSize(fyne.NewSize(1, 3))
	content := container.NewBorder(
		message,
		container.NewVBox(buttonBar, bottomGap),
		nil,
		nil,
		nil,
	)
	win.SetContent(container.NewPadded(content))
	win.SetCloseIntercept(func() {
		uiApp.Quit()
	})
	win.Show()
	uiApp.Run()
}
