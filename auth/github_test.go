package auth

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	. "github.com/smartystreets/goconvey/convey"
)

func TestGithubAuthManager(t *testing.T) {
	Convey("When creating a UserManager with a GitHub authentication", t, func() {
		g := evergreen.GithubAuthConfig{
			ClientId:     "foo",
			ClientSecret: "bar",
			Organization: "org",
			Users:        []string{"foobar"},
		}
		Convey("user manager should have functins for Login and LoginCallback handlers", func() {
			authConfig := evergreen.AuthConfig{
				Github: &g,
			}
			userManager, info, err := LoadUserManager(&evergreen.Settings{AuthConfig: authConfig})
			So(err, ShouldBeNil)
			So(info.CanClearTokens, ShouldBeFalse)
			So(info.CanReauthorize, ShouldBeFalse)
			So(userManager.GetLoginHandler(""), ShouldNotBeNil)
			So(userManager.GetLoginCallbackHandler(), ShouldNotBeNil)
		})
	})
}
