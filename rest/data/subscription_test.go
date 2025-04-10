package data

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetSubscriptions(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(event.SubscriptionsCollection))

	subs := []event.Subscription{
		{
			ID:           mgobson.NewObjectId().Hex(),
			Owner:        "someone",
			OwnerType:    event.OwnerTypePerson,
			ResourceType: event.ResourceTypePatch,
			Trigger:      "outcome",
			Selectors: []event.Selector{
				{
					Type: "id",
					Data: "1234",
				},
			},
			Subscriber: event.Subscriber{
				Type:   event.EmailSubscriberType,
				Target: "a@domain.invalid",
			},
		},
		{
			ID:           mgobson.NewObjectId().Hex(),
			Owner:        "someoneelse",
			OwnerType:    event.OwnerTypePerson,
			ResourceType: event.ResourceTypePatch,
			Trigger:      "outcomeelse",
			Selectors: []event.Selector{
				{
					Type: "id",
					Data: "1234",
				},
			},
			Subscriber: event.Subscriber{
				Type:   event.EmailSubscriberType,
				Target: "a@domain.invalid",
			},
		},
	}

	for i := range subs {
		assert.NoError(subs[i].Upsert(t.Context()))
	}

	apiSubs, err := GetSubscriptions("someone", event.OwnerTypePerson)
	assert.NoError(err)
	assert.Len(apiSubs, 1)

	apiSubs, err = GetSubscriptions("someoneelse", event.OwnerTypePerson)
	assert.NoError(err)
	assert.Len(apiSubs, 1)

	apiSubs, err = GetSubscriptions("who", event.OwnerTypePerson)
	assert.NoError(err)
	assert.Empty(apiSubs)

	apiSubs, err = GetSubscriptions("", event.OwnerTypePerson)
	assert.EqualError(err, "400 (Bad Request): no subscription owner provided")
	assert.Empty(apiSubs)
}

func TestConvertVersionSubscription(t *testing.T) {
	for name, test := range map[string]func(t *testing.T){
		"ProjectSubscription": func(t *testing.T) {
			subscription := event.Subscription{
				ResourceType: event.ResourceTypeVersion,
				Trigger:      event.TriggerFailure,
				Owner:        "project",
				OwnerType:    event.OwnerTypeProject,
				Selectors: []event.Selector{
					{
						Type: event.SelectorRequester,
						Data: evergreen.PatchVersionRequester,
					},
				},
				Subscriber: event.Subscriber{
					Type:   event.EmailSubscriberType,
					Target: "a@domain.invalid",
				},
			}
			assert.NoError(t, convertVersionSubscription(t.Context(), &subscription))
			assert.Equal(t, event.TriggerFamilyFailure, subscription.Trigger)
		},
		"PersonalSubscription": func(t *testing.T) {
			subscription := event.Subscription{
				ResourceType: event.ResourceTypeVersion,
				Trigger:      event.TriggerFailure,
				Owner:        "me",
				OwnerType:    event.OwnerTypePerson,
				Selectors: []event.Selector{
					{
						Type: event.SelectorObject,
						Data: event.ObjectVersion,
					},
					{
						Type: event.SelectorID,
						Data: "version_id",
					},
				},
				Subscriber: event.Subscriber{
					Type:   event.EmailSubscriberType,
					Target: "a@domain.invalid",
				},
			}
			assert.NoError(t, convertVersionSubscription(t.Context(), &subscription))
			assert.Equal(t, event.TriggerFamilyFailure, subscription.Trigger)
		},
		"PersonalSubscriptionVersionNotFound": func(t *testing.T) {
			subscription := event.Subscription{
				ResourceType: event.ResourceTypeVersion,
				Trigger:      event.TriggerFailure,
				Owner:        "me",
				OwnerType:    event.OwnerTypePerson,
				Selectors: []event.Selector{
					{
						Type: event.SelectorObject,
						Data: event.ObjectVersion,
					},
					{
						Type: event.SelectorID,
						Data: "version_1",
					},
				},
				Subscriber: event.Subscriber{
					Type:   event.EmailSubscriberType,
					Target: "a@domain.invalid",
				},
			}
			assert.Error(t, convertVersionSubscription(t.Context(), &subscription))
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.NoError(t, db.ClearCollections(event.SubscriptionsCollection, model.VersionCollection))

			v := &model.Version{
				Id:        "version_id",
				Requester: evergreen.GithubPRRequester,
			}
			require.NoError(t, v.Insert())

			test(t)
		})
	}
}

