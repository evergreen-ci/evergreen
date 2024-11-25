package model

import (
	"bytes"
	"reflect"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/githubapp"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type ProjectSettings struct {
	ProjectRef         ProjectRef              `bson:"proj_ref" json:"proj_ref"`
	GitHubAppAuth      githubapp.GithubAppAuth `bson:"github_app_auth" json:"github_app_auth"`
	GithubHooksEnabled bool                    `bson:"github_hooks_enabled" json:"github_hooks_enabled"`
	Vars               ProjectVars             `bson:"vars" json:"vars"`
	Aliases            []ProjectAlias          `bson:"aliases" json:"aliases"`
	Subscriptions      []event.Subscription    `bson:"subscriptions" json:"subscriptions"`
}

// NewProjectSettingsFromEvent creates project settings from a project settings
// event.
func NewProjectSettingsFromEvent(e ProjectSettingsEvent) ProjectSettings {
	return ProjectSettings{
		ProjectRef: e.ProjectRef,
		GitHubAppAuth: githubapp.GithubAppAuth{
			AppID:      e.GitHubAppAuth.AppID,
			PrivateKey: e.GitHubAppAuth.PrivateKey,
		},
		GithubHooksEnabled: e.GithubHooksEnabled,
		Vars: ProjectVars{
			Vars:          e.Vars.Vars,
			PrivateVars:   e.Vars.PrivateVars,
			AdminOnlyVars: e.Vars.AdminOnlyVars,
		},
		Aliases:       e.Aliases,
		Subscriptions: e.Subscriptions,
	}
}

// ProjectEventVars contains the project variable data relevant to project
// modification events.
type ProjectEventVars struct {
	// Vars contain the names of project variables and redacted placeholders for
	// their values.
	Vars          map[string]string `bson:"vars" json:"vars"`
	PrivateVars   map[string]bool   `bson:"private_vars" json:"private_vars"`
	AdminOnlyVars map[string]bool   `bson:"admin_only_vars" json:"admin_only_vars"`
}

// ProjectEventGitHubAppAuth contains the GitHub app auth data relevant to
// project modification events.
type ProjectEventGitHubAppAuth struct {
	AppID int64 `bson:"app_id" json:"app_id"`
	//  PriavetKey contains a redacted placeholder for the private key.
	PrivateKey []byte `bson:"private_key" json:"private_key"`
}

// redactPrivateKey redacts the GitHub app's private key so that it's not
// exposed via the UI or GraphQL.
func (g *ProjectEventGitHubAppAuth) redactPrivateKey() *ProjectEventGitHubAppAuth {
	g.PrivateKey = []byte(evergreen.RedactedValue)
	return g
}

