package pbrealtime

type Session struct {
	Token               string `json:"token"`
	Topic               string `json:"topic"`
	ExpiresAt           int64  `json:"expires_at"`
	RefreshAfterSeconds int64  `json:"refresh_after_seconds"`
}

type Event struct {
	Name string
	Data []byte
}

type connectPayload struct {
	ClientID string `json:"clientId"`
}

type subscribePayload struct {
	ClientID      string   `json:"clientId"`
	Subscriptions []string `json:"subscriptions"`
}
