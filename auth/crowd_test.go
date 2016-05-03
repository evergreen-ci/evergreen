package auth

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	. "github.com/smartystreets/goconvey/convey"
)

func TestCrowdAuthManager(t *testing.T) {
	Convey("When creating a UserManager with Crowd authentication", t, func() {
		c := evergreen.CrowdConfig{
			Username: "",
			Password: "",
			Urlroot:  "",
		}
		authConfig := evergreen.AuthConfig{
			Crowd:  &c,
			Naive:  nil,
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
