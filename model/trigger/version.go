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

func versionValidator(t func(e *event.EventLogEntry, v *version.Version) (*notificationGenerator, error)) trigger {
	return func(e *event.EventLogEntry, object interface{}) (*notificationGenerator, error) {
		v, ok := object.(*version.Version)
		if !ok {
			return nil, errors.New("expected a version, received unknown type")
		}
		if v == nil {
			return nil, errors.New("expected a version, received nil data")
		}

		return t(e, v)
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
			Type: "status",
			Data: v.Status,
		},
		{
			Type: selectorObject,
			Data: "version",
		},
	}
}

func generatorFromVersion(triggerName string, v *version.Version) (*notificationGenerator, error) {
	ui := evergreen.UIConfig{}
	if err := ui.Get(); err != nil {
		return nil, errors.Wrap(err, "Failed to fetch ui config")
	}

	api := restModel.APIVersion{}
	if err := api.BuildFromService(v); err != nil {
		return nil, errors.Wrap(err, "error building json model")
	}

	selectors := versionSelectors(v)

	pastTenseStatus := v.Status
	if v.Status == evergreen.VersionSucceeded {
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

func versionOutcome(e *event.EventLogEntry, v *version.Version) (*notificationGenerator, error) {
	const name = "outcome"

	if v.Status != evergreen.VersionSucceeded && v.Status != evergreen.VersionFailed {
		return nil, nil
	}

	gen, err := generatorFromVersion(name, v)
	return gen, err
}

func versionFailure(e *event.EventLogEntry, v *version.Version) (*notificationGenerator, error) {
	const name = "failure"

	if v.Status != evergreen.VersionFailed {
		return nil, nil
	}

	gen, err := generatorFromVersion(name, v)
	return gen, err
}

func versionSuccess(e *event.EventLogEntry, v *version.Version) (*notificationGenerator, error) {
	const name = "success"

	if v.Status != evergreen.VersionSucceeded {
		return nil, nil
	}

	gen, err := generatorFromVersion(name, v)
	return gen, err
}
