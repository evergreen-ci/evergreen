package grip

import (
	"errors"
	"fmt"
	"testing"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/suite"
)

const testMessage = "hello world"

type (
	basicMethod    func(interface{})
	lnMethod       func(...interface{})
	fMethod        func(string, ...interface{})
	catchMethod    func(error)
	whenMethod     func(bool, interface{})
	whenlnMethod   func(bool, ...interface{})
	whenfMethod    func(bool, string, ...interface{})
	manyMethod     func(...message.Composer)
	manyWhenMethod func(bool, ...message.Composer)
)

type LoggingMethodSuite struct {
	logger        Journaler
	loggingSender *send.InternalSender
	stdSender     *send.InternalSender
	suite.Suite
}

func TestLoggingMethodSuite(t *testing.T) {
	suite.Run(t, new(LoggingMethodSuite))
}

func (s *LoggingMethodSuite) SetupSuite() {
	s.logger = logging.NewGrip("test")

	s.stdSender = send.MakeInternalLogger()
	s.NoError(SetSender(s.stdSender))
	s.Exactly(GetSender(), s.stdSender)

	s.loggingSender = send.MakeInternalLogger()
	s.NoError(s.logger.SetSender(s.loggingSender))
	s.Exactly(s.logger.GetSender(), s.loggingSender)
}

func (s *LoggingMethodSuite) TestCatchMethods() {
	cases := map[string][]catchMethod{
		"emergency": []catchMethod{CatchEmergency, s.logger.CatchEmergency},
		"alert":     []catchMethod{CatchAlert, s.logger.CatchAlert},
		"critical":  []catchMethod{CatchCritical, s.logger.CatchCritical},
		"error":     []catchMethod{CatchError, s.logger.CatchError},
		"warning":   []catchMethod{CatchWarning, s.logger.CatchWarning},
		"notice":    []catchMethod{CatchNotice, s.logger.CatchNotice},
		"info":      []catchMethod{CatchInfo, s.logger.CatchInfo},
		"debug":     []catchMethod{CatchDebug, s.logger.CatchDebug},
	}

	msg := errors.New("hello world")
	for kind, loggers := range cases {
		s.Len(loggers, 2)
		loggers[0](msg)
		loggers[1](msg)

		lgrMsg := s.loggingSender.GetMessage()
		stdMsg := s.stdSender.GetMessage()
		s.Equal(lgrMsg.Rendered, stdMsg.Rendered,
			fmt.Sprintf("%s: \n\tlogger: %+v \n\tstandard: %+v", kind, lgrMsg, stdMsg))
	}

}

func (s *LoggingMethodSuite) TestWhenMethods() {
	cases := map[string][]whenMethod{
		"emergency": []whenMethod{EmergencyWhen, s.logger.EmergencyWhen},
		"alert":     []whenMethod{AlertWhen, s.logger.AlertWhen},
		"critical":  []whenMethod{CriticalWhen, s.logger.CriticalWhen},
		"error":     []whenMethod{ErrorWhen, s.logger.ErrorWhen},
		"warning":   []whenMethod{WarningWhen, s.logger.WarningWhen},
		"notice":    []whenMethod{NoticeWhen, s.logger.NoticeWhen},
		"info":      []whenMethod{InfoWhen, s.logger.InfoWhen},
		"debug":     []whenMethod{DebugWhen, s.logger.DebugWhen},
	}

	for kind, loggers := range cases {
		s.Len(loggers, 2)
		loggers[0](true, testMessage)
		loggers[1](true, testMessage)

		s.True(s.loggingSender.HasMessage())
		s.True(s.stdSender.HasMessage())
		lgrMsg := s.loggingSender.GetMessage()
		stdMsg := s.stdSender.GetMessage()

		s.Equal(lgrMsg.Rendered, stdMsg.Rendered,
			fmt.Sprintf("%s: \n\tlogger: %+v \n\tstandard: %+v", kind, lgrMsg, stdMsg))

		loggers[0](false, testMessage)
		loggers[1](false, testMessage)
		s.False(s.loggingSender.HasMessage())
		s.False(s.stdSender.HasMessage())
	}
}

