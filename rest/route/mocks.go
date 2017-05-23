package route

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/rest/servicecontext"
)

type mockRequestHandler struct {
	storedMetadata   interface{}
	storedModels     []model.Model
	parseValidateErr error
	executeErr       error
}

func (m *mockRequestHandler) Handler() RequestHandler {
	return &mockRequestHandler{
		storedMetadata:   m.storedMetadata,
		storedModels:     m.storedModels,
		parseValidateErr: m.parseValidateErr,
		executeErr:       m.executeErr,
	}
}
func (m *mockRequestHandler) ParseAndValidate(h *http.Request) error {
	return m.parseValidateErr
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

// Authenticate returns the error embedded in the mock authenticator.
func (m *mockAuthenticator) Authenticate(sc servicecontext.ServiceContext, r *http.Request) error {
	return m.err
}

// mockPaginatorFuncGenerator generates a PaginatorFunc which packages and
// returns the passed in parameters.
func mockPaginatorFuncGenerator(result []model.Model, nextKey, prevKey string,
	nextLimit, prevLimit int, errResult error) PaginatorFunc {
	return func(key string, limit int, args interface{},
		sc servicecontext.ServiceContext) ([]model.Model, *PageResult, error) {

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
