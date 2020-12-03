package remote

import (
	"context"
	"time"

	"github.com/evergreen-ci/mrpc/mongowire"
	"github.com/evergreen-ci/mrpc/shell"
	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
)

// mdbLoggingCache is the client-side representation of a jasper.LoggingCache
// for making requests to the remote MDB wire protocol service.
type mdbLoggingCache struct {
	client *mdbClient
	ctx    context.Context
}

func (lc *mdbLoggingCache) Create(id string, opts *options.Output) (*options.CachedLogger, error) {
	r := &loggingCacheCreateRequest{}
	r.Params.ID = id
	r.Params.Options = *opts
	req, err := shell.RequestToMessage(mongowire.OP_QUERY, r)
	if err != nil {
		return nil, errors.Wrap(err, "could not create request")
	}

	msg, err := lc.client.doRequest(lc.ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "failed during request")
	}

	resp := &loggingCacheCreateAndGetResponse{}
	if err = shell.MessageToResponse(msg, resp); err != nil {
		return nil, errors.Wrap(err, "could not read response")
	}
	if err = resp.SuccessOrError(); err != nil {
		return nil, errors.Wrap(err, "error in response")
	}

	return &resp.CachedLogger, nil
}

func (lc *mdbLoggingCache) Put(_ string, _ *options.CachedLogger) error {
	return errors.New("operation not supported for remote managers")
}

func (lc *mdbLoggingCache) Get(id string) *options.CachedLogger {
	req, err := shell.RequestToMessage(mongowire.OP_QUERY, &loggingCacheGetRequest{ID: id})
	if err != nil {
		return nil
	}

	msg, err := lc.client.doRequest(lc.ctx, req)
	if err != nil {
		return nil
	}

	resp := &loggingCacheCreateAndGetResponse{}
	if err = shell.MessageToResponse(msg, resp); err != nil {
		return nil
	}
	if err = resp.SuccessOrError(); err != nil {
		return nil
	}

	return &resp.CachedLogger
}

func (lc *mdbLoggingCache) Remove(id string) {
	req, err := shell.RequestToMessage(mongowire.OP_QUERY, &loggingCacheRemoveRequest{ID: id})
	if err != nil {
		return
	}

	_, _ = lc.client.doRequest(lc.ctx, req)
}

func (lc *mdbLoggingCache) CloseAndRemove(ctx context.Context, id string) error {
	req, err := shell.RequestToMessage(mongowire.OP_QUERY, &loggingCacheCloseAndRemoveRequest{ID: id})
	if err != nil {
		return errors.Wrap(err, "could not create request")
	}

	msg, err := lc.client.doRequest(ctx, req)
	if err != nil {
		return err
	}

	var resp shell.ErrorResponse
	if err = shell.MessageToResponse(msg, &resp); err != nil {
		return errors.Wrap(err, "could not read response")
	}

	return resp.SuccessOrError()
}

func (lc *mdbLoggingCache) Clear(ctx context.Context) error {
	req, err := shell.RequestToMessage(mongowire.OP_QUERY, &loggingCacheClearRequest{Clear: 1})
	if err != nil {
		return errors.Wrap(err, "could not create request")
	}

	msg, err := lc.client.doRequest(ctx, req)
	if err != nil {
		return err
	}

	var resp shell.ErrorResponse
	if err = shell.MessageToResponse(msg, &resp); err != nil {
		return errors.Wrap(err, "could not read response")
	}

	return resp.SuccessOrError()
}

func (lc *mdbLoggingCache) Prune(lastAccessed time.Time) {
	req, err := shell.RequestToMessage(mongowire.OP_QUERY, &loggingCachePruneRequest{LastAccessed: lastAccessed})
	if err != nil {
		return
	}

	_, _ = lc.client.doRequest(lc.ctx, req)
}

func (lc *mdbLoggingCache) Len() int {
	req, err := shell.RequestToMessage(mongowire.OP_QUERY, &loggingCacheLenRequest{})
	if err != nil {
		return -1
	}

	msg, err := lc.client.doRequest(lc.ctx, req)
	if err != nil {
		return -1
	}

	resp := &loggingCacheSizeResponse{}
	if err = shell.MessageToResponse(msg, &resp); err != nil {
		return -1
	}

	return resp.Size
}
