package model

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNotificationStats(t *testing.T) {
	assert := assert.New(t)

	stats := APINotificationStats{
		LastProcessed:        time.Now(),
		NumUnprocessedEvents: 1234,
	}

	assert.EqualError(stats.BuildFromService(nil), "(*NotificationsStats) BuildFromService not implemented")

	_, err := stats.ToService()
	assert.EqualError(err, "(*NotificationsStats) ToService not implemented")
	assert.Implements((*Model)(nil), &stats)
}
