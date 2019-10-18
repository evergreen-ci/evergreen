package gimlet

import (
	"net/http"
)

// WriteHTMLResponse writes an HTML response with the specified error code.
func WriteHTMLResponse(w http.ResponseWriter, code int, data interface{}) {
	writeResponse(HTML, w, code, convertToBytes(data))
}

// WriteHTML writes the data, converted to text as possible, to the
// response body as HTML with a successful status code.
func WriteHTML(w http.ResponseWriter, data interface{}) {
	// 200
	WriteHTMLResponse(w, http.StatusOK, data)
}

// WriteHTMLError write the data, converted to text as possible, to
// the response body as HTML with a bad-request (e.g. 400) response code.
func WriteHTMLError(w http.ResponseWriter, data interface{}) {
	// 400
	WriteHTMLResponse(w, http.StatusBadRequest, data)
}

// WriteHTMLInternalError write the data, converted to text as possible, to
// the response body as HTML with an internal server error (e.g. 500)
// response code.
func WriteHTMLInternalError(w http.ResponseWriter, data interface{}) {
	// 500
	WriteHTMLResponse(w, http.StatusInternalServerError, data)
}
