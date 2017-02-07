package route

import (
	"net/http"

	"github.com/evergreen-ci/evergreen/apiv3"
	"github.com/evergreen-ci/evergreen/apiv3/servicecontext"
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/util"
)

// Authenticator is an interface which defines how requests can authenticate
// against the API service.
type Authenticator interface {
	Authenticate(servicecontext.ServiceContext, *http.Request) error
}

// NoAuthAuthenticator is an authenticator which allows all requests to pass
// through.
type NoAuthAuthenticator struct{}

// Authenticate does not examine the request and allows all requests to pass
// through.
func (n *NoAuthAuthenticator) Authenticate(sc servicecontext.ServiceContext,
	r *http.Request) error {
	return nil
}

// SuperUserAuthenticator only allows user in the SuperUsers field of the
// settings file to complete the request
type SuperUserAuthenticator struct{}

// Authenticate fetches the user information from the http request
// and checks if it matches the users in the settings file. If no SuperUsers
// exist in the settings file, all users are considered super. It returns
// 'NotFound' errors to prevent leaking sensitive information.
func (s *SuperUserAuthenticator) Authenticate(sc servicecontext.ServiceContext,
	r *http.Request) error {
	u := GetUser(r)

	if auth.IsSuperUser(sc.GetSuperUsers(), u) {
		return nil
	}
	return apiv3.APIError{
		StatusCode: http.StatusNotFound,
		Message:    "Not Found",
	}
}

// ProjectOwnerAuthenticator only allows the owner of the project and
// superusers access to the information. It requires that the project be
// available and that the user also be set.
type ProjectAdminAuthenticator struct{}

// ProjectAdminAuthenticator checks that the user is either a super user or is
// part of the project context's project admins.
func (p *ProjectAdminAuthenticator) Authenticate(sc servicecontext.ServiceContext,
	r *http.Request) error {
	projCtx := MustHaveProjectContext(r)
	u := GetUser(r)

	// If either a superuser or admin, request is allowed to proceed.
	if auth.IsSuperUser(sc.GetSuperUsers(), u) ||
		util.SliceContains(projCtx.ProjectRef.Admins, u.Username()) {
		return nil
	}

	return apiv3.APIError{
		StatusCode: http.StatusNotFound,
		Message:    "Not Found",
	}
}
