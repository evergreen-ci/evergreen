package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/pkg/errors"
)

const (
	reauthorizationJobName = "reauthorize-user"
)

type reauthorizationJob struct {
	UserID   string `bson:"user_id" json:"user_id" yaml:"user_id"`
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`

	user gimlet.User
	env  evergreen.Environment
}

func makeReauthorizationJob() *reauthorizationJob {
	j := &reauthorizationJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    reauthorizationJob,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

func NewReauthorizationJob(env evergreen.Environment, u gimlet.User, id string) amboy.Job {
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

	// Get user manager from API server (idk how to do this).

	// Check TTL is still expired
	// Call GetUserByToken and check if reauth succeeds.

	// If they can't be reauthenticated, clear their login cache so they're
	// forced to log in.
}
