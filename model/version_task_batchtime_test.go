package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionTaskBatchTimesUpsertAndFindOne(t *testing.T) {
	require.NoError(t, db.ClearCollections(VersionTaskBatchTimeCollection))
	t.Cleanup(func() {
		require.NoError(t, db.ClearCollections(VersionTaskBatchTimeCollection))
	})

	bt := &VersionTaskBatchTimes{
		Id:         "version0",
		Identifier: "proj",
		Requester:  evergreen.RepotrackerVersionRequester,
		Order:      5,
		BuildVariants: []BuildVariantTaskBatchTime{
			{
				BuildVariant: "ubuntu",
				BatchTimeTasks: []BatchTimeTaskStatus{
					{TaskName: "task0", TaskId: "task_id0"},
				},
			},
		},
	}
	require.NoError(t, VersionTaskBatchTimesUpsert(t.Context(), bt))

	found, err := VersionTaskBatchTimesFindOne(t.Context(), bt.Id)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, bt.Identifier, found.Identifier)
	assert.Equal(t, evergreen.RepotrackerVersionRequester, found.Requester)
	assert.Equal(t, bt.Order, found.Order)
	require.Len(t, found.BuildVariants, 1)
	assert.Equal(t, bt.BuildVariants[0].BuildVariant, found.BuildVariants[0].BuildVariant)
	require.Len(t, found.BuildVariants[0].BatchTimeTasks, 1)
	assert.Equal(t, bt.BuildVariants[0].BatchTimeTasks[0].TaskName, found.BuildVariants[0].BatchTimeTasks[0].TaskName)

	t.Run("FindOneFailsWithNonexistent", func(t *testing.T) {
		notFound, err := VersionTaskBatchTimesFindOne(t.Context(), "nonexistent")
		require.NoError(t, err)
		assert.Nil(t, notFound)
	})
}

func TestVersionTaskBatchTimesUpsertReplacesBuildVariants(t *testing.T) {
	require.NoError(t, db.ClearCollections(VersionTaskBatchTimeCollection))
	t.Cleanup(func() {
		require.NoError(t, db.ClearCollections(VersionTaskBatchTimeCollection))
	})

	require.NoError(t, VersionTaskBatchTimesUpsert(t.Context(), &VersionTaskBatchTimes{
		Id:    "version0",
		Order: 1,
		BuildVariants: []BuildVariantTaskBatchTime{
			{BuildVariant: "ubuntu", BatchTimeTasks: []BatchTimeTaskStatus{{TaskName: "task0"}}},
		},
	}))
	require.NoError(t, VersionTaskBatchTimesUpsert(t.Context(), &VersionTaskBatchTimes{
		Id:    "version0",
		Order: 1,
		BuildVariants: []BuildVariantTaskBatchTime{
			{BuildVariant: "windows", BatchTimeTasks: []BatchTimeTaskStatus{{TaskName: "task1"}}},
		},
	}))

	found, err := VersionTaskBatchTimesFindOne(t.Context(), "version0")
	require.NoError(t, err)
	require.NotNil(t, found)
	require.Len(t, found.BuildVariants, 1)
	assert.Equal(t, "windows", found.BuildVariants[0].BuildVariant)
}

func TestVersionTaskBatchTimesFindByLastTaskActivation(t *testing.T) {
	require.NoError(t, db.ClearCollections(VersionTaskBatchTimeCollection))
	t.Cleanup(func() {
		require.NoError(t, db.ClearCollections(VersionTaskBatchTimeCollection))
	})

	activated := []BuildVariantTaskBatchTime{
		{
			BuildVariant: "ubuntu",
			BatchTimeTasks: []BatchTimeTaskStatus{
				{
					TaskName:         "task0",
					ActivationStatus: ActivationStatus{Activated: true},
				},
			},
		},
	}
	bts := []*VersionTaskBatchTimes{
		{
			Id:            "version0",
			Identifier:    "proj",
			Requester:     evergreen.RepotrackerVersionRequester,
			Order:         1,
			BuildVariants: activated,
		},
		{
			Id:            "version1",
			Identifier:    "proj",
			Requester:     evergreen.RepotrackerVersionRequester,
			Order:         2,
			BuildVariants: activated,
		},
		{
			// Patch requesters aren't considered.
			Id:            "version2",
			Identifier:    "proj",
			Requester:     evergreen.PatchVersionRequester,
			Order:         3,
			BuildVariants: activated,
		},
		{
			// Ignored versions aren't considered.
			Id:            "version3",
			Identifier:    "proj",
			Requester:     evergreen.RepotrackerVersionRequester,
			Order:         4,
			Ignored:       true,
			BuildVariants: activated,
		},
		{
			// Task isn't activated.
			Id:         "version4",
			Identifier: "proj",
			Requester:  evergreen.RepotrackerVersionRequester,
			Order:      5,
			BuildVariants: []BuildVariantTaskBatchTime{
				{
					BuildVariant: "ubuntu",
					BatchTimeTasks: []BatchTimeTaskStatus{
						{TaskName: "task0"},
					},
				},
			},
		},
	}
	for _, bt := range bts {
		require.NoError(t, VersionTaskBatchTimesUpsert(t.Context(), bt))
	}

	found, err := VersionTaskBatchTimesFindByLastTaskActivation(t.Context(), "proj", "ubuntu", "task0")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "version1", found.Id)

	notFound, err := VersionTaskBatchTimesFindByLastTaskActivation(t.Context(), "proj", "ubuntu", "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, notFound)
}

