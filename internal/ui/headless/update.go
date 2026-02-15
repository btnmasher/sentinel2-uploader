package headless

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"sentinel2-uploader/internal/client"
	"sentinel2-uploader/internal/config"
	"sentinel2-uploader/internal/logging"
	"sentinel2-uploader/internal/ui/headless/health"
	headlessview "sentinel2-uploader/internal/ui/headless/view"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *headlessModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.quitting {
		if _, ok := msg.(quitNowMsg); ok {
			m.cleanup()
			return m, tea.Quit
		}
		return m, nil
	}

	if m.ui.FilePickerOpen {
		if ws, ok := msg.(tea.WindowSizeMsg); ok {
			m.ui = m.ui.WithWindowSize(ws.Width, ws.Height)
			m.ui.ResizeLogs(nonLogLayoutReserveMin, minLogPanelHeight)
			headlessview.ResizePaneViewports(&m.ui, m.runtimeView())
			m.ui.ResizeFilePicker()
		}
		return m.updateFilePickerMsg(msg)
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.ui = m.ui.WithWindowSize(msg.Width, msg.Height)
		m.ui.ResizeLogs(nonLogLayoutReserveMin, minLogPanelHeight)
		headlessview.ResizePaneViewports(&m.ui, m.runtimeView())
		return m, nil
	case logMsg:
		line := string(msg)
		wasAtBottom := m.ui.LogView.AtBottom()
		m.ui.LogText = logging.AppendWithLimit(m.ui.LogText, line, headlessLogLimit)
		m.ui.SetLogViewportContent()
		if m.ui.FollowLogs || wasAtBottom {
			m.ui.LogView.GotoBottom()
			m.ui.FollowLogs = true
		}
		return m, waitForLog(m.logCh)
	case channelsUpdatedMsg:
		m.channels = append([]client.ChannelConfig(nil), msg...)
		m.refreshChannelHealth()
		return m, waitForChannels(m.cfgCh)
	case statusMsg:
		m.applyRuntimeStatus(string(msg))
		return m, waitForStatus(m.statusCh)
	case runDoneMsg:
		m.running = false
		m.connecting = false
		if msg.err != nil {
			m.status = "Disconnected (error)"
			m.kind = statusError
			m.ui.ErrorModalText = msg.err.Error()
		} else {
			m.status = "Idle"
			m.kind = statusIdle
			m.ui.ErrorModalText = ""
		}
		return m, nil
	case startResultMsg:
		m.connecting = false
		if msg.err != nil {
			m.status = "Disconnected (error)"
			m.kind = statusError
			m.ui.ErrorModalText = msg.err.Error()
			return m, nil
		}
		m.running = true
		if strings.TrimSpace(m.status) == "" || strings.EqualFold(m.status, "Connecting...") {
			m.status = "Starting"
			m.kind = statusConnecting
		}
		m.ui.ErrorModalText = ""
		return m, nil
	case tickMsg:
		m.ui = m.ui.WithTick()
		if time.Since(m.lastHealthRefresh) >= health.RefreshRate {
			m.refreshChannelHealth()
		}
		return m, tickCmd()
	case tea.MouseMsg:
		return m.updateMouseMsg(msg)
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)
	}

	next, cmd, ok := headlessview.ReduceInput(m.ui, msg)
	if ok {
		m.ui = next
		return m, cmd
	}
	return m, nil
}

func (m *headlessModel) updateMouseMsg(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	next, cmd, effect := headlessview.ReduceMouse(m.ui, msg)
	m.ui = next
	switch effect {
	case headlessview.MouseEffectActivateFocused:
		return m, tea.Batch(cmd, m.activateFocusedControl())
	case headlessview.MouseEffectConfirmQuitAccept:
		return m, tea.Batch(cmd, m.beginQuitCmd())
	}
	return m, cmd
}

func (m *headlessModel) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	next, effect := headlessview.ReduceKey(m.ui, msg)
	m.ui = next
	switch effect {
	case headlessview.KeyEffectRequestQuit:
		return m, m.requestQuitCmd()
	case headlessview.KeyEffectSaveSettings:
		return m, m.saveSettingsDraft()
	case headlessview.KeyEffectActivateFocused:
		return m, m.activateFocusedControl()
	case headlessview.KeyEffectConfirmQuitAccept:
		return m, m.beginQuitCmd()
	default:
		return m, nil
	}
}

