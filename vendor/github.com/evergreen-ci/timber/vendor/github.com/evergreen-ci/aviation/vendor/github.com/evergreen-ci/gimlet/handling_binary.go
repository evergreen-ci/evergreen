package gimlet

import (
	"net/http"
)

// WriteBinaryResponse writes binary data to a response with the specified code.
func WriteBinaryResponse(w http.ResponseWriter, code int, data interface{}) {
	writeResponse(BINARY, w, code, convertToBin(data))
}

// WriteBinary writes the data, converted to a byte slice as possible, to the response body, with a successful
// status code.
func WriteBinary(w http.ResponseWriter, data interface{}) {
	// 200
	WriteBinaryResponse(w, http.StatusOK, data)
}

// WriteBinaryError write the data, converted to a byte slice as
// possible, to the response body with a bad-request (e.g. 400)
// response code.
func WriteBinaryError(w http.ResponseWriter, data interface{}) {
	// 400
	WriteBinaryResponse(w, http.StatusBadRequest, data)
}

// WriteBinaryInternalError write the data, converted to a byte slice
// as possible, to the response body with an internal server error
// (e.g. 500) response code.
func WriteBinaryInternalError(w http.ResponseWriter, data interface{}) {
	// 500
	WriteBinaryResponse(w, http.StatusInternalServerError, data)
}
