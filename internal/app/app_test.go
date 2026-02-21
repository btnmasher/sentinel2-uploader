package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"sentinel2-uploader/internal/client"
	"sentinel2-uploader/internal/config"
	"sentinel2-uploader/internal/logging"
	"sentinel2-uploader/internal/runstatus"
)

func TestWithSessionRetry_NonRealtimeUnauthorizedFallsBackToLongLivedSession(t *testing.T) {
	var refreshCalls atomic.Int32
	var longLivedCalls atomic.Int32
	var heartbeatCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/uploader/heartbeat":
			heartbeatCalls.Add(1)
			auth := strings.TrimSpace(r.Header.Get("Authorization"))
			switch auth {
			case "Bearer short-old":
				http.Error(w, "expired short session", http.StatusUnauthorized)
				return
			case "Bearer short-new":
				w.WriteHeader(http.StatusOK)
				return
			default:
				http.Error(w, "unexpected token", http.StatusUnauthorized)
				return
			}
		case "/api/uploader/session/refresh":
			refreshCalls.Add(1)
			http.Error(w, "short refresh unauthorized", http.StatusUnauthorized)
			return
		case "/api/uploader/realtime/token":
			longLivedCalls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token":                 "short-new",
				"topic":                 "uploader.config",
				"expires_at":            1_800_000_000,
				"refresh_after_seconds": 120,
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer server.Close()

	endpoints, err := config.BuildEndpoints(server.URL)
	if err != nil {
		t.Fatalf("BuildEndpoints() error = %v", err)
	}
	logger := logging.New(false)
	logger.SetTerminalOutputEnabled(false)

	statuses := make([]string, 0, 2)
	app := New(config.Options{}, client.New(server.Client(), "long-lived", endpoints, logger), logger, Callbacks{
		OnStatusChange: func(status string) {
			statuses = append(statuses, status)
		},
	})

	state := &sessionState{}
	state.setSessionToken("short-old")

	authFailures := 0
	callErr := app.withSessionRetry(context.Background(), state, func(token string) error {
		return app.client.Heartbeat(context.Background(), token)
	}, func(error) {
		authFailures++
	})
	if callErr != nil {
		t.Fatalf("withSessionRetry() error = %v, want nil", callErr)
	}

	if got := heartbeatCalls.Load(); got != 2 {
		t.Fatalf("heartbeat calls = %d, want 2", got)
	}
	if got := refreshCalls.Load(); got != 1 {
		t.Fatalf("refresh calls = %d, want 1", got)
	}
	if got := longLivedCalls.Load(); got != 1 {
		t.Fatalf("long-lived session calls = %d, want 1", got)
	}
	if authFailures != 0 {
		t.Fatalf("authFailures = %d, want 0", authFailures)
	}
	if token, ok := state.sessionToken(); !ok || token != "short-new" {
		t.Fatalf("session token = %q (ok=%v), want short-new", token, ok)
	}
	for _, status := range statuses {
		if status == runstatus.DisconnectedAuth {
			t.Fatalf("unexpected status transition to %q", runstatus.DisconnectedAuth)
		}
	}
}

func TestWithSessionRetry_LongLivedUnauthorizedTriggersAuthFailureAndDisconnect(t *testing.T) {
	var refreshCalls atomic.Int32
	var longLivedCalls atomic.Int32
	var heartbeatCalls atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/uploader/heartbeat":
			heartbeatCalls.Add(1)
			http.Error(w, "expired short session", http.StatusUnauthorized)
			return
		case "/api/uploader/session/refresh":
			refreshCalls.Add(1)
			http.Error(w, "short refresh unauthorized", http.StatusUnauthorized)
			return
		case "/api/uploader/realtime/token":
			longLivedCalls.Add(1)
			http.Error(w, "long-lived unauthorized", http.StatusUnauthorized)
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer server.Close()

	endpoints, err := config.BuildEndpoints(server.URL)
	if err != nil {
		t.Fatalf("BuildEndpoints() error = %v", err)
	}
	logger := logging.New(false)
	logger.SetTerminalOutputEnabled(false)

	statuses := make([]string, 0, 2)
	app := New(config.Options{}, client.New(server.Client(), "long-lived", endpoints, logger), logger, Callbacks{
		OnStatusChange: func(status string) {
			statuses = append(statuses, status)
		},
	})

	state := &sessionState{}
	state.setConnectedSession("short-old")

	authFailures := 0
	callErr := app.withSessionRetry(context.Background(), state, func(token string) error {
		return app.client.Heartbeat(context.Background(), token)
	}, func(error) {
		authFailures++
	})
	if callErr == nil {
		t.Fatalf("withSessionRetry() error = nil, want unauthorized error")
	}
	if !client.IsUnauthorized(callErr) {
		t.Fatalf("withSessionRetry() error = %v, want unauthorized", callErr)
	}

	if got := heartbeatCalls.Load(); got != 1 {
		t.Fatalf("heartbeat calls = %d, want 1", got)
	}
	if got := refreshCalls.Load(); got != 1 {
		t.Fatalf("refresh calls = %d, want 1", got)
	}
	if got := longLivedCalls.Load(); got != 1 {
		t.Fatalf("long-lived session calls = %d, want 1", got)
	}
	if authFailures != 1 {
		t.Fatalf("authFailures = %d, want 1", authFailures)
	}
	if _, connected := state.sessionTokenIfConnected(); connected {
		t.Fatalf("session should be marked disconnected after long-lived unauthorized")
	}

	foundDisconnectedAuth := false
	for _, status := range statuses {
		if status == runstatus.DisconnectedAuth {
			foundDisconnectedAuth = true
			break
		}
	}
	if !foundDisconnectedAuth {
		t.Fatalf("expected status transition to %q, got %v", runstatus.DisconnectedAuth, statuses)
	}
}
