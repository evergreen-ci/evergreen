package jasper

import (
	"context"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
	"github.com/tychoish/lru"
)

// Service defines a REST service that provides a remote manager, using
// gimlet to publish routes.
type Service struct {
	hostID     string
	manager    Manager
	cache      *lru.Cache
	cacheOpts  CacheOptions
	cacheMutex sync.RWMutex
}

// NewManagerService creates a service object around an existing
// manager. You must access the application and routes via the App()
// method separately. The constructor wraps basic managers with a
// manager implementation that does locking.
func NewManagerService(m Manager) *Service {
	if bpm, ok := m.(*basicProcessManager); ok {
		m = &localProcessManager{manager: bpm}
	}

	return &Service{
		manager: m,
	}
}

const (
	// DefaultCachePruneDelay is the duration between LRU cache prunes.
	DefaultCachePruneDelay = 10 * time.Second
	// DefaultMaxCacheSize is the maximum allowed size of the LRU cache.
	DefaultMaxCacheSize = 1024 * 1024 * 1024
)

// App constructs and returns a gimlet application for this
// service. It attaches no middleware and does not start the service.
func (s *Service) App(ctx context.Context) *gimlet.APIApp {
	s.hostID, _ = os.Hostname()
	s.cache = lru.NewCache()
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()
	s.cacheOpts.PruneDelay = DefaultCachePruneDelay
	s.cacheOpts.MaxSize = DefaultMaxCacheSize
	s.cacheOpts.Disabled = false

	app := gimlet.NewApp()

	app.AddRoute("/").Version(1).Get().Handler(s.rootRoute)
	app.AddRoute("/id").Version(1).Get().Handler(s.id)
	app.AddRoute("/create").Version(1).Post().Handler(s.createProcess)
	app.AddRoute("/download").Version(1).Post().Handler(s.downloadFile)
	app.AddRoute("/download/cache").Version(1).Post().Handler(s.configureCache)
	app.AddRoute("/download/mongodb").Version(1).Post().Handler(s.downloadMongoDB)
	app.AddRoute("/list/oom").Version(1).Get().Handler(s.oomTrackerList)
	app.AddRoute("/list/oom").Version(1).Delete().Handler(s.oomTrackerClear)
	app.AddRoute("/list/{filter}").Version(1).Get().Handler(s.listProcesses)
	app.AddRoute("/list/group/{name}").Version(1).Get().Handler(s.listGroupMembers)
	app.AddRoute("/process/{id}").Version(1).Get().Handler(s.getProcess)
	app.AddRoute("/process/{id}/tags").Version(1).Get().Handler(s.getProcessTags)
	app.AddRoute("/process/{id}/tags").Version(1).Delete().Handler(s.deleteProcessTags)
	app.AddRoute("/process/{id}/tags").Version(1).Post().Handler(s.addProcessTag)
	app.AddRoute("/process/{id}/wait").Version(1).Get().Handler(s.waitForProcess)
	app.AddRoute("/process/{id}/respawn").Version(1).Get().Handler(s.respawnProcess)
	app.AddRoute("/process/{id}/metrics").Version(1).Get().Handler(s.processMetrics)
	app.AddRoute("/process/{id}/logs/{count}").Version(1).Get().Handler(s.getLogStream)
	app.AddRoute("/process/{id}/loginfo").Version(1).Get().Handler(s.getBuildloggerURLs)
	app.AddRoute("/process/{id}/signal/{signal}").Version(1).Patch().Handler(s.signalProcess)
	app.AddRoute("/process/{id}/trigger/signal/{trigger-id}").Version(1).Patch().Handler(s.registerSignalTriggerID)
	app.AddRoute("/signal/event/{name}").Version(1).Patch().Handler(s.signalEvent)
	app.AddRoute("/clear").Version(1).Post().Handler(s.clearManager)
	app.AddRoute("/close").Version(1).Delete().Handler(s.closeManager)

	go s.backgroundPrune(ctx)

	return app
}

func (s *Service) SetDisableCachePruning(v bool) {
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()

	s.cacheOpts.Disabled = v
}

func (s *Service) SetCacheMaxSize(size int) {
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()

	s.cacheOpts.MaxSize = size
}

func (s *Service) SetPruneDelay(dur time.Duration) {
	s.cacheMutex.Lock()
	defer s.cacheMutex.Unlock()

	s.cacheOpts.PruneDelay = dur
}

