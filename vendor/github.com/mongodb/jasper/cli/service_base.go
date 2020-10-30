package cli

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
)

// daemonOptions represent common options to initialize a daemon service.
type daemonOptions struct {
	host             string
	port             int
	manager          jasper.Manager
	logger           *options.LoggerConfig
	preconditionCmds []string
}

// baseDaemon represents common functionality for a daemon service.
type baseDaemon struct {
	daemonOptions
	exit chan struct{}
}

// newBaseDaemon initializes a base daemon service.
func newBaseDaemon(opts daemonOptions) baseDaemon {
	return baseDaemon{
		daemonOptions: opts,
		exit:          make(chan struct{}),
	}
}

// setup performs required setup to prepare to run the service daemon.
func (d *baseDaemon) setup(ctx context.Context, cancel context.CancelFunc) error {
	if d.logger != nil {
		if err := d.setupLogger(d.logger); err != nil {
			return errors.Wrap(err, "setting up logging")
		}
	}

	if d.manager == nil {
		var err error
		if d.manager, err = jasper.NewSynchronizedManager(false); err != nil {
			return errors.Wrap(err, "initializing process manager")
		}
	}

	if err := d.checkPreconditions(ctx); err != nil {
		return errors.Wrap(err, "precondition(s) failed")
	}

	go d.handleSignals(ctx, cancel, d.exit)

	return nil
}

// setupLogger creates a logger and sets it as the global logger.
func (d *baseDaemon) setupLogger(opts *options.LoggerConfig) error {
	sender, err := opts.Resolve()
	if err != nil {
		return errors.Wrap(err, "could not configure logging")
	}
	return errors.Wrap(grip.SetSender(sender), "could not set grip logger")
}

// checkPreconditions runs the daemon's precondition commands.
func (d *baseDaemon) checkPreconditions(ctx context.Context) error {
	catcher := grip.NewBasicCatcher()
	for _, cmd := range d.preconditionCmds {
		catcher.Wrapf(d.checkPrecondition(ctx, cmd), "file '%s'", cmd)
	}
	return catcher.Resolve()
}

// checkPrecondition runs a single precondition command.
func (d *baseDaemon) checkPrecondition(ctx context.Context, cmd string) error {
	if err := d.manager.CreateCommand(ctx).Append(cmd).Run(ctx); err != nil {
		return errors.Wrap(err, "executing precondition")
	}

	return nil
}

// handleSignals shuts down the daemon by cancelling the context, either
// when the context is done, it receives a terminate signal, or when it
// receives a signal to exit the daemon.
func (d *baseDaemon) handleSignals(ctx context.Context, cancel context.CancelFunc, exit chan struct{}) {
	defer recovery.LogStackTraceAndContinue("graceful shutdown")
	defer cancel()
	sig := make(chan os.Signal, 2)
	signal.Notify(sig, syscall.SIGTERM, os.Interrupt)

	select {
	case <-sig:
		grip.Debug("received signal")
	case <-ctx.Done():
		grip.Debug("context canceled")
	case <-exit:
		grip.Debug("received daemon exit signal")
	}
}
