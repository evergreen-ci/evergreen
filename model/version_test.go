package model

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func TestLastKnownGoodConfig(t *testing.T) {
	Convey("When calling LastKnownGoodConfig..", t, func() {
		identifier := "identifier"
		Convey("no versions should be returned if there're no good "+
			"last known configurations", func() {
			v := &Version{
				Identifier: identifier,
				Requester:  evergreen.RepotrackerVersionRequester,
				Errors:     []string{"error 1", "error 2"},
			}
			require.NoError(t, v.Insert(t.Context()), "Error inserting test version: %s", v.Id)
			lastGood, err := FindVersionByLastKnownGoodConfig(t.Context(), identifier, -1)
			require.NoError(t, err, "error finding last known good")
			So(lastGood, ShouldBeNil)
		})
		Convey("a version should be returned if there is a last known good configuration", func() {
			v := &Version{
				Identifier: identifier,
				Requester:  evergreen.RepotrackerVersionRequester,
			}
			require.NoError(t, v.Insert(t.Context()), "Error inserting test version: %s", v.Id)
			lastGood, err := FindVersionByLastKnownGoodConfig(t.Context(), identifier, -1)
			require.NoError(t, err, "error finding last known good: %s", lastGood.Id)
			So(lastGood, ShouldNotBeNil)
		})
		Convey("most recent version should be found if there are several recent good configs", func() {
			v := &Version{
				Id:                  "1",
				Identifier:          identifier,
				Requester:           evergreen.RepotrackerVersionRequester,
				RevisionOrderNumber: 1,
			}
			require.NoError(t, v.Insert(t.Context()), "Error inserting test version: %s", v.Id)
			v.Id = "5"
			v.RevisionOrderNumber = 5
			require.NoError(t, v.Insert(t.Context()), "Error inserting test version: %s", v.Id)
			v.Id = "2"
			v.RevisionOrderNumber = 2
			require.NoError(t, v.Insert(t.Context()), "Error inserting test version: %s", v.Id)
			lastGood, err := FindVersionByLastKnownGoodConfig(t.Context(), identifier, -1)
			require.NoError(t, err, "error finding last known good: %s", v.Id)
			So(lastGood, ShouldNotBeNil)
		})
		Reset(func() {
			So(db.Clear(VersionCollection), ShouldBeNil)
		})
	})
}

func TestVersionSortByCreateTime(t *testing.T) {
	assert := assert.New(t)
	versions := VersionsByCreateTime{
		{Id: "5", CreateTime: time.Now().Add(time.Hour * 3)},
		{Id: "3", CreateTime: time.Now().Add(time.Hour)},
		{Id: "1", CreateTime: time.Now()},
		{Id: "4", CreateTime: time.Now().Add(time.Hour * 2)},
		{Id: "100", CreateTime: time.Now().Add(time.Hour * 4)},
	}
	sort.Sort(versions)
	assert.Equal("1", versions[0].Id)
	assert.Equal("3", versions[1].Id)
	assert.Equal("4", versions[2].Id)
	assert.Equal("5", versions[3].Id)
	assert.Equal("100", versions[4].Id)
}

func TestFindLastPeriodicBuild(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.Clear(VersionCollection))
	now := time.Now()
	v1 := Version{
		Id:              "v1",
		PeriodicBuildID: "a",
		Identifier:      "myProj",
		CreateTime:      now.Add(-10 * time.Minute),
	}
	assert.NoError(v1.Insert(t.Context()))
	v2 := Version{
		Id:              "v2",
		PeriodicBuildID: "a",
		Identifier:      "myProj",
		CreateTime:      now.Add(-5 * time.Minute),
	}
	assert.NoError(v2.Insert(t.Context()))
	v3 := Version{
		Id:              "v3",
		PeriodicBuildID: "b",
		Identifier:      "myProj",
		CreateTime:      now,
	}
	assert.NoError(v3.Insert(t.Context()))
	v4 := Version{
		Id:              "v4",
		PeriodicBuildID: "a",
		Identifier:      "someProj",
		CreateTime:      now,
	}
	assert.NoError(v4.Insert(t.Context()))

	mostRecent, err := FindLastPeriodicBuild(t.Context(), "myProj", "a")
	assert.NoError(err)
	assert.Equal(v2.Id, mostRecent.Id)
}

