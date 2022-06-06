package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/model/pod/dispatcher"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const podTerminationJobName = "pod-termination"

func init() {
	registry.AddJobType(podTerminationJobName, func() amboy.Job {
		return makePodTerminationJob()
	})
}

type podTerminationJob struct {
	job.Base `bson:"metadata" json:"metadata"`
	PodID    string `bson:"pod_id" json:"pod_id"`
	Reason   string `bson:"reason,omitempty" json:"reason,omitempty"`

	pod       *pod.Pod
	smClient  cocoa.SecretsManagerClient
	vault     cocoa.Vault
	ecsClient cocoa.ECSClient
	ecsPod    cocoa.ECSPod
	env       evergreen.Environment
}

// podLifecycleScope is a job scope that applies to all jobs that seek to make
// transitions within the pod lifecycle.
func podLifecycleScope(id string) string {
	return fmt.Sprintf("pod-lifecycle.%s", id)
}

func makePodTerminationJob() *podTerminationJob {
	j := &podTerminationJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    podTerminationJobName,
				Version: 0,
			},
		},
	}
	return j
}

// NewPodTerminationJob creates a job to terminate the given pod with the given
// termination reason. Callers should populate the reason with as much context
// as possible for why the pod is being terminated.
func NewPodTerminationJob(podID, reason string, ts time.Time) amboy.Job {
	j := makePodTerminationJob()
	j.PodID = podID
	j.Reason = reason
	j.SetScopes([]string{fmt.Sprintf("%s.%s", podTerminationJobName, podID), podLifecycleScope(podID)})
	j.SetEnqueueAllScopes(true)
	j.SetID(fmt.Sprintf("%s.%s.%s", podTerminationJobName, podID, ts.Format(TSFormat)))

	return j
}

func (j *podTerminationJob) Run(ctx context.Context) {
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

	if err := j.fixStrandedTasks(ctx); err != nil {
		j.AddError(errors.Wrapf(err, "fixing tasks stranded by termination of pod '%s'", j.pod.ID))
		return
	}

	switch j.pod.Status {
	case pod.StatusInitializing:
		grip.Info(message.Fields{
			"message":                    "not deleting resources because pod has not initialized any yet",
			"pod":                        j.PodID,
			"termination_attempt_reason": j.Reason,
			"job":                        j.ID(),
		})
	case pod.StatusStarting, pod.StatusRunning, pod.StatusDecommissioned:
		if j.ecsPod != nil {
			if err := j.ecsPod.Delete(ctx); err != nil {
				j.AddError(errors.Wrap(err, "deleting pod resources"))
				return
			}
		}
	case pod.StatusTerminated:
		grip.Info(message.Fields{
			"message":                    "pod is already terminated",
			"pod":                        j.PodID,
			"termination_attempt_reason": j.Reason,
			"job":                        j.ID(),
		})
		return
	default:
		grip.Error(message.Fields{
			"message":                    "could not terminate pod with unrecognized status",
			"pod":                        j.PodID,
			"status":                     j.pod.Status,
			"termination_attempt_reason": j.Reason,
			"job":                        j.ID(),
		})
	}

	if err := j.pod.UpdateStatus(pod.StatusTerminated); err != nil {
		j.AddError(errors.Wrap(err, "marking pod as terminated"))
	}

	grip.Info(message.Fields{
		"message":                    "successfully terminated pod",
		"pod":                        j.PodID,
		"termination_attempt_reason": j.Reason,
		"job":                        j.ID(),
	})
}

func (j *podTerminationJob) populateIfUnset(ctx context.Context) error {
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	if j.ecsPod != nil && j.pod != nil {
		return nil
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

	// The pod does not exist in the cloud if's already deleted (i.e. it's
	// already terminated) or it never created any external resources (i.e. the
	// pod resources is empty).
	if j.pod.Status == pod.StatusTerminated || j.pod.Resources.IsZero() {
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

	ecsPod, err := cloud.ExportECSPod(j.pod, j.ecsClient, j.vault)
	if err != nil {
		return errors.Wrap(err, "exporting pod")
	}
	j.ecsPod = ecsPod

	return nil
}

// kim: TODO: check if this pod is currently running a task or is part of a
// pod group to decide if the tasks need resetting.
// If it's the last pod running in a dispatch queue and there are still
// tasks in the dispatch queue, reset all tasks in the dispatch queue to
// need reallocation. If it's running a task, reset that one task to need
// reallocation.

// fixStrandedTasks fixes tasks that are in an invalid state due to termination
// of this pod. If the pod is already running a task, that task is reset so that
// it can re-run. If the pod is part of a dispatcher that will have no pods left
// to run tasks after this one is terminated, it marks the tasks in the
// dispatcher as needing re-allocation.
func (j *podTerminationJob) fixStrandedTasks(ctx context.Context) error {
	if j.pod.RunningTask != "" {
		if err := model.ClearAndResetStrandedContainerTask(j.pod); err != nil {
			return errors.Wrap(err, "resetting stranded container task running on pod")
		}
	}

	disp, err := dispatcher.FindOneByPodID(j.pod.ID)
	if err != nil {
		return errors.Wrap(err, "finding dispatcher associated with pod")
	}
	if disp == nil {
		return nil
	}

	// kim: TODO: update dispatcher to remove pod and tasks, and mark tasks as
	// needing re-allocation. Maybe need transaction to do this safely. But
	// given that the task re-allocation update just sets
	// ContainerAllocated=false, it might not be necessary? It might be
	// sufficient to use an atomic update on the task collection with the IDs
	// and possibly check the other keys to make sure that they're still in a
	// needs-dispatch state. Since ContainerAllocated can only be set by pod
	// allocation and a pod allocation should not be happening right now (since
	// we have only one pod in the dispatch queue for now), it should be safe to
	// clear the field since there cannot be a race.
	if err := disp.RemovePod(ctx, j.env, j.pod.ID); err != nil {
		return errors.Wrap(err, "removing pod from dispatcher")
	}

	return nil
}
