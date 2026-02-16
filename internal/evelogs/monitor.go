package evelogs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"sentinel2-uploader/internal/client"
	"sentinel2-uploader/internal/logging"
)

const (
	defaultRescanPeriod    = 5 * time.Second
	defaultDedupWindow     = 15 * time.Second
	defaultInitialLookback = 1 * time.Minute
	defaultStaleAfter      = 10 * time.Minute
)

func NewMonitor(opts MonitorOptions, logger *logging.Logger, callbacks MonitorCallbacks) *Monitor {
	if logger == nil {
		panic("evelogs.NewMonitor: logger must not be nil")
	}
	if opts.RescanPeriod <= 0 {
		opts.RescanPeriod = defaultRescanPeriod
	}
	if opts.DedupWindow <= 0 {
		opts.DedupWindow = defaultDedupWindow
	}
	if opts.InitialLookback <= 0 {
		opts.InitialLookback = defaultInitialLookback
	}
	return &Monitor{
		opts:      opts,
		logger:    logger,
		callbacks: callbacks,
		channels:  append([]client.ChannelConfig(nil), opts.Channels...),
		tracked:   map[string]*trackedLog{},
		recent:    map[string]time.Time{},
		health:    map[string]channelHealthState{},
	}
}

func (m *Monitor) RunContext(ctx context.Context, configUpdates <-chan []client.ChannelConfig) error {
	m.logger.Debug("starting log monitor",
		logging.Field("configured_channels", len(m.channels)),
		logging.Field("log_dir", m.opts.LogDir),
		logging.Field("log_file", m.opts.LogFile),
	)
	if err := m.Prepare(); err != nil {
		return err
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return fmt.Errorf("failed to initialize fsnotify watcher: %w", err)
	}
	defer watcher.Close()

	if err := watcher.Add(m.watchDir); err != nil {
		return fmt.Errorf("failed to watch log directory %s: %w", m.watchDir, err)
	}
	m.logger.Debugf("watching directory: %s", m.watchDir)

	rescanTicker := time.NewTicker(m.opts.RescanPeriod)
	defer rescanTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.logger.Debug("stopping log monitor: context canceled")
			return nil
		case updated, ok := <-configUpdates:
			if !ok {
				m.logger.Debug("channel update stream closed")
				configUpdates = nil
				continue
			}
			m.handleChannelUpdate(updated)
		case event := <-watcher.Events:
			m.handleWatcherEvent(event)
		case err := <-watcher.Errors:
			m.handleWatcherError(err)
		case <-rescanTicker.C:
			m.handlePollTick()
		}
	}
}

func (m *Monitor) Prepare() error {
	if m.prepared {
		return nil
	}
	if err := m.initialize(); err != nil {
		return err
	}
	m.prepared = true
	return nil
}

func (m *Monitor) initialize() error {
	m.watchDir = m.opts.LogDir
	if m.watchDir == "" {
		m.watchDir = filepath.Dir(m.opts.LogFile)
	}
	if m.watchDir == "" {
		return fmt.Errorf("missing log directory")
	}

	if err := m.syncTrackedLogs(); err != nil {
		return err
	}
	m.reportChannelHealthTransitions(time.Now())
	if len(m.tracked) == 0 {
		m.logger.Warn("no matching log files found for configured channels")
	}

	cutoff := time.Now().Add(-1 * m.opts.InitialLookback)
	for _, tracked := range m.tracked {
		if err := m.sendExistingLines(tracked, cutoff); err != nil {
			m.logger.Warn("failed to read recent logs", logging.Field("path", tracked.selection.Path), logging.Field("error", err))
		}
		if err := tracked.tailer.Prime(); err != nil {
			m.logger.Warn("failed to prime log tailer", logging.Field("path", tracked.selection.Path), logging.Field("error", err))
		}
	}

	m.logger.Info("watching logs", logging.Field("directory", m.watchDir), logging.Field("files", len(m.tracked)), logging.Field("channels", len(m.channels)))
	return nil
}

