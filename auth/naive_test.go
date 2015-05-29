package auth

import (
	"github.com/evergreen-ci/evergreen"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestNaiveAuthManager(t *testing.T) {
	Convey("When creating a UserManager with a Naive authentication", t, func() {
		n := evergreen.NaiveAuthConfig{
			Users: nil,
		}
		authConfig := evergreen.AuthConfig{
			Crowd:  nil,
			Naive:  &n,
			Github: nil,
		}
		userManager, err := LoadUserManager(authConfig)
		So(err, ShouldBeNil)
		Convey("user manager should have nil functions for Login and LoginCallback handlers", func() {
			So(userManager.GetLoginHandler(""), ShouldBeNil)
			So(userManager.GetLoginCallbackHandler(), ShouldBeNil)
		})
	})
}
