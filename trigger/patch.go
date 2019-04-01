package trigger

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/model/patch"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	mgobson "gopkg.in/mgo.v2/bson"
)

const (
	objectPatch         = "patch"
	triggerPatchStarted = "started"
)

type patchTriggers struct {
	event    *event.EventLogEntry
	data     *event.PatchEventData
	patch    *patch.Patch
	uiConfig evergreen.UIConfig

	base
}

func makePatchTriggers() eventHandler {
	t := &patchTriggers{}
	t.base.triggers = map[string]trigger{
		triggerOutcome:      t.patchOutcome,
		triggerFailure:      t.patchFailure,
		triggerSuccess:      t.patchSuccess,
		triggerPatchStarted: t.patchStarted,
	}
	return t
}

func (t *patchTriggers) Fetch(e *event.EventLogEntry) error {
	var err error
	if err = t.uiConfig.Get(); err != nil {
		return errors.Wrap(err, "Failed to fetch ui config")
	}

	oid := mgobson.ObjectIdHex(e.ResourceId)

	t.patch, err = patch.FindOne(patch.ById(oid))
	if err != nil {
		return errors.Wrapf(err, "failed to fetch patch '%s'", e.ResourceId)
	}
	if t.patch == nil {
		return errors.Errorf("can't find patch '%s'", e.ResourceId)
	}
	var ok bool
	t.data, ok = e.Data.(*event.PatchEventData)
	if !ok {
		return errors.Errorf("patch '%s' contains unexpected data with type '%T'", e.ResourceId, e.Data)
	}
	t.event = e

	return nil
}

func (t *patchTriggers) Selectors() []event.Selector {
	return []event.Selector{
		{
			Type: selectorID,
			Data: t.patch.Id.Hex(),
		},
		{
			Type: selectorObject,
			Data: objectPatch,
		},
		{
			Type: selectorProject,
			Data: t.patch.Project,
		},
		{
			Type: selectorOwner,
			Data: t.patch.Author,
		},
		{
			Type: selectorStatus,
			Data: t.patch.Status,
		},
	}
}

func (t *patchTriggers) patchOutcome(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.PatchSucceeded && t.data.Status != evergreen.PatchFailed {
		return nil, nil
	}

	return t.generate(sub)
}

func (t *patchTriggers) patchFailure(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.PatchFailed {
		return nil, nil
	}

	return t.generate(sub)
}

func (t *patchTriggers) patchSuccess(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.PatchSucceeded {
		return nil, nil
	}

	return t.generate(sub)
}

func (t *patchTriggers) patchStarted(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.PatchStarted {
		return nil, nil
	}

	return t.generate(sub)
}

func (t *patchTriggers) makeData(sub *event.Subscription) (*commonTemplateData, error) {
	api := restModel.APIPatch{}
	if err := api.BuildFromService(*t.patch); err != nil {
		return nil, errors.Wrap(err, "error building json model")
	}

	data := commonTemplateData{
		ID:                t.patch.Id.Hex(),
		EventID:           t.event.ID,
		SubscriptionID:    sub.ID,
		DisplayName:       t.patch.Id.Hex(),
		Description:       t.patch.Description,
		Object:            objectPatch,
		Project:           t.patch.Project,
		URL:               fmt.Sprintf("%s/version/%s", t.uiConfig.Url, t.patch.Version),
		PastTenseStatus:   t.data.Status,
		apiModel:          &api,
		githubState:       message.GithubStatePending,
		githubContext:     "evergreen",
		githubDescription: "tasks are running",
	}
	slackColor := evergreenFailColor
	if t.data.Status == evergreen.PatchSucceeded {
		slackColor = evergreenSuccessColor
		data.githubState = message.GithubStateSuccess
		data.githubDescription = fmt.Sprintf("patch finished in %s", t.patch.FinishTime.Sub(t.patch.StartTime).String())

	} else if t.data.Status == evergreen.PatchFailed {
		data.githubState = message.GithubStateFailure
		data.githubDescription = fmt.Sprintf("patch finished in %s", t.patch.FinishTime.Sub(t.patch.StartTime).String())
	}
	if t.patch.IsGithubPRPatch() {
		data.slack = append(data.slack, message.SlackAttachment{
			Title:     "Github Pull Request",
			TitleLink: fmt.Sprintf("https://github.com/%s/%s/pull/%d#partial-pull-merging", t.patch.GithubPatchData.BaseOwner, t.patch.GithubPatchData.BaseRepo, t.patch.GithubPatchData.PRNumber),
			Color:     slackColor,
		})
	}

	data.slack = append(data.slack, message.SlackAttachment{
		Title:     "Evergreen Patch",
		TitleLink: data.URL,
		Text:      t.patch.Description,
		Color:     slackColor,
	})

	return &data, nil
}

func (t *patchTriggers) generate(sub *event.Subscription) (*notification.Notification, error) {
	data, err := t.makeData(sub)
	if err != nil {
		return nil, errors.Wrap(err, "failed to collect patch data")
	}

	payload, err := makeCommonPayload(sub, t.Selectors(), data)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build notification")
	}

	return notification.New(t.event.ID, sub.Trigger, &sub.Subscriber, payload)
}
