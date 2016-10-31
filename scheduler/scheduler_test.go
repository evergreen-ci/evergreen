package scheduler

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/providers/mock"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	. "github.com/smartystreets/goconvey/convey"
)

var (
	schedulerTestConf = evergreen.TestConfig()
)

func init() {
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(schedulerTestConf))
	if schedulerTestConf.Scheduler.LogFile != "" {
		evergreen.SetLogger(schedulerTestConf.Scheduler.LogFile)
	}
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
	return nil, fmt.Errorf("FindRunnableTasks not implemented")
}

type MockTaskPrioritizer struct{}

type revisionOrderComparator struct {
	tasks []task.Task
}

func (cmp *revisionOrderComparator) Less(i, j int) bool {
	return cmp.tasks[i].RevisionOrderNumber < cmp.tasks[j].RevisionOrderNumber
}

func (cmp *revisionOrderComparator) Len() int {
	return len(cmp.tasks)
}

func (cmp *revisionOrderComparator) Swap(i, j int) {
	cmp.tasks[i], cmp.tasks[j] = cmp.tasks[j], cmp.tasks[i]
}

func (self *MockTaskPrioritizer) PrioritizeTasks(settings *evergreen.Settings,
	tasks []task.Task) ([]task.Task, error) {
	cmp := revisionOrderComparator{tasks}
	sort.Sort(&cmp)
	return cmp.tasks, nil
}

type MockTaskQueuePersister struct{}

func (self *MockTaskQueuePersister) PersistTaskQueue(distro string,
	tasks []task.Task,
	projectTaskDuration model.ProjectTaskDurations) ([]model.TaskQueueItem, error) {
	taskQueue := make([]model.TaskQueueItem, 0, len(tasks))
	for _, t := range tasks {
		expectedTaskDuration := model.GetTaskExpectedDuration(t, projectTaskDuration)
		if t.Priority < 0 {
			return taskQueue, fmt.Errorf("priority was %v but cannot be less than 0", t.Priority)
		}
		taskQueue = append(taskQueue, model.TaskQueueItem{
			Id:                  t.Id,
			DisplayName:         t.DisplayName,
			BuildVariant:        t.BuildVariant,
			RevisionOrderNumber: t.RevisionOrderNumber,
			Requester:           t.Requester,
			Revision:            t.Revision,
			Project:             t.Project,
			ExpectedDuration:    expectedTaskDuration,
			Priority:            t.Priority,
		})
	}

	return taskQueue, nil
}

type MockTaskDurationEstimator struct{}

func (self *MockTaskDurationEstimator) GetExpectedDurations(
	runnableTasks []task.Task) (model.ProjectTaskDurations, error) {
	return model.ProjectTaskDurations{}, fmt.Errorf("GetExpectedDurations not " +
		"implemented")
}

type MockHostAllocator struct{}

func (self *MockHostAllocator) NewHostsNeeded(d HostAllocatorData, s *evergreen.Settings) (
	map[string]int, error) {
	return nil, fmt.Errorf("NewHostsNeeded not implemented")
}

type MockCloudManager struct{}

func (self *MockCloudManager) StartInstance(distroId string) (bool, error) {
	return false, fmt.Errorf("StartInstance not implemented")
}

func (self *MockCloudManager) SpawnInstance(distroId string,
	configDir string) (*host.Host, error) {
	return &host.Host{Distro: distro.Distro{Id: distroId}}, nil
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
			db.Clear(version.Collection)
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

			for _, id := range distroIds {
				d := distro.Distro{Id: id, PoolSize: 1, Provider: mock.ProviderName}
				So(d.Insert(), ShouldBeNil)
			}

			newHostsSpawned, err := schedulerInstance.spawnHosts(newHostsNeeded)
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
			db.Clear(distro.Collection)
		})

	})

}

func TestScheduleDistro(t *testing.T) {
	Convey("When scheduling a distro", t, func() {

		distroId := "test-distro"
		projectName := "test-project"
		buildVariantName := "test-buildvariant"

		runnableTasks := []task.Task{}
		projectTaskDurations := model.ProjectTaskDurations{map[string]*model.BuildVariantTaskDurations{}}
		bvTaskDurations := model.BuildVariantTaskDurations{map[string]*model.TaskDurations{}}
		taskDurations := model.TaskDurations{map[string]time.Duration{}}

		schedulerInstance := &Scheduler{
			schedulerTestConf,
			&MockTaskFinder{},
			&MockTaskPrioritizer{},
			&MockTaskDurationEstimator{},
			&MockTaskQueuePersister{},
			&MockHostAllocator{},
		}
		Convey("if there are tasks to be scheduled, then the Scheduler should"+
			" prioritize them", func() {
			var totalDuration time.Duration
			for i := 9; i >= 0; i-- {
				displayName := fmt.Sprintf("task%v", i)

				newTask := task.Task{
					Project:             projectName,
					BuildVariant:        buildVariantName,
					DisplayName:         displayName,
					RevisionOrderNumber: i,
				}
				runnableTasks = append(runnableTasks, newTask)
				duration := time.Duration(i) * time.Minute
				taskDurations.TaskDurationByDisplayName[displayName] = duration

				totalDuration += duration
			}
			bvTaskDurations.TaskDurationByBuildVariant[buildVariantName] = &taskDurations
			projectTaskDurations.TaskDurationByProject[projectName] = &bvTaskDurations

			res := schedulerInstance.scheduleDistro(distroId, runnableTasks, projectTaskDurations)
			So(res.err, ShouldBeNil)
			So(res.distroId, ShouldEqual, distroId)
			So(len(res.taskQueueItem), ShouldEqual, 10)

			// ensure that the prioritizer put them in the correct order by revision order number
			for i, queueItem := range res.taskQueueItem {
				So(queueItem.RevisionOrderNumber, ShouldEqual, i)
			}
			So(res.schedulerEvent.ExpectedDuration, ShouldEqual, totalDuration)
		})
		Convey("if one of the scheduler's functions returns an error, it should be returned in"+
			" the result", func() {
			displayName := fmt.Sprintf("task%v", 1)

			newTask := task.Task{
				Priority:            -1,
				Project:             projectName,
				BuildVariant:        buildVariantName,
				DisplayName:         displayName,
				RevisionOrderNumber: 1,
			}
			runnableTasks = append(runnableTasks, newTask)
			duration := time.Duration(1) * time.Minute
			taskDurations.TaskDurationByDisplayName[displayName] = duration

			bvTaskDurations.TaskDurationByBuildVariant[buildVariantName] = &taskDurations
			projectTaskDurations.TaskDurationByProject[projectName] = &bvTaskDurations

			res := schedulerInstance.scheduleDistro(distroId, runnableTasks, projectTaskDurations)
			So(res.err, ShouldNotBeNil)
			fmt.Println(res.err)
		})
	})
}
