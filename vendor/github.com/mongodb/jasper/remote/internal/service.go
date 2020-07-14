package internal

import (
	"fmt"
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
)

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
		return nil, errors.Wrap(err, "problem exporting create options")
	}

	// Spawn a new context so that the process' context is not potentially
	// canceled by the request's. See how rest_service.go's createProcess() does
	// this same thing.
	pctx, cancel := context.WithCancel(context.Background())

	proc, err := s.manager.CreateProcess(pctx, jopts)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if err := proc.RegisterTrigger(ctx, func(_ jasper.ProcessInfo) {
		cancel()
	}); err != nil {
		cancel()
		// If we get an error registering a trigger, then we should make sure
		// that the reason for it isn't just because the process has exited
		// already, since that should not be considered an error.
		if !getProcInfoNoHang(ctx, proc).Complete {
			return nil, errors.WithStack(err)
		}
	}

	info, err := ConvertProcessInfo(getProcInfoNoHang(ctx, proc))
	if err != nil {
		return nil, errors.Wrapf(err, "could not convert info for process '%s'", proc.ID())
	}
	return info, nil
}

func (s *jasperService) List(f *Filter, stream JasperProcessManager_ListServer) error {
	ctx := stream.Context()
	procs, err := s.manager.List(ctx, options.Filter(strings.ToLower(f.GetName().String())))
	if err != nil {
		return errors.WithStack(err)
	}

	for _, p := range procs {
		if ctx.Err() != nil {
			return errors.New("list canceled")
		}

		info, err := ConvertProcessInfo(getProcInfoNoHang(ctx, p))
		if err != nil {
			return errors.Wrapf(err, "could not convert info for process '%s'", p.ID())
		}
		if err := stream.Send(info); err != nil {
			return errors.Wrap(err, "problem sending process info")
		}
	}

	return nil
}

func (s *jasperService) Group(t *TagName, stream JasperProcessManager_GroupServer) error {
	ctx := stream.Context()
	procs, err := s.manager.Group(ctx, t.Value)
	if err != nil {
		return errors.WithStack(err)
	}

	for _, p := range procs {
		if ctx.Err() != nil {
			return errors.New("list canceled")
		}

		info, err := ConvertProcessInfo(getProcInfoNoHang(ctx, p))
		if err != nil {
			return errors.Wrapf(err, "could not get info for process '%s'", p.ID())
		}
		if err := stream.Send(info); err != nil {
			return errors.Wrap(err, "problem sending process info")
		}
	}

	return nil
}

func (s *jasperService) Get(ctx context.Context, id *JasperProcessID) (*ProcessInfo, error) {
	proc, err := s.manager.Get(ctx, id.Value)
	if err != nil {
		return nil, errors.Wrapf(err, "problem fetching process '%s'", id.Value)
	}

	info, err := ConvertProcessInfo(getProcInfoNoHang(ctx, proc))
	if err != nil {
		return nil, errors.Wrapf(err, "could not get info for process '%s'", id.Value)
	}
	return info, nil
}

func (s *jasperService) Signal(ctx context.Context, sig *SignalProcess) (*OperationOutcome, error) {
	proc, err := s.manager.Get(ctx, sig.ProcessID.Value)
	if err != nil {
		err = errors.Wrapf(err, "couldn't find process with id '%s'", sig.ProcessID)
		return &OperationOutcome{
			Success:  false,
			Text:     err.Error(),
			ExitCode: -2,
		}, nil
	}

	if err = proc.Signal(ctx, sig.Signal.Export()); err != nil {
		err = errors.Wrapf(err, "problem sending '%s' to '%s'", sig.Signal, sig.ProcessID)
		return &OperationOutcome{
			Success:  false,
			ExitCode: -3,
			Text:     err.Error(),
		}, nil
	}

	return &OperationOutcome{
		Success:  true,
		Text:     fmt.Sprintf("sending '%s' to '%s'", sig.Signal, sig.ProcessID),
		ExitCode: int32(getProcInfoNoHang(ctx, proc).ExitCode),
	}, nil
}

