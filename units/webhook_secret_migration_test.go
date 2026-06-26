package units

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/parameterstore/fakeparameter"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/mongodb/amboy/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func setupWebhookMigrationTest(t *testing.T) *mock.Environment {
	env := &mock.Environment{}
	require.NoError(t, env.Configure(t.Context()))
	env.EvergreenSettings.ServiceFlags.WebhookSecretMigrationEnabled = true
	env.EvergreenSettings.ServiceFlags.WebhookSecretCleanupEnabled = true
	require.NoError(t, db.ClearCollections(event.SubscriptionsCollection, fakeparameter.Collection))
	require.NoError(t, evergreen.SetServiceFlags(t.Context(), evergreen.ServiceFlags{
		WebhookSecretMigrationEnabled: true,
		WebhookSecretCleanupEnabled:   true,
	}))
	t.Cleanup(func() {
		require.NoError(t, db.ClearCollections(event.SubscriptionsCollection, fakeparameter.Collection))
	})
	return env
}

// insertUnmigratedWebhookSubscription inserts a subscription document directly
// into MongoDB with a plaintext secret and no secret_parameter, simulating
// a pre-migration state.
func insertUnmigratedWebhookSubscription(t *testing.T, id string, secret string) {
	t.Helper()
	sub := event.Subscription{
		ID:           id,
		ResourceType: event.ResourceTypePatch,
		Trigger:      event.TriggerOutcome,
		Selectors:    []event.Selector{{Type: event.SelectorID, Data: "test"}},
		Subscriber: event.Subscriber{
			Type: event.EvergreenWebhookSubscriberType,
			Target: &event.WebhookSubscriber{
				URL:    "https://example.com/webhook",
				Secret: []byte(secret),
			},
		},
		Owner:     "test-owner",
		OwnerType: event.OwnerTypePerson,
	}
	require.NoError(t, db.Insert(t.Context(), event.SubscriptionsCollection, sub))
}

// insertWebhookSubscriptionWithAuthHeader inserts a subscription with an
// Authorization header but no authorization_header_parameter, simulating a pre-migration
// state for the auth header.
func insertWebhookSubscriptionWithAuthHeader(t *testing.T, id, authValue string) {
	t.Helper()
	sub := event.Subscription{
		ID:           id,
		ResourceType: event.ResourceTypePatch,
		Trigger:      event.TriggerOutcome,
		Selectors:    []event.Selector{{Type: event.SelectorID, Data: "test"}},
		Subscriber: event.Subscriber{
			Type: event.EvergreenWebhookSubscriberType,
			Target: &event.WebhookSubscriber{
				URL:    "https://example.com/webhook",
				Secret: []byte("the-secret"),
				Headers: []event.WebhookHeader{
					{Key: event.WebhookAuthorizationHeader, Value: authValue},
				},
			},
		},
		Owner:     "test-owner",
		OwnerType: event.OwnerTypePerson,
	}
	require.NoError(t, db.Insert(t.Context(), event.SubscriptionsCollection, sub))
}

// insertWebhookSubscriptionWithBothUnmigrated inserts a subscription with both
// a secret and an Authorization header, neither migrated to Parameter Store.
func insertWebhookSubscriptionWithBothUnmigrated(t *testing.T, id, secret, authValue string) {
	t.Helper()
	sub := event.Subscription{
		ID:           id,
		ResourceType: event.ResourceTypePatch,
		Trigger:      event.TriggerOutcome,
		Selectors:    []event.Selector{{Type: event.SelectorID, Data: "test"}},
		Subscriber: event.Subscriber{
			Type: event.EvergreenWebhookSubscriberType,
			Target: &event.WebhookSubscriber{
				URL:    "https://example.com/webhook",
				Secret: []byte(secret),
				Headers: []event.WebhookHeader{
					{Key: event.WebhookAuthorizationHeader, Value: authValue},
				},
			},
		},
		Owner:     "test-owner",
		OwnerType: event.OwnerTypePerson,
	}
	require.NoError(t, db.Insert(t.Context(), event.SubscriptionsCollection, sub))
}

