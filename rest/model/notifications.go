package model

import (
	"time"

	"github.com/pkg/errors"
)

type APINotificationStats struct {
	LastProcessed        time.Time      `json:"last_processed_at"`
	NumUnprocessedEvents uint           `json:"unprocessed_events"`
	ByType               map[string]int `json:"by_type"`
}

func (n *APINotificationStats) BuildFromService(_ interface{}) error {
	return errors.New("(*NotificationsStats) BuildFromService not implemented")

}

func (n *APINotificationStats) ToService() (interface{}, error) {
	return nil, errors.New("(*NotificationsStats) ToService not implemented")
}
