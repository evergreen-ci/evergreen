package amboy

import (
	"fmt"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/sometimes"
)

// QueueStats is a simple structure that the Stats() method in the
// Queue interface returns and tracks the state of the queue, and
// provides a common format for different Queue implementations to
// report on their state.
//
// Implement's grip's message.Composer interface when passed as a
// pointer.
type QueueStats struct {
	Running   int `bson:"running" json:"running" yaml:"running"`
	Completed int `bson:"completed" json:"completed" yaml:"completed"`
	Pending   int `bson:"pending" json:"pending" yaml:"pending"`
	Blocked   int `bson:"blocked" json:"blocked" yaml:"blocked"`
	Total     int `bson:"total" json:"total" yaml:"total"`

	priority level.Priority
}

func (s QueueStats) String() string {
	return fmt.Sprintf("running='%d', completed='%d', pending='%d', blocked='%d', total='%d'",
		s.Running, s.Completed, s.Pending, s.Blocked, s.Total)
}

func (s QueueStats) isComplete() bool {
	grip.DebugWhen(sometimes.Fifth(), &s)

	if s.Total == s.Completed {
		return true
	}

	if s.Total <= s.Completed+s.Blocked {
		return true
	}

	return false
}

func (s QueueStats) Loggable() bool           { return s.Total > 0 }
func (s QueueStats) Raw() interface{}         { return s }
func (s QueueStats) Priority() level.Priority { return s.priority }
func (s *QueueStats) SetPriority(l level.Priority) error {
	if level.IsValidPriority(l) {
		s.priority = l
		return nil
	}
	return fmt.Errorf("%s (%d) is not a valid level", l, l)
}
