package send

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"net/mail"
	"net/smtp"
	"os"
	"strings"
	"sync"

	"github.com/mongodb/grip/message"
)

type smtpLogger struct {
	opts *SMTPOptions
	*Base
}

// NewSMTPLogger constructs a Sender implementation that delivers mail
// for every loggable message. The configuration of the outgoing SMTP
// server, and the formatting of the log message is handled by the
// SMTPOptions structure, which you must use to configure this sender.
func NewSMTPLogger(opts *SMTPOptions, l LevelInfo) (Sender, error) {
	s, err := MakeSMTPLogger(opts)
	if err != nil {
		return nil, err
	}

	if err = s.SetLevel(l); err != nil {
		return nil, err
	}

	return s, nil
}

// MakeSMTPLogger constructs an unconfigured (e.g. without level information)
// SMTP Sender implementation that delivers mail for every loggable message.
// The configuration of the outgoing SMTP server, and the formatting of the
// log message is handled by the SMTPOptions structure, which you must use
// to configure this sender.
func MakeSMTPLogger(opts *SMTPOptions) (Sender, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	s := &smtpLogger{
		Base: NewBase(opts.Name),
		opts: opts,
	}

	fallback := log.New(os.Stdout, "", log.LstdFlags)
	if err := s.SetErrorHandler(ErrorHandlerFromLogger(fallback)); err != nil {
		return nil, err
	}

	s.reset = func() {
		fallback.SetPrefix(fmt.Sprintf("[%s] ", s.Name()))
	}

	s.SetName(opts.Name)

	return s, nil
}

func (s *smtpLogger) Send(m message.Composer) {
	if s.Level().ShouldLog(m) {
		if err := s.opts.sendMail(m); err != nil {
			s.ErrorHandler()(err, m)
		}
	}
}

func (s *smtpLogger) Flush(_ context.Context) error { return nil }

///////////////////////////////////////////////////////////////////////////
//
// Implemntation of the Configuration Object
//
///////////////////////////////////////////////////////////////////////////

// The configuration object handles the (internal) implementation of
// generating and sending email.

// SMTPOptions configures the behavior of the SMTP logger. There is no
// explicit constructor; however, the Validate method (which is called
// by the Sender's constructor) checks for incomplete configuration
// and sets default options if they are unset.
//
// In additional to constructing this object with the necessary
// options. You must also set at least one recipient address using the
// AddRecipient or AddRecipients functions. You can add or reset the
// recipients after configuring the options or the sender.
type SMTPOptions struct { // nolint
	// Name controls both the name of the logger, and the name on
	// the from header field.
	Name string
	// From specifies the address specified.
	From string
	// Server, Port, and UseSSL control how we connect to the SMTP
	// server. If unspecified, these options default to
	// "localhost", port, and false.
	Server string
	Port   int
	UseSSL bool
	// Username and password define how the client authenticates
	// to the SMTP server. If no Username is specified, the client
	// will not authenticate to the server.
	Username string
	Password string

	// These options control the output behavior. You must specify
	// a subject for the emails, *or* one of the bool options that
	// specify how to generate the subject (e.g. NameAsSubject,
	// MessageAsSubject) you can specify a non-zero value to
	// TruncatedMessageSubjectLength to use a fragment of the
	// message as the subject. The use of these options is
	// depended on the implementation of the GetContents function.
	//
	// The GetContents function returns a pair of strings (subject,
	// body), and defaults to an implementation that generates a
	// plain text email according to the (Subject,
	// TruncatedMessageSubjectLength, NameAsSubject, and
	// MessageAsSubject). If you use the default GetContents
	// implementation, PlainTextContents is set to false.
	//
	// You can define your own implementation of the GetContents
	// function, to use a template, or to generate an HTML email
	// as needed, you must also set PlainTextContents as needed.
	GetContents                   func(*SMTPOptions, message.Composer) (string, string)
	Subject                       string
	TruncatedMessageSubjectLength int
	NameAsSubject                 bool
	MessageAsSubject              bool
	PlainTextContents             bool

	client   smtpClient
	fromAddr *mail.Address
	toAddrs  []*mail.Address
	mutex    sync.Mutex
}

