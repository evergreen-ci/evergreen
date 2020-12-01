package internal

import (
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/evergreen-ci/lru"
	"github.com/golang/protobuf/ptypes"
	empty "github.com/golang/protobuf/ptypes/empty"
	timestamp "github.com/golang/protobuf/ptypes/timestamp"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/scripting"
	"github.com/pkg/errors"
	context "golang.org/x/net/context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

func newGRPCError(code codes.Code, err error) error {
	if err == nil {
		return nil
	}
	return status.Errorf(code, "%v", err)
}

// AttachService attaches the given manager to the jasper GRPC server. This
// function eventually calls generated Protobuf code for registering the
// GRPC Jasper server with the given Manager.
func AttachService(ctx context.Context, manager jasper.Manager, s *grpc.Server) error {
	hn, err := os.Hostname()
	if err != nil {
		return errors.WithStack(err)
	}

	srv := &jasperService{
		hostID:    hn,
		manager:   manager,
		cache:     lru.NewCache(),
		scripting: scripting.NewCache(),
		cacheOpts: options.Cache{
			PruneDelay: jasper.DefaultCachePruneDelay,
			MaxSize:    jasper.DefaultMaxCacheSize,
		},
	}

	RegisterJasperProcessManagerServer(s, srv)

	go srv.pruneCache(ctx)

	return nil
}

func (s *jasperService) pruneCache(ctx context.Context) {
	defer func() {
		err := recovery.HandlePanicWithError(recover(), nil, "cache pruning")
		if ctx.Err() != nil || err == nil {
			return
		}
		go s.pruneCache(ctx)
	}()

	s.cacheMutex.RLock()
	timer := time.NewTimer(s.cacheOpts.PruneDelay)
	s.cacheMutex.RUnlock()

	for {
		select {
		case <-timer.C:
			s.cacheMutex.RLock()
			if !s.cacheOpts.Disabled {
				if err := s.cache.Prune(s.cacheOpts.MaxSize, nil, false); err != nil {
					grip.Error(errors.Wrap(err, "error during cache pruning"))
				}
			}
			timer.Reset(s.cacheOpts.PruneDelay)
			s.cacheMutex.RUnlock()
		case <-ctx.Done():
			return
		}
	}
}

func getProcInfoNoHang(ctx context.Context, p jasper.Process) jasper.ProcessInfo {
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	return p.Info(ctx)
}

type jasperService struct {
	hostID     string
	manager    jasper.Manager
	scripting  scripting.HarnessCache
	cache      *lru.Cache
	cacheOpts  options.Cache
	cacheMutex sync.RWMutex
}

func (s *jasperService) Status(ctx context.Context, _ *empty.Empty) (*StatusResponse, error) {
	return &StatusResponse{
		HostId: s.hostID,
		Active: true,
	}, nil
}

func (s *jasperService) ID(ctx context.Context, _ *empty.Empty) (*IDResponse, error) {
	return &IDResponse{Value: s.manager.ID()}, nil
}

func (s *jasperService) Create(ctx context.Context, opts *CreateOptions) (*ProcessInfo, error) {
	jopts, err := opts.Export()
	if err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrap(err, "exporting create options"))
	}

	// Spawn a new context so that the process' context is not potentially
	// canceled by the request's. See how rest_service.go's createProcess() does
	// this same thing.
	pctx, cancel := context.WithCancel(context.Background())

	proc, err := s.manager.CreateProcess(pctx, jopts)
	if err != nil {
		return nil, newGRPCError(codes.Internal, errors.WithStack(err))
	}

	if err := proc.RegisterTrigger(ctx, func(_ jasper.ProcessInfo) {
		cancel()
	}); err != nil {
		cancel()
		// If we get an error registering a trigger, then we should make sure
		// that the reason for it isn't just because the process has exited
		// already, since that should not be considered an error.
		if !getProcInfoNoHang(ctx, proc).Complete {
			return nil, newGRPCError(codes.Internal, errors.Wrap(err, "registering trigger"))
		}
	}

	info, err := ConvertProcessInfo(getProcInfoNoHang(ctx, proc))
	if err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrapf(err, "converting info for process '%s'", proc.ID()))
	}
	return info, nil
}

