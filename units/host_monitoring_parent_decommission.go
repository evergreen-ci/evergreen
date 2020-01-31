package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
)

const (
	parentDecommissionJobName = "parent-decommission-job"
)

func init() {
	registry.AddJobType(parentDecommissionJobName, func() amboy.Job {
		return makeParentDecommissionJob()
	})

}

type parentDecommissionJob struct {
	job.Base      `bson:"metadata" json:"metadata" yaml:"metadata"`
	DistroId      string `bson:"distro_id" json:"distro_id" yaml:"distro_id"`
	MaxContainers int    `bson:"max_containers" json:"max_containers" yaml:"max_containers"`
}

func makeParentDecommissionJob() *parentDecommissionJob {
	j := &parentDecommissionJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    parentDecommissionJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())

	return j
}

func NewParentDecommissionJob(id, d string, maxContainers int) amboy.Job {
	j := makeParentDecommissionJob()
	j.DistroId = d
	j.MaxContainers = maxContainers
	j.SetID(fmt.Sprintf("%s.%s.%s", parentDecommissionJobName, j.DistroId, id))
	return j
}

func (j *parentDecommissionJob) Run(ctx context.Context) {
	defer j.MarkComplete()
	parents, err := host.FindAllRunningParentsByDistro(j.DistroId)
	if err != nil {
		j.AddError(err)
		return
	}

	for _, h := range parents {
		// Decommission parent if its containers aren't running anymore
		idle, err := h.IsIdleParent()
		if err != nil {
			j.AddError(err)
			continue
		}
		if idle {
			j.AddError(h.SetDecommissioned(evergreen.User, "host only contains decommissioned containers and there is excess capacity"))
		}
	}
}
