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
	"10gen.com/mci"
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"net/mail"
	"net/smtp"
	"strings"
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
		tlsCon, err := tls.Dial("tcp", fmt.Sprintf("%v:%v", self.Server, self.Port), &tls.Config{})
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
	from := mail.Address{"MCI Notifications", self.From}
	err = c.Mail(self.From)
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error establishing mail sender (%v): %v", self.From, err)
		return err
	}

	// Set the recipient
	for _, recipient := range recipients {
		err = c.Rcpt(recipient)
		if err != nil {
			mci.Logger.Errorf(slogger.ERROR, "Error establishing mail recipient (%v): %v", recipient, err)
			return err
		}
	}

	// Send the email body.
	wc, err := c.Data()
	if err != nil {
		return err
	}
	defer wc.Close()

	// set header information
	header := make(map[string]string)
	header["From"] = from.String()
	header["To"] = strings.Join(recipients, ", ")
	header["Subject"] = encodeRFC2047(subject)
	header["MIME-Version"] = "1.0"
	header["Content-Type"] = "text/html; charset=\"utf-8\""
	header["Content-Transfer-Encoding"] = "base64"

	message := ""
	for k, v := range header {
		message += fmt.Sprintf("%s: %s\r\n", k, v)
	}

	message += "\r\n" + base64.StdEncoding.EncodeToString([]byte(body))

	// write the body
	buf := bytes.NewBufferString(message)
	if _, err = buf.WriteTo(wc); err != nil {
		return err
	}
	return nil
}
