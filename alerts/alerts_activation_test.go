package alerts

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/alertrecord"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/stretchr/testify/assert"
)

func TestRunSpawnWarningTriggers(t *testing.T) {
	assert := assert.New(t)
	assert.NoError(db.ClearCollections(alertrecord.Collection, event.AllLogCollection))
	h := host.Host{
		Id:             "host1",
		ExpirationTime: time.Now().Add(1 * time.Hour),
	}
	assert.NoError(RunSpawnWarningTriggers(&h))

	// check that the 2 and 12 hour warnings are logged
	events, err := event.Find(event.AllLogCollection, event.HostEventsInOrder(h.Id))
	assert.NoError(err)
	assert.Len(events, 2)

	// checking again should not generate more events
	assert.NoError(RunSpawnWarningTriggers(&h))
	events, err = event.Find(event.AllLogCollection, event.HostEventsInOrder(h.Id))
	assert.NoError(err)
	assert.Len(events, 2)
}
