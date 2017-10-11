package scheduler

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/providers/mock"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/pkg/errors"
	. "github.com/smartystreets/goconvey/convey"
)

var schedulerTestConf = testutil.TestConfig()

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(schedulerTestConf))
}

const versionProjectString = `
buildvariants:
- name: ubuntu
  display_name: ubuntu1404
  run_on:
  - ubuntu1404-test
  expansions:
    mongo_url: http://fastdl.mongodb.org/linux/mongodb-linux-x86_64-2.6.1.tgz
  tasks:
  - name: agent
  - name: plugin
  - name: model
tasks:
- name: agent
- name: plugin
- name: model
`

// mock implementations, for testing purposes

type MockTaskFinder struct{}

func (self *MockTaskFinder) FindRunnableTasks() ([]task.Task, error) {
	return nil, errors.New("FindRunnableTasks not implemented")
}

type MockTaskPrioritizer struct{}

func (self *MockTaskPrioritizer) PrioritizeTasks(settings *evergreen.Settings,
	tasks []task.Task) ([]task.Task, error) {
	return nil, errors.New("PrioritizeTasks not implemented")
}

type MockTaskQueuePersister struct{}

func (self *MockTaskQueuePersister) PersistTaskQueue(distro string,
	tasks []task.Task,
	projectTaskDuration model.ProjectTaskDurations) ([]model.TaskQueueItem, error) {
	return nil, errors.New("PersistTaskQueue not implemented")
}

type MockTaskDurationEstimator struct{}

func (self *MockTaskDurationEstimator) GetExpectedDurations(
	runnableTasks []task.Task) (model.ProjectTaskDurations, error) {
	return model.ProjectTaskDurations{}, errors.New("GetExpectedDurations not " +
		"implemented")
}

type MockHostAllocator struct{}

func (self *MockHostAllocator) NewHostsNeeded(d HostAllocatorData, s *evergreen.Settings) (
	map[string]int, error) {
	return nil, errors.New("NewHostsNeeded not implemented")
}

func TestUpdateVersionBuildVarMap(t *testing.T) {
	Convey("When updating a version build variant mapping... ", t, func() {
		versionBuildVarMap := make(map[versionBuildVariant]model.BuildVariant)
		schedulerInstance := &Scheduler{
			schedulerTestConf,
			&MockTaskFinder{},
			&MockTaskPrioritizer{},
			&MockTaskDurationEstimator{},
			&MockTaskQueuePersister{},
			&MockHostAllocator{},
		}

		Convey("if there are no versions with the given id, an error should "+
			"be returned", func() {
			err := schedulerInstance.updateVersionBuildVarMap("versionStr", versionBuildVarMap)
			So(err, ShouldNotBeNil)
		})

		Convey("if there is a version with the given id, no error should "+
			"be returned and the map should be updated", func() {
			v := &version.Version{
				Id:     "versionStr",
				Config: versionProjectString,
			}

			// insert the test version
			So(v.Insert(), ShouldBeNil)
			key := versionBuildVariant{"versionStr", "ubuntu"}
			err := schedulerInstance.updateVersionBuildVarMap("versionStr", versionBuildVarMap)
			So(err, ShouldBeNil)

			// check versionBuildVariant map
			buildVariant, ok := versionBuildVarMap[key]
			So(ok, ShouldBeTrue)
			So(buildVariant, ShouldNotBeNil)

			// check buildvariant tasks
			So(len(buildVariant.Tasks), ShouldEqual, 3)
		})

		Reset(func() {
			So(db.Clear(version.Collection), ShouldBeNil)
		})

	})

}

func TestSpawnHosts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	Convey("When spawning hosts", t, func() {

		distroIds := []string{"d1", "d2", "d3"}

		schedulerInstance := &Scheduler{
			schedulerTestConf,
			&MockTaskFinder{},
			&MockTaskPrioritizer{},
			&MockTaskDurationEstimator{},
			&MockTaskQueuePersister{},
			&MockHostAllocator{},
		}

		Convey("if there are no hosts to be spawned, the Scheduler should not"+
			" make any calls to the CloudManager", func() {
			newHostsNeeded := map[string]int{
				distroIds[0]: 0,
				distroIds[1]: 0,
				distroIds[2]: 0,
			}

			newHostsSpawned, err := schedulerInstance.spawnHosts(ctx, newHostsNeeded)
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
				d := distro.Distro{Id: id, PoolSize: 3, Provider: mock.ProviderName}
				So(d.Insert(), ShouldBeNil)
			}

			newHostsSpawned, err := schedulerInstance.spawnHosts(ctx, newHostsNeeded)
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
