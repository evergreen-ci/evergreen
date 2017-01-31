package route

import (
	"net/http"
)

// Authenticator is an interface which defines how requests can authenticate
// against the API service.
type Authenticator interface {
	Authenticate(*http.Request) error
}

// NoAuthAuthenticator is an authenticator which allows all requests to pass
// through.
type NoAuthAuthenticator struct{}

// Authenticate does not examine the request and allows all requests to pass
// through.
func (n *NoAuthAuthenticator) Authenticate(r *http.Request) error {
	return nil
}
