package client

import (
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
)

type timeoutLogSender struct {
	sender send.Sender
	comm   Communicator
}

func (s *timeoutLogSender) Send(m message.Composer) {
	s.sender.Send(m)
	s.comm.UpdateLastMessageTime()
}

func (s *timeoutLogSender) Close() error {
	return s.sender.Close()
}

func (s *timeoutLogSender) Name() string {
	return s.sender.Name()
}

func (s *timeoutLogSender) SetName(name string) {
	s.sender.SetName(name)
}

func (s *timeoutLogSender) SetLevel(l send.LevelInfo) error {
	return s.sender.SetLevel(l)
}

func (s *timeoutLogSender) Level() send.LevelInfo {
	return s.sender.Level()
}

func (s *timeoutLogSender) SetErrorHandler(h send.ErrorHandler) error {
	return s.sender.SetErrorHandler(h)
}

func (s *timeoutLogSender) SetFormatter(f send.MessageFormatter) error {
	return s.sender.SetFormatter(f)
}

func makeTimeoutLogSender(sender send.Sender, comm Communicator) send.Sender {
	return &timeoutLogSender{
		sender: sender,
		comm:   comm,
	}
}
