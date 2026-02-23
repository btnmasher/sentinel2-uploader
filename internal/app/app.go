package app

import (
	"context"
	"errors"
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
	opts               config.Options
	client             *client.SentinelClient
	logger             *logging.Logger
	hooks              Callbacks
	status             runtimeStatusState
	lastAPISuccessUnix atomic.Int64
}

type connectionEventKind string

type connectionEvent struct {
	kind             connectionEventKind
	epoch            uint64
	hasRealtimeEpoch bool
}

const (
	connectionEventAuthenticated        connectionEventKind = "authenticated"
	connectionEventChannelsReceived     connectionEventKind = "channels_received"
	connectionEventRealtimeConnected    connectionEventKind = "realtime_connected"
	connectionEventRealtimeDisconnected connectionEventKind = "realtime_disconnected"
	connectionEventAPISuccess           connectionEventKind = "api_success"
	connectionEventReconnectExhausted   connectionEventKind = "reconnect_exhausted"
	connectionEventAuthFailed           connectionEventKind = "auth_failed"
	connectionEventStopped              connectionEventKind = "stopped"
)

func newConnectionEvent(kind connectionEventKind) connectionEvent {
	return connectionEvent{kind: kind}
}

func newRealtimeEpochEvent(kind connectionEventKind, epoch uint64) connectionEvent {
	return connectionEvent{
		kind:             kind,
		epoch:            epoch,
		hasRealtimeEpoch: true,
	}
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
	var stopReasonMu sync.Mutex
	var stopReason error
	setStopReason := func(err error) {
		if err == nil {
			return
		}
		stopReasonMu.Lock()
		defer stopReasonMu.Unlock()
		if stopReason == nil {
			stopReason = err
		}
	}
	getStopReason := func() error {
		stopReasonMu.Lock()
		defer stopReasonMu.Unlock()
		return stopReason
	}

	stopForAuth := func(cause error) {
		if !authShutdown.CompareAndSwap(false, true) {
			return
		}
		a.logger.Warn("stopping uploader due to authentication failure", logging.Field("error", cause))
		a.applyConnectionEvent(newConnectionEvent(connectionEventAuthFailed))
		setStopReason(fmt.Errorf("%w: %w", ErrAuthenticationFailed, cause))
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
			a.applyConnectionEvent(newConnectionEvent(connectionEventAuthFailed))
			return fmt.Errorf("%w: %w", ErrAuthenticationFailed, err)
		}
		return fmt.Errorf("%w: %w", ErrStartupRealtimeConnect, err)
	}
	a.applyConnectionEvent(newConnectionEvent(connectionEventAuthenticated))

	sessionState := sessionState{}
	sessionState.setSessionToken(session.Token)

	channels, err := a.client.FetchChannels(runCtx, session.Token)
	if err != nil {
		return fmt.Errorf("failed to fetch channels: %w", err)
	}
	a.applyConnectionEvent(newConnectionEvent(connectionEventChannelsReceived))
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
		OnConnected: func(topic string, session pbrealtime.Session, epoch uint64) {
			sessionState.setConnectedSession(session.Token)
			a.logger.Info("realtime epoch connected",
				logging.Field("epoch", epoch),
				logging.Field("topic", topic),
				logging.Field("expires_at", session.ExpiresAt),
				logging.Field("refresh_after_seconds", session.RefreshAfterSeconds),
			)
			a.applyConnectionEvent(newRealtimeEpochEvent(connectionEventRealtimeConnected, epoch))
			select {
			case connected <- struct{}{}:
			default:
			}
		},
		OnDisconnected: func(err error, epoch uint64) {
			sessionState.clearConnection()
			a.logger.Warn("realtime epoch disconnected",
				logging.Field("epoch", epoch),
				logging.Field("error", err),
			)
			if runCtx.Err() == nil {
				if err == nil {
					a.logger.Debug("ignoring reconnect status transition: realtime disconnected without an error")
					return
				}
				if client.IsUnauthorized(err) {
					a.applyConnectionEvent(newConnectionEvent(connectionEventAuthFailed))
				} else {
					a.applyConnectionEvent(newRealtimeEpochEvent(connectionEventRealtimeDisconnected, epoch))
				}
			}
			if err != nil && runCtx.Err() == nil {
				a.logger.Debug("realtime channel stream disconnected", logging.Field("error", err))
			}
		},
		OnStopped: func(err error) {
			if runCtx.Err() != nil {
				return
			}
			a.logger.Warn("realtime channel sync retries exhausted", logging.Field("error", err))
			a.applyConnectionEvent(newConnectionEvent(connectionEventReconnectExhausted))
			setStopReason(fmt.Errorf("%w: %w", ErrRealtimeReconnectExhausted, err))
			runCancel()
		},
		OnAuthFailure: stopForAuth,
		ShouldContinueAfterReconnectExhausted: func(lastErr error, maxElapsed time.Duration) bool {
			lastSuccessUnix := a.lastAPISuccessUnix.Load()
			if lastSuccessUnix <= 0 {
				return false
			}
			lastSuccess := time.Unix(lastSuccessUnix, 0)
			since := time.Since(lastSuccess)
			if since > maxElapsed {
				return false
			}
			if errors.Is(lastErr, pbrealtime.ErrSessionRefreshDue) {
				return true
			}
			a.logger.Warn("keeping realtime reconnect attempts alive due to recent successful API activity",
				logging.Field("error", lastErr),
				logging.Field("last_api_success_ago", since.String()),
				logging.Field("max_elapsed", maxElapsed.String()),
			)
			return true
		},
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
		return fmt.Errorf("%w: %w", ErrRealtimeHandshakeTimeout, waitCtx.Err())
	case <-connected:
		// OnConnected already applies state with epoch; this path only gates startup.
	}

	go a.runHeartbeatLoop(runCtx, &sessionState, stopForAuth)

	runErr := monitor.RunContext(runCtx, monitorUpdates)
	if reason := getStopReason(); reason != nil {
		a.applyConnectionEvent(newConnectionEvent(connectionEventStopped))
		if authShutdown.Load() {
			a.logger.Warn("uploader app stopped due to authentication failure", logging.Field("error", reason))
		} else {
			a.logger.Warn("uploader app stopped after reconnect exhaustion", logging.Field("error", reason))
		}
		return reason
	}
	if runErr != nil {
		a.applyConnectionEvent(newConnectionEvent(connectionEventStopped))
		a.logger.Warn("uploader app stopped with error", logging.Field("error", runErr))
		return runErr
	}
	a.applyConnectionEvent(newConnectionEvent(connectionEventStopped))
	a.logger.Info("uploader app stopped")
	return nil
}