func (s *jasperService) Wait(ctx context.Context, id *JasperProcessID) (*OperationOutcome, error) {
	proc, err := s.manager.Get(ctx, id.Value)
	if err != nil {
		err = errors.Wrapf(err, "problem finding process '%s'", id.Value)
		return &OperationOutcome{
			Success:  false,
			Text:     err.Error(),
			ExitCode: -2,
		}, nil
	}

	exitCode, err := proc.Wait(ctx)
	if err != nil {
		err = errors.Wrap(err, "problem encountered while waiting")
		return &OperationOutcome{
			Success:  false,
			Text:     err.Error(),
			ExitCode: int32(exitCode),
		}, nil
	}

	return &OperationOutcome{
		Success:  true,
		Text:     fmt.Sprintf("wait completed on process with id '%s'", id.Value),
		ExitCode: int32(exitCode),
	}, nil
}

func (s *jasperService) Respawn(ctx context.Context, id *JasperProcessID) (*ProcessInfo, error) {
	proc, err := s.manager.Get(ctx, id.Value)
	if err != nil {
		err = errors.Wrapf(err, "problem finding process '%s'", id.Value)
		return nil, errors.WithStack(err)
	}

	// Spawn a new context so that the process' context is not potentially
	// canceled by the request's. See how rest_service.go's createProcess() does
	// this same thing.
	pctx, cancel := context.WithCancel(context.Background())
	newProc, err := proc.Respawn(pctx)
	if err != nil {
		err = errors.Wrap(err, "problem encountered while respawning")
		cancel()
		return nil, errors.WithStack(err)
	}
	if err := s.manager.Register(ctx, newProc); err != nil {
		cancel()
		return nil, errors.WithStack(err)
	}

	if err := newProc.RegisterTrigger(ctx, func(_ jasper.ProcessInfo) {
		cancel()
	}); err != nil {
		cancel()
		if !getProcInfoNoHang(ctx, newProc).Complete {
			return nil, errors.WithStack(err)
		}
	}

	newProcInfo, err := ConvertProcessInfo(getProcInfoNoHang(ctx, newProc))
	if err != nil {
		return nil, errors.Wrapf(err, "could not get info for process '%s'", newProc.ID())
	}
	return newProcInfo, nil
}

func (s *jasperService) Clear(ctx context.Context, _ *empty.Empty) (*OperationOutcome, error) {
	s.manager.Clear(ctx)

	return &OperationOutcome{Success: true, Text: "service cleared", ExitCode: 0}, nil
}

func (s *jasperService) Close(ctx context.Context, _ *empty.Empty) (*OperationOutcome, error) {
	if err := s.manager.Close(ctx); err != nil {
		err = errors.Wrap(err, "problem encountered closing service")
		return &OperationOutcome{
			Success:  false,
			ExitCode: 1,
			Text:     err.Error(),
		}, err
	}

	return &OperationOutcome{Success: true, Text: "service closed", ExitCode: 0}, nil
}

func (s *jasperService) GetTags(ctx context.Context, id *JasperProcessID) (*ProcessTags, error) {
	proc, err := s.manager.Get(ctx, id.Value)
	if err != nil {
		return nil, errors.Wrapf(err, "problem finding process '%s'", id.Value)
	}

	return &ProcessTags{ProcessID: id.Value, Tags: proc.GetTags()}, nil
}

func (s *jasperService) TagProcess(ctx context.Context, tags *ProcessTags) (*OperationOutcome, error) {
	proc, err := s.manager.Get(ctx, tags.ProcessID)
	if err != nil {
		err = errors.Wrapf(err, "problem finding process '%s'", tags.ProcessID)
		return &OperationOutcome{
			ExitCode: 1,
			Success:  false,
			Text:     err.Error(),
		}, err
	}

	for _, t := range tags.Tags {
		proc.Tag(t)
	}

	return &OperationOutcome{
		Success:  true,
		ExitCode: 0,
		Text:     "added tags",
	}, nil
}

func (s *jasperService) ResetTags(ctx context.Context, id *JasperProcessID) (*OperationOutcome, error) {
	proc, err := s.manager.Get(ctx, id.Value)
	if err != nil {
		err = errors.Wrapf(err, "problem finding process '%s'", id.Value)
		return &OperationOutcome{
			ExitCode: -1,
			Success:  false,
			Text:     err.Error(),
		}, err
	}
	proc.ResetTags()
	return &OperationOutcome{Success: true, Text: "set tags", ExitCode: 0}, nil
}

