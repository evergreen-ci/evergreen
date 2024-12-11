package trigger

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

func init() {
	registry.registerEventHandler(event.ResourceTypeCommitQueue, event.CommitQueueStartTest, makeCommitQueueTriggers)
	registry.registerEventHandler(event.ResourceTypeCommitQueue, event.CommitQueueConcludeTest, makeCommitQueueTriggers)
	registry.registerEventHandler(event.ResourceTypeCommitQueue, event.CommitQueueEnqueueFailed, makeCommitQueueTriggers)
}

type commitQueueTriggers struct {
	event    *event.EventLogEntry
	data     *event.CommitQueueEventData
	patch    *patch.Patch
	uiConfig evergreen.UIConfig

	base
}

func makeCommitQueueTriggers() eventHandler {
	t := &commitQueueTriggers{}
	t.base.triggers = map[string]trigger{
		event.TriggerOutcome: t.commitQueueOutcome,
	}
	return t
}

func (t *commitQueueTriggers) Fetch(ctx context.Context, e *event.EventLogEntry) error {
	var err error
	if err = t.uiConfig.Get(ctx); err != nil {
		return errors.Wrap(err, "fetching UI config")
	}

	oid := mgobson.ObjectIdHex(e.ResourceId)

	t.patch, err = patch.FindOne(patch.ById(oid))
	if err != nil {
		return errors.Wrapf(err, "finding patch '%s'", e.ResourceId)
	}
	if t.patch == nil {
		return errors.Errorf("patch '%s' not found", e.ResourceId)
	}
	var ok bool
	t.data, ok = e.Data.(*event.CommitQueueEventData)
	if !ok {
		return errors.Errorf("patch '%s' contains unexpected data with type '%T'", e.ResourceId, e.Data)
	}
	t.event = e

	return nil
}

func (t *commitQueueTriggers) Attributes() event.Attributes {
	return event.Attributes{
		Owner: []string{t.patch.Author},
	}
}

func (t *commitQueueTriggers) commitQueueOutcome(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	data, err := t.makeData(sub)
	if err != nil {
		return nil, errors.Wrap(err, "collecting patch data")
	}

	payload, err := makeCommonPayload(sub, t.Attributes(), data)
	if err != nil {
		return nil, errors.Wrap(err, "building notification")
	}

	return notification.New(t.event.ID, sub.Trigger, &sub.Subscriber, payload)
}

func (t *commitQueueTriggers) makeData(sub *event.Subscription) (*commonTemplateData, error) {
	text := t.patch.Description
	if t.data.Error != "" {
		text = t.data.Error
	}
	url := ""
	if t.patch.Version != "" {
		url = fmt.Sprintf("%s/version/%s?redirect_spruce_users=true", t.uiConfig.Url, t.patch.Version)
	}
	projectName := t.patch.Project
	identifier, err := model.GetIdentifierForProject(t.patch.Project)
	if err == nil && identifier != "" {
		projectName = identifier
	}
	data := commonTemplateData{
		ID:              t.patch.Id.Hex(),
		EventID:         t.event.ID,
		SubscriptionID:  sub.ID,
		DisplayName:     t.patch.Id.Hex(),
		Description:     text,
		Object:          "merge",
		Project:         projectName,
		URL:             url,
		PastTenseStatus: t.data.Status,
	}

	slackColor := evergreenFailColor
	if t.data.Status == evergreen.VersionSucceeded || t.data.Status == evergreen.MergeTestStarted {
		slackColor = evergreenSuccessColor
	}

	data.slack = append(data.slack, message.SlackAttachment{
		Title:     "Evergreen Merge Test",
		TitleLink: data.URL,
		Text:      text,
		Color:     slackColor,
	})

	if t.patch.IsPRMergePatch() {
		data.slack = append(data.slack, message.SlackAttachment{
			Title:     "GitHub Pull Request",
			TitleLink: fmt.Sprintf("https://github.com/%s/%s/pull/%d#partial-pull-merging", t.patch.GithubPatchData.BaseOwner, t.patch.GithubPatchData.BaseRepo, t.patch.GithubPatchData.PRNumber),
			Color:     slackColor,
		})
	}

	return &data, nil
}
