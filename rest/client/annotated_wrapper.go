package client

import (
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
)

type taskAnnotator struct {
	send.Sender
	taskID  string
	logType string
}

func (l *taskAnnotator) Send(m message.Composer) {
	grip.Error(m.Annotate("task_id", l.taskID))
	grip.Error(m.Annotate("type", l.logType))
	l.Sender.Send(m)
}

func newAnnotatedWrapper(taskID, logType string, sender send.Sender) send.Sender {
	return &taskAnnotator{
		taskID:  taskID,
		logType: logType,
		Sender:  sender,
	}
}
