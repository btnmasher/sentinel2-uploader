package logging

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"log/slog"
)

func TestDefaultLogDirPathSuffix(t *testing.T) {
	path, err := DefaultLogDirPath()
	if err != nil {
		t.Fatalf("DefaultLogDirPath() error = %v", err)
	}
	if got, want := path, filepath.Join("sentinel2", "uploader", "logs"); !strings.HasSuffix(got, want) {
		t.Fatalf("DefaultLogDirPath() = %q, want suffix %q", got, want)
	}
}

func TestFileSinkWritesJSONLAndRotates(t *testing.T) {
	tmp := t.TempDir()
	sink := &fileSink{
		dir:        tmp,
		sessionTag: "20260221-120000",
		maxBytes:   180,
	}
	if err := sink.rotateLocked(); err != nil {
		t.Fatalf("rotateLocked() error = %v", err)
	}

	event := Event{
		Time:    time.Unix(1700000000, 123456789),
		Level:   slog.LevelDebug,
		Message: "session log line",
		Fields: map[string]any{
			"count":  7,
			"status": "ok",
		},
	}

	for i := 0; i < 6; i++ {
		if err := sink.WriteEvent(event); err != nil {
			t.Fatalf("WriteEvent() error = %v", err)
		}
	}
	if err := sink.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) < 2 {
		t.Fatalf("expected rotation to create multiple files, got %d", len(entries))
	}

	foundLine := false
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".jsonl") {
			t.Fatalf("unexpected log filename %q", entry.Name())
		}
		data, readErr := os.ReadFile(filepath.Join(tmp, entry.Name()))
		if readErr != nil {
			t.Fatalf("ReadFile(%q) error = %v", entry.Name(), readErr)
		}
		for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
			if strings.TrimSpace(line) == "" {
				continue
			}
			var decoded map[string]any
			if unmarshalErr := json.Unmarshal([]byte(line), &decoded); unmarshalErr != nil {
				t.Fatalf("invalid json line %q: %v", line, unmarshalErr)
			}
			foundLine = true
		}
	}
	if !foundLine {
		t.Fatalf("expected at least one JSON line")
	}
}

func TestLoggerCloseStopsFilePersistence(t *testing.T) {
	tmp := t.TempDir()

	logger := New(true)
	logger.SetTerminalOutputEnabled(false)

	sink := &fileSink{
		dir:        tmp,
		sessionTag: "20260221-120001",
		maxBytes:   1024,
	}
	if err := sink.rotateLocked(); err != nil {
		t.Fatalf("rotateLocked() error = %v", err)
	}
	logger.fileSink = sink

	logger.Info("before close")
	if err := logger.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	logger.Info("after close")

	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("expected at least one log file")
	}
	path := filepath.Join(tmp, entries[0].Name())
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer f.Close()

	content, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "before close") {
		t.Fatalf("expected pre-close event in log content")
	}
	if strings.Contains(text, "after close") {
		t.Fatalf("did not expect post-close event in log content")
	}
}
