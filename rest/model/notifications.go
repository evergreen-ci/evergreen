package model

import (
	"time"

	"github.com/evergreen-ci/evergreen/model/notification"
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
