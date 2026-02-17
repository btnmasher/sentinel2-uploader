package client

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"sentinel2-uploader/internal/config"
	"sentinel2-uploader/internal/logging"
)

func TestHeartbeat_SetsTokenHeader(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if got := r.Method; got != http.MethodPost {
				t.Fatalf("method = %q, want POST", got)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer realtime-session-token" {
				t.Fatalf("Authorization = %q, want Bearer realtime-session-token", got)
			}
			return &http.Response{
				StatusCode: http.StatusNoContent,
				Status:     "204 No Content",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    r,
			}, nil
		}),
	}

	c := New(
		httpClient,
		"token-123",
		config.APIEndpoints{HeartbeatURL: "https://example.test/uploader/heartbeat"},
		logging.New(false),
	)
	if err := c.Heartbeat(context.Background(), "realtime-session-token"); err != nil {
		t.Fatalf("Heartbeat() error = %v", err)
	}
}

func TestHeartbeat_ReturnsErrorOnHTTPFailure(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Status:     "401 Unauthorized",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"error":"unauthorized"}`)),
				Request:    r,
			}, nil
		}),
	}

	c := New(
		httpClient,
		"token-123",
		config.APIEndpoints{HeartbeatURL: "https://example.test/uploader/heartbeat"},
		logging.New(false),
	)
	if err := c.Heartbeat(context.Background(), "realtime-session-token"); err == nil {
		t.Fatalf("Heartbeat() expected error for HTTP status >= 400")
	}
}

func TestHeartbeat_RequiresSessionToken(t *testing.T) {
	c := New(
		http.DefaultClient,
		"token-123",
		config.APIEndpoints{HeartbeatURL: "https://example.test/uploader/heartbeat"},
		logging.New(false),
	)
	if err := c.Heartbeat(context.Background(), "   "); err == nil {
		t.Fatalf("Heartbeat() expected error for empty session token")
	}
}
