package trigger

import (
	"context"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/suite"
)

func init() { testutil.Setup() }

func TestCommitQueueTriggers(t *testing.T) {
	suite.Run(t, &commitQueueSuite{})
}

type commitQueueSuite struct {
	event  event.EventLogEntry
	data   *event.CommitQueueEventData
	subs   []event.Subscription
	ctx    context.Context
	cancel context.CancelFunc

	t *commitQueueTriggers

	suite.Suite
}

func (s *commitQueueSuite) SetupSuite() {
	s.Require().Implements((*eventHandler)(nil), &commitQueueTriggers{})
}

func (s *commitQueueSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())

	s.NoError(db.ClearCollections(event.EventCollection, patch.Collection, event.SubscriptionsCollection, event.SubscriptionsCollection, model.ProjectRefCollection))

	s.data = &event.CommitQueueEventData{
		Status: evergreen.TaskStarted,
	}
	s.event = event.EventLogEntry{
		ID:           "event1234",
		ResourceType: event.ResourceTypeTask,
		EventType:    event.TaskFinished,
		ResourceId:   "test",
		Data:         s.data,
	}
	proj := model.ProjectRef{
		Id: "proj",
	}
	s.NoError(proj.Insert())
	p := patch.Patch{
		Id:          patch.NewId("aaaaaaaaaaff001122334455"),
		Description: "Testing 'quote' escape",
		Project:     "proj",
	}
	s.NoError(p.Insert())

	s.subs = []event.Subscription{
		{
			ID: mgobson.NewObjectId().Hex(),
			Subscriber: event.Subscriber{
				Type: event.EmailSubscriberType,
			},
			Trigger: "test",
		},
	}

	for i := range s.subs {
		s.NoError(s.subs[i].Upsert())
	}

	s.t = makeCommitQueueTriggers().(*commitQueueTriggers)
	s.t.event = &s.event
	s.t.data = s.data
	s.t.patch = &p
}

func (s *commitQueueSuite) TearDownTest() {
	s.cancel()
}

func (s *commitQueueSuite) TestEmailUnescapesDescription() {
	n, err := s.t.commitQueueOutcome(s.ctx, &s.subs[0])
	s.NoError(err)
	s.NotNil(n)
	payload, ok := n.Payload.(*message.Email)
	s.True(ok)
	s.True(strings.Contains(payload.Body, "'quote'"))
}
