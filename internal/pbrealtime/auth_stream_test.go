package pbrealtime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestAuthClient_FetchSessionAndSubscribe(t *testing.T) {
	var gotAuthHeader string
	var gotTokenHeader string
	var gotSubscribe subscribePayload

	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			switch r.URL.Path {
			case "/token":
				gotTokenHeader = r.Header.Get("X-Uploader-Token")
				body, _ := json.Marshal(Session{
					Token:               "rtok",
					RefreshAfterSeconds: 60,
				})
				return &http.Response{
					StatusCode: http.StatusOK,
					Status:     "200 OK",
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(string(body))),
					Request:    r,
				}, nil
			case "/realtime":
				gotAuthHeader = r.Header.Get("Authorization")
				if err := json.NewDecoder(r.Body).Decode(&gotSubscribe); err != nil {
					t.Fatalf("decode subscribe payload: %v", err)
				}
				return &http.Response{
					StatusCode: http.StatusNoContent,
					Status:     "204 No Content",
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("")),
					Request:    r,
				}, nil
			default:
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Status:     "404 Not Found",
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader("")),
					Request:    r,
				}, nil
			}
		}),
	}

	a := AuthClient{
		HTTP:             httpClient,
		RealtimeTokenURL: "https://example.test/token",
		RealtimeURL:      "https://example.test/realtime",
		BearerToken:      "uploader-token",
	}

	session, err := a.FetchSession(context.Background())
	if err != nil {
		t.Fatalf("FetchSession() error = %v", err)
	}
	if session.Topic != DefaultTopic {
		t.Fatalf("session.Topic = %q, want %q", session.Topic, DefaultTopic)
	}
	if gotTokenHeader != "uploader-token" {
		t.Fatalf("X-Uploader-Token = %q", gotTokenHeader)
	}

	err = a.Subscribe(context.Background(), "client-1", "sess-1", []string{"uploader.config"})
	if err != nil {
		t.Fatalf("Subscribe() error = %v", err)
	}
	if gotAuthHeader != "Bearer sess-1" {
		t.Fatalf("Authorization = %q", gotAuthHeader)
	}
	if gotSubscribe.ClientID != "client-1" || len(gotSubscribe.Subscriptions) != 1 {
		t.Fatalf("subscribe payload = %#v", gotSubscribe)
	}
}

func TestAuthClient_FetchSession_MissingTokenFails(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			body, _ := json.Marshal(Session{})
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(string(body))),
				Request:    r,
			}, nil
		}),
	}

	a := AuthClient{
		HTTP:             httpClient,
		RealtimeTokenURL: "https://example.test/token",
	}
	if _, err := a.FetchSession(context.Background()); err == nil {
		t.Fatalf("FetchSession() expected missing token error")
	}
}

func TestAuthClient_FetchSession_UnauthorizedTypedError(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Status:     "401 Unauthorized",
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"message":"invalid uploader token"}`)),
				Request:    r,
			}, nil
		}),
	}

	a := AuthClient{
		HTTP:             httpClient,
		RealtimeTokenURL: "https://example.test/token",
	}
	_, err := a.FetchSession(context.Background())
	if err == nil {
		t.Fatalf("FetchSession() expected error")
	}

	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) {
		t.Fatalf("error type = %T, want *HTTPStatusError", err)
	}
	if statusErr.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status code = %d, want %d", statusErr.StatusCode, http.StatusUnauthorized)
	}
}

func TestReadSSEEvents_ParsesMultilineData(t *testing.T) {
	in := strings.NewReader("event: test\ndata: line1\ndata: line2\n\n")
	out := make(chan Event, 2)
	errs := make(chan error, 1)
	readSSEEvents(in, out, errs)

	ev := <-out
	if ev.Name != "test" || string(ev.Data) != "line1\nline2" {
		t.Fatalf("event = %#v", ev)
	}
	if err := <-errs; err != io.EOF {
		t.Fatalf("stream err = %v, want io.EOF", err)
	}
}

func TestStreamClient_RunSession_ConnectMessageAndUnhandled(t *testing.T) {
	httpClient := &http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			pr, pw := io.Pipe()
			go func() {
				defer pw.Close()
				_, _ = io.WriteString(pw, "event: PB_CONNECT\ndata: {\"clientId\":\"cid-1\"}\n\n")
				time.Sleep(5 * time.Millisecond)
				_, _ = io.WriteString(pw, "event: PB_CONNECT\ndata: {\"clientId\":\"cid-1\"}\n\n")
				time.Sleep(5 * time.Millisecond)
				_, _ = io.WriteString(pw, "event: uploader.config\ndata: {\"channels\":[{\"id\":\"1\"}]}\n\n")
				time.Sleep(5 * time.Millisecond)
				_, _ = io.WriteString(pw, "event: something_else\ndata: {}\n\n")
				time.Sleep(5 * time.Millisecond)
			}()
			h := make(http.Header)
			h.Set("Content-Type", "text/event-stream")
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     h,
				Body:       pr,
				Request:    r,
			}, nil
		}),
	}

	var subscribeCalls int
	var connectedTopic string
	var messageCount int
	var unhandledCount int
	var unhandledNames []string

	stream := StreamClient{
		HTTP:        httpClient,
		RealtimeURL: "https://example.test/realtime",
		RefreshLead: time.Second,
	}
	err := stream.RunSession(
		context.Background(),
		Session{
			Token:               "session-token",
			Topic:               "uploader.config",
			RefreshAfterSeconds: 3600,
		},
		func(_ context.Context, clientID string, sessionToken string, topics []string) error {
			subscribeCalls++
			if clientID != "cid-1" || sessionToken != "session-token" || len(topics) != 1 || topics[0] != "uploader.config" {
				t.Fatalf("subscribe args clientID=%q token=%q topics=%v", clientID, sessionToken, topics)
			}
			return nil
		},
		SessionHandlers{
			OnConnected: func(topic string) { connectedTopic = topic },
			OnMessage:   func(_ Event) { messageCount++ },
			OnUnhandled: func(ev Event) {
				unhandledCount++
				unhandledNames = append(unhandledNames, ev.Name)
			},
		},
	)
	if err != io.EOF {
		t.Fatalf("RunSession() err = %v, want io.EOF", err)
	}
	if subscribeCalls != 1 {
		t.Fatalf("subscribe calls = %d, want 1 (duplicate PB_CONNECT ignored)", subscribeCalls)
	}
	if connectedTopic != "uploader.config" {
		t.Fatalf("connected topic = %q", connectedTopic)
	}
	if messageCount != 1 {
		t.Fatalf("message count = %d, want 1 (unhandled=%v)", messageCount, unhandledNames)
	}
	if unhandledCount != 1 {
		t.Fatalf("unhandled count = %d, want 1", unhandledCount)
	}
}

func TestBuildSubscribeTopics_DedupesAndSkipsEmpty(t *testing.T) {
	got := buildSubscribeTopics("uploader.config", []string{
		"",
		"realtime.keepalive",
		" uploader.config ",
		"realtime.keepalive",
	})
	if len(got) != 2 {
		t.Fatalf("len(topics) = %d, want 2 (%v)", len(got), got)
	}
	if got[0] != "uploader.config" || got[1] != "realtime.keepalive" {
		t.Fatalf("topics = %v", got)
	}
}
