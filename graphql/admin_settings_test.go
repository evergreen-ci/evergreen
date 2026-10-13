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
		MongoDBEnv:   utility.ToStringPtr(evergreen.MongoDBEnvironmentStaging),
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
	require.Equal(t, evergreen.MongoDBEnvironmentStaging, persistedSettings.Providers.AWS.ResourceTags.MongoDBEnv)
	require.Equal(t, "evergreen@mongodb.com", persistedSettings.Providers.AWS.ResourceTags.MongoDBOwner)
	require.Equal(t, []string{"mongodb.com", "10gen.com"}, persistedSettings.Providers.AWS.ResourceTags.MongoDBOwnerEmailDomains)
}

func TestSaveAdminSettingsRejectsClearingResourceTags(t *testing.T) {
	ctx := getContext(t)
	settings := testutil.MockConfig()
	settings.Providers.AWS.ResourceTags = evergreen.ResourceTagsConfig{
		MongoDBEnv:   evergreen.MongoDBEnvironmentStaging,
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

func TestSaveAdminSettingsOwnerEmailDomains(t *testing.T) {
	ctx := getContext(t)
	settings := testutil.MockConfig()
	settings.Providers.AWS.ResourceTags = evergreen.ResourceTagsConfig{MongoDBOwner: "team@mongodb.com"}
	require.NoError(t, evergreen.UpdateConfig(ctx, settings))
	resolver := &mutationResolver{&Resolver{}}
	saveResourceTagSettings := func(resourceTagSettings *restModel.APIResourceTagsConfig) (*restModel.APIAdminSettings, error) {
		return resolver.SaveAdminSettings(ctx, restModel.APIAdminSettings{
			Providers: &restModel.APICloudProviders{AWS: &restModel.APIAWSConfig{ResourceTags: resourceTagSettings}},
		})
	}
	updatedSettings, err := saveResourceTagSettings(&restModel.APIResourceTagsConfig{
		MongoDBOwner:             utility.ToStringPtr("team@example.com"),
		MongoDBOwnerEmailDomains: []string{"example.com"},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"example.com"}, updatedSettings.Providers.AWS.ResourceTags.MongoDBOwnerEmailDomains)
	_, err = saveResourceTagSettings(&restModel.APIResourceTagsConfig{MongoDBEnv: utility.ToStringPtr("dev")})
	require.NoError(t, err)
	_, err = resolver.SaveAdminSettings(ctx, restModel.APIAdminSettings{Banner: utility.ToStringPtr("Updated banner")})
	require.NoError(t, err)
	persistedSettings, err := evergreen.GetConfig(ctx)
	require.NoError(t, err)
	require.Equal(t, []string{"example.com"}, persistedSettings.Providers.AWS.ResourceTags.MongoDBOwnerEmailDomains)
	for name, domains := range map[string][]string{
		"EmptyDomains":       {},
		"MalformedDomains":   {"@example.com"},
		"RemovedOwnerDomain": {"mongodb.com"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := saveResourceTagSettings(&restModel.APIResourceTagsConfig{MongoDBOwnerEmailDomains: domains})
			require.Error(t, err)
			persistedSettings, err := evergreen.GetConfig(ctx)
			require.NoError(t, err)
			require.Equal(t, []string{"example.com"}, persistedSettings.Providers.AWS.ResourceTags.MongoDBOwnerEmailDomains)
			require.Equal(t, "team@example.com", persistedSettings.Providers.AWS.ResourceTags.MongoDBOwner)
		})
	}
}
