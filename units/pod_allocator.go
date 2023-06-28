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

	task     *task.Task
	pRef     *model.ProjectRef
	env      evergreen.Environment
	settings evergreen.Settings
	smClient cocoa.SecretsManagerClient
	vault    cocoa.Vault
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
	defer func() {
		j.MarkComplete()

		if j.smClient != nil {
			j.AddError(errors.Wrap(j.smClient.Close(ctx), "closing Secrets Manager client"))
		}
	}()

	canAllocate, err := j.systemCanAllocate()
	if err != nil {
		j.AddRetryableError(errors.Wrap(err, "checking allocation attempt against max parallel pod request limit"))
		return
	}
	if !canAllocate {
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

	if err := j.populate(ctx); err != nil {
		j.AddRetryableError(errors.Wrap(err, "populating job"))
		return
	}

	if j.task.RemainingContainerAllocationAttempts() == 0 {
		// A task that has used up all of its container allocation attempts
		// should not try to allocate again.
		if err := model.MarkUnallocatableContainerTasksSystemFailed(ctx, j.env.Settings(), []string{j.TaskID}); err != nil {
			j.AddRetryableError(errors.Wrap(err, "marking unallocatable container task as system-failed"))
		}

		grip.Info(message.Fields{
			"message": "refusing to allocate task because it has no more allocation attempts",
			"task":    j.task.Id,
			"project": j.pRef.Identifier,
			"job":     j.ID(),
		})

		return
	}
	if !j.task.ShouldAllocateContainer() {
		grip.Info(message.Fields{
			"message": "refusing to allocate task because it is not in a valid state to be allocated",
			"task":    j.task.Id,
			"project": j.pRef.Identifier,
			"job":     j.ID(),
		})
		return
	}

	canDispatch, reason := model.ProjectCanDispatchTask(j.pRef, j.task)
	if !canDispatch {
		grip.Info(message.Fields{
			"message": "refusing to allocate task due to project ref settings",
			"reason":  reason,
			"task":    j.task.Id,
			"project": j.pRef.Identifier,
			"job":     j.ID(),
		})
		return
	}

	opts, err := j.getIntentPodOptions(ctx)
	if err != nil {
		j.AddError(errors.Wrap(err, "getting intent pod options"))
		return
	}

	intentPod, err := pod.NewTaskIntentPod(j.settings.Providers.AWS.Pod.ECS, *opts)
	if err != nil {
		j.AddError(errors.Wrap(err, "creating new task intent pod"))
		return
	}

	pd, err := dispatcher.Allocate(ctx, j.env, j.task, intentPod)
	if err != nil {
		j.AddRetryableError(errors.Wrap(err, "allocating pod for task dispatch"))
		return
	}

	if utility.StringSliceContains(pd.PodIDs, intentPod.ID) {
		grip.Info(message.Fields{
			"message":                    "successfully allocated pod for container task",
			"included_on":                evergreen.ContainerHealthDashboard,
			"task":                       j.task.Id,
			"pod":                        intentPod.ID,
			"secs_since_task_activation": time.Since(j.task.ActivatedTime).Seconds(),
		})
	} else {
		grip.Info(message.Fields{
			"message":                    "container task already has a pod allocated to run it",
			"included_on":                evergreen.ContainerHealthDashboard,
			"task":                       j.task.Id,
			"secs_since_task_activation": time.Since(j.task.ActivatedTime).Seconds(),
		})
	}
}

func (j *podAllocatorJob) systemCanAllocate() (canAllocate bool, err error) {
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
	if numInitializing >= settings.PodLifecycle.MaxParallelPodRequests {
		return false, nil
	}

	return true, nil
}

func (j *podAllocatorJob) populate(ctx context.Context) error {
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	// Use the latest service flags instead of those cached in the environment.
	settings := *j.env.Settings()
	if err := settings.ServiceFlags.Get(ctx); err != nil {
		return errors.Wrap(err, "getting service flags")
	}
	j.settings = settings

	if j.task == nil {
		t, err := task.FindOneId(j.TaskID)
		if err != nil {
			return errors.Wrapf(err, "finding task '%s'", j.TaskID)
		}
		if t == nil {
			return errors.Errorf("task '%s' not found", j.TaskID)
		}
		j.task = t
	}

	if j.pRef == nil {
		pRef, err := model.FindBranchProjectRef(j.task.Project)
		if err != nil {
			return errors.Wrapf(err, "finding project ref '%s' for task '%s'", j.task.Project, j.TaskID)
		}
		if pRef == nil {
			return errors.Errorf("project ref '%s' not found", j.task.Project)
		}
		j.pRef = pRef
	}

	if j.smClient == nil {
		client, err := cloud.MakeSecretsManagerClient(&settings)
		if err != nil {
			return errors.Wrap(err, "initializing Secrets Manager client")
		}
		j.smClient = client
	}
	if j.vault == nil {
		vault, err := cloud.MakeSecretsManagerVault(j.smClient)
		if err != nil {
			return errors.Wrap(err, "initializing Secrets Manager vault")
		}
		j.vault = vault
	}

	return nil
}

func (j *podAllocatorJob) getIntentPodOptions(ctx context.Context) (*pod.TaskIntentPodOptions, error) {
	var (
		repoCredsExternalID string
		podSecretExternalID string
	)
	for _, containerSecret := range j.pRef.ContainerSecrets {
		if j.task.ContainerOpts.RepoCredsName != "" && containerSecret.Name == j.task.ContainerOpts.RepoCredsName && containerSecret.Type == model.ContainerSecretRepoCreds {
			repoCredsExternalID = containerSecret.ExternalID
		}
		if containerSecret.Type == model.ContainerSecretPodSecret {
			podSecretExternalID = containerSecret.ExternalID
		}
	}
	if j.task.ContainerOpts.RepoCredsName != "" && repoCredsExternalID == "" {
		return nil, errors.Errorf("repository credentials '%s' could not be found in project ref '%s'", j.task.ContainerOpts.RepoCredsName, j.pRef.Identifier)
	}
	if podSecretExternalID == "" {
		return nil, errors.Errorf("pod secret for project ref '%s' not found", j.pRef.Identifier)
	}
	podSecret, err := j.vault.GetValue(ctx, podSecretExternalID)
	if err != nil {
		return nil, errors.Wrap(err, "getting pod secret value")
	}

	os, err := pod.ImportOS(j.task.ContainerOpts.OS)
	if err != nil {
		return nil, errors.Wrap(err, "importing OS")
	}
	arch, err := pod.ImportArch(j.task.ContainerOpts.Arch)
	if err != nil {
		return nil, errors.Wrap(err, "importing CPU architecture")
	}
	var winVer pod.WindowsVersion
	if j.task.ContainerOpts.WindowsVersion != "" {
		winVer, err = pod.ImportWindowsVersion(j.task.ContainerOpts.WindowsVersion)
		if err != nil {
			return nil, errors.Wrap(err, "importing Windows version")
		}
	}
	return &pod.TaskIntentPodOptions{
		CPU:                 j.task.ContainerOpts.CPU,
		MemoryMB:            j.task.ContainerOpts.MemoryMB,
		OS:                  os,
		Arch:                arch,
		WindowsVersion:      winVer,
		Image:               j.task.ContainerOpts.Image,
		RepoCredsExternalID: repoCredsExternalID,
		WorkingDir:          j.task.ContainerOpts.WorkingDir,
		PodSecretExternalID: podSecretExternalID,
		PodSecretValue:      podSecret,
	}, nil
}
