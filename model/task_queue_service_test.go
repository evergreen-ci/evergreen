package model

// TODO Test tasks with dependencies
// TODO Test task groups aren't dispatched on more than max hosts

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/stretchr/testify/require"
)

func TestTaskDistroDAGDispatchService(t *testing.T) {
	for testName, testCase := range map[string]func(*testing.T, []TaskQueueItem, time.Duration){
		"TestConstructor": func(t *testing.T, items []TaskQueueItem, ttl time.Duration) {
			service, err := newDistroTaskDAGDispatchService("distro_1", items, ttl)
			require.NoError(t, err)
			require.NotNil(t, service)
		},
	} {
		require.NoError(t, db.ClearCollections(task.Collection))
		items := []TaskQueueItem{}
		var group string
		var variant string
		var version string
		var maxHosts int
		var dependencies []string

		for i := 0; i < 100; i++ {
			dependencies = []string{}
			if i%5 == 0 { // no group
				group = ""
				variant = "variant_1"
				version = "version_1"
				maxHosts = 0

				if i > 30 && i < 50 {
					dependencies = append(dependencies, strconv.Itoa(i+5))
				}
				if i > 60 && i < 80 {
					dependencies = append(dependencies, strconv.Itoa(i+5))
				}

			} else if i%5 == 1 { // group 1
				group = "group_1"
				variant = "variant_1"
				version = "version_1"
				maxHosts = 1
			} else if i%5 == 2 { // group 2
				group = "group_2"
				variant = "variant_1"
				version = "version_1"
				maxHosts = 2
			} else if i%5 == 3 { // different variant
				group = "group_1"
				variant = "variant_2"
				version = "version_1"
				maxHosts = 2
			} else if i%5 == 4 { // different version
				group = "group_1"
				variant = "variant_1"
				version = "version_2"
				maxHosts = 2
			}

			ID := fmt.Sprintf("%d", i)

			items = append(items, TaskQueueItem{
				Id:            ID,
				Group:         group,
				BuildVariant:  variant,
				Version:       version,
				GroupMaxHosts: maxHosts,
				Dependencies:  dependencies,
			})
			nextTask := task.Task{
				Id:                ID,
				TaskGroup:         group,
				BuildVariant:      variant,
				Version:           version,
				TaskGroupMaxHosts: maxHosts,
				StartTime:         util.ZeroTime,
				FinishTime:        util.ZeroTime,
			}
			require.NoError(t, nextTask.Insert())
		}
		t.Run(testName, func(t *testing.T) {
			testCase(t, items, 60)
		})
	}
}

