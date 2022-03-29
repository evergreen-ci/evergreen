package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/pod"
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

const podAllocatorJobName = "pod-allocator"

func init() {
	registry.AddJobType(podAllocatorJobName, func() amboy.Job {
		return makePodAllocatorJob()
	})
}

type podAllocatorJob struct {
	TaskID   string `bson:"task_id" json:"task_id"`
	job.Base `bson:"job_base" json:"job_base"`

	task *task.Task
	env  evergreen.Environment
}

func makePodAllocatorJob() *podAllocatorJob {
	return &podAllocatorJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    podAllocatorJobName,
				Version: 0,
			},
		},
	}
}

// NewPodAllocatorJob returns a job to allocate a pod for the given task ID.
func NewPodAllocatorJob(taskID, ts string) amboy.Job {
	j := makePodAllocatorJob()
	j.TaskID = taskID
	j.SetID(fmt.Sprintf("%s.%s.%s", podAllocatorJobName, taskID, ts))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", podAllocatorJobName, taskID)})
	j.SetEnqueueAllScopes(true)
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable:   utility.TruePtr(),
		MaxAttempts: utility.ToIntPtr(10),
		WaitUntil:   utility.ToTimeDurationPtr(10 * time.Second),
	})

	return j
}

func (j *podAllocatorJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	shouldAllocate, err := j.canAllocate()
	if err != nil {
		j.AddRetryableError(errors.Wrap(err, "checking allocation attempt against max parallel pod request limit"))
		return
	}
	if !shouldAllocate {
		grip.Info(message.Fields{
			"message":            "reached max parallel pod request limit, will re-attempt to allocate container to task later",
			"task":               j.TaskID,
			"remaining_attempts": j.RetryInfo().GetRemainingAttempts(),
			"job":                j.ID(),
		})
		j.UpdateRetryInfo(amboy.JobRetryOptions{
			NeedsRetry: utility.TruePtr(),
		})
		return
	}

	if err := j.populate(); err != nil {
		j.AddRetryableError(errors.Wrap(err, "populating job"))
		return
	}

	if !j.task.ShouldAllocateContainer() {
		return
	}

	intentPod, err := pod.NewTaskIntentPod(pod.TaskIntentPodOptions{
		// TODO (EVG-16371): These are just placeholder values for now. Fill in
		// the actual values from the task's container configuration once
		// they're available.
		CPU:        1024,
		MemoryMB:   1024,
		OS:         pod.OSLinux,
		Arch:       pod.ArchAMD64,
		Image:      "ubuntu",
		WorkingDir: "/",
	})
	if err != nil {
		j.AddError(errors.Wrap(err, "creating new task intent pod"))
		return
	}

	if _, err := dispatcher.Allocate(ctx, j.env, j.task, intentPod); err != nil {
		j.AddRetryableError(errors.Wrap(err, "allocating pod for task dispatch"))
		return
	}
}

func (j *podAllocatorJob) canAllocate() (shouldAllocate bool, err error) {
	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		return false, errors.Wrap(err, "getting service flags")
	}
	if flags.PodAllocatorDisabled {
		return false, nil
	}

	settings, err := evergreen.GetConfig()
	if err != nil {
		return false, errors.Wrap(err, "getting admin settings")
	}
	numInitializing, err := pod.CountByInitializing()
	if err != nil {
		return false, errors.Wrap(err, "counting initializing pods")
	}
	if numInitializing >= settings.PodInit.MaxParallelPodRequests {
		return false, nil
	}

	return true, nil
}

func (j *podAllocatorJob) populate() error {
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	if j.task == nil {
		t, err := task.FindOneId(j.TaskID)
		if err != nil {
			return errors.Wrapf(err, "finding task '%s'", j.TaskID)
		}
		if t == nil {
			return errors.New("task not found")
		}
		j.task = t
	}

	return nil
}
