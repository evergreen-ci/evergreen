package model

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetActiveWaterfallVersions(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(VersionCollection, build.Collection, task.Collection, ProjectRefCollection))
	}()

	projectId := "a_project"

	for tName, tCase := range map[string]func(t *testing.T, ctx context.Context){
		"Finds active versions with no order range specified": func(t *testing.T, ctx context.Context) {
			versions, err := GetActiveWaterfallVersions(t.Context(), projectId, WaterfallOptions{
				Limit:      4,
				Requesters: evergreen.SystemVersionRequesterTypes,
			})
			assert.NoError(t, err)
			require.Len(t, versions, 4)
			assert.EqualValues(t, "v_1", versions[0].Id)
			assert.EqualValues(t, "v_3", versions[1].Id)
			assert.EqualValues(t, "v_4", versions[2].Id)
			assert.EqualValues(t, "v_5", versions[3].Id)
		},
		"Finds active versions with MaxOrder parameter": func(t *testing.T, ctx context.Context) {
			versions, err := GetActiveWaterfallVersions(t.Context(), projectId, WaterfallOptions{
				Limit:      2,
				Requesters: evergreen.SystemVersionRequesterTypes,
				MaxOrder:   999,
			})
			assert.NoError(t, err)
			require.Len(t, versions, 2)
			assert.EqualValues(t, "v_3", versions[0].Id)
			assert.EqualValues(t, "v_4", versions[1].Id)
		},
		"Finds active versions with MinOrder parameter": func(t *testing.T, ctx context.Context) {
			versions, err := GetActiveWaterfallVersions(t.Context(), projectId, WaterfallOptions{
				Limit:      5,
				Requesters: evergreen.SystemVersionRequesterTypes,
				MinOrder:   997,
			})
			assert.NoError(t, err)
			require.Len(t, versions, 2)
			assert.EqualValues(t, "v_1", versions[0].Id)
			assert.EqualValues(t, "v_3", versions[1].Id)
		},
		"Errors when given invalid requester": func(t *testing.T, ctx context.Context) {
			versions, err := GetActiveWaterfallVersions(t.Context(), projectId, WaterfallOptions{
				Limit:      4,
				Requesters: []string{"foo"},
			})
			assert.Nil(t, versions)
			assert.Error(t, err)
			assert.True(t, strings.HasPrefix(err.Error(), "invalid requester"))
		},
		"Finds active versions with given build variant (case sensitive)": func(t *testing.T, ctx context.Context) {
			versions, err := GetActiveWaterfallVersions(t.Context(), projectId, WaterfallOptions{
				Limit:                4,
				Requesters:           evergreen.SystemVersionRequesterTypes,
				Variants:             []string{"Build Variant 1"},
				VariantCaseSensitive: true,
			})
			assert.NoError(t, err)
			require.Len(t, versions, 1)
			assert.EqualValues(t, "v_4", versions[0].Id)
		},
		"Finds active versions with given build variant, no results (case sensitive)": func(t *testing.T, ctx context.Context) {
			versions, err := GetActiveWaterfallVersions(t.Context(), projectId, WaterfallOptions{
				Limit:                4,
				Requesters:           evergreen.SystemVersionRequesterTypes,
				Variants:             []string{"build variant 1"},
				VariantCaseSensitive: true,
			})
			assert.NoError(t, err)
			assert.Len(t, versions, 0)
		},
		"Finds active versions with given build variant (case insensitive)": func(t *testing.T, ctx context.Context) {
			versions, err := GetActiveWaterfallVersions(t.Context(), projectId, WaterfallOptions{
				Limit:                4,
				Requesters:           evergreen.SystemVersionRequesterTypes,
				Variants:             []string{"build variant 1"},
				VariantCaseSensitive: false,
			})
			assert.NoError(t, err)
			require.Len(t, versions, 1)
			assert.EqualValues(t, "v_4", versions[0].Id)
		},
		"Finds active versions with given build variant (fetch display names)": func(t *testing.T, ctx context.Context) {
			// Inserting this version causes the pipeline to run a $unionWith stage that fetches build display names from the builds collection
			v := Version{
				Id:                  "v_6",
				Identifier:          "a_project",
				Requester:           evergreen.RepotrackerVersionRequester,
				RevisionOrderNumber: 1,
				CreateTime:          time.Date(2024, time.February, 7, 0, 0, 0, 0, time.UTC),
				Activated:           utility.TruePtr(),
			}
			assert.NoError(t, v.Insert(t.Context()))

			versions, err := GetActiveWaterfallVersions(t.Context(), projectId, WaterfallOptions{
				Limit:      4,
				Requesters: evergreen.SystemVersionRequesterTypes,
				Variants:   []string{"Build Variant 1"},
			})
			assert.Nil(t, err)
			require.Len(t, versions, 2)
			assert.EqualValues(t, "v_1", versions[0].Id)
			assert.EqualValues(t, "v_4", versions[1].Id)
		},
		"Returns version even if matching build variant is inactive": func(t *testing.T, ctx context.Context) {
			versions, err := GetActiveWaterfallVersions(t.Context(), projectId, WaterfallOptions{
				Limit:      4,
				Requesters: evergreen.SystemVersionRequesterTypes,
				Variants:   []string{"Build Variant 2"},
			})
			assert.NoError(t, err)
			require.Len(t, versions, 1)
			assert.EqualValues(t, "v_3", versions[0].Id)
		},
		"Returns no versions if there is no matching active build variant and omit inactive builds is true": func(t *testing.T, ctx context.Context) {
			versions, err := GetActiveWaterfallVersions(t.Context(), projectId, WaterfallOptions{
				Limit:              4,
				Requesters:         evergreen.SystemVersionRequesterTypes,
				Variants:           []string{"Build Variant 2"},
				OmitInactiveBuilds: true,
			})
			assert.NoError(t, err)
			require.Len(t, versions, 0)
		},
		"Returns versions with active build variant when omit inactive builds is true": func(t *testing.T, ctx context.Context) {
			versions, err := GetActiveWaterfallVersions(t.Context(), projectId, WaterfallOptions{
				Limit:              4,
				Requesters:         evergreen.SystemVersionRequesterTypes,
				Variants:           []string{"Build Variant 1"},
				OmitInactiveBuilds: true,
			})
			assert.NoError(t, err)
			require.Len(t, versions, 1)
			assert.EqualValues(t, "v_4", versions[0].Id)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			assert.NoError(t, db.ClearCollections(VersionCollection, build.Collection, task.Collection, ProjectRefCollection))

			start := time.Now()
			p := ProjectRef{
				Id:         projectId,
				Identifier: "a_project_identifier",
			}
			assert.NoError(t, p.Insert(t.Context()))

			b := build.Build{
				Id:          "b_1",
				DisplayName: "Build Variant 1",
				Activated:   true,
			}
			assert.NoError(t, b.Insert(t.Context()))

			v := Version{
				Id:                  "v_1",
				Identifier:          p.Id,
				Requester:           evergreen.RepotrackerVersionRequester,
				RevisionOrderNumber: 1000,
				CreateTime:          start,
				Activated:           utility.TruePtr(),
				BuildVariants: []VersionBuildStatus{
					{
						BuildId: "b_1",
					},
				},
			}
			assert.NoError(t, v.Insert(t.Context()))

			v = Version{
				Id:                  "v_2",
				Identifier:          p.Id,
				Requester:           evergreen.RepotrackerVersionRequester,
				RevisionOrderNumber: 999,
				CreateTime:          start.Add(-2 * time.Minute),
				Activated:           utility.FalsePtr(),
			}
			assert.NoError(t, v.Insert(t.Context()))

			v = Version{
				Id:                  "v_3",
				Identifier:          p.Id,
				Requester:           evergreen.RepotrackerVersionRequester,
				RevisionOrderNumber: 998,
				CreateTime:          start.Add(-2 * time.Minute),
				Activated:           utility.TruePtr(),
				BuildVariants: []VersionBuildStatus{
					{
						BuildId:     "b_2",
						DisplayName: "Build Variant 2",
						ActivationStatus: ActivationStatus{
							Activated: false,
						},
					},
				},
			}
			assert.NoError(t, v.Insert(t.Context()))

			v = Version{
				Id:                  "v_4",
				Identifier:          p.Id,
				Requester:           evergreen.RepotrackerVersionRequester,
				RevisionOrderNumber: 997,
				CreateTime:          start.Add(-2 * time.Minute),
				Activated:           utility.TruePtr(),
				BuildVariants: []VersionBuildStatus{
					{
						BuildId:     "b_1",
						DisplayName: "Build Variant 1",
						ActivationStatus: ActivationStatus{
							Activated: true,
						},
					},
				},
			}
			assert.NoError(t, v.Insert(t.Context()))

			v = Version{
				Id:                  "v_5",
				Identifier:          p.Id,
				Requester:           evergreen.RepotrackerVersionRequester,
				RevisionOrderNumber: 996,
				CreateTime:          start.Add(-2 * time.Minute),
				Activated:           utility.TruePtr(),
			}
			assert.NoError(t, v.Insert(t.Context()))

			tCase(t, t.Context())
		})
	}
}

