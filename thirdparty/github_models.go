package thirdparty

import "github.com/pkg/errors"

type GithubLoginUser struct {
	Login            string
	Id               int
	Company          string
	EmailAddress     string `json:"email"`
	Name             string
	OrganizationsURL string
}

func (u *GithubLoginUser) DisplayName() string { return u.Name }
func (u *GithubLoginUser) Email() string       { return u.EmailAddress }
func (u *GithubLoginUser) Username() string    { return u.Login }
func (u *GithubLoginUser) IsNil() bool         { return u == nil }
func (u *GithubLoginUser) GetAPIKey() string   { return "" }
func (u *GithubLoginUser) Roles() []string     { return []string{} }

func (u *GithubLoginUser) HasPermission(string, string, int) (bool, error) {
	return false, errors.New("HasPermission has not been implemented for GithubLoginUser")
}

type GithubAuthParameters struct {
	ClientId     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	Code         string `json:"code"`
	RedirectUri  string `json:"redirect_uri"`
	State        string `json:"state"`
}

type GithubAuthResponse struct {
	AccessToken string `json:"access_token"`
	Scope       string `json:"scope"`
	TokenType   string `json:"token_type"`
}
