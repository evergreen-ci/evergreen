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

type APIFailedWebhook struct {
	NotificationID *string    `json:"notification_id"`
	Project        *string    `json:"project,omitempty"`
	URL            *string    `json:"url"`
	Error          *string    `json:"error"`
	SentAt         *time.Time `json:"sent_at"`
	// Body and Headers allow consumers to reconstruct the request; neither
	// the HMAC secret nor the Authorization header is included.
	Body    *string             `json:"body,omitempty"`
	Headers map[string][]string `json:"headers,omitempty"`
}

func (m *APIFailedWebhook) BuildFromService(f notification.FailedWebhook) {
	m.NotificationID = utility.ToStringPtr(f.NotificationID)
	if f.Project != "" {
		m.Project = utility.ToStringPtr(f.Project)
	}
	m.URL = utility.ToStringPtr(f.URL)
	m.Error = utility.ToStringPtr(f.Error)
	m.SentAt = &f.SentAt
	if len(f.Body) > 0 {
		m.Body = utility.ToStringPtr(string(f.Body))
	}
	if len(f.Headers) > 0 {
		m.Headers = f.Headers
	}
}