func (s *jasperService) List(f *Filter, stream JasperProcessManager_ListServer) error {
	ctx := stream.Context()
	procs, err := s.manager.List(ctx, options.Filter(strings.ToLower(f.GetName().String())))
	if err != nil {
		return newGRPCError(codes.Internal, errors.WithStack(err))
	}

	for _, p := range procs {
		if ctx.Err() != nil {
			return newGRPCError(codes.DeadlineExceeded, errors.New("list cancelled"))
		}

		info, err := ConvertProcessInfo(getProcInfoNoHang(ctx, p))
		if err != nil {
			return newGRPCError(codes.Internal, errors.Wrapf(err, "converting info for process with id '%s'", p.ID()))
		}
		if err := stream.Send(info); err != nil {
			return newGRPCError(codes.Internal, errors.Wrap(err, "sending process info"))
		}
	}

	return nil
}

func (s *jasperService) Group(t *TagName, stream JasperProcessManager_GroupServer) error {
	ctx := stream.Context()
	procs, err := s.manager.Group(ctx, t.Value)
	if err != nil {
		return newGRPCError(codes.Internal, errors.WithStack(err))
	}

	for _, p := range procs {
		if ctx.Err() != nil {
			return newGRPCError(codes.DeadlineExceeded, errors.New("group cancelled"))
		}

		info, err := ConvertProcessInfo(getProcInfoNoHang(ctx, p))
		if err != nil {
			return newGRPCError(codes.Internal, errors.Wrapf(err, "getting info for process with id '%s'", p.ID()))
		}
		if err := stream.Send(info); err != nil {
			return newGRPCError(codes.Internal, errors.Wrap(err, "sending process info"))
		}
	}

	return nil
}

func (s *jasperService) Get(ctx context.Context, id *JasperProcessID) (*ProcessInfo, error) {
	proc, err := s.manager.Get(ctx, id.Value)
	if err != nil {
		return nil, newGRPCError(codes.NotFound, errors.Wrapf(err, "getting process with id '%s'", id.Value))
	}

	info, err := ConvertProcessInfo(getProcInfoNoHang(ctx, proc))
	if err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrapf(err, "could not get info for process '%s'", id.Value))
	}
	return info, nil
}

func (s *jasperService) Signal(ctx context.Context, sig *SignalProcess) (*OperationOutcome, error) {
	proc, err := s.manager.Get(ctx, sig.ProcessID.Value)
	if err != nil {
		err = errors.Wrapf(err, "couldn't find process with id '%s'", sig.ProcessID)
		return nil, newGRPCError(codes.NotFound, err)
	}

	if err = proc.Signal(ctx, sig.Signal.Export()); err != nil {
		err = errors.Wrapf(err, "problem sending '%s' to '%s'", sig.Signal, sig.ProcessID)
		return nil, newGRPCError(codes.Internal, err)
	}

	return &OperationOutcome{
		Success:  true,
		ExitCode: int32(getProcInfoNoHang(ctx, proc).ExitCode),
	}, nil
}

func (s *jasperService) Wait(ctx context.Context, id *JasperProcessID) (*OperationOutcome, error) {
	proc, err := s.manager.Get(ctx, id.Value)
	if err != nil {
		return nil, newGRPCError(codes.NotFound, errors.Wrapf(err, "getting process '%s'", id.Value))
	}

	exitCode, err := proc.Wait(ctx)
	if err != nil {
		return &OperationOutcome{
			Success:  false,
			Text:     errors.Wrap(err, "waiting for process").Error(),
			ExitCode: int32(exitCode),
		}, nil
	}

	return &OperationOutcome{
		Success:  true,
		ExitCode: int32(exitCode),
	}, nil
}

func (s *jasperService) Respawn(ctx context.Context, id *JasperProcessID) (*ProcessInfo, error) {
	proc, err := s.manager.Get(ctx, id.Value)
	if err != nil {
		return nil, newGRPCError(codes.NotFound, errors.Wrapf(err, "getting process with id '%s'", id.Value))
	}

	// Spawn a new context so that the process' context is not potentially
	// canceled by the request's. See how rest_service.go's createProcess() does
	// this same thing.
	pctx, cancel := context.WithCancel(context.Background())
	newProc, err := proc.Respawn(pctx)
	if err != nil {
		cancel()
		return nil, newGRPCError(codes.Internal, errors.Wrap(err, "respawning process"))
	}
	if err := s.manager.Register(ctx, newProc); err != nil {
		cancel()
		return nil, newGRPCError(codes.Internal, errors.WithStack(err))
	}

	if err := newProc.RegisterTrigger(ctx, func(_ jasper.ProcessInfo) {
		cancel()
	}); err != nil {
		cancel()
		// If we get an error registering a trigger, then we should make sure
		// that the reason for it isn't just because the process has exited
		// already, since that should not be considered an error.
		if !getProcInfoNoHang(ctx, newProc).Complete {
			return nil, newGRPCError(codes.Internal, errors.Wrap(err, "registering trigger"))
		}
	}

	newProcInfo, err := ConvertProcessInfo(getProcInfoNoHang(ctx, newProc))
	if err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrapf(err, "getting info for process '%s'", newProc.ID()))
	}
	return newProcInfo, nil
}

