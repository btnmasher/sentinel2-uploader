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

func TestRefreshSession_UsesSessionBearerToken(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if got := r.Method; got != http.MethodPost {
				t.Fatalf("method = %q, want POST", got)
			}
			if got := r.Header.Get("Authorization"); got != "Bearer session-123" {
				t.Fatalf("Authorization = %q, want Bearer session-123", got)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"token":"new-session-token","topic":"uploader.config","expires_at":123,"refresh_after_seconds":10}`,
				)),
				Request: r,
			}, nil
		}),
	}

	c := New(
		httpClient,
		"token-123",
		config.APIEndpoints{SessionRefreshURL: "https://example.test/uploader/session/refresh"},
		logging.New(false),
	)
	session, err := c.RefreshSession(context.Background(), "session-123")
	if err != nil {
		t.Fatalf("RefreshSession() error = %v", err)
	}
	if session.Token != "new-session-token" {
		t.Fatalf("session token = %q", session.Token)
	}
}
