package runstatus

import "strings"

const (
	Authenticated    = "Authenticated"
	ChannelsReceived = "Channels received"
	Connected        = "Connected"
	Reconnecting     = "Reconnecting"
	Disconnected     = "Disconnected"
	DisconnectedAuth = "Disconnected (auth)"
)

const (
	KeyAuthenticated    = "authenticated"
	KeyChannelsReceived = "channels received"
	KeyConnected        = "connected"
	KeyReconnecting     = "reconnecting"
	KeyDisconnected     = "disconnected"
	KeyDisconnectedAuth = "disconnected (auth)"
)

func Key(status string) string {
	return strings.ToLower(strings.TrimSpace(status))
}
