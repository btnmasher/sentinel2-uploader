package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sentinel2-uploader/internal/client"
	"sentinel2-uploader/internal/config"
	"sentinel2-uploader/internal/evelogs"
	"sentinel2-uploader/internal/logging"
	"sentinel2-uploader/internal/runctx"
)

type UploaderApp struct {
	opts   config.Options
	client *client.SentinelClient
	logger *logging.Logger
	hooks  Callbacks
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
	a.logger.Info("uploader app starting",
		logging.Field("log_dir", a.opts.LogDir),
		logging.Field("log_file", a.opts.LogFile),
	)

	if err := a.validateLogDirectory(); err != nil {
		return err
	}

	session, err := a.client.FetchRealtimeSession(ctx)
	if err != nil {
		return fmt.Errorf("failed to authenticate realtime session: %w", err)
	}
	a.notifyStatus("Authenticated")

	channels, err := a.client.FetchChannels(ctx)
	if err != nil {
		return fmt.Errorf("failed to fetch channels: %w", err)
	}
	a.notifyStatus("Channels received")
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
			return a.client.Submit(ctx, client.SubmitPayload{Text: event.Line, ChannelID: event.Channel.ID})
		},
		OnError: func(err error) {
			a.logger.Warn("log monitor callback error", logging.Field("error", err))
		},
	})
	if err := monitor.Prepare(); err != nil {
		return err
	}

	connected := make(chan struct{}, 1)
	configUpdates := a.client.StartChannelConfigSync(ctx, channels, client.SyncHooks{
		OnConnected: func(topic string) {
			select {
			case connected <- struct{}{}:
			default:
			}
		},
	}, &session)
	monitorUpdates := make(chan []client.ChannelConfig, 1)
	go a.forwardChannelUpdates(ctx, configUpdates, monitorUpdates)

	waitCtx, waitCancel := context.WithTimeout(ctx, 15*time.Second)
	defer waitCancel()
	select {
	case <-waitCtx.Done():
		if ctx.Err() != nil {
			a.logger.Debug("stopping startup handshake wait: context canceled", logging.Field("error", ctx.Err()))
		} else {
			a.logger.Debug("startup handshake wait expired", logging.Field("error", waitCtx.Err()))
		}
		return fmt.Errorf("realtime subscribe handshake timeout: %w", waitCtx.Err())
	case <-connected:
		a.notifyStatus("Connected")
	}

	runErr := monitor.RunContext(ctx, monitorUpdates)
	if runErr != nil {
		a.logger.Warn("uploader app stopped with error", logging.Field("error", runErr))
		return runErr
	}
	a.logger.Info("uploader app stopped")
	return nil
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
