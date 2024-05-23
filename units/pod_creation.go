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
	"github.com/evergreen-ci/evergreen/model/pod/dispatcher"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
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
	defer func() {
		j.MarkComplete()

		if j.pod != nil && j.pod.Status == pod.StatusInitializing && (j.RetryInfo().GetRemainingAttempts() == 0 || !j.RetryInfo().ShouldRetry()) {
			j.AddError(errors.Wrap(j.pod.UpdateStatus(pod.StatusDecommissioned, "pod failed to start and will not retry"), "updating pod status to decommissioned after pod failed to start"))

			terminationJob := NewPodTerminationJob(j.PodID, fmt.Sprintf("pod creation job hit max attempts %d", j.RetryInfo().MaxAttempts), time.Now())
			if err := amboy.EnqueueUniqueJob(ctx, j.env.RemoteQueue(), terminationJob); err != nil {
				j.AddError(errors.Wrap(err, "enqueueing job to terminate pod"))
			}

			grip.Info(message.Fields{
				"message": "decommissioned pod after it failed to be created",
				"pod":     j.pod.ID,
				"job":     j.ID(),
			})
		}
	}()
	if err := j.populateIfUnset(ctx); err != nil {
		j.AddRetryableError(err)
		return
	}

	switch j.pod.Status {
	case pod.StatusInitializing:
		execOpts, err := cloud.ExportECSPodExecutionOptions(j.env.Settings().Providers.AWS.Pod.ECS, j.pod.TaskContainerCreationOpts)
		if err != nil {
			j.AddError(errors.Wrap(err, "getting pod execution options"))
			return
		}

		// Wait for the pod definition to be asynchronously created. If the pod
		// definition is not ready yet, retry again later.
		podDef, err := j.checkForPodDefinition(j.pod.Family)
		if err != nil {
			j.AddRetryableError(errors.Wrap(err, "waiting for pod definition to be created"))
			return
		}

		p, err := j.ecsPodCreator.CreatePodFromExistingDefinition(ctx, cloud.ExportECSPodDefinition(*podDef), *execOpts)
		if err != nil {
			j.AddRetryableError(errors.Wrap(err, "starting pod"))
			return
		}

		j.ecsPod = p

		res := p.Resources()
		if err := j.pod.UpdateResources(cloud.ImportECSPodResources(res)); err != nil {
			j.AddError(errors.Wrap(err, "updating pod resources"))
		}

		// Bump the last communication time to ensure that the pod has a
		// sufficient grace period to start up.
		if err := j.pod.UpdateLastCommunicated(); err != nil {
			j.AddError(errors.Wrap(err, "updating pod last communication time"))
		}

		if err := j.pod.UpdateStatus(pod.StatusStarting, "pod successfully started"); err != nil {
			j.AddError(errors.Wrap(err, "marking pod as starting"))
		}

		if err := j.logTaskTimingStats(); err != nil {
			j.AddError(errors.Wrap(err, "logging task timing stats"))
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

	if j.ecsClient == nil {
		client, err := cloud.MakeECSClient(ctx, settings)
		if err != nil {
			return errors.Wrap(err, "initializing ECS client")
		}
		j.ecsClient = client
	}

	if j.ecsPodCreator == nil {
		creator, err := cloud.MakeECSPodCreator(j.ecsClient, nil)
		if err != nil {
			return errors.Wrap(err, "initializing ECS pod creator")
		}
		j.ecsPodCreator = creator
	}

	return nil
}

func (j *podCreationJob) checkForPodDefinition(family string) (*definition.PodDefinition, error) {
	podDef, err := definition.FindOneByFamily(family)
	if err != nil {
		return nil, errors.Wrapf(err, "finding pod definition with family '%s'", family)
	}
	if podDef == nil {
		return nil, errors.Errorf("pod definition with family '%s' not found", family)
	}

	grip.WarningWhen(podDef.LastAccessed.Add(podDefinitionTTL).Before(time.Now()), message.Fields{
		"message":        "Using a pod definition whose TTL has already elapsed, so it's at risk of being cleaned up. Starting this pod may fail if the pod definition gets cleaned up.",
		"pod":            j.pod.ID,
		"pod_definition": podDef.ID,
		"job":            j.ID(),
	})

	if err := podDef.UpdateLastAccessed(); err != nil {
		return nil, errors.Wrapf(err, "updating last access time for pod definition '%s'", podDef.ID)
	}

	return podDef, nil
}

func (j *podCreationJob) logTaskTimingStats() error {
	if j.pod.Type != pod.TypeAgent {
		return nil
	}

	disp, err := dispatcher.FindOneByPodID(j.pod.ID)
	if err != nil {
		return errors.Wrap(err, "finding dispatcher for task")
	}
	if disp == nil {
		return errors.Errorf("dispatcher with pod '%s' not found", j.pod.ID)
	}

	msg := message.Fields{
		"message":          "created pod to run container tasks",
		"included_on":      evergreen.ContainerHealthDashboard,
		"pod":              j.pod.ID,
		"dispatcher_group": disp.GroupID,
	}

	tsk, err := task.FindOneId(disp.GroupID)
	if err != nil {
		return errors.Wrapf(err, "finding tasks associated with dispatcher group '%s'", disp.GroupID)
	}

	if tsk != nil {
		msg["is_for_task_group"] = false
		msg["secs_since_task_activation"] = time.Since(tsk.ActivatedTime).Seconds()
	} else {
		// The dispatcher group will not be associated with a single task if
		// it's a task group. Task groups don't have a single activation time,
		// so we can't track statistics on the time since activation for a task
		// group.
		msg["is_for_task_group"] = true
	}

	grip.Info(msg)

	return nil
}