func (s *jasperService) DownloadMongoDB(ctx context.Context, opts *MongoDBDownloadOptions) (*OperationOutcome, error) {
	jopts := opts.Export()
	if err := jopts.Validate(); err != nil {
		return &OperationOutcome{
			Success:  false,
			Text:     errors.Wrap(err, "problem validating MongoDB download options").Error(),
			ExitCode: -2,
		}, nil
	}

	if err := jasper.SetupDownloadMongoDBReleases(ctx, s.cache, jopts); err != nil {
		err = errors.Wrap(err, "problem in download setup")
		return &OperationOutcome{Success: false, Text: err.Error(), ExitCode: -3}, nil
	}

	return &OperationOutcome{Success: true, Text: "download jobs started"}, nil
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
		err = errors.Wrap(err, "problem validating download options")
		return &OperationOutcome{Success: false, Text: err.Error(), ExitCode: -2}, nil
	}

	if err := jopts.Download(); err != nil {
		err = errors.Wrapf(err, "problem occurred during file download for URL %s to path %s", jopts.URL, jopts.Path)
		return &OperationOutcome{Success: false, Text: err.Error(), ExitCode: -3}, nil
	}

	return &OperationOutcome{
		Success: true,
		Text:    fmt.Sprintf("downloaded file %s to path %s", jopts.URL, jopts.Path),
	}, nil
}

func (s *jasperService) GetLogStream(ctx context.Context, request *LogRequest) (*LogStream, error) {
	id := request.Id
	proc, err := s.manager.Get(ctx, id.Value)
	if err != nil {
		return nil, errors.Wrapf(err, "problem finding process '%s'", id.Value)
	}

	stream := &LogStream{}
	stream.Logs, err = jasper.GetInMemoryLogStream(ctx, proc, int(request.Count))
	if err == io.EOF {
		stream.Done = true
	} else if err != nil {
		return nil, errors.Wrapf(err, "could not get logs for process '%s'", request.Id.Value)
	}
	return stream, nil
}

func (s *jasperService) GetBuildloggerURLs(ctx context.Context, id *JasperProcessID) (*BuildloggerURLs, error) {
	proc, err := s.manager.Get(ctx, id.Value)
	if err != nil {
		err = errors.Wrapf(err, "problem finding process '%s'", id.Value)
		return nil, err
	}

	urls := []string{}
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
		return nil, errors.Errorf("process '%s' does not use buildlogger", id.Value)
	}

	return &BuildloggerURLs{Urls: urls}, nil
}

func (s *jasperService) RegisterSignalTriggerID(ctx context.Context, params *SignalTriggerParams) (*OperationOutcome, error) {
	jasperProcessID, signalTriggerID := params.Export()

	proc, err := s.manager.Get(ctx, jasperProcessID)
	if err != nil {
		err = errors.Wrapf(err, "problem finding process '%s'", jasperProcessID)
		return &OperationOutcome{
			Success:  false,
			Text:     err.Error(),
			ExitCode: -2,
		}, nil
	}

	makeTrigger, ok := jasper.GetSignalTriggerFactory(signalTriggerID)
	if !ok {
		return &OperationOutcome{
			Success:  false,
			Text:     errors.Errorf("could not find signal trigger with id '%s'", signalTriggerID).Error(),
			ExitCode: -3,
		}, nil
	}

	if err := proc.RegisterSignalTrigger(ctx, makeTrigger()); err != nil {
		err = errors.Wrapf(err, "problem registering signal trigger '%s'", signalTriggerID)
		return &OperationOutcome{
			Success:  false,
			Text:     err.Error(),
			ExitCode: -4,
		}, nil
	}

	return &OperationOutcome{
		Success:  true,
		Text:     fmt.Sprintf("registered signal trigger with id '%s' on process with id '%s'", signalTriggerID, jasperProcessID),
		ExitCode: 0,
	}, nil
}

