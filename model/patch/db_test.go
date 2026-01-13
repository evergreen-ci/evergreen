package patch

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/user"
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
	assert.NoError(t, previousPatch.Insert(t.Context()))
	yourPatch := Patch{
		Id:         bson.NewObjectId(),
		Project:    "correct",
		Author:     "you",
		CreateTime: now,
		Activated:  true,
	}
	assert.NoError(t, yourPatch.Insert(t.Context()))
	notActivatedPatch := Patch{
		Id:         bson.NewObjectId(),
		Project:    "correct",
		Author:     "you",
		CreateTime: now,
		Activated:  false,
	}
	assert.NoError(t, notActivatedPatch.Insert(t.Context()))
	wrongPatch := Patch{
		Id:         bson.NewObjectId(),
		Project:    "wrong",
		Author:     "me",
		CreateTime: now,
		Activated:  true,
	}
	assert.NoError(t, wrongPatch.Insert(t.Context()))
	prPatch := Patch{
		Id:         bson.NewObjectId(),
		Project:    "correct",
		Author:     "me",
		CreateTime: now,
		Alias:      evergreen.GithubPRAlias,
		Activated:  true,
	}
	assert.NoError(t, prPatch.Insert(t.Context()))
	oldPatch := Patch{
		Id:         bson.NewObjectId(),
		Project:    "correct",
		Author:     "me",
		CreateTime: now.Add(-time.Minute),
		Activated:  true,
	}
	assert.NoError(t, oldPatch.Insert(t.Context()))

	p, err := FindOne(t.Context(), MostRecentPatchByUserAndProject("me", "correct"))
	assert.NoError(t, err)
	assert.NotNil(t, p)
	assert.Equal(t, p.Id, previousPatch.Id)
}
func TestByPatchNameStatusesMergeQueuePaginatedRequestersOption(t *testing.T) {
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T){
		"EmptyRequestersList": func(ctx context.Context, t *testing.T) {
			opts := ByPatchNameStatusesMergeQueuePaginatedOptions{
				Project:    utility.ToStringPtr("evergreen"),
				Requesters: []string{},
			}

			patches, count, err := ByPatchNameStatusesMergeQueuePaginated(ctx, opts)
			assert.NoError(t, err)
			assert.Equal(t, 3, count)
			require.Len(t, patches, 3)
		},
		"GithubPRRequester": func(ctx context.Context, t *testing.T) {
			opts := ByPatchNameStatusesMergeQueuePaginatedOptions{
				Project:    utility.ToStringPtr("evergreen"),
				Requesters: []string{evergreen.GithubPRRequester},
			}
			patches, count, err := ByPatchNameStatusesMergeQueuePaginated(ctx, opts)
			assert.NoError(t, err)
			assert.Equal(t, 1, count)
			require.Len(t, patches, 1)
			assert.Equal(t, "GH PR Patch", patches[0].Description)
		},
		"GithubMergeRequester": func(ctx context.Context, t *testing.T) {
			opts := ByPatchNameStatusesMergeQueuePaginatedOptions{
				Project:    utility.ToStringPtr("evergreen"),
				Requesters: []string{evergreen.GithubMergeRequester},
			}
			patches, count, err := ByPatchNameStatusesMergeQueuePaginated(ctx, opts)
			assert.NoError(t, err)
			assert.Equal(t, 1, count)
			require.Len(t, patches, 1)
			assert.Equal(t, "GH Merge Patch", patches[0].Description)
		},
		"PatchVersionRequester": func(ctx context.Context, t *testing.T) {
			opts := ByPatchNameStatusesMergeQueuePaginatedOptions{
				Project:    utility.ToStringPtr("evergreen"),
				Requesters: []string{evergreen.PatchVersionRequester},
			}
			patches, count, err := ByPatchNameStatusesMergeQueuePaginated(ctx, opts)
			assert.NoError(t, err)
			assert.Equal(t, 1, count)
			require.Len(t, patches, 1)
			assert.Equal(t, "Patch Request Patch", patches[0].Description)
		},
		"MultipleRequesters": func(ctx context.Context, t *testing.T) {
			opts := ByPatchNameStatusesMergeQueuePaginatedOptions{
				Project:    utility.ToStringPtr("evergreen"),
				Requesters: []string{evergreen.PatchVersionRequester, evergreen.GithubMergeRequester},
			}
			patches, count, err := ByPatchNameStatusesMergeQueuePaginated(ctx, opts)
			assert.NoError(t, err)
			assert.Equal(t, 2, count)
			require.Len(t, patches, 2)
			assert.Equal(t, "GH Merge Patch", patches[0].Description)
			assert.Equal(t, "Patch Request Patch", patches[1].Description)
		},
		"NoRequestersList": func(ctx context.Context, t *testing.T) {
			opts := ByPatchNameStatusesMergeQueuePaginatedOptions{
				Project: utility.ToStringPtr("evergreen"),
			}
			patches, count, err := ByPatchNameStatusesMergeQueuePaginated(ctx, opts)
			assert.NoError(t, err)
			assert.Equal(t, 3, count)
			require.Len(t, patches, 3)

			opts = ByPatchNameStatusesMergeQueuePaginatedOptions{
				Project:    utility.ToStringPtr("evergreen"),
				Requesters: []string{},
			}
			patches, count, err = ByPatchNameStatusesMergeQueuePaginated(ctx, opts)
			assert.NoError(t, err)
			assert.Equal(t, 3, count)
			require.Len(t, patches, 3)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			assert.NoError(t, db.ClearCollections(Collection))
			ghPRPatch := Patch{
				Id:          bson.NewObjectId(),
				Project:     "evergreen",
				Description: "GH PR Patch",
				GithubPatchData: thirdparty.GithubPatch{
					HeadOwner: "me", // indicates github_pull_request requester
				},
			}
			assert.NoError(t, ghPRPatch.Insert(t.Context()))
			ghMergePatch := Patch{
				Id:          bson.NewObjectId(),
				Project:     "evergreen",
				Description: "GH Merge Patch",
				GithubMergeData: thirdparty.GithubMergeGroup{
					HeadSHA: "head_sha_value", // indicates github_merge_test requester
				},
			}
			assert.NoError(t, ghMergePatch.Insert(t.Context()))

			patchRequestPatch := Patch{
				Id:          bson.NewObjectId(),
				Project:     "evergreen",
				Description: "Patch Request Patch", // patch_request requester
			}
			assert.NoError(t, patchRequestPatch.Insert(t.Context()))
			tCase(ctx, t)
		})
	}
}
func TestByPatchNameStatusesMergeQueuePaginated(t *testing.T) {
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
			patch.GithubMergeData.HeadSHA = "head_sha_value"
		}
		assert.NoError(t, patch.Insert(t.Context()))
	}
	opts := ByPatchNameStatusesMergeQueuePaginatedOptions{
		Project: utility.ToStringPtr("evergreen"),
	}
	ctx := context.TODO()
	patches, count, err := ByPatchNameStatusesMergeQueuePaginated(ctx, opts)
	assert.NoError(t, err)
	assert.Equal(t, 10, count)
	assert.Len(t, patches, 10)

	// Test pagination
	opts = ByPatchNameStatusesMergeQueuePaginatedOptions{
		Project: utility.ToStringPtr("evergreen"),
		Limit:   5,
		Page:    0,
	}
	patches, count, err = ByPatchNameStatusesMergeQueuePaginated(ctx, opts)
	assert.NoError(t, err)
	assert.Equal(t, 10, count)
	assert.Len(t, patches, 5)
	assert.Equal(t, "patch 0", patches[0].Description)

	opts = ByPatchNameStatusesMergeQueuePaginatedOptions{
		Project: utility.ToStringPtr("evergreen"),
		Limit:   5,
		Page:    1,
	}
	patches, count, err = ByPatchNameStatusesMergeQueuePaginated(ctx, opts)
	assert.NoError(t, err)
	assert.Equal(t, 10, count)
	assert.Len(t, patches, 5)
	assert.Equal(t, "patch 5", patches[0].Description)

	opts = ByPatchNameStatusesMergeQueuePaginatedOptions{
		Project:    utility.ToStringPtr("evergreen"),
		Requesters: []string{evergreen.GithubMergeRequester},
	}
	patches, count, err = ByPatchNameStatusesMergeQueuePaginated(ctx, opts)
	assert.NoError(t, err)
	assert.Equal(t, 5, count)
	assert.Len(t, patches, 5)
	for _, patch := range patches {
		assert.True(t, evergreen.IsGithubMergeQueueRequester(patch.GetRequester()))
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

	assert.NoError(t, db.InsertMany(t.Context(), Collection, p, childPatch, childPatch2))
	childPatchIds, err := GetFinalizedChildPatchIdsForPatch(t.Context(), p.Id.Hex())
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

	assert.NoError(t, db.InsertMany(t.Context(), Collection, patch1, patch2, cqPatch, wrongPRPatch))
	p, err := FindLatestGithubPRPatch(t.Context(), "parks", "rec", 12)
	assert.NoError(t, err)
	require.NotNil(t, p)
	assert.Equal(t, p.Id.Hex(), patch2.Id.Hex())
}

