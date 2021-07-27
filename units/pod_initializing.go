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
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const createPodJobName = "create-pod"

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
		MaxAttempts: utility.ToIntPtr(15),
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

	switch j.pod.Status {
	case pod.StatusInitializing:
		execOpts := cocoa.NewECSPodExecutionOptions().
			SetCluster(j.pod.Resources.Cluster).
			SetExecutionRole(j.env.Settings().Providers.AWS.Pod.ECS.ExecutionRole)

		opts := cocoa.NewECSPodCreationOptions().
			SetContainerDefinitions([]cocoa.ECSContainerDefinition{
				cloud.ExportPodContainerDef(j.pod.TaskContainerCreationOpts, j.pod.Resources.SecretIDs),
			}).
			SetExecutionOptions(*execOpts).
			SetTaskRole(j.env.Settings().Providers.AWS.Pod.ECS.TaskRole)

		if _, err := j.ecsPodCreator.CreatePod(ctx, opts); err != nil {
			j.AddError(errors.Wrap(err, "running pod"))
			return
		}
	default:
		grip.Info(message.Fields{
			"message": "not starting pod because pod has already started or stopped",
			"pod":     j.PodID,
			"job":     j.ID(),
		})
	}

	if err := j.pod.UpdateStatus(pod.StatusStarting); err != nil {
		j.AddError(errors.Wrap(err, "marking pod as starting"))
	}
}
