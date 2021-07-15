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
	client    cocoa.ECSClient
	vault     cocoa.Vault
	resources cocoa.ECSPodResources
	status    cocoa.ECSPodStatus
}

// BasicECSPodOptions are options to create a basic ECS pod.
type BasicECSPodOptions struct {
	Client    cocoa.ECSClient
	Vault     cocoa.Vault
	Resources *cocoa.ECSPodResources
	Status    *cocoa.ECSPodStatus
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

// SetStatus sets the current status for the pod.
func (o *BasicECSPodOptions) SetStatus(s cocoa.ECSPodStatus) *BasicECSPodOptions {
	o.Status = &s
	return o
}

// Validate checks that the required parameters to initialize a pod are given.
func (o *BasicECSPodOptions) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(o.Client == nil, "must specify a client")
	catcher.NewWhen(o.Resources == nil, "must specify at least one underlying resource being used by the pod")
	catcher.NewWhen(o.Resources != nil && o.Resources.TaskID == nil, "must specify task ID")
	if o.Status != nil {
		catcher.Add(o.Status.Validate())
	} else {
		catcher.New("must specify a status")
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

		if opt.Status != nil {
			merged.Status = opt.Status
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
		client:    merged.Client,
		vault:     merged.Vault,
		resources: *merged.Resources,
		status:    *merged.Status,
	}, nil
}

// Info returns information about the current state of the pod.
func (p *BasicECSPod) Info(ctx context.Context) (*cocoa.ECSPodInfo, error) {
	return &cocoa.ECSPodInfo{
		Status:    p.status,
		Resources: p.resources,
	}, nil
}

// Stop stops the running pod without cleaning up any of its underlying
// resources.
func (p *BasicECSPod) Stop(ctx context.Context) error {
	if p.status != cocoa.Running {
		return errors.Errorf("pod can only be stopped when status is '%s', but current status is '%s'", cocoa.Running, p.status)
	}

	stopTask := &ecs.StopTaskInput{}
	stopTask.SetCluster(utility.FromStringPtr(p.resources.Cluster)).SetTask(utility.FromStringPtr(p.resources.TaskID))

	if _, err := p.client.StopTask(ctx, stopTask); err != nil {
		return errors.Wrap(err, "stopping pod")
	}

	p.status = cocoa.Stopped

	return nil
}

// Delete deletes the pod and its owned resources.
func (p *BasicECSPod) Delete(ctx context.Context) error {
	if err := p.Stop(ctx); err != nil {
		return errors.Wrap(err, "stopping pod")
	}

	if utility.FromBoolPtr(p.resources.TaskDefinition.Owned) {
		deregisterDef := ecs.DeregisterTaskDefinitionInput{}
		deregisterDef.SetTaskDefinition(utility.FromStringPtr(p.resources.TaskDefinition.ID))

		if _, err := p.client.DeregisterTaskDefinition(ctx, &deregisterDef); err != nil {
			return errors.Wrap(err, "deregistering task definition")
		}
	}

	for _, secret := range p.resources.Secrets {
		if utility.FromBoolPtr(secret.Owned) {
			if err := p.vault.DeleteSecret(ctx, utility.FromStringPtr(secret.Name)); err != nil {
				return errors.Wrap(err, "deleting secret")
			}
		}
	}

	p.status = cocoa.Deleted

	return nil
}