func (s *Service) backgroundPrune(ctx context.Context) {
	defer func() {
		err := recovery.HandlePanicWithError(recover(), nil, "background pruning")
		if ctx.Err() != nil || err == nil {
			return
		}
		go s.backgroundPrune(ctx)
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

func getProcInfoNoHang(ctx context.Context, p Process) ProcessInfo {
	ctx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	return p.Info(ctx)
}

func writeError(rw http.ResponseWriter, err gimlet.ErrorResponse) {
	gimlet.WriteJSONResponse(rw, err.StatusCode, err)
}

func (s *Service) rootRoute(rw http.ResponseWriter, r *http.Request) {
	gimlet.WriteJSON(rw, struct {
		HostID string `json:"host_id"`
		Active bool   `json:"active"`
	}{
		HostID: s.hostID,
		Active: true,
	})
}

func (s *Service) id(rw http.ResponseWriter, r *http.Request) {
	gimlet.WriteJSON(rw, s.manager.ID())
}

func (s *Service) createProcess(rw http.ResponseWriter, r *http.Request) {
	opts := &CreateOptions{}
	if err := gimlet.GetJSON(r.Body, opts); err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Wrap(err, "problem reading request").Error(),
		})
		return
	}
	ctx := r.Context()

	if err := opts.Validate(); err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Wrap(err, "invalid creation options").Error(),
		})
		return
	}

	pctx, cancel := context.WithCancel(context.Background())

	proc, err := s.manager.CreateProcess(pctx, opts)
	if err != nil {
		cancel()
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Wrap(err, "problem submitting request").Error(),
		})
		return
	}

	if err := proc.RegisterTrigger(ctx, func(_ ProcessInfo) {
		cancel()
	}); err != nil {
		// If we get an error registering a trigger, then we should make sure that
		// the reason for it isn't just because the process has exited already,
		// since that should not be considered an error.
		if !getProcInfoNoHang(ctx, proc).Complete {
			writeError(rw, gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    errors.Wrap(err, "problem managing resources").Error(),
			})
			return
		}
		cancel()
	}

	gimlet.WriteJSON(rw, getProcInfoNoHang(ctx, proc))
}

func (s *Service) getBuildloggerURLs(rw http.ResponseWriter, r *http.Request) {
	id := gimlet.GetVars(r)["id"]
	ctx := r.Context()

	proc, err := s.manager.Get(ctx, id)
	if err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Wrapf(err, "no process '%s' found", id).Error(),
		})
		return
	}

	info := getProcInfoNoHang(ctx, proc)
	urls := []string{}
	for _, logger := range info.Options.Output.Loggers {
		if logger.Type == LogBuildloggerV2 || logger.Type == LogBuildloggerV3 {
			urls = append(urls, logger.Options.BuildloggerOptions.GetGlobalLogURL())
		}
	}

	if len(urls) == 0 {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Errorf("process '%s' does not use buildlogger", id).Error(),
		})
		return
	}

	gimlet.WriteJSON(rw, urls)
}

func (s *Service) listProcesses(rw http.ResponseWriter, r *http.Request) {
	filter := Filter(gimlet.GetVars(r)["filter"])
	if err := filter.Validate(); err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Wrap(err, "invalid input").Error(),
		})
		return
	}

	ctx := r.Context()

	procs, err := s.manager.List(ctx, filter)
	if err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    err.Error(),
		})
		return
	}

	out := []ProcessInfo{}
	for _, proc := range procs {
		out = append(out, getProcInfoNoHang(ctx, proc))
	}

	gimlet.WriteJSON(rw, out)
}

func (s *Service) listGroupMembers(rw http.ResponseWriter, r *http.Request) {
	name := gimlet.GetVars(r)["name"]

	ctx := r.Context()

	procs, err := s.manager.Group(ctx, name)
	if err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    err.Error(),
		})
		return
	}

	out := []ProcessInfo{}
	for _, proc := range procs {
		out = append(out, getProcInfoNoHang(ctx, proc))
	}

	gimlet.WriteJSON(rw, out)
}

func (s *Service) getProcess(rw http.ResponseWriter, r *http.Request) {
	id := gimlet.GetVars(r)["id"]
	ctx := r.Context()
	proc, err := s.manager.Get(ctx, id)
	if err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    errors.Wrapf(err, "no process '%s' found", id).Error(),
		})
		return
	}

	info := getProcInfoNoHang(ctx, proc)
	gimlet.WriteJSON(rw, info)
}

