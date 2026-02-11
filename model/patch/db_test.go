package patch

import (
	"context"
	"fmt"
	"math"
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
	"go.mongodb.org/mongo-driver/mongo"
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
func TestProjectOrUserPatchesRequestersOption(t *testing.T) {
	assert.NoError(t, db.EnsureIndex(Collection, mongo.IndexModel{Keys: ProjectCreateTimeIndex}))

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T){
		"EmptyRequestersList": func(ctx context.Context, t *testing.T) {
			opts := ProjectOrUserPatchesOptions{
				Project:    utility.ToStringPtr("evergreen"),
				Requesters: []string{},
				CountLimit: 10000,
			}

			patches, err := ProjectOrUserPatchesPage(ctx, opts)
			assert.NoError(t, err)
			require.Len(t, patches, 3)

			count, err := ProjectOrUserPatchesCount(ctx, opts)
			assert.NoError(t, err)
			assert.Equal(t, 3, count)
		},
		"GithubPRRequester": func(ctx context.Context, t *testing.T) {
			opts := ProjectOrUserPatchesOptions{
				Project:    utility.ToStringPtr("evergreen"),
				Requesters: []string{evergreen.GithubPRRequester},
				CountLimit: 10000,
			}
			patches, err := ProjectOrUserPatchesPage(ctx, opts)
			assert.NoError(t, err)
			require.Len(t, patches, 1)
			assert.Equal(t, "GH PR Patch", patches[0].Description)

			count, err := ProjectOrUserPatchesCount(ctx, opts)
			assert.NoError(t, err)
			assert.Equal(t, 1, count)
		},
		"GithubMergeRequester": func(ctx context.Context, t *testing.T) {
			opts := ProjectOrUserPatchesOptions{
				Project:    utility.ToStringPtr("evergreen"),
				Requesters: []string{evergreen.GithubMergeRequester},
				CountLimit: 10000,
			}
			patches, err := ProjectOrUserPatchesPage(ctx, opts)
			assert.NoError(t, err)
			require.Len(t, patches, 1)
			assert.Equal(t, "GH Merge Patch", patches[0].Description)

			count, err := ProjectOrUserPatchesCount(ctx, opts)
			assert.NoError(t, err)
			assert.Equal(t, 1, count)
		},
		"PatchVersionRequester": func(ctx context.Context, t *testing.T) {
			opts := ProjectOrUserPatchesOptions{
				Project:    utility.ToStringPtr("evergreen"),
				Requesters: []string{evergreen.PatchVersionRequester},
				CountLimit: 10000,
			}
			patches, err := ProjectOrUserPatchesPage(ctx, opts)
			assert.NoError(t, err)
			require.Len(t, patches, 1)
			assert.Equal(t, "Patch Request Patch", patches[0].Description)

			count, err := ProjectOrUserPatchesCount(ctx, opts)
			assert.NoError(t, err)
			assert.Equal(t, 1, count)
		},
		"MultipleRequesters": func(ctx context.Context, t *testing.T) {
			opts := ProjectOrUserPatchesOptions{
				Project:    utility.ToStringPtr("evergreen"),
				Requesters: []string{evergreen.PatchVersionRequester, evergreen.GithubMergeRequester},
				CountLimit: 10000,
			}
			patches, err := ProjectOrUserPatchesPage(ctx, opts)
			assert.NoError(t, err)
			require.Len(t, patches, 2)
			assert.Equal(t, "GH Merge Patch", patches[0].Description)
			assert.Equal(t, "Patch Request Patch", patches[1].Description)

			count, err := ProjectOrUserPatchesCount(ctx, opts)
			assert.NoError(t, err)
			assert.Equal(t, 2, count)
		},
		"NoRequestersList": func(ctx context.Context, t *testing.T) {
			opts := ProjectOrUserPatchesOptions{
				Project:    utility.ToStringPtr("evergreen"),
				CountLimit: 10000,
			}
			patches, err := ProjectOrUserPatchesPage(ctx, opts)
			assert.NoError(t, err)
			require.Len(t, patches, 3)

			count, err := ProjectOrUserPatchesCount(ctx, opts)
			assert.NoError(t, err)
			assert.Equal(t, 3, count)

			opts = ProjectOrUserPatchesOptions{
				Project:    utility.ToStringPtr("evergreen"),
				Requesters: []string{},
				CountLimit: 10000,
			}
			patches, err = ProjectOrUserPatchesPage(ctx, opts)
			assert.NoError(t, err)
			require.Len(t, patches, 3)

			count, err = ProjectOrUserPatchesCount(ctx, opts)
			assert.NoError(t, err)
			assert.Equal(t, 3, count)
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
func TestProjectOrUserPatchesCombined(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection))
	assert.NoError(t, db.EnsureIndex(Collection, mongo.IndexModel{Keys: ProjectCreateTimeIndex}))

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
	opts := ProjectOrUserPatchesOptions{
		Project:    utility.ToStringPtr("evergreen"),
		CountLimit: 10000,
	}
	ctx := context.TODO()
	patches, err := ProjectOrUserPatchesPage(ctx, opts)
	assert.NoError(t, err)
	assert.Len(t, patches, 10)

	count, err := ProjectOrUserPatchesCount(ctx, opts)
	assert.NoError(t, err)
	assert.Equal(t, 10, count)

	// Test pagination
	opts = ProjectOrUserPatchesOptions{
		Project:    utility.ToStringPtr("evergreen"),
		Limit:      5,
		Page:       0,
		CountLimit: 10000,
	}
	patches, err = ProjectOrUserPatchesPage(ctx, opts)
	assert.NoError(t, err)
	assert.Len(t, patches, 5)
	assert.Equal(t, "patch 0", patches[0].Description)

	count, err = ProjectOrUserPatchesCount(ctx, opts)
	assert.NoError(t, err)
	assert.Equal(t, 10, count)

	opts = ProjectOrUserPatchesOptions{
		Project:    utility.ToStringPtr("evergreen"),
		Limit:      5,
		Page:       1,
		CountLimit: 10000,
	}
	patches, err = ProjectOrUserPatchesPage(ctx, opts)
	assert.NoError(t, err)
	assert.Len(t, patches, 5)
	assert.Equal(t, "patch 5", patches[0].Description)

	count, err = ProjectOrUserPatchesCount(ctx, opts)
	assert.NoError(t, err)
	assert.Equal(t, 10, count)

	opts = ProjectOrUserPatchesOptions{
		Project:    utility.ToStringPtr("evergreen"),
		Requesters: []string{evergreen.GithubMergeRequester},
		CountLimit: 10000,
	}
	patches, err = ProjectOrUserPatchesPage(ctx, opts)
	assert.NoError(t, err)
	assert.Len(t, patches, 5)
	for _, patch := range patches {
		assert.True(t, evergreen.IsGithubMergeQueueRequester(patch.GetRequester()))
	}

	count, err = ProjectOrUserPatchesCount(ctx, opts)
	assert.NoError(t, err)
	assert.Equal(t, 5, count)
}

func TestProjectOrUserPatchesResults(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection))
	assert.NoError(t, db.EnsureIndex(Collection, mongo.IndexModel{Keys: ProjectCreateTimeIndex}))

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

	ctx := context.TODO()

	t.Run("ReturnsAllPatches", func(t *testing.T) {
		opts := ProjectOrUserPatchesOptions{
			Project: utility.ToStringPtr("evergreen"),
		}
		patches, err := ProjectOrUserPatchesPage(ctx, opts)
		assert.NoError(t, err)
		assert.Len(t, patches, 10)
	})

	t.Run("Pagination", func(t *testing.T) {
		opts := ProjectOrUserPatchesOptions{
			Project: utility.ToStringPtr("evergreen"),
			Limit:   5,
			Page:    0,
		}
		patches, err := ProjectOrUserPatchesPage(ctx, opts)
		assert.NoError(t, err)
		assert.Len(t, patches, 5)
		assert.Equal(t, "patch 0", patches[0].Description)

		opts.Page = 1
		patches, err = ProjectOrUserPatchesPage(ctx, opts)
		assert.NoError(t, err)
		assert.Len(t, patches, 5)
		assert.Equal(t, "patch 5", patches[0].Description)
	})

	t.Run("FiltersMergeQueuePatches", func(t *testing.T) {
		opts := ProjectOrUserPatchesOptions{
			Project:    utility.ToStringPtr("evergreen"),
			Requesters: []string{evergreen.GithubMergeRequester},
		}
		patches, err := ProjectOrUserPatchesPage(ctx, opts)
		assert.NoError(t, err)
		assert.Len(t, patches, 5)
		for _, patch := range patches {
			assert.True(t, evergreen.IsGithubMergeQueueRequester(patch.GetRequester()))
		}
	})

	t.Run("ExcludesPatchDiff", func(t *testing.T) {
		// Insert a patch with large diff data
		patchWithDiff := Patch{
			Id:          bson.NewObjectId(),
			Project:     "evergreen",
			CreateTime:  now,
			Description: "patch with diff",
			Patches: []ModulePatch{
				{
					PatchSet: PatchSet{
						Patch: "large diff content here",
					},
				},
			},
		}
		assert.NoError(t, patchWithDiff.Insert(t.Context()))

		opts := ProjectOrUserPatchesOptions{
			Project:   utility.ToStringPtr("evergreen"),
			PatchName: "patch with diff",
		}
		patches, err := ProjectOrUserPatchesPage(ctx, opts)
		assert.NoError(t, err)
		require.Len(t, patches, 1)
		// Verify that the diff data was excluded
		assert.Empty(t, patches[0].Patches[0].PatchSet.Patch)
	})
}

