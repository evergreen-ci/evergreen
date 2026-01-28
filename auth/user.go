package auth

import (
	"context"

	"github.com/evergreen-ci/gimlet"
)

type simpleUser struct {
	UserId       string
	Name         string
	EmailAddress string
	APIKey       string
	SiteRoles    []string
}

func (u *simpleUser) DisplayName() string                                       { return u.Name }
func (u *simpleUser) Email() string                                             { return u.EmailAddress }
func (u *simpleUser) Username() string                                          { return u.UserId }
func (u *simpleUser) IsNil() bool                                               { return u == nil }
func (u *simpleUser) GetAPIKey() string                                         { return u.APIKey }
func (u *simpleUser) GetAccessToken() string                                    { return "" }
func (u *simpleUser) GetRefreshToken() string                                   { return "" }
func (u *simpleUser) Roles() []string                                           { out := []string{}; copy(out, u.SiteRoles); return out }
func (u *simpleUser) HasPermission(context.Context, gimlet.PermissionOpts) bool { return true }
func (u *simpleUser) IsAPIOnly() bool                                           { return false }
