package alerts

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"fmt"
	"net/mail"
	"net/smtp"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/alertrecord"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	EmailSubjectPrologue = "[Evergreen]"
)

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
	render gimlet.Renderer
}

func (es *EmailDeliverer) Deliver(alertCtx AlertContext, alertConf model.AlertConfig) error {
	rcptRaw, ok := alertConf.Settings["recipient"]
	if !ok {
		return errors.New("missing email address")
	}

	grip.Info(message.Fields{
		"operation": "sending email",
		"recipient": rcptRaw,
		"runner":    RunnerName,
	})

	var rcpt string
	if rcpt, ok = rcptRaw.(string); !ok {
		return errors.New("email address must be a string")
	}

	var err error
	subject := getSubject(alertCtx)
	body, err := es.getBody(alertCtx)
	if err != nil {
		return err
	}

	var c *smtp.Client
	var tlsCon *tls.Conn
	if es.UseSSL {
		tlsCon, err = tls.Dial("tcp", fmt.Sprintf("%v:%v", es.Server, es.Port), &tls.Config{})
		if err != nil {
			return err
		}
		c, err = smtp.NewClient(tlsCon, es.Server)
		if err != nil {
			return err
		}
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
	from := mail.Address{
		Name:    "Evergreen Alerts",
		Address: es.From,
	}
	err = c.Mail(es.From)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"runner":  RunnerName,
			"message": "problem connecting to mail sender",
			"from":    es.From,
		}))

		return errors.WithStack(err)
	}

	err = c.Rcpt(rcpt)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"runner":    RunnerName,
			"message":   "error establishing mail recipient",
			"recipient": rcpt,
		}))
		return errors.WithStack(err)
	}

	// Send the email body.
	wc, err := c.Data()
	if err != nil {
		return errors.WithStack(err)
	}
	defer wc.Close()

	// set header information
	header := make(map[string]string)
	header["From"] = from.String()
	header["To"] = rcpt
	header["Subject"] = subject
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
		return errors.WithStack(err)
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
	err := es.render.Render(out, alertCtx, "content", template)
	if err != nil {
		return "", errors.WithStack(err)
	}
	return out.String(), nil
}

// taskFailureSubject creates an email subject for a task failure in the style of
//  Test Failures: Task_name on Variant (test1, test2) // ProjectName @ githash
// based on the given AlertContext.
func taskFailureSubject(ctx AlertContext) string {
	subj := &bytes.Buffer{}
	failed := []string{}
	for _, test := range ctx.Task.LocalTestResults {
		if test.Status == evergreen.TestFailedStatus {
			failed = append(failed, cleanTestName(test.TestFile))
		}
	}

	switch {
	case ctx.Task.Details.TimedOut:
		subj.WriteString("Task Timed Out: ")
	case len(failed) == 1:
		subj.WriteString("Test Failure: ")
	case len(failed) > 1:
		subj.WriteString("Test Failures: ")
	case ctx.Task.Details.Description == task.AgentHeartbeat:
		subj.WriteString("Task System Failure: ")
	case ctx.Task.Details.Type == evergreen.CommandTypeSystem:
		subj.WriteString("Task System Failure: ")
	case ctx.Task.Details.Type == evergreen.CommandTypeSetup:
		subj.WriteString("Task Setup Failure: ")
	default:
		subj.WriteString("Task Failed: ")
	}

	fmt.Fprintf(subj, "%s on %s ", ctx.Task.DisplayName, ctx.Build.DisplayName)

	// include test names if <= 4 failed, otherwise print two plus the number remaining
	if len(failed) > 0 {
		subj.WriteString("(")
		if len(failed) <= 4 {
			subj.WriteString(strings.Join(failed, ", "))
		} else {
			fmt.Fprintf(subj, "%s, %s, +%v more", failed[0], failed[1], len(failed)-2)
		}
		subj.WriteString(") ")
	}

	fmt.Fprintf(subj, "// %s @ %s", ctx.ProjectRef.DisplayName, ctx.Version.Revision[0:8])
	return subj.String()
}

// getSubject generates a subject line for an e-mail for the given alert.
func getSubject(alertCtx AlertContext) string {
	switch alertCtx.AlertRequest.Trigger {
	case alertrecord.FirstVersionFailureId:
		return fmt.Sprintf("First Task Failure: %s on %s // %s @ %s",
			alertCtx.Task.DisplayName,
			alertCtx.Build.DisplayName,
			alertCtx.ProjectRef.DisplayName,
			alertCtx.Version.Revision[0:8])
	case alertrecord.FirstVariantFailureId:
		return fmt.Sprintf("Variant Failure: %s // %s @ %s",
			alertCtx.Build.DisplayName,
			alertCtx.ProjectRef.DisplayName,
			alertCtx.Version.Revision[0:8],
		)
	case alertrecord.SpawnHostTwoHourWarning:
		return fmt.Sprintf("Your %s host (%s) will expire in two hours.",
			alertCtx.Host.Distro.Id, alertCtx.Host.Id)
	case alertrecord.SpawnHostTwelveHourWarning:
		return fmt.Sprintf("Your %s host (%s) will expire in twelve hours.",
			alertCtx.Host.Distro.Id, alertCtx.Host.Id)
		// TODO(EVG-224) alertrecord.SpawnHostExpired:
	}
	return taskFailureSubject(alertCtx)
}

// cleanTestName returns the last item of a test's path.
//   TODO: stop accommodating this.
func cleanTestName(path string) string {
	if unixIdx := strings.LastIndex(path, "/"); unixIdx != -1 {
		// if the path ends in a slash, remove it and try again
		if unixIdx == len(path)-1 {
			return cleanTestName(path[:len(path)-1])
		}
		return path[unixIdx+1:]
	}
	if windowsIdx := strings.LastIndex(path, `\`); windowsIdx != -1 {
		// if the path ends in a slash, remove it and try again
		if windowsIdx == len(path)-1 {
			return cleanTestName(path[:len(path)-1])
		}
		return path[windowsIdx+1:]
	}
	return path
}
