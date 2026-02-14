package client

import (
	"net/http"
	"time"

	"sentinel2-uploader/internal/config"
	"sentinel2-uploader/internal/logging"
)

const (
	realtimeRefreshLead = 10 * time.Second
	reconnectDelay      = 5 * time.Second
	reconnectMaxDelay   = 30 * time.Second
)

type SentinelClient struct {
	http      *http.Client
	token     string
	endpoints config.APIEndpoints
	logger    *logging.Logger
}

func New(httpClient *http.Client, token string, endpoints config.APIEndpoints, logger *logging.Logger) *SentinelClient {
	if logger == nil {
		panic("client.New: logger must not be nil")
	}
	return &SentinelClient{http: httpClient, token: token, endpoints: endpoints, logger: logger}
}
