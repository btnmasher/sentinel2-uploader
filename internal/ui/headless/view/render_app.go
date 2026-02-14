package view

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"sentinel2-uploader/internal/ui/headless/health"
	"sentinel2-uploader/internal/ui/headless/render"
	"sentinel2-uploader/internal/ui/headless/theme"
)

type Runtime struct {
	BuildVersion string
	Running      bool
	Connecting   bool
	Status       string
	StatusKind   int
	CanConnect   bool
	Channels     []health.Row
	HealthDetail string
}

const (
	outerPaneGap               = 2
	frameInnerInset            = 4
	minOverviewLeftContent     = 12
	minOverviewLeftHeight      = 6
	minOverviewRemainingWidth  = 24
	channelPaneMinWidth        = 8
	channelPaneMinHeight       = 3
	channelListMinWidth        = 10
	settingsLabelWidth         = 9
	settingsRowExtraCapacity   = 5
	settingsControlMinWidth    = 16
	settingsBrowsePaddingLeft  = settingsLabelWidth + 1
	dialogHorizontalInset      = 8
	quitDialogWidth            = 72
	errorDialogWidth           = 78
	filePickerDialogMaxWidth   = 96
	leftFrameExtraWidth        = 6
	leftFrameMinWidth          = 24
	rightPaneMinWidth          = 32
	sideBySideMinTotalWidth    = 84
	paneInnerMinWidth          = 1
	defaultOverviewPaneHeight  = 8
	largeOverviewPaneHeight    = 10
	largeOverviewHeightCutover = 36
	settingsHeightPadding      = 4
	settingsPaneMinHeight      = 12
)

func RenderApp(state *State, rt Runtime) string {
	if state.Width == 0 {
		return "initializing..."
	}

	base := renderBase(state, rt)
	if state.FilePickerOpen {
		return renderModalOverlay(state, base, renderFilePickerDialog(state))
	}

	if state.ErrorModalText != "" {
		return renderModalOverlay(state, base, renderErrorDialog(state))
	}

	if state.ConfirmQuit {
		return renderModalOverlay(state, base, renderQuitConfirmDialog(state))
	}

	return base
}

func renderBase(state *State, rt Runtime) string {
	header := theme.TitleStyle.Render("Sentinel2 Uploader (" + rt.BuildVersion + ")")
	tabs := RenderTabs(state.Tab == TabOverview)

	var content string
	if state.Tab == TabOverview {
		content = renderOverview(state, rt)
	} else {
		content = renderSettings(state)
	}

	helpText := state.HelpView.View(state.Keys)
	if state.Tab == TabSettings {
		helpText += " • ctrl+s save"
	}

	sections := []string{header, tabs, content}

	if state.Tab == TabOverview && state.ShowLogs {
		state.FitLogViewportHeight([]string{header, tabs, content, helpText}, DefaultNonLogLayoutReserveMin, DefaultMinLogPanelHeight)
		logPanel := renderLogPanel(state)
		sections = append(sections, logPanel)
	}

	sections = append(sections, theme.HelpStyle.Render(helpText))
	root := strings.Join(sections, "\n\n")
	return renderFrame(state, root, state.ContentWidth())
}

func renderFrame(state *State, content string, width int) string {
	return render.Frame(content, width, state.ImGay, state.AnimPhase, theme.PanelStyle)
}

