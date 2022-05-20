package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
)

const staticUpdateSSHKeysJobName = "update-ssh-keys-host"

type staticUpdateSSHKeysJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	HostID   string `bson:"host_id" json:"host_id" yaml:"host_id"`

	host *host.Host
}

func init() {
	registry.AddJobType(staticUpdateSSHKeysJobName, func() amboy.Job {
		return makeStaticUpdateSSHKeysJob()
	})
}

func makeStaticUpdateSSHKeysJob() *staticUpdateSSHKeysJob {
	j := &staticUpdateSSHKeysJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    staticUpdateSSHKeysJobName,
				Version: 0,
			},
		},
	}
	return j
}

// NewStaticUpdateSSHKeysJob updates the SSH keys for a static host.
func NewStaticUpdateSSHKeysJob(h host.Host, id string) amboy.Job {
	j := makeStaticUpdateSSHKeysJob()
	j.host = &h

	j.HostID = h.Id
	j.SetID(fmt.Sprintf("%s.%s.%s", staticUpdateSSHKeysJobName, h.Id, id))
	return j
}

func (j *staticUpdateSSHKeysJob) Run(ctx context.Context) {
	if j.host == nil {
		h, err := host.FindOneId(j.HostID)
		if err != nil {
			j.AddError(err)
			return
		}
		j.host = h
	}
}
