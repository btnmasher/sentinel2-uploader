package logging

import "log/slog"

func resolveAttr(attr slog.Attr) (string, any) {
	if attr.Key == "" {
		return "", nil
	}
	value := attr.Value.Resolve()
	switch value.Kind() {
	case slog.KindGroup:
		inner := map[string]any{}
		for _, groupAttr := range value.Group() {
			key, val := resolveAttr(groupAttr)
			if key != "" {
				inner[key] = val
			}
		}
		return attr.Key, inner
	default:
		return attr.Key, value.Any()
	}
}

func attrsToMap(attrs []slog.Attr) map[string]any {
	if len(attrs) == 0 {
		return nil
	}
	values := map[string]any{}
	for _, attr := range attrs {
		key, value := resolveAttr(attr)
		if key == "" {
			continue
		}
		values[key] = value
	}
	if len(values) == 0 {
		return nil
	}
	return values
}
