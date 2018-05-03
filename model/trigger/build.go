package trigger

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

func init() {
	registry.AddTrigger(event.ResourceTypePatch,
		buildValidator(buildOutcome),
		buildValidator(buildFailure),
		buildValidator(buildSuccess),
	)
	registry.AddPrefetch(event.ResourceTypePatch, patchFetch)
}

func buildFetch(e *event.EventLogEntry) (interface{}, error) {
	p, err := patch.FindOne(build.ById(e.ResourceId))
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch patch")
	}
	if p == nil {
		return nil, errors.New("couldn't find patch")
	}

	return p, nil
}

func buildValidator(t func(e *event.EventLogEntry, b *build.Build) (*notificationGenerator, error)) trigger {
	return func(e *event.EventLogEntry, object interface{}) (*notificationGenerator, error) {
		b, ok := object.(*build.Build)
		if !ok {
			return nil, errors.New("expected a build, received unknown type")
		}
		if b == nil {
			return nil, errors.New("expected a build, received nil data")
		}

		return t(e, b)
	}
}

func buildSelectors(b *build.Build) []event.Selector {
	return []event.Selector{
		{
			Type: "id",
			Data: b.Id,
		},
		{
			Type: "object",
			Data: "build",
		},
		{
			Type: "project",
			Data: b.Project,
		},
		//{
		//	Type: "owner",
		//	Data: p.Author,
		//},
	}
}

func generatorFromBuild(triggerName string, b *build.Build) (*notificationGenerator, error) {
	gen := notificationGenerator{
		triggerName: triggerName,
		selectors:   buildSelectors(b),
	}

	gen.selectors = append(gen.selectors, event.Selector{
		Type: "trigger",
		Data: triggerName,
	})

	ui := evergreen.UIConfig{}
	if err := ui.Get(); err != nil {
		return nil, errors.Wrap(err, "Failed to fetch ui config")
	}

	data := commonTemplateData{
		ID:              b.Id,
		Object:          "build",
		Project:         b.Project,
		URL:             fmt.Sprintf("%s/build/%s", ui.Url, b.Id),
		PastTenseStatus: b.Status,
		Headers:         makeHeaders(gen.selectors),
	}

	api := restModel.APIBuild{}
	if err := api.BuildFromService(*b); err != nil {
		return nil, errors.Wrap(err, "error building json model")
	}

	var err error
	gen.evergreenWebhook, err = webhookPayload(&api, data.Headers)
	if err != nil {
		return nil, errors.Wrap(err, "error building webhook payload")
	}

	gen.email, err = emailPayload(data)
	if err != nil {
		return nil, errors.Wrap(err, "error building email payload")
	}

	gen.jiraComment, err = jiraComment(data)
	if err != nil {
		return nil, errors.Wrap(err, "error building jira comment")
	}
	gen.jiraIssue, err = jiraIssue(data)
	if err != nil {
		return nil, errors.Wrap(err, "error building jira issue")
	}

	state := message.GithubStateSuccess
	if b.Status == evergreen.BuildFailed {
		state = message.GithubStateFailure
	}

	gen.githubStatusAPI = &message.GithubStatus{
		Context:     fmt.Sprintf("evergreen/%s", b.BuildVariant),
		State:       state,
		URL:         data.URL,
		Description: taskStatusToDesc(b),
	}

	// TODO improve slack body with additional info, like failing variants
	gen.slack, err = slack(data)
	if err != nil {
		return nil, errors.Wrap(err, "error building slack message")
	}

	return &gen, nil
}

func buildOutcome(e *event.EventLogEntry, b *build.Build) (*notificationGenerator, error) {
	const name = "outcome"

	if b.Status != evergreen.BuildSucceeded && b.Status != evergreen.BuildFailed {
		return nil, nil
	}

	gen, err := generatorFromBuild(name, b)
	gen.triggerName = name
	return gen, err
}

func buildFailure(e *event.EventLogEntry, b *build.Build) (*notificationGenerator, error) {
	const name = "failure"

	if b.Status != evergreen.BuildFailed {
		return nil, nil
	}

	gen, err := generatorFromBuild(name, b)
	gen.triggerName = name
	return gen, err
}

func buildSuccess(e *event.EventLogEntry, b *build.Build) (*notificationGenerator, error) {
	const name = "success"

	if b.Status != evergreen.BuildSucceeded {
		return nil, nil
	}

	gen, err := generatorFromBuild(name, b)
	gen.triggerName = name
	return gen, err
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
