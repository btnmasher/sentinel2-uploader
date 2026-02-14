package health

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"sentinel2-uploader/internal/client"
	"sentinel2-uploader/internal/evelogs"
)

const RefreshRate = 30 * time.Second

const (
	warnAfter  = 10 * time.Minute
	staleAfter = time.Hour
)

type Kind int

const (
	Missing Kind = iota
	Active
	Warn
	Stale
)

type Row struct {
	Name   string
	Kind   Kind
	Reason string
}

func Compute(logDir string, channels []client.ChannelConfig, now time.Time) ([]Row, string) {
	rows := make([]Row, 0, len(channels))
	logDir = strings.TrimSpace(logDir)
	if logDir == "" {
		return rows, "Log directory is not configured."
	}

	info, statErr := os.Stat(logDir)
	if statErr != nil {
		return rows, "Log directory is not accessible: " + statErr.Error()
	}
	if !info.IsDir() {
		return rows, "Log path is not a directory."
	}

	latestByChannel := map[string]time.Time{}
	latestFileByChannel := map[string]string{}
	logs, findErr := evelogs.FindLogs(logDir, channels)
	if findErr != nil {
		return rows, "Failed to scan logs: " + findErr.Error()
	}
	for _, selection := range logs {
		stat, err := os.Stat(selection.Path)
		if err != nil {
			continue
		}
		id := strings.TrimSpace(selection.Channel.ID)
		if id == "" {
			continue
		}
		current, ok := latestByChannel[id]
		if !ok || stat.ModTime().After(current) {
			latestByChannel[id] = stat.ModTime()
			latestFileByChannel[id] = filepath.Base(selection.Path)
		}
	}

	for _, channel := range channels {
		id := strings.TrimSpace(channel.ID)
		name := strings.TrimSpace(channel.Name)
		row := Row{
			Name:   name,
			Kind:   Missing,
			Reason: "No matching log file found.",
		}
		if last, ok := latestByChannel[id]; ok {
			age := now.Sub(last)
			fileName := latestFileByChannel[id]
			switch {
			case age <= warnAfter:
				row.Kind = Active
				row.Reason = fmt.Sprintf("%s updated %s ago.", fileName, age.Round(time.Second))
			case age <= staleAfter:
				row.Kind = Warn
				row.Reason = fmt.Sprintf("%s has no updates for %s.", fileName, age.Round(time.Second))
			default:
				row.Kind = Stale
				row.Reason = fmt.Sprintf("%s has no updates for %s.", fileName, age.Round(time.Second))
			}
		}
		rows = append(rows, row)
	}

	return rows, ""
}
