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

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestNormalizeChannels_DedupesSortsAndTrims(t *testing.T) {
	got := normalizeChannels([]ChannelConfig{
		{ID: " 2 ", Name: " beta "},
		{ID: "1", Name: "Alpha"},
		{ID: "1", Name: "alpha"},
		{ID: "3", Name: "  "},
		{ID: "", Name: "Gamma"},
		{ID: "2", Name: "beta"},
	})

	want := []ChannelConfig{
		{ID: "1", Name: "Alpha"},
		{ID: "2", Name: "beta"},
	}
	if !channelsEqual(got, want) {
		t.Fatalf("normalizeChannels() = %#v, want %#v", got, want)
	}
}

func TestFetchChannels_SetsTokenHeaderAndNormalizesResponse(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			if got := r.Header.Get("Authorization"); got != "Bearer session-123" {
				t.Fatalf("Authorization = %q, want Bearer session-123", got)
			}
			body, _ := json.Marshal(map[string]any{
				"channels": []map[string]string{
					{"id": "2", "name": "Bravo"},
					{"id": "1", "name": " Alpha "},
					{"id": "1", "name": "alpha"},
					{"id": "", "name": "skip"},
				},
			})
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(string(body))),
				Request:    r,
			}, nil
		}),
	}

	c := New(
		httpClient,
		"token-123",
		config.APIEndpoints{ConfigURL: "https://example.test/uploader/config"},
		logging.New(false),
	)

	got, err := c.FetchChannels(context.Background(), "session-123")
	if err != nil {
		t.Fatalf("FetchChannels() error = %v", err)
	}
	want := []ChannelConfig{
		{ID: "1", Name: "Alpha"},
		{ID: "2", Name: "Bravo"},
	}
	if !channelsEqual(got, want) {
		t.Fatalf("FetchChannels() = %#v, want %#v", got, want)
	}
}

func TestFetchChannels_HTTPErrorAndInvalidJSON(t *testing.T) {
	httpErrClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Status:     "401 Unauthorized",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"error":"nope"}`)),
				Request:    r,
			}, nil
		}),
	}

	c1 := New(
		httpErrClient,
		"token-123",
		config.APIEndpoints{ConfigURL: "https://example.test/uploader/config"},
		logging.New(false),
	)
	if _, err := c1.FetchChannels(context.Background(), "session-123"); err == nil {
		t.Fatalf("FetchChannels() expected error on HTTP status >= 400")
	}

	invalidJSONClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{not-json}`)),
				Request:    r,
			}, nil
		}),
	}

	c2 := New(
		invalidJSONClient,
		"token-123",
		config.APIEndpoints{ConfigURL: "https://example.test/uploader/config"},
		logging.New(false),
	)
	if _, err := c2.FetchChannels(context.Background(), "session-123"); err == nil {
		t.Fatalf("FetchChannels() expected error on invalid JSON")
	}
}
