package event

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
)

func TestRecentHostStatusFinder(t *testing.T) {
	assert := assert.New(t) // nolint

	const hostID = "host-one"

	assert.NoError(db.Clear(AllLogCollection))

	assert.False(AllRecentHostEventsMatchStatus(hostID, 3, "two"))

	LogHostEvent(hostID, EventTaskFinished, HostEventData{TaskStatus: "one", TaskId: "task"})
	LogHostEvent(hostID, EventTaskFinished, HostEventData{TaskStatus: "one", TaskId: "task"})
	LogHostEvent(hostID, EventTaskFinished, HostEventData{TaskStatus: "one", TaskId: "task"})

	assert.True(AllRecentHostEventsMatchStatus(hostID, 3, "one"))
	assert.False(AllRecentHostEventsMatchStatus(hostID, 3, "two"))

	LogHostEvent(hostID, EventTaskFinished, HostEventData{TaskStatus: "two", TaskId: "task"})

	assert.False(AllRecentHostEventsMatchStatus(hostID, 3, "one"))
	assert.False(AllRecentHostEventsMatchStatus(hostID, 3, "two"))
	assert.False(AllRecentHostEventsMatchStatus(hostID, 2, "two"))
	assert.True(AllRecentHostEventsMatchStatus(hostID, 1, "two"))

	assert.False(AllRecentHostEventsMatchStatus("none", 2, "two"))
	assert.False(AllRecentHostEventsMatchStatus("none", 1, "one"))
	assert.False(AllRecentHostEventsMatchStatus("none", 1, "one"))
}