func (s *LoggingMethodSuite) TestWhenlnMethods() {
	cases := map[string][]whenlnMethod{
		"emergency": []whenlnMethod{EmergencyWhenln, s.logger.EmergencyWhenln},
		"alert":     []whenlnMethod{AlertWhenln, s.logger.AlertWhenln},
		"critical":  []whenlnMethod{CriticalWhenln, s.logger.CriticalWhenln},
		"error":     []whenlnMethod{ErrorWhenln, s.logger.ErrorWhenln},
		"warning":   []whenlnMethod{WarningWhenln, s.logger.WarningWhenln},
		"notice":    []whenlnMethod{NoticeWhenln, s.logger.NoticeWhenln},
		"info":      []whenlnMethod{InfoWhenln, s.logger.InfoWhenln},
		"debug":     []whenlnMethod{DebugWhenln, s.logger.DebugWhenln},
	}

	for kind, loggers := range cases {

		s.Len(loggers, 2)
		s.False(s.loggingSender.HasMessage())
		s.False(s.stdSender.HasMessage())

		loggers[0](true, testMessage, testMessage)
		loggers[1](true, testMessage, testMessage)

		s.True(s.loggingSender.HasMessage())
		s.True(s.stdSender.HasMessage())
		lgrMsg := s.loggingSender.GetMessage()
		stdMsg := s.stdSender.GetMessage()
		s.Equal(lgrMsg.Rendered, stdMsg.Rendered,
			fmt.Sprintf("%s: \n\tlogger: %+v \n\tstandard: %+v", kind, lgrMsg, stdMsg))

		loggers[0](false, testMessage, testMessage)
		loggers[1](false, testMessage, testMessage)
		s.False(s.loggingSender.HasMessage())
		s.False(s.stdSender.HasMessage())
	}
}

func (s *LoggingMethodSuite) TestWhenfMethods() {
	cases := map[string][]whenfMethod{
		"emergency": []whenfMethod{EmergencyWhenf, s.logger.EmergencyWhenf},
		"alert":     []whenfMethod{AlertWhenf, s.logger.AlertWhenf},
		"critical":  []whenfMethod{CriticalWhenf, s.logger.CriticalWhenf},
		"error":     []whenfMethod{ErrorWhenf, s.logger.ErrorWhenf},
		"warning":   []whenfMethod{WarningWhenf, s.logger.WarningWhenf},
		"notice":    []whenfMethod{NoticeWhenf, s.logger.NoticeWhenf},
		"info":      []whenfMethod{InfoWhenf, s.logger.InfoWhenf},
		"debug":     []whenfMethod{DebugWhenf, s.logger.DebugWhenf},
	}

	for kind, loggers := range cases {
		s.Len(loggers, 2)
		s.False(s.loggingSender.HasMessage())
		s.False(s.stdSender.HasMessage())

		loggers[0](true, "%s: %d", testMessage, 3)
		loggers[1](true, "%s: %d", testMessage, 3)

		s.True(s.loggingSender.HasMessage())
		s.True(s.stdSender.HasMessage())
		lgrMsg := s.loggingSender.GetMessage()
		stdMsg := s.stdSender.GetMessage()
		s.Equal(lgrMsg.Rendered, stdMsg.Rendered,
			fmt.Sprintf("%s: \n\tlogger: %+v \n\tstandard: %+v", kind, lgrMsg, stdMsg))

		loggers[0](false, "%s: %d", testMessage, 3)
		loggers[1](false, "%s: %d", testMessage, 3)
		s.False(s.loggingSender.HasMessage())
		s.False(s.stdSender.HasMessage())
	}
}

func (s *LoggingMethodSuite) TestBasicMethod() {
	cases := map[string][]basicMethod{
		"emergency": []basicMethod{Emergency, s.logger.Emergency},
		"alert":     []basicMethod{Alert, s.logger.Alert},
		"critical":  []basicMethod{Critical, s.logger.Critical},
		"error":     []basicMethod{Error, s.logger.Error},
		"warning":   []basicMethod{Warning, s.logger.Warning},
		"notice":    []basicMethod{Notice, s.logger.Notice},
		"info":      []basicMethod{Info, s.logger.Info},
		"debug":     []basicMethod{Debug, s.logger.Debug},
	}

	inputs := []interface{}{true, false, []string{"a", "b"}, message.Fields{"a": 1}, 1, "foo"}

	for kind, loggers := range cases {
		s.Len(loggers, 2)
		s.False(s.loggingSender.HasMessage())
		s.False(s.stdSender.HasMessage())

		for _, msg := range inputs {
			loggers[0](msg)
			loggers[1](msg)

			s.True(s.loggingSender.HasMessage())
			s.True(s.stdSender.HasMessage())
			lgrMsg := s.loggingSender.GetMessage()
			stdMsg := s.stdSender.GetMessage()
			s.Equal(lgrMsg.Rendered, stdMsg.Rendered,
				fmt.Sprintf("%s: \n\tlogger: %+v \n\tstandard: %+v", kind, lgrMsg, stdMsg))
		}
	}

}

