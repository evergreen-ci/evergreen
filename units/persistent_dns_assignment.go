package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	persistentDNSAssignmentJobName   = "persistent-dns-assignment"
	persistentDNSAssignmentBatchSize = 10
)

func init() {
	registry.AddJobType(persistentDNSAssignmentJobName, func() amboy.Job {
		return makePersistentDNSAssignment()
	})
}

type persistentDNSAssignmentJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`

	env evergreen.Environment
}

func makePersistentDNSAssignment() *persistentDNSAssignmentJob {
	return &persistentDNSAssignmentJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    persistentDNSAssignmentJobName,
				Version: 0,
			},
		},
	}
}

func NewPersistentDNSAssignmentJob(ts string) amboy.Job {
	j := makePersistentDNSAssignment()
	j.SetID(fmt.Sprintf("%s.%s", persistentDNSAssignmentJobName, ts))
	j.SetScopes([]string{persistentDNSAssignmentJobName})
	return j
}

func (j *persistentDNSAssignmentJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	hosts, err := host.FindUnexpirableRunningWithoutPersistentDNSName(ctx, persistentDNSAssignmentBatchSize)
	if err != nil {
		j.AddError(err)
		return
	}
	if len(hosts) == 0 {
		// TODO (DEVPROD-4040): remove this job once all existing unexpirable
		// hosts have been assigned a persistent DNS name.
		grip.Info(message.Fields{
			"message": "all unexpirable hosts have an assigned persistent DNS name",
			"job_id":  j.ID(),
		})
		return
	}

	for _, h := range hosts {
		cloudHost, err := cloud.GetCloudHost(ctx, &h, j.env)
		if err != nil {
			j.AddError(errors.Wrapf(err, "getting cloud host for host '%s'", h.Id))
			continue
		}

		// GetInstanceStatus implicitly refreshes the host's cached data to
		// include its public IPv4 address and persistent DNS name.
		cloudStatus, err := cloudHost.GetInstanceStatus(ctx)
		if err != nil {
			j.AddError(errors.Wrapf(err, "getting instance status for host '%s'", h.Id))
			continue
		}

		grip.InfoWhen(cloudStatus == cloud.StatusRunning, message.Fields{
			"message":             "successfully assigned persistent DNS name to unexpirable host",
			"ipv4_address":        h.PublicIPv4,
			"persistent_dns_name": h.PersistentDNSName,
			"host_id":             h.Id,
			"job_id":              j.ID(),
		})
	}
}