func (m *Monitor) handleWatcherEvent(event fsnotify.Event) {
	m.logger.Debugf("fsnotify event: op=%s path=%s", event.Op.String(), event.Name)

	if event.Op&(fsnotify.Create|fsnotify.Rename|fsnotify.Write) != 0 {
		m.maybeTrackEventPath(event.Name)
	}
	if event.Op&(fsnotify.Remove|fsnotify.Rename) != 0 {
		m.maybeUntrackPath(event.Name)
	}

	if tracked, ok := m.tracked[filepath.Clean(event.Name)]; ok && event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) != 0 {
		m.readAndProcessTrackedLog(tracked)
	}
}

func (m *Monitor) handleWatcherError(err error) {
	if err == nil {
		return
	}
	m.logger.Warn("watcher error", logging.Field("error", err))
	if m.callbacks.OnError != nil {
		m.callbacks.OnError(err)
	}
}

func (m *Monitor) handlePollTick() {
	m.logger.Debug("poll tick: syncing tracked logs", logging.Field("tracked", len(m.tracked)))
	if err := m.syncTrackedLogs(); err != nil {
		m.logger.Debugf("log sync failed: %v", err)
	}
	for _, tracked := range m.tracked {
		m.readAndProcessTrackedLog(tracked)
	}
	m.reportChannelHealthTransitions(time.Now())
	m.pruneRecentDedup(time.Now())
}

func (m *Monitor) handleChannelUpdate(updated []client.ChannelConfig) {
	m.logger.Info("received channel update", logging.Field("count", len(updated)))
	m.channels = updated
	if err := m.syncTrackedLogs(); err != nil {
		m.logger.Warn("failed to sync logs after channel update", logging.Field("error", err))
	}
	m.reportChannelHealthTransitions(time.Now())
}

func (m *Monitor) syncTrackedLogs() error {
	desired, err := m.desiredSelections()
	if err != nil {
		return err
	}
	desiredByPath := make(map[string]LogSelection, len(desired))
	for _, sel := range desired {
		desiredByPath[filepath.Clean(sel.Path)] = sel
	}
	m.logger.Debug("computed desired log selections", logging.Field("count", len(desiredByPath)))

	for path, sel := range desiredByPath {
		if tracked, ok := m.tracked[path]; ok {
			tracked.selection.Channel = sel.Channel
			continue
		}
		m.addTrackedLog(sel)
	}

	for path := range m.tracked {
		if _, ok := desiredByPath[path]; ok {
			continue
		}
		m.logger.Info("stopped tracking log file", logging.Field("path", path))
		if m.callbacks.OnUntracked != nil {
			m.callbacks.OnUntracked(path)
		}
		delete(m.tracked, path)
	}

	return nil
}

func (m *Monitor) desiredSelections() ([]LogSelection, error) {
	if m.opts.LogFile != "" {
		channel, ok := ResolveChannelForPath(m.opts.LogFile, m.channels)
		if !ok || channel.ID == "" {
			return nil, fmt.Errorf("failed to map log file to configured channel: %s", m.opts.LogFile)
		}
		return []LogSelection{{Path: m.opts.LogFile, Channel: channel}}, nil
	}

	if m.opts.LogDir == "" {
		return nil, fmt.Errorf("missing log directory")
	}

	selections, err := FindLogs(m.opts.LogDir, m.channels)
	if err != nil {
		return nil, err
	}
	return selections, nil
}

func (m *Monitor) addTrackedLog(sel LogSelection) {
	path := filepath.Clean(sel.Path)
	tailer := &Tailer{Path: sel.Path}
	m.tracked[path] = &trackedLog{selection: sel, tailer: tailer}
	m.logger.Info(
		"tracking log file",
		logging.Field("path", sel.Path),
		logging.Field("channel", sel.Channel.Name),
		logging.Field("channel_id", sel.Channel.ID),
	)
	if m.callbacks.OnTracked != nil {
		m.callbacks.OnTracked(sel)
	}
}

