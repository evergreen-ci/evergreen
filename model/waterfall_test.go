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
			Id:           "b",
			Activated:    false,
			BuildVariant: "lint",
			DisplayName:  "Lint",
			Version:      "v_1",
			Tasks: []build.TaskCache{
				{Id: "t_80"},
				{Id: "t_79"},
				{Id: "t_86"},
				{Id: "t_200"},
			},
		}
		assert.NoError(t, b.Insert(t.Context()))
		b = build.Build{
			Id:           "a",
			Activated:    true,
			BuildVariant: "ubuntu",
			DisplayName:  "Ubuntu 2204",
			Version:      "v_1",
			Tasks: []build.TaskCache{
				{Id: "t_45"},
				{Id: "t_12"},
				{Id: "t_66"},
			},
		}
		assert.NoError(t, b.Insert(t.Context()))

		tsk := task.Task{Id: "t_80", DisplayName: "Task 80", DisplayStatusCache: evergreen.TaskSucceeded, BuildId: "b", BuildVariant: "lint", BuildVariantDisplayName: "Lint", Version: "v_1", Requester: evergreen.RepotrackerVersionRequester}
		assert.NoError(t, tsk.Insert(t.Context()))
		tsk = task.Task{Id: "t_79", DisplayName: "Task 79", DisplayStatusCache: evergreen.TaskFailed, BuildId: "b", BuildVariant: "lint", BuildVariantDisplayName: "Lint", Version: "v_1", Requester: evergreen.RepotrackerVersionRequester}
		assert.NoError(t, tsk.Insert(t.Context()))
		tsk = task.Task{Id: "t_86", DisplayName: "Task 86", DisplayStatusCache: evergreen.TaskSucceeded, BuildId: "b", BuildVariant: "lint", BuildVariantDisplayName: "Lint", Version: "v_1", Requester: evergreen.RepotrackerVersionRequester}
		assert.NoError(t, tsk.Insert(t.Context()))
		tsk = task.Task{Id: "t_200", DisplayName: "Task 200", DisplayStatusCache: evergreen.TaskSucceeded, BuildId: "b", BuildVariant: "lint", BuildVariantDisplayName: "Lint", Version: "v_1", Requester: evergreen.RepotrackerVersionRequester}
		assert.NoError(t, tsk.Insert(t.Context()))
		tsk = task.Task{Id: "t_45", DisplayName: "Task 12", DisplayStatusCache: evergreen.TaskWillRun, BuildId: "a", BuildVariant: "ubuntu", BuildVariantDisplayName: "Ubuntu 2204", Version: "v_1", Requester: evergreen.RepotrackerVersionRequester}
		assert.NoError(t, tsk.Insert(t.Context()))
		tsk = task.Task{Id: "t_12", DisplayName: "Task 12", DisplayStatusCache: evergreen.TaskWillRun, BuildId: "a", BuildVariant: "ubuntu", BuildVariantDisplayName: "Ubuntu 2204", Version: "v_1", Requester: evergreen.RepotrackerVersionRequester}
		assert.NoError(t, tsk.Insert(t.Context()))
		tsk = task.Task{Id: "t_66", DisplayName: "Task 66", DisplayStatusCache: evergreen.TaskWillRun, BuildId: "a", BuildVariant: "ubuntu", BuildVariantDisplayName: "Ubuntu 2204", Version: "v_1", Requester: evergreen.RepotrackerVersionRequester}
		assert.NoError(t, tsk.Insert(t.Context()))

		builds, err := GetVersionBuilds(t.Context(), v, WaterfallOptions{})
		assert.NoError(t, err)
		assert.Len(t, builds, 2)

		// Assert build variants are sorted alphabetically by display name.
		assert.Equal(t, "Lint", builds[0].DisplayName)
		assert.Equal(t, "Ubuntu 2204", builds[1].DisplayName)

		builds, err = GetVersionBuilds(t.Context(), v, WaterfallOptions{
			Tasks:                []string{"Task 79"},
			TaskCaseSensitive:    true,
			Statuses:             []string{evergreen.TaskFailed},
			Variants:             []string{"Lint"},
			VariantCaseSensitive: true,
		})
		assert.NoError(t, err)
		require.Len(t, builds, 1)
		require.Len(t, builds[0].Tasks, 1)
		assert.Equal(t, "t_79", builds[0].Tasks[0].Id)

		builds, err = GetVersionBuilds(t.Context(), v, WaterfallOptions{
			Tasks:             []string{"task 12"},
			TaskCaseSensitive: false,
			Variants:          []string{"UBUNTU"},
		})
		assert.NoError(t, err)
		require.Len(t, builds, 1)
		assert.Len(t, builds[0].Tasks, 2)

		builds, err = GetVersionBuilds(t.Context(), v, WaterfallOptions{OmitInactiveBuilds: true})
		assert.NoError(t, err)
		require.Len(t, builds, 1)
		assert.Equal(t, "Ubuntu 2204", builds[0].DisplayName)
	})

	t.Run("IncludesDisplayTasksExcludesExecutionTasks", func(t *testing.T) {
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

		// Display task (should be included)
		displayTask := task.Task{
			Id:                 "display_task",
			DisplayName:        "Display Task",
			DisplayStatusCache: evergreen.TaskSucceeded,
			BuildId:            "build_1",
			Version:            "v_1",
			DisplayOnly:        true,
			ExecutionTasks:     []string{"exec_task_1", "exec_task_2"},
			Requester:          evergreen.RepotrackerVersionRequester,
		}
		assert.NoError(t, displayTask.Insert(t.Context()))

		// Execution tasks (should be excluded - grouped under display task)
		execTask1 := task.Task{
			Id:                 "exec_task_1",
			DisplayName:        "Execution Task 1",
			DisplayStatusCache: evergreen.TaskSucceeded,
			BuildId:            "build_1",
			Version:            "v_1",
			DisplayTaskId:      utility.ToStringPtr("display_task"),
			Requester:          evergreen.RepotrackerVersionRequester,
		}
		assert.NoError(t, execTask1.Insert(t.Context()))

		execTask2 := task.Task{
			Id:                 "exec_task_2",
			DisplayName:        "Execution Task 2",
			DisplayStatusCache: evergreen.TaskSucceeded,
			BuildId:            "build_1",
			Version:            "v_1",
			DisplayTaskId:      utility.ToStringPtr("display_task"),
			Requester:          evergreen.RepotrackerVersionRequester,
		}
		assert.NoError(t, execTask2.Insert(t.Context()))

		// Regular task (should be included)
		regularTask := task.Task{
			Id:                 "regular_task",
			DisplayName:        "Regular Task",
			DisplayStatusCache: evergreen.TaskWillRun,
			BuildId:            "build_1",
			Version:            "v_1",
			Requester:          evergreen.RepotrackerVersionRequester,
		}
		assert.NoError(t, regularTask.Insert(t.Context()))

		builds, err := GetVersionBuilds(t.Context(), v, WaterfallOptions{})
		assert.NoError(t, err)
		require.Len(t, builds, 1)
		assert.Equal(t, "Ubuntu", builds[0].DisplayName)

		// Should have 2 tasks: display_task and regular_task
		// Execution tasks should be excluded (they're grouped under the display task)
		require.Len(t, builds[0].Tasks, 2)

		taskNames := []string{}
		for _, tsk := range builds[0].Tasks {
			taskNames = append(taskNames, tsk.DisplayName)
		}
		assert.Contains(t, taskNames, "Display Task")
		assert.Contains(t, taskNames, "Regular Task")
		assert.NotContains(t, taskNames, "Execution Task 1")
		assert.NotContains(t, taskNames, "Execution Task 2")
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

func TestWaterfallTagFilters(t *testing.T) {
	require.NoError(t, db.ClearCollections(VersionCollection, build.Collection, task.Collection))
	t.Cleanup(func() {
		assert.NoError(t, db.ClearCollections(VersionCollection, build.Collection, task.Collection))
	})

	requester := evergreen.RepotrackerVersionRequester
	versions := []Version{
		{
			Id: "v1", Identifier: "project", Requester: requester, RevisionOrderNumber: 100, Activated: utility.TruePtr(),
			BuildIds: []string{"b1", "b2"},
			BuildVariants: []VersionBuildStatus{
				{BuildId: "b1", BuildVariant: "alpha", DisplayName: "Alpha", Tags: []string{"linux"}, ActivationStatus: ActivationStatus{Activated: true}},
				{BuildId: "b2", BuildVariant: "beta", DisplayName: "Beta", Tags: []string{"arm"}},
			},
		},
		{
			Id: "v2", Identifier: "project", Requester: requester, RevisionOrderNumber: 101, Activated: utility.TruePtr(),
			BuildIds: []string{"b3"},
			BuildVariants: []VersionBuildStatus{
				{BuildId: "b3", BuildVariant: "beta", DisplayName: "Beta", Tags: []string{"arm"}, ActivationStatus: ActivationStatus{Activated: true}},
			},
		},
		{Id: "v3", Identifier: "project", Requester: requester, RevisionOrderNumber: 102, Activated: utility.TruePtr()},
	}
	for i := range versions {
		require.NoError(t, versions[i].Insert(t.Context()))
	}
	builds := []build.Build{
		{Id: "b1", Version: "v1", BuildVariant: "alpha", DisplayName: "Alpha", Activated: true, Tags: []string{"linux"}},
		{Id: "b2", Version: "v1", BuildVariant: "beta", DisplayName: "Beta", Tags: []string{"arm"}},
		{Id: "b3", Version: "v2", BuildVariant: "beta", DisplayName: "Beta", Activated: true, Tags: []string{"arm"}},
	}
	for i := range builds {
		require.NoError(t, builds[i].Insert(t.Context()))
	}
	tasks := []task.Task{
		{Id: "t1", DisplayName: "Owned", Project: "project", Version: "v1", RevisionOrderNumber: 100, Requester: requester, BuildId: "b1", BuildVariant: "alpha", BuildVariantDisplayName: "Alpha", BuildVariantTags: []string{"linux"}, Tags: []string{"team"}, Activated: true, DisplayStatusCache: evergreen.TaskFailed},
		{Id: "t2", DisplayName: "Other", Project: "project", Version: "v1", RevisionOrderNumber: 100, Requester: requester, BuildId: "b1", BuildVariant: "alpha", BuildVariantDisplayName: "Alpha", BuildVariantTags: []string{"linux"}, Tags: []string{"other"}, Activated: true, DisplayStatusCache: evergreen.TaskSucceeded},
		{Id: "t3", DisplayName: "Inactive Build", Project: "project", Version: "v1", RevisionOrderNumber: 100, Requester: requester, BuildId: "b2", BuildVariant: "beta", BuildVariantDisplayName: "Beta", BuildVariantTags: []string{"arm"}, Tags: []string{"team"}, Activated: true, DisplayStatusCache: evergreen.TaskFailed},
		{Id: "t4", DisplayName: "Other Version", Project: "project", Version: "v2", RevisionOrderNumber: 101, Requester: requester, BuildId: "b3", BuildVariant: "beta", BuildVariantDisplayName: "Beta", BuildVariantTags: []string{"arm"}, Tags: []string{"team"}, Activated: true, DisplayStatusCache: evergreen.TaskSucceeded},
		{Id: "display", DisplayName: "Display", Project: "project", Version: "v1", RevisionOrderNumber: 100, Requester: requester, BuildId: "b1", BuildVariant: "alpha", BuildVariantDisplayName: "Alpha", BuildVariantTags: []string{"linux"}, DisplayOnly: true, ExecutionTasks: []string{"exec1", "exec2"}, Activated: true, DisplayStatusCache: evergreen.TaskFailed},
		{Id: "exec1", DisplayName: "Execution One", Project: "project", Version: "v1", RevisionOrderNumber: 100, Requester: requester, BuildId: "b1", BuildVariant: "alpha", BuildVariantDisplayName: "Alpha", BuildVariantTags: []string{"linux"}, DisplayTaskId: utility.ToStringPtr("display"), Tags: []string{"display-team"}, Activated: true, DisplayStatusCache: evergreen.TaskFailed},
		{Id: "exec2", DisplayName: "Execution Two", Project: "project", Version: "v1", RevisionOrderNumber: 100, Requester: requester, BuildId: "b1", BuildVariant: "alpha", BuildVariantDisplayName: "Alpha", BuildVariantTags: []string{"linux"}, DisplayTaskId: utility.ToStringPtr("display"), Tags: []string{"display-team"}, Activated: true, DisplayStatusCache: evergreen.TaskFailed},
	}
	for i := range tasks {
		require.NoError(t, tasks[i].Insert(t.Context()))
	}

	t.Run("VersionVariantTags", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			opts WaterfallOptions
			want []string
		}{
			{name: "NoTagsIncludesUntaggedVersions", want: []string{"v3", "v2", "v1"}},
			{name: "MatchesAnyVariantTag", opts: WaterfallOptions{VariantTags: []string{"linux", "arm"}}, want: []string{"v2", "v1"}},
			{name: "TagsAreExactAndCaseSensitive", opts: WaterfallOptions{VariantTags: []string{"Linux"}}, want: []string{}},
			{name: "MatchesVariantNameAndTagOnSameBuild", opts: WaterfallOptions{Variants: []string{"Alpha"}, VariantCaseSensitive: true, VariantTags: []string{"arm"}}, want: []string{}},
			{name: "OmitsInactiveTaggedBuilds", opts: WaterfallOptions{VariantTags: []string{"arm"}, OmitInactiveBuilds: true}, want: []string{"v2"}},
			{name: "PaginatesMatchingVariants", opts: WaterfallOptions{VariantTags: []string{"arm"}, MinOrder: 100}, want: []string{"v2"}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				tc.opts.Limit = 5
				tc.opts.Requesters = []string{requester}
				found, err := GetActiveWaterfallVersions(t.Context(), "project", tc.opts)
				require.NoError(t, err)
				ids := []string{}
				for _, v := range found {
					ids = append(ids, v.Id)
				}
				assert.Equal(t, tc.want, ids)
			})
		}
	})

	t.Run("TaskFilteredVersions", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			opts WaterfallOptions
			want []string
		}{
			{name: "MatchesTaskTags", opts: WaterfallOptions{TaskTags: []string{"team"}}, want: []string{"v2", "v1"}},
			{name: "CombinesTaskAndVariantTags", opts: WaterfallOptions{TaskTags: []string{"team"}, VariantTags: []string{"linux"}}, want: []string{"v1"}},
			{name: "MatchesExecutionTaskTagsWhenSelectingVersions", opts: WaterfallOptions{TaskTags: []string{"display-team"}, Tasks: []string{"Execution"}, TaskCaseSensitive: true, Statuses: []string{evergreen.TaskFailed}}, want: []string{"v1"}},
			{name: "TagsDoNotMatchAcrossTasks", opts: WaterfallOptions{TaskTags: []string{"other"}, VariantTags: []string{"arm"}}, want: []string{}},
			{name: "DoesNotOmitInactiveBuildsWhenSelectingByTask", opts: WaterfallOptions{TaskTags: []string{"team"}, VariantTags: []string{"arm"}, Statuses: []string{evergreen.TaskFailed}, OmitInactiveBuilds: true}, want: []string{"v1"}},
			{name: "MatchesAnyTaskTag", opts: WaterfallOptions{TaskTags: []string{"missing", "team"}}, want: []string{"v2", "v1"}},
			{name: "TagsAreExactAndCaseSensitive", opts: WaterfallOptions{TaskTags: []string{"TEAM"}}, want: []string{}},
			{name: "PaginatesBackwardWithTagFilters", opts: WaterfallOptions{TaskTags: []string{"team"}, MinOrder: 100}, want: []string{"v2"}},
			{name: "PaginatesForwardWithTagFilters", opts: WaterfallOptions{TaskTags: []string{"team"}, MaxOrder: 101}, want: []string{"v1"}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				tc.opts.Limit = 5
				tc.opts.Requesters = []string{requester}
				searchOffset := 103
				if tc.opts.MinOrder != 0 {
					searchOffset = tc.opts.MinOrder
				} else if tc.opts.MaxOrder != 0 {
					searchOffset = tc.opts.MaxOrder
				}
				found, err := GetActiveVersionsByTaskFilters(t.Context(), "project", tc.opts, searchOffset)
				require.NoError(t, err)
				ids := []string{}
				for _, v := range found {
					ids = append(ids, v.Id)
				}
				assert.Equal(t, tc.want, ids)
			})
		}
	})

	t.Run("VersionBuilds", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			opts WaterfallOptions
			want map[string][]string
		}{
			{name: "NoTagsPreservesAllTasks", want: map[string][]string{"b1": {"display", "t2", "t1"}, "b2": {"t3"}}},
			{name: "MatchesTaskAndVariantTags", opts: WaterfallOptions{TaskTags: []string{"team"}, VariantTags: []string{"linux"}}, want: map[string][]string{"b1": {"t1"}}},
			{name: "OmitsDisplayTaskWhenOnlyExecutionTasksHaveMatchingTags", opts: WaterfallOptions{TaskTags: []string{"display-team"}}, want: map[string][]string{}},
			{name: "AppliesStatusToTaggedTask", opts: WaterfallOptions{TaskTags: []string{"team"}, Statuses: []string{evergreen.TaskSucceeded}}, want: map[string][]string{}},
			{name: "OmitsInactiveTaggedBuilds", opts: WaterfallOptions{VariantTags: []string{"arm"}, OmitInactiveBuilds: true}, want: map[string][]string{}},
			{name: "MatchesAnyVariantTag", opts: WaterfallOptions{VariantTags: []string{"missing", "arm"}}, want: map[string][]string{"b2": {"t3"}}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				found, err := GetVersionBuilds(t.Context(), versions[0], tc.opts)
				require.NoError(t, err)
				got := map[string][]string{}
				for _, b := range found {
					for _, task := range b.Tasks {
						got[b.Id] = append(got[b.Id], task.Id)
					}
				}
				assert.Equal(t, tc.want, got)
			})
		}
	})
}
