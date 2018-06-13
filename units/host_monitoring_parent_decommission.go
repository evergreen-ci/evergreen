package units

import (
	"context"
	"fmt"
	"math"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
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

// computeParentsToDecommission computes the number of hosts with containers to deco
func computeHostsToDecommission(parentIds []string, distroId string) (int, error) {

	numParents := len(parentIds)

	// Count number of containers on the parents
	containersQuery := db.Query(bson.M{
		host.StatusKey:   evergreen.HostRunning,
		host.ParentIDKey: bson.M{"$in": parentIds},
	})
	numContainers, err := host.Count(containersQuery)
	if err != nil {
		return 0, errors.Wrap(err, "Error counting containers on specified parents")
	}

	// Get maximum number of containers allowed on a parent with given distro
	d, err := distro.FindOne(distro.ById(distroId))
	if err != nil {
		return 0, errors.Wrap(err, "Error getting max number of containers for distro")
	}
	maxContainersPerParent := d.MaxContainers

	// Compute number of hosts to decommission based on excess capacity
	numParentsToDeco := numParents - int(math.Ceil(float64(numContainers)/float64(maxContainersPerParent)))

	// Sanity check
	if numParentsToDeco < 0 {
		numParentsToDeco = 0
	}

	return numParentsToDeco, nil
}

func (j *parentDecommissionJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	// Find hosts that will finish all container tasks soonest
	parents, err := host.FindAllRunningParentsByDistro(j.distroId)
	if err != nil {
		j.AddError(err)
		return
	}

	// Build list of parent Ids
	var parentIds []string
	for _, parent := range parents {
		parentIds = append(parentIds, parent.Id)
	}

	numParentsToDeco, err := computeHostsToDecommission(parentIds, j.distroId)
	if err != nil {
		j.AddError(err)
		return
	}

	// Get desired number parents with nearest LastContainerFinishTime
	parentsToDeco := parents[:numParentsToDeco]

	for _, h := range parentsToDeco {

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
