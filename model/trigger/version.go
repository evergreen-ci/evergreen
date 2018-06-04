package trigger

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/version"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/pkg/errors"
)

func init() {
	registry.AddTrigger(event.ResourceTypeVersion,
		versionValidator(versionOutcome),
		versionValidator(versionFailure),
		versionValidator(versionSuccess),
	)
	registry.AddPrefetch(event.ResourceTypeVersion, versionFetch)
}

func versionFetch(e *event.EventLogEntry) (interface{}, error) {
	v, err := version.FindOne(version.ById(e.ResourceId))
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch version")
	}
	if v == nil {
		return nil, errors.New("couldn't find version")
	}

	return v, nil
}

func versionValidator(t func(e *event.VersionEventData, v *version.Version) (*notificationGenerator, error)) oldTrigger {
	return func(e *event.EventLogEntry, object interface{}) (*notificationGenerator, error) {
		v, ok := object.(*version.Version)
		if !ok {
			return nil, errors.New("expected a version, received unknown type")
		}
		if v == nil {
			return nil, errors.New("expected a version, received nil data")
		}

		data, ok := e.Data.(*event.VersionEventData)
		if !ok {
			return nil, errors.New("expected version event data")
		}

		return t(data, v)
	}
}

func versionSelectors(v *version.Version) []event.Selector {
	return []event.Selector{
		{
			Type: selectorID,
			Data: v.Id,
		},
		{
			Type: selectorProject,
			Data: v.Identifier,
		},
		{
			Type: selectorObject,
			Data: "version",
		},
		{
			Type: selectorRequester,
			Data: v.Requester,
		},
	}
}

func generatorFromVersion(triggerName string, v *version.Version, status string) (*notificationGenerator, error) {
	ui := evergreen.UIConfig{}
	if err := ui.Get(); err != nil {
		return nil, errors.Wrap(err, "Failed to fetch ui config")
	}

	api := restModel.APIVersion{}
	if err := api.BuildFromService(v); err != nil {
		return nil, errors.Wrap(err, "error building json model")
	}

	selectors := versionSelectors(v)

	pastTenseStatus := status
	if status == evergreen.VersionSucceeded {
		pastTenseStatus = "succeeded"
	}

	data := commonTemplateData{
		ID:              v.Id,
		Object:          "version",
		Project:         v.Identifier,
		URL:             fmt.Sprintf("%s/version/%s", ui.Url, v.Id),
		PastTenseStatus: pastTenseStatus,
		apiModel:        &api,
	}

	return makeCommonGenerator(triggerName, selectors, data)
}

func versionOutcome(e *event.VersionEventData, v *version.Version) (*notificationGenerator, error) {
	const name = "outcome"

	if e.Status != evergreen.VersionSucceeded && e.Status != evergreen.VersionFailed {
		return nil, nil
	}

	gen, err := generatorFromVersion(name, v, e.Status)
	return gen, err
}

func versionFailure(e *event.VersionEventData, v *version.Version) (*notificationGenerator, error) {
	const name = "failure"

	if e.Status != evergreen.VersionFailed {
		return nil, nil
	}

	gen, err := generatorFromVersion(name, v, e.Status)
	return gen, err
}

func versionSuccess(e *event.VersionEventData, v *version.Version) (*notificationGenerator, error) {
	const name = "success"

	if e.Status != evergreen.VersionSucceeded {
		return nil, nil
	}

	gen, err := generatorFromVersion(name, v, e.Status)
	return gen, err
}
