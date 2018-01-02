package cloud

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

func init() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func TestSpawnSpotInstance(t *testing.T) {
	testConfig = testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig, "TestSpawnSpotInstance")

	provider := &ec2SpotManager{}
	testutil.HandleTestingErr(provider.Configure(testConfig), t, "error configuring provider")

	Convey("When spawning many hosts", t, func() {

		testutil.HandleTestingErr(
			db.ClearCollections(host.Collection), t, "error clearing test collections")

		hosts := make([]*host.Host, 1)

		hostOptions := HostOptions{
			UserName: evergreen.User,
			UserHost: false,
		}
		d := fetchTestDistro()
		for i := range hosts {
			h := NewIntent(*d, provider.GetInstanceName(d), d.Provider, hostOptions)
			h, err := provider.SpawnHost(h)
			hosts[i] = h
			So(err, ShouldBeNil)
			So(h.Insert(), ShouldBeNil)
		}
		Convey("and terminating all of them", func() {
			foundHosts, err := host.Find(host.IsUninitialized)
			So(err, ShouldBeNil)
			So(len(foundHosts), ShouldEqual, 1)
			for _, h := range foundHosts {
				err := provider.TerminateInstance(&h)
				So(err, ShouldBeNil)
			}
			for _, h := range hosts {
				err := provider.TerminateInstance(h)
				So(err, ShouldBeNil)
			}
		})
	})

}