func (s *jasperService) Clear(ctx context.Context, _ *empty.Empty) (*OperationOutcome, error) {
	s.manager.Clear(ctx)

	return &OperationOutcome{Success: true}, nil
}

func (s *jasperService) Close(ctx context.Context, _ *empty.Empty) (*OperationOutcome, error) {
	if err := s.manager.Close(ctx); err != nil {
		err = errors.Wrap(err, "problem encountered closing service")
		return nil, newGRPCError(codes.Internal, errors.Wrap(err, "closing service"))
	}

	return &OperationOutcome{Success: true, ExitCode: 0}, nil
}

func (s *jasperService) GetTags(ctx context.Context, id *JasperProcessID) (*ProcessTags, error) {
	proc, err := s.manager.Get(ctx, id.Value)
	if err != nil {
		return nil, newGRPCError(codes.NotFound, errors.Wrapf(err, "getting process with id '%s'", id.Value))
	}

	return &ProcessTags{ProcessID: id.Value, Tags: proc.GetTags()}, nil
}

func (s *jasperService) TagProcess(ctx context.Context, tags *ProcessTags) (*OperationOutcome, error) {
	proc, err := s.manager.Get(ctx, tags.ProcessID)
	if err != nil {
		return nil, newGRPCError(codes.NotFound, errors.Wrapf(err, "getting process with id '%s'", tags.ProcessID))
	}

	for _, t := range tags.Tags {
		proc.Tag(t)
	}

	return &OperationOutcome{Success: true}, nil
}

func (s *jasperService) ResetTags(ctx context.Context, id *JasperProcessID) (*OperationOutcome, error) {
	proc, err := s.manager.Get(ctx, id.Value)
	if err != nil {
		return nil, newGRPCError(codes.NotFound, errors.Wrapf(err, "getting process with id '%s'", id.Value))
	}
	proc.ResetTags()
	return &OperationOutcome{Success: true}, nil
}

func (s *jasperService) DownloadMongoDB(ctx context.Context, opts *MongoDBDownloadOptions) (*OperationOutcome, error) {
	jopts := opts.Export()
	if err := jopts.Validate(); err != nil {
		return nil, newGRPCError(codes.InvalidArgument, errors.Wrap(err, "invalid MongoDB download options"))
	}

	if err := jasper.SetupDownloadMongoDBReleases(ctx, s.cache, jopts); err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrap(err, "download setup"))
	}

	return &OperationOutcome{Success: true}, nil
}

func (s *jasperService) ConfigureCache(ctx context.Context, opts *CacheOptions) (*OperationOutcome, error) {
	jopts := opts.Export()
	if err := jopts.Validate(); err != nil {
		err = errors.Wrap(err, "problem validating cache options")
		return &OperationOutcome{
			Success:  false,
			Text:     errors.Wrap(err, "problem validating cache options").Error(),
			ExitCode: -2,
		}, nil
	}

	s.cacheMutex.Lock()
	if jopts.MaxSize > 0 {
		s.cacheOpts.MaxSize = jopts.MaxSize
	}
	if jopts.PruneDelay > time.Duration(0) {
		s.cacheOpts.PruneDelay = jopts.PruneDelay
	}
	s.cacheOpts.Disabled = jopts.Disabled
	s.cacheMutex.Unlock()

	return &OperationOutcome{Success: true, Text: "cache configured"}, nil
}

func (s *jasperService) DownloadFile(ctx context.Context, opts *DownloadInfo) (*OperationOutcome, error) {
	jopts := opts.Export()

	if err := jopts.Validate(); err != nil {
		return nil, newGRPCError(codes.InvalidArgument, errors.Wrap(err, "validating download options"))
	}

	if err := jopts.Download(); err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrap(err, "downloading file"))
	}

	return &OperationOutcome{Success: true}, nil
}

