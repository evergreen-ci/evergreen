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
	LoggingCacheCreateCommand         = "logging_cache_create"
	LoggingCacheGetCommand            = "logging_cache_get"
	LoggingCacheRemoveCommand         = "logging_cache_remove"
	LoggingCacheCloseAndRemoveCommand = "logging_cache_close_and_remove"
	LoggingCacheClearCommand          = "logging_cache_clear"
	LoggingCachePruneCommand          = "logging_cache_prune"
	LoggingCacheLenCommand            = "logging_cache_len"
)

func (s *mdbService) loggingCacheCreate(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := &loggingCacheCreateRequest{}
	lc, err := s.loggingCacheRequest(ctx, msg, req)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, err, LoggingCacheCreateCommand)
		return
	}

	if err := req.Params.Options.Validate(); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "invalid options"), LoggingCacheCreateCommand)
		return
	}

	logger, err := lc.Create(req.Params.ID, &req.Params.Options)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not create logger"), LoggingCacheCreateCommand)
		return
	}
	logger.ManagerID = s.manager.ID()

	s.loggingCacheResponse(ctx, w, loggingCacheLoggerResponse{CachedLogger: *logger, ErrorResponse: shell.MakeSuccessResponse()}, LoggingCacheCreateCommand)
}

func (s *mdbService) loggingCacheGet(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := &loggingCacheGetRequest{}
	lc, err := s.loggingCacheRequest(ctx, msg, req)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, err, LoggingCacheGetCommand)
		return
	}

	logger, err := lc.Get(req.ID)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrapf(err, "getting logger with id '%s'", req.ID), LoggingCacheGetCommand)
		return
	}

	s.loggingCacheResponse(ctx, w, loggingCacheLoggerResponse{CachedLogger: *logger, ErrorResponse: shell.MakeSuccessResponse()}, LoggingCacheGetCommand)
}

func (s *mdbService) loggingCacheRemove(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := &loggingCacheRemoveRequest{}
	lc, err := s.loggingCacheRequest(ctx, msg, req)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, err, LoggingCacheRemoveCommand)
		return
	}

	if err := lc.Remove(req.ID); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, err, LoggingCacheRemoveCommand)
		return
	}

	s.loggingCacheResponse(ctx, w, nil, LoggingCacheRemoveCommand)
}

func (s *mdbService) loggingCacheCloseAndRemove(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := &loggingCacheCloseAndRemoveRequest{}
	lc, err := s.loggingCacheRequest(ctx, msg, req)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, err, LoggingCacheCloseAndRemoveCommand)
		return
	}

	if err := lc.CloseAndRemove(ctx, req.ID); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, err, LoggingCacheCloseAndRemoveCommand)
		return
	}

	s.loggingCacheResponse(ctx, w, nil, LoggingCacheCloseAndRemoveCommand)
}

func (s *mdbService) loggingCacheClear(ctx context.Context, w io.Writer, msg mongowire.Message) {
	lc, err := s.loggingCacheRequest(ctx, msg, nil)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, err, LoggingCacheClearCommand)
		return
	}

	if err := lc.Clear(ctx); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, err, LoggingCacheClearCommand)
		return
	}

	s.loggingCacheResponse(ctx, w, nil, LoggingCacheClearCommand)
}

func (s *mdbService) loggingCachePrune(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := &loggingCachePruneRequest{}
	lc, err := s.loggingCacheRequest(ctx, msg, req)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, err, LoggingCachePruneCommand)
		return
	}

	if err := lc.Prune(req.LastAccessed); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, err, LoggingCachePruneCommand)
		return
	}

	s.loggingCacheResponse(ctx, w, nil, LoggingCachePruneCommand)
}

func (s *mdbService) loggingCacheLen(ctx context.Context, w io.Writer, msg mongowire.Message) {
	lc, err := s.loggingCacheRequest(ctx, msg, nil)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, err, LoggingCacheLenCommand)
		return
	}

	length, err := lc.Len()
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, err, LoggingCacheLenCommand)
		return
	}

	s.loggingCacheResponse(ctx, w, &loggingCacheLenResponse{Len: length, ErrorResponse: shell.MakeSuccessResponse()}, LoggingCacheLenCommand)
}

func (s *mdbService) loggingCacheRequest(ctx context.Context, msg mongowire.Message, req interface{}) (jasper.LoggingCache, error) {
	lc := s.manager.LoggingCache(ctx)
	if lc == nil {
		return nil, ErrLoggingCacheNotSupported
	}

	if req != nil {
		if err := shell.MessageToRequest(msg, req); err != nil {
			return nil, errors.Wrap(err, "could not read request")
		}
	}

	return lc, nil
}

func (s *mdbService) loggingCacheResponse(ctx context.Context, w io.Writer, resp interface{}, command string) {
	if resp == nil {
		shell.WriteOKResponse(ctx, w, mongowire.OP_REPLY, command)
		return
	}

	shellResp, err := shell.ResponseToMessage(mongowire.OP_REPLY, resp)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not make response"), command)
		return
	}

	shell.WriteResponse(ctx, w, shellResp, command)
}
