package send

import (
	"net/mail"
	"runtime"
	"strings"
	"testing"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/suite"
)

type SMTPSuite struct {
	opts *SMTPOptions
	suite.Suite
}

func TestSMTPSuite(t *testing.T) {
	suite.Run(t, new(SMTPSuite))
}

func (s *SMTPSuite) SetupTest() {
	s.opts = &SMTPOptions{
		client:        &smtpClientMock{},
		Subject:       "test email from logger",
		NameAsSubject: true,
		Name:          "test smtp sender",
		toAddrs: []*mail.Address{
			{
				Name:    "one",
				Address: "two",
			},
		},
	}
	s.Nil(s.opts.GetContents)
	s.NoError(s.opts.Validate())
	s.NotNil(s.opts.GetContents)
}

func (s *SMTPSuite) TestOptionsMustBeIValid() {
	invalidOpts := []*SMTPOptions{
		{},
		{
			Subject:          "too many subject uses",
			NameAsSubject:    true,
			MessageAsSubject: true,
		},
		{
			Subject: "missing name",
			toAddrs: []*mail.Address{
				{
					Name:    "one",
					Address: "two",
				},
			},
		},
		{
			Subject: "empty",
			Name:    "sender",
			toAddrs: []*mail.Address{},
		},
	}

	for _, opts := range invalidOpts {
		s.Error(opts.Validate())
	}
}

func (s *SMTPSuite) TestDefaultGetContents() {
	s.NotNil(s.opts)

	m := message.NewString("helllooooo!")
	sbj, msg := s.opts.GetContents(s.opts, m)

	s.True(s.opts.NameAsSubject)
	s.Equal(s.opts.Name, sbj)
	s.Equal(m.String(), msg)

	s.opts.NameAsSubject = false
	sbj, _ = s.opts.GetContents(s.opts, m)
	s.Equal(s.opts.Subject, sbj)

	s.opts.MessageAsSubject = true
	sbj, msg = s.opts.GetContents(s.opts, m)
	s.Equal("", msg)
	s.Equal(m.String(), sbj)
	s.opts.MessageAsSubject = false

	s.opts.Subject = ""
	sbj, msg = s.opts.GetContents(s.opts, m)
	s.Equal("", sbj)
	s.Equal(m.String(), msg)
	s.opts.Subject = "test email subject"

	s.opts.TruncatedMessageSubjectLength = len(m.String()) * 2
	sbj, msg = s.opts.GetContents(s.opts, m)
	s.Equal(m.String(), msg)
	s.Equal(m.String(), sbj)

	s.opts.TruncatedMessageSubjectLength = len(m.String()) - 2
	sbj, msg = s.opts.GetContents(s.opts, m)
	s.Equal(m.String(), msg)
	s.NotEqual(msg, sbj)
	s.True(len(msg) > len(sbj))
}

func (s *SMTPSuite) TestResetRecips() {
	s.True(len(s.opts.toAddrs) > 0)
	s.opts.ResetRecipients()
	s.Len(s.opts.toAddrs, 0)
}

func (s *SMTPSuite) TestAddRecipientsFailsWithNoArgs() {
	s.opts.ResetRecipients()
	s.Error(s.opts.AddRecipients())
	s.Len(s.opts.toAddrs, 0)
}

func (s *SMTPSuite) TestAddRecipientsErrorsWithInvalidAddresses() {
	s.opts.ResetRecipients()
	s.Error(s.opts.AddRecipients("foo", "bar", "baz"))
	s.Len(s.opts.toAddrs, 0)
}

func (s *SMTPSuite) TestAddingMultipleRecipients() {
	s.opts.ResetRecipients()

	s.NoError(s.opts.AddRecipients("test <one@example.net>"))
	s.Len(s.opts.toAddrs, 1)
	s.NoError(s.opts.AddRecipients("test <one@example.net>", "test2 <two@example.net>"))
	s.Len(s.opts.toAddrs, 3)
}

func (s *SMTPSuite) TestAddingSingleRecipientWithInvalidAddressErrors() {
	s.opts.ResetRecipients()
	s.Error(s.opts.AddRecipient("test", "address"))
	s.Len(s.opts.toAddrs, 0)

	if runtime.Compiler != "gccgo" {
		// this panics on gccgo1.4, but is generally an interesting test.
		// not worth digging into a standard library bug that
		// seems fixed on gcgo. and/or in a more recent version.
		s.Error(s.opts.AddRecipient("test", "address"))
		s.Len(s.opts.toAddrs, 0)
	}
}

func (s *SMTPSuite) TestAddingSingleRecipient() {
	s.opts.ResetRecipients()
	s.NoError(s.opts.AddRecipient("test", "one@example.net"))
	s.Len(s.opts.toAddrs, 1)
}

func (s *SMTPSuite) TestMakeConstructorFailureCases() {
	sender, err := MakeSMTPLogger(nil)
	s.Nil(sender)
	s.Error(err)

	sender, err = MakeSMTPLogger(&SMTPOptions{})
	s.Nil(sender)
	s.Error(err)
}

func (s *SMTPSuite) TestSendMailErrorsIfNoAddresses() {
	s.opts.ResetRecipients()
	s.Len(s.opts.toAddrs, 0)

	m := message.NewString("hello world!")
	s.Error(s.opts.sendMail(m))
}