func (s *jasperService) SignalEvent(ctx context.Context, name *EventName) (*OperationOutcome, error) {
	eventName := name.Value

	if err := jasper.SignalEvent(ctx, eventName); err != nil {
		return &OperationOutcome{
			Success:  false,
			Text:     errors.Wrapf(err, "problem signaling event '%s'", eventName).Error(),
			ExitCode: -2,
		}, nil
	}

	return &OperationOutcome{
		Success:  true,
		Text:     fmt.Sprintf("signaled event named '%s'", eventName),
		ExitCode: 0,
	}, nil
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
				Text:     errors.Wrap(err, "error receiving from client stream").Error(),
				ExitCode: -2,
			}); sendErr != nil {
				return errors.Wrapf(sendErr, "could not send error response to client: %s", err.Error())
			}
			return nil
		}

		jopts = opts.Export()

		if err := jopts.Validate(); err != nil {
			if sendErr := stream.SendAndClose(&OperationOutcome{
				Success:  false,
				Text:     errors.Wrap(err, "problem validating file write options").Error(),
				ExitCode: -3,
			}); sendErr != nil {
				return errors.Wrapf(sendErr, "could not send error response to client: %s", err.Error())
			}
			return nil
		}

		if err := jopts.DoWrite(); err != nil {
			if sendErr := stream.SendAndClose(&OperationOutcome{
				Success:  false,
				Text:     errors.Wrap(err, "problem validating file write opts").Error(),
				ExitCode: -4,
			}); sendErr != nil {
				return errors.Wrapf(sendErr, "could not send error response to client: %s", err.Error())
			}
			return nil
		}
	}

	if err := jopts.SetPerm(); err != nil {
		if sendErr := stream.SendAndClose(&OperationOutcome{
			Success:  false,
			Text:     errors.Wrapf(err, "problem setting permissions for file %s", jopts.Path).Error(),
			ExitCode: -5,
		}); sendErr != nil {
			return errors.Wrapf(sendErr, "could not send error response to client: %s", err.Error())
		}
		return nil
	}

	return errors.Wrap(stream.SendAndClose(&OperationOutcome{
		Success: true,
		Text:    fmt.Sprintf("file %s successfully written", jopts.Path),
	}), "could not send success response to client")
}

func (s *jasperService) ScriptingHarnessCreate(ctx context.Context, opts *ScriptingOptions) (*ScriptingHarnessID, error) {
	xopts, err := opts.Export()
	if err != nil {
		return nil, errors.Wrap(err, "problem converting options")
	}

	se, err := s.scripting.Create(s.manager, xopts)
	if err != nil {
		return nil, errors.Wrap(err, "problem generating scripting environment")
	}
	return &ScriptingHarnessID{
		Id:    se.ID(),
		Setup: true,
	}, nil
}
func (s *jasperService) ScriptingHarnessCheck(ctx context.Context, id *ScriptingHarnessID) (*OperationOutcome, error) {
	se, err := s.scripting.Get(id.Id)
	if err != nil {
		return &OperationOutcome{
			Success:  false,
			Text:     err.Error(),
			ExitCode: -1,
		}, nil
	}

	return &OperationOutcome{
		Success:  true,
		Text:     se.ID(),
		ExitCode: 0,
	}, nil
}

func (s *jasperService) ScriptingHarnessSetup(ctx context.Context, id *ScriptingHarnessID) (*OperationOutcome, error) {
	se, err := s.scripting.Get(id.Id)
	if err != nil {
		return &OperationOutcome{
			Success:  false,
			Text:     err.Error(),
			ExitCode: -1,
		}, nil
	}

	err = se.Setup(ctx)

	if err != nil {
		return &OperationOutcome{
			Success:  false,
			Text:     err.Error(),
			ExitCode: -2,
		}, nil
	}

	return &OperationOutcome{
		Success:  true,
		Text:     se.ID(),
		ExitCode: 0,
	}, nil
}

func (s *jasperService) ScriptingHarnessCleanup(ctx context.Context, id *ScriptingHarnessID) (*OperationOutcome, error) {
	se, err := s.scripting.Get(id.Id)
	if err != nil {
		return &OperationOutcome{
			Success:  false,
			Text:     err.Error(),
			ExitCode: -1,
		}, nil
	}

	err = se.Cleanup(ctx)

	if err != nil {
		return &OperationOutcome{
			Success:  false,
			Text:     err.Error(),
			ExitCode: -2,
		}, nil
	}

	return &OperationOutcome{
		Success:  true,
		Text:     se.ID(),
		ExitCode: 0,
	}, nil
}

