package remote

import (
	"context"
	"io"
	"time"

	"github.com/evergreen-ci/mrpc/mongowire"
	"github.com/evergreen-ci/mrpc/shell"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// Constants representing manager commands.
const (
	ManagerIDCommand     = "id"
	CreateProcessCommand = "create_process"
	GetProcessCommand    = "get_process"
	ListCommand          = "list"
	GroupCommand         = "group"
	ClearCommand         = "clear"
	CloseCommand         = "close"
	WriteFileCommand     = "write_file"
)

func (s *mdbService) managerID(ctx context.Context, w io.Writer, msg mongowire.Message) {
	resp, err := shell.ResponseToMessage(mongowire.OP_REPLY, makeIDResponse(s.manager.ID()))
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.New("could not make response"), ManagerIDCommand)
		return
	}
	shell.WriteResponse(ctx, w, resp, ManagerIDCommand)
}

func (s *mdbService) managerCreateProcess(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := createProcessRequest{}
	if err := shell.MessageToRequest(msg, &req); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not read request"), CreateProcessCommand)
		return
	}
	opts := req.Options

	// Spawn a new context so that the process' context is not potentially
	// canceled by the request's. See how rest_service.go's createProcess() does
	// this same thing.
	pctx, cancel := context.WithCancel(context.Background())

	proc, err := s.manager.CreateProcess(pctx, &opts)
	if err != nil {
		cancel()
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not create process"), CreateProcessCommand)
		return
	}

	if err = proc.RegisterTrigger(ctx, func(_ jasper.ProcessInfo) {
		cancel()
	}); err != nil {
		info := getProcInfoNoHang(ctx, proc)
		cancel()
		// If we get an error registering a trigger, then we should make sure that
		// the reason for it isn't just because the process has exited already,
		// since that should not be considered an error.
		if !info.Complete {
			shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not register trigger"), CreateProcessCommand)
			return
		}
	}

	resp, err := shell.ResponseToMessage(mongowire.OP_REPLY, makeInfoResponse(getProcInfoNoHang(ctx, proc)))
	if err != nil {
		cancel()
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not make response"), CreateProcessCommand)
		return
	}
	shell.WriteResponse(ctx, w, resp, CreateProcessCommand)
}

func (s *mdbService) managerList(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := listRequest{}
	if err := shell.MessageToRequest(msg, &req); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not read request"), ListCommand)
		return
	}
	filter := req.Filter

	procs, err := s.manager.List(ctx, filter)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not list processes"), ListCommand)
		return
	}

	infos := make([]jasper.ProcessInfo, 0, len(procs))
	for _, proc := range procs {
		infos = append(infos, proc.Info(ctx))
	}
	resp, err := shell.ResponseToMessage(mongowire.OP_REPLY, makeInfosResponse(infos))
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not make response"), ListCommand)
		return
	}
	shell.WriteResponse(ctx, w, resp, ListCommand)
}

func (s *mdbService) managerGroup(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := groupRequest{}
	if err := shell.MessageToRequest(msg, &req); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not read request"), GroupCommand)
		return
	}
	tag := req.Tag

	procs, err := s.manager.Group(ctx, tag)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not get process group"), GroupCommand)
		return
	}

	infos := make([]jasper.ProcessInfo, 0, len(procs))
	for _, proc := range procs {
		infos = append(infos, proc.Info(ctx))
	}

	resp, err := shell.ResponseToMessage(mongowire.OP_REPLY, makeInfosResponse(infos))
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not make response"), GroupCommand)
		return
	}
	shell.WriteResponse(ctx, w, resp, GroupCommand)
}

func (s *mdbService) managerGetProcess(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := getProcessRequest{}
	if err := shell.MessageToRequest(msg, &req); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not read request"), GetProcessCommand)
		return
	}
	id := req.ID

	proc, err := s.manager.Get(ctx, id)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not get process"), GetProcessCommand)
		return
	}

	resp, err := shell.ResponseToMessage(mongowire.OP_REPLY, makeInfoResponse(proc.Info(ctx)))
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not make response"), GetProcessCommand)
		return
	}
	shell.WriteResponse(ctx, w, resp, GetProcessCommand)
}

