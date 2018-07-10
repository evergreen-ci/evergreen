package gimlet

import (
	"net/http"

	"github.com/pkg/errors"
)

func getError(e error, defaultCode int) ErrorResponse {
	switch eresp := errors.Cause(e).(type) {
	case *ErrorResponse:
		return *eresp
	case ErrorResponse:
		return eresp
	default:
		return ErrorResponse{
			StatusCode: defaultCode,
			Message:    e.Error(),
		}
	}
}

func MakeTextErrorResponder(err error) Responder {
	e := getError(err, http.StatusBadRequest)

	return &responderImpl{
		data:   e.Error(),
		status: e.StatusCode,
		format: TEXT,
	}
}

func MakeJSONErrorResponder(err error) Responder {
	e := getError(err, http.StatusBadRequest)

	return &responderImpl{
		data:   e,
		status: e.StatusCode,
		format: JSON,
	}
}

func MakeYAMLErrorResponder(err error) Responder {
	e := getError(err, http.StatusBadRequest)

	return &responderImpl{
		data:   e,
		status: e.StatusCode,
		format: YAML,
	}
}
