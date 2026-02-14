package evelogs

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"sentinel2-uploader/internal/client"
)

func TestParseReportLine_ValidAndInvalid(t *testing.T) {
	line := "[ 2026.02.14 12:34:56 ] Pilot > Contact in Jita"
	got, ok := ParseReportLine(line)
	if !ok {
		t.Fatalf("ParseReportLine() expected ok")
	}
	if got.Author != "Pilot" || got.Message != "Contact in Jita" {
		t.Fatalf("ParseReportLine() = %#v", got)
	}
	if got.Time.UTC().Format("2006-01-02 15:04:05") != "2026-02-14 12:34:56" {
		t.Fatalf("unexpected parsed time: %v", got.Time)
	}

	if _, ok := ParseReportLine("not a report"); ok {
		t.Fatalf("ParseReportLine() expected false for invalid line")
	}
}

func TestResolveChannelForPath_CaseInsensitive(t *testing.T) {
	channels := []client.ChannelConfig{
		{ID: "intel-id", Name: "Intel"},
	}
	got, ok := ResolveChannelForPath("/tmp/INTEL_20260214_123456_9001.txt", channels)
	if !ok {
		t.Fatalf("ResolveChannelForPath() expected match")
	}
	if got.ID != "intel-id" {
		t.Fatalf("ResolveChannelForPath() ID = %q, want intel-id", got.ID)
	}
}

func TestFindLogs_OnlyLatestPerChannelAndCharacter(t *testing.T) {
	dir := t.TempDir()
	write := func(name string, mod time.Time) string {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
		if err := os.Chtimes(path, mod, mod); err != nil {
			t.Fatalf("chtimes %s: %v", name, err)
		}
		return path
	}

	base := time.Date(2026, 2, 14, 12, 0, 0, 0, time.UTC)
	write("Intel_20260214_120000_charA.txt", base.Add(-2*time.Minute))
	newerA := write("Intel_20260214_120100_charA.txt", base.Add(-1*time.Minute))
	write("Intel_20260214_120050_charB.txt", base.Add(-90*time.Second))
	write("Other_20260214_120200_charX.txt", base.Add(-30*time.Second))
	write("invalid_name.txt", base)

	channels := []client.ChannelConfig{
		{ID: "intel", Name: "Intel"},
		{ID: "other", Name: "Other"},
	}
	logs, err := FindLogs(dir, channels)
	if err != nil {
		t.Fatalf("FindLogs() error = %v", err)
	}
	if len(logs) != 3 {
		t.Fatalf("FindLogs() len = %d, want 3", len(logs))
	}

	var foundNewerA bool
	for _, sel := range logs {
		if sel.Path == newerA {
			foundNewerA = true
		}
	}
	if !foundNewerA {
		t.Fatalf("FindLogs() missing latest file for Intel/charA")
	}
}