func TestBuildVariantsStatusUnmarshal(t *testing.T) {
	str := `
{
	"id" : "myVersion",
	"build_variants_status" : [
		{
			"id" : "b1_name",
			"activated" : true,
			"activate_at" : "2020-10-06T15:00:21.239Z",
			"build_id" : "b1",
            "batchtime_tasks": [
                {
                    "task_id": "t1",
                    "task_name": "t1_name",
                    "activated": false
                }
            ]
		}
	]
}
`
	v := Version{}
	assert.NoError(t, json.Unmarshal([]byte(str), &v))
	assert.NotEmpty(t, v)
	assert.Equal(t, "myVersion", v.Id)

	require.Len(t, v.BuildVariants, 1)
	bv := v.BuildVariants[0]
	assert.Equal(t, "b1", bv.BuildId)
	assert.Equal(t, "b1_name", bv.BuildVariant)
	assert.True(t, bv.Activated)
	assert.False(t, utility.IsZeroTime(bv.ActivateAt))

	require.Len(t, bv.BatchTimeTasks, 1)
	assert.Equal(t, "t1", bv.BatchTimeTasks[0].TaskId)
	assert.Equal(t, "t1_name", bv.BatchTimeTasks[0].TaskName)
	assert.False(t, bv.BatchTimeTasks[0].Activated)
	assert.True(t, utility.IsZeroTime(bv.BatchTimeTasks[0].ActivateAt))
}

func TestGetVersionsWithTaskOptions(t *testing.T) {
	assert.NoError(t, db.ClearCollections(VersionCollection, build.Collection, task.Collection, ProjectRefCollection))
	p := ProjectRef{
		Id:         "my_project",
		Identifier: "my_ident",
	}
	assert.NoError(t, p.Insert(t.Context()))

	for i := 0; i <= 20; i++ {
		myTask := task.Task{
			Id:          fmt.Sprintf("t%d", i),
			BuildId:     fmt.Sprintf("bv%d", i),
			Version:     fmt.Sprintf("v%d", i),
			DisplayName: "my_task",
		}
		bv := build.Build{
			Id:           myTask.BuildId,
			BuildVariant: "my_bv",
			Version:      myTask.Version,
			Tasks: []build.TaskCache{
				{Id: myTask.Id},
			},
			Activated: true,
		}
		v := Version{
			Id:                  myTask.Version,
			Identifier:          "my_project",
			Requester:           evergreen.RepotrackerVersionRequester,
			RevisionOrderNumber: i,
			BuildVariants: []VersionBuildStatus{
				{
					BuildId:      bv.Id,
					BuildVariant: "my_bv",
				},
			},
		}

		if i == 0 || i == 1 || i == 20 {
			myTask.Activated = true
		}
		assert.NoError(t, v.Insert(t.Context()))
		assert.NoError(t, bv.Insert(t.Context()))
		assert.NoError(t, myTask.Insert(t.Context()))
	}

	// test with tasks
	opts := GetVersionsOptions{Requester: evergreen.RepotrackerVersionRequester, IncludeBuilds: true, IncludeTasks: true,
		Limit: 2}

	versions, err := GetVersionsWithOptions(t.Context(), "my_ident", opts)
	assert.NoError(t, err)
	require.Len(t, versions, 2)
	assert.Equal(t, "v20", versions[0].Id)
	assert.Equal(t, 20, versions[0].RevisionOrderNumber)
	require.Len(t, versions[0].Builds, 1)
	require.Len(t, versions[0].Builds[0].Tasks, 1)
	assert.Equal(t, "t20", versions[0].Builds[0].Tasks[0].Id)
	assert.Equal(t, "v1", versions[1].Id)
	assert.Equal(t, 1, versions[1].RevisionOrderNumber)
	require.Len(t, versions[1].Builds, 1)
	require.Len(t, versions[1].Builds[0].Tasks, 1)
	assert.Equal(t, "t1", versions[1].Builds[0].Tasks[0].Id)

	// test with tasks and ByBuildVariant and ByTask
	opts = GetVersionsOptions{Requester: evergreen.RepotrackerVersionRequester, IncludeBuilds: true, IncludeTasks: true,
		ByBuildVariant: "my_bv", ByTask: "my_task", Limit: 2}

	versions, err = GetVersionsWithOptions(t.Context(), "my_ident", opts)
	assert.NoError(t, err)
	require.Len(t, versions, 2)
	assert.Equal(t, "v20", versions[0].Id)
	assert.Equal(t, 20, versions[0].RevisionOrderNumber)
	require.Len(t, versions[0].Builds, 1)
	require.Len(t, versions[0].Builds[0].Tasks, 1)
	assert.Equal(t, "t20", versions[0].Builds[0].Tasks[0].Id)
	assert.Equal(t, "v1", versions[1].Id)
	assert.Equal(t, 1, versions[1].RevisionOrderNumber)
	require.Len(t, versions[1].Builds, 1)
	require.Len(t, versions[1].Builds[0].Tasks, 1)
	assert.Equal(t, "t1", versions[1].Builds[0].Tasks[0].Id)
}

