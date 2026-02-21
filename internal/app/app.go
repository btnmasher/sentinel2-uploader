package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"sentinel2-uploader/internal/client"
	"sentinel2-uploader/internal/config"
	"sentinel2-uploader/internal/evelogs"
	"sentinel2-uploader/internal/logging"
	"sentinel2-uploader/internal/pbrealtime"
	"sentinel2-uploader/internal/runctx"
	"sentinel2-uploader/internal/runstatus"
)

const heartbeatInterval = 30 * time.Second

type UploaderApp struct {
	opts   config.Options
	client *client.SentinelClient
	logger *logging.Logger
	hooks  Callbacks
	status runtimeStatusState
}

type Callbacks struct {
	OnChannelsUpdate func([]client.ChannelConfig)
	OnStatusChange   func(string)
}

func New(opts config.Options, client *client.SentinelClient, logger *logging.Logger, hooks Callbacks) *UploaderApp {
	if client == nil {
		panic("app.New: client must not be nil")
	}
	if logger == nil {
		panic("app.New: logger must not be nil")
	}
	return &UploaderApp{opts: opts, client: client, logger: logger, hooks: hooks}
}

func (a *UploaderApp) Run() error {
	return a.RunContext(context.Background())
}

func (a *UploaderApp) RunContext(ctx context.Context) error {
	runCtx, runCancel := context.WithCancel(ctx)
	defer runCancel()

	var authShutdown atomic.Bool
	stopForAuth := func(cause error) {
		if !authShutdown.CompareAndSwap(false, true) {
			return
		}
		a.logger.Warn("stopping uploader due to authentication failure", logging.Field("error", cause))
		a.setRuntimeStatus(runstatus.DisconnectedAuth)
		runCancel()
	}

	a.logger.Info("uploader app starting",
		logging.Field("log_dir", a.opts.LogDir),
		logging.Field("log_file", a.opts.LogFile),
	)

	if err := a.validateLogDirectory(); err != nil {
		return err
	}

	session, err := a.client.FetchRealtimeSession(runCtx)
	if err != nil {
		if client.IsUnauthorized(err) {
			stopForAuth(err)
			return nil
		}
		return fmt.Errorf("failed to authenticate realtime session: %w", err)
	}
	a.setRuntimeStatus(runstatus.Authenticated)

	sessionState := sessionState{}
	sessionState.setSessionToken(session.Token)

	channels, err := a.client.FetchChannels(runCtx, session.Token)
	if err != nil {
		return fmt.Errorf("failed to fetch channels: %w", err)
	}
	a.setRuntimeStatus(runstatus.ChannelsReceived)
	if len(channels) == 0 {
		return fmt.Errorf("no channels configured")
	}
	a.logger.Info("initial channels loaded", logging.Field("count", len(channels)))
	a.notifyChannels(channels)

	monitor := evelogs.NewMonitor(evelogs.MonitorOptions{
		LogDir:   a.opts.LogDir,
		LogFile:  a.opts.LogFile,
		Channels: channels,
	}, a.logger, evelogs.MonitorCallbacks{
		OnReport: func(event evelogs.ReportEvent) error {
			return a.withSessionRetry(runCtx, &sessionState, func(token string) error {
				return a.client.Submit(runCtx, client.SubmitPayload{Text: event.Line, ChannelID: event.Channel.ID}, token)
			}, stopForAuth)
		},
		OnError: func(err error) {
			a.logger.Warn("log monitor callback error", logging.Field("error", err))
		},
	})
	if err := monitor.Prepare(); err != nil {
		return err
	}

	connected := make(chan struct{}, 1)
	configUpdates := a.client.StartChannelConfigSync(runCtx, channels, client.SyncHooks{
		OnConnected: func(topic string, session pbrealtime.Session) {
			sessionState.setConnectedSession(session.Token)
			a.setRuntimeStatus(runstatus.Connected)
			select {
			case connected <- struct{}{}:
			default:
			}
		},
		OnDisconnected: func(err error) {
			sessionState.clearConnection()
			if runCtx.Err() == nil {
				if client.IsUnauthorized(err) {
					a.setRuntimeStatus(runstatus.DisconnectedAuth)
				} else {
					a.setRuntimeStatus(runstatus.Reconnecting)
				}
			}
			if err != nil && runCtx.Err() == nil {
				a.logger.Debug("realtime channel stream disconnected", logging.Field("error", err))
			}
		},
		OnAuthFailure: stopForAuth,
	}, &session)
	monitorUpdates := make(chan []client.ChannelConfig, 1)
	go a.forwardChannelUpdates(runCtx, configUpdates, monitorUpdates)

	waitCtx, waitCancel := context.WithTimeout(runCtx, 15*time.Second)
	defer waitCancel()
	select {
	case <-waitCtx.Done():
		if runCtx.Err() != nil {
			a.logger.Debug("stopping startup handshake wait: context canceled", logging.Field("error", runCtx.Err()))
		} else {
			a.logger.Debug("startup handshake wait expired", logging.Field("error", waitCtx.Err()))
		}
		return fmt.Errorf("realtime subscribe handshake timeout: %w", waitCtx.Err())
	case <-connected:
		a.setRuntimeStatus(runstatus.Connected)
	}

	go a.runHeartbeatLoop(runCtx, &sessionState, stopForAuth)

	runErr := monitor.RunContext(runCtx, monitorUpdates)
	if authShutdown.Load() {
		a.logger.Info("uploader app stopped due to authentication failure")
		return nil
	}
	if runErr != nil {
		a.setRuntimeStatus(runstatus.Disconnected)
		a.logger.Warn("uploader app stopped with error", logging.Field("error", runErr))
		return runErr
	}
	a.setRuntimeStatus(runstatus.Disconnected)
	a.logger.Info("uploader app stopped")
	return nil
}

