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

	// first make sure that things are errors when you try them
	assert.False(AllRecentHostEventsMatchStatus(hostID, 3, "two"))

	// insert three things into the collection, so we have some data to work with
	LogHostEvent(hostID, EventTaskFinished, HostEventData{TaskStatus: "one", TaskId: "task"})
	LogHostEvent(hostID, EventTaskFinished, HostEventData{TaskStatus: "one", TaskId: "task"})
	LogHostEvent(hostID, EventTaskFinished, HostEventData{TaskStatus: "one", TaskId: "task"})

	// make sure that we have the expected outcome for a trivial example
	assert.True(AllRecentHostEventsMatchStatus(hostID, 3, "one"))
	assert.True(AllRecentHostEventsMatchStatus(hostID, 2, "one"))
	assert.False(AllRecentHostEventsMatchStatus(hostID, 3, "two"))

	// log a different type so so that we can see
	LogHostEvent(hostID, EventTaskFinished, HostEventData{TaskStatus: "two", TaskId: "task"})
	LogHostEvent(hostID, EventTaskFinished, HostEventData{TaskStatus: "two", TaskId: "task"})

	// ensure that the outcome of the predicate matches our
	// current understanding of the state of the database
	assert.False(AllRecentHostEventsMatchStatus(hostID, 3, "one"))
	assert.False(AllRecentHostEventsMatchStatus(hostID, 3, "two"))
	assert.False(AllRecentHostEventsMatchStatus(hostID, 2, "one"))
	assert.True(AllRecentHostEventsMatchStatus(hostID, 2, "two"))

	// zero events should always be false.
	assert.False(AllRecentHostEventsMatchStatus(hostID, 0, "two"))
	assert.False(AllRecentHostEventsMatchStatus(hostID, 0, "one"))

	// make sure that different host ids return false because
	// there isn't enough data, and make sure that we don't
	// accidentally ignore hostID
	assert.False(AllRecentHostEventsMatchStatus("none", 2, "two"))
	assert.False(AllRecentHostEventsMatchStatus("none", 1, "two"))
	assert.False(AllRecentHostEventsMatchStatus("none", 1, "one"))
}
