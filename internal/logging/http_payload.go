package logging

import (
	"bytes"
	"encoding/json"
	"strings"
)

// FormatHTTPPayload normalizes HTTP response payloads for log output.
// It attempts to decode JSON so escaped characters are rendered cleanly.
func FormatHTTPPayload(raw []byte) string {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return "<empty>"
	}

	// JSON string body: "\"{...}\"" or "\"error\""
	var quoted string
	if err := json.Unmarshal([]byte(trimmed), &quoted); err == nil {
		trimmed = strings.TrimSpace(quoted)
	}

	// JSON object/array body: pretty-print without HTML escaping.
	var value any
	if err := json.Unmarshal([]byte(trimmed), &value); err == nil {
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if encErr := enc.Encode(value); encErr == nil {
			return strings.TrimSpace(buf.String())
		}
	}

	return trimmed
}