func (s *mdbService) managerClear(ctx context.Context, w io.Writer, msg mongowire.Message) {
	s.manager.Clear(ctx)
	shell.WriteOKResponse(ctx, w, mongowire.OP_REPLY, ClearCommand)
}

func (s *mdbService) managerClose(ctx context.Context, w io.Writer, msg mongowire.Message) {
	if err := s.manager.Close(ctx); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, err, CloseCommand)
		return
	}
	shell.WriteOKResponse(ctx, w, mongowire.OP_REPLY, CloseCommand)
}

func (s *mdbService) managerWriteFile(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := &writeFileRequest{}
	if err := shell.MessageToRequest(msg, &req); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "could not read request"), WriteFileCommand)
		return
	}
	opts := req.Options

	if err := opts.Validate(); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "invalid write file options"), WriteFileCommand)
		return
	}
	if err := opts.DoWrite(); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "failed to write to file"), WriteFileCommand)
		return
	}

	shell.WriteOKResponse(ctx, w, mongowire.OP_REPLY, WriteFileCommand)
}

// Constants representing remote client commands.
const (
	ConfigureCacheCommand     = "configure_cache"
	DownloadFileCommand       = "download_file"
	DownloadMongoDBCommand    = "download_mongodb"
	GetLogStreamCommand       = "get_log_stream"
	GetBuildloggerURLsCommand = "get_buildlogger_urls"
	SignalEventCommand        = "signal_event"
	SendMessagesCommand       = "send_messages"
)

func (s *mdbService) configureCache(ctx context.Context, w io.Writer, msg mongowire.Message) {
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

func (s *mdbService) downloadFile(ctx context.Context, w io.Writer, msg mongowire.Message) {
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

func (s *mdbService) downloadMongoDB(ctx context.Context, w io.Writer, msg mongowire.Message) {
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

func (s *mdbService) getLogStream(ctx context.Context, w io.Writer, msg mongowire.Message) {
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

func (s *mdbService) getBuildloggerURLs(ctx context.Context, w io.Writer, msg mongowire.Message) {
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

func (s *mdbService) signalEvent(ctx context.Context, w io.Writer, msg mongowire.Message) {
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

func (s *mdbService) sendMessages(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := &sendMessagesRequest{}
	lc := s.loggingCacheRequest(ctx, w, msg, req, SendMessagesCommand)
	if lc == nil {
		return
	}

	if err := req.Payload.Validate(); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "invalid logging payload"), SendMessagesCommand)
		return
	}

	cachedLogger := lc.Get(req.Payload.LoggerID)
	if cachedLogger == nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.New("named logger does not exist"), SendMessagesCommand)
		return
	}
	if err := cachedLogger.Send(&req.Payload); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "problem sending message"), SendMessagesCommand)
		return
	}

	s.loggingCacheResponse(ctx, w, nil, SendMessagesCommand)
}

func (s *mdbService) scriptingGet(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := &scriptingGetRequest{}
	if !s.serviceScriptingRequest(ctx, w, msg, req, ScriptingGetCommand) {
		return
	}

	harness := s.getHarness(ctx, w, req.ID, ScriptingGetCommand)
	if harness == nil {
		return
	}

	s.serviceScriptingResponse(ctx, w, nil, ScriptingGetCommand)
}

func (s *mdbService) scriptingCreate(ctx context.Context, w io.Writer, msg mongowire.Message) {
	req := &scriptingCreateRequest{}
	if !s.serviceScriptingRequest(ctx, w, msg, req, ScriptingCreateCommand) {
		return
	}

	opts, err := options.NewScriptingHarness(req.Params.Type)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "problem creating harness options"), ScriptingCreateCommand)
		return
	}
	if err = bson.Unmarshal(req.Params.Options, opts); err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "problem unmarshalling options"), ScriptingCreateCommand)
		return
	}

	harness, err := s.harnessCache.Create(s.manager, opts)
	if err != nil {
		shell.WriteErrorResponse(ctx, w, mongowire.OP_REPLY, errors.Wrap(err, "problem creating harness"), ScriptingCreateCommand)
		return
	}

	s.serviceScriptingResponse(ctx, w, makeScriptingCreateResponse(harness.ID()), ScriptingCreateCommand)
}
