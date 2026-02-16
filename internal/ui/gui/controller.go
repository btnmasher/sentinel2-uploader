//go:build !headless

package gui

import (
	"context"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"

	"sentinel2-uploader/internal/client"
	"sentinel2-uploader/internal/config"
	"sentinel2-uploader/internal/evelogs"
	"sentinel2-uploader/internal/logging"
	"sentinel2-uploader/internal/runtime"
)

var (
	statusIdleColor       = color.NRGBA{R: 145, G: 145, B: 145, A: 255}
	statusConnectingColor = color.NRGBA{R: 219, G: 167, B: 74, A: 255}
	statusChannelsColor   = color.NRGBA{R: 120, G: 190, B: 255, A: 255}
	statusRunningColor    = color.NRGBA{R: 72, G: 189, B: 109, A: 255}
	statusStoppingColor   = color.NRGBA{R: 232, G: 145, B: 77, A: 255}
	statusErrorColor      = color.NRGBA{R: 220, G: 84, B: 84, A: 255}
	channelGreenColor     = color.NRGBA{R: 72, G: 189, B: 109, A: 255}
	channelYellowColor    = color.NRGBA{R: 219, G: 167, B: 74, A: 255}
	channelOrangeColor    = color.NRGBA{R: 232, G: 145, B: 77, A: 255}
	channelRedColor       = color.NRGBA{R: 220, G: 84, B: 84, A: 255}
)

const (
	channelStatusWarnAfter   = 10 * time.Minute
	channelStatusStaleAfter  = time.Hour
	channelStatusRefreshRate = 30 * time.Second
	tooltipCursorGap         = 10
)

type channelHealth struct {
	Color  color.NRGBA
	Reason string
}

type channelStatusRow struct {
	Channel client.ChannelConfig
	Health  channelHealth
}

type controller struct {
	app      fyne.App
	settings config.UploaderSettings
	draft    config.UploaderSettings
	win      fyne.Window
	logger   *logging.Logger
	runner   *runtime.Controller

	baseURL *widget.Entry
	token   *widget.Entry
	logDir  *widget.Entry

	debugLogs      *widget.Check
	connectOnStart *sliderToggle
	minimizeToTray *sliderToggle
	startMinimized *sliderToggle
	statusBadge    *statusBadge
	statusText     *widget.Label

	startButton    *widget.Button
	stopButton     *widget.Button
	showLogsButton *widget.Button
	saveSettings   *widget.Button
	cancelSettings *widget.Button

	logWindow       fyne.Window
	logWindowOpen   bool
	logGrid         *widget.TextGrid
	logScroll       *container.Scroll
	logSelectable   *widget.Entry
	logSelectScroll *container.Scroll
	selectableLogs  *widget.Check
	followButton    *widget.Button
	followEnabled   bool
	followJumping   bool
	logRawLines     []string
	logRows         []widget.TextGridRow
	logCols         int
	hoverTipLayer   *fyne.Container
	hoverTipShadow  *canvas.Rectangle
	hoverTipCard    *fyne.Container
	hoverTipLabel   *widget.Label
	hoverTipBG      *canvas.Rectangle
	channels        []client.ChannelConfig
	channelRows     []channelStatusRow
	channelList     *container.Scroll
	channelRowsBox  *fyne.Container
	channelEmpty    *fyne.Container
	channelNotice   *widget.Label

	dirPickerWindow  fyne.Window
	dirPickerPath    *widget.Entry
	dirPickerCurrent string
	dirPickerItems   []string
	dirPickerList    *widget.List

	cleanupOnce    sync.Once
	quitOnce       sync.Once
	bgWG           sync.WaitGroup
	unsubscribe    func()
	appCtx         context.Context
	appCancel      context.CancelFunc
	shuttingDown   bool
	confirmingQuit bool
}

func Run(rootCtx context.Context, buildVersion string, defaults config.Options) {
	uiApp := app.New()
	uiApp.Settings().SetTheme(newUploaderTheme())
	c := newController(rootCtx, uiApp, defaults)
	c.logger.Info("starting uploader UI", logging.Field("version", buildVersion))
	c.run()
}

