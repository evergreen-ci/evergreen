package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/notify"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/pkg/errors"
)

const (
	spawnhostTerminationAlertName = "spawnhost-termination-alert"

	emailSubject = "Spawn Host Terminated"
	emailBody    = "Your spawn host '%s' has been terminated by Evergreen because it failed to provision. " +
		"Feel free to spawn another."
)

func init() {
	registry.AddJobType(spawnhostTerminationAlertName, func() amboy.Job {
		return makeHostMonitorExternalState()
	})
}

type spawnhostTerminationAlertJob struct {
	job.Base `bson:"base" json:"base" yaml:"base"`

	User string `bson:"user"`
	Host string `bson:"host"`

	// for testing
	UseMock   bool              `bson:"use_mock,omitempty"`
	sentMails []notify.MockMail `bson:"-"`
}

func makeSpawnhostTerminationAlertJob() *spawnhostTerminationAlertJob {
	j := &spawnhostTerminationAlertJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    spawnhostTerminationAlertName,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

func NewSpawnhostTerminationAlertJob(user, host string, useMock bool, id string) amboy.Job {
	job := makeSpawnhostTerminationAlertJob()
	job.User = user
	job.Host = host
	job.UseMock = useMock

	job.SetID(fmt.Sprintf("%s.%s", spawnhostTerminationAlertName, id))

	return job
}

func (j *spawnhostTerminationAlertJob) Run(ctx context.Context) {
	var cancel context.CancelFunc

	ctx, cancel = context.WithCancel(ctx)
	defer cancel()
	defer j.MarkComplete()

	config, err := evergreen.GetConfig()
	if err != nil {
		j.AddError(errors.Wrap(err, "error retrieving admin settings"))
		return
	}
	if config.ServiceFlags.AlertsDisabled {
		j.AddError(errors.New("alerts are disabled"))
		return
	}

	var mailer notify.Mailer
	if j.UseMock {
		mailer = &notify.MockMailer{}
	} else {
		smtp := config.Alerts.SMTP
		if smtp == nil {
			j.AddError(errors.New("SMTP server not configured"))
			return
		}

		mailer = notify.SmtpMailer{
			From:     smtp.From,
			Server:   smtp.Server,
			Port:     smtp.Port,
			UseSSL:   smtp.UseSSL,
			Username: smtp.Username,
			Password: smtp.Password,
		}
	}

	err = notify.TrySendNotificationToUser(j.User, emailSubject, fmt.Sprintf(emailBody, j.Host), mailer)
	if err != nil {
		j.AddError(err)
	}

	if j.UseMock {
		mock := mailer.(*notify.MockMailer)
		j.sentMails = mock.Sent
	}
}