func TestGetVersionsWithOptions(t *testing.T) {
	assert.NoError(t, db.ClearCollections(VersionCollection, build.Collection, task.Collection, ProjectRefCollection))
	start := time.Now()
	p := ProjectRef{
		Id:         "my_project",
		Identifier: "my_ident",
	}
	assert.NoError(t, p.Insert(t.Context()))
	v := Version{
		Id:                  "my_version",
		Identifier:          "my_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 10,
		BuildVariants: []VersionBuildStatus{
			{
				BuildId:      "bv1",
				BuildVariant: "my_bv",
			},
			{
				BuildId:      "bv2",
				BuildVariant: "your_bv",
			},
		},
		CreateTime: start,
	}
	assert.NoError(t, v.Insert(t.Context()))
	v = Version{
		Id:                  "your_version",
		Identifier:          "my_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 9,
		CreateTime:          start.Add(-1 * time.Minute),
		BuildVariants: []VersionBuildStatus{
			{
				BuildId: "bv_not_activated",
			},
		},
	}
	assert.NoError(t, v.Insert(t.Context()))
	v = Version{
		Id:                  "another_version",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 8,
		Identifier:          "my_project",
		CreateTime:          start.Add(-2 * time.Minute),
	}
	assert.NoError(t, v.Insert(t.Context()))
	v = Version{
		Id:                  "seven_version",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 7,
		Identifier:          "my_project",
		CreateTime:          start.Add(-3 * time.Minute),
	}
	assert.NoError(t, v.Insert(t.Context()))

	bv := build.Build{
		Id:           "bv1",
		Version:      "my_version",
		BuildVariant: "my_bv",
		Tasks: []build.TaskCache{
			{Id: "t1"},
		},
		Activated: true,
		Status:    evergreen.BuildFailed,
	}
	assert.NoError(t, bv.Insert(t.Context()))
	bv = build.Build{
		Id:           "bv_not_activated",
		Version:      "your_version",
		BuildVariant: "my_bv",
		Activated:    false,
		Status:       evergreen.BuildFailed,
	}
	assert.NoError(t, bv.Insert(t.Context()))
	bv = build.Build{
		Id:           "bv2",
		Version:      "my_version",
		BuildVariant: "your_bv",
		Tasks: []build.TaskCache{
			{Id: "t2"},
		},
		Activated: true,
		Status:    evergreen.BuildSucceeded,
	}
	assert.NoError(t, bv.Insert(t.Context()))

	t1 := task.Task{
		Id:          "t1",
		DisplayName: "my_task",
		Status:      evergreen.TaskSucceeded,
		BuildId:     "bv1",
		Version:     "my_version",
		Activated:   true,
	}
	assert.NoError(t, t1.Insert(t.Context()))
	t2 := task.Task{
		Id:          "t2",
		DisplayName: "your_task",
		Status:      evergreen.TaskFailed,
		BuildId:     "bv2",
		Version:     "my_version",
		Activated:   true,
	}
	assert.NoError(t, t2.Insert(t.Context()))

	opts := GetVersionsOptions{Requester: evergreen.RepotrackerVersionRequester}
	versions, err := GetVersionsWithOptions(t.Context(), "my_ident", opts)
	assert.NoError(t, err)
	require.Len(t, versions, 4)
	assert.Equal(t, "my_version", versions[0].Id)
	require.Empty(t, versions[0].Builds)

	// filter out versions with no builds/tasks
	opts = GetVersionsOptions{IncludeBuilds: true, IncludeTasks: true, Requester: evergreen.RepotrackerVersionRequester}
	versions, err = GetVersionsWithOptions(t.Context(), "my_ident", opts)
	assert.NoError(t, err)
	require.Len(t, versions, 1)
	assert.Equal(t, "my_version", versions[0].Id)

	opts.ByBuildVariant = "my_bv"
	opts.IncludeTasks = false
	versions, err = GetVersionsWithOptions(t.Context(), "my_ident", opts)
	assert.NoError(t, err)
	require.Len(t, versions, 1)
	require.Len(t, versions[0].Builds, 1)
	assert.Equal(t, "bv1", versions[0].Builds[0].Id)
	assert.Equal(t, evergreen.BuildFailed, versions[0].Builds[0].Status)
	assert.True(t, versions[0].Builds[0].Activated)
	require.Empty(t, versions[0].Builds[0].Tasks) // not including tasks

	opts = GetVersionsOptions{Limit: 1, Requester: evergreen.RepotrackerVersionRequester}
	versions, err = GetVersionsWithOptions(t.Context(), "my_ident", opts)
	assert.NoError(t, err)
	require.Len(t, versions, 1)
	assert.Equal(t, "my_version", versions[0].Id)
	assert.Empty(t, versions[0].Builds)
	assert.Len(t, versions[0].BuildVariants, 2)

	opts = GetVersionsOptions{Skip: 1, Requester: evergreen.RepotrackerVersionRequester}
	versions, err = GetVersionsWithOptions(t.Context(), "my_project", opts)
	assert.NoError(t, err)
	require.Len(t, versions, 3)
	assert.Equal(t, "your_version", versions[0].Id)
	assert.Equal(t, "another_version", versions[1].Id)
	assert.Equal(t, "seven_version", versions[2].Id)

	opts = GetVersionsOptions{Start: 9, Requester: evergreen.RepotrackerVersionRequester}
	versions, err = GetVersionsWithOptions(t.Context(), "my_project", opts)
	assert.NoError(t, err)
	require.Len(t, versions, 2)
	assert.Equal(t, "another_version", versions[0].Id)
	assert.Equal(t, "seven_version", versions[1].Id)

	opts = GetVersionsOptions{Start: 9, Requester: evergreen.RepotrackerVersionRequester}
	versions, err = GetVersionsWithOptions(t.Context(), "my_project", opts)
	assert.NoError(t, err)
	require.Len(t, versions, 2)
	assert.Equal(t, "another_version", versions[0].Id)
	assert.Equal(t, "seven_version", versions[1].Id)

	opts = GetVersionsOptions{RevisionEnd: 9, Requester: evergreen.RepotrackerVersionRequester}
	versions, err = GetVersionsWithOptions(t.Context(), "my_project", opts)
	assert.NoError(t, err)
	require.Len(t, versions, 2)
	assert.Equal(t, "my_version", versions[0].Id)
	assert.Equal(t, "your_version", versions[1].Id)

	opts = GetVersionsOptions{Start: 9, RevisionEnd: 8, Requester: evergreen.RepotrackerVersionRequester}
	versions, err = GetVersionsWithOptions(t.Context(), "my_project", opts)
	assert.NoError(t, err)
	require.Len(t, versions, 1)
	assert.Equal(t, "another_version", versions[0].Id)

	opts = GetVersionsOptions{CreatedAfter: start.Add(-3 * time.Minute), CreatedBefore: start.Add(-1 * time.Minute), Requester: evergreen.RepotrackerVersionRequester}
	versions, err = GetVersionsWithOptions(t.Context(), "my_project", opts)
	assert.NoError(t, err)
	require.Len(t, versions, 3)
}

