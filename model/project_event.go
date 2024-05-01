package model

import (
	"reflect"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type ProjectSettings struct {
	ProjectRef         ProjectRef           `bson:"proj_ref" json:"proj_ref"`
	GithubHooksEnabled bool                 `bson:"github_hooks_enabled" json:"github_hooks_enabled"`
	Vars               ProjectVars          `bson:"vars" json:"vars"`
	Aliases            []ProjectAlias       `bson:"aliases" json:"aliases"`
	Subscriptions      []event.Subscription `bson:"subscriptions" json:"subscriptions"`
	// HasGithubApp indicates if a user has saved an app key and secret for a github app
	HasGitHubApp bool `bson:"has_github_app" json:"has_github_app"`
}

type ProjectSettingsEvent struct {
	ProjectSettings `bson:",inline"`

	// The following boolean fields are flags that indicate that a given
	// field is nil instead of [], since this information is lost when
	// casting the event to a generic interface.
	GitTagAuthorizedTeamsDefault bool `bson:"git_tag_authorized_teams_default,omitempty" json:"git_tag_authorized_teams_default,omitempty"`
	GitTagAuthorizedUsersDefault bool `bson:"git_tag_authorized_users_default,omitempty" json:"git_tag_authorized_users_default,omitempty"`
	PatchTriggerAliasesDefault   bool `bson:"patch_trigger_aliases_default,omitempty" json:"patch_trigger_aliases_default,omitempty"`
	PeriodicBuildsDefault        bool `bson:"periodic_builds_default,omitempty" json:"periodic_builds_default,omitempty"`
	TriggersDefault              bool `bson:"triggers_default,omitempty" json:"triggers_default,omitempty"`
	WorkstationCommandsDefault   bool `bson:"workstation_commands_default,omitempty" json:"workstation_commands_default,omitempty"`
}

type ProjectChangeEvent struct {
	User   string               `bson:"user" json:"user"`
	Before ProjectSettingsEvent `bson:"before" json:"before"`
	After  ProjectSettingsEvent `bson:"after" json:"after"`
}

type ProjectChangeEvents []ProjectChangeEventEntry

// ApplyDefaults checks for any flags that indicate that a field in a project event should be nil and sets the field accordingly.
// Attached projects need to be able to distinguish between empty arrays and nil: nil values default to repo, while empty arrays do not.
// Look at the flags set in the ProjectSettingsEvent so that fields that were converted to empty arrays when casting to an interface{} can be correctly set to nil
func (p *ProjectChangeEvents) ApplyDefaults() {
	for _, event := range *p {
		changeEvent, isChangeEvent := event.Data.(*ProjectChangeEvent)
		if !isChangeEvent {
			continue
		}

		// Iterate through all flags for Before and After to properly
		// nullify fields.
		if changeEvent.Before.GitTagAuthorizedTeamsDefault {
			changeEvent.Before.ProjectRef.GitTagAuthorizedTeams = nil
		}
		if changeEvent.After.GitTagAuthorizedTeamsDefault {
			changeEvent.After.ProjectRef.GitTagAuthorizedTeams = nil
		}

		if changeEvent.Before.GitTagAuthorizedUsersDefault {
			changeEvent.Before.ProjectRef.GitTagAuthorizedUsers = nil
		}
		if changeEvent.After.GitTagAuthorizedUsersDefault {
			changeEvent.After.ProjectRef.GitTagAuthorizedUsers = nil
		}

		if changeEvent.Before.PatchTriggerAliasesDefault {
			changeEvent.Before.ProjectRef.PatchTriggerAliases = nil
		}
		if changeEvent.After.PatchTriggerAliasesDefault {
			changeEvent.After.ProjectRef.PatchTriggerAliases = nil
		}

		if changeEvent.Before.PeriodicBuildsDefault {
			changeEvent.Before.ProjectRef.PeriodicBuilds = nil
		}
		if changeEvent.After.PeriodicBuildsDefault {
			changeEvent.After.ProjectRef.PeriodicBuilds = nil
		}

		if changeEvent.Before.TriggersDefault {
			changeEvent.Before.ProjectRef.Triggers = nil
		}
		if changeEvent.After.TriggersDefault {
			changeEvent.After.ProjectRef.Triggers = nil
		}

		if changeEvent.Before.WorkstationCommandsDefault {
			changeEvent.Before.ProjectRef.WorkstationConfig.SetupCommands = nil
		}
		if changeEvent.After.WorkstationCommandsDefault {
			changeEvent.After.ProjectRef.WorkstationConfig.SetupCommands = nil
		}
	}

}

// RedactPrivateVars redacts private variables from the project modification event.
func (p *ProjectChangeEvents) RedactPrivateVars() {
	for _, event := range *p {
		changeEvent, isChangeEvent := event.Data.(*ProjectChangeEvent)
		if !isChangeEvent {
			continue
		}
		modifiedPrivateVarKeys := findModifiedPrivateVars(changeEvent)
		changeEvent.After.Vars = *changeEvent.After.Vars.RedactPrivateVars()
		changeEvent.Before.Vars = *changeEvent.Before.Vars.RedactPrivateVars()
		// Here we need to change the after value of modified private variables to
		// {REDACTED} so the event log will show that the private variable
		// has been changed in the UI. This is necessary because the above RedactPrivateVars
		// function redacts private variables to an empty string, which will not get picked up
		// by the UI as it relies on the before / after diff.
		for _, privateVarKey := range modifiedPrivateVarKeys {
			changeEvent.After.Vars.Vars[privateVarKey] = "{REDACTED_AFTER}"
			changeEvent.Before.Vars.Vars[privateVarKey] = "{REDACTED_BEFORE}"
		}
		event.EventLogEntry.Data = changeEvent
	}
}

// findModifiedPrivateVars returns a list of private variables that have had
// their values modified.
func findModifiedPrivateVars(changeEvent *ProjectChangeEvent) []string {
	modifiedPrivateVarKeys := []string{}
	for key, afterVal := range changeEvent.After.Vars.Vars {
		isPrivateVar := changeEvent.After.Vars.PrivateVars[key]
		beforeVal, exists := changeEvent.Before.Vars.Vars[key]
		if isPrivateVar && exists && afterVal != beforeVal {
			modifiedPrivateVarKeys = append(modifiedPrivateVarKeys, key)
		}
	}
	return modifiedPrivateVarKeys
}

type ProjectChangeEventEntry struct {
	event.EventLogEntry
}

func (e *ProjectChangeEventEntry) UnmarshalBSON(in []byte) error {
	return mgobson.Unmarshal(in, e)
}

func (e *ProjectChangeEventEntry) MarshalBSON() ([]byte, error) {
	return mgobson.Marshal(e)
}

func (e *ProjectChangeEventEntry) SetBSON(raw mgobson.Raw) error {
	temp := event.UnmarshalEventLogEntry{}
	if err := raw.Unmarshal(&temp); err != nil {
		return errors.Wrap(err, "unmarshalling event log entry")
	}

	e.Data = &ProjectChangeEvent{}

	if err := temp.Data.Unmarshal(e.Data); err != nil {
		return errors.Wrap(err, "unmarshalling event data")
	}

	// IDs for events were ObjectIDs previously, so we need to do this
	// TODO (EVG-17214): Remove once old events are TTLed and/or migrated.
	switch v := temp.ID.(type) {
	case string:
		e.ID = v
	case mgobson.ObjectId:
		e.ID = v.Hex()
	case primitive.ObjectID:
		e.ID = v.Hex()
	default:
		return errors.Errorf("unrecognized ID format for event %T", v)
	}
	e.Timestamp = temp.Timestamp
	e.ResourceId = temp.ResourceId
	e.EventType = temp.EventType
	e.ProcessedAt = temp.ProcessedAt
	e.ResourceType = temp.ResourceType

	return nil
}

// MostRecentProjectEvents returns the n most recent project events for the given project ID.
func MostRecentProjectEvents(id string, n int) (ProjectChangeEvents, error) {
	filter := event.ResourceTypeKeyIs(event.EventResourceTypeProject)
	filter[event.ResourceIdKey] = id

	query := db.Query(filter).Sort([]string{"-" + event.TimestampKey}).Limit(n)
	events := ProjectChangeEvents{}
	err := db.FindAllQ(event.EventCollection, query, &events)

	return events, err
}

// ProjectEventsBefore returns the n most recent project events for the given project ID
// that occurred before the given time.
func ProjectEventsBefore(id string, before time.Time, n int) (ProjectChangeEvents, error) {
	filter := event.ResourceTypeKeyIs(event.EventResourceTypeProject)
	filter[event.ResourceIdKey] = id
	filter[event.TimestampKey] = bson.M{
		"$lt": before,
	}

	query := db.Query(filter).Sort([]string{"-" + event.TimestampKey}).Limit(n)
	events := ProjectChangeEvents{}
	err := db.FindAllQ(event.EventCollection, query, &events)

	return events, err
}

// LogProjectEvent logs a project event.
func LogProjectEvent(eventType string, projectId string, eventData ProjectChangeEvent) error {
	projectEvent := event.EventLogEntry{
		Timestamp:    time.Now(),
		ResourceType: event.EventResourceTypeProject,
		EventType:    eventType,
		ResourceId:   projectId,
		Data:         eventData,
	}

	if err := projectEvent.Log(); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"resource_type": event.EventResourceTypeProject,
			"message":       "error logging event",
			"source":        "event-log-fail",
			"projectId":     projectId,
		}))
		return errors.Wrap(err, "logging project event")
	}

	return nil
}

