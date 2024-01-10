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
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	podDefinitionCreationJobName     = "pod-definition-creation"
	podDefinitionCreationMaxAttempts = 15
)

func init() {
	registry.AddJobType(podDefinitionCreationJobName, func() amboy.Job {
		return makePodDefinitionCreationJob()
	})
}

type podDefinitionCreationJob struct {
	job.Base      `bson:"metadata" json:"metadata" yaml:"metadata"`
	ContainerOpts pod.TaskContainerCreationOptions `bson:"container_opts" json:"container_opts" yaml:"container_opts"`
	Family        string                           `bson:"family" json:"family" yaml:"family"`

	ecsClient cocoa.ECSClient
	podDefMgr cocoa.ECSPodDefinitionManager
	env       evergreen.Environment
	settings  evergreen.Settings
}

func makePodDefinitionCreationJob() *podDefinitionCreationJob {
	j := &podDefinitionCreationJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    podDefinitionCreationJobName,
				Version: 0,
			},
		},
	}
	return j
}

// NewPodDefinitionCreationJob creates a job that creates a pod definition in
// preparation for running a pod.
func NewPodDefinitionCreationJob(ecsConf evergreen.ECSConfig, opts pod.TaskContainerCreationOptions, id string) amboy.Job {
	j := makePodDefinitionCreationJob()
	j.ContainerOpts = opts
	j.Family = j.ContainerOpts.GetFamily(ecsConf)
	j.SetID(fmt.Sprintf("%s.%s.%s", podDefinitionCreationJobName, j.Family, id))
	j.SetScopes([]string{fmt.Sprintf("%s.%s", podDefinitionCreationJobName, j.Family)})
	j.SetEnqueueAllScopes(true)
	j.UpdateRetryInfo(amboy.JobRetryOptions{
		Retryable:   utility.TruePtr(),
		MaxAttempts: utility.ToIntPtr(podDefinitionCreationMaxAttempts),
		WaitUntil:   utility.ToTimeDurationPtr(10 * time.Second),
	})

	return j
}

func (j *podDefinitionCreationJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	defer func() {
		if j.ecsClient != nil {
			j.AddError(errors.Wrap(j.ecsClient.Close(ctx), "closing ECS client"))
		}

		if j.HasErrors() && (!j.RetryInfo().ShouldRetry() || j.RetryInfo().GetRemainingAttempts() == 0) {
			j.AddError(errors.Wrap(j.decommissionDependentIntentPods(), "decommissioning intent pods after pod definition creation failed"))
		}
	}()

	defer func() {
		if j.ecsClient != nil {
			j.AddError(j.ecsClient.Close(ctx))
		}
	}()
	if err := j.populateIfUnset(ctx); err != nil {
		j.AddRetryableError(err)
		return
	}

	dependents, err := pod.FindIntentByFamily(j.Family)
	if err != nil {
		j.AddRetryableError(errors.Wrapf(err, "finding dependent intent pods with family '%s'", j.Family))
		return
	}
	if len(dependents) == 0 {
		// No-op if there are no pods that need this definition to be created.
		return
	}

	podDefOpts, err := cloud.ExportECSPodDefinitionOptions(&j.settings, j.ContainerOpts)
	if err != nil {
		j.AddError(errors.Wrapf(err, "creating pod definition with family '%s'", j.Family))
		return
	}

	podDef, err := definition.FindOneByFamily(j.Family)
	if err != nil {
		j.AddRetryableError(errors.Wrapf(err, "checking for existing pod definition with family '%s'", j.Family))
		return
	}
	if podDef != nil {
		// No-op if this pod definition already exists.
		return
	}

	item, err := j.podDefMgr.CreatePodDefinition(ctx, *podDefOpts)
	if err != nil {
		j.AddRetryableError(errors.Wrapf(err, "creating pod definition with family '%s'", j.Family))
		return
	}

	grip.Info(message.Fields{
		"message":     "successfully created pod definition",
		"family":      j.Family,
		"external_id": item.ID,
		"job":         j.ID(),
	})
}

func (j *podDefinitionCreationJob) populateIfUnset(ctx context.Context) error {
	if j.env == nil {
		j.env = evergreen.GetEnvironment()
	}

	if j.ecsClient == nil {
		client, err := cloud.MakeECSClient(ctx, j.env.Settings())
		if err != nil {
			return errors.Wrap(err, "initializing ECS client")
		}
		j.ecsClient = client
	}

	if j.podDefMgr == nil {
		podDefMgr, err := cloud.MakeECSPodDefinitionManager(j.ecsClient, nil)
		if err != nil {
			return errors.Wrap(err, "initializing ECS pod definition manager")
		}
		j.podDefMgr = podDefMgr
	}

	return nil
}

// decommissionDependentIntentPods decommissions all intent pods that depend on
// the pod definition created by this job.
func (j *podDefinitionCreationJob) decommissionDependentIntentPods() error {
	podsToDecommission, err := pod.FindIntentByFamily(j.Family)
	if err != nil {
		return errors.Wrap(err, "finding intent pods to decommission")
	}
	catcher := grip.NewBasicCatcher()
	var podIDs []string
	for _, p := range podsToDecommission {
		catcher.Wrapf(p.UpdateStatus(pod.StatusDecommissioned, "pod definition could not be created"), "pod '%s'", p.ID)
		podIDs = append(podIDs, p.ID)
	}

	grip.InfoWhen(len(podIDs) > 0, message.Fields{
		"message": "decommissioned dependent intent pods that have failed to create the pod definition",
		"pod_ids": podIDs,
		"family":  j.Family,
		"job":     j.ID(),
	})

	return errors.Wrap(catcher.Resolve(), "decommissioning intent pods")
}
