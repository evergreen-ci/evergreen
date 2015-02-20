package scheduler

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"10gen.com/mci/model/host"
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

var (
	schedulerTestConf = mci.TestConfig()
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(schedulerTestConf))
	if schedulerTestConf.Scheduler.LogFile != "" {
		mci.SetLogger(schedulerTestConf.Scheduler.LogFile)
	}
}

// mock implementations, for testing purposes

type MockTaskFinder struct{}

func (self *MockTaskFinder) FindRunnableTasks() ([]model.Task, error) {
	return nil, fmt.Errorf("FindRunnableTasks not implemented")
}

type MockTaskPrioritizer struct{}

func (self *MockTaskPrioritizer) PrioritizeTasks(mciSettings *mci.MCISettings,
	tasks []model.Task) ([]model.Task, error) {
	return nil, fmt.Errorf("PrioritizeTasks not implemented")
}

type MockTaskQueuePersister struct{}

func (self *MockTaskQueuePersister) PersistTaskQueue(distro string,
	tasks []model.Task,
	projectTaskDuration model.ProjectTaskDurations) ([]model.TaskQueueItem, error) {
	return nil, fmt.Errorf("PersistTaskQueue not implemented")
}

type MockTaskDurationEstimator struct{}

func (self *MockTaskDurationEstimator) GetExpectedDurations(
	runnableTasks []model.Task) (model.ProjectTaskDurations, error) {
	return model.ProjectTaskDurations{}, fmt.Errorf("GetExpectedDurations not " +
		"implemented")
}

type MockHostAllocator struct{}

func (self *MockHostAllocator) NewHostsNeeded(
	hostAllocatorData HostAllocatorData, mciSettings *mci.MCISettings) (
	map[string]int, error) {
	return nil, fmt.Errorf("NewHostsNeeded not implemented")
}

type MockCloudManager struct{}

func (self *MockCloudManager) StartInstance(distroName string) (bool, error) {
	return false, fmt.Errorf("StartInstance not implemented")
}

func (self *MockCloudManager) SpawnInstance(distroName string,
	configDir string) (*host.Host, error) {
	return &host.Host{Distro: distroName}, nil
}

func (self *MockCloudManager) GetInstanceStatus(inst *host.Host) (
	string, error) {
	return "", fmt.Errorf("GetInstanceStatus not implemented")
}

func (self *MockCloudManager) GetInstanceDNS(inst *host.Host) (string, error) {
	return "", fmt.Errorf("GetInstanceDNS not implemented")
}

func (self *MockCloudManager) ReconcileInstanceLists() (bool, error) {
	return false, fmt.Errorf("ReconcileInstanceLists not implemented")
}

func (self *MockCloudManager) StopInstance(inst *host.Host) error {
	return fmt.Errorf("StopInstance not implemented")
}

func (self *MockCloudManager) TerminateInstance(inst *host.Host) error {
	return fmt.Errorf("TerminateInstance not implemented")
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
			version := &model.Version{
				Id:     "versionStr",
				Config: "\nbuildvariants:\n  - name: ubuntu\n    display_name: ubuntu\n    run_on:\n    - ubuntu1404-test\n    expansions: \n      mongo_url: http://fastdl.mongodb.org/linux/mongodb-linux-x86_64-2.6.1.tgz\n    tasks:\n    - name: agent\n    - name: plugin\n    - name: mci\n    - name: model\n    - name: scheduler\n    - name: notify\n    - name: cleanup\n    - name: thirdparty\n    - name: util\n    - name: db\n    - name: repotracker\n",
			}

			// insert the test version
			So(version.Insert(), ShouldBeNil)
			key := versionBuildVariant{"versionStr", "ubuntu"}
			err := schedulerInstance.updateVersionBuildVarMap("versionStr", versionBuildVarMap)
			So(err, ShouldBeNil)

			// check versionBuildVariant map
			buildVariant, ok := versionBuildVarMap[key]
			So(ok, ShouldBeTrue)
			So(buildVariant, ShouldNotBeNil)

			// check buildvariant tasks
			So(len(buildVariant.Tasks), ShouldEqual, 11)
		})

		Reset(func() {
			db.Clear(model.VersionsCollection)
		})

	})

}

func TestSpawnHosts(t *testing.T) {

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

			newHostsSpawned, err := schedulerInstance.spawnHosts(newHostsNeeded)
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

			newHostsSpawned, err := schedulerInstance.spawnHosts(newHostsNeeded)
			So(err, ShouldBeNil)
			distroZeroHosts := newHostsSpawned[distroIds[0]]
			distroOneHosts := newHostsSpawned[distroIds[1]]
			distroTwoHosts := newHostsSpawned[distroIds[2]]
			So(len(distroZeroHosts), ShouldEqual, 3)
			So(distroZeroHosts[0].Distro, ShouldEqual, distroIds[0])
			So(distroZeroHosts[1].Distro, ShouldEqual, distroIds[0])
			So(distroZeroHosts[2].Distro, ShouldEqual, distroIds[0])
			So(len(distroOneHosts), ShouldEqual, 0)
			So(len(distroTwoHosts), ShouldEqual, 1)
			So(distroTwoHosts[0].Distro, ShouldEqual, distroIds[2])
		})

	})

}
