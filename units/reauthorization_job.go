package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	reauthorizationJobName  = "reauthorize-user"
	defaultBackgroundReauth = time.Hour
	maxReauthAttempts       = 10
)

type reauthorizationJob struct {
	UserID   string `bson:"user_id" json:"user_id" yaml:"user_id"`
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`

	user *user.DBUser
	env  evergreen.Environment
}

func init() {
	registry.AddJobType(reauthorizationJobName, func() amboy.Job {
		return makeReauthorizationJob()
	})
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
	j.UserID = u.Username()
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
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		j.AddError(err)
		return
	}
	if flags.BackgroundReauthDisabled {
		return
	}

	// Do not reauth them if they are already logged out.
	if j.user.LoginCache.Token == "" {
		return
	}

	// Do not try background reauth if they've exceeded their attempt limit
	// until they refresh their login cache.
	if j.user.LoginCache.ReauthAttempts >= maxReauthAttempts {
		return
	}

	reauthAfter := time.Duration(j.env.Settings().AuthConfig.BackgroundReauthMinutes) * time.Minute
	if reauthAfter == 0 {
		reauthAfter = defaultBackgroundReauth
	}

	if time.Since(j.user.LoginCache.TTL) <= reauthAfter {
		return
	}

	um := j.env.UserManager()
	if um == nil {
		grip.Notice(message.WrapError(err, message.Fields{
			"message": "cannot get user manager",
			"job":     j.ID(),
		}))
		return
	}
	if !j.env.UserManagerInfo().CanReauthorize {
		return
	}

	if err = um.ReauthorizeUser(j.user); err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "could not reauthorize user",
			"user":    j.user.Username(),
			"job":     j.ID(),
		}))
		j.AddError(err)
		if err := j.user.IncReauthAttempts(); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "failed to modify user reauth attempts",
				"user":    j.user.Username(),
				"job":     j.ID(),
			}))
		}
		return
	}
}
