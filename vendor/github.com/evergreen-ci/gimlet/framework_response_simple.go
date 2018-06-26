package gimlet

import (
	"encoding/json"
	"fmt"
	"net/http"

	yaml "gopkg.in/yaml.v2"
)

func NewTextResponse(data interface{}) Responder {
	return &responderImpl{
		data:   convertToBytes(data),
		status: http.StatusOK,
		format: TEXT,
	}
}

func NewTextErrorResponse(data interface{}) Responder {
	return &responderImpl{
		data:   convertToBytes(data),
		status: http.StatusBadRequest,
		format: TEXT,
	}
}

func NewTextInternalErrorResponse(data interface{}) Responder {
	return &responderImpl{
		data:   convertToBytes(data),
		status: http.StatusInternalServerError,
		format: TEXT,
	}
}

func getJSONResponseBody(data interface{}) ([]byte, bool) {
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return convertToBytes(err.Error()), false
	}

	return out, true
}

func NewJSONResponse(data interface{}) Responder {
	out, ok := getJSONResponseBody(data)
	if !ok {
		return &responderImpl{
			data:   out,
			status: http.StatusInternalServerError,
			format: TEXT,
		}
	}

	return &responderImpl{
		data:   append(out, []byte("\n")...),
		status: http.StatusOK,
		format: JSON,
	}
}

func NewJSONErrorResponse(data interface{}) Responder {
	out, ok := getJSONResponseBody(data)
	if !ok {
		return &responderImpl{
			data:   out,
			status: http.StatusInternalServerError,
			format: TEXT,
		}
	}

	return &responderImpl{
		data:   append(out, []byte("\n")...),
		status: http.StatusBadRequest,
		format: JSON,
	}
}

func NewJSONInternalErrorResponse(data interface{}) Responder {
	out, ok := getJSONResponseBody(data)
	if !ok {
		return &responderImpl{
			data:   out,
			status: http.StatusInternalServerError,
			format: TEXT,
		}
	}

	return &responderImpl{
		data:   append(out, []byte("\n")...),
		status: http.StatusInternalServerError,
		format: JSON,
	}
}

func NewBinaryResponse(data interface{}) Responder {
	return &responderImpl{
		data:   convertToBin(data),
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
		data:   convertToBin(data),
		status: http.StatusInternalServerError,
		format: BINARY,
	}
}

func NewHTMLResponse(data interface{}) Responder {
	return &responderImpl{
		data:   convertToBytes(data),
		status: http.StatusOK,
		format: HTML,
	}
}

func NewHTMLErrorResponse(data interface{}) Responder {
	return &responderImpl{
		data:   convertToBytes(data),
		status: http.StatusBadRequest,
		format: HTML,
	}
}

func NewHTMLInternalErrorResponse(data interface{}) Responder {
	return &responderImpl{
		data:   convertToBytes(data),
		status: http.StatusInternalServerError,
		format: HTML,
	}
}

func getYAMLResponseBody(data interface{}) (out []byte, ok bool) {
	defer func() {
		if msg := recover(); msg != nil {
			out = convertToBytes(fmt.Sprintf("problem yaml parsing message: %v", msg))
			ok = false
		}
	}()

	// ignoring the error because the yaml library always panics
	out, _ = yaml.Marshal(data)
	return out, true
}

func NewYAMLResponse(data interface{}) Responder {
	out, ok := getYAMLResponseBody(data)
	if !ok {
		return &responderImpl{
			data:   out,
			status: http.StatusInternalServerError,
			format: TEXT,
		}
	}

	return &responderImpl{
		data:   append(out, []byte("\n")...),
		status: http.StatusOK,
		format: YAML,
	}
}

func NewYAMLErrorResponse(data interface{}) Responder {
	out, ok := getYAMLResponseBody(data)
	if !ok {
		return &responderImpl{
			data:   out,
			status: http.StatusInternalServerError,
			format: TEXT,
		}
	}

	return &responderImpl{
		data:   append(out, []byte("\n")...),
		status: http.StatusBadRequest,
		format: YAML,
	}
}

func NewYAMLInternalErrorResponse(data interface{}) Responder {
	out, ok := getYAMLResponseBody(data)
	if !ok {
		return &responderImpl{
			data:   out,
			status: http.StatusInternalServerError,
			format: TEXT,
		}
	}

	return &responderImpl{
		data:   append(out, []byte("\n")...),
		status: http.StatusInternalServerError,
		format: YAML,
	}
}
