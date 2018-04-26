package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
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
		return makeSpawnhostTerminationAlertJob()
	})
}

type spawnhostTerminationAlertJob struct {
	job.Base `bson:"base" json:"base" yaml:"base"`

	User string `bson:"user"`
	Host string `bson:"host"`

	sender send.Sender
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

	job.SetID(fmt.Sprintf("%s.%s", spawnhostTerminationAlertName, id))

	return job
}

func (j *spawnhostTerminationAlertJob) Run(_ context.Context) {
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

	dbUser, err := user.FindOne(user.ById(j.User))
	if err != nil {
		j.AddError(errors.Wrap(err, "unable to find user"))
		return
	}
	address := dbUser.Email()

	if j.sender == nil {
		env := evergreen.GetEnvironment()
		j.sender, err = env.GetSender(evergreen.SenderEmail)
		if err != nil {
			j.AddError(err)
			return
		}
	}

	email := message.Email{
		Recipients:        []string{address},
		Subject:           emailSubject,
		Body:              fmt.Sprintf(emailBody, j.Host),
		PlainTextContents: true,
	}

	j.sender.Send(message.MakeEmailMessage(email))
}
