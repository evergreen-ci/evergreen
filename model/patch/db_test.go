package patch

import (
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMostRecentByUserAndProject(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection))

	now := time.Now()
	previousPatch := Patch{
		Id:         bson.NewObjectId(),
		Project:    "correct",
		Author:     "me",
		CreateTime: now,
		Activated:  true,
	}
	assert.NoError(t, previousPatch.Insert())
	yourPatch := Patch{
		Id:         bson.NewObjectId(),
		Project:    "correct",
		Author:     "you",
		CreateTime: now,
		Activated:  true,
	}
	assert.NoError(t, yourPatch.Insert())
	notActivatedPatch := Patch{
		Id:         bson.NewObjectId(),
		Project:    "correct",
		Author:     "you",
		CreateTime: now,
		Activated:  false,
	}
	assert.NoError(t, notActivatedPatch.Insert())
	wrongPatch := Patch{
		Id:         bson.NewObjectId(),
		Project:    "wrong",
		Author:     "me",
		CreateTime: now,
		Activated:  true,
	}
	assert.NoError(t, wrongPatch.Insert())
	prPatch := Patch{
		Id:         bson.NewObjectId(),
		Project:    "correct",
		Author:     "me",
		CreateTime: now,
		Alias:      evergreen.GithubPRAlias,
		Activated:  true,
	}
	assert.NoError(t, prPatch.Insert())
	oldPatch := Patch{
		Id:         bson.NewObjectId(),
		Project:    "correct",
		Author:     "me",
		CreateTime: now.Add(-time.Minute),
		Activated:  true,
	}
	assert.NoError(t, oldPatch.Insert())

	p, err := FindOne(MostRecentPatchByUserAndProject("me", "correct"))
	assert.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, p.Id, previousPatch.Id)
}

func TestByPatchNameStatusesCommitQueuePaginated(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection))

	now := time.Now()
	for i := 0; i < 10; i++ {
		isCommitQueue := i%2 == 0
		createTime := time.Duration(i) * time.Minute

		patch := Patch{
			Id:          bson.NewObjectId(),
			Project:     "evergreen",
			CreateTime:  now.Add(-createTime),
			Description: fmt.Sprintf("patch %d", i),
		}
		if isCommitQueue {
			patch.Alias = evergreen.CommitQueueAlias
		}
		assert.NoError(t, patch.Insert())
	}
	opts := ByPatchNameStatusesCommitQueuePaginatedOptions{
		Project:            utility.ToStringPtr("evergreen"),
		IncludeCommitQueue: utility.TruePtr(),
	}
	patches, count, err := ByPatchNameStatusesCommitQueuePaginated(opts)
	assert.NoError(t, err)
	assert.Equal(t, 10, count)
	assert.Equal(t, 10, len(patches))

	// Test pagination
	opts = ByPatchNameStatusesCommitQueuePaginatedOptions{
		Project:            utility.ToStringPtr("evergreen"),
		IncludeCommitQueue: utility.TruePtr(),
		Limit:              5,
		Page:               0,
	}
	patches, count, err = ByPatchNameStatusesCommitQueuePaginated(opts)
	assert.NoError(t, err)
	assert.Equal(t, 10, count)
	assert.Equal(t, 5, len(patches))
	assert.Equal(t, "patch 0", patches[0].Description)

	opts = ByPatchNameStatusesCommitQueuePaginatedOptions{
		Project:            utility.ToStringPtr("evergreen"),
		IncludeCommitQueue: utility.TruePtr(),
		Limit:              5,
		Page:               1,
	}
	patches, count, err = ByPatchNameStatusesCommitQueuePaginated(opts)
	assert.NoError(t, err)
	assert.Equal(t, 10, count)
	assert.Equal(t, 5, len(patches))
	assert.Equal(t, "patch 5", patches[0].Description)

	// Test filtering by commit queue
	opts = ByPatchNameStatusesCommitQueuePaginatedOptions{
		Project:            utility.ToStringPtr("evergreen"),
		IncludeCommitQueue: utility.FalsePtr(),
	}
	patches, count, err = ByPatchNameStatusesCommitQueuePaginated(opts)
	assert.NoError(t, err)
	assert.Equal(t, 5, count)
	assert.Equal(t, 5, len(patches))
	for _, patch := range patches {
		assert.NotEqual(t, evergreen.CommitQueueAlias, patch.Alias)
	}
	opts = ByPatchNameStatusesCommitQueuePaginatedOptions{
		Project:         utility.ToStringPtr("evergreen"),
		OnlyCommitQueue: utility.TruePtr(),
	}
	patches, count, err = ByPatchNameStatusesCommitQueuePaginated(opts)
	assert.NoError(t, err)
	assert.Equal(t, 5, count)
	assert.Equal(t, 5, len(patches))
	for _, patch := range patches {
		assert.Equal(t, evergreen.CommitQueueAlias, patch.Alias)
	}
}

func TestGetFinalizedChildPatchIdsForPatch(t *testing.T) {
	childPatch := Patch{
		Id:      bson.NewObjectId(),
		Version: "myVersion",
	}
	childPatch2 := Patch{
		Id: bson.NewObjectId(), // not yet finalized
	}

	p := Patch{
		Id: bson.NewObjectId(),
		Triggers: TriggerInfo{
			ChildPatches: []string{childPatch.Id.Hex(), childPatch2.Id.Hex()},
		},
	}

	assert.NoError(t, db.InsertMany(Collection, p, childPatch, childPatch2))
	childPatchIds, err := GetFinalizedChildPatchIdsForPatch(p.Id.Hex())
	assert.NoError(t, err)
	require.Len(t, childPatchIds, 1)
	assert.Equal(t, childPatchIds[0], childPatch.Id.Hex())
}

func TestLatestGithubPRPatch(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection))
	patch1 := Patch{
		Id:         bson.NewObjectId(),
		CreateTime: time.Now().Add(-time.Hour),
		GithubPatchData: thirdparty.GithubPatch{
			BaseOwner: "parks",
			BaseRepo:  "rec",
			PRNumber:  12,
		},
	}
	patch2 := Patch{
		Id:         bson.NewObjectId(),
		CreateTime: time.Now(),
		GithubPatchData: thirdparty.GithubPatch{
			BaseOwner: "parks",
			BaseRepo:  "rec",
			PRNumber:  12,
		},
	}
	cqPatch := Patch{
		Id:         bson.NewObjectId(),
		CreateTime: time.Now().Add(time.Hour),
		Alias:      evergreen.CommitQueueAlias,
		GithubPatchData: thirdparty.GithubPatch{
			BaseOwner: "parks",
			BaseRepo:  "rec",
			PRNumber:  12,
		},
	}
	wrongPRPatch := Patch{
		Id:         bson.NewObjectId(),
		CreateTime: time.Now(),
		GithubPatchData: thirdparty.GithubPatch{
			BaseOwner: "parks",
			BaseRepo:  "rec",
			PRNumber:  14,
		},
	}

	assert.NoError(t, db.InsertMany(Collection, patch1, patch2, cqPatch, wrongPRPatch))
	p, err := FindLatestGithubPRPatch("parks", "rec", 12)
	assert.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, p.Id.Hex(), patch2.Id.Hex())
}
