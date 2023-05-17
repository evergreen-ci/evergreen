package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const cloudUpdateSSHKeysJobName = "update-ssh-keys-cloud"

type cloudUpdateSSHKeysJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`
	Region   string `bson:"region" json:"region" yaml:"region"`
	Provider string `bson:"provider" json:"provider" yaml:"provider"`

	env evergreen.Environment
}

func init() {
	registry.AddJobType(cloudUpdateSSHKeysJobName, func() amboy.Job {
		return makeCloudUpdateSSHKeysJob()
	})
}

func makeCloudUpdateSSHKeysJob() *cloudUpdateSSHKeysJob {
	j := &cloudUpdateSSHKeysJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    cloudUpdateSSHKeysJobName,
				Version: 0,
			},
		},
	}
	return j
}

// NewCloudUpdateSSHKeysJob updates the SSH keys available for a single region
// in the given cloud provider.
func NewCloudUpdateSSHKeysJob(provider, region, id string) amboy.Job {
	j := makeCloudUpdateSSHKeysJob()
	j.Provider = provider
	j.Region = region
	j.SetID(fmt.Sprintf("%s.%s.%s.%s", cloudUpdateSSHKeysJobName, provider, region, id))
	return j
}

func (j *cloudUpdateSSHKeysJob) Run(ctx context.Context) {
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	if j.Provider == "" || j.Region == "" {
		err := errors.New("provider or region was not specified")
		grip.Error(message.WrapError(err, message.Fields{
			"message":  "could not update ssh keys",
			"provider": j.Provider,
			"region":   j.Region,
			"job":      j.ID(),
		}))
		j.AddError(err)
		return
	}
	mgr, err := cloud.GetManager(ctx, j.env, cloud.ManagerOpts{
		Provider: j.Provider,
		Region:   j.Region,
	})
	if err != nil {
		j.AddError(errors.Wrap(err, "could not get cloud manager"))
		return
	}

	settings := j.env.Settings()

	for _, pair := range settings.SSHKeyPairs {
		switch j.Provider {
		case evergreen.ProviderNameEc2Fleet, evergreen.ProviderNameEc2OnDemand:
			// Ignore if region already contains the public key.
			if utility.StringSliceContains(pair.EC2Regions, j.Region) {
				continue
			}
		default:
			err := errors.New("adding SSH keys has not been implemented for this provider")
			grip.Critical(message.WrapError(err, message.Fields{
				"provider": j.Provider,
				"region":   j.Region,
				"job":      j.ID(),
			}))
			j.AddError(err)
			return
		}

		if err := mgr.AddSSHKey(ctx, pair); err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":  "could not add SSH key to cloud manager",
				"provider": j.Provider,
				"region":   j.Region,
				"job":      j.ID(),
			}))
			j.AddError(err)
			return
		}

		switch j.Provider {
		case evergreen.ProviderNameEc2Fleet, evergreen.ProviderNameEc2OnDemand:
			if err := pair.AddEC2Region(j.Region); err != nil {
				grip.Error(message.WrapError(err, message.Fields{
					"message":  "could not update EC2 regions for SSH key",
					"provider": j.Provider,
					"region":   j.Region,
					"job":      j.ID(),
				}))
				j.AddError(err)
				return
			}
		default:
			err := errors.New("adding SSH keys has not been implemented for this provider")
			grip.Critical(message.WrapError(err, message.Fields{
				"provider": j.Provider,
				"region":   j.Region,
				"job":      j.ID(),
			}))
			j.AddError(err)
			return
		}
	}
}
