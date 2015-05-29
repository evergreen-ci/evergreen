package auth

import (
	"github.com/evergreen-ci/evergreen"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestGithubAuthManager(t *testing.T) {
	Convey("When creating a UserManager with a GitHub authentication", t, func() {
		g := evergreen.GithubAuthConfig{
			ClientId:     "",
			ClientSecret: "",
			Users:        nil,
			Organization: "",
		}
		Convey("user manager should have functins for Login and LoginCallback handlers", func() {
			authConfig := evergreen.AuthConfig{nil, nil, &g}
			userManager, err := LoadUserManager(authConfig)
			So(err, ShouldBeNil)
			So(userManager.GetLoginHandler(""), ShouldNotBeNil)
			So(userManager.GetLoginCallbackHandler(), ShouldNotBeNil)
		})
	})
}