func TestWebhookSecretMigrationJobFactory(t *testing.T) {
	factory, err := registry.GetJobFactory(webhookSecretMigrationJobName)
	require.NoError(t, err)
	require.NotNil(t, factory)
	j := factory()
	require.NotNil(t, j)
	assert.Equal(t, webhookSecretMigrationJobName, j.Type().Name)
}

func TestWebhookSecretMigrationJobConstructor(t *testing.T) {
	j := NewWebhookSecretMigrationJob("sub-123", "2025-01-01")
	require.NotNil(t, j)
	assert.Equal(t, "webhook-secret-migration.sub-123.2025-01-01", j.ID())
	assert.Equal(t, webhookSecretMigrationJobName, j.Type().Name)
}

func TestWebhookSecretMigrationJobRun(t *testing.T) {
	t.Run("MigratesSecretToParameterStore", func(t *testing.T) {
		env := setupWebhookMigrationTest(t)
		insertUnmigratedWebhookSubscription(t, "sub-migrate", "my-secret")

		j := makeWebhookSecretMigrationJob()
		j.env = env
		j.SubscriptionID = "sub-migrate"
		j.SetID("test-migrate")

		j.Run(t.Context())
		require.True(t, j.Status().Completed)
		require.False(t, j.HasErrors())

		// Verify the DB document was updated.
		raw := bson.M{}
		require.NoError(t, db.FindOneQ(t.Context(), event.SubscriptionsCollection, db.Query(bson.M{"_id": "sub-migrate"}), &raw))
		subscriberRaw, ok := raw["subscriber"].(bson.M)
		require.True(t, ok)
		targetRaw, ok := subscriberRaw["target"].(bson.M)
		require.True(t, ok)
		assert.NotNil(t, targetRaw["secret"], "secret should be kept in MongoDB for phase 2 cleanup")
		assert.NotEmpty(t, targetRaw["secret_parameter"], "secret_parameter should be set")

		// Verify the secret is in the fake Parameter Store.
		paramName, ok := targetRaw["secret_parameter"].(string)
		require.True(t, ok)
		require.NotEmpty(t, paramName)
		fakeParams, err := fakeparameter.FindByIDs(t.Context(), paramName)
		require.NoError(t, err)
		require.Len(t, fakeParams, 1)
		assert.Equal(t, "my-secret", fakeParams[0].Value)
	})

	t.Run("SkipsAlreadyMigratedSubscription", func(t *testing.T) {
		env := setupWebhookMigrationTest(t)

		// Insert a subscription that already has secret_parameter set.
		doc := bson.M{
			"_id":           "sub-already-migrated",
			"resource_type": "PATCH",
			"trigger":       "outcome",
			"selectors":     bson.A{bson.M{"type": "id", "data": "test"}},
			"subscriber": bson.M{
				"type": event.EvergreenWebhookSubscriberType,
				"target": bson.M{
					"url":              "https://example.com/webhook",
					"secret_parameter": "/some/existing/param",
				},
			},
			"owner":      "test-owner",
			"owner_type": "person",
		}
		require.NoError(t, db.Insert(t.Context(), event.SubscriptionsCollection, doc))

		j := makeWebhookSecretMigrationJob()
		j.env = env
		j.SubscriptionID = "sub-already-migrated"
		j.SetID("test-already-migrated")

		j.Run(t.Context())
		require.True(t, j.Status().Completed)
		require.False(t, j.HasErrors())

		// No new parameters should have been created.
		allParams, err := fakeparameter.FindByIDs(t.Context())
		require.NoError(t, err)
		assert.Empty(t, allParams)
	})

	t.Run("SkipsSubscriptionNotFound", func(t *testing.T) {
		env := setupWebhookMigrationTest(t)

		j := makeWebhookSecretMigrationJob()
		j.env = env
		j.SubscriptionID = "nonexistent"
		j.SetID("test-not-found")

		j.Run(t.Context())
		require.True(t, j.Status().Completed)
		require.False(t, j.HasErrors())
	})

	t.Run("SkipsEmptySecret", func(t *testing.T) {
		env := setupWebhookMigrationTest(t)

		// Insert a webhook subscription with no secret.
		doc := bson.M{
			"_id":           "sub-no-secret",
			"resource_type": "PATCH",
			"trigger":       "outcome",
			"selectors":     bson.A{bson.M{"type": "id", "data": "test"}},
			"subscriber": bson.M{
				"type": event.EvergreenWebhookSubscriberType,
				"target": bson.M{
					"url": "https://example.com/webhook",
				},
			},
			"owner":      "test-owner",
			"owner_type": "person",
		}
		require.NoError(t, db.Insert(t.Context(), event.SubscriptionsCollection, doc))

		j := makeWebhookSecretMigrationJob()
		j.env = env
		j.SubscriptionID = "sub-no-secret"
		j.SetID("test-no-secret")

		j.Run(t.Context())
		require.True(t, j.Status().Completed)
		require.False(t, j.HasErrors())

		// No parameters should have been created.
		allParams, err := fakeparameter.FindByIDs(t.Context())
		require.NoError(t, err)
		assert.Empty(t, allParams)
	})

	t.Run("MigratesAuthHeaderToParameterStore", func(t *testing.T) {
		env := setupWebhookMigrationTest(t)
		insertWebhookSubscriptionWithAuthHeader(t, "sub-auth", "Bearer token-xyz")

		j := makeWebhookSecretMigrationJob()
		j.env = env
		j.SubscriptionID = "sub-auth"
		j.SetID("test-auth-migrate")

		j.Run(t.Context())
		require.True(t, j.Status().Completed)
		require.False(t, j.HasErrors())

		raw := bson.M{}
		require.NoError(t, db.FindOneQ(t.Context(), event.SubscriptionsCollection, db.Query(bson.M{"_id": "sub-auth"}), &raw))
		subscriberRaw, ok := raw["subscriber"].(bson.M)
		require.True(t, ok)
		targetRaw, ok := subscriberRaw["target"].(bson.M)
		require.True(t, ok)
		authParamName, ok := targetRaw["authorization_header_parameter"].(string)
		require.True(t, ok, "authorization_parameter should be set")
		require.NotEmpty(t, authParamName)

		fakeParams, err := fakeparameter.FindByIDs(t.Context(), authParamName)
		require.NoError(t, err)
		require.Len(t, fakeParams, 1)
		assert.Equal(t, "Bearer token-xyz", fakeParams[0].Value)
	})

	t.Run("MigratesBothSecretAndAuthHeaderInOneRun", func(t *testing.T) {
		env := setupWebhookMigrationTest(t)
		insertWebhookSubscriptionWithBothUnmigrated(t, "sub-both", "my-secret", "Bearer my-token")

		j := makeWebhookSecretMigrationJob()
		j.env = env
		j.SubscriptionID = "sub-both"
		j.SetID("test-both-migrate")

		j.Run(t.Context())
		require.True(t, j.Status().Completed)
		require.False(t, j.HasErrors())

		raw := bson.M{}
		require.NoError(t, db.FindOneQ(t.Context(), event.SubscriptionsCollection, db.Query(bson.M{"_id": "sub-both"}), &raw))
		subscriberRaw, ok := raw["subscriber"].(bson.M)
		require.True(t, ok)
		targetRaw, ok := subscriberRaw["target"].(bson.M)
		require.True(t, ok)
		secretParamName, ok := targetRaw["secret_parameter"].(string)
		require.True(t, ok, "secret_parameter should be set")
		authParamName, ok := targetRaw["authorization_header_parameter"].(string)
		require.True(t, ok, "authorization_parameter should be set")

		fakeParams, err := fakeparameter.FindByIDs(t.Context(), secretParamName, authParamName)
		require.NoError(t, err)
		require.Len(t, fakeParams, 2)
	})

	t.Run("SkipsAlreadyMigratedAuthHeader", func(t *testing.T) {
		env := setupWebhookMigrationTest(t)

		doc := bson.M{
			"_id":           "sub-auth-already-migrated",
			"resource_type": event.ResourceTypePatch,
			"trigger":       event.TriggerOutcome,
			"selectors":     bson.A{bson.M{"type": "id", "data": "test"}},
			"subscriber": bson.M{
				"type": event.EvergreenWebhookSubscriberType,
				"target": bson.M{
					"url":                            "https://example.com/webhook",
					"secret":                         []byte("already-in-ps"),
					"secret_parameter":               "/some/secret/param",
					"headers":                        bson.A{bson.M{"key": event.WebhookAuthorizationHeader, "value": "Bearer token"}},
					"authorization_header_parameter": "/some/auth/param",
				},
			},
			"owner":      "test-owner",
			"owner_type": "person",
		}
		require.NoError(t, db.Insert(t.Context(), event.SubscriptionsCollection, doc))

		j := makeWebhookSecretMigrationJob()
		j.env = env
		j.SubscriptionID = "sub-auth-already-migrated"
		j.SetID("test-auth-already-migrated")

		j.Run(t.Context())
		require.True(t, j.Status().Completed)
		require.False(t, j.HasErrors())

		// No new parameters should have been created.
		allParams, err := fakeparameter.FindByIDs(t.Context())
		require.NoError(t, err)
		assert.Empty(t, allParams)
	})
}

