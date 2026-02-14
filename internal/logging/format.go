package logging

import (
	"bytes"
	"encoding"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"
	"sort"
	"strings"
)

const clipLimit = 240

func Truncate(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "\r", " ")
	if value == "" {
		return "<empty>"
	}
	if len(value) > clipLimit {
		return value[:clipLimit] + "..."
	}
	return value
}

func AppendWithLimit(current string, next string, limit int) string {
	combined := current + next
	if len(combined) > limit {
		return combined[len(combined)-limit:]
	}
	return combined
}

func FormatEventLine(event Event) string {
	ts := event.Time.Format("15:04:05")
	level := strings.ToUpper(event.Level.String())
	fields := ""
	if len(event.Fields) > 0 {
		keys := orderedFieldKeys(event.Level, event.Fields)
		parts := make([]string, 0, len(keys))
		for _, key := range keys {
			parts = append(parts, fmt.Sprintf("%s=%s", key, formatFieldValue(event.Fields[key])))
		}
		fields = " " + strings.Join(parts, " ")
	}
	return fmt.Sprintf("%s [%s] %s%s\n", ts, level, event.Message, fields)
}

func formatFieldValue(value any) string {
	if value == nil {
		return "<nil>"
	}
	if pretty, ok := prettyJSONString(value); ok {
		return pretty
	}
	switch v := value.(type) {
	case string:
		return maybePrettyJSONString(v)
	case []byte:
		return maybePrettyJSONString(string(v))
	default:
		kind := reflect.ValueOf(value).Kind()
		switch kind {
		case reflect.Map, reflect.Slice, reflect.Array, reflect.Struct:
			if payload, err := marshalPrettyJSON(value); err == nil {
				return payload
			}
		}
		return fmt.Sprintf("%v", value)
	}
}

func maybePrettyJSONString(input string) string {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return input
	}
	// Decode JSON-shaped strings only; leave normal text untouched.
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") || strings.HasPrefix(trimmed, "\"") {
		return FormatHTTPPayload([]byte(trimmed))
	}
	return input
}

func marshalPrettyJSON(value any) (string, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(value); err != nil {
		return "", err
	}
	return strings.TrimSpace(buf.String()), nil
}

func prettyJSONString(value any) (string, bool) {
	if value == nil {
		return "", false
	}
	if errValue, ok := value.(error); ok && errValue != nil {
		return prettyJSONString(errValue.Error())
	}
	if textValue, ok := value.(encoding.TextMarshaler); ok {
		if text, err := textValue.MarshalText(); err == nil {
			return prettyJSONString(string(text))
		}
	}

	rv := reflect.ValueOf(value)
	for rv.IsValid() && rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return "", false
		}
		rv = rv.Elem()
	}
	if rv.IsValid() {
		value = rv.Interface()
	}

	switch v := value.(type) {
	case string:
		if out, ok := parseJSONStringCandidate(v); ok {
			return out, true
		}
	case []byte:
		return prettyJSONString(string(v))
	default:
		kind := reflect.ValueOf(value).Kind()
		switch kind {
		case reflect.Map, reflect.Slice, reflect.Array, reflect.Struct:
			if out, err := marshalPrettyJSON(value); err == nil {
				return out, true
			}
		}
	}
	return "", false
}

func parseJSONStringCandidate(input string) (string, bool) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return "", false
	}

	if decoded, ok := decodeJSONContainer(trimmed); ok {
		if out, err := marshalPrettyJSON(decoded); err == nil {
			return out, true
		}
	}
	return "", false
}

func decodeJSONContainer(input string) (any, bool) {
	var decoded any
	if err := json.Unmarshal([]byte(input), &decoded); err != nil {
		return nil, false
	}
	switch decoded.(type) {
	case map[string]any, []any:
		return decoded, true
	default:
		return nil, false
	}
}

func orderedFieldKeys(_ slog.Level, fields map[string]any) []string {
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	inline := make([]string, 0, len(keys))
	jsonKeys := make([]string, 0, len(keys))
	payloadJSONKeys := make([]string, 0, len(keys))
	for _, key := range keys {
		if _, ok := prettyJSONString(fields[key]); ok {
			if isPayloadFieldKey(key) {
				payloadJSONKeys = append(payloadJSONKeys, key)
			} else {
				jsonKeys = append(jsonKeys, key)
			}
			continue
		}
		inline = append(inline, key)
	}
	ordered := append(inline, jsonKeys...)
	return append(ordered, payloadJSONKeys...)
}

func isPayloadFieldKey(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "payload", "response", "response_body", "body", "data":
		return true
	default:
		return false
	}
}
