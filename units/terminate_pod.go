package units

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/cocoa/awsutil"
	"github.com/evergreen-ci/cocoa/ecs"
	"github.com/evergreen-ci/cocoa/secret"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/dependency"
	"github.com/mongodb/amboy/job"
	"github.com/mongodb/amboy/registry"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const terminatePodJobName = "terminate-pod"

func init() {
	registry.AddJobType(terminatePodJobName, func() amboy.Job {
		return makeTerminatePodJob()
	})
}

type terminatePodJob struct {
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

func makeTerminatePodJob() *terminatePodJob {
	j := &terminatePodJob{
		Base: job.Base{
			JobType: amboy.JobType{
				Name:    terminatePodJobName,
				Version: 0,
			},
		},
	}
	j.SetDependency(dependency.NewAlways())
	return j
}

// NewTerminatePodJob creates a job to terminate the given pod with the given
// termination reason. Callers should populate the reason with as much context
// as possible for why the pod is being terminated.
func NewTerminatePodJob(env evergreen.Environment, p *pod.Pod, reason string) amboy.Job {
	j := makeTerminatePodJob()
	j.pod = p
	j.SetScopes([]string{fmt.Sprintf("%s.%s", terminatePodJobName, p.ID)})
	j.SetShouldApplyScopesOnEnqueue(true)

	return j
}

func (j *terminatePodJob) Run(ctx context.Context) {
	defer j.MarkComplete()

	if err := j.populateIfUnset(ctx); err != nil {
		j.AddError(err)
		return
	}

	defer func() {
		if j.smClient != nil {
			j.AddError(j.smClient.Close(ctx))
		}
		if j.ecsClient != nil {
			j.AddError(j.ecsClient.Close(ctx))
		}
	}()

	switch j.pod.Status {
	case pod.InitializingStatus:
		grip.Info(message.Fields{
			"message":                    "not deleting resources because pod has not initialized any yet",
			"pod":                        j.PodID,
			"termination_attempt_reason": j.Reason,
			"job":                        j.ID(),
		})
	case pod.StartingStatus, pod.RunningStatus:
		if err := j.ecsPod.Delete(ctx); err != nil {
			j.AddError(errors.Wrap(err, "deleting pod resources"))
			return
		}
	case pod.TerminatedStatus:
		grip.Info(message.Fields{
			"message":                    "pod is already terminated",
			"pod":                        j.PodID,
			"termination_attempt_reason": j.Reason,
			"job":                        j.ID(),
		})
	}

	if err := j.pod.UpdateStatus(pod.TerminatedStatus); err != nil {
		j.AddError(errors.Wrap(err, "marking pod as terminated"))
	}
}

func (j *terminatePodJob) populateIfUnset(ctx context.Context) (populateErr error) {
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
			return errors.New("pod not founnd")
		}
		j.pod = p
	}

	settings := j.env.Settings()

	defer func() {
		if populateErr == nil {
			return
		}

		if j.smClient != nil {
			j.AddError(j.smClient.Close(ctx))
		}

		if j.ecsClient != nil {
			j.AddError(j.ecsClient.Close(ctx))
		}
	}()

	if j.vault == nil {
		if j.smClient == nil {
			client, err := secret.NewBasicSecretsManagerClient(j.exportAWSOpts(settings))
			if err != nil {
				return errors.Wrap(err, "initializing Secrets Manager client")
			}
			j.smClient = client
		}

		j.vault = secret.NewBasicSecretsManager(j.smClient)
	}

	if j.ecsClient == nil {
		// This intentionally does not share the AWS options between the Secrets
		// Manager and ECS clients to avoid dependencies between each other's
		// HTTP clients.
		client, err := ecs.NewBasicECSClient(j.exportAWSOpts(settings))
		if err != nil {
			return errors.Wrap(err, "initializing ECS client")
		}
		j.ecsClient = client
	}

	// kim: TODO: add converter from DB pod status to cocoa pod status
	// kim: TODO: add converter from DB pod resources to cocoa pod resources.
	opts := ecs.NewBasicECSPodOptions().SetClient(j.ecsClient).SetVault(j.vault)

	ecsPod, err := ecs.NewBasicECSPod(opts)
	if err != nil {
		return errors.Wrap(err, "initializing ECS pod")
	}

	j.ecsPod = ecsPod

	return nil
}

// kim: TODO: make AWS options from global settings
// kim: TODO: wait for awsutil change to merge
func (j *terminatePodJob) exportAWSOpts(settings *evergreen.Settings) awsutil.ClientOptions {
	opts := *awsutil.NewClientOptions().
		SetRegion(settings.Providers.AWS.Pod.Region).
		SetRole(settings.Providers.AWS.Pod.Role)

		// kim: TODO: deal with populated AWS credentials if they're set
	// if settings.AWS.Pod.Creds != nil {
	//
	// }

	return opts
}
