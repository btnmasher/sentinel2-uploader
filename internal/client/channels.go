package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"

	"sentinel2-uploader/internal/logging"
)

func (c *SentinelClient) FetchChannels(ctx context.Context, sessionToken string) ([]ChannelConfig, error) {
	token := strings.TrimSpace(sessionToken)
	if token == "" {
		return nil, &HTTPStatusError{StatusCode: http.StatusUnauthorized, Status: "missing uploader realtime session token"}
	}
	c.logger.Debug("fetching channel config", logging.Field("url", c.endpoints.ConfigURL))
	req, err := http.NewRequestWithContext(ctx, "GET", c.endpoints.ConfigURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	c.logger.Debugf("GET %s -> %s", c.endpoints.ConfigURL, resp.Status)

	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		body := logging.FormatHTTPPayload(data)
		c.logger.Warn("config request failed",
			logging.Field("status", resp.Status),
			logging.Field("content_type", resp.Header.Get("Content-Type")),
			logging.Field("response", body),
		)
		return nil, &HTTPStatusError{StatusCode: resp.StatusCode, Status: resp.Status}
	}

	var cfg uploaderConfigResponse
	if err := json.Unmarshal(data, &cfg); err != nil {
		body := logging.FormatHTTPPayload(data)
		c.logger.Warn("invalid config JSON",
			logging.Field("url", c.endpoints.ConfigURL),
			logging.Field("content_type", resp.Header.Get("Content-Type")),
			logging.Field("error", err),
			logging.Field("response", body),
		)
		return nil, err
	}

	out := []ChannelConfig{}
	for _, channel := range cfg.Channels {
		trimmed := strings.TrimSpace(channel.Name)
		if trimmed != "" {
			out = append(out, ChannelConfig{ID: strings.TrimSpace(channel.ID), Name: trimmed})
		}
	}
	normalized := normalizeChannels(out)
	c.logger.Debug("channel config loaded",
		logging.Field("count", len(normalized)),
	)
	return normalized, nil
}

func normalizeChannels(channels []ChannelConfig) []ChannelConfig {
	normalized := make([]ChannelConfig, 0, len(channels))
	seen := map[string]struct{}{}
	for _, channel := range channels {
		id := strings.TrimSpace(channel.ID)
		name := strings.TrimSpace(channel.Name)
		if id == "" || name == "" {
			continue
		}
		key := id + "\x00" + strings.ToLower(name)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		normalized = append(normalized, ChannelConfig{ID: id, Name: name})
	}
	slices.SortFunc(normalized, func(a, b ChannelConfig) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	return normalized
}

func channelsEqual(a []ChannelConfig, b []ChannelConfig) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID || a[i].Name != b[i].Name {
			return false
		}
	}
	return true
}
