package client

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"golang.org/x/net/context"
)

type logSender struct {
	logTaskData TaskData
	logChannel  string
	comm        Communicator
	*send.Base
}

func newLogSender(comm Communicator, channel string, taskData TaskData) send.Sender {
	s := &logSender{
		comm:        comm,
		logChannel:  channel,
		logTaskData: taskData,
		Base:        send.NewBase(taskData.ID),
	}

	_ = s.Base.SetErrorHandler(send.ErrorHandlerFromSender(grip.GetSender()))

	return s
}

func (s *logSender) Send(m message.Composer) {
	if s.Level().ShouldLog(m) {
		err := s.comm.SendTaskLogMessages(
			context.TODO(),
			s.logTaskData,
			s.convertMessages(m))

		if err != nil {
			s.ErrorHandler(err, m)
		}
	}
}

func (s *logSender) convertMessages(m message.Composer) []apimodels.LogMessage {
	g, ok := m.(*message.GroupComposer)
	if ok {
		out := []apimodels.LogMessage{}

		for _, msg := range g.Messages() {
			out = append(out, s.convertMessages(msg)...)
		}

		return out
	}

	msg, err := s.Formatter(m)

	if err != nil {
		msg = m.String()
	}

	return []apimodels.LogMessage{
		{
			Type:      s.logChannel,
			Severity:  priorityToString(m.Priority()),
			Message:   msg,
			Timestamp: time.Now(),
			Version:   evergreen.LogmessageCurrentVersion,
		},
	}
}

func priorityToString(l level.Priority) string {
	switch l {
	case level.Trace, level.Debug:
		return apimodels.LogDebugPrefix
	case level.Notice, level.Info:
		return apimodels.LogInfoPrefix
	case level.Warning:
		return apimodels.LogWarnPrefix
	case level.Error, level.Alert, level.Critical, level.Emergency:
		return apimodels.LogErrorPrefix
	default:
		return "UNKNOWN"
	}
}
