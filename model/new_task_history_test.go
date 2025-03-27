package model

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindActiveTasksForHistory(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(task.Collection))
	}()

	projectId := "evergreen"
	taskName := "test-graphql"
	buildVariant := "ubuntu2204"

	for tName, tCase := range map[string]func(t *testing.T, ctx context.Context){
		"with lower bound only": func(t *testing.T, ctx context.Context) {
			tasks, err := findActiveTasksForHistory(t.Context(), FindTaskHistoryOptions{
				TaskName:     taskName,
				BuildVariant: buildVariant,
				ProjectId:    projectId,
				LowerBound:   utility.ToIntPtr(98),
				UpperBound:   nil,
			})
			require.NoError(t, err)
			require.Len(t, tasks, 2)
			assert.Equal(t, "t_4", tasks[0].Id)
			assert.Equal(t, 101, tasks[0].RevisionOrderNumber)
			assert.Equal(t, "t_2", tasks[1].Id)
			assert.Equal(t, 99, tasks[1].RevisionOrderNumber)
		},
		"with upper bound only": func(t *testing.T, ctx context.Context) {
			tasks, err := findActiveTasksForHistory(t.Context(), FindTaskHistoryOptions{
				TaskName:     taskName,
				BuildVariant: buildVariant,
				ProjectId:    projectId,
				LowerBound:   nil,
				UpperBound:   utility.ToIntPtr(101),
			})
			require.NoError(t, err)
			require.Len(t, tasks, 2)
			assert.Equal(t, "t_4", tasks[0].Id)
			assert.Equal(t, 101, tasks[0].RevisionOrderNumber)
			assert.Equal(t, "t_2", tasks[1].Id)
			assert.Equal(t, 99, tasks[1].RevisionOrderNumber)
		},
		"limit works with lower bound": func(t *testing.T, ctx context.Context) {
			tasks, err := findActiveTasksForHistory(t.Context(), FindTaskHistoryOptions{
				TaskName:     taskName,
				BuildVariant: buildVariant,
				ProjectId:    projectId,
				LowerBound:   utility.ToIntPtr(98),
				UpperBound:   nil,
				Limit:        utility.ToIntPtr(1),
			})
			require.NoError(t, err)
			require.Len(t, tasks, 1)
			// Since lower bound means paginating backwards, older task should be returned.
			assert.Equal(t, "t_2", tasks[0].Id)
			assert.Equal(t, 99, tasks[0].RevisionOrderNumber)
		},
		"limit works with upper bound": func(t *testing.T, ctx context.Context) {
			tasks, err := findActiveTasksForHistory(t.Context(), FindTaskHistoryOptions{
				TaskName:     taskName,
				BuildVariant: buildVariant,
				ProjectId:    projectId,
				LowerBound:   nil,
				UpperBound:   utility.ToIntPtr(101),
				Limit:        utility.ToIntPtr(1),
			})
			require.NoError(t, err)
			require.Len(t, tasks, 1)
			// Since upper bound means paginating forwards, newer task should be returned.
			assert.Equal(t, "t_4", tasks[0].Id)
			assert.Equal(t, 101, tasks[0].RevisionOrderNumber)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			assert.NoError(t, db.ClearCollections(task.Collection))

			t1 := task.Task{
				Id:                  "t_1",
				Requester:           evergreen.GithubPRRequester,
				RevisionOrderNumber: 98,
				Activated:           false,
				Project:             projectId,
				DisplayName:         taskName,
				BuildVariant:        buildVariant,
			}
			assert.NoError(t, t1.Insert())

			t2 := task.Task{
				Id:                  "t_2",
				Requester:           evergreen.TriggerRequester,
				RevisionOrderNumber: 99,
				Activated:           true,
				Project:             projectId,
				DisplayName:         taskName,
				BuildVariant:        buildVariant,
			}
			assert.NoError(t, t2.Insert())

			t3 := task.Task{
				Id:                  "t_3",
				Requester:           evergreen.RepotrackerVersionRequester,
				RevisionOrderNumber: 100,
				Activated:           false,
				Project:             projectId,
				DisplayName:         taskName,
				BuildVariant:        buildVariant,
			}
			assert.NoError(t, t3.Insert())

			t4 := task.Task{
				Id:                  "t_4",
				Requester:           evergreen.AdHocRequester,
				RevisionOrderNumber: 101,
				Activated:           true,
				Project:             projectId,
				DisplayName:         taskName,
				BuildVariant:        buildVariant,
			}
			assert.NoError(t, t4.Insert())

			tCase(t, t.Context())
		})
	}
}

