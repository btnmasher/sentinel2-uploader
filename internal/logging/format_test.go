package logging

import "testing"

type structPayload struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func TestPrettyJSONString_EmbeddedJSONSuffixIgnored(t *testing.T) {
	input := `500 Internal Server Error: {"message":"failed","status":500}`
	if _, ok := prettyJSONString(input); ok {
		t.Fatalf("expected embedded JSON suffix to be ignored")
	}
}

func TestOrderedFieldKeys_PayloadJSONLast(t *testing.T) {
	fields := map[string]any{
		"status":  "500",
		"payload": `{"message":"failed","status":500}`,
		"error":   "submit failed",
	}
	keys := orderedFieldKeys(0, fields)
	if len(keys) != 3 {
		t.Fatalf("unexpected keys length: %d", len(keys))
	}
	if keys[len(keys)-1] != "payload" {
		t.Fatalf("expected payload last, got %v", keys)
	}
}

func TestPrettyJSONString_StructField(t *testing.T) {
	pretty, ok := prettyJSONString(structPayload{Name: "abc", Count: 2})
	if !ok {
		t.Fatalf("expected struct to be rendered as pretty JSON")
	}
	if pretty == "" || pretty[0] != '{' {
		t.Fatalf("expected pretty JSON object, got %q", pretty)
	}
}
