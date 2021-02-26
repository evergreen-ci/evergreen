package remote

import (
	"net/http"
	"time"

	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
)

// ErrLoggingCacheNotSupported is an error indicating that the remote interface
// does not have a logging cache available.
var ErrLoggingCacheNotSupported = errors.New("logging cache is not supported")

func (s *Service) loggingCacheCreate(rw http.ResponseWriter, r *http.Request) {
	opts := &options.Output{}
	id := gimlet.GetVars(r)["id"]
	if err := gimlet.GetJSON(r.Body, opts); err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Wrap(err, "problem parsing options").Error(),
		})
		return
	}

	if err := opts.Validate(); err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Wrap(err, "invalid options").Error(),
		})
		return
	}

	lc := s.manager.LoggingCache(r.Context())
	if lc == nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    ErrLoggingCacheNotSupported.Error(),
		})
		return
	}

	logger, err := lc.Create(id, opts)
	if err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Wrap(err, "creating logger").Error(),
		})
		return
	}
	logger.ManagerID = s.manager.ID()

	gimlet.WriteJSON(rw, logger)
}

func (s *Service) loggingCacheGet(rw http.ResponseWriter, r *http.Request) {
	id := gimlet.GetVars(r)["id"]
	lc := s.manager.LoggingCache(r.Context())
	if lc == nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    ErrLoggingCacheNotSupported.Error(),
		})
		return
	}
	logger, err := lc.Get(id)
	if err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    err.Error(),
		})
		return
	}
	gimlet.WriteJSON(rw, logger)
}

func (s *Service) loggingCacheRemove(rw http.ResponseWriter, r *http.Request) {
	id := gimlet.GetVars(r)["id"]
	lc := s.manager.LoggingCache(r.Context())
	if lc == nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    ErrLoggingCacheNotSupported.Error(),
		})
		return
	}

	if err := lc.Remove(id); err != nil {
		code := http.StatusInternalServerError
		if errors.Cause(err) == jasper.ErrCachedLoggerNotFound {
			code = http.StatusNotFound
		}
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: code,
			Message:    err.Error(),
		})
		return
	}

	gimlet.WriteJSON(rw, struct{}{})
}

func (s *Service) loggingCacheCloseAndRemove(rw http.ResponseWriter, r *http.Request) {
	id := gimlet.GetVars(r)["id"]
	lc := s.manager.LoggingCache(r.Context())
	if lc == nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    ErrLoggingCacheNotSupported.Error(),
		})
		return
	}

	if err := lc.CloseAndRemove(r.Context(), id); err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		})
		return
	}

	gimlet.WriteJSON(rw, struct{}{})
}

func (s *Service) loggingCacheClear(rw http.ResponseWriter, r *http.Request) {
	lc := s.manager.LoggingCache(r.Context())
	if lc == nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    ErrLoggingCacheNotSupported.Error(),
		})
		return
	}

	if err := lc.Clear(r.Context()); err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		})
		return
	}

	gimlet.WriteJSON(rw, struct{}{})
}

type restLoggingCacheLen struct {
	Len int `json:"len"`
}

func (s *Service) loggingCacheLen(rw http.ResponseWriter, r *http.Request) {
	lc := s.manager.LoggingCache(r.Context())
	if lc == nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    ErrLoggingCacheNotSupported.Error(),
		})
		return
	}
	length, err := lc.Len()
	if err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		})
		return
	}
	gimlet.WriteJSON(rw, &restLoggingCacheLen{Len: length})
}

func (s *Service) loggingCachePrune(rw http.ResponseWriter, r *http.Request) {
	ts, err := time.Parse(time.RFC3339, gimlet.GetVars(r)["time"])
	if err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Wrapf(err, "parsing prune timestamp").Error(),
		})
		return
	}

	lc := s.manager.LoggingCache(r.Context())
	if lc == nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    ErrLoggingCacheNotSupported.Error(),
		})
		return
	}

	if err := lc.Prune(ts); err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		})
		return
	}

	gimlet.WriteJSON(rw, struct{}{})
}
