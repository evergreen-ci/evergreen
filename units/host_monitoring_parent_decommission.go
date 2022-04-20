package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/amboy"
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
	parents, err := host.FindAllRunningParentsByDistroID(j.DistroId)
	if err != nil {
		j.AddError(err)
		return
	}
	parentDistro, err := distro.FindOneId(j.DistroId)
	if err != nil {
		j.AddError(err)
		return
	}
	minHosts := 0
	if parentDistro == nil {
		j.AddError(errors.Errorf("distro '%s' not found", j.DistroId))
	} else if parentDistro != nil {
		minHosts = parentDistro.HostAllocatorSettings.MinimumHosts
	}
	parentCount := len(parents)

	for _, h := range parents {
		if parentCount <= minHosts {
			return
		}
		// Decommission parent if its containers aren't running anymore
		idle, err := h.IsIdleParent()
		if err != nil {
			j.AddError(err)
			continue
		}
		if idle {
			err = h.SetDecommissioned(evergreen.User, false, "host only contains decommissioned containers and there is excess capacity")
			if err != nil {
				j.AddError(err)
				continue
			}
			parentCount--
		}
	}
}
