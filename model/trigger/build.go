package trigger

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	objectBuild = "build"
)

func init() {
	registry.AddTrigger(event.ResourceTypeBuild,
		buildValidator(buildOutcome),
		buildValidator(buildFailure),
		buildValidator(buildSuccess),
	)
	registry.AddPrefetch(event.ResourceTypeBuild, buildFetch)
}

func buildFetch(e *event.EventLogEntry) (interface{}, error) {
	p, err := build.FindOne(build.ById(e.ResourceId))
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch build")
	}
	if p == nil {
		return nil, errors.New("couldn't find build")
	}

	return p, nil
}

func buildValidator(t func(e *event.BuildEventData, b *build.Build) (*notificationGenerator, error)) trigger {
	return func(e *event.EventLogEntry, object interface{}) (*notificationGenerator, error) {
		b, ok := object.(*build.Build)
		if !ok {
			return nil, errors.New("expected a build, received unknown type")
		}
		if b == nil {
			return nil, errors.New("expected a build, received nil data")
		}

		data, ok := e.Data.(*event.BuildEventData)
		if !ok {
			return nil, errors.New("expected build event data")
		}

		return t(data, b)
	}
}

func buildSelectors(b *build.Build) []event.Selector {
	return []event.Selector{
		{
			Type: selectorID,
			Data: b.Id,
		},
		{
			Type: selectorObject,
			Data: objectBuild,
		},
		{
			Type: selectorProject,
			Data: b.Project,
		},
		{
			Type: selectorRequester,
			Data: b.Requester,
		},
	}
}

func generatorFromBuild(triggerName string, b *build.Build, status string) (*notificationGenerator, error) {
	ui := evergreen.UIConfig{}
	if err := ui.Get(); err != nil {
		return nil, errors.Wrap(err, "Failed to fetch ui config")
	}

	api := restModel.APIBuild{}
	if err := api.BuildFromService(*b); err != nil {
		return nil, errors.Wrap(err, "error building json model")
	}

	selectors := buildSelectors(b)
	data := commonTemplateData{
		ID:              b.Id,
		Object:          objectBuild,
		Project:         b.Project,
		URL:             fmt.Sprintf("%s/build/%s", ui.Url, b.Id),
		PastTenseStatus: status,
		apiModel:        &api,
	}
	if status == evergreen.BuildSucceeded {
		data.githubState = message.GithubStateSuccess
		data.PastTenseStatus = "succeeded"
	}
	if b.Status == status {
		data.githubState = message.GithubStateFailure
		data.githubDescription = TaskStatusToDesc(b)
	}

	return makeCommonGenerator(triggerName, selectors, data)
}

func buildOutcome(e *event.BuildEventData, b *build.Build) (*notificationGenerator, error) {
	const name = "outcome"

	if e.Status != evergreen.BuildSucceeded && e.Status != evergreen.BuildFailed {
		return nil, nil
	}

	gen, err := generatorFromBuild(name, b, e.Status)
	return gen, err
}

func buildFailure(e *event.BuildEventData, b *build.Build) (*notificationGenerator, error) {
	const name = "failure"

	if e.Status != evergreen.BuildFailed {
		return nil, nil
	}

	gen, err := generatorFromBuild(name, b, e.Status)
	return gen, err
}

func buildSuccess(e *event.BuildEventData, b *build.Build) (*notificationGenerator, error) {
	const name = "success"

	if e.Status != evergreen.BuildSucceeded {
		return nil, nil
	}

	gen, err := generatorFromBuild(name, b, e.Status)
	return gen, err
}

// TODO: EVG-3087 stop using this in units and make it private
func TaskStatusToDesc(b *build.Build) string {
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
	grip.ErrorWhen(noReport > 0, message.Fields{
		"source":   "status updates",
		"message":  "updating status for incomplete build",
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
