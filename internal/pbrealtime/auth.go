package pbrealtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"sentinel2-uploader/internal/logging"
)

const DefaultTopic = "uploader.config"

type AuthClient struct {
	HTTP             *http.Client
	RealtimeTokenURL string
	RealtimeURL      string
	BearerToken      string
	Logger           *logging.Logger
}

func (a AuthClient) FetchSession(ctx context.Context) (Session, error) {
	if a.Logger != nil {
		a.Logger.Debug("requesting realtime session token", logging.Field("url", a.RealtimeTokenURL))
	}
	req, reqErr := http.NewRequestWithContext(ctx, "POST", a.RealtimeTokenURL, bytes.NewReader([]byte("{}")))
	if reqErr != nil {
		return Session{}, reqErr
	}
	req.Header.Set("X-Uploader-Token", a.BearerToken)
	req.Header.Set("Content-Type", "application/json")

	resp, respErr := a.HTTP.Do(req)
	if respErr != nil {
		return Session{}, respErr
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode >= 400 {
		body := logging.FormatHTTPPayload(data)
		if a.Logger != nil {
			a.Logger.Warn("realtime token request failed",
				logging.Field("status", resp.Status),
				logging.Field("response", body),
			)
		}
		return Session{}, &HTTPStatusError{StatusCode: resp.StatusCode, Status: resp.Status}
	}

	session := Session{}
	if unmarshalErr := json.Unmarshal(data, &session); unmarshalErr != nil {
		return Session{}, fmt.Errorf("invalid realtime token response: %w", unmarshalErr)
	}

	if strings.TrimSpace(session.Token) == "" {
		return Session{}, errors.New("missing realtime token")
	}

	if strings.TrimSpace(session.Topic) == "" {
		session.Topic = DefaultTopic
	}

	if a.Logger != nil {
		a.Logger.Debug("realtime session token acquired",
			logging.Field("topic", session.Topic),
			logging.Field("expires_at", session.ExpiresAt),
			logging.Field("refresh_after_seconds", session.RefreshAfterSeconds),
		)
	}

	return session, nil
}

func (a AuthClient) Subscribe(ctx context.Context, clientID string, sessionToken string, topics []string) error {
	if a.Logger != nil {
		a.Logger.Debug("subscribing realtime topics",
			logging.Field("client_id", clientID),
			logging.Field("topics", topics),
		)
	}

	payload := subscribePayload{ClientID: clientID, Subscriptions: topics}
	body, bodyErr := json.Marshal(payload)
	if bodyErr != nil {
		return bodyErr
	}

	req, reqErr := http.NewRequestWithContext(ctx, "POST", a.RealtimeURL, bytes.NewReader(body))
	if reqErr != nil {
		return reqErr
	}

	req.Header.Set("Authorization", "Bearer "+sessionToken)
	req.Header.Set("Content-Type", "application/json")

	resp, respErr := a.HTTP.Do(req)
	if respErr != nil {
		return respErr
	}

	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		body := logging.FormatHTTPPayload(data)
		if a.Logger != nil {
			a.Logger.Warn("realtime subscribe failed",
				logging.Field("status", resp.Status),
				logging.Field("response", body),
			)
		}
		return &HTTPStatusError{StatusCode: resp.StatusCode, Status: resp.Status}
	}

	if a.Logger != nil {
		a.Logger.Debug("realtime subscribe succeeded", logging.Field("client_id", clientID))
	}

	return nil
}