func TestGetMainlineCommitVersionsWithOptions(t *testing.T) {
	assert.NoError(t, db.ClearCollections(VersionCollection, build.Collection, task.Collection, ProjectRefCollection))
	start := time.Now()
	p := ProjectRef{
		Id:         "my_project",
		Identifier: "my_ident",
	}
	assert.NoError(t, p.Insert(t.Context()))
	v := Version{
		Id:                  "my_version",
		Identifier:          "my_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 10,
		CreateTime:          start,
		Activated:           utility.TruePtr(),
	}
	assert.NoError(t, v.Insert(t.Context()))
	v = Version{
		Id:                  "your_version",
		Identifier:          "my_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 9,
		CreateTime:          start.Add(-1 * time.Minute),
		Activated:           utility.TruePtr(),
	}
	assert.NoError(t, v.Insert(t.Context()))
	v = Version{
		Id:                  "another_version",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 8,
		Identifier:          "my_project",
		CreateTime:          start.Add(-2 * time.Minute),
		Activated:           utility.FalsePtr(),
	}
	assert.NoError(t, v.Insert(t.Context()))
	v = Version{
		Id:                  "yet_another_version",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 7,
		Identifier:          "my_project",
		CreateTime:          start.Add(-2 * time.Minute),
	}
	assert.NoError(t, v.Insert(t.Context()))

	opts := MainlineCommitVersionOptions{
		Limit:      4,
		Requesters: evergreen.SystemVersionRequesterTypes,
	}
	ctx := context.TODO()
	versions, err := GetMainlineCommitVersionsWithOptions(ctx, p.Id, opts)
	assert.NoError(t, err)
	assert.Len(t, versions, 4)
	assert.EqualValues(t, "my_version", versions[0].Id)
	assert.EqualValues(t, "your_version", versions[1].Id)
	assert.EqualValues(t, "another_version", versions[2].Id)
	assert.EqualValues(t, "yet_another_version", versions[3].Id)

	opts = MainlineCommitVersionOptions{
		Limit:           4,
		SkipOrderNumber: 10,
		Requesters:      evergreen.SystemVersionRequesterTypes,
	}
	versions, err = GetMainlineCommitVersionsWithOptions(ctx, p.Id, opts)
	assert.NoError(t, err)
	assert.Len(t, versions, 3)
	assert.EqualValues(t, "your_version", versions[0].Id)
	assert.EqualValues(t, "another_version", versions[1].Id)
	assert.EqualValues(t, "yet_another_version", versions[2].Id)

	opts = MainlineCommitVersionOptions{
		Limit:           4,
		SkipOrderNumber: 8,
		Requesters:      evergreen.SystemVersionRequesterTypes,
	}
	versions, err = GetMainlineCommitVersionsWithOptions(ctx, p.Id, opts)
	assert.NoError(t, err)
	assert.Len(t, versions, 1)
	assert.EqualValues(t, "yet_another_version", versions[0].Id)

	opts = MainlineCommitVersionOptions{
		Limit:      4,
		Requesters: []string{"Not a real requester"},
	}
	versions, err = GetMainlineCommitVersionsWithOptions(ctx, p.Id, opts)
	assert.Nil(t, versions)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), fmt.Sprintf("invalid requesters %s", opts.Requesters))

}

