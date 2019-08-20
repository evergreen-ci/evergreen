package units

import (
	"context"
	"fmt"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

const staticUpdateSSHKeysJobName = "update-ssh-key-pairs-host"

type staticUpdateSSHKeysJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	HostID   string `bson:"host_id" json:"host_id" yaml:"host_id"`

	env  evergreen.Environment
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
	j.SetDependency(dependency.NewAlways())
	return j
}

// NewStaticUpdateSSHKeysJob updates the SSH keys for a static host.
func NewStaticUpdateSSHKeysJob(h host.Host, id string) amboy.Job {
	j := makeStaticUpdateSSHKeysJob()
	j.host = &h

	j.HostID = h.Id
	j.SetID(fmt.Sprintf("%s.%s", staticUpdateSSHKeysJobName, id))
	return j
}

func (j *staticUpdateSSHKeysJob) Run(ctx context.Context) {
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	if j.host == nil {
		h, err := host.FindOneId(j.HostID)
		if err != nil {
			j.AddError(err)
		}
		j.host = h
	}

	settings := j.env.Settings()

	if len(settings.SSHKeyPairs) == 0 {
		return
	}

	pubKeys := []string{}
	names := []string{}
	for _, pair := range settings.SSHKeyPairs {
		pubKeys = append(pubKeys, fmt.Sprintf("'%s'", pair.Public))
		names = append(names, pair.Name)
	}

	sshOpts, err := j.host.GetSSHOptions(settings)
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "could not get SSH options",
			"host":    j.host.Id,
			"job":     j.ID(),
		}))
		j.AddError(err)
		return
	}

	// Either key is already in authorized keys or it is appended.
	addKeysCmd := fmt.Sprintf(`pub_keys=(%s);
		for key in "${pub_keys[@]}"; do
			grep "^${key}$" ~/.ssh/authorized_keys2 || echo "${key}" >> ~/.ssh/authorized_keys2;
		done`, strings.Join(pubKeys, " "))
	if out, err := j.host.RunSSHCommand(ctx, addKeysCmd, sshOpts); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "could not run SSH command to add to authorized keys",
			"host":    j.host.Id,
			"logs":    out,
		}))
		j.AddError(err)
	}
	if j.HasErrors() {
		return
	}

	if err := j.host.SetSSHKeyNames(names); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "could not set host SSH key names",
			"host":    j.host.Id,
			"job":     j.ID(),
		}))
		j.AddError(err)
	}

	return
}