func (m *Monitor) reportChannelHealthTransitions(now time.Time) {
	latestByID := map[string]time.Time{}
	for _, tracked := range m.tracked {
		id := strings.TrimSpace(tracked.selection.Channel.ID)
		if id == "" {
			continue
		}
		info, err := os.Stat(tracked.selection.Path)
		if err != nil {
			continue
		}
		mod := info.ModTime()
		if prev, ok := latestByID[id]; !ok || mod.After(prev) {
			latestByID[id] = mod
		}
	}

	for _, ch := range m.channels {
		id := strings.TrimSpace(ch.ID)
		if id == "" {
			continue
		}
		next := channelHealthMissing
		if mod, ok := latestByID[id]; ok {
			if now.Sub(mod) > defaultStaleAfter {
				next = channelHealthStale
			} else {
				next = channelHealthOK
			}
		}
		prev, had := m.health[id]
		if had && prev == next {
			continue
		}
		m.health[id] = next
		switch next {
		case channelHealthMissing:
			m.logger.Warn("channel log not found",
				logging.Field("channel", ch.Name),
				logging.Field("channel_id", ch.ID))
		case channelHealthStale:
			m.logger.Info("channel log is stale",
				logging.Field("channel", ch.Name),
				logging.Field("channel_id", ch.ID))
		}
	}
}

func (m *Monitor) maybeTrackEventPath(path string) {
	if path == "" {
		return
	}
	clean := filepath.Clean(path)
	channel, ok := ResolveChannelForPath(clean, m.channels)
	if !ok || channel.ID == "" {
		return
	}
	meta, ok := parseLogFileMeta(clean)
	if !ok {
		return
	}
	if info, err := os.Stat(clean); err != nil || info.IsDir() {
		return
	}
	if existingPath, found := m.findTrackedForChannelCharacter(channel.ID, meta.CharacterID); found {
		if existingPath == clean {
			return
		}
		if !isPathNewerByMeta(clean, existingPath) {
			return
		}
		m.logger.Info("switching tracked log file",
			logging.Field("from", existingPath),
			logging.Field("to", clean),
			logging.Field("channel_id", channel.ID),
			logging.Field("character_id", meta.CharacterID),
		)
		if m.callbacks.OnUntracked != nil {
			m.callbacks.OnUntracked(existingPath)
		}
		delete(m.tracked, existingPath)
	}
	m.addTrackedLog(LogSelection{Path: clean, Channel: channel})
	if tracked, exists := m.tracked[clean]; exists {
		if err := tracked.tailer.Prime(); err != nil {
			m.logger.Debugf("failed to prime new log file %s: %v", clean, err)
		}
	}
}

func (m *Monitor) findTrackedForChannelCharacter(channelID string, characterID string) (string, bool) {
	channelID = strings.TrimSpace(channelID)
	characterID = strings.TrimSpace(characterID)
	if channelID == "" || characterID == "" {
		return "", false
	}
	for path, tracked := range m.tracked {
		if strings.TrimSpace(tracked.selection.Channel.ID) != channelID {
			continue
		}
		meta, ok := parseLogFileMeta(path)
		if !ok {
			continue
		}
		if strings.TrimSpace(meta.CharacterID) == characterID {
			return path, true
		}
	}
	return "", false
}

func isPathNewerByMeta(candidatePath string, existingPath string) bool {
	candidateMeta, okCandidate := parseLogFileMeta(candidatePath)
	existingMeta, okExisting := parseLogFileMeta(existingPath)
	if okCandidate && okExisting {
		if candidateMeta.Timestamp.After(existingMeta.Timestamp) {
			return true
		}
		if existingMeta.Timestamp.After(candidateMeta.Timestamp) {
			return false
		}
	}

	candidateInfo, candidateErr := os.Stat(candidatePath)
	existingInfo, existingErr := os.Stat(existingPath)
	if candidateErr == nil && existingErr == nil {
		if candidateInfo.ModTime().After(existingInfo.ModTime()) {
			return true
		}
		if existingInfo.ModTime().After(candidateInfo.ModTime()) {
			return false
		}
	}

	return candidatePath > existingPath
}

func (m *Monitor) maybeUntrackPath(path string) {
	clean := filepath.Clean(path)
	if _, ok := m.tracked[clean]; !ok {
		return
	}
	if _, err := os.Stat(clean); err == nil {
		return
	}
	m.logger.Info("stopped tracking removed log file", logging.Field("path", clean))
	if m.callbacks.OnUntracked != nil {
		m.callbacks.OnUntracked(clean)
	}
	delete(m.tracked, clean)
}

func (m *Monitor) readAndProcessTrackedLog(tracked *trackedLog) {
	lines, err := tracked.tailer.ReadNewLines()
	if err != nil {
		m.logger.Debugf("failed to read new lines from %s: %v", tracked.tailer.Path, err)
		return
	}
	m.processLines(lines, tracked.selection)
}
