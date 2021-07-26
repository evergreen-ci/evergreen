package units

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/evergreen"
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

const podInitJobName = "pod-initializing-job"

func init() {
	registry.AddJobType(podInitJobName, func() amboy.Job {
		return makePodInitJob()
	})
}

type podInitJob struct {
	job.Base `bson:"metadata" json:"metadata" yaml:"metadata"`
	PodID    string `bson:"pod_id" json:"pod_id" yaml:"pod_id"`

	pod       *pod.Pod
	smClient  cocoa.SecretsManagerClient
	vault     cocoa.Vault
	ecsClient cocoa.ECSClient
	ecsPod    cocoa.ECSPod
	env       evergreen.Environment
}

func makePodInitJob() *podInitJob {
	j := &podInitJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    podInitJobName,
				Version: 0,
			},
		},
	}

	j.SetDependency(dependency.NewAlways())
	return j
}

// NewPodInitJob creates a job that starts the given pod.
func NewPodInitJob(env evergreen.Environment, p *pod.Pod, id string) amboy.Job {
	j := makePodInitJob()
	j.pod = p
	j.PodID = p.ID
	j.env = env
	j.SetPriority(1)
	j.SetID(fmt.Sprintf("%s.%s.%s", podInitJobName, j.PodID, id))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", podInitJobName, j.PodID)})
	j.SetShouldApplyScopesOnEnqueue(true)
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable: utility.TruePtr(),
		WaitUntil: utility.ToTimeDurationPtr(10 * time.Second),
	})

	return j
}

func (j *podInitJob) Run(ctx context.Context) {
	// TODO (WIP)
	// only field that will be set is job.Base and podID
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
		if _, err := j.ecsClient.RunTask(ctx, &ecs.RunTaskInput{
			Cluster:        utility.ToStringPtr(j.pod.Resources.Cluster),
			ReferenceId:    utility.ToStringPtr(j.PodID),
			TaskDefinition: utility.ToStringPtr(j.TaskID),
		}); err != nil {
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
}
