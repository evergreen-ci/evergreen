package notification

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/pkg/errors"
)

func init() {
	registry.AddTrigger(event.ResourceTypePatch,
		patchValidator(patchOutcome),
		patchValidator(patchFailure),
		patchValidator(patchSuccess),
	)
}

func patchValidator(t func(data *event.PatchEventData) (*notificationGenerator, error)) trigger {
	return func(e *event.EventLogEntry) (*notificationGenerator, error) {
		data, ok := e.Data.(*event.PatchEventData)
		if !ok {
			return nil, errors.New("unexpected event payload (expected EventPatchData)")
		}

		return t(data)
	}
}

func patchOutcome(data *event.PatchEventData) (*notificationGenerator, error) {
	const name = "outcome"

	gen := notificationGenerator{
		triggerName: name,
		selectors:   patchBaseSelectors(data),
	}

	return &gen, nil
}

func patchFailure(data *event.PatchEventData) (*notificationGenerator, error) {
	const name = "failure"

	if data.Status != evergreen.PatchFailed {
		return nil, nil
	}

	return patchOutcome(data)
}

func patchSuccess(data *event.PatchEventData) (*notificationGenerator, error) {
	const name = "success"

	if data.Status != evergreen.PatchSucceeded {
		return nil, nil
	}

	return patchOutcome(data)
}

//func patchTimeExceedsConstant(data *event.PatchEventData) (*notificationGenerator, error) {
//	const name = "time-exceeds-n-constant"
//
//	if data.Status != evergreen.PatchSucceeded || data.Status != evergreen.PatchFailed {
//		return nil, nil
//	}
//
//	return patchOutcome(data)
//}
//
//func patchTimeExceedsRelativePercent(data *event.PatchEventData) (*notificationGenerator, error) {
//	const name = "time-exceeds-n%"
//
//	if data.Status != evergreen.PatchSucceeded || data.Status != evergreen.PatchFailed {
//		return nil, nil
//	}
//
//	return patchOutcome(data)
//}
