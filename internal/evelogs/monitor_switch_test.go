package evelogs

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"sentinel2-uploader/internal/client"
	"sentinel2-uploader/internal/logging"
)

func TestMaybeTrackEventPath_ReplacesOlderTrackedFileForSameChannelAndCharacter(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, mod time.Time) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("line\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		if err := os.Chtimes(path, mod, mod); err != nil {
			t.Fatalf("chtimes %s: %v", name, err)
		}
		return path
	}

	base := time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC)
	oldPath := write("Intel_20260216_120000_charA.txt", base)
	newPath := write("Intel_20260216_120100_charA.txt", base.Add(30*time.Second))

	logger := logging.New(false)
	logger.SetTerminalOutputEnabled(false)

	monitor := NewMonitor(
		MonitorOptions{
			LogDir:   dir,
			Channels: []client.ChannelConfig{{ID: "intel", Name: "Intel"}},
		},
		logger,
		MonitorCallbacks{},
	)

	channel, ok := ResolveChannelForPath(oldPath, monitor.channels)
	if !ok {
		t.Fatalf("ResolveChannelForPath(%q) expected match", oldPath)
	}
	monitor.addTrackedLog(LogSelection{Path: oldPath, Channel: channel})
	if _, ok := monitor.tracked[oldPath]; !ok {
		t.Fatalf("expected old file to be tracked")
	}

	monitor.maybeTrackEventPath(newPath)

	if _, ok := monitor.tracked[newPath]; !ok {
		t.Fatalf("expected new file to be tracked")
	}
	if _, ok := monitor.tracked[oldPath]; ok {
		t.Fatalf("expected old file to be untracked after newer file appears")
	}
}