func TestFindInactiveTasksForHistory(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(task.Collection))
	}()

	projectId := "evergreen"
	taskName := "test-graphql"
	buildVariant := "ubuntu2204"

	for tName, tCase := range map[string]func(t *testing.T, ctx context.Context){
		"with lower bound only": func(t *testing.T, ctx context.Context) {
			tasks, err := findInactiveTasksForHistory(t.Context(), FindTaskHistoryOptions{
				TaskName:     taskName,
				BuildVariant: buildVariant,
				ProjectId:    projectId,
				LowerBound:   utility.ToIntPtr(98),
				UpperBound:   nil,
			})
			require.NoError(t, err)
			require.Len(t, tasks, 2)
			assert.Equal(t, "t_5", tasks[0].Id)
			assert.Equal(t, 102, tasks[0].RevisionOrderNumber)
			assert.Equal(t, "t_3", tasks[1].Id)
			assert.Equal(t, 100, tasks[1].RevisionOrderNumber)
		},
		"with upper bound only": func(t *testing.T, ctx context.Context) {
			tasks, err := findInactiveTasksForHistory(t.Context(), FindTaskHistoryOptions{
				TaskName:     taskName,
				BuildVariant: buildVariant,
				ProjectId:    projectId,
				LowerBound:   nil,
				UpperBound:   utility.ToIntPtr(102),
			})
			require.NoError(t, err)
			require.Len(t, tasks, 2)
			assert.Equal(t, "t_5", tasks[0].Id)
			assert.Equal(t, 102, tasks[0].RevisionOrderNumber)
			assert.Equal(t, "t_3", tasks[1].Id)
			assert.Equal(t, 100, tasks[1].RevisionOrderNumber)
		},
		"with both lower and upper bounds": func(t *testing.T, ctx context.Context) {
			tasks, err := findInactiveTasksForHistory(t.Context(), FindTaskHistoryOptions{
				TaskName:     taskName,
				BuildVariant: buildVariant,
				ProjectId:    projectId,
				LowerBound:   utility.ToIntPtr(98),
				UpperBound:   utility.ToIntPtr(99),
			})
			require.NoError(t, err)
			require.Len(t, tasks, 0)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			assert.NoError(t, db.ClearCollections(task.Collection))
			t1 := task.Task{
				Id:                  "t_1",
				Requester:           evergreen.GithubPRRequester,
				RevisionOrderNumber: 98,
				Activated:           false,
				Project:             projectId,
				DisplayName:         taskName,
				BuildVariant:        buildVariant,
			}
			assert.NoError(t, t1.Insert())

			t2 := task.Task{
				Id:                  "t_2",
				Requester:           evergreen.TriggerRequester,
				RevisionOrderNumber: 99,
				Activated:           true,
				Project:             projectId,
				DisplayName:         taskName,
				BuildVariant:        buildVariant,
			}
			assert.NoError(t, t2.Insert())

			t3 := task.Task{
				Id:                  "t_3",
				Requester:           evergreen.RepotrackerVersionRequester,
				RevisionOrderNumber: 100,
				Activated:           false,
				Project:             projectId,
				DisplayName:         taskName,
				BuildVariant:        buildVariant,
			}
			assert.NoError(t, t3.Insert())

			t4 := task.Task{
				Id:                  "t_4",
				Requester:           evergreen.AdHocRequester,
				RevisionOrderNumber: 101,
				Activated:           true,
				Project:             projectId,
				DisplayName:         taskName,
				BuildVariant:        buildVariant,
			}
			assert.NoError(t, t4.Insert())

			t5 := task.Task{
				Id:                  "t_5",
				Requester:           evergreen.AdHocRequester,
				RevisionOrderNumber: 102,
				Activated:           false,
				Project:             projectId,
				DisplayName:         taskName,
				BuildVariant:        buildVariant,
			}
			assert.NoError(t, t5.Insert())

			tCase(t, t.Context())
		})
	}
}

