package cloud

import (
	"testing"

	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

func TestGetCloudManager(t *testing.T) {
	Convey("GetCloudManager() should return non-nil for all valid provider names", t, func() {

		Convey("EC2Auto should be returned for ec2-auto provider name", func() {
			cloudMgr, err := GetCloudManager("ec2-auto", testutil.TestConfig())
			So(cloudMgr, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(cloudMgr, ShouldHaveSameTypeAs, &ec2Manager{})
		})

		Convey("EC2Spot should be returned for ec2-spot provider name", func() {
			cloudMgr, err := GetCloudManager("ec2-spot", testutil.TestConfig())
			So(cloudMgr, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(cloudMgr, ShouldHaveSameTypeAs, &ec2Manager{})
		})

		Convey("EC2 should be returned for ec2 provider name", func() {
			cloudMgr, err := GetCloudManager("ec2-ondemand", testutil.TestConfig())
			So(cloudMgr, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(cloudMgr, ShouldHaveSameTypeAs, &ec2Manager{})
		})

		Convey("Static should be returned for static provider name", func() {
			cloudMgr, err := GetCloudManager("static", testutil.TestConfig())
			So(cloudMgr, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(cloudMgr, ShouldHaveSameTypeAs, &staticManager{})
		})

		Convey("Mock should be returned for mock provider name", func() {
			cloudMgr, err := GetCloudManager("mock", testutil.TestConfig())
			So(cloudMgr, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(cloudMgr, ShouldHaveSameTypeAs, &mockManager{})
		})

		Convey("Invalid provider names should return nil with err", func() {
			cloudMgr, err := GetCloudManager("bogus", testutil.TestConfig())
			So(cloudMgr, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})

	})

}
