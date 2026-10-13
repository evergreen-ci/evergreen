package evergreen

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestGetSectionsBSON(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	defer func(client *mongo.Client) {
		GetEnvironment().(*envState).sharedDBClient = client
	}(GetEnvironment().(*envState).sharedDBClient)
	GetEnvironment().(*envState).sharedDBClient = GetEnvironment().(*envState).client

	t.Run("IncludeOverrides", func(t *testing.T) {
		require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx))
		defer func() { require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx)) }()

		notifyConfig := NotifyConfig{BufferTargetPerInterval: 1}
		require.NoError(t, notifyConfig.Set(ctx))

		overrides := OverridesConfig{Overrides: []Override{
			{
				SectionID: notifyConfig.SectionId(),
				Field:     "buffer_target_per_interval",
				Value:     2,
			},
		}}
		require.NoError(t, overrides.Set(ctx))

		sections, err := getSectionsBSON(ctx, []string{notifyConfig.SectionId()}, true)
		assert.NoError(t, err)
		require.Len(t, sections, 1)
		assert.NoError(t, bson.Unmarshal(sections[0], &notifyConfig))
		assert.Equal(t, 2, notifyConfig.BufferTargetPerInterval)
	})
	t.Run("NoOverrides", func(t *testing.T) {
		require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx))
		defer func() { require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx)) }()

		notifyConfig := NotifyConfig{BufferTargetPerInterval: 1}
		require.NoError(t, notifyConfig.Set(ctx))

		overrides := OverridesConfig{Overrides: []Override{
			{
				SectionID: notifyConfig.SectionId(),
				Field:     "buffer_target_per_interval",
				Value:     2,
			},
		}}
		require.NoError(t, overrides.Set(ctx))

		sections, err := getSectionsBSON(ctx, []string{notifyConfig.SectionId()}, false)
		assert.NoError(t, err)
		require.Len(t, sections, 1)
		assert.NoError(t, bson.Unmarshal(sections[0], &notifyConfig))
		assert.Equal(t, 1, notifyConfig.BufferTargetPerInterval)
	})
}

func TestGetConfigSection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	defer func(client *mongo.Client) {
		GetEnvironment().(*envState).sharedDBClient = client
	}(GetEnvironment().(*envState).sharedDBClient)
	GetEnvironment().(*envState).sharedDBClient = GetEnvironment().(*envState).client

	t.Run("NoOverrides", func(t *testing.T) {
		require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx))
		defer func() { require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx)) }()

		config := NotifyConfig{BufferTargetPerInterval: 1}
		require.NoError(t, config.Set(ctx))

		overrides := OverridesConfig{Overrides: nil}
		require.NoError(t, overrides.Set(ctx))

		var fetchedConfig NotifyConfig
		assert.NoError(t, getConfigSection(ctx, &fetchedConfig))
		assert.Equal(t, config, fetchedConfig)
	})

	t.Run("WithOverrides", func(t *testing.T) {
		require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx))
		defer func() { require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx)) }()

		config := NotifyConfig{BufferTargetPerInterval: 1}
		require.NoError(t, config.Set(ctx))

		overrides := OverridesConfig{Overrides: []Override{
			{
				SectionID: config.SectionId(),
				Field:     "buffer_target_per_interval",
				Value:     2,
			},
		}}
		require.NoError(t, overrides.Set(ctx))

		var fetchedConfig NotifyConfig
		assert.NoError(t, getConfigSection(ctx, &fetchedConfig))
		assert.Equal(t, 2, fetchedConfig.BufferTargetPerInterval)
	})

	t.Run("DocumentMissing", func(t *testing.T) {
		require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx))

		fetchedConfig := NotifyConfig{BufferTargetPerInterval: 1}
		assert.NoError(t, getConfigSection(ctx, &fetchedConfig))
		assert.Zero(t, fetchedConfig)
	})
}

func TestSetConfigSection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Run("AlreadyExists", func(t *testing.T) {
		require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx))
		defer func() { require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx)) }()

		targetPerInterval := 1
		config := NotifyConfig{BufferTargetPerInterval: targetPerInterval}
		_, err := GetEnvironment().DB().Collection(ConfigCollection).InsertOne(ctx, struct {
			ID           string `bson:"_id"`
			NotifyConfig `bson:",inline"`
		}{ID: config.SectionId(), NotifyConfig: config})
		require.NoError(t, err)

		intervalSecs := 2
		assert.NoError(t, setConfigSection(ctx, config.SectionId(), bson.M{"$set": bson.M{"buffer_interval_seconds": intervalSecs}}))
		fetchedConfig := NotifyConfig{}
		assert.NoError(t, getConfigSection(ctx, &fetchedConfig))
		assert.Equal(t, targetPerInterval, fetchedConfig.BufferTargetPerInterval)
		assert.Equal(t, intervalSecs, fetchedConfig.BufferIntervalSeconds)
	})
	t.Run("New", func(t *testing.T) {
		require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx))
		defer func() { require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx)) }()

		intervalSecs := 1
		config := NotifyConfig{}
		assert.NoError(t, setConfigSection(ctx, config.SectionId(), bson.M{"$set": bson.M{"buffer_interval_seconds": intervalSecs}}))
		fetchedConfig := NotifyConfig{}
		assert.NoError(t, getConfigSection(ctx, &fetchedConfig))
		assert.Equal(t, intervalSecs, fetchedConfig.BufferIntervalSeconds)
	})
}

