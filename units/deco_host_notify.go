package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const decoHostNotifyJobName = "deco-host-notify"

func init() {
	registry.AddJobType(decoHostNotifyJobName, func() amboy.Job { return makeDecoHostNotifyJob() })
}

type decoHostNotifyJob struct {
	HostID   string `bson:"host_id" json:"host_id" yaml:"host_id"`
	Message  string `bson:"message" json:"message" yaml:"message"`
	User     string `bson:"user" json:"user" yaml:"user"`
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`

	host *host.Host
	env  evergreen.Environment
}

func makeDecoHostNotifyJob() *decoHostNotifyJob {
	j := &decoHostNotifyJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    decoHostNotifyJobName,
				Version: 0,
			},
		},
	}
	return j
}

// NewDecoHostNotifyJob notifies the relevant team that a host has been
// decommissioned/quarantined and needs investigation.
func NewDecoHostNotifyJob(env evergreen.Environment, hostID, message, usr string) amboy.Job {
	j := makeDecoHostNotifyJob()
	j.env = env
	j.HostID = hostID
	j.Message = message
	j.User = usr

	j.SetID(fmt.Sprintf("%s.%s.%s", decoHostNotifyJobName, hostID, utility.RoundPartOfMinute(0)))
	return j
}

func (j *decoHostNotifyJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.host == nil {
		h, err := host.FindOneId(ctx, j.HostID)
		if err != nil {
			j.AddError(errors.Wrapf(err, "finding host '%s'", j.HostID))
		}
		if h == nil {
			j.AddError(errors.Errorf("host '%s' not found", j.HostID))
			return
		}
		j.host = h
	}

	if j.host.Status != evergreen.HostDecommissioned && j.host.Status != evergreen.HostQuarantined {
		return
	}

	m := message.Fields{
		"message":          "host has been decommissioned or quarantined",
		"detailed_message": j.Message,
		"job":              decoHostNotifyJobName,
		"distro":           j.host.Distro.Id,
		"provider":         j.host.Provider,
		"host_id":          j.host.Id,
		"host_status":      j.host.Status,
		"modified_by":      j.User,
	}
	if j.host.Provider != evergreen.ProviderNameStatic {
		uptime := time.Since(j.host.CreationTime)
		m["time_since_creation_secs"] = int(uptime.Seconds())
		m["time_since_creation_str"] = uptime.String()
	}

	grip.Notice(m)
}
