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

const (
	bufferTime  = 15 * time.Second
	bufferCount = 100
)

type logSender struct {
	logTaskData TaskData
	logChannel  string
	comm        Communicator
	cancel      context.CancelFunc
	pipe        chan message.Composer
	*send.Base
}

func newLogSender(ctx context.Context, comm Communicator, channel string, taskData TaskData) send.Sender {
	s := &logSender{
		comm:        comm,
		logChannel:  channel,
		logTaskData: taskData,
		Base:        send.NewBase(taskData.ID),
		pipe:        make(chan message.Composer, bufferCount/2),
	}

	ctx, s.cancel = context.WithCancel(ctx)

	go s.startBackgroundSender(ctx)

	return s
}

func (s *logSender) Close() error { s.cancel(); return s.Base.Close() }

func (s *logSender) startBackgroundSender(ctx context.Context) {
	timer := time.NewTimer(bufferTime)
	buffer := []apimodels.LogMessage{}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			grip.CatchAlert(s.comm.SendLogMessages(ctx, s.logTaskData, buffer))
			buffer = []apimodels.LogMessage{}
			return
		case <-timer.C:
			grip.CatchAlert(s.comm.SendLogMessages(ctx, s.logTaskData, buffer))
			buffer = []apimodels.LogMessage{}
			timer.Reset(bufferTime)
		case m := <-s.pipe:
			buffer = append(buffer, s.convertMessage(m))
			if len(buffer) >= bufferCount/2 {
				grip.CatchAlert(s.comm.SendLogMessages(ctx, s.logTaskData, buffer))
				buffer = []apimodels.LogMessage{}
			}
		}
	}
}

func (s *logSender) Send(m message.Composer) {
	if s.Level().ShouldLog(m) {
		s.pipe <- m
	}
}

func (s *logSender) convertMessage(m message.Composer) apimodels.LogMessage {
	return apimodels.LogMessage{
		Type:      s.logChannel,
		Severity:  priorityToString(m.Priority()),
		Message:   m.String(),
		Timestamp: time.Now(),
		Version:   evergreen.LogmessageCurrentVersion,
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
