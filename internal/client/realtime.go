package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/cenkalti/backoff/v5"

	"sentinel2-uploader/internal/logging"
	"sentinel2-uploader/internal/pbrealtime"
)

type SyncHooks struct {
	OnConnected                           func(string, pbrealtime.Session, uint64)
	OnDisconnected                        func(error, uint64)
	OnStopped                             func(error)
	OnAuthFailure                         func(error)
	ShouldContinueAfterReconnectExhausted func(lastErr error, maxElapsed time.Duration) bool
}

func (c *SentinelClient) FetchRealtimeSession(ctx context.Context) (pbrealtime.Session, error) {
	auth := pbrealtime.AuthClient{
		HTTP:             c.http,
		RealtimeTokenURL: c.endpoints.RealtimeTokenURL,
		RealtimeURL:      c.endpoints.RealtimeURL,
		BearerToken:      c.token,
		Logger:           c.logger,
	}
	return auth.FetchSession(ctx)
}

func (c *SentinelClient) StartChannelConfigSync(ctx context.Context, initial []ChannelConfig, hooks SyncHooks, initialSession *pbrealtime.Session) <-chan []ChannelConfig {
	updates := make(chan []ChannelConfig, 1)

	go func() {
		defer close(updates)
		current := normalizeChannels(initial)
		c.logger.Debug("starting channel config sync", logging.Field("initial_count", len(current)))

		// ignore empty/duplicate snapshots and forward real changes.
		pushUpdate := func(next []ChannelConfig) {
			if len(next) == 0 || channelsEqual(current, next) {
				if len(next) == 0 {
					c.logger.Debug("ignoring empty channel snapshot")
				} else {
					c.logger.Debug("ignoring unchanged channel snapshot", logging.Field("count", len(next)))
				}
				return
			}
			current = next
			c.logger.Debug("publishing channel config update", logging.Field("count", len(next)))
			updates <- next
		}

		// When realtime disconnects, do a direct config fetch so updates can still
		// propagate while reconnect attempts are backoff-scheduled.
		prefetchedToken := ""
		fallbackFetch := func() {
			c.logger.Debug("running fallback channel refresh")
			if prefetchedToken == "" {
				c.logger.Debug("skipping fallback channel refresh: no active session token")
				return
			}
			channels, fetchErr := c.FetchChannels(ctx, prefetchedToken)
			if fetchErr != nil {
				c.logger.Warn("fallback channel refresh failed", logging.Field("error", fetchErr))
				return
			}
			pushUpdate(normalizeChannels(channels))
		}

		// Retry the long-lived realtime session with exponential backoff.
		// Each attempt runs one session until it ends from stream error, disconnect,
		// or refresh boundary. Non-context errors trigger the next attempt.
		retry := backoff.NewExponentialBackOff()
		retry.InitialInterval = reconnectDelay
		retry.MaxInterval = reconnectMaxDelay
		retry.Reset()

		useInitialSession := initialSession != nil
		var sessionEpoch uint64
		for {
			_, retryErr := backoff.Retry(ctx, func() (struct{}, error) {
				sessionEpoch++
				attemptEpoch := sessionEpoch
				// Session blocks while connected; returns when disconnected/expired.
				var prefetched *pbrealtime.Session
				if useInitialSession && initialSession != nil {
					s := *initialSession
					prefetched = &s
					prefetchedToken = s.Token
					useInitialSession = false
				}
				runHooks := hooks
				runHooks.OnConnected = func(topic string, session pbrealtime.Session, _ uint64) {
					prefetchedToken = session.Token
					if hooks.OnConnected != nil {
						hooks.OnConnected(topic, session, attemptEpoch)
					}
				}
				err := c.runRealtimeConfigSession(ctx, pushUpdate, runHooks, prefetched, attemptEpoch)
				if err == nil {
					return struct{}{}, nil
				}

				if IsUnauthorized(err) {
					if hooks.OnDisconnected != nil {
						hooks.OnDisconnected(err, attemptEpoch)
					}
					if hooks.OnAuthFailure != nil {
						hooks.OnAuthFailure(err)
					}
					return struct{}{}, backoff.Permanent(err)
				}

				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return struct{}{}, err
				}

				if isExpectedRealtimeReconnect(err) {
					c.logger.Debug("realtime channel sync reconnecting", logging.Field("error", err))
					return struct{}{}, err
				}

				c.logger.Warn("realtime channel sync disconnected", logging.Field("error", err))

				fallbackFetch()

				return struct{}{}, err
			},
				backoff.WithBackOff(retry),
				backoff.WithMaxElapsedTime(reconnectMaxElapsed),
				backoff.WithNotify(func(err error, next time.Duration) {
					c.logger.Debug("retrying realtime channel sync",
						logging.Field("error", err),
						logging.Field("next_retry", next.String()))
				}),
			)
			if retryErr == nil || errors.Is(retryErr, context.Canceled) || errors.Is(retryErr, context.DeadlineExceeded) {
				break
			}
			if hooks.ShouldContinueAfterReconnectExhausted != nil &&
				hooks.ShouldContinueAfterReconnectExhausted(retryErr, reconnectMaxElapsed) {
				c.logger.Warn(
					"realtime reconnect window extended due to recent successful API activity",
					logging.Field("error", retryErr),
					logging.Field("max_elapsed", reconnectMaxElapsed.String()),
				)
				continue
			}
			c.logger.Warn("realtime channel sync stopped", logging.Field("error", retryErr))
			if hooks.OnStopped != nil {
				hooks.OnStopped(retryErr)
			}
			return
		}

		if ctx.Err() != nil {
			c.logger.Debug("channel config sync stopped: context canceled", logging.Field("error", ctx.Err()))
		} else {
			c.logger.Debug("channel config sync stopped")
		}
	}()

	return updates
}

