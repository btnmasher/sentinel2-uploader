//go:build windows

package main

import (
	"fmt"

	"golang.org/x/sys/windows"
)

const instanceMutexName = `Local\Sentinel2UploaderInstance`

type instanceLock struct {
	handle windows.Handle
}

func (l *instanceLock) Release() error {
	if l == nil || l.handle == 0 {
		return nil
	}
	err := windows.CloseHandle(l.handle)
	l.handle = 0
	if err != nil {
		return fmt.Errorf("close instance mutex handle: %w", err)
	}
	return nil
}

func acquireInstanceLock() (*instanceLock, bool, error) {
	name, err := windows.UTF16PtrFromString(instanceMutexName)
	if err != nil {
		return nil, false, fmt.Errorf("encode mutex name: %w", err)
	}
	handle, err := windows.CreateMutex(nil, false, name)
	if err != nil {
		return nil, false, fmt.Errorf("create instance mutex: %w", err)
	}
	if windows.GetLastError() == windows.ERROR_ALREADY_EXISTS {
		_ = windows.CloseHandle(handle)
		return nil, true, nil
	}
	return &instanceLock{handle: handle}, false, nil
}
