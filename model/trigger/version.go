package trigger

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	objectVersion = "version"
)

func init() {
	registry.registerEventHandler(event.ResourceTypeVersion, event.VersionStateChange, makeVersionTriggers)
}

type versionTriggers struct {
	event    *event.EventLogEntry
	data     *event.VersionEventData
	version  *version.Version
	uiConfig evergreen.UIConfig

	base
}

func makeVersionTriggers() eventHandler {
	t := &versionTriggers{}
	t.base.triggers = map[string]trigger{
		triggerOutcome:    t.versionOutcome,
		triggerFailure:    t.versionFailure,
		triggerSuccess:    t.versionSuccess,
		triggerRegression: t.versionRegression,
	}
	return t
}

func (t *versionTriggers) Fetch(e *event.EventLogEntry) error {
	var err error
	if err = t.uiConfig.Get(); err != nil {
		return errors.Wrap(err, "Failed to fetch ui config")
	}

	t.version, err = version.FindOne(version.ById(e.ResourceId))
	if err != nil {
		return errors.Wrap(err, "failed to fetch version")
	}
	if t.version == nil {
		return errors.New("couldn't find version")
	}

	var ok bool
	t.data, ok = e.Data.(*event.VersionEventData)
	if !ok {
		return errors.Wrapf(err, "version '%s' contains unexpected data with type '%T'", e.ResourceId, e.Data)
	}
	t.event = e

	return nil
}

func (t *versionTriggers) Selectors() []event.Selector {
	return []event.Selector{
		{
			Type: selectorID,
			Data: t.version.Id,
		},
		{
			Type: selectorProject,
			Data: t.version.Identifier,
		},
		{
			Type: selectorObject,
			Data: objectVersion,
		},
		{
			Type: selectorRequester,
			Data: t.version.Requester,
		},
		{
			Type: selectorProject,
			Data: t.version.Branch,
		},
	}
}

func (t *versionTriggers) makeData(sub *event.Subscription) (*commonTemplateData, error) {
	api := restModel.APIVersion{}
	if err := api.BuildFromService(t.version); err != nil {
		return nil, errors.Wrap(err, "error building json model")
	}

	data := commonTemplateData{
		ID:              t.version.Id,
		Object:          objectVersion,
		Project:         t.version.Identifier,
		URL:             fmt.Sprintf("%s/version/%s", t.uiConfig.Url, t.version.Id),
		PastTenseStatus: t.data.Status,
		apiModel:        &api,
	}
	slackColor := evergreenFailColor
	if data.PastTenseStatus == evergreen.VersionSucceeded {
		data.PastTenseStatus = "succeeded"
		slackColor = evergreenSuccessColor
	}
	data.slack = []message.SlackAttachment{
		{
			Title:     "Evergreen Version",
			TitleLink: data.URL,
			Color:     slackColor,
			Text:      t.version.Message,
		},
	}

	return &data, nil
}

func (t *versionTriggers) generate(sub *event.Subscription) (*notification.Notification, error) {
	data, err := t.makeData(sub)
	if err != nil {
		return nil, errors.Wrap(err, "failed to collect version data")
	}

	payload, err := makeCommonPayload(sub, t.Selectors(), data)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build notification")
	}

	return notification.New(t.event, sub.Trigger, &sub.Subscriber, payload)
}

func (t *versionTriggers) versionOutcome(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.VersionSucceeded && t.data.Status != evergreen.VersionFailed {
		return nil, nil
	}

	return t.generate(sub)
}

func (t *versionTriggers) versionFailure(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.VersionFailed {
		return nil, nil
	}

	return t.generate(sub)
}

func (t *versionTriggers) versionSuccess(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.VersionSucceeded {
		return nil, nil
	}

	return t.generate(sub)
}

func (t *versionTriggers) versionRegression(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.VersionFailed || t.version.Requester != evergreen.RepotrackerVersionRequester {
		return nil, nil
	}

	versionTasks, err := task.FindWithDisplayTasks(task.ByVersion(t.version.Id))
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving tasks for version")
	}
	for i := range versionTasks {
		task := &versionTasks[i]
		isRegression, _, err := isTaskRegression(task)
		if err != nil {
			return nil, errors.Wrap(err, "error evaluating task regression")
		}
		if isRegression {
			return t.generate(sub)
		}
	}
	return nil, nil
}
