package pbrealtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"sentinel2-uploader/internal/logging"
)

type SubscribeFunc func(ctx context.Context, clientID string, sessionToken string, topics []string) error

type SessionHandlers struct {
	OnConnected func(topic string)
	OnMessage   func(Event)
	OnUnhandled func(Event)
}

type StreamClient struct {
	HTTP        *http.Client
	RealtimeURL string
	RefreshLead time.Duration
	Logger      *logging.Logger
}

var ErrSessionRefreshDue = errors.New("realtime session refresh due")

func (s StreamClient) RunSession(ctx context.Context, session Session, subscribe SubscribeFunc, handlers SessionHandlers) error {
	if strings.TrimSpace(session.Topic) == "" {
		session.Topic = DefaultTopic
	}

	refreshAfter := time.Duration(session.RefreshAfterSeconds) * time.Second
	if refreshAfter <= 0 && session.ExpiresAt > 0 {
		lead := s.RefreshLead
		if lead <= 0 {
			lead = 10 * time.Second
		}
		refreshAfter = time.Until(time.Unix(session.ExpiresAt, 0).Add(-lead))
	}
	if refreshAfter <= 0 {
		refreshAfter = time.Minute
	}
	if s.Logger != nil {
		s.Logger.Debug("starting realtime stream session",
			logging.Field("topic", session.Topic),
			logging.Field("refresh_after", refreshAfter.String()),
		)
	}

	req, reqErr := http.NewRequestWithContext(ctx, "GET", s.RealtimeURL, nil)
	if reqErr != nil {
		return reqErr
	}
	req.Header.Set("Accept", "text/event-stream")

	httpClient := s.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	// SSE is a long-lived stream; disable whole-request timeout so the body can
	// stay open until server disconnect/reconnect boundaries.
	streamHTTP := *httpClient
	streamHTTP.Timeout = 0

	resp, respErr := streamHTTP.Do(req)
	if respErr != nil {
		return respErr
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		body := logging.FormatHTTPPayload(data)
		if s.Logger != nil {
			s.Logger.Warn("realtime connect failed",
				logging.Field("status", resp.Status),
				logging.Field("response", body),
			)
		}
		return &HTTPStatusError{StatusCode: resp.StatusCode, Status: resp.Status}
	}
	defer resp.Body.Close()

	events := make(chan Event, 16)
	streamErrs := make(chan error, 1)
	go readSSEEvents(resp.Body, events, streamErrs)

	refreshTimer := time.NewTimer(refreshAfter)
	defer refreshTimer.Stop()

	var currentClientID string
	for {
		select {
		case <-ctx.Done():
			if s.Logger != nil {
				s.Logger.Debug("stopping realtime stream session: context canceled", logging.Field("error", ctx.Err()))
			}
			return ctx.Err()
		case <-refreshTimer.C:
			if s.Logger != nil {
				s.Logger.Debug("realtime stream refresh boundary reached")
			}
			return ErrSessionRefreshDue
		case streamErr := <-streamErrs:
			if s.Logger != nil {
				s.Logger.Debug("realtime stream ended", logging.Field("error", streamErr))
			}
			return streamErr
		case event, ok := <-events:
			if !ok {
				return io.EOF
			}

			switch event.Name {
			case "PB_CONNECT":
				payload := connectPayload{}
				if unmarshalErr := json.Unmarshal(event.Data, &payload); unmarshalErr != nil {
					return fmt.Errorf("invalid PB_CONNECT payload: %w", unmarshalErr)
				}
				if payload.ClientID == "" {
					return errors.New("missing realtime client id")
				}
				if payload.ClientID == currentClientID {
					if s.Logger != nil {
						s.Logger.Debug("ignoring duplicate PB_CONNECT event", logging.Field("client_id", payload.ClientID))
					}
					continue
				}
				currentClientID = payload.ClientID
				if subscribeErr := subscribe(ctx, currentClientID, session.Token, []string{session.Topic}); subscribeErr != nil {
					return subscribeErr
				}
				if handlers.OnConnected != nil {
					handlers.OnConnected(session.Topic)
				}
			case session.Topic:
				if handlers.OnMessage != nil {
					handlers.OnMessage(event)
				}
			default:
				if handlers.OnUnhandled != nil {
					handlers.OnUnhandled(event)
				}
			}
		}
	}
}
