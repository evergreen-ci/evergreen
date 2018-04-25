package scheduler

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
)

var schedulerTestConf = testutil.TestConfig()

func init() {
	db.SetGlobalSessionProvider(schedulerTestConf.SessionFactory())
}

func TestSpawnHosts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("When spawning hosts", t, func() {

		distroIds := []string{"d1", "d2", "d3"}
		Convey("if there are no hosts to be spawned, the Scheduler should not"+
			" make any calls to the CloudManager", func() {
			newHostsNeeded := map[string]int{
				distroIds[0]: 0,
				distroIds[1]: 0,
				distroIds[2]: 0,
			}

			newHostsSpawned, err := spawnHosts(ctx, newHostsNeeded)
			So(err, ShouldBeNil)
			So(len(newHostsSpawned[distroIds[0]]), ShouldEqual, 0)
			So(len(newHostsSpawned[distroIds[1]]), ShouldEqual, 0)
			So(len(newHostsSpawned[distroIds[2]]), ShouldEqual, 0)
		})

		Convey("if there are hosts to be spawned, the Scheduler should make"+
			" one call to the CloudManager for each host, and return the"+
			" results bucketed by distro", func() {

			newHostsNeeded := map[string]int{
				distroIds[0]: 3,
				distroIds[1]: 0,
				distroIds[2]: 1,
			}

			for _, id := range distroIds {
				d := distro.Distro{Id: id, PoolSize: 3, Provider: evergreen.ProviderNameMock}
				So(d.Insert(), ShouldBeNil)
			}

			newHostsSpawned, err := spawnHosts(ctx, newHostsNeeded)
			So(err, ShouldBeNil)
			distroZeroHosts := newHostsSpawned[distroIds[0]]
			distroOneHosts := newHostsSpawned[distroIds[1]]
			distroTwoHosts := newHostsSpawned[distroIds[2]]
			So(len(distroZeroHosts), ShouldEqual, 3)
			So(distroZeroHosts[0].Distro.Id, ShouldEqual, distroIds[0])
			So(distroZeroHosts[1].Distro.Id, ShouldEqual, distroIds[0])
			So(distroZeroHosts[2].Distro.Id, ShouldEqual, distroIds[0])
			So(len(distroOneHosts), ShouldEqual, 0)
			So(len(distroTwoHosts), ShouldEqual, 1)
			So(distroTwoHosts[0].Distro.Id, ShouldEqual, distroIds[2])
		})

		Reset(func() {
			So(db.Clear(distro.Collection), ShouldBeNil)
			So(db.Clear(host.Collection), ShouldBeNil)
		})
	})
}
