package headless

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os/exec"
	goruntime "runtime"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"sentinel2-uploader/internal/logging"
)

const (
	headlessUpdateCheckInterval = 30 * time.Minute
	headlessUpdateCheckTimeout  = 10 * time.Second
	uploaderReleaseAPI          = "https://api.github.com/repos/btnmasher/sentinel2-uploader/releases/latest"
	uploaderReleasePage         = "https://github.com/btnmasher/sentinel2-uploader/releases/latest"
)

type latestRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
	Draft   bool   `json:"draft"`
}

type semver struct {
	major int
	minor int
	patch int
}

type versionInfo struct {
	semver semver
	hash   string
}

func (m *headlessModel) startUpdateCheckerCmd() tea.Cmd {
	current, ok := parseVersionInfo(m.buildVersion)
	if !ok {
		m.logger.Debug("skipping update checks: build version is not semver", logging.Field("version", m.buildVersion))
		return nil
	}
	return func() tea.Msg {
		go m.runUpdateCheckLoop(current)
		return nil
	}
}

func (m *headlessModel) runUpdateCheckLoop(current versionInfo) {
	check := func(ctx context.Context) {
		latest, err := fetchLatestRelease(ctx)
		if err != nil {
			m.logger.Debug("update check failed", logging.Field("error", err.Error()))
			return
		}
		m.logger.Debug("update check fetched latest release", logging.Field("tag", latest.TagName), logging.Field("url", latest.HTMLURL))
		if latest.Draft {
			m.logger.Debug("update check skipped draft release", logging.Field("tag", latest.TagName))
			return
		}
		latestVersion, valid := parseVersionInfo(latest.TagName)
		if !valid {
			m.logger.Debug("update check skipped: latest tag is not semver", logging.Field("tag", latest.TagName))
			return
		}
		needsUpdate, reason := isUpdateAvailable(current, latestVersion)
		if !needsUpdate {
			m.logger.Debug(
				"update check: no update available",
				logging.Field("current_version", m.buildVersion),
				logging.Field("latest_tag", latest.TagName),
				logging.Field("reason", reason),
			)
			return
		}
		m.logger.Debug(
			"update check: update available",
			logging.Field("current_version", m.buildVersion),
			logging.Field("latest_tag", latest.TagName),
			logging.Field("reason", reason),
		)

		msg := updateAvailableMsg{
			tag: strings.TrimSpace(latest.TagName),
			url: strings.TrimSpace(latest.HTMLURL),
		}
		select {
		case <-ctx.Done():
			return
		case m.updateCh <- msg:
		default:
			select {
			case <-m.updateCh:
			default:
			}
			select {
			case <-ctx.Done():
				return
			case m.updateCh <- msg:
			}
		}
	}

	ctx := m.rootCtx
	if ctx == nil {
		ctx = context.Background()
	}
	check(ctx)
	ticker := time.NewTicker(headlessUpdateCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			check(ctx)
		}
	}
}

func fetchLatestRelease(parent context.Context) (latestRelease, error) {
	ctx, cancel := context.WithTimeout(parent, headlessUpdateCheckTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, uploaderReleaseAPI, nil)
	if err != nil {
		return latestRelease{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "sentinel2-uploader")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return latestRelease{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return latestRelease{}, fmt.Errorf("github api status %d", resp.StatusCode)
	}

	var latest latestRelease
	if err := json.NewDecoder(resp.Body).Decode(&latest); err != nil {
		return latestRelease{}, err
	}
	return latest, nil
}

func parseVersionInfo(raw string) (versionInfo, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return versionInfo{}, false
	}
	s = strings.TrimPrefix(strings.TrimPrefix(s, "v"), "V")
	hash := ""
	if i := strings.IndexAny(s, "+-"); i >= 0 {
		hash = parseCommitHash(s[i+1:])
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return versionInfo{}, false
	}
	ints := make([]int, 3)
	for i := 0; i < 3; i++ {
		if i >= len(parts) {
			ints[i] = 0
			continue
		}
		n, err := strconv.Atoi(parts[i])
		if err != nil || n < 0 {
			return versionInfo{}, false
		}
		ints[i] = n
	}
	return versionInfo{
		semver: semver{major: ints[0], minor: ints[1], patch: ints[2]},
		hash:   hash,
	}, true
}

func isUpdateAvailable(current versionInfo, latest versionInfo) (bool, string) {
	if latest.semver.greaterThan(current.semver) {
		return true, "latest_semver_newer"
	}
	if latest.semver.equal(current.semver) && latest.hash != "" && current.hash != "" && latest.hash != current.hash {
		return true, "same_semver_different_hash"
	}
	return false, "latest_not_newer"
}

func (v semver) greaterThan(other semver) bool {
	if v.major != other.major {
		return v.major > other.major
	}
	if v.minor != other.minor {
		return v.minor > other.minor
	}
	return v.patch > other.patch
}

func (v semver) equal(other semver) bool {
	return v.major == other.major && v.minor == other.minor && v.patch == other.patch
}

func parseCommitHash(s string) string {
	meta := strings.TrimSpace(strings.ToLower(s))
	if meta == "" {
		return ""
	}
	parts := strings.FieldsFunc(meta, func(r rune) bool {
		return (r < '0' || r > '9') && (r < 'a' || r > 'z')
	})
	for _, part := range parts {
		candidate := strings.TrimPrefix(part, "g")
		if len(candidate) < 7 || len(candidate) > 40 {
			continue
		}
		valid := true
		for _, r := range candidate {
			if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
				valid = false
				break
			}
		}
		if valid {
			return candidate
		}
	}
	return ""
}

func (m *headlessModel) openLatestReleaseCmd() tea.Cmd {
	dest := strings.TrimSpace(m.ui.UpdateReleaseURL)
	if dest == "" {
		dest = uploaderReleasePage
	}
	return func() tea.Msg {
		return openReleaseResultMsg{
			url: dest,
			err: openExternalURL(dest),
		}
	}
}

func openExternalURL(rawURL string) error {
	var cmd *exec.Cmd
	switch goruntime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	return cmd.Start()
}