func TestFindTasksForHistory(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(task.Collection))
	}()

	projectId := "evergreen"
	taskName := "test-graphql"
	buildVariant := "ubuntu2204"

	for tName, tCase := range map[string]func(t *testing.T, ctx context.Context){
		"errors when both bounds are defined": func(t *testing.T, ctx context.Context) {
			tasks, err := FindTasksForHistory(t.Context(), FindTaskHistoryOptions{
				TaskName:     taskName,
				BuildVariant: buildVariant,
				ProjectId:    projectId,
				LowerBound:   utility.ToIntPtr(98),
				UpperBound:   utility.ToIntPtr(102),
			})
			assert.Error(t, err)
			assert.Nil(t, tasks)
		},
		"errors when no bounds are defined": func(t *testing.T, ctx context.Context) {
			tasks, err := FindTasksForHistory(t.Context(), FindTaskHistoryOptions{
				TaskName:     taskName,
				BuildVariant: buildVariant,
				ProjectId:    projectId,
				LowerBound:   nil,
				UpperBound:   nil,
			})
			assert.Error(t, err)
			assert.Nil(t, tasks)
		},
		"with lower bound": func(t *testing.T, ctx context.Context) {
			tasks, err := FindTasksForHistory(t.Context(), FindTaskHistoryOptions{
				TaskName:     taskName,
				BuildVariant: buildVariant,
				ProjectId:    projectId,
				LowerBound:   utility.ToIntPtr(98),
				UpperBound:   nil,
			})
			require.NoError(t, err)
			require.Len(t, tasks, 4)
			assert.Equal(t, "t_5", tasks[0].Id)
			assert.Equal(t, 102, tasks[0].RevisionOrderNumber)
			assert.Equal(t, "t_4", tasks[1].Id)
			assert.Equal(t, 101, tasks[1].RevisionOrderNumber)
			assert.Equal(t, "t_3", tasks[2].Id)
			assert.Equal(t, 100, tasks[2].RevisionOrderNumber)
			assert.Equal(t, "t_2", tasks[3].Id)
			assert.Equal(t, 99, tasks[3].RevisionOrderNumber)
		},
		"with upper bound": func(t *testing.T, ctx context.Context) {
			tasks, err := FindTasksForHistory(t.Context(), FindTaskHistoryOptions{
				TaskName:     taskName,
				BuildVariant: buildVariant,
				ProjectId:    projectId,
				LowerBound:   nil,
				UpperBound:   utility.ToIntPtr(102),
			})
			require.NoError(t, err)
			require.Len(t, tasks, 4)
			assert.Equal(t, "t_5", tasks[0].Id)
			assert.Equal(t, 102, tasks[0].RevisionOrderNumber)
			assert.Equal(t, "t_4", tasks[1].Id)
			assert.Equal(t, 101, tasks[1].RevisionOrderNumber)
			assert.Equal(t, "t_3", tasks[2].Id)
			assert.Equal(t, 100, tasks[2].RevisionOrderNumber)
			assert.Equal(t, "t_2", tasks[3].Id)
			assert.Equal(t, 99, tasks[3].RevisionOrderNumber)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			assert.NoError(t, db.ClearCollections(task.Collection))
			t1 := task.Task{
				Id:                  "t_1",
				Requester:           evergreen.GithubPRRequester,
				RevisionOrderNumber: 98,
				Activated:           true,
				Project:             projectId,
				DisplayName:         taskName,
				BuildVariant:        buildVariant,
			}
			assert.NoError(t, t1.Insert())

			t2 := task.Task{
				Id:                  "t_2",
				Requester:           evergreen.TriggerRequester,
				RevisionOrderNumber: 99,
				Activated:           true,
				Project:             projectId,
				DisplayName:         taskName,
				BuildVariant:        buildVariant,
			}
			assert.NoError(t, t2.Insert())

			t3 := task.Task{
				Id:                  "t_3",
				Requester:           evergreen.RepotrackerVersionRequester,
				RevisionOrderNumber: 100,
				Activated:           false,
				Project:             projectId,
				DisplayName:         taskName,
				BuildVariant:        buildVariant,
			}
			assert.NoError(t, t3.Insert())

			t4 := task.Task{
				Id:                  "t_4",
				Requester:           evergreen.AdHocRequester,
				RevisionOrderNumber: 101,
				Activated:           false,
				Project:             projectId,
				DisplayName:         taskName,
				BuildVariant:        buildVariant,
			}
			assert.NoError(t, t4.Insert())

			t5 := task.Task{
				Id:                  "t_5",
				Requester:           evergreen.AdHocRequester,
				RevisionOrderNumber: 102,
				Activated:           true,
				Project:             projectId,
				DisplayName:         taskName,
				BuildVariant:        buildVariant,
			}
			assert.NoError(t, t5.Insert())

			tCase(t, t.Context())
		})
	}
}