func TestGetBuildVariantsIncludesBatchTimeTasks(t *testing.T) {
	require.NoError(t, db.ClearCollections(VersionCollection, VersionTaskBatchTimeCollection))
	t.Cleanup(func() {
		require.NoError(t, db.ClearCollections(VersionCollection, VersionTaskBatchTimeCollection))
	})

	require.NoError(t, db.Insert(t.Context(), VersionCollection, Version{
		Id:         "version0",
		Identifier: "proj",
		Requester:  evergreen.RepotrackerVersionRequester,
		BuildVariants: []VersionBuildStatus{
			{BuildVariant: "ubuntu"},
			{BuildVariant: "windows"},
		},
	}))
	require.NoError(t, VersionTaskBatchTimesUpsert(t.Context(), &VersionTaskBatchTimes{
		Id:         "version0",
		Identifier: "proj",
		Requester:  evergreen.RepotrackerVersionRequester,
		BuildVariants: []BuildVariantTaskBatchTime{
			{BuildVariant: "ubuntu", BatchTimeTasks: []BatchTimeTaskStatus{{TaskName: "task0"}}},
		},
	}))

	v, err := VersionFindOneId(t.Context(), "version0")
	require.NoError(t, err)
	require.NotNil(t, v)
	bvs, err := v.GetBuildVariants(t.Context())
	require.NoError(t, err)
	require.Len(t, bvs, 2)
	require.Len(t, bvs[0].BatchTimeTasks, 1, "should populate batchtime tasks for build variant from separate collection")
	assert.Equal(t, "task0", bvs[0].BatchTimeTasks[0].TaskName)
	assert.Empty(t, bvs[1].BatchTimeTasks)
}

func TestGetBuildVariantsIncludesLegacyBatchTimeTasks(t *testing.T) {
	require.NoError(t, db.ClearCollections(VersionCollection, VersionTaskBatchTimeCollection))
	t.Cleanup(func() {
		require.NoError(t, db.ClearCollections(VersionCollection, VersionTaskBatchTimeCollection))
	})

	// Legacy versions still store batchtime tasks in the version doc.
	require.NoError(t, db.Insert(t.Context(), VersionCollection, Version{
		Id: "version0",
		BuildVariants: []VersionBuildStatus{
			{BuildVariant: "ubuntu", BatchTimeTasks: []BatchTimeTaskStatus{{TaskName: "task0"}}},
		},
	}))

	v, err := VersionFindOneId(t.Context(), "version0")
	require.NoError(t, err)
	require.NotNil(t, v)
	bvs, err := v.GetBuildVariants(t.Context())
	require.NoError(t, err)
	require.Len(t, bvs, 1)
	require.Len(t, bvs[0].BatchTimeTasks, 1)
	assert.Equal(t, "task0", bvs[0].BatchTimeTasks[0].TaskName)
}