func TestFindUnmigratedWebhookSubscriptionIDs(t *testing.T) {
	setupWebhookMigrationTest(t)

	// 3 secret-unmigrated, 1 auth-header-unmigrated, 1 fully migrated.
	insertUnmigratedWebhookSubscription(t, "unmigrated-1", "secret-1")
	insertUnmigratedWebhookSubscription(t, "unmigrated-2", "secret-2")
	insertUnmigratedWebhookSubscription(t, "unmigrated-3", "secret-3")
	insertWebhookSubscriptionWithAuthHeader(t, "unmigrated-auth", "Bearer token")

	migratedDoc := bson.M{
		"_id":           "already-migrated",
		"resource_type": event.ResourceTypePatch,
		"trigger":       event.TriggerOutcome,
		"selectors":     bson.A{bson.M{"type": "id", "data": "test"}},
		"subscriber": bson.M{
			"type": event.EvergreenWebhookSubscriberType,
			"target": bson.M{
				"url":              "https://example.com/webhook",
				"secret_parameter": "/some/param",
			},
		},
		"owner":      "test-owner",
		"owner_type": "person",
	}
	require.NoError(t, db.Insert(t.Context(), event.SubscriptionsCollection, migratedDoc))

	t.Run("ReturnsSecretAndAuthHeaderUnmigrated", func(t *testing.T) {
		ids, err := findUnmigratedWebhookSubscriptionIDs(t.Context(), 100)
		require.NoError(t, err)
		assert.Len(t, ids, 4)
		assert.NotContains(t, ids, "already-migrated")
		assert.Contains(t, ids, "unmigrated-auth")
	})

	t.Run("RespectsLimit", func(t *testing.T) {
		ids, err := findUnmigratedWebhookSubscriptionIDs(t.Context(), 2)
		require.NoError(t, err)
		assert.Len(t, ids, 2)
	})
}