func (s *Service) processMetrics(rw http.ResponseWriter, r *http.Request) {
	id := gimlet.GetVars(r)["id"]
	ctx := r.Context()
	proc, err := s.manager.Get(ctx, id)
	if err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    errors.Wrapf(err, "no process '%s' found", id).Error(),
		})
		return
	}

	info := getProcInfoNoHang(ctx, proc)
	gimlet.WriteJSON(rw, message.CollectProcessInfoWithChildren(int32(info.PID)))
}

func (s *Service) getProcessTags(rw http.ResponseWriter, r *http.Request) {
	id := gimlet.GetVars(r)["id"]
	ctx := r.Context()
	proc, err := s.manager.Get(ctx, id)
	if err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    errors.Wrapf(err, "no process '%s' found", id).Error(),
		})
		return
	}

	gimlet.WriteJSON(rw, proc.GetTags())
}

func (s *Service) deleteProcessTags(rw http.ResponseWriter, r *http.Request) {
	id := gimlet.GetVars(r)["id"]
	ctx := r.Context()
	proc, err := s.manager.Get(ctx, id)
	if err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    errors.Wrapf(err, "no process '%s' found", id).Error(),
		})
		return
	}

	proc.ResetTags()
	gimlet.WriteJSON(rw, struct{}{})
}

func (s *Service) addProcessTag(rw http.ResponseWriter, r *http.Request) {
	id := gimlet.GetVars(r)["id"]
	ctx := r.Context()
	proc, err := s.manager.Get(ctx, id)
	if err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    errors.Wrapf(err, "no process '%s' found", id).Error(),
		})
		return
	}

	newtags := r.URL.Query()["add"]
	if len(newtags) == 0 {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "no new tags specified",
		})
		return
	}

	for _, t := range newtags {
		proc.Tag(t)
	}

	gimlet.WriteJSON(rw, struct{}{})
}

func (s *Service) waitForProcess(rw http.ResponseWriter, r *http.Request) {
	id := gimlet.GetVars(r)["id"]
	ctx := r.Context()
	proc, err := s.manager.Get(ctx, id)
	if err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    errors.Wrapf(err, "no process '%s' found", id).Error(),
		})
		return
	}

	exitCode, err := proc.Wait(ctx)
	if err != nil && exitCode == -1 {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		})
		return
	}

	gimlet.WriteJSON(rw, exitCode)
}

func (s *Service) respawnProcess(rw http.ResponseWriter, r *http.Request) {
	id := gimlet.GetVars(r)["id"]
	ctx := r.Context()

	proc, err := s.manager.Get(r.Context(), id)
	if err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    errors.Wrapf(err, "no process '%s' found", id).Error(),
		})
		return
	}

	// Spawn a new context so that the process' context is not potentially
	// canceled by the request's. See how createProcess() does this same thing.
	pctx, cancel := context.WithCancel(context.Background())
	newProc, err := proc.Respawn(pctx)
	if err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		})
		cancel()
		return
	}
	if err := s.manager.Register(ctx, newProc); err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message: errors.Wrap(
				err, "failed to register respawned process").Error(),
		})
		cancel()
		return
	}

	if err := newProc.RegisterTrigger(ctx, func(_ ProcessInfo) {
		cancel()
	}); err != nil {
		if !getProcInfoNoHang(ctx, newProc).Complete {
			writeError(rw, gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message: errors.Wrap(
					err, "failed to register trigger on respawn").Error(),
			})
			return
		}
		cancel()
	}

	info := getProcInfoNoHang(ctx, newProc)
	gimlet.WriteJSON(rw, info)
}

func (s *Service) signalProcess(rw http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)
	id := vars["id"]
	sig, err := strconv.Atoi(vars["signal"])
	if err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Wrapf(err, "problem converting signal '%s'", vars["signal"]).Error(),
		})
		return
	}

	ctx := r.Context()
	proc, err := s.manager.Get(ctx, id)
	if err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    errors.Wrapf(err, "no process '%s' found", id).Error(),
		})
		return
	}

	if err := proc.Signal(ctx, syscall.Signal(sig)); err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		})
		return
	}

	gimlet.WriteJSON(rw, struct{}{})
}

