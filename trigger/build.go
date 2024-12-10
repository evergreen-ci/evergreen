package trigger

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/model/task"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

func init() {
	registry.registerEventHandler(event.ResourceTypeBuild, event.BuildStateChange, makeBuildTriggers)
	registry.registerEventHandler(event.ResourceTypeBuild, event.BuildGithubCheckFinished, makeBuildTriggers)
}

type buildTriggers struct {
	event    *event.EventLogEntry
	data     *event.BuildEventData
	build    *build.Build
	tasks    []task.Task
	uiConfig evergreen.UIConfig

	base
}

func makeBuildTriggers() eventHandler {
	t := &buildTriggers{}
	t.base.triggers = map[string]trigger{
		event.TriggerOutcome:                t.buildOutcome,
		event.TriggerGithubCheckOutcome:     t.buildGithubCheckOutcome,
		event.TriggerFailure:                t.buildFailure,
		event.TriggerSuccess:                t.buildSuccess,
		event.TriggerExceedsDuration:        t.buildExceedsDuration,
		event.TriggerRuntimeChangeByPercent: t.buildRuntimeChange,
	}
	return t
}

func (t *buildTriggers) Fetch(ctx context.Context, e *event.EventLogEntry) error {
	var err error
	if err = t.uiConfig.Get(ctx); err != nil {
		return errors.Wrap(err, "fetching UI config")
	}

	t.build, err = build.FindOne(build.ById(e.ResourceId))
	if err != nil {
		return errors.Wrapf(err, "finding build '%s'", e.ResourceId)
	}
	if t.build == nil {
		return errors.Errorf("build '%s' not found", e.ResourceId)
	}

	var tasks []task.Task
	if e.EventType == event.BuildGithubCheckFinished {
		query := db.Query(task.ByBuildIdAndGithubChecks(t.build.Id))
		tasks, err = task.FindAll(query)
		if err != nil {
			return errors.Wrapf(err, "finding tasks in build '%s' for GitHub check", t.build.Id)
		}
	} else {
		query := db.Query(task.ByBuildId(t.build.Id))
		tasks, err = task.FindAll(query)
		if err != nil {
			return errors.Wrapf(err, "finding tasks in build '%s'", t.build.Id)
		}
	}

	taskMap := task.TaskSliceToMap(tasks)
	for _, taskCache := range t.build.Tasks {
		dbTask, ok := taskMap[taskCache.Id]
		if !ok {
			continue
		}
		t.tasks = append(t.tasks, dbTask)
	}

	var ok bool
	t.data, ok = e.Data.(*event.BuildEventData)
	if !ok {
		return errors.Errorf("build '%s' contains unexpected data with type '%T'", e.ResourceId, e.Data)
	}
	t.event = e

	return nil
}

func (t *buildTriggers) Attributes() event.Attributes {
	attributes := event.Attributes{
		ID:           []string{t.build.Id},
		Object:       []string{event.ObjectBuild},
		Project:      []string{t.build.Project},
		Requester:    []string{t.build.Requester},
		InVersion:    []string{t.build.Version},
		DisplayName:  []string{t.build.DisplayName},
		BuildVariant: []string{t.build.BuildVariant},
	}

	if t.build.Requester == evergreen.TriggerRequester {
		attributes.Requester = append(attributes.Requester, evergreen.RepotrackerVersionRequester)
	}
	return attributes
}

func (t *buildTriggers) buildGithubCheckOutcome(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if t.data.GithubCheckStatus != evergreen.BuildSucceeded && t.data.GithubCheckStatus != evergreen.BuildFailed {
		return nil, nil
	}
	return t.generate(sub, "")
}

func (t *buildTriggers) buildOutcome(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.BuildSucceeded && t.data.Status != evergreen.BuildFailed {
		return nil, nil
	}

	return t.generate(sub, "")
}

func (t *buildTriggers) buildFailure(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.BuildFailed {
		return nil, nil
	}

	return t.generate(sub, "")
}

func (t *buildTriggers) buildSuccess(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.BuildSucceeded {
		return nil, nil
	}

	return t.generate(sub, "")
}

func (t *buildTriggers) buildExceedsDuration(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.BuildSucceeded && t.data.Status != evergreen.BuildFailed {
		return nil, nil
	}
	thresholdString, ok := sub.TriggerData[event.BuildDurationKey]
	if !ok {
		return nil, errors.Errorf("subscription '%s' has no build time threshold", sub.ID)
	}
	threshold, err := strconv.Atoi(thresholdString)
	if err != nil {
		return nil, errors.Errorf("subscription '%s' has an invalid time threshold", sub.ID)
	}

	maxDuration := time.Duration(threshold) * time.Second
	if t.build.TimeTaken < maxDuration {
		return nil, nil
	}
	return t.generate(sub, fmt.Sprintf("exceeded %d seconds", threshold))
}

