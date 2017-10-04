/**
 *  mailer.go
 *
 *  Created on: October 23 2013
 *      Author: Valeri Karpov <valeri.karpov@mongodb.com>
 *
 *  Defines a functional abstraction for sending an email and a few concrete
 *  functions matching that abstraction.
 *
 */

package notify

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net/mail"
	"net/smtp"
	"strings"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type Mailer interface {
	SendMail([]string, string, string) error
}

type SmtpMailer struct {
	From     string
	Server   string
	Port     int
	UseSSL   bool
	Username string
	Password string
}

/* Connects an SMTP server (usually localhost:25 in prod) and uses that to
   send an email with the body encoded in base64. */
func (self SmtpMailer) SendMail(recipients []string, subject, body string) error {
	// 'recipients' is expected a comma-separated list of emails in either of
	// these formats:
	// - bob@example.com
	// - Bob Smith <bob@example.com>
	var c *smtp.Client
	var err error

	if self.UseSSL {
		var tlsCon *tls.Conn

		tlsCon, err = tls.Dial("tcp", fmt.Sprintf("%v:%v", self.Server, self.Port), &tls.Config{})
		if err != nil {
			return err
		}
		c, err = smtp.NewClient(tlsCon, self.Server)
	} else {
		c, err = smtp.Dial(fmt.Sprintf("%v:%v", self.Server, self.Port))
	}

	if err != nil {
		return err
	}

	if self.Username != "" {
		err = c.Auth(smtp.PlainAuth("", self.Username, self.Password, self.Server))
		if err != nil {
			return err
		}
	}

	// Set the sender
	from := mail.Address{
		Name:    "Evergreen Alerts",
		Address: self.From,
	}

	if err = c.Mail(self.From); err != nil {
		grip.Errorf("Error establishing mail sender (%s): %+v", self.From, err)
		return err
	}

	// Set the recipient
	for _, recipient := range recipients {
		err = c.Rcpt(recipient)
		if err != nil {
			grip.Errorf("Error establishing mail recipient (%s): %+v", recipient, err)
			return err
		}
	}

	// Send the email body.
	wc, err := c.Data()
	if err != nil {
		return err
	}
	defer wc.Close()

	contents := []string{
		fmt.Sprintf("From: %s", from.String()),
		fmt.Sprintf("To: %s", strings.Join(recipients, ", ")),
		fmt.Sprintf("Subject: %s", subject),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=\"utf-8\"",
		"Content-Transfer-Encoding: base64",
		"",
		base64.StdEncoding.EncodeToString([]byte(body)),
	}

	_, err = bytes.NewBufferString(strings.Join(contents, "\r\n")).WriteTo(wc)

	return errors.WithStack(err)
}
