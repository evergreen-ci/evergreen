package migrations

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	evgdb "github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/db"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
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

	require.NoError(s.env.Configure(ctx, filepath.Join(evergreen.FindEvergreenHome(), testutil.TestDir, testutil.TestSettings), nil))
	require.NoError(s.env.LocalQueue().Start(ctx))

	anser.ResetEnvironment()
	require.NoError(anser.GetEnvironment().Setup(s.env.LocalQueue(), s.session))
	anser.GetEnvironment().RegisterCloser(func() error { cancel(); return nil })

	suite.Run(t, s)
}

func (s *adminEventSuite) SetupTest() {
	s.NoError(evgdb.ClearCollections(eventCollection))
	s.now = time.Now().Round(time.Millisecond).Truncate(time.Millisecond)

	data := db.Document{
		"r_id":       "",
		"ts":         s.now,
		eventTypeKey: eventTypeTheme,
		"data": db.Document{
			"r_type":  adminDataType,
			"user":    "me",
			"old_val": "old theme",
			"new_val": "new theme",
		},
	}
	s.NoError(evgdb.Insert(eventCollection, data))
	data = db.Document{
		"r_id":       "",
		"ts":         s.now,
		eventTypeKey: eventTypeBanner,
		"data": db.Document{
			"r_type":  adminDataType,
			"user":    "me",
			"old_val": "old banner",
			"new_val": "new banner",
		},
	}
	s.NoError(evgdb.Insert(eventCollection, data))
	data = db.Document{
		"r_id":       "",
		"ts":         s.now,
		eventTypeKey: eventTypeServiceFlags,
		"data": db.Document{
			"r_type": adminDataType,
			"user":   "me",
			"old_flags": db.Document{
				"task_dispatch_disabled": false,
			},
			"new_flags": db.Document{
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

	var events []db.Document
	err = evgdb.FindAllQ(eventCollection, evgdb.Q{}, &events)
	s.NoError(err)
	foundThemeChange := false
	foundBannerChange := false
	foundServiceFlagChange := false
	s.Len(events, 3)
	for _, evt := range events {
		s.EqualValues(s.now, evt["ts"])
		s.EqualValues("", evt["r_id"])
		s.EqualValues(eventTypeValueChanged, evt[eventTypeKey])
		data := evt["data"].(db.Document)
		s.NotEmpty(data["guid"])
		s.EqualValues("me", data["user"])
		s.NotContains(data, "old_val")
		s.NotContains(data, "new_val")
		s.NotContains(data, "old_flags")
		s.NotContains(data, "new_flags")
		changes := data["changes"].(db.Document)
		after := changes["after"].(db.Document)
		before := changes["before"].(db.Document)
		switch data["section"] {
		case "global":
			if before["banner_theme"] == "old theme" && after["banner_theme"] == "new theme" {
				foundThemeChange = true
			}
			if before["banner"] == "old banner" && after["banner"] == "new banner" {
				foundBannerChange = true
			}
		case "service_flags":
			if before["task_dispatch_disabled"] == false && after["task_dispatch_disabled"] == true {
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