func (s *jasperService) GetLogStream(ctx context.Context, request *LogRequest) (*LogStream, error) {
	id := request.Id
	proc, err := s.manager.Get(ctx, id.Value)
	if err != nil {
		return nil, newGRPCError(codes.NotFound, errors.Wrapf(err, "getting process with id '%s'", id.Value))
	}

	stream := &LogStream{}
	stream.Logs, err = jasper.GetInMemoryLogStream(ctx, proc, int(request.Count))
	if err == io.EOF {
		stream.Done = true
	} else if err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrapf(err, "getting logs for process '%s'", request.Id.Value))
	}
	return stream, nil
}

func (s *jasperService) GetBuildloggerURLs(ctx context.Context, id *JasperProcessID) (*BuildloggerURLs, error) {
	proc, err := s.manager.Get(ctx, id.Value)
	if err != nil {
		return nil, newGRPCError(codes.NotFound, errors.Wrapf(err, "getting process with id '%s'", id.Value))
	}

	var urls []string
	for _, logger := range getProcInfoNoHang(ctx, proc).Options.Output.Loggers {
		if logger.Type() == options.LogBuildloggerV2 {
			producer := logger.Producer()
			if producer == nil {
				continue
			}
			rawProducer, ok := producer.(*options.BuildloggerV2Options)
			if ok {
				urls = append(urls, rawProducer.Buildlogger.GetGlobalLogURL())
			}
		}
	}

	if len(urls) == 0 {
		return nil, newGRPCError(codes.InvalidArgument, errors.Errorf("process with id '%s' does not use buildlogger", id.Value))
	}

	return &BuildloggerURLs{Urls: urls}, nil
}

func (s *jasperService) RegisterSignalTriggerID(ctx context.Context, params *SignalTriggerParams) (*OperationOutcome, error) {
	jasperProcessID, signalTriggerID := params.Export()

	proc, err := s.manager.Get(ctx, jasperProcessID)
	if err != nil {
		return nil, newGRPCError(codes.NotFound, errors.Wrapf(err, "getting process with id '%s'", jasperProcessID))
	}

	makeTrigger, ok := jasper.GetSignalTriggerFactory(signalTriggerID)
	if !ok {
		return nil, newGRPCError(codes.NotFound, errors.Errorf("getting signal trigger with id '%s'", signalTriggerID))
	}

	if err := proc.RegisterSignalTrigger(ctx, makeTrigger()); err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrapf(err, "registering signal  trigger '%s'", signalTriggerID))
	}

	return &OperationOutcome{Success: true}, nil
}

func (s *jasperService) SignalEvent(ctx context.Context, name *EventName) (*OperationOutcome, error) {
	eventName := name.Value

	if err := jasper.SignalEvent(ctx, eventName); err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrapf(err, "signaling event '%s'", eventName))
	}

	return &OperationOutcome{Success: true}, nil
}

func (s *jasperService) WriteFile(stream JasperProcessManager_WriteFileServer) error {
	var jopts options.WriteFile

	for opts, err := stream.Recv(); err == nil; opts, err = stream.Recv() {
		if err == io.EOF {
			break
		}
		if err != nil {
			if sendErr := stream.SendAndClose(&OperationOutcome{
				Success:  false,
				Text:     errors.Wrap(err, "receiving from client stream").Error(),
				ExitCode: -2,
			}); sendErr != nil {
				return newGRPCError(codes.Internal, errors.Wrapf(sendErr, "sending error response to client: %s", err.Error()))
			}
			return nil
		}

		jopts = opts.Export()

		if err := jopts.Validate(); err != nil {
			if sendErr := stream.SendAndClose(&OperationOutcome{
				Success:  false,
				Text:     errors.Wrap(err, "validating file write options").Error(),
				ExitCode: -3,
			}); sendErr != nil {
				return newGRPCError(codes.Internal, errors.Wrapf(sendErr, "sending error response to client: %s", err.Error()))
			}
			return nil
		}

		if err := jopts.DoWrite(); err != nil {
			if sendErr := stream.SendAndClose(&OperationOutcome{
				Success:  false,
				Text:     errors.Wrap(err, "problem validating file write opts").Error(),
				ExitCode: -4,
			}); sendErr != nil {
				return newGRPCError(codes.Internal, errors.Wrapf(sendErr, "sending error response to client: %s", err.Error()))
			}
			return nil
		}
	}

	if err := jopts.SetPerm(); err != nil {
		if sendErr := stream.SendAndClose(&OperationOutcome{
			Success:  false,
			Text:     errors.Wrapf(err, "setting permissions for file %s", jopts.Path).Error(),
			ExitCode: -5,
		}); sendErr != nil {
			return newGRPCError(codes.Internal, errors.Wrapf(sendErr, "sending error response to client: %s", err.Error()))
		}
		return nil
	}

	if err := stream.SendAndClose(&OperationOutcome{
		Success: true,
	}); err != nil {
		return newGRPCError(codes.Internal, errors.Wrapf(err, "sending succses response to client"))
	}

	return nil
}

