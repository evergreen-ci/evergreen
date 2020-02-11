package remote

import (
	"context"
	"io"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/evergreen-ci/mrpc"
	"github.com/evergreen-ci/mrpc/mongowire"
	"github.com/evergreen-ci/mrpc/shell"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/util"
	"github.com/pkg/errors"
	"github.com/tychoish/lru"
)

type service struct {
	mrpc.Service
	manager    jasper.Manager
	cache      *lru.Cache
	cacheOpts  options.Cache
	cacheMutex sync.RWMutex
}

// StartMDBService wraps an existing Jasper manager in a MongoDB wire protocol
// service and starts it. The caller is responsible for closing the connection
// using the returned jasper.CloseFunc.
func StartMDBService(ctx context.Context, m jasper.Manager, addr net.Addr) (util.CloseFunc, error) { //nolint: interfacer
	host, p, err := net.SplitHostPort(addr.String())
	if err != nil {
		return nil, errors.Wrap(err, "invalid address")
	}
	port, err := strconv.Atoi(p)
	if err != nil {
		return nil, errors.Wrap(err, "port is not a number")
	}

	baseSvc, err := shell.NewShellService(host, port)
	if err != nil {
		return nil, errors.Wrap(err, "could not create base service")
	}
	svc := &service{
		Service: baseSvc,
		manager: m,
		cache:   lru.NewCache(),
		cacheOpts: options.Cache{
			PruneDelay: jasper.DefaultCachePruneDelay,
			MaxSize:    jasper.DefaultMaxCacheSize,
		},
	}
	if err := svc.registerHandlers(); err != nil {
		return nil, errors.Wrap(err, "error registering handlers")
	}

	cctx, ccancel := context.WithCancel(context.Background())
	go func() {
		defer func() {
			grip.Error(recovery.HandlePanicWithError(recover(), nil, "running wire service"))
		}()
		grip.Notice(svc.Run(cctx))
	}()

	go svc.pruneCache(cctx)

	return func() error { ccancel(); return nil }, nil
}

func (s *service) registerHandlers() error {
	for name, handler := range map[string]mrpc.HandlerFunc{
		// Manager commands
		ManagerIDCommand:     s.managerID,
		CreateProcessCommand: s.managerCreateProcess,
		ListCommand:          s.managerList,
		GroupCommand:         s.managerGroup,
		GetProcessCommand:    s.managerGetProcess,
		ClearCommand:         s.managerClear,
		CloseCommand:         s.managerClose,
		WriteFileCommand:     s.managerWriteFile,

		// Process commands
		InfoCommand:                    s.processInfo,
		RunningCommand:                 s.processRunning,
		CompleteCommand:                s.processComplete,
		WaitCommand:                    s.processWait,
		SignalCommand:                  s.processSignal,
		RegisterSignalTriggerIDCommand: s.processRegisterSignalTriggerID,
		RespawnCommand:                 s.processRespawn,
		TagCommand:                     s.processTag,
		GetTagsCommand:                 s.processGetTags,
		ResetTagsCommand:               s.processResetTags,

		// Remote client commands
		ConfigureCacheCommand:     s.configureCache,
		DownloadFileCommand:       s.downloadFile,
		DownloadMongoDBCommand:    s.downloadMongoDB,
		GetLogStreamCommand:       s.getLogStream,
		GetBuildloggerURLsCommand: s.getBuildloggerURLs,
		SignalEventCommand:        s.signalEvent,
	} {
		if err := s.RegisterOperation(&mongowire.OpScope{
			Type:    mongowire.OP_COMMAND,
			Command: name,
		}, handler); err != nil {
			return errors.Wrapf(err, "could not register handler for %s", name)
		}
	}

	return nil
}

func (s *service) pruneCache(ctx context.Context) {
	defer func() {
		err := recovery.HandlePanicWithError(recover(), nil, "pruning cache")
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
				grip.Error(message.WrapError(s.cache.Prune(s.cacheOpts.MaxSize, nil, false), "error during cache pruning"))
			}
			timer.Reset(s.cacheOpts.PruneDelay)
			s.cacheMutex.RUnlock()
		case <-ctx.Done():
			return
		}
	}
}

// Constants representing remote client commands.
const (
	ConfigureCacheCommand     = "configure_cache"
	DownloadFileCommand       = "download_file"
	DownloadMongoDBCommand    = "download_mongodb"
	GetLogStreamCommand       = "get_log_stream"
	GetBuildloggerURLsCommand = "get_buildlogger_urls"
	SignalEventCommand        = "signal_event"
)