func newController(rootCtx context.Context, uiApp fyne.App, defaults config.Options) *controller {
	settings := config.SettingsFromOptions(defaults)
	if saved, err := config.LoadSettings(); err == nil {
		defaults = config.MergeOptionsWithSettings(defaults, saved)
		settings = saved
	}
	if defaults.LogDir == "" {
		defaults.LogDir = config.DefaultLogDir()
	}
	settings.BaseURL = defaults.BaseURL
	settings.Token = defaults.Token
	settings.LogDir = defaults.LogDir
	settings.AutoConnect = defaults.AutoConnect
	settings.Debug = defaults.Debug

	logger := logging.New(false)
	if logger == nil {
		panic("gui.newController: logging.New returned nil")
	}
	logger.SetDebugEnabled(settings.Debug)
	if rootCtx == nil {
		rootCtx = context.Background()
	}
	appCtx, appCancel := context.WithCancel(rootCtx)

	c := &controller{
		app:       uiApp,
		settings:  settings,
		draft:     settings,
		logger:    logger,
		runner:    runtime.NewController(appCtx),
		appCtx:    appCtx,
		appCancel: appCancel,
	}

	uiApp.SetIcon(uploaderIconResource())
	c.win = uiApp.NewWindow("Sentinel2 Uploader")
	c.win.SetMaster()
	c.win.Resize(fyne.NewSize(460, 390))
	c.buildUI(defaults)
	c.bindLogs()
	c.setupTray()
	c.app.Lifecycle().SetOnStopped(func() {
		c.logger.Debug("app lifecycle OnStopped hook triggered")
		c.cleanup()
	})
	return c
}

func (c *controller) run() {
	c.setRunningState(false)
	c.startChannelHealthLoop()
	go func() {
		<-c.appCtx.Done()
		fyne.Do(func() {
			if c.shuttingDown {
				return
			}
			c.logger.Info("root context canceled; shutting down uploader UI")
			c.quitApp()
		})
	}()
	c.win.SetOnClosed(func() {
		c.logger.Debug("main window OnClosed hook triggered")
		if c.shuttingDown {
			c.logger.Debug("main window OnClosed hook ignored: already shutting down")
			return
		}
		c.cleanup()
	})
	c.win.SetCloseIntercept(func() {
		c.logger.Debug("main window CloseIntercept hook triggered",
			logging.Field("minimize_to_tray", c.shouldMinimizeToTrayOnClose()),
		)
		if c.shouldMinimizeToTrayOnClose() {
			c.logger.Debug("main window close intercepted: hiding to tray")
			c.win.Hide()
			return
		}
		c.logger.Debug("main window close intercepted: requesting quit")
		c.requestQuit()
	})

	if c.settings.StartMinimized {
		c.win.Show()
		c.win.Hide()
		c.tryAutoConnect()
		c.app.Run()
		return
	}

	c.win.Show()
	c.tryAutoConnect()
	c.app.Run()
}

func (c *controller) startChannelHealthLoop() {
	c.startBackgroundLoop("channel health", func(ctx context.Context) {
		ticker := time.NewTicker(channelStatusRefreshRate)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				fyne.Do(func() {
					c.refreshChannelHealth()
				})
			}
		}
	})
}

