package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/pkg/errors"
)

const (
	hostIPAssociationJobName     = "host-ip-association"
	hostIPAssociationMaxAttempts = 5
)

func init() {
	registry.AddJobType(hostIPAssociationJobName, func() amboy.Job {
		return makeHostIPAssociationJob()
	})
}

type hostIPAssociationJob struct {
	job.Base

	HostID string `bson:"host_id" json:"host_id" yaml:"host_id"`

	env  evergreen.Environment
	host *host.Host
}

func makeHostIPAssociationJob() *hostIPAssociationJob {
	return &hostIPAssociationJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    hostIPAssociationJobName,
				Version: 0,
			},
		},
	}
}

// NewHostIPAssociationJob creates a job to associate an allocated IP address
// with the host's network interface.
func NewHostIPAssociationJob(env evergreen.Environment, h *host.Host, ts string) amboy.Job {
	j := makeHostIPAssociationJob()
	j.env = env
	j.host = h
	j.HostID = h.Id
	j.SetID(fmt.Sprintf("%s.%s.%s", hostIPAssociationJobName, h.Id, ts))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", hostIPAssociationJobName, h.Id)})
	j.SetEnqueueAllScopes(true)
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable:   utility.TruePtr(),
		MaxAttempts: utility.ToIntPtr(hostIPAssociationMaxAttempts),
	})
	return j
}

// kim: TODO: manually test job associates address
func (j *hostIPAssociationJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if err := j.populate(ctx); err != nil {
		j.AddError(errors.Wrap(err, "populating job"))
		return
	}

	if j.host.IPAllocationID == "" || j.host.IPAssociationID != "" {
		return
	}
	if j.host.Status != evergreen.HostStarting {
		return
	}

	cloudHost, err := cloud.GetCloudHost(ctx, j.host, j.env)
	if err != nil {
		j.AddError(errors.Wrapf(err, "getting cloud host for host '%s'", j.host.Id))
		return
	}
	if err := cloudHost.AssociateIP(ctx, j.host); err != nil {
		j.AddRetryableError(errors.Wrapf(err, "associating IP for host '%s'", j.host.Id))
	}
}

func (j *hostIPAssociationJob) populate(ctx context.Context) error {
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}
	if j.host == nil {
		h, err := host.FindOneId(ctx, j.HostID)
		if err != nil {
			return errors.Wrapf(err, "finding host '%s'", j.HostID)
		}
		if h == nil {
			return errors.Errorf("host '%s' not found", j.HostID)
		}
		j.host = h
	}
	return nil
}
