package client

import (
	"context"

	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
)

type timeoutLogSender struct {
	send.Sender
	comm SharedCommunicator
}

func (s *timeoutLogSender) Send(ctx context.Context, m message.Composer) {
	s.Sender.Send(ctx, m)
	s.comm.UpdateLastMessageTime()
}

func makeTimeoutLogSender(sender send.Sender, comm SharedCommunicator) send.Sender {
	return &timeoutLogSender{
		Sender: sender,
		comm:   comm,
	}
}
