package scheduler

import (
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/testutil"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupFuncs(t *testing.T) {

	var taskComparator *CmpBasedTaskComparator
	var taskIds []string
	var tasks []task.Task

	Convey("When running the setup funcs for task prioritizing", t, func() {

		taskComparator = &CmpBasedTaskComparator{}

		taskIds = []string{"t1", "t2", "t3"}

		tasks = []task.Task{
			{Id: taskIds[0]},
			{Id: taskIds[1]},
			{Id: taskIds[2]},
		}

		testutil.HandleTestingErr(
			db.ClearCollections(build.Collection, task.Collection),
			t, "Failed to clear test collections")

		Convey("the previous task caching setup func should fetch and save the"+
			" relevant previous runs of tasks", func() {

			displayNames := []string{"disp1", "disp2", "disp3"}
			buildVariant := "bv"
			prevTaskIds := []string{"pt1", "pt2", "pt3"}
			project := "project"

			tasks[0].RevisionOrderNumber = 100
			tasks[0].Requester = evergreen.RepotrackerVersionRequester
			tasks[0].DisplayName = displayNames[0]
			tasks[0].BuildVariant = buildVariant
			tasks[0].Project = project

			tasks[1].RevisionOrderNumber = 200
			tasks[1].Requester = evergreen.RepotrackerVersionRequester
			tasks[1].DisplayName = displayNames[1]
			tasks[1].BuildVariant = buildVariant
			tasks[1].Project = project

			tasks[2].RevisionOrderNumber = 300
			tasks[2].Requester = evergreen.RepotrackerVersionRequester
			tasks[2].DisplayName = displayNames[2]
			tasks[2].BuildVariant = buildVariant
			tasks[2].Project = project

			// the previous tasks

			prevTaskOne := &task.Task{
				Id:                  prevTaskIds[0],
				RevisionOrderNumber: 99,
				Requester:           evergreen.RepotrackerVersionRequester,
				DisplayName:         displayNames[0],
				BuildVariant:        buildVariant,
				Project:             project,
				Status:              evergreen.TaskFailed,
			}

			prevTaskTwo := &task.Task{
				Id:                  prevTaskIds[1],
				RevisionOrderNumber: 199,
				Requester:           evergreen.RepotrackerVersionRequester,
				DisplayName:         displayNames[1],
				BuildVariant:        buildVariant,
				Project:             project,
				Status:              evergreen.TaskSucceeded,
			}

			prevTaskThree := &task.Task{
				Id:                  prevTaskIds[2],
				RevisionOrderNumber: 299,
				Requester:           evergreen.RepotrackerVersionRequester,
				DisplayName:         displayNames[2],
				BuildVariant:        buildVariant,
				Project:             project,
				Status:              evergreen.TaskSucceeded,
			}

			So(prevTaskOne.Insert(), ShouldBeNil)
			So(prevTaskTwo.Insert(), ShouldBeNil)
			So(prevTaskThree.Insert(), ShouldBeNil)

			taskComparator.tasks = tasks
			So(cachePreviousTasks(taskComparator), ShouldBeNil)
			So(len(taskComparator.previousTasksCache), ShouldEqual, 3)
			So(taskComparator.previousTasksCache[taskIds[0]].Id, ShouldEqual,
				prevTaskIds[0])
			So(taskComparator.previousTasksCache[taskIds[1]].Id, ShouldEqual,
				prevTaskIds[1])
			So(taskComparator.previousTasksCache[taskIds[2]].Id, ShouldEqual,
				prevTaskIds[2])

		})
	})
}

func TestCacheTaskGroups(t *testing.T) {
	require := require.New(t)
	assert := assert.New(t)
	require.NoError(db.ClearCollections(version.Collection))
	tasks := []task.Task{}
	versions := []version.Version{}
	oddYml := `
task_groups:
- name: odd_task_group
  tasks:
`
	evenYml := `
task_groups:
- name: even_task_group
`
	versionMap := map[string]version.Version{}
	for i := 0; i < 10; i++ {
		versionId := fmt.Sprintf("version-%d", i)
		v := version.Version{
			Id: versionId,
		}

		if i%2 == 0 {
			v.Config = evenYml
		} else {
			v.Config = oddYml
		}
		require.NoError(v.Insert())
		versionMap[v.Id] = v
		versions = append(versions, v)
		tasks = append(tasks, task.Task{
			Id:      fmt.Sprintf("task-%d", i),
			Version: versionId,
		})
	}
	comparator := &CmpBasedTaskComparator{}
	comparator.tasks = tasks
	comparator.versions = versionMap
	assert.NoError(cacheTaskGroups(comparator))
	for i, v := range versions {
		if i%2 == 0 {
			assert.Equal("even_task_group", comparator.projects[v.Id].TaskGroups[0].Name)
		} else {
			assert.Equal("odd_task_group", comparator.projects[v.Id].TaskGroups[0].Name)
		}
	}
}
