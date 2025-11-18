package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	cloudCleanupName = "cloud-cleanup"
)

func init() {
	registry.AddJobType(cloudCleanupName,
		func() amboy.Job { return makeCloudCleanupNameJob() })
}

type cloudCleanupJob struct {
	job.Base `bson:"job_base" json:"job_base" yaml:"job_base"`

	// Provider is the cloud provider to perform cleanup for.
	Provider string
	// Region is the cloud region to clean up.
	Region string

	env evergreen.Environment
}

func makeCloudCleanupNameJob() *cloudCleanupJob {
	j := &cloudCleanupJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    cloudCleanupName,
				Version: 0,
			},
		},
	}
	return j
}

// NewCloudCleanupJob returns a job to call the cloud manager's Cleanup method for the given Provider and Region.
func NewCloudCleanupJob(env evergreen.Environment, ts, provider, region string) amboy.Job {
	j := makeCloudCleanupNameJob()
	j.SetID(fmt.Sprintf("%s.%s.%s.%s", cloudCleanupName, provider, region, ts))
	j.SetScopes([]string{fmt.Sprintf("%s.%s.%s", cloudCleanupName, provider, region)})
	j.SetEnqueueAllScopes(true)
	j.Provider = provider
	j.Region = region

	j.env = env

	return j
}

func (j *cloudCleanupJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	accountRoles := j.env.Settings().Providers.AWS.AccountRoles
	//Adding the empty value account as internally empty string is mapped to Kernel-Build AWS account
	accountRoles = append(accountRoles, evergreen.AWSAccountRoleMapping{
		Account: "",
	})

	for _, accountRole := range accountRoles {
		grip.Info(fmt.Sprintf("starting clean up for provider with account '%s'", accountRole.Account))

		cloudManager, err := cloud.GetManager(ctx, j.env, cloud.ManagerOpts{
			Provider: j.Provider,
			Region:   j.Region,
			Account:  accountRole.Account,
		})

		if err != nil {
			j.AddError(errors.Wrapf(err, "getting cloud manager for provider '%s' in region '%s'", j.Provider, j.Region))
			return
		}

		err = cloudManager.Cleanup(ctx)
		j.AddError(errors.Wrap(err, "cleaning up for provider"))
		grip.Error(message.WrapError(err, message.Fields{
			"message":  "cleaning up cloud resources",
			"provider": j.Provider,
			"region":   j.Region,
			"job_id":   j.ID(),
		}))
	}
}
