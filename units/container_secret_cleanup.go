package units

import (
	"context"
	"fmt"
	"strconv"

	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const (
	containerSecretCleanupJobName = "container-secret-cleanup"
)

func init() {
	registry.AddJobType(containerSecretCleanupJobName, func() amboy.Job {
		return makeContainerSecretCleanupJob()
	})
}

type containerSecretCleanupJob struct {
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`

	env       evergreen.Environment
	smClient  cocoa.SecretsManagerClient
	vault     cocoa.Vault
	tagClient cocoa.TagClient
}

func makeContainerSecretCleanupJob() *containerSecretCleanupJob {
	j := &containerSecretCleanupJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    containerSecretCleanupJobName,
				Version: 0,
			},
		},
	}
	return j
}

// NewContainerSecretCleanupJob creates a job that cleans up stranded container
// secrets that are not tracked by Evergreen.
func NewContainerSecretCleanupJob(id string) amboy.Job {
	j := makeContainerSecretCleanupJob()
	j.SetID(fmt.Sprintf("%s.%s", containerSecretCleanupJobName, id))
	return j
}

func (j *containerSecretCleanupJob) Run(ctx context.Context) {
	defer func() {
		j.MarkComplete()
	}()
	if err := j.populate(ctx); err != nil {
		j.AddError(err)
		return
	}

	secretIDs, err := cloud.GetFilteredResourceIDs(ctx, j.tagClient, []string{cloud.SecretsManagerResourceFilter}, map[string][]string{
		model.ContainerSecretTag: {strconv.FormatBool(false)},
	}, j.env.Settings().PodLifecycle.MaxSecretCleanupRate)
	if err != nil {
		j.AddError(errors.Wrap(err, "getting stranded Secrets Manager secrets"))
		return
	}

	catcher := grip.NewBasicCatcher()
	for _, secretID := range secretIDs {
		catcher.Wrapf(j.vault.DeleteSecret(ctx, secretID), "secret '%s'", secretID)
	}

	j.AddError(errors.Wrap(catcher.Resolve(), "deleting secrets"))
}

func (j *containerSecretCleanupJob) populate(ctx context.Context) error {
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	if j.smClient == nil {
		client, err := cloud.MakeSecretsManagerClient(ctx, j.env.Settings())
		if err != nil {
			return errors.Wrap(err, "initializing Secrets Manager client")
		}
		j.smClient = client
	}

	if j.vault == nil {
		vault, err := cloud.MakeSecretsManagerVault(j.smClient)
		if err != nil {
			return errors.Wrap(err, "initializing Secrets Manager vault")
		}
		j.vault = vault
	}

	if j.tagClient == nil {
		client, err := cloud.MakeTagClient(ctx, j.env.Settings())
		if err != nil {
			return errors.Wrap(err, "initializing tag client")
		}
		j.tagClient = client
	}

	return nil
}
