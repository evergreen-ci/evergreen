package remote

import (
	"io/ioutil"
	"net/http"

	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/jasper/scripting"
)

func (s *Service) scriptingSetup(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	sh, err := s.harnesses.Get(gimlet.GetVars(r)["id"])
	if err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    err.Error(),
		})
		return
	}

	if err := sh.Setup(ctx); err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		})
		return
	}

	gimlet.WriteJSON(rw, struct{}{})
}

func (s *Service) scriptingRun(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	sh, err := s.harnesses.Get(gimlet.GetVars(r)["id"])
	if err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    err.Error(),
		})
		return
	}

	args := &struct {
		Args []string `json:"args"`
	}{}
	if err := gimlet.GetJSON(r.Body, args); err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		})
		return
	}

	if err := sh.Run(ctx, args.Args); err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		})
		return
	}

	gimlet.WriteJSON(rw, struct{}{})
}

func (s *Service) scriptingRunScript(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	sh, err := s.harnesses.Get(gimlet.GetVars(r)["id"])
	if err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    err.Error(),
		})
		return
	}

	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		})
		return
	}

	if err := sh.RunScript(ctx, string(data)); err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		})
		return
	}

	gimlet.WriteJSON(rw, struct{}{})
}

func (s *Service) scriptingBuild(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	sh, err := s.harnesses.Get(gimlet.GetVars(r)["id"])
	if err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    err.Error(),
		})
		return
	}

	args := &struct {
		Directory string   `json:"directory"`
		Args      []string `json:"args"`
	}{}
	if err = gimlet.GetJSON(r.Body, args); err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		})
		return
	}
	path, err := sh.Build(ctx, args.Directory, args.Args)
	if err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		})
		return
	}

	gimlet.WriteJSON(rw, struct {
		Path string `json:"path"`
	}{
		Path: path,
	})
}

func (s *Service) scriptingTest(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	sh, err := s.harnesses.Get(gimlet.GetVars(r)["id"])
	if err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    err.Error(),
		})
		return
	}

	args := &struct {
		Directory string                  `json:"directory"`
		Options   []scripting.TestOptions `json:"options"`
	}{}
	if err = gimlet.GetJSON(r.Body, args); err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		})
		return
	}
	var errOut string
	res, err := sh.Test(ctx, args.Directory, args.Options...)
	if err != nil {
		errOut = err.Error()
	}

	gimlet.WriteJSON(rw, struct {
		Results []scripting.TestResult `json:"results"`
		Error   string                 `json:"error"`
	}{
		Results: res,
		Error:   errOut,
	})
}

func (s *Service) scriptingCleanup(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	sh, err := s.harnesses.Get(gimlet.GetVars(r)["id"])
	if err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    err.Error(),
		})
		return
	}

	if err := sh.Cleanup(ctx); err != nil {
		writeError(rw, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		})
		return
	}
	gimlet.WriteJSON(rw, struct{}{})
}