func renderOverview(state *State, rt Runtime) string {
	total := state.PageWidth()
	gap := outerPaneGap
	ResizePaneViewports(state, rt)

	leftWidth, rightWidth, stacked := overviewPaneLayout(total, overviewLeftFrameWidth(state, rt, total))
	statusLine := "Status: " + RenderStatus(rt.Status, rt.StatusKind)
	leftRenderWidth := leftWidth

	if stacked {
		leftRenderWidth = total
	}

	leftContentWidth := leftRenderWidth - frameInnerInset

	if leftContentWidth <= 0 {
		leftContentWidth = state.LeftView.Width
	}

	if leftContentWidth > outerPaneGap {
		leftContentWidth -= outerPaneGap
	}

	leftContentWidth = max(leftContentWidth, minOverviewLeftContent)
	state.LeftView.Width = leftContentWidth
	actionsLine := renderActionsRowState(state, rt, leftContentWidth)
	requiredLeftHeight := max(1+outerPaneGap+lipgloss.Height(actionsLine), minOverviewLeftHeight)

	if state.LeftView.Height < requiredLeftHeight {
		state.LeftView.Height = requiredLeftHeight
	}

	if !stacked && state.RightView.Height < requiredLeftHeight {
		state.RightView.Height = requiredLeftHeight
	}

	state.LeftView.SetContent(strings.Join([]string{statusLine, actionsLine}, "\n\n"))
	left := renderFrame(state, state.LeftView.View(), leftRenderWidth)

	if stacked {
		state.RightView.SetContent(renderChannelPanelBody(state, rt, total-frameInnerInset, state.RightView.Height))
		right := renderFrame(state, state.RightView.View(), rightWidth)
		layout := left + "\n\n" + right
		return lipgloss.NewStyle().Width(total).Render(layout)
	}

	remaining := max(total-lipgloss.Width(left)-gap, minOverviewRemainingWidth)
	state.RightView.SetContent(renderChannelPanelBody(state, rt, remaining-frameInnerInset, state.RightView.Height))
	right := renderFrame(state, state.RightView.View(), remaining)
	layout := lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", gap), right)

	return lipgloss.NewStyle().Width(total).Render(layout)
}

func renderActionsRowState(state *State, rt Runtime, maxWidth int) string {
	segments := []string{renderConnectToggle(state, rt), renderLogsButton(state), renderQuitButton(state)}

	return RenderActionsRow(segments, maxWidth)
}

func renderConnectToggle(state *State, rt Runtime) string {
	if !rt.Running && !rt.Connecting && !rt.CanConnect {
		connect := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("Connect")
		disconnect := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("Disconnect")
		content := connect + theme.SegmentBaseStyle.Render("|") + disconnect

		if state.Focus == state.ConnectIndex() {
			return theme.ButtonDisabledFocusedStyle.Render(content)
		}

		return theme.ButtonDisabledStyle.Render(content)
	}

	if rt.Connecting {
		connecting := RainbowText("Connecting...", state.AnimPhase)
		content := theme.SegmentOnStyle.Render(connecting) + theme.SegmentBaseStyle.Render("|") + theme.SegmentOffStyle.Render("Disconnect")

		if state.Focus == state.ConnectIndex() {
			return theme.ButtonFocusedStyle.Render(content)
		}

		return theme.ButtonStyle.Render(content)
	}

	connect := theme.SegmentOffStyle.Render("Connect")
	disconnect := theme.SegmentOffStyle.Render("Disconnect")

	if rt.Running {
		disconnect = theme.SegmentOnStyle.Render("Disconnect")
	} else {
		connect = theme.SegmentOnStyle.Render("Connect")
	}

	content := connect + theme.SegmentBaseStyle.Render("|") + disconnect

	if state.Focus == state.ConnectIndex() {
		return theme.ButtonFocusedStyle.Render(content)
	}

	return theme.ButtonStyle.Render(content)
}

func renderLogsButton(state *State) string {
	label := "Logs"
	if state.ShowLogs {
		label = "Hide Logs"
	}

	if state.Focus == state.LogsIndex() {
		return theme.ButtonFocusedStyle.Render(label)
	}

	return theme.ButtonStyle.Render(label)
}

func renderQuitButton(state *State) string {
	label := "Quit"
	if state.Focus == state.QuitIndex() {
		return theme.ButtonFocusedStyle.Render(label)
	}

	return theme.ButtonStyle.Render(label)
}

func renderChannelPanelBody(state *State, rt Runtime, width int, height int) string {
	header := theme.TitleStyle.Render("Configured Channels")
	if !rt.Running && !rt.Connecting {
		placeholder := "Not connected"
		width = max(width, channelPaneMinWidth)
		height = max(height, channelPaneMinHeight)
		content := lipgloss.NewStyle().Width(width).Height(height - 1).AlignHorizontal(lipgloss.Center).AlignVertical(lipgloss.Center).Foreground(lipgloss.Color("245")).Render(placeholder)
		return header + "\n" + content
	}

	if len(rt.Channels) == 0 {
		placeholder := "No channels configured"
		if rt.HealthDetail != "" {
			placeholder = rt.HealthDetail
		}
		width = max(width, channelPaneMinWidth)
		height = max(height, channelPaneMinHeight)
		content := lipgloss.NewStyle().Width(width).Height(height - 1).AlignHorizontal(lipgloss.Center).AlignVertical(lipgloss.Center).Foreground(lipgloss.Color("245")).Render(placeholder)
		return header + "\n" + content
	}

	lines := make([]string, 0, len(rt.Channels))
	width = max(width, channelListMinWidth)
	for _, row := range rt.Channels {
		dot, style := ChannelDotStyle(row.Kind)
		prefix := style.Render(dot) + " "
		availableName := max(width-ansi.StringWidth(prefix), 1)
		name := render.TruncateDisplayWidth(row.Name, availableName)
		lines = append(lines, prefix+name)
	}

	body := strings.Join(lines, "\n")
	if rt.HealthDetail != "" {
		body += "\n" + theme.HelpStyle.Render(rt.HealthDetail)
	}

	return header + "\n" + body
}

