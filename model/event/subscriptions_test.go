package event

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud/parameterstore/fakeparameter"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestSubscriptions(t *testing.T) {
	suite.Run(t, &subscriptionsSuite{})
}

type subscriptionsSuite struct {
	suite.Suite
	subscriptions []Subscription
	now           time.Time
}

func (s *subscriptionsSuite) SetupTest() {
	s.NoError(db.ClearCollections(SubscriptionsCollection, fakeparameter.Collection))

	t1 := "someone@example.com"
	t2 := "someone2@example.com"
	t3 := "someone3@example.com"
	t4 := "someone4@example.com"
	t5 := "slack_user"
	s.now = time.Now().Round(time.Second)
	s.subscriptions = []Subscription{
		{
			ID:           "sub0",
			ResourceType: "type1",
			Trigger:      "trigger1",
			Selectors: []Selector{
				{
					Type: SelectorID,
					Data: "something",
				},
			},
			RegexSelectors: []Selector{},
			Filter:         Filter{ID: "something"},
			Subscriber: Subscriber{
				Type:   EmailSubscriberType,
				Target: &t1,
			},
			Owner:     "me",
			OwnerType: OwnerTypePerson,
		},
		{
			ID:           "sub1",
			ResourceType: "type1",
			Trigger:      "trigger1",
			Selectors: []Selector{
				{
					Type: SelectorProject,
					Data: "somethingelse",
				},
			},
			RegexSelectors: []Selector{},
			Filter:         Filter{Project: "somethingelse"},
			Subscriber: Subscriber{
				Type:   EmailSubscriberType,
				Target: &t2,
			},
			Owner:     "you",
			OwnerType: OwnerTypePerson,
		},
		{
			ID:           "sub2",
			ResourceType: "type1",
			Trigger:      "trigger1",
			Selectors: []Selector{
				{
					Type: SelectorID,
					Data: "something",
				},
			},
			RegexSelectors: []Selector{
				{
					Type: SelectorProject,
					Data: "else$",
				},
			},
			Filter: Filter{
				ID: "something",
			},
			Subscriber: Subscriber{
				Type:   EmailSubscriberType,
				Target: &t3,
			},
			Owner: "someone",
		},
		{
			ID:           "sub3",
			ResourceType: "type2",
			Trigger:      "trigger2",
			Selectors: []Selector{
				{
					Type: SelectorObject,
					Data: "somethingspecial",
				},
			},
			RegexSelectors: []Selector{},
			Filter:         Filter{Object: "somethingspecial"},
			Subscriber: Subscriber{
				Type:   EmailSubscriberType,
				Target: &t4,
			},
			Owner:     "me",
			OwnerType: OwnerTypePerson,
			TriggerData: map[string]string{
				"key1": "val1",
				"key2": "val2",
			},
		},
		{
			ID:           "sub4",
			ResourceType: "type2",
			Trigger:      "trigger2",
			Selectors: []Selector{
				{
					Type: SelectorObject,
					Data: "somethingspecial",
				},
			},
			RegexSelectors: []Selector{},
			Filter:         Filter{Object: "somethingspecial"},
			Subscriber: Subscriber{
				Type:   SlackSubscriberType,
				Target: &t5,
			},
			Owner:       "me",
			OwnerType:   OwnerTypeProject,
			LastUpdated: s.now,
		},
		NewPatchOutcomeSubscriptionByOwner("user_0", Subscriber{
			Type:   EmailSubscriberType,
			Target: "a@b.com",
		}),
	}

	for _, sub := range s.subscriptions {
		s.NoError(sub.Upsert(s.T().Context()))
	}
}

func (s *subscriptionsSuite) TestUpsert() {
	out := []Subscription{}
	s.NoError(db.FindAllQ(s.T().Context(), SubscriptionsCollection, db.Q{}, &out))

	s.Require().Len(out, 6)

	for _, sub := range out {
		if sub.ID == s.subscriptions[3].ID {
			s.Equal(sub.Owner, s.subscriptions[3].Owner)
			s.Equal(sub.OwnerType, s.subscriptions[3].OwnerType)
			s.Equal(sub.Selectors, s.subscriptions[3].Selectors)
			s.Equal(s.subscriptions[3].RegexSelectors, sub.RegexSelectors)
			s.Equal(s.subscriptions[3].Subscriber, sub.Subscriber)
			s.Equal(s.subscriptions[3].Filter, sub.Filter)
			s.Equal(s.subscriptions[3].TriggerData, sub.TriggerData)
		}
	}
}

func (s *subscriptionsSuite) TestRemove() {
	for i := range s.subscriptions {
		s.NoError(RemoveSubscription(s.T().Context(), s.subscriptions[i].ID))

		out := []Subscription{}
		s.NoError(db.FindAllQ(s.T().Context(), SubscriptionsCollection, db.Q{}, &out))
		s.Len(out, len(s.subscriptions)-i-1)
	}
}

