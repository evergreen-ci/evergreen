package notify

import (
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/util"
	"strings"
)

// Interface around an email that may be sent as a notification. Provides
// getters for body, subject, and change info, and functions to determine
// if this email should be skipped.
type Email interface {
	GetBody() string
	GetSubject() string
	// for system-related failures, we only want to notify the admins
	// regardless of who the default recipient is
	IsLikelySystemFailure() bool
	GetRecipients(defaultRecipient string) []string
	GetChangeInfo() []ChangeInfo
	// Should you skip sending this email given the provided variants to skip
	ShouldSkip(skipVariants []string) bool
}

// "Base class" for structs that implement the Email interface. Stores the
// basics that any email should have.
type EmailBase struct {
	Body       string
	Subject    string
	ChangeInfo []ChangeInfo
}

func (self *EmailBase) GetBody() string {
	return self.Body
}

func (self *EmailBase) GetSubject() string {
	return self.Subject
}

func (self *EmailBase) GetChangeInfo() []ChangeInfo {
	return self.ChangeInfo
}

// Implements Email interface for build-specific emails. Defines skip logic for
// emails specific to build
type BuildEmail struct {
	EmailBase
	Trigger TriggeredBuildNotification
}

func (self *BuildEmail) ShouldSkip(skipVariants []string) bool {
	buildVariant := self.Trigger.Current.BuildVariant
	if util.SliceContains(skipVariants, buildVariant) {
		evergreen.Logger.Logf(slogger.DEBUG, "Skipping buildvariant %v “%v” notification: “%v”",
			buildVariant, self.Trigger.Key.NotificationName, self.Subject)
		return true
	}
	return false
}

func (self *BuildEmail) IsLikelySystemFailure() bool {
	return false
}

func (self *BuildEmail) GetRecipients(defaultRecipient string) []string {
	return []string{defaultRecipient}
}

// Implements Email interface for task-specific emails. Defines skip logic for
// emails specific to tasks
type TaskEmail struct {
	EmailBase
	Trigger TriggeredTaskNotification
}

func (self *TaskEmail) ShouldSkip(skipVariants []string) bool {
	// skip the buildvariant notification if necessary
	buildVariant := self.Trigger.Current.BuildVariant
	if util.SliceContains(skipVariants, buildVariant) {
		evergreen.Logger.Logf(slogger.DEBUG, "Skipping buildvariant %v “%v” notification: “%v”",
			buildVariant, self.Trigger.Key.NotificationName, self.Subject)
		return true
	}
	return false
}

func (self *TaskEmail) IsLikelySystemFailure() bool {
	return strings.Contains(self.GetSubject(), UnresponsiveMessage)
}

func (self *TaskEmail) GetRecipients(defaultRecipient string) []string {
	return []string{defaultRecipient}
}
