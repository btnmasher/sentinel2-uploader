package runctx

import (
	"context"

	"sentinel2-uploader/internal/logging"
)

func RecvOrDone[T any](ctx context.Context, name string, logger *logging.Logger, in <-chan T) (T, bool) {
	if logger == nil {
		panic("runctx.RecvOrDone: logger must not be nil")
	}
	select {
	case <-ctx.Done():
		logger.Debug("stopping "+name+": context canceled", logging.Field("error", ctx.Err()))
		var zero T
		return zero, false
	case v, ok := <-in:
		if !ok {
			logger.Debug("stopping " + name + ": input channel closed")
		}
		return v, ok
	}
}

func SendOrDone[T any](ctx context.Context, name string, logger *logging.Logger, out chan<- T, value T) bool {
	if logger == nil {
		panic("runctx.SendOrDone: logger must not be nil")
	}
	select {
	case <-ctx.Done():
		logger.Debug("stopping "+name+": context canceled before send", logging.Field("error", ctx.Err()))
		return false
	case out <- value:
		return true
	}
}
