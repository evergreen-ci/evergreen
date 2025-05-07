package model

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVersionActivationWithIncompleteVersion(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	ctx := context.Background()

	// Clear the collections before running the test
	require.NoError(db.ClearCollections(VersionCollection))

	// Create a partially constructed version (simulating version creation in progress)
	v := &Version{
		Id:             "test_version",
		Identifier:     "project",
		CreateTime:     time.Now().Add(-time.Hour),
		Requester:      evergreen.RepotrackerVersionRequester,
		CreateComplete: false,
		BuildVariants: []VersionBuildStatus{
			{
				BuildVariant: "bv1",
				ActivationStatus: ActivationStatus{
					Activated:  false,
					ActivateAt: time.Now().Add(-time.Minute),
				},
			},
		},
	}
	require.NoError(v.Insert(ctx))

	// Try to activate the version
	activated, err := DoProjectActivation(ctx, "project", time.Now())
	assert.NoError(err)
	assert.False(activated, "version should not be activated when CreateComplete is false")

	// Complete version creation
	require.NoError(v.MarkVersionCreationComplete(ctx))

	// Verify the version was also activated when marked as complete
	v, err = VersionFindOne(ctx, VersionById(v.Id))
	assert.NoError(err)
	assert.True(utility.FromBoolPtr(v.Activated))

	// Try to activate again - should be skipped since it's already activated
	activated, err = DoProjectActivation(ctx, "project", time.Now())
	assert.NoError(err)
	assert.False(activated, "version should not be activated when already activated")
}
