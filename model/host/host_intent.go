package host

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

const defaultMaxImagesPerParent = 3

// CreateOptions is a struct of options that are commonly passed around when creating a
// new cloud host.
type CreateOptions struct {
	ProvisionOptions   *ProvisionOptions
	ExpirationDuration *time.Duration
	Region             string
	UserName           string
	UserHost           bool

	HasContainers         bool
	ParentID              string
	ContainerPoolSettings *evergreen.ContainerPool
	SpawnOptions          SpawnOptions
	DockerOptions         DockerOptions
	InstanceTags          []Tag
	InstanceType          string
	NoExpiration          bool
	IsVirtualWorkstation  bool
	IsCluster             bool
	HomeVolumeSize        int
	HomeVolumeID          string
}

// NewIntent creates an IntentHost using the given host settings. An IntentHost is a host that
// does not exist yet but is intended to be picked up by the hostinit package and started. This
// function takes distro information, the name of the instance, the provider of the instance and
// a CreateOptions and returns an IntentHost.
func NewIntent(d distro.Distro, instanceName, provider string, options CreateOptions) *Host {
	creationTime := time.Now()
	// proactively write all possible information pertaining
	// to the host we want to create. this way, if we are unable
	// to start it or record its instance id, we have a way of knowing
	// something went wrong - and what

	intentHost := &Host{
		Id:                    instanceName,
		User:                  d.User,
		Distro:                d,
		Tag:                   instanceName,
		CreationTime:          creationTime,
		Status:                evergreen.HostUninitialized,
		TerminationTime:       utility.ZeroTime,
		Provider:              provider,
		StartedBy:             options.UserName,
		UserHost:              options.UserHost,
		HasContainers:         options.HasContainers,
		ParentID:              options.ParentID,
		ContainerPoolSettings: options.ContainerPoolSettings,
		SpawnOptions:          options.SpawnOptions,
		DockerOptions:         options.DockerOptions,
		InstanceTags:          options.InstanceTags,
		InstanceType:          options.InstanceType,
		IsVirtualWorkstation:  options.IsVirtualWorkstation,
		HomeVolumeSize:        options.HomeVolumeSize,
		HomeVolumeID:          options.HomeVolumeID,
		NoExpiration:          options.NoExpiration,
	}

	if options.ExpirationDuration != nil {
		intentHost.ExpirationTime = creationTime.Add(*options.ExpirationDuration)
	}
	if options.ProvisionOptions != nil {
		intentHost.ProvisionOptions = options.ProvisionOptions
	}
	return intentHost
}

func newIntentFromHost(h *Host) *Host {
	newID := h.Distro.GenerateName()
	intentHost := &Host{
		Id:                    newID,
		Tag:                   newID,
		CreationTime:          time.Now(),
		Status:                evergreen.HostUninitialized,
		TerminationTime:       utility.ZeroTime,
		User:                  h.User,
		Distro:                h.Distro,
		Provider:              h.Provider,
		StartedBy:             h.StartedBy,
		UserHost:              h.UserHost,
		HasContainers:         h.HasContainers,
		ParentID:              h.ParentID,
		ContainerPoolSettings: h.ContainerPoolSettings,
		SpawnOptions:          h.SpawnOptions,
		DockerOptions:         h.DockerOptions,
		InstanceTags:          h.InstanceTags,
		InstanceType:          h.InstanceType,
		IsVirtualWorkstation:  h.IsVirtualWorkstation,
		HomeVolumeSize:        h.HomeVolumeSize,
		HomeVolumeID:          h.HomeVolumeID,
		NoExpiration:          h.NoExpiration,
		ExpirationTime:        h.ExpirationTime,
		ProvisionOptions:      h.ProvisionOptions,
	}

	return intentHost
}

