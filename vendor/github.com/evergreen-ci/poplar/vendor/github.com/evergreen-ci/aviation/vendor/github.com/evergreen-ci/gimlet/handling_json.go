package gimlet

import (
	"encoding/json"
	"net/http"

	"github.com/mongodb/grip"
)

// WriteJSONResponse writes a JSON document to the body of an HTTP
// request, setting the return status of to 500 if the JSON
// seralization process encounters an error, otherwise return
func WriteJSONResponse(w http.ResponseWriter, code int, data interface{}) {
	response, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		grip.Debug(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	writeResponse(JSON, w, code, append(response, []byte("\n")...))
}

// WriteJSON is a helper method to write JSON data to the body of an
// HTTP request and return 200 (successful.)
func WriteJSON(w http.ResponseWriter, data interface{}) {
	// 200
	WriteJSONResponse(w, http.StatusOK, data)
}

// WriteJSONError is a helper method to write JSON data to the body of
// an HTTP request and return 400 (user error.)
func WriteJSONError(w http.ResponseWriter, data interface{}) {
	// 400
	WriteJSONResponse(w, http.StatusBadRequest, data)
}

// WriteJSONInternalError is a helper method to write JSON data to the
// body of an HTTP request and return 500 (internal error.)
func WriteJSONInternalError(w http.ResponseWriter, data interface{}) {
	// 500
	WriteJSONResponse(w, http.StatusInternalServerError, data)
}
