//go:build !headless

package gui

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

func (c *controller) setupTray() {
	if _, ok := c.app.(desktop.App); !ok {
		return
	}
	c.refreshTrayMenu()
}

func (c *controller) refreshTrayMenu() {
	if c.shuttingDown {
		return
	}
	desk, ok := c.app.(desktop.App)
	if !ok {
		return
	}

	desk.SetSystemTrayIcon(uploaderIconResource())

	running := c.runner.IsRunning()
	canStart := c.startButton != nil && !c.startButton.Disabled()

	openItem := fyne.NewMenuItem("Open Window", func() {
		c.win.Show()
		c.win.RequestFocus()
	})
	showLogsItem := fyne.NewMenuItem("Show Logs", func() {
		c.setLogVisibility(!c.logWindowOpen)
		c.refreshTrayMenu()
	})
	showLogsItem.Checked = c.logWindowOpen

	connectItem := fyne.NewMenuItem("Connect", c.startUploader)
	connectItem.Disabled = running || !canStart

	disconnectItem := fyne.NewMenuItem("Disconnect", c.stopUploader)
	disconnectItem.Disabled = !running

	minTrayItem := fyne.NewMenuItem("Minimize to tray", func() {
		next := !c.settings.MinimizeToTray
		c.settings.MinimizeToTray = next
		c.draft.MinimizeToTray = next
		c.minimizeToTray.SetChecked(next)
		c.persistSettings()
		c.refreshSettingsActions()
		c.refreshTrayMenu()
	})
	minTrayItem.Checked = c.settings.MinimizeToTray

	startMinItem := fyne.NewMenuItem("Start minimized", func() {
		next := !c.settings.StartMinimized
		c.settings.StartMinimized = next
		c.draft.StartMinimized = next
		c.startMinimized.SetChecked(next)
		c.persistSettings()
		c.refreshSettingsActions()
		c.refreshTrayMenu()
	})
	startMinItem.Checked = c.settings.StartMinimized

	exitItem := fyne.NewMenuItem("Exit", func() {
		c.requestQuit()
	})

	tray := fyne.NewMenu("Sentinel2 Uploader",
		openItem,
		showLogsItem,
		connectItem,
		disconnectItem,
		fyne.NewMenuItemSeparator(),
		minTrayItem,
		startMinItem,
		fyne.NewMenuItemSeparator(),
		exitItem,
	)
	desk.SetSystemTrayMenu(tray)
}
