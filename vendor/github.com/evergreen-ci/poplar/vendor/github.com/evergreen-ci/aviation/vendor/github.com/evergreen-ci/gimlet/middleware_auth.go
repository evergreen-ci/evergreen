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

// GetAuthenticator returns an the attached interface to the
// context. If there is no authenticator attached, then
// GetAutenticator returns nil.
func GetAuthenticator(ctx context.Context) Authenticator {
	a := ctx.Value(authHandlerKey)
	if a == nil {
		return nil
	}

	amgr, ok := a.(Authenticator)
	if !ok {
		return nil
	}

	return amgr
}

// GetUserManager returns the attached UserManager to the current
// request, returning nil if no such object is attached.
func GetUserManager(ctx context.Context) UserManager {
	m := ctx.Value(userManagerKey)
	if m == nil {
		return nil
	}

	umgr, ok := m.(UserManager)
	if !ok {
		return nil
	}

	return umgr
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

	user := GetUser(ctx)
	if user == nil {
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

	authenticator := GetAuthenticator(ctx)
	if authenticator == nil {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	user := GetUser(ctx)
	if user == nil {
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

// NewRequireAuthHandler provides middlesware that requires that users be
// authenticated generally to access the resource, but does no
// validation of their access.
func NewRequireAuthHandler() Middleware { return &requireAuthHandler{} }

type requireAuthHandler struct{}

func (*requireAuthHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	ctx := r.Context()

	authenticator := GetAuthenticator(ctx)
	if authenticator == nil {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	user := GetUser(ctx)
	if user == nil {
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

	user := GetUser(ctx)
	if user == nil {
		rw.WriteHeader(http.StatusUnauthorized)
		return
	}

	authenticator := GetAuthenticator(ctx)
	if authenticator == nil {
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