func TestGetBuildVariantsMergesSplitBatchTimeTasks(t *testing.T) {
	require.NoError(t, db.ClearCollections(VersionCollection, VersionTaskBatchTimeCollection))
	t.Cleanup(func() {
		require.NoError(t, db.ClearCollections(VersionCollection, VersionTaskBatchTimeCollection))
	})

	// A version whose batchtime tasks are split across the version doc (a
	// pre-existing build variant) and the separate collection (a build variant
	// added later, e.g. by generate.tasks).
	require.NoError(t, db.Insert(t.Context(), VersionCollection, Version{
		Id: "version0",
		BuildVariants: []VersionBuildStatus{
			{BuildVariant: "ubuntu", BatchTimeTasks: []BatchTimeTaskStatus{{TaskName: "task0"}}},
			{BuildVariant: "windows"},
		},
	}))
	require.NoError(t, VersionTaskBatchTimesUpsert(t.Context(), &VersionTaskBatchTimes{
		Id: "version0",
		BuildVariants: []BuildVariantTaskBatchTime{
			{BuildVariant: "windows", BatchTimeTasks: []BatchTimeTaskStatus{{TaskName: "task1"}}},
		},
	}))

	v, err := VersionFindOneId(t.Context(), "version0")
	require.NoError(t, err)
	require.NotNil(t, v)
	bvs, err := v.GetBuildVariants(t.Context())
	require.NoError(t, err)
	require.Len(t, bvs, 2)
	require.Len(t, bvs[0].BatchTimeTasks, 1, "should keep the batchtime tasks stored in the version doc")
	assert.Equal(t, "task0", bvs[0].BatchTimeTasks[0].TaskName)
	require.Len(t, bvs[1].BatchTimeTasks, 1, "should attach the batchtime tasks stored in the separate collection")
	assert.Equal(t, "task1", bvs[1].BatchTimeTasks[0].TaskName)
}

func TestVersionTaskBatchTimesAppend(t *testing.T) {
	require.NoError(t, db.ClearCollections(VersionTaskBatchTimeCollection))
	t.Cleanup(func() {
		require.NoError(t, db.ClearCollections(VersionTaskBatchTimeCollection))
	})

	require.NoError(t, VersionTaskBatchTimesAppend(t.Context(), &VersionTaskBatchTimes{
		Id:         "version0",
		Identifier: "proj",
		Requester:  evergreen.RepotrackerVersionRequester,
		Order:      1,
		BuildVariants: []BuildVariantTaskBatchTime{
			{
				BuildVariant:   "ubuntu",
				BatchTimeTasks: []BatchTimeTaskStatus{{TaskName: "task0"}},
			},
		},
	}))
	require.NoError(t, VersionTaskBatchTimesAppend(t.Context(), &VersionTaskBatchTimes{
		Id: "version0",
		BuildVariants: []BuildVariantTaskBatchTime{
			{
				BuildVariant:   "windows",
				BatchTimeTasks: []BatchTimeTaskStatus{{TaskName: "task1"}},
			},
		},
	}))

	found, err := VersionTaskBatchTimesFindOne(t.Context(), "version0")
	require.NoError(t, err)
	require.NotNil(t, found)
	require.Len(t, found.BuildVariants, 2)
	assert.Equal(t, "ubuntu", found.BuildVariants[0].BuildVariant)
	assert.Equal(t, "windows", found.BuildVariants[1].BuildVariant)
	assert.Equal(t, "proj", found.Identifier, "should set the denormalized fields when creating the doc")
}

func TestActivateAndSetBuildVariantsStoresBatchTimeTasksInCollection(t *testing.T) {
	require.NoError(t, db.ClearCollections(VersionCollection, VersionTaskBatchTimeCollection))
	t.Cleanup(func() {
		require.NoError(t, db.ClearCollections(VersionCollection, VersionTaskBatchTimeCollection))
	})

	require.NoError(t, db.Insert(t.Context(), VersionCollection, Version{
		Id:         "version0",
		Identifier: "proj",
		Requester:  evergreen.RepotrackerVersionRequester,
		BuildVariants: []VersionBuildStatus{
			{
				BuildVariant:   "ubuntu",
				BatchTimeTasks: []BatchTimeTaskStatus{{TaskName: "task0"}},
			},
		},
	}))

	v, err := VersionFindOneId(t.Context(), "version0")
	require.NoError(t, err)
	require.NotNil(t, v)
	require.NoError(t, v.ActivateAndSetBuildVariants(t.Context()))

	stored, err := VersionFindOneIdWithBuildVariants(t.Context(), "version0")
	require.NoError(t, err)
	require.NotNil(t, stored)
	require.Len(t, stored.BuildVariants, 1)
	assert.Empty(t, stored.BuildVariants[0].BatchTimeTasks, "version document should not store batchtime tasks")

	bt, err := VersionTaskBatchTimesFindOne(t.Context(), "version0")
	require.NoError(t, err)
	require.NotNil(t, bt)
	require.Len(t, bt.BuildVariants, 1)
	require.Len(t, bt.BuildVariants[0].BatchTimeTasks, 1)
	assert.Equal(t, "task0", bt.BuildVariants[0].BatchTimeTasks[0].TaskName)

	bvs, err := v.GetBuildVariants(t.Context())
	require.NoError(t, err)
	require.Len(t, bvs, 1)
	require.Len(t, bvs[0].BatchTimeTasks, 1)
	assert.Equal(t, "task0", bvs[0].BatchTimeTasks[0].TaskName)
}