func (s *subscriptionsSuite) TestAttributesFilterQuery() {
	s.Run("EmptySlice", func() {
		a := Attributes{Object: []string{}}
		s.Equal(bson.M{
			filterObjectKey:       nil,
			filterIDKey:           nil,
			filterProjectKey:      nil,
			filterOwnerKey:        nil,
			filterRequesterKey:    nil,
			filterStatusKey:       nil,
			filterDisplayNameKey:  nil,
			filterBuildVariantKey: nil,
			filterInVersionKey:    nil,
			filterInBuildKey:      nil,
		}, a.filterQuery())
	})

	s.Run("PopulatedFields", func() {
		a := Attributes{
			Object:    []string{"TASK"},
			Requester: []string{evergreen.TriggerRequester, evergreen.RepotrackerVersionRequester},
		}
		s.Equal(bson.M{
			filterObjectKey: bson.M{
				"$in": bson.A{
					nil,
					"TASK",
				}},
			filterIDKey:      nil,
			filterProjectKey: nil,
			filterOwnerKey:   nil,
			filterRequesterKey: bson.M{
				"$in": bson.A{
					nil,
					evergreen.TriggerRequester,
					evergreen.RepotrackerVersionRequester,
				}},
			filterStatusKey:       nil,
			filterDisplayNameKey:  nil,
			filterBuildVariantKey: nil,
			filterInVersionKey:    nil,
			filterInBuildKey:      nil,
		}, a.filterQuery())
	})
}

func (s *subscriptionsSuite) TestAttributesIsUnset() {
	s.Run("EmptySlice", func() {
		a := Attributes{Object: []string{}}
		s.True(a.isUnset())
	})

	s.Run("PopulatedField", func() {
		a := Attributes{Object: []string{"TASK"}}
		s.False(a.isUnset())
	})
}

func (s *subscriptionsSuite) TestAttributesToSelectorMap() {
	s.Run("EmptySlice", func() {
		a := Attributes{Object: []string{}}
		s.Empty(a.ToSelectorMap())
	})

	s.Run("PopulatedField", func() {
		a := Attributes{Object: []string{"TASK"}}
		objectValues := a.ToSelectorMap()[SelectorObject]
		s.Require().Len(objectValues, 1)
		s.Equal("TASK", objectValues[0])
	})
}

func (s *subscriptionsSuite) TestAttributesValuesForSelector() {
	s.Run("UnsetField", func() {
		a := Attributes{}
		s.Nil(a.valuesForSelector(SelectorObject))
	})

	s.Run("ExistingField", func() {
		a := Attributes{Object: []string{"TASK"}}
		fields, err := a.valuesForSelector(SelectorObject)
		s.NoError(err)
		s.Require().Len(fields, 1)
		s.Equal("TASK", fields[0])
	})

	s.Run("NonexistentField", func() {
		a := Attributes{}
		fields, err := a.valuesForSelector("no-such-thing")
		s.Error(err)
		s.Nil(fields)
	})
}

func (s *subscriptionsSuite) TestFindSubscriptionsByAttributes() {
	s.Run("EmptySelectors", func() {
		subs, err := FindSubscriptionsByAttributes(s.T().Context(), "type2", Attributes{})
		s.NoError(err)
		s.Empty(subs)
	})

	s.Run("NothingMatches", func() {
		subs, err := FindSubscriptionsByAttributes(s.T().Context(), "type1", Attributes{
			Object: []string{"nothing_matches"},
		})
		s.NoError(err)
		s.Empty(subs)
	})

	s.Run("MatchesMultipleSubscriptions", func() {
		subs, err := FindSubscriptionsByAttributes(s.T().Context(), "type2", Attributes{
			Object: []string{"somethingspecial"},
		})
		s.NoError(err)
		s.Len(subs, 2)
		expectedSubs := []string{s.subscriptions[3].ID, s.subscriptions[4].ID}
		for _, sub := range subs {
			s.Contains(expectedSubs, sub.ID)
		}
	})

	s.Run("MatchesRegexSelector", func() {
		subs, err := FindSubscriptionsByAttributes(s.T().Context(), "type1", Attributes{
			ID:      []string{"something"},
			Project: []string{"somethingelse"},
		})
		s.NoError(err)
		s.Len(subs, 3)
		expectedSubs := []string{s.subscriptions[0].ID, s.subscriptions[1].ID, s.subscriptions[2].ID}
		for _, sub := range subs {
			s.Contains(expectedSubs, sub.ID)
		}
	})
}