func (c *controller) buildUI(defaults config.Options) {
	c.baseURL = widget.NewEntry()
	c.baseURL.SetText(c.draft.BaseURL)

	c.token = widget.NewPasswordEntry()
	c.token.SetText(c.draft.Token)

	c.logDir = widget.NewEntry()
	c.logDir.SetText(c.draft.LogDir)

	c.debugLogs = widget.NewCheck("Debug level", func(v bool) {
		c.draft.Debug = v
		c.logger.SetDebugEnabled(v)
		c.refreshSettingsActions()
	})
	c.debugLogs.SetChecked(c.draft.Debug)
	c.logger.SetDebugEnabled(c.draft.Debug)
	c.connectOnStart = newSliderToggle(func(v bool) {
		c.draft.AutoConnect = v
		c.refreshSettingsActions()
	})
	c.connectOnStart.SetChecked(c.draft.AutoConnect)

	c.minimizeToTray = newSliderToggle(func(v bool) {
		c.draft.MinimizeToTray = v
		c.refreshSettingsActions()
	})
	c.minimizeToTray.SetChecked(c.draft.MinimizeToTray)

	c.startMinimized = newSliderToggle(func(v bool) {
		c.draft.StartMinimized = v
		c.refreshSettingsActions()
	})
	c.startMinimized.SetChecked(c.draft.StartMinimized)

	c.statusBadge = newStatusBadge(statusBadgeHandlers{
		Show: c.showHoverTooltip,
		Move: c.moveHoverTooltip,
		Hide: c.hideHoverTooltip,
	})
	c.statusBadge.SetStatus(statusIdleColor, "Idle")
	c.statusText = widget.NewLabel("Idle")
	c.channelRowsBox = container.NewVBox()
	c.channelList = container.NewVScroll(c.channelRowsBox)

	c.initLogWindow()
	c.setStatus("Idle", statusIdleColor)

	c.startButton = widget.NewButton("Connect", func() {
		c.startUploader()
		c.refreshTrayMenu()
	})
	c.stopButton = widget.NewButton("Disconnect", func() {
		c.stopUploader()
		c.refreshTrayMenu()
	})
	c.showLogsButton = widget.NewButton("Show logs", func() {
		c.setLogVisibility(true)
		c.refreshTrayMenu()
	})
	c.stopButton.Disable()
	controlsGap := canvas.NewRectangle(color.Transparent)
	controlsGap.SetMinSize(fyne.NewSize(12, 1))

	c.baseURL.OnChanged = func(v string) {
		c.draft.BaseURL = strings.TrimSpace(v)
		c.refreshSettingsActions()
		c.refreshStartAvailability()
	}
	c.token.OnChanged = func(v string) {
		c.draft.Token = strings.TrimSpace(v)
		c.refreshSettingsActions()
		c.refreshStartAvailability()
	}
	c.logDir.OnChanged = func(v string) {
		c.draft.LogDir = strings.TrimSpace(v)
		c.refreshSettingsActions()
		c.refreshChannelHealth()
	}

	browseLogDir := widget.NewButton("Browse...", c.selectLogDir)
	logDirRow := container.NewBorder(nil, nil, nil, browseLogDir, c.logDir)

	form := container.NewVBox(
		widget.NewLabel("Base URL"),
		c.baseURL,
		c.verticalGap(8),
		widget.NewLabel("Uploader Token"),
		c.token,
		c.verticalGap(8),
		widget.NewLabel("Log Directory"),
		logDirRow,
	)

	settingsRow := container.NewVBox(
		c.toggleRow("Connect on startup", c.connectOnStart),
		c.toggleRow("Close to tray", c.minimizeToTray),
		c.toggleRow("Start minimized", c.startMinimized),
	)
	c.saveSettings = widget.NewButton("Save", c.saveDraftSettings)
	c.cancelSettings = widget.NewButton("Cancel", c.cancelDraftSettings)
	settingsActions := container.NewHBox(c.saveSettings, c.cancelSettings)
	statusRow := container.NewHBox(c.statusBadge, c.statusText)
	controls := container.NewHBox(c.startButton, c.stopButton, controlsGap, c.showLogsButton, widget.NewLabel("Status:"), statusRow)

	overviewTop := container.NewPadded(container.NewVBox(
		controls,
	))
	c.channelNotice = widget.NewLabel("Not connected")
	c.channelNotice.Alignment = fyne.TextAlignCenter
	c.channelEmpty = container.NewVBox(
		layout.NewSpacer(),
		container.NewCenter(c.channelNotice),
		layout.NewSpacer(),
	)
	channelStack := container.NewMax(c.channelList, c.channelEmpty)
	channelPanel := container.NewPadded(container.NewBorder(
		widget.NewLabel("Configured Channels"),
		nil,
		nil,
		nil,
		channelStack,
	))
	pad := func(obj fyne.CanvasObject) fyne.CanvasObject {
		return container.NewPadded(container.NewPadded(obj))
	}

	overviewTab := container.NewTabItem("Overview", pad(container.NewBorder(
		overviewTop,
		nil,
		nil,
		nil,
		channelPanel,
	)))
	settingsTab := container.NewTabItem("Settings", pad(container.NewVBox(
		form,
		c.verticalGap(12),
		settingsRow,
		c.verticalGap(8),
		settingsActions,
	)))
	tabs := container.NewAppTabs(overviewTab, settingsTab)
	tabs.SetTabLocation(container.TabLocationTop)
	minAnchor := canvas.NewRectangle(color.Transparent)
	minAnchor.SetMinSize(fyne.NewSize(500, 340))
	c.hoverTipLabel = widget.NewLabel("")
	c.hoverTipLabel.Wrapping = fyne.TextWrapOff
	c.hoverTipBG = canvas.NewRectangle(color.NRGBA{R: 44, G: 44, B: 44, A: 250})
	c.hoverTipShadow = canvas.NewRectangle(color.NRGBA{R: 0, G: 0, B: 0, A: 120})
	c.hoverTipShadow.Hide()
	c.hoverTipCard = container.NewMax(c.hoverTipBG, container.NewPadded(c.hoverTipLabel))
	c.hoverTipCard.Hide()
	c.hoverTipLayer = container.NewWithoutLayout(c.hoverTipShadow, c.hoverTipCard)
	c.win.SetContent(container.NewStack(minAnchor, tabs, c.hoverTipLayer))
	c.refreshStartAvailability()
	c.refreshChannelHealth()
	c.refreshSettingsActions()
}

