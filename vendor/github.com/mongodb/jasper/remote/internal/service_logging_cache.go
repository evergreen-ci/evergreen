package internal

import (
	context "context"

	"github.com/golang/protobuf/ptypes"
	empty "github.com/golang/protobuf/ptypes/empty"
	timestamp "github.com/golang/protobuf/ptypes/timestamp"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	codes "google.golang.org/grpc/codes"
)

var errLoggingCacheNotSupported = errors.New("logging cache is not supported")

func (s *jasperService) LoggingCacheCreate(ctx context.Context, args *LoggingCacheCreateArgs) (*LoggingCacheInstance, error) {
	lc := s.manager.LoggingCache(ctx)
	if lc == nil {
		return nil, newGRPCError(codes.FailedPrecondition, errLoggingCacheNotSupported)
	}
	opts, err := args.Options.Export()
	if err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrap(err, "exporting options"))
	}
	if err := opts.Validate(); err != nil {
		return nil, newGRPCError(codes.InvalidArgument, errors.Wrap(err, "invalid options"))
	}

	out, err := lc.Create(args.Id, &opts)
	if err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrap(err, "creating logger"))
	}
	out.ManagerID = s.manager.ID()

	logger, err := ConvertCachedLogger(out)
	if err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrap(err, "converting cached logger"))
	}

	return logger, nil
}

func (s *jasperService) LoggingCacheGet(ctx context.Context, args *LoggingCacheArgs) (*LoggingCacheInstance, error) {
	lc := s.manager.LoggingCache(ctx)
	if lc == nil {
		return nil, newGRPCError(codes.FailedPrecondition, errLoggingCacheNotSupported)
	}

	out, err := lc.Get(args.Id)
	if err != nil {
		return nil, newGRPCError(codes.NotFound, errors.Errorf("getting logger with id '%s'", args.Id))
	}

	logger, err := ConvertCachedLogger(out)
	if err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrap(err, "converting cached logger"))
	}

	return logger, nil
}

func (s *jasperService) LoggingCacheRemove(ctx context.Context, args *LoggingCacheArgs) (*OperationOutcome, error) {
	lc := s.manager.LoggingCache(ctx)
	if lc == nil {
		return nil, newGRPCError(codes.FailedPrecondition, errLoggingCacheNotSupported)
	}

	if err := lc.Remove(args.Id); err != nil {
		code := codes.Internal
		if errors.Cause(err) == jasper.ErrCachedLoggerNotFound {
			code = codes.NotFound
		}
		return nil, newGRPCError(code, errors.Wrapf(err, "removing logger with id '%s'", args.Id))
	}

	return &OperationOutcome{Success: true}, nil
}

func (s *jasperService) LoggingCacheCloseAndRemove(ctx context.Context, args *LoggingCacheArgs) (*OperationOutcome, error) {
	lc := s.manager.LoggingCache(ctx)
	if lc == nil {
		return nil, newGRPCError(codes.FailedPrecondition, errLoggingCacheNotSupported)
	}

	if err := lc.CloseAndRemove(ctx, args.Id); err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrapf(err, "closing and removing logger with id '%s'", args.Id))
	}

	return &OperationOutcome{Success: true}, nil
}

func (s *jasperService) LoggingCacheClear(ctx context.Context, _ *empty.Empty) (*OperationOutcome, error) {
	lc := s.manager.LoggingCache(ctx)
	if lc == nil {
		return nil, newGRPCError(codes.FailedPrecondition, errLoggingCacheNotSupported)
	}

	if err := lc.Clear(ctx); err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrap(err, "clearing logging cache"))
	}

	return &OperationOutcome{Success: true}, nil
}

func (s *jasperService) LoggingCachePrune(ctx context.Context, arg *timestamp.Timestamp) (*OperationOutcome, error) {
	lc := s.manager.LoggingCache(ctx)
	if lc == nil {
		return nil, newGRPCError(codes.FailedPrecondition, errLoggingCacheNotSupported)
	}

	ts, err := ptypes.Timestamp(arg)
	if err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrap(err, "converting input timestamp"))
	}

	if err := lc.Prune(ts); err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrap(err, "pruning logging cache"))
	}

	return &OperationOutcome{Success: true}, nil
}

func (s *jasperService) LoggingCacheLen(ctx context.Context, _ *empty.Empty) (*LoggingCacheLenResponse, error) {
	lc := s.manager.LoggingCache(ctx)
	if lc == nil {
		return nil, newGRPCError(codes.FailedPrecondition, errLoggingCacheNotSupported)
	}

	length, err := lc.Len()
	if err != nil {
		return nil, errors.Wrap(err, "getting logging cache length")
	}

	return &LoggingCacheLenResponse{
		Outcome: &OperationOutcome{Success: true},
		Len:     int64(length),
	}, nil
}
