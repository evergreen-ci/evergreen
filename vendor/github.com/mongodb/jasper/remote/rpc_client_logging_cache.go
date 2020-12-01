package remote

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes"
	empty "github.com/golang/protobuf/ptypes/empty"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/jasper/options"
	internal "github.com/mongodb/jasper/remote/internal"
	"github.com/pkg/errors"
)

// rpcLoggingCache is the client-side representation of a jasper.LoggingCache
// for making requests to the remote gRPC service.
type rpcLoggingCache struct {
	client internal.JasperProcessManagerClient
	ctx    context.Context
}

func (lc *rpcLoggingCache) Create(id string, opts *options.Output) (*options.CachedLogger, error) {
	args, err := internal.ConvertLoggingCreateArgs(id, opts)
	if err != nil {
		return nil, errors.Wrap(err, "problem converting create args")
	}
	resp, err := lc.client.LoggingCacheCreate(lc.ctx, args)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	out, err := resp.Export()
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return out, nil
}

func (lc *rpcLoggingCache) Put(id string, opts *options.CachedLogger) error {
	return errors.New("operation not supported for remote managers")
}

func (lc *rpcLoggingCache) Get(id string) *options.CachedLogger {
	resp, err := lc.client.LoggingCacheGet(lc.ctx, &internal.LoggingCacheArgs{Id: id})
	if err != nil {
		return nil
	}
	if !resp.Outcome.Success {
		return nil
	}

	out, err := resp.Export()
	if err != nil {
		return nil
	}

	return out
}

func (lc *rpcLoggingCache) Remove(id string) {
	_, _ = lc.client.LoggingCacheRemove(lc.ctx, &internal.LoggingCacheArgs{Id: id})
}

func (lc *rpcLoggingCache) CloseAndRemove(ctx context.Context, id string) error {
	resp, err := lc.client.LoggingCacheCloseAndRemove(ctx, &internal.LoggingCacheArgs{Id: id})
	if err != nil {
		return err
	}

	if !resp.Success {
		return errors.Errorf("failed to close and remove: %s", resp.Text)
	}
	return nil
}

func (lc *rpcLoggingCache) Clear(ctx context.Context) error {
	resp, err := lc.client.LoggingCacheClear(ctx, &empty.Empty{})
	if err != nil {
		return err
	}

	if !resp.Success {
		return errors.Errorf("failed to clear the logging cache: %s", resp.Text)
	}
	return nil
}

func (lc *rpcLoggingCache) Prune(ts time.Time) {
	pbts, err := ptypes.TimestampProto(ts)
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "could not convert prune timestamp to equivalent protobuf RPC timestamp",
		}))
		return
	}
	_, _ = lc.client.LoggingCachePrune(lc.ctx, pbts)
}

func (lc *rpcLoggingCache) Len() int {
	resp, err := lc.client.LoggingCacheLen(lc.ctx, &empty.Empty{})
	if err != nil {
		return -1
	}

	return int(resp.Size)
}
