package providers

import (
	"10gen.com/mci"
	"10gen.com/mci/cloud/providers/digitalocean"
	"10gen.com/mci/cloud/providers/ec2"
	"10gen.com/mci/cloud/providers/mock"
	"10gen.com/mci/cloud/providers/static"
	"10gen.com/mci/model/host"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

func TestGetCloudManager(t *testing.T) {
	Convey("GetCloudManager() should return non-nil for all valid provider names", t, func() {

		Convey("EC2 should be returned for ec2 provider name", func() {
			cloudMgr, err := GetCloudManager("ec2", mci.TestConfig())
			So(cloudMgr, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(cloudMgr, ShouldHaveSameTypeAs, &ec2.EC2Manager{})
		})

		Convey("EC2Spot should be returned for ec2-spot provider name", func() {
			cloudMgr, err := GetCloudManager("ec2-spot", mci.TestConfig())
			So(cloudMgr, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(cloudMgr, ShouldHaveSameTypeAs, &ec2.EC2SpotManager{})
		})

		Convey("Static should be returned for static provider name", func() {
			cloudMgr, err := GetCloudManager("static", mci.TestConfig())
			So(cloudMgr, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(cloudMgr, ShouldHaveSameTypeAs, &static.StaticManager{})
		})

		Convey("Mock should be returned for mock provider name", func() {
			cloudMgr, err := GetCloudManager("mock", mci.TestConfig())
			So(cloudMgr, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(cloudMgr, ShouldHaveSameTypeAs, &mock.MockCloudManager{})
		})

		Convey("DigitalOcean should be returned for digitalocean provider name", func() {
			cloudMgr, err := GetCloudManager("digitalocean", mci.TestConfig())
			So(cloudMgr, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(cloudMgr, ShouldHaveSameTypeAs, &digitalocean.DigitalOceanManager{})
		})

		Convey("Invalid provider names should return nil with err", func() {
			cloudMgr, err := GetCloudManager("bogus", mci.TestConfig())
			So(cloudMgr, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})

	})

}

func TestIsHostReachable(t *testing.T) {
	Convey("A reachable static host should return true", t, func() {
		// try with a reachable static host
		reachableHost := &host.Host{
			Host:        "localhost",
			Provisioned: true,
			Provider:    mci.HostTypeStatic,
		}
		cloudManager, err := GetCloudManager(reachableHost.Provider, mci.TestConfig())
		So(err, ShouldBeNil)

		reachable, err := cloudManager.IsSSHReachable(reachableHost, "")
		So(reachable, ShouldBeTrue)
		So(err, ShouldBeNil)
	})
	Convey("An unreachable static host should return false", t, func() {
		// try with an unreachable static host
		reachableHost := &host.Host{
			Host:        "fakehost",
			Provisioned: true,
			Provider:    mci.HostTypeStatic,
		}
		cloudManager, err := GetCloudManager(reachableHost.Provider, mci.TestConfig())
		So(err, ShouldBeNil)

		reachable, err := cloudManager.IsSSHReachable(reachableHost, "")
		So(reachable, ShouldBeFalse)
		So(err, ShouldBeNil)
	})

}