func TestSaveProjectSubscriptions(t *testing.T) {
	for name, test := range map[string]func(t *testing.T){
		"InvalidSubscription": func(t *testing.T) {
			subscription := restModel.APISubscription{
				ResourceType: utility.ToStringPtr(event.ResourceTypeTask),
				Trigger:      utility.ToStringPtr(event.TriggerOutcome),
				Owner:        utility.ToStringPtr("project"),
				OwnerType:    utility.ToStringPtr(string(event.OwnerTypeProject)),
				Selectors: []restModel.APISelector{
					{
						Type: utility.ToStringPtr(event.SelectorObject),
						Data: utility.ToStringPtr(""),
					},
				},
				Subscriber: restModel.APISubscriber{
					Type:   utility.ToStringPtr(event.EmailSubscriberType),
					Target: "a@domain.invalid",
				},
			}
			assert.Error(t, SaveSubscriptions(t.Context(), utility.FromStringPtr(subscription.Owner), []restModel.APISubscription{subscription}, false))
		},
		"VersionRequesterSubscription": func(t *testing.T) {
			subscription := restModel.APISubscription{
				ResourceType: utility.ToStringPtr(event.ResourceTypeVersion),
				Trigger:      utility.ToStringPtr(event.TriggerOutcome),
				Owner:        utility.ToStringPtr("project"),
				OwnerType:    utility.ToStringPtr(string(event.OwnerTypeProject)),
				Selectors: []restModel.APISelector{
					{
						Type: utility.ToStringPtr(event.SelectorRequester),
						Data: utility.ToStringPtr(evergreen.AdHocRequester),
					},
				},
				Subscriber: restModel.APISubscriber{
					Type:   utility.ToStringPtr(event.EmailSubscriberType),
					Target: "a@domain.invalid",
				},
			}
			assert.NoError(t, SaveSubscriptions(t.Context(),
				utility.FromStringPtr(subscription.Owner),
				[]restModel.APISubscription{subscription},
				false))

			dbSubs, err := GetSubscriptions(utility.FromStringPtr(subscription.Owner), event.OwnerTypeProject)
			assert.NoError(t, err)
			require.Len(t, dbSubs, 1)
			require.Equal(t, event.TriggerOutcome, utility.FromStringPtr(dbSubs[0].Trigger))
		},
		"PatchRequesterSubscription": func(t *testing.T) {
			subscription := restModel.APISubscription{
				ResourceType: utility.ToStringPtr(event.ResourceTypeVersion),
				Trigger:      utility.ToStringPtr(event.TriggerOutcome),
				Owner:        utility.ToStringPtr("project"),
				OwnerType:    utility.ToStringPtr(string(event.OwnerTypeProject)),
				Selectors: []restModel.APISelector{
					{
						Type: utility.ToStringPtr(event.SelectorRequester),
						Data: utility.ToStringPtr(evergreen.GithubPRRequester),
					},
				},
				Subscriber: restModel.APISubscriber{
					Type:   utility.ToStringPtr(event.EmailSubscriberType),
					Target: "a@domain.invalid",
				},
			}
			assert.NoError(t, SaveSubscriptions(t.Context(),
				utility.FromStringPtr(subscription.Owner),
				[]restModel.APISubscription{subscription},
				false))

			dbSubs, err := GetSubscriptions(utility.FromStringPtr(subscription.Owner), event.OwnerTypeProject)
			assert.NoError(t, err)
			require.Len(t, dbSubs, 1)
			require.Equal(t, event.TriggerFamilyOutcome, utility.FromStringPtr(dbSubs[0].Trigger))
		},
		"ModifyExistingSubscription": func(t *testing.T) {
			subscription := restModel.APISubscription{
				ID:           utility.ToStringPtr("existing_subscription"),
				ResourceType: utility.ToStringPtr(event.ResourceTypeTask),
				Trigger:      utility.ToStringPtr(event.TriggerOutcome),
				Owner:        utility.ToStringPtr("existing_subscription_project"),
				OwnerType:    utility.ToStringPtr(string(event.OwnerTypeProject)),
				Selectors: []restModel.APISelector{
					{
						Type: utility.ToStringPtr(event.SelectorObject),
						Data: utility.ToStringPtr(event.ObjectTask),
					},
					{
						Type: utility.ToStringPtr(event.SelectorID),
						Data: utility.ToStringPtr("task-1"),
					},
				},
				Subscriber: restModel.APISubscriber{
					Type:   utility.ToStringPtr(event.EmailSubscriberType),
					Target: "a@domain.invalid",
				},
			}
			newData := utility.ToStringPtr("5678")
			subscription.Selectors[0].Data = newData
			assert.NoError(t, SaveSubscriptions(t.Context(), utility.FromStringPtr(subscription.Owner), []restModel.APISubscription{subscription}, true))

			dbSubs, err := GetSubscriptions(utility.FromStringPtr(subscription.Owner), event.OwnerTypeProject)
			assert.NoError(t, err)
			require.Len(t, dbSubs, 1)
			require.Equal(t, dbSubs[0].Selectors[0].Data, newData)
		},
		"DisallowedSubscription": func(t *testing.T) {
			subscription := restModel.APISubscription{
				ResourceType: utility.ToStringPtr(event.ResourceTypeTask),
				Trigger:      utility.ToStringPtr(event.TriggerOutcome),
				Owner:        utility.ToStringPtr("project"),
				OwnerType:    utility.ToStringPtr(string(event.OwnerTypeProject)),
				Selectors:    []restModel.APISelector{},
				Subscriber: restModel.APISubscriber{
					Type:   utility.ToStringPtr(event.JIRACommentSubscriberType),
					Target: "ticket",
				},
			}
			assert.Error(t, SaveSubscriptions(t.Context(), utility.FromStringPtr(subscription.Owner), []restModel.APISubscription{subscription}, false))
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.NoError(t, db.ClearCollections(event.SubscriptionsCollection))
			projectSubscription := event.Subscription{
				ID:           "existing_subscription",
				Owner:        "existing_subscription_project",
				OwnerType:    event.OwnerTypeProject,
				ResourceType: event.ResourceTypeTask,
				Trigger:      event.TriggerOutcome,
				Selectors: []event.Selector{
					{
						Type: event.SelectorObject,
						Data: event.ObjectTask,
					},
					{
						Type: event.SelectorID,
						Data: "task-1",
					},
				},
				Subscriber: event.Subscriber{
					Type:   event.EmailSubscriberType,
					Target: "a@domain.invalid",
				},
			}
			assert.NoError(t, projectSubscription.Upsert(t.Context()))
			test(t)
		})
	}
}

