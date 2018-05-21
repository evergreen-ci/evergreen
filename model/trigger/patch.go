package trigger

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

func init() {
	registry.AddTrigger(event.ResourceTypePatch,
		patchValidator(patchOutcome),
		patchValidator(patchFailure),
		patchValidator(patchSuccess),
		patchValidator(patchCreated),
		patchValidator(patchStarted),
	)
	registry.AddPrefetch(event.ResourceTypePatch, patchFetch)
}

func patchFetch(e *event.EventLogEntry) (interface{}, error) {
	p, err := patch.FindOne(patch.ById(bson.ObjectIdHex(e.ResourceId)))
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch patch")
	}
	if p == nil {
		return nil, errors.New("couldn't find patch")
	}

	return p, nil
}

func patchValidator(t func(e *event.EventLogEntry, p *patch.Patch) (*notificationGenerator, error)) trigger {
	return func(e *event.EventLogEntry, object interface{}) (*notificationGenerator, error) {
		p, ok := object.(*patch.Patch)
		if !ok {
			return nil, errors.New("expected a patch, received unknown type")
		}
		if p == nil {
			return nil, errors.New("expected a patch, received nil data")
		}

		return t(e, p)
	}
}

func patchSelectors(p *patch.Patch) []event.Selector {
	return []event.Selector{
		{
			Type: selectorID,
			Data: p.Id.Hex(),
		},
		{
			Type: selectorObject,
			Data: "patch",
		},
		{
			Type: selectorProject,
			Data: p.Project,
		},
		{
			Type: "owner",
			Data: p.Author,
		},
		{
			Type: "status",
			Data: p.Status,
		},
	}
}

func generatorFromPatch(triggerName string, p *patch.Patch) (*notificationGenerator, error) {
	ui := evergreen.UIConfig{}
	if err := ui.Get(); err != nil {
		return nil, errors.Wrap(err, "Failed to fetch ui config")
	}

	api := restModel.APIPatch{}
	if err := api.BuildFromService(*p); err != nil {
		return nil, errors.Wrap(err, "error building json model")
	}

	selectors := patchSelectors(p)

	data := commonTemplateData{
		ID:                p.Id.Hex(),
		Object:            "patch",
		Project:           p.Project,
		URL:               fmt.Sprintf("%s/version/%s", ui.Url, p.Version),
		PastTenseStatus:   p.Status,
		apiModel:          &api,
		githubState:       message.GithubStateFailure,
		githubDescription: fmt.Sprintf("patch finished in %s", p.FinishTime.Sub(p.StartTime).String()),
	}
	if p.Status == evergreen.PatchSucceeded {
		data.githubState = message.GithubStateSuccess
	}

	return makeCommonGenerator(triggerName, selectors, data)
}

func patchOutcome(e *event.EventLogEntry, p *patch.Patch) (*notificationGenerator, error) {
	const name = "outcome"

	if p.Status != evergreen.PatchSucceeded && p.Status != evergreen.PatchFailed {
		return nil, nil
	}

	gen, err := generatorFromPatch(name, p)
	return gen, err
}

func patchFailure(e *event.EventLogEntry, p *patch.Patch) (*notificationGenerator, error) {
	const name = "failure"

	if p.Status != evergreen.PatchFailed {
		return nil, nil
	}

	gen, err := generatorFromPatch(name, p)
	return gen, err
}

func patchSuccess(e *event.EventLogEntry, p *patch.Patch) (*notificationGenerator, error) {
	const name = "success"

	if p.Status != evergreen.PatchSucceeded {
		return nil, nil
	}

	gen, err := generatorFromPatch(name, p)
	return gen, err
}

// patchCreated and patchStarted are for Github Status API use only
func patchCreated(e *event.EventLogEntry, p *patch.Patch) (*notificationGenerator, error) {
	const name = "created"

	if p.Status != evergreen.PatchCreated {
		return nil, nil
	}

	ui := evergreen.UIConfig{}
	if err := ui.Get(); err != nil {
		return nil, errors.Wrap(err, "Failed to fetch ui config")
	}

	gen := &notificationGenerator{
		triggerName: name,
		selectors:   patchSelectors(p),
		githubStatusAPI: &message.GithubStatus{
			Context:     "evergreen",
			State:       message.GithubStatePending,
			URL:         fmt.Sprintf("%s/version/%s", ui.Url, p.Id.Hex()),
			Description: "preparing to run tasks",
		},
	}
	gen.selectors = append(gen.selectors, event.Selector{
		Type: "trigger",
		Data: name,
	})

	return gen, nil
}

func patchStarted(e *event.EventLogEntry, p *patch.Patch) (*notificationGenerator, error) {
	const name = "started"

	if p.Status != evergreen.PatchStarted {
		return nil, nil
	}

	ui := evergreen.UIConfig{}
	if err := ui.Get(); err != nil {
		return nil, errors.Wrap(err, "Failed to fetch ui config")
	}

	gen := &notificationGenerator{
		triggerName: name,
		selectors:   patchSelectors(p),
		githubStatusAPI: &message.GithubStatus{
			Context:     "evergreen",
			State:       message.GithubStatePending,
			URL:         fmt.Sprintf("%s/version/%s", ui.Url, p.Id.Hex()),
			Description: "tasks are running",
		},
	}
	gen.selectors = append(gen.selectors, event.Selector{
		Type: "trigger",
		Data: name,
	})

	return gen, nil
}
