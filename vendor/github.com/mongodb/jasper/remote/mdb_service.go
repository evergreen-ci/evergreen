package remote

import (
	"context"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/evergreen-ci/lru"
	"github.com/evergreen-ci/mrpc"
	"github.com/evergreen-ci/mrpc/mongowire"
	"github.com/evergreen-ci/mrpc/shell"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/scripting"
	"github.com/mongodb/jasper/util"
	"github.com/pkg/errors"
)

type mdbService struct {
	mrpc.Service
	manager      jasper.Manager
	harnessCache scripting.HarnessCache
	cache        *lru.Cache
	cacheOpts    options.Cache
	cacheMutex   sync.RWMutex
}

// StartMDBService wraps an existing Jasper manager in a MongoDB wire protocol
// service and starts it. The caller is responsible for closing the connection
// using the returned jasper.CloseFunc.
func StartMDBService(ctx context.Context, m jasper.Manager, addr net.Addr) (util.CloseFunc, error) {
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
	svc := &mdbService{
		Service:      baseSvc,
		manager:      m,
		harnessCache: scripting.NewCache(),
		cache:        lru.NewCache(),
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

func (s *mdbService) registerHandlers() error {
	for name, handler := range map[string]mrpc.HandlerFunc{
		// jasper.Manager commands
		ManagerIDCommand:     s.managerID,
		CreateProcessCommand: s.managerCreateProcess,
		ListCommand:          s.managerList,
		GroupCommand:         s.managerGroup,
		GetProcessCommand:    s.managerGetProcess,
		ClearCommand:         s.managerClear,
		CloseCommand:         s.managerClose,
		WriteFileCommand:     s.managerWriteFile,

		// jasper.Process commands
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

		// Manager commands
		ConfigureCacheCommand:     s.configureCache,
		DownloadFileCommand:       s.downloadFile,
		DownloadMongoDBCommand:    s.downloadMongoDB,
		GetLogStreamCommand:       s.getLogStream,
		GetBuildloggerURLsCommand: s.getBuildloggerURLs,
		SignalEventCommand:        s.signalEvent,
		SendMessagesCommand:       s.sendMessages,
		ScriptingCreateCommand:    s.scriptingCreate,
		ScriptingGetCommand:       s.scriptingGet,

		// scripting.Harness commands
		ScriptingSetupCommand:     s.scriptingSetup,
		ScriptingCleanupCommand:   s.scriptingCleanup,
		ScriptingRunCommand:       s.scriptingRun,
		ScriptingRunScriptCommand: s.scriptingRunScript,
		ScriptingBuildCommand:     s.scriptingBuild,
		ScriptingTestCommand:      s.scriptingTest,

		// jasper.LoggingCache commands
		LoggingCacheCreateCommand:         s.loggingCacheCreate,
		LoggingCacheGetCommand:            s.loggingCacheGet,
		LoggingCacheRemoveCommand:         s.loggingCacheRemove,
		LoggingCacheCloseAndRemoveCommand: s.loggingCacheCloseAndRemove,
		LoggingCacheClearCommand:          s.loggingCacheClear,
		LoggingCachePruneCommand:          s.loggingCachePrune,
		LoggingCacheLenCommand:            s.loggingCacheLen,
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

func (s *mdbService) pruneCache(ctx context.Context) {
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
