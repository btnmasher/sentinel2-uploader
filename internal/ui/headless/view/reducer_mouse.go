package view

import tea "github.com/charmbracelet/bubbletea"

func ReduceMouse(state State, msg tea.MouseMsg) (State, tea.Cmd) {
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
	if state.ShowLogs {
		var cmd tea.Cmd
		state.LogView, cmd = state.LogView.Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
		state.FollowLogs = state.LogView.AtBottom()
	}
	return state, tea.Batch(cmds...)
}
