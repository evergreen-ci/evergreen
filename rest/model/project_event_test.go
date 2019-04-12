package model

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/stretchr/testify/suite"
	mgobson "gopkg.in/mgo.v2/bson"
)

const (
	projectId = "mci2"
	username  = "me"
)

func getMockProjectSettings() model.ProjectSettingsEvent {
	return model.ProjectSettingsEvent{
		ProjectRef: model.ProjectRef{
			Owner:      "admin",
			Enabled:    true,
			Private:    true,
			Identifier: projectId,
			Admins:     []string{},
		},
		GitHubHooksEnabled: true,
		Vars: model.ProjectVars{
			Id:          projectId,
			Vars:        map[string]string{},
			PrivateVars: map[string]bool{},
		},
		Aliases: []model.ProjectAlias{model.ProjectAlias{
			ID:        mgobson.ObjectIdHex("5bedc72ee4055d31f0340b1d"),
			ProjectID: projectId,
			Alias:     "alias1",
			Variant:   "ubuntu",
			Task:      "subcommand",
		},
		},
		Subscriptions: []event.Subscription{event.Subscription{
			ID:           "subscription1",
			ResourceType: "project",
			Owner:        "admin",
			Subscriber: event.Subscriber{
				Type:   event.GithubPullRequestSubscriberType,
				Target: &event.GithubPullRequestSubscriber{},
			},
		},
		},
	}
}

type ProjectEventSuite struct {
	suite.Suite
	modelEvent  model.ProjectChangeEventEntry
	APIEvent    APIProjectEvent
	projChanges *model.ProjectChangeEvent
}

func TestProjectEventSuite(t *testing.T) {
	s := new(ProjectEventSuite)
	suite.Run(t, s)
}

func (s *ProjectEventSuite) SetupTest() {
	before := getMockProjectSettings()
	after := getMockProjectSettings()
	after.GitHubHooksEnabled = false

	h := model.ProjectChangeEventEntry{
		EventLogEntry: event.EventLogEntry{
			Timestamp:    time.Now(),
			ResourceType: model.EventResourceTypeProject,
			EventType:    model.EventTypeProjectModified,
			ResourceId:   projectId,
			Data: &model.ProjectChangeEvent{
				User:   username,
				Before: before,
				After:  after,
			},
		},
	}

	c := APIProjectEvent{}
	err := c.BuildFromService(h)
	s.Require().NoError(err)

	projChanges := h.Data.(*model.ProjectChangeEvent)

	s.projChanges = projChanges
	s.modelEvent = h
	s.APIEvent = c
}

func (s *ProjectEventSuite) TestProjectEventMetadata() {
	s.Equal(s.modelEvent.Timestamp, s.APIEvent.Timestamp)
	s.Equal(s.projChanges.User, FromAPIString(s.APIEvent.User))
}

func (s *ProjectEventSuite) TestProjectRef() {
	checkProjRef(s, s.projChanges.Before.ProjectRef, s.APIEvent.Before.ProjectRef)
	checkProjRef(s, s.projChanges.After.ProjectRef, s.APIEvent.After.ProjectRef)
}

func (s *ProjectEventSuite) TestGithubHooksEnabled() {
	s.Equal(s.projChanges.Before.GitHubHooksEnabled, s.APIEvent.Before.GitHubWebhooksEnabled)
	s.Equal(s.projChanges.After.GitHubHooksEnabled, s.APIEvent.After.GitHubWebhooksEnabled)
}

func (s *ProjectEventSuite) TestProjectVars() {
	checkVars(s, s.projChanges.Before.Vars, s.APIEvent.Before.Vars)
	checkVars(s, s.projChanges.After.Vars, s.APIEvent.After.Vars)
}

func (s *ProjectEventSuite) TestProjectSubscriptions() {
	checkSubscriptions(s, s.projChanges.Before.Subscriptions, s.APIEvent.Before.Subscriptions)
	checkSubscriptions(s, s.projChanges.After.Subscriptions, s.APIEvent.After.Subscriptions)
}

func (s *ProjectEventSuite) TestProjectAliases() {
	checkAliases(s, s.projChanges.Before.Aliases, s.APIEvent.Before.Aliases)
	checkAliases(s, s.projChanges.After.Aliases, s.APIEvent.After.Aliases)
}