func (c *controller) persistSettings() {
	_ = config.SaveSettings(c.settings)
}

func (c *controller) settingsDirty() bool {
	return c.draft != c.settings
}

func (c *controller) refreshSettingsActions() {
	dirty := c.settingsDirty()
	if c.saveSettings != nil {
		if dirty {
			c.saveSettings.Enable()
		} else {
			c.saveSettings.Disable()
		}
	}
	if c.cancelSettings != nil {
		if dirty {
			c.cancelSettings.Enable()
		} else {
			c.cancelSettings.Disable()
		}
	}
}

func (c *controller) saveDraftSettings() {
	c.settings = c.draft
	c.persistSettings()
	c.refreshTrayMenu()
	c.refreshSettingsActions()
}

func (c *controller) cancelDraftSettings() {
	c.draft = c.settings
	c.baseURL.SetText(c.draft.BaseURL)
	c.token.SetText(c.draft.Token)
	c.logDir.SetText(c.draft.LogDir)
	c.debugLogs.SetChecked(c.draft.Debug)
	c.connectOnStart.SetChecked(c.draft.AutoConnect)
	c.minimizeToTray.SetChecked(c.draft.MinimizeToTray)
	c.startMinimized.SetChecked(c.draft.StartMinimized)
	c.logger.SetDebugEnabled(c.draft.Debug)
	c.refreshStartAvailability()
	c.refreshChannelHealth()
	c.refreshSettingsActions()
}

func (c *controller) toggleRow(label string, sw *sliderToggle) fyne.CanvasObject {
	return container.NewBorder(nil, nil, widget.NewLabel(label), sw, nil)
}

func (c *controller) verticalGap(height float32) fyne.CanvasObject {
	spacer := canvas.NewRectangle(color.Transparent)
	spacer.SetMinSize(fyne.NewSize(1, height))
	return spacer
}

func (c *controller) setStatus(text string, dotColor color.NRGBA) {
	c.statusText.SetText(text)
	if c.statusBadge != nil {
		c.statusBadge.SetStatus(dotColor, "")
	}
}

func (c *controller) showHoverTooltip(text string, anchor fyne.Position) {
	if c.hoverTipCard == nil || c.hoverTipLabel == nil || c.hoverTipBG == nil {
		return
	}
	c.hoverTipLabel.SetText(text)
	size := c.hoverTipCard.MinSize()
	c.hoverTipCard.Resize(size)
	pos := c.hoverTooltipPosition(anchor, size)
	c.hoverTipCard.Move(pos)
	if c.hoverTipShadow != nil {
		c.hoverTipShadow.Resize(size)
		c.hoverTipShadow.Move(fyne.NewPos(pos.X+2, pos.Y+2))
		c.hoverTipShadow.Show()
	}
	c.hoverTipCard.Show()
	c.hoverTipLayer.Refresh()
}

