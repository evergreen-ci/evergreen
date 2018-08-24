package migrations

import (
	"context"
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/mongodb/anser"
	"github.com/mongodb/anser/db"
	"github.com/stretchr/testify/suite"
)

func TestLegacyNotificationsMigration(t *testing.T) {
	suite.Run(t, &legacyNotificationsSuite{})
}

type legacyNotificationsSuite struct {
	migrationSuite
}

func (s *legacyNotificationsSuite) SetupTest() {
	const (
		projectRefCollection    = "project_ref"
		subscriptionsCollection = "subscriptions"
	)

	dbSess := s.session.DB(s.database)
	s.NotNil(dbSess)
	_, err := dbSess.C(subscriptionsCollection).RemoveAll(db.Document{})
	s.Require().NoError(err)

	c := dbSess.C(projectRefCollection)
	s.Require().NotNil(c)
	_, err = c.RemoveAll(db.Document{})
	s.Require().NoError(err)

	rawProjectRefs := []db.Document{
		{
			"_id":            "p1",
			"identifier":     "p1",
			"alert_settings": db.Document{},
		},
		{
			"_id":        "p2",
			"identifier": "p2",
			"alert_settings": db.Document{
				"something_invalid": []db.Document{
					{
						"provider": "email",
						"settings": db.Document{
							"recipient": "ci-spam-mongo-tools@10gen.com",
						},
					},
				},
			},
		},
		{
			"_id":            "p-empty",
			"identifier":     "p-empty",
			"alert_settings": db.Document{},
		},
		{
			"_id":        "p-notexist",
			"identifier": "p-notexist",
		},
		{
			"_id":        "mongodb-mongo-master",
			"identifier": "mongodb-mongo-master",
			"alert_settings": db.Document{
				"task_transition_failure": []db.Document{
					{
						"provider": "jira",
						"settings": db.Document{
							"project": "BFG",
							"issue":   "Build Failure",
						},
					},
				},
			},
		},
	}

	insertAt := rawProjectRefs[0]["alert_settings"].(db.Document)
	triggers := []string{"task_failed", "first_version_failure", "first_variant_failure", "first_tasktype_failure", "task_transition_failure"}
	for i, trigger := range triggers {
		insertAt[trigger] = []db.Document{
			{
				"provider": "email",
				"settings": db.Document{
					"recipient": fmt.Sprintf("%d@domain.invalid", i),
				},
			},
			{
				"provider": "jira",
				"settings": db.Document{
					"project": "BFG",
					"issue":   fmt.Sprintf("%d", i),
				},
			},
		}
	}

	for i := range rawProjectRefs {
		s.Require().NoError(c.Insert(&rawProjectRefs[i]))
	}
}

func (s *legacyNotificationsSuite) TestMigration() {
	const (
		projectRefCollection    = "project_ref"
		subscriptionsCollection = "subscriptions"
	)
	args := migrationGeneratorFactoryOptions{
		db:    s.database,
		limit: 50,
		id:    "legacy-notifications-to-subscriptions",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gen, err := legacyNotificationsToSubscriptionsGenerator(anser.GetEnvironment(), args)
	s.Require().NoError(err)
	gen.Run(ctx)
	s.Require().NoError(gen.Error())

	i := 0
	for j := range gen.Jobs() {
		i++
		j.Run(ctx)
		if j.ID() == "legacy-notifications-to-subscriptions.p2.1" {
			s.EqualError(j.Error(), "something_invalid, index 0: unknown trigger")
		} else {
			s.NoError(j.Error())
		}
	}
	s.Equal(4, i)

	subsC := s.session.DB(s.database).C(subscriptionsCollection)
	q := subsC.Find(db.Document{})
	out := []event.Subscription{}
	s.NoError(q.All(&out))
	s.Len(out, 11)

	senderType := map[string]int{}
	for i := range out {
		senderType[out[i].Subscriber.Type] += 1
		s.NoError(out[i].Validate())
	}
	s.Len(senderType, 2)
	s.Equal(senderType["jira-issue"], 6)
	s.Equal(senderType["email"], 5)

	// assert that all but 1 project ref had their alert_settings emptied
	projectsC := s.session.DB(s.database).C(projectRefCollection)
	q = projectsC.Find(db.Document{
		"alert_settings": db.Document{
			"$exists": true,
		},
	})
	projects := []db.Document{}
	s.NoError(q.All(&projects))
	s.Require().Len(projects, 1)

	// assert that project refs with invalid notifications configs are
	// left unscathed
	as := projects[0]["alert_settings"].(db.Document)
	s.Len(as["something_invalid"], 1)

	q = projectsC.Find(db.Document{})
	s.NoError(q.All(&projects))
	s.Len(projects, 5)

	// let's check that jira notifications look the way we think they should
	q = subsC.Find(db.Document{
		"owner_type": "project",
		"owner":      "mongodb-mongo-master",
	})
	out = []event.Subscription{}
	s.NoError(q.All(&out))
	s.Require().Len(out, 1)

	s.Equal("TASK", out[0].ResourceType)
	s.Equal("project", string(out[0].OwnerType))
	s.Equal("mongodb-mongo-master", string(out[0].Owner))
	s.Equal("regression", out[0].Trigger)
	s.Equal("jira-issue", out[0].Subscriber.Type)
	target := out[0].Subscriber.Target.(*event.JIRAIssueSubscriber)
	s.Equal("Build Failure", target.IssueType)
	s.Equal("BFG", target.Project)
	s.Require().Len(out[0].Selectors, 2)
	s.Equal("project", string(out[0].Selectors[0].Type))
	s.Equal("mongodb-mongo-master", out[0].Selectors[0].Data)
	s.Equal("requester", out[0].Selectors[1].Type)
	s.Equal("gitter_request", out[0].Selectors[1].Data)
}