func (s *subscriptionsSuite) TestFilterRegexSelectors() {
	eventAttributes := Attributes{
		Object: []string{"apple"},
		Status: []string{"sweet"},
	}

	s.Run("MultipleMatches", func() {
		subs := []Subscription{
			{
				RegexSelectors: []Selector{
					{
						Type: SelectorObject,
						Data: "apple",
					},
				},
			},
			{
				RegexSelectors: []Selector{
					{
						Type: SelectorStatus,
						Data: "sweet",
					},
				},
			},
		}

		filtered := filterRegexSelectors(subs, eventAttributes)
		s.Len(filtered, 2)
		expectedSubs := []Subscription{subs[0], subs[1]}
		for _, sub := range filtered {
			s.Contains(expectedSubs, sub)
		}
	})

	s.Run("SingleMatch", func() {
		subs := []Subscription{
			{
				RegexSelectors: []Selector{
					{
						Type: SelectorObject,
						Data: "apple",
					},
				},
			},
			{
				RegexSelectors: []Selector{
					{
						Type: SelectorStatus,
						Data: "sour",
					},
				},
			},
		}

		filtered := filterRegexSelectors(subs, eventAttributes)
		s.Require().Len(filtered, 1)
		s.Equal(subs[0], filtered[0])
	})

	s.Run("NoMatches", func() {
		subs := []Subscription{
			{
				RegexSelectors: []Selector{
					{
						Type: SelectorObject,
						Data: "orange",
					},
				},
			},
			{
				RegexSelectors: []Selector{
					{
						Type: SelectorStatus,
						Data: "tangy",
					},
				},
			},
		}

		s.Empty(filterRegexSelectors(subs, eventAttributes))
	})

	s.Run("NoRegexSelectors", func() {
		subs := []Subscription{{ID: "sub0"}}
		filtered := filterRegexSelectors(subs, eventAttributes)
		s.Require().Len(filtered, 1)
		s.Equal(subs[0], filtered[0])
	})
}

func (s *subscriptionsSuite) TestRegexSelectorsMatchEvent() {
	eventAttributes := Attributes{
		Object: []string{"apple"},
		Status: []string{"sweet"},
	}

	s.Run("AllMatch", func() {
		regexSelectors := []Selector{
			{
				Type: SelectorObject,
				Data: "^apple",
			},
			{
				Type: SelectorStatus,
				Data: "sweet$",
			},
		}
		s.True(regexSelectorsMatchEvent(regexSelectors, eventAttributes))
	})

	s.Run("MixedMatch", func() {
		regexSelectors := []Selector{
			{
				Type: SelectorObject,
				Data: "^apple",
			},
			{
				Type: SelectorStatus,
				Data: "sour",
			},
		}
		s.False(regexSelectorsMatchEvent(regexSelectors, eventAttributes))
	})
}

func (s *subscriptionsSuite) TestRegexMatchesValue() {
	s.Run("NoValues", func() {
		s.False(regexMatchesValue("regex", nil))
	})

	s.Run("NoMatch", func() {
		s.False(regexMatchesValue("^hello", []string{"goodbye"}))
	})

	s.Run("MatchValue", func() {
		s.True(regexMatchesValue("^hello", []string{"goodbye", "helloworld"}))
	})

	s.Run("InvalidRegex", func() {
		s.False(regexMatchesValue("[", []string{"["}))
	})
}

func (s *subscriptionsSuite) TestFilterFromSelectors() {
	for _, sub := range s.subscriptions {
		filter := Filter{}
		s.Require().NoError(filter.FromSelectors(sub.Selectors))
		s.Equal(sub.Filter, filter)
	}

	filter := Filter{}
	s.Error(filter.FromSelectors([]Selector{{Type: "non-existent-type"}}))
}

func (s *subscriptionsSuite) TestValidateSelectors() {
	for _, sub := range s.subscriptions {
		s.NoError(sub.ValidateSelectors())
	}

	noFilterParams := Subscription{}
	s.Error(noFilterParams.ValidateSelectors())
}

func (s *subscriptionsSuite) TestFromSelectors() {
	s.Run("NoType", func() {
		f := Filter{}
		s.Error(f.FromSelectors([]Selector{{Data: "id"}}))
	})

	s.Run("NoData", func() {
		f := Filter{}
		s.Error(f.FromSelectors([]Selector{{Type: SelectorID}}))
	})

	s.Run("DuplicateSelectors", func() {
		f := Filter{}
		s.Error(f.FromSelectors([]Selector{
			{Type: SelectorID, Data: "id1"},
			{Type: SelectorID, Data: "id2"},
		}))
	})

	s.Run("InvalidSelectorType", func() {
		f := Filter{}
		s.Error(f.FromSelectors([]Selector{{Type: "not-a-type", Data: "data"}}))
	})

	s.Run("ValidSelectors", func() {
		f := Filter{}
		s.NoError(f.FromSelectors([]Selector{
			{Type: SelectorID, Data: "id"},
			{Type: SelectorObject, Data: "obj"},
		}))
		s.Equal("id", f.ID)
		s.Equal("obj", f.Object)
	})
}

func (s *subscriptionsSuite) TestFindByOwnerForPerson() {
	subscriptions, err := FindSubscriptionsByOwner(s.T().Context(), "me", OwnerTypePerson)
	s.NoError(err)
	s.Len(subscriptions, 2)
	for _, sub := range subscriptions {
		s.Equal("me", sub.Owner)
		s.EqualValues(OwnerTypePerson, sub.OwnerType)
	}
}

