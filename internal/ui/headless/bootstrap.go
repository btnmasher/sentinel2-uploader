package headless

import (
	"context"
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"

	"sentinel2-uploader/internal/client"
	"sentinel2-uploader/internal/config"
	"sentinel2-uploader/internal/logging"
	"sentinel2-uploader/internal/runtime"
	headlessview "sentinel2-uploader/internal/ui/headless/view"
)

const (
	logChannelBufferSize    = 512
	configChannelBufferSize = 8
	statusChannelBufferSize = 16
	updateTickInterval      = 120 * time.Millisecond
	runErrorExitCode        = 1
)

func Run(rootCtx context.Context, buildVersion string, opts config.Options) {
	defer forceDisableMouseTracking()

	var savedSettings config.UploaderSettings
	if saved, loadErr := config.LoadSettings(); loadErr == nil {
		savedSettings = saved
		opts = config.MergeOptionsWithSettings(opts, saved)
	}

	logger := logging.New(false)
	if logger == nil {
		panic("headless.Run: logging.New returned nil")
	}
	logger.SetDebugEnabled(opts.Debug)
	if err := logger.EnableFilePersistence(0); err != nil {
		logger.Warn("failed to enable file log persistence", logging.Field("error", err))
	}
	logger.SetTerminalOutputEnabled(false)
	logger.Info("starting uploader TUI", logging.Field("version", buildVersion))

	m := newHeadlessModel(rootCtx, buildVersion, opts, logger)
	m.dismissedTag = savedSettings.LastDismissedUpdateTag
	zone.NewGlobal()
	program := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseAllMotion())
	m.program = program
	result, runErr := program.Run()
	model, _ := result.(*headlessModel)
	if model != nil {
		model.cleanup()
	}
	if runErr != nil {
		fmt.Fprintln(os.Stderr, runErr)
		os.Exit(runErrorExitCode)
	}
}

func forceDisableMouseTracking() {
	_, _ = os.Stdout.WriteString("\x1b[?1000l\x1b[?1002l\x1b[?1003l\x1b[?1006l\x1b[?1015l")
}

func newHeadlessModel(rootCtx context.Context, buildVersion string, opts config.Options, logger *logging.Logger) *headlessModel {
	if rootCtx == nil {
		rootCtx = context.Background()
	}
	runCtx, runCancel := context.WithCancel(rootCtx)

	m := &headlessModel{
		buildVersion: buildVersion,
		modelDeps: modelDeps{
			runner:     runtime.NewController(runCtx),
			logger:     logger,
			rootCtx:    runCtx,
			rootCancel: runCancel,
		},
		modelChannels: modelChannels{
			logCh:    make(chan string, logChannelBufferSize),
			cfgCh:    make(chan []client.ChannelConfig, configChannelBufferSize),
			statusCh: make(chan string, statusChannelBufferSize),
			updateCh: make(chan updateAvailableMsg, 2),
		},
		modelRuntime: modelRuntime{
			status: "Idle",
			kind:   statusIdle,
		},
		ui: headlessview.NewState(opts, config.DefaultLogDir()),
	}

	m.unsubscribe = logger.Subscribe(func(event logging.Event) {
		line := logging.FormatEventANSI(event)
		select {
		case m.logCh <- line:
		default:
			select {
			case <-m.logCh:
			default:
			}
			m.logCh <- line
		}
	})

	return m
}

func (m *headlessModel) Init() tea.Cmd {
	cmds := []tea.Cmd{
		waitForLog(m.logCh),
		waitForChannels(m.cfgCh),
		waitForStatus(m.statusCh),
		waitForUpdate(m.updateCh),
		tickCmd(),
		m.startUpdateCheckerCmd(),
	}
	if m.ui.AutoConn && m.canConnect() {
		cmds = append(cmds, m.startUploaderCmd(true))
	}
	return tea.Batch(cmds...)
}

func waitForLog(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		line, ok := <-ch
		if !ok {
			return nil
		}
		return logMsg(line)
	}
}

func waitForChannels(ch <-chan []client.ChannelConfig) tea.Cmd {
	return func() tea.Msg {
		channels, ok := <-ch
		if !ok {
			return nil
		}
		return channelsUpdatedMsg(channels)
	}
}

func waitForStatus(ch <-chan string) tea.Cmd {
	return func() tea.Msg {
		status, ok := <-ch
		if !ok {
			return nil
		}
		return statusMsg(status)
	}
}

func waitForUpdate(ch <-chan updateAvailableMsg) tea.Cmd {
	return func() tea.Msg {
		update, ok := <-ch
		if !ok {
			return nil
		}
		return update
	}
}

func tickCmd() tea.Cmd {
	return tea.Tick(updateTickInterval, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}
