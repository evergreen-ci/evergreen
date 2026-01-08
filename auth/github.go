package auth

import (
	"context"
	"crypto/md5"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// GithubAuthManager implements the UserManager with GitHub authentication using Oauth authentication.
// The process starts off with a redirect GET request to GitHub sent with the application's ClientId,
// the CallbackURI of the application where Github should redirect the user to after authenticating with username/password,
// the scope (the email and organization information of the user the application uses to authorize the user,
// and an unguessable State string. The state string is concatenation of a timestamp and a hash of the timestamp
// and the Salt field in GithubUserManager.
// After authenticating the User, GitHub redirects the user back to the CallbackURI given with a code parameter
// and the unguessable State string. The application checks that the State strings are the same by reproducing
// the state string using the Salt and the timestamp that is in plain-text before the hash and checking to make sure
// that they are the same.
// The application sends the code back in a POST with the ClientId and ClientSecret and receives a response that has the
// accessToken used to get the user's information. The application stores the accessToken in a session cookie.
// Whenever GetUserByToken is called, the application sends the token to GitHub, gets the user's login username and organization
// and ensures that the user is either in an Authorized organization or an Authorized user.

type GithubUserManager struct {
	ClientId               string
	ClientSecret           string
	AuthorizedUsers        []string
	AuthorizedOrganization string
	Salt                   string
	LoginDomain            string
}

// NewGithubUserManager initializes a GithubUserManager with a Salt as randomly generated string used in Github
// authentication
func NewGithubUserManager(g *evergreen.GithubAuthConfig, loginDomain string) (gimlet.UserManager, error) {
	if g.ClientId == "" {
		return nil, errors.New("no client id for config")
	}
	if g.ClientSecret == "" {
		return nil, errors.New("no client secret for config given")
	}
	return &GithubUserManager{
		ClientId:               g.ClientId,
		ClientSecret:           g.ClientSecret,
		AuthorizedUsers:        g.Users,
		AuthorizedOrganization: g.Organization,
		Salt:                   utility.RandomString(),
		LoginDomain:            loginDomain,
	}, nil
}

// GetUserByToken sends the token to Github and gets back a user and optionally an organization.
// If there are Authorized Users, it checks the authorized usernames against the GitHub user's login
// If there is no match and there is an organization it checks the user's organizations against
// the UserManager's Authorized organization string.
func (gum *GithubUserManager) GetUserByToken(ctx context.Context, token string) (gimlet.User, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	user, isMember, err := thirdparty.GetGithubTokenUser(ctx, token, gum.AuthorizedOrganization)
	if err != nil {
		return nil, err
	}
	if user != nil {
		if !isMember {
			if gum.AuthorizedUsers != nil {
				for _, u := range gum.AuthorizedUsers {
					if u == user.Username() {
						return user, nil
					}
				}
			}

		} else {
			return user, nil
		}
	}

	return nil, errors.New("No authorized user or organization given")
}

// CreateUserToken is not implemented in GithubUserManager
func (*GithubUserManager) CreateUserToken(string, string) (string, error) {
	return "", errors.New("GithubUserManager does not create tokens via username/password")
}

// GetLoginHandler returns the function that starts oauth by redirecting the user to authenticate with Github
func (gum *GithubUserManager) GetLoginHandler(callbackUri string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		githubScope := "user:email, read:org"
		githubUrl := "https://github.com/login/oauth/authorize"
		timestamp := time.Now().String()
		// create a combination of the current time and the config's salt to hash as the unguessable string
		githubState := fmt.Sprintf("%v%x", timestamp, md5.Sum([]byte(timestamp+gum.Salt)))
		parameters := url.Values{}
		parameters.Set("client_id", gum.ClientId)
		parameters.Set("redirect_uri", fmt.Sprintf("%v/login/redirect/callback?%v", callbackUri, r.URL.RawQuery))
		parameters.Set("scope", githubScope)
		parameters.Set("state", githubState)
		http.Redirect(w, r, fmt.Sprintf("%v?%v", githubUrl, parameters.Encode()), http.StatusFound)
	}
}

// GetLoginCallbackHandler returns the function that is called when GitHub redirects the user back to Evergreen.
func (gum *GithubUserManager) GetLoginCallbackHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := r.FormValue("code")
		if code == "" {
			grip.Error("Error getting code from github for authentication")
			return
		}
		githubState := r.FormValue("state")
		if githubState == "" {
			grip.Error("Error getting state from github for authentication")
			return
		}
		// if there is an internal redirect page, redirect the user back to that page
		// otherwise redirect the user back to the home page
		redirect := r.FormValue("redirect")
		if redirect == "" {
			redirect = "/"
		} else {
			// validate that redirect is a local path to avoid open redirects
			normalized := strings.ReplaceAll(redirect, "\\", "/")
			u, err := url.Parse(normalized)
			if err != nil {
				grip.Errorf("invalid redirect parameter %q: %+v", redirect, err)
				redirect = "/"
			} else if u.Scheme != "" || u.Hostname() != "" || !strings.HasPrefix(u.Path, "/") {
				// disallow absolute or external URLs; require an absolute path within this host
				grip.Errorf("disallowed redirect parameter %q (scheme=%q host=%q path=%q)", redirect, u.Scheme, u.Hostname(), u.Path)
				redirect = "/"
			} else {
				redirect = u.String()
			}
		}
		// create the state from the timestamp and Salt and check against the one GitHub sent back
		timestamp := githubState[:len(time.Now().String())]
		state := fmt.Sprintf("%v%x", timestamp, md5.Sum([]byte(timestamp+gum.Salt)))

		// if the state doesn't match, log the error and redirect back to the login page
		if githubState != state {
			grip.Errorf("Error unmatching states when authenticating with GitHub: ours: %vb, theirs %v",
				state, githubState)
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		githubResponse, err := thirdparty.GithubAuthenticate(ctx, code, gum.ClientId, gum.ClientSecret)
		if err != nil {
			grip.Errorf("Error sending code and authentication info to GitHub: %+v", err)
			return
		}
		SetLoginToken(githubResponse.AccessToken, gum.LoginDomain, w)
		http.Redirect(w, r, redirect, http.StatusFound)
	}
}

func (*GithubUserManager) IsRedirect() bool                           { return true }
func (*GithubUserManager) ReauthorizeUser(gimlet.User) error          { return errors.New("not implemented") }
func (*GithubUserManager) GetUserByID(id string) (gimlet.User, error) { return getUserByID(id) }
func (*GithubUserManager) GetOrCreateUser(u gimlet.User) (gimlet.User, error) {
	return getOrCreateUser(u)
}
func (*GithubUserManager) ClearUser(u gimlet.User, all bool) error {
	return errors.New("GitHub Authentication does not support Clear User")
}
func (*GithubUserManager) GetGroupsForUser(string) ([]string, error) {
	return nil, errors.New("GetGroupsForUser has not yet been implemented for the GitHub user manager")
}
