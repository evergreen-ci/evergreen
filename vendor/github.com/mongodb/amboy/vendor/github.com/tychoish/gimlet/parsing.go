package gimlet

import (
	"encoding/json"
	"io"
	"io/ioutil"
	"net/http"

	"github.com/gorilla/mux"
	yaml "gopkg.in/yaml.v2"
)

const maxRequestSize = 16 * 1024 * 1024 // 16 MB

// GetVars is a helper method that processes an http.Request and
// returns a map of strings to decoded strings for all arguments
// passed to the method in the URL. Use this helper function when
// writing handler functions.
func GetVars(r *http.Request) map[string]string {
	return mux.Vars(r)
}

// GetJSON parses JSON from a io.ReadCloser (e.g. http/*Request.Body
// or http/*Response.Body) into an object specified by the
// request. Used in handler functiosn to retreve and parse data
// submitted by the client.
func GetJSON(r io.ReadCloser, data interface{}) error {
	defer r.Close()

	bytes, err := ioutil.ReadAll(&io.LimitedReader{R: r, N: maxRequestSize})
	if err != nil {
		return err
	}

	return json.Unmarshal(bytes, data)
}

// GetYAML parses YAML from a io.ReadCloser (e.g. http/*Request.Body
// or http/*Response.Body) into an object specified by the
// request. Used in handler functiosn to retreve and parse data
// submitted by the client.u
func GetYAML(r io.ReadCloser, data interface{}) error {
	defer r.Close()

	bytes, err := ioutil.ReadAll(&io.LimitedReader{R: r, N: maxRequestSize})
	if err != nil {
		return err
	}

	return yaml.Unmarshal(bytes, data)
}
