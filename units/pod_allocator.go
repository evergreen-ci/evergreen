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
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson/primitive"
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

// kim: TODO: test
func (j *podAllocatorJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if err := j.populate(); err != nil {
		j.AddRetryableError(errors.Wrap(err, "populating job"))
		return
	}

	if !j.task.ShouldAllocateContainer() {
		return
	}

	podID := primitive.NewObjectID().Hex()
	intentPod, err := pod.NewTaskIntentPod(pod.TaskIntentPodOptions{
		// TODO (EVG-16371): fill in the actual values from the task's container
		// configuration. These are just placeholder values.
		ID:         podID,
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

	// groupID := j.getPodDispatcherGroupID()
	//
	// pd, err := dispatcher.FindOne(dispatcher.ByGroupID(groupID))
	// if err != nil {
	//     j.AddRetryableError(errors.Wrap(err, "checking for existing pod dispatcher"))
	//     return
	// }
	//
	// podID := primitive.NewObjectID().Hex()
	// if pd != nil {
	//     pd.PodIDs = append(pd.PodIDs, podID)
	//
	//     if !utility.StringSliceContains(pd.TaskIDs, j.task.Id) {
	//         pd.TaskIDs = append(pd.TaskIDs, j.task.Id)
	//     }
	//
	//     change, err := pd.UpsertAtomically()
	//     if err != nil {
	//         j.AddRetryableError(errors.Wrap(err, "updating existing pod dispatcher"))
	//         return
	//     }
	//     if change.Updated == 0 {
	//         j.AddRetryableError(errors.New("existing pod dispatcher was not updated"))
	//         return
	//     }
	// } else {
	//     pd := dispatcher.NewPodDispatcher(groupID, []string{podID}, []string{j.task.Id})
	//     if err := pd.Insert(); err != nil {
	//         j.AddRetryableError(errors.Wrap(err, "inserting new pod dispatcher"))
	//         return
	//     }
	// }
	//
	// intentPod, err := pod.NewTaskIntentPod(pod.TaskIntentPodOptions{
	//     // TODO (EVG-16371): fill in the actual values from the task's container
	//     // configuration. These are just placeholder values.
	//     ID:         podID,
	//     CPU:        1024,
	//     MemoryMB:   1024,
	//     OS:         pod.OSLinux,
	//     Arch:       pod.ArchAMD64,
	//     Image:      "ubuntu",
	//     WorkingDir: "/",
	// })
	// if err != nil {
	//     j.AddError(errors.Wrap(err, "creating new task intent pod"))
	//     return
	// }
	//
	// mongoClient := evergreen.GetEnvironment().Client()
	// session, err := mongoClient.StartSession()
	// if err != nil {
	//     j.AddRetryableError(errors.Wrap(err, "starting transaction session"))
	//     return
	// }
	// defer session.EndSession(ctx)
	//
	// insertPodAndUpdateTaskStatus := func(sessCtx mongo.SessionContext) (interface{}, error) {
	// }
	//
	// if _, err := session.WithTransaction(ctx, insertPodAndUpdateTaskStatus); err != nil {
	//     j.AddRetryableError(errors.Wrap(err, "transaction to insert pod and update task"))
	//     return
	// }
	//
	// if err := intentPod.Insert(); err != nil {
	//     j.AddRetryableError(errors.Wrap(err, "inserting new task intent pod"))
	//     return
	// }
	//
	// if err := j.task.MarkAsContainerAllocated(); err != nil {
	//     j.AddRetryableError(errors.Wrap(err, "marking task as container allocated"))
	//     return
	// }

	/*
		 kim: NOTE: all operations must be idempotent.
		 * Get DB state.
		 * Check task state is "waiting for container", activated, and not
		   disabled priority.
		 * Create or update pod group dispatch queue with new pod ID. If pod ID
		   already exists and is for a pod that's still active, update it
		   atomically. If pod ID does not correspond to an existing pod
		   document, replace with a new pod ID.
			   * kim: QUESTION: does the pod group dispatch queue need an
				incrementing mod lock to avoid concurrent modification issues on
				the agent side?
		 * Create intent pod with new pod ID.
		 * Change task state from "waiting for container" to "container allocated".
	*/
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