// insertMigratedWebhookSubscription inserts a subscription that has both a
// secret and a secret_parameter, simulating a post-migration state.
func insertMigratedWebhookSubscription(t *testing.T, id, secret, paramPath string) {
	t.Helper()
	doc := bson.M{
		"_id":           id,
		"resource_type": event.ResourceTypePatch,
		"trigger":       event.TriggerOutcome,
		"selectors":     bson.A{bson.M{"type": "id", "data": "test"}},
		"subscriber": bson.M{
			"type": event.EvergreenWebhookSubscriberType,
			"target": bson.M{
				"url":              "https://example.com/webhook",
				"secret":           []byte(secret),
				"secret_parameter": paramPath,
			},
		},
		"owner":      "test-owner",
		"owner_type": "person",
	}
	require.NoError(t, db.Insert(t.Context(), event.SubscriptionsCollection, doc))
}

func TestWebhookSecretCleanupJobFactory(t *testing.T) {
	factory, err := registry.GetJobFactory(webhookSecretCleanupJobName)
	require.NoError(t, err)
	require.NotNil(t, factory)
	j := factory()
	require.NotNil(t, j)
	assert.Equal(t, webhookSecretCleanupJobName, j.Type().Name)
}

func TestWebhookSecretCleanupJobConstructor(t *testing.T) {
	j := NewWebhookSecretCleanupJob("sub-456", "2025-01-01")
	require.NotNil(t, j)
	assert.Equal(t, "webhook-secret-cleanup.sub-456.2025-01-01", j.ID())
	assert.Equal(t, webhookSecretCleanupJobName, j.Type().Name)
}

