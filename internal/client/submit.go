package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"sentinel2-uploader/internal/logging"
)

func (c *SentinelClient) Submit(ctx context.Context, payload SubmitPayload) error {
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
	req.Header.Set("X-Uploader-Token", c.token)

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
		return fmt.Errorf("submit rejected: %s", resp.Status)
	}
	c.logger.Debug("report submit accepted", logging.Field("channel_id", payload.ChannelID))
	return nil
}