func TestGetPreviousPageCommit(t *testing.T) {
	assert.NoError(t, db.ClearCollections(VersionCollection, build.Collection, task.Collection, ProjectRefCollection))
	start := time.Now()
	p := ProjectRef{
		Id:         "my_project",
		Identifier: "my_ident",
	}
	assert.NoError(t, p.Insert(t.Context()))
	v := Version{
		Id:                  "my_version",
		Identifier:          "my_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 10,
		CreateTime:          start,
		Activated:           utility.TruePtr(),
	}
	assert.NoError(t, v.Insert(t.Context()))
	v = Version{
		Id:                  "your_version",
		Identifier:          "my_project",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 9,
		CreateTime:          start.Add(-1 * time.Minute),
		Activated:           utility.TruePtr(),
	}
	assert.NoError(t, v.Insert(t.Context()))
	v = Version{
		Id:                  "another_version",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 8,
		Identifier:          "my_project",
		CreateTime:          start.Add(-2 * time.Minute),
		Activated:           utility.FalsePtr(),
	}
	assert.NoError(t, v.Insert(t.Context()))
	v = Version{
		Id:                  "yet_another_version",
		Requester:           evergreen.RepotrackerVersionRequester,
		RevisionOrderNumber: 7,
		Identifier:          "my_project",
		CreateTime:          start.Add(-2 * time.Minute),
	}
	assert.NoError(t, v.Insert(t.Context()))
	ctx := context.TODO()
	// If you are viewing the latest commit it should return nil to indicate that there is no previous page.
	orderNumber, err := GetPreviousPageCommitOrderNumber(ctx, p.Id, 10, 2, evergreen.SystemVersionRequesterTypes)
	assert.NoError(t, err)
	assert.Nil(t, orderNumber)
	// If you are viewing a commit it should return the previous activated version that is LIMIT versions ago.
	orderNumber, err = GetPreviousPageCommitOrderNumber(ctx, p.Id, 7, 2, evergreen.SystemVersionRequesterTypes)
	assert.NoError(t, err)
	assert.NotNil(t, orderNumber)
	assert.Equal(t, 10, utility.FromIntPtr(orderNumber))
	// If the previous pages activated version is the latest it should return 0.
	orderNumber, err = GetPreviousPageCommitOrderNumber(ctx, p.Id, 9, 2, evergreen.SystemVersionRequesterTypes)
	assert.NoError(t, err)
	assert.NotNil(t, orderNumber)
	assert.Equal(t, 0, utility.FromIntPtr(orderNumber))
}