// partitionParents will split parent hosts based on those that already have/will have the image for this distro
// it does not handle scenarios where the image for a distro has changed recently or multiple app servers calling this
// and racing each other, on the assumption that having to download a small number of extra images is not a big deal
func partitionParents(parents []ContainersOnParents, distro string, maxImages int) ([]ContainersOnParents, []ContainersOnParents) {
	matched := []ContainersOnParents{}
	notMatched := []ContainersOnParents{}
parentLoop:
	for _, parent := range parents {
		currentImages := map[string]bool{}
		for _, c := range parent.Containers {
			currentImages[c.Distro.Id] = true
			if c.Distro.Id == distro {
				matched = append(matched, parent)
				continue parentLoop
			}
		}
		// only return parent hosts that would be below the max if they downloaded another image
		if len(currentImages) < maxImages {
			notMatched = append(notMatched, parent)
		}
	}
	return matched, notMatched
}

// generateParentCreateOptions generates host options for a parent host
func generateParentCreateOptions(pool *evergreen.ContainerPool) CreateOptions {
	options := CreateOptions{
		HasContainers:         true,
		UserName:              evergreen.User,
		ContainerPoolSettings: pool,
	}
	return options
}

func MakeContainersAndParents(d distro.Distro, pool *evergreen.ContainerPool, newContainersNeeded int, hostOptions CreateOptions) ([]Host, []Host, error) {
	// get the parents that are running and split into ones that already have a container from this distro
	currentHosts, err := GetContainersOnParents(d)
	if err != nil {
		return nil, nil, errors.Wrap(err, "Could not find number of containers on parents")
	}
	maxImages := defaultMaxImagesPerParent
	if pool.MaxImages > 0 {
		maxImages = pool.MaxImages
	}
	matched, notMatched := partitionParents(currentHosts, d.Id, maxImages)
	existingHosts := append(matched, notMatched...)

	// add containers to existing parents
	containersToInsert := []Host{}
	parentsToInsert := []Host{}
	containersLeftToCreate := newContainersNeeded
	for _, parent := range existingHosts {
		intents := makeContainerIntentsForParent(parent, containersLeftToCreate, d, hostOptions)
		containersToInsert = append(containersToInsert, intents...)
		containersLeftToCreate -= len(intents)
		if containersLeftToCreate <= 0 {
			return containersToInsert, parentsToInsert, nil
		}
	}

	// create new parents and add containers to them
	numNewParents, _, err := getNumNewParentsAndHostsToSpawn(pool, containersLeftToCreate, false)
	if err != nil {
		return nil, nil, err
	}
	parentDistro, err := distro.FindByIdWithDefaultSettings(pool.Distro)
	if err != nil {
		return nil, nil, errors.Wrap(err, "error finding distro")
	}
	if parentDistro == nil {
		return nil, nil, errors.Errorf("distro %s not found", pool.Distro)
	}
	for i := 0; i < numNewParents; i++ {
		newParent := NewIntent(*parentDistro, parentDistro.GenerateName(), parentDistro.Provider, generateParentCreateOptions(pool))
		parentsToInsert = append(parentsToInsert, *newParent)
		parentInfo := ContainersOnParents{ParentHost: *newParent}
		intents := makeContainerIntentsForParent(parentInfo, containersLeftToCreate, d, hostOptions)
		containersToInsert = append(containersToInsert, intents...)
		containersLeftToCreate -= len(intents)
	}

	return containersToInsert, parentsToInsert, nil
}

func makeContainerIntentsForParent(parent ContainersOnParents, newContainersNeeded int, d distro.Distro, hostOptions CreateOptions) []Host {
	// find out how many more containers this parent can fit
	containerSpace := parent.ParentHost.ContainerPoolSettings.MaxContainers - len(parent.Containers)
	containersToCreate := containerSpace
	// only create containers as many as we need
	if newContainersNeeded < containerSpace {
		containersToCreate = newContainersNeeded
	}
	containerHostIntents := []Host{}
	for i := 0; i < containersToCreate; i++ {
		hostOptions.ParentID = parent.ParentHost.Id
		containerHostIntents = append(containerHostIntents, *NewIntent(d, d.GenerateName(), d.Provider, hostOptions))
	}
	return containerHostIntents
}
