package cli

import (
	"context"
	"encoding/json"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
)

// sshLoggingCache is the client-side representation of a
// jasper.LoggingCache for making requests to the remote service via the CLI
// over SSH.
type sshLoggingCache struct {
	ctx    context.Context
	client *sshRunner
}

func newSSHLoggingCache(ctx context.Context, client *sshRunner) *sshLoggingCache {
	return &sshLoggingCache{
		ctx:    ctx,
		client: client,
	}
}

func (lc *sshLoggingCache) Create(id string, opts *options.Output) (*options.CachedLogger, error) {
	output, err := lc.runCommand(lc.ctx, LoggingCacheCreateCommand, LoggingCacheCreateInput{
		ID:     id,
		Output: *opts,
	})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	resp, err := ExtractCachedLoggerResponse(output)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &resp.Logger, nil
}

func (lc *sshLoggingCache) Put(id string, cl *options.CachedLogger) error {
	return errors.New("operation not supported for remote managers")
}

func (lc *sshLoggingCache) Get(id string) *options.CachedLogger {
	output, err := lc.runCommand(lc.ctx, LoggingCacheGetCommand, IDInput{ID: id})
	if err != nil {
		grip.Warning(errors.Wrap(err, "running command"))
		return nil
	}

	resp, err := ExtractCachedLoggerResponse(output)
	if err != nil {
		grip.Warning(errors.Wrap(err, "reading cached logger response"))
		return nil
	}

	return &resp.Logger
}

func (lc *sshLoggingCache) Remove(id string) {
	output, err := lc.runCommand(lc.ctx, LoggingCacheRemoveCommand, IDInput{ID: id})
	if err != nil {
		grip.Warning(errors.Wrap(err, "running command"))
		return
	}

	if _, err = ExtractOutcomeResponse(output); err != nil {
		grip.Warning(errors.Wrap(err, "reading outcome response"))
	}
}

func (lc *sshLoggingCache) CloseAndRemove(ctx context.Context, id string) error {
	output, err := lc.runCommand(ctx, LoggingCacheCloseAndRemoveCommand, IDInput{ID: id})
	if err != nil {
		return errors.Wrap(err, "problem running command")
	}

	_, err = ExtractOutcomeResponse(output)
	return err
}

func (lc *sshLoggingCache) Clear(ctx context.Context) error {
	output, err := lc.runCommand(ctx, LoggingCacheCloseAndRemoveCommand, nil)
	if err != nil {
		return errors.Wrap(err, "problem running command")
	}

	_, err = ExtractOutcomeResponse(output)
	return err
}

func (lc *sshLoggingCache) Prune(ts time.Time) {
	output, err := lc.runCommand(lc.ctx, LoggingCachePruneCommand, LoggingCachePruneInput{LastAccessed: ts})
	if err != nil {
		grip.Warning(errors.Wrap(err, "running command"))
		return
	}

	if _, err = ExtractOutcomeResponse(output); err != nil {
		grip.Warning(errors.Wrap(err, "reading outcome response"))
	}
}

func (lc *sshLoggingCache) Len() int {
	output, err := lc.runCommand(lc.ctx, LoggingCacheLenCommand, nil)
	if err != nil {
		grip.Warning(errors.Wrap(err, "running command"))
		return -1
	}

	resp, err := ExtractLoggingCacheLenResponse(output)
	if err != nil {
		grip.Warning(errors.Wrap(err, "reading outcome response"))
		return -1
	}

	return resp.Length
}

func (lc *sshLoggingCache) runCommand(ctx context.Context, loggingCacheSubcommand string, subcommandInput interface{}) (json.RawMessage, error) {
	return lc.client.runClientCommand(ctx, []string{LoggingCacheCommand, loggingCacheSubcommand}, subcommandInput)
}