func TestTaskDispatchService(t *testing.T) {
	constructors := map[string]TaskDistroQueueServiceConstructor{
		"taskDistroDispatchService": newDistroTaskDispatchService,
		// "taskDistroDAGDispatchService": newDistroTaskDAGDispatchService,
	}
	for name, constructor := range constructors {
		t.Run(name, func(t *testing.T) {
			// map["TestName"]: func(*testing.T, []TaskQueueItem, TaskDistroQueueServiceConstructor)
			for testName, testCase := range map[string]func(*testing.T, []TaskQueueItem, TaskDistroQueueServiceConstructor){
				"TestConstructor": func(t *testing.T, items []TaskQueueItem, constructor TaskDistroQueueServiceConstructor) {
					require.NotNil(t, constructor("distro_1", items, time.Minute))
				},

				"TestFindOneTask": func(t *testing.T, items []TaskQueueItem, constructor TaskDistroQueueServiceConstructor) {
					service := constructor("distro_1", []TaskQueueItem{
						{
							Id:    "0",
							Group: "",
						},
					}, time.Minute)
					next := service.FindNextTask(TaskSpec{})
					require.NotNil(t, next)
					require.Equal(t, "0", next.Id)
					next = service.FindNextTask(TaskSpec{})
					require.Nil(t, next)

				},

				"TestSingleHostTaskGroupsBlock": func(t *testing.T, items []TaskQueueItem, constructor TaskDistroQueueServiceConstructor) {
					require.NoError(t, db.ClearCollections(task.Collection))
					items = []TaskQueueItem{}
					var startTime time.Time
					var endTime time.Time
					var status string
					for i := 0; i < 5; i++ {
						items = append(items, TaskQueueItem{
							Id:            fmt.Sprintf("%d", i),
							Group:         "group_1",
							BuildVariant:  "variant_1",
							Version:       "version_1",
							GroupMaxHosts: 1,
						})
						if i == 0 {
							startTime = time.Now().Add(-2 * time.Minute)
							endTime = time.Now().Add(-time.Minute)
							status = evergreen.TaskSucceeded
						} else if i == 1 {
							startTime = time.Now().Add(-time.Minute)
							endTime = time.Now()
							status = evergreen.TaskFailed
						} else {
							startTime = util.ZeroTime
							endTime = util.ZeroTime
							status = ""
						}
						nextTask := task.Task{
							Id:                fmt.Sprintf("%d", i),
							TaskGroup:         "group_1",
							BuildVariant:      "variant_1",
							Version:           "version_1",
							TaskGroupMaxHosts: 1,
							StartTime:         startTime,
							FinishTime:        endTime,
							Status:            status,
						}
						require.NoError(t, nextTask.Insert())
					}
					service := constructor("distro_1", items, time.Minute)
					spec := TaskSpec{
						Group:        "group_1",
						BuildVariant: "variant_1",
						Version:      "version_1",
					}
					next := service.FindNextTask(spec)
					require.Nil(t, next)
				},

				"TestFindAllTasks": func(t *testing.T, items []TaskQueueItem, constructor TaskDistroQueueServiceConstructor) {

					service := constructor("distro_1", items, time.Minute)
					var spec TaskSpec
					var next *TaskQueueItem

					// Dispatch 5 tasks from a group
					for i := 0; i < 5; i++ {
						spec = TaskSpec{
							Group:        "group_1",
							BuildVariant: "variant_1",
							Version:      "version_1",
						}
						next = service.FindNextTask(spec)

						require.NotNil(t, next)
						require.Equal(t, fmt.Sprintf("%d", 5*i+1), next.Id)
					}

					// Dispatch 5 tasks from a different group
					for i := 0; i < 5; i++ {
						spec = TaskSpec{
							Group:        "group_2",
							BuildVariant: "variant_1",
							Version:      "version_1",
						}
						next = service.FindNextTask(spec)
						require.Equal(t, fmt.Sprintf("%d", 5*i+2), next.Id)
					}

					// Dispatch 5 tasks from a group in another variant
					for i := 0; i < 5; i++ {
						spec = TaskSpec{
							Group:        "group_1",
							BuildVariant: "variant_2",
							Version:      "version_1",
						}
						next = service.FindNextTask(spec)
						require.Equal(t, fmt.Sprintf("%d", 5*i+3), next.Id)
					}

					// Dispatch 5 tasks from a group in another variant
					for i := 0; i < 5; i++ {
						spec = TaskSpec{
							Group:        "group_1",
							BuildVariant: "variant_1",
							Version:      "version_2",
						}
						next = service.FindNextTask(spec)
						require.Equal(t, fmt.Sprintf("%d", 5*i+4), next.Id)
					}

					// Dispatch 5 more tasks from the first group
					for i := 0; i < 5; i++ {
						spec = TaskSpec{
							Group:        "group_1",
							BuildVariant: "variant_1",
							Version:      "version_1",
						}
						next = service.FindNextTask(spec)
						require.Equal(t, fmt.Sprintf("%d", 5*i+26), next.Id)
					}

					// Dispatch a task with an empty task group should get a non-task group task, then the rest
					// of the task group tasks, then the non-task group tasks again
					spec = TaskSpec{
						Group:        "",
						BuildVariant: "variant_1",
						Version:      "version_1",
					}
					next = service.FindNextTask(spec)
					require.Equal(t, "0", next.Id)
					currentID := 0
					var nextInt int
					var err error
					for i := 0; i < 10; i++ {
						next = service.FindNextTask(spec)
						nextInt, err = strconv.Atoi(next.Id)
						require.NoError(t, err)
						require.True(t, nextInt > currentID)
						currentID = nextInt
						require.Equal(t, "group_1", next.Group)
						require.Equal(t, "variant_1", next.BuildVariant)
						require.Equal(t, "version_1", next.Version)
					}
					currentID = 0
					for i := 0; i < 15; i++ {
						next = service.FindNextTask(spec)
						nextInt, err = strconv.Atoi(next.Id)
						require.NoError(t, err)
						require.True(t, nextInt > currentID)
						currentID = nextInt
						require.Equal(t, "group_2", next.Group)
						require.Equal(t, "variant_1", next.BuildVariant)
						require.Equal(t, "version_1", next.Version)
					}
					currentID = 0
					for i := 0; i < 15; i++ {
						next = service.FindNextTask(spec)
						nextInt, err = strconv.Atoi(next.Id)
						require.NoError(t, err)
						require.True(t, nextInt > currentID)
						currentID = nextInt
						require.Equal(t, "group_1", next.Group)
						require.Equal(t, "variant_2", next.BuildVariant)
						require.Equal(t, "version_1", next.Version)
					}
					currentID = 0
					for i := 0; i < 15; i++ {
						next = service.FindNextTask(spec)
						nextInt, err = strconv.Atoi(next.Id)
						require.NoError(t, err)
						require.True(t, nextInt > currentID)
						currentID = nextInt
						require.Equal(t, "group_1", next.Group)
						require.Equal(t, "variant_1", next.BuildVariant)
						require.Equal(t, "version_2", next.Version)
					}

					// Dispatch the rest of the non-task group tasks
					currentID = 0
					expectedDAGDispatchOrder := []int{
						5,
						10,
						15,
						20,
						25,
						30,
						50,
						45,
						40,
						35,
						55,
						60,
						80,
						75,
						70,
						65,
						85,
						90,
						95,
					}

					for i := 0; i < 19; i++ {
						// next is a TaskQueueItem
						next = service.FindNextTask(spec)
						nextInt, err = strconv.Atoi(next.Id)
						require.NoError(t, err)

						if name == "taskDistroDAGDispatchService" {
							// validate new order w.r.t. dependencies
							// fmt.Println("The nextInt is: " + strconv.Itoa(nextInt))
							require.Equal(t, nextInt, expectedDAGDispatchOrder[i])

						}
						if name == "taskDistroDispatchService" {
							// validate original order
							require.True(t, nextInt > currentID)
						}
						currentID = nextInt
						require.Equal(t, "", next.Group)
						require.Equal(t, "variant_1", next.BuildVariant)
						require.Equal(t, "version_1", next.Version)
					}

				},
			} {
				require.NoError(t, db.ClearCollections(task.Collection))
				items := []TaskQueueItem{}
				var group string
				var variant string
				var version string
				var maxHosts int
				var dependencies []string

				for i := 0; i < 10; i++ {
					dependencies = []string{}
					if i%5 == 0 { // no group
						group = ""
						variant = "variant_1"
						version = "version_1"
						maxHosts = 0

						if i > 30 && i < 50 {
							dependencies = append(dependencies, strconv.Itoa(i+5))
						}
						if i > 60 && i < 80 {
							dependencies = append(dependencies, strconv.Itoa(i+5))
						}

					} else if i%5 == 1 { // group 1
						group = "group_1"
						variant = "variant_1"
						version = "version_1"
						maxHosts = 1
					} else if i%5 == 2 { // group 2
						group = "group_2"
						variant = "variant_1"
						version = "version_1"
						maxHosts = 2
					} else if i%5 == 3 { // different variant
						group = "group_1"
						variant = "variant_2"
						version = "version_1"
						maxHosts = 2
					} else if i%5 == 4 { // different version
						group = "group_1"
						variant = "variant_1"
						version = "version_2"
						maxHosts = 2
					}

					ID := fmt.Sprintf("%d", i)

					// if len(dependencies) > 1 {
					// 	fmt.Println("********************")
					// 	fmt.Println("Adding an item!")
					// 	fmt.Println("Id: " + ID)
					// 	fmt.Println("Group: " + group)
					// 	fmt.Println("Build Variant: " + variant)
					// 	fmt.Println("GroupMaxHosts: " + strconv.Itoa(maxHosts))
					// 	fmt.Println("Number of Dependencies: " + strconv.Itoa(len(dependencies)))
					// 	// Id:            ID,
					// 	// Group:         group,
					// 	// BuildVariant:  variant,
					// 	// Version:       version,
					// 	// GroupMaxHosts: maxHosts,
					// 	// Dependencies:  dependencies,
					// 	fmt.Println("********************")
					// }

					items = append(items, TaskQueueItem{
						Id:            ID,
						Group:         group,
						BuildVariant:  variant,
						Version:       version,
						GroupMaxHosts: maxHosts,
						Dependencies:  dependencies,
					})
					nextTask := task.Task{
						Id:                ID,
						TaskGroup:         group,
						BuildVariant:      variant,
						Version:           version,
						TaskGroupMaxHosts: maxHosts,
						StartTime:         util.ZeroTime,
						FinishTime:        util.ZeroTime,
					}
					require.NoError(t, nextTask.Insert())
				}
				t.Run(testName, func(t *testing.T) {
					testCase(t, items, constructor)
				})
			}
		})
	}
}
