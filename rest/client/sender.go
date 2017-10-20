package client

import (
	"context"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
)

const (
	defaultBufferTime = 15 * time.Second
	bufferCount       = 1000
)

type logSender struct {
	logTaskData   TaskData
	logChannel    string
	comm          Communicator
	cancel        context.CancelFunc
	pipe          chan message.Composer
	lastBatch     chan struct{}
	signalEnd     chan struct{}
	updateTimeout bool
	bufferTime    time.Duration
	closed        bool
	sync.RWMutex
	*send.Base
}

func newTimeoutLogSender(ctx context.Context, comm Communicator, channel string, taskData TaskData) send.Sender {
	s := newLogSender(ctx, comm, channel, taskData).(*logSender)
	s.updateTimeout = true
	return s
}

func newLogSender(ctx context.Context, comm Communicator, channel string, taskData TaskData) send.Sender {
	s := &logSender{
		comm:        comm,
		logChannel:  channel,
		logTaskData: taskData,
		Base:        send.NewBase(taskData.ID),
		pipe:        make(chan message.Composer, bufferCount/2),
		lastBatch:   make(chan struct{}),
		signalEnd:   make(chan struct{}),
	}
	ctx, s.cancel = context.WithCancel(ctx)

	go s.startBackgroundSender(ctx)

	return s
}

func (s *logSender) getBufferTime() time.Duration {
	s.RLock()
	defer s.RUnlock()
	return s.bufferTime
}

func (s *logSender) setBufferTime(d time.Duration) {
	s.Lock()
	defer s.Unlock()
	s.bufferTime = d
}

func (s *logSender) Close() error {
	s.Lock()
	close(s.signalEnd)
	s.closed = true
	s.Unlock()

	<-s.lastBatch
	return s.Base.Close()
}

func (s *logSender) flush(ctx context.Context, buffer []apimodels.LogMessage) {
	grip.CatchAlert(s.comm.SendLogMessages(ctx, s.logTaskData, buffer))

	if s.updateTimeout {
		s.comm.UpdateLastMessageTime()
	}
}

func (s *logSender) startBackgroundSender(ctx context.Context) {
	bufferTime := s.getBufferTime()
	if bufferTime == 0 {
		bufferTime = defaultBufferTime
	}
	timer := time.NewTimer(bufferTime)
	buffer := []apimodels.LogMessage{}
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer timer.Stop()

backgroundSender:
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			if len(buffer) > 0 {
				s.flush(ctx, buffer)
				buffer = []apimodels.LogMessage{}
			}
			timer.Reset(bufferTime)
		case m := <-s.pipe:
			buffer = append(buffer, s.convertMessage(m))
			if len(buffer) >= bufferCount/2 {
				s.flush(ctx, buffer)
				buffer = []apimodels.LogMessage{}
				timer.Reset(bufferTime)
			}
		case <-s.signalEnd:
			break backgroundSender
		}
	}

	// set the level really high, (which is mutexed) so that we
	// never send another message
	_ = s.SetLevel(send.LevelInfo{Threshold: level.Priority(200)})
	// close the pipe so we can drain things
	close(s.pipe)
	// drain the pipe
	for msg := range s.pipe {
		buffer = append(buffer, s.convertMessage(msg))
	}

	// send the final batch
	s.flush(ctx, buffer)

	// let close return
	close(s.lastBatch)
}

func (s *logSender) Send(m message.Composer) {
	s.RLock()
	defer s.RUnlock()
	if s.closed {
		return
	}
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
