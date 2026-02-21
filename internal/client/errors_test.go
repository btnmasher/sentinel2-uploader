package client

import (
	"testing"

	"sentinel2-uploader/internal/pbrealtime"
)

func TestIsUnauthorized_PBRealtimeStatusError(t *testing.T) {
	err := &pbrealtime.HTTPStatusError{StatusCode: 401, Status: "401 Unauthorized"}
	if !IsUnauthorized(err) {
		t.Fatalf("IsUnauthorized(%v) = false, want true", err)
	}
}
