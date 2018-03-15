package units

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/model/triggers"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/grip/sometimes"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

const (
	eventMetaJobName         = "event-metajob"
	eventNotificationJobName = "event-send"
	evergreenWebhookTimeout  = 5 * time.Second

	EventMetaJobPeriod = 5 * time.Minute
)

func init() {
	registry.AddJobType(eventMetaJobName, func() amboy.Job { return makeEventMetaJob() })
	registry.AddJobType(eventNotificationJobName, func() amboy.Job { return makeEventNotificationJob() })
}

type eventMetaJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment
}

func makeEventMetaJob() *eventMetaJob {
	j := &eventMetaJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    eventMetaJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())

	return j
}

func NewEventMetaJob() amboy.Job {
	j := makeEventMetaJob()
	j.env = evergreen.GetEnvironment()

	// TODO: safe?
	j.SetID(fmt.Sprintf("%s:%s:%d", eventMetaJobName, time.Now().String(), job.GetNumber()))

	return j
}

func (j *eventMetaJob) Run() {
	defer j.MarkComplete()

	if j.env == nil || j.env.RemoteQueue() == nil || !j.env.RemoteQueue().Started() {
		j.AddError(errors.New("evergreen environment not setup correctly"))
		return
	}

	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		j.AddError(errors.Wrap(err, "error retrieving admin settings"))
		return
	}
	if flags.EventProcessingDisabled {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"job":     eventMetaJobName,
			"message": "events processing is disabled, all events will be marked processed",
		})
	}

	events, err := event.Find(event.AllLogCollection, db.Query(event.UnprocessedEvents()))
	logger := event.NewDBEventLogger(event.AllLogCollection)

	if err != nil {
		j.AddError(err)
		return
	}

	if len(events) == 0 {
		grip.Info(message.Fields{
			"job_id":  j.ID(),
			"job":     eventMetaJobName,
			"time":    time.Now().String(),
			"message": "no events need to be processed",
			"source":  "events-processing",
		})
		return
	}

	// TODO: if this is a perf problem, it could be multithreaded. For now,
	// we just log time
	startTime := time.Now()
	for i := range events {
		triggerStartTime := time.Now()

		var notifications []notification.Notification
		notifications, err = triggers.ProcessTriggers(&events[i])

		triggerEndTime := time.Now()
		triggerDuration := triggerEndTime.Sub(triggerStartTime)
		j.AddError(err)
		grip.Info(message.Fields{
			"job_id":            j.ID(),
			"job":               eventMetaJobName,
			"source":            "events-processing",
			"message":           "event-stats",
			"event_id":          events[i].ID.Hex(),
			"event_type":        events[i].Type(),
			"start_time":        triggerStartTime.String(),
			"end_time":          triggerEndTime.String(),
			"duration":          triggerDuration.String(),
			"num_notifications": len(notifications),
		})
		grip.Error(message.WrapError(err, message.Fields{
			"job_id":     j.ID(),
			"job":        eventMetaJobName,
			"source":     "events-processing",
			"message":    "errors processing triggers for event",
			"event_id":   events[i].ID.Hex(),
			"event_type": events[i].Type(),
		}))

		if err = notification.InsertMany(notifications...); err != nil {
			j.AddError(err)
			continue
		}

		if flags.EventProcessingDisabled {
			for i := range notifications {
				j.AddError(j.env.RemoteQueue().Put(newEventNotificationJob(notifications[i].ID)))
			}
		}

		j.AddError(logger.MarkProcessed(&events[i]))
	}
	endTime := time.Now()
	totalDuration := endTime.Sub(startTime)

	grip.Info(message.Fields{
		"job_id":        j.ID(),
		"job":           eventMetaJobName,
		"source":        "events-processing",
		"message":       "stats",
		"start_time":    startTime.String(),
		"end_time":      endTime.String(),
		"duration":      totalDuration.String(),
		"n":             len(events),
		"degraded_mode": flags.EventProcessingDisabled,
	})
}

type eventNotificationJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	settings *evergreen.Settings

	NotificationID bson.ObjectId `bson:"notification_id" json:"notification_id" yaml:"notification_id"`
}

func makeEventNotificationJob() *eventNotificationJob {
	j := &eventNotificationJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    eventNotificationJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())

	return j
}

func newEventNotificationJob(id bson.ObjectId) amboy.Job {
	j := makeEventNotificationJob()

	j.SetID(fmt.Sprintf("%s:%s", eventNotificationJobName, id.Hex()))
	return j
}

const (
	githubPullRequestSubscriberType = "github_pull_request"
	slackSubscriberType             = "slack"
	jiraIssueSubscriberType         = "jira-issue"
	jiraCommentSubscriberType       = "jira-comment"
	evergreenWebhookSubscriberType  = "evergreen-webhook"
	emailSubscriberType             = "email"
)

