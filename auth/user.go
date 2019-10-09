package auth

type simpleUser struct {
	UserId       string
	Name         string
	EmailAddress string
	APIKey       string
	SiteRoles    []string
}

func (u *simpleUser) DisplayName() string                             { return u.Name }
func (u *simpleUser) Email() string                                   { return u.EmailAddress }
func (u *simpleUser) Username() string                                { return u.UserId }
func (u *simpleUser) IsNil() bool                                     { return u == nil }
func (u *simpleUser) GetAPIKey() string                               { return u.APIKey }
func (u *simpleUser) Roles() []string                                 { out := []string{}; copy(out, u.SiteRoles); return out }
func (u *simpleUser) HasPermission(string, string, int) (bool, error) { return true, nil }
