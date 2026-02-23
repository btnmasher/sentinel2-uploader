package logging

import (
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type Logger struct {
	debugEnabled atomic.Bool
	terminalOut  atomic.Bool
	pretty       bool
	fileSink     *fileSink
	mu           sync.RWMutex
	nextID       int
	subscribers  map[int]func(Event)
}

type Event struct {
	Time    time.Time
	Level   slog.Level
	Message string
	Fields  map[string]any
}

func New(debug bool) *Logger {
	logger := &Logger{
		pretty:      shouldPrettyPrint(),
		subscribers: map[int]func(Event){},
	}
	logger.debugEnabled.Store(debug)
	logger.terminalOut.Store(true)
	return logger
}

func Field(key string, value any) slog.Attr {
	return slog.Any(key, value)
}

func (l *Logger) Debugf(format string, args ...any) {
	l.Debug(fmt.Sprintf(format, args...))
}

func (l *Logger) Debug(msg string, fields ...slog.Attr) {
	if l == nil {
		return
	}
	if !l.debugEnabled.Load() {
		// Persist debug telemetry to file even when debug output is hidden in UI/terminal.
		l.log(slog.LevelDebug, msg, fields, false)
		return
	}
	l.log(slog.LevelDebug, msg, fields, true)
}

func (l *Logger) SetDebugEnabled(enabled bool) {
	if l == nil {
		return
	}
	l.debugEnabled.Store(enabled)
}

func (l *Logger) SetTerminalOutputEnabled(enabled bool) {
	if l == nil {
		return
	}
	l.terminalOut.Store(enabled)
}

func (l *Logger) EnableFilePersistence(maxBytes int64) error {
	if l == nil {
		return nil
	}
	sink, err := newFileSink(maxBytes)
	if err != nil {
		return err
	}
	l.mu.Lock()
	old := l.fileSink
	l.fileSink = sink
	l.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	return nil
}

func (l *Logger) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	sink := l.fileSink
	l.fileSink = nil
	l.mu.Unlock()
	if sink == nil {
		return nil
	}
	return sink.Close()
}

func (l *Logger) Info(msg string, fields ...slog.Attr) {
	if l == nil {
		return
	}
	l.log(slog.LevelInfo, msg, fields, true)
}

func (l *Logger) Warn(msg string, fields ...slog.Attr) {
	if l == nil {
		return
	}
	l.log(slog.LevelWarn, msg, fields, true)
}

func (l *Logger) Error(msg string, fields ...slog.Attr) {
	if l == nil {
		return
	}
	l.log(slog.LevelError, msg, fields, true)
}

func (l *Logger) Subscribe(fn func(Event)) func() {
	if l == nil {
		panic("logging.Logger.Subscribe: logger must not be nil")
	}
	if fn == nil {
		panic("logging.Logger.Subscribe: callback must not be nil")
	}
	l.mu.Lock()
	id := l.nextID
	l.nextID++
	l.subscribers[id] = fn
	l.mu.Unlock()
	return func() {
		l.mu.Lock()
		delete(l.subscribers, id)
		l.mu.Unlock()
	}
}

func (l *Logger) log(level slog.Level, msg string, attrs []slog.Attr, publish bool) {
	event := Event{
		Time:    time.Now(),
		Level:   level,
		Message: msg,
		Fields:  attrsToMap(attrs),
	}
	l.mu.RLock()
	sink := l.fileSink
	l.mu.RUnlock()
	if sink != nil {
		_ = sink.WriteEvent(event)
	}
	if publish && l.terminalOut.Load() {
		l.emit(event)
	}
	if publish {
		l.publishEvent(event)
	}
}

func (l *Logger) emit(event Event) {
	if l.pretty {
		_, _ = os.Stderr.WriteString(FormatEventANSI(event))
		return
	}
	_, _ = os.Stderr.WriteString(FormatEventLine(event))
}

func (l *Logger) publishEvent(event Event) {
	if l == nil {
		return
	}
	l.mu.RLock()
	if len(l.subscribers) == 0 {
		l.mu.RUnlock()
		return
	}
	callbacks := make([]func(Event), 0, len(l.subscribers))
	for _, cb := range l.subscribers {
		callbacks = append(callbacks, cb)
	}
	l.mu.RUnlock()

	for _, cb := range callbacks {
		cb(event)
	}
}
