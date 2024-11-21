package model

import (
	"testing"
	"time"

	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/suite"
)

const (
	projectId = "mci2"
	username  = "me"
)

func getMockProjectSettings() model.ProjectSettings {
	return model.ProjectSettings{
		ProjectRef: model.ProjectRef{
			Owner:   "admin",
			Enabled: true,
			Id:      projectId,
			Admins:  []string{},
		},
		GithubHooksEnabled: true,
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
	after.GithubHooksEnabled = false

	h := model.ProjectChangeEventEntry{
		EventLogEntry: event.EventLogEntry{
			Timestamp:    time.Now(),
			ResourceType: event.EventResourceTypeProject,
			EventType:    event.EventTypeProjectModified,
			ResourceId:   projectId,
			Data: &model.ProjectChangeEvent{
				User:   username,
				Before: model.NewProjectSettingsEvent(before),
				After:  model.NewProjectSettingsEvent(after),
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
	s.Equal(s.modelEvent.Timestamp, *s.APIEvent.Timestamp)
	s.Equal(s.projChanges.User, utility.FromStringPtr(s.APIEvent.User))
}

func (s *ProjectEventSuite) TestProjectRef() {
	checkProjRef(s, s.projChanges.Before.ProjectRef, s.APIEvent.Before.ProjectRef)
	checkProjRef(s, s.projChanges.After.ProjectRef, s.APIEvent.After.ProjectRef)
}

func (s *ProjectEventSuite) TestGithubHooksEnabled() {
	s.Equal(s.projChanges.Before.GithubHooksEnabled, s.APIEvent.Before.GithubWebhooksEnabled)
	s.Equal(s.projChanges.After.GithubHooksEnabled, s.APIEvent.After.GithubWebhooksEnabled)
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
	suite.Equal(in.Owner, utility.FromStringPtr(out.Owner))
	suite.Equal(in.Repo, utility.FromStringPtr(out.Repo))
	suite.Equal(in.Branch, utility.FromStringPtr(out.Branch))
	suite.Equal(in.Enabled, utility.FromBoolPtr(out.Enabled))
	suite.Equal(in.BatchTime, out.BatchTime)
	suite.Equal(in.RemotePath, utility.FromStringPtr(out.RemotePath))
	suite.Equal(in.Id, utility.FromStringPtr(out.Id))
	suite.Equal(in.Identifier, utility.FromStringPtr(out.Identifier))
	suite.Equal(in.DisplayName, utility.FromStringPtr(out.DisplayName))
	suite.Equal(in.DeactivatePrevious, out.DeactivatePrevious)
	suite.Equal(in.TracksPushEvents, out.TracksPushEvents)
	suite.Equal(in.PRTestingEnabled, out.PRTestingEnabled)
	suite.Equal(in.PatchingDisabled, out.PatchingDisabled)

	suite.Require().Equal(len(in.Admins), len(out.Admins))
	for i, admin := range in.Admins {
		suite.Equal(admin, utility.FromStringPtr(out.Admins[i]))
	}

	suite.Equal(in.NotifyOnBuildFailure, out.NotifyOnBuildFailure)
}

func checkVars(suite *ProjectEventSuite, in model.ProjectEventVars, out APIProjectVars) {
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
		suite.Equal(alias.Alias, utility.FromStringPtr(out[i].Alias))
		suite.Equal(alias.Variant, utility.FromStringPtr(out[i].Variant))
		suite.Equal(alias.Task, utility.FromStringPtr(out[i].Task))

		suite.Require().Equal(len(alias.TaskTags), len(out[i].TaskTags))
		suite.Require().Equal(len(alias.VariantTags), len(out[i].VariantTags))
		for j, tag := range alias.TaskTags {
			suite.Equal(tag, out[i].TaskTags[j])
		}
		for j, tag := range alias.VariantTags {
			suite.Equal(tag, out[i].VariantTags[j])
		}
	}
}

func checkSubscriptions(suite *ProjectEventSuite, in []event.Subscription, out []APISubscription) {
	suite.Require().Equal(len(in), len(out))
	for i, subscription := range in {
		suite.Equal(subscription.ID, utility.FromStringPtr(out[i].ID))
		suite.Equal(subscription.ResourceType, utility.FromStringPtr(out[i].ResourceType))
		suite.Equal(subscription.Trigger, utility.FromStringPtr(out[i].Trigger))

		suite.Require().Equal(len(subscription.Selectors), len(out[i].Selectors))
		for j, selector := range subscription.Selectors {
			suite.Equal(selector.Type, utility.FromStringPtr(out[i].Selectors[j].Type))
			suite.Equal(selector.Data, utility.FromStringPtr(out[i].Selectors[j].Data))
		}

		suite.Require().Equal(len(subscription.RegexSelectors), len(out[i].RegexSelectors))
		for j, selector := range subscription.RegexSelectors {
			suite.Equal(selector.Type, utility.FromStringPtr(out[i].RegexSelectors[j].Type))
			suite.Equal(selector.Data, utility.FromStringPtr(out[i].RegexSelectors[j].Data))
		}

		suite.Equal(subscription.Subscriber.Type, utility.FromStringPtr(out[i].Subscriber.Type))
		checkSubscriptionTarget(suite, subscription.Subscriber.Target, out[i].Subscriber.Target)
		suite.Equal(string(subscription.OwnerType), utility.FromStringPtr(out[i].OwnerType))
		suite.Equal(subscription.Owner, utility.FromStringPtr(out[i].Owner))
	}
}

func checkSubscriptionTarget(suite *ProjectEventSuite, inTarget interface{}, outTarget interface{}) {
	switch v := inTarget.(type) {
	case *event.GithubPullRequestSubscriber:
		outTargetGithub, ok := outTarget.(APIGithubPRSubscriber)
		suite.Require().True(ok)
		suite.Equal(v.Owner, utility.FromStringPtr(outTargetGithub.Owner))
		suite.Equal(v.Repo, utility.FromStringPtr(outTargetGithub.Repo))
		suite.Equal(v.PRNumber, outTargetGithub.PRNumber)
		suite.Equal(v.Ref, utility.FromStringPtr(outTargetGithub.Ref))

	case *event.WebhookSubscriber:
		outTargetWebhook, ok := outTarget.(APIWebhookSubscriber)
		suite.Require().True(ok)
		suite.Equal(v.URL, utility.FromStringPtr(outTargetWebhook.URL))
		suite.Equal(v.Secret, utility.FromStringPtr(outTargetWebhook.Secret))

	case *event.JIRAIssueSubscriber:
		outTargetJIRA, ok := outTarget.(APIJIRAIssueSubscriber)
		suite.Require().True(ok)
		suite.Equal(v.Project, utility.FromStringPtr(outTargetJIRA.Project))
		suite.Equal(v.IssueType, utility.FromStringPtr(outTargetJIRA.IssueType))

	case *string: // JIRACommentSubscriberType, EmailSubscriberType, SlackSubscriberType
		outTargetString, ok := outTarget.(*string)
		suite.Require().True(ok)
		suite.Equal(v, utility.FromStringPtr(outTargetString))

	default:
		suite.FailNow("Unknown target type")
	}
}
