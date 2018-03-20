package event

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

func TestSubscriptions(t *testing.T) {
	suite.Run(t, &subscriptionsSuite{})
}

type subscriptionsSuite struct {
	suite.Suite
	subscriptions []Subscription
}

func (s *subscriptionsSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func (s *subscriptionsSuite) SetupTest() {
	s.NoError(db.ClearCollections(SubscriptionsCollection))

	t1 := "someone@example.com"
	t2 := "someone2@example.com"
	t3 := "someone3@example.com"
	t4 := "someone4@example.com"
	s.subscriptions = []Subscription{
		{
			ID:      bson.NewObjectId(),
			Type:    "type1",
			Trigger: "trigger1",
			Selectors: []Selector{
				{
					Type: "data1",
					Data: "something",
				},
			},
			RegexSelectors: []Selector{},
			Subscriber: Subscriber{
				Type:   "email",
				Target: &t1,
			},
		},
		{
			ID:      bson.NewObjectId(),
			Type:    "type1",
			Trigger: "trigger1",
			Selectors: []Selector{
				{
					Type: "data2",
					Data: "somethingelse",
				},
			},
			RegexSelectors: []Selector{},
			Subscriber: Subscriber{
				Type:   "email",
				Target: &t2,
			},
		},
		{
			ID:      bson.NewObjectId(),
			Type:    "type1",
			Trigger: "trigger1",
			Selectors: []Selector{
				{
					Type: "data1",
					Data: "something",
				},
			},
			RegexSelectors: []Selector{
				{
					Type: "data2",
					Data: "else$",
				},
			},
			Subscriber: Subscriber{
				Type:   "email",
				Target: &t3,
			},
		},
		{
			ID:      bson.NewObjectId(),
			Type:    "type2",
			Trigger: "trigger2",
			Selectors: []Selector{
				{
					Type: "data",
					Data: "somethingspecial",
				},
			},
			RegexSelectors: []Selector{},
			Subscriber: Subscriber{
				Type:   "email",
				Target: &t4,
			},
		},
	}

	for _, sub := range s.subscriptions {
		s.NoError(sub.Upsert())
	}
}

func (s *subscriptionsSuite) TestUpsert() {
	out := []Subscription{}
	s.NoError(db.FindAllQ(SubscriptionsCollection, db.Q{}, &out))

	s.Require().Len(out, 4)

	for _, sub := range out {
		if sub.ID == s.subscriptions[3].ID {
			s.Equal(sub, s.subscriptions[3])
		}
	}
}

func (s *subscriptionsSuite) TestRemove() {
	for i := range s.subscriptions {
		s.NoError(s.subscriptions[i].Remove())

		out := []Subscription{}
		s.NoError(db.FindAllQ(SubscriptionsCollection, db.Q{}, &out))
		s.Len(out, len(s.subscriptions)-i-1)
	}
}

func (s *subscriptionsSuite) TestFind() {
	// Empty selectors should select nothing
	subs, err := FindSubscribers("type2", "trigger2", nil)
	s.NoError(err)
	s.Empty(subs)

	subs, err = FindSubscribers("type2", "trigger2", []Selector{
		{
			Type: "data",
			Data: "somethingspecial",
		},
	})
	s.NoError(err)
	s.Len(subs, 1)
	s.NotPanics(func() {
		s.Len(subs[0].Subscribers, 1)
		s.Equal("email", subs[0].Subscribers[0].Subscriber.Type)
		s.Equal("someone4@example.com", *subs[0].Subscribers[0].Subscriber.Target.(*string))
	})

	// regex selector
	subs, err = FindSubscribers("type1", "trigger1", []Selector{
		{
			Type: "data1",
			Data: "something",
		},
		{
			Type: "data2",
			Data: "somethingelse",
		},
	})
	s.NoError(err)
	s.Len(subs, 1)
	s.NotPanics(func() {
		s.Len(subs[0].Subscribers, 3)
	})
}

func (s *subscriptionsSuite) TestFindSelectors() {
	selectors := []Selector{
		{
			Type: "type",
			Data: "something",
		},
		{
			Type: "type2",
			Data: "something",
		},
	}

	s.Equal(&selectors[0], findSelector(selectors, "type"))
	s.Nil(findSelector(selectors, "nope"))
}

func (s *subscriptionsSuite) TestRegexSelectorsMatch() {
	selectors := []Selector{
		{
			Type: "type",
			Data: "something",
		},
		{
			Type: "type2",
			Data: "somethingelse",
		},
	}

	a := SubscriberWithRegex{
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

	s.True(regexSelectorsMatch(selectors, &a))

	a.RegexSelectors[0].Data = "^S"
	s.False(regexSelectorsMatch(selectors, &a))
}