func (c *controller) moveHoverTooltip(anchor fyne.Position) {
	if c.hoverTipCard == nil || !c.hoverTipCard.Visible() {
		return
	}
	size := c.hoverTipCard.Size()
	if size.Width <= 0 || size.Height <= 0 {
		size = c.hoverTipCard.MinSize()
		c.hoverTipCard.Resize(size)
	}
	pos := c.hoverTooltipPosition(anchor, size)
	c.hoverTipCard.Move(pos)
	if c.hoverTipShadow != nil {
		c.hoverTipShadow.Resize(size)
		c.hoverTipShadow.Move(fyne.NewPos(pos.X+2, pos.Y+2))
	}
	c.hoverTipLayer.Refresh()
}

func (c *controller) hideHoverTooltip() {
	if c.hoverTipCard == nil {
		return
	}
	if c.hoverTipShadow != nil {
		c.hoverTipShadow.Hide()
	}
	c.hoverTipCard.Hide()
	c.hoverTipLayer.Refresh()
}

func (c *controller) hoverTooltipPosition(anchor fyne.Position, size fyne.Size) fyne.Position {
	const pad = float32(4)
	x := anchor.X + tooltipCursorGap
	y := anchor.Y + tooltipCursorGap
	canvasSize := c.win.Canvas().Size()
	maxX := max(pad, canvasSize.Width-size.Width-pad)
	maxY := max(pad, canvasSize.Height-size.Height-pad)
	x = min(max(pad, x), maxX)
	y = min(max(pad, y), maxY)
	return fyne.NewPos(x, y)
}

func (c *controller) applyRuntimeStatus(status string) {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "authenticated":
		c.setStatus("Authenticated", statusConnectingColor)
	case "channels received":
		c.setStatus("Channels received", statusChannelsColor)
	case "connected":
		c.setStatus("Connected", statusRunningColor)
	default:
		c.setStatus(status, statusIdleColor)
	}
}

func (c *controller) refreshStartAvailability() {
	if c.runner.IsRunning() {
		return
	}
	baseURL := strings.TrimSpace(c.baseURL.Text)
	token := strings.TrimSpace(c.token.Text)
	if baseURL == "" || token == "" {
		c.startButton.Disable()
		return
	}
	c.startButton.Enable()
}

func (c *controller) tryAutoConnect() {
	if !c.settings.AutoConnect || c.runner.IsRunning() {
		return
	}
	if strings.TrimSpace(c.baseURL.Text) == "" || strings.TrimSpace(c.token.Text) == "" {
		return
	}
	fyne.Do(func() {
		c.startUploaderWithContext(true)
		c.refreshTrayMenu()
	})
}

func (c *controller) setChannels(channels []client.ChannelConfig) {
	c.channels = append([]client.ChannelConfig(nil), channels...)
	c.refreshChannelHealth()
}

