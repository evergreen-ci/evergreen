package client

import (
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
)

type timeoutLogSender struct {
	send.Sender
	comm Communicator
}

func (s *timeoutLogSender) Send(m message.Composer) {
	s.Sender.Send(m)
	s.comm.UpdateLastMessageTime()
}

func makeTimeoutLogSender(sender send.Sender, comm Communicator) send.Sender {
	return &timeoutLogSender{
		Sender: sender,
		comm:   comm,
	}
}
