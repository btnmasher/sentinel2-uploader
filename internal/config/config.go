package config

import (
	"errors"
	"net/url"
	"strings"

	flags "github.com/jessevdk/go-flags"
	"github.com/joho/godotenv"
)

type Options struct {
	BaseURL     string `long:"base-url" env:"SENTINEL_BASE_URL" description:"Sentinel base URL (e.g. https://intel.example.com)"`
	Token       string `long:"token" env:"SENTINEL_TOKEN" description:"Uploader token"`
	Headless    bool   `long:"headless" env:"SENTINEL_HEADLESS" description:"Run uploader in headless mode (GUI builds only)"`
	AutoConnect bool   `long:"auto-connect" env:"AUTO_CONNECT" description:"Auto-connect on startup when base URL and token are configured"`
	ImGay       bool   `long:"imgay" description:"Enable rainbow border animation in headless TUI"`
	LogFile     string `long:"log-file" env:"SENTINEL_LOG_FILE" description:"EVE chat log file to watch"`
	LogDir      string `long:"log-dir" env:"SENTINEL_LOG_DIR" description:"Directory containing EVE chat logs"`
	Debug       bool   `long:"debug" env:"SENTINEL_DEBUG" description:"Enable verbose debug output"`
}

type APIEndpoints struct {
	BaseURL          string
	ConfigURL        string
	SubmitURL        string
	RealtimeTokenURL string
	RealtimeURL      string
}

const (
	realtimeTokenPath = "/uploader/realtime/token"
	realtimeEventsURL = "/realtime"
)

func ParseOptions(defaultLogDirFn func() string) (Options, error) {
	_ = godotenv.Load()
	opts := Options{}
	if _, err := flags.Parse(&opts); err != nil {
		return Options{}, err
	}
	if opts.LogDir == "" && opts.LogFile == "" && defaultLogDirFn != nil {
		opts.LogDir = defaultLogDirFn()
	}
	return opts, nil
}

func ValidateRequired(opts Options) error {
	if strings.TrimSpace(opts.BaseURL) == "" {
		return errors.New("base URL is required")
	}
	if strings.TrimSpace(opts.Token) == "" {
		return errors.New("uploader token is required")
	}
	if strings.TrimSpace(opts.LogFile) == "" && strings.TrimSpace(opts.LogDir) == "" {
		return errors.New("set either log file or log directory")
	}
	return nil
}

func BuildEndpoints(rawBaseURL string) (APIEndpoints, error) {
	apiBaseURL, err := buildAPIBaseURL(rawBaseURL)
	if err != nil {
		return APIEndpoints{}, err
	}
	return APIEndpoints{
		BaseURL:          apiBaseURL,
		ConfigURL:        apiBaseURL + "/uploader/config",
		SubmitURL:        apiBaseURL + "/uploader/submit",
		RealtimeTokenURL: apiBaseURL + realtimeTokenPath,
		RealtimeURL:      apiBaseURL + realtimeEventsURL,
	}, nil
}

func buildAPIBaseURL(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	parsed, err := url.Parse(value)
	if err != nil {
		return "", err
	}
	if parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("expected absolute URL like https://example.com")
	}
	if !strings.EqualFold(parsed.Scheme, "http") && !strings.EqualFold(parsed.Scheme, "https") {
		return "", errors.New("base URL scheme must be http or https")
	}

	// Normalize any pasted endpoint/path to canonical API base.
	parsed.Path = "/api"
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""

	return strings.TrimRight(parsed.String(), "/"), nil
}