func TestGetAllWaterfallVersions(t *testing.T) {
	assert.NoError(t, db.ClearCollections(VersionCollection, build.Collection, task.Collection, ProjectRefCollection))
	start := time.Now()
	p := ProjectRef{
		Id:         "a_project",
		Identifier: "a_project_identifier",
	}
	assert.NoError(t, p.Insert(t.Context()))

	v := Version{
		Id:                  "v_1",
		Identifier:          "a_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 10,
		CreateTime:          start,
		Activated:           utility.TruePtr(),
	}
	assert.NoError(t, v.Insert(t.Context()))
	v = Version{
		Id:                  "v_2",
		Identifier:          "a_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 9,
		CreateTime:          start.Add(-2 * time.Minute),
		Activated:           utility.FalsePtr(),
	}
	assert.NoError(t, v.Insert(t.Context()))
	v = Version{
		Id:                  "v_3",
		Identifier:          "a_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 8,
		CreateTime:          start.Add(-2 * time.Minute),
		Activated:           utility.TruePtr(),
	}
	assert.NoError(t, v.Insert(t.Context()))
	v = Version{
		Id:                  "v_4",
		Identifier:          "a_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 7,
		CreateTime:          start.Add(-2 * time.Minute),
		Activated:           utility.TruePtr(),
	}
	assert.NoError(t, v.Insert(t.Context()))
	v = Version{
		Id:                  "v_5",
		Identifier:          "a_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 6,
		CreateTime:          start.Add(-2 * time.Minute),
		Activated:           utility.TruePtr(),
	}
	assert.NoError(t, v.Insert(t.Context()))

	versions, err := GetAllWaterfallVersions(t.Context(), p.Id, 7, 9)
	assert.NoError(t, err)
	require.Len(t, versions, 3)
	assert.EqualValues(t, "v_2", versions[0].Id)
	assert.EqualValues(t, "v_3", versions[1].Id)
	assert.EqualValues(t, "v_4", versions[2].Id)

	versions, err = GetAllWaterfallVersions(t.Context(), p.Id, 2, 3)
	assert.NoError(t, err)
	assert.Empty(t, versions)

	versions, err = GetAllWaterfallVersions(t.Context(), p.Id, 9, 8)
	assert.Error(t, err)
	assert.Empty(t, versions)

	versions, err = GetAllWaterfallVersions(t.Context(), p.Id, 10, 12)
	assert.NoError(t, err)
	require.Len(t, versions, 1)
	assert.EqualValues(t, "v_1", versions[0].Id)

	versions, err = GetAllWaterfallVersions(t.Context(), p.Id, 0, 0)
	assert.NoError(t, err)
	require.Len(t, versions, 5)
}

