//go:build linux

package config

import (
	"os"
	"path/filepath"
)

func DefaultLogDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Documents", "EVE", "logs", "Chatlogs")
}