func (s *service) configureCache(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := configureCacheRequest{}
	if err := shell.MessageToRequest(msg, &req); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not read request"), ConfigureCacheCommand)
		return
	}
	opts := req.Options
	if err := opts.Validate(); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "invalid cache options"), ConfigureCacheCommand)
		return
	}

	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()
	if opts.MaxSize > 0 {
		s.cacheOpts.MaxSize = opts.MaxSize
	}
	if opts.PruneDelay > time.Duration(0) {
		s.cacheOpts.PruneDelay = opts.PruneDelay
	}
	s.cacheOpts.Disabled = opts.Disabled

	shell.WriteOKResponse(ctx, w, mongowire.OP_REPLY, ConfigureCacheCommand)
}

func (s *service) downloadFile(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := downloadFileRequest{}
	if err := shell.MessageToRequest(msg, &req); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not read request"), DownloadFileCommand)
		return
	}
	opts := req.Options

	if err := opts.Validate(); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "invalid download options"), DownloadFileCommand)
		return
	}

	if err := opts.Download(); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not download file"), DownloadFileCommand)
		return
	}

	shell.WriteOKResponse(ctx, w, mongowire.OP_REPLY, DownloadFileCommand)
}

func (s *service) downloadMongoDB(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := &downloadMongoDBRequest{}
	if err := shell.MessageToRequest(msg, &req); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not read request"), DownloadMongoDBCommand)
		return
	}
	opts := req.Options

	if err := opts.Validate(); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "invalid download options"), DownloadMongoDBCommand)
		return
	}

	if err := jasper.SetupDownloadMongoDBReleases(ctx, s.cache, opts); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "problem setting up download"), DownloadMongoDBCommand)
		return
	}

	shell.WriteOKResponse(ctx, w, mongowire.OP_REPLY, DownloadMongoDBCommand)
}

func (s *service) getLogStream(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := getLogStreamRequest{}
	if err := shell.MessageToRequest(msg, &req); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not read request"), DownloadMongoDBCommand)
		return
	}
	id := req.Params.ID
	count := req.Params.Count

	proc, err := s.manager.Get(ctx, id)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not get process"), GetLogStreamCommand)
		return
	}

	var done bool
	logs, err := jasper.GetInMemoryLogStream(ctx, proc, count)
	if err == io.EOF {
		done = true
	} else if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not get logs"), GetLogStreamCommand)
		return
	}

	resp, err := shell.ResponseToMessage(mongowire.OP_REPLY, makeGetLogStreamResponse(logs, done))
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not make response"), GetLogStreamCommand)
		return
	}

	shell.WriteResponse(ctx, w, resp, GetLogStreamCommand)
}

func (s *service) getBuildloggerURLs(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := &getBuildloggerURLsRequest{}
	if err := shell.MessageToRequest(msg, &req); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not read request"), GetBuildloggerURLsCommand)
		return
	}
	id := req.ID

	proc, err := s.manager.Get(ctx, id)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not get process"), GetBuildloggerURLsCommand)
		return
	}

	urls := []string{}
	for _, logger := range getProcInfoNoHang(ctx, proc).Options.Output.Loggers {
		if logger.Type == options.LogBuildloggerV2 || logger.Type == options.LogBuildloggerV3 {
			urls = append(urls, logger.Options.BuildloggerOptions.GetGlobalLogURL())
		}
	}
	if len(urls) == 0 {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Errorf("process '%s' does not use buildlogger", proc.ID()), GetBuildloggerURLsCommand)
		return
	}

	resp, err := shell.ResponseToMessage(mongowire.OP_REPLY, urls)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not make response"), GetBuildloggerURLsCommand)
		return
	}
	shell.WriteResponse(ctx, w, resp, GetBuildloggerURLsCommand)
}

func (s *service) signalEvent(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := &signalEventRequest{}
	if err := shell.MessageToRequest(msg, &req); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not read request"), SignalEventCommand)
		return
	}
	name := req.Name

	if err := jasper.SignalEvent(ctx, name); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrapf(err, "could not signal event '%s'", name), SignalEventCommand)
		return
	}

	shell.WriteOKResponse(ctx, w, mongowire.OP_REPLY, SignalEventCommand)
}
