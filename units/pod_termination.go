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
	"github.com/evergreen-ci/evergreen/model/task"
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
		if j.ecsClient != nil {
			j.AddError(errors.Wrap(j.ecsClient.Close(ctx), "closing ECS client"))
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
			"message":            "not deleting resources because pod has not initialized any yet",
			"pod":                j.PodID,
			"termination_reason": j.Reason,
			"job":                j.ID(),
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
			"message":            "pod is already terminated",
			"pod":                j.PodID,
			"termination_reason": j.Reason,
			"job":                j.ID(),
		})
		return
	default:
		grip.Error(message.Fields{
			"message":            "could not terminate pod with unrecognized status",
			"pod":                j.PodID,
			"status":             j.pod.Status,
			"termination_reason": j.Reason,
			"job":                j.ID(),
		})
	}

	if err := j.pod.UpdateStatus(pod.StatusTerminated, j.Reason); err != nil {
		j.AddError(errors.Wrap(err, "marking pod as terminated"))
	}

	grip.Info(message.Fields{
		"message":            "successfully terminated pod",
		"pod":                j.PodID,
		"included_on":        evergreen.ContainerHealthDashboard,
		"termination_reason": j.Reason,
		"job":                j.ID(),
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
			return errors.Wrapf(err, "finding pod '%s'", j.PodID)
		}
		if p == nil {
			return errors.Errorf("pod '%s' not found", j.PodID)
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

	if j.ecsClient == nil {
		client, err := cloud.MakeECSClient(settings)
		if err != nil {
			return errors.Wrap(err, "initializing ECS client")
		}
		j.ecsClient = client
	}

	if j.ecsPod == nil {
		ecsPod, err := cloud.ExportECSPod(j.pod, j.ecsClient, nil)
		if err != nil {
			return errors.Wrap(err, "exporting pod")
		}
		j.ecsPod = ecsPod
	}

	return nil
}

// fixStrandedTasks fixes tasks that are in an invalid state due to termination
// of this pod. If the pod is already running a task, that task is reset so that
// it can re-run if possible. If the pod is part of a dispatcher that will have
// no pods remaining to run tasks after this one is terminated, it marks the
// tasks in the dispatcher as needing re-allocation.
func (j *podTerminationJob) fixStrandedTasks(ctx context.Context) error {
	if err := j.fixStrandedRunningTask(ctx); err != nil {
		return errors.Wrapf(err, "fixing container task stranded on pod '%s'", j.pod.ID)
	}

	disp, err := dispatcher.FindOneByPodID(j.pod.ID)
	if err != nil {
		return errors.Wrapf(err, "finding dispatcher associated with pod '%s'", j.pod.ID)
	}
	if disp == nil {
		return nil
	}

	if err := disp.RemovePod(ctx, j.env, j.pod.ID); err != nil {
		return errors.Wrapf(err, "removing pod '%s' from dispatcher '%s'", j.pod.ID, disp.ID)
	}

	return nil
}

func (j *podTerminationJob) fixStrandedRunningTask(ctx context.Context) error {
	if j.pod.TaskRuntimeInfo.RunningTaskID == "" {
		return nil
	}

	// A stranded task will need to be re-allocated to ensure that it
	// dispatches to a new pod after this one is terminated.

	t, err := task.FindOneIdAndExecution(j.pod.TaskRuntimeInfo.RunningTaskID, j.pod.TaskRuntimeInfo.RunningTaskExecution)
	if err != nil {
		return errors.Wrapf(err, "finding stranded container task '%s' execution %d", j.pod.TaskRuntimeInfo.RunningTaskID, j.pod.TaskRuntimeInfo.RunningTaskExecution)
	}
	if t == nil {
		return nil
	}
	if t.Archived {
		grip.Warning(message.Fields{
			"message":   "stranded container task has already been archived, refusing to fix it",
			"task":      t.Id,
			"execution": t.Execution,
			"status":    t.Status,
		})
		return nil
	}
	grip.WarningWhen(!t.ContainerAllocated, message.Fields{
		"message":   "stranded container task was already marked as deallocated, refusing to fix it",
		"task":      t.Id,
		"execution": t.Execution,
		"status":    t.Status,
	})
	if t.ContainerAllocated {
		if err := t.MarkAsContainerDeallocated(ctx, j.env); err != nil {
			return errors.Wrapf(err, "marking stranded container task '%s' execution %d running on pod '%s' as deallocated", j.pod.TaskRuntimeInfo.RunningTaskID, j.pod.TaskRuntimeInfo.RunningTaskExecution, j.pod.ID)
		}
	}

	if err := model.ClearAndResetStrandedContainerTask(ctx, j.env.Settings(), j.pod); err != nil {
		return errors.Wrapf(err, "resetting stranded container task '%s' execution %d running on pod '%s'", j.pod.TaskRuntimeInfo.RunningTaskID, j.pod.TaskRuntimeInfo.RunningTaskExecution, j.pod.ID)
	}

	return nil
}