func checkProjRef(suite *ProjectEventSuite, in model.ProjectRef, out APIProjectRef) {
	suite.Equal(in.Owner, FromAPIString(out.Owner))
	suite.Equal(in.Repo, FromAPIString(out.Repo))
	suite.Equal(in.Branch, FromAPIString(out.Branch))
	suite.Equal(in.Enabled, out.Enabled)
	suite.Equal(in.Private, out.Private)
	suite.Equal(in.BatchTime, out.BatchTime)
	suite.Equal(in.RemotePath, FromAPIString(out.RemotePath))
	suite.Equal(in.Identifier, FromAPIString(out.Identifier))
	suite.Equal(in.DisplayName, FromAPIString(out.DisplayName))
	suite.Equal(in.DeactivatePrevious, out.DeactivatePrevious)
	suite.Equal(in.TracksPushEvents, out.TracksPushEvents)
	suite.Equal(in.PRTestingEnabled, out.PRTestingEnabled)
	suite.Equal(in.PatchingDisabled, out.PatchingDisabled)

	suite.Require().Equal(len(in.Admins), len(out.Admins))
	for i, admin := range in.Admins {
		suite.Equal(admin, FromAPIString(out.Admins[i]))
	}

	suite.Equal(in.NotifyOnBuildFailure, out.NotifyOnBuildFailure)
}

func checkVars(suite *ProjectEventSuite, in model.ProjectVars, out APIProjectVars) {
	suite.Require().Equal(len(in.Vars), len(out.Vars))
	for k, v := range in.Vars {
		suite.Equal(v, out.Vars[k])
	}

	suite.Require().Equal(len(in.PrivateVars), len(out.PrivateVars))
	for k, v := range in.PrivateVars {
		suite.Equal(v, out.PrivateVars[k])
	}
}

func checkAliases(suite *ProjectEventSuite, in []model.ProjectAlias, out []APIProjectAlias) {
	suite.Require().Equal(len(in), len(out))
	for i, alias := range in {
		suite.Equal(alias.Alias, FromAPIString(out[i].Alias))
		suite.Equal(alias.Variant, FromAPIString(out[i].Variant))
		suite.Equal(alias.Task, FromAPIString(out[i].Task))

		suite.Require().Equal(len(alias.Tags), len(out[i].Tags))
		for j, tag := range alias.Tags {
			suite.Equal(tag, out[i].Tags[j])
		}
	}
}

func checkSubscriptions(suite *ProjectEventSuite, in []event.Subscription, out []APISubscription) {
	suite.Require().Equal(len(in), len(out))
	for i, subscription := range in {
		suite.Equal(subscription.ID, FromAPIString(out[i].ID))
		suite.Equal(subscription.ResourceType, FromAPIString(out[i].ResourceType))
		suite.Equal(subscription.Trigger, FromAPIString(out[i].Trigger))

		suite.Require().Equal(len(subscription.Selectors), len(out[i].Selectors))
		for j, selector := range subscription.Selectors {
			suite.Equal(selector.Type, FromAPIString(out[i].Selectors[j].Type))
			suite.Equal(selector.Data, FromAPIString(out[i].Selectors[j].Data))
		}

		suite.Require().Equal(len(subscription.RegexSelectors), len(out[i].RegexSelectors))
		for j, selector := range subscription.RegexSelectors {
			suite.Equal(selector.Type, FromAPIString(out[i].RegexSelectors[j].Type))
			suite.Equal(selector.Data, FromAPIString(out[i].RegexSelectors[j].Data))
		}

		suite.Equal(subscription.Subscriber.Type, FromAPIString(out[i].Subscriber.Type))
		checkSubscriptionTarget(suite, subscription.Subscriber.Target, out[i].Subscriber.Target)
		suite.Equal(string(subscription.OwnerType), FromAPIString(out[i].OwnerType))
		suite.Equal(subscription.Owner, FromAPIString(out[i].Owner))
	}
}

func checkSubscriptionTarget(suite *ProjectEventSuite, inTarget interface{}, outTarget interface{}) {
	switch v := inTarget.(type) {
	case *event.GithubPullRequestSubscriber:
		outTargetGithub, ok := outTarget.(APIGithubPRSubscriber)
		suite.Require().True(ok)
		suite.Equal(v.Owner, FromAPIString(outTargetGithub.Owner))
		suite.Equal(v.Repo, FromAPIString(outTargetGithub.Repo))
		suite.Equal(v.PRNumber, outTargetGithub.PRNumber)
		suite.Equal(v.Ref, FromAPIString(outTargetGithub.Ref))

	case *event.WebhookSubscriber:
		outTargetWebhook, ok := outTarget.(APIWebhookSubscriber)
		suite.Require().True(ok)
		suite.Equal(v.URL, FromAPIString(outTargetWebhook.URL))
		suite.Equal(v.Secret, FromAPIString(outTargetWebhook.Secret))

	case *event.JIRAIssueSubscriber:
		outTargetJIRA, ok := outTarget.(APIJIRAIssueSubscriber)
		suite.Require().True(ok)
		suite.Equal(v.Project, FromAPIString(outTargetJIRA.Project))
		suite.Equal(v.IssueType, FromAPIString(outTargetJIRA.IssueType))

	case *string: // JIRACommentSubscriberType, EmailSubscriberType, SlackSubscriberType
		outTargetString, ok := outTarget.(APIString)
		suite.Require().True(ok)
		suite.Equal(v, FromAPIString(outTargetString))

	default:
		suite.FailNow("Unknown target type")
	}
}
