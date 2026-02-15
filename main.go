package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"sentinel2-uploader/internal/config"
	"sentinel2-uploader/internal/ui/gui"
	"sentinel2-uploader/internal/ui/headless"

	flags "github.com/jessevdk/go-flags"
)

var BuildVersion = "dev"

func main() {
	rootCtx, stopSignals := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopSignals()

	opts, err := config.ParseOptions(nil)
	if err != nil {
		var flagErr *flags.Error
		if errors.As(err, &flagErr) && flagErr.Type == flags.ErrHelp {
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	// Headless-tag builds always run headless; runtime UI selection is ignored.
	if !gui.Available() {
		headless.Run(rootCtx, BuildVersion, opts)
		return
	}

	if opts.Headless {
		headless.Run(rootCtx, BuildVersion, opts)
		return
	}
	hideAndDetachConsoleForGUI()
	gui.Run(rootCtx, BuildVersion, opts)
}
