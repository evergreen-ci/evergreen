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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(t, db.ClearCollections(VersionCollection, build.Collection, task.Collection, ProjectRefCollection))
	start := time.Now()
	p := ProjectRef{
		Id:         "a_project",
		Identifier: "a_project_identifier",
	}
	assert.NoError(t, p.Insert())

	v := Version{
		Id:                  "v_1",
		Identifier:          "a_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 10,
		CreateTime:          start,
		Activated:           utility.TruePtr(),
	}
	assert.NoError(t, v.Insert())
	v = Version{
		Id:                  "v_2",
		Identifier:          "a_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 9,
		CreateTime:          start.Add(-2 * time.Minute),
		Activated:           utility.FalsePtr(),
	}
	assert.NoError(t, v.Insert())
	v = Version{
		Id:                  "v_3",
		Identifier:          "a_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 8,
		CreateTime:          start.Add(-2 * time.Minute),
		Activated:           utility.TruePtr(),
	}
	assert.NoError(t, v.Insert())
	v = Version{
		Id:                  "v_4",
		Identifier:          "a_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 7,
		CreateTime:          start.Add(-2 * time.Minute),
		Activated:           utility.TruePtr(),
	}
	assert.NoError(t, v.Insert())
	v = Version{
		Id:                  "v_5",
		Identifier:          "a_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 6,
		CreateTime:          start.Add(-2 * time.Minute),
		Activated:           utility.TruePtr(),
	}
	assert.NoError(t, v.Insert())

	versions, err := GetActiveWaterfallVersions(ctx, p.Id, WaterfallOptions{
		Limit:      4,
		Requesters: evergreen.SystemVersionRequesterTypes,
	})
	assert.NoError(t, err)
	require.Len(t, versions, 4)
	assert.EqualValues(t, "v_1", versions[0].Id)
	assert.EqualValues(t, "v_3", versions[1].Id)
	assert.EqualValues(t, "v_4", versions[2].Id)
	assert.EqualValues(t, "v_5", versions[3].Id)

	versions, err = GetActiveWaterfallVersions(ctx, p.Id, WaterfallOptions{
		Limit:      2,
		Requesters: evergreen.SystemVersionRequesterTypes,
		MaxOrder:   9,
	})
	assert.NoError(t, err)
	require.Len(t, versions, 2)
	assert.EqualValues(t, "v_3", versions[0].Id)
	assert.EqualValues(t, "v_4", versions[1].Id)

	versions, err = GetActiveWaterfallVersions(ctx, p.Id, WaterfallOptions{
		Limit:      5,
		Requesters: evergreen.SystemVersionRequesterTypes,
		MinOrder:   7,
	})
	assert.NoError(t, err)
	require.Len(t, versions, 2)
	assert.EqualValues(t, "v_1", versions[0].Id)
	assert.EqualValues(t, "v_3", versions[1].Id)

	versions, err = GetActiveWaterfallVersions(ctx, p.Id,
		WaterfallOptions{
			Limit:      4,
			Requesters: []string{"foo"},
		})
	assert.Nil(t, versions)
	assert.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "invalid requester"))
}

func TestGetAllWaterfallVersions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(t, db.ClearCollections(VersionCollection, build.Collection, task.Collection, ProjectRefCollection))
	start := time.Now()
	p := ProjectRef{
		Id:         "a_project",
		Identifier: "a_project_identifier",
	}
	assert.NoError(t, p.Insert())

	v := Version{
		Id:                  "v_1",
		Identifier:          "a_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 10,
		CreateTime:          start,
		Activated:           utility.TruePtr(),
	}
	assert.NoError(t, v.Insert())
	v = Version{
		Id:                  "v_2",
		Identifier:          "a_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 9,
		CreateTime:          start.Add(-2 * time.Minute),
		Activated:           utility.FalsePtr(),
	}
	assert.NoError(t, v.Insert())
	v = Version{
		Id:                  "v_3",
		Identifier:          "a_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 8,
		CreateTime:          start.Add(-2 * time.Minute),
		Activated:           utility.TruePtr(),
	}
	assert.NoError(t, v.Insert())
	v = Version{
		Id:                  "v_4",
		Identifier:          "a_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 7,
		CreateTime:          start.Add(-2 * time.Minute),
		Activated:           utility.TruePtr(),
	}
	assert.NoError(t, v.Insert())
	v = Version{
		Id:                  "v_5",
		Identifier:          "a_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 6,
		CreateTime:          start.Add(-2 * time.Minute),
		Activated:           utility.TruePtr(),
	}
	assert.NoError(t, v.Insert())

	versions, err := GetAllWaterfallVersions(ctx, p.Id, 7, 9)
	assert.NoError(t, err)
	require.Len(t, versions, 3)
	assert.EqualValues(t, "v_2", versions[0].Id)
	assert.EqualValues(t, "v_3", versions[1].Id)
	assert.EqualValues(t, "v_4", versions[2].Id)

	versions, err = GetAllWaterfallVersions(ctx, p.Id, 2, 3)
	assert.NoError(t, err)
	assert.Len(t, versions, 0)

	versions, err = GetAllWaterfallVersions(ctx, p.Id, 9, 8)
	assert.Error(t, err)
	assert.Empty(t, versions)

	versions, err = GetAllWaterfallVersions(ctx, p.Id, 10, 12)
	assert.NoError(t, err)
	require.Len(t, versions, 1)
	assert.EqualValues(t, "v_1", versions[0].Id)
}

