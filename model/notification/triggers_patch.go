package notification

//package notification
//
//import (
//	"github.com/evergreen-ci/evergreen"
//	"github.com/evergreen-ci/evergreen/model/event"
//	"github.com/pkg/errors"
//)
//
//func init() {
//
//}
//
//func patchBaseSelectors(data *PatchEventData) []Selector {
//	return []Selector{
//		{
//			Type: "id",
//			Data: data.Version,
//		},
//		{
//			Type: "user",
//			Data: data.Author,
//		},
//	}
//}
//
//func patchOutcome(e *event.EventLogEntry) (notificationGenerator, error) {
//	const name = "outcome"
//
//	gen := notificationGenerator{
//		triggerName: name,
//		selectors: []Selector{
//			{
//				Type: "id",
//				Data: data.Version,
//			},
//			{
//				Type: "user",
//				Data: data.Author,
//			},
//		},
//	}
//
//}
//
//// patch failure trigger is the outcome trigger, but only for failures
//func patchFailure(e *event.EventLogEntry) (notificationGenerator, error) {
//	const name = "failure"
//
//	data, ok := e.Data.(*PatchEventData)
//	if !ok {
//		return notificationGenerator{}, errors.New("unvalid event data")
//	}
//
//	if data.Status != evergreen.PatchFailed {
//		return notificationGenerator{}, nil
//	}
//
//	return patchOutcome(e)
//}
//
//func patchSuccess(e *event.EventLogEntry) (notificationGenerator, error) {
//	const name = "success"
//
//	data, ok := e.Data.(*PatchEventData)
//	if !ok {
//		return notificationGenerator{}, errors.New("unvalid event data")
//	}
//
//	if data.Status != evergreen.PatchSucceeded {
//		return notificationGenerator{}, nil
//	}
//
//	return patchOutcome(e)
//}
//
//func patchTimeExceedsConstant(e *event.EventLogEntry) (notificationGenerator, error) {
//	const name = "time-exceeds-n-constant"
//
//	data, ok := e.Data.(*PatchEventData)
//	if !ok {
//		return notificationGenerator{}, errors.New("unvalid event data")
//	}
//
//	if data.Status != evergreen.PatchSucceeded || data.Status != evergreen.PatchFailed {
//		return notificationGenerator{}, nil
//	}
//
//	return patchOutcome(e)
//}
//
//func patchTimeExceedsRelativePercent(e *event.EventLogEntry) (notificationGenerator, error) {
//	const name = "time-exceeds-n%"
//
//	data, ok := e.Data.(*PatchEventData)
//	if !ok {
//		return notificationGenerator{}, errors.New("unvalid event data")
//	}
//
//	if data.Status != evergreen.PatchSucceeded || data.Status != evergreen.PatchFailed {
//		return notificationGenerator{}, nil
//	}
//
//	return patchOutcome(e)
//}