func (t *buildTriggers) buildRuntimeChange(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if t.data.Status != evergreen.BuildSucceeded && t.data.Status != evergreen.BuildFailed {
		return nil, nil
	}
	percentString, ok := sub.TriggerData[event.BuildPercentChangeKey]
	if !ok {
		return nil, errors.Errorf("subscription '%s' has no percentage increase", sub.ID)
	}
	percent, err := strconv.ParseFloat(percentString, 64)
	if err != nil {
		return nil, errors.Wrapf(err, "subscription '%s' has an invalid percentage", sub.ID)
	}

	lastGreen, err := t.build.PreviousSuccessful()
	if err != nil {
		return nil, errors.Wrap(err, "retrieving last green build")
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
	api.BuildFromService(*t.build, nil)
	projectName := t.build.Project
	if api.ProjectIdentifier != nil {
		projectName = utility.FromStringPtr(api.ProjectIdentifier)
	}

	data := commonTemplateData{
		ID:              t.build.Id,
		EventID:         t.event.ID,
		SubscriptionID:  sub.ID,
		DisplayName:     t.build.DisplayName,
		Object:          event.ObjectBuild,
		Project:         projectName,
		URL:             t.build.GetURL(t.uiConfig.Url),
		PastTenseStatus: t.data.Status,
		apiModel:        &api,
	}

	if t.data.GithubCheckStatus != "" {
		data.PastTenseStatus = t.data.GithubCheckStatus
	}
	if t.build.Requester == evergreen.GithubPRRequester || t.build.Requester == evergreen.RepotrackerVersionRequester || t.build.Requester == evergreen.GithubMergeRequester {
		data.githubContext = fmt.Sprintf("evergreen/%s", t.build.BuildVariant)
		data.githubDescription = t.build.GetPRNotificationDescription(t.tasks)
	}
	if data.PastTenseStatus == evergreen.BuildFailed {
		data.githubState = message.GithubStateFailure
	}
	if data.PastTenseStatus == evergreen.BuildSucceeded {
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
	hasPatch := evergreen.IsPatchRequester(t.build.Requester)
	attachments := []message.SlackAttachment{}
	attachments = append(attachments, message.SlackAttachment{
		Title:     fmt.Sprintf("Build: %s", t.build.DisplayName),
		TitleLink: data.URL,
		Text:      t.build.GetPRNotificationDescription(t.tasks),
		Fields: []*message.SlackAttachmentField{
			{
				Title: "Version",
				Value: fmt.Sprintf("<%s|%s>", versionLink(
					versionLinkInput{
						uiBase:    t.uiConfig.Url,
						versionID: t.build.Version,
						hasPatch:  hasPatch,
						isChild:   false,
					},
				),
					t.build.Version,
				),
			},
			{
				Title: "Makespan",
				Value: t.build.ActualMakespan.String(),
			},
			{
				Title: "Duration",
				Value: t.build.TimeTaken.String(),
			},
		},
	})
	if t.data.Status == evergreen.BuildSucceeded {
		attachments[0].Color = evergreenSuccessColor
	} else {
		attachments[0].Color = evergreenFailColor
	}

	attachmentsCount := 0
	for i := range t.tasks {
		if attachmentsCount == slackAttachmentsLimit {
			break
		}
		if t.tasks[i].Status == evergreen.TaskSucceeded || t.tasks[i].IsUnscheduled() {
			continue
		}
		attachments = append(attachments, message.SlackAttachment{
			Title:     fmt.Sprintf("Task: %s", t.tasks[i].DisplayName),
			TitleLink: taskLink(t.uiConfig.Url, t.tasks[i].Id, -1),
			Color:     evergreenFailColor,
			Text:      taskFormatFromCache(t.tasks[i]),
			Fields: []*message.SlackAttachmentField{
				{
					Title: "Duration",
					Value: t.tasks[i].TimeTaken.String(),
				},
			},
		})
		attachmentsCount++
	}

	return attachments
}

func (t *buildTriggers) generate(sub *event.Subscription, pastTenseOverride string) (*notification.Notification, error) {
	data, err := t.makeData(sub, pastTenseOverride)
	if err != nil {
		return nil, errors.Wrap(err, "collecting build data")
	}

	payload, err := makeCommonPayload(sub, t.Attributes(), data)
	if err != nil {
		return nil, errors.Wrap(err, "building notification")
	}

	return notification.New(t.event.ID, sub.Trigger, &sub.Subscriber, payload)
}

func taskFormatFromCache(t task.Task) string {
	if t.Status == evergreen.TaskSucceeded {
		return fmt.Sprintf("took %s", t.TimeTaken)
	}

	return fmt.Sprintf("took %s, the task failed %s", t.TimeTaken, detailStatusToHumanSpeak(t.GetDisplayStatus()))
}