func TestSaveTaskSubscriptions(t *testing.T) {
	for name, test := range map[string]func(t *testing.T){
		"ValidSubscription": func(t *testing.T) {
			subscription := restModel.APISubscription{
				ResourceType: utility.ToStringPtr(event.ResourceTypeTask),
				Trigger:      utility.ToStringPtr(event.TriggerOutcome),
				Owner:        utility.ToStringPtr("me"),
				OwnerType:    utility.ToStringPtr(string(event.OwnerTypePerson)),
				Selectors: []restModel.APISelector{
					{
						Type: utility.ToStringPtr(event.SelectorObject),
						Data: utility.ToStringPtr(event.ObjectTask),
					},
					{
						Type: utility.ToStringPtr(event.SelectorID),
						Data: utility.ToStringPtr("task-1"),
					},
				},
				Subscriber: restModel.APISubscriber{
					Type:   utility.ToStringPtr(event.EmailSubscriberType),
					Target: "a@domain.invalid",
				},
			}
			assert.NoError(t, SaveSubscriptions(t.Context(), "me", []restModel.APISubscription{subscription}, false))
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.NoError(t, db.ClearCollections(event.SubscriptionsCollection))
			test(t)
		})
	}
}

func TestSaveVersionSubscriptions(t *testing.T) {
	for name, test := range map[string]func(t *testing.T){
		"VersionRequester": func(t *testing.T) {
			subscription := restModel.APISubscription{
				ResourceType: utility.ToStringPtr(event.ResourceTypeVersion),
				Trigger:      utility.ToStringPtr(event.TriggerOutcome),
				Owner:        utility.ToStringPtr("me"),
				OwnerType:    utility.ToStringPtr(string(event.OwnerTypePerson)),
				Selectors: []restModel.APISelector{
					{
						Type: utility.ToStringPtr(event.SelectorObject),
						Data: utility.ToStringPtr(event.ObjectVersion),
					},
					{
						Type: utility.ToStringPtr(event.SelectorID),
						Data: utility.ToStringPtr("version-1"),
					},
				},
				Subscriber: restModel.APISubscriber{
					Type:   utility.ToStringPtr(event.EmailSubscriberType),
					Target: "a@domain.invalid",
				},
			}
			assert.NoError(t, SaveSubscriptions(t.Context(), "me", []restModel.APISubscription{subscription}, false))

			dbSubs, err := GetSubscriptions("me", event.OwnerTypePerson)
			assert.NoError(t, err)
			require.Len(t, dbSubs, 1)
			require.Equal(t, event.TriggerOutcome, utility.FromStringPtr(dbSubs[0].Trigger))
		},
		"PatchRequester": func(t *testing.T) {
			subscription := restModel.APISubscription{
				ResourceType: utility.ToStringPtr(event.ResourceTypeVersion),
				Trigger:      utility.ToStringPtr(event.TriggerOutcome),
				Owner:        utility.ToStringPtr("me"),
				OwnerType:    utility.ToStringPtr(string(event.OwnerTypePerson)),
				Selectors: []restModel.APISelector{
					{
						Type: utility.ToStringPtr(event.SelectorObject),
						Data: utility.ToStringPtr(event.ObjectVersion),
					},
					{
						Type: utility.ToStringPtr(event.SelectorID),
						Data: utility.ToStringPtr("version-2"),
					},
				},
				Subscriber: restModel.APISubscriber{
					Type:   utility.ToStringPtr(event.EmailSubscriberType),
					Target: "a@domain.invalid",
				},
			}
			assert.NoError(t, SaveSubscriptions(t.Context(), "me", []restModel.APISubscription{subscription}, false))

			dbSubs, err := GetSubscriptions("me", event.OwnerTypePerson)
			assert.NoError(t, err)
			require.Len(t, dbSubs, 1)
			require.Equal(t, event.TriggerFamilyOutcome, utility.FromStringPtr(dbSubs[0].Trigger))
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.NoError(t, db.ClearCollections(event.SubscriptionsCollection, model.VersionCollection))

			v1 := &model.Version{
				Id:        "version-1",
				Requester: evergreen.AdHocRequester,
			}
			require.NoError(t, v1.Insert())

			v2 := &model.Version{
				Id:        "version-2",
				Requester: evergreen.PatchVersionRequester,
			}
			require.NoError(t, v2.Insert())

			test(t)
		})
	}
}