func TestProjectOrUserPatchesCount(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection))
	assert.NoError(t, db.EnsureIndex(Collection, mongo.IndexModel{Keys: ProjectCreateTimeIndex}))

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

	ctx := context.TODO()

	t.Run("CountsAllPatches", func(t *testing.T) {
		opts := ProjectOrUserPatchesOptions{
			Project:    utility.ToStringPtr("evergreen"),
			CountLimit: 10000,
		}
		count, err := ProjectOrUserPatchesCount(ctx, opts)
		assert.NoError(t, err)
		assert.Equal(t, 10, count)
	})

	t.Run("CountsMergeQueuePatches", func(t *testing.T) {
		opts := ProjectOrUserPatchesOptions{
			Project:    utility.ToStringPtr("evergreen"),
			Requesters: []string{evergreen.GithubMergeRequester},
			CountLimit: 10000,
		}
		count, err := ProjectOrUserPatchesCount(ctx, opts)
		assert.NoError(t, err)
		assert.Equal(t, 5, count)
	})

	t.Run("CountsWithPatchNameFilter", func(t *testing.T) {
		opts := ProjectOrUserPatchesOptions{
			Project:    utility.ToStringPtr("evergreen"),
			PatchName:  "patch 5",
			CountLimit: 10000,
		}
		count, err := ProjectOrUserPatchesCount(ctx, opts)
		assert.NoError(t, err)
		assert.Equal(t, 1, count)
	})

	t.Run("ReturnsZeroForNoMatches", func(t *testing.T) {
		opts := ProjectOrUserPatchesOptions{
			Project:    utility.ToStringPtr("nonexistent"),
			PatchName:  "nonexistent patch",
			CountLimit: 10000,
		}
		count, err := ProjectOrUserPatchesCount(ctx, opts)
		assert.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("ReturnsMaxInt32WhenHittingLimit", func(t *testing.T) {
		opts := ProjectOrUserPatchesOptions{
			Project:    utility.ToStringPtr("evergreen"),
			CountLimit: 5, // Set limit lower than actual count
		}
		count, err := ProjectOrUserPatchesCount(ctx, opts)
		assert.NoError(t, err)
		assert.Equal(t, math.MaxInt32, count)
	})
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

	usr, err := user.FindOneById(t.Context(), "new_me")
	assert.NoError(t, err)
	require.NotNil(t, usr)
	assert.Equal(t, 9, usr.PatchNumber)
}

func TestMarkMergeQueuePatchesRemovedFromQueue(t *testing.T) {
	assert.NoError(t, db.ClearCollections(Collection))

	originalTime := time.Now().Add(-time.Hour).UTC().Round(time.Millisecond)
	patches := []Patch{
		{
			Id: bson.NewObjectId(),
			GithubMergeData: thirdparty.GithubMergeGroup{
				Org:     "mongodb",
				Repo:    "mongo",
				HeadSHA: "abc123",
			},
		},
		{
			Id: bson.NewObjectId(),
			GithubMergeData: thirdparty.GithubMergeGroup{
				Org:                "mongodb",
				Repo:               "mongo",
				HeadSHA:            "abc123",
				RemovedFromQueueAt: originalTime,
				RemovalReason:      "original reason",
			},
		},
		{
			Id: bson.NewObjectId(),
			GithubMergeData: thirdparty.GithubMergeGroup{
				Org:     "other-org",
				Repo:    "mongo",
				HeadSHA: "abc123",
			},
		},
	}
	assert.NoError(t, db.InsertMany(t.Context(), Collection, patches[0], patches[1], patches[2]))

	count, err := MarkMergeQueuePatchesRemovedFromQueue(t.Context(), "mongodb", "mongo", "abc123", "merge failed")
	assert.NoError(t, err)
	assert.Equal(t, 1, count)

	p, err := FindOneId(t.Context(), patches[0].Id.Hex())
	assert.NoError(t, err)
	assert.False(t, p.GithubMergeData.RemovedFromQueueAt.IsZero())
	assert.Equal(t, "merge failed", p.GithubMergeData.RemovalReason)

	p, err = FindOneId(t.Context(), patches[1].Id.Hex())
	assert.NoError(t, err)
	assert.Equal(t, originalTime, p.GithubMergeData.RemovedFromQueueAt)
	assert.Equal(t, "original reason", p.GithubMergeData.RemovalReason)

	p, err = FindOneId(t.Context(), patches[2].Id.Hex())
	assert.NoError(t, err)
	assert.True(t, p.GithubMergeData.RemovedFromQueueAt.IsZero())

	count, err = MarkMergeQueuePatchesRemovedFromQueue(t.Context(), "mongodb", "mongo", "different-sha", "reason")
	assert.NoError(t, err)
	assert.Equal(t, 0, count)

	_, err = MarkMergeQueuePatchesRemovedFromQueue(t.Context(), "mongodb", "mongo", "", "reason")
	assert.Error(t, err)

	_, err = MarkMergeQueuePatchesRemovedFromQueue(t.Context(), "mongodb", "mongo", "abc123", "")
	assert.Error(t, err)
}
