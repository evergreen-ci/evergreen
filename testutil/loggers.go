package testutil

import (
	"sync"

	slogger "github.com/10gen-labs/slogger/v1"
)

//SliceAppender is a slogger.Appender implemenation that just adds every
//log message to an internal slice. Useful for testing when a test needs to
//capture data sent to a slogger.Logger and verify what data was written.
type SliceAppender struct {
	messages []slogger.Log
	mutex    sync.RWMutex
}

func (self *SliceAppender) Append(log *slogger.Log) error {
	self.mutex.Lock()
	defer self.mutex.Unlock()

	self.messages = append(self.messages, *log)
	return nil
}

func (self *SliceAppender) Messages() []slogger.Log {
	self.mutex.RLock()
	defer self.mutex.RUnlock()

	out := make([]slogger.Log, len(self.messages))

	copy(out, self.messages)

	return out
}
