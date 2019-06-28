package model

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
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
			require.NoError(t, v.Insert(), "Error inserting test version: %s", v.Id)
			lastGood, err := VersionFindOne(VersionByLastKnownGoodConfig(identifier))
			require.NoError(t, err, "error finding last known good")
			So(lastGood, ShouldBeNil)
		})
		Convey("a version should be returned if there is a last known good configuration", func() {
			v := &Version{
				Identifier: identifier,
				Requester:  evergreen.RepotrackerVersionRequester,
			}
			require.NoError(t, v.Insert(), "Error inserting test version: %s", v.Id)
			lastGood, err := VersionFindOne(VersionByLastKnownGoodConfig(identifier))
			require.NoError(t, err, "error finding last known good: %s", lastGood.Id)
			So(lastGood, ShouldNotBeNil)
		})
		Convey("most recent version should be found if there are several recent good configs", func() {
			v := &Version{
				Id:                  "1",
				Identifier:          identifier,
				Requester:           evergreen.RepotrackerVersionRequester,
				RevisionOrderNumber: 1,
				Config:              "1",
			}
			require.NoError(t, v.Insert(), "Error inserting test version: %s", v.Id)
			v.Id = "5"
			v.RevisionOrderNumber = 5
			v.Config = "5"
			require.NoError(t, v.Insert(), "Error inserting test version: %s", v.Id)
			v.Id = "2"
			v.RevisionOrderNumber = 2
			v.Config = "2"
			require.NoError(t, v.Insert(), "Error inserting test version: %s", v.Id)
			lastGood, err := VersionFindOne(VersionByLastKnownGoodConfig(identifier))
			require.NoError(t, err, "error finding last known good: %s", v.Id)
			So(lastGood, ShouldNotBeNil)
			So(lastGood.Config, ShouldEqual, "5")
		})
		Reset(func() {
			So(db.Clear(VersionCollection), ShouldBeNil)
		})
	})
}

func TestVersionSortByOrder(t *testing.T) {
	assert := assert.New(t)
	versions := VersionsByOrder{
		{Id: "5", RevisionOrderNumber: 5},
		{Id: "3", RevisionOrderNumber: 3},
		{Id: "1", RevisionOrderNumber: 1},
		{Id: "4", RevisionOrderNumber: 4},
		{Id: "100", RevisionOrderNumber: 100},
	}
	sort.Sort(versions)
	assert.Equal("1", versions[0].Id)
	assert.Equal("3", versions[1].Id)
	assert.Equal("4", versions[2].Id)
	assert.Equal("5", versions[3].Id)
	assert.Equal("100", versions[4].Id)
}

func TestUpdateMergeTaskDependencies(t *testing.T) {
	require.NoError(t, db.ClearCollections(task.Collection, ProjectAliasCollection))

	project := &Project{
		Identifier: "evergreen",
		BuildVariants: []BuildVariant{
			{
				Name: evergreen.MergeTaskVariant,
				Tasks: []BuildVariantTaskUnit{
					{Name: evergreen.MergeTaskName},
				},
			},
			{
				Name: "v1",
				Tasks: []BuildVariantTaskUnit{
					{Name: "t1"},
				},
			},
		},
		Tasks: []ProjectTask{
			{Name: evergreen.MergeTaskName},
			{Name: "t1"},
		},
	}

	version := &Version{
		Id:         "v1",
		Revision:   "abcdef",
		Requester:  evergreen.MergeTestRequester,
		CreateTime: time.Date(2009, time.November, 10, 23, 0, 0, 0, time.UTC),
	}

	taskID := fmt.Sprintf("%s_%s_%s_%s_%s",
		project.Identifier,
		project.BuildVariants[0].Name,
		evergreen.MergeTaskName,
		fmt.Sprintf("patch_%s_%s", version.Revision, version.Id),
		version.CreateTime.Format(build.IdTimeLayout))
	taskID = util.CleanName(taskID)

	mergeTask := task.Task{
		Id: taskID,
	}
	assert.NoError(t, mergeTask.Insert())

	alias := ProjectAlias{
		ProjectID: "evergreen",
		Alias:     evergreen.CommitQueueAlias,
		Variant:   "v1",
		Task:      "t1",
	}
	assert.NoError(t, alias.Upsert())

	assert.NoError(t, version.UpdateMergeTaskDependencies(project))

	tDb, err := task.FindOneId(taskID)
	assert.NoError(t, err)
	require.Len(t, tDb.DependsOn, 1)
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
	assert.NoError(v1.Insert())
	v2 := Version{
		Id:              "v2",
		PeriodicBuildID: "a",
		Identifier:      "myProj",
		CreateTime:      now.Add(-5 * time.Minute),
	}
	assert.NoError(v2.Insert())
	v3 := Version{
		Id:              "v3",
		PeriodicBuildID: "b",
		Identifier:      "myProj",
		CreateTime:      now,
	}
	assert.NoError(v3.Insert())
	v4 := Version{
		Id:              "v4",
		PeriodicBuildID: "a",
		Identifier:      "someProj",
		CreateTime:      now,
	}
	assert.NoError(v4.Insert())

	mostRecent, err := FindLastPeriodicBuild("myProj", "a")
	assert.NoError(err)
	assert.Equal(v2.Id, mostRecent.Id)
}
