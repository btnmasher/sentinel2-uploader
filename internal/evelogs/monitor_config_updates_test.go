package evelogs

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"

	"sentinel2-uploader/internal/client"
	"sentinel2-uploader/internal/logging"
)

func TestPrepare_TracksExistingLatestLogsPerConfiguredChannelCharacter(t *testing.T) {
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
	write("Intel_20260216_120000_charA.txt", base.Add(-2*time.Minute))
	intelLatest := write("Intel_20260216_120100_charA.txt", base.Add(-1*time.Minute))
	otherLatest := write("Other_20260216_120200_charB.txt", base)

	logger := logging.New(false)
	logger.SetTerminalOutputEnabled(false)

	monitor := NewMonitor(
		MonitorOptions{
			LogDir:   dir,
			Channels: []client.ChannelConfig{{ID: "intel", Name: "Intel"}, {ID: "other", Name: "Other"}},
		},
		logger,
		MonitorCallbacks{},
	)

	if err := monitor.Prepare(); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}

	if len(monitor.tracked) != 2 {
		t.Fatalf("tracked len = %d, want 2", len(monitor.tracked))
	}
	if _, ok := monitor.tracked[intelLatest]; !ok {
		t.Fatalf("missing latest intel file in tracked set: %s", intelLatest)
	}
	if _, ok := monitor.tracked[otherLatest]; !ok {
		t.Fatalf("missing latest other file in tracked set: %s", otherLatest)
	}
}

func TestHandleChannelUpdate_NewlyAddedChannelTracksNewestLogfile(t *testing.T) {
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
	intelLatest := write("Intel_20260216_120100_charA.txt", base.Add(-1*time.Minute))
	write("Other_20260216_120000_charB.txt", base.Add(-2*time.Minute))
	otherLatest := write("Other_20260216_120200_charB.txt", base)

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

	if err := monitor.Prepare(); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if len(monitor.tracked) != 1 {
		t.Fatalf("tracked len after initial prepare = %d, want 1", len(monitor.tracked))
	}
	if _, ok := monitor.tracked[intelLatest]; !ok {
		t.Fatalf("expected intel latest file tracked initially")
	}

	monitor.handleChannelUpdate([]client.ChannelConfig{
		{ID: "intel", Name: "Intel"},
		{ID: "other", Name: "Other"},
	})

	if len(monitor.tracked) != 2 {
		t.Fatalf("tracked len after channel add = %d, want 2", len(monitor.tracked))
	}
	if _, ok := monitor.tracked[otherLatest]; !ok {
		t.Fatalf("expected newest logfile for newly added channel to be tracked")
	}
}

func TestHandleChannelUpdate_RemovedChannelStopsTrackingAndReporting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Intel_20260216_120100_charA.txt")
	if err := os.WriteFile(path, []byte("header\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}

	logger := logging.New(false)
	logger.SetTerminalOutputEnabled(false)

	var untracked []string
	reportCount := 0
	monitor := NewMonitor(
		MonitorOptions{
			LogDir:   dir,
			Channels: []client.ChannelConfig{{ID: "intel", Name: "Intel"}},
		},
		logger,
		MonitorCallbacks{
			OnReport: func(ReportEvent) error {
				reportCount++
				return nil
			},
			OnUntracked: func(p string) {
				untracked = append(untracked, filepath.Clean(p))
			},
		},
	)

	if err := monitor.Prepare(); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if len(monitor.tracked) != 1 {
		t.Fatalf("tracked len after prepare = %d, want 1", len(monitor.tracked))
	}

	monitor.handleChannelUpdate(nil)
	if len(monitor.tracked) != 0 {
		t.Fatalf("tracked len after channel removal = %d, want 0", len(monitor.tracked))
	}
	if len(untracked) != 1 || untracked[0] != filepath.Clean(path) {
		t.Fatalf("untracked callback = %v, want [%s]", untracked, filepath.Clean(path))
	}

	line := "[ " + time.Now().UTC().Format("2006.01.02 15:04:05") + " ] Pilot > test after removal\n"
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open append %s: %v", path, err)
	}
	if _, err := f.WriteString(line); err != nil {
		_ = f.Close()
		t.Fatalf("append line: %v", err)
	}
	_ = f.Close()

	monitor.handleWatcherEvent(fsnotify.Event{Name: path, Op: fsnotify.Write})
	if reportCount != 0 {
		t.Fatalf("reportCount = %d, want 0 after channel removal", reportCount)
	}
}

func TestHandleWatcherEvent_WriteOnTrackedFileEmitsReport(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "Intel_20260216_120100_charA.txt")
	if err := os.WriteFile(path, []byte("header\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}

	logger := logging.New(false)
	logger.SetTerminalOutputEnabled(false)

	reportCount := 0
	var last ReportEvent
	monitor := NewMonitor(
		MonitorOptions{
			LogDir:   dir,
			Channels: []client.ChannelConfig{{ID: "intel", Name: "Intel"}},
		},
		logger,
		MonitorCallbacks{
			OnReport: func(event ReportEvent) error {
				reportCount++
				last = event
				return nil
			},
		},
	)

	if err := monitor.Prepare(); err != nil {
		t.Fatalf("Prepare() error = %v", err)
	}
	if len(monitor.tracked) != 1 {
		t.Fatalf("tracked len after prepare = %d, want 1", len(monitor.tracked))
	}

	lineTime := time.Now().UTC()
	line := "[ " + lineTime.Format("2006.01.02 15:04:05") + " ] Pilot > fresh report\n"
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open append %s: %v", path, err)
	}
	if _, err := f.WriteString(line); err != nil {
		_ = f.Close()
		t.Fatalf("append line: %v", err)
	}
	_ = f.Close()

	monitor.handleWatcherEvent(fsnotify.Event{Name: path, Op: fsnotify.Write})

	if reportCount != 1 {
		t.Fatalf("reportCount = %d, want 1", reportCount)
	}
	if filepath.Clean(last.SourcePath) != filepath.Clean(path) {
		t.Fatalf("SourcePath = %q, want %q", last.SourcePath, filepath.Clean(path))
	}
	if last.Channel.ID != "intel" {
		t.Fatalf("Channel.ID = %q, want intel", last.Channel.ID)
	}
	if last.CharacterID != "charA" {
		t.Fatalf("CharacterID = %q, want charA", last.CharacterID)
	}
}
