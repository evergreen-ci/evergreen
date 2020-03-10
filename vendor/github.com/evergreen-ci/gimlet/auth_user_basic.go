package gimlet

import (
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

type BasicUserOptions struct {
	id           string
	name         string
	email        string
	password     string
	key          string
	accessToken  string
	refreshToken string
	roles        []string
	roleManager  RoleManager
}

func NewBasicUserOptions(id string) (BasicUserOptions, error) {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(id == "", "ID must not be empty")
	if catcher.HasErrors() {
		return BasicUserOptions{}, catcher.Resolve()
	}
	return BasicUserOptions{
		id: id,
	}, nil
}

func (opts BasicUserOptions) Name(name string) BasicUserOptions {
	opts.name = name
	return opts
}

func (opts BasicUserOptions) Email(email string) BasicUserOptions {
	opts.email = email
	return opts
}

func (opts BasicUserOptions) Password(password string) BasicUserOptions {
	opts.password = password
	return opts
}

func (opts BasicUserOptions) Key(key string) BasicUserOptions {
	opts.key = key
	return opts
}

func (opts BasicUserOptions) AccessToken(token string) BasicUserOptions {
	opts.accessToken = token
	return opts
}

func (opts BasicUserOptions) RefreshToken(token string) BasicUserOptions {
	opts.refreshToken = token
	return opts
}

func (opts BasicUserOptions) Roles(roles ...string) BasicUserOptions {
	opts.roles = roles
	return opts
}

func (opts BasicUserOptions) RoleManager(rm RoleManager) BasicUserOptions {
	opts.roleManager = rm
	return opts
}

// NewBasicUser constructs a simple user. The underlying type has
// serialization tags.
func NewBasicUser(opts BasicUserOptions) *BasicUser {
	return &BasicUser{
		ID:           opts.id,
		Name:         opts.name,
		EmailAddress: opts.email,
		Password:     opts.password,
		Key:          opts.key,
		AccessToken:  opts.accessToken,
		RefreshToken: opts.refreshToken,
		AccessRoles:  opts.roles,
		roleManager:  opts.roleManager,
	}
}

type BasicUser struct {
	ID           string   `bson:"_id" json:"id" yaml:"id"`
	Name         string   `bson:"name" json:"name" yaml:"name"`
	EmailAddress string   `bson:"email" json:"email" yaml:"email"`
	Password     string   `bson:"password" json:"password" yaml:"password"`
	Key          string   `bson:"key" json:"key" yaml:"key"`
	AccessToken  string   `bson:"access_token" json:"access_token" yaml:"access_token"`
	RefreshToken string   `bson:"refresh_token" json:"refresh_token" yaml:"refresh_token"`
	AccessRoles  []string `bson:"roles" json:"roles" yaml:"roles"`
	roleManager  RoleManager
	invalid      bool
}

func (u *BasicUser) Username() string        { return u.ID }
func (u *BasicUser) Email() string           { return u.EmailAddress }
func (u *BasicUser) DisplayName() string     { return u.Name }
func (u *BasicUser) GetAPIKey() string       { return u.Key }
func (u *BasicUser) GetAccessToken() string  { return u.AccessToken }
func (u *BasicUser) GetRefreshToken() string { return u.RefreshToken }
func (u *BasicUser) Roles() []string {
	out := make([]string, len(u.AccessRoles))
	copy(out, u.AccessRoles)
	return out
}
func (u *BasicUser) HasPermission(opts PermissionOpts) bool {
	if u.roleManager == nil {
		return false
	}
	roles, err := u.roleManager.GetRoles(u.Roles())
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error getting roles",
		}))
		return false
	}
	return HasPermission(u.roleManager, opts, roles)
}

// userHasRole determines if the user has the defined role.
func userHasRole(u User, role string) bool {
	for _, r := range u.Roles() {
		if r == role {
			return true
		}
	}

	return false
}

func userInSlice(u User, users []string) bool {
	if len(users) == 0 {
		return false
	}

	id := u.Username()
	for _, u := range users {
		if id == u {
			return true
		}
	}

	return false
}
