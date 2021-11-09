package gimlet

import (
	"context"
	"net/http"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

// User provides a common way of interacting with users from
// authentication systems.
//
// Note: this is the User interface from Evergreen with the addition
// of the Roles() method.
type User interface {
	DisplayName() string
	Email() string
	Username() string
	GetAPIKey() string
	GetAccessToken() string
	GetRefreshToken() string
	Roles() []string
	HasPermission(PermissionOpts) bool
}

// PermissionOpts is the required data to be provided when asking if a user has permission for a resource
type PermissionOpts struct {
	Resource      string
	ResourceType  string
	Permission    string
	RequiredLevel int
}

type Permissions map[string]int

// Authenticator represents a service that answers specific
// authentication related questions, and is the public interface
// used for authentication workflows.
type Authenticator interface {
	CheckResourceAccess(User, string) bool
	CheckGroupAccess(User, string) bool
	CheckAuthenticated(User) bool
}

// UserManager sets and gets user tokens for implemented
// authentication mechanisms, and provides the data that is sent by
// the api and ui server after authenticating
type UserManager interface {
	// The first 5 methods are borrowed directly from evergreen without modification
	GetUserByToken(context.Context, string) (User, error)
	CreateUserToken(string, string) (string, error)
	// GetLoginHandler returns the function that starts the login process for auth mechanisms
	// that redirect to a thirdparty site for authentication
	GetLoginHandler(url string) http.HandlerFunc
	// GetLoginRedirectHandler returns the function that does login for the
	// user once it has been redirected from a thirdparty site.
	GetLoginCallbackHandler() http.HandlerFunc

	// IsRedirect returns true if the user must be redirected to a
	// thirdparty site to authenticate.
	// TODO: should add a "do redirect if needed".
	IsRedirect() bool

	// ReauthorizeUser reauthorizes a user that is already logged in.
	ReauthorizeUser(User) error

	// These methods are simple wrappers around the user
	// persistence layer. May consider moving them to the
	// authenticator.
	GetUserByID(string) (User, error)
	GetOrCreateUser(User) (User, error)

	// Log out user or all users
	ClearUser(user User, all bool) error

	// Returns the groups or roles to which a user belongs
	GetGroupsForUser(string) ([]string, error)
}

// RoleManager provides methods to get and set role data and is often used
// along with Users to check permissions
type RoleManager interface {
	// GetAllRoles returns all roles known by the manager
	GetAllRoles() ([]Role, error)

	// GetAllRoles returns roles matching the specified IDs
	GetRoles([]string) ([]Role, error)

	// DeleteRole deletes a single role
	DeleteRole(string) error

	// UpdateRole adds the given role to the manager if it does not exist, or updates the role
	// with the same ID
	UpdateRole(Role) error

	// FilterForResource takes a list of roles and returns the subset of those applicable for a certain resource
	FilterForResource([]Role, string, string) ([]Role, error)

	// FilterScopesByResourceType takes a list of scope IDs and a resource
	// type and returns a list of scopes filtered by the resource type.
	FilterScopesByResourceType([]string, string) ([]Scope, error)

	// FindScopeForResources returns a scope that exactly matches a given set of resources
	FindScopeForResources(string, ...string) (*Scope, error)

	// AddScope adds a scope to the manager
	AddScope(Scope) error

	// DeleteScope removes a scope from the manager
	DeleteScope(Scope) error

	// GetScope returns the given scope
	GetScope(context.Context, string) (*Scope, error)

	// AddResourceToScope adds the specified resource to the given scope, updating parents
	AddResourceToScope(string, string) error

	// RemoveResourceFromScope deletes the specified resource from the given scope, updating parents
	RemoveResourceFromScope(string, string) error

	// RegisterPermissions adds a list of strings to the role manager as valid permission keys. Returns an
	// error if the same permission is registered more than once
	RegisterPermissions([]string) error

	// FindRoleWithResources returns all roles that apply to exactly the given resources
	FindRolesWithResources(string, []string) ([]Role, error)

	// FindRolesWithPermissions returns a role that exactly matches the given resources and permissions
	FindRoleWithPermissions(string, []string, Permissions) (*Role, error)

	// Clear deletes all roles and scopes. This should only be used in tests
	Clear() error

	// IsValidPermissions checks if the passed permissions are registered
	IsValidPermissions(Permissions) error
}

func HasPermission(rm RoleManager, opts PermissionOpts, roles []Role) bool {
	roles, err := rm.FilterForResource(roles, opts.Resource, opts.ResourceType)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error filtering for resource",
		}))
		return false
	}

	for _, role := range roles {
		level, hasPermission := role.Permissions[opts.Permission]
		if hasPermission && level >= opts.RequiredLevel {
			return true
		}
	}
	return false
}
