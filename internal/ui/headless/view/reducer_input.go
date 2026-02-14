package view

import tea "github.com/charmbracelet/bubbletea"

func ReduceInput(state State, msg tea.Msg) (State, tea.Cmd, bool) {
	if state.Tab != TabSettings || state.Focus >= len(state.Inputs) {
		return state, nil, false
	}
	updated, cmd := state.Inputs[state.Focus].Update(msg)
	state.Inputs[state.Focus] = updated
	return state.WithDraftFromControls(), cmd, true
}
