package units

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	reauthorizeUserJobName  = "reauthorize-user"
	defaultBackgroundReauth = time.Hour
	maxReauthAttempts       = 10
)

type reauthorizeUserJob struct {
	UserID   string `bson:"user_id" json:"user_id" yaml:"user_id"`
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`

	user *user.DBUser
	env  evergreen.Environment
}

func init() {
	registry.AddJobType(reauthorizeUserJobName, func() amboy.Job {
		return makeReauthorizeUserJob()
	})
}

func makeReauthorizeUserJob() *reauthorizeUserJob {
	j := &reauthorizeUserJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    reauthorizeUserJobName,
				Version: 0,
			},
		},
	}
	return j
}

// NewReauthorizeUserJob returns a job that attempts to reauthorize the given
// user.
func NewReauthorizeUserJob(env evergreen.Environment, u *user.DBUser, id string) amboy.Job {
	j := makeReauthorizeUserJob()
	j.UserID = u.Username()
	j.env = env
	j.user = u
	j.SetScopes([]string{fmt.Sprintf("%s.%s", reauthorizeUserJobName, u.Username())})
	j.SetEnqueueAllScopes(true)
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable:   utility.TruePtr(),
		MaxAttempts: utility.ToIntPtr(maxReauthAttempts),
		WaitUntil:   utility.ToTimeDurationPtr(time.Hour),
	})
	j.SetID(fmt.Sprintf("%s.%s.%s", reauthorizeUserJobName, u.Username(), id))
	return j
}

func (j *reauthorizeUserJob) Run(ctx context.Context) {
	if j.user == nil {
		user, err := user.FindOneByIdContext(ctx, j.UserID)
		if err != nil {
			j.AddRetryableError(errors.Wrapf(err, "finding user '%s'", j.UserID))
			return
		}
		if user == nil {
			j.AddError(errors.Errorf("user '%s' not found", j.UserID))
			return
		}
		j.user = user
	}
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	flags, err := evergreen.GetServiceFlags(ctx)
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

	reauthAfter := time.Duration(j.env.Settings().AuthConfig.BackgroundReauthMinutes) * time.Minute
	if reauthAfter == 0 {
		reauthAfter = defaultBackgroundReauth
	}

	if time.Since(j.user.LoginCache.TTL) <= reauthAfter {
		return
	}

	um := j.env.UserManager()
	if um == nil {
		j.AddRetryableError(errors.New("getting global user manager"))
		return
	}
	if !j.env.UserManagerInfo().CanReauthorize {
		j.AddRetryableError(errors.New("reauthorizing user when the user manager does not support reauthorization"))
		return
	}

	err = um.ReauthorizeUser(j.user)

	// This handles a special Okta case in which the user's refresh token from
	// Okta has expired, in which case they should be logged out, so that they
	// are forced to log in to get a new refresh token.
	if err != nil && strings.Contains(err.Error(), "invalid_grant") && strings.Contains(err.Error(), "The refresh token is invalid or expired.") {
		grip.Info(message.WrapError(err, message.Fields{
			"message": "user's refresh token is invalid, logging them out",
			"user":    j.UserID,
			"job":     j.ID(),
		}))
		if err = user.ClearLoginCache(j.user); err != nil {
			j.AddError(errors.Wrapf(err, "clearing login cache"))
		}
		return
	}

	j.AddRetryableError(errors.Wrap(err, "reauthorizing user"))

	if err != nil && j.IsLastAttempt() {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "user has no more background reauth attempts, logging them out",
			"user":    j.UserID,
			"job":     j.ID(),
		}))
		if err := user.ClearLoginCache(j.user); err != nil {
			j.AddError(errors.Wrapf(err, "clearing login cache"))
		}
	}
}
