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

type requiresPermissionHandler struct {
	opts RequiresPermissionMiddlewareOpts
}

type FindResourceFunc func(*http.Request) ([]string, int, error)

// RequiresPermissionMiddlewareOpts defines what permissions the middleware shoud check and how. The ResourceFunc parameter
// can be used to specify custom behavior to extract a valid resource name from request variables
type RequiresPermissionMiddlewareOpts struct {
	RM             RoleManager
	PermissionKey  string
	ResourceType   string
	RequiredLevel  int
	ResourceLevels []string
	DefaultRoles   []Role
	ResourceFunc   FindResourceFunc
}

// RequiresPermission allows a route to specify that access to a given resource in the route requires a certain permission
// at a certain level. The resource ID must be defined somewhere in the URL as mux.Vars. The specific URL params to check
// need to be sent in the last parameter of this function, in order of most to least specific
func RequiresPermission(opts RequiresPermissionMiddlewareOpts) Middleware {
	return &requiresPermissionHandler{
		opts: opts,
	}
}

func (rp *requiresPermissionHandler) ServeHTTP(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	vars := GetVars(r)
	var resources []string
	var status int
	var err error
	if rp.opts.ResourceFunc != nil {
		resources, status, err = rp.opts.ResourceFunc(r)
		if err != nil {
			http.Error(rw, err.Error(), status)
			return
		}
	} else {
		for _, level := range rp.opts.ResourceLevels {
			if resourceVal, exists := vars[level]; exists {
				resources = []string{resourceVal}
				break
			}
		}
	}

	if len(resources) == 0 {
		http.Error(rw, "no resources found", http.StatusNotFound)
		return
	}
	if ok := rp.checkPermissions(rw, r.Context(), resources); !ok {
		return
	}

	next(rw, r)
}

func (rp *requiresPermissionHandler) checkPermissions(rw http.ResponseWriter, ctx context.Context, resources []string) bool {
	user := GetUser(ctx)
	opts := PermissionOpts{
		ResourceType:  rp.opts.ResourceType,
		Permission:    rp.opts.PermissionKey,
		RequiredLevel: rp.opts.RequiredLevel,
	}
	if user == nil {
		for _, item := range resources {
			opts.Resource = item
			if rp.opts.DefaultRoles != nil {
				if !HasPermission(rp.opts.RM, opts, rp.opts.DefaultRoles) {
					http.Error(rw, "not authorized for this action", http.StatusUnauthorized)
					return false
				}
				return true
			}
			http.Error(rw, "no user found", http.StatusUnauthorized)
			return false
		}
		return true
	}

	authenticator := GetAuthenticator(ctx)
	if authenticator == nil {
		http.Error(rw, "unable to determine an authenticator", http.StatusInternalServerError)
		return false
	}
	if !authenticator.CheckAuthenticated(user) {
		http.Error(rw, "not authenticated", http.StatusUnauthorized)
		return false
	}
	for _, item := range resources {
		opts.Resource = item
		if !user.HasPermission(opts) {
			http.Error(rw, "not authorized for this action", http.StatusUnauthorized)
			return false
		}
	}
	return true
}
