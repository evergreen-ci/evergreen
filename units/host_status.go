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

const cloudHostReadyJobName = "set-cloud-hosts-ready"

func init() {
	registry.AddJobType(cloudHostReadyJobName,
		func() amboy.Job { return makeCloudHostReadyJob() })
}

type cloudHostReadyJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	env      evergreen.Environment
}

// NewCloudHostReadyJob gets statuses for all jobs created by Cloud providers which the Cloud providers'
// APIs have not yet returned all running. It marks the hosts running in the database.
func NewCloudHostReadyJob(env evergreen.Environment, id string) amboy.Job {
	j := makeCloudHostReadyJob()
	j.SetID(fmt.Sprintf("%s.%s", cloudHostReadyJobName, id))
	j.env = env
	j.SetPriority(1)
	return j
}

func makeCloudHostReadyJob() *cloudHostReadyJob {
	j := &cloudHostReadyJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    cloudHostReadyJobName,
				Version: 1,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

func (j *cloudHostReadyJob) Run(ctx context.Context) {
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
	if len(hostsToCheck) == 0 {
		return
	}
	providers := map[string][]host.Host{}
	for _, h := range hostsToCheck {
		providers[h.Provider] = append(providers[h.Provider], h)
	}

	for p, hosts := range providers {
		if len(hosts) == 0 {
			continue
		}
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
			hostIDs := []string{}
			for _, h := range hosts {
				hostIDs = append(hostIDs, h.Id)
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
		grip.Debug(message.Fields{
			"ticket":     "EVG-6100",
			"message":    "host status",
			"host":       h,
			"hostStatus": hostStatus.String(),
		})
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
