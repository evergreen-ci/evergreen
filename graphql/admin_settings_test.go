package graphql

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/require"
)

func TestSaveAdminSettingsPersistsResourceTags(t *testing.T) {
	ctx := getContext(t)
	require.NoError(t, evergreen.UpdateConfig(ctx, testutil.MockConfig()))

	resourceTags := &restModel.APIResourceTagsConfig{
		MongoDBEnv:   utility.ToStringPtr("staging"),
		MongoDBOwner: utility.ToStringPtr("evergreen@mongodb.com"),
	}
	resolver := &mutationResolver{&Resolver{}}
	updatedSettings, err := resolver.SaveAdminSettings(ctx, restModel.APIAdminSettings{
		ResourceTags: resourceTags,
	})
	require.NoError(t, err)
	require.NotNil(t, updatedSettings.ResourceTags)
	require.Equal(t, resourceTags.MongoDBEnv, updatedSettings.ResourceTags.MongoDBEnv)
	require.Equal(t, resourceTags.MongoDBOwner, updatedSettings.ResourceTags.MongoDBOwner)

	persistedSettings, err := evergreen.GetConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, "staging", persistedSettings.ResourceTags.MongoDBEnv)
	require.Equal(t, "evergreen@mongodb.com", persistedSettings.ResourceTags.MongoDBOwner)
}
