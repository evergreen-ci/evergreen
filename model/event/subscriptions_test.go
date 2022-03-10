package event

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/stretchr/testify/suite"
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
			ID:           mgobson.NewObjectId().Hex(),
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
			ID:           mgobson.NewObjectId().Hex(),
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
			ID:           mgobson.NewObjectId().Hex(),
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
				ID:      "something",
				Project: "else$",
			},
			Subscriber: Subscriber{
				Type:   EmailSubscriberType,
				Target: &t3,
			},
			Owner: "someone",
		},
		{
			ID:           "5949645c9acd9604fdd202d8",
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
			ID:           mgobson.NewObjectId().Hex(),
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

func (s *subscriptionsSuite) TestFindSubscriptions() {
	s.Run("EmptySelectors", func() {
		subs, err := FindSubscriptions("type2", nil)
		s.NoError(err)
		s.Empty(subs)
	})

	s.Run("NothingMatches", func() {
		subs, err := FindSubscriptions("type1", map[string][]string{
			SelectorObject: {"nothing_matches"},
		})
		s.NoError(err)
		s.Empty(subs)
	})

	s.Run("MatchesMultipleSubscriptions", func() {
		subs, err := FindSubscriptions("type2", map[string][]string{
			SelectorObject: {"somethingspecial"},
		})
		s.NoError(err)
		s.Require().Len(subs, 2)
		for i := range subs {
			if subs[i].Subscriber.Type == EmailSubscriberType {
				s.Equal("someone4@example.com", *subs[i].Subscriber.Target.(*string))

			} else if subs[i].Subscriber.Type == SlackSubscriberType {
				s.Equal("slack_user", *subs[i].Subscriber.Target.(*string))

			} else {
				s.T().Errorf("unknown subscriber type: %s", subs[i].Subscriber.Type)
			}
		}
	})

	s.Run("MatchesRegexSelector", func() {
		// this query hits a subscriber with a regex selector
		subs, err := FindSubscriptions("type1", map[string][]string{
			SelectorID:      {"something"},
			SelectorProject: {"somethingelse"},
		})
		s.NoError(err)
		s.Len(subs, 3)
	})
}

func (s *subscriptionsSuite) TestRegexSelectorsMatch() {
	selectors := map[string][]string{
		"type":  {"something"},
		"type2": {"somethingelse"},
	}

	a := Subscription{
		RegexSelectors: []Selector{
			{
				Type: "type",
				Data: "^some",
			},
			{
				Type: "type2",
				Data: "else$",
			},
		},
	}

	s.True(regexSelectorsMatch(selectors, a.RegexSelectors))

	a.RegexSelectors[0].Data = "^S"
	s.False(regexSelectorsMatch(selectors, a.RegexSelectors))
}

func (s *subscriptionsSuite) TestFilterFromSelectors() {
	for _, sub := range s.subscriptions {
		filter := Filter{}
		s.Require().NoError(filter.FromSelectors(append(sub.Selectors, sub.RegexSelectors...)))
		s.Equal(sub.Filter, filter)
	}

	filter := Filter{}
	s.Error(filter.FromSelectors([]Selector{{Type: "non-existent-type"}}))
}

func (s *subscriptionsSuite) TestValidateFilter() {
	for _, sub := range s.subscriptions {
		s.NoError(sub.ValidateFilter())
	}

	noFilterParams := Subscription{}
	s.Error(noFilterParams.ValidateFilter())
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

func (s *subscriptionsSuite) TestCreateOrUpdateImplicitSubscription() {
	subscriber := Subscriber{
		Type:   SlackSubscriberType,
		Target: "@octocat",
	}

	subscription, err := CreateOrUpdateImplicitSubscription(ImplicitSubscriptionCommitQueue, "",
		subscriber, "octocat")
	s.NoError(err)

	subscriptions, err := FindSubscriptionsByOwner("octocat", OwnerTypePerson)
	s.NoError(err)
	s.Require().Len(subscriptions, 1)
	s.Equal(subscriptions[0].ID, subscription.ID)
}
