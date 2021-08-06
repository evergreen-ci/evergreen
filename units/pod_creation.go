package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/pkg/errors"
)

const (
	createPodJobName     = "create-pod"
	createPodMaxAttempts = 15
)

func init() {
	registry.AddJobType(createPodJobName, func() amboy.Job {
		return makeCreatePodJob()
	})
}

type createPodJob struct {
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`
	PodID    string `bson:"pod_id" json:"pod_id" yaml:"pod_id"`

	pod           *pod.Pod
	smClient      cocoa.SecretsManagerClient
	vault         cocoa.Vault
	ecsClient     cocoa.ECSClient
	ecsPod        cocoa.ECSPod
	ecsPodCreator cocoa.ECSPodCreator
	env           evergreen.Environment
}

func makeCreatePodJob() *createPodJob {
	j := &createPodJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    createPodJobName,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

// NewCreatePodJob creates a job that starts the given pod.
func NewCreatePodJob(env evergreen.Environment, p *pod.Pod, id string) amboy.Job {
	j := makeCreatePodJob()
	j.pod = p
	j.PodID = p.ID
	j.env = env
	j.SetID(fmt.Sprintf("%s.%s.%s", createPodJobName, j.PodID, id))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", createPodJobName, j.PodID), podLifecycleScope(j.PodID)})
	j.SetShouldApplyScopesOnEnqueue(true)
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable:   utility.TruePtr(),
		MaxAttempts: utility.ToIntPtr(createPodMaxAttempts),
		WaitUntil:   utility.ToTimeDurationPtr(10 * time.Second),
	})

	return j
}

func (j *createPodJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	defer func() {
		if j.smClient != nil {
			j.AddError(j.smClient.Close(ctx))
		}
		if j.ecsClient != nil {
			j.AddError(j.ecsClient.Close(ctx))
		}
	}()
	if err := j.populateIfUnset(ctx); err != nil {
		j.AddError(err)
		return
	}

	switch j.pod.Status {
	case pod.StatusInitializing:
		opts, err := cloud.ExportPodCreationOptions(j.env.Settings().Providers.AWS.Pod.ECS, j.pod.TaskContainerCreationOpts)
		if err != nil {
			j.AddError(errors.Wrap(err, "exporting pod creation options"))
		}

		p, err := j.ecsPodCreator.CreatePod(ctx, opts)
		if err != nil {
			j.AddRetryableError(errors.Wrap(err, "starting pod"))
			return
		}

		j.ecsPod = p

		info, err := p.Info(ctx)
		if err != nil {
			j.AddError(errors.Wrap(err, "getting pod info"))
		}

		var ids []string
		for _, secret := range info.Resources.Secrets {
			ids = append(ids, utility.FromStringPtr(secret.NamedSecret.Name))
		}

		resourceInfo := pod.ResourceInfo{
			ExternalID:   utility.FromStringPtr(info.Resources.TaskID),
			DefinitionID: utility.FromStringPtr(info.Resources.TaskDefinition.ID),
			Cluster:      utility.FromStringPtr(info.Resources.Cluster),
			SecretIDs:    ids,
		}

		if err := j.pod.UpdateResources(resourceInfo); err != nil {
			j.AddError(errors.Wrap(err, "updating pod resources"))
		}

		if err := j.pod.UpdateStatus(pod.StatusStarting); err != nil {
			j.AddError(errors.Wrap(err, "marking pod as starting"))
		}

	default:
		j.AddError(errors.Errorf("not starting pod because pod status is '%s'", j.pod.Status))
	}
}

func (j *createPodJob) populateIfUnset(ctx context.Context) error {
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	if j.pod == nil {
		p, err := pod.FindOneByID(j.PodID)
		if err != nil {
			return err
		}
		if p == nil {
			return errors.New("pod not found")
		}
		j.pod = p
	}

	if j.pod.Status != pod.StatusInitializing {
		return nil
	}

	settings := j.env.Settings()

	if j.vault == nil {
		if j.smClient == nil {
			client, err := cloud.MakeSecretsManagerClient(settings)
			if err != nil {
				return errors.Wrap(err, "initializing Secrets Manager client")
			}
			j.smClient = client
		}
		j.vault = cloud.MakeSecretsManagerVault(j.smClient)
	}

	if j.ecsClient == nil {
		client, err := cloud.MakeECSClient(settings)
		if err != nil {
			return errors.Wrap(err, "initializing ECS client")
		}
		j.ecsClient = client
	}

	if j.ecsPodCreator == nil {
		creator, err := cloud.MakeECSPodCreator(j.ecsClient, j.vault)
		if err != nil {
			return errors.Wrap(err, "initializing ECS pod creator")
		}
		j.ecsPodCreator = creator
	}

	return nil
}
