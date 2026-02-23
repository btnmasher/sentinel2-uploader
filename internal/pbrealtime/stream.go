package pbrealtime

import (
	"context"
	"crypto/tls"
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
	ExtraTopics []string
	ForceHTTP1  bool
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
	if s.ForceHTTP1 {
		streamHTTP.Transport = http1OnlyRoundTripper(streamHTTP.Transport)
	}

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
				if subscribeErr := subscribe(ctx, currentClientID, session.Token, buildSubscribeTopics(session.Topic, s.ExtraTopics)); subscribeErr != nil {
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

func buildSubscribeTopics(primary string, extras []string) []string {
	seen := make(map[string]struct{}, len(extras)+1)
	topics := make([]string, 0, len(extras)+1)

	appendTopic := func(topic string) {
		name := strings.TrimSpace(topic)
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		topics = append(topics, name)
	}

	appendTopic(primary)
	for _, topic := range extras {
		appendTopic(topic)
	}
	return topics
}

func http1OnlyRoundTripper(rt http.RoundTripper) http.RoundTripper {
	switch transport := rt.(type) {
	case nil:
		base, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			return rt
		}
		clone := base.Clone()
		disableHTTP2(clone)
		return clone
	case *http.Transport:
		clone := transport.Clone()
		disableHTTP2(clone)
		return clone
	default:
		// Custom transports (eg test round-trippers) may not support HTTP/2 anyway.
		return rt
	}
}

func disableHTTP2(transport *http.Transport) {
	transport.ForceAttemptHTTP2 = false
	transport.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
}
