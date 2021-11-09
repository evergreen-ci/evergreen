package cli

import (
	"context"
	"encoding/json"
	"time"

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
		return nil, errors.Wrap(err, "running command")
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

func (lc *sshLoggingCache) Get(id string) (*options.CachedLogger, error) {
	output, err := lc.runCommand(lc.ctx, LoggingCacheGetCommand, IDInput{ID: id})
	if err != nil {
		return nil, errors.Wrap(err, "running command")
	}

	resp, err := ExtractCachedLoggerResponse(output)
	if err != nil {
		return nil, errors.Wrap(err, "reading cached logger response")
	}

	return &resp.Logger, nil
}

func (lc *sshLoggingCache) Remove(id string) error {
	output, err := lc.runCommand(lc.ctx, LoggingCacheRemoveCommand, IDInput{ID: id})
	if err != nil {
		return errors.Wrap(err, "running command")
	}

	if _, err = ExtractOutcomeResponse(output); err != nil {
		return errors.Wrap(err, "reading outcome response")
	}

	return nil
}

func (lc *sshLoggingCache) CloseAndRemove(ctx context.Context, id string) error {
	output, err := lc.runCommand(ctx, LoggingCacheCloseAndRemoveCommand, IDInput{ID: id})
	if err != nil {
		return errors.Wrap(err, "problem running command")
	}

	if _, err = ExtractOutcomeResponse(output); err != nil {
		return errors.Wrap(err, "reading outcome response")
	}

	return nil
}

func (lc *sshLoggingCache) Clear(ctx context.Context) error {
	output, err := lc.runCommand(ctx, LoggingCacheClearCommand, nil)
	if err != nil {
		return errors.Wrap(err, "problem running command")
	}

	_, err = ExtractOutcomeResponse(output)
	return err
}

func (lc *sshLoggingCache) Prune(ts time.Time) error {
	output, err := lc.runCommand(lc.ctx, LoggingCachePruneCommand, LoggingCachePruneInput{LastAccessed: ts})
	if err != nil {
		return errors.Wrap(err, "running command")
	}

	if _, err = ExtractOutcomeResponse(output); err != nil {
		return errors.Wrap(err, "reading outcome response")
	}

	return nil
}

func (lc *sshLoggingCache) Len() (int, error) {
	output, err := lc.runCommand(lc.ctx, LoggingCacheLenCommand, nil)
	if err != nil {
		return -1, errors.Wrap(err, "running command")
	}

	resp, err := ExtractLoggingCacheLenResponse(output)
	if err != nil {
		return -1, errors.Wrap(err, "reading outcome response")
	}

	return resp.Len, nil
}

func (lc *sshLoggingCache) runCommand(ctx context.Context, loggingCacheSubcommand string, subcommandInput interface{}) (json.RawMessage, error) {
	return lc.client.runClientCommand(ctx, []string{LoggingCacheCommand, loggingCacheSubcommand}, subcommandInput)
}