func TestGetVersionBuilds(t *testing.T) {
	t.Run("ReturnsBuildsWithTasksSortedByDisplayName", func(t *testing.T) {
		assert.NoError(t, db.ClearCollections(VersionCollection, build.Collection, task.Collection, ProjectRefCollection))
		start := time.Now()
		p := ProjectRef{
			Id:         "a_project",
			Identifier: "a_project_identifier",
		}
		assert.NoError(t, p.Insert(t.Context()))

		v := Version{
			Id:                  "v_1",
			Identifier:          "a_project",
			Requester:           evergreen.RepotrackerVersionRequester,
			RevisionOrderNumber: 10,
			CreateTime:          start,
			Activated:           utility.TruePtr(),
			BuildIds:            []string{"a", "b"},
		}
		assert.NoError(t, v.Insert(t.Context()))

		b := build.Build{
			Id:          "b",
			Activated:   true,
			DisplayName: "Lint",
			Version:     "v_1",
			Tasks: []build.TaskCache{
				{Id: "t_80"},
				{Id: "t_79"},
				{Id: "t_86"},
				{Id: "t_200"},
			},
		}
		assert.NoError(t, b.Insert(t.Context()))
		b = build.Build{
			Id:          "a",
			Activated:   true,
			DisplayName: "Ubuntu 2204",
			Version:     "v_1",
			Tasks: []build.TaskCache{
				{Id: "t_45"},
				{Id: "t_12"},
				{Id: "t_66"},
			},
		}
		assert.NoError(t, b.Insert(t.Context()))

		tsk := task.Task{Id: "t_80", DisplayName: "Task 80", Status: evergreen.TaskSucceeded, BuildId: "b"}
		assert.NoError(t, tsk.Insert(t.Context()))
		tsk = task.Task{Id: "t_79", DisplayName: "Task 79", Status: evergreen.TaskFailed, BuildId: "b"}
		assert.NoError(t, tsk.Insert(t.Context()))
		tsk = task.Task{Id: "t_86", DisplayName: "Task 86", Status: evergreen.TaskSucceeded, BuildId: "b"}
		assert.NoError(t, tsk.Insert(t.Context()))
		tsk = task.Task{Id: "t_200", DisplayName: "Task 200", Status: evergreen.TaskSucceeded, BuildId: "b"}
		assert.NoError(t, tsk.Insert(t.Context()))
		tsk = task.Task{Id: "t_45", DisplayName: "Task 12", Status: evergreen.TaskWillRun, BuildId: "a"}
		assert.NoError(t, tsk.Insert(t.Context()))
		tsk = task.Task{Id: "t_12", DisplayName: "Task 12", Status: evergreen.TaskWillRun, BuildId: "a"}
		assert.NoError(t, tsk.Insert(t.Context()))
		tsk = task.Task{Id: "t_66", DisplayName: "Task 66", Status: evergreen.TaskWillRun, BuildId: "a"}
		assert.NoError(t, tsk.Insert(t.Context()))

		builds, err := GetVersionBuilds(t.Context(), v.BuildIds)
		assert.NoError(t, err)
		assert.Len(t, builds, 2)

		// Assert build variants are sorted alphabetically by display name.
		assert.Equal(t, "Lint", builds[0].DisplayName)
		assert.Equal(t, "Ubuntu 2204", builds[1].DisplayName)
	})

	t.Run("ExcludesDisplayTasksIncludesExecutionTasks", func(t *testing.T) {
		assert.NoError(t, db.ClearCollections(VersionCollection, build.Collection, task.Collection, ProjectRefCollection))
		start := time.Now()
		p := ProjectRef{
			Id:         "a_project",
			Identifier: "a_project_identifier",
		}
		assert.NoError(t, p.Insert(t.Context()))

		v := Version{
			Id:                  "v_1",
			Identifier:          "a_project",
			Requester:           evergreen.RepotrackerVersionRequester,
			RevisionOrderNumber: 10,
			CreateTime:          start,
			Activated:           utility.TruePtr(),
			BuildIds:            []string{"build_1"},
		}
		assert.NoError(t, v.Insert(t.Context()))

		b := build.Build{
			Id:          "build_1",
			Activated:   true,
			DisplayName: "Ubuntu",
			Version:     "v_1",
			Tasks: []build.TaskCache{
				{Id: "display_task"},
				{Id: "exec_task_1"},
				{Id: "exec_task_2"},
				{Id: "regular_task"},
			},
		}
		assert.NoError(t, b.Insert(t.Context()))

		// Display task (should be excluded)
		displayTask := task.Task{
			Id:             "display_task",
			DisplayName:    "Display Task",
			Status:         evergreen.TaskSucceeded,
			BuildId:        "build_1",
			DisplayOnly:    true,
			ExecutionTasks: []string{"exec_task_1", "exec_task_2"},
		}
		assert.NoError(t, displayTask.Insert(t.Context()))

		// Execution tasks (should be included)
		execTask1 := task.Task{
			Id:            "exec_task_1",
			DisplayName:   "Execution Task 1",
			Status:        evergreen.TaskSucceeded,
			BuildId:       "build_1",
			DisplayTaskId: utility.ToStringPtr("display_task"),
		}
		assert.NoError(t, execTask1.Insert(t.Context()))

		execTask2 := task.Task{
			Id:            "exec_task_2",
			DisplayName:   "Execution Task 2",
			Status:        evergreen.TaskSucceeded,
			BuildId:       "build_1",
			DisplayTaskId: utility.ToStringPtr("display_task"),
		}
		assert.NoError(t, execTask2.Insert(t.Context()))

		// Regular task (should be included)
		regularTask := task.Task{
			Id:          "regular_task",
			DisplayName: "Regular Task",
			Status:      evergreen.TaskWillRun,
			BuildId:     "build_1",
		}
		assert.NoError(t, regularTask.Insert(t.Context()))

		builds, err := GetVersionBuilds(t.Context(), v.BuildIds)
		assert.NoError(t, err)
		require.Len(t, builds, 1)
		assert.Equal(t, "Ubuntu", builds[0].DisplayName)

		// Should have 3 tasks: exec_task_1, exec_task_2, and regular_task
		// Display task should be excluded
		require.Len(t, builds[0].Tasks, 3)

		taskNames := []string{}
		for _, tsk := range builds[0].Tasks {
			taskNames = append(taskNames, tsk.DisplayName)
		}
		assert.Contains(t, taskNames, "Execution Task 1")
		assert.Contains(t, taskNames, "Execution Task 2")
		assert.Contains(t, taskNames, "Regular Task")
		assert.NotContains(t, taskNames, "Display Task")
	})
}

