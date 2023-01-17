package mock

import (
	"net/http"
	"net/http/httptest"

	"github.com/evergreen-ci/evergreen"
)

// CedarHandler is a simple handler for an instance a mock Cedar server.
type CedarHandler struct {
	Responses   [][]byte
	StatusCode  int
	LastRequest *http.Request
}

func (h *CedarHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.LastRequest = r
	if h.StatusCode > 0 {
		w.WriteHeader(h.StatusCode)
	}

	if len(h.Responses) > 0 {
		_, _ = w.Write(h.Responses[0])
		h.Responses = h.Responses[1:]
	} else {
		_, _ = w.Write(nil)
	}
}

// NewCedarServer returns a test server and handler for mocking the Cedar
// service. Callers are responsible for closing the server.
func NewCedarServer(env evergreen.Environment) (*httptest.Server, *CedarHandler) {
	handler := &CedarHandler{}
	srv := httptest.NewServer(handler)

	if env == nil {
		env = evergreen.GetEnvironment()
	}
	env.Settings().Cedar.BaseURL = srv.URL

	return srv, handler
}