// LogProjectAdded logs a project added event.
func LogProjectAdded(projectId, username string) error {
	return LogProjectEvent(event.EventTypeProjectAdded, projectId, ProjectChangeEvent{User: username})
}

// GetAndLogProjectModified retrieves the project settings before and after some change, and logs an event for the modification.
func GetAndLogProjectModified(id, userId string, isRepo bool, before *ProjectSettings) error {
	after, err := GetProjectSettingsById(id, isRepo)
	if err != nil {
		return errors.Wrap(err, "getting after project settings event")
	}
	return errors.Wrap(LogProjectModified(id, userId, before, after), "logging project modified")
}

// GetAndLogProjectRepoAttachment retrieves the project settings before and after the change, and logs the modification
// as a repo attachment/detachment event.
func GetAndLogProjectRepoAttachment(id, userId, attachmentType string, isRepo bool, before *ProjectSettings) error {
	after, err := GetProjectSettingsById(id, isRepo)
	if err != nil {
		return errors.Wrap(err, "getting after project settings event")
	}

	return errors.Wrap(LogProjectRepoAttachment(id, userId, attachmentType, before, after), "logging project repo attachment")
}

// resolveDefaults checks if certain project event fields are nil, and if so, sets the field's corresponding flag.
// ProjectChangeEvents must be cast to a generic interface to utilize event logging, which casts all nil objects of array types to empty arrays.
// Set flags if these values should indeed be nil so that we can correct these values when the event log is read from the database.
func (p *ProjectSettings) resolveDefaults() *ProjectSettingsEvent {
	projectSettingsEvent := &ProjectSettingsEvent{
		ProjectSettings: *p,
	}

	if p.ProjectRef.GitTagAuthorizedTeams == nil {
		projectSettingsEvent.GitTagAuthorizedTeamsDefault = true
	}
	if p.ProjectRef.GitTagAuthorizedUsers == nil {
		projectSettingsEvent.GitTagAuthorizedUsersDefault = true
	}
	if p.ProjectRef.PatchTriggerAliases == nil {
		projectSettingsEvent.PatchTriggerAliasesDefault = true
	}
	if p.ProjectRef.PeriodicBuilds == nil {
		projectSettingsEvent.PeriodicBuildsDefault = true
	}
	if p.ProjectRef.Triggers == nil {
		projectSettingsEvent.TriggersDefault = true
	}
	if p.ProjectRef.WorkstationConfig.SetupCommands == nil {
		projectSettingsEvent.WorkstationCommandsDefault = true
	}
	return projectSettingsEvent
}

// LogProjectModified logs an event for a modification of a project's settings.
func LogProjectModified(projectId, username string, before, after *ProjectSettings) error {
	eventData := constructProjectChangeEvent(username, before, after)
	if eventData == nil {
		return nil
	}
	return LogProjectEvent(event.EventTypeProjectModified, projectId, *eventData)
}

// LogProjectRepoAttachment logs an event for either the attachment of a project to a repo,
// or a detachment of a project from a repo.
func LogProjectRepoAttachment(projectId, username, attachmentType string, before, after *ProjectSettings) error {
	eventData := constructProjectChangeEvent(username, before, after)
	return LogProjectEvent(attachmentType, projectId, *eventData)
}

func constructProjectChangeEvent(username string, before, after *ProjectSettings) *ProjectChangeEvent {
	if before == nil || after == nil {
		return nil
	}
	// Stop if there are no changes
	if reflect.DeepEqual(*before, *after) {
		return nil
	}

	eventData := ProjectChangeEvent{
		User:   username,
		Before: *before.resolveDefaults(),
		After:  *after.resolveDefaults(),
	}
	return &eventData
}
