package logger

import (
	"context"
	"errors"
	"fmt"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
)

type multiSendMessageJob struct {
	message message.Composer
	senders []send.Sender
	job.Base
}

// NewMultiSendMessageJob buils and amboy.Job instance that sends a
// single message to a group of Sender implementations. The job sends
// the message to each Sender serially.
//
// This job is not compatible with remote-storage backed queues.
func NewMultiSendMessageJob(m message.Composer, s []send.Sender) amboy.Job {
	j := &multiSendMessageJob{
		message: m,
		senders: s,
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    "multi-send-grip-message",
				Version: -1,
			},
		},
	}
	j.SetID(fmt.Sprintf("queued-multi-send-message-%d-<m:%T>-<ns:%d>",
		job.GetNumber(), m, len(s)))
	return j
}

func (j *multiSendMessageJob) Run(_ context.Context) {
	defer j.MarkComplete()

	if j.message == nil {
		j.AddError(errors.New("invalid message"))
		return
	}

	if j.senders == nil || len(j.senders) == 0 {
		j.AddError(errors.New("no senders defined"))
		return
	}

	for _, s := range j.senders {
		s.Send(j.message)
	}
}