func TestGetWaterfallBuildVariants(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	assert.NoError(t, db.ClearCollections(VersionCollection, build.Collection, task.Collection, ProjectRefCollection))
	start := time.Now()
	p := ProjectRef{
		Id:         "a_project",
		Identifier: "a_project_identifier",
	}
	assert.NoError(t, p.Insert())

	v1 := Version{
		Id:                  "v_1",
		Identifier:          "a_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 10,
		CreateTime:          start,
		Activated:           utility.TruePtr(),
		BuildVariants: []VersionBuildStatus{
			VersionBuildStatus{
				ActivationStatus: ActivationStatus{
					Activated: true,
				},
				BuildVariant: "bv_3",
				BuildId:      "b_a",
			},
			VersionBuildStatus{
				ActivationStatus: ActivationStatus{
					Activated: true,
				},
				BuildVariant: "bv_2",
				BuildId:      "b_b",
			},
			VersionBuildStatus{
				ActivationStatus: ActivationStatus{
					Activated: true,
				},
				BuildVariant: "bv_1",
				BuildId:      "b_c",
			},
		},
	}
	assert.NoError(t, v1.Insert())

	v2 := Version{
		Id:                  "v_2",
		Identifier:          "a_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 9,
		CreateTime:          start.Add(-2 * time.Minute),
		Activated:           utility.FalsePtr(),
		BuildVariants: []VersionBuildStatus{
			VersionBuildStatus{
				ActivationStatus: ActivationStatus{
					Activated: false,
				},
				BuildVariant: "bv_2",
				BuildId:      "b_d",
			},
			VersionBuildStatus{
				ActivationStatus: ActivationStatus{
					Activated: false,
				},
				BuildVariant: "bv_1",
				BuildId:      "b_e",
			},
			VersionBuildStatus{
				ActivationStatus: ActivationStatus{
					Activated: false,
				},
				BuildVariant: "bv_3",
				BuildId:      "b_f",
			},
		},
	}
	assert.NoError(t, v2.Insert())

	v3 := Version{
		Id:                  "v_3",
		Identifier:          "a_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 8,
		CreateTime:          start.Add(-2 * time.Minute),
		Activated:           utility.TruePtr(),
		BuildVariants: []VersionBuildStatus{
			VersionBuildStatus{
				ActivationStatus: ActivationStatus{
					Activated: true,
				},
				BuildVariant: "bv_1",
				BuildId:      "b_g",
			},
			VersionBuildStatus{
				ActivationStatus: ActivationStatus{
					Activated: true,
				},
				BuildVariant: "bv_2",
				BuildId:      "b_h",
			},
			VersionBuildStatus{
				ActivationStatus: ActivationStatus{
					Activated: true,
				},
				BuildVariant: "bv_3",
				BuildId:      "b_i",
			},
		},
	}
	assert.NoError(t, v3.Insert())

	v4 := Version{
		Id:                  "v_4",
		Identifier:          "a_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 7,
		CreateTime:          start.Add(-2 * time.Minute),
		Activated:           utility.TruePtr(),
		BuildVariants: []VersionBuildStatus{
			VersionBuildStatus{
				ActivationStatus: ActivationStatus{
					Activated: true,
				},
				BuildVariant: "bv_3",
				BuildId:      "b_j",
			},
			VersionBuildStatus{
				ActivationStatus: ActivationStatus{
					Activated: true,
				},
				BuildVariant: "bv_1",
				BuildId:      "b_k",
			},
			VersionBuildStatus{
				ActivationStatus: ActivationStatus{
					Activated: true,
				},
				BuildVariant: "bv_2",
				BuildId:      "b_l",
			},
		},
	}
	assert.NoError(t, v4.Insert())

	v5 := Version{
		Id:                  "v_5",
		Identifier:          "a_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 6,
		CreateTime:          start.Add(-2 * time.Minute),
		Activated:           utility.TruePtr(),
		BuildVariants: []VersionBuildStatus{
			VersionBuildStatus{
				ActivationStatus: ActivationStatus{
					Activated: true,
				},
				BuildVariant: "bv_2",
				BuildId:      "b_m",
			},
			VersionBuildStatus{
				ActivationStatus: ActivationStatus{
					Activated: true,
				},
				BuildVariant: "bv_1",
				BuildId:      "b_n",
			},
			VersionBuildStatus{
				ActivationStatus: ActivationStatus{
					Activated: true,
				},
				BuildVariant: "bv_3",
				BuildId:      "b_o",
			},
		},
	}
	assert.NoError(t, v5.Insert())

	b := build.Build{
		Id:          "b_a",
		Activated:   true,
		DisplayName: "Build A",
		Version:     "v_1",
		Tasks: []build.TaskCache{
			build.TaskCache{
				Id: "t_80",
			},
			build.TaskCache{
				Id: "t_79",
			},
			build.TaskCache{
				Id: "t_86",
			},
			build.TaskCache{
				Id: "t_200",
			},
		},
	}
	assert.NoError(t, b.Insert())
	b = build.Build{
		Id:          "b_b",
		Activated:   true,
		DisplayName: "Build B",
		Version:     "v_1",
		Tasks: []build.TaskCache{
			build.TaskCache{
				Id: "t_45",
			},
			build.TaskCache{
				Id: "t_12",
			},
		},
	}
	assert.NoError(t, b.Insert())
	b = build.Build{
		Id:          "b_c",
		Activated:   true,
		DisplayName: "Build C",
		Version:     "v_1",
		Tasks: []build.TaskCache{
			build.TaskCache{
				Id: "t_66",
			},
			build.TaskCache{
				Id: "t_89",
			},
			build.TaskCache{
				Id: "t_32",
			},
		},
	}
	assert.NoError(t, b.Insert())
	b = build.Build{
		Id:          "b_d",
		Activated:   false,
		DisplayName: "Build D",
		Version:     "v_2",
		Tasks: []build.TaskCache{
			build.TaskCache{
				Id: "t_54",
			},
			build.TaskCache{
				Id: "t_432",
			},
			build.TaskCache{
				Id: "t_98",
			},
			build.TaskCache{
				Id: "t_235",
			},
		},
	}
	assert.NoError(t, b.Insert())
	b = build.Build{
		Id:          "b_e",
		Activated:   false,
		DisplayName: "Build E",
		Version:     "v_2",
		Tasks: []build.TaskCache{
			build.TaskCache{
				Id: "t_995",
			},
			build.TaskCache{
				Id: "t_473",
			},
		},
	}
	assert.NoError(t, b.Insert())
	b = build.Build{
		Id:          "b_f",
		Activated:   false,
		DisplayName: "Build F",
		Version:     "v_2",
		Tasks: []build.TaskCache{
			build.TaskCache{
				Id: "t_347",
			},
			build.TaskCache{
				Id: "t_36",
			},
			build.TaskCache{
				Id: "t_3632",
			},
		},
	}
	assert.NoError(t, b.Insert())
	b = build.Build{
		Id:          "b_g",
		Activated:   true,
		DisplayName: "Build G",
		Version:     "v_3",
		Tasks: []build.TaskCache{
			build.TaskCache{
				Id: "t_537",
			},
			build.TaskCache{
				Id: "t_737",
			},
			build.TaskCache{
				Id: "t_135",
			},
			build.TaskCache{
				Id: "t_1",
			},
		},
	}
	assert.NoError(t, b.Insert())
	b = build.Build{
		Id:          "b_h",
		Activated:   true,
		DisplayName: "Build H",
		Version:     "v_3",
		Tasks: []build.TaskCache{
			build.TaskCache{
				Id: "t_92",
			},
			build.TaskCache{
				Id: "t_91",
			},
		},
	}
	assert.NoError(t, b.Insert())
	b = build.Build{
		Id:          "b_i",
		Activated:   true,
		DisplayName: "Build I",
		Version:     "v_3",
		Tasks: []build.TaskCache{
			build.TaskCache{
				Id: "t_9166",
			},
			build.TaskCache{
				Id: "t_46",
			},
			build.TaskCache{
				Id: "t_236",
			},
		},
	}
	assert.NoError(t, b.Insert())
	b = build.Build{
		Id:          "b_j",
		Activated:   true,
		DisplayName: "Build J",
		Version:     "v_4",
		Tasks: []build.TaskCache{
			build.TaskCache{
				Id: "t_23",
			},
			build.TaskCache{
				Id: "t_3333",
			},
			build.TaskCache{
				Id: "t_8458",
			},
			build.TaskCache{
				Id: "t_8423",
			},
		},
	}
	assert.NoError(t, b.Insert())
	b = build.Build{
		Id:          "b_k",
		Activated:   true,
		DisplayName: "Build K",
		Version:     "v_4",
		Tasks: []build.TaskCache{
			build.TaskCache{
				Id: "t_8648",
			},
			build.TaskCache{
				Id: "t_845",
			},
		},
	}
	assert.NoError(t, b.Insert())
	b = build.Build{
		Id:          "b_l",
		Activated:   true,
		DisplayName: "Build L",
		Version:     "v_4",
		Tasks: []build.TaskCache{
			build.TaskCache{
				Id: "t_4834",
			},
			build.TaskCache{
				Id: "t_233",
			},
			build.TaskCache{
				Id: "t_37",
			},
		},
	}
	assert.NoError(t, b.Insert())
	b = build.Build{
		Id:          "b_m",
		Activated:   true,
		DisplayName: "Build M",
		Version:     "v_5",
		Tasks: []build.TaskCache{
			build.TaskCache{
				Id: "t_377",
			},
			build.TaskCache{
				Id: "t_1366",
			},
			build.TaskCache{
				Id: "t_2372",
			},
			build.TaskCache{
				Id: "t_8548",
			},
		},
	}
	assert.NoError(t, b.Insert())
	b = build.Build{
		Id:          "b_n",
		Activated:   true,
		DisplayName: "Build N",
		Version:     "v_5",
		Tasks: []build.TaskCache{
			build.TaskCache{
				Id: "t_695",
			},
			build.TaskCache{
				Id: "t_854",
			},
		},
	}
	assert.NoError(t, b.Insert())
	b = build.Build{
		Id:          "b_o",
		Activated:   true,
		DisplayName: "Build O",
		Version:     "v_5",
		Tasks: []build.TaskCache{
			build.TaskCache{
				Id: "t_5888",
			},
			build.TaskCache{
				Id: "t_894",
			},
			build.TaskCache{
				Id: "t_394",
			},
		},
	}
	assert.NoError(t, b.Insert())

	tsk := task.Task{Id: "t_80", DisplayName: "Task 80", Status: evergreen.TaskSucceeded}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_79", DisplayName: "Task 79", Status: evergreen.TaskFailed}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_86", DisplayName: "Task 86", Status: evergreen.TaskSucceeded}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_200", DisplayName: "Task 200", Status: evergreen.TaskSucceeded}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_45", DisplayName: "Task 12", Status: evergreen.TaskWillRun}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_12", DisplayName: "Task 12", Status: evergreen.TaskWillRun}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_66", DisplayName: "Task 66", Status: evergreen.TaskWillRun}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_89", DisplayName: "Task 89", Status: evergreen.TaskWillRun}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_32", DisplayName: "Task 32", Status: evergreen.TaskWillRun}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_54", DisplayName: "Task 54", Status: evergreen.TaskDispatched}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_932", DisplayName: "Task 932", Status: evergreen.TaskSucceeded}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_98", DisplayName: "Task 98", Status: evergreen.TaskStarted}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_235", DisplayName: "Task 235", Status: evergreen.TaskSucceeded}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_995", DisplayName: "Task 995", Status: evergreen.TaskUnscheduled}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_473", DisplayName: "Task 473", Status: evergreen.TaskUnscheduled}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_347", DisplayName: "Task 347", Status: evergreen.TaskUnscheduled}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_36", DisplayName: "Task 36", Status: evergreen.TaskUnscheduled}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_3632", DisplayName: "Task 3632", Status: evergreen.TaskUnscheduled}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_537", DisplayName: "Task 537", Status: evergreen.TaskUnscheduled}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_737", DisplayName: "Task 737", Status: evergreen.TaskUnscheduled}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_135", DisplayName: "Task 135", Status: evergreen.TaskUnscheduled}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_1", DisplayName: "Task 1", Status: evergreen.TaskUnscheduled}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_92", DisplayName: "Task 92", Status: evergreen.TaskSucceeded}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_91", DisplayName: "Task 91", Status: evergreen.TaskSucceeded}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_9166", DisplayName: "Task 9166", Status: evergreen.TaskSucceeded}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_46", DisplayName: "Task 436", Status: evergreen.TaskSucceeded}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_236", DisplayName: "Task 236", Status: evergreen.TaskSucceeded}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_23", DisplayName: "Task 23", Status: evergreen.TaskSucceeded}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_3333", DisplayName: "Task 3333", Status: evergreen.TaskSucceeded}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_8458", DisplayName: "Task 8458", Status: evergreen.TaskSucceeded}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_8423", DisplayName: "Task 8423", Status: evergreen.TaskSucceeded}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_8648", DisplayName: "Task 8648", Status: evergreen.TaskSucceeded}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_845", DisplayName: "Task 845", Status: evergreen.TaskSucceeded}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_4834", DisplayName: "Task 4834", Status: evergreen.TaskSucceeded}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_233", DisplayName: "Task 233", Status: evergreen.TaskSucceeded}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_37", DisplayName: "Task 37", Status: evergreen.TaskSucceeded}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_377", DisplayName: "Task 377", Status: evergreen.TaskSucceeded}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_1366", DisplayName: "Task 1366", Status: evergreen.TaskSucceeded}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_2372", DisplayName: "Task 2372", Status: evergreen.TaskSucceeded}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_8548", DisplayName: "Task 8548", Status: evergreen.TaskSucceeded}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_695", DisplayName: "Task 695", Status: evergreen.TaskSucceeded}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_854", DisplayName: "Task 854", Status: evergreen.TaskSucceeded}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_5888", DisplayName: "Task 5888", Status: evergreen.TaskSucceeded}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_894", DisplayName: "Task 894", Status: evergreen.TaskSucceeded}
	assert.NoError(t, tsk.Insert())
	tsk = task.Task{Id: "t_394", DisplayName: "Task 394", Status: evergreen.TaskSucceeded}
	assert.NoError(t, tsk.Insert())

	buildVariants, err := GetWaterfallBuildVariants(ctx, []string{v1.Id, v2.Id, v3.Id, v4.Id})
	assert.NoError(t, err)
	assert.Len(t, buildVariants, 3)

	// Assert build variants are sorted alphabetically
	assert.Equal(t, buildVariants[0].Id, "bv_1")
	assert.Equal(t, buildVariants[1].Id, "bv_2")
	assert.Equal(t, buildVariants[2].Id, "bv_3")

	// Each variant has 4 builds, corresponding to `limit`
	assert.Len(t, buildVariants[0].Builds, 4)
	assert.Len(t, buildVariants[1].Builds, 4)
	assert.Len(t, buildVariants[2].Builds, 4)

	assert.Equal(t, buildVariants[0].Builds[0].Id, "b_c")
	assert.Len(t, buildVariants[0].Builds[0].Tasks, 3)
	assert.Equal(t, buildVariants[0].Builds[0].Tasks[0].Id, "t_32")
	assert.Equal(t, buildVariants[0].Builds[0].Tasks[1].Id, "t_66")
	assert.Equal(t, buildVariants[0].Builds[0].Tasks[2].Id, "t_89")
	assert.Equal(t, buildVariants[0].Builds[1].Id, "b_e")
	assert.Len(t, buildVariants[0].Builds[1].Tasks, 2)
	assert.Equal(t, buildVariants[0].Builds[1].Tasks[0].Id, "t_473")
	assert.Equal(t, buildVariants[0].Builds[1].Tasks[1].Id, "t_995")

	assert.Equal(t, buildVariants[0].Builds[2].Id, "b_g")
	assert.Len(t, buildVariants[0].Builds[2].Tasks, 4)
	assert.Equal(t, buildVariants[0].Builds[3].Id, "b_k")
	assert.Len(t, buildVariants[0].Builds[3].Tasks, 2)
}
