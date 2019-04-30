package units

import (
	"context"
	"fmt"
	"math"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/pkg/errors"
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

// findParentsToDecommission finds hosts with containers to deco
func (j *parentDecommissionJob) findParentsToDecommission() ([]host.Host, error) {

	// Find hosts that will finish all container tasks soonest
	parents, err := host.FindAllRunningParentsByDistro(j.DistroId)
	if err != nil {
		return nil, errors.Wrap(err, "Error retrieving running parents by distro")
	}
	numParents := len(parents)

	numContainers, err := host.HostGroup(parents).CountContainersOnParents()
	if err != nil {
		return nil, errors.Wrap(err, "Error counting containers on specified parents")
	}

	// Compute number of hosts to decommission based on excess capacity
	numParentsToDeco := numParents - int(math.Ceil(float64(numContainers)/float64(j.MaxContainers)))

	// Sanity check
	if numParentsToDeco < 0 || numParentsToDeco > numParents {
		return nil, errors.New("Invalid number of parents to decommission")
	}

	// Get desired number parents with nearest LastContainerFinishTime
	parentsToDeco := parents[:numParentsToDeco]

	return parentsToDeco, nil
}

func (j *parentDecommissionJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	_, err := distro.FindOne(distro.ById(j.DistroId))
	if err != nil {
		j.AddError(err)
	}

	parentsToDeco, err := j.findParentsToDecommission()
	if err != nil {
		j.AddError(err)
		return
	}

	for _, h := range parentsToDeco {

		// Decommission each container on parent host
		containersToDeco, err := h.GetContainers()
		if err != nil {
			j.AddError(err)
			continue
		}

		for _, c := range containersToDeco {
			if c.Status == evergreen.HostRunning && !c.SpawnOptions.SpawnedByTask {
				j.AddError(c.SetDecommissioned(evergreen.User, ""))
				continue
			}
		}

		// Decommission parent only if all containers have terminated
		idle, err := h.IsIdleParent()
		if err != nil {
			j.AddError(err)
			continue
		}
		if idle {
			j.AddError(h.SetDecommissioned(evergreen.User, ""))
		}
	}
}