func (s *LoggingMethodSuite) TestlnMethods() {
	cases := map[string][]lnMethod{
		"emergency": []lnMethod{Emergencyln, s.logger.Emergencyln},
		"alert":     []lnMethod{Alertln, s.logger.Alertln},
		"critical":  []lnMethod{Criticalln, s.logger.Criticalln},
		"error":     []lnMethod{Errorln, s.logger.Errorln},
		"warning":   []lnMethod{Warningln, s.logger.Warningln},
		"notice":    []lnMethod{Noticeln, s.logger.Noticeln},
		"info":      []lnMethod{Infoln, s.logger.Infoln},
		"debug":     []lnMethod{Debugln, s.logger.Debugln},
	}

	for kind, loggers := range cases {
		s.Len(loggers, 2)
		s.False(s.loggingSender.HasMessage())
		s.False(s.stdSender.HasMessage())

		loggers[0](true, testMessage, testMessage)
		loggers[1](true, testMessage, testMessage)

		s.True(s.loggingSender.HasMessage())
		s.True(s.stdSender.HasMessage())
		lgrMsg := s.loggingSender.GetMessage()
		stdMsg := s.stdSender.GetMessage()
		s.Equal(lgrMsg.Rendered, stdMsg.Rendered,
			fmt.Sprintf("%s: \n\tlogger: %+v \n\tstandard: %+v", kind, lgrMsg, stdMsg))
	}
}

func (s *LoggingMethodSuite) TestfMethods() {
	cases := map[string][]fMethod{
		"emergency": []fMethod{Emergencyf, s.logger.Emergencyf},
		"alert":     []fMethod{Alertf, s.logger.Alertf},
		"critical":  []fMethod{Criticalf, s.logger.Criticalf},
		"error":     []fMethod{Errorf, s.logger.Errorf},
		"warning":   []fMethod{Warningf, s.logger.Warningf},
		"notice":    []fMethod{Noticef, s.logger.Noticef},
		"info":      []fMethod{Infof, s.logger.Infof},
		"debug":     []fMethod{Debugf, s.logger.Debugf},
	}

	for kind, loggers := range cases {
		s.Len(loggers, 2)
		s.False(s.loggingSender.HasMessage())
		s.False(s.stdSender.HasMessage())

		loggers[0]("%s: %d", testMessage, 3)
		loggers[1]("%s: %d", testMessage, 3)

		s.True(s.loggingSender.HasMessage())
		s.True(s.stdSender.HasMessage())
		lgrMsg := s.loggingSender.GetMessage()
		stdMsg := s.stdSender.GetMessage()
		s.Equal(lgrMsg.Rendered, stdMsg.Rendered,
			fmt.Sprintf("%s: \n\tlogger: %+v \n\tstandard: %+v", kind, lgrMsg, stdMsg))
	}
}

func (s *LoggingMethodSuite) TestManyMethods() {
	msgs := []message.Composer{
		message.NewString("three"),
		message.NewString("two"),
		message.NewString("one"),
	}

	cases := map[string][]manyMethod{
		"emergency": []manyMethod{EmergencyMany, s.logger.EmergencyMany},
		"alert":     []manyMethod{AlertMany, s.logger.AlertMany},
		"critical":  []manyMethod{CriticalMany, s.logger.CriticalMany},
		"error":     []manyMethod{ErrorMany, s.logger.ErrorMany},
		"warning":   []manyMethod{WarningMany, s.logger.WarningMany},
		"notice":    []manyMethod{NoticeMany, s.logger.NoticeMany},
		"info":      []manyMethod{InfoMany, s.logger.InfoMany},
		"debug":     []manyMethod{DebugMany, s.logger.DebugMany},
	}

	for kind, loggers := range cases {
		s.Len(loggers, 2)
		s.False(s.loggingSender.HasMessage())
		s.False(s.stdSender.HasMessage())

		loggers[0](msgs...)
		loggers[1](msgs...)

		for i := 0; i < len(msgs); i++ {
			s.True(s.loggingSender.HasMessage())
			s.True(s.stdSender.HasMessage())
			lgrMsg := s.loggingSender.GetMessage()
			stdMsg := s.stdSender.GetMessage()
			s.Equal(lgrMsg.Rendered, stdMsg.Rendered,
				fmt.Sprintf("%s: \n\tlogger: %+v \n\tstandard: %+v", kind, lgrMsg, stdMsg))
		}
	}
}

