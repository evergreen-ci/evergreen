package patch

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestGitHubIntentInfo(t *testing.T) {
	require.NoError(t, db.ClearCollections(GitHubIntentInfoCollection))

	stored, err := InsertGitHubIntentInfo(t.Context(), "project-id", "intent-id", "processing failed")
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.NotZero(t, stored.ID)
	assert.Equal(t, "intent-id", stored.IntentID)
	assert.NotZero(t, stored.CreatedAt)

	found, err := FindGitHubIntentInfo(t.Context(), stored.ID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, stored.ID, found.ID)
	assert.Equal(t, stored.ProjectID, found.ProjectID)
	assert.Equal(t, stored.IntentID, found.IntentID)
	assert.Equal(t, stored.Message, found.Message)
	assert.True(t, stored.CreatedAt.Equal(found.CreatedAt))

	notFound, err := FindGitHubIntentInfo(t.Context(), primitive.NewObjectID().Hex())
	require.NoError(t, err)
	assert.Nil(t, notFound)

	_, err = InsertGitHubIntentInfo(t.Context(), "", "intent-id", "processing failed")
	assert.Error(t, err)
	_, err = InsertGitHubIntentInfo(t.Context(), "project-id", "intent-id", "")
	assert.Error(t, err)
}
