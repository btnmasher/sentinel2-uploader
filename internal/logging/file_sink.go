package logging

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	defaultLogFileMaxBytes = 5 * 1024 * 1024
)

type fileSink struct {
	mu         sync.Mutex
	dir        string
	sessionTag string
	maxBytes   int64
	part       int
	file       *os.File
	size       int64
	closed     bool
}

type jsonLogLine struct {
	Time    string         `json:"time"`
	Level   string         `json:"level"`
	Message string         `json:"message"`
	Fields  map[string]any `json:"fields,omitempty"`
}

func DefaultLogDirPath() (string, error) {
	root, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "sentinel2", "uploader", "logs"), nil
}

func newFileSink(maxBytes int64) (*fileSink, error) {
	if maxBytes <= 0 {
		maxBytes = defaultLogFileMaxBytes
	}
	dir, err := DefaultLogDirPath()
	if err != nil {
		return nil, err
	}
	sink := &fileSink{
		dir:        dir,
		sessionTag: time.Now().UTC().Format("20060102-150405"),
		maxBytes:   maxBytes,
	}
	if err := sink.rotateLocked(); err != nil {
		return nil, err
	}
	return sink, nil
}

func (s *fileSink) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.file == nil {
		return nil
	}
	err := s.file.Close()
	s.file = nil
	s.size = 0
	return err
}

func (s *fileSink) WriteEvent(event Event) error {
	if s == nil {
		return nil
	}
	entry := jsonLogLine{
		Time:    event.Time.UTC().Format(time.RFC3339Nano),
		Level:   strings.ToUpper(event.Level.String()),
		Message: event.Message,
	}
	if len(event.Fields) > 0 {
		entry.Fields = normalizeLogFields(event.Fields)
	}
	payload, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	line := append(payload, '\n')

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return os.ErrClosed
	}

	if s.file == nil {
		if err := s.rotateLocked(); err != nil {
			return err
		}
	}
	if s.maxBytes > 0 && s.size > 0 && s.size+int64(len(line)) > s.maxBytes {
		if err := s.rotateLocked(); err != nil {
			return err
		}
	}
	n, writeErr := s.file.Write(line)
	s.size += int64(n)
	return writeErr
}

func (s *fileSink) rotateLocked() error {
	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return err
	}
	if s.file != nil {
		_ = s.file.Close()
		s.file = nil
		s.size = 0
	}
	s.part++
	filename := fmt.Sprintf("uploader-%s-%03d.jsonl", s.sessionTag, s.part)
	path := filepath.Join(s.dir, filename)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	info, statErr := f.Stat()
	if statErr != nil {
		_ = f.Close()
		return statErr
	}
	s.file = f
	s.size = info.Size()
	return nil
}

func normalizeLogFields(fields map[string]any) map[string]any {
	out := make(map[string]any, len(fields))
	for key, value := range fields {
		out[key] = normalizeLogFieldValue(value)
	}
	return out
}

func normalizeLogFieldValue(value any) any {
	if value == nil {
		return nil
	}
	if errValue, ok := value.(error); ok {
		return errValue.Error()
	}
	if text, ok := value.(interface{ String() string }); ok {
		if _, isLevel := value.(slog.Level); !isLevel {
			return text.String()
		}
	}
	return value
}
