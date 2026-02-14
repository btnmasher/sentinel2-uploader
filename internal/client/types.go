package client

type SubmitPayload struct {
	Text      string `json:"text"`
	ChannelID string `json:"channel_id"`
}

type ChannelConfig struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type uploaderConfigResponse struct {
	Channels []ChannelConfig `json:"channels"`
}