func (s *subscriptionsSuite) TestFindByOwnerForProject() {
	subscriptions, err := FindSubscriptionsByOwner(s.T().Context(), "me", OwnerTypeProject)
	s.NoError(err)
	s.Require().Len(subscriptions, 1)
	s.Equal("me", subscriptions[0].Owner)
	s.EqualValues(OwnerTypeProject, subscriptions[0].OwnerType)
}

func (s *subscriptionsSuite) TestFindSubscriptionsByOwner() {
	for i := range s.subscriptions {
		sub, err := FindSubscriptionByID(s.T().Context(), s.subscriptions[i].ID)
		s.NoError(err)
		s.NotNil(sub)
		s.NotEqual("", sub.ID)
	}

	s.NoError(db.ClearCollections(SubscriptionsCollection))
	sub, err := FindSubscriptionByID(s.T().Context(), s.subscriptions[0].ID)
	s.NoError(err)
	s.Nil(sub)
}

func (s *subscriptionsSuite) TestCreateOrUpdateGeneralSubscription() {
	subscriber := Subscriber{
		Type:   SlackSubscriberType,
		Target: "@octocat",
	}

	subscription, err := CreateOrUpdateGeneralSubscription(s.T().Context(), GeneralSubscriptionPatchOutcome, "",
		subscriber, "octocat")
	s.NoError(err)

	subscriptions, err := FindSubscriptionsByOwner(s.T().Context(), "octocat", OwnerTypePerson)
	s.NoError(err)
	s.Require().Len(subscriptions, 1)
	s.Equal(subscriptions[0].ID, subscription.ID)
}

func (s *subscriptionsSuite) TestUpsertWebhookSavesSecretToParameterStore() {
	webhookSub := &WebhookSubscriber{
		URL:    "https://example.com/webhook",
		Secret: []byte("my-secret"),
	}
	sub := Subscription{
		ID:           "webhook-sub",
		ResourceType: ResourceTypePatch,
		Trigger:      TriggerOutcome,
		Selectors:    []Selector{{Type: SelectorID, Data: "test"}},
		Filter:       Filter{ID: "test"},
		Subscriber: Subscriber{
			Type:   EvergreenWebhookSubscriberType,
			Target: webhookSub,
		},
		Owner:     "me",
		OwnerType: OwnerTypePerson,
	}
	s.Require().NoError(sub.Upsert(s.T().Context()))

	// Secret should have been saved to Parameter Store.
	s.Require().NotEmpty(webhookSub.SecretParameter)
	fakeParams, err := fakeparameter.FindByIDs(s.T().Context(), webhookSub.SecretParameter)
	s.Require().NoError(err)
	s.Require().Len(fakeParams, 1)
	s.Equal("my-secret", fakeParams[0].Value)

	// Both the secret bytes and the Parameter Store path are persisted to MongoDB during
	// Phase 1. The secret stays in MongoDB as a fallback until the Phase 2 cleanup job removes it.
	// TODO(DEVPROD-15500): remove this assertion once Phase 2 cleanup job removes secrets from MongoDB.
	raw := bson.M{}
	s.Require().NoError(db.FindOneQ(s.T().Context(), SubscriptionsCollection, db.Query(bson.M{"_id": "webhook-sub"}), &raw))
	subscriberRaw, ok := raw["subscriber"].(bson.M)
	s.Require().True(ok)
	targetRaw, ok := subscriberRaw["target"].(bson.M)
	s.Require().True(ok)
	storedSecret, ok := targetRaw["secret"].(primitive.Binary)
	s.Require().True(ok, "secret should be persisted to MongoDB as a fallback during Phase 1")
	s.Equal([]byte("my-secret"), storedSecret.Data)
	s.NotEmpty(targetRaw["secret_parameter"], "secret_parameter path should be stored in MongoDB")
}

func (s *subscriptionsSuite) TestFindSubscriptionByIDPopulatesSecretFromParameterStore() {
	webhookSub := &WebhookSubscriber{
		URL:    "https://example.com/webhook",
		Secret: []byte("ps-secret"),
	}
	sub := Subscription{
		ID:           "webhook-find",
		ResourceType: ResourceTypePatch,
		Trigger:      TriggerOutcome,
		Selectors:    []Selector{{Type: SelectorID, Data: "test"}},
		Filter:       Filter{ID: "test"},
		Subscriber: Subscriber{
			Type:   EvergreenWebhookSubscriberType,
			Target: webhookSub,
		},
		Owner:     "me",
		OwnerType: OwnerTypePerson,
	}
	s.Require().NoError(sub.Upsert(s.T().Context()))

	found, err := FindSubscriptionByID(s.T().Context(), "webhook-find")
	s.Require().NoError(err)
	s.Require().NotNil(found)
	foundWebhook, ok := found.Subscriber.Target.(*WebhookSubscriber)
	s.Require().True(ok)
	s.Require().NotNil(foundWebhook)
	s.Equal([]byte("ps-secret"), foundWebhook.Secret)
	s.NotEmpty(foundWebhook.SecretParameter)
}

