package hostinit

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/smartystreets/goconvey/convey/reporting"
)

func init() {
	reporting.QuietMode()
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func startHosts(ctx context.Context, settings *evergreen.Settings) error {
	hostsToStart, err := host.Find(host.IsUninitialized)
	if err != nil {
		return errors.Wrap(err, "error fetching uninitialized hosts")
	}

	catcher := grip.NewBasicCatcher()

	var started int
	for _, h := range hostsToStart {
		if h.UserHost {
			// pass:
			//    always start spawn hosts asap
		} else if started > 12 {
			// throttle hosts, so that we're starting very
			// few hosts on every pass. Hostinit runs very
			// frequently, lets not start too many all at
			// once.

			continue
		}

		err = CreateHost(ctx, &h, settings)

		if errors.Cause(err) == errIgnorableCreateHost {
			continue
		} else if err != nil {
			catcher.Add(err)
			continue
		}

		started++
	}

	return catcher.Resolve()
}

// setupReadyHosts runs the distro setup script of all hosts that are up and reachable.
func setupReadyHosts(ctx context.Context, settings *evergreen.Settings) error {
	// find all hosts in the uninitialized state
	uninitializedHosts, err := host.Find(host.Starting())
	if err != nil {
		return errors.Wrap(err, "error fetching starting hosts")
	}

	catcher := grip.NewBasicCatcher()
	for _, h := range uninitializedHosts {

		err := SetupHost(ctx, &h, settings)
		if errors.Cause(err) == errRetryHost {
			continue
		}

		catcher.Add(err)
	}
	return catcher.Resolve()
}

func TestSetupReadyHosts(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testutil.TestConfig(), "TestSetupReadyHosts")

	conf := testutil.TestConfig()
	mockCloud := cloud.GetMockProvider()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("When hosts are spawned but not running", t, func() {
		testutil.HandleTestingErr(
			db.ClearCollections(host.Collection), t, "error clearing test collections")
		mockCloud.Reset()

		hostsForTest := make([]host.Host, 10)
		for i := 0; i < 10; i++ {
			mockHost, err := spawnMockHost(ctx)
			So(err, ShouldBeNil)
			hostsForTest[i] = *mockHost
		}
		So(mockCloud.Len(), ShouldEqual, 10)

		for i := range hostsForTest {
			h := hostsForTest[i]
			So(h.Status, ShouldNotEqual, evergreen.HostRunning)
		}
		// call it twice to get around rate-limiting
		So(startHosts(ctx, conf), ShouldBeNil)
		So(startHosts(ctx, conf), ShouldBeNil)
		Convey("and all of the hosts have failed", func() {
			for id := range mockCloud.IterIDs() {
				instance := mockCloud.Get(id)
				instance.Status = cloud.StatusFailed
				instance.DNSName = "dnsName"
				instance.IsSSHReachable = true
				mockCloud.Set(id, instance)
			}
		})

		Convey("and all of the hosts are ready with properly set fields", func() {
			for id := range mockCloud.IterIDs() {
				instance := mockCloud.Get(id)
				instance.Status = cloud.StatusRunning
				instance.DNSName = "dnsName"
				instance.IsSSHReachable = true
				mockCloud.Set(id, instance)
			}
			Convey("when running setup", func() {
				err := setupReadyHosts(ctx, conf)
				So(err, ShouldBeNil)

				Convey("then all of the 'OnUp' functions should have been run and "+
					"host should have been marked as provisioned", func() {
					for instance := range mockCloud.IterInstances() {
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

func spawnMockHost(ctx context.Context) (*host.Host, error) {
	mockDistro := distro.Distro{
		Id:       "mock_distro",
		Arch:     "mock_arch",
		WorkDir:  "src",
		PoolSize: 10,
		Provider: evergreen.ProviderNameMock,
	}

	hostOptions := cloud.HostOptions{
		UserName: evergreen.User,
		UserHost: false,
	}

	cloudManager, err := cloud.GetManager(ctx, evergreen.ProviderNameMock, testutil.TestConfig())
	if err != nil {
		return nil, errors.WithStack(err)
	}

	testUser := &user.DBUser{
		Id:     "testuser",
		APIKey: "testapikey",
	}
	testUser.PubKeys = append(testUser.PubKeys, user.PubKey{
		Name: "keyName",
		Key:  "ssh-rsa 1234567890abcdef",
	})

	newHost := cloud.NewIntent(mockDistro, mockDistro.GenerateName(), evergreen.ProviderNameMock, hostOptions)
	newHost, err = cloudManager.SpawnHost(ctx, newHost)
	if err != nil {
		return nil, errors.Wrap(err, "Error spawning instance")
	}
	err = newHost.Insert()
	if err != nil {
		return nil, errors.Wrap(err, "Error inserting host")
	}

	return newHost, nil
}
