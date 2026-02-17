package client

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"sentinel2-uploader/internal/logging"
)

func (c *SentinelClient) Submit(ctx context.Context, payload SubmitPayload, sessionToken string) error {
	token := strings.TrimSpace(sessionToken)
	if token == "" {
		return &HTTPStatusError{StatusCode: http.StatusUnauthorized, Status: "missing uploader realtime session token"}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	c.logger.Debug("submitting report",
		logging.Field("channel_id", payload.ChannelID),
		logging.Field("payload", logging.FormatHTTPPayload(body)),
	)

	req, err := http.NewRequestWithContext(ctx, "PUT", c.endpoints.SubmitURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	c.logger.Debugf("PUT %s -> %s", c.endpoints.SubmitURL, resp.Status)

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		formatted := logging.FormatHTTPPayload(data)
		c.logger.Warn("submit rejected",
			logging.Field("status", resp.Status),
			logging.Field("channel_id", payload.ChannelID),
			logging.Field("response", formatted),
		)
		return &HTTPStatusError{StatusCode: resp.StatusCode, Status: resp.Status}
	}
	c.logger.Debug("report submit accepted", logging.Field("channel_id", payload.ChannelID))
	return nil
}