func (s *subscriptionsSuite) TestFindSubscriptionByIDFallsBackToMongoDBSecret() {
	// Simulate a legacy subscription with secret in MongoDB but no SecretParameter.
	// Insert directly to bypass the Upsert write path which saves to PS.
	sub := Subscription{
		ID:           "legacy-webhook",
		ResourceType: ResourceTypePatch,
		Trigger:      TriggerOutcome,
		Filter:       Filter{ID: "test"},
		Subscriber: Subscriber{
			Type: EvergreenWebhookSubscriberType,
			Target: &WebhookSubscriber{
				URL:    "https://legacy.example.com",
				Secret: []byte("legacy-secret"),
			},
		},
		Owner:     "me",
		OwnerType: OwnerTypePerson,
	}
	_, err := evergreen.GetEnvironment().DB().Collection(SubscriptionsCollection).InsertOne(s.T().Context(), sub)
	s.Require().NoError(err)

	found, err := FindSubscriptionByID(s.T().Context(), "legacy-webhook")
	s.Require().NoError(err)
	s.Require().NotNil(found)
	foundWebhook, ok := found.Subscriber.Target.(*WebhookSubscriber)
	s.Require().True(ok)
	s.Require().NotNil(foundWebhook)
	s.Equal([]byte("legacy-secret"), foundWebhook.Secret, "should fall back to MongoDB secret")
	s.Empty(foundWebhook.SecretParameter, "legacy subscription should not have a parameter path")
}

func (s *subscriptionsSuite) TestFindSubscriptionsByOwnerPopulatesSecrets() {
	webhookSub := &WebhookSubscriber{
		URL:    "https://example.com/webhook",
		Secret: []byte("owner-secret"),
	}
	sub := Subscription{
		ID:           "webhook-owner",
		ResourceType: ResourceTypePatch,
		Trigger:      TriggerOutcome,
		Selectors:    []Selector{{Type: SelectorID, Data: "test"}},
		Filter:       Filter{ID: "test"},
		Subscriber: Subscriber{
			Type:   EvergreenWebhookSubscriberType,
			Target: webhookSub,
		},
		Owner:     "webhook-owner-user",
		OwnerType: OwnerTypePerson,
	}
	s.Require().NoError(sub.Upsert(s.T().Context()))

	subs, err := FindSubscriptionsByOwner(s.T().Context(), "webhook-owner-user", OwnerTypePerson)
	s.Require().NoError(err)
	s.Require().Len(subs, 1)
	foundWebhook, ok := subs[0].Subscriber.Target.(*WebhookSubscriber)
	s.Require().True(ok)
	s.Require().NotNil(foundWebhook)
	s.Equal([]byte("owner-secret"), foundWebhook.Secret)
}

func (s *subscriptionsSuite) TestRemoveSubscriptionDeletesWebhookSecretFromParameterStore() {
	webhookSub := &WebhookSubscriber{
		URL:    "https://example.com/webhook",
		Secret: []byte("delete-me-secret"),
	}
	sub := Subscription{
		ID:           "webhook-delete",
		ResourceType: ResourceTypePatch,
		Trigger:      TriggerOutcome,
		Selectors:    []Selector{{Type: SelectorID, Data: "test"}},
		Filter:       Filter{ID: "test"},
		Subscriber: Subscriber{
			Type:   EvergreenWebhookSubscriberType,
			Target: webhookSub,
		},
		Owner:     "me",
		OwnerType: OwnerTypePerson,
	}
	s.Require().NoError(sub.Upsert(s.T().Context()))
	paramName := webhookSub.SecretParameter
	s.Require().NotEmpty(paramName)

	fakeParams, err := fakeparameter.FindByIDs(s.T().Context(), paramName)
	s.Require().NoError(err)
	s.Require().Len(fakeParams, 1)

	s.Require().NoError(RemoveSubscription(s.T().Context(), "webhook-delete"))

	found, err := FindSubscriptionByID(s.T().Context(), "webhook-delete")
	s.Require().NoError(err)
	s.Nil(found)

	fakeParams, err = fakeparameter.FindByIDs(s.T().Context(), paramName)
	s.Require().NoError(err)
	s.Empty(fakeParams, "webhook secret should be removed from Parameter Store when its subscription is deleted")
}

