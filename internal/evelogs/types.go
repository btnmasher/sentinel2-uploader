package evelogs

import (
	"time"

	"sentinel2-uploader/internal/client"
	"sentinel2-uploader/internal/logging"
)

type Monitor struct {
	opts      MonitorOptions
	logger    *logging.Logger
	callbacks MonitorCallbacks

	channels []client.ChannelConfig
	watchDir string
	prepared bool

	tracked map[string]*trackedLog
	recent  map[string]time.Time
	health  map[string]channelHealthState
}

type MonitorOptions struct {
	LogDir          string
	LogFile         string
	Channels        []client.ChannelConfig
	RescanPeriod    time.Duration
	DedupWindow     time.Duration
	InitialLookback time.Duration
}

type MonitorCallbacks struct {
	OnReport    func(ReportEvent) error
	OnError     func(error)
	OnTracked   func(LogSelection)
	OnUntracked func(string)
}

type ReportEvent struct {
	Line        string
	Channel     client.ChannelConfig
	SourcePath  string
	CharacterID string
	Timestamp   time.Time
}

type LogSelection struct {
	Path    string
	Channel client.ChannelConfig
}

type Tailer struct {
	Path         string
	Offset       int64
	Encoding     string
	PendingBytes []byte
}

type logFileMeta struct {
	ChannelName string
	CharacterID string
	Timestamp   time.Time
}

type logMatch struct {
	Selection LogSelection
	Meta      logFileMeta
	ModTime   time.Time
}

type trackedLog struct {
	selection LogSelection
	tailer    *Tailer
}

type channelHealthState int

const (
	channelHealthOK channelHealthState = iota
	channelHealthStale
	channelHealthMissing
)
