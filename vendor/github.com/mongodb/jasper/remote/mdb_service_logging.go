package remote

import (
	"context"
	"io"

	"github.com/evergreen-ci/mrpc/mongowire"
	"github.com/evergreen-ci/mrpc/shell"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
)

// Constants representing logging cache commands.
const (
	LoggingCacheSizeCommand   = "logging_cache_size"
	LoggingCacheCreateCommand = "logging_cache_create"
	LoggingCacheRemoveCommand = "logging_cache_remove"
	LoggingCacheGetCommand    = "logging_cache_get"
	LoggingCachePruneCommand  = "logging_cache_prune"
)

func (s *mdbService) loggingCreate(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := &loggingCacheCreateRequest{}
	lc := s.loggingCacheRequest(ctx, w, msg, req, LoggingCacheCreateCommand)
	if lc == nil {
		return
	}

	if err := req.Params.Options.Validate(); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "invalid options"), LoggingCacheCreateCommand)
		return
	}

	cachedLogger, err := lc.Create(req.Params.ID, &req.Params.Options)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not create logger"), LoggingCacheCreateCommand)
		return
	}
	cachedLogger.ManagerID = s.manager.ID()

	s.loggingCacheResponse(ctx, w, makeLoggingCacheCreateAndGetResponse(*cachedLogger), LoggingCacheCreateCommand)
}

func (s *mdbService) loggingGet(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := &loggingCacheGetRequest{}
	lc := s.loggingCacheRequest(ctx, w, msg, req, LoggingCacheGetCommand)
	if lc == nil {
		return
	}

	cachedLogger := lc.Get(req.ID)
	if cachedLogger == nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.New("named logger does not exist"), LoggingCacheGetCommand)
		return
	}

	s.loggingCacheResponse(ctx, w, makeLoggingCacheCreateAndGetResponse(*cachedLogger), LoggingCacheGetCommand)
}

func (s *mdbService) loggingRemove(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := &loggingCacheRemoveRequest{}
	lc := s.loggingCacheRequest(ctx, w, msg, req, LoggingCacheRemoveCommand)
	if lc == nil {
		return
	}

	lc.Remove(req.ID)

	s.loggingCacheResponse(ctx, w, nil, LoggingCacheRemoveCommand)
}

func (s *mdbService) loggingPrune(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := &loggingCachePruneRequest{}
	lc := s.loggingCacheRequest(ctx, w, msg, req, LoggingCachePruneCommand)
	if lc == nil {
		return
	}

	lc.Prune(req.LastAccessed)

	s.loggingCacheResponse(ctx, w, nil, LoggingCachePruneCommand)
}

func (s *mdbService) loggingSize(ctx context.Context, w io.Writer, msg mongowire.Message) {
	lc := s.loggingCacheRequest(ctx, w, msg, nil, LoggingCacheSizeCommand)
	if lc == nil {
		return
	}

	s.loggingCacheResponse(ctx, w, &loggingCacheSizeResponse{Size: lc.Len()}, LoggingCacheSizeCommand)
}

func (s *mdbService) loggingCacheRequest(ctx context.Context, w io.Writer, msg mongowire.Message, req interface{}, command string) jasper.LoggingCache {
	lc := s.manager.LoggingCache(ctx)
	if lc == nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.New("logging cache not supported"), command)
		return nil
	}

	if req != nil {
		if err := shell.MessageToRequest(msg, req); err != nil {
			shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not read request"), command)
			return nil
		}
	}

	return lc
}

func (s *mdbService) loggingCacheResponse(ctx context.Context, w io.Writer, resp interface{}, command string) {
	if resp != nil {
		shellResp, err := shell.ResponseToMessage(mongowire.OP_REPLY, resp)
		if err != nil {
			shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not make response"), command)
			return
		}

		shell.WriteResponse(ctx, w, shellResp, command)
	} else {
		shell.WriteOKResponse(ctx, w, mongowire.OP_REPLY, command)
	}
}
