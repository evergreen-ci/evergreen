package units

import (
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
)

const decoHostNotifyJobName = "deco-host-notify"

func init() {
	registry.AddJobType(decoHostNotifyJobName, func() amboy.Job { return makeDecoHostsNotifyJob() })
}

type decoHostNotifyJob struct {
	Host     *host.Host `bson:"host" json:"host" yaml:"host"`
	Message  string     `bson:"message" json:"message" yaml:"message"`
	OpError  string     `bson:"error" json:"error" yaml:"error"`
	HasError bool       `bson:"errorp" json:"errorp" yaml:"errorp"`
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment
}

func makeDecoHostsNotifyJob() *decoHostNotifyJob {
	j := &decoHostNotifyJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    decoHostNotifyJobName,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

func NewDecoHostNotifyJob(env evergreen.Environment, h *host.Host, err error, message string) amboy.Job {
	j := makeDecoHostsNotifyJob()
	j.env = env
	j.Host = h
	j.Message = message
	if err != nil {
		j.OpError = err.Error()
		j.HasError = true
	}

	j.SetID(fmt.Sprintf("%s.%s.%d", decoHostNotifyJobName, h.Id, job.GetNumber()))

	return j
}

func (j *decoHostNotifyJob) Run() {
	defer j.MarkComplete()

	hostUptime := time.Since(j.Host.CreationTime)

	if j.Host.Provider != evergreen.HostTypeStatic {
		// if this isn't a static host
		m := message.Fields{
			"operation":   decoHostNotifyJobName,
			"message":     j.Message,
			"distro":      j.Host.Distro.Id,
			"provider":    j.Host.Provider,
			"uptime":      hostUptime,
			"uptime_span": hostUptime.String(),
			"host":        j.Host.Id,
		}

		if j.HasError {
			m["error"] = j.OpError
		}

		grip.Error(m)
		return
	}

	// otherwise, it was a static host and we should create jira tickets for this.
	client := util.GetHttpClient()
	defer util.PutHttpClient(client)

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	conf := j.env.Settings()
	opts := &send.JiraOptions{
		Name:       "evergreen",
		BaseURL:    conf.Jira.GetHostURL(),
		Username:   conf.Jira.Username,
		Password:   conf.Jira.Password,
		HTTPClient: client,
	}
	sender, err := send.MakeJiraLogger(opts)
	if err != nil {
		j.AddError(err)

		m := message.Fields{
			"operation":   decoHostNotifyJobName,
			"message":     j.Message,
			"state":       "host disabled, jira ticket creation failed",
			"distro":      j.Host.Distro.Id,
			"provider":    j.Host.Provider,
			"uptime":      hostUptime,
			"uptime_span": hostUptime.String(),
			"host":        j.Host.Id,
			"jira_error":  err.Error(),
		}

		if j.HasError {
			m["error"] = j.OpError
		}

		grip.Alert(m)

		return
	}
	defer func() { grip.Warning(sender.Close()) }()

	grip.Warning(sender.SetErrorHandler(send.ErrorHandlerFromSender(grip.GetSender())))
	descParts := []string{
		fmt.Sprintf("Distro: [%s|%s/distros##%s]", j.Host.Distro.Id, conf.Ui.Url, j.Host.Distro.Id),
		fmt.Sprintf("Host: [%s|%s/host/%s]", j.Host.Id, conf.Ui.Url, j.Host.Id),
		fmt.Sprintln("Provider:", j.Host.Provider),
		fmt.Sprintf("Target: %s@%s", j.Host.Distro.User, j.Host.Host),
	}

	if j.HasError {
		descParts = append(descParts, fmt.Sprintln("Error:", j.OpError))
	}

	issue := message.JiraIssue{
		Project:     conf.Jira.DefaultProject,
		Summary:     fmt.Sprintf("investigate automatically decommissioned host '%s'", j.Host.Id),
		Type:        "Incident",
		Description: strings.Join(descParts, "\n"),
		Components:  []string{"Evergreen"},
	}

	msg := message.MakeJiraMessage(issue)
	grip.CatchWarning(msg.SetPriority(level.Notice))

	sender.Send(msg)
}
