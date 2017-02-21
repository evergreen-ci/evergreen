package route

import (
	"encoding/json"
	"net/http"

	"github.com/evergreen-ci/evergreen/apiv3"
	"github.com/evergreen-ci/evergreen/apiv3/model"
	"github.com/evergreen-ci/evergreen/apiv3/servicecontext"
)

// MethodHandler contains all of the methods necessary for completely processing
// an API request. It contains an Authenticator to control access to the method
// and a RequestHandler to perform the required work for the request.
type MethodHandler struct {
	// MethodType is the HTTP Method Type that this handler will handler.
	// POST, PUT, DELETE, etc.
	MethodType string

	Authenticator
	RequestHandler
}

// RequestHandler is an interface that defines how to process an HTTP request
// against an API resource.
type RequestHandler interface {
	// Handler defines how to fetch a new version of this handler.
	Handler() RequestHandler

	// Parse defines how to retrieve the needed parameters from the HTTP request.
	// All needed data should be retrieved during the parse function since
	// other functions do not have access to the HTTP request.
	Parse(*http.Request) error

	// Validate defines how to ensure all needed data is present and in the format
	// expected by the route
	Validate() error

	// Execute performs the necessary work on the evergreen backend and returns
	// an API model to be surfaced to the user.
	Execute(*servicecontext.ServiceContext) (model.Model, error)
}

// makeHandler makes an http.HandlerFunc that wraps calls to each of the api
// Method functions. It marshalls the response to JSON and writes it out to
// as the response. If any of the functions return an error, it handles creating
// a JSON error and sending it as the response.
func makeHandler(methodHandler MethodHandler, sc servicecontext.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")

		var err error
		err = methodHandler.Authenticate(r)
		if err != nil {
			handleAPIError(err, w)
			return
		}
		reqHandler := methodHandler.RequestHandler.New()

		err = reqHandler.Parse(r)
		if err != nil {
			handleAPIError(err, w)
			return
		}
		err = reqHandler.Validate()
		if err != nil {
			handleAPIError(err, w)
			return
		}
		result, err := reqHandler.Execute(&sc)
		if err != nil {
			handleAPIError(err, w)
			return
		}
		jsonResult, err := json.Marshal(result)
		if err != nil {
			http.Error(w, "{}", http.StatusInternalServerError)
			return
		}
		w.Write(jsonResult)
	}
}

// handleAPIError handles writing the given error to the response writer.
// It checks if the given error is an APIError and turns it into JSON to be
// written back to the requester. If the error is unknown, it must have come
// from a service layer package, in which case it is an internal server error
// and is returned as such.
func handleAPIError(e error, w http.ResponseWriter) {
	apiErr := apiv3.APIError{}

	apiErr.StatusCode = http.StatusInternalServerError
	apiErr.Message = e.Error()

	if castError, ok := e.(apiv3.APIError); ok {
		apiErr = castError
	}

	marshalled, err := json.Marshal(apiErr)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("{}"))
		return
	}

	w.WriteHeader(apiErr.StatusCode)
	w.Write(marshalled)
}
