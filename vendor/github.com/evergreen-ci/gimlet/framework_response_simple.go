package gimlet

import (
	"net/http"

	"github.com/pkg/errors"
)

func newResponder(data interface{}, code int, of OutputFormat) Responder {
	switch in := data.(type) {
	case error:
		var eresp ErrorResponse
		switch err := errors.Cause(in).(type) {
		case *ErrorResponse:
			if http.StatusText(eresp.StatusCode) == "" {
				eresp.StatusCode = code
			}

			eresp = *err
		case ErrorResponse:
			if http.StatusText(eresp.StatusCode) == "" {
				eresp.StatusCode = code
			}

			eresp = err
		default:
			eresp = ErrorResponse{
				StatusCode: code,
				Message:    err.Error(),
			}
		}

		if of == TEXT {
			return &responderImpl{
				data:   eresp.Error(),
				status: eresp.StatusCode,
				format: of,
			}
		}
		return &responderImpl{
			data:   eresp,
			status: eresp.StatusCode,
			format: of,
		}
	default:
		return &responderImpl{
			data:   data,
			status: code,
			format: of,
		}
	}
}

func NewTextResponse(data interface{}) Responder {
	return newResponder(data, http.StatusOK, TEXT)
}

func NewTextErrorResponse(data interface{}) Responder {
	return newResponder(data, http.StatusBadRequest, TEXT)
}

func NewTextInternalErrorResponse(data interface{}) Responder {
	return newResponder(data, http.StatusInternalServerError, TEXT)
}

func NewJSONResponse(data interface{}) Responder {
	return newResponder(data, http.StatusOK, JSON)
}

func NewJSONErrorResponse(data interface{}) Responder {
	return newResponder(data, http.StatusBadRequest, JSON)
}

func NewJSONInternalErrorResponse(data interface{}) Responder {
	return newResponder(data, http.StatusInternalServerError, JSON)
}

func NewBinaryResponse(data interface{}) Responder {
	return newResponder(data, http.StatusOK, BINARY)
}

func NewBinaryErrorResponse(data interface{}) Responder {
	return newResponder(data, http.StatusBadRequest, BINARY)
}

func NewBinaryInternalErrorResponse(data interface{}) Responder {
	return newResponder(data, http.StatusInternalServerError, BINARY)
}

func NewHTMLResponse(data interface{}) Responder {
	return newResponder(data, http.StatusOK, HTML)
}

func NewHTMLErrorResponse(data interface{}) Responder {
	return newResponder(data, http.StatusBadRequest, HTML)
}

func NewHTMLInternalErrorResponse(data interface{}) Responder {
	return newResponder(data, http.StatusInternalServerError, HTML)
}

func NewYAMLResponse(data interface{}) Responder {
	return newResponder(data, http.StatusOK, YAML)
}

func NewYAMLErrorResponse(data interface{}) Responder {
	return newResponder(data, http.StatusBadRequest, YAML)
}

func NewYAMLInternalErrorResponse(data interface{}) Responder {
	return newResponder(data, http.StatusInternalServerError, YAML)
}