func TestWebhookSecretCleanupJobRun(t *testing.T) {
	t.Run("RemovesSecretFromMongoDB", func(t *testing.T) {
		env := setupWebhookMigrationTest(t)
		insertMigratedWebhookSubscription(t, "sub-cleanup", "old-secret", "/some/param/path")

		j := makeWebhookSecretCleanupJob()
		j.env = env
		j.SubscriptionID = "sub-cleanup"
		j.SetID("test-cleanup")

		j.Run(t.Context())
		require.True(t, j.Status().Completed)
		require.False(t, j.HasErrors())

		raw := bson.M{}
		require.NoError(t, db.FindOneQ(t.Context(), event.SubscriptionsCollection, db.Query(bson.M{"_id": "sub-cleanup"}), &raw))
		subscriberRaw, ok := raw["subscriber"].(bson.M)
		require.True(t, ok)
		targetRaw, ok := subscriberRaw["target"].(bson.M)
		require.True(t, ok)
		assert.Nil(t, targetRaw["secret"], "secret should be removed from MongoDB")
		assert.Equal(t, "/some/param/path", targetRaw["secret_parameter"], "secret_parameter should remain")
	})

	t.Run("SkipsNotMigratedSubscription", func(t *testing.T) {
		env := setupWebhookMigrationTest(t)
		insertUnmigratedWebhookSubscription(t, "sub-not-migrated", "still-in-mongo")

		j := makeWebhookSecretCleanupJob()
		j.env = env
		j.SubscriptionID = "sub-not-migrated"
		j.SetID("test-not-migrated")

		j.Run(t.Context())
		require.True(t, j.Status().Completed)
		require.False(t, j.HasErrors())

		// Secret should still be in MongoDB since it was not migrated.
		raw := bson.M{}
		require.NoError(t, db.FindOneQ(t.Context(), event.SubscriptionsCollection, db.Query(bson.M{"_id": "sub-not-migrated"}), &raw))
		subscriberRaw, ok := raw["subscriber"].(bson.M)
		require.True(t, ok)
		targetRaw, ok := subscriberRaw["target"].(bson.M)
		require.True(t, ok)
		assert.NotNil(t, targetRaw["secret"], "secret should remain since subscription is not migrated")
	})

	t.Run("SkipsSubscriptionNotFound", func(t *testing.T) {
		env := setupWebhookMigrationTest(t)

		j := makeWebhookSecretCleanupJob()
		j.env = env
		j.SubscriptionID = "nonexistent"
		j.SetID("test-cleanup-not-found")

		j.Run(t.Context())
		require.True(t, j.Status().Completed)
		require.False(t, j.HasErrors())
	})

	t.Run("RemovesAuthHeaderFromMongoDBAfterMigration", func(t *testing.T) {
		env := setupWebhookMigrationTest(t)

		doc := bson.M{
			"_id":           "sub-auth-cleanup",
			"resource_type": event.ResourceTypePatch,
			"trigger":       event.TriggerOutcome,
			"selectors":     bson.A{bson.M{"type": "id", "data": "test"}},
			"subscriber": bson.M{
				"type": event.EvergreenWebhookSubscriberType,
				"target": bson.M{
					"url":                            "https://example.com/webhook",
					"secret":                         []byte("old-secret"),
					"secret_parameter":               "/some/secret/param",
					"headers":                        bson.A{bson.M{"key": event.WebhookAuthorizationHeader, "value": "Bearer old-token"}},
					"authorization_header_parameter": "/some/auth/param",
				},
			},
			"owner":      "test-owner",
			"owner_type": "person",
		}
		require.NoError(t, db.Insert(t.Context(), event.SubscriptionsCollection, doc))

		j := makeWebhookSecretCleanupJob()
		j.env = env
		j.SubscriptionID = "sub-auth-cleanup"
		j.SetID("test-auth-cleanup")

		j.Run(t.Context())
		require.True(t, j.Status().Completed)
		require.False(t, j.HasErrors())

		raw := bson.M{}
		require.NoError(t, db.FindOneQ(t.Context(), event.SubscriptionsCollection, db.Query(bson.M{"_id": "sub-auth-cleanup"}), &raw))
		subscriberRaw, ok := raw["subscriber"].(bson.M)
		require.True(t, ok)
		targetRaw, ok := subscriberRaw["target"].(bson.M)
		require.True(t, ok)
		assert.Nil(t, targetRaw["secret"], "secret should be removed from MongoDB")
		assert.Equal(t, "/some/secret/param", targetRaw["secret_parameter"], "secret_parameter should remain")
		assert.Equal(t, "/some/auth/param", targetRaw["authorization_header_parameter"], "authorization_parameter should remain")

		// Verify Authorization header was removed from the headers array.
		headers, ok := targetRaw["headers"].(bson.A)
		require.True(t, ok)
		for _, h := range headers {
			headerMap, ok := h.(bson.M)
			require.True(t, ok)
			assert.NotEqual(t, event.WebhookAuthorizationHeader, headerMap["key"], "Authorization header should be removed")
		}
	})

	t.Run("SkipsAuthHeaderCleanupWhenNotMigrated", func(t *testing.T) {
		env := setupWebhookMigrationTest(t)
		insertWebhookSubscriptionWithAuthHeader(t, "sub-auth-not-migrated", "Bearer old-token")

		j := makeWebhookSecretCleanupJob()
		j.env = env
		j.SubscriptionID = "sub-auth-not-migrated"
		j.SetID("test-auth-not-migrated-cleanup")

		j.Run(t.Context())
		require.True(t, j.Status().Completed)
		require.False(t, j.HasErrors())

		// Authorization header should remain because authorization_parameter is not set.
		raw := bson.M{}
		require.NoError(t, db.FindOneQ(t.Context(), event.SubscriptionsCollection, db.Query(bson.M{"_id": "sub-auth-not-migrated"}), &raw))
		subscriberRaw, ok := raw["subscriber"].(bson.M)
		require.True(t, ok)
		targetRaw, ok := subscriberRaw["target"].(bson.M)
		require.True(t, ok)
		headers, ok := targetRaw["headers"].(bson.A)
		require.True(t, ok)
		require.Len(t, headers, 1, "Authorization header should still be present")
	})
}