func (s *jasperService) ScriptingHarnessRun(ctx context.Context, args *ScriptingHarnessRunArgs) (*OperationOutcome, error) {
	se, err := s.scripting.Get(args.Id)
	if err != nil {
		return &OperationOutcome{
			Success:  false,
			Text:     err.Error(),
			ExitCode: -1,
		}, nil
	}

	err = se.Run(ctx, args.Args)
	if err != nil {
		return &OperationOutcome{
			Success:  false,
			Text:     err.Error(),
			ExitCode: -2,
		}, nil
	}

	return &OperationOutcome{
		Success:  true,
		Text:     se.ID(),
		ExitCode: 0,
	}, nil
}

func (s *jasperService) ScriptingHarnessRunScript(ctx context.Context, args *ScriptingHarnessRunScriptArgs) (*OperationOutcome, error) {
	se, err := s.scripting.Get(args.Id)
	if err != nil {
		return &OperationOutcome{
			Success:  false,
			Text:     err.Error(),
			ExitCode: -1,
		}, nil
	}

	err = se.RunScript(ctx, args.Script)
	if err != nil {
		return &OperationOutcome{
			Success:  false,
			Text:     err.Error(),
			ExitCode: -2,
		}, nil
	}

	return &OperationOutcome{
		Success:  true,
		Text:     se.ID(),
		ExitCode: 0,
	}, nil
}

func (s *jasperService) ScriptingHarnessBuild(ctx context.Context, args *ScriptingHarnessBuildArgs) (*ScriptingHarnessBuildResponse, error) {
	se, err := s.scripting.Get(args.Id)
	if err != nil {
		return &ScriptingHarnessBuildResponse{
			Outcome: &OperationOutcome{
				Success:  false,
				Text:     err.Error(),
				ExitCode: -1,
			}}, nil
	}

	path, err := se.Build(ctx, args.Directory, args.Args)
	if err != nil {
		return &ScriptingHarnessBuildResponse{
			Path: path,
			Outcome: &OperationOutcome{
				Success:  false,
				Text:     err.Error(),
				ExitCode: -2,
			}}, nil
	}

	return &ScriptingHarnessBuildResponse{
		Path: path,
		Outcome: &OperationOutcome{
			Success:  true,
			Text:     se.ID(),
			ExitCode: 0,
		}}, nil
}

func (s *jasperService) ScriptingHarnessTest(ctx context.Context, args *ScriptingHarnessTestArgs) (*ScriptingHarnessTestResponse, error) {
	se, err := s.scripting.Get(args.Id)
	if err != nil {
		return &ScriptingHarnessTestResponse{
			Outcome: &OperationOutcome{
				Success:  false,
				Text:     err.Error(),
				ExitCode: -1,
			},
		}, nil
	}

	exportedArgs, err := args.Export()
	if err != nil {
		return &ScriptingHarnessTestResponse{
			Outcome: &OperationOutcome{
				Success:  false,
				Text:     err.Error(),
				ExitCode: -2,
			},
		}, nil
	}

	res, err := se.Test(ctx, args.Directory, exportedArgs...)
	if err != nil {
		return &ScriptingHarnessTestResponse{
			Outcome: &OperationOutcome{
				Success:  false,
				Text:     err.Error(),
				ExitCode: -3,
			},
		}, nil
	}
	convertedRes, err := ConvertScriptingTestResults(res)
	if err != nil {
		return &ScriptingHarnessTestResponse{
			Outcome: &OperationOutcome{
				Success:  false,
				Text:     err.Error(),
				ExitCode: -4,
			},
		}, nil
	}

	return &ScriptingHarnessTestResponse{
		Outcome: &OperationOutcome{
			Success:  true,
			ExitCode: 0,
		},
		Results: convertedRes,
	}, nil
}