func isExpectedRealtimeReconnect(err error) bool {
	return errors.Is(err, io.EOF) || errors.Is(err, pbrealtime.ErrSessionRefreshDue)
}

func (c *SentinelClient) runRealtimeConfigSession(ctx context.Context, onUpdate func([]ChannelConfig), hooks SyncHooks, prefetched *pbrealtime.Session, epoch uint64) error {
	// Acquire short-lived realtime credentials scoped to uploader subscriptions.
	auth := pbrealtime.AuthClient{
		HTTP:             c.http,
		RealtimeTokenURL: c.endpoints.RealtimeTokenURL,
		RealtimeURL:      c.endpoints.RealtimeURL,
		BearerToken:      c.token,
		Logger:           c.logger,
	}

	session := pbrealtime.Session{}
	if prefetched != nil {
		session = *prefetched
	} else {
		sessionErr := error(nil)
		session, sessionErr = auth.FetchSession(ctx)
		if sessionErr != nil {
			return sessionErr
		}
	}

	c.logger.Debug("fetched realtime session",
		logging.Field("topic", session.Topic),
		logging.Field("expires_at", session.ExpiresAt),
		logging.Field("refresh_after_seconds", session.RefreshAfterSeconds),
	)

	stream := pbrealtime.StreamClient{
		HTTP:        c.http,
		RealtimeURL: c.endpoints.RealtimeURL,
		RefreshLead: realtimeRefreshLead,
		Logger:      c.logger,
	}

	connected := false
	// StreamClient owns PB_CONNECT + subscribe + SSE transport details.
	// This layer only handles uploader-specific payload decoding and update apply.
	runErr := stream.RunSession(ctx, session, auth.Subscribe, pbrealtime.SessionHandlers{
		OnConnected: func(topic string) {
			c.logger.Info("realtime config stream connected", logging.Field("topic", topic))
			connected = true
			if hooks.OnConnected != nil {
				hooks.OnConnected(topic, session, epoch)
			}
		},
		OnMessage: func(event pbrealtime.Event) {
			cfg := uploaderConfigResponse{}
			if unmarshalErr := json.Unmarshal(event.Data, &cfg); unmarshalErr != nil {
				c.logger.Warn("failed to decode realtime config payload", logging.Field("error", unmarshalErr))
				return
			}
			channels := normalizeChannels(cfg.Channels)
			if len(channels) == 0 {
				c.logger.Warn("realtime config payload had no channels")
				return
			}
			c.logger.Debug("received realtime channel payload", logging.Field("count", len(channels)))
			onUpdate(channels)
		},
		OnUnhandled: func(event pbrealtime.Event) {
			c.logger.Debug("ignoring realtime event",
				logging.Field("event", event.Name),
				logging.Field("data", logging.FormatHTTPPayload(event.Data)),
			)
		},
	})
	if IsUnauthorized(runErr) {
		c.logger.Warn("realtime stream unauthorized; attempting short session refresh", logging.Field("error", runErr))
		refreshed, refreshErr := c.RefreshSession(ctx, session.Token)
		if refreshErr == nil {
			c.logger.Info("realtime short session refresh succeeded; reconnecting stream")
			return c.runRealtimeConfigSession(ctx, onUpdate, hooks, &refreshed, epoch)
		}
		c.logger.Warn("realtime short session refresh failed", logging.Field("error", refreshErr))
	}
	if connected && hooks.OnDisconnected != nil {
		hooks.OnDisconnected(runErr, epoch)
	}
	return runErr
}