func TestGetLatestMainlineTask(t *testing.T) {
	assert.NoError(t, db.ClearCollections(task.Collection))

	projectId := "evergreen"
	taskName := "test-graphql"
	buildVariant := "ubuntu2204"

	t1 := task.Task{
		Id:                  "t_1",
		Requester:           evergreen.GithubPRRequester,
		RevisionOrderNumber: 102,
		Activated:           true,
		Project:             projectId,
		DisplayName:         taskName,
		BuildVariant:        buildVariant,
	}
	assert.NoError(t, t1.Insert())

	t2 := task.Task{
		Id:                  "t_2",
		Requester:           evergreen.PatchVersionRequester,
		RevisionOrderNumber: 101,
		Activated:           true,
		Project:             projectId,
		DisplayName:         taskName,
		BuildVariant:        buildVariant,
	}
	assert.NoError(t, t2.Insert())

	t3 := task.Task{
		Id:                  "t_3",
		Requester:           evergreen.TriggerRequester,
		RevisionOrderNumber: 100,
		Activated:           false,
		Project:             projectId,
		DisplayName:         taskName,
		BuildVariant:        buildVariant,
	}
	assert.NoError(t, t3.Insert())

	t4 := task.Task{
		Id:                  "t_4",
		Requester:           evergreen.GitTagRequester,
		RevisionOrderNumber: 99,
		Activated:           true,
		Project:             projectId,
		DisplayName:         taskName,
		BuildVariant:        buildVariant,
	}
	assert.NoError(t, t4.Insert())

	latestMainlineTask, err := GetLatestMainlineTask(t.Context(), FindTaskHistoryOptions{
		TaskName:     taskName,
		BuildVariant: buildVariant,
		ProjectId:    projectId,
	})
	require.NoError(t, err)
	require.NotNil(t, latestMainlineTask)
	assert.Equal(t, "t_3", latestMainlineTask.Id)
	assert.Equal(t, 100, latestMainlineTask.RevisionOrderNumber)
}

func TestGetOldestMainlineTask(t *testing.T) {
	assert.NoError(t, db.ClearCollections(task.Collection))

	projectId := "evergreen"
	taskName := "test-graphql"
	buildVariant := "ubuntu2204"

	t1 := task.Task{
		Id:                  "t_1",
		Requester:           evergreen.GithubPRRequester,
		RevisionOrderNumber: 98,
		Activated:           true,
		Project:             projectId,
		DisplayName:         taskName,
		BuildVariant:        buildVariant,
	}
	assert.NoError(t, t1.Insert())

	t2 := task.Task{
		Id:                  "t_2",
		Requester:           evergreen.PatchVersionRequester,
		RevisionOrderNumber: 99,
		Activated:           true,
		Project:             projectId,
		DisplayName:         taskName,
		BuildVariant:        buildVariant,
	}
	assert.NoError(t, t2.Insert())

	t3 := task.Task{
		Id:                  "t_3",
		Requester:           evergreen.TriggerRequester,
		RevisionOrderNumber: 100,
		Activated:           true,
		Project:             projectId,
		DisplayName:         taskName,
		BuildVariant:        buildVariant,
	}
	assert.NoError(t, t3.Insert())

	t4 := task.Task{
		Id:                  "t_4",
		Requester:           evergreen.GitTagRequester,
		RevisionOrderNumber: 101,
		Activated:           true,
		Project:             projectId,
		DisplayName:         taskName,
		BuildVariant:        buildVariant,
	}
	assert.NoError(t, t4.Insert())

	oldestMainlineTask, err := GetOldestMainlineTask(t.Context(), FindTaskHistoryOptions{
		TaskName:     taskName,
		BuildVariant: buildVariant,
		ProjectId:    projectId,
	})
	require.NoError(t, err)
	require.NotNil(t, oldestMainlineTask)
	assert.Equal(t, "t_3", oldestMainlineTask.Id)
	assert.Equal(t, 100, oldestMainlineTask.RevisionOrderNumber)
}
