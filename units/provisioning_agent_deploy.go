package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/hostinit"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/grip"
)

// this is the legacy taskrunner

const agentDeployJobName = "agent-deploy"

type agentDeployJob struct {
	HostID   string `bson:"host_id" json:"host_id" yaml:"host_id"`
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`

	host *host.Host
	env  evergreen.Environment
}

func makeAgentDeployJob() *agentDeployJob {
	j := &agentDeployJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    agentDeployJobName,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

func NewAgentDeployJob(env evergreen.Environment, h host.Host, id string) amboy.Job {
	j := makeAgentDeployJob()
	j.host = &h
	j.HostID = h.Id
	j.env = env
	j.SetPriority(1)
	j.SetID(fmt.Sprintf("%s.%s.%s", agentDeployJobName, j.HostID, id))

	return j
}

func (j *agentDeployJob) Run(ctx context.Context) {
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

	err = hostinit.StartAgentOnHost(ctx, settings, *j.host)
	j.AddError(err)
	if err != nil {
		stat, err := event.GetRecentAgentDeployStatuses(j.HostID, 15)
		j.AddError(err)
		if err != nil {
			return
		}

		if stat.LastAttemptFailed() && stat.AllAttemptsFailed() && stat.Count == 15 {
			grip.Critical(stat)
		}

	}
}