func TestGetNewerActiveWaterfallVersion(t *testing.T) {
	assert.NoError(t, db.ClearCollections(VersionCollection, ProjectRefCollection))
	start := time.Now()
	p := ProjectRef{
		Id:         "a_project",
		Identifier: "a_project_identifier",
	}
	assert.NoError(t, p.Insert(t.Context()))

	// Versions are ordered from new to old.
	v := Version{
		Id:                  "v_0",
		Identifier:          "a_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 11,
		CreateTime:          start,
		Activated:           utility.TruePtr(),
	}
	assert.NoError(t, v.Insert(t.Context()))
	v = Version{
		Id:                  "v_1",
		Identifier:          "a_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 10,
		CreateTime:          start.Add(-2 * time.Minute),
		Activated:           utility.TruePtr(),
	}
	assert.NoError(t, v.Insert(t.Context()))
	v = Version{
		Id:                  "v_2",
		Identifier:          "a_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 9,
		CreateTime:          start.Add(-2 * time.Minute),
		Activated:           utility.FalsePtr(),
	}
	assert.NoError(t, v.Insert(t.Context()))
	v = Version{
		Id:                  "v_3",
		Identifier:          "a_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 8,
		CreateTime:          start.Add(-2 * time.Minute),
		Activated:           utility.FalsePtr(),
	}
	assert.NoError(t, v.Insert(t.Context()))
	v = Version{
		Id:                  "v_4",
		Identifier:          "a_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 7,
		CreateTime:          start.Add(-2 * time.Minute),
		Activated:           utility.TruePtr(),
	}
	assert.NoError(t, v.Insert(t.Context()))

	version, err := GetNewerActiveWaterfallVersion(t.Context(), p.Id, v)
	assert.NoError(t, err)
	require.NotNil(t, version)
	assert.Equal(t, "v_1", version.Id)
}

