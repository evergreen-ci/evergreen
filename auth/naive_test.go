package auth

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	. "github.com/smartystreets/goconvey/convey"
)

func TestNaiveAuthManager(t *testing.T) {
	Convey("When creating a UserManager with a Naive authentication", t, func() {
		n := evergreen.NaiveAuthConfig{}
		authConfig := evergreen.AuthConfig{
			Naive: &n,
		}
		userManager, info, err := LoadUserManager(&evergreen.Settings{AuthConfig: authConfig})
		So(err, ShouldBeNil)
		So(info.CanClearTokens, ShouldBeFalse)
		So(info.CanReauthorize, ShouldBeFalse)
		Convey("user manager should have nil functions for Login and LoginCallback handlers", func() {
			So(userManager.GetLoginHandler(""), ShouldBeNil)
			So(userManager.GetLoginCallbackHandler(), ShouldBeNil)
		})
	})
}
