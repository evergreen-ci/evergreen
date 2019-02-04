package trigger

import (
	"fmt"
	"strconv"
	"time"

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
	objectBuild = "build"
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
		triggerOutcome:                t.buildOutcome,
		triggerFailure:                t.buildFailure,
		triggerSuccess:                t.buildSuccess,
		triggerExceedsDuration:        t.buildExceedsDuration,
		triggerRuntimeChangeByPercent: t.buildRuntimeChange,
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
		return errors.Errorf("build '%s' contains unexpected data with type '%T'", e.ResourceId, e.Data)
	}
	t.event = e

	return nil
}

func (t *buildTriggers) Selectors() []event.Selector {
	selectors := []event.Selector{
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
		{
			Type: selectorDisplayName,
			Data: t.build.DisplayName,
		},
		{
			Type: selectorBuildVariant,
			Data: t.build.BuildVariant,
		},
	}
	if t.build.Requester == evergreen.TriggerRequester {
		selectors = append(selectors, event.Selector{
			Type: selectorRequester,
			Data: evergreen.RepotrackerVersionRequester,
		})
	}
	return selectors
}

func (t *buildTriggers) buildOutcome(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.BuildSucceeded && t.data.Status != evergreen.BuildFailed {
		return nil, nil
	}

	return t.generate(sub, "")
}

func (t *buildTriggers) buildFailure(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.BuildFailed {
		return nil, nil
	}

	return t.generate(sub, "")
}

func (t *buildTriggers) buildSuccess(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.BuildSucceeded {
		return nil, nil
	}

	return t.generate(sub, "")
}

func (t *buildTriggers) buildExceedsDuration(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.BuildSucceeded && t.data.Status != evergreen.BuildFailed {
		return nil, nil
	}
	thresholdString, ok := sub.TriggerData[event.BuildDurationKey]
	if !ok {
		return nil, fmt.Errorf("subscription %s has no build time threshold", sub.ID)
	}
	threshold, err := strconv.Atoi(thresholdString)
	if err != nil {
		return nil, fmt.Errorf("subscription %s has an invalid time threshold", sub.ID)
	}

	maxDuration := time.Duration(threshold) * time.Second
	if t.build.TimeTaken < maxDuration {
		return nil, nil
	}
	return t.generate(sub, fmt.Sprintf("exceeded %d seconds", threshold))
}

func (t *buildTriggers) buildRuntimeChange(sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.BuildSucceeded && t.data.Status != evergreen.BuildFailed {
		return nil, nil
	}
	percentString, ok := sub.TriggerData[event.BuildPercentChangeKey]
	if !ok {
		return nil, fmt.Errorf("subscription %s has no percentage increase", sub.ID)
	}
	percent, err := strconv.ParseFloat(percentString, 64)
	if err != nil {
		return nil, fmt.Errorf("subscription %s has an invalid percentage", sub.ID)
	}

	lastGreen, err := t.build.PreviousSuccessful()
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving last green build")
	}
	if lastGreen == nil {
		return nil, nil
	}
	thisBuildDuration := float64(t.build.TimeTaken)
	prevBuildDuration := float64(lastGreen.TimeTaken)
	shouldNotify, percentChange := runtimeExceedsThreshold(percent, prevBuildDuration, thisBuildDuration)
	if !shouldNotify {
		return nil, nil
	}
	return t.generate(sub, fmt.Sprintf("changed in runtime by %.1f%% (over threshold of %s%%)", percentChange, percentString))
}

func (t *buildTriggers) makeData(sub *event.Subscription, pastTenseOverride string) (*commonTemplateData, error) {
	api := restModel.APIBuild{}
	if err := api.BuildFromService(*t.build); err != nil {
		return nil, errors.Wrap(err, "error building json model")
	}

	data := commonTemplateData{
		ID:              t.build.Id,
		EventID:         t.event.ID,
		SubscriptionID:  sub.ID,
		DisplayName:     t.build.DisplayName,
		Object:          objectBuild,
		Project:         t.build.Project,
		URL:             buildLink(t.uiConfig.Url, t.build.Id),
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
	if pastTenseOverride != "" {
		data.PastTenseStatus = pastTenseOverride
	}
	data.slack = t.buildAttachments(&data)

	return &data, nil
}

func (t *buildTriggers) buildAttachments(data *commonTemplateData) []message.SlackAttachment {
	attachments := []message.SlackAttachment{}

	attachments = append(attachments, message.SlackAttachment{
		Title:     fmt.Sprintf("Build: %s", t.build.DisplayName),
		TitleLink: data.URL,
		Text:      taskStatusToDesc(t.build),
	})
	if t.data.Status == evergreen.BuildSucceeded {
		attachments[0].Color = evergreenSuccessColor
	} else {
		attachments[0].Color = evergreenFailColor
	}

	attachmentsCount := 0
	for i := range t.build.Tasks {
		if attachmentsCount == slackAttachmentsLimit {
			break
		}
		if t.build.Tasks[i].Status == evergreen.TaskSucceeded {
			continue
		}
		attachments = append(attachments, message.SlackAttachment{
			Title:     fmt.Sprintf("Task: %s", t.build.Tasks[i].DisplayName),
			TitleLink: taskLink(t.uiConfig.Url, t.build.Tasks[i].Id, -1),
			Color:     evergreenFailColor,
			Text:      taskFormatFromCache(&t.build.Tasks[i]),
		})
		attachmentsCount++
	}

	return attachments
}

func (t *buildTriggers) generate(sub *event.Subscription, pastTenseOverride string) (*notification.Notification, error) {
	data, err := t.makeData(sub, pastTenseOverride)
	if err != nil {
		return nil, errors.Wrap(err, "failed to collect build data")
	}

	payload, err := makeCommonPayload(sub, t.Selectors(), data)
	if err != nil {
		return nil, errors.Wrap(err, "failed to build notification")
	}

	return notification.New(t.event.ID, sub.Trigger, &sub.Subscriber, payload)
}

func taskFormatFromCache(t *build.TaskCache) string {
	if t.Status == evergreen.TaskSucceeded {
		return fmt.Sprintf("took %s", t.TimeTaken)
	}

	return fmt.Sprintf("took %s, the task failed %s", t.TimeTaken, detailStatusToHumanSpeak(t.StatusDetails.Status))
}
