package trigger

import (
	"context"
	"regexp"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v52/github"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// NotificationsFromEvent takes an event, processes all of its triggers, and returns
// a slice of notifications, and an error object representing all errors
// that occurred while processing triggers

// It is possible for this function to return notifications and errors at the
// same time. If the notifications array is not nil, they are valid and should
// be processed as normal.
func NotificationsFromEvent(ctx context.Context, e *event.EventLogEntry) ([]notification.Notification, error) {
	h := registry.eventHandler(e.ResourceType, e.EventType)
	if h == nil {
		return nil, errors.Errorf("unknown event resource type '%s' or event type '%s'", e.ResourceType, e.EventType)
	}

	if err := h.Fetch(ctx, e); err != nil {
		return nil, errors.Wrapf(err, "fetching data for event '%s' (resource type: '%s', event type: '%s')", e.ID, e.ResourceType, e.EventType)
	}

	subscriptions, err := event.FindSubscriptionsByAttributes(e.ResourceType, h.Attributes())
	subIDs := make([]string, 0, len(subscriptions))
	for _, sub := range subscriptions {
		subIDs = append(subIDs, sub.ID)
	}
	msg := message.Fields{
		"source":            "events-processing",
		"message":           "processing event",
		"event_id":          e.ID,
		"event_type":        e.EventType,
		"resource_id":       e.ResourceId,
		"resource_type":     e.ResourceType,
		"num_subscriptions": len(subscriptions),
		"subscription_ids":  subIDs,
	}
	if err != nil {
		err = errors.Wrapf(err, "fetching subscriptions for event '%s' (resource type: '%s', event type: '%s')", e.ID, e.ResourceType, e.EventType)
		grip.Error(message.WrapError(err, msg))
		return nil, err
	}
	if len(subscriptions) == 0 {
		return nil, nil
	}
	grip.Info(msg)

	notifications := make([]notification.Notification, 0, len(subscriptions))

	catcher := grip.NewSimpleCatcher()
	for i := range subscriptions {
		n, err := h.Process(ctx, &subscriptions[i])
		msg := message.Fields{
			"source":              "events-processing",
			"message":             "processed subscription and created notifications",
			"event_id":            e.ID,
			"event_type":          e.EventType,
			"event_resource_type": e.ResourceType,
			"event_resource":      e.ResourceId,
			"subscription_id":     subscriptions[i].ID,
		}
		if n != nil {
			msg["notification_id"] = n.ID
		}
		catcher.Add(err)
		grip.Error(message.WrapError(err, msg))
		if n == nil {
			continue
		}
		grip.Info(msg)

		notifications = append(notifications, *n)
	}

	return notifications, catcher.Resolve()
}

type projectProcessor func(context.Context, ProcessorArgs) (*model.Version, error)

type ProcessorArgs struct {
	SourceVersion                *model.Version
	DownstreamProject            model.ProjectRef
	ConfigFile                   string
	TriggerID                    string
	TriggerType                  string
	EventID                      string
	DefinitionID                 string
	Alias                        string
	UnscheduleDownstreamVersions bool
	PushRevision                 model.Revision
}

// EvalProjectTriggers takes an event log entry and a processor (either the mock or TriggerDownstreamVersion)
// and checks if any downstream builds should be triggered, creating them if they should
func EvalProjectTriggers(ctx context.Context, e *event.EventLogEntry, processor projectProcessor) ([]model.Version, error) {
	switch e.EventType {
	case event.TaskFinished:
		t, err := task.FindOneId(e.ResourceId)
		if err != nil {
			return nil, errors.Wrapf(err, "finding task '%s'", e.ResourceId)
		}
		if t == nil {
			return nil, errors.Errorf("task '%s' not found", e.ResourceId)
		}
		return triggerDownstreamProjectsForTask(ctx, t, e, processor)
	case event.BuildStateChange:
		if e.ResourceType != event.ResourceTypeBuild {
			return nil, nil
		}
		data, ok := e.Data.(*event.BuildEventData)
		if !ok {
			return nil, errors.Errorf("unable to convert %T to BuildEventData", e.Data)
		}
		if data.Status != evergreen.BuildFailed && data.Status != evergreen.BuildSucceeded {
			return nil, nil
		}
		b, err := build.FindOneId(e.ResourceId)
		if err != nil {
			return nil, errors.Wrapf(err, "finding build '%s'", e.ResourceId)
		}
		if b == nil {
			return nil, errors.Errorf("build '%s' not found", e.ResourceId)
		}
		return triggerDownstreamProjectsForBuild(ctx, b, e, processor)
	default:
		return nil, nil
	}
}

func triggerDownstreamProjectsForTask(ctx context.Context, t *task.Task, e *event.EventLogEntry, processor projectProcessor) ([]model.Version, error) {
	if t.Requester != evergreen.RepotrackerVersionRequester {
		return nil, nil
	}
	downstreamProjects, err := model.FindDownstreamProjects(t.Project)
	if err != nil {
		return nil, errors.Wrapf(err, "finding downstream projects of project '%s'", t.Project)
	}
	sourceVersion, err := model.VersionFindOneId(t.Version)
	if err != nil {
		return nil, errors.Wrapf(err, "finding source version '%s'", t.Version)
	}
	if sourceVersion == nil {
		return nil, errors.Errorf("source version '%s' not found", t.Version)
	}

	catcher := grip.NewBasicCatcher()
	versions := []model.Version{}

	for _, ref := range downstreamProjects {

		for _, trigger := range ref.Triggers {
			if trigger.Level != model.ProjectTriggerLevelTask {
				continue
			}
			if trigger.Project != t.Project {
				continue
			}
			if trigger.Status != "" && trigger.Status != t.Status {
				continue
			}
			if trigger.DateCutoff != nil && time.Now().Add(-24*time.Duration(*trigger.DateCutoff)*time.Hour).After(t.IngestTime) {
				continue
			}
			if utility.StringSliceContains(sourceVersion.SatisfiedTriggers, trigger.DefinitionID) {
				continue
			}
			if trigger.TaskRegex != "" {
				regex, err := regexp.Compile(trigger.TaskRegex)
				if err != nil {
					catcher.Add(err)
					continue
				}
				if !regex.MatchString(t.DisplayName) {
					continue
				}
			}
			if trigger.BuildVariantRegex != "" {
				regex, err := regexp.Compile(trigger.BuildVariantRegex)
				if err != nil {
					catcher.Add(err)
					continue
				}
				if !regex.MatchString(t.BuildVariant) {
					continue
				}
			}

			args := ProcessorArgs{
				SourceVersion:                sourceVersion,
				DownstreamProject:            ref,
				ConfigFile:                   trigger.ConfigFile,
				TriggerType:                  model.ProjectTriggerLevelTask,
				TriggerID:                    t.Id,
				EventID:                      e.ID,
				DefinitionID:                 trigger.DefinitionID,
				Alias:                        trigger.Alias,
				UnscheduleDownstreamVersions: trigger.UnscheduleDownstreamVersions,
			}
			v, err := processor(ctx, args)
			if err != nil {
				catcher.Add(err)
				continue
			}
			if v != nil {
				versions = append(versions, *v)
			}
			break
		}
	}

	return versions, catcher.Resolve()
}

func triggerDownstreamProjectsForBuild(ctx context.Context, b *build.Build, e *event.EventLogEntry, processor projectProcessor) ([]model.Version, error) {
	if b.Requester != evergreen.RepotrackerVersionRequester {
		return nil, nil
	}
	downstreamProjects, err := model.FindDownstreamProjects(b.Project)
	if err != nil {
		return nil, errors.Wrapf(err, "finding downstream projects of project '%s'", b.Project)
	}
	sourceVersion, err := model.VersionFindOneId(b.Version)
	if err != nil {
		return nil, errors.Wrapf(err, "finding source version '%s'", b.Version)
	}
	if sourceVersion == nil {
		return nil, errors.Errorf("source version '%s' not found", b.Version)
	}

	catcher := grip.NewBasicCatcher()
	versions := []model.Version{}

	for _, ref := range downstreamProjects {
		for _, trigger := range ref.Triggers {
			if trigger.Level != model.ProjectTriggerLevelBuild {
				continue
			}
			if trigger.Project != b.Project {
				continue
			}
			if trigger.Status != "" && trigger.Status != b.Status {
				continue
			}
			if utility.StringSliceContains(sourceVersion.SatisfiedTriggers, trigger.DefinitionID) {
				continue
			}
			if trigger.BuildVariantRegex != "" {
				regex, err := regexp.Compile(trigger.BuildVariantRegex)
				if err != nil {
					catcher.Wrapf(err, "compiling build variant regexp '%s'", trigger.BuildVariantRegex)
					continue
				}
				if !regex.MatchString(b.BuildVariant) {
					continue
				}
			}

			args := ProcessorArgs{
				SourceVersion:                sourceVersion,
				DownstreamProject:            ref,
				ConfigFile:                   trigger.ConfigFile,
				TriggerType:                  model.ProjectTriggerLevelBuild,
				TriggerID:                    b.Id,
				EventID:                      e.ID,
				DefinitionID:                 trigger.DefinitionID,
				Alias:                        trigger.Alias,
				UnscheduleDownstreamVersions: trigger.UnscheduleDownstreamVersions,
			}
			v, err := processor(ctx, args)
			if err != nil {
				catcher.Add(err)
				continue
			}
			if v != nil {
				versions = append(versions, *v)
			}
			break
		}
	}

	return versions, catcher.Resolve()
}

// TriggerDownstreamProjectsForPush triggers downstream projects for a push event from a repo that does not
// have repotracker enabled.
func TriggerDownstreamProjectsForPush(ctx context.Context, projectId string, event *github.PushEvent, processor projectProcessor) error {
	downstreamProjects, err := model.FindDownstreamProjects(projectId)
	if err != nil {
		return errors.Wrapf(err, "finding downstream projects of project '%s'", projectId)
	}

	catcher := grip.NewBasicCatcher()
	versionIds := []string{}
	for _, ref := range downstreamProjects {

		for _, trigger := range ref.Triggers {
			if trigger.Level != model.ProjectTriggerLevelPush || trigger.Project != projectId {
				continue
			}

			args := ProcessorArgs{
				DownstreamProject:            ref,
				ConfigFile:                   trigger.ConfigFile,
				TriggerType:                  model.ProjectTriggerLevelPush,
				TriggerID:                    trigger.Project,
				DefinitionID:                 trigger.DefinitionID,
				Alias:                        trigger.Alias,
				UnscheduleDownstreamVersions: trigger.UnscheduleDownstreamVersions,
				PushRevision: model.Revision{
					Revision:        utility.FromStringPtr(event.GetHeadCommit().ID),
					CreateTime:      event.GetHeadCommit().Timestamp.Time,
					Author:          utility.FromStringPtr(event.GetHeadCommit().Author.Name),
					AuthorEmail:     utility.FromStringPtr(event.GetHeadCommit().Author.Email),
					RevisionMessage: utility.FromStringPtr(event.GetHeadCommit().Message),
				},
			}
			v, err := processor(ctx, args)
			if err != nil {
				catcher.Add(err)
				continue
			}
			if v != nil {
				versionIds = append(versionIds, v.Id)
			}
			break
		}
	}
	grip.InfoWhen(len(versionIds) > 0, message.Fields{
		"source":      "GitHub hook",
		"message":     "triggered versions for push event for project",
		"version_ids": versionIds,
		"project_id":  projectId,
	})

	return catcher.Resolve()
}
