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
	podCreationJobName     = "pod-creation"
	podCreationMaxAttempts = 15
)

func init() {
	registry.AddJobType(podCreationJobName, func() amboy.Job {
		return makePodCreationJob()
	})
}

type podCreationJob struct {
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

func makePodCreationJob() *podCreationJob {
	j := &podCreationJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    podCreationJobName,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

// NewPodCreationJob creates a job that starts the pod in the container service.
func NewPodCreationJob(podID, id string) amboy.Job {
	j := makePodCreationJob()
	j.PodID = podID
	j.SetID(fmt.Sprintf("%s.%s.%s", podCreationJobName, j.PodID, id))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", podCreationJobName, j.PodID), podLifecycleScope(j.PodID)})
	j.SetEnqueueAllScopes(true)
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable:   utility.TruePtr(),
		MaxAttempts: utility.ToIntPtr(podCreationMaxAttempts),
		WaitUntil:   utility.ToTimeDurationPtr(10 * time.Second),
	})

	return j
}

func (j *podCreationJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	defer func() {
		if j.smClient != nil {
			j.AddError(j.smClient.Close(ctx))
		}
		if j.ecsClient != nil {
			j.AddError(j.ecsClient.Close(ctx))
		}

		if j.pod != nil && j.pod.Status == pod.StatusInitializing && (j.RetryInfo().GetRemainingAttempts() == 0 || !j.RetryInfo().ShouldRetry()) {
			j.AddError(errors.Wrap(j.pod.UpdateStatus(pod.StatusDecommissioned), "updating pod status to decommissioned after pod failed to start"))
		}
	}()
	if err := j.populateIfUnset(ctx); err != nil {
		j.AddRetryableError(err)
		return
	}

	settings := *j.env.Settings()
	// Use the latest service flags instead of those cached in the environment.
	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		j.AddRetryableError(errors.Wrap(err, "getting service flags"))
		return
	}
	settings.ServiceFlags = *flags

	switch j.pod.Status {
	case pod.StatusInitializing:
		opts, err := cloud.ExportECSPodCreationOptions(&settings, j.pod)
		if err != nil {
			j.AddError(errors.Wrap(err, "exporting pod creation options"))
			return
		}

		p, err := j.ecsPodCreator.CreatePod(ctx, *opts)
		if err != nil {
			j.AddRetryableError(errors.Wrap(err, "starting pod"))
			return
		}

		j.ecsPod = p

		res := p.Resources()
		if err := j.pod.UpdateResources(cloud.ImportECSPodResources(res)); err != nil {
			j.AddError(errors.Wrap(err, "updating pod resources"))
		}

		if err := j.pod.UpdateStatus(pod.StatusStarting); err != nil {
			j.AddError(errors.Wrap(err, "marking pod as starting"))
		}

	default:
		j.AddError(errors.Errorf("not starting pod because pod status is '%s'", j.pod.Status))
	}
}

func (j *podCreationJob) populateIfUnset(ctx context.Context) error {
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
