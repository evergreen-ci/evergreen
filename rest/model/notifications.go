package model

import (
	"time"

	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/pkg/errors"
)

type APIEventStats struct {
	LastProcessedAt            *time.Time           `json:"last_processed_at"`
	NumUnprocessedEvents       int                  `json:"unprocessed_events"`
	PendingNotificationsByType apiNotificationStats `json:"pending_notifications_by_type"`
}

func (n *APIEventStats) BuildFromService(h interface{}) error {
	stats, ok := h.(*notification.NotificationStats)
	if !ok {
		return errors.Errorf("programmatic error: expected notification stats but got type %T", h)
	}

	return n.PendingNotificationsByType.BuildFromService(stats)
}

func (n *APIEventStats) ToService() (interface{}, error) {
	return nil, errors.New("(*APIEventStats) ToService not implemented")
}

type apiNotificationStats struct {
	GithubPullRequest int `json:"github_pull_request"`
	JIRAIssue         int `json:"jira_issue"`
	JIRAComment       int `json:"jira_comment"`
	EvergreenWebhook  int `json:"evergreen_webhook"`
	Email             int `json:"email"`
	Slack             int `json:"slack"`
}

func (n *apiNotificationStats) BuildFromService(h interface{}) error {
	data, ok := h.(*notification.NotificationStats)
	if !ok {
		return errors.Errorf("programmatic error: expected notification stats but got type %T", h)
	}

	n.GithubPullRequest = data.GithubPullRequest
	n.JIRAIssue = data.JIRAIssue
	n.JIRAComment = data.JIRAComment
	n.EvergreenWebhook = data.EvergreenWebhook
	n.Email = data.Email
	n.Slack = data.Slack

	return nil
}

func (n *apiNotificationStats) ToService() (interface{}, error) {
	return nil, errors.New("(*apiNotificationsStats) ToService not implemented")
}