// TODO(DEVPROD-15500): remove this test once the migration job has backfilled all legacy subscriptions.
func (s *subscriptionsSuite) TestRemoveSubscriptionDoesNotTouchParameterStoreForLegacyWebhook() {
	// Simulate a legacy webhook subscription with the secret in MongoDB but no SecretParameter set.
	// Insert directly to bypass the Upsert write path, which would otherwise save to Parameter Store.
	sub := Subscription{
		ID:           "legacy-webhook-delete",
		ResourceType: ResourceTypePatch,
		Trigger:      TriggerOutcome,
		Filter:       Filter{ID: "test"},
		Subscriber: Subscriber{
			Type: EvergreenWebhookSubscriberType,
			Target: &WebhookSubscriber{
				URL:    "https://legacy.example.com",
				Secret: []byte("legacy-secret"),
			},
		},
		Owner:     "me",
		OwnerType: OwnerTypePerson,
	}
	_, err := evergreen.GetEnvironment().DB().Collection(SubscriptionsCollection).InsertOne(s.T().Context(), sub)
	s.Require().NoError(err)

	allParams, err := fakeparameter.FindByIDs(s.T().Context())
	s.Require().NoError(err)
	s.Require().Empty(allParams)

	s.Require().NoError(RemoveSubscription(s.T().Context(), "legacy-webhook-delete"))

	found, err := FindSubscriptionByID(s.T().Context(), "legacy-webhook-delete")
	s.Require().NoError(err)
	s.Nil(found)

	allParams, err = fakeparameter.FindByIDs(s.T().Context())
	s.Require().NoError(err)
	s.Empty(allParams, "legacy webhook removal should not touch Parameter Store since no SecretParameter is set")
}

func (s *subscriptionsSuite) TestUpsertWebhookSavesAuthHeaderToParameterStore() {
	webhookSub := &WebhookSubscriber{
		URL:    "https://example.com/webhook",
		Secret: []byte("my-secret"),
		Headers: []WebhookHeader{
			{Key: "Authorization", Value: "Bearer my-token"},
			{Key: "X-Custom", Value: "custom-value"},
		},
	}
	sub := Subscription{
		ID:           "webhook-auth-sub",
		ResourceType: ResourceTypePatch,
		Trigger:      TriggerOutcome,
		Selectors:    []Selector{{Type: SelectorID, Data: "test"}},
		Filter:       Filter{ID: "test"},
		Subscriber: Subscriber{
			Type:   EvergreenWebhookSubscriberType,
			Target: webhookSub,
		},
		Owner:     "me",
		OwnerType: OwnerTypePerson,
	}
	s.Require().NoError(sub.Upsert(s.T().Context()))

	s.Require().NotEmpty(webhookSub.AuthorizationHeaderParameter)
	authParams, err := fakeparameter.FindByIDs(s.T().Context(), webhookSub.AuthorizationHeaderParameter)
	s.Require().NoError(err)
	s.Require().Len(authParams, 1)
	s.Equal("Bearer my-token", authParams[0].Value)

	found, err := FindSubscriptionByID(s.T().Context(), "webhook-auth-sub")
	s.Require().NoError(err)
	s.Require().NotNil(found)
	foundWebhook, ok := found.Subscriber.Target.(*WebhookSubscriber)
	s.Require().True(ok)
	s.Require().NotNil(foundWebhook)
	s.NotEmpty(foundWebhook.AuthorizationHeaderParameter, "authorization_parameter path should be stored in MongoDB")
	s.Equal("Bearer my-token", foundWebhook.GetHeader("Authorization"), "Authorization header should be populated from Parameter Store on read")
}

func (s *subscriptionsSuite) TestUpsertWebhookWithoutAuthHeaderDoesNotCreateAuthParameter() {
	webhookSub := &WebhookSubscriber{
		URL:    "https://example.com/webhook",
		Secret: []byte("my-secret"),
	}
	sub := Subscription{
		ID:           "webhook-no-auth",
		ResourceType: ResourceTypePatch,
		Trigger:      TriggerOutcome,
		Selectors:    []Selector{{Type: SelectorID, Data: "test"}},
		Filter:       Filter{ID: "test"},
		Subscriber: Subscriber{
			Type:   EvergreenWebhookSubscriberType,
			Target: webhookSub,
		},
		Owner:     "me",
		OwnerType: OwnerTypePerson,
	}
	s.Require().NoError(sub.Upsert(s.T().Context()))

	s.Empty(webhookSub.AuthorizationHeaderParameter, "no auth header means no authorization_parameter")
	allParams, err := fakeparameter.FindByIDs(s.T().Context())
	s.Require().NoError(err)
	s.Require().Len(allParams, 1, "only the secret parameter should exist")
}

func (s *subscriptionsSuite) TestFindSubscriptionByIDPopulatesAuthHeaderFromParameterStore() {
	webhookSub := &WebhookSubscriber{
		URL:    "https://example.com/webhook",
		Secret: []byte("my-secret"),
		Headers: []WebhookHeader{
			{Key: "Authorization", Value: "Bearer ps-token"},
		},
	}
	sub := Subscription{
		ID:           "webhook-auth-find",
		ResourceType: ResourceTypePatch,
		Trigger:      TriggerOutcome,
		Selectors:    []Selector{{Type: SelectorID, Data: "test"}},
		Filter:       Filter{ID: "test"},
		Subscriber: Subscriber{
			Type:   EvergreenWebhookSubscriberType,
			Target: webhookSub,
		},
		Owner:     "me",
		OwnerType: OwnerTypePerson,
	}
	s.Require().NoError(sub.Upsert(s.T().Context()))

	found, err := FindSubscriptionByID(s.T().Context(), "webhook-auth-find")
	s.Require().NoError(err)
	s.Require().NotNil(found)
	foundWebhook, ok := found.Subscriber.Target.(*WebhookSubscriber)
	s.Require().True(ok)
	s.Require().NotNil(foundWebhook)
	s.Equal("Bearer ps-token", foundWebhook.GetHeader("Authorization"))
	s.NotEmpty(foundWebhook.AuthorizationHeaderParameter)
}

