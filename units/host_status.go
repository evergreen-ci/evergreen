package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const cloudHostReadyToProvisionJobName = "set-cloud-hosts-ready-to-provision"

func init() {
	registry.AddJobType(cloudHostReadyToProvisionJobName,
		func() amboy.Job { return makeCloudHostReadyToProvisionJob() })
}

type cloudHostReadyToProvisionJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment
}

// NewCloudHostReadyToProvisionJob gets statuses for all jobs created by Cloud providers which the Cloud providers'
// APIs have not yet returned all running. It marks the hosts running in the database.
func NewCloudHostReadyToProvisionJob(env evergreen.Environment, id string) amboy.Job {
	j := makeCloudHostReadyToProvisionJob()
	j.SetID(fmt.Sprintf("%s.%s", cloudHostReadyToProvisionJobName, id))
	j.env = env
	j.SetPriority(1)
	return j
}

func makeCloudHostReadyToProvisionJob() *cloudHostReadyToProvisionJob {
	j := &cloudHostReadyToProvisionJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    cloudHostReadyToProvisionJobName,
				Version: 1,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

func (j *cloudHostReadyToProvisionJob) Run(ctx context.Context) {
	var cancel context.CancelFunc
	ctx, cancel = context.WithCancel(ctx)
	defer cancel()
	defer j.MarkComplete()
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	hostsToCheck, err := host.Find(host.Starting())
	if err != nil {
		j.AddError(errors.Wrap(err, "problem finding hosts that are not running"))
		return
	}
	providers := map[string][]host.Host{}
	for _, h := range hostsToCheck {
		providers[h.Provider] = append(providers[h.Provider], h)
	}

	for p, hosts := range providers {
		m, err := cloud.GetManager(ctx, p, j.env.Settings())
		if err != nil {
			j.AddError(errors.Wrap(err, "error getting cloud manager"))
			return
		}
		if batch, ok := m.(cloud.BatchManager); ok {
			statuses, err := batch.GetInstanceStatuses(ctx, hosts)
			if err != nil {
				j.AddError(errors.Wrap(err, "error getting host statuses for providers"))
				continue
			}
			if len(statuses) != len(hosts) {
				j.AddError(errors.Errorf("programmer error: length of statuses != length of hosts"))
				continue
			}
			for i := range hosts {
				j.AddError(errors.Wrap(setCloudHostStatus(ctx, m, hosts[i], statuses[i]), "error settings cloud host status"))
			}
			continue
		}
		for _, h := range hosts {
			hostStatus, err := m.GetInstanceStatus(ctx, &h)
			if err != nil {
				j.AddError(errors.Wrapf(err, "error checking instance status of host %s", h.Id))
				continue
			}
			j.AddError(errors.Wrap(setCloudHostStatus(ctx, m, h, hostStatus), "error settings instance statuses"))
		}
	}
}

// setCloudHostStatus sets the host's status to HostProvisioning if host is running.
func setCloudHostStatus(ctx context.Context, m cloud.Manager, h host.Host, hostStatus cloud.CloudStatus) error {
	switch hostStatus {
	case cloud.StatusFailed:
		return errors.Wrap(m.TerminateInstance(ctx, &h, evergreen.User), "error terminating instance")
	case cloud.StatusRunning:
		return errors.Wrap(h.SetProvisioning(), "error setting host to provisioning")
	}
	grip.Info(message.Fields{
		"message": "host not ready for setup",
		"hostid":  h.Id,
		"DNS":     h.Host,
		"distro":  h.Distro.Id,
		"runner":  "hostinit",
	})
	return nil
}
