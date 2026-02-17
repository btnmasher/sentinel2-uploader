package headless

import (
	"os"
	"strings"
	"time"

	"sentinel2-uploader/internal/client"
	"sentinel2-uploader/internal/config"
	"sentinel2-uploader/internal/runstatus"
	"sentinel2-uploader/internal/runtime"
	"sentinel2-uploader/internal/ui/headless/health"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *headlessModel) currentOptions() config.Options {
	return config.Options{
		BaseURL:     strings.TrimSpace(m.ui.Inputs[0].Value()),
		Token:       strings.TrimSpace(m.ui.Inputs[1].Value()),
		AutoConnect: m.ui.AutoConn,
		ImGay:       m.ui.ImGay,
		LogFile:     "",
		LogDir:      strings.TrimSpace(m.ui.Inputs[2].Value()),
		Debug:       m.ui.DebugOn,
	}
}

func (m *headlessModel) canConnect() bool {
	return strings.TrimSpace(m.ui.Inputs[0].Value()) != "" && strings.TrimSpace(m.ui.Inputs[1].Value()) != ""
}

func (m *headlessModel) startUploaderCmd(auto bool) tea.Cmd {
	opts := m.currentOptions()
	if strings.TrimSpace(opts.LogDir) == "" {
		m.ui.ErrorModalText = m.startErrorText(auto, "Log directory is required.")
		return nil
	}

	info, statErr := os.Stat(opts.LogDir)
	if statErr != nil || !info.IsDir() {
		if statErr != nil {
			m.ui.ErrorModalText = m.startErrorText(auto, "Log directory is not accessible: "+statErr.Error())
		} else {
			m.ui.ErrorModalText = m.startErrorText(auto, "Log directory is not a directory.")
		}
		return nil
	}

	if err := config.ValidateRequired(opts); err != nil {
		m.ui.ErrorModalText = m.startErrorText(auto, err.Error())
		return nil
	}

	m.connecting = true
	m.status = "Connecting..."
	m.kind = statusConnecting
	m.ui.ErrorModalText = ""

	return func() tea.Msg {
		err := m.runner.Start(opts, m.logger, runtime.StartHooks{
			OnChannelsUpdate: m.onRuntimeChannelsUpdate,
			OnStatus:         m.onRuntimeStatus,
			OnExit:           m.onRuntimeExit,
		})

		return startResultMsg{err: err}
	}
}

func (m *headlessModel) onRuntimeChannelsUpdate(channels []client.ChannelConfig) {
	normalized := make([]client.ChannelConfig, 0, len(channels))
	for _, channel := range channels {
		name := strings.TrimSpace(channel.Name)
		id := strings.TrimSpace(channel.ID)
		if name == "" || id == "" {
			continue
		}

		normalized = append(normalized, client.ChannelConfig{ID: id, Name: name})
	}

	select {
	case m.cfgCh <- normalized:
	default:
		select {
		case <-m.cfgCh:
		default:
		}
		m.cfgCh <- normalized
	}
}

func (m *headlessModel) onRuntimeStatus(status string) {
	select {
	case m.statusCh <- status:
	default:
		select {
		case <-m.statusCh:
		default:
		}
		m.statusCh <- status
	}
}

func (m *headlessModel) onRuntimeExit(runErr error) {
	if m.program == nil {
		return
	}

	m.program.Send(runDoneMsg{err: runErr})
}

func (m *headlessModel) applyRuntimeStatus(status string) {
	switch runstatus.Key(status) {
	case runstatus.KeyAuthenticated:
		m.status = runstatus.Authenticated
		m.kind = statusConnecting
	case runstatus.KeyChannelsReceived:
		m.status = runstatus.ChannelsReceived
		m.kind = statusIdle
	case runstatus.KeyConnected:
		m.status = runstatus.Connected
		m.kind = statusConnected
		m.running = true
		m.connecting = false
	case runstatus.KeyReconnecting:
		m.status = runstatus.Reconnecting
		m.kind = statusConnecting
		m.connecting = true
	case runstatus.KeyDisconnected:
		m.status = runstatus.Disconnected
		m.kind = statusIdle
		m.connecting = false
	case runstatus.KeyDisconnectedAuth:
		m.status = runstatus.DisconnectedAuth
		m.kind = statusError
		m.connecting = false
	default:
		m.status = status
	}
}

func (m *headlessModel) startErrorText(auto bool, message string) string {
	if !auto {
		return message
	}

	return "Couldn't auto-connect due to: " + message
}

func (m *headlessModel) refreshChannelHealth() {
	m.lastHealthRefresh = time.Now()
	m.channelHealth, m.healthDetail = health.Compute(m.ui.Inputs[2].Value(), m.channels, m.lastHealthRefresh)
}

func (m *headlessModel) cleanup() {
	m.cleanupOnce.Do(func() {
		m.logger.Debug("headless cleanup started")

		if m.rootCancel != nil {
			m.logger.Debug("canceling headless root context")
			m.rootCancel()
		}

		if m.unsubscribe != nil {
			m.logger.Debug("unsubscribing headless log listener")
			m.unsubscribe()
		}

		m.logger.Debug("stopping runtime controller")
		m.runner.Stop()
		m.logger.Debug("runtime controller stop requested")

		m.logger.Debug("headless cleanup complete")
	})
}