func TestDeleteProjectSubscriptions(t *testing.T) {
	for name, test := range map[string]func(t *testing.T, ids []string){
		"InvalidOwner": func(t *testing.T, ids []string) {
			assert.Error(t, DeleteSubscriptions(t.Context(), "my-project", ids))
		},
		"ValidOwner": func(t *testing.T, ids []string) {
			assert.NoError(t, DeleteSubscriptions(t.Context(), "my-project", []string{ids[0]}))
			subs, err := event.FindSubscriptionsByOwner("my-project", event.OwnerTypeProject)
			assert.NoError(t, err)
			assert.Empty(t, subs)
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.NoError(t, db.ClearCollections(event.SubscriptionsCollection))
			subs := []event.Subscription{
				{
					ID:           mgobson.NewObjectId().Hex(),
					Owner:        "my-project",
					OwnerType:    event.OwnerTypeProject,
					ResourceType: event.ResourceTypePatch,
					Trigger:      "outcome",
					Selectors: []event.Selector{
						{
							Type: "id",
							Data: "1234",
						},
					},
					Subscriber: event.Subscriber{
						Type:   event.EmailSubscriberType,
						Target: "a@domain.invalid",
					},
				},
				{
					ID:           mgobson.NewObjectId().Hex(),
					Owner:        "not-my-project",
					OwnerType:    event.OwnerTypeProject,
					ResourceType: event.ResourceTypePatch,
					Trigger:      "outcome",
					Selectors: []event.Selector{
						{
							Type: "id",
							Data: "1234",
						},
					},
					Subscriber: event.Subscriber{
						Type:   event.EmailSubscriberType,
						Target: "a@domain.invalid",
					},
				},
			}
			toDelete := []string{}
			for _, sub := range subs {
				assert.NoError(t, sub.Upsert(t.Context()))
				toDelete = append(toDelete, sub.ID)
			}
			test(t, toDelete)
		})
	}
}
