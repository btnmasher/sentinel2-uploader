package view

import (
	"runtime"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"

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
	updateDialogWidth          = 84
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
	settingsHeightPadding      = 8
	settingsPaneMinHeight      = 16
	settingsLabelGap           = 1
	settingsRightEdgeGuard     = 1
)

func RenderApp(state *State, rt Runtime) string {
	if state.Width == 0 {
		return "initializing..."
	}

	base := renderBase(state, rt)
	if state.FilePickerOpen {
		return zone.Scan(renderModalOverlay(state, base, renderFilePickerDialog(state)))
	}

	if state.ErrorModalText != "" {
		return zone.Scan(renderModalOverlay(state, base, renderErrorDialog(state)))
	}

	if state.UpdateModalOpen {
		return zone.Scan(renderModalOverlay(state, base, renderUpdateDialog(state, rt)))
	}

	if state.ConfirmQuit {
		return zone.Scan(renderModalOverlay(state, base, renderQuitConfirmDialog(state)))
	}

	return zone.Scan(base)
}

func renderBase(state *State, rt Runtime) string {
	header := RainbowTitle("Sentinel2 Uploader ("+rt.BuildVersion+")", state.AnimPhase, state.ImGay)
	tabs := RenderTabs(state.Tab, state.HoverZone)

	var content string
	if state.Tab == TabOverview {
		content = renderOverview(state, rt)
	} else {
		content = renderSettings(state)
	}

	helpWidth := max(state.PageWidth()-frameInnerInset, minPageWidth)
	state.HelpView.Width = helpWidth
	helpText := state.HelpView.View(state.Keys)
	hints := []string{
		renderHelpHint("mouse click", "focus/activate"),
	}
	if state.Tab == TabSettings {
		hints = append([]string{renderHelpHint("ctrl+s", "save")}, hints...)
	}
	if len(hints) > 0 {
		if strings.TrimSpace(helpText) != "" {
			helpText += " " + theme.HelpStyle.Render("•") + " " + strings.Join(hints, " "+theme.HelpStyle.Render("•")+" ")
		} else {
			helpText = strings.Join(hints, " "+theme.HelpStyle.Render("•")+" ")
		}
	}
	helpText = ansi.Wrap(helpText, helpWidth, "")

	top := strings.Join([]string{header, tabs, content}, "\n")
	sections := []string{top}

	if state.Tab == TabOverview && state.ShowLogs {
		state.FitLogViewportHeight([]string{header, tabs, content, helpText}, DefaultNonLogLayoutReserveMin, DefaultMinLogPanelHeight)
		logPanel := renderLogPanel(state)
		sections = append(sections, logPanel)
	}

	sections = append(sections, theme.HelpStyle.Render(helpText))
	root := strings.Join(sections, "\n")
	return renderFrame(state, root, state.ContentWidth())
}

func renderFrame(state *State, content string, width int) string {
	return render.Frame(content, width, state.ImGay, state.AnimPhase, theme.PanelStyle)
}