// ProjectSettingsEvent contains the event data about a single revision of a
// project's settings.
type ProjectSettingsEvent struct {
	// These fields are mostly copied from ProjectSettings, except for a few
	// where the data available to project events differs from the original data
	// model.
	// For example, the ProjectVars model cannot be used as-is because the
	// project variables are not stored in the database for security reasons.
	// However, the project event model needs to keep track of which project
	// variables changed, hence the need for a separate model in that situation.
	ProjectRef         ProjectRef                `bson:"proj_ref" json:"proj_ref"`
	GithubHooksEnabled bool                      `bson:"github_hooks_enabled" json:"github_hooks_enabled"`
	Aliases            []ProjectAlias            `bson:"aliases" json:"aliases"`
	Subscriptions      []event.Subscription      `bson:"subscriptions" json:"subscriptions"`
	GitHubAppAuth      ProjectEventGitHubAppAuth `bson:"github_app_auth" json:"github_app_auth"`
	Vars               ProjectEventVars          `bson:"vars" json:"vars"`

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

// NewProjectSettingsEvent creates project settings event data from project
// settings.
func NewProjectSettingsEvent(p ProjectSettings) ProjectSettingsEvent {
	return ProjectSettingsEvent{
		ProjectRef:         p.ProjectRef,
		GithubHooksEnabled: p.GithubHooksEnabled,
		Aliases:            p.Aliases,
		Subscriptions:      p.Subscriptions,
		GitHubAppAuth: ProjectEventGitHubAppAuth{
			AppID:      p.GitHubAppAuth.AppID,
			PrivateKey: p.GitHubAppAuth.PrivateKey,
		},
		Vars: ProjectEventVars{
			Vars:          p.Vars.Vars,
			PrivateVars:   p.Vars.PrivateVars,
			AdminOnlyVars: p.Vars.AdminOnlyVars,
		},
	}
}

type ProjectChangeEvent struct {
	User   string               `bson:"user" json:"user"`
	Before ProjectSettingsEvent `bson:"before" json:"before"`
	After  ProjectSettingsEvent `bson:"after" json:"after"`
}

// RedactSecrets redacts project secrets from a project change event. Project
// variables that are not changed are cleared and project variables that are
// changed are replaced with redacted placeholders.
func (e *ProjectChangeEvent) RedactSecrets() {
	modifiedVarKeys := e.getModifiedProjectVars()
	e.Before.Vars.Vars = getRedactedVarsCopy(e.Before.Vars.Vars, modifiedVarKeys, evergreen.RedactedBeforeValue)
	e.After.Vars.Vars = getRedactedVarsCopy(e.After.Vars.Vars, modifiedVarKeys, evergreen.RedactedAfterValue)
	isGHAppKeyModified := !bytes.Equal(e.Before.GitHubAppAuth.PrivateKey, e.After.GitHubAppAuth.PrivateKey)
	e.Before.GitHubAppAuth = getRedactedGitHubAppCopy(e.Before.GitHubAppAuth, isGHAppKeyModified, evergreen.RedactedBeforeValue)
	e.After.GitHubAppAuth = getRedactedGitHubAppCopy(e.After.GitHubAppAuth, isGHAppKeyModified, evergreen.RedactedAfterValue)
}

// getModifiedProjectVars returns the set of project variables in the change
// event that have been added, removed, or had their values modified.
func (e *ProjectChangeEvent) getModifiedProjectVars() map[string]struct{} {
	modifiedVarsSet := map[string]struct{}{}

	for name, value := range e.Before.Vars.Vars {
		afterVal, exists := e.After.Vars.Vars[name]
		if !exists || afterVal != value {
			// Variable was modified or deleted.
			modifiedVarsSet[name] = struct{}{}
		}
	}
	for name, value := range e.After.Vars.Vars {
		beforeVal, exists := e.Before.Vars.Vars[name]
		if !exists || beforeVal != value {
			// Variable was modified or added.
			modifiedVarsSet[name] = struct{}{}
		}
	}

	return modifiedVarsSet
}

// getRedactedVarsCopy returns a copy of the project variables with modified
// variables redacted and unmodified variables set to the empty string.
// This intentionally makes a copy to avoid potentially modifying the original
// vars map, which may be shared by the actual project settings. That way,
// callers can still access the unredacted variable values.
func getRedactedVarsCopy(vars map[string]string, modifiedVarNames map[string]struct{}, placeholder string) map[string]string {
	if len(vars) == 0 {
		return map[string]string{}
	}
	// Note: this copy logic can be replaced by maps.Clone(vars) once Evergreen
	// can compile with go 1.21 or higher.
	redactedVars := make(map[string]string, len(vars))
	for name, value := range vars {
		redactedVars[name] = value
	}

	for name, value := range redactedVars {
		if _, ok := modifiedVarNames[name]; ok && value != "" {
			// The project var was modified and it had a value, so replace the
			// value with a placeholder string to indicate that it changed.
			redactedVars[name] = placeholder
		} else if value != "" {
			// The project var was not modified, but it still has a non-empty
			// value. Set the value to empty to indicate that the project var
			// exists but was not modified.
			redactedVars[name] = ""
		}
	}
	return redactedVars
}

// getRedactedGitHubAppCopy returns a copy of the GitHub app auth with the
// GitHub app's private key redacted if it is set.
// This intentionally makes a copy to avoid potentially modifying the original
// GitHub app auth, which may be shared by the actual project settings. That
// way, callers can still access the unredacted auth credentials.
func getRedactedGitHubAppCopy(auth ProjectEventGitHubAppAuth, isGHAppKeyModified bool, placeholder string) ProjectEventGitHubAppAuth {
	if len(auth.PrivateKey) == 0 {
		return auth
	}
	redactedAuth := auth
	if len(redactedAuth.PrivateKey) > 0 && isGHAppKeyModified {
		redactedAuth.PrivateKey = []byte(placeholder)
	} else if len(redactedAuth.PrivateKey) > 0 {
		redactedAuth.PrivateKey = []byte{}
	}
	return redactedAuth
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

// RedactGitHubPrivateKey redacts the GitHub app's private key from the project modification event.
func (p *ProjectChangeEvents) RedactGitHubPrivateKey() {
	for _, event := range *p {
		changeEvent, isChangeEvent := event.Data.(*ProjectChangeEvent)
		if !isChangeEvent {
			continue
		}
		if len(changeEvent.After.GitHubAppAuth.PrivateKey) > 0 && len(changeEvent.Before.GitHubAppAuth.PrivateKey) > 0 {
			if string(changeEvent.After.GitHubAppAuth.PrivateKey) == string(changeEvent.Before.GitHubAppAuth.PrivateKey) {
				changeEvent.After.GitHubAppAuth.redactPrivateKey()
				changeEvent.Before.GitHubAppAuth.redactPrivateKey()
			} else {
				changeEvent.After.GitHubAppAuth.PrivateKey = []byte(evergreen.RedactedAfterValue)
				changeEvent.Before.GitHubAppAuth.PrivateKey = []byte(evergreen.RedactedBeforeValue)
			}
		} else if len(changeEvent.After.GitHubAppAuth.PrivateKey) > 0 {
			changeEvent.After.GitHubAppAuth.redactPrivateKey()
		} else if len(changeEvent.Before.GitHubAppAuth.PrivateKey) > 0 {
			changeEvent.Before.GitHubAppAuth.redactPrivateKey()
		}
		event.EventLogEntry.Data = changeEvent
	}
}

// RedactSecrets redacts project variables from all the project modification
// events.
// TODO (DEVPROD-11827): this can be removed entirely once project event logs
// are migrated to not store any project var values or GitHub app credentials.
// Project change events should already redact those secret values when the log
// is inserted into the DB (see (ProjectChangeEvent).RedactSecrets).
func (p *ProjectChangeEvents) RedactSecrets() {
	for _, event := range *p {
		changeEvent, isChangeEvent := event.Data.(*ProjectChangeEvent)
		if !isChangeEvent {
			continue
		}
		changeEvent.RedactSecrets()
		event.EventLogEntry.Data = changeEvent
	}
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
	// TODO (DEVPROD-1838): Remove once old events are TTLed and/or migrated.
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
	eventData.RedactSecrets()
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
	projectSettingsEvent := NewProjectSettingsEvent(*p)

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
	return &projectSettingsEvent
}

// LogProjectModified logs an event for a modification of a project's settings.
// Secrets are redacted from the event data.
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
