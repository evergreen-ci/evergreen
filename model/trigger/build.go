package trigger

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	objectBuild         = "build"
	triggerBuildOutcome = "outcome"
	triggerBuildFailure = "failure"
	triggerBuildSuccess = "success"
	triggerBuildStarted = "started"
)

func init() {
	registry.registerEventHandler(event.ResourceTypeBuild, event.BuildStateChange, makeBuildTriggers)
}

func taskStatusToDesc(b *build.Build) string {
	success := 0
	failed := 0
	systemError := 0
	other := 0
	noReport := 0
	for _, task := range b.Tasks {
		switch task.Status {
		case evergreen.TaskSucceeded:
			success++

		case evergreen.TaskFailed:
			failed++

		case evergreen.TaskSystemFailed, evergreen.TaskTimedOut,
			evergreen.TaskSystemUnresponse, evergreen.TaskSystemTimedOut,
			evergreen.TaskTestTimedOut:
			systemError++

		case evergreen.TaskStarted, evergreen.TaskUnstarted,
			evergreen.TaskUndispatched, evergreen.TaskDispatched,
			evergreen.TaskConflict, evergreen.TaskInactive:
			noReport++

		default:
			other++
		}
	}

	grip.ErrorWhen(other > 0, message.Fields{
		"source":   "status updates",
		"message":  "unknown task status",
		"build_id": b.Id,
	})

	if success == 0 && failed == 0 && systemError == 0 && other == 0 {
		return "no tasks were run"
	}

	desc := fmt.Sprintf("%s, %s", taskStatusSubformat(success, "succeeded"),
		taskStatusSubformat(failed, "failed"))
	if systemError > 0 {
		desc += fmt.Sprintf(", %d internal errors", systemError)
	}
	if other > 0 {
		desc += fmt.Sprintf(", %d other", other)
	}

	return appendTime(b, desc)
}

func taskStatusSubformat(n int, verb string) string {
	if n == 0 {
		return fmt.Sprintf("none %s", verb)
	}
	return fmt.Sprintf("%d %s", n, verb)
}

func appendTime(b *build.Build, txt string) string {
	return fmt.Sprintf("%s in %s", txt, b.FinishTime.Sub(b.StartTime).String())
}

type buildTriggers struct {
	event    *event.EventLogEntry
	data     *event.BuildEventData
	build    *build.Build
	uiConfig evergreen.UIConfig

	base
}

func makeBuildTriggers() eventHandler {
	t := &buildTriggers{}
	t.base.triggers = map[string]trigger{
		triggerBuildOutcome: t.buildOutcome,
		triggerBuildFailure: t.buildFailure,
		triggerBuildSuccess: t.buildSuccess,
	}
	return t
}

func (t *buildTriggers) Fetch(e *event.EventLogEntry) error {
	var err error
	if err = t.uiConfig.Get(); err != nil {
		return errors.Wrap(err, "Failed to fetch ui config")
	}

	t.build, err = build.FindOne(build.ById(e.ResourceId))
	if err != nil {
		return errors.Wrap(err, "failed to fetch build")
	}
	if t.build == nil {
		return errors.New("couldn't find build")
	}

	var ok bool
	t.data, ok = e.Data.(*event.BuildEventData)
	if !ok {
		return errors.Wrapf(err, "patch '%s' contains unexpected data with type '%T'", e.ResourceId, e.Data)
	}
	t.event = e

	return nil
}

func (t *buildTriggers) Selectors() []event.Selector {
	return []event.Selector{
		{
			Type: selectorID,
			Data: t.build.Id,
		},
		{
			Type: selectorObject,
			Data: objectBuild,
		},
		{
			Type: selectorProject,
			Data: t.build.Project,
		},
		{
			Type: selectorRequester,
			Data: t.build.Requester,
		},
		{
			Type: selectorInVersion,
			Data: t.build.Version,
		},
	}
}

func (t *buildTriggers) buildOutcome(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.BuildSucceeded && t.data.Status != evergreen.BuildFailed {
		return nil, nil
	}

	return t.generate(sub)
}

func (t *buildTriggers) buildFailure(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.BuildFailed {
		return nil, nil
	}

	return t.generate(sub)
}

func (t *buildTriggers) buildSuccess(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.BuildSucceeded {
		return nil, nil
	}

	return t.generate(sub)
}

func (t *buildTriggers) makeData(sub *event.Subscription) (*commonTemplateData, error) {
	api := restModel.APIBuild{}
	if err := api.BuildFromService(*t.build); err != nil {
		return nil, errors.Wrap(err, "error building json model")
	}

	data := commonTemplateData{
		ID:              t.build.Id,
		Object:          objectBuild,
		Project:         t.build.Project,
		URL:             fmt.Sprintf("%s/build/%s", t.uiConfig.Url, t.build.Id),
		PastTenseStatus: t.data.Status,
		apiModel:        &api,
	}
	if t.build.Requester == evergreen.GithubPRRequester && t.build.Status == t.data.Status {
		data.githubContext = fmt.Sprintf("evergreen/%s", t.build.BuildVariant)
		data.githubState = message.GithubStateFailure
		data.githubDescription = taskStatusToDesc(t.build)
	}
	if t.data.Status == evergreen.BuildSucceeded {
		data.githubState = message.GithubStateSuccess
		data.PastTenseStatus = "succeeded"
	}
	return &data, nil
}

func (t *buildTriggers) generate(sub *event.Subscription) (*notification.Notification, error) {
	data, err := t.makeData(sub)
	if err != nil {
		return nil, errors.Wrap(err, "failed to collect patch data")
	}

	payload, err := makeCommonPayload(sub, t.Selectors(), *data)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build notification")
	}

	return notification.New(t.event, sub.Trigger, &sub.Subscriber, payload)
}