func (s *jasperService) LoggingCacheCreate(ctx context.Context, args *LoggingCacheCreateArgs) (*LoggingCacheInstance, error) {
	lc := s.manager.LoggingCache(ctx)
	if lc == nil {
		return nil, errors.New("logging cache not supported")
	}
	opt, err := args.Options.Export()
	if err != nil {
		return nil, errors.Wrap(err, "problem exporting output options")
	}

	out, err := lc.Create(args.Name, &opt)
	if err != nil {
		return &LoggingCacheInstance{
			Outcome: &OperationOutcome{
				Success:  false,
				Text:     err.Error(),
				ExitCode: -1,
			},
		}, nil
	}

	logger, err := ConvertCachedLogger(out)
	if err != nil {
		return &LoggingCacheInstance{
			Outcome: &OperationOutcome{
				Success:  false,
				Text:     err.Error(),
				ExitCode: -2,
			},
		}, nil
	}
	return logger, nil
}

func (s *jasperService) LoggingCacheGet(ctx context.Context, args *LoggingCacheArgs) (*LoggingCacheInstance, error) {
	lc := s.manager.LoggingCache(ctx)
	if lc == nil {
		return nil, errors.New("logging cache not supported")
	}

	out := lc.Get(args.Name)
	if out == nil {
		return &LoggingCacheInstance{
			Outcome: &OperationOutcome{
				Success:  false,
				Text:     "not found",
				ExitCode: -1,
			},
		}, nil
	}

	logger, err := ConvertCachedLogger(out)
	if err != nil {
		return &LoggingCacheInstance{
			Outcome: &OperationOutcome{
				Success:  false,
				Text:     err.Error(),
				ExitCode: -2,
			},
		}, nil
	}
	return logger, nil
}

func (s *jasperService) LoggingCacheRemove(ctx context.Context, args *LoggingCacheArgs) (*OperationOutcome, error) {
	lc := s.manager.LoggingCache(ctx)
	if lc == nil {
		return &OperationOutcome{
			Success:  false,
			Text:     "logging cache is not supported",
			ExitCode: -1,
		}, nil
	}

	lc.Remove(args.Name)

	return &OperationOutcome{
		Success: true,
	}, nil
}

func (s *jasperService) LoggingCachePrune(ctx context.Context, arg *timestamp.Timestamp) (*OperationOutcome, error) {
	lc := s.manager.LoggingCache(ctx)
	if lc == nil {
		return nil, errors.New("logging cache not supported")
	}

	ts, err := ptypes.Timestamp(arg)
	if err != nil {
		return nil, errors.Wrap(err, "could not convert prune timestamp to equivalent protobuf RPC timestamp")
	}
	lc.Prune(ts)

	return &OperationOutcome{
		Success: true,
	}, nil
}

func (s *jasperService) LoggingCacheLen(ctx context.Context, _ *empty.Empty) (*LoggingCacheSize, error) {
	lc := s.manager.LoggingCache(ctx)
	if lc == nil {
		return &LoggingCacheSize{
			Outcome: &OperationOutcome{
				Success:  false,
				Text:     "logging cache is not supported",
				ExitCode: -1,
			},
		}, nil
	}

	return &LoggingCacheSize{
		Outcome: &OperationOutcome{
			Success:  true,
			ExitCode: 0,
		},
		Id:   s.manager.ID(),
		Size: int64(lc.Len()),
	}, nil
}

func (s *jasperService) SendMessages(ctx context.Context, lp *LoggingPayload) (*OperationOutcome, error) {
	lc := s.manager.LoggingCache(ctx)
	if lc == nil {
		return &OperationOutcome{
			Success:  false,
			Text:     "logging cache is not supported",
			ExitCode: -1,
		}, nil
	}

	logger := lc.Get(lp.LoggerID)
	if logger == nil {
		return &OperationOutcome{
			Success:  false,
			Text:     fmt.Sprintf("logging instance '%s' does not exist", lp.LoggerID),
			ExitCode: -2,
		}, nil
	}

	payload := lp.Export()
	if err := logger.Send(payload); err != nil {
		return &OperationOutcome{
			Success:  false,
			Text:     err.Error(),
			ExitCode: -3,
		}, nil
	}

	return &OperationOutcome{Success: true}, nil
}