func TestUpdateProjectStorageMethod(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(VersionCollection))
	}()

	for tName, tCase := range map[string]func(t *testing.T, v *Version){
		"ChangesProjectStorageMethod": func(t *testing.T, v *Version) {
			assert.NoError(t, v.UpdateProjectStorageMethod(t.Context(), evergreen.ProjectStorageMethodS3))

			assert.Equal(t, evergreen.ProjectStorageMethodS3, v.ProjectStorageMethod)

			dbVersion, err := VersionFindOneId(t.Context(), v.Id)
			require.NoError(t, err)
			require.NotZero(t, dbVersion)
			assert.Equal(t, evergreen.ProjectStorageMethodS3, dbVersion.ProjectStorageMethod)
		},
		"NoopsWhenVersionStorageMethodIsIdentical": func(t *testing.T, v *Version) {
			v.ProjectStorageMethod = evergreen.ProjectStorageMethodS3
			assert.NoError(t, v.UpdateProjectStorageMethod(t.Context(), evergreen.ProjectStorageMethodS3))

			assert.Equal(t, evergreen.ProjectStorageMethodS3, v.ProjectStorageMethod)

			dbVersion, err := VersionFindOneId(t.Context(), v.Id)
			require.NoError(t, err)
			require.NotZero(t, dbVersion)
			assert.Equal(t, evergreen.ProjectStorageMethodDB, dbVersion.ProjectStorageMethod)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			assert.NoError(t, db.ClearCollections(VersionCollection))
			v := Version{
				Id:                   "id",
				ProjectStorageMethod: evergreen.ProjectStorageMethodDB,
			}
			require.NoError(t, v.Insert(t.Context()))

			tCase(t, &v)
		})
	}

}

func TestUpdatePreGenerationProjectStorageMethod(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(VersionCollection))
	}()

	for tName, tCase := range map[string]func(t *testing.T, v *Version){
		"ChangesPreGenerationProjectStorageMethod": func(t *testing.T, v *Version) {
			assert.NoError(t, v.UpdatePreGenerationProjectStorageMethod(t.Context(), evergreen.ProjectStorageMethodS3))

			assert.Equal(t, evergreen.ProjectStorageMethodS3, v.PreGenerationProjectStorageMethod)

			dbVersion, err := VersionFindOneId(t.Context(), v.Id)
			require.NoError(t, err)
			require.NotZero(t, dbVersion)
			assert.Equal(t, evergreen.ProjectStorageMethodS3, dbVersion.PreGenerationProjectStorageMethod)
		},
		"NoopsWhenVersionStorageMethodIsIdentical": func(t *testing.T, v *Version) {
			v.PreGenerationProjectStorageMethod = evergreen.ProjectStorageMethodS3
			assert.NoError(t, v.UpdatePreGenerationProjectStorageMethod(t.Context(), evergreen.ProjectStorageMethodS3))

			assert.Equal(t, evergreen.ProjectStorageMethodS3, v.PreGenerationProjectStorageMethod)

			dbVersion, err := VersionFindOneId(t.Context(), v.Id)
			require.NoError(t, err)
			require.NotZero(t, dbVersion)
			assert.Equal(t, evergreen.ProjectStorageMethodDB, dbVersion.PreGenerationProjectStorageMethod)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			assert.NoError(t, db.ClearCollections(VersionCollection))
			v := Version{
				Id:                                "id",
				PreGenerationProjectStorageMethod: evergreen.ProjectStorageMethodDB,
			}
			require.NoError(t, v.Insert(t.Context()))

			tCase(t, &v)
		})
	}

}