func TestFindMigratedWebhookSubscriptionIDs(t *testing.T) {
	setupWebhookMigrationTest(t)

	insertMigratedWebhookSubscription(t, "migrated-1", "secret-1", "/param/1")
	insertMigratedWebhookSubscription(t, "migrated-2", "secret-2", "/param/2")
	insertUnmigratedWebhookSubscription(t, "unmigrated-1", "secret-3")

	// Insert a subscription where auth header has been migrated but not yet cleaned up.
	authMigratedDoc := bson.M{
		"_id":           "auth-migrated",
		"resource_type": event.ResourceTypePatch,
		"trigger":       event.TriggerOutcome,
		"selectors":     bson.A{bson.M{"type": "id", "data": "test"}},
		"subscriber": bson.M{
			"type": event.EvergreenWebhookSubscriberType,
			"target": bson.M{
				"url":                            "https://example.com/webhook",
				"headers":                        bson.A{bson.M{"key": event.WebhookAuthorizationHeader, "value": "Bearer old"}},
				"authorization_header_parameter": "/auth/param",
			},
		},
		"owner":      "test-owner",
		"owner_type": "person",
	}
	require.NoError(t, db.Insert(t.Context(), event.SubscriptionsCollection, authMigratedDoc))

	t.Run("ReturnsSecretAndAuthHeaderNeedingCleanup", func(t *testing.T) {
		ids, err := findMigratedWebhookSubscriptionIDs(t.Context(), 100)
		require.NoError(t, err)
		assert.Len(t, ids, 3)
		assert.NotContains(t, ids, "unmigrated-1")
		assert.Contains(t, ids, "auth-migrated")
	})

	t.Run("RespectsLimit", func(t *testing.T) {
		ids, err := findMigratedWebhookSubscriptionIDs(t.Context(), 1)
		require.NoError(t, err)
		assert.Len(t, ids, 1)
	})
}
