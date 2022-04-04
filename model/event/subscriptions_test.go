package event

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
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
	s.NoError(db.ClearCollections(SubscriptionsCollection))

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
		s.NoError(sub.Upsert())
	}
}

func (s *subscriptionsSuite) TestUpsert() {
	out := []Subscription{}
	s.NoError(db.FindAllQ(SubscriptionsCollection, db.Q{}, &out))

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
		s.NoError(RemoveSubscription(s.subscriptions[i].ID))

		out := []Subscription{}
		s.NoError(db.FindAllQ(SubscriptionsCollection, db.Q{}, &out))
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
		subs, err := FindSubscriptionsByAttributes("type2", Attributes{})
		s.NoError(err)
		s.Empty(subs)
	})

	s.Run("NothingMatches", func() {
		subs, err := FindSubscriptionsByAttributes("type1", Attributes{
			Object: []string{"nothing_matches"},
		})
		s.NoError(err)
		s.Empty(subs)
	})

	s.Run("MatchesMultipleSubscriptions", func() {
		subs, err := FindSubscriptionsByAttributes("type2", Attributes{
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
		subs, err := FindSubscriptionsByAttributes("type1", Attributes{
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
	subscriptions, err := FindSubscriptionsByOwner("me", OwnerTypePerson)
	s.NoError(err)
	s.Len(subscriptions, 2)
	for _, sub := range subscriptions {
		s.Equal("me", sub.Owner)
		s.EqualValues(OwnerTypePerson, sub.OwnerType)
	}
}

func (s *subscriptionsSuite) TestFindByOwnerForProject() {
	subscriptions, err := FindSubscriptionsByOwner("me", OwnerTypeProject)
	s.NoError(err)
	s.Require().Len(subscriptions, 1)
	s.Equal("me", subscriptions[0].Owner)
	s.EqualValues(OwnerTypeProject, subscriptions[0].OwnerType)
}

func (s *subscriptionsSuite) TestFindSubscriptionsByOwner() {
	for i := range s.subscriptions {
		sub, err := FindSubscriptionByID(s.subscriptions[i].ID)
		s.NoError(err)
		s.NotNil(sub)
		s.NotEqual("", sub.ID)
	}

	s.NoError(db.ClearCollections(SubscriptionsCollection))
	sub, err := FindSubscriptionByID(s.subscriptions[0].ID)
	s.NoError(err)
	s.Nil(sub)
}

func (s *subscriptionsSuite) TestCreateOrUpdateGeneralSubscription() {
	subscriber := Subscriber{
		Type:   SlackSubscriberType,
		Target: "@octocat",
	}

	subscription, err := CreateOrUpdateGeneralSubscription(GeneralSubscriptionCommitQueue, "",
		subscriber, "octocat")
	s.NoError(err)

	subscriptions, err := FindSubscriptionsByOwner("octocat", OwnerTypePerson)
	s.NoError(err)
	s.Require().Len(subscriptions, 1)
	s.Equal(subscriptions[0].ID, subscription.ID)
}
