package evelogs

import (
	"bufio"
	"os"
	"strings"
	"time"

	"sentinel2-uploader/internal/logging"
)

func (m *Monitor) sendExistingLines(tracked *trackedLog, cutoff time.Time) error {
	file, err := os.Open(tracked.selection.Path)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	var scanned int
	var submitted int
	for scanner.Scan() {
		scanned++
		line := NormalizeLogLine(scanner.Text())
		report, ok := ParseReportLine(line)
		if !ok {
			continue
		}
		if shouldIgnoreReport(report) {
			m.logger.Debug("ignoring historical report line",
				logging.Field("author", report.Author),
				logging.Field("message", logging.Truncate(report.Message)),
			)
			continue
		}
		if !cutoff.IsZero() && report.Time.Before(cutoff) {
			continue
		}
		if m.shouldSkipLocalDuplicate(tracked.selection.Channel.ID, line, time.Now()) {
			continue
		}
		if err := m.emitReport(tracked.selection, line, report.Time, time.Now()); err != nil {
			m.logger.Debugf("failed to emit existing report: %v", err)
			continue
		}
		submitted++
	}
	m.logger.Debugf("existing scan complete: scanned=%d submitted=%d file=%s", scanned, submitted, tracked.selection.Path)
	return scanner.Err()
}

func (m *Monitor) processLines(lines []string, selection LogSelection) {
	if len(lines) == 0 {
		return
	}
	m.logger.Debugf("read %d new lines from %s", len(lines), selection.Path)
	for _, line := range lines {
		line = NormalizeLogLine(line)
		m.logger.Debugf("line: %s", logging.Truncate(line))
		report, ok := ParseReportLine(line)
		if !ok {
			m.logger.Debugf("skipping non-report line")
			continue
		}
		if shouldIgnoreReport(report) {
			m.logger.Debug("skipping report line due to filtered author/message",
				logging.Field("author", report.Author),
				logging.Field("message", logging.Truncate(report.Message)),
			)
			continue
		}
		if m.shouldSkipLocalDuplicate(selection.Channel.ID, line, time.Now()) {
			m.logger.Debugf("skipping local duplicate line")
			continue
		}
		if err := m.emitReport(selection, line, report.Time, time.Now()); err != nil {
			m.logger.Warn("failed to emit report line", logging.Field("error", err))
			continue
		}
		m.logger.Debug("report accepted",
			logging.Field("channel", selection.Channel.Name),
			logging.Field("channel_id", selection.Channel.ID),
			logging.Field("report_time", report.Time.Unix()),
		)
	}
}

func shouldIgnoreReport(report ParsedReport) bool {
	if strings.EqualFold(strings.TrimSpace(report.Author), "EVE System") {
		return true
	}
	return strings.TrimSpace(report.Author) == "" || strings.TrimSpace(report.Message) == ""
}

func (m *Monitor) emitReport(selection LogSelection, line string, reportTime time.Time, now time.Time) error {
	if m.callbacks.OnReport == nil {
		m.markLocalDuplicate(selection.Channel.ID, line, now)
		return nil
	}
	meta, _ := parseLogFileMeta(selection.Path)
	err := m.callbacks.OnReport(ReportEvent{
		Line:        line,
		Channel:     selection.Channel,
		SourcePath:  selection.Path,
		CharacterID: meta.CharacterID,
		Timestamp:   reportTime,
	})
	if err != nil {
		if m.callbacks.OnError != nil {
			m.callbacks.OnError(err)
		}
		return err
	}
	m.logger.Debug("submitted parsed report",
		logging.Field("channel", selection.Channel.Name),
		logging.Field("channel_id", selection.Channel.ID),
		logging.Field("source_path", selection.Path),
	)
	m.markLocalDuplicate(selection.Channel.ID, line, now)
	return nil
}

func (m *Monitor) shouldSkipLocalDuplicate(channelID string, line string, now time.Time) bool {
	if channelID == "" || line == "" {
		return false
	}
	key := channelID + "\x00" + line
	if seenAt, ok := m.recent[key]; ok && now.Sub(seenAt) <= m.opts.DedupWindow {
		return true
	}
	return false
}

func (m *Monitor) markLocalDuplicate(channelID string, line string, now time.Time) {
	if channelID == "" || line == "" {
		return
	}
	key := channelID + "\x00" + line
	m.recent[key] = now
}

func (m *Monitor) pruneRecentDedup(now time.Time) {
	for key, seenAt := range m.recent {
		if now.Sub(seenAt) > m.opts.DedupWindow {
			delete(m.recent, key)
		}
	}
}
