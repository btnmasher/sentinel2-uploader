package config

import "testing"

func TestBuildEndpoints_NormalizeAPIBaseURL(t *testing.T) {
	tests := []struct {
		name string
		base string
		want string
	}{
		{name: "root host", base: "http://127.0.0.1:8090", want: "http://127.0.0.1:8090/api"},
		{name: "already api", base: "http://127.0.0.1:8090/api", want: "http://127.0.0.1:8090/api"},
		{name: "api with trailing", base: "http://127.0.0.1:8090/api/", want: "http://127.0.0.1:8090/api"},
		{name: "pasted config endpoint", base: "http://127.0.0.1:8090/api/uploader/config", want: "http://127.0.0.1:8090/api"},
		{name: "pasted token endpoint", base: "http://127.0.0.1:8090/api/uploader/realtime/token", want: "http://127.0.0.1:8090/api"},
		{name: "subpath api endpoint drops extra path", base: "https://example.com/s2/api/uploader/config", want: "https://example.com/api"},
		{name: "query fragment dropped", base: "https://example.com/anything?x=1#y", want: "https://example.com/api"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoints, err := BuildEndpoints(tt.base)
			if err != nil {
				t.Fatalf("BuildEndpoints failed: %v", err)
			}
			if endpoints.BaseURL != tt.want {
				t.Fatalf("BaseURL = %q, want %q", endpoints.BaseURL, tt.want)
			}
			if endpoints.RealtimeTokenURL != tt.want+"/uploader/realtime/token" {
				t.Fatalf("RealtimeTokenURL = %q", endpoints.RealtimeTokenURL)
			}
			if endpoints.HeartbeatURL != tt.want+"/uploader/heartbeat" {
				t.Fatalf("HeartbeatURL = %q", endpoints.HeartbeatURL)
			}
			if endpoints.SessionRefreshURL != tt.want+"/uploader/session/refresh" {
				t.Fatalf("SessionRefreshURL = %q", endpoints.SessionRefreshURL)
			}
		})
	}
}

func TestBuildEndpoints_InvalidScheme(t *testing.T) {
	tests := []string{
		"ftp://example.com",
		"ws://example.com",
		"file:///tmp/sentinel",
	}
	for _, base := range tests {
		t.Run(base, func(t *testing.T) {
			if _, err := BuildEndpoints(base); err == nil {
				t.Fatalf("expected error for %q", base)
			}
		})
	}
}
