package view

import (
	"strings"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"

	"sentinel2-uploader/internal/config"
	"sentinel2-uploader/internal/ui/headless/keyboard"
)

const (
	inputCount            = 3
	defaultInputCharLimit = 2048
	defaultInputWidth     = 80
	baseURLInputIndex     = 0
	tokenInputIndex       = 1
	logDirInputIndex      = 2
	defaultTab            = TabOverview
	defaultAnimPhase      = 0
	defaultLogViewWidth   = 80
	defaultLogViewHeight  = 20
	defaultPaneWidth      = 24
	defaultPaneHeight     = 8
	defaultSettingsHeight = 12
	maxAnimPhaseValue     = 1_000_000_000
)

type State struct {
	Inputs []textinput.Model
	Focus  int
	Tab    int

	HelpView help.Model
	Keys     keyboard.Map

	ShowLogs      bool
	AutoConn      bool
	SettingsDirty bool
	FollowLogs    bool
	DebugOn       bool

	LogText      string
	LogView      viewport.Model
	LeftView     viewport.Model
	RightView    viewport.Model
	SettingsView viewport.Model

	Width     int
	Height    int
	AnimPhase int
	ImGay     bool

	ConfirmQuit       bool
	ConfirmQuitChoice int
	ErrorModalText    string
	FilePickerOpen    bool
	FilePicker        filepicker.Model
	HoverZone         string

	SavedSettings config.UploaderSettings
	DraftSettings config.UploaderSettings
}

func NewState(opts config.Options, defaultLogDir string) State {
	inputs := make([]textinput.Model, inputCount)
	for i := range inputs {
		inputs[i] = textinput.New()
		inputs[i].CharLimit = defaultInputCharLimit
		inputs[i].Width = defaultInputWidth
		inputs[i].Prompt = ""
	}
	inputs[baseURLInputIndex].Placeholder = "https://intel.example.com"
	inputs[baseURLInputIndex].SetValue(strings.TrimSpace(opts.BaseURL))
	inputs[tokenInputIndex].Placeholder = "Uploader token"
	inputs[tokenInputIndex].EchoMode = textinput.EchoPassword
	inputs[tokenInputIndex].EchoCharacter = '•'
	inputs[tokenInputIndex].SetValue(strings.TrimSpace(opts.Token))
	inputs[logDirInputIndex].Placeholder = defaultLogDir
	inputs[logDirInputIndex].SetValue(strings.TrimSpace(opts.LogDir))
	inputs[baseURLInputIndex].Focus()

	picker := filepicker.New()
	picker.FileAllowed = false
	picker.DirAllowed = true
	picker.ShowHidden = false
	picker.ShowSize = false
	picker.ShowPermissions = false
	picker.KeyMap.Open = key.NewBinding(key.WithKeys(" ", "right", "l"), key.WithHelp("space", "open"))
	picker.KeyMap.Select = key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "select"))

	saved := config.SettingsFromOptions(opts)
	helpView := help.New()
	helpView.Styles.ShortKey = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
	helpView.Styles.FullKey = lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
	helpView.Styles.ShortDesc = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	helpView.Styles.FullDesc = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	helpView.Styles.ShortSeparator = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	helpView.Styles.FullSeparator = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	helpView.Styles.Ellipsis = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	return State{
		Inputs:        inputs,
		Tab:           defaultTab,
		HelpView:      helpView,
		Keys:          keyboard.New(),
		AutoConn:      opts.AutoConnect,
		DebugOn:       opts.Debug,
		FollowLogs:    true,
		ImGay:         opts.ImGay,
		AnimPhase:     defaultAnimPhase,
		LogView:       viewport.New(defaultLogViewWidth, defaultLogViewHeight),
		LeftView:      viewport.New(defaultPaneWidth, defaultPaneHeight),
		RightView:     viewport.New(defaultPaneWidth, defaultPaneHeight),
		SettingsView:  viewport.New(defaultLogViewWidth, defaultSettingsHeight),
		FilePicker:    picker,
		SavedSettings: saved,
		DraftSettings: saved,
	}
}

func (s State) WithWindowSize(width int, height int) State {
	s.Width = width
	s.Height = height
	return s
}

func (s State) WithTick() State {
	s.AnimPhase++
	if s.AnimPhase > maxAnimPhaseValue {
		s.AnimPhase = defaultAnimPhase
	}
	return s
}