func TestGetBuildVariants(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(VersionCollection))
	}()

	checkBuildVariantsMatch := func(t *testing.T, bvs0, bvs1 []VersionBuildStatus) {
		require.Len(t, bvs0, len(bvs1), "lengths should match")
		for i := range bvs0 {
			bv0 := bvs0[i]
			bv1 := bvs1[i]
			assert.Equal(t, bv0.BuildVariant, bv1.BuildVariant)
			assert.Equal(t, bv0.DisplayName, bv1.DisplayName)
			assert.Equal(t, bv0.BuildId, bv1.BuildId)
			assert.Equal(t, bv0.ActivationStatus.Activated, bv1.ActivationStatus.Activated)
			assert.WithinDuration(t, bv0.ActivationStatus.ActivateAt, bv1.ActivationStatus.ActivateAt, 0)
		}
	}

	for tName, tCase := range map[string]func(t *testing.T, v *Version){
		"LoadsBuildVariantsIdempotently": func(t *testing.T, v *Version) {
			activateAt := utility.BSONTime(time.Now().Add(time.Hour))
			v.BuildVariants = []VersionBuildStatus{
				{
					BuildVariant: "bv",
					DisplayName:  "bv_name",
					BuildId:      "build_id",
					ActivationStatus: ActivationStatus{
						Activated:  false,
						ActivateAt: activateAt,
					},
				},
			}
			require.NoError(t, v.Insert(t.Context()))

			dbVersion, err := VersionFindOneId(t.Context(), v.Id)
			require.NoError(t, err)
			require.NotZero(t, dbVersion)
			assert.Nil(t, dbVersion.BuildVariants, "build variants should not be loaded by default from query")

			bvs, err := dbVersion.GetBuildVariants(t.Context())
			require.NoError(t, err)
			checkBuildVariantsMatch(t, v.BuildVariants, bvs)
			checkBuildVariantsMatch(t, v.BuildVariants, dbVersion.BuildVariants)

			bvs, err = dbVersion.GetBuildVariants(t.Context())
			require.NoError(t, err)
			checkBuildVariantsMatch(t, v.BuildVariants, bvs)
			checkBuildVariantsMatch(t, v.BuildVariants, dbVersion.BuildVariants)
		},
		"ReturnsBuildVariantsIfAlreadySetInMemory": func(t *testing.T, v *Version) {
			activateAt := utility.BSONTime(time.Now().Add(time.Hour))
			originalBVs := []VersionBuildStatus{
				{
					BuildVariant: "bv",
					DisplayName:  "bv_name",
					BuildId:      "build_id",
					ActivationStatus: ActivationStatus{
						Activated:  false,
						ActivateAt: activateAt,
					},
				},
			}
			v.BuildVariants = originalBVs

			bvs, err := v.GetBuildVariants(t.Context())
			require.NoError(t, err)
			checkBuildVariantsMatch(t, originalBVs, bvs)
			checkBuildVariantsMatch(t, originalBVs, v.BuildVariants)
		},
		"ReturnsEmptyBuildVariantsIfNotSetInDB": func(t *testing.T, v *Version) {
			require.NoError(t, v.Insert(t.Context()))
			bvs, err := v.GetBuildVariants(t.Context())
			require.NoError(t, err)
			assert.NotNil(t, bvs, "loaded build variants should be empty slice, not nil")
			assert.Empty(t, bvs, "loaded build variants should be empty slice")
			assert.Equal(t, bvs, v.BuildVariants)
		},
		"ReturnsEmptyBuildVariantsIfVersionNotInDB": func(t *testing.T, v *Version) {
			bvs, err := v.GetBuildVariants(t.Context())
			require.NoError(t, err)
			assert.NotNil(t, bvs, "loaded build variants should be empty slice, not nil")
			assert.Empty(t, bvs, "loaded build variants should be empty slice")
			assert.Equal(t, bvs, v.BuildVariants)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(VersionCollection))

			v := Version{
				Id: "version_id",
			}

			tCase(t, &v)
		})
	}
}

