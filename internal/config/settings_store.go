package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type UploaderSettings struct {
	BaseURL                string `json:"base_url"`
	Token                  string `json:"token"`
	LogDir                 string `json:"log_dir"`
	AutoConnect            bool   `json:"auto_connect"`
	Debug                  bool   `json:"debug"`
	MinimizeToTray         bool   `json:"minimize_to_tray"`
	StartMinimized         bool   `json:"start_minimized"`
	LastDismissedUpdateTag string `json:"last_dismissed_update_tag,omitempty"`
}

func SettingsPath() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "sentinel2", "uploader-settings.json"), nil
}

func LoadSettings() (UploaderSettings, error) {
	path, err := SettingsPath()
	if err != nil {
		return UploaderSettings{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return UploaderSettings{}, err
	}
	var settings UploaderSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return UploaderSettings{}, err
	}
	return settings, nil
}

func SaveSettings(settings UploaderSettings) error {
	path, err := SettingsPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, payload, 0o600)
}

func MergeOptionsWithSettings(cli Options, saved UploaderSettings) Options {
	if strings.TrimSpace(cli.BaseURL) == "" {
		cli.BaseURL = saved.BaseURL
	}
	if strings.TrimSpace(cli.Token) == "" {
		cli.Token = saved.Token
	}
	if strings.TrimSpace(cli.LogDir) == "" {
		cli.LogDir = saved.LogDir
	}
	if !cli.AutoConnect {
		cli.AutoConnect = saved.AutoConnect
	}
	if !cli.Debug {
		cli.Debug = saved.Debug
	}
	cli.LogFile = ""
	return cli
}

func SettingsFromOptions(opts Options) UploaderSettings {
	return UploaderSettings{
		BaseURL:     strings.TrimSpace(opts.BaseURL),
		Token:       strings.TrimSpace(opts.Token),
		LogDir:      strings.TrimSpace(opts.LogDir),
		AutoConnect: opts.AutoConnect,
		Debug:       opts.Debug,
	}
}