func (c *controller) refreshChannelHealth() {
	rows := make([]channelStatusRow, 0, len(c.channels))
	now := time.Now()
	logDir := strings.TrimSpace(c.logDir.Text)

	latestByChannel := map[string]time.Time{}
	latestPathByChannel := map[string]string{}
	scanErrText := ""
	if logDir == "" {
		scanErrText = "Log directory is not configured."
	} else if info, statErr := os.Stat(logDir); statErr != nil {
		scanErrText = fmt.Sprintf("Log directory is not accessible: %v", statErr)
	} else if !info.IsDir() {
		scanErrText = "Log path is not a directory."
	} else {
		logs, findErr := evelogs.FindLogs(logDir, c.channels)
		if findErr != nil {
			scanErrText = fmt.Sprintf("Failed to scan logs: %v", findErr)
		} else {
			for _, selection := range logs {
				stat, statErr := os.Stat(selection.Path)
				if statErr != nil {
					continue
				}
				id := strings.TrimSpace(selection.Channel.ID)
				if id == "" {
					continue
				}
				current, ok := latestByChannel[id]
				if !ok || stat.ModTime().After(current) {
					latestByChannel[id] = stat.ModTime()
					latestPathByChannel[id] = filepath.Base(selection.Path)
				}
			}
		}
	}

	for _, channel := range c.channels {
		id := strings.TrimSpace(channel.ID)
		name := strings.TrimSpace(channel.Name)
		health := channelHealth{
			Color:  channelRedColor,
			Reason: "Log file for channel not found.",
		}
		if scanErrText != "" {
			health.Reason = scanErrText
		} else if last, ok := latestByChannel[id]; ok {
			age := now.Sub(last)
			if age <= channelStatusWarnAfter {
				health.Color = channelGreenColor
				health.Reason = fmt.Sprintf("Active: Last report %s ago.", age.Round(time.Second))
			} else if age <= channelStatusStaleAfter {
				health.Color = channelYellowColor
				health.Reason = fmt.Sprintf("Stale: Last report %s ago.", age.Round(time.Second))
			} else {
				health.Color = channelOrangeColor
				health.Reason = fmt.Sprintf("Very stale: Last report %s ago.", age.Round(time.Second))
			}
		}

		rows = append(rows, channelStatusRow{
			Channel: client.ChannelConfig{
				ID:   id,
				Name: name,
			},
			Health: health,
		})
	}
	c.channelRows = rows
	c.rebuildChannelRows()
	c.refreshChannelPlaceholder()
}

func (c *controller) refreshChannelPlaceholder() {
	if c.channelList == nil || c.channelRowsBox == nil || c.channelEmpty == nil || c.channelNotice == nil {
		return
	}
	if c.runner != nil && c.runner.IsRunning() && len(c.channelRows) > 0 {
		c.channelEmpty.Hide()
		c.channelList.Show()
		return
	}
	if c.runner != nil && c.runner.IsRunning() {
		c.channelNotice.SetText("No channels configured")
	} else {
		c.channelNotice.SetText("Not connected")
	}
	c.channelList.Hide()
	c.channelEmpty.Show()
}

func (c *controller) rebuildChannelRows() {
	if c.channelRowsBox == nil {
		return
	}
	rows := make([]fyne.CanvasObject, 0, len(c.channelRows))
	for _, item := range c.channelRows {
		badge := newStatusBadge(statusBadgeHandlers{
			Show: c.showHoverTooltip,
			Move: c.moveHoverTooltip,
			Hide: c.hideHoverTooltip,
		})
		badge.SetStatus(item.Health.Color, item.Health.Reason)
		name := item.Channel.Name
		if strings.TrimSpace(name) == "" {
			name = item.Channel.ID
		}
		label := widget.NewLabel(name)
		label.Truncation = fyne.TextTruncateEllipsis
		label.Wrapping = fyne.TextWrapOff
		row := container.NewBorder(nil, nil, container.NewCenter(badge), nil, label)
		rows = append(rows, row)
	}
	c.channelRowsBox.Objects = rows
	c.channelRowsBox.Refresh()
}

