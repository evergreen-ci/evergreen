package gimlet

import (
	"context"
	"net/http"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

// NewAuthenticationHandler produces middleware that attaches
// Authenticator and UserManager instances to the request context,
// enabling the use of GetAuthenticator and GetUserManager accessors.
//
// While your application can have multiple authentication mechanisms,
// a single request can only have one authentication provider
// associated with it.
func NewAuthenticationHandler(a Authenticator, um UserManager) Middleware {
	return &authHandler{
		auth: a,
		um:   um,
	}
}

type authHandler struct {
	auth Authenticator
	um   UserManager
}

func (a *authHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	ctx := r.Context()
	ctx = setAuthenticator(ctx, a.auth)
	ctx = setUserManager(ctx, a.um)

	r = r.WithContext(ctx)

	next(rw, r)
}

func setAuthenticator(ctx context.Context, a Authenticator) context.Context {
	return context.WithValue(ctx, authHandlerKey, a)
}

func setUserManager(ctx context.Context, um UserManager) context.Context {
	return context.WithValue(ctx, userManagerKey, um)
}

func GetAuthenticator(ctx context.Context) (Authenticator, bool) {
	a := ctx.Value(authHandlerKey)
	if a == nil {
		return nil, false
	}

	amgr, ok := a.(Authenticator)
	if !ok {
		return nil, false
	}

	return amgr, true
}

func GetUserManager(ctx context.Context) (UserManager, bool) {
	m := ctx.Value(userManagerKey)
	if m == nil {
		return nil, false
	}

	umgr, ok := m.(UserManager)
	if !ok {
		return nil, false
	}

	return umgr, true
}

// NewRoleRequired provides middlesware that requires a specific role
// to access a resource. This access is defined as a property of the
// user objects.
func NewRoleRequired(role string) Middleware { return &requiredRole{role: role} }

type requiredRole struct {
	role string
}

func (rr *requiredRole) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	ctx := r.Context()

	user, ok := GetUser(ctx)
	if !ok {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	if !userHasRole(user, rr.role) {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	grip.Info(message.Fields{
		"path":          r.URL.Path,
		"remote":        r.RemoteAddr,
		"request":       GetRequestID(ctx),
		"user":          user.Username(),
		"user_roles":    user.Roles(),
		"required_role": rr.role,
	})

	next(rw, r)
}

// NewGroupMembershipRequired provides middleware that requires that
// users belong to a group to gain access to a resource. This is
// access is defined as a property of the authentication system.
func NewGroupMembershipRequired(name string) Middleware { return &requiredGroup{group: name} }

type requiredGroup struct {
	group string
}

func (rg *requiredGroup) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	ctx := r.Context()

	authenticator, ok := GetAuthenticator(ctx)
	if !ok {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	user, ok := GetUser(ctx)
	if !ok {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	if !authenticator.CheckGroupAccess(user, rg.group) {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	grip.Info(message.Fields{
		"path":           r.URL.Path,
		"remote":         r.RemoteAddr,
		"request":        GetRequestID(ctx),
		"user":           user.Username(),
		"user_roles":     user.Roles(),
		"required_group": rg.group,
	})

	next(rw, r)
}

// NewRequireAuth provides middlesware that requires that users be
// authenticated generally to access the resource, but does no
// validation of their access.
func NewRequireAuthHandler() Middleware { return &requireAuthHandler{} }

type requireAuthHandler struct{}

func (_ *requireAuthHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	ctx := r.Context()

	authenticator, ok := GetAuthenticator(ctx)
	if !ok {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	user, ok := GetUser(ctx)
	if !ok {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	if !authenticator.CheckAuthenticated(user) {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	grip.Info(message.Fields{
		"path":       r.URL.Path,
		"remote":     r.RemoteAddr,
		"request":    GetRequestID(ctx),
		"user":       user.Username(),
		"user_roles": user.Roles(),
	})

	next(rw, r)
}

// NewRestrictAccessToUsers allows you to define a list of users that
// may access certain resource. This is similar to "groups," but allows
// you to control access centrally rather than needing to edit or
// change user documents.
//
// This middleware is redundant to the "access required middleware."
func NewRestrictAccessToUsers(userIDs []string) Middleware {
	return &restrictedAccessHandler{ids: userIDs}
}

type restrictedAccessHandler struct {
	ids []string
}

func (ra *restrictedAccessHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	ctx := r.Context()

	user, ok := GetUser(ctx)
	if !ok {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	authenticator, ok := GetAuthenticator(ctx)
	if !ok {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	if !authenticator.CheckAuthenticated(user) {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	id := user.Username()
	for _, allowed := range ra.ids {
		if id == allowed {
			next(rw, r)
			return
		}
	}

	rw.WriteHeader(http.StatusUnauthorized)
}
