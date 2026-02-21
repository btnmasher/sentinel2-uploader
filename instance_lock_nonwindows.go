//go:build !windows

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

type instanceLock struct {
	lock *flock.Flock
}

func (l *instanceLock) Release() error {
	if l == nil || l.lock == nil {
		return nil
	}
	locked := l.lock.Locked()
	if !locked {
		return nil
	}
	if err := l.lock.Unlock(); err != nil {
		return fmt.Errorf("unlock instance lock: %w", err)
	}
	return nil
}

func acquireInstanceLock() (*instanceLock, bool, error) {
	lockPath, err := instanceLockPath()
	if err != nil {
		return nil, false, err
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, false, fmt.Errorf("create lock directory: %w", err)
	}
	f := flock.New(lockPath)
	locked, err := f.TryLock()
	if err != nil {
		return nil, false, fmt.Errorf("acquire instance lock: %w", err)
	}
	if !locked {
		return nil, true, nil
	}
	return &instanceLock{lock: f}, false, nil
}

func instanceLockPath() (string, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config directory: %w", err)
	}
	return filepath.Join(root, "sentinel2", "uploader.lock"), nil
}
