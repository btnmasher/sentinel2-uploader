package config

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestSettingsSaveLoadAndPath(t *testing.T) {
	root := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("AppData", root)
	} else {
		t.Setenv("XDG_CONFIG_HOME", root)
	}

	path, err := SettingsPath()
	if err != nil {
		t.Fatalf("SettingsPath() error = %v", err)
	}
	wantPath := filepath.Join(root, "sentinel2", "uploader-settings.json")
	if path != wantPath {
		t.Fatalf("SettingsPath() = %q, want %q", path, wantPath)
	}

	in := UploaderSettings{
		BaseURL:        "https://intel.example.com",
		Token:          "tok",
		LogDir:         "/tmp/chatlogs",
		AutoConnect:    true,
		Debug:          true,
		MinimizeToTray: true,
		StartMinimized: true,
	}
	if err := SaveSettings(in); err != nil {
		t.Fatalf("SaveSettings() error = %v", err)
	}
	out, err := LoadSettings()
	if err != nil {
		t.Fatalf("LoadSettings() error = %v", err)
	}
	if out.BaseURL != in.BaseURL || out.Token != in.Token || out.LogDir != in.LogDir || !out.AutoConnect || !out.Debug {
		t.Fatalf("loaded settings = %#v", out)
	}
}

func TestMergeOptionsWithSettings_PrefersCLIAndClearsLogFile(t *testing.T) {
	merged := MergeOptionsWithSettings(
		Options{
			BaseURL:     "https://cli.example.com",
			Token:       "",
			LogFile:     "/tmp/specific.log",
			LogDir:      "",
			AutoConnect: false,
			Debug:       false,
		},
		UploaderSettings{
			BaseURL:     "https://saved.example.com",
			Token:       "saved-token",
			LogDir:      "/tmp/saved-dir",
			AutoConnect: true,
			Debug:       true,
		},
	)

	if merged.BaseURL != "https://cli.example.com" {
		t.Fatalf("BaseURL = %q", merged.BaseURL)
	}
	if merged.Token != "saved-token" {
		t.Fatalf("Token = %q", merged.Token)
	}
	if merged.LogDir != "/tmp/saved-dir" {
		t.Fatalf("LogDir = %q", merged.LogDir)
	}
	if !merged.AutoConnect || !merged.Debug {
		t.Fatalf("bool flags should merge from saved when CLI false: %#v", merged)
	}
	if merged.LogFile != "" {
		t.Fatalf("LogFile should be cleared, got %q", merged.LogFile)
	}
}