// ResetRecipients removes all recipients from the configuration
// object. You can reset the recipients at any time, but you must have
// at least one recipient configured when you use this options object to
// configure a sender *or* attempt to send a message.
func (o *SMTPOptions) ResetRecipients() {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	o.toAddrs = []*mail.Address{}
}

// AddRecipient takes a name and email address as an argument and
// attempts to parse a valid email address from this data, and if
// valid adds this email address.
func (o *SMTPOptions) AddRecipient(name, address string) error {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	addr, err := mail.ParseAddress(fmt.Sprintf("%s <%s>", name, address))
	if err != nil {
		return err
	}

	o.toAddrs = append(o.toAddrs, addr)
	return nil
}

// AddRecipients accepts one or more string that can be, itself, comma
// separated lists of email addresses, which are then added to the
// recipients for the logger.
func (o *SMTPOptions) AddRecipients(addresses ...string) error {
	if len(addresses) == 0 {
		return errors.New("AddRecipients requires one or more strings that contain comma " +
			"separated email addresses")
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()

	addrs, err := mail.ParseAddressList(strings.Join(addresses, ","))
	if err != nil {
		return err
	}

	o.toAddrs = append(o.toAddrs, addrs...)

	return nil
}

// Validate checks the contents of the SMTPOptions struct and sets
// default values in appropriate cases. Returns an error if the struct
// is not valid. The constructor for the SMTP sender calls this method
// to make sure that options struct is valid before initiating the sender.
func (o *SMTPOptions) Validate() error {
	if o == nil {
		return errors.New("must specify non-nil SMTP options")
	}

	o.mutex.Lock()
	defer o.mutex.Unlock()

	// setup defaults

	if o.Port == 0 {
		o.Port = 25
	}

	if o.Server == "" {
		o.Server = "localhost"
	}

	if o.client == nil {
		o.client = &smtpClientImpl{}
	}

	if o.GetContents == nil {
		o.PlainTextContents = true
		o.GetContents = func(opts *SMTPOptions, m message.Composer) (string, string) {
			msg := m.String()

			// we can assume that it's valid because Validate has already run
			if o.MessageAsSubject {
				return m.String(), ""
			}

			if o.NameAsSubject {
				return opts.Name, msg
			}

			if o.TruncatedMessageSubjectLength > 0 {
				if len(msg) <= opts.TruncatedMessageSubjectLength {
					return msg, msg
				}

				return msg[:opts.TruncatedMessageSubjectLength-1], msg
			}

			if o.Subject != "" {
				return o.Subject, msg
			}

			return "", msg
		}
	}

	o.fromAddr = &mail.Address{
		Name:    o.Name,
		Address: o.From,
	}

	// validate user configuration options

	errs := []string{}
	if !o.MessageAsSubject && !o.NameAsSubject && o.TruncatedMessageSubjectLength == 0 && o.Subject == "" {
		errs = append(errs, "no subject policy defined in SMTP options")
	}

	if o.NameAsSubject && o.MessageAsSubject {
		errs = append(errs, "conflicting message subject policy defined")
	}

	if o.Name == "" {
		errs = append(errs, "no name specified")
	}

	if len(o.toAddrs) < 1 {
		errs = append(errs, "no recipient addresses defined.")
	}

	// put additional pre-flight checks above this line, as needed.

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}

	return nil
}

/* Connects an SMTP server (usually localhost:25 in prod) and uses that to
   send an email with the body encoded in base64. */
