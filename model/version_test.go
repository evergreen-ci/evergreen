package model

import (
	"encoding/json"
	"sort"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/utility"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
			lastGood, err := FindVersionByLastKnownGoodConfig(identifier, -1)
			require.NoError(t, err, "error finding last known good")
			So(lastGood, ShouldBeNil)
		})
		Convey("a version should be returned if there is a last known good configuration", func() {
			v := &Version{
				Identifier: identifier,
				Requester:  evergreen.RepotrackerVersionRequester,
			}
			require.NoError(t, v.Insert(), "Error inserting test version: %s", v.Id)
			lastGood, err := FindVersionByLastKnownGoodConfig(identifier, -1)
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
			lastGood, err := FindVersionByLastKnownGoodConfig(identifier, -1)
			require.NoError(t, err, "error finding last known good: %s", v.Id)
			So(lastGood, ShouldNotBeNil)
			So(lastGood.Config, ShouldEqual, "5")
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

func TestVersionExistsForCommitQueueIssue(t *testing.T) {
	assert.NoError(t, db.Clear(VersionCollection))
	v1 := Version{Id: "version-1234"}
	assert.NoError(t, v1.Insert())

	for testName, testCase := range map[string]struct {
		cq        *commitqueue.CommitQueue
		patchType string
	}{
		"CLIQueue": {
			cq: &commitqueue.CommitQueue{
				Queue: []commitqueue.CommitQueueItem{
					{Issue: "version-1234"},
					{Issue: "patch-2345"},
				},
			},
			patchType: commitqueue.CLIPatchType,
		},
		"PRQueue": {
			cq: &commitqueue.CommitQueue{
				Queue: []commitqueue.CommitQueueItem{
					{Issue: "1234", Version: "version-1234"},
					{Issue: "2345"},
				},
			},
			patchType: commitqueue.PRPatchType,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			exists, err := VersionExistsForCommitQueueItem(testCase.cq, testCase.cq.Queue[0].Issue, testCase.patchType)
			assert.NoError(t, err)
			assert.True(t, exists)

			exists, err = VersionExistsForCommitQueueItem(testCase.cq, testCase.cq.Queue[1].Issue, testCase.patchType)
			assert.NoError(t, err)
			assert.False(t, exists)
		})
	}
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
	assert.Equal(t, bv.BatchTimeTasks[0].TaskId, "t1")
	assert.Equal(t, bv.BatchTimeTasks[0].TaskName, "t1_name")
	assert.Equal(t, bv.BatchTimeTasks[0].Activated, false)
	assert.True(t, utility.IsZeroTime(bv.BatchTimeTasks[0].ActivateAt))
}
