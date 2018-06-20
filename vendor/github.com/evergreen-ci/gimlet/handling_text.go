package gimlet

import (
	"net/http"
)

// WriteTextResponse writes data to the response body with the given
// code as plain text after attempting to convert the data to a byte
// array.
func WriteTextResponse(w http.ResponseWriter, code int, data interface{}) {
	writeResponse(TEXT, w, code, convertToBytes(data))
}

// WriteText writes the data, converted to text as possible, to the response body, with a successful
// status code.
func WriteText(w http.ResponseWriter, data interface{}) {
	// 200
	WriteTextResponse(w, http.StatusOK, data)
}

// WriteTextError write the data, converted to text as possible, to the response body with a
// bad-request (e.g. 400) response code.
func WriteTextError(w http.ResponseWriter, data interface{}) {
	// 400
	WriteTextResponse(w, http.StatusBadRequest, data)
}

// WriteTextInternalError write the data, converted to text as possible, to the response body with an
// internal server error (e.g. 500) response code.
func WriteTextInternalError(w http.ResponseWriter, data interface{}) {
	// 500
	WriteTextResponse(w, http.StatusInternalServerError, data)
}
