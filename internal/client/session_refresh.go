package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"sentinel2-uploader/internal/logging"
	"sentinel2-uploader/internal/pbrealtime"
)

func (c *SentinelClient) RefreshSession(ctx context.Context, sessionToken string) (pbrealtime.Session, error) {
	token := strings.TrimSpace(sessionToken)
	if token == "" {
		return pbrealtime.Session{}, &HTTPStatusError{StatusCode: http.StatusUnauthorized, Status: "missing uploader realtime session token"}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoints.SessionRefreshURL, nil)
	if err != nil {
		return pbrealtime.Session{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := c.http.Do(req)
	if err != nil {
		return pbrealtime.Session{}, err
	}
	defer resp.Body.Close()
	c.logger.Debugf("POST %s -> %s", c.endpoints.SessionRefreshURL, resp.Status)

	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= http.StatusBadRequest {
		c.logger.Warn("session refresh rejected",
			logging.Field("status", resp.Status),
			logging.Field("response", logging.FormatHTTPPayload(data)),
		)
		return pbrealtime.Session{}, &HTTPStatusError{StatusCode: resp.StatusCode, Status: resp.Status}
	}

	session := pbrealtime.Session{}
	if unmarshalErr := json.Unmarshal(data, &session); unmarshalErr != nil {
		return pbrealtime.Session{}, unmarshalErr
	}
	return session, nil
}
