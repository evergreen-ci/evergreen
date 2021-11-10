package units

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
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
	j.SetDependency(dependency.NewAlways())
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
