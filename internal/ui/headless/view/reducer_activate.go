package view

type ActivateEffect int

const (
	ActivateEffectNone ActivateEffect = iota
	ActivateEffectStartUploader
	ActivateEffectStopUploader
	ActivateEffectRequestQuit
	ActivateEffectOpenBrowse
	ActivateEffectSaveSettings
	ActivateEffectDebugLevelChanged
)

func ReduceActivate(state State, canConnect bool, running bool, connecting bool) (State, ActivateEffect) {
	if state.Tab == TabOverview {
		switch state.Focus {
		case state.ConnectIndex():
			if connecting {
				return state, ActivateEffectNone
			}
			if !canConnect {
				state.ErrorModalText = "Base URL and uploader token are required."
				return state, ActivateEffectNone
			}
			if running {
				return state, ActivateEffectStopUploader
			}
			return state, ActivateEffectStartUploader
		case state.LogsIndex():
			state.ShowLogs = !state.ShowLogs
			if state.ShowLogs {
				state.FollowLogs = true
				state.LogView.GotoBottom()
			}
			if !state.ShowLogs && state.Focus >= state.FocusCount() {
				state.Focus = state.FocusCount() - 1
			}
			return state, ActivateEffectNone
		case state.QuitIndex():
			return state, ActivateEffectRequestQuit
		case state.LogsDebugIndex():
			state.DebugOn = !state.DebugOn
			state.DraftSettings.Debug = state.DebugOn
			state.SettingsDirty = state.DraftSettings != state.SavedSettings
			return state, ActivateEffectDebugLevelChanged
		default:
			return state, ActivateEffectNone
		}
	}

	switch state.Focus {
	case state.BrowseIndex():
		return state, ActivateEffectOpenBrowse
	case state.AutoConnectIndex():
		state.AutoConn = !state.AutoConn
		state.DraftSettings.AutoConnect = state.AutoConn
		state.SettingsDirty = state.DraftSettings != state.SavedSettings
		return state, ActivateEffectNone
	case state.SaveIndex():
		return state, ActivateEffectSaveSettings
	case state.CancelIndex():
		if !state.SettingsDirty {
			return state, ActivateEffectNone
		}
		return state.WithCancelDraft(), ActivateEffectNone
	default:
		return state, ActivateEffectNone
	}
}
