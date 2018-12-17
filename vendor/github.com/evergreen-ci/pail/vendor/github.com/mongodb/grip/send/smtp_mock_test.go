package send

import (
	"bytes"
	"errors"
	"io"
)

type bufferCloser struct {
	*bytes.Buffer
}

func (bufferCloser) Close() error {
	return nil
}

type smtpClientMock struct {
	failCreate bool
	failMail   bool
	failRcpt   bool
	failData   bool
	message    bufferCloser
	numMsgs    int
}

func (c *smtpClientMock) Create(opts *SMTPOptions) error {
	if c.failCreate {
		return errors.New("failed creation")
	}

	return nil
}

func (c *smtpClientMock) Mail(to string) error {
	if c.failMail {
		return errors.New("failed to send mail")
	}

	return nil
}

func (c *smtpClientMock) Rcpt(addr string) error {
	if c.failRcpt {
		return errors.New("fail recpt")
	}

	return nil
}

func (c *smtpClientMock) Data() (io.WriteCloser, error) {
	if c.failData {
		return nil, errors.New("failed data")
	}
	c.message = bufferCloser{&bytes.Buffer{}}
	c.numMsgs++

	return c.message, nil
}

func (c *smtpClientMock) Close() error {
	return nil
}