func renderHelpHint(key string, description string) string {
	keyStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true).Render(key)
	descStyled := lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Render(description)
	return keyStyled + " " + descStyled
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
		layout := left + "\n" + right
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
	hovered := state.HoverZone == zoneOverviewConnect
	focused := state.Focus == state.ConnectIndex()
	if !rt.Running && !rt.Connecting && !rt.CanConnect {
		connect := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("Connect")
		disconnect := lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render("Disconnect")
		content := connect + theme.SegmentBaseStyle.Render("|") + disconnect

		if focused {
			return zone.Mark(zoneOverviewConnect, theme.ButtonDisabledFocusedStyle.Render(content))
		}
		if hovered {
			return zone.Mark(zoneOverviewConnect, theme.ButtonDisabledHoverStyle.Render(content))
		}

		return zone.Mark(zoneOverviewConnect, theme.ButtonDisabledStyle.Render(content))
	}

	if rt.Connecting {
		connecting := RainbowText("Connecting...", state.AnimPhase)
		content := theme.SegmentOnStyle.Render(connecting) + theme.SegmentBaseStyle.Render("|") + theme.SegmentOffStyle.Render("Disconnect")

		if focused {
			return zone.Mark(zoneOverviewConnect, theme.ButtonFocusedStyle.Render(content))
		}
		if hovered {
			return zone.Mark(zoneOverviewConnect, theme.ButtonHoverStyle.Render(content))
		}

		return zone.Mark(zoneOverviewConnect, theme.ButtonStyle.Render(content))
	}

	connect := theme.SegmentOffStyle.Render("Connect")
	disconnect := theme.SegmentOffStyle.Render("Disconnect")

	if rt.Running {
		disconnect = theme.SegmentOnStyle.Render("Disconnect")
	} else {
		connect = theme.SegmentOnStyle.Render("Connect")
	}

	content := connect + theme.SegmentBaseStyle.Render("|") + disconnect

	if focused {
		return zone.Mark(zoneOverviewConnect, theme.ButtonFocusedStyle.Render(content))
	}
	if hovered {
		return zone.Mark(zoneOverviewConnect, theme.ButtonHoverStyle.Render(content))
	}

	return zone.Mark(zoneOverviewConnect, theme.ButtonStyle.Render(content))
}

func renderLogsButton(state *State) string {
	label := "Logs"
	if state.ShowLogs {
		label = "Hide Logs"
	}

	if state.Focus == state.LogsIndex() {
		return zone.Mark(zoneOverviewLogs, theme.ButtonFocusedStyle.Render(label))
	}
	if state.HoverZone == zoneOverviewLogs {
		return zone.Mark(zoneOverviewLogs, theme.ButtonHoverStyle.Render(label))
	}

	return zone.Mark(zoneOverviewLogs, theme.ButtonStyle.Render(label))
}

