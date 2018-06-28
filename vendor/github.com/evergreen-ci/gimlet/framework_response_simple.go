package gimlet

import (
	"net/http"
)

func NewTextResponse(data interface{}) Responder {
	return &responderImpl{
		data:   data,
		status: http.StatusOK,
		format: TEXT,
	}
}

func NewTextErrorResponse(data interface{}) Responder {
	return &responderImpl{
		data:   data,
		status: http.StatusBadRequest,
		format: TEXT,
	}
}

func NewTextInternalErrorResponse(data interface{}) Responder {
	return &responderImpl{
		data:   data,
		status: http.StatusInternalServerError,
		format: TEXT,
	}
}

func NewJSONResponse(data interface{}) Responder {
	return &responderImpl{
		data:   data,
		status: http.StatusOK,
		format: JSON,
	}
}

func NewJSONErrorResponse(data interface{}) Responder {
	return &responderImpl{
		data:   data,
		status: http.StatusBadRequest,
		format: JSON,
	}
}

func NewJSONInternalErrorResponse(data interface{}) Responder {
	return &responderImpl{
		data:   data,
		status: http.StatusInternalServerError,
		format: JSON,
	}
}

func NewBinaryResponse(data interface{}) Responder {
	return &responderImpl{
		data:   data,
		status: http.StatusOK,
		format: BINARY,
	}
}

func NewBinaryErrorResponse(data interface{}) Responder {
	return &responderImpl{
		data:   convertToBin(data),
		status: http.StatusBadRequest,
		format: BINARY,
	}
}

func NewBinaryInternalErrorResponse(data interface{}) Responder {
	return &responderImpl{
		data:   data,
		status: http.StatusInternalServerError,
		format: BINARY,
	}
}

func NewHTMLResponse(data interface{}) Responder {
	return &responderImpl{
		data:   data,
		status: http.StatusOK,
		format: HTML,
	}
}

func NewHTMLErrorResponse(data interface{}) Responder {
	return &responderImpl{
		data:   data,
		status: http.StatusBadRequest,
		format: HTML,
	}
}

func NewHTMLInternalErrorResponse(data interface{}) Responder {
	return &responderImpl{
		data:   data,
		status: http.StatusInternalServerError,
		format: HTML,
	}
}

func NewYAMLResponse(data interface{}) Responder {
	return &responderImpl{
		data:   data,
		status: http.StatusOK,
		format: YAML,
	}
}

func NewYAMLErrorResponse(data interface{}) Responder {
	return &responderImpl{
		data:   data,
		status: http.StatusBadRequest,
		format: YAML,
	}
}

func NewYAMLInternalErrorResponse(data interface{}) Responder {
	return &responderImpl{
		data:   data,
		status: http.StatusInternalServerError,
		format: YAML,
	}
}
