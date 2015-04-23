package thirdparty

import (
	"10gen.com/mci/testutils"
	"10gen.com/mci/util"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestGetUser(t *testing.T) {
	testutils.ConfigureIntegrationTest(t, testConfig, "TestGetUser")
	Convey("A crowd REST service with valid credentials", t, func() {
		crowdRest, err := NewRESTCrowdService(testConfig.Crowd.Username, testConfig.Crowd.Password, testConfig.Crowd.Urlroot)
		util.HandleTestingErr(err, t, "Could not construct crowd REST service")

		Convey("should be able to fetch a user and receive valid data", func() {
			testUserId := "mpobrien"
			user, err := crowdRest.GetUser(testUserId)
			So(err, ShouldBeNil)
			So(user, ShouldNotBeNil)
			So(user.Name, ShouldEqual, testUserId)
		})
	})
}
