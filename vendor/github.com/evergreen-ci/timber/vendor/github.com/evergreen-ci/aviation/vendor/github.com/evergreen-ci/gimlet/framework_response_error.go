package gimlet

import (
	"net/http"

	"github.com/pkg/errors"
)

func getError(e error, defaultCode int) ErrorResponse {
	switch eresp := errors.Cause(e).(type) {
	case *ErrorResponse:
		if http.StatusText(eresp.StatusCode) == "" {
			eresp.StatusCode = defaultCode
		}

		return *eresp
	case ErrorResponse:
		if http.StatusText(eresp.StatusCode) == "" {
			eresp.StatusCode = defaultCode
		}

		return eresp
	default:
		return ErrorResponse{
			StatusCode: defaultCode,
			Message:    e.Error(),
		}
	}
}

// MakeTextErrorResponder takes an error object and converts
// it into a responder object that wrap's gimlet's ErrorResponse
// type. Since ErrorResponse implements the error interface, if you
// pass an ErrorResposne object, this function will propogate the
// status code specified in the ErrorResponse if that code is valid,
// otherwise the status code of the request and the response object
// will be 400.
func MakeTextErrorResponder(err error) Responder {
	e := getError(err, http.StatusBadRequest)

	return &responderImpl{
		data:   e.Error(),
		status: e.StatusCode,
		format: TEXT,
	}
}

// MakeJSONErrorResponder takes an error object and converts
// it into a responder object that wrap's gimlet's ErrorResponse
// type. Since ErrorResponse implements the error interface, if you
// pass an ErrorResposne object, this function will propogate the
// status code specified in the ErrorResponse if that code is valid,
// otherwise the status code of the request and the response object
// will be 400.
func MakeJSONErrorResponder(err error) Responder {
	e := getError(err, http.StatusBadRequest)

	return &responderImpl{
		data:   e,
		status: e.StatusCode,
		format: JSON,
	}
}

// MakeYAMLErrorResponder takes an error object and converts
// it into a responder object that wrap's gimlet's ErrorResponse
// type. Since ErrorResponse implements the error interface, if you
// pass an ErrorResposne object, this function will propogate the
// status code specified in the ErrorResponse if that code is valid,
// otherwise the status code of the request and the response object
// will be 400.
func MakeYAMLErrorResponder(err error) Responder {
	e := getError(err, http.StatusBadRequest)

	return &responderImpl{
		data:   e,
		status: e.StatusCode,
		format: YAML,
	}
}

// MakeTextInternalErrorResponder takes an error object and converts
// it into a responder object that wrap's gimlet's ErrorResponse
// type. Since ErrorResponse implements the error interface, if you
// pass an ErrorResposne object, this function will propogate the
// status code specified in the ErrorResponse if that code is valid,
// otherwise the status code of the request and the response object
// will be 500.
func MakeTextInternalErrorResponder(err error) Responder {
	e := getError(err, http.StatusInternalServerError)

	return &responderImpl{
		data:   e.Error(),
		status: e.StatusCode,
		format: TEXT,
	}
}

// MakeJSONInternalErrorResponder takes an error object and converts
// it into a responder object that wrap's gimlet's ErrorResponse
// type. Since ErrorResponse implements the error interface, if you
// pass an ErrorResposne object, this function will propogate the
// status code specified in the ErrorResponse if that code is valid,
// otherwise the status code of the request and the response object
// will be 500.
func MakeJSONInternalErrorResponder(err error) Responder {
	e := getError(err, http.StatusInternalServerError)

	return &responderImpl{
		data:   e,
		status: e.StatusCode,
		format: JSON,
	}
}

// MakeYAMLInternalErrorResponder takes an error object and converts
// it into a responder object that wrap's gimlet's ErrorResponse
// type. Since ErrorResponse implements the error interface, if you
// pass an ErrorResposne object, this function will propogate the
// status code specified in the ErrorResponse if that code is valid,
// otherwise the status code of the request and the response object
// will be 500.
func MakeYAMLInternalErrorResponder(err error) Responder {
	e := getError(err, http.StatusInternalServerError)

	return &responderImpl{
		data:   e,
		status: e.StatusCode,
		format: YAML,
	}
}
