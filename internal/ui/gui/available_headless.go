//go:build headless

package gui

import (
	"context"
	"sentinel2-uploader/internal/config"
)

func Available() bool {
	return false
}

func Run(_ context.Context, _ string, _ config.Options) {}
