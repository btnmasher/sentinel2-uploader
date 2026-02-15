//go:build windows

package main

import (
	"syscall"
)

func hideAndDetachConsoleForGUI() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	user32 := syscall.NewLazyDLL("user32.dll")
	getConsoleWindow := kernel32.NewProc("GetConsoleWindow")
	freeConsole := kernel32.NewProc("FreeConsole")
	showWindow := user32.NewProc("ShowWindow")

	const swHide = 0

	if hwnd, _, _ := getConsoleWindow.Call(); hwnd != 0 {
		_, _, _ = showWindow.Call(hwnd, swHide)
	}
	_, _, _ = freeConsole.Call()
}