func (s *Service) downloadFile(rw http.ResponseWriter, r *http.Request) {
	var info DownloadInfo
	if err := gimlet.GetJSON(r.Body, &info); err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Wrap(err, "problem reading request").Error(),
		})
		return
	}

	if err := info.Validate(); err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Wrap(err, "problem validating download info").Error(),
		})
		return
	}

	if err := info.Download(); err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Wrapf(err, "problem occurred during file download for URL %s", info.URL).Error(),
		})
		return
	}

	gimlet.WriteJSON(rw, struct{}{})
}

func (s *Service) getLogStream(rw http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)
	id := vars["id"]
	count, err := strconv.Atoi(vars["count"])
	if err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Wrapf(err, "problem converting count '%s'", vars["count"]).Error(),
		})
		return
	}

	ctx := r.Context()

	proc, err := s.manager.Get(ctx, id)
	if err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    errors.Wrapf(err, "no process '%s' found", id).Error(),
		})
		return
	}

	stream := LogStream{}
	stream.Logs, err = GetInMemoryLogStream(ctx, proc, count)

	if err == io.EOF {
		stream.Done = true
	} else if err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "could not get logs for process '%s'", id).Error(),
		})
		return
	}

	gimlet.WriteJSON(rw, stream)
}

func (s *Service) clearManager(rw http.ResponseWriter, r *http.Request) {
	s.manager.Clear(r.Context())
	gimlet.WriteJSON(rw, struct{}{})
}

func (s *Service) closeManager(rw http.ResponseWriter, r *http.Request) {
	if err := s.manager.Close(r.Context()); err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		})
		return
	}

	gimlet.WriteJSON(rw, struct{}{})
}

func (s *Service) downloadMongoDB(rw http.ResponseWriter, r *http.Request) {
	opts := MongoDBDownloadOptions{}
	if err := gimlet.GetJSON(r.Body, &opts); err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Wrap(err, "problem reading request").Error(),
		})
		return
	}

	if err := opts.Validate(); err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Wrap(err, "problem validating MongoDB download options").Error(),
		})
		return
	}

	if err := SetupDownloadMongoDBReleases(r.Context(), s.cache, opts); err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "problem in download setup").Error(),
		})
		return
	}

	gimlet.WriteJSON(rw, struct{}{})
}

func (s *Service) configureCache(rw http.ResponseWriter, r *http.Request) {
	opts := CacheOptions{}
	if err := gimlet.GetJSON(r.Body, &opts); err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Wrap(err, "problem reading request").Error(),
		})
		return
	}

	if err := opts.Validate(); err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Wrap(err, "problem validating cache options").Error(),
		})
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

	gimlet.WriteJSON(rw, struct{}{})
}

func (s *Service) registerSignalTriggerID(rw http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)
	id := vars["id"]
	triggerID := vars["trigger-id"]
	ctx := r.Context()

	proc, err := s.manager.Get(ctx, id)
	if err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Wrapf(err, "no process '%s' found", id).Error(),
		})
		return
	}

	sigTriggerID := SignalTriggerID(triggerID)
	makeTrigger, ok := GetSignalTriggerFactory(sigTriggerID)
	if !ok {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Errorf("could not find signal trigger with id '%s'", sigTriggerID).Error(),
		})
		return
	}

	if err := proc.RegisterSignalTrigger(ctx, makeTrigger()); err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "problem registering signal trigger with id '%s'", sigTriggerID).Error(),
		})
		return
	}

	gimlet.WriteJSON(rw, struct{}{})
}

func (s *Service) signalEvent(rw http.ResponseWriter, r *http.Request) {
	vars := gimlet.GetVars(r)
	name := vars["name"]
	ctx := r.Context()

	if err := SignalEvent(ctx, name); err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "problem signaling event named '%s'", name).Error(),
		})
		return
	}

	gimlet.WriteJSON(rw, struct{}{})
}

func (s *Service) oomTrackerClear(rw http.ResponseWriter, r *http.Request) {
	resp := &oomTrackerImpl{}

	if err := resp.Clear(r.Context()); err != nil {
		gimlet.WriteJSONInternalError(rw, err.Error())
		return
	}

	gimlet.WriteJSON(rw, resp)
}

func (s *Service) oomTrackerList(rw http.ResponseWriter, r *http.Request) {
	resp := &oomTrackerImpl{}

	if err := resp.Check(r.Context()); err != nil {
		gimlet.WriteJSONInternalError(rw, err.Error())
		return
	}

	gimlet.WriteJSON(rw, resp)
}