func (j *eventNotificationJob) Run() {
	defer j.MarkComplete()

	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		j.AddError(errors.Wrap(err, "error retrieving admin settings"))

	} else if flags == nil {
		j.AddError(errors.Wrap(err, "fetched no service flags configuration"))
	}
	if j.HasErrors() {
		return
	}

	if !j.NotificationID.Valid() {
		j.AddError(errors.New("notification ID is not valid"))
		return
	}

	j.settings, err = evergreen.GetConfig()
	j.AddError(err)
	if err == nil && j.settings == nil {
		j.AddError(errors.New("settings object is nil"))
	}
	if j.HasErrors() {
		return
	}

	n, err := notification.Find(j.NotificationID)
	j.AddError(err)
	if err == nil && n == nil {
		j.AddError(errors.Errorf("can't find notification with ID: '%s", j.NotificationID.Hex()))
	}
	if j.HasErrors() {
		return
	}

	var sendError error
	switch n.Type {
	// TODO I'm tired
	//case githubPullRequestSubscriberType:
	//	if err = checkFlag(flags.GithubStatusAPIDisabled); err != nil {
	//		j.AddError(err)
	//		return
	//	}

	case slackSubscriberType:
		if err = checkFlag(flags.SlackNotificationsDisabled); err != nil {
			j.AddError(err)
			return
		}
		sendError = j.slackMessage(n)

	case jiraIssueSubscriberType:
		if err = checkFlag(flags.JIRANotificationsDisabled); err != nil {
			j.AddError(err)
			return
		}
		sendError = j.jiraIssue(n)

	case jiraCommentSubscriberType:
		if err = checkFlag(flags.JIRANotificationsDisabled); err != nil {
			j.AddError(err)
			return
		}
		sendError = j.jiraComment(n)

	case evergreenWebhookSubscriberType:
		if err = checkFlag(flags.WebhookNotificationsDisabled); err != nil {
			j.AddError(err)
			return
		}
		sendError = j.evergreenWebhook(n)

	case emailSubscriberType:
		if err = checkFlag(flags.EmailNotificationsDisabled); err != nil {
			j.AddError(err)
			return
		}
		sendError = j.email(n)

	default:
		j.AddError(errors.Errorf("unknown notification type: %s", n.Type))
	}

	j.AddError(n.MarkSent())
	if sendError != nil {
		j.AddError(sendError)
		j.AddError(n.MarkError(sendError))
	}
}

func jiraOptions(c evergreen.JiraConfig) (*send.JiraOptions, error) {
	url, err := url.Parse(c.Host)
	if err != nil {
		return nil, errors.Wrap(err, "invalid JIRA host")
	}
	url.Scheme = "https"

	jiraOpts := send.JiraOptions{
		Name:     "evergreen",
		BaseURL:  url.String(),
		Username: c.Username,
		Password: c.Password,
	}

	return &jiraOpts, nil
}

func (j *eventNotificationJob) jiraComment(n *notification.Notification) error {
	jiraOpts, err := jiraOptions(j.settings.Jira)
	if err != nil {
		return errors.Wrap(err, "error building jira settings")
	}

	jiraIssue, ok := n.Target.Target.(string)
	if !ok {
		return fmt.Errorf("jira-comment subscriber was invalid (expected string)")
	}

	sender, err := send.MakeJiraCommentLogger(jiraIssue, jiraOpts)
	if err != nil {
		return errors.Wrap(err, "jira-comment sender error")
	}

	c, err := n.Composer()
	if err != nil {
		return errors.Wrap(err, "jira-comment error building message")
	}

	j.send(sender, c, n)

	return nil
}

func (j *eventNotificationJob) jiraIssue(n *notification.Notification) error {
	jiraOpts, err := jiraOptions(j.settings.Jira)
	if err != nil {
		return errors.Wrap(err, "error building jira settings")
	}

	_, ok := n.Target.Target.(string)
	if !ok {
		return fmt.Errorf("jira-issue subscriber was invalid (expected string)")
	}

	sender, err := send.MakeJiraLogger(jiraOpts)
	if err != nil {
		return errors.Wrap(err, "jira-comment sender error")
	}

	c, err := n.Composer()
	if err != nil {
		return errors.Wrap(err, "jira-comment error building message")
	}

	j.send(sender, c, n)

	return nil
}

// calculatHMACHash calculates a sha256 HMAC has of the body with the given
// secret. The body must NOT be modified after calculating this hash
func calculateHMACHash(secret []byte, body []byte) (string, error) {
	// from genMAC in google/go-github/github/messages.go
	mac := hmac.New(sha256.New, secret)
	n, err := mac.Write(body)
	if n != len(body) {
		return "", errors.Errorf("Body length expected to be %d, but was %d", len(body), n)
	}
	if err != nil {
		return "", err
	}

	return "sha256=" + hex.EncodeToString(mac.Sum(nil)), nil
}