func (c *controller) initLogWindow() {
	c.logGrid = widget.NewTextGrid()
	c.logGrid.Scroll = fyne.ScrollNone
	c.logScroll = container.NewVScroll(c.logGrid)
	c.logSelectable = widget.NewMultiLineEntry()
	c.logSelectable.Wrapping = fyne.TextWrapWord
	c.logSelectScroll = container.NewVScroll(c.logSelectable)
	c.logSelectScroll.Hide()
	c.followEnabled = true
	c.logCols = c.logWrapColumns()
	c.selectableLogs = widget.NewCheck("Selectable text", func(v bool) {
		if v {
			c.logScroll.Hide()
			c.logSelectScroll.Show()
		} else {
			c.logSelectScroll.Hide()
			c.logScroll.Show()
		}
		if c.followEnabled {
			c.scrollLogsToBottom()
		}
	})

	c.followButton = widget.NewButton("Following", func() {
		c.setFollowEnabled(true)
		c.scrollLogsToBottom()
	})
	c.followButton.Disable()
	clearButton := widget.NewButton("Clear", func() {
		c.logRawLines = nil
		c.logRows = nil
		c.logGrid.Rows = nil
		c.logGrid.Refresh()
		c.scrollLogsToBottom()
	})
	c.logWindow = c.app.NewWindow("Sentinel2 Uploader Logs")
	c.logWindow.Resize(fyne.NewSize(900, 520))
	centerGap := container.NewHBox(c.debugLogs, c.selectableLogs, layout.NewSpacer())
	header := container.NewBorder(nil, nil, clearButton, c.followButton, centerGap)
	logBG := canvas.NewRectangle(color.NRGBA{R: 0, G: 0, B: 0, A: 255})
	c.logScroll.OnScrolled = func(pos fyne.Position) {
		if c.followJumping {
			return
		}
		if !c.logAtBottom(pos) {
			c.setFollowEnabled(false)
		}
	}
	c.logWindow.SetContent(container.NewBorder(header, nil, nil, nil, container.NewMax(logBG, c.logScroll, c.logSelectScroll)))
	c.logWindowOpen = false
	c.logWindow.SetCloseIntercept(func() {
		if c.shuttingDown {
			return
		}
		c.logWindowOpen = false
		c.logWindow.Hide()
		c.refreshTrayMenu()
	})

	c.watchLogGridWidth()
}

func (c *controller) setFollowEnabled(enabled bool) {
	c.followEnabled = enabled
	if c.followButton == nil {
		return
	}
	if enabled {
		c.followButton.SetText("Following")
		c.followButton.Disable()
		return
	}
	c.followButton.SetText("Follow")
	c.followButton.Enable()
}

func (c *controller) scrollLogsToBottom() {
	c.followJumping = true
	if c.selectableLogs != nil && c.selectableLogs.Checked {
		if c.logSelectScroll != nil {
			c.logSelectScroll.ScrollToBottom()
		}
	} else if c.logScroll != nil {
		c.logScroll.ScrollToBottom()
	}
	c.followJumping = false
}

func (c *controller) logAtBottom(pos fyne.Position) bool {
	if c.logScroll == nil || c.logGrid == nil {
		return true
	}
	contentHeight := c.logGrid.MinSize().Height
	viewportHeight := c.logScroll.Size().Height
	if contentHeight <= viewportHeight+1 {
		return true
	}
	return pos.Y+viewportHeight >= contentHeight-1
}

func (c *controller) watchLogGridWidth() {
	c.startBackgroundLoop("log wrap watcher", func(ctx context.Context) {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				next := c.logWrapColumns()
				if next == c.logCols {
					continue
				}
				c.logCols = next
				fyne.Do(func() {
					c.rebuildLogRows()
					c.refreshLogView()
					if c.followEnabled {
						c.scrollLogsToBottom()
					}
				})
			}
		}
	})
}

func (c *controller) refreshLogView() {
	if c.logGrid != nil {
		c.logGrid.Rows = c.logRows
		c.logGrid.Refresh()
	}
	if c.logSelectable != nil {
		plain := make([]string, 0, len(c.logRawLines))
		for _, line := range c.logRawLines {
			plain = append(plain, stripANSIText(line))
		}
		c.logSelectable.SetText(strings.Join(plain, "\n"))
	}
}

func (c *controller) shouldMinimizeToTrayOnClose() bool {
	if c.minimizeToTray != nil {
		return c.minimizeToTray.Checked
	}
	return c.settings.MinimizeToTray
}

func (c *controller) ensureDirPickerStartPath(path string) string {
	candidate := strings.TrimSpace(path)
	if candidate == "" {
		candidate = config.DefaultLogDir()
	}
	if info, err := os.Stat(candidate); err == nil && info.IsDir() {
		return filepath.Clean(candidate)
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Clean(home)
	}
	return "/"
}

func (c *controller) refreshDirPickerList() {
	entries, err := os.ReadDir(c.dirPickerCurrent)
	if err != nil {
		c.dirPickerItems = nil
		c.dirPickerList.Refresh()
		return
	}
	items := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			items = append(items, entry.Name())
		}
	}
	sort.Strings(items)
	c.dirPickerItems = items
	c.dirPickerList.Refresh()
}
