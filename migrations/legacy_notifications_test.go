package migrations

import (
	"context"
	"fmt"
	"testing"

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
	const subscriptionsCollection = "subscriptions"
	args := migrationGeneratorFactoryOptions{
		db:    s.database,
		limit: 50,
		id:    "legacy-notifications-to-subscriptions",
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	gen, err := legacyNotificationsToSubscriptions(anser.GetEnvironment(), args)
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
	s.Equal(2, i)

	subsC := s.session.DB(s.database).C(subscriptionsCollection)
	q := subsC.Find(db.Document{})
	out := []subscription{}
	s.NoError(q.All(&out))
	s.Len(out, 10)

	senderType := map[string]int{}
	for i := range out {
		senderType[out[i].Subscriber.Type] += 1
	}
	s.Len(senderType, 2)
	s.Equal(senderType["jira-issue"], 5)
	s.Equal(senderType["email"], 5)
}
