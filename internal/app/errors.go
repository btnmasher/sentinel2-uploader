package app

import "errors"

var (
	ErrAuthenticationFailed       = errors.New("uploader authentication failed")
	ErrRealtimeReconnectExhausted = errors.New("uploader realtime reconnect exhausted")
	ErrStartupRealtimeConnect     = errors.New("uploader startup realtime connect failed")
	ErrRealtimeHandshakeTimeout   = errors.New("uploader realtime subscribe handshake timeout")
)