func TestConsolidatePatchesForUser(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection, user.Collection))
	p1 := Patch{
		Id:          bson.NewObjectId(),
		Author:      "me",
		PatchNumber: 6,
	}
	p2 := Patch{
		Id:          bson.NewObjectId(),
		Author:      "me",
		PatchNumber: 7,
	}
	pNew := Patch{
		Id:          bson.NewObjectId(),
		Author:      "new_me",
		PatchNumber: 1,
	}
	pNewAlso := Patch{
		Id:          bson.NewObjectId(),
		Author:      "new_me",
		PatchNumber: 2,
	}
	assert.NoError(t, db.InsertMany(t.Context(), Collection, p1, p2, pNew, pNewAlso))

	newUsr := &user.DBUser{
		Id:          "new_me",
		PatchNumber: 7,
	}
	assert.NoError(t, db.Insert(t.Context(), user.Collection, newUsr))
	assert.NoError(t, ConsolidatePatchesForUser(t.Context(), "me", newUsr))

	patchFromDB, err := FindOneId(t.Context(), p1.Id.Hex())
	assert.NoError(t, err)
	require.NotNil(t, patchFromDB)
	assert.Equal(t, "new_me", patchFromDB.Author)
	assert.Equal(t, p1.PatchNumber, patchFromDB.PatchNumber)

	patchFromDB, err = FindOneId(t.Context(), p2.Id.Hex())
	assert.NoError(t, err)
	require.NotNil(t, patchFromDB)
	assert.Equal(t, "new_me", patchFromDB.Author)
	assert.Equal(t, p2.PatchNumber, patchFromDB.PatchNumber)

	patchFromDB, err = FindOneId(t.Context(), pNew.Id.Hex())
	assert.NoError(t, err)
	require.NotNil(t, patchFromDB)
	assert.Equal(t, "new_me", patchFromDB.Author)
	assert.Equal(t, 8, patchFromDB.PatchNumber)

	patchFromDB, err = FindOneId(t.Context(), pNewAlso.Id.Hex())
	assert.NoError(t, err)
	require.NotNil(t, patchFromDB)
	assert.Equal(t, "new_me", patchFromDB.Author)
	assert.Equal(t, 9, patchFromDB.PatchNumber)

	usr, err := user.FindOneByIdContext(t.Context(), "new_me")
	assert.NoError(t, err)
	require.NotNil(t, usr)
	assert.Equal(t, 9, usr.PatchNumber)
}
