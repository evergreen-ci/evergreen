package util

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

// WriteJSON writes a json response with the supplied code on the given writer.
func WriteJSON(w *http.ResponseWriter, data interface{}, status int) {
	// write the response
	(*w).Header().Add("Content-Type", "application/json")
	(*w).Header().Add("Connection", "close")

	jsonBytes, err := json.Marshal(data)
	if err != nil {
		(*w).WriteHeader(500)
		(*w).Write([]byte("{}"))
		return
	}

	(*w).WriteHeader(status)
	(*w).Write([]byte(jsonBytes))
}

// MakeTlsConfig creates a TLS Config from a certificate and key.
func MakeTlsConfig(cert string, key string) (*tls.Config, error) {
	// Adapted from http.ListenAndServeTLS
	tlsConfig := &tls.Config{}
	tlsConfig.NextProtos = []string{"http/1.1"}
	tlsConfig.Certificates = make([]tls.Certificate, 1)
	var err error
	tlsConfig.Certificates[0], err =
		tls.X509KeyPair([]byte(cert), []byte(key))
	if err != nil {
		return nil, err
	}
	return tlsConfig, nil
}

// MountHandler routes all requests to the given mux.Router under the prefix to be handled by
// the http.Handler, which the request's path rooted under that prefix.
// So for example, if a router configured with the path /foo is given to
// MountHandler(r, "/bar/baz", newHandler)
// Then a request to the router at /foo/bar/baz/hello will be handled by newHandler,
// appearing with the path "/baz/hello"
func MountHandler(r *mux.Router, prefix string, h http.Handler) http.Handler {
	root := r.PathPrefix(prefix)
	root.Handler(
		http.HandlerFunc(func(w http.ResponseWriter, rq *http.Request) {
			flattened := make([]string, 0)
			for k, v := range mux.Vars(rq) {
				flattened = append(flattened, k)
				flattened = append(flattened, v)
			}
			url, err := root.URL(flattened...)
			if err != nil {
				http.NotFound(w, rq)
				return
			}
			strip := strings.TrimSuffix(url.String(), "/")
			http.StripPrefix(strip, h).ServeHTTP(w, rq)
		}))
	return r
}

// GetIntValue returns a form value as an integer
func GetIntValue(r *http.Request, valueKey string, defaultValue int) (int, error) {
	val := r.FormValue(valueKey)
	if val == "" {
		return defaultValue, nil
	}
	intVal, err := strconv.Atoi(val)
	if err != nil {
		return 0, errors.Errorf("%v: cannot convert %v to integer: %v", valueKey, val, err.Error())
	}
	return intVal, nil
}

// GetBoolValue returns a form value as an integer
func GetBoolValue(r *http.Request, valueKey string, defaultValue bool) (bool, error) {
	val := r.FormValue(valueKey)
	if val == "" {
		return defaultValue, nil
	}
	boolVal, err := strconv.ParseBool(val)
	if err != nil {
		return defaultValue, errors.Errorf("%v: cannot convert %v to boolean: %v", valueKey, val, err.Error())
	}
	return boolVal, nil
}

// GetStringArrayValue returns a form value as a string array
func GetStringArrayValue(r *http.Request, valueKey string, defaultValue []string) []string {
	val := r.FormValue(valueKey)
	if val == "" {
		return defaultValue
	}
	return strings.Split(val, ",")
}
