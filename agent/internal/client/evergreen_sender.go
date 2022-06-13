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

type evergreenLogSender struct {
	logTaskData         TaskData
	logChannel          string
	comm                SharedCommunicator
	cancel              context.CancelFunc
	pipe                chan message.Composer
	signalFlush         chan struct{}
	signalFlushComplete chan struct{}
	lastBatch           chan struct{}
	signalEnd           chan struct{}
	bufferTime          time.Duration
	bufferSize          int
	closed              bool
	sync.RWMutex
	*send.Base
}

func newEvergreenLogSender(ctx context.Context, comm SharedCommunicator, channel string, taskData TaskData, bufferSize int, bufferTime time.Duration) send.Sender {
	s := &evergreenLogSender{
		comm:                comm,
		logChannel:          channel,
		logTaskData:         taskData,
		Base:                send.NewBase(taskData.ID),
		bufferSize:          bufferSize,
		pipe:                make(chan message.Composer, bufferSize/2),
		signalFlush:         make(chan struct{}),
		signalFlushComplete: make(chan struct{}),
		lastBatch:           make(chan struct{}),
		signalEnd:           make(chan struct{}),
	}
	ctx, s.cancel = context.WithCancel(ctx)

	go s.startBackgroundSender(ctx)

	return s
}

func (s *evergreenLogSender) getBufferTime() time.Duration {
	s.RLock()
	defer s.RUnlock()
	return s.bufferTime
}

func (s *evergreenLogSender) setBufferTime(d time.Duration) {
	s.Lock()
	defer s.Unlock()
	s.bufferTime = d
}

func (s *evergreenLogSender) Close() error {
	s.Lock()
	close(s.signalEnd)
	s.closed = true
	s.Unlock()

	<-s.lastBatch
	return s.Base.Close()
}

func (s *evergreenLogSender) flush(ctx context.Context, buffer []apimodels.LogMessage) {
	grip.Critical(s.comm.SendLogMessages(ctx, s.logTaskData, buffer))
}

func (s *evergreenLogSender) startBackgroundSender(ctx context.Context) {
	bufferTime := s.getBufferTime()
	if bufferTime == 0 {
		bufferTime = defaultLogBufferTime
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
			s.Lock()
			s.closed = true
			s.Unlock()
			break backgroundSender
		case <-timer.C:
			if len(buffer) > 0 {
				s.flush(ctx, buffer)
				buffer = []apimodels.LogMessage{}
			}
			timer.Reset(bufferTime)
		case <-s.signalFlush:
			if len(buffer) > 0 {
				s.flush(ctx, buffer)
				buffer = []apimodels.LogMessage{}
			}
			timer.Reset(bufferTime)
			s.signalFlushComplete <- struct{}{}
		case m := <-s.pipe:
			buffer = append(buffer, s.convertMessage(m))
			if len(buffer) >= s.bufferSize/2 {
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

	// drain the pipe
	close(s.pipe)
	for msg := range s.pipe {
		buffer = append(buffer, s.convertMessage(msg))
		if len(buffer) >= s.bufferSize/2 {
			s.flush(ctx, buffer)
			buffer = []apimodels.LogMessage{}
		}
	}

	// send the final batch
	s.flush(ctx, buffer)

	// let close return
	close(s.lastBatch)
}

func (s *evergreenLogSender) Send(m message.Composer) {
	s.RLock()
	defer s.RUnlock()
	if s.closed {
		return
	}
	if s.Level().ShouldLog(m) {
		s.pipe <- m
	}
}

func (s *evergreenLogSender) Flush(_ context.Context) error {
	s.RLock()
	defer s.RUnlock()

	if s.closed {
		return nil
	}

	s.signalFlush <- struct{}{}
	<-s.signalFlushComplete

	return nil
}

func (s *evergreenLogSender) convertMessage(m message.Composer) apimodels.LogMessage {
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
