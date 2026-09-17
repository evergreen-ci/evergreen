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
		Providers: &restModel.APICloudProviders{
			AWS: &restModel.APIAWSConfig{ResourceTags: resourceTags},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, updatedSettings.Providers.AWS.ResourceTags)
	require.Equal(t, resourceTags.MongoDBEnv, updatedSettings.Providers.AWS.ResourceTags.MongoDBEnv)
	require.Equal(t, resourceTags.MongoDBOwner, updatedSettings.Providers.AWS.ResourceTags.MongoDBOwner)

	persistedSettings, err := evergreen.GetConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, "staging", persistedSettings.Providers.AWS.ResourceTags.MongoDBEnv)
	require.Equal(t, "evergreen@mongodb.com", persistedSettings.Providers.AWS.ResourceTags.MongoDBOwner)
}

func TestSaveAdminSettingsRejectsClearingResourceTags(t *testing.T) {
	ctx := getContext(t)
	settings := testutil.MockConfig()
	settings.Providers.AWS.ResourceTags = evergreen.ResourceTagsConfig{
		MongoDBEnv:   "staging",
		MongoDBOwner: "evergreen@mongodb.com",
	}
	require.NoError(t, evergreen.UpdateConfig(ctx, settings))

	resolver := &mutationResolver{&Resolver{}}
	for name, resourceTags := range map[string]*restModel.APIResourceTagsConfig{
		"Environment": {MongoDBEnv: utility.ToStringPtr("")},
		"Owner":       {MongoDBOwner: utility.ToStringPtr("")},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := resolver.SaveAdminSettings(ctx, restModel.APIAdminSettings{
				Providers: &restModel.APICloudProviders{
					AWS: &restModel.APIAWSConfig{ResourceTags: resourceTags},
				},
			})
			require.Error(t, err)
		})
	}
}
