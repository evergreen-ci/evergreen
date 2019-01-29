package client

import (
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
)

const (
	taskIDKey  = "task_id"
	logTypeKey = "type"
)

type taskAnnotator struct {
	send.Sender
	taskID  string
	logType string
}

func (l *taskAnnotator) Send(m message.Composer) {
	grip.Error(m.Annotate(taskIDKey, l.taskID))
	grip.Error(m.Annotate(logTypeKey, l.logType))
	l.Sender.Send(m)
}

func newAnnotatedWrapper(taskID, logType string, sender send.Sender) send.Sender {
	return &taskAnnotator{
		taskID:  taskID,
		logType: logType,
		Sender:  sender,
	}
}
