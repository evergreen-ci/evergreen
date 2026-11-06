package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionBatchTimeTasksSetAndFindOne(t *testing.T) {
	require.NoError(t, db.ClearCollections(VersionBatchTimeTasksCollection))
	t.Cleanup(func() {
		require.NoError(t, db.ClearCollections(VersionBatchTimeTasksCollection))
	})

	doc := &VersionBatchTimeTasks{
		Id:         "version0",
		Identifier: "proj",
		Requester:  evergreen.RepotrackerVersionRequester,
		Order:      5,
		Variants: []VersionVariantBatchTimeTasks{
			{
				BuildVariant: "ubuntu",
				BatchTimeTasks: []BatchTimeTaskStatus{
					{TaskName: "task0", TaskId: "task_id0"},
				},
			},
		},
	}
	require.NoError(t, VersionBatchTimeTasksSet(t.Context(), doc))

	found, err := VersionBatchTimeTasksFindOne(t.Context(), "version0")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "proj", found.Identifier)
	assert.Equal(t, evergreen.RepotrackerVersionRequester, found.Requester)
	assert.Equal(t, 5, found.Order)
	require.Len(t, found.Variants, 1)
	assert.Equal(t, "ubuntu", found.Variants[0].BuildVariant)
	require.Len(t, found.Variants[0].BatchTimeTasks, 1)
	assert.Equal(t, "task0", found.Variants[0].BatchTimeTasks[0].TaskName)

	notFound, err := VersionBatchTimeTasksFindOne(t.Context(), "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, notFound)
}

func TestVersionBatchTimeTasksSetReplacesVariants(t *testing.T) {
	require.NoError(t, db.ClearCollections(VersionBatchTimeTasksCollection))
	t.Cleanup(func() {
		require.NoError(t, db.ClearCollections(VersionBatchTimeTasksCollection))
	})

	require.NoError(t, VersionBatchTimeTasksSet(t.Context(), &VersionBatchTimeTasks{
		Id:    "version0",
		Order: 1,
		Variants: []VersionVariantBatchTimeTasks{
			{BuildVariant: "ubuntu", BatchTimeTasks: []BatchTimeTaskStatus{{TaskName: "task0"}}},
		},
	}))
	require.NoError(t, VersionBatchTimeTasksSet(t.Context(), &VersionBatchTimeTasks{
		Id:    "version0",
		Order: 1,
		Variants: []VersionVariantBatchTimeTasks{
			{BuildVariant: "windows", BatchTimeTasks: []BatchTimeTaskStatus{{TaskName: "task1"}}},
		},
	}))

	found, err := VersionBatchTimeTasksFindOne(t.Context(), "version0")
	require.NoError(t, err)
	require.NotNil(t, found)
	require.Len(t, found.Variants, 1)
	assert.Equal(t, "windows", found.Variants[0].BuildVariant)
}

func TestVersionBatchTimeTasksPushVariants(t *testing.T) {
	require.NoError(t, db.ClearCollections(VersionBatchTimeTasksCollection))
	t.Cleanup(func() {
		require.NoError(t, db.ClearCollections(VersionBatchTimeTasksCollection))
	})

	require.NoError(t, VersionBatchTimeTasksPushVariants(t.Context(), &VersionBatchTimeTasks{
		Id:         "version0",
		Identifier: "proj",
		Requester:  evergreen.RepotrackerVersionRequester,
		Order:      1,
		Variants: []VersionVariantBatchTimeTasks{
			{BuildVariant: "ubuntu", BatchTimeTasks: []BatchTimeTaskStatus{{TaskName: "task0"}}},
		},
	}))
	require.NoError(t, VersionBatchTimeTasksPushVariants(t.Context(), &VersionBatchTimeTasks{
		Id: "version0",
		Variants: []VersionVariantBatchTimeTasks{
			{BuildVariant: "windows", BatchTimeTasks: []BatchTimeTaskStatus{{TaskName: "task1"}}},
		},
	}))

	found, err := VersionBatchTimeTasksFindOne(t.Context(), "version0")
	require.NoError(t, err)
	require.NotNil(t, found)
	require.Len(t, found.Variants, 2)
	assert.Equal(t, "ubuntu", found.Variants[0].BuildVariant)
	assert.Equal(t, "windows", found.Variants[1].BuildVariant)
	assert.Equal(t, "proj", found.Identifier)
}

func TestVersionBatchTimeTasksFindByLastTaskActivation(t *testing.T) {
	require.NoError(t, db.ClearCollections(VersionBatchTimeTasksCollection))
	t.Cleanup(func() {
		require.NoError(t, db.ClearCollections(VersionBatchTimeTasksCollection))
	})

	activated := []VersionVariantBatchTimeTasks{
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
	docs := []*VersionBatchTimeTasks{
		{
			Id:         "version0",
			Identifier: "proj",
			Requester:  evergreen.RepotrackerVersionRequester,
			Order:      1,
			Variants:   activated,
		},
		{
			Id:         "version1",
			Identifier: "proj",
			Requester:  evergreen.RepotrackerVersionRequester,
			Order:      2,
			Variants:   activated,
		},
		{
			// Patch requesters aren't considered.
			Id:         "version2",
			Identifier: "proj",
			Requester:  evergreen.PatchVersionRequester,
			Order:      3,
			Variants:   activated,
		},
		{
			// Ignored versions aren't considered.
			Id:         "version3",
			Identifier: "proj",
			Requester:  evergreen.RepotrackerVersionRequester,
			Order:      4,
			Ignored:    true,
			Variants:   activated,
		},
		{
			// Task isn't activated.
			Id:         "version4",
			Identifier: "proj",
			Requester:  evergreen.RepotrackerVersionRequester,
			Order:      5,
			Variants: []VersionVariantBatchTimeTasks{
				{
					BuildVariant: "ubuntu",
					BatchTimeTasks: []BatchTimeTaskStatus{
						{TaskName: "task0"},
					},
				},
			},
		},
	}
	for _, doc := range docs {
		require.NoError(t, VersionBatchTimeTasksSet(t.Context(), doc))
	}

	found, err := VersionBatchTimeTasksFindByLastTaskActivation(t.Context(), "proj", "ubuntu", "task0")
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, "version1", found.Id)

	notFound, err := VersionBatchTimeTasksFindByLastTaskActivation(t.Context(), "proj", "ubuntu", "nonexistent")
	require.NoError(t, err)
	assert.Nil(t, notFound)
}
