package route

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/apiv3/model"
	"github.com/evergreen-ci/evergreen/apiv3/servicecontext"
)

type mockRequestHandler struct {
	storedMetadata interface{}
	storedModels   []model.Model
	parseErr       error
	validateErr    error
	executeErr     error
}

func (m *mockRequestHandler) Handler() RequestHandler {
	return &mockRequestHandler{
		storedMetadata: m.storedMetadata,
		storedModels:   m.storedModels,
		parseErr:       m.parseErr,
		validateErr:    m.validateErr,
		executeErr:     m.executeErr,
	}
}
func (m *mockRequestHandler) Parse(h *http.Request) error {
	return m.parseErr
}

func (m *mockRequestHandler) Validate() error {
	return m.validateErr
}

func (m *mockRequestHandler) Execute(sc servicecontext.ServiceContext) (ResponseData, error) {
	return ResponseData{
		Result:   m.storedModels,
		Metadata: m.storedMetadata,
	}, m.executeErr
}

// MockAuthenticator is an authenticator for testing uses of authenticators.
// It returns whatever is stored within it.
type mockAuthenticator struct {
	err error
}

// Authenticate returns the error embeded in the mock authenticator.
func (m *mockAuthenticator) Authenticate(sc servicecontext.ServiceContext, r *http.Request) error {
	return m.err
}

// mockPaginatorFuncGenerator generates a PaginatorFunc which packages and
// returns the passed in parameters.
func mockPaginatorFuncGenerator(result []model.Model, nextKey, prevKey string,
	nextLimit, prevLimit int, errResult error) PaginatorFunc {
	return func(key string, limit int, sc servicecontext.ServiceContext) ([]model.Model,
		*PageResult, error) {

		nextPage := Page{
			Limit:    nextLimit,
			Key:      nextKey,
			Relation: "next",
		}

		prevPage := Page{
			Limit:    prevLimit,
			Key:      prevKey,
			Relation: "prev",
		}

		return result, &PageResult{&nextPage, &prevPage}, errResult

	}
}
