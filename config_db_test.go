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

		config := NotifyConfig{BufferTargetPerInterval: 1}
		_, err := GetEnvironment().DB().Collection(ConfigCollection).InsertOne(ctx, struct {
			ID           string `bson:"_id"`
			NotifyConfig `bson:",inline"`
		}{ID: config.SectionId(), NotifyConfig: config})
		require.NoError(t, err)

		sections, err := getSectionsBSON(ctx, []string{config.SectionId()}, true)
		assert.NoError(t, err)
		assert.Len(t, sections, 1)
	})
	t.Run("OnlyShared", func(t *testing.T) {
		require.NoError(t, GetEnvironment().SharedDB().Collection(ConfigCollection).Drop(ctx))
		defer func() { require.NoError(t, GetEnvironment().SharedDB().Collection(ConfigCollection).Drop(ctx)) }()

		config := NotifyConfig{BufferTargetPerInterval: 1}
		_, err := GetEnvironment().SharedDB().Collection(ConfigCollection).InsertOne(ctx, struct {
			ID           string `bson:"_id"`
			NotifyConfig `bson:",inline"`
		}{ID: config.SectionId(), NotifyConfig: config})
		require.NoError(t, err)

		sections, err := getSectionsBSON(ctx, []string{config.SectionId()}, false)
		assert.NoError(t, err)
		assert.Len(t, sections, 1)
	})
}

func TestGetConfigSection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	t.Run("NilSharedDB", func(t *testing.T) {
		t.Run("ConfigDocumentExists", func(t *testing.T) {
			require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx))
			defer func() { require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx)) }()

			config := NotifyConfig{BufferTargetPerInterval: 1}
			_, err := GetEnvironment().DB().Collection(ConfigCollection).InsertOne(ctx, struct {
				ID           string `bson:"_id"`
				NotifyConfig `bson:",inline"`
			}{ID: config.SectionId(), NotifyConfig: config})
			require.NoError(t, err)

			var fetchedConfig NotifyConfig
			assert.NoError(t, getConfigSection(ctx, &fetchedConfig))
			assert.Equal(t, config, fetchedConfig)

		})

		t.Run("DocumentMissing", func(t *testing.T) {
			require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx))

			fetchedConfig := NotifyConfig{BufferTargetPerInterval: 1}
			assert.NoError(t, getConfigSection(ctx, &fetchedConfig))
			assert.Zero(t, fetchedConfig)
		})
	})

	t.Run("SharedDBSet", func(t *testing.T) {
		defer func(client *mongo.Client) {
			GetEnvironment().(*envState).sharedDBClient = client
		}(GetEnvironment().(*envState).sharedDBClient)
		GetEnvironment().(*envState).sharedDBClient = GetEnvironment().(*envState).client

		t.Run("ConfigDocumentExists", func(t *testing.T) {
			require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx))
			defer func() { require.NoError(t, GetEnvironment().DB().Collection(ConfigCollection).Drop(ctx)) }()

			config := NotifyConfig{BufferTargetPerInterval: 1}
			_, err := GetEnvironment().DB().Collection(ConfigCollection).InsertOne(ctx, struct {
				ID           string `bson:"_id"`
				NotifyConfig `bson:",inline"`
			}{ID: config.SectionId(), NotifyConfig: config})
			require.NoError(t, err)

			var fetchedConfig NotifyConfig
			assert.NoError(t, getConfigSection(ctx, &fetchedConfig))
			assert.Equal(t, config, fetchedConfig)

		})

		t.Run("DocumentMissing", func(t *testing.T) {
			require.NoError(t, GetEnvironment().SharedDB().Collection(ConfigCollection).Drop(ctx))

			fetchedConfig := NotifyConfig{BufferTargetPerInterval: 1}
			assert.NoError(t, getConfigSection(ctx, &fetchedConfig))
			assert.Zero(t, fetchedConfig)
		})
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