// TODO(DEVPROD-15500): remove this test once the migration job has backfilled all legacy subscriptions.
func (s *subscriptionsSuite) TestFindSubscriptionByIDFallsBackToMongoDBAuthHeader() {
	// Insert directly (bypassing Upsert) to simulate a legacy subscription that has the
	// Authorization header in MongoDB but no authorization_parameter set.
	sub := Subscription{
		ID:           "legacy-webhook-auth",
		ResourceType: ResourceTypePatch,
		Trigger:      TriggerOutcome,
		Filter:       Filter{ID: "test"},
		Subscriber: Subscriber{
			Type: EvergreenWebhookSubscriberType,
			Target: &WebhookSubscriber{
				URL:    "https://legacy.example.com",
				Secret: []byte("legacy-secret"),
				Headers: []WebhookHeader{
					{Key: "Authorization", Value: "Bearer legacy-token"},
				},
			},
		},
		Owner:     "me",
		OwnerType: OwnerTypePerson,
	}
	s.Require().NoError(db.Insert(s.T().Context(), SubscriptionsCollection, sub))

	found, err := FindSubscriptionByID(s.T().Context(), "legacy-webhook-auth")
	s.Require().NoError(err)
	s.Require().NotNil(found)
	foundWebhook, ok := found.Subscriber.Target.(*WebhookSubscriber)
	s.Require().True(ok)
	s.Require().NotNil(foundWebhook)
	s.Equal("Bearer legacy-token", foundWebhook.GetHeader("Authorization"), "should fall back to MongoDB auth header")
	s.Empty(foundWebhook.AuthorizationHeaderParameter)
}

func (s *subscriptionsSuite) TestRemoveSubscriptionDeletesAuthHeaderFromParameterStore() {
	webhookSub := &WebhookSubscriber{
		URL:    "https://example.com/webhook",
		Secret: []byte("delete-me-secret"),
		Headers: []WebhookHeader{
			{Key: "Authorization", Value: "Bearer delete-me-token"},
		},
	}
	sub := Subscription{
		ID:           "webhook-auth-delete",
		ResourceType: ResourceTypePatch,
		Trigger:      TriggerOutcome,
		Selectors:    []Selector{{Type: SelectorID, Data: "test"}},
		Filter:       Filter{ID: "test"},
		Subscriber: Subscriber{
			Type:   EvergreenWebhookSubscriberType,
			Target: webhookSub,
		},
		Owner:     "me",
		OwnerType: OwnerTypePerson,
	}
	s.Require().NoError(sub.Upsert(s.T().Context()))
	authParamName := webhookSub.AuthorizationHeaderParameter
	s.Require().NotEmpty(authParamName)

	s.Require().NoError(RemoveSubscription(s.T().Context(), "webhook-auth-delete"))

	found, err := FindSubscriptionByID(s.T().Context(), "webhook-auth-delete")
	s.Require().NoError(err)
	s.Nil(found)

	authParams, err := fakeparameter.FindByIDs(s.T().Context(), authParamName)
	s.Require().NoError(err)
	s.Empty(authParams, "webhook Authorization header should be removed from Parameter Store when its subscription is deleted")
}

func (s *subscriptionsSuite) TestUpsertWebhookDeletesAuthParameterWhenHeaderRemoved() {
	webhookSub := &WebhookSubscriber{
		URL:    "https://example.com/webhook",
		Secret: []byte("my-secret"),
		Headers: []WebhookHeader{
			{Key: "Authorization", Value: "Bearer original-token"},
		},
	}
	sub := Subscription{
		ID:           "webhook-auth-header-removed",
		ResourceType: ResourceTypePatch,
		Trigger:      TriggerOutcome,
		Selectors:    []Selector{{Type: SelectorID, Data: "test"}},
		Filter:       Filter{ID: "test"},
		Subscriber: Subscriber{
			Type:   EvergreenWebhookSubscriberType,
			Target: webhookSub,
		},
		Owner:     "me",
		OwnerType: OwnerTypePerson,
	}
	s.Require().NoError(sub.Upsert(s.T().Context()))
	authParamName := webhookSub.AuthorizationHeaderParameter
	s.Require().NotEmpty(authParamName, "authorization_parameter should be set after initial upsert")

	updatedWebhookSub := &WebhookSubscriber{
		URL:    "https://example.com/webhook",
		Secret: []byte("my-secret"),
		Headers: []WebhookHeader{
			{Key: "X-Custom-Header", Value: "custom-value"},
		},
	}
	sub.Subscriber.Target = updatedWebhookSub
	s.Require().NoError(sub.Upsert(s.T().Context()))

	authParams, err := fakeparameter.FindByIDs(s.T().Context(), authParamName)
	s.Require().NoError(err)
	s.Empty(authParams, "old Authorization header PS entry should be deleted when header is removed")

	updated, err := FindSubscriptionByID(s.T().Context(), "webhook-auth-header-removed")
	s.Require().NoError(err)
	s.Require().NotNil(updated)
	updatedTarget, ok := updated.Subscriber.Target.(*WebhookSubscriber)
	s.Require().True(ok)
	s.Empty(updatedTarget.AuthorizationHeaderParameter, "authorization_parameter should be cleared")
	s.Equal("custom-value", updatedTarget.GetHeader("X-Custom-Header"), "non-sensitive header should remain")
}

