package view

import (
	"path/filepath"
	"runtime"
	"strings"

	"sentinel2-uploader/internal/ui/headless/theme"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	TabOverview = iota
	TabSettings
)

const (
	DefaultNonLogLayoutReserveMin = 24
	DefaultMinLogPanelHeight      = 8
	ConfirmQuitChoiceCancel       = 0
)

const (
	connectControlIndex = iota
	logsControlIndex
	quitControlIndex
	logsDebugControlIndex
)

const (
	overviewFocusCountWithoutLogs = 3
	overviewFocusCountWithLogs    = 4
	settingsExtraFocusSlots       = 4
)

const (
	minPageWidth = 24
)

const (
	logPanelHorizontalInset = 8
	minViewportDimension    = 1
	minLogViewportWidth     = 20
	logViewportHeightOffset = 3
	minLogViewportHeight    = 3
	panelFrameOverhead      = 4
	filePickerHeightOffset  = 14
	minFilePickerHeight     = 8
	borderRows              = 2
	sectionGapRows          = 2
)

func (s *State) ApplyFocus() {
	for i := range s.Inputs {
		if s.Tab == TabSettings && i == s.Focus {
			s.Inputs[i].Focus()
		} else {
			s.Inputs[i].Blur()
		}
	}
}

func (s State) FocusCount() int {
	if s.Tab == TabOverview {
		if s.ShowLogs {
			return overviewFocusCountWithLogs
		}
		return overviewFocusCountWithoutLogs
	}
	return len(s.Inputs) + settingsExtraFocusSlots
}

func (s State) LogsIndex() int        { return logsControlIndex }
func (s State) QuitIndex() int        { return quitControlIndex }
func (s State) LogsDebugIndex() int   { return logsDebugControlIndex }
func (s State) AutoConnectIndex() int { return len(s.Inputs) + logsControlIndex }
func (s State) BrowseIndex() int      { return len(s.Inputs) }
func (s State) SaveIndex() int        { return len(s.Inputs) + quitControlIndex }
func (s State) CancelIndex() int      { return len(s.Inputs) + logsDebugControlIndex }
func (s State) ConnectIndex() int     { return connectControlIndex }

func (s State) ContentWidth() int {
	width := max(s.Width, 1)
	// Some Windows terminals wrap when a styled line lands exactly on the
	// reported last column; keep one-column headroom to avoid right-edge drift.
	if runtime.GOOS == "windows" && width > 1 {
		width--
	}
	return width
}

func (s State) PageWidth() int {
	return max(s.ContentWidth()-theme.PanelStyle.GetHorizontalFrameSize(), minPageWidth)
}

func (s State) LogPanelHeight(nonLogLayoutReserveMin int, minLogPanelHeight int) int {
	available := s.Height - nonLogLayoutReserveMin
	if available < minLogPanelHeight {
		return minLogPanelHeight
	}
	return available
}

func (s *State) SetLogViewportContent() {
	width := max(s.LogView.Width, minViewportDimension)
	s.LogView.SetContent(wrapLogText(s.LogText, width))
}

func (s *State) ResizeLogs(nonLogLayoutReserveMin int, minLogPanelHeight int) {
	w := max(s.PageWidth()-logPanelHorizontalInset, minLogViewportWidth)
	h := max(s.LogPanelHeight(nonLogLayoutReserveMin, minLogPanelHeight)-logViewportHeightOffset, minLogViewportHeight)
	s.LogView.Width = w
	s.LogView.Height = h
	s.SetLogViewportContent()
}

func (s *State) FitLogViewportHeight(nonLogSections []string, nonLogLayoutReserveMin int, minLogPanelHeight int) {
	if s.Height <= 0 {
		return
	}
	desired := max(s.LogPanelHeight(nonLogLayoutReserveMin, minLogPanelHeight)-logViewportHeightOffset, minLogViewportHeight)
	nonLogHeight := lipgloss.Height(strings.Join(nonLogSections, "\n\n"))
	availablePanel := s.Height - borderRows - nonLogHeight - sectionGapRows
	maxLogHeight := max(availablePanel-panelFrameOverhead, minLogViewportHeight)
	if desired > maxLogHeight {
		desired = maxLogHeight
	}
	s.LogView.Height = desired
}

func (s *State) ResizeFilePicker() {
	h := max(s.Height-filePickerHeightOffset, minFilePickerHeight)
	s.FilePicker.SetHeight(h)
}

func (s State) WithDraftFromControls() State {
	s.DraftSettings.BaseURL = strings.TrimSpace(s.Inputs[0].Value())
	s.DraftSettings.Token = strings.TrimSpace(s.Inputs[1].Value())
	s.DraftSettings.LogDir = strings.TrimSpace(s.Inputs[2].Value())
	s.DraftSettings.AutoConnect = s.AutoConn
	s.DraftSettings.Debug = s.DebugOn
	s.SettingsDirty = s.DraftSettings != s.SavedSettings
	return s
}

func (s State) WithDraftAppliedToControls() State {
	s.Inputs[0].SetValue(strings.TrimSpace(s.DraftSettings.BaseURL))
	s.Inputs[1].SetValue(strings.TrimSpace(s.DraftSettings.Token))
	s.Inputs[2].SetValue(strings.TrimSpace(s.DraftSettings.LogDir))
	s.AutoConn = s.DraftSettings.AutoConnect
	return s
}

func (s State) WithSaveCommitted() State {
	s.SavedSettings = s.DraftSettings
	s.SettingsDirty = false
	return s
}

func (s State) WithCancelDraft() State {
	s.DraftSettings = s.SavedSettings
	s = s.WithDraftAppliedToControls()
	s.SettingsDirty = false
	return s
}

func (s State) WithSelectedLogDir(path string) State {
	if abs, err := filepath.Abs(path); err == nil {
		path = abs
	}
	s.Inputs[2].SetValue(path)
	s.FilePickerOpen = false
	return s.WithDraftFromControls()
}

func wrapLogText(text string, width int) string {
	if width <= 0 || text == "" {
		return text
	}
	return ansi.Wrap(text, width, "")
}