func renderQuitButton(state *State) string {
	label := "Quit"
	if state.Focus == state.QuitIndex() {
		return zone.Mark(zoneOverviewQuit, theme.ButtonFocusedStyle.Render(label))
	}
	if state.HoverZone == zoneOverviewQuit {
		return zone.Mark(zoneOverviewQuit, theme.ButtonHoverStyle.Render(label))
	}

	return zone.Mark(zoneOverviewQuit, theme.ButtonStyle.Render(label))
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
	panelWidth := settingsPanelWidth(state)
	labels := []string{"Base URL", "Token", "Log Dir"}
	labelWidth := settingsLabelWidth
	rows := make([]string, 0, len(state.Inputs)+settingsRowExtraCapacity)
	controlWidth := max(state.SettingsView.Width-labelWidth-settingsLabelGap-settingsRightEdgeGuard, settingsControlMinWidth)
	for i := range state.Inputs {
		label := labels[i]
		if state.Focus == i {
			label = theme.FocusStyle.Render(label)
		}
		state.Inputs[i].Width = controlWidth
		inputView := zone.Mark(zoneSettingsInput(i), state.Inputs[i].View())
		rows = append(rows, renderSettingsLabelRow(label+":", inputView, labelWidth))
	}

	browseButton := theme.ButtonStyle.Render("Choose Folder")
	if state.Focus == state.BrowseIndex() {
		browseButton = theme.ButtonFocusedStyle.Render("Choose Folder")
	} else if state.HoverZone == zoneSettingsBrowse {
		browseButton = theme.ButtonHoverStyle.Render("Choose Folder")
	}
	browseButton = zone.Mark(zoneSettingsBrowse, browseButton)

	browseLine := lipgloss.NewStyle().PaddingLeft(settingsBrowsePaddingLeft).Render(browseButton)
	rows = append(rows, browseLine)
	auto := "[ ] Auto-connect"
	if state.AutoConn {
		auto = "[x] Auto-connect"
	}

	autoLabel := "Auto"
	autoHover := state.HoverZone == zoneSettingsAutoConnect
	autoControl := theme.ButtonStyle.Render(auto)
	if state.Focus == state.AutoConnectIndex() {
		autoLabel = theme.FocusStyle.Render("-> Auto")
		autoControl = theme.ButtonFocusedStyle.Render(auto)
	} else if autoHover {
		autoControl = theme.ButtonHoverStyle.Render(auto)
	}
	rows = append(rows, renderSettingsLabelRow(autoLabel+":", "", labelWidth))
	rows = append(rows, lipgloss.NewStyle().PaddingLeft(settingsBrowsePaddingLeft).Render(zone.Mark(zoneSettingsAutoConnect, autoControl)))
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

	if state.HoverZone == zoneSettingsSave {
		if state.SettingsDirty {
			saveLabel = theme.ButtonHoverStyle.Render("Save")
		} else {
			saveLabel = theme.ButtonDisabledHoverStyle.Render("Save")
		}
	}
	if state.HoverZone == zoneSettingsCancel {
		if state.SettingsDirty {
			cancelLabel = theme.ButtonHoverStyle.Render("Cancel")
		} else {
			cancelLabel = theme.ButtonDisabledHoverStyle.Render("Cancel")
		}
	}

	saveLabel = zone.Mark(zoneSettingsSave, saveLabel)
	cancelLabel = zone.Mark(zoneSettingsCancel, cancelLabel)
	rows = append(rows, "")
	rows = append(rows, lipgloss.JoinHorizontal(lipgloss.Left, saveLabel, " ", cancelLabel))
	if state.SettingsDirty {
		rows = append(rows, theme.HelpStyle.Render("unsaved changes"))
	}
	settingsContent := strings.Join(rows, "\n")
	settingsContent = fitTextToWidth(settingsContent, state.SettingsView.Width)
	state.SettingsView.SetContent(settingsContent)

	return renderFrame(state, state.SettingsView.View(), panelWidth)
}

func renderSettingsLabelRow(label string, control string, labelWidth int) string {
	labelCell := lipgloss.NewStyle().Width(labelWidth).Render(label)
	if control == "" {
		return labelCell
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, labelCell, strings.Repeat(" ", settingsLabelGap), control)
}

func fitTextToWidth(text string, width int) string {
	if width <= 0 {
		return text
	}
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		fitted := ansi.Cut(line, 0, width)
		if pad := width - ansi.StringWidth(fitted); pad > 0 {
			fitted += strings.Repeat(" ", pad)
		}
		out = append(out, fitted)
	}
	return strings.Join(out, "\n")
}

func renderLogPanel(state *State) string {
	check := "[ ] Debug"
	if state.DebugOn {
		check = "[x] Debug"
	}

	debug := theme.ButtonStyle.Render(check)
	if state.Focus == state.LogsDebugIndex() {
		debug = theme.ButtonFocusedStyle.Render(check)
	} else if state.HoverZone == zoneOverviewLogsDebug {
		debug = theme.ButtonHoverStyle.Render(check)
	}
	debug = zone.Mark(zoneOverviewLogsDebug, debug)

	toolbar := lipgloss.JoinHorizontal(
		lipgloss.Center,
		theme.TitleStyle.Render("Logs"),
		"  ",
		debug,
		"  ",
		renderHelpHint("wheel", "scroll"),
	)
	content := state.LogView.View()
	withBar := WithScrollBar(content, state.LogView.Width, state.LogView.Height, state.LogView.ScrollPercent())

	return renderFrame(state, toolbar+"\n"+withBar, state.PageWidth())
}