func TestOverrideConfig(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx))
	defer func() { require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx)) }()

	for testName, testCase := range map[string]struct {
		inputDoc  bson.M
		overrides OverridesConfig
		expected  bson.M
	}{
		"nested_levels": {
			inputDoc: bson.M{
				"_id": "testing",
				"level0": bson.M{
					"level1": "value",
				},
			},
			overrides: OverridesConfig{
				Overrides: []Override{
					{
						SectionID: "testing",
						Field:     "level0.level1",
						Value:     "new_value",
					},
				},
			},
			expected: bson.M{
				"_id": "testing",
				"level0": bson.M{
					"level1": "new_value",
				},
			},
		},
		"multiple_fields": {
			inputDoc: bson.M{
				"_id":    "testing",
				"field1": "original_value_1",
				"field2": "original_value_2",
			},
			overrides: OverridesConfig{
				Overrides: []Override{
					{
						SectionID: "testing",
						Field:     "field1",
						Value:     "new_value_1",
					},
					{
						SectionID: "testing",
						Field:     "field2",
						Value:     "new_value_2",
					},
				},
			},
			expected: bson.M{
				"_id":    "testing",
				"field1": "new_value_1",
				"field2": "new_value_2",
			},
		},
		"empty_overrides": {
			inputDoc: bson.M{
				"_id":   "testing",
				"field": "value",
			},
			overrides: OverridesConfig{},
			expected: bson.M{
				"_id":   "testing",
				"field": "value",
			},
		},
	} {
		t.Run(testName, func(t *testing.T) {
			require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx))
			require.NoError(t, testCase.overrides.Set(ctx))

			docRaw, err := bson.Marshal(testCase.inputDoc)
			require.NoError(t, err)

			newDocs, err := overrideConfig(ctx, []bson.Raw{docRaw}, true)
			assert.NoError(t, err)
			require.Len(t, newDocs, 1)
			var doc bson.M
			require.NoError(t, bson.Unmarshal(newDocs[0], &doc))
			assert.Equal(t, testCase.expected, doc)
		})
	}
}

func TestCloudProvidersGetWithFallback(t *testing.T) {
	ctx := t.Context()
	env, ok := GetEnvironment().(*envState)
	require.True(t, ok)
	originalSharedClient := env.sharedDBClient
	env.sharedDBClient = env.client
	collection := env.DB().Collection(ConfigCollection)
	filter := bson.M{"_id": bson.M{"$in": []string{"providers", overridesSectionID}}}
	cursor, err := collection.Find(ctx, filter)
	require.NoError(t, err)
	var originalDocs []bson.Raw
	require.NoError(t, cursor.All(ctx, &originalDocs))
	t.Cleanup(func() {
		defer func() { env.sharedDBClient = originalSharedClient }()
		cleanupCtx := context.WithoutCancel(t.Context())
		_, err := collection.DeleteMany(cleanupCtx, filter)
		assert.NoError(t, err)
		for _, doc := range originalDocs {
			_, err := collection.InsertOne(cleanupCtx, doc)
			assert.NoError(t, err)
		}
	})
	fallback := CloudProviders{AWS: AWSConfig{ResourceTags: ResourceTagsConfig{
		MongoDBOwner: "team@example.com", MongoDBEnv: "test", MongoDBOwnerEmailDomains: []string{"example.com"},
	}}}
	persisted := CloudProviders{AWS: AWSConfig{ResourceTags: ResourceTagsConfig{
		MongoDBOwner: "team@mongodb.com", MongoDBEnv: "prod", MongoDBOwnerEmailDomains: []string{"mongodb.com"},
	}}}
	for name, testCase := range map[string]struct {
		persisted *CloudProviders
		overrides []Override
		expected  CloudProviders
	}{
		"MissingProvidersShouldUseLoadedSettings":            {expected: fallback},
		"PersistedProvidersShouldReplaceLoadedSettings":      {persisted: &persisted, expected: persisted},
		"EmptyPersistedProvidersShouldReplaceLoadedSettings": {persisted: &CloudProviders{}, expected: CloudProviders{}},
		"PersistedProvidersShouldApplyOverrides": {
			persisted: &persisted,
			overrides: []Override{{SectionID: "providers", Field: "aws.resource_tags.mongodb_env", Value: "dev"}},
			expected: CloudProviders{AWS: AWSConfig{ResourceTags: ResourceTagsConfig{
				MongoDBOwner: "team@mongodb.com", MongoDBEnv: "dev", MongoDBOwnerEmailDomains: []string{"mongodb.com"},
			}}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := collection.DeleteMany(t.Context(), filter)
			require.NoError(t, err)
			if testCase.persisted != nil {
				require.NoError(t, testCase.persisted.Set(t.Context()))
			}
			if testCase.overrides != nil {
				overrides := OverridesConfig{Overrides: testCase.overrides}
				require.NoError(t, overrides.Set(t.Context()))
			}
			var providers CloudProviders
			require.NoError(t, providers.GetWithFallback(t.Context(), fallback))
			assert.Equal(t, testCase.expected, providers)
		})
	}
	t.Run("DatabaseErrorShouldBeReturned", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		var providers CloudProviders
		assert.Error(t, providers.GetWithFallback(ctx, fallback))
	})
}
