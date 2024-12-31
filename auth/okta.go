package auth

import (
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/gimlet/okta"
	"github.com/evergreen-ci/gimlet/usercache"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

func NewOktaUserManager(conf *evergreen.OktaConfig, evgURL, loginDomain string) (gimlet.UserManager, error) {
	expireAfter := time.Duration(conf.ExpireAfterMinutes) * time.Minute
	opts := okta.CreationOptions{
		ClientID:             conf.ClientID,
		ClientSecret:         conf.ClientSecret,
		RedirectURI:          strings.TrimRight(evgURL, "/") + "/login/redirect/callback",
		Issuer:               conf.Issuer,
		Scopes:               conf.Scopes,
		UserGroup:            conf.UserGroup,
		CookiePath:           "/",
		CookieDomain:         loginDomain,
		LoginCookieName:      evergreen.AuthTokenCookie,
		LoginCookieTTL:       evergreen.LoginCookieTTL,
		AllowReauthorization: true,
		ReconciliateID: func(id string) string {
			emailDomainStart := strings.LastIndex(id, "@")
			if emailDomainStart == -1 {
				return id
			}
			return id[:emailDomainStart]
		},
		GetHTTPClient: utility.GetHTTPClient,
		PutHTTPClient: utility.PutHTTPClient,
		ExternalCache: &usercache.ExternalOptions{
			PutUserGetToken: user.PutLoginCache,
			GetUserByToken:  func(token string) (gimlet.User, bool, error) { return user.GetLoginCache(token, expireAfter) },
			ClearUserToken: func(u gimlet.User, all bool) error {
				if all {
					return user.ClearAllLoginCaches()
				}
				return user.ClearLoginCache(u)
			},
			GetUserByID:     func(id string) (gimlet.User, bool, error) { return getUserByIdWithExpiration(id, expireAfter) },
			GetOrCreateUser: getOrCreateUser,
		},
	}
	um, err := okta.NewUserManager(opts)
	if err != nil {
		return nil, errors.Wrap(err, "could not construct Okta user manager")
	}
	return um, nil
}
