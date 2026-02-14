package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"sentinel2-uploader/internal/config"
	"sentinel2-uploader/internal/logging"
)

func TestSubmit_SetsHeadersAndPayload(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if got := r.Method; got != http.MethodPut {
				t.Fatalf("method = %q, want PUT", got)
			}
			if got := r.Header.Get("X-Uploader-Token"); got != "token-123" {
				t.Fatalf("X-Uploader-Token = %q, want token-123", got)
			}
			if got := r.Header.Get("Content-Type"); got != "application/json" {
				t.Fatalf("Content-Type = %q, want application/json", got)
			}
			var payload SubmitPayload
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode payload: %v", err)
			}
			if payload.ChannelID != "abc" || payload.Text != "report text" {
				t.Fatalf("payload = %#v", payload)
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
		config.APIEndpoints{SubmitURL: "https://example.test/uploader/submit"},
		logging.New(false),
	)
	if err := c.Submit(context.Background(), SubmitPayload{ChannelID: "abc", Text: "report text"}); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
}

func TestSubmit_ReturnsErrorOnHTTPFailure(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusBadRequest,
				Status:     "400 Bad Request",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"error":"bad"}`)),
				Request:    r,
			}, nil
		}),
	}

	c := New(
		httpClient,
		"token-123",
		config.APIEndpoints{SubmitURL: "https://example.test/uploader/submit"},
		logging.New(false),
	)
	if err := c.Submit(context.Background(), SubmitPayload{ChannelID: "abc", Text: "report text"}); err == nil {
		t.Fatalf("Submit() expected error for HTTP status >= 400")
	}
}
