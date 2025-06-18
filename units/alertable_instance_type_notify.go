package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const alertableInstanceTypeNotifyJobName = "alertable-instance-type-notify"

func init() {
	registry.AddJobType(alertableInstanceTypeNotifyJobName, func() amboy.Job {
		return makeAlertableInstanceTypeNotifyJob()
	})
}

type alertableInstanceTypeNotifyJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment
}

func makeAlertableInstanceTypeNotifyJob() *alertableInstanceTypeNotifyJob {
	j := &alertableInstanceTypeNotifyJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    alertableInstanceTypeNotifyJobName,
				Version: 0,
			},
		},
	}
	return j
}

// NewAlertableInstanceTypeNotifyJob creates a job to check for spawn hosts using alertable instance types
func NewAlertableInstanceTypeNotifyJob(id string) amboy.Job {
	j := makeAlertableInstanceTypeNotifyJob()
	j.SetID(fmt.Sprintf("%s.%s", alertableInstanceTypeNotifyJobName, id))
	return j
}

func (j *alertableInstanceTypeNotifyJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		j.AddError(errors.Wrap(err, "getting evergreen settings"))
		return
	}

	alertableTypes := settings.Providers.AWS.AlertableInstanceTypes
	if len(alertableTypes) == 0 {
		return
	}

	// Find all active spawn hosts
	hosts, err := host.Find(ctx, host.ByUnterminatedSpawnHosts())
	if err != nil {
		j.AddError(errors.Wrap(err, "finding spawn hosts"))
		return
	}

	// Group hosts by user and check for alertable instance types.
	userHosts := make(map[string][]host.Host)
	const alertThreshhold = 72 * time.Hour

	for _, h := range hosts {
		if h.UserHost && h.StartedBy != "" {
			// Check if this host has been using an alertable instance type for longer than 3 days
			if utility.StringSliceContains(alertableTypes, h.InstanceType) {
				if h.LastInstanceEditTime.IsZero() || (time.Since(h.LastInstanceEditTime) >= alertThreshhold) {
					userHosts[h.StartedBy] = append(userHosts[h.StartedBy], h)
				}
			}
		}
	}

	if len(userHosts) == 0 {
		grip.Debug(message.Fields{
			"job_id":   j.ID(),
			"job_type": j.Type().Name,
			"message":  "no spawn hosts using alertable instance types found within the alert threshhold",
		})
		return
	}

	// Send notifications to users and log to Slack
	for userID, userHostList := range userHosts {
		if err := j.notifyUser(userID, userHostList); err != nil {
			j.AddError(errors.Wrapf(err, "notifying user '%s'", userID))
		}
	}

	// Log summary to Splunk for admin monitoring
	j.logNotificationSummary(userHosts)
}

func (j *alertableInstanceTypeNotifyJob) notifyUser(userID string, hosts []host.Host) error {
	// Get user information
	u, err := user.FindOneById(userID)
	if err != nil {
		return errors.Wrapf(err, "finding user '%s'", userID)
	}
	if u == nil {
		return errors.Errorf("user '%s' not found", userID)
	}

	catcher := grip.NewBasicCatcher()
	if err := j.sendEmailNotification(u, hosts); err != nil {
		catcher.Wrap(err, "failed to send email notification")
	}

	if err := j.sendSlackNotification(u, hosts); err != nil {
		catcher.Wrap(err, "failed to send slack notification")
	}

	return catcher.Resolve()
}

func (j *alertableInstanceTypeNotifyJob) sendEmailNotification(u *user.DBUser, hosts []host.Host) error {
	if u.EmailAddress == "" {
		return nil // Skip if no email address
	}

	subject := "Evergreen Large Instance Type Reminder"
	body := j.buildEmailBody(hosts)

	email := message.Email{
		Recipients: []string{u.EmailAddress},
		Subject:    subject,
		Body:       body,
	}

	sender, err := j.env.GetSender(evergreen.SenderEmail)
	if err != nil {
		return errors.Wrap(err, "getting email sender")
	}
	if sender == nil {
		return errors.New("email sender not configured")
	}

	composer := message.NewEmailMessage(level.Notice, email)
	sender.Send(composer)

	return nil
}

func (j *alertableInstanceTypeNotifyJob) sendSlackNotification(u *user.DBUser, hosts []host.Host) error {
	// Determine the Slack target (prefer member ID over username)
	var slackTarget string
	if u.Settings.SlackMemberId != "" {
		slackTarget = u.Settings.SlackMemberId
	} else if u.Settings.SlackUsername != "" {
		slackTarget = fmt.Sprintf("@%s", u.Settings.SlackUsername)
	} else {
		return nil // Skip if no Slack information available
	}

	slackMsg := j.buildSlackMessage(hosts)
	sender, err := j.env.GetSender(evergreen.SenderSlack)
	if err != nil {
		return errors.Wrap(err, "getting Slack sender")
	}
	if sender == nil {
		return errors.New("Slack sender not configured")
	}

	composer := message.NewSlackMessage(level.Notice, slackTarget, slackMsg, nil)
	sender.Send(composer)

	return nil
}

func (j *alertableInstanceTypeNotifyJob) buildEmailBody(hosts []host.Host) string {
	body := "You have spawn hosts using large instance types ("

	for idx, h := range hosts {
		if idx > 1 {
			body += ", "
		}
		body += fmt.Sprintf("Host: %s (Instance Type: %s, Status: %s)", h.Id, h.InstanceType, h.Status)
	}

	body += ") . Please remember to switch to smaller instance types when you're finished with development."

	return body
}

func (j *alertableInstanceTypeNotifyJob) buildSlackMessage(hosts []host.Host) string {
	msg := "You have spawn hosts using large instance types:\n\n"

	for _, h := range hosts {
		msg += fmt.Sprintf("• *%s* (Instance Type: `%s`, Status: `%s`)\n", h.Id, h.InstanceType, h.Status)
	}

	msg += "\nPlease remember to switch to smaller instance types when you're finished with development.\n\n"
	msg += "Thank you, \nThe Evergreen Team"

	return msg
}

func (j *alertableInstanceTypeNotifyJob) logNotificationSummary(userHosts map[string][]host.Host) {
	totalHosts := 0
	hostList := []string{}
	userList := []string{}

	for userID, hosts := range userHosts {
		totalHosts += len(hosts)
		userList = append(userList, userID)
		for _, userHost := range hosts {
			hostList = append(hostList, userHost.Id)
		}
	}

	grip.Info(message.Fields{
		"message":                           "alerted users about spawn hosts using alertable instance types",
		"job_id":                            j.ID(),
		"job_type":                          j.Type().Name,
		"total_users_affected":              len(userHosts),
		"total_hosts_using_alertable_types": totalHosts,
		"hosts_using_alertable_types":       hostList,
		"affected_users":                    userList,
	})
}
