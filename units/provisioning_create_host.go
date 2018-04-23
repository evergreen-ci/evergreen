package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/hostinit"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
)

const createHostJobName = "provisioning-create-host"

type createHostJob struct {
	HostID   string `bson:"host_id" json:"host_id" yaml:"host_id"`
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`

	host *host.Host
	env  evergreen.Environment
}

func makeCreateHostJob() *createHostJob {
	j := &createHostJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    createHostJobName,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

func NewHostCreateJob(env evergreen.Environment, h host.Host, id string) amboy.Job {
	j := makeCreateHostJob()
	j.host = &h
	j.HostID = h.Id
	j.env = env
	j.SetPriority(1)
	j.SetID(fmt.Sprintf("%s.%s.%s", createHostJobName, j.HostID, id))
	return j
}

func (j *createHostJob) Run(ctx context.Context) {
	var err error
	defer j.MarkComplete()

	if j.host == nil {
		j.host, err = host.FindOneId(j.HostID)
		if err != nil {
			j.AddError(err)
			return
		}
		if j.host == nil {
			j.AddError(fmt.Errorf("could not find host %s for job %s", j.HostID, j.TaskID))
			return
		}
	}

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	settings := j.env.Settings()

	err = hostinit.CreateHost(ctx, j.host, settings)
	j.AddError(err)
}
