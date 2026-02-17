package view

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

type KeyEffect int

const (
	KeyEffectNone KeyEffect = iota
	KeyEffectRequestQuit
	KeyEffectActivateFocused
	KeyEffectSaveSettings
	KeyEffectConfirmQuitAccept
	KeyEffectUpdateAccept
)

const confirmChoiceCount = 2

const confirmChoiceQuit = 1
const updateChoiceOpen = 1

func ReduceKey(state State, msg tea.KeyMsg) (State, KeyEffect) {
	if state.ErrorModalText != "" {
		if msg.String() == "esc" || key.Matches(msg, state.Keys.Activate) {
			state.ErrorModalText = ""
		}
		return state, KeyEffectNone
	}

	if state.UpdateModalOpen {
		switch {
		case msg.String() == "esc":
			state.UpdateModalOpen = false
			return state, KeyEffectNone
		case key.Matches(msg, state.Keys.ModalToggle):
			state.UpdateModalChoice = (state.UpdateModalChoice + 1) % confirmChoiceCount
			return state, KeyEffectNone
		case key.Matches(msg, state.Keys.Activate):
			if state.UpdateModalChoice == updateChoiceOpen {
				state.UpdateModalOpen = false
				return state, KeyEffectUpdateAccept
			}
			state.UpdateModalOpen = false
			return state, KeyEffectNone
		default:
			return state, KeyEffectNone
		}
	}

	if state.ConfirmQuit {
		switch {
		case msg.String() == "esc":
			state.ConfirmQuit = false
			return state, KeyEffectNone
		case key.Matches(msg, state.Keys.ModalToggle):
			state.ConfirmQuitChoice = (state.ConfirmQuitChoice + 1) % confirmChoiceCount
			return state, KeyEffectNone
		case key.Matches(msg, state.Keys.Activate):
			if state.ConfirmQuitChoice == confirmChoiceQuit {
				return state, KeyEffectConfirmQuitAccept
			}
			state.ConfirmQuit = false
			return state, KeyEffectNone
		default:
			return state, KeyEffectNone
		}
	}

	switch {
	case key.Matches(msg, state.Keys.Quit):
		return state, KeyEffectRequestQuit
	case msg.String() == "ctrl+f" && state.Tab == TabOverview && state.ShowLogs:
		state.FollowLogs = true
		state.LogView.GotoBottom()
		return state, KeyEffectNone
	case msg.String() == "ctrl+s" && state.Tab == TabSettings:
		return state, KeyEffectSaveSettings
	case key.Matches(msg, state.Keys.PrevTab):
		state.Tab = TabOverview
		state.Focus = 0
		state.ApplyFocus()
		return state, KeyEffectNone
	case key.Matches(msg, state.Keys.NextTab):
		state.Tab = TabSettings
		state.Focus = 0
		state.ApplyFocus()
		return state, KeyEffectNone
	case key.Matches(msg, state.Keys.NextFocus):
		state.Focus = (state.Focus + 1) % state.FocusCount()
		state.ApplyFocus()
		return state, KeyEffectNone
	case key.Matches(msg, state.Keys.PrevFocus):
		state.Focus = (state.Focus + state.FocusCount() - 1) % state.FocusCount()
		state.ApplyFocus()
		return state, KeyEffectNone
	case key.Matches(msg, state.Keys.Activate):
		if state.Tab == TabOverview || state.Focus >= len(state.Inputs) {
			return state, KeyEffectActivateFocused
		}
	}

	return state, KeyEffectNone
}
