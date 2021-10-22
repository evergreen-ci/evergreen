package trigger

import (
	"regexp"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
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
func NotificationsFromEvent(e *event.EventLogEntry) ([]notification.Notification, error) {
	h := registry.eventHandler(e.ResourceType, e.EventType)
	if h == nil {
		return nil, errors.Errorf("unknown event ResourceType '%s' or EventType '%s'", e.ResourceType, e.EventType)
	}

	if err := h.Fetch(e); err != nil {
		return nil, errors.Wrapf(err, "error fetching data for event: %s (%s, %s)", e.ID, e.ResourceType, e.EventType)
	}

	subscriptions, err := event.FindSubscriptions(e.ResourceType, h.Selectors())
	msg := message.Fields{
		"source":              "events-processing",
		"message":             "processing event",
		"event_id":            e.ID,
		"event_type":          e.EventType,
		"event_resource_type": e.ResourceType,
		"event_resource":      e.ResourceId,
		"num_subscriptions":   len(subscriptions),
	}
	if err != nil {
		err = errors.Wrapf(err, "error fetching subscriptions for event: %s (%s, %s)", e.ID, e.ResourceType, e.EventType)
		grip.Error(message.WrapError(err, msg))
		return nil, err
	}
	grip.Info(msg)
	if len(subscriptions) == 0 {
		return nil, nil
	}

	notifications := make([]notification.Notification, 0, len(subscriptions))

	catcher := grip.NewSimpleCatcher()
	for i := range subscriptions {
		n, err := h.Process(&subscriptions[i])
		msg := message.Fields{
			"source":              "events-processing",
			"message":             "processing subscription",
			"event_id":            e.ID,
			"event_type":          e.EventType,
			"event_resource_type": e.ResourceType,
			"event_resource":      e.ResourceId,
			"subscription_id":     subscriptions[i].ID,
			"notification_is_nil": n == nil,
		}
		if n != nil {
			msg["notification_id"] = n.ID
		}
		catcher.Add(err)
		grip.Error(message.WrapError(err, msg))
		grip.InfoWhen(err == nil, msg)
		if n == nil {
			continue
		}

		notifications = append(notifications, *n)
	}

	return notifications, catcher.Resolve()
}

type projectProcessor func(ProcessorArgs) (*model.Version, error)

type ProcessorArgs struct {
	SourceVersion     *model.Version
	DownstreamProject model.ProjectRef
	ConfigFile        string
	Command           string
	GenerateFile      string
	TriggerID         string
	TriggerType       string
	EventID           string
	DefinitionID      string
	Alias             string
}

// EvalProjectTriggers takes an event log entry and a processor (either the mock or TriggerDownstreamVersion)
// and checks if any downstream builds should be triggered, creating them if they should
func EvalProjectTriggers(e *event.EventLogEntry, processor projectProcessor) ([]model.Version, error) {
	switch e.EventType {
	case event.TaskFinished:
		t, err := task.FindOneId(e.ResourceId)
		if err != nil {
			return nil, errors.Wrap(err, "error finding task")
		}
		if t == nil {
			return nil, errors.Errorf("task '%s' not found", e.ResourceId)
		}
		return triggerDownstreamProjectsForTask(t, e, processor)
	case event.BuildStateChange:
		if e.ResourceType != event.ResourceTypeBuild {
			return nil, nil
		}
		data, ok := e.Data.(*event.BuildEventData)
		if !ok {
			return nil, errors.Errorf("unable to convert %#v to BuildEventData", e.Data)
		}
		if data.Status != evergreen.BuildFailed && data.Status != evergreen.BuildSucceeded {
			return nil, nil
		}
		b, err := build.FindOneId(e.ResourceId)
		if err != nil {
			return nil, errors.Wrap(err, "error finding build")
		}
		if b == nil {
			return nil, errors.Errorf("build '%s' not found", e.ResourceId)
		}
		return triggerDownstreamProjectsForBuild(b, e, processor)
	default:
		return nil, nil
	}
}

func triggerDownstreamProjectsForTask(t *task.Task, e *event.EventLogEntry, processor projectProcessor) ([]model.Version, error) {
	if t.Requester != evergreen.RepotrackerVersionRequester {
		return nil, nil
	}
	downstreamProjects, err := model.FindDownstreamProjects(t.Project)
	if err != nil {
		return nil, errors.Wrap(err, "error finding project ref")
	}
	sourceVersion, err := model.VersionFindOneId(t.Version)
	if err != nil {
		return nil, errors.Wrap(err, "error finding version")
	}

	catcher := grip.NewBasicCatcher()
	versions := []model.Version{}
projectLoop:
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
				SourceVersion:     sourceVersion,
				DownstreamProject: ref,
				ConfigFile:        trigger.ConfigFile,
				Command:           trigger.Command,
				GenerateFile:      trigger.GenerateFile,
				TriggerType:       model.ProjectTriggerLevelTask,
				TriggerID:         t.Id,
				EventID:           e.ID,
				DefinitionID:      trigger.DefinitionID,
				Alias:             trigger.Alias,
			}
			v, err := processor(args)
			if err != nil {
				catcher.Add(err)
				continue
			}
			if v != nil {
				versions = append(versions, *v)
			}
			continue projectLoop
		}
	}

	return versions, catcher.Resolve()
}

func triggerDownstreamProjectsForBuild(b *build.Build, e *event.EventLogEntry, processor projectProcessor) ([]model.Version, error) {
	if b.Requester != evergreen.RepotrackerVersionRequester {
		return nil, nil
	}
	downstreamProjects, err := model.FindDownstreamProjects(b.Project)
	if err != nil {
		return nil, errors.Wrap(err, "error finding project ref")
	}
	sourceVersion, err := model.VersionFindOneId(b.Version)
	if err != nil {
		return nil, errors.Wrap(err, "error finding version")
	}

	catcher := grip.NewBasicCatcher()
	versions := []model.Version{}
projectLoop:
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
			if trigger.BuildVariantRegex != "" {
				regex, err := regexp.Compile(trigger.BuildVariantRegex)
				if err != nil {
					catcher.Add(err)
					continue
				}
				if !regex.MatchString(b.BuildVariant) {
					continue
				}
			}

			args := ProcessorArgs{
				SourceVersion:     sourceVersion,
				DownstreamProject: ref,
				ConfigFile:        trigger.ConfigFile,
				Command:           trigger.Command,
				TriggerType:       model.ProjectTriggerLevelBuild,
				TriggerID:         b.Id,
				EventID:           e.ID,
				DefinitionID:      trigger.DefinitionID,
				Alias:             trigger.Alias,
			}
			v, err := processor(args)
			if err != nil {
				catcher.Add(err)
				continue
			}
			if v != nil {
				versions = append(versions, *v)
			}
			continue projectLoop
		}
	}

	return versions, catcher.Resolve()
}