func TestUpdateAggregateTaskCosts(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, db.ClearCollections(VersionCollection, taskCollection, oldTaskCollection))

	t.Run("AggregatesTaskAndPredictedCosts", func(t *testing.T) {
		v := &Version{Id: "v1"}
		require.NoError(t, v.Insert(ctx))

		// Insert tasks using BSON directly to avoid import cycle
		require.NoError(t, db.Insert(ctx, taskCollection, bson.M{
			"_id": "t1", "version": "v1", "display_only": false,
			"cost":           bson.M{"on_demand_ec2_cost": 10.0, "adjusted_ec2_cost": 8.0},
			"predicted_cost": bson.M{"on_demand_ec2_cost": 3.0, "adjusted_ec2_cost": 2.4},
		}))
		require.NoError(t, db.Insert(ctx, taskCollection, bson.M{
			"_id": "t2", "version": "v1", "display_only": false,
			"cost":           bson.M{"on_demand_ec2_cost": 5.0, "adjusted_ec2_cost": 4.0},
			"predicted_cost": bson.M{"on_demand_ec2_cost": 2.0, "adjusted_ec2_cost": 1.6},
		}))

		err := v.UpdateAggregateTaskCosts(ctx)
		require.NoError(t, err)
		assert.InDelta(t, 15.0, v.Cost.OnDemandEC2Cost, 0.01)
		assert.InDelta(t, 12.0, v.Cost.AdjustedEC2Cost, 0.01)
		assert.InDelta(t, 5.0, v.PredictedCost.OnDemandEC2Cost, 0.01)
		assert.InDelta(t, 4.0, v.PredictedCost.AdjustedEC2Cost, 0.01)
	})

	t.Run("ExcludesDisplayOnlyTasks", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(VersionCollection, taskCollection))
		v := &Version{Id: "v2"}
		require.NoError(t, v.Insert(ctx))

		require.NoError(t, db.Insert(ctx, taskCollection, bson.M{
			"_id": "t1", "version": "v2", "display_only": false,
			"cost": bson.M{"on_demand_ec2_cost": 10.0, "adjusted_ec2_cost": 8.0},
		}))
		require.NoError(t, db.Insert(ctx, taskCollection, bson.M{
			"_id": "display", "version": "v2", "display_only": true,
			"cost": bson.M{"on_demand_ec2_cost": 100.0, "adjusted_ec2_cost": 80.0},
		}))

		err := v.UpdateAggregateTaskCosts(ctx)
		require.NoError(t, err)
		assert.InDelta(t, 10.0, v.Cost.OnDemandEC2Cost, 0.01)
		assert.InDelta(t, 8.0, v.Cost.AdjustedEC2Cost, 0.01)
	})

	t.Run("IncludesOldTaskExecutions", func(t *testing.T) {
		require.NoError(t, db.ClearCollections(VersionCollection, taskCollection, oldTaskCollection))
		v := &Version{Id: "v3"}
		require.NoError(t, v.Insert(ctx))

		require.NoError(t, db.Insert(ctx, taskCollection, bson.M{
			"_id": "t1", "version": "v3", "display_only": false,
			"cost": bson.M{"on_demand_ec2_cost": 10.0, "adjusted_ec2_cost": 8.0},
		}))
		require.NoError(t, db.Insert(ctx, oldTaskCollection, bson.M{
			"_id": "t1_old", "version": "v3", "display_only": false,
			"cost": bson.M{"on_demand_ec2_cost": 5.0, "adjusted_ec2_cost": 4.0},
		}))

		err := v.UpdateAggregateTaskCosts(ctx)
		require.NoError(t, err)
		assert.InDelta(t, 15.0, v.Cost.OnDemandEC2Cost, 0.01)
		assert.InDelta(t, 12.0, v.Cost.AdjustedEC2Cost, 0.01)
	})
}
