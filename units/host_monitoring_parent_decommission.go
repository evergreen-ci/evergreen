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
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`
	distroId string `bson:"distro_id" json:"distro_id" yaml:"distro_id"`
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

func NewParentDecommissionJob(id string, distroId string) amboy.Job {
	j := makeParentDecommissionJob()
	j.distroId = distroId
	j.SetID(fmt.Sprintf("%s.%s.%s", parentDecommissionJobName, j.distroId, id))
	return j
}

func (j *parentDecommissionJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	// Compute how many hosts with containers to decommission
	numHostsToDeco, err := host.CountParentsToDecommission(j.distroId)
	if err != nil {
		j.AddError(err)
		return
	}

	// Find hosts that will finish all container tasks soonest
	hosts, err := host.FindAllRunningParentsByDistro(j.distroId)
	if err != nil {
		j.AddError(err)
		return
	}
	hostsToDeco := hosts[:numHostsToDeco]

	for _, h := range hostsToDeco {

		// Decommission each container on parent host
		containersToDeco, err := h.GetContainers()
		if err != nil {
			j.AddError(err)
			return
		}
		for _, c := range containersToDeco {
			err := c.SetDecommissioned(evergreen.User, "")
			j.AddError(err)
		}

		// Decommission parent host
		err = h.SetDecommissioned(evergreen.User, "")
		j.AddError(err)
	}

}