func TestGetOlderActiveWaterfallVersion(t *testing.T) {
	assert.NoError(t, db.ClearCollections(VersionCollection, ProjectRefCollection))
	start := time.Now()
	p := ProjectRef{
		Id:         "a_project",
		Identifier: "a_project_identifier",
	}
	assert.NoError(t, p.Insert(t.Context()))

	// Versions are ordered from old to new.
	v := Version{
		Id:                  "v_5",
		Identifier:          "a_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 6,
		CreateTime:          start,
		Activated:           utility.TruePtr(),
	}
	assert.NoError(t, v.Insert(t.Context()))
	v = Version{
		Id:                  "v_4",
		Identifier:          "a_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 7,
		CreateTime:          start.Add(2 * time.Minute),
		Activated:           utility.TruePtr(),
	}
	assert.NoError(t, v.Insert(t.Context()))
	v = Version{
		Id:                  "v_3",
		Identifier:          "a_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 8,
		CreateTime:          start.Add(2 * time.Minute),
		Activated:           utility.FalsePtr(),
	}
	assert.NoError(t, v.Insert(t.Context()))
	v = Version{
		Id:                  "v_2",
		Identifier:          "a_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 9,
		CreateTime:          start.Add(2 * time.Minute),
		Activated:           utility.FalsePtr(),
	}
	assert.NoError(t, v.Insert(t.Context()))
	v = Version{
		Id:                  "v_1",
		Identifier:          "a_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 10,
		CreateTime:          start.Add(2 * time.Minute),
		Activated:           utility.TruePtr(),
	}
	assert.NoError(t, v.Insert(t.Context()))

	version, err := GetOlderActiveWaterfallVersion(t.Context(), p.Id, v)
	assert.NoError(t, err)
	require.NotNil(t, version)
	assert.Equal(t, "v_4", version.Id)
}

