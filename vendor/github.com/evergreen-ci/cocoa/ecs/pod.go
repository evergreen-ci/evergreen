package ecs

import (
	"context"

	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// BasicECSPod represents a pod that is backed by AWS ECS.
type BasicECSPod struct {
	client     cocoa.ECSClient
	vault      cocoa.Vault
	resources  cocoa.ECSPodResources
	statusInfo cocoa.ECSPodStatusInfo
}

// BasicECSPodOptions are options to create a basic ECS pod.
type BasicECSPodOptions struct {
	Client     cocoa.ECSClient
	Vault      cocoa.Vault
	Resources  *cocoa.ECSPodResources
	StatusInfo *cocoa.ECSPodStatusInfo
}

// NewBasicECSPodOptions returns new uninitialized options to create a basic ECS
// pod.
func NewBasicECSPodOptions() *BasicECSPodOptions {
	return &BasicECSPodOptions{}
}

// SetClient sets the client the pod uses to communicate with ECS.
func (o *BasicECSPodOptions) SetClient(c cocoa.ECSClient) *BasicECSPodOptions {
	o.Client = c
	return o
}

// SetVault sets the vault that the pod uses to manage secrets.
func (o *BasicECSPodOptions) SetVault(v cocoa.Vault) *BasicECSPodOptions {
	o.Vault = v
	return o
}

// SetResources sets the resources used by the pod.
func (o *BasicECSPodOptions) SetResources(res cocoa.ECSPodResources) *BasicECSPodOptions {
	o.Resources = &res
	return o
}

// SetStatusInfo sets the current status for the pod.
func (o *BasicECSPodOptions) SetStatusInfo(s cocoa.ECSPodStatusInfo) *BasicECSPodOptions {
	o.StatusInfo = &s
	return o
}

// Validate checks that the required parameters to initialize a pod are given.
func (o *BasicECSPodOptions) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(o.Client == nil, "must specify a client")
	if o.Resources != nil {
		catcher.Wrap(o.Resources.Validate(), "invalid resources")
	} else {
		catcher.New("missing pod resources")
	}
	if o.StatusInfo != nil {
		catcher.Add(o.StatusInfo.Validate())
	} else {
		catcher.New("must specify status information")
	}
	return catcher.Resolve()
}

// MergeECSPodOptions merges all the given options describing an ECS pod.
// Options are applied in the order that they're specified and conflicting
// options are overwritten.
func MergeECSPodOptions(opts ...*BasicECSPodOptions) BasicECSPodOptions {
	merged := BasicECSPodOptions{}

	for _, opt := range opts {
		if opt == nil {
			continue
		}

		if opt.Client != nil {
			merged.Client = opt.Client
		}

		if opt.Vault != nil {
			merged.Vault = opt.Vault
		}

		if opt.Resources != nil {
			merged.Resources = opt.Resources
		}

		if opt.StatusInfo != nil {
			merged.StatusInfo = opt.StatusInfo
		}
	}

	return merged
}

// NewBasicECSPod initializes a new pod that is backed by ECS.
func NewBasicECSPod(opts ...*BasicECSPodOptions) (*BasicECSPod, error) {
	merged := MergeECSPodOptions(opts...)
	if err := merged.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid options")
	}
	return &BasicECSPod{
		client:     merged.Client,
		vault:      merged.Vault,
		resources:  *merged.Resources,
		statusInfo: *merged.StatusInfo,
	}, nil
}

// Resources returns information about the resources used by the pod.
func (p *BasicECSPod) Resources() cocoa.ECSPodResources {
	return p.resources
}

// StatusInfo returns the cached status information for the pod.
func (p *BasicECSPod) StatusInfo() cocoa.ECSPodStatusInfo {
	return p.statusInfo
}

// LatestStatusInfo returns the most up-to-date status information for the pod.
func (p *BasicECSPod) LatestStatusInfo(ctx context.Context) (*cocoa.ECSPodStatusInfo, error) {
	out, err := p.client.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: p.resources.Cluster,
		Tasks:   []*string{p.resources.TaskID},
	})
	if err != nil {
		return nil, errors.Wrap(err, "describing task")
	}

	if len(out.Failures) != 0 {
		catcher := grip.NewBasicCatcher()
		for _, failure := range out.Failures {
			catcher.Errorf("%s: %s\n", utility.FromStringPtr(failure.Detail), utility.FromStringPtr(failure.Reason))
		}
		return nil, errors.Wrap(catcher.Resolve(), "describing task")
	}
	if len(out.Tasks) == 0 {
		return nil, errors.New("expected a task to exist in ECS, but none was returned")
	}

	p.statusInfo = translatePodStatusInfo(out.Tasks[0])

	return &p.statusInfo, nil
}

// Stop stops the running pod without cleaning up any of its underlying
// resources.
func (p *BasicECSPod) Stop(ctx context.Context) error {
	switch p.statusInfo.Status {
	case cocoa.StatusStopped, cocoa.StatusDeleted:
		return nil
	}

	var stopTask ecs.StopTaskInput
	stopTask.SetCluster(utility.FromStringPtr(p.resources.Cluster)).SetTask(utility.FromStringPtr(p.resources.TaskID))

	if _, err := p.client.StopTask(ctx, &stopTask); err != nil {
		return errors.Wrap(err, "stopping pod")
	}

	p.statusInfo.Status = cocoa.StatusStopped
	for i := range p.statusInfo.Containers {
		p.statusInfo.Containers[i].Status = cocoa.StatusStopped
	}

	return nil
}

// Delete deletes the pod and its owned resources.
func (p *BasicECSPod) Delete(ctx context.Context) error {
	catcher := grip.NewBasicCatcher()

	catcher.Wrap(p.Stop(ctx), "stopping pod")

	if utility.FromBoolPtr(p.resources.TaskDefinition.Owned) {
		var deregisterDef ecs.DeregisterTaskDefinitionInput
		deregisterDef.SetTaskDefinition(utility.FromStringPtr(p.resources.TaskDefinition.ID))

		if _, err := p.client.DeregisterTaskDefinition(ctx, &deregisterDef); err != nil {
			catcher.Wrap(err, "deregistering task definition")
		}
	}

	for _, c := range p.resources.Containers {
		for _, s := range c.Secrets {
			if !utility.FromBoolPtr(s.Owned) {
				continue
			}

			id := utility.FromStringPtr(s.ID)
			catcher.Wrapf(p.vault.DeleteSecret(ctx, id), "deleting secret '%s' for container '%s'", id, utility.FromStringPtr(c.Name))
		}
	}

	if catcher.HasErrors() {
		return catcher.Resolve()
	}

	p.statusInfo.Status = cocoa.StatusDeleted
	for i := range p.statusInfo.Containers {
		p.statusInfo.Containers[i].Status = cocoa.StatusDeleted
	}

	return nil
}