func (s *jasperService) ScriptingHarnessCreate(ctx context.Context, opts *ScriptingOptions) (*ScriptingHarnessID, error) {
	xopts, err := opts.Export()
	if err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrap(err, "converting options"))
	}

	if err = xopts.Validate(); err != nil {
		return nil, newGRPCError(codes.InvalidArgument, errors.Wrap(err, "invalid scripting options"))
	}

	sh, err := s.scripting.Create(s.manager, xopts)
	if err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrap(err, "generating scripting harness"))
	}

	return &ScriptingHarnessID{Id: sh.ID()}, nil
}
func (s *jasperService) ScriptingHarnessGet(ctx context.Context, id *ScriptingHarnessID) (*OperationOutcome, error) {
	sh, err := s.scripting.Get(id.Id)
	if err != nil {
		return nil, newGRPCError(codes.NotFound, errors.Wrapf(err, "getting scripting harness with id '%s'", id.Id))
	}

	return &OperationOutcome{
		Success: true,
		Text:    sh.ID(),
	}, nil
}

func (s *jasperService) ScriptingHarnessSetup(ctx context.Context, id *ScriptingHarnessID) (*OperationOutcome, error) {
	sh, err := s.scripting.Get(id.Id)
	if err != nil {
		return nil, newGRPCError(codes.NotFound, errors.Wrapf(err, "getting scripting harness with id '%s'", id.Id))
	}

	if err = sh.Setup(ctx); err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrapf(err, "setting up scripting harness"))
	}

	return &OperationOutcome{Success: true}, nil
}

func (s *jasperService) ScriptingHarnessCleanup(ctx context.Context, id *ScriptingHarnessID) (*OperationOutcome, error) {
	sh, err := s.scripting.Get(id.Id)
	if err != nil {
		return nil, newGRPCError(codes.NotFound, errors.Wrapf(err, "getting scripting harness with id '%s'", id.Id))
	}

	if err = sh.Cleanup(ctx); err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrapf(err, "cleaning up scripting harness"))
	}

	return &OperationOutcome{Success: true}, nil
}

func (s *jasperService) ScriptingHarnessRun(ctx context.Context, args *ScriptingHarnessRunArgs) (*OperationOutcome, error) {
	sh, err := s.scripting.Get(args.Id)
	if err != nil {
		return nil, newGRPCError(codes.NotFound, errors.Wrapf(err, "getting scripting harness with id '%s'", args.Id))
	}

	if err = sh.Run(ctx, args.Args); err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrapf(err, "running scripting command"))
	}

	return &OperationOutcome{Success: true}, nil
}

func (s *jasperService) ScriptingHarnessRunScript(ctx context.Context, args *ScriptingHarnessRunScriptArgs) (*OperationOutcome, error) {
	sh, err := s.scripting.Get(args.Id)
	if err != nil {
		return nil, newGRPCError(codes.NotFound, errors.Wrapf(err, "getting scripting harness with id '%s'", args.Id))
	}

	err = sh.RunScript(ctx, args.Script)
	if err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrapf(err, "running script"))
	}

	return &OperationOutcome{Success: true}, nil
}

func (s *jasperService) ScriptingHarnessBuild(ctx context.Context, args *ScriptingHarnessBuildArgs) (*ScriptingHarnessBuildResponse, error) {
	sh, err := s.scripting.Get(args.Id)
	if err != nil {
		return nil, newGRPCError(codes.NotFound, errors.Wrapf(err, "getting scripting harness with id '%s'", args.Id))
	}

	path, err := sh.Build(ctx, args.Directory, args.Args)
	if err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrapf(err, "running build"))
	}

	return &ScriptingHarnessBuildResponse{
		Path: path,
		Outcome: &OperationOutcome{
			Success: true,
		}}, nil
}