func TestSetHeader(t *testing.T) {
	t.Run("UpdatesExistingKey", func(t *testing.T) {
		ws := &WebhookSubscriber{
			Headers: []WebhookHeader{
				{Key: "Authorization", Value: "Bearer old-token"},
				{Key: "X-Custom", Value: "custom"},
			},
		}
		ws.setHeader("Authorization", "Bearer new-token")
		require.Len(t, ws.Headers, 2)
		assert.Equal(t, "Bearer new-token", ws.GetHeader("Authorization"))
		assert.Equal(t, "custom", ws.GetHeader("X-Custom"))
	})

	t.Run("AddsNewKey", func(t *testing.T) {
		ws := &WebhookSubscriber{
			Headers: []WebhookHeader{
				{Key: "X-Custom", Value: "custom"},
			},
		}
		ws.setHeader("Authorization", "Bearer new-token")
		require.Len(t, ws.Headers, 2)
		assert.Equal(t, "Bearer new-token", ws.GetHeader("Authorization"))
	})
}

func TestCopyProjectSubscriptions(t *testing.T) {
	require.NoError(t, db.ClearCollections(SubscriptionsCollection))
	oldProjectId := "my-project"
	subs := []Subscription{
		{
			ID:           mgobson.NewObjectId().Hex(),
			Owner:        oldProjectId,
			OwnerType:    OwnerTypeProject,
			ResourceType: ResourceTypePatch,
			Trigger:      TriggerOutcome,
			Selectors: []Selector{
				{
					Type: SelectorProject,
					Data: oldProjectId,
				},
			},
			Filter: Filter{Project: oldProjectId},
			Subscriber: Subscriber{
				Type:   EmailSubscriberType,
				Target: "a@domain.invalid",
			},
		},
		{
			ID:           mgobson.NewObjectId().Hex(),
			Owner:        "not-my-project",
			OwnerType:    OwnerTypeProject,
			ResourceType: ResourceTypePatch,
			Trigger:      TriggerOutcome,
			Selectors: []Selector{
				{
					Type: SelectorProject,
					Data: "not-my-project",
				},
			},
			Filter: Filter{Project: "not-my-project"},
			Subscriber: Subscriber{
				Type:   EmailSubscriberType,
				Target: "a@domain.invalid",
			},
		},
	}
	for _, sub := range subs {
		require.NoError(t, sub.Upsert(t.Context()))
	}

	for name, test := range map[string]func(t *testing.T){
		"FromNonExistentProject": func(t *testing.T) {
			assert.NoError(t, CopyProjectSubscriptions(t.Context(), "not-a-project", "my-new-project"))
			apiSubs, err := FindSubscriptionsByOwner(t.Context(), "my-new-project", OwnerTypeProject)
			assert.NoError(t, err)
			require.Empty(t, apiSubs)
		},
		"FromExistentProject": func(t *testing.T) {
			newProjectId := "my-newest-project"
			assert.NoError(t, CopyProjectSubscriptions(t.Context(), oldProjectId, newProjectId))
			apiSubs, err := FindSubscriptionsByOwner(t.Context(), oldProjectId, OwnerTypeProject)
			assert.NoError(t, err)
			require.Len(t, apiSubs, 1)
			assert.Equal(t, subs[0].ID, apiSubs[0].ID)
			require.Len(t, apiSubs[0].Selectors, 1)
			assert.Equal(t, oldProjectId, apiSubs[0].Selectors[0].Data)
			assert.Equal(t, oldProjectId, apiSubs[0].Filter.Project)

			apiSubs, err = FindSubscriptionsByOwner(t.Context(), newProjectId, OwnerTypeProject)
			assert.NoError(t, err)
			require.Len(t, apiSubs, 1)
			assert.NotEqual(t, subs[0].ID, apiSubs[0].ID)
			require.Len(t, apiSubs[0].Selectors, 1)
			assert.Equal(t, newProjectId, apiSubs[0].Selectors[0].Data)
			assert.Equal(t, newProjectId, apiSubs[0].Filter.Project)
		},
	} {
		t.Run(name, func(t *testing.T) {
			test(t)
		})
	}

}
