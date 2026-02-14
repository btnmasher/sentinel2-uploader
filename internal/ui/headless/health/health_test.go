package health

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"sentinel2-uploader/internal/client"
)

func TestCompute_ClassifiesChannelHealthByFileAge(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 2, 14, 12, 0, 0, 0, time.UTC)
	write := func(name string, age time.Duration) {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		ts := now.Add(-age)
		if err := os.Chtimes(path, ts, ts); err != nil {
			t.Fatalf("chtimes %s: %v", name, err)
		}
	}

	write("Intel_20260214_100000_char1.txt", 5*time.Minute)    // active
	write("Staging_20260214_090000_char2.txt", 20*time.Minute) // warn
	write("Deep_20260214_080000_char3.txt", 2*time.Hour)       // stale

	channels := []client.ChannelConfig{
		{ID: "1", Name: "Intel"},
		{ID: "2", Name: "Staging"},
		{ID: "3", Name: "Deep"},
		{ID: "4", Name: "Missing"},
	}

	rows, msg := Compute(dir, channels, now)
	if msg != "" {
		t.Fatalf("Compute() message = %q, want empty", msg)
	}
	if len(rows) != 4 {
		t.Fatalf("rows len = %d, want 4", len(rows))
	}

	kinds := map[string]Kind{}
	for _, r := range rows {
		kinds[r.Name] = r.Kind
	}
	if kinds["Intel"] != Active || kinds["Staging"] != Warn || kinds["Deep"] != Stale || kinds["Missing"] != Missing {
		t.Fatalf("unexpected kinds: %#v", kinds)
	}
}

func TestCompute_ReportsMissingOrInvalidDirectory(t *testing.T) {
	rows, msg := Compute("", nil, time.Now())
	if len(rows) != 0 || !strings.Contains(msg, "not configured") {
		t.Fatalf("Compute(empty) rows=%d msg=%q", len(rows), msg)
	}

	path := filepath.Join(t.TempDir(), "notadir.txt")
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	rows, msg = Compute(path, nil, time.Now())
	if len(rows) != 0 || !strings.Contains(msg, "not a directory") {
		t.Fatalf("Compute(file) rows=%d msg=%q", len(rows), msg)
	}
}
