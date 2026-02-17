package client

import (
	"context"
	"io"
	"net/http"
	"strings"

	"sentinel2-uploader/internal/logging"
)

func (c *SentinelClient) Heartbeat(ctx context.Context, sessionToken string) error {
	token := strings.TrimSpace(sessionToken)
	if token == "" {
		return &HTTPStatusError{StatusCode: http.StatusUnauthorized, Status: "missing uploader realtime session token"}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoints.HeartbeatURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	c.logger.Debugf("POST %s -> %s", c.endpoints.HeartbeatURL, resp.Status)

	if resp.StatusCode >= http.StatusBadRequest {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		c.logger.Warn("heartbeat rejected",
			logging.Field("status", resp.Status),
			logging.Field("response", logging.FormatHTTPPayload(data)),
		)
		return &HTTPStatusError{StatusCode: resp.StatusCode, Status: resp.Status}
	}

	c.logger.Debug("heartbeat accepted")
	return nil
}
