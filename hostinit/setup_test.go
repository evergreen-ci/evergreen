package hostinit

import (
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/cloud/providers"
	"github.com/evergreen-ci/evergreen/cloud/providers/mock"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testutil.TestConfig()))
}

func TestSetupReadyHosts(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testutil.TestConfig(), "TestSetupReadyHosts")

	hostInit := &HostInit{
		testutil.TestConfig(),
	}

	Convey("When hosts are spawned but not running", t, func() {
		testutil.HandleTestingErr(
			db.ClearCollections(host.Collection), t, "error clearing test collections")
		mock.Clear()

		hostsForTest := make([]host.Host, 10)
		for i := 0; i < 10; i++ {
			mockHost, err := spawnMockHost()
			So(err, ShouldBeNil)
			hostsForTest[i] = *mockHost
		}
		So(len(mock.MockInstances), ShouldEqual, 10)

		for i := range hostsForTest {
			h := hostsForTest[i]
			So(h.Status, ShouldNotEqual, evergreen.HostRunning)
		}
		Convey("and all of the hosts have failed", func() {
			for id := range mock.MockInstances {
				instance := mock.MockInstances[id]
				instance.Status = cloud.StatusFailed
				instance.DNSName = "dnsName"
				instance.IsSSHReachable = true
				mock.MockInstances[id] = instance
			}
			Convey("when running setup", func() {
				err := hostInit.setupReadyHosts()
				So(err, ShouldBeNil)

				Convey("then all of the hosts should be terminated", func() {
					for id := range mock.MockInstances {
						instance := mock.MockInstances[id]
						So(instance.Status, ShouldEqual, cloud.StatusTerminated)
					}
					for i := range hostsForTest {
						h := hostsForTest[i]
						dbHost, err := host.FindOne(host.ById(h.Id))
						So(err, ShouldBeNil)
						So(dbHost.Status, ShouldEqual, evergreen.HostTerminated)
					}
				})
			})
		})

		Convey("and all of the hosts are ready with properly set fields", func() {
			for id := range mock.MockInstances {
				instance := mock.MockInstances[id]
				instance.Status = cloud.StatusRunning
				instance.DNSName = "dnsName"
				instance.IsSSHReachable = true
				mock.MockInstances[id] = instance
			}
			Convey("when running setup", func() {
				err := hostInit.setupReadyHosts()
				So(err, ShouldBeNil)

				Convey("then all of the 'OnUp' functions should have been run and "+
					"host should have been marked as provisioned", func() {
					for id := range mock.MockInstances {
						instance := mock.MockInstances[id]
						So(instance.OnUpRan, ShouldBeTrue)
					}
					for i := range hostsForTest {
						h := hostsForTest[i]
						dbHost, err := host.FindOne(host.ById(h.Id))
						So(err, ShouldBeNil)
						So(dbHost.Status, ShouldEqual, evergreen.HostRunning)
					}
				})
			})
		})
	})

}

func TestHostIsReady(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testutil.TestConfig(), "TestHostIsReady")

	hostInit := &HostInit{
		testutil.TestConfig(),
	}

	Convey("When hosts are spawned", t, func() {
		testutil.HandleTestingErr(
			db.ClearCollections(host.Collection), t, "error clearing test collections")
		mock.Clear()

		hostsForTest := make([]host.Host, 10)
		// Spawn 10 hosts
		for i := 0; i < 10; i++ {
			mockHost, err := spawnMockHost()
			So(err, ShouldBeNil)
			hostsForTest[i] = *mockHost
		}
		So(len(mock.MockInstances), ShouldEqual, 10)

		Convey("and none of the hosts are ready", func() {
			for id := range mock.MockInstances {
				instance := mock.MockInstances[id]
				instance.Status = cloud.StatusInitializing
				mock.MockInstances[id] = instance
			}

			Convey("then checking for readiness should return false", func() {
				for i := range hostsForTest {
					h := hostsForTest[i]
					ready, err := hostInit.IsHostReady(&h)
					So(err, ShouldBeNil)
					So(ready, ShouldBeFalse)
				}

			})
		})
		Convey("and all of the hosts are ready", func() {
			for id := range mock.MockInstances {
				instance := mock.MockInstances[id]
				instance.Status = cloud.StatusRunning
				mock.MockInstances[id] = instance
			}
			Convey("and all of the hosts fields are properly set", func() {
				for id := range mock.MockInstances {
					instance := mock.MockInstances[id]
					instance.DNSName = "dnsName"
					instance.IsSSHReachable = true
					mock.MockInstances[id] = instance
				}
				Convey("then checking for readiness should return true", func() {
					for i := range hostsForTest {
						h := hostsForTest[i]
						ready, err := hostInit.IsHostReady(&h)
						So(err, ShouldBeNil)
						So(ready, ShouldBeTrue)
					}

				})
			})
			Convey("and dns is not set", func() {
				for id := range mock.MockInstances {
					instance := mock.MockInstances[id]
					instance.IsSSHReachable = true
					mock.MockInstances[id] = instance
				}
				Convey("then checking for readiness should error", func() {
					for i := range hostsForTest {
						h := hostsForTest[i]
						ready, err := hostInit.IsHostReady(&h)
						So(err, ShouldNotBeNil)
						So(ready, ShouldBeFalse)
					}

				})
			})
		})
		Convey("and all of the hosts failed", func() {
			for id := range mock.MockInstances {
				instance := mock.MockInstances[id]
				instance.Status = cloud.StatusFailed
				mock.MockInstances[id] = instance
			}
			Convey("then checking for readiness should terminate", func() {
				for i := range hostsForTest {
					h := hostsForTest[i]
					ready, err := hostInit.IsHostReady(&h)
					So(err, ShouldNotBeNil)
					So(ready, ShouldBeFalse)
					So(h.Status, ShouldEqual, evergreen.HostTerminated)
				}
				for _, instance := range mock.MockInstances {
					So(instance.Status, ShouldEqual, cloud.StatusTerminated)
				}
			})
		})
	})

}

func spawnMockHost() (*host.Host, error) {
	mockDistro := distro.Distro{
		Id:       "mock_distro",
		Arch:     "mock_arch",
		WorkDir:  "src",
		PoolSize: 10,
		Provider: mock.ProviderName,
	}
	hostOptions := cloud.HostOptions{
		UserName: evergreen.User,
		UserHost: false,
	}
	cloudManager, err := providers.GetCloudManager(mock.ProviderName, testutil.TestConfig())
	if err != nil {
		return nil, err
	}

	newHost, err := cloudManager.SpawnInstance(&mockDistro, hostOptions)
	if err != nil {
		return nil, fmt.Errorf("Error spawning instance: %v,", err)
	}

	return newHost, nil
}