type sessionState struct {
	mu        sync.RWMutex
	connected bool
	token     string
}

type runtimeStatusState struct {
	mu                sync.Mutex
	current           string
	authFailed        bool
	realtimeConnected bool
	stopped           bool
	realtimeEpoch     uint64
}

func (s *runtimeStatusState) apply(event connectionEvent) (string, string, bool) {
	next := strings.TrimSpace(s.current)
	s.mu.Lock()
	defer s.mu.Unlock()
	switch event.kind {
	case connectionEventAuthenticated:
		s.authFailed = false
		s.stopped = false
		s.realtimeConnected = false
		s.realtimeEpoch = 0
		if !s.realtimeConnected {
			next = runstatus.Authenticated
		}
	case connectionEventChannelsReceived:
		if s.authFailed || s.stopped {
			break
		}
		if s.realtimeConnected {
			next = runstatus.Connected
		} else {
			next = runstatus.ChannelsReceived
		}
	case connectionEventRealtimeConnected:
		if s.authFailed || s.stopped {
			break
		}
		if !event.hasRealtimeEpoch {
			break
		}
		if event.epoch < s.realtimeEpoch {
			break
		}
		s.realtimeEpoch = event.epoch
		s.realtimeConnected = true
		next = runstatus.Connected
	case connectionEventRealtimeDisconnected:
		if s.authFailed || s.stopped {
			break
		}
		if !event.hasRealtimeEpoch {
			break
		}
		if event.epoch < s.realtimeEpoch {
			break
		}
		s.realtimeEpoch = event.epoch
		s.realtimeConnected = false
		next = runstatus.Reconnecting
	case connectionEventAPISuccess:
		if s.authFailed || s.stopped {
			break
		}
		next = runstatus.Connected
	case connectionEventReconnectExhausted:
		if s.authFailed || s.stopped {
			break
		}
		s.realtimeConnected = false
		next = runstatus.Disconnected
	case connectionEventAuthFailed:
		s.authFailed = true
		s.realtimeConnected = false
		s.stopped = true
		next = runstatus.DisconnectedAuth
	case connectionEventStopped:
		s.stopped = true
		s.realtimeConnected = false
		if s.authFailed {
			next = runstatus.DisconnectedAuth
		} else {
			next = runstatus.Disconnected
		}
	}

	if s.current == next {
		return s.current, next, false
	}
	previous := s.current
	s.current = next
	return previous, next, true
}

func (s *runtimeStatusState) key() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return runstatus.Key(s.current)
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
		if err == nil {
			a.markConnectionHealthy()
		}
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
				a.applyConnectionEvent(newConnectionEvent(connectionEventAuthFailed))
				if onAuthFailure != nil {
					onAuthFailure(fetchErr)
				}
			}
			return err
		}
		refreshed = longLivedSession
	}

	state.setSessionToken(refreshed.Token)
	retryErr := call(refreshed.Token)
	if retryErr == nil {
		a.markConnectionHealthy()
	}
	return retryErr
}

func (a *UploaderApp) markConnectionHealthy() {
	a.lastAPISuccessUnix.Store(time.Now().Unix())
	key := a.status.key()
	if key == runstatus.KeyDisconnected || key == runstatus.KeyDisconnectedAuth {
		return
	}
	a.applyConnectionEvent(newConnectionEvent(connectionEventAPISuccess))
}

func (a *UploaderApp) runHeartbeatLoop(ctx context.Context, state *sessionState, onAuthFailure func(error)) {
	send := func() bool {
		if _, ok := state.sessionToken(); !ok {
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

func (a *UploaderApp) applyConnectionEvent(event connectionEvent) {
	previous, next, changed := a.status.apply(event)
	if !changed {
		return
	}
	a.logger.Debug("runtime status transition",
		logging.Field("event", string(event.kind)),
		logging.Field("realtime_epoch", event.epoch),
		logging.Field("from", previous),
		logging.Field("to", next),
	)
	a.notifyStatus(next)
}
