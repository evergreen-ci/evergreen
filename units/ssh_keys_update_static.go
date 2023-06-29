package units

import (
	"context"
	"fmt"

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

const staticUpdateSSHKeysJobName = "update-ssh-keys-host"

type staticUpdateSSHKeysJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	HostID   string `bson:"host_id" json:"host_id" yaml:"host_id"`

	host *host.Host
}

func init() {
	registry.AddJobType(staticUpdateSSHKeysJobName, func() amboy.Job {
		return makeStaticUpdateSSHKeysJob()
	})
}

func makeStaticUpdateSSHKeysJob() *staticUpdateSSHKeysJob {
	j := &staticUpdateSSHKeysJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    staticUpdateSSHKeysJobName,
				Version: 0,
			},
		},
	}
	return j
}

// NewStaticUpdateSSHKeysJob updates the SSH keys for a static host.
func NewStaticUpdateSSHKeysJob(h host.Host, id string) amboy.Job {
	j := makeStaticUpdateSSHKeysJob()
	j.host = &h

	j.HostID = h.Id
	j.SetID(fmt.Sprintf("%s.%s.%s", staticUpdateSSHKeysJobName, h.Id, id))
	return j
}

func (j *staticUpdateSSHKeysJob) Run(ctx context.Context) {
	if j.host == nil {
		h, err := host.FindOneId(ctx, j.HostID)
		if err != nil {
			j.AddError(errors.Wrapf(err, "finding host '%s'", j.HostID))
			return
		}
		if h == nil {
			j.AddError(errors.Errorf("host '%s' not found", j.HostID))
			return
		}
		j.host = h
	}
	if j.host.Status != evergreen.HostRunning {
		return
	}

	settings := evergreen.GetEnvironment().Settings()

	for _, pair := range settings.SSHKeyPairs {
		// Ignore if host already contains the public key.
		if utility.StringSliceContains(j.host.SSHKeyNames, pair.Name) {
			continue
		}

		// Either key is already in the authorized keys or it is appended.
		addKeyCmd := j.host.AddPublicKeyScript(pair.Public)
		if logs, err := j.host.RunSSHCommand(ctx, addKeyCmd); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "could not run SSH command to add to authorized keys",
				"host_id": j.host.Id,
				"key":     pair.Name,
				"logs":    logs,
			}))
			j.AddError(err)
			return
		}
		if err := j.host.AddSSHKeyName(ctx, pair.Name); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "could not add SSH key name to host",
				"host_id": j.host.Id,
				"name":    pair.Name,
				"job":     j.ID(),
			}))
			j.AddError(err)
			return
		}
	}
}
