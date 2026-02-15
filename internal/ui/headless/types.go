package headless

import (
	"context"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"sentinel2-uploader/internal/client"
	"sentinel2-uploader/internal/logging"
	"sentinel2-uploader/internal/runtime"
	"sentinel2-uploader/internal/ui/headless/health"
	headlessview "sentinel2-uploader/internal/ui/headless/view"
)

const headlessLogLimit = 200_000

const (
	minLogPanelHeight      = 8
	nonLogLayoutReserveMin = 24
)

type logMsg string
type statusMsg string
type tickMsg struct{}
type channelsUpdatedMsg []client.ChannelConfig

type runDoneMsg struct {
	err error
}

type startResultMsg struct {
	err error
}

type quitNowMsg struct{}

type statusKind int

const (
	statusIdle statusKind = iota
	statusConnecting
	statusConnected
	statusStopping
	statusError
)

type modelDeps struct {
	runner      *runtime.Controller
	logger      *logging.Logger
	unsubscribe func()
	rootCancel  context.CancelFunc
	program     *tea.Program
}

type modelChannels struct {
	logCh    chan string
	cfgCh    chan []client.ChannelConfig
	statusCh chan string
}

type modelRuntime struct {
	running    bool
	connecting bool
	quitting   bool
	status     string
	kind       statusKind

	channels          []client.ChannelConfig
	channelHealth     []health.Row
	healthDetail      string
	lastHealthRefresh time.Time
}

type headlessModel struct {
	buildVersion string
	modelDeps
	modelChannels
	modelRuntime
	cleanupOnce sync.Once
	ui          headlessview.State
}
