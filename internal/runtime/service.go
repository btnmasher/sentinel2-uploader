package runtime

import (
	"context"
	"net/http"
	"time"

	"sentinel2-uploader/internal/app"
	"sentinel2-uploader/internal/client"
	"sentinel2-uploader/internal/config"
	"sentinel2-uploader/internal/logging"
)

const defaultHTTPTimeout = 10 * time.Second

type Service interface {
	RunContext(ctx context.Context) error
}

func NewService(opts config.Options, logger *logging.Logger) (Service, error) {
	return NewServiceWithHooks(opts, logger, StartHooks{})
}

func NewServiceWithHooks(opts config.Options, logger *logging.Logger, hooks StartHooks) (Service, error) {
	if logger == nil {
		panic("runtime.NewServiceWithHooks: logger must not be nil")
	}
	if err := config.ValidateRequired(opts); err != nil {
		return nil, err
	}

	endpoints, err := config.BuildEndpoints(opts.BaseURL)
	if err != nil {
		return nil, err
	}
	logger.Debug("constructed API endpoints",
		logging.Field("config_url", endpoints.ConfigURL),
		logging.Field("heartbeat_url", endpoints.HeartbeatURL),
		logging.Field("session_refresh_url", endpoints.SessionRefreshURL),
		logging.Field("submit_url", endpoints.SubmitURL),
		logging.Field("realtime_token_url", endpoints.RealtimeTokenURL),
		logging.Field("realtime_url", endpoints.RealtimeURL),
	)

	httpClient := &http.Client{Timeout: defaultHTTPTimeout}
	sentinelClient := client.New(httpClient, opts.Token, endpoints, logger)
	return app.New(opts, sentinelClient, logger, app.Callbacks{
		OnChannelsUpdate: hooks.OnChannelsUpdate,
		OnStatusChange:   hooks.OnStatus,
	}), nil
}
