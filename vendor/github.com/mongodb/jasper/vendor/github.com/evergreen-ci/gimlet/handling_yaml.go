package gimlet

import (
	"fmt"
	"net/http"

	"github.com/mongodb/grip"
	yaml "gopkg.in/yaml.v2"
)

// WriteYAMLResponse writes a YAML document to the body of an HTTP
// request, setting the return status of to 500 if the YAML
// seralization process encounters an error, otherwise return
func WriteYAMLResponse(w http.ResponseWriter, code int, data interface{}) {
	defer func() {
		if msg := recover(); msg != nil {
			m := fmt.Sprintf("problem yaml parsing message: %v", msg)
			grip.Debug(m)
			http.Error(w, m, http.StatusInternalServerError)
		}
	}()

	// ignoring the error because the yaml library always panics
	out, _ := yaml.Marshal(data)
	writeResponse(YAML, w, code, out)
}

// WriteYAML is a helper method to write YAML data to the body of an
// HTTP request and return 200 (successful.)
func WriteYAML(w http.ResponseWriter, data interface{}) {
	// 200
	WriteYAMLResponse(w, http.StatusOK, data)
}

// WriteYAMLError is a helper method to write YAML data to the body of
// an HTTP request and return 400 (user error.)
func WriteYAMLError(w http.ResponseWriter, data interface{}) {
	// 400
	WriteYAMLResponse(w, http.StatusBadRequest, data)
}

// WriteYAMLInternalError is a helper method to write YAML data to the
// body of an HTTP request and return 500 (internal error.)
func WriteYAMLInternalError(w http.ResponseWriter, data interface{}) {
	// 500
	WriteYAMLResponse(w, http.StatusInternalServerError, data)
}
