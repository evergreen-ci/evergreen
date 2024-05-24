package client

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model/log"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTimeoutSender(t *testing.T) {
	comm := NewMock("url")
	tsk := &task.Task{Id: "task"}
	ms := newMockSender("test_timeout_sender", func(line log.LogLine) error {
		return comm.sendTaskLogLine(tsk.Id, line)
	})
	sender := makeTimeoutLogSender(ms, comm)

	// If no messages are sent, the last message time *should not* update.
	last1 := comm.LastMessageAt()
	time.Sleep(20 * time.Millisecond)
	last2 := comm.LastMessageAt()
	assert.Equal(t, last1, last2)

	// If a message is sent, the last message time *should* update.
	sender.Send(message.NewDefaultMessage(level.Error, "hello world!!"))
	time.Sleep(20 * time.Millisecond)
	require.NoError(t, sender.Close())
	last3 := comm.LastMessageAt()
	assert.NotEqual(t, last2, last3)
}
