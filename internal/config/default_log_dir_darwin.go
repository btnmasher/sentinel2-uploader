//go:build darwin

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
	return filepath.Join(home, "Library", "Application Support", "EVE Online", "p_drive", "User", "My Documents", "EVE", "logs", "Chatlogs")
}
