package model

import (
	"reflect"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/stretchr/testify/assert"
)

func TestEventStats(t *testing.T) {
	assert := assert.New(t)

	nstats := notification.NotificationStats{Email: 5}

	stats := APIEventStats{
		LastProcessedAt:      ToTimePtr(time.Now()),
		NumUnprocessedEvents: 1234,
	}

	assert.EqualError(stats.BuildFromService(nil), "can't convert unknown type to APIEventStats")
	assert.NoError(stats.BuildFromService(&nstats))
	assert.NotZero(stats.PendingNotificationsByType)

	x, err := stats.ToService()
	assert.Nil(x)
	assert.EqualError(err, "(*APIEventStats) ToService not implemented")
	assert.Implements((*Model)(nil), &stats)
}

func TestNotificationStats(t *testing.T) {
	assert := assert.New(t)

	// set all fields in nstats to 1
	nstats := notification.NotificationStats{}
	v := reflect.ValueOf(&nstats).Elem()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		f.SetInt(1)
	}

	stats := apiNotificationStats{}
	assert.EqualError(stats.BuildFromService(5), "can't convert unknown type to apiNotificationStats")
	assert.EqualError(stats.BuildFromService(nstats), "can't convert unknown type to apiNotificationStats")
	assert.NoError(stats.BuildFromService(&nstats))

	// all fields should be 1
	v = reflect.ValueOf(&stats).Elem()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		assert.Equal(1, int(f.Int()))
	}

	x, err := stats.ToService()
	assert.Nil(x)
	assert.EqualError(err, "(*apiNotificationsStats) ToService not implemented")
	assert.Implements((*Model)(nil), &stats)
}