func renderSettings(state *State) string {
	labels := []string{"Base URL", "Token", "Log Dir"}
	labelWidth := settingsLabelWidth
	rows := make([]string, 0, len(state.Inputs)+settingsRowExtraCapacity)
	controlWidth := max(state.SettingsView.Width-labelWidth-outerPaneGap, settingsControlMinWidth)
	for i := range state.Inputs {
		label := labels[i]
		if state.Focus == i {
			label = theme.FocusStyle.Render("-> " + label)
		}
		state.Inputs[i].Width = controlWidth
		rows = append(rows, fmt.Sprintf("%-*s %s", labelWidth, label+":", state.Inputs[i].View()))
	}

	browseButton := theme.ButtonStyle.Render("Choose Folder")
	if state.Focus == state.BrowseIndex() {
		browseButton = theme.ButtonFocusedStyle.Render("Choose Folder")
	}

	browseLine := lipgloss.NewStyle().PaddingLeft(settingsBrowsePaddingLeft).Render(browseButton)
	rows = append(rows, browseLine)
	auto := "[ ] Auto-connect"
	if state.AutoConn {
		auto = "[x] Auto-connect"
	}

	autoLabel := "Auto"
	if state.Focus == state.AutoConnectIndex() {
		autoLabel = theme.FocusStyle.Render("-> Auto")
	}
	rows = append(rows, fmt.Sprintf("%-*s %s", labelWidth, autoLabel+":", auto))
	saveLabel := "Save"
	cancelLabel := "Cancel"
	if state.SettingsDirty {
		saveLabel = theme.ButtonStyle.Render("Save")
		cancelLabel = theme.ButtonStyle.Render("Cancel")
	} else {
		saveLabel = theme.ButtonDisabledStyle.Render("Save")
		cancelLabel = theme.ButtonDisabledStyle.Render("Cancel")
	}

	if state.Focus == state.SaveIndex() {
		if state.SettingsDirty {
			saveLabel = theme.ButtonFocusedStyle.Render("Save")
		} else {
			saveLabel = theme.ButtonDisabledFocusedStyle.Render("Save")
		}
	}

	if state.Focus == state.CancelIndex() {
		if state.SettingsDirty {
			cancelLabel = theme.ButtonFocusedStyle.Render("Cancel")
		} else {
			cancelLabel = theme.ButtonDisabledFocusedStyle.Render("Cancel")
		}
	}

	rows = append(rows, "")
	rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Left, saveLabel, " ", cancelLabel))
	if state.SettingsDirty {
		rows = append(rows, theme.HelpStyle.Render("unsaved changes"))
	}

	state.SettingsView.SetContent(strings.Join(rows, "\n"))

	return renderFrame(state, state.SettingsView.View(), state.PageWidth())
}

func renderLogPanel(state *State) string {
	check := "[ ] Debug"
	if state.DebugOn {
		check = "[x] Debug"
	}

	debug := theme.ButtonStyle.Render(check)
	if state.Focus == state.LogsDebugIndex() {
		debug = theme.ButtonFocusedStyle.Render(check)
	}

	followHint := theme.HelpStyle.Render("ctrl+f follow")
	toolbar := lipgloss.JoinHorizontal(lipgloss.Center, theme.TitleStyle.Render("Logs"), "  ", debug, "  ", followHint)
	content := state.LogView.View()
	withBar := WithScrollBar(content, state.LogView.Width, state.LogView.Height, state.LogView.ScrollPercent())

	return renderFrame(state, toolbar+"\n"+withBar, state.PageWidth())
}

