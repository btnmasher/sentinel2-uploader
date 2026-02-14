//go:build !headless

package gui

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"sentinel2-uploader/internal/client"
	"sentinel2-uploader/internal/config"
	"sentinel2-uploader/internal/logging"
	"sentinel2-uploader/internal/runctx"
	"sentinel2-uploader/internal/runtime"
)

func waitGroupWithTimeout(wg *sync.WaitGroup, timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	if timeout <= 0 {
		<-done
		return true
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}

func (c *controller) startBackgroundLoop(name string, fn func(context.Context)) {
	c.bgWG.Go(func() {
		c.logger.Debug("background loop started", logging.Field("loop", name))
		fn(c.appCtx)
		c.logger.Debug("background loop stopped", logging.Field("loop", name))
	})
}

func (c *controller) bindLogs() {
	logCh := make(chan string, 256)
	c.unsubscribe = c.logger.Subscribe(func(event logging.Event) {
		line := logging.FormatEventANSI(event)
		select {
		case logCh <- line:
		default:
			select {
			case <-logCh:
			default:
			}
			logCh <- line
		}
	})

	c.startBackgroundLoop("gui log pump", func(ctx context.Context) {
		for {
			line, ok := runctx.RecvOrDone(ctx, "GUI log pump", c.logger, logCh)
			if !ok {
				return
			}
			text := line
			fyne.Do(func() {
				c.appendLog(text)
			})
		}
	})
}

func (c *controller) currentOptions() config.Options {
	debugEnabled := false
	if c.debugLogs != nil {
		debugEnabled = c.debugLogs.Checked
	}
	return config.Options{
		BaseURL: strings.TrimSpace(c.baseURL.Text),
		Token:   strings.TrimSpace(c.token.Text),
		LogFile: "",
		LogDir:  strings.TrimSpace(c.logDir.Text),
		Debug:   debugEnabled,
	}
}

func (c *controller) startUploader() {
	c.startUploaderWithContext(false)
}

func (c *controller) startUploaderWithContext(auto bool) {
	c.setStatus("Connecting", statusConnectingColor)
	opts := c.currentOptions()
	if strings.TrimSpace(opts.LogDir) == "" {
		c.setStatus("Error", statusErrorColor)
		dialog.ShowError(errors.New(c.startErrorText(auto, "log directory is required")), c.win)
		return
	}
	info, statErr := os.Stat(opts.LogDir)
	if statErr != nil || !info.IsDir() {
		c.setStatus("Error", statusErrorColor)
		if statErr != nil {
			dialog.ShowError(errors.New(c.startErrorText(auto, "log directory is not accessible: "+statErr.Error())), c.win)
		} else {
			dialog.ShowError(errors.New(c.startErrorText(auto, "log directory is not a directory")), c.win)
		}
		return
	}
	if err := config.ValidateRequired(opts); err != nil {
		c.setStatus("Error", statusErrorColor)
		dialog.ShowError(errors.New(c.startErrorText(auto, err.Error())), c.win)
		return
	}

	err := c.runner.Start(opts, c.logger, runtime.StartHooks{
		OnChannelsUpdate: c.onChannelsUpdate,
		OnStatus: func(status string) {
			fyne.Do(func() {
				c.applyRuntimeStatus(status)
			})
		},
		OnExit: func(runErr error) {
			fyne.Do(func() {
				c.setRunningState(false)
				if !c.shuttingDown {
					c.refreshTrayMenu()
				}
				if runErr != nil {
					c.setStatus("Disconnected", statusErrorColor)
					dialog.ShowError(runErr, c.win)
					return
				}
				c.setStatus("Idle", statusIdleColor)
			})
		},
	})
	if err != nil {
		c.setStatus("Error", statusErrorColor)
		dialog.ShowError(errors.New(c.startErrorText(auto, err.Error())), c.win)
		return
	}
	c.setRunningState(true)
	c.setStatus("Starting", statusConnectingColor)
}

func (c *controller) startErrorText(auto bool, message string) string {
	if !auto {
		return message
	}
	return "Couldn't auto-connect due to: " + message
}

func (c *controller) onChannelsUpdate(channels []client.ChannelConfig) {
	fyne.Do(func() {
		c.setChannels(channels)
	})
}

func (c *controller) stopUploader() {
	if c.runner.IsRunning() {
		c.setStatus("Stopping", statusStoppingColor)
	}
	c.runner.Stop()
}

func (c *controller) setRunningState(running bool) {
	if running {
		c.startButton.Disable()
		c.stopButton.Enable()
		c.refreshChannelPlaceholder()
		return
	}
	c.stopButton.Disable()
	c.refreshStartAvailability()
	c.refreshChannelPlaceholder()
}

func (c *controller) setLogVisibility(visible bool) {
	if visible {
		c.logWindowOpen = true
		c.logWindow.Show()
		c.logWindow.RequestFocus()
	} else {
		c.logWindowOpen = false
		c.logWindow.Hide()
	}
}

func (c *controller) selectLogDir() {
	start := c.ensureDirPickerStartPath(c.logDir.Text)
	c.dirPickerCurrent = start

	if c.dirPickerWindow == nil {
		c.dirPickerWindow = c.app.NewWindow("Select EVE Chat Logs Folder")
		c.dirPickerWindow.Resize(fyne.NewSize(760, 520))
		c.dirPickerPath = widget.NewEntry()
		c.dirPickerPath.OnSubmitted = func(value string) {
			candidate := c.ensureDirPickerStartPath(value)
			c.dirPickerCurrent = candidate
			c.dirPickerPath.SetText(candidate)
			c.refreshDirPickerList()
		}
		upButton := widget.NewButton("Up", func() {
			parent := filepath.Dir(c.dirPickerCurrent)
			if parent == "" || parent == c.dirPickerCurrent {
				return
			}
			c.dirPickerCurrent = parent
			c.dirPickerPath.SetText(parent)
			c.refreshDirPickerList()
		})
		useCurrent := widget.NewButton("Use Current Folder", func() {
			c.logDir.SetText(c.dirPickerCurrent)
			c.draft.LogDir = strings.TrimSpace(c.logDir.Text)
			c.refreshSettingsActions()
			c.dirPickerWindow.Hide()
		})
		closeButton := widget.NewButton("Close", func() {
			c.dirPickerWindow.Hide()
		})

		c.dirPickerList = widget.NewList(
			func() int { return len(c.dirPickerItems) },
			func() fyne.CanvasObject { return widget.NewLabel("directory") },
			func(id widget.ListItemID, obj fyne.CanvasObject) {
				obj.(*widget.Label).SetText(c.dirPickerItems[id])
			},
		)
		c.dirPickerList.OnSelected = func(id widget.ListItemID) {
			if id < 0 || id >= len(c.dirPickerItems) {
				return
			}
			next := filepath.Join(c.dirPickerCurrent, c.dirPickerItems[id])
			c.dirPickerCurrent = c.ensureDirPickerStartPath(next)
			c.dirPickerPath.SetText(c.dirPickerCurrent)
			c.refreshDirPickerList()
		}

		header := container.NewBorder(nil, nil, upButton, nil, c.dirPickerPath)
		actions := container.NewHBox(useCurrent, closeButton)
		c.dirPickerWindow.SetContent(container.NewBorder(header, actions, nil, nil, c.dirPickerList))
	}

	c.dirPickerPath.SetText(c.dirPickerCurrent)
	c.refreshDirPickerList()
	c.dirPickerWindow.Show()
	c.dirPickerWindow.RequestFocus()
}

func (c *controller) appendLog(line string) {
	if c.logGrid == nil && c.logSelectable == nil {
		return
	}

	lines := splitLogLines(line)
	if len(lines) == 0 {
		return
	}
	c.logRawLines = append(c.logRawLines, lines...)
	c.trimLogRows()
	c.rebuildLogRows()
	c.refreshLogView()
	if c.followEnabled {
		c.scrollLogsToBottom()
	}
}

func (c *controller) trimLogRows() {
	const maxLogRows = 1000
	if len(c.logRawLines) <= maxLogRows {
		return
	}
	c.logRawLines = append([]string(nil), c.logRawLines[len(c.logRawLines)-maxLogRows:]...)
	if len(c.logRows) > maxLogRows {
		c.logRows = append([]widget.TextGridRow(nil), c.logRows[len(c.logRows)-maxLogRows:]...)
	}
}

func (c *controller) rebuildLogRows() {
	const maxRenderedRows = 1000
	cols := c.logWrapColumns()
	wrapped := wrapANSILines(c.logRawLines, cols)
	if len(wrapped) > maxRenderedRows {
		wrapped = wrapped[len(wrapped)-maxRenderedRows:]
	}
	rows := make([]widget.TextGridRow, 0, len(wrapped))
	for _, line := range wrapped {
		rows = append(rows, parseANSITextGridRow(line))
	}
	c.logRows = rows
}

func (c *controller) logWrapColumns() int {
	if c.logGrid == nil {
		return 120
	}
	widthPx := c.logGrid.Size().Width
	if c.logScroll != nil && c.logScroll.Size().Width > 0 {
		widthPx = c.logScroll.Size().Width
	}
	if widthPx <= 0 {
		widthPx = 900
	}
	charSize := fyne.MeasureText("M", theme.TextSize(), fyne.TextStyle{Monospace: true})
	if charSize.Width <= 0 {
		return 120
	}
	cols := int(widthPx / charSize.Width)
	if cols < 40 {
		cols = 40
	}
	if cols > 240 {
		cols = 240
	}
	return cols - 2
}

func (c *controller) cleanup() {
	c.cleanupOnce.Do(func() {
		c.shuttingDown = true
		c.logger.Debug("gui cleanup started")
		if c.appCancel != nil {
			c.logger.Debug("canceling GUI root context")
			c.appCancel()
		}
		if c.unsubscribe != nil {
			c.logger.Debug("unsubscribing GUI log listener")
			c.unsubscribe()
		}
		c.logger.Debug("waiting for GUI background loops to stop")
		if ok := waitGroupWithTimeout(&c.bgWG, 2*time.Second); !ok {
			c.logger.Warn("GUI background loops did not stop within timeout")
		}
		c.logger.Debug("stopping runtime controller")
		if ok := c.runner.StopAndWait(3 * time.Second); !ok {
			c.logger.Warn("runtime controller did not stop within timeout")
		} else {
			c.logger.Debug("runtime controller stopped")
		}
		c.logger.Debug("gui cleanup complete")
	})
}

func (c *controller) quitApp() {
	c.quitOnce.Do(func() {
		c.logger.Debug("quit requested")
		c.cleanup()
		c.logger.Debug("calling fyne app quit")
		c.app.Quit()
	})
}

func (c *controller) requestQuit() {
	if c.shuttingDown {
		return
	}
	if !c.runner.IsRunning() {
		c.quitApp()
		return
	}
	if c.confirmingQuit {
		return
	}
	c.confirmingQuit = true
	dialog.ShowConfirm(
		"Quit Sentinel2 Uploader?",
		"This will stop the uploader connection.",
		func(ok bool) {
			c.confirmingQuit = false
			if !ok {
				return
			}
			c.quitApp()
		},
		c.win,
	)
}