func (s *LoggingMethodSuite) TestManyWhenMethods() {
	msgs := []message.Composer{
		message.NewString("one"),
		message.NewString("two"),
		message.NewString("three"),
	}

	cases := map[string][]manyWhenMethod{
		"emergency": []manyWhenMethod{EmergencyManyWhen, s.logger.EmergencyManyWhen},
		"alert":     []manyWhenMethod{AlertManyWhen, s.logger.AlertManyWhen},
		"critical":  []manyWhenMethod{CriticalManyWhen, s.logger.CriticalManyWhen},
		"error":     []manyWhenMethod{ErrorManyWhen, s.logger.ErrorManyWhen},
		"warning":   []manyWhenMethod{WarningManyWhen, s.logger.WarningManyWhen},
		"notice":    []manyWhenMethod{NoticeManyWhen, s.logger.NoticeManyWhen},
		"info":      []manyWhenMethod{InfoManyWhen, s.logger.InfoManyWhen},
		"debug":     []manyWhenMethod{DebugManyWhen, s.logger.DebugManyWhen},
	}

	for kind, loggers := range cases {
		s.Len(loggers, 2)
		s.False(s.loggingSender.HasMessage())
		s.False(s.stdSender.HasMessage())

		loggers[0](true, msgs...)
		loggers[1](true, msgs...)

		for i := 0; i < len(msgs); i++ {
			s.True(s.loggingSender.HasMessage())
			s.True(s.stdSender.HasMessage())
			lgrMsg := s.loggingSender.GetMessage()
			stdMsg := s.stdSender.GetMessage()
			s.Equal(lgrMsg.Rendered, stdMsg.Rendered,
				fmt.Sprintf("%s: \n\tlogger: %+v \n\tstandard: %+v", kind, lgrMsg, stdMsg))
		}

		s.False(s.loggingSender.HasMessage())
		s.False(s.stdSender.HasMessage())
		loggers[0](false, msgs...)
		loggers[1](false, msgs...)

		for i := 0; i < len(msgs); i++ {
			s.False(s.loggingSender.HasMessage())
			s.False(s.stdSender.HasMessage())
		}
	}
}

func (s *LoggingMethodSuite) TestProgramaticLevelMethods() {
	type (
		lgwhen     func(bool, level.Priority, interface{})
		lgwhenln   func(bool, level.Priority, ...interface{})
		lgwhenf    func(bool, level.Priority, string, ...interface{})
		lgmanywhen func(bool, level.Priority, ...message.Composer)
		lg         func(level.Priority, interface{})
		lgln       func(level.Priority, ...interface{})
		lgf        func(level.Priority, string, ...interface{})
		lgmany     func(level.Priority, ...message.Composer)
	)

	cases := map[string]interface{}{
		"when":     []lgwhen{LogWhen, s.logger.LogWhen},
		"whenln":   []lgwhenln{LogWhenln, s.logger.LogWhenln},
		"whenf":    []lgwhenf{LogWhenf, s.logger.LogWhenf},
		"manywhen": []lgmanywhen{LogManyWhen, s.logger.LogManyWhen},
		"lg":       []lg{Log, s.logger.Log},
		"lgln":     []lgln{Logln, s.logger.Logln},
		"lgf":      []lgf{Logf, s.logger.Logf},
		"lgmany":   []lgmany{LogMany, s.logger.LogMany},
	}

	const l = level.Emergency

	for kind, loggers := range cases {
		s.Len(loggers, 2)
		s.False(s.loggingSender.HasMessage())
		s.False(s.stdSender.HasMessage())

		switch log := loggers.(type) {
		case []lgwhen:
			log[0](true, l, testMessage)
			log[1](true, l, testMessage)
		case []lgwhenln:
			log[0](true, l, testMessage, "->", testMessage)
			log[1](true, l, testMessage, "->", testMessage)
		case []lgwhenf:
			log[0](true, l, "%T: (%s) %s", log, kind, testMessage)
			log[1](true, l, "%T: (%s) %s", log, kind, testMessage)
		case []lg:
			log[0](l, testMessage)
			log[1](l, testMessage)
		case []lgln:
			log[0](l, testMessage, "->", testMessage)
			log[1](l, testMessage, "->", testMessage)
		case []lgf:
			log[0](l, "%T: (%s) %s", log, kind, testMessage)
			log[1](l, "%T: (%s) %s", log, kind, testMessage)
		case []lgmany:
			log[0](l, message.ConvertToComposer(l, testMessage))
			log[1](l, message.ConvertToComposer(l, testMessage))
		case []lgmanywhen:
			log[0](true, l, message.ConvertToComposer(l, testMessage))
			log[1](true, l, message.ConvertToComposer(l, testMessage))
		default:
			panic("testing error")
		}

		s.True(s.loggingSender.HasMessage())
		s.True(s.stdSender.HasMessage())
		lgrMsg := s.loggingSender.GetMessage()
		stdMsg := s.stdSender.GetMessage()
		s.Equal(lgrMsg.Rendered, stdMsg.Rendered,
			fmt.Sprintf("%s: \n\tlogger: %+v \n\tstandard: %+v", kind, lgrMsg, stdMsg))
	}
}
