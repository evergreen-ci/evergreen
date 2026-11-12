package model

import (
	"time"

	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/utility"
)

type APIEventStats struct {
	LastProcessedAt            *time.Time           `json:"last_processed_at"`
	NumUnprocessedEvents       int                  `json:"unprocessed_events"`
	PendingNotificationsByType apiNotificationStats `json:"pending_notifications_by_type"`
}

func (n *APIEventStats) BuildFromService(stats notification.NotificationStats) {
	n.PendingNotificationsByType.BuildFromService(stats)
}

type apiNotificationStats struct {
	GithubPullRequest int `json:"github_pull_request"`
	JIRAIssue         int `json:"jira_issue"`
	JIRAComment       int `json:"jira_comment"`
	EvergreenWebhook  int `json:"evergreen_webhook"`
	Email             int `json:"email"`
	Slack             int `json:"slack"`
}

func (n *apiNotificationStats) BuildFromService(data notification.NotificationStats) {
	n.GithubPullRequest = data.GithubPullRequest
	n.JIRAIssue = data.JIRAIssue
	n.JIRAComment = data.JIRAComment
	n.EvergreenWebhook = data.EvergreenWebhook
	n.Email = data.Email
	n.Slack = data.Slack
}

// APIFailedWebhook is a webhook notification that failed to deliver.
type APIFailedWebhook struct {
	NotificationID *string    `json:"notification_id" extensions:"!x-nullable"`
	Project        *string    `json:"project,omitempty"`
	Error          *string    `json:"error" extensions:"!x-nullable"`
	FailedAt       *time.Time `json:"failed_at" extensions:"!x-nullable"`
}

// BuildFromService fills the API model from a failed webhook delivery.
func (m *APIFailedWebhook) BuildFromService(f notification.FailedWebhook) {
	m.NotificationID = utility.ToStringPtr(f.NotificationID)
	if f.Project != "" {
		m.Project = utility.ToStringPtr(f.Project)
	}
	m.Error = utility.ToStringPtr(f.Error)
	m.FailedAt = &f.FailedAt
}
