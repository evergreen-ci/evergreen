package patch

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGitHubIntentProcessingError(t *testing.T) {
	require.NoError(t, db.ClearCollections(GitHubIntentProcessingErrorCollection))

	stored, err := InsertGitHubIntentProcessingError(t.Context(), "project-id", "processing failed")
	require.NoError(t, err)
	require.NotNil(t, stored)
	assert.NotZero(t, stored.ID)

	found, err := FindGitHubIntentProcessingError(t.Context(), stored.ID)
	require.NoError(t, err)
	require.NotNil(t, found)
	assert.Equal(t, stored, found)

	notFound, err := FindGitHubIntentProcessingError(t.Context(), mgobson.NewObjectId())
	require.NoError(t, err)
	assert.Nil(t, notFound)

	_, err = InsertGitHubIntentProcessingError(t.Context(), "", "processing failed")
	assert.Error(t, err)
	_, err = InsertGitHubIntentProcessingError(t.Context(), "project-id", "")
	assert.Error(t, err)
}
