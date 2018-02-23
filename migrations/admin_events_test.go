package migrations

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	evgdb "github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/db"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

type adminEventSuite struct {
	env      *mock.Environment
	session  db.Session
	database string
	now      time.Time
	cancel   func()
	suite.Suite
}

func TestAdminEventMigration(t *testing.T) {
	require := require.New(t)

	mgoSession, database, err := evgdb.GetGlobalSessionFactory().GetSession()
	require.NoError(err)
	defer mgoSession.Close()

	session := db.WrapSession(mgoSession.Copy())
	defer session.Close()

	ctx, cancel := context.WithCancel(context.Background())
	s := &adminEventSuite{
		env:      &mock.Environment{},
		session:  session,
		database: database.Name,
		cancel:   cancel,
	}

	require.NoError(s.env.Configure(ctx, filepath.Join(evergreen.FindEvergreenHome(), testutil.TestDir, testutil.TestSettings)))
	require.NoError(s.env.LocalQueue().Start(ctx))

	anser.ResetEnvironment()
	require.NoError(anser.GetEnvironment().Setup(s.env.LocalQueue(), s.session))
	anser.GetEnvironment().RegisterCloser(func() error { cancel(); return nil })

	suite.Run(t, s)
}

func (s *adminEventSuite) SetupTest() {
	s.NoError(evgdb.ClearCollections(eventCollection))
	s.now = time.Now()

	data := bson.M{
		"r_id":       "",
		"ts":         s.now,
		eventTypeKey: eventTypeTheme,
		"data": bson.M{
			"r_type":  adminDataType,
			"user":    "me",
			"old_val": "old theme",
			"new_val": "new theme",
		},
	}
	s.NoError(evgdb.Insert(eventCollection, data))
	data = bson.M{
		"r_id":       "",
		"ts":         s.now,
		eventTypeKey: eventTypeBanner,
		"data": bson.M{
			"r_type":  adminDataType,
			"user":    "me",
			"old_val": "old banner",
			"new_val": "new banner",
		},
	}
	s.NoError(evgdb.Insert(eventCollection, data))
	data = bson.M{
		"r_id":       "",
		"ts":         s.now,
		eventTypeKey: eventTypeServiceFlags,
		"data": bson.M{
			"r_type": adminDataType,
			"user":   "me",
			"old_flags": bson.M{
				"task_dispatch_disabled": false,
			},
			"new_flags": bson.M{
				"task_dispatch_disabled": true,
			},
		},
	}
	s.NoError(evgdb.Insert(eventCollection, data))
}

func (s *adminEventSuite) TestMigration() {
	gen, err := adminEventRestructureGenerator(anser.GetEnvironment(), s.database, 10)
	s.NoError(err)
	gen.Run()
	s.NoError(gen.Error())

	for j := range gen.Jobs() {
		j.Run()
		s.NoError(j.Error())
	}

	events, err := event.FindAdmin(event.RecentAdminEvents(10))
	s.NoError(err)
	foundThemeChange := false
	foundBannerChange := false
	foundServiceFlagChange := false
	for _, evt := range events {
		s.WithinDuration(s.now, evt.Timestamp, 1*time.Millisecond)
		s.Equal("", evt.ResourceId)
		s.Equal(event.EventTypeValueChanged, evt.EventType)
		data, ok := evt.Data.Data.(*event.AdminEventData)
		s.Require().True(ok)
		s.Equal("me", data.User)
		switch data.Section {
		case "global":
			after := data.Changes.After.(*evergreen.Settings)
			before := data.Changes.Before.(*evergreen.Settings)
			if before.BannerTheme == "old theme" && after.BannerTheme == "new theme" {
				foundThemeChange = true
			}
			if before.Banner == "old banner" && after.Banner == "new banner" {
				foundBannerChange = true
			}
		case "service_flags":
			after := data.Changes.After.(*evergreen.ServiceFlags)
			before := data.Changes.Before.(*evergreen.ServiceFlags)
			if before.TaskDispatchDisabled == false && after.TaskDispatchDisabled == true {
				foundServiceFlagChange = true
			}
		}
	}
	s.True(foundThemeChange)
	s.True(foundBannerChange)
	s.True(foundServiceFlagChange)
}

func (s *adminEventSuite) TearDownSuite() {
	s.cancel()
}
