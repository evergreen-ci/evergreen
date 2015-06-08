package alerts

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/alertrecord"
	"github.com/evergreen-ci/render"
	"net/mail"
	"net/smtp"
	"strings"
)

const EmailSubjectPrologue = "[Evergreen]"

type SMTPSettings struct {
	From     string
	Server   string
	Port     int
	UseSSL   bool
	Username string
	Password string
}

// EmailDeliverer is an implementation of Deliverer that sends notifications to an SMTP server
type EmailDeliverer struct {
	SMTPSettings
	render *render.Render
}

func (es *EmailDeliverer) Deliver(alertCtx AlertContext, alertConf model.AlertConfig) error {
	rcptRaw, ok := alertConf.Settings["recipient"]
	if !ok {
		return fmt.Errorf("missing email address")
	}
	evergreen.Logger.Logf(slogger.INFO, "Sending email to %v", rcptRaw)

	var rcpt string
	if rcpt, ok = rcptRaw.(string); !ok {
		return fmt.Errorf("email address must be a string")
	}

	var err error
	subject := getSubject(alertCtx)
	body, err := es.getBody(alertCtx)
	if err != nil {
		return err
	}

	var c *smtp.Client
	if es.UseSSL {
		tlsCon, err := tls.Dial("tcp", fmt.Sprintf("%v:%v", es.Server, es.Port), &tls.Config{})
		if err != nil {
			return err
		}
		c, err = smtp.NewClient(tlsCon, es.Server)
	} else {
		c, err = smtp.Dial(fmt.Sprintf("%v:%v", es.Server, es.Port))
	}

	if err != nil {
		return err
	}

	if es.Username != "" {
		err = c.Auth(smtp.PlainAuth("", es.Username, es.Password, es.Server))
		if err != nil {
			return err
		}
	}

	// Set the sender
	from := mail.Address{"Evergreen Alerts", es.From}
	err = c.Mail(es.From)
	if err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "Error establishing mail sender (%v): %v", es.From, err)
		return err
	}

	err = c.Rcpt(rcpt)
	if err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "Error establishing mail recipient (%v): %v", rcpt, err)
		return err
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
	header["To"] = rcpt
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

func getTemplate(alertCtx AlertContext) string {
	switch alertCtx.AlertRequest.Trigger {
	case alertrecord.SpawnHostTwoHourWarning:
		fallthrough
	case alertrecord.SpawnHostTwelveHourWarning:
		return "email/host_spawn.html"
	default:
		return "email/task_fail.html"
	}
}

// getBody executes a template with the alert data and returns the body of the notification
// e-mail to be sent as an HTML string.
func (es *EmailDeliverer) getBody(alertCtx AlertContext) (string, error) {
	out := &bytes.Buffer{}
	template := getTemplate(alertCtx)
	err := es.render.HTML(out, alertCtx, "content", template)
	if err != nil {
		return "", err
	}
	return out.String(), nil
}

// getSubject generates a subject line for an e-mail for the given alert.
func getSubject(alertCtx AlertContext) string {
	switch alertCtx.AlertRequest.Trigger {
	case alertrecord.FirstVersionFailureId:
		return fmt.Sprintf("First Task Failure %s in %s @ %s: '%s' on %s",
			alertCtx.ProjectRef.DisplayName,
			alertCtx.Version.Revision[0:5],
			alertCtx.Task.DisplayName,
			alertCtx.Build.DisplayName)
	case alertrecord.FirstVariantFailureId:
		return fmt.Sprintf("Variant '%s' has failures (%s @ %s)",
			alertCtx.Build.DisplayName,
			alertCtx.ProjectRef.DisplayName,
			alertCtx.Version.Revision[0:5],
		)
	case alertrecord.FirstTaskTypeFailureId:
		return fmt.Sprintf("Task '%s' has failures (%s @ %s)",
			alertCtx.Task.DisplayName,
			alertCtx.ProjectRef.DisplayName,
			alertCtx.Version.Revision[0:5],
		)
	case alertrecord.TaskFailTransitionId:
		return fmt.Sprintf("Task '%s' status transitioned to failure on %s (%s @ %s)",
			alertCtx.Task.DisplayName,
			alertCtx.Build.DisplayName,
			alertCtx.ProjectRef.DisplayName,
			alertCtx.Version.Revision[0:5],
		)
	case alertrecord.SpawnHostTwoHourWarning:
		return fmt.Sprintf("Your %s host (%s) will expire in two hours.",
			alertCtx.Host.Distro, alertCtx.Host.Id)
	case alertrecord.SpawnHostTwelveHourWarning:
		return fmt.Sprintf("Your %s host (%s) will expire in twelve hours.",
			alertCtx.Host.Distro, alertCtx.Host.Id)
		// TODO
		//case alertrecord.SpawnHostExpired:
		//return fmt.Sprintf("Your %s host (%s) has expired.", alertCtx.Host.Distro, alertCtx.Host.Id)
	}

	return fmt.Sprintf("%s on %s (%s @ %s)",
		alertCtx.Task.DisplayName,
		alertCtx.Build.DisplayName,
		alertCtx.ProjectRef.DisplayName,
		alertCtx.Version.Revision[0:5])

}

func encodeRFC2047(String string) string {
	addr := mail.Address{String, ""}
	return strings.Trim(addr.String(), " <>")
}
