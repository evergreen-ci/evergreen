package ec2

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(evergreen.TestConfig()))
}

func TestSpawnSpotInstance(t *testing.T) {
	testConfig = evergreen.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig, "TestSpawnSpotInstance")

	provider := &EC2SpotManager{}
	provider.Configure(testConfig)

	Convey("When spawning many hosts", t, func() {

		testutil.HandleTestingErr(
			db.ClearCollections(host.Collection), t, "error clearing test collections")

		hosts := make([]*host.Host, 1)

		hostOptions := cloud.HostOptions{
			UserName: evergreen.User,
			UserHost: false,
		}
		d := fetchTestDistro()
		for i := range hosts {
			h, err := provider.SpawnInstance(d, hostOptions)
			hosts[i] = h
			So(err, ShouldBeNil)
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

func fetchTestDistro() *distro.Distro {
	return &distro.Distro{
		Id:       "test_distro",
		Arch:     "linux_amd64",
		WorkDir:  "/data/mci",
		PoolSize: 10,
		Provider: SpotProviderName,
		ProviderSettings: &map[string]interface{}{
			"ami":            "ami-c7e7f2d0",
			"instance_type":  "t1.micro",
			"key_name":       "mci",
			"bid_price":      .004,
			"security_group": "default",
		},

		SetupAsSudo: true,
		Setup:       "",
		Teardown:    "",
		User:        "root",
		SSHKey:      "",
	}
}
