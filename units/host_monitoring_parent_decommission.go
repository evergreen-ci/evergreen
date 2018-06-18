package units

import (
	"context"
	"fmt"

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
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`
	distroId string
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

// findParentsToDecommission finds hosts with containers to deco
func findParentsToDecommission(distroId string) ([]host.Host, error) {

	// Find hosts that will finish all container tasks soonest
	parents, err := host.FindAllRunningParentsByDistro(distroId)
	if err != nil {
		return nil, errors.Wrap(err, "Error retrieving running parents by distro")
	}

	// Build list of parent Ids
	var parentIds []string
	for _, parent := range parents {
		parentIds = append(parentIds, parent.Id)
	}

	numParents := len(parentIds)
	numContainers, err := host.CountContainersOnParents(parentIds)
	if err != nil {
		return nil, errors.Wrap(err, "Error counting containers on specified parents")
	}

	// Get maximum number of containers allowed on a parent with given distro
	d, err := distro.FindOne(distro.ById(distroId))
	if err != nil {
		return nil, errors.Wrap(err, "Error getting max number of containers for distro")
	}
	maxContainersPerParent := d.MaxContainers

	// Compute number of hosts to decommission based on excess capacity
	numParentsToDeco, err := host.ComputeParentsToDecommission(numParents,
		numContainers, maxContainersPerParent)

	// Sanity check
	if err != nil {
		return nil, errors.Wrap(err, "Error computing number of parents to decommission")
	} else if numParentsToDeco < 0 || numParentsToDeco > numParents {
		return nil, errors.New("Invalid number of parents to decommission")
	}

	// Get desired number parents with nearest LastContainerFinishTime
	parentsToDeco := parents[:numParentsToDeco]

	return parentsToDeco, nil
}

func (j *parentDecommissionJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	parentsToDeco, err := findParentsToDecommission(j.distroId)
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
			if c.Status == evergreen.HostRunning {
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
