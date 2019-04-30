package commitqueue

import (
	"fmt"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
)

type DequeueItem struct {
	ProjectID string
	Item      string
	Status    string
}

type dequeueItemMessage struct {
	raw DequeueItem
	message.Base
}

// NewDequeueItemMessage returns a composer for DequeueItem messages
func NewDequeueItemMessage(p level.Priority, dequeueMsg DequeueItem) message.Composer {
	s := &dequeueItemMessage{
		raw: dequeueMsg,
	}
	if err := s.SetPriority(p); err != nil {
		_ = s.SetPriority(level.Notice)
	}

	return s
}

func (c *dequeueItemMessage) Loggable() bool {
	return c.raw.ProjectID != "" && c.raw.Item != "" && c.raw.Status != ""
}

func (c *dequeueItemMessage) String() string {
	return fmt.Sprintf("commit queue '%s' item '%s'", c.raw.ProjectID, c.raw.Item)
}

func (c *dequeueItemMessage) Raw() interface{} {
	return &c.raw
}