func TestGetActiveVersionsByTaskFilters(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(task.Collection, VersionCollection, build.Collection))
	}()

	for tName, tCase := range map[string]func(t *testing.T, ctx context.Context){
		"Finds versions with active tasks within the correct order range": func(t *testing.T, ctx context.Context) {
			versions, err := GetActiveVersionsByTaskFilters(ctx, "a_project",
				WaterfallOptions{
					Limit:      5,
					Requesters: evergreen.SystemVersionRequesterTypes,
				}, 1002)
			assert.NoError(t, err)
			require.Len(t, versions, 2)
		},
		"Applies a task name filter (case sensitive)": func(t *testing.T, ctx context.Context) {
			versions, err := GetActiveVersionsByTaskFilters(ctx, "a_project",
				WaterfallOptions{
					Limit:             5,
					Requesters:        evergreen.SystemVersionRequesterTypes,
					Tasks:             []string{"Task 80"},
					TaskCaseSensitive: true,
				}, 1002)
			assert.NoError(t, err)
			require.Len(t, versions, 1)
			assert.Equal(t, versions[0].Id, "v_1")
		},
		"Applies a task name filter, no results (case sensitive)": func(t *testing.T, ctx context.Context) {
			versions, err := GetActiveVersionsByTaskFilters(ctx, "a_project",
				WaterfallOptions{
					Limit:             5,
					Requesters:        evergreen.SystemVersionRequesterTypes,
					Tasks:             []string{"task 80"},
					TaskCaseSensitive: true,
				}, 1002)
			assert.NoError(t, err)
			assert.Len(t, versions, 0)
		},
		"Applies a task name filter (case insensitive)": func(t *testing.T, ctx context.Context) {
			versions, err := GetActiveVersionsByTaskFilters(ctx, "a_project",
				WaterfallOptions{
					Limit:             5,
					Requesters:        evergreen.SystemVersionRequesterTypes,
					Tasks:             []string{"task 80"},
					TaskCaseSensitive: false,
				}, 1002)
			assert.NoError(t, err)
			require.Len(t, versions, 1)
			assert.Equal(t, versions[0].Id, "v_1")
		},
		"Applies a task status filter": func(t *testing.T, ctx context.Context) {
			versions, err := GetActiveVersionsByTaskFilters(ctx, "a_project",
				WaterfallOptions{
					Limit:      5,
					Requesters: evergreen.SystemVersionRequesterTypes,
					Statuses:   []string{evergreen.TaskFailed},
				}, 1002)
			assert.NoError(t, err)
			require.Len(t, versions, 2)
			assert.Equal(t, versions[0].Id, "v_2")
			assert.Equal(t, versions[1].Id, "v_1")
		},
		"Applies a task name and task status filter with no matches": func(t *testing.T, ctx context.Context) {
			versions, err := GetActiveVersionsByTaskFilters(ctx, "a_project",
				WaterfallOptions{
					Limit:      5,
					Requesters: evergreen.SystemVersionRequesterTypes,
					Statuses:   []string{evergreen.TaskFailed},
					Tasks:      []string{"Task 80"},
				}, 1002)
			assert.NoError(t, err)
			require.Len(t, versions, 0)
		},
		"Applies a task name and task status filter": func(t *testing.T, ctx context.Context) {
			versions, err := GetActiveVersionsByTaskFilters(ctx, "a_project",
				WaterfallOptions{
					Limit:      5,
					Requesters: evergreen.SystemVersionRequesterTypes,
					Statuses:   []string{evergreen.TaskFailed},
					Tasks:      []string{"Task 120"},
				}, 1002)
			assert.NoError(t, err)
			require.Len(t, versions, 1)
			assert.Equal(t, versions[0].Id, "v_2")
		},
		"Applies a task name and build variant filter (case sensitive)": func(t *testing.T, ctx context.Context) {
			versions, err := GetActiveVersionsByTaskFilters(ctx, "a_project",
				WaterfallOptions{
					Limit:                5,
					Requesters:           evergreen.SystemVersionRequesterTypes,
					Tasks:                []string{"Task 100"},
					Variants:             []string{"Build Variant 1"},
					TaskCaseSensitive:    true,
					VariantCaseSensitive: true,
				}, 1002)
			assert.NoError(t, err)
			require.Len(t, versions, 1)
			assert.Equal(t, versions[0].Id, "v_1")
		},
		"Applies a task name and build variant filter (case insensitive)": func(t *testing.T, ctx context.Context) {
			versions, err := GetActiveVersionsByTaskFilters(ctx, "a_project",
				WaterfallOptions{
					Limit:                5,
					Requesters:           evergreen.SystemVersionRequesterTypes,
					Tasks:                []string{"task 100"},
					Variants:             []string{"build variant 1"},
					TaskCaseSensitive:    false,
					VariantCaseSensitive: false,
				}, 1002)
			assert.NoError(t, err)
			require.Len(t, versions, 1)
			assert.Equal(t, versions[0].Id, "v_1")
		},
		"Applies a task status and build variant filter": func(t *testing.T, ctx context.Context) {
			versions, err := GetActiveVersionsByTaskFilters(ctx, "a_project",
				WaterfallOptions{
					Limit:      5,
					Requesters: evergreen.SystemVersionRequesterTypes,
					Statuses:   []string{evergreen.TaskFailed},
					Variants:   []string{"bv_2"},
				}, 1002)
			assert.NoError(t, err)
			require.Len(t, versions, 1)
			assert.Equal(t, versions[0].Id, "v_2")
		},
		"Applies a task name, task status, requester, and build variant filter": func(t *testing.T, ctx context.Context) {
			versions, err := GetActiveVersionsByTaskFilters(ctx, "a_project",
				WaterfallOptions{
					Limit:      5,
					Requesters: []string{evergreen.RepotrackerVersionRequester},
					Statuses:   []string{evergreen.TaskSucceeded},
					Tasks:      []string{"Task 80"},
					Variants:   []string{"bv_1"},
				}, 1002)
			assert.NoError(t, err)
			require.Len(t, versions, 1)
			assert.Equal(t, versions[0].Id, "v_1")
		},
	} {
		t.Run(tName, func(t *testing.T) {
			assert.NoError(t, db.ClearCollections(task.Collection, VersionCollection, build.Collection))

			start := time.Now()
			b := build.Build{
				Id:          "b_1",
				DisplayName: "Build Variant 1",
				Activated:   true,
			}
			assert.NoError(t, b.Insert(t.Context()))

			v := Version{
				Id:                  "v_1",
				Identifier:          "a_project",
				Requester:           evergreen.RepotrackerVersionRequester,
				RevisionOrderNumber: 1000,
				CreateTime:          start,
				Activated:           utility.TruePtr(),
			}
			assert.NoError(t, v.Insert(t.Context()))

			v = Version{
				Id:                  "v_2",
				Identifier:          "a_project",
				Requester:           evergreen.GitTagRequester,
				RevisionOrderNumber: 1001,
				CreateTime:          start,
				Activated:           utility.TruePtr(),
				BuildVariants: []VersionBuildStatus{
					{
						BuildId:     "b_2",
						DisplayName: "Build Variant 2",
						ActivationStatus: ActivationStatus{
							Activated: false,
						},
					},
				},
			}
			assert.NoError(t, v.Insert(t.Context()))

			v = Version{
				Id:                  "v_3",
				Identifier:          "a_project",
				Requester:           evergreen.RepotrackerVersionRequester,
				RevisionOrderNumber: 600,
				CreateTime:          start,
				Activated:           utility.TruePtr(),
			}
			assert.NoError(t, v.Insert(t.Context()))

			tsk := task.Task{
				Id:                      "t_80",
				Activated:               true,
				DisplayName:             "Task 80",
				Status:                  evergreen.TaskSucceeded,
				DisplayStatusCache:      evergreen.TaskSucceeded,
				Project:                 "a_project",
				Requester:               evergreen.RepotrackerVersionRequester,
				Version:                 "v_1",
				RevisionOrderNumber:     1000,
				BuildVariant:            "bv_1",
				BuildVariantDisplayName: "Build Variant 1",
			}
			assert.NoError(t, tsk.Insert(t.Context()))

			tsk = task.Task{
				Id:                      "t_100",
				Activated:               true,
				DisplayName:             "Task 100",
				Status:                  evergreen.TaskFailed,
				DisplayStatusCache:      evergreen.TaskFailed,
				Project:                 "a_project",
				Requester:               evergreen.RepotrackerVersionRequester,
				Version:                 "v_1",
				RevisionOrderNumber:     1000,
				BuildVariant:            "bv_1",
				BuildVariantDisplayName: "Build Variant 1",
			}
			assert.NoError(t, tsk.Insert(t.Context()))

			tsk = task.Task{
				Id:                      "t_120",
				Activated:               true,
				DisplayName:             "Task 120",
				Status:                  evergreen.TaskFailed,
				DisplayStatusCache:      evergreen.TaskFailed,
				Project:                 "a_project",
				Requester:               evergreen.RepotrackerVersionRequester,
				Version:                 "v_2",
				RevisionOrderNumber:     1001,
				BuildVariant:            "bv_2",
				BuildVariantDisplayName: "Build Variant 2",
			}
			assert.NoError(t, tsk.Insert(t.Context()))

			tsk = task.Task{
				Id:                      "t_80_2",
				Activated:               true,
				DisplayName:             "Task 80",
				Status:                  evergreen.TaskFailed,
				DisplayStatusCache:      evergreen.TaskFailed,
				Project:                 "a_project",
				Requester:               evergreen.RepotrackerVersionRequester,
				Version:                 "v_1",
				RevisionOrderNumber:     1000,
				BuildVariant:            "bv_1",
				BuildVariantDisplayName: "Build Variant 1",
			}

			tCase(t, t.Context())
		})
	}
}