func (o *SMTPOptions) sendMail(m message.Composer) error {
	o.mutex.Lock()
	defer o.mutex.Unlock()

	if err := o.client.Create(o); err != nil {
		return err
	}
	defer o.client.Close()

	var subject, body string
	toAddrs := o.toAddrs
	fromAddr := o.fromAddr
	isPlainText := o.PlainTextContents
	var headers map[string][]string

	if emailMsg, ok := m.Raw().(*message.Email); ok {
		var err error
		if len(emailMsg.From) != 0 {
			fromAddr, err = mail.ParseAddress(emailMsg.From)
			if err != nil {
				return fmt.Errorf("invalid from address: %+v", err)
			}
		}

		toAddrs = make([]*mail.Address, len(emailMsg.Recipients))

		for i := range emailMsg.Recipients {
			toAddrs[i], err = mail.ParseAddress(emailMsg.Recipients[i])
			if err != nil {
				return fmt.Errorf("invalid recipient: %+v", err)
			}
		}

		subject = emailMsg.Subject
		body = emailMsg.Body
		isPlainText = emailMsg.PlainTextContents
		headers = emailMsg.Headers
	}
	if fromAddr == nil {
		return fmt.Errorf("no from address specified, cannot send mail")
	}
	if len(toAddrs) == 0 {
		return fmt.Errorf("no recipients specified, cannot send mail")
	}

	if err := o.client.Mail(fromAddr.Address); err != nil {
		return fmt.Errorf("error establishing mail sender (%s): %+v", fromAddr, err)
	}

	var err error
	var errs []string
	var recipients []string

	// Set the recipients
	for _, target := range toAddrs {
		if err = o.client.Rcpt(target.Address); err != nil {
			errs = append(errs,
				fmt.Sprintf("Error establishing mail recipient (%s): %+v", target.String(), err))
			continue
		}
		recipients = append(recipients, target.String())
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}

	// Send the email body.
	wc, err := o.client.Data()
	if err != nil {
		return err
	}
	defer wc.Close()

	if len(subject) == 0 && len(body) == 0 {
		subject, body = o.GetContents(o, m)
	}

	contents := []string{
		fmt.Sprintf("From: %s", fromAddr.String()),
		fmt.Sprintf("To: %s", strings.Join(recipients, ", ")),
		fmt.Sprintf("Subject: %s", subject),
		"MIME-Version: 1.0",
	}

	skipContentType := false
	for k, v := range headers {
		if k == "To" || k == "From" || k == "Subject" || k == "Content-Transfer-Encoding" {
			continue
		}
		if k == "Content-Type" {
			skipContentType = true
		}
		for i := range v {
			contents = append(contents, fmt.Sprintf("%s: %s", k, v[i]))
		}
	}

	if !skipContentType {
		if isPlainText {
			contents = append(contents, "Content-Type: text/plain; charset=\"utf-8\"")
		} else {
			contents = append(contents, "Content-Type: text/html; charset=\"utf-8\"")
		}
	}

	contents = append(contents,
		"Content-Transfer-Encoding: base64",
		base64.StdEncoding.EncodeToString([]byte(body)))

	// write the body
	_, err = bytes.NewBufferString(strings.Join(contents, "\r\n")).WriteTo(wc)
	return err
}

////////////////////////////////////////////////////////////////////////
//
// interface mock for testability
//
////////////////////////////////////////////////////////////////////////

type smtpClient interface {
	Create(*SMTPOptions) error
	Mail(string) error
	Rcpt(string) error
	Data() (io.WriteCloser, error)
	Close() error
}

type smtpClientImpl struct {
	*smtp.Client
}

func (c *smtpClientImpl) Create(opts *SMTPOptions) error {
	var err error
	c.Client, err = smtp.Dial(fmt.Sprintf("%v:%v", opts.Server, opts.Port))
	if err != nil {
		return err
	}

	if opts.UseSSL {
		config := &tls.Config{ServerName: opts.Server}
		err = c.Client.StartTLS(config)

	} else {
		var hostname string
		hostname, err = os.Hostname()
		if err != nil {
			return err
		}

		err = c.Hello(hostname)
	}
	if err != nil {
		return err
	}

	if opts.Username != "" {
		if err = c.Client.Auth(smtp.PlainAuth("", opts.Username, opts.Password, opts.Server)); err != nil {
			return err
		}
	}

	return nil
}

func (c *smtpClientImpl) Close() error {
	return c.Client.Close()
}