func (j *eventNotificationJob) evergreenWebhook(n *notification.Notification) error {
	raw := n.Payload.(*notification.EvergreenWebhookPayload)

	hookSubscriber, ok := n.Target.Target.(event.WebhookSubscriber)
	if !ok {
		return fmt.Errorf("evergreen-webhook invalid subscriber")
	}

	u, err := url.Parse(hookSubscriber.URL)
	if err != nil {
		return errors.Wrap(err, "evergreen-webhook bad URL")
	}
	u.Scheme = "https"

	reader := bytes.NewReader(raw.Payload)
	req, err := http.NewRequest(http.MethodPost, u.String(), reader)
	if err != nil {
		return errors.Wrap(err, "failed to create http request")
	}

	hash, err := calculateHMACHash(hookSubscriber.Secret, raw.Payload)
	if err != nil {
		return errors.Wrap(err, "failed to calculate hash")
	}

	for k, v := range raw.Headers {
		req.Header.Add(k, v)
	}

	req.Header.Del("X-Evergreen-Signature")
	req.Header.Add("X-Evergreen-Signature", hash)

	ctx, cancel := context.WithTimeout(req.Context(), evergreenWebhookTimeout)
	defer cancel()

	req = req.WithContext(ctx)

	client := util.GetHttpClient()
	defer util.PutHttpClient(client)

	resp, err := client.Do(req)
	defer resp.Body.Close()
	if err != nil {
		return errors.Wrap(err, "evergreen-webhook failed to sent webhook data")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.Errorf("evergreen-webhook response status was %d", resp.StatusCode)
	}

	return nil
}

func (j *eventNotificationJob) slackMessage(n *notification.Notification) error {
	// TODO slack sender can only send to channels
	// TODO slack rate limiting
	target, ok := n.Target.Target.(string)
	if !ok {
		return fmt.Errorf("slack subscriber was invalid (expected string)")
	}

	opts := send.SlackOptions{
		Channel:       target,
		Fields:        true,
		AllFields:     true,
		BasicMetadata: false,
		Name:          "evergreen",
	}
	// TODO other attributes

	sender, err := send.NewSlackLogger(&opts, j.settings.Slack.Token, send.LevelInfo{level.Notice, level.Notice})
	if err != nil {
		return errors.Wrap(err, "slack sender error")
	}

	c, err := n.Composer()
	if err != nil {
		return errors.Wrap(err, "slack error building message")
	}

	j.send(sender, c, n)

	return nil
}

func (j *eventNotificationJob) email(n *notification.Notification) error {
	// TODO modify grip to allow for email headers to be specified
	smtpConf := j.settings.Notify.SMTP
	if smtpConf == nil {
		return fmt.Errorf("email smtp settings are empty")
	}
	recipient, ok := n.Target.Target.(string)
	if !ok {
		return fmt.Errorf("email recipient email is not a string")
	}
	payload, ok := n.Payload.(*notification.EmailPayload)
	if !ok {
		return fmt.Errorf("email payload is invalid")
	}
	opts := send.SMTPOptions{
		Name:              "evergreen",
		From:              smtpConf.From,
		Server:            smtpConf.Server,
		Port:              smtpConf.Port,
		UseSSL:            smtpConf.UseSSL,
		Username:          smtpConf.Username,
		Password:          smtpConf.Password,
		PlainTextContents: false,
		GetContents:       payload.GetContents,
	}
	if err := opts.AddRecipients(recipient); err != nil {
		return errors.Wrap(err, "email was invalid")
	}
	sender, err := send.MakeSMTPLogger(&opts)
	if err != nil {
		return errors.Wrap(err, "email settings are invalid")
	}

	c, err := n.Composer()
	if err != nil {
		return errors.Wrap(err, "email error building message")
	}

	j.send(sender, c, n)

	return nil
}

func (j *eventNotificationJob) send(s send.Sender, c message.Composer, n *notification.Notification) {
	s.SetErrorHandler(getSendErrorHandler(n))
	s.Send(c)
}

func getSendErrorHandler(n *notification.Notification) send.ErrorHandler {
	return func(err error, c message.Composer) {
		if err == nil || c == nil {
			return
		}

		err = n.MarkError(err)
		grip.Error(message.WrapError(err, message.Fields{
			"job":             eventMetaJobName,
			"notification_id": n.ID.Hex(),
			"source":          "events-processing",
			"message":         "failed to add error to notification",
			"composer":        c.String(),
		}))
	}
}

func checkFlag(flag bool) error {
	if flag {
		grip.InfoWhen(sometimes.Percent(evergreen.DegradedLoggingPercent), message.Fields{
			"job":     eventMetaJobName,
			"message": "sender is disabled, not sending notification",
		})
		return errors.New("sender is disabled, not sending notification")
	}

	return nil
}
