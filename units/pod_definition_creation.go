package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/model/pod/definition"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	podDefinitionCreationJobName     = "pod-definition-creation"
	podDefinitionCreationMaxAttempts = 15
)

func init() {
	registry.AddJobType(podDefinitionCreationJobName, func() amboy.Job {
		return makePodDefinitionCreationJob()
	})
}

type podDefinitionCreationJob struct {
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`
	Opts     pod.TaskContainerCreationOptions `bson:"opts" json:"opts" yaml:"opts"`

	ecsClient        cocoa.ECSClient
	ecsPodDefManager cocoa.ECSPodDefinitionManager
	smClient         cocoa.SecretsManagerClient
	vault            cocoa.Vault
	env              evergreen.Environment
	settings         evergreen.Settings
}

func makePodDefinitionCreationJob() *podDefinitionCreationJob {
	j := &podDefinitionCreationJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    podDefinitionCreationJobName,
				Version: 0,
			},
		},
	}
	return j
}

// NewPodDefinitionCreationJob creates a job that creates a pod definition in
// preparation for running a pod.
func NewPodDefinitionCreationJob(opts pod.TaskContainerCreationOptions, id string) amboy.Job {
	j := makePodDefinitionCreationJob()
	intentDigest := opts.Hash()
	j.SetID(fmt.Sprintf("%s.%s.%s", podCreationJobName, intentDigest, id))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", podDefinitionCreationJobName, intentDigest)})
	j.SetEnqueueAllScopes(true)
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable:   utility.TruePtr(),
		MaxAttempts: utility.ToIntPtr(podDefinitionCreationMaxAttempts),
		WaitUntil:   utility.ToTimeDurationPtr(10 * time.Second),
	})

	return j
}

func (j *podDefinitionCreationJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	// kim: TODO: if pod definition creation fails and won't retry, decommission
	// failed intent pods.

	defer func() {
		if j.smClient != nil {
			j.AddError(j.smClient.Close(ctx))
		}
		if j.ecsClient != nil {
			j.AddError(j.ecsClient.Close(ctx))
		}
	}()
	if err := j.populateIfUnset(ctx); err != nil {
		j.AddRetryableError(err)
		return
	}

	intentDigest := j.Opts.Hash()

	podDefOpts, err := cloud.ExportECSPodDefinitionOptions(j.settings, j.Opts)
	if err != nil {
		j.AddError(errors.Wrapf(err, "creating pod definition from pod '%s' with intent digest '%s'", intentDigest))
		return
	}

	digest := podDefOpts.Hash()
	podDef, err := definition.FindOneByDigest(digest)
	if err != nil {
		j.AddRetryableError(errors.Wrapf(err, "checking for existing pod definition with digest '%s'", digest))
		return
	}
	if podDef != nil {
		return
	}

	item, err := j.ecsPodDefManager.CreatePodDefinition(ctx, *podDefOpts)
	if err != nil {
		j.AddRetryableError(errors.Wrapf(err, "creating pod definition from pod '%s' with digest '%s'", intentDigest))
		return
	}

	grip.Info(message.Fields{
		"message":       "successfully created pod definition",
		"intent_digest": intentDigest,
		"digest":        digest,
		"external_id":   item.ID,
	})
}

func (j *podDefinitionCreationJob) populateIfUnset(ctx context.Context) error {
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	// Use the latest service flags instead of those cached in the environment.
	settings := *j.env.Settings()
	if err := settings.ServiceFlags.Get(j.env); err != nil {
		return errors.Wrap(err, "getting service flags")
	}
	j.settings = settings

	if j.vault == nil {
		if j.smClient == nil {
			client, err := cloud.MakeSecretsManagerClient(&settings)
			if err != nil {
				return errors.Wrap(err, "initializing Secrets Manager client")
			}
			j.smClient = client
		}
		j.vault = cloud.MakeSecretsManagerVault(j.smClient)
	}

	if j.ecsClient == nil {
		client, err := cloud.MakeECSClient(&settings)
		if err != nil {
			return errors.Wrap(err, "initializing ECS client")
		}
		j.ecsClient = client
	}

	if j.ecsPodDefManager == nil {
		podDefMgr, err := cloud.MakeECSPodDefinitionManager(j.ecsClient, j.vault)
		if err != nil {
			return errors.Wrap(err, "initializing ECS pod creator")
		}
		j.ecsPodDefManager = podDefMgr
	}

	return nil
}
