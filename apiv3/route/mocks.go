package route

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/apiv3/model"
	"github.com/evergreen-ci/evergreen/apiv3/servicecontext"
)

type mockRequestHandler struct {
	storedModel model.Model
	parseErr    error
	validateErr error
	executeErr  error
}

func (m *mockRequestHandler) Handler() RequestHandler {
	return &mockRequestHandler{
		storedModel: m.storedModel,
		parseErr:    m.parseErr,
		validateErr: m.validateErr,
		executeErr:  m.executeErr,
	}
}
func (m *mockRequestHandler) Parse(h *http.Request) error {
	return m.parseErr
}

func (m *mockRequestHandler) Validate() error {
	return m.validateErr
}

func (m *mockRequestHandler) Execute(sc *servicecontext.ServiceContext) (model.Model, error) {
	return m.storedModel, m.executeErr
}

// MockAuthenticator is an authenticator for testing uses of authenticators.
// It returns whatever is stored within it.
type mockAuthenticator struct {
	err error
}

// Authenticate returns the error embeded in the mock authenticator.
func (m *mockAuthenticator) Authenticate(r *http.Request) error {
	return m.err
}
