package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	reauthorizationJobName = "reauthorize-user"
)

type reauthorizationJob struct {
	UserID   string `bson:"user_id" json:"user_id" yaml:"user_id"`
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`

	user *user.DBUser
	env  evergreen.Environment
}

func makeReauthorizationJob() *reauthorizationJob {
	j := &reauthorizationJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    reauthorizationJobName,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

// NewReauthorizationJob returns a job that attempts to reauthorize the given
// user.
func NewReauthorizationJob(env evergreen.Environment, u *user.DBUser, id string) amboy.Job {
	j := makeReauthorizationJob()
	j.env = env
	j.user = u
	j.SetPriority(1)
	j.SetID(fmt.Sprintf("%s.%s.%s", reauthorizationJobName, u.Username(), id))
	return j
}

func (j *reauthorizationJob) Run(ctx context.Context) {
	if j.user == nil {
		user, err := user.FindOneById(j.UserID)
		if err != nil {
			j.AddError(err)
			return
		}
		if user == nil {
			j.AddError(errors.Errorf("could not find user '%s'", j.UserID))
			return
		}
		j.user = user
	}

	// TODO: service degraded flag

	// Do not reauth them if they are already logged out.
	if j.user.LoginCache.Token == "" {
		return
	}

	// TODO: replace with reauthorization timeout from settings
	if time.Since(j.user.LoginCache.TTL) <= time.Hour {
		return
	}

	um, err := auth.UserManager()
	if err != nil {
		grip.Notice(errors.Wrap(err, "cannot get user manager"))
		return
	}

	if _, err = um.GetUserByID(j.user.Username()); err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "could not reauthorize user",
			"user":    j.user.Username(),
		}))
		j.AddError(err)
		return
	}
}