type sessionState struct {
	mu        sync.RWMutex
	connected bool
	token     string
}

type runtimeStatusState struct {
	mu      sync.Mutex
	current string
}

func (s *runtimeStatusState) update(status string) (string, string, bool) {
	trimmed := strings.TrimSpace(status)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.current == trimmed {
		return s.current, trimmed, false
	}
	previous := s.current
	s.current = trimmed
	return previous, trimmed, true
}

func (s *sessionState) setConnectedSession(token string) {
	s.mu.Lock()
	s.connected = true
	s.token = strings.TrimSpace(token)
	s.mu.Unlock()
}

func (s *sessionState) setSessionToken(token string) {
	s.mu.Lock()
	s.token = strings.TrimSpace(token)
	s.mu.Unlock()
}

func (s *sessionState) clearConnection() {
	s.mu.Lock()
	s.connected = false
	s.mu.Unlock()
}

func (s *sessionState) sessionToken() (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.token == "" {
		return "", false
	}
	return s.token, true
}

func (s *sessionState) sessionTokenIfConnected() (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.connected || s.token == "" {
		return "", false
	}
	return s.token, true
}

func (a *UploaderApp) withSessionRetry(ctx context.Context, state *sessionState, call func(token string) error, onAuthFailure func(error)) error {
	token, ok := state.sessionToken()
	if !ok {
		return fmt.Errorf("uploader session unavailable")
	}
	err := call(token)
	if !client.IsUnauthorized(err) {
		return err
	}

	refreshed, refreshErr := a.client.RefreshSession(ctx, token)
	if refreshErr != nil {
		if !client.IsUnauthorized(refreshErr) {
			return err
		}

		a.logger.Warn("short session refresh unauthorized; attempting long-lived re-auth", logging.Field("error", refreshErr))
		longLivedSession, fetchErr := a.client.FetchRealtimeSession(ctx)
		if fetchErr != nil {
			if client.IsUnauthorized(fetchErr) {
				state.clearConnection()
				a.setRuntimeStatus(runstatus.DisconnectedAuth)
				if onAuthFailure != nil {
					onAuthFailure(fetchErr)
				}
			}
			return err
		}
		refreshed = longLivedSession
	}

	state.setSessionToken(refreshed.Token)
	return call(refreshed.Token)
}

func (a *UploaderApp) runHeartbeatLoop(ctx context.Context, state *sessionState, onAuthFailure func(error)) {
	send := func() bool {
		if _, ok := state.sessionTokenIfConnected(); !ok {
			return true
		}
		if err := a.withSessionRetry(ctx, state, func(token string) error {
			return a.client.Heartbeat(ctx, token)
		}, onAuthFailure); err != nil {
			if ctx.Err() != nil {
				return false
			}
			a.logger.Warn("heartbeat failed", logging.Field("error", err))
			return true
		}
		return true
	}

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !send() {
				return
			}
		}
	}
}

func (a *UploaderApp) validateLogDirectory() error {
	logDir := strings.TrimSpace(a.opts.LogDir)
	if logDir == "" {
		logFile := strings.TrimSpace(a.opts.LogFile)
		if logFile == "" {
			return fmt.Errorf("log directory is required")
		}
		logDir = filepath.Dir(logFile)
	}
	if logDir == "" {
		return fmt.Errorf("log directory is required")
	}
	info, err := os.Stat(logDir)
	if err != nil {
		return fmt.Errorf("log directory is not accessible: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("log path is not a directory")
	}
	return nil
}

func (a *UploaderApp) forwardChannelUpdates(ctx context.Context, source <-chan []client.ChannelConfig, target chan<- []client.ChannelConfig) {
	defer close(target)
	for {
		channels, ok := runctx.RecvOrDone(ctx, "channel update forwarder", a.logger, source)
		if !ok {
			return
		}
		a.logger.Debug("forwarding channel update", logging.Field("count", len(channels)))
		a.notifyChannels(channels)
		if !runctx.SendOrDone(ctx, "channel update forwarder", a.logger, target, channels) {
			return
		}
		a.logger.Debug("channel update forwarded", logging.Field("count", len(channels)))
	}
}

func (a *UploaderApp) notifyChannels(channels []client.ChannelConfig) {
	if a.hooks.OnChannelsUpdate == nil {
		return
	}
	copied := append([]client.ChannelConfig(nil), channels...)
	a.hooks.OnChannelsUpdate(copied)
}

func (a *UploaderApp) notifyStatus(status string) {
	if a.hooks.OnStatusChange == nil {
		return
	}
	a.hooks.OnStatusChange(status)
}

func (a *UploaderApp) setRuntimeStatus(status string) {
	previous, next, changed := a.status.update(status)
	if !changed {
		return
	}
	a.logger.Debug("runtime status transition",
		logging.Field("from", previous),
		logging.Field("to", next),
	)
	a.notifyStatus(status)
}