func renderQuitConfirmDialog(state *State) string {
	cancelButton := theme.ButtonStyle.Render("Cancel")
	quitButton := theme.ButtonStyle.Render("Quit")
	if state.ConfirmQuitChoice == 0 {
		cancelButton = theme.ButtonFocusedStyle.Render("Cancel")
	} else {
		quitButton = theme.ButtonFocusedStyle.Render("Quit")
	}

	buttonRow := lipgloss.JoinHorizontal(lipgloss.Top, cancelButton, "  ", quitButton)
	dialogWidth := min(state.ContentWidth()-dialogHorizontalInset, quitDialogWidth)
	buttonLine := lipgloss.NewStyle().
		Width(max(dialogWidth-frameInnerInset, 1)).
		AlignHorizontal(lipgloss.Center).
		Render(buttonRow)

	body := strings.Join([]string{
		theme.TitleStyle.Render("Quit while connected?"),
		"This will stop the uploader connection.",
		buttonLine,
		theme.HelpStyle.Render("tab/arrow switch • enter confirms"),
	}, "\n")

	return renderFrame(state, body, dialogWidth)
}

func renderErrorDialog(state *State) string {
	body := strings.Join([]string{
		theme.ErrorStyle.Render("Error"),
		state.ErrorModalText,
		theme.HelpStyle.Render("Press Enter or Esc to close"),
	}, "\n")

	return renderFrame(state, body, min(state.ContentWidth()-dialogHorizontalInset, errorDialogWidth))
}

func renderFilePickerDialog(state *State) string {
	title := theme.TitleStyle.Render("Select Log Directory")
	picker := state.FilePicker.View()
	help := theme.HelpStyle.Render("up/down move • space open • enter select • left/backspace up • esc close")
	body := strings.Join([]string{title, picker, help}, "\n")

	return renderFrame(state, body, min(state.PageWidth(), filePickerDialogMaxWidth))
}

func renderModalOverlay(state *State, base string, dialog string) string {
	faded := theme.ModalBackdrop.Render(base)
	overlay := lipgloss.Place(state.Width, state.Height, lipgloss.Center, lipgloss.Center, dialog)

	return faded + "\n" + overlay
}

func overviewLeftFrameWidth(state *State, rt Runtime, total int) int {
	statusLine := "Status: " + RenderStatus(rt.Status, rt.StatusKind)
	actionsLine := renderActionsRowState(state, rt, 10_000)
	leftInner := max(lipgloss.Width(statusLine), lipgloss.Width(actionsLine))
	leftInner = max(leftInner, actionsRowPreferredWidth(state, rt))
	leftWidth := max(leftInner+leftFrameExtraWidth, leftFrameMinWidth)
	if leftWidth > total {
		leftWidth = total
	}

	return leftWidth
}

func actionsRowPreferredWidth(state *State, rt Runtime) int {
	connect := renderConnectToggle(state, rt)
	logs := theme.ButtonStyle.Render("Hide Logs")
	quit := theme.ButtonStyle.Render("Quit")
	row := lipgloss.JoinHorizontal(lipgloss.Top, connect, " ", logs, " ", quit)

	return lipgloss.Width(row)
}

func overviewPaneLayout(total int, leftWidth int) (int, int, bool) {
	gap := outerPaneGap
	minRightWidth := rightPaneMinWidth
	rightWidth := total - leftWidth - gap
	if total < sideBySideMinTotalWidth || rightWidth < minRightWidth {
		return leftWidth, total, true
	}

	return leftWidth, rightWidth, false
}

func ResizePaneViewports(state *State, rt Runtime) {
	total := state.PageWidth()
	leftW := overviewLeftFrameWidth(state, rt, total)
	leftWidth, rightWidth, stacked := overviewPaneLayout(total, leftW)
	if stacked {
		rightWidth = total
	}

	leftInner := max(leftWidth-frameInnerInset, paneInnerMinWidth)
	rightInner := max(rightWidth-frameInnerInset, paneInnerMinWidth)
	settingsInner := max(total-frameInnerInset, paneInnerMinWidth)
	paneHeight := defaultOverviewPaneHeight
	if state.Height >= largeOverviewHeightCutover {
		paneHeight = largeOverviewPaneHeight
	}

	state.LeftView.Width = leftInner
	state.LeftView.Height = paneHeight
	state.RightView.Width = rightInner
	state.RightView.Height = paneHeight
	state.SettingsView.Width = settingsInner
	state.SettingsView.Height = max(settingsPaneMinHeight, paneHeight+settingsHeightPadding)
}