func (s *jasperService) ScriptingHarnessTest(ctx context.Context, args *ScriptingHarnessTestArgs) (*ScriptingHarnessTestResponse, error) {
	sh, err := s.scripting.Get(args.Id)
	if err != nil {
		return nil, newGRPCError(codes.NotFound, errors.Wrapf(err, "getting scripting harness with id '%s'", args.Id))
	}

	exportedArgs, err := args.Export()
	if err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrapf(err, "exporting arguments"))
	}

	var testErr error
	res, err := sh.Test(ctx, args.Directory, exportedArgs...)
	if err != nil {
		testErr = err
	}
	convertedRes, err := ConvertScriptingTestResults(res)
	if err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrapf(err, "converting test results"))
	}

	outcome := &OperationOutcome{Success: testErr == nil}
	if testErr != nil {
		outcome.Text = testErr.Error()
	}
	return &ScriptingHarnessTestResponse{
		Outcome: outcome,
		Results: convertedRes,
	}, nil
}

func (s *jasperService) LoggingCacheCreate(ctx context.Context, args *LoggingCacheCreateArgs) (*LoggingCacheInstance, error) {
	lc := s.manager.LoggingCache(ctx)
	if lc == nil {
		return nil, newGRPCError(codes.FailedPrecondition, errors.New("logging cache not supported"))
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
		return nil, newGRPCError(codes.FailedPrecondition, errors.New("logging cache not supported"))
	}

	out := lc.Get(args.Id)
	if out == nil {
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
		return nil, newGRPCError(codes.FailedPrecondition, errors.New("logging cache not supported"))
	}

	lc.Remove(args.Id)

	return &OperationOutcome{Success: true}, nil
}

func (s *jasperService) LoggingCacheCloseAndRemove(ctx context.Context, args *LoggingCacheArgs) (*OperationOutcome, error) {
	lc := s.manager.LoggingCache(ctx)
	if lc == nil {
		return nil, newGRPCError(codes.FailedPrecondition, errors.New("logging cache not supported"))
	}

	if err := lc.CloseAndRemove(ctx, args.Id); err != nil {
		return nil, newGRPCError(codes.Internal, err)
	}

	return &OperationOutcome{Success: true}, nil
}

func (s *jasperService) LoggingCacheClear(ctx context.Context, _ *empty.Empty) (*OperationOutcome, error) {
	lc := s.manager.LoggingCache(ctx)
	if lc == nil {
		return nil, newGRPCError(codes.FailedPrecondition, errors.New("logging cache not supported"))
	}

	if err := lc.Clear(ctx); err != nil {
		return nil, newGRPCError(codes.Internal, err)
	}

	return &OperationOutcome{Success: true}, nil
}

func (s *jasperService) LoggingCachePrune(ctx context.Context, arg *timestamp.Timestamp) (*OperationOutcome, error) {
	lc := s.manager.LoggingCache(ctx)
	if lc == nil {
		return nil, newGRPCError(codes.FailedPrecondition, errors.New("logging cache not supported"))
	}

	ts, err := ptypes.Timestamp(arg)
	if err != nil {
		return nil, newGRPCError(codes.Internal, errors.Wrap(err, "converting timestamp"))
	}

	lc.Prune(ts)

	return &OperationOutcome{Success: true}, nil
}

func (s *jasperService) LoggingCacheLen(ctx context.Context, _ *empty.Empty) (*LoggingCacheSize, error) {
	lc := s.manager.LoggingCache(ctx)
	if lc == nil {
		return nil, newGRPCError(codes.FailedPrecondition, errors.New("logging cache not supported"))
	}

	return &LoggingCacheSize{
		Outcome: &OperationOutcome{Success: true},
		Id:      s.manager.ID(),
		Size:    int64(lc.Len()),
	}, nil
}

func (s *jasperService) SendMessages(ctx context.Context, lp *LoggingPayload) (*OperationOutcome, error) {
	lc := s.manager.LoggingCache(ctx)
	if lc == nil {
		return nil, newGRPCError(codes.FailedPrecondition, errors.New("logging cache not supported"))
	}

	logger := lc.Get(lp.LoggerID)
	if logger == nil {
		return nil, newGRPCError(codes.NotFound, errors.Errorf("getting logger with id '%s'", lp.LoggerID))
	}

	payload := lp.Export()

	if err := logger.Send(payload); err != nil {
		return nil, newGRPCError(codes.NotFound, errors.Errorf("sending message to logger with id '%s'", lp.LoggerID))
	}

	return &OperationOutcome{Success: true}, nil
}
