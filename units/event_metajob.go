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
	j.SetDependency(dependency.NewAlways()) //wat?

	return j
}

func NewEventMetaJob() amboy.Job {
	j := makeEventMetaJob()
	j.env = evergreen.GetEnvironment()

	// TODO: safe?
	j.SetID(fmt.Sprintf("%s:%d", eventMetaJobName, job.GetNumber()))

	return j
}

func (j *eventMetaJob) Run() {
	defer j.MarkComplete()

	if j.env == nil || j.env.RemoteQueue() == nil || !j.env.RemoteQueue().Started() {
		j.AddError(errors.New("evergreen environment not setup correctly"))
		return
	}
	// TODO degraded mode

	events, err := event.Find(event.AllLogCollection, event.UnprocessedEvents())
	logger := event.NewDBEventLogger(event.AllLogCollection)

	if err != nil {
		j.AddError(err)
		return
	}

	if len(events) == 0 {
		grip.Info(message.Fields{
			"job_id":  j.ID(),
			"job":     eventMetaJobName,
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

		j.AddError(logger.MarkProcessed(&events[i]))
	}
	endTime := time.Now()
	totalDuration := endTime.Sub(startTime)

	grip.Info(message.Fields{
		"job_id":     j.ID(),
		"job":        eventMetaJobName,
		"source":     "events-processing",
		"message":    "stats",
		"start_time": startTime.String(),
		"end_time":   endTime.String(),
		"duration":   totalDuration.String(),
		"n":          len(events),
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

func NewEventNotificationJob(id bson.ObjectId) amboy.Job {
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

	if !j.NotificationID.Valid() {
		j.AddError(errors.New("notification ID is not valid"))
		return
	}

	var err error
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
	//	case githubPullRequestSubscriberType:
	//
	case slackSubscriberType:
		sendError = j.slackMessage(n)

	case jiraIssueSubscriberType:
		sendError = j.jiraIssue(n)

	case jiraCommentSubscriberType:
		sendError = j.jiraComment(n)

	case evergreenWebhookSubscriberType:
		sendError = j.evergreenWebhook(n)

	case emailSubscriberType:
		sendError = j.email(n)

	default:
		j.AddError(errors.Errorf("unknown notification type: %s", n.Type))
	}

	j.AddError(sendError)
	j.AddError(n.MarkSent(sendError))
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
	//return errors.Wrap(sender.Send(c), "jira-comment error posting issue")
	sender.Send(c)
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

	sender.Send(c)
	// TODO errors
	return nil
	//return errors.Wrap(sender.Send(c), "jira-comment error posting issue")
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

// TODO move elsewhere
type evergreenWebhook struct {
	Headers map[string]string `bson:"headers"`
	Payload []byte            `bson:"payload"`
}

func (j *eventNotificationJob) evergreenWebhook(n *notification.Notification) error {
	raw := n.Payload.(evergreenWebhook)

	hookSubscriber, ok := n.Target.Target.(event.WebhookSubscriber)
	if !ok {
		return fmt.Errorf("evergreen-webhook invalid subscriber")
	}

	reader := bytes.NewReader(raw.Payload)
	hookSubscriber.URL.Scheme = "https"
	req, err := http.NewRequest(http.MethodPost, hookSubscriber.URL.String(), reader)
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

	sender.Send(c)
	// TODO errors
	return nil
}

func (j *eventNotificationJob) email(n *notification.Notification) error {
	smtpConf := j.settings.Notify.SMTP
	if smtpConf == nil {
		return fmt.Errorf("email smtp settings are empty")
	}
	opts := send.SMTPOptions{
		Name:     "evergreen",
		From:     smtpConf.From,
		Server:   smtpConf.Server,
		Port:     smtpConf.Port,
		UseSSL:   smtpConf.UseSSL,
		Username: smtpConf.Username,
		Password: smtpConf.Password,
		GetContents: func(opts *send.SMTPOptions, m message.Composer) (string, string) {
			return "", ""
		}, // TODO
	}
	sender, err := send.MakeSMTPLogger(&opts)
	if err != nil {
		return errors.Wrap(err, "email settings are invalid")
	}

	c, err := n.Composer()
	if err != nil {
		return errors.Wrap(err, "email error building message")
	}

	sender.Send(c)
	// TODO errors
	return nil
}