func renderQuitConfirmDialog(state *State) string {
	cancelButton := theme.ButtonStyle.Render("Cancel")
	quitButton := theme.ButtonStyle.Render("Quit")
	if state.ConfirmQuitChoice == 0 {
		cancelButton = theme.ButtonFocusedStyle.Render("Cancel")
	}
	if state.HoverZone == zoneDialogQuitCancel {
		cancelButton = theme.ButtonHoverStyle.Render("Cancel")
	}
	if state.ConfirmQuitChoice == 1 {
		quitButton = theme.ButtonFocusedStyle.Render("Quit")
	}
	if state.HoverZone == zoneDialogQuitAccept {
		quitButton = theme.ButtonHoverStyle.Render("Quit")
	}
	cancelButton = zone.Mark(zoneDialogQuitCancel, cancelButton)
	quitButton = zone.Mark(zoneDialogQuitAccept, quitButton)

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

func renderUpdateDialog(state *State, rt Runtime) string {
	laterButton := theme.ButtonStyle.Render("Later")
	openButton := theme.ButtonStyle.Render("Open Release")
	if state.UpdateModalChoice == UpdateChoiceLater {
		laterButton = theme.ButtonFocusedStyle.Render("Later")
	}
	if state.HoverZone == zoneDialogUpdateLater {
		laterButton = theme.ButtonHoverStyle.Render("Later")
	}
	if state.UpdateModalChoice == 1 {
		openButton = theme.ButtonFocusedStyle.Render("Open Release")
	}
	if state.HoverZone == zoneDialogUpdateOpen {
		openButton = theme.ButtonHoverStyle.Render("Open Release")
	}
	laterButton = zone.Mark(zoneDialogUpdateLater, laterButton)
	openButton = zone.Mark(zoneDialogUpdateOpen, openButton)

	buttonRow := lipgloss.JoinHorizontal(lipgloss.Top, laterButton, "  ", openButton)
	dialogWidth := min(state.ContentWidth()-dialogHorizontalInset, updateDialogWidth)
	buttonLine := lipgloss.NewStyle().
		Width(max(dialogWidth-frameInnerInset, 1)).
		AlignHorizontal(lipgloss.Center).
		Render(buttonRow)

	url := strings.TrimSpace(state.UpdateReleaseURL)
	if url == "" {
		url = "https://github.com/btnmasher/sentinel2-uploader/releases/latest"
	}
	body := strings.Join([]string{
		theme.TitleStyle.Render("Update Available"),
		"A newer uploader version is available (" + state.UpdateLatestTag + ").",
		"Current version: " + strings.TrimSpace(rt.BuildVersion),
		"",
		url,
		"",
		buttonLine,
		theme.HelpStyle.Render("tab/arrow switch • enter confirms"),
	}, "\n")

	return renderFrame(state, body, dialogWidth)
}

func renderFilePickerDialog(state *State) string {
	title := theme.TitleStyle.Render("Select Log Directory")
	picker := state.FilePicker.View()
	help := theme.HelpStyle.Render("up/down move • space open • enter select • left/backspace up • esc close")
	body := strings.Join([]string{title, picker, help}, "\n")

	return renderFrame(state, body, min(state.PageWidth(), filePickerDialogMaxWidth))
}

func renderModalOverlay(state *State, base string, dialog string) string {
	_ = base
	return lipgloss.Place(state.Width, state.Height, lipgloss.Center, lipgloss.Center, dialog)
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
	settingsTotal := settingsPanelWidth(state)
	leftW := overviewLeftFrameWidth(state, rt, total)
	leftWidth, rightWidth, stacked := overviewPaneLayout(total, leftW)
	if stacked {
		rightWidth = total
	}

	leftInner := max(leftWidth-frameInnerInset, paneInnerMinWidth)
	rightInner := max(rightWidth-frameInnerInset, paneInnerMinWidth)
	settingsInner := max(settingsTotal-frameInnerInset, paneInnerMinWidth)
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

func settingsPanelWidth(state *State) int {
	width := state.PageWidth()
	if runtime.GOOS == "windows" && width > minPageWidth {
		width--
	}
	return max(width, minPageWidth)
}
