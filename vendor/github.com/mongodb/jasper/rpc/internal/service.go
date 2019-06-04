package internal

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	empty "github.com/golang/protobuf/ptypes/empty"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	"github.com/tychoish/lru"
	context "golang.org/x/net/context"
	grpc "google.golang.org/grpc"
)

// AttachService attaches the given manager to the jasper GRPC server. This
// function eventually calls generated Protobuf code for registering the
// GRPC Jasper server with the given Manager.
func AttachService(manager jasper.Manager, s *grpc.Server) error {
	hn, err := os.Hostname()
	if err != nil {
		return errors.WithStack(err)
	}

	srv := &jasperService{
		hostID:  hn,
		manager: manager,
		cache:   lru.NewCache(),
		cacheOpts: jasper.CacheOptions{
			PruneDelay: jasper.DefaultCachePruneDelay,
			MaxSize:    jasper.DefaultMaxCacheSize,
		},
	}

	RegisterJasperProcessManagerServer(s, srv)

	go srv.backgroundPrune()

	return nil
}

func (s *jasperService) backgroundPrune() {
	s.cacheMutex.RLock()
	timer := time.NewTimer(s.cacheOpts.PruneDelay)
	s.cacheMutex.RUnlock()

	for {
		<-timer.C
		s.cacheMutex.RLock()
		if !s.cacheOpts.Disabled {
			if err := s.cache.Prune(s.cacheOpts.MaxSize, nil, false); err != nil {
				grip.Error(errors.Wrap(err, "error during cache pruning"))
			}
		}
		timer.Reset(s.cacheOpts.PruneDelay)
		s.cacheMutex.RUnlock()
	}
}

func getProcInfoNoHang(ctx context.Context, p jasper.Process) *ProcessInfo {
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	return ConvertProcessInfo(p.Info(ctx))
}

type jasperService struct {
	hostID     string
	manager    jasper.Manager
	client     http.Client
	cache      *lru.Cache
	cacheOpts  jasper.CacheOptions
	cacheMutex sync.RWMutex
}

func (s *jasperService) Status(ctx context.Context, _ *empty.Empty) (*StatusResponse, error) {
	return &StatusResponse{
		HostId: s.hostID,
		Active: true,
	}, nil
}

func (s *jasperService) Create(ctx context.Context, opts *CreateOptions) (*ProcessInfo, error) {
	jopts := opts.Export()

	// Spawn a new context so that the process' context is not potentially
	// canceled by the request's. See how rest_service.go's createProcess() does
	// this same thing.
	var cctx context.Context
	var cancel context.CancelFunc
	if jopts.Timeout > 0 {
		cctx, cancel = context.WithTimeout(context.Background(), jopts.Timeout)
	} else {
		cctx, cancel = context.WithCancel(context.Background())
	}

	proc, err := s.manager.CreateProcess(cctx, jopts)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if err := proc.RegisterTrigger(cctx, func(_ jasper.ProcessInfo) {
		cancel()
	}); err != nil {
		if !proc.Info(cctx).Complete {
			return ConvertProcessInfo(proc.Info(cctx)), nil
		}
		cancel()
	}

	return getProcInfoNoHang(cctx, proc), nil
}

func (s *jasperService) List(f *Filter, stream JasperProcessManager_ListServer) error {
	ctx := stream.Context()
	procs, err := s.manager.List(ctx, jasper.Filter(strings.ToLower(f.GetName().String())))
	if err != nil {
		return errors.WithStack(err)
	}

	for _, p := range procs {
		if ctx.Err() != nil {
			return errors.New("operation canceled")
		}

		if err := stream.Send(getProcInfoNoHang(ctx, p)); err != nil {
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
			return errors.New("operation canceled")
		}

		if err := stream.Send(getProcInfoNoHang(ctx, p)); err != nil {
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

	return getProcInfoNoHang(ctx, proc), nil
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
	if err != nil && exitCode == -1 {
		err = errors.Wrap(err, "problem encountered while waiting")
		return &OperationOutcome{
			Success:  false,
			Text:     err.Error(),
			ExitCode: int32(exitCode),
		}, nil
	}

	return &OperationOutcome{
		Success:  true,
		Text:     fmt.Sprintf("'%s' operation complete", id.Value),
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
	cctx, cancel := context.WithCancel(context.Background())
	newProc, err := proc.Respawn(cctx)
	if err != nil {
		err = errors.Wrap(err, "problem encountered while respawning")
		cancel()
		return nil, errors.WithStack(err)
	}
	_ = s.manager.Register(ctx, newProc)

	if err := newProc.RegisterTrigger(ctx, func(_ jasper.ProcessInfo) {
		cancel()
	}); err != nil {
		newProcInfo := getProcInfoNoHang(ctx, newProc)
		if !newProcInfo.Complete {
			return newProcInfo, nil
		}
		cancel()
	}

	return getProcInfoNoHang(ctx, newProc), nil
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
			ExitCode: 1,
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

func (s *jasperService) DownloadFile(ctx context.Context, info *DownloadInfo) (*OperationOutcome, error) {
	jinfo := info.Export()

	if err := jinfo.Validate(); err != nil {
		err = errors.Wrap(err, "problem validating download info")
		return &OperationOutcome{Success: false, Text: err.Error(), ExitCode: -2}, nil
	}

	if err := jinfo.Download(); err != nil {
		err = errors.Wrapf(err, "problem occurred during file download for URL %s to path %s", jinfo.URL, jinfo.Path)
		return &OperationOutcome{Success: false, Text: err.Error(), ExitCode: -3}, nil
	}

	return &OperationOutcome{
		Success:  true,
		Text:     fmt.Sprintf("downloaded file %s to path %s", jinfo.URL, jinfo.Path),
		ExitCode: 0,
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
	for _, logger := range getProcInfoNoHang(ctx, proc).Export().Options.Output.Loggers {
		if logger.Type == jasper.LogBuildloggerV2 || logger.Type == jasper.LogBuildloggerV3 {
			urls = append(urls, logger.Options.BuildloggerOptions.GetGlobalLogURL())
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
			ExitCode: -2}, nil
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
