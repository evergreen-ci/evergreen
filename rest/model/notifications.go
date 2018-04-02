package model

import (
	"time"

	"github.com/pkg/errors"
)

type APINotificationStats struct {
	LastProcessedAt      time.Time `json:"last_processed_at"`
	NumUnprocessedEvents int       `json:"unprocessed_events"`
}

func (n *APINotificationStats) BuildFromService(_ interface{}) error {
	return errors.New("(*NotificationsStats) BuildFromService not implemented")

}

func (n *APINotificationStats) ToService() (interface{}, error) {
	return nil, errors.New("(*NotificationsStats) ToService not implemented")
}
