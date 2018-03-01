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
}

func (s *subscriptionsSuite) SetupSuite() {
	s.Require().Equal(subAggregationRegexSelectorsKey, subscriptionRegexSelectorsKey)
	s.Require().Equal(subAggregationSubscriberKey, subscriptionSubscriberKey)

	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func (s *subscriptionsSuite) SetupTest() {
	s.NoError(db.ClearCollections(SubscriptionsCollection))

	subscriptions := []Subscription{
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
			Subscriber: Subscriber{
				Type:   "email",
				Target: "someone@example.com",
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
			Subscriber: Subscriber{
				Type:   "email",
				Target: "someone1@example.com",
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
				Target: "someone2@example.com",
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
			Subscriber: Subscriber{
				Type:   "email",
				Target: "someone3@example.com",
			},
		},
	}

	for _, sub := range subscriptions {
		s.NoError(db.InsertMany(SubscriptionsCollection, sub))
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
	s.Require().Len(subs, 1)
	s.Equal("email", subs[0].Type)
	s.Equal("someone3@example.com", subs[0].Target.(string))

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
	s.Len(subs, 3)
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

	a := subscriptionAggregation{
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
