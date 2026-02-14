package evelogs

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"sentinel2-uploader/internal/client"
)

func ResolveChannelForPath(path string, channels []client.ChannelConfig) (client.ChannelConfig, bool) {
	meta, ok := parseLogFileMeta(path)
	if !ok {
		return client.ChannelConfig{}, false
	}
	index := channelIndex(channels)
	channel, found := index[normalizeChannelKey(meta.ChannelName)]
	if !found || strings.TrimSpace(channel.ID) == "" {
		return client.ChannelConfig{}, false
	}
	return channel, true
}

func FindLatestLog(dir string, channels []client.ChannelConfig) (LogSelection, bool) {
	matches, err := findLogMatches(dir, channels)
	if err != nil {
		return LogSelection{}, false
	}
	if len(matches) == 0 {
		return LogSelection{}, false
	}
	latest := matches[0]
	for _, m := range matches[1:] {
		if m.Meta.Timestamp.After(latest.Meta.Timestamp) || (m.Meta.Timestamp.Equal(latest.Meta.Timestamp) && m.ModTime.After(latest.ModTime)) {
			latest = m
		}
	}
	return latest.Selection, true
}

func FindLogs(dir string, channels []client.ChannelConfig) ([]LogSelection, error) {
	matches, err := findLogMatches(dir, channels)
	if err != nil {
		return nil, err
	}

	// Keep only the newest file per (channel, character) tuple.
	latestByKey := make(map[string]logMatch)
	for _, m := range matches {
		key := m.Selection.Channel.ID + "\x00" + m.Meta.CharacterID
		current, ok := latestByKey[key]
		if !ok || m.Meta.Timestamp.After(current.Meta.Timestamp) || (m.Meta.Timestamp.Equal(current.Meta.Timestamp) && m.ModTime.After(current.ModTime)) {
			latestByKey[key] = m
		}
	}

	latest := make([]logMatch, 0, len(latestByKey))
	for _, m := range latestByKey {
		latest = append(latest, m)
	}
	sort.Slice(latest, func(i, j int) bool {
		if latest[i].Meta.Timestamp.Equal(latest[j].Meta.Timestamp) {
			return latest[i].Selection.Path < latest[j].Selection.Path
		}
		return latest[i].Meta.Timestamp.Before(latest[j].Meta.Timestamp)
	})

	out := make([]LogSelection, 0, len(latest))
	for _, m := range latest {
		out = append(out, m.Selection)
	}
	return out, nil
}

func findLogMatches(dir string, channels []client.ChannelConfig) ([]logMatch, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	index := channelIndex(channels)

	matches := make([]logMatch, 0)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		meta, ok := parseLogFileMeta(entry.Name())
		if !ok {
			continue
		}
		channel, found := index[normalizeChannelKey(meta.ChannelName)]
		if !found || strings.TrimSpace(channel.ID) == "" {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}
		matches = append(matches, logMatch{
			Selection: LogSelection{Path: filepath.Join(dir, entry.Name()), Channel: channel},
			Meta:      meta,
			ModTime:   info.ModTime(),
		})
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Meta.Timestamp.Equal(matches[j].Meta.Timestamp) {
			return matches[i].Selection.Path < matches[j].Selection.Path
		}
		return matches[i].Meta.Timestamp.Before(matches[j].Meta.Timestamp)
	})
	return matches, nil
}

func channelIndex(channels []client.ChannelConfig) map[string]client.ChannelConfig {
	index := make(map[string]client.ChannelConfig, len(channels))
	for _, channel := range channels {
		key := normalizeChannelKey(channel.Name)
		if key == "" {
			continue
		}
		if _, exists := index[key]; exists {
			continue
		}
		index[key] = channel
	}
	return index
}

func normalizeChannelKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func parseLogFileMeta(path string) (logFileMeta, bool) {
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	if !strings.EqualFold(ext, ".txt") {
		return logFileMeta{}, false
	}

	stem := strings.TrimSuffix(base, ext)
	parts := strings.Split(stem, "_")
	if len(parts) < 4 {
		return logFileMeta{}, false
	}

	channelName := strings.TrimSpace(strings.Join(parts[:len(parts)-3], "_"))
	datePart := strings.TrimSpace(parts[len(parts)-3])
	timePart := strings.TrimSpace(parts[len(parts)-2])
	characterID := strings.TrimSpace(parts[len(parts)-1])
	if channelName == "" || characterID == "" || len(datePart) != 8 || len(timePart) != 6 {
		return logFileMeta{}, false
	}

	ts, err := time.ParseInLocation("20060102 150405", datePart+" "+timePart, time.UTC)
	if err != nil {
		return logFileMeta{}, false
	}

	return logFileMeta{
		ChannelName: channelName,
		CharacterID: characterID,
		Timestamp:   ts,
	}, true
}
