package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"sentinel2-uploader/internal/client"
	"sentinel2-uploader/internal/config"
	"sentinel2-uploader/internal/logging"
)

type Controller struct {
	rootCtx context.Context
	mu      sync.Mutex
	cancel  context.CancelFunc
	running bool
	wg      sync.WaitGroup
}

type StartHooks struct {
	OnChannelsUpdate func([]client.ChannelConfig)
	OnStatus         func(string)
	OnExit           func(error)
}

func NewController(rootCtx context.Context) *Controller {
	if rootCtx == nil {
		rootCtx = context.Background()
	}
	return &Controller{rootCtx: rootCtx}
}

func (c *Controller) Start(opts config.Options, logger *logging.Logger, hooks StartHooks) error {
	if logger == nil {
		panic("runtime.Controller.Start: logger must not be nil")
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return fmt.Errorf("uploader is already running")
	}
	if err := config.ValidateRequired(opts); err != nil {
		return err
	}
	logger.Debug("runtime start requested",
		logging.Field("log_dir", opts.LogDir),
		logging.Field("log_file", opts.LogFile),
		logging.Field("has_channel_hook", hooks.OnChannelsUpdate != nil),
	)

	service, err := NewServiceWithHooks(opts, logger, hooks)
	if err != nil {
		return err
	}

	parent := c.rootCtx
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)

	c.cancel = cancel
	c.running = true
	c.wg.Go(func() {
		defer cancel()
		runErr := service.RunContext(ctx)
		if errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded) {
			logger.Debug("runtime service exited due to context cancellation", logging.Field("error", runErr))
		} else if runErr != nil {
			logger.Warn("runtime service exited with error", logging.Field("error", runErr))
		} else {
			logger.Info("runtime service exited")
		}
		c.mu.Lock()
		c.running = false
		c.cancel = nil
		c.mu.Unlock()

		if hooks.OnExit != nil {
			hooks.OnExit(runErr)
		}
	})

	return nil
}

func (c *Controller) Stop() {
	c.mu.Lock()
	cancel := c.cancel
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (c *Controller) Wait(timeout time.Duration) bool {
	waitDone := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(waitDone)
	}()
	if timeout <= 0 {
		<-waitDone
		return true
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-waitDone:
		return true
	case <-timer.C:
		return false
	}
}

func (c *Controller) StopAndWait(timeout time.Duration) bool {
	c.Stop()
	return c.Wait(timeout)
}

func (c *Controller) IsRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}
