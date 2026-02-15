package view

import (
	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
)

type MouseEffect int

const (
	MouseEffectNone MouseEffect = iota
	MouseEffectActivateFocused
	MouseEffectConfirmQuitAccept
)

func ReduceMouse(state State, msg tea.MouseMsg) (State, tea.Cmd, MouseEffect) {
	var cmds []tea.Cmd
	if state.Tab == TabOverview {
		var cmd tea.Cmd
		state.RightView, cmd = state.RightView.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		state.LeftView, cmd = state.LeftView.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if state.Tab == TabSettings {
		var cmd tea.Cmd
		state.SettingsView, cmd = state.SettingsView.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if state.ShowLogs {
		var cmd tea.Cmd
		state.LogView, cmd = state.LogView.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		state.FollowLogs = state.LogView.AtBottom()
	}

	state.HoverZone = hoveredZone(state, msg)

	isPrimaryClick := msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft
	if !isPrimaryClick {
		return state, tea.Batch(cmds...), MouseEffectNone
	}

	if state.ConfirmQuit {
		switch {
		case inBounds(zoneDialogQuitCancel, msg):
			state.ConfirmQuitChoice = ConfirmQuitChoiceCancel
			state.ConfirmQuit = false
			return state, tea.Batch(cmds...), MouseEffectNone
		case inBounds(zoneDialogQuitAccept, msg):
			state.ConfirmQuitChoice = confirmChoiceQuit
			return state, tea.Batch(cmds...), MouseEffectConfirmQuitAccept
		default:
			return state, tea.Batch(cmds...), MouseEffectNone
		}
	}

	if inBounds(zoneTabOverview, msg) {
		state.Tab = TabOverview
		state.Focus = 0
		state.ApplyFocus()
		return state, tea.Batch(cmds...), MouseEffectNone
	}
	if inBounds(zoneTabSettings, msg) {
		state.Tab = TabSettings
		state.Focus = 0
		state.ApplyFocus()
		return state, tea.Batch(cmds...), MouseEffectNone
	}

	if state.Tab == TabOverview {
		switch {
		case inBounds(zoneOverviewConnect, msg):
			state.Focus = state.ConnectIndex()
			state.ApplyFocus()
			return state, tea.Batch(cmds...), MouseEffectActivateFocused
		case inBounds(zoneOverviewLogs, msg):
			state.Focus = state.LogsIndex()
			state.ApplyFocus()
			return state, tea.Batch(cmds...), MouseEffectActivateFocused
		case inBounds(zoneOverviewQuit, msg):
			state.Focus = state.QuitIndex()
			state.ApplyFocus()
			return state, tea.Batch(cmds...), MouseEffectActivateFocused
		case inBounds(zoneOverviewLogsDebug, msg):
			state.Focus = state.LogsDebugIndex()
			state.ApplyFocus()
			return state, tea.Batch(cmds...), MouseEffectActivateFocused
		default:
			return state, tea.Batch(cmds...), MouseEffectNone
		}
	}

	for i := range state.Inputs {
		if inBounds(zoneSettingsInput(i), msg) {
			state.Focus = i
			state.Inputs[i].CursorEnd()
			state.ApplyFocus()
			return state, tea.Batch(cmds...), MouseEffectNone
		}
	}

	switch {
	case inBounds(zoneSettingsBrowse, msg):
		state.Focus = state.BrowseIndex()
		state.ApplyFocus()
		return state, tea.Batch(cmds...), MouseEffectActivateFocused
	case inBounds(zoneSettingsAutoConnect, msg):
		state.Focus = state.AutoConnectIndex()
		state.ApplyFocus()
		return state, tea.Batch(cmds...), MouseEffectActivateFocused
	case inBounds(zoneSettingsSave, msg):
		state.Focus = state.SaveIndex()
		state.ApplyFocus()
		return state, tea.Batch(cmds...), MouseEffectActivateFocused
	case inBounds(zoneSettingsCancel, msg):
		state.Focus = state.CancelIndex()
		state.ApplyFocus()
		return state, tea.Batch(cmds...), MouseEffectActivateFocused
	default:
		return state, tea.Batch(cmds...), MouseEffectNone
	}
}

func inBounds(zoneID string, msg tea.MouseMsg) bool {
	info := zone.Get(zoneID)
	if info == nil || info.IsZero() {
		return false
	}
	return info.InBounds(msg)
}

func hoveredZone(state State, msg tea.MouseMsg) string {
	if state.ConfirmQuit {
		if inBounds(zoneDialogQuitCancel, msg) {
			return zoneDialogQuitCancel
		}
		if inBounds(zoneDialogQuitAccept, msg) {
			return zoneDialogQuitAccept
		}
		return ""
	}

	if inBounds(zoneTabOverview, msg) {
		return zoneTabOverview
	}
	if inBounds(zoneTabSettings, msg) {
		return zoneTabSettings
	}

	if state.Tab == TabOverview {
		if inBounds(zoneOverviewConnect, msg) {
			return zoneOverviewConnect
		}
		if inBounds(zoneOverviewLogs, msg) {
			return zoneOverviewLogs
		}
		if inBounds(zoneOverviewQuit, msg) {
			return zoneOverviewQuit
		}
		if inBounds(zoneOverviewLogsDebug, msg) {
			return zoneOverviewLogsDebug
		}
		return ""
	}

	for i := range state.Inputs {
		id := zoneSettingsInput(i)
		if inBounds(id, msg) {
			return id
		}
	}
	if inBounds(zoneSettingsBrowse, msg) {
		return zoneSettingsBrowse
	}
	if inBounds(zoneSettingsAutoConnect, msg) {
		return zoneSettingsAutoConnect
	}
	if inBounds(zoneSettingsSave, msg) {
		return zoneSettingsSave
	}
	if inBounds(zoneSettingsCancel, msg) {
		return zoneSettingsCancel
	}
	return ""
}