func (m *headlessModel) activateFocusedControl() tea.Cmd {
	next, effect := headlessview.ReduceActivate(m.ui, m.canConnect(), m.running, m.connecting)
	m.ui = next
	switch effect {
	case headlessview.ActivateEffectStartUploader:
		return m.startUploaderCmd(false)
	case headlessview.ActivateEffectStopUploader:
		m.runner.Stop()
		m.status = "Stopping..."
		m.kind = statusStopping
		headlessview.ResizePaneViewports(&m.ui, m.runtimeView())
		return nil
	case headlessview.ActivateEffectRequestQuit:
		return m.requestQuitCmd()
	case headlessview.ActivateEffectDebugLevelChanged:
		m.logger.SetDebugEnabled(m.ui.DebugOn)
		return nil
	case headlessview.ActivateEffectOpenBrowse:
		return m.openBrowseCmd()
	case headlessview.ActivateEffectSaveSettings:
		return m.saveSettingsDraft()
	default:
		return nil
	}
}

func (m *headlessModel) openBrowseCmd() tea.Cmd {
	startDir := strings.TrimSpace(m.ui.Inputs[2].Value())
	if startDir == "" {
		startDir = config.DefaultLogDir()
	}
	if abs, err := filepath.Abs(startDir); err == nil {
		startDir = abs
	}
	if info, err := os.Stat(startDir); err != nil || !info.IsDir() {
		startDir = "."
		if abs, err := filepath.Abs(startDir); err == nil {
			startDir = abs
		}
	}
	m.ui.FilePicker.CurrentDirectory = startDir
	m.ui.FilePicker.Path = ""
	m.ui.FilePickerOpen = true
	m.ui.ResizeFilePicker()
	return m.ui.FilePicker.Init()
}

func (m *headlessModel) requestQuitCmd() tea.Cmd {
	if m.running || m.connecting {
		m.ui.ConfirmQuit = true
		m.ui.ConfirmQuitChoice = headlessview.ConfirmQuitChoiceCancel
		return nil
	}
	return m.beginQuitCmd()
}

func quitProgramCmd() tea.Cmd {
	return tea.Sequence(func() tea.Msg {
		return tea.DisableMouse()
	}, waitForMouseDrainCmd(), func() tea.Msg {
		return quitNowMsg{}
	})
}

func waitForMouseDrainCmd() tea.Cmd {
	return func() tea.Msg {
		time.Sleep(120 * time.Millisecond)
		return nil
	}
}

func (m *headlessModel) beginQuitCmd() tea.Cmd {
	m.quitting = true
	m.ui.ConfirmQuit = false
	return quitProgramCmd()
}

func (m *headlessModel) updateFilePickerMsg(msg tea.Msg) (tea.Model, tea.Cmd) {
	if keyMsg, ok := msg.(tea.KeyMsg); ok {
		switch keyMsg.String() {
		case "ctrl+c":
			m.ui.FilePickerOpen = false
			return m, m.requestQuitCmd()
		case "esc":
			m.ui.FilePickerOpen = false
			return m, nil
		case "left", "backspace":
			parent := filepath.Dir(m.ui.FilePicker.CurrentDirectory)
			if parent == "" || parent == m.ui.FilePicker.CurrentDirectory {
				return m, nil
			}
			m.ui.FilePicker.CurrentDirectory = parent
			return m, m.ui.FilePicker.Init()
		case "enter":
			return m.selectCurrentFilePickerDir()
		}
	}
	var cmd tea.Cmd
	m.ui.FilePicker, cmd = m.ui.FilePicker.Update(msg)
	if ok, path := m.ui.FilePicker.DidSelectFile(msg); ok {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			path = filepath.Dir(path)
		}
		m.applySelectedLogDir(path)
		return m, nil
	}
	return m, cmd
}

func (m *headlessModel) selectCurrentFilePickerDir() (tea.Model, tea.Cmd) {
	path := strings.TrimSpace(m.ui.FilePicker.CurrentDirectory)
	if path == "" {
		path = "."
	}
	m.applySelectedLogDir(path)
	return m, nil
}

func (m *headlessModel) applySelectedLogDir(path string) {
	m.ui = m.ui.WithSelectedLogDir(path)
}

func (m *headlessModel) saveSettingsDraft() tea.Cmd {
	if !m.ui.SettingsDirty {
		return nil
	}

	m.ui = m.ui.WithDraftFromControls()
	next := m.ui.WithSaveCommitted()

	settings := config.SettingsFromOptions(m.currentOptions())
	if saved, err := config.LoadSettings(); err == nil {
		settings.MinimizeToTray = saved.MinimizeToTray
		settings.StartMinimized = saved.StartMinimized
	}
	if err := config.SaveSettings(settings); err != nil {
		m.ui.ErrorModalText = err.Error()
		return nil
	}

	m.ui = next
	return nil
}