func (s *SMTPSuite) TestSendMailErrorsIfMailCallFails() {
	s.opts.client = &smtpClientMock{
		failMail: true,
	}

	m := message.NewString("hello world!")
	s.Error(s.opts.sendMail(m))
}

func (s *SMTPSuite) TestSendMailErrorsIfRecptFails() {
	s.opts.client = &smtpClientMock{
		failRcpt: true,
	}

	m := message.NewString("hello world!")
	s.Error(s.opts.sendMail(m))
}

func (s *SMTPSuite) TestSendMailErrorsIfDataFails() {
	s.opts.client = &smtpClientMock{
		failData: true,
	}

	m := message.NewString("hello world!")
	s.Error(s.opts.sendMail(m))
}

func (s *SMTPSuite) TestSendMailErrorsIfCreateFails() {
	s.opts.client = &smtpClientMock{
		failCreate: true,
	}

	m := message.NewString("hello world!")
	s.Error(s.opts.sendMail(m))
}

func (s *SMTPSuite) TestSendMailRecordsMessage() {
	m := message.NewString("hello world!")
	s.NoError(s.opts.sendMail(m))
	mock, ok := s.opts.client.(*smtpClientMock)
	s.Require().True(ok)
	s.True(strings.Contains(mock.message.String(), s.opts.Name))
	s.True(strings.Contains(mock.message.String(), "plain"))
	s.False(strings.Contains(mock.message.String(), "html"))

	s.opts.PlainTextContents = false
	s.NoError(s.opts.sendMail(m))
	s.True(strings.Contains(mock.message.String(), s.opts.Name))
	s.True(strings.Contains(mock.message.String(), "html"))
	s.False(strings.Contains(mock.message.String(), "plain"))
}

func (s *SMTPSuite) TestNewConstructor() {
	sender, err := NewSMTPLogger(nil, LevelInfo{level.Trace, level.Info})
	s.Error(err)
	s.Nil(sender)

	sender, err = NewSMTPLogger(s.opts, LevelInfo{level.Invalid, level.Info})
	s.Error(err)
	s.Nil(sender)

	sender, err = NewSMTPLogger(s.opts, LevelInfo{level.Trace, level.Info})
	s.NoError(err)
	s.NotNil(sender)
}

func (s *SMTPSuite) TestSendMethod() {
	sender, err := NewSMTPLogger(s.opts, LevelInfo{level.Trace, level.Info})
	s.NoError(err)
	s.NotNil(sender)

	mock, ok := s.opts.client.(*smtpClientMock)
	s.True(ok)
	s.Equal(mock.numMsgs, 0)

	m := message.NewDefaultMessage(level.Debug, "hello")
	sender.Send(m)
	s.Equal(mock.numMsgs, 0)

	m = message.NewDefaultMessage(level.Alert, "")
	sender.Send(m)
	s.Equal(mock.numMsgs, 0)

	m = message.NewDefaultMessage(level.Alert, "world")
	sender.Send(m)
	s.Equal(mock.numMsgs, 1)
}

func (s *SMTPSuite) TestSendMethodWithError() {
	sender, err := NewSMTPLogger(s.opts, LevelInfo{level.Trace, level.Info})
	s.NoError(err)
	s.NotNil(sender)

	mock, ok := s.opts.client.(*smtpClientMock)
	s.True(ok)
	s.Equal(mock.numMsgs, 0)
	s.False(mock.failData)

	m := message.NewDefaultMessage(level.Alert, "world")
	sender.Send(m)
	s.Equal(mock.numMsgs, 1)

	mock.failData = true
	sender.Send(m)
	s.Equal(mock.numMsgs, 1)
}

func (s *SMTPSuite) TestSendMethodWithEmailComposerOverridesSMTPOptions() {
	sender, err := NewSMTPLogger(s.opts, LevelInfo{level.Trace, level.Info})
	s.NoError(err)
	s.NotNil(sender)

	s.NoError(sender.SetErrorHandler(func(err error, m message.Composer) {
		s.T().Errorf("unexpected error in sender: %+v", err)
		s.T().FailNow()
	}))

	mock, ok := s.opts.client.(*smtpClientMock)
	s.True(ok)
	s.Equal(0, mock.numMsgs)
	m := message.NewEmailMessage(level.Notice, message.Email{
		From:              "Mr Super Powers <from@example.com>",
		Recipients:        []string{"to@example.com"},
		Subject:           "Test",
		Body:              "just a test",
		PlainTextContents: true,
		Headers: map[string][]string{
			"X-Custom-Header":           []string{"special"},
			"Content-Type":              []string{"something/proprietary"},
			"Content-Transfer-Encoding": []string{"somethingunexpected"},
		},
	})
	s.True(m.Loggable())

	sender.Send(m)
	s.Equal(1, mock.numMsgs)

	contains := []string{
		"From: \"Mr Super Powers\" <from@example.com>\r\n",
		"To: <to@example.com>\r\n",
		"Subject: Test\r\n",
		"MIME-Version: 1.0\r\n",
		"Content-Type: something/proprietary\r\n",
		"X-Custom-Header: special\r\n",
		"Content-Transfer-Encoding: base64\r\n",
		"anVzdCBhIHRlc3Q=",
	}
	data := mock.message.String()
	for i := range contains {
		s.Contains(data, contains[i])
	}
}
