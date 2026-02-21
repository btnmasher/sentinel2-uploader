//go:build !headless

package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"

	"sentinel2-uploader/internal/config"
	"sentinel2-uploader/internal/logging"
)

const (
	updateCheckInterval = 30 * time.Minute
	updateCheckTimeout  = 10 * time.Second
	uploaderReleaseAPI  = "https://api.github.com/repos/btnmasher/sentinel2-uploader/releases/latest"
	uploaderReleasePage = "https://github.com/btnmasher/sentinel2-uploader/releases/latest"
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

func (c *controller) startUpdateCheckLoop() {
	current, ok := parseVersionInfo(c.version)
	if !ok {
		c.logger.Debug("skipping update checks: build version is not semver", logging.Field("version", c.version))
		return
	}

	c.startBackgroundLoop("update checker", func(ctx context.Context) {
		check := func() {
			latest, err := fetchLatestRelease(ctx)
			if err != nil {
				c.logger.Debug("update check failed", logging.Field("error", err.Error()))
				return
			}
			c.logger.Debug("update check fetched latest release", logging.Field("tag", latest.TagName), logging.Field("url", latest.HTMLURL))
			if latest.Draft {
				c.logger.Debug("update check skipped draft release", logging.Field("tag", latest.TagName))
				return
			}
			latestVersion, valid := parseVersionInfo(latest.TagName)
			if !valid {
				c.logger.Debug("update check skipped: latest tag is not semver", logging.Field("tag", latest.TagName))
				return
			}
			needsUpdate, reason := isUpdateAvailable(current, latestVersion)
			if !needsUpdate {
				c.logger.Debug(
					"update check: no update available",
					logging.Field("current_version", c.version),
					logging.Field("latest_tag", latest.TagName),
					logging.Field("reason", reason),
				)
				return
			}
			c.logger.Debug(
				"update check: update available",
				logging.Field("current_version", c.version),
				logging.Field("latest_tag", latest.TagName),
				logging.Field("reason", reason),
			)

			fyne.Do(func() {
				c.promptForUpdate(latest)
			})
		}

		check()
		ticker := time.NewTicker(updateCheckInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				check()
			}
		}
	})
}

func fetchLatestRelease(parent context.Context) (latestRelease, error) {
	ctx, cancel := context.WithTimeout(parent, updateCheckTimeout)
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

func (c *controller) promptForUpdate(latest latestRelease) {
	tag := strings.TrimSpace(latest.TagName)
	if tag == "" {
		return
	}
	if c.shouldSuppressUpdatePrompt(tag) {
		return
	}
	c.updatePrompted = tag

	dest := strings.TrimSpace(latest.HTMLURL)
	if dest == "" {
		dest = uploaderReleasePage
	}
	dialog.ShowConfirm(
		"Update Available",
		fmt.Sprintf("A newer uploader version is available (%s). Current version is %s.\n\nOpen the releases page?", tag, c.version),
		func(ok bool) {
			if !ok {
				c.rememberDismissedUpdateTag(tag)
				return
			}
			u, err := url.Parse(dest)
			if err != nil {
				c.logger.Warn("failed to parse release url", logging.Field("url", dest), logging.Field("error", err))
				return
			}
			if err := c.app.OpenURL(u); err != nil {
				c.logger.Warn("failed to open release url", logging.Field("url", dest), logging.Field("error", err))
			}
		},
		c.win,
	)
}

func (c *controller) shouldSuppressUpdatePrompt(tag string) bool {
	if c.updatePrompted == tag {
		return true
	}
	dismissed := strings.TrimSpace(c.dismissedTag)
	if dismissed == "" {
		return false
	}
	return shouldSuppressDismissedTag(tag, dismissed)
}

func shouldSuppressDismissedTag(latestTag string, dismissedTag string) bool {
	latestVersion, latestValid := parseVersionInfo(latestTag)
	dismissedVersion, dismissedValid := parseVersionInfo(dismissedTag)
	if latestValid && dismissedValid {
		newerThanDismissed, _ := isUpdateAvailable(dismissedVersion, latestVersion)
		return !newerThanDismissed
	}
	return strings.EqualFold(strings.TrimSpace(latestTag), strings.TrimSpace(dismissedTag))
}

func (c *controller) rememberDismissedUpdateTag(tag string) {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return
	}
	c.dismissedTag = tag
	c.updatePrompted = tag
	c.settings.LastDismissedUpdateTag = tag
	c.draft.LastDismissedUpdateTag = tag
	if err := config.SaveSettings(c.settings); err != nil {
		c.logger.Warn("failed to persist dismissed update tag", logging.Field("tag", tag), logging.Field("error", err))
	}
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
	// If semver is equal, use commit hash as a secondary signal only when both exist.
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
